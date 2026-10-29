//go:build windows

package main

// The document view of the details window, drawn with GDI: formatted text,
// label/value fields in two aligned columns, links, command blocks (light text on a
// black background, with a copy button in their top right corner), mouse selection
// and scrolling.

import (
	"strings"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

type docFont struct {
	h              uintptr
	height, ascent int32
}

type docFrag struct {
	x, y, w, h int32 // y and h: the line the text is on
	ty         int32 // top of the text
	text       []uint16
	font       *docFont
	color      uint32
	link       string
	off        int // offset of text in the document text
}

type docBtn struct {
	rc     rect
	cmds   []string
	copied time.Time
}

type docIcon struct {
	x, y   int32
	status Status
}

// styled is a piece of text with its look, before layout.
type styled struct {
	text  string
	font  *docFont
	color uint32
	link  string
}

type docView struct {
	hwnd      uintptr
	blocks    []docBlock
	fonts     map[string]*docFont
	icons     map[Status]uintptr
	frags     []docFrag
	blockRcs  []rect
	btns      []*docBtn
	marks     []docIcon
	text      []uint16
	height    int32
	scroll    int32
	selA      int
	selB      int
	down      bool
	moved     bool
	downPt    point
	downLink  string
	downBtn   int
	hoverBtn  int
	tip       uintptr
	tips      int // tools of the copy buttons, 1 to tips
	tipFrag   int // the fragment of the link whose tooltip is placed (-1: none)
	onStatus  func(string)
	onCopyAll func()
}

// linkToolID is the tool of the link under the mouse (the copy buttons' are 1, 2…).
const linkToolID = 0x10000

const (
	docCmdBg   = 0x0C0C0C // rgb(12,12,12)
	docCmdText = 0xCCCCCC // rgb(204,204,204)
)

var (
	docViews   = map[uintptr]*docView{}
	docProc    = syscall.NewCallback(docWndProc)
	docClassOK bool
)

func (a *ui) newDocView(parent uintptr, blocks []docBlock) *docView {
	if !docClassOK {
		inst, _, _ := pGetModuleHandleW.Call(0)
		wc := wndClassEx{Style: 0x8 /* CS_DBLCLKS */, WndProc: docProc, Instance: inst, Cursor: a.curArrow, ClassName: u16("eMudScanDoc")}
		wc.Size = uint32(unsafe.Sizeof(wc))
		pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
		docClassOK = true
	}
	v := &docView{blocks: blocks, downBtn: -1, hoverBtn: -1, tipFrag: -1}
	v.fonts = map[string]*docFont{}
	mk := func(name string, pt, weight int, underline bool, face string) {
		f := &docFont{h: createFont(pt, a.dpi, weight, underline, face)}
		dc, _, _ := pCreateCompatibleDC.Call(0)
		old, _, _ := pSelectObject.Call(dc, f.h)
		var tm textMetric
		pGetTextMetricsW.Call(dc, uintptr(unsafe.Pointer(&tm)))
		pSelectObject.Call(dc, old)
		pDeleteDC.Call(dc)
		f.height, f.ascent = tm.Height, tm.Ascent
		v.fonts[name] = f
	}
	mk("body", 10, 400, false, "Segoe UI")
	mk("bold", 10, 600, false, "Segoe UI")
	mk("link", 10, 400, true, "Segoe UI")
	mk("title", 17, 600, false, "Segoe UI")
	mk("status", 11, 700, false, "Segoe UI")
	mk("summary", 11, 400, false, "Segoe UI")
	mk("mono", 10, 400, false, "Consolas")
	mk("symbol", 10, 400, false, "Segoe UI Symbol") // ⚠, as in the log
	v.icons = map[Status]uintptr{}
	isz := int(a.scale(20))
	for _, st := range []Status{StOK, StInfo, StCaution, StWarn, StError} {
		v.icons[st] = iconFromRGBA(renderStatus(st, isz), isz)
	}
	h, _, _ := pCreateWindowExW.Call(0, up("eMudScanDoc"), 0, uintptr(wsChild|wsVisible|wsVScroll|wsTabStop),
		0, 0, 10, 10, parent, uintptr(idDlgRich), 0, 0)
	v.hwnd = h
	docViews[h] = v
	v.tip = a.newTooltip(h)
	if v.tip != 0 { // the link under the mouse: placed by linkTipAt
		ti := toolInfo{Flags: 0x10 /* TTF_SUBCLASS */, Hwnd: h, ID: linkToolID, Text: u16(" ")}
		ti.Size = uint32(unsafe.Sizeof(ti))
		send(v.tip, ttmAddToolW, 0, uintptr(unsafe.Pointer(&ti)))
	}
	return v
}

func (v *docView) destroy() {
	for _, f := range v.fonts {
		pDeleteObject.Call(f.h)
	}
	for _, ic := range v.icons {
		pDestroyIcon.Call(ic)
	}
	if v.tip != 0 {
		pDestroyWindow.Call(v.tip)
	}
	delete(docViews, v.hwnd)
}

// ---------------------------------------------------------------------------
// Layout

type docLayouter struct {
	v  *docView
	dc uintptr
}

func (L *docLayouter) measure(f *docFont, s []uint16) int32 {
	if len(s) == 0 {
		return 0
	}
	pSelectObject.Call(L.dc, f.h)
	var sz point
	pGetTextExtentPoint32.Call(L.dc, uintptr(unsafe.Pointer(&s[0])), uintptr(len(s)), uintptr(unsafe.Pointer(&sz)))
	return sz.X
}

// fit returns how many characters of s fit in width.
func (L *docLayouter) fit(f *docFont, s []uint16, width int32) int {
	if len(s) == 0 || width <= 0 {
		return 0
	}
	pSelectObject.Call(L.dc, f.h)
	var n int32
	var sz point
	pGetTextExtentExPoint.Call(L.dc, uintptr(unsafe.Pointer(&s[0])), uintptr(len(s)), uintptr(width),
		uintptr(unsafe.Pointer(&n)), 0, uintptr(unsafe.Pointer(&sz)))
	return int(n)
}

// splitWords cuts text after each run of spaces, keeping the spaces with the word before.
func splitWords(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ' ' && (i+1 == len(s) || s[i+1] != ' ') {
			out = append(out, s[start:i+1])
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

// flow lays out styled text between xFirst (first line) or xIndent (next lines) and xMax,
// from y; it returns the y below the last line.
func (L *docLayouter) flow(parts []styled, xFirst, xIndent, xMax, y int32, spacing float64) int32 {
	v := L.v
	x := xFirst
	var line []docFrag
	commit := func() {
		lh, base := int32(0), int32(0)
		for _, f := range line {
			lh, base = max(lh, f.font.height), max(base, f.font.ascent)
		}
		if len(line) == 0 {
			lh, base = v.fonts["body"].height, v.fonts["body"].ascent
		}
		lineH := int32(float64(lh) * spacing)
		for _, f := range line {
			f.y, f.h = y, lineH
			f.ty = y + (lineH-lh)/2 + base - f.font.ascent
			f.off = len(v.text)
			v.text = append(v.text, f.text...)
			v.frags = append(v.frags, f)
		}
		y += lineH
		line = nil
		x = xIndent
	}
	add := func(p styled, u []uint16, w int32) {
		if n := len(line); n > 0 {
			l := &line[n-1]
			if l.font == p.font && l.color == p.color && l.link == p.link && l.x+l.w == x {
				l.text = append(l.text, u...)
				l.w += w
				x += w
				return
			}
		}
		line = append(line, docFrag{x: x, w: w, text: append([]uint16{}, u...), font: p.font, color: p.color, link: p.link})
		x += w
	}
	for _, p := range parts {
		for _, word := range splitWords(p.text) {
			u := utf16.Encode([]rune(word))
			w := L.measure(p.font, u)
			wt := L.measure(p.font, utf16.Encode([]rune(strings.TrimRight(word, " "))))
			if x+wt > xMax && len(line) > 0 {
				commit()
			}
			if x+wt <= xMax {
				add(p, u, w)
				continue
			}
			// longer than a whole line: cut it
			for len(u) > 0 {
				n := L.fit(p.font, u, xMax-x)
				if n == 0 {
					if len(line) > 0 {
						commit()
						continue
					}
					n = 1
				}
				add(p, u[:n], L.measure(p.font, u[:n]))
				u = u[n:]
				if len(u) > 0 {
					commit()
				}
			}
		}
	}
	commit()
	return y
}

func (L *docLayouter) newline() { L.v.text = append(L.v.text, '\r', '\n') }

// runsStyled converts markup runs with a base font and color.
func (v *docView) runsStyled(runs []mrun, base *docFont, color uint32) []styled {
	var out []styled
	for _, r := range runs {
		switch r.kind {
		case runBold:
			out = append(out, styled{r.text, v.fonts["bold"], color, ""})
		case runLink:
			out = append(out, styled{r.text, v.fonts["link"], rgb(0, 102, 204), r.target})
		case runWarn: // amber, as in the lists, with its ⚠ from the symbol font
			text := r.text
			if rest, ok := strings.CutPrefix(text, "⚠"); ok {
				out = append(out, styled{"⚠", v.fonts["symbol"], optinColor, ""})
				text = rest
			}
			out = append(out, styled{text, base, optinColor, ""})
		default:
			out = append(out, styled{r.text, base, color, ""})
		}
	}
	return out
}

func (v *docView) layout() {
	a := g
	s := a.scale
	var rc rect
	pGetClientRect.Call(v.hwnd, uintptr(unsafe.Pointer(&rc)))
	dc, _, _ := pCreateCompatibleDC.Call(0)
	defer pDeleteDC.Call(dc)
	L := &docLayouter{v: v, dc: dc}
	v.frags, v.blockRcs, v.btns, v.marks, v.text = nil, nil, nil, nil, nil
	x0, xMax := s(24), rc.Right-s(24)
	if xMax < x0+s(240) {
		xMax = x0 + s(240)
	}
	text, gray := rgb(32, 32, 32), rgb(96, 96, 96)
	labelW := int32(0)
	for _, b := range v.blocks {
		for _, r := range b.rows {
			labelW = max(labelW, L.measure(v.fonts["bold"], utf16.Encode([]rune(r.label))))
		}
	}
	labelW = min(labelW, (xMax-x0)*2/5)
	gap := s(20)
	y := s(16)
	for _, b := range v.blocks {
		switch b.kind {
		case dkTitle:
			y = L.flow(v.runsStyled(b.runs, v.fonts["title"], rgb(16, 58, 120)), x0, x0, xMax, y, 1.1)
			L.newline()
		case dkStatus:
			isz := s(20)
			lh := int32(float64(v.fonts["status"].height) * 1.3)
			v.marks = append(v.marks, docIcon{x0, y + (lh-isz)/2, b.status})
			c := statusColor(b.status)
			parts := []styled{{statusText(b.status), v.fonts["status"], c, ""}, {"  —  " + oneLine(b.summary), v.fonts["summary"], c, ""}}
			y = L.flow(parts, x0+isz+s(8), x0+isz+s(8), xMax, y, 1.3) + s(10)
			L.newline()
		case dkFields:
			for _, r := range b.rows {
				yl := L.flow([]styled{{r.label, v.fonts["bold"], gray, ""}}, x0, x0, x0+labelW, y, 1.35)
				v.text = append(v.text, '\t')
				yv := L.flow(v.runsStyled(r.value, v.fonts["body"], text), x0+labelW+gap, x0+labelW+gap, xMax, y, 1.35)
				L.newline()
				y = max(yl, yv)
			}
		case dkPara:
			y = L.flow(v.runsStyled(b.runs, v.fonts["body"], text), x0, x0, xMax, y, 1.35) + s(2)
			L.newline()
		case dkBullet:
			ind := s(18)
			L.flow([]styled{{"•", v.fonts["body"], text, ""}}, x0+s(4), x0+s(4), x0+ind, y, 1.35)
			v.text = append(v.text, ' ')
			y = L.flow(v.runsStyled(b.runs, v.fonts["body"], text), x0+ind, x0+ind, xMax, y, 1.35) + s(2)
			L.newline()
		case dkSpace:
			y += s(12)
		case dkCmds:
			pad, bsz := s(12), s(28)
			top := y + s(2)
			y = top + pad
			tx := x0 + pad
			for _, c := range b.cmds {
				y = L.flow([]styled{{c, v.fonts["mono"], docCmdText, ""}}, tx, tx, xMax-pad-bsz-s(4), y, 1.3)
				L.newline()
			}
			y = max(y+pad, top+bsz+2*s(6))
			v.blockRcs = append(v.blockRcs, rect{x0, top, xMax, y})
			v.btns = append(v.btns, &docBtn{rc: rect{xMax - s(6) - bsz, top + s(6), xMax - s(6), top + s(6) + bsz}, cmds: b.cmds})
			y += s(8)
		}
	}
	v.height = y + s(16)
	v.setScroll(v.scroll)
}

// ---------------------------------------------------------------------------
// Scrolling, hit tests, selection

func (v *docView) clientH() int32 {
	var rc rect
	pGetClientRect.Call(v.hwnd, uintptr(unsafe.Pointer(&rc)))
	return rc.Bottom
}

func (v *docView) setScroll(pos int32) {
	h := v.clientH()
	pos = max(0, min(pos, v.height-h))
	v.scroll = pos
	si := scrollInfo{Mask: 0x1 | 0x2 | 0x4 | 0x8 /* RANGE|PAGE|POS|DISABLENOSCROLL */, Min: 0, Max: max(v.height-1, 0), Page: uint32(max(h, 0)), Pos: pos}
	si.Size = uint32(unsafe.Sizeof(si))
	pSetScrollInfo.Call(v.hwnd, 1 /* SB_VERT */, uintptr(unsafe.Pointer(&si)), 1)
	pInvalidateRect.Call(v.hwnd, 0, 0)
	v.updateTips()
	v.linkTipAt(-1) // placed again at the next mouse move
}

// linkTipAt places the tooltip of the links on fragment i, the one under the mouse
// (-1: no link there). A link cut over two lines has a tool on each line in turn.
func (v *docView) linkTipAt(i int) {
	if v.tip == 0 || i == v.tipFrag {
		return
	}
	send(v.tip, ttmPop, 0, 0)
	v.tipFrag = i
	ti := toolInfo{Hwnd: v.hwnd, ID: linkToolID, Text: u16(" ")}
	if i >= 0 {
		f := v.frags[i]
		ti.Rect = rect{f.x, f.y - v.scroll, f.x + f.w, f.y + f.h - v.scroll}
		ti.Text = u16(strings.ReplaceAll(linkTip(f.link), "\n", "\r\n"))
	}
	ti.Size = uint32(unsafe.Sizeof(ti))
	send(v.tip, ttmNewToolRectW, 0, uintptr(unsafe.Pointer(&ti)))
	send(v.tip, ttmUpdateTipTextW, 0, uintptr(unsafe.Pointer(&ti)))
}

// updateTips gives each visible copy button its tooltip.
func (v *docView) updateTips() {
	if v.tip == 0 {
		return
	}
	for i := 1; i <= v.tips; i++ {
		ti := toolInfo{Hwnd: v.hwnd, ID: uintptr(i)}
		ti.Size = uint32(unsafe.Sizeof(ti))
		send(v.tip, 0x0433 /* TTM_DELTOOLW */, 0, uintptr(unsafe.Pointer(&ti)))
	}
	v.tips = 0
	text := u16(T("dlg.copyBlock"))
	for i, b := range v.btns {
		r := b.rc
		r.Top, r.Bottom = r.Top-v.scroll, r.Bottom-v.scroll
		ti := toolInfo{Flags: 0x10 /* TTF_SUBCLASS */, Hwnd: v.hwnd, ID: uintptr(i + 1), Rect: r, Text: text}
		ti.Size = uint32(unsafe.Sizeof(ti))
		send(v.tip, 0x0432 /* TTM_ADDTOOLW */, 0, uintptr(unsafe.Pointer(&ti)))
		v.tips = i + 1
	}
}

// docPoint converts a client point to document coordinates.
func (v *docView) docPoint(pt point) point { return point{pt.X, pt.Y + v.scroll} }

func (v *docView) btnAt(p point) int {
	for i, b := range v.btns {
		if p.X >= b.rc.Left && p.X < b.rc.Right && p.Y >= b.rc.Top && p.Y < b.rc.Bottom {
			return i
		}
	}
	return -1
}

func (v *docView) fragAt(p point) *docFrag {
	for i := range v.frags {
		f := &v.frags[i]
		if p.X >= f.x && p.X < f.x+f.w && p.Y >= f.y && p.Y < f.y+f.h {
			return f
		}
	}
	return nil
}

// linkFragAt is the fragment of a link at a document point (-1: none).
func (v *docView) linkFragAt(p point) int {
	for i := range v.frags {
		f := &v.frags[i]
		if f.link != "" && p.X >= f.x && p.X < f.x+f.w && p.Y >= f.y && p.Y < f.y+f.h {
			return i
		}
	}
	return -1
}

func (v *docView) linkAt(p point) string {
	if f := v.fragAt(p); f != nil {
		return f.link
	}
	return ""
}

// offsetAt returns the position in the document text nearest to a document point.
func (v *docView) offsetAt(p point) int {
	if len(v.frags) == 0 {
		return 0
	}
	// the frags of the nearest line
	best, bestD := -1, int32(1<<30)
	for i := range v.frags {
		f := &v.frags[i]
		var d int32
		switch {
		case p.Y < f.y:
			d = f.y - p.Y
		case p.Y >= f.y+f.h:
			d = p.Y - (f.y + f.h) + 1
		}
		if d < bestD {
			best, bestD = i, d
		}
	}
	ly := v.frags[best].y
	var row []*docFrag
	for i := range v.frags {
		if f := &v.frags[i]; f.y == ly || (p.Y >= f.y && p.Y < f.y+f.h && bestD == 0) {
			row = append(row, f)
		}
	}
	// the frag under or after the point
	var left *docFrag
	for _, f := range row {
		if p.X >= f.x && p.X < f.x+f.w {
			return f.off + v.charAt(f, p.X-f.x)
		}
		if f.x+f.w <= p.X && (left == nil || f.x > left.x) {
			left = f
		}
	}
	if left != nil {
		return left.off + len(left.text)
	}
	first := row[0]
	for _, f := range row {
		if f.x < first.x {
			first = f
		}
	}
	return first.off
}

// charAt is the index of the character boundary nearest to dx in a frag.
func (v *docView) charAt(f *docFrag, dx int32) int {
	dc, _, _ := pCreateCompatibleDC.Call(0)
	defer pDeleteDC.Call(dc)
	L := &docLayouter{v: v, dc: dc}
	n := L.fit(f.font, f.text, dx)
	if n < len(f.text) {
		a := L.measure(f.font, f.text[:n])
		b := L.measure(f.font, f.text[:n+1])
		if dx-a > b-dx {
			n++
		}
	}
	return n
}

func (v *docView) sel() (int, int) {
	if v.selA <= v.selB {
		return v.selA, v.selB
	}
	return v.selB, v.selA
}

func (v *docView) selectedText() string {
	s, e := v.sel()
	if s == e || e > len(v.text) {
		return ""
	}
	return string(utf16.Decode(v.text[s:e]))
}

// selectWord selects the run of non-space characters around an offset.
func (v *docView) selectWord(off int) {
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

func (v *docView) copyButton(i int) {
	b := v.btns[i]
	if setClipboard(v.hwnd, strings.Join(b.cmds, "\n")) {
		b.copied = time.Now()
		if v.onStatus != nil {
			v.onStatus(T("dlg.copiedBlock"))
		}
		pSetTimer.Call(v.hwnd, 1, 1600, 0)
		pInvalidateRect.Call(v.hwnd, 0, 0)
	}
}

func (v *docView) copySelection() {
	if t := v.selectedText(); t != "" && setClipboard(v.hwnd, t) && v.onStatus != nil {
		v.onStatus(T("copied"))
	}
}

// ---------------------------------------------------------------------------
// Painting

func (v *docView) paint() {
	a := g
	s := a.scale
	var ps paintStruct
	hdc, _, _ := pBeginPaint.Call(v.hwnd, uintptr(unsafe.Pointer(&ps)))
	defer pEndPaint.Call(v.hwnd, uintptr(unsafe.Pointer(&ps)))
	var rc rect
	pGetClientRect.Call(v.hwnd, uintptr(unsafe.Pointer(&rc)))
	if rc.Right <= 0 || rc.Bottom <= 0 {
		return
	}
	dc, _, _ := pCreateCompatibleDC.Call(hdc)
	bmp, _, _ := pCreateCompatibleBmp.Call(hdc, uintptr(rc.Right), uintptr(rc.Bottom))
	oldBmp, _, _ := pSelectObject.Call(dc, bmp)
	oldFont, _, _ := pSelectObject.Call(dc, v.fonts["body"].h)
	pFillRect.Call(dc, uintptr(unsafe.Pointer(&rc)), a.brushWindow)
	oy := -v.scroll
	visible := func(top, bottom int32) bool { return bottom+oy >= 0 && top+oy <= rc.Bottom }

	// command blocks
	bg, _, _ := pCreateSolidBrush.Call(docCmdBg)
	nullPen, _, _ := pGetStockObject.Call(8 /* NULL_PEN */)
	oldBrush, _, _ := pSelectObject.Call(dc, bg)
	oldPen, _, _ := pSelectObject.Call(dc, nullPen)
	for _, r := range v.blockRcs {
		if visible(r.Top, r.Bottom) {
			pRoundRect.Call(dc, uintptr(r.Left), uintptr(r.Top+oy), uintptr(r.Right+1), uintptr(r.Bottom+oy+1), uintptr(s(8)), uintptr(s(8)))
		}
	}
	pSelectObject.Call(dc, oldBrush)
	pSelectObject.Call(dc, oldPen)

	// status icons
	isz := s(20)
	for _, m := range v.marks {
		if visible(m.y, m.y+isz) {
			pDrawIconEx.Call(dc, uintptr(m.x), uintptr(m.y+oy), v.icons[m.status], uintptr(isz), uintptr(isz), 0, 0, 3)
		}
	}

	// text and selection
	pSetBkMode.Call(dc, 1 /* TRANSPARENT */)
	selS, selE := v.sel()
	hl, _, _ := pGetSysColorBrush.Call(13 /* COLOR_HIGHLIGHT */)
	for i := range v.frags {
		f := &v.frags[i]
		if !visible(f.y, f.y+f.h) || len(f.text) == 0 {
			continue
		}
		pSelectObject.Call(dc, f.font.h)
		pSetTextColor.Call(dc, uintptr(f.color))
		pExtTextOutW.Call(dc, uintptr(f.x), uintptr(f.ty+oy), 0, 0, uintptr(unsafe.Pointer(&f.text[0])), uintptr(len(f.text)), 0)
		if selE > f.off && selS < f.off+len(f.text) {
			i0, i1 := max(selS-f.off, 0), min(selE-f.off, len(f.text))
			L := &docLayouter{v: v, dc: dc}
			xa, xb := f.x+L.measure(f.font, f.text[:i0]), f.x+L.measure(f.font, f.text[:i1])
			r := rect{xa, f.y + oy, xb, f.y + f.h + oy}
			pFillRect.Call(dc, uintptr(unsafe.Pointer(&r)), hl)
			pSetTextColor.Call(dc, uintptr(sysColor(14 /* COLOR_HIGHLIGHTTEXT */)))
			pExtTextOutW.Call(dc, uintptr(xa), uintptr(f.ty+oy), 0, 0, uintptr(unsafe.Pointer(&f.text[i0])), uintptr(i1-i0), 0)
		}
	}

	// copy buttons
	for i, b := range v.btns {
		if visible(b.rc.Top, b.rc.Bottom) {
			v.drawCopyButton(dc, b, oy, i == v.hoverBtn)
		}
	}
	pDeleteObject.Call(bg)

	pBitBlt.Call(hdc, 0, 0, uintptr(rc.Right), uintptr(rc.Bottom), dc, 0, 0, 0x00CC0020 /* SRCCOPY */)
	pSelectObject.Call(dc, oldFont)
	pSelectObject.Call(dc, oldBmp)
	pDeleteObject.Call(bmp)
	pDeleteDC.Call(dc)
}

// drawCopyButton draws two overlapping sheets (or a check mark just after a copy).
func (v *docView) drawCopyButton(dc uintptr, b *docBtn, oy int32, hover bool) {
	r := b.rc
	r.Top, r.Bottom = r.Top+oy, r.Bottom+oy
	unit := float64(r.Right-r.Left) / 28
	px := func(f float64) uintptr { return uintptr(int32(f * unit)) }
	bgColor := uint32(docCmdBg)
	if hover {
		bgColor = rgb(58, 58, 58)
		br, _, _ := pCreateSolidBrush.Call(uintptr(bgColor))
		np, _, _ := pGetStockObject.Call(8)
		ob, _, _ := pSelectObject.Call(dc, br)
		op, _, _ := pSelectObject.Call(dc, np)
		pRoundRect.Call(dc, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right+1), uintptr(r.Bottom+1), px(6), px(6))
		pSelectObject.Call(dc, ob)
		pSelectObject.Call(dc, op)
		pDeleteObject.Call(br)
	}
	width := max(1, int32(1.6*unit+0.5))
	if time.Since(b.copied) < 1500*time.Millisecond {
		pen, _, _ := pCreatePen.Call(0, uintptr(max(2, int32(2.2*unit+0.5))), uintptr(rgb(110, 210, 120)))
		op, _, _ := pSelectObject.Call(dc, pen)
		x, y := uintptr(r.Left), uintptr(r.Top)
		pMoveToEx.Call(dc, x+px(7), y+px(14.5), 0)
		pLineTo.Call(dc, x+px(12), y+px(19.5))
		pLineTo.Call(dc, x+px(21.5), y+px(9))
		pSelectObject.Call(dc, op)
		pDeleteObject.Call(pen)
		return
	}
	color := rgb(170, 170, 170)
	if hover {
		color = rgb(245, 245, 245)
	}
	pen, _, _ := pCreatePen.Call(0, uintptr(width), uintptr(color))
	fill, _, _ := pCreateSolidBrush.Call(uintptr(bgColor))
	hollow, _, _ := pGetStockObject.Call(5 /* NULL_BRUSH */)
	op, _, _ := pSelectObject.Call(dc, pen)
	ob, _, _ := pSelectObject.Call(dc, hollow)
	x, y := uintptr(r.Left), uintptr(r.Top)
	pRoundRect.Call(dc, x+px(10), y+px(6), x+px(22), y+px(18), px(3), px(3))
	pSelectObject.Call(dc, fill)
	pRoundRect.Call(dc, x+px(6), y+px(10), x+px(18), y+px(22), px(3), px(3))
	pSelectObject.Call(dc, op)
	pSelectObject.Call(dc, ob)
	pDeleteObject.Call(pen)
	pDeleteObject.Call(fill)
}

// ---------------------------------------------------------------------------
// Window procedure

func docWndProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	v := docViews[hwnd]
	a := g
	if v != nil {
		switch msg {
		case wmEraseBkgnd:
			return 1
		case wmPaint:
			v.paint()
			return 0
		case wmSize:
			v.layout()
			return 0
		case wmVScroll:
			step := a.scale(40)
			page := max(v.clientH()-step, step)
			switch loword(wParam) {
			case 0:
				v.setScroll(v.scroll - step)
			case 1:
				v.setScroll(v.scroll + step)
			case 2:
				v.setScroll(v.scroll - page)
			case 3:
				v.setScroll(v.scroll + page)
			case 5, 4:
				si := scrollInfo{Mask: 0x10 /* SIF_TRACKPOS */}
				si.Size = uint32(unsafe.Sizeof(si))
				pGetScrollInfo.Call(hwnd, 1, uintptr(unsafe.Pointer(&si)))
				v.setScroll(si.TrackPos)
			case 6:
				v.setScroll(0)
			case 7:
				v.setScroll(v.height)
			}
			return 0
		case wmMouseWheel:
			delta := int32(int16(hiword(wParam)))
			v.setScroll(v.scroll - delta*a.scale(54)/120)
			return 0
		case wmGetDlgCode:
			return 0x1 // DLGC_WANTARROWS
		case wmKeyDown:
			ctrl := ctrlDown()
			step := a.scale(40)
			switch {
			case ctrl && wParam == 'C':
				v.copySelection()
			case ctrl && wParam == 'A':
				v.selA, v.selB = 0, len(v.text)
				pInvalidateRect.Call(hwnd, 0, 0)
			case wParam == 0x26: // up
				v.setScroll(v.scroll - step)
			case wParam == 0x28: // down
				v.setScroll(v.scroll + step)
			case wParam == 0x21: // page up
				v.setScroll(v.scroll - max(v.clientH()-step, step))
			case wParam == 0x22: // page down
				v.setScroll(v.scroll + max(v.clientH()-step, step))
			case wParam == 0x24: // home
				v.setScroll(0)
			case wParam == 0x23: // end
				v.setScroll(v.height)
			}
			return 0
		case wmSetCursor:
			if loword(lParam) == 1 {
				p := v.docPoint(cursorIn(hwnd))
				switch {
				case v.btnAt(p) >= 0 || v.linkAt(p) != "":
					pSetCursor.Call(a.curHand)
				case v.fragAt(p) != nil:
					c, _, _ := pLoadCursorW.Call(0, 32513 /* IDC_IBEAM */)
					pSetCursor.Call(c)
				default:
					pSetCursor.Call(a.curArrow)
				}
				return 1
			}
		case wmLButtonDown:
			pSetFocus.Call(hwnd)
			pt := point{xParam(lParam), yParam(lParam)}
			p := v.docPoint(pt)
			v.down, v.moved, v.downPt = true, false, pt
			v.downBtn, v.downLink = v.btnAt(p), v.linkAt(p)
			if v.downBtn < 0 {
				off := v.offsetAt(p)
				if wParam&0x4 /* MK_SHIFT */ != 0 {
					v.selB = off
				} else {
					v.selA, v.selB = off, off
				}
			}
			pSetCapture.Call(hwnd)
			pInvalidateRect.Call(hwnd, 0, 0)
			return 0
		case wmMouseMove:
			pt := point{xParam(lParam), yParam(lParam)}
			if v.down && v.downBtn < 0 {
				if abs32(pt.X-v.downPt.X)+abs32(pt.Y-v.downPt.Y) > a.scale(4) {
					v.moved = true
				}
				if v.moved {
					h := v.clientH()
					if pt.Y < 0 {
						v.setScroll(v.scroll - a.scale(24))
					} else if pt.Y > h {
						v.setScroll(v.scroll + a.scale(24))
					}
					v.selB = v.offsetAt(v.docPoint(pt))
					pInvalidateRect.Call(hwnd, 0, 0)
				}
				return 0
			}
			v.linkTipAt(v.linkFragAt(v.docPoint(pt)))
			if hb := v.btnAt(v.docPoint(pt)); hb != v.hoverBtn {
				v.hoverBtn = hb
				pInvalidateRect.Call(hwnd, 0, 0)
				tme := struct {
					Size, Flags uint32
					Hwnd        uintptr
					Hover       uint32
				}{Flags: 0x2 /* TME_LEAVE */, Hwnd: hwnd}
				tme.Size = uint32(unsafe.Sizeof(tme))
				pTrackMouseEvent.Call(uintptr(unsafe.Pointer(&tme)))
			}
			return 0
		case wmMouseLeave:
			if v.hoverBtn >= 0 {
				v.hoverBtn = -1
				pInvalidateRect.Call(hwnd, 0, 0)
			}
			return 0
		case wmLButtonUp:
			if !v.down {
				return 0
			}
			v.down = false
			pReleaseCapture.Call()
			p := v.docPoint(point{xParam(lParam), yParam(lParam)})
			if !v.moved {
				switch {
				case v.downBtn >= 0 && v.btnAt(p) == v.downBtn:
					v.copyButton(v.downBtn)
				case v.downLink != "" && v.linkAt(p) == v.downLink:
					v.selA, v.selB = 0, 0
					if key, ok := strings.CutPrefix(v.downLink, itemLinkPrefix); ok {
						g.openItemRef(key) // another element: its details replace these
					} else {
						openTarget(hwnd, v.downLink)
					}
				}
			}
			pInvalidateRect.Call(hwnd, 0, 0)
			return 0
		case wmLButtonDblClk:
			p := v.docPoint(point{xParam(lParam), yParam(lParam)})
			if v.btnAt(p) < 0 && v.linkAt(p) == "" && len(v.text) > 0 {
				v.selectWord(min(v.offsetAt(p), len(v.text)-1))
				pInvalidateRect.Call(hwnd, 0, 0)
			}
			return 0
		case wmCaptureChanged:
			v.down = false
			return 0
		case wmContextMenu:
			pt := point{xParam(lParam), yParam(lParam)}
			if int32(lParam) == -1 {
				pt = point{a.scale(40), a.scale(40)}
				pClientToScreen.Call(hwnd, uintptr(unsafe.Pointer(&pt)))
			}
			cp := pt
			pScreenToClient.Call(hwnd, uintptr(unsafe.Pointer(&cp)))
			link := v.linkAt(v.docPoint(cp))
			menu, _, _ := pCreatePopupMenu.Call()
			gray := uintptr(0)
			if v.selectedText() == "" {
				gray = 1
			}
			pAppendMenuW.Call(menu, gray, 1, up(T("dlg.menuCopy")))
			if link != "" {
				pAppendMenuW.Call(menu, 0, 2, up(T("dlg.menuCopyLink")))
			}
			pAppendMenuW.Call(menu, 0x800, 0, 0)
			pAppendMenuW.Call(menu, 0, 3, up(T("dlg.menuSelAll")))
			pAppendMenuW.Call(menu, 0, 4, up(T("dlg.menuCopyAll")))
			id, _, _ := pTrackPopupMenu.Call(menu, 0x100|0x2, uintptr(pt.X), uintptr(pt.Y), 0, hwnd, 0)
			pDestroyMenu.Call(menu)
			switch id {
			case 1:
				v.copySelection()
			case 2:
				if setClipboard(hwnd, link) && v.onStatus != nil {
					v.onStatus(T("copied"))
				}
			case 3:
				v.selA, v.selB = 0, len(v.text)
				pInvalidateRect.Call(hwnd, 0, 0)
			case 4:
				if v.onCopyAll != nil {
					v.onCopyAll()
				}
			}
			return 0
		case wmTimer:
			pKillTimer.Call(hwnd, wParam)
			pInvalidateRect.Call(hwnd, 0, 0)
			return 0
		case wmSetFocus, wmKillFocus:
			pInvalidateRect.Call(hwnd, 0, 0)
		}
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
	return r
}

func abs32(x int32) int32 {
	if x < 0 {
		return -x
	}
	return x
}
