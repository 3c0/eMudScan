# Changelog

All notable changes to eMudScan are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). eMudScan does not follow Semantic Versioning. Its versions are numbered MAJOR.MINOR.HOTFIX: a major version is a milestone; a minor version brings new checks, fixes and improvements; a hotfix version brings only an urgent fix to the latest version.

## [Unreleased]

### Added

- Checks jetbrainsd, the background service of the JetBrains Toolbox App and IDEs, which has its own list of trusted certificates: version 0.9.9728 or later is fixed.
- Checks the JetBrains IDEs and Gateway by their version, for their built-in browser, and names the Java runtime of an IDE or of the Toolbox App after it, with the version that fixes it.
- Updates an outdated Python 3.14 or later installed with the Python Install Manager, with that manager, when it is installed.

### Changed

- Shorter details for Python: one sentence for the problem, then the recommended action, which names the program a bundled Python belongs to.
- A finding in several places shows its explanation once, with the commands for each place.

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
