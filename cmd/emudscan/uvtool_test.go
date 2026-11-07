package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A tool that uv installed: uv updates the packages of its environment, which has no
// pip, and which uv says never to change with pip. eMudScan runs the update when the
// tool is in the tool folder of the uv of this user (UV_TOOL_DIR, as Scoop sets it)
// and uv is on the PATH; else it only shows the command.
func TestUvTool(t *testing.T) {
	Lang = 0
	root := t.TempDir()
	tools := filepath.Join(root, "scoop", "persist", "uv", "tools", "versions")
	t.Setenv("UV_TOOL_DIR", tools)
	shims := filepath.Join(root, "scoop", "shims")
	os.MkdirAll(shims, 0o755)
	os.WriteFile(filepath.Join(shims, "uv.exe"), []byte("MZ"), 0o644)
	badBundle := outdatedBundle(t)
	base := filepath.Join(root, "scoop", "persist", "uv", "python", "versions", "cpython-3.12-windows-x86_64-none")
	fakePython(t, base, true)
	check := func(env string, path bool) Item {
		t.Helper()
		fakeVenv(t, env, base, "3.12.16")
		os.RemoveAll(filepath.Join(env, "Lib", "site-packages", "pip")) // uv puts no pip in it
		os.WriteFile(filepath.Join(env, "uv-receipt.toml"), []byte("[tool]\nrequirements = [{ name = \"example-app-1\" }]\n"), 0o644)
		pem := filepath.Join(env, "Lib", "site-packages", "certifi", "cacert.pem")
		os.MkdirAll(filepath.Dir(pem), 0o755)
		os.WriteFile(pem, badBundle, 0o644)
		if path {
			t.Setenv("PATH", shims)
		} else {
			t.Setenv("PATH", "")
		}
		tp := testApps(t)
		it := tp.c.begin(GroupApps, "bundle", "certifi", filepath.Dir(pem), func(it *Item) { tp.bundleOne(it, pem) })
		tp.packageAdvice(it)
		return *it
	}
	got := check(filepath.Join(tools, "example-app-1"), true)
	if f := fixesIn(got.Details); len(f) != 1 || f[0].Kind != fixPackage || f[0].Manager != "uv" || f[0].key() != "uv tool upgrade example-app-1" || strings.Join(f[0].Servers, " ") != "pypi.org" ||
		!strings.Contains(got.Details, T("py.uvTool")) || strings.Contains(got.Details, "pip install") || strings.Contains(got.Details, "scoop update") {
		t.Errorf("the tool: %+v\n%s", f, got.Details)
	}
	for name, c := range map[string]struct {
		it   Item
		line string
	}{
		"no uv on the PATH": {check(filepath.Join(tools, "other-tool"), false), cmd("uv tool upgrade other-tool")},
		"another folder": {check(filepath.Join(root, "old", "tools", "example-app-1"), true),
			cmd("$env:UV_TOOL_DIR = " + psPath(filepath.Join(root, "old", "tools"), false) + "; uv tool upgrade example-app-1")},
	} {
		if fixesIn(c.it.Details) != nil || !strings.Contains(c.it.Details, T("py.uvTool")+"\n"+c.line) {
			t.Errorf("%s:\n%s", name, c.it.Details)
		}
	}
	// a virtual environment that uv did not install as a tool keeps its pip command
	plain := filepath.Join(root, "proj", ".venv")
	fakeVenv(t, plain, base, "3.12.16")
	pem := filepath.Join(plain, "Lib", "site-packages", "certifi", "cacert.pem")
	os.MkdirAll(filepath.Dir(pem), 0o755)
	os.WriteFile(pem, badBundle, 0o644)
	tp := testApps(t)
	if it := tp.c.begin(GroupApps, "bundle", "certifi", filepath.Dir(pem), func(it *Item) { tp.bundleOne(it, pem) }); !strings.Contains(it.Details, "pip install --upgrade certifi") {
		t.Errorf("not a tool:\n%s", it.Details)
	}
}

// The tool folder of uv is the one uv itself chooses.
func TestUvToolDir(t *testing.T) {
	app := t.TempDir()
	t.Setenv("APPDATA", app)
	t.Setenv("UV_TOOL_DIR", "")
	if got := uvToolDir(); got != filepath.Join(app, "uv", "tools") {
		t.Errorf("default: %q", got)
	}
	os.MkdirAll(filepath.Join(app, "uv", "data"), 0o755) // made by older versions of uv
	if got := uvToolDir(); got != filepath.Join(app, "uv", "data", "tools") {
		t.Errorf("older folder: %q", got)
	}
	t.Setenv("UV_TOOL_DIR", `C:\Users\me\scoop\persist\uv\tools\versions`)
	if got := uvToolDir(); !strings.HasSuffix(got, `scoop\persist\uv\tools\versions`) {
		t.Errorf("named: %q", got)
	}
	t.Setenv("UV_TOOL_DIR", `tools`) // relative to where uv runs: unknown
	if got := uvToolDir(); got != "" {
		t.Errorf("relative: %q", got)
	}
}
