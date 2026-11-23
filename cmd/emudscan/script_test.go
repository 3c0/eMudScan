package main

import (
	"fmt"
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
	units := []fixUnit{{ids: []int{1}, titles: []string{"A"}, fix: script, cmds: script.Cmds}, {ids: []int{2}, titles: []string{"B"}, fix: other, cmds: other.Cmds}}
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
	want := []string{"    # what it does", "  ✓ $a = 1", "", "    try {", "  ✓     copy", "  ✓     test", "    }", "    catch {", "        put back", "        throw 'no'", "    }", "", "    # the end", "  ✓ clean",
		"  ✓ # not a comment here", "  ✓ $b = 2"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") || !res[0].ok || !res[1].ok {
		t.Errorf("all well: %+v\n%s", res, strings.Join(got, "\n"))
	}
	// the test fails: the "catch" ran, and the fix stopped there; the next fix ran
	got, res = run(ok(1)+ok(4)+"##m cmd 0 5\nno ssl\n##m end 0 5 1 1\n"+ok(8)+"##m cmd 0 9\n##m! no\n##m end 0 9 1 0\n"+"##m cmd 1 0\n##m end 1 0 0 0\n", true)
	want = []string{Tf("fix.software", "A"), "    # what it does", "  ✓ $a = 1", "", "    try {", "  ✓     copy", "  ✗     test", "      no ssl", "      Exit code 1", "    }", "    catch {", "  ✓     put back", "  ✗     throw 'no'", "      no", "    }", "", "    # the end", "    clean",
		"", Tf("fix.software", "B"), "  ✓ # not a comment here"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") || !res[0].ran || res[0].ok || !res[1].ok {
		t.Errorf("the test fails: %+v\n%s", res, strings.Join(got, "\n"))
	}
	// it stops at its first command: the rest has no result
	got, res = run("##m cmd 0 1\n##m! broken\n##m end 0 1 1 0\n", false)
	if strings.Join(got, "\n") != "    # what it does\n  ✗ $a = 1\n      broken\n\n    try {\n        copy\n        test\n    }\n    catch {\n        put back\n        throw 'no'\n    }\n\n    # the end\n    clean" || res[0].ok || res[1].ran {
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
		if l.text != `$check  = "$emudscanFiles\check_ssl.py"` || n != 12 || tail != 1 {
			t.Errorf("%q: %d, %d", l.text, n, tail)
		}
	}
	if found != 1 {
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
