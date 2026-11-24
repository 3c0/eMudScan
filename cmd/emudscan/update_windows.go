//go:build windows

package main

// The install of an update of eMudScan.exe (update.go). A running program cannot be
// written over: a copy of this eMudScan, run from Temp, replaces eMudScan.exe once
// this one has closed. So the version that installs an update is the one that the
// user runs, and the new version only has to start. In four stages:
//
//  1. This eMudScan (startUpdate) makes a new folder of its own in Temp,
//     eMudScan-update-<random>, that only the user, SYSTEM and the administrators can
//     open. It writes into it the checked download, eMudScan-<version>.exe, and a
//     copy of itself, eMudScan.exe. It keeps the folder and both files open, so that
//     nothing can change, rename or delete them, and checks both files again through
//     their handles. It starts the copy with --update-install and a handle on itself
//     (inherited: not a process ID, which Windows can give to another process), and
//     closes.
//  2. The copy (updateInstall) keeps its own file and the download open in the same
//     way, checks the download again, and waits until this eMudScan has ended. Then
//     it replaces eMudScan.exe with the download (replaceFile): it writes a new file
//     in the folder of eMudScan.exe, under a random name that must not exist yet,
//     checks it, then renames it over eMudScan.exe through its own handle, in one
//     step: eMudScan.exe is never half written.
//  3. When that folder needs administrator rights, the copy starts itself as
//     administrator for the replacement only (--update-copy, updateCopy): one prompt.
//  4. The copy starts eMudScan.exe, not as administrator, with --update-done and a
//     handle on itself, and ends. The new version (updateDone) waits until the copy
//     has ended, then deletes the two files and their folder: by their names, which
//     it checks, never through a link, and never a whole tree.
//
// When the install fails, a message says why and gives the release page; the copy
// starts the old eMudScan.exe, unchanged, which deletes the files in the same way.
// Every version keeps --update-done as it is: the version that installs an update
// gives it to the next one.

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

var (
	procSetFileInformationByHandle = kernel32.NewProc("SetFileInformationByHandle")
	procConvertSDDL                = modadvapi32.NewProc("ConvertStringSecurityDescriptorToSecurityDescriptorW")
	procAllowSetForegroundWindow   = moduser32.NewProc("AllowSetForegroundWindow")
)

const (
	fileAttributeDirectory    = 0x10
	fileAttributeReparsePoint = 0x400
	fileFlagBackupSemantics   = 0x02000000
	fileFlagOpenReparsePoint  = 0x00200000
	accessDelete              = 0x00010000
	fileReadAttributes        = 0x80
	accessSynchronize         = 0x00100000
	errorSharingViolation     = 32
)

// updateDirRe is the name of the folder of an update, in Temp; updateFileRe, the
// name of a file in it: the copy of the version that installs, or the download;
// updateTmpRe, the name of the file that replaces eMudScan.exe, beside it, while it
// is written.
var (
	updateDirRe  = regexp.MustCompile(`^eMudScan-update-[0-9a-f]{16}$`)
	updateFileRe = regexp.MustCompile(`^eMudScan(-\d+\.\d+\.\d+)?\.exe$`)
	updateTmpRe  = regexp.MustCompile(`^\.eMudScan-update-[0-9a-f]{16}\.tmp$`)
)

// updateDirHeld and updateHeld are the handles that keep the folder of an update and
// its files from changing, until this process ends.
var (
	updateDirHeld syscall.Handle
	updateHeld    []syscall.Handle
)

// langCode is the language of the messages, given to the next stage.
func langCode() string {
	if Lang == 1 {
		return "fr"
	}
	return "en"
}

