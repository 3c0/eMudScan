package main

// Generates the rsrc_windows_amd64.syso of each of the two programs (exeFile): a
// COFF object holding its Windows resources (application icon, manifest, version
// information). The Go linker picks up, when building for windows/amd64, the one of
// the package that the program imports (winres/gui for eMudScan.exe, winres/cli for
// eMudScan-cli.exe).
//
//   go run ./cmd/emudscan --make-syso cmd/emudscan/winres/gui/rsrc_windows_amd64.syso
//   go run ./cmd/emudscan --make-syso-cli cmd/emudscan/winres/cli/rsrc_windows_amd64.syso
//
// (from the root of the repository, on any system). Every build regenerates them:
// scripts/build.sh, scripts\build.ps1, scripts/release.sh and the GitHub build. The
// result is the same with any Go version, so the committed files never show as
// changed; TestSysoUpToDate checks that they are up to date.

import (
	"bytes"
	_ "embed"
	"encoding/binary"
	"fmt"
	"image"
	"image/png"
	"os"
	"sort"
	"strings"
	"unicode/utf16"
)

// icon256PNG is the 256-pixel application icon, as renderIcon draws it, compressed
// once: Go's PNG compression changes from one Go version to another, which would
// change the Windows resources, and the exe would say it was built from modified
// files. TestIcon256 checks that it is still the icon of renderIcon; when the
// drawing changes, write it again with
//
//	go run ./cmd/emudscan --make-icon256 cmd/emudscan/icon256.png
//
// then compress it, as docs/building.md says.
//
//go:embed icon256.png
var icon256PNG []byte

// manifestXML is the application manifest; {{version}} is replaced by the version
// in four numbers ("1.0.0.0").
const manifestXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<assembly xmlns="urn:schemas-microsoft-com:asm.v1" manifestVersion="1.0">
  <assemblyIdentity type="win32" name="eMudScan" version="{{version}}" processorArchitecture="*"/>
  <description>Checks whether your PC still trusts the eMudhra certificates compromised in October 2026</description>
  <dependency>
    <dependentAssembly>
      <assemblyIdentity type="win32" name="Microsoft.Windows.Common-Controls" version="6.0.0.0" processorArchitecture="*" publicKeyToken="6595b64144ccf1df" language="*"/>
    </dependentAssembly>
  </dependency>
  <trustInfo xmlns="urn:schemas-microsoft-com:asm.v3">
    <security>
      <requestedPrivileges>
        <requestedExecutionLevel level="asInvoker" uiAccess="false"/>
      </requestedPrivileges>
    </security>
  </trustInfo>
  <compatibility xmlns="urn:schemas-microsoft-com:compatibility.v1">
    <application>
      <supportedOS Id="{8e0f7a12-bfb3-4fe8-b9a5-48fd50a15a9a}"/>
    </application>
  </compatibility>
  <application xmlns="urn:schemas-microsoft-com:asm.v3">
    <windowsSettings>
      <dpiAware xmlns="http://schemas.microsoft.com/SMI/2005/WindowsSettings">true/pm</dpiAware>
      <dpiAwareness xmlns="http://schemas.microsoft.com/SMI/2016/WindowsSettings">PerMonitorV2, PerMonitor</dpiAwareness>
      <longPathAware xmlns="http://schemas.microsoft.com/SMI/2016/WindowsSettings">true</longPathAware>
    </windowsSettings>
  </application>
