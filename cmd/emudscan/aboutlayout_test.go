package main

import (
	"fmt"
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

// The text of the About box: its paragraphs, the items of its list, its links.
func TestAboutBlocks(t *testing.T) {
	got := aboutBlocks("One <a href=\"https://a.example.invalid/\">a</a>, two.\nFixes:\n• first;\n• <a href=\"https://b.example.invalid\">b</a>\n\nLast.")
	var lines []string
	for _, b := range got {
		l := fmt.Sprint(b.kind)
		for _, r := range b.runs {
			l += fmt.Sprintf(" %d:%s>%s", r.kind, r.text, r.target)
		}
		lines = append(lines, l)
	}
	want := []string{
		fmt.Sprintf("%d %d:One >|%d:a>https://a.example.invalid/|%d:, two.>", dkPara, runText, runLink, runText),
		fmt.Sprintf("%d %d:Fixes:>", dkPara, runText),
		fmt.Sprintf("%d %d:first;>", dkBullet, runText),
		fmt.Sprintf("%d %d:b>https://b.example.invalid", dkBullet, runLink),
		fmt.Sprint(dkSpace),
		fmt.Sprintf("%d %d:Last.>", dkPara, runText),
	}
	if strings.Join(lines, "\n") != strings.ReplaceAll(strings.Join(want, "\n"), "|", " ") {
		t.Errorf("got:\n%s\nwant:\n%s", strings.Join(lines, "\n"), strings.Join(want, "\n"))
	}
}
