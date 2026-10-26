package main

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// fakePE builds a minimal 64-bit program with a time stamp, an optional debug entry
// and an optional signature (certificate table).
func fakePE(t *testing.T, stamp uint32, debugType uint32, sig []byte) string {
	t.Helper()
	const peOff, optSize = 0x40, 240
	b := make([]byte, 0x400)
	copy(b, "MZ")
	binary.LittleEndian.PutUint32(b[0x3C:], peOff)
	copy(b[peOff:], "PE\x00\x00")
	coff := peOff + 4
	binary.LittleEndian.PutUint16(b[coff:], 0x8664)
	binary.LittleEndian.PutUint16(b[coff+2:], 1) // one section
	binary.LittleEndian.PutUint32(b[coff+4:], stamp)
	binary.LittleEndian.PutUint16(b[coff+16:], optSize)
	opt := coff + 20
	binary.LittleEndian.PutUint16(b[opt:], 0x20b)
	binary.LittleEndian.PutUint32(b[opt+108:], 16)
	sect := opt + optSize
	binary.LittleEndian.PutUint32(b[sect+8:], 0x200)   // virtual size
	binary.LittleEndian.PutUint32(b[sect+12:], 0x1000) // virtual address
	binary.LittleEndian.PutUint32(b[sect+16:], 0x200)  // raw size
	binary.LittleEndian.PutUint32(b[sect+20:], 0x200)  // raw offset
	if debugType != 0 {
		binary.LittleEndian.PutUint32(b[opt+112+8*6:], 0x1010)
		binary.LittleEndian.PutUint32(b[opt+112+8*6+4:], 28)
		binary.LittleEndian.PutUint32(b[0x210+12:], debugType)
	}
	if sig != nil {
		binary.LittleEndian.PutUint32(b[opt+112+8*4:], uint32(len(b)))
		binary.LittleEndian.PutUint32(b[opt+112+8*4+4:], uint32(8+len(sig)))
		hdr := make([]byte, 8)
		binary.LittleEndian.PutUint32(hdr, uint32(8+len(sig)))
		binary.LittleEndian.PutUint16(hdr[4:], 0x200)
		binary.LittleEndian.PutUint16(hdr[6:], 2)
		b = append(append(b, hdr...), sig...)
	}
	p := filepath.Join(t.TempDir(), "prog.exe")
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSigningTime(t *testing.T) {
	sig, err := os.ReadFile("testdata/authenticode-npp-8.7.1.der")
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2024, 10, 30, 23, 56, 19, 0, time.UTC) // RFC 3161 time stamp of Notepad++ 8.7.1
	if got, ok := pkcs7Time(sig, 0); !ok || !got.Equal(want) {
		t.Fatalf("pkcs7Time = %v %v, want %v", got, ok, want)
	}
	p := fakePE(t, 0, 0, sig)
	if got, ok := signingTime(p); !ok || !got.Equal(want) {
		t.Errorf("signingTime = %v %v", got, ok)
	}
	if got, src := publisherDate(p, false); src != dateSigned || !got.Equal(want) {
		t.Errorf("publisherDate = %v %v", got, src)
	}
}

func TestBuildTime(t *testing.T) {
	stamp := uint32(time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC).Unix())
	if got, ok := peBuildTime(fakePE(t, stamp, 2 /* CodeView */, nil)); !ok || got.Unix() != int64(stamp) {
		t.Errorf("build time = %v %v", got, ok)
	}
	if _, ok := peBuildTime(fakePE(t, stamp, 16 /* repro */, nil)); ok {
		t.Error("a reproducible build has no build date")
	}
	if _, ok := peBuildTime(fakePE(t, 0x2A425E19 /* Delphi, 1992 */, 0, nil)); ok {
		t.Error("1992 is not a build date")
	}
	if _, ok := peBuildTime(fakePE(t, 0, 0, nil)); ok {
		t.Error("0 is not a build date")
	}
	// without creation time (not Windows), the modified date comes first
	p := fakePE(t, stamp, 0, nil)
	old := time.Date(2025, 7, 7, 1, 51, 35, 0, time.Local)
	os.Chtimes(p, old, old)
	if got, src := publisherDate(p, false); src != dateModified || !got.Equal(old) {
		t.Errorf("publisherDate = %v %v", got, src)
	}
}

func TestPickDate(t *testing.T) {
	day := func(m time.Month, d int) time.Time { return time.Date(2026, m, d, 12, 0, 0, 0, time.UTC) }
	built := day(9, 30)
	withBuild := func() (time.Time, bool) { return built, true }
	noBuild := func() (time.Time, bool) { return time.Time{}, false }
	for _, c := range []struct {
		name     string
		mt, ct   time.Time
		build    func() (time.Time, bool)
		want     time.Time
		wantFrom dateSource
	}{
		{"kept from an archive", day(7, 7), day(10, 1), withBuild, day(7, 7), dateModified},
		{"written at installation", day(10, 1), day(10, 1).Add(30 * time.Second), withBuild, built, dateBuilt},
		{"written at installation, no build date", day(10, 1), day(10, 1), noBuild, day(10, 1), dateModified},
		{"reproducible build placeholder", time.Date(2000, 1, 1, 1, 0, 0, 0, time.UTC), day(10, 1), noBuild, time.Time{}, dateNone},
		{"update written over an older file", day(10, 8), day(3, 1), withBuild, built, dateBuilt},
		{"update written over an older file, no build date", day(10, 8), day(3, 1), noBuild, day(10, 8), dateModified},
	} {
		got, from := pickDate(c.mt, c.ct, true, c.build)
		if from != c.wantFrom || !got.Equal(c.want) {
			t.Errorf("%s: %v %v, want %v %v", c.name, got, from, c.want, c.wantFrom)
		}
	}
}

func TestMainExeIn(t *testing.T) {
	dir := t.TempDir()
	write := func(rel string, size int) {
		p := filepath.Join(dir, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, make([]byte, size), 0o644)
	}
	write("Stirling PDF/Stirling-PDF.exe", 10)
	write("Stirling PDF/unins000.exe", 500)
	write("Stirling PDF/helper.exe", 400)
	if got, named := mainExeIn(filepath.Join(dir, "Stirling PDF"), "Stirling PDF"); filepath.Base(got) != "Stirling-PDF.exe" || !named {
		t.Errorf("named like the software: %s", got)
	}
	write("Tool/a.exe", 10)
	write("Tool/b.exe", 90)
	if got, named := mainExeIn(filepath.Join(dir, "Tool"), "Something"); filepath.Base(got) != "b.exe" || named {
		t.Errorf("largest: %s", got)
	}
	write("Inkscape/bin/inkscape.exe", 10)
	if got, _ := mainExeIn(filepath.Join(dir, "Inkscape"), "Inkscape"); filepath.Base(got) != "inkscape.exe" {
		t.Errorf("bin folder: %s", got)
	}
	write("Discord/Update.exe", 900)
	write("Discord/Discord.exe", 1)
	write("Discord/app-1.0.9214/Discord.exe", 100)
	write("Discord/app-1.0.9215/Discord.exe", 100)
	if got, _ := mainExeIn(filepath.Join(dir, "Discord"), "Discord"); got != filepath.Join(dir, "Discord", "app-1.0.9215", "Discord.exe") {
		t.Errorf("Squirrel: %s", got)
	}
}
