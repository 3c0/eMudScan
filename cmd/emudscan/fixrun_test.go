package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
	"unicode/utf16"
)

// The script runs each command between markers in the log, in the order of the
// fixes; the commands without backups when "Back up the files to change" is cleared. It
// shows errors by their message. The
// command that starts it holds its SHA-256, and PowerShell receives it in UTF-16.
func TestFixScript(t *testing.T) {
	Lang = 0
	bundle := fixAction{Kind: fixBundle, Cmds: []string{"$a = 1", "if ($a) { backup; write }"}, Plain: []string{"$a = 1", "if ($a) { write }"}}
	pkg := fixAction{Kind: fixPackage, Cmds: []string{"scoop update app"}}
	for _, backup := range []bool{true, false} {
		if fixSimulation {
			break // the simulation replaces the commands: see TestFixSimulation
		}
		r := fixRunner{backup: backup}
		units := []fixUnit{r.unit(Item{ID: 1, Name: "App"}, pkg), r.unit(Item{ID: 2, Name: "Tool"}, bundle)}
		s := fixScript(units, `C:\Users\me\AppData\Local\Temp\x’y\fix.log`, "##emudscan-0123")
		want := "if ($a) { backup; write }"
		if !backup {
			want = "if ($a) { write }"
		}
		for _, w := range []string{"##emudscan-0123 cmd 0 0", "\nscoop update app\n", "##emudscan-0123 cmd 1 1", "\n" + want + "\n", "##emudscan-0123 end 1 1 ",
			psQuote(`C:\Users\me\AppData\Local\Temp\x’y\fix.log`), "ErrorRecord]) { $_.ToString() }", "catch { $emudscanOut = $_.ToString() }"} {
			if !strings.Contains(s, w) {
				t.Errorf("backup %v: no %q in\n%s", backup, w, s)
			}
		}
		if strings.Index(s, "cmd 0 0") > strings.Index(s, "cmd 1 0") || strings.Contains(s, "cmd 2 0") {
			t.Errorf("order:\n%s", s)
		}
	}
	script := []byte("Write-Output 1\n")
	boot := fixBootstrap(`C:\Temp\fix.ps1`, script)
	sum := sha256.Sum256(script)
	if !strings.Contains(boot, strings.ToUpper(hex.EncodeToString(sum[:]))) || !strings.Contains(boot, `'C:\Temp\fix.ps1'`) || !strings.Contains(boot, fmt.Sprint("exit ", fixTampered)) {
		t.Errorf("bootstrap:\n%s", boot)
	}
	raw, err := base64.StdEncoding.DecodeString(encodedCommand("é‘x"))
	if err != nil || len(raw) != 6 {
		t.Fatalf("encoded: %v %d", err, len(raw))
	}
	u := make([]uint16, 3)
	for i := range u {
		u[i] = binary.LittleEndian.Uint16(raw[2*i:])
	}
	if string(utf16.Decode(u)) != "é‘x" {
		t.Errorf("decoded %q", string(utf16.Decode(u)))
	}
}

// The console shows each command, its result, then what it wrote (which the script
// writes once the command has ended), as the log grows, even when a line arrives in
// pieces. Only the markers of this run count: an output
// that looks like one is shown as it is. As administrator, where the commands of
// several software run together, each software is named before its commands (all
// those of a fix that they share), after a blank line below those of the one before.
func TestFixLog(t *testing.T) {
	Lang = 0
	units := []fixUnit{{ids: []int{1}, titles: []string{"A"}, cmds: []string{"$a = 1", "write"}}, {ids: []int{2, 4}, titles: []string{"B", "B › D"}, cmds: []string{"scoop update app"}},
		{ids: []int{3}, titles: []string{"C"}, cmds: []string{"never"}}}
	log := "##m cmd 0 0\n\n##m end 0 0 0 0\n##m cmd 0 1\nwritten\n##m end 0 1 0 0\n" +
		"##m cmd 1 0\nUpdating app\n##other cmd 2 0\n##m end 1 0 1 2\n##m cmd 9 0\n##m end 1"
	for _, titles := range []bool{false, true} {
		var console []fixLine
		l := newFixLog(units, "##m", func(s fixLine) { console = append(console, s) }, titles)
		for i := 0; i < len(log); i += 7 {
			l.write(log[i:min(i+7, len(log))])
		}
		l.close()
		done, failed := "✓ Done", "✗ Failed (exit code 2)"
		if fixSimulation {
			done, failed = "✓ Done (simulated)", "✗ Failed (simulated)"
		}
		want := []fixLine{{lineCommand, "$a = 1"}, {lineOK, done}, {lineCommand, "write"}, {lineOK, done}, {lineOutput, "written"},
			{lineCommand, "scoop update app"}, {lineFailed, failed}, {lineOutput, "Updating app"}, {lineOutput, "##other cmd 2 0"}}
		if titles {
			want = append(append([]fixLine{{lineSoftware, "A"}}, want[:5]...), append([]fixLine{{lineNote, ""}, {lineSoftware, "B"}, {lineSoftware, "B › D"}}, want[5:]...)...)
		}
		if fmt.Sprint(console) != fmt.Sprint(want) {
			t.Errorf("titles %v, console:\n%s", titles, strings.Join(plainLines(console), "\n"))
		}
		if r := l.results; !r[0].ran || !r[0].ok || !r[1].ran || r[1].ok || r[2].ran {
			t.Errorf("results: %+v", r)
		}
	}
}

