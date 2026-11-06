//go:build windows

package main

// "Report saved": a box drawn like the About box (and a Windows task dialog), which
// says where the report was saved. The path of the report is a link, on one line, with
// a tooltip: it shows the report in File Explorer. The box is as wide as the path
// needs, within the screen.

import (
	"syscall"
	"unsafe"
)

type savedBox struct {
	aboutBox
	text, path string
	over       uintptr // the window it is centred on
}

var (
	savedDlg     *savedBox
	savedDlgProc = syscall.NewCallback(savedProc)
)

// showSaved says that the report was saved in path, after text.
func (a *ui) showSaved(owner uintptr, text, path string) {
	savedDlg = &savedBox{aboutBox: aboutBox{title: T("saved.title")}, text: text, path: path, over: owner}
	if !modalBox(owner, appName+simulationMark, false, savedDlgProc) {
		msgBox(owner, text+"\n"+path, 0x40 /* ICONINFORMATION */)
	}
	if f := savedDlg.textFont; f != 0 {
		pDeleteObject.Call(f) // fails harmlessly if a link control already did it
	}
	savedDlg = nil
}

func savedProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	a, b := g, savedDlg
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
		if h := (*nmhdr)(ptr(lParam)); h.Code == -2 || h.Code == -4 { // NM_CLICK, NM_RETURN on the link
			openPath(hwnd, b.path)
			return 1
		}
	case wmCtlColorStatic: // the text and the link, on the white panel
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

// build makes the text, the link and the OK button, then sizes and places the box,
// with the margins of the About box.
func (b *savedBox) build(hwnd uintptr) {
	a := g
	s := a.scale
	left, top := s(11), s(10)
	isz := s(32)
	inset := isz / 8
	b.icon = iconFromRGBA(renderIcon(iconInfo, int(isz)), int(isz))
	b.titleFont = createFont(12, a.dpi, 400, false, "Segoe UI")
	b.textFont = createFont(9, a.dpi, 400, false, "Segoe UI")
	b.textX = left + isz + s(9)
	right := left + inset + s(4)
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
	link := func(markup string, x, y, w, h int32) uintptr {
		c, _, _ := pCreateWindowExW.Call(0, up("SysLink"), up(markup), uintptr(wsChild|wsVisible|wsTabStop),
			uintptr(x), uintptr(y), uintptr(w), uintptr(h), hwnd, 0, 0, 0)
		send(c, wmSetFont, b.textFont, 0)
		return c
	}
	// the path on one line, as wide as the screen allows; the text above it, cut to
	// the same width (or more, for a short path)
	wa := a.workArea()
	most := wa.Right - wa.Left - s(40) - b.textX - right
	shown := shownPath(b.path) // what the link opens is b.path
	pathW := measure(shown) + s(4)
	pathH := lineH
	if pathW > most { // too long for the screen: on several lines
		pathH = lineH * ((pathW + most - 1) / most)
		pathW = most
	}
	lines := wrapMarkup(b.text, max(s(460), pathW), measure)
	used := pathW
	for _, l := range lines {
		used = max(used, measure(l))
	}
	w := b.textX + used + right
	for _, l := range lines {
		link(l, b.textX, y, w-b.textX, lineH)
		y += lineH
	}
	y += s(8)
	path := link(`<a href="#report">`+shown+`</a>`, b.textX, y, pathW, pathH) // as wide as the path: so is its tooltip
	addTool(a.newTooltip(hwnd), hwnd, path, nil, linkTip(b.path))
	y += pathH + lineH + s(4) // an empty line under the path, as under the text of the About box
	pSelectObject.Call(dc, old)
	pReleaseDC.Call(hwnd, dc)
	btnW, btnH, pad := s(88), s(30), s(13)
	b.barTop = y
	ok, _, _ := pCreateWindowExW.Call(0, up("BUTTON"), up("OK"), uintptr(wsChild|wsVisible|wsTabStop|0x1 /* BS_DEFPUSHBUTTON */),
		uintptr(w-right-btnW), uintptr(y+pad), uintptr(btnW), uintptr(btnH), hwnd, 1 /* IDOK */, 0, 0)
	send(ok, wmSetFont, a.fontUI, 0)
	b.iconX, b.iconY = left, top
	a.placeBox(hwnd, b.over, w, y+pad+btnH+pad)
	pSetFocus.Call(ok)
}
