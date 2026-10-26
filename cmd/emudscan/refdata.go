package main

// Reference data about the October 2026 eMudhra intermediate CA compromise.
//
// The files of the data folder (package data, at the root of the repository) come
// from https://github.com/galadran/we-have-a-problem.com (dataSourceURL)
// (Copyright 2026 Dennis Jackson and the we-have-a-problem.com contributors,
// Apache License 2.0). impacted-intermediates.json is the same file that the
// site's Block-EmudhraCompromisedCA.ps1 script pins by SHA-256.

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
	"time"

	refdata "github.com/3c0/eMudScan/data"
)

// dataFS is the embedded reference data (see the data folder).
var dataFS = refdata.FS

// licensesText is what the About box shows under "Licenses and notices": the
// notice and the license of the reference data (Apache License 2.0, section 4
// asks for both with any copy), then the license of Go, whose standard library is
// built into the program (its license asks for its notice in binary copies).
func licensesText() string {
	read := func(name string) string {
		b, _ := dataFS.ReadFile(name)
		t := strings.ReplaceAll(string(b), "\r\n", "\n")
		return strings.TrimRight(strings.TrimLeft(t, "\n"), " \n") // keeps the indent of the first line
	}
	rule := strings.Repeat("─", 72)
	parts := []string{
		T("lic.intro"),
		rule + "\n" + T("lic.app") + "\n" + rule,
		read("LICENSE-emudscan.txt"),
		rule + "\n" + T("lic.data") + "\n" + rule,
		read("NOTICE-we-have-a-problem.txt"),
		read("LICENSE-we-have-a-problem.txt"),
		rule + "\n" + T("lic.go") + "\n" + rule,
		read("LICENSE-go.txt"),
	}
	return strings.Join(parts, "\n\n") + "\n"
}

// manifestSHA256 is the hash pinned by the advisory's own blocking script.
const manifestSHA256 = "F67B3ABE9B4879F4E91874ED041BECCD71257F1DB41C588C724CCF12AA90C59E"

// Microsoft started distributing its block on this date.
var microsoftFixDate = time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)

// The test chains were issued on 20 and 22 October 2026 and expire in May 2027.
// Verifying at a fixed date keeps results stable however old this program gets.
var chainVerifyTime = time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)

type badCert struct {
	Name       string `json:"name"`
	Issuer     string `json:"issuer"`
	IssuedBy   string `json:"issued_by"`
	Usage      string `json:"usage"`
	SHA256     string `json:"sha256"`
	SPKISHA256 string `json:"spki_sha256"`
	PEM        string `json:"pem"`

	DER  []byte            `json:"-"`
	Cert *x509.Certificate `json:"-"`
}

// CrossSigned reports whether this is the Sectigo (AAA Certificate Services) cross-sign.
func (b *badCert) CrossSigned() bool { return strings.Contains(b.Issuer, "AAA Certificate Services") }

type rootCert struct {
	Name       string `json:"name"`
	Role       string `json:"role"`
	SHA256     string `json:"sha256"`
	SPKISHA256 string `json:"spki_sha256"`
	PEM        string `json:"pem"`

	DER  []byte            `json:"-"`
	Cert *x509.Certificate `json:"-"`
}

type testChain struct {
	Host          string
	AAA           bool // chain goes through the Sectigo cross-sign
	NoCT          bool
	Leaf          *x509.Certificate
	Intermediates []*x509.Certificate
}

var (
	badCerts   []*badCert  // the 16 compromised certificates (8 intermediates + 8 cross-signs)
	rootCerts  []*rootCert // roots that lead to them
	testChains []*testChain
)

const (
	rootNameECCG3  = "emSign ECC Root CA - G3"
	rootNameTLSG3  = "emSign Root TLS CA - G3"
	rootNameCSG3   = "emSign Root CS CA - G3"
	rootNameAAA    = "AAA Certificate Services"
	tlsChainsTotal = 12 // 6 direct chains + 6 Sectigo cross-signed chains (CT-less ones excluded)
)

