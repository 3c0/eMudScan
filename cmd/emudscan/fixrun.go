package main

// The Fix function runs the fixes of the software the user selects (fixes.go), in
// Windows PowerShell, then checks each software again. The Restore button of the
// details runs in the same way the restore of a software's backups, without a window;
// eMudScan then moves the files itself when they need no administrator rights
// (fixRunner.restore, moveBackups).
//
// It works in two rounds. First the updates through the package managers that
// installed the software, which may bring the fix; then, for the software still not
// fixed, the fixes of its files. After each round, the software that ran something is
// checked again: it counts as fixed only when its check finds nothing left to fix.
// Stop takes effect before the next software, so that none is left half fixed. A
// connection that fails stops the run too, as the fixes that follow would most likely
// fail as well: a failed download, before its fix changed anything, or a failed update
// whose package manager's servers cannot be reached (fixScript).
//
// The commands of one software run in one PowerShell, as they would when pasted
// together. Those that need administrator rights run, for all the software of a
// round, in one Windows PowerShell started as administrator: one Windows prompt,
// which names Microsoft as the publisher, and eMudScan itself never runs as
// administrator.
//
// PowerShell reads the commands from a script that eMudScan writes in the user's
// Temp folder; the command line that starts PowerShell, which the Windows prompt
// approves, holds the SHA-256 of that script, and PowerShell runs it only if it has
// not changed since (fixBootstrap). Each command writes what it shows into a log, between
// markers that the runner reads as the commands run (fixLog).

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
)

// fixUnit is one fix, as the runner runs it: once for all the elements that have it,
// such as the update of a software for two of its problems.
type fixUnit struct {
	ids    []int    // the elements
	titles []string // their titles, for the console
	fix    fixAction
	cmds   []string // what runs: the commands, with or without the backups
}

// fixLineKind is what a line of the console of the Fix window is. The console shows
// the commands as plain PowerShell, with what they do as comments, then their results.
type fixLineKind int

const (
	lineNote     fixLineKind = iota // a message of eMudScan
	lineSoftware                    // the software whose commands follow
	lineComment                     // what the commands that follow do
	lineCommand                     // a command
	lineOutput                      // what a command showed
	lineOK                          // a command succeeded
	lineFailed                      // a command failed
	lineError                       // the commands could not run
	lineFixed                       // a software is fixed
	lineNotFixed                    // a software is still not fixed
	lineCmdError                    // an error of a command, or its exit code
	lineWarning                     // a warning of a command
)

// fixLine is a line of the console.
type fixLine struct {
	kind fixLineKind
	text string
}

// The results of a command, in the left column of the console, and before it in the
// report.
const (
	markOK     = "✓"
	markFailed = "✗"
)

// plain is the line as text, for the report: the commands indented under their
// software, what the commands showed further.
func (l fixLine) plain() string {
	switch l.kind {
	case lineSoftware:
		return Tf("fix.software", l.text)
	case lineComment:
		return "# " + l.text
	case lineCommand:
		return "    " + l.text
	case lineOK, lineFailed, lineOutput, lineCmdError, lineWarning:
		return "      " + l.text
	}
	return l.text
}

// reportLines is lines as text, for the report: the result of a command before it,
// as in the console ("  ✓ scoop update app"), then what it showed.
func reportLines(lines []fixLine) []string {
	var out []string
	for i, l := range lines {
		if l.kind == lineCommand && i+1 < len(lines) && (lines[i+1].kind == lineOK || lines[i+1].kind == lineFailed) {
			mark := markOK
			if lines[i+1].kind == lineFailed {
				mark = markFailed
			}
			out = append(out, "  "+mark+" "+l.text)
			continue
		}
		if (l.kind == lineOK || l.kind == lineFailed) && l.text == "" {
			continue // in the mark before the command
		}
		out = append(out, strings.TrimRight(l.plain(), " "))
	}
	return out
}

// plainLines is lines as text.
func plainLines(lines []fixLine) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = l.plain()
	}
	return out
}

// unitResult is what came of a fix: whether its commands ran, and all succeeded; halted:
// the run stopped after it, as a connection failed (fixScript).
type unitResult struct{ ran, ok, halted bool }

