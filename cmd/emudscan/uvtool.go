package main

// uv's tools: uv installs each tool (uv tool install) in a virtual environment of its
// own, in its tool folder (%APPDATA%\uv\tools, or the one that UV_TOOL_DIR names),
// named after the tool, beside its receipt (uv-receipt.toml). uv does not put pip in
// it, and its documentation says never to change it with pip: "uv tool upgrade" updates
// the tool and all the packages in its environment, certifi among them.

import (
	"path/filepath"
	"regexp"
	"strings"
)

// uvToolNameRe: a tool's name, as uv names its folder, fit for a command.
var uvToolNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// uvToolDir is the folder where the uv of this user installs its tools.
func uvToolDir() string { return uvDir("UV_TOOL_DIR", "tools") }

// isUvTool: the virtual environment env is that of a tool that uv installed.
func isUvTool(env pyEnv) bool {
	return env.venv && exists(filepath.Join(env.home, "uv-receipt.toml")) && uvToolNameRe.MatchString(filepath.Base(env.home))
}

// uvToolAdvice is how uv updates the packages of its tool in home: when the uv of this
// user manages its folder and is on the PATH, with a fix; else the command alone.
func uvToolAdvice(home string) string {
	upgrade := "uv tool upgrade " + filepath.Base(home)
	if dir := uvToolDir(); dir == "" || pathKey(dir) != pathKey(filepath.Dir(home)) {
		return T("py.uvTool") + "\n" + cmd("$env:UV_TOOL_DIR = "+psPath(filepath.Dir(home), false)+"; "+upgrade)
	}
	if !uvOnPath() {
		return T("py.uvTool") + "\n" + cmd(upgrade)
	}
	return T("py.uvTool") + "\n" + newFix(fixAction{Kind: fixPackage, Manager: "uv", Cmds: []string{upgrade}, Servers: serversPyPI}).lines()
}

// uvDir is a folder of the uv of this user, as uv chooses it: the one that variable
// names (UV_PYTHON_INSTALL_DIR, UV_TOOL_DIR, which Scoop sets), else name in its data
// folder (%APPDATA%\uv\data when it exists, as older versions of uv made it, else
// %APPDATA%\uv); "" when it cannot be known.
func uvDir(variable, name string) string {
	if d := getenv(variable); d != "" {
		if !filepath.IsAbs(d) && !winAbs(d) { // relative to the folder uv runs in
			return ""
		}
		return filepath.Clean(d)
	}
	app := getenv("APPDATA")
	if app == "" {
		return ""
	}
	if isDir(filepath.Join(app, "uv", "data")) {
		return filepath.Join(app, "uv", "data", name)
	}
	return filepath.Join(app, "uv", name)
}

// winAbs: p is an absolute Windows path (C:\…, \\server\…).
func winAbs(p string) bool {
	return len(p) >= 3 && p[1] == ':' && (p[2] == '\\' || p[2] == '/') || strings.HasPrefix(p, `\\`)
}

// uvOnPath: the uv command, which the fix runs, is in a folder of the PATH.
func uvOnPath() bool { return onPath("uv.exe") }

// onPath: the command exe is in a folder of the PATH.
func onPath(exe string) bool {
	for _, d := range pathDirs() {
		if exists(filepath.Join(d, exe)) {
			return true
		}
	}
	return false
}
