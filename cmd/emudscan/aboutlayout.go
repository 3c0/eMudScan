package main

// Line breaking for the About box: paragraphs are cut into lines by eMudScan
// itself, at spaces only, so that a link (a web address) is never cut, nor
// separated from the punctuation that follows it.

import (
	"regexp"
	"strings"
)

var (
	linkTagRe = regexp.MustCompile(`<a href="[^"]*">|</a>`)
	linkRe    = regexp.MustCompile(`<a href="([^"]*)">(.*?)</a>`)
)

// lineLink is a link of a line of the About box: its address, and where its text is
// drawn (from the left of the line).
type lineLink struct {
	url    string
	x0, x1 int32
}

// lineLinks finds the links of a line of markup, for their tooltips.
func lineLinks(line string, measure func(string) int32) []lineLink {
	var out []lineLink
	for _, m := range linkRe.FindAllStringSubmatchIndex(line, -1) {
		x0 := measure(linkTagRe.ReplaceAllString(line[:m[0]], ""))
		out = append(out, lineLink{url: line[m[2]:m[3]], x0: x0, x1: x0 + measure(line[m[4]:m[5]])})
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
