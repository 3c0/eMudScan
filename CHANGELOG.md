# Changelog

All notable changes to eMudScan are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). eMudScan does not follow Semantic Versioning. Its versions are numbered MAJOR.MINOR.HOTFIX: a major version is a milestone; a minor version brings new checks, fixes and improvements; a hotfix version brings small or urgent fixes to the latest version.

## [Unreleased]

### Added

- Add self-update. eMudScan asks the GitHub API for its latest release at the first scan of a session and from **Check for updates** in the About box, and shows the release notes of up to 10 new versions. It installs only an immutable release whose `eMudScan.exe` has the SHA-256 that the API and the release notes give. A copy of itself, run from a folder in Temp that only the user, SYSTEM and the administrators can open, writes the new exe under a random name beside the old one, checks it, and renames it over `eMudScan.exe` in one step, with one UAC prompt when the folder needs it. `eMudScan-cli.exe` never updates itself.
- Add a viewer for `check_ssl.py`, the tests that the fix of a Python runs in that Python: its name, in the commands of the Fix window, is a link that shows the file with syntax highlighting and a copy button. The file is embedded as it is from `cmd/emudscan/check_ssl.py`, and its docstrings say what each test checks. The commands say what `$emudscanFiles` is: the folder of the fix in Temp. Add a **Copy all** button to the box of the licenses.
- Name the PowerShell that runs the fixes in the Fix window and in the report. Without Windows PowerShell 5.1, run the fixes in PowerShell 7 or later from Program Files, never in a `pwsh.exe` found in the PATH, since some fixes run as administrator; with neither, disable the fixes.
- Wrap a command line too long for its box with ↩ and ↪, at the indent of the line, in the detailed information and the Fix console. The signs are not selected or copied.

### Changed

- Rewrite the fixes of a certificate bundle and of a Java block list, and the restore of backups, as scripts made to be read: comments, one command per line, a stop at the first error. The detailed information gives each as one block that also runs when pasted into a PowerShell window. Set Windows PowerShell to TLS 1.2 for the download of a bundle, instead of adding TLS 1.2 to its protocols.
- Separate two software with a rule in the Fix console. Start each software with its name and `# Location:`, and each fix with `# Fix:`, or `# Fix 1:` and `# Fix 2, if the update does not fix it:`, instead of “Then, if the update does not fix it:”.
- Replace “In PowerShell:”, above the commands of a fix in the detailed information, with a sentence that says what to do: click **Fix this software**, or run the script in PowerShell 5.1, 7.6 or 7.7, the versions that the tests run on GitHub. End it with **(Run as administrator)**, in bold red, when the commands need it.
- Say what happened, what it means and what to do in the errors of the scripts that replace the ssl files of a Python.
- **Breaking:** remove the commands of the fixes from the reports, which keep the explanations. In the JSON report, `fixes` loses `commands`, and `format_version` becomes 2.
- Report what came of the fixes in the Fixes section of the reports (`fixes_run` in JSON): the result of each software and, for a command that failed, the command and its output, instead of every command.
- Give, in the detailed information of a Python inside a program, a standalone script that replaces its ssl files without eMudScan: one block that stops at its first error, tests before and after that this Python loads its `ssl` module, and downloads into a folder of its own in Temp. The Fix window runs the version with eMudScan’s tests.
- Rewrite the fix of the ssl files of a Python as a script made to be read: comments, one command per line, a stop at the first error, and a `try` block whose `catch` puts the original files back when the replacement or a test after it fails. Show the result of each command that ran (✔ or ✘) in the Fix window and the report, instead of the warning “No file was changed”.
- Download the latest release of its line in the fix of a Python, instead of a pinned one. eMudScan looks for it when a Fix window opens: on GitHub for python-build-standalone, whose releases are immutable, and on python.org, where it takes the latest release of the line that has Windows files. The commands keep one URL and its SHA-256. Without a release found, or with **Online checks and updates** cleared, the fix uses the pinned release, now 20261124 for python-build-standalone.
- Rewrite the fix of a node.exe as a script made to be read. It downloads the latest release of its line, which eMudScan looks for on nodejs.org when a Fix window opens (pinned: Node.js 22.23.5, 24.22.3 and 26.13.1), and checks its SHA-256, its OpenJS Foundation signature and its version before it changes anything. The detailed information gives a standalone version. A 32-bit node.exe of Node.js 24 or later gets no fix: nodejs.org builds none.
- Rename “Allow the online test” to “Online checks and updates”: the box covers every connection of eMudScan itself, such as the lookup of the latest releases.
- Open the detailed information 1000 pixels wide at 100 % scaling, instead of 850, so that most commands fit on one line, and the Fix window of one software 900 pixels wide, instead of 860.
- Justify the text of the detailed information and of the About box, with tighter lines and square bullets. Scroll the detailed information smoothly. Let the text of the About box be selected and copied.
- Widen the About box from 525 to 720 pixels.
- Show the User-Agent on a line of its own, in a smaller font, in the About box, and say that every site eMudScan connects to sees it, with the IP address: the text named the test site only.
- Scroll the lists one row per notch of the mouse wheel, instead of three.
- Show **Copied** on **Copy report** for 2 seconds, instead of a message box.
- Focus the search box when a scan starts, not when it ends.
- Show the wait cursor during a scan, **Scan again** and the lookup of a new version, except over **Stop**.
- Search the web for the software alone, for a certificate bundle of an unknown software, instead of the software with the name of the file.
- Say in the About box and the README that eMudScan was made by 3c0 with the help of AI.
- Name python-build-standalone in the About box and in the licenses (Mozilla Public License 2.0, no file included), and the publishers of the files that the fixes download in the disclaimer.
- Show the running test, and those of 1 second or more, in `scripts/build.ps1`. Look for the PowerShells once in the PowerShell tests, and run their cases in parallel.

