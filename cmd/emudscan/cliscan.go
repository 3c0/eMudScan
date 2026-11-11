package main

// The scan without the window, for administrators: eMudScan-cli.exe, a console
// program (main_cli_windows.go), scans, then exits with a code that gives the worst
// status found. It asks nothing, never runs a fix, and connects to nothing unless
// --online is given. The log goes to the standard error output as the scan runs; then
// the reports are written in the files that --report names or, without any, the text
// report goes to the standard output.

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"time"
)

// The exit codes of eMudScan-cli.exe: the worst status found, or exitFailed when
// the scan could not run or a report could not be written (never mistaken for a
// result).
const (
	exitOK       = 0 // OK, or information only
	exitWarning  = 1
	exitUpdate   = 2
	exitCritical = 3
	exitFailed   = 10
)

// exitCode is the exit code for the worst status found.
func exitCode(worst Status) int {
	switch worst {
	case StOK, StInfo:
		return exitOK
	case StCaution:
		return exitWarning
	case StWarn:
		return exitUpdate
	case StError:
		return exitCritical
	}
	return exitFailed // no result
}

// scanDrives are the drives that eMudScan-cli.exe reads: those asked for, which
// must all exist (a drive left out would make the result look better than it is);
// without --drives, the local drives, which the window ticks. Never nil.
func scanDrives(asked []string, have []driveInfo) ([]string, error) {
	out := []string{}
	if asked == nil {
		for _, d := range have {
			if d.Fixed {
				out = append(out, d.Root)
			}
		}
		return out, nil
	}
	for _, root := range asked {
		found := false
		for _, d := range have {
			found = found || strings.EqualFold(d.Root, root)
		}
		if !found {
			return nil, errors.New(Tf("cli.noDrive", strings.TrimSuffix(root, `\`)))
		}
		out = append(out, root)
	}
	return out, nil
}

// scanLog is the Reporter of eMudScan-cli.exe: it writes the log as it comes, laid
// out as the log of the window is in a report, and keeps it for the reports.
type scanLog struct {
	mu      sync.Mutex
	out     io.Writer
	text    strings.Builder
	pending bool // the line of a step is started: its result ends it
}

func (l *scanLog) write(s string) {
	l.text.WriteString(s)
	io.WriteString(l.out, s)
}

func (l *scanLog) Log(kind LogKind, st Status, text string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if kind != LogEnd && l.pending {
		l.write("\r\n")
		l.pending = false
	}
	switch kind {
	case LogSection:
		l.write("\r\n" + text + "\r\n")
	case LogStart:
		l.write(text)
		l.pending = true
	case LogNote:
		l.write("\r\n" + statusMark(st) + " " + text + "\r\n")
	case LogEnd:
		prefix := " "
		if !l.pending {
			prefix = "   "
		}
		l.write(prefix + statusMark(st) + " " + text + "\r\n")
		l.pending = false
	default:
		if st != StPending {
			l.write("   " + statusMark(st) + " ")
		}
		l.write(text + "\r\n")
	}
}

func (l *scanLog) Upsert(Item)     {}
func (l *scanLog) Activity(string) {}

// String is the log so far.
func (l *scanLog) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.text.String()
}

// consoleText is a text as it is written in a console. Terminals draw the marks ℹ
// and ⚠ as emoji, wider than the place they give them: over the space that follows.
// One more space keeps the mark apart from the text.
func consoleText(s string) string { return consoleMarks.Replace(s) }

var consoleMarks = strings.NewReplacer(statusMark(StInfo)+" ", statusMark(StInfo)+"  ", statusMark(StCaution)+" ", statusMark(StCaution)+"  ")

// consoleWriter writes in a console, as consoleText lays the text out. Each Write
// must be a whole text.
type consoleWriter struct{ w io.Writer }

func (c consoleWriter) Write(p []byte) (int, error) {
	if _, err := io.WriteString(c.w, consoleText(string(p))); err != nil {
		return 0, err
	}
	return len(p), nil
}

// usage is the help of eMudScan-cli.exe (--help).
func usage() string {
	return strings.ReplaceAll(appName+" "+appVersionDate()+" · "+appURL+"\n\n"+T("cli.usage")+"\n", "\n", "\r\n")
}

// workerPanic, when set, is called with the panic of a goroutine that the checks
// start (to read files and folders at once). Go ends a program whose goroutine
// panics with the exit code 2, which eMudScan-cli.exe gives to "update needed": its
// scan sets workerPanic, to say what happened and exit with exitFailed. The window
// does not set it: for it, nothing changes.
var workerPanic func(r any)

// guardWorker is deferred in the goroutines that the checks start, after their other
// deferred calls, so that it runs before them.
func guardWorker() {
	if r := recover(); r != nil {
		if workerPanic != nil {
			workerPanic(r)
		}
		panic(r)
	}
}

// scanCrashed says on stderr that the scan stopped on an error of eMudScan itself,
// with where (for a bug report), and gives its exit code.
func scanCrashed(stderr io.Writer, r any) int {
	return scanFailed(stderr, fmt.Sprintf("Internal error: %v\r\n%s", r, strings.ReplaceAll(string(debug.Stack()), "\n", "\r\n")))
}

// scanFailed says on stderr why the scan could not run, and gives its exit code.
func scanFailed(stderr io.Writer, reason string) int {
	fmt.Fprintf(stderr, "%s\r\n", reason)
	return exitFailed
}

// runScan runs the scan of eMudScan-cli.exe with the options o and returns its exit
// code.
func runScan(o cliOptions, stdout, stderr io.Writer) (code int) {
	drives, err := scanDrives(o.drives, listDrives())
	if err != nil {
		return scanFailed(stderr, err.Error())
	}
	for _, path := range o.reports { // before the scan, which can be long
		if fi, err := os.Stat(filepath.Dir(path)); err != nil || !fi.IsDir() {
			return scanFailed(stderr, Tf("cli.noFolder", path))
		}
	}
	log := &scanLog{out: stderr}
	c := newChecker(log, nil)
	c.offline = !o.online
	workerPanic = func(r any) { os.Exit(scanCrashed(stderr, r)) }
	defer func() {
		if r := recover(); r != nil {
			code = scanCrashed(stderr, r)
		}
	}()
	runAllChecks(c, drives, nil)
	return finishScan(c, log.String(), o, stdout, stderr)
}

// finishScan writes the reports of a scan that ended: in the files asked for, their
// types given by their extensions (text for another one), replacing those that
// exist; without any, the text report on stdout, as Copy report gives it, after an
// empty line on stderr. It returns the exit code: the worst status, unless a report
// could not be written.
func finishScan(c *checker, logText string, o cliOptions, stdout, stderr io.Writer) int {
	now := time.Now()
	for _, path := range o.reports {
		_, format, _ := saveTarget(path, formatText, noFormat)
		data, err := reportFile(c, logText, format, now)
		if err == nil {
			err = os.WriteFile(path, data, 0o644)
		}
		if err != nil {
			return scanFailed(stderr, Tf("saveError", err))
		}
		fmt.Fprintf(stderr, "%s\r\n", Tf("cli.saved", path))
	}
	if len(o.reports) == 0 {
		// in a console, the report comes right after the log: an empty line sets them
		// apart (on stderr, so that a report redirected to a file starts with its title)
		io.WriteString(stderr, "\r\n")
		if _, err := io.WriteString(stdout, buildReport(c, logText)); err != nil {
			return scanFailed(stderr, Tf("saveError", err))
		}
	}
	code := exitCode(c.counts().worst())
	if code == exitFailed {
		return scanFailed(stderr, T("cli.noResult"))
	}
	return code
}
