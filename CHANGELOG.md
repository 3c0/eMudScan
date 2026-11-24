# Changelog

All notable changes to eMudScan are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). eMudScan does not follow Semantic Versioning. Its versions are numbered MAJOR.MINOR.HOTFIX: a major version is a milestone; a minor version brings new checks, fixes and improvements; a hotfix version brings small or urgent fixes to the latest version.

## [Unreleased]

### Added

- In the Fix window, the name `check_ssl.py`, in the commands that replace the ssl files of a Python, is a link that shows this file, in colors of its own on a very dark blue: the tests that the fix runs in that Python, which you can read, and copy, before you run anything. Its docstrings and comments say what each test checks and why. These commands first say what `$emudscanFiles` is, the folder that eMudScan makes for the fix, with this file and its test certificates, and show it when the fix runs. The file is also in the source code as it is (`cmd/emudscan/check_ssl.py`). The box of the licenses and notices has the same new button, **Copy all**.
- The Fix window names the PowerShell that runs the fixes, such as Windows PowerShell 5.1, above the commands and above their results; the report names it too, in the line that says when the fixes ran. On a PC without Windows PowerShell, the fixes now run in PowerShell 7 or later, installed in Program Files; never in a `pwsh.exe` found elsewhere, such as in the PATH, since some fixes run as administrator. When there is neither, the window says so, and its button that runs the fixes stays disabled. The messages that named Windows PowerShell now name the PowerShell that runs the fixes.
- In the commands of the detailed information and in the console of the Fix window, a line too long for its box ends with ↩, and its rest starts with ↪ on the next row, at the indent of the line, so that it is not taken for another command. These signs are drawn in gray, and are not selected or copied.

### Changed

- The fixes that replace a certificate bundle, add certificates to a Java block list or restore backups are now scripts written to be read, as the fix of a Python: comments that say what each part does, one command on each line, and empty lines between the parts. They stop at their first command that fails, which replaces the warning “The download failed or is incomplete: no file was changed.” The detailed information gives them as one block that runs alone, also pasted into a PowerShell window. A certificate bundle is now downloaded with Windows PowerShell set to TLS 1.2, instead of TLS 1.2 added to its list.
- In the Fix window, a line drawn across the console separates two software. Each one starts with its name and its location (`# Location: …`), and each of its fixes with a line that says which fix it is: `# Fix: Replace the ssl files of Python…`, or `# Fix 1: Update with Scoop` and `# Fix 2, if the update does not fix it: Replace the certificate bundle…`. They replace the line that only said what the commands do, and “Then, if the update does not fix it:”. The console shows the same while the fixes run. When copied, the line between two software is a comment, which PowerShell skips.
- Above the commands of a fix, the detailed information says what to do with them, in place of “In PowerShell:”: “To fix this vulnerability, click **Fix this software**, or run this script yourself in PowerShell:” (or “this command”, “these commands”); “To do so, …” when the fix is only good practice; “To undo the fix, click **Restore the backups**, …” for a restore; and “Run this command in PowerShell:” for the commands that the button does not run. Above a script, the sentence names the versions of PowerShell that the tests run the scripts in, on GitHub: “in PowerShell 5.1, 7.6 or 7.7”. When the commands need administrator rights, the sentence ends with “in PowerShell (Run as administrator):”, with “(Run as administrator)” in bold, bright red. The advice that leads to commands of a fix now ends with a period, the sentence “To undo these changes, run the following command…” and, for a Python inside a program, “If you manage this Python yourself:” are gone, replaced by the new sentence.
- The errors of the scripts that replace the ssl files of a Python say what happened, what it means and what to do, such as “The replacement failed. The original files are back: this Python works as before, and is still not fixed. Update the software it belongs to when it has a fixed version.” The script that runs alone also shows why a replacement failed, before it puts the original files back.
- The reports no longer contain the commands of the fixes. The text and JSON reports give the detailed information without its commands, without the lines that announce or explain them, and without what eMudScan offers to do itself (“Until then, eMudScan can replace…”), which is for who runs eMudScan; a sentence that led to commands, such as “update it with uv:”, stays, and ends with a period. In the JSON report, `fixes` no longer has `commands`, and `format_version` is now 2. In the detailed information of a Python installed with uv, “The last command removes this copy…” is now on its own line, and a conda environment whose conda is not found gets the same recommended action as the others.
- After fixes, the Fixes section of the reports (`fixes_run` in the JSON report) says what came of them: the result of each software and, for a command that failed, that command and what it showed. It no longer lists the commands that succeeded or did not run.
- The detailed information of a Python inside a program now gives a script that you can copy and run without eMudScan to replace its ssl files, also pasted into a PowerShell window: one block that stops at its first error, tests before and after that this Python loads its ssl module, and downloads into a folder of its own in the Temp folder, which it deletes at its end. Until now it gave commands that only eMudScan could run, since they use its own tests. The Fix window shows, and runs, the version with these tests.
- The fix that replaces the ssl files of a Python inside a program is now a script written to be read: one command on each line, with comments that say what each part does. It stops at its first command that fails, and its `try` block puts the original files back when the replacement or the tests after it fail. In the Fix window and in the report, each command that ran has its result (✔ or ✘); a comment, a line of a block and a command that did not run have none. When a file is in use, the message is now the one of Windows; the warning “No file was changed” is gone, since the result of each command says where the fix stopped.
- The fix that replaces the ssl files of a Python inside a program uses the latest release of its line. When a Fix window opens, eMudScan itself looks for it: on GitHub for python-build-standalone, whose releases are immutable, and on python.org, where it takes the latest release of the line that has Windows files. The commands stay simple: one URL and its SHA-256. When no release is found, or when **Online checks and updates** is cleared, the commands use the release written in eMudScan, now 20261110 for python-build-standalone, and the window says so.
- The fix that replaces the node.exe inside a program is now a script written to be read, as the fix of a Python. It downloads one release, by its URL, and checks it before it changes anything: its SHA-256, written in the commands, its signature by the OpenJS Foundation and the version it gives. When a Fix window opens, eMudScan itself looks for the latest release of its line on nodejs.org. When no release is found, or when **Online checks and updates** is cleared, the commands use the release written in eMudScan, Node.js 22.23.5, 24.22.3 or 26.13.1, and the window says so. The detailed information gives a version that runs alone, also pasted into a PowerShell window: it downloads into a folder of its own in the Temp folder, and deletes it at its end. A 32-bit node.exe of Node.js 24 or later gets no fix, since nodejs.org builds none: the detailed information says so.
- The box “Allow the online test” is now named “Online checks and updates”: it allows the online test and the other connections of eMudScan itself, such as the search for the latest release of what a fix downloads. The About box now names the node.exe of a program among them.
- The window of the detailed information opens wider, so that most commands fit on one line: 1000 pixels at 100 % scaling instead of 850. The Fix window of one software opens a little wider too: 900 pixels instead of 860.
- The text of the detailed information and of the About box is justified, its lines are closer together, and its lists have square bullets. The detailed information scrolls smoothly, with the mouse wheel, the scroll bar and the keys. The text of the About box can now be selected and copied.
- The web search for a certificate bundle of a software that eMudScan does not know searches for the software alone, such as “example-app”, instead of the software with the name of the file, such as “example-app cacert.pem”. This applies to the link of the detailed information and to **Search on Google** in the right-click menu.
- The About box and the README say that eMudScan was made by 3c0 with the help of AI.