// fixExecutor runs fixes in Windows PowerShell: all of them in one PowerShell
// started as administrator when admin is true (one Windows prompt), else in one
// PowerShell. It writes the console with out as the commands run, and returns a
// result for each fix, or an error when they could not run at all.
type fixExecutor func(units []fixUnit, admin bool, out func(fixLine)) ([]unitResult, error)

// fixRunner runs the fixes of elements; the window that shows it gives its functions.
type fixRunner struct {
	exec      fixExecutor
	item      func(id int) (Item, bool) // an element as it is now
	recheck   func(ids []int)           // checks elements again (checker.recheck)
	stopped   func() bool               // Stop was clicked
	out       func(fixLine)             // the console
	fixed     func(id int)              // an element is fixed: the window unselects it
	simulated func(id int)              // simulation only: shows an element as fixed, or restored
	backup    bool                      // "Back up the files to change"
	pick      fixPick                   // the fixes of each element that run (the ticked rows); nil: all
	titles    map[int]string            // the titles of the elements, for the console (fixTitles)
	fixedAny  bool                      // a software is fixed, or restored: it is to be restarted if it was open
	restoring bool                      // it restores backups (restore), instead of fixing
	restores  map[int][]string          // for a restore: the restores of each element, by their commands (fixAction.key)
}

// anyRan: at least one fix ran.
func anyRan(res []unitResult) bool {
	for _, r := range res {
		if r.ran {
			return true
		}
	}
	return false
}

// fixedStatus: a status that leaves nothing to fix.
func fixedStatus(st Status) bool { return st == StOK || st == StInfo }

// run fixes the elements ids, with the fixes that r.pick runs. It reports whether
// Stop ended it. Once a software is fixed, it ends by saying to restart those that
// were open.
func (r *fixRunner) run(ids []int) (stopped bool) {
	var items []Item // those to fix, as the Fix window names them
	for _, id := range ids {
		if it, ok := r.item(id); ok && len(it.Fixes) > 0 && !fixedStatus(it.Status) {
			items = append(items, it)
		}
	}
	r.titles = fixTitles(r.pick.items(items))
	for _, update := range []bool{true, false} {
		ran, ok, stop := r.round(ids, func(it Item) []fixAction {
			if fixedStatus(it.Status) {
				return nil
			}
			var out []fixAction
			for _, f := range it.Fixes {
				if (f.Kind == fixPackage) == update && r.pick.runs(it.ID, f) {
					out = append(out, f)
				}
			}
			return out
		})
		r.checkAgain(ran, ok)
		if stop {
			stopped = true
			break
		}
	}
	if r.fixedAny {
		r.out(fixLine{lineNote, T("fix.restart")})
	}
	return stopped
}

// restore puts back the backups of the elements ids (Item.Restores), in one round,
// then checks them again. It reports whether Stop ended it. Once a software is
// restored, it ends by saying to restart it if it was open.
func (r *fixRunner) restore(ids []int) (stopped bool) {
	r.restoring = true
	r.restores = map[int][]string{}
	var items []Item
	for _, id := range ids {
		if it, ok := r.item(id); ok && len(it.Restores) > 0 {
			items = append(items, it)
			for _, f := range it.Restores {
				r.restores[id] = append(r.restores[id], f.key())
			}
		}
	}
	r.titles = fixTitles(items)
	ran, ok, stopped := r.round(ids, func(it Item) []fixAction { return it.Restores })
	r.checkAgain(ran, ok)
	if r.fixedAny {
		r.out(fixLine{lineNote, T("restore.restart")})
	}
	return stopped
}

// restored: none of the restores that ran for it are still proposed: the check
// found no backup left to put back, or another one, older.
func (r *fixRunner) restored(it Item) bool {
	for _, k := range r.restores[it.ID] {
		for _, f := range it.Restores {
			if f.key() == k {
				return false
			}
		}
	}
	return true
}

