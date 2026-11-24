package main

// eMudScan's update of itself, from its latest release on GitHub.
//
// When a scan starts with "Online checks and updates" checked, once in a session,
// and when the About box asks for it, eMudScan asks GitHub for its latest release
// (updateLookup). A newer one is offered in a dialog, with the release notes of the
// new versions (updateNotes); if the user accepts it, eMudScan downloads its
// eMudScan.exe and checks it (updateDownload), then installs it (update_windows.go).
//
// A release is an update only if it is published (not a draft or a pre-release),
// immutable (once published, its files and its tag cannot change), with a version
// higher than this one, and if its eMudScan.exe has the same SHA-256 in three places:
// GitHub's digest of the file, which GitHub computes when the file is uploaded; the
// release notes, which the build writes (build.yml); and the downloaded file.
// eMudScan builds the address of the download itself, from the version.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	appRepo        = appAuthor + "/" + appName // 3c0/eMudScan, on GitHub
	updateAPIHost  = "api.github.com"
	updateLatest   = "/repos/" + appRepo + "/releases/latest"
	updateList     = "/repos/" + appRepo + "/releases?per_page=30" // the newest releases, with their notes
	updateExe      = appName + ".exe"
	updateMaxSize  = 64 << 20 // the largest eMudScan.exe accepted
	updateNotesMax = 10       // the most versions whose release notes the dialog shows
)

// releaseNotes is what a release says of itself.
type releaseNotes struct {
	version   string    // its tag: "4.0.1"
	published time.Time // when it was published; zero when unknown
	changes   string    // "In this version" of its release notes, in Markdown
}

// updateRelease is a newer release of eMudScan, which parseRelease checked.
type updateRelease struct {
	releaseNotes
	sha256 string // the SHA-256 of its eMudScan.exe, in lower-case hexadecimal
	size   int    // the size of its eMudScan.exe
}

// path is where github.com serves its eMudScan.exe.
func (r updateRelease) path() string {
	return "/" + appRepo + "/releases/download/" + r.version + "/" + updateExe
}

// page is its release page.
func (r updateRelease) page() string { return appURL + "/releases/tag/" + r.version }

var (
	updateTagRe    = regexp.MustCompile(`^\d+\.\d+\.\d+$`)
	updateDigestRe = regexp.MustCompile(`^sha256:([0-9a-f]{64})$`)
	// the line of the release notes that the build writes (.github/release-notes.md)
	updateNotesSumRe = regexp.MustCompile("(?m)^- SHA-256 of `" + regexp.QuoteMeta(updateExe) + "`: `([0-9a-f]{64})`\\s*$")
)

