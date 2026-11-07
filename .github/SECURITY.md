# Security policy

eMudScan is a security tool: please report vulnerabilities privately, not in a public issue.

## What to report

For example:

- a check that reports software as OK while it still trusts the compromised certificates;
- eMudScan changing anything on the PC before the user accepts the warning of a fix, or a fix running commands other than those it shows;
- a connection made by eMudScan other than the online test, or one made while the online test is not allowed;
- a release file that does not match its SHA-256 or its build attestation.

Report problems with the data or test sites of we-have-a-problem.com to [its repository](https://github.com/galadran/we-have-a-problem.com).

## How to report

Click **Report a vulnerability** on the [Security tab](https://github.com/3c0/eMudScan/security) of this project: only the maintainer sees the report. Please include the eMudScan version (shown in its window and its About box), your Windows version, and the steps to reproduce the problem.

## Supported versions

Only the latest version receives security fixes. They are published as a new version: a minor version (2.1.0) or a hotfix version (2.0.1).
