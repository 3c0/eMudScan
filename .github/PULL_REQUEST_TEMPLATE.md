<!-- Thanks! A fix (bug, text, translation, documentation) can come directly. For a new check or a new feature, please open a Suggestion issue first, so that no work is wasted: see .github/CONTRIBUTING.md. -->

## What this changes

<!-- What, and why. For a fix, name the issue: "Fixes #12". -->

## Checklist

- [ ] `scripts/build.sh` (or `scripts\build.ps1` on Windows) passes: vet, tests and the Windows build, as GitHub will run them.
- [ ] Every new or changed text exists in both English and French (`cmd/emudscan/texts.go`).
- [ ] eMudScan still only reads: no file or registry write, no connection beyond the opt-in online test, no DLL loaded by name (`sysDLL`).
- [ ] One line in `CHANGELOG.md` under `## Unreleased`, if users will notice the change.

By opening this pull request, I agree that my contribution is licensed under the GNU GPL 3.0 or later, like eMudScan.
