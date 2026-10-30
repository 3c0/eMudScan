# Building and releasing

## Project layout

| Path | Contents |
|---|---|
| `cmd/emudscan/` | The program (Go, one package), its tests, and `rsrc_windows_amd64.syso` (icon, manifest, version information) |
| `data/` | The reference data from emudhra.we-have-a-problem.com, with its notice and license, Go's license and a copy of `LICENSE`: all embedded into the program |
| `docs/` | This documentation, and in `docs/images/` the screenshots and icons it and the README show (the icons are drawn by eMudScan's own code, `cmd/emudscan/icons.go`) |
| `scripts/` | Build scripts for your own PC |
| `.github/` | The GitHub build (`workflows/build.yml`), the release page template (`release-notes.md`), Dependabot's settings (`dependabot.yml`), the security policy (`SECURITY.md`), the issue forms (`ISSUE_TEMPLATE/`), the pull request template and the contributing guide |

## GitHub builds the program

Each push to `main` and each pull request runs the checks and the tests, then
builds `eMudScan.exe`. The exe can be downloaded from the run's page on the
*Actions* tab, under *Artifacts* (a zip file).

## Publishing a version

1. In `cmd/emudscan/model.go`, set `appVersion` and `appDate` (YYYYMMDD).
2. In `CHANGELOG.md`, rename the `## Unreleased` section, where changes are noted
   as they are made, to `## <version> (<date>)`. It is copied to the release
   page, where GitHub keeps line breaks: write each paragraph and each item on a
   single line.
3. Commit these changes as `chore(release): <version>`, push them, then push a
   tag named after the version, such as `1.1`.

GitHub then builds the exe and checks that the tag matches `appVersion` and that
`CHANGELOG.md` has a section for this version. It creates a **draft release**
with `eMudScan.exe` and a page made from `.github/release-notes.md`: the text
around `@CHANGES@` is the same for every version, `@CHANGES@` is replaced by the
changelog section, and the SHA-256 and build log are filled in. It also records a
build attestation proving that the exe was built by GitHub from this tag. Anyone can
check a copy with `gh attestation verify eMudScan.exe -R <owner>/<repository>`.

Check the draft, then click *Publish release*. GitHub adds the source code to
the release, as the GPL requires.

Version numbers are explained at the top of [CHANGELOG.md](../CHANGELOG.md).

## Building on your PC

You need Go 1.24 or later, and nothing else. From the root of the repository:

- Windows (PowerShell): `.\scripts\build.ps1`
- Linux or macOS: `./scripts/build.sh`

Both run the checks and the tests, then write `dist/eMudScan.exe`.

`scripts/build.sh` and the GitHub build also regenerate the Windows resources
(`rsrc_windows_amd64.syso`), so that the file's properties show the version set
in `model.go`. `scripts/build.ps1` uses the committed file.

## Notes

- GitHub builds with the latest Go release, which receives security fixes. Each
  Go release makes programs a little larger: about 5.4 MB with Go 1.24, 6.1 MB
  with Go 1.27.
- Do not compress the exe with UPX or another packer: antivirus software often
  flags packed programs.
- `data/LICENSE-emudscan.txt` must stay identical to `LICENSE`; a test checks it.
- On Linux, `go run ./cmd/emudscan <folder>` runs the third-party checks against
  a folder.
