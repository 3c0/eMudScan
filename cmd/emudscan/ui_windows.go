//go:build windows

package main

// The main window: progress log, Windows list, Internet programs list and
// third-party programs list in four frames separated by draggable splitters.

import (
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

type logRun struct {
	text  string
	color uint32
	bold  bool
	sym   bool // status mark: drawn with the symbol font
}

type uiEvent struct {
	kind  int // 0 log, 1 item, 2 activity, 3 done
	lkind LogKind
	st    Status
	text  string
	item  Item
}

// listCtl is one of the two program lists (Internet programs, third-party programs).
type listCtl struct {
	h, hide   uintptr
	group     Group
	caption   string
	rows      []int // row -> item ID
	sortKeys  []sortKey
	hideOK    bool
	userSized [numAppCols]bool
	lastW     int32
	dirty     bool
}

type driveCtl struct {
	h     uintptr
	root  string
	label string
}

type ui struct {
	hwnd, hLogo, hTitle, hInfo, hLangLbl, hLang, hDrivesLbl uintptr
	drives                                                  []driveCtl
	hCheck, hCopy, hSave, hSearch                           uintptr
	query                                                   []string // the words of the search box (search.go)
	hLog, hSpin, hListWin                                   uintptr
	net, app                                                *listCtl
	log                                                     *logView
	hSummary                                                uintptr
	hOnline, hOnlineLbl, onlineTip                          uintptr   // the box that allows the online test, its label, their tooltip
	keyTips                                                 []uintptr // tooltips giving the buttons' keyboard shortcuts
	onlineOff                                               bool      // the user turned the online test off (not saved)

	dpi                                   int
	fontUI, fontTitle, fontMono, fontLink uintptr
	imgStatus                             uintptr // status icons of the lists
	spin                                  *spinner
	doneIcon, infoIcon, logoIcon          uintptr
	brushWindow                           uintptr
	curArrow, curHand, curSizeNS          uintptr

	sep          rect        // between the online test and the drives (drawn by the main window)
	frames       [4]frameBox // log, Windows, Internet and third-party frames (drawn by the main window)
	heights      [3]int32    // heights of the log, Windows and Internet frames chosen by the user (0 = default)
	splits       [3]rect
	dragging     int
	dragY, dragH int32

	items  map[int]Item
	shown  map[int]Item // program lists: the merged elements, by the ID of their first copy (repeated.go)
	winIdx map[int]int  // Windows list: item ID -> row

	running   bool
	stopping  bool
	startTime time.Time
	spinDrawn time.Time // when the spinner was last drawn during a check
	step      string    // the current step of the log (shown when no file is being read)
	// position of the spinner (it is sized to its text)
	spinX, spinY, spinRight int32
	activity                string
	spinText                string
	current                 *checker
	cancel                  atomic.Bool
	langSel                 int   // language list: 0 automatic, 1 English, 2 French
	rowH                    int32 // height of a list row

	logPending bool
	logText    strings.Builder

	detailQueue []Item
	dlg         *detailsDlg
	detailBack  []detailsBack // details left through a link, shown again when closed
	navigating  bool
}

var g = &ui{items: map[int]Item{}, shown: map[int]Item{}, winIdx: map[int]int{}, dragging: -1}

var (
	evMu    sync.Mutex
	evQueue []uiEvent
)

func postEvent(e uiEvent) {
	evMu.Lock()
	evQueue = append(evQueue, e)
	first := len(evQueue) == 1
	evMu.Unlock()
	if first {
		pPostMessageW.Call(g.hwnd, wmAppEvents, 0, 0)
	}
}

type guiReporter struct{}

func (guiReporter) Log(kind LogKind, st Status, text string) {
	postEvent(uiEvent{kind: 0, lkind: kind, st: st, text: text})
}
func (guiReporter) Upsert(it Item)       { postEvent(uiEvent{kind: 1, item: it}) }
func (guiReporter) Activity(text string) { postEvent(uiEvent{kind: 2, text: text}) }

func (a *ui) scale(v int) int32 { return int32(v * a.dpi / 96) }

func (a *ui) lists() []*listCtl { return []*listCtl{a.net, a.app} }

func (a *ui) listOf(h uintptr) *listCtl {
	for _, l := range a.lists() {
		if l != nil && l.h == h && h != 0 {
			return l
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Resources

func (a *ui) createResources() {
	for _, f := range []uintptr{a.fontUI, a.fontTitle, a.fontMono, a.fontLink} {
		if f != 0 {
			pDeleteObject.Call(f)
		}
	}
	a.fontUI = createFont(9, a.dpi, 400, false, "Segoe UI")
	a.fontTitle = createFont(14, a.dpi, 600, false, "Segoe UI")
	a.fontMono = createFont(9, a.dpi, 400, false, "Consolas")
	a.fontLink = createFont(9, a.dpi, 400, true, "Segoe UI")
	for _, ic := range glyphIcons {
		pDestroyIcon.Call(ic)
	}
	glyphIcons = map[[3]uint32]uintptr{}
	size := int(a.scale(16))
	pad := int(a.scale(3)) // space between the icon and the text of a row
	// one image list for the three lists (they have LVS_SHAREIMAGELISTS)
	il, _, _ := pImageListCreate.Call(uintptr(size+pad), uintptr(size), 0x20 /* ILC_COLOR32 */, 6, 1)
	for st := StPending; st <= StCaution; st++ { // the image index is the status
		px := renderStatus(st, size)
		wide := make([]byte, (size+pad)*size*4)
		for y := 0; y < size; y++ {
			copy(wide[y*(size+pad)*4:], px[y*size*4:(y+1)*size*4])
		}
		ic := iconFromRGBAWH(wide, size+pad, size)
		pImageListReplaceIcon.Call(il, ^uintptr(0), ic)
		pDestroyIcon.Call(ic)
	}
	old := a.imgStatus
	a.imgStatus = il
	if a.hListWin != 0 {
		for _, h := range []uintptr{a.hListWin, a.net.h, a.app.h} {
			send(h, lvmSetImageList, 1, a.imgStatus)
		}
	}
	if old != 0 {
		pImageListDestroy.Call(old)
	}
	for _, ic := range []uintptr{a.doneIcon, a.infoIcon, a.logoIcon} {
		if ic != 0 {
			pDestroyIcon.Call(ic)
		}
	}
	if a.spin != nil {
		a.spin.destroy()
	}
	ssz := int(a.scale(20))
	a.spin = newSpinner(ssz) // drawn in the background: only needed once a check runs
	a.doneIcon = iconFromRGBA(renderStatus(StOK, ssz), ssz)
	isz := int(a.infoSize())
	a.infoIcon = iconFromRGBA(renderIcon(iconInfo, isz), isz)
	lsz := int(a.logoSize())
	a.logoIcon = iconFromRGBA(renderIcon(iconApp, lsz), lsz)
	a.refreshButtonIcons()
}

func (a *ui) applyFonts() {
	hs := []uintptr{a.hLangLbl, a.hLang, a.hDrivesLbl, a.hCheck, a.hSearch, a.hCopy, a.hSave, a.hListWin, a.hSummary}
	for _, l := range a.lists() {
		hs = append(hs, l.h, l.hide)
	}
	for _, d := range a.drives {
		hs = append(hs, d.h)
	}
	for _, h := range hs {
		send(h, wmSetFont, a.fontUI, 1)
	}
	send(a.hTitle, wmSetFont, a.fontTitle, 1)
	a.setLogFont()
}

func (a *ui) child(exStyle uint32, class, text string, style uint32, id int) uintptr {
	h, _, _ := pCreateWindowExW.Call(uintptr(exStyle), up(class), up(text),
		uintptr(wsChild|wsVisible|style), 0, 0, 10, 10, a.hwnd, uintptr(id), 0, 0)
	return h
}

// ---------------------------------------------------------------------------
// Controls

var (
	listSubclassProc = syscall.NewCallback(listSubclass)
	spinProc         = syscall.NewCallback(spinWndProc)
	infoProc         = syscall.NewCallback(infoWndProc)
	logoProc         = syscall.NewCallback(logoWndProc)
)

func (a *ui) setupList(h uintptr, cols []string) {
	pSetWindowTheme.Call(h, up("Explorer"), 0)
	send(h, lvmSetExtendedStyle, 0, 0x20|0x10000) // FULLROWSELECT|DOUBLEBUFFER (tooltips: see tooltips_windows.go)
	for i, key := range cols {
		col := lvColumn{Mask: 0x1 | 0x2 | 0x4 | 0x8, Cx: 100, Text: u16(T(key)), SubItem: int32(i)}
		send(h, lvmInsertColumnW, uintptr(i), uintptr(unsafe.Pointer(&col)))
	}
}

func (a *ui) createControls() {
	a.net = &listCtl{group: GroupNet, caption: "grp.net", sortKeys: []sortKey{{col: colStatus}, {col: colProgram}}}
	a.app = &listCtl{group: GroupApps, caption: "grp.apps", sortKeys: []sortKey{{col: colStatus}, {col: colProgram}}}
	a.frames = [4]frameBox{{caption: "grp.progress"}, {caption: "grp.windows"}, {caption: a.net.caption, list: a.net}, {caption: a.app.caption, list: a.app}}
	a.hLogo = a.child(0, "eMudScanLogo", "", 0, 0)
	a.hTitle = a.child(0, "STATIC", appTitle(), 0x80, 0)
	a.hInfo = a.child(0, "eMudScanIcon", T("btn.about"), 0, 0)
	a.hLangLbl = a.child(0, "STATIC", T("lbl.language"), 0x80|0x2|0x200 /* NOPREFIX|RIGHT|CENTERIMAGE */, 0)
	a.hLang = a.child(0, "COMBOBOX", "", 0x3 /* CBS_DROPDOWNLIST */ |wsTabStop|wsVScroll, idLang)
	for _, label := range []string{T("lang.auto"), "English", "Français"} {
		send(a.hLang, cbAddString, 0, up(label))
	}
	send(a.hLang, cbSetCurSel, uintptr(a.langSel), 0)
	a.hDrivesLbl = a.child(0, "STATIC", T("lbl.drives"), 0x80|0x200 /* NOPREFIX|CENTERIMAGE */, 0)
	for i, d := range listDrives() {
		h := a.child(0, "BUTTON", d.Label, 3|wsTabStop, idDrive0+i)
		if d.Fixed {
			send(h, bmSetCheck, 1, 0)
		}
		a.drives = append(a.drives, driveCtl{h, d.Root, d.Label})
	}
	// the online test, which the user can refuse; its tooltip shows what it sends
	a.hOnline = a.child(0, "BUTTON", "", 3|wsTabStop, idOnline) // the box; its label is drawn by optin_windows.go
	if !a.onlineOff {
		send(a.hOnline, bmSetCheck, 1, 0)
	}
	a.hOnlineLbl = a.child(0, "eMudScanOptIn", T("chk.online"), 0, 0)
	a.onlineTip = a.addTip([]uintptr{a.hOnline, a.hOnlineLbl}, Tf("tip.online", userAgent()))
	a.hCheck = a.iconButton(a.hwnd, T("btn.check"), idCheck, true, glyphPlay)
	// the search box (ES_AUTOHSCROLL, with a border), shown with a hint while empty
	a.hSearch = a.child(0x200 /* WS_EX_CLIENTEDGE */, "EDIT", "", wsTabStop|0x80, idSearch)
	send(a.hSearch, 0x1501 /* EM_SETCUEBANNER */, 1, up(T("search.cue")))
	a.query = nil
	a.hCopy = a.iconButton(a.hwnd, T("btn.copy"), idCopy, false, glyphCopy)
	a.hSave = a.iconButton(a.hwnd, T("btn.save"), idSave, false, glyphSave)
	for _, k := range []struct {
		h   uintptr
		key string
	}{{a.hCheck, "F5"}, {a.hCopy, "Ctrl+Shift+C"}, {a.hSave, "Ctrl+S"}} {
		a.keyTips = append(a.keyTips, a.addTip([]uintptr{k.h}, Tf("tip.key", k.key)))
	}
	a.hSpin = a.child(0, "eMudScanSpinner", "", 0, 0)
	a.log = a.newLogView(a.hwnd, idLog)
	a.hLog = a.log.hwnd
	// LVS_REPORT|LVS_SHOWSELALWAYS|LVS_SHAREIMAGELISTS (the image lists are shared and kept across resets)
	a.hListWin = a.child(wsExClientEdge, "SysListView32", "", 0x1|0x8|0x40|0x8000|wsTabStop, idListWin)
	a.net.h = a.child(wsExClientEdge, "SysListView32", "", 0x1|0x8|0x40|wsTabStop, idListNet)
	a.net.hide = a.child(0, "BUTTON", T("chk.hideOK"), 3|wsTabStop, idHideNet)
	a.app.h = a.child(wsExClientEdge, "SysListView32", "", 0x1|0x8|0x40|wsTabStop, idListApp)
	a.app.hide = a.child(0, "BUTTON", T("chk.hideOK"), 3|wsTabStop, idHideApp)
	a.hSummary = a.child(0, "STATIC", T("hint.start"), 0x80|0x4000, 0)
	pShowWindow.Call(a.hSpin, 0)
	pEnableWindow.Call(a.hCopy, 0)
	pEnableWindow.Call(a.hSave, 0)

	a.setupList(a.hListWin, []string{"col.element", "col.status", "col.details"})
	for _, l := range a.lists() {
		a.setupList(l.h, appColKeys[:])
	}
	cellTips = map[uintptr]*cellTip{}
	for _, h := range []uintptr{a.hListWin, a.net.h, a.app.h} {
		pSetWindowSubclass.Call(h, listSubclassProc, 1, 0)
		a.addCellTips(h)
	}
	for _, h := range []uintptr{a.hListWin, a.net.h, a.app.h} {
		send(h, lvmSetImageList, 1, a.imgStatus)
	}
	a.applyFonts()
	a.measureRow()
	for _, l := range a.lists() {
		a.updateSortArrows(l)
	}
}

// measureRow measures the height of a list row (font and icon size).
func (a *ui) measureRow() {
	l := a.hListWin
	temp := send(l, lvmGetItemCount, 0, 0) == 0
	if temp {
		li := lvItem{Mask: lvifText | lvifImage, Text: u16("X"), Image: 1}
		send(l, lvmInsertItemW, 0, uintptr(unsafe.Pointer(&li)))
	}
	rc := rect{Left: 0 /* LVIR_BOUNDS */}
	if send(l, lvmGetItemRect, 0, uintptr(unsafe.Pointer(&rc))) != 0 && rc.Bottom > rc.Top {
		a.rowH = rc.Bottom - rc.Top
	}
	if temp {
		send(l, lvmDeleteAllItems, 0, 0)
	}
}

// fontMetrics returns the metrics of a font.
func (a *ui) fontMetrics(font uintptr) textMetric {
	dc, _, _ := pGetDC.Call(a.hwnd)
	old, _, _ := pSelectObject.Call(dc, font)
	var tm textMetric
	pGetTextMetricsW.Call(dc, uintptr(unsafe.Pointer(&tm)))
	pSelectObject.Call(dc, old)
	pReleaseDC.Call(a.hwnd, dc)
	return tm
}

// ---------------------------------------------------------------------------
// Layout and splitters

func (a *ui) textWidth(font uintptr, s string) int32 {
	dc, _, _ := pGetDC.Call(a.hwnd)
	old, _, _ := pSelectObject.Call(dc, font)
	var sz point
	u := utf16z(s)
	pGetTextExtentPoint32.Call(dc, uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)-1), uintptr(unsafe.Pointer(&sz)))
	pSelectObject.Call(dc, old)
	pReleaseDC.Call(a.hwnd, dc)
	return sz.X
}

func (a *ui) move(h uintptr, x, y, w, hh int32) {
	// NOZORDER|NOACTIVATE|NOCOPYBITS: a moved control is drawn again entirely (the lists
	// and frames are drawn off screen, so this does not blink), never pieced together
	// from its old image
	pSetWindowPos.Call(h, 0, uintptr(x), uintptr(y), uintptr(max(w, 0)), uintptr(max(hh, 0)), 0x0004|0x0010|0x0100)
}

func (a *ui) redrawAll() {
	pRedrawWindow.Call(a.hwnd, 0, 0, 0x1|0x4|0x80|0x100) // INVALIDATE|ERASE|ALLCHILDREN|UPDATENOW
}

// ncHeight is the height of the border (non-client area) of a control.
func ncHeight(h uintptr) int32 {
	var wr, cr rect
	pGetWindowRect.Call(h, uintptr(unsafe.Pointer(&wr)))
	pGetClientRect.Call(h, uintptr(unsafe.Pointer(&cr)))
	return wr.Bottom - wr.Top - cr.Bottom
}

// listHeight returns the height of a list that shows exactly n rows: its border
// (with the horizontal scroll bar, when shown), the header and n rows. The rows
// start where the first visible row of the list is, else right under the header.
func (a *ui) listHeight(l uintptr, n int32) int32 {
	first := int32(0)
	if send(l, lvmGetItemCount, 0, 0) > 0 {
		rc := rect{Left: 0 /* LVIR_BOUNDS */}
		if send(l, lvmGetItemRect, send(l, lvmGetTopIndex, 0, 0), uintptr(unsafe.Pointer(&rc))) != 0 {
			first = rc.Top
		}
	}
	if first <= 0 {
		var hr rect
		pGetWindowRect.Call(send(l, lvmGetHeader, 0, 0), uintptr(unsafe.Pointer(&hr)))
		first = hr.Bottom - hr.Top
	}
	if first <= 0 {
		first = a.scale(26)
	}
	row := a.rowH
	if row <= 0 {
		row = a.scale(22)
	}
	return ncHeight(l) + first + n*row
}

func (a *ui) layout() {
	if a.app == nil || a.app.h == 0 {
		return
	}
	var rc rect
	pGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&rc)))
	W, H := rc.Right, rc.Bottom
	if W <= 0 || H <= 0 {
		return
	}
	s := a.scale
	m := s(14)
	y := m

	// logo, title, ⓘ and language, centred on the middle of the title's capitals
	tw := a.textWidth(a.fontTitle, appTitle())
	tm := a.fontMetrics(a.fontTitle)
	cy := y + (tm.InternalLeading+tm.Ascent)/2
	lsz := a.logoSize()
	a.move(a.hLogo, m, cy-lsz/2, lsz, lsz)
	tx := m + lsz + s(8)
	a.move(a.hTitle, tx, y, tw+s(4), tm.Height)
	isz := a.infoSize() + s(4) // the ⓘ with a small margin around it to click
	a.move(a.hInfo, tx+tw+s(8), cy-isz/2, isz, isz)
	langW := max(s(190), a.textWidth(a.fontUI, T("lang.auto"))+s(40))
	a.move(a.hLang, W-m-langW, y, langW, s(200))
	var cr rect
	pGetWindowRect.Call(a.hLang, uintptr(unsafe.Pointer(&cr)))
	ch := cr.Bottom - cr.Top
	a.move(a.hLang, W-m-langW, cy-ch/2, langW, s(200))
	lw := a.textWidth(a.fontUI, T("lbl.language")) + s(4)
	a.move(a.hLangLbl, W-m-langW-s(8)-lw, cy-ch/2, lw, ch)
	y = max(y+tm.Height, cy+ch/2, cy+lsz/2) + s(10)

	// the box that allows the online test, a separator, then the drives (wrapped
	// when there are many)
	dl := a.textWidth(a.fontUI, T("lbl.drives")) + s(6)
	rowH := s(26)
	a.move(a.hOnline, m, y, s(18), rowH) // the box alone
	lx := m + s(18) + s(2)
	olw := a.textWidth(a.fontUI, T("chk.online")) + s(2)
	a.move(a.hOnlineLbl, lx, y, olw, rowH)
	sepX := lx + olw + s(14)
	rowTop := y
	a.move(a.hDrivesLbl, sepX+s(15), y, dl, rowH)
	x0 := sepX + s(15) + dl + s(8)
	x := x0
	for _, d := range a.drives {
		w := a.textWidth(a.fontUI, d.label) + s(28)
		if x > x0 && x+w > W-m {
			x = x0
			y += rowH
		}
		a.move(d.h, x, y, w, rowH)
		x += w + s(12)
	}
	a.sep = rect{sepX, rowTop + s(3), sepX + max(s(1), 1), y + rowH - s(3)}
	y += s(32)

	// buttons: Scan on the left, Copy report and Save report on the far right
	bh := s(30)
	btnW := func(least int, keys ...string) int32 {
		w := int32(0)
		for _, k := range keys {
			w = max(w, a.textWidth(a.fontUI, T(k)))
		}
		return max(s(least), w+s(16)+s(44)) // icon and margins
	}
	chw := btnW(150, "btn.check", "btn.again", "btn.stop")
	a.move(a.hCheck, m, y, chw, bh)
	sw, cw := btnW(150, "btn.save"), btnW(140, "btn.copy")
	a.move(a.hSave, W-m-sw, y, sw, bh)
	a.move(a.hCopy, W-m-sw-s(8)-cw, y, cw, bh)
	// the search box, before Copy report, as wide as there is room for (up to 360)
	eh := a.fontMetrics(a.fontUI).Height + s(10)
	ew := min(s(360), W-m-sw-s(8)-cw-s(24)-(m+chw+s(24)))
	a.move(a.hSearch, W-m-sw-s(8)-cw-s(24)-ew, y+(bh-eh)/2, max(ew, 0), eh)
	y += bh + s(12)

	// four frames
	sumH := s(20)
	bottom := H - m - sumH - s(6)
	gap := s(10)
	avail := bottom - y
	mins := [4]int32{s(80), s(80), s(80), s(110)}
	pad, top := s(10), s(22)
	// whole lines: each frame is fitted so that its log or list shows whole lines
	// (rounded down), none cut at the bottom; not while a frame border is dragged,
	// which follows the mouse
	whole := func(i int, fh int32) int32 {
		var base, line int32
		if i == 0 {
			base, line = ncHeight(a.hLog)+2*a.log.padY(), a.log.pitch
		} else {
			l := []uintptr{a.hListWin, a.net.h, a.app.h}[i-1]
			base = a.listHeight(l, 0)
			line = a.listHeight(l, 1) - a.listHeight(l, 0)
		}
		if line <= 0 || a.dragging >= 0 {
			return fh
		}
		n := max((fh-top-pad-base)/line, 1)
		return top + pad + base + n*line
	}
	h := a.heights
	if h[0] == 0 {
		h[0] = max(avail*22/100, s(110))
	}
	if h[1] == 0 {
		h[1] = s(22) + a.listHeight(a.hListWin, winChecks) + s(10) // all the Windows checks, without scrolling
	}
	if h[2] == 0 {
		h[2] = max(avail*20/100, s(22)+a.listHeight(a.net.h, 3)+s(10))
	}
	if over := h[0] + h[1] + h[2] + 3*gap + mins[3] - avail; over > 0 {
		for _, i := range []int{0, 2, 1} {
			cut := min(over, max(h[i]-mins[i], 0))
			h[i] -= cut
			over -= cut
		}
	}
	for i := 0; i < 3; i++ {
		h[i] = whole(i, h[i])
	}
	frame := func(i int, inner uintptr, fy, fh int32) {
		a.frames[i].rc = rect{m, fy, W - m, fy + fh}
		a.move(inner, m+pad, fy+top, W-2*m-2*pad, fh-top-pad)
	}
	var ys [4]int32
	ys[0] = y
	for i := 1; i < 4; i++ {
		ys[i] = ys[i-1] + h[i-1] + gap
	}
	frame(0, a.hLog, ys[0], h[0])
	frame(1, a.hListWin, ys[1], h[1])
	frame(2, a.net.h, ys[2], h[2])
	frame(3, a.app.h, ys[3], whole(3, max(bottom-ys[3], mins[3])))
	for i := 0; i < 3; i++ {
		a.splits[i] = rect{m, ys[i] + h[i], W - m, ys[i+1]}
	}
	a.move(a.hSummary, m, H-m-sumH, W-2*m, sumH)

	// the spinner right after "Log", "Hide OK items" right after the captions of
	// the program lists, centred on the frame's top line; each one ends with a space
	mid := a.textHeight() / 2
	a.spinRight = W - m - s(12)
	a.spinX, a.spinY = a.captionEnd(m, T("grp.progress")), ys[0]+mid-s(11)
	a.move(a.hSpin, a.spinX, a.spinY, a.spinRight-a.spinX, s(22)) // fixed: it draws the frame's line after its text
	a.redrawSpinner()
	for i, l := range a.lists() { // "Title · [ ☐ Hide items with an OK status ] ───"
		x := a.captionEnd(m, T(l.caption)) + a.textWidth(a.fontUI, hidePrefix)
		bw := a.checkWidth(l.hide, T("chk.hideOK"))
		a.move(l.hide, x, ys[2+i]+mid-s(11), bw, s(22))
		a.frames[2+i].boxRight = x + bw
	}

	a.fitWinColumns()
	for _, l := range a.lists() {
		a.fitColumns(l, false)
	}
}

