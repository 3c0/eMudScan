//go:build windows

package main

// The About box: a small dialog drawn like a Windows task dialog (white panel with
// the shield and the name in the theme's title style, gray button bar), with
// margins of its own: air on the right of the text and an empty line after the
// last paragraph. The paragraphs are cut into lines here (aboutlayout.go), so that
// a web address is never cut; each line is a link control, so the links open in
// the browser, also from the keyboard. "Licenses and notices", in the button bar,
// opens the notices and licenses that come with the program.

import (
	"strings"
	"syscall"
	"unsafe"
)

var (
	pDialogBoxIndirectParamW = moduser32.NewProc("DialogBoxIndirectParamW")
	pEndDialog               = moduser32.NewProc("EndDialog")
	pAdjustWindowRectExForDp = moduser32.NewProc("AdjustWindowRectExForDpi")
	pGetWindowLongPtrW       = moduser32.NewProc("GetWindowLongPtrW")
	aboutDlgProc             = syscall.NewCallback(aboutProc)
	licDlgProc               = syscall.NewCallback(licProc)
)

const (
	wmInitDialog  = 0x0110
	tdlgPrimary   = 1 // TDLG_PRIMARYPANEL
	tdlgTitle     = 2 // TDLG_MAININSTRUCTIONPANE
	tdlgSecondary = 8 // TDLG_SECONDARYPANEL (the button bar)
	licensesHref  = "#licenses"
)

// modalBox shows an empty dialog (its controls are made in WM_INITDIALOG) and
// returns when it is closed, or false if it could not be made.
func modalBox(owner uintptr, caption string, resizable bool, proc uintptr) bool {
	style := uint32(0x80000000 | 0x00C00000 | 0x00080000 | 0x80 | 0x02000000) // POPUP|CAPTION|SYSMENU|DS_MODALFRAME|CLIPCHILDREN
	if resizable {
		style |= 0x00040000 | 0x00010000 // THICKFRAME|MAXIMIZEBOX
	}
	// style, extended style, no controls, position and size, no menu, default class, title
	tmpl := []uint16{uint16(style), uint16(style >> 16), 0, 0, 0, 0, 0, 100, 100, 0, 0}
	tmpl = append(tmpl, utf16z(caption)...)
	inst, _, _ := pGetModuleHandleW.Call(0)
	r, _, _ := pDialogBoxIndirectParamW.Call(inst, uintptr(unsafe.Pointer(&tmpl[0])), owner, proc, 0)
	return int(r) != -1
}

// placeBox gives a dialog the window size for a client size, centred on another
// window and kept on the screen.
func (a *ui) placeBox(hwnd, over uintptr, w, h int32) {
	style, _, _ := pGetWindowLongPtrW.Call(hwnd, ^uintptr(15) /* GWL_STYLE */)
	exStyle, _, _ := pGetWindowLongPtrW.Call(hwnd, ^uintptr(19) /* GWL_EXSTYLE */)
	wr := rect{0, 0, w, h}
	pAdjustWindowRectExForDp.Call(uintptr(unsafe.Pointer(&wr)), style, 0, exStyle, uintptr(a.dpi))
	ww, wh := wr.Right-wr.Left, wr.Bottom-wr.Top
	var or rect
	pGetWindowRect.Call(over, uintptr(unsafe.Pointer(&or)))
	x, y := (or.Left+or.Right-ww)/2, (or.Top+or.Bottom-wh)/2
	wa := a.workArea()
	x = max(wa.Left, min(x, wa.Right-ww))
	y = max(wa.Top, min(y, wa.Bottom-wh))
	pSetWindowPos.Call(hwnd, 0, uintptr(x), uintptr(y), uintptr(ww), uintptr(wh), 0x0004|0x0010)
}

// paintBar draws the gray button bar of a box (as a task dialog's) from y down.
func paintBar(hwnd, dc uintptr, rc rect, y int32) {
	bar := rect{0, y, rc.Right, rc.Bottom}
	if theme, _, _ := pOpenThemeData.Call(hwnd, up("TASKDIALOG")); theme != 0 {
		pDrawThemeBackground.Call(theme, dc, tdlgSecondary, 0, uintptr(unsafe.Pointer(&bar)), 0)
		pCloseThemeData.Call(theme)
		return
	}
	face, _, _ := pGetSysColorBrush.Call(15 /* COLOR_BTNFACE */)
	pFillRect.Call(dc, uintptr(unsafe.Pointer(&bar)), face)
	line := rect{0, y, rc.Right, y + 1}
	edge, _, _ := pGetSysColorBrush.Call(16 /* COLOR_BTNSHADOW */)
	pFillRect.Call(dc, uintptr(unsafe.Pointer(&line)), edge)
}

