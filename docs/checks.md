# What eMudScan checks

For how eMudScan checks each program that the advisory lists as fixed, see
[Software listed by the advisory](coverage.md).

## Windows certificate system

- The last update of the Windows block list (Microsoft added the eMudhra
  certificates on 20 October 2026), and whether a policy turns automatic updates
  off.
- How Windows judges each of the 16 compromised certificates: the 8 eMudhra
  intermediates and their 8 copies cross-signed by Sectigo.
- **Windows TLS test (offline)**, always: the advisory’s 12 test chains go
  through the checks Windows applies to a secure connection, without any network
  access. Each chain must be rejected.
- **Windows TLS test (online)**, when the online test is allowed: 3 HTTPS
  requests through Windows (WinHTTP) to the advisory’s test sites. If the sites
  cannot be reached, the result is *inconclusive*.
- **HTTPS inspection**: antivirus software and company proxies that inspect
  secure connections (Kaspersky, ESET, Zscaler…) are recognized by the root
  certificate they install. They validate certificates instead of Windows: only
  the online test can tell whether they reject the compromised ones.
- The eMudhra and Sectigo roots present (for information), and what software
  that copies the Windows root store without its block list (Python’s `ssl`
  module before its fixed versions) would accept.

## Browsers and internet apps

- **Browsers**, wherever they are on the scanned drives (installed, Scoop,
  portable). Official Chrome, Edge, Brave, Vivaldi, Opera, Firefox and
  Thunderbird receive their vendor’s block list. eMudScan reads the block list
  that a Chromium browser has downloaded (CRLSet), or else looks for the block
  list built into its recent versions, and says when the browser must be opened
  to get the current one. It also reads the block list included in
  Firefox-based browsers (OneCRL), such as Waterfox and Floorp. Other Chromium
  builds (Thorium…) are checked through their built-in root store.
- **Other internet apps** (Discord, Slack, Zoom, Dropbox…): Electron
  version, bundled Python, Node.js, Java or curl, certificate bundles, built-in
  root stores, and OpenSSL-based software that loads the Windows roots
  (BitTorrent clients built on libtorrent).
- **yt-dlp**, wherever it is on the scanned drives, by its version: 2026.10.26
  completes the fix that 2026.10.21 began, which its contents cannot show.

## Other software

Git for Windows, curl, Python (pip, certifi, every virtual environment),
Node.js, Java (including copies bundled with other software), Electron
applications, Go, Docker Desktop, GitHub CLI, programs with a built-in root
store (uv, Bun, Deno…), and every other certificate bundle (`.pem`, `.crt`…) on
the scanned drives.

Python 3.11.18, 3.12.16, 3.13.18, 3.14.10 and 3.15.2 reject the compromised
certificates in their `ssl` module itself (CVE-2026-115557), whatever
certificate bundle is used. eMudScan recognizes this from the version and from
the block list in the module’s library (`DLLs\_ssl.pyd`), which also covers
builds with other version numbers. Such a Python is green, and the outdated pip
or certifi bundles of its virtual environments are reported for information. An
older Python is amber, even with pip and certifi up to date, because the
connections of its standard library remain exposed through its copy of the
Windows roots.

Certificate bundles are not judged by their version: each one is tested against
the 12 test chains, so the result shows whether that file would really accept a
compromised certificate.

**Dart and Flutter:** on Windows, Dart copies the Windows certificate stores
into its own TLS library, without the Windows block list. The Dart SDK and the
Flutter SDK are checked by their version (Dart 3.13.7 and Flutter 3.47.8 or later
are fixed), or by the block list in `dart.exe`. An app built with Flutter is
checked by the block list in its engine (`flutter_windows.dll`), and without it,
is judged like other software that copies the Windows root store. The engine
copies in Flutter projects are not reported: they are rebuilt with the Flutter
SDK.

