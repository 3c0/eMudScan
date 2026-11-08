//go:build windows

package main

// The Fix window: the PowerShell commands that will run, in a console drawn as a
// PowerShell window, "Back up the files to change", and a button that runs them once the
// user accepts a warning; the console then shows what they do.
//
// Opened from the details of a software ("Run the fix now"), it fixes that software
// (and its copies, when the details show several), without Stop: its commands always
// run to their end; it stays above the details, which are shown again, checked anew,
// when it closes.
//
// Opened from the main window ("Fix the selected software"), it lists every software
// that can be fixed, with its vulnerable component, each with a check box, ticked
// unless its fixes are optional (tickedFirst), above the console, which shows the
// commands of those ticked. A software with optional fixes and others has a row for
// each (fixParts), the optional ones unticked; each row runs its own. A fixed software is
// unticked, so that running the fixes again, after Stop, does not fix it twice. At the
// end, a report is saved in the Downloads folder.

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

const (
	idFixBackup = 401
	idFixRun    = 402
	idFixStop   = 403
	idFixClose  = 404
	idFixLog    = 405
	idFixList   = 406
	idFixAll    = 407
	idFixNone   = 408
	idDlgFix    = 306 // the Fix button of the details
)

type fixDlg struct {
	hwnd, owner                  uintptr
	title                        uintptr // the title of the console
	font                         uintptr // its font, bold
	console                      *logView
	lines                        []fixLine      // what the console shows
	lineAt                       []int          // the line of the console where each starts
	cmdLine                      int            // the line of the console of the command running, or -1
	results                      bool           // the console shows the results of the commands
	titles                       map[int]string // the titles of the elements in the console (fixTitles)
	frame                        rect           // the black frame around the console
	bBackup, bFix, bStop, bClose uintptr
	ids                          []int  // the elements to fix (from the details)
	label                        string // what the window fixes, for the report
	runText, question            string // the button that runs the fixes, and the warning's question (from the details)
	running, ran                 bool
	asking                       bool // the warning is shown
	frontTries                   int  // after the administrator prompt: how long to keep the window in front
	stop                         atomic.Bool

	// from the main window: the list of the software that can be fixed
	list, bAll, bNone uintptr
	count             uintptr      // "3/9 software selected"
	rows              []fixRow     // the rows of the list: a software in one place each
	placeText         []string     // the Location of each row, as shown: shortened to its column (fitPlaces)
	ranRows           map[int]bool // the rows that the last run fixed or tried to
	updating          bool         // the check boxes are being changed by eMudScan
	sortKeys          []sortKey    // how the list is sorted, after clicks on its column headers

	// the bar between the list and the console, which the user drags to make the list
	// taller or shorter
	part         float64 // the share of the list in the room for both; 0 until dragged
	split        rect    // the bar
	room, listH  int32   // the room for both, and the height of the list
	drag         bool
	dragY, fromH int32 // where the drag started, and the height of the list then
}

// nmListViewState is NMLISTVIEW, as LVN_ITEMCHANGED sends it.
type nmListViewState struct {
	Hdr                       nmhdr
	Item, SubItem             int32
	NewState, OldState, Flags uint32
	Pt                        point
	LParam                    uintptr
}

var (
	fixProc     = syscall.NewCallback(fixWndProc)
	fixListProc = syscall.NewCallback(fixListSubclass)
)

// fixItems are the elements of ids as they are now, those that still need their
// fixes: a fixed element has none left, and the Fix buttons are greyed for it.
func (a *ui) fixItems(ids []int) []Item {
	var out []Item
	for _, id := range ids {
		if it, ok := a.items[id]; ok && len(it.Fixes) > 0 && !fixedStatus(it.Status) {
			out = append(out, it)
		}
	}
	return out
}

// restoreItems are the elements of ids as they are now that have backups to restore.
func (a *ui) restoreItems(ids []int) []Item {
	var out []Item
	for _, id := range ids {
		if it, ok := a.items[id]; ok && len(it.Restores) > 0 {
			out = append(out, it)
		}
	}
	return out
}

// fixRows are the software that can be fixed, by name: a finding repeated in
// several places, which the lists show as one row, has a row for each place, with
// its own Location; a software with optional fixes and others, a row for each
// (fixParts).
func (a *ui) fixRows() []fixRow {
	var rows []fixRow
	for _, it := range a.items {
		if len(a.fixItems([]int{it.ID})) > 0 {
			for _, part := range fixParts(it) {
				rows = append(rows, fixRow{it, part})
			}
		}
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID }) // the places of a software in the order found
	sort.SliceStable(rows, func(i, j int) bool { return lower(itemTitle(rows[i].Item)) < lower(itemTitle(rows[j].Item)) })
	return rows
}

// updateFixButton enables the Fix button of the main window once a scan has found
// something to fix, while no scan runs.
func (a *ui) updateFixButton() {
	if a.hFix != 0 {
		a.setButton(a.hFix, T("btn.fix"), glyphFirstAid, !a.running && a.current != nil && len(a.fixRows()) > 0)
	}
}

// openFix opens the Fix window for the element it (with its copies), above owner: the
// details, opened from the main window or from the Fix software window, which then
// waits under them (fixUnder).
func (a *ui) openFix(owner uintptr, it Item) {
	if a.fixBusy() || a.current == nil {
		return
	}
	d := &fixDlg{owner: owner, ids: it.memberIDs(), label: Tf("fix.title", itemTitle(it)), runText: T("fix.run"), question: T("fix.confirm"), cmdLine: -1}
	if len(a.fixItems(d.ids)) == 0 {
		return
	}
	a.fixUnder = a.fix
	if !a.newFixWindow(d, 860, 600) {
		a.fix, a.fixUnder = a.fixUnder, nil
		return
	}
	a.showFix(d)
}

// fixBusy: a Fix window is open, other than the Fix software window under the details
// it opened, from which the software can be fixed, or restored, as from the main window.
func (a *ui) fixBusy() bool {
	return a.fix != nil && (a.dlgOwner == 0 || a.fix.hwnd != a.dlgOwner || a.fix.running || a.fixUnder != nil)
}

// fixDlgOf is the Fix window hwnd, in front or under the details (fixUnder).
func (a *ui) fixDlgOf(hwnd uintptr) *fixDlg {
	for _, d := range []*fixDlg{a.fix, a.fixUnder} {
		if d != nil && d.hwnd == hwnd {
			return d
		}
	}
	return nil
}

// fixDlgOfList is the Fix software window whose list is list.
func (a *ui) fixDlgOfList(list uintptr) *fixDlg {
	for _, d := range []*fixDlg{a.fix, a.fixUnder} {
		if d != nil && list != 0 && d.list == list {
			return d
		}
	}
	return nil
}

// openFixDetails opens the details of the software of a row of the list, above the Fix
// software window, which waits under them: the software can be fixed there too, and
// the list follows once they close (rowsAgain).
func (a *ui) openFixDetails(d *fixDlg, row int) {
	if row < 0 || row >= len(d.rows) || a.dlg != nil || d.running || d.asking {
		return
	}
	it, ok := a.itemFor(d.rows[row].ID)
	if !ok {
		return
	}
	a.dlgOwner = d.hwnd
	a.openDetails(it)
	if a.dlg == nil {
		a.dlgOwner = 0
	}
}

