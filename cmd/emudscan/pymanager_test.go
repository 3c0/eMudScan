package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A Python of the Python Install Manager is known by its __install__.json, whose
// company and tag name it in the update command; a tag that a command could not take
// as it is, or another schema, is not used.
func TestPymanagerRuntimeOf(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []struct {
		json, arg string
		ok        bool
	}{
		{`{"schema": 1, "id": "pythoncore-3.14-64", "company": "PythonCore", "tag": "3.14-64", "sort-version": "3.14.7"}`, "3.14-64", true},
		{`{"schema": 1, "id": "astral-3.12", "company": "Astral", "tag": "CPython3.12", "sort-version": "3.12.9"}`, `Astral\CPython3.12`, true},
		{`{"schema": 1, "company": "PythonCore", "tag": "3.14-64; Remove-Item x"}`, "", false},
		{`{"schema": 2, "company": "PythonCore", "tag": "3.14-64"}`, "", false},
		{`not json`, "", false},
	} {
		os.WriteFile(filepath.Join(dir, "__install__.json"), []byte(c.json), 0o644)
		rt, ok := pymanagerRuntimeOf(dir)
		if ok != c.ok || (ok && rt.arg() != c.arg) {
			t.Errorf("%s: %v %q", c.json, ok, rt.arg())
		}
	}
	if _, ok := pymanagerRuntimeOf(t.TempDir()); ok {
		t.Error("a Python without __install__.json")
	}
}

// An outdated Python of the Python Install Manager, on a line that it has a fixed
// build of (3.14 and later), is updated with it, when it is installed: a fix that the
// Fix function runs as a package update. On an older line, it installs a newer
// Python. Without the manager, or for a Python installed otherwise, nothing changes.
func TestPymanagerAdvice(t *testing.T) {
	Lang = 0
	root := t.TempDir()
	t.Setenv("ProgramFiles", "")
	manager := filepath.Join(root, "Local", "Microsoft", "WindowsApps", "pymanager.exe")
	for i, c := range []struct {
		version   string
		managed   bool
		installed bool
		fix       bool   // the pymanager update, as a fix
		want      string // in the details, or "" for none of the manager's texts
	}{
		{"3.14.7", true, true, true, T("py.pymanagerFix") + "\n" + cmd("pymanager install --update -y 3.14-64")},
		{"3.15.1", true, true, true, cmd("pymanager install --update -y 3.15-64")},
		{"3.13.17", true, true, false, Tf("py.pymanagerNewer", "3.13", "3.14", "3.14") + "\n" + cmd("pymanager install 3.14")},
		{"3.10.19", true, true, false, Tf("py.pymanagerNewer", "3.10", "3.14", "3.14")},
		{"3.14.7", true, false, false, ""},
		{"3.14.7", false, true, false, ""},
		{"3.14.10", true, true, false, ""},
	} {
		home := filepath.Join(root, "py"+itoa(i))
		fakePython(t, home, false)
		if c.managed {
			line := strings.Join(strings.Split(c.version, ".")[:2], ".")
			os.WriteFile(filepath.Join(home, "__install__.json"),
				[]byte(`{"schema": 1, "company": "PythonCore", "tag": "`+line+`-64", "sort-version": "`+c.version+`", "executable": "python.exe"}`), 0o644)
		}
		os.RemoveAll(filepath.Dir(manager))
		if c.installed {
			os.MkdirAll(filepath.Dir(manager), 0o755)
			os.WriteFile(manager, nil, 0o644)
		}
		t.Setenv("LOCALAPPDATA", filepath.Join(root, "Local"))
		tp := testApps(t)
		exe := filepath.Join(home, "python.exe")
		got := tp.c.begin(GroupApps, "python", "Python", home, func(it *Item) {
			env := pythonEnvFor(exe)
			env.version = c.version
			tp.pythonOneEnv(it, env)
		})
		hasFix := false
		for _, f := range got.Fixes {
			hasFix = hasFix || (f.Kind == fixPackage && f.Manager == "Python Install Manager" && !f.Admin && strings.Join(f.Servers, " ") == "www.python.org")
		}
		mentions := strings.Contains(got.Details, "Python Install Manager")
		if hasFix != c.fix || (c.want == "" && mentions) || (c.want != "" && !strings.Contains(got.Details, c.want)) {
			t.Errorf("%s managed %v installed %v: fix %v\n%s", c.version, c.managed, c.installed, hasFix, got.Details)
		}
	}
}
