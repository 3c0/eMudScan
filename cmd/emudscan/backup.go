package main

// eMudScan's backups: before changing a file, the commands that eMudScan proposes keep
// a copy of it next to it, named after the file and the time the commands ran
// (cacert.pem.emudscan-20261031-011500.bak), which tells it from the copies that other
// programs keep. While one is there, eMudScan proposes to put it back.

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// backupThen is a command that keeps a copy of file, named after the time the command
// runs (so that commands run twice do not replace the first copy), then changes the
// file with change, only once the copy is made and when cond, if any, is true.
// Copy-Item -PassThru returns the copy, and nothing when it fails.
func backupThen(file, cond, change string) string {
	return cmd(backupThenLine(file, cond, change, needsAdmin(file)))
}

// backupThenLine is the command of backupThen, without its "$ " mark, run as
// administrator or not (admin).
func backupThenLine(file, cond, change string, admin bool) string {
	backup := fmt.Sprintf(`(Copy-Item -LiteralPath %s -Destination (%s + (Get-Date -Format 'yyyyMMdd-HHmmss') + '.bak') -PassThru)`, psPath(file, admin), psPath(file+".emudscan-", admin))
	if cond != "" {
		backup = cond + " -and " + backup
	}
	return fmt.Sprintf(`if (%s) { %s }`, backup, change)
}

// changeLine is the same change without the backup, when cond, if any, is true: what
// the Fix function runs when "Back up the certificate files" is cleared.
func changeLine(cond, change string) string {
	if cond == "" {
		return change
	}
	return fmt.Sprintf(`if (%s) { %s }`, cond, change)
}

var backupSuffixRe = regexp.MustCompile(`(?i)^\.emudscan-(\d{8}-\d{6})\.bak$`)

type backup struct {
	path string
	at   time.Time // when it was made
}

// latestBackup is the most recent of eMudScan's backups of file that differs from it:
// the file as it was before the last change by eMudScan's commands. A copy that is the
// same as the file, as when the commands ran twice, is passed over.
func latestBackup(file string) (backup, bool) {
	if file == "" {
		return backup{}, false
	}
	dir, base := filepath.Dir(file), filepath.Base(file)
	var found []backup
	for _, e := range readDir(dir) {
		n := e.Name()
		if !e.Type().IsRegular() || len(n) <= len(base) || !strings.EqualFold(n[:len(base)], base) {
			continue
		}
		m := backupSuffixRe.FindStringSubmatch(n[len(base):])
		if m == nil {
			continue
		}
		at, err := time.ParseInLocation("20060102-150405", m[1], time.Local)
		if err != nil {
			continue
		}
		found = append(found, backup{filepath.Join(dir, n), at})
	}
	if len(found) == 0 {
		return backup{}, false
	}
	cur, err := os.ReadFile(file)
	if err != nil {
		return backup{}, false
	}
	sort.Slice(found, func(i, j int) bool { return found[i].at.After(found[j].at) })
	for _, b := range found {
		if data, err := os.ReadFile(b.path); err == nil && !bytes.Equal(data, cur) {
			return b, true
		}
	}
	return backup{}, false
}

// moveBackups runs restores that need no administrator rights without Windows
// PowerShell: it moves each backup back in place of its file itself, as the command
// shown would (Move-Item -Force), and reports each move as the runner reports a
// command. The simulation moves nothing.
func moveBackups(units []fixUnit, out func(fixLine)) []unitResult {
	res := make([]unitResult, len(units))
	for i, u := range units {
		res[i] = unitResult{ran: true, ok: true}
		for j, file := range u.fix.Files {
			if j >= len(u.fix.From) {
				break
			}
			out(fixLine{lineCommand, Tf("restore.move", shownPath(u.fix.From[j]), shownPath(file))})
			if fixSimulation {
				out(fixLine{lineOK, "✓ " + simulatedResult(false)})
				continue
			}
			if err := os.Rename(u.fix.From[j], file); err != nil { // MoveFileEx, replacing the file
				out(fixLine{lineFailed, "✗ " + T("fix.cmdFailed")})
				out(fixLine{lineOutput, err.Error()})
				res[i].ok = false
				continue
			}
			out(fixLine{lineOK, "✓ " + T("fix.cmdOK")})
		}
	}
	return res
}

// restoreAdvice proposes to put back, in place of each file, its most recent backup by
// eMudScan's commands; nothing when there is none. The copy is moved over the file, so
// that each restore undoes one change. The commands are recorded as a restore, which
// the Restore button of the details runs (fixes.go).
func restoreAdvice(files ...string) string {
	var list, changed, refused []string
	var backups []backup
	for _, f := range files {
		b, ok := latestBackup(f)
		if !ok {
			continue
		}
		if why := restoreUnsafe(f, b.path); why != "" {
			refused = append(refused, why)
			continue
		}
		list = append(list, "• "+lk(b.path)+" — "+Tf("restore.when", longDate(b.at), b.at.Format("15:04")))
		changed = append(changed, f)
		backups = append(backups, b)
	}
	if len(changed) == 0 {
		return joinParas(refused...)
	}
	admin := anyNeedsAdmin(changed...) // as psHeader says
	var cmds []string
	for i, f := range changed {
		cmds = append(cmds, fmt.Sprintf(`Move-Item -LiteralPath %s -Destination %s -Force`, psPath(backups[i].path, admin), psPath(f, admin)))
	}
	from := make([]string, len(backups))
	for i, b := range backups {
		from[i] = b.path
	}
	restore := newFix(fixAction{Kind: fixRestore, Admin: admin, Cmds: cmds, Files: changed, From: from})
	intro, how, warn := T("restore.intro1"), T("restore.how1"), T("restore.warn1")
	if len(cmds) > 1 {
		intro, how, warn = T("restore.introN"), T("restore.howN"), T("restore.warnN")
	}
	return joinParas(intro+"\n"+strings.Join(list, "\n")+"\n\n"+how+"\n"+warn+"\n"+psHeader(changed...)+"\n"+restore.lines(), joinParas(refused...))
}
