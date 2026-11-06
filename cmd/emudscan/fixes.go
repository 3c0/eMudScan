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
	// Optional: the fix replaces a component inside a program (its node.exe, the ssl
	// files of its Python), a heavier change than the others, which the Fix window does
	// not tick: the user ticks it, or clicks Select All.
	Optional bool
}

// lines are the commands of the fix as the details page shows them.
func (f fixAction) lines() string {
	var out []string
	for _, c := range f.Cmds {
		out = append(out, cmd(c))
	}
	return strings.Join(out, "\n")
}

// tickedFirst tells whether the Fix window ticks a row of these elements when it
// opens: not when all their fixes are optional (fixAction.Optional).
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

// backsUp tells whether a fix of these elements backs up files (fixAction.Plain), the
// only ones that "Back up the certificate files" changes: the Fix window shows that
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
