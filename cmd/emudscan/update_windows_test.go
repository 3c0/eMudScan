//go:build windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// releaseHeld closes the handles that an update keeps until its process ends.
func releaseHeld() {
	for _, h := range append(updateHeld, updateDirHeld) {
		syscall.CloseHandle(h)
	}
	updateHeld, updateDirHeld = nil, 0
}

// junction makes link, a junction (a link to a folder, which needs no rights) to target.
func junction(t *testing.T, link, target string) {
	t.Helper()
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput(); err != nil {
		t.Skipf("no junction: %v %s", err, out)
	}
}

// The folder of a download: in Temp, under a random name, new each time; kept open,
// so that it can be neither renamed nor deleted until the update ends.
func TestUpdateDirOnWindows(t *testing.T) {
	temp := t.TempDir()
	t.Setenv("TMP", temp)
	t.Setenv("TEMP", temp)
	defer releaseHeld()
	a, err := makeUpdateDir()
	if err != nil {
		t.Fatal(err)
	}
	ha := updateDirHeld
	b, err := makeUpdateDir()
	if err != nil || a == b || !samePath(filepath.Dir(a), temp) || !updateDirRe.MatchString(filepath.Base(a)) || !updateDirRe.MatchString(filepath.Base(b)) {
		t.Fatalf("%q %q %v", a, b, err)
	}
	if err := os.Rename(a, a+"x"); err == nil {
		t.Error("renamed while held")
	}
	if err := os.Remove(a); err == nil {
		t.Error("deleted while held")
	}
	syscall.CloseHandle(ha)
	if err := os.Rename(a, a+"x"); err != nil {
		t.Errorf("not renamed once released: %v", err)
	}
}

// The files of stage 1: new files, checked through the handles that keep them from
// changing; when the update does not start, they are deleted with their folder.
func TestWriteHeldOnWindows(t *testing.T) {
	temp := t.TempDir()
	t.Setenv("TMP", temp)
	t.Setenv("TEMP", temp)
	defer releaseHeld()
	dir, err := makeUpdateDir()
	if err != nil {
		t.Fatal(err)
	}
	exe, dl := filepath.Join(dir, updateExe), filepath.Join(dir, downloadName("9.0.0"))
	if dl != filepath.Join(dir, "eMudScan-9.0.0.exe") {
		t.Errorf("download: %s", dl)
	}
	if err := writeHeld(exe, []byte("self"), sumOf([]byte("self"))); err != nil {
		t.Fatal(err)
	}
	if err := writeHeld(exe, []byte("other"), sumOf([]byte("other"))); err == nil {
		t.Error("written over an existing file")
	}
	if err := writeHeld(dl, []byte("new"), sumOf([]byte("other"))); err == nil {
		t.Error("another SHA-256 accepted")
	}
	for _, p := range []string{exe, dl} {
		if err := os.Remove(p); err == nil {
			t.Errorf("%s: deleted while held", p)
		}
		if f, err := os.OpenFile(p, os.O_WRONLY, 0); err == nil {
			f.Close()
			t.Errorf("%s: opened for writing while held", p)
		}
	}
	os.WriteFile(filepath.Join(temp, "kept.txt"), []byte("x"), 0o644)
	dropUpdateDir(dir)
	if _, err := os.Stat(dir); err == nil {
		t.Error("the folder of the update is still there")
	}
	if _, err := os.Stat(filepath.Join(temp, "kept.txt")); err != nil {
		t.Error("another file deleted")
	}
	// the copy runs from a folder of an update only
	if _, _, err := updateFiles("9.0.0", sumOf([]byte("new")), false); err == nil || err.Error() != T("update.notFromTemp") {
		t.Errorf("from elsewhere: %v", err)
	}
}

// The checked file: others may read it, not change, rename or delete it while it is
// held; a new file is never one that exists, nor a link.
func TestReadLockedOnWindows(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "eMudScan.exe")
	if err := writeNew(p, []byte("new")); err != nil {
		t.Fatal(err)
	}
	if err := writeNew(p, []byte("other")); err == nil {
		t.Error("written over an existing file")
	}
	data, h, err := readLocked(p)
	if err != nil || string(data) != "new" {
		t.Fatalf("%q %v", data, err)
	}
	if f, err := os.OpenFile(p, os.O_WRONLY, 0); err == nil {
		f.Close()
		t.Error("opened for writing while held")
	}
	if err := os.Remove(p); err == nil {
		t.Error("deleted while held")
	}
	if err := os.Rename(p, p+".old"); err == nil {
		t.Error("renamed while held")
	}
	if b, err := os.ReadFile(p); err != nil || string(b) != "new" {
		t.Errorf("not readable while held: %v", err)
	}
	syscall.CloseHandle(h)
	if err := os.Remove(p); err != nil {
		t.Errorf("not deleted once released: %v", err)
	}
	// a link to a folder is refused
	junction(t, filepath.Join(dir, "link"), dir)
	if _, _, err := readLocked(filepath.Join(dir, "link")); err == nil {
		t.Error("read through a junction")
	}
	if _, err := openChecked(filepath.Join(dir, "link"), syscall.GENERIC_READ, syscall.FILE_SHARE_READ, true); err == nil {
		t.Error("a junction opened as a folder")
	}
}

