package main

import "testing"

func TestLeadSpaces(t *testing.T) {
	for s, want := range map[string]int{"": 0, "$x = 1": 0, "    $x = 1": 4, "  ": 2, "\t$x": 0} {
		if got := leadSpaces(s); got != want {
			t.Errorf("leadSpaces(%q) = %d, want %d", s, got, want)
		}
	}
}

// ↩ is drawn between a third and a half of its row lower.
func TestWrapDrop(t *testing.T) {
	for _, h := range []int32{12, 16, 20, 24, 41, 60} {
		if d := wrapDrop(h); d*3 < h || d*2 >= h {
			t.Errorf("wrapDrop(%d) = %d, want between %d/3 and %d/2", h, d, h, h)
		}
	}
	if d := wrapDrop(41); d != 17 {
		t.Errorf("wrapDrop(41) = %d, want 17", d)
	}
}

// The next rows of a line start at its indent, but never past half of the room.
func TestWrapIndent(t *testing.T) {
	for _, c := range []struct {
		lead        int
		space, room int32
		want        int32
	}{
		{0, 8, 600, 0},
		{4, 8, 600, 32},
		{8, 8, 600, 64},
		{40, 8, 600, 300},
		{4, 8, 40, 20},
		{4, 8, -10, 0},
	} {
		if got := wrapIndent(c.lead, c.space, c.room); got != c.want {
			t.Errorf("wrapIndent(%d, %d, %d) = %d, want %d", c.lead, c.space, c.room, got, c.want)
		}
	}
}

// The lines of the script show the wrap signs; what the commands showed and the
// messages of eMudScan do not.
func TestOfScript(t *testing.T) {
	script := map[fixLineKind]bool{lineSoftware: true, lineComment: true, lineCommand: true, lineScript: true}
	for k := lineNote; k <= lineRule; k++ {
		if got := k.ofScript(); got != script[k] {
			t.Errorf("kind %d: ofScript() = %v, want %v", k, got, script[k])
		}
	}
}
