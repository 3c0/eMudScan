package main

// Repeated findings: the same finding of the same software in several places (LM
// Studio carries two copies of Node.js 25.5.0) is shown as one element, which lists
// every copy. Each copy is still checked on its own, and "Scan again" checks them
// all again; copies that no longer agree are shown apart again.

import "strings"

// repeatKey: elements with the same key are the same finding. Elements being
// checked, the Windows checks and the summaries are never merged.
func (it Item) repeatKey() string {
	if it.Group == GroupWindows || it.Status == StPending || it.Kind == "info" || it.File == "" && it.Folder == "" {
		return ""
	}
	return strings.Join([]string{itoa(int(it.Group)), lower(it.Program), lower(it.ProgVersion), lower(it.Component),
		lower(it.Version), itoa(int(it.Status)), it.Summary}, "\x00")
}

// mergeRepeated returns items with each repeated finding merged into its first
// element, whose Copies are the others. The order of items is kept.
func mergeRepeated(items []Item) []Item {
	at := map[string]int{}
	var out []Item
	for _, it := range items {
		it.Copies = nil
		if k := it.repeatKey(); k != "" {
			if i, ok := at[k]; ok {
				out[i].Copies = append(out[i].Copies, it)
				continue
			}
			at[k] = len(out)
		}
		out = append(out, it)
	}
	return out
}

// memberIDs are the elements an element stands for: itself and its copies.
func (it Item) memberIDs() []int {
	ids := []int{it.ID}
	for _, c := range it.Copies {
		ids = append(ids, c.ID)
	}
	return ids
}

// copyPath is the place of one copy: its file, else its folder.
func (it Item) copyPath() string { return firstNonEmpty(it.File, it.Folder) }

// shownDetails is the explanation of an element. A merged element lists its copies
// first; their explanation follows once when it is the same for all, else for each.
func (it Item) shownDetails() string {
	if len(it.Copies) == 0 {
		return it.Details
	}
	all := append([]Item{it}, it.Copies...)
	link := func(m Item) string { p := m.copyPath(); return "[[" + p + "|" + p + "]]" }
	var list []string
	for _, m := range all {
		list = append(list, "• "+link(m))
	}
	intro := Tf("copies.found", len(all)) + "\n" + strings.Join(list, "\n")
	same := true
	for _, m := range it.Copies {
		if m.Details != it.Details {
			same = false
			break
		}
	}
	if same {
		return joinParas(intro, it.Details)
	}
	parts := []string{intro}
	for _, m := range all {
		parts = append(parts, Tf("copies.in", link(m))+"\n"+m.Details)
	}
	return joinParas(parts...)
}
