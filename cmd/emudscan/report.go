package main

import (
	"fmt"
	"strings"
	"time"
)

// runAllChecks runs every check. extraRoots is a test hook (additional folders to scan).
func runAllChecks(c *checker, drives []string, extraRoots []string) {
	start := time.Now()
	c.line(StPending, Tf("log.start", appName, appVersionDate(), windowsVersionString(), time.Now().Format("2006-01-02 15:04")))
	if defaultEmbedScanner == nil {
		defaultEmbedScanner = buildEmbedScanner()
	}
	blocked, risk := runWindowsChecks(c)
	if !c.cancel() {
		runThirdPartyChecks(c, drives, blocked, risk, extraRoots)
	}
	if c.cancel() {
		c.line(StInfo, T("log.cancelled"))
		return
	}
	c.section(c.counts().text("log.done", fmtElapsed(time.Since(start))))
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
// texts: emoji, shown in colour by Notepad++ and the Windows 11 Notepad.
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

// buildReport renders all results and the log as plain text (CRLF line ends).
func buildReport(c *checker, logText string) string {
	var b strings.Builder
	line := strings.Repeat("=", 78)
	fmt.Fprintf(&b, "%s\n%s %s — %s — %s\n%s\n%s\n\n", line, T("report.title"), appVersionDate(), windowsVersionString(), time.Now().Format("2006-01-02 15:04"),
		Tf("report.by", appAuthor, appURL), line)
	items := mergeRepeated(c.snapshot())
	for _, g := range []Group{GroupWindows, GroupNet, GroupApps} {
		title := groupTitle(g)
		fmt.Fprintf(&b, "%s\n%s\n", title, strings.Repeat("-", len([]rune(title))))
		for _, it := range sortedForReport(items, g) {
			fmt.Fprintf(&b, "%s %s — %s — %s\n", statusMark(it.Status), itemTitle(it), statusText(it.Status), it.Summary)
			if g != GroupWindows {
				if u := it.Updated; u.shown() {
					stale := ""
					if m := it.stale(); m > 0 {
						stale = " — " + Tf("stale.short", ageText(m))
					}
					fmt.Fprintf(&b, "      %s%s%s%s\n", T("dlg.updated"), labelSep(), u.text(u.File), stale)
				}
				file, folder := it.File, it.Folder
				if len(it.Copies) > 0 { // the copies are listed below
					file, folder = "", ""
				}
				for _, f := range []struct{ k, v string }{{it.fileLabel(), file}, {T("col.folder"), folder}, {T("dlg.homepage"), it.Homepage()}} {
					if f.v != "" {
						fmt.Fprintf(&b, "      %s%s%s\n", f.k, labelSep(), f.v)
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
		b.WriteString("\n")
	}
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
