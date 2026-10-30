package main

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

const appName = "eMudScan"

const appVersion = "1.4"

// appDate is the date of this version (YYYYMMDD), written with appVersion.
const appDate = "20261029"

// appVersionDate is the version with its date, as it is shown everywhere:
// "1.0 (20261026)".
func appVersionDate() string { return appVersion + " (" + appDate + ")" }

// appTitle is the name shown at the top of the window: "eMudScan 1.0 (20261026)".
func appTitle() string { return appName + " " + appVersionDate() }

// minWindowsBuild: eMudScan needs Windows 10 version 1809 or Windows Server 2019 (both
// build 17763), or a later Windows. Go needs Windows 10 or Windows Server 2016, and
// SetProcessDpiAwarenessContext, which eMudScan calls at launch to draw at any screen
// resolution, exists from version 1703, after Windows Server 2016.
const minWindowsBuild = 17763

// windowsRecentEnough reports whether eMudScan runs on this Windows version.
func windowsRecentEnough(major, build uint32) bool {
	return major > 10 || major == 10 && build >= minWindowsBuild
}

// The author, the project page (source code and releases), and where the reference
// data is published (it outlives the domain).
const (
	appAuthor     = "3c0"
	appURL        = "https://github.com/3c0/eMudScan"
	dataSourceURL = "https://github.com/galadran/we-have-a-problem.com"
)

// Status of one checked element.
type Status int

const (
	StPending Status = iota
	StOK
	StInfo
	StWarn    // "Update needed"
	StError   // "Problem": confirmed (Windows itself still trusts the certificates)
	StCaution // "Warning": may be affected, worth checking
)

// severity orders statuses from worst (0) to best.
func (s Status) severity() int {
	switch s {
	case StError:
		return 0
	case StWarn:
		return 1
	case StCaution:
		return 2
	case StInfo:
		return 3
	case StOK:
		return 4
	}
	return 5
}

type Group int

const (
	GroupWindows Group = iota
	GroupApps
	GroupNet // programs that work mainly over the internet
)

// Item is one row of a status list.
type Item struct {
	ID         int
	Group      Group
	Kind       string // product key: git, python, node, java, electron, go, app, tool, bundle, curl, info, windows
	Name       string
	Version    string
	Status     Status
	Summary    string // short text shown in the list
	Details    string // full explanation, in the small markup of markup.go
	Owner      string // program the element belongs to (Apps & features, Scoop, folder)
	OwnerURL   string
	OwnerVer   string // version of the owner (Apps & features, Scoop manifest)
	ProductURL string
	Bundled    bool // the component is a copy inside another program (Owner)

	// shown in the program lists
	Packages    []pkgRef // packages of the software (Scoop, WinGet, Chocolatey)
	Program     string   // the program the user knows: Owner, else the item itself, else a guess from its folder
	ProgVersion string   // version of the program (Version is the version of the component)
	Component   string   // the part that is checked (certifi, Git, Electron…); empty when it is the program itself
	File        string   // outdated file (or the program file)
	Folder      string   // program folder
	Search      string   // web search query
	Recheck     bool     // can be checked again on its own
	Updated     lastUpdate
	Copies      []Item // the same finding in other places, shown with this element (repeated.go)
}

// lastUpdate is when the publisher last updated the software: the date of its main
// program (see pedate.go).
type lastUpdate struct {
	Time    time.Time
	Src     dateSource
	File    string // the file the date comes from (or that has no date)
	Unknown bool   // looked for, not found
}

func (u lastUpdate) known() bool { return u.Src != dateNone }

// shown: the date, or why it is unknown, is shown for every software.
func (u lastUpdate) shown() bool { return u.known() || u.Unknown }

// text is the date in words with where it comes from (file is how the file is
// named), or why it is unknown.
func (u lastUpdate) text(file string) string {
	switch {
	case u.known():
		return longDate(u.Time) + " (" + u.source(file) + ")"
	case u.File != "":
		return Tf("upd.unknownFile", file)
	}
	return T("upd.unknownNoFile")
}

// staleAfter: when software needs an update and its installed version is this many
// months old, a newer version may exist, or it may no longer be maintained (a fix
// may never come).
const staleAfter = 12