// The runner updates first, checks again, then fixes the files of the software not
// fixed yet; the commands that need administrator rights run together, after the
// others of their round. A fixed software is reported, for the window to unselect it.
// Stop takes effect before the next software.
func TestFixRunner(t *testing.T) {
	Lang = 0
	pkg := func(c string, admin bool) fixAction {
		return fixAction{Kind: fixPackage, Admin: admin, Cmds: []string{c}}
	}
	file := func(c string, admin bool) fixAction {
		return fixAction{Kind: fixBundle, Admin: admin, Cmds: []string{c + " with backup"}, Plain: []string{c}}
	}
	type state struct {
		it        Item
		fixedWhen string // the command after which a check finds it fixed
	}
	setup := func() map[int]*state {
		return map[int]*state{
			1: {Item{ID: 1, Name: "A", Status: StWarn, Fixes: []fixAction{pkg("scoop update a", false), file("replace a", true)}}, "replace a"},
			2: {Item{ID: 2, Name: "B", Status: StWarn, Fixes: []fixAction{pkg("winget upgrade b", false), file("replace b", false)}}, "winget upgrade b"},
			3: {Item{ID: 3, Name: "C", Status: StWarn, Fixes: []fixAction{file("pip c", true)}}, "pip c"},
			4: {Item{ID: 4, Name: "D", Status: StWarn, Fixes: []fixAction{pkg("choco upgrade d -y", true)}}, "never"},
			5: {Item{ID: 5, Name: "E", Status: StOK, Fixes: []fixAction{pkg("scoop update e", false)}}, ""},
		}
	}
	run := func(items map[int]*state, backup bool, stopAfter int, ids ...int) (calls []string, fixed []int, console []string, stopped bool) {
		if ids == nil {
			ids = []int{1, 2, 3, 4, 5}
		}
		var lines []fixLine
		defer func() { console = plainLines(lines) }()
		ranCmds := map[string]bool{}
		execs := 0
		r := fixRunner{backup: backup,
			exec: func(units []fixUnit, admin bool, out func(fixLine)) ([]unitResult, error) {
				execs++
				var cmds []string
				res := make([]unitResult, len(units))
				for i, u := range units {
					cmds = append(cmds, u.cmds...)
					for _, c := range u.cmds {
						ranCmds[strings.TrimSuffix(c, " with backup")] = true
					}
					res[i] = unitResult{ran: true, ok: true}
				}
				calls = append(calls, fmt.Sprintf("%v:%s", admin, strings.Join(cmds, ",")))
				return res, nil
			},
			item: func(id int) (Item, bool) {
				s, ok := items[id]
				if !ok {
					return Item{}, false
				}
				return s.it, true
			},
			recheck: func(ids []int) {
				calls = append(calls, fmt.Sprint("recheck", ids))
				for _, id := range ids {
					if s := items[id]; ranCmds[s.fixedWhen] {
						s.it.Status = StOK
					}
				}
			},
			stopped: func() bool { return stopAfter > 0 && execs >= stopAfter },
			out:     func(s fixLine) { lines = append(lines, s) },
			fixed:   func(id int) { fixed = append(fixed, id) },
		}
		stopped = r.run(ids)
		return
	}
	calls, fixed, console, stopped := run(setup(), true, 0)
	want := []string{
		"false:scoop update a", "false:winget upgrade b", "true:choco upgrade d -y", "recheck[1 2 4]",
		"true:replace a with backup,pip c with backup", "recheck[1 3]",
	}
	if strings.Join(calls, " | ") != strings.Join(want, " | ") || stopped {
		t.Errorf("calls:\n%s", strings.Join(calls, "\n"))
	}
	if fmt.Sprint(fixed) != "[2 1 3]" {
		t.Errorf("fixed %v", fixed)
	}
	text := strings.Join(console, "\n")
	for _, w := range []string{Tf("fix.fixed", "B"), Tf("fix.notFixed", "D"), T("fix.adminPrompt"), Tf("fix.software", "A"), T("fix.restart")} {
		if !strings.Contains(text, w) {
			t.Errorf("console: no %q in\n%s", w, text)
		}
	}
	if console[len(console)-1] != T("fix.restart") {
		t.Errorf("console: the last line is not the reminder to restart:\n%s", text)
	}
	// a blank line after the commands of each software, and after those run as
	// administrator
	if !strings.Contains(text, Tf("fix.software", "A")+"\n\n"+Tf("fix.software", "B")+"\n\n"+T("fix.adminPrompt")+"\n\n"+T("fix.checking")) {
		t.Errorf("console: no blank lines between the software:\n%s", text)
	}
	// nothing fixed: no reminder to restart
	if _, _, console, _ := run(setup(), true, 0, 4); strings.Contains(strings.Join(console, "\n"), T("fix.restart")) {
		t.Errorf("nothing fixed: console:\n%s", strings.Join(console, "\n"))
	}
	// an update that two elements share runs once, with both named; each is checked
	// again and, here, fixed by it
	shared := setup()
	shared[6] = &state{Item{ID: 6, Name: "A2", Status: StWarn, Fixes: []fixAction{pkg("scoop update a", false), file("replace a2", false)}}, "scoop update a"}
	shared[4].it.Fixes = []fixAction{pkg("choco upgrade d -y", true), pkg("choco upgrade d2 -y", true)}
	shared[7] = &state{Item{ID: 7, Name: "D2", Status: StWarn, Fixes: []fixAction{pkg("choco upgrade d2 -y", true)}}, "choco upgrade d2 -y"}
	calls, fixed, console, _ = run(shared, true, 0, 1, 6, 2, 4, 7)
	if strings.Count(strings.Join(calls, ","), "scoop update a") != 1 || calls[2] != "true:choco upgrade d -y,choco upgrade d2 -y" || calls[3] != "recheck[1 6 2 4 7]" {
		t.Errorf("shared update: calls:\n%s", strings.Join(calls, "\n"))
	}
	if text := strings.Join(console, "\n"); !strings.Contains(text, Tf("fix.software", "A")+"\n"+Tf("fix.software", "A2")+"\n") || !strings.Contains(text, Tf("fix.fixed", "A2")) {
		t.Errorf("shared update: console:\n%s", text)
	}
	if fmt.Sprint(fixed) != "[6 2 7 1 3]" && fmt.Sprint(fixed) != "[6 2 7 1]" {
		t.Errorf("shared update: fixed %v", fixed)
	}
	// Stop after the shared update: the element that has other fixes in the same round
	// is not counted as fixed by it (the simulation would show it fixed)
	stopShared := setup()
	stopShared[6] = &state{Item{ID: 6, Name: "A2", Status: StWarn, Fixes: []fixAction{pkg("scoop update a", false), pkg("winget upgrade a2", false)}}, ""}
	calls, _, console, stopped = run(stopShared, true, 1, 1, 6)
	if !stopped || strings.Join(calls, " | ") != "false:scoop update a | recheck[1 6]" {
		t.Errorf("stop after a shared update: %v %v", stopped, calls)
	}
	// without backups
	if calls, _, _, _ := run(setup(), false, 0); calls[4] != "true:replace a,pip c" {
		t.Errorf("without backups: %v", calls)
	}
	// Stop after the first software: the others are not run, the first is checked again,
	// after a line in red below its commands
	calls, _, console, stopped = run(setup(), true, 1)
	if !stopped || strings.Join(calls, " | ") != "false:scoop update a | recheck[1]" ||
		!strings.Contains(strings.Join(console, "\n"), Tf("fix.software", "A")+"\n\n"+T("fix.interrupted")+"\n\n"+T("fix.checking")) {
		t.Errorf("stop: %v %v\n%s", stopped, calls, strings.Join(console, "\n"))
	}
}

