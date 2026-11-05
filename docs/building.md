# Building and releasing

## Project layout

| Path | Contents |
|---|---|
| `cmd/emudscan/` | The program (Go, one package), its tests, and `rsrc_windows_amd64.syso` (icon, manifest, version information) |
| `data/` | The reference data from emudhra.we-have-a-problem.com, with its notice and license, Go’s license and a copy of `LICENSE`, all embedded in the program |
| `docs/` | This documentation; `docs/images/` holds the screenshots and icons shown here and in the README (the icons are drawn by eMudScan’s own code, `cmd/emudscan/icons.go`) |
| `scripts/` | Build scripts for your own PC |
| `.github/` | The GitHub build (`workflows/build.yml`), the release page template (`release-notes.md`), the Dependabot settings (`dependabot.yml`), the security policy (`SECURITY.md`), the issue forms (`ISSUE_TEMPLATE/`), the pull request template and the contributing guide |

## GitHub builds the program

Each push to `main` and each pull request runs the checks and the tests, then
builds `eMudScan.exe`. The exe can be downloaded from the run’s page on the
**Actions** tab, under **Artifacts** (a zip file).

## Publishing a version

1. In `cmd/emudscan/model.go`, set `appVersion` (three numbers from 2.0.0, such
   as `2.1.0`) and `appDate` (YYYYMMDD).
2. In `CHANGELOG.md`, rename the `## [Unreleased]` section, where changes are
   recorded as they are made, to `## [<version>] - <YYYY-MM-DD>`, with the date
   of `appDate`, and add an empty `## [Unreleased]` section above it. At the end
   of the file, point the `[unreleased]` link to `<version>...HEAD` and add a
   link for the version, to `<previous version>...<version>`. The version’s
   section, with its Security, Fixed, Changed… subsections, is copied to the
   release page, where GitHub keeps line breaks: write each paragraph and each
   item on a single line.
3. Commit these changes as `chore(release): <version>`, push them, then push a
   tag named after the version, such as `2.1.0`.

GitHub then builds the exe and checks that the tag matches `appVersion` and that
`CHANGELOG.md` has a section for this version. It creates a **draft release**
with `eMudScan.exe` and a page made from `.github/release-notes.md`: the text
around `@CHANGES@` is the same for every version, `@CHANGES@` is replaced by the
changelog section, and the SHA-256 and the build log are filled in. It also
records a build attestation proving that GitHub built the exe from this tag.
Anyone can verify a copy with
`gh attestation verify eMudScan.exe -R <owner>/<repository>`.

Download `eMudScan.exe` from the draft and scan it with
[Jotti’s malware scan](https://virusscan.jotti.org/en-US). In the draft, replace
the comment under the first paragraph with the line that it gives, completed
with the number of antivirus engines and the link to the results, so that the
release page shows that the exe was tested clean on the release date.

Check the draft, then click **Publish release**. GitHub adds the source code to
the release, as the GPL requires. Publish only a draft that is right: releases
are immutable in this repository, so the tag of a published release can never be
reused, even after the release is deleted.

Version numbers are explained at the top of [CHANGELOG.md](../CHANGELOG.md).

## Publishing a hotfix

A hotfix brings only an urgent fix to the latest version, such as 2.1.1 after
2.1.0; an older version gets none. When `main` holds no other change since that
version, publish it from `main` as above. Otherwise:

1. Create a branch from the version’s tag: `git switch -c hotfix-2.1.1 2.1.0`.
2. Commit the fix there, with its changelog entry, then publish the version from
   this branch as above: commit `chore(release): 2.1.1`, then push the tag.
3. Merge the branch into `main` (`git switch main`, then
   `git merge hotfix-2.1.1`), keeping the unreleased changes of `main` where
   `CHANGELOG.md` conflicts, then delete the branch (`git branch -d hotfix-2.1.1`).

A build that is not of a release, that is of a commit other than the one tagged
with `appVersion`, or with changes not yet committed, shows “+dev” and its commit
instead of the date: “2.0.0+dev (ae7832d)”, or “2.0.0+dev (ae7832d-modified)”. It
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
but runs nothing in place of each command, which then shows “✓ Done
(simulated)”, after a second for each software, as its commands would take some
time; a software whose commands all succeeded is then shown as fixed. A command
that contains the text of the environment variable `EMUDSCAN_SIMULATE_FAIL` shows
“✗ Failed (simulated)” instead. Its window title and
its report say SIMULATION. The normal build contains none of this: the
simulation is compiled only with the `simulate` build tag, which the GitHub build
never sets.

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
