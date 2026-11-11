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

// fakePython lays out a Python installation: python.exe, DLLs\_ssl.pyd, Lib\site-packages\pip.
func fakePython(t *testing.T, dir string, blocklist bool) {
	t.Helper()
	for _, d := range []string{"DLLs", filepath.Join("Lib", "site-packages", "pip")} {
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

// fakeVenv lays out a virtual environment of base: Scripts\python.exe, pyvenv.cfg and pip.
func fakeVenv(t *testing.T, dir, base, version string) {
	t.Helper()
	for _, d := range []string{"Scripts", filepath.Join("Lib", "site-packages", "pip")} {
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
		{"fixed", true, "3.13.18", false, StOK, "Rejects the compromised certificates itself"},
		{"fixed, old lists", true, "3.13.18", true, StOK, "Rejects the compromised certificates itself"},
		{"old", false, "3.13.17", false, StCaution, "Update Python to 3.13.18"},
		{"old, old lists", false, "3.13.17", true, StWarn, "Outdated certificate bundle (pip/certifi)"},
		{"unsupported", false, "3.10.19", false, StCaution, "Python 3.10.19 is no longer maintained: no fix"},
		{"blocklist, version unknown", true, "", false, StOK, "Rejects the compromised certificates itself"},
		// version unknown, neither pip nor certifi: the Windows roots it loads accept test chains
		{"nothing known", false, "", false, StCaution, "Unknown version"},
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
		case c.name == "fixed, old lists" && !strings.Contains(got.Details, "is still good practice"):
			t.Errorf("%s: the lists' update should stay as good practice:\n%s", c.name, got.Details)
		case c.name == "old, old lists" && !strings.Contains(got.Details, "update Python to 3.13.18"):
			t.Errorf("%s: the Python update should come first:\n%s", c.name, got.Details)
		case c.name == "old" && !strings.Contains(got.Details, "12 of the 12 test chains"):
			t.Errorf("%s: the Windows copy risk should be explained:\n%s", c.name, got.Details)
		case c.name == "nothing known" && (!strings.Contains(got.Details, T("py.unknown")) || !strings.Contains(got.Details, "12 of the 12 test chains") || !strings.Contains(got.Details, Tf("py.unknownFix", fixedVersionsList("python")))):
			t.Errorf("%s: what is known and what to do:\n%s", c.name, got.Details)
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

// pip refuses to change a Python that the tool which installed it manages (uv marks
// its Pythons so): no pip command for it, but for its virtual environments.
func TestExternallyManagedPython(t *testing.T) {
	Lang = 0
	badBundle := outdatedBundle(t)
	root := t.TempDir()
	for _, c := range []struct {
		name, version string
		blocklist     bool
		says          string // what the details say instead of the pip command
	}{
		{"outdated", "3.13.17", false, "update Python to 3.13.18"},
		{"version unknown", "", false, T("py.managed")},
		{"fixed", "3.13.18", true, T("py.managed")},
	} {
		home := filepath.Join(root, c.name)
		fakePython(t, home, c.blocklist)
		sp := filepath.Join(home, "Lib", "site-packages")
		os.MkdirAll(filepath.Join(sp, "certifi"), 0o755)
		os.WriteFile(filepath.Join(sp, "certifi", "cacert.pem"), badBundle, 0o644)
		os.WriteFile(filepath.Join(home, "Lib", "EXTERNALLY-MANAGED"), []byte("[externally-managed]\nError=managed by uv\n"), 0o644)
		tp := testApps(t)
		exe := filepath.Join(home, "python.exe")
		got := tp.c.begin(GroupApps, "python", "Python", home, func(it *Item) {
			env := pythonEnvFor(exe)
			env.version = c.version
			tp.pythonOneEnv(it, env)
		})
		if strings.Contains(got.Details, "pip install") || !strings.Contains(got.Details, c.says) {
			t.Errorf("%s: no pip command, and %q:\n%s", c.name, c.says, got.Details)
		}
		if c.name == "outdated" && strings.Contains(got.Details, T("py.managed")) {
			t.Errorf("%s: the Python update is enough:\n%s", c.name, got.Details)
		}
		// its virtual environment: pip works there
		venv := filepath.Join(root, c.name+"-venv")
		fakeVenv(t, venv, home, "3.13.17")
		vp := filepath.Join(venv, "Lib", "site-packages", "certifi", "cacert.pem")
		os.MkdirAll(filepath.Dir(vp), 0o755)
		os.WriteFile(vp, badBundle, 0o644)
		v := tp.c.begin(GroupApps, "bundle", "certifi", filepath.Dir(vp), func(it *Item) { tp.bundleOne(it, vp) })
		if !strings.Contains(v.Details, "pip install --upgrade certifi") {
			t.Errorf("%s: its virtual environment keeps its pip command:\n%s", c.name, v.Details)
		}
	}
	// the bundle of the managed Python itself, when it is checked as a bundle
	p := filepath.Join(root, "outdated", "Lib", "site-packages", "certifi", "cacert.pem")
	tp := testApps(t)
	b := tp.c.begin(GroupApps, "bundle", "certifi", filepath.Dir(p), func(it *Item) { tp.bundleOne(it, p) })
	if strings.Contains(b.Details, "pip install") {
		t.Errorf("bundle of a managed Python: no pip command:\n%s", b.Details)
	}
}

// Where pip is not installed (a uv environment, some embedded Pythons), "python -m
// pip" fails: no pip command, but for an environment that sees the packages of its
// base Python, which has pip.
func TestNoPip(t *testing.T) {
	Lang = 0
	badBundle := outdatedBundle(t)
	root := t.TempDir()
	home := filepath.Join(root, "Python")
	fakePython(t, home, false)
	os.RemoveAll(filepath.Join(home, "Lib", "site-packages", "pip"))
	pem := filepath.Join(home, "Lib", "site-packages", "certifi", "cacert.pem")
	os.MkdirAll(filepath.Dir(pem), 0o755)
	os.WriteFile(pem, badBundle, 0o644)
	tp := testApps(t)
	exe := filepath.Join(home, "python.exe")
	got := tp.c.begin(GroupApps, "python", "Python", home, func(it *Item) {
		env := pythonEnvFor(exe)
		env.version = ""
		tp.pythonOneEnv(it, env)
	})
	if strings.Contains(got.Details, "pip install") || !strings.Contains(got.Details, Tf("py.noPip", "certifi")) {
		t.Errorf("Python without pip:\n%s", got.Details)
	}
	for _, c := range []struct {
		name   string
		system bool // sees the packages of its base Python
		want   string
	}{
		{"own packages only", false, Tf("py.noPipEnv", "certifi")},
		{"sees its base's", true, "pip install --upgrade certifi"},
	} {
		base := filepath.Join(root, "Base")
		fakePython(t, base, false)
		venv := filepath.Join(root, c.name)
		fakeVenv(t, venv, base, "3.13.17")
		os.RemoveAll(filepath.Join(venv, "Lib", "site-packages", "pip"))
		if c.system {
			cfg := filepath.Join(venv, "pyvenv.cfg")
			data, _ := os.ReadFile(cfg)
			os.WriteFile(cfg, []byte(strings.Replace(string(data), "include-system-site-packages = false", "include-system-site-packages = true", 1)), 0o644)
		}
		vp := filepath.Join(venv, "Lib", "site-packages", "certifi", "cacert.pem")
		os.MkdirAll(filepath.Dir(vp), 0o755)
		os.WriteFile(vp, badBundle, 0o644)
		v := tp.c.begin(GroupApps, "bundle", "certifi", filepath.Dir(vp), func(it *Item) { tp.bundleOne(it, vp) })
		if !strings.Contains(v.Details, c.want) || (!c.system && strings.Contains(v.Details, "pip install")) {
			t.Errorf("%s: want %q:\n%s", c.name, c.want, v.Details)
		}
	}
}

// A Python inside another program: that program's update is given once, in the
// recommended action, then the command that updates the outdated bundles until then,
// after the warning about modifying a copy inside a program. One sentence says what
// is wrong.
func TestBundledPythonAdvice(t *testing.T) {
	Lang = 0
	badBundle := outdatedBundle(t)
	root := t.TempDir()
	program := "Recommended action: update Example App 1, which this copy belongs to,"
	until := Tf("py.untilThen", "certifi") + "\n"
	warned := T("bundled.warn") + "\nIn PowerShell:\n" // inside a program
	for i, c := range []struct {
		name, version string
		blocklist     bool
		owned         bool
		want          []string // in this order
	}{
		{"bundled, old", "3.13.17", false, true, []string{"Python 3.13.17 does not reject the compromised certificates yet (Python 3.13.18 does), and its certifi still trusts ", program, until, warned, "pip install --upgrade certifi"}},
		{"bundled, version unknown", "", false, true, []string{"certifi, in this Python, still trusts ", program, until, warned, "pip install --upgrade certifi"}},
		{"bundled, fixed", "3.13.18", true, true, []string{"rejects the compromised certificates itself", Tf("py.listsOldSafe.1", "certifi"), program, until, warned, "pip install --upgrade certifi"}},
		{"own, old", "3.13.17", false, false, []string{"Recommended action: update Python to 3.13.18.", until + "In PowerShell:\n", "pip install --upgrade certifi"}},
		{"own, fixed", "3.13.18", true, false, []string{Tf("py.listsOldSafe.1", "certifi") + "\nIn PowerShell:\n", "pip install --upgrade certifi"}},
	} {
		app := filepath.Join(root, "app"+itoa(i))
		home := filepath.Join(app, "python")
		if !c.owned {
			home = filepath.Join(root, "Python313-"+itoa(i))
		}
		fakePython(t, home, c.blocklist)
		sp := filepath.Join(home, "Lib", "site-packages")
		os.MkdirAll(filepath.Join(sp, "certifi"), 0o755)
		os.WriteFile(filepath.Join(sp, "certifi", "cacert.pem"), badBundle, 0o644)
		tp := testApps(t)
		tp.owners = []ownerDir{{dir: app, name: "Example App 1", version: "1.0.4"}}
		exe := filepath.Join(home, "python.exe")
		got := tp.c.begin(GroupApps, "python", "Python", home, func(it *Item) {
			env := pythonEnvFor(exe)
			env.version = c.version
			tp.pythonOneEnv(it, env)
		})
		d, at := got.Details, 0
		for _, w := range c.want {
			i := strings.Index(d[at:], w)
			if i < 0 {
				t.Errorf("%s: %q missing or out of order:\n%s", c.name, w, d)
				break
			}
			at += i + len(w)
		}
		if n := strings.Count(d, "which this copy belongs to"); n != map[bool]int{true: 1, false: 0}[c.owned] || strings.Contains(d, T("bundled.warn")) != c.owned {
			t.Errorf("%s: the program's advice %d times:\n%s", c.name, n, d)
		}
		for _, bad := range []string{"If you manage", ":\nIn PowerShell"} {
			if strings.Contains(d, bad) {
				t.Errorf("%s: %q:\n%s", c.name, bad, d)
			}
		}
	}
}

// A Python whose version cannot be read, with neither pip nor certifi, is not
// "up to date": it is for information when the Windows roots it loads accept none
// of the test chains.
func TestPythonUnknownWithoutLists(t *testing.T) {
	Lang = 0
	tp := testApps(t)
	tp.pyStoreRisk = 0
	home := filepath.Join(t.TempDir(), "python-embed")
	fakePython(t, home, false)
	got := tp.c.begin(GroupApps, "python", "Python", home, func(it *Item) {
		env := pythonEnvFor(filepath.Join(home, "python.exe"))
		env.version = ""
		tp.pythonOneEnv(it, env)
	})
	if got.Status != StInfo || got.Summary != T("version.unknown") || !strings.Contains(got.Details, T("py.unknownNoRisk")) || strings.Contains(got.Details, "up to date") {
		t.Errorf("%v %q:\n%s", got.Status, got.Summary, got.Details)
	}
}

// A program can keep its Python in a folder of its own, behind a python.exe of the
// program: the ssl module's library is found there, the block list in it counts, and
// eMudScan offers no replacement of the ssl files, which are made for the layouts of
// python.org and python-build-standalone.
func TestPythonBehindLauncher(t *testing.T) {
	tp := testApps(t)
	for _, blocklist := range []bool{false, true} {
		home := filepath.Join(t.TempDir(), "Example App 5", "program")
		core := filepath.Join(home, "runtime-3.13.5", "lib")
		if err := os.MkdirAll(core, 0o755); err != nil {
			t.Fatal(err)
		}
		for name, data := range map[string][]byte{
			filepath.Join(home, "python.exe"):      []byte("MZ"),
			filepath.Join(home, "python3.dll"):     []byte("MZ"),
			filepath.Join(core, "_ssl.pyd"):        sslLibWith(blocklist),
			filepath.Join(core, "_hashlib.pyd"):    []byte("MZ"),
			filepath.Join(core, "libssl-3.dll"):    []byte("MZ"),
			filepath.Join(core, "libcrypto-3.dll"): []byte("MZ"),
		} {
			if err := os.WriteFile(name, data, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		env := pyEnv{exe: filepath.Join(home, "python.exe"), home: home, version: "3.13.5"}
		fx := tp.pythonFix(env)
		if fx.sslLib != filepath.Join(core, "_ssl.pyd") {
			t.Fatalf("ssl library: got %q, want the one of the folder below", fx.sslLib)
		}
		if fx.state != FixOutdated || fx.inSSL != blocklist || fx.protected() != blocklist {
			t.Errorf("block list %v: state %v, inSSL %v, protected %v", blocklist, fx.state, fx.inSSL, fx.protected())
		}
		if _, ok := pySSLPlanFor(env, fx.sslLib, fx.fixed); ok {
			t.Error("a replacement of the ssl files is offered for a Python laid out otherwise")
		}
	}
}
