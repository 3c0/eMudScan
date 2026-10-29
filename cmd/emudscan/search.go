package main

// The search box above the lists: an element is shown when every word typed is
// found, in any order and case, in its software, component, versions, status,
// summary, owner, package, file or folder — or in those of one of its copies.

import "strings"

// queryWords splits what was typed into words to find.
func queryWords(q string) []string { return strings.Fields(lower(q)) }

func (it Item) searchText() string {
	parts := []string{it.Name, it.Program, it.ProgVersion, it.Component, it.Version, statusText(it.Status), it.Summary,
		it.Owner, it.File, it.Folder}
	for _, p := range it.Packages {
		parts = append(parts, p.Manager, p.ID)
	}
	for _, c := range it.Copies {
		parts = append(parts, c.File, c.Folder)
	}
	return lower(strings.Join(parts, "\x00"))
}

// matchesQuery: every word is found in the element.
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
