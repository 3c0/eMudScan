package main

import (
	"debug/pe"
	"encoding/binary"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// fakeExeFor writes a minimal Windows program header for the processor machine.
func fakeExeFor(t *testing.T, p string, machine uint16) {
	t.Helper()
	b := make([]byte, 0x100)
	copy(b, "MZ")
	binary.LittleEndian.PutUint32(b[0x3c:], 0x80)
	copy(b[0x80:], "PE\x00\x00")
	binary.LittleEndian.PutUint16(b[0x84:], machine)
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

// The fix of a node.exe inside a program is a script that reads as it runs: it names
// one release of nodejs.org, by its URL and SHA-256, and checks the download (its
// SHA-256, then its signature and the version it gives) before it changes anything;
// then the backup and the copy. It is optional (not ticked by default).
func TestNodeFix(t *testing.T) {
	Lang = 0
	exe := filepath.Join(t.TempDir(), "Example App 1", "resources", "node.exe")
	fakeExeFor(t, exe, pe.IMAGE_FILE_MACHINE_AMD64)
	f, ok := nodeReplaceFix(exe, 22, "22.23.4")
	src := nodeExe[22]["win-x64"]
	if !ok || src.version != "22.23.5" {
		t.Fatalf("no fix: %v %+v", ok, src)
	}
	download := func(dir, badSum string) []string {
		return []string{
			"# Download Node.js 22.23.5 (nodejs.org) and check its SHA-256",
			"$url    = 'https://nodejs.org/dist/v22.23.5/win-x64/node.exe'",
			"$sha256 = '" + src.sha256 + "'",
			`$new    = "` + dir + `\node-22.23.5-win-x64.exe"`,
			"[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12",
			"if (-not (Test-Path -LiteralPath $new)) { Invoke-WebRequest -UseBasicParsing -Uri $url -OutFile $new }",
			"if ((Get-FileHash -LiteralPath $new).Hash -ne $sha256) { " + badSum + "throw " + psQuote(T("fix.badSum")) + " }",
			"",
			"# Check its signature, by the OpenJS Foundation, and the version it gives",
			"$signature = Get-AuthenticodeSignature -LiteralPath $new",
			"if ($signature.Status -ne 'Valid' -or $signature.SignerCertificate.Subject -notmatch '(^|, )O=OpenJS Foundation(,|$)') { throw " + psQuote(T("node.badSignature")) + " }",
			"if ((& $new --version) -ne 'v22.23.5') { throw " + psQuote(T("node.badVersion")) + " }",
			"",
			"# Check that no program is using the file to replace",
			"[IO.File]::Open($node, 'Open', 'ReadWrite', 'None').Dispose()",
			"",
		}
	}
	backup := []string{
		"# Back up the file, with its dates",
		"$stamp = Get-Date -Format 'yyyyMMdd-HHmmss'",
		`Copy-Item -LiteralPath $node -Destination "$node.emudscan-$stamp.bak"`,
		`(Get-Item -LiteralPath "$node.emudscan-$stamp.bak").CreationTime = (Get-Item -LiteralPath $node).CreationTime`,
		"",
	}
	replace := []string{
		"# Replace the file",
		"Copy-Item -LiteralPath $new -Destination $node -Force",
	}
	head := []string{
		// what $emudscanFiles is, then its value, shown when the fix runs
		"# The folder that eMudScan made for this fix. eMudScan deletes it after the fix",
		"$emudscanFiles",
		"",
		"# The node.exe to replace",
		"$node = " + psPath(exe, false),
		"",
	}
	join := func(parts ...[]string) string {
		var all []string
		for _, p := range parts {
			all = append(all, p...)
		}
		return strings.Join(all, "\n")
	}
	if got, want := strings.Join(f.Cmds, "\n"), join(head, download("$emudscanFiles", ""), backup, replace); got != want {
		t.Errorf("the script:\n%s\nwant:\n%s", got, want)
	}
	// without "Back up the files to change": no backup
	if got, want := strings.Join(f.Plain, "\n"), join(head, download("$emudscanFiles", ""), replace); got != want {
		t.Errorf("plain:\n%s\nwant:\n%s", got, want)
	}
	// the version that runs alone: one block, which downloads into a folder of its own,
	// deletes a download that is not the expected one, and the folder at its end
	var body []string
	for _, l := range strings.Split(join([]string{
		"# The node.exe to replace, and a folder for the download",
		"$node = " + psPath(exe, false),
		`$temp = "$env:TEMP\eMudScan-fix"`,
		"New-Item -ItemType Directory -Force -Path $temp | Out-Null",
		"",
	}, download("$temp", "Remove-Item -LiteralPath $new; "), backup, replace, []string{"", "# Delete the download", "Remove-Item -Recurse -Force -LiteralPath $temp"}), "\n") {
		if l != "" {
			l = "    " + l
		}
		body = append(body, l)
	}
	if got, want := strings.Join(f.Alone, "\n"), join([]string{"& {", "    $ErrorActionPreference = 'Stop'", ""}, body, []string{"}"}); got != want {
		t.Errorf("alone:\n%s\nwant:\n%s", got, want)
	}
	if f.Kind != fixNode || !f.Optional || !f.Script || f.Admin || len(f.Files) != 1 || f.Files[0] != exe || !fetchesRight(f) ||
		len(f.Fetch) != 1 || f.node == nil || f.node.src != src || f.node.file != exe || !backsUp([]Item{{Fixes: []fixAction{f}}}) {
		t.Errorf("%+v", f)
	}
	// each line is a whole command, a comment or an empty line
	for _, c := range f.Plain {
		if strings.Count(c, "{") != strings.Count(c, "}") {
			t.Errorf("not a line of the script: %q", c)
		}
	}
	// the same fix, for another release of the line (the latest one, found by latest.go)
	newer := nodeSource{version: "22.24.0", arch: "win-x64", sha256: testSum}
	nf := f.node.fixFrom(newer)
	if all := strings.Join(nf.Cmds, "\n"); !strings.Contains(all, "\n$url    = 'https://nodejs.org/dist/v22.24.0/win-x64/node.exe'\n$sha256 = '"+testSum+"'\n") ||
		!strings.Contains(all, `\node-22.24.0-win-x64.exe"`) || !strings.Contains(all, "-ne 'v22.24.0') { throw") || strings.Contains(all, "22.23.5") || nf.node.src != newer {
		t.Errorf("newer:\n%s", all)
	}
}

// The node.exe of each processor gets nodejs.org's node.exe for that processor, of a
// release that has the fix; none for a processor that nodejs.org builds none for, such
// as the 32-bit one for Node.js 24 and later, or for a release line without a fix.
func TestNodeReplaceFix(t *testing.T) {
	Lang = 0
	dir := t.TempDir()
	exes := map[string]string{}
	for arch, machine := range map[string]uint16{"win-x64": pe.IMAGE_FILE_MACHINE_AMD64, "win-arm64": pe.IMAGE_FILE_MACHINE_ARM64,
		"win-x86": pe.IMAGE_FILE_MACHINE_I386, "": 0x1c4 /* ARM (32-bit) */} {
		folder := "Example App " + arch
		if arch == "" {
			folder = "Example App arm" // a folder name cannot end with a space on Windows
		}
		exes[arch] = filepath.Join(dir, folder, "node.exe")
		fakeExeFor(t, exes[arch], machine)
		if got := nodeArch(exes[arch]); got != arch {
			t.Errorf("%s: %q", arch, got)
		}
	}
	for _, c := range []struct {
		arch  string
		major int
		fixed string
		want  string // the version of the fix, "" when there is none
	}{
		{"win-x64", 22, "22.23.4", "22.23.5"},
		{"win-arm64", 22, "22.23.4", "22.23.5"},
		{"win-x86", 22, "22.23.4", "22.23.5"},
		{"win-x64", 24, "24.22.1", "24.22.3"},
		{"win-arm64", 26, "26.12.1", "26.13.1"},
		{"win-x86", 24, "24.22.1", ""},
		{"win-x86", 26, "26.12.1", ""},
		{"", 22, "22.23.4", ""},
		{"win-x64", 20, "20.20.1", ""}, // a line without a fix
		{"win-x64", 22, "22.24.0", ""}, // a release written here older than the fixed one
		{"win-x64", 22, "", ""},        // no fixed release
	} {
		f, ok := nodeReplaceFix(exes[c.arch], c.major, c.fixed)
		if ok != (c.want != "") || ok && (f.node.src.version != c.want || f.node.src.arch != c.arch || !strings.Contains(strings.Join(f.Cmds, "\n"), "/"+c.arch+"/node.exe'")) {
			t.Errorf("%s, Node.js %d: %v %+v", c.arch, c.major, ok, f.node)
		}
	}
	// the advice: what it does and that the program must be closed, then the commands
	a := nodeReplaceAdvice(exes["win-x64"], 22, "22.23.4", "Example App 1")
	if !strings.HasPrefix(a, how(Tf("node.replace", 22, "Example App 1"))+"\n"+psHeader(exes["win-x64"])+"\n$ ") || len(fixesIn(a)) != 1 ||
		!fixesIn(a)[0].Optional || fixesIn(a)[0].node == nil {
		t.Errorf("advice:\n%s", a)
	}
	// a 32-bit node.exe of a line that nodejs.org builds none for: said
	if a := nodeReplaceAdvice(exes["win-x86"], 24, "24.22.1", "Example App 1"); a != how(Tf("node.no32", 24)) || fixesIn(a) != nil {
		t.Errorf("32-bit:\n%s", a)
	}
	if a := nodeReplaceAdvice(exes[""], 22, "22.23.4", "Example App 1"); a != "" {
		t.Errorf("no build:\n%s", a)
	}
}

// The node.exe written in eMudScan (nodeExe) are those of the SHASUMS256.txt of their
// release, signed by a releaser of Node.js, which are in testdata/node: the SHA-256 of
// each processor, all those of the release, and nothing else. docs/checks.md says how
// to check these signatures.
func TestNodeExe(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join("testdata", "node", "*"))
	used := map[string]bool{}
	line := regexp.MustCompile(`^([0-9a-f]{64})  (win-[a-z0-9]+)/node\.exe$`)
	for major, builds := range nodeExe {
		version := builds["win-x64"].version
		if !strings.HasPrefix(version, strconv.Itoa(major)+".") {
			t.Errorf("Node.js %d: %q", major, version)
		}
		name := filepath.Join("testdata", "node", "v"+version+"-SHASUMS256.txt.asc")
		used[name] = true
		data, err := os.ReadFile(name)
		if err != nil {
			t.Errorf("Node.js %d: %v", major, err)
			continue
		}
		text := string(data)
		start, end := strings.Index(text, "-----BEGIN PGP SIGNED MESSAGE-----\nHash: SHA256\n\n"), strings.Index(text, "\n-----BEGIN PGP SIGNATURE-----\n")
		if start != 0 || end < 0 || strings.Contains(text, "\r") {
			t.Errorf("%s: not a signed text with LF line ends", name)
			continue
		}
		signed := map[string]string{}
		for _, l := range strings.Split(text[:end], "\n") {
			if m := line.FindStringSubmatch(l); m != nil {
				signed[m[2]] = m[1]
			}
		}
		if len(signed) == 0 || len(signed) != len(builds) {
			t.Errorf("Node.js %d: signed %v, written %v", major, signed, builds)
		}
		for arch, src := range builds {
			if src.version != version || src.arch != arch || src.sha256 != signed[arch] || src.url() != nodeDist+"v"+version+"/"+arch+"/node.exe" {
				t.Errorf("Node.js %d, %s: %+v, signed %q", major, arch, src, signed[arch])
			}
		}
	}
	for _, f := range files {
		if !used[f] {
			t.Errorf("%s is not the release of a line", f)
		}
	}
	// a node.exe for each release line that has a fixed release
	for _, v := range fixRules["node"].fixed {
		major, _, _ := strings.Cut(v, ".")
		if n, _ := strconv.Atoi(major); nodeExe[n] == nil {
			t.Errorf("no node.exe for the line of %s", v)
		}
	}
	if len(fixRules["node"].fixed) != len(nodeExe) {
		t.Errorf("fixed %v, node.exe %v", fixRules["node"].fixed, nodeExe)
	}
	// nodejs.org builds a 32-bit node.exe for Node.js 22 only
	if nodeExe[22]["win-x86"].sha256 == "" || nodeExe[24]["win-x86"].sha256 != "" || nodeExe[26]["win-x86"].sha256 != "" {
		t.Error("32-bit builds")
	}
}
