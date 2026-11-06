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
	fix := joinParas(Tf("jbd.fix", "0.9.9728"), Tf("jb.toolboxGet", toolboxURL, ".exe (Windows)"), Tf("jbd.byHand", jetbrainsBlog))
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
		st, sum, details := jetbrainsdVerdict(c.ver, c.f, ".exe (Windows)")
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

// The IDEs are judged by their version with JetBrains' fixed versions: on the
// 2025.2 line, these fix only the Java runtime, not the built-in browser; versions
// 2025.1 and earlier (2025.2 for Rider) get no fix; the preview builds of 2026.3
// cannot be told apart, but its release candidate and release are fixed.
func TestJetbrainsIDEVerdict(t *testing.T) {
	Lang = 0
	for _, c := range []struct {
		code, ver, suffix string
		status            Status
		want              []string
	}{
		{"IU", "2026.2.3", "", StWarn, []string{Tf("app.sumOutdated", "2026.2.4"), T("jb.why"), Tf("app.fix", "IntelliJ IDEA", "2026.2.4"),
			Tf("jb.ideGet", "IntelliJ IDEA", productsURL)}},
		{"IU", "2026.2.4", "", StOK, []string{T("app.sumOK"), Tf("jb.ok", "IntelliJ IDEA 2026.2.4")}},
		{"IU", "2026.3", "", StOK, []string{T("app.sumOK")}},
		{"IU", "2026.3", "RC", StOK, []string{T("app.sumOK")}},
		{"IU", "2026.3", "EAP", StCaution, []string{T("jb.sumPreview"), Tf("jb.preview", "2026.3", "2026.3")}},
		{"RD", "2026.3", "EAP", StCaution, []string{T("jb.sumPreview")}},
		{"IU", "2027.1", "", StOK, []string{T("app.sumOK")}},
		{"IU", "2025.2.6.4", "", StCaution, []string{T("jb.sumBrowser"), Tf("jb.jbrOnly", "IntelliJ IDEA 2025.2.6.4", "2025.3.6.2"), T("jb.jcefOff")}},
		{"IU", "2025.2.6.3", "", StWarn, []string{Tf("app.sumOutdated", "2025.2.6.4"), Tf("jb.line20252", "IntelliJ IDEA 2025.2.6.4", "2025.3.6.2")}},
		{"PS", "2025.1.4", "", StWarn, []string{T("jb.sumUnsupported"), Tf("jb.unsupported", "PhpStorm 2025.1.4", "2025.2", fixedVersionsList("jb-pycharm"))}},
		{"RD", "2025.2.6", "", StWarn, []string{T("jb.sumUnsupported"), "2025.3 "}},
		{"CL", "2026.2.4", "", StWarn, []string{Tf("app.sumOutdated", "2026.2.4.1")}},
		{"IU", "", "", StCaution, []string{T("version.unknown")}},
		{"DS", "2025.1.3", "", StWarn, []string{T("jb.sumUnsupported"), Tf("jb.ideGetNoPage", "DataSpell")}},
	} {
		names := map[string]string{"IU": "IntelliJ IDEA", "PS": "PhpStorm", "RD": "Rider", "CL": "CLion", "DS": "DataSpell"}
		pi := productInfo{Name: names[c.code], Version: c.ver, VersionSuffix: c.suffix, ProductCode: c.code}
		st, sum, details := jetbrainsIDEVerdict(pi, jetbrainsProducts[c.code])
		if st != c.status {
			t.Errorf("%s %s %s: status %v, want %v", c.code, c.ver, c.suffix, st, c.status)
		}
		for _, w := range c.want {
			if !strings.Contains(sum+"\n"+details, w) {
				t.Errorf("%s %s %s: missing %q in %q / %q", c.code, c.ver, c.suffix, w, sum, details)
			}
		}
	}
}

