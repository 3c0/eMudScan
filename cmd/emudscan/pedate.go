package main

// When a software was last updated by its publisher: the date of its main program,
// taken in this order from
//  1. the digital signature: the time stamp added when the publisher signed the file
//     (Properties › Digital Signatures). It is the publisher's date and cannot be the
//     date of the installation;
//  2. the modified date of the file, when it was kept from the publisher's package:
//     archives (Scoop, portable programs) and most installers keep it, the file is
//     then older than its creation on this PC;
//  3. the build date written in the program by the compiler, when it is a real date
//     (reproducible builds write a hash instead, and Electron applications carry
//     the date of Electron itself);
//  4. else the modified date of the file, even when it is the date the file was
//     written on this PC (by an installer or an updater).
// Firefox-based browsers use their build date (application.ini) before 2 to 4 (see
// appsContext.dated).

import (
	"bytes"
	"encoding/asn1"
	"encoding/binary"
	"os"
	"strings"
	"time"
)

// dateSource tells where the date of a software comes from.
type dateSource int

const (
	dateNone     dateSource = iota
	dateSigned              // digital signature time stamp
	dateModified            // modified date of the file
	dateBuilt               // build date in the program header
)

// writtenHere: modified and created within this delay, the file was written on this
// PC (its date is the date of the installation).
const writtenHere = 2 * time.Minute

// placeholderDate: reproducible builds give all their files a fixed date, often
// 1 January 2000 (Tor Browser) or 1 January 1980 (zip archives).
var placeholderDate = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

func plausibleDate(t time.Time) bool {
	if d := t.Sub(placeholderDate); d > -14*time.Hour && d < 14*time.Hour {
		return false
	}
	return t.Year() >= 1995 && t.Before(time.Now().Add(24*time.Hour))
}

func isPEName(p string) bool {
	switch lower(p[strings.LastIndex(p, ".")+1:]) {
	case "exe", "dll":
		return true
	}
	return false
}

// publisherDate is the date the publisher gave to a file (see above). noBuild skips
// the build date (Electron applications: it is the date of Electron).
func publisherDate(path string, noBuild bool) (time.Time, dateSource) {
	fi, err := os.Stat(path)
	if err != nil || fi.IsDir() {
		return time.Time{}, dateNone
	}
	pe := isPEName(path) && !isCloudPlaceholder(fi) // never read files that are only in the cloud
	if pe {
		if t, ok := signingTime(path); ok {
			return t, dateSigned
		}
	}
	ct, hasCT := creationTime(fi)
	return pickDate(fi.ModTime(), ct, hasCT, func() (time.Time, bool) {
		if !pe || noBuild {
			return time.Time{}, false
		}
		return peBuildTime(path)
	})
}

// pickDate chooses between the modified date of a file (mt), kept from the
// publisher's package when it is older than the creation of the file on this PC
// (ct), and its build date. The modified date is the last resort in any case.
func pickDate(mt, ct time.Time, hasCT bool, build func() (time.Time, bool)) (time.Time, dateSource) {
	if !plausibleDate(mt) {
		mt = time.Time{}
	}
	if !mt.IsZero() && (!hasCT || ct.Sub(mt) >= writtenHere) {
		return mt, dateModified
	}
	if t, ok := build(); ok {
		return t, dateBuilt
	}
	if !mt.IsZero() {
		return mt, dateModified
	}
	return time.Time{}, dateNone
}

// ---------------------------------------------------------------------------
// PE headers

type peHeaders struct {
	timeStamp uint32
	dirs      [][2]uint32 // data directories: RVA (file offset for the certificates), size
	sections  [][4]uint32 // virtual address, virtual size, raw size, raw offset
	sectInfo  []peSection
	pe64      bool // PE32+ (64-bit)
}

type peSection struct {
	name               string
	chars              uint32 // characteristics (code, executable…)
	rawOffset, rawSize uint32
}

