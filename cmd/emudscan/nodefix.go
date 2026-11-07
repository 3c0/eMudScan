package main

// A node.exe inside a program, which its publisher has not updated yet: replaced with
// the latest release of its release line (the same major version, so that the
// program's native modules keep working), from nodejs.org, once checked: its SHA-256
// as Node.js publishes it (SHASUMS256.txt), its signature by the OpenJS Foundation, and
// its version. It is an optional fix, not ticked by default: it changes a program that
// its publisher has not updated.

import (
	"debug/pe"
	"fmt"
)

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
// later releases fix, with the latest release of that line from nodejs.org, once
// checked. Each command works on its own, as replaceBundleFix's: a failed download or
// check changes nothing.
func nodeReplaceFix(file string, major int, fixed string) (fixAction, bool) {
	arch := nodeArch(file)
	if arch == "" || len(ParseVersion(fixed)) < 3 {
		return fixAction{}, false
	}
	admin := needsAdmin(file)
	check := func() []string {
		return []string{
			`[Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12`,
			// the latest release of the line, by its version number
			fmt.Sprintf(`$nodeVersion = $null; $nodeVersion = ((Invoke-RestMethod -UseBasicParsing -Uri https://nodejs.org/dist/index.json) | Where-Object { $_.version -like 'v%d.*' } | Sort-Object { [version]$_.version.TrimStart('v') } -Descending | Select-Object -First 1).version`, major),
			// its SHA-256, as Node.js publishes it
			fmt.Sprintf(`$nodeSum = $null; if ($nodeVersion) { $nodeSum = (([Text.Encoding]::ASCII.GetString((Invoke-WebRequest -UseBasicParsing -Uri "https://nodejs.org/dist/$nodeVersion/SHASUMS256.txt").RawContentStream.ToArray()) -split '\r?\n') -match '^[0-9a-f]{64}  %s/node\.exe$' | Select-Object -First 1); if ($nodeSum) { $nodeSum = $nodeSum.Substring(0, 64) } }`, arch),
			fmt.Sprintf(`$nodeNew = Join-Path $env:TEMP 'emudscan-node.exe'; Remove-Item -LiteralPath $nodeNew -Force -ErrorAction Ignore; if ($nodeSum) { Invoke-WebRequest -UseBasicParsing -Uri "https://nodejs.org/dist/$nodeVersion/%s/node.exe" -OutFile $nodeNew }`, arch),
			// that SHA-256, the OpenJS Foundation's signature, a release that is fixed, and the
			// version it says it is
			fmt.Sprintf(`$nodeSig = $null; $nodeOK = [bool]$nodeSum -and (Test-Path -LiteralPath $nodeNew) -and (Get-FileHash -LiteralPath $nodeNew).Hash -eq $nodeSum -and ($nodeSig = Get-AuthenticodeSignature -LiteralPath $nodeNew).Status -eq 'Valid' -and $nodeSig.SignerCertificate.Subject -match '(^|, )O=OpenJS Foundation(,|$)' -and [version]$nodeVersion.TrimStart('v') -ge [version]'%s' -and (& $nodeNew --version) -eq $nodeVersion`, fixed),
			fmt.Sprintf(`if (-not $nodeOK) { Write-Warning %s }`, psQuote(T("node.checkFailed"))),
		}
	}
	replace := fmt.Sprintf(`Copy-Item -LiteralPath $nodeNew -Destination %s -Force`, psPath(file, admin))
	// Ignore, not SilentlyContinue: the temporary file is usually not there, and an error
	// that SilentlyContinue hides is still recorded, which marks the command as failed
	cleanup := `Remove-Item -LiteralPath $nodeNew -Force -ErrorAction Ignore`
	f := fixAction{Kind: fixNode, Admin: admin, Optional: true, Files: []string{file},
		Cmds:  append(check(), backupThenLine(file, "$nodeOK", replace, admin), cleanup),
		Plain: append(check(), changeLine("$nodeOK", replace), cleanup)}
	return f, true
}

// nodeReplaceAdvice offers nodeReplaceFix for file, inside program: "" when there is
// no build of nodejs.org for its processor.
func nodeReplaceAdvice(file string, major int, fixed, program string) string {
	f, ok := nodeReplaceFix(file, major, fixed)
	if !ok {
		return ""
	}
	return Tf("node.replace", major, program) + "\n" + psHeader(file) + "\n" + newFix(f).lines()
}
