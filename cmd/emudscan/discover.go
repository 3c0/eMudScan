package main

// Discovery: one pass over the usual install locations collects everything
// the third-party checks need (certificate bundles, Java runtimes, Node.js,
// Python, Git, curl, Electron apps, Go toolchains).

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type discovery struct {
	bundles      map[string]bool // candidate PEM bundle files
	javaHomes    map[string]bool // folders with lib\security\cacerts
	nodeExes     map[string]bool
	pythonExes   map[string]bool
	gitRoots     map[string]bool
	curlExes     map[string]bool
	electronDirs map[string]bool
	goRoots      map[string]bool
	browsers     map[string]bool

	dirs, files int
	lastLog     time.Time
	seenRoots   map[string]bool
	explicit    map[string]bool   // roots that are inside another root but skipped by it
	scope       func(string) bool // paths that may be read (selected drives)
}

// prune forgets what was found outside the selected drives.
func (d *discovery) prune() {
	if d.scope == nil {
		return
	}
	for _, m := range []map[string]bool{d.bundles, d.javaHomes, d.nodeExes, d.pythonExes, d.gitRoots, d.curlExes, d.electronDirs, d.goRoots, d.browsers} {
		for p := range m {
			if !d.scope(p) {
				delete(m, p)
			}
		}
	}
}

func newDiscovery() *discovery {
	return &discovery{
		bundles: map[string]bool{}, javaHomes: map[string]bool{}, nodeExes: map[string]bool{},
		pythonExes: map[string]bool{}, gitRoots: map[string]bool{}, curlExes: map[string]bool{},
		electronDirs: map[string]bool{}, goRoots: map[string]bool{}, browsers: map[string]bool{}, seenRoots: map[string]bool{}, explicit: map[string]bool{},
	}
}

func getenv(k string) string { return os.Getenv(k) }

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

func lower(s string) string { return strings.ToLower(s) }

// pathKey normalises a path for de-duplication (Windows paths are case-insensitive).
func pathKey(p string) string { return lower(filepath.Clean(p)) }

// hasPathPrefix reports whether p is inside dir (case-insensitive).
func hasPathPrefix(p, dir string) bool {
	p, dir = pathKey(p), pathKey(dir)
	return p == dir || strings.HasPrefix(p, strings.TrimRight(dir, `\/`)+string(filepath.Separator))
}

// walkSpec describes one folder tree to scan.
type walkSpec struct {
	root     string
	maxDepth int
	skip     []string // absolute folders to skip (case-insensitive)
	explicit bool     // scan even if inside an already scanned tree (which skips it)
}

// Folder names that never contain anything relevant, or are huge.
var skipDirNames = map[string]bool{
	"$recycle.bin": true, "system volume information": true, ".git": true, ".svn": true, ".hg": true,
	"__pycache__": true, "winsxs": true, "servicing": true, "installer": true, "$windows.~bt": true,
	"$windows.~ws": true, "windowsapps": true, "cache": true, "caches": true, "code cache": true,
	"gpucache": true, "crashpad": true, ".npm": true, ".nuget": true, "user data": true,
}

