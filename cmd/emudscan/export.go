package main

// The report for other programs: the results of the text report as a JSON document,
// or as a CSV table with one row for each element. The names of the fields and the
// keywords (groups, statuses, kinds of fixes) are in English and do not change with
// the language; the texts of eMudScan (summaries, explanations, log) are in the
// language of the window, which the JSON document names. The paths are written as
// in the text report (shownPath).

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// reportFormat is the type of a report file.
type reportFormat int

const (
	formatText reportFormat = iota
	formatJSON
	formatCSV
)

// reportFormats are the types, in the order of the Save report dialog.
var reportFormats = []reportFormat{formatText, formatJSON, formatCSV}

// ext is the extension of the files of the type, without its dot.
func (f reportFormat) ext() string {
	switch f {
	case formatJSON:
		return "json"
	case formatCSV:
		return "csv"
	}
	return "txt"
}

// noFormat: none of the types (a name with another extension or none, "All files"
// in the list of types).
const noFormat reportFormat = -1

// formatOfFile is the type that the extension of the file path names, else noFormat.
func formatOfFile(path string) reportFormat {
	name := path[strings.LastIndexAny(path, `\/`)+1:]
	if dot := strings.LastIndex(name, "."); dot >= 0 {
		for _, f := range reportFormats {
			if strings.EqualFold(name[dot+1:], f.ext()) {
				return f
			}
		}
	}
	return noFormat
}

// saveTarget is the file that Save report writes and its type, from what its dialog
// gave: the file path, and the type chosen in its list (noFormat: "All files"), which
// showed the type offered first when it opened. The content of a file is always
// what its name says:
//   - a name with the extension of a type gets that type, whatever the list says: it
//     was typed, or the dialog gave it when the type was chosen;
//   - but when the type was changed in the list and the name still has the extension
//     of the type offered first (a dialog that does not change the name with the
//     type), the type chosen is written, under its own extension (renamed: the caller
//     must not replace a file of that name, which the dialog did not ask about);
//   - a name with another extension, or none, gets the type chosen; text for "All
//     files".
func saveTarget(path string, offered, chosen reportFormat) (file string, f reportFormat, renamed bool) {
	named := formatOfFile(path)
	switch {
	case named == noFormat && chosen == noFormat:
		return path, formatText, false
	case named == noFormat:
		return path, chosen, false
	case chosen != noFormat && chosen != offered && named == offered:
		return path[:len(path)-len(offered.ext())] + chosen.ext(), chosen, true
	}
	return path, named, false
}

// writeNewFile writes data in a new file of the folder dir, named base with the
// extension ext, or base-2, base-3… when files of these names exist: it never
// replaces a file. It returns the path of the file.
func writeNewFile(dir, base, ext string, data []byte) (string, error) {
	for i := 1; i < 100; i++ {
		name := base + "." + ext
		if i > 1 {
			name = fmt.Sprintf("%s-%d.%s", base, i, ext)
		}
		path := filepath.Join(dir, name)
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		_, err = f.Write(data)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		return path, err
	}
	return "", fs.ErrExist
}

// utf8BOM starts the text and CSV reports: Notepad and Excel then read them as UTF-8.
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// reportFile is the report as the content of a file of the type f, in UTF-8: the
// text and the CSV after a byte order mark, the JSON without one (RFC 8259).
func reportFile(c *checker, logText string, f reportFormat, now time.Time) ([]byte, error) {
	switch f {
	case formatJSON:
		return jsonReport(c, logText, now)
	case formatCSV:
		data, err := csvReport(c, now)
		return append(append([]byte(nil), utf8BOM...), data...), err
	}
	return append(append([]byte(nil), utf8BOM...), buildReport(c, logText)...), nil
}

// scanRun is how the whole scan ran, for the reports.
type scanRun struct {
	drives   []string      // the drives that were scanned
	took     time.Duration // how long it took
	complete bool          // it ran to its end (false: it was stopped)
}

func (c *checker) setScan(s scanRun) {
	c.mu.Lock()
	c.scan = s
	c.mu.Unlock()
}

func (c *checker) scanInfo() scanRun {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.scan
}

// The format of the JSON report: its name, and its version, which changes when a
// field is removed or changes meaning (not when one is added).
const (
	reportFormatName    = "emudscan-report"
	reportFormatVersion = 1
)

type reportDoc struct {
	Format        string       `json:"format"`
	FormatVersion int          `json:"format_version"`
	Tool          reportTool   `json:"tool"`
	Created       string       `json:"created"`
	Language      string       `json:"language"`
	PC            reportPC     `json:"pc"`
	Scan          reportScan   `json:"scan"`
	WorstStatus   string       `json:"worst_status"`
	Counts        reportCounts `json:"counts"`
	Items         []reportItem `json:"items"`
	FixesRun      []string     `json:"fixes_run,omitempty"`
	Log           []string     `json:"log"`
}

