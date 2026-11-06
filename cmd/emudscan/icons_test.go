package main

import "testing"

// While an item is checked, and before the title of a section without a result, a
// grey shield without a mark, in the style of the others: a dark grey border, a
// lighter grey inside, lighter at the top.
func TestGreyShield(t *testing.T) {
	const n = 40
	px := renderStatus(StPending, n)
	at := func(x, y int) (r, g, b, a int) {
		i := (y*n + x) * 4
		return int(px[i]), int(px[i+1]), int(px[i+2]), int(px[i+3])
	}
	grey := func(x, y int) int { // brightness of an opaque grey pixel, or -1
		r, g, b, a := at(x, y)
		if a != 255 || r-b > 16 || b-r > 16 || g-r > 16 || r-g > 16 {
			return -1
		}
		return (r + g + b) / 3
	}
	if _, _, _, a := at(0, 0); a != 0 {
		t.Errorf("corner: alpha %d", a)
	}
	top, bottom := grey(n/2, n/4), grey(n/2, n*3/4)
	if top < 0 || bottom < 0 || top <= bottom {
		t.Errorf("inside: top %d, bottom %d", top, bottom)
	}
	border := 255
	for x := 0; x < n/2; x++ { // the darkest pixel of the middle line: the border
		if v := grey(x, n/2); v >= 0 && v < border {
			border = v
		}
	}
	if border >= bottom {
		t.Errorf("border %d not darker than the inside %d", border, bottom)
	}
}
