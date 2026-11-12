eMudScan checks whether Windows and the software installed on your PC still trust the eight eMudhra intermediate CA certificates compromised in October 2026, tells you what to do about each one, and runs the fixes you select.

**Download `eMudScan.exe` below and run it:** nothing to install. It runs without administrator rights; for a fix that needs them, Windows asks your permission. It is not code-signed, so Windows SmartScreen may show it as an unrecognized app: select **More info**, then **Run anyway**.

Administrators can scan from the command line with `eMudScan-cli.exe`, a console program that works alone: it writes a text, JSON or CSV report, and its exit code gives the worst status found. It never runs a fix.

✅ **`eMudScan.exe` tested clean** on @DATE@ by 13 antivirus engines, including Avast, Bitdefender and Kaspersky ([Jotti’s malware scan results](https://virusscan.jotti.org/en-US/filescanjob/JOTTI_SCAN_ID)).

✅ **`eMudScan-cli.exe` tested clean** on @DATE@ by 13 antivirus engines, including Avast, Bitdefender and Kaspersky ([Jotti’s malware scan results](https://virusscan.jotti.org/en-US/filescanjob/JOTTI_SCAN_ID_CLI)).

## In this version

@CHANGES@

Full list of changes: [changelog](https://github.com/@REPOSITORY@/blob/@VERSION@/CHANGELOG.md).

## Verify your download

- SHA-256 of `eMudScan.exe`: `@SHA256@`
- SHA-256 of `eMudScan-cli.exe`: `@SHA256_CLI@`
- Both built by GitHub from this version’s source code ([build log](@BUILD_LOG@)). To verify that a copy comes from this build, with the GitHub CLI: `gh attestation verify eMudScan.exe -R @REPOSITORY@` (or `eMudScan-cli.exe`)
- The build is reproducible: anyone can [rebuild this version](https://github.com/@REPOSITORY@/blob/main/docs/building.md#rebuilding-a-release) from its source code and get the same two files, byte for byte. So anyone can check that these files contain exactly the public source code and nothing else, without trusting GitHub or eMudScan’s author.