// listClientWidth is the width available to the columns of a list: its client width,
// so that the last column reaches the right side. When the vertical scroll bar
// appears or disappears, the list gets a WM_SIZE and its columns are fitted again.
func (a *ui) listClientWidth(l uintptr) int32 {
	var rc rect
	pGetClientRect.Call(l, uintptr(unsafe.Pointer(&rc)))
	return rc.Right
}

const autosizeUseHeader = ^uintptr(1) // LVSCW_AUTOSIZE_USEHEADER (-2)

// autosize fits a column to its longest text and to its header (with room for
// the sort arrow), within limit when limit > 0.
func (a *ui) autosize(h uintptr, col int, header string, limit int32) {
	send(h, lvmSetColumnWidth, uintptr(col), autosizeUseHeader)
	cw := max(int32(send(h, lvmGetColumnWidth, uintptr(col), 0)), a.textWidth(a.fontUI, T(header))+a.scale(42))
	if limit > 0 {
		cw = min(cw, limit)
	}
	send(h, lvmSetColumnWidth, uintptr(col), uintptr(cw))
}

// fitWinColumns: Element and Status fit their text, Details takes the rest. The widths
// are measured from the texts (not left to the list, whose automatic sizing varies
// while the list is resized) and only set when they change, so the columns stay put.
func (a *ui) fitWinColumns() {
	w := a.listClientWidth(a.hListWin)
	if w <= 0 {
		return
	}
	pad := a.scale(16)
	nameW := a.textWidth(a.fontUI, T("col.element")) + pad
	statusW := a.textWidth(a.fontUI, T("col.status")) + pad
	for _, it := range a.items {
		if it.Group == GroupWindows {
			nameW = max(nameW, a.scale(20)+a.scale(10)+a.textWidth(a.fontUI, it.Name)+pad) // status icon, then the name
			statusW = max(statusW, a.textWidth(a.fontUI, statusText(it.Status))+pad)
		}
	}
	for i, cw := range []int32{nameW, statusW, max(w-nameW-statusW, a.scale(120))} {
		if int32(send(a.hListWin, lvmGetColumnWidth, uintptr(i), 0)) != cw {
			send(a.hListWin, lvmSetColumnWidth, uintptr(i), uintptr(cw))
		}
	}
}

