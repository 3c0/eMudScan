//go:build !cli

package main

// eMudScan.exe, the window program: this code built without the tag "cli" and as a
// window program (scripts/build.sh, build.ps1). See main_cli_windows.go for
// eMudScan-cli.exe.

import _ "github.com/3c0/eMudScan/cmd/emudscan/winres/gui" // the Windows resources of eMudScan.exe

func main() { guiMain() }
