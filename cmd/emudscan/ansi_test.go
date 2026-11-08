package main

import (
	"fmt"
	"strings"
	"testing"
)

// The colors of a line of output give its pieces their color; the other sequences
// are dropped, and the text stays whole.
func TestANSIPieces(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"plain text", "-1:plain text"},
		// git log, as Scoop shows it
		{" * \x1b[33m0123456789ab\x1b[m example-app-1: Add version 1.2.3 (#1234)  \x1b[32mmain   \x1b[m \x1b[36m44 minutes ago\x1b[m",
			"-1: * |C19C00:0123456789ab|-1: example-app-1: Add version 1.2.3 (#1234)  |13A10E:main   |-1: |3A96DD:44 minutes ago"},
		{"\x1b[1;91mERROR:\x1b[0m failed", "E74856+bold:ERROR:|-1: failed"},
		{"\x1b[38;5;208morange\x1b[39m \x1b[38;2;1;2;3mrgb\x1b[22m", "FF8700:orange|-1: |010203:rgb"},
		{"\x1b[48;5;21m\x1b[97mon blue\x1b[0m", "F2F2F2:on blue"},
		{"\x1b[2K\rprogress 50%\x1b[1A", "-1:progress 50%"},
		{"\x1b[32m 10%\r 60%\r100%", "13A10E:100%"},
		{"\x1b]0;title\x07after", "-1:after"},
		{"\x1b]8;;https://x\x1b\\link\x1b]8;;\x1b\\", "-1:link"},
		{"\x1b(Bkept\x1b", "-1:kept"},
	} {
		var got []string
		for _, p := range ansiPieces(c.in) {
			k := "-1"
			if p.color >= 0 {
				k = fmt.Sprintf("%06X", p.color)
			}
			if p.bold {
				k += "+bold"
			}
			got = append(got, k+":"+p.text)
		}
		if strings.Join(got, "|") != c.want {
			t.Errorf("%q:\ngot  %s\nwant %s", c.in, strings.Join(got, "|"), c.want)
		}
	}
	if s := ansiStrip(" * \x1b[33mdede0cc\x1b[m wash \x1b[36m3 hours ago\x1b[m"); s != " * dede0cc wash 3 hours ago" {
		t.Errorf("strip: %q", s)
	}
	if ansi256(16) != 0 || ansi256(231) != 0xFFFFFF || ansi256(244) != 0x808080 || ansi256(9) != 0xE74856 {
		t.Errorf("256 colors: %06X %06X %06X", ansi256(16), ansi256(231), ansi256(244))
	}
	// the report shows the text without the codes
	if p := (fixLine{lineOutput, "\x1b[32mmain\x1b[m"}).plain(); p != "      main" {
		t.Errorf("report: %q", p)
	}
}
