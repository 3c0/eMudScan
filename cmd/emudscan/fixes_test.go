package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The commands that replace a certificate bundle are one fix, a script: those that the
// Fix function runs, with the backups; without them, the same without its backup
// block; and the details show it as one block that runs alone.
func TestFixBundle(t *testing.T) {
	Lang = 0
	pd := t.TempDir()
	t.Setenv("ProgramData", pd)
	a, b := filepath.Join(pd, "App", "cacert.pem"), filepath.Join(t.TempDir(), "x‘y", "b.crt")
	text := replaceBundleFix(a, b)
	f := fixesIn("Outdated.\n\n" + text + "\n\nAfter.")
	if len(f) != 1 {
		t.Fatalf("fixes: %+v", f)
	}
	fx := f[0]
	want := []string{
		"# The certificate bundles to replace",
		"$files = " + psQuote(a) + ", " + psQuote(b),
		"",
		"# Download Mozilla’s certificate bundle, as published by curl, into memory, and check its SHA-256",
		"[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12",
		"$cacert = (Invoke-WebRequest -UseBasicParsing -Uri https://curl.se/ca/cacert.pem).RawContentStream.ToArray()",
		"$sha256 = ([Text.Encoding]::ASCII.GetString((Invoke-WebRequest -UseBasicParsing -Uri https://curl.se/ca/cacert.pem.sha256).RawContentStream.ToArray()) -split '\\s')[0]",
		"if ((Get-FileHash -InputStream ([IO.MemoryStream]::new($cacert))).Hash -ne $sha256) { throw " + psQuote(T("fix.badSum")) + " }",
		"",
		"# Back up the files, with their dates",
		"$stamp = Get-Date -Format 'yyyyMMdd-HHmmss'",
		`$files | ForEach-Object { Copy-Item -LiteralPath $_ -Destination "$_.emudscan-$stamp.bak" }`,
		`$files | ForEach-Object { (Get-Item -LiteralPath "$_.emudscan-$stamp.bak").CreationTime = (Get-Item -LiteralPath $_).CreationTime }`,
		"",
		"# Replace the files",
		"$files | ForEach-Object { [IO.File]::WriteAllBytes($_, $cacert) }",
	}
	if fx.Kind != fixBundle || !fx.Admin || !fx.Script || len(fx.Files) != 2 || strings.Join(fx.Cmds, "\n") != strings.Join(want, "\n") || !strings.HasSuffix(text, fx.lines()) || !fetchesRight(fx) ||
		fmt.Sprint(fx.Fetch) != "[5 6]" {
		t.Errorf("fix: %+v\n%s", fx, strings.Join(fx.Cmds, "\n"))
	}
	// without the backups: the same, but its backup block
	if strings.Join(fx.Plain, "\n") != strings.Join(append(append([]string(nil), want[:8]...), want[13:]...), "\n") {
		t.Errorf("without backup:\n%s", strings.Join(fx.Plain, "\n"))
	}
	// the details: one block, which stops at its first error
	if fx.Alone[0] != "& {" || fx.Alone[1] != "    $ErrorActionPreference = 'Stop'" || fx.Alone[3] != "    "+want[0] || fx.Alone[len(fx.Alone)-1] != "}" || len(fx.Alone) != len(want)+4 {
		t.Errorf("alone:\n%s", strings.Join(fx.Alone, "\n"))
	}
	// in a user's folder, no administrator rights
	if f := fixesIn(replaceBundleFix(b)); len(f) != 1 || f[0].Admin {
		t.Errorf("user file: %+v", f)
	}
}

