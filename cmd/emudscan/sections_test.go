package main

import (
	"strings"
	"testing"
)

// The shield of a section is the worst status of its finished elements, a repeated
// finding counted once; none yet: the "pending" icon.
func TestSectionCounts(t *testing.T) {
	items := []Item{
		{ID: 1, Group: GroupWindows, Status: StOK},
		{ID: 2, Group: GroupNet, Status: StOK, Program: "Signal"},
		{ID: 3, Group: GroupNet, Status: StCaution, Program: "Tor Browser"},
		{ID: 4, Group: GroupApps, Status: StInfo},
		{ID: 5, Group: GroupApps, Status: StPending},
		{ID: 6, Group: GroupApps, Status: StWarn, Program: "App", Summary: "outdated", File: `C:\a\cacert.pem`},
		{ID: 7, Group: GroupApps, Status: StWarn, Program: "App", Summary: "outdated", File: `C:\b\cacert.pem`},
	}
	for _, c := range []struct {
		g     Group
		want  statusCounts
		worst Status
	}{
		{GroupWindows, statusCounts{ok: 1}, StOK},
		{GroupNet, statusCounts{ok: 1, caution: 1}, StCaution},
		{GroupApps, statusCounts{info: 1, warn: 1}, StWarn},
	} {
		n := sectionCounts(items, c.g, nil)
		if n != c.want || n.worst() != c.worst {
			t.Errorf("group %d: %+v, worst %v", c.g, n, n.worst())
		}
	}
	// all of them, as the report counts them
	if n := countItems(items, nil); n != (statusCounts{ok: 2, info: 1, caution: 1, warn: 1}) {
		t.Errorf("all: %+v", n)
	}
	// with a search, what it finds
	if n := sectionCounts(items, GroupNet, queryWords("TOR")); n != (statusCounts{caution: 1}) {
		t.Errorf("search: %+v", n)
	}
	if n := sectionCounts(items, GroupNet, queryWords("zzz")); n.total() != 0 {
		t.Errorf("search without result: %+v", n)
	}
	if w := sectionCounts(nil, GroupApps, nil).worst(); w != StPending {
		t.Errorf("no element: %v", w)
	}
	if w := (statusCounts{ok: 3, errs: 1, warn: 2}).worst(); w != StError {
		t.Errorf("worst: %v", w)
	}
}

// At the right end of the top line of a section: how many elements it has, by
// status, those with none left out; nothing while none is finished. When its list
// hides the OK items, they are counted as hidden, in the same place.
func TestSectionText(t *testing.T) {
	defer func() { Lang = 0 }()
	for _, c := range []struct {
		lang   int
		n      statusCounts
		hideOK bool
		want   string
	}{
		{0, statusCounts{}, false, ""},
		{0, statusCounts{ok: 1}, false, "1 item: 1 OK"},
		{0, statusCounts{ok: 5, warn: 7}, false, "12 items: 5 OK · 7 to update"},
		{0, statusCounts{ok: 2, info: 1, caution: 1, warn: 1, errs: 2}, false, "7 items: 2 OK · 1 to update · 1 warning · 2 critical · 1 informational"},
		{1, statusCounts{ok: 5, caution: 2}, false, "7 éléments : 5 OK · 2 avertissements"},
		{1, statusCounts{warn: 1}, false, "1 élément : 1 à mettre à jour"},
		{0, statusCounts{ok: 5, warn: 7}, true, "12 items: 5 OK (hidden) · 7 to update"},
		{0, statusCounts{ok: 1}, true, "1 item: 1 OK (hidden)"},
		{0, statusCounts{warn: 2}, true, "2 items: 2 to update"},
		{1, statusCounts{ok: 4, caution: 1}, true, "5 éléments : 4 OK (masqués) · 1 avertissement"},
	} {
		Lang = c.lang
		if got := c.n.sectionText(c.hideOK); got != c.want {
			t.Errorf("%+v %v: %q, want %q", c.n, c.hideOK, got, c.want)
		}
	}
	// the summary at the bottom keeps all the counts
	Lang = 0
	if got := (statusCounts{ok: 5}).text("summary.done"); !strings.Contains(got, "5 OK · 0 to update · 0 warnings · 0 critical · 0 informational") {
		t.Errorf("summary: %q", got)
	}
}

// With a search, the list's top line says what it found.
func TestSearchText(t *testing.T) {
	defer func() { Lang = 0 }()
	for _, c := range []struct {
		lang   int
		n      statusCounts
		hideOK bool
		want   string
	}{
		{0, statusCounts{}, false, "Search found no items"},
		{0, statusCounts{warn: 1}, false, "Search found 1 item: 1 to update"},
		{0, statusCounts{ok: 1, caution: 1}, false, "Search found 2 items: 1 OK · 1 warning"},
		{0, statusCounts{ok: 1, caution: 1}, true, "Search found 2 items: 1 OK (hidden) · 1 warning"},
		{1, statusCounts{}, false, "La recherche n’a trouvé aucun élément"},
		{1, statusCounts{ok: 2, warn: 1}, false, "La recherche a trouvé 3 éléments : 2 OK · 1 à mettre à jour"},
		{1, statusCounts{ok: 1}, true, "La recherche a trouvé 1 élément : 1 OK (masqué)"},
	} {
		Lang = c.lang
		if got := c.n.searchText(c.hideOK); got != c.want {
			t.Errorf("%+v %v: %q, want %q", c.n, c.hideOK, got, c.want)
		}
	}
}
