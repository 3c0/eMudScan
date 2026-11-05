package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPythonVersionFromFile(t *testing.T) {
	cases := []struct {
		fv   fileVersion
		want string
	}{
		{fileVersion{Fixed: Version{3, 13, 17150, 1013}}, "3.13.17"},
		{fileVersion{Fixed: Version{3, 12, 7150, 1013}, Product: "3.12.7"}, "3.12.7"},
		{fileVersion{Product: "3.14.9"}, "3.14.9"},
		{fileVersion{}, ""},
	}
	for _, c := range cases {
		if got := pythonVersionFromFile(c.fv); got != c.want {
			t.Errorf("%+v: got %q want %q", c.fv, got, c.want)
		}
	}
}

func TestGitConfig(t *testing.T) {
	d := t.TempDir()
	f := filepath.Join(d, "gitconfig")
	os.WriteFile(f, []byte("[core]\n\tsslbackend = nope\n[]\n[http \"https://x\"]\n\tsslBackend=openssl\n[http]\n\tsslBackend = \"schannel\"\n"), 0o644)
	if v := gitConfigValue(f, "http", "sslbackend"); v != "schannel" {
		t.Errorf("got %q", v)
	}
}

func TestSquirrel(t *testing.T) {
	got := newestSquirrel([]string{"/x/Slack/app-4.9.0", "/x/Slack/app-4.10.1", "/y/Code"})
	if len(got) != 2 || got[0] != "/x/Slack/app-4.10.1" {
		t.Errorf("got %v", got)
	}
}

func TestSitePackagesRe(t *testing.T) {
	for p, want := range map[string]string{
		`C:\proj\.venv\Lib\site-packages\pip\_vendor\certifi\cacert.pem`:                                                           `C:\proj\.venv`,
		`C:\Users\a\AppData\Local\Packages\PSF.Python.3.13_x\LocalCache\local-packages\Python313\site-packages\certifi\cacert.pem`: `C:\Users\a\AppData\Local\Packages\PSF.Python.3.13_x\LocalCache\local-packages`,
		`C:\tools\foo\cacert.pem`: "",
	} {
		got := ""
		if m := sitePackagesRe.FindStringSubmatch(p); m != nil {
			got = m[1]
		}
		if got != want {
			t.Errorf("%s: got %q want %q", p, got, want)
		}
	}
}

