package main

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
