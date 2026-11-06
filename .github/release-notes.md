eMudScan checks whether Windows and the software installed on your PC still trust the eight eMudhra intermediate CA certificates compromised in October 2026, tells you what to do about each one, and fixes what it can, once you confirm.

**Download `eMudScan.exe` below and run it:** nothing to install. It runs without administrator rights; for a fix that needs them, Windows asks your permission. It is not code-signed, so Windows SmartScreen may show it as an unrecognized app: select **More info**, then **Run anyway**.

<!-- Before publishing: download eMudScan.exe from this draft, scan it on https://virusscan.jotti.org/en-US, then replace this comment with this line, with the number of engines and the link to the results:
✅ **Tested clean** on the release date by 13 antivirus engines, including Avast, Bitdefender and Kaspersky ([Jotti’s results](https://virusscan.jotti.org/en-US/filescanjob/...)).
-->

## In this version

@CHANGES@

## Verify your download

- SHA-256 of `eMudScan.exe`: `@SHA256@`
- Built by GitHub from this version’s source code ([build log](@BUILD_LOG@)). To verify that a copy comes from this build, with the GitHub CLI: `gh attestation verify eMudScan.exe -R @REPOSITORY@`
