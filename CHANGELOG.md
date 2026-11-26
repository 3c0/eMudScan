# Changelog

All notable changes to eMudScan are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). eMudScan does not follow Semantic Versioning. Its versions are numbered MAJOR.MINOR.HOTFIX: a major version is a milestone; a minor version brings new checks, fixes and improvements; a hotfix version brings small or urgent fixes to the latest version.

## [Unreleased]

### Added

- Add self-update. eMudScan asks the GitHub API for its latest release at the first scan of a session and from **Check for updates** in the About box, and shows the release notes of up to 10 new versions. It installs only an immutable release whose `eMudScan.exe` has the SHA-256 that the API and the release notes give. A copy of itself, run from a folder in Temp that only the user, SYSTEM and the administrators can open, writes the new exe under a random name beside the old one, checks it, and renames it over `eMudScan.exe` in one step, with one UAC prompt when the folder needs it. `eMudScan-cli.exe` never updates itself ([`67ed9d1`](https://github.com/3c0/eMudScan/commit/67ed9d1), [`441fb9c`](https://github.com/3c0/eMudScan/commit/441fb9c))
- Add a viewer for `check_ssl.py`, the tests that the fix of a Python runs in that Python: its name, in the commands of the Fix window, is a link that shows the file with syntax highlighting and a copy button. The file is embedded as it is from `cmd/emudscan/check_ssl.py`, and its docstrings say what each test checks. The commands say what `$emudscanFiles` is: the folder of the fix in Temp. Add a **Copy all** button to the box of the licenses ([`1b68a1a`](https://github.com/3c0/eMudScan/commit/1b68a1a), [`c7a683e`](https://github.com/3c0/eMudScan/commit/c7a683e), [`e2a6b0a`](https://github.com/3c0/eMudScan/commit/e2a6b0a), [`cb685eb`](https://github.com/3c0/eMudScan/commit/cb685eb), [`be68d3a`](https://github.com/3c0/eMudScan/commit/be68d3a))
- Name the PowerShell that runs the fixes in the Fix window and in the report. Without Windows PowerShell 5.1, run the fixes in PowerShell 7 or later from Program Files, never in a `pwsh.exe` found in the PATH, since some fixes run as administrator; with neither, disable the fixes ([`2cd5cf0`](https://github.com/3c0/eMudScan/commit/2cd5cf0), [`35b3478`](https://github.com/3c0/eMudScan/commit/35b3478))
- Wrap a command line too long for its box with ↩ and ↪, at the indent of the line, in the detailed information and the Fix console. The signs are not selected or copied ([`6ce1a6d`](https://github.com/3c0/eMudScan/commit/6ce1a6d))

### Changed

- Rewrite the fixes of a certificate bundle and of a Java block list, and the restore of backups, as scripts made to be read: comments, one command per line, a stop at the first error. The detailed information gives each as one block that also runs when pasted into a PowerShell window. Set Windows PowerShell to TLS 1.2 for the download of a bundle, instead of adding TLS 1.2 to its protocols ([`720c184`](https://github.com/3c0/eMudScan/commit/720c184))
- Separate two software with a rule in the Fix console. Start each software with its name and `# Location:`, and each fix with `# Fix:`, or `# Fix 1:` and `# Fix 2, if the update does not fix it:`, instead of “Then, if the update does not fix it:” ([`e87eb59`](https://github.com/3c0/eMudScan/commit/e87eb59))
- Replace “In PowerShell:”, above the commands of a fix in the detailed information, with a sentence that says what to do: click **Fix this software**, or run the script in PowerShell 5.1, 7.6 or 7.7, the versions that the tests run on GitHub. End it with **(Run as administrator)**, in bold red, when the commands need it ([`d714402`](https://github.com/3c0/eMudScan/commit/d714402), [`16215ea`](https://github.com/3c0/eMudScan/commit/16215ea))
- Say what happened, what it means and what to do in the errors of the scripts that replace the ssl files of a Python ([`c7a683e`](https://github.com/3c0/eMudScan/commit/c7a683e))
- **Breaking:** remove the commands of the fixes from the reports, which keep the explanations. In the JSON report, `fixes` loses `commands`, and `format_version` becomes 2 ([`d60f679`](https://github.com/3c0/eMudScan/commit/d60f679))
- Report what came of the fixes in the Fixes section of the reports (`fixes_run` in JSON): the result of each software and, for a command that failed, the command and its output, instead of every command ([`d60f679`](https://github.com/3c0/eMudScan/commit/d60f679))
- Give, in the detailed information of a Python inside a program, a standalone script that replaces its ssl files without eMudScan: one block that stops at its first error, tests before and after that this Python loads its `ssl` module, and downloads into a folder of its own in Temp. The Fix window runs the version with eMudScan’s tests ([`1b68a1a`](https://github.com/3c0/eMudScan/commit/1b68a1a))
- Rewrite the fix of the ssl files of a Python as a script made to be read: comments, one command per line, a stop at the first error, and a `try` block whose `catch` puts the original files back when the replacement or a test after it fails. Show the result of each command that ran (✔ or ✘) in the Fix window and the report, instead of the warning “No file was changed” ([`1b68a1a`](https://github.com/3c0/eMudScan/commit/1b68a1a))
- Download the latest release of its line in the fix of a Python, instead of a pinned one. eMudScan looks for it when a Fix window opens: on GitHub for python-build-standalone, whose releases are immutable, and on python.org, where it takes the latest release of the line that has Windows files. The commands keep one URL and its SHA-256. Without a release found, or with **Online checks and updates** cleared, the fix uses the pinned release, now 20261124 for python-build-standalone ([`99d7ae3`](https://github.com/3c0/eMudScan/commit/99d7ae3), [`dc6e004`](https://github.com/3c0/eMudScan/commit/dc6e004))
- Rewrite the fix of a node.exe as a script made to be read. It downloads the latest release of its line, which eMudScan looks for on nodejs.org when a Fix window opens (pinned: Node.js 22.23.5, 24.22.3 and 26.13.1), and checks its SHA-256, its OpenJS Foundation signature and its version before it changes anything. The detailed information gives a standalone version. A 32-bit node.exe of Node.js 24 or later gets no fix: nodejs.org builds none ([`675a036`](https://github.com/3c0/eMudScan/commit/675a036))
- Rename “Allow the online test” to “Online checks and updates”: the box covers every connection of eMudScan itself, such as the lookup of the latest releases ([`349f1fb`](https://github.com/3c0/eMudScan/commit/349f1fb))
- Open the detailed information 1000 pixels wide at 100 % scaling, instead of 850, so that most commands fit on one line, and the Fix window of one software 900 pixels wide, instead of 860 ([`6e9d415`](https://github.com/3c0/eMudScan/commit/6e9d415))
- Justify the text of the detailed information and of the About box, with tighter lines and square bullets. Scroll the detailed information smoothly. Let the text of the About box be selected and copied ([`5ab078d`](https://github.com/3c0/eMudScan/commit/5ab078d), [`a77de58`](https://github.com/3c0/eMudScan/commit/a77de58))
- Widen the About box from 525 to 720 pixels ([`67ed9d1`](https://github.com/3c0/eMudScan/commit/67ed9d1), [`209e168`](https://github.com/3c0/eMudScan/commit/209e168))
- Show the User-Agent on a line of its own, in a smaller font, in the About box, and say that every site eMudScan connects to sees it, with the IP address: the text named the test site only ([`209e168`](https://github.com/3c0/eMudScan/commit/209e168))
- Scroll the lists one row per notch of the mouse wheel, instead of three ([`e7ba84d`](https://github.com/3c0/eMudScan/commit/e7ba84d))
- Show **Copied** on **Copy report** for 2 seconds, instead of a message box ([`06df12f`](https://github.com/3c0/eMudScan/commit/06df12f))
- Focus the search box when a scan starts, not when it ends ([`e7a7cba`](https://github.com/3c0/eMudScan/commit/e7a7cba))
- Show the wait cursor during a scan, **Scan again** and the lookup of a new version, except over **Stop** ([`6323548`](https://github.com/3c0/eMudScan/commit/6323548))
- Search the web for the software alone, for a certificate bundle of an unknown software, instead of the software with the name of the file ([`e97875e`](https://github.com/3c0/eMudScan/commit/e97875e))
- Say in the About box and the README that eMudScan was made by 3c0 with the help of AI ([`14e0aeb`](https://github.com/3c0/eMudScan/commit/14e0aeb))
- Name python-build-standalone in the About box and in the licenses (Mozilla Public License 2.0, no file included), and the publishers of the files that the fixes download in the disclaimer ([`dc6e004`](https://github.com/3c0/eMudScan/commit/dc6e004))
- Show the running test, and those of 1 second or more, in `scripts/build.ps1`. Look for the PowerShells once in the PowerShell tests, and run their cases in parallel ([`c4bee17`](https://github.com/3c0/eMudScan/commit/c4bee17))

### Fixed

- Fix the beep of Ctrl+C and Ctrl+A in the log, the Fix console and the detailed information: the dialog manager took them for the mnemonic of a button ([`a3ce7f5`](https://github.com/3c0/eMudScan/commit/a3ce7f5))
- Fix the fixes that check a SHA-256 when eMudScan is started from PowerShell 7: Windows PowerShell got the `PSModulePath` of PowerShell 7 and no longer found `Get-FileHash`. The fixes put the modules of their own PowerShell first, and the check of eMudScan’s own files no longer uses a module command ([`651068f`](https://github.com/3c0/eMudScan/commit/651068f))
- Fix the empty rows under the final rule of the Fix console in a narrow window ([`126b681`](https://github.com/3c0/eMudScan/commit/126b681))
- Break a word too long for its line, such as a URL or a path, after a slash, a backslash or a hyphen, instead of in the middle of a name ([`cfc9592`](https://github.com/3c0/eMudScan/commit/cfc9592))
- Fix the scroll of the Fix console on a click on a copy of a software, such as one of two Pythons inside the same program ([`4e09c0c`](https://github.com/3c0/eMudScan/commit/4e09c0c))
- Fix `scripts/build.ps1` in Windows PowerShell 5.1 (“no Go files”): it gave go an empty `-tags` option ([`07fdfc0`](https://github.com/3c0/eMudScan/commit/07fdfc0))
- Show the tooltip of a greyed-out button, which says when it can be used: a disabled button gets no mouse messages ([`fcb8e42`](https://github.com/3c0/eMudScan/commit/fcb8e42))
- Fix the room after each drive of **Drives to scan:**, which was wider than after the label ([`25d9db2`](https://github.com/3c0/eMudScan/commit/25d9db2))
- Center the separator between **Online checks and updates** and the drives, measured to the ink of the letters ([`25d9db2`](https://github.com/3c0/eMudScan/commit/25d9db2))
- Fix the Tab order: it follows the reading order and reaches every control, the ⓘ button and the Fix consoles included. Inside a text (log, console, detailed information, About box, release notes), Tab goes through the links and the copy buttons, and Enter or Space activates them. Draw a dotted focus frame where none showed, and give the focus back to its control when a box or a Fix window closes ([`e7a7cba`](https://github.com/3c0/eMudScan/commit/e7a7cba))
- Follow a change of the display scale (`WM_DPICHANGED`) in the windows open above the main one: About box, licenses, saved report, update, detailed information and Fix windows. They kept the fonts of the old scale in a window that Windows resized. Each window, the main one included, gets back the size it had at a scale ([`df4ff7e`](https://github.com/3c0/eMudScan/commit/df4ff7e))

## [3.1.2] - 2026-11-12

### Changed

- Give the result on this PC first in the detailed information of “Software copying the Windows root store”, which no longer reads as a check of Python only ([`f0b67fa`](https://github.com/3c0/eMudScan/commit/f0b67fa))

### Fixed

- Remove the outdated advice not to delete the roots from the detailed information about the Windows roots, which now says only what is found on this PC ([`9164a5f`](https://github.com/3c0/eMudScan/commit/9164a5f))

## [3.1.1] - 2026-11-11

### Added

- Flag a certificate bundle that is a byte-for-byte copy of the root store built into a program of its folder, or of the folder above, in the detailed information of both: the program can write the file again ([`4093363`](https://github.com/3c0/eMudScan/commit/4093363))
- Add a note at the end of the log when a report is saved, with a link that shows the file in File Explorer ([`19802f4`](https://github.com/3c0/eMudScan/commit/19802f4))

### Changed

- Give the file of a certificate bundle, instead of its folder, in the log, and say “in” before the folder of a program ([`a34451b`](https://github.com/3c0/eMudScan/commit/a34451b))
- Add the name of the PC to the name of a saved report: `eMudScan-report-MY-PC-20261201-101548.txt` ([`55a4897`](https://github.com/3c0/eMudScan/commit/55a4897))

## [3.1.0] - 2026-11-11

### Added

- Add `eMudScan-cli.exe`, a console program that scans from the command line, for scripts and deployment tools. `--report FILE` writes the report as text, JSON or CSV, by the extension of the file, `--drives C:,D:` chooses the drives, and `--online` allows the online test. The log goes to the standard error output, and the exit code gives the worst status: 0 OK or information, 1 warning, 2 update needed, 3 critical, 10 the scan could not run. It never runs a fix ([`b810d2b`](https://github.com/3c0/eMudScan/commit/b810d2b), [`052014a`](https://github.com/3c0/eMudScan/commit/052014a))
- Add JSON and CSV reports to **Save report**: every item with all its fields in JSON, one row per item in CSV, with field names in English. `docs/using.md` describes the fields ([`19e2d12`](https://github.com/3c0/eMudScan/commit/19e2d12))

### Changed

- **Breaking:** replace `--en` and `--fr` with `--lang en` and `--lang fr` ([`32b4aae`](https://github.com/3c0/eMudScan/commit/32b4aae))
- Move the note “No drive selected…” below the first line of the log ([`8af41f0`](https://github.com/3c0/eMudScan/commit/8af41f0))
- Add the seconds to the name of a saved report: `eMudScan-report-20261201-101548.txt` ([`115e34a`](https://github.com/3c0/eMudScan/commit/115e34a))
- Say in the About box and the README that eMudScan is a technical tool for advanced users: security researchers, administrators and developers ([`77e7003`](https://github.com/3c0/eMudScan/commit/77e7003))

### Fixed

- Fix a Python or a Node.js shown as OK when its program starts it through a `python.exe` or `node.exe` of its own, which carries the version of the program: read the version of Python from Python’s own files ([`672bf76`](https://github.com/3c0/eMudScan/commit/672bf76))
- Fix the “+dirty” mark in the build information of the exe built by GitHub: generate the same Windows resources with any Go version ([`73bb0ab`](https://github.com/3c0/eMudScan/commit/73bb0ab))

## [3.0.1] - 2026-11-10

### Changed

- Show whole the row over which a border of the main window is released, when at least half of it was shown, and hide it otherwise, instead of always hiding it ([`b6efc8f`](https://github.com/3c0/eMudScan/commit/b6efc8f))

### Fixed

- Show whole rows in the list of the Fix software window, as in the lists of the main window ([`fbdd7ce`](https://github.com/3c0/eMudScan/commit/fbdd7ce))

## 3.0.0 - 2026-11-10

Changes since 2.0.0, where this repository starts.

### Added

- Check jetbrainsd, the background service of the JetBrains Toolbox App and IDEs, which has its own list of trusted certificates: version 0.9.9728 or later is fixed ([`c0accab`](https://github.com/3c0/eMudScan/commit/c0accab))
- Check the JetBrains IDEs and Gateway by their version, for their built-in browser, and name the Java runtime of an IDE or of the Toolbox App after it ([`df6b453`](https://github.com/3c0/eMudScan/commit/df6b453), [`f67bbb3`](https://github.com/3c0/eMudScan/commit/f67bbb3))
- Update an outdated Python 3.14 or later with the Python Install Manager, when it is installed ([`59844b0`](https://github.com/3c0/eMudScan/commit/59844b0))
- Upgrade an outdated Python of uv with uv, after updating uv as it was installed (Scoop, WinGet, Chocolatey, its installer or pipx), then remove the old copy and name the virtual environments that used it ([`6e86682`](https://github.com/3c0/eMudScan/commit/6e86682), [`1769a38`](https://github.com/3c0/eMudScan/commit/1769a38))
- Add an optional fix that replaces the node.exe inside a program with the latest release of its Node.js line from nodejs.org, once checked: SHA-256, OpenJS Foundation signature and version ([`9faab52`](https://github.com/3c0/eMudScan/commit/9faab52), [`dfb7861`](https://github.com/3c0/eMudScan/commit/dfb7861), [`9c1458e`](https://github.com/3c0/eMudScan/commit/9c1458e))
- Add an optional fix that replaces the ssl files of a 64-bit Python 3.11 to 3.15 inside a program with those of the fixed release of its line, from python.org’s embeddable package or python-build-standalone, checked by SHA-256. Before and after, that Python runs eMudScan’s tests (ssl and hashlib modules, Windows roots, TLS 1.2 and 1.3 in memory, rejection of a compromised chain): if one that passed fails after, the original files are put back. **Restore the backup** undoes the fix as a whole, even after a copy that a crash interrupted ([`9faab52`](https://github.com/3c0/eMudScan/commit/9faab52), [`4225354`](https://github.com/3c0/eMudScan/commit/4225354), [`43d7401`](https://github.com/3c0/eMudScan/commit/43d7401))
- Update a Deno installed with its own installer with `deno upgrade` (Deno 2.9.8 is fixed) ([`2da1d79`](https://github.com/3c0/eMudScan/commit/2da1d79))
- Give the command that updates certifi in a Pipenv, Poetry or uv project whose lock file pins the outdated version; eMudScan never runs it ([`24277bd`](https://github.com/3c0/eMudScan/commit/24277bd))
- Open the detailed information of a software on a double-click in the Fix software window, where it can also be fixed or restored ([`dbba91e`](https://github.com/3c0/eMudScan/commit/dbba91e))
- Sort the list of the Fix software window on a click on a column header ([`f71d0cb`](https://github.com/3c0/eMudScan/commit/f71d0cb))
- Add the right-click menu of the main window, and Ctrl+C, to the list of the Fix software window ([`570659d`](https://github.com/3c0/eMudScan/commit/570659d))
- Add **Fix** to the right-click menu of the lists, for the selected items that can be fixed ([`971021b`](https://github.com/3c0/eMudScan/commit/971021b))
- Link each result in the Fix console to the detailed information of its software ([`83b60c4`](https://github.com/3c0/eMudScan/commit/83b60c4))
- Highlight the PowerShell commands in the colors of PowerShell 7 ([`adebf92`](https://github.com/3c0/eMudScan/commit/adebf92))
- Name the PC at the top of the report ([`c204734`](https://github.com/3c0/eMudScan/commit/c204734))

### Changed

- Shorten the detailed information for Python: one sentence for the problem, then the recommended action, which names the program a bundled Python belongs to ([`a46ca27`](https://github.com/3c0/eMudScan/commit/a46ca27))
- Show the explanation of a finding in several places once, with the commands for each place ([`a46ca27`](https://github.com/3c0/eMudScan/commit/a46ca27))
- Rename “Back up the certificate files” to “Back up the files to change”: it also covers node.exe and the ssl files of a Python ([`dfb7861`](https://github.com/3c0/eMudScan/commit/dfb7861), [`43d7401`](https://github.com/3c0/eMudScan/commit/43d7401))
- Stop suggesting an update of everything installed with a package manager (`scoop update -a`, `winget upgrade --all`, `choco upgrade all`) ([`b861bcd`](https://github.com/3c0/eMudScan/commit/b861bcd))
- Remove the **Stop** button from the Fix window opened from the detailed information: a fix that has started to change files runs to its end ([`454f7ae`](https://github.com/3c0/eMudScan/commit/454f7ae))
- Say in the About box and the README that eMudScan is intended for administrators, developers and advanced users ([`354e4f8`](https://github.com/3c0/eMudScan/commit/354e4f8))
- Show the link to the source code beside the title of the About box, and the sites that the fixes connect to as a list ([`5c9ccf5`](https://github.com/3c0/eMudScan/commit/5c9ccf5), [`095341c`](https://github.com/3c0/eMudScan/commit/095341c))
- Name we-have-a-problem.com, a third-party site, instead of “the advisory” ([`e5ae72a`](https://github.com/3c0/eMudScan/commit/e5ae72a))
- Explain the order of the fixes in the warning when it differs from the one shown, and scroll the console only as much as needed when a software is ticked ([`544a71f`](https://github.com/3c0/eMudScan/commit/544a71f), [`38de6a2`](https://github.com/3c0/eMudScan/commit/38de6a2))
- Stop the fixes at the first download that fails, and after an update that fails because the servers of its package manager cannot be reached ([`cc87d6d`](https://github.com/3c0/eMudScan/commit/cc87d6d), [`8ef52ab`](https://github.com/3c0/eMudScan/commit/8ef52ab))
- Show the result of each command in the Fix console (a green ✔ or a red ✘), its warnings in yellow, its errors in red and its exit code when it failed; put ✓ or ✗ before each command in the report ([`3a0506c`](https://github.com/3c0/eMudScan/commit/3a0506c), [`1a54540`](https://github.com/3c0/eMudScan/commit/1a54540))
- End the Fix console with a horizontal rule instead of “End of the fixes.”; in the report, separate the sections with a line of em dashes ([`6b3fc3e`](https://github.com/3c0/eMudScan/commit/6b3fc3e), [`d52fde3`](https://github.com/3c0/eMudScan/commit/d52fde3))
- Say “PowerShell commands that eMudScan will run” in the Fix window, and which stage finished before each check after the fixes ([`afafff5`](https://github.com/3c0/eMudScan/commit/afafff5))
- Rename the details window to “Detailed information” ([`9b0f06c`](https://github.com/3c0/eMudScan/commit/9b0f06c))
- Count what each entry of the right-click menu acts on with several items (“Open 2 software folders”), and stop asking before **Scan again** ([`6a916d6`](https://github.com/3c0/eMudScan/commit/6a916d6), [`c09a175`](https://github.com/3c0/eMudScan/commit/c09a175))
- Name what **Search on Google** searches for, such as **Search on Google: “cacert.pem”**: the first three searches, then how many more ([`cc8a56f`](https://github.com/3c0/eMudScan/commit/cc8a56f))
- Open the detailed information of several items in one window, with a block to go from one item to another (Alt+Home, Alt+←, Alt+→, Alt+End), instead of one window after another ([`6d3e1f5`](https://github.com/3c0/eMudScan/commit/6d3e1f5), [`9a36685`](https://github.com/3c0/eMudScan/commit/9a36685))
- Make the path of the report saved after the fixes a link in the Fix console ([`ce4abe6`](https://github.com/3c0/eMudScan/commit/ce4abe6))
- Untick a software still not fixed after its fixes, and say when an update installed no newer version: “still not fixed (no newer version installed)” ([`5297fb5`](https://github.com/3c0/eMudScan/commit/5297fb5))
- Show the date of the certificate data of a bundle made by curl, and no software date for a bundle that belongs to no software: it was the modification date of the file ([`a30d55d`](https://github.com/3c0/eMudScan/commit/a30d55d))

### Fixed

- Fix the web search for a certificate bundle, which used the folder of the file (“Downloads cacert.pem”): search for the software with the name of the file, or for the name of the file alone ([`b2c2610`](https://github.com/3c0/eMudScan/commit/b2c2610))
- Open several homepages or searches in one start of the default browser, as tabs: Firefox asked for a profile once per page, and a page could get lost ([`20dc37c`](https://github.com/3c0/eMudScan/commit/20dc37c))
- Hide “Back up the files to change” when no fix to run backs up a file, such as an update with pip ([`4e76ee7`](https://github.com/3c0/eMudScan/commit/4e76ee7))
- Give no pip command where it cannot work: where pip is not installed, or for a Python marked EXTERNALLY-MANAGED, as uv does ([`631fede`](https://github.com/3c0/eMudScan/commit/631fede))
- Give no Scoop update command for a file in Scoop’s `persist` folder, which its updates keep as it is ([`631fede`](https://github.com/3c0/eMudScan/commit/631fede))
- Update the packages of a tool that uv installed with `uv tool upgrade`, instead of pip ([`631fede`](https://github.com/3c0/eMudScan/commit/631fede))
- Update the packages of an application that pipx installed with `pipx runpip` ([`24277bd`](https://github.com/3c0/eMudScan/commit/24277bd))
- Keep the original creation date of a file put back by **Restore the backup** ([`e66e9ce`](https://github.com/3c0/eMudScan/commit/e66e9ce))
- Update the summary of the certificate bundles after fixes and restores ([`1820712`](https://github.com/3c0/eMudScan/commit/1820712))
- Render the ANSI color codes in the output of commands, such as git under Scoop, as colors in the Fix console, and strip them from the report ([`c2d410e`](https://github.com/3c0/eMudScan/commit/c2d410e))

## 2.0.0 - 2026-11-05

First public version.

### Added

- Check that Windows blocks the 16 compromised certificates (8 eMudhra intermediates and their copies cross-signed by Sectigo), offline and with an optional online test, and whether an antivirus or a proxy inspects secure connections ([`b33749f`](https://github.com/3c0/eMudScan/commit/b33749f))
- Check browsers and internet apps by the block lists they receive or carry, including Microsoft Edge and WebView2 ([`b33749f`](https://github.com/3c0/eMudScan/commit/b33749f))
- Check other software: certificate bundles (certifi, pip, Git, curl…), Python, Node.js, Java, Go, Electron applications, PyInstaller programs and built-in root stores ([`b33749f`](https://github.com/3c0/eMudScan/commit/b33749f))
- Give the recommended action for each item, with its commands, and flag software that needs an update and is a year old or more ([`b33749f`](https://github.com/3c0/eMudScan/commit/b33749f))
- Fix what can be fixed, once confirmed: package manager updates (Scoop, WinGet, Chocolatey, pip, conda), Mozilla’s certificate bundle, Java block lists. Back up the files first, check each software again, and save a report in Downloads ([`b33749f`](https://github.com/3c0/eMudScan/commit/b33749f))
- Add a report to copy or save, in English and French ([`b33749f`](https://github.com/3c0/eMudScan/commit/b33749f))
- Change nothing on the PC until a fix is confirmed, and make no internet connection when the online test is cleared ([`b33749f`](https://github.com/3c0/eMudScan/commit/b33749f))

[unreleased]: https://github.com/3c0/eMudScan/compare/3.1.2...HEAD
[3.1.2]: https://github.com/3c0/eMudScan/compare/3.1.1...3.1.2
[3.1.1]: https://github.com/3c0/eMudScan/compare/3.1.0...3.1.1
[3.1.0]: https://github.com/3c0/eMudScan/compare/3.0.1...3.1.0
[3.0.1]: https://github.com/3c0/eMudScan/releases/tag/3.0.1
