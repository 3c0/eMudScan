package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A Python that uv installed is known by its folder's name, beside uv's lock file.
func TestUvRuntimeOf(t *testing.T) {
	root := t.TempDir()
	for name, want := range map[string]bool{
		"cpython-3.12.9-windows-x86_64-none":               true,
		"cpython-3.13.17+freethreaded-windows-x86_64-none": true,
		"cpython-3.12-windows-x86_64-none":                 false, // the folder of the release line, which follows the upgrades
		"pypy-3.11.13-windows-x86_64-none":                 false,
	} {
		if _, ok := uvRuntimeOf(filepath.Join(root, name)); ok {
			t.Errorf("%s: without uv's lock file", name)
		}
		os.WriteFile(filepath.Join(root, ".lock"), nil, 0o644)
		rt, ok := uvRuntimeOf(filepath.Join(root, name))
		if ok != want || (ok && rt.dir != root) {
			t.Errorf("%s: %v %+v", name, ok, rt)
		}
		os.Remove(filepath.Join(root, ".lock"))
	}
}

// The folder of uv's Pythons is the one uv itself chooses.
func TestUvPythonDir(t *testing.T) {
	app := t.TempDir()
	t.Setenv("APPDATA", app)
	t.Setenv("UV_PYTHON_INSTALL_DIR", "")
	if got := uvPythonDir(); got != filepath.Join(app, "uv", "python") {
		t.Errorf("default: %q", got)
	}
	os.MkdirAll(filepath.Join(app, "uv", "data"), 0o755) // made by older versions of uv
	if got := uvPythonDir(); got != filepath.Join(app, "uv", "data", "python") {
		t.Errorf("older folder: %q", got)
	}
	t.Setenv("UV_PYTHON_INSTALL_DIR", `D:\uv\python\`)
	if got := uvPythonDir(); !strings.HasPrefix(got, `D:\uv\python`) {
		t.Errorf("named: %q", got)
	}
	t.Setenv("UV_PYTHON_INSTALL_DIR", `pythons`) // relative to where uv runs: unknown
	if got := uvPythonDir(); got != "" {
		t.Errorf("relative: %q", got)
	}
}

// An outdated Python of uv is not a copy inside a program: uv upgrades it, then the
// old copy is removed once a fixed version is installed; the virtual environments
// found that use that copy are named, also in the warning before the fix runs, but not
// those that use the folder of the release line. pip, which uv's Pythons refuse, gets
// no command. Without uv on the PATH, or for a folder that is not uv's current one,
// the command is only shown; a free-threaded Python gets the general advice. A removed
// copy is OK.
func TestUvPythonAdvice(t *testing.T) {
	Lang = 0
	root := t.TempDir()
	t.Setenv("APPDATA", filepath.Join(root, "AppData", "Roaming"))
	t.Setenv("UV_PYTHON_INSTALL_DIR", "")
	bin := filepath.Join(root, "bin")
	os.MkdirAll(bin, 0o755)
	os.WriteFile(filepath.Join(bin, "uv.exe"), []byte("MZ"), 0o644)
	uvDir := filepath.Join(root, "AppData", "Roaming", "uv", "python")
	other := filepath.Join(t.TempDir(), "uv", "python")
	for _, d := range []string{uvDir, other} {
		os.MkdirAll(d, 0o755)
		os.WriteFile(filepath.Join(d, ".lock"), nil, 0o644)
	}
	pinned := filepath.Join(root, "proj", ".venv")
	follows := filepath.Join(root, "proj2", ".venv")
	for v, home := range map[string]string{pinned: filepath.Join(uvDir, "cpython-3.12.9-windows-x86_64-none"), follows: filepath.Join(uvDir, "cpython-3.12-windows-x86_64-none")} {
		fakeVenv(t, v, home, "3.12.9")
	}
	badBundle := outdatedBundle(t)
	check := func(home, version string, path bool) Item {
		t.Helper()
		fakePython(t, home, false)
		os.WriteFile(filepath.Join(home, "Lib", "EXTERNALLY-MANAGED"), []byte("[externally-managed]\n"), 0o644)
		pem := filepath.Join(home, "Lib", "site-packages", "pip", "_vendor", "certifi", "cacert.pem")
		os.MkdirAll(filepath.Dir(pem), 0o755)
		os.WriteFile(pem, badBundle, 0o644)
		if path {
			t.Setenv("PATH", bin)
		} else {
			t.Setenv("PATH", "")
		}
		tp := testApps(t)
		tp.d = newDiscovery()
		for _, v := range []string{pinned, follows} {
			tp.d.pythonExes[filepath.Join(v, "Scripts", "python.exe")] = true
		}
		exe := filepath.Join(home, "python.exe")
		return *tp.c.begin(GroupApps, "python", "Python", home, func(it *Item) {
			env := pythonEnvFor(exe)
			env.version = version
			tp.pythonOneEnv(it, env)
		})
	}
	home := filepath.Join(uvDir, "cpython-3.12.9-windows-x86_64-none")
	got := check(home, "3.12.9", true)
	want := []string{
		"uv python upgrade 3.12",
		fmt.Sprintf(`if (uv python find --system --managed-python '>=3.12.16,<3.13' 2>$null) { uv python uninstall cpython-3.12.9-windows-x86_64-none } else { Write-Warning %s }`, psQuote(Tf("py.uvKept", "3.12"))),
	}
	if len(got.Fixes) != 1 || got.Fixes[0].Kind != fixPackage || got.Fixes[0].Manager != "uv" || got.Fixes[0].Optional ||
		strings.Join(got.Fixes[0].Cmds, "\n") != strings.Join(want, "\n") {
		t.Errorf("the fix: %+v", got.Fixes)
	}
	if got.Bundled || !strings.Contains(got.Details, T("py.uvFix")) || !strings.Contains(got.Details, T("py.uvVenvs")+"\n"+lk(pinned)) ||
		strings.Contains(got.Details, follows) || strings.Contains(got.Details, "pip install") || strings.Contains(got.Details, "update Python to") {
		t.Errorf("the details:\n%s", got.Details)
	}
	if w := fixWarnings([]Item{got, got}); w != Tf("py.uvWarn", "cpython-3.12.9-windows-x86_64-none", shownPath(pinned)) {
		t.Errorf("the warning: %q", w)
	}

	for name, c := range map[string]struct {
		it   Item
		line string
	}{
		"no uv on the PATH": {check(filepath.Join(uvDir, "cpython-3.12.8-windows-x86_64-none"), "3.12.8", false), cmd("uv python upgrade 3.12")},
		"another folder": {check(filepath.Join(other, "cpython-3.12.9-windows-x86_64-none"), "3.12.9", true),
			cmd("$env:UV_PYTHON_INSTALL_DIR = " + psPath(other, false) + "; uv python upgrade 3.12")},
	} {
		if len(c.it.Fixes) != 0 || !strings.Contains(c.it.Details, T("py.uvAdvice")+"\n"+c.line) {
			t.Errorf("%s: %+v\n%s", name, c.it.Fixes, c.it.Details)
		}
	}
	if it := check(filepath.Join(uvDir, "cpython-3.13.17+freethreaded-windows-x86_64-none"), "3.13.17", true); len(it.Fixes) != 0 || strings.Contains(it.Details, "uv python") {
		t.Errorf("free-threaded: %+v\n%s", it.Fixes, it.Details)
	}
	if it := check(filepath.Join(uvDir, "cpython-3.12.16-windows-x86_64-none"), "3.12.16", true); len(it.Fixes) != 0 || strings.Contains(it.Details, "uv python") {
		t.Errorf("fixed: %+v\n%s", it.Fixes, it.Details)
	}

	os.Remove(filepath.Join(home, "python.exe"))
	tp := testApps(t)
	if it := tp.c.begin(GroupApps, "python", "Python", home, func(it *Item) { tp.pythonOne(it, filepath.Join(home, "python.exe")) }); it.Status != StOK || it.Summary != T("py.sumGone") {
		t.Errorf("removed: %v %q", it.Status, it.Summary)
	}
}

// uv installed with Scoop keeps its Pythons in Scoop's persist folder, which
// UV_PYTHON_INSTALL_DIR names: uv upgrades them; Scoop's update of uv does not.
func TestUvPythonScoop(t *testing.T) {
	Lang = 0
	root := t.TempDir()
	dir := filepath.Join(root, "scoop", "persist", "uv", "python", "versions")
	t.Setenv("UV_PYTHON_INSTALL_DIR", dir)
	shims := filepath.Join(root, "scoop", "shims")
	os.MkdirAll(shims, 0o755)
	os.WriteFile(filepath.Join(shims, "uv.exe"), []byte("MZ"), 0o644)
	t.Setenv("PATH", shims)
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, ".lock"), nil, 0o644)
	home := filepath.Join(dir, "cpython-3.12.9-windows-x86_64-none")
	fakePython(t, home, false)
	tp := testApps(t)
	exe := filepath.Join(home, "python.exe")
	it := tp.c.begin(GroupApps, "python", "Python", home, func(it *Item) {
		env := pythonEnvFor(exe)
		env.version = "3.12.9"
		tp.pythonOneEnv(it, env)
	})
	tp.packageAdvice(it)
	f := fixesIn(it.Details)
	if len(f) != 1 || f[0].Manager != "uv" || strings.Contains(it.Details, "scoop") || len(it.Packages) != 1 || it.Packages[0].Manager != "Scoop" {
		t.Errorf("%+v %+v\n%s", f, it.Packages, it.Details)
	}
}
