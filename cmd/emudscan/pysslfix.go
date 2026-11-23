package main

// The ssl files of a Python inside a program, which its publisher has not updated yet:
// the fix of CVE-2026-115557 is entirely in the ssl module's library (_ssl.pyd), with
// the OpenSSL libraries it needs (OpenSSL 3.3 or later for Python 3.11.18). eMudScan can
// replace these files with those of the fixed release of the same minor version, for a
// 64-bit Python (x64):
//
//   - a Python laid out as python-build-standalone's (libssl-3-x64.dll), and python.org's
//     3.11 to 3.13, for which python.org publishes no fixed Windows build: from a
//     release of python-build-standalone (on GitHub). For python.org's layout,
//     _hashlib.pyd is replaced too and the two OpenSSL libraries are added beside the
//     old ones, as python-build-standalone names them;
//   - python.org's 3.14 and 3.15 (libssl-3.dll): from python.org's embeddable package.
//
// The commands download one release, by its URL, and check it against its SHA-256,
// both written in them. During the scan, which connects to nothing, they are those of
// the releases written here (pbsSSL, pyorgSSL). When a Fix window opens, eMudScan
// itself looks for the latest release (latest.go) and writes the commands again for
// it: to find the release is the program's work, not the commands'.
// The checks of pycheck.go run
// before and after; when one fails after, the original files are put back. It is an
// optional fix, not ticked by default: it changes a program that its publisher has not
// updated.

import (
	"fmt"
	"path/filepath"
	"strings"
)

// pySSLSource is a download that holds the fixed ssl files of a minor version.
type pySSLSource struct {
	version, url, sha256 string
	zip                  bool   // python.org's embeddable package, its files at its root; else python-build-standalone's archive, in python\DLLs
	tag                  string // python-build-standalone's release, such as "20261110"
}

func (s pySSLSource) from() string {
	if s.zip {
		return "python.org"
	}
	return "python-build-standalone"
}

// name names the download in the fix's folder, and the folder of its files: two
// Pythons fixed in the same run, from the same download, share them.
func (s pySSLSource) name() string {
	if s.zip {
		return "python-" + s.version + "-embed"
	}
	return "python-" + s.version + "-" + s.tag + "-standalone"
}

// pbsFiles are the files taken out of python-build-standalone's archive (from
// python\DLLs), the same for every plan, since the plans share them.
var pbsFiles = []string{"_ssl.pyd", "_hashlib.pyd", "libssl-3-x64.dll", "libcrypto-3-x64.dll"}

// pbsTag is the release of python-build-standalone written here.
const (
	pbsTag      = "20261110"
	pbsReleases = "https://github.com/astral-sh/python-build-standalone/releases/"
	pbsRelease  = pbsReleases + "download/" + pbsTag + "/"
)

// pbsSSL: python-build-standalone pbsTag, x64, by minor version.
var pbsSSL = map[string]pySSLSource{
	"3.11": {version: "3.11.18", url: pbsRelease + "cpython-3.11.18%2B" + pbsTag + "-x86_64-pc-windows-msvc-install_only_stripped.tar.gz", sha256: "f31751f4a8483f5ac7cd77ad83f8dfb80d9f2a7245bff1585e27516c777fb46f", tag: pbsTag},
	"3.12": {version: "3.12.16", url: pbsRelease + "cpython-3.12.16%2B" + pbsTag + "-x86_64-pc-windows-msvc-install_only_stripped.tar.gz", sha256: "355ee61b8207b91574491b1c6dc9b0ebb4408fc56bfafe498f3ef439314b52dc", tag: pbsTag},
	"3.13": {version: "3.13.18", url: pbsRelease + "cpython-3.13.18%2B" + pbsTag + "-x86_64-pc-windows-msvc-install_only_stripped.tar.gz", sha256: "2e0fed09d48989f64c9305da9ff2d37a41c0d13ab2aa6283405b2b620dfbf805", tag: pbsTag},
	"3.14": {version: "3.14.10", url: pbsRelease + "cpython-3.14.10%2B" + pbsTag + "-x86_64-pc-windows-msvc-install_only_stripped.tar.gz", sha256: "32b6cf90a03e2bc15fda8d2ab7ff78c4870f138887d612932650ce764efaab8f", tag: pbsTag},
	"3.15": {version: "3.15.2", url: pbsRelease + "cpython-3.15.2%2B" + pbsTag + "-x86_64-pc-windows-msvc-install_only_stripped.tar.gz", sha256: "ecd90d78accb906e3d47f6cdb409a610da10475f94d15ebdd8cb2e0561b71df6", tag: pbsTag},
}