// staleMonths is how many whole months old the software is, at now, when it needs
// an update (not OK, not information) and its date is staleAfter months old or more;
// else 0.
func (it Item) staleMonths(now time.Time) int {
	switch it.Status {
	case StWarn, StError, StCaution:
	default:
		return 0
	}
	if it.Group == GroupWindows || !it.Updated.known() {
		return 0
	}
	a, b := it.Updated.Time.Local(), now.Local()
	m := (b.Year()-a.Year())*12 + int(b.Month()) - int(a.Month())
	if b.Day() < a.Day() {
		m--
	}
	if m < staleAfter {
		return 0
	}
	return m
}

// stale is staleMonths now.
func (it Item) stale() int { return it.staleMonths(time.Now()) }

// ageText writes an age: "15 months", "2 years".
func ageText(months int) string {
	if months >= 24 {
		return Tf("stale.years", months/12)
	}
	return Tf("stale.months", months)
}

// staleAdvice is the paragraph about software whose installed version is old.
func staleAdvice(it Item) string {
	if m := it.stale(); m > 0 {
		return Tf("stale.advice", ageText(m))
	}
	return ""
}

// source names where the date comes from, with the file ("digital signature of X").
func (u lastUpdate) source(file string) string {
	switch u.Src {
	case dateSigned:
		return Tf("upd.signed", file)
	case dateModified:
		return Tf("upd.modified", file)
	case dateBuilt:
		return Tf("upd.built", file)
	}
	return ""
}

var monthNames = [2][12]string{
	{"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"},
	{"janvier", "février", "mars", "avril", "mai", "juin", "juillet", "août", "septembre", "octobre", "novembre", "décembre"},
}

// longDate writes a date in words: "8 October 2026", "1er octobre 2026".
func longDate(t time.Time) string {
	t = t.Local()
	day := strconv.Itoa(t.Day())
	if Lang == 1 && t.Day() == 1 {
		day = "1er"
	}
	return day + " " + monthNames[Lang][t.Month()-1] + " " + strconv.Itoa(t.Year())
}

// Homepage returns the page where the program can be updated: the homepage of the
// program the component belongs to, never the component's own page when it is a copy
// inside another program.
func (it Item) Homepage() string {
	if it.OwnerURL != "" {
		return it.OwnerURL
	}
	if it.Owner != "" && it.Bundled {
		return ""
	}
	return it.ProductURL
}

// fileLabel is the label of the file of an element: the file found outdated, or for
// an element that is OK, the file that was checked.
func (it Item) fileLabel() string {
	if it.Status == StOK {
		return T("dlg.fileChecked")
	}
	return T("dlg.file")
}

// DisplayName is the software with its version and, when known, the package manager
// it was installed with: "tesseract 5.5.1 [Scoop]".
func (it Item) DisplayName() string {
	n := strings.TrimSpace(it.Program + " " + it.ProgVersion)
	if len(it.Packages) > 0 {
		n += " [" + it.Packages[0].Manager + "]"
	}
	return n
}

// SearchText is what "Search on Google" looks for.
func (it Item) SearchText() string {
	if it.Program != "" && (it.Bundled || it.Search == "") {
		return it.Program
	}
	if it.Search != "" {
		return it.Search
	}
	return it.Name
}

// programKinds are items that are a program themselves (not a component of one).
var programKinds = map[string]bool{"browser": true, "netapp": true, "app": true, "tool": true, "electron": true}

// fillDisplay computes Program and Component.
func (it *Item) fillDisplay(component string) {
	switch {
	case component != "":
		it.Component = component
	case programKinds[it.Kind]:
		it.Component = ""
	default:
		it.Component = it.Name
	}
	guessedVer := ""
	switch {
	case it.Owner != "":
		it.Program = it.Owner
	case programKinds[it.Kind]:
		it.Program = it.Name
	default:
		it.Program, guessedVer = guessProgramVersion(firstNonEmpty(it.File, it.Folder))
		if it.Program == "" {
			it.Program = it.Name
		}
	}
	if strings.EqualFold(strings.TrimSpace(it.Component), strings.TrimSpace(it.Program)) {
		it.Component = ""
	}
	// the version of the software: when it is checked itself, the version found in it
	// (Apps & features may record something else: "3.14-64" for Python 3.14.10);
	// for a part of it, the one Windows or its package manager records (Apps &
	// features, Scoop, the Store), else its own, so that all its parts show the same
	switch {
	case it.Component == "":
		it.ProgVersion = firstNonEmpty(it.ProgVersion, it.Version, it.OwnerVer)
	default:
		it.ProgVersion = firstNonEmpty(it.OwnerVer, it.ProgVersion, guessedVer)
	}
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}