func TestCleanProgramName(t *testing.T) {
	for _, c := range []struct{ name, version, want string }{
		{"Signal 8.32.0", "8.32.0", "Signal"},
		{"Model Studio 0.4.26+4", "0.4.26+4", "Model Studio"},
		{"Mozilla Firefox (x64 en-US)", "158.0.1", "Mozilla Firefox"},
		{"7-Zip 24.08 (x64)", "24.08", "7-Zip"},
		{"Python 3.14.9 (64-bit)", "3.14.9150.0", "Python"},
		{"CodeEditor (User)", "1.140", "CodeEditor (User)"},
		{"Microsoft Edge WebView2 Runtime", "155.0.4283.62", "Microsoft Edge WebView2 Runtime"},
		{"DrawApp", "1.4.2", "DrawApp"},
		{"1.2.3", "", "1.2.3"},
		{"Example App version 2.1.0", "2.1.0", "Example App"},
		{"Example Editor Version 1.40", "", "Example Editor"},
		{"Version Manager 2.0", "2.0", "Version Manager"},
	} {
		if got := cleanProgramName(c.name, c.version); got != c.want {
			t.Errorf("cleanProgramName(%q) = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestGuessProgram(t *testing.T) {
	root := t.TempDir()
	mk := func(rel string) string {
		p := filepath.Join(root, filepath.FromSlash(rel))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte("x"), 0o644)
		return p
	}
	mk("W/Dev/proj/.venv/pyvenv.cfg")
	mk("Work/gpu_check/xpu/pyvenv.cfg")
	for _, c := range []struct{ rel, want string }{
		{"Portable/radio-next/certifi/cacert.pem", "radio-next"},
		{"Install/Tools/Sideloader_0.9.51/Sideloader.pem", "Sideloader"},
		{"W/Dev/proj/.venv/Lib/site-packages/certifi/cacert.pem", "proj"},
		{"Work/gpu_check/xpu/Lib/site-packages/pip/_vendor/certifi/cacert.pem", "gpu_check"},
		{"Portable/aitools/ai-connect.exe", "aitools"},
		{"Portable/cacert.pem", ""},
		{"Tools/anaconda3/Library/ssl/cacert.pem", "anaconda3"},
	} {
		if got := guessProgram(mk(c.rel)); got != c.want {
			t.Errorf("guessProgram(%s) = %q, want %q", c.rel, got, c.want)
		}
	}
}

func TestDisplayFields(t *testing.T) {
	it := Item{Kind: "bundle", Name: "certifi", Owner: "p2papp", OwnerURL: "https://p2p.example.org", ProductURL: "https://pypi.org/project/certifi/", Bundled: true}
	it.fillDisplay("")
	if it.Program != "p2papp" || it.Component != "certifi" || it.Homepage() != "https://p2p.example.org" {
		t.Errorf("bundle: %q %q %q", it.Program, it.Component, it.Homepage())
	}
	it = Item{Kind: "git", Name: "Git for Windows", Owner: "Example Device Builder", ProductURL: "https://gitforwindows.org/", Bundled: true}
	it.fillDisplay("")
	if it.Homepage() != "" || it.SearchText() != "Example Device Builder" {
		t.Errorf("bundled copy without owner page: homepage %q, search %q", it.Homepage(), it.SearchText())
	}
	it = Item{Kind: "git", Name: "Git for Windows", Owner: "Git", ProductURL: "https://gitforwindows.org/"}
	it.fillDisplay("")
	if it.Homepage() != "https://gitforwindows.org/" {
		t.Errorf("standalone: homepage %q", it.Homepage())
	}
	it = Item{Group: GroupNet, Kind: "browser", Name: "Tor Browser", Version: "140.17.0"}
	it.fillDisplay("")
	if it.Program != "Tor Browser" || it.Component != "" || itemTitle(it) != "Tor Browser 140.17.0" {
		t.Errorf("browser: %q %q %q", it.Program, it.Component, itemTitle(it))
	}
	it = Item{Group: GroupNet, Kind: "electron", Name: "Signal", Owner: "Signal", Version: "44.9.1", ProgVersion: "8.32.0"}
	it.fillDisplay("Electron")
	if it.Program != "Signal" || it.Component != "Electron" || itemTitle(it) != "Signal 8.32.0 › Electron 44.9.1" {
		t.Errorf("electron: %q %q %q", it.Program, it.Component, itemTitle(it))
	}
	it = Item{Kind: "python", Name: "Python", Owner: "Python", Version: "3.14.9"}
	it.fillDisplay("")
	if it.Component != "" {
		t.Errorf("same program and component should show once: %q", it.Component)
	}
}

func TestVersionsShown(t *testing.T) {
	it := Item{Group: GroupApps, Kind: "bundle", Name: "certifi", Version: "2025.11.12", Owner: "DrawApp", OwnerVer: "1.4.2", Bundled: true}
	it.fillDisplay("")
	if it.ProgVersion != "1.4.2" || appCell(it, colComponent) != "certifi 2025.11.12" || itemTitle(it) != "DrawApp 1.4.2 › certifi 2025.11.12" {
		t.Errorf("bundled: %q %q %q", it.ProgVersion, appCell(it, colComponent), itemTitle(it))
	}
	it = Item{Group: GroupApps, Kind: "tool", Name: "chatcli", Version: "0.30.0", Owner: "chatcli", OwnerVer: "0.30.0"}
	it.fillDisplay("")
	if it.ProgVersion != "0.30.0" || appCell(it, colComponent) != "" {
		t.Errorf("tool: %q %q", it.ProgVersion, appCell(it, colComponent))
	}
	root := t.TempDir()
	p := filepath.Join(root, "Install", "Sideloader_0.9.51", "Sideloader.pem")
	os.MkdirAll(filepath.Dir(p), 0o755)
	os.WriteFile(p, []byte("x"), 0o644)
	it = Item{Group: GroupApps, Kind: "bundle", Name: "Certificate bundle", File: p}
	it.fillDisplay("")
	if it.Program != "Sideloader" || it.ProgVersion != "0.9.51" {
		t.Errorf("guessed: %q %q", it.Program, it.ProgVersion)
	}
}

func TestPackages(t *testing.T) {
	home, pd := t.TempDir(), t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("ProgramData", pd)
	t.Setenv("ChocolateyInstall", "")
	write := func(p, data string) {
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(data), 0o644)
	}
	write(filepath.Join(home, "scoop", "apps", "ocrtool", "current", "manifest.json"), `{"version":"5.5.1","homepage":"https://ocrtool.github.io"}`)
	write(filepath.Join(pd, "chocolatey", "lib", "cleanerapp", "cleanerapp.nuspec"), `<?xml version="1.0"?><package><metadata><id>cleanerapp</id><version>5.0.2</version><title>CleanerApp</title></metadata></package>`)
	tp := &appsContext{scoopApps: map[string]scoopManifest{}}
	tp.loadChocolatey()

	tess := filepath.Join(home, "scoop", "apps", "ocrtool", "current", "ocrtool.exe")
	if o := tp.ownerOf(tess); o.Name != "ocrtool" || o.Version != "5.5.1" {
		t.Errorf("scoop owner: %+v", o)
	}
	fixFile := T("fix.orDirectly") + "\n$ copy a b"
	it := Item{Group: GroupApps, Kind: "tool", Name: "ocrtool", Owner: "ocrtool", OwnerVer: "5.5.1", File: tess, Status: StCaution, Details: "May trust.\n\n" + fixFile}
	it.fillDisplay("")
	tp.packageAdvice(&it)
	if len(it.Packages) != 1 || it.Packages[0].Manager != "Scoop" || !strings.Contains(it.Details, "$ scoop update ocrtool") {
		t.Errorf("scoop package: %+v\n%s", it.Packages, it.Details)
	}
	one, all, direct := strings.Index(it.Details, "$ scoop update ocrtool"), strings.Index(it.Details, "$ scoop update -a"), strings.Index(it.Details, fixFile)
	if !(one > 0 && all > one && direct > all) || !strings.Contains(it.Details, Tf("fix.pkgAll", "Scoop")) {
		t.Errorf("order of the advice:\n%s", it.Details)
	}
	tp.packageAdvice(&it) // run again: no second command
	if strings.Count(it.Details, "scoop update") != 2 {
		t.Errorf("duplicate command:\n%s", it.Details)
	}
	// the Fix function runs the update of this package, not that of all packages
	if f := fixesIn(it.Details); len(f) != 1 || f[0].Kind != fixPackage || f[0].Admin || f[0].key() != "scoop update ocrtool" || f[0].Manager != "Scoop" {
		t.Errorf("fixes: %+v", f)
	}
	if got := itemTitle(it); got != "ocrtool 5.5.1 [Scoop]" {
		t.Errorf("title: %q", got)
	}
	runs := packageRuns(it)
	if len(runs) != 1 || runs[0].kind != runLink || runs[0].text != "ocrtool [Scoop]" {
		t.Errorf("package row: %+v", runs)
	}
	ink := Item{Group: GroupApps, Kind: "bundle", Name: "certifi", Owner: "DrawApp", Status: StWarn}
	ink.fillDisplay("")
	tp.packageAdvice(&ink)
	if len(ink.Packages) != 0 || packageRuns(ink) != nil || strings.Contains(ink.Details, "scoop") {
		t.Errorf("not installed with a package manager: %+v %q", ink.Packages, ink.Details)
	}
	bb := Item{Group: GroupApps, Kind: "bundle", Name: "Certificate bundle", Owner: "CleanerApp", Status: StWarn}
	bb.fillDisplay("")
	tp.packageAdvice(&bb)
	if len(bb.Packages) != 1 || bb.Packages[0].Manager != "Chocolatey" || !strings.Contains(bb.Details, "$ choco upgrade cleanerapp -y") || !strings.Contains(bb.Details, "$ choco upgrade all") {
		t.Errorf("chocolatey: %+v %q", bb.Packages, bb.Details)
	}
	if f := fixesIn(bb.Details); len(f) != 1 || !f[0].Admin || f[0].key() != "choco upgrade cleanerapp -y" {
		t.Errorf("chocolatey fixes: %+v", f)
	}
}

type nopReporter struct{}

func (nopReporter) Log(LogKind, Status, string) {}
func (nopReporter) Upsert(Item)                 {}
func (nopReporter) Activity(string)             {}

// "Scan again" must show the new version of a program updated since the scan.
func TestRecheckUpdatesVersions(t *testing.T) {
	c := newChecker(nopReporter{}, nil)
	ownerVer, javaVer, file := "3.2.1", "25.0.4", `C:\Program Files\Sample PDF\runtime\jre\lib\security\cacerts`
	c.refresh = func() { ownerVer, javaVer, file = "3.2.2", "25.0.5", "" }
	it := c.begin(GroupApps, "java", "Java", "", func(it *Item) {
		r := result{Status: StWarn, Version: javaVer, File: file, Owner: "Sample PDF", OwnerVer: ownerVer}
		if file == "" {
			r.Status = StOK
		}
		c.finish(it, r)
	})
	if got := itemTitle(*it); got != "Sample PDF 3.2.1 › Java 25.0.4" {
		t.Fatalf("first check: %q", got)
	}
	c.recheck([]int{it.ID})
	if got := itemTitle(*it); got != "Sample PDF 3.2.2 › Java 25.0.5" {
		t.Errorf("after Scan again: %q", got)
	}
	if it.File != "" || it.Status != StOK {
		t.Errorf("after Scan again: file %q, status %v", it.File, it.Status)
	}
}

// The last update is shown in the report and the details, with where it comes from.
func TestLastUpdateShown(t *testing.T) {
	Lang = 0
	c := newChecker(nopReporter{}, nil)
	when := time.Date(2026, 10, 8, 14, 8, 0, 0, time.Local)
	c.dated = func(kind string, r result) lastUpdate {
		return lastUpdate{Time: when, Src: dateSigned, File: r.Exe}
	}
	c.begin(GroupApps, "electron", "Model Studio", "", func(it *Item) {
		c.finish(it, result{Status: StOK, Version: "0.4.26", Exe: `C:\Program Files\Model Studio\Model Studio.exe`})
	})
	rep := buildReport(c, "")
	if want := `Software date: 8 October 2026 (digital signature of C:\Program Files\Model Studio\Model Studio.exe)`; !strings.Contains(rep, want) {
		t.Errorf("report without %q:\n%s", want, rep)
	}
	it := c.snapshot()[0]
	if got := appCell(it, colUpdated); got != shortDate(when) {
		t.Errorf("list cell %q", got)
	}
	text := blocksText(detailBlocks(it))
	if want := "Software date: 8 October 2026 (digital signature of Model Studio.exe)"; !strings.Contains(text, want) {
		t.Errorf("details without %q:\n%s", want, text)
	}
	Lang = 1
	defer func() { Lang = 0 }()
	if got := longDate(time.Date(2026, 10, 1, 12, 0, 0, 0, time.Local)); got != "1er octobre 2026" {
		t.Errorf("French date %q", got)
	}
}

// The date of a software comes from its main program, not from the first program
// checked (ocrtool tools: ocrtrain.exe…), and an unknown date says why.
func TestMainProgramOfSoftware(t *testing.T) {
	Lang = 0
	root := t.TempDir()
	cur := filepath.Join(root, "scoop", "apps", "ocrtool", "current")
	os.MkdirAll(cur, 0o755)
	for _, n := range []string{"ocrtrain.exe", "ocrtool.exe", "ocrimage.exe"} {
		os.WriteFile(filepath.Join(cur, n), []byte("MZ"), 0o644)
	}
	tp := &appsContext{scoopApps: map[string]scoopManifest{}, allDrives: true}
	first := filepath.Join(cur, "ocrtrain.exe")
	r := result{Exe: first, File: first, Folder: cur}
	tp.withOwner(&r, first)
	if r.Owner != "ocrtool" {
		t.Fatalf("owner %q", r.Owner)
	}
	if got := filepath.Base(tp.mainProgram(r)); got != "ocrtool.exe" {
		t.Errorf("main program of ocrtool: %s", got)
	}
	// a certificate bundle in a portable folder: the program of that folder
	radio := filepath.Join(root, "Portable", "radio-next")
	os.MkdirAll(filepath.Join(radio, "certifi"), 0o755)
	os.WriteFile(filepath.Join(radio, "radio.exe"), []byte("MZ"), 0o644)
	os.WriteFile(filepath.Join(radio, "certifi", "cacert.pem"), []byte("x"), 0o644)
	if got := filepath.Base(tp.mainProgram(result{File: filepath.Join(radio, "certifi", "cacert.pem")})); got != "radio.exe" {
		t.Errorf("main program of radio-next: %s", got)
	}
	u := lastUpdate{File: `C:\Tools\Tor Browser\Browser\firefox.exe`, Unknown: true}
	if got := u.text(u.File); got != `unknown (C:\Tools\Tor Browser\Browser\firefox.exe has no usable date)` {
		t.Errorf("unknown date: %q", got)
	}
}

func TestGeckoBuildDate(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "application.ini"), []byte("[App]\r\nVendor=Mozilla\r\nName=Firefox\r\nVersion=140.17.0esr\r\nBuildID=20261020114512\r\n"), 0o644)
	got, ini, ok := geckoBuildDate(dir)
	if !ok || filepath.Base(ini) != "application.ini" || got.Format("2006-01-02") != "2026-10-20" {
		t.Errorf("build date %v %s %v", got, ini, ok)
	}
}