// restoreOutcome is what the message at the end of a restore says, from what the
// runner reported (lines): for each software, whether it is restored; then why not,
// from the errors and what the failed commands showed; and, once one is restored, to
// restart it. ok: all are restored.
func restoreOutcome(lines []fixLine) (msg string, ok bool) {
	var results, why []string
	ok, restored, failed := true, false, false
	for _, l := range lines {
		switch l.kind {
		case lineFixed:
			results = append(results, l.text)
			restored = true
		case lineNotFixed:
			results = append(results, l.text)
			ok = false
		case lineCommand:
			failed = false
		case lineFailed:
			failed = true
		case lineOutput, lineCmdError, lineWarning:
			if failed {
				why = append(why, l.text)
			}
		case lineError:
			why = append(why, l.text)
		}
	}
	if len(why) > 8 { // the report has all of it
		why = append(why[:8], "…")
	}
	msg = strings.Join(results, "\n")
	if !ok && len(why) > 0 {
		msg += "\n\n" + strings.Join(why, "\n")
	}
	if restored {
		msg += "\n\n" + T("restore.restart")
	}
	return strings.TrimSpace(msg), ok && restored
}

// unit is the fix f of it, with the commands to run.
func (r *fixRunner) unit(it Item, f fixAction) fixUnit {
	cmds := f.Cmds
	if !r.backup && f.Plain != nil {
		cmds = f.Plain
	}
	return fixUnit{ids: []int{it.ID}, titles: []string{r.title(it)}, fix: f, cmds: cmds}
}

// title is the title of an element in the console.
func (r *fixRunner) title(it Item) string {
	if t, ok := r.titles[it.ID]; ok {
		return t
	}
	return itemTitle(it)
}

// fixTitles are the titles of items in the console where no file follows them (in
// what the fixes do, and in "Then, if the update does not fix …"): their own, but for
// copies of a software, which have the same, followed by the place of each.
func fixTitles(items []Item) map[int]string {
	count, inFolder := map[string]int{}, map[string]int{}
	for _, it := range items {
		count[itemTitle(it)]++
		inFolder[itemTitle(it)+"\x00"+it.Folder]++
	}
	out := map[int]string{}
	for _, it := range items {
		t := itemTitle(it)
		if count[t] > 1 {
			place := it.Folder
			if place == "" || inFolder[t+"\x00"+place] > 1 { // in the same folder: its file
				place = it.copyPath()
			}
			if place != "" {
				t = Tf("fix.titleIn", t, shownPath(place))
			}
		}
		out[it.ID] = t
	}
	return out
}

// unitTitles are the titles of the elements of units, each once.
func unitTitles(units []fixUnit) []string {
	var out []string
	seen := map[string]bool{}
	for _, u := range units {
		for _, t := range u.titles {
			if !seen[t] {
				seen[t] = true
				out = append(out, t)
			}
		}
	}
	return out
}