### Fixed

- Fix the beep of Ctrl+C and Ctrl+A in the log, the Fix console and the detailed information: the dialog manager took them for the mnemonic of a button.
- Fix the fixes that check a SHA-256 when eMudScan is started from PowerShell 7: Windows PowerShell got the `PSModulePath` of PowerShell 7 and no longer found `Get-FileHash`. The fixes put the modules of their own PowerShell first, and the check of eMudScan’s own files no longer uses a module command.
- Fix the empty rows under the final rule of the Fix console in a narrow window.
- Break a word too long for its line, such as a URL or a path, after a slash, a backslash or a hyphen, instead of in the middle of a name.
- Fix the scroll of the Fix console on a click on a copy of a software, such as one of two Pythons inside the same program.
- Fix `scripts/build.ps1` in Windows PowerShell 5.1 (“no Go files”): it gave go an empty `-tags` option.
- Show the tooltip of a greyed-out button, which says when it can be used: a disabled button gets no mouse messages.
- Fix the room after each drive of **Drives to scan:**, which was wider than after the label.
- Center the separator between **Online checks and updates** and the drives, measured to the ink of the letters.
- Fix the Tab order: it follows the reading order and reaches every control, the ⓘ button and the Fix consoles included. Inside a text (log, console, detailed information, About box, release notes), Tab goes through the links and the copy buttons, and Enter or Space activates them. Draw a dotted focus frame where none showed, and give the focus back to its control when a box or a Fix window closes.
- Follow a change of the display scale (`WM_DPICHANGED`) in the windows open above the main one: About box, licenses, saved report, update, detailed information and Fix windows. They kept the fonts of the old scale in a window that Windows resized. Each window, the main one included, gets back the size it had at a scale.

## [3.1.2] - 2026-11-12

### Changed

- Give the result on this PC first in the detailed information of “Software copying the Windows root store”, which no longer reads as a check of Python only.

### Fixed

- Remove the outdated advice not to delete the roots from the detailed information about the Windows roots, which now says only what is found on this PC.

## [3.1.1] - 2026-11-11

### Added

- Flag a certificate bundle that is a byte-for-byte copy of the root store built into a program of its folder, or of the folder above, in the detailed information of both: the program can write the file again.
- Add a note at the end of the log when a report is saved, with a link that shows the file in File Explorer.

### Changed

