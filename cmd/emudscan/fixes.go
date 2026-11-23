package main

// Fixes: the commands that the details page gives to fix an element, described as
// data, so that the Fix function runs exactly what the page shows; but a fix whose
// commands use what only eMudScan has, of which the page shows a version that runs
// alone (fixAction.Alone), and the Fix window the commands that it runs. Each fix is
// made where its commands are written (newFix), and the page shows its commands as
// they are recorded. When an element is finished, its fixes are the recorded ones whose
// commands appear in its details, in the order of the page (Item.Fixes). Restoring a
// backup is recorded too, but kept apart (Item.Restores): it runs only from the
// Restore button of the details, never with the fixes. Other commands are never
// recorded, and so never run: updating every package of a package manager, removing
// a setting.

import (
	"regexp"
	"slices"
	"strings"
	"sync"
)

// fixKind is what a fix does.
type fixKind int

const (
	fixPackage fixKind = iota // updates the software with the package manager that installed it
	fixPip                    // updates pip or certifi with pip
	fixConda                  // updates conda's certificates with conda
	fixBundle                 // replaces a certificate bundle with Mozilla's current one
	fixJava                   // adds the compromised certificates to a Java block list
	fixRestore                // puts back files that eMudScan's commands backed up (restoreAdvice)
	fixTool                   // updates with a tool, after the package managers' updates (uv's Python, after uv)
	fixNode                   // replaces a node.exe inside a program with nodejs.org's of its release line
	fixPySSL                  // replaces the ssl files of a Python inside a program with those of its fixed release
)

// fixAction is one fix: commands that run together, in Windows PowerShell.
type fixAction struct {
	Kind  fixKind
	Admin bool     // the commands need administrator rights
	Cmds  []string // the commands, as the details page shows them (with their backups)
	// Script: the commands are the lines of a script written to be read: its commands,
	// each on one line, and between them comments, empty lines and the lines that open
	// or close a block ("try {", "catch {", "}"). Each command that runs has its result;
	// the fix stops at its first command that fails, and inside "try" the failure goes
	// to "catch", as in any PowerShell script whose errors all stop it (fixScript).
	Script bool
	// Alone is the script that the details page shows, when it is not the commands
	// that the Fix function runs: a version of them that runs without eMudScan, for a
	// fix whose commands use what only eMudScan has (the checks of a Python's ssl
	// module). The Fix window shows, and runs, the commands.
	Alone   []string
	Plain   []string // the same commands without the backups of the files; nil when they make none
	Files   []string // the files the commands change, backed up first
	From    []string // for a restore: the backup of each file, which is put back in its place
	Manager string   // for an update: the package manager (Scoop, WinGet, Chocolatey)
	Warn    string   // what the fix breaks, which the warning before it runs adds
	Needs   string   // helper files that the runner writes next to the script ("pycheck": pyCheckFiles)
	// Note is a line that the Fix window shows above the commands, such as the release
	// that could not be looked for.
	Note string
	// py, for a fix that replaces the ssl files of a Python (pySSLFix): what it was made
	// for, to make it again for the latest release when a Fix window opens (latest.go).
	py *pySSLTarget
	// Optional: the fix replaces a component inside a program (its node.exe, the ssl
	// files of its Python), a heavier change than the others, which the Fix window does
	// not tick: the user ticks it, or clicks Select All.
	Optional bool
	// Fetch are the commands that download what the fix needs, before it changes
	// anything, by their place (the same in Cmds and Plain): when one fails, the fix has
	// changed nothing, and the run stops there (fixScript).
	Fetch []int
	// Cleanup: the last command removes what the fix downloaded; it runs even when a
	// download fails and the run stops.
	Cleanup bool
	// Servers are, for an update, the servers that its package manager needs: when the
	// update fails and none of them can be reached, the run stops there (fixScript).
	Servers []string
}

// fetches: the command c of the fix downloads what it needs (Fetch).
func (f fixAction) fetches(c int) bool { return slices.Contains(f.Fetch, c) }

// blockLineRe is a line of a script that only opens or closes a block.
var blockLineRe = regexp.MustCompile(`^\s*(try \{|catch \{|& \{|\})\s*$`)

// blockLine: the line s of a script only opens or closes a block.
func blockLine(s string) bool { return blockLineRe.MatchString(s) }

// isCommand: the line s of a script is a command, which runs and has a result: not an
// empty line, a comment or a line of a block.
func isCommand(s string) bool {
	t := strings.TrimSpace(s)
	return t != "" && !strings.HasPrefix(t, "#") && !blockLine(s)
}

// runs: the line s of the fix is a command. Every line is one, but in a script.
func (f fixAction) runs(s string) bool { return !f.Script || isCommand(s) }

