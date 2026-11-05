package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The warning about changing a copy that belongs to other software comes right
// above the line that says where to run the commands that change it, set apart from
// the text before, and only then: not for advice without commands, nor before the
// commands that restore a backup, which have their own.
func TestCopyWarning(t *testing.T) {
	Lang = 0
	warn := T("bundled.warn")
	advice := "**This copy belongs to App.**\n\nRecommended action: update App."
	lead := "If no update is available yet, the following commands fix it:"
	header := T("fix.psAdmin") + "\n$ Copy-Item a b"
	cmds := lead + "\n" + header
	restore := T("restore.intro1") + "\n• a\n\n" + T("restore.how1") + "\n" + T("restore.warn1") + "\n" + T("fix.ps") + "\n$ Move-Item a b"
	for _, c := range []struct {
		name, details, want string
	}{
		{"commands", advice + "\n\n" + cmds, advice + "\n\n" + lead + "\n\n" + warn + "\n" + header},
		{"commands and restore", advice + "\n\n" + cmds + "\n\n" + restore, advice + "\n\n" + lead + "\n\n" + warn + "\n" + header + "\n\n" + restore},
		{"header only", advice + "\n\n" + header, advice + "\n\n" + warn + "\n" + header},
		{"header on the line of the text", advice + "\n\n" + lead + " " + header, advice + "\n\n" + warn + "\n" + lead + " " + header},
		{"commands only", advice + "\n\n$ Copy-Item a b", advice + "\n\n" + warn + "\n$ Copy-Item a b"},
		{"restore only", advice + "\n\n" + restore, advice + "\n\n" + restore},
		{"no commands", advice, advice},
	} {
		if got := copyWarning(c.details); got != c.want {
			t.Errorf("%s:\n%s\nwant:\n%s", c.name, got, c.want)
		}
	}
	// an element that is a copy inside other software gets it when it finishes
	c := newChecker(nopReporter{}, nil)
	it := c.begin(GroupApps, "java", "Java", "", func(it *Item) {
		c.finish(it, result{Bundled: true, Details: advice + "\n\n" + cmds})
	})
	if !strings.Contains(it.Details, lead+"\n\n"+warn+"\n"+header) {
		t.Errorf("finished element:\n%s", it.Details)
	}
}

// A certificate bundle of software that eMudScan does not know (a portable program
// on another drive) belongs to it all the same: the warning comes before the
// commands that replace it.
func TestCopyWarningUnknownOwner(t *testing.T) {
	Lang = 0
	p := filepath.Join(t.TempDir(), "Portable", "app", "certifi", "cacert.pem")
	os.MkdirAll(filepath.Dir(p), 0o755)
	os.WriteFile(p, outdatedBundle(t), 0o644)
	tp := testApps(t)
	tp.d = newDiscovery()
	tp.d.bundles[p] = true
	tp.checkBundles()
	var it *Item
	for _, id := range tp.c.order {
		if i := tp.c.items[id]; i.Kind == "bundle" {
			it = i
		}
	}
	if it == nil {
		t.Fatal("no element")
	}
	if it.Bundled || !strings.Contains(it.Details, T("bundle.genericFix")) {
		t.Fatalf("not the advice for unknown software:\n%s", it.Details)
	}
	at := strings.Index(it.Details, T("bundled.warn")+"\n"+T("fix.ps"))
	if at < 0 || at < strings.Index(it.Details, T("bundle.genericFix")) || strings.Count(it.Details, T("bundled.warn")) != 1 {
		t.Errorf("warning missing or misplaced:\n%s", it.Details)
	}
}