// round runs the actions of the elements ids (their updates, the other fixes, or their
// restores): the commands of each element in turn, then those that need administrator
// rights, together. A fix that several elements have, such as the update of a software
// for two of its problems, runs once, with the first of them. It returns the elements
// that ran something, with whether all their commands succeeded.
func (r *fixRunner) round(ids []int, actions func(Item) []fixAction) (ran []int, ok map[int]bool, stopped bool) {
	ok = map[int]bool{}
	// note records what came of units, and returns the one after which the run stopped
	// (unitResult.halted), or nil
	note := func(units []fixUnit, res []unitResult, err error) (halt *fixUnit) {
		if err != nil {
			r.out(fixLine{lineError, err.Error()})
		} else if !anyRan(res) {
			r.out(fixLine{lineError, T("fix.notRun")}) // PowerShell started, but ran nothing (blocked by a policy?)
		}
		for i := range res {
			if res[i].halted && i < len(units) {
				halt = &units[i]
			}
		}
		for i, u := range units {
			if halt != nil && (i >= len(res) || !res[i].ran) { // not reached: as after Stop
				for _, id := range u.ids {
					if _, seen := ok[id]; seen {
						ok[id] = false
					}
				}
				continue
			}
			for _, id := range u.ids {
				if _, seen := ok[id]; !seen {
					ok[id] = true
					ran = append(ran, id)
				}
				ok[id] = ok[id] && err == nil && i < len(res) && res[i].ran && res[i].ok
			}
		}
		return halt
	}
	var jobs [][]*fixUnit // for each element, its fixes that run without administrator rights
	var elevated []*fixUnit
	byCmds := map[string]*fixUnit{}
	for _, id := range ids {
		it, found := r.item(id)
		if !found {
			continue
		}
		var own []*fixUnit
		for _, f := range actions(it) {
			u := r.unit(it, f)
			k := strings.Join(u.cmds, "\n")
			if first := byCmds[k]; first != nil { // already run for another element
				first.ids = append(first.ids, u.ids...)
				first.titles = append(first.titles, u.titles...)
				continue
			}
			byCmds[k] = &u
			if f.Admin {
				elevated = append(elevated, &u)
			} else {
				own = append(own, &u)
			}
		}
		if len(own) > 0 {
			jobs = append(jobs, own)
		}
	}
	values := func(us []*fixUnit) []fixUnit {
		out := make([]fixUnit, len(us))
		for i, u := range us {
			out[i] = *u
		}
		return out
	}
	// Stop, or a failed connection: an element that has a fix that ran, and others that
	// did not, is not fixed; the console says why in red below the commands that ran
	notRun := func(why string, left ...[]*fixUnit) {
		r.out(fixLine{lineError, why})
		r.out(fixLine{lineNote, ""})
		for _, us := range left {
			for _, u := range us {
				for _, id := range u.ids {
					if _, seen := ok[id]; seen {
						ok[id] = false
					}
				}
			}
		}
	}
	for i, job := range jobs {
		if r.stopped() {
			notRun(T("fix.interrupted"), append(jobs[i:], elevated)...)
			return ran, ok, true
		}
		units := values(job)
		for _, t := range unitTitles(units) {
			r.out(fixLine{lineSoftware, t})
		}
		res, err := r.exec(units, false, r.out)
		if halt := note(units, res, err); halt != nil {
			notRun(haltReason(*halt), append(jobs[i+1:], elevated)...)
			return ran, ok, true
		}
		r.out(fixLine{lineNote, ""}) // a blank line after the commands of each software
	}
	if len(elevated) > 0 {
		if r.stopped() {
			notRun(T("fix.interrupted"), elevated)
			return ran, ok, true
		}
		r.out(fixLine{lineNote, T("fix.adminPrompt")}) // the log names each software as its commands run
		units := values(elevated)
		res, err := r.exec(units, true, r.out)
		if halt := note(units, res, err); halt != nil {
			notRun(haltReason(*halt))
			return ran, ok, true
		}
		r.out(fixLine{lineNote, ""})
	}
	return ran, ok, false
}

// haltReason says why the run stopped after the fix u (unitResult.halted): one of its
// downloads failed, or it is an update that failed, and its servers cannot be reached.
func haltReason(u fixUnit) string {
	if len(u.fix.Fetch) > 0 {
		return T("fix.stoppedDownload")
	}
	return Tf("fix.stoppedServer", strings.Join(u.fix.Servers, T("fix.serversOr")))
}

// checkAgain checks again the elements that ran something, and reports each one:
// fixed, or restored.
func (r *fixRunner) checkAgain(ids []int, ok map[int]bool) {
	if len(ids) == 0 {
		return
	}
	r.out(fixLine{lineNote, T("fix.checking")})
	r.recheck(ids)
	for _, id := range ids {
		it, found := r.item(id)
		if !found {
			continue
		}
		if r.restoring {
			if fixSimulation && ok[id] && !r.restored(it) && r.simulated != nil {
				r.simulated(id) // nothing changed on disk: the simulation shows what a restore would
				it, _ = r.item(id)
			}
			if ok[id] && r.restored(it) {
				r.out(fixLine{lineFixed, Tf("restore.done", r.title(it))})
				r.fixedAny = true
			} else {
				r.out(fixLine{lineNotFixed, Tf("restore.notDone", r.title(it))})
			}
			continue
		}
		if fixSimulation && ok[id] && !fixedStatus(it.Status) && r.simulated != nil {
			r.simulated(id) // nothing changed on disk: the simulation shows what a fix would
			it, _ = r.item(id)
		}
		if fixedStatus(it.Status) {
			r.out(fixLine{lineFixed, Tf("fix.fixed", r.title(it))})
			r.fixed(id)
			r.fixedAny = true
		} else {
			r.out(fixLine{lineNotFixed, Tf("fix.notFixed", r.title(it))})
		}
	}
}

