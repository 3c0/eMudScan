# Changelog

Versions: 1.0 is the first public version; 1.1 to 1.8 bring fixes and small improvements; 1.9.x are the betas of the "Fix all!" function, from 1.9.1. Each version has a date (YYYYMMDD), shown next to its number.

## 1.2 (20261028)

- The online test's requests name eMudScan (`?client=emudscan`); they still used an earlier name of the project.
- The online test's requests follow no redirection and send no cookie: each answer is the test site's own, and nothing more than the request is sent.
- A file that eMudScan's readers fail on (a bug of eMudScan, on a file they did not expect) is skipped with a line in the log, instead of closing eMudScan without a word.

## 1.1 (20261027)

- Safer loading of Windows' DLLs: eMudScan loads them only from System32, so a DLL with the same name placed next to eMudScan.exe (in the Downloads folder, for example) can no longer be loaded instead.
- Character references above U+10FFFF in Chocolatey's .nuspec files are ignored.

## 1.0 (20261026)

First public version.

- Checks that Windows blocks the 16 compromised certificates (the 8 eMudhra intermediates and their Sectigo cross-signed copies), offline and, if you accept, with a live test against the advisory's test sites.
- Detects antivirus and proxies that inspect secure connections.
- Checks browsers, messaging, mail, sync, VPN and remote-access software.
- Checks other software: certificate files (certifi, pip, Git, curl…), Python, Node.js, Java, Go, Electron applications and programs with their own certificate list.
- Says what to do for each one, with the commands to run (eMudScan never runs them), and flags software that needs an update and is a year old or more.
- Report to copy or save. English and French.
- Read-only: eMudScan changes nothing on your PC. Untick the online test and it makes no internet connection at all.