// LogKind tells the progress log how to lay out a message.
type LogKind int

const (
	LogLine    LogKind = iota // a complete line (with a status mark if Status != StPending)
	LogSection                // a section title
	LogStart                  // the start of a step: the result follows on the same line
	LogEnd                    // the result of the current step
)

// Reporter receives progress and results from the checking goroutine.
type Reporter interface {
	Log(kind LogKind, st Status, text string)
	Upsert(it Item)
	Activity(text string) // what is being done right now (shown beside the spinner)
}

// result is what a check computes for one item.
type result struct {
	Status                                  Status
	Summary, Details                        string
	Name, Version, File, Folder, ProductURL string
	Owner, OwnerURL, Search                 string
	Bundled                                 bool
	Component, ProgVersion, OwnerVer        string // see Item
	Exe                                     string // the main program of the software, when the check knows it
}

type checker struct {
	r           Reporter
	afterFinish func(*Item)                            // completes an element once its check is done (package managers)
	refresh     func()                                 // reads again what may have changed before "Scan again" (installed software)
	offline     bool                                   // the user turned the online test off: no connection at all
	dated       func(kind string, r result) lastUpdate // when the software of a result was last updated
	mu          sync.Mutex
	nextID      int
	items       map[int]*Item
	order       []int
	rechecks    map[int]func(*Item)
	where       map[int]string // location shown in the log for each item
	cancel      func() bool
}

func newChecker(r Reporter, cancel func() bool) *checker {
	if cancel == nil {
		cancel = func() bool { return false }
	}
	return &checker{r: r, items: map[int]*Item{}, rechecks: map[int]func(*Item){}, where: map[int]string{}, cancel: cancel}
}

func (c *checker) section(s string)            { c.r.Log(LogSection, StPending, s) }
func (c *checker) line(st Status, s string)    { c.r.Log(LogLine, st, s) }
func (c *checker) logf(f string, a ...any)     { c.r.Log(LogLine, StPending, fmt.Sprintf(f, a...)) }
func (c *checker) stepStart(s string)          { c.r.Log(LogStart, StPending, s+" …") }
func (c *checker) stepEnd(st Status, s string) { c.r.Log(LogEnd, st, s) }
func (c *checker) activity(s string)           { c.r.Activity(s) }

// stepLabel is the text logged when an item is checked.
func stepLabel(name, where string) string {
	if where == "" {
		return name
	}
	return name + " — " + where
}

// begin adds an item in the "checking" state and starts its line in the log.
// check, if not nil, computes the item and can be run again later.
func (c *checker) begin(g Group, kind, name, where string, check func(*Item)) *Item {
	c.mu.Lock()
	c.nextID++
	it := &Item{ID: c.nextID, Group: g, Kind: kind, Name: name, Status: StPending, Summary: T("checking"), Recheck: check != nil}
	it.fillDisplay("")
	c.where[it.ID] = where
	c.items[it.ID] = it
	c.order = append(c.order, it.ID)
	if check != nil {
		c.rechecks[it.ID] = check
	}
	cp := *it
	c.mu.Unlock()
	c.r.Upsert(cp)
	c.stepStart(stepLabel(name, where))
	if check != nil {
		check(it)
	}
	return it
}

// finish records the result of a check and ends its line in the log.
func (c *checker) finish(it *Item, r result) {
	var upd lastUpdate
	if c.dated != nil && it.Group != GroupWindows {
		upd = c.dated(it.Kind, r)
	}
	c.mu.Lock()
	it.Updated = upd
	it.Status, it.Summary, it.Details = r.Status, r.Summary, strings.Trim(r.Details, "\n")
	// a result describes the item entirely: after "Scan again", nothing is kept from
	// the previous check (the software may have been updated in the meantime)
	if r.Name != "" {
		it.Name = r.Name
	}
	it.Version, it.File, it.Folder, it.ProductURL = r.Version, r.File, r.Folder, r.ProductURL
	it.Owner, it.OwnerURL, it.OwnerVer, it.Search = r.Owner, r.OwnerURL, r.OwnerVer, r.Search
	it.ProgVersion, it.Bundled = r.ProgVersion, r.Bundled
	it.fillDisplay(r.Component)
	if c.afterFinish != nil {
		c.afterFinish(it)
	}
	cp := *it
	c.mu.Unlock()
	c.r.Upsert(cp)
	c.stepEnd(r.Status, r.Summary)
}

