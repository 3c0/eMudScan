# Privacy

## eMudScan writes nothing

eMudScan only reads. It saves no settings, writes nothing to the registry, and
creates no files, except a report you choose to save. It needs no administrator
rights. The commands shown in its details windows are run only by you.

## The online test is its only connection

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
  `eMudScan/1.7 (20261031; Windows 11 Pro 25H2 26200.9711; +https://github.com/3c0/eMudScan)`

The site’s author sees all three, and the User-Agent may appear in the site’s
public statistics.

The requests go through the proxy configured in Windows, if any (Settings ›
Network & internet › Proxy). When **Automatically detect settings** is turned on
there, Windows may first look for the network’s proxy settings (WPAD) on the
local network, as it does for its own connections. A proxy sees the same three
things. Your Windows credentials are not sent to the test sites: WinHTTP’s
default policy sends them to intranet servers only, and eMudScan does not change
it.

## Opting out

Clear **Allow the online test**, or start `eMudScan.exe --offline`: eMudScan
then makes no internet connection at all, not even for proxy detection. The
online test is skipped; every other check works the same. The choice is not
saved.

## Everything else stays on your PC

The Windows certificate checks run without network access and without updating
the Windows certificate lists. Nothing else is sent anywhere. Links in the
details windows (homepages, web searches, test pages) open in your browser only
when you click them.