// The servers of the updates (fixAction.Servers): those of their package managers'
// default sources.
var (
	serversGitHub = []string{"github.com"} // Scoop and its buckets; uv's own update, and the Pythons it installs
	serversPyPI   = []string{"pypi.org"}   // pip, pipx, uv's tools
	serversConda  = []string{"repo.anaconda.com", "conda.anaconda.org"}
	// managerServers are those of the package managers whose name is enough to know them.
	managerServers = map[string][]string{
		"Scoop":                  serversGitHub,
		"WinGet":                 {"cdn.winget.microsoft.com"},
		"Chocolatey":             {"community.chocolatey.org"},
		"pipx":                   serversPyPI,
		"Python Install Manager": {"www.python.org"},
		"Deno":                   {"dl.deno.land"},
	}
)

// shown are the commands of the fix that the details page shows: those that the Fix
// function runs, or their version that runs alone (Alone).
func (f fixAction) shown() []string {
	if f.Alone != nil {
		return f.Alone
	}
	return f.Cmds
}

// lines are the commands of the fix as the details page shows them.
func (f fixAction) lines() string {
	var out []string
	for _, c := range f.shown() {
		out = append(out, cmd(c))
	}
	return strings.Join(out, "\n")
}

// tickedFirst tells whether the Fix window ticks a row of these elements (with the
// fixes of that row, fixParts) when it opens: not when all their fixes are optional
// (fixAction.Optional).
func tickedFirst(items []Item) bool {
	for _, it := range items {
		for _, f := range it.Fixes {
			if !f.Optional {
				return true
			}
		}
	}
	return false
}

// fixPart is which fixes of an element a row of the Fix window's list runs.
type fixPart int

const (
	partAll      fixPart = iota // all of them
	partUsual                   // those that are not optional
	partOptional                // the optional ones
)

func (p fixPart) has(f fixAction) bool { return p == partAll || f.Optional == (p == partOptional) }

// fixParts are the rows of the Fix window's list for it: one, but two when it has
// optional fixes and others, so that the optional ones stay unticked, and run only
// when the user ticks their row.
func fixParts(it Item) []fixPart {
	usual, optional := false, false
	for _, f := range it.Fixes {
		usual, optional = usual || !f.Optional, optional || f.Optional
	}
	if usual && optional {
		return []fixPart{partUsual, partOptional}
	}
	return []fixPart{partAll}
}

// fixRow is a row of the list of the Fix window: an element, and which of its fixes
// the row runs.
type fixRow struct {
	Item
	part fixPart
}

// fixPick is, for each element, the parts of its fixes that run (those of the ticked
// rows); nil: all of them.
type fixPick map[int][]fixPart

func (p fixPick) runs(id int, f fixAction) bool {
	if p == nil {
		return true
	}
	for _, part := range p[id] {
		if part.has(f) {
			return true
		}
	}
	return false
}

// items are items with only the fixes that run, without those left with none.
func (p fixPick) items(items []Item) []Item {
	if p == nil {
		return items
	}
	var out []Item
	for _, it := range items {
		var fixes []fixAction
		for _, f := range it.Fixes {
			if p.runs(it.ID, f) {
				fixes = append(fixes, f)
			}
		}
		if len(fixes) > 0 {
			it.Fixes = fixes
			out = append(out, it)
		}
	}
	return out
}

// backsUp tells whether a fix of these elements backs up files (fixAction.Plain), the
// only ones that "Back up the files to change" changes: the Fix window shows that
// box for them alone.
func backsUp(items []Item) bool {
	for _, it := range items {
		for _, f := range it.Fixes {
			if f.Plain != nil {
				return true
			}
		}
	}
	return false
}

// fixWarnings is what the fixes of these elements break (fixAction.Warn), each once,
// for the warning before they run.
func fixWarnings(items []Item) string {
	var out []string
	seen := map[string]bool{}
	for _, it := range items {
		for _, f := range it.Fixes {
			if f.Warn != "" && !seen[f.Warn] {
				seen[f.Warn] = true
				out = append(out, f.Warn)
			}
		}
	}
	return strings.Join(out, "\n\n")
}

// fixWarningText is the warning before the fixes of items run: what they break (the
// virtual environments of a Python they remove), then the general warning, with, after
// its first paragraph (what eMudScan will run), the order of the fixes when the
// preview says it (fixOrderDiffers).
func fixWarningText(items []Item) string {
	first, rest, _ := strings.Cut(T("fix.warning"), "\n\n")
	order := ""
	if fixOrderDiffers(items) {
		order = T("fix.order")
	}
	return joinParas(fixWarnings(items), first, order, rest)
}

// key identifies a fix by its commands, as the details page shows them.
func (f fixAction) key() string { return strings.Join(f.shown(), "\n") }

