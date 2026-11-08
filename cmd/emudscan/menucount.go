package main

// The right-click menu of the lists says how many things each action acts on, once
// several items are selected: "Open 3 file locations". What appears several times
// (two items in the same folder) is counted once, as the action opens or copies it
// once.

// menuCounts are the numbers of the right-click menu for the selected items.
type menuCounts struct {
	rows      int // the items: Detailed information, Copy
	files     int // the different files: Open file location
	folders   int // the different folders: Open software folder
	homepages int // the different homepages: Open homepage
	searches  int // the different searches: Search on Google
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
	n.searches = distinct(func(it Item) []string { return []string{it.SearchText()} }, items)
	n.paths = distinct(func(it Item) []string { return []string{it.File, it.Folder} }, append(append([]Item(nil), items...), copiesOf(items)...))
	for _, it := range items {
		if it.Recheck {
			n.rechecks++
		}
	}
	return n
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