// rowsAgain shows each row of the list as its software is now, once the details that
// the list opened are closed: fixed there, it loses its check box, as after a run;
// restored there, it gets it back, unticked; its fixes, and the commands, follow.
func (d *fixDlg) rowsAgain(a *ui) {
	d.updating = true
	setCell := func(row, col int, text string) {
		sub := lvItem{SubItem: int32(col), Text: u16(text)}
		send(d.list, lvmSetItemTextW, uintptr(row), uintptr(unsafe.Pointer(&sub)))
	}
	for i, row := range d.rows {
		it, ok := a.items[row.ID]
		if !ok {
			continue
		}
		items := d.rowItems(a, i)
		switch {
		case len(items) == 0 && d.hasBox(i):
			d.done(i)
			setCell(i, 4, T("fix.resultFixed"))
		case len(items) > 0 && !d.hasBox(i):
			d.check(i, false)
			setCell(i, 4, "")
		}
		if len(items) > 0 {
			setCell(i, 2, fixSummary(items))
		}
		a.setRow(d.list, i, it, []string{fixSoftware(it)}, false) // its status icon
	}
	d.updating = false
	d.preview(a)
}

// openFixAll opens the Fix window for all the software that can be fixed.
func (a *ui) openFixAll() {
	if a.fix != nil || a.current == nil || a.running {
		return
	}
	d := &fixDlg{owner: a.hwnd, label: T("fix.allTitle"), runText: T("fix.runAll"), rows: a.fixRows(), cmdLine: -1}
	if len(d.rows) == 0 || !a.newFixWindow(d, 1100, 860) {
		return
	}
	d.bAll = a.iconButton(d.hwnd, T("fix.selectAll"), idFixAll, false, glyphNone)
	d.bNone = a.iconButton(d.hwnd, T("fix.selectNone"), idFixNone, false, glyphNone)
	d.count, _, _ = pCreateWindowExW.Call(0, up("STATIC"), 0, uintptr(wsChild|wsVisible|0x80|0x200|0x4000 /* SS_NOPREFIX|SS_CENTERIMAGE|SS_ENDELLIPSIS */),
		0, 0, 10, 10, d.hwnd, 0, 0, 0)
	send(d.count, wmSetFont, a.fontUI, 1)
	// LVS_REPORT|LVS_SINGLESEL|LVS_SHOWSELALWAYS|LVS_SHAREIMAGELISTS
	d.list, _, _ = pCreateWindowExW.Call(wsExClientEdge, up("SysListView32"), 0, uintptr(wsChild|wsVisible|wsTabStop|0x1|0x4|0x8|0x40),
		0, 0, 10, 10, d.hwnd, idFixList, 0, 0)
	pSetWindowTheme.Call(d.list, up("Explorer"), 0)
	send(d.list, wmSetFont, a.fontUI, 1)
	send(d.list, lvmSetExtendedStyle, 0, 0x4|0x20|0x10000) // CHECKBOXES|FULLROWSELECT|DOUBLEBUFFER
	send(d.list, lvmSetImageList, 1, a.imgStatus)
	pSetWindowSubclass.Call(d.list, fixListProc, 1, 0)
	a.addCellTips(d.list)
	for i, key := range []string{"fix.colSoftware", "col.component", "fix.colFix", "fix.colPlace", "fix.colResult"} {
		col := lvColumn{Mask: 0x1 | 0x2 | 0x4 | 0x8, Cx: 100, Text: u16(T(key)), SubItem: int32(i)}
		send(d.list, lvmInsertColumnW, uintptr(i), uintptr(unsafe.Pointer(&col)))
	}
	d.updating = true
	for i, r := range d.rows {
		items := d.rowItems(a, i)
		row := a.setRow(d.list, i, r.Item, []string{fixSoftware(r.Item), appCell(r.Item, colComponent), fixSummary(items), r.copyPath(), ""}, true)
		d.check(row, tickedFirst(items))
	}
	d.fitPlaces(a)
	d.updating = false
	a.showFix(d)
}

// fixSoftware is the software of a row of the list, without its component, which has
// its own column, as in the main window.
func fixSoftware(it Item) string {
	if s := appCell(it, colProgram); s != "" && it.Group != GroupWindows {
		return s
	}
	return itemTitle(it)
}

// fixSummary says in one line what the fixes of items do.
func fixSummary(items []Item) string {
	var parts []string
	seen, admin := map[string]bool{}, false
	for _, it := range items {
		for _, f := range it.Fixes {
			admin = admin || f.Admin
			g := f
			g.Admin = false
			s := strings.TrimRight(strings.TrimSuffix(fixIntro(g), ":"), " \u00a0")
			if !seen[s] {
				seen[s] = true
				parts = append(parts, s)
			}
		}
	}
	s := strings.Join(parts, " + ")
	if admin {
		s += " " + T("fix.asAdmin")
	}
	return s
}

// placeOf is the file of a row, which its Location shows as a link.
func (d *fixDlg) placeOf(row int) string {
	if row < 0 || row >= len(d.rows) {
		return ""
	}
	return d.rows[row].copyPath()
}

// fixLinkAt is the file under a point of the list, when the point is on the text of
// a Location.
func (a *ui) fixLinkAt(d *fixDlg, pt point) (string, bool) {
	ht := lvHitTestInfo{Pt: pt}
	if int32(send(d.list, lvmSubItemHitTest, 0, uintptr(unsafe.Pointer(&ht)))) < 0 || ht.Item < 0 || ht.SubItem != fixColPlace {
		return "", false
	}
	row := int(ht.Item)
	target := d.placeOf(row)
	if target == "" {
		return "", false
	}
	shown := target
	if row < len(d.placeText) {
		shown = d.placeText[row]
	}
	rc := rect{Top: ht.SubItem, Left: lvirBounds}
	send(d.list, lvmGetSubItemRect, uintptr(ht.Item), uintptr(unsafe.Pointer(&rc)))
	if pt.X > rc.Left+a.scale(10)+min(a.textWidth(a.fontLink, shown), rc.Right-rc.Left) { // after the text
		return "", false
	}
	return target, true
}

// fitPlaces shortens the Location of each row to its column in the middle, as the
// main window does its paths, but so that the places of a software stay apart
// (shortPlaces).
func (d *fixDlg) fitPlaces(a *ui) {
	n := min(len(d.rows), int(send(d.list, lvmGetItemCount, 0, 0)))
	w := int32(send(d.list, lvmGetColumnWidth, fixColPlace, 0)) - a.scale(14)
	paths := make([]string, n)
	for i := range n {
		paths[i] = d.placeOf(i)
	}
	a.withListDC(d.list, func(dc uintptr) {
		for i, t := range shortPlaces(paths, fitsIn(dc, w)) {
			if i < len(d.placeText) && d.placeText[i] == t {
				continue
			}
			sub := lvItem{SubItem: fixColPlace, Text: u16(t)}
			send(d.list, lvmSetItemTextW, uintptr(i), uintptr(unsafe.Pointer(&sub)))
			if i >= len(d.placeText) {
				d.placeText = append(d.placeText, make([]string, i+1-len(d.placeText))...)
			}
			d.placeText[i] = t
		}
	})
}

// cellTip is the full text of a cell of the list, for its tooltip (cellTipText), and
// whether it is a path, which is shown even when it is not cut.
func (d *fixDlg) cellTip(row, col int) (string, bool) {
	if row < 0 || row >= len(d.rows) {
		return "", false
	}
	if col == fixColPlace {
		return shownPath(d.placeOf(row)), true
	}
	return d.cellText(row, col), false
}

