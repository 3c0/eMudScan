package main

import (
	"strings"
	"testing"
)

// LM Studio's two copies of Node.js 25.5.0 are one element listing both files; a
// copy with another verdict, one still being checked and the Windows checks stay apart.
func TestMergeRepeated(t *testing.T) {
	Lang = 0
	node := func(id int, file string, st Status, summary string) Item {
		it := Item{ID: id, Group: GroupApps, Kind: "node", Name: "Node.js", Version: "25.5.0", Owner: "LM Studio", OwnerVer: "0.4.26+4",
			Status: st, Summary: summary, File: file, Details: "This Node.js release line no longer receives fixes."}
		it.fillDisplay("Node.js")
		return it
	}
	items := []Item{
		node(1, `C:\Users\me\.lmstudio\.internal\utils\node.exe`, StWarn, "Release line without the fix"),
		{ID: 2, Group: GroupWindows, Kind: "windows", Name: "Windows TLS test (offline)", Status: StOK, Summary: "12 of 12 test chains rejected"},
		node(3, `C:\Users\me\AppData\Local\Programs\LM Studio\resources\app\.webpack\bin\node.exe`, StWarn, "Release line without the fix"),
		node(4, `C:\elsewhere\node.exe`, StOK, "Fixed version"),
		node(5, `C:\pending\node.exe`, StPending, "Checking…"),
		{ID: 6, Group: GroupWindows, Kind: "windows", Name: "Windows TLS test (offline)", Status: StOK, Summary: "12 of 12 test chains rejected"},
	}
	got := mergeRepeated(items)
	var ids []int
	for _, it := range got {
		ids = append(ids, it.ID)
	}
	if len(got) != 5 || got[0].ID != 1 || len(got[0].Copies) != 1 || got[0].Copies[0].ID != 3 {
		t.Fatalf("merged IDs %v, first copies %+v", ids, got[0].Copies)
	}
	if m := got[0].memberIDs(); len(m) != 2 || m[0] != 1 || m[1] != 3 {
		t.Errorf("memberIDs = %v", m)
	}
	d := got[0].shownDetails()
	if !strings.Contains(d, "Found in 2 places:") || !strings.Contains(d, `.lmstudio\.internal\utils\node.exe`) || !strings.Contains(d, `.webpack\bin\node.exe`) {
		t.Errorf("details do not list both copies:\n%s", d)
	}
	if strings.Count(d, "no longer receives fixes") != 1 {
		t.Errorf("the same explanation must be shown once:\n%s", d)
	}
	if c := appCell(got[0], colFile); !strings.HasSuffix(c, "node.exe (+1)") {
		t.Errorf("file cell %q", c)
	}
	// the input is not changed
	if len(items[0].Copies) != 0 {
		t.Errorf("mergeRepeated changed its input")
	}
}

// Copies whose explanations differ (their commands name their own python.exe) each
// keep theirs.
func TestMergeRepeatedDifferentDetails(t *testing.T) {
	Lang = 0
	py := func(id int, home string) Item {
		it := Item{ID: id, Group: GroupApps, Kind: "python", Name: "Python", Version: "3.14.7", Owner: "ESPHome Device Builder",
			Status: StWarn, Summary: "Outdated certificate bundle (pip/certifi)", File: home + `\python.exe`,
			Details: "In PowerShell:\n$ & \"" + home + `\python.exe" -m pip install --upgrade pip`}
		it.fillDisplay("Python")
		return it
	}
	got := mergeRepeated([]Item{py(1, `C:\a\python`), py(2, `C:\b\python`)})
	if len(got) != 1 {
		t.Fatalf("%d elements, want 1", len(got))
	}
	d := got[0].shownDetails()
	for _, want := range []string{`In [[C:\a\python\python.exe|C:\a\python\python.exe]]:`, `& "C:\a\python\python.exe" -m pip`, `& "C:\b\python\python.exe" -m pip`} {
		if !strings.Contains(d, want) {
			t.Errorf("details without %q:\n%s", want, d)
		}
	}
}

// The counts and the report show a repeated finding once.
func TestRepeatedCountsAndReport(t *testing.T) {
	Lang = 0
	c := newChecker(nopReporter{}, nil)
	for _, f := range []string{`C:\one\node.exe`, `C:\two\node.exe`} {
		f := f
		c.begin(GroupApps, "node", "Node.js", "", func(it *Item) {
			c.finish(it, result{Status: StWarn, Summary: "Release line without the fix", Version: "25.5.0", File: f, Owner: "LM Studio", Component: "Node.js"})
		})
	}
	if n := c.counts(); n.warn != 1 {
		t.Errorf("counts: %d to update, want 1", n.warn)
	}
	rep := buildReport(c, "")
	if strings.Count(rep, "LM Studio › Node.js 25.5.0") != 1 || !strings.Contains(rep, "Found in 2 places:") ||
		!strings.Contains(rep, `C:\one\node.exe`) || !strings.Contains(rep, `C:\two\node.exe`) {
		t.Errorf("report:\n%s", rep)
	}
}
