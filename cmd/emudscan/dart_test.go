package main

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Dart is fixed from 3.13.7 on, or when dart.exe holds BoringSSL's block list. An
// older Dart copies the Windows root store without its block list: the advice is the
// update, for Flutter through "flutter upgrade".
func TestDartVerdict(t *testing.T) {
	Lang = 0
	for _, c := range []struct {
		flutter   bool
		ver       string
		blocklist bool
		risk      int
		status    Status
		want      []string
	}{
		{false, "3.13.7", false, 0, StOK, []string{Tf("dart.ok", "Dart 3.13.7")}},
		{false, "3.14.1", false, 0, StOK, []string{T("app.sumOK")}},
		{false, "3.12.4", false, 2, StCaution, []string{Tf("app.sumOutdated", "3.13.7"), Tf("dart.copies", T("dart.theSDK")), Tf("dart.fixedBy", "Dart 3.13.7"), Tf("net.copiesRisk", 2, tlsChainsTotal, refWinCopy()), Tf("dart.fix", "3.13.7")}},
		{false, "3.13.6", false, 0, StCaution, []string{Tf("net.copiesNoRisk", refWinCopy())}},
		{true, "3.13.6", false, 0, StCaution, []string{Tf("app.sumOutdated", "Flutter 3.47.8"), Tf("dart.copies", T("flutter.theSDK")), Tf("dart.fixedBy", "Flutter 3.47.8 (Dart 3.13.7)"), "$ flutter upgrade"}},
		{false, "3.14.0-200.0.dev", false, 0, StCaution, []string{Tf("dart.fix", "3.13.7")}},
		{false, "3.14.0-315.3.beta", true, 0, StOK, []string{T("app.sumOK")}},
		{false, "", false, 0, StCaution, []string{T("version.unknown")}},
		{false, "", true, 0, StOK, []string{Tf("dart.ok", "Dart")}},
	} {
		st, sum, details := dartVerdict(c.flutter, c.ver, c.blocklist, c.risk)
		if st != c.status {
			t.Errorf("%v %q: status %v", c.flutter, c.ver, st)
		}
		for _, w := range c.want {
			if !strings.Contains(sum+"\n"+details, w) {
				t.Errorf("%v %q: missing %q in %q / %q", c.flutter, c.ver, w, sum, details)
			}
		}
	}
}

// An app built with Flutter is fixed when its engine holds the block list; otherwise
// it is judged as a program that copies the Windows root store.
func TestFlutterAppVerdict(t *testing.T) {
	Lang = 0
	if st, sum, _ := flutterAppVerdict("App", true, 2); st != StOK || sum != T("py.sumSSLBlocks") {
		t.Errorf("fixed engine: %v %q", st, sum)
	}
	if st, sum, d := flutterAppVerdict("App", false, 2); st != StCaution || sum != T("net.copies") || !strings.Contains(d, Tf("flutterApp.fix", "App", "3.47.8")) {
		t.Errorf("old engine, risk: %v %q %q", st, sum, d)
	}
	if st, sum, d := flutterAppVerdict("App", false, 0); st != StOK || sum != T("net.copiesOK") || !strings.Contains(d, Tf("flutterApp.later", "App", "3.47.8")) {
		t.Errorf("old engine, no risk: %v %q %q", st, sum, d)
	}
}

// The scan finds a Dart SDK, the Dart SDK inside a Flutter SDK (reported as Flutter,
// with Flutter's version), although it is in a cache folder, which the scan skips,
// and an app's Flutter engine, but not the engine files of the Flutter SDK itself.
func TestDartAndFlutterFound(t *testing.T) {
	Lang = 0
	root := t.TempDir()
	write := func(rel, data string) {
		p := filepath.Join(root, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	h, _ := hex.DecodeString(uniqueBadSPKIs()[0])
	write(`dart-sdk/version`, "3.12.4\n")
	write(`dart-sdk/bin/dart.exe`, "MZ")
	write(`flutter/bin/flutter.bat`, "@echo off")
	write(`flutter/bin/cache/dart-sdk/version`, "3.13.7")
	write(`flutter/bin/cache/dart-sdk/bin/dart.exe`, "MZ")
	write(`flutter/bin/cache/flutter.version.json`, `{"frameworkVersion": "3.47.8", "flutterVersion": "3.47.8", "dartSdkVersion": "3.13.7"}`)
	write(`flutter/bin/cache/artifacts/engine/windows-x64/flutter_windows.dll`, "MZ")
	write(`apps/Fixed/flutter_windows.dll`, "MZ engine "+string(h))
	write(`apps/Old/flutter_windows.dll`, "MZ engine")
	// a Flutter project: its copy of the engine, and the app built from it
	write(`project/windows/flutter/ephemeral/flutter_windows.dll`, "MZ engine")
	write(`project/build/windows/x64/runner/Release/flutter_windows.dll`, "MZ engine")

	tp := testApps(t)
	tp.d = newDiscovery()
	tp.d.walk(tp.c, walkSpec{root: root, quiet: true}, true)
	tp.checkDart()
	got := map[string]Item{}
	for _, it := range tp.c.snapshot() {
		got[it.Name+"|"+it.Version] = it
	}
	for key, st := range map[string]Status{"Dart SDK|3.12.4": StCaution, "Flutter SDK|3.13.7": StOK, "Fixed|": StOK, "Old|": StOK} {
		if it, ok := got[key]; !ok || it.Status != st {
			t.Errorf("%s: %+v (found %v)", key, it, ok)
		}
	}
	if len(got) != 4 {
		t.Errorf("%d elements: %v", len(got), got)
	}
	if it := got["Old|"]; it.Summary != T("net.copiesOK") || it.Component != "Flutter" {
		t.Errorf("old engine: %q, component %q", it.Summary, it.Component)
	}
	// the Dart of a Flutter SDK: Flutter is the program, with its own version
	if it := got["Flutter SDK|3.13.7"]; it.Program != "Flutter SDK" || it.ProgVersion != "3.47.8" || it.Component != "Dart" {
		t.Errorf("Flutter SDK: program %q %q, component %q", it.Program, it.ProgVersion, it.Component)
	}
	if it := got["Dart SDK|3.12.4"]; it.Component != "" {
		t.Errorf("Dart SDK: component %q", it.Component)
	}
}

// The Flutter engine of an internet program is reported on its own: the program's
// other files are still checked, here a library with its own outdated root store.
func TestFlutterEngineInInternetProgram(t *testing.T) {
	Lang = 0
	dir := t.TempDir()
	pad := make([]byte, 300<<10) // program files from 256 KB are read
	os.WriteFile(filepath.Join(dir, "flutter_windows.dll"), append([]byte("MZ engine"), pad...), 0o644)
	os.WriteFile(filepath.Join(dir, "native.dll"), append(append([]byte("MZ "), rootByName(rootNameECCG3).Cert.RawSubjectPublicKeyInfo...), pad...), 0o644)
	tp := testApps(t)
	tp.d = newDiscovery()
	tp.d.flutterApps[dir] = true
	tp.netPrograms = []netProgram{{name: "Remote", dir: dir}}
	tp.checkDart()
	tp.checkInternetFolders()
	kinds := map[string]Status{}
	for _, it := range tp.c.snapshot() {
		kinds[it.Kind] = it.Status
	}
	if kinds["flutter"] != StOK || kinds["netapp"] != StWarn {
		t.Errorf("elements: %v", kinds)
	}
}
