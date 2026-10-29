package main

// Third-party programs: Git, curl, Python, Node.js, Java, Electron apps, Go,
// programs with a built-in certificate list, and certificate bundle files.

import (
	"bufio"
	"bytes"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

type appsContext struct {
	c            *checker
	d            *discovery
	bc           *bundleCache
	es           *embedScanner
	drives       []string        // drives to scan entirely; nothing is read on the other ones
	allDrives    bool            // no drive selection (command-line test runner)
	handled      map[string]bool // bundles reported by a program-specific check
	windowsBlock bool            // Windows' chain engine blocks all 16 certificates
	pyStoreRisk  int             // test chains accepted by a copy of the Windows root list
	extraRoots   []string        // test hook: additional folders to scan
	winDir       string
	owners       []ownerDir
	scoopApps    map[string]scoopManifest // installed Scoop apps
	choco        []chocoPkg               // packages installed with Chocolatey
	chocoRoot    string
	netPrograms  []netProgram     // installed programs that work over the internet
	entries      []uninstallEntry // Apps & features
	scanMu       sync.Mutex
	scanned      map[scanKey]scanEntry // files already scanned (for "Scan again" and files checked twice)
	idMu         sync.Mutex
	idOwners     map[string]ownerDir // owners of identifier-named folders (identifier.go)
}

type scanKey struct {
	path      string
	size, mod int64
}

type scanEntry struct {
	what scanWhat
	f    embedFindings
	err  error
}

// scan scans a program file for built-in certificate lists, once: a file that has not
// changed since it was scanned (same size and date) is not read again.
func (tp *appsContext) scan(path string, what scanWhat) (embedFindings, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return embedFindings{}, err
	}
	k := scanKey{pathKey(path), fi.Size(), fi.ModTime().UnixNano()}
	tp.scanMu.Lock()
	if tp.scanned == nil {
		tp.scanned = map[scanKey]scanEntry{}
	}
	e, ok := tp.scanned[k]
	tp.scanMu.Unlock()
	if ok && e.what&what == what {
		return e.f, e.err
	}
	f, err := tp.scanFile(path, what)
	tp.scanMu.Lock()
	tp.scanned[k] = scanEntry{what, f, err}
	tp.scanMu.Unlock()
	return f, err
}

// scanFile reads one file. A failure of the readers themselves (a bug, on a file
// they did not expect) becomes an error of that file, noted in the log, instead of
// ending eMudScan: the files are read on several goroutines, and a panic on any of
// them would close the program without a word.
func (tp *appsContext) scanFile(path string, what scanWhat) (f embedFindings, err error) {
	defer func() {
		if r := recover(); r != nil {
			f, err = embedFindings{}, fmt.Errorf("internal error: %v", r)
			tp.c.line(StError, Tf("log.readerFailed", path, r))
		}
	}()
	return tp.es.scanFile(path, what)
}

// scanAll scans files on several processors at once. The results are in the order
// of paths (a result is empty when the check was stopped before its file).
func (tp *appsContext) scanAll(paths []string, what scanWhat) []scanEntry {
	out := make([]scanEntry, len(paths))
	var next atomic.Int64
	var wg sync.WaitGroup
	for w := 0; w < min(runtime.NumCPU(), 6, len(paths)); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				i := int(next.Add(1) - 1)
				if i >= len(paths) || tp.c.cancel() {
					return
				}
				tp.c.activity(Tf("act.read", paths[i]))
				f, err := tp.scan(paths[i], what)
				out[i] = scanEntry{what, f, err}
			}
		}()
	}
	wg.Wait()
	tp.c.activity("")
	return out
}

func runThirdPartyChecks(c *checker, drives []string, windowsBlock bool, pyStoreRisk int, extraRoots []string) {
	tp := &appsContext{
		c: c, d: newDiscovery(), bc: newBundleCache(), es: defaultEmbedScanner, drives: drives, allDrives: drives == nil,
		handled: map[string]bool{}, windowsBlock: windowsBlock, pyStoreRisk: pyStoreRisk,
		extraRoots: extraRoots, winDir: windowsDir(), scoopApps: map[string]scoopManifest{},
	}
	tp.d.scope = tp.inScope
	tp.loadChocolatey()
	c.afterFinish = tp.packageAdvice // kept for "Scan again"
	c.refresh = tp.refresh
	c.dated = tp.dated
	c.section(T("log.thirdPartyStart"))
	c.stepStart(T("log.owners"))
	tp.entries = uninstallEntries()
	tp.loadOwners(tp.entries)
	tp.loadInternetPrograms(tp.entries)
	c.stepEnd(StOK, Tf("log.ownersDone", len(tp.owners)))
	tp.discover()
	for _, f := range []func(){tp.checkBrowsers, tp.checkGit, tp.checkCurl, tp.checkPython, tp.checkNode, tp.checkJava,
		tp.checkElectron, tp.checkGo, tp.checkCertSettings, tp.checkUninstallApps, tp.checkEmbeddedTools, tp.checkBundles,
		tp.checkInternetFolders, tp.checkOthers} {
		if c.cancel() {
			return
		}
		f()
	}
}

// refresh reads again the installed software (Apps & features, Scoop, Chocolatey)
// before "Scan again": a program may have been updated since the scan.
func (tp *appsContext) refresh() {
	tp.entries = uninstallEntries()
	tp.owners, tp.netPrograms = nil, nil
	tp.loadOwners(tp.entries)
	tp.idMu.Lock()
	tp.idOwners = nil
	tp.idMu.Unlock()
	tp.loadInternetPrograms(tp.entries)
	tp.scoopApps = map[string]scoopManifest{}
	tp.choco = nil
	tp.loadChocolatey()
}

// currentEntry is the Apps & features entry e as it is now (found by its folder,
// else by its name), or e when it is gone.
func (tp *appsContext) currentEntry(e uninstallEntry) uninstallEntry {
	for _, x := range tp.entries {
		if e.location != "" && pathKey(x.location) == pathKey(e.location) {
			return x
		}
	}
	for _, x := range tp.entries {
		if lower(x.name) == lower(e.name) {
			return x
		}
	}
	return e
}

// ---------------------------------------------------------------------------
// Discovery

// walkSpecs scans each selected drive entirely, except Windows and caches.
func (tp *appsContext) walkSpecs() []walkSpec {
	var specs []walkSpec
	lad, ad, pd := getenv("LOCALAPPDATA"), getenv("APPDATA"), getenv("ProgramData")
	skip := []string{tp.winDir}
	if pd != "" {
		skip = append(skip, filepath.Join(pd, "Microsoft"), filepath.Join(pd, "Package Cache"), filepath.Join(pd, "Packages"))
	}
	if lad != "" {
		skip = append(skip, filepath.Join(lad, "Temp"), filepath.Join(lad, "Packages"), filepath.Join(lad, "Microsoft"),
			filepath.Join(lad, "Google", "Chrome", "User Data"), filepath.Join(lad, "Mozilla"),
			filepath.Join(lad, "BraveSoftware", "Brave-Browser", "User Data"), filepath.Join(lad, "CrashDumps"),
			filepath.Join(lad, "D3DSCache"), filepath.Join(lad, "pip", "cache"), filepath.Join(lad, "npm-cache"))
	}
	if ad != "" {
		skip = append(skip, filepath.Join(ad, "Microsoft"), filepath.Join(ad, "Mozilla"))
	}
	for _, drv := range tp.drives {
		specs = append(specs, walkSpec{root: drv, maxDepth: 40, skip: skip})
	}
	if lad != "" && tp.driveSelected(lad) {
		// folders skipped above that can hold programs
		specs = append(specs, walkSpec{root: filepath.Join(lad, "Microsoft", "WinGet", "Packages"), maxDepth: 8, explicit: true})
		if m, _ := filepath.Glob(filepath.Join(lad, "Packages", "PythonSoftwareFoundation.Python.*", "LocalCache", "local-packages")); m != nil {
			for _, p := range m {
				specs = append(specs, walkSpec{root: p, maxDepth: 8, explicit: true})
			}
		}
	}
	for _, r := range tp.extraRoots {
		specs = append(specs, walkSpec{root: r, explicit: true})
	}
	return specs
}

// inScope reports whether a path may be read: it must be on a selected drive (or in
// a folder given to the command-line test runner). Programs found through PATH, the
// registry or Windows' list of browsers are skipped when their drive is not selected.
func (tp *appsContext) inScope(p string) bool {
	if tp.allDrives || p == "" {
		return true
	}
	for _, r := range tp.extraRoots {
		if hasPathPrefix(p, r) {
			return true
		}
	}
	return tp.driveSelected(p)
}

func (tp *appsContext) driveSelected(p string) bool {
	for _, d := range tp.drives {
		if hasPathPrefix(p, d) {
			return true
		}
	}
	return false
}

func (tp *appsContext) discover() {
	for _, s := range tp.walkSpecs() {
		if tp.c.cancel() {
			return
		}
		tp.d.walk(tp.c, s, false)
	}
	tp.c.activity("")
	tp.c.stepStart(T("log.pathRegistry"))
	tp.d.addFromPath()
	tp.addFromRegistry()
	tp.d.prune()
	tp.c.stepEnd(StOK, T("log.done.short"))
}