// TestAppDate: the date of the version is a real date, written YYYYMMDD.
func TestAppDate(t *testing.T) {
	if _, err := time.Parse("20060102", appDate); err != nil || len(appDate) != 8 {
		t.Fatalf("appDate %q: %v", appDate, err)
	}
	if got := appTitle(); got != "eMudScan "+appVersion+" ("+appDate+")"+simulationMark {
		t.Fatalf("appTitle() = %q", got)
	}
	// a build that is not of a release shows its commit instead of the date
	defer func(r string) { buildRev = r }(buildRev)
	buildRev = "ae7832d-modified"
	if got := appTitle(); got != "eMudScan "+appVersion+"+dev (ae7832d-modified)"+simulationMark {
		t.Fatalf("appTitle() of a build that is not of a release = %q", got)
	}
}

// TestSystemCurlNeedsItsDrive: Windows' own curl.exe is reported only when the drive
// that holds Windows is scanned.
func TestSystemCurlNeedsItsDrive(t *testing.T) {
	Lang = 0
	root := t.TempDir()
	win := filepath.Join(root, "C", "Windows")
	os.MkdirAll(filepath.Join(win, "System32"), 0o755)
	os.WriteFile(filepath.Join(win, "System32", "curl.exe"), []byte("MZ"), 0o644)
	other := filepath.Join(root, "U")
	os.MkdirAll(other, 0o755)
	for _, tc := range []struct {
		drives []string
		want   bool
	}{{[]string{other}, false}, {[]string{filepath.Join(root, "C"), other}, true}} {
		c := newChecker(nopReporter{}, nil)
		tp := &appsContext{c: c, d: newDiscovery(), winDir: win, drives: tc.drives}
		tp.checkCurl()
		found := false
		for _, it := range c.snapshot() {
			found = found || it.Kind == "curl"
		}
		if found != tc.want {
			t.Errorf("drives %v: Windows' curl reported = %v, want %v", tc.drives, found, tc.want)
		}
	}
}

