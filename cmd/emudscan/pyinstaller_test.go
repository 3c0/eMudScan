package main

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type pyiFile struct {
	name     string
	data     []byte
	compress bool
}

// pyiArchiveBytes builds a PyInstaller archive (CArchive) holding files, as appended
// to a program: the files, the table of contents, then the cookie.
func pyiArchiveBytes(files []pyiFile) []byte {
	var pkg, toc bytes.Buffer
	for _, f := range files {
		stored := f.data
		if f.compress {
			var z bytes.Buffer
			w := zlib.NewWriter(&z)
			w.Write(f.data)
			w.Close()
			stored = z.Bytes()
		}
		off := pkg.Len()
		pkg.Write(stored)
		name := append([]byte(f.name), 0)
		for (18+len(name))%16 != 0 {
			name = append(name, 0)
		}
		var e [18]byte
		binary.BigEndian.PutUint32(e[0:], uint32(18+len(name)))
		binary.BigEndian.PutUint32(e[4:], uint32(off))
		binary.BigEndian.PutUint32(e[8:], uint32(len(stored)))
		binary.BigEndian.PutUint32(e[12:], uint32(len(f.data)))
		if f.compress {
			e[16] = 1
		}
		e[17] = 'x'
		toc.Write(e[:])
		toc.Write(name)
	}
	tocOff := pkg.Len()
	pkg.Write(toc.Bytes())
	cookie := make([]byte, pyiCookieLen)
	copy(cookie, pyiMagic)
	binary.BigEndian.PutUint32(cookie[8:], uint32(pkg.Len()+pyiCookieLen))
	binary.BigEndian.PutUint32(cookie[12:], uint32(tocOff))
	binary.BigEndian.PutUint32(cookie[16:], uint32(toc.Len()))
	binary.BigEndian.PutUint32(cookie[20:], 314)
	copy(cookie[24:], "python314.dll")
	return append(pkg.Bytes(), cookie...)
}

// compressible surrounds data with text that compresses well. Data too short to
// gain from compression, such as a few hashes, may otherwise be kept as is in the
// compressed stream, depending on the Go version, where a plain reading of the
// program would then see it.
func compressible(data string) []byte {
	pad := strings.Repeat("padding text ", 2000)
	return []byte(pad + data + pad)
}

// hidden fails the test when a compressed test archive still shows one of needles as
// is: the test would then not show what reading the archive brings.
func hidden(t *testing.T, archive []byte, needles ...[]byte) {
	t.Helper()
	for _, n := range needles {
		if bytes.Contains(archive, n) {
			t.Fatalf("the test archive keeps %x as is", n)
		}
	}
}

