package main

// The list of the Fix window sorts by a click on a column header, as the lists of the
// main window do (clickSort).

import (
	"sort"
	"strings"
)

// Columns of the list of the Fix window.
const (
	fixColSoftware = iota
	fixColComponent
	fixColFix
	fixColPlace // the file to fix, a link
	fixColResult
	numFixCols
)

// fixRowOrder is the order of rows sorted by keys, then by software and component;
// rows that compare equal keep their order. cell is the full text of a cell.
func fixRowOrder(rows []fixRow, cell func(row, col int) string, keys []sortKey) []int {
	order := make([]int, len(rows))
	for i := range order {
		order[i] = i
	}
	all := append(append([]sortKey{}, keys...), sortKey{col: fixColSoftware}, sortKey{col: fixColComponent})
	compare := func(col, i, j int) int {
		a, b := rows[i].Item, rows[j].Item
		switch col {
		case fixColSoftware: // by name, then version, as the main window
			if a.Group != GroupWindows && b.Group != GroupWindows {
				return compareCol(colProgram, a, b)
			}
		case fixColComponent:
			return compareCol(colComponent, a, b)
		}
		x, y := lower(cell(i, col)), lower(cell(j, col))
		if (x == "") != (y == "") { // empty cells last
			if x == "" {
				return 1
			}
			return -1
		}
		return strings.Compare(x, y)
	}
	sort.SliceStable(order, func(p, q int) bool {
		i, j := order[p], order[q]
		for _, k := range all {
			c := compare(k.col, i, j)
			if k.desc {
				c = -c
			}
			if c != 0 {
				return c < 0
			}
		}
		return false
	})
	return order
}