// fixPreview is what the Fix window shows before the fixes run: for each element, its
// title and, below it, its file, which also tells copies of a software apart; then the
// commands of each fix, as they will run, after a comment that says what they do. The elements that share an
// update, such as two problems of the same software, are shown together, with the
// update once, as it runs: the fixes of their files follow, for when it does not fix
// them.
func fixPreview(items []Item, backup bool) []fixLine {
	var out []fixLine
	cmdLines := func(f fixAction) {
		out = append(out, fixLine{lineComment, fixIntro(f)})
		cmds := f.Cmds
		if !backup && f.Plain != nil {
			cmds = f.Plain
		}
		for _, c := range cmds {
			out = append(out, fixLine{lineCommand, c})
		}
	}
	titles := fixTitles(items)
	for _, group := range updateGroups(items) {
		var updates []fixAction
		seen := map[string]bool{}
		for _, it := range group {
			out = append(out, fixLine{lineSoftware, itemTitle(it)})
			if p := it.copyPath(); p != "" {
				out = append(out, fixLine{lineComment, shownPath(p)})
			}
			for _, f := range it.Fixes {
				if f.Kind == fixPackage && !seen[f.key()] {
					seen[f.key()] = true
					updates = append(updates, f)
				}
			}
		}
		for _, f := range updates {
			cmdLines(f)
		}
		for _, it := range group {
			first := true
			for _, f := range it.Fixes {
				if f.Kind == fixPackage {
					continue
				}
				if first && len(updates) > 0 {
					if len(group) > 1 {
						out = append(out, fixLine{lineComment, Tf("fix.stillFor", titles[it.ID])})
					} else {
						out = append(out, fixLine{lineComment, T("fix.ifStillNeeded")})
					}
				}
				first = false
				cmdLines(f)
			}
		}
		out = append(out, fixLine{lineNote, ""})
	}
	return out
}

// updateGroups are the elements that have fixes, in groups that share an update
// (the same commands), in the order of their first element.
func updateGroups(items []Item) [][]Item {
	parent := make([]int, len(items)) // the group of each element: a smaller index in it, down to its first
	find := func(i int) int {
		for parent[i] != i {
			i = parent[i]
		}
		return i
	}
	first := map[string]int{} // the first element of each update
	for i, it := range items {
		parent[i] = i
		for _, f := range it.Fixes {
			if f.Kind != fixPackage {
				continue
			}
			if j, ok := first[f.key()]; ok {
				a, b := find(i), find(j)
				parent[max(a, b)] = min(a, b)
			} else {
				first[f.key()] = i
			}
		}
	}
	var groups [][]Item
	at := map[int]int{}
	for i, it := range items {
		if len(it.Fixes) == 0 {
			continue
		}
		root := find(i)
		if g, ok := at[root]; ok {
			groups[g] = append(groups[g], it)
		} else {
			at[root] = len(groups)
			groups = append(groups, []Item{it})
		}
	}
	return groups
}

// fixIntro says what a fix does.
func fixIntro(f fixAction) string {
	admin := ""
	if f.Admin {
		admin = " " + T("fix.asAdmin")
	}
	switch f.Kind {
	case fixPackage, fixTool:
		return Tf("fix.kindPackage", f.Manager, admin)
	case fixPip:
		return Tf("fix.kindPip", admin)
	case fixConda:
		return Tf("fix.kindConda", admin)
	case fixJava:
		return Tf("fix.kindJava", admin)
	case fixNode:
		return Tf("fix.kindNode", admin)
	case fixPySSL:
		return Tf("fix.kindPySSL", admin)
	}
	return Tf("fix.kindBundle", admin)
}

// ---------------------------------------------------------------------------
// The script and its log

// fixMarker starts the lines of the log that are not output: a token drawn for each
// run, which no output can guess.
func fixMarker() string {
	b := make([]byte, 8)
	rand.Read(b)
	return "##emudscan-" + hex.EncodeToString(b)
}

