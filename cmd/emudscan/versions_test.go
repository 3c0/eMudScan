package main

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A program that keeps each version in a folder named after it is judged by its
// newest version: the folder of the previous one stays until the program restarts.
func TestNewestVersionFolder(t *testing.T) {
	Lang = 0
	g3 := rootByName(rootNameECCG3)
	h, _ := hex.DecodeString(uniqueBadSPKIs()[0])
	pad := make([]byte, 300<<10) // program files from 256 KB are read
	roots := append(append([]byte("MZ "), g3.Cert.RawSubjectPublicKeyInfo...), pad...)
	fixed := append(append(append([]byte("MZ "), g3.Cert.RawSubjectPublicKeyInfo...), h...), pad...)
	for _, c := range []struct {
		name       string
		old, new   []byte
		status     Status
		newestFile bool
	}{
		{"fixed in the new version", roots, fixed, StOK, true},
		{"fixed only in the old version", fixed, roots, StWarn, true},
	} {
		dir := t.TempDir()
		for v, data := range map[string][]byte{"156.0.4314.40": c.old, "156.0.4314.58": c.new} {
			p := filepath.Join(dir, "Application", v, "msedge.dll")
			os.MkdirAll(filepath.Dir(p), 0o755)
			os.WriteFile(p, data, 0o644)
		}
		tp := testApps(t)
		tp.netPrograms = []netProgram{{name: "Runtime", dir: dir}}
		tp.checkInternetFolders()
		var it Item
		for _, x := range tp.c.snapshot() {
			if x.Kind == "netapp" {
				it = x
			}
		}
		if it.Status != c.status || strings.Contains(it.File, "156.0.4314.40") {
			t.Errorf("%s: %v, file %s", c.name, it.Status, it.File)
		}
	}
	files := []string{
		filepath.Join("P", "app.exe"),
		filepath.Join("P", "1.2.9", "a.dll"), filepath.Join("P", "1.10.0", "a.dll"), filepath.Join("P", "1.10.0", "sub", "b.dll"),
		filepath.Join("P", "plugins", "2.0.0.1", "c.dll"), filepath.Join("P", "plugins", "2.0.0.0", "c.dll"),
		filepath.Join("P", "1.0", "d.dll"), // not a full version: kept
	}
	want := []string{files[0], files[2], files[3], files[4], files[6]}
	if got := newestVersionFiles("P", files); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("kept %v", got)
	}
}

// The WebView2 Runtime of Windows says that the software using it is protected from
// its next start, or, when outdated, that it must be updated: Microsoft updates it.
func TestWebView2Runtime(t *testing.T) {
	Lang = 0
	g3 := rootByName(rootNameECCG3)
	h, _ := hex.DecodeString(uniqueBadSPKIs()[0])
	pad := make([]byte, 300<<10)
	roots := append(append([]byte("MZ "), g3.Cert.RawSubjectPublicKeyInfo...), pad...)
	fixed := append(append(append([]byte("MZ "), g3.Cert.RawSubjectPublicKeyInfo...), h...), pad...)
	for _, c := range []struct {
		name, prog string
		data       []byte
		status     Status
		want, not  string
	}{
		{"runtime, fixed", "Microsoft Edge WebView2 Runtime", fixed, StOK, T("webview2.runtimeOK"), T("webview2.runtimeOld")},
		{"runtime, outdated", "Microsoft Edge WebView2 Runtime", roots, StWarn, T("webview2.runtimeOld"), Tf("net.fix", "Microsoft Edge WebView2 Runtime")},
		{"other program", "Runtime", fixed, StOK, T("embed.blocklistNote"), T("webview2.runtimeOK")},
	} {
		dir := t.TempDir()
		p := filepath.Join(dir, "Application", "156.0.4314.58", "msedge.dll")
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, c.data, 0o644)
		tp := testApps(t)
		tp.netPrograms = []netProgram{{name: c.prog, dir: filepath.Join(dir, "Application")}}
		tp.checkInternetFolders()
		var it Item
		for _, x := range tp.c.snapshot() {
			if x.Kind == "netapp" {
				it = x
			}
		}
		if it.Status != c.status || !strings.Contains(it.Details, c.want) || strings.Contains(it.Details, c.not) {
			t.Errorf("%s: %v\n%s", c.name, it.Status, it.Details)
		}
	}
	if !isWebView2Runtime(netProgram{name: "x", dir: `C:\Program Files (x86)\Microsoft\EdgeWebView\Application`}) {
		t.Error("runtime folder not recognized")
	}
}