- Give the file of a certificate bundle, instead of its folder, in the log, and say “in” before the folder of a program.
- Add the name of the PC to the name of a saved report: `eMudScan-report-MY-PC-20261201-101548.txt`.

## [3.1.0] - 2026-11-11

### Added

- Add `eMudScan-cli.exe`, a console program that scans from the command line, for scripts and deployment tools. `--report FILE` writes the report as text, JSON or CSV, by the extension of the file, `--drives C:,D:` chooses the drives, and `--online` allows the online test. The log goes to the standard error output, and the exit code gives the worst status: 0 OK or information, 1 warning, 2 update needed, 3 critical, 10 the scan could not run. It never runs a fix.
- Add JSON and CSV reports to **Save report**: every item with all its fields in JSON, one row per item in CSV, with field names in English. `docs/using.md` describes the fields.

### Changed

- **Breaking:** replace `--en` and `--fr` with `--lang en` and `--lang fr`.
- Move the note “No drive selected…” below the first line of the log.
- Add the seconds to the name of a saved report: `eMudScan-report-20261201-101548.txt`.
- Say in the About box and the README that eMudScan is a technical tool for advanced users: security researchers, administrators and developers.

### Fixed

- Fix a Python or a Node.js shown as OK when its program starts it through a `python.exe` or `node.exe` of its own, which carries the version of the program: read the version of Python from Python’s own files.
- Fix the “+dirty” mark in the build information of the exe built by GitHub: generate the same Windows resources with any Go version.

## [3.0.1] - 2026-11-10

### Changed

- Show whole the row over which a border of the main window is released, when at least half of it was shown, and hide it otherwise, instead of always hiding it.

### Fixed

- Show whole rows in the list of the Fix software window, as in the lists of the main window.

## 3.0.0 - 2026-11-10

Changes since 2.0.0, where this repository starts.

### Added

- Check jetbrainsd, the background service of the JetBrains Toolbox App and IDEs, which has its own list of trusted certificates: version 0.9.9728 or later is fixed.
- Check the JetBrains IDEs and Gateway by their version, for their built-in browser, and name the Java runtime of an IDE or of the Toolbox App after it.
- Update an outdated Python 3.14 or later with the Python Install Manager, when it is installed.
- Upgrade an outdated Python of uv with uv, after updating uv as it was installed (Scoop, WinGet, Chocolatey, its installer or pipx), then remove the old copy and name the virtual environments that used it.
- Add an optional fix that replaces the node.exe inside a program with the latest release of its Node.js line from nodejs.org, once checked: SHA-256, OpenJS Foundation signature and version.
- Add an optional fix that replaces the ssl files of a 64-bit Python 3.11 to 3.15 inside a program with those of the fixed release of its line, from python.org’s embeddable package or python-build-standalone, checked by SHA-256. Before and after, that Python runs eMudScan’s tests (ssl and hashlib modules, Windows roots, TLS 1.2 and 1.3 in memory, rejection of a compromised chain): if one that passed fails after, the original files are put back. **Restore the backup** undoes the fix as a whole, even after a copy that a crash interrupted.
- Update a Deno installed with its own installer with `deno upgrade` (Deno 2.9.8 is fixed).
- Give the command that updates certifi in a Pipenv, Poetry or uv project whose lock file pins the outdated version; eMudScan never runs it.
- Open the detailed information of a software on a double-click in the Fix software window, where it can also be fixed or restored.
- Sort the list of the Fix software window on a click on a column header.
- Add the right-click menu of the main window, and Ctrl+C, to the list of the Fix software window.
- Add **Fix** to the right-click menu of the lists, for the selected items that can be fixed.
- Link each result in the Fix console to the detailed information of its software.
- Highlight the PowerShell commands in the colors of PowerShell 7.
- Name the PC at the top of the report.

### Changed

