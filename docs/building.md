# Building and releasing

## Project layout

| Path | Contents |
|---|---|
| `cmd/emudscan/` | The program (Go, one package) and its tests. It builds as `eMudScan.exe`, the window program, and, with the `cli` build tag, as `eMudScan-cli.exe`, the console program that scans from the command line. `winres/gui` and `winres/cli` hold their Windows resources, `rsrc_windows_amd64.syso` (icon, manifest, version information); `icon256.png` is the 256-pixel icon they contain |
| `data/` | The reference data from emudhra.we-have-a-problem.com, with its notice and license, Go’s license and a copy of `LICENSE`, all embedded in the program, except the two chains of `chains/rollover`, which only the tests use |
| `docs/` | This documentation; `docs/images/` holds the screenshots and icons shown here and in the README (the icons are drawn by eMudScan’s own code, `cmd/emudscan/icons.go`) |
| `scripts/` | Build scripts for your own PC, and the scripts that publish a version (`release.sh`) and copy a hotfix into `main` (`hotfix-to-main.sh`) |
| `.github/` | The GitHub build (`workflows/build.yml`), the code analysis (`workflows/codeql.yml`), the release page template (`release-notes.md`) and the release notes of each version (`release-notes/`), the Dependabot settings (`dependabot.yml`), the security policy (`SECURITY.md`), the issue forms (`ISSUE_TEMPLATE/`), the pull request template and the contributing guide |

## GitHub builds the program

Each push to `main` and each pull request runs the checks and the tests, then
builds `eMudScan.exe` and `eMudScan-cli.exe`. Both can be downloaded from the
run’s page on the **Actions** tab, under **Artifacts** (a zip file).

## Branches

`main` is where the next version is prepared, and each version is a tag on it.
Its history is linear (no merge commits):

- pull requests are merged on GitHub with **Rebase and merge**, or **Squash and
  merge** to turn them into a single commit;
- for your own work, commit on `main` and run `git pull --rebase` before
  `git push`: your commits then go on top of what GitHub has.
  `git config pull.rebase true` makes a plain `git pull` do the same. With
  changes not committed, Git refuses to rebase: commit or stash them first;
- a commit on `main` goes out with the next push, finished or not: for longer
  work, use a branch, rebase it on `main` often (`git rebase main`), and bring
  it in with `git merge --ff-only`. If unfinished commits are already on `main`,
  `git branch wip` keeps them on a branch, then `git reset --hard origin/main`
  takes `main` back to what GitHub has.

## Publishing a version

1. Check that `## [Unreleased]` in `CHANGELOG.md` lists the version’s changes,
   in its Security, Fixed, Changed… subsections.
2. Write the release notes of the version in `.github/release-notes/3.1.0.md`,
   and commit them. They are not the changelog: they tell the users what they
   can now see or do, under `### New`, `### Improved` and `### Fixed`, with one
   short line for each change they can notice, in plain words, without file
   names or explanations. A change that a user cannot notice is left out. The
   release page links to the changelog for the full list.
3. In Git Bash, at the root of eMudScan, on an up-to-date `main` (`git pull`),
   run `sh scripts/release.sh 3.1.0` with the new version (three numbers, as
   explained at the top of [CHANGELOG.md](../CHANGELOG.md)).

The script first checks that the version follows the current one (in a
repository without any version tag yet, that it is the one `appVersion`
already gives, such as 3.0.1, and that the `[unreleased]` link at the end of
`CHANGELOG.md` gives the commits of `main`), that nothing is left uncommitted,
that `main` is up to date with GitHub, that the changelog has changes to
publish, and that the release notes of the version are written and committed. It shows the commit the version is based on,
the commits it brings (and those not pushed yet, which it would publish too),
the changes and the release notes, then asks for your confirmation. Only then does it set
`appVersion` and `appDate` (today) in `cmd/emudscan/model.go`, turn
`## [Unreleased]` into `## [3.1.0] - <date>` with its links, run the checks and
the tests (putting the files back if they fail), commit
`chore(release): 3.1.0`, push it, and create and push the tag `3.1.0`.

GitHub then builds the two exes and checks that the tag matches `appVersion` and
that `CHANGELOG.md` has a section for this version. It creates a **draft
release** with `eMudScan.exe`, `eMudScan-cli.exe` and a page made from
`.github/release-notes.md`: the text
around `@CHANGES@` is the same for every version, and `@CHANGES@` is replaced by
the release notes of the version (`.github/release-notes/3.1.0.md`), followed by
a link to the changelog. The SHA-256 of
each exe, the build log and the date of the release are filled in too. It also
records a build attestation proving that GitHub built the two exes from this
tag. Anyone can verify a copy with
`gh attestation verify eMudScan.exe -R <owner>/<repository>`.

