package main

// Programs built with PyInstaller (such as mitmproxy's standalone programs) carry
// Python and its packages in an archive appended to the program, each file
// compressed on its own. A certificate bundle stored there (certifi\cacert.pem)
// cannot be seen by reading the program as it is: pyiPacked finds the archive and the
// files in it that are worth reading.
//
// The archive ends with an 88-byte "cookie": a magic string, the length of the
// archive, the offset and length of its table of contents, the Python version and the
// name of the Python library. Each entry of the table gives the offset of a file in
// the archive, its stored and uncompressed sizes, whether it is compressed (zlib), its
// type and its name.

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"io"
	"strings"
)

const (
	pyiMagic     = "MEI\x0c\x0b\x0a\x0b\x0e"
	pyiCookieLen = 88
	pyiWindow    = 4096      // the archive ends within this many bytes before the end looked at
	pyiMaxTOC    = 16 << 20  // largest table of contents read
	pyiMaxFile   = 64 << 20  // largest file read from an archive, once decompressed
	pyiMaxTotal  = 256 << 20 // most read from the files of one archive, once decompressed
	pyiMaxFiles  = 64        // most files read from one archive
)

type pyiEntry struct {
	name       string
	off, size  int64 // offset in the program file, and stored size
	compressed bool
}

// pyiPacked returns the files that may hold certificates in the PyInstaller archive
// that ends just before one of ends (the end of the program file, or the start of its
// signature): certificate bundles, and Python's ssl module, whose block list
// (CVE-2026-115557) rejects the compromised certificates whatever bundle is used.
// It returns nothing for a program that is not built with PyInstaller.
func pyiPacked(r io.ReaderAt, ends []int64) []pyiEntry {
	for _, end := range ends {
		entries, ok := pyiEntries(r, end)
		if !ok {
			continue
		}
		var out []pyiEntry
		seen := map[[2]int64]bool{}
		for _, e := range entries {
			k := [2]int64{e.off, e.size}
			if pyiWorthReading(e.name) && !seen[k] && len(out) < pyiMaxFiles {
				seen[k] = true
				out = append(out, e)
			}
		}
		return out
	}
	return nil
}

func pyiWorthReading(name string) bool {
	base := pyiBase(name)
	switch {
	case strings.HasSuffix(base, ".pem"), strings.HasSuffix(base, ".crt"), strings.HasSuffix(base, ".cer"):
		return true
	}
	return pyiSSLModule(name)
}

func pyiBase(name string) string {
	n := lower(strings.ReplaceAll(name, "/", `\`))
	return n[strings.LastIndex(n, `\`)+1:]
}

// pyiSSLModule: Python's ssl module (_ssl.pyd, _ssl.cp314-win_amd64.pyd).
func pyiSSLModule(name string) bool {
	base := pyiBase(name)
	return strings.HasPrefix(base, "_ssl.") && strings.HasSuffix(base, ".pyd")
}

// pyiDisplayName is the name of a file of the archive as shown in the details: the
// program decides it, so it keeps only ordinary characters, without the details'
// markup, and is shortened.
func pyiDisplayName(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r < 0x20 || r == 0x7f || r == '[' || r == ']' || r == '*' || r == '@' || r == '$':
			b.WriteRune('_')
		default:
			b.WriteRune(r)
		}
	}
	s := []rune(strings.TrimSpace(b.String()))
	if len(s) > 100 {
		s = append(s[:99], '…')
	}
	return string(s)
}

// pyiEntries reads the table of contents of the archive whose cookie ends within
// pyiWindow bytes before end. ok is false when there is no such archive, or when its
// cookie or its table of contents does not hold together.
func pyiEntries(r io.ReaderAt, end int64) (entries []pyiEntry, ok bool) {
	start := max(0, end-pyiWindow)
	buf := make([]byte, end-start)
	if n, _ := r.ReadAt(buf, start); int64(n) != end-start {
		return nil, false
	}
	i := bytes.LastIndex(buf, []byte(pyiMagic))
	if i < 0 || len(buf)-i < pyiCookieLen {
		return nil, false
	}
	c := buf[i : i+pyiCookieLen]
	pkgLen := int64(binary.BigEndian.Uint32(c[8:]))
	tocOff := int64(binary.BigEndian.Uint32(c[12:]))
	tocLen := int64(binary.BigEndian.Uint32(c[16:]))
	pkgStart := start + int64(i) + pyiCookieLen - pkgLen
	if pkgStart < 0 || tocLen > pyiMaxTOC || tocOff+tocLen > pkgLen {
		return nil, false
	}
	toc := make([]byte, tocLen)
	if n, _ := r.ReadAt(toc, pkgStart+tocOff); int64(n) != tocLen {
		return nil, false
	}
	for p := 0; p < len(toc); {
		if p+18 > len(toc) {
			return nil, false
		}
		n := int(binary.BigEndian.Uint32(toc[p:]))
		if n < 18 || n > len(toc)-p {
			return nil, false
		}
		off := int64(binary.BigEndian.Uint32(toc[p+4:]))
		size := int64(binary.BigEndian.Uint32(toc[p+8:]))
		name := strings.TrimRight(string(toc[p+18:p+n]), "\x00")
		if off+size > pkgLen {
			return nil, false
		}
		entries = append(entries, pyiEntry{name, pkgStart + off, size, toc[p+16] == 1})
		p += n
	}
	return entries, true
}

// pyiOpen returns the content of a file of the archive, decompressed, or nil.
func pyiOpen(r io.ReaderAt, e pyiEntry) io.Reader {
	sr := io.NewSectionReader(r, e.off, e.size)
	if !e.compressed {
		return io.LimitReader(sr, pyiMaxFile)
	}
	z, err := zlib.NewReader(sr)
	if err != nil {
		return nil
	}
	return io.LimitReader(z, pyiMaxFile)
}
