package main

// The latest release of what a fix downloads. The commands of a fix that replaces the
// ssl files of a Python (pysslfix.go) name one release, with its SHA-256. The scan,
// which connects to nothing, writes them for the release written in eMudScan. When a
// Fix window opens, eMudScan looks for the latest release of the same minor version
// and writes the commands again for it, so that the window shows, and runs, the
// commands of that release:
//
//   - python-build-standalone: in the list of SHA-256 of its latest release on GitHub
//     (SHA256SUMS; its releases are immutable);
//   - python.org: in its list of releases, then in the list of the files of a release,
//     with their SHA-256. A minor version that only gets security fixes has releases
//     without Windows files: the package is the one of its latest release that has it.
//
// A release that is found is used when it is at least the one written in eMudScan.
// When none is found (no connection, a list that changed), the commands stay those
// of the release written in eMudScan, and the window says so. These are connections
// of eMudScan itself, as the online test: when "Allow the online test" is cleared, it
// does not look, and the window says so too.

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// httpsGet gets https://host/path, following redirections, and gives at most max
// bytes of the answer; false when it could not, or when the answer is not a success.
// It is eMudScan's own connection, made with Windows' HTTPS (latest_windows.go); nil
// where there is none.
var httpsGet func(host, path string, max int) ([]byte, bool)

const (
	pbsListHost   = "github.com"
	pbsListPath   = "/astral-sh/python-build-standalone/releases/latest/download/SHA256SUMS"
	pyorgHost     = "www.python.org"
	pyorgReleases = "/api/v2/downloads/release/?is_published=true&pre_release=false&version=3"
	pyorgFilesOf  = "/api/v2/downloads/release_file/?release="
)

// minorOf is the minor version of a version: "3.11" for "3.11.18".
func minorOf(version string) string {
	return strings.Join(strings.SplitN(version, ".", 3)[:2], ".")
}

// pbsLatestIn finds, in the list of SHA-256 of a release of python-build-standalone,
// the build for 64-bit Windows of the minor version of known, the build written in
// eMudScan: its line is the SHA-256, two spaces and the name of the file, which has
// the version (three numbers: never a pre-release) and the release. It must be at
// least the version and the release of known.
func pbsLatestIn(list string, known pySSLSource) (pySSLSource, bool) {
	line := regexp.MustCompile(`^([0-9a-f]{64})  (cpython-(` + regexp.QuoteMeta(minorOf(known.version)) + `\.\d+)\+(\d{8})-x86_64-pc-windows-msvc-install_only_stripped\.tar\.gz)$`)
	for _, l := range strings.Split(list, "\n") {
		m := line.FindStringSubmatch(strings.TrimRight(l, "\r"))
		if m == nil {
			continue
		}
		if ParseVersion(m[3]).Cmp(ParseVersion(known.version)) < 0 || m[4] < known.tag {
			return pySSLSource{}, false
		}
		return pySSLSource{version: m[3], url: pbsReleases + "download/" + m[4] + "/" + strings.ReplaceAll(m[2], "+", "%2B"), sha256: m[1], tag: m[4]}, true
	}
	return pySSLSource{}, false
}

// pyorgLatestIn finds python.org's embeddable package for 64-bit Windows of the
// minor version of known, the package written in eMudScan. releases is python.org's
// list of releases; filesOf gives the list of the files of a release, by its number.
// The releases of the minor version that are at least the version of known (their
// name is "Python" and three numbers: never a pre-release) are looked at from the
// latest one, and the package is the one of the first release that has it: it must be
// on python.org, in the folder of its version, with a SHA-256.
func pyorgLatestIn(releases []byte, filesOf func(release string) ([]byte, bool), known pySSLSource) (pySSLSource, bool) {
	var list []struct {
		Name string `json:"name"`
		URI  string `json:"resource_uri"`
	}
	if json.Unmarshal(releases, &list) != nil {
		return pySSLSource{}, false
	}
	name := regexp.MustCompile(`^Python (` + regexp.QuoteMeta(minorOf(known.version)) + `\.\d+)$`)
	number := regexp.MustCompile(`/release/(\d+)/$`)
	type release struct{ version, number string }
	var found []release
	for _, r := range list {
		n, id := name.FindStringSubmatch(r.Name), number.FindStringSubmatch(r.URI)
		if n != nil && id != nil && ParseVersion(n[1]).Cmp(ParseVersion(known.version)) >= 0 {
			found = append(found, release{n[1], id[1]})
		}
	}
	slices.SortFunc(found, func(a, b release) int { return ParseVersion(b.version).Cmp(ParseVersion(a.version)) })
	sum := regexp.MustCompile(`^[0-9a-f]{64}$`)
	for _, r := range found {
		data, ok := filesOf(r.number)
		if !ok {
			return pySSLSource{}, false // not known: an older release must not be taken for the latest
		}
		var files []struct {
			URL    string `json:"url"`
			SHA256 string `json:"sha256_sum"`
		}
		if json.Unmarshal(data, &files) != nil {
			return pySSLSource{}, false
		}
		want := fmt.Sprintf("%s%s/python-%s-embed-amd64.zip", pyorgFiles, r.version, r.version)
		for _, f := range files {
			if f.URL == want && sum.MatchString(f.SHA256) {
				return pySSLSource{version: r.version, url: f.URL, sha256: f.SHA256, zip: true}, true
			}
		}
		// no Windows files: a release published as source code only; the one before
	}
	return pySSLSource{}, false
}

