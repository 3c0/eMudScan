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
	if versionDirRe.MatchString(filepath.Base(dir)) && exists(filepath.Join(filepath.Dir(dir), filepath.Base(p))) {
		return true
	}
	return isStagedUpdate(p)
}

// stagedDir is the folder of an installation of Firefox, Thunderbird or a browser
// based on them where their updater (updater.exe) puts a downloaded update: at the
// next start of the program, it replaces the installation with that folder.
const stagedDir = "updated"

// isStagedUpdate: the program of such an update, which is not an installation of
// its own.
func isStagedUpdate(p string) bool {
	dir := filepath.Dir(p)
	if !geckoExes[lower(filepath.Base(p))] || !strings.EqualFold(filepath.Base(dir), stagedDir) {
		return false
	}
	install := filepath.Dir(dir)
	return exists(filepath.Join(install, filepath.Base(p))) && exists(filepath.Join(install, "updater.exe"))
}

// stagedUpdate is the program of the update that waits for the next start of the
// program exe, if any.
func stagedUpdate(exe string) string {
	p := filepath.Join(filepath.Dir(exe), stagedDir, filepath.Base(exe))
	if exists(p) && isStagedUpdate(p) {
		return p
	}
	return ""
}

// stagedNote tells that the update to version staged waits for the next start of
// the browser name, whose version is current; none when there is nothing new.
func stagedNote(name, current, staged string) string {
	if staged == "" || staged == current {
		return ""
	}
	return Tf("browser.staged", staged, name)
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
	lib := "chrome.dll"
	if exists(filepath.Join(dir, "msedge.exe")) {
		lib = "msedge.dll" // Microsoft Edge
	}
	if p := filepath.Join(dir, lib); exists(p) {
		return p
	}
	m, _ := filepath.Glob(filepath.Join(dir, "*", lib))
	sort.Slice(m, func(i, j int) bool {
		return ParseVersion(filepath.Base(filepath.Dir(m[i]))).Cmp(ParseVersion(filepath.Base(filepath.Dir(m[j])))) > 0
	})
	if len(m) > 0 {
		return m[0]
	}
	return ""
}

