package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The 256-pixel icon of the Windows resources is the icon that renderIcon draws, in
// the PNG format that every version of Windows reads: 8-bit RGBA, not interlaced,
// with only the IHDR, IDAT and IEND chunks (no metadata).
func TestIcon256(t *testing.T) {
	const redo = "write it again with go run ./cmd/emudscan --make-icon256 cmd/emudscan/icon256.png, then compress it as docs/building.md says"
	b := icon256PNG
	if len(b) < 33 || !bytes.HasPrefix(b, []byte("\x89PNG\r\n\x1a\n")) {
		t.Fatal("icon256.png is not a PNG file: " + redo)
	}
	var chunks []string
	for i := 8; i+12 <= len(b); i += 12 + int(binary.BigEndian.Uint32(b[i:])) {
		chunks = append(chunks, string(b[i+4:i+8]))
	}
	for i, c := range chunks {
		if (i == 0 && c != "IHDR") || (i == len(chunks)-1 && c != "IEND") || (i > 0 && i < len(chunks)-1 && c != "IDAT") {
			t.Errorf("icon256.png has the chunks %v; want IHDR, IDAT…, IEND only: %s", chunks, redo)
			break
		}
	}
	// IHDR: width, height, bit depth, color type, compression, filter, interlace
	w, h := binary.BigEndian.Uint32(b[16:]), binary.BigEndian.Uint32(b[20:])
	if w != 256 || h != 256 || b[24] != 8 || b[25] != 6 || b[28] != 0 {
		t.Errorf("icon256.png: %dx%d, bit depth %d, color type %d, interlace %d; want 256x256, 8, 6 (RGBA), 0: %s",
			w, h, b[24], b[25], b[28], redo)
	}
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("icon256.png: %v: %s", err, redo)
	}
	px, ok := img.(*image.NRGBA)
	if !ok || px.Stride != 4*256 || !bytes.Equal(px.Pix, renderIcon(iconApp, 256)) {
		t.Error("icon256.png is not the icon that renderIcon draws: " + redo)
	}
}

// The committed Windows resources of both programs are those that --make-syso and
// --make-syso-cli write, so the build does not change them, and the exes do not say
// they were built from modified files. Each program is named in its own.
func TestSysoUpToDate(t *testing.T) {
	for _, x := range []struct {
		dir, command string
		exe          exeFile
	}{{"gui", "--make-syso", exeWindow}, {"cli", "--make-syso-cli", exeCLI}} {
		path := "winres/" + x.dir + "/rsrc_windows_amd64.syso"
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(b, sysoBytes(x.exe)) {
			t.Errorf("%s is not up to date: regenerate it with go run ./cmd/emudscan %s cmd/emudscan/%s", path, x.command, path)
		}
		if name := utf16zLE(x.exe.file); !bytes.Contains(b, name) {
			t.Errorf("%s does not name %s", path, x.exe.file)
		}
	}
	if bytes.Equal(sysoBytes(exeWindow), sysoBytes(exeCLI)) || !bytes.Contains(sysoBytes(exeWindow), utf16zLE("eMudScan.exe")) ||
		bytes.Contains(sysoBytes(exeWindow), utf16zLE("eMudScan-cli.exe")) {
		t.Error("the two programs must have their own version information")
	}
}

// writeIfChanged leaves a file that already holds the data untouched, with its
// modification time, and rewrites one that does not.
func TestWriteIfChanged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f")
	if err := writeIfChanged(path, []byte("one")); err != nil {
		t.Fatal(err)
	}
	past := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := os.Chtimes(path, past, past); err != nil {
		t.Fatal(err)
	}
	if err := writeIfChanged(path, []byte("one")); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(path); err != nil || !fi.ModTime().Equal(past) {
		t.Errorf("same data: the file was rewritten (%v, %v)", fi.ModTime(), err)
	}
	if err := writeIfChanged(path, []byte("two")); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != "two" {
		t.Errorf("new data: the file holds %q", b)
	}
}
