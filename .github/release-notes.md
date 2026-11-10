eMudScan checks whether Windows and the software installed on your PC still trust the eight eMudhra intermediate CA certificates compromised in October 2026, tells you what to do about each one, and runs the fixes you select.

**Download `eMudScan.exe` below and run it:** nothing to install. It runs without administrator rights; for a fix that needs them, Windows asks your permission. It is not code-signed, so Windows SmartScreen may show it as an unrecognized app: select **More info**, then **Run anyway**.

✅ **Tested clean** on @DATE@ by 13 antivirus engines, including Avast, Bitdefender and Kaspersky ([Jotti’s malware scan results](https://virusscan.jotti.org/en-US/filescanjob/JOTTI_SCAN_ID)).

## In this version

@CHANGES@

## Verify your download

- SHA-256 of `eMudScan.exe`: `@SHA256@`
- Built by GitHub from this version’s source code ([build log](@BUILD_LOG@)). To verify that a copy comes from this build, with the GitHub CLI: `gh attestation verify eMudScan.exe -R @REPOSITORY@`
