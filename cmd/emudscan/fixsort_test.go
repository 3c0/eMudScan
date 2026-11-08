package main

import (
	"fmt"
	"testing"
)

// The rows of the Fix window sort by a column, then by software and component; equal
// rows keep their order, and empty cells come last.
func TestFixRowOrder(t *testing.T) {
	app := func(id int, program, version, component string) fixRow {
		return fixRow{Item: Item{ID: id, Group: GroupApps, Program: program, ProgVersion: version, Component: component}}
	}
	rows := []fixRow{app(1, "Example App 4", "4.0.1", ""), app(2, "Example App 2", "2.0.1.200", "Certificate bundle"),
		app(3, "Example App 1", "1.0.1", ""), app(4, "Example App 3", "3.0.2", "Certificate bundle"), app(5, "Example App 3", "3.0.1", "Certificate bundle")}
	results := map[int]string{1: "Not fixed", 3: "Fixed"}
	cell := func(row, col int) string {
		if col == fixColResult {
			return results[rows[row].ID]
		}
		return ""
	}
	ids := func(order []int) string {
		var out []int
		for _, i := range order {
			out = append(out, rows[i].ID)
		}
		return fmt.Sprint(out)
	}
	for _, c := range []struct {
		keys []sortKey
		want string
	}{
		{nil, "[3 2 5 4 1]"}, // by name, then version
		{[]sortKey{{col: fixColSoftware, desc: true}}, "[1 4 5 2 3]"},
		{[]sortKey{{col: fixColResult}}, "[3 1 2 5 4]"},                         // Fixed, Not fixed, then the others by name
		{[]sortKey{{col: fixColComponent}, {col: fixColResult}}, "[2 5 4 3 1]"}, // the empty components last
	} {
		if got := ids(fixRowOrder(rows, cell, c.keys)); got != c.want {
			t.Errorf("%v: %s, want %s", c.keys, got, c.want)
		}
	}
}
