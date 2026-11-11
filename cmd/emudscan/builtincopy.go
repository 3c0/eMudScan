package main

// A program can carry a root store in its data and write it to a file of its folder
// when that file is missing. eMudScan then finds the same list twice: in the program
// and as a certificate bundle. The fix of the bundle replaces the file, but the
// program still carries the outdated list and can write it again: after the file
// is deleted, or after an update of the program in a new folder.
//
// eMudScan says so in the details of both, when it is certain: when the bundle is,
// byte for byte, in the data of a program of its folder or of the folder above. After
// a fix replaced the bundle, its backup is what the program holds.

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"sort"
)

const (
	copyMinSize = 4 << 10 // a whole list, not a certificate
	copyMaxSize = 8 << 20
	copyMaxExes = 16 // the programs read in a folder: beyond, none
)

// builtInCopy is a bundle that is a copy of the root store built into a program.
type builtInCopy struct {
	exe, bundle string
	replaced    bool // a fix replaced the bundle: its backup is the copy
}

// builtInSource finds the program that holds the bundle p byte for byte, or its
// backup when a fix replaced it: a program of the folder of p or of the folder above,
// in which eMudScan found a root store.
func (tp *appsContext) builtInSource(p string) (builtInCopy, bool) {
	cur := wholeList(p)
	var old []byte
	if b, ok := latestBackup(p); ok && !b.added {
		old = wholeList(b.path)
	}
	if cur == nil && old == nil {
		return builtInCopy{}, false
	}
	dir := filepath.Dir(p)
	for _, d := range []string{dir, filepath.Dir(dir)} {
		for _, exe := range programsIn(d) {
			if f, err := tp.scan(exe, scanRoots); err != nil || !f.HasRoots() {
				continue
			}
			switch held := fileHolds(exe, cur, old); {
			case held[0]:
				return builtInCopy{exe: exe, bundle: p}, true
			case held[1]:
				return builtInCopy{exe: exe, bundle: p, replaced: true}, true
			}
		}
	}
	return builtInCopy{}, false
}

// builtInCopies are the bundles that are copies of the root store built into the
// program exe, among those that eMudScan lists.
func (tp *appsContext) builtInCopies(exe string) []builtInCopy {
	if tp.d == nil {
		return nil
	}
	var out []builtInCopy
	for _, p := range dedupePaths(tp.d.bundles) {
		dir := filepath.Dir(p)
		if pathKey(dir) != pathKey(filepath.Dir(exe)) && pathKey(filepath.Dir(dir)) != pathKey(filepath.Dir(exe)) || tp.isHandled(p) {
			continue
		}
		if c, ok := tp.builtInSource(p); ok && pathKey(c.exe) == pathKey(exe) {
			out = append(out, c)
		}
	}
	return out
}

// copyNotes says, in the details of programs with a root store, which bundles are
// copies of it.
func (tp *appsContext) copyNotes(exes []string) string {
	var notes []string
	for _, exe := range exes {
		for _, c := range tp.builtInCopies(exe) {
			key := "embed.copy"
			if c.replaced {
				key = "embed.copyReplaced"
			}
			notes = append(notes, Tf(key, lk(c.bundle)))
		}
	}
	return joinParas(notes...)
}

// wholeList is the content of a file that can be a whole certificate list, or nil.
func wholeList(p string) []byte {
	fi, err := os.Stat(p)
	if err != nil || !fi.Mode().IsRegular() || fi.Size() < copyMinSize || fi.Size() > copyMaxSize {
		return nil
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	return data
}

// programsIn are the programs of a folder that can hold a root store, as the scan of
// the programs takes them; none when the folder has too many.
func programsIn(dir string) []string {
	var out []string
	for _, e := range readDir(dir) {
		if lower(filepath.Ext(e.Name())) != ".exe" || !e.Type().IsRegular() {
			continue
		}
		p := filepath.Join(dir, e.Name())
		if fi, err := os.Stat(p); err != nil || fi.Size() < 256<<10 || fi.Size() > 800<<20 || isCloudPlaceholder(fi) {
			continue
		}
		if out = append(out, p); len(out) > copyMaxExes {
			return nil
		}
	}
	sort.Strings(out)
	return out
}

// fileHolds tells, for each of the patterns, whether the file holds it. The file is
// read by parts: a program can be large. A nil pattern is never held.
func fileHolds(path string, patterns ...[]byte) []bool {
	held := make([]bool, len(patterns))
	longest := 0
	for _, p := range patterns {
		longest = max(longest, len(p))
	}
	f, err := os.Open(path)
	if err != nil || longest == 0 {
		return held
	}
	defer f.Close()
	const part = 4 << 20
	buf := make([]byte, 0, part+longest)
	for {
		n, err := io.ReadFull(f, buf[len(buf):len(buf)+part])
		buf = buf[:len(buf)+n]
		for i, p := range patterns {
			held[i] = held[i] || len(p) > 0 && bytes.Contains(buf, p)
		}
		if err != nil { // the end of the file
			return held
		}
		// the end of this part can start a pattern that the next part ends
		keep := min(longest-1, len(buf))
		buf = buf[:copy(buf, buf[len(buf)-keep:])]
	}
}
