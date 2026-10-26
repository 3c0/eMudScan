package main

// Tiny vector rasterizer for the status icons and the application icon, so the
// program needs no image files and icons stay sharp at every DPI.

import "math"

type rgba struct{ r, g, b, a float64 }

func hexColor(c uint32, a float64) rgba {
	return rgba{float64(c>>16&0xff) / 255, float64(c>>8&0xff) / 255, float64(c&0xff) / 255, a}
}

type shapeFn func(x, y float64) bool

type layer struct {
	in shapeFn
	c  func(x, y float64) rgba
}

func solid(c rgba) func(x, y float64) rgba { return func(float64, float64) rgba { return c } }

// vertical gradient from top color to bottom color between y0 and y1
func vgrad(top, bottom rgba, y0, y1 float64) func(x, y float64) rgba {
	return func(_, y float64) rgba {
		t := math.Max(0, math.Min(1, (y-y0)/(y1-y0)))
		return rgba{top.r + (bottom.r-top.r)*t, top.g + (bottom.g-top.g)*t, top.b + (bottom.b-top.b)*t, top.a + (bottom.a-top.a)*t}
	}
}

func circle(cx, cy, r float64) shapeFn {
	return func(x, y float64) bool { return (x-cx)*(x-cx)+(y-cy)*(y-cy) <= r*r }
}

func ring(cx, cy, r0, r1 float64) shapeFn {
	return func(x, y float64) bool {
		d := (x-cx)*(x-cx) + (y-cy)*(y-cy)
		return d <= r1*r1 && d >= r0*r0
	}
}

func segDist(px, py, ax, ay, bx, by float64) float64 {
	dx, dy := bx-ax, by-ay
	t := ((px-ax)*dx + (py-ay)*dy) / (dx*dx + dy*dy)
	t = math.Max(0, math.Min(1, t))
	ex, ey := ax+t*dx-px, ay+t*dy-py
	return math.Sqrt(ex*ex + ey*ey)
}

// stroke: polyline with round caps and joins
func stroke(w float64, pts ...[2]float64) shapeFn {
	return func(x, y float64) bool {
		for i := 0; i+1 < len(pts); i++ {
			if segDist(x, y, pts[i][0], pts[i][1], pts[i+1][0], pts[i+1][1]) <= w/2 {
				return true
			}
		}
		return false
	}
}

func inPoly(x, y float64, pts [][2]float64) bool {
	in := false
	for i, j := 0, len(pts)-1; i < len(pts); j, i = i, i+1 {
		xi, yi, xj, yj := pts[i][0], pts[i][1], pts[j][0], pts[j][1]
		if (yi > y) != (yj > y) && x < (xj-xi)*(y-yi)/(yj-yi)+xi {
			in = !in
		}
	}
	return in
}

// roundedPoly fills polygon pts grown by radius r (rounded corners).
func roundedPoly(r float64, pts ...[2]float64) shapeFn {
	return func(x, y float64) bool {
		if inPoly(x, y, pts) {
			return true
		}
		for i := range pts {
			j := (i + 1) % len(pts)
			if segDist(x, y, pts[i][0], pts[i][1], pts[j][0], pts[j][1]) <= r {
				return true
			}
		}
		return false
	}
}

func union(fs ...shapeFn) shapeFn {
	return func(x, y float64) bool {
		for _, f := range fs {
			if f(x, y) {
				return true
			}
		}
		return false
	}
}