// latestPySSL looks online for the latest release of the source known, the one
// written in eMudScan; false when it finds none.
func latestPySSL(known pySSLSource) (pySSLSource, bool) {
	if httpsGet == nil {
		return pySSLSource{}, false
	}
	if !known.zip {
		list, ok := httpsGet(pbsListHost, pbsListPath, 2<<20)
		if !ok {
			return pySSLSource{}, false
		}
		return pbsLatestIn(string(list), known)
	}
	releases, ok := httpsGet(pyorgHost, pyorgReleases, 4<<20)
	if !ok {
		return pySSLSource{}, false
	}
	return pyorgLatestIn(releases, func(release string) ([]byte, bool) { return httpsGet(pyorgHost, pyorgFilesOf+release, 1<<20) }, known)
}

// latestLooker gives look, which is latestPySSL, once for each source: several
// Pythons of the same minor version share its answer.
func latestLooker() func(pySSLSource) (pySSLSource, bool) {
	type answer struct {
		src pySSLSource
		ok  bool
	}
	seen := map[string]answer{}
	return func(known pySSLSource) (pySSLSource, bool) {
		a, ok := seen[known.url]
		if !ok {
			a.src, a.ok = latestPySSL(known)
			seen[known.url] = a
		}
		return a.src, a.ok
	}
}

// useLatest makes again the fixes of it that download a release (fixAction.py) for
// the release that look finds for theirs, and writes their commands in its details
// in place of the old ones, so that the details show what the fix runs. A fix whose
// release is not found keeps its commands, with the note notFound, which says why. It
// tells whether it changed.
func useLatest(it *Item, look func(pySSLSource) (pySSLSource, bool), notFound string) bool {
	fixes, changed := slices.Clone(it.Fixes), false // the copies of the element keep theirs
	for i, f := range fixes {
		if f.py == nil {
			continue
		}
		src, ok := look(f.py.plan.src)
		if !ok {
			if f.Note != notFound {
				fixes[i].Note, changed = notFound, true
			}
			continue
		}
		nf := f.py.fixFrom(src)
		if nf.key() == f.key() && f.Note == "" {
			continue
		}
		if !strings.Contains(it.Details, f.lines()) {
			continue // not as the details show it: left as it is
		}
		it.Details = strings.Replace(it.Details, f.lines(), nf.lines(), 1)
		fixes[i], changed = newFix(nf), true
	}
	if changed {
		it.Fixes = fixes
	}
	return changed
}

// useLatest looks for the latest releases of what the fixes of the elements ids
// download, before a Fix window opens for them, and makes these fixes again for them
// (useLatest). The connections are made first, without holding the elements. It gives
// the elements that changed, which are reported as after a check.
func (c *checker) useLatest(ids []int, look func(pySSLSource) (pySSLSource, bool), notFound string) []Item {
	answers := map[string]struct {
		src pySSLSource
		ok  bool
	}{}
	for _, id := range ids {
		if it, ok := c.get(id); ok && !fixedStatus(it.Status) {
			for _, f := range it.Fixes {
				if f.py != nil {
					if _, done := answers[f.py.plan.src.url]; !done {
						src, ok := look(f.py.plan.src)
						answers[f.py.plan.src.url] = struct {
							src pySSLSource
							ok  bool
						}{src, ok}
					}
				}
			}
		}
	}
	if len(answers) == 0 {
		return nil
	}
	known := func(s pySSLSource) (pySSLSource, bool) { a := answers[s.url]; return a.src, a.ok }
	var out []Item
	for _, id := range ids {
		c.mu.Lock()
		it := c.items[id]
		changed := it != nil && !fixedStatus(it.Status) && useLatest(it, known, notFound)
		var cp Item
		if changed {
			cp = *it
		}
		c.mu.Unlock()
		if changed {
			c.r.Upsert(cp)
			out = append(out, cp)
		}
	}
	return out
}
