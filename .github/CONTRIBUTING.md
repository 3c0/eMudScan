# Contributing to eMudScan

## Reporting something

Use the [issue forms](https://github.com/3c0/eMudScan/issues/new/choose): a bug,
a false detection (a software judged wrongly), a suggestion. A vulnerability goes
through **Report a vulnerability** on the [Security tab](https://github.com/3c0/eMudScan/security),
never through a public issue (see [SECURITY.md](SECURITY.md)).

## Pull requests

- A fix — a bug, a text, a translation, the documentation — can come directly.
- A new check or a new feature: please open a *Suggestion* issue first and wait
  for an answer, so that no work is wasted on something that will not be merged.
- One subject per pull request, kept small; the description template says what
  to check before opening it.
- The GitHub build runs on a pull request from a fork after the maintainer
  approves it.

## What eMudScan must keep

These rules are the project's promises to its users; a change that breaks one
of them is not merged:

- **It only reads.** No file is written except the report the user chooses to
  save; nothing is written in the registry; no setting is saved.
- **No connection** other than the online test, and none at all when it is
  unticked (`docs/privacy.md`).
- **Nothing is sent to anyone** without a click: links open on click, commands
  are shown, never run.
- **Windows' DLLs are loaded from System32 only** (`sysDLL`, `dll_windows.go`);
  never by bare name.
- **Standard library only**, no cgo, one self-contained `.exe`: adding a
  dependency is discussed in an issue first.
- **A verdict is checked, not assumed**: a certificate list is tested against
  the advisory's chains, a block list is looked for in the file, a version is
  compared with the publisher's fixed versions (`fixRules`, `model.go`).

## Code

- Go, `gofmt`, no linter directives needed. `scripts/build.sh` runs vet (Linux
  and Windows), the tests and the Windows build; `scripts\build.ps1` does the
  same on Windows. See [docs/building.md](../docs/building.md).
- Every text shown to the user is in `cmd/emudscan/texts.go`, in English and
  French; `TestTranslations` checks that both exist and use the same verbs.
- Logic gets a test (`*_test.go`); the readers of files have test data in
  `cmd/emudscan/testdata/`.
- A user-visible change gets one line in `CHANGELOG.md` under `## Unreleased`,
  written on a single line (it becomes the release page): one short sentence
  for a new feature or an improvement, two for a bug fix (the bug, then the fix).
- Commit messages follow [Conventional Commits](https://www.conventionalcommits.org/):
  `type(scope): description`, in lower case and without a final period. The type
  is `feat` for a new feature, `fix` for a bug fix, and `perf`, `refactor`,
  `test`, `docs`, `build` or `chore` for the rest, such as
  `perf(scan): read 8 folders at a time when scanning the drives`. The title of a
  pull request follows the same form.
- The reference data (`data/`) comes from the advisory and is checked against
  the SHA-256 it publishes: do not edit it by hand.

## Licence

eMudScan is free software under the GNU GPL 3.0 or later. By contributing, you
agree that your contribution is licensed the same way.
