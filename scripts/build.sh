#!/bin/sh
# Builds dist/eMudScan.exe (the window program) and dist/eMudScan-cli.exe (the console
# program that scans from the command line) from Linux or macOS, like the GitHub
# build: checks and tests the code, regenerates the Windows resources (icon, manifest,
# version information), then compiles for Windows.
# scripts/build.sh --simulate builds only dist/eMudScan-simulation.exe instead, whose
# Fix function runs nothing (see cmd/emudscan/fixsim_on.go).
set -e
cd "$(dirname "$0")/.."
tags="" out=dist/eMudScan.exe cli=dist/eMudScan-cli.exe
if [ "$1" = "--simulate" ]; then
	tags=simulate out=dist/eMudScan-simulation.exe cli=""
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
ldflags="-s -w"
[ -n "$rev" ] && ldflags="$ldflags -X main.buildRev=$rev"

build() {
	echo "Checking the code (go vet)..."
	go vet -tags "$tags" ./...
	GOOS=windows GOARCH=amd64 go vet -tags "$tags" ./...
	if [ -n "$cli" ]; then
		GOOS=windows GOARCH=amd64 go vet -tags cli ./...
	fi
	echo "Running the tests (go test)..."
	go test -tags "$tags" ./...
	echo "Regenerating the Windows resources..."
	go run ./cmd/emudscan --make-syso cmd/emudscan/winres/gui/rsrc_windows_amd64.syso
	go run ./cmd/emudscan --make-syso-cli cmd/emudscan/winres/cli/rsrc_windows_amd64.syso
	# the window program: -H windowsgui; the console program: the tag cli, without it
	echo "Building $out..."
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -tags "$tags" -ldflags "$ldflags -H windowsgui" -o "$out" ./cmd/emudscan
	if [ -n "$cli" ]; then
		echo "Building $cli..."
		GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -tags cli -ldflags "$ldflags" -o "$cli" ./cmd/emudscan
	fi
	echo "Done: $out${cli:+ and $cli}"
}

# Each line starts with the time since the line before it and since the start, such
# as "[+02.40 06.90]": each stage says what it does, and the times show which one
# takes long.
now() { # in hundredths of a second; in whole seconds where date has no %N (macOS)
	t=$(date +%s%N)
	case $t in
	*N) echo $(($(date +%s) * 100)) ;;
	*) echo "${t%???????}" ;;
	esac
}
cs() { printf '%02d.%02d' $(($1 / 100)) $(($1 % 100)); }
stamp() {
	start=$(now) last="" sign=" "
	while IFS= read -r line; do
		t=$(now)
		printf '[%s%s %s] %s\n' "$sign" "$(cs $((t - ${last:-$t})))" "$(cs $((t - start)))" "$line"
		last=$t sign=+
	done
}
# the status of build, which the pipe would lose; a failed command stops build
# before it writes it
status=$(mktemp)
trap 'rm -f "$status"' EXIT
{
	build 2>&1
	echo $? >"$status"
} | stamp
exit "$(cat "$status" 2>/dev/null | grep . || echo 1)"