// The Restore button of the details runs the restores of a software, and nothing
// else; the fixes never run them. A software is restored when its commands all
// succeeded and its check no longer proposes the backups they put back, though it may
// propose an older one.
func TestFixRestore(t *testing.T) {
	Lang = 0
	restore := func(c string, files ...string) fixAction {
		return fixAction{Kind: fixRestore, Cmds: []string{c}, Files: files}
	}
	items := map[int]*Item{
		1: {ID: 1, Name: "A", Status: StOK, Restores: []fixAction{restore("move a", "a")}},
		2: {ID: 2, Name: "B", Status: StWarn, Fixes: []fixAction{{Kind: fixBundle, Cmds: []string{"replace b"}}}, Restores: []fixAction{restore("move b", "b")}},
		3: {ID: 3, Name: "C", Status: StOK, Restores: []fixAction{restore("move c", "c")}},
		4: {ID: 4, Name: "D", Status: StOK, Restores: []fixAction{restore("move d", "d")}},
		5: {ID: 5, Name: "E", Status: StOK},
	}
	var calls []string
	var lines []fixLine
	r := fixRunner{
		exec: func(units []fixUnit, admin bool, out func(fixLine)) ([]unitResult, error) {
			res := make([]unitResult, len(units))
			for i, u := range units {
				calls = append(calls, strings.Join(u.cmds, ","))
				res[i] = unitResult{ran: true, ok: u.cmds[0] != "move d"} // D's command fails
			}
			return res, nil
		},
		item: func(id int) (Item, bool) {
			if it, ok := items[id]; ok {
				return *it, true
			}
			return Item{}, false
		},
		recheck: func(ids []int) {
			calls = append(calls, fmt.Sprint("recheck", ids))
			items[1].Restores, items[1].Status = nil, StWarn              // put back: nothing left to restore
			items[2].Restores = []fixAction{restore("move b-older", "b")} // an older backup is proposed now
			items[3].Restores = []fixAction{restore("move c", "c")}       // still proposed: not restored
			items[4].Restores = nil                                       // its command failed
		},
		stopped: func() bool { return false },
		out:     func(l fixLine) { lines = append(lines, l) },
		fixed:   func(id int) { t.Errorf("restore: %d reported as fixed", id) },
	}
	r.restore([]int{1, 2, 3, 4, 5})
	if got := strings.Join(calls, " | "); got != "move a | move b | move c | move d | recheck[1 2 3 4]" {
		t.Errorf("calls: %s", got)
	}
	text := strings.Join(plainLines(lines), "\n")
	for _, w := range []string{Tf("restore.done", "A"), Tf("restore.done", "B"), Tf("restore.notDone", "C"), Tf("restore.notDone", "D"), T("restore.restart")} {
		if !strings.Contains(text, w) {
			t.Errorf("console: no %q in\n%s", w, text)
		}
	}
	if strings.Contains(text, "replace b") || strings.Contains(text, T("fix.restart")) {
		t.Errorf("console:\n%s", text)
	}
	// the fixes leave the restores out
	calls = nil
	r = fixRunner{exec: r.exec, item: r.item, recheck: func([]int) {}, stopped: r.stopped, out: func(fixLine) {}, fixed: func(int) {}, backup: true}
	items[2].Status = StWarn
	r.run([]int{1, 2})
	if got := strings.Join(calls, " | "); got != "replace b" {
		t.Errorf("fix: calls %s", got)
	}
	// the message at the end: whether each is restored, why not, and to restart
	msg, ok := restoreOutcome(lines)
	if ok || !strings.HasPrefix(msg, Tf("restore.done", "A")+"\n"+Tf("restore.done", "B")+"\n"+Tf("restore.notDone", "C")) || !strings.HasSuffix(msg, "\n\n"+T("restore.restart")) {
		t.Errorf("message (%v):\n%s", ok, msg)
	}
}

