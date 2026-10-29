package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSplitPackageFullName(t *testing.T) {
	name, ver := splitPackageFullName("PythonSoftwareFoundation.Python.3.13_3.13.3824.0_x64__qbz5n2kfra8p0")
	if name != "PythonSoftwareFoundation.Python.3.13" || ver != "3.13.3824.0" {
		t.Errorf("got %q %q", name, ver)
	}
}

func TestAppxManifest(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "AppxManifest.xml"), []byte(`<?xml version="1.0" encoding="utf-8"?>
<Package xmlns="http://schemas.microsoft.com/appx/manifest/foundation/windows10">
  <Identity Name="Microsoft.VCLibs.140.00" Version="14.0.33519.0"/>
  <Properties><Framework>true</Framework><DisplayName>ms-resource:AppName</DisplayName></Properties>
</Package>`), 0o644)
	m, ok := readAppxManifest(dir)
	if !ok || !m.Properties.Framework || plainName(m.Properties.DisplayName) {
		t.Errorf("manifest: ok %v %+v", ok, m)
	}
	for s, want := range map[string]bool{"Python 3.13": true, "@{X?ms-resource://X/DisplayName}": false, "ms-resource:AppName": false, " ": false} {
		if plainName(s) != want {
			t.Errorf("plainName(%q) = %v", s, !want)
		}
	}
}

// The folder of a Store app is searched like any other, in one step of the log, and
// what is found there belongs to the app.
func TestStoreAppsSearched(t *testing.T) {
	Lang = 0
	root := filepath.Join(t.TempDir(), "WindowsApps", "PythonSoftwareFoundation.Python.3.13_3.13.3824.0_x64__qbz5n2kfra8p0")
	fakePython(t, root, false)
	var lines []string
	tp := testApps(t)
	tp.c = newChecker(logReporter{&lines}, nil)
	tp.d = newDiscovery()
	tp.allDrives = true
	tp.store = []storePackage{{fullName: filepath.Base(root), name: "Python 3.13", version: "3.13.3824.0", root: root}}
	tp.addStoreOwners()
	tp.walkStoreApps()
	if !tp.d.pythonExes[filepath.Join(root, "python.exe")] {
		t.Errorf("python.exe of the Store app not found: %v", tp.d.pythonExes)
	}
	if got := strings.Join(lines, "\n"); strings.Count(got, "Scanning") != 1 || !strings.Contains(got, "Scanning 1 Microsoft Store apps") {
		t.Errorf("log:\n%s", got)
	}
	if o := tp.ownerOf(filepath.Join(root, "Lib", "site-packages", "certifi", "cacert.pem")); o.Name != "Python 3.13" || o.Version != "3.13.3824.0" {
		t.Errorf("owner %+v", o)
	}
}

// AppxManifest.xml is read without encoding/xml: the package's Properties only.
func TestAppxManifestProperties(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "AppxManifest.xml"), []byte(`<?xml version="1.0" encoding="utf-8"?>
<Package xmlns="http://schemas.microsoft.com/appx/manifest/foundation/windows10" xmlns:uap="http://schemas.microsoft.com/appx/manifest/uap/windows10">
  <Identity Name="Example.Notes" Publisher="CN=Example" Version="2.4.0.0"/>
  <Properties>
    <DisplayName>Notes &amp; Sketches</DisplayName>
    <PublisherDisplayName>Example</PublisherDisplayName>
  </Properties>
  <Applications><Application Id="App"><uap:VisualElements DisplayName="ms-resource:Other"/></Application></Applications>
</Package>`), 0o644)
	m, ok := readAppxManifest(dir)
	if !ok || m.Properties.DisplayName != "Notes & Sketches" || m.Properties.Framework || m.Properties.ResourcePackage {
		t.Errorf("manifest: ok %v %+v", ok, m)
	}
	os.WriteFile(filepath.Join(dir, "AppxManifest.xml"), []byte(`<Package><Properties><ResourcePackage>true</ResourcePackage></Properties></Package>`), 0o644)
	if m, ok := readAppxManifest(dir); !ok || !m.Properties.ResourcePackage {
		t.Errorf("resource package: ok %v %+v", ok, m)
	}
	if _, ok := readAppxManifest(t.TempDir()); ok {
		t.Error("no manifest read as one")
	}
}

// What is found in a Store app's folder is tagged [Microsoft Store], and its advice is
// the Store's own update; its files cannot be fixed by hand.
func TestStorePackageTag(t *testing.T) {
	Lang = 0
	root := filepath.Join(t.TempDir(), "WindowsApps", "PythonSoftwareFoundation.Python.3.13_3.13.3824.0_x64__qbz5n2kfra8p0")
	tp := &appsContext{scoopApps: map[string]scoopManifest{}}
	tp.store = []storePackage{{fullName: filepath.Base(root), name: "Python 3.13", version: "3.13.3824.0", root: root}}
	fixFile := T("fix.orDirectly") + "\n$ copy a b"
	it := Item{Group: GroupApps, Kind: "bundle", Name: "certifi", Owner: "Python 3.13", OwnerVer: "3.13.3824.0",
		File: filepath.Join(root, "Lib", "site-packages", "certifi", "cacert.pem"), Status: StWarn, Details: "Trusts them.\n\n" + fixFile}
	it.fillDisplay("")
	tp.packageAdvice(&it)
	if len(it.Packages) != 1 {
		t.Fatalf("packages: %+v", it.Packages)
	}
	p := it.Packages[0]
	if p.Manager != "Microsoft Store" || p.ID != "PythonSoftwareFoundation.Python.3.13" || p.Page != "https://apps.microsoft.com/search?query=Python+3.13" || p.Update != "" {
		t.Errorf("package: %+v", p)
	}
	if got := it.DisplayName(); got != "Python 3.13 3.13.3824.0 [Microsoft Store]" {
		t.Errorf("title: %q", got)
	}
	store, locked := strings.Index(it.Details, T("fix.store")), strings.Index(it.Details, T("fix.storeLocked"))
	if !(store > 0 && locked > store) || strings.Contains(it.Details, fixFile) || strings.Contains(it.Details, "$ ") {
		t.Errorf("advice (no fix by hand in a Store app's folder):\n%s", it.Details)
	}
	tp.packageAdvice(&it) // run again: the advice once
	if strings.Count(it.Details, T("fix.store")) != 1 {
		t.Errorf("advice repeated:\n%s", it.Details)
	}
	if other := (Item{Group: GroupApps, Owner: "Other", File: filepath.Join(filepath.Dir(root), "Other_1.0_x64__abc", "a.pem")}); tp.packagesOf(&other) != nil {
		t.Errorf("another app's folder tagged: %+v", tp.packagesOf(&other))
	}
}
