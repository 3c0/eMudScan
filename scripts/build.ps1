# Builds dist\eMudScan.exe on Windows (PowerShell): .\scripts\build.ps1
# Checks and tests the code, then compiles it. The Windows resources
# (cmd\emudscan\rsrc_windows_amd64.syso) can only be regenerated on Linux or macOS, by
# scripts/build.sh or the GitHub build; this script uses the file as it is.
$ErrorActionPreference = 'Stop'
Set-Location (Join-Path $PSScriptRoot '..')
$env:CGO_ENABLED = '0'
go vet ./...
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
go test ./...
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
go build -trimpath -ldflags '-s -w -H windowsgui' -o dist\eMudScan.exe ./cmd/emudscan
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
Write-Host 'dist\eMudScan.exe'
