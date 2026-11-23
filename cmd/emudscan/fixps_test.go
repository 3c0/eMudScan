package main

// The fix that is a script (fixAction.Script), run for real: by each PowerShell of the
// PC, as the Fix function runs it (the same script, started in the same way, its log
// read in the same way), on a stand-in Python in the folder of the test, with its
// download already there: nothing is downloaded, and nothing is written elsewhere.
//
// On Windows, the PowerShells are Windows PowerShell (5.1), the pwsh of the PATH
// (PowerShell 7), and those that EMUDSCAN_TEST_POWERSHELL names (paths, separated as
// in PATH). The fixes are scripts for Windows: on another system the tests are
// skipped, unless EMUDSCAN_TEST_POWERSHELL names a PowerShell; they then run with the
// paths of the scripts written for that system (/ for \, and its tar), which tests the
// PowerShell of the scripts, not the tools of Windows. EMUDSCAN_TEST_POWERSHELL_WANT
// names versions that must run, such as "5.1,7.6,7.7": the tests fail when one of them
// is not found (.github/workflows/powershell.yml).

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// testPowerShells are the PowerShells that run the tests, each with its version.
func testPowerShells(t *testing.T) map[string]string {
	t.Helper()
	if fixSimulation {
		t.Skip("the simulation replaces the commands")
	}
	var paths []string
	if runtime.GOOS == "windows" {
		paths = append(paths, filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe"))
		if p, err := exec.LookPath("pwsh"); err == nil {
			paths = append(paths, p)
		}
	}
	paths = append(paths, filepath.SplitList(os.Getenv("EMUDSCAN_TEST_POWERSHELL"))...)
	found := map[string]string{}
	for _, p := range paths {
		out, err := exec.Command(p, "-NoProfile", "-NonInteractive", "-Command", "$PSVersionTable.PSVersion.ToString()").Output()
		if v := strings.TrimSpace(string(out)); err == nil && v != "" {
			if _, seen := found[v]; !seen {
				found[v] = p
			}
		}
	}
	for _, want := range strings.Split(os.Getenv("EMUDSCAN_TEST_POWERSHELL_WANT"), ",") {
		if want = strings.TrimSpace(want); want != "" && !hasVersion(found, want) {
			t.Fatalf("no PowerShell %s to run the fixes with: found %v", want, found)
		}
	}
	if len(found) == 0 {
		t.Skip("no PowerShell to run the fixes with")
	}
	return found
}

// hasVersion: one of the PowerShells found has the version want, or a version of it
// ("7.6" for 7.6.10).
func hasVersion(found map[string]string, want string) bool {
	for v := range found {
		if v == want || strings.HasPrefix(v, want+".") {
			return true
		}
	}
	return false
}

// standInPython builds, in dir, a program that stands in for python.exe: it shows a
// line, and fails, with a line on its error output, when the file fail-before is next
// to it and its _ssl.pyd (in DLLs, or next to it) is the old one, or when fail-after
// is and its _ssl.pyd is a new one.
func standInPython(t *testing.T, dir string) string {
	t.Helper()
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte(`package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	exe, _ := os.Executable()
	home, when := filepath.Dir(exe), "after"
	for _, d := range []string{"DLLs", "."} {
		if b, err := os.ReadFile(filepath.Join(home, d, "_ssl.pyd")); err == nil {
			if strings.HasPrefix(string(b), "old") {
				when = "before"
			}
			break
		}
	}
	if _, err := os.Stat(filepath.Join(home, "fail-"+when)); err == nil {
		fmt.Fprintln(os.Stderr, "ImportError: DLL load failed ("+when+")")
		os.Exit(1)
	}
	fmt.Println("stand-in Python, " + when + " the replacement")
}
`), 0o644)
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/python\n\ngo 1.21\n"), 0o644)
	exe := filepath.Join(dir, "python.exe")
	cmd := exec.Command(filepath.Join(runtime.GOROOT(), "bin", "go"), "build", "-o", exe, ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOFLAGS=")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("cannot build the stand-in Python: %v\n%s", err, out)
	}
	return exe
}