func (tp *appsContext) checkBrowsers() {
	tp.regBrowsers = map[string]bool{}
	for _, p := range registeredBrowsers() {
		tp.regBrowsers[pathKey(p)] = true
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
		if p := stagedUpdate(exe); p != "" {
			sfv, _ := getFileVersion(p)
			r.Details = joinParas(r.Details, stagedNote(name, r.Version, trimZeros(displayVersion(sfv))))
		}
		if r.Status != StOK && r.Status != StInfo { // the user can check it in the browser
			r.Details = joinParas(r.Details, browserTests(name, exe, tp.startableBrowser(exe)))
		}
		tp.c.finish(it, r)
	}
	if ok, list := officialBrowser(exe, fv); ok {
		tp.officialVerdict(&r, name, exe, list)
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
	tp.c.activity(Tf("act.read", shownPath(lib)))
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

// officialVerdict judges a browser that receives its vendor's block list. For a
// Chromium browser, Microsoft Edge included, the copy it has downloaded tells, when
// it can be read: if that copy is older than the block of the compromised
// intermediates, the block list built into recent versions protects it all the same.
func (tp *appsContext) officialVerdict(r *result, name, exe, list string) {
	r.Status, r.Summary, r.Details = StOK, T("browser.official"), T("browser.official."+list)
	if list != "crlset" && list != "edge" {
		return
	}
	cs, found := browserCRLSet(exe)
	found = found && tp.inScope(cs.path)
	if found && cs.blocked == cs.total {
		r.File = cs.path
		r.Details = joinParas(r.Details, Tf("browser.crlsetHas", name, cs.describe()))
		return
	}
	if lib := rootStoreLib(filepath.Dir(exe), false); lib != "" && found {
		tp.c.activity(Tf("act.read", shownPath(lib)))
		f, err := tp.scan(lib, scanRoots)
		tp.c.activity("")
		if err == nil && f.Blocklist {
			r.File = lib
			r.Status, r.Summary, r.Details = StOK, T("embed.sumBlocklist"), T("embed.blocklistNote")
			return
		}
	}
	if found {
		r.File = cs.path
		r.Status, r.Summary = StCaution, T("browser.crlsetOld")
		key := "browser.crlsetOldDetails"
		if list == "edge" {
			key = "browser.crlsetOldDetailsEdge" // Microsoft's list, not Chrome's
		}
		r.Details = Tf(key, name, cs.describe(), name)
	}
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

// browserTests: links that open the test pages in the browser itself when start,
// otherwise the pages' addresses, to open in it.
func browserTests(name, exe string, start bool) string {
	lines := []string{Tf("browser.testCopy", name)}
	if start {
		lines[0] = Tf("browser.test", name)
		allowBrowser(exe)
	}
	for _, p := range browserTestPages {
		link := p.url
		if start {
			link = "[[" + p.url + "|" + openInPrefix + exe + ">" + p.url + "]]"
		}
		lines = append(lines, "• "+link+" ("+T(p.key)+")")
	}
	return strings.Join(lines, "\n") + "\n\n" + Tf("browser.testNoCTNote", name)
}

// startableBrowser: a browser that the links of its details may start: one that is
// registered in Windows, or one in a folder that only administrators or the user can
// change (Program Files, Windows, the user's own folder). A program elsewhere, such
// as in ProgramData, in a folder at the root of a drive or in another user's folder,
// may have been put there by someone else under a browser's name.
func (tp *appsContext) startableBrowser(exe string) bool {
	if !drivePath(exe) || !linkable(exe) || strings.ContainsRune(exe, '>') {
		return false
	}
	if tp.regBrowsers[pathKey(exe)] {
		return true
	}
	for _, v := range []string{"ProgramFiles", "ProgramFiles(x86)", "ProgramW6432", "SystemRoot", "USERPROFILE"} {
		if d := getenv(v); d != "" && hasPathPrefix(exe, d) {
			return true
		}
	}
	return false
}

// copierVerdict: software that loads the Windows roots into its own OpenSSL library
// is affected like Python's ssl module, through the roots present on this PC.
func (tp *appsContext) copierVerdict(r *result, name, file string, f embedFindings) {
	why := Tf("net.copiesDetails", name, plain(filepath.Base(file)))
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
	tp.c.activity(Tf("act.read", shownPath(filepath.Join(dir, "omni.ja"))))
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

// componentKinds: elements that report one file of a program (yt-dlp.exe, the Flutter
// engine…), not its whole folder. The folder of an internet program that holds one is
// still checked, without that file.
var componentKinds = map[string]bool{"ytdlp": true, "flutter": true}

// componentFiles: the files reported by component elements (pathKey).
func (tp *appsContext) componentFiles() map[string]bool {
	out := map[string]bool{}
	for _, it := range tp.c.snapshot() {
		if componentKinds[it.Kind] && it.File != "" {
			out[pathKey(it.File)] = true
		}
	}
	return out
}

// netFiles: the files of an internet program to read, without those that component
// elements report.
func (tp *appsContext) netFiles(dir string) []string {
	skip := tp.componentFiles()
	var out []string
	for _, f := range newestVersionFiles(dir, programFiles(dir, 3)) {
		if !skip[pathKey(f)] {
			out = append(out, f)
		}
	}
	return out
}

// appVersionDirRe: a folder named after a full version (156.0.4314.58).
var appVersionDirRe = regexp.MustCompile(`^\d+(\.\d+){2,}$`)

// newestVersionFiles keeps, of files of the program at dir, those of its newest
// version when it keeps each version in a folder named after it (Microsoft Edge
// WebView2 Runtime: Application\156.0.4314.58\…): the folder of the previous version
// stays until the program restarts, and is no longer used.
func newestVersionFiles(dir string, files []string) []string {
	type place struct {
		parent string
		ver    Version
	}
	where := func(f string) (place, bool) {
		rel, err := filepath.Rel(dir, f)
		if err != nil {
			return place{}, false
		}
		parts := strings.Split(rel, string(filepath.Separator))
		for i, p := range parts[:len(parts)-1] {
			if appVersionDirRe.MatchString(p) {
				return place{pathKey(filepath.Join(append([]string{dir}, parts[:i]...)...)), ParseVersion(p)}, true
			}
		}
		return place{}, false
	}
	newest := map[string]Version{}
	for _, f := range files {
		if pl, ok := where(f); ok && (newest[pl.parent] == nil || pl.ver.Cmp(newest[pl.parent]) > 0) {
			newest[pl.parent] = pl.ver
		}
	}
	var out []string
	for _, f := range files {
		if pl, ok := where(f); !ok || pl.ver.Cmp(newest[pl.parent]) == 0 {
			out = append(out, f)
		}
	}
	return out
}

func (tp *appsContext) checkInternetFolders() {
	covered := func(dir string) bool {
		for _, it := range tp.c.snapshot() {
			if componentKinds[it.Kind] {
				continue
			}
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
		if covered(p.dir) || tp.componentsOnly(p) {
			continue
		}
		first := p
		tp.c.begin(GroupNet, "netapp", p.name, p.dir, func(it *Item) { tp.netFolderOne(it, tp.currentNetProgram(first)) })
	}
}

// componentsOnly: the folder of internet program p holds files that component
// elements report, and nothing else to report: no affected root (other than a
// harmless packed bundle) and no copy of the Windows root store. The component
// elements then speak for the program.
func (tp *appsContext) componentsOnly(p netProgram) bool {
	skip := tp.componentFiles()
	has := false
	for k := range skip {
		has = has || hasPathPrefix(k, pathKey(p.dir))
	}
	if !has {
		return false
	}
	files := tp.netFiles(p.dir)
	for i, s := range tp.scanAll(files, scanMarkers) {
		if s.err != nil {
			continue
		}
		if s.f.HasRoots() && !s.f.packedHarmless() || !s.f.HasRoots() && copiesWindowsRoots(files[i], s.f) {
			return false
		}
	}
	return true
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

// isWebView2Runtime: the Microsoft Edge WebView2 Runtime installed in Windows, which
// software uses to display web content unless it ships its own WebView2.
func isWebView2Runtime(p netProgram) bool {
	return strings.Contains(lower(p.name), "webview2 runtime") ||
		strings.Contains(strings.ReplaceAll(lower(p.dir)+`\`, "/", `\`), `\microsoft\edgewebview\`)
}

func (tp *appsContext) netFolderOne(it *Item, p netProgram) {
	r := result{Version: p.version, Folder: p.dir, File: p.exe, Exe: p.exe, Owner: p.name, OwnerURL: p.url, Search: p.name}
	var hit, copier embedFindings
	var hitFile, copierFile string
	var blocklist bool
	files := tp.netFiles(p.dir)
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
		if ff.HasRoots() && !ff.packedHarmless() && (hitFile == "" || ff.HasEmsignRoot() && !hit.HasEmsignRoot()) {
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
		r.Status, r.Summary, r.Details = StOK, T("embed.sumBlocklist"), rootsField(hit)+"\n\n"+T("embed.blocklistNote")
		if isWebView2Runtime(p) {
			r.Details = joinParas(r.Details, T("webview2.runtimeOK"))
		}
	case hit.HasEmsignRoot():
		r.File = hitFile
		r.Status, r.Summary, r.Details = StWarn, T("embed.sumBad"), rootsField(hit)+"\n\n"+Tf("net.fix", p.name)
		if isWebView2Runtime(p) {
			r.Details = rootsField(hit) + "\n\n" + T("webview2.runtimeOld")
		}
	default:
		r.File = hitFile
		r.Status, r.Summary, r.Details = StCaution, T("embed.sumAAA"), rootsField(hit)+"\n\n"+T("embed.aaaNote")
	}
	tp.c.finish(it, r)
}