// pyorgFiles is where python.org has the files of its releases, each in the folder of
// its version.
const pyorgFiles = "https://www.python.org/ftp/python/"

// pyorgSSL: python.org's embeddable packages, x64, by minor version.
var pyorgSSL = map[string]pySSLSource{
	"3.14": {version: "3.14.10", url: pyorgFiles + "3.14.10/python-3.14.10-embed-amd64.zip", sha256: "90905e64a840557d8c1bc4f5b74db3d7ee85e3ffb217611cac1c7813acea4159", zip: true},
	"3.15": {version: "3.15.2", url: pyorgFiles + "3.15.2/python-3.15.2-embed-amd64.zip", sha256: "d012f3aa29cf07436dae8aea1fec0fd195c2612a0b059b1f915ae0afb9ad1f62", zip: true},
}

// pySSLPlan is what the fix changes in dir, the folder of _ssl.pyd: the files it
// replaces, and those it adds, from src.
type pySSLPlan struct {
	src            pySSLSource
	dir            string
	replace, added []string
}

// pySSLShapes are the files that the fix replaces, and adds, by layout.
var pySSLShapes = []struct{ replace, added []string }{
	{[]string{"_ssl.pyd", "libssl-3-x64.dll", "libcrypto-3-x64.dll"}, nil},                      // python-build-standalone's
	{[]string{"_ssl.pyd", "libssl-3.dll", "libcrypto-3.dll"}, nil},                              // python.org's, from python.org
	{[]string{"_ssl.pyd", "_hashlib.pyd"}, []string{"libssl-3-x64.dll", "libcrypto-3-x64.dll"}}, // python.org's, from python-build-standalone
}

// pySSLPlanFor is the plan for the Python of env, whose ssl module's library is
// sslLib, outdated (its version is outdated, fixed is the version that fixes it), from
// the release pinned for its line, that version or a later one: none for another
// processor than x64, a layout that is not one of the two, a free-threaded Python, or
// a minor version without a fixed build; none either for a library that is not in the
// folder of python.exe or in its DLLs folder, where the two layouts have it: a Python
// laid out otherwise is another build, which these files were not made for.
func pySSLPlanFor(env pyEnv, sslLib string, fixed string) (pySSLPlan, bool) {
	v := ParseVersion(fixed)
	if sslLib == "" || len(v) < 2 || peMachine(env.exe) != 0x8664 /* x64 */ {
		return pySSLPlan{}, false
	}
	if d := pathKey(filepath.Dir(sslLib)); d != pathKey(env.home) && d != pathKey(filepath.Join(env.home, "DLLs")) {
		return pySSLPlan{}, false
	}
	minor := fmt.Sprintf("%d.%d", v[0], v[1])
	fixes := func(s pySSLSource) bool { return ParseVersion(s.version).Cmp(v) >= 0 }
	if m, _ := filepath.Glob(filepath.Join(env.home, fmt.Sprintf("python%d%dt.dll", v[0], v[1]))); len(m) > 0 {
		return pySSLPlan{}, false // free-threaded
	}
	dir := filepath.Dir(sslLib)
	has := func(n string) bool { return exists(filepath.Join(dir, n)) }
	switch {
	case has("libssl-3-x64.dll") && has("libcrypto-3-x64.dll"): // python-build-standalone's
		if s, ok := pbsSSL[minor]; ok && fixes(s) {
			return pySSLPlan{s, dir, pySSLShapes[0].replace, nil}, true
		}
	case has("libssl-3.dll") && has("libcrypto-3.dll") && has("_hashlib.pyd"): // python.org's
		if s, ok := pyorgSSL[minor]; ok && fixes(s) {
			return pySSLPlan{s, dir, pySSLShapes[1].replace, nil}, true
		}
		if s, ok := pbsSSL[minor]; ok && fixes(s) {
			return pySSLPlan{s, dir, pySSLShapes[2].replace, pySSLShapes[2].added}, true
		}
	}
	return pySSLPlan{}, false
}