// fitColumns sizes the columns of a program list: Software, Component, Last update
// and Status fit their longest text, the two paths and Details share the rest.
// Columns resized by the user keep their width. Without force, nothing is done
// when the width of the list has not changed.
func (a *ui) fitColumns(l *listCtl, force bool) {
	w := a.listClientWidth(l.h)
	if w <= 0 || (!force && w == l.lastW) {
		return
	}
	l.lastW = w
	used := int32(0)
	limits := map[int]int32{colProgram: 28, colComponent: 22, colUpdated: 16, colStatus: 15}
	for _, c := range []int{colProgram, colComponent, colUpdated, colStatus} {
		if !l.userSized[c] {
			a.autosize(l.h, c, appColKeys[c], w*limits[c]/100)
		}
		used += int32(send(l.h, lvmGetColumnWidth, uintptr(c), 0))
	}
	share := map[int]int32{colFile: 36, colFolder: 28, colDetails: 36}
	total := int32(0)
	for _, c := range []int{colFile, colFolder, colDetails} {
		if l.userSized[c] {
			used += int32(send(l.h, lvmGetColumnWidth, uintptr(c), 0))
		} else {
			total += share[c]
		}
	}
	rest := w - used
	for _, c := range []int{colFile, colFolder, colDetails} {
		if !l.userSized[c] && total > 0 {
			send(l.h, lvmSetColumnWidth, uintptr(c), uintptr(max(rest*share[c]/total, a.scale(90))))
		}
	}
	a.refreshPathCells(l)
}

func (a *ui) splitAt(x, y int32) int {
	for i, r := range a.splits {
		if x >= r.Left && x < r.Right && y >= r.Top && y < r.Bottom {
			return i
		}
	}
	return -1
}

func (a *ui) frameHeight(i int) int32 {
	r := a.frames[i].rc
	return r.Bottom - r.Top
}

// ---------------------------------------------------------------------------
// Progress log (see logview_windows.go)

func (a *ui) setLogFont() {
	if a.log != nil {
		a.log.setFonts(a.dpi)
	}
}

// appendLog adds runs at the end of the log, which follows its end if it was
// showing it (or when forceScroll is set).
func (a *ui) appendLog(runs []logRun, forceScroll bool) {
	if len(runs) == 0 {
		return
	}
	a.log.append(runs, forceScroll)
	for _, r := range runs {
		saved := r.text
		if r.sym { // ✔ ℹ ⚠ ❗ ❌ are written as text marks in the report
			saved = reportMark(r.text)
		}
		a.logText.WriteString(saved)
	}
}

func statusColor(s Status) uint32 {
	switch s {
	case StOK:
		return rgb(30, 123, 52)
	case StCaution:
		return rgb(230, 108, 0) // orange
	case StWarn:
		return rgb(200, 36, 26)
	case StError:
		return rgb(122, 16, 34)
	case StInfo:
		return rgb(21, 101, 192)
	}
	return rgb(32, 32, 32)
}

func (a *ui) logRuns(e uiEvent) []logRun {
	normal := rgb(32, 32, 32)
	var runs []logRun
	if e.lkind != LogEnd && a.logPending {
		runs = append(runs, logRun{"\r\n", normal, false, false})
		a.logPending = false
	}
	switch e.lkind {
	case LogSection:
		runs = append(runs, logRun{"\r\n" + e.text + "\r\n", rgb(16, 58, 120), true, false})
	case LogStart:
		runs = append(runs, logRun{e.text, normal, false, false})
		a.logPending = true
	case LogEnd:
		prefix := " "
		if !a.logPending {
			prefix = "   "
		}
		bold := e.st == StError || e.st == StWarn || e.st == StCaution
		runs = append(runs, logRun{prefix, statusColor(e.st), false, false},
			logRun{text: logMark(e.st), color: statusColor(e.st), bold: bold, sym: true},
			logRun{" " + e.text + "\r\n", statusColor(e.st), bold, false})
		a.logPending = false
	default:
		if e.st != StPending {
			runs = append(runs, logRun{"   ", normal, false, false}, logRun{text: logMark(e.st), color: statusColor(e.st), bold: true, sym: true}, logRun{" ", normal, false, false})
		}
		runs = append(runs, logRun{e.text + "\r\n", normal, false, false})
	}
	return runs
}

// ---------------------------------------------------------------------------
// Small custom controls: the spinner beside "Log" and the ⓘ button

func spinWndProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	a := g
	switch msg {
	case wmEraseBkgnd:
		return 1
	case wmPaint:
		var ps paintStruct
		hdc, _, _ := pBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		var rc rect
		pGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
		if rc.Right > 0 && rc.Bottom > 0 {
			// drawn off screen, then copied at once: nothing is erased on screen
			dc, _, _ := pCreateCompatibleDC.Call(hdc)
			bmp, _, _ := pCreateCompatibleBmp.Call(hdc, uintptr(rc.Right), uintptr(rc.Bottom))
			oldBmp, _, _ := pSelectObject.Call(dc, bmp)
			a.paintSpinner(hwnd, dc, rc)
			pBitBlt.Call(hdc, 0, 0, uintptr(rc.Right), uintptr(rc.Bottom), dc, 0, 0, 0x00CC0020 /* SRCCOPY */)
			pSelectObject.Call(dc, oldBmp)
			pDeleteObject.Call(bmp)
			pDeleteDC.Call(dc)
		}
		pEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
	return r
}

