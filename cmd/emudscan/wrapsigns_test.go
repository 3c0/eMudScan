package main

import (
	"strings"
	"testing"
	"unicode/utf16"
)

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

// A word too long for a row is cut between names: after the last /, \ or - that fits,
// never inside a name such as x86_64, nor after a run of them that starts the word.
func TestCutAt(t *testing.T) {
	url := "'https://example.com/download/20261110/cpython-3.11.18%2B20261110-x86_64-pc-windows-msvc.tar.gz'"
	for _, c := range []struct {
		s    string
		n    int
		want string // what stays on the row
	}{
		{url, strings.Index(url, "x86") + 1, "'https://example.com/download/20261110/cpython-3.11.18%2B20261110-"},
		{url, strings.Index(url, "x86") + 4, "'https://example.com/download/20261110/cpython-3.11.18%2B20261110-"},
		{url, strings.Index(url, "_64-") + 4, "'https://example.com/download/20261110/cpython-3.11.18%2B20261110-x86_64-"},
		{url, strings.Index(url, "cpython") + 2, "'https://example.com/download/20261110/"},
		{url, 12, "'https://"},
		{url, 9, "'https://"},
		{url, 5, "'http"}, // no break that fits: where the row is full
		{`"$env:LOCALAPPDATA\ExampleApp\cert.pem"`, 30, `"$env:LOCALAPPDATA\ExampleApp\`},
		{"--long-option-name", 5, "--lon"},
		{"--long-option-name", 9, "--long-"},
		{"f31751f4a8483f5ac7cd77ad83f8dfb80d9f2a72", 20, "f31751f4a8483f5ac7cd"},
	} {
		u := utf16.Encode([]rune(c.s))
		if got := string(utf16.Decode(u[:cutAt(u, c.n)])); got != c.want {
			t.Errorf("cutAt(%q, %d) keeps %q, want %q", c.s, c.n, got, c.want)
		}
	}
}
