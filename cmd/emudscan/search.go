package main

// The search box above the lists: an element is shown when every word typed is
// found, in any order and case, in its Software or Component column as the list shows
// them (the software with its version and package manager, the component with its
// version). "check" finds ucheck and xpu_check. Paths, statuses and details are left
// out: a row must not be kept for a word the list does not show ("do" in a hidden
// part of a path, in "Windows" or "_vendor").

import "strings"

// queryWords splits what was typed into words to find.
func queryWords(q string) []string { return strings.Fields(lower(q)) }

func (it Item) searchText() string {
	return lower(appCell(it, colProgram) + "\x00" + appCell(it, colComponent))
}

// matchesQuery: every word is found in the element's Software or Component.
func matchesQuery(it Item, words []string) bool {
	if len(words) == 0 {
		return true
	}
	text := it.searchText()
	for _, w := range words {
		if !strings.Contains(text, w) {
			return false
		}
	}
	return true
}