// cellText is the text of a cell of the list, as shown.
func (d *fixDlg) cellText(row, col int) string {
	buf := make([]uint16, 1024)
	li := lvItem{SubItem: int32(col), Text: &buf[0], TextMax: int32(len(buf))}
	send(d.list, lvmGetItemTextW, uintptr(row), uintptr(unsafe.Pointer(&li)))
	return syscall.UTF16ToString(buf)
}

// sortRows sorts the list by its sort keys (fixRowOrder): each row keeps its tick, its
// check box or its absence, its result and its selection; the commands follow the new
// order, unless the console shows the results of a run.
func (d *fixDlg) sortRows(a *ui) {
	order := fixRowOrder(d.rows, func(row, col int) string {
		if col == fixColPlace {
			return d.placeOf(row) // the whole path, not as shortened
		}
		return d.cellText(row, col)
	}, d.sortKeys)
	type rowState struct {
		texts                 []string
		box, ticked, sel, ran bool
	}
	old := make([]rowState, len(d.rows))
	for i := range d.rows {
		texts := make([]string, numFixCols)
		for c := range texts {
			texts[c] = d.cellText(i, c)
		}
		texts[fixColPlace] = d.placeOf(i)
		old[i] = rowState{texts, d.hasBox(i), d.checked(i), send(d.list, lvmGetItemState, uintptr(i), 2) != 0, d.ranRows[i]}
	}
	rows, ran := make([]fixRow, len(order)), map[int]bool{}
	for i, o := range order {
		rows[i], ran[i] = d.rows[o], old[o].ran
	}
	d.rows, d.ranRows, d.placeText = rows, ran, nil
	d.updating = true
	for i, o := range order {
		s := old[o]
		a.setRow(d.list, i, d.rows[i].Item, s.texts, false)
		if s.box {
			d.check(i, s.ticked)
		} else {
			d.done(i)
		}
		li := lvItem{StateMask: 0x3 /* LVIS_FOCUSED|LVIS_SELECTED */}
		if s.sel {
			li.State = 0x3
		}
		send(d.list, lvmSetItemState, uintptr(i), uintptr(unsafe.Pointer(&li)))
	}
	d.fitPlaces(a)
	d.updating = false
	setSortArrows(d.list, numFixCols, d.sortKeys)
	if !d.results {
		d.preview(a)
	}
}

// fixListDraw draws each Location that is a link as the lists of the main window
// draw their paths: in blue, underlined.
func (a *ui) fixListDraw(d *fixDlg, cd *nmLVCustomDraw) uintptr {
	switch cd.DrawStage {
	case 0x1, 0x10001: // CDDS_PREPAINT, CDDS_ITEMPREPAINT
		return 0x20 // CDRF_NOTIFYITEMDRAW, CDRF_NOTIFYSUBITEMDRAW
	case 0x30001: // CDDS_SUBITEM | CDDS_ITEMPREPAINT
		row := int(cd.ItemSpec)
		if cd.SubItem == fixColPlace && d.placeOf(row) != "" {
			if send(d.list, lvmGetItemState, uintptr(row), 2) == 0 { // not selected
				cd.ClrText = rgb(0, 102, 204)
			}
			pSelectObject.Call(cd.Hdc, a.fontLink)
		} else {
			cd.ClrText = sysColor(8) // COLOR_WINDOWTEXT
			pSelectObject.Call(cd.Hdc, a.fontUI)
		}
		return 0x2 // CDRF_NEWFONT
	}
	return 0
}

// fixListSubclass gives the list of the Fix window what the lists of the main window
// have (listSubclass): a click on a Location shows the file in File Explorer, and the
// cells have tooltips.
func fixListSubclass(hwnd uintptr, msg uint32, wParam, lParam, id, ref uintptr) uintptr {
	a, d := g, g.fixDlgOfList(hwnd)
	if d != nil {
		switch msg {
		case wmMouseMove:
			a.updateCellTip(hwnd, point{xParam(lParam), yParam(lParam)})
		case wmMouseLeave, wmMouseWheel, wmVScroll, wmHScroll:
			resetCellTip(hwnd)
		case wmSetCursor:
			if loword(lParam) == 1 /* HTCLIENT */ {
				if _, ok := a.fixLinkAt(d, cursorIn(hwnd)); ok {
					pSetCursor.Call(a.curHand)
					return 1
				}
			}
		case wmLButtonDown:
			resetCellTip(hwnd)
			// as in listSubclass: the list view handles the whole click in its
			// WM_LBUTTONDOWN (the row is selected); the file is shown once it returns,
			// if the mouse is still on it. Ctrl and Shift clicks only select.
			if wParam&(0x4|0x8) == 0 { // MK_SHIFT|MK_CONTROL
				if target, ok := a.fixLinkAt(d, point{xParam(lParam), yParam(lParam)}); ok {
					r, _, _ := pDefSubclassProc.Call(hwnd, uintptr(msg), wParam, lParam)
					if t2, ok := a.fixLinkAt(d, cursorIn(hwnd)); ok && t2 == target {
						openPath(d.hwnd, target)
					}
					return r
				}
			}
		}
	}
	r, _, _ := pDefSubclassProc.Call(hwnd, uintptr(msg), wParam, lParam)
	return r
}

// check ticks or unticks a row of the list.
func (d *fixDlg) check(row int, on bool) {
	li := lvItem{StateMask: 0xF000 /* LVIS_STATEIMAGEMASK */, State: 1 << 12}
	if on {
		li.State = 2 << 12
	}
	send(d.list, lvmSetItemState, uintptr(row), uintptr(unsafe.Pointer(&li)))
}

func (d *fixDlg) checked(row int) bool {
	return send(d.list, lvmGetItemState, uintptr(row), 0xF000)>>12 == 2
}

// done removes the check box of a fixed row: there is nothing left to tick it for.
func (d *fixDlg) done(row int) {
	li := lvItem{StateMask: 0xF000 /* LVIS_STATEIMAGEMASK */, State: 0}
	send(d.list, lvmSetItemState, uintptr(row), uintptr(unsafe.Pointer(&li)))
}

// hasBox tells whether a row still has its check box: it is not fixed.
func (d *fixDlg) hasBox(row int) bool {
	return send(d.list, lvmGetItemState, uintptr(row), 0xF000)>>12 != 0
}

// showCount says how many software are ticked, of how many that are not fixed: a
// software with two rows (fixParts) counts once.
func (d *fixDlg) showCount() {
	if d.count == 0 {
		return
	}
	ticked, left := map[int]bool{}, map[int]bool{}
	for i, row := range d.rows {
		if d.hasBox(i) {
			left[row.ID] = true
		}
		if d.checked(i) {
			ticked[row.ID] = true
		}
	}
	n, of := len(ticked), len(left)
	key := "fix.selected"
	if n <= 1 {
		key = "fix.selected1" // in French, "logiciel" in the singular
	}
	setText(d.count, Tf(key, n, of)+" · "+T("fix.listHint"))
}

// copyRow copies a row of the list, with the headers of the list, as Copy (with
// headers) does in the main window (copyRows).
func (d *fixDlg) copyRow(row int) {
	if row < 0 || row >= len(d.rows) {
		return
	}
	var hdr, cells []string
	for c, key := range []string{"fix.colSoftware", "col.component", "fix.colFix", "fix.colPlace", "fix.colResult"} {
		hdr = append(hdr, T(key))
		if c == fixColPlace {
			cells = append(cells, d.placeOf(row))
		} else {
			cells = append(cells, d.cellText(row, c))
		}
	}
	setClipboard(d.hwnd, strings.Join(hdr, "\t")+"\n"+strings.Join(cells, "\t")+"\n")
}

