package main

import "testing"

func TestMatchesQuery(t *testing.T) {
	Lang = 0
	it := Item{Name: "Node.js", Version: "25.1.0", Owner: "Example App 1", OwnerVer: "1.2.3+1", Status: StWarn, Summary: "Release line without the fix",
		File: `C:\Users\me\.example-app-1\tools\node.exe`, Packages: []pkgRef{{Manager: "Scoop", ID: "example-app-1"}}}
	it.fillDisplay("Node.js")
	it.Copies = []Item{{File: `C:\Program Files\Example App 1\resources\app\bin\node.exe`}}
	for q, want := range map[string]bool{
		"":              true,
		"example app":   true,  // both words, anywhere
		"NODE 25.1":     true,  // any case, with the component's version
		"1.2.3":         true,  // the software's version
		"scoop":         true,  // the package manager, shown with the software
		"xampl":         true,  // inside a word
		"app python":    false, // every word must be found
		"python":        false,
		"update needed": false, // the status is not searched
		"without":       false, // nor the details
		"resources":     false, // nor the paths
		"users":         false,
	} {
		if got := matchesQuery(it, queryWords(q)); got != want {
			t.Errorf("query %q: %v, want %v", q, got, want)
		}
	}
}

// "do" keeps the rows that show it (dotnet, Git for Windows), not those where it is
// only in a path or the details; "app-" finds example-app-3 and example-app-4.
func TestSearchShownColumns(t *testing.T) {
	Lang = 0
	rows := []Item{
		{Name: ".NET SDK code-signing roots", Owner: "dotnet", Status: StInfo, File: `C:\Program Files\dotnet\sdk\10.0.403\trustedroots\codesignctl.pem`},
		{Kind: "app", Name: "Example App 2", Version: "2.5.0", Status: StOK, Summary: "Copies the Windows root store: not affected", Folder: `C:\Program Files\Example App 2`},
		{Kind: "git", Name: "Git for Windows", Version: "2.56.0.windows.3", Owner: "Git", OwnerVer: "2.56.0.3", Status: StOK, Folder: `C:\Program Files\Git\cmd`},
		{Name: "pip", Version: "26.2.1", Owner: "example-app-3", Status: StInfo, File: `D:\Work\example-app-3\.venv\Lib\site-packages\pip\_vendor\certifi\cacert.pem`},
		{Name: "Python", Version: "3.12.16", Owner: "uv", OwnerVer: "0.13.6", Status: StOK, Folder: `C:\Users\me\scoop\persist\uv\python\versions\cpython-3.12.16-windows-x86_64-none`},
		{Kind: "tool", Name: "example-app-4", Owner: "example-app-4", OwnerVer: "4.0.1", Status: StWarn, File: `C:\Users\me\scoop\apps\example-app-4\4.0.1\example-app-4.exe`},
	}
	for i := range rows {
		rows[i].fillDisplay("")
	}
	for q, want := range map[string]string{
		"do":      ".NET SDK code-signing roots|Git for Windows",
		"app-":    "pip|example-app-4",
		"windows": "Git for Windows",
		"vendor":  "",
		"0.13":    "Python",
	} {
		got := ""
		for _, it := range rows {
			if matchesQuery(it, queryWords(q)) {
				if got != "" {
					got += "|"
				}
				got += it.Name
			}
		}
		if got != want {
			t.Errorf("query %q: %q, want %q", q, got, want)
		}
	}
}
