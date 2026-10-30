package main

import (
	"bytes"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"encoding/pem"
	"os"
	"sort"
	"sync"
)

// bundleResult says whether a set of trusted roots still accepts the test chains.
type bundleResult struct {
	Certs        int      // certificates parsed
	Trusted      int      // test chains accepted (out of tlsChainsTotal)
	TrustedAAA   int      // ... of which via the Sectigo cross-sign
	RootsPresent []string // affected roots found in the bundle
	Err          error
}

func (b bundleResult) Bad() bool { return b.Trusted > 0 }

// verifyPool runs the 12 test chains (CT-less duplicates excluded) against roots,
// as a TLS library that trusts exactly these roots would: it builds each chain
// from the leaf up to one of the roots, checking each signature, validity dates
// (at chainVerifyTime), CA flags and the server authentication usage.
//
// It does not use x509.Certificate.Verify: from Go 1.27.3, Verify itself refuses
// every chain through the compromised eMudhra intermediates, whatever the roots,
// which would make every bundle look safe.
func verifyPool(roots []*x509.Certificate) (trusted, viaAAA int) {
	bySubject := map[string][]*x509.Certificate{}
	for _, r := range roots {
		bySubject[string(r.RawSubject)] = append(bySubject[string(r.RawSubject)], r)
	}
	for _, tc := range testChains {
		if tc.NoCT {
			continue
		}
		if usableAt(tc.Leaf) && chainsTo(tc.Leaf, tc.Intermediates, bySubject, 0) {
			trusted++
			if tc.AAA {
				viaAAA++
			}
		}
	}
	return
}

// maxChainDepth bounds the intermediates between a leaf and its root.
const maxChainDepth = 5

// chainsTo reports whether c was signed by one of roots, directly or through the
// intermediates.
func chainsTo(c *x509.Certificate, inter []*x509.Certificate, roots map[string][]*x509.Certificate, depth int) bool {
	for _, r := range roots[string(c.RawIssuer)] {
		if usableAt(r) && c.CheckSignatureFrom(r) == nil {
			return true
		}
	}
	if depth >= maxChainDepth {
		return false
	}
	for _, i := range inter {
		if bytes.Equal(i.RawSubject, c.RawIssuer) && !bytes.Equal(i.Raw, c.Raw) && usableAt(i) &&
			c.CheckSignatureFrom(i) == nil && chainsTo(i, inter, roots, depth+1) {
			return true
		}
	}
	return false
}

// usableAt reports whether c is valid at chainVerifyTime and, when it restricts
// its extended key usages, allows server authentication.
func usableAt(c *x509.Certificate) bool {
	if chainVerifyTime.Before(c.NotBefore) || chainVerifyTime.After(c.NotAfter) {
		return false
	}
	if len(c.ExtKeyUsage) == 0 && len(c.UnknownExtKeyUsage) == 0 {
		return true
	}
	for _, u := range c.ExtKeyUsage {
		if u == x509.ExtKeyUsageServerAuth || u == x509.ExtKeyUsageAny {
			return true
		}
	}
	return false
}

