package main

// Running fixes in Windows PowerShell (see fixrun.go): the one in System32, without
// the user's profile scripts, hidden; as administrator through the Windows prompt
// (ShellExecuteEx "runas"), which names Windows PowerShell and Microsoft.

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

var procShellExecuteExW = modshell32w.NewProc("ShellExecuteExW")

// fixPromptOwner is the window that the administrator prompt belongs to;
// fixPromptClosed is called once the prompt is answered (see fixDlg.toFront).
var (
	fixPromptOwner  uintptr
	fixPromptClosed = func() {}
)

// shellExecuteInfo is SHELLEXECUTEINFOW.
type shellExecuteInfo struct {
	cbSize       uint32
	fMask        uint32
	hwnd         uintptr
	lpVerb       *uint16
	lpFile       *uint16
	lpParameters *uint16
	lpDirectory  *uint16
	nShow        int32
	hInstApp     uintptr
	lpIDList     uintptr
	lpClass      *uint16
	hkeyClass    uintptr
	dwHotKey     uint32
	hIcon        uintptr
	hProcess     syscall.Handle
}

const errorCancelled = 1223 // the user refused the administrator prompt

func runFixes(units []fixUnit, admin bool, out func(fixLine)) ([]unitResult, error) {
	dir, err := os.MkdirTemp("", "eMudScan-fix-")
	if err != nil {
		return nil, fmt.Errorf(T("fix.noPowerShell"), err)
	}
	defer os.RemoveAll(dir)
	logPath, scriptPath := filepath.Join(dir, "fix.log"), filepath.Join(dir, "fix.ps1")
	marker := fixMarker()
	files, err := fixFiles(units, time.Now())
	if err != nil {
		return nil, fmt.Errorf(T("fix.noPowerShell"), err)
	}
	for n, b := range files { // next to the script, which checks them
		if err := os.WriteFile(filepath.Join(dir, n), b, 0o600); err != nil {
			return nil, fmt.Errorf(T("fix.noPowerShell"), err)
		}
	}
	script := []byte(fixScript(units, logPath, marker, files))
	if err := os.WriteFile(scriptPath, script, 0o600); err != nil {
		return nil, fmt.Errorf(T("fix.noPowerShell"), err)
	}
	if err := os.WriteFile(logPath, nil, 0o600); err != nil {
		return nil, fmt.Errorf(T("fix.noPowerShell"), err)
	}
	ps := filepath.Join(systemDir, "WindowsPowerShell", "v1.0", "powershell.exe")
	args := []string{"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-EncodedCommand", encodedCommand(fixBootstrap(scriptPath, script))}

	var done func() (bool, uint32) // whether PowerShell has ended, and its exit code
	if admin {
		verb, file, params := utf16p("runas"), utf16p(ps), utf16p(strings.Join(args, " "))
		info := shellExecuteInfo{fMask: 0x40 /* SEE_MASK_NOCLOSEPROCESS */, hwnd: fixPromptOwner,
			lpVerb: verb, lpFile: file, lpParameters: params, nShow: 0 /* SW_HIDE */}
		info.cbSize = uint32(unsafe.Sizeof(info))
		r, _, e := procShellExecuteExW.Call(uintptr(unsafe.Pointer(&info)))
		fixPromptClosed()
		if r == 0 {
			if errno, ok := e.(syscall.Errno); ok && errno == errorCancelled {
				return nil, errors.New(T("fix.adminRefused"))
			}
			return nil, fmt.Errorf(T("fix.noPowerShell"), e)
		}
		h := info.hProcess
		defer syscall.CloseHandle(h)
		done = func() (bool, uint32) {
			if ev, _ := syscall.WaitForSingleObject(h, 0); ev != syscall.WAIT_OBJECT_0 {
				return false, 0
			}
			var code uint32
			syscall.GetExitCodeProcess(h, &code)
			return true, code
		}
	} else {
		c := exec.Command(ps, args...)
		c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000 /* CREATE_NO_WINDOW */}
		if err := c.Start(); err != nil {
			return nil, fmt.Errorf(T("fix.noPowerShell"), err)
		}
		exited := make(chan struct{})
		go func() { c.Wait(); close(exited) }()
		done = func() (bool, uint32) {
			select {
			case <-exited:
				return true, uint32(c.ProcessState.ExitCode())
			default:
				return false, 0
			}
		}
	}

	// the console follows the log as the commands write it
	log := newFixLog(units, marker, out, admin) // as administrator, the software of each command is named
	f, err := os.Open(logPath)
	if err != nil {
		return nil, fmt.Errorf(T("fix.noPowerShell"), err)
	}
	defer f.Close()
	buf := make([]byte, 64<<10)
	read := func() {
		for {
			n, err := f.Read(buf)
			if n > 0 {
				log.write(string(buf[:n]))
			}
			if err == io.EOF || n == 0 {
				return
			}
		}
	}
	for {
		ended, code := done()
		read()
		if ended {
			log.close()
			if code == fixTampered {
				return log.results, errors.New(T("fix.tampered"))
			}
			return log.results, nil
		}
		time.Sleep(150 * time.Millisecond)
	}
}

func utf16p(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(s)
	return p
}
