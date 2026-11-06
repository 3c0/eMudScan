#!/bin/sh
# Publishes a version: run it in Git Bash (or a Linux or macOS terminal) at the
# root of eMudScan, on main, as docs/building.md says ("Publishing a version"):
#   sh scripts/release.sh 2.1.0
#
# Before running it, list the version's changes under "## [Unreleased]" in
# CHANGELOG.md. The script then:
# 1. checks that the version follows the current one (2.0.1, 2.1.0 or 3.0.0 after
#    2.0.0), that nothing is left uncommitted, that main is up to date with
#    GitHub, and that the changelog has changes to publish;
# 2. shows the commit the version is based on, the commits it brings, the changes
#    of the changelog, and asks for your confirmation: until you answer "y",
#    nothing changes;
# 3. sets appVersion and appDate (today) in cmd/emudscan/model.go, turns
#    "## [Unreleased]" into the version's section in CHANGELOG.md, with its links,
#    and regenerates the Windows resources (except on Windows, where
#    scripts\build.ps1 uses the committed file and the GitHub build regenerates
#    them);
# 4. runs the checks and the tests: if they fail, it puts the files back as they
#    were and stops;
# 5. commits "chore(release): 2.1.0", pushes it, creates the tag 2.1.0 and pushes
#    it. GitHub then builds the exe and creates the draft release.
#
# On a hotfix branch (hotfix-2.1.1, see "Publishing a hotfix"), it pushes only the
# tag, not the branch.
set -e
fail() { echo "Error: $*" >&2; exit 1; }

v=$1
echo "$v" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+$' || fail "usage: sh scripts/release.sh <version>, such as 2.1.0"
cd "$(dirname "$0")/.."
model=cmd/emudscan/model.go

# 1. checks
branch=$(git branch --show-current)
case "$branch" in
main | "hotfix-$v") ;;
hotfix-*) fail "on the branch $branch, the version to publish is ${branch#hotfix-}." ;;
*) fail "run it on main (or on the branch hotfix-$v for a hotfix): you are on \"$branch\"." ;;
esac
[ -z "$(git status --porcelain --untracked-files=no)" ] || fail "there are changes that are not committed: commit or stash them first."
prev=$(sed -n 's/^const appVersion = "\(.*\)"$/\1/p' "$model")
[ -n "$prev" ] || fail "no appVersion in $model."
git rev-parse -q --verify "refs/tags/$prev" >/dev/null || fail "there is no tag $prev, the current version."
pM=${prev%%.*} pr=${prev#*.} pm=${pr%%.*} ph=${pr#*.}
case "$v" in
"$pM.$pm.$((ph + 1))" | "$pM.$((pm + 1)).0" | "$((pM + 1)).0.0") ;;
*) fail "$v does not follow $prev: the next version is $pM.$pm.$((ph + 1)) (hotfix), $pM.$((pm + 1)).0 (minor) or $((pM + 1)).0.0 (major)." ;;
esac
git merge-base --is-ancestor "$prev" HEAD || fail "this branch does not contain $prev, the current version."
if [ "$branch" != main ] && [ "$v" != "$pM.$pm.$((ph + 1))" ]; then
	fail "on a hotfix branch, the version is $pM.$pm.$((ph + 1))."
fi
[ -n "$(git rev-list "$prev..HEAD")" ] || fail "there is no commit since $prev: nothing to publish."
! git rev-parse -q --verify "refs/tags/$v" >/dev/null || fail "the tag $v already exists."
echo "Checking GitHub…"
git fetch -q origin
[ -z "$(git ls-remote --tags origin "refs/tags/$v")" ] || fail "GitHub already has a tag $v."
unpushed=""
if [ "$branch" = main ]; then
	[ -z "$(git rev-list HEAD..origin/main)" ] || fail "GitHub's main has commits that yours does not: git pull first."
	unpushed=$(git log --oneline origin/main..HEAD)