func decodeOnePEM(s string) ([]byte, error) {
	b, _ := pem.Decode([]byte(s))
	if b == nil || b.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("invalid PEM")
	}
	return b.Bytes, nil
}

func hexSHA256(b []byte) string {
	h := sha256.Sum256(b)
	return strings.ToUpper(hex.EncodeToString(h[:]))
}

func loadData() error {
	raw, err := dataFS.ReadFile("impacted-intermediates.json")
	if err != nil {
		return err
	}
	if got := hexSHA256(raw); got != manifestSHA256 {
		return fmt.Errorf("embedded data corrupted (hash %s)", got)
	}
	if err := json.Unmarshal(raw, &badCerts); err != nil {
		return err
	}
	if len(badCerts) != 16 {
		return fmt.Errorf("expected 16 certificates, got %d", len(badCerts))
	}
	for _, c := range badCerts {
		if c.DER, err = decodeOnePEM(c.PEM); err != nil {
			return fmt.Errorf("%s: %v", c.Name, err)
		}
		if hexSHA256(c.DER) != strings.ToUpper(c.SHA256) {
			return fmt.Errorf("%s: fingerprint mismatch", c.Name)
		}
		if c.Cert, err = x509.ParseCertificate(c.DER); err != nil {
			return fmt.Errorf("%s: %v", c.Name, err)
		}
		c.SHA256 = strings.ToUpper(c.SHA256)
		c.SPKISHA256 = strings.ToUpper(c.SPKISHA256)
	}

	raw, err = dataFS.ReadFile("roots.json")
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, &rootCerts); err != nil {
		return err
	}
	for _, r := range rootCerts {
		if r.DER, err = decodeOnePEM(r.PEM); err != nil {
			return fmt.Errorf("%s: %v", r.Name, err)
		}
		if r.Cert, err = x509.ParseCertificate(r.DER); err != nil {
			return fmt.Errorf("%s: %v", r.Name, err)
		}
		r.SHA256 = strings.ToUpper(r.SHA256)
	}

	entries, err := fs.ReadDir(dataFS, "chains")
	if err != nil {
		return err
	}
	for _, e := range entries {
		raw, err := dataFS.ReadFile(path.Join("chains", e.Name()))
		if err != nil {
			return err
		}
		var certs []*x509.Certificate
		for rest := raw; ; {
			var b *pem.Block
			b, rest = pem.Decode(rest)
			if b == nil {
				break
			}
			c, err := x509.ParseCertificate(b.Bytes)
			if err != nil {
				return fmt.Errorf("%s: %v", e.Name(), err)
			}
			certs = append(certs, c)
		}
		if len(certs) < 2 {
			return fmt.Errorf("%s: incomplete chain", e.Name())
		}
		host := strings.TrimSuffix(e.Name(), ".pem")
		testChains = append(testChains, &testChain{
			Host:          host,
			AAA:           strings.Contains(host, "-aaa-"),
			NoCT:          strings.Contains(host, "-noct-"),
			Leaf:          certs[0],
			Intermediates: certs[1:],
		})
	}
	sort.Slice(testChains, func(i, j int) bool { return testChains[i].Host < testChains[j].Host })
	n := 0
	for _, c := range testChains {
		if !c.NoCT {
			n++
		}
	}
	if n != tlsChainsTotal {
		return fmt.Errorf("expected %d test chains, got %d", tlsChainsTotal, n)
	}
	return nil
}

func rootByName(name string) *rootCert {
	for _, r := range rootCerts {
		if r.Name == name {
			return r
		}
	}
	return nil
}

// badCertBySHA256 maps an upper-case hex SHA-256 to the compromised certificate.
func badCertBySHA256(h string) *badCert {
	for _, c := range badCerts {
		if c.SHA256 == h {
			return c
		}
	}
	return nil
}

// uniqueBadSPKIs returns the SPKI SHA-256 of the 8 compromised keys.
func uniqueBadSPKIs() []string {
	seen := map[string]bool{}
	var out []string
	for _, c := range badCerts {
		if !seen[c.SPKISHA256] {
			seen[c.SPKISHA256] = true
			out = append(out, c.SPKISHA256)
		}
	}
	return out
}
