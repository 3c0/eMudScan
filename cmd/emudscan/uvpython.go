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
	"encoding/json"
	"fmt"
	"os"
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
	// uv first, as it was installed: an older uv knows older Pythons only. Its update runs
	// with the package managers' updates (as administrator for Chocolatey), this
	// Python's after them (fixTool).
	up, hasUp := tp.uvUpdate()
	if dir := uvPythonDir(); dir == "" || pathKey(dir) != pathKey(rt.dir) {
		first := ""
		if hasUp {
			first = cmd(up.Cmds[0]) + "\n"
		}
		return T("py.uvAdvice") + "\n" + first + cmd("$env:UV_PYTHON_INSTALL_DIR = "+psPath(rt.dir, false)+"; "+upgrade)
	}
	if !uvOnPath() {
		return T("py.uvAdvice") + "\n" + cmd(upgrade)
	}
	next := fmt.Sprintf("%d.%d", fixed[0], fixed[1]+1)
	kept, text, lines := Tf("py.uvKept", rt.line), T("py.uvFix"), ""
	if hasUp { // each fix after its label, as in the Fix window, so that they stay apart
		kept, text, lines = Tf("py.uvKeptAfter", rt.line), T("py.uvFixUpdate"), fixIntro(up)+"\n"+newFix(up).lines()+"\n"
	}
	f := fixAction{Kind: fixTool, Manager: "uv", Cmds: []string{upgrade,
		fmt.Sprintf("if (uv python find --system --managed-python '>=%s,<%s' 2>$null) { uv python uninstall %s } else { Write-Warning %s }",
			fx.fixed, next, rt.key, psQuote(kept))}}
	pinned := ""
	if venvs := tp.uvPinnedVenvs(home); len(venvs) > 0 {
		var links, shown []string
		for _, v := range venvs {
			links, shown = append(links, lk(v)), append(shown, shownPath(v))
		}
		pinned = T("py.uvVenvs") + "\n" + strings.Join(links, "\n")
		f.Warn = Tf("py.uvWarn", rt.key, strings.Join(shown, "\n"))
	}
	if hasUp {
		lines += fixIntro(f) + "\n"
	}
	return joinParas(text+"\n"+lines+newFix(f).lines(), pinned)
}

// shimPathRe: the program that a Scoop shim runs (uv.shim: path = "…\apps\uv\current\uv.exe").
var shimPathRe = regexp.MustCompile(`(?m)^path\s*=\s*"?([^"\r\n]+)"?`)

// uvUpdate is the update of the uv on the PATH, as it was installed, which eMudScan
// finds by reading files only: with Scoop (its shim names the app's folder), WinGet
// (its link into a WinGet package), Chocolatey (as administrator), the standalone
// installer (its receipt names uv's folder, as "uv self update" checks) or pipx (its
// application). false when it is not known.
func (tp *appsContext) uvUpdate() (fixAction, bool) {
	update := func(manager, command string, admin bool) (fixAction, bool) {
		return fixAction{Kind: fixPackage, Manager: manager, Admin: admin, Cmds: []string{command}}, true
	}
	exe := ""
	for _, d := range pathDirs() {
		if p := filepath.Join(d, "uv.exe"); exists(p) {
			exe = p
			break
		}
	}
	if exe == "" {
		return fixAction{}, false
	}
	dir := filepath.Dir(exe)
	if data, err := os.ReadFile(filepath.Join(dir, "uv.shim")); err == nil { // Scoop
		if m := shimPathRe.FindSubmatch(data); m != nil {
			target := string(m[1])
			if s := scoopRe.FindStringSubmatch(target); s != nil && !scoopPersistRe.MatchString(target) && !needsAdmin(target) && packageIDRe.MatchString(s[2]) {
				return update("Scoop", "scoop update "+s[2], false)
			}
		}
		return fixAction{}, false
	}
	real := exe
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		real = r
	}
	if m := wingetRe.FindStringSubmatch(real); m != nil { // WinGet
		id := m[1]
		if i := strings.Index(id, "_Microsoft.Winget"); i > 0 {
			id = id[:i]
		}
		if packageIDRe.MatchString(id) && !needsAdmin(real) {
			return update("WinGet", wingetUpdate(id), false)
		}
		return fixAction{}, false
	}
	if tp.chocoRoot != "" && pathKey(dir) == pathKey(filepath.Join(tp.chocoRoot, "bin")) && isDir(filepath.Join(tp.chocoRoot, "lib", "uv")) {
		return update("Chocolatey", "choco upgrade uv -y", true)
	}
	if uvReceiptFor(dir) {
		return update("uv", "uv self update", false)
	}
	if h := pipxHome(); h != "" && exists(filepath.Join(h, "venvs", "uv", "pipx_metadata.json")) && pathKey(dir) == pathKey(pipxBinDir()) {
		return update("pipx", "pipx upgrade uv", false)
	}
	return fixAction{}, false
}

// uvReceiptFor: the receipt of uv's standalone installer names dir as uv's folder. uv
// reads the first receipt it finds ($XDG_CONFIG_HOME\uv, then %LOCALAPPDATA%\uv).
func uvReceiptFor(dir string) bool {
	var places []string
	if x := getenv("XDG_CONFIG_HOME"); x != "" && isDir(filepath.Join(x, "uv")) {
		places = append(places, filepath.Join(x, "uv"))
	}
	if l := getenv("LOCALAPPDATA"); l != "" {
		places = append(places, filepath.Join(l, "uv"))
	}
	for _, p := range places {
		data, err := os.ReadFile(filepath.Join(p, "uv-receipt.json"))
		if err != nil {
			continue
		}
		var r struct {
			InstallPrefix string `json:"install_prefix"`
		}
		if json.Unmarshal(data, &r) != nil || r.InstallPrefix == "" {
			return false
		}
		return pathKey(r.InstallPrefix) == pathKey(dir) || pathKey(filepath.Join(r.InstallPrefix, "bin")) == pathKey(dir)
	}
	return false
}

// pipxBinDir is the folder where pipx puts the commands of its applications.
func pipxBinDir() string {
	if b := getenv("PIPX_BIN_DIR"); b != "" {
		return filepath.Clean(b)
	}
	if u := getenv("USERPROFILE"); u != "" {
		return filepath.Join(u, ".local", "bin")
	}
	return ""
}