// randomHex is n random bytes in hexadecimal.
func randomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// startUpdate is stage 1: it writes data, the checked eMudScan.exe of r, and a copy of
// this eMudScan into a new folder in Temp, and starts the copy, which replaces this
// eMudScan.exe with data. The caller then closes. When it fails, it deletes what it
// wrote.
func startUpdate(r updateRelease, data []byte) (err error) {
	target, err := os.Executable()
	if err != nil {
		return errors.New(Tf("update.startFailed", err))
	}
	// this eMudScan's own file, kept from changing while it is copied: a running
	// program cannot be written to anyway
	self, h, err := readLocked(target)
	if err != nil {
		return errors.New(Tf("update.selfFailed", err))
	}
	defer syscall.CloseHandle(h)
	dir, err := makeUpdateDir()
	if err != nil {
		return errors.New(Tf("update.tempFailed", err))
	}
	defer func() {
		if err != nil {
			dropUpdateDir(dir)
		}
	}()
	if err := writeHeld(filepath.Join(dir, downloadName(r.version)), data, r.sha256); err != nil {
		return err
	}
	exe := filepath.Join(dir, updateExe)
	if err := writeHeld(exe, self, sumOf(self)); err != nil {
		return err
	}
	me, err := inheritableSelf()
	if err != nil {
		return errors.New(Tf("update.startFailed", err))
	}
	c := exec.Command(exe, "--update-install", langCode(), target, strconv.FormatUint(uint64(me), 10), r.version, r.sha256)
	if err := startStage(c, me); err != nil {
		syscall.CloseHandle(me)
		return errors.New(Tf("update.startFailed", err))
	}
	return nil
}

// downloadName is the name of the download of a version, in the folder of an update:
// "eMudScan-4.0.1.exe".
func downloadName(version string) string { return appName + "-" + version + ".exe" }

// writeHeld writes data into path, a new file, then reads it through a handle that
// keeps it from changing, and checks it against sum: the handle is kept until this
// process ends.
func writeHeld(path string, data []byte, sum string) error {
	if err := writeNew(path, data); err != nil {
		return errors.New(Tf("update.tempFailed", err))
	}
	got, h, err := readLocked(path)
	if err != nil {
		return errors.New(Tf("update.tempFailed", err))
	}
	updateHeld = append(updateHeld, h)
	if len(got) != len(data) || !sameSum(got, sum) {
		return errors.New(T("update.badDownload"))
	}
	return nil
}

// dropUpdateDir deletes the folder of an update that did not start, and its files.
func dropUpdateDir(dir string) {
	for _, h := range updateHeld {
		syscall.CloseHandle(h)
	}
	updateHeld = nil
	cleanUpdateDir(dir)
	if updateDirHeld != 0 {
		syscall.CloseHandle(updateDirHeld)
		updateDirHeld = 0
	}
	if p, err := syscall.UTF16PtrFromString(dir); err == nil {
		syscall.RemoveDirectory(p) // only when empty
	}
}

// startStage starts c, which inherits the handle h when it is not 0, and lets it come
// to the front: its message, the administrator prompt or the window of eMudScan.
func startStage(c *exec.Cmd, h syscall.Handle) error {
	if h != 0 {
		c.SysProcAttr = &syscall.SysProcAttr{AdditionalInheritedHandles: []syscall.Handle{h}}
	}
	if err := c.Start(); err != nil {
		return err
	}
	procAllowSetForegroundWindow.Call(uintptr(c.Process.Pid))
	return nil
}

// makeUpdateDir makes the folder of a download: in Temp, under a random name, made
// only if it does not exist yet, which only the user, SYSTEM and the administrators
// can open. It is kept open, so that it cannot be renamed or deleted, and it must not
// be a link.
func makeUpdateDir() (string, error) {
	token, err := syscall.OpenCurrentProcessToken()
	if err != nil {
		return "", err
	}
	user, err := token.GetTokenUser()
	token.Close()
	if err != nil {
		return "", err
	}
	sid, err := user.User.Sid.String()
	if err != nil {
		return "", err
	}
	// protected (no rule inherited from Temp): full control for the user, SYSTEM and
	// the administrators, given to what is made inside
	sddl, _ := syscall.UTF16PtrFromString("D:P(A;OICI;FA;;;" + sid + ")(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)")
	var sd uintptr
	if r, _, e := procConvertSDDL.Call(uintptr(unsafe.Pointer(sddl)), 1, uintptr(unsafe.Pointer(&sd)), 0); r == 0 {
		return "", e
	}
	defer procLocalFree.Call(sd)
	sa := syscall.SecurityAttributes{Length: uint32(unsafe.Sizeof(syscall.SecurityAttributes{})), SecurityDescriptor: sd}
	for range 5 {
		dir := filepath.Join(os.TempDir(), "eMudScan-update-"+randomHex(8))
		p, _ := syscall.UTF16PtrFromString(dir)
		if err = syscall.CreateDirectory(p, &sa); err == syscall.ERROR_ALREADY_EXISTS {
			continue
		}
		if err != nil {
			return "", err
		}
		h, err := openChecked(dir, syscall.GENERIC_READ, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE, true)
		if err != nil {
			return "", err
		}
		updateDirHeld = h
		return dir, nil
	}
	return "", err
}

