package main

// The text of the About box, in blocks for its text view (aboutBlocks); and line
// breaking for the box of a saved report: paragraphs are cut into lines by eMudScan
// itself, at spaces only, so that a link (a web address) is never cut, nor separated
// from the punctuation that follows it.

import (
	"regexp"
	"strings"
)

var (
	linkTagRe = regexp.MustCompile(`<a href="[^"]*">|</a>`)
	linkRe    = regexp.MustCompile(`<a href="([^"]*)">(.*?)</a>`)
)

// aboutBlocks reads the text of the About box: paragraphs, cut by empty lines; in a
// paragraph, a line that starts with "• " is an item of a list, another line a
// paragraph of its own, without space before it; <a href="…">…</a>, a link.
func aboutBlocks(text string) []docBlock {
	var out []docBlock
	for i, para := range strings.Split(text, "\n\n") {
		if i > 0 {
			out = append(out, docBlock{kind: dkSpace})
		}
		for _, l := range strings.Split(para, "\n") {
			kind := dkPara
			if item, ok := strings.CutPrefix(l, "• "); ok {
				kind, l = dkBullet, item
			}
			var runs []mrun
			for m := linkRe.FindStringSubmatchIndex(l); m != nil; m = linkRe.FindStringSubmatchIndex(l) {
				if m[0] > 0 {
					runs = append(runs, mrun{kind: runText, text: l[:m[0]]})
				}
				runs = append(runs, mrun{kind: runLink, text: l[m[4]:m[5]], target: l[m[2]:m[3]]})
				l = l[m[1]:]
			}
			if l != "" {
				runs = append(runs, mrun{kind: runText, text: l})
			}
			out = append(out, docBlock{kind: kind, runs: runs})
		}
	}
	return out
}

// markupTok is a word, or a whole link, of a paragraph.
type markupTok struct {
	markup, text string
	space        bool // preceded by a space: a line may start with it
}

func markupTokens(para string) []markupTok {
	var toks []markupTok
	space := false
	for para != "" {
		switch {
		case para[0] == ' ':
			space, para = true, para[1:]
			continue
		case strings.HasPrefix(para, "<a "):
			n := len(para)
			if end := strings.Index(para, "</a>"); end >= 0 {
				n = end + len("</a>")
			}
			toks = append(toks, markupTok{para[:n], linkTagRe.ReplaceAllString(para[:n], ""), space})
			para = para[n:]
		default:
			n := len(para)
			if i := strings.IndexByte(para, ' '); i >= 0 {
				n = i
			}
			if i := strings.Index(para[:n], "<a "); i > 0 {
				n = i
			}
			toks = append(toks, markupTok{para[:n], para[:n], space})
			para = para[n:]
		}
		space = false
	}
	return toks
}

func joinTokens(toks []markupTok, markup bool) string {
	var b strings.Builder
	for i, t := range toks {
		if i > 0 && t.space {
			b.WriteByte(' ')
		}
		if markup {
			b.WriteString(t.markup)
		} else {
			b.WriteString(t.text)
		}
	}
	return b.String()
}

// wrapMarkup cuts a paragraph into lines (with their links) whose text is at most
// width wide, as measured; a word or a link wider than that gets a line of its own.
func wrapMarkup(para string, width int32, measure func(string) int32) []string {
	var lines []string
	var cur []markupTok
	for _, t := range markupTokens(para) {
		if len(cur) > 0 && t.space && measure(joinTokens(append(cur, t), false)) > width {
			lines = append(lines, joinTokens(cur, true))
			cur = nil
		}
		if len(cur) == 0 {
			t.space = false
		}
		cur = append(cur, t)
	}
	if len(cur) > 0 {
		lines = append(lines, joinTokens(cur, true))
	}
	return lines
}
