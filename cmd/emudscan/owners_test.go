package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	sidUsers = "S-1-5-32-545"
	sidMe    = "S-1-5-21-1-2-3-1001"
	sidOther = "S-1-5-21-1-2-3-1002"
)

// fakeSecurity makes files belong to administrators, except those in owners, and
// lets the accounts in writers change those listed (keys made with pathKey).
func fakeSecurity(t *testing.T, owners map[string]string, writers map[string][]string) {
	t.Helper()
	fs, an, cu := fileSecurity, accountName, currentUserSID
	t.Cleanup(func() { fileSecurity, accountName, currentUserSID = fs, an, cu })
	fileSecurity = func(p string) (fileSec, error) {
		s := fileSec{owner: sidAdministrators, writers: writers[pathKey(p)]}
		if o, ok := owners[pathKey(p)]; ok {
			s.owner = o
		}
		return s, nil
	}
	accountName = func(sid string) string {
		return map[string]string{sidUsers: `BUILTIN\Users`, sidOther: `PC\bob`}[sid]
	}
	currentUserSID = func() string { return sidMe }
}

// fakeProgramDataPython lays out a Python with an outdated certifi in a folder of
// ProgramData, where its commands need administrator rights.
func fakeProgramDataPython(t *testing.T) (pd, home string) {
	t.Helper()
	pd = t.TempDir()
	t.Setenv("ProgramData", pd)
	home = filepath.Join(pd, "Python")
	fakePython(t, home, false)
	certifi := filepath.Join(home, "Lib", "site-packages", "certifi", "cacert.pem")
	os.MkdirAll(filepath.Dir(certifi), 0o755)
	os.WriteFile(certifi, outdatedBundle(t), 0o644)
	return pd, home
}

// The pip commands of a Python that needs administrator rights are given only when
// its files and folders belong to administrators, SYSTEM, TrustedInstaller or the
// user, and no one else may change the folders it loads code from.
func TestPipCommandsAndOwners(t *testing.T) {
	Lang = 0
	_, home := fakeProgramDataPython(t)
	exe := filepath.Join(home, "python.exe")
	sp := filepath.Join(home, "Lib", "site-packages")
	k := pathKey
	for _, c := range []struct {
		name    string
		owners  map[string]string
		writers map[string][]string
		want    string // "" for the commands
	}{
		{"administrators", nil, nil, ""},
		{"the user", map[string]string{k(home): sidMe, k(exe): sidMe}, map[string][]string{k(home): {sidMe}}, ""},
		{"creator owner only", nil, map[string][]string{k(home): {sidCreatorOwner, sidSystem}}, ""},
		{"python.exe of another user", map[string]string{k(exe): sidOther}, nil, Tf("pip.ownedBy", lk(exe), `PC\bob`)},
		{"folder of another user", map[string]string{k(home): sidOther}, nil, Tf("pip.ownedBy", lk(home), `PC\bob`)},
		{"users may add files", nil, map[string][]string{k(sp): {sidUsers}}, Tf("pip.changeableBy", lk(sp), `BUILTIN\Users`)},
		{"users may add DLLs", nil, map[string][]string{k(filepath.Join(home, "DLLs")): {sidUsers}}, Tf("pip.changeableBy", lk(filepath.Join(home, "DLLs")), `BUILTIN\Users`)},
		// permissions inherited from ProgramData: the Python's folder is named, which fixes all
		{"users may add files everywhere", nil, map[string][]string{k(home): {sidUsers}, k(filepath.Join(home, "Lib")): {sidUsers}, k(filepath.Join(home, "DLLs")): {sidUsers}, k(sp): {sidUsers}}, Tf("pip.changeableBy", lk(home), `BUILTIN\Users`)},
		{"another user's folder and file", map[string]string{k(home): sidOther, k(exe): sidOther}, nil, Tf("pip.ownedBy", lk(home), `PC\bob`)},
	} {
		fakeSecurity(t, c.owners, c.writers)
		tp := testApps(t)
		tp.d = newDiscovery()
		tp.d.pythonExes[exe] = true
		tp.checkPython()
		items := tp.c.snapshot()
		if len(items) != 1 {
			t.Fatalf("%s: %d elements", c.name, len(items))
		}
		d := items[0].Details
		hasCmd := strings.Contains(d, pipCmd(exe, "certifi"))
		if c.want == "" && (!hasCmd || !strings.Contains(d, T("fix.psAdmin"))) || c.want != "" && (hasCmd || !strings.Contains(d, c.want)) {
			t.Errorf("%s:\n%s", c.name, d)
		}
		// the Fix function runs exactly the commands shown, as administrator
		if f := items[0].Fixes; c.want == "" && (len(f) != 1 || f[0].Kind != fixPip || !f[0].Admin || f[0].key() != pipLine(exe, "certifi", true)) || c.want != "" && len(f) != 0 {
			t.Errorf("%s: fixes %+v", c.name, f)
		}
	}
}

