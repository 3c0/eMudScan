# Changelog

All notable changes to eMudScan are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). eMudScan does not follow Semantic Versioning. Its versions are numbered MAJOR.MINOR.HOTFIX: a major version is a milestone; a minor version brings new checks, fixes and improvements; a hotfix version brings small or urgent fixes to the latest version.

## [Unreleased]

### Added

- When a report is saved, a note at the end of the log gives its path, as a link that shows the file in File Explorer, to find the reports again. The notes are shown in the window only: the reports do not have them.

### Changed

- In the log, the line of a certificate bundle gives the file itself instead of its folder (“Certificate bundle — file”), and the line of a program says “in” before its folder (“Java in folder”) instead of a dash.
- The name of a saved report has the name of the PC, such as `eMudScan-report-MY-PC-20261201-101548.txt`, so that the reports of several PCs can be kept in one folder.

## [3.1.0] - 2026-11-11

### Added

- `eMudScan-cli.exe`, a second program for administrators, which scans from the command line: a console program, so command prompts and scripts wait for it and get its exit code. It is the same scanner as `eMudScan.exe` and works alone. Its exit code gives the worst status found (0: OK or information only, 1: warning, 2: update needed, 3: critical, 10: the scan could not run). `--report FILE` writes the report as text, JSON or CSV, by the extension of the file, and can be given several times; without it, the text report goes to the standard output. The log goes to the standard error output. `--drives C:,D:` chooses the drives. It never runs a fix, and makes no internet connection unless `--online` is given.
- **Save report** can save the report as a JSON or a CSV file, for other programs and spreadsheets: choose the type in the list of types of its dialog. The JSON file has every item with all its fields, the fixes and the log; the CSV file has one row for each item, with commas. Their field names and keywords are in English in both languages. “The report”, in `docs/using.md`, describes the fields. **Copy report** still copies the text.

### Changed

- The language is chosen on the command line with `--lang en` or `--lang fr`, which replace `--en` and `--fr`.
- In the log, the note “No drive selected…” comes below the first line, between two empty lines and without the indent of the results, instead of above it.
- The name of a saved report has the seconds, such as `eMudScan-report-20261201-101548.txt`, so that two reports saved in the same minute have different names.
- The About box, like the README, says that eMudScan is a technical tool for advanced users: security researchers, administrators and developers.

### Fixed

- No longer takes the version of a program for the version of its Python or Node.js. A program can start its own copy through a `python.exe` or `node.exe` of its own, which carries the version of the program: a high number passed for a fixed version, and the copy was shown as OK. eMudScan now reads the version of Python from Python’s own files, and finds its ssl library in the folder where the program keeps it.
- The exe built by GitHub no longer says, in its Go build information, that it was built from modified files (“+dirty”): the Windows resources (icon, manifest, version information) now come out the same with any Go version.

## [3.0.1] - 2026-11-10

### Changed

- In the main window, when a border between two frames is released over a line of the log or a row of a list, that line or row is shown whole if at least half of it was shown, and hidden otherwise, instead of always hidden.

### Fixed

- The list of the Fix software window shows whole rows, as the lists of the main window do: when the window is resized, its height is rounded down to whole rows, and when the bar below the list is released over a row, that row is shown whole if at least half of it was shown, and hidden otherwise.

## 3.0.0 - 2026-11-10

Changes since 2.0.0, where this repository starts.

### Added

