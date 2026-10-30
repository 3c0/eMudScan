package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
)

// walkTree, which reads several folders at once, visits exactly what filepath.WalkDir
// visits, with the same folders skipped, on a made-up tree and on Go's own files.
func TestWalkTreeLikeWalkDir(t *testing.T) {
	root := t.TempDir()
	for i := range 12 {
		for j := range 9 {
			for k := range 7 {
				dir := filepath.Join(root, fmt.Sprint("a", i), fmt.Sprint("b", j), fmt.Sprint("c", k))
				if k == 3 {
					dir = filepath.Join(filepath.Dir(dir), "cache", "inner")
				}
				os.MkdirAll(dir, 0o755)
				os.WriteFile(filepath.Join(dir, "f.txt"), nil, 0o644)
			}
		}
	}
	os.Symlink(filepath.Join(root, "a1"), filepath.Join(root, "a0", "link")) // not entered
	enter := func(p string, e fs.DirEntry) bool { return e.Name() != "cache" && !strings.HasSuffix(p, "b4") }
	for _, tree := range []string{root, filepath.Join(runtime.GOROOT(), "src")} {
		if !isDir(tree) {
			continue
		}
		want := map[string]bool{}
		filepath.WalkDir(tree, func(p string, e fs.DirEntry, err error) error {
			if err != nil || p == tree {
				return nil
			}
			want[p] = true
			if e.IsDir() && !enter(p, e) {
				return filepath.SkipDir
			}
			return nil
		})
		var mu sync.Mutex
		got := map[string]bool{}
		walkTree(tree, func() bool { return false }, func(p string, e fs.DirEntry) bool {
			mu.Lock()
			if got[p] {
				t.Errorf("%s visited twice", p)
			}
			got[p] = true
			mu.Unlock()
			return e.IsDir() && enter(p, e)
		})
		if len(got) != len(want) || len(want) < 1000 {
			t.Errorf("%s: %d visited, want %d", tree, len(got), len(want))
		}
		for p := range want {
			if !got[p] {
				t.Errorf("%s not visited", p)
				break
			}
		}
	}
}

// The walk finds each kind of file, skips the same folders as before, does not enter
// links, and counts what it reads; a stopped walk ends early.
func TestDiscoveryWalk(t *testing.T) {
	root := t.TempDir()
	file := func(rel string, size int) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, make([]byte, size), 0o644)
	}
	file("certs/ca.pem", 10<<10)
	file("certs/small.pem", 1<<10) // too small for a bundle
	file("jre/lib/security/cacerts", 1)
	file("node/node.exe", 1)
	file("Git/cmd/git.exe", 1)
	os.MkdirAll(filepath.Join(root, "Git", "mingw64"), 0o755)
	file("tools/curl.exe", 1)
	file("App/v8_context_snapshot.bin", 1)
	file("App/resources/app.asar", 1)
	file("Browser/chrome.exe", 1)
	file("go/bin/go.exe", 1)
	file("go/VERSION", 1)
	file("a/b/c/python.exe", 1)             // depth 3: found
	file("a/b/c/d/python.exe", 1)           // depth 4: skipped
	file("Cache/x.pem", 10<<10)             // skipped by name
	file("proj/node_modules/n/node.exe", 1) // skipped: not a deep scan
	file("Skipped/python.exe", 1)           // skipped by the caller
	outside := t.TempDir()
	os.WriteFile(filepath.Join(outside, "python.exe"), nil, 0o644)
	os.Symlink(outside, filepath.Join(root, "linked")) // not entered

	d := newDiscovery()
	c := newChecker(nopReporter{}, nil)
	d.walk(c, walkSpec{root: root, maxDepth: 3, skip: []string{filepath.Join(root, "skipped")}, quiet: true}, false)
	j := func(rel string) string { return filepath.Join(root, filepath.FromSlash(rel)) }
	want := map[string][]string{
		"bundles":  {j("certs/ca.pem")},
		"java":     {j("jre")},
		"node":     {j("node/node.exe")},
		"python":   {j("a/b/c/python.exe")},
		"git":      {j("Git")},
		"curl":     {j("tools/curl.exe")},
		"electron": {j("App")},
		"go":       {j("go")},
		"browsers": {j("Browser/chrome.exe")},
	}
	got := map[string][]string{
		"bundles": sortedKeys(d.bundles), "java": sortedKeys(d.javaHomes), "node": sortedKeys(d.nodeExes),
		"python": sortedKeys(d.pythonExes), "git": sortedKeys(d.gitRoots), "curl": sortedKeys(d.curlExes),
		"electron": sortedKeys(d.electronDirs), "go": sortedKeys(d.goRoots), "browsers": sortedKeys(d.browsers),
	}
	for k, w := range want {
		sort.Strings(w)
		if strings.Join(got[k], "|") != strings.Join(w, "|") {
			t.Errorf("%s: %v, want %v", k, got[k], w)
		}
	}
	// folders: the root, certs, jre, lib, security, node, Git, cmd, mingw64, tools, App,
	// resources, Browser, go, bin, a, b, c, proj; files: those found or read in them
	if d.dirs != 19 || d.files != 12 {
		t.Errorf("%d folders, %d files, want 19 and 12", d.dirs, d.files)
	}

	// a link given as the folder to scan is not entered either
	d = newDiscovery()
	d.walk(c, walkSpec{root: filepath.Join(root, "linked"), quiet: true}, false)
	if d.dirs != 0 || len(d.pythonExes) != 0 {
		t.Errorf("link entered: %d folders, %v", d.dirs, d.pythonExes)
	}

	// stopped: nothing more is read
	d = newDiscovery()
	d.walk(newChecker(nopReporter{}, func() bool { return true }), walkSpec{root: root, quiet: true}, false)
	if d.dirs != 0 || d.files != 0 {
		t.Errorf("stopped walk: %d folders, %d files", d.dirs, d.files)
	}
}