// addQuiet creates an item whose result is already known, without logging it.
func (c *checker) addQuiet(g Group, kind, name string, r result) {
	c.mu.Lock()
	c.nextID++
	it := &Item{ID: c.nextID, Group: g, Kind: kind, Name: name}
	c.items[it.ID] = it
	c.order = append(c.order, it.ID)
	c.mu.Unlock()
	c.mu.Lock()
	it.Status, it.Summary, it.Details, it.Search = r.Status, r.Summary, r.Details, r.Search
	it.fillDisplay("")
	cp := *it
	c.mu.Unlock()
	c.r.Upsert(cp)
}

// add creates an item whose result is already known.
func (c *checker) add(g Group, kind, name string, r result) *Item {
	it := c.begin(g, kind, name, "", nil)
	c.finish(it, r)
	return it
}

// recheck runs the checks of the given items again.
func (c *checker) recheck(ids []int) {
	if c.refresh != nil {
		c.refresh()
	}
	for _, id := range ids {
		if c.cancel() {
			return
		}
		c.mu.Lock()
		it, fn := c.items[id], c.rechecks[id]
		if it == nil || fn == nil {
			c.mu.Unlock()
			continue
		}
		it.Status, it.Summary = StPending, T("checking")
		cp := *it
		c.mu.Unlock()
		c.r.Upsert(cp)
		c.mu.Lock()
		where := c.where[id]
		c.mu.Unlock()
		c.stepStart(stepLabel(cp.Name, where))
		fn(it)
	}
}

func (c *checker) snapshot() []Item {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]Item, 0, len(c.order))
	for _, id := range c.order {
		out = append(out, *c.items[id])
	}
	return out
}

// statusCounts counts the elements by status.
type statusCounts struct{ ok, info, caution, warn, errs int }

func (c *checker) counts() statusCounts {
	var n statusCounts
	for _, it := range mergeRepeated(c.snapshot()) { // a repeated finding counts once
		switch it.Status {
		case StOK:
			n.ok++
		case StInfo:
			n.info++
		case StCaution:
			n.caution++
		case StWarn:
			n.warn++
		case StError:
			n.errs++
		}
	}
	return n
}

// text formats the counts into a message: "Result: 15 OK · 22 to update · 1 problem…".
func (n statusCounts) text(key string, args ...any) string {
	part := func(count int, k string) string {
		if count == 1 {
			return Tf(k+"1", count)
		}
		return Tf(k+"N", count)
	}
	parts := []string{part(n.ok, "cnt.ok"), part(n.warn, "cnt.warn"), part(n.caution, "cnt.caution"), part(n.errs, "cnt.err"), part(n.info, "cnt.info")}
	return Tf(key, append(args, strings.Join(parts, " · "))...)
}

// ---------------------------------------------------------------------------
// Versions

// Version is a dotted numeric version. Non-numeric parts are ignored, so
// "2.55.0.windows.5" -> [2 55 0 5], "v26.12.1" -> [26 12 1], "1.8.0_511" -> [1 8 0 511].
type Version []int

func ParseVersion(s string) Version {
	var v Version
	cur, in := 0, false
	for _, r := range s {
		if r >= '0' && r <= '9' {
			if cur > 100000000 {
				return v
			}
			cur = cur*10 + int(r-'0')
			in = true
			continue
		}
		if in {
			v = append(v, cur)
			cur, in = 0, false
		}
		if r == ' ' || r == '(' || r == '+' || r == '-' {
			if len(v) > 0 {
				return v
			}
		}
	}
	if in {
		v = append(v, cur)
	}
	return v
}

func (v Version) String() string {
	parts := make([]string, len(v))
	for i, n := range v {
		parts[i] = strconv.Itoa(n)
	}
	return strings.Join(parts, ".")
}

// Cmp compares two versions; missing trailing parts count as zero.
func (v Version) Cmp(o Version) int {
	n := max(len(v), len(o))
	for i := 0; i < n; i++ {
		a, b := 0, 0
		if i < len(v) {
			a = v[i]
		}
		if i < len(o) {
			b = o[i]
		}
		if a != b {
			if a < b {
				return -1
			}
			return 1
		}
	}
	return 0
}

type FixState int

const (
	FixUnknown     FixState = iota
	FixOK                   // contains the fix
	FixOutdated             // a fixed version exists on the same release line
	FixUnsupported          // release line no longer maintained
	FixNoRelease            // no fixed release on this line
)

