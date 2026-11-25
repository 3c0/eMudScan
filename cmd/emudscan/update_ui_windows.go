//go:build windows

package main

// The update in the window (update.go, update_windows.go). eMudScan looks for a new
// version when a scan starts ("Scan" or "Scan again"), once in a session, with
// "Online checks and updates" checked, and each time "Check for updates" of the
// About box is clicked, which closes the box and cannot be clicked while a scan runs.
// It looks before the scan starts. So an update never meets a running scan: it
// interrupts nothing. A newer version is offered in the update dialog, with the
// release notes of the new versions. Once accepted, the dialog shows the download and
// its checks, then eMudScan closes for the new version.

import (
	"strings"
	"sync/atomic"
	"syscall"
	"unsafe"
)

const (
	evUpdateLooked   = 10 // GitHub's answer: e.upd
	evUpdateNotes    = 11 // the release notes of the new versions: e.upd
	evUpdateProgress = 12 // how much of the download has come: e.upd
	evUpdateGot      = 13 // the download, checked, or its error: e.upd
)

// updateNews is what a lookup, a download, or the list of the releases gave.
type updateNews struct {
	rel   updateRelease
	found bool
	err   error
	asked bool           // the user asked: "Check for updates"; else a scan
	notes []releaseNotes // the release notes of the new versions, up to rel
	more  bool           // there are more new versions than notes
	read  bool           // the list of the releases, which gave the notes, could be read
	run   int            // the download it is about (updateBox.run)
	got   int            // how much of the download has come
	data  []byte         // the download, checked
}

// lookForUpdate looks for a newer eMudScan, in the background: when the user asks,
// each time, or for a scan, once in a session; never during a scan, nor when "Online
// checks and updates" is cleared. It reports whether it looks: a scan then starts
// once it has (updateEvent). Meanwhile, the window is disabled, and at once, so that
// the click shows: the Scan button is greyed out, the cursor is busy, the log and the
// summary say that eMudScan looks, the log in a section of its own, "eMudScan
// Update". For a scan, the log starts anew with its first line (scanHeader), then
// this section, which the scan keeps. When the user asks, the section comes after
// the log.
func (a *ui) lookForUpdate(asked bool) bool {
	if a.updateBusy || a.onlineOff || a.running || !asked && a.updateLooked {
		return false
	}
	a.updateLooked, a.updateBusy = true, true
	a.setButton(a.hCheck, getText(a.hCheck), btnGlyphs[a.hCheck], false)
	var runs []logRun
	switch {
	case !asked:
		a.clearLog()
		a.updateLogged = true
		runs = a.logRuns(uiEvent{lkind: LogLine, st: StPending, text: scanHeader()})
	case a.logText.Len() > 0 && !strings.HasSuffix(a.logText.String(), "\n"):
		runs = []logRun{{"\r\n", rgb(32, 32, 32), false, false}} // after "Ready…"
	}
	runs = append(runs, a.logRuns(uiEvent{lkind: LogSection, text: T("log.updateTitle")})...)
	a.appendLog(append(runs, a.logRuns(uiEvent{lkind: LogStart, st: StPending, text: T("log.update") + " …"})...), true)
	a.summaryBefore = getText(a.hSummary)
	setText(a.hSummary, T("update.looking"))
	pEnableWindow.Call(a.hwnd, 0)
	a.cursorAgain()
	go func() {
		n := &updateNews{asked: asked}
		n.rel, n.found, n.err = updateLookup()
		postEvent(uiEvent{kind: evUpdateLooked, upd: n})
	}()
	return true
}

// updateEvent handles GitHub's answer, and the other news, which go to the update
// dialog.
func (a *ui) updateEvent(e uiEvent) {
	n := e.upd
	if e.kind != evUpdateLooked {
		if updateDlg != nil {
			updateDlg.news(e.kind, n)
		}
		return
	}
	a.updateBusy = false
	st, text := StOK, strings.TrimSuffix(Tf("update.latest", appVersion), ".")
	switch {
	case n.found:
		st, text = StInfo, Tf("update.title", n.rel.version)
	case n.err != nil:
		st, text = StInfo, reason(n.err)
	}
	a.appendLog(a.logRuns(uiEvent{lkind: LogEnd, st: st, text: text}), true)
	a.setButton(a.hCheck, getText(a.hCheck), btnGlyphs[a.hCheck], true)
	setText(a.hSummary, a.summaryBefore)
	pEnableWindow.Call(a.hwnd, 1)
	a.cursorAgain() // before the dialog or the message, or the scan, which sets it again
	switch {
	case n.found:
		if a.offerUpdate(n.rel) {
			pDestroyWindow.Call(a.hwnd) // eMudScan closes for the new version
			return
		}
	case n.asked && n.err != nil:
		msgBox(a.hwnd, Tf("update.lookFailed", reason(n.err)), 0x30 /* MB_ICONWARNING */)
	case n.asked:
		msgBox(a.hwnd, Tf("update.latest", appVersion), 0x40 /* MB_ICONINFORMATION */)
	}
	if !n.asked {
		a.start() // the scan, which waited for the lookup
	}
}

