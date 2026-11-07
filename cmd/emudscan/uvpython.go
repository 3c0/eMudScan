package main

// uv's Pythons: uv installs python-build-standalone's builds in a folder of its own
// (%APPDATA%\uv\python, or the one that UV_PYTHON_INSTALL_DIR names), one folder per
// version (cpython-3.12.9-windows-x86_64-none), beside its lock file (.lock), and
// marks them as managed by uv, so pip does not change them. "uv python upgrade 3.12"
// installs the newest Python 3.12 that this uv knows (uv 0.13.6 or later knows the
// fixed ones), which the virtual environments created for 3.12 then use. uv keeps
// the old one, which the virtual environments created for that exact version still
// use: eMudScan removes it after the upgrade (uv python uninstall), only once a fixed
// version is installed, and names the virtual environments it found that use it.

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// uvKey: the folder of a Python that uv installed: its version, variant
// (+freethreaded…) and processor.
var uvKey = regexp.MustCompile(`^cpython-(\d+)\.(\d+)\.(\d+)((?:\+[a-z]+)?)-windows-(x86_64|aarch64|i686)-none$`)

// uvRuntime is a Python that uv installed in dir, the folder of uv's Pythons.
type uvRuntime struct {
	dir, key, line, variant string
}

// uvRuntimeOf tells whether uv installed the Python in home.
func uvRuntimeOf(home string) (uvRuntime, bool) {
	m := uvKey.FindStringSubmatch(filepath.Base(home))
	if m == nil || !exists(filepath.Join(filepath.Dir(home), ".lock")) {
		return uvRuntime{}, false
	}
	return uvRuntime{dir: filepath.Dir(home), key: m[0], line: m[1] + "." + m[2], variant: m[4]}, true
}

// uvPythonDir is the folder where the uv of this user installs its Pythons.
func uvPythonDir() string { return uvDir("UV_PYTHON_INSTALL_DIR", "python") }

// uvPinnedVenvs are the virtual environments found that run the Python in home
// itself, not through the folder that uv keeps for its release line
// (cpython-3.12-windows-…), which follows its upgrades: they stop working once it is
// removed.
func (tp *appsContext) uvPinnedVenvs(home string) []string {
	if tp.d == nil {
		return nil
	}
	var out []string
	for _, exe := range dedupePaths(tp.d.pythonExes) {
		env := pythonEnvFor(exe)
		if !env.venv {
			continue
		}
		if h := pyvenvHome(filepath.Join(env.home, "pyvenv.cfg")); h != "" && pathKey(h) == pathKey(home) {
			out = append(out, env.home)
		}
	}
	return out
}

// uvAdvice is how uv updates its Python rt, in home, outdated (fx): when the uv of
// this user manages its folder and is on the PATH, with a fix that upgrades it, then
// removes it once a fixed version is installed; else the command alone. "" for a
// variant (free-threaded…), which the general advice covers.
func (tp *appsContext) uvAdvice(rt uvRuntime, fx pyFix, home string) string {
	fixed := ParseVersion(fx.fixed)
	if fx.state != FixOutdated || rt.variant != "" || len(fixed) < 3 {
		return ""
	}
	upgrade := "uv python upgrade " + rt.line
	if dir := uvPythonDir(); dir == "" || pathKey(dir) != pathKey(rt.dir) {
		return T("py.uvAdvice") + "\n" + cmd("$env:UV_PYTHON_INSTALL_DIR = "+psPath(rt.dir, false)+"; "+upgrade)
	}
	if !uvOnPath() {
		return T("py.uvAdvice") + "\n" + cmd(upgrade)
	}
	next := fmt.Sprintf("%d.%d", fixed[0], fixed[1]+1)
	f := fixAction{Kind: fixPackage, Manager: "uv", Cmds: []string{upgrade,
		fmt.Sprintf("if (uv python find --system --managed-python '>=%s,<%s' 2>$null) { uv python uninstall %s } else { Write-Warning %s }",
			fx.fixed, next, rt.key, psQuote(Tf("py.uvKept", rt.line)))}}
	pinned := ""
	if venvs := tp.uvPinnedVenvs(home); len(venvs) > 0 {
		var links, shown []string
		for _, v := range venvs {
			links, shown = append(links, lk(v)), append(shown, shownPath(v))
		}
		pinned = T("py.uvVenvs") + "\n" + strings.Join(links, "\n")
		f.Warn = Tf("py.uvWarn", rt.key, strings.Join(shown, "\n"))
	}
	return joinParas(T("py.uvFix")+"\n"+newFix(f).lines(), pinned)
}
