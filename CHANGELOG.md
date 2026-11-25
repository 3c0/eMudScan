# Changelog

All notable changes to eMudScan are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). eMudScan does not follow Semantic Versioning. Its versions are numbered MAJOR.MINOR.HOTFIX: a major version is a milestone; a minor version brings new checks, fixes and improvements; a hotfix version brings small or urgent fixes to the latest version.

## [Unreleased]

## [4.0.0] - 2026-11-25

### Added

- Self-update: eMudScan asks GitHub for its latest release at the first scan of a session and from **Check for updates** in the About box, with **Online checks and updates** checked.
- The update dialog shows the release notes of up to 10 new versions, then downloads `eMudScan.exe` with a progress bar: **Update now**, **Not now**, **Cancel**, **Try again**.
- An update is installed only from an immutable release whose `eMudScan.exe` has the SHA-256 that the GitHub API and the release notes give.
- A copy of eMudScan, in a folder of its own in Temp, replaces `eMudScan.exe` in one step and starts it; it asks for administrator rights once when the folder of the exe needs them.
- When an update fails, the old version starts again, says why and gives the release page. `eMudScan-cli.exe` never updates itself.
- Fix window: `check_ssl.py`, in the fix of a Python, is a link that shows this file with syntax highlighting and a copy button. The file is in the source as it is (`cmd/emudscan/check_ssl.py`).
- The fix of a Python says what `$emudscanFiles` is, the folder that eMudScan makes for it in Temp, and shows it when it runs.
- **Copy all** button in the box of the licenses and notices.
- The Fix window and the report name the PowerShell that runs the fixes.
- On a PC without Windows PowerShell, the fixes run in PowerShell 7 or later from Program Files, never in a `pwsh.exe` of the PATH; with neither, the fixes are disabled.
- A command line too long for its box wraps with ↩ and ↪, at the indent of the line, in the detailed information and the console of the Fix window; the signs are not copied.

### Changed

- Every fix that changes files (certificate bundle, Java block list, node.exe, ssl files of a Python, restore of backups) is a commented script, one command per line, that stops at its first error.
- A certificate bundle is downloaded with Windows PowerShell set to TLS 1.2, instead of TLS 1.2 added to its protocols.
- Fix console: a rule separates two software; each starts with its name and `# Location:`, and each of its fixes with `# Fix:`, or `# Fix 1:` and `# Fix 2, if the update does not fix it:`.
- Detailed information: the sentence above the commands of a fix says to click **Fix this software** or to run them in PowerShell, and replaces “In PowerShell:”.
- Above a script, that sentence names the PowerShell versions that the tests run it in (5.1, 7.6, 7.7), and ends with **(Run as administrator)**, in bold red, when the commands need it.
- The fix of a Python: its errors say what happened, what it means and what to do, and `catch` puts the original files back when the replacement or a test after it fails.
- The fix of a Python: each command that ran has its result (✔ or ✘) in the Fix window and the report; the warning “No file was changed” is gone.
- The detailed information of a Python inside a program gives a script that replaces its ssl files without eMudScan; the Fix window runs the version with eMudScan’s tests.
- The fixes of a Python and of a node.exe download the latest release of their line, which eMudScan looks for when a Fix window opens (python.org, python-build-standalone, nodejs.org).
- Without a release found, or with **Online checks and updates** cleared, these fixes use the pinned releases: python-build-standalone 20261124, Node.js 22.23.5, 24.22.3 and 26.13.1.
- The fix of a node.exe checks the SHA-256, the OpenJS Foundation signature and the version of its download before it changes anything; the detailed information gives a script that runs alone.
- A 32-bit node.exe of Node.js 24 or later gets no fix: nodejs.org builds none.
- The reports no longer contain the commands of the fixes. In the JSON report, `fixes` has no `commands`, and `format_version` is 2.
- The Fixes section of the reports (`fixes_run` in JSON) gives the result of each software and, for a command that failed, the command and its output.
- A conda environment whose conda is not found gets the same recommended action as the others.
- “Allow the online test” is renamed “Online checks and updates”: it covers every connection of eMudScan itself.
- Wider windows at 100 % scaling: the detailed information is 1000 pixels (was 850), the Fix window of one software 900 (was 860), the About box 720 (was 525).
- Detailed information and About box: justified text, tighter lines, square bullets. The detailed information scrolls smoothly; the text of the About box can be selected and copied.
- About box: the User-Agent is on a line of its own, in a smaller font, and the privacy text covers every site that eMudScan connects to, not only the test site.
- About box and licenses: python-build-standalone is named (Mozilla Public License 2.0, no file included); the disclaimer names the publishers of the files that the fixes download.
- The About box and the README say that eMudScan was made by 3c0 with the help of AI.
- The mouse wheel scrolls the lists one row a notch, instead of three.
- **Copy report** says **Copied** for 2 seconds, instead of a message box.
- The search box gets the focus when a scan starts, not when it ends.
- Wait cursor during a scan, **Scan again** and the lookup of a new version, except over **Stop**; the controls are greyed out while a Fix window looks for the latest releases.
- The web search for a certificate bundle of an unknown software searches for the software alone, without the name of the file.
- `scripts/build.ps1` shows the test that runs and those of 1 second or more. The PowerShell tests look for the PowerShells once and run in parallel.