// An IDE is found by its product-info.json, reported under its name and version,
// and its Java (jbr) is shown as a part of it, with the version that fixes it and how
// to update it. The Toolbox App's Java is shown as a part of the Toolbox App, with
// where to download it. Copies of IDEs that Gradle downloads and products outside the
// bulletin are not reported.
func TestJetbrainsIDEs(t *testing.T) {
	Lang = 0
	g3 := rootByName(rootNameECCG3)
	root := t.TempDir()
	java := func(home string) {
		os.MkdirAll(filepath.Join(home, "lib", "security"), 0o755)
		os.MkdirAll(filepath.Join(home, "bin"), 0o755)
		os.WriteFile(filepath.Join(home, "lib", "security", "cacerts"), append([]byte("keystore "), g3.Cert.Raw...), 0o644)
		os.WriteFile(filepath.Join(home, "release"), []byte("IMPLEMENTOR=\"JetBrains s.r.o.\"\nJAVA_VERSION=\"21.0.8\"\n"), 0o644)
	}
	ide := func(dir, name, code, ver string) string {
		os.MkdirAll(filepath.Join(dir, "bin"), 0o755)
		os.WriteFile(filepath.Join(dir, "bin", "idea64.exe"), []byte("MZ"), 0o644)
		os.WriteFile(filepath.Join(dir, "product-info.json"), []byte(`{"name": "`+name+`", "version": "`+ver+`", "buildNumber": "262.1.2",
  "productCode": "`+code+`", "dataDirectoryName": "IntelliJIdea2026.2",
  "launch": [{"os": "Windows", "arch": "amd64", "launcherPath": "bin/idea64.exe", "javaExecutablePath": "jbr/bin/java.exe"}]}`), 0o644)
		java(filepath.Join(dir, "jbr"))
		return dir
	}
	idea := ide(filepath.Join(root, "Programs", "IntelliJ IDEA"), "IntelliJ IDEA", "IU", "2026.2.3")
	gradle := ide(filepath.Join(root, ".gradle", "caches", "ideaIC-2024.1"), "IntelliJ IDEA", "IC", "2024.1")
	studio := ide(filepath.Join(root, "Android Studio"), "Android Studio", "AI", "2025.1.2")
	toolbox := filepath.Join(root, "JetBrains", "Toolbox", "bin")
	os.MkdirAll(toolbox, 0o755)
	os.WriteFile(filepath.Join(toolbox, "jetbrains-toolbox.exe"), []byte("MZ"), 0o644)
	os.WriteFile(filepath.Join(toolbox, "build.txt"), []byte("3.9.0.88839"), 0o644)
	java(filepath.Join(toolbox, "jre"))

	tp := testApps(t)
	tp.d, tp.allDrives = newDiscovery(), true
	for _, dir := range []string{idea, gradle, studio, toolbox} {
		ents, _ := os.ReadDir(dir)
		for _, e := range ents {
			tp.d.classify(filepath.Join(dir, e.Name()), e.Name(), e)
		}
	}
	if len(tp.d.ideDirs) != 3 {
		t.Errorf("IDE folders found: %v", tp.d.ideDirs)
	}
	for _, home := range []string{filepath.Join(idea, "jbr"), filepath.Join(toolbox, "jre")} {
		tp.d.javaHomes[home] = true
	}
	tp.checkJava()
	tp.checkJetbrainsIDEs()
	var ides []Item
	javas := map[string]Item{}
	for _, it := range tp.c.snapshot() {
		switch it.Kind {
		case "jetbrains":
			ides = append(ides, it)
		case "java":
			javas[it.Program] = it
		}
	}
	if len(ides) != 1 || ides[0].Program != "IntelliJ IDEA" || ides[0].Version != "2026.2.3" || ides[0].Status != StWarn {
		t.Errorf("IDEs: %+v", ides)
	}
	advises := func(it Item, texts ...string) bool {
		for _, s := range texts {
			if !strings.Contains(it.Details, s) {
				return false
			}
		}
		return !strings.Contains(it.Details, Tf("bundled.advice", it.Program, productsURL))
	}
	if it, ok := javas["IntelliJ IDEA"]; !ok || it.ProgVersion != "2026.2.3" ||
		!advises(it, Tf("jb.javaFix", "IntelliJ IDEA", "2026.2.4"), Tf("jb.ideGet", "IntelliJ IDEA", productsURL)) {
		t.Errorf("the IDE's Java: %+v", javas["IntelliJ IDEA"])
	}
	if it, ok := javas["JetBrains Toolbox App"]; !ok || it.ProgVersion != "3.9.0" ||
		!advises(it, T("jb.toolbox"), Tf("jb.toolboxGet", toolboxURL, T("jb.anyArch")), T("jb.toolboxDaemon")) {
		t.Errorf("the Toolbox App's Java: %+v", javas["JetBrains Toolbox App"])
	}
}

// The download to choose for the Toolbox App follows the processor of the JetBrains
// program installed on the PC, read from its header.
func TestDownloadChoice(t *testing.T) {
	Lang = 0
	dir := t.TempDir()
	exe := func(name string, machine uint16) string {
		b := make([]byte, 0x40+4+20)
		copy(b, "MZ")
		b[0x3c] = 0x40
		copy(b[0x40:], "PE\x00\x00")
		b[0x44], b[0x45] = byte(machine), byte(machine>>8)
		p := filepath.Join(dir, name)
		os.WriteFile(p, b, 0o644)
		return p
	}
	for p, want := range map[string]string{
		exe("x64.exe", 0x8664):            ".exe (Windows)",
		exe("arm64.exe", 0xaa64):          ".exe (Windows ARM64)",
		exe("x86.exe", 0x14c):             T("jb.anyArch"),
		filepath.Join(dir, "missing.exe"): T("jb.anyArch"),
	} {
		if got := downloadChoice(p); got != want {
			t.Errorf("%s: %q, want %q", filepath.Base(p), got, want)
		}
	}
}