// fixContextMenu is the menu of a right-click on a row of the list: that of the
// software lists of the main window (contextMenu), for that row, without Select all,
// which would be taken for the Select All button.
func (a *ui) fixContextMenu(d *fixDlg, lParam uintptr) {
	row := int(int32(send(d.list, lvmGetNextItem, ^uintptr(0), 2 /* LVNI_SELECTED */)))
	if row < 0 || row >= len(d.rows) || d.asking {
		return
	}
	it := d.rows[row].Item
	if now, ok := a.items[it.ID]; ok {
		it = now
	}
	var pt point
	if int32(lParam) == -1 { // from the keyboard
		rc := rect{}
		send(d.list, lvmGetItemRect, uintptr(row), uintptr(unsafe.Pointer(&rc)))
		pt = point{rc.Left + a.scale(20), rc.Bottom}
		pClientToScreen.Call(d.list, uintptr(unsafe.Pointer(&pt)))
	} else {
		pt = point{xParam(lParam), yParam(lParam)}
	}
	grayed := func(ok bool) uintptr {
		if ok {
			return 0
		}
		return 1 // MF_GRAYED
	}
	menu, _, _ := pCreatePopupMenu.Call()
	add := func(flags uintptr, id int, key string) { pAppendMenuW.Call(menu, flags, uintptr(id), up(T(key))) }
	sep := func() { pAppendMenuW.Call(menu, 0x800, 0, 0) }
	add(grayed(!d.running), menuDetails, "menu.details")
	sep()
	add(grayed(it.File != ""), menuOpenFile, "menu.openFile")
	add(grayed(it.Folder != ""), menuOpenFolder, "menu.openFolder")
	add(grayed(it.Homepage() != ""), menuHomepage, "menu.homepage")
	add(0, menuSearch, "menu.search")
	sep()
	add(grayed(it.Recheck && !a.running && a.current != nil && !d.running), menuRecheck, "menu.recheck")
	sep()
	add(0, menuCopy, "menu.copy")
	add(grayed(it.File != "" || it.Folder != ""), menuCopyPaths, "menu.copyPaths")
	cmd, _, _ := pTrackPopupMenu.Call(menu, 0x100|0x2 /* RETURNCMD|RIGHTBUTTON */, uintptr(pt.X), uintptr(pt.Y), 0, d.hwnd, 0)
	pDestroyMenu.Call(menu)
	switch int(cmd) {
	case 0:
	case menuDetails:
		a.openFixDetails(d, row)
	case menuCopy:
		d.copyRow(row)
	default:
		a.runMenu(int(cmd), d.list, []Item{it})
		if a.running { // Scan again: no fix until the list shows the result (finish)
			pEnableWindow.Call(d.bFix, 0)
		}
	}
}

// targets are the elements to fix, those of the details or of the ticked rows, and
// which of their fixes run: all of them from the details; from the list, those of
// the ticked rows.
func (d *fixDlg) targets() ([]int, fixPick) {
	if d.list == 0 {
		return d.ids, nil
	}
	var ids []int
	pick := fixPick{}
	for i, row := range d.rows {
		if !d.checked(i) {
			continue
		}
		for _, id := range row.memberIDs() {
			if _, seen := pick[id]; !seen {
				ids = append(ids, id)
			}
			pick[id] = append(pick[id], row.part)
		}
	}
	return ids, pick
}

// selected are the elements to fix as they are now, with the fixes that will run.
func (d *fixDlg) selected(a *ui) []Item {
	ids, pick := d.targets()
	return pick.items(a.fixItems(ids))
}

// rowItems are the elements of a row of the list as they are now, with the fixes
// that the row runs: none once they have run, or the software is fixed.
func (d *fixDlg) rowItems(a *ui, row int) []Item {
	r := d.rows[row]
	pick := fixPick{}
	for _, id := range r.memberIDs() {
		pick[id] = []fixPart{r.part}
	}
	return pick.items(a.fixItems(r.memberIDs()))
}

// newFixWindow makes the window of d, w × h (before scaling), above its owner, with
// the controls both kinds of Fix window have.
func (a *ui) newFixWindow(d *fixDlg, w, h int) bool {
	a.fix = d
	owner := d.owner
	wa := a.workArea()
	w32 := min(a.scale(w), wa.Right-wa.Left-a.scale(40))
	h32 := min(a.scale(h), wa.Bottom-wa.Top-a.scale(40))
	var or rect
	pGetWindowRect.Call(owner, uintptr(unsafe.Pointer(&or)))
	x := max(wa.Left, min(or.Left+(or.Right-or.Left-w32)/2, wa.Right-w32))
	y := max(wa.Top, min(or.Top+(or.Bottom-or.Top-h32)/2, wa.Bottom-h32))
	inst, _, _ := pGetModuleHandleW.Call(0)
	title := d.label + " · " + appName + simulationMark
	d.hwnd, _, _ = pCreateWindowExW.Call(wsExControlParent, up("eMudScanFix"), up(title),
		uintptr(wsOverlappedWindow&^wsMinimizeBox|0x02000000 /* WS_CLIPCHILDREN: no flicker as the list is resized */), uintptr(x), uintptr(y), uintptr(w32), uintptr(h32), owner, 0, inst, 0)
	if d.hwnd == 0 {
		a.fix = nil
		return false
	}
	d.font = createFont(9, a.dpi, 600, false, "Segoe UI")
	d.title, _, _ = pCreateWindowExW.Call(0, up("STATIC"), 0, uintptr(wsChild|wsVisible|0x80|0x4000 /* SS_NOPREFIX|SS_ENDELLIPSIS */),
		0, 0, 10, 10, d.hwnd, 0, 0, 0)
	send(d.title, wmSetFont, d.font, 1)
	d.console = a.newConsoleView(d.hwnd, idFixLog)
	// its copy button: the commands, as a script with their comments, or, once they
	// have run, their results
	d.console.copyText = func() string { return string(utf16.Decode(d.console.text)) }
	d.console.copyTip = func() string {
		if d.results {
			return T("fix.copyResults")
		}
		return T("dlg.copyBlock")
	}
	d.bBackup, _, _ = pCreateWindowExW.Call(0, up("BUTTON"), up(T("fix.backup")), uintptr(wsChild|wsVisible|wsTabStop|3 /* BS_AUTOCHECKBOX */),
		0, 0, 10, 10, d.hwnd, idFixBackup, 0, 0)
	send(d.bBackup, wmSetFont, a.fontUI, 1)
	send(d.bBackup, bmSetCheck, 1, 0)
	addTool(a.newTooltip(d.hwnd), d.hwnd, d.bBackup, nil, T("tip.fixBackup")) // what it backs up, and what not
	d.bFix = a.iconButton(d.hwnd, d.runText, idFixRun, false, glyphFirstAid)
	if d.ids == nil { // the Fix software window: Stop skips the software not reached yet
		d.bStop = a.iconButton(d.hwnd, T("btn.stop"), idFixStop, false, glyphStop)
		pEnableWindow.Call(d.bStop, 0)
	}
	// Close is the default: Enter never starts the fixes, only a click on the button
	d.bClose = a.iconButton(d.hwnd, T("dlg.close"), idFixClose, true, glyphClose)
	return true
}

// showFix shows the Fix window once its controls are made.
func (a *ui) showFix(d *fixDlg) {
	d.layout(a)
	d.preview(a) // once the console has its size, from its first line
	pEnableWindow.Call(d.owner, 0)
	pShowWindow.Call(d.hwnd, 1)
	pSetForegroundWindow.Call(d.hwnd)
	pSetFocus.Call(d.bClose)
}