func readPEHeaders(f *os.File) (peHeaders, bool) {
	var h peHeaders
	buf := make([]byte, 4096)
	n, _ := f.ReadAt(buf, 0)
	buf = buf[:n]
	if len(buf) < 0x40 || buf[0] != 'M' || buf[1] != 'Z' {
		return h, false
	}
	off := int(binary.LittleEndian.Uint32(buf[0x3C:]))
	if off <= 0 || off+24 > len(buf) || !bytes.Equal(buf[off:off+4], []byte("PE\x00\x00")) {
		return h, false
	}
	coff := off + 4
	nSect := int(binary.LittleEndian.Uint16(buf[coff+2:]))
	h.timeStamp = binary.LittleEndian.Uint32(buf[coff+4:])
	optSize := int(binary.LittleEndian.Uint16(buf[coff+16:]))
	opt := coff + 20
	if opt+optSize > len(buf) || optSize < 2 {
		return h, false
	}
	nDirsAt, dirsAt := 92, 96
	switch binary.LittleEndian.Uint16(buf[opt:]) {
	case 0x10b: // PE32
	case 0x20b: // PE32+
		nDirsAt, dirsAt = 108, 112
		h.pe64 = true
	default:
		return h, false
	}
	if optSize >= dirsAt {
		nDirs := int(binary.LittleEndian.Uint32(buf[opt+nDirsAt:]))
		for i := 0; i < nDirs && i < 16 && dirsAt+8*i+8 <= optSize; i++ {
			p := opt + dirsAt + 8*i
			h.dirs = append(h.dirs, [2]uint32{binary.LittleEndian.Uint32(buf[p:]), binary.LittleEndian.Uint32(buf[p+4:])})
		}
	}
	sect := opt + optSize
	if nSect > 96 {
		return h, false
	}
	if sect+40*nSect > len(buf) {
		more := make([]byte, sect+40*nSect)
		if n, _ := f.ReadAt(more, 0); n < len(more) {
			return h, true
		}
		buf = more
	}
	for i := 0; i < nSect; i++ {
		s := buf[sect+40*i:]
		h.sections = append(h.sections, [4]uint32{binary.LittleEndian.Uint32(s[12:]), binary.LittleEndian.Uint32(s[8:]),
			binary.LittleEndian.Uint32(s[16:]), binary.LittleEndian.Uint32(s[20:])})
		h.sectInfo = append(h.sectInfo, peSection{name: string(bytes.TrimRight(s[:8], "\x00")), chars: binary.LittleEndian.Uint32(s[36:]),
			rawOffset: binary.LittleEndian.Uint32(s[20:]), rawSize: binary.LittleEndian.Uint32(s[16:])})
	}
	return h, true
}

// fileOffset converts an address in the loaded program to a position in the file.
func (h peHeaders) fileOffset(rva uint32) (int64, bool) {
	for _, s := range h.sections {
		size := max(s[1], s[2])
		if rva >= s[0] && rva < s[0]+size {
			return int64(rva - s[0] + s[3]), true
		}
	}
	return 0, false
}

// peBuildTime is the build date written by the linker, unless it is not a date:
// 0, before 2000 (Delphi writes 1992), in the future, or a hash (reproducible builds,
// marked by a "repro" debug entry).
func peBuildTime(path string) (time.Time, bool) {
	f, err := os.Open(path)
	if err != nil {
		return time.Time{}, false
	}
	defer f.Close()
	h, ok := readPEHeaders(f)
	if !ok || h.timeStamp == 0 {
		return time.Time{}, false
	}
	t := time.Unix(int64(h.timeStamp), 0)
	if t.Year() < 2000 || !plausibleDate(t) {
		return time.Time{}, false
	}
	if len(h.dirs) > 6 && h.dirs[6][1] >= 28 && h.dirs[6][1] < 28*64 {
		if off, ok := h.fileOffset(h.dirs[6][0]); ok {
			dbg := make([]byte, h.dirs[6][1])
			if n, _ := f.ReadAt(dbg, off); n == len(dbg) {
				for i := 0; i+28 <= len(dbg); i += 28 {
					if binary.LittleEndian.Uint32(dbg[i+12:]) == 16 { // IMAGE_DEBUG_TYPE_REPRO
						return time.Time{}, false
					}
				}
			}
		}
	}
	return t, true
}

// ---------------------------------------------------------------------------
// Digital signature (Authenticode)

// signingTime is the time stamp of the digital signature of a program, read from
// the signature itself (it is not checked: the date is only shown).
func signingTime(path string) (time.Time, bool) {
	f, err := os.Open(path)
	if err != nil {
		return time.Time{}, false
	}
	defer f.Close()
	h, ok := readPEHeaders(f)
	if !ok || len(h.dirs) <= 4 {
		return time.Time{}, false
	}
	off, size := h.dirs[4][0], h.dirs[4][1] // the certificate table: a file offset
	if off == 0 || size < 8 || size > 4<<20 {
		return time.Time{}, false
	}
	buf := make([]byte, size)
	if n, _ := f.ReadAt(buf, int64(off)); n != len(buf) {
		return time.Time{}, false
	}
	for p := 0; p+8 <= len(buf); {
		l := int(binary.LittleEndian.Uint32(buf[p:]))
		if l < 8 || p+l > len(buf) {
			break
		}
		if binary.LittleEndian.Uint16(buf[p+6:]) == 2 { // WIN_CERT_TYPE_PKCS_SIGNED_DATA
			if t, ok := pkcs7Time(buf[p+8:p+l], 0); ok {
				return t, true
			}
		}
		p += (l + 7) &^ 7
	}
	return time.Time{}, false
}

