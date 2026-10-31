package main

// A very small markup for the details texts:
//   [[C:\path]] or [[https://url]]  link (opens Explorer or the browser)
//   [[text|target]]                 link with its own text
//   **text**                        bold
//   $ command                       a command line (monospace, copyable)
//   @ Label<TAB>value               a field: label and value are shown in two aligned columns
//   Label: rest                     a short label at the start of a line is shown in bold
// Plain URLs are turned into links automatically.

import (
	"regexp"
	"strings"
)

// pathBase is the last part of a Windows path (also when tested on another system).
func pathBase(p string) string { return p[strings.LastIndexAny(p, `\/`)+1:] }

type runKind int

const (
	runText runKind = iota
	runBold
	runLink
	runCommand
	runWarn // in amber, as in the lists: the software's age
)

type mrun struct {
	kind   runKind
	text   string
	target string // for links
}

// lk marks a path or URL as a link.
func lk(p string) string {
	if p == "" {
		return ""
	}
	return "[[" + p + "]]"
}

// cmd marks a line as a command.
func cmd(s string) string { return "$ " + s }

// Links to other elements: in the window they open that element's details; their
// text says where the element is, for the report.
const itemLinkPrefix = "item:"

// openInPrefix starts a link that opens a page in a given browser: "open:exe>url".
const openInPrefix = "open:"

// linkTip says what clicking a link does, for its tooltip: the address of a page, the
// browser it opens in, the file or folder shown in Explorer, another element.
func linkTip(target string) string {
	switch {
	case strings.HasPrefix(target, itemLinkPrefix):
		return T("tip.linkItem")
	case strings.HasPrefix(target, openInPrefix):
		exe, u, _ := strings.Cut(strings.TrimPrefix(target, openInPrefix), ">")
		if i := strings.LastIndexAny(exe, `\/`); i >= 0 {
			exe = exe[i+1:]
		}
		return Tf("tip.linkWith", exe, u)
	case strings.HasPrefix(lower(target), "http://") || strings.HasPrefix(lower(target), "https://"):
		return Tf("tip.linkWeb", target)
	}
	return Tf("tip.linkPath", target)
}

// itemRefs maps the key of an item link to the name of the element.
var itemRefs = map[string]func() string{"wincopy": func() string { return T("win.copyName") }}

// refWinCopy refers to the "Software copying the Windows root list" element.
func refWinCopy() string { return "[[" + T("ref.winCopy") + "|" + itemLinkPrefix + "wincopy]]" }

// field marks a line as a label/value field.
func field(label, value string) string { return "@ " + label + "\t" + value }

// parseField splits a field line.
func parseField(line string) (label, value string, ok bool) {
	if !strings.HasPrefix(line, "@ ") {
		return "", "", false
	}
	label, value, ok = strings.Cut(line[2:], "\t")
	return
}

// labelSep is the separator written after a label in plain text.
func labelSep() string {
	if Lang == 1 {
		return "\u00a0: " // French: a non-breaking space before the colon
	}
	return ": "
}

var (
	inlineRe = regexp.MustCompile(`\[\[(.+?)\]\]|\*\*(.+?)\*\*|https?://[^\s)\]"'<>]+`)
	labelRe  = regexp.MustCompile(`^([^:,;\\\[\]•*\n]{2,60}?)\s?:(\s|$)`) // a short label (a sentence with a comma is not one)
)

// parseLine splits one line of markup into runs.
func parseLine(line string) []mrun {
	if strings.HasPrefix(line, "$ ") {
		return []mrun{{kind: runCommand, text: line[2:]}}
	}
	if label, value, ok := parseField(line); ok {
		return append([]mrun{{kind: runBold, text: label + labelSep()}}, parseInline(value)...)
	}
	var runs []mrun
	if m := labelRe.FindStringSubmatchIndex(line); m != nil && !strings.Contains(line[:m[1]], "http") {
		runs = append(runs, mrun{kind: runBold, text: line[:m[4]]})
		line = line[m[4]:]
	}
	return append(runs, parseInline(line)...)
}

