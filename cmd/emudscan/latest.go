package main

// The latest release of what a fix downloads. The commands of a fix that replaces the
// ssl files of a Python (pysslfix.go), or a node.exe (nodefix.go), name one release,
// with its SHA-256. The scan, which connects to nothing, writes them for the release
// written in eMudScan. When a Fix window opens, eMudScan looks for the latest release of
// the same line (the same minor version of Python, the same major version of Node.js)
// and writes the commands again for it, so that the window shows, and runs, the
// commands of that release:
//
//   - python-build-standalone: in the list of SHA-256 of its latest release on GitHub
//     (SHA256SUMS; its releases are immutable);
//   - python.org: in its list of releases, then in the list of the files of a release,
//     with their SHA-256. A minor version that only gets security fixes has releases
//     without Windows files: the package is the one of its latest release that has it;
//   - nodejs.org: in its list of releases (index.json), then in the list of SHA-256 of
//     that release (SHASUMS256.txt), which has its node.exe for each processor.
//
// A release that is found is used when it is at least the one written in eMudScan.
// When none is found (no connection, a list that changed), the commands stay those
// of the release written in eMudScan, and the window says so. These are connections
// of eMudScan itself, as the online test: when "Online checks and updates" is
// cleared, it does not look, and the window says so too.

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

// httpsGetWatched is httpsGet, which tells watch how many bytes have come, as they
// come; when watch gives false, it stops, and fails.
var httpsGetWatched func(host, path string, max int, watch func(got int) bool) ([]byte, bool)

