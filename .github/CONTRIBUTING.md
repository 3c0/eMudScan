# Contributing to eMudScan

## Reporting an issue

Use the [issue forms](https://github.com/3c0/eMudScan/issues/new/choose) for a
bug, a false detection (software judged incorrectly) or a suggestion. Report a
vulnerability through **Report a vulnerability** on the
[Security tab](https://github.com/3c0/eMudScan/security), never in a public
issue (see [SECURITY.md](SECURITY.md)).

## Pull requests

- A fix (a bug, a text, a translation, the documentation) can be submitted
  directly.
- For a new check or a new feature, please open a **Suggestion** issue first and
  wait for an answer, so that no work is wasted on something that will not be
  merged.
- One subject per pull request, kept small; the description template lists what
  to check before opening it.
- `main` has a linear history, without merge commits: pull requests are merged
  with **Rebase and merge** or **Squash and merge**. To bring your branch up to
  date, rebase it on `main` (`git rebase main`) instead of merging `main` into it.
- The GitHub build runs on a pull request from a fork once the maintainer
  approves it.

## What eMudScan must keep

These rules are the project’s promises to its users; a change that breaks any of
them is not merged:

- **It changes nothing until the user confirms a fix.** A scan writes no file
  except the report the user chooses to save, writes nothing to the registry,
  and saves no settings. Only the Fix function changes the PC, once the user
  accepts its warning, and it runs only commands that the detailed information
  shows (`newFix`, `fixes.go`).
- **eMudScan itself needs no administrator rights.** The commands of a fix that
  need them run in a Windows PowerShell for which Windows asks the user’s
  permission (User Account Control).
- **No connection** other than the online test, and none at all when the online
  test is not allowed (`docs/privacy.md`). The commands of a fix connect only to
  the sources of the package managers and to curl.se.
- **Nothing runs or is sent without a click**: links open on click, and commands
  run only from the Fix window, once the user accepts its warning.
- **Windows DLLs and Windows PowerShell come from System32 only** (`sysDLL`,
  `dll_windows.go`; `fixexec_windows.go`), never by bare name.
- **Standard library only**, no cgo, a single self-contained `.exe`: adding a
  dependency is discussed in an issue first.
- **A verdict is checked, not assumed**: a certificate bundle is tested against
  the chains of we-have-a-problem.com, a block list is looked for in the file,
  and a version is compared with the publisher’s fixed versions (`fixRules`,
  `model.go`).

## Code

- Go, formatted with `gofmt`, with no linter directives needed.
  `scripts/build.sh` runs vet (Linux and Windows), the tests and the Windows
  build; `scripts\build.ps1` does the same on Windows. See
  [docs/building.md](../docs/building.md).
- Every text shown to the user is in `cmd/emudscan/texts.go`, in English and
  French; `TestTranslations` checks that both exist and use the same format
  verbs.
- Texts follow the style of the existing ones: American English, short
  sentences, the same terms throughout (certificate bundle, root store, block
  list; in French, fichier de certificats, magasin de certificats racines, liste
  de blocage), names of buttons and items in bold in the detailed information
  windows, and curly quotation marks and apostrophes (“ ” and ’ in English; « »
  with non-breaking spaces, and a non-breaking space before : ; ! ? in French).
- Logic gets a test (`*_test.go`); file readers have test data in
  `cmd/emudscan/testdata/`.
- A user-visible change gets one line in `CHANGELOG.md`, which follows
  [Keep a Changelog](https://keepachangelog.com/en/1.1.0/): under
  `## [Unreleased]`, in the section for its type. The sections come in this
  order, the most urgent first: `### Security` for a vulnerability, `### Fixed`
  for a bug fix, `### Removed` for a removed feature, `### Deprecated` for a
  feature that will be removed, `### Changed` for a change to an existing one
  and `### Added` for a new feature.
  Write it on a single line (it becomes the release page): one short sentence,
  or two for a bug fix or a vulnerability (the problem, then the fix).
- Commit messages follow [Conventional Commits](https://www.conventionalcommits.org/):
  `type(scope): description`, in lowercase and without a final period. The type
  is `feat` for a new feature, `fix` for a bug fix, and `perf`, `refactor`,
  `test`, `docs`, `build` or `chore` for the rest, such as
  `perf(scan): read 8 folders at a time when scanning the drives`. Pull request
  titles follow the same format.
- The reference data (`data/`) comes from we-have-a-problem.com and is checked
  against the SHA-256 it publishes: do not edit it by hand.

## License

eMudScan is free software under the GNU GPL 3.0 or later. By contributing, you
agree that your contribution is licensed under the same terms.
