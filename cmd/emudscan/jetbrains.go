package main

// JetBrains, as its bulletin says (jetbrainsBlog):
//   - the IDEs, the Toolbox App and Gateway check certificates with the trust store of
//     the Java runtime they include (JetBrains Runtime), which the Java check judges
//     by its contents; the fixed versions of each IDE also fix its built-in browser
//     (JCEF), which the IDE check judges by the version of the IDE;
//   - jetbrainsd, the background service of the Toolbox App and of the IDEs, checks its
//     connections with its own built-in list of trusted certificates, which updating
//     Windows or the IDEs' Java does not change. Version 0.9.9728 adds the fingerprints
//     of the 16 compromised certificates to it, as text, which the scan finds
//     (embedFindings.Blocklist). Each user has a single copy, installed in
//     %LOCALAPPDATA%\JetBrains\Daemon\bundles\current by the Toolbox App or an IDE when
//     it starts; the newest version replaces the older ones.

import (
	"debug/pe"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const jetbrainsBlog = "https://blog.jetbrains.com/security/2026/10/compromised-emudhra-certificates-ides/"

// isJetbrainsdExe: the program of an installed jetbrainsd. The archives that the
// Toolbox App and the IDEs carry are not programs, and a copy unpacked elsewhere does
// not run.
func isJetbrainsdExe(p string) bool {
	return strings.HasSuffix(strings.ReplaceAll(lower(p), `\`, "/"), "/jetbrains/daemon/bundles/current/jetbrainsd/bin/jetbrainsd.exe")
}

func (tp *appsContext) checkJetbrainsd() {
	var cands []string
	for p := range tp.d.jetbrainsds {
		cands = append(cands, p)
	}
	if lad := getenv("LOCALAPPDATA"); lad != "" {
		cands = append(cands, filepath.Join(lad, "JetBrains", "Daemon", "bundles", "current", "jetbrainsd", "bin", "jetbrainsd.exe"))
	}
	seen := map[string]bool{}
	var exes []string
	for _, p := range cands {
		if seen[pathKey(p)] || !tp.inScope(p) {
			continue
		}
		seen[pathKey(p)] = true
		if fi, err := os.Stat(p); err != nil || fi.IsDir() || isCloudPlaceholder(fi) {
			continue
		}
		exes = append(exes, p)
	}
	sort.Strings(exes)
	for _, p := range exes {
		p := p
		tp.c.begin(tp.groupOf("jetbrainsd", p), "jetbrainsd", "jetbrainsd", filepath.Dir(p), func(it *Item) { tp.jetbrainsdOne(it, p) })
	}
}

// jetbrainsdVersion is written in bin\VERSION, and in the program's version
// information.
func jetbrainsdVersion(exe string) string {
	if v := firstLine(filepath.Join(filepath.Dir(exe), "VERSION")); len(ParseVersion(v)) >= 3 {
		return v
	}
	if fv, ok := getFileVersion(exe); ok && len(ParseVersion(fv.Product)) >= 3 {
		return fv.Product
	}
	return ""
}

func (tp *appsContext) jetbrainsdOne(it *Item, exe string) {
	ver := jetbrainsdVersion(exe)
	r := result{Version: ver, File: exe, Exe: exe, Folder: filepath.Dir(exe), ProductURL: "https://www.jetbrains.com/toolbox-app/", Search: "jetbrainsd"}
	tp.withOwner(&r, exe)
	if r.Owner == "" {
		r.Owner = "JetBrains"
	}
	f, _ := tp.scan(exe, scanRoots)
	r.Status, r.Summary, r.Details = jetbrainsdVerdict(ver, f, downloadChoice(exe))
	tp.c.finish(it, r)
}

// jetbrainsdVerdict judges version ver of jetbrainsd, whose program holds f; choice is
// the Toolbox App to download for its processor (downloadChoice).
func jetbrainsdVerdict(ver string, f embedFindings, choice string) (Status, string, string) {
	details := ""
	if f.HasRoots() {
		details = rootsField(f)
	}
	st, fixed := fixStatus("jetbrainsd", ParseVersion(ver))
	if f.Blocklist || st == FixOK {
		return StOK, T("app.sumOK"), joinParas(details, Tf("jbd.ok", strings.TrimSpace("jetbrainsd "+ver)))
	}
	fix := joinParas(Tf("jbd.fix", fixRules["jetbrainsd"].fixed[0]), Tf("jb.toolboxGet", toolboxURL, choice), Tf("jbd.byHand", jetbrainsBlog))
	if st == FixOutdated {
		return StWarn, Tf("app.sumOutdated", fixed), joinParas(details, T("jbd.why"), fix)
	}
	return StCaution, T("version.unknown"), joinParas(details, T("jbd.why"), fix)
}

// ---------------------------------------------------------------------------
// IDEs and the Toolbox App

// The pages to download the Toolbox App and the other JetBrains products.
const (
	toolboxURL  = "https://www.jetbrains.com/toolbox-app/"
	productsURL = "https://www.jetbrains.com/products/"
)

// jetbrainsProducts: the IDEs of JetBrains' bulletin, by the product code of their
// product-info.json, with their fixed versions (fixRules).
var jetbrainsProducts = map[string]string{
	"IU": "jb-idea", "IC": "jb-idea",
	"PY": "jb-pycharm", "PC": "jb-pycharm", "PS": "jb-pycharm", "RM": "jb-pycharm", "WS": "jb-pycharm",
	"RD": "jb-rider", "CL": "jb-clion", "DS": "jb-dataspell", "DB": "jb-datagrip", "GO": "jb-goland",
	"RR": "jb-rustrover", "GW": "jb-gateway",
}

// The fixed versions of the bulletin, one per release line. "2026.3" stands for the
// 2026.3 release and its release candidate; Rider's and RustRover's 2026.3 preview
// builds (EAP 9 and EAP 8) cannot be told apart from the earlier ones (see
// jetbrainsIDEVerdict). PyCharm, PhpStorm, RubyMine and WebStorm share their fixed
// versions.
func init() {
	for k, fixed := range map[string][]string{
		"jb-idea":      {"2025.2.6.4", "2025.3.6.2", "2026.1.5.1", "2026.2.4", "2026.3"},
		"jb-pycharm":   {"2025.2.6.3", "2025.3.6.2", "2026.1.5.1", "2026.2.4", "2026.3"},
		"jb-rider":     {"2025.3.5.3", "2026.1.5.3", "2026.2.4", "2026.3"},
		"jb-clion":     {"2025.2.6.3", "2025.3.6.3", "2026.1.5.2", "2026.2.4.1", "2026.3"},
		"jb-dataspell": {"2025.2.4.1", "2025.3.3.1", "2026.1.3.1"},
		"jb-datagrip":  {"2025.2.6.1", "2025.3.6.1", "2026.1.4.1", "2026.2.6.1", "2026.3"},
		"jb-goland":    {"2025.2.6.3", "2025.3.5.2", "2026.1.5.1", "2026.2.4", "2026.3"},
		"jb-rustrover": {"2025.2.6.1", "2025.3.7.1", "2026.1.4.1", "2026.2.5", "2026.3"},
		"jb-gateway":   {"2025.2.6", "2025.3.3", "2026.1.4", "2026.2.4", "2026.3"},
	} {
		fixRules[k] = fixRule{2, fixed, FixUnsupported}
	}
}

// productInfo is what eMudScan reads in the product-info.json of an IDE.
type productInfo struct {
	Name, Version, VersionSuffix, ProductCode string
	Launch                                    []struct{ OS, LauncherPath string }
}

// readProductInfo reads dir\product-info.json, for an IDE of the bulletin.
func readProductInfo(dir string) (productInfo, bool) {
	var pi productInfo
	data, err := os.ReadFile(filepath.Join(dir, "product-info.json"))
	if err != nil || len(data) > 4<<20 || json.Unmarshal(data, &pi) != nil {
		return pi, false
	}
	pi.Name, pi.Version, pi.VersionSuffix = plain(pi.Name), plain(pi.Version), plain(pi.VersionSuffix)
	_, known := jetbrainsProducts[pi.ProductCode]
	return pi, known && pi.Name != ""
}

// shownVersion: 2026.2.4, or 2026.3 EAP for a preview build.
func (pi productInfo) shownVersion() string {
	return strings.TrimSpace(pi.Version + " " + pi.VersionSuffix)
}

// launcher is the IDE's program for Windows (bin\idea64.exe).
func (pi productInfo) launcher(dir string) string {
	for _, l := range pi.Launch {
		if strings.EqualFold(l.OS, "Windows") && l.LauncherPath != "" {
			return filepath.Join(dir, filepath.FromSlash(l.LauncherPath))
		}
	}
	return ""
}

// ideBuildCopy: the copies of IDEs that Gradle downloads to build plugins are not
// installed software.
func ideBuildCopy(p string) bool {
	s := strings.ReplaceAll(lower(p), `\`, "/")
	return strings.Contains(s, "/.gradle/") || strings.Contains(s, "/.intellijplatform/")
}

// isToolboxBin: the folder of the Toolbox App's program, with its version (build.txt).
func isToolboxBin(dir string) bool {
	return exists(filepath.Join(dir, "jetbrains-toolbox.exe")) && exists(filepath.Join(dir, "build.txt"))
}

// toolboxVersion: 3.9.0 for build 3.9.0.88839.
func toolboxVersion(bin string) string {
	v := ParseVersion(firstLine(filepath.Join(bin, "build.txt")))
	if len(v) > 3 {
		v = v[:3]
	}
	if len(v) < 2 {
		return ""
	}
	return v.String()
}

// jetbrainsOwner: what belongs to an IDE of the bulletin (its jbr folder…) or to the
// Toolbox App (its bin\jre folder), when Windows does not record the software.
func jetbrainsOwner(p string) (ownerInfo, bool) {
	for dir, i := p, 0; i < 4; dir, i = filepath.Dir(dir), i+1 {
		if pi, ok := readProductInfo(dir); ok {
			return ownerInfo{Name: pi.Name, URL: productsURL, Loc: dir, Version: pi.shownVersion(), Exe: pi.launcher(dir)}, true
		}
		if isToolboxBin(dir) {
			return ownerInfo{Name: "JetBrains Toolbox App", URL: toolboxURL, Loc: filepath.Dir(dir), Version: toolboxVersion(dir),
				Exe: filepath.Join(dir, "jetbrains-toolbox.exe")}, true
		}
		if filepath.Dir(dir) == dir {
			break
		}
	}
	return ownerInfo{}, false
}

// jetbrainsJavaAdvice is the advice for the Java of an IDE (home: its jbr folder) or
// of the Toolbox App (its bin\jre folder): update it to the version that fixes it,
// and how. For any other Java, it is update.
func jetbrainsJavaAdvice(home, update string) string {
	parent := filepath.Dir(home)
	if isToolboxBin(parent) {
		return joinParas(T("jb.toolbox"), Tf("jb.toolboxGet", toolboxURL, downloadChoice(filepath.Join(parent, "jetbrains-toolbox.exe"))),
			T("jb.toolboxDaemon"))
	}
	pi, ok := readProductInfo(parent)
	if !ok || !strings.EqualFold(filepath.Base(home), "jbr") {
		return update
	}
	rule := jetbrainsProducts[pi.ProductCode]
	rec := Tf("jb.javaUpdate", pi.Name)
	switch st, fixed := fixStatus(rule, ParseVersion(pi.Version)); st {
	case FixOutdated:
		rec = Tf("jb.javaFix", pi.Name, fixed)
	case FixUnsupported:
		rec = Tf("jb.javaNoFix", pi.Name+" "+pi.shownVersion(), firstFixedLine(rule))
	}
	return joinParas(rec, ideGet(pi))
}

// ideGet says how to update the IDE pi: with the Toolbox App, from the IDE, or from
// JetBrains' products page (which no longer has DataSpell).
func ideGet(pi productInfo) string {
	if pi.ProductCode == "DS" {
		return Tf("jb.ideGetNoPage", pi.Name)
	}
	return Tf("jb.ideGet", pi.Name, productsURL)
}

// exeMachine is the processor of a Windows program, from its header: "amd64",
// "arm64" or "".
func exeMachine(p string) string {
	f, err := os.Open(p)
	if err != nil {
		return ""
	}
	defer f.Close()
	var h [6]byte
	if _, err := f.ReadAt(h[:4], 0x3c); err != nil {
		return ""
	}
	off := int64(binary.LittleEndian.Uint32(h[:4]))
	if _, err := f.ReadAt(h[:], off); err != nil || string(h[:4]) != "PE\x00\x00" {
		return ""
	}
	switch binary.LittleEndian.Uint16(h[4:]) {
	case pe.IMAGE_FILE_MACHINE_AMD64:
		return "amd64"
	case pe.IMAGE_FILE_MACHINE_ARM64:
		return "arm64"
	}
	return ""
}

// downloadChoice is the file to choose on the Toolbox App's page, for the processor of
// exe, a JetBrains program installed on the PC.
func downloadChoice(exe string) string {
	switch exeMachine(exe) {
	case "amd64":
		return ".exe (Windows)"
	case "arm64":
		return ".exe (Windows ARM64)"
	}
	return T("jb.anyArch")
}

// firstFixedLine is the oldest release line that JetBrains fixes (2025.2).
func firstFixedLine(rule string) string {
	return lineOf(ParseVersion(fixRules[rule].fixed[0]), 2).String()
}

// fixedAfter is the fixed version of the first release line after line (2025.3.6.2
// after 2025.2), or "".
func fixedAfter(rule string, line Version) string {
	for _, f := range fixRules[rule].fixed {
		if lineOf(ParseVersion(f), 2).Cmp(line) > 0 {
			return f
		}
	}
	return ""
}

func (tp *appsContext) checkJetbrainsIDEs() {
	for _, dir := range dedupePaths(tp.d.ideDirs) {
		pi, ok := readProductInfo(dir)
		if !ok || ideBuildCopy(dir) || !tp.inScope(dir) {
			continue
		}
		dir := dir
		tp.c.begin(tp.groupOf(pi.Name, dir), "jetbrains", pi.Name, dir, func(it *Item) { tp.jetbrainsIDEOne(it, dir) })
	}
}

func (tp *appsContext) jetbrainsIDEOne(it *Item, dir string) {
	pi, _ := readProductInfo(dir)
	r := result{Version: pi.shownVersion(), File: firstNonEmpty(pi.launcher(dir), filepath.Join(dir, "product-info.json")), Exe: pi.launcher(dir), Folder: dir,
		ProductURL: productsURL, Search: pi.Name}
	tp.withOwner(&r, dir)
	r.Status, r.Summary, r.Details = jetbrainsIDEVerdict(pi, jetbrainsProducts[pi.ProductCode])
	tp.c.finish(it, r)
}

// jetbrainsIDEVerdict judges an IDE by its version, with the fixed versions of rule.
func jetbrainsIDEVerdict(pi productInfo, rule string) (Status, string, string) {
	v := ParseVersion(pi.Version)
	full := pi.Name + " " + pi.shownVersion()
	why, get, bulletin := T("jb.why"), ideGet(pi), Tf("jb.bulletin", jetbrainsBlog)
	line := lineOf(v, 2)
	st, fixed := fixStatus(rule, v)
	fixes := fixRules[rule].fixed
	newest := lineOf(ParseVersion(fixes[len(fixes)-1]), 2)
	switch {
	case line == nil:
		return StCaution, T("version.unknown"), joinParas(why, Tf("jb.fixedList", fixedVersionsList(rule)), get, bulletin)
	case pi.VersionSuffix != "" && !strings.HasPrefix(strings.ToUpper(pi.VersionSuffix), "RC") && line.Cmp(newest) == 0:
		// a preview build of the newest line: only the latest ones have the fix
		return StCaution, T("jb.sumPreview"), joinParas(why, Tf("jb.preview", line.String(), line.String()), get, bulletin)
	case st == FixOK && line.Cmp(Version{2025, 2}) == 0:
		// the 2025.2.6 updates fix the Java runtime only
		return StCaution, T("jb.sumBrowser"), joinParas(Tf("jb.jbrOnly", full, fixedAfter(rule, line)), get, T("jb.jcefOff"), bulletin)
	case st == FixOK:
		return StOK, T("app.sumOK"), joinParas(Tf("jb.ok", full), T("jb.jcefOff"), bulletin)
	case st == FixOutdated:
		fix := joinParas(Tf("app.fix", pi.Name, fixed), get)
		if line.Cmp(Version{2025, 2}) == 0 {
			fix = joinParas(fix, Tf("jb.line20252", pi.Name+" "+fixed, fixedAfter(rule, line)), T("jb.jcefOff"))
		}
		return StWarn, Tf("app.sumOutdated", fixed), joinParas(why, fix, bulletin)
	}
	return StWarn, T("jb.sumUnsupported"), joinParas(why, Tf("jb.unsupported", full, firstFixedLine(rule), fixedVersionsList(rule)), get, bulletin)
}
