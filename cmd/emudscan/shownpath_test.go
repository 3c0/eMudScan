package main

import (
	"strings"
	"testing"
)

// A path in a folder of the user is shown with the variable that names the folder,
// the most precise one; other paths, and other users' folders, are shown as they are.
func TestShownPath(t *testing.T) {
	env := map[string]string{"USERPROFILE": `C:\Users\me`, "APPDATA": `C:\Users\me\AppData\Roaming\`, "LOCALAPPDATA": `C:\Users\me\AppData\Local`}
	folders := namedFolders(func(v string) string { return env[v] })
	for p, want := range map[string]string{
		`C:\Users\me\AppData\Local\Builder\python\python.exe`: `%LOCALAPPDATA%\Builder\python\python.exe`,
		`c:\users\ME\appdata\roaming\App`:                     `%APPDATA%\App`,
		`C:\Users\me\scoop\apps\go\1.27.3\bin\go.exe`:         `%USERPROFILE%\scoop\apps\go\1.27.3\bin\go.exe`,
		`C:\Users\me`:                            `%USERPROFILE%`,
		`C:\Users\meg\app.exe`:                   `C:\Users\meg\app.exe`, // another user, whose name starts the same
		`C:\Program Files\Inkscape\inkscape.exe`: `C:\Program Files\Inkscape\inkscape.exe`,
		`https://example.org/`:                   `https://example.org/`,
		``:                                       ``,
	} {
		if got := shownPathIn(p, folders); got != want {
			t.Errorf("shownPath(%q) = %q, want %q", p, got, want)
		}
	}
	// a variable that is not a full path on a drive names nothing
	if got := namedFolders(func(v string) string { return map[string]string{"USERPROFILE": `Users\me`, "APPDATA": `C:\`}[v] }); len(got) != 0 {
		t.Errorf("namedFolders = %v", got)
	}
}

// In a command, a path in a folder of the user starts with the variable that names
// it, as PowerShell reads it: in double quotes when nothing in the rest of the path
// means something there, else with the rest in single quotes. A command run as
// administrator keeps the full path, as do other paths.
func TestPsPath(t *testing.T) {
	folders := []namedFolder{{"LOCALAPPDATA", `C:\Users\me\AppData\Local`}, {"USERPROFILE", `C:\Users\me`}}
	for _, c := range []struct{ p, want string }{
		{`C:\Users\me\AppData\Local\Builder\python\python.exe`, `"$env:LOCALAPPDATA\Builder\python\python.exe"`},
		{`C:\Users\me\it's ‘here’\cacert.pem`, `"$env:USERPROFILE\it's ‘here’\cacert.pem"`}, // single quotes mean nothing in double quotes
		{`C:\Users\me\a$(calc)\python.exe`, `($env:USERPROFILE + '\a$(calc)\python.exe')`},
		{"C:\\Users\\me\\a`b\\python.exe", "($env:USERPROFILE + '\\a`b\\python.exe')"},
		{`C:\Users\me\a"b\python.exe`, `($env:USERPROFILE + '\a"b\python.exe')`},
		{`C:\Users\me\a“b”\python.exe`, `($env:USERPROFILE + '\a“b”\python.exe')`},
		{`C:\Users\me\a'$b\python.exe`, `($env:USERPROFILE + '\a''$b\python.exe')`},
		{`C:\Program Files\App\python.exe`, `'C:\Program Files\App\python.exe'`},
	} {
		got := psPathIn(c.p, folders)
		if got != c.want {
			t.Errorf("psPath(%q) = %s, want %s", c.p, got, c.want)
		}
		// with the rest in single quotes, they hold it exactly
		if strings.HasPrefix(got, "(") {
			if _, values, ok := psSplit(got); !ok || len(values) != 1 || !strings.HasSuffix(c.p, values[0]) {
				t.Errorf("psPath(%q) = %s: its quoted part is %q", c.p, got, values)
			}
		}
	}
	userFolders = func() []namedFolder { return folders }
	defer func() { userFolders = func() []namedFolder { return nil } }()
	if got := psPath(`C:\Users\me\x\python.exe`, true); got != `'C:\Users\me\x\python.exe'` {
		t.Errorf("as administrator: %s", got)
	}
	if got := pipLine(`C:\Users\me\x\python.exe`, "pip", false); got != `& "$env:USERPROFILE\x\python.exe" -m pip install --upgrade pip` {
		t.Errorf("pip: %s", got)
	}
	if got := backupThenLine(`C:\Users\me\x\cacert.pem`, "$ok", "write", false); got != `if ($ok -and ($emudscanBak = Copy-Item -LiteralPath "$env:USERPROFILE\x\cacert.pem" -Destination ("$env:USERPROFILE\x\cacert.pem.emudscan-" + (Get-Date -Format 'yyyyMMdd-HHmmss') + '.bak') -PassThru)) { $emudscanBak.CreationTimeUtc = (Get-Item -LiteralPath "$env:USERPROFILE\x\cacert.pem").CreationTimeUtc; write }` {
		t.Errorf("backup: %s", got)
	}
}