// paintSpinner draws the spinner: the shield, the elapsed time and the current
// folder, then the frame's top line up to the right side, so that the window never
// changes size while a check runs.
func (a *ui) paintSpinner(hwnd, dc uintptr, rc rect) {
	pFillRect.Call(dc, uintptr(unsafe.Pointer(&rc)), a.brushWindow)
	isz := a.scale(20)
	space := a.textWidth(a.fontUI, " ")
	var icon uintptr
	text := a.spinnerText()
	if a.running {
		icon = a.spin.icon(int(time.Since(a.startTime) / spinFrameTime))
	} else if a.spinText != "" {
		icon = a.doneIcon
	}
	x := int32(0)
	if icon != 0 {
		pDrawIconEx.Call(dc, 0, uintptr((rc.Bottom-isz)/2), icon, uintptr(isz), uintptr(isz), 0, 0, 3)
		x = isz + space
	}
	end := x
	if text != "" {
		end = x + min(a.textWidth(a.fontUI, text), rc.Right-x-space)
		old, _, _ := pSelectObject.Call(dc, a.fontUI)
		pSetBkMode.Call(dc, 1 /* TRANSPARENT */)
		pSetTextColor.Call(dc, uintptr(sysColor(17)))
		tr := rect{x, 0, end, rc.Bottom}
		pDrawTextW.Call(dc, up(text), ^uintptr(0), uintptr(unsafe.Pointer(&tr)), 0x20|0x4|0x4000|0x800) // SINGLELINE|VCENTER|PATH_ELLIPSIS|NOPREFIX
		pSelectObject.Call(dc, old)
	}
	if end > 0 {
		end += space
	}
	// the frame's line, drawn exactly as the frame draws it (its border, seen from here)
	fr := a.frames[0].rc
	if fr.Right <= fr.Left || end >= rc.Right {
		return
	}
	var sr rect // the spinner, in the main window
	pGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&sr)))
	pScreenToClient.Call(a.hwnd, uintptr(unsafe.Pointer(&sr.Left)))
	saved, _, _ := pSaveDC.Call(dc)
	pExcludeClipRect.Call(dc, 0, 0, uintptr(end), uintptr(rc.Bottom))
	a.groupBorder(dc, rect{fr.Left - sr.Left, fr.Top - sr.Top, fr.Right - sr.Left, fr.Bottom - sr.Top})
	pRestoreDC.Call(dc, saved)
}

// checkWidth is the width of a check box with its text and nothing more.
func (a *ui) checkWidth(h uintptr, text string) int32 {
	var sz point
	if send(h, 0x1601 /* BCM_GETIDEALSIZE */, 0, uintptr(unsafe.Pointer(&sz))) != 0 && sz.X > 0 {
		return sz.X
	}
	return a.scale(16) + a.textWidth(a.fontUI, text) // the box and its gap, then the text
}

// spinnerText is the text beside the spinner: the elapsed time and what is being
// done during a check — the file or folder being read, else the current step of the
// log, so that the text never disappears between two steps —, then how long it took.
func (a *ui) spinnerText() string {
	if !a.running {
		return a.spinText
	}
	text := fmtElapsed(time.Since(a.startTime))
	if now := firstNonEmpty(a.activity, a.step); now != "" {
		text += "   " + now
	}
	return text
}

// spinFrameTime is the time each frame of the spinner is shown.
const spinFrameTime = 55 * time.Millisecond

// tickSpinner shows the next frame of the spinner, with the elapsed time and the
// current folder, at once. It is called by the timer and after each batch of
// progress messages: while a check sends many of them, the timer and the usual
// painting would wait for a pause, and the spinner would freeze.
func (a *ui) tickSpinner() {
	if !a.running || a.hSpin == 0 || time.Since(a.spinDrawn) < spinFrameTime {
		return
	}
	a.spinDrawn = time.Now()
	pInvalidateRect.Call(a.hSpin, 0, 0)
	pUpdateWindow.Call(a.hSpin)
}

// redrawSpinner draws the spinner again (off screen, see spinWndProc).
func (a *ui) redrawSpinner() {
	if a.hSpin != 0 {
		pInvalidateRect.Call(a.hSpin, 0, 0)
	}
}

func infoWndProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	a := g
	switch msg {
	case wmEraseBkgnd:
		return 1
	case wmPaint:
		var ps paintStruct
		dc, _, _ := pBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		var rc rect
		pGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
		pFillRect.Call(dc, uintptr(unsafe.Pointer(&rc)), a.brushWindow)
		isz := a.infoSize()
		pDrawIconEx.Call(dc, uintptr((rc.Right-isz)/2), uintptr((rc.Bottom-isz)/2), a.infoIcon, uintptr(isz), uintptr(isz), 0, 0, 3)
		pEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0
	case wmSetCursor:
		pSetCursor.Call(a.curHand)
		return 1
	case wmLButtonUp:
		a.about()
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
	return r
}

// infoSize is the size of the ⓘ after the title.
func (a *ui) infoSize() int32 { return a.scale(16) }

// logoSize is the size of the shield shown before the title.
func (a *ui) logoSize() int32 { return a.scale(32) }

// logoWndProc paints the shield of the program before the title.
func logoWndProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	a := g
	switch msg {
	case wmEraseBkgnd:
		return 1
	case wmPaint:
		var ps paintStruct
		dc, _, _ := pBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		var rc rect
		pGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
		pFillRect.Call(dc, uintptr(unsafe.Pointer(&rc)), a.brushWindow)
		sz := a.logoSize()
		pDrawIconEx.Call(dc, uintptr((rc.Right-sz)/2), uintptr((rc.Bottom-sz)/2), a.logoIcon, uintptr(sz), uintptr(sz), 0, 0, 3)
		pEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
	return r
}

// ---------------------------------------------------------------------------
// Lists

func (a *ui) setRow(l uintptr, row int, it Item, texts []string, insert bool) int {
	li := lvItem{Mask: lvifText | lvifImage | lvifParam, Item: int32(row), Text: u16(texts[0]), Image: int32(it.Status), LParam: uintptr(it.ID)}
	if insert {
		r := send(l, lvmInsertItemW, 0, uintptr(unsafe.Pointer(&li)))
		if int32(r) < 0 {
			return -1
		}
		row = int(r)
	} else {
		li.Mask = lvifText | lvifImage
		send(l, lvmSetItemW, 0, uintptr(unsafe.Pointer(&li)))
	}
	for i := 1; i < len(texts); i++ {
		sub := lvItem{SubItem: int32(i), Text: u16(texts[i])}
		send(l, lvmSetItemTextW, uintptr(row), uintptr(unsafe.Pointer(&sub)))
	}
	return row
}

func oneLine(s string) string { return strings.ReplaceAll(strings.ReplaceAll(s, "\r", ""), "\n", " ") }

func (a *ui) upsertWin(it Item) {
	texts := []string{it.Name, statusText(it.Status), oneLine(it.Summary)}
	if row, ok := a.winIdx[it.ID]; ok {
		a.setRow(a.hListWin, row, it, texts, false)
	} else {
		n := int(send(a.hListWin, lvmGetItemCount, 0, 0))
		if row := a.setRow(a.hListWin, n, it, texts, true); row >= 0 {
			a.winIdx[it.ID] = row
			send(a.hListWin, lvmEnsureVisible, uintptr(row), 0)
		}
	}
	a.fitWinColumns()
}

