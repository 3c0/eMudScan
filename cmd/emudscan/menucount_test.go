package main

import "testing"

// The menu counts each file, folder, homepage, search and path once: Inkscape's
// certifi, and two findings of example-app-1 in the same folder.
func TestCountMenu(t *testing.T) {
	inkscape := Item{Program: "Inkscape", File: `C:\Program Files\Inkscape\lib\certifi\cacert.pem`, Folder: `C:\Program Files\Inkscape`,
		ProductURL: "https://inkscape.org/", Bundled: true, Owner: "Inkscape", OwnerURL: "https://inkscape.org/", Recheck: true}
	bundle := Item{Program: "example-app-1", File: `C:\scoop\apps\example-app-1\current\cacert.pem`, Folder: `C:\scoop\apps\example-app-1\current`,
		ProductURL: "https://app1.example.org/", Recheck: true,
		Copies: []Item{{Program: "example-app-1", File: `C:\scoop\apps\example-app-1\1.0.9\cacert.pem`, Folder: `C:\scoop\apps\example-app-1\1.0.9`}}}
	exe := Item{Program: "example-app-1", File: `C:\scoop\apps\example-app-1\current\example-app-1.exe`, Folder: `C:\scoop\apps\example-app-1\current`,
		ProductURL: "https://app1.example.org/"}
	got := countMenu([]Item{inkscape, bundle, exe})
	want := menuCounts{rows: 3, files: 3, folders: 2, homepages: 2, searches: 2, rechecks: 2, paths: 7}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
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
