# Building and releasing

## Project layout

| Path | Contents |
|---|---|
| `cmd/emudscan/` | The program (Go, one package), its tests, and `rsrc_windows_amd64.syso` (icon, manifest, version information) |
| `data/` | The reference data from emudhra.we-have-a-problem.com, with its notice and license, Go’s license and a copy of `LICENSE`, all embedded in the program, except the two chains of `chains/rollover`, which only the tests use |
| `docs/` | This documentation; `docs/images/` holds the screenshots and icons shown here and in the README (the icons are drawn by eMudScan’s own code, `cmd/emudscan/icons.go`) |
| `scripts/` | Build scripts for your own PC, and the scripts that publish a version (`release.sh`) and copy a hotfix into `main` (`hotfix-to-main.sh`) |
| `.github/` | The GitHub build (`workflows/build.yml`), the release page template (`release-notes.md`), the Dependabot settings (`dependabot.yml`), the security policy (`SECURITY.md`), the issue forms (`ISSUE_TEMPLATE/`), the pull request template and the contributing guide |

## GitHub builds the program

Each push to `main` and each pull request runs the checks and the tests, then
builds `eMudScan.exe`. The exe can be downloaded from the run’s page on the
**Actions** tab, under **Artifacts** (a zip file).

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
   in its Security, Fixed, Changed… subsections. They are copied to the release
   page, where GitHub keeps line breaks: write each paragraph and each item on a
   single line.
2. In Git Bash, at the root of eMudScan, on an up-to-date `main` (`git pull`),
   run `sh scripts/release.sh 3.1.0` with the new version (three numbers, as
   explained at the top of [CHANGELOG.md](../CHANGELOG.md)).

The script first checks that the version follows the current one (in a
repository without any version tag yet, that it is the one `appVersion`
already gives, such as 3.0.1, and that the `[unreleased]` link at the end of
`CHANGELOG.md` gives the commits of `main`), that nothing is left uncommitted,
that `main` is up to date with GitHub, and that the changelog has changes to
publish. It shows the commit the version is based on,
the commits it brings (and those not pushed yet, which it would publish too),
and the changes, then asks for your confirmation. Only then does it set
`appVersion` and `appDate` (today) in `cmd/emudscan/model.go`, turn
`## [Unreleased]` into `## [3.1.0] - <date>` with its links, run the checks and
the tests (putting the files back if they fail), commit
`chore(release): 3.1.0`, push it, and create and push the tag `3.1.0`.

GitHub then builds the exe and checks that the tag matches `appVersion` and that
`CHANGELOG.md` has a section for this version. It creates a **draft release**
with `eMudScan.exe` and a page made from `.github/release-notes.md`: the text
around `@CHANGES@` is the same for every version, `@CHANGES@` is replaced by the
changes since the previous version published in this repository (its highest
version tag below this one): the version’s section of the changelog, then, each
under its own heading, the sections of the versions in between that have no
release here. Without such a tag, the previous version is the one this
repository starts from, the `appVersion` of its first commit. The SHA-256, the
build log and the date of the release are filled in too. It also records a build
attestation proving that GitHub built the exe from this tag. Anyone can verify a
copy with `gh attestation verify eMudScan.exe -R <owner>/<repository>`.

Download `eMudScan.exe` from the draft and scan it with
[Jotti’s malware scan](https://virusscan.jotti.org/en-US). In the draft,
complete the **Tested clean** line under the first paragraph, so that the
release page shows that the exe was tested clean: replace `JOTTI_SCAN_ID` (a
double-click selects it) with the ID at the end of the address of the results,
and the number of antivirus engines if Jotti used another one. Its date is the
date of the release: change it if you scan on another day.

Antivirus engines update their detections all the time: scan the exe again a day
or more after the release, then edit the notes of the published release with the
new date and link. GitHub allows editing the notes of a published release, even
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

A hotfix brings only an urgent fix to the latest version, such as 3.1.1 after
3.1.0; an older version gets none.

- When the commits on `main` since that version can be published with the fix,
  such as documentation changes, commit the fix on `main` and publish it as
  above: `sh scripts/release.sh 3.1.1`.
- When `main` holds changes for the next version, publish them with the fix as
  the next minor version (3.2.0) instead, if they are ready.

Otherwise, publish the hotfix from a branch:

1. Put your work in progress aside: `git stash` the changes not committed, and
   move the commits not pushed yet to a branch (`git branch wip`, then
   `git reset --hard origin/main`).
2. Create a branch from the version’s tag: `git switch -c hotfix-3.1.1 3.1.0`.
3. Commit the fix there, with its changelog entry, then run
   `sh scripts/release.sh 3.1.1`: on this branch, it pushes only the tag.
4. Copy the hotfix into `main`: `git switch main`, then
   `sh scripts/hotfix-to-main.sh 3.1.1`. It copies the commits of the fix,
   without merge commits, and leaves the unreleased changes in `CHANGELOG.md` as
   they are. Then it adds the 3.1.1 section and its links to `CHANGELOG.md`, and
   its version to `model.go`, as in the tag, in a commit
   `chore(release): add 3.1.1 to main`. If a commit of the fix conflicts with
   `main`, it stops and says how to go on.
5. Finish in `main`:
   - `git push`: sends to GitHub what `hotfix-to-main.sh` added to your `main`,
     the fix and `chore(release): add 3.1.1 to main`, so that the next version
     includes the fix. No tag to create: `release.sh` already created and pushed
     `3.1.1`, on the hotfix branch.
   - `git branch -D hotfix-3.1.1`: deletes the branch, which is no longer
     needed. `-D` because Git sees that its commits are not in `main` (only
     copies of them are) and would refuse `-d`. Nothing is lost: the tag `3.1.1`
     keeps them.
   - If you moved commits to `wip` at the start, put them back on `main`, after
     the fix:
     - `git switch wip`, then `git rebase main`: replays your commits on top of
       the fix, as if you had started them after it. Where `CHANGELOG.md`
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

Both run the checks and the tests, then write `dist/eMudScan.exe`.

To test the Fix function without changing anything, `.\scripts\build.ps1 -Simulate`
(or `./scripts/build.sh --simulate`) writes `dist/eMudScan-simulation.exe`
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

`scripts/build.sh` and the GitHub build also regenerate the Windows resources
(`rsrc_windows_amd64.syso`), so that the file’s properties show the version set
in `model.go`. `scripts/build.ps1` uses the committed file.

## Notes

- GitHub builds with the latest Go release, which receives security fixes. Each
  Go release makes programs slightly larger: about 5.4 MB with Go 1.24, 6.1 MB
  with Go 1.27.
- Do not compress the exe with UPX or any other packer: antivirus software often
  flags packed programs.
- `data/LICENSE-emudscan.txt` must stay identical to `LICENSE`; a test checks
  this.
- On Linux, `go run ./cmd/emudscan <folder>` runs the checks of the **Other
  software** list on a folder.