// parseInline splits text into plain, bold and link runs.
func parseInline(line string) []mrun {
	var runs []mrun
	for {
		m := inlineRe.FindStringSubmatchIndex(line)
		if m == nil {
			break
		}
		if m[0] > 0 {
			runs = append(runs, mrun{kind: runText, text: line[:m[0]]})
		}
		switch {
		case m[2] >= 0:
			t := line[m[2]:m[3]]
			text, target := t, t
			if a, b, ok := strings.Cut(t, "|"); ok {
				text, target = a, b
			}
			runs = append(runs, mrun{kind: runLink, text: text, target: target})
		case m[4] >= 0:
			runs = append(runs, mrun{kind: runBold, text: line[m[4]:m[5]]})
		default:
			u := strings.TrimRight(line[m[0]:m[1]], ".,;:")
			runs = append(runs, mrun{kind: runLink, text: u, target: u})
			line = line[m[0]+len(u):]
			continue
		}
		line = line[m[1]:]
	}
	if line != "" {
		runs = append(runs, mrun{kind: runText, text: line})
	}
	return runs
}

func parseMarkup(s string) [][]mrun {
	var out [][]mrun
	for _, l := range strings.Split(strings.ReplaceAll(s, "\r", ""), "\n") {
		out = append(out, parseLine(l))
	}
	return out
}

// plainText converts markup to plain text (for reports and the clipboard).
func plainText(s string) string {
	var b strings.Builder
	for i, line := range parseMarkup(s) {
		if i > 0 {
			b.WriteString("\n")
		}
		for _, r := range line {
			if r.kind == runCommand {
				b.WriteString("    ")
			}
			b.WriteString(r.text)
		}
	}
	return b.String()
}

