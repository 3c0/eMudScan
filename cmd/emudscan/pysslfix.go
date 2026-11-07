package main

// The ssl files of a Python inside a program, which its publisher has not updated yet:
// the fix of CVE-2026-115557 is entirely in the ssl module's library (_ssl.pyd), with
// the OpenSSL libraries it needs (OpenSSL 3.3 or later for Python 3.11.18). eMudScan can
// replace these files with those of the fixed release of the same minor version, for a
// 64-bit Python (x64):
//
//   - a Python laid out as python-build-standalone's (libssl-3-x64.dll), and python.org's
//     3.11 to 3.13, for which python.org publishes no fixed Windows build: from
//     python-build-standalone 20261026 (on GitHub). For python.org's layout, _hashlib.pyd
//     is replaced too and the two OpenSSL libraries are added beside the old ones, as
//     python-build-standalone names them;
//   - python.org's 3.14 and 3.15 (libssl-3.dll): from python.org's embeddable package of
//     3.14.10 or 3.15.2.
//
// Each download is checked against a SHA-256 written here: the releases are pinned, so
// that what runs is what was tested, and its hash does not come from the server that
// serves it. To move to a newer release of a line, change its URL and SHA-256 (from
// the release's SHA256SUMS, or python.org's page), then test its files as these were.
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
	zip                  bool // python.org's embeddable package, its files at its root; else python-build-standalone's archive, in python\DLLs
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
	return "python-" + s.version + "-standalone"
}

// pbsFiles are the files taken out of python-build-standalone's archive (from
// python\DLLs), the same for every plan, since the plans share them.
var pbsFiles = []string{"_ssl.pyd", "_hashlib.pyd", "libssl-3-x64.dll", "libcrypto-3-x64.dll"}

const pbsRelease = "https://github.com/astral-sh/python-build-standalone/releases/download/20261026/"

// pbsSSL: python-build-standalone 20261026, x64, by minor version.
var pbsSSL = map[string]pySSLSource{
	"3.11": {"3.11.18", pbsRelease + "cpython-3.11.18%2B20261026-x86_64-pc-windows-msvc-install_only_stripped.tar.gz", "67215a7bf1643e33d01b17ee3f1e91a8b0b39163746855ee02594bf0608f68b4", false},
	"3.12": {"3.12.16", pbsRelease + "cpython-3.12.16%2B20261026-x86_64-pc-windows-msvc-install_only_stripped.tar.gz", "1f2ab53880fc67ea323398c6388e05af28addc99a8e389209f08a51fdff4b149", false},
	"3.13": {"3.13.18", pbsRelease + "cpython-3.13.18%2B20261026-x86_64-pc-windows-msvc-install_only_stripped.tar.gz", "7369f7a35f261222a6ac8b3a87c1c88eeebee8af945d85346202591a64038dd3", false},
	"3.14": {"3.14.10", pbsRelease + "cpython-3.14.10%2B20261026-x86_64-pc-windows-msvc-install_only_stripped.tar.gz", "c1876a9795d3a94ee49a1499a69cd91e26ca3e2829593f93fa6e4db90d05402b", false},
	"3.15": {"3.15.2", pbsRelease + "cpython-3.15.2%2B20261026-x86_64-pc-windows-msvc-install_only_stripped.tar.gz", "02e532b60e720aa3d77e757c349ae9cc945441c05851fa646336fc9ff0403041", false},
}