// files are the paths of the files that the plan replaces or adds.
func (p pySSLPlan) files() []string {
	var out []string
	for _, n := range append(append([]string(nil), p.replace...), p.added...) {
		out = append(out, filepath.Join(p.dir, n))
	}
	return out
}

// pySSLFix replaces the ssl files of the Python exe as plan says. It is a script
// (fixAction.Script), which stops at its first command that fails. In order: the
// checks before (the ssl module must load); the download, checked against its SHA-256
// (once for the Pythons fixed in the same run from the same download); its files taken
// out; the files to replace must not be in use (the program closed); a backup of each
// file replaced, with the creation date of its file (see backupThen), and a mark for
// each file added, all named after the same time; then, in a "try", the copy and the
// checks after: when one of them fails, its "catch" puts the original files back and
// removes the added ones. The backups are made in any case, for the files to be put
// back; without "Back up the files to change" (Plain), they and the marks are deleted
// at the end, which is reached only when the checks after pass.
//
// It has two versions (pySSLScript). The Fix function runs the one above, with the
// checks of pycheck.go, which only eMudScan has. The details page shows the other one
// (Alone), which a user can copy and run without eMudScan.
func pySSLFix(exe string, plan pySSLPlan) fixAction {
	admin := needsAdmin(plan.dir)
	cmds, fetch := pySSLScript(exe, plan, admin, false, true)
	plain, _ := pySSLScript(exe, plan, admin, false, false)
	alone, _ := pySSLScript(exe, plan, admin, true, true)
	return fixAction{Kind: fixPySSL, Admin: admin, Optional: true, Script: true, Needs: "pycheck", Files: plan.files(), Cmds: cmds, Plain: plain, Alone: alone, Fetch: []int{fetch},
		py: &pySSLTarget{exe: exe, plan: plan}}
}