// compactPath shortens a path in the middle so that it fits width pixels.
func compactPath(dc uintptr, p string, width int32) string {
	fits := func(s string) bool {
		u := utf16z(s)
		var sz point
		pGetTextExtentPoint32.Call(dc, uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)-1), uintptr(unsafe.Pointer(&sz)))
		return sz.X <= width
	}
	if p == "" || fits(p) {
		return p
	}
	parts := strings.Split(p, `\`)
	if len(parts) < 3 {
		return p
	}
	head := parts[0] + `\`
	for k := 2; k < len(parts); k++ {
		if s := head + `…\` + strings.Join(parts[k:], `\`); fits(s) {
			return s
		}
	}
	return `…\` + parts[len(parts)-1]
}

// appTexts returns the cells of a row; with a DC, the paths are shortened to their column.
func (a *ui) appTexts(l *listCtl, it Item, dc uintptr) []string {
	t := make([]string, numAppCols)
	for c := 0; c < numAppCols; c++ {
		t[c] = oneLine(appCell(it, c))
	}
	if dc != 0 {
		for _, c := range []int{colFile, colFolder} {
			w := int32(send(l.h, lvmGetColumnWidth, uintptr(c), 0)) - a.scale(14)
			t[c] = compactPath(dc, t[c], w)
		}
	}
	return t
}

func (a *ui) withListDC(h uintptr, f func(dc uintptr)) {
	dc, _, _ := pGetDC.Call(h)
	old, _, _ := pSelectObject.Call(dc, a.fontLink)
	f(dc)
	pSelectObject.Call(dc, old)
	pReleaseDC.Call(h, dc)
}

func (a *ui) selectedIDs(h uintptr) []int {
	var ids []int
	i := ^uintptr(0)
	for {
		r := send(h, lvmGetNextItem, i, 2 /* LVNI_SELECTED */)
		if int32(r) < 0 {
			break
		}
		i = r
		li := lvItem{Mask: lvifParam, Item: int32(r)}
		send(h, lvmGetItemW, 0, uintptr(unsafe.Pointer(&li)))
		ids = append(ids, int(li.LParam))
	}
	return ids
}

func (a *ui) setSelected(h uintptr, row int, sel bool) {
	li := lvItem{StateMask: 2}
	if sel {
		li.State = 2
	}
	send(h, lvmSetItemState, uintptr(row), uintptr(unsafe.Pointer(&li)))
}

// rebuild refills a program list in sorted order, keeping the selection and scroll position.
func (a *ui) rebuild(l *listCtl) {
	l.dirty = false
	var items []Item
	for _, it := range a.items {
		if it.Group == l.group && !(l.hideOK && it.Status == StOK) {
			items = append(items, it)
		}
	}
	// a repeated finding is one row, shown by its first copy
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	items = mergeRepeated(items)
	for id, it := range a.shown {
		if it.Group == l.group {
			delete(a.shown, id)
		}
	}
	shown := items[:0]
	for _, it := range items {
		if len(it.Copies) > 0 {
			a.shown[it.ID] = it
		}
		if matchesQuery(it, a.query) { // the search box
			shown = append(shown, it)
		}
	}
	items = shown
	sortItems(items, l.sortKeys)
	selected := map[int]bool{}
	for _, id := range a.selectedIDs(l.h) {
		selected[id] = true
	}
	top := int32(send(l.h, lvmGetTopIndex, 0, 0))
	send(l.h, wmSetRedraw, 0, 0)
	send(l.h, lvmDeleteAllItems, 0, 0)
	l.rows = l.rows[:0]
	for i, it := range items {
		if a.setRow(l.h, i, it, a.appTexts(l, it, 0), true) >= 0 {
			l.rows = append(l.rows, it.ID)
			if selected[it.ID] {
				a.setSelected(l.h, i, true)
			}
		}
	}
	a.fitColumns(l, true) // also shortens the paths
	if top > 0 && len(items) > 0 {
		rc := rect{Left: 0 /* LVIR_BOUNDS */}
		send(l.h, lvmGetItemRect, 0, uintptr(unsafe.Pointer(&rc)))
		send(l.h, lvmScroll, 0, uintptr(top*(rc.Bottom-rc.Top)))
	}
	send(l.h, wmSetRedraw, 1, 0)
	pInvalidateRect.Call(l.h, 0, 1)
}

// refreshPathCells recomputes the shortened paths after a column width change.
func (a *ui) refreshPathCells(l *listCtl) {
	a.withListDC(l.h, func(dc uintptr) {
		for row, id := range l.rows {
			t := a.appTexts(l, a.rowItem(id), dc)
			for _, c := range []int{colFile, colFolder} {
				sub := lvItem{SubItem: int32(c), Text: u16(t[c])}
				send(l.h, lvmSetItemTextW, uintptr(row), uintptr(unsafe.Pointer(&sub)))
			}
		}
	})
}

func (a *ui) updateSortArrows(l *listCtl) {
	hdr := send(l.h, lvmGetHeader, 0, 0)
	for c := 0; c < numAppCols; c++ {
		hi := hdItem{Mask: 0x4 /* HDI_FORMAT */}
		send(hdr, hdmGetItemW, uintptr(c), uintptr(unsafe.Pointer(&hi)))
		hi.Fmt &^= 0x400 | 0x200
		if len(l.sortKeys) > 0 && l.sortKeys[0].col == c {
			if l.sortKeys[0].desc {
				hi.Fmt |= 0x200 // HDF_SORTDOWN
			} else {
				hi.Fmt |= 0x400 // HDF_SORTUP
			}
		}
		send(hdr, hdmSetItemW, uintptr(c), uintptr(unsafe.Pointer(&hi)))
	}
}

// linkAt returns the path shown under a point of a program list, when the point is on its text.
func (a *ui) linkAt(l *listCtl, pt point) (string, bool) {
	ht := lvHitTestInfo{Pt: pt}
	if int32(send(l.h, lvmSubItemHitTest, 0, uintptr(unsafe.Pointer(&ht)))) < 0 || ht.Item < 0 {
		return "", false
	}
	if ht.SubItem != colFile && ht.SubItem != colFolder || int(ht.Item) >= len(l.rows) {
		return "", false
	}
	it := a.rowItem(l.rows[ht.Item])
	target := it.File
	if ht.SubItem == colFolder {
		target = it.Folder
	}
	if target == "" {
		return "", false
	}
	rc := rect{Top: ht.SubItem, Left: 0 /* LVIR_BOUNDS */}
	send(l.h, lvmGetSubItemRect, uintptr(ht.Item), uintptr(unsafe.Pointer(&rc)))
	var text string
	a.withListDC(l.h, func(dc uintptr) { text = a.appTexts(l, it, dc)[ht.SubItem] })
	if text == "" || pt.X > rc.Left+a.scale(10)+a.textWidth(a.fontLink, text) {
		return "", false
	}
	return target, true
}

func cursorIn(h uintptr) point {
	var pt point
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	pScreenToClient.Call(h, uintptr(unsafe.Pointer(&pt)))
	return pt
}

func keyDown(vk uintptr) bool {
	r, _, _ := pGetKeyState.Call(vk)
	return r&0x8000 != 0
}

func listSubclass(hwnd uintptr, msg uint32, wParam, lParam, id, ref uintptr) uintptr {
	a := g
	switch msg {
	case wmSize: // also when the scroll bar appears or disappears
		pPostMessageW.Call(a.hwnd, wmApp+3, hwnd, 0)
	case wmMouseMove:
		a.updateCellTip(hwnd, point{xParam(lParam), yParam(lParam)})
	case wmMouseLeave, wmMouseWheel, wmVScroll, wmHScroll, wmLButtonDown:
		resetCellTip(hwnd)
	}
	if l := a.listOf(hwnd); l != nil {
		switch msg {
		case wmSetCursor:
			if loword(lParam) == 1 /* HTCLIENT */ {
				if _, ok := a.linkAt(l, cursorIn(hwnd)); ok {
					pSetCursor.Call(a.curHand)
					return 1
				}
			}
		case wmLButtonDown:
			// A click on a path opens Explorer. The list view handles the whole click
			// (button down, drag detection, button up) inside its WM_LBUTTONDOWN, so the
			// link is opened once that returns, if the mouse is still on the same path.
			// Ctrl and Shift clicks only select.
			if wParam&(0x4|0x8) == 0 { // MK_SHIFT|MK_CONTROL
				if target, ok := a.linkAt(l, point{xParam(lParam), yParam(lParam)}); ok {
					r, _, _ := pDefSubclassProc.Call(hwnd, uintptr(msg), wParam, lParam)
					if t2, ok := a.linkAt(l, cursorIn(hwnd)); ok && t2 == target {
						openPath(a.hwnd, target)
					}
					return r
				}
			}
		case wmNotify:
			switch (*nmhdr)(ptr(lParam)).Code {
			case hdnEndTrack, hdnDividerDbl:
				// NMHEADER: iItem is at the same offset as in NMLISTVIEW
				if c := int((*nmListView)(ptr(lParam)).Item); c >= 0 && c < numAppCols {
					l.userSized[c] = true
				}
				pPostMessageW.Call(a.hwnd, wmApp+2, uintptr(l.group), 0)
			}
		}
	}
	r, _, _ := pDefSubclassProc.Call(hwnd, uintptr(msg), wParam, lParam)
	return r
}

// itemFor is the element shown for an item ID: merged with its copies when it stands
// for a repeated finding.
func (a *ui) itemFor(id int) (Item, bool) {
	if it, ok := a.shown[id]; ok {
		return it, true
	}
	it, ok := a.items[id]
	return it, ok
}

func (a *ui) rowItem(id int) Item {
	it, _ := a.itemFor(id)
	return it
}

func (a *ui) itemsOf(h uintptr) []Item {
	var out []Item
	for _, id := range a.selectedIDs(h) {
		if it, ok := a.itemFor(id); ok {
			out = append(out, it)
		}
	}
	return out
}

// copyRows copies the selected rows of a list as tab-separated text with headers.
func (a *ui) copyRows(h uintptr) {
	items := a.itemsOf(h)
	if len(items) == 0 {
		return
	}
	var b strings.Builder
	if h == a.hListWin {
		b.WriteString(T("col.element") + "\t" + T("col.status") + "\t" + T("col.details") + "\n")
		for _, it := range items {
			b.WriteString(it.Name + "\t" + statusText(it.Status) + "\t" + oneLine(it.Summary) + "\n")
		}
	} else {
		var hdr []string
		for _, k := range appColKeys {
			hdr = append(hdr, T(k))
		}
		b.WriteString(strings.Join(hdr, "\t") + "\n")
		for _, it := range items {
			var row []string
			for c := 0; c < numAppCols; c++ {
				row = append(row, oneLine(appCell(it, c)))
			}
			b.WriteString(strings.Join(row, "\t") + "\n")
		}
	}
	setClipboard(a.hwnd, b.String())
}

// copiesOf: the other copies of merged elements.
func copiesOf(items []Item) []Item {
	var out []Item
	for _, it := range items {
		out = append(out, it.Copies...)
	}
	return out
}

func (a *ui) selectAll(h uintptr) {
	li := lvItem{StateMask: 2, State: 2}
	send(h, lvmSetItemState, ^uintptr(0), uintptr(unsafe.Pointer(&li)))
}

// ---------------------------------------------------------------------------
// Context menu and actions

func (a *ui) confirmMany(n int) bool {
	if n <= 2 {
		return true
	}
	return msgBox(a.hwnd, Tf("confirm.many", n), 0x4|0x20 /* YESNO|QUESTION */) == 6
}

func (a *ui) contextMenu(h uintptr, lParam uintptr) {
	items := a.itemsOf(h)
	if len(items) == 0 {
		return
	}
	var pt point
	if int32(lParam) == -1 { // from the keyboard
		row := send(h, lvmGetNextItem, ^uintptr(0), 2)
		rc := rect{Left: 0}
		send(h, lvmGetItemRect, row, uintptr(unsafe.Pointer(&rc)))
		pt = point{rc.Left + a.scale(20), rc.Bottom}
		pClientToScreen.Call(h, uintptr(unsafe.Pointer(&pt)))
	} else {
		pt = point{xParam(lParam), yParam(lParam)}
	}
	grayedUnless := func(f func(Item) bool) uintptr {
		for _, it := range items {
			if f(it) {
				return 0
			}
		}
		return 1 // MF_GRAYED
	}
	menu, _, _ := pCreatePopupMenu.Call()
	add := func(flags uintptr, id int, key string) { pAppendMenuW.Call(menu, flags, uintptr(id), up(T(key))) }
	sep := func() { pAppendMenuW.Call(menu, 0x800, 0, 0) }
	program := h != a.hListWin
	add(0, menuDetails, "menu.details")
	sep()
	if program {
		add(grayedUnless(func(it Item) bool { return it.File != "" }), menuOpenFile, "menu.openFile")
		add(grayedUnless(func(it Item) bool { return it.Folder != "" }), menuOpenFolder, "menu.openFolder")
		add(grayedUnless(func(it Item) bool { return it.Homepage() != "" }), menuHomepage, "menu.homepage")
		add(0, menuSearch, "menu.search")
		sep()
	}
	recheck := grayedUnless(func(it Item) bool { return it.Recheck })
	if a.running || a.current == nil {
		recheck = 1
	}
	add(recheck, menuRecheck, "menu.recheck")
	sep()
	add(0, menuCopy, "menu.copy")
	if program {
		add(grayedUnless(func(it Item) bool { return it.File != "" || it.Folder != "" }), menuCopyPaths, "menu.copyPaths")
	}
	add(0, menuSelectAll, "menu.selectAll")
	cmdID, _, _ := pTrackPopupMenu.Call(menu, 0x100|0x2 /* RETURNCMD|RIGHTBUTTON */, uintptr(pt.X), uintptr(pt.Y), 0, a.hwnd, 0)
	pDestroyMenu.Call(menu)
	a.runMenu(int(cmdID), h, items)
}

func (a *ui) runMenu(id int, h uintptr, items []Item) {
	filter := func(f func(Item) bool) []Item {
		var out []Item
		for _, it := range items {
			if f(it) {
				out = append(out, it)
			}
		}
		return out
	}
	unique := func(f func(Item) string) []string {
		seen := map[string]bool{}
		var out []string
		for _, it := range items {
			if v := f(it); v != "" && !seen[v] {
				seen[v] = true
				out = append(out, v)
			}
		}
		return out
	}
	switch id {
	case menuDetails:
		if len(items) > 0 && a.confirmMany(len(items)) {
			a.detailQueue = append(a.detailQueue, items...)
			a.nextDetails()
		}
	case menuOpenFile:
		if sel := filter(func(it Item) bool { return it.File != "" }); a.confirmMany(len(sel)) {
			for _, it := range sel {
				openPath(a.hwnd, it.File)
			}
		}
	case menuOpenFolder:
		if dirs := unique(func(it Item) string { return it.Folder }); a.confirmMany(len(dirs)) {
			for _, d := range dirs {
				openPath(a.hwnd, d)
			}
		}
	case menuHomepage:
		if urls := unique(func(it Item) string { return it.Homepage() }); a.confirmMany(len(urls)) {
			for _, u := range urls {
				openURL(a.hwnd, u)
			}
		}
	case menuSearch:
		qs := unique(func(it Item) string { return it.SearchText() })
		if a.confirmMany(len(qs)) {
			for _, q := range qs {
				openURL(a.hwnd, googleURL(q))
			}
		}
	case menuRecheck:
		var ids []int
		for _, it := range items {
			if it.Recheck {
				ids = append(ids, it.memberIDs()...)
			}
		}
		if len(ids) > 0 && a.confirmMany(len(ids)) {
			a.startRecheck(ids)
		}
	case menuCopy:
		a.copyRows(h)
	case menuCopyPaths:
		var b strings.Builder
		seen := map[string]bool{}
		for _, it := range append(items, copiesOf(items)...) {
			for _, p := range []string{it.File, it.Folder} {
				if p != "" && !seen[p] {
					seen[p] = true
					b.WriteString(p + "\n")
				}
			}
		}
		setClipboard(a.hwnd, b.String())
	case menuSelectAll:
		a.selectAll(h)
	}
}

// ---------------------------------------------------------------------------
// Running the checks

func boolArg(b bool) uintptr {
	if b {
		return 1
	}
	return 0
}

// setRunning enables or disables the controls that must not change during a check.
func (a *ui) setRunning(on bool) {
	for _, d := range a.drives {
		pEnableWindow.Call(d.h, boolArg(!on))
	}
	pEnableWindow.Call(a.hLang, boolArg(!on))
	pEnableWindow.Call(a.hOnline, boolArg(!on))
	pEnableWindow.Call(a.hOnlineLbl, boolArg(!on))
	pEnableWindow.Call(a.hCopy, boolArg(!on && a.current != nil))
	pEnableWindow.Call(a.hSave, boolArg(!on && a.current != nil))
}

func (a *ui) beginRun() {
	a.running, a.stopping = true, false
	a.cancel.Store(false)
	a.startTime = time.Now()
	a.activity, a.step, a.spinText = "", "", ""
	a.redrawSpinner()
	pShowWindow.Call(a.hSpin, 5)
	pSetTimer.Call(a.hwnd, timerSpin, 55, 0)
	setText(a.hCheck, T("btn.stop"))
	a.setButtonGlyph(a.hCheck, glyphStop)
	a.setRunning(true)
	setText(a.hSummary, "")
}

func (a *ui) selectedDrives() []string {
	out := []string{} // never nil: nil means "no drive selection" (test runner)
	for _, d := range a.drives {
		if send(d.h, bmGetCheck, 0, 0) == 1 {
			out = append(out, d.root)
		}
	}
	return out
}

func (a *ui) start() {
	if a.running {
		a.stopping = true
		a.cancel.Store(true)
		pEnableWindow.Call(a.hCheck, 0)
		return
	}
	drives := a.selectedDrives()
	if a.hSearch != 0 && len(a.query) > 0 {
		setText(a.hSearch, "") // a new scan shows everything
	}
	a.beginRun()
	send(a.hListWin, lvmDeleteAllItems, 0, 0)
	for _, l := range a.lists() {
		send(l.h, lvmDeleteAllItems, 0, 0)
		l.rows = nil
	}
	a.items, a.shown, a.winIdx = map[int]Item{}, map[int]Item{}, map[int]int{}
	a.logText.Reset()
	a.log.clear()
	a.logPending = false
	c := newChecker(guiReporter{}, func() bool { return a.cancel.Load() })
	c.offline = a.onlineOff
	a.current = c
	go func() {
		defer func() {
			if r := recover(); r != nil {
				guiReporter{}.Log(LogLine, StError, fmt.Sprintf("Internal error: %v", r))
			}
			postEvent(uiEvent{kind: 3})
		}()
		if len(drives) == 0 {
			c.line(StInfo, T("log.noDrive"))
		}
		runAllChecks(c, drives, nil)
	}()
}

func (a *ui) startRecheck(ids []int) {
	if a.running || a.current == nil {
		return
	}
	a.beginRun()
	c := a.current
	c.offline = a.onlineOff // as ticked now
	go func() {
		defer func() {
			if r := recover(); r != nil {
				guiReporter{}.Log(LogLine, StError, fmt.Sprintf("Internal error: %v", r))
			}
			postEvent(uiEvent{kind: 3})
		}()
		c.section(T("menu.recheck"))
		c.recheck(ids)
	}()
}

func (a *ui) finish() {
	elapsed := fmtElapsed(time.Since(a.startTime))
	a.running = false
	pKillTimer.Call(a.hwnd, timerSpin)
	if a.stopping {
		a.spinText = Tf("spin.stopped", elapsed)
	} else {
		a.spinText = Tf("spin.done", elapsed)
	}
	a.activity, a.step = "", ""
	a.redrawSpinner()
	pEnableWindow.Call(a.hCheck, 1)
	a.setRunning(false)
	setText(a.hCheck, T("btn.again"))
	a.setButtonGlyph(a.hCheck, glyphRefresh)
	if a.current != nil {
		setText(a.hSummary, a.current.counts().text("summary.done"))
	}
}

func (a *ui) drainEvents() {
	evMu.Lock()
	evs := evQueue
	evQueue = nil
	evMu.Unlock()
	var runs []logRun
	flush := func() {
		a.appendLog(runs, false)
		runs = nil
		for _, l := range a.lists() {
			if l.dirty {
				a.rebuild(l)
			}
		}
	}
	for _, e := range evs {
		switch e.kind {
		case 0:
			runs = append(runs, a.logRuns(e)...)
			if e.lkind == LogStart || e.lkind == LogSection { // the current step, shown beside the spinner
				a.step = strings.TrimSuffix(strings.TrimSpace(e.text), " …")
			}
		case 1:
			old, existed := a.items[e.item.ID]
			a.items[e.item.ID] = e.item
			if e.item.Group == GroupWindows {
				a.upsertWin(e.item)
				continue
			}
			for _, l := range a.lists() {
				if l.group == e.item.Group || existed && l.group == old.Group {
					l.dirty = true
				}
			}
		case 2: // shown at the next frame of the spinner (every 55 ms), not for each file
			a.activity = e.text
		case 3:
			flush()
			a.finish()
		}
	}
	flush()
	a.tickSpinner()
}

// shortcut runs the main window's keyboard shortcuts, wherever the focus is in it:
// F5 Scan / Scan again, Ctrl+S Save report, Ctrl+Shift+C Copy report, Ctrl+F the
// search box. It reports
// whether the key was one of them. A button that is disabled does nothing.
func (a *ui) shortcut(key uintptr) bool {
	enabled := func(h uintptr) bool { r, _, _ := pIsWindowEnabled.Call(h); return h != 0 && r != 0 }
	ctrl, shift := ctrlDown(), shiftDown()
	switch {
	case key == 0x74 && !ctrl && !shift: // F5
		if !a.running && enabled(a.hCheck) {
			a.start()
		}
	case key == 'S' && ctrl && !shift:
		if enabled(a.hSave) {
			a.saveReport()
		}
	case key == 'C' && ctrl && shift:
		if enabled(a.hCopy) {
			a.copyReport()
		}
	case key == 'F' && ctrl && !shift:
		pSetFocus.Call(a.hSearch)
		send(a.hSearch, 0xB1 /* EM_SETSEL */, 0, ^uintptr(0))
	default:
		return false
	}
	return true
}

func (a *ui) copyReport() {
	if a.current == nil {
		return
	}
	if setClipboard(a.hwnd, buildReport(a.current, a.logText.String())) {
		msgBox(a.hwnd, T("copied"), 0x40)
	}
}

// saveReport asks for a file name and writes the report as UTF-8 text.
func (a *ui) saveReport() {
	if a.current == nil {
		return
	}
	buf := make([]uint16, 1024)
	copy(buf, utf16z("eMudScan-report-"+time.Now().Format("20060102-1504")+".txt"))
	// the filter is a list of NUL-separated strings ending with two NULs
	filter := utf16.Encode([]rune(T("save.filterTxt") + "\x00*.txt\x00" + T("save.filterAll") + "\x00*.*\x00\x00"))
	defExt := utf16z("txt")
	ofn := openFileName{Owner: a.hwnd, Filter: &filter[0], File: &buf[0], MaxFile: uint32(len(buf)), DefExt: &defExt[0],
		Flags: 0x2 | 0x800 | 0x8 | 0x80000 /* OVERWRITEPROMPT|PATHMUSTEXIST|NOCHANGEDIR|EXPLORER */}
	ofn.StructSize = uint32(unsafe.Sizeof(ofn))
	r, _, _ := pGetSaveFileNameW.Call(uintptr(unsafe.Pointer(&ofn)))
	runtime.KeepAlive(filter)
	runtime.KeepAlive(defExt)
	runtime.KeepAlive(buf)
	if r == 0 {
		return
	}
	path := syscall.UTF16ToString(buf)
	data := append([]byte{0xEF, 0xBB, 0xBF}, buildReport(a.current, a.logText.String())...)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		msgBox(a.hwnd, Tf("saveError", err), 0x10)
		return
	}
	msgBox(a.hwnd, Tf("saved", path), 0x40)
}

// changeLanguage switches the interface to the chosen language. Nothing is saved: the
// window is reset as if the program had just been started, after a warning when there
// are results to lose.
func (a *ui) changeLanguage() {
	sel := int(send(a.hLang, cbGetCurSel, 0, 0))
	if sel < 0 || sel > 2 || sel == a.langSel {
		return
	}
	cur := Lang
	switch sel {
	case 0:
		setLangFromCode(userUILanguage())
	case 1:
		Lang = 0
	case 2:
		Lang = 1
	}
	next := Lang
	if a.current != nil {
		// ask in the chosen language, and in the current one when it differs
		question := T("lang.reset")
		if next != cur {
			Lang = cur
			question += "\n\n" + T("lang.reset")
			Lang = next
		}
		if msgBox(a.hwnd, question, 0x4|0x30 /* YESNO|WARNING */) != 6 {
			Lang = cur
			send(a.hLang, cbSetCurSel, uintptr(a.langSel), 0)
			return
		}
	}
	a.langSel = sel
	a.reset()
}

var (
	childList     []uintptr
	collectChilds = syscall.NewCallback(func(h, _ uintptr) uintptr {
		childList = append(childList, h)
		return 1
	})
)

// relayout places the controls again (window resized, frame border dragged). Each
// control that moved or changed size is drawn again entirely, with its parts (list
// header, scroll bars, border); the background and the frames are drawn again off
// screen and copied at once, around the controls. What did not move (the title,
// the buttons…) is not drawn again, so it does not blink.
func (a *ui) relayout() {
	childList = nil
	pEnumChildWindows.Call(a.hwnd, collectChilds, 0)
	kids := childList
	before := make([]rect, len(kids))
	for i, h := range kids {
		pGetWindowRect.Call(h, uintptr(unsafe.Pointer(&before[i])))
	}
	a.layout()
	for i, h := range kids {
		var now rect
		pGetWindowRect.Call(h, uintptr(unsafe.Pointer(&now)))
		if now != before[i] {
			pRedrawWindow.Call(h, 0, 0, rdwInvalidate|rdwFrame|rdwAllChildren)
		}
	}
	pRedrawWindow.Call(a.hwnd, 0, 0, rdwInvalidate|rdwNoChildren)
	pRedrawWindow.Call(a.hwnd, 0, 0, rdwUpdateNow|rdwAllChildren)
}

// reset rebuilds the window in the current language, as when the program starts.
func (a *ui) reset() {
	if a.log != nil {
		a.log.destroyFonts()
		a.log = nil
	}
	if a.onlineTip != 0 {
		pDestroyWindow.Call(a.onlineTip) // a tooltip is not a child window
		a.onlineTip = 0
	}
	for _, t := range a.keyTips {
		pDestroyWindow.Call(t)
	}
	a.keyTips = nil
	childList = nil
	pEnumChildWindows.Call(a.hwnd, collectChilds, 0)
	for _, h := range childList {
		pDestroyWindow.Call(h)
	}
	a.hLogo, a.hTitle, a.hInfo, a.hLangLbl, a.hLang, a.hDrivesLbl, a.hOnline, a.hOnlineLbl = 0, 0, 0, 0, 0, 0, 0, 0
	a.drives = nil
	a.hCheck, a.hCopy, a.hSave, a.hSearch = 0, 0, 0, 0
	a.hLog, a.hSpin, a.hListWin = 0, 0, 0
	a.frames = [4]frameBox{}
	a.net, a.app, a.hSummary = nil, nil, 0
	a.heights = [3]int32{}
	a.items, a.shown, a.winIdx = map[int]Item{}, map[int]Item{}, map[int]int{}
	a.current, a.running, a.stopping = nil, false, false
	a.activity, a.spinText = "", ""
	a.logText.Reset()
	a.logPending = false
	a.detailQueue, a.detailBack = nil, nil
	a.createControls()
	a.layout()
	a.appendLog([]logRun{{T("log.ready"), rgb(110, 110, 110), false, false}}, true)
	a.redrawAll()
	pSetFocus.Call(a.hLang)
}

// ---------------------------------------------------------------------------
// Details window

type detailsDlg struct {
	hwnd, bCopy, bCmds, bFolder, bClose, status uintptr
	view                                        *docView
	item                                        Item
	blocks                                      []docBlock
	cmds                                        []string
}

var detailsProc = syscall.NewCallback(detailsWndProc)

// openItemRef shows the details of the element an item link refers to. It is done
// after the click is handled, since the current details window is closed first.
func (a *ui) openItemRef(key string) {
	name, ok := itemRefs[key]
	if !ok {
		return
	}
	for id, it := range a.items {
		if it.Name == name() {
			pPostMessageW.Call(a.hwnd, wmApp+4, uintptr(id), 0)
			return
		}
	}
}

func (a *ui) nextDetails() {
	if a.dlg != nil || len(a.detailQueue) == 0 {
		return
	}
	it := a.detailQueue[0]
	a.detailQueue = a.detailQueue[1:]
	if cur, ok := a.itemFor(it.ID); ok {
		it = cur
	}
	a.openDetails(it)
}

// workArea is the usable part of the screen the main window is on.
func (a *ui) workArea() rect {
	mon, _, _ := pMonitorFromWindow.Call(a.hwnd, 2 /* MONITOR_DEFAULTTONEAREST */)
	mi := monitorInfo{}
	mi.Size = uint32(unsafe.Sizeof(mi))
	if r, _, _ := pGetMonitorInfoW.Call(mon, uintptr(unsafe.Pointer(&mi))); r != 0 {
		return mi.Work
	}
	var wa rect
	pSystemParametersInfoW.Call(0x0030, 0, uintptr(unsafe.Pointer(&wa)), 0)
	return wa
}

func (a *ui) openDetails(it Item) {
	d := &detailsDlg{item: it, blocks: detailBlocks(it)}
	for _, b := range d.blocks {
		d.cmds = append(d.cmds, b.cmds...)
	}
	a.dlg = d
	wa := a.workArea()
	w := min(a.scale(850), wa.Right-wa.Left-a.scale(40)) // lines short enough to read
	h := min(a.scale(900), wa.Bottom-wa.Top-a.scale(40))
	var mr rect
	pGetWindowRect.Call(a.hwnd, uintptr(unsafe.Pointer(&mr)))
	x := max(wa.Left, min(mr.Left+(mr.Right-mr.Left-w)/2, wa.Right-w))
	y := max(wa.Top, min(mr.Top+(mr.Bottom-mr.Top-h)/2, wa.Bottom-h))
	title := itemTitle(it)
	inst, _, _ := pGetModuleHandleW.Call(0)
	d.hwnd, _, _ = pCreateWindowExW.Call(wsExControlParent, up("eMudScanDetails"), up(title+" — "+appName),
		uintptr(wsOverlappedWindow&^wsMinimizeBox), uintptr(x), uintptr(y), uintptr(w), uintptr(h), a.hwnd, 0, inst, 0)
	if d.hwnd == 0 {
		a.dlg = nil
		return
	}
	mk := func(class, text string, style uint32, id int) uintptr {
		hh, _, _ := pCreateWindowExW.Call(0, up(class), up(text), uintptr(wsChild|wsVisible|style), 0, 0, 10, 10, d.hwnd, uintptr(id), 0, 0)
		send(hh, wmSetFont, a.fontUI, 1)
		return hh
	}
	d.view = a.newDocView(d.hwnd, d.blocks)
	d.view.onStatus = func(s string) { setText(d.status, s) }
	d.view.onCopyAll = func() { d.copyAll() }
	d.bCopy = a.iconButton(d.hwnd, T("dlg.copyAll"), idDlgCopy, false, glyphCopy)
	d.bCmds = a.iconButton(d.hwnd, T("dlg.copyCmds"), idDlgCmds, false, glyphTerminal)
	d.bFolder = a.iconButton(d.hwnd, T("dlg.openFolder"), idDlgFolder, false, glyphFolder)
	d.bClose = a.iconButton(d.hwnd, T("dlg.close"), idDlgClose, true, glyphClose)
	d.status = mk("STATIC", "", 0x80|0x4000|0x200 /* NOPREFIX|ENDELLIPSIS|CENTERIMAGE */, 0)
	if len(d.cmds) == 0 {
		pEnableWindow.Call(d.bCmds, 0)
	}
	if it.Folder == "" && it.File == "" {
		pEnableWindow.Call(d.bFolder, 0)
	}
	d.layout(a)
	pEnableWindow.Call(a.hwnd, 0)
	pShowWindow.Call(d.hwnd, 1)
	pSetForegroundWindow.Call(d.hwnd) // also when it replaces other details (keys go to it)
	pSetFocus.Call(d.view.hwnd)
}

func (d *detailsDlg) copyAll() {
	if setClipboard(d.hwnd, blocksText(d.blocks)) {
		setText(d.status, T("copied"))
	}
}

func (d *detailsDlg) layout(a *ui) {
	var rc rect
	pGetClientRect.Call(d.hwnd, uintptr(unsafe.Pointer(&rc)))
	s := a.scale
	bh := s(30)
	bar := bh + s(24)
	a.move(d.view.hwnd, 0, 0, rc.Right, rc.Bottom-bar)
	y := rc.Bottom - bh - s(12)
	x := s(14)
	for _, b := range []uintptr{d.bCopy, d.bCmds, d.bFolder} {
		var buf [128]uint16
		n, _, _ := pGetWindowTextW.Call(b, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
		bw := max(s(130), a.textWidth(a.fontUI, syscall.UTF16ToString(buf[:n]))+s(16)+s(44))
		a.move(b, x, y, bw, bh)
		x += bw + s(8)
	}
	a.move(d.bClose, rc.Right-s(14)-s(120), y, s(120), bh)
	a.move(d.status, x+s(8), y, rc.Right-s(14)-s(120)-x-s(16), bh)
	pInvalidateRect.Call(d.hwnd, 0, 1)
}

func (a *ui) closeDetails() {
	d := a.dlg
	if d == nil {
		return
	}
	pEnableWindow.Call(a.hwnd, 1)
	d.view.destroy()
	pDestroyWindow.Call(d.hwnd)
	a.dlg = nil
	if n := len(a.detailBack); n > 0 && !a.navigating {
		// back to the details this one was opened from, where they were scrolled to
		b := a.detailBack[n-1]
		a.detailBack = a.detailBack[:n-1]
		it := b.item
		if cur, ok := a.itemFor(it.ID); ok {
			it = cur
		}
		a.openDetails(it)
		if a.dlg != nil {
			a.dlg.view.setScroll(b.scroll)
		}
		return
	}
	a.nextDetails()
}

// detailsBack is a details window to show again.
type detailsBack struct {
	item   Item
	scroll int32
}

func detailsWndProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	a := g
	d := a.dlg
	if d != nil && d.hwnd == hwnd {
		switch msg {
		case wmSize:
			d.layout(a)
			return 0
		case wmPaint:
			// a thin line between the text and the buttons
			var ps paintStruct
			dc, _, _ := pBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
			var rc rect
			pGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
			top := rc.Bottom - a.scale(30) - a.scale(24)
			line := rect{0, top, rc.Right, top + max(1, a.scale(1))}
			br, _, _ := pGetSysColorBrush.Call(16 /* COLOR_BTNSHADOW */)
			pFillRect.Call(dc, uintptr(unsafe.Pointer(&line)), br)
			pEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
			return 0
		case wmGetMinMaxInfo:
			pt := (*[2]int32)(unsafe.Add(ptr(lParam), 24))
			pt[0], pt[1] = a.scale(700), a.scale(420)
			return 0
		case wmMouseWheel:
			if d.view != nil {
				send(d.view.hwnd, msg, wParam, lParam)
				return 0
			}
		case wmCtlColorStatic:
			pSetBkColor.Call(wParam, uintptr(sysColor(5)))
			pSetTextColor.Call(wParam, uintptr(rgb(30, 123, 52)))
			return a.brushWindow
		case wmCommand:
			switch loword(wParam) {
			case idDlgCopy:
				d.copyAll()
			case idDlgCmds:
				if setClipboard(hwnd, strings.Join(d.cmds, "\n")) {
					setText(d.status, T("dlg.copiedBlock"))
				}
			case idDlgFolder:
				if d.item.Folder != "" {
					openPath(hwnd, d.item.Folder)
				} else {
					openPath(hwnd, d.item.File)
				}
			case idDlgClose, 1, 2: // Close, Enter, Escape
				a.closeDetails()
			}
			return 0
		case wmClose:
			a.closeDetails()
			return 0
		}
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
	return r
}

// ---------------------------------------------------------------------------
// Main window procedure

func wndProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	a := g
	switch msg {
	case wmSize:
		a.relayout()
		return 0
	case wmEraseBkgnd:
		return 1 // all drawn by WM_PAINT, off screen
	case wmPaint:
		a.paintMain(hwnd)
		return 0
	case wmPrintClient: // a themed control drawing what is behind it
		var rc rect
		pGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
		a.paintBackground(wParam, rc)
		return 0
	case wmGetMinMaxInfo:
		if a.dpi > 0 {
			pt := (*[2]int32)(unsafe.Add(ptr(lParam), 24))
			pt[0], pt[1] = a.scale(960), a.scale(720)
		}
		return 0
	case wmSetCursor:
		if wParam == hwnd && loword(lParam) == 1 {
			pt := cursorIn(hwnd)
			if a.splitAt(pt.X, pt.Y) >= 0 {
				pSetCursor.Call(a.curSizeNS)
				return 1
			}
		}
	case wmLButtonDown:
		if i := a.splitAt(xParam(lParam), yParam(lParam)); i >= 0 {
			for j := range a.heights {
				if a.heights[j] == 0 {
					a.heights[j] = a.frameHeight(j)
				}
			}
			a.dragging, a.dragY = i, yParam(lParam)
			a.dragH = a.heights[i]
			pSetCapture.Call(hwnd)
			return 0
		}
	case wmMouseMove:
		if a.dragging >= 0 {
			if h := max(a.dragH+yParam(lParam)-a.dragY, a.scale(80)); h != a.heights[a.dragging] {
				a.heights[a.dragging] = h
				a.relayout()
			}
			return 0
		}
	case wmLButtonUp:
		if a.dragging >= 0 {
			a.dragging = -1
			pReleaseCapture.Call()
			return 0
		}
	case wmCaptureChanged:
		a.dragging = -1
	case wmTimer:
		switch wParam {
		case timerSpin:
			a.tickSpinner()
		case timerLang:
			if send(a.hLang, cbGetDroppedState, 0, 0) == 0 {
				pKillTimer.Call(hwnd, timerLang)
				a.changeLanguage()
			}
		}
		return 0
	case wmCommand:
		switch id := int(loword(wParam)); {
		case id == 1: // IDOK: Enter key routed by IsDialogMessage
			f, _, _ := pGetFocus.Call()
			if f == a.hListWin || a.listOf(f) != nil {
				a.runMenu(menuDetails, f, a.itemsOf(f))
			}
		case id == idCheck:
			a.start()
		case id == idCopy:
			a.copyReport()
		case id == idSave:
			a.saveReport()
		case id == idLang:
			// CBN_SELCHANGE or CBN_CLOSEUP: wait a little, so that moving through
			// the list with the arrow keys does not ask at every step
			if n := hiword(wParam); n == 1 || n == 8 {
				pSetTimer.Call(hwnd, timerLang, 700, 0)
			}
		case id == idOnline:
			a.onlineOff = send(a.hOnline, bmGetCheck, 0, 0) != 1
		case id == idSearch && hiword(wParam) == 0x300: // EN_CHANGE
			a.query = queryWords(getText(a.hSearch))
			for _, l := range a.lists() {
				a.rebuild(l)
			}
		case id == 2: // IDCANCEL: Escape, routed by IsDialogMessage
			if f, _, _ := pGetFocus.Call(); f == a.hSearch && len(a.query) > 0 {
				setText(a.hSearch, "") // clears the search
			}
		case id == idHideNet || id == idHideApp:
			l := a.net
			if id == idHideApp {
				l = a.app
			}
			l.hideOK = send(l.hide, bmGetCheck, 0, 0) == 1
			a.rebuild(l)
		}
		return 0
	case wmNotify:
		h := (*nmhdr)(ptr(lParam))
		l := a.listOf(h.HwndFrom)
		if h.HwndFrom != a.hListWin && l == nil {
			break
		}
		switch h.Code {
		case nmDblClk:
			if l != nil {
				if _, onLink := a.linkAt(l, cursorIn(l.h)); onLink {
					return 0 // the click already opened the path
				}
			}
			a.runMenu(menuDetails, h.HwndFrom, a.itemsOf(h.HwndFrom))
			return 0
		case lvnColumnClick:
			if l != nil {
				nl := (*nmListView)(ptr(lParam))
				l.sortKeys = clickSort(l.sortKeys, int(nl.SubItem))
				a.updateSortArrows(l)
				a.rebuild(l)
			}
			return 0
		case lvnKeyDown:
			kd := (*nmLVKeyDown)(ptr(lParam))
			if ctrlDown() {
				switch kd.VKey {
				case 'C':
					a.copyRows(h.HwndFrom)
				case 'A':
					a.selectAll(h.HwndFrom)
				}
			}
			return 0
		case nmCustomDraw:
			if l == nil {
				break
			}
			cd := (*nmLVCustomDraw)(ptr(lParam))
			switch cd.DrawStage {
			case 0x1: // CDDS_PREPAINT
				return 0x20 // CDRF_NOTIFYITEMDRAW
			case 0x10001: // CDDS_ITEMPREPAINT
				return 0x20 // CDRF_NOTIFYSUBITEMDRAW
			case 0x30001: // CDDS_SUBITEM | CDDS_ITEMPREPAINT
				row := int(cd.ItemSpec)
				isLink := false
				if row < len(l.rows) && (cd.SubItem == colFile || cd.SubItem == colFolder) {
					it := a.rowItem(l.rows[row])
					isLink = appCell(it, int(cd.SubItem)) != ""
				}
				selected := send(l.h, lvmGetItemState, uintptr(row), 2) != 0
				stale := row < len(l.rows) && cd.SubItem == colUpdated && a.rowItem(l.rows[row]).stale() > 0
				if isLink {
					if !selected {
						cd.ClrText = rgb(0, 102, 204)
					}
					pSelectObject.Call(cd.Hdc, a.fontLink)
				} else if stale && !selected { // the installed version is a year old or more
					cd.ClrText = optinColor
					pSelectObject.Call(cd.Hdc, a.fontUI)
				} else {
					cd.ClrText = sysColor(8) // COLOR_WINDOWTEXT
					pSelectObject.Call(cd.Hdc, a.fontUI)
				}
				return 0x2 // CDRF_NEWFONT
			}
			return 0
		}
	case wmContextMenu:
		if wParam == a.hListWin || a.listOf(wParam) != nil {
			a.contextMenu(wParam, lParam)
			return 0
		}
	case wmCtlColorStatic:
		hdc := wParam
		pSetBkColor.Call(hdc, uintptr(sysColor(5)))
		if lParam == a.hSummary {
			pSetTextColor.Call(hdc, uintptr(sysColor(17)))
		}
		return a.brushWindow
	case wmAppEvents:
		a.drainEvents()
		return 0
	case wmApp + 4: // show the details of another element (item link)
		if it, ok := a.items[int(wParam)]; ok {
			if d := a.dlg; d != nil {
				// the current details come back when the new ones are closed
				a.detailBack = append(a.detailBack, detailsBack{d.item, d.view.scroll})
				a.navigating = true
				a.closeDetails()
				a.navigating = false
			}
			a.openDetails(it)
		}
		return 0
	case wmApp + 3: // the client area of a list changed
		if wParam == a.hListWin {
			a.fitWinColumns()
		} else if l := a.listOf(wParam); l != nil {
			a.fitColumns(l, false)
		}
		return 0
	case wmApp + 2: // a column of a program list was resized by the user
		for _, l := range a.lists() {
			if uintptr(l.group) == wParam {
				a.fitColumns(l, true)
			}
		}
		return 0
	case wmDpiChanged:
		a.dpi = int(loword(wParam))
		r := (*rect)(ptr(lParam))
		a.createResources()
		a.applyFonts()
		a.measureRow()
		for _, l := range a.lists() {
			l.lastW = 0
		}
		pSetWindowPos.Call(hwnd, 0, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right-r.Left), uintptr(r.Bottom-r.Top), 0x0004|0x0010)
		a.layout()
		return 0
	case wmClose:
		a.cancel.Store(true)
		pDestroyWindow.Call(hwnd)
		return 0
	case wmDestroy:
		pPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
	return r
}

func registerClass(inst uintptr, name string, proc uintptr, icon, iconSm, cursor uintptr) {
	// CS_HREDRAW|CS_VREDRAW, except for the main window: when it is resized, relayout
	// draws again what has to be, and only that
	style := uint32(0x3)
	switch name {
	case "eMudScanWindow":
		style = 0
	case "eMudScanLog":
		style = 0x8 // CS_DBLCLKS (the log draws again what changed)
	}
	wc := wndClassEx{Style: style, WndProc: proc, Instance: inst, Icon: icon, Cursor: cursor, Background: 5 + 1, ClassName: u16(name), IconSm: iconSm}
	wc.Size = uint32(unsafe.Sizeof(wc))
	pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
}

func main() {
	runtime.LockOSThread()
	// language: Windows display language, unless --en or --fr is given (nothing is saved)
	setLangFromCode(userUILanguage())
	for _, arg := range os.Args[1:] {
		switch strings.ToLower(strings.TrimLeft(arg, "-/")) {
		case "fr", "lang=fr":
			Lang, g.langSel = 1, 2
		case "en", "lang=en":
			Lang, g.langSel = 0, 1
		case "offline": // start with the online test turned off
			g.onlineOff = true
		}
	}
	pSetProcessDpiAwarenessCtx.Call(^uintptr(3)) // PER_MONITOR_AWARE_V2; also set by the manifest
	pCoInitializeEx.Call(0, 2)                   // COINIT_APARTMENTTHREADED, for the shell functions
	icc := struct{ Size, ICC uint32 }{8, 0x1 | 0x20 | 0x4000 | 0x8000 /* LINK_CLASS: the About box */}
	pInitCommonControlsEx.Call(uintptr(unsafe.Pointer(&icc)))

	if err := loadData(); err != nil {
		msgBox(0, Tf("err.data", err), 0x10)
		return
	}
	defaultEmbedScanner = buildEmbedScanner()

	a := g
	inst, _, _ := pGetModuleHandleW.Call(0)
	a.dpi = 96
	if d, _, _ := pGetDpiForSystem.Call(); d != 0 {
		a.dpi = int(d)
	}
	a.brushWindow, _, _ = pGetSysColorBrush.Call(5)
	a.curArrow, _, _ = pLoadCursorW.Call(0, 32512)
	a.curHand, _, _ = pLoadCursorW.Call(0, 32649)
	a.curSizeNS, _, _ = pLoadCursorW.Call(0, 32645)
	big, _, _ := pLoadImageW.Call(inst, 1, 1, uintptr(a.scale(32)), uintptr(a.scale(32)), 0)
	small, _, _ := pLoadImageW.Call(inst, 1, 1, uintptr(a.scale(16)), uintptr(a.scale(16)), 0)
	if big == 0 {
		big = iconFromRGBA(renderIcon(iconApp, int(a.scale(32))), int(a.scale(32)))
		small = iconFromRGBA(renderIcon(iconApp, int(a.scale(16))), int(a.scale(16)))
	}
	registerClass(inst, "eMudScanWindow", syscall.NewCallback(wndProc), big, small, a.curArrow)
	registerClass(inst, "eMudScanDetails", detailsProc, big, small, a.curArrow)
	registerClass(inst, "eMudScanSpinner", spinProc, 0, 0, a.curArrow)
	registerClass(inst, "eMudScanIcon", infoProc, 0, 0, a.curHand)
	registerClass(inst, "eMudScanLogo", logoProc, 0, 0, a.curArrow)
	registerClass(inst, "eMudScanLog", logProc, 0, 0, a.curArrow)
	registerClass(inst, "eMudScanOptIn", optinProc, 0, 0, a.curArrow)

	w, h := a.scale(1280), a.scale(980)
	var work rect
	if r, _, _ := pSystemParametersInfoW.Call(0x0030 /* SPI_GETWORKAREA */, 0, uintptr(unsafe.Pointer(&work)), 0); r != 0 {
		w = min(w, work.Right-work.Left-a.scale(20))
		h = min(h, work.Bottom-work.Top-a.scale(20))
	}
	// WS_CLIPCHILDREN: the window never paints its background over its controls
	hwnd, _, _ := pCreateWindowExW.Call(wsExControlParent, up("eMudScanWindow"), up(appName),
		wsOverlappedWindow|0x02000000, 0x80000000, 0x80000000, uintptr(w), uintptr(h), 0, 0, inst, 0)
	if hwnd == 0 {
		return
	}
	a.hwnd = hwnd
	if d, _, _ := pGetDpiForWindow.Call(hwnd); d != 0 {
		a.dpi = int(d)
	}
	a.createResources()
	a.createControls()
	a.layout()
	a.appendLog([]logRun{{T("log.ready"), rgb(110, 110, 110), false, false}}, true)
	pShowWindow.Call(hwnd, 1)
	pUpdateWindow.Call(hwnd)
	pSetFocus.Call(a.hCheck)

	var m msgT
	for {
		r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		if m.Message == wmKeyDown && a.dlg == nil && a.shortcut(m.WParam) {
			continue
		}
		target := hwnd
		if a.dlg != nil {
			if child, _, _ := pIsChild.Call(a.dlg.hwnd, m.Hwnd); child != 0 || m.Hwnd == a.dlg.hwnd {
				target = a.dlg.hwnd
			}
		}
		if ok, _, _ := pIsDialogMessageW.Call(target, uintptr(unsafe.Pointer(&m))); ok != 0 {
			continue
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}