// Outside the folders that need administrator rights, the commands run as the user:
// who else may change the folder does not matter to them.
func TestPipCommandsWithoutAdmin(t *testing.T) {
	t.Setenv("ProgramData", t.TempDir())
	t.Setenv("ProgramFiles", "")
	home := filepath.Join(t.TempDir(), "Python")
	fakePython(t, home, false)
	fakeSecurity(t, map[string]string{pathKey(home): sidOther}, map[string][]string{pathKey(home): {sidUsers}})
	if why := pipUnsafe(pythonEnvFor(filepath.Join(home, "python.exe"))); why != "" {
		t.Errorf("%q", why)
	}
}

// A copy of certifi in such a Python: the same.
func TestPipBundleCommandAndOwners(t *testing.T) {
	Lang = 0
	_, home := fakeProgramDataPython(t)
	exe := filepath.Join(home, "python.exe")
	certifi := filepath.Join(home, "Lib", "site-packages", "certifi", "cacert.pem")
	fakeSecurity(t, map[string]string{pathKey(exe): sidOther}, nil)
	if got := pipFix(certifi, exe, "certifi", false, ownerInfo{}); got != Tf("pip.ownedBy", lk(exe), `PC\bob`) {
		t.Errorf("another user's python.exe: %q", got)
	}
	o := ownerInfo{Name: "App", URL: "https://example.org"}
	if got := pipFix(certifi, exe, "certifi", true, o); !strings.HasPrefix(got, bundledAdvice(o)) || strings.Contains(got, "$ ") {
		t.Errorf("inside another program: %q", got)
	}
	fakeSecurity(t, nil, nil)
	if got := pipFix(certifi, exe, "certifi", false, ownerInfo{}); got != T("fix.psAdmin")+"\n"+pipCmd(exe, "certifi") {
		t.Errorf("administrators: %q", got)
	}
}

// A backup that another user may have put next to a file in ProgramData is not
// offered for restoring; one that belongs to administrators is.
func TestRestoreAndOwners(t *testing.T) {
	Lang = 0
	pd := t.TempDir()
	t.Setenv("ProgramData", pd)
	f := filepath.Join(pd, "App", "cacert.pem")
	os.MkdirAll(filepath.Dir(f), 0o755)
	os.WriteFile(f, []byte("current"), 0o644)
	b := f + ".emudscan-20261031-011500.bak"
	os.WriteFile(b, []byte("before"), 0o644)
	move := "Move-Item -LiteralPath " + psQuote(b) + " -Destination " + psQuote(f) + " -Force"

	fakeSecurity(t, map[string]string{pathKey(b): sidOther}, nil)
	got := restoreAdvice(f)
	if strings.Contains(got, move) || got != Tf("restore.ownedBy", lk(f), lk(b), `PC\bob`) {
		t.Errorf("another user's backup:\n%s", got)
	}
	fakeSecurity(t, map[string]string{pathKey(filepath.Dir(f)): sidOther}, nil)
	if got := restoreAdvice(f); strings.Contains(got, move) || !strings.Contains(got, lk(filepath.Dir(f))) {
		t.Errorf("another user's folder:\n%s", got)
	}
	fakeSecurity(t, nil, map[string][]string{pathKey(filepath.Dir(f)): {sidUsers}}) // may add files: the backup is still the administrators'
	if got := restoreAdvice(f); !strings.Contains(got, move) || !strings.Contains(got, T("fix.psAdmin")) {
		t.Errorf("administrators' backup:\n%s", got)
	}
}

// Files that cannot be read count as another user's.
func TestOwnerUnreadable(t *testing.T) {
	Lang = 0
	fakeSecurity(t, nil, nil)
	dir := t.TempDir()
	fileSecurity = func(p string) (fileSec, error) {
		if p == dir {
			return fileSec{}, os.ErrPermission
		}
		return fileSec{owner: sidAdministrators}, nil
	}
	if p, who, ok := foreignOwner(dir); !ok || p != dir || who != T("owner.unknown") {
		t.Errorf("owner: %q %q %v", p, who, ok)
	}
	if p, who, ok := othersCanChange(dir); !ok || p != dir || who != T("owner.unknown") {
		t.Errorf("writers: %q %q %v", p, who, ok)
	}
	if _, _, ok := foreignOwner(filepath.Join(dir, "missing")); ok {
		t.Errorf("a missing file is checked")
	}
}