// updateDoneArgs reads the arguments of an eMudScan started after an update
// (--update-done, given by updateInstall): it deletes the files of the update, in the
// background, and gives the version it was updated from (none when the update
// failed), and the arguments that are left.
func updateDoneArgs(args []string) (from string, rest []string) {
	if len(args) != 5 || args[0] != "--update-done" {
		return "", args
	}
	auto := Lang
	setLangFromCode(args[1])
	if Lang != auto {
		g.langSel = Lang + 1
	}
	go updateDone(args[2], args[3])
	if !updateTagRe.MatchString(args[4]) {
		return "", nil
	}
	return args[4], nil
}

// ---------------------------------------------------------------------------
// The update dialog: a header band with the shield, the new version and the one the
// user has; the release notes of the new versions, in a frame where they scroll
// (docView); under it, what "Update now" does, or, once clicked, the progress of
// the download and what eMudScan does; then the button bar, with the release page.
// It opens at once, with the release notes of the latest version, which came with
// it; those of the other new versions come in the background.

type updateBox struct {
	rel     updateRelease
	notes   []releaseNotes
	more    bool
	loading bool // the release notes of the other new versions are on their way
	unread  bool // they could not be read

	hwnd                      uintptr
	doc                       *docView
	icon, titleFont, textFont uintptr
	how, bar, status          uintptr // what "Update now" does; the progress and what eMudScan does
	link, yes, no             uintptr // the release page, "Update now", "Not now"
	headH, barH, lowH         int32   // the header band, the button bar, the area of how
	frame                     rect    // around the release notes

	working bool         // downloading
	erred   bool         // the download or a check failed: the status says why
	started bool         // the update has started: eMudScan closes
	run     int          // the number of the download, to drop the news of a stopped one
	stop    *atomic.Bool // stops the download
}

var (
	updateDlg     *updateBox
	updateDlgProc = syscall.NewCallback(updateProc)
)

const (
	pbmSetRange32 = 0x0406 // PBM_SETRANGE32
	pbmSetPos     = 0x0402 // PBM_SETPOS
)

// offerUpdate offers the update to r, and returns when the dialog is closed: true
// when the update has started, and eMudScan must close.
func (a *ui) offerUpdate(r updateRelease) bool {
	b := &updateBox{rel: r, notes: []releaseNotes{r.releaseNotes}, loading: !nextVersion(appVersion, r.version)}
	updateDlg = b
	if b.loading {
		go func() {
			n := &updateNews{rel: r}
			n.notes, n.more, n.read = updateNotes(r)
			postEvent(uiEvent{kind: evUpdateNotes, upd: n})
		}()
	}
	modalBox(a.hwnd, T("update.caption"), true, updateDlgProc)
	updateDlg = nil
	if b.stop != nil {
		b.stop.Store(true) // the download stops, and its news are dropped
	}
	for _, f := range []uintptr{b.titleFont, b.textFont} {
		if f != 0 {
			pDeleteObject.Call(f) // fails harmlessly if a link control already did it
		}
	}
	return b.started
}

// blocks are the release notes, then a line that says that the others are on their
// way, or that they could not be read, or that there are more on GitHub.
func (b *updateBox) blocks() []docBlock {
	out := notesBlocks(b.notes, b.more)
	note := func(text string) {
		out = append(out, docBlock{kind: dkSpace}, docBlock{kind: dkPara, runs: parseInline(text)})
	}
	switch {
	case b.loading:
		note(T("update.loadingNotes"))
	case b.unread:
		note(Tf("update.notesFailed", appURL+"/releases"))
	}
	return out
}

func updateProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	a, b := g, updateDlg
	if b == nil {
		return 0
	}
	switch msg {
	case wmInitDialog:
		b.build(hwnd)
		return 0 // the focus is set on "Update now"
	case wmSize:
		if b.doc != nil { // made by WM_INITDIALOG, which may come after a first WM_SIZE
			b.layout()
			// all drawn again at once, the controls included: none left where it was
			pRedrawWindow.Call(hwnd, 0, 0, rdwInvalidate|rdwAllChildren|rdwUpdateNow)
		}
		return 1
	case wmGetMinMaxInfo:
		pt := (*[2]int32)(unsafe.Add(ptr(lParam), 24)) // ptMinTrackSize
		pt[0], pt[1] = a.scale(460), a.scale(400)
		return 1
	case wmCommand:
		switch loword(wParam) {
		case 1: // "Update now", "Try again"
			if !b.working {
				b.download()
			}
			return 1
		case 2: // "Not now", "Cancel", Esc, ×
			pEndDialog.Call(hwnd, 2)
			return 1
		}
	case wmNotify:
		if h := (*nmhdr)(ptr(lParam)); h.HwndFrom == b.link && (h.Code == -2 || h.Code == -4) { // NM_CLICK, NM_RETURN
			openTarget(hwnd, b.rel.page())
			return 1
		}
	case wmCtlColorStatic:
		if lParam == b.link { // on the button bar
			pSetBkColor.Call(wParam, uintptr(sysColor(15)))
			face, _, _ := pGetSysColorBrush.Call(15 /* COLOR_BTNFACE */)
			return face
		}
		color := rgb(32, 32, 32)
		switch {
		case lParam == b.how:
			color = rgb(96, 96, 96)
		case lParam == b.status && b.erred:
			color = rgb(196, 43, 28) // a red that reads on white
		}
		pSetTextColor.Call(wParam, uintptr(color))
		pSetBkColor.Call(wParam, uintptr(sysColor(5)))
		return a.brushWindow
	case wmEraseBkgnd:
		return 1 // all drawn by WM_PAINT, off screen
	case wmPaint:
		bufferedPaint(hwnd, func(dc uintptr, rc rect) { b.paint(hwnd, dc, rc) })
		return 1
	case wmPrintClient: // a themed button drawing what is behind its corners
		var rc rect
		pGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
		b.paint(hwnd, wParam, rc)
		return 1
	case wmDestroy:
		if b.doc != nil {
			b.doc.destroy()
		}
		pDestroyIcon.Call(b.icon)
	}
	return 0
}

// build makes the controls, then sizes and places the dialog (layout places them).
func (b *updateBox) build(hwnd uintptr) {
	a := g
	s := a.scale
	b.hwnd = hwnd
	b.icon = iconFromRGBA(renderIcon(iconApp, int(s(32))), int(s(32)))
	b.titleFont = createFont(12, a.dpi, 400, false, "Segoe UI")
	b.textFont = createFont(9, a.dpi, 400, false, "Segoe UI")
	b.doc = a.newDocView(hwnd, b.blocks())
	b.doc.margin = s(14)
	b.doc.lines = 0.88 // closer than in the other windows: short items, many of them
	static := func(text string, style uint32) uintptr {
		h, _, _ := pCreateWindowExW.Call(0, up("STATIC"), up(text), uintptr(wsChild|style), 0, 0, 10, 10, hwnd, 0, 0, 0)
		send(h, wmSetFont, b.textFont, 0)
		return h
	}
	b.how = static(T("update.how"), wsVisible)
	b.status = static("", 0x80 /* SS_NOPREFIX */)
	b.bar, _, _ = pCreateWindowExW.Call(0, up("msctls_progress32"), 0, uintptr(wsChild), 0, 0, 10, 10, hwnd, 0, 0, 0)
	send(b.bar, pbmSetRange32, 0, 1000)
	page := T("update.page")
	b.link, _, _ = pCreateWindowExW.Call(0, up("SysLink"), up(`<a>`+page+`</a>`), uintptr(wsChild|wsVisible|wsTabStop),
		0, 0, uintptr(a.textWidth(b.textFont, page)+s(8)), uintptr(a.fontMetrics(b.textFont).Height), hwnd, 0, 0, 0)
	send(b.link, wmSetFont, b.textFont, 0)
	button := func(id uintptr, text string, style uint32) uintptr {
		h, _, _ := pCreateWindowExW.Call(0, up("BUTTON"), up(text), uintptr(wsChild|wsVisible|wsTabStop|style), 0, 0, 10, 10, hwnd, id, 0, 0)
		send(h, wmSetFont, a.fontUI, 0)
		return h
	}
	b.yes = button(1, T("update.yes"), 0x1 /* BS_DEFPUSHBUTTON */)
	b.no = button(2, T("update.no"), 0)
	tip := a.newTooltip(hwnd)
	addTool(tip, hwnd, b.link, nil, linkTip(b.rel.page()))
	b.barH = s(13) + s(30) + s(13)
	a.placeBox(hwnd, a.hwnd, s(700), s(720))
	pSetFocus.Call(b.yes)
}