// render rasterizes layers (bottom first) into straight-alpha RGBA, top-down rows.
// Each layer's shape is first tested at 9 points of every pixel (corners, middles of
// the sides, centre): where they all agree the pixel is not crossed by an edge of that
// layer and its sub-samples need no test; pixels entirely away from any edge are computed once.
// Only the edges are supersampled, which makes it fast enough to run at start-up.
func render(size int, layers []layer) []byte {
	ss := 8
	if size >= 48 {
		ss = 4
	}
	if len(layers) > 64 {
		layers = layers[:64]
	}
	fs := float64(size)
	mask := func(x, y float64) uint64 {
		var m uint64
		for i, l := range layers {
			if l.in(x, y) {
				m |= 1 << uint(i)
			}
		}
		return m
	}
	// shape tests on a half-pixel grid: corners, middles of the sides and centres
	gw := 2*size + 1
	grid := make([]uint64, gw*gw)
	for y := 0; y < gw; y++ {
		for x := 0; x < gw; x++ {
			grid[y*gw+x] = mask(float64(x)/(2*fs), float64(y)/(2*fs))
		}
	}
	// composite the layers of mask m at (x, y): premultiplied colour and alpha
	over := func(m uint64, x, y float64) (r, g, b, a float64) {
		for i, l := range layers {
			if m&(1<<uint(i)) == 0 {
				continue
			}
			c := l.c(x, y)
			r = c.r*c.a + r*(1-c.a)
			g = c.g*c.a + g*(1-c.a)
			b = c.b*c.a + b*(1-c.a)
			a = c.a + a*(1-c.a)
		}
		return
	}
	out := make([]byte, size*size*4)
	n := float64(ss * ss)
	for py := 0; py < size; py++ {
		for px := 0; px < size; px++ {
			cx, cy := (float64(px)+.5)/fs, (float64(py)+.5)/fs
			mc := grid[(2*py+1)*gw+2*px+1]
			all, anyIn := mc, mc
			for dy := 0; dy <= 2; dy++ {
				for dx := 0; dx <= 2; dx++ {
					m := grid[(2*py+dy)*gw+2*px+dx]
					all &= m
					anyIn |= m
				}
			}
			var pr, pg, pb, pa float64 // premultiplied sums
			if all == anyIn {          // no edge crosses this pixel
				r, g, b, a := over(mc, cx, cy)
				pr, pg, pb, pa, n = r, g, b, a, 1
			} else {
				n = float64(ss * ss)
				edge := all ^ anyIn // layers whose edge crosses the pixel
				for sy := 0; sy < ss; sy++ {
					for sx := 0; sx < ss; sx++ {
						x := (float64(px) + (float64(sx)+0.5)/float64(ss)) / fs
						y := (float64(py) + (float64(sy)+0.5)/float64(ss)) / fs
						m := all
						for i, l := range layers {
							if edge&(1<<uint(i)) != 0 && l.in(x, y) {
								m |= 1 << uint(i)
							}
						}
						r, g, b, a := over(m, x, y)
						pr += r
						pg += g
						pb += b
						pa += a
					}
				}
			}
			i := (py*size + px) * 4
			if pa > 0 {
				out[i] = byte(math.Round(pr / pa * 255))
				out[i+1] = byte(math.Round(pg / pa * 255))
				out[i+2] = byte(math.Round(pb / pa * 255))
				out[i+3] = byte(math.Round(pa / n * 255))
			}
		}
	}
	return out
}

type iconKind int

const (
	iconPending iconKind = iota
	iconOK
	iconInfo
	iconWarn
	iconError
	iconApp
)

func statusIcon(s Status) iconKind {
	switch s {
	case StOK:
		return iconOK
	case StInfo:
		return iconInfo
	case StWarn:
		return iconWarn
	case StError:
		return iconError
	}
	return iconPending
}

var white = rgba{1, 1, 1, 1}

func iconLayers(k iconKind) []layer {
	disc := func(top, bottom uint32) []layer {
		return []layer{
			{circle(.5, .5, .47), solid(hexColor(bottom, 1))},
			{circle(.5, .5, .43), vgrad(hexColor(top, 1), hexColor(bottom, 1), .07, .93)},
		}
	}
	const w = .125 // stroke width
	switch k {
	case iconOK:
		return append(disc(0x34C46A, 0x168A3F), layer{stroke(w, [2]float64{.29, .52}, [2]float64{.44, .67}, [2]float64{.72, .36}), solid(white)})
	case iconError:
		return append(disc(0xF0554A, 0xC22417),
			layer{union(stroke(w, [2]float64{.34, .34}, [2]float64{.66, .66}), stroke(w, [2]float64{.66, .34}, [2]float64{.34, .66})), solid(white)})
	case iconInfo:
		return append(disc(0x4A93F0, 0x1A5FC8),
			layer{union(circle(.5, .29, .075), stroke(w, [2]float64{.5, .45}, [2]float64{.5, .72})), solid(white)})
	case iconWarn:
		tri := [][2]float64{{.5, .13}, {.9, .84}, {.1, .84}}
		ink := hexColor(0x3D2E00, 1)
		return []layer{
			{roundedPoly(.06, tri...), solid(hexColor(0xC98A00, 1))},
			{roundedPoly(.03, [2]float64{.5, .18}, [2]float64{.85, .81}, [2]float64{.15, .81}), vgrad(hexColor(0xFFD34D, 1), hexColor(0xF2A900, 1), .15, .85)},
			{union(stroke(.11, [2]float64{.5, .39}, [2]float64{.5, .6}), circle(.5, .72, .065)), solid(ink)},
		}
	case iconApp:
		shield := [][2]float64{{.5, .05}, {.86, .17}}
		// right side curve down to the tip, then mirror
		var right [][2]float64
		for i := 0; i <= 12; i++ {
			t := float64(i) / 12
			// quadratic bezier from (.86,.17) via (.86,.72) to (.5,.95)
			x := (1-t)*(1-t)*.86 + 2*(1-t)*t*.86 + t*t*.5
			y := (1-t)*(1-t)*.17 + 2*(1-t)*t*.72 + t*t*.95
			right = append(right, [2]float64{x, y})
		}
		shield = append(shield, right...)
		for i := len(right) - 2; i >= 0; i-- {
			shield = append(shield, [2]float64{1 - right[i][0], right[i][1]})
		}
		shield = append(shield, [2]float64{.14, .17})
		inner := make([][2]float64, len(shield))
		for i, p := range shield { // shrink towards the centre
			inner[i] = [2]float64{.5 + (p[0]-.5)*.84, .52 + (p[1]-.52)*.86}
		}
		return []layer{
			{roundedPoly(.02, shield...), solid(hexColor(0x123B7A, 1))},
			{roundedPoly(.01, inner...), vgrad(hexColor(0x3D8BF2, 1), hexColor(0x1C54B8, 1), .1, .9)},
			{stroke(.1, [2]float64{.32, .5}, [2]float64{.45, .64}, [2]float64{.7, .37}), solid(white)},
		}
	default: // pending: grey ring
		return []layer{{ring(.5, .5, .27, .43), solid(hexColor(0x9AA0A6, 1))}}
	}
}