fi
changes=$(awk '/^## \[Unreleased\]$/ {on = 1; next} on && /^## \[/ {exit} on' CHANGELOG.md)
echo "$changes" | grep -q '[^[:space:]]' || fail "CHANGELOG.md has no changes under \"## [Unreleased]\": list the changes of $v there first."
grep -q '^\[unreleased\]: .*/compare/'"$prev"'\.\.\.HEAD$' CHANGELOG.md || fail "the [unreleased] link at the end of CHANGELOG.md does not compare $prev with HEAD."

# 2. what it will publish
date=$(date +%Y%m%d)
day=$(date +%Y-%m-%d)
echo
echo "Version $v ($date), after $prev, on the branch $branch, based on:"
git log --oneline -n 1 | sed 's/^/  /'
echo
echo "Commits since $prev:"
git log --oneline "$prev..HEAD" | sed 's/^/  /'
if [ -n "$unpushed" ]; then
	echo
	echo "Not on GitHub yet, so published with it (check that they are finished):"
	echo "$unpushed" | sed 's/^/  /'
fi
echo
echo "Its changes, from CHANGELOG.md:"
echo "$changes" | sed '/^$/d; s/^/  /'
echo
printf 'Publish %s? Type y to go on, anything else to stop: ' "$v"
read -r answer || answer=""
[ "$answer" = y ] || { echo "Stopped: nothing changed."; exit 1; }

# 3. the release changes
restore() {
	git checkout -q -- "$model" CHANGELOG.md cmd/emudscan/rsrc_windows_amd64.syso
	echo "Error: $1 The files are back as they were: nothing changed." >&2
	exit 1
}
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
awk -v v="$v" -v d="$date" '
	/^const appVersion = "/ { print "const appVersion = \"" v "\""; next }
	/^const appDate = "/ { print "const appDate = \"" d "\""; next }
	{ print }
' "$model" >"$tmp/model" && cp "$tmp/model" "$model"
awk -v v="$v" -v p="$prev" -v day="$day" '
	$0 == "## [Unreleased]" { print; print ""; print "## [" v "] - " day; next }
	/^\[unreleased\]: / {
		base = $0; sub(/^\[unreleased\]: /, "", base); sub(/\/compare\/.*$/, "", base)
		print "[unreleased]: " base "/compare/" v "...HEAD"
		print "[" v "]: " base "/compare/" p "..." v
		next
	}
	{ print }
' CHANGELOG.md >"$tmp/changelog" && cp "$tmp/changelog" CHANGELOG.md
case "$(uname -s)" in
MINGW* | MSYS* | CYGWIN*) echo "Windows: the Windows resources are not regenerated (the GitHub build does it)." ;;
*) go run ./cmd/emudscan --make-syso cmd/emudscan/rsrc_windows_amd64.syso || restore "the Windows resources could not be regenerated." ;;
esac

# 4. checks and tests
echo "Checking and testing…"
go vet ./... || restore "go vet failed."
GOOS=windows GOARCH=amd64 go vet ./... || restore "go vet failed (Windows)."
go test ./... || restore "the tests failed."

# 5. commit, tag, push
git commit -q -m "chore(release): $v" -- "$model" CHANGELOG.md cmd/emudscan/rsrc_windows_amd64.syso
if [ "$branch" = main ]; then
	git push -q origin main || fail "the push of main failed: the commit \"chore(release): $v\" is only on your PC. Fix the problem, then run: git push, git tag $v and git push origin $v"
fi
git tag "$v"
git push -q origin "$v" || fail "the push of the tag failed: run git push origin $v"

repo=$(git remote get-url origin | sed 's/\.git$//; s|^git@github\.com:|https://github.com/|')
echo
echo "Done: $v is committed, tagged and pushed."
echo "Next, as docs/building.md says:"
echo "  1. Wait for the build: $repo/actions"
echo "  2. Check the draft release, scan its eMudScan.exe on Jotti and add the line: $repo/releases"
echo "  3. Publish it."
if [ "$branch" != main ]; then
	echo "  4. Copy the hotfix into main: git switch main, then sh scripts/hotfix-to-main.sh $v"
fi
