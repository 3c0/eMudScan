package main

// Fixes: the commands that the details page gives to fix an element, described as
// data, so that the Fix function runs exactly what the page shows. Each fix is made
// where its commands are written (newFix), and the page shows its commands as they
// are recorded. When an element is finished, its fixes are the recorded ones whose
// commands appear in its details, in the order of the page (Item.Fixes). Restoring a
// backup is recorded too, but kept apart (Item.Restores): it runs only from the
// Restore button of the details, never with the fixes. Other commands are never
// recorded, and so never run: updating every package of a package manager, removing
// a setting.

import (
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
	Kind    fixKind
	Admin   bool     // the commands need administrator rights
	Cmds    []string // the commands, as the details page shows them (with their backups)
	Plain   []string // the same commands without the backups of the files; nil when they make none
	Files   []string // the files the commands change, backed up first
	From    []string // for a restore: the backup of each file, which is put back in its place
	Manager string   // for an update: the package manager (Scoop, WinGet, Chocolatey)
	Warn    string   // what the fix breaks, which the warning before it runs adds
	Needs   string   // helper files that the runner writes next to the script ("pycheck": pyCheckFiles)
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

// lines are the commands of the fix as the details page shows them.
func (f fixAction) lines() string {
	var out []string
	for _, c := range f.Cmds {
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

// key identifies a fix by its commands.
func (f fixAction) key() string { return strings.Join(f.Cmds, "\n") }

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
		if c, ok := strings.CutPrefix(line, "$ "); ok {
			group = append(group, c)
			continue
		}
		flush()
	}
	flush()
	return out
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