// recordedFixes are the fixes made so far, by their commands: the same commands are
// the same fix wherever they are shown.
var recordedFixes = struct {
	sync.Mutex
	m map[string]fixAction
}{m: map[string]fixAction{}}

// newFix records a fix; the details page shows its commands with f.lines().
func newFix(f fixAction) fixAction {
	recordedFixes.Lock()
	recordedFixes.m[f.key()] = f
	recordedFixes.Unlock()
	return f
}

// fixesIn finds the fixes whose commands the details show, in their order: each
// group of commands that follow one another is looked up as a whole.
func fixesIn(details string) []fixAction {
	var out []fixAction
	seen := map[string]bool{}
	var group []string
	flush := func() {
		if len(group) == 0 {
			return
		}
		k := strings.Join(group, "\n")
		group = nil
		recordedFixes.Lock()
		f, ok := recordedFixes.m[k]
		recordedFixes.Unlock()
		if ok && !seen[k] {
			seen[k] = true
			out = append(out, f)
		}
	}
	for _, line := range strings.Split(strings.ReplaceAll(details, "\r", ""), "\n") {
		if c, ok := cmdOf(line); ok {
			group = append(group, c)
			continue
		}
		flush()
	}
	flush()
	return out
}

// guide writes, above the commands of the details of a finished element, what to do
// with them, in place of the line that announced them until then (psHeader): for a fix
// that the Fix function can run, to click the button of the window, by its name, or to
// run them oneself in PowerShell; for the restore of backups, the same with the name
// that the button has when the element has nothing to fix, else without the button,
// which then fixes; for other commands, to run them in PowerShell. It says "this
// command", "these commands" or "this script", and, when they need them, the
// administrator rights, which must not be missed. vulnerable: the fix is for a
// vulnerability, not for something that is only good to do.
func guide(details string, vulnerable bool) string {
	fixes, _ := restoresApart(fixesIn(details))
	lines := strings.Split(details, "\n")
	for i, line := range lines {
		text, ok := howOf(line)
		if !ok || text != T("fix.ps") && text != T("fix.psAdmin") {
			continue
		}
		// the commands below it: its first group, then the rest of its paragraph
		var first []string
		n, grouped := 0, false
		for _, l := range lines[i+1:] {
			if strings.TrimSpace(l) == "" {
				break
			}
			c, isCmd := cmdOf(l)
			if !isCmd {
				grouped = grouped || len(first) > 0
				continue
			}
			if isCommand(c) { // not the comments and the empty lines of a script
				n++
			}
			if !grouped {
				first = append(first, c)
			}
		}
		recordedFixes.Lock()
		f, recorded := recordedFixes.m[strings.Join(first, "\n")]
		recordedFixes.Unlock()
		what := T("fix.what1")
		switch {
		case recorded && f.Script && n > 1:
			what = T("fix.whatScript")
		case n > 1:
			what = T("fix.whatN")
		}
		admin := ""
		if text == T("fix.psAdmin") || recorded && f.Admin {
			admin = T("fix.withAdmin")
		}
		switch {
		case !recorded:
			lines[i] = how(Tf("fix.runSelf", what, admin))
		case f.Kind == fixRestore && len(fixes) > 0:
			lines[i] = how(Tf("fix.undoSelf", what, admin))
		case f.Kind == fixRestore && len(f.Files) > 1:
			lines[i] = how(Tf("fix.undo", T("dlg.restoreN"), what, admin))
		case f.Kind == fixRestore:
			lines[i] = how(Tf("fix.undo", T("dlg.restore1"), what, admin))
		case vulnerable:
			lines[i] = how(Tf("fix.do", T("dlg.fix"), what, admin))
		default:
			lines[i] = how(Tf("fix.doSo", T("dlg.fix"), what, admin))
		}
	}
	return strings.Join(lines, "\n")
}

// fixBlock is the commands of a fix under the advice that leads to them: the advice,
// which ends there, then what to do with the commands (guide), then the commands.
func fixBlock(advice string, f fixAction) string {
	return endSentence(advice) + "\n" + how(T("fix.ps")) + "\n" + f.lines()
}

// restoresApart splits the actions of an element: its fixes, and the restores of its
// backups.
func restoresApart(actions []fixAction) (fixes, restores []fixAction) {
	for _, f := range actions {
		if f.Kind == fixRestore {
			restores = append(restores, f)
		} else {
			fixes = append(fixes, f)
		}
	}
	return fixes, restores
}

// anyNeedsAdmin: one of the files needs administrator rights to be changed.
func anyNeedsAdmin(files ...string) bool {
	for _, f := range files {
		if needsAdmin(f) {
			return true
		}
	}
	return false
}