// The message at the end of a restore says whether it worked and, if not, why: what
// went wrong, and what the failed commands showed, not what the others did.
func TestRestoreOutcome(t *testing.T) {
	Lang = 0
	done := []fixLine{{lineSoftware, "A"}, {lineCommand, "move a"}, {lineOK, "✓ Done"}, {lineOutput, "moved"}, {lineNote, ""},
		{lineNote, T("fix.checking")}, {lineFixed, Tf("restore.done", "A")}, {lineNote, T("restore.restart")}}
	if msg, ok := restoreOutcome(done); !ok || msg != Tf("restore.done", "A")+"\n\n"+T("restore.restart") {
		t.Errorf("done (%v):\n%s", ok, msg)
	}
	failed := []fixLine{{lineSoftware, "A"}, {lineCommand, "move a"}, {lineOK, "✓ Done"}, {lineOutput, "moved"},
		{lineCommand, "move b"}, {lineFailed, "✗ Failed"}, {lineOutput, "Access to the path is denied."}, {lineNote, ""},
		{lineNote, T("fix.checking")}, {lineNotFixed, Tf("restore.notDone", "A")}}
	if msg, ok := restoreOutcome(failed); ok || msg != Tf("restore.notDone", "A")+"\n\nAccess to the path is denied." {
		t.Errorf("failed (%v):\n%s", ok, msg)
	}
	refused := []fixLine{{lineNote, T("fix.adminPrompt")}, {lineError, T("fix.adminRefused")}, {lineNote, ""},
		{lineNote, T("fix.checking")}, {lineNotFixed, Tf("restore.notDone", "A")}}
	if msg, ok := restoreOutcome(refused); ok || msg != Tf("restore.notDone", "A")+"\n\n"+T("fix.adminRefused") {
		t.Errorf("refused (%v):\n%s", ok, msg)
	}
}