// psCase is a Python to fix, and what happens to it.
type psCase struct {
	name           string
	sslDir         string   // where its ssl files are: DLLs, or "." for python.org's embeddable package
	replace, added []string // what the fix replaces and adds (pySSLShapes)
	zip            bool     // from python.org's package; else from python-build-standalone's archive
	backup         bool     // "Back up the files to change"
	failBefore     bool     // its ssl module does not load
	failAfter      bool     // its ssl module does not load with the new files
	badSum         bool     // the download is not the one the commands name
	lacks          string   // a file that the download does not have
	inUse          bool     // a program has a file to replace open (Windows)
	fixed          bool     // what is expected: the new files in place, else the original ones
	stopsAt        string   // the command that fails, by a part of it; "" when none does
}

// psFixture lays out the Python of c in root, with the download of its fix in run (the
// folder of the fix's script), and gives its fix.
func psFixture(t *testing.T, c psCase, python, root, run string) (fixAction, string) {
	t.Helper()
	home := filepath.Join(root, "Example App 1", "python")
	dir := filepath.Join(home, c.sslDir)
	os.MkdirAll(dir, 0o755)
	data, err := os.ReadFile(python)
	if err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(home, "python.exe")
	os.WriteFile(exe, data, 0o755)
	old := append(append([]string{"_hashlib.pyd"}, c.replace...), "libssl-3.dll", "libcrypto-3.dll")
	for _, n := range old {
		if _, err := os.Stat(filepath.Join(dir, n)); err != nil {
			os.WriteFile(filepath.Join(dir, n), []byte("old "+n), 0o644)
		}
	}
	if c.failBefore {
		os.WriteFile(filepath.Join(home, "fail-before"), nil, 0o644)
	}
	if c.failAfter {
		os.WriteFile(filepath.Join(home, "fail-after"), nil, 0o644)
	}
	// the download
	src := pySSLSource{version: "3.11.18", url: "https://example.invalid/python.tar.gz", tag: "20261110", zip: c.zip}
	var archive bytes.Buffer
	names := append(append([]string(nil), c.replace...), c.added...)
	if c.zip {
		src.version, src.url, src.tag = "3.14.10", "https://example.invalid/python.zip", ""
		w := zip.NewWriter(&archive)
		for _, n := range append(names, "python314.dll") {
			if n != c.lacks {
				f, _ := w.Create(n)
				f.Write([]byte("new " + n))
			}
		}
		w.Close()
	} else {
		gz := gzip.NewWriter(&archive)
		w := tar.NewWriter(gz)
		for _, n := range append(names, "python311.dll") {
			if n != c.lacks {
				body := []byte("new " + n)
				w.WriteHeader(&tar.Header{Name: "python/DLLs/" + n, Mode: 0o644, Size: int64(len(body)), ModTime: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)})
				w.Write(body)
			}
		}
		w.Close()
		gz.Close()
	}
	sum := sha256.Sum256(archive.Bytes())
	src.sha256 = hex.EncodeToString(sum[:])
	if c.badSum {
		archive.WriteString("changed")
	}
	ext := ".tar.gz"
	if c.zip {
		ext = ".zip"
	}
	if err := os.WriteFile(filepath.Join(run, src.name()+ext), archive.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return pySSLFix(exe, pySSLPlan{src: src, dir: dir, replace: c.replace, added: c.added}), dir
}

