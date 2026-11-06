//go:build windows

package main

// "I accept the online test": the label of the box that allows the live test. It is
// drawn here, in amber with a dashed underline, so that it catches the eye without
// looking like a link (blue, solid underline): hovering it shows the tooltip that
// explains what the test sends, clicking it ticks or unticks the box, as a label.

import (
	"syscall"
	"unsafe"
)

var optinProc = syscall.NewCallback(optinWndProc)

// optinColor is the color of the label and of its underline (amber).
var optinColor = rgb(176, 88, 0)

func optinWndProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	a := g
	switch msg {
	case wmEraseBkgnd:
		return 1
	case wmPaint:
		bufferedPaint(hwnd, func(dc uintptr, rc rect) {
			pFillRect.Call(dc, uintptr(unsafe.Pointer(&rc)), a.brushWindow)
			text := T("chk.online")
			color := optinColor
			if on, _, _ := pIsWindowEnabled.Call(hwnd); on == 0 {
				color = sysColor(17) // COLOR_GRAYTEXT, during a scan
			}
			old, _, _ := pSelectObject.Call(dc, a.fontUI)
			var tm textMetric
			pGetTextMetricsW.Call(dc, uintptr(unsafe.Pointer(&tm)))
			pSetBkMode.Call(dc, 1 /* TRANSPARENT */)
			pSetTextColor.Call(dc, uintptr(color))
			ty := (rc.Bottom - tm.Height) / 2
			tr := rect{0, ty, rc.Right, ty + tm.Height}
			pDrawTextW.Call(dc, up(text), ^uintptr(0), uintptr(unsafe.Pointer(&tr)), 0x20|0x800) // SINGLELINE|NOPREFIX
			pSelectObject.Call(dc, old)
			// the dashed underline, under the baseline
			w := min(a.textWidth(a.fontUI, text), rc.Right)
			uy := ty + tm.Ascent + a.scale(2)
			thick := max(a.scale(1), 1)
			dash, gap := a.scale(3), a.scale(2)
			brush, _, _ := pCreateSolidBrush.Call(uintptr(color))
			for x := int32(0); x < w; x += dash + gap {
				r := rect{x, uy, min(x+dash, w), uy + thick}
				pFillRect.Call(dc, uintptr(unsafe.Pointer(&r)), brush)
			}
			pDeleteObject.Call(brush)
		})
		return 0
	case wmLButtonUp: // as the label of the box
		send(a.hOnline, 0x00F5 /* BM_CLICK */, 0, 0)
		return 0
	case wmEnable:
		pInvalidateRect.Call(hwnd, 0, 0)
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
	return r
}
