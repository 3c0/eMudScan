package main

// Dart and Flutter. On Windows, dart:io checks certificates with its own TLS library
// (BoringSSL), into which it copies the Windows certificate stores, but not the
// Windows block list (advisory GHSA-frr6-h895-5pxp). Dart 3.13.7 updates BoringSSL,
// which then rejects the compromised intermediates itself: it holds the SHA-256 of
// their public keys, as eMudScan's block list patterns do.
//
//   - The Dart SDK (dart-sdk\version, dart-sdk\bin\dart.exe) is judged by its version,
//     or by that block list in dart.exe.
//   - The Flutter SDK carries a Dart SDK (bin\cache\dart-sdk), judged the same way and
//     reported as Flutter, whose update brings it.
//   - An app built with Flutter has its engine, with dart:io and BoringSSL, in
//     flutter_windows.dll, which gives no version: it is judged by the block list
//     only, and otherwise as a program that copies the Windows root store.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

const (
	dartFixed    = "3.13.7"
	flutterFixed = "3.47.8"
)

// flutterRootOf returns the Flutter SDK that holds the Dart SDK at root
// (<flutter>\bin\cache\dart-sdk), or "".
func flutterRootOf(root string) string {
	cache := filepath.Dir(root)
	bin := filepath.Dir(cache)
	if lower(filepath.Base(cache)) != "cache" || lower(filepath.Base(bin)) != "bin" {
		return ""
	}
	fr := filepath.Dir(bin)
	if exists(filepath.Join(bin, "flutter.bat")) || exists(filepath.Join(bin, "flutter")) {
		return fr
	}
	return ""
}

// flutterVersion reads the version of the Flutter SDK at fr: bin\cache\flutter.version.json,
// which the flutter tool writes, or the older version file; "" if neither is there.
func flutterVersion(fr string) string {
	if data, err := os.ReadFile(filepath.Join(fr, "bin", "cache", "flutter.version.json")); err == nil {
		var v struct {
			FlutterVersion   string `json:"flutterVersion"`
			FrameworkVersion string `json:"frameworkVersion"`
		}
		if json.Unmarshal(data, &v) == nil {
			if v.FlutterVersion != "" {
				return plain(v.FlutterVersion)
			}
			if v.FrameworkVersion != "" {
				return plain(v.FrameworkVersion)
			}
		}
	}
	return firstLine(filepath.Join(fr, "version"))
}

func firstLine(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return plain(strings.TrimSpace(strings.SplitN(string(data), "\n", 2)[0]))
}

// flutterBuildFile: the copy of the engine in a Flutter project (windows\flutter\ephemeral)
// and the apps built from it (build\windows\…) are not installed apps: they are rebuilt
// with the Flutter SDK, which is checked.
func flutterBuildFile(p string) bool {
	s := lower(filepath.ToSlash(p))
	return strings.Contains(s, "/windows/flutter/ephemeral/") || strings.Contains(s, "/build/windows/")
}

func (tp *appsContext) checkDart() {
	for _, root := range dedupePaths(tp.d.dartSDKs) {
		root := root
		name := "Dart SDK"
		if flutterRootOf(root) != "" {
			name = "Flutter SDK"
		}
		tp.c.begin(tp.groupOf(name, root), "dart", name, root, func(it *Item) { tp.dartOne(it, root) })
	}
	for _, dir := range dedupePaths(tp.d.flutterApps) {
		if tp.inWindowsDir(dir) {
			continue
		}
		dir := dir
		name := tp.ownerOf(dir).Name
		if name == "" {
			name = plain(filepath.Base(dir))
		}
		tp.c.begin(tp.groupOf(name, dir), "flutter", name, dir, func(it *Item) { tp.flutterAppOne(it, name, dir) })
	}
}

