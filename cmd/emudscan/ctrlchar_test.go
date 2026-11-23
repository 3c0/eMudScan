package main

import "testing"

// The characters of Ctrl+C and Ctrl+A go to the text views; Tab, Enter, Escape and the
// letters stay with the dialog manager.
func TestCtrlChar(t *testing.T) {
	for c, want := range map[uintptr]bool{3: true, 1: true, 26: true, '\t': false, '\n': false, '\r': false, 0x1b: false, 0: false, 'c': false, 'C': false, ' ': false} {
		if ctrlChar(c) != want {
			t.Errorf("%d: %v", c, !want)
		}
	}
}
