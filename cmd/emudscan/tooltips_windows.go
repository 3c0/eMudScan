//go:build windows

package main

// Tooltips of the list cells. Each list has one tooltip whose tool is the cell under
// the mouse, so the tip disappears as soon as the mouse leaves that cell. The first
// cell (the name) shows a summary of the element, paths are always shown in full,
// other cells only when their text is cut.

import (
	"strings"
	"unsafe"
)

const (
	ttmSetMaxTipWidth  = 0x0418
	ttmPop             = 0x041C
	ttmAddToolW        = 0x0432
	ttmNewToolRectW    = 0x0434
	ttmUpdateTipTextW  = 0x0439
	ttmSetDelayTime    = 0x0403
	wmHScroll          = 0x0114
	lvhtOnItem         = 0x0E // LVHT_ONITEMICON|LVHT_ONITEMLABEL|LVHT_ONITEMSTATEICON
	lvirBounds, lvirLb = 0, 2
)

type cellTip struct {
	tip      uintptr
	row, col int32 // cell of the tool; -2 = to be found again
}

var cellTips = map[uintptr]*cellTip{}

// addTip gives controls of the main window a tooltip with a fixed text; it returns
// the tooltip window.
func (a *ui) addTip(ctls []uintptr, text string) uintptr {
	tip, _, _ := pCreateWindowExW.Call(0x8 /* WS_EX_TOPMOST */, up("tooltips_class32"), 0, 0x80000000|0x1|0x2, /* WS_POPUP|TTS_ALWAYSTIP|TTS_NOPREFIX */
		0x80000000, 0x80000000, 0x80000000, 0x80000000, a.hwnd, 0, 0, 0)
	if tip == 0 {
		return 0
	}
	send(tip, ttmSetMaxTipWidth, 0, uintptr(a.scale(320)))
	send(tip, ttmSetDelayTime, 2 /* TTDT_AUTOPOP */, 30000)
	for _, c := range ctls {
		ti := toolInfo{Flags: 0x1 | 0x10 /* TTF_IDISHWND|TTF_SUBCLASS */, Hwnd: a.hwnd, ID: c, Text: u16(text)}
		ti.Size = uint32(unsafe.Sizeof(ti))
		send(tip, ttmAddToolW, 0, uintptr(unsafe.Pointer(&ti)))
	}
	return tip
}

// newTooltip creates a tooltip window for the tools of a window (a dialog).
func (a *ui) newTooltip(owner uintptr) uintptr {
	tip, _, _ := pCreateWindowExW.Call(0x8 /* WS_EX_TOPMOST */, up("tooltips_class32"), 0, 0x80000000|0x1|0x2, /* WS_POPUP|TTS_ALWAYSTIP|TTS_NOPREFIX */
		0x80000000, 0x80000000, 0x80000000, 0x80000000, owner, 0, 0, 0)
	if tip != 0 {
		send(tip, ttmSetMaxTipWidth, 0, uintptr(a.scale(560)))
		send(tip, ttmSetDelayTime, 2 /* TTDT_AUTOPOP */, 30000)
	}
	return tip
}

// addTool adds to a tooltip a control of a window (id: the control), or (rc not nil)
// a rectangle of a window's client area (id: its number in that window).
func addTool(tip, hwnd, id uintptr, rc *rect, text string) {
	ti := toolInfo{Flags: 0x1 | 0x10 /* TTF_IDISHWND|TTF_SUBCLASS */, Hwnd: hwnd, ID: id, Text: u16(strings.ReplaceAll(text, "\n", "\r\n"))}
	if rc != nil {
		ti.Flags, ti.Rect = 0x10 /* TTF_SUBCLASS */, *rc
	}
	ti.Size = uint32(unsafe.Sizeof(ti))
	send(tip, ttmAddToolW, 0, uintptr(unsafe.Pointer(&ti)))
}

// addCellTips creates the tooltip of a list.
func (a *ui) addCellTips(list uintptr) {
	tip, _, _ := pCreateWindowExW.Call(0x8 /* WS_EX_TOPMOST */, up("tooltips_class32"), 0, 0x80000000|0x1|0x2, /* WS_POPUP|TTS_ALWAYSTIP|TTS_NOPREFIX */
		0x80000000, 0x80000000, 0x80000000, 0x80000000, list, 0, 0, 0)
	if tip == 0 {
		return
	}
	ti := toolInfo{Flags: 0x10 | 0x100 /* TTF_SUBCLASS|TTF_TRANSPARENT */, Hwnd: list, ID: 1, Text: u16(" ")}
	ti.Size = uint32(unsafe.Sizeof(ti))
	send(tip, ttmAddToolW, 0, uintptr(unsafe.Pointer(&ti)))
	send(tip, ttmSetMaxTipWidth, 0, uintptr(a.scale(640)))
	send(tip, ttmSetDelayTime, 2 /* TTDT_AUTOPOP */, 30000)
	cellTips[list] = &cellTip{tip: tip, row: -2, col: -2}
}