// Adding the compromised certificates to a Java block list is one fix.
func TestFixJava(t *testing.T) {
	Lang = 0
	pf := t.TempDir()
	t.Setenv("ProgramFiles", pf)
	bl := javaBlockList{file: filepath.Join(pf, "Java", "lib", "security", "blocked.certs")}
	f := fixesIn(javaBlockCmd(bl, javaRoots))
	if len(f) != 1 || f[0].Kind != fixJava || !f[0].Admin || !f[0].Script || f[0].Alone == nil {
		t.Fatalf("fixes: %+v", f)
	}
	// a script: the list and the certificates, the backup, then the addition; without
	// the backups, the same without its backup block
	cmds := strings.Join(f[0].Cmds, "\n")
	if !strings.HasPrefix(cmds, "# The block list of this Java, and the certificates to add to it\n$list  = "+psQuote(bl.file)+"\n$certs = '") ||
		!strings.Contains(cmds, "\n\n# Back up the block list, with its dates\n$stamp = Get-Date -Format 'yyyyMMdd-HHmmss'\nCopy-Item -LiteralPath $list -Destination \"$list.emudscan-$stamp.bak\"\n") ||
		!strings.HasSuffix(cmds, "\n\n# Add the certificates to the block list\nAdd-Content -LiteralPath $list -Value '', $certs") {
		t.Errorf("script:\n%s", cmds)
	}
	if plain := strings.Join(f[0].Plain, "\n"); strings.Contains(plain, "Copy-Item") || !strings.HasSuffix(plain, "Add-Content -LiteralPath $list -Value '', $certs") {
		t.Errorf("without backup:\n%s", plain)
	}
}

// Only what is recorded as a fix is run: not the commands that restore a backup,
// update all packages or remove a setting, nor commands the details do not show.
// An element gets its fixes when it is finished, in the order of its details.
func TestFixesOfElement(t *testing.T) {
	Lang = 0
	file := filepath.Join(t.TempDir(), "cacert.pem")
	pkg := newFix(fixAction{Kind: fixPackage, Cmds: []string{"scoop update app"}, Manager: "Scoop"})
	details := "Outdated.\n\n" + Tf("fix.pkg", "Scoop") + "\n" + pkg.lines() + "\n\n" +
		how(T("fix.orDirectly")) + "\n" + replaceBundleFix(file) + "\n\n" + T("set.remove") + "\n" + how(T("fix.ps")) + "\n$ git config --global --unset http.sslCAInfo\n\n" +
		how(T("fix.ps")) + "\n$ Move-Item -LiteralPath a -Destination b -Force"
	c := newChecker(nopReporter{}, nil)
	it := c.begin(GroupApps, "bundle", "App", "", func(it *Item) { c.finish(it, result{Status: StWarn, Details: details}) })
	if len(it.Fixes) != 2 || it.Fixes[0].Kind != fixPackage || it.Fixes[1].Kind != fixBundle {
		t.Errorf("fixes: %+v", it.Fixes)
	}
	// the same commands shown twice are one fix
	if f := fixesIn(pkg.lines() + "\n\nAgain:\n" + pkg.lines()); len(f) != 1 {
		t.Errorf("twice: %+v", f)
	}
	// a part of a fix's commands is not that fix
	if f := fixesIn(cmd(pkg.Cmds[0]) + "\n$ extra"); len(f) != 0 {
		t.Errorf("changed commands: %+v", f)
	}
}

// The Fix window ticks the software whose fixes it can run, except those whose fixes
// are all optional (a component inside a program replaced): the flag goes with the
// fix that the details show, and one fix that is not optional is enough to tick it.
func TestTickedFirst(t *testing.T) {
	Lang = 0
	light := newFix(fixAction{Kind: fixBundle, Cmds: []string{"Copy-Item a b"}})
	heavy := newFix(fixAction{Kind: fixBundle, Cmds: []string{"Copy-Item c d"}, Optional: true})
	c := newChecker(nopReporter{}, nil)
	item := func(fixes ...fixAction) Item {
		var parts []string
		for _, f := range fixes {
			parts = append(parts, how(T("fix.ps"))+"\n"+f.lines())
		}
		return *c.begin(GroupApps, "bundle", "App", "", func(it *Item) {
			c.finish(it, result{Status: StWarn, Details: strings.Join(parts, "\n\n")})
		})
	}
	for i, x := range []struct {
		items []Item
		want  bool
	}{
		{[]Item{item(light)}, true},
		{[]Item{item(heavy)}, false},
		{[]Item{item(heavy, light)}, true},
		{[]Item{item(heavy), item(light)}, true},
		{[]Item{item(heavy), item(heavy)}, false},
		{nil, false},
	} {
		if got := tickedFirst(x.items); got != x.want {
			t.Errorf("case %d: %v, want %v", i, got, x.want)
		}
	}
	if it := item(heavy); len(it.Fixes) != 1 || !it.Fixes[0].Optional {
		t.Errorf("the optional fix as the details show it: %+v", it.Fixes)
	}
}

