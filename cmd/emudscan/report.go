package main

import (
	"fmt"
	"os"
	"strings"
	"time"
	"unicode"
)

// runAllChecks runs every check. extraRoots is a test hook (additional folders to scan).
func runAllChecks(c *checker, drives []string, extraRoots []string) {
	start := time.Now()
	c.setScan(scanRun{drives: drives})
	c.line(StPending, Tf("log.start", appName, appVersionDate(), windowsVersionString(), time.Now().Format("2006-01-02 15:04")))
	if drives != nil && len(drives) == 0 { // no drive ticked in the window, or --drives none
		c.note(StInfo, T("log.noDrive"))
	}
	if defaultEmbedScanner == nil {
		defaultEmbedScanner = buildEmbedScanner()
	}
	blocked, risk := runWindowsChecks(c)
	if !c.cancel() {
		runThirdPartyChecks(c, drives, blocked, risk, extraRoots)
	}
	if c.cancel() {
		c.setScan(scanRun{drives: drives, took: time.Since(start)})
		c.line(StInfo, T("log.cancelled"))
		return
	}
	took := time.Since(start)
	c.setScan(scanRun{drives: drives, took: took, complete: true})
	c.section(c.counts().text("log.done", fmtElapsed(took)))
}

func fmtElapsed(d time.Duration) string {
	s := int(d.Round(time.Second).Seconds())
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
	}
	return fmt.Sprintf("%02d:%02d", s/60, s%60)
}

func statusText(s Status) string {
	switch s {
	case StOK:
		return T("status.ok")
	case StInfo:
		return T("status.info")
	case StWarn:
		return T("status.warn")
	case StCaution:
		return T("status.caution")
	case StError:
		return T("status.error")
	}
	return T("status.pend")
}

// statusMark is the symbol of a status in the progress log, the reports and copied
// texts: emoji, shown in colour by the Windows 11 Notepad and other editors.
func statusMark(s Status) string {
	switch s {
	case StOK:
		return "✅"
	case StInfo:
		return "ℹ"
	case StCaution:
		return "⚠"
	case StWarn:
		return "❗"
	case StError:
		return "❌"
	}
	return "·"
}

// logMark is the symbol of a status in the progress log of the window. The log is
// drawn with GDI, which draws emoji in one colour only, and in one colour
// ✅ ("white heavy check mark") is hollow: the log shows the solid ✔ instead, in
// green. The saved report has ✅ (reportMark).
func logMark(s Status) string {
	if s == StOK {
		return "✔"
	}
	return statusMark(s)
}

// reportMark converts a mark of the log to the mark of the reports.
func reportMark(m string) string {
	if m == logMark(StOK) {
		return statusMark(StOK)
	}
	return m
}

// computerName is the name of this PC, shown at the top of the report so that one
// can tell on which PC a report was made; empty when Windows does not give it.
var computerName = func() string {
	name, err := os.Hostname()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(name)
}

// reportFileName is the name of a report saved at t, without its extension: with the
// name of the PC, so that the reports of several PCs can be kept together, and with
// the seconds, so that two reports saved in the same minute have different names.
func reportFileName(t time.Time) string {
	name := "eMudScan-report-"
	if pc := fileNamePart(computerName()); pc != "" {
		name += pc + "-"
	}
	return name + t.Format("20060102-150405")
}

// fileNamePart is the name of a PC as a file name has it. Windows allows only
// letters, digits and "-" in the name of a PC, so it is nearly always the name itself.
// Any other character becomes "-", as Windows does when it registers the name on
// the network, and no "-" is left at the start or at the end.
func fileNamePart(pc string) string {
	return strings.Trim(strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsMark(r) || unicode.IsDigit(r) {
			return r
		}
		return '-'
	}, pc), "-")
}

