package main

// Scanning program files for built-in certificate lists.
//
// Many tools do not use the Windows certificate store: Node.js, Bun and
// Electron compile Mozilla's root list into the executable as PEM text, Rust
// tools embed it as DER (webpki-roots), Java keeps it in lib\security\cacerts.
// We look for the affected roots in those files, and for traces of a block
// list (the SPKI or certificate hashes of the compromised intermediates)
// that patched versions added.

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"io"
	"os"
	"sort"
)

// ---------------------------------------------------------------------------
// Multi-pattern search that skips: Wu-Manber with blocks of 2 bytes. A window as long
// as the shortest pattern slides over the data; when its last 2 bytes appear in no
// pattern prefix, it jumps ahead by up to the window length minus one. Program files
// are mostly code and data that contain none of the patterns, so most bytes are never
// looked at, which is several times faster than reading each byte.

type wmMatcher struct {
	m      int // window: length of the shortest pattern (at least 8)
	maxLen int
	shift  [1 << 16]uint8
	pats   [][]byte
	cand   map[uint64][]int32 // patterns by their first 8 bytes
}

func newWM(patterns [][]byte) *wmMatcher {
	w := &wmMatcher{m: 1 << 30, pats: patterns, cand: map[uint64][]int32{}}
	for _, p := range patterns {
		w.m, w.maxLen = min(w.m, len(p)), max(w.maxLen, len(p))
	}
	if w.m < 8 {
		panic("scan patterns must be at least 8 bytes long")
	}
	w.m = min(w.m, 200)
	for i := range w.shift {
		w.shift[i] = uint8(w.m - 1)
	}
	for pid, p := range patterns {
		for j := 1; j < w.m; j++ { // block p[j-1:j+1], ending at j of the window
			h := uint16(p[j-1])<<8 | uint16(p[j])
			w.shift[h] = min(w.shift[h], uint8(w.m-1-j))
		}
		k := binary.LittleEndian.Uint64(p)
		w.cand[k] = append(w.cand[k], int32(pid))
	}
	return w
}

// find calls hit for each pattern found in data[from:]: its id and the position just
// after it. Without final, the search stops where a pattern (and the version that may
// follow it, tail bytes) could go past the end of data; the position returned is
// where to resume once more data follows.
func (w *wmMatcher) find(data []byte, from, tail int, final bool, hit func(pid int32, end int)) int {
	s, m := from, w.m
	stop := len(data) - m
	if !final {
		stop = len(data) - w.maxLen - tail
	}
	shift := &w.shift
	for s <= stop {
		if sh := shift[binary.BigEndian.Uint16(data[s+m-2:])]; sh > 0 { // the last 2 bytes of the window
			s += int(sh)
			continue
		}
		if pids, ok := w.cand[binary.LittleEndian.Uint64(data[s:])]; ok {
			for _, pid := range pids {
				p := w.pats[pid]
				if s+len(p) <= len(data) && bytes.Equal(data[s:s+len(p)], p) {
					hit(pid, s+len(p))
				}
			}
		}
		s++
	}
	if final {
		return len(data)
	}
	return max(s, from)
}

// ---------------------------------------------------------------------------
// Patterns

type patKind int

const (
	patRoot      patKind = iota // material of an affected root certificate
	patBlocklist                // hash of a compromised intermediate (block list)
	patElectron                 // "Electron/" followed by the Electron version
	patChrome                   // "Chrome/" followed by the Chromium version
	patMarker                   // a text that identifies a library (OpenSSL, libtorrent…)
)

type scanPattern struct {
	kind patKind
	root string // for patRoot; the marker name for patMarker
}

// Library markers: OpenSSL 3 always contains its default provider; OpenSSL 1.1.1
// its version text, which BoringSSL (Chromium, Electron) imitates as "OpenSSL 1.1.1
// (compatible; BoringSSL)".
var libraryMarkers = []struct{ name, text string }{
	{"openssl3", "OpenSSL Default Provider"},
	{"openssl1", "OpenSSL 1.1.1"},
	{"boringssl", "BoringSSL"},
	{"libtorrent", "libtorrent/"},
}

type embedScanner struct {
	wm      *wmMatcher // all patterns
	roots   *wmMatcher // only the root and block list patterns: all long, so it skips more
	rootIDs []int32    // pattern of each pattern of roots
	pats    []scanPattern
	raw     [][]byte
}

// scanWhat: what a scan looks for besides the roots and the block list.
type scanWhat uint8

const (
	scanRoots    scanWhat = 0
	scanMarkers  scanWhat = 1 // library markers (OpenSSL, libtorrent…)
	scanVersions scanWhat = 2 // Electron and Chromium versions
)

var defaultEmbedScanner *embedScanner

func pemLines(der []byte) []string {
	s := base64.StdEncoding.EncodeToString(der)
	var lines []string
	for len(s) > 64 {
		lines = append(lines, s[:64])
		s = s[64:]
	}
	return append(lines, s)
}

