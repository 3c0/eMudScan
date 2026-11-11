package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// Without --drives, the scan reads the local drives, as the window ticks them; a
// drive asked for that does not exist stops it.
func TestScanDrives(t *testing.T) {
	Lang = 0
	have := []driveInfo{{Root: `C:\`, Fixed: true}, {Root: `D:\`}, {Root: `E:\`, Fixed: true}}
	if got, err := scanDrives(nil, have); err != nil || !reflect.DeepEqual(got, []string{`C:\`, `E:\`}) {
		t.Errorf("default: %q, %v", got, err)
	}
	if got, err := scanDrives(nil, nil); err != nil || got == nil || len(got) != 0 {
		t.Errorf("no drive at all: %q, %v", got, err)
	}
	if got, err := scanDrives([]string{`D:\`}, have); err != nil || !reflect.DeepEqual(got, []string{`D:\`}) {
		t.Errorf("asked: %q, %v", got, err)
	}
	if got, err := scanDrives([]string{}, have); err != nil || got == nil || len(got) != 0 {
		t.Errorf("none: %q, %v", got, err)
	}
	if _, err := scanDrives([]string{`C:\`, `Q:\`}, have); err == nil || err.Error() != "The drive Q: was not found" {
		t.Errorf("missing: %v", err)
	}
}

func TestExitCode(t *testing.T) {
	for st, want := range map[Status]int{StOK: 0, StInfo: 0, StCaution: 1, StWarn: 2, StError: 3, StPending: 10} {
		if got := exitCode(st); got != want {
			t.Errorf("exitCode(%v) = %d, want %d", st, got, want)
		}
	}
}

// The log of a command-line scan is written as it comes, laid out as the log of the
// window is in a report.
func TestScanLog(t *testing.T) {
	var out bytes.Buffer
	l := &scanLog{out: &out}
	l.Log(LogLine, StPending, "eMudScan")
	l.Log(LogSection, StPending, "Windows certificate system")
	l.Log(LogStart, StPending, "Windows block list …")
	l.Log(LogEnd, StOK, "Up to date")
	l.Log(LogStart, StPending, "Scanning C:\\ …")
	l.Log(LogLine, StCaution, "A folder could not be read")
	l.Log(LogEnd, StWarn, "2 folders")
	l.Log(LogLine, StInfo, "Scan interrupted.")
	want := "eMudScan\r\n\r\nWindows certificate system\r\nWindows block list … ✅ Up to date\r\nScanning C:\\ …\r\n" +
		"   ⚠ A folder could not be read\r\n   ❗ 2 folders\r\n   ℹ Scan interrupted.\r\n"
	if l.String() != want || out.String() != want {
		t.Errorf("log:\n%q\nwritten:\n%q\nwant:\n%q", l.String(), out.String(), want)
	}
}

// In a console, the marks that terminals draw wider than their place get one more
// space; the others, and a text written in a file, do not change.
func TestConsoleText(t *testing.T) {
	for in, want := range map[string]string{
		"Windows TLS test (online) … ℹ Not run\r\n": "Windows TLS test (online) … ℹ  Not run\r\n",
		"   ⚠ A folder could not be read":           "   ⚠  A folder could not be read",
		"ℹ Windows TLS test — Information":          "ℹ  Windows TLS test — Information",
		"Windows block list … ✅ Up to date":         "Windows block list … ✅ Up to date",
		"❗ Example App 22 — Update needed":          "❗ Example App 22 — Update needed",
		"ℹ":                                         "ℹ",
	} {
		if got := consoleText(in); got != want {
			t.Errorf("consoleText(%q) = %q, want %q", in, got, want)
		}
	}
	var out bytes.Buffer
	if n, err := (consoleWriter{&out}).Write([]byte("ℹ Not run\r\n")); err != nil || n != len("ℹ Not run\r\n") || out.String() != "ℹ  Not run\r\n" {
		t.Errorf("consoleWriter: %d, %v, %q", n, err, out.String())
	}
}

// The help names every option, in both languages, with CRLF line ends.
func TestUsage(t *testing.T) {
	defer func() { Lang = 0 }()
	for Lang = 0; Lang < 2; Lang++ {
		u := usage()
		for _, opt := range []string{"eMudScan-cli.exe", "--report", "--drives", "--online", "--lang en|fr", "--help", " 10 "} {
			if !strings.Contains(u, opt) {
				t.Errorf("language %d: the help does not name %q:\n%s", Lang, opt, u)
			}
		}
		if !strings.HasPrefix(u, appName+" "+appVersionDate()) || strings.Count(u, "\n") != strings.Count(u, "\r\n") || !strings.HasSuffix(u, "\r\n") {
			t.Errorf("language %d: first line or line ends:\n%q", Lang, u)
		}
	}
}

// A panic in a goroutine that the checks start is given to the command-line scan,
// which exits with 10: left alone, it would end eMudScan with the exit code 2, the
// code of "update needed".
func TestGuardWorker(t *testing.T) {
	defer func() { workerPanic = nil }()
	var got any
	workerPanic = func(r any) {
		got = r
		runtime.Goexit() // the scan exits here
	}
	run := func(work func()) {
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer guardWorker()
			work()
		}()
		wg.Wait()
	}
	if run(func() {}); got != nil {
		t.Errorf("without a panic: %v", got)
	}
	if run(func() { panic("boom") }); got != "boom" {
		t.Errorf("with a panic: %v", got)
	}
}

// A note about the whole scan (no drive to read) comes below the first line of the
// log, after an empty line and without the indent of the results; the title of the
// first check follows after another empty line.
func TestLogNote(t *testing.T) {
	var out bytes.Buffer
	c := newChecker(&scanLog{out: &out}, nil)
	c.line(StPending, "eMudScan")
	c.note(StInfo, "No drive selected")
	c.section("Windows certificate system")
	if want := "eMudScan\r\n\r\nℹ No drive selected\r\n\r\nWindows certificate system\r\n"; out.String() != want {
		t.Errorf("log:\n%q\nwant:\n%q", out.String(), want)
	}
}

// The line of an item in the log says where it is: a program, in its folder; a file
// (a certificate bundle, the file of a setting) or the value of a variable, after a
// dash; nothing when it is not known.
func TestStepLabel(t *testing.T) {
	defer func() { Lang = 0 }()
	for _, c := range []struct {
		name string
		at   place
		en   string
		fr   string
	}{
		{"Java", place{path: `C:\Apps\jdk-25`}, `Java in C:\Apps\jdk-25`, `Java dans C:\Apps\jdk-25`},
		{"Certificate bundle", place{path: `C:\Apps\example-app-31\cacert.pem`, file: true}, `Certificate bundle — C:\Apps\example-app-31\cacert.pem`, `Certificate bundle — C:\Apps\example-app-31\cacert.pem`},
		{"Example App 31", place{}, "Example App 31", "Example App 31"},
	} {
		Lang = 0
		if got := stepLabel(c.name, c.at); got != c.en {
			t.Errorf("English: %q, want %q", got, c.en)
		}
		Lang = 1
		if got := stepLabel(c.name, c.at); got != c.fr {
			t.Errorf("French: %q, want %q", got, c.fr)
		}
	}
	Lang = 0
	var out bytes.Buffer
	c := newChecker(&scanLog{out: &out}, nil)
	c.finish(c.begin(GroupApps, "app", "Example App 31", `C:\Apps\example-app-31`, nil), result{Status: StOK, Summary: "Checked"})
	c.finish(c.beginFile(GroupApps, "bundle", "certifi", `C:\Apps\example-app-31\certifi\cacert.pem`, nil), result{Status: StOK, Summary: "Checked"})
	c.finish(c.beginFile(GroupApps, "setting", "SSL_CERT_FILE", `C:\Apps\certs\ca.pem`, nil), result{Status: StOK, Summary: "Checked"})
	want := "Example App 31 in C:\\Apps\\example-app-31 … ✅ Checked\r\n" +
		"certifi — C:\\Apps\\example-app-31\\certifi\\cacert.pem … ✅ Checked\r\n" +
		"SSL_CERT_FILE — C:\\Apps\\certs\\ca.pem … ✅ Checked\r\n"
	if out.String() != want {
		t.Errorf("log:\n%s\nwant:\n%s", out.String(), want)
	}
}

// At its end, a command-line scan writes the reports in the files asked for, their
// types given by their extensions, or the text report on stdout; its exit code is
// the worst status, or 10 when a report could not be written.
func TestFinishScan(t *testing.T) {
	c := exportChecker(t) // the worst status: update needed
	dir := t.TempDir()
	js, cs, tx, other := filepath.Join(dir, "pc.json"), filepath.Join(dir, "pc.CSV"), filepath.Join(dir, "pc.txt"), filepath.Join(dir, "pc.report")
	if err := os.WriteFile(js, []byte("old"), 0o644); err != nil { // replaced
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := finishScan(c, "the log\r\n", cliOptions{reports: []string{js, cs, tx, other}}, &stdout, &stderr); code != exitUpdate {
		t.Errorf("exit code %d\n%s", code, stderr.String())
	}
	if stdout.Len() != 0 || stderr.String() != "Report saved: "+js+"\r\nReport saved: "+cs+"\r\nReport saved: "+tx+"\r\nReport saved: "+other+"\r\n" {
		t.Errorf("stdout %q\nstderr %q", stdout.String(), stderr.String())
	}
	var doc reportDoc
	if data, err := os.ReadFile(js); err != nil || json.Unmarshal(data, &doc) != nil || len(doc.Items) != 4 || !reflect.DeepEqual(doc.Log, []string{"the log"}) {
		t.Errorf("JSON report: %v, %+v", err, doc)
	}
	if data, _ := os.ReadFile(cs); !bytes.HasPrefix(data, append(append([]byte{}, utf8BOM...), "pc,windows,"...)) {
		t.Errorf("CSV report: %.40q", data)
	}
	for _, path := range []string{tx, other} { // another extension: text
		if data, _ := os.ReadFile(path); !bytes.HasPrefix(data, append(append([]byte{}, utf8BOM...), "===="...)) || !bytes.HasSuffix(data, []byte("the log\r\n\r\n")) {
			t.Errorf("text report %s: %.40q", path, data)
		}
	}

	// without --report: the text report on stdout, without a byte order mark, after an
	// empty line on stderr, which sets it apart from the log in a console
	stdout.Reset()
	stderr.Reset()
	if code := finishScan(c, "the log\r\n", cliOptions{}, &stdout, &stderr); code != exitUpdate || stderr.String() != "\r\n" {
		t.Errorf("exit code %d, stderr %q", code, stderr.String())
	}
	if out := stdout.String(); !strings.HasPrefix(out, "====") || !strings.Contains(out, "Example App 22 4.2.0 [Scoop] › Node.js 25.1.0") || !strings.HasSuffix(out, "the log\r\n\r\n") {
		t.Errorf("stdout:\n%s", out)
	}

	// a report that cannot be written: 10, whatever the scan found
	stderr.Reset()
	missing := filepath.Join(dir, "missing", "pc.json")
	if code := finishScan(c, "", cliOptions{reports: []string{missing}}, &stdout, &stderr); code != exitFailed || !strings.HasPrefix(stderr.String(), "The report could not be saved: ") {
		t.Errorf("exit code %d, stderr %q", code, stderr.String())
	}

	// no result: 10
	stderr.Reset()
	if code := finishScan(newChecker(nopReporter{}, nil), "", cliOptions{reports: []string{filepath.Join(dir, "none.txt")}}, &stdout, &stderr); code != exitFailed ||
		!strings.HasSuffix(stderr.String(), "The scan gave no result\r\n") {
		t.Errorf("no result: exit code %d, stderr %q", code, stderr.String())
	}

	// the other statuses
	for st, want := range map[Status]int{StOK: exitOK, StInfo: exitOK, StCaution: exitWarning, StError: exitCritical} {
		c := newChecker(nopReporter{}, nil)
		c.add(GroupWindows, "windows", "Automatic certificate updates", result{Status: StOK, Summary: "Enabled"})
		c.add(GroupApps, "app", "Example App 26", result{Status: st, Summary: "Checked"})
		stdout.Reset()
		if code := finishScan(c, "", cliOptions{}, &stdout, &stderr); code != want {
			t.Errorf("status %v: exit code %d, want %d", st, code, want)
		}
	}
}