func (d *discovery) walk(c *checker, spec walkSpec, deep bool) {
	if spec.explicit {
		d.explicit[pathKey(spec.root)] = true
	}
	if spec.root == "" || !isDir(spec.root) || d.seenRoots[pathKey(spec.root)] || d.insideSeen(spec.root) {
		return
	}
	d.seenRoots[pathKey(spec.root)] = true
	dirs0, files0 := d.dirs, d.files
	c.stepStart(Tf("log.walk", spec.root))
	defer func() { c.stepEnd(StOK, Tf("log.walkDone", d.dirs-dirs0, d.files-files0)) }()
	d.lastLog = time.Now()
	base := strings.Count(filepath.Clean(spec.root), string(filepath.Separator))
	filepath.WalkDir(spec.root, func(p string, e fs.DirEntry, err error) error {
		if c.cancel() {
			return filepath.SkipAll
		}
		if err != nil {
			if e != nil && e.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		name := lower(e.Name())
		if e.IsDir() {
			if p != spec.root {
				if skipDirNames[name] || (!deep && name == "node_modules") {
					return filepath.SkipDir
				}
				for _, s := range spec.skip {
					if pathKey(p) == pathKey(s) {
						return filepath.SkipDir
					}
				}
				if spec.maxDepth > 0 && strings.Count(p, string(filepath.Separator))-base > spec.maxDepth {
					return filepath.SkipDir
				}
			}
			d.dirs++
			if time.Since(d.lastLog) > 250*time.Millisecond {
				d.lastLog = time.Now()
				c.activity(Tf("act.scan", p))
			}
			return nil
		}
		if !e.Type().IsRegular() {
			return nil
		}
		d.files++
		d.classify(p, name, e)
		return nil
	})
}

// insideSeen reports whether root lies inside a tree already scanned.
func (d *discovery) insideSeen(root string) bool {
	for r := range d.seenRoots {
		if hasPathPrefix(root, r) && !d.skippedInside(root) {
			return true
		}
	}
	return false
}

// skippedInside: roots added explicitly because their parent scan skips them.
func (d *discovery) skippedInside(root string) bool { return d.explicit[pathKey(root)] }

func (d *discovery) classify(p, name string, e fs.DirEntry) {
	ext := filepath.Ext(name)
	parent := filepath.Dir(p)
	parentName := lower(filepath.Base(parent))
	switch {
	case ext == ".pem" || ext == ".crt" || ext == ".cer" || ext == ".bundle":
		fi, err := e.Info()
		if err != nil || fi.Size() < 8<<10 || fi.Size() > 20<<20 || isCloudPlaceholder(fi) {
			return
		}
		d.bundles[p] = true
	case name == "cacerts" && parentName == "security":
		d.javaHomes[filepath.Dir(filepath.Dir(parent))] = true
	case name == "node.exe":
		d.nodeExes[p] = true
	case name == "python.exe":
		d.pythonExes[p] = true
	case name == "git.exe" && parentName == "cmd":
		root := filepath.Dir(parent)
		if isDir(filepath.Join(root, "mingw64")) || isDir(filepath.Join(root, "usr")) || isDir(filepath.Join(root, "clangarm64")) {
			d.gitRoots[root] = true
		}
	case name == "curl.exe":
		d.curlExes[p] = true
	case name == "v8_context_snapshot.bin" || name == "snapshot_blob.bin":
		if exists(filepath.Join(parent, "resources", "app.asar")) || exists(filepath.Join(parent, "resources", "app", "package.json")) {
			d.electronDirs[parent] = true
		}
	case isBrowserExe[name]:
		d.browsers[p] = true
	case name == "go.exe" && parentName == "bin":
		if exists(filepath.Join(filepath.Dir(parent), "VERSION")) {
			d.goRoots[filepath.Dir(parent)] = true
		}
	}
}

var isBrowserExe = func() map[string]bool {
	m := map[string]bool{}
	for _, n := range browserExeNames {
		m[n] = true
	}
	return m
}()

// pathDirs returns the folders listed in PATH.
func pathDirs() []string {
	var out []string
	seen := map[string]bool{}
	for _, d := range filepath.SplitList(getenv("PATH")) {
		d = strings.Trim(strings.TrimSpace(d), `"`)
		if d == "" || seen[pathKey(d)] || !isDir(d) {
			continue
		}
		seen[pathKey(d)] = true
		out = append(out, d)
	}
	return out
}

// addFromPath registers programs found in PATH folders.
func (d *discovery) addFromPath() {
	for _, dir := range pathDirs() {
		if d.scope != nil && !d.scope(dir) {
			continue
		}
		for _, n := range []string{"node.exe", "python.exe", "curl.exe", "java.exe", "go.exe", "git.exe"} {
			p := filepath.Join(dir, n)
			if !exists(p) {
				continue
			}
			switch n {
			case "node.exe":
				d.nodeExes[p] = true
			case "python.exe":
				d.pythonExes[p] = true
			case "curl.exe":
				d.curlExes[p] = true
			case "java.exe":
				home := filepath.Dir(dir)
				if exists(filepath.Join(home, "lib", "security", "cacerts")) {
					d.javaHomes[home] = true
				}
			case "go.exe":
				if exists(filepath.Join(filepath.Dir(dir), "VERSION")) {
					d.goRoots[filepath.Dir(dir)] = true
				}
			case "git.exe":
				if lower(filepath.Base(dir)) == "cmd" {
					d.gitRoots[filepath.Dir(dir)] = true
				}
			}
		}
	}
	if jh := getenv("JAVA_HOME"); jh != "" && exists(filepath.Join(jh, "lib", "security", "cacerts")) {
		d.javaHomes[jh] = true
	}
	if gr := getenv("GOROOT"); gr != "" && exists(filepath.Join(gr, "VERSION")) {
		d.goRoots[gr] = true
	}
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// dedupePaths removes case-insensitive duplicates, keeping the first spelling.
func dedupePaths(m map[string]bool) []string {
	seen := map[string]bool{}
	var out []string
	for _, k := range sortedKeys(m) {
		if !seen[pathKey(k)] {
			seen[pathKey(k)] = true
			out = append(out, k)
		}
	}
	return out
}