// Before the fixes run, the window shows each fix's commands as they will run, after
// a line that says what they do; the fixes of the files come after the update, for
// when it does not fix the software.
func TestFixPreview(t *testing.T) {
	Lang = 0
	it := Item{ID: 1, Name: "App", Program: "App", Fixes: []fixAction{
		{Kind: fixPackage, Manager: "Scoop", Cmds: []string{"scoop update app"}},
		{Kind: fixBundle, Admin: true, Cmds: []string{"$a = 1", "backup; write"}, Plain: []string{"$a = 1", "write"}},
	}}
	none := Item{ID: 2, Name: "Other"}
	for _, backup := range []bool{true, false} {
		last := "backup; write"
		if !backup {
			last = "write"
		}
		want := []fixLine{{lineSoftware, itemTitle(it)}, {lineComment, "Update with Scoop:"}, {lineCommand, "scoop update app"}, {lineComment, T("fix.ifStillNeeded")},
			{lineComment, "Replace the certificate bundle with Mozilla’s current one (as administrator):"}, {lineCommand, "$a = 1"}, {lineCommand, last}, {lineNote, ""}}
		if got := fixPreview([]Item{it}, backup); fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("backup %v:\n%s", backup, strings.Join(plainLines(got), "\n"))
		}
	}
	// its file, even when it is the only one
	one := it
	one.File = `C:\App\app.exe`
	if got := fixPreview([]Item{one}, true); len(got) < 2 || got[1] != (fixLine{lineComment, one.File}) {
		t.Errorf("one:\n%s", strings.Join(plainLines(got), "\n"))
	}
	// several copies: each one's file is named below its title; an element without
	// fixes is left out
	copy := it
	copy.ID, copy.File, copy.Fixes = 3, `C:\Other\app.exe`, it.Fixes[1:]
	if got := fixPreview([]Item{it, none, copy}, true); len(got) != 14 || got[8] != (fixLine{lineSoftware, itemTitle(copy)}) || got[9] != (fixLine{lineComment, copy.File}) {
		t.Errorf("copies:\n%s", strings.Join(plainLines(got), "\n"))
	}
	// two problems of a software that one update may fix: shown together, with the
	// update once, then the fix of the file of the second, for when the update does not
	// fix it; another software in between comes after them
	exe := Item{ID: 4, Group: GroupApps, Name: "tool", Program: "tool", File: `C:\t\tool.exe`, Fixes: it.Fixes[:1]}
	bundle := Item{ID: 5, Group: GroupApps, Name: "tool", Program: "tool", Component: "Certificate bundle", File: `C:\t\cacert.pem`, Fixes: it.Fixes}
	other := Item{ID: 6, Group: GroupApps, Name: "Other", Program: "Other", File: `C:\o\cacert.pem`, Fixes: it.Fixes[1:]}
	got := strings.Join(plainLines(fixPreview([]Item{exe, other, bundle}, true)), "\n")
	want := `— tool —
# C:\t\tool.exe
— tool › Certificate bundle —
# C:\t\cacert.pem
# Update with Scoop:
  scoop update app
# Then, if the update does not fix tool › Certificate bundle:
# Replace the certificate bundle with Mozilla’s current one (as administrator):
  $a = 1
  backup; write

— Other —
# C:\o\cacert.pem
# Replace the certificate bundle with Mozilla’s current one (as administrator):
  $a = 1
  backup; write
`
	if got != want {
		t.Errorf("shared update:\n%s", got)
	}
}