// openChecked opens path, never through a link (a link itself is refused), as a
// folder when dir is set, else as a file.
func openChecked(path string, access, share uint32, dir bool) (syscall.Handle, error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	flags := uint32(fileFlagOpenReparsePoint)
	if dir {
		flags |= fileFlagBackupSemantics
	}
	h, err := syscall.CreateFile(p, access, share, nil, syscall.OPEN_EXISTING, flags, 0)
	if err != nil {
		return 0, err
	}
	var info syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(h, &info); err != nil {
		syscall.CloseHandle(h)
		return 0, err
	}
	if info.FileAttributes&fileAttributeReparsePoint != 0 || (info.FileAttributes&fileAttributeDirectory != 0) != dir {
		syscall.CloseHandle(h)
		return 0, errors.New(Tf("update.notPlain", path))
	}
	return h, nil
}

// writeNew writes data into path, a new file: it fails if path exists, a link too.
func writeNew(path string, data []byte) error {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	h, err := syscall.CreateFile(p, syscall.GENERIC_WRITE, 0, nil, syscall.CREATE_NEW, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return err
	}
	defer syscall.CloseHandle(h)
	if err := writeAll(h, data); err != nil {
		return err
	}
	return syscall.FlushFileBuffers(h)
}

func writeAll(h syscall.Handle, data []byte) error {
	for len(data) > 0 {
		var n uint32
		if err := syscall.WriteFile(h, data[:min(len(data), 1<<20)], &n, nil); err != nil {
			return err
		}
		data = data[n:]
	}
	return nil
}

func readAll(h syscall.Handle) ([]byte, error) {
	var out []byte
	buf := make([]byte, 1<<20)
	for {
		var n uint32
		if err := syscall.ReadFile(h, buf, &n, nil); err != nil {
			return nil, err
		}
		if n == 0 {
			return out, nil
		}
		out = append(out, buf[:n]...)
		if len(out) > updateMaxSize {
			return nil, errors.New(T("update.badDownload"))
		}
	}
}

// readLocked reads path, a file and not a link, through a handle that lets others
// read it, or run it, but not change, rename or delete it while it is open; it gives
// the handle, which keeps the file so.
func readLocked(path string) ([]byte, syscall.Handle, error) {
	h, err := openChecked(path, syscall.GENERIC_READ, syscall.FILE_SHARE_READ, false)
	if err != nil {
		return nil, 0, err
	}
	data, err := readAll(h)
	if err != nil {
		syscall.CloseHandle(h)
		return nil, 0, err
	}
	return data, h, nil
}

// inheritableSelf is a handle on this process that a process it starts inherits: it
// can wait for this one to end, which a process ID cannot do safely.
func inheritableSelf() (syscall.Handle, error) {
	cur, _ := syscall.GetCurrentProcess()
	var h syscall.Handle
	err := syscall.DuplicateHandle(cur, cur, cur, &h, accessSynchronize, true, 0)
	return h, err
}

// samePath tells whether a and b name the same path, as Windows compares them.
func samePath(a, b string) bool {
	long := func(p string) string {
		p = filepath.Clean(p)
		u, err := syscall.UTF16PtrFromString(p)
		if err != nil {
			return p
		}
		buf := make([]uint16, 1024)
		if n, err := syscall.GetLongPathName(u, &buf[0], uint32(len(buf))); err == nil && n > 0 && int(n) < len(buf) {
			return syscall.UTF16ToString(buf[:n])
		}
		return p
	}
	return strings.EqualFold(long(a), long(b))
}

