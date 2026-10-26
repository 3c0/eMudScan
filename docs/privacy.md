# Privacy

## eMudScan writes nothing

eMudScan only reads. It saves no settings, writes nothing in the registry and
creates no file, except a report you choose to save. It needs no administrator
rights.

## The online test is its only connection

When *I accept the online test* is ticked (it is at start), each scan connects
3 times to the advisory's test sites at
[emudhra.we-have-a-problem.com](https://emudhra.we-have-a-problem.com), to check
that Windows refuses the compromised certificates on a real connection.

These connections disclose to that third-party site:

- your IP address;
- a User-Agent that describes eMudScan and your Windows, for example:
  `eMudScan/1.0 (20261026; Windows 11 Pro 25H2 26200.9711; +https://github.com/3c0/eMudScan)`

The site's author sees both, and the User-Agent may appear in the site's public
statistics.

## Opting out

Untick *I accept the online test*, or start `eMudScan.exe --offline`: eMudScan
then makes no internet connection at all. The live test is skipped; every other
check works the same. The choice is not saved.

## Everything else stays on your PC

Windows' certificate checks run without network access and without updating
Windows' certificate lists. Nothing else is sent anywhere. Links in the details
windows (homepages, web searches, test pages) open in your browser only when you
click them.