Download `eMudScan.exe` and `eMudScan-cli.exe` from the draft and scan each one
with [Jotti’s malware scan](https://virusscan.jotti.org/en-US). In the draft,
complete the two **Tested clean** lines, so that the release page shows that
each exe was tested clean: replace `JOTTI_SCAN_ID` (for `eMudScan.exe`) and
`JOTTI_SCAN_ID_CLI` (for `eMudScan-cli.exe`; a double-click selects each) with
the ID at the end of the address of its results, and the number of antivirus
engines if Jotti used another one. Their date is the date of the release: change
it if you scan on another day.

Antivirus engines update their detections all the time: scan the exes again a day
or more after the release, then edit the notes of the published release with the
new date and links. GitHub allows editing the notes of a published release, even
an immutable one.

Check the draft, then click **Publish release**. GitHub adds the source code to
the release, as the GPL requires. Publish only a draft that is right: releases
are immutable in this repository, so the tag of a published release can never be
reused, even after the release is deleted.

If the GitHub build fails, no draft is created. Fix the problem in a new
commit and push it, then move the tag to it: the **Version tags** ruleset
protects tags, so set it to **Disabled** meanwhile, run
`git push origin :refs/tags/3.1.0`, `git tag -f 3.1.0` and
`git push origin 3.1.0`, then set it back to **Active**.

## Publishing a hotfix

A hotfix brings small or urgent fixes to the latest version, such as 3.1.1
after 3.1.0; an older version gets none.

- When the commits on `main` since that version can be published with the
  fixes, such as documentation changes, commit the fixes on `main` and publish
  them as above: `sh scripts/release.sh 3.1.1`.
- When `main` holds changes for the next version, publish them with the fixes
  as the next minor version (3.2.0) instead, if they are ready.

Otherwise, publish the hotfix from a branch:

1. Put your work in progress aside: `git stash` the changes not committed, and
   move the commits not pushed yet to a branch (`git branch wip`, then
   `git reset --hard origin/main`).
2. Create a branch from the version’s tag: `git switch -c hotfix-3.1.1 3.1.0`.
3. Commit the fixes there, with their changelog entries and the release notes
   of the hotfix (`.github/release-notes/3.1.1.md`), then run
   `sh scripts/release.sh 3.1.1`: on this branch, it pushes only the tag.
4. Copy the hotfix into `main`: `git switch main`, then
   `sh scripts/hotfix-to-main.sh 3.1.1`. It copies the commits of the fixes,
   without merge commits, and leaves the unreleased changes in `CHANGELOG.md` as
   they are. Then it adds the 3.1.1 section and its links to `CHANGELOG.md`, and
   its version to `model.go`, as in the tag, in a commit
   `chore(release): add 3.1.1 to main`. If a commit of the fixes conflicts with
   `main`, it stops and says how to go on.
5. Finish in `main`:
   - `git push`: sends to GitHub what `hotfix-to-main.sh` added to your `main`,
     the fixes and `chore(release): add 3.1.1 to main`, so that the next version
     includes them. No tag to create: `release.sh` already created and pushed
     `3.1.1`, on the hotfix branch.
   - `git branch -D hotfix-3.1.1`: deletes the branch, which is no longer
     needed. `-D` because Git sees that its commits are not in `main` (only
     copies of them are) and would refuse `-d`. Nothing is lost: the tag `3.1.1`
     keeps them.
   - If you moved commits to `wip` at the start, put them back on `main`, after
     the hotfix:
     - `git switch wip`, then `git rebase main`: replays your commits on top of
       the hotfix, as if you had started them after it. Where `CHANGELOG.md`
       conflicts, keep both sides: your entries under `## [Unreleased]`, the
       3.1.1 section below.
     - `git switch main`, then `git merge --ff-only wip`: moves `main` forward
       to your commits, without a merge commit.
     - `git branch -d wip`: deletes `wip`, whose commits are now in `main`.
   - If you stashed changes at the start, `git stash pop` gives them back.

A build that is not of a release, that is of a commit other than the one tagged
with `appVersion`, or with changes not yet committed, shows “+dev” and its commit
instead of the date: “3.0.1+dev (ae7832d)”, or “3.0.1+dev (ae7832d-modified)”. It
does so in the window title, the report and the User-Agent of the online test,
so that a report tells which code produced it. The build scripts set it from git
(`-ldflags "-X main.buildRev=…"`); the GitHub build of a version tag is a release.

## Building on your PC

You need Go 1.24 or later, and nothing else. From the root of the repository:

- Windows (PowerShell): `.\scripts\build.ps1`
- Linux or macOS: `./scripts/build.sh`

Both run the checks and the tests, regenerate the Windows resources, then write
`dist/eMudScan.exe` and `dist/eMudScan-cli.exe`. Each line they show starts with
the time since the line before it and since the start, such as `[+02.40 06.90]`.

The two exes are the same code. `eMudScan.exe` is built as a window program
(`-H windowsgui`). `eMudScan-cli.exe` is built with the `cli` build tag and
without that flag, as a console program: Windows reads this type in the exe, and
a command prompt waits only for a console program, gives it its outputs and gets
its exit code. The tag selects its entry point (`main_cli_windows.go` instead of
`main_gui_windows.go`) and its resources.

To test the Fix function without changing anything, `.\scripts\build.ps1 -Simulate`
(or `./scripts/build.sh --simulate`) writes only `dist/eMudScan-simulation.exe`
instead. It runs Windows PowerShell and asks for administrator rights as usual,
but runs nothing in place of each command, which then shows ✓ and “Done
(simulated)”, after a second for each software, as its commands would take some
time; a software whose commands all succeeded is then shown as fixed. A command
that contains the text of the environment variable `EMUDSCAN_SIMULATE_FAIL` shows
✗ and “Failed (simulated)” instead; a download that fails stops the fixes, and an
update that fails stops them too when `EMUDSCAN_SIMULATE_OFFLINE` is set, as when
the servers of its package manager cannot be reached (the simulation connects to
none). Its window title and its report say SIMULATION. The normal build contains
none of this: the simulation is compiled only with the `simulate` build tag,
which the GitHub build never sets.

Every build also regenerates the Windows resources of the two exes (the
`rsrc_windows_amd64.syso` of `cmd/emudscan/winres/gui` and of
`cmd/emudscan/winres/cli`: the icon, the manifest and the version information,
which names each exe), so that the files’ properties show the version set in
`model.go`: `scripts/build.ps1`, `scripts/build.sh`, `scripts/release.sh` and the
GitHub build. Go links into each exe the resources of the package that its entry
point imports. The result is the same with any Go version and on any system, so
the committed files never show as changed and the exes do not say they were
built from modified files; a test checks that the committed files are up to
date.

The 256-pixel icon is the only part that needs care: Go’s PNG compression
changes from one Go version to another, so the resources take it as it is from
`cmd/emudscan/icon256.png`, compressed once. If you change the drawing of the
application icon (`icons.go`), a test fails until you write it again:
`go run ./cmd/emudscan --make-icon256 cmd/emudscan/icon256.png`, then compress
it with a PNG optimizer that keeps 8-bit RGBA, the exact pixels (no alpha
optimization), no interlacing and no metadata, and regenerate the resources.

## Rebuilding a release

The build is reproducible: built from the same commit with the same Go version,
the two exes come out the same, byte for byte, on Windows, Linux or macOS and in
any folder. So anyone can check that the exes of a release come from its source
code, without trusting GitHub or eMudScan’s author:

1. Find the Go version of the release: `go version eMudScan.exe` shows the one
   that built an exe, such as `go1.27.4` for eMudScan 3.1.0.
2. Get the source code of the release:
   `git clone https://github.com/3c0/eMudScan`, then `git checkout 3.1.0` in
   its folder.
3. With that Go version, run `.\scripts\build.ps1` or `./scripts/build.sh`.
4. Compare the SHA-256 of `dist/eMudScan.exe` and of `dist/eMudScan-cli.exe` with
   those of the release page: `Get-FileHash dist\eMudScan.exe` in PowerShell,
   `sha256sum dist/eMudScan.exe` on Linux.

It works because eMudScan has no dependencies, the build records no folder
(`-trimpath`) and no date but the one of the commit, and the Windows resources
are generated from the source code. The files must be exactly those of the
version tag: with another commit or a changed file, the build scripts make a
build that is not of a release, which shows its commit and is another file.

It was checked for eMudScan 3.1.0: built on a Windows PC, both exes have the
SHA-256 of those that GitHub built on Linux.

## Notes

- GitHub builds with the latest Go release, which receives security fixes.
- Do not compress the exe with UPX or any other packer: antivirus software often
  flags packed programs.
- `data/LICENSE-emudscan.txt` must stay identical to `LICENSE`; a test checks
  this.
- On Linux, `go run ./cmd/emudscan <folder>` runs the checks of the **Other
  software** list on a folder.
