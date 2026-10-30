# Privacy

## eMudScan writes nothing

eMudScan only reads. It saves no settings, writes nothing in the registry and
creates no file, except a report you choose to save. It needs no administrator
rights.

## The online test is its only connection

When *I accept the online test* is ticked (it is at start), each scan makes up to
3 HTTPS requests through Windows (WinHTTP) to the advisory's test sites at
[emudhra.we-have-a-problem.com](https://emudhra.we-have-a-problem.com), to check
that Windows refuses the compromised certificates on a real connection:

1. `probe-emudhra.we-have-a-problem.com`, whose certificate is valid, to check
   that the test sites can be reached (if not, the test stops there);
2. `dv-g3b-emudhra.we-have-a-problem.com`, whose certificate comes from a
   compromised intermediate;
3. `dv-g3b-aaa-emudhra.we-have-a-problem.com`, the same through the Sectigo
   cross-signed copy.

Each request is `GET /?client=emudscan`. It carries no cookie, follows no
redirection, and eMudScan reads at most the first 4 KB of the answer, only to
recognise the site's own page. These requests disclose to that third-party site:

- your IP address;
- the request's address, which names eMudScan (`client=emudscan`);
- a User-Agent that describes eMudScan and your Windows, for example:
  `eMudScan/1.5 (20261030; Windows 11 Pro 25H2 26200.9711; +https://github.com/3c0/eMudScan)`

The site's author sees all three, and the User-Agent may appear in the site's
public statistics.

The requests go through the proxy configured in Windows, if there is one
(Settings › Network & Internet › Proxy). When *Automatically detect settings* is
on there, Windows may first look for the network's proxy settings (WPAD) on the
local network, as it does for its own connections. A proxy sees the same three
things; Windows' credentials are not sent to the test sites (WinHTTP's default
policy sends them to intranet servers only, which eMudScan does not change).

## Opting out

Untick *I accept the online test*, or start `eMudScan.exe --offline`: eMudScan
then makes no internet connection at all, not even the proxy detection. The live
test is skipped; every other check works the same. The choice is not saved.

## Everything else stays on your PC

Windows' certificate checks run without network access and without updating
Windows' certificate lists. Nothing else is sent anywhere. Links in the details
windows (homepages, web searches, test pages) open in your browser only when you
click them.
