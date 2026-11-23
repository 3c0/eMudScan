package main

// A node.exe inside a program, which its publisher has not updated yet: replaced with
// nodejs.org's node.exe of the latest release of its release line (the same major
// version, so that the program's native modules keep working), for its processor.
//
// The commands download one release, by its URL, and check it: its SHA-256, both
// written in them, the signature of the OpenJS Foundation, and the version it gives.
// During the scan, which connects to nothing, they are those of the release written
// here (nodeExe). When a Fix window opens, eMudScan itself looks for the latest release
// of the line (latest.go) and writes them again for it: to find the release is the
// program's work, not the commands'. It is an optional fix, not ticked by default: it
// changes a program that its publisher has not updated.

import (
	"debug/pe"
	"fmt"
)

// nodeSource is nodejs.org's node.exe of a release, for a processor (nodeArch), with
// its SHA-256.
type nodeSource struct {
	version, arch, sha256 string
}

// nodeDist is where nodejs.org has its releases, each in the folder of its version.
const nodeDist = "https://nodejs.org/dist/"

func (s nodeSource) url() string { return nodeDist + "v" + s.version + "/" + s.arch + "/node.exe" }

// nodeExe: the node.exe written in eMudScan, by release line, then by processor: those
// of the latest release of each line that has a fix, when it was written, with their
// SHA-256 from that release's SHASUMS256.txt, which a releaser of Node.js signed. These
// signed files are in testdata/node, where TestNodeExe checks that the values written
// here are theirs. nodejs.org builds no 32-bit node.exe for Node.js 24 and later.
var nodeExe = map[int]map[string]nodeSource{
	22: {
		"win-x64":   {"22.23.5", "win-x64", "247e9693d316da9857e44990e5e11709f596f18112251b8026ec6eed80bdf38a"},
		"win-arm64": {"22.23.5", "win-arm64", "c2eaa48231229b966bfef21554df203221862a9fc94ec2a6908aad5eaa6c0ba0"},
		"win-x86":   {"22.23.5", "win-x86", "a6f12cfaa84f16d1cfb36ccca361e155acee9e8a4e47fdc3a684cda361ad245a"},
	},
	24: {
		"win-x64":   {"24.22.3", "win-x64", "86fc4785327db5f5c7558dc6156c543b433c313e813a222e088086222ab4afa6"},
		"win-arm64": {"24.22.3", "win-arm64", "172e320db179ce416fc012b96b9917dacfb442beb081de57c4deaed7b5a24644"},
	},
	26: {
		"win-x64":   {"26.13.1", "win-x64", "f5109dc868ec38703ad8f305e0aceece717cb60d4889873f434017af4e96cccb"},
		"win-arm64": {"26.13.1", "win-arm64", "00edc356a01d7fe586beebe7519bb1acdff9dd62064ce4cf12ff1ed4a8d1810e"},
	},
}

// nodeArch is the folder of nodejs.org's builds for the processor of exe.
func nodeArch(exe string) string {
	switch peMachine(exe) {
	case pe.IMAGE_FILE_MACHINE_AMD64:
		return "win-x64"
	case pe.IMAGE_FILE_MACHINE_ARM64:
		return "win-arm64"
	case pe.IMAGE_FILE_MACHINE_I386:
		return "win-x86"
	}
	return ""
}

// nodeReplaceFix replaces file, a node.exe of the release line major, which fixed and
// later releases fix, with nodejs.org's node.exe of that line written here (nodeExe);
// false when there is none for its processor.
func nodeReplaceFix(file string, major int, fixed string) (fixAction, bool) {
	src, ok := nodeExe[major][nodeArch(file)]
	if !ok || len(ParseVersion(fixed)) < 3 || ParseVersion(src.version).Cmp(ParseVersion(fixed)) < 0 {
		return fixAction{}, false
	}
	return nodeFix(file, src), true
}

