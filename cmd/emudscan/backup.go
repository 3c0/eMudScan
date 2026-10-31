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

// backupCmd keeps a copy of file, named after the time the command runs, so that
// commands run twice do not replace the first copy.
func backupCmd(file string) string {
	return cmd(fmt.Sprintf(`Copy-Item -LiteralPath %s -Destination (%s + (Get-Date -Format 'yyyyMMdd-HHmmss') + '.bak')`, psQuote(file), psQuote(file+".emudscan-")))
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

// restoreAdvice proposes to put back, in place of each file, its most recent backup by
// eMudScan's commands; nothing when there is none. The copy is moved over the file, so
// that each restore undoes one change.
func restoreAdvice(files ...string) string {
	var list, cmds, changed []string
	for _, f := range files {
		b, ok := latestBackup(f)
		if !ok {
			continue
		}
		list = append(list, "• "+lk(b.path)+" — "+Tf("restore.when", longDate(b.at), b.at.Format("15:04")))
		cmds = append(cmds, cmd(fmt.Sprintf(`Move-Item -LiteralPath %s -Destination %s -Force`, psQuote(b.path), psQuote(f))))
		changed = append(changed, f)
	}
	if len(cmds) == 0 {
		return ""
	}
	intro, how, warn := T("restore.intro1"), T("restore.how1"), T("restore.warn1")
	if len(cmds) > 1 {
		intro, how, warn = T("restore.introN"), T("restore.howN"), T("restore.warnN")
	}
	return intro + "\n" + strings.Join(list, "\n") + "\n\n" + how + "\n" + warn + "\n" + psHeader(changed...) + "\n" + strings.Join(cmds, "\n")
}