// eMudScan runs on Windows 10 version 1809 and Windows Server 2019 (build 17763) and
// later Windows, not on older ones such as Windows Server 2016 (build 14393).
func TestWindowsRecentEnough(t *testing.T) {
	for _, tc := range []struct {
		major, build uint32
		want         bool
	}{{10, 26200, true}, {10, 22000, true}, {10, 17763, true}, {10, 17134, false}, {10, 15063, false}, {10, 14393, false}, {10, 10240, false}, {6, 9600, false}, {6, 7601, false}, {11, 1, true}} {
		if got := windowsRecentEnough(tc.major, tc.build); got != tc.want {
			t.Errorf("%d.%d: %v, want %v", tc.major, tc.build, got, tc.want)
		}
	}
}

// TestStale: software that needs an update and has had no new version for 12
// months or more is flagged, with its age; OK software never is.
func TestStale(t *testing.T) {
	Lang = 0
	now := time.Date(2026, 10, 26, 19, 0, 0, 0, time.Local)
	at := func(y int, m time.Month, d int) lastUpdate {
		return lastUpdate{Time: time.Date(y, m, d, 12, 0, 0, 0, time.Local), Src: dateModified}
	}
	for _, tc := range []struct {
		date   lastUpdate
		status Status
		want   int
	}{
		{at(2025, 7, 7), StWarn, 15},    // chatcli
		{at(2025, 10, 26), StWarn, 12},  // exactly a year
		{at(2025, 10, 27), StWarn, 0},   // a day short of a year
		{at(2024, 4, 1), StCaution, 30}, // a warning too
		{at(2025, 7, 7), StOK, 0},       // OK: not flagged
		{at(2025, 7, 7), StInfo, 0},     // information: not flagged
		{lastUpdate{Unknown: true}, StWarn, 0},
	} {
		it := Item{Group: GroupApps, Status: tc.status, Updated: tc.date}
		if got := it.staleMonths(now); got != tc.want {
			t.Errorf("%v %v: %d months, want %d", tc.date.Time.Format("2006-01-02"), tc.status, got, tc.want)
		}
	}
	if ageText(15) != "15 months" || ageText(30) != "2 years" {
		t.Errorf("ageText: %q, %q", ageText(15), ageText(30))
	}
}

