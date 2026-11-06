package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestWrapMarkup(t *testing.T) {
	measure := func(s string) int32 { return int32(len([]rune(s))) } // one unit a character
	link := `<a href="https://emudhra.we-have-a-problem.com/">https://emudhra.we-have-a-problem.com/</a>`
	para := "Reference data (certificates, test chains, fixed versions) comes from " + link + " by Dennis Jackson and contributors (Apache License 2.0)."
	got := wrapMarkup(para, 70, measure)
	want := []string{
		"Reference data (certificates, test chains, fixed versions) comes from",
		link + " by Dennis Jackson and",
		"contributors (Apache License 2.0).",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q", got)
	}
	// the link and its full stop stay together, even when too wide
	got = wrapMarkup("The live test connects to "+link+".", 30, measure)
	if len(got) != 2 || got[0] != "The live test connects to" || got[1] != link+"." {
		t.Fatalf("got %q", got)
	}
	// every word kept, in order
	for _, w := range []int32{1, 10, 25, 200} {
		lines := wrapMarkup(para, w, measure)
		if j := strings.Join(lines, " "); j != para {
			t.Fatalf("width %d: %q", w, j)
		}
		for _, l := range lines {
			if text := joinTokens(markupTokens(l), false); measure(text) > w && len(markupTokens(l)) > 1 && w > 45 {
				t.Fatalf("width %d: line too wide: %q", w, text)
			}
		}
	}
}