var (
	oidSigningTime = []byte{0x2a, 0x86, 0x48, 0x86, 0xf7, 0x0d, 0x01, 0x09, 0x05}             // 1.2.840.113549.1.9.5
	oidTSTInfo     = []byte{0x2a, 0x86, 0x48, 0x86, 0xf7, 0x0d, 0x01, 0x09, 0x10, 0x01, 0x04} // 1.2.840.113549.1.9.16.1.4
)

// readTLV reads one DER (or BER) element: its tag, its content, the element as a
// whole and what follows it.
func readTLV(b []byte) (tag byte, content, whole, rest []byte, ok bool) {
	if len(b) < 2 || b[0]&0x1f == 0x1f {
		return
	}
	tag = b[0]
	if b[1] == 0x80 { // indefinite length: the content ends with 00 00
		if tag&0x20 == 0 {
			return
		}
		p := b[2:]
		for n := 0; ; {
			if len(p)-n < 2 {
				return
			}
			if p[n] == 0 && p[n+1] == 0 {
				return tag, p[:n], b[:2+n+2], p[n+2:], true
			}
			_, _, _, r, ok2 := readTLV(p[n:])
			if !ok2 {
				return
			}
			n = len(p) - len(r)
		}
	}
	l, i := int(b[1]), 2
	if l&0x80 != 0 {
		k := l & 0x7f
		if k > 4 || len(b) < 2+k {
			return
		}
		l = 0
		for j := 0; j < k; j++ {
			l = l<<8 | int(b[2+j])
		}
		i += k
	}
	if l < 0 || i+l > len(b) {
		return
	}
	return tag, b[i : i+l], b[:i+l], b[i+l:], true
}

type tlv struct {
	tag            byte
	content, whole []byte
}

func children(b []byte) []tlv {
	var out []tlv
	for len(b) > 0 {
		tag, c, w, rest, ok := readTLV(b)
		if !ok {
			break
		}
		out = append(out, tlv{tag, c, w})
		b = rest
	}
	return out
}

func asn1Time(e tlv) (time.Time, bool) {
	if e.tag != 0x17 && e.tag != 0x18 { // UTCTime, GeneralizedTime
		return time.Time{}, false
	}
	var t time.Time
	if _, err := asn1.Unmarshal(e.whole, &t); err != nil {
		return time.Time{}, false
	}
	return t, true
}

// octets is the content of an OCTET STRING, also when it is split in parts (BER).
func octets(e tlv) []byte {
	if e.tag == 0x04 {
		return e.content
	}
	if e.tag == 0x24 {
		var b []byte
		for _, c := range children(e.content) {
			b = append(b, octets(c)...)
		}
		return b
	}
	return nil
}

// pkcs7Time finds the first time stamp of a PKCS #7 signature: the time of an
// RFC 3161 time stamp (TSTInfo), or a signing time attribute (older counter-signatures).
func pkcs7Time(b []byte, depth int) (time.Time, bool) {
	if depth > 48 {
		return time.Time{}, false
	}
	for _, e := range children(b) {
		if e.tag == 0x30 {
			kids := children(e.content)
			if len(kids) >= 2 && kids[0].tag == 0x06 {
				switch {
				case bytes.Equal(kids[0].content, oidSigningTime) && kids[1].tag == 0x31:
					if vals := children(kids[1].content); len(vals) > 0 {
						if t, ok := asn1Time(vals[0]); ok {
							return t, true
						}
					}
				case bytes.Equal(kids[0].content, oidTSTInfo) && kids[1].tag == 0xA0:
					if in := children(kids[1].content); len(in) > 0 {
						if tst := children(octets(in[0])); len(tst) > 0 && tst[0].tag == 0x30 {
							for _, f := range children(tst[0].content) {
								if t, ok := asn1Time(f); ok { // genTime: the only time of a TSTInfo
									return t, true
								}
							}
						}
					}
				}
			}
		}
		switch {
		case e.tag&0x20 != 0:
			if t, ok := pkcs7Time(e.content, depth+1); ok {
				return t, true
			}
		case e.tag == 0x04 && len(e.content) > 2 && e.content[0] == 0x30:
			if t, ok := pkcs7Time(e.content, depth+1); ok {
				return t, true
			}
		}
	}
	return time.Time{}, false
}
