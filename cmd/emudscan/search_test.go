package main

import "testing"

func TestMatchesQuery(t *testing.T) {
	Lang = 0
	it := Item{Name: "Node.js", Version: "25.5.0", Owner: "LM Studio", Status: StWarn, Summary: "Release line without the fix",
		File: `C:\Users\me\.lmstudio\.internal\utils\node.exe`, Packages: []pkgRef{{Manager: "Scoop", ID: "lmstudio"}}}
	it.fillDisplay("Node.js")
	it.Copies = []Item{{File: `C:\Program Files\LM Studio\resources\app\.webpack\bin\node.exe`}}
	for q, want := range map[string]bool{
		"":                  true,
		"lm studio":         true, // both words, anywhere
		"NODE 25.5":         true, // any case
		"update needed":     true, // the status
		"webpack":           true, // a copy's path
		"scoop":             true, // the package manager
		"python":            false,
		"studio python":     false, // every word must be found
		"without the fix":   true,
		`.internal\utils`:   true,
		"release candidate": false,
	} {
		if got := matchesQuery(it, queryWords(q)); got != want {
			t.Errorf("query %q: %v, want %v", q, got, want)
		}
	}
}
