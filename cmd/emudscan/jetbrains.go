package main

// JetBrains: jetbrainsd, the background service of the Toolbox App and of the
// JetBrains IDEs, checks its connections with its own built-in list of trusted
// certificates, which updating Windows or the IDEs' Java does not change (JetBrains:
// jetbrainsBlog). Version 0.9.9728 adds the fingerprints of the 16 compromised
// certificates to it, as text, which the scan finds (embedFindings.Blocklist).
// Each user has a single copy, installed in
// %LOCALAPPDATA%\JetBrains\Daemon\bundles\current by the Toolbox App or an IDE when
// it starts; the newest version replaces the older ones.

import (
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
	r.Status, r.Summary, r.Details = jetbrainsdVerdict(ver, f)
	tp.c.finish(it, r)
}

// jetbrainsdVerdict judges version ver of jetbrainsd, whose program holds f.
func jetbrainsdVerdict(ver string, f embedFindings) (Status, string, string) {
	details := ""
	if f.HasRoots() {
		details = rootsField(f)
	}
	st, fixed := fixStatus("jetbrainsd", ParseVersion(ver))
	if f.Blocklist || st == FixOK {
		return StOK, T("app.sumOK"), joinParas(details, Tf("jbd.ok", strings.TrimSpace("jetbrainsd "+ver)))
	}
	fix := Tf("jbd.fix", fixRules["jetbrainsd"].fixed[0], jetbrainsBlog)
	if st == FixOutdated {
		return StWarn, Tf("app.sumOutdated", fixed), joinParas(details, T("jbd.why"), fix)
	}
	return StCaution, T("version.unknown"), joinParas(details, T("jbd.why"), fix)
}
