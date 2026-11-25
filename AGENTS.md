# AGENTS.md

Instructions for AI coding agents working on eMudScan. They sum up
[CONTRIBUTING.md](.github/CONTRIBUTING.md) and
[docs/building.md](docs/building.md), and add what an agent must not miss.

## The project

eMudScan finds the software on a Windows PC that still trusts the compromised
eMudhra intermediate certificates, and fixes what it can once the user confirms.
It is written in Go, with the standard library only and without cgo, as one
package, `cmd/emudscan/`, which builds as two programs:

- `eMudScan.exe`, the window program (Win32, through `syscall`);
- `eMudScan-cli.exe`, the console program, with the `cli` build tag.

The code that needs Windows (the window, running the fixes, installing the
update) is in the `_windows.go` files; the rest builds and is tested on any
system. `data/` is the reference data of we-have-a-problem.com, checked against
the SHA-256 it publishes: never edit it.

## Build and test

Go 1.24 or later, nothing else.

- `./scripts/build.sh` (Linux or macOS) or `.\scripts\build.ps1` (Windows): vet
  for Linux and Windows, the tests, then the two exes in `dist/`.
- `gofmt -l cmd data` must print nothing.
- A change to the Fix function also passes `./scripts/build.sh --simulate`.

On Linux, the tests that run the fixes in PowerShell and those of the update
that need Windows are skipped: the GitHub workflows run them.

## What a change must keep

The rules of [CONTRIBUTING.md](.github/CONTRIBUTING.md#what-emudscan-must-keep)
come first: nothing changes on the PC until the user confirms a fix, no
connection beyond **Online checks and updates**, Windows DLLs and Windows
PowerShell from System32 only (`sysDLL`), standard library only. Also:

- **No false detection.** Add only checks that are reliable, and leave out a
  result that cannot be proven.
- **A fix uses the latest release of the same MAJOR.MINOR line** of the
  software, which eMudScan looks for when the Fix window opens.
- **One User Account Control prompt** for all the commands that need
  administrator rights.
- **Commands run through `os/exec`.**
- **The ssl files are replaced only in a Python inside another program**, never
  in a Python installed with an installer.
- **The certificates of the tests of a fixed Python** (`pycheck.go`) are made as
  they are now.
- **The update of eMudScan** trusts only an immutable release whose notes give
  the SHA-256 of `eMudScan.exe`. Keep the `--update-done` option as it is in
  every version. Never move the running exe aside to write the new one in its
  place.
- **Guard against the other programs of the same PC**: a file that eMudScan
  writes, then runs or trusts, is created new in a folder of its own that other
  users cannot change, kept open and locked while in use, and checked before
  use.

## Texts and documentation

- Every text shown to the user is in `cmd/emudscan/texts.go`, in English and
  French, in the style of the existing ones: short, plain words, the same terms
  throughout.
- `docs/using.md` tells the users only what they would not find or expect by
  themselves: not what a common control does, nor how eMudScan works inside.
- `AGENTS.md`, `docs/` and `.github/CONTRIBUTING.md` are wrapped at 80 columns,
  never inside a bold span, a code span or a link. `README.md`, `CHANGELOG.md`
  and the release notes have one line per paragraph or item.

## Changelog and release notes

- `CHANGELOG.md` follows
  [Keep a Changelog 1.1.0](https://keepachangelog.com/en/1.1.0/): each change
  that users can notice gets one entry under `## [Unreleased]`, in `### Added`,
  `### Changed`, `### Fixed`… An entry is for developers, written as
  [Common Changelog](https://github.com/vweevers/common-changelog) asks: one
  line per change, in the imperative, starting with a verb (“Add…”, “Rename…”,
  “Fix…”), in technical words, with the previous behavior when it changed. A
  simple change stays short; an important feature or a major rewrite may take up
  to 650 characters. A change that breaks a use starts with `**Breaking:**`. No
  link to commits.
- `.github/release-notes/<version>.md` is for the users, while the changelog
  is for developers. It says what they can now do, in plain words, without file
  names: one sentence that sums up the version; `### Highlights`, the few main
  changes, each with a title in bold, one or two sentences and a link to the
  documentation; `### Also in this version`, a short line for each other change
  they can notice; `### Fixed`, what happens now. It says “you can now click…”,
  and never gives an order (“click…”). Minor changes are left to the changelog.
  [docs/building.md](docs/building.md#publishing-a-version) has the details.

## Commits

- [Conventional Commits](https://www.conventionalcommits.org/):
  `type(scope): description`, in lowercase and in ASCII, at most 76 characters,
  without a final period.
- One commit per change; small related changes go together.
- `main` has a linear history. Never force-push: a fix to a pushed commit is a
  new commit on top of it.
- Never tag or publish a version: the maintainer does it with
  `scripts/release.sh`.

## Privacy

Nothing personal goes into the code, the tests, the texts, the documentation or
the commits: no real user, PC, drive, email address or installed software. Use
invented names, such as `Example App 1`, `MY-PC` and `user@example.invalid`.