// conda's certificates are updated by the conda that manages the environment, named
// by its full path, for that environment (-p), without asking: the Fix function runs
// that command. Without a conda, the command alone is shown, and not run.
func TestCondaFix(t *testing.T) {
	Lang = 0
	t.Setenv("ProgramData", "")
	root := t.TempDir()
	write := func(rel string) {
		p := filepath.Join(root, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte("x"), 0o644)
	}
	write(filepath.Join("anaconda3", "Scripts", "conda.exe"))
	base := filepath.Join(root, "anaconda3")
	env := filepath.Join(base, "envs", "work")
	conda := psQuote(filepath.Join(base, "Scripts", "conda.exe"))
	for _, c := range []struct{ env, want string }{
		{base, "& " + conda + " update -y -p " + psQuote(base) + " ca-certificates certifi"},
		{env, "& " + conda + " update -y -p " + psQuote(env) + " ca-certificates certifi"},
	} {
		text := condaFix(filepath.Join(c.env, "Library", "ssl", "cacert.pem"))
		if f := fixesIn(text); len(f) != 1 || f[0].Kind != fixConda || f[0].key() != c.want || !strings.HasPrefix(text, T("conda.fix")+"\n"+how(T("fix.ps"))+"\n") ||
			strings.Join(f[0].Servers, " ") != "repo.anaconda.com conda.anaconda.org" {
			t.Errorf("%s: %q %+v", c.env, text, f)
		}
	}
	other := filepath.Join(root, "miniforge")
	if text := condaFix(filepath.Join(other, "Library", "ssl", "cacert.pem")); len(fixesIn(text)) != 0 || !strings.Contains(text, "$ conda update ca-certificates certifi") {
		t.Errorf("no conda: %q", text)
	}
}

// "Back up the files to change" is shown only for fixes that back up files: not for
// updates (pip, a package manager), which make no backup.
func TestBacksUp(t *testing.T) {
	pip := Item{Fixes: []fixAction{{Kind: fixPip, Cmds: []string{"python -m pip install --upgrade certifi"}}}}
	bundle := Item{Fixes: []fixAction{{Kind: fixBundle, Cmds: []string{"backup; write"}, Plain: []string{"write"}}}}
	for i, x := range []struct {
		items []Item
		want  bool
	}{
		{[]Item{pip}, false},
		{[]Item{pip, pip}, false},
		{[]Item{pip, bundle}, true},
		{[]Item{bundle}, true},
		{nil, false},
	} {
		if got := backsUp(x.items); got != x.want {
			t.Errorf("case %d: %v, want %v", i, got, x.want)
		}
	}
}

// fetchesRight: the commands of f that download (fixAction.Fetch) are those that call
// Invoke-WebRequest or Invoke-RestMethod, the same with and without the backups, and
// none comes after a command that changes a file (a backup, a copy, a write).
func fetchesRight(f fixAction) bool {
	changed := false
	for i, c := range f.Cmds {
		downloads := strings.Contains(c, "Invoke-WebRequest") || strings.Contains(c, "Invoke-RestMethod")
		if downloads != f.fetches(i) || downloads && (changed || i >= len(f.Plain) || f.Plain[i] != c) {
			return false
		}
		changed = changed || strings.Contains(c, "Copy-Item") || strings.Contains(c, "WriteAllBytes") || strings.Contains(c, "Move-Item")
	}
	return len(f.Fetch) > 0
}
