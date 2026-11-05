//go:build windows

package main

// Restoring a backup, from the details of a fixed software ("Restore the backup"): a
// warning, then eMudScan moves each backup back in place of its file, as the command
// that the details show would (restoreAdvice), and a message says whether the files
// are back. eMudScan moves them itself (moveBackups), but for files that need
// administrator rights, such as those in Program Files: their commands run in Windows
// PowerShell as administrator, as the fixes do, since eMudScan itself never runs as
// administrator. The report keeps what was done, as for the fixes.

import (
	"fmt"
	"time"
)

// Events of a restore (uiEvent.kind).
const (
	evRestorePrompt = 8 // the administrator prompt is answered
	evRestoreDone   = 9 // the restore has ended: text says how, st is StOK when all is restored
)

// restoreBackups asks whether to restore the backups of the software of the details
// d, then restores them in the background; the details are disabled until it ends
// (restoreEvent).
func (a *ui) restoreBackups(d *detailsDlg) {
	if a.restoring || a.fix != nil || a.current == nil || a.running {
		return
	}
	ids := d.item.memberIDs()
	items := a.restoreItems(ids)
	if len(items) == 0 {
		return
	}
	files, admin := 0, false
	for _, it := range items {
		for _, f := range it.Restores {
			files += len(f.Files)
			admin = admin || f.Admin
		}
	}
	title := itemTitle(d.item)
	question, text := T("restore.confirm1"), Tf("restore.warning1", title)
	if files > 1 {
		question, text = T("restore.confirmN"), Tf("restore.warningN", title)
	}
	if admin {
		text += "\n\n" + T("restore.admin")
	}
	text += "\n\n" + T("restore.warningEnd")
	if !askWarning(d.hwnd, question, text, T("restore.accept"), T("fix.decline")) {
		return
	}
	a.restoring = true
	setText(d.status, T("restore.running"))
	a.setDetailsFix(d)
	pEnableWindow.Call(d.hwnd, 0) // nothing else until it ends: not even closing the details
	c := a.current
	var lines []fixLine // what the runner reports, for the message at the end
	out := func(l fixLine) {
		c.recordFix(l)
		lines = append(lines, l)
	}
	if len(c.fixRecord()) > 0 {
		c.recordFix(fixLine{lineNote, ""})
	}
	c.recordFix(fixLine{lineNote, Tf("fix.started", time.Now().Format("2006-01-02 15:04"), Tf("restore.title", title))})
	logged := false
	r := &fixRunner{
		exec: func(units []fixUnit, admin bool, out func(fixLine)) ([]unitResult, error) {
			if admin {
				return runFixes(units, true, out)
			}
			return moveBackups(units, out), nil
		},
		item: c.get,
		recheck: func(ids []int) {
			if !logged { // the log of the main window shows the checks after it in their own section
				logged = true
				c.section(T("restore.logSection"))
			}
			c.recheck(ids)
		},
		stopped:   func() bool { return false },
		out:       out,
		fixed:     func(int) {},
		simulated: func(id int) { simulateRestored(c, id) },
	}
	fixPromptOwner = d.hwnd
	fixPromptClosed = func() { postEvent(uiEvent{kind: evRestorePrompt}) }
	go func() {
		defer func() {
			if p := recover(); p != nil {
				out(fixLine{lineError, fmt.Sprintf("Internal error: %v", p)})
			}
			out(fixLine{lineNote, T("restore.end")})
			msg, ok := restoreOutcome(lines)
			st := StWarn
			if ok {
				st = StOK
			}
			postEvent(uiEvent{kind: evRestoreDone, text: msg, st: st})
		}()
		r.restore(ids)
	}()
}

// restoreEvent shows what a restore reports (from processEvents).
func (a *ui) restoreEvent(e uiEvent) {
	d := a.dlg
	switch e.kind {
	case evRestorePrompt: // Windows may have given the foreground to another window
		if d != nil {
			bringToFront(d.hwnd)
		}
	case evRestoreDone:
		a.restoring = false
		if d == nil {
			return
		}
		pEnableWindow.Call(d.hwnd, 1)
		a.refreshDetails() // the details as the restore left the software
		setText(d.status, "")
		bringToFront(d.hwnd)
		icon := uintptr(0x30) // MB_ICONWARNING
		if e.st == StOK {
			icon = 0x40 // MB_ICONINFORMATION
		}
		msgBox(d.hwnd, e.text, icon)
	}
}
