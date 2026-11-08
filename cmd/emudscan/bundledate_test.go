package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The date that curl writes at the top of a bundle: "as of" or "downloaded on", the
// day it wrote whatever the time zone; none without that line, or with another one.
func TestMozillaDateIn(t *testing.T) {
	head := "##\n## Bundle of CA Root Certificates\n##\n## Certificate data from Mozilla as of: Fri Sep 25 03:12:01 2026 GMT\n##\n"
	u := mozillaDateIn(head)
	if u.Src != dateMozilla || u.Time.Format("2006-01-02") != "2026-09-25" {
		t.Errorf("as of: %v %v", u.Src, u.Time)
	}
	u = mozillaDateIn("##\r\n## Certificate data from Mozilla downloaded on: Thu Nov  5 23:59:59 2026 GMT\r\n")
	if u.Src != dateMozillaDownload || u.Time.Format("2006-01-02") != "2026-11-05" {
		t.Errorf("downloaded on: %v %v", u.Src, u.Time)
	}
	for _, h := range []string{"", "-----BEGIN CERTIFICATE-----\n", "## Certificate data from Mozilla as of: yesterday\n", "## Certificate data from Mozilla on: Fri Sep 25 03:12:01 2026 GMT\n"} {
		if u := mozillaDateIn(h); u.known() {
			t.Errorf("%q: %v", h, u.Time)
		}
	}
}

// A bundle on its own shows the date curl wrote in it, with curl's words, under its
// file (report, details, list), and no software date; without that line, no date at
// all. A bundle of a software keeps the software's date, and has curl's date too.
func TestBundleDateShown(t *testing.T) {
	Lang = 0
	dir := t.TempDir()
	lone := filepath.Join(dir, "cacert-2026-09-25.pem")
	os.WriteFile(lone, []byte("##\n## Bundle of CA Root Certificates\n##\n## Certificate data from Mozilla as of: Fri Sep 25 03:12:01 2026 GMT\n"), 0o644)
	plain := filepath.Join(dir, "plain.pem")
	os.WriteFile(plain, []byte("-----BEGIN CERTIFICATE-----\n"), 0o644)
	if u := mozillaDateOf(lone); u.File != lone || !u.mozilla() {
		t.Fatalf("date of the file: %+v", u)
	}
	if u := mozillaDateOf(filepath.Join(dir, "example-app-1.exe")); u.known() {
		t.Errorf("a program: %+v", u)
	}
	tp := &appsContext{scoopApps: map[string]scoopManifest{}, allDrives: true}
	c := newChecker(nopReporter{}, nil)
	signed := time.Date(2026, 10, 18, 9, 0, 0, 0, time.Local)
	c.dated = func(kind string, r result) lastUpdate {
		if r.Exe != "" { // the program of a software
			return lastUpdate{Time: signed, Src: dateSigned, File: r.Exe}
		}
		return tp.dated(kind, r)
	}
	finish := func(name string, r result) Item {
		var got Item
		c.begin(GroupApps, "bundle", name, dir, func(it *Item) { c.finish(it, r); got = *it })
		return got
	}
	alone := finish("Certificate bundle", result{Status: StWarn, Summary: "Outdated", File: lone})
	other := finish("Certificate bundle", result{Status: StWarn, Summary: "Outdated too", File: plain}) // not merged with the first
	exe := filepath.Join(dir, "example-app-1.exe")
	os.WriteFile(exe, []byte("MZ"), 0o644)
	owned := finish("Example App 1", result{Status: StWarn, File: lone, Exe: exe})

	if !alone.Updated.mozilla() || appCell(alone, colUpdated) != shortDate(alone.Data.Time) {
		t.Errorf("on its own: %+v, list %q", alone.Updated, appCell(alone, colUpdated))
	}
	if other.Updated.shown() || other.Data.known() || appCell(other, colUpdated) != "" {
		t.Errorf("without curl's line: %+v %+v", other.Updated, other.Data)
	}
	if owned.Updated.Src != dateSigned || !owned.Data.mozilla() {
		t.Errorf("of a software: %+v %+v", owned.Updated, owned.Data)
	}
	line := "Certificate data from Mozilla as of: 25 September 2026"
	rep := strings.ReplaceAll(buildReport(c, ""), "\r\n", "\n")
	if n := strings.Count(rep, line); n != 2 {
		t.Errorf("report: %d times %q:\n%s", n, line, rep)
	}
	if !strings.Contains(rep, "file: "+lone+"\n      "+line) {
		t.Errorf("report: the line is not under the file:\n%s", rep)
	}
	if n := strings.Count(rep, "Software date: "); n != 1 { // the software's only
		t.Errorf("report: %d software dates:\n%s", n, rep)
	}
	for _, it := range []Item{alone, owned} {
		if text := blocksText(detailBlocks(it)); !strings.Contains(text, line) {
			t.Errorf("details of %s without %q:\n%s", it.Name, line, text)
		}
	}
	if text := blocksText(detailBlocks(alone)); strings.Contains(text, "Software date") {
		t.Errorf("details of a bundle on its own with a software date:\n%s", text)
	}
	Lang = 1
	defer func() { Lang = 0 }()
	if got := alone.Data.text(""); got != "Date des données de certificats issues de Mozilla\u00a0: 25 septembre 2026" {
		t.Errorf("French: %q", got)
	}
}

// The web search for a bundle: its software, for its own copy; else the software the
// list shows with the file's name; for a bundle of no software, its name alone (not
// the folder it is in: "Downloads cacert.pem", "certifi cacert.pem").
func TestBundleSearch(t *testing.T) {
	root := t.TempDir()
	tool := filepath.Join(root, "Tools", "example-app-2", "lib", "site-packages", "certifi", "cacert.pem")
	alone := filepath.Join(root, "Downloads", "cacert-2026-09-25.pem")
	for _, c := range []struct {
		owner, p string
		bundled  bool
		want     string
	}{
		{"Example App 1", filepath.Join(root, "Example App 1", "cacert.pem"), true, "Example App 1"},
		{"", tool, false, "example-app-2 cacert.pem"},
		{"", alone, false, "cacert-2026-09-25.pem"},
		{"Python 3.14", tool, false, "Python 3.14 cacert.pem"},
	} {
		if got := bundleSearch(c.owner, c.p, c.bundled); got != c.want {
			t.Errorf("%s: %q, want %q", c.p, got, c.want)
		}
	}
}
