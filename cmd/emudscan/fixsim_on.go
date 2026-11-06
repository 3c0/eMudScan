//go:build simulate

package main

// The simulation build (go build -tags simulate; scripts\build.ps1 -Simulate) runs
// the Fix function as usual, Windows PowerShell and the administrator prompt
// included, but each command only says that it was not run. A command that contains
// the text of the environment variable EMUDSCAN_SIMULATE_FAIL fails instead, to show
// what eMudScan does then. A second passes before the commands of each software, as
// they would take some time. Since nothing changes, a software whose commands all
// "succeeded" is then shown as fixed. The window title and the report say
// SIMULATION. The GitHub build never sets this tag.

import (
	"os"
	"strings"
)

const fixSimulation = true

// The texts of the simulation, which a normal build does not contain.
func init() {
	for k, v := range map[string][2]string{
		"fix.simulatedDone":            {"Done (simulated)", "Fait (simulé)"},
		"fix.simulatedFailed":          {"Failed (simulated)", "Échec (simulé)"},
		"fix.simulatedOK":              {"Fixed (simulated)", "Corrigé (simulé)"},
		"fix.simulatedRestored":        {"Restored (simulated)", "Restauré (simulé)"},
		"fix.simulatedRestoredDetails": {"Restored in the simulation: its commands were not run, so its files did not change. After a real restore, this page shows what a new check finds.", "Restauré dans la simulation\u00a0: ses commandes n’ont pas été exécutées, ses fichiers n’ont donc pas changé. Après une vraie restauration, cette page montre ce que trouve une nouvelle vérification."},
		"fix.simulatedDetails":         {"Fixed in the simulation: its commands were not run, so its files did not change. After a real fix, this page shows what a new check finds.", "Corrigé dans la simulation\u00a0: ses commandes n’ont pas été exécutées, ses fichiers n’ont donc pas changé. Après une vraie correction, cette page montre ce que trouve une nouvelle vérification."},
	} {
		tr[k] = v
	}
}

// simulateFixed shows an element whose commands all "succeeded" as a check would
// after a real fix: OK, with nothing left to fix, and so in its details, the log and
// the report. Nothing changed on disk, so its check found the same as before.
func simulateFixed(c *checker, id int) {
	c.mu.Lock()
	it := c.items[id]
	if it == nil {
		c.mu.Unlock()
		return
	}
	it.Status, it.Summary, it.Details, it.Fixes, it.Restores = StOK, T("fix.simulatedOK"), T("fix.simulatedDetails"), nil, nil
	cp, where := *it, c.where[id]
	c.mu.Unlock()
	c.r.Upsert(cp)
	c.stepStart(stepLabel(cp.Name, where))
	c.stepEnd(StOK, cp.Summary)
}

// simulateRestored shows an element whose restore "succeeded" with nothing left to
// restore. Its status stays as it was: what a real restore leaves depends on the
// files put back, which the simulation did not change.
func simulateRestored(c *checker, id int) {
	c.mu.Lock()
	it := c.items[id]
	if it == nil {
		c.mu.Unlock()
		return
	}
	it.Summary, it.Details, it.Restores = T("fix.simulatedRestored"), T("fix.simulatedRestoredDetails"), nil
	cp, where := *it, c.where[id]
	c.mu.Unlock()
	c.r.Upsert(cp)
	c.stepStart(stepLabel(cp.Name, where))
	c.stepEnd(cp.Status, cp.Summary)
}

const simulationMark = " · SIMULATION"

// simulatedPause comes before the commands of each software: a second, as they would
// take some time.
const simulatedPause = "Start-Sleep -Seconds 1\n"

// simulatedResult is the result of a simulated command, which failed or not.
func simulatedResult(failed bool) string {
	if failed {
		return T("fix.simulatedFailed")
	}
	return T("fix.simulatedDone")
}

// simulatedCommand runs in place of the command c: nothing, or, when c contains
// EMUDSCAN_SIMULATE_FAIL, a failure (exit code 1), which shows nothing either.
func simulatedCommand(c string) string {
	if f := os.Getenv("EMUDSCAN_SIMULATE_FAIL"); f != "" && strings.Contains(c, f) {
		return "$global:LASTEXITCODE = 1"
	}
	return "$null = $null"
}
