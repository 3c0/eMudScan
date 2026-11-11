package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// programWith writes a program that holds data, among other bytes.
func programWith(t *testing.T, path string, data []byte) {
	t.Helper()
	fill := bytes.Repeat([]byte("program data "), 30000) // 390 KB
	os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, append(append(append([]byte{}, fill...), data...), fill...), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A bundle that is, byte for byte, in the data of a program of its folder or of the
// folder above is a copy of the root store built into that program: the details of
// both say so, before and after a fix replaced the bundle. Their statuses do not
// change.
func TestBuiltInCopy(t *testing.T) {
	Lang = 0
	root := t.TempDir()
	list := outdatedBundle(t)
	exe := filepath.Join(root, "example-app-32", "example-app-32.exe")
	programWith(t, exe, list)
	pem := filepath.Join(root, "example-app-32", "data", "cacert.pem")
	os.MkdirAll(filepath.Dir(pem), 0o755)
	os.WriteFile(pem, list, 0o644)

	check := func() (bundle, program Item) {
		tp := testApps(t)
		tp.d = &discovery{bundles: map[string]bool{pem: true}}
		b := tp.c.beginFile(GroupApps, "bundle", "Certificate bundle", pem, func(it *Item) { tp.bundleOne(it, pem) })
		p := tp.c.begin(GroupApps, "tool", "Example App 32", filepath.Dir(exe), func(it *Item) { tp.toolsOne(it, []string{exe}) })
		return *b, *p
	}
	b, p := check()
	if b.Status != StWarn || !strings.Contains(b.Details, Tf("bundle.builtIn", lk(exe))) {
		t.Errorf("bundle: %v\n%s", b.Status, b.Details)
	}
	if p.Status != StWarn || p.Summary != T("embed.sumBad") || !strings.Contains(p.Details, Tf("embed.copy", lk(pem))) {
		t.Errorf("program: %v %q\n%s", p.Status, p.Summary, p.Details)
	}

	// a fix replaced the bundle: its backup is the copy
	os.WriteFile(pem+".emudscan-20261101-120000.bak", list, 0o644)
	os.WriteFile(pem, upToDateBundle(), 0o644)
	b, p = check()
	if b.Status != StOK || !strings.Contains(b.Details, Tf("bundle.builtInReplaced", lk(exe))) || strings.Contains(b.Details, Tf("bundle.builtIn", lk(exe))) {
		t.Errorf("bundle replaced: %v\n%s", b.Status, b.Details)
	}
	if p.Status != StWarn || !strings.Contains(p.Details, Tf("embed.copyReplaced", lk(pem))) || strings.Contains(p.Details, Tf("embed.copy", lk(pem))) {
		t.Errorf("program, bundle replaced: %v\n%s", p.Status, p.Details)
	}

	// the backup put back: as before the fix
	os.WriteFile(pem, list, 0o644)
	os.Remove(pem + ".emudscan-20261101-120000.bak")
	if b, p = check(); !strings.Contains(b.Details, Tf("bundle.builtIn", lk(exe))) || !strings.Contains(p.Details, Tf("embed.copy", lk(pem))) {
		t.Errorf("bundle put back:\n%s\n%s", b.Details, p.Details)
	}
}

// Nothing is said when it is not certain: a bundle that differs from the root store
// of the program, even by one byte; a bundle two folders below the program; a
// program without a root store, whatever it holds.
func TestBuiltInCopyNot(t *testing.T) {
	Lang = 0
	list := outdatedBundle(t)
	said := func(exe, pem string) bool {
		tp := testApps(t)
		tp.d = &discovery{bundles: map[string]bool{pem: true}}
		b := tp.c.beginFile(GroupApps, "bundle", "Certificate bundle", pem, func(it *Item) { tp.bundleOne(it, pem) })
		p := tp.c.begin(GroupApps, "tool", "Example App 33", filepath.Dir(exe), func(it *Item) { tp.toolsOne(it, []string{exe}) })
		_, found := tp.builtInSource(pem)
		return found || strings.Contains(b.Details, lk(exe)) || strings.Contains(p.Details, lk(pem))
	}
	write := func(p string, data []byte) string {
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, data, 0o644)
		return p
	}

	root := t.TempDir()
	exe := filepath.Join(root, "example-app-33.exe")
	programWith(t, exe, list)
	if !said(exe, write(filepath.Join(root, "cacert.pem"), list)) { // the same folder: said
		t.Error("a copy in the folder of the program is not seen")
	}
	if said(exe, write(filepath.Join(root, "cacert.pem"), append(append([]byte{}, list...), '\n'))) {
		t.Error("one more byte: not a copy")
	}
	if said(exe, write(filepath.Join(root, "cacert.pem"), bytes.ReplaceAll(list, []byte("\n"), []byte("\r\n")))) {
		t.Error("other line ends: not a copy")
	}
	os.Remove(filepath.Join(root, "cacert.pem"))
	if said(exe, write(filepath.Join(root, "a", "b", "cacert.pem"), list)) {
		t.Error("two folders below the program: not looked at")
	}

	// a program that holds the bytes of the bundle, in which eMudScan finds no root store
	root = t.TempDir()
	plain := bytes.Repeat([]byte("not a certificate list\n"), 400)
	exe = filepath.Join(root, "example-app-34.exe")
	programWith(t, exe, plain)
	if _, found := testApps(t).builtInSource(write(filepath.Join(root, "notes.pem"), plain)); found {
		t.Error("a program without a root store: not a source")
	}
}

// fileHolds finds a pattern wherever it is in a file read by parts: at its start, at
// its end, and across two parts.
func TestFileHolds(t *testing.T) {
	const part = 4 << 20
	pattern := bytes.Repeat([]byte("0123456789abcdef"), 600) // 9600 bytes
	other := bytes.Repeat([]byte("fedcba9876543210"), 600)
	path := filepath.Join(t.TempDir(), "program.exe")
	for _, at := range []int{0, part - len(pattern), part - 5000, part - 1, part, 2*part - 1, 2*part + 300000 - len(pattern)} {
		data := bytes.Repeat([]byte{'.'}, 2*part+300000)
		copy(data[at:], pattern)
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
		if held := fileHolds(path, pattern, other, nil); !held[0] || held[1] || held[2] {
			t.Errorf("pattern at %d: %v", at, held)
		}
	}
	if held := fileHolds(filepath.Join(t.TempDir(), "missing.exe"), pattern); held[0] {
		t.Error("a missing file holds nothing")
	}
}
