package main

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// buildCRLSet writes a CRLSet as Chromium does.
func buildCRLSet(seq int, spkis [][]byte, serials map[string][][]byte) []byte {
	var b64 []string
	for _, h := range spkis {
		b64 = append(b64, base64.StdEncoding.EncodeToString(h))
	}
	hdr, _ := json.Marshal(map[string]any{"Version": 0, "ContentType": "CRLSet", "Sequence": seq, "BlockedSPKIs": b64, "NotAfter": 0})
	var out bytes.Buffer
	binary.Write(&out, binary.LittleEndian, uint16(len(hdr)))
	out.Write(hdr)
	for parent, list := range serials {
		out.WriteString(parent)
		binary.Write(&out, binary.LittleEndian, uint32(len(list)))
		for _, s := range list {
			out.WriteByte(byte(len(s)))
			out.Write(s)
		}
	}
	return out.Bytes()
}

func TestCRLSet(t *testing.T) {
	var spkis [][]byte
	seen := map[string]bool{}
	for _, c := range badCerts {
		if c.Usage == "tls" {
			h := sha256.Sum256(c.Cert.RawSubjectPublicKeyInfo)
			if !seen[string(h[:])] {
				seen[string(h[:])] = true
				spkis = append(spkis, h[:])
			}
		}
	}
	// blocked by public key (the key is shared by an intermediate and its cross-signed copy)
	s, err := parseCRLSet(buildCRLSet(9871, spkis, nil))
	if err != nil {
		t.Fatal(err)
	}
	if b, n := s.blocks(); b != n || n == 0 {
		t.Errorf("by key: %d of %d", b, n)
	}
	// blocked by issuer and serial number
	serials := map[string][][]byte{}
	for _, c := range badCerts {
		if c.Usage != "tls" {
			continue
		}
		for _, r := range rootCerts {
			if bytes.Equal(r.Cert.RawSubject, c.Cert.RawIssuer) {
				p := sha256.Sum256(r.Cert.RawSubjectPublicKeyInfo)
				serials[string(p[:])] = append(serials[string(p[:])], append([]byte{0}, c.Cert.SerialNumber.Bytes()...))
			}
		}
	}
	s, err = parseCRLSet(buildCRLSet(9872, nil, serials))
	if err != nil {
		t.Fatal(err)
	}
	if b, n := s.blocks(); b != n {
		t.Errorf("by serial: %d of %d", b, n)
	}
	// an older list without them
	s, _ = parseCRLSet(buildCRLSet(9000, spkis[:1], nil))
	if b, n := s.blocks(); b == n || b == 0 {
		t.Errorf("partial list: %d of %d", b, n)
	}
	if _, err := parseCRLSet([]byte{5, 0, 'x'}); err == nil {
		t.Error("garbage accepted")
	}

	// found next to a portable browser, newest version first
	dir := t.TempDir()
	exe := filepath.Join(dir, "ungoogled-chromium", "chrome.exe")
	for _, v := range []struct {
		ver  string
		data []byte
	}{{"9000", buildCRLSet(9000, nil, nil)}, {"9871", buildCRLSet(9871, spkis, nil)}} {
		p := filepath.Join(dir, "ungoogled-chromium", "User Data", "CertificateRevocation", v.ver, "crl-set")
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, v.data, 0o644)
	}
	cs, ok := browserCRLSet(exe)
	if !ok || cs.sequence != 9871 || cs.blocked != cs.total {
		t.Errorf("browserCRLSet: %+v %v", cs, ok)
	}
}

func TestInterceptingSoftware(t *testing.T) {
	mk := func(cn, org string) *x509.Certificate {
		key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		tpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: cn, Organization: []string{org}},
			NotBefore: time.Now(), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true}
		der, _ := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
		c, _ := x509.ParseCertificate(der)
		return c
	}
	roots := []*x509.Certificate{
		mk("ISRG Root X1", "Internet Security Research Group"),
		mk("Kaspersky Anti-Virus Personal Root Certificate", "AO Kaspersky Lab"),
		mk("ESET SSL Filter CA", "ESET, spol. s r. o."),
		mk("Kaspersky Anti-Virus Personal Root Certificate", "AO Kaspersky Lab"),
	}
	got := interceptingSoftware(roots)
	if len(got) != 2 || got[0] != "ESET" || got[1] != "Kaspersky" {
		t.Errorf("found %v", got)
	}
	if got := interceptingSoftware(roots[:1]); len(got) != 0 {
		t.Errorf("false positive: %v", got)
	}
}