// bufferedPaint answers WM_PAINT: paint draws off screen, then it is copied at once.
func bufferedPaint(hwnd uintptr, paint func(dc uintptr, rc rect)) {
	var ps paintStruct
	hdc, _, _ := pBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	defer pEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	var rc rect
	pGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
	if rc.Right <= 0 || rc.Bottom <= 0 {
		return
	}
	dc, _, _ := pCreateCompatibleDC.Call(hdc)
	bmp, _, _ := pCreateCompatibleBmp.Call(hdc, uintptr(rc.Right), uintptr(rc.Bottom))
	oldBmp, _, _ := pSelectObject.Call(dc, bmp)
	paint(dc, rc)
	pBitBlt.Call(hdc, 0, 0, uintptr(rc.Right), uintptr(rc.Bottom), dc, 0, 0, 0x00CC0020 /* SRCCOPY */)
	pSelectObject.Call(dc, oldBmp)
	pDeleteObject.Call(bmp)
	pDeleteDC.Call(dc)
}

// linkURL reads the address of a clicked link (NMLINK: the header, then LITEM:
// mask, iLink, state, stateMask, szID[48], szUrl).
func linkURL(lParam uintptr) string {
	url := (*[2084]uint16)(unsafe.Add(ptr(lParam), unsafe.Sizeof(nmhdr{})+16+96))
	return syscall.UTF16ToString(url[:])
}

// ---------------------------------------------------------------------------
// About

type aboutBox struct {
	icon, titleFont uintptr
	textFont        uintptr // the links' own font: a link control may delete its font when it is destroyed
	footLink        uintptr // "Licenses and notices", on the button bar
	title           string
	textX, titleY   int32
	iconX, iconY    int32
	barTop          int32 // top of the button bar
}

var aboutDlg *aboutBox

func (a *ui) about() {
	aboutDlg = &aboutBox{title: Tf("about.title", appVersionDate())}
	if !modalBox(a.hwnd, T("about.caption"), false, aboutDlgProc) {
		msgBox(a.hwnd, aboutDlg.title+"\n\n"+linkTagRe.ReplaceAllString(Tf("about.text", userAgent()), ""), 0x40)
	}
	if f := aboutDlg.textFont; f != 0 {
		pDeleteObject.Call(f) // fails harmlessly if a link control already did it
	}
	aboutDlg = nil
}

func aboutProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	a, b := g, aboutDlg
	if b == nil {
		return 0
	}
	switch msg {
	case wmInitDialog:
		b.build(hwnd)
		return 0 // the focus is set on OK
	case wmCommand:
		if id := loword(wParam); id == 1 || id == 2 { // IDOK, IDCANCEL (Esc, ×)
			pEndDialog.Call(hwnd, uintptr(id))
			return 1
		}
	case wmNotify:
		h := (*nmhdr)(ptr(lParam))
		if h.Code == -2 || h.Code == -4 { // NM_CLICK, NM_RETURN on a link
			if url := linkURL(lParam); url == licensesHref {
				a.licenses(hwnd)
			} else {
				openTarget(hwnd, url)
			}
			return 1
		}
	case wmCtlColorStatic: // the links: on the white panel, or on the button bar
		if lParam == b.footLink {
			pSetBkColor.Call(wParam, uintptr(sysColor(15)))
			face, _, _ := pGetSysColorBrush.Call(15 /* COLOR_BTNFACE */)
			return face
		}
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
		pDestroyIcon.Call(b.icon)
		pDeleteObject.Call(b.titleFont)
	}
	return 0
}

