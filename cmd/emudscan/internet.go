package main

// Programs that work mainly over the internet (browsers, messaging, mail,
// file sharing, sync, VPN, remote access…): they get their own section.

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Names (lower case, whole words) of popular programs that work mainly over the internet.
var internetNames = []string{
	// browsers
	"chrome", "chromium", "ungoogled-chromium", "edge", "firefox", "brave", "vivaldi", "opera", "tor browser",
	"librewolf", "waterfox", "floorp", "mullvad browser", "thorium", "zen browser", "arc",
	// messaging and video calls
	"signal", "discord", "slack", "teams", "telegram", "whatsapp", "element", "zoom", "skype", "mattermost",
	"rocket.chat", "threema", "viber", "beeper", "ferdium", "franz", "rambox", "webex", "jitsi", "wire",
	// mail
	"thunderbird", "betterbird", "mailspring", "em client", "postbox",
	// file sharing and downloads
	"qbittorrent", "transmission", "deluge", "tribler", "utorrent", "bittorrent", "jdownloader", "filezilla",
	"winscp", "yt-dlp", "free download manager",
	// cloud sync and backup
	"dropbox", "onedrive", "google drive", "nextcloud", "owncloud", "megasync", "mega", "pcloud", "syncthing",
	"resilio", "proton drive", "icloud",
	// VPN and remote access
	"protonvpn", "proton vpn", "mullvad", "nordvpn", "expressvpn", "wireguard", "openvpn", "tailscale",
	"anydesk", "teamviewer", "rustdesk", "parsec",
	// password managers and streaming
	"bitwarden", "1password", "dashlane", "keepassxc", "spotify", "steam",
}

var internetRe = func() *regexp.Regexp {
	var q []string
	for _, n := range internetNames {
		q = append(q, regexp.QuoteMeta(n))
	}
	return regexp.MustCompile(`(?i)(^|[^a-z0-9])(` + strings.Join(q, "|") + `)($|[^a-z0-9])`)
}()

func isInternetName(name string) bool { return name != "" && internetRe.MatchString(name) }

type netProgram struct {
	name, version, url, dir, exe string
}

// loadInternetPrograms keeps the installed programs (Apps & features) that work over the internet.
func (tp *appsContext) loadInternetPrograms(entries []uninstallEntry) {
	generic := tp.genericDirs()
	seen := map[string]bool{}
	for _, e := range entries {
		if !isInternetName(e.name) || isBrowserWebApp(e) {
			continue
		}
		dir, exe := "", ""
		for _, d := range e.dirs {
			if tp.acceptableOwnerDir(d, generic) {
				dir = filepath.Clean(d)
				break
			}
		}
		if dir == "" || !tp.inScope(dir) || !isDir(dir) || seen[pathKey(dir)] {
			continue
		}
		seen[pathKey(dir)] = true
		if e.icon != "" && exists(e.icon) && strings.EqualFold(filepath.Ext(e.icon), ".exe") {
			exe = e.icon
		}
		tp.netPrograms = append(tp.netPrograms, netProgram{cleanProgramName(e.name, e.version), e.version, e.url, dir, exe})
		// browsers installed in folders that the scan skips are still found
		for _, b := range browserExeNames {
			if p := filepath.Join(dir, b); exists(p) {
				tp.d.browsers[p] = true
			}
		}
	}
}