// resetCellTip hides the tip of a list; it is placed again at the next mouse move.
func resetCellTip(list uintptr) {
	if ct := cellTips[list]; ct != nil {
		send(ct.tip, ttmPop, 0, 0)
		ct.row, ct.col = -2, -2
	}
}

// updateCellTip moves the tool of a list to the cell under the mouse.
func (a *ui) updateCellTip(list uintptr, pt point) {
	ct := cellTips[list]
	if ct == nil {
		return
	}
	row, col := int32(-1), int32(-1)
	ht := lvHitTestInfo{Pt: pt}
	if int32(send(list, lvmSubItemHitTest, 0, uintptr(unsafe.Pointer(&ht)))) >= 0 && ht.Item >= 0 && ht.Flags&lvhtOnItem != 0 {
		row, col = ht.Item, ht.SubItem
	}
	if row == ct.row && col == ct.col {
		return
	}
	send(ct.tip, ttmPop, 0, 0)
	ct.row, ct.col = row, col
	var text string
	var rc rect
	if row >= 0 {
		text, rc = a.cellTipText(list, row, col)
	}
	if text == "" {
		rc, text = rect{}, " "
	}
	ti := toolInfo{Hwnd: list, ID: 1, Rect: rc, Text: u16(strings.ReplaceAll(text, "\n", "\r\n"))}
	ti.Size = uint32(unsafe.Sizeof(ti))
	send(ct.tip, ttmNewToolRectW, 0, uintptr(unsafe.Pointer(&ti)))
	send(ct.tip, ttmUpdateTipTextW, 0, uintptr(unsafe.Pointer(&ti)))
}

// cellTipText returns the tip of a cell ("" for none) and the rectangle of the cell.
func (a *ui) cellTipText(list uintptr, row, col int32) (string, rect) {
	li := lvItem{Mask: lvifParam, Item: row}
	send(list, lvmGetItemW, 0, uintptr(unsafe.Pointer(&li)))
	it, ok := a.itemFor(int(li.LParam))
	if !ok {
		return "", rect{}
	}
	var full string
	isPath := false
	if list == a.hListWin {
		switch col {
		case 0:
			full = it.Name
		case 1:
			full = statusText(it.Status)
		case 2:
			full = it.Summary
		}
	} else {
		full = appCell(it, int(col))
		isPath = col == colFile || col == colFolder
		if u := it.Updated; col == colUpdated && u.shown() { // where the date comes from, or why it is unknown
			full, isPath = u.text(u.File), true
			if m := it.stale(); m > 0 {
				full += "\n" + Tf("stale.tip", ageText(m))
			}
		}
	}
	full = strings.TrimSpace(full)
	if full == "" && col != 0 {
		return "", rect{}
	}
	var rc rect
	if col == 0 { // the first column: from the left of the row to the end of its label
		bounds, label := rect{Left: lvirBounds}, rect{Left: lvirLb}
		send(list, lvmGetItemRect, uintptr(row), uintptr(unsafe.Pointer(&bounds)))
		send(list, lvmGetItemRect, uintptr(row), uintptr(unsafe.Pointer(&label)))
		rc = rect{bounds.Left, label.Top, label.Right, label.Bottom}
		title := it.Name
		if list != a.hListWin {
			title = itemTitle(it)
		}
		summary := statusText(it.Status)
		if s := strings.TrimSpace(it.Summary); s != "" {
			summary += " — " + s
		}
		return title + "\n" + summary, rc
	}
	rc = rect{Top: col, Left: lvirBounds}
	send(list, lvmGetSubItemRect, uintptr(row), uintptr(unsafe.Pointer(&rc)))
	if !isPath && a.textWidth(a.fontUI, oneLine(full)) <= rc.Right-rc.Left-a.scale(14) && !strings.Contains(full, "\n") {
		return "", rect{}
	}
	return full, rc
}
