package main

import "unsafe"

// The scale of the display changes: a setting of Windows, or the windows moved to a
// screen of another scale. Windows tells each window (WM_DPICHANGED), in an order of
// its own. The main window follows it: new fonts and icons, its controls placed again.
// The windows open above it then follow the main window (rescale), each once Windows
// has given it that scale too (scaleGiven), so that its frame is the one it is sized
// for: a window left alone on a screen of another scale keeps the scale it has.
//
// The size of a window at a scale comes from the size it had before its first change
// of scale, or since its user last resized it (baseSize): not from the size it has,
// which the screen may have cut at a larger scale. Back at a scale, a window is back
// at the size it had there.

// wmRescale tells a window above the main one to show at the scale of the main
// window: wParam is the DPI it was made for.
const wmRescale = wmApp + 5

// wmSizing: the user is resizing a window by its border.
const wmSizing = 0x0214

// baseSize is a size of a window, and the DPI it had it at (0: none yet).
type baseSize struct {
	w, h int32
	dpi  int
}

// at is the base size at the scale of the main window, taken from the size w × h that
// the window has at the DPI was when it has none yet.
func (b *baseSize) at(a *ui, w, h int32, was int) (int32, int32) {
	if b.dpi == 0 {
		*b = baseSize{w, h, was}
	}
	return int32(int(b.w) * a.dpi / b.dpi), int32(int(b.h) * a.dpi / b.dpi)
}

var (
	pGetWindow                  = moduser32.NewProc("GetWindow")
	pFindWindowExW              = moduser32.NewProc("FindWindowExW")
	pSetDialogDpiChangeBehavior = moduser32.NewProc("SetDialogDpiChangeBehavior")
)

// follower is a window above the main one: the DPI it is made for, and the DPI that
// Windows gave it last.
type follower struct {
	hwnd     uintptr
	dpi, got int
	base     baseSize // of its client area, or of the whole window
}

// followers are the windows above the main one, in the order they were made: a
// window before those it opened.
var followers []follower

// follows notes that hwnd, just made, follows the scale of the main window. A dialog
// is left as it is by Windows, which would otherwise resize it and move its controls
// itself, without their fonts (SetDialogDpiChangeBehavior: in Windows 10 1703 and
// later, as this way of Windows).
func (a *ui) follows(hwnd uintptr, dialog bool) {
	a.followersOpen()
	followers = append(followers, follower{hwnd: hwnd, dpi: a.dpi, got: a.dpi})
	if dialog && pSetDialogDpiChangeBehavior.Find() == nil {
		pSetDialogDpiChangeBehavior.Call(hwnd, 1 /* DDC_DISABLE_ALL */, 1)
	}
}

// followersOpen forgets the followers that are closed.
func (a *ui) followersOpen() {
	open := followers[:0]
	for _, f := range followers {
		if r, _, _ := pIsWindow.Call(f.hwnd); r != 0 {
			open = append(open, f)
		}
	}
	followers = open
}

// scaleGiven notes the DPI that Windows gives hwnd (WM_DPICHANGED), which follows the
// main window if that is its scale too.
func (a *ui) scaleGiven(hwnd uintptr, dpi int) {
	for i := range followers {
		if followers[i].hwnd == hwnd {
			followers[i].got = dpi
		}
	}
	a.rescale(hwnd)
}

// rescale shows hwnd at the scale of the main window, when it is made for another
// and Windows has given it that scale.
func (a *ui) rescale(hwnd uintptr) {
	for i, f := range followers {
		if f.hwnd == hwnd && f.dpi != a.dpi && f.got == a.dpi {
			followers[i].dpi = a.dpi
			send(hwnd, wmRescale, uintptr(f.dpi), 0)
			return
		}
	}
}

// rescaleAll shows the windows above the main one at its scale: each one before
// those it opened, which are placed over it.
func (a *ui) rescaleAll() {
	a.followersOpen()
	for _, f := range append([]follower(nil), followers...) {
		a.rescale(f.hwnd)
	}
}

