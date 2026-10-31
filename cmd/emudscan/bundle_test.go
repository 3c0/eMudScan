package main

import (
	"bytes"
	"crypto/x509"
	"encoding/asn1"
	"encoding/pem"
	"os"
	"strings"
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
		{"python", "3.13.18", FixOK},
		{"python", "3.13.17", FixOutdated},
		{"python", "3.12.16", FixOK},
		{"python", "3.12.9", FixOutdated},
		{"python", "3.11.18", FixOK},
		{"python", "3.10.19", FixUnsupported},
		{"python", "3.9.25", FixUnsupported},
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

// An outdated Git gets its update and nothing else; a Git of unknown version may also
// have its file replaced; a copy inside another program gets that program's update,
// or its file replaced.
func TestGitBundleAdvice(t *testing.T) {
	Lang = 0
	files := []string{`C:\Program Files\Git\mingw64\etc\ssl\certs\ca-bundle.crt`}
	update := Tf("git.fix", fixedVersionsList("git"))
	if got := gitBundleAdvice(FixOutdated, false, ownerInfo{Name: "Git"}, files); got != update {
		t.Errorf("outdated Git: %q", got)
	}
	if got := gitBundleAdvice(FixUnknown, false, ownerInfo{Name: "Git"}, files); !strings.HasPrefix(got, update) || !strings.Contains(got, "Copy-Item") {
		t.Errorf("Git of unknown version: %q", got)
	}
	o := ownerInfo{Name: "ESPHome Device Builder", URL: "https://esphome.io"}
	if got := gitBundleAdvice(FixOutdated, true, o, files); strings.Contains(got, update) || !strings.Contains(got, "ESPHome Device Builder") || !strings.Contains(got, "Copy-Item") {
		t.Errorf("Git inside another program: %q", got)
	}
}

// The current list is downloaded once, into memory, and written directly over each
// file: no file in between that another program could replace. It is used only when it
// matches the SHA-256 that curl publishes, and each file is written only once its
// backup, named after the time the command runs, is made.
func TestReplaceBundleFix(t *testing.T) {
	Lang = 0
	fix := replaceBundleFix(`C:\Program Files\App\cacert.pem`, `C:\Users\o'neil\$x\cert.pem`)
	if strings.Contains(fix, "TEMP") || strings.Contains(fix, "-OutFile") || strings.Count(fix, "-Uri https://curl.se/ca/cacert.pem)") != 1 {
		t.Errorf("download: %s", fix)
	}
	at := 0
	for _, w := range []string{
		"$ $cacert = $null; $cacert = (Invoke-WebRequest",
		"$ $sha256 = $null; $sha256 = ([Text.Encoding]::ASCII.GetString((Invoke-WebRequest -UseBasicParsing -Uri https://curl.se/ca/cacert.pem.sha256)",
		"$ $ok = $cacert -and $sha256 -and (Get-FileHash -InputStream ([IO.MemoryStream]::new($cacert))).Hash -eq $sha256",
		"$ if (-not $ok) { Write-Warning " + psQuote(T("fix.downloadFailed")) + " }",
		`$ if ($ok -and (Copy-Item -LiteralPath 'C:\Program Files\App\cacert.pem' -Destination ('C:\Program Files\App\cacert.pem.emudscan-' + (Get-Date -Format 'yyyyMMdd-HHmmss') + '.bak') -PassThru)) { [IO.File]::WriteAllBytes('C:\Program Files\App\cacert.pem', $cacert) }`,
		`$ if ($ok -and (Copy-Item -LiteralPath 'C:\Users\o''neil\$x\cert.pem' -Destination ('C:\Users\o''neil\$x\cert.pem.emudscan-' + (Get-Date -Format 'yyyyMMdd-HHmmss') + '.bak') -PassThru)) { [IO.File]::WriteAllBytes('C:\Users\o''neil\$x\cert.pem', $cacert) }`,
	} {
		i := strings.Index(fix[at:], w)
		if i < 0 {
			t.Fatalf("%q missing or out of order:\n%s", w, fix)
		}
		at += i + len(w)
	}
	if strings.Contains(fix, "\n$ [IO.File]") || strings.Contains(fix, "\n$ Copy-Item") {
		t.Errorf("a file changed outside the checks:\n%s", fix)
	}
}

// trustedPEM writes c in OpenSSL's "trusted certificate" format, with the uses it is
// trusted for and those it is rejected for (no settings at all when both are nil).
func trustedPEM(c *x509.Certificate, trust, reject []asn1.ObjectIdentifier) []byte {
	der := append([]byte{}, c.Raw...)
	if trust != nil || reject != nil {
		list := func(class, tag int, oids []asn1.ObjectIdentifier) []byte {
			var b []byte
			for _, o := range oids {
				m, _ := asn1.Marshal(o)
				b = append(b, m...)
			}
			m, _ := asn1.Marshal(asn1.RawValue{Class: class, Tag: tag, IsCompound: true, Bytes: b})
			return m
		}
		var aux []byte
		if trust != nil {
			aux = append(aux, list(asn1.ClassUniversal, asn1.TagSequence, trust)...)
		}
		if reject != nil {
			aux = append(aux, list(asn1.ClassContextSpecific, 0, reject)...)
		}
		alias, _ := asn1.MarshalWithParams("a root", "utf8")
		m, _ := asn1.Marshal(asn1.RawValue{Class: asn1.ClassUniversal, Tag: asn1.TagSequence, IsCompound: true, Bytes: append(aux, alias...)})
		der = append(der, m...)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "TRUSTED CERTIFICATE", Bytes: der})
}

// A list in OpenSSL's "trusted certificate" format (ca-bundle.trust.crt) is tested
// like any other, with each root's trust settings: a root counts only when OpenSSL
// would accept it for websites.
func TestTrustedCertificateFormat(t *testing.T) {
	email := asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 3, 4}
	tls := []asn1.ObjectIdentifier{oidTLSServer}
	for _, tc := range []struct {
		name          string
		trust, reject []asn1.ObjectIdentifier
		want          int // test chains accepted
	}{
		{"trusted for websites", []asn1.ObjectIdentifier{email, oidTLSServer}, nil, 12},
		{"trusted for any use", []asn1.ObjectIdentifier{oidAnyUse}, nil, 12},
		{"no trust settings", nil, nil, 12},
		{"trusted for e-mail only", []asn1.ObjectIdentifier{email}, nil, 0},
		{"rejected for websites", tls, tls, 0},
		{"rejected for any use", nil, []asn1.ObjectIdentifier{oidAnyUse}, 0},
	} {
		var b bytes.Buffer
		for _, r := range rootCerts {
			b.Write(trustedPEM(r.Cert, tc.trust, tc.reject))
		}
		for _, c := range testChains { // padding to the size of a trust store, trusted for websites
			b.Write(trustedPEM(c.Leaf, tls, nil))
		}
		br, ok := newBundleCache().checkData(b.Bytes())
		if !ok || br.Trusted != tc.want {
			t.Errorf("%s: list %v, %d test chains accepted, want %d", tc.name, ok, br.Trusted, tc.want)
		}
	}
	// settings that cannot be read: OpenSSL skips the certificate
	if _, ok := trustedCert(append(append([]byte{}, rootCerts[0].Cert.Raw...), 0x30, 0x05, 0x06, 0x01)); ok {
		t.Error("a certificate with unreadable trust settings is accepted")
	}
}
