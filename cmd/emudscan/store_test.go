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
