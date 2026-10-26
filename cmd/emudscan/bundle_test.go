package main

import (
	"crypto/x509"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	if err := loadData(); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

func TestVerifyPool(t *testing.T) {
	g3 := rootByName(rootNameECCG3).Cert
	aaa := rootByName(rootNameAAA).Cert
	tlsg3 := rootByName(rootNameTLSG3).Cert
	cases := []struct {
		name         string
		roots        []*x509.Certificate
		want, wantAA int
	}{
		{"none", nil, 0, 0},
		{"g3", []*x509.Certificate{g3}, 6, 0},
		{"aaa", []*x509.Certificate{aaa}, 6, 6},
		{"both", []*x509.Certificate{g3, aaa}, 12, 6},
		{"tlsg3 (Microsoft root)", []*x509.Certificate{tlsg3}, 6, 0},
	}
	for _, c := range cases {
		got, aa := verifyPool(c.roots)
		if got != c.want || aa != c.wantAA {
			t.Errorf("%s: got %d (%d via AAA), want %d (%d)", c.name, got, aa, c.want, c.wantAA)
		}
	}
}

func TestSystemBundle(t *testing.T) {
	bc := newBundleCache()
	for _, f := range []string{"/etc/ssl/certs/ca-certificates.crt"} {
		r, ok := bc.checkFile(f)
		t.Logf("%s: bundle=%v certs=%d trusted=%d aaa=%d roots=%v", f, ok, r.Certs, r.Trusted, r.TrustedAAA, r.RootsPresent)
	}
}

func TestVersions(t *testing.T) {
	cases := []struct {
		prod, v string
		want    FixState
	}{
		{"git", "2.55.0.windows.5", FixOutdated},
		{"git", "2.56.0.windows.3", FixOK},
		{"git", "2.56.0", FixUnknown},
		{"git", "2.57.0.windows.1", FixOK},
		{"node", "26.12.0", FixOutdated},
		{"node", "26.12.1", FixOK},
		{"node", "25.9.0", FixUnsupported},
		{"node", "20.19.0", FixUnsupported},
		{"node", "27.0.0", FixOK},
		{"python", "3.13.17", FixOK},
		{"python", "3.13.16", FixOutdated},
		{"python", "3.12.9", FixNoRelease},
		{"python", "3.16.0", FixOK},
		{"electron", "44.9.0", FixOutdated},
		{"electron", "45.0.0", FixOK},
		{"electron", "39.2.7", FixUnsupported},
		{"go", "1.27.3", FixOK},
		{"go", "1.25.1", FixUnsupported},
		{"pip", "26.2.1", FixOutdated},
		{"certifi", "2026.10.19", FixOK},
	}
	for _, c := range cases {
		got, _ := fixStatus(c.prod, ParseVersion(c.v))
		if got != c.want {
			t.Errorf("%s %s: got %d want %d", c.prod, c.v, got, c.want)
		}
	}
	if v := ParseVersion("jdk-21.0.13+9"); v.String() != "21.0.13" {
		t.Errorf("jdk version parse: %v", v)
	}
	if v := ParseVersion("1.8.0_511-b32"); v.String() != "1.8.0.511" {
		t.Errorf("jdk8 version parse: %v", v)
	}
}