// renderIcon returns straight-alpha RGBA pixels, top-down.
func renderIcon(k iconKind, size int) []byte { return render(size, iconLayers(k)) }

// ---------------------------------------------------------------------------
// Spinner

// spinFrames is the number of frames of the spinner (one scan down and up).
const spinFrames = 40

// memo remembers the last result of a shape (not safe for concurrent use).
func memo(f shapeFn) shapeFn {
	lx, ly, lv := math.NaN(), math.NaN(), false
	return func(x, y float64) bool {
		if x != lx || y != ly {
			lx, ly, lv = x, y, f(x, y)
		}
		return lv
	}
}

// spinnerShield: the program's shield, scanned by a white line that goes down and
// up, with a fading trail behind it.
func spinnerShield(f, n, size int) []byte {
	p := float64(f) / float64(n)
	pos := .12 + .76*(.5-.5*math.Cos(2*math.Pi*p))
	dir := math.Sin(2 * math.Pi * p) // > 0 going down
	trail := .07 + .33*math.Abs(dir) // longer when the line moves fast
	app := iconLayers(iconApp)
	shape := memo(app[1].in) // tested several times in a row at the same point
	layers := []layer{app[0], {shape, app[1].c},
		{func(x, y float64) bool {
			d := (pos - y) * math.Copysign(1, dir) // distance behind the line
			return d >= 0 && d < trail && shape(x, y)
		}, func(x, y float64) rgba {
			d := math.Abs(pos-y) / trail
			return rgba{.82, .97, 1, .8 * math.Pow(1-d, 1.8)}
		}},
		{func(x, y float64) bool { return math.Abs(y-pos) < .034 && shape(x, y) }, solid(rgba{1, 1, 1, 1})},
		app[2],
	}
	return render(size, layers)
}

// ---------------------------------------------------------------------------
// Button glyphs (single colour)

type glyph int

const (
	glyphNone glyph = iota
	glyphPlay
	glyphStop
	glyphCopy
	glyphSave
	glyphTerminal
	glyphFolder
	glyphClose
	glyphRefresh
	glyphSearch
)

func rectOutline(w, x0, y0, x1, y1 float64) shapeFn {
	return stroke(w, [2]float64{x0, y0}, [2]float64{x1, y0}, [2]float64{x1, y1}, [2]float64{x0, y1}, [2]float64{x0, y0})
}

