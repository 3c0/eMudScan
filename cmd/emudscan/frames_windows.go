//go:build windows

package main

// Frames around the log and the lists. They are not windows: the main window draws
// them, with its background, off screen and then at once, around the controls (which
// it never draws over). So no window lies under another one, and moving the frames
// can never leave a piece of an old image behind.
//
// The border is the themed group box border, but the caption is drawn here, with a
// space on each side, so that its width is known exactly: the spinner and the "Hide
// OK items" boxes are placed right after it.

import (
	"unsafe"
)

// Around the "Hide items with an OK status" check box of a list: after the title,
// a middle dot and an opening bracket; after the check box, the closing bracket.
const (
	hidePrefix = "· [ "
	hideSuffix = " ] "
)

var (
	pOpenThemeData       = moduxtheme.NewProc("OpenThemeData")
	pCloseThemeData      = moduxtheme.NewProc("CloseThemeData")
	pDrawThemeBackground = moduxtheme.NewProc("DrawThemeBackground")
	pGetThemeColor       = moduxtheme.NewProc("GetThemeColor")
	pExcludeClipRect     = modgdi32.NewProc("ExcludeClipRect")
	pSaveDC              = modgdi32.NewProc("SaveDC")
	pRestoreDC           = modgdi32.NewProc("RestoreDC")
	pDrawEdge            = moduser32.NewProc("DrawEdge")
)

const (
	wmNcHitTest    = 0x0084
	wmSetText      = 0x000C
	wmPrintClient  = 0x0318
	rdwInvalidate  = 0x0001
	rdwErase       = 0x0004
	rdwNoChildren  = 0x0040
	rdwAllChildren = 0x0080
	rdwUpdateNow   = 0x0100
	rdwFrame       = 0x0400
)

// frameBox is a frame: where it is in the main window, its caption, and the list
// whose "Hide items with an OK status" box sits on its top line, if any.
type frameBox struct {
	rc       rect
	caption  string // translation key
	list     *listCtl
	boxRight int32 // right side of that check box
}

// frameCaption is the caption as drawn: with a space before and after.
func frameCaption(s string) string { return " " + s + " " }

// captionX is where the caption starts in a frame.
func (a *ui) captionX() int32 { return a.scale(6) }

// textHeight is the height of a line of the interface font.
func (a *ui) textHeight() int32 {
	dc, _, _ := pGetDC.Call(a.hwnd)
	old, _, _ := pSelectObject.Call(dc, a.fontUI)
	var tm textMetric
	pGetTextMetricsW.Call(dc, uintptr(unsafe.Pointer(&tm)))
	pSelectObject.Call(dc, old)
	pReleaseDC.Call(a.hwnd, dc)
	return tm.Height
}

// captionEnd is the x (in the main window) just after the caption of a frame,
// whose left side is at x.
func (a *ui) captionEnd(x int32, caption string) int32 {
	return x + a.captionX() + a.textWidth(a.fontUI, frameCaption(caption))
}

// groupBorder draws the border of a frame whose outer rectangle is r (the top line
// at mid-height of the caption), as a group box of the current theme.
func (a *ui) groupBorder(dc uintptr, r rect) {
	border := rect{r.Left, r.Top + a.textHeight()/2, r.Right, r.Bottom}
	if theme, _, _ := pOpenThemeData.Call(a.hwnd, up("BUTTON")); theme != 0 {
		pDrawThemeBackground.Call(theme, dc, 4 /* BP_GROUPBOX */, 1 /* GBS_NORMAL */, uintptr(unsafe.Pointer(&border)), 0)
		pCloseThemeData.Call(theme)
	} else {
		pDrawEdge.Call(dc, uintptr(unsafe.Pointer(&border)), 6 /* EDGE_ETCHED */, 0xF /* BF_RECT */)
	}
}

// captionColor is the text color of group box captions in the current theme.
func (a *ui) captionColor() uint32 {
	color := sysColor(8) // COLOR_WINDOWTEXT
	if theme, _, _ := pOpenThemeData.Call(a.hwnd, up("BUTTON")); theme != 0 {
		var c uint32
		if r, _, _ := pGetThemeColor.Call(theme, 4, 1, 3803 /* TMT_TEXTCOLOR */, uintptr(unsafe.Pointer(&c))); r == 0 {
			color = c
		}
		pCloseThemeData.Call(theme)
	}
	return color
}

// paintBackground draws the background of the main window and its frames into dc
// (client coordinates). The controls are drawn by themselves, over it.
func (a *ui) paintBackground(dc uintptr, rc rect) {
	pFillRect.Call(dc, uintptr(unsafe.Pointer(&rc)), a.brushWindow)
	if a.sep.Bottom > a.sep.Top { // between the online test and the drives
		b, _, _ := pCreateSolidBrush.Call(uintptr(rgb(200, 200, 200)))
		pFillRect.Call(dc, uintptr(unsafe.Pointer(&a.sep)), b)
		pDeleteObject.Call(b)
	}
	th := a.textHeight()
	color := a.captionColor()
	old, _, _ := pSelectObject.Call(dc, a.fontUI)
	pSetBkMode.Call(dc, 1 /* TRANSPARENT */)
	pSetTextColor.Call(dc, uintptr(color))
	text := func(s string, r rect) {
		pDrawTextW.Call(dc, up(s), ^uintptr(0), uintptr(unsafe.Pointer(&r)), 0x20|0x800) // SINGLELINE|NOPREFIX
	}
	for _, f := range a.frames {
		r := f.rc
		if r.Right <= r.Left || r.Bottom <= r.Top {
			continue
		}
		caption := frameCaption(T(f.caption))
		cx, cw := r.Left+a.captionX(), a.textWidth(a.fontUI, caption)
		end := cx + cw // where the top line starts again
		if f.list != nil {
			end = f.boxRight + a.textWidth(a.fontUI, hideSuffix)
		}
		saved, _, _ := pSaveDC.Call(dc)
		pExcludeClipRect.Call(dc, uintptr(cx), uintptr(r.Top), uintptr(end), uintptr(r.Top+th))
		a.groupBorder(dc, r)
		pRestoreDC.Call(dc, saved)
		text(caption, rect{cx, r.Top, cx + cw, r.Top + th})
		if f.list != nil { // "Title · [ ☐ Hide items with an OK status ] ───"
			pw := a.textWidth(a.fontUI, hidePrefix)
			text(hidePrefix, rect{cx + cw, r.Top, cx + cw + pw, r.Top + th})
			text(hideSuffix, rect{f.boxRight, r.Top, end, r.Top + th})
		}
	}
	pSelectObject.Call(dc, old)
}

// paintMain answers WM_PAINT of the main window: everything is drawn off screen,
// then copied at once (the controls are left out: WS_CLIPCHILDREN).
func (a *ui) paintMain(hwnd uintptr) {
	var ps paintStruct
	hdc, _, _ := pBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	var rc rect
	pGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
	if rc.Right > 0 && rc.Bottom > 0 {
		dc, _, _ := pCreateCompatibleDC.Call(hdc)
		bmp, _, _ := pCreateCompatibleBmp.Call(hdc, uintptr(rc.Right), uintptr(rc.Bottom))
		oldBmp, _, _ := pSelectObject.Call(dc, bmp)
		a.paintBackground(dc, rc)
		pBitBlt.Call(hdc, 0, 0, uintptr(rc.Right), uintptr(rc.Bottom), dc, 0, 0, 0x00CC0020 /* SRCCOPY */)
		pSelectObject.Call(dc, oldBmp)
		pDeleteObject.Call(bmp)
		pDeleteDC.Call(dc)
	}
	pEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
}