</assembly>
`

type resItem struct {
	typ, id uint32
	data    []byte
}

const (
	rtIcon      = 3
	rtGroupIcon = 14
	rtVersion   = 16
	rtManifest  = 24
	langEnUS    = 0x0409
)

func le16(b *bytes.Buffer, v uint16) { binary.Write(b, binary.LittleEndian, v) }
func le32(b *bytes.Buffer, v uint32) { binary.Write(b, binary.LittleEndian, v) }

func utf16zLE(s string) []byte {
	u := append(utf16.Encode([]rune(s)), 0)
	b := make([]byte, len(u)*2)
	for i, c := range u {
		binary.LittleEndian.PutUint16(b[i*2:], c)
	}
	return b
}

func pad4(b *bytes.Buffer) {
	for b.Len()%4 != 0 {
		b.WriteByte(0)
	}
}

// iconImage encodes one icon image: the PNG of icon256PNG for 256 px, otherwise a
// 32-bit DIB with AND mask.
func iconImage(size int) []byte {
	if size == 256 {
		return icon256PNG
	}
	px := renderIcon(iconApp, size)
	var b bytes.Buffer
	maskRow := ((size + 31) / 32) * 4
	le32(&b, 40)
	le32(&b, uint32(size))
	le32(&b, uint32(size*2))
	le16(&b, 1)
	le16(&b, 32)
	le32(&b, 0)
	le32(&b, uint32(size*size*4+maskRow*size))
	le32(&b, 0)
	le32(&b, 0)
	le32(&b, 0)
	le32(&b, 0)
	for y := size - 1; y >= 0; y-- { // bottom-up
		for x := 0; x < size; x++ {
			i := (y*size + x) * 4
			b.Write([]byte{px[i+2], px[i+1], px[i], px[i+3]})
		}
	}
	for y := size - 1; y >= 0; y-- {
		row := make([]byte, maskRow)
		for x := 0; x < size; x++ {
			if px[(y*size+x)*4+3] < 128 {
				row[x/8] |= 0x80 >> (x % 8)
			}
		}
		b.Write(row)
	}
	return b.Bytes()
}

// versionNode encodes one VS_VERSIONINFO-style node.
func versionNode(key string, typ uint16, value []byte, valueLen uint16, children ...[]byte) []byte {
	var b bytes.Buffer
	le16(&b, 0) // wLength, patched below
	le16(&b, valueLen)
	le16(&b, typ)
	b.Write(utf16zLE(key))
	pad4(&b)
	b.Write(value)
	for _, c := range children {
		pad4(&b)
		b.Write(c)
	}
	out := b.Bytes()
	binary.LittleEndian.PutUint16(out, uint16(len(out)))
	return out
}

// version4 is appVersion in the four numbers Windows expects: "2.0.0" → "2.0.0.0".
func version4() string {
	v := ParseVersion(appVersion)
	for len(v) < 4 {
		v = append(v, 0)
	}
	return fmt.Sprintf("%d.%d.%d.%d", v[0], v[1], v[2], v[3])
}

// exeFile is one of the two programs built from this code, as its Windows resources
// name it. They have the same icon and manifest.
type exeFile struct {
	file        string // its file name ("OriginalFilename")
	name        string // its internal name
	description string // the name that Task Manager, SmartScreen and the UAC prompt show
}

var (
	exeWindow = exeFile{"eMudScan.exe", "eMudScan", "eMudScan"}                        // the window program
	exeCLI    = exeFile{"eMudScan-cli.exe", "eMudScan-cli", "eMudScan (command line)"} // the console program (cliscan.go)
)

func versionResource(exe exeFile) []byte {
	v := ParseVersion(appVersion)
	for len(v) < 4 {
		v = append(v, 0)
	}
	var ffi bytes.Buffer
	for _, x := range []uint32{0xFEEF04BD, 0x00010000, uint32(v[0]<<16 | v[1]), uint32(v[2]<<16 | v[3]),
		uint32(v[0]<<16 | v[1]), uint32(v[2]<<16 | v[3]), 0x3F, 0, 0x00040004, 1, 0, 0, 0} {
		le32(&ffi, x)
	}
	str := func(k, val string) []byte {
		return versionNode(k, 1, utf16zLE(val), uint16(len(utf16.Encode([]rune(val)))+1))
	}
	table := versionNode("040904b0", 1, nil, 0,
		str("FileDescription", exe.description),
		str("FileVersion", appVersionDate()),
		str("InternalName", exe.name),
		str("OriginalFilename", exe.file),
		str("ProductName", "eMudScan"),
		str("ProductVersion", appVersionDate()),
		str("CompanyName", appAuthor),
		str("LegalCopyright", "Copyright © 2026 "+appAuthor+" - GNU GPL 3.0 or later"),
		// " | " between the parts, so that the description does not run into the
		// author's name
		str("Comments", "Checks whether your PC still trusts the eMudhra certificates compromised in October 2026 | Made by "+appAuthor+" - "+appURL+" | Reference data: emudhra.we-have-a-problem.com, "+dataSourceURL+" (Apache License 2.0)"),
	)
	sfi := versionNode("StringFileInfo", 1, nil, 0, table)
	var tr bytes.Buffer
	le16(&tr, langEnUS)
	le16(&tr, 1200)
	vfi := versionNode("VarFileInfo", 1, nil, 0, versionNode("Translation", 0, tr.Bytes(), 4))
	return versionNode("VS_VERSION_INFO", 0, ffi.Bytes(), 52, sfi, vfi)
}

func resourceItems(exe exeFile) []resItem {
	var items []resItem
	sizes := []int{16, 20, 24, 32, 40, 48, 64, 256}
	var grp bytes.Buffer
	le16(&grp, 0)
	le16(&grp, 1)
	le16(&grp, uint16(len(sizes)))
	for i, s := range sizes {
		data := iconImage(s)
		items = append(items, resItem{rtIcon, uint32(i + 1), data})
		dim := byte(s)
		if s >= 256 {
			dim = 0
		}
		grp.Write([]byte{dim, dim, 0, 0})
		le16(&grp, 1)
		le16(&grp, 32)
		le32(&grp, uint32(len(data)))
		le16(&grp, uint16(i+1))
	}
	items = append(items, resItem{rtGroupIcon, 1, grp.Bytes()})
	items = append(items, resItem{rtVersion, 1, versionResource(exe)})
	items = append(items, resItem{rtManifest, 1, []byte(strings.Replace(manifestXML, "{{version}}", version4(), 1))})
	return items
}

// buildResourceSection lays out the resource tree: type -> id -> language -> data.
// It returns the section bytes and the offsets of the RVA fields to relocate.
func buildResourceSection(items []resItem) ([]byte, []uint32) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].typ != items[j].typ {
			return items[i].typ < items[j].typ
		}
		return items[i].id < items[j].id
	})
	var types []uint32
	byType := map[uint32][]resItem{}
	for _, it := range items {
		if len(byType[it.typ]) == 0 {
			types = append(types, it.typ)
		}
		byType[it.typ] = append(byType[it.typ], it)
	}
	dirSize := func(n int) int { return 16 + 8*n }
	off := dirSize(len(types))
	typeDirOff := map[uint32]int{}
	for _, t := range types {
		typeDirOff[t] = off
		off += dirSize(len(byType[t]))
	}
	langDirOff := make([]int, len(items))
	for i := range items {
		langDirOff[i] = off
		off += dirSize(1)
	}
	dataEntryOff := make([]int, len(items))
	for i := range items {
		dataEntryOff[i] = off
		off += 16
	}
	dataOff := make([]int, len(items))
	for i, it := range items {
		off = (off + 7) &^ 7
		dataOff[i] = off
		off += len(it.data)
	}
	sec := make([]byte, (off+7)&^7)
	put32 := func(o int, v uint32) { binary.LittleEndian.PutUint32(sec[o:], v) }
	put16 := func(o int, v uint16) { binary.LittleEndian.PutUint16(sec[o:], v) }
	dir := func(o, n int) { put16(o+14, uint16(n)) } // NumberOfIdEntries
	dir(0, len(types))
	idx := 0
	for ti, t := range types {
		put32(16+ti*8, t)
		put32(16+ti*8+4, 0x80000000|uint32(typeDirOff[t]))
		dir(typeDirOff[t], len(byType[t]))
		for j, it := range byType[t] {
			e := typeDirOff[t] + 16 + j*8
			put32(e, it.id)
			put32(e+4, 0x80000000|uint32(langDirOff[idx]))
			dir(langDirOff[idx], 1)
			put32(langDirOff[idx]+16, langEnUS)
			put32(langDirOff[idx]+20, uint32(dataEntryOff[idx]))
			idx++
		}
	}
	var relocs []uint32
	for i, it := range items {
		put32(dataEntryOff[i], uint32(dataOff[i])) // relocated to an RVA by the linker
		put32(dataEntryOff[i]+4, uint32(len(it.data)))
		copy(sec[dataOff[i]:], it.data)
		relocs = append(relocs, uint32(dataEntryOff[i]))
	}
	return sec, relocs
}

func writeSyso(path string, exe exeFile) error { return writeIfChanged(path, sysoBytes(exe)) }

// writeIfChanged writes data to path, unless the file already holds exactly this
// data: every build regenerates the Windows resources, and rewriting the file would
// change its modification time, so that go test would run again the tests that
// read it (TestSysoUpToDate), all of them, instead of reusing their results.
func writeIfChanged(path string, data []byte) error {
	if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, data) {
		return nil
	}
	return os.WriteFile(path, data, 0o644)
}

// sysoBytes is the content of the rsrc_windows_amd64.syso of a program.
func sysoBytes(exe exeFile) []byte {
	sec, relocs := buildResourceSection(resourceItems(exe))
	var b bytes.Buffer
	const hdr, shdr = 20, 40
	relocOff := hdr + shdr + len(sec)
	symOff := relocOff + 10*len(relocs)
	// COFF file header
	le16(&b, 0x8664)
	le16(&b, 1)
	le32(&b, 0)
	le32(&b, uint32(symOff))
	le32(&b, 1)
	le16(&b, 0)
	le16(&b, 0)
	// section header
	b.Write([]byte(".rsrc\x00\x00\x00"))
	le32(&b, 0)
	le32(&b, 0)
	le32(&b, uint32(len(sec)))
	le32(&b, hdr+shdr)
	le32(&b, uint32(relocOff))
	le32(&b, 0)
	le16(&b, uint16(len(relocs)))
	le16(&b, 0)
	le32(&b, 0x40000040) // INITIALIZED_DATA | MEM_READ
	b.Write(sec)
	for _, r := range relocs {
		le32(&b, r)
		le32(&b, 0) // symbol 0 (.rsrc)
		le16(&b, 3) // IMAGE_REL_AMD64_ADDR32NB
	}
	// symbol table: .rsrc, section 1, static
	b.Write([]byte(".rsrc\x00\x00\x00"))
	le32(&b, 0)
	le16(&b, 1)
	le16(&b, 0)
	b.WriteByte(3)
	b.WriteByte(0)
	le32(&b, 4) // empty string table
	return b.Bytes()
}

// icon256Encoded is the 256-pixel application icon as renderIcon draws it, encoded
// by Go's PNG encoder: what --make-icon256 writes, to be compressed.
func icon256Encoded() []byte {
	img := image.NewNRGBA(image.Rect(0, 0, 256, 256))
	copy(img.Pix, renderIcon(iconApp, 256))
	var b bytes.Buffer
	png.Encode(&b, img)
	return b.Bytes()
}

// toolCommand runs the commands that make files from the source, on any system:
// --make-syso and --make-syso-cli (the Windows resources of eMudScan.exe and of
// eMudScan-cli.exe), --make-ico (the icon as a .ico file) and --make-icon256 (the
// 256-pixel icon, to compress into icon256.png), each followed by the path of the
// file to write. It reports whether args is one of them; it exits with status 1 if
// the file cannot be written.
func toolCommand(args []string) bool {
	if len(args) != 2 {
		return false
	}
	var err error
	switch args[0] {
	case "--make-syso":
		err = writeSyso(args[1], exeWindow)
	case "--make-syso-cli":
		err = writeSyso(args[1], exeCLI)
	case "--make-ico":
		err = writeICO(args[1])
	case "--make-icon256":
		err = os.WriteFile(args[1], icon256Encoded(), 0o644)
	default:
		return false
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	return true
}

// writeICO saves the application icon as a .ico file (for documentation and checks).
func writeICO(path string) error {
	sizes := []int{16, 24, 32, 48, 64, 256}
	var b bytes.Buffer
	le16(&b, 0)
	le16(&b, 1)
	le16(&b, uint16(len(sizes)))
	var imgs [][]byte
	for _, s := range sizes {
		imgs = append(imgs, iconImage(s))
	}
	off := 6 + 16*len(sizes)
	for i, s := range sizes {
		dim := byte(s)
		if s >= 256 {
			dim = 0
		}
		b.Write([]byte{dim, dim, 0, 0})
		le16(&b, 1)
		le16(&b, 32)
		le32(&b, uint32(len(imgs[i])))
		le32(&b, uint32(off))
		off += len(imgs[i])
	}
	for _, im := range imgs {
		b.Write(im)
	}
	return os.WriteFile(path, b.Bytes(), 0o644)
}