// buildReport renders all results and the log as plain text (CRLF line ends).
func buildReport(c *checker, logText string) string {
	var b strings.Builder
	line := strings.Repeat("=", 78)
	// eMudScan's version and the date, the PC and its Windows, then who made eMudScan
	pc := windowsVersionString()
	if name := computerName(); name != "" {
		pc = name + " · " + pc
	}
	fmt.Fprintf(&b, "%s\n%s %s · %s\n%s\n%s\n%s\n\n", line, T("report.title"), appVersionDate()+simulationMark, time.Now().Format("2006-01-02 15:04"),
		pc, Tf("report.by", appAuthor, appURL), line)
	// the sections are separated by the bar of the fixes (fixBar), with one empty line
	// before and after it; once, after the fixes, which end with it
	separate := func() {
		s := strings.TrimRight(b.String(), "\n")
		b.Reset()
		b.WriteString(s + "\n\n")
		if !strings.HasSuffix(s, fixBar) {
			b.WriteString(fixBar + "\n\n")
		}
	}
	items := mergeRepeated(c.snapshot())
	for i, g := range []Group{GroupWindows, GroupNet, GroupApps} {
		if i > 0 {
			separate()
		}
		title := groupTitle(g)
		fmt.Fprintf(&b, "%s\n%s\n", title, strings.Repeat("-", len([]rune(title))))
		for _, it := range sortedForReport(items, g) {
			fmt.Fprintf(&b, "%s %s — %s — %s\n", statusMark(it.Status), itemTitle(it), statusText(it.Status), it.Summary)
			if g != GroupWindows {
				stale := ""
				if m := it.stale(); m > 0 {
					stale = " — " + Tf("stale.short", ageText(m))
				}
				if u := it.Updated; u.shown() && !u.mozilla() { // a bundle on its own: the date curl wrote, below
					fmt.Fprintf(&b, "      %s%s%s%s\n", T("dlg.updated"), labelSep(), u.text(shownPath(u.File)), stale)
				}
				file, folder := it.File, it.Folder
				if len(it.Copies) > 0 { // the copies are listed below
					file, folder = "", ""
				}
				for _, f := range []struct{ k, v string }{{it.fileLabel(), file}, {T("col.folder"), folder}, {T("dlg.homepage"), it.Homepage()}} {
					if f.v != "" {
						fmt.Fprintf(&b, "      %s%s%s\n", f.k, labelSep(), shownPath(f.v))
					}
					if d := it.Data; f.v != "" && f.v == file && d.known() { // under the bundle: the date curl wrote in it
						age := ""
						if it.Updated.mozilla() {
							age = stale
						}
						fmt.Fprintf(&b, "      %s%s\n", d.text(""), age)
					}
				}
				if len(it.Packages) > 0 {
					p := it.Packages[0]
					fmt.Fprintf(&b, "      %s%s%s [%s] — %s\n", T("dlg.package"), labelSep(), p.ID, p.Manager, p.Page)
				}
			}
			if details := joinParas(staleAdvice(it), it.shownDetails()); details != "" { // the same explanation as in the details window
				b.WriteString("\n")
				for _, l := range strings.Split(plainText(details), "\n") {
					fmt.Fprintf(&b, "      %s\n", l)
				}
				b.WriteString("\n")
			}
		}
	}
	if fixes := c.fixRecord(); len(fixes) > 0 { // what the fixes ran and showed, as the Fix window did
		separate()
		fmt.Fprintf(&b, "%s\n%s\n", T("report.fixes"), strings.Repeat("-", len([]rune(T("report.fixes")))))
		for _, l := range reportLines(fixes) {
			fmt.Fprintf(&b, "%s\n", l)
		}
	}
	separate()
	fmt.Fprintf(&b, "%s\n%s\n%s\n", T("report.log"), strings.Repeat("-", len([]rune(T("report.log")))), logText)
	return strings.ReplaceAll(strings.ReplaceAll(b.String(), "\r\n", "\n"), "\n", "\r\n")
}

// itemTitle names an element in one line: "Program › Component version".
func itemTitle(it Item) string {
	if it.Group == GroupWindows {
		return it.Name
	}
	prog := it.DisplayName()
	if it.Component == "" {
		return prog
	}
	return prog + " › " + strings.TrimSpace(it.Component+" "+it.Version)
}

func sortedForReport(items []Item, g Group) []Item {
	var out []Item
	for _, it := range items {
		if it.Group == g {
			out = append(out, it)
		}
	}
	if g != GroupWindows {
		sortItems(out, []sortKey{{col: colStatus}, {col: colProgram}})
	}
	return out
}

func groupTitle(g Group) string {
	switch g {
	case GroupNet:
		return T("grp.net")
	case GroupApps:
		return T("grp.apps")
	}
	return T("grp.windows")
}
