package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/png"
	"os"
	"testing"
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

// The committed Windows resources are those that --make-syso writes, so the build
// does not change them, and the exe does not say it was built from modified files.
func TestSysoUpToDate(t *testing.T) {
	b, err := os.ReadFile("rsrc_windows_amd64.syso")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b, sysoBytes()) {
		t.Error("rsrc_windows_amd64.syso is not up to date: regenerate it with go run ./cmd/emudscan --make-syso cmd/emudscan/rsrc_windows_amd64.syso")
	}
}
