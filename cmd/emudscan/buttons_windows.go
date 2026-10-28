//go:build windows

package main

// Standard Windows push buttons, with a small icon beside the text (BM_SETIMAGE:
// with visual styles the button draws both). The icon turns grey when the button
// is disabled. Also the frames of the progress animation.

import (
	"runtime"
	"sync"
	"syscall"
)

const (
	bmSetImage = 0x00F7
	wmEnable   = 0x000A
)

var (
	btnGlyphs     = map[uintptr]glyph{}
	glyphIcons    = map[[3]uint32]uintptr{}
	btnSubclassCb = syscall.NewCallback(btnSubclass)
)

// iconButton creates a push button with an icon; def makes it the default button
// (the main action, drawn with an accent border).
func (a *ui) iconButton(parent uintptr, text string, id int, def bool, gl glyph) uintptr {
	style := uintptr(wsChild | wsVisible | wsTabStop)
	if def {
		style |= 1 // BS_DEFPUSHBUTTON
	}
	h, _, _ := pCreateWindowExW.Call(0, up("BUTTON"), up(text), style, 0, 0, 10, 10, parent, uintptr(id), 0, 0)
	send(h, wmSetFont, a.fontUI, 1)
	btnGlyphs[h] = gl
	pSetWindowSubclass.Call(h, btnSubclassCb, 2, 0)
	a.updateButtonIcon(h)
	return h
}

// glyphColor: the colour of a button icon.
func glyphColor(gl glyph, enabled bool) uint32 {
	if !enabled {
		return 0xA0A0A0
	}
	switch gl {
	case glyphPlay:
		return 0x1B8A3A
	case glyphStop:
		return 0xC42B1C
	case glyphRefresh:
		return 0x0F6CBD
	}
	return 0x404040
}

func (a *ui) glyphIcon(gl glyph, color uint32, size int32) uintptr {
	key := [3]uint32{uint32(gl), color, uint32(size)}
	if h, ok := glyphIcons[key]; ok {
		return h
	}
	// a transparent margin on the right separates the icon from the text
	n, pad := int(size), int(size)/4
	px := render(n, glyphLayers(gl, hexColor(color, 1)))
	wide := make([]byte, (n+pad)*n*4)
	for y := 0; y < n; y++ {
		copy(wide[y*(n+pad)*4:], px[y*n*4:(y+1)*n*4])
	}
	h := iconFromRGBAWH(wide, n+pad, n)
	glyphIcons[key] = h
	return h
}

// updateButtonIcon sets the icon of a button for its glyph and state.
func (a *ui) updateButtonIcon(h uintptr) {
	gl, ok := btnGlyphs[h]
	if !ok {
		return
	}
	var ic uintptr
	if gl != glyphNone {
		en, _, _ := pIsWindowEnabled.Call(h)
		ic = a.glyphIcon(gl, glyphColor(gl, en != 0), a.scale(16))
	}
	send(h, bmSetImage, 1 /* IMAGE_ICON */, ic)
}

func (a *ui) setButtonGlyph(h uintptr, gl glyph) {
	btnGlyphs[h] = gl
	a.updateButtonIcon(h)
}

// refreshButtonIcons sets the icons again after they were re-created (DPI change).
func (a *ui) refreshButtonIcons() {
	for h := range btnGlyphs {
		a.updateButtonIcon(h)
	}
}

func btnSubclass(hwnd uintptr, msg uint32, wParam, lParam, id, ref uintptr) uintptr {
	switch msg {
	case wmEnable:
		g.updateButtonIcon(hwnd)
	case wmDestroy:
		delete(btnGlyphs, hwnd)
	}
	r, _, _ := pDefSubclassProc.Call(hwnd, uintptr(msg), wParam, lParam)
	return r
}

// spinner holds the frames of the progress animation. Their pixels are computed in
// the background at start-up, so the window opens at once; the icons are made the
// first time each frame is shown.
type spinner struct {
	size  int
	px    [][]byte
	icons []uintptr
	done  chan struct{}
}

func newSpinner(size int) *spinner {
	s := &spinner{size: size, px: make([][]byte, spinFrames), icons: make([]uintptr, spinFrames), done: make(chan struct{})}
	go func() {
		var wg sync.WaitGroup
		work := make(chan int)
		for w := 0; w < max(1, runtime.NumCPU()-1); w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for f := range work {
					s.px[f] = spinnerFrame(f, size)
				}
			}()
		}
		for f := range s.px {
			work <- f
		}
		close(work)
		wg.Wait()
		close(s.done)
	}()
	return s
}

// spinnerFrame draws one frame. Should the drawing fail (a bug), the frame stays
// empty, rather than the failure closing eMudScan at start-up without a word: it
// runs on its own goroutine, out of reach of any other recovery.
func spinnerFrame(f, size int) (px []byte) {
	defer func() {
		if recover() != nil {
			px = nil
		}
	}()
	return spinnerShield(f, spinFrames, size)
}

// icon is the frame's icon, 0 when the frame could not be drawn (nothing is shown).
func (s *spinner) icon(f int) uintptr {
	f %= spinFrames
	if s.icons[f] == 0 {
		<-s.done
		if s.px[f] == nil {
			return 0
		}
		s.icons[f] = iconFromRGBA(s.px[f], s.size)
	}
	return s.icons[f]
}

func (s *spinner) destroy() {
	for i, ic := range s.icons {
		if ic != 0 {
			pDestroyIcon.Call(ic)
			s.icons[i] = 0
		}
	}
}