// updateCommand runs a stage of an update, from the arguments it was started with;
// false when they are not those of a stage.
func updateCommand(args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch {
	case args[0] == "--update-install" && len(args) == 6:
		setLangFromCode(args[1])
		updateInstall(args[2], args[3], args[4], args[5])
	case args[0] == "--update-copy" && len(args) == 5:
		setLangFromCode(args[1])
		if !updateCopy(args[2], args[3], args[4]) {
			os.Exit(1)
		}
	default:
		return false
	}
	return true
}

// updateFail says that the update failed, and why.
func updateFail(err error) {
	msgBox(0, Tf("update.failed", reason(err), appURL+"/releases/tag/"+appVersion), 0x10|0x10000|0x40000 /* ICONERROR|SETFOREGROUND|TOPMOST */)
}

// updateFiles is where stages 2 and 3, the copy, start: it must be the eMudScan.exe
// of a folder of an update, in Temp. (Stage 3 may run as another user, an
// administrator, whose Temp is not that one: it does not check where the folder
// is.) It keeps the folder and its own file open, so that they cannot be renamed or
// replaced (stage 3 starts the same file), then reads the download of version,
// beside it, through a handle that keeps it from changing, and checks it. It gives
// the folder and the download.
func updateFiles(version, sum string, inTemp bool) (string, []byte, error) {
	self, err := os.Executable()
	if err != nil {
		return "", nil, err
	}
	dir := filepath.Dir(self)
	if !strings.EqualFold(filepath.Base(self), updateExe) || !updateDirRe.MatchString(filepath.Base(dir)) ||
		inTemp && !samePath(filepath.Dir(dir), os.TempDir()) || !updateTagRe.MatchString(version) {
		return "", nil, errors.New(T("update.notFromTemp"))
	}
	if updateDirHeld, err = openChecked(dir, syscall.GENERIC_READ, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE, true); err != nil {
		return "", nil, err
	}
	h, err := openChecked(self, syscall.GENERIC_READ, syscall.FILE_SHARE_READ, false)
	if err != nil {
		return "", nil, err
	}
	updateHeld = append(updateHeld, h)
	data, h, err := readLocked(filepath.Join(dir, downloadName(version)))
	if err != nil {
		return dir, nil, err
	}
	updateHeld = append(updateHeld, h)
	if !sameSum(data, sum) {
		return dir, nil, errors.New(T("update.badDownload"))
	}
	return dir, data, nil
}

// waitFor waits until the process whose handle (inherited) is given has ended, for at
// most ms milliseconds, and tells whether it has.
func waitFor(handle string, ms uint32) bool {
	h, err := strconv.ParseUint(handle, 10, 64)
	if err != nil {
		return false
	}
	ev, _ := syscall.WaitForSingleObject(syscall.Handle(h), ms)
	syscall.CloseHandle(syscall.Handle(h))
	return ev == syscall.WAIT_OBJECT_0
}

// updateInstall is stage 2, the copy, run from Temp: it replaces target, the
// eMudScan.exe of the eMudScan whose process the handle old is, with the download of
// version, whose SHA-256 is sum, then starts it; or, when it fails, starts the old
// target again.
func updateInstall(target, old, version, sum string) {
	dir, data, err := updateFiles(version, sum, true) // kept from changing from now on
	// the old eMudScan must have ended: its file cannot be replaced while it runs
	if !waitFor(old, 120000) {
		updateFail(errors.New(T("update.oldRunning")))
		return
	}
	from, terr := checkTarget(target)
	if terr != nil {
		updateFail(terr) // not eMudScan.exe: it is not started either
		return
	}
	if err == nil {
		err = replaceFile(target, data, sum)
		if errors.Is(err, syscall.ERROR_ACCESS_DENIED) { // a folder that needs administrator rights: this step only
			err = copyAsAdmin(target, version, sum)
		}
	}
	if err == nil {
		if got, h, e := readLocked(target); e != nil || !sameSum(got, sum) {
			err = errors.New(Tf("update.notReplaced", target, T("update.badDownload")))
		} else {
			syscall.CloseHandle(h)
		}
	}
	if err != nil {
		if !errors.Is(err, errShown) {
			updateFail(err)
		}
		from = "" // the old eMudScan.exe, unchanged, starts again
	}
	me, err := inheritableSelf()
	if err != nil {
		me = 0 // started all the same, but it cannot wait for this copy to end
	}
	c := exec.Command(target, "--update-done", langCode(), dir, strconv.FormatUint(uint64(me), 10), from)
	if err := startStage(c, me); err != nil && from != "" {
		updateFail(errors.New(Tf("update.startFailed", err)))
	}
}

