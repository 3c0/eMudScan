package main

import (
	"path/filepath"
	"strings"
)

// Several web pages (the homepages or the Google searches of the selected software)
// open at once in the default browser, started once with all of them, as tabs: a
// browser started once per page asks again for a profile if it asks at start
// (Firefox), and may lose pages that arrive together. The program is the one Windows
// gives for https addresses (the way Raymond Chen of Microsoft describes, in The Old
// New Thing, 14 September 2015); browsers take several addresses on their command
// line. Without such a program, the pages open one by one, as Windows opens a link.

// pagesBrowser is the default browser that opens several pages: exe, a program of its
// own; "" for none, or for a tool of Windows (under systemRoot) that some defaults
// name, such as rundll32.exe or OpenWith.exe, which would not take several addresses.
func pagesBrowser(exe, systemRoot string) string {
	if !drivePath(exe) || !strings.EqualFold(filepath.Ext(exe), ".exe") {
		return ""
	}
	if root := strings.TrimRight(systemRoot, `\/`); root != "" && strings.HasPrefix(lower(exe), lower(root)+string(filepath.Separator)) {
		return ""
	}
	return exe
}

// webAddresses are the addresses that may open: web addresses (webAddress).
func webAddresses(urls []string) []string {
	var out []string
	for _, u := range urls {
		if webAddress(u) {
			out = append(out, u)
		}
	}
	return out
}
