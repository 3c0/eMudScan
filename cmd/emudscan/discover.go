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
	"sync"
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
	ytdlpExes    map[string]bool // yt-dlp*.exe, identified later by their version information
	dartSDKs     map[string]bool // dart-sdk folders (bin\dart.exe and version), also inside Flutter SDKs
	flutterApps  map[string]bool // folders of apps built with Flutter (flutter_windows.dll)
	webview2Dirs map[string]bool // folders of a WebView2 shipped by software (msedgewebview2.exe)
	jetbrainsds  map[string]bool // the installed jetbrainsd.exe of each user (isJetbrainsdExe)
	ideDirs      map[string]bool // folders of a JetBrains IDE (product-info.json)

	dirs, files int
	lastLog     time.Time
	seenRoots   map[string]bool
	explicit    map[string]bool   // roots that are inside another root but skipped by it
	scope       func(string) bool // paths that may be read (selected drives)
}

// found is everything found, by kind.
func (d *discovery) found() []map[string]bool {
	return []map[string]bool{d.bundles, d.javaHomes, d.nodeExes, d.pythonExes, d.gitRoots, d.curlExes, d.electronDirs, d.goRoots, d.browsers, d.ytdlpExes, d.dartSDKs, d.flutterApps, d.webview2Dirs, d.jetbrainsds, d.ideDirs}
}

// prune forgets what was found outside the selected drives.
func (d *discovery) prune() {
	if d.scope == nil {
		return
	}
	for _, m := range d.found() {
		for p := range m {
			if !d.scope(p) {
				delete(m, p)
			}
		}
	}
}

// rewrite gives each path found its own spelling f(path): the same file found by two
// paths is then checked once.
func (d *discovery) rewrite(f func(string) string) {
	for _, m := range d.found() {
		for p := range m {
			if q := f(p); q != p {
				delete(m, p)
				m[q] = true
			}
		}
	}
}

func newDiscovery() *discovery {
	return &discovery{
		bundles: map[string]bool{}, javaHomes: map[string]bool{}, nodeExes: map[string]bool{},
		pythonExes: map[string]bool{}, gitRoots: map[string]bool{}, curlExes: map[string]bool{},
		electronDirs: map[string]bool{}, goRoots: map[string]bool{}, browsers: map[string]bool{}, ytdlpExes: map[string]bool{},
		dartSDKs: map[string]bool{}, flutterApps: map[string]bool{}, webview2Dirs: map[string]bool{},
		jetbrainsds: map[string]bool{}, ideDirs: map[string]bool{}, seenRoots: map[string]bool{}, explicit: map[string]bool{},
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
	quiet    bool     // no step of its own in the log (the caller logs one for several folders)
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
	if !spec.quiet {
		dirs0, files0 := d.dirs, d.files
		c.stepStart(Tf("log.walk", shownPath(spec.root)))
		defer func() { c.stepEnd(StOK, Tf("log.walkDone", d.dirs-dirs0, d.files-files0)) }()
	}
	// A link to a folder (a junction) is not entered, as inside the tree.
	if fi, err := os.Lstat(spec.root); err != nil || !fi.IsDir() || c.cancel() {
		return
	}
	d.lastLog = time.Now()
	base := strings.Count(filepath.Clean(spec.root), string(filepath.Separator))
	skip := map[string]bool{}
	for _, s := range spec.skip {
		skip[pathKey(s)] = true
	}
	var mu sync.Mutex // what is found, and the log
	counted := func(p string) {
		d.dirs++
		if time.Since(d.lastLog) > 250*time.Millisecond {
			d.lastLog = time.Now()
			c.activity(Tf("act.scan", shownPath(p)))
		}
	}
	counted(spec.root)
	walkTree(spec.root, c.cancel, func(p string, e fs.DirEntry) bool {
		name := lower(e.Name())
		if e.IsDir() {
			if skipDirNames[name] || (!deep && name == "node_modules") || skip[pathKey(p)] ||
				spec.maxDepth > 0 && strings.Count(p, string(filepath.Separator))-base > spec.maxDepth {
				return false
			}
			mu.Lock()
			counted(p)
			mu.Unlock()
			return true
		}
		if e.Type().IsRegular() {
			mu.Lock()
			d.files++
			d.classify(p, name, e)
			mu.Unlock()
		}
		return false
	})
}

// walkWorkers is the number of folders read at once. Reading a folder is mostly
// waiting, for Windows and, when the folder is not in memory yet, for the disk:
// reading several at once saves most of that waiting.
const walkWorkers = 8

// walkTree calls visit for each file and folder inside root, from several goroutines
// at once and in no particular order; a folder is entered when visit returns true. A
// folder that cannot be read entirely is skipped. stop ends the walk early.
func walkTree(root string, stop func() bool, visit func(p string, e fs.DirEntry) bool) {
	var (
		mu      sync.Mutex
		more    = sync.NewCond(&mu)
		todo    = []string{root} // folders to read
		pending = 1              // folders to read or being read
		wg      sync.WaitGroup
	)
	for range walkWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				mu.Lock()
				for len(todo) == 0 && pending > 0 {
					more.Wait()
				}
				if pending == 0 {
					mu.Unlock()
					return
				}
				dir := todo[len(todo)-1]
				todo = todo[:len(todo)-1]
				mu.Unlock()
				var sub []string
				if !stop() {
					for _, e := range readDir(dir) {
						p := filepath.Join(dir, e.Name())
						if visit(p, e) && e.IsDir() {
							sub = append(sub, p)
						}
					}
				}
				mu.Lock()
				todo = append(todo, sub...)
				pending += len(sub) - 1
				if pending == 0 || len(sub) > 0 {
					more.Broadcast()
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
}

// readDir lists a folder, unsorted; nothing if it cannot be read entirely.
func readDir(dir string) []fs.DirEntry {
	f, err := os.Open(dir)
	if err != nil {
		return nil
	}
	defer f.Close()
	entries, err := f.ReadDir(-1)
	if err != nil {
		return nil
	}
	return entries
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
	case strings.HasPrefix(name, "yt-dlp") && ext == ".exe":
		if localFile(e) {
			d.ytdlpExes[p] = true
		}
	case name == "dart.exe" && parentName == "bin":
		if root := filepath.Dir(parent); exists(filepath.Join(root, "version")) && localFile(e) {
			d.dartSDKs[root] = true
		}
	case name == "flutter.bat" && parentName == "bin":
		// the Dart SDK of a Flutter SDK, in bin\cache, a folder the scan skips
		if sdk := filepath.Join(parent, "cache", "dart-sdk"); exists(filepath.Join(sdk, "version")) {
			d.dartSDKs[sdk] = true
		}
	case name == "flutter_windows.dll":
		if localFile(e) && !flutterBuildFile(p) {
			d.flutterApps[parent] = true
		}
	case name == "product-info.json":
		if isDir(filepath.Join(parent, "bin")) {
			d.ideDirs[parent] = true
		}
	case name == "jetbrainsd.exe":
		if localFile(e) && isJetbrainsdExe(p) {
			d.jetbrainsds[p] = true
		}
	case name == "msedgewebview2.exe":
		if localFile(e) && !microsoftWebView2(p) {
			d.webview2Dirs[parent] = true
		}
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

// localFile: a file that is on the disk, not kept online only (reading it would
// download it).
func localFile(e fs.DirEntry) bool {
	fi, err := e.Info()
	return err == nil && !isCloudPlaceholder(fi)
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