// errShown is the error of a stage that has already said it.
var errShown = errors.New("shown")

// copyAsAdmin runs stage 3, the replacement of target, as administrator: one prompt.
func copyAsAdmin(target, version, sum string) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	params := strings.Join([]string{"--update-copy", langCode(), syscall.EscapeArg(target), version, sum}, " ")
	info := shellExecuteInfo{fMask: 0x40 /* SEE_MASK_NOCLOSEPROCESS */, lpVerb: utf16p("runas"), lpFile: utf16p(self),
		lpParameters: utf16p(params), nShow: 1 /* SW_SHOWNORMAL */}
	info.cbSize = uint32(unsafe.Sizeof(info))
	if r, _, e := procShellExecuteExW.Call(uintptr(unsafe.Pointer(&info))); r == 0 {
		if errno, ok := e.(syscall.Errno); ok && errno == errorCancelled {
			return errors.New(T("update.adminRefused"))
		}
		return e
	}
	defer syscall.CloseHandle(info.hProcess)
	syscall.WaitForSingleObject(info.hProcess, syscall.INFINITE)
	var code uint32
	syscall.GetExitCodeProcess(info.hProcess, &code)
	if code != 0 {
		return errShown // stage 3 said why
	}
	return nil
}

// updateCopy is stage 3, the copy as administrator: it replaces target with the
// download of version.
func updateCopy(target, version, sum string) bool {
	_, data, err := updateFiles(version, sum, false)
	if err == nil {
		_, err = checkTarget(target)
	}
	if err == nil {
		err = replaceFile(target, data, sum)
	}
	if err != nil {
		updateFail(err)
		return false
	}
	return true
}

// checkTarget checks target, the file to replace: a path in full, a file and not a
// link, of eMudScan.exe, the window program (its version resource: exeWindow). It
// gives its version.
func checkTarget(target string) (string, error) {
	if !filepath.IsAbs(target) {
		return "", errors.New(Tf("update.notEmudscan", target))
	}
	h, err := openChecked(target, fileReadAttributes, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE, false)
	if err != nil {
		return "", err
	}
	syscall.CloseHandle(h)
	fv, ok := getFileVersion(target)
	if !ok || fv.Name != appName || fv.Desc != exeWindow.description || len(fv.Fixed) < 3 {
		return "", errors.New(Tf("update.notEmudscan", target))
	}
	return fv.Fixed[:3].String(), nil
}

// replaceTries is how many times replaceFile tries to rename, every half second.
var replaceTries = 20

