package main

import (
	"os"
	"path/filepath"
	"testing"
)

// Scoop's "current" folder is a link to the folder of the installed version: a program
// reached through it (PATH) and through its version folder (the drive scan) is one
// program, shown under its version folder.
func TestScoopCurrentIsTheVersion(t *testing.T) {
	scoop := filepath.Join(t.TempDir(), "scoop")
	app := filepath.Join(scoop, "apps", "nodejs-lts")
	ver := filepath.Join(app, "24.22.1")
	os.MkdirAll(ver, 0o755)
	os.WriteFile(filepath.Join(ver, "node.exe"), []byte("MZ"), 0o644)
	// the name recent versions of Scoop give it (older ones: manifest.json)
	os.WriteFile(filepath.Join(ver, "scoop-manifest.json"), []byte(`{"version":"24.22.1","homepage":"https://nodejs.org"}`), 0o644)
	if err := os.Symlink(ver, filepath.Join(app, "current")); err != nil {
		t.Skip("no links here:", err)
	}
	// an app whose "current" is a plain folder, not a link: left as it is
	plain := filepath.Join(scoop, "apps", "plain", "current")
	os.MkdirAll(plain, 0o755)

	tp := &appsContext{scoopApps: map[string]scoopManifest{}, handled: map[string]bool{}}
	cur := filepath.Join(app, "current", "node.exe")
	for in, want := range map[string]string{
		cur:                                    filepath.Join(ver, "node.exe"),
		filepath.Join(app, "current"):          ver,
		filepath.Join(ver, "node.exe"):         filepath.Join(ver, "node.exe"),
		filepath.Join(plain, "x.exe"):          filepath.Join(plain, "x.exe"),
		filepath.Join(scoop, "shims", "a.exe"): filepath.Join(scoop, "shims", "a.exe"),
	} {
		if got := tp.scoopReal(in); got != want {
			t.Errorf("scoopReal(%s) = %s, want %s", in, got, want)
		}
	}

	d := newDiscovery()
	d.nodeExes[cur] = true
	d.nodeExes[filepath.Join(ver, "node.exe")] = true
	d.rewrite(tp.scoopReal)
	if len(d.nodeExes) != 1 || !d.nodeExes[filepath.Join(ver, "node.exe")] {
		t.Errorf("found once, under its version: %v", d.nodeExes)
	}

	if o := tp.ownerOf(cur); o.Name != "nodejs-lts" || o.Version != "24.22.1" || o.Loc != ver || o.URL != "https://nodejs.org" {
		t.Errorf("owner %+v", o)
	}
	tp.markHandled(filepath.Join(app, "current", "cacert.pem"))
	if !tp.isHandled(filepath.Join(ver, "cacert.pem")) {
		t.Error("a bundle reported through current is reported again through its version")
	}
}

// Without a version in the manifest, a file kept apart from the versions (persist) gets
// the version of the folder "current" links to.
func TestScoopVersionFromLink(t *testing.T) {
	scoop := filepath.Join(t.TempDir(), "scoop")
	ver := filepath.Join(scoop, "apps", "uv", "0.11.3")
	os.MkdirAll(ver, 0o755)
	if err := os.Symlink(ver, filepath.Join(scoop, "apps", "uv", "current")); err != nil {
		t.Skip("no links here:", err)
	}
	tp := &appsContext{scoopApps: map[string]scoopManifest{}}
	o := tp.ownerOf(filepath.Join(scoop, "persist", "uv", "python", "cpython-3.12.16", "python.exe"))
	if o.Name != "uv" || o.Version != "0.11.3" || o.Loc != ver {
		t.Errorf("owner %+v", o)
	}
}

// When the link cannot be followed, "current" is the folder named after the version
// its manifest gives (Scoop installs each version in a folder of that name).
func TestScoopCurrentFromManifest(t *testing.T) {
	app := filepath.Join(t.TempDir(), "scoop", "apps", "uv")
	for _, d := range []string{"current", "0.13.5", "0.13.6"} {
		os.MkdirAll(filepath.Join(app, d), 0o755)
	}
	os.WriteFile(filepath.Join(app, "current", "scoop-manifest.json"), []byte(`{"version":"0.13.6","homepage":"https://docs.astral.sh/uv/"}`), 0o644)
	tp := &appsContext{scoopApps: map[string]scoopManifest{}}
	if got, want := tp.scoopReal(filepath.Join(app, "current", "uv.exe")), filepath.Join(app, "0.13.6", "uv.exe"); got != want {
		t.Errorf("got %s, want %s", got, want)
	}
	if o := tp.ownerOf(filepath.Join(app, "current", "uv.exe")); o.Version != "0.13.6" || o.URL != "https://docs.astral.sh/uv/" {
		t.Errorf("owner %+v", o)
	}
}
