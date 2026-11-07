//go:build simulate

package main

import (
	"strings"
	"testing"
)

// The simulation runs, in place of each command, one that does nothing, or fails
// when the command contains EMUDSCAN_SIMULATE_FAIL, after a second for each software; a software whose
// commands all "succeeded" is then shown as fixed, although its check finds the same
// as before.
func TestFixSimulation(t *testing.T) {
	Lang = 0
	t.Setenv("EMUDSCAN_SIMULATE_FAIL", "choco")
	units := []fixUnit{{ids: []int{1}, cmds: []string{"scoop update app"}}, {ids: []int{1}, cmds: []string{"choco upgrade x -y", "choco upgrade y -y"}},
		{ids: []int{2, 3}, cmds: []string{"pip"}}}
	s := fixScript(units, `C:\Temp\fix.log`, "##m", nil)
	if strings.Contains(s, "scoop update app") || strings.Contains(s, "choco upgrade") || strings.Count(s, "\n$null = $null\n") != 2 ||
		strings.Count(s, "\n$global:LASTEXITCODE = 1\n") != 2 || strings.Count(s, simulatedPause) != 2 || strings.Contains(s, "Milliseconds") {
		t.Errorf("script:\n%s", s)
	}
	if !strings.HasSuffix(appTitle(), " · SIMULATION") {
		t.Errorf("title %q", appTitle())
	}
	it := Item{ID: 1, Name: "A", Status: StWarn, Fixes: []fixAction{{Kind: fixPackage, Cmds: []string{"scoop update a"}}}}
	var fixed []int
	r := fixRunner{
		exec: func(units []fixUnit, admin bool, out func(fixLine)) ([]unitResult, error) {
			return []unitResult{{ran: true, ok: true}}, nil
		},
		item:      func(int) (Item, bool) { return it, true },
		recheck:   func([]int) {},
		stopped:   func() bool { return false },
		out:       func(fixLine) {},
		fixed:     func(id int) { fixed = append(fixed, id) },
		simulated: func(int) { it.Status, it.Summary = StOK, T("fix.simulatedOK") },
	}
	r.run([]int{1})
	if len(fixed) != 1 || it.Summary != T("fix.simulatedOK") {
		t.Errorf("fixed %v, %q", fixed, it.Summary)
	}

	// the element then has nothing left to fix, in its details too, and the log says so
	var lines []string
	c := newChecker(logReporter{&lines}, nil)
	el := c.begin(GroupApps, "bundle", "certifi", `C:\App\certifi`, nil)
	c.finish(el, result{Status: StWarn, Summary: "Outdated", Details: "Replace it:\n" + cmd("scoop update app")})
	simulateFixed(c, el.ID)
	got, _ := c.get(el.ID)
	if got.Status != StOK || len(got.Fixes) != 0 || strings.Contains(got.Details, "scoop") || !strings.Contains(strings.Join(lines, "\n"), T("fix.simulatedOK")) {
		t.Errorf("simulated: %+v\n%s", got, strings.Join(lines, "\n"))
	}
	// a restore whose commands all "succeeded" shows as restored, with nothing left to
	// restore; its status stays
	rit := Item{ID: 2, Name: "B", Status: StOK, Restores: []fixAction{{Kind: fixRestore, Cmds: []string{"move b"}, Files: []string{"b"}}}}
	var out []fixLine
	r = fixRunner{exec: r.exec, item: func(int) (Item, bool) { return rit, true }, recheck: func([]int) {}, stopped: r.stopped,
		out: func(l fixLine) { out = append(out, l) }, simulated: func(int) { rit.Restores = nil }}
	r.restore([]int{2})
	if text := strings.Join(plainLines(out), "\n"); !strings.Contains(text, Tf("restore.done", "B")) {
		t.Errorf("simulated restore:\n%s", text)
	}
	el = c.begin(GroupApps, "bundle", "certifi", `C:\App2\certifi`, nil)
	c.finish(el, result{Status: StOK, Summary: "Up to date", Details: "Restore it:\n" + newFix(fixAction{Kind: fixRestore, Cmds: []string{"move app2"}}).lines()})
	if got, _ := c.get(el.ID); len(got.Restores) != 1 {
		t.Fatalf("restores: %+v", got)
	}
	simulateRestored(c, el.ID)
	if got, _ := c.get(el.ID); got.Status != StOK || len(got.Restores) != 0 || strings.Contains(got.Details, "move") || got.Summary != T("fix.simulatedRestored") {
		t.Errorf("simulated restore: %+v", got)
	}
}