// nodeFix replaces file with the node.exe src. It is a script (fixAction.Script), which
// stops at its first command that fails. In order: the download, checked against its
// SHA-256, then its signature and the version it gives; the file to replace must not
// be in use (the program closed); its backup, with its dates (backupLines); then the
// copy. Without "Back up the files to change" (Plain), it makes no backup.
//
// It has two versions (nodeScript). The Fix function runs the one that downloads into
// the folder that eMudScan makes for the run, and deletes after it. The details page
// shows the other one (Alone), which a user can copy and run without eMudScan: one
// block, which stops at its first error by itself, also when it is pasted into a
// PowerShell window, and downloads into a folder of its own in the Temp folder, deleted
// at its end.
func nodeFix(file string, src nodeSource) fixAction {
	admin := needsAdmin(file)
	cmds := nodeScript(file, src, admin, false, true)
	return fixAction{Kind: fixNode, Admin: admin, Optional: true, Script: true, Files: []string{file}, Cmds: cmds,
		Plain: nodeScript(file, src, admin, false, false), Alone: nodeScript(file, src, admin, true, true), Fetch: fetchLines(cmds),
		node: &nodeTarget{file: file, src: src}}
}

// nodeScript is the script of nodeFix; without backups, it makes none.
func nodeScript(file string, src nodeSource, admin, alone, backups bool) []string {
	var s scriptLines
	dir := "$emudscanFiles"
	if alone {
		dir = "$temp"
		s.block(T("node.a.node"), "$node = "+psPath(file, admin), `$temp = "$env:TEMP\eMudScan-fix"`,
			`New-Item -ItemType Directory -Force -Path $temp | Out-Null`)
	} else {
		// what $emudscanFiles is, which the commands use: said, then shown when the fix runs
		s.block(T("node.s.folder"), "$emudscanFiles")
		s.block(T("node.s.node"), "$node = "+psPath(file, admin))
	}
	check := fmt.Sprintf(`if ((Get-FileHash -LiteralPath $new).Hash -ne $sha256) { throw %s }`, psQuote(T("fix.badSum")))
	if alone { // its folder stays when it stops: without the file, which the next run would take for a new download
		check = fmt.Sprintf(`if ((Get-FileHash -LiteralPath $new).Hash -ne $sha256) { Remove-Item -LiteralPath $new; throw %s }`, psQuote(T("fix.badSum")))
	}
	s.block(Tf("node.s.download", src.version),
		"$url    = "+psQuote(src.url()),
		"$sha256 = "+psQuote(src.sha256),
		`$new    = "`+dir+`\node-`+src.version+`-`+src.arch+`.exe"`,
		`[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12`,
		`if (-not (Test-Path -LiteralPath $new)) { Invoke-WebRequest -UseBasicParsing -Uri $url -OutFile $new }`,
		check)
	s.block(T("node.s.check"),
		`$signature = Get-AuthenticodeSignature -LiteralPath $new`,
		fmt.Sprintf(`if ($signature.Status -ne 'Valid' -or $signature.SignerCertificate.Subject -notmatch '(^|, )O=OpenJS Foundation(,|$)') { throw %s }`, psQuote(T("node.badSignature"))),
		fmt.Sprintf(`if ((& $new --version) -ne 'v%s') { throw %s }`, src.version, psQuote(T("node.badVersion"))))
	s.block(T("node.s.inUse"), `[IO.File]::Open($node, 'Open', 'ReadWrite', 'None').Dispose()`)
	if backups {
		s.block(T("bundle.s.backup1"), backupLines("$node")...)
	}
	s.block(T("bundle.s.replace1"), `Copy-Item -LiteralPath $new -Destination $node -Force`)
	if alone {
		s.block(T("pyssl.a.clean"), `Remove-Item -Recurse -Force -LiteralPath $temp`)
		return runsAlone(s)
	}
	return s
}

// nodeTarget is what nodeFix was made for, kept with the fix: the same fix can be made
// again for another release of its line (the latest one: latest.go).
type nodeTarget struct {
	file string
	src  nodeSource
}

// fixFrom is the fix of the target with the node.exe src.
func (t nodeTarget) fixFrom(src nodeSource) fixAction { return nodeFix(t.file, src) }

// nodeReplaceAdvice offers nodeReplaceFix for file, inside program. For a 32-bit
// node.exe of a line that nodejs.org builds none for, it says that it cannot be
// replaced; "" for a processor that it builds none for at all.
func nodeReplaceAdvice(file string, major int, fixed, program string) string {
	f, ok := nodeReplaceFix(file, major, fixed)
	if !ok {
		if _, line := nodeExe[major]; line && nodeArch(file) == "win-x86" {
			return how(Tf("node.no32", major))
		}
		return ""
	}
	return how(Tf("node.replace", major, program)) + "\n" + psHeader(file) + "\n" + newFix(f).lines()
}
