package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// venvBundle lays out a virtual environment of base in env, with an outdated certifi
// 2024.7.4, and checks it; pip: it has pip of its own.
func venvBundle(t *testing.T, env, base string, pip bool, extra func()) Item {
	t.Helper()
	fakeVenv(t, env, base, "3.13.17")
	if !pip {
		os.RemoveAll(filepath.Join(env, "Lib", "site-packages", "pip"))
	}
	sp := filepath.Join(env, "Lib", "site-packages")
	pem := filepath.Join(sp, "certifi", "cacert.pem")
	os.MkdirAll(filepath.Dir(pem), 0o755)
	os.WriteFile(pem, outdatedBundle(t), 0o644)
	os.MkdirAll(filepath.Join(sp, "certifi-2024.7.4.dist-info"), 0o755)
	if extra != nil {
		extra()
	}
	tp := testApps(t)
	it := tp.c.begin(GroupApps, "bundle", "certifi", filepath.Dir(pem), func(it *Item) { tp.bundleOne(it, pem) })
	tp.packageAdvice(it)
	return *it
}

// pipx: its applications have no pip of their own; "pipx runpip" updates certifi,
// run when the application is in the folder of the pipx of this user and pipx is on
// the PATH, else only shown.
func TestPipx(t *testing.T) {
	Lang = 0
	root := t.TempDir()
	base := filepath.Join(root, "Python313")
	fakePython(t, base, false)
	home := filepath.Join(root, "pipx")
	t.Setenv("PIPX_HOME", home)
	bin := filepath.Join(root, "bin")
	os.MkdirAll(bin, 0o755)
	os.WriteFile(filepath.Join(bin, "pipx.exe"), []byte("MZ"), 0o644)
	app := func(dir string) func() {
		return func() { os.WriteFile(filepath.Join(dir, "pipx_metadata.json"), []byte("{}"), 0o644) }
	}
	t.Setenv("PATH", bin)
	env := filepath.Join(home, "venvs", "httpie")
	got := venvBundle(t, env, base, false, app(env))
	if f := fixesIn(got.Details); len(f) != 1 || f[0].Manager != "pipx" || f[0].key() != "pipx runpip httpie install --upgrade certifi" || strings.Join(f[0].Servers, " ") != "pypi.org" ||
		strings.Contains(got.Details, "-m pip") || strings.Contains(got.Details, T("py.noPipEnv")[:20]) {
		t.Errorf("pipx: %+v\n%s", f, got.Details)
	}
	t.Setenv("PATH", "")
	env2 := filepath.Join(home, "venvs", "black") // another name: fixes are known by their commands
	if got := venvBundle(t, env2, base, false, app(env2)); fixesIn(got.Details) != nil || !strings.Contains(got.Details, cmd("pipx runpip black install --upgrade certifi")) {
		t.Errorf("no pipx on the PATH:\n%s", got.Details)
	}
	other := filepath.Join(root, "old", "venvs", "httpie")
	if got := venvBundle(t, other, base, false, app(other)); !strings.Contains(got.Details,
		cmd("$env:PIPX_HOME = "+psPath(filepath.Join(root, "old"), false)+"; pipx runpip httpie install --upgrade certifi")) {
		t.Errorf("another folder:\n%s", got.Details)
	}
}

// The folder of pipx is the one pipx itself chooses.
func TestPipxHome(t *testing.T) {
	user, local := t.TempDir(), t.TempDir()
	t.Setenv("USERPROFILE", user)
	t.Setenv("LOCALAPPDATA", local)
	t.Setenv("PIPX_HOME", "")
	if got := pipxHome(); got != filepath.Join(local, "pipx", "pipx") {
		t.Errorf("default: %q", got)
	}
	os.MkdirAll(filepath.Join(user, "pipx"), 0o755) // older pipx on Windows
	if got := pipxHome(); got != filepath.Join(user, "pipx") {
		t.Errorf("older: %q", got)
	}
	os.MkdirAll(filepath.Join(user, ".local", "pipx"), 0o755)
	if got := pipxHome(); got != filepath.Join(user, ".local", "pipx") {
		t.Errorf("oldest: %q", got)
	}
	t.Setenv("PIPX_HOME", `D:\pipx`)
	if got := pipxHome(); !strings.HasPrefix(got, `D:\pipx`) {
		t.Errorf("named: %q", got)
	}
	t.Setenv("PIPX_HOME", `%USERPROFILE%\apps`) // pipx expands it itself
	if got := pipxHome(); got != "" {
		t.Errorf("to expand: %q", got)
	}
}