func (d *fixDlg) backup() bool { return send(d.bBackup, bmGetCheck, 0, 0) == 1 }

// preview shows the commands that will run, and enables the button that runs them
// when there are some.
func (d *fixDlg) preview(a *ui) {
	title := "fix.cmdsThis"
	if d.list != 0 {
		title = "fix.cmdsSelected"
	}
	setText(d.title, T(title))
	d.clear()
	d.results = false
	d.console.updateCopyTip()
	items := d.selected(a)
	d.titles = fixTitles(items)
	lines := fixPreview(items, d.backup())
	// "Back up the files to change" only when a fix backs up files
	show := uintptr(0) // SW_HIDE
	if backsUp(items) {
		show = 5 // SW_SHOW
	}
	pShowWindow.Call(d.bBackup, show)
	if len(items) == 0 {
		lines = []fixLine{{lineComment, T("fix.noneSelected")}}
	}
	for _, l := range lines {
		d.show(l)
	}
	d.console.scrollTo(0)
	pEnableWindow.Call(d.bFix, boolArg(len(items) > 0))
	d.showCount()
}

// clear empties the console.
func (d *fixDlg) clear() {
	d.console.clear()
	d.lines, d.lineAt, d.cmdLine = nil, nil, -1
}

// show adds a line to the console. While the fixes run, the command running has … in
// the left column, then its result: ✓ or ✗.
func (d *fixDlg) show(l fixLine) {
	if (l.kind == lineOK || l.kind == lineFailed) && d.cmdLine >= 0 {
		mark, color := "✔", consoleOK // heavier than ✓ and ✗ of the report
		if l.kind == lineFailed {
			mark, color = "✘", consoleFailed
		}
		d.console.setMark(d.cmdLine, mark, color)
		d.lineAt = append(d.lineAt, d.cmdLine)
		d.lines = append(d.lines, l)
		d.cmdLine = -1
		if l.text != "" { // what the simulation did
			d.lineAt[len(d.lineAt)-1] = d.console.nextLine()
			d.console.append(consoleRuns(l), false)
		}
		return
	}
	if d.cmdLine >= 0 { // no result came
		d.console.setMark(d.cmdLine, "", 0)
		d.cmdLine = -1
	}
	blank := fixLine{lineNote, ""}
	if l.kind == lineEnd && len(d.lines) > 0 && d.lines[len(d.lines)-1] != blank {
		d.console.append(consoleRuns(blank), false) // an empty line before the bar, once
	}
	d.lineAt = append(d.lineAt, d.console.nextLine())
	d.lines = append(d.lines, l)
	d.console.append(consoleRuns(l), false)
	if l.kind == lineEnd { // drawn in the middle of its row, then an empty line
		d.console.setBar(d.lineAt[len(d.lineAt)-1], rgb(0x76, 0x76, 0x76))
		d.console.append(consoleRuns(blank), false)
	}
	if l.kind == lineCommand && d.running {
		d.cmdLine = d.lineAt[len(d.lineAt)-1]
		d.console.setMark(d.cmdLine, "…", rgb(0x76, 0x76, 0x76))
	}
}

// The colors of the results: those of Monokai, brighter than the green and the red
// of PowerShell's palette that the commands use.
var (
	consoleOK     = rgb(0xA6, 0xE2, 0x2E)
	consoleFailed = rgb(0xF9, 0x26, 0x72)
)

// showRow scrolls the console to the commands of a row of the list, or to what they
// did, up to the blank line after them, as little as needed, so that what is above
// stays in view when it can; it reports whether the console shows them.
func (d *fixDlg) showRow(row int) bool {
	if row < 0 || row >= len(d.rows) {
		return false
	}
	want := map[string]bool{}
	for _, id := range d.rows[row].memberIDs() {
		if t, ok := d.titles[id]; ok {
			want[t] = true
		}
	}
	for i, l := range d.lines {
		if l.kind == lineSoftware && want[l.text] {
			last := d.lineAt[len(d.lineAt)-1]
			for j := i + 1; j < len(d.lines); j++ {
				if d.lines[j] == (fixLine{lineNote, ""}) {
					last = d.lineAt[j] - 1
					break
				}
			}
			d.console.showLines(d.lineAt[i], last)
			return true
		}
	}
	return false
}

// showNear scrolls the console to the commands of a row just ticked, or, for a row
// unticked, to those of the next row ticked, else of the one before.
func (d *fixDlg) showNear(row int) {
	if d.checked(row) && d.showRow(row) {
		return
	}
	for j := row + 1; j < len(d.rows); j++ {
		if d.checked(j) && d.showRow(j) {
			return
		}
	}
	for j := row - 1; j >= 0; j-- {
		if d.checked(j) && d.showRow(j) {
			return
		}
	}
}

// consoleRuns is how the console shows a line, in the colors of a PowerShell window:
// the commands in those of PowerShell 7 (psTokens), what they do as comments, in
// green; then what the commands show in gray, their warnings in dark yellow, their
// errors in red, eMudScan's messages in cyan. The last run ends the line.
func consoleRuns(l fixLine) []logRun {
	text, color, bold := l.text, rgb(220, 220, 220), false
	switch l.kind {
	case lineCommand:
		var runs []logRun
		for _, tk := range psTokens(l.text) {
			c := color
			if r, g, b, ok := psColor(tk.kind); ok {
				c = rgb(r, g, b)
			}
			runs = append(runs, logRun{tk.text, c, false, false})
		}
		if len(runs) == 0 {
			runs = []logRun{{text: ""}}
		}
		runs[len(runs)-1].text += "\n"
		return runs
	case lineSoftware:
		text, color, bold = "# "+l.text, rgb(106, 153, 85), true
	case lineComment:
		text, color = "# "+l.text, rgb(106, 153, 85)
	case lineOutput, lineOK:
		color = rgb(150, 150, 150)
	case lineWarning:
		color = rgb(0xC1, 0x9C, 0x00) // DarkYellow
	case lineFailed, lineError, lineCmdError:
		color = consoleFailed
	case lineFixed:
		color, bold = consoleOK, true
	case lineNotFixed:
		color, bold = consoleFailed, true
	case lineNote:
		color = rgb(97, 214, 214)
	case lineEnd:
		text, color = fixBar, rgb(0x76, 0x76, 0x76) // drawn as a line (setBar), copied as text
	}
	if l.kind == lineOutput || l.kind == lineOK || l.kind == lineFailed || l.kind == lineCmdError || l.kind == lineWarning {
		// what a command showed, in its own colors when it has some (ansiPieces)
		var runs []logRun
		for _, p := range ansiPieces(text) {
			c := color
			if p.color >= 0 {
				c = rgb(byte(p.color>>16), byte(p.color>>8), byte(p.color))
			}
			runs = append(runs, logRun{p.text, c, bold || p.bold, false})
		}
		runs[len(runs)-1].text += "\n"
		return runs
	}
	return []logRun{{text + "\n", color, bold, false}}
}

