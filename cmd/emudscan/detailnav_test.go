package main

import "testing"

// The buttons go to the first, previous, next and last items, and nowhere past the
// ends or with a single item; the keys are those of the buttons.
func TestNavTo(t *testing.T) {
	for _, c := range []struct{ button, pos, n, want int }{
		{navFirst, 1, 3, 0},
		{navPrevious, 1, 3, 0},
		{navNext, 1, 3, 2},
		{navLast, 1, 3, 2},
		{navFirst, 0, 3, -1},
		{navPrevious, 0, 3, -1},
		{navNext, 2, 3, -1},
		{navLast, 2, 3, -1},
		{navNext, 0, 1, -1},
		{-1, 0, 3, -1},
	} {
		if got := navTo(c.button, c.pos, c.n); got != c.want {
			t.Errorf("navTo(%d, %d, %d) = %d, want %d", c.button, c.pos, c.n, got, c.want)
		}
	}
	for vk, want := range map[uintptr]int{0x24: navFirst, 0x25: navPrevious, 0x27: navNext, 0x23: navLast, 0x26: -1} {
		if got := navKey(vk); got != want {
			t.Errorf("navKey(%#x) = %d, want %d", vk, got, want)
		}
	}
	for _, k := range navTipKeys {
		if T(k)[0] == '!' {
			t.Errorf("no text for %s", k)
		}
	}
}

// The shadow is darkest under the middle of the block, lowered, fades out to nothing
// at the edges of its area, and is symmetric from left to right.
func TestNavShadow(t *testing.T) {
	aw, ah, pad, a := navShadow(100, 30, 6, 2, 4, 70)
	if pad != 12 || aw != 124 || ah != 54 {
		t.Fatalf("area %d × %d, pad %d", aw, ah, pad)
	}
	at := func(x, y int) uint8 { return a[y*aw+x] }
	mid := at(aw/2, ah/2+2)
	if mid < 65 || mid > 70 {
		t.Errorf("middle: %d", mid)
	}
	if at(0, 0) != 0 || at(aw-1, ah-1) > 1 {
		t.Errorf("corners: %d %d", at(0, 0), at(aw-1, ah-1))
	}
	if top, bottom := at(aw/2, pad-2), at(aw/2, ah-pad+1); bottom <= top {
		t.Errorf("not lowered: above %d, below %d", top, bottom)
	}
	for y := 0; y < ah; y++ {
		for x := 0; x < aw/2; x++ {
			if d := int(at(x, y)) - int(at(aw-1-x, y)); d > 1 || d < -1 {
				t.Fatalf("not symmetric at %d,%d: %d", x, y, d)
			}
		}
	}
}
