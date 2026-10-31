# What eMudScan checks

## Windows certificate system

- The last update of Windows' list of untrusted certificates (Microsoft added the
  eMudhra certificates on 20 October 2026), and whether a policy turns automatic
  updates off.
- Windows' verdict on each of the 16 compromised certificates: the 8 eMudhra
  intermediates and their 8 Sectigo cross-signed copies.
- **Offline TLS test**, always: the advisory's 12 test chains go through the
  checks Windows applies to a secure connection, without any network access. Each
  one must be refused.
- **Windows TLS test (online)**, when the online test is allowed: 3 HTTPS requests through
  Windows (WinHTTP) to the advisory's test sites. If the sites cannot be reached,
  the result is *inconclusive*.
- **HTTPS inspection**: antivirus and company proxies that inspect secure
  connections (Kaspersky, ESET, Zscaler…) are recognised from the root certificate
  they install. They check certificates instead of Windows: only the online test
  can tell whether they refuse the compromised ones.
- The eMudhra and Sectigo roots present (for information), and what software that
  copies the Windows root store without its block list (Python's `ssl` module
  before its fixed versions) would accept.

## Browsers and internet apps

- **Browsers**, wherever they are on the scanned drives (installed, Scoop,
  portable). Official Chrome, Edge, Brave, Vivaldi, Opera, Firefox and Thunderbird
  receive their vendor's block list. eMudScan reads the block list a Chromium
  browser has downloaded (CRLSet) and says when the browser must be opened to get
  the current one. It reads the list that Firefox-based builds carry (OneCRL),
  such as LibreWolf and Tor Browser. Other Chromium builds (ungoogled-chromium,
  Thorium…) are checked through their built-in root store.
- **Other internet apps** (Signal, Discord, Slack, Teams, Dropbox…): Electron
  version, bundled Python, Node.js, Java or curl, certificate bundles, built-in
  root stores, and OpenSSL-based software that loads the Windows roots
  (qBittorrent).

## Other software

Git for Windows, curl, Python (pip, certifi, every virtual environment), Node.js,
Java (including copies bundled with other software), Electron applications, Go,
Docker Desktop, GitHub CLI, programs with a built-in certificate list (uv, Bun,
Deno…), and every other certificate bundle (`.pem`, `.crt`…) on the scanned
drives.

Python 3.11.18, 3.12.16, 3.13.18, 3.14.10 and 3.15.2 refuse the compromised
certificates in their `ssl` module itself (CVE-2026-115557), whatever
certificate list is used. eMudScan recognises this from the version and from
the block list in the module's library (`DLLs\_ssl.pyd`), which also covers
builds with other version numbers. Such a Python is green, and the outdated
pip or certifi lists of its virtual environments are reported as information.
An older Python is amber, even with pip and certifi up to date, because the
standard library's connections remain exposed through the Windows root copy.

Certificate bundles are not judged by their version: each one is tested against
the 12 test chains, so the result says whether that file would really accept a
compromised certificate.

**Settings that point software to a certificate file** are checked the same way,
because such a file replaces the software's own list, even an up-to-date one: the
environment variables `SSL_CERT_FILE`, `SSL_CERT_DIR`, `CURL_CA_BUNDLE`,
`REQUESTS_CA_BUNDLE`, `PIP_CERT`, `GIT_SSL_CAINFO`, `NODE_EXTRA_CA_CERTS` (which
adds to Node.js' list) and `NPM_CONFIG_CAFILE`; `cert` in pip.ini; `http.sslCAInfo`
in Git's global configuration; `cafile` in npm's. eMudScan only reads them.

When a component is a copy inside other software (Python inside ESPHome, Java
inside Stirling PDF…), eMudScan says to update that software, not the copy.

Some programs keep their data in a folder named after their identifier rather than
their name (`%LOCALAPPDATA%\io.esphome.builder` belongs to ESPHome Device Builder).
Such a folder is given to an installed program only when exactly one program
contains the identifier in its executable or, failing that, has every word of it in
its name; otherwise it keeps the identifier as its name.

**Microsoft Store apps** are searched too: their folders cannot be listed, but
Windows records the folder of each app installed for you, and eMudScan reads them
like any other (Python from the Store, Electron applications…). What is found there
is shown under the app's name, tagged *[Microsoft Store]*. Libraries shared by apps
and Windows' own apps are skipped, and so are programs installed in their own
folder that Windows gives an app identity (Microsoft Edge): they are not Store apps.

Programs are searched for built-in certificate lists without being read
entirely. *Scan again* reads a file again only if it has changed.

## Software date

The date the publisher gave to the software's main program, taken from, in this
order:

1. the time stamp of its digital signature;
2. its modified date, when it was kept from the publisher's package (archives,
   most installers);
3. the build date written by the compiler, when it is a real date;
4. otherwise, its modified date on this PC.

Firefox-based browsers without a signature time stamp use the BuildID of their
`application.ini`. Placeholder dates, such as the 1 January 2000 of reproducible
builds, are never shown.

## Package managers

Software installed with Scoop, WinGet or Chocolatey is recognised from its files.
Scoop's `current` folder is a link to the folder of the installed version: what is
found through it is shown once, under the version's folder.
When it needs an update, the details give the command (`scoop update …`,
`winget upgrade --id …`, `choco upgrade …`). The Microsoft Store updates its apps
by itself; the details say how to get the update sooner. Windows protects the
folders of Store apps, so no fix by hand is proposed for them.

## Not covered

Linux distributions in WSL, Docker images and virtual machines: they have their
own certificate lists.
