package main

import (
	"bytes"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sslLibWith makes a fake _ssl.pyd: not a program, so it is read entirely; with
// the block list of CPython 3.11.18+ (the SHA-256 digests of the 8 compromised
// keys, as raw bytes) or without it.
func sslLibWith(blocklist bool) []byte {
	b := bytes.Repeat([]byte{0x90}, 3000)
	if blocklist {
		for _, h := range uniqueBadSPKIs() {
			d, _ := hex.DecodeString(h)
			b = append(b, d...)
		}
	}
	return append(b, bytes.Repeat([]byte{0x90}, 3000)...)
}

// fakePython lays out a Python installation: python.exe, DLLs\_ssl.pyd, Lib\site-packages.
func fakePython(t *testing.T, dir string, blocklist bool) {
	t.Helper()
	for _, d := range []string{"DLLs", filepath.Join("Lib", "site-packages")} {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "python.exe"), []byte("MZ"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "DLLs", "_ssl.pyd"), sslLibWith(blocklist), 0o644); err != nil {
		t.Fatal(err)
	}
}

// fakeVenv lays out a virtual environment of base: Scripts\python.exe and pyvenv.cfg.
func fakeVenv(t *testing.T, dir, base, version string) {
	t.Helper()
	for _, d := range []string{"Scripts", filepath.Join("Lib", "site-packages")} {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "Scripts", "python.exe"), []byte("MZ"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := "home = " + base + "\ninclude-system-site-packages = false\nversion = " + version + "\n"
	if err := os.WriteFile(filepath.Join(dir, "pyvenv.cfg"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
}

// outdatedBundle is a PEM bundle that still trusts the affected roots: the roots of
// the reference data, padded with the test chains' certificates to look like a
// trust store (at least minBundleCerts certificates).
func outdatedBundle(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	for _, r := range rootCerts {
		b.WriteString(r.PEM)
		b.WriteString("\n")
	}
	for _, tc := range testChains {
		for _, c := range append([]*x509.Certificate{tc.Leaf}, tc.Intermediates...) {
			pem.Encode(&b, &pem.Block{Type: "CERTIFICATE", Bytes: c.Raw})
		}
	}
	br, ok := newBundleCache().checkData(b.Bytes())
	if !ok || !br.Bad() {
		t.Fatalf("the test bundle is not an outdated bundle: ok=%v %+v", ok, br)
	}
	return b.Bytes()
}

func testApps(t *testing.T) *appsContext {
	t.Helper()
	return &appsContext{c: newChecker(nopReporter{}, nil), bc: newBundleCache(), es: buildEmbedScanner(),
		handled: map[string]bool{}, scoopApps: map[string]scoopManifest{}}
}

func TestPyvenvHome(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "pyvenv.cfg")
	os.WriteFile(cfg, []byte("home = C:\\Users\\me\\AppData\\Local\\Programs\\Python\\Python313\r\nversion = 3.13.17\r\n"), 0o644)
	if got := pyvenvHome(cfg); got != `C:\Users\me\AppData\Local\Programs\Python\Python313` {
		t.Errorf("home = %q", got)
	}
	if got := pyvenvVersion(cfg); got != "3.13.17" {
		t.Errorf("version = %q", got)
	}
}

// The fix is recognised in the ssl module's library, for a Python and for the
// virtual environments that run it, whatever version pyvenv.cfg recorded.
func TestPythonFixFromSSLModule(t *testing.T) {
	tp := testApps(t)
	root := t.TempDir()
	fixed, old := filepath.Join(root, "Python313"), filepath.Join(root, "Python312")
	fakePython(t, fixed, true)
	fakePython(t, old, false)
	fakeVenv(t, filepath.Join(root, "venv-fixed"), fixed, "3.13.17")
	fakeVenv(t, filepath.Join(root, "venv-old"), old, "3.12.9")
	cases := []struct {
		exe       string
		protected bool
	}{
		{filepath.Join(fixed, "python.exe"), true},
		{filepath.Join(old, "python.exe"), false},
		{filepath.Join(root, "venv-fixed", "Scripts", "python.exe"), true},
		{filepath.Join(root, "venv-old", "Scripts", "python.exe"), false},
	}
	for _, c := range cases {
		f := tp.pythonFix(pythonEnvFor(c.exe))
		if f.protected() != c.protected || f.inSSL != c.protected {
			t.Errorf("%s: protected %v, inSSL %v, want %v (%+v)", c.exe, f.protected(), f.inSSL, c.protected, f)
		}
		if f.sslLib == "" {
			t.Errorf("%s: _ssl.pyd not found", c.exe)
		}
	}
	// the version alone decides when the library is not found
	for v, want := range map[string]bool{"3.13.18": true, "3.13.17": false, "3.11.18": true, "3.10.19": false, "": false} {
		f := tp.pythonFix(pyEnv{home: filepath.Join(root, "nowhere"), version: v})
		if f.protected() != want {
			t.Errorf("version %q: protected %v, want %v", v, f.protected(), want)
		}
	}
}

// What the Python item says: green when the ssl module blocks, amber when a fix
// exists, amber too when the line is no longer maintained, red when the lists are
// outdated and no fix is in place.
func TestPythonVerdicts(t *testing.T) {
	Lang = 0
	root := t.TempDir()
	badBundle := outdatedBundle(t)
	type tc struct {
		name      string
		blocklist bool
		version   string // "" on this system: file versions cannot be read, so the version is given
		oldLists  bool
		status    Status
		summary   string
	}
	cases := []tc{
		{"fixed", true, "3.13.18", false, StOK, "Refuses the compromised certificates itself"},
		{"fixed, old lists", true, "3.13.18", true, StOK, "Refuses the compromised certificates itself"},
		{"old", false, "3.13.17", false, StCaution, "Update Python to 3.13.18"},
		{"old, old lists", false, "3.13.17", true, StWarn, "Outdated certificate list (pip/certifi)"},
		{"unsupported", false, "3.10.19", false, StCaution, "Python 3.10.19 is no longer maintained: no fix"},
		{"blocklist, version unknown", true, "", false, StOK, "Refuses the compromised certificates itself"},
		{"nothing known", false, "", false, StOK, "pip and certifi up to date"},
	}
	for i, c := range cases {
		tp := testApps(t)
		tp.pyStoreRisk = 12
		home := filepath.Join(root, "py"+itoa(i))
		fakePython(t, home, c.blocklist)
		if c.oldLists {
			sp := filepath.Join(home, "Lib", "site-packages")
			os.MkdirAll(filepath.Join(sp, "certifi"), 0o755)
			os.WriteFile(filepath.Join(sp, "certifi", "cacert.pem"), badBundle, 0o644)
		}
		exe := filepath.Join(home, "python.exe")
		got := tp.c.begin(GroupApps, "python", "Python", home, func(it *Item) {
			env := pythonEnvFor(exe)
			env.version = c.version // file versions cannot be read on this system
			tp.pythonOneEnv(it, env)
		})
		if got.Status != c.status || got.Summary != c.summary {
			t.Errorf("%s: status %v, summary %q; want %v, %q\n%s", c.name, got.Status, got.Summary, c.status, c.summary, got.Details)
		}
		switch {
		case c.name == "fixed, old lists" && !strings.Contains(got.Details, "remains good practice"):
			t.Errorf("%s: the lists' update should stay as good practice:\n%s", c.name, got.Details)
		case c.name == "old, old lists" && !strings.Contains(got.Details, "update Python to 3.13.18"):
			t.Errorf("%s: the Python update should come first:\n%s", c.name, got.Details)
		case c.name == "old" && !strings.Contains(got.Details, "12 of 12 test chains"):
			t.Errorf("%s: the Windows copy risk should be explained:\n%s", c.name, got.Details)
		}
	}
}

// The outdated certifi of a virtual environment is harmless when the Python it
// runs refuses the compromised certificates itself.
func TestVenvBundleWithFixedPython(t *testing.T) {
	Lang = 0
	badBundle := outdatedBundle(t)
	root := t.TempDir()
	for _, c := range []struct {
		name      string
		blocklist bool
		status    Status
	}{{"fixed", true, StInfo}, {"old", false, StWarn}} {
		base := filepath.Join(root, c.name, "Python313")
		venv := filepath.Join(root, c.name, "venv")
		fakePython(t, base, c.blocklist)
		fakeVenv(t, venv, base, "3.13.17")
		sp := filepath.Join(venv, "Lib", "site-packages")
		os.MkdirAll(filepath.Join(sp, "certifi"), 0o755)
		p := filepath.Join(sp, "certifi", "cacert.pem")
		os.WriteFile(p, badBundle, 0o644)
		tp := testApps(t)
		it := tp.c.begin(GroupApps, "bundle", "certifi", filepath.Dir(p), func(it *Item) { tp.bundleOne(it, p) })
		if it.Status != c.status {
			t.Errorf("%s: status %v, want %v\n%s", c.name, it.Status, c.status, it.Details)
		}
		if c.blocklist && !strings.Contains(it.Details, "pip install --upgrade certifi") {
			t.Errorf("%s: the certifi update should stay as good practice:\n%s", c.name, it.Details)
		}
	}
}