func (tp *appsContext) addFromRegistry() {
	for _, root := range []regRoot{hklm, hkcu} {
		for _, view := range []uint32{view64, view32} {
			if p, ok := regString(root, `SOFTWARE\GitForWindows`, "InstallPath", view); ok && isDir(p) {
				tp.d.gitRoots[p] = true
			}
			if p, ok := regString(root, `SOFTWARE\Node.js`, "InstallPath", view); ok && exists(filepath.Join(p, "node.exe")) {
				tp.d.nodeExes[filepath.Join(p, "node.exe")] = true
			}
			for _, company := range regSubkeys(root, `SOFTWARE\Python`, view) {
				if company == "PyLauncher" {
					continue
				}
				for _, tag := range regSubkeys(root, `SOFTWARE\Python\`+company, view) {
					k := `SOFTWARE\Python\` + company + `\` + tag + `\InstallPath`
					exe, _ := regString(root, k, "ExecutablePath", view)
					if exe == "" {
						if dir, ok := regString(root, k, "", view); ok {
							exe = filepath.Join(dir, "python.exe")
						}
					}
					if exe != "" && exists(exe) {
						tp.d.pythonExes[exe] = true
					}
				}
			}
			for _, vk := range []string{`SOFTWARE\JavaSoft`, `SOFTWARE\Eclipse Adoptium`, `SOFTWARE\Eclipse Foundation`,
				`SOFTWARE\Azul Systems`, `SOFTWARE\Microsoft\JDK`, `SOFTWARE\Amazon Corretto`, `SOFTWARE\BellSoft`} {
				tp.findJavaInRegistry(root, vk, view, 0)
			}
		}
	}
}

func (tp *appsContext) findJavaInRegistry(root regRoot, key string, view uint32, depth int) {
	if depth > 4 {
		return
	}
	for _, name := range []string{"JavaHome", "Path", "InstallationPath"} {
		if p, ok := regString(root, key, name, view); ok && p != "" {
			if exists(filepath.Join(p, "lib", "security", "cacerts")) {
				tp.d.javaHomes[p] = true
			} else if exists(filepath.Join(p, "jre", "lib", "security", "cacerts")) {
				tp.d.javaHomes[filepath.Join(p, "jre")] = true
			}
		}
	}
	for _, sub := range regSubkeys(root, key, view) {
		tp.findJavaInRegistry(root, key+`\`+sub, view, depth+1)
	}
}

// ---------------------------------------------------------------------------
// Owners: which program a file belongs to

type uninstallEntry struct {
	name, version, location, url, icon string
	dirs                               []string
}

type ownerDir struct{ dir, name, url, version, exe string }

// ownerInfo: Exe is the main program of the owner when Apps & features gives it.
type ownerInfo struct{ Name, URL, Loc, Version, Exe string }

// exePathFrom extracts the program path from a DisplayIcon or UninstallString value.
func exePathFrom(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, `"`) {
		if i := strings.Index(s[1:], `"`); i >= 0 {
			return s[1 : 1+i]
		}
		return ""
	}
	if i := strings.Index(lower(s), ".exe"); i >= 0 {
		return s[:i+4]
	}
	if i := strings.LastIndex(s, ","); i > 0 {
		s = s[:i]
	}
	return s
}

func uninstallEntries() []uninstallEntry {
	var out []uninstallEntry
	seen := map[string]bool{}
	for _, root := range []regRoot{hklm, hkcu} {
		for _, view := range []uint32{view64, view32} {
			base := `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`
			for _, k := range regSubkeys(root, base, view) {
				key := base + `\` + k
				n, _ := regString(root, key, "DisplayName", view)
				if n == "" {
					continue
				}
				v, _ := regString(root, key, "DisplayVersion", view)
				e := uninstallEntry{name: n, version: v}
				e.location, _ = regString(root, key, "InstallLocation", view)
				for _, u := range []string{"URLInfoAbout", "URLUpdateInfo", "HelpLink"} {
					if s, ok := regString(root, key, u, view); ok && strings.HasPrefix(lower(s), "http") {
						e.url = s
						break
					}
				}
				if e.location != "" {
					e.dirs = append(e.dirs, e.location)
				}
				for _, vn := range []string{"DisplayIcon", "UninstallString", "QuietUninstallString"} {
					if s, ok := regString(root, key, vn, view); ok {
						if p := exePathFrom(s); p != "" {
							e.dirs = append(e.dirs, filepath.Dir(p))
							if vn == "DisplayIcon" {
								e.icon = p
							}
						}
					}
				}
				if seen[n+"|"+v+"|"+e.location] {
					continue
				}
				seen[n+"|"+v+"|"+e.location] = true
				out = append(out, e)
			}
		}
	}
	return out
}

// genericDirs are never taken as a program's own folder.
func (tp *appsContext) genericDirs() map[string]bool {
	m := map[string]bool{}
	for _, v := range []string{"ProgramFiles", "ProgramW6432", "ProgramFiles(x86)", "ProgramData", "LOCALAPPDATA", "APPDATA", "USERPROFILE", "TEMP", "CommonProgramFiles", "CommonProgramFiles(x86)"} {
		if p := getenv(v); p != "" {
			m[pathKey(p)] = true
		}
	}
	if lad := getenv("LOCALAPPDATA"); lad != "" {
		m[pathKey(filepath.Join(lad, "Programs"))] = true
	}
	return m
}

func (tp *appsContext) acceptableOwnerDir(dir string, generic map[string]bool) bool {
	if dir == "" || !filepath.IsAbs(dir) || generic[pathKey(dir)] || len(filepath.Clean(dir)) <= 3 {
		return false
	}
	ld := lower(dir)
	if tp.winDir != "" && hasPathPrefix(dir, tp.winDir) {
		return false
	}
	for _, bad := range []string{`\package cache\`, `\installer\`, `\temp\`, `\{`, `\user data\`, `\web applications\`} {
		if strings.Contains(ld+`\`, bad) {
			return false
		}
	}
	return true
}

func (tp *appsContext) loadOwners(entries []uninstallEntry) {
	generic := tp.genericDirs()
	seen := map[string]bool{}
	for _, e := range entries {
		for _, d := range e.dirs {
			if !tp.acceptableOwnerDir(d, generic) || seen[pathKey(d)] {
				continue
			}
			seen[pathKey(d)] = true
			exe := ""
			if e.icon != "" && strings.EqualFold(filepath.Ext(e.icon), ".exe") && hasPathPrefix(e.icon, d) {
				exe = e.icon
			}
			tp.owners = append(tp.owners, ownerDir{filepath.Clean(d), cleanProgramName(e.name, e.version), e.url, e.version, exe})
		}
	}
	sort.Slice(tp.owners, func(i, j int) bool { return len(tp.owners[i].dir) > len(tp.owners[j].dir) })
}

var (
	scoopRe  = regexp.MustCompile(`(?i)^(.*?[\\/]scoop)[\\/](?:apps|persist)[\\/]([^\\/]+)`)
	wingetRe = regexp.MustCompile(`(?i)[\\/]Microsoft[\\/]WinGet[\\/]Packages[\\/]([^\\/]+)`)
)

var genericFolderNames = map[string]bool{"microsoft": true, "common files": true, "programs": true, "temp": true,
	"packages": true, "windowsapps": true, "scoop": true, "microsoft shared": true}

// ownerOf finds the program a path belongs to.
func (tp *appsContext) ownerOf(p string) ownerInfo {
	if p == "" {
		return ownerInfo{}
	}
	if m := scoopRe.FindStringSubmatch(p); m != nil {
		sm := tp.scoopApp(m[1], m[2])
		ver := sm.Version
		if ver == "" { // apps\<app>\<version>\...: the folder of a version
			rest := strings.FieldsFunc(p[len(m[0]):], func(c rune) bool { return c == '\\' || c == '/' })
			if len(rest) > 0 && rest[0][0] >= '0' && rest[0][0] <= '9' {
				ver = rest[0]
			}
		}
		return ownerInfo{Name: m[2], URL: sm.Homepage, Loc: filepath.Join(m[1], "apps", m[2], "current"), Version: ver}
	}
	if m := wingetRe.FindStringSubmatchIndex(p); m != nil {
		id := p[m[2]:m[3]]
		if i := strings.Index(id, "_Microsoft.Winget"); i > 0 {
			id = id[:i]
		}
		return ownerInfo{Name: id, Loc: p[:m[1]]}
	}
	if o, ok := tp.chocoOwner(p); ok {
		return o
	}
	for _, o := range tp.owners {
		if hasPathPrefix(p, o.dir) {
			return ownerInfo{Name: o.name, URL: o.url, Loc: o.dir, Version: o.version, Exe: o.exe}
		}
	}
	// fall back on the program's top-level folder in the usual install locations
	lad := getenv("LOCALAPPDATA")
	roots := []string{getenv("ProgramFiles"), getenv("ProgramFiles(x86)"), getenv("ProgramData"), getenv("APPDATA")}
	if lad != "" {
		roots = append(roots, filepath.Join(lad, "Programs"), lad)
	}
	for _, r := range roots {
		if r == "" || !hasPathPrefix(p, r) || pathKey(p) == pathKey(r) {
			continue
		}
		rel := strings.TrimLeft(p[len(filepath.Clean(r)):], `\/`)
		first := strings.FieldsFunc(rel, func(c rune) bool { return c == '\\' || c == '/' })[0]
		if genericFolderNames[lower(first)] {
			continue
		}
		if pathKey(r) == pathKey(lad) || pathKey(r) == pathKey(getenv("APPDATA")) {
			if o, ok := tp.identifierOwnerOf(r, first); ok { // %LOCALAPPDATA%\io.esphome.builder
				return o
			}
		}
		return ownerInfo{Name: first, Loc: filepath.Join(r, first)}
	}
	if up := getenv("USERPROFILE"); up != "" && hasPathPrefix(p, up) && pathKey(p) != pathKey(up) {
		rel := strings.TrimLeft(p[len(filepath.Clean(up)):], `\/`)
		first := strings.FieldsFunc(rel, func(c rune) bool { return c == '\\' || c == '/' })[0]
		if strings.HasPrefix(first, ".") && len(first) > 1 {
			// data folder of an installed program: C:\Users\me\.lmstudio -> LM Studio
			for _, o := range tp.owners {
				if alnum(o.name) == alnum(first) {
					return ownerInfo{Name: o.name, URL: o.url, Loc: filepath.Join(up, first), Version: o.version, Exe: o.exe}
				}
			}
			return ownerInfo{Name: first, Loc: filepath.Join(up, first)}
		}
	}
	return ownerInfo{}
}

var (
	archSuffixRe = regexp.MustCompile(`(?i)\s*\((x64|x86|64[ -]?bit|32[ -]?bit|arm64|amd64)\b[^)]*\)\s*$`)
	verSuffixRe  = regexp.MustCompile(`(?i)\s+v?\d+(\.\d+)+([+\-._ ]?[0-9a-z]+)*\s*$`)
)

// cleanProgramName removes the version and architecture that installers often add to
// their display name ("Signal 8.32.0" -> "Signal", "Mozilla Firefox (x64 fr)" -> "Mozilla Firefox").
func cleanProgramName(name, version string) string {
	n := strings.TrimSpace(name)
	for i := 0; i < 2; i++ {
		if version != "" && strings.HasSuffix(n, " "+version) {
			n = strings.TrimSpace(strings.TrimSuffix(n, " "+version))
		}
		n = archSuffixRe.ReplaceAllString(n, "")
		if c := verSuffixRe.ReplaceAllString(n, ""); c != "" {
			n = c
		}
	}
	if n == "" {
		return name
	}
	return n
}

// Folder names that say nothing about the program (structure of an installation).
var structuralDirs = map[string]bool{"bin": true, "sbin": true, "lib": true, "lib64": true, "libs": true, "dll": true,
	"dlls": true, "site-packages": true, "dist-packages": true, "certifi": true, "certs": true, "cert": true, "ssl": true,
	"etc": true, "share": true, "resources": true, "res": true, "app": true, "scripts": true, ".venv": true, "venv": true,
	"env": true, ".env": true, "_vendor": true, "vendor": true, "pip": true, "node_modules": true, "runtime": true,
	"jre": true, "jdk": true, "current": true, "data": true, "x64": true, "x86": true, "amd64": true, "arm64": true,
	"win64": true, "win32": true, "mingw64": true, "mingw32": true, "usr": true, "include": true, "tools": true,
	"plugins": true, "modules": true, "dist": true, "build": true, "release": true, "security": true, "conf": true,
	"config": true, "internal": true, "_internal": true, ".internal": true, "extensions": true, "backends": true,
	"packages": true, "browser": true, "program": true, "core": true, "python": true, "python3": true, "cacert": true,
	"library": true, "ca-certificates": true, "ca-bundle": true, "pki": true, "tls": true}

// Folder names that hold many programs: the program is the folder below them.
var containerDirs = map[string]bool{"portable": true, "portableapps": true, "apps": true, "programs": true,
	"program files": true, "program files (x86)": true, "install": true, "installs": true, "installers": true,
	"downloads": true, "desktop": true, "documents": true, "temp": true, "tmp": true, "dev": true, "src": true,
	"projects": true, "code": true, "work": true, "users": true, "appdata": true, "local": true, "roaming": true,
	"programdata": true, "software": true, "utils": true, "utilities": true}

var (
	versionedDirRe = regexp.MustCompile(`(?i)^python\d+\.\d+$|^lib(python)?\d|^v?\d+(\.\d+)+`)
	dirVersionRe   = regexp.MustCompile(`(?i)[-_ ]v?\d+(\.\d+)+.*$`)
)

// guessProgram names the program a file probably belongs to, from its folders, when
// nothing else tells (portable programs, projects): U:\Portable\chirp-next\certifi\cacert.pem
// gives "chirp-next". It is only shown; the advice does not depend on it.
func guessProgram(p string) string {
	name, _ := guessProgramVersion(p)
	return name
}

// guessProgramVersion also returns the version found in the folder name
// (Impactor_0.9.51 -> "Impactor", "0.9.51").
func guessProgramVersion(p string) (string, string) {
	name, ver, _ := guessProgramDir(p)
	return name, ver
}

// guessProgramDir also returns the folder the name comes from.
func guessProgramDir(p string) (name, version, folder string) {
	if p == "" || !filepath.IsAbs(p) {
		return "", "", ""
	}
	up := getenv("USERPROFILE")
	dir := filepath.Dir(p)
	if isDir(p) {
		dir = p
	}
	for i := 0; i < 12; i++ {
		parent := filepath.Dir(dir)
		if parent == dir || (up != "" && pathKey(dir) == pathKey(up)) {
			return "", "", ""
		}
		name := filepath.Base(dir)
		ln := lower(name)
		switch {
		case containerDirs[ln]:
			return "", "", ""
		case structuralDirs[ln] || versionedDirRe.MatchString(ln) || exists(filepath.Join(dir, "pyvenv.cfg")):
		default:
			if loc := dirVersionRe.FindStringIndex(name); loc != nil && loc[0] > 0 {
				return name[:loc[0]], strings.TrimLeft(name[loc[0]:], "-_ vV"), dir
			}
			return name, "", dir
		}
		dir = parent
	}
	return "", "", ""
}

// alnum keeps the lower-case letters and digits of s.
func alnum(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// bundledIn reports whether an owner is another program (the component is a copy inside it).
func bundledIn(owner string, keywords ...string) bool {
	if owner == "" {
		return false
	}
	lo := lower(owner)
	for _, k := range keywords {
		if strings.Contains(lo, k) {
			return false
		}
	}
	return true
}

var (
	kwGit    = []string{"git"}
	kwPython = []string{"python", "uv", "conda", "anaconda", "miniforge", "pyenv", "pixi"}
	kwNode   = []string{"node", "nvm", "volta", "fnm"}
	kwJava   = []string{"java", "jdk", "jre", "temurin", "zulu", "corretto", "openjdk", "liberica", "semeru", "adoptium"}
	kwGo     = []string{"go programming", "golang", "go ("}
)

// withOwner fills the owner fields of r for path p.
func (tp *appsContext) withOwner(r *result, p string) ownerInfo {
	o := tp.ownerOf(p)
	r.Owner, r.OwnerURL, r.OwnerVer = o.Name, o.URL, o.Version
	return o
}

// ---------------------------------------------------------------------------
// Last update of a software: the publisher's date of its main program

// dated is the last update of the software of a result (checker.dated).
func (tp *appsContext) dated(kind string, r result) lastUpdate {
	if kind == "info" {
		return lastUpdate{}
	}
	exe := tp.mainProgram(r)
	if exe == "" || !tp.inScope(exe) {
		return lastUpdate{Unknown: true}
	}
	// Electron applications carry the build date of Electron, not their own
	noBuild := kind == "electron" || r.Component == "Electron" || isElectronDir(filepath.Dir(exe))
	t, src := publisherDate(exe, noBuild)
	if src != dateSigned && geckoExes[lower(filepath.Base(exe))] {
		// Firefox-based browsers write their build date in application.ini (Tor
		// Browser's files are rewritten by its updater and carry no other date)
		if bt, ini, ok := geckoBuildDate(filepath.Dir(exe)); ok {
			return lastUpdate{Time: bt, Src: dateBuilt, File: ini}
		}
	}
	if src == dateNone {
		return lastUpdate{File: exe, Unknown: true}
	}
	return lastUpdate{Time: t, Src: src, File: exe}
}

// mainProgram is the file whose date is the date of the software of a result:
//   - for software from Apps & features, Scoop…: the program Apps & features names,
//     else the program checked when it is named like the software, else the program
//     named like the software in its folder, else (for the software's own program)
//     the program checked, else the largest program in its folder;
//   - else the program checked, else the main program of the folder the software
//     was named after, else the file checked (a certificate bundle).
func (tp *appsContext) mainProgram(r result) string {
	exeOK := r.Exe != "" && exists(r.Exe)
	if r.Owner != "" {
		if o := tp.ownerOf(firstNonEmpty(r.Exe, r.File, r.Folder)); o.Name != "" {
			if o.Exe != "" && exists(o.Exe) {
				return o.Exe
			}
			if exeOK && namedLike(r.Exe, o.Name) {
				return r.Exe
			}
			guess, named := mainExeIn(o.Loc, o.Name)
			switch {
			case named:
				return guess
			case exeOK && !r.Bundled:
				return r.Exe
			case guess != "":
				return guess
			}
		}
	}
	if exeOK {
		return r.Exe
	}
	if r.File != "" {
		if name, _, dir := guessProgramDir(r.File); dir != "" {
			if exe, _ := mainExeIn(dir, name); exe != "" {
				return exe
			}
		}
		if !isDir(r.File) && exists(r.File) {
			return r.File
		}
	}
	return ""
}

// namedLike: the name of a program file matches the name of a software
// (git.exe and Git, node.exe and nodejs-lts, tesseract.exe and tesseract).
func namedLike(exe, name string) bool {
	b := pathBase(exe)
	a, w := alnum(strings.TrimSuffix(b, filepath.Ext(b))), alnum(name)
	return a != "" && w != "" && (a == w || len(a) >= 3 && (strings.Contains(w, a) || strings.Contains(a, w)))
}

func isElectronDir(dir string) bool {
	return exists(filepath.Join(dir, "resources", "app.asar")) || exists(filepath.Join(dir, "v8_context_snapshot.bin"))
}

// helperExes are programs that come with a software but are not the software.
var helperExeRe = regexp.MustCompile(`(?i)^(unins|uninst|update|updater|setup|install|crash|elevat|notification_helper|squirrel|createdump|vc_?redist|maintenancetool|helper)`)

// mainExeIn guesses the main program in the folder of a software: the program named
// like the software (named is then true), else the largest one (in the folder, else
// in its bin folder). Squirrel installs (Discord, Teams…) keep each version in an
// app-x.y.z folder.
func mainExeIn(dir, name string) (exe string, named bool) {
	if dir == "" || !isDir(dir) {
		return "", false
	}
	if apps, _ := filepath.Glob(filepath.Join(dir, "app-*")); len(apps) > 0 {
		if n := newestSquirrel(apps); len(n) > 0 && squirrelDirRe.MatchString(filepath.Base(n[len(n)-1])) {
			dir = n[len(n)-1]
		}
	}
	best, bestScore, bestSize := "", 0, int64(-1)
	for i, d := range []string{dir, filepath.Join(dir, "bin")} {
		exes, _ := filepath.Glob(filepath.Join(d, "*.exe"))
		for _, e := range exes {
			if helperExeRe.MatchString(filepath.Base(e)) {
				continue
			}
			fi, err := os.Stat(e)
			if err != nil {
				continue
			}
			score := 2 - i // the folder itself before its bin folder
			if namedLike(e, name) {
				score += 10
				if alnum(strings.TrimSuffix(filepath.Base(e), filepath.Ext(e))) == alnum(name) {
					score += 10
				}
			}
			if score > bestScore || score == bestScore && fi.Size() > bestSize {
				best, bestScore, bestSize = e, score, fi.Size()
			}
		}
	}
	return best, bestScore >= 10
}

// bundledAdvice is the "what to do" for a component that is a copy inside another program,
// with a link to the program's homepage (or a web search when it is not known).
func bundledAdvice(o ownerInfo) string {
	if o.URL != "" {
		return Tf("bundled.advice", o.Name, o.Name, o.URL)
	}
	return Tf("bundled.adviceSearch", o.Name, o.Name, searchLink(o.Name))
}

// searchLink is a markup link to a web search.
func searchLink(q string) string {
	return "[[" + Tf("search.label", q) + "|" + googleURL(q) + "]]"
}

func searchFor(owner, name string, bundled bool) string {
	if bundled || name == "" {
		return owner
	}
	return name
}

// ---------------------------------------------------------------------------
// Helpers

func bundleProblem(r bundleResult) string {
	roots := strings.Join(r.RootsPresent, ", ")
	if roots == "" {
		roots = "?"
	}
	return Tf("bundle.problem", r.Trusted, tlsChainsTotal, roots)
}

func (tp *appsContext) checkBundleFile(p string) (bundleResult, bool) {
	tp.handled[pathKey(p)] = true
	if !exists(p) {
		return bundleResult{}, false
	}
	return tp.bc.checkFile(p)
}

func distInfoVersion(sitePackages, pkg string) string {
	m, _ := filepath.Glob(filepath.Join(sitePackages, pkg+"-*.dist-info"))
	best := ""
	for _, d := range m {
		v := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(d), pkg+"-"), ".dist-info")
		if best == "" || ParseVersion(v).Cmp(ParseVersion(best)) > 0 {
			best = v
		}
	}
	return best
}

func displayVersion(fv fileVersion) string {
	if fv.Product != "" {
		return fv.Product
	}
	if fv.File != "" {
		return fv.File
	}
	if len(fv.Fixed) > 0 {
		return fv.Fixed.String()
	}
	return ""
}

// trimZeros turns "26.12.1.0" into "26.12.1".
func trimZeros(s string) string {
	v := ParseVersion(s)
	if len(v) == 4 && v[3] == 0 && !strings.ContainsAny(s, " -+") {
		return v[:3].String()
	}
	return s
}

func (tp *appsContext) inWindowsDir(p string) bool {
	return tp.winDir != "" && hasPathPrefix(p, tp.winDir)
}

func bullet(lines []string) string { return "• " + strings.Join(lines, "\n• ") }

// ---------------------------------------------------------------------------
// Git for Windows

func gitSSLBackend(root string) string {
	backend := "openssl" // Git for Windows default
	files := []string{
		filepath.Join(getenv("ProgramData"), "Git", "config"),
		filepath.Join(root, "etc", "gitconfig"),
		filepath.Join(root, "mingw64", "etc", "gitconfig"),
	}
	up := getenv("USERPROFILE")
	if x := getenv("XDG_CONFIG_HOME"); x != "" {
		files = append(files, filepath.Join(x, "git", "config"))
	} else if up != "" {
		files = append(files, filepath.Join(up, ".config", "git", "config"))
	}
	if h := getenv("HOME"); h != "" {
		files = append(files, filepath.Join(h, ".gitconfig"))
	} else if up != "" {
		files = append(files, filepath.Join(up, ".gitconfig"))
	}
	for _, f := range files {
		if v := gitConfigValue(f, "http", "sslbackend"); v != "" {
			backend = lower(v)
		}
	}
	return backend
}

func gitConfigValue(file, section, key string) string {
	f, err := os.Open(file)
	if err != nil {
		return ""
	}
	defer f.Close()
	cur, val := "", ""
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] == '#' || line[0] == ';' {
			continue
		}
		if line[0] == '[' {
			cur = ""
			if fl := strings.Fields(strings.Trim(line, "[]")); len(fl) > 0 {
				cur = lower(strings.Trim(fl[0], `"`))
			}
			continue
		}
		if cur != section {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if ok && lower(strings.TrimSpace(k)) == key {
			val = strings.Trim(strings.TrimSpace(v), `"`)
		}
	}
	return val
}

func (tp *appsContext) checkGit() {
	for _, root := range dedupePaths(tp.d.gitRoots) {
		root := root
		tp.c.begin(tp.groupOf("Git for Windows", root), "git", "Git for Windows", root, func(it *Item) { tp.gitOne(it, root) })
	}
}

func (tp *appsContext) gitOne(it *Item, root string) {
	exe := filepath.Join(root, "cmd", "git.exe")
	if !exists(exe) {
		exe = filepath.Join(root, "mingw64", "bin", "git.exe")
	}
	fv, _ := getFileVersion(exe)
	ver := displayVersion(fv)
	v := ParseVersion(ver)
	if len(fv.Fixed) == 4 && len(v) < 4 {
		v = fv.Fixed
	}
	r := result{Version: ver, Folder: filepath.Dir(exe), File: exe, Exe: exe, ProductURL: "https://gitforwindows.org/"}
	o := tp.withOwner(&r, root)
	bundled := bundledIn(o.Name, kwGit...)
	r.Bundled = bundled
	r.Search = searchFor(o.Name, "Git for Windows", bundled)
	state, fixedIn := fixStatus("git", v)

	candidates := map[string]bool{}
	for _, rel := range [][]string{{"mingw64", "etc", "ssl", "certs", "ca-bundle.crt"}, {"mingw64", "etc", "ssl", "cert.pem"},
		{"mingw64", "ssl", "certs", "ca-bundle.crt"}, {"clangarm64", "etc", "ssl", "certs", "ca-bundle.crt"},
		{"mingw32", "etc", "ssl", "certs", "ca-bundle.crt"}, {"usr", "ssl", "certs", "ca-bundle.crt"}} {
		candidates[filepath.Join(append([]string{root}, rel...)...)] = true
	}
	for b := range tp.d.bundles {
		if hasPathPrefix(b, root) {
			candidates[b] = true
		}
	}
	var bad, badFiles []string
	anyBundle := false
	for _, p := range dedupePaths(candidates) {
		br, ok := tp.checkBundleFile(p)
		if !ok {
			continue
		}
		anyBundle = true
		if br.Bad() {
			if len(bad) == 0 {
				r.File = p
			}
			bad = append(bad, lk(p)+" — "+bundleProblem(br))
			badFiles = append(badFiles, p)
		}
	}
	backend := gitSSLBackend(root)
	details := field(T("git.backendLbl"), backend)
	fix := Tf("git.fix", fixedVersionsList("git"))
	switch {
	case len(bad) > 0:
		backendText := T("git.backendOpenSSL")
		if backend == "schannel" {
			backendText = T("git.backendSchannel")
		}
		r.Status, r.Summary = StWarn, T("git.sumBad")
		if bundled {
			fix = joinParas(bundledAdvice(o), T("fix.directly")+"\n"+replaceBundleFix(badFiles...))
		} else {
			fix = joinParas(fix, T("fix.orDirectly")+"\n"+replaceBundleFix(badFiles...))
		}
		r.Details = details + "\n\n" + T("git.badBundle") + "\n" + bullet(bad) + "\n\n" + backendText + "\n\n" + fix
	case !anyBundle && state != FixOK:
		r.Status, r.Summary = StWarn, Tf("git.sumVersion", fixedIn)
		if state == FixUnknown {
			r.Status = StCaution
		}
		if bundled {
			fix = joinParas(bundledAdvice(o), fix)
		}
		r.Details = details + "\n\n" + T("git.noBundle") + "\n\n" + fix
	default:
		r.Status, r.Summary = StOK, T("git.sumOK")
		r.Details = details + "\n\n" + T("git.okBundle")
	}
	tp.c.finish(it, r)
}

// ---------------------------------------------------------------------------
// curl

func (tp *appsContext) checkCurl() {
	for _, p := range dedupePaths(tp.d.curlExes) {
		inGit := false
		for g := range tp.d.gitRoots {
			inGit = inGit || hasPathPrefix(p, g)
		}
		if inGit || tp.inWindowsDir(p) || curlBundle(p) == "" {
			continue // Git's own curl is covered by Git; others without a bundle use Windows TLS
		}
		p := p
		tp.c.begin(tp.groupOf("curl", p), "curl", "curl", filepath.Dir(p), func(it *Item) { tp.curlOne(it, p) })
	}
	// Windows' own curl.exe, only when the Windows drive is scanned
	sys := filepath.Join(tp.winDir, "System32", "curl.exe")
	if tp.winDir != "" && tp.inScope(sys) && exists(sys) {
		tp.c.begin(GroupApps, "curl", T("curl.sysName"), filepath.Dir(sys), func(it *Item) {
			fv, _ := getFileVersion(sys)
			r := result{Version: trimZeros(displayVersion(fv)), File: sys, Exe: sys, Folder: filepath.Dir(sys), Owner: "Windows",
				ProductURL: "https://curl.se/windows/", Search: "curl.exe Windows", Status: StOK, Summary: T("curl.sysOK"), Details: T("curl.sysDetails")}
			if !tp.windowsBlock {
				r.Status, r.Summary = StWarn, T("curl.sysDependsOnWindows")
			}
			tp.c.finish(it, r)
		})
	}
}

func curlBundle(exe string) string {
	dir := filepath.Dir(exe)
	for _, cand := range []string{filepath.Join(dir, "curl-ca-bundle.crt"), filepath.Join(filepath.Dir(dir), "ssl", "certs", "ca-bundle.crt"),
		filepath.Join(filepath.Dir(dir), "etc", "ssl", "certs", "ca-bundle.crt")} {
		if exists(cand) {
			return cand
		}
	}
	return ""
}

func (tp *appsContext) curlOne(it *Item, exe string) {
	fv, _ := getFileVersion(exe)
	bundle := curlBundle(exe)
	r := result{Version: trimZeros(displayVersion(fv)), File: bundle, Exe: exe, Folder: filepath.Dir(exe), ProductURL: "https://curl.se/windows/"}
	o := tp.withOwner(&r, exe)
	bundled := bundledIn(o.Name, "curl")
	r.Bundled = bundled
	r.Search = searchFor(o.Name, "curl for Windows", bundled)
	br, ok := tp.checkBundleFile(bundle)
	details := ""
	switch {
	case ok && br.Bad():
		fix := joinParas(T("curl.fix"), replaceBundleFix(bundle))
		if bundled {
			fix = joinParas(bundledAdvice(o), T("fix.directly")+"\n"+replaceBundleFix(bundle))
		}
		r.Status, r.Summary, r.Details = StWarn, T("curl.sumBad"), details+"\n\n"+bundleProblem(br)+"\n\n"+fix
	case ok:
		r.Status, r.Summary, r.Details = StOK, T("curl.sumOK"), T("ok.bundle")
	default:
		r.Status, r.Summary, r.Details = StCaution, T("curl.sumUnknown"), details
	}
	tp.c.finish(it, r)
}

// ---------------------------------------------------------------------------
// Python

type pyEnv struct {
	exe, home, sitePackages string
	venv                    bool
	version                 string
}

func pythonEnvFor(exe string) pyEnv {
	dir := filepath.Dir(exe)
	e := pyEnv{exe: exe, home: dir}
	if lower(filepath.Base(dir)) == "scripts" && exists(filepath.Join(filepath.Dir(dir), "pyvenv.cfg")) {
		e.venv, e.home = true, filepath.Dir(dir)
	} else if exists(filepath.Join(dir, "pyvenv.cfg")) {
		e.venv = true
	}
	e.sitePackages = filepath.Join(e.home, "Lib", "site-packages")
	if e.venv {
		e.version = pyvenvVersion(filepath.Join(e.home, "pyvenv.cfg"))
	} else {
		fv, _ := getFileVersion(exe)
		e.version = pythonVersionFromFile(fv)
		if e.version == "" {
			m, _ := filepath.Glob(filepath.Join(dir, "python3*.dll"))
			for _, dll := range m {
				if fv, ok := getFileVersion(dll); ok {
					if v := pythonVersionFromFile(fv); v != "" {
						e.version = v
					}
				}
			}
		}
	}
	return e
}

// Python encodes the micro version as micro*1000+level*10+serial in its file version.
func pythonVersionFromFile(fv fileVersion) string {
	if fv.Product != "" && len(ParseVersion(fv.Product)) >= 3 && ParseVersion(fv.Product)[2] < 1000 {
		return ParseVersion(fv.Product).String()
	}
	f := fv.Fixed
	if len(f) >= 3 && f[0] == 3 {
		micro := f[2]
		if micro >= 1000 {
			micro /= 1000
		}
		return fmt.Sprintf("%d.%d.%d", f[0], f[1], micro)
	}
	return ""
}

func pyvenvVersion(cfg string) string { return pyvenvValue(cfg, "version", "version_info") }

// pyvenvHome is the folder of the Python that a virtual environment runs (home =
// in pyvenv.cfg): its python.exe is a launcher for that one, whose ssl module and
// version are the environment's, even when pyvenv.cfg recorded an older version.
func pyvenvHome(cfg string) string { return pyvenvValue(cfg, "home") }

func pyvenvValue(cfg string, keys ...string) string {
	data, err := os.ReadFile(cfg)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		for _, key := range keys {
			if strings.TrimSpace(k) == key {
				return strings.TrimSpace(v)
			}
		}
	}
	return ""
}

// pyFix tells whether a Python refuses the compromised certificates itself: from
// 3.11.18, 3.12.16, 3.13.18, 3.14.10 and 3.15.2 (CVE-2026-115557), its ssl module
// rejects any chain containing one of the 8 compromised keys, whatever certificate
// list is used (the Windows roots it copies, certifi, pip's copy of certifi). The
// block list is in the ssl module's library (_ssl.pyd), where eMudScan also looks
// for it: that recognises the fix in builds with other version numbers (conda,
// Store, embedded or backported Pythons).
type pyFix struct {
	version string   // of the Python that runs: the base one, for a virtual environment
	state   FixState // of that version
	fixed   string   // version to update to, when outdated
	sslLib  string   // the ssl module's library read, "" when not found
	inSSL   bool     // the block list is in sslLib
}

// protected: secure connections made through the ssl module refuse the compromised certificates.
func (f pyFix) protected() bool { return f.state == FixOK || f.inSSL }

func (tp *appsContext) pythonFix(env pyEnv) pyFix {
	f := pyFix{version: env.version}
	base := env.home
	if env.venv {
		base = ""
		if h := pyvenvHome(filepath.Join(env.home, "pyvenv.cfg")); h != "" && (exists(filepath.Join(h, "python.exe")) || exists(filepath.Join(h, "python3.dll"))) {
			base = h
			if exists(filepath.Join(h, "python.exe")) {
				if v := pythonEnvFor(filepath.Join(h, "python.exe")).version; v != "" {
					f.version = v
				}
			}
		}
	}
	f.state, f.fixed = fixStatus("python", ParseVersion(f.version))
	if base != "" {
		for _, lib := range []string{filepath.Join(base, "DLLs", "_ssl.pyd"), filepath.Join(base, "_ssl.pyd")} {
			if exists(lib) {
				f.sslLib = lib
				if found, err := tp.scan(lib, scanRoots); err == nil && found.Blocklist {
					f.inSSL = true
				}
				break
			}
		}
	}
	return f
}

// pythonBlocksText says why a Python refuses the compromised certificates: its version,
// or the block list found in its ssl module.
func pythonBlocksText(f pyFix) string {
	if f.state != FixOK && f.inSSL {
		return Tf("py.sslBlocksFile", lk(f.sslLib)) + " " + T("py.sslBlocksNote")
	}
	return Tf("py.sslBlocksVersion", f.version, fixedVersionsList("python")) + " " + T("py.sslBlocksNote")
}

func (tp *appsContext) checkPython() {
	for _, exe := range dedupePaths(tp.d.pythonExes) {
		env := pythonEnvFor(exe)
		if env.venv {
			continue // virtual environments are reported through their bundles
		}
		if !exists(filepath.Join(env.home, "Lib")) && !exists(filepath.Join(env.home, "python3.dll")) {
			continue // a launcher or a shim
		}
		exe := exe
		tp.c.begin(tp.groupOf("Python", env.home), "python", "Python", env.home, func(it *Item) { tp.pythonOne(it, exe) })
	}
}

func (tp *appsContext) pythonOne(it *Item, exe string) { tp.pythonOneEnv(it, pythonEnvFor(exe)) }

func (tp *appsContext) pythonOneEnv(it *Item, env pyEnv) {
	exe := env.exe
	r := result{Version: env.version, Folder: env.home, File: exe, Exe: exe, ProductURL: "https://www.python.org/downloads/windows/"}
	o := tp.withOwner(&r, env.home)
	bundled := bundledIn(o.Name, kwPython...)
	r.Bundled = bundled
	r.Search = searchFor(o.Name, "Python "+env.version, bundled)
	fx := tp.pythonFix(env)
	var problems, cmds []string
	listsChecked := false // pip's or certifi's list exists and was tested
	pipVer := distInfoVersion(env.sitePackages, "pip")
	pipBundle := filepath.Join(env.sitePackages, "pip", "_vendor", "certifi", "cacert.pem")
	if br, ok := tp.checkBundleFile(pipBundle); ok {
		listsChecked = true
		if br.Bad() {
			problems = append(problems, Tf("py.pipBad", pipVer, lk(pipBundle))+" "+bundleProblem(br))
			cmds = append(cmds, pipCmd(exe, "pip"))
			r.File = pipBundle
		}
	}
	certifiVer := distInfoVersion(env.sitePackages, "certifi")
	certifiBundle := filepath.Join(env.sitePackages, "certifi", "cacert.pem")
	if br, ok := tp.checkBundleFile(certifiBundle); ok {
		listsChecked = true
		if br.Bad() {
			problems = append(problems, Tf("py.certifiBad", certifiVer, lk(certifiBundle))+" "+bundleProblem(br))
			cmds = append(cmds, pipCmd(exe, "certifi"))
			if r.File == exe {
				r.File = certifiBundle
			}
		}
	}
	ensurepip := ""
	if m, _ := filepath.Glob(filepath.Join(env.home, "Lib", "ensurepip", "_bundled", "pip-*.whl")); len(m) > 0 {
		ensurepip = strings.SplitN(strings.TrimPrefix(filepath.Base(m[len(m)-1]), "pip-"), "-", 2)[0]
	}
	var dl []string
	if pipVer != "" {
		dl = append(dl, field(T("py.pipVerLbl"), pipVer))
	}
	if certifiVer != "" {
		dl = append(dl, field(T("py.certifiVerLbl"), certifiVer))
	}
	details := strings.Join(dl, "\n")
	// the pip and certifi commands, as the fix or as good practice
	listsFix := func() string {
		if len(cmds) == 0 {
			return ""
		}
		if bundled {
			return joinParas(bundledAdvice(o), T("py.ifYouManage")+" "+psHeader(exe)+"\n"+strings.Join(cmds, "\n"))
		}
		return psHeader(exe) + "\n" + strings.Join(cmds, "\n")
	}
	if fx.protected() {
		// the ssl module refuses the compromised certificates: the lists no longer decide
		r.File = exe
		r.Status, r.Summary = StOK, T("py.sumSSLBlocks")
		r.Details = joinParas(pythonBlocksText(fx), details)
		if len(problems) > 0 {
			r.Details = joinParas(r.Details, T("py.listsOldSafe")+"\n"+strings.Join(problems, "\n"), T("py.updateAnyway")+"\n"+listsFix())
		} else if listsChecked {
			r.Details = joinParas(r.Details, T("py.okDetails"))
		}
		tp.c.finish(it, r)
		return
	}
	ensureOld := ensurepip != "" && ParseVersion(ensurepip).Cmp(ParseVersion("26.2.2")) < 0
	ensureText := ""
	if ensureOld {
		ensureText = Tf("py.ensurepip", ensurepip, pyFixedText(fx.state, fx.fixed))
	}
	// what this Python does not do yet, and the update that brings it
	exposed := ""
	if fx.state != FixUnknown {
		exposed = Tf("py.notBlocking", pyFixedVersions(fx))
	}
	if tp.pyStoreRisk > 0 {
		exposed = joinParas(exposed, Tf("py.sslStore", tp.pyStoreRisk, tlsChainsTotal, refWinCopy()))
	}
	update := ""
	switch fx.state {
	case FixOutdated:
		update = Tf("py.updateFix", fx.fixed)
	case FixUnsupported:
		update = Tf("py.unsupportedFix", fx.version, fixedVersionsList("python"))
	}
	if update != "" && bundled {
		update = joinParas(bundledAdvice(o), T("py.ifYouManage")+" "+update)
	}
	switch {
	case len(problems) > 0:
		r.Status, r.Summary = StWarn, T("py.sumBad")
		r.Details = joinParas(details, strings.Join(problems, "\n"), exposed, update, listsFix(), ensureText)
	case fx.state == FixOutdated:
		r.Status, r.Summary = StCaution, Tf("py.sumUpdate", fx.fixed)
		r.Details = joinParas(details, exposed, update, ensureText)
	case fx.state == FixUnsupported:
		r.Status, r.Summary = StCaution, Tf("py.sumUnsupported", fx.version)
		r.Details = joinParas(details, exposed, update, ensureText)
	case ensureOld:
		r.Status, r.Summary = StCaution, Tf("py.sumEnsurepip", ensurepip)
		r.Details = joinParas(details, exposed, ensureText)
	default: // version unknown: judged by its lists, as before
		r.Status, r.Summary, r.Details = StOK, T("py.sumOK"), joinParas(details, exposed)
		if listsChecked {
			r.Details = joinParas(T("py.okDetails"), r.Details)
		}
	}
	tp.c.finish(it, r)
}

// pyFixedVersions is the fixed version of this Python's line, or all of them.
func pyFixedVersions(f pyFix) string {
	if f.state == FixOutdated && f.fixed != "" {
		return f.fixed
	}
	return fixedVersionsList("python")
}

func pyFixedText(st FixState, fixed string) string {
	switch st {
	case FixOutdated:
		return Tf("py.updateTo", fixed)
	case FixUnsupported:
		return Tf("py.unsupportedLine", fixedVersionsList("python"))
	}
	return T("py.upgradePipEach")
}

// ---------------------------------------------------------------------------
// Node.js

func (tp *appsContext) checkNode() {
	for _, exe := range dedupePaths(tp.d.nodeExes) {
		exe := exe
		tp.c.begin(tp.groupOf("Node.js", exe), "node", "Node.js", filepath.Dir(exe), func(it *Item) { tp.nodeOne(it, exe) })
	}
}

func (tp *appsContext) nodeOne(it *Item, exe string) {
	fv, _ := getFileVersion(exe)
	v := fv.Fixed
	if len(ParseVersion(fv.Product)) >= 3 {
		v = ParseVersion(fv.Product)
	}
	r := result{Version: trimZeros(displayVersion(fv)), File: exe, Exe: exe, Folder: filepath.Dir(exe), ProductURL: "https://nodejs.org/en/download"}
	o := tp.withOwner(&r, exe)
	bundled := bundledIn(o.Name, kwNode...)
	r.Bundled = bundled
	r.Search = searchFor(o.Name, "Node.js", bundled)
	details := ""
	fix := Tf("node.fix", fixedVersionsList("node"))
	fixU := Tf("node.fixUnsupported", fixedVersionsList("node"))
	if bundled {
		fix = joinParas(bundledAdvice(o), Tf("node.bundledNeeds", fixedVersionsList("node")))
		fixU = fix
	}
	st, fixed := fixStatus("node", v)
	switch st {
	case FixOK:
		r.Status, r.Summary, r.Details = StOK, T("node.sumOK"), Tf("node.okDetails", r.Version, fixedVersionsList("node"))
	case FixOutdated:
		r.Status, r.Summary, r.Details = StWarn, Tf("node.sumOutdated", fixed), details+"\n\n"+fix
	case FixUnsupported:
		r.Status, r.Summary, r.Details = StWarn, T("node.sumUnsupported"), details+"\n\n"+T("node.unsupportedNote")+"\n\n"+fixU
	default:
		f, _ := tp.scan(exe, scanRoots)
		if f.HasEmsignRoot() && !f.Blocklist {
			r.Status, r.Summary, r.Details = StWarn, T("embed.sumBad"), details+"\n\n"+fix
		} else {
			r.Status, r.Summary, r.Details = StCaution, T("version.unknown"), details+"\n\n"+fix
		}
	}
	tp.c.finish(it, r)
}

// ---------------------------------------------------------------------------
// Java

var javaReleaseRe = regexp.MustCompile(`(?m)^(JAVA_VERSION|IMPLEMENTOR|IMPLEMENTOR_VERSION|JAVA_RUNTIME_VERSION)="?([^"\r\n]*)"?`)

func javaRelease(home string) (version, vendor string) {
	for _, p := range []string{filepath.Join(home, "release"), filepath.Join(filepath.Dir(home), "release")} {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		vals := map[string]string{}
		for _, m := range javaReleaseRe.FindAllStringSubmatch(string(data), -1) {
			vals[m[1]] = m[2]
		}
		version = vals["JAVA_RUNTIME_VERSION"]
		if version == "" {
			version = vals["JAVA_VERSION"]
		}
		vendor = vals["IMPLEMENTOR"]
		return
	}
	return
}

func javaURL(vendor string) string {
	v := lower(vendor)
	switch {
	case strings.Contains(v, "adoptium") || strings.Contains(v, "temurin"):
		return "https://adoptium.net/"
	case strings.Contains(v, "oracle"):
		return "https://www.oracle.com/java/technologies/downloads/"
	case strings.Contains(v, "azul"):
		return "https://www.azul.com/downloads/"
	case strings.Contains(v, "microsoft"):
		return "https://learn.microsoft.com/java/openjdk/download"
	case strings.Contains(v, "amazon"):
		return "https://aws.amazon.com/corretto/"
	case strings.Contains(v, "bellsoft"):
		return "https://bell-sw.com/pages/downloads/"
	case strings.Contains(v, "jetbrains"):
		return "https://github.com/JetBrains/JetBrainsRuntime/releases"
	}
	return "https://jdk.java.net/"
}

// javaBlockedCount returns how many compromised TLS certificates are in the JDK block list.
func javaBlockedCount(home string) (direct, cross int, file string) {
	for _, n := range []string{"blocked.certs", "blacklisted.certs"} {
		p := filepath.Join(home, "lib", "security", n)
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		file = p
		up := bytes.ReplaceAll(bytes.ToUpper(data), []byte(":"), nil)
		for _, c := range badCerts {
			if c.Usage == "tls" && bytes.Contains(up, []byte(c.SHA256)) {
				if c.CrossSigned() {
					cross++
				} else {
					direct++
				}
			}
		}
		return
	}
	return
}

// javaBlockCmd builds the PowerShell commands that add the missing fingerprints to the JDK block list.
func javaBlockCmd(home, blFile string) string {
	if blFile == "" {
		return ""
	}
	data, _ := os.ReadFile(blFile)
	up := bytes.ReplaceAll(bytes.ToUpper(data), []byte(":"), nil)
	var missing []string
	for _, c := range badCerts {
		if !bytes.Contains(up, []byte(c.SHA256)) {
			missing = append(missing, "'"+c.SHA256+"'")
		}
	}
	if len(missing) == 0 {
		return ""
	}
	q := strings.ReplaceAll(blFile, "'", "''")
	return psHeader(blFile) + "\n" +
		cmd(fmt.Sprintf(`Copy-Item -LiteralPath '%s' -Destination '%s.bak'`, q, q)) + "\n" +
		cmd(fmt.Sprintf(`Add-Content -LiteralPath '%s' -Value '',%s`, q, strings.Join(missing, ",")))
}

func (tp *appsContext) checkJava() {
	for _, home := range dedupePaths(tp.d.javaHomes) {
		if !exists(filepath.Join(home, "lib", "security", "cacerts")) {
			continue
		}
		home := home
		tp.c.begin(tp.groupOf("Java", home), "java", "Java", home, func(it *Item) { tp.javaOne(it, home) })
	}
}

func (tp *appsContext) javaOne(it *Item, home string) {
	cacerts := filepath.Join(home, "lib", "security", "cacerts")
	ver, vendor := javaRelease(home)
	name := "Java"
	if vendor != "" {
		name += " (" + vendor + ")"
	}
	bin := filepath.Join(home, "bin")
	r := result{Name: name, Version: ver, File: cacerts, Exe: filepath.Join(bin, "java.exe"), Folder: bin, ProductURL: javaURL(vendor)}
	o := tp.withOwner(&r, home)
	bundled := bundledIn(o.Name, kwJava...)
	r.Bundled = bundled
	r.Search = searchFor(o.Name, name+" "+ver, bundled)
	details := field(T("java.homeLbl"), lk(home))
	f, err := tp.scan(cacerts, scanRoots)
	if err != nil {
		r.Status, r.Summary, r.Details = StCaution, T("java.sumUnreadable"), details+"\n\n"+err.Error()
		tp.c.finish(it, r)
		return
	}
	hasEm, hasAAA := f.HasEmsignRoot(), false
	for _, rt := range f.Roots {
		hasAAA = hasAAA || rt == rootNameAAA
	}
	direct, cross, blFile := javaBlockedCount(home)
	roots := strings.Join(f.Roots, ", ")
	if roots == "" {
		roots = "—"
	}
	details += "\n" + field(T("lbl.affectedRoots"), roots)
	if blFile != "" {
		details += "\n" + field(T("java.blockedLbl"), Tf("java.blockedVal", direct+cross, lk(blFile)))
	}
	switch {
	case (hasEm && direct < 6) || (hasAAA && cross < 6):
		fix := joinParas(T("java.fix"), T("java.manual"), javaBlockCmd(home, blFile))
		if bundled {
			// the program ships its own Java: its update brings the fix
			fix = joinParas(bundledAdvice(o), T("java.manual"), javaBlockCmd(home, blFile))
		}
		r.Status, r.Summary, r.Details = StWarn, T("java.sumBad"), details+"\n\n"+fix
	case hasEm || hasAAA:
		r.Status, r.Summary, r.Details = StOK, T("java.sumBlocked"), joinParas(details, T("java.okBlocked"))
	default:
		r.Status, r.Summary, r.Details = StOK, T("java.sumNoRoots"), joinParas(details, T("java.okNoRoots"))
	}
	tp.c.finish(it, r)
}

// ---------------------------------------------------------------------------
// Electron applications

var electronExcluded = map[string]bool{
	"update.exe": true, "squirrel.exe": true, "elevate.exe": true, "crashpad_handler.exe": true,
	"notification_helper.exe": true, "chrome_proxy.exe": true, "chrome_pwa_launcher.exe": true,
}

func electronMainExe(dir string) string {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	best, bestSize := "", int64(0)
	for _, e := range ents {
		n := lower(e.Name())
		if e.IsDir() || filepath.Ext(n) != ".exe" || electronExcluded[n] || strings.HasPrefix(n, "unins") {
			continue
		}
		fi, err := e.Info()
		if err != nil || isCloudPlaceholder(fi) {
			continue
		}
		if fi.Size() > bestSize {
			best, bestSize = filepath.Join(dir, e.Name()), fi.Size()
		}
	}
	if bestSize < 20<<20 {
		return ""
	}
	return best
}

var squirrelDirRe = regexp.MustCompile(`(?i)^app-(\d+(\.\d+)*)`)

// newestSquirrel keeps only the newest app-x.y.z folder of Squirrel-installed apps.
func newestSquirrel(dirs []string) []string {
	best := map[string]string{}
	var out []string
	for _, d := range dirs {
		m := squirrelDirRe.FindStringSubmatch(filepath.Base(d))
		if m == nil {
			out = append(out, d)
			continue
		}
		parent := pathKey(filepath.Dir(d))
		if cur, ok := best[parent]; !ok || ParseVersion(m[1]).Cmp(ParseVersion(squirrelDirRe.FindStringSubmatch(filepath.Base(cur))[1])) > 0 {
			best[parent] = d
		}
	}
	for _, d := range best {
		out = append(out, d)
	}
	sort.Strings(out)
	return out
}

// currentSquirrelExe: Squirrel installs each update in a new app-x.y.z folder (and
// removes the old one), so "Scan again" looks for the program in the newest folder.
func currentSquirrelExe(exe string) string {
	dir := filepath.Dir(exe)
	if !squirrelDirRe.MatchString(filepath.Base(dir)) {
		return exe
	}
	all, _ := filepath.Glob(filepath.Join(filepath.Dir(dir), "app-*"))
	for _, d := range newestSquirrel(all) {
		if p := filepath.Join(d, filepath.Base(exe)); squirrelDirRe.MatchString(filepath.Base(d)) && exists(p) {
			return p
		}
	}
	return exe
}

type knownApp struct{ exe, contains, product string }

var knownElectronApps = []knownApp{
	{"code.exe", "visual studio code", "vscode"},
	{"obsidian.exe", "obsidian", "obsidian"},
	{"docker desktop.exe", "docker", "docker-desktop"},
}

func (tp *appsContext) checkElectron() {
	for _, dir := range newestSquirrel(dedupePaths(tp.d.electronDirs)) {
		exe := electronMainExe(dir)
		if exe == "" {
			continue
		}
		fv, _ := getFileVersion(exe)
		name := fv.Name
		if name == "" {
			name = fv.Desc
		}
		if name == "" {
			name = strings.TrimSuffix(filepath.Base(exe), filepath.Ext(exe))
		}
		tp.c.begin(tp.groupOf(name, dir), "electron", name, dir, func(it *Item) { tp.electronOne(it, currentSquirrelExe(exe), name) })
	}
}

func (tp *appsContext) electronOne(it *Item, exe, name string) {
	fv, _ := getFileVersion(exe)
	appVer := trimZeros(displayVersion(fv))
	r := result{Version: appVer, File: exe, Exe: exe, Folder: filepath.Dir(exe)}
	o := tp.withOwner(&r, exe)
	r.Search = searchFor(o.Name, name, false)
	tp.c.activity(Tf("act.read", exe))
	f, _ := tp.scan(exe, scanVersions)
	tp.c.activity("")
	details := ""
	if f.Electron != "" {
		built := "Electron " + f.Electron
		if f.Chrome != "" {
			built += " (Chromium " + f.Chrome + ")"
		}
		details = field(T("electron.builtOnLbl"), built)
	}
	product := ""
	for _, k := range knownElectronApps {
		if lower(filepath.Base(exe)) == k.exe && strings.Contains(lower(name), k.contains) {
			product = k.product
		}
	}
	if product != "" {
		switch st, fixed := fixStatus(product, ParseVersion(appVer)); st {
		case FixOK:
			r.Status, r.Summary, r.Details = StOK, T("app.sumOK"), joinParas(details, Tf("app.okDetails", name, appVer))
			tp.c.finish(it, r)
			return
		case FixOutdated:
			r.Status, r.Summary, r.Details = StWarn, Tf("app.sumOutdated", fixed), details+"\n\n"+Tf("app.fix", name, fixed)
			tp.c.finish(it, r)
			return
		}
	}
	if f.Electron != "" {
		// the checked component is Electron: the list shows its version
		r.Component, r.Version, r.ProgVersion = "Electron", f.Electron, appVer
	}
	switch {
	case f.Electron != "":
		st, fixed := fixStatus("electron", ParseVersion(f.Electron))
		switch st {
		case FixOK:
			r.Status, r.Summary, r.Details = StOK, Tf("electron.sumOKVer", f.Electron), joinParas(details, Tf("electron.okDetails", name, f.Electron, fixedVersionsList("electron")))
		case FixOutdated:
			r.Status, r.Summary, r.Details = StWarn, Tf("electron.sumOutdated", f.Electron), details+"\n\n"+Tf("electron.fix", name, fixed, fixedVersionsList("electron"))
		default:
			r.Status, r.Summary, r.Details = StWarn, Tf("electron.sumUnsupported", f.Electron), details+"\n\n"+Tf("electron.fixUnsupported", name, fixedVersionsList("electron"))
		}
	case f.HasEmsignRoot() && !f.Blocklist:
		r.Status, r.Summary, r.Details = StWarn, T("embed.sumBad"), details+"\n\n"+Tf("electron.fix", name, "?", fixedVersionsList("electron"))
	default:
		r.Status, r.Summary, r.Details = StCaution, T("version.unknown"), details+"\n\n"+Tf("electron.unknown", fixedVersionsList("electron"))
	}
	tp.c.finish(it, r)
}

// ---------------------------------------------------------------------------
// Go toolchains

func (tp *appsContext) checkGo() {
	for _, root := range dedupePaths(tp.d.goRoots) {
		root := root
		tp.c.begin(tp.groupOf("Go", root), "go", "Go", root, func(it *Item) { tp.goOne(it, root) })
	}
}

func (tp *appsContext) goOne(it *Item, root string) {
	data, _ := os.ReadFile(filepath.Join(root, "VERSION"))
	ver := strings.TrimPrefix(strings.TrimSpace(strings.SplitN(string(data), "\n", 2)[0]), "go")
	exe := filepath.Join(root, "bin", "go.exe")
	r := result{Version: ver, File: exe, Exe: exe, Folder: filepath.Join(root, "bin"), ProductURL: "https://go.dev/dl/"}
	o := tp.withOwner(&r, root)
	r.Bundled = bundledIn(o.Name, kwGo...)
	r.Search = searchFor(o.Name, "Go "+ver, r.Bundled)
	details := ""
	switch st, fixed := fixStatus("go", ParseVersion(ver)); st {
	case FixOK:
		r.Status, r.Summary, r.Details = StOK, T("app.sumOK"), Tf("go.okDetails", ver)
	case FixUnknown:
		r.Status, r.Summary, r.Details = StCaution, T("version.unknown"), details
	default:
		r.Status, r.Summary, r.Details = StWarn, Tf("app.sumOutdated", fixed), details+"\n\n"+Tf("go.fix", fixedVersionsList("go"))
	}
	tp.c.finish(it, r)
}

// ---------------------------------------------------------------------------
// Programs identified from "Apps & features"

func (tp *appsContext) checkUninstallApps() {
	rules := map[string]struct{ product, url string }{
		"docker desktop": {"docker-desktop", "https://www.docker.com/products/docker-desktop/"},
		"github cli":     {"gh", "https://cli.github.com/"},
	}
	for _, e := range tp.entries {
		rule, ok := rules[lower(e.name)]
		if !ok || !tp.inScope(e.location) {
			continue
		}
		first := e
		tp.c.begin(tp.groupOf(e.name, e.location), "app", e.name, e.location, func(it *Item) {
			e := tp.currentEntry(first)
			r := result{Version: e.version, Folder: e.location, Owner: e.name, OwnerURL: e.url, ProductURL: rule.url, Search: e.name}
			if e.icon != "" && strings.EqualFold(filepath.Ext(e.icon), ".exe") {
				r.Exe = e.icon
			}
			details := ""
			switch st, fixed := fixStatus(rule.product, ParseVersion(e.version)); st {
			case FixOK:
				r.Status, r.Summary, r.Details = StOK, T("app.sumOK"), Tf("app.okDetails", e.name, e.version)
			case FixOutdated:
				r.Status, r.Summary, r.Details = StWarn, Tf("app.sumOutdated", fixed), details+"\n\n"+Tf("app.fix", e.name, fixed)
			default:
				r.Status, r.Summary, r.Details = StCaution, T("version.unknown"), details
			}
			tp.c.finish(it, r)
		})
	}
}

// ---------------------------------------------------------------------------
// Command-line programs with a built-in certificate list

var embeddedToolProducts = map[string]string{
	"uv.exe": "uv", "uvx.exe": "uv", "pixi.exe": "pixi", "deno.exe": "deno", "bun.exe": "bun",
	"rustup.exe": "rustup", "rustup-init.exe": "rustup", "mise.exe": "mise", "claude.exe": "claude-code",
	"gh.exe": "gh", "docker.exe": "docker", "conda.exe": "conda",
}

func (tp *appsContext) toolDirs() []string {
	dirs := pathDirs()
	up, lad := getenv("USERPROFILE"), getenv("LOCALAPPDATA")
	if up != "" {
		for _, d := range [][]string{{".local", "bin"}, {".cargo", "bin"}, {".deno", "bin"}, {".bun", "bin"}} {
			dirs = append(dirs, filepath.Join(append([]string{up}, d...)...))
		}
		if m, _ := filepath.Glob(filepath.Join(up, "scoop", "apps", "*", "current")); m != nil {
			dirs = append(dirs, m...)
		}
	}
	if lad != "" {
		dirs = append(dirs, filepath.Join(lad, "Microsoft", "WinGet", "Links"))
		if m, _ := filepath.Glob(filepath.Join(lad, "Microsoft", "WinGet", "Packages", "*")); m != nil {
			dirs = append(dirs, m...)
		}
	}
	for exe := range tp.d.pythonExes {
		if env := pythonEnvFor(exe); !env.venv {
			dirs = append(dirs, filepath.Join(env.home, "Scripts"))
		}
	}
	var out []string
	for _, d := range dirs {
		if tp.inScope(d) {
			out = append(out, d)
		}
	}
	return out
}

type toolHit struct {
	exe string
	f   embedFindings
}

func (tp *appsContext) checkEmbeddedTools() {
	skipNames := map[string]bool{"node.exe": true, "java.exe": true, "javaw.exe": true, "python.exe": true, "pythonw.exe": true, "git.exe": true}
	seen := map[string]bool{}
	var exes []string
	for _, dir := range tp.toolDirs() {
		if tp.inWindowsDir(dir) {
			continue
		}
		ents, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range ents {
			n := lower(e.Name())
			if filepath.Ext(n) != ".exe" || skipNames[n] {
				continue
			}
			p := filepath.Join(dir, e.Name())
			if rp, err := filepath.EvalSymlinks(p); err == nil {
				p = rp
			}
			if seen[pathKey(p)] {
				continue
			}
			seen[pathKey(p)] = true
			fi, err := os.Stat(p)
			if err != nil || fi.Size() < 256<<10 || fi.Size() > 800<<20 || isCloudPlaceholder(fi) {
				continue
			}
			other := false
			for g := range tp.d.gitRoots {
				other = other || hasPathPrefix(p, g)
			}
			for ed := range tp.d.electronDirs {
				other = other || hasPathPrefix(p, ed)
			}
			if !other {
				exes = append(exes, p)
			}
		}
	}
	sort.Strings(exes)
	tp.c.stepStart(Tf("log.toolScan", len(exes)))
	var hits []toolHit
	for i, e := range tp.scanAll(exes, scanRoots) {
		if e.err == nil && e.f.HasRoots() {
			hits = append(hits, toolHit{exes[i], e.f})
		}
	}
	if tp.c.cancel() {
		return
	}
	tp.c.stepEnd(StOK, Tf("log.toolScanDone", len(hits)))
	// group programs of the same folder with the same findings (e.g. the tesseract tools)
	groups := map[string][]toolHit{}
	var keys []string
	for _, h := range hits {
		k := pathKey(filepath.Dir(h.exe)) + "|" + strings.Join(h.f.Roots, ",") + fmt.Sprint(h.f.Blocklist)
		if _, known := embeddedToolProducts[lower(filepath.Base(h.exe))]; known {
			k += "|" + h.exe // tools with their own fixed version stay separate
		}
		if groups[k] == nil {
			keys = append(keys, k)
		}
		groups[k] = append(groups[k], h)
	}
	for _, k := range keys {
		g := groups[k]
		var paths []string
		for _, h := range g {
			paths = append(paths, h.exe)
		}
		name := toolName(paths[0])
		if len(paths) > 1 {
			o := tp.ownerOf(paths[0])
			base := o.Name
			if base == "" {
				base = filepath.Base(filepath.Dir(paths[0]))
			}
			name = Tf("tools.group", base, len(paths))
		}
		tp.c.begin(tp.groupOf(name, paths[0]), "tool", name, filepath.Dir(paths[0]), func(it *Item) { tp.toolsOne(it, paths) })
	}
}

func toolName(exe string) string {
	fv, _ := getFileVersion(exe)
	if fv.Name != "" {
		return fv.Name
	}
	return strings.TrimSuffix(filepath.Base(exe), filepath.Ext(exe))
}

func toolVersion(exe string) string {
	fv, _ := getFileVersion(exe)
	if v := displayVersion(fv); v != "" {
		return trimZeros(v)
	}
	return goModuleVersion(exe)
}

func (tp *appsContext) toolsOne(it *Item, exes []string) {
	var f embedFindings
	var present []string
	for _, p := range exes {
		ff, err := tp.scan(p, scanRoots)
		if err != nil || !ff.HasRoots() {
			continue
		}
		if len(present) == 0 {
			f = ff
		}
		present = append(present, p)
	}
	if len(present) == 0 {
		tp.c.finish(it, result{Status: StOK, Summary: T("embed.sumGone"), Details: T("embed.sumGone")})
		return
	}
	first := present[0]
	r := result{Version: toolVersion(first), File: first, Exe: first, Folder: filepath.Dir(first)}
	o := tp.withOwner(&r, first)
	r.Search = searchFor(o.Name, toolName(first), len(present) > 1) // several programs: search for their software
	var links []string
	for _, p := range present {
		links = append(links, lk(p))
	}
	details := field(T("lbl.affectedRoots"), strings.Join(f.Roots, ", "))
	if len(links) > 1 {
		details = T("embed.programs") + "\n" + bullet(links) + "\n\n" + details
	}
	if product, ok := embeddedToolProducts[lower(filepath.Base(first))]; ok && len(present) == 1 && r.Version != "" {
		switch st, fixed := fixStatus(product, ParseVersion(r.Version)); st {
		case FixOK:
			r.Status, r.Summary, r.Details = StOK, T("app.sumOK"), joinParas(details, Tf("app.okDetails", toolName(first), r.Version))
			tp.c.finish(it, r)
			return
		case FixOutdated:
			r.Status, r.Summary, r.Details = StWarn, Tf("app.sumOutdated", fixed), details+"\n\n"+Tf("app.fix", toolName(first), fixed)
			tp.c.finish(it, r)
			return
		}
	}
	switch {
	case f.Blocklist:
		r.Status, r.Summary, r.Details = StOK, T("embed.sumBlocklist"), details+"\n\n"+T("embed.blocklistNote")
	case f.HasEmsignRoot():
		r.Status, r.Summary, r.Details = StWarn, T("embed.sumBad"), details+"\n\n"+T("embed.fix")
	default:
		r.Status, r.Summary, r.Details = StCaution, T("embed.sumAAA"), details+"\n\n"+T("embed.aaaNote")
	}
	tp.c.finish(it, r)
}

// ---------------------------------------------------------------------------
// Remaining certificate bundles

var (
	sitePackagesRe = regexp.MustCompile(`(?i)^(.*?)[\\/](?:lib[\\/](?:python[0-9.]+[\\/])?|python3\d+[\\/])site-packages[\\/]`)
	dotnetRootsRe  = regexp.MustCompile(`(?i)[\\/]dotnet[\\/]sdk[\\/][^\\/]+[\\/]trustedroots[\\/]`)
)

func (tp *appsContext) checkBundles() {
	all := dedupePaths(tp.d.bundles)
	tp.c.stepStart(Tf("log.bundles", len(all)))
	total, bad := 0, 0
	var badPaths, badAll, okAll []string
	for _, p := range all {
		if tp.c.cancel() {
			return
		}
		tp.c.activity(Tf("act.read", p))
		r, ok := tp.bc.checkFile(p)
		if !ok {
			continue
		}
		total++
		if r.Bad() {
			bad++
			badAll = append(badAll, p)
			if !tp.handled[pathKey(p)] {
				badPaths = append(badPaths, p)
			}
		} else {
			okAll = append(okAll, p)
		}
	}
	tp.c.activity("")
	st := StOK
	sum := Tf("bundle.sumAllOK", total)
	if bad > 0 {
		st, sum = StWarn, Tf("bundle.sumSome", bad, total)
	}
	tp.c.stepEnd(st, sum)
	const maxItems = 80
	for i, p := range badPaths {
		if i == maxItems {
			tp.c.add(GroupApps, "info", Tf("bundle.moreName", len(badPaths)-maxItems), result{Status: StWarn, Summary: T("bundle.sumBad"), Details: T("bundle.moreDetails")})
			break
		}
		p := p
		name := T("bundle.name")
		lp := strings.ReplaceAll(lower(p), "/", `\`)
		switch {
		case strings.Contains(lp, `pip\_vendor\certifi`):
			name = "pip"
		case strings.HasSuffix(lp, `certifi\cacert.pem`):
			name = "certifi"
		case dotnetRootsRe.MatchString(p):
			name = T("dotnet.name")
		}
		tp.c.begin(tp.groupOf(name, p), "bundle", name, filepath.Dir(p), func(it *Item) { tp.bundleOne(it, p) })
	}
	for i, p := range badPaths {
		if i >= maxItems {
			tp.c.line(StWarn, Tf("log.bundleBad", p))
		}
	}
	// a summary of all the bundles tested, with their paths; the outdated ones also
	// have their own line (or are listed with their software)
	sumSt := StOK
	if bad > 0 {
		sumSt, sum = StInfo, Tf("bundle.sumSomeInfo", bad, total)
	}
	paths := func(ps []string) string {
		const limit = 150
		var lines []string
		for i, p := range ps {
			if i == limit {
				lines = append(lines, "• "+Tf("bundle.listMore", len(ps)-limit))
				break
			}
			lines = append(lines, "• [["+p+"|"+p+"]]")
		}
		return strings.Join(lines, "\n")
	}
	details := T("bundle.summaryDetails")
	if len(badAll) > 0 {
		details = joinParas(details, Tf("bundle.badList", len(badAll))+"\n"+paths(badAll))
	}
	if len(okAll) > 0 {
		details = joinParas(details, Tf("bundle.okList", len(okAll))+"\n"+paths(okAll))
	}
	tp.c.addQuiet(GroupApps, "info", T("bundle.summaryName"), result{Status: sumSt, Summary: sum, Details: details})
}

func (tp *appsContext) bundleOne(it *Item, p string) {
	br, ok := tp.bc.checkFile(p)
	r := result{File: p}
	o := tp.withOwner(&r, p)
	lp := strings.ReplaceAll(lower(p), "/", `\`)
	env := ""
	if m := sitePackagesRe.FindStringSubmatch(p); m != nil {
		env = m[1]
	}
	py := filepath.Join(env, "Scripts", "python.exe")
	if !exists(py) {
		py = filepath.Join(env, "python.exe")
	}
	ownsPython := env != "" && exists(py)
	if ownsPython {
		r.Folder = filepath.Dir(py)
	} else if o.Loc != "" {
		r.Folder = o.Loc
	}
	details := ""
	if !ok {
		tp.c.finish(it, result{Status: StOK, Summary: T("bundle.sumGone"), Details: T("bundle.goneDetails")})
		return
	}
	if !br.Bad() {
		r.Status, r.Summary, r.Details = StOK, T("bundle.sumNowOK"), T("ok.bundle")
		tp.c.finish(it, r)
		return
	}
	details += "\n\n" + bundleProblem(br)
	var fix string
	bundled := o.Name != "" && (!ownsPython || bundledIn(o.Name, kwPython...))
	r.Bundled = bundled
	// a list of pip or certifi in a Python: harmless when that Python's ssl module
	// refuses the compromised certificates itself (see pythonFix)
	pyList := false
	var fx pyFix
	if ownsPython {
		fx = tp.pythonFix(pythonEnvFor(py))
	}
	switch {
	case dotnetRootsRe.MatchString(p):
		r.Status, r.Summary, r.Details = StInfo, T("dotnet.sum"), details+"\n\n"+T("dotnet.details")
		r.Search = ".NET SDK"
		r.ProductURL = "https://dotnet.microsoft.com/download"
		tp.c.finish(it, r)
		return
	case ownsPython && strings.Contains(lp, `pip\_vendor\certifi`):
		sp := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(p))))
		r.Version = distInfoVersion(sp, "pip")
		r.ProductURL = "https://pypi.org/project/pip/"
		fix = psHeader(p) + "\n" + pipCmd(py, "pip")
		if bundled {
			fix = joinParas(bundledAdvice(o), T("py.ifYouManage")+"\n"+pipCmd(py, "pip"))
		}
		pyList = true
	case ownsPython && strings.HasSuffix(lp, `certifi\cacert.pem`):
		sp := filepath.Dir(filepath.Dir(p))
		r.Version = distInfoVersion(sp, "certifi")
		r.ProductURL = "https://pypi.org/project/certifi/"
		fix = psHeader(p) + "\n" + pipCmd(py, "certifi")
		if bundled {
			fix = joinParas(bundledAdvice(o), T("py.ifYouManage")+"\n"+pipCmd(py, "certifi"))
		}
		pyList = true
	case strings.HasSuffix(lp, `library\ssl\cacert.pem`):
		fix = T("fix.header") + "\n" + cmd("conda update ca-certificates certifi")
	default:
		if strings.HasSuffix(lp, `certifi\cacert.pem`) {
			sp := filepath.Dir(filepath.Dir(p))
			r.Version = distInfoVersion(sp, "certifi")
		}
		if bundled {
			fix = joinParas(bundledAdvice(o), T("fix.directly")+"\n"+replaceBundleFix(p))
		} else {
			fix = joinParas(T("bundle.genericFix"), replaceBundleFix(p))
		}
	}
	r.Search = searchFor(o.Name, filepath.Base(filepath.Dir(p))+" "+filepath.Base(p), bundled)
	if pyList && fx.protected() {
		r.Status, r.Summary = StInfo, T("bundle.sumPySafe")
		r.Details = joinParas(strings.TrimSpace(details), pythonBlocksText(fx), T("py.updateAnyway")+"\n"+fix)
		tp.c.finish(it, r)
		return
	}
	r.Status, r.Summary, r.Details = StWarn, T("bundle.sumBad"), details+"\n\n"+fix
	tp.c.finish(it, r)
}

// pipCmd is a PowerShell command running pip with a given Python.
func pipCmd(python, pkg string) string {
	return cmd(fmt.Sprintf(`& "%s" -m pip install --upgrade %s`, python, pkg))
}

// needsAdmin: files under Program Files, ProgramData or Windows need an elevated prompt to change.
func needsAdmin(p string) bool {
	for _, v := range []string{"ProgramFiles", "ProgramFiles(x86)", "ProgramW6432", "ProgramData", "SystemRoot"} {
		if d := getenv(v); d != "" && hasPathPrefix(p, d) {
			return true
		}
	}
	return false
}

// psHeader introduces commands to run in PowerShell (as administrator if a file needs it).
func psHeader(files ...string) string {
	for _, f := range files {
		if needsAdmin(f) {
			return T("fix.psAdmin")
		}
	}
	return T("fix.ps")
}

// replaceBundleFix: keep a copy of an outdated bundle and replace it with Mozilla's
// current list, as published by curl (downloaded with Windows' own curl.exe).
func replaceBundleFix(files ...string) string {
	lines := []string{T("fix.replaceBundle"), psHeader(files...)}
	for _, f := range files {
		lines = append(lines, cmd(fmt.Sprintf(`Copy-Item -LiteralPath "%s" -Destination "%s.bak"`, f, f)),
			cmd(fmt.Sprintf(`curl.exe -fsSL -o "%s" https://curl.se/ca/cacert.pem`, f)))
	}
	return strings.Join(lines, "\n")
}

func joinParas(parts ...string) string {
	var out []string
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, "\n\n")
}

// ---------------------------------------------------------------------------
// Browsers and what is not covered

func (tp *appsContext) checkOthers() {
	wsl := tp.winDir != "" && exists(filepath.Join(tp.winDir, "System32", "wsl.exe")) && len(regSubkeys(hkcu, `SOFTWARE\Microsoft\Windows\CurrentVersion\Lxss`, view64)) > 0
	docker := false
	for g := range tp.d.electronDirs {
		docker = docker || strings.Contains(lower(g), "docker")
	}
	if wsl || docker {
		tp.c.add(GroupApps, "info", T("notcovered.name"), result{Status: StInfo, Summary: T("notcovered.sum"), Details: T("notcovered.details")})
	}
}

// googleURL builds a web search URL.
func googleURL(q string) string { return "https://www.google.com/search?q=" + url.QueryEscape(q) }