type reportTool struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Date    string `json:"date,omitempty"`   // of a release
	Commit  string `json:"commit,omitempty"` // of a build that is not of a release
	URL     string `json:"url"`
}

type reportPC struct {
	Name    string `json:"name,omitempty"`
	Windows string `json:"windows"`
}

type reportScan struct {
	OnlineTest bool     `json:"online_test"`
	Drives     []string `json:"drives"`
	Seconds    int      `json:"seconds"`
	Complete   bool     `json:"complete"`
}

type reportCounts struct {
	Critical     int `json:"critical"`
	UpdateNeeded int `json:"update_needed"`
	Warning      int `json:"warning"`
	Information  int `json:"information"`
	OK           int `json:"ok"`
}

type reportItem struct {
	Group            string          `json:"group"`
	Status           string          `json:"status"`
	Program          string          `json:"program"`
	ProgramVersion   string          `json:"program_version,omitempty"`
	Component        string          `json:"component,omitempty"`
	ComponentVersion string          `json:"component_version,omitempty"`
	Summary          string          `json:"summary"`
	Details          string          `json:"details,omitempty"`
	SoftwareDate     string          `json:"software_date,omitempty"`
	File             string          `json:"file,omitempty"`
	Folder           string          `json:"folder,omitempty"`
	Homepage         string          `json:"homepage,omitempty"`
	Packages         []reportPackage `json:"packages,omitempty"`
	Copies           []reportCopy    `json:"copies,omitempty"`
	Fixes            []reportFix     `json:"fixes,omitempty"`
}

type reportPackage struct {
	Manager string `json:"manager"`
	ID      string `json:"id"`
	Page    string `json:"page,omitempty"`
}

type reportCopy struct {
	File   string `json:"file,omitempty"`
	Folder string `json:"folder,omitempty"`
}

type reportFix struct {
	Kind          string   `json:"kind"`
	Manager       string   `json:"manager,omitempty"`
	Optional      bool     `json:"optional"`
	Administrator bool     `json:"administrator"`
	Commands      []string `json:"commands"`
}

// statusKeyword names a status in the JSON and CSV reports.
func statusKeyword(s Status) string {
	switch s {
	case StOK:
		return "ok"
	case StInfo:
		return "information"
	case StCaution:
		return "warning"
	case StWarn:
		return "update_needed"
	case StError:
		return "critical"
	}
	return "unknown" // an element whose check did not end
}

// groupKeyword names the list of an element in the JSON and CSV reports.
func groupKeyword(g Group) string {
	switch g {
	case GroupNet:
		return "internet"
	case GroupApps:
		return "other"
	}
	return "windows"
}

// keyword names what a fix does in the JSON and CSV reports.
func (k fixKind) keyword() string {
	switch k {
	case fixPackage:
		return "update"
	case fixPip:
		return "pip"
	case fixConda:
		return "conda"
	case fixBundle:
		return "replace_bundle"
	case fixJava:
		return "java_block_list"
	case fixRestore:
		return "restore"
	case fixTool:
		return "tool"
	case fixNode:
		return "replace_node"
	case fixPySSL:
		return "replace_python_ssl"
	}
	return "other"
}

// isoDate writes a date as 2026-12-01.
func isoDate(t time.Time) string { return t.Local().Format("2006-01-02") }

// reportItems are the elements of the reports, in the order of the text report: a
// repeated finding once, with its copies.
func reportItems(c *checker) []Item {
	items := mergeRepeated(c.snapshot())
	var out []Item
	for _, g := range []Group{GroupWindows, GroupNet, GroupApps} {
		out = append(out, sortedForReport(items, g)...)
	}
	return out
}

// itemFixes are the fixes that the Fix function can run for an element and its
// copies, each once; none once it is fixed.
func itemFixes(it Item) []fixAction {
	if fixedStatus(it.Status) {
		return nil
	}
	var out []fixAction
	seen := map[string]bool{}
	for _, m := range append([]Item{it}, it.Copies...) {
		for _, f := range m.Fixes {
			if k := strings.Join(f.Cmds, "\n"); !seen[k] {
				seen[k] = true
				out = append(out, f)
			}
		}
	}
	return out
}

// softwareDate is the date of the software of an element, as its Software date
// column shows it; empty when it is unknown.
func softwareDate(it Item) string {
	if it.Group == GroupWindows || !it.Updated.known() {
		return ""
	}
	return isoDate(it.Updated.Time)
}

