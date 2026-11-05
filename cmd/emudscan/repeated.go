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

// shortenPath shortens a path in the middle until it fits: its drive, "…", then as
// much of its end as fits ("C:\…\certifi\cacert.pem"). With keep > 0, it keeps the
// folder of that rank (from 0, the drive) before its end when it can, such as the
// folder that tells two copies apart ("C:\…\io.example.tool\…\cacert.pem").
func shortenPath(p string, keep int, fits func(string) bool) string {
	if p == "" || fits(p) {
		return p
	}
	parts := strings.Split(p, `\`)
	if len(parts) < 3 {
		return p
	}
	head := parts[0] + `\`
	if keep > 0 && keep < len(parts)-1 {
		kept := head
		if keep > 1 {
			kept += `…\`
		}
		kept += parts[keep] + `\`
		for k := keep + 1; k < len(parts); k++ {
			s := kept + strings.Join(parts[k:], `\`)
			if k > keep+1 {
				s = kept + `…\` + strings.Join(parts[k:], `\`)
			}
			if fits(s) {
				return s
			}
		}
	}
	for k := 2; k < len(parts); k++ {
		if s := head + `…\` + strings.Join(parts[k:], `\`); fits(s) {
			return s
		}
	}
	return `…\` + parts[len(parts)-1]
}

// shortPlaces shortens paths as shortenPath does; paths that would then look the
// same keep the first folder where they differ, so that they stay apart.
func shortPlaces(paths []string, fits func(string) bool) []string {
	out := make([]string, len(paths))
	same := map[string][]int{}
	for i, p := range paths {
		out[i] = shortenPath(p, -1, fits)
		same[out[i]] = append(same[out[i]], i)
	}
	for _, rows := range same {
		if len(rows) < 2 {
			continue
		}
		split := make([][]string, len(rows))
		for n, i := range rows {
			split[n] = strings.Split(paths[i], `\`)
		}
		k := 0 // the first folder where they are not all the same
		for ; ; k++ {
			alike := k < len(split[0])
			for _, s := range split {
				if !alike || k >= len(s) || !strings.EqualFold(s[k], split[0][k]) {
					alike = false
				}
			}
			if !alike {
				break
			}
		}
		for _, i := range rows {
			out[i] = shortenPath(paths[i], k, fits)
		}
	}
	return out
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
	link := func(m Item) string { return lk(m.copyPath()) }
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
