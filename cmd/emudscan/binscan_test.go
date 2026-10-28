package main

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEmbedScanner(t *testing.T) {
	es := buildEmbedScanner()
	g3 := rootByName(rootNameECCG3)
	// Node.js style: PEM text in the binary, version string straddling the 1 MiB chunk boundary
	data := bytes.Repeat([]byte{0}, 1<<20-12)
	data = append(data, []byte("Chrome/138.0.1.2 Electron/44.9.1\x00")...)
	data = append(data, bytes.Repeat([]byte{1}, 5000)...)
	data = append(data, []byte("-----BEGIN CERTIFICATE-----\n"+pemLines(g3.DER)[0]+"\n"+pemLines(g3.DER)[1]+"\n"+pemLines(g3.DER)[2]+"\n")...)
	data = append(data, []byte("%s/%s Chrome/%s Electron/44.9.1 and Electron/12.0.0\x00")...)
	f, err := es.scan(bytes.NewReader(data), scanVersions)
	if err != nil {
		t.Fatal(err)
	}
	if !f.HasEmsignRoot() || f.Blocklist || f.Electron != "44.9.1" {
		t.Fatalf("unexpected findings: %+v", f)
	}
	// DER SPKI of AAA (webpki-roots style) + block list as raw SPKI hash
	aaa := rootByName(rootNameAAA)
	h, _ := hex.DecodeString(uniqueBadSPKIs()[3])
	data2 := append([]byte("xx"), aaa.Cert.RawSubjectPublicKeyInfo...)
	data2 = append(data2, h...)
	f2, _ := es.scan(bytes.NewReader(data2), scanRoots)
	if len(f2.Roots) != 1 || f2.Roots[0] != rootNameAAA || !f2.Blocklist || f2.HasEmsignRoot() {
		t.Fatalf("unexpected findings 2: %+v", f2)
	}
	f3, _ := es.scan(bytes.NewReader(bytes.Repeat([]byte("hello world "), 100000)), scanVersions)
	if f3.HasRoots() || f3.Blocklist || f3.Electron != "" {
		t.Fatalf("false positive: %+v", f3)
	}
}

func TestWebpkiStyle(t *testing.T) {
	es := buildEmbedScanner()
	g3 := rootByName(rootNameECCG3)
	inner := derContents(g3.Cert.RawSubjectPublicKeyInfo)
	if inner == nil || len(inner) != len(g3.Cert.RawSubjectPublicKeyInfo)-2 {
		t.Fatalf("derContents: %d", len(inner))
	}
	aaa := derContents(rootByName(rootNameAAA).Cert.RawSubjectPublicKeyInfo)
	if aaa == nil {
		t.Fatal("derContents AAA nil")
	}
	f, _ := es.scan(bytes.NewReader(append(append([]byte("abc"), inner...), 'z')), scanRoots)
	if !f.HasEmsignRoot() {
		t.Fatalf("webpki-style SPKI not found: %+v", f)
	}
}

// ---------------------------------------------------------------------------
// Aho-Corasick automaton with a full transition table: the previous matcher,
// kept as a reference for the faster one.

type acMatcher struct {
	delta [][256]int32
	out   [][]int32
	plen  []int
}

func newAC(patterns [][]byte) *acMatcher {
	m := &acMatcher{plen: make([]int, len(patterns))}
	newState := func() int32 {
		var row [256]int32
		for i := range row {
			row[i] = -1
		}
		m.delta = append(m.delta, row)
		m.out = append(m.out, nil)
		return int32(len(m.delta) - 1)
	}
	newState()
	for pid, p := range patterns {
		m.plen[pid] = len(p)
		s := int32(0)
		for _, b := range p {
			if m.delta[s][b] < 0 {
				ns := newState()
				m.delta[s][b] = ns
			}
			s = m.delta[s][b]
		}
		m.out[s] = append(m.out[s], int32(pid))
	}
	fail := make([]int32, len(m.delta))
	var queue []int32
	for b := 0; b < 256; b++ {
		if t := m.delta[0][b]; t > 0 {
			fail[t] = 0
			queue = append(queue, t)
		} else {
			m.delta[0][b] = 0
		}
	}
	for len(queue) > 0 {
		s := queue[0]
		queue = queue[1:]
		m.out[s] = append(m.out[s], m.out[fail[s]]...)
		for b := 0; b < 256; b++ {
			t := m.delta[s][b]
			if t < 0 {
				m.delta[s][b] = m.delta[fail[s]][b]
				continue
			}
			fail[t] = m.delta[fail[s]][b]
			queue = append(queue, t)
		}
	}
	return m
}