// replaceFonts gives the controls of the windows above the main one the fonts that
// replace those of the main window they use, which are about to be deleted.
func (a *ui) replaceFonts(old, by []uintptr) {
	a.followersOpen()
	for _, f := range followers {
		replaceFontsIn(f.hwnd, old, by)
	}
}

// replaceFontsIn gives the controls of hwnd that use a font of old the font of by
// that replaces it.
func replaceFontsIn(hwnd uintptr, old, by []uintptr) {
	childList = nil
	pEnumChildWindows.Call(hwnd, collectChilds, 0)
	for _, c := range childList {
		font := send(c, 0x0031 /* WM_GETFONT */, 0, 0)
		for i, o := range old {
			if font != 0 && font == o {
				send(c, wmSetFont, by[i], 1)
			}
		}
	}
}

// userSized notes that the user resizes hwnd: its size is the one it keeps.
func (a *ui) userSized(hwnd uintptr) {
	if hwnd == a.hwnd {
		a.base.dpi = 0
	}
	for i := range followers {
		if followers[i].hwnd == hwnd {
			followers[i].base.dpi = 0
		}
	}
}

// baseOf is the base size of a window above the main one.
func baseOf(hwnd uintptr) *baseSize {
	for i := range followers {
		if followers[i].hwnd == hwnd {
			return &followers[i].base
		}
	}
	return &baseSize{}
}

// scaledClient is the size of the client area of hwnd at the scale of the main
// window, for a window made for the DPI was.
func (a *ui) scaledClient(hwnd uintptr, was int) (w, h int32) {
	var rc rect
	pGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
	return baseOf(hwnd).at(a, rc.Right, rc.Bottom, was)
}

// resizeScaled gives hwnd, made for the DPI was, its size at the scale of the main
// window: around its center, within the screen. A maximized window stays so.
func (a *ui) resizeScaled(hwnd uintptr, was int) {
	if z, _, _ := pIsZoomed.Call(hwnd); z != 0 {
		return
	}
	var r rect
	pGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	w, h := baseOf(hwnd).at(a, r.Right-r.Left, r.Bottom-r.Top, was)
	wa := a.workArea()
	w, h = min(w, wa.Right-wa.Left), min(h, wa.Bottom-wa.Top)
	x := max(wa.Left, min((r.Left+r.Right-w)/2, wa.Right-w))
	y := max(wa.Top, min((r.Top+r.Bottom-h)/2, wa.Bottom-h))
	pSetWindowPos.Call(hwnd, 0, uintptr(x), uintptr(y), uintptr(w), uintptr(h), 0x0004|0x0010) // NOZORDER|NOACTIVATE
}

// mainRect is where the main window, made for the DPI was, goes at its new scale:
// where Windows suggests (r), with its size at that scale, within its screen.
func (a *ui) mainRect(r rect, was int) rect {
	var cur rect
	pGetWindowRect.Call(a.hwnd, uintptr(unsafe.Pointer(&cur)))
	w, h := a.base.at(a, cur.Right-cur.Left, cur.Bottom-cur.Top, was)
	wa := a.workArea()
	w, h = min(w, wa.Right-wa.Left), min(h, wa.Bottom-wa.Top)
	x := max(wa.Left, min(r.Left, wa.Right-w))
	y := max(wa.Top, min(r.Top, wa.Bottom-h))
	return rect{x, y, x + w, y + h}
}

// destroyControls destroys the controls of a window that makes them again, and the
// tooltips it has for them.
func destroyControls(hwnd uintptr) {
	for {
		c, _, _ := pGetWindow.Call(hwnd, 5 /* GW_CHILD */)
		if c == 0 {
			break
		}
		pDestroyWindow.Call(c)
	}
	var tips []uintptr
	for t := uintptr(0); ; {
		t, _, _ = pFindWindowExW.Call(0, t, up("tooltips_class32"), 0)
		if t == 0 {
			break
		}
		if o, _, _ := pGetWindow.Call(t, 4 /* GW_OWNER */); o == hwnd {
			tips = append(tips, t)
		}
	}
	for _, t := range tips {
		pDestroyWindow.Call(t)
	}
}
