# Builds dist\eMudScan.exe on Windows (PowerShell): .\scripts\build.ps1
# Checks and tests the code, then compiles it. The Windows resources
# (cmd\emudscan\rsrc_windows_amd64.syso) can only be regenerated on Linux or macOS, by
# scripts/build.sh or the GitHub build; this script uses the file as it is.
# .\scripts\build.ps1 -Simulate builds dist\eMudScan-simulation.exe instead, whose
# Fix function runs nothing (see cmd\emudscan\fixsim_on.go).
param([switch]$Simulate)
$ErrorActionPreference = 'Stop'
Set-Location (Join-Path $PSScriptRoot '..')
$env:CGO_ENABLED = '0'
# A build that is not of a release (the commit tagged with appVersion, unchanged)
# shows its commit instead of the date: "3.0.0+dev (ae7832d)", "3.0.0+dev (ae7832d-modified)".
$rev = ''
if (Get-Command git -ErrorAction SilentlyContinue) {
    $ErrorActionPreference = 'Continue' # git writes to stderr when there is no tag
    if ((git rev-parse --is-inside-work-tree 2>$null) -eq 'true') {
        $version = (Select-String -Path cmd\emudscan\model.go -Pattern '^const appVersion *= *"([^"]*)"').Matches[0].Groups[1].Value
        $tag = git describe --tags --exact-match 2>$null
        $modified = [bool](git status --porcelain --untracked-files=no)
        if ($tag -ne $version -or $modified) {
            $rev = git rev-parse --short HEAD
            if ($modified) { $rev += '-modified' }
        }
    }
    $ErrorActionPreference = 'Stop'
}
$ldflags = '-s -w -H windowsgui'
if ($rev) { $ldflags += " -X main.buildRev=$rev" }
$tags, $out = '', 'dist\eMudScan.exe'
if ($Simulate) { $tags, $out = 'simulate', 'dist\eMudScan-simulation.exe' }
go vet -tags "$tags" ./...
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
go test -tags "$tags" ./...
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
go build -trimpath -tags "$tags" -ldflags $ldflags -o $out ./cmd/emudscan
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
Write-Host $out