// derContents strips the tag and length of a DER element and returns its value.
func derContents(der []byte) []byte {
	if len(der) < 2 {
		return nil
	}
	l := int(der[1])
	hdr := 2
	if l&0x80 != 0 {
		n := l & 0x7f
		if n == 0 || n > 3 || len(der) < 2+n {
			return nil
		}
		l = 0
		for _, b := range der[2 : 2+n] {
			l = l<<8 | int(b)
		}
		hdr += n
	}
	if hdr+l != len(der) {
		return nil
	}
	return der[hdr:]
}

func buildEmbedScanner() *embedScanner {
	es := &embedScanner{}
	var raw [][]byte
	add := func(p []byte, sp scanPattern) {
		raw = append(raw, p)
		es.pats = append(es.pats, sp)
	}
	for _, r := range rootCerts {
		if r.Name == rootNameCSG3 {
			continue
		}
		sp := scanPattern{kind: patRoot, root: r.Name}
		add(r.Cert.RawSubjectPublicKeyInfo, sp) // DER (Java cacerts, Go/Chromium root stores)
		if inner := derContents(r.Cert.RawSubjectPublicKeyInfo); inner != nil {
			add(inner, sp) // value without the SEQUENCE header (Rust webpki-roots trust anchors)
		}
		lines := pemLines(r.DER) // PEM text (Node.js, Bun, Electron's Node)
		for i := 1; i < len(lines)-1 && i <= 3; i++ {
			add([]byte(lines[i]), sp)
		}
	}
	bl := scanPattern{kind: patBlocklist}
	for _, h := range uniqueBadSPKIs() {
		b, _ := hex.DecodeString(h)
		add(b, bl)
		add([]byte(h), bl)
		add(bytes.ToLower([]byte(h)), bl)
		add([]byte(base64.StdEncoding.EncodeToString(b)), bl)
	}
	for _, c := range badCerts {
		b, _ := hex.DecodeString(c.SHA256)
		add(b, bl)
		add([]byte(c.SHA256), bl)
		add(bytes.ToLower([]byte(c.SHA256)), bl)
	}
	for _, m := range libraryMarkers {
		add([]byte(m.text), scanPattern{kind: patMarker, root: m.name})
	}
	add([]byte("Electron/"), scanPattern{kind: patElectron})
	add([]byte(" Chrome/"), scanPattern{kind: patChrome})
	es.wm = newWM(raw)
	es.raw = raw
	var long [][]byte
	for i, sp := range es.pats {
		if sp.kind == patRoot || sp.kind == patBlocklist {
			long = append(long, raw[i])
			es.rootIDs = append(es.rootIDs, int32(i))
		}
	}
	es.roots = newWM(long)
	return es
}

// embedFindings is the result of scanning one file.
type embedFindings struct {
	Roots     []string // affected roots found
	Blocklist bool     // hashes of the compromised intermediates found
	Electron  string   // most frequent "Electron/x.y.z" version
	Chrome    string
	Markers   map[string]bool // library markers found (libraryMarkers)
}

// OpenSSL reports whether the file contains OpenSSL (not BoringSSL).
func (f embedFindings) OpenSSL() bool {
	return f.Markers["openssl3"] || f.Markers["openssl1"] && !f.Markers["boringssl"]
}

func (f embedFindings) HasRoots() bool { return len(f.Roots) > 0 }

func (f embedFindings) HasEmsignRoot() bool {
	for _, r := range f.Roots {
		if r != rootNameAAA {
			return true
		}
	}
	return false
}

// readVersionAt reads a version like 44.9.1 or 138.0.7204.251 starting at buf[i:].
// It returns "" if fewer than three numeric parts are present or buf ends too early.
func readVersionAt(buf []byte, i int) (string, bool) {
	j := i
	for j < len(buf) && j-i < 24 && (buf[j] >= '0' && buf[j] <= '9' || buf[j] == '.') {
		j++
	}
	if j == len(buf) && j-i < 24 {
		return "", false // may continue in the next chunk
	}
	v := string(buf[i:j])
	for len(v) > 0 && v[len(v)-1] == '.' {
		v = v[:len(v)-1]
	}
	if len(ParseVersion(v)) < 3 || v[0] == '.' {
		return "", true
	}
	return v, true
}

// scanFile scans one file. In a program (PE file), only what can hold data is read:
// its compiled code (.text) and the GPU code of CUDA programs (.nv_fatb), which can
// be most of a large program, never contain certificates; nor do the sections that
// the program does not use once loaded (debugging information, relocations); nor
// does its digital signature, whose certificates (often up to Sectigo's AAA
// Certificate Services root) are not a list the program trusts.
func (es *embedScanner) scanFile(path string, what scanWhat) (embedFindings, error) {
	f, err := os.Open(path)
	if err != nil {
		return embedFindings{}, err
	}
	defer f.Close()
	acc := es.newAcc(what)
	if ranges, ok := peDataRanges(f); ok {
		for _, r := range ranges {
			if err := acc.feed(io.NewSectionReader(f, r[0], r[1])); err != nil {
				return embedFindings{}, err
			}
		}
	} else if err := acc.feed(f); err != nil {
		return embedFindings{}, err
	}
	return acc.result(), nil
}