// githubRelease is what eMudScan reads of a release in GitHub's answers.
type githubRelease struct {
	Tag        string `json:"tag_name"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
	Immutable  bool   `json:"immutable"`
	Published  string `json:"published_at"`
	Body       string `json:"body"`
	Assets     []struct {
		Name   string `json:"name"`
		State  string `json:"state"`
		Size   int    `json:"size"`
		Digest string `json:"digest"`
	} `json:"assets"`
}

// newer tells whether rel is a published version, higher than current.
func (rel githubRelease) newer(current string) bool {
	cur := ParseVersion(current)
	return updateTagRe.MatchString(rel.Tag) && !rel.Draft && !rel.Prerelease && len(cur) >= 3 && ParseVersion(rel.Tag).Cmp(cur) > 0
}

// notes is what rel says of itself.
func (rel githubRelease) notes() releaseNotes {
	t, _ := time.Parse(time.RFC3339, rel.Published)
	return releaseNotes{version: rel.Tag, published: t, changes: releaseChanges(rel.Body)}
}

// parseRelease reads GitHub's answer about the latest release (JSON). It gives the
// release when it is newer than current and checked; false when it is not newer; an
// error, which says why, when it is newer but not to be trusted.
func parseRelease(body []byte, current string) (updateRelease, bool, error) {
	var rel githubRelease
	if err := json.Unmarshal(body, &rel); err != nil {
		return updateRelease{}, false, errors.New(T("update.badAnswer"))
	}
	if !rel.newer(current) {
		return updateRelease{}, false, nil
	}
	if !rel.Immutable {
		return updateRelease{}, false, errors.New(Tf("update.notImmutable", rel.Tag))
	}
	r := updateRelease{releaseNotes: rel.notes()}
	found := 0
	for _, a := range rel.Assets {
		if a.Name != updateExe {
			continue
		}
		found++
		m := updateDigestRe.FindStringSubmatch(a.Digest)
		if a.State != "uploaded" || m == nil {
			return updateRelease{}, false, errors.New(Tf("update.noDigest", rel.Tag))
		}
		if a.Size <= 0 || a.Size > updateMaxSize {
			return updateRelease{}, false, errors.New(Tf("update.badSize", rel.Tag))
		}
		r.sha256, r.size = m[1], a.Size
	}
	if found != 1 {
		return updateRelease{}, false, errors.New(Tf("update.noExe", rel.Tag))
	}
	if m := updateNotesSumRe.FindAllStringSubmatch(rel.Body, -1); len(m) != 1 || m[0][1] != r.sha256 {
		return updateRelease{}, false, errors.New(Tf("update.notesSum", rel.Tag))
	}
	return r, true, nil
}

// releaseChanges is the part "In this version" of release notes, in Markdown, without
// the link to the changelog that ends it: whole lines, at most 4000 characters.
func releaseChanges(body string) string {
	_, body, ok := strings.Cut("\n"+strings.ReplaceAll(body, "\r\n", "\n"), "\n## In this version\n")
	if !ok {
		return ""
	}
	if j := strings.Index(body, "\n## "); j >= 0 {
		body = body[:j]
	}
	if j := strings.Index(body, "\nFull list of changes"); j >= 0 {
		body = body[:j]
	}
	var lines []string
	n := 0
	for _, l := range strings.Split(strings.TrimSpace(body), "\n") {
		if n += utf8.RuneCountInString(l) + 1; n > 4000 {
			lines = append(lines, "…")
			break
		}
		lines = append(lines, strings.TrimRight(l, " \t"))
	}
	return strings.Join(lines, "\n")
}

// parseReleaseList reads GitHub's list of releases (JSON): the release notes of the
// versions newer than current, up to latest, the newest first, at most
// updateNotesMax, and whether there are more. Those of latest, from its own answer,
// come first, even when the list cannot be read; the last result tells whether it
// could.
func parseReleaseList(body []byte, current string, latest releaseNotes) ([]releaseNotes, bool, bool) {
	var rels []githubRelease
	read := json.Unmarshal(body, &rels) == nil
	top := ParseVersion(latest.version)
	out := []releaseNotes{latest}
	for _, rel := range rels {
		if rel.newer(current) && ParseVersion(rel.Tag).Cmp(top) < 0 &&
			!slices.ContainsFunc(out, func(n releaseNotes) bool { return n.version == rel.Tag }) {
			out = append(out, rel.notes())
		}
	}
	slices.SortStableFunc(out[1:], func(a, b releaseNotes) int { return ParseVersion(b.version).Cmp(ParseVersion(a.version)) })
	if len(out) > updateNotesMax {
		return out[:updateNotesMax], true, read
	}
	return out, false, read
}

// nextVersion tells whether latest comes right after current, with no version
// between them: the next patch (4.0.1 after 4.0.0). Then no list of the releases is
// needed for their notes.
func nextVersion(current, latest string) bool {
	c, l := ParseVersion(current), ParseVersion(latest)
	return len(c) >= 3 && len(l) == 3 && l[0] == c[0] && l[1] == c[1] && l[2] == c[2]+1
}

// updateLookup asks GitHub for the latest release of eMudScan (parseRelease).
func updateLookup() (updateRelease, bool, error) {
	if httpsGet == nil {
		return updateRelease{}, false, errors.New(T("update.noAnswer"))
	}
	body, ok := httpsGet(updateAPIHost, updateLatest, 1<<20)
	if !ok {
		return updateRelease{}, false, errors.New(T("update.noAnswer"))
	}
	return parseRelease(body, appVersion)
}

// updateNotes asks GitHub for its list of releases, which gives the release notes of
// the new versions, up to r (parseReleaseList).
func updateNotes(r updateRelease) ([]releaseNotes, bool, bool) {
	var body []byte
	if httpsGet != nil {
		body, _ = httpsGet(updateAPIHost, updateList, 4<<20)
	}
	return parseReleaseList(body, appVersion, r.releaseNotes)
}

// errUpdateStopped is the error of a download that the user stopped.
var errUpdateStopped = errors.New("stopped")

// updateDownload downloads the eMudScan.exe of r from github.com, which redirects to
// the host that serves the files of releases, and checks its size and its SHA-256.
// watch, when not nil, is told how many bytes have come, as they come; when it gives
// false, the download stops (errUpdateStopped).
func updateDownload(r updateRelease, watch func(got int) bool) ([]byte, error) {
	if httpsGetWatched == nil || r.size <= 0 || r.size > updateMaxSize {
		return nil, errors.New(T("update.downloadFailed"))
	}
	stopped := false
	data, ok := httpsGetWatched("github.com", r.path(), r.size+1, func(got int) bool {
		stopped = watch != nil && !watch(got)
		return !stopped
	})
	if stopped {
		return nil, errUpdateStopped
	}
	if !ok {
		return nil, errors.New(T("update.downloadFailed"))
	}
	if len(data) != r.size || !sameSum(data, r.sha256) {
		return nil, errors.New(T("update.badDownload"))
	}
	return data, nil
}

// sameSum tells whether data has the SHA-256 sum (lower-case hexadecimal).
func sameSum(data []byte, sum string) bool { return sumOf(data) == sum }

// sumOf is the SHA-256 of data, in lower-case hexadecimal.
func sumOf(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// megabytes writes a size in megabytes, with one decimal: "7.4 MB", "7,4 Mo".
func megabytes(n int) string {
	t := strconv.FormatFloat(float64(n)/(1<<20), 'f', 1, 64)
	if Lang == 1 {
		t = strings.Replace(t, ".", ",", 1)
	}
	return Tf("update.mb", t)
}

// reason is err as a message gives it, after a colon: without its final period.
func reason(err error) string { return strings.TrimRight(err.Error(), ". ") }

// ---------------------------------------------------------------------------
// The release notes in the update dialog

// notesBlocks shows release notes in the update dialog: for each version, a heading
// with the day it was published, then its changes; when there are more new versions
// than those shown, a link to the release notes of all of them.
func notesBlocks(notes []releaseNotes, more bool) []docBlock {
	var out []docBlock
	for _, n := range notes {
		head := appName + " " + n.version
		if !n.published.IsZero() {
			head += " – " + longDate(n.published)
		}
		out = append(out, docBlock{kind: dkHeading, runs: []mrun{{kind: runText, text: head}}})
		out = append(out, markdownBlocks(n.changes)...)
	}
	if more {
		out = append(out, docBlock{kind: dkSpace}, docBlock{kind: dkPara, runs: parseInline(Tf("update.more", appURL+"/releases"))})
	}
	return out
}

// markdownBlocks reads the Markdown of release notes: headings, which it shows in
// bold, lists and paragraphs, whose lines may be cut (a line that starts with spaces
// goes on with the item or the paragraph before it).
func markdownBlocks(md string) []docBlock {
	var out []docBlock
	kind, text := dkPara, ""
	flush := func() {
		if text != "" {
			out = append(out, docBlock{kind: kind, runs: markdownInline(text)})
		}
		kind, text = dkPara, ""
	}
	for _, l := range strings.Split(md, "\n") {
		t := strings.TrimSpace(l)
		switch {
		case t == "":
			flush()
		case strings.HasPrefix(l, "#"):
			flush()
			if len(out) > 0 {
				out = append(out, docBlock{kind: dkSpace})
			}
			out = append(out, docBlock{kind: dkPara, runs: []mrun{{kind: runBold, text: strings.TrimSpace(strings.TrimLeft(t, "#"))}}})
		case strings.HasPrefix(l, "- ") || strings.HasPrefix(l, "* "):
			flush()
			kind, text = dkBullet, t[2:]
		case text != "":
			text += " " + t
		default:
			text = t
		}
	}
	flush()
	return out
}

// mdInlineRe finds what markdownInline shows: **bold**, `code`, [text](address).
var mdInlineRe = regexp.MustCompile("\\*\\*(.+?)\\*\\*|`([^`]+)`|\\[([^\\]]+)\\]\\(([^)\\s]+)\\)")

// markdownInline reads a line of Markdown: bold text, code as plain text, and links,
// of which only those to web pages (https) are kept.
func markdownInline(line string) []mrun {
	var runs []mrun
	for {
		m := mdInlineRe.FindStringSubmatchIndex(line)
		if m == nil {
			break
		}
		if m[0] > 0 {
			runs = append(runs, mrun{kind: runText, text: line[:m[0]]})
		}
		switch {
		case m[2] >= 0:
			runs = append(runs, mrun{kind: runBold, text: strings.ReplaceAll(line[m[2]:m[3]], "`", "")})
		case m[4] >= 0:
			runs = append(runs, mrun{kind: runText, text: line[m[4]:m[5]]})
		case strings.HasPrefix(line[m[8]:m[9]], "https://"):
			runs = append(runs, mrun{kind: runLink, text: line[m[6]:m[7]], target: line[m[8]:m[9]]})
		default:
			runs = append(runs, mrun{kind: runText, text: line[m[6]:m[7]]})
		}
		line = line[m[1]:]
	}
	if line != "" {
		runs = append(runs, mrun{kind: runText, text: line})
	}
	return runs
}