// fixScript is the PowerShell script that runs units, writing into logPath, for each
// command, a marker, what it shows (its errors after the marker and "!", its warnings
// after the marker and "?", but the lines that a program writes to its error output,
// which are part of what it shows), and a marker with its result: failed when it
// raised an error, other than the lines that a program writes to its error output
// (which pip, WinGet or Chocolatey do for information), or exited with a code other
// than 0. An error shows as its message only: PowerShell would show it with the line
// of this script where it happened, which is not the user's command. The commands run
// in the script's scope, so that the variables of a fix ($cacert, $ok) go from one
// command to the next.
//
// The helper files of the fixes (fixFiles) are next to the script, in $emudscanFiles:
// the script exits with code 97, as when it changed, if one of them did.
//
// A failed connection ends the script, after a "halt" line in the log, so that the run
// stops (unitResult.halted): a download of a fix that failed (fixAction.Fetch), before
// the fix changed anything (its cleanup still runs: fixAction.Cleanup); an update that
// failed, when none of the servers its package manager needs can be reached
// (fixAction.Servers, Test-EmudscanServers). A failed update does not say why it failed
// (its exit code is the same for a program in use), so the test decides; a failed
// download stops the run in any case: its fix cannot go on.
func fixScript(units []fixUnit, logPath, marker string, files map[string][]byte) string {
	var b strings.Builder
	b.WriteString("$ErrorActionPreference = 'Continue'\n")
	b.WriteString("$ProgressPreference = 'SilentlyContinue'\n") // Invoke-WebRequest is much slower with its progress bar
	fmt.Fprintf(&b, "$emudscanLog = %s\n", psQuote(logPath))
	b.WriteString("$emudscanFiles = Split-Path -Parent $emudscanLog\n")
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		sum := sha256.Sum256(files[n])
		fmt.Fprintf(&b, "if ((Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $emudscanFiles %s)).Hash -ne '%s') { exit %d }\n",
			psQuote(n), strings.ToUpper(hex.EncodeToString(sum[:])), fixTampered)
	}
	b.WriteString("function Write-EmudscanLog([string]$s) { [IO.File]::AppendAllText($emudscanLog, $s + \"`n\", [Text.UTF8Encoding]::new($false)) }\n")
	for _, u := range units {
		if len(u.fix.Servers) > 0 {
			b.WriteString(serversTest())
			break
		}
	}
	prev := 0 // the first element of the previous fix
	for u, unit := range units {
		first := -1 - u // a fix without elements (in the tests): one of its own
		if len(unit.ids) > 0 {
			first = unit.ids[0]
		}
		if simulatedPause != "" && (u == 0 || first != prev) { // the commands of another software
			b.WriteString(simulatedPause)
		}
		prev = first
		run := func(c int) {
			command := unit.cmds[c]
			if fixSimulation {
				command = simulatedCommand(command)
			}
			fmt.Fprintf(&b, "Write-EmudscanLog '%s cmd %d %d'\n", marker, u, c)
			b.WriteString("$Error.Clear(); $global:LASTEXITCODE = 0\n")
			fmt.Fprintf(&b, "try { $emudscanOut = . {\n%s\n} *>&1 | ForEach-Object { %s } | Out-String -Width 4096 } catch { $emudscanOut = %s }\n", command, fixOutputSort(marker), fixMarkLines(marker+"!", "$_.ToString()"))
			b.WriteString("$emudscanCode = $global:LASTEXITCODE; if ($null -eq $emudscanCode) { $emudscanCode = 0 }\n")
			b.WriteString("$emudscanFailed = $emudscanCode -ne 0 -or @($Error | Where-Object { \"$($_.FullyQualifiedErrorId)\" -notlike 'NativeCommandError*' }).Count -gt 0\n")
			b.WriteString("Write-EmudscanLog (\"$emudscanOut\".TrimEnd())\n")
			fmt.Fprintf(&b, "Write-EmudscanLog ('%s end %d %d ' + [int]$emudscanFailed + ' ' + $emudscanCode)\n", marker, u, c)
		}
		for c := range unit.cmds {
			run(c)
			halt := fmt.Sprintf("{ Write-EmudscanLog '%s halt %d %d'; exit 0 }\n", marker, u, c)
			switch {
			case unit.fix.fetches(c) && unit.fix.Cleanup && c < len(unit.cmds)-1:
				// the last command removes what the fix downloaded: it runs before the end
				fmt.Fprintf(&b, "if ($emudscanFailed) {\nWrite-EmudscanLog '%s halt %d %d'\n", marker, u, c)
				run(len(unit.cmds) - 1)
				b.WriteString("exit 0\n}\n")
			case unit.fix.fetches(c):
				b.WriteString("if ($emudscanFailed) " + halt)
			case len(unit.fix.Servers) > 0:
				q := make([]string, len(unit.fix.Servers))
				for i, s := range unit.fix.Servers {
					q[i] = psQuote(s)
				}
				b.WriteString("if ($emudscanFailed -and -not (Test-EmudscanServers " + strings.Join(q, ", ") + ")) " + halt)
			}
		}
	}
	b.WriteString("exit 0\n")
	return b.String()
}