func (d *fixDlg) layout(a *ui) {
	var rc rect
	pGetClientRect.Call(d.hwnd, uintptr(unsafe.Pointer(&rc)))
	s := a.scale
	bh := s(30)
	bar := bh + s(24)
	m := s(14)
	top := s(12)
	if d.list != 0 { // Select All, Select None, and the list, above the console
		x := m
		for _, b := range []uintptr{d.bAll, d.bNone} {
			bw := max(s(110), a.textWidth(a.fontUI, getText(b))+s(32))
			a.move(b, x, top, bw, bh)
			x += bw + s(8)
		}
		a.move(d.count, x+s(8), top, max(s(40), rc.Right-m-x-s(8)), bh)
		top += bh + s(8)
		// the list takes 30% of the room it shares with the console, or what the user
		// chose by dragging the bar below it, down to a single row, leaving the console
		// room for a few lines
		d.room = rc.Bottom - bar - top
		part := d.part
		if part == 0 {
			part = .3
		}
		d.listH = max(a.listHeight(d.list, 1), min(int32(float64(d.room)*part), d.room-s(170)))
		listH := d.listH
		lw := rc.Right - 2*m
		a.move(d.list, m, top, lw, listH)
		inner := lw - s(4) - s(18) // borders, and room for a scroll bar
		// Software, Component, Fix, Location (what is left), Result
		widths := []int32{inner * 24 / 100, inner * 16 / 100, inner * 30 / 100, 0, max(s(90), a.textWidth(a.fontUI, T("fix.resultNotFixed"))+s(16))}
		widths[3] = max(s(60), inner-widths[0]-widths[1]-widths[2]-widths[4])
		for i, cw := range widths {
			send(d.list, lvmSetColumnWidth, uintptr(i), uintptr(cw))
		}
		d.fitPlaces(a)
		d.split = rect{m, top + listH, rc.Right - m, top + listH + s(14)}
		top += listH + s(14)
	}
	th := a.fontMetrics(d.font).Height
	a.move(d.title, m, top, rc.Right-2*m, th)
	// the console in its black frame, rounded like the commands of the details page
	d.frame = rect{m, top + th + s(8), rc.Right - m, rc.Bottom - bar - s(12)}
	a.move(d.console.hwnd, d.frame.Left+s(10), d.frame.Top+s(8), d.frame.Right-d.frame.Left-s(10)-s(4), d.frame.Bottom-d.frame.Top-s(16))
	y := rc.Bottom - bh - s(12)
	a.move(d.bBackup, m, y, a.textWidth(a.fontUI, T("fix.backup"))+s(40), bh)
	x := rc.Right - m
	for _, b := range []uintptr{d.bClose, d.bStop, d.bFix} {
		if b == 0 { // no Stop in the window of the details
			continue
		}
		bw := max(s(120), a.textWidth(a.fontUI, getText(b))+s(16)+s(44))
		x -= bw
		a.move(b, x, y, bw, bh)
		x -= s(8)
	}
	pInvalidateRect.Call(d.hwnd, 0, 1)
}

// paint draws the black frame of the console and a thin line above the buttons.
func (d *fixDlg) paint(a *ui, hwnd uintptr) {
	var ps paintStruct
	dc, _, _ := pBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	defer pEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	var rc rect
	pGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
	top := rc.Bottom - a.scale(30) - a.scale(24)
	line := rect{0, top, rc.Right, top + max(1, a.scale(1))}
	br, _, _ := pGetSysColorBrush.Call(16 /* COLOR_BTNSHADOW */)
	pFillRect.Call(dc, uintptr(unsafe.Pointer(&line)), br)
	if d.list != 0 { // a grip in the middle of the bar below the list
		cx, cy := (d.split.Left+d.split.Right)/2, (d.split.Top+d.split.Bottom)/2
		grip := rect{cx - a.scale(16), cy - max(1, a.scale(1)), cx + a.scale(16), cy + max(1, a.scale(1))}
		pFillRect.Call(dc, uintptr(unsafe.Pointer(&grip)), br)
	}
	bg, _, _ := pCreateSolidBrush.Call(consoleBg)
	nullPen, _, _ := pGetStockObject.Call(8 /* NULL_PEN */)
	oldBrush, _, _ := pSelectObject.Call(dc, bg)
	oldPen, _, _ := pSelectObject.Call(dc, nullPen)
	f := d.frame
	pRoundRect.Call(dc, uintptr(f.Left), uintptr(f.Top), uintptr(f.Right+1), uintptr(f.Bottom+1), uintptr(a.scale(10)), uintptr(a.scale(10)))
	pSelectObject.Call(dc, oldPen)
	pSelectObject.Call(dc, oldBrush)
	pDeleteObject.Call(bg)
}

// run asks the user to accept the warning, then starts the fixes, in the background;
// the console follows them.
func (d *fixDlg) run(a *ui) {
	if d.running || d.asking || a.running || !pEnabled(d.bFix) {
		return
	}
	question := d.question
	if d.list != 0 { // "Fix the 3 selected software?"
		n := len(d.selected(a))
		if n == 1 {
			question = T("fix.confirmOne")
		}
	}
	d.asking = true
	accepted := askWarning(d.hwnd, question, fixWarningText(d.selected(a)), T("fix.accept"), T("fix.decline"))
	d.asking = false
	if !accepted {
		return
	}
	ids, pick := d.targets()
	if len(pick.items(a.fixItems(ids))) == 0 {
		return
	}
	d.running, d.ran = true, true
	d.stop.Store(false)
	d.ranRows = map[int]bool{}
	for i := range d.rows {
		if d.checked(i) {
			d.ranRows[i] = true
		}
	}
	for _, h := range []uintptr{d.bFix, d.bClose, d.bBackup, d.list, d.bAll, d.bNone} {
		if h != 0 {
			pEnableWindow.Call(h, 0)
		}
	}
	if d.bStop != 0 {
		pEnableWindow.Call(d.bStop, 1)
		pSetFocus.Call(d.bStop)
	} else {
		pSetFocus.Call(d.console.hwnd)
	}
	setText(d.title, T("fix.resultsTitle"))
	d.clear()
	d.results = true
	d.console.updateCopyTip()
	d.titles = fixTitles(pick.items(a.fixItems(ids))) // as the runner names them
	c := a.current
	// the console, also kept for the report, after a line that says when and what
	out := func(l fixLine) {
		c.recordFix(l)
		postEvent(uiEvent{kind: 4, fl: l})
	}
	if len(c.fixRecord()) > 0 {
		c.recordFix(fixLine{lineNote, ""})
	}
	c.recordFix(fixLine{lineNote, Tf("fix.started", time.Now().Format("2006-01-02 15:04"), d.label)})
	logged := false // the log of the main window shows the checks after the fixes in their own section
	r := &fixRunner{
		exec: runFixes,
		item: c.get,
		recheck: func(ids []int) {
			if !logged {
				logged = true
				c.section(T("fix.logSection"))
			}
			c.recheck(ids)
		},
		stopped:   d.stop.Load,
		out:       out,
		fixed:     func(id int) { postEvent(uiEvent{kind: 5, item: Item{ID: id}}) },
		simulated: func(id int) { simulateFixed(c, id) },
		backup:    d.backup(),
		pick:      pick,
	}
	fixPromptOwner = d.hwnd
	fixPromptClosed = func() { postEvent(uiEvent{kind: 7}) }
	go func() {
		defer func() {
			if p := recover(); p != nil {
				out(fixLine{lineError, fmt.Sprintf("Internal error: %v", p)})
			}
			out(fixLine{lineEnd, ""})
			postEvent(uiEvent{kind: 6})
		}()
		r.run(ids)
	}()
}