### Fixed

- Copying text with Ctrl+C, or selecting it all with Ctrl+A, in the log, the console of the Fix window or the detailed information no longer plays the default sound of Windows. The dialog manager took the character of these keys for the shortcut of a button, found none, and beeped.
- When eMudScan was started from a PowerShell 7 window, Windows PowerShell, which runs the fixes, got the module path of PowerShell 7 and took its modules for its own: `Get-FileHash` was then not found. A fix that checks a SHA-256 (a certificate bundle, Node.js, the ssl files of a Python) failed without changing anything, and the check of eMudScan’s own files, before a fix that needs some, was not made. The fixes now put the modules of the PowerShell that runs them first. The check of eMudScan’s own files no longer uses a command of a module, and stops the fixes when it cannot be made.
- In a narrow Fix window, the line drawn across the console at the end of the fixes no longer leaves empty rows under it: like the new line between two software, it takes a single row, whatever the width of the window.
- In the detailed information, the Fix windows and the log, a word too long for its line, such as a URL or a path, is cut after a slash, a backslash or a hyphen, instead of in the middle of a name such as x86_64.
- In the Fix software window, a click on a row scrolls the console so that the head of its software is at its top. A click on a copy of a software, such as one of two copies of Python inside the same program, did not scroll it at all.
- `scripts/build.ps1` now works in Windows PowerShell 5.1, where it stopped with “no Go files”: it no longer gives go an empty `-tags` option, which Windows PowerShell 5.1 does not pass.

## [3.1.2] - 2026-11-12

### Changed

- The detailed information of “Software copying the Windows root store” gives the result on this PC first, then says in a few words what such software is. It no longer reads as a check of Python only, and leaves the fixed versions of Python to the item of each Python found.

### Fixed

- The detailed information about the Windows roots, and about software that uses them and is not affected, now says only what is found on this PC. It no longer explains how such software works, names a version of Flutter, or advises not to delete the roots: that advice was outdated, since Microsoft removed the eMudhra roots from Windows.

## [3.1.1] - 2026-11-11

### Added

- When a certificate bundle is, byte for byte, a copy of the root store built into a program of its folder or of the folder above, the detailed information of both says so. The fix replaces the file, but the program still carries its outdated list and can write it again when the file is missing, for example after its update.
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

[unreleased]: https://github.com/3c0/eMudScan/compare/3.1.2...HEAD
[3.1.2]: https://github.com/3c0/eMudScan/compare/3.1.1...3.1.2
[3.1.1]: https://github.com/3c0/eMudScan/compare/3.1.0...3.1.1
[3.1.0]: https://github.com/3c0/eMudScan/compare/3.0.1...3.1.0
[3.0.1]: https://github.com/3c0/eMudScan/releases/tag/3.0.1
