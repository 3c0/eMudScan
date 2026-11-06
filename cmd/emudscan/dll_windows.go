package main

import (
	"os"
	"syscall"
	"unsafe"
)

// Windows looks for a DLL loaded by name in the program's folder first: a DLL
// named like a system one and placed next to eMudScan.exe (in the Downloads
// folder, for example) would be loaded instead of it ("DLL preloading"). So
// eMudScan loads its DLLs by their full path in System32 (sysDLL), and tells
// Windows to look only in System32 for the DLLs it loads on eMudScan's behalf.
//
// comctl32.dll is the exception: it is loaded by name, so that the manifest
// selects version 6 (visual styles) from WinSxS, which Windows does before
// looking in any folder.

// kernel32 is always the system's copy (a "known DLL"), already loaded.
var kernel32 = syscall.NewLazyDLL("kernel32.dll")

// systemDir is Windows' System32 folder.
var systemDir = func() string {
	buf := make([]uint16, 512)
	n, _, _ := kernel32.NewProc("GetSystemDirectoryW").Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n == 0 || int(n) >= len(buf) {
		return os.Getenv("SystemRoot") + `\System32`
	}
	return syscall.UTF16ToString(buf[:n])
}()

// sysDLL returns a DLL of Windows, loaded from System32 when first used.
func sysDLL(name string) *syscall.LazyDLL {
	return syscall.NewLazyDLL(systemDir + `\` + name)
}

func init() {
	const loadLibrarySearchSystem32 = 0x00000800
	kernel32.NewProc("SetDefaultDllDirectories").Call(loadLibrarySearchSystem32)
}
