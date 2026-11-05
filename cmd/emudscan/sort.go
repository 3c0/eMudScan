package main

import (
	"sort"
	"strings"
)

// Columns of the program lists.
const (
	colProgram = iota // software name, version and package manager
	colComponent
	colUpdated // last update of the software by its publisher
	colStatus
	colFile
	colFolder
	colDetails
	numAppCols
)

var appColKeys = [numAppCols]string{"col.program", "col.component", "col.updated", "col.status", "col.file", "col.folder", "col.details"}

// appCell is the full text of a cell of a program list.
func appCell(it Item, col int) string {
	switch col {
	case colProgram:
		return it.DisplayName()
	case colComponent:
		if it.Component == "" {
			return ""
		}
		return strings.TrimSpace(it.Component + " " + it.Version)
	case colUpdated:
		if !it.Updated.known() {
			return ""
		}
		if m := it.stale(); m > 0 { // with its age, in amber (see ui_windows.go)
			return shortDate(it.Updated.Time) + " · " + ageText(m)
		}
		return shortDate(it.Updated.Time)
	case colStatus:
		return statusText(it.Status)
	case colFile:
		if it.Status == StOK {
			return "" // nothing outdated (the details name the file that was checked)
		}
		if len(it.Copies) > 0 && it.File != "" {
			return it.File + " " + Tf("copies.more", len(it.Copies))
		}
		return it.File
	case colFolder:
		return it.Folder
	case colDetails:
		return it.Summary
	}
	return ""
}

type sortKey struct {
	col  int
	desc bool
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// compareCol compares two items on one column. Empty texts sort last.
func compareCol(col int, a, b Item) int {
	switch col {
	case colStatus:
		return cmpInt(a.Status.severity(), b.Status.severity()) // worst first
	case colUpdated:
		if a.Updated.known() != b.Updated.known() {
			if a.Updated.known() {
				return -1
			}
			return 1
		}
		return a.Updated.Time.Compare(b.Updated.Time)
	case colProgram:
		if c := strings.Compare(lower(a.Program), lower(b.Program)); c != 0 {
			return c
		}
		if c := ParseVersion(a.ProgVersion).Cmp(ParseVersion(b.ProgVersion)); c != 0 {
			return c
		}
	case colComponent:
		if c := strings.Compare(lower(a.Component), lower(b.Component)); c != 0 || a.Component == "" {
			if (a.Component == "") != (b.Component == "") {
				if a.Component == "" {
					return 1
				}
				return -1
			}
			return c
		}
		return ParseVersion(a.Version).Cmp(ParseVersion(b.Version))
	}
	x, y := lower(appCell(a, col)), lower(appCell(b, col))
	if (x == "") != (y == "") {
		if x == "" {
			return 1
		}
		return -1
	}
	return strings.Compare(x, y)
}

// sortItems sorts by the given keys, then by status, software, component and order of discovery.
func sortItems(items []Item, keys []sortKey) {
	all := append(append([]sortKey{}, keys...), sortKey{col: colStatus}, sortKey{col: colProgram}, sortKey{col: colComponent})
	sort.SliceStable(items, func(i, j int) bool {
		for _, k := range all {
			c := compareCol(k.col, items[i], items[j])
			if k.desc {
				c = -c
			}
			if c != 0 {
				return c < 0
			}
		}
		return items[i].ID < items[j].ID
	})
}

// clickSort updates the sort keys after a click on a column header:
// the clicked column becomes the first key and the previous first key the second.
func clickSort(keys []sortKey, col int) []sortKey {
	if len(keys) > 0 && keys[0].col == col {
		keys[0].desc = !keys[0].desc
		return keys
	}
	out := []sortKey{{col: col}}
	if len(keys) > 0 {
		out = append(out, keys[0])
	}
	return out
}