// build makes the lines of text, the links and the OK button, then sizes and
// places the box.
func (b *aboutBox) build(hwnd uintptr) {
	a := g
	s := a.scale
	// as in a task dialog: the shield close to the corner, the text close to it,
	// and on the right of the text the margin there is on the left of the shield
	left, top := s(11), s(10)
	isz := s(32)
	inset := isz / 8 // the shield is drawn a little inside its square
	b.icon = iconFromRGBA(renderIcon(iconApp, int(isz)), int(isz))
	b.titleFont = createFont(12, a.dpi, 400, false, "Segoe UI")
	b.textFont = createFont(9, a.dpi, 400, false, "Segoe UI")
	b.textX = left + isz + s(9)
	textW, right := s(460), left+inset+s(4) // text looks closer to an edge than the solid shield
	lineH := a.fontMetrics(b.textFont).Height
	titleH := a.fontMetrics(b.titleFont).Height
	b.titleY = top + (isz-titleH)/2
	y := max(top+isz, b.titleY+titleH) + s(14)

	dc, _, _ := pGetDC.Call(hwnd)
	old, _, _ := pSelectObject.Call(dc, b.textFont)
	measure := func(t string) int32 {
		u := utf16z(t)
		var sz point
		pGetTextExtentPoint32.Call(dc, uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)-1), uintptr(unsafe.Pointer(&sz)))
		return sz.X
	}
	link := func(markup string, x, y, w int32) uintptr {
		h, _, _ := pCreateWindowExW.Call(0, up("SysLink"), up(markup), uintptr(wsChild|wsVisible|wsTabStop),
			uintptr(x), uintptr(y), uintptr(w), uintptr(lineH), hwnd, 0, 0, 0)
		send(h, wmSetFont, b.textFont, 0)
		return h
	}
	// the lines, cut to textW; the box is as wide as the longest one, so that the
	// margin on its right is the one on the left of the shield
	var paras [][]string
	used := int32(0)
	for _, para := range strings.Split(Tf("about.text", userAgent()), "\n\n") {
		lines := wrapMarkup(para, textW, measure)
		for _, l := range lines {
			used = max(used, measure(linkTagRe.ReplaceAllString(l, "")))
		}
		paras = append(paras, lines)
	}
	w := b.textX + used + right
	// one control per line (up to the right side, so that it never cuts the line
	// again), an empty line after each paragraph, the last one included; each link
	// has a tooltip saying where it leads
	tip := a.newTooltip(hwnd)
	for _, lines := range paras {
		for _, line := range lines {
			h := link(line, b.textX, y, w-b.textX)
			for i, l := range lineLinks(line, measure) {
				addTool(tip, h, uintptr(i+1), &rect{l.x0, 0, l.x1, lineH}, linkTip(l.url))
			}
			y += lineH
		}
		y += lineH
	}
	y += s(4) // under the last line, as much air as above the title
	// the button bar: "Licenses and notices" on the left, OK on the right
	btnW, btnH, pad := s(88), s(30), s(13)
	b.barTop = y
	lic := T("about.licenses")
	b.footLink = link(`<a href="`+licensesHref+`">`+lic+`</a>`, left+inset, y+pad+(btnH-lineH)/2, measure(lic)+s(8))
	addTool(tip, hwnd, b.footLink, nil, T("tip.licenses"))
	pSelectObject.Call(dc, old)
	pReleaseDC.Call(hwnd, dc)
	ok, _, _ := pCreateWindowExW.Call(0, up("BUTTON"), up("OK"), uintptr(wsChild|wsVisible|wsTabStop|0x1 /* BS_DEFPUSHBUTTON */),
		uintptr(w-right-btnW), uintptr(y+pad), uintptr(btnW), uintptr(btnH), hwnd, 1 /* IDOK */, 0, 0)
	send(ok, wmSetFont, a.fontUI, 0)
	b.iconX, b.iconY = left, top
	a.placeBox(hwnd, a.hwnd, w, y+pad+btnH+pad)
	pSetFocus.Call(ok)
}

// paint draws the white panel with the shield and the title, and the button bar.
func (b *aboutBox) paint(hwnd, dc uintptr, rc rect) {
	a := g
	panel := rect{0, 0, rc.Right, b.barTop}
	pFillRect.Call(dc, uintptr(unsafe.Pointer(&panel)), a.brushWindow)
	color := rgb(0, 51, 153) // title color of task dialogs
	if theme, _, _ := pOpenThemeData.Call(hwnd, up("TASKDIALOG")); theme != 0 {
		pDrawThemeBackground.Call(theme, dc, tdlgPrimary, 0, uintptr(unsafe.Pointer(&panel)), 0)
		var c uint32
		if r, _, _ := pGetThemeColor.Call(theme, tdlgTitle, 0, 3803 /* TMT_TEXTCOLOR */, uintptr(unsafe.Pointer(&c))); r == 0 {
			color = c
		}
		pCloseThemeData.Call(theme)
	}
	paintBar(hwnd, dc, rc, b.barTop)
	s := a.scale
	pDrawIconEx.Call(dc, uintptr(b.iconX), uintptr(b.iconY), b.icon, uintptr(s(32)), uintptr(s(32)), 0, 0, 3)
	old, _, _ := pSelectObject.Call(dc, b.titleFont)
	pSetBkMode.Call(dc, 1 /* TRANSPARENT */)
	pSetTextColor.Call(dc, uintptr(color))
	tr := rect{b.textX, b.titleY, rc.Right, b.barTop}
	pDrawTextW.Call(dc, up(b.title), ^uintptr(0), uintptr(unsafe.Pointer(&tr)), 0x20|0x800) // SINGLELINE|NOPREFIX
	pSelectObject.Call(dc, old)
}

