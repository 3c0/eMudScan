package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Only the jetbrainsd that the Toolbox App or an IDE installed for a user is checked.
func TestIsJetbrainsdExe(t *testing.T) {
	for _, c := range []struct {
		p    string
		want bool
	}{
		{`C:\Users\me\AppData\Local\JetBrains\Daemon\bundles\current\jetbrainsd\bin\jetbrainsd.exe`, true},
		{`D:\Users\Other\AppData\Local\JetBrains\Daemon\Bundles\Current\JetBrainsD\bin\JetBrainsD.exe`, true},
		{"/home/me/.local/JetBrains/Daemon/bundles/current/jetbrainsd/bin/jetbrainsd.exe", true},
		{`C:\Users\me\Downloads\jetbrainsd\bin\jetbrainsd.exe`, false},
		{`C:\Users\me\AppData\Local\JetBrains\Daemon\bundles\tmp\jetbrainsd\bin\jetbrainsd.exe`, false},
	} {
		if got := isJetbrainsdExe(c.p); got != c.want {
			t.Errorf("%s: %v", c.p, got)
		}
	}
}

// jetbrainsd 0.9.9728 holds the fingerprints of the compromised certificates, while
// keeping the affected roots in its list; 0.9.8000 holds only the roots.
func TestJetbrainsdVerdict(t *testing.T) {
	roots := []string{rootNameAAA, rootNameECCG3}
	fix := Tf("jbd.fix", "0.9.9728", jetbrainsBlog)
	for _, c := range []struct {
		ver    string
		f      embedFindings
		status Status
		want   []string
	}{
		{"0.9.8000", embedFindings{Roots: roots}, StWarn, []string{Tf("app.sumOutdated", "0.9.9728"), rootNameECCG3, T("jbd.why"), fix}},
		{"0.9.9728", embedFindings{Roots: roots, Blocklist: true}, StOK, []string{T("app.sumOK"), Tf("jbd.ok", "jetbrainsd 0.9.9728")}},
		{"0.9.9999", embedFindings{}, StOK, []string{T("app.sumOK")}},
		{"", embedFindings{Roots: roots, Blocklist: true}, StOK, []string{Tf("jbd.ok", "jetbrainsd")}},
		{"", embedFindings{Roots: roots}, StCaution, []string{T("version.unknown"), fix}},
	} {
		st, sum, details := jetbrainsdVerdict(c.ver, c.f)
		if st != c.status {
			t.Errorf("%s: status %v, want %v", c.ver, st, c.status)
		}
		for _, w := range c.want {
			if !strings.Contains(sum+"\n"+details, w) {
				t.Errorf("%s: missing %q in %q / %q", c.ver, w, sum, details)
			}
		}
	}
}

// The jetbrainsd of the user running the scan is found in %LOCALAPPDATA%, the others
// by the scan of the drives; each is reported once, as a part of JetBrains, with its
// version from bin\VERSION and its verdict from what its program holds.
func TestCheckJetbrainsd(t *testing.T) {
	Lang = 0
	g3 := rootByName(rootNameECCG3)
	exeIn := func(lad, ver string, fixed bool) string {
		bin := filepath.Join(lad, "JetBrains", "Daemon", "bundles", "current", "jetbrainsd", "bin")
		os.MkdirAll(bin, 0o755)
		data := append([]byte("MZ "), g3.Cert.Raw...)
		if fixed {
			for _, b := range badCerts {
				data = append(append(data, ' '), strings.ToUpper(b.SHA256)...)
			}
		}
		exe := filepath.Join(bin, "jetbrainsd.exe")
		os.WriteFile(exe, append(data, make([]byte, 300<<10)...), 0o644)
		os.WriteFile(filepath.Join(bin, "VERSION"), []byte(ver), 0o644)
		return exe
	}
	mine := exeIn(filepath.Join(t.TempDir(), "me"), "0.9.8000", false)
	other := exeIn(filepath.Join(t.TempDir(), "other"), "0.9.9728", true)
	t.Setenv("LOCALAPPDATA", filepath.Join(filepath.Dir(mine), "..", "..", "..", "..", "..", ".."))
	tp := testApps(t)
	tp.d, tp.allDrives = newDiscovery(), true
	tp.d.jetbrainsds[mine] = true // found by the scan too
	tp.d.jetbrainsds[other] = true
	tp.checkJetbrainsd()
	got := map[string]Item{}
	for _, it := range tp.c.snapshot() {
		if it.Kind == "jetbrainsd" {
			got[it.File] = it
		}
	}
	if len(got) != 2 {
		t.Fatalf("%d elements, want 2: %+v", len(got), got)
	}
	for exe, want := range map[string]struct {
		st  Status
		ver string
	}{mine: {StWarn, "0.9.8000"}, other: {StOK, "0.9.9728"}} {
		it, ok := got[exe]
		if !ok {
			t.Errorf("%s: not reported", exe)
			continue
		}
		if it.Status != want.st || it.Version != want.ver || it.Program != "JetBrains" || it.Component != "jetbrainsd" {
			t.Errorf("%s: %v %q, shown as %q › %q", exe, it.Status, it.Version, it.Program, it.Component)
		}
	}
}