func (es *embedScanner) scan(r io.Reader, what scanWhat) (embedFindings, error) {
	acc := es.newAcc(what)
	if err := acc.feed(r); err != nil {
		return embedFindings{}, err
	}
	return acc.result(), nil
}

// peDataRanges lists the parts of a program file to scan (offset, length): its
// sections except compiled code and CUDA GPU code, and what follows them (data
// appended to the program) except the signature. ok is false when the file is not a
// program, or not one whose layout can be trusted: it is then read entirely.
func peDataRanges(f *os.File) (ranges [][2]int64, ok bool) {
	fi, err := f.Stat()
	if err != nil {
		return nil, false
	}
	size := fi.Size()
	h, ok := readPEHeaders(f)
	if !ok || len(h.sectInfo) == 0 {
		return nil, false
	}
	var end int64
	for _, sec := range h.sectInfo {
		off, n := int64(sec.rawOffset), int64(sec.rawSize)
		if n == 0 {
			continue
		}
		if off+n > size {
			return nil, false // truncated or unusual: read it all
		}
		end = max(end, off+n)
		code := sec.chars&0x20000000 != 0 /* MEM_EXECUTE */ && sec.chars&0x20 != 0 /* CNT_CODE */ && sec.name == ".text"
		discardable := sec.chars&0x02000000 != 0 // MEM_DISCARDABLE: debugging information, relocations
		if code || discardable || sec.name == ".nv_fatb" {
			continue
		}
		ranges = append(ranges, [2]int64{off, n})
	}
	// data appended after the sections, without the signature (certificate table)
	sigOff, sigLen := int64(0), int64(0)
	if len(h.dirs) > 4 {
		sigOff, sigLen = int64(h.dirs[4][0]), int64(h.dirs[4][1])
	}
	if end < size {
		tail := [2]int64{end, size - end}
		if sigLen > 0 && sigOff >= end && sigOff+sigLen <= size {
			if sigOff > end {
				ranges = append(ranges, [2]int64{end, sigOff - end})
			}
			if sigOff+sigLen < size {
				ranges = append(ranges, [2]int64{sigOff + sigLen, size - sigOff - sigLen})
			}
		} else {
			ranges = append(ranges, tail)
		}
	}
	return ranges, true
}

// scanAcc gathers the findings of one file, possibly read in several parts.
type scanAcc struct {
	es       *embedScanner
	what     scanWhat
	w        *wmMatcher
	ids      []int32 // pattern of each pattern of w (nil: the same)
	roots    map[string]bool
	electron map[string]int
	chrome   map[string]int
	res      embedFindings
}

func (es *embedScanner) newAcc(what scanWhat) *scanAcc {
	a := &scanAcc{es: es, what: what, w: es.wm, roots: map[string]bool{}, electron: map[string]int{}, chrome: map[string]int{}}
	if what == scanRoots {
		a.w, a.ids = es.roots, es.rootIDs
	}
	return a
}

// versionTail: room after a pattern for the version that follows it.
const versionTail = 32

// feed scans one part of a file.
func (a *scanAcc) feed(r io.Reader) error {
	const chunk = 1 << 20
	w := a.w
	keep := w.maxLen + versionTail // a pattern can start in a chunk and end in the next one
	buf := make([]byte, keep+chunk)
	carried := 0
	for {
		n, err := io.ReadFull(r, buf[carried:carried+chunk])
		if n == 0 && err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
			return err
		}
		data := buf[:carried+n]
		final := err != nil
		next := w.find(data, 0, versionTail, final, func(pid int32, end int) {
			if a.ids != nil {
				pid = a.ids[pid]
			}
			sp := a.es.pats[pid]
			switch sp.kind {
			case patRoot:
				a.roots[sp.root] = true
			case patBlocklist:
				a.res.Blocklist = true
			case patMarker:
				if a.what&scanMarkers == 0 {
					return
				}
				if a.res.Markers == nil {
					a.res.Markers = map[string]bool{}
				}
				a.res.Markers[sp.root] = true
			case patElectron, patChrome:
				if a.what&scanVersions == 0 {
					return
				}
				if v, _ := readVersionAt(data, end); v != "" {
					if sp.kind == patElectron {
						a.electron[v]++
					} else {
						a.chrome[v]++
					}
				}
			}
		})
		if final {
			return nil
		}
		carried = copy(buf, data[next:])
	}
}

func (a *scanAcc) result() embedFindings {
	res := a.res
	for r := range a.roots {
		res.Roots = append(res.Roots, r)
	}
	sort.Strings(res.Roots)
	res.Electron = mostFrequent(a.electron)
	res.Chrome = mostFrequent(a.chrome)
	return res
}

func mostFrequent(m map[string]int) string {
	best, bestN := "", 0
	for k, n := range m {
		if n > bestN || n == bestN && ParseVersion(k).Cmp(ParseVersion(best)) > 0 {
			best, bestN = k, n
		}
	}
	return best
}
