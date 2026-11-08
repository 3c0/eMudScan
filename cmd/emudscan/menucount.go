package main

// The right-click menu of the lists says how many things each action acts on, once
// several items are selected: "Open 3 file locations". What appears several times
// (two items in the same folder) is counted once, as the action opens or copies it
// once; Search on Google searches once per software (searchQueries).

import "strings"

// menuCounts are the numbers of the right-click menu for the selected items.
type menuCounts struct {
	rows      int // the items: Detailed information, Copy
	files     int // the different files: Open file location
	folders   int // the different folders: Open software folder
	homepages int // the different homepages: Open homepage
	searches  int // the software to search for: Search on Google (searchQueries)
	rechecks  int // the items that can be scanned again: Scan again
	paths     int // the different files and folders, with the other places of a finding: Copy paths
}

// countMenu counts what the actions of the menu act on for items.
func countMenu(items []Item) menuCounts {
	n := menuCounts{rows: len(items)}
	distinct := func(values func(Item) []string, from []Item) int {
		seen := map[string]bool{}
		for _, it := range from {
			for _, v := range values(it) {
				if v != "" {
					seen[v] = true
				}
			}
		}
		return len(seen)
	}
	n.files = distinct(func(it Item) []string { return []string{it.File} }, items)
	n.folders = distinct(func(it Item) []string { return []string{it.Folder} }, items)
	n.homepages = distinct(func(it Item) []string { return []string{it.Homepage()} }, items)
	n.searches = len(searchQueries(items))
	n.paths = distinct(func(it Item) []string { return []string{it.File, it.Folder} }, append(append([]Item(nil), items...), copiesOf(items)...))
	for _, it := range items {
		if it.Recheck {
			n.rechecks++
		}
	}
	return n
}

// searchQueries are the searches of Search on Google for items: one per software.
// An item searches for its own text (SearchText); the items of a software that search
// for different texts, such as its program ("Example App 1", the name in its file) and its
// certificate bundle ("example-app-1"), search for the name of the software.
func searchQueries(items []Item) []string {
	type software struct {
		name  string
		texts []string
	}
	var keys []string
	soft := map[string]*software{}
	for _, it := range items {
		q := it.SearchText()
		if q == "" {
			continue
		}
		key := "program:" + strings.ToLower(it.Program)
		if it.Program == "" {
			key = "search:" + strings.ToLower(q)
		}
		s := soft[key]
		if s == nil {
			s = &software{name: it.Program}
			soft[key] = s
			keys = append(keys, key)
		}
		known := false
		for _, t := range s.texts {
			known = known || strings.EqualFold(t, q)
		}
		if !known {
			s.texts = append(s.texts, q)
		}
	}
	var out []string
	seen := map[string]bool{}
	for _, k := range keys {
		s := soft[k]
		q := s.texts[0]
		if len(s.texts) > 1 && s.name != "" {
			q = s.name
		}
		if !seen[strings.ToLower(q)] {
			seen[strings.ToLower(q)] = true
			out = append(out, q)
		}
	}
	return out
}

// menuLabel is the label of a menu entry that acts on n things: the label for one
// (also when there are none, as the entry is then greyed), or, for several, the label
// that gives their number.
func menuLabel(one, several string, n int) string {
	if n >= 2 {
		return Tf(several, n)
	}
	return T(one)
}

// copiesOf: the other copies of merged elements.
func copiesOf(items []Item) []Item {
	var out []Item
	for _, it := range items {
		out = append(out, it.Copies...)
	}
	return out
}