func glyphLayers(k glyph, c rgba) []layer {
	const w = .11
	col := solid(c)
	switch k {
	case glyphPlay:
		return []layer{{roundedPoly(.05, [2]float64{.3, .2}, [2]float64{.8, .5}, [2]float64{.3, .8}), col}}
	case glyphStop:
		return []layer{{roundedPoly(.05, [2]float64{.25, .25}, [2]float64{.75, .25}, [2]float64{.75, .75}, [2]float64{.25, .75}), col}}
	case glyphCopy:
		back := rectOutline(w, .4, .12, .86, .58)
		return []layer{
			{func(x, y float64) bool { return back(x, y) && !(x > .08 && x < .68 && y > .34 && y < .94) }, col},
			{rectOutline(w, .16, .42, .6, .86), col},
		}
	case glyphSave:
		return []layer{
			{union(stroke(w, [2]float64{.5, .12}, [2]float64{.5, .6}), stroke(w, [2]float64{.3, .42}, [2]float64{.5, .62}, [2]float64{.7, .42})), col},
			{stroke(w, [2]float64{.14, .62}, [2]float64{.14, .86}, [2]float64{.86, .86}, [2]float64{.86, .62}), col},
		}
	case glyphTerminal:
		return []layer{{union(stroke(w, [2]float64{.16, .3}, [2]float64{.4, .5}, [2]float64{.16, .7}), stroke(w, [2]float64{.5, .74}, [2]float64{.84, .74})), col}}
	case glyphSearch: // a magnifying glass
		return []layer{{union(ring(.42, .42, .25-w/2, .25+w/2), stroke(w*1.2, [2]float64{.6, .6}, [2]float64{.86, .86})), col}}
	case glyphFolder:
		return []layer{{stroke(w, [2]float64{.12, .3}, [2]float64{.12, .82}, [2]float64{.88, .82}, [2]float64{.88, .38}, [2]float64{.5, .38}, [2]float64{.42, .26}, [2]float64{.12, .26}, [2]float64{.12, .3}), col}}
	case glyphRefresh:
		// a clockwise circular arrow: "check again"
		const r = .29
		start, end := -math.Pi/3, -math.Pi/2-math.Pi/9 // the arc runs clockwise from start to end
		arc := func(x, y float64) bool {
			d := math.Hypot(x-.5, y-.5)
			if d < r-w/2 || d > r+w/2 {
				return false
			}
			a := math.Atan2(y-.5, x-.5)
			return !(a > end && a < start) // the gap at the top
		}
		px, py := .5+r*math.Cos(end), .5+r*math.Sin(end)
		tx, ty := -math.Sin(end), math.Cos(end) // direction of travel
		nx, ny := math.Cos(end), math.Sin(end)
		const h, b = .2, .15
		head := roundedPoly(.015, [2]float64{px + tx*h, py + ty*h}, [2]float64{px + nx*b, py + ny*b}, [2]float64{px - nx*b, py - ny*b})
		return []layer{{arc, col}, {head, col}}
	case glyphClose:
		return []layer{{union(stroke(w, [2]float64{.24, .24}, [2]float64{.76, .76}), stroke(w, [2]float64{.76, .24}, [2]float64{.24, .76})), col}}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Status icon sets (proposals): 1 shields, 2 signs, 3 stop sign

// markLayer draws a status mark (check, bang, cross, i) in a colour.
func markLayer(mark string, ink rgba, cy float64) layer {
	const w = .12
	switch mark {
	case "check":
		return layer{stroke(w, [2]float64{.3, cy + .01}, [2]float64{.45, cy + .15}, [2]float64{.71, cy - .13}), solid(ink)}
	case "cross":
		d := .14
		return layer{union(stroke(w, [2]float64{.5 - d, cy - d}, [2]float64{.5 + d, cy + d}), stroke(w, [2]float64{.5 + d, cy - d}, [2]float64{.5 - d, cy + d})), solid(ink)}
	case "info":
		return layer{union(circle(.5, cy-.19, .07), stroke(w, [2]float64{.5, cy - .04}, [2]float64{.5, cy + .2})), solid(ink)}
	default: // bang
		return layer{union(stroke(.115, [2]float64{.5, cy - .21}, [2]float64{.5, cy + .05}), circle(.5, cy+.2, .068)), solid(ink)}
	}
}

type tint struct{ top, bottom, rim uint32 }

var (
	tGreen  = tint{0x34C46A, 0x168A3F, 0x0F6B30}
	tBlue   = tint{0x4A93F0, 0x1A5FC8, 0x134A9E}
	tYellow = tint{0xFFD34D, 0xF2A900, 0xC98A00}
	tRed    = tint{0xFF6157, 0xE5372C, 0xA3150C}
	tDark   = tint{0xB3263A, 0x7A1022, 0x4E0814}
)

func shieldStatus(t tint, mark string, ink rgba) []layer {
	app := iconLayers(iconApp)
	return []layer{{app[0].in, solid(hexColor(t.rim, 1))}, {app[1].in, vgrad(hexColor(t.top, 1), hexColor(t.bottom, 1), .1, .9)}, markLayer(mark, ink, .49)}
}

// statusLayers draws the shield of a status: green OK, blue Information, yellow
// Warning, red Update needed, dark red Problem; a grey ring while checking.
func statusLayers(st Status) []layer {
	ink, dark := white, hexColor(0x3D2E00, 1)
	switch st {
	case StOK:
		return shieldStatus(tGreen, "check", ink)
	case StInfo:
		return shieldStatus(tBlue, "info", ink)
	case StCaution:
		return shieldStatus(tYellow, "bang", dark)
	case StWarn:
		return shieldStatus(tRed, "bang", ink)
	case StError:
		return shieldStatus(tDark, "cross", ink)
	}
	return []layer{{ring(.5, .5, .27, .43), solid(hexColor(0x9AA0A6, 1))}}
}

// renderStatus returns the icon of a status as straight-alpha RGBA pixels.
func renderStatus(st Status, size int) []byte { return render(size, statusLayers(st)) }
