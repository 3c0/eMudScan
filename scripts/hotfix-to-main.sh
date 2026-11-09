#!/bin/sh
# Copies a published hotfix into main, whose history stays linear (no merge
# commits). Run it on main once the hotfix's tag is pushed, as docs/building.md
# says ("Publishing a hotfix"):
#   sh scripts/hotfix-to-main.sh 3.1.1
#
# 1. It copies the commits of the hotfix, except "chore(release): 3.1.1", without
#    their changes to CHANGELOG.md, which keeps the unreleased changes of main.
# 2. It adds to CHANGELOG.md the version's section and links, and to model.go its
#    appVersion and appDate, as in the tag, in a commit "chore(release): add 3.1.1
#    to main".
#
# If a commit conflicts with main elsewhere than in CHANGELOG.md, it stops and says
# how to finish that commit; run it again then: it goes on where it stopped.
set -e
fail() { echo "Error: $*" >&2; exit 1; }

v=$1
case "$v" in
[0-9]*.[0-9]*.[0-9]*) ;;
*) fail "usage: sh scripts/hotfix-to-main.sh <version>, such as 3.1.1" ;;
esac
cd "$(dirname "$0")/.."
n=${v##*.}
[ "$n" -gt 0 ] 2>/dev/null || fail "$v is not a hotfix version: its last number is 0."
base=${v%.*}.$((n - 1))

[ "$(git branch --show-current)" = main ] || fail "switch to main first: git switch main"
[ -z "$(git status --porcelain --untracked-files=no)" ] || fail "main has changes that are not committed: commit or stash them first."
git rev-parse -q --verify "refs/tags/$v" >/dev/null || fail "there is no tag $v."
git rev-parse -q --verify "refs/tags/$base" >/dev/null || fail "there is no tag $base, which $v should start from."
[ "$(git log -1 --format=%s "$v")" = "chore(release): $v" ] || fail "the tag $v is not on the commit \"chore(release): $v\"."
git merge-base --is-ancestor "$base" "$v" || fail "$v does not start from $base."
git merge-base --is-ancestor "$base" HEAD || fail "main does not contain $base."
if git merge-base --is-ancestor "$v" HEAD; then
	fail "main already contains $v: there is nothing to copy."
fi
fixes=$(git rev-list --reverse "$base..$v~1")
[ -n "$fixes" ] || fail "$v has no commit besides \"chore(release): $v\"."

# 1. the fixes, each once: a commit already copied (same author date and subject,
# as git commit -C keeps them) is passed over
for c in $fixes; do
	key=$(git log -1 --format='%at %s' "$c")
	if git log --format='%at %s' "$base..HEAD" | grep -qxF "$key"; then
		continue
	fi
	echo "Copying: $(git log -1 --format=%s "$c")"
	git cherry-pick --no-commit "$c" >/dev/null 2>&1 || true
	git reset -q -- CHANGELOG.md >/dev/null 2>&1 || true
	git checkout -q -- CHANGELOG.md # main's, unchanged
	conflicts=$(git diff --name-only --diff-filter=U)
	if [ -n "$conflicts" ]; then
		echo
		echo "This commit conflicts with main in:"
		echo "$conflicts" | sed 's/^/  /'
		echo "Resolve these files, then run:"
		echo "  git add <the files>"
		echo "  git commit -C $(git rev-parse --short "$c")"
		echo "  sh scripts/hotfix-to-main.sh $v"
		exit 1
	fi
	if git diff --cached --quiet; then
		echo "  (it only changed CHANGELOG.md: nothing to copy)"
		continue
	fi
	git commit -q -C "$c"
done
git cherry-pick --quit 2>/dev/null || true

# 2. the version's section, links and version, as in the tag
grep -q "^## \[$v\] - " CHANGELOG.md && { echo "CHANGELOG.md already has $v."; exit 0; }
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
git show "$v:CHANGELOG.md" >"$tmp/tag"
git show "$v:cmd/emudscan/model.go" >"$tmp/model"
heading=$(grep -m1 "^## \[$v\] - " "$tmp/tag") || fail "the CHANGELOG.md of $v has no section for $v."
# the section: from its heading to the next version or the links at the end
awk -v h="$heading" '$0 == h {on = 1; print; next} on && (/^## \[/ || /^\[[^]]*\]: /) {exit} on' "$tmp/tag" >"$tmp/section"
unreleased=$(grep -m1 '^\[unreleased\]: ' "$tmp/tag") || fail "the CHANGELOG.md of $v has no [unreleased] link."
link=$(grep -m1 "^\[$v\]: " "$tmp/tag") || fail "the CHANGELOG.md of $v has no [$v] link."
awk -v sec="$tmp/section" -v unrel="$unreleased" -v link="$link" '
	!s && /^## \[[0-9]/ { while ((getline l < sec) > 0) print l; s = 1 }
	/^\[unreleased\]: / { print unrel; next }
	!k && /^\[[0-9][^]]*\]: / { print link; k = 1 }
	{ print }
' CHANGELOG.md >"$tmp/changelog"
cp "$tmp/changelog" CHANGELOG.md
version=$(grep -m1 '^const appVersion = ' "$tmp/model")
date=$(grep -m1 '^const appDate = ' "$tmp/model")
awk -v ver="$version" -v dat="$date" '
	/^const appVersion = / { print ver; next }
	/^const appDate = / { print dat; next }
	{ print }
' cmd/emudscan/model.go >"$tmp/model.go"
cp "$tmp/model.go" cmd/emudscan/model.go
git commit -q -m "chore(release): add $v to main" CHANGELOG.md cmd/emudscan/model.go

echo
git log --oneline "$base..HEAD"
echo
echo "Done: main holds the fixes of $v, its section in CHANGELOG.md and its version."
echo "Check them, then: git push, and git branch -D hotfix-$v"