// acFind lists the patterns found by the reference automaton (id, end position).
func acFind(m *acMatcher, data []byte) map[[2]int]bool {
	out := map[[2]int]bool{}
	state := int32(0)
	for i, b := range data {
		state = m.delta[state][b]
		for _, pid := range m.out[state] {
			out[[2]int{int(pid), i + 1}] = true
		}
	}
	return out
}

// The skipping matcher finds exactly what the automaton finds, also across chunks.
func TestMatcherAgainstAutomaton(t *testing.T) {
	es := buildEmbedScanner()
	ac := newAC(es.raw)
	rnd := rand.New(rand.NewSource(7))
	for round := 0; round < 40; round++ {
		data := make([]byte, 3<<20+rnd.Intn(5000))
		rnd.Read(data)
		if round%2 == 0 { // text-like data
			for i := range data {
				data[i] = "0123456789abcdefABCDEF/+= .\n"[int(data[i])%28]
			}
		}
		for k := 0; k < 30; k++ { // plant patterns, some across the 1 MiB boundaries
			p := es.raw[rnd.Intn(len(es.raw))]
			at := rnd.Intn(len(data) - len(p))
			if k < 6 {
				at = (1+rnd.Intn(2))<<20 - rnd.Intn(len(p))
			}
			copy(data[at:], p)
		}
		want := acFind(ac, data)
		got := map[[2]int]bool{}
		w := es.wm
		buf, base := []byte{}, 0
		for off := 0; ; {
			n := min(1<<20, len(data)-off)
			buf = append(buf, data[off:off+n]...)
			off += n
			final := off == len(data)
			next := w.find(buf, 0, versionTail, final, func(pid int32, end int) { got[[2]int{int(pid), base + end}] = true })
			if final {
				break
			}
			base += next
			buf = append([]byte{}, buf[next:]...)
		}
		if len(got) != len(want) {
			t.Fatalf("round %d: %d matches, want %d", round, len(got), len(want))
		}
		for k := range want {
			if !got[k] {
				t.Fatalf("round %d: missing %v", round, k)
			}
		}
	}
}

// Files are scanned in parallel, each once, with their results in order.
func TestScanAll(t *testing.T) {
	dir := t.TempDir()
	g3 := rootByName(rootNameECCG3)
	var paths []string
	for i := 0; i < 20; i++ {
		data := bytes.Repeat([]byte{byte(i)}, 300<<10)
		if i%3 == 0 {
			data = append(data, g3.Cert.RawSubjectPublicKeyInfo...)
		}
		p := filepath.Join(dir, fmt.Sprintf("tool%02d.exe", i))
		os.WriteFile(p, data, 0o644)
		paths = append(paths, p)
	}
	tp := &appsContext{c: newChecker(nopReporter{}, nil), es: buildEmbedScanner()}
	res := tp.scanAll(paths, scanRoots)
	for i, e := range res {
		if e.err != nil || e.f.HasRoots() != (i%3 == 0) {
			t.Errorf("%s: %+v", paths[i], e)
		}
	}
	if len(tp.scanned) != 20 {
		t.Errorf("%d files in the cache", len(tp.scanned))
	}
	if f, _ := tp.scan(paths[0], scanRoots); !f.HasRoots() {
		t.Error("cached result lost")
	}
}

// logReporter keeps the log lines.
type logReporter struct{ lines *[]string }

func (r logReporter) Log(_ LogKind, _ Status, text string) { *r.lines = append(*r.lines, text) }
func (logReporter) Upsert(Item)                            {}
func (logReporter) Activity(string)                        {}

// A failure of the readers on one file is an error of that file, noted in the log,
// not the end of the program.
func TestScanFileRecovers(t *testing.T) {
	var lines []string
	tp := &appsContext{c: newChecker(logReporter{&lines}, nil)} // es == nil: the readers fail at once
	path := filepath.Join(t.TempDir(), "some-program.exe")
	if err := os.WriteFile(path, []byte("MZ"), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := tp.scanFile(path, scanRoots)
	if err == nil || !strings.Contains(err.Error(), "internal error") {
		t.Fatalf("err = %v, want an internal error", err)
	}
	if f.HasRoots() || f.Blocklist {
		t.Errorf("findings = %+v, want none", f)
	}
	if len(lines) != 1 || !strings.Contains(lines[0], "some-program.exe") {
		t.Errorf("log = %q, want one line naming the file", lines)
	}
}
