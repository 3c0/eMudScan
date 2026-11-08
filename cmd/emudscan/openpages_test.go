package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// A browser of its own opens several pages; a tool of Windows, a relative path or
// another kind of file does not; only web addresses are given to it.
func TestPagesBrowser(t *testing.T) {
	abs := func(p string) string { // a full path: from C:\ on Windows, from / elsewhere (drivePath)
		if filepath.Separator == '\\' {
			return "C:" + filepath.FromSlash(p)
		}
		return p
	}
	root := abs("/win")
	for _, c := range []struct {
		exe  string
		want bool
	}{
		{abs("/apps/Mozilla Firefox/firefox.exe"), true},
		{abs("/apps/Edge/MSEDGE.EXE"), true},
		{abs("/WIN/System32/rundll32.exe"), false},
		{abs("/win/system32/OpenWith.exe"), false},
		{abs("/window/browser.exe"), true}, // not under /win itself
		{"firefox.exe", false},
		{abs("/apps/browser.cmd"), false},
		{"", false},
	} {
		if got := pagesBrowser(c.exe, root) != ""; got != c.want {
			t.Errorf("%s: %v, want %v", c.exe, got, c.want)
		}
	}
	got := webAddresses([]string{"https://app1.example.org/", "file:///c:/x", "https://a b/", "https://www.example.com/app-2/"})
	if strings.Join(got, " ") != "https://app1.example.org/ https://www.example.com/app-2/" {
		t.Errorf("addresses: %v", got)
	}
}