- Checks jetbrainsd, the background service of the JetBrains Toolbox App and IDEs, which has its own list of trusted certificates: version 0.9.9728 or later is fixed.
- Checks the JetBrains IDEs and Gateway by their version, for their built-in browser, and names the Java runtime of an IDE or of the Toolbox App after it, with the version that fixes it.
- Updates an outdated Python 3.14 or later installed with the Python Install Manager, with that manager, when it is installed.
- Upgrades an outdated Python installed by uv with uv, after updating uv itself as it was installed (Scoop, WinGet, Chocolatey, its standalone installer or pipx), then removes the old copy once a fixed version is installed, naming the virtual environments found that use that copy, also in the warning before the fix runs.
- Can replace the node.exe inside a program with the latest release of its Node.js release line from nodejs.org, once checked (its published SHA-256, its OpenJS Foundation signature and its version), after backing it up: a fix that is not ticked by default, since it changes a program that its publisher has not updated yet.
- Can replace the ssl files of a 64-bit Python 3.11 to 3.15 inside a program with those of the fixed release of its line, from python.org’s embeddable package or python-build-standalone’s build (on GitHub), once checked against their SHA-256. Before and after, that Python runs eMudScan’s tests (its ssl and hashlib modules, the Windows roots, TLS 1.2 and 1.3 connections in memory, the rejection of a compromised chain); if one that passed before fails after, the original files are put back. A fix that is not ticked by default: a software that also has other fixes gets a second row for it in the Fix window. **Restore the backup** puts the original files back and removes those the fix added, all together, only when all the backups and marks of that fix are there, even after a copy that a crash interrupted.
- Updates a Deno installed with its own installer with `deno upgrade`, Deno’s own updater (Deno 2.9.8 is fixed).
- For the virtual environment of a Pipenv, Poetry or uv project whose lock file pins the outdated certifi, which the project’s next install would bring back, the detailed information gives the command that updates it in the project, which eMudScan never runs.
- In the Fix software window, a double-click on a software opens its detailed information, where it can also be fixed or restored; the list is updated when they close.
- The list of the Fix software window sorts by a click on a column header, as the lists of the main window do.
- The list of the Fix software window has the right-click menu of the main window, and Ctrl+C copies its selected row. A reminder beside the number of selected software says so.
- **Fix**, in the right-click menu of the lists, opens the Fix software window for the selected items that can be fixed, such as “Fix 3 items…”.
- After the fixes, each result of the checks in the console of the Fix software window ends with a link to the detailed information of its software, such as “Detailed information on Inkscape…”.
- The PowerShell commands, in the detailed information and in the Fix window, are shown in the colors that PowerShell 7 gives them.
- The top of the report names the PC it was made on.

### Changed

- Shorter detailed information for Python: one sentence for the problem, then the recommended action, which names the program a bundled Python belongs to.
- A finding in several places shows its explanation once, with the commands for each place.
- “Back up the certificate files”, in the Fix window, is now “Back up the files to change”, since it also covers node.exe and the ssl files of a Python.
- The detailed information no longer suggests updating everything installed with a package manager (`scoop update -a`, `winget upgrade --all`, `choco upgrade all`): eMudScan stays targeted, with the update of the software it found only.
- The Fix window opened from the detailed information no longer has a **Stop** button: it fixes one software, and a fix that has started to change files always runs to its end.
- The About box, like the README, says that eMudScan is intended for administrators, developers and advanced users.
- The About box shows the link to the source code beside its title, and the sites that the fixes connect to as a list.
- The texts name we-have-a-problem.com, a third-party site, instead of “the advisory”.
- When the fixes run in a different order from the one shown in the Fix window, the warning before they run explains the order. Ticking a software now scrolls the console only as much as needed to show its commands.
- The fixes stop at the first download that fails, before its fix changes anything, and after an update that fails when the servers of its package manager cannot be reached (eMudScan then checks them): the next fixes would most likely fail too. A red line says why.
- The console of the Fix window shows the result of each command in a column on its left (a green ✔ or a red ✘), its warnings in yellow and its errors in red below it, and its exit code when it failed; the report puts the ✓ or ✗ before each command.
- The console of the Fix window ends with a horizontal bar instead of “End of the fixes.”, with an empty line before and after it. In the report, the bar is a line of em dashes (—), as wide as the top of the report, and it also separates the sections.
- The Fix window says “PowerShell commands that eMudScan will run” instead of “PowerShell commands to run”. Each check after the fixes says which stage finished, such as “Updates finished. Checking again whether the software is still vulnerable…”.
- The details window is now named “Detailed information”, in the right-click menu, the reminders, the tooltip of the links to an item and the docs.
- With several items selected, each entry of the right-click menu says how many things it acts on, counting once what the items share, such as “Open 2 software folders” for three items in two folders; **Search on Google** searches once per software. **Scan again** no longer asks before scanning more than two items, since it opens no window.
- **Search on Google**, in the right-click menu, names what it searches for, such as **Search on Google: “cacert.pem”**; for more than three searches, it names the first three and says how many more.
- The detailed information of several items opens in one window, instead of one window after another, and without asking first for more than two items. A block in its top right corner, which stays in place as the page scrolls, says which item is shown and goes to the first, previous, next or last one; Alt+Home, Alt+←, Alt+→ and Alt+End do the same.
- At the end of the console of the Fix software window, the path of the report saved after the fixes is a link that shows it in File Explorer, as in the message that gives it.
- After the fixes, a software still not fixed after all its fixes ran is unticked in the Fix software window, so that they do not run again by mistake; when an update installed no newer version, its result says so: “still not fixed (no newer version installed)”.
- A certificate bundle made by curl shows the date of its certificate data, as curl wrote it at the top of the file: “Certificate data from Mozilla as of: 25 September 2026”. A bundle that belongs to no software no longer shows a software date: it was the modification date of the file, which only tells when the file was copied or downloaded.

