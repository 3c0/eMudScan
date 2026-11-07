package main

import (
	"crypto/sha256"
	"debug/pe"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// pyLayout lays out a Python in home: python.exe for machine, and in sslDir (DLLs, or
// home itself for an embeddable package) _ssl.pyd and the named files.
func pyLayout(t *testing.T, home string, machine uint16, sslDir string, files ...string) string {
	t.Helper()
	fakePython(t, home, false)
	fakeExeFor(t, filepath.Join(home, "python.exe"), machine)
	dir := filepath.Join(home, sslDir)
	for _, n := range append([]string{"_ssl.pyd"}, files...) {
		os.MkdirAll(dir, 0o755)
		os.WriteFile(filepath.Join(dir, n), []byte("old "+n), 0o644)
	}
	return filepath.Join(dir, "_ssl.pyd")
}

// The source of the fixed ssl files follows the layout of the Python and its minor
// version: python-build-standalone's for its own layout and for python.org's 3.11 to
// 3.13 (with _hashlib.pyd, and the two libraries added under its names), python.org's
// embeddable package for python.org's 3.14 and 3.15. Nothing for another processor, a
// free-threaded Python, another layout, or a version without a fixed build.
func TestPySSLPlan(t *testing.T) {
	root := t.TempDir()
	for _, c := range []struct {
		name, fixed    string
		machine        uint16
		sslDir         string
		files          []string
		extra          string // another file in home
		src            string // "" for no plan
		replace, added string
	}{
		{"python-build-standalone", "3.11.18", pe.IMAGE_FILE_MACHINE_AMD64, "DLLs", []string{"libssl-3-x64.dll", "libcrypto-3-x64.dll", "_hashlib.pyd"}, "",
			pbsSSL["3.11"].url, "_ssl.pyd libssl-3-x64.dll libcrypto-3-x64.dll", ""},
		{"python.org 3.14, embeddable", "3.14.10", pe.IMAGE_FILE_MACHINE_AMD64, ".", []string{"libssl-3.dll", "libcrypto-3.dll", "_hashlib.pyd"}, "",
			pyorgSSL["3.14"].url, "_ssl.pyd libssl-3.dll libcrypto-3.dll", ""},
		{"python.org 3.12", "3.12.16", pe.IMAGE_FILE_MACHINE_AMD64, "DLLs", []string{"libssl-3.dll", "libcrypto-3.dll", "_hashlib.pyd"}, "",
			pbsSSL["3.12"].url, "_ssl.pyd _hashlib.pyd", "libssl-3-x64.dll libcrypto-3-x64.dll"},
		{"ARM64", "3.11.18", pe.IMAGE_FILE_MACHINE_ARM64, "DLLs", []string{"libssl-3-x64.dll", "libcrypto-3-x64.dll"}, "", "", "", ""},
		{"free-threaded", "3.13.18", pe.IMAGE_FILE_MACHINE_AMD64, "DLLs", []string{"libssl-3-x64.dll", "libcrypto-3-x64.dll"}, "python313t.dll", "", "", ""},
		{"OpenSSL 1.1", "3.11.18", pe.IMAGE_FILE_MACHINE_AMD64, "DLLs", []string{"libssl-1_1.dll", "libcrypto-1_1.dll", "_hashlib.pyd"}, "", "", "", ""},
		{"a version without a fixed build", "3.12.17", pe.IMAGE_FILE_MACHINE_AMD64, "DLLs", []string{"libssl-3-x64.dll", "libcrypto-3-x64.dll"}, "", "", "", ""},
		{"a later release pinned", "3.12.15", pe.IMAGE_FILE_MACHINE_AMD64, "DLLs", []string{"libssl-3-x64.dll", "libcrypto-3-x64.dll"}, "",
			pbsSSL["3.12"].url, "_ssl.pyd libssl-3-x64.dll libcrypto-3-x64.dll", ""},
		{"3.10", "3.10.20", pe.IMAGE_FILE_MACHINE_AMD64, "DLLs", []string{"libssl-3-x64.dll", "libcrypto-3-x64.dll"}, "", "", "", ""},
	} {
		home := filepath.Join(root, strings.ReplaceAll(c.name, " ", "-"))
		ssl := pyLayout(t, home, c.machine, c.sslDir, c.files...)
		if c.extra != "" {
			os.WriteFile(filepath.Join(home, c.extra), nil, 0o644)
		}
		plan, ok := pySSLPlanFor(pythonEnvFor(filepath.Join(home, "python.exe")), ssl, c.fixed)
		if ok != (c.src != "") || ok && (plan.src.url != c.src || strings.Join(plan.replace, " ") != c.replace ||
			strings.Join(plan.added, " ") != c.added || plan.dir != filepath.Dir(ssl)) {
			t.Errorf("%s: %v %+v", c.name, ok, plan)
		}
	}
}

// The pinned releases: each of its line, fixed, its version in its URL, its SHA-256
// written in full.
func TestPySSLPins(t *testing.T) {
	for _, m := range []map[string]pySSLSource{pbsSSL, pyorgSSL} {
		for minor, s := range m {
			v := ParseVersion(s.version)
			state, _ := fixStatus("python", v)
			if len(v) != 3 || fmt.Sprintf("%d.%d", v[0], v[1]) != minor || state != FixOK ||
				!strings.Contains(s.url, "/"+s.version+"/") && !strings.Contains(s.url, "-"+s.version+"%2B") || len(s.sha256) != 64 {
				t.Errorf("%s: %+v (%v)", minor, s, state)
			}
		}
	}
}

// The commands: the checks before, the download checked against its SHA-256, its
// files, nothing when a file is in use, a backup of each file and a mark for each added
// one (named after the same time), the copy, the checks after, and, when they fail, the
// original files back and the added ones removed. Optional, with the checks' files.
func TestPySSLFix(t *testing.T) {
	Lang = 0
	root := t.TempDir()
	home := filepath.Join(root, "app", "python")
	ssl := pyLayout(t, home, pe.IMAGE_FILE_MACHINE_AMD64, "DLLs", "libssl-3.dll", "libcrypto-3.dll", "_hashlib.pyd")
	exe := filepath.Join(home, "python.exe")
	plan, ok := pySSLPlanFor(pythonEnvFor(exe), ssl, "3.12.16")
	if !ok {
		t.Fatal("no plan")
	}
	f := pySSLFix(exe, plan)
	all := strings.Join(f.Cmds, "\n")
	for _, want := range []string{
		"$pyExe = " + psPath(exe, false) + "; $pyDir = " + psPath(filepath.Dir(ssl), false) + "; $pyFiles = '_ssl.pyd', '_hashlib.pyd'; $pyAdded = 'libssl-3-x64.dll', 'libcrypto-3-x64.dll'",
		"& $pyExe -I (Join-Path $emudscanFiles 'check_ssl.py') $emudscanFiles before; $pyOK = $LASTEXITCODE -eq 0",
		// once for the Pythons fixed in the same run from the same download, with the same files
		"$pyArchive = Join-Path $emudscanFiles 'python-3.12.16-standalone.tar.gz'; if ($pyOK -and -not (Test-Path -LiteralPath $pyArchive)) {",
		"-Uri '" + pbsSSL["3.12"].url + "' -OutFile $pyArchive", "(Get-FileHash -LiteralPath $pyArchive).Hash -eq '" + pbsSSL["3.12"].sha256 + "'",
		"$pyNew = Join-Path $emudscanFiles 'python-3.12.16-standalone'; if ($pyOK -and -not (Test-Path -LiteralPath $pyNew)) {",
		`tar.exe" -xzf $pyArchive -C $pyNew python/DLLs/_ssl.pyd python/DLLs/_hashlib.pyd python/DLLs/libssl-3-x64.dll python/DLLs/libcrypto-3-x64.dll }; $pyNew = Join-Path $pyNew 'python\DLLs';`,
		"[IO.File]::Open((Join-Path $pyDir $n), 'Open', 'ReadWrite', 'None')",
		`"$n.emudscan-$pyStamp.bak"`, `"$n.emudscan-$pyStamp.added"`,
		"$pyBak.CreationTimeUtc = (Get-Item -LiteralPath (Join-Path $pyDir $n)).CreationTimeUtc", // the creation date of the file
		"Copy-Item -LiteralPath (Join-Path $pyNew $n) -Destination (Join-Path $pyDir $n) -Force",
		"$emudscanFiles after; $pyGood = $LASTEXITCODE -eq 0",
		"if ($pyOK -and -not $pyGood) { foreach ($n in $pyFiles) { Move-Item",
	} {
		if !strings.Contains(all, want) {
			t.Errorf("no %q in\n%s", want, all)
		}
	}
	if f.Kind != fixPySSL || !f.Optional || f.Needs != "pycheck" || len(f.Files) != 4 || !backsUp([]Item{{Fixes: []fixAction{f}}}) || !fetchesRight(f) {
		t.Errorf("%+v", f)
	}
	// without "Back up the files to change": the same, then the backups and marks deleted once the checks pass
	if len(f.Plain) != len(f.Cmds)+1 || strings.Join(f.Plain[:len(f.Cmds)], "\n") != all ||
		f.Plain[len(f.Cmds)] != `if ($pyGood) { foreach ($n in $pyFiles) { Remove-Item -LiteralPath (Join-Path $pyDir "$n.emudscan-$pyStamp.bak") -Force }; foreach ($n in $pyAdded) { Remove-Item -LiteralPath (Join-Path $pyDir "$n.emudscan-$pyStamp.added") -Force } }` {
		t.Errorf("plain:\n%s", strings.Join(f.Plain, "\n"))
	}
	// python.org's package: unzipped
	home2 := filepath.Join(root, "app2", "python")
	ssl2 := pyLayout(t, home2, pe.IMAGE_FILE_MACHINE_AMD64, ".", "libssl-3.dll", "libcrypto-3.dll", "_hashlib.pyd")
	plan2, _ := pySSLPlanFor(pythonEnvFor(filepath.Join(home2, "python.exe")), ssl2, "3.14.10")
	if all := strings.Join(pySSLFix(filepath.Join(home2, "python.exe"), plan2).Cmds, "\n"); !strings.Contains(all, "[IO.Compression.ZipFile]::ExtractToDirectory($pyArchive, $pyNew)") || strings.Contains(all, "tar.exe") ||
		!strings.Contains(all, "'python-3.14.10-embed.zip'") || !strings.Contains(all, "$pyNew = Join-Path $emudscanFiles 'python-3.14.10-embed';") {
		t.Errorf("zip:\n%s", all)
	}
}

// A bundled Python, outdated, gets the offer after its program's update, before pip's
// command, with the warning about changing a copy before its commands; not ticked by
// default.
func TestPySSLOffer(t *testing.T) {
	Lang = 0
	root := t.TempDir()
	app := filepath.Join(root, "app")
	home := filepath.Join(app, "python")
	pyLayout(t, home, pe.IMAGE_FILE_MACHINE_AMD64, "DLLs", "libssl-3-x64.dll", "libcrypto-3-x64.dll", "_hashlib.pyd")
	sp := filepath.Join(home, "Lib", "site-packages")
	os.MkdirAll(filepath.Join(sp, "certifi"), 0o755)
	os.WriteFile(filepath.Join(sp, "certifi", "cacert.pem"), outdatedBundle(t), 0o644)
	tp := testApps(t)
	tp.owners = []ownerDir{{dir: app, name: "Example App 1", version: "1.0.3"}}
	exe := filepath.Join(home, "python.exe")
	got := tp.c.begin(GroupApps, "python", "Python", home, func(it *Item) {
		env := pythonEnvFor(exe)
		env.version = "3.11.9"
		tp.pythonOneEnv(it, env)
	})
	d, at := got.Details, 0
	for _, w := range []string{"Recommended action: update Example App 1", Tf("pyssl.offer", "3.11.18", "python-build-standalone", "Example App 1"),
		T("bundled.warn"), "$ $pyExe = ", Tf("py.untilThenAlso", "certifi"), "pip install --upgrade certifi"} {
		i := strings.Index(d[at:], w)
		if i < 0 {
			t.Fatalf("%q missing or out of order:\n%s", w, d)
		}
		at += i + len(w)
	}
	if f := fixesIn(d); len(f) != 2 || f[0].Kind != fixPySSL || !f[0].Optional || f[1].Kind != fixPip {
		t.Errorf("fixes: %+v", f)
	}
}

// The runner writes the checks' files next to the script, which checks their SHA-256
// before anything runs.
func TestFixFiles(t *testing.T) {
	units := []fixUnit{{fix: fixAction{Kind: fixPip, Cmds: []string{"x"}}, cmds: []string{"x"}}, {fix: fixAction{Kind: fixPySSL, Needs: "pycheck", Cmds: []string{"y"}}, cmds: []string{"y"}}}
	files, err := fixFiles(units, time.Now())
	if err != nil || len(files) != 5 || string(files["check_ssl.py"]) != pyCheckScript {
		t.Fatalf("%v %d", err, len(files))
	}
	if f, _ := fixFiles(units[:1], time.Now()); len(f) != 0 {
		t.Errorf("no helper files needed: %d", len(f))
	}
	s := fixScript(units, `C:\Temp\fix.log`, "##m", files)
	sum := sha256.Sum256(files["denied.pem"])
	want := "if ((Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $emudscanFiles 'denied.pem')).Hash -ne '" + strings.ToUpper(hex.EncodeToString(sum[:])) + "') { exit 97 }"
	if !strings.Contains(s, "$emudscanFiles = Split-Path -Parent $emudscanLog\n") || !strings.Contains(s, want) || strings.Index(s, want) > strings.Index(s, "cmd 0 0") {
		t.Errorf("script:\n%s", s[:min(len(s), 1500)])
	}
}

// A file that a fix added has a mark: restoring removes the file and its mark.
func TestRestoreAdded(t *testing.T) {
	Lang = 0
	dir := t.TempDir()
	dll := filepath.Join(dir, "libssl-3-x64.dll")
	mark := dll + ".emudscan-20261107-120000.added"
	os.WriteFile(dll, []byte("new"), 0o644)
	os.WriteFile(mark, nil, 0o644)
	a := restoreAdvice(dll)
	f := fixesIn(a)
	if !strings.Contains(a, Tf("restore.added", longDate(time.Date(2026, 11, 7, 12, 0, 0, 0, time.Local)), "12:00")) || len(f) != 1 ||
		f[0].Cmds[0] != "Remove-Item -LiteralPath "+psPath(dll, false)+", "+psPath(mark, false)+" -Force" {
		t.Fatalf("%+v\n%s", f, a)
	}
	res := moveBackups([]fixUnit{{fix: f[0]}}, func(fixLine) {})
	if !res[0].ok || !fixSimulation && (exists(dll) || exists(mark)) { // the simulation removes nothing
		t.Errorf("not removed: %+v %v %v", res, exists(dll), exists(mark))
	}
}

// The ssl files of a Python are put back together, from the run that backed up
// _ssl.pyd, when all its backups and marks are there; never some of them alone. After a
// copy interrupted by a crash, the backups the same as their files count too.
func TestPySSLRestoreTogether(t *testing.T) {
	Lang = 0
	dir := t.TempDir()
	write := func(n, data string) string {
		p := filepath.Join(dir, n)
		if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	ssl := write("_ssl.pyd", "new")
	write("_hashlib.pyd", "new")
	write("libssl-3.dll", "old") // left as it was
	write("libssl-3-x64.dll", "new")
	write("libcrypto-3-x64.dll", "new")
	const run, other = ".emudscan-20261107-120000", ".emudscan-20261107-130000"
	restored := func() []string {
		var out []string
		for _, f := range fixesIn(pySSLRestore(ssl)) {
			out = append(out, f.Files...)
		}
		for i, f := range out {
			out[i] = filepath.Base(f)
		}
		return out
	}
	// the marks alone: nothing, the libraries stay for the new _ssl.pyd
	write("libssl-3-x64.dll"+run+".added", "")
	write("libcrypto-3-x64.dll"+run+".added", "")
	if got := restored(); len(got) != 0 {
		t.Errorf("marks alone: %v", got)
	}
	// with the backups of the same run: all of them
	write("_ssl.pyd"+run+".bak", "old")
	write("_hashlib.pyd"+run+".bak", "old")
	if got := fmt.Sprint(restored()); got != "[_ssl.pyd _hashlib.pyd libssl-3-x64.dll libcrypto-3-x64.dll]" {
		t.Errorf("same run: %s", got)
	}
	// a backup of that run missing, or from another time: nothing
	os.Remove(filepath.Join(dir, "_hashlib.pyd"+run+".bak"))
	if got := restored(); len(got) != 0 {
		t.Errorf("one missing: %v", got)
	}
	write("_hashlib.pyd"+other+".bak", "older")
	if got := restored(); len(got) != 0 {
		t.Errorf("other run: %v", got)
	}
	// python-build-standalone's layout: its 3 files, backed up
	dir2 := t.TempDir()
	for _, n := range []string{"_ssl.pyd", "libssl-3-x64.dll", "libcrypto-3-x64.dll"} {
		os.WriteFile(filepath.Join(dir2, n), []byte("new"), 0o644)
		os.WriteFile(filepath.Join(dir2, n+run+".bak"), []byte("old"), 0o644)
	}
	if got := fixesIn(pySSLRestore(filepath.Join(dir2, "_ssl.pyd"))); len(got) != 1 || len(got[0].Files) != 3 {
		t.Errorf("standalone: %+v", got)
	}
	// a copy interrupted after _ssl.pyd: the libraries not reached are the same as their
	// backups, which count, and are put back too
	for _, n := range []string{"libssl-3-x64.dll", "libcrypto-3-x64.dll"} {
		os.WriteFile(filepath.Join(dir2, n), []byte("old"), 0o644)
	}
	if got := fixesIn(pySSLRestore(filepath.Join(dir2, "_ssl.pyd"))); len(got) != 1 || len(got[0].Files) != 3 {
		t.Errorf("interrupted: %+v", got)
	}
	// python.org's layout, interrupted before the added libraries: _ssl.pyd and
	// _hashlib.pyd are put back, nothing else to remove
	dir3 := t.TempDir()
	os.WriteFile(filepath.Join(dir3, "_ssl.pyd"), []byte("new"), 0o644)
	os.WriteFile(filepath.Join(dir3, "_ssl.pyd"+run+".bak"), []byte("old"), 0o644)
	os.WriteFile(filepath.Join(dir3, "_hashlib.pyd"), []byte("old"), 0o644)
	os.WriteFile(filepath.Join(dir3, "_hashlib.pyd"+run+".bak"), []byte("old"), 0o644)
	for _, n := range []string{"libssl-3-x64.dll", "libcrypto-3-x64.dll"} {
		os.WriteFile(filepath.Join(dir3, n+run+".added"), nil, 0o644)
	}
	got := fixesIn(pySSLRestore(filepath.Join(dir3, "_ssl.pyd")))
	if len(got) != 1 || fmt.Sprint(got[0].Files) != fmt.Sprint([]string{filepath.Join(dir3, "_ssl.pyd"), filepath.Join(dir3, "_hashlib.pyd")}) {
		t.Errorf("interrupted, python.org: %+v", got)
	}
	// nothing changed (a crash before the copy): nothing to restore
	os.WriteFile(filepath.Join(dir3, "_ssl.pyd"), []byte("old"), 0o644)
	if got := pySSLRestore(filepath.Join(dir3, "_ssl.pyd")); got != "" {
		t.Errorf("nothing changed: %s", got)
	}
}

// A software with an optional fix and others has two rows in the Fix window: the
// others ticked, the optional one not; each row runs its own fixes only, both rows
// all of them.
func TestFixParts(t *testing.T) {
	Lang = 0
	pip := fixAction{Kind: fixPip, Cmds: []string{"pip"}}
	ssl := fixAction{Kind: fixPySSL, Optional: true, Cmds: []string{"ssl"}}
	both := Item{ID: 1, Name: "A", Status: StWarn, Fixes: []fixAction{ssl, pip}}
	only := Item{ID: 2, Name: "B", Status: StCaution, Fixes: []fixAction{{Kind: fixPySSL, Optional: true, Cmds: []string{"sslB"}}}}
	if got := fmt.Sprint(fixParts(both), fixParts(only), fixParts(Item{Fixes: []fixAction{pip}})); got != fmt.Sprint([]fixPart{partUsual, partOptional}, []fixPart{partAll}, []fixPart{partAll}) {
		t.Errorf("parts: %s", got)
	}
	// ticked when the window opens: the row of the others, and that of a software whose fixes are all optional not
	usual := fixPick{1: {partUsual}, 2: {partUsual}}.items([]Item{both, only})
	optional := fixPick{1: {partOptional}}.items([]Item{both, only})
	if len(usual) != 1 || len(usual[0].Fixes) != 1 || usual[0].Fixes[0].Kind != fixPip || !tickedFirst(usual) ||
		len(optional) != 1 || len(optional[0].Fixes) != 1 || optional[0].Fixes[0].Kind != fixPySSL || tickedFirst(optional) || tickedFirst([]Item{only}) {
		t.Errorf("rows: %+v %+v", usual, optional)
	}
	if len(both.Fixes) != 2 || len(fixPick(nil).items([]Item{both})[0].Fixes) != 2 || len(fixPick{1: {partUsual, partOptional}}.items([]Item{both})[0].Fixes) != 2 {
		t.Errorf("all: %+v", both)
	}
	// the runner runs the fixes picked, and names only the software that has some
	run := func(pick fixPick) string {
		var calls []string
		r := fixRunner{
			exec: func(units []fixUnit, admin bool, out func(fixLine)) ([]unitResult, error) {
				res := make([]unitResult, len(units))
				for i, u := range units {
					calls = append(calls, strings.Join(u.titles, "+")+":"+strings.Join(u.cmds, ","))
					res[i] = unitResult{ran: true, ok: true}
				}
				return res, nil
			},
			item: func(id int) (Item, bool) {
				if id == 1 {
					return both, true
				}
				return only, true
			},
			recheck: func([]int) {}, stopped: func() bool { return false }, out: func(fixLine) {}, fixed: func(int) {},
			pick: pick,
		}
		r.run([]int{1, 2})
		return strings.Join(calls, " | ")
	}
	for _, c := range []struct {
		pick fixPick
		want string
	}{
		{nil, "A:ssl | A:pip | B:sslB"},
		{fixPick{1: {partUsual}}, "A:pip"},
		{fixPick{1: {partOptional}, 2: {partAll}}, "A:ssl | B:sslB"},
		{fixPick{1: {partUsual, partOptional}}, "A:ssl | A:pip"},
		{fixPick{}, ""},
	} {
		if got := run(c.pick); got != c.want {
			t.Errorf("run %v: %q, want %q", c.pick, got, c.want)
		}
	}
}
