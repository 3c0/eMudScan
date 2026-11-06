package main

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The WebView2 that software ships in its own folder is found by the scan and judged
// by the block list built into its msedge.dll; Microsoft Edge's and the WebView2
// Runtime's own copies are checked as such, not here.
func TestWebView2Copies(t *testing.T) {
	Lang = 0
	g3 := rootByName(rootNameECCG3)
	h, _ := hex.DecodeString(uniqueBadSPKIs()[0])
	roots := append([]byte("MZ "), g3.Cert.RawSubjectPublicKeyInfo...)
	fixed := append(append([]byte("MZ "), g3.Cert.RawSubjectPublicKeyInfo...), h...)
	root := t.TempDir()
	write := func(rel string, data []byte) {
		p := filepath.Join(root, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for dir, dll := range map[string][]byte{
		filepath.Join("apps", "Viewer", "Microsoft.WebView2.FixedVersionRuntime.120.0.2210.91.x64"): roots,
		filepath.Join("apps", "Reader", "runtime"):                                                  fixed,
		filepath.Join("Microsoft", "EdgeWebView", "Application", "156.0.4314.58"):                   fixed,
		filepath.Join("Microsoft", "Edge", "Application", "156.0.4314.58"):                          roots,
	} {
		write(filepath.Join(dir, "msedgewebview2.exe"), []byte("MZ"))
		write(filepath.Join(dir, "msedge.dll"), dll)
	}
	tp := testApps(t)
	tp.d = newDiscovery()
	tp.d.walk(tp.c, walkSpec{root: root, quiet: true}, true)
	if len(tp.d.webview2Dirs) != 2 {
		t.Fatalf("found %v", tp.d.webview2Dirs)
	}
	tp.checkWebView2Copies()
	got := map[string]Item{}
	for _, it := range tp.c.snapshot() {
		got[it.Name] = it
	}
	viewer, reader := got["Viewer"], got["Reader"]
	if viewer.Status != StWarn || viewer.Program != "Viewer" || viewer.Component != "WebView2" || viewer.Version != "120.0.2210.91" ||
		!strings.Contains(viewer.Details, Tf("webview2.ownOld", "Viewer")) {
		t.Errorf("outdated copy: %+v", viewer)
	}
	if reader.Status != StOK || reader.Summary != T("embed.sumBlocklist") || reader.Version != "" {
		t.Errorf("fixed copy: %+v", reader)
	}
	if len(got) != 2 {
		t.Errorf("items: %v", got)
	}
}