// affectedRootsIn lists which of the relevant roots appear among certs (by public key and name).
func affectedRootsIn(certs []*x509.Certificate) []string {
	found := map[string]bool{}
	for _, c := range certs {
		for _, r := range rootCerts {
			if r.Name == rootNameCSG3 {
				continue // code signing only, irrelevant for TLS
			}
			if bytes.Equal(c.RawSubjectPublicKeyInfo, r.Cert.RawSubjectPublicKeyInfo) &&
				bytes.Equal(c.RawSubject, r.Cert.RawSubject) {
				found[r.Name] = true
			}
		}
	}
	var out []string
	for n := range found {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// checkCertList builds a pool from certs and verifies the test chains.
func checkCertList(certs []*x509.Certificate) bundleResult {
	res := bundleResult{Certs: len(certs), RootsPresent: affectedRootsIn(certs)}
	res.Trusted, res.TrustedAAA = verifyPool(certs)
	return res
}

// parsePEMBundle extracts every certificate from PEM data, skipping invalid blocks.
// From OpenSSL's "trusted certificate" format (ca-bundle.trust.crt), it keeps only
// those OpenSSL would accept as roots for websites (see trustedCert).
func parsePEMBundle(data []byte) []*x509.Certificate {
	var certs []*x509.Certificate
	for rest := data; ; {
		var b *pem.Block
		b, rest = pem.Decode(rest)
		if b == nil {
			break
		}
		switch b.Type {
		case "CERTIFICATE":
			if c, err := x509.ParseCertificate(b.Bytes); err == nil {
				certs = append(certs, c)
			}
		case "TRUSTED CERTIFICATE":
			if c, ok := trustedCert(b.Bytes); ok {
				certs = append(certs, c)
			}
		}
	}
	return certs
}

var (
	oidTLSServer = asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 3, 1}
	oidAnyUse    = asn1.ObjectIdentifier{2, 5, 29, 37, 0}
)

// trustedCert reads a certificate in OpenSSL's "trusted certificate" format: the
// certificate, followed by its trust settings (X509_CERT_AUX: a SEQUENCE holding the
// uses it is trusted for, a SEQUENCE OF OBJECT IDENTIFIER, then the uses it is
// rejected for, [0], then a name and a key identifier). As OpenSSL does, it accepts
// the certificate as a root for websites unless it is rejected for server
// authentication (or any use), and, when trust settings are present, only if they
// trust it for server authentication (or any use): a root trusted only for e-mail
// does not count. Settings that cannot be read make OpenSSL skip the certificate.
func trustedCert(der []byte) (*x509.Certificate, bool) {
	var raw asn1.RawValue
	rest, err := asn1.Unmarshal(der, &raw)
	if err != nil {
		return nil, false
	}
	c, err := x509.ParseCertificate(raw.FullBytes)
	if err != nil {
		return nil, false
	}
	if len(rest) == 0 {
		return c, true // no trust settings
	}
	var aux asn1.RawValue
	if _, err := asn1.Unmarshal(rest, &aux); err != nil || aux.Class != asn1.ClassUniversal || aux.Tag != asn1.TagSequence {
		return nil, false
	}
	settings, trusted, rejected := false, false, false
	for in := aux.Bytes; len(in) > 0; {
		var el asn1.RawValue
		if in, err = asn1.Unmarshal(in, &el); err != nil {
			return nil, false
		}
		var uses *bool
		switch {
		case el.Class == asn1.ClassUniversal && el.Tag == asn1.TagSequence:
			uses = &trusted
		case el.Class == asn1.ClassContextSpecific && el.Tag == 0:
			uses = &rejected
		default:
			continue // name, key identifier, other settings
		}
		settings = true
		for oids := el.Bytes; len(oids) > 0; {
			var oid asn1.ObjectIdentifier
			if oids, err = asn1.Unmarshal(oids, &oid); err != nil {
				return nil, false
			}
			if oid.Equal(oidTLSServer) || oid.Equal(oidAnyUse) {
				*uses = true
			}
		}
	}
	if rejected || settings && !trusted {
		return c, false
	}
	return c, true
}

var (
	pemCertMarker        = []byte("-----BEGIN CERTIFICATE-----")
	pemTrustedCertMarker = []byte("-----BEGIN TRUSTED CERTIFICATE-----")
)

// minBundleCerts: smaller files are a single certificate or a client chain, not a trust store.
const minBundleCerts = 10

// bundleCache avoids verifying identical bundles twice (pip copies certifi everywhere).
type bundleCache struct {
	mu sync.Mutex
	m  map[[32]byte]bundleResult
}

func newBundleCache() *bundleCache { return &bundleCache{m: map[[32]byte]bundleResult{}} }

// checkFile returns (result, isBundle). Files with fewer than minBundleCerts
// certificates are not considered trust bundles.
func (bc *bundleCache) checkFile(path string) (bundleResult, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return bundleResult{Err: err}, false
	}
	return bc.checkData(data)
}

func (bc *bundleCache) checkData(data []byte) (bundleResult, bool) {
	if bytes.Count(data, pemCertMarker)+bytes.Count(data, pemTrustedCertMarker) < minBundleCerts {
		return bundleResult{}, false
	}
	key := sha256.Sum256(data)
	bc.mu.Lock()
	if r, ok := bc.m[key]; ok {
		bc.mu.Unlock()
		return r, true
	}
	bc.mu.Unlock()
	certs := parsePEMBundle(data)
	if len(certs) < minBundleCerts {
		return bundleResult{}, false
	}
	r := checkCertList(certs)
	bc.mu.Lock()
	bc.m[key] = r
	bc.mu.Unlock()
	return r, true
}