**JetBrains:** jetbrainsd, the background service of the JetBrains Toolbox App
and of the JetBrains IDEs, checks its connections with its own built-in list of
trusted certificates, which neither Windows nor the IDEs’ Java changes. eMudScan
checks the one installed for each user
(`%LOCALAPPDATA%\JetBrains\Daemon\bundles\current\jetbrainsd\bin\jetbrainsd.exe`):
version 0.9.9728 or later is fixed, and so is a version whose program holds the
fingerprints of the compromised certificates, which this version adds to it. The
Toolbox App or an IDE installs the newest version when it starts.

**Microsoft Edge WebView2:** software that displays web content with WebView2
uses the WebView2 Runtime of Windows, which eMudScan checks for the block list
built into its `msedge.dll`: once the runtime has it, each software is protected
the next time it starts. Software that ships its own copy of WebView2 instead is
shown with that copy, judged by the block list in its `msedge.dll`: only an
update of the software updates it.

Programs built with PyInstaller (such as mitmproxy’s standalone programs) keep
their certificate bundle compressed inside the program file. eMudScan reads it
there, in every program it checks for a built-in root store. When the Python
packed with it rejects the compromised certificates itself, an outdated bundle
is reported for information.

**Settings that point software to a certificate bundle** are checked the same
way, because such a bundle replaces the software’s own certificates, even
up-to-date ones: the environment variables `SSL_CERT_FILE`, `SSL_CERT_DIR`,
`CURL_CA_BUNDLE`, `REQUESTS_CA_BUNDLE`, `PIP_CERT`, `GIT_SSL_CAINFO`,
`NODE_EXTRA_CA_CERTS` (which adds certificates to those of Node.js) and
`NPM_CONFIG_CAFILE`; `cert` in pip.ini; `http.sslCAInfo` in Git’s global
configuration; and `cafile` in npm’s. eMudScan only reads them. A bundle that
such a setting names on a network share is read and reported too, but without
commands: the share’s administrators manage it.

When a component is a copy inside other software (a Python or a Java that a
program ships in its folder), eMudScan recommends updating that software, not
the copy.

Some programs store their data in a folder named after their identifier rather
than their name (`%LOCALAPPDATA%\io.example.app-1` would belong to Example
App 1). Such a folder is attributed to an installed program only when
exactly one program contains the identifier in its executable or, failing that,
has every word of the identifier in its name; otherwise, the folder keeps the
identifier as its name.

**Microsoft Store apps** are searched too: their folders cannot be listed, but
Windows records the folder of each app installed for you, and eMudScan reads
these folders like any other (Python from the Store, Electron applications…).
What is found there is shown under the app’s name, tagged *[Microsoft Store]*.
Libraries shared by apps and the apps built into Windows are skipped, and so are
programs installed in their own folder to which Windows gives an app identity
(Microsoft Edge): they are not Store apps.

Programs are searched for built-in root stores without being read in full.
**Scan again** reads a file again only if it has changed.

## Software date

The date the publisher gave to the software’s main program, taken from, in this
order:

1. the time stamp of its digital signature;
2. its modification date, when it was kept from the publisher’s package
   (archives, most installers);
3. the build date recorded by the compiler, when it is a real date;
4. otherwise, its modification date on this PC.

Firefox-based browsers without a signature time stamp use the BuildID in their
`application.ini`. Placeholder dates, such as the 1 January 2000 of reproducible
builds, are never shown.

## Package managers

Software installed with Scoop, WinGet or Chocolatey is recognized from its files.
Scoop’s `current` folder is a link to the folder of the installed version: what
is found through it is shown once, under the version’s folder. When an update is
needed, the details give the command (`scoop update …`, `winget upgrade --id …`,
`choco upgrade …`). The Microsoft Store updates its apps automatically; the
details explain how to get the update sooner. Windows protects the folders of
Store apps, so no manual fix is offered for them.

## Not covered

Linux distributions in WSL, Docker images and virtual machines: they have their
own root stores.