type fixRule struct {
	lineParts int      // leading parts that define a release line (0 = single minimum)
	fixed     []string // first fixed version on each maintained line, ascending
	oldLines  FixState // state for lines older than the oldest listed one
}

// Fixed versions as listed on https://emudhra.we-have-a-problem.com/ (28 October 2026).
// Python's are those of CVE-2026-115557: from them, the ssl module refuses any chain
// containing one of the compromised keys, whatever certificate list is used (see
// pythonFix). Python 3.10 and older are no longer maintained and get no fix.
var fixRules = map[string]fixRule{
	"electron":       {1, []string{"42.11.16", "43.8.1", "44.9.1"}, FixUnsupported},
	"node":           {1, []string{"22.23.4", "24.22.1", "26.12.1"}, FixUnsupported},
	"python":         {2, []string{"3.11.18", "3.12.16", "3.13.18", "3.14.10", "3.15.2"}, FixUnsupported},
	"go":             {2, []string{"1.26.10", "1.27.3"}, FixUnsupported},
	"git":            {0, []string{"2.56.0.3"}, FixOutdated},
	"pip":            {0, []string{"26.2.2"}, FixOutdated},
	"certifi":        {0, []string{"2026.10.19"}, FixOutdated},
	"docker-desktop": {0, []string{"4.96.2"}, FixOutdated},
	"docker":         {0, []string{"29.9.2"}, FixOutdated},
	"gh":             {0, []string{"2.102.1"}, FixOutdated},
	"vscode":         {0, []string{"1.143.1"}, FixOutdated},
	"obsidian":       {0, []string{"1.14.5"}, FixOutdated},
	"uv":             {0, []string{"0.13.5"}, FixOutdated},
	"pixi":           {0, []string{"0.82.1"}, FixOutdated},
	"deno":           {0, []string{"2.9.8"}, FixOutdated},
	"bun":            {0, []string{"1.4.4"}, FixOutdated},
	"rustup":         {0, []string{"1.29.2"}, FixOutdated},
	"mise":           {0, []string{"2026.10.17"}, FixOutdated},
	"conda":          {0, []string{"26.9.4"}, FixOutdated},
	"claude-code":    {0, []string{"2.1.303"}, FixOutdated},
}

// How fixed versions are written by their publishers, when it differs from the numeric form.
var fixedDisplay = map[string]string{"2.56.0.3": "2.56.0.windows.3"}

func showFixed(v string) string {
	if d, ok := fixedDisplay[v]; ok {
		return d
	}
	return v
}

func lineOf(v Version, parts int) Version {
	if len(v) < parts {
		return nil
	}
	return v[:parts]
}

// fixStatus returns the state of version v of product, and the fixed version to suggest.
func fixStatus(product string, v Version) (FixState, string) {
	rule, ok := fixRules[product]
	if !ok || len(v) == 0 {
		return FixUnknown, ""
	}
	newest := rule.fixed[len(rule.fixed)-1]
	if rule.lineParts == 0 {
		f := ParseVersion(rule.fixed[0])
		if len(v) < len(f) && v.Cmp(f[:len(v)]) == 0 {
			return FixUnknown, showFixed(rule.fixed[0])
		}
		if v.Cmp(f) >= 0 {
			return FixOK, ""
		}
		return FixOutdated, showFixed(rule.fixed[0])
	}
	line := lineOf(v, rule.lineParts)
	if line == nil {
		return FixUnknown, newest
	}
	for _, fs := range rule.fixed {
		f := ParseVersion(fs)
		if line.Cmp(f[:rule.lineParts]) == 0 {
			if v.Cmp(f) >= 0 {
				return FixOK, ""
			}
			return FixOutdated, fs
		}
	}
	if line.Cmp(lineOf(ParseVersion(newest), rule.lineParts)) > 0 {
		return FixOK, ""
	}
	return rule.oldLines, newest
}

func fixedVersionsList(product string) string {
	var out []string
	for _, f := range fixRules[product].fixed {
		out = append(out, showFixed(f))
	}
	return strings.Join(out, ", ")
}

// shortPath keeps long paths readable in log lines.
func shortPath(p string, max int) string {
	r := []rune(p)
	if len(r) <= max {
		return p
	}
	return string(r[:max/3]) + "…" + string(r[len(r)-(max-max/3-1):])
}

func itoa(i int) string { return strconv.Itoa(i) }
