#!/bin/sh
# Builds dist/eMudScan.exe from Linux or macOS, like the GitHub build: checks and
# tests the code, regenerates the Windows resources (icon, manifest, version
# information), then compiles for Windows.
set -e
cd "$(dirname "$0")/.."
go vet ./...
GOOS=windows GOARCH=amd64 go vet ./...
go test ./...
go run ./cmd/emudscan --make-syso cmd/emudscan/rsrc_windows_amd64.syso
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -H windowsgui" -o dist/eMudScan.exe ./cmd/emudscan
echo "dist/eMudScan.exe"
