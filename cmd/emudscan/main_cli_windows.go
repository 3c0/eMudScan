//go:build cli

package main

// eMudScan-cli.exe, the console program that scans without the window: this code
// built with the tag "cli" and as a console program (scripts/build.sh, build.ps1).
// A shell waits for a console program, gives it its outputs and gets its exit code,
// which it does not do for a window program such as eMudScan.exe. The scan itself is
// in cliscan.go.

import (
	"io"
	"os"
	"runtime"
	"syscall"

	_ "github.com/3c0/eMudScan/cmd/emudscan/winres/cli" // the Windows resources of eMudScan-cli.exe
)

func main() { os.Exit(cliMain(os.Args[1:])) }

// cliStream is where a standard output is written: in a console, as Go writes there
// (in Unicode) with the layout of consoleText; in a file or a pipe, as it is, in
// UTF-8; nowhere when the program that started eMudScan-cli.exe gave it no output.
func cliStream(f *os.File) io.Writer {
	if f == nil || f.Fd() == 0 || syscall.Handle(f.Fd()) == syscall.InvalidHandle {
		return io.Discard
	}
	var mode uint32
	if syscall.GetConsoleMode(syscall.Handle(f.Fd()), &mode) == nil {
		return consoleWriter{f}
	}
	return f
}

// cliMain runs eMudScan-cli.exe with the arguments args and returns its exit code. It
// shows no window and no message box: nobody may be there to close it.
func cliMain(args []string) int {
	runtime.LockOSThread()
	stdout, stderr := cliStream(os.Stdout), cliStream(os.Stderr)
	o, err := parseArgs(args)
	setLangFromCode(userUILanguage())
	if o.lang >= 0 {
		Lang = o.lang
	}
	switch {
	case o.help:
		io.WriteString(stdout, usage())
		return exitOK
	case err != nil:
		return scanFailed(stderr, err.Error()+"\r\n"+T("cli.seeHelp"))
	}
	if major, build := windowsBuild(); !windowsRecentEnough(major, build) {
		return scanFailed(stderr, Tf("err.oldWindows", appVersion, olderWindows()))
	}
	pCoInitializeEx.Call(0, 2) // COINIT_APARTMENTTHREADED, for the shell functions
	if err := loadData(); err != nil {
		return scanFailed(stderr, Tf("err.data", err))
	}
	return runScan(o, stdout, stderr)
}
