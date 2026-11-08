package main

// ANSI escape sequences in what the commands show: some programs color their output
// even when it goes to a file, such as git, which Scoop runs. The console of the Fix
// window shows these colors, as a terminal does, with the palette of Windows Terminal
// that the commands use (psColor); the report and copies get the text without them.

import "strings"

// ansiPiece is a piece of a line with the same color and weight.
type ansiPiece struct {
	text  string
	color int32 // 0xRRGGBB, or -1: the color of the line
	bold  bool
}

// ansiPalette is Windows Terminal's Campbell palette: the 8 colors, then their bright
// variants.
var ansiPalette = [16]int32{
	0x0C0C0C, 0xC50F1F, 0x13A10E, 0xC19C00, 0x0037DA, 0x881798, 0x3A96DD, 0xCCCCCC,
	0x767676, 0xE74856, 0x16C60C, 0xF9F1A5, 0x3B78FF, 0xB4009E, 0x61D6D6, 0xF2F2F2,
}

// ansi256 is a color of the 256-color palette: the 16 above, a 6×6×6 cube, then 24
// grays.
func ansi256(n int) int32 {
	switch {
	case n < 16:
		return ansiPalette[n]
	case n < 232:
		level := func(v int) int32 {
			if v == 0 {
				return 0
			}
			return int32(55 + 40*v)
		}
		n -= 16
		return level(n/36)<<16 | level(n/6%6)<<8 | level(n%6)
	default:
		g := int32(8 + 10*(n-232))
		return g<<16 | g<<8 | g
	}
}

// ansiPieces cuts a line at its escape sequences: the colors (SGR, ESC[…m) give the
// pieces their color and weight; the other sequences (cursor moves, erasing, window
// titles) are dropped. A carriage return starts the line again, as a progress bar that
// a terminal redraws in place: what came before it is dropped. A line without any of
// these is one piece.
func ansiPieces(s string) []ansiPiece {
	if !strings.ContainsAny(s, "\x1b\u009b\r") {
		return []ansiPiece{{s, -1, false}}
	}
	var out []ansiPiece
	var b strings.Builder
	color, bold := int32(-1), false
	flush := func() {
		if b.Len() > 0 {
			out = append(out, ansiPiece{b.String(), color, bold})
			b.Reset()
		}
	}
	r := []rune(s)
	for i := 0; i < len(r); i++ {
		c := r[i]
		if c == '\r' {
			out = nil
			b.Reset()
			continue
		}
		if c != 0x1b && c != 0x9b {
			b.WriteRune(c)
			continue
		}
		if c == 0x1b && i+1 < len(r) && r[i+1] == ']' { // OSC: up to BEL or ESC \
			j := i + 2
			for j < len(r) && r[j] != 0x07 && !(r[j] == 0x1b && j+1 < len(r) && r[j+1] == '\\') {
				j++
			}
			if j < len(r) && r[j] == 0x1b {
				j++
			}
			i = j
			continue
		}
		csi := c == 0x9b || i+1 < len(r) && r[i+1] == '['
		j := i + 1
		if c == 0x1b && csi {
			j++
		}
		if !csi { // ESC, its intermediate bytes, then one final byte
			for j < len(r) && r[j] >= 0x20 && r[j] <= 0x2f {
				j++
			}
			i = j
			continue
		}
		start := j
		for j < len(r) && r[j] >= 0x20 && r[j] <= 0x3f { // parameters and intermediates
			j++
		}
		if j < len(r) && r[j] == 'm' {
			flush()
			color, bold = ansiSGR(string(r[start:j]), color, bold)
		}
		i = j // the final byte
	}
	flush()
	if len(out) == 0 {
		out = []ansiPiece{{"", -1, false}}
	}
	return out
}

// ansiSGR applies the parameters of ESC[…m to a color and a weight; backgrounds and
// other attributes are left out.
func ansiSGR(params string, color int32, bold bool) (int32, bool) {
	p := strings.FieldsFunc(params, func(r rune) bool { return r == ';' || r == ':' })
	if len(p) == 0 {
		p = []string{"0"}
	}
	num := func(s string) int {
		n := 0
		for _, d := range s {
			if d < '0' || d > '9' {
				return -1
			}
			n = n*10 + int(d-'0')
		}
		return n
	}
	for i := 0; i < len(p); i++ {
		switch n := num(p[i]); {
		case n == 0:
			color, bold = -1, false
		case n == 1:
			bold = true
		case n == 22:
			bold = false
		case n >= 30 && n <= 37:
			color = ansiPalette[n-30]
		case n >= 90 && n <= 97:
			color = ansiPalette[n-90+8]
		case n == 39:
			color = -1
		case n == 38 || n == 48: // 5;n or 2;r;g;b, of the text or of the background
			c := int32(-2)
			if i+2 < len(p) && num(p[i+1]) == 5 {
				if v := num(p[i+2]); v >= 0 && v < 256 {
					c = ansi256(v)
				}
				i += 2
			} else if i+4 < len(p) && num(p[i+1]) == 2 {
				rr, gg, bb := num(p[i+2]), num(p[i+3]), num(p[i+4])
				if rr >= 0 && rr < 256 && gg >= 0 && gg < 256 && bb >= 0 && bb < 256 {
					c = int32(rr)<<16 | int32(gg)<<8 | int32(bb)
				}
				i += 4
			}
			if n == 38 && c != -2 {
				color = c
			}
		}
	}
	return color, bold
}

// ansiStrip is a line without its escape sequences.
func ansiStrip(s string) string {
	if !strings.ContainsAny(s, "\x1b\u009b\r") {
		return s
	}
	var b strings.Builder
	for _, p := range ansiPieces(s) {
		b.WriteString(p.text)
	}
	return b.String()
}
