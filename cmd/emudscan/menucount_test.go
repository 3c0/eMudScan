package main

import (
	"strings"
	"testing"
)

// The menu counts each file, folder, homepage, search and path once: Inkscape's
// certifi, and two findings of example-app-1 in the same folder.
func TestCountMenu(t *testing.T) {
	inkscape := Item{Program: "Inkscape", File: `C:\Program Files\Inkscape\lib\certifi\cacert.pem`, Folder: `C:\Program Files\Inkscape`,
		ProductURL: "https://inkscape.org/", Bundled: true, Owner: "Inkscape", OwnerURL: "https://inkscape.org/", Recheck: true}
	bundle := Item{Program: "example-app-1", File: `C:\scoop\apps\example-app-1\current\cacert.pem`, Folder: `C:\scoop\apps\example-app-1\current`,
		ProductURL: "https://app1.example.org/", Recheck: true,
		Copies: []Item{{Program: "example-app-1", File: `C:\scoop\apps\example-app-1\1.0.9\cacert.pem`, Folder: `C:\scoop\apps\example-app-1\1.0.9`}}}
	exe := Item{Program: "example-app-1", File: `C:\scoop\apps\example-app-1\current\example-app-1.exe`, Folder: `C:\scoop\apps\example-app-1\current`,
		ProductURL: "https://app1.example.org/", Search: "Example App 1"} // the name in its file
	got := countMenu([]Item{inkscape, bundle, exe})
	want := menuCounts{rows: 3, files: 3, folders: 2, homepages: 2, rechecks: 2, paths: 7}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
	// one search per software: example-app-1's program and bundle search for its name; alone,
	// an item searches for its own text
	if q := strings.Join(searchQueries([]Item{inkscape, bundle, exe}), "|"); q != "Inkscape|example-app-1" {
		t.Errorf("searches: %s", q)
	}
	if q := strings.Join(searchQueries([]Item{exe}), "|"); q != "Example App 1" {
		t.Errorf("one item: %s", q)
	}
	if q := strings.Join(searchQueries([]Item{exe, {Program: "EXAMPLE-APP-1", Search: "Example App 1"}}), "|"); q != "Example App 1" {
		t.Errorf("same text: %s", q)
	}
	defer func(l int) { Lang = l }(Lang)
	Lang = 0
	if s := menuLabel("menu.openFolder", "menu.openFolderN", want.folders); s != "Open 2 software folders" {
		t.Errorf("several: %q", s)
	}
	if s := menuLabel("menu.openFolder", "menu.openFolderN", 1); s != "Open software folder" {
		t.Errorf("one: %q", s)
	}
	if s := menuLabel("menu.homepage", "menu.homepageN", 0); s != "Open homepage" {
		t.Errorf("none: %q", s)
	}
	Lang = 1
	if s := menuLabel("menu.details", "menu.detailsN", 3); s != "Informations détaillées sur 3 éléments…" {
		t.Errorf("French: %q", s)
	}
}

// Search on Google says what it searches for: up to three, then how many more; a &
// is doubled, so that the menu shows it.
func TestSearchLabel(t *testing.T) {
	defer func(l int) { Lang = l }(Lang)
	Lang = 0
	eleven := []string{"Example App 4", "example-app-3", "Example Tool", "4", "5", "6", "7", "8", "9", "10", "11"}
	for _, c := range []struct {
		qs   []string
		want string
	}{
		{nil, "Search on Google"},
		{[]string{"cacert-2026-09-25.pem"}, "Search on Google: “cacert-2026-09-25.pem”"},
		{[]string{"Example App 2", "example-app-3"}, "Search on Google: “Example App 2” and “example-app-3”"},
		{[]string{"Example App 2", "example-app-3", "Example Tool"}, "Search on Google: “Example App 2”, “example-app-3” and “Example Tool”"},
		{eleven, "Search on Google: “Example App 4”, “example-app-3”, “Example Tool” and 8 more"},
		{[]string{"Notes & Tasks"}, "Search on Google: “Notes && Tasks”"},
	} {
		if got := searchLabel(c.qs); got != c.want {
			t.Errorf("%v: %q, want %q", c.qs, got, c.want)
		}
	}
	Lang = 1
	if got := searchLabel([]string{"Example App 2", "example-app-3"}); got != "Rechercher sur Google\u00a0: «\u00a0Example App 2\u00a0» et «\u00a0example-app-3\u00a0»" {
		t.Errorf("French: %q", got)
	}
	if got := searchLabel(eleven); got != "Rechercher sur Google\u00a0: «\u00a0Example App 4\u00a0», «\u00a0example-app-3\u00a0», «\u00a0Example Tool\u00a0» et 8 autres" {
		t.Errorf("French, more: %q", got)
	}
}
