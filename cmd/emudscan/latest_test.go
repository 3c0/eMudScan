package main

import (
	"debug/pe"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const testSum = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// The list of SHA-256 of a release of python-build-standalone gives the build for
// 64-bit Windows of the minor version, with its URL: no other build, minor version or
// pre-release, and none that is older than the one written in eMudScan.
func TestPbsLatestIn(t *testing.T) {
	known := pbsSSL["3.11"]
	list := func(names ...string) string {
		var b strings.Builder
		for _, n := range names {
			b.WriteString(testSum + "  " + n + "\n")
		}
		return b.String()
	}
	others := []string{
		"cpython-3.11.19+20270105-aarch64-pc-windows-msvc-install_only_stripped.tar.gz",
		"cpython-3.11.19+20270105-x86_64-pc-windows-msvc-install_only.tar.gz",
		"cpython-3.11.19+20270105-x86_64-pc-windows-msvc-pgo-full.tar.zst",
		"cpython-3.11.19+20270105-x86_64-unknown-linux-gnu-install_only_stripped.tar.gz",
		"cpython-3.11.19+20270105-x86_64-pc-windows-msvc-freethreaded-install_only_stripped.tar.gz",
		"cpython-3.12.17+20270105-x86_64-pc-windows-msvc-install_only_stripped.tar.gz",
		"cpython-3.110.1+20270105-x86_64-pc-windows-msvc-install_only_stripped.tar.gz",
		"cpython-3.11.20rc1+20270105-x86_64-pc-windows-msvc-install_only_stripped.tar.gz",
	}
	good := "cpython-3.11.19+20270105-x86_64-pc-windows-msvc-install_only_stripped.tar.gz"
	got, ok := pbsLatestIn(list(append(append([]string{}, others...), good)...), known)
	want := pySSLSource{version: "3.11.19", sha256: testSum, tag: "20270105",
		url: "https://github.com/astral-sh/python-build-standalone/releases/download/20270105/cpython-3.11.19%2B20270105-x86_64-pc-windows-msvc-install_only_stripped.tar.gz"}
	if !ok || got != want || got.name() != "python-3.11.19-20270105-standalone" {
		t.Errorf("found: %v %+v", ok, got)
	}
	// the same release as the one written in eMudScan: the same source, with CRLF lines too
	same := strings.ReplaceAll(known.sha256+"  cpython-"+known.version+"+"+pbsTag+"-x86_64-pc-windows-msvc-install_only_stripped.tar.gz\n", "\n", "\r\n")
	if got, ok := pbsLatestIn(same, known); !ok || got != known {
		t.Errorf("same release: %v %+v", ok, got)
	}
	for name, l := range map[string]string{
		"no build of the minor version": list(others...),
		"an older release":              list("cpython-3.11.18+20261026-x86_64-pc-windows-msvc-install_only_stripped.tar.gz"),
		"an older version":              list("cpython-3.11.17+20270105-x86_64-pc-windows-msvc-install_only_stripped.tar.gz"),
		"not a list":                    "<html>Not Found</html>",
		"empty":                         "",
	} {
		if got, ok := pbsLatestIn(l, known); ok {
			t.Errorf("%s: %+v", name, got)
		}
	}
}

// pyorgLists is a copy of python.org's two lists, with their fields: the releases with
// their names and numbers, in the order of their names as texts, and the files of each
// release; a release in sourceOnly has no Windows files.
func pyorgLists(sourceOnly ...string) (releases []byte, filesOf func(string) ([]byte, bool), asked *[]string) {
	versions := []string{"3.13.18", "3.14.0", "3.14.10", "3.14.11", "3.14.12", "3.14.9", "3.15.2", "3.9.25"}
	var rel []string
	files := map[string]string{}
	for i, v := range versions {
		id := 1160 + i
		rel = append(rel, `{"name":"Python `+v+`","slug":"python-x","version":3,"is_published":true,"is_latest":false,"release_date":"2026-10-25T04:15:45Z","pre_release":false,"release_page":null,"release_notes_url":"","show_on_download_page":true,"resource_uri":"https://www.python.org/api/v2/downloads/release/`+strconv.Itoa(id)+`/"}`)
		f := []string{`{"name":"XZ compressed source tarball","os":"https://www.python.org/api/v2/downloads/os/3/","is_source":true,"url":"https://www.python.org/ftp/python/` + v + `/Python-` + v + `.tar.xz","md5_sum":"","sha256_sum":"` + testSum + `","filesize":1}`}
		only := false
		for _, s := range sourceOnly {
			only = only || s == v
		}
		if !only {
			for _, arch := range []string{"win32", "amd64", "arm64"} {
				f = append(f, `{"name":"Windows embeddable package","os":"https://www.python.org/api/v2/downloads/os/1/","is_source":false,"url":"https://www.python.org/ftp/python/`+v+`/python-`+v+`-embed-`+arch+`.zip","md5_sum":"","sha256_sum":"`+testSum[:60]+strings.Repeat(arch[len(arch)-1:], 4)+`","filesize":2}`)
			}
			f = append(f, `{"name":"Android embeddable package (aarch64)","url":"https://www.python.org/ftp/python/`+v+`/python-`+v+`-aarch64-linux-android.tar.gz","sha256_sum":"`+testSum+`"}`)
		}
		files[strconv.Itoa(id)] = "[" + strings.Join(f, ",") + "]"
	}
	rel = append(rel, `{"name":"Python 3.14.20rc1","pre_release":false,"resource_uri":"https://www.python.org/api/v2/downloads/release/999/"}`)
	asked = &[]string{}
	return []byte("[" + strings.Join(rel, ",") + "]"), func(id string) ([]byte, bool) {
		*asked = append(*asked, id)
		s, ok := files[id]
		return []byte(s), ok
	}, asked
}

// python.org's lists give the embeddable package for 64-bit Windows of the latest
// release of the minor version that has it: the latest release while it has Windows
// files; the last one that has them once the later ones are published as source code
// only; never a pre-release, another minor version, or a release older than the one
// written in eMudScan.
func TestPyorgLatestIn(t *testing.T) {
	known := pyorgSSL["3.14"]
	releases, filesOf, asked := pyorgLists()
	got, ok := pyorgLatestIn(releases, filesOf, known)
	want := pySSLSource{version: "3.14.12", url: "https://www.python.org/ftp/python/3.14.12/python-3.14.12-embed-amd64.zip", sha256: testSum[:60] + "4444", zip: true}
	if !ok || got != want || len(*asked) != 1 || got.name() != "python-3.14.12-embed" {
		t.Errorf("latest: %v %+v, asked %v", ok, got, *asked)
	}
	// 3.14.11 and 3.14.12 published as source code only: the package of 3.14.10
	releases, filesOf, asked = pyorgLists("3.14.11", "3.14.12")
	got, ok = pyorgLatestIn(releases, filesOf, known)
	if !ok || got.version != "3.14.10" || got.url != known.url || len(*asked) != 3 {
		t.Errorf("source only: %v %+v, asked %v", ok, got, *asked)
	}
	// no release with Windows files that is at least the one written in eMudScan
	releases, filesOf, asked = pyorgLists("3.14.10", "3.14.11", "3.14.12")
	if got, ok := pyorgLatestIn(releases, filesOf, known); ok || len(*asked) != 3 {
		t.Errorf("none: %+v, asked %v", got, *asked)
	}
	// the files of the latest release cannot be read: an older release is not taken for it
	releases, filesOf, _ = pyorgLists()
	if got, ok := pyorgLatestIn(releases, func(string) ([]byte, bool) { return nil, false }, known); ok {
		t.Errorf("files not read: %+v", got)
	}
	for name, data := range map[string]string{"not a list": "<html>", "empty": "", "an object": `{"detail":"Not found."}`} {
		if got, ok := pyorgLatestIn([]byte(data), filesOf, known); ok {
			t.Errorf("%s: %+v", name, got)
		}
	}
	// a package that is not on python.org, or without a SHA-256, is not one
	bad := func(id string) ([]byte, bool) {
		return []byte(`[{"url":"https://www.python.org.example.com/ftp/python/3.14.12/python-3.14.12-embed-amd64.zip","sha256_sum":"` + testSum + `"},{"url":"https://www.python.org/ftp/python/3.14.12/python-3.14.12-embed-amd64.zip","sha256_sum":""}]`), true
	}
	if got, ok := pyorgLatestIn(releases, bad, known); ok {
		t.Errorf("bad package: %+v", got)
	}
}

// latestPySSL asks GitHub for python-build-standalone's list and python.org for its
// lists, and nothing without a connection.
func TestLatestPySSL(t *testing.T) {
	defer func(f func(string, string, int) ([]byte, bool)) { httpsGet = f }(httpsGet)
	httpsGet = nil
	if _, ok := latestPySSL(pbsSSL["3.11"]); ok {
		t.Error("found without a connection")
	}
	releases, filesOf, _ := pyorgLists()
	var asked []string
	httpsGet = func(host, path string, max int) ([]byte, bool) {
		asked = append(asked, host+path)
		switch {
		case host == "github.com" && path == "/astral-sh/python-build-standalone/releases/latest/download/SHA256SUMS":
			return []byte(testSum + "  cpython-3.11.19+20270105-x86_64-pc-windows-msvc-install_only_stripped.tar.gz\n"), true
		case host == "www.python.org" && path == "/api/v2/downloads/release/?is_published=true&pre_release=false&version=3":
			return releases, true
		case host == "www.python.org" && strings.HasPrefix(path, "/api/v2/downloads/release_file/?release="):
			return filesOf(strings.TrimPrefix(path, "/api/v2/downloads/release_file/?release="))
		}
		return nil, false
	}
	if got, ok := latestPySSL(pbsSSL["3.11"]); !ok || got.version != "3.11.19" || got.tag != "20270105" {
		t.Errorf("python-build-standalone: %v %+v", ok, got)
	}
	if got, ok := latestPySSL(pyorgSSL["3.14"]); !ok || got.version != "3.14.12" || !got.zip {
		t.Errorf("python.org: %v %+v", ok, got)
	}
	// once for each source
	asked = nil
	look := latestLooker()
	look(pbsSSL["3.11"])
	look(pbsSSL["3.11"])
	if len(asked) != 1 {
		t.Errorf("asked %v", asked)
	}
}

// When a Fix window opens, the fix that replaces the ssl files of a Python is written
// again for the latest release found: its commands, simple, name that release, and the
// details show them. A release that is not found leaves the commands as they are, with
// a note that the Fix window shows; the same release changes nothing.
func TestUseLatest(t *testing.T) {
	Lang = 0
	root := t.TempDir()
	app := filepath.Join(root, "app")
	home := filepath.Join(app, "python")
	pyLayout(t, home, pe.IMAGE_FILE_MACHINE_AMD64, "DLLs", "libssl-3-x64.dll", "libcrypto-3-x64.dll", "_hashlib.pyd")
	sp := filepath.Join(home, "Lib", "site-packages")
	os.MkdirAll(filepath.Join(sp, "certifi"), 0o755)
	os.WriteFile(filepath.Join(sp, "certifi", "cacert.pem"), outdatedBundle(t), 0o644)
	tp := testApps(t)
	tp.owners = []ownerDir{{dir: app, name: "Example App 36", version: "1.0.3"}}
	exe := filepath.Join(home, "python.exe")
	it := tp.c.begin(GroupApps, "python", "Python", home, func(it *Item) {
		env := pythonEnvFor(exe)
		env.version = "3.11.9"
		tp.pythonOneEnv(it, env)
	})
	id := it.ID
	old, _ := tp.c.get(id)
	if len(old.Fixes) != 2 || old.Fixes[0].Kind != fixPySSL || old.Fixes[0].py == nil || !strings.Contains(old.Details, pbsSSL["3.11"].url) {
		t.Fatalf("fixes: %+v\n%s", old.Fixes, old.Details)
	}
	newer := pySSLSource{version: "3.11.19", sha256: testSum, tag: "20270105", url: pbsReleases + "download/20270105/cpython-3.11.19%2B20270105-x86_64-pc-windows-msvc-install_only_stripped.tar.gz"}
	found := func(s pySSLSource) (pySSLSource, bool) { return newer, s == pbsSSL["3.11"] }

	// the same release: nothing changes
	if changed := tp.c.useLatest([]int{id}, func(s pySSLSource) (pySSLSource, bool) { return s, true }, "not found"); len(changed) != 0 {
		t.Errorf("same release: %d changed", len(changed))
	}
	// not found: the same commands, with a note
	changed := tp.c.useLatest([]int{id}, func(pySSLSource) (pySSLSource, bool) { return pySSLSource{}, false }, T("fix.latestNotFound"))
	got, _ := tp.c.get(id)
	if len(changed) != 1 || got.Details != old.Details || got.Fixes[0].key() != old.Fixes[0].key() || got.Fixes[0].Note != T("fix.latestNotFound") || old.Fixes[0].Note != "" {
		t.Errorf("not found: %d changed, note %q", len(changed), got.Fixes[0].Note)
	}
	if lines := fixPreview([]Item{got}, true); !hasLine(lines, fixLine{lineComment, T("fix.latestNotFound")}) {
		t.Errorf("the Fix window does not show the note: %+v", lines)
	}
	// found: the commands of that release, in the fix and in the details, without the note
	changed = tp.c.useLatest([]int{id}, found, "not found")
	got, _ = tp.c.get(id)
	f := got.Fixes[0]
	all := strings.Join(f.Cmds, "\n")
	if len(changed) != 1 || f.Kind != fixPySSL || f.Note != "" || len(got.Fixes) != 2 || got.Fixes[1].Kind != fixPip ||
		!strings.Contains(all, "\n$url     = '"+newer.url+"'\n") || !strings.Contains(all, "\n$sha256  = '"+testSum+"'\n") ||
		!strings.Contains(all, `\python-3.11.19-20270105-standalone.tar.gz"`) || strings.Contains(all, pbsTag) || len(f.Cmds) != len(old.Fixes[0].Cmds) {
		t.Errorf("found: %d changed\n%s", len(changed), all)
	}
	if !strings.Contains(got.Details, f.lines()) || strings.Contains(got.Details, pbsSSL["3.11"].url) {
		t.Errorf("details:\n%s", got.Details)
	}
	if again := fixesIn(got.Details); len(again) != 2 || again[0].key() != f.key() || again[0].py == nil {
		t.Errorf("the details do not give the new fix: %+v", again)
	}
	// the commands look for nothing themselves: one download, of one URL
	for _, c := range f.Cmds {
		if strings.Contains(c, "SHA256SUMS") || strings.Contains(c, "Invoke-RestMethod") || strings.Contains(c, "try {") && strings.Contains(c, "Invoke-WebRequest") {
			t.Errorf("the command looks for a release: %s", c)
		}
	}
	// an element without such a fix, or already fixed, is left alone
	other := tp.c.add(GroupApps, "app", "Example App 37", result{Status: StOK, Summary: "Checked"})
	if changed := tp.c.useLatest([]int{other.ID, 9999}, found, "not found"); len(changed) != 0 {
		t.Errorf("other: %d changed", len(changed))
	}
}

func hasLine(lines []fixLine, l fixLine) bool {
	for _, x := range lines {
		if x == l {
			return true
		}
	}
	return false
}
