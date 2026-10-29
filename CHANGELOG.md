# Changelog

Versions: 1.0 is the first public version; 1.1 to 1.8 bring fixes and small improvements; 1.9.x are the betas of the "Fix all!" function, from 1.9.1. Each version has a date (YYYYMMDD), shown next to its number.

## Unreleased

- The file properties of eMudScan.exe (Details tab) describe eMudScan as its project page does.
- Tests refuse any Windows DLL loaded by name, so that the DLL preloading fixed in 1.1 cannot come back.
- Data folders named after an application identifier (`io.esphome.builder`) are attributed to their program (ESPHome Device Builder) when its executable contains the identifier or its name has all of its words, and only when exactly one program matches.
- The same finding in several places (two copies of Node.js inside LM Studio) is one element, whose details list every copy, and counts once in the summary.
- Settings that point software to a certificate file are checked: the environment variables SSL_CERT_FILE, SSL_CERT_DIR, CURL_CA_BUNDLE, REQUESTS_CA_BUNDLE, PIP_CERT, GIT_SSL_CAINFO, NODE_EXTRA_CA_CERTS and NPM_CONFIG_CAFILE, pip's cert, Git's http.sslCAInfo and npm's cafile. The file they point to is tested like any certificate bundle.
- Microsoft Store apps are searched like other software (Python from the Store, Electron applications…), from the folders Windows records for them; what is found there is shown under the app's name, tagged [Microsoft Store], with the Store's way to update it (Windows protects their folders, so no fix by hand is proposed).
- Keyboard shortcuts: F5 to scan (or scan again), Ctrl+S to save the report, Ctrl+Shift+C to copy it, Ctrl+Q to quit; the buttons' tooltips show them.
- A search box above the lists (Ctrl+F) keeps the rows whose Software or Component column contains every word typed. Esc clears it, and so do Scan and Scan again.
- The details of a Python inside another program (ESPHome Device Builder, LM Studio) give that program's update once, then what to do if you manage this Python yourself, without repeating it.
- Software installed with Scoop is shown once, under the folder of its version: Scoop's `current` folder is a link to it, and Node.js from Scoop was listed twice, once through each.
- Software installed by a recent version of Scoop got no homepage from Scoop. eMudScan now also reads Scoop's new file names, scoop-manifest.json and scoop-install.json.
- LM Studio showed version 0.4.26 on its Electron row and 0.4.26+4 on its other rows. All the parts of a program now show the version listed in Apps & features or by its package manager.
- The “Package” row of the details and of the report is renamed “Package webpage”: it links to the software's page at its package manager.
- The links of the details window and of the About box, and the (i) button, had no tooltip. Each one now says what clicking it does.
- Dragging the border between two panes far down shrank the Log and moved the panes above it. The border now stops when the lowest list reaches its least height, and the lists show whole rows again once it is released.
- In the details, the software's age was in black. It is now in amber, as in the lists, after a warning sign and with a capital letter (“⚠ This version is 15 months old”).

## 1.3 (20261028)

- Python 3.11.18, 3.12.16, 3.13.18, 3.14.10 and 3.15.2 refuse the compromised certificates in their ssl module itself (CVE-2026-115557), whatever certificate list is used: eMudScan recognises this from the version and from the block list in `_ssl.pyd` (which also covers conda, Store or embedded builds), and reports such a Python as green, with the outdated pip or certifi lists of its virtual environments as information. An older Python is now amber even with pip and certifi up to date, since a fix exists; 3.10 and older get none.
- The Windows check “Software copying the Windows root list” no longer says that no update changes this for Python's ssl module.
- The status “Problem”, given only when Windows itself still trusts the compromised certificates, is now called “Critical”, and the summary counts “x critical” instead of “x problems”.
- The text of Firefox-based browsers carrying the updated block list says that later versions carry it too.

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