func (tp *appsContext) dartOne(it *Item, root string) {
	ver := firstLine(filepath.Join(root, "version"))
	exe := filepath.Join(root, "bin", "dart.exe")
	r := result{Version: ver, File: exe, Exe: exe, Folder: filepath.Join(root, "bin"), ProductURL: "https://dart.dev/get-dart"}
	name := "Dart SDK"
	fr := flutterRootOf(root)
	if fr != "" {
		// the Dart of a Flutter SDK, which brings it
		name = "Flutter SDK"
		r.Component, r.ProgVersion = "Dart", flutterVersion(fr)
		r.Folder, r.ProductURL = filepath.Join(fr, "bin"), "https://docs.flutter.dev/release/upgrade"
	}
	o := tp.withOwner(&r, root)
	if fr != "" && r.Owner == "" {
		r.Owner = name
	}
	r.Search = searchFor(o.Name, name, false)
	var f embedFindings
	if fi, err := os.Stat(exe); err == nil && !isCloudPlaceholder(fi) {
		f, _ = tp.scan(exe, scanRoots)
	}
	r.Status, r.Summary, r.Details = dartVerdict(fr != "", ver, f.Blocklist, tp.pyStoreRisk)
	tp.c.finish(it, r)
}

// dartFixedVersion: a Dart release from 3.13.7 on. A prerelease (3.14.0-315.3.beta)
// does not count: only some have the fix, which the block list then shows.
func dartFixedVersion(ver string) bool {
	st, _ := fixStatus("dart", ParseVersion(ver))
	return st == FixOK && !strings.Contains(ver, "-")
}

// dartVerdict judges a Dart SDK of version ver (inside a Flutter SDK if flutter),
// whose dart.exe holds the block list or not, when a copy of the Windows root store
// would accept risk test chains.
func dartVerdict(flutter bool, ver string, blocklist bool, risk int) (Status, string, string) {
	if blocklist || dartFixedVersion(ver) {
		return StOK, T("app.sumOK"), Tf("dart.ok", strings.TrimSpace("Dart "+ver))
	}
	why := Tf("dart.copies", T("dart.theSDK")) + " " + Tf("dart.fixedBy", "Dart "+dartFixed)
	sum := Tf("app.sumOutdated", dartFixed)
	if flutter {
		why = Tf("dart.copies", T("flutter.theSDK")) + " " + Tf("dart.fixedBy", flutterWithDart())
		sum = Tf("app.sumOutdated", "Flutter "+flutterFixed)
	}
	if ver == "" {
		return StCaution, T("version.unknown"), joinParas(why, dartFix(flutter))
	}
	return StCaution, sum, joinParas(why, copyRisk(risk), dartFix(flutter))
}

// flutterWithDart names the fixed Flutter, with the Dart it brings.
func flutterWithDart() string { return "Flutter " + flutterFixed + " (Dart " + dartFixed + ")" }

func dartFix(flutter bool) string {
	if flutter {
		return Tf("flutter.fix", flutterFixed)
	}
	return Tf("dart.fix", dartFixed)
}

// copyRisk says what a copy of the Windows root store accepts on this PC.
func copyRisk(risk int) string {
	if risk > 0 {
		return Tf("net.copiesRisk", risk, tlsChainsTotal, refWinCopy())
	}
	return Tf("net.copiesNoRisk", refWinCopy())
}

func (tp *appsContext) flutterAppOne(it *Item, name, dir string) {
	dll := filepath.Join(dir, "flutter_windows.dll")
	r := result{File: dll, Folder: dir, Search: name, Component: "Flutter"}
	tp.withOwner(&r, dir)
	f, _ := tp.scan(dll, scanRoots)
	r.Status, r.Summary, r.Details = flutterAppVerdict(name, f.Blocklist, tp.pyStoreRisk)
	tp.c.finish(it, r)
}

// flutterAppVerdict judges an app built with Flutter, whose engine holds the block
// list or not, when a copy of the Windows root store would accept risk test chains.
func flutterAppVerdict(name string, blocklist bool, risk int) (Status, string, string) {
	if blocklist {
		return StOK, T("py.sumSSLBlocks"), Tf("flutterApp.ok", name)
	}
	why := Tf("dart.copies", name) + " " + Tf("dart.fixedBy", flutterWithDart())
	if risk > 0 {
		return StCaution, T("net.copies"), joinParas(why, copyRisk(risk), Tf("flutterApp.fix", name, flutterFixed))
	}
	return StOK, T("net.copiesOK"), joinParas(why, copyRisk(risk), Tf("flutterApp.later", name, flutterFixed))
}