const (
	pbsListHost   = "github.com"
	pbsListPath   = "/astral-sh/python-build-standalone/releases/latest/download/SHA256SUMS"
	pyorgHost     = "www.python.org"
	pyorgReleases = "/api/v2/downloads/release/?is_published=true&pre_release=false&version=3"
	pyorgFilesOf  = "/api/v2/downloads/release_file/?release="
	nodeHost      = "nodejs.org"
	nodeIndex     = "/dist/index.json"
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

// nodeLatestIn finds, in nodejs.org's list of releases (index.json), the latest release
// of the line of known, the node.exe written in eMudScan: its version, which must be at
// least known's. A release is named "v" and three numbers: never a pre-release.
func nodeLatestIn(index []byte, known nodeSource) (string, bool) {
	var list []struct {
		Version string `json:"version"`
	}
	if json.Unmarshal(index, &list) != nil {
		return "", false
	}
	line := regexp.MustCompile(`^v(` + regexp.QuoteMeta(strings.SplitN(known.version, ".", 2)[0]) + `\.\d+\.\d+)$`)
	latest := ""
	for _, r := range list {
		if m := line.FindStringSubmatch(r.Version); m != nil && (latest == "" || ParseVersion(m[1]).Cmp(ParseVersion(latest)) > 0) {
			latest = m[1]
		}
	}
	if latest == "" || ParseVersion(latest).Cmp(ParseVersion(known.version)) < 0 {
		return "", false
	}
	return latest, true
}

// nodeSumIn finds, in nodejs.org's list of SHA-256 of a release (SHASUMS256.txt), the
// one of its node.exe for arch: its line is the SHA-256, two spaces, and the name of
// the file, such as "win-x64/node.exe".
func nodeSumIn(sums, arch string) (string, bool) {
	line := regexp.MustCompile(`^([0-9a-f]{64})  ` + regexp.QuoteMeta(arch) + `/node\.exe$`)
	for _, l := range strings.Split(sums, "\n") {
		if m := line.FindStringSubmatch(strings.TrimRight(l, "\r")); m != nil {
			return m[1], true
		}
	}
	return "", false
}

// latestNode looks online for the latest release of the line of known, the node.exe
// written in eMudScan, for the same processor; false when it finds none, or a release
// without a node.exe for that processor. When it is known's release, its SHA-256 is
// already known.
func latestNode(known nodeSource) (nodeSource, bool) {
	if httpsGet == nil {
		return nodeSource{}, false
	}
	index, ok := httpsGet(nodeHost, nodeIndex, 8<<20)
	if !ok {
		return nodeSource{}, false
	}
	version, ok := nodeLatestIn(index, known)
	if !ok {
		return nodeSource{}, false
	}
	if version == known.version {
		return known, true
	}
	sums, ok := httpsGet(nodeHost, "/dist/v"+version+"/SHASUMS256.txt", 1<<20)
	if !ok {
		return nodeSource{}, false
	}
	sum, ok := nodeSumIn(string(sums), known.arch)
	if !ok {
		return nodeSource{}, false
	}
	return nodeSource{version: version, arch: known.arch, sha256: sum}, true
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

// latestLook gives the latest release of what fixes download, by kind of download
// (latestPySSL, latestNode); false when it finds none.
type latestLook struct {
	py   func(pySSLSource) (pySSLSource, bool)
	node func(nodeSource) (nodeSource, bool)
}

// notLooking is the latestLook of a Fix window that does not look for the latest
// releases ("Online checks and updates" cleared): it finds none.
var notLooking = latestLook{
	py:   func(pySSLSource) (pySSLSource, bool) { return pySSLSource{}, false },
	node: func(nodeSource) (nodeSource, bool) { return nodeSource{}, false },
}

// latestLooker gives a latestLook that looks online (latestPySSL, latestNode), once for
// each release: the fixes of the same release share its answer.
func latestLooker() latestLook { return latestLook{py: latestPySSL, node: latestNode}.once() }

// once is look, which looks once for each release, by its URL.
func (look latestLook) once() latestLook {
	return latestLook{py: onceEach(look.py, func(s pySSLSource) string { return s.url }), node: onceEach(look.node, nodeSource.url)}
}

// onceEach is look, which gives the answer of its first call for each key of what it is
// asked for.
func onceEach[T any](look func(T) (T, bool), key func(T) string) func(T) (T, bool) {
	type answer struct {
		found T
		ok    bool
	}
	seen := map[string]answer{}
	return func(known T) (T, bool) {
		a, done := seen[key(known)]
		if !done {
			a.found, a.ok = look(known)
			seen[key(known)] = a
		}
		return a.found, a.ok
	}
}

// latest is the fix f made again for the latest release that look finds for what it
// downloads; false when it finds none. download: f downloads a release that is looked
// for (fixAction.py or node).
func (look latestLook) latest(f fixAction) (nf fixAction, download, ok bool) {
	switch {
	case f.py != nil:
		src, ok := look.py(f.py.plan.src)
		if ok {
			nf = f.py.fixFrom(src)
		}
		return nf, true, ok
	case f.node != nil:
		src, ok := look.node(f.node.src)
		if ok {
			nf = f.node.fixFrom(src)
		}
		return nf, true, ok
	}
	return nf, false, false
}

// useLatest makes again the fixes of it that download a release (fixAction.py or node)
// for the release that look finds for theirs, and writes their commands in its details
// in place of the old ones, so that the details show what the fix runs. A fix whose
// release is not found keeps its commands, with the note notFound, which says why. It
// tells whether it changed.
func useLatest(it *Item, look latestLook, notFound string) bool {
	fixes, changed := slices.Clone(it.Fixes), false // the copies of the element keep theirs
	for i, f := range fixes {
		nf, download, ok := look.latest(f)
		if !download {
			continue
		}
		if !ok {
			if f.Note != notFound {
				fixes[i].Note, changed = notFound, true
			}
			continue
		}
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
// (useLatest). The connections are made first, once for each release, without holding
// the elements. It gives the elements that changed, which are reported as after a check.
func (c *checker) useLatest(ids []int, look latestLook, notFound string) []Item {
	look = look.once()
	asked := false
	for _, id := range ids {
		if it, ok := c.get(id); ok && !fixedStatus(it.Status) {
			for _, f := range it.Fixes {
				_, download, _ := look.latest(f)
				asked = asked || download
			}
		}
	}
	if !asked {
		return nil
	}
	var out []Item
	for _, id := range ids {
		c.mu.Lock()
		it := c.items[id]
		changed := it != nil && !fixedStatus(it.Status) && useLatest(it, look, notFound)
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
