package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// exportChecker is a checker with elements in every list, for the tests of the JSON
// and CSV reports: a check of Windows, a browser that is OK, a program with two
// copies of Node.js to update (installed with a package manager, with fixes), and a
// program whose name a spreadsheet would take for a formula.
func exportChecker(t *testing.T) *checker {
	t.Helper()
	Lang = 0
	saved := computerName
	computerName = func() string { return "MY-PC" }
	t.Cleanup(func() { computerName = saved })

	c := newChecker(nopReporter{}, nil)
	c.offline = true
	c.setScan(scanRun{drives: []string{`C:\`, `D:\`}, took: 4400 * time.Millisecond, complete: true})
	when := time.Date(2026, 3, 2, 9, 30, 0, 0, time.Local)
	c.dated = func(kind string, r result) lastUpdate { return lastUpdate{Time: when, Src: dateSigned, File: r.File} }
	c.afterFinish = func(it *Item) {
		if it.Program == "Example App 22" {
			it.Packages = []pkgRef{{Manager: "Scoop", ID: "example-app-22", Page: scoopPage("example-app-22")}}
		}
	}
	c.add(GroupWindows, "windows", "Automatic certificate updates", result{Status: StOK, Summary: "Enabled"})
	c.add(GroupNet, "browser", "Example App 21", result{Status: StOK, Summary: "Blocks the compromised certificates", Version: "9.4.1",
		File: `C:\Program Files\Example App 21\app21.exe`, Folder: `C:\Program Files\Example App 21`, ProductURL: "https://example.com/app-21"})
	update := newFix(fixAction{Kind: fixPackage, Cmds: []string{"scoop update example-app-22"}, Manager: "Scoop"})
	for _, dir := range []string{`C:\Tools\Example App 22\one`, `C:\Tools\Example App 22\two`} {
		file := dir + `\node.exe`
		replace := newFix(fixAction{Kind: fixNode, Admin: true, Optional: true,
			Cmds: []string{"Copy-Item -LiteralPath " + psQuote(file+".new") + " -Destination " + psQuote(file)}})
		c.begin(GroupApps, "node", "Node.js", "", func(it *Item) {
			c.finish(it, result{Status: StWarn, Summary: "Update Node.js & restart", Version: "25.1.0", File: file, Folder: dir,
				Owner: "Example App 22", OwnerVer: "4.2.0", Component: "Node.js",
				Details: "Outdated.\n\n" + T("fix.ps") + "\n" + update.lines() + "\n\n" + T("fix.ps") + "\n" + replace.lines()})
		})
	}
	c.add(GroupApps, "app", "=Example App 23", result{Status: StCaution, Summary: "Has its own list of certificates", Version: "0.9.1",
		File: `C:\Tools\Example App 23\app23.exe`, Folder: `C:\Tools\Example App 23`})
	return c
}

// The JSON report gives the results of the text report in fields: the header, the
// elements in the order of the text report (a repeated finding once, with its
// copies), their fixes, then the log. Its keywords do not change with the language.
func TestJSONReport(t *testing.T) {
	c := exportChecker(t)
	now := time.Date(2026, 12, 1, 10, 15, 48, 0, time.Local)
	data, err := reportFile(c, "first line\r\nsecond line\r\n", formatJSON, now)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.HasPrefix(data, utf8BOM) || !bytes.HasSuffix(data, []byte("}\n")) || bytes.Contains(data, []byte("\r")) {
		t.Errorf("not plain UTF-8 JSON with LF line ends:\n%.80s", data)
	}
	if !bytes.Contains(data, []byte(`"summary": "Update Node.js & restart"`)) { // "&" as it is
		t.Errorf("summary escaped:\n%s", data)
	}
	var doc reportDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Format != "emudscan-report" || doc.FormatVersion != 1 || doc.Tool.Name != appName || doc.Tool.URL != appURL ||
		doc.Tool.Version == "" || (doc.Tool.Date == "") == (doc.Tool.Commit == "") {
		t.Errorf("format and tool: %+v", doc)
	}
	if doc.Created != now.Format(time.RFC3339) || doc.Language != "en" || doc.PC != (reportPC{Name: "MY-PC", Windows: windowsVersionString()}) {
		t.Errorf("created %q, language %q, pc %+v", doc.Created, doc.Language, doc.PC)
	}
	if want := (reportScan{OnlineTest: false, Drives: []string{"C:", "D:"}, Seconds: 4, Complete: true}); !reflect.DeepEqual(doc.Scan, want) {
		t.Errorf("scan: %+v", doc.Scan)
	}
	if doc.WorstStatus != "update_needed" || doc.Counts != (reportCounts{UpdateNeeded: 1, Warning: 1, OK: 2}) {
		t.Errorf("worst status %q, counts %+v", doc.WorstStatus, doc.Counts)
	}
	if !reflect.DeepEqual(doc.Log, []string{"first line", "second line"}) || doc.FixesRun != nil {
		t.Errorf("log %q, fixes run %q", doc.Log, doc.FixesRun)
	}
	if len(doc.Items) != 4 {
		t.Fatalf("%d items:\n%s", len(doc.Items), data)
	}
	win, browser, node, app := doc.Items[0], doc.Items[1], doc.Items[2], doc.Items[3]
	if want := (reportItem{Group: "windows", Status: "ok", Program: "Windows", Component: "Automatic certificate updates", Summary: "Enabled"}); !reflect.DeepEqual(win, want) {
		t.Errorf("check of Windows: %+v", win)
	}
	if want := (reportItem{Group: "internet", Status: "ok", Program: "Example App 21", ProgramVersion: "9.4.1", Summary: "Blocks the compromised certificates",
		SoftwareDate: "2026-03-02", File: `C:\Program Files\Example App 21\app21.exe`, Folder: `C:\Program Files\Example App 21`,
		Homepage: "https://example.com/app-21"}); !reflect.DeepEqual(browser, want) {
		t.Errorf("browser: %+v", browser)
	}
	if node.Group != "other" || node.Status != "update_needed" || node.Program != "Example App 22" || node.ProgramVersion != "4.2.0" ||
		node.Component != "Node.js" || node.ComponentVersion != "25.1.0" || node.SoftwareDate != "2026-03-02" ||
		node.File != `C:\Tools\Example App 22\one\node.exe` || node.Folder != `C:\Tools\Example App 22\one` ||
		!strings.Contains(node.Details, "Found in 2 places:") || !strings.Contains(node.Details, "    scoop update example-app-22") {
		t.Errorf("Node.js: %+v", node)
	}
	if want := []reportPackage{{Manager: "Scoop", ID: "example-app-22", Page: scoopPage("example-app-22")}}; !reflect.DeepEqual(node.Packages, want) {
		t.Errorf("packages: %+v", node.Packages)
	}
	if want := []reportCopy{{File: `C:\Tools\Example App 22\two\node.exe`, Folder: `C:\Tools\Example App 22\two`}}; !reflect.DeepEqual(node.Copies, want) {
		t.Errorf("copies: %+v", node.Copies)
	}
	// the update once, though both copies show it, then the replacement of each copy
	if len(node.Fixes) != 3 ||
		!reflect.DeepEqual(node.Fixes[0], reportFix{Kind: "update", Manager: "Scoop", Commands: []string{"scoop update example-app-22"}}) ||
		node.Fixes[1].Kind != "replace_node" || !node.Fixes[1].Optional || !node.Fixes[1].Administrator || !strings.Contains(node.Fixes[1].Commands[0], `\one\node.exe`) ||
		node.Fixes[2].Kind != "replace_node" || !strings.Contains(node.Fixes[2].Commands[0], `\two\node.exe`) {
		t.Errorf("fixes: %+v", node.Fixes)
	}
	if app.Status != "warning" || app.Program != "=Example App 23" || app.Component != "" || app.ComponentVersion != "" || app.Fixes != nil {
		t.Errorf("program: %+v", app)
	}

	// in French, the same keywords; what the fixes ran follows the elements
	Lang = 1
	defer func() { Lang = 0 }()
	c.recordFix(fixLine{lineCommand, "scoop update example-app-22"})
	c.recordFix(fixLine{lineOK, ""})
	doc = reportDocOf(c, "", now)
	if doc.Language != "fr" || doc.WorstStatus != "update_needed" || doc.Items[2].Status != "update_needed" || doc.Items[2].Group != "other" ||
		doc.Items[2].Fixes[0].Kind != "update" || len(doc.Log) != 0 || doc.Log == nil {
		t.Errorf("in French: %+v", doc)
	}
	if want := []string{"  " + markOK + " scoop update example-app-22"}; !reflect.DeepEqual(doc.FixesRun, want) {
		t.Errorf("fixes run: %q", doc.FixesRun)
	}
}

// An element that is fixed has no fix left; a scan that was stopped says so, and an
// element whose check did not end is not counted.
func TestJSONReportStates(t *testing.T) {
	Lang = 0
	c := newChecker(nopReporter{}, nil)
	c.setScan(scanRun{took: 1600 * time.Millisecond})
	fix := newFix(fixAction{Kind: fixBundle, Cmds: []string{"Copy-Item -LiteralPath 'new.pem' -Destination 'C:\\Tools\\Example App 24\\cacert.pem'"}})
	c.add(GroupApps, "bundle", "Example App 24", result{Status: StOK, Summary: "Up to date", File: `C:\Tools\Example App 24\cacert.pem`,
		Details: "Replaced.\n\n" + T("fix.ps") + "\n" + fix.lines()})
	c.begin(GroupApps, "app", "Example App 25", "", nil)
	doc := reportDocOf(c, "", time.Now())
	if doc.Scan.Complete || doc.Scan.Seconds != 2 || !doc.Scan.OnlineTest || doc.Scan.Drives == nil || len(doc.Scan.Drives) != 0 {
		t.Errorf("scan: %+v", doc.Scan)
	}
	if len(doc.Items) != 2 || doc.Items[0].Fixes != nil || doc.Items[1].Status != "unknown" || doc.WorstStatus != "ok" || doc.Counts != (reportCounts{OK: 1}) {
		t.Errorf("items: %+v, worst %q, counts %+v", doc.Items, doc.WorstStatus, doc.Counts)
	}
	// without any element, no worst status
	if doc := reportDocOf(newChecker(nopReporter{}, nil), "", time.Now()); doc.WorstStatus != "unknown" || doc.Items == nil || len(doc.Items) != 0 {
		t.Errorf("empty: %+v", doc)
	}
}

// The CSV report has one row for each element, with the same values as the JSON
// report, commas, CRLF line ends and a byte order mark; a value that a spreadsheet
// would run as a formula gets an apostrophe.
func TestCSVReport(t *testing.T) {
	c := exportChecker(t)
	now := time.Date(2026, 12, 1, 10, 15, 48, 0, time.Local)
	data, err := reportFile(c, "the log is not in the CSV", formatCSV, now)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(data, utf8BOM) || !bytes.HasSuffix(data, []byte("\r\n")) || bytes.Count(data, []byte("\r\n")) != bytes.Count(data, []byte("\n")) {
		t.Errorf("no byte order mark, or line ends that are not CRLF:\n%s", data)
	}
	rows, err := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(data, utf8BOM))).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 5 || !reflect.DeepEqual(rows[0], csvColumns) || len(csvColumns) != 16 {
		t.Fatalf("rows:\n%s", data)
	}
	created, win := now.Format(time.RFC3339), windowsVersionString()
	version, _ := appVersionStamp()
	for i, want := range [][]string{
		{"MY-PC", win, created, version + simulationMark, "windows", "ok", "Windows", "", "Automatic certificate updates", "", "Enabled", "", "", "", "", ""},
		{"MY-PC", win, created, version + simulationMark, "internet", "ok", "Example App 21", "9.4.1", "", "", "Blocks the compromised certificates", "2026-03-02",
			`C:\Program Files\Example App 21\app21.exe`, `C:\Program Files\Example App 21`, "", ""},
		{"MY-PC", win, created, version + simulationMark, "other", "update_needed", "Example App 22", "4.2.0", "Node.js", "25.1.0", "Update Node.js & restart", "2026-03-02",
			`C:\Tools\Example App 22\one\node.exe`, `C:\Tools\Example App 22\one`, `C:\Tools\Example App 22\two\node.exe`, "update | replace_node"},
		{"MY-PC", win, created, version + simulationMark, "other", "warning", "'=Example App 23", "0.9.1", "", "", "Has its own list of certificates", "2026-03-02",
			`C:\Tools\Example App 23\app23.exe`, `C:\Tools\Example App 23`, "", ""},
	} {
		if !reflect.DeepEqual(rows[i+1], want) {
			t.Errorf("row %d:\n got %q\nwant %q", i+1, rows[i+1], want)
		}
	}
	for in, want := range map[string]string{"": "", "1.2.3": "1.2.3", "=1+1": "'=1+1", "+1": "'+1", "-1": "'-1", "@x": "'@x", "\tx": "'\tx", "a=b": "a=b", `%APPDATA%\x`: `%APPDATA%\x`} {
		if got := csvCell(in); got != want {
			t.Errorf("csvCell(%q) = %q, want %q", in, got, want)
		}
	}
}

// The content of a saved report is what the name of its file says: the type of its
// extension, else the type chosen in the list of the dialog. A dialog that leaves
// the first extension in the name when the type is changed gets the type chosen,
// under its extension.
func TestSaveTarget(t *testing.T) {
	for _, x := range []struct {
		path            string
		offered, chosen reportFormat
		file            string
		f               reportFormat
		renamed         bool
	}{
		// the dialog gives the name the extension of the type chosen
		{`C:\Reports\eMudScan-report-20261201-101548.txt`, formatText, formatText, "", formatText, false},
		{`C:\Reports\eMudScan-report-20261201-101548.json`, formatText, formatJSON, "", formatJSON, false},
		{`C:\Reports\eMudScan-report-20261201-101548.csv`, formatJSON, formatCSV, "", formatCSV, false},
		// the dialog leaves the name as it is: the type chosen, under its extension
		{`C:\Reports\eMudScan-report-20261201-101548.txt`, formatText, formatJSON, `C:\Reports\eMudScan-report-20261201-101548.json`, formatJSON, true},
		{`C:\Reports\report.JSON`, formatJSON, formatCSV, `C:\Reports\report.csv`, formatCSV, true},
		// an extension typed: its type, whatever the list says
		{`C:\Reports\report.CSV`, formatText, formatText, "", formatCSV, false},
		{`C:\Reports\report.csv`, formatText, formatJSON, "", formatCSV, false},
		{`C:\Reports\report.txt`, formatJSON, formatJSON, "", formatText, false},
		{`C:\Reports\report.json`, formatText, noFormat, "", formatJSON, false},
		{`C:\Reports\report.txt`, formatText, noFormat, "", formatText, false},
		// another extension, or none: the type chosen; text for "All files"
		{`C:\Reports\report`, formatText, formatCSV, "", formatCSV, false},
		{`C:\Reports\report.log`, formatText, formatJSON, "", formatJSON, false},
		{`C:\Reports.json\report`, formatText, formatText, "", formatText, false},
		{`C:\Reports\report.log`, formatJSON, noFormat, "", formatText, false},
		{`/tmp/reports/report.json`, formatText, noFormat, "", formatJSON, false},
	} {
		want := x.file
		if want == "" {
			want = x.path
		}
		if file, f, renamed := saveTarget(x.path, x.offered, x.chosen); file != want || f != x.f || renamed != x.renamed {
			t.Errorf("saveTarget(%q, %v, %v) = %q, %v, %v; want %q, %v, %v", x.path, x.offered, x.chosen, file, f, renamed, want, x.f, x.renamed)
		}
	}
	for i, f := range reportFormats { // the place of a type in the list of the dialog is its number
		if int(f) != i || formatOfFile("report."+f.ext()) != f {
			t.Errorf("type %d: %v, extension %q", i, f, f.ext())
		}
	}
}

// A report saved under a name that the dialog did not ask about never replaces a
// file: it gets the next free name.
func TestWriteNewFile(t *testing.T) {
	dir := t.TempDir()
	for i, want := range []string{"eMudScan-report-20261201-101548.json", "eMudScan-report-20261201-101548-2.json", "eMudScan-report-20261201-101548-3.json"} {
		data := []byte{byte('a' + i)}
		path, err := writeNewFile(dir, "eMudScan-report-20261201-101548", "json", data)
		if err != nil || path != filepath.Join(dir, want) {
			t.Fatalf("file %d: %q, %v", i+1, path, err)
		}
		if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, data) {
			t.Errorf("file %d: %q, %v", i+1, got, err)
		}
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "eMudScan-report-20261201-101548.json")); string(got) != "a" {
		t.Errorf("first file replaced: %q", got)
	}
	if _, err := writeNewFile(filepath.Join(dir, "missing"), "report", "txt", nil); err == nil {
		t.Error("no error in a folder that does not exist")
	}
}

// The text report is the report of Copy report after a byte order mark.
func TestTextReportFile(t *testing.T) {
	c := exportChecker(t)
	data, err := reportFile(c, "the log", formatText, time.Now())
	if err != nil || !bytes.HasPrefix(data, utf8BOM) || !bytes.Contains(data, []byte(T("report.title"))) || !bytes.HasSuffix(data, []byte("the log\r\n")) {
		t.Errorf("text report: %v\n%.200s", err, data)
	}
}