// layout places the controls in the dialog, for its size.
func (b *updateBox) layout() {
	a := g
	s := a.scale
	var rc rect
	pGetClientRect.Call(b.hwnd, uintptr(unsafe.Pointer(&rc)))
	w, h := rc.Right, rc.Bottom
	pad, textH := s(16), a.fontMetrics(b.textFont).Height
	titleH := a.fontMetrics(b.titleFont).Height
	b.headH = max(s(14)+s(32), s(11)+titleH+s(2)+textH) + s(14)
	move := func(c uintptr, x, y, cw, ch int32) {
		pSetWindowPos.Call(c, 0, uintptr(x), uintptr(y), uintptr(max(cw, 0)), uintptr(max(ch, 0)), 0x0004|0x0010|0x0100) // NOZORDER|NOACTIVATE|NOCOPYBITS
	}
	// the area under the notes: what "Update now" does, as high as its lines; the
	// progress and the status (two lines, for an error) fit in it
	b.lowH = max(b.textHeight(T("update.how"), w-2*pad), s(16)+s(8)+2*textH)
	barTop := h - b.barH
	lowTop := barTop - s(14) - b.lowH
	b.frame = rect{pad, b.headH + s(14), w - pad, lowTop - s(12)}
	move(b.doc.hwnd, b.frame.Left+1, b.frame.Top+1, b.frame.Right-b.frame.Left-2, b.frame.Bottom-b.frame.Top-2)
	move(b.how, pad, lowTop, w-2*pad, b.lowH)
	move(b.bar, pad, lowTop, w-2*pad, s(16))
	if b.erred { // the error in place of the progress
		move(b.status, pad, lowTop, w-2*pad, b.lowH)
	} else {
		move(b.status, pad, lowTop+s(16)+s(8), w-2*pad, b.lowH-s(16)-s(8))
	}
	show := func(c uintptr, on bool) { pShowWindow.Call(c, boolArg(on)) }
	show(b.how, !b.working && !b.erred)
	show(b.bar, b.working)
	show(b.status, b.working || b.erred)
	// the button bar: the release page on the left, the buttons on the right
	btnH := s(30)
	width := func(texts ...string) int32 {
		m := s(88)
		for _, t := range texts {
			m = max(m, a.textWidth(a.fontUI, t)+s(24))
		}
		return m
	}
	noW, yesW := width(T("update.no"), T("update.cancel")), width(T("update.yes"), T("update.retry"))
	move(b.no, w-pad-noW, barTop+s(13), noW, btnH)
	move(b.yes, w-pad-noW-s(8)-yesW, barTop+s(13), yesW, btnH)
	var lr rect
	pGetWindowRect.Call(b.link, uintptr(unsafe.Pointer(&lr)))
	move(b.link, pad, barTop+s(13)+(btnH-textH)/2, lr.Right-lr.Left, textH)
}

// textHeight is the height of text in the dialog's font, in lines of width.
func (b *updateBox) textHeight(text string, width int32) int32 {
	dc, _, _ := pGetDC.Call(b.hwnd)
	old, _, _ := pSelectObject.Call(dc, b.textFont)
	r := rect{0, 0, width, 0}
	pDrawTextW.Call(dc, up(text), ^uintptr(0), uintptr(unsafe.Pointer(&r)), 0x400|0x10|0x800) // CALCRECT|WORDBREAK|NOPREFIX
	pSelectObject.Call(dc, old)
	pReleaseDC.Call(b.hwnd, dc)
	return r.Bottom
}