### Fixed

- Ctrl+C and Ctrl+A in the log, the Fix console and the detailed information no longer beep: the dialog manager took them for the mnemonic of a button.
- Started from PowerShell 7, eMudScan gave its `PSModulePath` to Windows PowerShell, which no longer found `Get-FileHash`: the fixes put the modules of their own PowerShell first.
- The check of eMudScan’s own files before a fix no longer uses a module command, and stops the fixes when it cannot be made.
- Fix console: the rule at the end of the fixes takes one row at any width of the window.
- A word too long for its line, such as a URL or a path, is cut after a slash, a backslash or a hyphen.
- Fix software window: a click on a copy of a software scrolls the console to its commands.
- `scripts/build.ps1` works in Windows PowerShell 5.1: it no longer gives go an empty `-tags` option.
- A greyed-out button shows its tooltip, which says when it can be used.
- Main window: the same room after each drive of **Drives to scan:**, and on both sides of the separator before them.
- Tab follows the reading order and reaches every control, the ⓘ button and the consoles included; inside a text, it goes through the links and the copy buttons.
- A focus rectangle shows where the keyboard focus is, and the focus is back where it was when a box or a Fix window closes.
- Display scale (DPI) change: the windows open above the main one follow it, fonts and layout, and every window gets back the size it had at a scale.

## [3.1.2] - 2026-11-12

### Changed

- The detailed information of “Software copying the Windows root store” gives the result on this PC first, and no longer reads as a check of Python only.

### Fixed

- The detailed information about the Windows roots says only what is found on this PC; the outdated advice not to delete the roots is gone.

## [3.1.1] - 2026-11-11

### Added

- A certificate bundle that is a byte-for-byte copy of the root store built into a program nearby is flagged in the detailed information of both: the program can write it again.
- When a report is saved, a note at the end of the log links to its file in File Explorer.

### Changed

- Log: the line of a certificate bundle gives the file instead of its folder, and the line of a program says “in” before its folder.
- The name of a saved report has the name of the PC: `eMudScan-report-MY-PC-20261201-101548.txt`.

## [3.1.0] - 2026-11-11

### Added

- `eMudScan-cli.exe`, a console program that scans from the command line: `--report FILE` (text, JSON or CSV, by its extension), `--drives`, `--online`, `--lang`. It never runs a fix.
- Exit codes of `eMudScan-cli.exe`: 0 OK or information, 1 warning, 2 update needed, 3 critical, 10 the scan could not run.
- **Save report** can save a JSON or a CSV file, with field names in English; `docs/using.md` describes the fields.

### Changed

- `--lang en` and `--lang fr` replace `--en` and `--fr`.
- Log: the note “No drive selected…” comes below the first line, instead of above it.
- The name of a saved report has the seconds: `eMudScan-report-20261201-101548.txt`.
- The About box and the README say that eMudScan is a technical tool for advanced users: security researchers, administrators and developers.

### Fixed

- The version of a program is no longer taken for that of its Python or Node.js, when it starts them through a `python.exe` or `node.exe` of its own: such a copy was shown as OK.
- The exe built by GitHub is no longer marked “+dirty”: the Windows resources come out the same with any Go version.

## [3.0.1] - 2026-11-10

### Changed

- Main window: a border released over a row shows that row whole if at least half of it was shown, and hides it otherwise.

### Fixed

- The list of the Fix software window shows whole rows, as the lists of the main window do.

## 3.0.0 - 2026-11-10

Changes since 2.0.0, where this repository starts.

### Added

- Checks jetbrainsd, the background service of the JetBrains Toolbox App and IDEs, which has its own list of trusted certificates: version 0.9.9728 or later is fixed.
- Checks the JetBrains IDEs and Gateway by their version, for their built-in browser, and names the Java runtime of an IDE or of the Toolbox App after it.
- Updates an outdated Python 3.14 or later with the Python Install Manager, when it is installed.
- Upgrades an outdated Python of uv with uv, after updating uv as it was installed, then removes the old copy and names the virtual environments that used it.
- Optional fix: the node.exe inside a program is replaced with the latest release of its Node.js line from nodejs.org, once checked (SHA-256, OpenJS Foundation signature, version).
- Optional fix: the ssl files of a 64-bit Python 3.11 to 3.15 inside a program are replaced with those of the fixed release of its line (python.org or python-build-standalone), checked by SHA-256.
- Before and after that fix, the Python runs eMudScan’s tests (ssl, hashlib, Windows roots, TLS 1.2 and 1.3, a compromised chain); the original files are put back if one fails.
- **Restore the backup** undoes the fix of a Python as a whole, even after a copy that a crash interrupted.
- Updates a Deno installed with its own installer with `deno upgrade` (Deno 2.9.8 is fixed).
- For a Pipenv, Poetry or uv project whose lock file pins the outdated certifi, the detailed information gives the command that updates it; eMudScan never runs it.
- Fix software window: a double-click opens the detailed information of a software, the columns sort, and the list has the right-click menu of the main window and Ctrl+C.
- **Fix**, in the right-click menu of the lists, opens the Fix software window for the selected items.
- After the fixes, each result in the Fix console links to the detailed information of its software.
- Syntax highlighting of the PowerShell commands, in the colors of PowerShell 7.
- The top of the report names the PC.