- Shorten the detailed information for Python: one sentence for the problem, then the recommended action, which names the program a bundled Python belongs to.
- Show the explanation of a finding in several places once, with the commands for each place.
- Rename “Back up the certificate files” to “Back up the files to change”: it also covers node.exe and the ssl files of a Python.
- Stop suggesting an update of everything installed with a package manager (`scoop update -a`, `winget upgrade --all`, `choco upgrade all`).
- Remove the **Stop** button from the Fix window opened from the detailed information: a fix that has started to change files runs to its end.
- Say in the About box and the README that eMudScan is intended for administrators, developers and advanced users.
- Show the link to the source code beside the title of the About box, and the sites that the fixes connect to as a list.
- Name we-have-a-problem.com, a third-party site, instead of “the advisory”.
- Explain the order of the fixes in the warning when it differs from the one shown, and scroll the console only as much as needed when a software is ticked.
- Stop the fixes at the first download that fails, and after an update that fails because the servers of its package manager cannot be reached.
- Show the result of each command in the Fix console (a green ✔ or a red ✘), its warnings in yellow, its errors in red and its exit code when it failed; put ✓ or ✗ before each command in the report.
- End the Fix console with a horizontal rule instead of “End of the fixes.”; in the report, separate the sections with a line of em dashes.
- Say “PowerShell commands that eMudScan will run” in the Fix window, and which stage finished before each check after the fixes.
- Rename the details window to “Detailed information”.
- Count what each entry of the right-click menu acts on with several items (“Open 2 software folders”), and stop asking before **Scan again**.
- Name what **Search on Google** searches for, such as **Search on Google: “cacert.pem”**: the first three searches, then how many more.
- Open the detailed information of several items in one window, with a block to go from one item to another (Alt+Home, Alt+←, Alt+→, Alt+End), instead of one window after another.
- Make the path of the report saved after the fixes a link in the Fix console.
- Untick a software still not fixed after its fixes, and say when an update installed no newer version: “still not fixed (no newer version installed)”.
- Show the date of the certificate data of a bundle made by curl, and no software date for a bundle that belongs to no software: it was the modification date of the file.

### Fixed

- Fix the web search for a certificate bundle, which used the folder of the file (“Downloads cacert.pem”): search for the software with the name of the file, or for the name of the file alone.
- Open several homepages or searches in one start of the default browser, as tabs: Firefox asked for a profile once per page, and a page could get lost.
- Hide “Back up the files to change” when no fix to run backs up a file, such as an update with pip.
- Give no pip command where it cannot work: where pip is not installed, or for a Python marked EXTERNALLY-MANAGED, as uv does.
- Give no Scoop update command for a file in Scoop’s `persist` folder, which its updates keep as it is.
- Update the packages of a tool that uv installed with `uv tool upgrade`, instead of pip.
- Update the packages of an application that pipx installed with `pipx runpip`.
- Keep the original creation date of a file put back by **Restore the backup**.
- Update the summary of the certificate bundles after fixes and restores.
- Render the ANSI color codes in the output of commands, such as git under Scoop, as colors in the Fix console, and strip them from the report.

## 2.0.0 - 2026-11-05

First public version.

### Added

- Check that Windows blocks the 16 compromised certificates (8 eMudhra intermediates and their copies cross-signed by Sectigo), offline and with an optional online test, and whether an antivirus or a proxy inspects secure connections.
- Check browsers and internet apps by the block lists they receive or carry, including Microsoft Edge and WebView2.
- Check other software: certificate bundles (certifi, pip, Git, curl…), Python, Node.js, Java, Go, Electron applications, PyInstaller programs and built-in root stores.
- Give the recommended action for each item, with its commands, and flag software that needs an update and is a year old or more.
- Fix what can be fixed, once confirmed: package manager updates (Scoop, WinGet, Chocolatey, pip, conda), Mozilla’s certificate bundle, Java block lists. Back up the files first, check each software again, and save a report in Downloads.
- Add a report to copy or save, in English and French.
- Change nothing on the PC until a fix is confirmed, and make no internet connection when the online test is cleared.

[unreleased]: https://github.com/3c0/eMudScan/compare/3.1.2...HEAD
[3.1.2]: https://github.com/3c0/eMudScan/compare/3.1.1...3.1.2
[3.1.1]: https://github.com/3c0/eMudScan/compare/3.1.0...3.1.1
[3.1.0]: https://github.com/3c0/eMudScan/compare/3.0.1...3.1.0
[3.0.1]: https://github.com/3c0/eMudScan/releases/tag/3.0.1