// isBrowserWebApp: web sites installed as apps by a browser (Chrome Remote Desktop…) are
// registered in Apps & features but live in the browser's profile.
func isBrowserWebApp(e uninstallEntry) bool {
	for _, d := range append([]string{e.location, e.icon}, e.dirs...) {
		ld := lower(d)
		if strings.Contains(ld, `\web applications\`) || strings.Contains(ld, `\user data\`) || strings.Contains(ld, `\_crx_`) {
			return true
		}
	}
	return false
}

// groupOf decides in which section an element is listed.
func (tp *appsContext) groupOf(name, path string) Group {
	if isInternetName(name) {
		return GroupNet
	}
	if path != "" {
		if isInternetName(tp.ownerOf(path).Name) {
			return GroupNet
		}
		for _, p := range tp.netPrograms {
			if hasPathPrefix(path, p.dir) {
				return GroupNet
			}
		}
	}
	return GroupApps
}

// ---------------------------------------------------------------------------
// Browsers

var browserExeNames = []string{"chrome.exe", "msedge.exe", "brave.exe", "vivaldi.exe", "opera.exe", "chromium.exe",
	"thorium.exe", "firefox.exe", "librewolf.exe", "waterfox.exe", "floorp.exe", "mullvadbrowser.exe",
	"thunderbird.exe", "betterbird.exe", "zen.exe"}

var geckoExes = map[string]bool{"firefox.exe": true, "librewolf.exe": true, "waterfox.exe": true, "floorp.exe": true,
	"mullvadbrowser.exe": true, "thunderbird.exe": true, "betterbird.exe": true, "zen.exe": true}

var versionDirRe = regexp.MustCompile(`^\d+(\.\d+)+$`)

// isBrowserCopy filters out helper copies (Edge's EdgeCore, Opera's versioned folder, installers).
func isBrowserCopy(p string) bool {
	lp := strings.ReplaceAll(lower(p), "/", `\`)
	for _, bad := range []string{`\edgecore\`, `\edgewebview\`, `\edgeupdate\`, `\temp\`, `\installer\`, `\package cache\`, `\setup\`, `\scoop\shims\`} {
		if strings.Contains(lp, bad) {
			return true
		}
	}
	// Scoop-style shims: a small launcher next to a .shim file that points to the real program
	if exists(strings.TrimSuffix(p, filepath.Ext(p)) + ".shim") {
		return true
	}
	// Opera keeps a launcher next to its versioned folder: opera.exe and 114.0.5282.102\opera.exe.
	// A versioned folder alone (Scoop: apps\firefox\132.0\firefox.exe) is the browser itself.
	dir := filepath.Dir(p)
	return versionDirRe.MatchString(filepath.Base(dir)) && exists(filepath.Join(filepath.Dir(dir), filepath.Base(p)))
}

// registeredBrowsers returns the browsers registered with Windows (Default apps),
// so that they are found even on drives that are not scanned.
func registeredBrowsers() []string {
	var out []string
	for _, root := range []regRoot{hklm, hkcu} {
		for _, view := range []uint32{view64, view32} {
			base := `SOFTWARE\Clients\StartMenuInternet`
			for _, k := range regSubkeys(root, base, view) {
				cmd, _ := regString(root, base+`\`+k+`\shell\open\command`, "", view)
				if p := exePathFrom(cmd); p != "" && exists(p) {
					out = append(out, p)
				}
			}
			for _, k := range regSubkeys(root, `SOFTWARE\Clients\Mail`, view) {
				cmd, _ := regString(root, `SOFTWARE\Clients\Mail\`+k+`\shell\open\command`, "", view)
				if p := exePathFrom(cmd); p != "" && exists(p) && isBrowserExe[lower(filepath.Base(p))] {
					out = append(out, p)
				}
			}
		}
	}
	return out
}

func browserName(exe string, fv fileVersion) string {
	lp := lower(exe)
	switch {
	case strings.Contains(lp, "tor browser"):
		return "Tor Browser"
	case strings.Contains(lp, "ungoogled"):
		return "ungoogled-chromium"
	case strings.Contains(lp, "mullvad browser"):
		return "Mullvad Browser"
	case fv.Name != "":
		return fv.Name
	}
	n := strings.TrimSuffix(filepath.Base(exe), filepath.Ext(exe))
	if n != "" {
		n = strings.ToUpper(n[:1]) + n[1:]
	}
	return n
}

// officialBrowser: builds that receive their vendor's online block list automatically.
func officialBrowser(exe string, fv fileVersion) (bool, string) {
	co, name, base, lp := lower(fv.Company), lower(fv.Name), lower(filepath.Base(exe)), lower(exe)
	if strings.Contains(lp, "tor browser") || strings.Contains(lp, "ungoogled") {
		return false, ""
	}
	switch {
	case base == "chrome.exe" && strings.Contains(co, "google") && strings.Contains(name, "chrome"):
		return true, "crlset"
	case base == "msedge.exe" && strings.Contains(co, "microsoft"):
		return true, "edge"
	case base == "brave.exe" && strings.Contains(co, "brave"):
		return true, "crlset"
	case base == "vivaldi.exe" && strings.Contains(co, "vivaldi"):
		return true, "crlset"
	case base == "opera.exe" && strings.Contains(co, "opera"):
		return true, "crlset"
	case (base == "firefox.exe" || base == "thunderbird.exe") && strings.Contains(co, "mozilla"):
		return true, "onecrl"
	}
	return false, ""
}

func browserURL(name string) string {
	n := lower(name)
	for _, u := range []struct{ k, url string }{
		{"tor browser", "https://www.torproject.org/download/"},
		{"ungoogled", "https://github.com/ungoogled-software/ungoogled-chromium-windows/releases"},
		{"mullvad", "https://mullvad.net/browser"},
		{"chrome", "https://www.google.com/chrome/"},
		{"edge", "https://www.microsoft.com/edge"},
		{"thunderbird", "https://www.thunderbird.net/"},
		{"librewolf", "https://librewolf.net/installation/windows/"},
		{"waterfox", "https://www.waterfox.net/"},
		{"floorp", "https://floorp.app/"},
		{"firefox", "https://www.mozilla.org/firefox/"},
		{"brave", "https://brave.com/download/"},
		{"vivaldi", "https://vivaldi.com/download/"},
		{"opera", "https://www.opera.com/download"},
		{"chromium", "https://www.chromium.org/getting-involved/download-chromium/"},
	} {
		if strings.Contains(n, u.k) {
			return u.url
		}
	}
	return ""
}

// rootStoreLib finds the library that holds the browser's built-in root list.
func rootStoreLib(dir string, gecko bool) string {
	if gecko {
		for _, n := range []string{"nssckbi.dll", "xul.dll"} {
			if p := filepath.Join(dir, n); exists(p) {
				return p
			}
		}
		return ""
	}
	if p := filepath.Join(dir, "chrome.dll"); exists(p) {
		return p
	}
	m, _ := filepath.Glob(filepath.Join(dir, "*", "chrome.dll"))
	sort.Slice(m, func(i, j int) bool {
		return ParseVersion(filepath.Base(filepath.Dir(m[i]))).Cmp(ParseVersion(filepath.Base(filepath.Dir(m[j])))) > 0
	})
	if len(m) > 0 {
		return m[0]
	}
	return ""
}

func (tp *appsContext) checkBrowsers() {
	for _, p := range registeredBrowsers() {
		if isBrowserExe[lower(filepath.Base(p))] && tp.inScope(p) {
			tp.d.browsers[p] = true
		}
	}
	seen := map[string]bool{}
	for _, exe := range dedupePaths(tp.d.browsers) {
		if isBrowserCopy(exe) || seen[pathKey(filepath.Dir(exe))] {
			continue
		}
		seen[pathKey(filepath.Dir(exe))] = true
		fv, _ := getFileVersion(exe)
		exe, name := exe, browserName(exe, fv)
		tp.c.begin(GroupNet, "browser", name, filepath.Dir(exe), func(it *Item) { tp.browserOne(it, exe) })
	}
}

func (tp *appsContext) browserOne(it *Item, exe string) {
	fv, _ := getFileVersion(exe)
	name := browserName(exe, fv)
	dir := filepath.Dir(exe)
	gecko := geckoExes[lower(filepath.Base(exe))]
	r := result{Name: name, Version: trimZeros(displayVersion(fv)), File: exe, Exe: exe, Folder: dir, ProductURL: browserURL(name), Search: name}
	tp.withOwner(&r, exe)
	finish := func() {
		if r.Status != StOK && r.Status != StInfo { // the user can check it in the browser
			r.Details = joinParas(r.Details, browserTests(name, exe))
		}
		tp.c.finish(it, r)
	}
	if ok, list := officialBrowser(exe, fv); ok {
		r.Status, r.Summary, r.Details = StOK, T("browser.official"), T("browser.official."+list)
		if list == "crlset" { // the block list it has downloaded, when it can be read
			if cs, found := browserCRLSet(exe); found && tp.inScope(cs.path) {
				r.File = cs.path
				if cs.blocked == cs.total {
					r.Details = joinParas(r.Details, Tf("browser.crlsetHas", name, cs.describe()))
				} else {
					r.Status, r.Summary = StCaution, T("browser.crlsetOld")
					r.Details = Tf("browser.crlsetOldDetails", name, cs.describe(), name)
				}
			}
		}
		finish()
		return
	}
	if gecko && tp.geckoBlocked(&r, name, dir) {
		finish()
		return
	}
	lib := rootStoreLib(dir, gecko)
	if lib == "" && gecko {
		tp.geckoNotBlocked(&r, name, dir)
		finish()
		return
	}
	if lib == "" {
		r.Status, r.Summary, r.Details = StCaution, T("browser.noStore"), T("browser.noStoreDetails")
		finish()
		return
	}
	tp.c.activity(Tf("act.read", lib))
	f, err := tp.scan(lib, scanRoots)
	tp.c.activity("")
	r.File = lib
	switch {
	case err != nil:
		r.Status, r.Summary, r.Details = StCaution, T("browser.noStore"), err.Error()
	case f.Blocklist:
		r.Status, r.Summary, r.Details = StOK, T("embed.sumBlocklist"), T("embed.blocklistNote")
	case !f.HasEmsignRoot():
		r.Status, r.Summary, r.Details = StOK, T("browser.updated"), T("browser.updatedDetails")
	case gecko:
		tp.geckoNotBlocked(&r, name, dir)
	default:
		r.Status, r.Summary, r.Details = StWarn, T("browser.noCRLSet"), Tf("browser.chromiumDetails", name, name)
		// a Chromium build that does download Chrome's block list after all
		if cs, found := browserCRLSet(exe); found && tp.inScope(cs.path) && cs.blocked == cs.total {
			r.File = cs.path
			r.Status, r.Summary, r.Details = StOK, T("browser.official"), Tf("browser.crlsetChromium", name, cs.describe())
		}
	}
	finish()
}

// The advisory's test pages: a site whose certificate chains to a compromised
// intermediate directly, and one through the Sectigo cross-signed copy.
var browserTestPages = []struct{ url, key string }{
	{"https://emudhra.we-have-a-problem.com/", "browser.testPage"},
	{"https://dv-g3b-emudhra.we-have-a-problem.com/", "browser.testDirect"},
	{"https://dv-g3b-aaa-emudhra.we-have-a-problem.com/", "browser.testCross"},
	{"https://dv-g3b-noct-emudhra.we-have-a-problem.com/", "browser.testNoCT"},
	{"https://dv-g3b-aaa-noct-emudhra.we-have-a-problem.com/", "browser.testNoCTCross"},
}

// browserTests: links that open the test pages in the browser itself.
func browserTests(name, exe string) string {
	lines := []string{Tf("browser.test", name)}
	for _, p := range browserTestPages {
		lines = append(lines, "• [["+p.url+"|"+openInPrefix+exe+">"+p.url+"]] ("+T(p.key)+")")
	}
	return strings.Join(lines, "\n") + "\n\n" + Tf("browser.testNoCTNote", name)
}

// copierVerdict: software that loads the Windows roots into its own OpenSSL library
// is affected like Python's ssl module, through the roots present on this PC.
func (tp *appsContext) copierVerdict(r *result, name, file string, f embedFindings) {
	why := Tf("net.copiesDetails", name, filepath.Base(file))
	if f.Markers["libtorrent"] {
		why += "\n\n" + T("net.copiesLibtorrent")
	}
	if tp.pyStoreRisk > 0 {
		r.Status, r.Summary = StCaution, T("net.copies")
		r.Details = why + "\n\n" + Tf("net.copiesRisk", tp.pyStoreRisk, tlsChainsTotal, refWinCopy()) + "\n\n" + Tf("net.copiesFix", name)
		return
	}
	r.Status, r.Summary = StOK, T("net.copiesOK")
	r.Details = why + "\n\n" + Tf("net.copiesNoRisk", refWinCopy())
}

// geckoBlocked: a Firefox-based program that carries the updated list of blocked
// certificates (OneCRL), read from its omni.ja, or that is based on a Firefox
// release that carries it, is protected even if it never downloads the list.
func (tp *appsContext) geckoBlocked(r *result, name, dir string) bool {
	tp.c.activity(Tf("act.read", filepath.Join(dir, "omni.ja")))
	blocked, total, found := geckoOneCRL(dir)
	tp.c.activity("")
	gv := geckoVersion(dir)
	switch {
	case found && total > 0 && blocked == total:
		if strings.Contains(lower(name), "librewolf") {
			for _, v := range []string{r.Version, r.OwnerVer} {
				if reads, ok := librewolfReadsBundled(v); ok && !reads {
					r.Status, r.Summary, r.Details = StCaution, T("gecko.sumNotRead"), Tf("gecko.notRead", name, name)
					return true
				}
			}
		}
		r.Status, r.Summary, r.Details = StOK, T("gecko.sumBundled"), Tf("gecko.bundled", name)
		r.File = "" // nothing outdated
		return true
	case !found && gv != "" && geckoVersionFixed(ParseVersion(gv)):
		r.Status, r.Summary, r.Details = StOK, Tf("gecko.sumVersion", gv), Tf("gecko.versionOK", name, gv)
		r.File = ""
		return true
	}
	return false
}

// geckoNotBlocked: a Firefox-based program whose list of blocked certificates
// predates the compromise (or could not be read).
func (tp *appsContext) geckoNotBlocked(r *result, name, dir string) {
	_, _, found := geckoOneCRL(dir)
	gv := geckoVersion(dir)
	fix := Tf("gecko.fix", name)
	var why string
	switch {
	case found && gv != "" && geckoVersionFixed(ParseVersion(gv)):
		// based on a release that carries the list, but this copy does not have it
		why, fix = Tf("gecko.listMissing", name, gv), Tf("gecko.fixMissing", name)
	case found:
		why = Tf("gecko.listOld", name, name)
	case gv != "":
		why = Tf("gecko.versionOld", name, gv)
	default:
		why = Tf("gecko.unknown", name)
	}
	r.Status, r.Summary, r.Details = StCaution, T("browser.dependsOneCRL"), why+"\n\n"+fix
}

// ---------------------------------------------------------------------------
// Internet programs with nothing else to report: read their files for a built-in root list

func (tp *appsContext) checkInternetFolders() {
	covered := func(dir string) bool {
		for _, it := range tp.c.snapshot() {
			for _, p := range []string{it.File, it.Folder} {
				if p != "" && (hasPathPrefix(p, dir) || hasPathPrefix(dir, p) && it.Kind != "bundle") {
					return true
				}
			}
		}
		return false
	}
	for _, p := range tp.netPrograms {
		if tp.c.cancel() {
			return
		}
		if covered(p.dir) {
			continue
		}
		first := p
		tp.c.begin(GroupNet, "netapp", p.name, p.dir, func(it *Item) { tp.netFolderOne(it, tp.currentNetProgram(first)) })
	}
}

// currentNetProgram is p as it is now in Apps & features (a new version, say).
func (tp *appsContext) currentNetProgram(p netProgram) netProgram {
	for _, x := range tp.netPrograms {
		if pathKey(x.dir) == pathKey(p.dir) {
			return x
		}
	}
	return p
}

func programFiles(dir string, maxDepth int) []string {
	var out []string
	var total int64
	base := strings.Count(filepath.Clean(dir), string(filepath.Separator))
	filepath.WalkDir(dir, func(p string, e os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if e.IsDir() {
			if strings.Count(p, string(filepath.Separator))-base > maxDepth {
				return filepath.SkipDir
			}
			return nil
		}
		ext := lower(filepath.Ext(p))
		if ext != ".exe" && ext != ".dll" {
			return nil
		}
		fi, err := e.Info()
		if err != nil || fi.Size() < 256<<10 || fi.Size() > 600<<20 || isCloudPlaceholder(fi) {
			return nil
		}
		if len(out) >= 400 || total > 3<<30 {
			return filepath.SkipAll
		}
		total += fi.Size()
		out = append(out, p)
		return nil
	})
	return out
}

func (tp *appsContext) netFolderOne(it *Item, p netProgram) {
	r := result{Version: p.version, Folder: p.dir, File: p.exe, Exe: p.exe, Owner: p.name, OwnerURL: p.url, Search: p.name}
	var hit, copier embedFindings
	var hitFile, copierFile string
	var blocklist bool
	files := programFiles(p.dir, 3)
	scans := tp.scanAll(files, scanMarkers)
	if tp.c.cancel() {
		return
	}
	for i, f := range files {
		ff, err := scans[i].f, scans[i].err
		if err != nil {
			continue
		}
		if copierFile == "" && !ff.HasRoots() && copiesWindowsRoots(f, ff) {
			copier, copierFile = ff, f
		}
		blocklist = blocklist || ff.Blocklist
		if ff.HasRoots() && (hitFile == "" || ff.HasEmsignRoot() && !hit.HasEmsignRoot()) {
			hit, hitFile = ff, f
		}
	}
	tp.c.activity("")
	switch {
	case hitFile == "" && copierFile != "":
		r.File = copierFile
		tp.copierVerdict(&r, p.name, copierFile, copier)
	case hitFile == "":
		r.Status, r.Summary, r.Details = StOK, T("net.noOwnList"), T("net.noOwnListDetails")
	case blocklist:
		r.File = hitFile
		r.Status, r.Summary, r.Details = StOK, T("embed.sumBlocklist"), field(T("lbl.affectedRoots"), strings.Join(hit.Roots, ", "))+"\n\n"+T("embed.blocklistNote")
	case hit.HasEmsignRoot():
		r.File = hitFile
		r.Status, r.Summary, r.Details = StWarn, T("embed.sumBad"), field(T("lbl.affectedRoots"), strings.Join(hit.Roots, ", "))+"\n\n"+Tf("net.fix", p.name)
	default:
		r.File = hitFile
		r.Status, r.Summary, r.Details = StCaution, T("embed.sumAAA"), field(T("lbl.affectedRoots"), strings.Join(hit.Roots, ", "))+"\n\n"+T("embed.aaaNote")
	}
	tp.c.finish(it, r)
}