### Fixed

- The web search for a certificate bundle of a software that eMudScan does not know is the software shown in the list with the file’s name, and the file’s name alone for a bundle that belongs to no software: it used the folder the file is in, such as “Downloads cacert.pem”.
- Several homepages or Google searches, from the right-click menu, open in one start of the default browser, as tabs: Firefox, set to ask for a profile at start, asked once per page, and a page could get lost when the browser was already open.
- The Fix window no longer shows “Back up the files to change” when no fix to run backs up a file, such as an update with pip.
- No pip command where it cannot work: for a Python or virtual environment where pip is not installed, or for a Python marked as managed by the tool that installed it (EXTERNALLY-MANAGED, as uv does), which pip refuses to change.
- No Scoop update command for a file in Scoop’s `persist` folder, which Scoop’s updates keep as it is.
- The packages of a tool that uv installed are updated with `uv tool upgrade`, as uv asks, instead of pip, which uv does not put in its tools.
- The packages of an application that pipx installed are updated with `pipx runpip`, which works whether or not its environment has pip of its own.
- A file put back by **Restore the backup** has its original creation date again: eMudScan’s backups now keep it, where Windows gave them the date of the backup.
- The summary of the certificate bundles is updated after fixes and restores: the files it lists are tested again, instead of keeping the results of the scan.
- The color codes in the output of some commands, such as git when Scoop updates its buckets, no longer appear as garbage. The console of the Fix window shows these colors, and the report shows the text alone.

## 2.0.0 - 2026-11-05

First public version.

### Added

- Checks that Windows blocks the 16 compromised certificates (the 8 eMudhra intermediates and their copies cross-signed by Sectigo), offline and, if you allow it, with an online test against the advisory’s test sites, and whether an antivirus or a proxy inspects secure connections.
- Checks browsers and internet apps (messaging, email, sync, VPN, remote access) by the block lists they receive or carry, including Microsoft Edge and the WebView2 that software uses or ships.
- Checks other software: every certificate bundle on the scanned drives (certifi, pip, Git, curl…), Python, Node.js, Java, Go, Electron applications, programs built with PyInstaller and programs with their own root store.
- Says what to do about each one, with the commands to run, and flags software that needs an update and is a year old or more.
- Fixes what it can, once you confirm: an update through the package manager that installed the software (Scoop, WinGet, Chocolatey, pip or conda), Mozilla’s current certificate bundle in place of an outdated one, or the compromised certificates added to a Java block list. The certificate files are backed up first, and can be put back from the details window; each software is checked again, and a report is saved in your Downloads folder.
- A report to copy or save. English and French.
- Changes nothing on your PC until you confirm a fix. Clear the online test, and a scan makes no internet connection at all.

[unreleased]: https://github.com/3c0/eMudScan/compare/3.1.0...HEAD
[3.1.0]: https://github.com/3c0/eMudScan/compare/3.0.1...3.1.0
[3.0.1]: https://github.com/3c0/eMudScan/releases/tag/3.0.1