// commandsIn returns the command lines of a markup text.
func commandsIn(s string) []string {
	var out []string
	for _, line := range parseMarkup(s) {
		for _, r := range line {
			if r.kind == runCommand {
				out = append(out, r.text)
			}
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Blocks: the details text as a list of paragraphs, fields and command blocks.

type docKind int

const (
	dkPara   docKind = iota // a paragraph of inline runs
	dkBullet                // a bulleted paragraph
	dkFields                // label/value rows shown in two aligned columns
	dkCmds                  // consecutive command lines, shown in a block with a copy button
	dkSpace                 // space between paragraphs
	dkTitle                 // the title of the window
	dkStatus                // the status line (icon, status, summary)
)

type docRow struct {
	label string
	value []mrun
}

type docBlock struct {
	kind    docKind
	runs    []mrun
	rows    []docRow
	cmds    []string
	status  Status
	summary string
}

// markupBlocks splits details markup into blocks.
func markupBlocks(s string) []docBlock {
	var out []docBlock
	last := func() *docBlock {
		if len(out) == 0 {
			return nil
		}
		return &out[len(out)-1]
	}
	for _, line := range strings.Split(strings.ReplaceAll(s, "\r", ""), "\n") {
		if strings.TrimSpace(line) == "" {
			if l := last(); l != nil && l.kind != dkSpace {
				out = append(out, docBlock{kind: dkSpace})
			}
			continue
		}
		if c, ok := strings.CutPrefix(line, "$ "); ok {
			if l := last(); l != nil && l.kind == dkCmds {
				l.cmds = append(l.cmds, c)
			} else {
				out = append(out, docBlock{kind: dkCmds, cmds: []string{c}})
			}
			continue
		}
		if label, value, ok := parseField(line); ok {
			row := docRow{label, parseInline(value)}
			if l := last(); l != nil && l.kind == dkFields {
				l.rows = append(l.rows, row)
			} else {
				out = append(out, docBlock{kind: dkFields, rows: []docRow{row}})
			}
			continue
		}
		if rest, ok := strings.CutPrefix(line, "• "); ok {
			out = append(out, docBlock{kind: dkBullet, runs: parseInline(rest)})
			continue
		}
		out = append(out, docBlock{kind: dkPara, runs: parseLine(line)})
	}
	for len(out) > 0 && out[len(out)-1].kind == dkSpace {
		out = out[:len(out)-1]
	}
	return out
}

// detailBlocks is the content of the details window of an item.
func detailBlocks(it Item) []docBlock {
	title := strings.TrimSpace(it.Program + " " + it.ProgVersion)
	if it.Group == GroupWindows || it.Program == "" {
		title = it.Name
	}
	bs := []docBlock{
		{kind: dkTitle, runs: []mrun{{kind: runText, text: title}}},
		{kind: dkStatus, status: it.Status, summary: it.Summary},
	}
	var rows []docRow
	add := func(label, value string, link bool) {
		if value == "" {
			return
		}
		r := mrun{kind: runText, text: value}
		if link {
			r = mrun{kind: runLink, text: value, target: value}
		}
		rows = append(rows, docRow{label, []mrun{r}})
	}
	if it.Group != GroupWindows {
		if it.Component != "" {
			add(T("dlg.component"), strings.TrimSpace(it.Component+" "+it.Version), false)
		}
		if u := it.Updated; u.shown() {
			parts := strings.SplitN(u.text("\x00"), "\x00", 2) // the file name is a link
			runs := []mrun{{kind: runText, text: parts[0]}}
			if len(parts) == 2 {
				runs = append(runs, mrun{kind: runLink, text: pathBase(u.File), target: u.File}, mrun{kind: runText, text: parts[1]})
			}
			if m := it.stale(); m > 0 {
				runs = append(runs, mrun{kind: runText, text: " — "}, mrun{kind: runWarn, text: "⚠ " + Tf("stale.short", ageText(m))})
			}
			rows = append(rows, docRow{T("dlg.updated"), runs})
		}
		if len(it.Copies) == 0 { // else the copies are listed below
			add(T("dlg.folder"), it.Folder, true)
			add(it.fileLabel(), it.File, true)
		}
		add(T("dlg.homepage"), it.Homepage(), true)
		if q := it.SearchText(); q != "" {
			rows = append(rows, docRow{T("dlg.search"), []mrun{{kind: runLink, text: Tf("dlg.searchGoogle", q), target: googleURL(q)}}})
		}
		if runs := packageRuns(it); len(runs) > 0 {
			rows = append(rows, docRow{T("dlg.package"), runs})
		}
	}
	if len(rows) > 0 {
		bs = append(bs, docBlock{kind: dkFields, rows: rows})
	}
	text := it.shownDetails()
	if adv := staleAdvice(it); adv != "" { // first: it concerns everything below
		text = joinParas(adv, text)
	}
	if details := markupBlocks(text); len(details) > 0 {
		bs = append(bs, docBlock{kind: dkSpace})
		bs = append(bs, details...)
	}
	return bs
}

// blocksText is the plain text of blocks (for "Copy all").
func blocksText(bs []docBlock) string {
	var b strings.Builder
	runsText := func(rs []mrun) string {
		var t strings.Builder
		for _, r := range rs {
			t.WriteString(r.text)
			if r.kind == runLink && r.text != r.target && strings.HasPrefix(r.target, "http") {
				t.WriteString(" — " + r.target) // a web link whose text is not its address
			}
		}
		return t.String()
	}
	for _, bl := range bs {
		switch bl.kind {
		case dkTitle, dkPara:
			b.WriteString(runsText(bl.runs) + "\n")
		case dkBullet:
			b.WriteString("• " + runsText(bl.runs) + "\n")
		case dkStatus:
			b.WriteString(statusMark(bl.status) + " " + statusText(bl.status) + " — " + bl.summary + "\n")
		case dkFields:
			for _, r := range bl.rows {
				b.WriteString(r.label + labelSep() + runsText(r.value) + "\n")
			}
		case dkCmds:
			for _, c := range bl.cmds {
				b.WriteString("    " + c + "\n")
			}
		case dkSpace:
			b.WriteString("\n")
		}
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

// packageRuns shows the package a software was installed with, as a link to its page.
func packageRuns(it Item) []mrun {
	if it.Group == GroupWindows || len(it.Packages) == 0 {
		return nil
	}
	p := it.Packages[0]
	return []mrun{{kind: runLink, text: p.ID + " [" + p.Manager + "]", target: p.Page}}
}
