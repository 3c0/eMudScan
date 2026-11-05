package main

// The build information that the Go linker writes into the programs it makes: the
// Go version and the modules. debug/buildinfo reads it too, but it would add about
// 300 KB to eMudScan (it also reads ELF and Mach-O files and compressed sections),
// for the few lines below.

import (
	"bytes"
	"encoding/binary"
	"os"
	"strings"
)

var goBuildInfoMagic = []byte("\xff Go buildinf:")

// goBuildInfo returns the Go version and the module information of a Windows
// program built with Go 1.18 or later.
func goBuildInfo(path string) (goVersion, modInfo string, ok bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", "", false
	}
	defer f.Close()
	h, ok := readPEHeaders(f)
	if !ok {
		return "", "", false
	}
	// the blob is near the start of the first writable data section
	const dataChars = 0x40 | 0x40000000 | 0x80000000 // INITIALIZED_DATA|MEM_READ|MEM_WRITE
	for _, s := range h.sectInfo {
		if s.chars&dataChars != dataChars || s.rawSize == 0 {
			continue
		}
		data := make([]byte, min(s.rawSize, 1<<20))
		n, _ := f.ReadAt(data, int64(s.rawOffset))
		return parseGoBuildInfo(data[:n])
	}
	return "", "", false
}

// parseGoBuildInfo reads the blob: the magic, 16-byte aligned, then the pointer size
// and flags, then (flag 2: Go 1.18 and later) the two strings, each one after its
// length (uvarint), from offset 32.
func parseGoBuildInfo(data []byte) (goVersion, modInfo string, ok bool) {
	const align, header = 16, 32
	for off := 0; ; {
		i := bytes.Index(data[off:], goBuildInfoMagic)
		if i < 0 {
			return "", "", false
		}
		off += i
		if off%align == 0 && len(data)-off >= header {
			data = data[off:]
			break
		}
		off++
	}
	if data[15]&2 == 0 { // older programs point to the strings: not read here
		return "", "", false
	}
	str := func(b []byte) (string, []byte, bool) {
		n, w := binary.Uvarint(b)
		if w <= 0 || n > uint64(len(b)-w) {
			return "", nil, false
		}
		return string(b[w : w+int(n)]), b[w+int(n):], true
	}
	rest := data[header:]
	goVersion, rest, ok = str(rest)
	if !ok {
		return "", "", false
	}
	modInfo, _, ok = str(rest)
	if !ok {
		return "", "", false
	}
	// the module information is between two 16-byte markers
	if len(modInfo) >= 33 && modInfo[len(modInfo)-17] == '\n' {
		modInfo = modInfo[16 : len(modInfo)-16]
	} else {
		modInfo = ""
	}
	return goVersion, modInfo, true
}

// goModuleVersion is the version of the main module of a Go program ("v2.40.0"),
// or "" (unknown, or built from a working copy: "(devel)").
func goModuleVersion(path string) string {
	_, mod, ok := goBuildInfo(path)
	if !ok {
		return ""
	}
	for _, line := range strings.Split(mod, "\n") {
		if f := strings.Split(line, "\t"); len(f) >= 3 && f[0] == "mod" {
			if f[2] == "(devel)" {
				return ""
			}
			return f[2]
		}
	}
	return ""
}