// psAdapt writes the lines of a fix for the system of the test, when it is not Windows.
func psAdapt(lines []string) []string {
	if runtime.GOOS == "windows" {
		return lines
	}
	out := make([]string, len(lines))
	for i, l := range lines {
		l = strings.ReplaceAll(l, `& "$env:SystemRoot\System32\tar.exe"`, "& tar")
		out[i] = strings.ReplaceAll(l, `\`, "/")
	}
	return out
}

// psRun runs units in the PowerShell ps as runFixes does, in the folder run, and gives
// the lines of the console and the result of each fix.
func psRun(t *testing.T, ps, run string, units []fixUnit) ([]fixLine, []unitResult) {
	t.Helper()
	logPath, scriptPath, marker := filepath.Join(run, "fix.log"), filepath.Join(run, "fix.ps1"), fixMarker()
	files, err := fixFiles(units, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for n, b := range files {
		os.WriteFile(filepath.Join(run, n), b, 0o600)
	}
	script := []byte(fixScript(units, logPath, marker, files))
	os.WriteFile(scriptPath, script, 0o600)
	os.WriteFile(logPath, nil, 0o600)
	out, err := exec.Command(ps, "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-EncodedCommand", encodedCommand(fixBootstrap(scriptPath, script))).CombinedOutput()
	if err != nil {
		t.Fatalf("PowerShell: %v\n%s", err, out)
	}
	data, _ := os.ReadFile(logPath)
	var lines []fixLine
	log := newFixLog(units, marker, func(l fixLine) { lines = append(lines, l) }, false)
	log.write(string(data))
	log.close()
	return lines, log.results
}

var pbsShape, orgShape, mixedShape = pySSLShapes[0], pySSLShapes[1], pySSLShapes[2]

// The script of the fix of a Python, in each PowerShell: the new files in place when
// all goes well, with their backups, or without them and without the marks when
// "Back up the files to change" is cleared; the original files, and nothing else, when
// a command fails, wherever it is; a result for each command that ran, and for no
// other line.
func TestPySSLFixInPowerShell(t *testing.T) {
	Lang = 0
	shells := testPowerShells(t)
	base := t.TempDir() // the folders of the cases are in it, by number: short paths, for Windows PowerShell
	python := standInPython(t, filepath.Join(base, "build"))
	cases := []psCase{
		{name: "standalone", sslDir: "DLLs", replace: pbsShape.replace, backup: true, fixed: true},
		{name: "standalone, no backups", sslDir: "DLLs", replace: pbsShape.replace, fixed: true},
		{name: "python.org", sslDir: ".", replace: orgShape.replace, zip: true, backup: true, fixed: true},
		{name: "added files", sslDir: "DLLs", replace: mixedShape.replace, added: mixedShape.added, backup: true, fixed: true},
		{name: "added files, no backups", sslDir: "DLLs", replace: mixedShape.replace, added: mixedShape.added, fixed: true},
		{name: "fails before", sslDir: "DLLs", replace: pbsShape.replace, backup: true, failBefore: true, stopsAt: "$emudscanFiles before"},
		{name: "another download", sslDir: "DLLs", replace: pbsShape.replace, backup: true, badSum: true, stopsAt: "Get-FileHash"},
		{name: "a file missing", sslDir: "DLLs", replace: pbsShape.replace, backup: true, lacks: "libssl-3-x64.dll", stopsAt: "tar"},
		{name: "a file missing, python.org", sslDir: ".", replace: orgShape.replace, zip: true, backup: true, lacks: "libssl-3.dll", stopsAt: "Where-Object"},
		{name: "in use", sslDir: "DLLs", replace: pbsShape.replace, backup: true, inUse: true, stopsAt: "[IO.File]::Open"},
		{name: "fails after", sslDir: "DLLs", replace: pbsShape.replace, backup: true, failAfter: true, stopsAt: "$emudscanFiles after"},
		{name: "fails after, no backups", sslDir: "DLLs", replace: pbsShape.replace, failAfter: true, stopsAt: "$emudscanFiles after"},
		{name: "fails after, added files", sslDir: "DLLs", replace: mixedShape.replace, added: mixedShape.added, backup: true, failAfter: true, stopsAt: "$emudscanFiles after"},
		{name: "fails after, python.org", sslDir: ".", replace: orgShape.replace, zip: true, backup: true, failAfter: true, stopsAt: "$emudscanFiles after"},
	}
	n := 0
	for version, ps := range shells {
		for _, c := range cases {
			n++
			root, run := filepath.Join(base, fmt.Sprint(n), "a"), filepath.Join(base, fmt.Sprint(n), "r")
			t.Run("PowerShell "+version+"/"+c.name, func(t *testing.T) {
				t.Parallel() // each case in its own folders
				if c.inUse && runtime.GOOS != "windows" {
					t.Skip("a file in use can be replaced on this system")
				}
				os.MkdirAll(run, 0o755)
				f, dir := psFixture(t, c, python, root, run)
				created := map[string]int64{}
				for _, n := range c.replace {
					created[n] = fileCreated(filepath.Join(dir, n))
				}
				if c.inUse {
					held, err := os.Open(filepath.Join(dir, c.replace[len(c.replace)-1]))
					if err != nil {
						t.Fatal(err)
					}
					defer held.Close()
				}
				r := fixRunner{backup: c.backup}
				unit := r.unit(Item{ID: 1, Name: "Example App 1"}, f)
				unit.cmds = psAdapt(unit.cmds)
				lines, res := psRun(t, ps, run, []fixUnit{unit})
				report := strings.Join(reportLines(lines), "\n")
				if len(res) != 1 || !res[0].ran || res[0].ok != c.fixed || res[0].halted {
					t.Errorf("result %+v, want fixed %v\n%s", res, c.fixed, report)
				}
				// the whole script, in its order; a result for each command that ran
				var script, failed []string
				for i, l := range lines {
					if l.kind == lineCommand || l.kind == lineScript {
						script = append(script, l.text)
					}
					if l.kind == lineScript && i+1 < len(lines) && (lines[i+1].kind == lineOK || lines[i+1].kind == lineFailed) {
						t.Errorf("a result for %q", l.text)
					}
					if l.kind == lineCommand && (i+1 == len(lines) || lines[i+1].kind != lineOK && lines[i+1].kind != lineFailed) {
						t.Errorf("no result for %q", l.text)
					}
					if l.kind == lineFailed && i > 0 {
						failed = append(failed, lines[i-1].text)
					}
				}
				if strings.Join(script, "\n") != strings.Join(unit.cmds, "\n") {
					t.Errorf("the script of the console:\n%s\nwant:\n%s", strings.Join(script, "\n"), strings.Join(unit.cmds, "\n"))
				}
				switch {
				case c.stopsAt == "" && len(failed) > 0:
					t.Errorf("failed: %q\n%s", failed, report)
				case c.stopsAt != "" && (len(failed) == 0 || !strings.Contains(failed[0], c.stopsAt)):
					t.Errorf("failed: %q, want %q first\n%s", failed, c.stopsAt, report)
				case c.failAfter && (len(failed) != 2 || !strings.Contains(failed[1], "throw "+psQuote(T("pyssl.putBack")))):
					t.Errorf("failed: %q, want the test after, then the files put back\n%s", failed, report)
				case c.stopsAt != "" && !c.failAfter && len(failed) != 1:
					t.Errorf("failed: %q, want one command\n%s", failed, report)
				}
				// the files
				entries, _ := os.ReadDir(dir)
				var got []string
				for _, e := range entries {
					b, _ := os.ReadFile(filepath.Join(dir, e.Name()))
					n := e.Name()
					if i := strings.Index(n, ".emudscan-"); i >= 0 {
						if at := fileCreated(filepath.Join(dir, n)); strings.HasSuffix(n, ".bak") && at != created[n[:i]] {
							t.Errorf("%s does not have the creation date of its file", n)
						}
						n = n[:i] + n[strings.LastIndex(n, "."):] // without the time
					}
					if e.Name() != "python.exe" && !strings.HasPrefix(e.Name(), "fail-") {
						got = append(got, n+"="+strings.Fields(string(b) + " empty")[0])
					}
				}
				want := map[string]string{"_hashlib.pyd": "old", "libssl-3.dll": "old", "libcrypto-3.dll": "old"}
				for _, n := range c.replace {
					want[n] = "old"
					if c.fixed {
						want[n] = "new"
						if c.backup {
							want[n+".bak"] = "old"
						}
					}
				}
				for _, n := range c.added {
					if c.fixed {
						want[n] = "new"
						if c.backup {
							want[n+".added"] = "empty"
						}
					}
				}
				for _, g := range got {
					n, v, _ := strings.Cut(g, "=")
					if want[n] != v {
						t.Errorf("%s is %q, want %q; the folder: %q\n%s", n, v, want[n], got, report)
					}
					delete(want, n)
				}
				if len(want) > 0 {
					t.Errorf("missing %v; the folder: %q\n%s", want, got, report)
				}
				if t.Failed() {
					t.Logf("the lines of the fix:\n%s", strings.Join(unit.cmds, "\n"))
				}
			})
		}
	}
}

// The version of the script that runs alone, which the details page shows, run as a
// script by each PowerShell, with the Temp folder of the test as the user's: the same
// outcomes as the fix (but a file missing in python.org's package, which the same
// line finds in both), by itself: it stops at its first error, and its download is
// deleted at its end.
func TestPySSLAloneInPowerShell(t *testing.T) {
	Lang = 0
	shells := testPowerShells(t)
	base := t.TempDir()
	python := standInPython(t, filepath.Join(base, "build"))
	cases := []psCase{
		{name: "standalone", sslDir: "DLLs", replace: pbsShape.replace, fixed: true},
		{name: "python.org", sslDir: ".", replace: orgShape.replace, zip: true, fixed: true},
		{name: "added files", sslDir: "DLLs", replace: mixedShape.replace, added: mixedShape.added, fixed: true},
		{name: "fails before", sslDir: "DLLs", replace: pbsShape.replace, failBefore: true, stopsAt: T("pyssl.a.noSsl")},
		{name: "another download", sslDir: "DLLs", replace: pbsShape.replace, badSum: true, stopsAt: T("pyssl.badSum")},
		{name: "a file missing", sslDir: "DLLs", replace: pbsShape.replace, lacks: "libssl-3-x64.dll", stopsAt: T("pyssl.missing")},
		{name: "a file missing, python.org", sslDir: ".", replace: orgShape.replace, zip: true, lacks: "libssl-3.dll", stopsAt: T("pyssl.missing")},
		{name: "in use", sslDir: "DLLs", replace: pbsShape.replace, inUse: true, stopsAt: "libcrypto-3-x64.dll"},
		{name: "fails after", sslDir: "DLLs", replace: pbsShape.replace, failAfter: true, stopsAt: T("pyssl.putBack")},
		{name: "fails after, added files", sslDir: "DLLs", replace: mixedShape.replace, added: mixedShape.added, failAfter: true, stopsAt: T("pyssl.putBack")},
		{name: "fails after, python.org", sslDir: ".", replace: orgShape.replace, zip: true, failAfter: true, stopsAt: T("pyssl.putBack")},
	}
	n := 0
	for version, ps := range shells {
		for _, c := range cases {
			n++
			root, temp := filepath.Join(base, fmt.Sprint(n), "a"), filepath.Join(base, fmt.Sprint(n), "t")
			t.Run("PowerShell "+version+"/"+c.name, func(t *testing.T) {
				t.Parallel()
				if c.inUse && runtime.GOOS != "windows" {
					t.Skip("a file in use can be replaced on this system")
				}
				download := filepath.Join(temp, "eMudScan-fix")
				os.MkdirAll(download, 0o755)
				f, dir := psFixture(t, c, python, root, download)
				if c.inUse {
					held, err := os.Open(filepath.Join(dir, c.replace[len(c.replace)-1]))
					if err != nil {
						t.Fatal(err)
					}
					defer held.Close()
				}
				if f.Alone == nil || f.lines() == "" || strings.Contains(f.lines(), "emudscanFiles") || strings.Contains(f.lines(), "check_ssl") {
					t.Fatalf("not a script that runs alone:\n%s", f.lines())
				}
				script := filepath.Join(temp, "alone.ps1")
				os.WriteFile(script, append([]byte{0xEF, 0xBB, 0xBF}, strings.Join(psAdapt(f.Alone), "\r\n")...), 0o644) // as Notepad saves it
				cmd := exec.Command(ps, "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", script)
				cmd.Env = append(os.Environ(), "TEMP="+temp)
				out, err := cmd.CombinedOutput()
				// Windows PowerShell cuts the lines of an error at the width of its window: without the spaces
				tight := func(s string) string { return strings.Join(strings.Fields(s), "") }
				if (err == nil) != c.fixed || c.stopsAt != "" && !strings.Contains(tight(string(out)), tight(c.stopsAt)) {
					t.Errorf("%v, want fixed %v and %q:\n%s", err, c.fixed, c.stopsAt, out)
				}
				want := map[string]string{"_hashlib.pyd": "old", "libssl-3.dll": "old", "libcrypto-3.dll": "old"}
				for _, n := range c.replace {
					want[n] = "old"
					if c.fixed {
						want[n], want[n+".bak"] = "new", "old"
					}
				}
				for _, n := range c.added {
					if c.fixed {
						want[n], want[n+".added"] = "new", "empty"
					}
				}
				if got := folderState(dir); fmt.Sprint(got) != fmt.Sprint(want) {
					t.Errorf("the folder: %v\nwant: %v\n%s", got, want, out)
				}
				// the download: deleted at the end; else left for the next time
				if _, err := os.Stat(download); (err != nil) != c.fixed {
					t.Errorf("the folder of the download: %v, with fixed %v", err, c.fixed)
				}
			})
		}
	}
}

// folderState is the files of the folder of a stand-in Python, each with the first
// word of its text ("empty" for none), the backups and marks without their time.
func folderState(dir string) map[string]string {
	got := map[string]string{}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		n := e.Name()
		if n == "python.exe" || strings.HasPrefix(n, "fail-") {
			continue
		}
		b, _ := os.ReadFile(filepath.Join(dir, n))
		if i := strings.Index(n, ".emudscan-"); i >= 0 {
			n = n[:i] + n[strings.LastIndex(n, "."):]
		}
		got[n] = strings.Fields(string(b) + " empty")[0]
	}
	return got
}

// A fix that stops does not stop the fixes that follow it.
func TestScriptStopsAlone(t *testing.T) {
	Lang = 0
	for version, ps := range testPowerShells(t) {
		first := fixAction{Kind: fixPySSL, Script: true, Cmds: []string{"# a fix", "$a = 1", "", "throw 'stop'", "$a = 2"}}
		second := fixAction{Kind: fixBundle, Cmds: []string{"if ($a -ne 1) { throw 'the fix before did not stop' }", "$b = 1"}}
		units := []fixUnit{{ids: []int{1}, titles: []string{"A"}, fix: first, cmds: first.Cmds}, {ids: []int{2}, titles: []string{"B"}, fix: second, cmds: second.Cmds}}
		lines, res := psRun(t, ps, t.TempDir(), units)
		if len(res) != 2 || !res[0].ran || res[0].ok || !res[1].ran || !res[1].ok {
			t.Errorf("PowerShell %s: %+v\n%s", version, res, strings.Join(reportLines(lines), "\n"))
		}
		want := []string{"    # a fix", "  ✓ $a = 1", "", "  ✗ throw 'stop'", "      stop", "    $a = 2", "  ✓ if ($a -ne 1) { throw 'the fix before did not stop' }", "  ✓ $b = 1"}
		if got := reportLines(lines); strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Errorf("PowerShell %s:\n%s\nwant:\n%s", version, strings.Join(got, "\n"), strings.Join(want, "\n"))
		}
	}
}
