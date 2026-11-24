package main

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

// releaseJSON is GitHub's answer about a release (the fields that eMudScan reads,
// among others), changed by edit.
func releaseJSON(t *testing.T, tag, sum string, edit func(rel map[string]any)) []byte {
	t.Helper()
	notes, err := os.ReadFile("../../.github/release-notes.md") // the release page that the build writes
	if err != nil {
		t.Fatal(err)
	}
	body := strings.NewReplacer("@CHANGES@", "### New\n\n- **Something** new, in `code`.\n\n### Fixed\n\n- A fix.",
		"@SHA256@", sum, "@SHA256_CLI@", strings.Repeat("c", 64), "@VERSION@", tag, "@REPOSITORY@", appRepo,
		"@DATE@", "2026-12-01", "@BUILD_LOG@", "https://github.com/"+appRepo+"/actions/runs/1").Replace(string(notes))
	rel := map[string]any{
		"tag_name": tag, "name": "eMudScan " + tag, "draft": false, "prerelease": false, "immutable": true, "body": body,
		"published_at": "2026-12-01T10:00:00Z",
		"assets": []any{
			map[string]any{"name": "eMudScan.exe", "state": "uploaded", "size": 3, "digest": "sha256:" + sum,
				"browser_download_url": "https://github.com/" + appRepo + "/releases/download/" + tag + "/eMudScan.exe"},
			map[string]any{"name": "eMudScan-cli.exe", "state": "uploaded", "size": 3, "digest": "sha256:" + strings.Repeat("c", 64)},
		},
	}
	if edit != nil {
		edit(rel)
	}
	b, err := json.Marshal(rel)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// A release is an update when it is newer, immutable, published, and its eMudScan.exe
// has the same SHA-256 for GitHub and in its notes; else none, or an error that says
// why. Its notes give its changes, in Markdown, and the day it was published.
func TestParseRelease(t *testing.T) {
	Lang = 0
	sum := sumOf([]byte("new"))
	r, found, err := parseRelease(releaseJSON(t, "3.2.0", sum, nil), "3.1.2")
	if !found || err != nil || r.version != "3.2.0" || r.sha256 != sum || r.size != 3 ||
		r.path() != "/3c0/eMudScan/releases/download/3.2.0/eMudScan.exe" || r.page() != appURL+"/releases/tag/3.2.0" {
		t.Fatalf("%v %v %+v", found, err, r)
	}
	if r.changes != "### New\n\n- **Something** new, in `code`.\n\n### Fixed\n\n- A fix." || !r.published.Equal(time.Date(2026, 12, 1, 10, 0, 0, 0, time.UTC)) {
		t.Errorf("notes: %v\n%s", r.published, r.changes)
	}
	// not newer: none, without an error
	for _, c := range []struct{ tag, current string }{{"3.1.2", "3.1.2"}, {"3.1.1", "3.1.2"}, {"3.10.0", "3.10.0"}, {"v4.0.0", "3.1.2"}, {"4.0", "3.1.2"}, {"4.0.0-rc.1", "3.1.2"}} {
		if r, found, err := parseRelease(releaseJSON(t, c.tag, sum, nil), c.current); found || err != nil {
			t.Errorf("%s over %s: %v %v %+v", c.tag, c.current, found, err, r)
		}
	}
	if _, found, err := parseRelease(releaseJSON(t, "3.10.0", sum, nil), "3.9.9"); !found || err != nil {
		t.Errorf("3.10.0 over 3.9.9: %v %v", found, err)
	}
	for name, edit := range map[string]func(map[string]any){
		"a draft":       func(rel map[string]any) { rel["draft"] = true },
		"a pre-release": func(rel map[string]any) { rel["prerelease"] = true },
	} {
		if _, found, err := parseRelease(releaseJSON(t, "3.2.0", sum, edit), "3.1.2"); found || err != nil {
			t.Errorf("%s: %v %v", name, found, err)
		}
	}
	// newer, but not to be trusted: an error that says why
	asset := func(rel map[string]any) map[string]any { return rel["assets"].([]any)[0].(map[string]any) }
	for name, c := range map[string]struct {
		edit func(map[string]any)
		key  string
	}{
		"not immutable":      {func(rel map[string]any) { rel["immutable"] = false }, "update.notImmutable"},
		"no immutable field": {func(rel map[string]any) { delete(rel, "immutable") }, "update.notImmutable"},
		"no eMudScan.exe":    {func(rel map[string]any) { rel["assets"] = rel["assets"].([]any)[1:] }, "update.noExe"},
		"two eMudScan.exe":   {func(rel map[string]any) { rel["assets"] = append(rel["assets"].([]any), asset(rel)) }, "update.noExe"},
		"no digest":          {func(rel map[string]any) { delete(asset(rel), "digest") }, "update.noDigest"},
		"another digest":     {func(rel map[string]any) { asset(rel)["digest"] = "sha512:" + sum }, "update.noDigest"},
		"an upper-case one":  {func(rel map[string]any) { asset(rel)["digest"] = "sha256:" + strings.ToUpper(sum) }, "update.noDigest"},
		"not uploaded":       {func(rel map[string]any) { asset(rel)["state"] = "starter" }, "update.noDigest"},
		"empty":              {func(rel map[string]any) { asset(rel)["size"] = 0 }, "update.badSize"},
		"too large":          {func(rel map[string]any) { asset(rel)["size"] = updateMaxSize + 1 }, "update.badSize"},
		"other notes": {func(rel map[string]any) {
			rel["body"] = strings.ReplaceAll(rel["body"].(string), sum, strings.Repeat("d", 64))
		}, "update.notesSum"},
		"no SHA-256 in notes": {func(rel map[string]any) {
			rel["body"] = strings.ReplaceAll(rel["body"].(string), "SHA-256 of `eMudScan.exe`", "SHA-256 of eMudScan")
		}, "update.notesSum"},
		"two in notes": {func(rel map[string]any) {
			rel["body"] = rel["body"].(string) + "\n- SHA-256 of `eMudScan.exe`: `" + strings.Repeat("d", 64) + "`\n"
		}, "update.notesSum"},
	} {
		r, found, err := parseRelease(releaseJSON(t, "3.2.0", sum, c.edit), "3.1.2")
		if found || err == nil || err.Error() != Tf(c.key, "3.2.0") {
			t.Errorf("%s: %v %v %+v", name, found, err, r)
		}
	}
	if _, found, err := parseRelease([]byte("<html>"), "3.1.2"); found || err == nil || err.Error() != T("update.badAnswer") {
		t.Errorf("not JSON: %v %v", found, err)
	}
	// the notes may be written with CRLF
	crlf := func(rel map[string]any) { rel["body"] = strings.ReplaceAll(rel["body"].(string), "\n", "\r\n") }
	if r, found, err := parseRelease(releaseJSON(t, "3.2.0", sum, crlf), "3.1.2"); !found || err != nil || !strings.HasPrefix(r.changes, "### New\n\n- **Something**") {
		t.Errorf("CRLF: %v %v %q", found, err, r.changes)
	}
}

// The changes of release notes: the part "In this version", in Markdown, without its
// link to the changelog; cut after whole lines when long.
func TestReleaseChanges(t *testing.T) {
	for body, want := range map[string]string{
		"x\n## In this version\n\n### New\n\n- **A** `b`  \n\nFull list of changes: [changelog](x).\n\n## Verify\n- y": "### New\n\n- **A** `b`",
		"x\n## In this version\n- a\n## Verify your download\n- b":                                                     "- a",
		"## In this version\n- first": "- first",
		"no such part":                "",
	} {
		if got := releaseChanges(body); got != want {
			t.Errorf("%q: %q, want %q", body, got, want)
		}
	}
	long := "\n## In this version\n" + strings.Repeat(strings.Repeat("é", 99)+"\n", 50)
	if got := releaseChanges(long); len([]rune(got)) != 40*100+1 || !strings.HasSuffix(got, "\n…") {
		t.Errorf("long: %d", len([]rune(got)))
	}
}

// The release notes of the new versions: those of the list newer than this one and
// not newer than the latest, published, the newest first; those of the latest come
// from its own answer, even when the list cannot be read, which it says; at most 10.
func TestParseReleaseList(t *testing.T) {
	rel := func(tag string, edit func(map[string]any)) map[string]any {
		r := map[string]any{"tag_name": tag, "draft": false, "prerelease": false, "published_at": "2026-12-01T10:00:00Z",
			"body": "x\n## In this version\n\n- In " + tag + ".\n"}
		if edit != nil {
			edit(r)
		}
		return r
	}
	list := func(rels ...map[string]any) []byte {
		b, _ := json.Marshal(rels)
		return b
	}
	latest := releaseNotes{version: "3.4.0", changes: "- Latest, from its own answer."}
	versions := func(notes []releaseNotes) string {
		var v []string
		for _, n := range notes {
			v = append(v, n.version)
		}
		return strings.Join(v, " ")
	}
	body := list(rel("3.4.0", nil), rel("3.2.0", nil), rel("3.3.0", nil), rel("3.5.0", nil), rel("3.1.2", nil), rel("3.1.0", nil),
		rel("3.3.1", func(r map[string]any) { r["draft"] = true }), rel("3.3.2", func(r map[string]any) { r["prerelease"] = true }),
		rel("v3.3.3", nil), rel("3.2.0", nil))
	notes, more, read := parseReleaseList(body, "3.1.2", latest)
	if versions(notes) != "3.4.0 3.3.0 3.2.0" || more || !read || notes[0].changes != latest.changes || notes[1].changes != "- In 3.3.0." {
		t.Errorf("%s %v %v %+v", versions(notes), more, read, notes)
	}
	for name, c := range map[string]struct {
		body []byte
		read bool
	}{"no list": {nil, false}, "not JSON": {[]byte("<html>"), false}, "empty": {list(), true}} {
		if notes, more, read := parseReleaseList(c.body, "3.1.2", latest); versions(notes) != "3.4.0" || more || read != c.read {
			t.Errorf("%s: %s %v %v", name, versions(notes), more, read)
		}
	}
	var many []map[string]any
	for i := 1; i <= 12; i++ {
		many = append(many, rel(fmt.Sprintf("4.0.%d", i), nil))
	}
	notes, more, _ = parseReleaseList(list(many...), "4.0.0", releaseNotes{version: "4.0.12"})
	if versions(notes) != "4.0.12 4.0.11 4.0.10 4.0.9 4.0.8 4.0.7 4.0.6 4.0.5 4.0.4 4.0.3" || !more {
		t.Errorf("many: %s %v", versions(notes), more)
	}
}

// The next patch needs no list of the releases: no version comes between.
func TestNextVersion(t *testing.T) {
	for _, c := range []struct {
		current, latest string
		next            bool
	}{
		{"4.0.0", "4.0.1", true}, {"4.0.1+dev", "4.0.2", true}, {"4.0.9", "4.0.10", true},
		{"4.0.0", "4.0.2", false}, {"4.0.0", "4.1.0", false}, {"4.0.9", "4.1.0", false}, {"4.0.0", "5.0.1", false}, {"4.0", "4.0.1", false},
	} {
		if got := nextVersion(c.current, c.latest); got != c.next {
			t.Errorf("%s after %s: %v", c.latest, c.current, got)
		}
	}
}

// The release notes in the dialog: for each version, a heading with its date; the
// headings of the notes in bold; their items, whose lines may be cut; bold, code
// and the links to web pages; and a link to the other ones when there are more.
func TestNotesBlocks(t *testing.T) {
	Lang = 0
	published := time.Date(2026, 11, 12, 12, 0, 0, 0, time.Local)
	md := "### New\n\n- **A** link: [the page](https://example.invalid/p), `code`,\n  [here](docs/x.md).\n- Two.\n\n### Fixed\n\nA paragraph\non two lines."
	got := notesBlocks([]releaseNotes{{version: "3.1.2", published: published, changes: md}, {version: "3.1.1", changes: "- One."}}, true)
	text := func(runs []mrun) string {
		var s []string
		for _, r := range runs {
			s = append(s, fmt.Sprintf("%d:%s>%s", r.kind, r.text, r.target))
		}
		return strings.Join(s, "|")
	}
	var lines []string
	for _, b := range got {
		lines = append(lines, fmt.Sprintf("%d %s", b.kind, text(b.runs)))
	}
	want := []string{
		fmt.Sprintf("%d %d:eMudScan 3.1.2 – 12 November 2026>", dkHeading, runText),
		fmt.Sprintf("%d %d:New>", dkPara, runBold),
		fmt.Sprintf("%d %d:A>|%d: link: >|%d:the page>https://example.invalid/p|%d:, >|%d:code>|%d:, >|%d:here>|%d:.>", dkBullet, runBold, runText, runLink, runText, runText, runText, runText, runText),
		fmt.Sprintf("%d %d:Two.>", dkBullet, runText),
		fmt.Sprintf("%d ", dkSpace),
		fmt.Sprintf("%d %d:Fixed>", dkPara, runBold),
		fmt.Sprintf("%d %d:A paragraph on two lines.>", dkPara, runText),
		fmt.Sprintf("%d %d:eMudScan 3.1.1>", dkHeading, runText),
		fmt.Sprintf("%d %d:One.>", dkBullet, runText),
		fmt.Sprintf("%d ", dkSpace),
		fmt.Sprintf("%d %d:The release notes of the other new versions are on >|%d:GitHub>%s/releases|%d:.>", dkPara, runText, runLink, appURL, runText),
	}
	if !reflect.DeepEqual(lines, want) {
		t.Errorf("got:\n%s\nwant:\n%s", strings.Join(lines, "\n"), strings.Join(want, "\n"))
	}
	if !strings.HasPrefix(blocksText(got), "eMudScan 3.1.2 – 12 November 2026\nNew\n• A link: the page — https://example.invalid/p") {
		t.Errorf("text:\n%s", blocksText(got))
	}
}

// A size in megabytes, in each language.
func TestMegabytes(t *testing.T) {
	defer func() { Lang = 0 }()
	Lang = 0
	if got := megabytes(7_759_000); got != "7.4 MB" {
		t.Errorf("en: %q", got)
	}
	Lang = 1
	if got := megabytes(7_759_000); got != "7,4\u00a0Mo" {
		t.Errorf("fr: %q", got)
	}
}

// updateLookup asks GitHub's API for the latest release, and updateNotes for the list
// of releases; updateDownload downloads its eMudScan.exe from github.com, at the
// address that eMudScan builds, tells how much has come, stops when asked, and checks
// it.
func TestUpdateLookupAndDownload(t *testing.T) {
	Lang = 0
	defer func(f func(string, string, int) ([]byte, bool)) { httpsGet = f }(httpsGet)
	defer func(f func(string, string, int, func(int) bool) ([]byte, bool)) { httpsGetWatched = f }(httpsGetWatched)
	data := []byte("new")
	sum := sumOf(data)
	var asked []string
	httpsGet = func(host, path string, max int) ([]byte, bool) {
		asked = append(asked, host+path)
		switch host + path {
		case "api.github.com/repos/3c0/eMudScan/releases/latest":
			return releaseJSON(t, "9.0.0", sum, nil), true
		case "api.github.com/repos/3c0/eMudScan/releases?per_page=30":
			return []byte(`[{"tag_name": "8.0.0", "body": "\n## In this version\n- Eight."}]`), true
		}
		return nil, false
	}
	served := data
	httpsGetWatched = func(host, path string, max int, watch func(int) bool) ([]byte, bool) {
		asked = append(asked, host+path)
		if host+path != "github.com/3c0/eMudScan/releases/download/9.0.0/eMudScan.exe" || len(served) >= max {
			return nil, false
		}
		for got := 1; got <= len(served); got++ {
			if !watch(got) {
				return nil, false
			}
		}
		return served, true
	}
	r, found, err := updateLookup()
	if !found || err != nil || r.version != "9.0.0" {
		t.Fatalf("%v %v %+v", found, err, r)
	}
	if notes, more, read := updateNotes(r); len(notes) != 2 || notes[1].version != "8.0.0" || more || !read {
		t.Errorf("notes: %+v %v %v", notes, more, read)
	}
	var seen []int
	got, err := updateDownload(r, func(n int) bool { seen = append(seen, n); return true })
	if err != nil || string(got) != "new" || len(asked) != 3 || !reflect.DeepEqual(seen, []int{1, 2, 3}) {
		t.Errorf("download: %q %v %v %v", got, err, asked, seen)
	}
	if _, err := updateDownload(r, func(n int) bool { return n < 2 }); err != errUpdateStopped {
		t.Errorf("stopped: %v", err)
	}
	for name, s := range map[string][]byte{"another file": []byte("old"), "shorter": []byte("ne"), "longer": []byte("newer")} {
		served = s
		if _, err := updateDownload(r, nil); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
	httpsGet = func(string, string, int) ([]byte, bool) { return nil, false }
	httpsGetWatched = func(string, string, int, func(int) bool) ([]byte, bool) { return nil, false }
	if _, found, err := updateLookup(); found || err == nil || err.Error() != T("update.noAnswer") {
		t.Errorf("no answer: %v %v", found, err)
	}
	if _, err := updateDownload(r, nil); err == nil || err.Error() != T("update.downloadFailed") {
		t.Errorf("no download: %v", err)
	}
	if reason(os.ErrNotExist) != "file does not exist" || reason(errString("Access is denied.")) != "Access is denied" {
		t.Error("reason")
	}
}

type errString string

func (e errString) Error() string { return string(e) }
