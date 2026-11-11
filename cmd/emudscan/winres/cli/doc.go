// Package cli holds the Windows resources of eMudScan-cli.exe, the console program:
// the same icon and manifest as eMudScan.exe, and its own version information, in
// rsrc_windows_amd64.syso, which Go links into a program that imports this package.
// They are generated from the source (cmd/emudscan/mkrsrc.go):
//
//	go run ./cmd/emudscan --make-syso-cli cmd/emudscan/winres/cli/rsrc_windows_amd64.syso
package cli
