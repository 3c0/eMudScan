package main

import (
	"os"
	"syscall"
)

// fileCreated is the creation date of a file, as Windows keeps it; 0 when it has none.
func fileCreated(path string) int64 {
	fi, err := os.Stat(path)
	if err != nil {
		return 0
	}
	if d, ok := fi.Sys().(*syscall.Win32FileAttributeData); ok {
		return d.CreationTime.Nanoseconds()
	}
	return 0
}