// ---------------------------------------------------------------------------
// Licenses and notices: a resizable box with the text in a read-only edit control
// (it scrolls, and its text can be selected and copied).

type licBox struct {
	edit, ok, font uintptr
	over           uintptr // the window it is centred on
	barH           int32
}

var licDlg *licBox

func (a *ui) licenses(owner uintptr) {
	licDlg = &licBox{over: owner}
	modalBox(owner, T("lic.caption"), true, licDlgProc)
	if licDlg.font != 0 {
		pDeleteObject.Call(licDlg.font)
	}
	licDlg = nil
}

func licProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	a, b := g, licDlg
	if b == nil {
		return 0
	}
	s := a.scale
	switch msg {
	case wmInitDialog:
		b.font = createFont(9, a.dpi, 400, false, "Consolas")
		text := strings.ReplaceAll(licensesText(), "\n", "\r\n")
		b.edit, _, _ = pCreateWindowExW.Call(0, up("EDIT"), up(text), uintptr(wsChild|wsVisible|wsVScroll|wsTabStop|0x4|0x40|0x800), // MULTILINE|AUTOVSCROLL|READONLY
			0, 0, 10, 10, hwnd, 100, 0, 0)
		send(b.edit, wmSetFont, b.font, 0)
		m := uintptr(s(14))
		send(b.edit, 0x00D3 /* EM_SETMARGINS */, 0x1|0x2, m|m<<16)
		b.ok, _, _ = pCreateWindowExW.Call(0, up("BUTTON"), up("OK"), uintptr(wsChild|wsVisible|wsTabStop|0x1 /* BS_DEFPUSHBUTTON */),
			0, 0, 10, 10, hwnd, 1 /* IDOK */, 0, 0)
		send(b.ok, wmSetFont, a.fontUI, 0)
		b.barH = s(13) + s(30) + s(13)
		a.placeBox(hwnd, b.over, s(660), s(540))
		pSetFocus.Call(b.edit)
		send(b.edit, 0x00B1 /* EM_SETSEL */, 0, 0)
		return 0
	case wmSize:
		w, h := int32(loword(lParam)), int32(hiword(lParam))
		top := s(10)
		pSetWindowPos.Call(b.edit, 0, 0, uintptr(top), uintptr(w), uintptr(max(h-b.barH-top, 0)), 0x0004|0x0010)
		pSetWindowPos.Call(b.ok, 0, uintptr(w-s(16)-s(88)), uintptr(h-b.barH+s(13)), uintptr(s(88)), uintptr(s(30)), 0x0004|0x0010)
		pInvalidateRect.Call(hwnd, 0, 0)
		return 1
	case wmGetMinMaxInfo:
		pt := (*[2]int32)(unsafe.Add(ptr(lParam), 24)) // ptMinTrackSize
		pt[0], pt[1] = s(420), s(300)
		return 1
	case wmCommand:
		if id := loword(wParam); id == 1 || id == 2 {
			pEndDialog.Call(hwnd, uintptr(id))
			return 1
		}
	case wmCtlColorStatic: // the read-only text, on white
		pSetBkColor.Call(wParam, uintptr(sysColor(5)))
		return a.brushWindow
	case wmEraseBkgnd:
		return 1
	case wmPaint, wmPrintClient:
		draw := func(dc uintptr, rc rect) {
			panel := rect{0, 0, rc.Right, rc.Bottom - b.barH}
			pFillRect.Call(dc, uintptr(unsafe.Pointer(&panel)), a.brushWindow)
			paintBar(hwnd, dc, rc, rc.Bottom-b.barH)
		}
		if msg == wmPaint {
			bufferedPaint(hwnd, draw)
		} else {
			var rc rect
			pGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
			draw(wParam, rc)
		}
		return 1
	}
	return 0
}
