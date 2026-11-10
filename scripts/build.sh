#!/bin/sh
# Builds dist/eMudScan.exe from Linux or macOS, like the GitHub build: checks and
# tests the code, regenerates the Windows resources (icon, manifest, version
# information), then compiles for Windows.
# scripts/build.sh --simulate builds dist/eMudScan-simulation.exe instead, whose Fix
# function runs nothing (see cmd/emudscan/fixsim_on.go).
set -e
cd "$(dirname "$0")/.."
tags="" out=dist/eMudScan.exe
if [ "$1" = "--simulate" ]; then
	tags=simulate out=dist/eMudScan-simulation.exe
fi
# A build that is not of a release (the commit tagged with appVersion, unchanged)
# shows its commit instead of the date: "3.0.1+dev (ae7832d)", "3.0.1+dev (ae7832d-modified)".
# The GitHub build of a tag is a release (the workflow checks the tag).
rev=""
if [ "$GITHUB_REF_TYPE" != "tag" ] && command -v git >/dev/null 2>&1 && git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
	version=$(sed -n 's/^const appVersion *= *"\([^"]*\)".*/\1/p' cmd/emudscan/model.go)
	modified=$(git status --porcelain --untracked-files=no)
	if [ "$(git describe --tags --exact-match 2>/dev/null)" != "$version" ] || [ -n "$modified" ]; then
		rev=$(git rev-parse --short HEAD)
		[ -n "$modified" ] && rev="$rev-modified"
	fi
fi
ldflags="-s -w -H windowsgui"
[ -n "$rev" ] && ldflags="$ldflags -X main.buildRev=$rev"
go vet -tags "$tags" ./...
GOOS=windows GOARCH=amd64 go vet -tags "$tags" ./...
go test -tags "$tags" ./...
go run ./cmd/emudscan --make-syso cmd/emudscan/rsrc_windows_amd64.syso
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -tags "$tags" -ldflags "$ldflags" -o "$out" ./cmd/emudscan
echo "$out"