// paint draws the header band with the shield, the title and the version the user
// has; the white panel with the frame of the notes; and the button bar.
func (b *updateBox) paint(hwnd, dc uintptr, rc rect) {
	a := g
	s := a.scale
	fill := func(r rect, color uint32) {
		br, _, _ := pCreateSolidBrush.Call(uintptr(color))
		pFillRect.Call(dc, uintptr(unsafe.Pointer(&r)), br)
		pDeleteObject.Call(br)
	}
	barTop := rc.Bottom - b.barH
	fill(rect{0, 0, rc.Right, barTop}, sysColor(5))
	fill(rect{0, 0, rc.Right, b.headH - 1}, rgb(246, 248, 252))
	fill(rect{0, b.headH - 1, rc.Right, b.headH}, rgb(221, 226, 236))
	f := b.frame
	for _, r := range []rect{{f.Left, f.Top, f.Right, f.Top + 1}, {f.Left, f.Bottom - 1, f.Right, f.Bottom},
		{f.Left, f.Top, f.Left + 1, f.Bottom}, {f.Right - 1, f.Top, f.Right, f.Bottom}} {
		fill(r, rgb(204, 204, 204))
	}
	paintBar(hwnd, dc, rc, barTop)
	pDrawIconEx.Call(dc, uintptr(s(16)), uintptr(s(14)), b.icon, uintptr(s(32)), uintptr(s(32)), 0, 0, 3)
	color := rgb(0, 51, 153) // title color of task dialogs
	if theme, _, _ := pOpenThemeData.Call(hwnd, up("TASKDIALOG")); theme != 0 {
		var c uint32
		if r, _, _ := pGetThemeColor.Call(theme, tdlgTitle, 0, 3803 /* TMT_TEXTCOLOR */, uintptr(unsafe.Pointer(&c))); r == 0 {
			color = c
		}
		pCloseThemeData.Call(theme)
	}
	pSetBkMode.Call(dc, 1 /* TRANSPARENT */)
	x, y := s(58), s(11)
	text := func(t string, font uintptr, c uint32) int32 {
		old, _, _ := pSelectObject.Call(dc, font)
		pSetTextColor.Call(dc, uintptr(c))
		r := rect{x, y, rc.Right - s(16), b.headH}
		pDrawTextW.Call(dc, up(t), ^uintptr(0), uintptr(unsafe.Pointer(&r)), 0x20|0x800|0x8000) // SINGLELINE|NOPREFIX|END_ELLIPSIS
		pSelectObject.Call(dc, old)
		return a.fontMetrics(font).Height
	}
	y += text(Tf("update.title", b.rel.version), b.titleFont, color) + s(2)
	text(Tf("update.have", appVersion), b.textFont, rgb(32, 32, 32))
}

// download starts the download, in the background: the dialog shows its progress.
func (b *updateBox) download() {
	b.working, b.erred = true, false
	b.run++
	run, stop, r := b.run, &atomic.Bool{}, b.rel
	b.stop = stop
	pEnableWindow.Call(b.yes, 0)
	setText(b.no, T("update.cancel"))
	pSetFocus.Call(b.no)
	b.progress(0)
	b.layout()
	go func() {
		shown := -1
		data, err := updateDownload(r, func(got int) bool {
			if k := got * 1000 / r.size; k != shown { // at most a thousand news
				shown = k
				postEvent(uiEvent{kind: evUpdateProgress, upd: &updateNews{run: run, got: got}})
			}
			return !stop.Load()
		})
		postEvent(uiEvent{kind: evUpdateGot, upd: &updateNews{run: run, data: data, err: err}})
	}()
}

// news shows the release notes of the other new versions, as soon as they come; and
// the progress of the download: once it has come, checked, it starts the update
// (startUpdate), and the dialog closes for eMudScan to close.
func (b *updateBox) news(kind int, n *updateNews) {
	if kind == evUpdateNotes {
		if n.rel.version != b.rel.version || !b.loading {
			return
		}
		b.loading, b.unread = false, !n.read
		b.notes, b.more = n.notes, n.more
		at := b.doc.scroll // the user may have scrolled already: kept
		b.doc.setBlocks(b.blocks())
		b.doc.setScroll(at)
		return
	}
	if n.run != b.run || !b.working {
		return // a download that was stopped
	}
	if kind == evUpdateProgress {
		b.progress(n.got)
		return
	}
	if n.err != nil {
		b.fail(n.err)
		return
	}
	b.say(T("update.preparing"))
	if err := startUpdate(b.rel, n.data); err != nil {
		b.fail(err)
		return
	}
	b.started = true
	b.say(T("update.closing"))
	pEndDialog.Call(b.hwnd, 1)
}

// progress shows how much of the download has come; all of it is being checked.
func (b *updateBox) progress(got int) {
	send(b.bar, pbmSetPos, uintptr(min(got, b.rel.size)*1000/b.rel.size), 0)
	if got >= b.rel.size {
		b.say(T("update.checking"))
		return
	}
	b.say(Tf("update.downloading", b.rel.version, megabytes(got), megabytes(b.rel.size)))
}

// say shows what eMudScan does, at once.
func (b *updateBox) say(text string) {
	setText(b.status, text)
	pUpdateWindow.Call(b.status)
}

// fail says why the update failed: "Update now" becomes "Try again".
func (b *updateBox) fail(err error) {
	b.working, b.erred = false, true
	setText(b.status, Tf("update.notDone", reason(err)))
	setText(b.yes, T("update.retry"))
	setText(b.no, T("update.no"))
	pEnableWindow.Call(b.yes, 1)
	pSetFocus.Call(b.yes)
	b.layout()
	pInvalidateRect.Call(b.status, 0, 1)
}