// Every part of a program shows the same version: the one Windows or its package
// manager records, else its own. A program checked itself shows the version found in
// it.
func TestProgramVersionFromOnePlace(t *testing.T) {
	electron := Item{Kind: "electron", Name: "Model Studio", Owner: "Model Studio", OwnerVer: "0.4.26+4", ProgVersion: "0.4.26", Version: "38.6.0"}
	electron.fillDisplay("Electron")
	node := Item{Kind: "node", Name: "Node.js", Owner: "Model Studio", OwnerVer: "0.4.26+4", Version: "25.5.0"}
	node.fillDisplay("")
	// the Python install manager records its tag as the version in Apps & features
	python := Item{Kind: "python", Name: "Python", Owner: "Python", OwnerVer: "3.14-64", Version: "3.14.10"}
	python.fillDisplay("")
	alone := Item{Kind: "tool", Name: "chatcli", Version: "0.30.0"}
	alone.fillDisplay("")
	for _, c := range []struct {
		it   Item
		want string
	}{{electron, "0.4.26+4"}, {node, "0.4.26+4"}, {python, "3.14.10"}, {alone, "0.30.0"}} {
		if c.it.ProgVersion != c.want {
			t.Errorf("%s › %s: version %q, want %q", c.it.Program, c.it.Component, c.it.ProgVersion, c.want)
		}
	}
}

// The top of the report: eMudScan's version and the date, Windows, then who made
// eMudScan.
func TestReportHeader(t *testing.T) {
	rep := strings.ReplaceAll(buildReport(newChecker(nopReporter{}, nil), ""), "\r\n", "\n")
	lines := strings.SplitN(rep, "\n", 6)
	line := strings.Repeat("=", 78)
	if len(lines) < 6 || lines[0] != line || lines[4] != line ||
		!strings.HasPrefix(lines[1], T("report.title")+" "+appVersionDate()+simulationMark+" · ") ||
		lines[2] != windowsVersionString() || lines[3] != Tf("report.by", appAuthor, appURL) {
		t.Errorf("top of the report:\n%s", strings.Join(lines[:min(5, len(lines))], "\n"))
	}
}
