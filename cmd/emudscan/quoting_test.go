package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// PowerShell takes the curly single quotes for quotes too.
func isPSSingleQuote(r rune) bool {
	return r == '\'' || r == '\u2018' || r == '\u2019' || r == '\u201a' || r == '\u201b'
}

// psSplit reads a PowerShell command line as PowerShell does for its single-quoted
// strings: it returns the code outside them, where each string is left as an empty
// one, and the strings' values. ok is false when a string is not closed.
func psSplit(line string) (code string, values []string, ok bool) {
	rs := []rune(line)
	var c strings.Builder
	for i := 0; i < len(rs); i++ {
		if !isPSSingleQuote(rs[i]) {
			c.WriteRune(rs[i])
			continue
		}
		var v strings.Builder
		closed := false
		for i++; i < len(rs); i++ {
			if isPSSingleQuote(rs[i]) {
				if i+1 < len(rs) && isPSSingleQuote(rs[i+1]) { // a quote doubled: one quote
					i++
					v.WriteRune(rs[i])
					continue
				}
				closed = true
				break
			}
			v.WriteRune(rs[i])
		}
		if !closed {
			return c.String(), values, false
		}
		values = append(values, v.String())
		c.WriteString("''")
	}
	return c.String(), values, true
}

// Folder names that would end a quoted string, or that PowerShell would run inside a
// double-quoted one. The names of the commands they hold are looked for in the code.
var hostileNames = []string{
	"’; Start-Process cmd; ’",
	"‘; Start-Process cmd; ‘",
	"‚; Start-Process cmd; ‛",
	"'; Start-Process cmd; '",
	"$(Start-Process cmd)",
	"x\"; Start-Process cmd; \"",
}

// checkCommands fails the test when a command line of details holds anything of the
// hostile names outside a quoted string, or a double-quoted string at all.
func checkCommands(t *testing.T, what, details string) int {
	t.Helper()
	n := 0
	for _, line := range strings.Split(details, "\n") {
		c, ok := strings.CutPrefix(line, "$ ")
		if !ok {
			continue
		}
		n++
		code, _, closed := psSplit(c)
		if !closed || strings.Contains(code, "Start-Process") || strings.ContainsAny(code, "\"“”„") {
			t.Errorf("%s: unsafe command (code outside the quotes: %q):\n%s", what, code, c)
		}
	}
	return n
}

// psQuote's strings are read back as they were, whatever quotes they hold, and
// nothing in them is left outside the string.
func TestPsQuote(t *testing.T) {
	for _, s := range append([]string{"", "plain", `C:\Users\o'neil\$x\cert.pem`, "''", "‘’", "a'b‘c’d‚e‛f", "`$env:x"}, hostileNames...) {
		q := psQuote(s)
		code, values, ok := psSplit(q)
		if !ok || code != "''" || len(values) != 1 || values[0] != s {
			t.Errorf("%q: %s gives %q %q %v", s, q, code, values, ok)
		}
	}
}

// The commands built from a path keep the path inside a string.
func TestCommandsWithHostilePaths(t *testing.T) {
	Lang = 0
	for _, name := range hostileNames {
		p := filepath.Join(`C:\ProgramData`, name, "cacert.pem")
		checkCommands(t, "replace "+name, replaceBundleFix(p))
		checkCommands(t, "pip "+name, pipCmd(filepath.Join(`C:\Users\me`, name, "python.exe"), "certifi"))
	}
}

// An outdated bundle in a folder that any user may create, named to add a command of
// its own: the proposed commands, which run as administrator there, keep that name
// inside strings. The same for a Python whose folder holds $(…).
func TestHostileFolderNames(t *testing.T) {
	Lang = 0
	pd := t.TempDir()
	t.Setenv("ProgramData", pd)
	tp := testApps(t)
	tp.d = newDiscovery()
	for i, name := range hostileNames {
		if strings.ContainsAny(name, `"`) { // not allowed in Windows file names
			continue
		}
		dir := filepath.Join(pd, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		p := filepath.Join(dir, "cacert.pem")
		os.WriteFile(p, outdatedBundle(t), 0o644)
		os.WriteFile(p+".emudscan-20261031-011500.bak", []byte("before"), 0o644)
		tp.d.bundles[p] = true

		py := filepath.Join(pd, "py"+itoa(i), name)
		fakePython(t, py, false)
		certifi := filepath.Join(py, "Lib", "site-packages", "certifi", "cacert.pem")
		os.MkdirAll(filepath.Dir(certifi), 0o755)
		os.WriteFile(certifi, outdatedBundle(t), 0o644)
		tp.d.pythonExes[filepath.Join(py, "python.exe")] = true
	}
	tp.checkBundles()
	tp.checkPython()
	n := 0
	for _, it := range tp.c.snapshot() {
		n += checkCommands(t, it.Name+" "+it.File, it.Details)
	}
	if n < 10 {
		t.Errorf("only %d commands checked", n)
	}
}

// A package name read from a folder name is used in a command only when it is an
// ordinary one: otherwise the package is still named, without a command.
func TestPackageNamesInCommands(t *testing.T) {
	for id, ok := range map[string]bool{
		"yt-dlp": true, "Git.Git": true, "python3_x64": true, "Example++.App++": true, "1app.1app": true,
		"": false, "a b": false, "x;calc": false, "$(calc)": false, "’x": false, "-x": false, ".x": false, "x|y": false,
	} {
		p := withCommands(pkgRef{"Scoop", id, "", true, "scoop update " + id, "scoop update -a"})
		if (p.Update != "") != ok || (p.UpdateAll != "") != ok || p.ID != id {
			t.Errorf("%q: %+v", id, p)
		}
	}
	tp := testApps(t)
	for _, c := range []struct {
		file, manager, cmd string
	}{
		{`C:\Users\me\scoop\apps\x;calc\current\x.exe`, "Scoop", ""},
		{`C:\Users\me\AppData\Local\Microsoft\WinGet\Packages\Evil$(calc)_Microsoft.Winget.Source_8wekyb3d8bbwe\x.exe`, "WinGet", ""},
		{`C:\Users\me\scoop\apps\example-app-1\current\example-app-1.exe`, "Scoop", "$ scoop update example-app-1"},
		// WinGet's command accepts its agreements, which it would otherwise ask for
		{`C:\Users\me\AppData\Local\Microsoft\WinGet\Packages\yt-dlp.yt-dlp_Microsoft.Winget.Source_8wekyb3d8bbwe\yt-dlp.exe`, "WinGet",
			"$ winget upgrade --id yt-dlp.yt-dlp --exact --accept-package-agreements --accept-source-agreements\n"},
	} {
		it := Item{Group: GroupApps, Kind: "tool", Name: "x", File: c.file, Status: StWarn, Details: "Outdated."}
		tp.packageAdvice(&it)
		if len(it.Packages) != 1 || it.Packages[0].Manager != c.manager {
			t.Errorf("%s: packages %+v", c.file, it.Packages)
		}
		if c.cmd == "" && (strings.Contains(it.Details, "$ ") || it.Details != "Outdated.") || c.cmd != "" && !strings.Contains(it.Details, c.cmd) {
			t.Errorf("%s: details\n%s", c.file, it.Details)
		}
	}
}