// The replacement: in one step, checked before; nothing left beside the file; the
// file unchanged when it fails.
func TestReplaceFileOnWindows(t *testing.T) {
	defer func(n int) { replaceTries = n }(replaceTries)
	replaceTries = 1
	dir := t.TempDir()
	target := filepath.Join(dir, "eMudScan.exe")
	os.WriteFile(target, []byte("old"), 0o644)
	left := func() []string {
		m, _ := filepath.Glob(filepath.Join(dir, ".eMudScan-update-*.tmp"))
		return m
	}
	if err := replaceFile(target, []byte("new"), sumOf([]byte("new"))); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(target); string(b) != "new" || len(left()) != 0 {
		t.Errorf("%q, left %v", b, left())
	}
	// another SHA-256: nothing changes
	if err := replaceFile(target, []byte("newer"), sumOf([]byte("other"))); err == nil {
		t.Error("replaced with another file")
	}
	if b, _ := os.ReadFile(target); string(b) != "new" || len(left()) != 0 {
		t.Errorf("after another SHA-256: %q, left %v", b, left())
	}
	// in use, as a running program: nothing changes, and it says so
	p, _ := syscall.UTF16PtrFromString(target)
	h, err := syscall.CreateFile(p, syscall.GENERIC_READ, syscall.FILE_SHARE_READ, nil, syscall.OPEN_EXISTING, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	err = replaceFile(target, []byte("newer"), sumOf([]byte("newer")))
	syscall.CloseHandle(h)
	if err == nil || !strings.Contains(err.Error(), T("update.inUse")) {
		t.Errorf("in use: %v", err)
	}
	if b, _ := os.ReadFile(target); string(b) != "new" || len(left()) != 0 {
		t.Errorf("after in use: %q, left %v", b, left())
	}
	// not a program of eMudScan, a relative path, a folder: refused
	for _, p := range []string{filepath.Join(systemDir, "notepad.exe"), "eMudScan.exe", dir, target} {
		if _, err := checkTarget(p); err == nil {
			t.Errorf("%s: accepted", p)
		}
	}
}

// After the update, the copy, the download and their folder are deleted: by their
// names, in Temp only, and never through a link.
func TestUpdateDoneOnWindows(t *testing.T) {
	temp := t.TempDir()
	t.Setenv("TMP", temp)
	t.Setenv("TEMP", temp)
	put := func(dir string) string {
		os.MkdirAll(dir, 0o755)
		os.WriteFile(filepath.Join(dir, "eMudScan.exe"), []byte("old"), 0o644)
		os.WriteFile(filepath.Join(dir, "eMudScan-9.0.0.exe"), []byte("new"), 0o644)
		return dir
	}
	good := put(filepath.Join(temp, "eMudScan-update-0123456789abcdef"))
	updateDone(good, "")
	if _, err := os.Stat(good); err == nil {
		t.Error("the folder of the download is still there")
	}
	// another name, or a folder elsewhere: left
	other := put(filepath.Join(temp, "eMudScan-update-x"))
	elsewhere := put(filepath.Join(t.TempDir(), "eMudScan-update-0123456789abcdef"))
	for _, d := range []string{other, elsewhere} {
		updateDone(d, "")
		if _, err := os.Stat(filepath.Join(d, "eMudScan-9.0.0.exe")); err != nil {
			t.Errorf("%s: deleted", d)
		}
	}
	// a folder with more in it: only the copy and the download go
	more := put(filepath.Join(temp, "eMudScan-update-1123456789abcdef"))
	for _, name := range []string{"kept.txt", "eMudScan-9.0.exe", "eMudScan-x.exe"} {
		os.WriteFile(filepath.Join(more, name), []byte("x"), 0o644)
	}
	updateDone(more, "")
	for _, name := range []string{"kept.txt", "eMudScan-9.0.exe", "eMudScan-x.exe"} {
		if _, err := os.Stat(filepath.Join(more, name)); err != nil {
			t.Errorf("%s deleted", name)
		}
	}
	for _, name := range []string{"eMudScan.exe", "eMudScan-9.0.0.exe"} {
		if _, err := os.Stat(filepath.Join(more, name)); err == nil {
			t.Errorf("%s is still there", name)
		}
	}
	// links: the folder of the download, and its file, as junctions to a folder with a
	// file named as the download: never followed
	victim := put(t.TempDir())
	junction(t, filepath.Join(temp, "eMudScan-update-2123456789abcdef"), victim)
	updateDone(filepath.Join(temp, "eMudScan-update-2123456789abcdef"), "")
	linked := filepath.Join(temp, "eMudScan-update-3123456789abcdef")
	os.MkdirAll(linked, 0o755)
	junction(t, filepath.Join(linked, "eMudScan.exe"), victim)
	junction(t, filepath.Join(linked, "eMudScan-9.0.0.exe"), victim)
	updateDone(linked, "")
	for _, name := range []string{"eMudScan.exe", "eMudScan-9.0.0.exe"} {
		if _, err := os.Stat(filepath.Join(victim, name)); err != nil {
			t.Errorf("%s deleted through a junction", name)
		}
	}
	// a replacement that did not end, beside this program: deleted; another file: left
	self, _ := os.Executable()
	tmp := filepath.Join(filepath.Dir(self), ".eMudScan-update-4123456789abcdef.tmp")
	kept := filepath.Join(filepath.Dir(self), ".eMudScan-update-kept.tmp")
	os.WriteFile(tmp, []byte("x"), 0o644)
	os.WriteFile(kept, []byte("x"), 0o644)
	defer os.Remove(kept)
	updateDone(filepath.Join(temp, "nothing"), "")
	if _, err := os.Stat(tmp); err == nil {
		os.Remove(tmp)
		t.Error("the temporary file of a replacement is still there")
	}
	if _, err := os.Stat(kept); err != nil {
		t.Error("another file deleted")
	}
}