// reportItemOf is an element as the JSON and CSV reports give it. A check of Windows
// is a component of the program "Windows".
func reportItemOf(it Item) reportItem {
	out := reportItem{Group: groupKeyword(it.Group), Status: statusKeyword(it.Status), Summary: it.Summary,
		Details:      plainText(joinParas(staleAdvice(it), it.shownDetails())),
		SoftwareDate: softwareDate(it),
		File:         shownPath(it.File), Folder: shownPath(it.Folder), Homepage: it.Homepage()}
	if it.Group == GroupWindows {
		out.Program, out.Component = "Windows", it.Name
	} else {
		out.Program, out.ProgramVersion, out.Component = it.Program, it.ProgVersion, it.Component
		if it.Component != "" {
			out.ComponentVersion = it.Version
		}
	}
	for _, p := range it.Packages {
		out.Packages = append(out.Packages, reportPackage{Manager: p.Manager, ID: p.ID, Page: p.Page})
	}
	for _, m := range it.Copies {
		out.Copies = append(out.Copies, reportCopy{File: shownPath(m.File), Folder: shownPath(m.Folder)})
	}
	for _, f := range itemFixes(it) {
		out.Fixes = append(out.Fixes, reportFix{Kind: f.Kind.keyword(), Manager: f.Manager, Optional: f.Optional,
			Administrator: f.Admin, Commands: append([]string{}, f.Cmds...)})
	}
	return out
}

// textLines splits a text into its lines, without the empty ones at its end.
func textLines(s string) []string {
	s = strings.TrimRight(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	if s == "" {
		return []string{}
	}
	return strings.Split(s, "\n")
}

// reportDocOf is the JSON report of the results of c, made at now.
func reportDocOf(c *checker, logText string, now time.Time) reportDoc {
	version, stamp := appVersionStamp()
	tool := reportTool{Name: appName, Version: version + simulationMark, URL: appURL}
	if buildRev != "" {
		tool.Commit = stamp
	} else if d, err := time.ParseInLocation("20060102", stamp, time.Local); err == nil {
		tool.Date = isoDate(d)
	}
	lang := "en"
	if Lang == 1 {
		lang = "fr"
	}
	scan, n := c.scanInfo(), c.counts()
	drives := []string{}
	for _, d := range scan.drives {
		drives = append(drives, strings.TrimRight(d, `\`))
	}
	doc := reportDoc{
		Format: reportFormatName, FormatVersion: reportFormatVersion, Tool: tool,
		Created: now.Format(time.RFC3339), Language: lang,
		PC:          reportPC{Name: computerName(), Windows: windowsVersionString()},
		Scan:        reportScan{OnlineTest: !c.offline, Drives: drives, Seconds: int(scan.took.Round(time.Second).Seconds()), Complete: scan.complete},
		WorstStatus: statusKeyword(n.worst()),
		Counts:      reportCounts{Critical: n.errs, UpdateNeeded: n.warn, Warning: n.caution, Information: n.info, OK: n.ok},
		Items:       []reportItem{},
		Log:         textLines(logText),
	}
	for _, it := range reportItems(c) {
		doc.Items = append(doc.Items, reportItemOf(it))
	}
	if fixes := c.fixRecord(); len(fixes) > 0 {
		doc.FixesRun = reportLines(fixes)
	}
	return doc
}

// jsonReport is the JSON report as a file: indented, one field on each line.
func jsonReport(c *checker, logText string, now time.Time) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false) // "<", ">" and "&" as they are: the file is not for a web page
	enc.SetIndent("", "  ")
	err := enc.Encode(reportDocOf(c, logText, now))
	return b.Bytes(), err
}

// csvColumns are the columns of the CSV report.
var csvColumns = []string{"pc", "windows", "created", "tool_version", "group", "status", "program", "program_version",
	"component", "component_version", "summary", "software_date", "file", "folder", "copies", "fix"}

// csvListSep separates the values of a cell that has several (copies, fixes): a
// character that a path cannot have.
const csvListSep = " | "

// csvCell is a value as a cell. A value that a spreadsheet would run as a formula
// (it starts with =, +, - or @, or with a tab or a carriage return) gets an
// apostrophe before it: the names and versions come from the software found on the
// PC, and must stay text.
func csvCell(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}

// csvReport is the CSV report: a line of column names, then one row for each
// element, with commas and CRLF line ends (RFC 4180).
func csvReport(c *checker, now time.Time) ([]byte, error) {
	doc := reportDocOf(c, "", now)
	var b bytes.Buffer
	w := csv.NewWriter(&b)
	w.UseCRLF = true
	if err := w.Write(csvColumns); err != nil {
		return nil, err
	}
	for _, it := range doc.Items {
		var copies, fixes []string
		for _, m := range it.Copies {
			copies = append(copies, firstNonEmpty(m.File, m.Folder))
		}
		for _, f := range it.Fixes {
			if k := f.Kind; len(fixes) == 0 || fixes[len(fixes)-1] != k {
				fixes = append(fixes, k)
			}
		}
		row := []string{doc.PC.Name, doc.PC.Windows, doc.Created, doc.Tool.Version, it.Group, it.Status, it.Program, it.ProgramVersion,
			it.Component, it.ComponentVersion, it.Summary, it.SoftwareDate, it.File, it.Folder,
			strings.Join(copies, csvListSep), strings.Join(fixes, csvListSep)}
		for i := range row {
			row[i] = csvCell(row[i])
		}
		if err := w.Write(row); err != nil {
			return nil, err
		}
	}
	w.Flush()
	return b.Bytes(), w.Error()
}
