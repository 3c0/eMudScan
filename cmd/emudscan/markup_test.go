package main

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestMarkup(t *testing.T) {
	got := parseLine(`Folder: [[C:\Program Files\Git]] see https://gitforwindows.org. and **bold**`)
	want := []mrun{
		{runBold, "Folder:", ""},
		{runText, " ", ""},
		{runLink, `C:\Program Files\Git`, `C:\Program Files\Git`},
		{runText, " see ", ""},
		{runLink, "https://gitforwindows.org", "https://gitforwindows.org"},
		{runText, ". and ", ""},
		{runBold, "bold", ""},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v", got)
	}
	if r := parseLine("Que faire : mettez à jour"); r[0].text != "Que faire :" {
		t.Errorf("french label: %#v", r)
	}
	if r := parseLine(`C:\x: y`); r[0].kind == runBold {
		t.Errorf("path taken as label: %#v", r)
	}
	if r := parseLine("$ git update-git-for-windows"); r[0].kind != runCommand || r[0].text != "git update-git-for-windows" {
		t.Errorf("command: %#v", r)
	}
	if p := plainText("A [[x]]\n$ cmd **b**"); p != "A x\n    cmd **b**" {
		t.Errorf("plain: %q", p)
	}
}

func TestMarkupBlocks(t *testing.T) {
	bs := markupBlocks("@ Version\t2.55\n@ Backend\topenssl\n\nText with [[a link|https://x.org]].\n$ cmd one\n$ cmd two\n• item\n\n\n")
	kinds := []docKind{}
	for _, b := range bs {
		kinds = append(kinds, b.kind)
	}
	want := []docKind{dkFields, dkSpace, dkPara, dkCmds, dkBullet}
	if fmt.Sprint(kinds) != fmt.Sprint(want) {
		t.Fatalf("kinds %v, want %v", kinds, want)
	}
	if len(bs[0].rows) != 2 || bs[0].rows[1].label != "Backend" || len(bs[3].cmds) != 2 {
		t.Errorf("blocks: %+v", bs)
	}
	var link mrun
	for _, r := range bs[2].runs {
		if r.kind == runLink {
			link = r
		}
	}
	if link.text != "a link" || link.target != "https://x.org" {
		t.Errorf("labelled link: %+v", link)
	}
	if got := plainText("@ Version\t2.55"); got != "Version: 2.55" {
		t.Errorf("plain field: %q", got)
	}
}

func TestLabels(t *testing.T) {
	if r := parseLine("What to do: update it."); len(r) == 0 || r[0].kind != runBold {
		t.Errorf("label: %+v", r)
	}
	if r := parseLine(Tf("fix.pkgAll", "Scoop")); len(r) == 0 || r[0].kind == runBold {
		t.Errorf("a sentence is not a label: %+v", r)
	}
}

func TestSearchRowAndTestLinks(t *testing.T) {
	it := Item{Group: GroupNet, Kind: "browser", Name: "ungoogled-chromium", Status: StWarn,
		Details: browserTests("ungoogled-chromium", `C:\x\chrome.exe`)}
	it.fillDisplay("")
	bs := detailBlocks(it)
	text := blocksText(bs)
	if !strings.Contains(text, Tf("dlg.searchGoogle", "ungoogled-chromium")+" — https://www.google.com/search?q=ungoogled-chromium") {
		t.Errorf("search row:\n%s", text)
	}
	for _, b := range bs {
		if b.kind != dkBullet {
			continue
		}
		if len(b.runs) != 2 || b.runs[0].kind != runLink || b.runs[0].text != b.runs[0].target[strings.Index(b.runs[0].target, ">")+1:] || b.runs[1].kind != runText {
			t.Errorf("test link: %+v", b.runs)
		}
	}
	if s := plainText(Tf("py.sslStore", 6, 12, refWinCopy())); strings.Contains(s, "[[") || !strings.Contains(s, T("ref.winCopy")) {
		t.Errorf("reference: %s", s)
	}
}
