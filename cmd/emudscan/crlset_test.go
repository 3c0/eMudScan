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
	"encoding/hex"
	"encoding/json"
	"math/big"
	"os"
	"path/filepath"
	"strings"
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
	exe := filepath.Join(dir, "thorium", "chrome.exe")
	for _, v := range []struct {
		ver  string
		data []byte
	}{{"9000", buildCRLSet(9000, nil, nil)}, {"9871", buildCRLSet(9871, spkis, nil)}} {
		p := filepath.Join(dir, "thorium", "User Data", "CertificateRevocation", v.ver, "crl-set")
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

// An official Chromium browser is judged by the block list it has downloaded; when
// that copy is older than the block of the compromised intermediates, the block
// list built into recent versions protects it all the same.
func TestOfficialVerdict(t *testing.T) {
	Lang = 0
	var spkis [][]byte
	for _, h := range uniqueBadSPKIs() {
		b, _ := hex.DecodeString(h)
		spkis = append(spkis, b)
	}
	root := t.TempDir() // a browser that keeps its User Data next to its Application folder
	exe := filepath.Join(root, "Chrome", "Application", "chrome.exe")
	lib := filepath.Join(filepath.Dir(exe), "156.0.8078.73", "chrome.dll")
	crl := filepath.Join(root, "Chrome", "User Data", "CertificateRevocation", "9871", "crl-set")
	write := func(p string, data []byte) {
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(exe, []byte("MZ"))
	for _, c := range []struct {
		name    string
		crl     []byte // nil: none downloaded
		engine  string
		st      Status
		summary string
		file    string
	}{
		{"current list", buildCRLSet(9871, spkis, nil), "MZ engine", StOK, T("browser.official"), crl},
		{"old list, fixed version", buildCRLSet(9000, nil, nil), "MZ engine " + string(spkis[0]), StOK, T("embed.sumBlocklist"), lib},
		{"old list, old version", buildCRLSet(9000, nil, nil), "MZ engine", StCaution, T("browser.crlsetOld"), crl},
		{"no list", nil, "MZ engine", StOK, T("browser.official"), exe},
	} {
		os.RemoveAll(filepath.Join(root, "Chrome", "User Data"))
		if c.crl != nil {
			write(crl, c.crl)
		}
		write(lib, []byte(c.engine))
		tp := testApps(t)
		tp.allDrives = true
		r := result{File: exe}
		tp.officialVerdict(&r, "Google Chrome", exe, "crlset")
		if r.Status != c.st || r.Summary != c.summary || r.File != c.file {
			t.Errorf("%s: %v %q %s\n%s", c.name, r.Status, r.Summary, r.File, r.Details)
		}
	}
}

// Microsoft Edge is judged the same way, by the block list it downloads from
// Microsoft, found in its User Data folder, or by the one built into its msedge.dll.
func TestOfficialVerdictEdge(t *testing.T) {
	Lang = 0
	t.Setenv("LOCALAPPDATA", `C:\Users\me\AppData\Local`)
	for _, c := range []struct{ exe, sub string }{
		{`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`, `Microsoft\Edge`},
		{`C:\Program Files (x86)\Microsoft\Edge Beta\Application\msedge.exe`, `Microsoft\Edge Beta`},
	} {
		want := filepath.Join(`C:\Users\me\AppData\Local`, c.sub, "User Data")
		if got := userDataDirs(c.exe); len(got) == 0 || got[0] != want {
			t.Errorf("%s: %q", c.exe, got)
		}
	}

	var spkis [][]byte
	for _, h := range uniqueBadSPKIs() {
		b, _ := hex.DecodeString(h)
		spkis = append(spkis, b)
	}
	root := t.TempDir()
	exe := filepath.Join(root, "Edge", "Application", "msedge.exe")
	lib := filepath.Join(filepath.Dir(exe), "156.0.4314.58", "msedge.dll")
	crl := filepath.Join(root, "Edge", "User Data", "CertificateRevocation", "6498.2026.10.20", "crl-set")
	for p, data := range map[string][]byte{exe: []byte("MZ"), lib: []byte("MZ engine"), crl: buildCRLSet(6498, spkis, nil)} {
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, data, 0o644)
	}
	if got := rootStoreLib(filepath.Dir(exe), false); got != lib {
		t.Errorf("engine: %q", got)
	}
	tp := testApps(t)
	tp.allDrives = true
	r := result{File: exe}
	tp.officialVerdict(&r, "Microsoft Edge", exe, "edge")
	if r.Status != StOK || r.File != crl || !strings.Contains(r.Details, "downloaded by Microsoft Edge (version 6498.2026.10.20,") || strings.Contains(r.Details, "Windows") {
		t.Errorf("current list: %v %s\n%s", r.Status, r.File, r.Details)
	}
	os.WriteFile(crl, buildCRLSet(6000, nil, nil), 0o644)
	r = result{File: exe}
	tp.officialVerdict(&r, "Microsoft Edge", exe, "edge")
	if r.Status != StCaution || !strings.Contains(r.Details, "Microsoft’s certificate block list") {
		t.Errorf("old list: %v\n%s", r.Status, r.Details)
	}
	os.WriteFile(lib, []byte("MZ engine "+string(spkis[2])), 0o644)
	r = result{File: exe}
	tp.officialVerdict(&r, "Microsoft Edge", exe, "edge")
	if r.Status != StOK || r.Summary != T("embed.sumBlocklist") || r.File != lib {
		t.Errorf("built-in list: %v %q %s", r.Status, r.Summary, r.File)
	}
}

// Microsoft Edge's block list has the format of Chrome's: the one Microsoft
// published on 20 October 2026 (component version 6498.2026.10.20) blocks every
// compromised intermediate.
func TestEdgeCRLSet(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "crlset-edge-6498.2026.10.20"))
	if err != nil {
		t.Fatal(err)
	}
	s, err := parseCRLSet(data)
	if err != nil {
		t.Fatal(err)
	}
	if b, n := s.blocks(); s.Sequence != 201 || n == 0 || b != n {
		t.Errorf("sequence %d: %d of %d blocked", s.Sequence, b, n)
	}
}

// A block list is named by the version that the browser's components page shows,
// not by the sequence number written in the file.
func TestCRLSetDescribe(t *testing.T) {
	Lang = 0
	day := time.Date(2026, 10, 20, 21, 8, 0, 0, time.UTC)
	for _, c := range []struct {
		dir  string
		seq  int
		want string
	}{
		{"9871", 9871, "version 9871, downloaded on 20 October 2026"},
		{"6498.2026.10.20", 201, "version 6498.2026.10.20, downloaded on 20 October 2026"},
	} {
		cs := crlSetInfo{path: filepath.Join("User Data", "CertificateRevocation", c.dir, "crl-set"), sequence: c.seq, downloaded: day}
		if got := cs.describe(); got != c.want {
			t.Errorf("%s: %q", c.dir, got)
		}
	}
}
