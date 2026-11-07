//go:build windows

package main

// The log: lines of colored text drawn with GDI, like the lists (so with ClearType,
// where the Windows rich edit control smooths text in gray), all of the same height
// and scrolled by whole lines, so that no line is ever cut. Text can be selected
// with the mouse and copied (Ctrl+C, Ctrl+A, right-click menu).
//
// The console of the Fix window is a log too, drawn as a PowerShell window: light
// text on black, without a border (the window draws a frame around it), and its text
// cannot be selected.

import (
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

var (
	pGetWindowDC = moduser32.NewProc("GetWindowDC")
	logProc      = syscall.NewCallback(logWndProc)
)

const wmNcPaint = 0x0085

// The fonts of the log: Consolas for the text, the symbol fonts for the marks
// (✔ ℹ from Segoe UI Symbol, ❗ ❌ from Segoe UI Emoji), each regular and bold.
const (
	logMono = iota
	logSymbol
	logEmoji
	logBold = 3 // added to one of the above
)

var logFaces = [3]string{"Consolas", "Segoe UI Symbol", "Segoe UI Emoji"}

// logFrag is a piece of a row drawn with one font and one color.
type logFrag struct {
	off, n int // its characters in the text of the log
	font   int
	color  uint32
	x, w   int32
}

type logSpan struct {
	n     int
	font  int
	color uint32
}

// logLine is a line of the log: its text, as appended, and its rows, as cut to
// the width of the log.
type logLine struct {
	off   int // start of the line in the text
	spans []logSpan
	rows  [][]logFrag
	done  bool // ended by a line break
	// in the left column (marks), on its first row: the result of a command
	mark      string
	markColor uint32
}

type logRowRef struct{ line, row int }

type logView struct {
	hwnd       uintptr
	fonts      [6]uintptr
	ascents    [6]int32
	ascent     int32 // baseline of every row
	pitch      int32 // height of every row
	text       []uint16
	lines      []logLine
	rows       []logRowRef
	width      int32 // width the lines were cut to
	top        int   // first row shown
	selA, selB int
	down       bool
	moved      bool
	downPt     point
	wheel      int32
	console    bool    // the console of the Fix window: dark
	marks      bool    // a left column for the results of the commands (setMark)
	markFont   uintptr // the font of ✔ and ✘, larger than the text
	markAscent int32
	markTops   map[string]int32 // where each mark is drawn in its row (markTop)
	bg         uintptr          // its background brush
	// the copy button of the console, in its top right corner, as the commands of the
	// details page have: copyText is what it copies, copyTip its tooltip
	copyText            func() string
	copyTip             func() string
	copyHover, copyDown bool
	copied              time.Time
	tip                 uintptr
}

// copyRect is where the copy button is: in the top right corner, as in the details.
func (v *logView) copyRect() rect {
	w, _ := v.clientSize()
	s := g.scale
	return rect{w - s(6) - s(28), s(6), w - s(6), s(6) + s(28)}
}

// onCopy tells whether p is on the copy button.
func (v *logView) onCopy(p point) bool {
	r := v.copyRect()
	return v.copyText != nil && p.X >= r.Left && p.X < r.Right && p.Y >= r.Top && p.Y < r.Bottom
}

// copyAll copies what the copy button gives, and shows a check mark for a moment.
func (v *logView) copyAll() {
	if setClipboard(v.hwnd, v.copyText()) {
		v.copied = time.Now()
		pSetTimer.Call(v.hwnd, 1, 1600, 0)
		pInvalidateRect.Call(v.hwnd, 0, 0)
	}
}

// updateCopyTip places the tooltip of the copy button, and gives it its text.
func (v *logView) updateCopyTip() {
	if v.copyText == nil {
		return
	}
	if v.tip == 0 {
		v.tip = g.newTooltip(v.hwnd)
		addTool(v.tip, v.hwnd, 1, &rect{}, v.copyTip())
	}
	ti := toolInfo{Hwnd: v.hwnd, ID: 1, Rect: v.copyRect(), Text: u16(v.copyTip())}
	ti.Size = uint32(unsafe.Sizeof(ti))
	send(v.tip, ttmNewToolRectW, 0, uintptr(unsafe.Pointer(&ti)))
	send(v.tip, ttmUpdateTipTextW, 0, uintptr(unsafe.Pointer(&ti)))
}

// consoleBg is the background of the console: the black of the details page's
// commands, and of Windows Terminal.
const consoleBg = docCmdBg

func (v *logView) padX() int32 {
	if v.console {
		return g.scale(4)
	}
	return g.scale(6)
}

func (v *logView) padY() int32 { return g.scale(4) }

// gutter is the width of the left column of marks, if any.
func (v *logView) gutter() int32 {
	if v.marks {
		return g.scale(18)
	}
	return 0
}

// setMark shows mark in color in the left column of a line: ✓ or ✗ once its command
// has ended, … while it runs, nothing.
func (v *logView) setMark(line int, mark string, color uint32) {
	if line >= 0 && line < len(v.lines) {
		v.lines[line].mark, v.lines[line].markColor = mark, color
		pInvalidateRect.Call(v.hwnd, 0, 0)
	}
}

// logViews are the logs shown, by window: the main window's, and the console of the
// Fix window.
var logViews = map[uintptr]*logView{}

// newLogView makes a log as a child of parent.
func (a *ui) newLogView(parent uintptr, id int) *logView {
	h, _, _ := pCreateWindowExW.Call(wsExClientEdge, up("eMudScanLog"), 0,
		uintptr(wsChild|wsVisible|wsVScroll|wsTabStop), 0, 0, 10, 10, parent, uintptr(id), 0, 0)
	v := &logView{hwnd: h}
	logViews[h] = v
	v.setFonts(a.dpi)
	return v
}

// newConsoleView makes the console of the Fix window, as a child of parent: it is
// not a tab stop (a click selects its text, the wheel scrolls it).
func (a *ui) newConsoleView(parent uintptr, id int) *logView {
	h, _, _ := pCreateWindowExW.Call(0, up("eMudScanLog"), 0,
		uintptr(wsChild|wsVisible|wsVScroll), 0, 0, 10, 10, parent, uintptr(id), 0, 0)
	v := &logView{hwnd: h, console: true, marks: true}
	v.bg, _, _ = pCreateSolidBrush.Call(consoleBg)
	logViews[h] = v
	pSetWindowTheme.Call(h, up("DarkMode_Explorer"), 0) // a dark scroll bar, where Windows has one
	v.setFonts(a.dpi)
	return v
}

// destroy removes a log made for a window that closes.
func (v *logView) destroy() {
	delete(logViews, v.hwnd)
	pDestroyWindow.Call(v.hwnd)
	v.destroyFonts()
	if v.bg != 0 {
		pDeleteObject.Call(v.bg)
		v.bg = 0
	}
}

func (v *logView) destroyFonts() {
	for i, f := range v.fonts {
		if f != 0 {
			pDeleteObject.Call(f)
			v.fonts[i] = 0
		}
	}
	if v.markFont != 0 {
		pDeleteObject.Call(v.markFont)
		v.markFont = 0
	}
}

// glyphMetrics is GLYPHMETRICS: the box of a glyph's ink, from its origin on the
// baseline.
type glyphMetrics struct {
	BlackBoxX, BlackBoxY uint32
	Origin               point
	CellIncX, CellIncY   int16
}

// inkBox is the height of the ink of the character c in the font selected into dc,
// and how far above the baseline it starts; false if Windows cannot say.
func inkBox(dc uintptr, c rune) (height, top int32, ok bool) {
	var gm glyphMetrics
	mat := [4]uint32{1 << 16, 0, 0, 1 << 16} // MAT2: the identity, in 16.16 fixed point
	r, _, _ := pGetGlyphOutlineW.Call(dc, uintptr(c), 0 /* GGO_METRICS */, uintptr(unsafe.Pointer(&gm)), 0, 0, uintptr(unsafe.Pointer(&mat)))
	if r == 0xFFFFFFFF || gm.BlackBoxY == 0 {
		return 0, 0, false
	}
	return int32(gm.BlackBoxY), gm.Origin.Y, true
}

// markTop is where a mark is drawn in its row, from the top of the row: its ink
// centered on the capital letters of the text, so that it is neither higher nor lower
// than the command beside it.
func (v *logView) markTop(dc uintptr, mark string) int32 {
	font, ascent := v.markFont, v.markAscent
	if y, ok := v.markTops[mark]; ok {
		return y
	}
	y := v.ascent - ascent // on the baseline of the text, if Windows cannot say
	pSelectObject.Call(dc, v.fonts[logMono])
	capH, _, ok1 := inkBox(dc, 'H')
	pSelectObject.Call(dc, font)
	h, top, ok2 := inkBox(dc, []rune(mark)[0])
	if ok1 && ok2 {
		center := v.ascent - capH/2 // below the top of the row
		y = center - h/2 + top - ascent
	}
	if v.markTops == nil {
		v.markTops = map[string]int32{}
	}
	v.markTops[mark] = y
	return y
}

// setFonts makes the fonts for a DPI; every row is as high as the tallest of them.
func (v *logView) setFonts(dpi int) {
	v.destroyFonts()
	var desc int32
	v.ascent = 0
	for i := range v.fonts {
		weight := 400
		if i >= logBold {
			weight = 700
		}
		v.fonts[i] = createFont(9, dpi, weight, false, logFaces[i%logBold])
		tm := g.fontMetrics(v.fonts[i])
		v.ascents[i] = tm.Ascent
		v.ascent = max(v.ascent, tm.Ascent)
		desc = max(desc, tm.Height-tm.Ascent)
	}
	v.pitch = v.ascent + desc
	if v.marks { // ✔ and ✘, heavier and one size up, to be seen at a glance
		v.markFont = createFont(10, dpi, 700, false, "Segoe UI Symbol")
		v.markAscent = g.fontMetrics(v.markFont).Ascent
	}
	v.markTops = nil
	v.width = -1 // cut again
	v.relayout()
}

// clear empties the log.
func (v *logView) clear() {
	v.text, v.lines, v.rows = nil, nil, nil
	v.top, v.selA, v.selB = 0, 0, 0
	v.update()
}

// append adds runs at the end. The log follows its end if it was showing it (or
// when follow is set).
func (v *logView) append(runs []logRun, follow bool) {
	follow = follow || v.atBottom()
	changed := len(v.lines) // the first line to cut again: the last one if it is not ended
	if changed > 0 && !v.lines[changed-1].done {
		changed--
	}
	for _, r := range runs {
		font := logMono
		if r.sym {
			font = logSymbol
			if r.text == statusMark(StWarn) || r.text == statusMark(StError) {
				font = logEmoji
			}
		}
		if r.bold {
			font += logBold
		}
		s := utf16.Encode([]rune(r.text))
		for len(s) > 0 {
			if len(v.lines) == 0 || v.lines[len(v.lines)-1].done {
				v.lines = append(v.lines, logLine{off: len(v.text)})
			}
			ln := &v.lines[len(v.lines)-1]
			n, brk := len(s), false
			for i, c := range s {
				if c == '\n' {
					n, brk = i, true
					break
				}
			}
			piece := s[:n]
			if len(piece) > 0 && piece[len(piece)-1] == '\r' {
				piece = piece[:len(piece)-1]
			}
			if len(piece) > 0 {
				v.text = append(v.text, piece...)
				if k := len(ln.spans) - 1; k >= 0 && ln.spans[k].font == font && ln.spans[k].color == r.color {
					ln.spans[k].n += len(piece)
				} else {
					ln.spans = append(ln.spans, logSpan{len(piece), font, r.color})
				}
			}
			if brk {
				ln.done = true
				v.text = append(v.text, '\r', '\n')
				n++
			}
			s = s[n:]
		}
	}
	v.layoutFrom(changed)
	if follow {
		v.top = len(v.rows)
	}
	v.update()
}

// atBottom tells whether the last row is shown.
func (v *logView) atBottom() bool { return v.top+v.visibleRows() >= len(v.rows) }

func (v *logView) clientSize() (int32, int32) {
	var rc rect
	pGetClientRect.Call(v.hwnd, uintptr(unsafe.Pointer(&rc)))
	return rc.Right, rc.Bottom
}

// visibleRows is the number of whole rows the log can show.
func (v *logView) visibleRows() int {
	_, h := v.clientSize()
	if v.pitch <= 0 {
		return 1
	}
	return max(int((h-2*v.padY())/v.pitch), 1)
}

// relayout cuts every line again when the width changed.
func (v *logView) relayout() {
	w, _ := v.clientSize()
	w -= 2*v.padX() + v.gutter()
	if v.copyText != nil { // the lines end before the copy button
		w -= g.scale(6) + g.scale(28)
	}
	if w == v.width {
		return
	}
	v.width = w
	v.layoutFrom(0)
}

// layoutFrom cuts the lines from the given one to the width of the log.
func (v *logView) layoutFrom(from int) {
	if v.width < 0 {
		return
	}
	dc, _, _ := pCreateCompatibleDC.Call(0)
	for i := from; i < len(v.lines); i++ {
		v.cut(dc, &v.lines[i])
	}
	pDeleteDC.Call(dc)
	v.rows = v.rows[:0]
	for i := range v.lines {
		for j := range v.lines[i].rows {
			v.rows = append(v.rows, logRowRef{i, j})
		}
	}
}

func (v *logView) measure(dc uintptr, font int, s []uint16) int32 {
	if len(s) == 0 {
		return 0
	}
	pSelectObject.Call(dc, v.fonts[font])
	var sz point
	pGetTextExtentPoint32.Call(dc, uintptr(unsafe.Pointer(&s[0])), uintptr(len(s)), uintptr(unsafe.Pointer(&sz)))
	return sz.X
}

// fit returns how many characters of s fit in width.
func (v *logView) fit(dc uintptr, font int, s []uint16, width int32) int {
	if len(s) == 0 || width <= 0 {
		return 0
	}
	pSelectObject.Call(dc, v.fonts[font])
	var n int32
	var sz point
	pGetTextExtentExPoint.Call(dc, uintptr(unsafe.Pointer(&s[0])), uintptr(len(s)), uintptr(width),
		uintptr(unsafe.Pointer(&n)), 0, uintptr(unsafe.Pointer(&sz)))
	return int(n)
}

// cut lays a line out in rows: words (with their spaces) are kept whole, even in
// several colors (a command), unless one is wider than the log by itself.
func (v *logView) cut(dc uintptr, ln *logLine) {
	ln.rows = ln.rows[:0]
	var row []logFrag
	x := int32(0)
	add := func(off, n, font int, color uint32) {
		w := v.measure(dc, font, v.text[off:off+n])
		if k := len(row) - 1; k >= 0 && row[k].font == font && row[k].color == color && row[k].off+row[k].n == off {
			row[k].n += n
			row[k].w = v.measure(dc, font, v.text[row[k].off:off+n])
			x = row[k].x + row[k].w
			return
		}
		row = append(row, logFrag{off, n, font, color, x, w})
		x += w
	}
	newRow := func() {
		ln.rows = append(ln.rows, row)
		row, x = nil, 0
	}
	// the spans, from where they start in the text
	type seg struct {
		a, b, font int
		color      uint32
	}
	var segs []seg
	end := ln.off
	for _, sp := range ln.spans {
		segs = append(segs, seg{end, end + sp.n, sp.font, sp.color})
		end += sp.n
	}
	pieces := func(a, b int, fn func(a, b int, s seg)) { // what of a…b each span holds
		for _, s := range segs {
			if lo, hi := max(a, s.a), min(b, s.b); lo < hi {
				fn(lo, hi, s)
			}
		}
	}
	for i := ln.off; i < end; {
		j := i
		for j < end && v.text[j] != ' ' {
			j++
		}
		word := j // without its spaces
		for j < end && v.text[j] == ' ' {
			j++
		}
		ww := int32(0)
		pieces(i, word, func(a, b int, s seg) { ww += v.measure(dc, s.font, v.text[a:b]) })
		if x > 0 && x+ww > v.width {
			newRow()
		}
		if x == 0 && ww > v.width { // too wide by itself: cut anywhere
			pieces(i, word, func(a, b int, s seg) {
				for a < b {
					n := v.fit(dc, s.font, v.text[a:b], v.width-x)
					if n == 0 && x > 0 {
						newRow()
						continue
					}
					n = max(n, 1)
					if n < b-a && n > 1 && v.text[a+n]&0xFC00 == 0xDC00 { // not inside a surrogate pair
						n--
					}
					add(a, n, s.font, s.color)
					if a += n; a < b {
						newRow()
					}
				}
			})
			i = word
		}
		pieces(i, j, func(a, b int, s seg) { add(a, b-a, s.font, s.color) })
		i = j
	}
	ln.rows = append(ln.rows, row)
}

// update keeps the first row in range, then updates the scroll bar and the drawing.
func (v *logView) update() {
	vis := v.visibleRows()
	v.top = max(0, min(v.top, len(v.rows)-vis))
	si := scrollInfo{Mask: 0x1 | 0x2 | 0x4 | 0x8 /* RANGE|PAGE|POS|DISABLENOSCROLL */, Min: 0, Max: int32(max(len(v.rows)-1, 0)), Page: uint32(vis), Pos: int32(v.top)}
	si.Size = uint32(unsafe.Sizeof(si))
	pSetScrollInfo.Call(v.hwnd, 1 /* SB_VERT */, uintptr(unsafe.Pointer(&si)), 1)
	pInvalidateRect.Call(v.hwnd, 0, 0)
}

func (v *logView) scrollTo(top int) {
	if top != v.top {
		v.top = top
		v.update()
	}
}

// nextLine is the number of the line that the next text appended starts.
func (v *logView) nextLine() int {
	n := len(v.lines)
	if n > 0 && !v.lines[n-1].done {
		n--
	}
	return n
}

// scrollToLine shows a line at the top, or as near as the end of the log allows.
func (v *logView) scrollToLine(line int) {
	for i, r := range v.rows {
		if r.line == line {
			v.scrollTo(i)
			return
		}
	}
}

// ---------------------------------------------------------------------------
// Selection

func (v *logView) sel() (int, int) {
	if v.selA <= v.selB {
		return v.selA, v.selB
	}
	return v.selB, v.selA
}

func (v *logView) selectedText() string {
	s, e := v.sel()
	if s == e || e > len(v.text) {
		return ""
	}
	return string(utf16.Decode(v.text[s:e]))
}

// offsetAt is the position in the text nearest to a point of the window.
func (v *logView) offsetAt(pt point) int {
	if len(v.rows) == 0 {
		return 0
	}
	y := pt.Y - v.padY()
	r := v.top
	if y >= 0 {
		r += int(y / v.pitch)
	} else {
		r--
	}
	r = max(0, min(r, len(v.rows)-1))
	ref := v.rows[r]
	ln := &v.lines[ref.line]
	row := ln.rows[ref.row]
	if len(row) == 0 {
		return ln.off
	}
	x := pt.X - v.padX() - v.gutter()
	if x <= row[0].x {
		return row[0].off
	}
	dc, _, _ := pCreateCompatibleDC.Call(0)
	defer pDeleteDC.Call(dc)
	for _, f := range row {
		if x < f.x+f.w {
			s := v.text[f.off : f.off+f.n]
			dx := x - f.x
			n := v.fit(dc, f.font, s, dx)
			if n < len(s) && dx-v.measure(dc, f.font, s[:n]) > v.measure(dc, f.font, s[:n+1])-dx {
				n++
			}
			return f.off + n
		}
	}
	last := row[len(row)-1]
	end := last.off + last.n
	if ref.row < len(ln.rows)-1 { // a cut row: before the spaces that end it
		for end > last.off && v.text[end-1] == ' ' {
			end--
		}
	}
	return end
}

// selectWord selects the run of non-space characters around an offset.
func (v *logView) selectWord(off int) {
	isSpace := func(c uint16) bool { return c == ' ' || c == '\t' || c == '\r' || c == '\n' }
	s, e := off, off
	for s > 0 && !isSpace(v.text[s-1]) {
		s--
	}
	for e < len(v.text) && !isSpace(v.text[e]) {
		e++
	}
	v.selA, v.selB = s, e
}

func (v *logView) copySelection() {
	if t := v.selectedText(); t != "" {
		setClipboard(v.hwnd, t)
	}
}

// ---------------------------------------------------------------------------
// Drawing

func (v *logView) paint() {
	a := g
	var ps paintStruct
	hdc, _, _ := pBeginPaint.Call(v.hwnd, uintptr(unsafe.Pointer(&ps)))
	defer pEndPaint.Call(v.hwnd, uintptr(unsafe.Pointer(&ps)))
	w, h := v.clientSize()
	if w <= 0 || h <= 0 {
		return
	}
	dc, _, _ := pCreateCompatibleDC.Call(hdc)
	bmp, _, _ := pCreateCompatibleBmp.Call(hdc, uintptr(w), uintptr(h))
	oldBmp, _, _ := pSelectObject.Call(dc, bmp)
	oldFont, _, _ := pSelectObject.Call(dc, v.fonts[0])
	rc := rect{0, 0, w, h}
	bg := a.brushWindow
	if v.console {
		bg = v.bg
	}
	pFillRect.Call(dc, uintptr(unsafe.Pointer(&rc)), bg)
	pSetBkMode.Call(dc, 1 /* TRANSPARENT */)
	selS, selE := v.sel()
	focused, _, _ := pGetFocus.Call()
	hlColor, hlText := uint32(13), uint32(14) // COLOR_HIGHLIGHT, COLOR_HIGHLIGHTTEXT
	if focused != v.hwnd {
		hlColor, hlText = 15, 18 // COLOR_BTNFACE, COLOR_BTNTEXT: selection of an inactive control
	}
	hl, _, _ := pGetSysColorBrush.Call(uintptr(hlColor))
	px, py := v.padX()+v.gutter(), v.padY()
	for i := 0; i < v.visibleRows() && v.top+i < len(v.rows); i++ {
		ref := v.rows[v.top+i]
		y := py + int32(i)*v.pitch
		if ln := &v.lines[ref.line]; ref.row == 0 && ln.mark != "" { // the result of its command
			font, top := v.fonts[logMono], v.ascent-v.ascents[logMono] // …: on the baseline of the text
			if ln.mark != "…" && v.markFont != 0 {
				font, top = v.markFont, v.markTop(dc, ln.mark)
			}
			s := utf16.Encode([]rune(ln.mark))
			pSelectObject.Call(dc, font)
			pSetTextColor.Call(dc, uintptr(ln.markColor))
			pExtTextOutW.Call(dc, uintptr(v.padX()), uintptr(y+top), 0, 0, uintptr(unsafe.Pointer(&s[0])), uintptr(len(s)), 0)
		}
		for _, f := range v.lines[ref.line].rows[ref.row] {
			if f.n == 0 {
				continue
			}
			s := v.text[f.off : f.off+f.n]
			ty := y + v.ascent - v.ascents[f.font] // the same baseline for all fonts
			pSelectObject.Call(dc, v.fonts[f.font])
			pSetTextColor.Call(dc, uintptr(f.color))
			pExtTextOutW.Call(dc, uintptr(px+f.x), uintptr(ty), 0, 0, uintptr(unsafe.Pointer(&s[0])), uintptr(len(s)), 0)
			if selE > f.off && selS < f.off+f.n {
				i0, i1 := max(selS-f.off, 0), min(selE-f.off, f.n)
				xa, xb := px+f.x+v.measure(dc, f.font, s[:i0]), px+f.x+v.measure(dc, f.font, s[:i1])
				r := rect{xa, y, xb, y + v.pitch}
				pFillRect.Call(dc, uintptr(unsafe.Pointer(&r)), hl)
				pSetTextColor.Call(dc, uintptr(sysColor(int(hlText))))
				pExtTextOutW.Call(dc, uintptr(xa), uintptr(ty), 0, 0, uintptr(unsafe.Pointer(&s[i0])), uintptr(i1-i0), 0)
			}
		}
	}
	if v.copyText != nil {
		drawCopyIcon(dc, v.copyRect(), v.copyHover, time.Since(v.copied) < 1500*time.Millisecond)
	}
	pBitBlt.Call(hdc, 0, 0, uintptr(w), uintptr(h), dc, 0, 0, 0x00CC0020 /* SRCCOPY */)
	pSelectObject.Call(dc, oldFont)
	pSelectObject.Call(dc, oldBmp)
	pDeleteObject.Call(bmp)
	pDeleteDC.Call(dc)
}

// paintBorder draws the border as a themed edit box, as the rich edit control did.
func (v *logView) paintBorder() {
	theme, _, _ := pOpenThemeData.Call(v.hwnd, up("EDIT"))
	if theme == 0 {
		return
	}
	defer pCloseThemeData.Call(theme)
	var wr rect
	pGetWindowRect.Call(v.hwnd, uintptr(unsafe.Pointer(&wr)))
	org := point{}
	pClientToScreen.Call(v.hwnd, uintptr(unsafe.Pointer(&org)))
	bx, by := org.X-wr.Left, org.Y-wr.Top // border width and height
	w, h := wr.Right-wr.Left, wr.Bottom-wr.Top
	dc, _, _ := pGetWindowDC.Call(v.hwnd)
	pExcludeClipRect.Call(dc, uintptr(bx), uintptr(by), uintptr(w-bx), uintptr(h-by))
	r := rect{0, 0, w, h}
	pDrawThemeBackground.Call(theme, dc, 6 /* EP_EDITBORDER_NOSCROLL */, 1 /* EPSN_NORMAL */, uintptr(unsafe.Pointer(&r)), 0)
	pReleaseDC.Call(v.hwnd, dc)
}

// ---------------------------------------------------------------------------
// Window procedure

func logWndProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	a := g
	v := logViews[hwnd]
	if v == nil {
		r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
		return r
	}
	switch msg {
	case wmEraseBkgnd:
		return 1
	case wmPaint:
		v.paint()
		return 0
	case wmNcPaint:
		r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
		if !v.console {
			v.paintBorder()
		}
		return r
	case wmSize:
		follow := v.atBottom()
		v.relayout()
		v.updateCopyTip()
		if follow {
			v.top = len(v.rows)
		}
		v.update()
		return 0
	case wmVScroll:
		page := max(v.visibleRows()-1, 1)
		switch loword(wParam) {
		case 0: // SB_LINEUP
			v.scrollTo(v.top - 1)
		case 1: // SB_LINEDOWN
			v.scrollTo(v.top + 1)
		case 2: // SB_PAGEUP
			v.scrollTo(v.top - page)
		case 3: // SB_PAGEDOWN
			v.scrollTo(v.top + page)
		case 4, 5: // SB_THUMBPOSITION, SB_THUMBTRACK
			si := scrollInfo{Mask: 0x10 /* SIF_TRACKPOS */}
			si.Size = uint32(unsafe.Sizeof(si))
			pGetScrollInfo.Call(hwnd, 1, uintptr(unsafe.Pointer(&si)))
			v.scrollTo(int(si.TrackPos))
		case 6: // SB_TOP
			v.scrollTo(0)
		case 7: // SB_BOTTOM
			v.scrollTo(len(v.rows))
		}
		return 0
	case wmMouseWheel: // 3 rows a notch
		v.wheel += int32(int16(hiword(wParam)))
		if n := v.wheel / 40; n != 0 {
			v.wheel -= n * 40
			v.scrollTo(v.top - int(n))
		}
		return 0
	case wmGetDlgCode:
		return 0x1 // DLGC_WANTARROWS
	case wmKeyDown:
		page := max(v.visibleRows()-1, 1)
		switch ctrl := ctrlDown(); {
		case ctrl && wParam == 'C':
			v.copySelection()
		case ctrl && wParam == 'A':
			v.selA, v.selB = 0, len(v.text)
			pInvalidateRect.Call(hwnd, 0, 0)
		case wParam == 0x26: // up
			v.scrollTo(v.top - 1)
		case wParam == 0x28: // down
			v.scrollTo(v.top + 1)
		case wParam == 0x21: // page up
			v.scrollTo(v.top - page)
		case wParam == 0x22: // page down
			v.scrollTo(v.top + page)
		case wParam == 0x24: // home
			v.scrollTo(0)
		case wParam == 0x23: // end
			v.scrollTo(len(v.rows))
		}
		return 0
	case wmSetCursor:
		if loword(lParam) == 1 { // HTCLIENT
			if v.onCopy(cursorIn(hwnd)) {
				pSetCursor.Call(a.curHand)
				return 1
			}
			c, _, _ := pLoadCursorW.Call(0, 32513 /* IDC_IBEAM */)
			pSetCursor.Call(c)
			return 1
		}
	case wmTimer: // the check mark of the copy button has been shown long enough
		pKillTimer.Call(hwnd, wParam)
		pInvalidateRect.Call(hwnd, 0, 0)
		return 0
	case wmMouseLeave:
		if v.copyHover {
			v.copyHover = false
			pInvalidateRect.Call(hwnd, 0, 0)
		}
		return 0
	case wmLButtonDown:
		pt := point{xParam(lParam), yParam(lParam)}
		if v.onCopy(pt) {
			v.copyDown = true
			pSetCapture.Call(hwnd)
			return 0
		}
		pSetFocus.Call(hwnd)
		v.down, v.moved, v.downPt = true, false, pt
		off := v.offsetAt(pt)
		if wParam&0x4 /* MK_SHIFT */ != 0 {
			v.selB = off
		} else {
			v.selA, v.selB = off, off
		}
		pSetCapture.Call(hwnd)
		pInvalidateRect.Call(hwnd, 0, 0)
		return 0
	case wmMouseMove:
		if v.copyText != nil && !v.down {
			if hover := v.onCopy(point{xParam(lParam), yParam(lParam)}); hover != v.copyHover {
				v.copyHover = hover
				pInvalidateRect.Call(hwnd, 0, 0)
				tme := struct {
					Size, Flags uint32
					Hwnd        uintptr
					Hover       uint32
				}{Flags: 0x2 /* TME_LEAVE */, Hwnd: hwnd}
				tme.Size = uint32(unsafe.Sizeof(tme))
				pTrackMouseEvent.Call(uintptr(unsafe.Pointer(&tme)))
			}
		}
		if v.down {
			pt := point{xParam(lParam), yParam(lParam)}
			if abs32(pt.X-v.downPt.X)+abs32(pt.Y-v.downPt.Y) > a.scale(4) {
				v.moved = true
			}
			if v.moved {
				_, h := v.clientSize()
				if pt.Y < 0 {
					v.scrollTo(v.top - 1)
				} else if pt.Y > h {
					v.scrollTo(v.top + 1)
				}
				v.selB = v.offsetAt(pt)
				pInvalidateRect.Call(hwnd, 0, 0)
			}
		}
		return 0
	case wmLButtonUp:
		if v.copyDown {
			v.copyDown = false
			pReleaseCapture.Call()
			if v.onCopy(point{xParam(lParam), yParam(lParam)}) {
				v.copyAll()
			}
			return 0
		}
		if v.down {
			v.down = false
			pReleaseCapture.Call()
		}
		return 0
	case wmCaptureChanged:
		v.down, v.copyDown = false, false
		return 0
	case wmLButtonDblClk:
		if v.onCopy(point{xParam(lParam), yParam(lParam)}) {
			return 0
		}
		if len(v.text) > 0 {
			v.selectWord(min(v.offsetAt(point{xParam(lParam), yParam(lParam)}), len(v.text)-1))
			pInvalidateRect.Call(hwnd, 0, 0)
		}
		return 0
	case wmContextMenu:
		pt := point{xParam(lParam), yParam(lParam)}
		if int32(lParam) == -1 { // from the keyboard
			pt = point{a.scale(40), a.scale(20)}
			pClientToScreen.Call(hwnd, uintptr(unsafe.Pointer(&pt)))
		}
		menu, _, _ := pCreatePopupMenu.Call()
		gray := uintptr(0)
		if v.selectedText() == "" {
			gray = 1 // MF_GRAYED
		}
		pAppendMenuW.Call(menu, gray, 1, up(T("dlg.menuCopy")))
		pAppendMenuW.Call(menu, 0x800, 0, 0) // separator
		pAppendMenuW.Call(menu, 0, 2, up(T("dlg.menuSelAll")))
		pAppendMenuW.Call(menu, 0, 3, up(T("dlg.menuCopyAll")))
		id, _, _ := pTrackPopupMenu.Call(menu, 0x100|0x2 /* RETURNCMD|RIGHTBUTTON */, uintptr(pt.X), uintptr(pt.Y), 0, hwnd, 0)
		pDestroyMenu.Call(menu)
		switch id {
		case 1:
			v.copySelection()
		case 2:
			v.selA, v.selB = 0, len(v.text)
			pInvalidateRect.Call(hwnd, 0, 0)
		case 3:
			setClipboard(hwnd, string(utf16.Decode(v.text)))
		}
		return 0
	case wmSetFocus, wmKillFocus:
		pInvalidateRect.Call(hwnd, 0, 0)
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
	return r
}