// pySSLScript is the script of pySSLFix, and the place of its download among its
// lines; without backups, it deletes them at its end.
//
// The version that runs alone differs in what eMudScan does for the other one. It is
// one block, in which every error stops the script ("& {", $ErrorActionPreference),
// with a line after each program that tells PowerShell when it failed: so it stops at
// its first error by itself, also when it is pasted into a PowerShell window, and it
// leaves no setting and no variable behind. Its test is that the ssl module loads
// (with _hashlib, which the new libraries serve too), before and after. Its download
// goes to a folder of its own in the user's Temp folder, deleted at its end.
func pySSLScript(exe string, plan pySSLPlan, admin, alone, backups bool) (lines []string, fetch int) {
	list := func(names []string) string {
		q := make([]string, len(names))
		for i, n := range names {
			q[i] = psQuote(n)
		}
		return strings.Join(q, ", ")
	}
	each := func(names, do string) string { return names + " | ForEach-Object { " + do + " }" }
	added := len(plan.added) > 0
	all := "$files"
	if added {
		all = "$files + $added"
	}
	from := plan.src.from()
	if !plan.src.zip {
		from += " " + plan.src.tag
	}
	indent, dir := "", "$emudscanFiles"
	if alone {
		indent, dir = "    ", "$temp"
	}
	add := func(ls ...string) {
		for _, l := range ls {
			if l != "" {
				l = indent + l
			}
			lines = append(lines, l)
		}
	}
	first := true
	title := func(key string, args ...any) {
		if !first {
			add("")
		}
		first = false
		add("# " + Tf(key, args...))
	}
	// failed tells PowerShell that the program of the line before failed: eMudScan
	// sees it by itself
	failed := func(key string) {
		if alone {
			add("if ($LASTEXITCODE -ne 0) { throw " + psQuote(T(key)) + " }")
		}
	}
	test := func(when string) string {
		if alone {
			return `& $python -I -c 'import ssl, _hashlib; print(ssl.OPENSSL_VERSION)'`
		}
		return `& $python -I $check $emudscanFiles ` + when
	}

	if alone {
		lines = append(lines, "& {", indent+"$ErrorActionPreference = 'Stop'", "")
		title("pyssl.a.python")
	} else {
		// what $emudscanFiles is, which the commands use: said, then shown when the fix runs
		title("pyssl.s.folder", pyCheckName)
		add("$emudscanFiles")
		title("pyssl.s.python")
	}
	add("$python = "+psPath(exe, admin), "$folder = "+psPath(plan.dir, admin), "$files  = "+list(plan.replace))
	if added {
		add("$added  = " + list(plan.added))
	}
	if alone {
		add(`$temp   = "$env:TEMP\eMudScan-fix"`, `New-Item -ItemType Directory -Force -Path $temp | Out-Null`)
		title("pyssl.a.test")
	} else {
		add(`$check  = "$emudscanFiles\` + pyCheckName + `"`)
		title("pyssl.s.before")
	}
	add(test("before"))
	failed("pyssl.a.noSsl")

	title("pyssl.s.download", plan.src.version, from)
	ext := ".tar.gz"
	if plan.src.zip {
		ext = ".zip"
	}
	add("$url     = "+psQuote(plan.src.url), "$sha256  = "+psQuote(plan.src.sha256), `$archive = "`+dir+`\`+plan.src.name()+ext+`"`,
		`[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12`)
	fetch = len(lines)
	add(`if (-not (Test-Path -LiteralPath $archive)) { Invoke-WebRequest -UseBasicParsing -Uri $url -OutFile $archive }`)
	if alone { // its folder stays when it stops: without the file, which the next run would take for a new download
		add(`if ((Get-FileHash -LiteralPath $archive).Hash -ne $sha256) { Remove-Item -LiteralPath $archive; throw ` + psQuote(T("pyssl.badSum")) + ` }`)
	} else {
		add(`if ((Get-FileHash -LiteralPath $archive).Hash -ne $sha256) { throw ` + psQuote(T("pyssl.badSum")) + ` }`)
	}

	title("pyssl.s.unpack")
	members := strings.Join(prefixAll("python/DLLs/", append(append([]string(nil), plan.replace...), plan.added...)), " ")
	switch {
	case plan.src.zip:
		// python.org's package is a zip, which Windows' tar reads too: as for the other
		// download, its files are named one by one, and tar fails when one is missing.
		// Not Expand-Archive: a function of a module, which shows its progress bar
		// whatever the script sets, and leaves it on the screen
		add(`$new = "`+dir+`\`+plan.src.name()+`"`,
			`New-Item -ItemType Directory -Force -Path $new | Out-Null`,
			`& "$env:SystemRoot\System32\tar.exe" -xf $archive -C $new `+strings.Join(append(append([]string(nil), plan.replace...), plan.added...), " "))
		failed("pyssl.missing")
	case alone:
		add(`$new = "$temp\python\DLLs"`, `& "$env:SystemRoot\System32\tar.exe" -xzf $archive -C $temp `+members)
		failed("pyssl.missing")
	default:
		add(`$unpacked = "$emudscanFiles\`+plan.src.name()+`"`, `$new      = "$unpacked\python\DLLs"`,
			`New-Item -ItemType Directory -Force -Path $unpacked | Out-Null`,
			`& "$env:SystemRoot\System32\tar.exe" -xzf $archive -C $unpacked `+members)
	}

	title("pyssl.s.inUse")
	add(each("$files", `[IO.File]::Open("$folder\$_", 'Open', 'ReadWrite', 'None').Dispose()`))

	if added {
		title("pyssl.s.backupAdded")
	} else {
		title("pyssl.s.backup")
	}
	add(`$stamp = Get-Date -Format 'yyyyMMdd-HHmmss'`,
		each("$files", `Copy-Item -LiteralPath "$folder\$_" -Destination "$folder\$_.emudscan-$stamp.bak"`),
		each("$files", `(Get-Item -LiteralPath "$folder\$_.emudscan-$stamp.bak").CreationTime = (Get-Item -LiteralPath "$folder\$_").CreationTime`))
	if added {
		add(each("$added", `New-Item -ItemType File -Path "$folder\$_.emudscan-$stamp.added" | Out-Null`))
	}

	title("pyssl.s.replace")
	add("try {", "    "+each(all, `Copy-Item -LiteralPath "$new\$_" -Destination "$folder\$_" -Force`), "    "+test("after"))
	if alone {
		add("    if ($LASTEXITCODE -ne 0) { throw " + psQuote(T("pyssl.a.noNewSsl")) + " }")
	}
	add("}", "catch {")
	if alone { // why: the error of the copy or of the test, which eMudScan shows under its command
		add("    Write-Warning $_")
	}
	add("    " + each("$files", `Move-Item -LiteralPath "$folder\$_.emudscan-$stamp.bak" -Destination "$folder\$_" -Force`))
	if added {
		add("    " + each("$added", `Remove-Item -LiteralPath "$folder\$_", "$folder\$_.emudscan-$stamp.added" -ErrorAction Ignore`))
	}
	add("    throw "+psQuote(T("pyssl.putBack")), "}")

	if !backups {
		if added {
			title("pyssl.s.noBackupAdded")
		} else {
			title("pyssl.s.noBackup")
		}
		add(each("$files", `Remove-Item -LiteralPath "$folder\$_.emudscan-$stamp.bak"`))
		if added {
			add(each("$added", `Remove-Item -LiteralPath "$folder\$_.emudscan-$stamp.added"`))
		}
	}
	if alone {
		title("pyssl.a.clean")
		add(`Remove-Item -Recurse -Force -LiteralPath $temp`)
		lines = append(lines, "}")
	}
	return lines, fetch
}

// pySSLTarget is what pySSLFix was made for, kept with the fix: the same fix can be
// made again for another release of its source (the latest one: latest.go).
type pySSLTarget struct {
	exe  string
	plan pySSLPlan
}

// fixFrom is the fix of the target with the files of the release src.
func (t pySSLTarget) fixFrom(src pySSLSource) fixAction {
	plan := t.plan
	plan.src = src
	return pySSLFix(t.exe, plan)
}

func prefixAll(prefix string, names []string) []string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = prefix + n
	}
	return out
}

