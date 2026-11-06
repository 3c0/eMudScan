package main

// WebView2 shipped by software. Most software that displays web content with
// Microsoft Edge WebView2 uses the WebView2 Runtime of Windows, which Microsoft
// updates (internet.go). Some ships its own copy instead, a "fixed version" of
// WebView2 that only its publisher updates: its msedge.dll is checked for the block
// list built into recent versions, as Microsoft Edge's is.

import (
	"path/filepath"
	"regexp"
	"strings"
)

// microsoftWebView2: a msedgewebview2.exe of Microsoft Edge or of the WebView2
// Runtime of Windows, which are checked as such.
func microsoftWebView2(p string) bool {
	lp := strings.ReplaceAll(lower(p), "/", `\`)
	for _, d := range []string{`\microsoft\edgewebview\`, `\microsoft\edgecore\`, `\microsoft\edge\`,
		`\microsoft\edge beta\`, `\microsoft\edge dev\`, `\microsoft\edge sxs\`} {
		if strings.Contains(lp, d) {
			return true
		}
	}
	return false
}

// webview2DirVersionRe: the version in the name of a fixed version of WebView2
// ("Microsoft.WebView2.FixedVersionRuntime.120.0.2210.91.x64").
var webview2DirVersionRe = regexp.MustCompile(`(\d+\.\d+\.\d+\.\d+)`)

// webview2Software is the folder of the software that ships a WebView2 in dir: the
// folder above, as WebView2 is shipped in a folder of its own.
func webview2Software(dir string) string { return filepath.Dir(dir) }

func (tp *appsContext) checkWebView2Copies() {
	for _, dir := range dedupePaths(tp.d.webview2Dirs) {
		if tp.inWindowsDir(dir) || !exists(filepath.Join(dir, "msedge.dll")) {
			continue
		}
		dir := dir
		name := tp.ownerOf(dir).Name
		if name == "" {
			name = plain(filepath.Base(webview2Software(dir)))
		}
		tp.c.begin(tp.groupOf(name, dir), "webview2", name, dir, func(it *Item) { tp.webview2One(it, name, dir) })
	}
}

func (tp *appsContext) webview2One(it *Item, name, dir string) {
	dll := filepath.Join(dir, "msedge.dll")
	ver := ""
	fv, _ := getFileVersion(filepath.Join(dir, "msedgewebview2.exe"))
	if ver = trimZeros(displayVersion(fv)); ver == "" {
		ver = webview2DirVersionRe.FindString(filepath.Base(dir))
	}
	r := result{File: dll, Folder: webview2Software(dir), Search: name, Component: "WebView2", Version: ver}
	if tp.withOwner(&r, dir).Name == "" {
		r.Owner = name // the folder of WebView2 does not name the software: the one above does
	}
	tp.c.activity(Tf("act.read", shownPath(dll)))
	f, err := tp.scan(dll, scanRoots)
	tp.c.activity("")
	r.Status, r.Summary, r.Details = webview2Verdict(name, f, err)
	tp.c.finish(it, r)
}

// webview2Verdict judges the WebView2 that software ships, by its msedge.dll.
func webview2Verdict(name string, f embedFindings, err error) (Status, string, string) {
	switch {
	case err != nil:
		return StCaution, T("browser.noStore"), err.Error()
	case f.Blocklist:
		return StOK, T("embed.sumBlocklist"), Tf("webview2.ownOK", name)
	case !f.HasEmsignRoot():
		return StOK, T("browser.updated"), joinParas(Tf("webview2.own", name), T("browser.updatedDetails"))
	}
	return StWarn, T("embed.sumBad"), joinParas(rootsField(f), Tf("webview2.ownOld", name))
}
