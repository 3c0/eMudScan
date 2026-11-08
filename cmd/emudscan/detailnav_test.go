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
