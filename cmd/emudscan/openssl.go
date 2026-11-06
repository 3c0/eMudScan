package main

// Software that loads the Windows list of trusted roots into its own OpenSSL library
// verifies certificates without Windows' list of blocked certificates: it accepts the
// compromised intermediates when they chain to a root present on the PC, like Python's
// ssl module ("Software copying the Windows root list"). libtorrent does this on
// Windows for HTTPS trackers and web seeds (session_impl::start_session).

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"strings"
)

type peImportInfo struct {
	enumStore bool // CertEnumCertificatesInStore (crypt32): reads a certificate store
	openSSL   bool // imports from an OpenSSL library (libssl, libcrypto)
}

// peImports reads what a Windows executable imports.
func peImports(path string) (peImportInfo, error) {
	var info peImportInfo
	syms, err := peImportedSymbols(path)
	if err != nil {
		return info, err
	}
	for _, s := range syms {
		name, dll, _ := strings.Cut(s, ":")
		dll = lower(dll)
		if name == "CertEnumCertificatesInStore" && strings.HasPrefix(dll, "crypt32") {
			info.enumStore = true
		}
		for _, p := range []string{"libssl", "libcrypto", "ssleay32", "libeay32"} {
			if strings.HasPrefix(dll, p) {
				info.openSSL = true
			}
		}
	}
	return info, nil
}

// copiesWindowsRoots reports whether a file (scanned with the embed scanner) carries
// OpenSSL and reads a Windows certificate store, i.e. loads the Windows roots into
// OpenSSL.
func copiesWindowsRoots(path string, f embedFindings) bool {
	info, err := peImports(path)
	if err != nil || !info.enumStore {
		return false
	}
	return f.OpenSSL() || info.openSSL
}

var errNotPE = errors.New("not a Windows program")

// peImportedSymbols lists the functions a Windows program imports by name, as
// "function:library" (as debug/pe's ImportedSymbols, which would add about 100 KB
// to eMudScan), from its import table.
func peImportedSymbols(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	h, ok := readPEHeaders(f)
	if !ok {
		return nil, errNotPE
	}
	if len(h.dirs) < 2 || h.dirs[1][0] == 0 {
		return nil, nil
	}
	// the sections holding the table are read once, whole
	cache := map[int][]byte{}
	at := func(rva uint32, n int) []byte {
		for i, s := range h.sections {
			if rva < s[0] || rva >= s[0]+max(s[1], s[2]) {
				continue
			}
			data, ok := cache[i]
			if !ok {
				data = make([]byte, min(s[2], 64<<20))
				m, _ := f.ReadAt(data, int64(s[3]))
				data = data[:m]
				cache[i] = data
			}
			p := int(rva - s[0])
			if p >= len(data) {
				return nil
			}
			return data[p:min(p+n, len(data))]
		}
		return nil
	}
	cstr := func(rva uint32) string {
		b := at(rva, 512)
		if i := bytes.IndexByte(b, 0); i >= 0 {
			b = b[:i]
		}
		return string(b)
	}
	thunk, byOrdinal := uint32(4), uint64(1)<<31
	if h.pe64 {
		thunk, byOrdinal = 8, 1<<63
	}
	var syms []string
	for d := h.dirs[1][0]; ; d += 20 { // import descriptors, up to an empty one
		desc := at(d, 20)
		if len(desc) < 20 {
			break
		}
		names, lib, addrs := binary.LittleEndian.Uint32(desc), binary.LittleEndian.Uint32(desc[12:]), binary.LittleEndian.Uint32(desc[16:])
		if names == 0 && lib == 0 && addrs == 0 {
			break
		}
		dll := cstr(lib)
		if names == 0 {
			names = addrs
		}
		for t := names; len(syms) < 1<<20; t += thunk {
			b := at(t, int(thunk))
			if len(b) < int(thunk) {
				break
			}
			v := uint64(binary.LittleEndian.Uint32(b))
			if h.pe64 {
				v = binary.LittleEndian.Uint64(b)
			}
			if v == 0 {
				break
			}
			if v&byOrdinal == 0 { // else imported by number: no name
				syms = append(syms, cstr(uint32(v)+2)+":"+dll) // after the 2-byte hint
			}
		}
	}
	return syms, nil
}