// pyorgSSL: python.org's embeddable packages, x64, by minor version.
var pyorgSSL = map[string]pySSLSource{
	"3.14": {"3.14.10", "https://www.python.org/ftp/python/3.14.10/python-3.14.10-embed-amd64.zip", "90905e64a840557d8c1bc4f5b74db3d7ee85e3ffb217611cac1c7813acea4159", true},
	"3.15": {"3.15.2", "https://www.python.org/ftp/python/3.15.2/python-3.15.2-embed-amd64.zip", "d012f3aa29cf07436dae8aea1fec0fd195c2612a0b059b1f915ae0afb9ad1f62", true},
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
// a minor version without a fixed build.
func pySSLPlanFor(env pyEnv, sslLib string, fixed string) (pySSLPlan, bool) {
	v := ParseVersion(fixed)
	if sslLib == "" || len(v) < 2 || peMachine(env.exe) != 0x8664 /* x64 */ {
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

// pySSLFix replaces the ssl files of the Python exe as plan says. Its commands, in
// order: the checks before (the ssl module must load); the download, checked against
// its SHA-256; its files taken out (both once for the Pythons fixed in the same run
// from the same download); the files to change must not be in use (the program
// closed); a backup of each file replaced, and a mark for each file added, named after
// the same time, with the creation date of its file (see backupThen); the copy; the
// checks after; when they fail, the original files put
// back and the added ones removed. The backups are made in any case, for the files to
// be put back; without "Back up the files to change" (Plain), they and the marks are
// deleted once the checks after pass.
func pySSLFix(exe string, plan pySSLPlan) fixAction {
	admin := needsAdmin(plan.dir)
	list := func(names []string) string {
		if len(names) == 0 {
			return "@()"
		}
		q := make([]string, len(names))
		for i, n := range names {
			q[i] = psQuote(n)
		}
		return strings.Join(q, ", ")
	}
	archive, extract, inside := plan.src.name()+".tar.gz", `New-Item -ItemType Directory -Force -Path $pyNew | Out-Null; & "$env:SystemRoot\System32\tar.exe" -xzf $pyArchive -C $pyNew `+
		strings.Join(prefixAll("python/DLLs/", pbsFiles), " "), ` $pyNew = Join-Path $pyNew 'python\DLLs';`
	if plan.src.zip {
		archive, extract, inside = plan.src.name()+".zip", `Add-Type -AssemblyName System.IO.Compression.FileSystem; [IO.Compression.ZipFile]::ExtractToDirectory($pyArchive, $pyNew)`, ""
	}
	check := `& $pyExe -I (Join-Path $emudscanFiles 'check_ssl.py') $emudscanFiles`
	cmds := []string{
		fmt.Sprintf(`$pyExe = %s; $pyDir = %s; $pyFiles = %s; $pyAdded = %s`, psPath(exe, admin), psPath(plan.dir, admin), list(plan.replace), list(plan.added)),
		check + ` before; $pyOK = $LASTEXITCODE -eq 0`,
		`[Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12`,
		fmt.Sprintf(`$pyArchive = Join-Path $emudscanFiles %s; if ($pyOK -and -not (Test-Path -LiteralPath $pyArchive)) { Invoke-WebRequest -UseBasicParsing -Uri '%s' -OutFile $pyArchive }; $pyOK = $pyOK -and (Test-Path -LiteralPath $pyArchive) -and (Get-FileHash -LiteralPath $pyArchive).Hash -eq '%s'`, psQuote(archive), plan.src.url, plan.src.sha256),
		`$pyNew = Join-Path $emudscanFiles ` + psQuote(plan.src.name()) + `; if ($pyOK -and -not (Test-Path -LiteralPath $pyNew)) { ` + extract + ` };` + inside + ` $pyOK = $pyOK -and -not ($pyFiles + $pyAdded | Where-Object { -not (Test-Path -LiteralPath (Join-Path $pyNew $_)) })`,
		// in use (the program is running): nothing changes
		`if ($pyOK) { foreach ($n in $pyFiles) { try { [IO.File]::Open((Join-Path $pyDir $n), 'Open', 'ReadWrite', 'None').Dispose() } catch { $pyOK = $false; Write-Warning ` + psQuote(T("pyssl.inUse")) + ` } } }`,
		`$pyStamp = Get-Date -Format 'yyyyMMdd-HHmmss'; if ($pyOK) { foreach ($n in $pyFiles) { if ($pyOK) { $pyBak = Copy-Item -LiteralPath (Join-Path $pyDir $n) -Destination (Join-Path $pyDir "$n.emudscan-$pyStamp.bak") -PassThru; if ($pyBak) { $pyBak.CreationTimeUtc = (Get-Item -LiteralPath (Join-Path $pyDir $n)).CreationTimeUtc }; $pyOK = [bool]$pyBak } }; foreach ($n in $pyAdded) { $pyOK = $pyOK -and [bool](New-Item -ItemType File -Force -Path (Join-Path $pyDir "$n.emudscan-$pyStamp.added")) } }`,
		`if ($pyOK) { foreach ($n in $pyFiles + $pyAdded) { Copy-Item -LiteralPath (Join-Path $pyNew $n) -Destination (Join-Path $pyDir $n) -Force } }`,
		`$pyGood = $false; if ($pyOK) { ` + check + ` after; $pyGood = $LASTEXITCODE -eq 0 }`,
		`if ($pyOK -and -not $pyGood) { foreach ($n in $pyFiles) { Move-Item -LiteralPath (Join-Path $pyDir "$n.emudscan-$pyStamp.bak") -Destination (Join-Path $pyDir $n) -Force }; foreach ($n in $pyAdded) { Remove-Item -LiteralPath (Join-Path $pyDir $n), (Join-Path $pyDir "$n.emudscan-$pyStamp.added") -Force }; Write-Warning ` + psQuote(T("pyssl.putBack")) + ` }`,
		`if (-not $pyOK) { Write-Warning ` + psQuote(T("pyssl.unchanged")) + ` }`,
	}
	plain := append(append([]string(nil), cmds...),
		`if ($pyGood) { foreach ($n in $pyFiles) { Remove-Item -LiteralPath (Join-Path $pyDir "$n.emudscan-$pyStamp.bak") -Force }; foreach ($n in $pyAdded) { Remove-Item -LiteralPath (Join-Path $pyDir "$n.emudscan-$pyStamp.added") -Force } }`)
	return fixAction{Kind: fixPySSL, Admin: admin, Optional: true, Needs: "pycheck", Files: plan.files(), Cmds: cmds, Plain: plain}
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
	return Tf("pyssl.offer", plan.src.version, plan.src.from(), program) + "\n" + psHeader(plan.dir) + "\n" + f.lines()
}

// pySSLRestore offers to restore what pySSLFix changed in the folder of sslLib: all
// the files of the run that backed up _ssl.pyd, together, when its backups and marks
// are all there (pySSLShapes), else nothing: some of them alone would break the ssl
// module (a new _ssl.pyd without the OpenSSL libraries it loads, or with old ones, does
// not load), and a file that a mark names as added stays.
func pySSLRestore(sslLib string) string {
	if sslLib == "" {
		return ""
	}
	dir := filepath.Dir(sslLib)
	first, ok := latestBackup(filepath.Join(dir, "_ssl.pyd"))
	if !ok || first.added {
		return ""
	}
	of := func(n string, added bool) bool { // backed up, or marked, by that run
		b, ok := latestBackup(filepath.Join(dir, n))
		return ok && b.added == added && b.at.Equal(first.at)
	}
	for _, shape := range pySSLShapes {
		var files []string
		for _, n := range shape.replace {
			if of(n, false) {
				files = append(files, filepath.Join(dir, n))
			}
		}
		for _, n := range shape.added {
			if of(n, true) {
				files = append(files, filepath.Join(dir, n))
			}
		}
		if len(files) < len(shape.replace)+len(shape.added) {
			continue
		}
		var refused []string // one of them refused (restoreUnsafe): none of them
		for _, f := range files {
			b, _ := latestBackup(f)
			if why := restoreUnsafe(f, b.path); why != "" {
				refused = append(refused, why)
			}
		}
		if len(refused) > 0 {
			return joinParas(refused...)
		}
		return restoreAdvice(files...)
	}
	return ""
}