// replaceFile replaces target with data, whose SHA-256 is sum, in one step: it
// writes a new file in the folder of target, under a random name that must not exist
// yet, through a handle that no one else can open, checks it, then renames it over
// target through that handle. On a failure, it deletes it. A target that is in use
// (an antivirus that reads it) is tried again for 10 seconds.
func replaceFile(target string, data []byte, sum string) error {
	tmp := filepath.Join(filepath.Dir(target), "."+appName+"-update-"+randomHex(8)+".tmp")
	p, err := syscall.UTF16PtrFromString(tmp)
	if err != nil {
		return err
	}
	h, err := syscall.CreateFile(p, syscall.GENERIC_READ|syscall.GENERIC_WRITE|accessDelete, 0, nil, syscall.CREATE_NEW, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return err // ERROR_ACCESS_DENIED: a folder that needs administrator rights
	}
	defer syscall.CloseHandle(h)
	fail := func(err error) error {
		deleteByHandle(h)
		return errors.New(Tf("update.notReplaced", target, err))
	}
	if err := writeAll(h, data); err != nil {
		return fail(err)
	}
	if err := syscall.FlushFileBuffers(h); err != nil {
		return fail(err)
	}
	if _, err := syscall.Seek(h, 0, 0); err != nil {
		return fail(err)
	}
	if got, err := readAll(h); err != nil || !sameSum(got, sum) {
		return fail(errors.New(T("update.badDownload")))
	}
	for try := 1; ; try++ {
		err = renameByHandle(h, target)
		if err == nil || try >= replaceTries || !errors.Is(err, syscall.ERROR_ACCESS_DENIED) && !errors.Is(err, syscall.Errno(errorSharingViolation)) {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if err != nil {
		if errors.Is(err, syscall.ERROR_ACCESS_DENIED) || errors.Is(err, syscall.Errno(errorSharingViolation)) {
			return fail(errors.New(T("update.inUse")))
		}
		return fail(err)
	}
	syscall.FlushFileBuffers(h)
	return nil
}

// renameByHandle renames the file of h to path, which it replaces (FileRenameInfo).
func renameByHandle(h syscall.Handle, path string) error {
	name, err := syscall.UTF16FromString(path)
	if err != nil {
		return err
	}
	type renameInfo struct { // FILE_RENAME_INFO
		replace uint32
		root    syscall.Handle
		length  uint32
		name    [1]uint16
	}
	size := unsafe.Offsetof(renameInfo{}.name) + uintptr(len(name))*2
	buf := make([]uint64, (size+7)/8) // aligned as the structure
	info := (*renameInfo)(unsafe.Pointer(&buf[0]))
	info.replace = 1
	info.length = uint32((len(name) - 1) * 2) // in bytes, without the final zero
	copy(unsafe.Slice(&info.name[0], len(name)), name)
	if r, _, e := procSetFileInformationByHandle.Call(uintptr(h), 3 /* FileRenameInfo */, uintptr(unsafe.Pointer(info)), size); r == 0 {
		return e
	}
	return nil
}

// deleteByHandle deletes the file of h when h is closed (FileDispositionInfo).
func deleteByHandle(h syscall.Handle) {
	del := uint32(1)
	procSetFileInformationByHandle.Call(uintptr(h), 4 /* FileDispositionInfo */, uintptr(unsafe.Pointer(&del)), 4)
}

// updateDone is stage 4, the eMudScan that the copy started: it waits until the copy,
// whose process the handle stage is, has ended, then deletes the files of the folder
// of the update, dir, and the folder; and a temporary file of a replacement that did
// not end beside eMudScan.exe.
func updateDone(dir, stage string) {
	waitFor(stage, 60000)
	if updateDirRe.MatchString(filepath.Base(dir)) && samePath(filepath.Dir(dir), os.TempDir()) {
		if h, err := openChecked(dir, syscall.GENERIC_READ, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE, true); err == nil {
			cleanUpdateDir(dir)
			syscall.CloseHandle(h)
			if p, err := syscall.UTF16PtrFromString(dir); err == nil {
				syscall.RemoveDirectory(p) // only when empty
			}
		}
	}
	if self, err := os.Executable(); err == nil {
		left, _ := filepath.Glob(filepath.Join(filepath.Dir(self), "."+appName+"-update-*.tmp"))
		for _, f := range left {
			if updateTmpRe.MatchString(filepath.Base(f)) {
				deleteChecked(f)
			}
		}
	}
}

// cleanUpdateDir deletes the files of an update in dir, a folder that the caller
// keeps open: those named as the copy or a download, and not links.
func cleanUpdateDir(dir string) {
	files, _ := os.ReadDir(dir)
	for _, f := range files {
		if updateFileRe.MatchString(f.Name()) {
			deleteChecked(filepath.Join(dir, f.Name()))
		}
	}
}

// deleteChecked deletes path, a file and not a link.
func deleteChecked(path string) {
	if h, err := openChecked(path, accessDelete, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE, false); err == nil {
		deleteByHandle(h)
		syscall.CloseHandle(h)
	}
}
