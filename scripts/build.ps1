# Builds dist\eMudScan.exe (the window program) and dist\eMudScan-cli.exe (the
# console program that scans from the command line) on Windows (PowerShell):
# .\scripts\build.ps1
# Checks and tests the code, regenerates the Windows resources
# (rsrc_windows_amd64.syso in cmd\emudscan\winres\gui and cli), as scripts/build.sh
# and the GitHub build do, then compiles them.
# .\scripts\build.ps1 -Simulate builds only dist\eMudScan-simulation.exe instead,
# whose Fix function runs nothing (see cmd\emudscan\fixsim_on.go).
param([switch]$Simulate)
$ErrorActionPreference = 'Stop'
Set-Location (Join-Path $PSScriptRoot '..')
$env:CGO_ENABLED = '0'
# A build that is not of a release (the commit tagged with appVersion, unchanged)
# shows its commit instead of the date: "3.0.1+dev (ae7832d)", "3.0.1+dev (ae7832d-modified)".
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
$ldflags = '-s -w'
if ($rev) { $ldflags += " -X main.buildRev=$rev" }
$tags, $out, $cli = '', 'dist\eMudScan.exe', 'dist\eMudScan-cli.exe'
if ($Simulate) { $tags, $out, $cli = 'simulate', 'dist\eMudScan-simulation.exe', '' }

# Each line starts with the time since the line before it and since the start, such
# as "[+02.40 06.90]": each stage says what it does, and the times show which one
# takes long.
$clock = [Diagnostics.Stopwatch]::StartNew()
$script:last = $null
function Say([string]$text) {
    $now = $clock.Elapsed.TotalSeconds
    $sign, $delta = '+', ($now - $script:last)
    if ($null -eq $script:last) { $sign, $delta = ' ', 0.0 }
    $script:last = $now
    $inv = [Globalization.CultureInfo]::InvariantCulture # 02.40, not 02,40
    Write-Host ('[' + $sign + $delta.ToString('00.00', $inv) + ' ' + $now.ToString('00.00', $inv) + '] ' + $text)
}
# Run says the stage, runs its command and says each line that the command writes;
# it stops the script if the command fails.
function Run([string]$stage, [scriptblock]$command) {
    Say $stage
    $ErrorActionPreference = 'Continue' # what a command writes to stderr is a line like the others
    & $command 2>&1 | ForEach-Object { Say "$_" }
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
}
Run 'Checking the code (go vet)...' { go vet -tags "$tags" ./... }
if ($cli) { Run 'Checking the code of the console program (go vet)...' { go vet -tags cli ./... } }
Run 'Running the tests (go test)...' { go test -tags "$tags" ./... }
Run 'Regenerating the Windows resources...' { go run ./cmd/emudscan --make-syso cmd\emudscan\winres\gui\rsrc_windows_amd64.syso }
Run 'Regenerating the Windows resources of the console program...' { go run ./cmd/emudscan --make-syso-cli cmd\emudscan\winres\cli\rsrc_windows_amd64.syso }
# the window program: -H windowsgui; the console program: the tag cli, without it
Run "Building $out..." { go build -trimpath -tags "$tags" -ldflags "$ldflags -H windowsgui" -o $out ./cmd/emudscan }
if ($cli) {
    Run "Building $cli..." { go build -trimpath -tags cli -ldflags $ldflags -o $cli ./cmd/emudscan }
    Say "Done: $out and $cli"
} else {
    Say "Done: $out"
}