### Changed

- Shorter detailed information for Python: one sentence for the problem, then the recommended action, which names the program a bundled Python belongs to.
- A finding in several places shows its explanation once, with the commands for each place.
- “Back up the certificate files” is now “Back up the files to change”: it also covers node.exe and the ssl files of a Python.
- The detailed information no longer suggests updating everything with a package manager (`scoop update -a`, `winget upgrade --all`, `choco upgrade all`).
- The Fix window opened from the detailed information has no **Stop** button: a fix that has started to change files runs to its end.
- The About box and the README say that eMudScan is intended for administrators, developers and advanced users.
- The About box shows the link to the source code beside its title, and the sites that the fixes connect to as a list.
- The texts name we-have-a-problem.com, a third-party site, instead of “the advisory”.
- The warning before the fixes explains their order when it differs from the one shown. Ticking a software scrolls the console only as much as needed.
- The fixes stop at the first download that fails, and after an update that fails because the servers of its package manager cannot be reached.
- Fix console: each command gets a green ✔ or a red ✘, its warnings in yellow, its errors in red, and its exit code when it failed; the report has ✓ or ✗ before each command.
- The Fix console ends with a horizontal rule instead of “End of the fixes.”; in the report, a line of em dashes also separates the sections.
- The Fix window says “PowerShell commands that eMudScan will run”, and each check after the fixes says which stage finished.
- The details window is renamed “Detailed information” everywhere.
- Right-click menu with several items: each entry counts what it acts on (“Open 2 software folders”); **Scan again** no longer asks first.
- **Search on Google** names what it searches for, such as **Search on Google: “cacert.pem”**: the first three searches, then how many more.
- The detailed information of several items opens in one window, with a block to go from one to another (Alt+Home, Alt+←, Alt+→, Alt+End).
- Fix console: the path of the report saved after the fixes is a link that shows it in File Explorer.
- A software still not fixed after its fixes is unticked; an update that installed no newer version says so: “still not fixed (no newer version installed)”.
- A certificate bundle made by curl shows the date of its certificate data. A bundle that belongs to no software has no software date: it was the modification date of the file.

### Fixed

- Web search for a certificate bundle: the software with the name of the file, or the name of the file alone, instead of the folder of the file (“Downloads cacert.pem”).
- Several homepages or searches open in one start of the default browser, as tabs: Firefox asked for a profile once per page, and a page could get lost.
- “Back up the files to change” is hidden when no fix to run backs up a file, such as an update with pip.
- No pip command where it cannot work: where pip is not installed, or for a Python marked EXTERNALLY-MANAGED, as uv does.
- No Scoop update command for a file in Scoop’s `persist` folder, which its updates keep as it is.
- The packages of a tool that uv installed are updated with `uv tool upgrade`, instead of pip.
- The packages of an application that pipx installed are updated with `pipx runpip`.
- A file put back by **Restore the backup** has its original creation date again.
- The summary of the certificate bundles is updated after fixes and restores.
- The ANSI color codes in the output of commands, such as git under Scoop, are shown as colors in the Fix console and left out of the report.

## 2.0.0 - 2026-11-05

First public version.

### Added

- Checks that Windows blocks the 16 compromised certificates (8 eMudhra intermediates and their copies cross-signed by Sectigo), offline and with an optional online test.
- Detects an antivirus or a proxy that inspects secure connections.
- Checks browsers and internet apps by the block lists they receive or carry, including Microsoft Edge and WebView2.
- Checks other software: certificate bundles (certifi, pip, Git, curl…), Python, Node.js, Java, Go, Electron applications, PyInstaller programs and built-in root stores.
- Gives the recommended action for each item, with its commands, and flags software that needs an update and is a year old or more.
- Fixes, once confirmed: package manager updates (Scoop, WinGet, Chocolatey, pip, conda), Mozilla’s certificate bundle, Java block lists. The files are backed up and can be restored.
- After the fixes, each software is checked again, and a report is saved in the Downloads folder.
- A report to copy or save. English and French.
- Changes nothing on the PC until a fix is confirmed. Without the online test, a scan makes no internet connection.

[unreleased]: https://github.com/3c0/eMudScan/compare/4.0.0...HEAD
[4.0.0]: https://github.com/3c0/eMudScan/compare/3.1.2...4.0.0
[3.1.2]: https://github.com/3c0/eMudScan/compare/3.1.1...3.1.2
[3.1.1]: https://github.com/3c0/eMudScan/compare/3.1.0...3.1.1
[3.1.0]: https://github.com/3c0/eMudScan/compare/3.0.1...3.1.0
[3.0.1]: https://github.com/3c0/eMudScan/releases/tag/3.0.1