// Pipenv, Poetry and uv keep a project's environment as its lock file says: when it
// names the outdated certifi, the command that updates it in the project is shown,
// never run; after pip's, or alone where there is no pip (uv).
func TestProjectLocks(t *testing.T) {
	Lang = 0
	root := t.TempDir()
	base := filepath.Join(root, "Python313")
	fakePython(t, base, false)
	write := func(p, s string) func() {
		return func() { os.MkdirAll(filepath.Dir(p), 0o755); os.WriteFile(p, []byte(s), 0o644) }
	}
	pipfile := `{"_meta": {}, "default": {"certifi": {"hashes": [], "version": "==2024.7.4"}}, "develop": {}}`
	toml := "[[package]]\r\nname = \"certifi\"\r\nversion = \"2024.7.4\"\r\ndescription = \"x\"\r\n"
	for _, c := range []struct {
		name    string
		env     string // relative to root
		pip     bool
		extra   func(env string) func()
		want    string // the command in the project, "" for none
		withPip bool   // pip's command stays
	}{
		{"Pipenv, in the project", `app\.venv`, true, func(string) func() { return write(filepath.Join(root, "app", "Pipfile.lock"), pipfile) },
			"pipenv update certifi", true},
		{"Pipenv, in its folder", `virtualenvs\app2-AbCd1234`, true, func(env string) func() {
			return func() {
				write(filepath.Join(root, "app2", "Pipfile.lock"), pipfile)()
				write(filepath.Join(env, ".project"), filepath.Join(root, "app2"))()
			}
		}, "pipenv update certifi", true},
		{"Poetry", `app3\.venv`, true, func(string) func() { return write(filepath.Join(root, "app3", "poetry.lock"), toml) },
			"poetry update certifi", true},
		{"uv", `app4\.venv`, false, func(env string) func() {
			return func() {
				write(filepath.Join(root, "app4", "uv.lock"), toml)()
				cfg := filepath.Join(env, "pyvenv.cfg")
				data, _ := os.ReadFile(cfg)
				os.WriteFile(cfg, append(data, "uv = 0.13.6\r\n"...), 0o644)
			}
		}, "uv sync --upgrade-package certifi", false},
		{"a lock file naming another version", `app5\.venv`, true, func(string) func() {
			return write(filepath.Join(root, "app5", "poetry.lock"), strings.Replace(toml, "2024.7.4", "2026.10.23", 1))
		}, "", true},
		{"Poetry, in its cache: no project known", `pypoetry\Cache\virtualenvs\app6-x-py3.13`, true, func(string) func() {
			return write(filepath.Join(root, "app6", "poetry.lock"), toml)
		}, "", true},
	} {
		env := filepath.Join(root, filepath.FromSlash(strings.ReplaceAll(c.env, `\`, "/")))
		got := venvBundle(t, env, base, c.pip, c.extra(env))
		hasPip := strings.Contains(got.Details, "-m pip install --upgrade certifi")
		project := filepath.Dir(env)
		if c.name == "Pipenv, in its folder" {
			project = filepath.Join(root, "app2")
		}
		line := cmd("Set-Location -LiteralPath " + psPath(project, false) + "; " + c.want)
		switch {
		case hasPip != c.withPip:
			t.Errorf("%s: pip's command %v, want %v:\n%s", c.name, hasPip, c.withPip, got.Details)
		case c.want == "" && strings.Contains(got.Details, "Set-Location"):
			t.Errorf("%s: no command for the project:\n%s", c.name, got.Details)
		case c.want != "" && !strings.Contains(got.Details, line):
			t.Errorf("%s: want %q:\n%s", c.name, line, got.Details)
		case c.want != "" && !c.withPip && (!strings.Contains(got.Details, "Recommended action: its project’s uv.lock pins certifi 2024.7.4") ||
			strings.Contains(got.Details, "pip is not installed")):
			t.Errorf("%s: the project's update is the recommended action:\n%s", c.name, got.Details)
		}
		if fixesIn(got.Details) != nil && strings.Contains(strings.Join(fixesIn(got.Details)[0].Cmds, "\n"), "Set-Location") {
			t.Errorf("%s: the project's command is never run:\n%s", c.name, got.Details)
		}
	}
}