// fixEvent shows what the runner reports (from processEvents).
func (a *ui) fixEvent(e uiEvent) {
	d := a.fix
	if d == nil {
		return
	}
	switch e.kind {
	case 4:
		d.show(e.fl)
	case 5: // a software is fixed: its row is unticked, so that it is not fixed twice
		d.rowResults(a, false)
	case 7: // the administrator prompt is answered
		d.toFront()
	case 6:
		d.rowResults(a, true)
		d.running = false
		for _, h := range []uintptr{d.bClose, d.bBackup, d.list, d.bAll, d.bNone} {
			if h != 0 {
				pEnableWindow.Call(h, 1)
			}
		}
		if d.bStop != 0 {
			pEnableWindow.Call(d.bStop, 0)
		}
		pEnableWindow.Call(d.bFix, boolArg(len(d.selected(a)) > 0)) // what is left to fix
		pSetFocus.Call(d.bClose)
		if d.list != 0 {
			a.saveFixReport(d)
		}
	}
}

var (
	pAttachThreadInput        = moduser32.NewProc("AttachThreadInput")
	pGetWindowThreadProcessId = moduser32.NewProc("GetWindowThreadProcessId")
	pBringWindowToTop         = moduser32.NewProc("BringWindowToTop")
	pFlashWindowEx            = moduser32.NewProc("FlashWindowEx")
	pGetCurrentThreadId       = kernel32.NewProc("GetCurrentThreadId")
)

const timerFront = 1

// toFront brings the Fix window back to the front once the administrator prompt is
// answered: Windows then gives the foreground to another window, such as File
// Explorer, now or a moment later. So it does it again for a moment; if it cannot,
// the window flashes in the taskbar.
func (d *fixDlg) toFront() {
	d.frontTries = 6
	bringToFront(d.hwnd)
	pSetTimer.Call(d.hwnd, timerFront, 250, 0)
}

// flashWInfo is FLASHWINFO.
type flashWInfo struct {
	size             uint32
	hwnd             uintptr
	flags, count, ms uint32
}

// bringToFront makes hwnd the foreground window, and reports whether it is. Windows
// lets a program do so only in some cases, such as when the window in front is its
// own; otherwise, eMudScan shares for a moment the input of the window in front,
// which lets it.
func bringToFront(hwnd uintptr) bool {
	front := func() bool {
		fg, _, _ := pGetForegroundWindow.Call()
		return fg == hwnd
	}
	if front() {
		return true
	}
	pSetForegroundWindow.Call(hwnd)
	if front() {
		return true
	}
	fg, _, _ := pGetForegroundWindow.Call()
	me, _, _ := pGetCurrentThreadId.Call()
	other, _, _ := pGetWindowThreadProcessId.Call(fg, 0)
	if fg != 0 && other != 0 && other != me {
		if r, _, _ := pAttachThreadInput.Call(me, other, 1); r != 0 {
			pBringWindowToTop.Call(hwnd)
			pSetForegroundWindow.Call(hwnd)
			pAttachThreadInput.Call(me, other, 0)
		}
	}
	return front()
}

// rowResults shows in the list which rows are fixed, unticked; at the end, also those
// that are not. A row has nothing left to run once its fixes have, or once the other
// row of its software (fixParts) has fixed it.
func (d *fixDlg) rowResults(a *ui, end bool) {
	if d.list == 0 {
		return
	}
	d.updating = true
	defer func() {
		d.updating = false
		d.showCount()
	}()
	ran := map[int]bool{} // the elements of the rows that ran
	for i, row := range d.rows {
		for _, id := range row.memberIDs() {
			ran[id] = ran[id] || d.ranRows[i]
		}
	}
	for i, row := range d.rows {
		concerned := false
		for _, id := range row.memberIDs() {
			concerned = concerned || ran[id]
		}
		if !concerned || !d.hasBox(i) {
			continue
		}
		result := ""
		if len(d.rowItems(a, i)) == 0 {
			result = T("fix.resultFixed")
			d.done(i)
			if it, ok := a.items[row.ID]; ok {
				a.setRow(d.list, i, it, []string{fixSoftware(it)}, false) // its new status icon
			}
		} else if end && d.ranRows[i] {
			result = T("fix.resultNotFixed")
		}
		if result != "" {
			sub := lvItem{SubItem: 4, Text: u16(result)}
			send(d.list, lvmSetItemTextW, uintptr(i), uintptr(unsafe.Pointer(&sub)))
		}
	}
}

// saveFixReport saves the report, with what the fixes did, in the Downloads folder,
// and says where, with a link that shows it in File Explorer (showSaved).
func (a *ui) saveFixReport(d *fixDlg) {
	path, err := a.saveReportIn(downloadsDir())
	if err != nil {
		msg := Tf("fix.reportError", err)
		d.show(fixLine{lineError, msg})
		msgBox(d.hwnd, msg, 0x30 /* ICONWARNING */)
		return
	}
	d.show(fixLine{lineNote, Tf("fix.reportSaved", shownPath(path))})
	a.showSaved(d.hwnd, T("fix.reportSavedIn"), path)
}

var (
	pSHGetKnownFolderPath = modshell32w.NewProc("SHGetKnownFolderPath")
	pCoTaskMemFree        = modole32.NewProc("CoTaskMemFree")
)

// downloadsDir is the user's Downloads folder (FOLDERID_Downloads), wherever the user
// moved it; else the Downloads folder of the profile.
func downloadsDir() string {
	id := syscall.GUID{Data1: 0x374DE290, Data2: 0x123F, Data3: 0x4565, Data4: [8]byte{0x91, 0x64, 0x39, 0xC4, 0x92, 0x5E, 0x46, 0x7B}}
	var p *uint16
	if r, _, _ := pSHGetKnownFolderPath.Call(uintptr(unsafe.Pointer(&id)), 0, 0, uintptr(unsafe.Pointer(&p))); r == 0 && p != nil {
		defer pCoTaskMemFree.Call(uintptr(unsafe.Pointer(p)))
		n := 0
		for *(*uint16)(unsafe.Add(unsafe.Pointer(p), 2*n)) != 0 {
			n++
		}
		if dir := syscall.UTF16ToString(unsafe.Slice(p, n)); dir != "" {
			return dir
		}
	}
	if up := os.Getenv("USERPROFILE"); up != "" {
		return filepath.Join(up, "Downloads")
	}
	return ""
}

// saveReportIn saves the report in dir, under a name that no file there has yet.
func (a *ui) saveReportIn(dir string) (string, error) {
	if dir == "" {
		return "", errors.New("no Downloads folder")
	}
	base := "eMudScan-report-" + time.Now().Format("20060102-1504")
	for i := 1; i < 100; i++ {
		name := base + ".txt"
		if i > 1 {
			name = fmt.Sprintf("%s-%d.txt", base, i)
		}
		path := filepath.Join(dir, name)
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		_, err = f.Write(a.reportData())
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		return path, err
	}
	return "", fs.ErrExist
}

func (a *ui) closeFix() {
	d := a.fix
	if d == nil || d.running {
		return
	}
	pEnableWindow.Call(d.owner, 1)
	if fg, _, _ := pGetForegroundWindow.Call(); fg == d.hwnd {
		pSetForegroundWindow.Call(d.owner)
	}
	d.console.destroy()
	delete(cellTips, d.list) // its tooltip goes with it
	pDestroyWindow.Call(d.hwnd)
	pDeleteObject.Call(d.font)
	a.fix, a.fixUnder = a.fixUnder, nil // back to the Fix software window, when the details opened from it opened this one
	if d.ran {
		a.refreshDetails() // the details as the fixes left the software
	}
	a.updateFixButton()
}

func fixWndProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	a := g
	if d := a.fixDlgOf(hwnd); d != nil {
		switch msg {
		case wmSize:
			d.layout(a)
			return 0
		case wmPaint:
			d.paint(a, hwnd)
			return 0
		case wmSetCursor: // over the bar below the list: the cursor that resizes it
			if d.list != 0 && loword(lParam) == 1 /* HTCLIENT */ {
				var pt point
				pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
				pScreenToClient.Call(hwnd, uintptr(unsafe.Pointer(&pt)))
				if d.split.has(pt) {
					c, _, _ := pLoadCursorW.Call(0, 32645 /* IDC_SIZENS */)
					pSetCursor.Call(c)
					return 1
				}
			}
		case wmLButtonDown:
			if pt := (point{xParam(lParam), yParam(lParam)}); d.list != 0 && d.split.has(pt) {
				d.drag, d.dragY, d.fromH = true, pt.Y, d.listH
				pSetCapture.Call(hwnd)
			}
			return 0
		case wmMouseMove:
			if d.drag && d.room > 0 {
				d.part = float64(d.fromH+yParam(lParam)-d.dragY) / float64(d.room)
				d.layout(a)
			}
			return 0
		case wmLButtonUp:
			if d.drag {
				pReleaseCapture.Call()
			}
			return 0
		case wmCaptureChanged:
			d.drag = false
			return 0
		case wmTimer:
			if wParam == timerFront {
				d.frontTries--
				if !bringToFront(hwnd) && d.frontTries <= 0 {
					fw := flashWInfo{hwnd: hwnd, flags: 0x3 | 0xC /* FLASHW_ALL|FLASHW_TIMERNOFG */}
					fw.size = uint32(unsafe.Sizeof(fw))
					pFlashWindowEx.Call(uintptr(unsafe.Pointer(&fw)))
				}
				if d.frontTries <= 0 {
					pKillTimer.Call(hwnd, timerFront)
				}
				return 0
			}
		case wmGetMinMaxInfo:
			pt := (*[2]int32)(unsafe.Add(ptr(lParam), 24))
			pt[0], pt[1] = a.scale(640), a.scale(420)
			return 0
		case wmCtlColorStatic: // the title and the check box, on the window's background
			pSetBkColor.Call(wParam, uintptr(sysColor(5)))
			return a.brushWindow
		case wmContextMenu:
			if wParam == d.list && d.list != 0 {
				a.fixContextMenu(d, lParam)
				return 0
			}
		case wmNotify:
			if h := (*nmhdr)(ptr(lParam)); h.HwndFrom == d.list && h.Code == nmCustomDraw {
				return a.fixListDraw(d, (*nmLVCustomDraw)(ptr(lParam)))
			}
			nm := (*nmListViewState)(ptr(lParam))
			// a double-click on a software, except on its Location (a link) or its check box: its details
			if nm.Hdr.HwndFrom == d.list && nm.Hdr.Code == -3 /* NM_DBLCLK, NMITEMACTIVATE */ && !d.running && !d.asking {
				ht := lvHitTestInfo{Pt: nm.Pt}
				if int32(send(d.list, lvmSubItemHitTest, 0, uintptr(unsafe.Pointer(&ht)))) >= 0 && ht.Item >= 0 &&
					ht.SubItem != fixColPlace && ht.Flags&0x8 /* LVHT_ONITEMSTATEICON */ == 0 {
					a.openFixDetails(d, int(ht.Item))
				}
				return 0
			}
			if nm.Hdr.HwndFrom == d.list && nm.Hdr.Code == lvnKeyDown && ctrlDown() && (*nmLVKeyDown)(ptr(lParam)).VKey == 'C' {
				d.copyRow(int(int32(send(d.list, lvmGetNextItem, ^uintptr(0), 2 /* LVNI_SELECTED */)))) // as Copy (with headers)
				return 0
			}
			if nm.Hdr.HwndFrom == d.list && nm.Hdr.Code == lvnColumnClick && !d.running && !d.asking {
				d.sortKeys = clickSort(d.sortKeys, int(nm.SubItem))
				d.sortRows(a)
				return 0
			}
			if nm.Hdr.HwndFrom == d.list && nm.Hdr.Code == -100 /* LVN_ITEMCHANGING */ && nm.Flags&0x8 /* LVIF_STATE */ != 0 &&
				nm.OldState&0xF000 == 0 && nm.NewState&0xF000 != 0 && !d.updating {
				return 1 // a fixed row, which has no check box: it cannot be ticked again
			}
			if nm.Hdr.HwndFrom == d.list && nm.Hdr.Code == -101 /* LVN_ITEMCHANGED */ && nm.Flags&0x8 /* LVIF_STATE */ != 0 &&
				!d.updating && !d.running && !d.asking {
				switch row := int(nm.Item); {
				case (nm.NewState^nm.OldState)&0xF000 != 0: // ticked or unticked: the commands as they will run now
					d.preview(a)
					d.showNear(row)
				case nm.NewState&0x2 != 0 && nm.OldState&0x2 == 0: // selected: its commands, or what they did
					d.showRow(row)
				}
			}
			return 0
		case wmCommand:
			switch loword(wParam) {
			case idFixAll, idFixNone:
				if !d.running && !d.asking {
					d.updating = true
					for i := range d.rows {
						if d.hasBox(i) { // not a fixed one
							d.check(i, loword(wParam) == idFixAll)
						}
					}
					d.updating = false
					d.preview(a)
				}
			case idFixBackup:
				if !d.running && !d.asking {
					d.preview(a) // the commands as they will run now
				}
			case idFixRun:
				d.run(a)
			case idFixStop:
				d.stop.Store(true)
				pEnableWindow.Call(d.bStop, 0)
			case idFixClose, 1, 2: // Close, Enter, Escape
				if !d.asking {
					a.closeFix()
				}
			}
			return 0
		case wmClose:
			if d.running { // the Fix software window stops first; that of the details waits for the end of its commands
				if d.bStop != 0 {
					d.stop.Store(true)
					pEnableWindow.Call(d.bStop, 0)
				}
				return 0
			}
			a.closeFix()
			return 0
		}
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
	return r
}

// has: p is in r.
func (r rect) has(p point) bool {
	return p.X >= r.Left && p.X < r.Right && p.Y >= r.Top && p.Y < r.Bottom
}

// pEnabled: the control can be used.
func pEnabled(h uintptr) bool {
	r, _, _ := pIsWindowEnabled.Call(h)
	return r != 0
}

// refreshDetails shows the details window again with the element as it is now.
func (a *ui) refreshDetails() {
	d := a.dlg
	if d == nil {
		return
	}
	it, ok := a.itemFor(d.item.ID)
	if !ok {
		return
	}
	old := d.view.hwnd
	d.view.destroy()
	pDestroyWindow.Call(old) // else, over the new page, it would get the clicks
	d.item, d.blocks, d.cmds = it, detailBlocks(it), nil
	for _, b := range d.blocks {
		d.cmds = append(d.cmds, b.cmds...)
	}
	d.view = a.newDocView(d.hwnd, d.blocks)
	d.view.onStatus = func(s string) { setText(d.status, s) }
	d.view.onCopyAll = func() { d.copyAll() }
	pEnableWindow.Call(d.bCmds, boolArg(len(d.cmds) > 0))
	a.setDetailsFix(d) // fixed: Restore the backup; restored: Fix this software
	d.fitButtons(a)
	d.layout(a)
	pSetFocus.Call(d.view.hwnd)
}
