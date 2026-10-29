package main

// Microsoft Store apps. Their folders (C:\Program Files\WindowsApps\…) cannot be
// listed, so the drive scan skips them, but each one can be read: Windows records
// the folder of every package installed for the user in the registry. Apps written
// for the Store use Windows' own certificate checks; desktop programs repackaged for
// it keep theirs (Python from the Store, Electron applications…), so the folders are
// searched like any other, and what is found there belongs to the package.

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// storePackagesKey lists the packages installed for the user (values PackageRootFolder,
// DisplayName).
const storePackagesKey = `Software\Classes\Local Settings\Software\Microsoft\Windows\CurrentVersion\AppModel\Repository\Packages`

type storePackage struct {
	fullName string // PythonSoftwareFoundation.Python.3.13_3.13.3824.0_x64__qbz5n2kfra8p0
	name     string // its name, shown
	version  string // the package version
	root     string // its folder
}

// splitPackageFullName: Name_Version_Architecture_ResourceId_PublisherId.
func splitPackageFullName(full string) (name, version string) {
	parts := strings.Split(full, "_")
	if len(parts) < 2 {
		return full, ""
	}
	return parts[0], parts[1]
}

// appxManifest is the part of AppxManifest.xml that eMudScan reads.
type appxManifest struct {
	Properties struct {
		DisplayName     string `xml:"DisplayName"`
		Framework       bool   `xml:"Framework"`
		ResourcePackage bool   `xml:"ResourcePackage"`
	} `xml:"Properties"`
}

func readAppxManifest(root string) (appxManifest, bool) {
	var m appxManifest
	data, err := os.ReadFile(filepath.Join(root, "AppxManifest.xml"))
	if err != nil || len(data) > 4<<20 {
		return m, false
	}
	return m, xml.Unmarshal(data, &m) == nil
}

// plainName: a display name that is text, not a reference to the package's resources.
func plainName(s string) bool {
	s = strings.TrimSpace(s)
	return s != "" && !strings.HasPrefix(s, "@{") && !strings.HasPrefix(lower(s), "ms-resource:")
}

// storePackages lists the Store apps installed for the user, frameworks and
// resource packages (libraries shared by apps) and Windows' own apps excluded.
func (tp *appsContext) storePackages() []storePackage {
	var out []storePackage
	seen := map[string]bool{}
	for _, full := range regSubkeys(hkcu, storePackagesKey, view64) {
		root, _ := regString(hkcu, storePackagesKey+`\`+full, "PackageRootFolder", view64)
		if root == "" || seen[pathKey(root)] || tp.inWindowsDir(root) || !isDir(root) {
			continue
		}
		seen[pathKey(root)] = true
		m, _ := readAppxManifest(root)
		if m.Properties.Framework || m.Properties.ResourcePackage {
			continue
		}
		name, version := splitPackageFullName(full)
		shown := ""
		if d, ok := regString(hkcu, storePackagesKey+`\`+full, "DisplayName", view64); ok {
			if plainName(d) {
				shown = d
			} else if strings.HasPrefix(d, "@{") {
				shown = loadIndirectString(d) // the package's resources hold its name
			}
		}
		if !plainName(shown) && plainName(m.Properties.DisplayName) {
			shown = m.Properties.DisplayName
		}
		if !plainName(shown) {
			shown = name
		}
		out = append(out, storePackage{fullName: full, name: strings.TrimSpace(shown), version: version, root: root})
	}
	sort.Slice(out, func(i, j int) bool { return lower(out[i].name) < lower(out[j].name) })
	return out
}

// loadStoreApps reads the Store apps; what is found in their folders belongs to them.
func (tp *appsContext) loadStoreApps() {
	tp.store = tp.storePackages()
	tp.addStoreOwners()
}

func (tp *appsContext) addStoreOwners() {
	for _, p := range tp.store {
		tp.owners = append(tp.owners, ownerDir{dir: filepath.Clean(p.root), name: p.name, version: p.version})
	}
	sort.SliceStable(tp.owners, func(i, j int) bool { return len(tp.owners[i].dir) > len(tp.owners[j].dir) })
}

// walkStoreApps searches the Store apps' folders, on the selected drives, in one step
// of the log.
func (tp *appsContext) walkStoreApps() {
	var roots []string
	for _, p := range tp.store {
		if tp.inScope(p.root) {
			roots = append(roots, p.root)
		}
	}
	if len(roots) == 0 {
		return
	}
	tp.c.stepStart(Tf("log.store", len(roots)))
	dirs0, files0 := tp.d.dirs, tp.d.files
	for _, r := range roots {
		if tp.c.cancel() {
			return
		}
		tp.d.walk(tp.c, walkSpec{root: r, maxDepth: 12, explicit: true, quiet: true}, false)
	}
	tp.c.stepEnd(StOK, Tf("log.walkDone", tp.d.dirs-dirs0, tp.d.files-files0))
}