// serversTest defines Test-EmudscanServers, which tells whether one of the servers it
// is given can be reached, as Invoke-WebRequest reaches them (through the proxy of
// Windows): true once one of them answers, whatever its answer, even an error page;
// false when none can be, for lack of a connection: a name that does not resolve (nor
// the proxy's), a connection refused or that does not answer (no route, server down),
// a timeout. Any other error counts as reached (TLS, a connection cut later): the run
// goes on, as it should when the cause is not certain. The simulation reaches them or
// not as it is told (simulatedServers).
func serversTest() string {
	if s := simulatedServers(); s != "" {
		return s
	}
	return `function Test-EmudscanServers([string[]]$servers) {
  [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
  foreach ($s in $servers) {
    try { $null = Invoke-WebRequest -UseBasicParsing -Method Head -Uri "https://$s/" -TimeoutSec 10; return $true }
    catch {
      $x = $_.Exception; while ($x -and $x -isnot [Net.WebException]) { $x = $x.InnerException }
      if (-not $x -or "$($x.Status)" -notin 'NameResolutionFailure', 'ProxyNameResolutionFailure', 'ConnectFailure', 'Timeout') { return $true }
    }
  }
  return $false
}
`
}

// fixOutputSort sorts what a command shows ($_): its errors (as their message), then
// its warnings, marked for the console; what a program writes to its error output
// (NativeCommandError) and the rest, as they are.
func fixOutputSort(marker string) string {
	return "if ($_ -is [System.Management.Automation.ErrorRecord] -and \"$($_.FullyQualifiedErrorId)\" -notlike 'NativeCommandError*') { " + fixMarkLines(marker+"!", "$_.ToString()") + " } " +
		"elseif ($_ -is [System.Management.Automation.ErrorRecord]) { $_.ToString() } " +
		"elseif ($_ -is [System.Management.Automation.WarningRecord]) { " + fixMarkLines(marker+"?", "$_.Message") + " } else { $_ }"
}

// fixMarkLines is the lines of text, each after mark.
func fixMarkLines(mark, text string) string {
	return "((" + text + ") -split \"`r?`n\" | ForEach-Object { '" + mark + " ' + $_ }) -join \"`n\""
}

// fixBootstrap is the command that starts the script: it reads it, and runs it only
// if its SHA-256 is the one it was written with; otherwise it exits with code 97.
func fixBootstrap(scriptPath string, script []byte) string {
	sum := sha256.Sum256(script)
	return fmt.Sprintf("$t = [IO.File]::ReadAllText(%s, [Text.Encoding]::UTF8)\n"+
		"$h = -join ([Security.Cryptography.SHA256]::Create().ComputeHash([Text.Encoding]::UTF8.GetBytes($t)) | ForEach-Object { $_.ToString('X2') })\n"+
		"if ($h -ne '%s') { exit 97 }\n"+
		". ([scriptblock]::Create($t))\n", psQuote(scriptPath), strings.ToUpper(hex.EncodeToString(sum[:])))
}

// fixFiles are the helper files that the fixes of units need, made for a run at now.
func fixFiles(units []fixUnit, now time.Time) (map[string][]byte, error) {
	files := map[string][]byte{}
	for _, u := range units {
		if u.fix.Needs == "pycheck" {
			f, err := pyCheckFiles(now)
			if err != nil {
				return nil, err
			}
			for n, b := range f {
				files[n] = b
			}
			break
		}
	}
	return files, nil
}

// fixTampered is the exit code of a script that changed before it ran.
const fixTampered = 97