// pySSLAdvice offers pySSLFix for the Python exe of env, inside program, outdated
// (fixed: the version that fixes it), whose ssl module's library is sslLib: "" when
// there is no plan for it.
func pySSLAdvice(env pyEnv, sslLib, fixed, program string) string {
	plan, ok := pySSLPlanFor(env, sslLib, fixed)
	if !ok {
		return ""
	}
	f := newFix(pySSLFix(env.exe, plan))
	return how(Tf("pyssl.offer", plan.src.version, plan.src.from(), program)) + "\n" + psHeader(plan.dir) + "\n" + f.lines()
}

// pySSLRestore offers to restore what pySSLFix changed in the folder of sslLib: all
// the files of the run that backed up _ssl.pyd, together, when its backups and marks
// are all there (pySSLShapes), else nothing: some of them alone would break the ssl
// module (a new _ssl.pyd without the OpenSSL libraries it loads, or with old ones, does
// not load), and a file that a mark names as added stays. A backup the same as its
// file counts too: a copy interrupted by a crash replaced only some of the files.
func pySSLRestore(sslLib string) string {
	if sslLib == "" {
		return ""
	}
	dir := filepath.Dir(sslLib)
	first, ok := latestBackup(filepath.Join(dir, "_ssl.pyd")) // a backup that differs from it: a run changed it
	if !ok || first.added {
		return ""
	}
	stamp := ".emudscan-" + first.at.Format("20060102-150405")
	for _, shape := range pySSLShapes {
		var pairs []restorePair
		complete := true
		for _, n := range shape.replace {
			f, bak := filepath.Join(dir, n), filepath.Join(dir, n+stamp+".bak")
			if !exists(f) || !exists(bak) {
				complete = false
				break
			}
			pairs = append(pairs, restorePair{f, backup{bak, first.at, false}})
		}
		for _, n := range shape.added {
			f, mark := filepath.Join(dir, n), filepath.Join(dir, n+stamp+".added")
			if !exists(mark) {
				complete = false
				break
			}
			if exists(f) { // else the copy did not reach it: nothing to remove
				pairs = append(pairs, restorePair{f, backup{mark, first.at, true}})
			}
		}
		if !complete {
			continue
		}
		var refused []string // one of them refused (restoreUnsafe): none of them
		for _, p := range pairs {
			if why := restoreUnsafe(p.file, p.b.path); why != "" {
				refused = append(refused, why)
			}
		}
		if len(refused) > 0 {
			return joinParas(refused...)
		}
		return restoreAdviceFor(pairs)
	}
	return ""
}