// Copies of a software, which have the same title, are named in the results of the
// fixes with their folder, or their file when they share it; the preview shows their
// file below their title instead.
func TestFixTitles(t *testing.T) {
	Lang = 0
	py := func(id int, folder, file string) Item {
		return Item{ID: id, Group: GroupApps, Name: "Python", Program: "Builder", Component: "Python", Version: "3.14.7", Folder: folder, File: file,
			Fixes: []fixAction{{Kind: fixPip, Cmds: []string{"& '" + folder + `\python.exe' -m pip install --upgrade pip`}}}}
	}
	items := []Item{py(1, `C:\a\python`, `C:\a\python\cacert.pem`), py(2, `C:\b\python`, `C:\b\python\cacert.pem`),
		py(3, `C:\c`, `C:\c\1.pem`), py(4, `C:\c`, `C:\c\2.pem`), {ID: 5, Group: GroupApps, Name: "Other", Program: "Other"}}
	got := fixTitles(items)
	want := map[int]string{1: `Builder › Python 3.14.7 in C:\a\python`, 2: `Builder › Python 3.14.7 in C:\b\python`,
		3: `Builder › Python 3.14.7 in C:\c\1.pem`, 4: `Builder › Python 3.14.7 in C:\c\2.pem`, 5: "Other"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("titles: %v", got)
	}
	// the preview names the file of each below its title, which needs no place then
	if lines := fixPreview(items[:2], true); len(lines) < 2 || lines[0].text != itemTitle(items[0]) || lines[1].text != `C:\a\python\cacert.pem` {
		t.Errorf("preview: %v", lines)
	}
	if lines := fixPreview(items[2:4], true); len(lines) < 2 || lines[0].text != itemTitle(items[2]) || lines[1].text != `C:\c\1.pem` {
		t.Errorf("preview: %v", lines)
	}
}

// The report shows what the fixes ran and showed, under their own title, before the
// log: the commands under their software, each with its result at the end of its
// line, what they showed further in.
func TestFixReport(t *testing.T) {
	Lang = 0
	c := newChecker(nopReporter{}, nil)
	if rep := buildReport(c, ""); strings.Contains(rep, "\r\nFixes\r\n") {
		t.Errorf("a report without fixes has a Fixes section:\n%s", rep)
	}
	for _, l := range []fixLine{{lineNote, "2026-11-04 11:05 · Fix A"}, {lineSoftware, "A"}, {lineCommand, "scoop update a"}, {lineOK, "✓ Done"},
		{lineOutput, "Updating a"}, {lineCommand, "scoop update b"}, {lineFailed, "✗ Failed"}, {lineNote, T("fix.checking")}, {lineFixed, Tf("fix.fixed", "A")}, {lineNote, T("fix.end")}} {
		c.recordFix(l)
	}
	rep := strings.ReplaceAll(buildReport(c, "log"), "\r\n", "\n")
	want := "\nFixes\n-----\n2026-11-04 11:05 · Fix A\n— A —\n  scoop update a  ✓ Done\n      Updating a\n  scoop update b  ✗ Failed\nChecking again…\nA: fixed.\nEnd of the fixes.\n\nLog\n"
	if !strings.Contains(rep, want) {
		t.Errorf("report without\n%s\nin\n%s", want, rep)
	}
}
