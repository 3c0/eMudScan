# Privacy

## eMudScan changes nothing until you confirm a fix

A scan only reads. eMudScan saves no settings, writes nothing to the registry,
and needs no administrator rights. It creates no files, except a report you
choose to save and, when you run fixes, the files described below.

## What a fix writes

The Fix window shows the PowerShell commands that will run, and nothing runs
until you accept its warning (see [Fixing software](using.md#fixing-software)).
Then:

- the commands, and then what they show, are written to a new folder in your
  temporary folder (`%TEMP%\eMudScan-fix-…`), from which Windows PowerShell runs
  them; eMudScan deletes that folder once they have run;
- the commands change only what they show: they update software through its
  package manager, replace certificate bundles, and add the compromised
  certificates to Java block lists, after backing up the files they replace,
  next to them, unless you clear **Back up the certificate files**;
- **Restore the backup**, in the details window of a fixed software, moves these
  backups back in place of their files, once you accept its warning; a file that
  needs administrator rights is moved by the command of the details, in a Windows
  PowerShell started as for the fixes;
- package managers write what they usually write when they update software;
- the commands that need administrator rights run in a Windows PowerShell for
  which Windows asks your permission; eMudScan itself does not need them;
- at the end, the **Fix software** window saves the report in your Downloads
  folder.

## What a fix connects to

Fixes connect to the internet once you accept the warning, whether or not
**Allow the online test** is checked:

- an update through a package manager (Scoop, WinGet, Chocolatey, the Python
  Install Manager, pip or conda) connects to that package manager’s sources, as
  when you run it yourself;
- replacing a certificate bundle downloads Mozilla’s current bundle and its
  SHA-256 checksum from [curl.se](https://curl.se/docs/caextract.html), which
  sees your IP address and the User-Agent of Windows PowerShell, which names its
  version, your Windows version and your language.

Adding the compromised certificates to a Java block list connects to nothing.

## The online test is the only connection of a scan

When **Allow the online test** is checked (it is by default), each scan makes up
to 3 HTTPS requests through Windows (WinHTTP) to the advisory’s test sites at
[emudhra.we-have-a-problem.com](https://emudhra.we-have-a-problem.com), to check
that Windows rejects the compromised certificates on a real connection:

1. `probe-emudhra.we-have-a-problem.com`, whose certificate is valid, to check
   that the test sites can be reached (if not, the test stops there);
2. `dv-g3b-emudhra.we-have-a-problem.com`, whose certificate was issued by a
   compromised intermediate;
3. `dv-g3b-aaa-emudhra.we-have-a-problem.com`, the same through the copy
   cross-signed by Sectigo.

Each request is `GET /?client=emudscan`. It carries no cookies and follows no
redirects, and eMudScan reads at most the first 4 KB of the response, only to
recognize the site’s own page. These requests disclose the following to that
third-party site:

- your IP address;
- the request address, which names eMudScan (`client=emudscan`);
- a User-Agent that describes eMudScan and your Windows, for example:
  `eMudScan/2.0.0 (20261105; Windows 11 Pro 25H2 26200.1234; +https://github.com/3c0/eMudScan)`

The site’s author sees all three, and the User-Agent may appear verbatim in the
site’s [public statistics](https://statistics-emudhra.we-have-a-problem.com/).

The requests go through the proxy configured in Windows, if any (Settings ›
Network & internet › Proxy). When **Automatically detect settings** is turned on
there, Windows may first look for the network’s proxy settings (WPAD) on the
local network, as it does for its own connections. A proxy sees the same three
things. Your Windows credentials are not sent to the test sites: WinHTTP’s
default policy sends them to intranet servers only, and eMudScan does not change
it.

## Opting out

Clear **Allow the online test**, or start `eMudScan.exe --offline`: a scan then
makes no internet connection at all, not even for proxy detection. The online
test is skipped; every other check works the same. The choice is not saved. The
fixes that you run still connect, as described above.

## Everything else stays on your PC

The Windows certificate checks run without network access and without updating
the Windows certificate lists. Apart from the online test and the fixes, nothing
is sent anywhere. The only files
read over your network are those on the drives you select to scan, and a
certificate bundle on a network share that one of your certificate settings
names, as the software that uses that setting reads it. Links in the
details windows (homepages, web searches, test pages) open in your browser only
when you click them.