// pyiProgram writes a program with archive appended, optionally followed by padding
// and a signature, as signtool leaves them.
func pyiProgram(t *testing.T, archive []byte, signed bool) string {
	t.Helper()
	b, err := os.ReadFile(fakePE(t, 0, 0, nil))
	if err != nil {
		t.Fatal(err)
	}
	b = append(b, archive...)
	if signed {
		b = append(b, 0, 0, 0) // alignment of the certificate table
		const secDir = 0x40 + 4 + 20 + 112 + 8*4
		sig := bytes.Repeat([]byte("signature "), 800) // more than pyiWindow: the archive is found before it
		binary.LittleEndian.PutUint32(b[secDir:], uint32(len(b)))
		binary.LittleEndian.PutUint32(b[secDir+4:], uint32(8+len(sig)))
		hdr := make([]byte, 8)
		binary.LittleEndian.PutUint32(hdr, uint32(8+len(sig)))
		binary.LittleEndian.PutUint16(hdr[4:], 0x200)
		binary.LittleEndian.PutUint16(hdr[6:], 2)
		b = append(append(b, hdr...), sig...)
	}
	p := filepath.Join(t.TempDir(), "tool.exe")
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// A program built with PyInstaller keeps its certificate bundle compressed in the
// archive appended to it: the roots it holds are found there, and named with the file
// that holds them. The block list of a fixed Python's ssl module makes them harmless
// to that Python only: it is kept apart from the program's own block list, and a
// block list pattern in a bundle does not count.
func TestPyInstallerArchive(t *testing.T) {
	es := buildEmbedScanner()
	g3 := rootByName(rootNameECCG3)
	h, _ := hex.DecodeString(uniqueBadSPKIs()[0])
	bundle := []pyiFile{{`certifi\cacert.pem`, compressible(g3.PEM + "\n"), true}, {"base_library.zip", bytes.Repeat([]byte("x"), 5000), true}}
	withSSL := append(append([]pyiFile{}, bundle...), pyiFile{"_ssl.pyd", compressible("MZ code " + string(h)), true})
	hashInBundle := []pyiFile{{`certifi\cacert.pem`, compressible("# " + badCerts[0].SHA256 + "\n" + g3.PEM), true}}
	pemLine := []byte(pemLines(g3.DER)[1])
	hidden(t, pyiArchiveBytes(withSSL), h, pemLine)
	hidden(t, pyiArchiveBytes(hashInBundle), []byte(badCerts[0].SHA256), pemLine)
	for _, c := range []struct {
		name               string
		files              []pyiFile
		signed             bool
		roots, pyFixed     bool
		blocklist, harmful bool
	}{
		{"bundle", bundle, false, true, false, false, true},
		{"signed program", bundle, true, true, false, false, true},
		{"stored without compression", []pyiFile{{"certifi/cacert.pem", []byte(g3.PEM), false}}, false, true, false, false, true},
		{"fixed Python", withSSL, false, true, true, false, false},
		{"block list pattern in a bundle", hashInBundle, false, true, false, false, true},
		{"no bundle", []pyiFile{{"base_library.zip", []byte("data"), true}}, false, false, false, false, false},
	} {
		f, err := es.scanFile(pyiProgram(t, pyiArchiveBytes(c.files), c.signed), scanRoots)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if f.HasEmsignRoot() != c.roots || f.PackedPyFixed != c.pyFixed || f.Blocklist != c.blocklist || (c.roots && !f.packedHarmless()) != c.harmful {
			t.Errorf("%s: %+v", c.name, f)
		}
		// stored without compression, the bundle is also seen in the program itself
		if c.roots && (len(f.Packed) != 1 || f.Packed[0] != c.files[0].name || f.PackedOnly != c.files[0].compress) {
			t.Errorf("%s: packed %v", c.name, f.Packed)
		}
	}
	// roots in the program itself too: the packed Python does not make them harmless
	own := append([]byte("MZ "), []byte(g3.PEM)...)
	p := pyiProgram(t, append(own, pyiArchiveBytes(withSSL)...), false)
	if f, _ := es.scanFile(p, scanRoots); !f.HasEmsignRoot() || f.PackedOnly || f.packedHarmless() {
		t.Errorf("roots of the program itself: %+v", f)
	}
	// the compressed bundle itself shows no root to a plain reading of the program
	if f, _ := es.scan(bytes.NewReader(pyiArchiveBytes(bundle)), scanRoots); f.HasRoots() {
		t.Errorf("compressed data should not match: %+v", f)
	}
}

// A damaged archive, or data that only looks like a cookie, is passed over.
func TestPyInstallerDamaged(t *testing.T) {
	es := buildEmbedScanner()
	g3 := rootByName(rootNameECCG3)
	good := pyiArchiveBytes([]pyiFile{{`certifi\cacert.pem`, compressible(g3.PEM), true}})
	hidden(t, good, []byte(pemLines(g3.DER)[1]))
	tocLenAt := len(good) - pyiCookieLen + 16
	for name, archive := range map[string][]byte{
		"table of contents too long": func() []byte {
			b := bytes.Clone(good)
			binary.BigEndian.PutUint32(b[tocLenAt:], 1<<30)
			return b
		}(),
		"archive longer than the file": func() []byte {
			b := bytes.Clone(good)
			binary.BigEndian.PutUint32(b[len(b)-pyiCookieLen+8:], 1<<30)
			return b
		}(),
		"entry length past the table": func() []byte {
			b := bytes.Clone(good)
			tocOff := int(binary.BigEndian.Uint32(b[len(b)-pyiCookieLen+12:]))
			binary.BigEndian.PutUint32(b[tocOff:], 1<<20)
			return b
		}(),
		"magic only": []byte("data " + pyiMagic),
	} {
		f, err := es.scanFile(pyiProgram(t, archive, false), scanRoots)
		if err != nil || f.HasRoots() || len(f.Packed) > 0 {
			t.Errorf("%s: %+v %v", name, f, err)
		}
	}
}

// The files read from one archive are limited in number (each counted once), and
// their names, which the program decides, keep no markup of the details.
func TestPyInstallerLimits(t *testing.T) {
	var files []pyiFile
	for i := 0; i < 100; i++ {
		files = append(files, pyiFile{"f" + itoa(i) + ".pem", []byte("data " + itoa(i)), false})
	}
	archive := pyiArchiveBytes(files)
	if got := pyiPacked(bytes.NewReader(archive), []int64{int64(len(archive))}); len(got) != pyiMaxFiles {
		t.Errorf("%d files read", len(got))
	}
	for name, want := range map[string]string{
		`certifi\cacert.pem`:                    `certifi\cacert.pem`,
		"a\n$ cmd.pem":                          "a__ cmd.pem",
		"[[x|open:C:\\cmd.exe>/c]]**bold**.pem": "__x|open:C:\\cmd.exe>/c____bold__.pem",
		strings.Repeat("a", 150) + ".pem":       strings.Repeat("a", 99) + "…",
	} {
		if got := pyiDisplayName(name); got != want {
			t.Errorf("%q: %q, want %q", name, got, want)
		}
	}
}

func TestPyInstallerWorthReading(t *testing.T) {
	for name, want := range map[string]bool{
		`certifi\cacert.pem`: true, "certifi/cacert.pem": true, "ca-bundle.CRT": true, "x.cer": true,
		"_ssl.pyd": true, "_ssl.cp314-win_amd64.pyd": true, "_socket.pyd": false, "python314.dll": false,
		"base_library.zip": false, "PYZ.pyz": false,
	} {
		if got := pyiWorthReading(name); got != want {
			t.Errorf("%s: %v", name, got)
		}
	}
}

// A program with a packed outdated bundle needs an update; when its packed Python
// rejects the compromised certificates itself, the bundle is reported for information.
func TestPyInstallerVerdict(t *testing.T) {
	Lang = 0
	g3 := rootByName(rootNameECCG3)
	h, _ := hex.DecodeString(uniqueBadSPKIs()[0])
	bundle := []pyiFile{{`certifi\cacert.pem`, compressible(g3.PEM), true}}
	withSSL := append(append([]pyiFile{}, bundle...), pyiFile{"_ssl.pyd", compressible("MZ code " + string(h)), true})
	hidden(t, pyiArchiveBytes(withSSL), h, []byte(pemLines(g3.DER)[1]))
	for _, c := range []struct {
		files  []pyiFile
		status Status
		want   string
	}{
		{bundle, StWarn, Tf("embed.packedIn", `certifi\cacert.pem`)},
		{withSSL, StInfo, T("embed.packedHarmless")},
	} {
		tp := testApps(t)
		p := pyiProgram(t, pyiArchiveBytes(c.files), false)
		it := tp.c.begin(GroupApps, "tool", "tool", filepath.Dir(p), func(it *Item) { tp.toolsOne(it, []string{p}) })
		if it.Status != c.status || !strings.Contains(it.Details, c.want) {
			t.Errorf("%v %q:\n%s", it.Status, it.Summary, it.Details)
		}
	}
}
