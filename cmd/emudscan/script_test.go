package main

import (
	"debug/pe"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The lines of a script: its commands, and the lines that have no result.
func TestScriptLines(t *testing.T) {
	for line, want := range map[string]bool{
		"$a = 1": true, "    & $python -I $check": true, "if ($a) { throw 'x' }": true, "    throw 'x'": true, "& { $a }": true,
		"": false, "   ": false, "# a comment": false, "    # a comment": false, "try {": false, "}": false, "catch {": false, "    }": false, "& {": false,
	} {
		if isCommand(line) != want {
			t.Errorf("isCommand(%q) = %v", line, !want)
		}
	}
	// every line of another fix is a command, as before
	if f := (fixAction{}); !f.runs("# x") || !f.runs("}") {
		t.Error("a fix that is not a script")
	}
	if f := (fixAction{Script: true}); f.runs("# x") || f.runs("}") || !f.runs("$a = 1") {
		t.Error("a script")
	}
}

// The details show a script as a block of commands, its empty lines too, which are
// the mark alone; the fix is found again from them, and a text read from the PC cannot
// make such a line.
func TestScriptInDetails(t *testing.T) {
	f := newFix(fixAction{Kind: fixPySSL, Script: true, Cmds: []string{"# what it does", "$a = 1", "", "try {", "    $b = 2", "}", "catch {", "    throw 'no'", "}"}})
	lines := f.lines()
	if want := "$ # what it does\n$ $a = 1\n$\n$ try {\n$     $b = 2\n$ }\n$ catch {\n$     throw 'no'\n$ }"; lines != want {
		t.Fatalf("lines:\n%s", lines)
	}
	details := "Some text.\nIn PowerShell:\n" + lines + "\n\nMore text."
	if got := fixesIn(details); len(got) != 1 || got[0].key() != f.key() || !got[0].Script {
		t.Errorf("fixesIn: %+v", got)
	}
	blocks := markupBlocks(details)
	var cmds [][]string
	for _, b := range blocks {
		if b.kind == dkCmds {
			cmds = append(cmds, b.cmds)
		}
	}
	if len(cmds) != 1 || strings.Join(cmds[0], "\n") != strings.Join(f.Cmds, "\n") {
		t.Errorf("blocks: %q", cmds)
	}
	if got := commandsIn(details); strings.Join(got, "\n") != strings.Join(f.Cmds, "\n") {
		t.Errorf("commandsIn: %q", got)
	}
	if got, want := plainText(lines), "    # what it does\n    $a = 1\n    \n    try {\n        $b = 2\n    }\n    catch {\n        throw 'no'\n    }"; got != want {
		t.Errorf("plainText:\n%q", got)
	}
	for _, s := range []string{"$", " $", "$ x"} {
		if _, ok := cmdOf(plain(s)); ok {
			t.Errorf("plain(%q) reads as a command", s)
		}
	}
	if c, ok := cmdOf(cmd("")); !ok || c != "" {
		t.Error("an empty line")
	}
	if c, ok := cmdOf(cmd("  x")); !ok || c != "  x" {
		t.Error("an indented line")
	}
	if _, ok := cmdOf("$x"); ok {
		t.Error("$x is not a command")
	}
}

// The script that runs a fix that is a script: its commands one by one, as those of
// the other fixes, inside a block that ends the fix when one fails; the lines of its
// own blocks as they are; nothing for its comments and its empty lines.
func TestFixScriptOfScript(t *testing.T) {
	if fixSimulation {
		t.Skip("the simulation replaces the commands")
	}
	script := fixAction{Kind: fixPySSL, Script: true, Fetch: []int{2}, Cmds: []string{"# what it does", "$a = 1", "download", "", "try {", "    copy", "}", "catch {", "    put back", "    throw 'no'", "}"}}
	other := fixAction{Kind: fixBundle, Cmds: []string{"$b = 1", "$b = 2"}}
	s := fixScript([]fixUnit{{fix: script, cmds: script.Cmds}, {fix: other, cmds: other.Cmds}}, `C:\Temp\fix.log`, "##m", nil)
	stop := "if ($emudscanFailed) { throw 'eMudScan: a command failed' }\n"
	end := func(u, c int) string {
		return fmt.Sprintf("Write-EmudscanLog ('##m end %d %d ' + [int]$emudscanFailed + ' ' + $emudscanCode)\n", u, c)
	}
	at := 0
	for _, w := range []string{
		"\ntry {\nWrite-EmudscanLog '##m cmd 0 1'\n", "\n$a = 1\n", end(0, 1) + stop + "Write-EmudscanLog '##m cmd 0 2'\n",
		// a failed download stops the run, as before; else the fix
		end(0, 2) + "if ($emudscanFailed) { Write-EmudscanLog '##m halt 0 2'; exit 0 }\n" + stop + "try {\nWrite-EmudscanLog '##m cmd 0 5'\n", "\n    copy\n",
		end(0, 5) + stop + "}\ncatch {\nWrite-EmudscanLog '##m cmd 0 8'\n", "\n    put back\n", end(0, 8) + stop + "Write-EmudscanLog '##m cmd 0 9'\n", "\n    throw 'no'\n",
		end(0, 9) + stop + "}\n} catch { }\nWrite-EmudscanLog '##m cmd 1 0'\n", end(1, 0) + "Write-EmudscanLog '##m cmd 1 1'\n", end(1, 1) + "exit 0\n",
	} {
		i := strings.Index(s[at:], w)
		if i < 0 {
			t.Fatalf("%q missing or out of order in:\n%s", w, s)
		}
		at += i + len(w)
	}
	if strings.Contains(s, "what it does") || strings.Count(s, "cmd 0 ") != 5 || strings.Count(s, stop) != 5 {
		t.Errorf("script:\n%s", s)
	}
}

// The console has the whole script of a fix, in its order: a result for each command
// that ran; none for its comments, its empty lines, the lines of its blocks and the
// commands that did not run.
func TestFixLogOfScript(t *testing.T) {
	Lang = 0
	if fixSimulation {
		t.Skip("the simulation has its own results")
	}
	script := fixAction{Kind: fixPySSL, Script: true, Cmds: []string{"# what it does", "$a = 1", "", "try {", "    copy", "    test", "}", "catch {", "    put back", "    throw 'no'", "}", "", "# the end", "clean"}}
	other := fixAction{Kind: fixBundle, Cmds: []string{"# not a comment here", "$b = 2"}}
	units := []fixUnit{{ids: []int{1}, titles: []string{"A"}, names: []string{"A"}, places: []string{""}, label: "Fix: Replace", fix: script, cmds: script.Cmds},
		{ids: []int{2}, titles: []string{"B"}, names: []string{"B"}, places: []string{""}, label: "Fix: Bundle", fix: other, cmds: other.Cmds}}
	run := func(log string, titles bool) ([]string, []unitResult) {
		var console []fixLine
		l := newFixLog(units, "##m", func(s fixLine) { console = append(console, s) }, titles)
		for i := 0; i < len(log); i += 5 {
			l.write(log[i:min(i+5, len(log))])
		}
		l.close()
		return reportLines(console), l.results
	}
	ok := func(c int) string { return fmt.Sprintf("##m cmd 0 %d\n##m end 0 %d 0 0\n", c, c) }
	// all goes well: the "catch" did not run
	got, res := run(ok(1)+ok(4)+ok(5)+ok(13)+"##m cmd 1 0\n##m end 1 0 0 0\n##m cmd 1 1\n##m end 1 1 0 0\n", false)
	// the line of the fix, an empty line before its own comments; an empty line between two fixes
	want := []string{"# Fix: Replace", "", "    # what it does", "  ✓ $a = 1", "", "    try {", "  ✓     copy", "  ✓     test", "    }", "    catch {", "        put back", "        throw 'no'", "    }", "", "    # the end", "  ✓ clean",
		"", "# Fix: Bundle", "  ✓ # not a comment here", "  ✓ $b = 2"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") || !res[0].ok || !res[1].ok {
		t.Errorf("all well: %+v\n%s", res, strings.Join(got, "\n"))
	}
	// the test fails: the "catch" ran, and the fix stopped there; the next fix ran
	got, res = run(ok(1)+ok(4)+"##m cmd 0 5\nno ssl\n##m end 0 5 1 1\n"+ok(8)+"##m cmd 0 9\n##m! no\n##m end 0 9 1 0\n"+"##m cmd 1 0\n##m end 1 0 0 0\n", true)
	want = []string{Tf("fix.software", "A"), "# Fix: Replace", "", "    # what it does", "  ✓ $a = 1", "", "    try {", "  ✓     copy", "  ✗     test", "      no ssl", "      Exit code 1", "    }", "    catch {", "  ✓     put back", "  ✗     throw 'no'", "      no", "    }", "", "    # the end", "    clean",
		"", scriptRule, "", Tf("fix.software", "B"), "# Fix: Bundle", "  ✓ # not a comment here"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") || !res[0].ran || res[0].ok || !res[1].ok {
		t.Errorf("the test fails: %+v\n%s", res, strings.Join(got, "\n"))
	}
	// it stops at its first command: the rest has no result
	got, res = run("##m cmd 0 1\n##m! broken\n##m end 0 1 1 0\n", false)
	if strings.Join(got, "\n") != "# Fix: Replace\n\n    # what it does\n  ✗ $a = 1\n      broken\n\n    try {\n        copy\n        test\n    }\n    catch {\n        put back\n        throw 'no'\n    }\n\n    # the end\n    clean" || res[0].ok || res[1].ran {
		t.Errorf("stops at once: %+v\n%s", res, strings.Join(got, "\n"))
	}
	// the preview: the same lines, the commands apart
	lines := fixPreview([]Item{{ID: 1, Name: "A", Fixes: []fixAction{script}}}, true)
	var kinds []string
	for _, l := range lines {
		switch l.kind {
		case lineCommand:
			kinds = append(kinds, "C "+l.text)
		case lineScript:
			kinds = append(kinds, "S "+l.text)
		}
	}
	if got, want := strings.Join(kinds, "\n"), "S # what it does\nC $a = 1\nS \nS try {\nC     copy\nC     test\nS }\nS catch {\nC     put back\nC     throw 'no'\nS }\nS \nS # the end\nC clean"; got != want {
		t.Errorf("preview:\n%s", got)
	}
}

// The console makes the name of the file of the checks, in the script of the fix of a
// Python, a link that shows the file: where it is on its line, as the console counts.
func TestCheckFileLink(t *testing.T) {
	Lang = 0
	f := pySSLFix(`C:\Example App 1\python\python.exe`, pySSLPlan{src: pbsSSL["3.11"], dir: `C:\Example App 1\python\DLLs`, replace: pySSLShapes[0].replace})
	found := 0
	for _, l := range fixPreview([]Item{{ID: 1, Name: "Example App 1", Fixes: []fixAction{f}}}, true) {
		n, tail, ok := checkFileLink(l)
		if !ok {
			continue
		}
		found++
		if strings.HasPrefix(l.text, "# ") { // the comment that says what the folder of the fix holds
			if l.kind != lineScript || l.text[len(l.text)-tail-n:len(l.text)-tail] != pyCheckName {
				t.Errorf("%q: %d, %d", l.text, n, tail)
			}
		} else if l.text != `$check  = "$emudscanFiles\check_ssl.py"` || n != 12 || tail != 1 {
			t.Errorf("%q: %d, %d", l.text, n, tail)
		}
	}
	if found != 2 {
		t.Errorf("%d lines name the file", found)
	}
	// counted as the console does, in UTF-16; not in what a command showed
	if n, tail, ok := checkFileLink(fixLine{lineScript, "# check_ssl.py — é😀"}); !ok || n != 12 || tail != 6 {
		t.Errorf("after the name: %d, %d, %v", n, tail, ok)
	}
	if _, _, ok := checkFileLink(fixLine{lineOutput, "check_ssl.py"}); ok {
		t.Error("an output is not a line of the script")
	}
	// the script that runs alone does not use the file
	if strings.Contains(f.lines(), pyCheckName) {
		t.Error("the details name the file")
	}
	if string(mustFiles(t)[pyCheckName]) != pyCheckScript || !strings.HasPrefix(pyCheckScript, "# eMudScan's checks of a Python's ssl module") {
		t.Error("the file of the checks")
	}
}

func mustFiles(t *testing.T) map[string][]byte {
	t.Helper()
	files, err := pyCheckFiles(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// The reports give the details without their commands: the command lines and the
// lines about them are left out; the sentence that led to commands stays, and ends
// with a period.
func TestWithoutCommands(t *testing.T) {
	Lang = 0
	for _, c := range []struct{ name, details, want string }{
		{"no command", "Not affected.\n\nNo action needed.", "Not affected.\n\nNo action needed."},
		{"a sentence, then its command", "Recommended action: update it with uv, which installed it:\n$ uv python upgrade 3.12", "Recommended action: update it with uv, which installed it."},
		{"in French", "Action recommandée\u00a0: mettez-le à jour avec uv\u00a0:\n$ uv python upgrade 3.12", "Action recommandée\u00a0: mettez-le à jour avec uv."},
		{"text after the command", "Recommended action: update Flutter:\n$ flutter upgrade\n\nThen rebuild your apps.", "Recommended action: update Flutter.\n\nThen rebuild your apps."},
		{"what announces the commands", "Its certifi is outdated.\n" + how(T("fix.ps")) + "\n$ & {\n$     $a = 1\n$\n$ }", "Its certifi is outdated."},
		{"what eMudScan offers to do", "Recommended action: update Example App 1.\n\n" + how("Until then, eMudScan can replace the files. Close it first.") + "\n" + how(T("fix.ps")) + "\n$ a", "Recommended action: update Example App 1."},
		{"what explains them", "Outdated.\n\n" + how("The following commands replace the file.\n\nPowerShell downloads it.") + "\n" + how(T("fix.psAdmin")) + "\n$ a\n$ b\n\nScan again.", "Outdated.\n\nScan again."},
		{"a sentence, lines about the commands, the commands", "Recommended action: update uv, then this Python:\n" + how("The last command removes this copy:") + "\n$ a\n" + how("Update with uv:") + "\n$ b", "Recommended action: update uv, then this Python."},
		{"a bold sentence", "**Or fix the file directly:**\n$ a", "**Or fix the file directly.**"},
		{"only commands", how(T("fix.ps")) + "\n$ a", ""},
		{"a restore", "**Restore available:** backed up to:\n• a.bak\n\n" + how("To undo these changes, run the following command.\n⚠ Warning: no more protection.") + "\n" + how(T("fix.ps")) + "\n$ Move-Item a.bak a", "**Restore available:** backed up to:\n• a.bak"},
	} {
		if got := withoutCommands(c.details); got != c.want {
			t.Errorf("%s:\n%q\nwant:\n%q", c.name, got, c.want)
		}
	}
	// a marked line shows as any text; a text read from the PC cannot be one
	if got := plainText(how("**In PowerShell (Run as administrator):**") + "\n$ a"); got != "In PowerShell (Run as administrator):\n    a" {
		t.Errorf("shown: %q", got)
	}
	if got := how("a\n\nb"); got != "> a\n\n> b" {
		t.Errorf("how: %q", got)
	}
	if _, ok := howOf(plain("> x")); ok {
		t.Error("plain text reads as a line about commands")
	}
	// every fix that the details give is announced by a marked line, or by a sentence
	// that reads well without it: none of them leaves a command in a report
	// what eMudScan offers to do is for who runs it, not for who reads its report
	home := filepath.Join(t.TempDir(), "Example App 1", "python")
	ssl := pyLayout(t, home, pe.IMAGE_FILE_MACHINE_AMD64, "DLLs", "libssl-3-x64.dll", "libcrypto-3-x64.dll", "_hashlib.pyd")
	offer := pySSLAdvice(pythonEnvFor(filepath.Join(home, "python.exe")), ssl, "3.11.18", "Example App 1")
	if !strings.Contains(offer, "> "+Tf("pyssl.offer", "3.11.18", "python-build-standalone", "Example App 1")+"\n> In PowerShell:\n$ & {\n$     $ErrorActionPreference = 'Stop'\n") {
		t.Fatalf("the offer:\n%s", offer)
	}
	if got := withoutCommands(copyWarning("Outdated.\n\nRecommended action: update Example App 1.\n\n" + offer)); got != "Outdated.\n\nRecommended action: update Example App 1." {
		t.Errorf("the Python fix in a report:\n%s", got)
	}
	for _, text := range []string{replaceBundleFix(`C:\Example App 1\cacert.pem`), restoreAdvice(`C:\Example App 1\cacert.pem`), offer} {
		if got := withoutCommands(text); strings.Contains(got, "$ ") || strings.Contains(got, "PowerShell") || strings.Contains(strings.ToLower(got), "command") {
			t.Errorf("left in the report:\n%s", got)
		}
	}
}

// The reports keep what came of the fixes, not what ran.
func TestFixResults(t *testing.T) {
	Lang = 0
	lines := []fixLine{{lineNote, "2026-11-04 11:05 · Fix software"},
		{lineSoftware, "A"}, {lineComment, "Update with Scoop:"}, {lineCommand, "scoop update a"}, {lineOK, ""}, {lineOutput, "Updating a"}, {lineNote, ""},
		{lineSoftware, "B"}, {lineScript, "# The Python to fix"}, {lineCommand, "$a = 1"}, {lineOK, ""}, {lineScript, ""}, {lineScript, "try {"},
		{lineCommand, "    & $python -I $check $emudscanFiles after"}, {lineFailed, ""}, {lineOutput, "Worse than before: tls13"}, {lineCmdError, "Exit code 1"},
		{lineScript, "}"}, {lineScript, "catch {"}, {lineCommand, "    Move-Item a b"}, {lineOK, ""}, {lineCommand, "    throw 'put back'"}, {lineFailed, ""}, {lineCmdError, "put back"}, {lineScript, "}"}, {lineNote, ""},
		{lineSoftware, "C"}, {lineCommand, "never ran"}, {lineNote, ""},
		{lineNote, T("fix.checking")}, {lineFixed, Tf("fix.fixed", "A")}, {lineNotFixed, Tf("fix.notFixed", "B")}, {lineEnd, ""}}
	want := []string{"2026-11-04 11:05 · Fix software", "",
		Tf("fix.software", "B"), "  ✗     & $python -I $check $emudscanFiles after", "      Worse than before: tls13", "      Exit code 1", "  ✗     throw 'put back'", "      put back", "",
		T("fix.checking"), Tf("fix.fixed", "A"), Tf("fix.notFixed", "B"), "", fixBar}
	if got := reportLines(fixResults(lines)); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("results:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	// an error of the run stays
	if got := reportLines(fixResults([]fixLine{{lineSoftware, "A"}, {lineError, T("fix.adminRefused")}, {lineEnd, ""}})); strings.Join(got, "|") != T("fix.adminRefused")+"||"+fixBar {
		t.Errorf("an error: %q", got)
	}
}

// When an element is finished, the line that announced its commands says what to do
// with them: the button, by its name, or PowerShell; "this command", "these commands"
// or "this script"; the administrator rights, in red.
func TestGuide(t *testing.T) {
	Lang = 0
	one := newFix(fixAction{Kind: fixPip, Cmds: []string{"pip install --upgrade example-app-1"}})
	several := newFix(fixAction{Kind: fixBundle, Admin: true, Cmds: []string{"$a = 1", "Copy-Item a b"}})
	script := newFix(fixAction{Kind: fixPySSL, Script: true, Cmds: []string{"# run by the Fix window", "$a = 1"}, Alone: []string{"& {", "    $a = 2", "    $b = 3", "}"}})
	undo1 := newFix(fixAction{Kind: fixRestore, Cmds: []string{"Move-Item a.bak a"}, Files: []string{"a"}})
	undo2 := newFix(fixAction{Kind: fixRestore, Cmds: []string{"Move-Item a.bak a", "Move-Item b.bak b"}, Files: []string{"a", "b"}})
	ps, admin := how(T("fix.ps")), how(T("fix.psAdmin"))
	for _, c := range []struct {
		name, details string
		vulnerable    bool
		want          string
	}{
		{"a command", "Outdated.\n" + ps + "\n" + one.lines(), true, "Outdated.\n> To fix this vulnerability, click **Fix this software**, or run this command yourself in PowerShell:\n" + one.lines()},
		{"good to do", "Still good practice.\n" + ps + "\n" + one.lines(), false, "Still good practice.\n> To do so, click **Fix this software**, or run this command yourself in PowerShell:\n" + one.lines()},
		{"commands, as administrator", admin + "\n" + several.lines(), true, "> To fix this vulnerability, click **Fix this software**, or run these commands yourself in PowerShell !!(Run as administrator)!!:\n" + several.lines()},
		{"as administrator, by the fix", ps + "\n" + several.lines(), true, "> To fix this vulnerability, click **Fix this software**, or run these commands yourself in PowerShell !!(Run as administrator)!!:\n" + several.lines()},
		{"a script", ps + "\n" + script.lines(), true, "> To fix this vulnerability, click **Fix this software**, or run this script yourself in PowerShell:\n" + script.lines()},
		{"two fixes under one line", "Update uv, then this Python.\n" + ps + "\n" + how("Update with Scoop:") + "\n" + one.lines() + "\n" + how("Update with uv:") + "\n" + several.lines(), true,
			"Update uv, then this Python.\n> To fix this vulnerability, click **Fix this software**, or run these commands yourself in PowerShell:\n> Update with Scoop:\n" + one.lines() + "\n> Update with uv:\n" + several.lines()},
		{"a restore", "Fixed.\n\n" + ps + "\n" + undo1.lines(), false, "Fixed.\n\n> To undo the fix, click **Restore the backup**, or run this command yourself in PowerShell:\n" + undo1.lines()},
		{"a restore of two files", ps + "\n" + undo2.lines(), false, "> To undo the fix, click **Restore the backups**, or run these commands yourself in PowerShell:\n" + undo2.lines()},
		// the button then fixes: it does not restore
		{"a fix and a restore", ps + "\n" + one.lines() + "\n\n" + ps + "\n" + undo1.lines(), true,
			"> To fix this vulnerability, click **Fix this software**, or run this command yourself in PowerShell:\n" + one.lines() + "\n\n> To undo the fix, run this command yourself in PowerShell:\n" + undo1.lines()},
		// commands that the button does not run
		{"not a fix", "If the setting is no longer needed, remove it:\n" + admin + "\n$ git config --system --unset http.sslCAInfo", true,
			"If the setting is no longer needed, remove it:\n> Run this command in PowerShell !!(Run as administrator)!!:\n$ git config --system --unset http.sslCAInfo"},
		{"no line to replace", "Recommended action: update Flutter:\n$ flutter upgrade", true, "Recommended action: update Flutter:\n$ flutter upgrade"},
	} {
		if got := guide(c.details, c.vulnerable); got != c.want {
			t.Errorf("%s:\n%s\nwant:\n%s", c.name, got, c.want)
		}
		if len(fixesIn(guide(c.details, c.vulnerable))) != len(fixesIn(c.details)) {
			t.Errorf("%s: the fixes changed", c.name)
		}
	}
	// an advice that led to its commands ends there, before what to do with them
	if got := fixBlock("Recommended action: update it with uv, which installed it:", one); got != "Recommended action: update it with uv, which installed it.\n"+ps+"\n"+one.lines() {
		t.Errorf("fixBlock:\n%s", got)
	}
	// in red: its own run, which a text read from the PC cannot make
	runs := parseLine("run it yourself, in PowerShell !!(Run as administrator)!!:")
	if len(runs) != 3 || runs[1].kind != runAlert || runs[1].text != "(Run as administrator)" || runs[2].text != ":" {
		t.Errorf("runs: %+v", runs)
	}
	if got := plainText(how("run it yourself, in PowerShell !!(Run as administrator)!!:")); got != "run it yourself, in PowerShell (Run as administrator):" {
		t.Errorf("as text: %q", got)
	}
	if runs := parseLine(plain("a!!b!!c")); len(runs) != 1 || runs[0].kind != runText {
		t.Errorf("plain text in red: %+v", runs)
	}
	// in French too
	Lang = 1
	defer func() { Lang = 0 }()
	if got, want := guide(how(T("fix.psAdmin"))+"\n"+several.lines(), true), "> Pour corriger cette vulnérabilité, cliquez sur **Corriger ce logiciel**, ou exécutez vous-même ces commandes dans PowerShell !!(Exécuter en tant qu’administrateur)!!\u00a0:\n"+several.lines(); got != want {
		t.Errorf("in French:\n%s", got)
	}
}

// Above the commands of each fix, the console says which fix it is: "Fix:" when its
// software has only this one, else "Fix 1:", "Fix 2:"…, and, after an update, "if
// the update does not fix it".
func TestFixLabels(t *testing.T) {
	Lang = 0
	update := fixAction{Kind: fixPackage, Manager: "Scoop", Cmds: []string{"scoop update example-app-1"}}
	bundle := fixAction{Kind: fixBundle, Admin: true, Cmds: []string{"$a = 1"}}
	pip := fixAction{Kind: fixPip, Cmds: []string{"pip install --upgrade certifi"}}
	python := fixAction{Kind: fixPySSL, Script: true, Cmds: []string{"# x", "$b = 1"}}
	one := Item{ID: 1, Group: GroupApps, Name: "Example App 1", Program: "Example App 1", Fixes: []fixAction{python}}
	two := Item{ID: 2, Group: GroupApps, Name: "Example App 2", Program: "Example App 2", Fixes: []fixAction{update, bundle}}
	three := Item{ID: 3, Group: GroupApps, Name: "Example App 3", Program: "Example App 3", Fixes: []fixAction{bundle, pip}}
	got := fixLabels([]Item{one, two, three})
	for _, c := range []struct {
		id   int
		f    fixAction
		want string
	}{
		{1, python, "Fix: Replace the ssl files of Python, checked before and after"},
		{2, update, "Fix 1: Update with Scoop"},
		{2, bundle, "Fix 2, if the update does not fix it: Replace the certificate bundle with Mozilla’s current one (as administrator)"},
		{3, bundle, "Fix 1: Replace the certificate bundle with Mozilla’s current one (as administrator)"},
		{3, pip, "Fix 2: Update with pip"},
	} {
		if got[labelKey(c.id, c.f)] != c.want {
			t.Errorf("%d %v: %q, want %q", c.id, c.f.Kind, got[labelKey(c.id, c.f)], c.want)
		}
	}
	// the runner gives each fix its line
	r := &fixRunner{labels: got}
	if u := r.unit(two, bundle); u.label != got[labelKey(2, bundle)] || u.names[0] != itemTitle(two) {
		t.Errorf("unit: %+v", u)
	}
	// the line between two software is a comment: copied and run, PowerShell skips it
	if !strings.HasPrefix(scriptRule, "# ") || strings.Trim(scriptRule[2:], "─") != "" || len([]rune(scriptRule)) != 78 {
		t.Errorf("rule: %q", scriptRule)
	}
	Lang = 1
	defer func() { Lang = 0 }()
	if got := fixLabels([]Item{two})[labelKey(2, bundle)]; got != "Correction 2, si la mise à jour ne le corrige pas : Remplacement du fichier de certificats par celui, à jour, de Mozilla (en tant qu’administrateur)" {
		t.Errorf("in French: %q", got)
	}
}
