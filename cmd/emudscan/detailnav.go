package main

import "math"

// The detailed information of several items opens in one window, with a navigation
// block in its top right corner: First, Previous, the item shown and the number of
// items, Next and Last. Alt+Home, Alt+←, Alt+→ and Alt+End do the same.

// The buttons of the navigation block, from left to right.
const (
	navFirst = iota
	navPrevious
	navNext
	navLast
	numNav
)

// navTo is the item that a button goes to, from item pos of n; -1 when it goes
// nowhere, as on the first or the last item, where its button is greyed.
func navTo(button, pos, n int) int {
	to := -1
	switch button {
	case navFirst:
		to = 0
	case navPrevious:
		to = pos - 1
	case navNext:
		to = pos + 1
	case navLast:
		to = n - 1
	}
	if n < 2 || to < 0 || to >= n || to == pos {
		return -1
	}
	return to
}

// navKey is the button of a key pressed with Alt: Home, ←, → or End; -1 for another
// key.
func navKey(vk uintptr) int {
	switch vk {
	case 0x24: // Home
		return navFirst
	case 0x25: // left
		return navPrevious
	case 0x27: // right
		return navNext
	case 0x23: // End
		return navLast
	}
	return -1
}

// navTipKeys are the tooltips of the buttons, which give their keys.
var navTipKeys = [numNav]string{"nav.first", "nav.previous", "nav.next", "nav.last"}

// navShadow is the shadow of the navigation block, as the menus of Windows 11 have:
// a w × h rounded rectangle (corner radius r), lowered by dy and blurred (Gaussian,
// sigma), dark at most peak (0-255). It is the alpha of each pixel, in an area pad
// pixels wider than the block on each side; the block is drawn over it, at (pad, pad).
func navShadow(w, h, r, dy, sigma int, peak uint8) (aw, ah, pad int, alpha []uint8) {
	pad = 3 * sigma
	aw, ah = w+2*pad, h+2*pad
	cover := make([]float64, aw*ah)
	inside := func(x, y float64) bool { // in the rounded rectangle, from its top left corner
		cx, cy := min(max(x, float64(r)), float64(w-r)), min(max(y, float64(r)), float64(h-r))
		return x >= 0 && x < float64(w) && y >= 0 && y < float64(h) && (x-cx)*(x-cx)+(y-cy)*(y-cy) <= float64(r*r)
	}
	for y := 0; y < ah; y++ {
		for x := 0; x < aw; x++ {
			if inside(float64(x-pad)+0.5, float64(y-pad-dy)+0.5) {
				cover[y*aw+x] = float64(peak)
			}
		}
	}
	if sigma > 0 { // the Gaussian, across then down
		k := make([]float64, 2*pad+1)
		sum := 0.0
		for i := range k {
			d := float64(i - pad)
			k[i] = math.Exp(-d * d / (2 * float64(sigma*sigma)))
			sum += k[i]
		}
		blur := func(at func(i, j int) *float64, n, m int) {
			line := make([]float64, n)
			for j := 0; j < m; j++ {
				for i := range line {
					v := 0.0
					for t, kt := range k {
						if p := i + t - pad; p >= 0 && p < n {
							v += kt * *at(p, j)
						}
					}
					line[i] = v / sum
				}
				for i, v := range line {
					*at(i, j) = v
				}
			}
		}
		blur(func(x, y int) *float64 { return &cover[y*aw+x] }, aw, ah)
		blur(func(y, x int) *float64 { return &cover[y*aw+x] }, ah, aw)
	}
	alpha = make([]uint8, aw*ah)
	for i, v := range cover {
		alpha[i] = uint8(min(255, v+0.5))
	}
	return aw, ah, pad, alpha
}