// encodedCommand is a command as PowerShell's -EncodedCommand takes it: UTF-16, in
// Base64.
func encodedCommand(s string) string {
	u := utf16.Encode([]rune(s))
	b := make([]byte, 2*len(u))
	for i, c := range u {
		binary.LittleEndian.PutUint16(b[2*i:], c)
	}
	return base64.StdEncoding.EncodeToString(b)
}

// fixLog reads the log of a script as it grows, and writes the console: each command,
// its result, and what it showed. With titles, it names the software before its
// first command (for the commands of several software, run as administrator).
type fixLog struct {
	units   []fixUnit
	marker  string
	out     func(fixLine)
	titles  bool
	last    int // the first element of the fix named last, plus 1
	results []unitResult
	partial string    // a line not complete yet
	shown   []fixLine // what the command running showed, given after its result
	errors  bool      // the command running showed an error
}

func newFixLog(units []fixUnit, marker string, out func(fixLine), titles bool) *fixLog {
	return &fixLog{units: units, marker: marker, out: out, titles: titles, results: make([]unitResult, len(units))}
}

// write takes what was added to the log since the last call.
func (l *fixLog) write(data string) {
	data = l.partial + data
	lines := strings.Split(data, "\n")
	l.partial = lines[len(lines)-1]
	for _, line := range lines[:len(lines)-1] {
		l.line(strings.TrimRight(line, "\r"))
	}
}

// close takes what remains once the script has ended.
func (l *fixLog) close() {
	if l.partial != "" {
		l.line(l.partial)
		l.partial = ""
	}
	l.flush()
}

// flush gives what the last command showed.
func (l *fixLog) flush() {
	for _, s := range l.shown {
		l.out(s)
	}
	l.shown = nil
}

// line takes a line of the log. The script writes what a command showed once it has
// ended, just before its result: the console gives the result first, in the left
// column of the command's line, then what it showed, its errors in red, then its exit
// code, when it is not 0.
func (l *fixLog) line(s string) {
	if e, ok := strings.CutPrefix(s, l.marker+"! "); ok {
		l.shown = append(l.shown, fixLine{lineCmdError, e})
		l.errors = l.errors || strings.TrimSpace(e) != ""
		return
	}
	if w, ok := strings.CutPrefix(s, l.marker+"? "); ok {
		l.shown = append(l.shown, fixLine{lineWarning, Tf("fix.warningLine", w)})
		return
	}
	rest, ok := strings.CutPrefix(s, l.marker+" ")
	if !ok {
		if strings.TrimSpace(s) != "" {
			l.shown = append(l.shown, fixLine{lineOutput, s})
		}
		return
	}
	f := strings.Fields(rest)
	num := func(i int) int {
		if i < len(f) {
			if n, err := strconv.Atoi(f[i]); err == nil {
				return n
			}
		}
		return -1
	}
	u, c := num(1), num(2)
	if u < 0 || u >= len(l.units) || c < 0 || c >= len(l.units[u].cmds) {
		return
	}
	switch f[0] {
	case "cmd":
		l.flush()
		if l.titles && l.last != l.units[u].ids[0]+1 {
			if l.last != 0 { // a blank line after the commands of the software before
				l.out(fixLine{lineNote, ""})
			}
			l.last = l.units[u].ids[0] + 1
			for _, t := range l.units[u].titles {
				l.out(fixLine{lineSoftware, t})
			}
		}
		l.out(fixLine{lineCommand, l.units[u].cmds[c]})
		l.errors = false
		if c == 0 {
			l.results[u] = unitResult{ran: true, ok: true}
		}
	case "end":
		failed, code := num(3) != 0, num(4)
		switch {
		case fixSimulation && !failed:
			l.out(fixLine{lineOK, simulatedResult(false)})
		case !failed:
			l.out(fixLine{lineOK, ""})
		default:
			l.out(fixLine{lineFailed, ""})
		}
		l.flush()
		switch {
		case !failed:
		case fixSimulation:
			l.out(fixLine{lineCmdError, simulatedResult(true)})
		case code != 0:
			l.out(fixLine{lineCmdError, Tf("fix.exitCode", code)})
		case !l.errors: // failed, with nothing to say why
			l.out(fixLine{lineCmdError, T("fix.cmdFailed")})
		}
		l.results[u].ok = l.results[u].ok && !failed
	case "halt":
		l.results[u].halted = true
	}
}
