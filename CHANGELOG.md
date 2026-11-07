# Changelog

All notable changes to eMudScan are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). eMudScan does not follow Semantic Versioning. Its versions are numbered MAJOR.MINOR.HOTFIX: a major version is a milestone; a minor version brings new checks, fixes and improvements; a hotfix version brings only an urgent fix to the latest version.

## [Unreleased]

### Added

- Checks jetbrainsd, the background service of the JetBrains Toolbox App and IDEs, which has its own list of trusted certificates: version 0.9.9728 or later is fixed.
- Checks the JetBrains IDEs and Gateway by their version, for their built-in browser, and names the Java runtime of an IDE or of the Toolbox App after it, with the version that fixes it.
- Updates an outdated Python 3.14 or later installed with the Python Install Manager, with that manager, when it is installed.
- Upgrades an outdated Python installed by uv with uv, after updating uv itself as it was installed (Scoop, WinGet, Chocolatey, its standalone installer or pipx), then removes the old copy once a fixed version is installed, naming the virtual environments found that use that copy, also in the warning before the fix runs.
- Can replace the node.exe inside a program with the latest release of its Node.js release line from nodejs.org, once checked (its published SHA-256, its OpenJS Foundation signature and its version), after backing it up: a fix that is not ticked by default, since it changes a program that its publisher has not updated yet.
- Can replace the ssl files of a 64-bit Python 3.11 to 3.15 inside a program with those of the fixed release of its line, from python.org’s embeddable package or python-build-standalone’s build (on GitHub), once checked against their SHA-256. Before and after, that Python runs eMudScan’s tests (its ssl and hashlib modules, the Windows roots, TLS 1.2 and 1.3 connections in memory, the rejection of a compromised chain); if one that passed before fails after, the original files are put back. A fix that is not ticked by default: a software that also has other fixes gets a second row for it in the Fix window. **Restore the backup** puts the original files back and removes those the fix added, all together, only when all the backups and marks of that fix are there, even after a copy that a crash interrupted.
- Updates a Deno installed with its own installer with `deno upgrade`, Deno’s own updater (Deno 2.9.8 is fixed).
- For the virtual environment of a Pipenv, Poetry or uv project whose lock file pins the outdated certifi, which the project’s next install would bring back, the details give the command that updates it in the project, which eMudScan never runs.
- In the Fix software window, a double-click on a software opens its details, where it can also be fixed or restored; the list is updated when they close.

### Changed

- Shorter details for Python: one sentence for the problem, then the recommended action, which names the program a bundled Python belongs to.
- A finding in several places shows its explanation once, with the commands for each place.
- “Back up the certificate files”, in the Fix window, is now “Back up the files to change”, since it also covers node.exe and the ssl files of a Python.
- The details no longer suggest updating everything installed with a package manager (`scoop update -a`, `winget upgrade --all`, `choco upgrade all`): eMudScan stays targeted, with the update of the software it found only.
- The Fix window opened from the details no longer has a **Stop** button: it fixes one software, whose commands always run to their end.
- The About box, like the README, says that eMudScan is intended for administrators, developers and advanced users.

### Fixed

- The Fix window no longer shows “Back up the files to change” when no fix to run backs up a file, such as an update with pip.
- No pip command where it cannot work: for a Python or virtual environment where pip is not installed, or for a Python marked as managed by the tool that installed it (EXTERNALLY-MANAGED, as uv does), which pip refuses to change.
- No Scoop update command for a file in Scoop’s `persist` folder, which Scoop’s updates keep as it is.
- The packages of a tool that uv installed are updated with `uv tool upgrade`, as uv asks, instead of pip, which uv does not put in its tools.
- The packages of an application that pipx installed are updated with `pipx runpip`, which works whether or not its environment has pip of its own.
- A file put back by **Restore the backup** has its original creation date again: eMudScan’s backups now keep it, where Windows gave them the date of the backup.
- The summary of the certificate bundles is updated after fixes and restores: the files it lists are tested again, instead of keeping the results of the scan.

## [2.0.0] - 2026-11-05

First public version.

### Added

- Checks that Windows blocks the 16 compromised certificates (the 8 eMudhra intermediates and their copies cross-signed by Sectigo), offline and, if you allow it, with an online test against the advisory’s test sites, and whether an antivirus or a proxy inspects secure connections.
- Checks browsers and internet apps (messaging, email, sync, VPN, remote access) by the block lists they receive or carry, including Microsoft Edge and the WebView2 that software uses or ships.
- Checks other software: every certificate bundle on the scanned drives (certifi, pip, Git, curl…), Python, Node.js, Java, Go, Electron applications, programs built with PyInstaller and programs with their own root store.
- Says what to do about each one, with the commands to run, and flags software that needs an update and is a year old or more.
- Fixes what it can, once you confirm: an update through the package manager that installed the software (Scoop, WinGet, Chocolatey, pip or conda), Mozilla’s current certificate bundle in place of an outdated one, or the compromised certificates added to a Java block list. The certificate files are backed up first, and can be put back from the details window; each software is checked again, and a report is saved in your Downloads folder.
- A report to copy or save. English and French.
- Changes nothing on your PC until you confirm a fix. Clear the online test, and a scan makes no internet connection at all.

[unreleased]: https://github.com/3c0/eMudScan/compare/2.0.0...HEAD
[2.0.0]: https://github.com/3c0/eMudScan/releases/tag/2.0.0
