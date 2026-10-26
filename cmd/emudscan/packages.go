package main

// Package managers: Scoop, WinGet and Chocolatey. Only local files are read
// (the installed packages): nothing is sent anywhere.

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
)

// pkgRef is a package of a software in a package manager.
type pkgRef struct {
	Manager   string // Scoop, WinGet, Chocolatey
	ID        string // package name or identifier
	Page      string // page of the package
	Installed bool   // installed with this manager
	Update    string // command that updates it
	UpdateAll string // command that updates everything installed with this manager
}

func scoopPage(app string) string { return "https://scoop.sh/#/apps?q=" + url.QueryEscape(app) }
func wingetPage(id string) string { return "https://winstall.app/apps/" + url.PathEscape(id) }
func chocoPage(id string) string {
	return "https://community.chocolatey.org/packages/" + url.PathEscape(id)
}

type scoopManifest struct {
	Homepage string `json:"homepage"`
	Version  string `json:"version"`
	bucket   string
}

// scoopApp reads what Scoop knows about an installed app.
func (tp *appsContext) scoopApp(scoopRoot, app string) scoopManifest {
	key := pathKey(scoopRoot) + "|" + lower(app)
	if m, ok := tp.scoopApps[key]; ok {
		return m
	}
	var m scoopManifest
	cur := filepath.Join(scoopRoot, "apps", app, "current")
	if data, err := os.ReadFile(filepath.Join(cur, "manifest.json")); err == nil {
		json.Unmarshal(data, &m)
	}
	var inst struct {
		Bucket string `json:"bucket"`
	}
	if data, err := os.ReadFile(filepath.Join(cur, "install.json")); err == nil {
		json.Unmarshal(data, &inst)
	}
	m.bucket = inst.Bucket
	tp.scoopApps[key] = m
	return m
}

type chocoPkg struct {
	ID, Title, Version, URL string
}

// loadChocolatey reads the packages installed with Chocolatey.
func (tp *appsContext) loadChocolatey() {
	root := getenv("ChocolateyInstall")
	if root == "" {
		if pd := getenv("ProgramData"); pd != "" {
			root = filepath.Join(pd, "chocolatey")
		}
	}
	if root == "" {
		return
	}
	tp.chocoRoot = root
	specs, _ := filepath.Glob(filepath.Join(root, "lib", "*", "*.nuspec"))
	for _, p := range specs {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if pkg, ok := parseNuspec(string(data)); ok {
			tp.choco = append(tp.choco, pkg)
		}
	}
}

// chocoOwner: files inside a Chocolatey package folder belong to that package.
func (tp *appsContext) chocoOwner(p string) (ownerInfo, bool) {
	if tp.chocoRoot == "" {
		return ownerInfo{}, false
	}
	lib := filepath.Join(tp.chocoRoot, "lib")
	if !hasPathPrefix(p, lib) {
		return ownerInfo{}, false
	}
	rel := strings.TrimLeft(p[len(filepath.Clean(lib)):], `\/`)
	id := strings.FieldsFunc(rel, func(c rune) bool { return c == '\\' || c == '/' })
	if len(id) == 0 {
		return ownerInfo{}, false
	}
	for _, c := range tp.choco {
		if strings.EqualFold(c.ID, id[0]) {
			name := c.Title
			if name == "" {
				name = c.ID
			}
			return ownerInfo{Name: name, URL: c.URL, Loc: filepath.Join(lib, id[0]), Version: c.Version}, true
		}
	}
	return ownerInfo{}, false
}

// packagesOf finds the package manager a software was installed with (Scoop, WinGet
// portable packages, Chocolatey).
func (tp *appsContext) packagesOf(it *Item) []pkgRef {
	if it.Group == GroupWindows {
		return nil
	}
	path := firstNonEmpty(it.File, it.Folder)
	if m := scoopRe.FindStringSubmatch(path); m != nil {
		return []pkgRef{{"Scoop", m[2], scoopPage(m[2]), true, "scoop update " + m[2], "scoop update -a"}}
	}
	if m := wingetRe.FindStringSubmatch(path); m != nil {
		id := m[1]
		if i := strings.Index(id, "_Microsoft.Winget"); i > 0 {
			id = id[:i]
		}
		return []pkgRef{{"WinGet", id, wingetPage(id), true, "winget upgrade --id " + id + " --exact", "winget upgrade --all"}}
	}
	key := alnum(it.Program)
	for _, c := range tp.choco {
		inLib := path != "" && tp.chocoRoot != "" && hasPathPrefix(path, filepath.Join(tp.chocoRoot, "lib", c.ID))
		if inLib || (key != "" && (alnum(c.ID) == key || alnum(c.Title) == key)) {
			return []pkgRef{{"Chocolatey", c.ID, chocoPage(c.ID), true, "choco upgrade " + c.ID, "choco upgrade all"}}
		}
	}
	return nil
}

// packageAdvice is run on each finished element: it records the package manager the
// software was installed with and, when an update is needed, adds its update commands
// (before the instructions to fix a file by hand, which are the last resort).
func (tp *appsContext) packageAdvice(it *Item) {
	it.Packages = tp.packagesOf(it)
	if it.Status != StWarn && it.Status != StCaution && it.Status != StError {
		return
	}
	for _, p := range it.Packages {
		if strings.Contains(it.Details, cmd(p.Update)) {
			continue
		}
		one, all := Tf("fix.pkg", p.Manager), Tf("fix.pkgAll", p.Manager)
		if p.Manager == "Chocolatey" {
			one, all = Tf("fix.pkgAdmin", p.Manager), Tf("fix.pkgAllAdmin", p.Manager)
		}
		advice := one + "\n" + cmd(p.Update) + "\n\n" + all + "\n" + cmd(p.UpdateAll)
		details := strings.Trim(it.Details, "\n")
		cut := len(details)
		for _, k := range []string{"fix.directly", "fix.orDirectly"} {
			if i := strings.Index(details, T(k)); i >= 0 && i < cut {
				cut = i
			}
		}
		head, tail := strings.TrimRight(details[:cut], "\n"), details[cut:]
		it.Details = joinParas(head, advice, tail)
	}
}

// parseNuspec reads the id, title, version and project page of a Chocolatey package
// from its .nuspec file: a small XML file, read here without encoding/xml (which
// would add about 110 KB to eMudScan).
func parseNuspec(s string) (chocoPkg, bool) {
	meta, ok := xmlText(s, "metadata")
	if !ok {
		return chocoPkg{}, false
	}
	field := func(name string) string {
		t, _ := xmlText(meta, name)
		return xmlUnescape(t)
	}
	p := chocoPkg{ID: field("id"), Title: field("title"), Version: field("version"), URL: field("projectUrl")}
	return p, p.ID != ""
}

// xmlText returns what is between the first <name …> and the following </name>.
func xmlText(s, name string) (string, bool) {
	for i := 0; ; {
		j := strings.Index(s[i:], "<"+name)
		if j < 0 {
			return "", false
		}
		k := i + j + 1 + len(name)
		if k < len(s) && strings.IndexByte(">/ \t\r\n", s[k]) >= 0 {
			end := strings.IndexByte(s[k:], '>')
			if end < 0 {
				return "", false
			}
			if s[k+end-1] == '/' { // <name/>
				return "", true
			}
			body := s[k+end+1:]
			if c := strings.Index(body, "</"+name); c >= 0 {
				return body[:c], true
			}
			return "", false
		}
		i = k
	}
}

// xmlUnescape decodes the text of an XML element: CDATA and entities.
func xmlUnescape(t string) string {
	t = strings.TrimSpace(t)
	if inner, ok := strings.CutPrefix(t, "<![CDATA["); ok {
		return strings.TrimSpace(strings.TrimSuffix(inner, "]]>"))
	}
	if !strings.Contains(t, "&") {
		return t
	}
	var b strings.Builder
	for len(t) > 0 {
		i := strings.IndexByte(t, '&')
		j := strings.IndexByte(t[max(i, 0):], ';')
		if i < 0 || j < 0 {
			b.WriteString(t)
			break
		}
		b.WriteString(t[:i])
		ent := t[i+1 : i+j]
		t = t[i+j+1:]
		switch {
		case ent == "amp":
			b.WriteByte('&')
		case ent == "lt":
			b.WriteByte('<')
		case ent == "gt":
			b.WriteByte('>')
		case ent == "quot":
			b.WriteByte('"')
		case ent == "apos":
			b.WriteByte('\'')
		case strings.HasPrefix(ent, "#x") || strings.HasPrefix(ent, "#X"):
			if r, ok := charRef(ent[2:], 16); ok {
				b.WriteRune(r)
			}
		case strings.HasPrefix(ent, "#"):
			if r, ok := charRef(ent[1:], 10); ok {
				b.WriteRune(r)
			}
		default:
			b.WriteString("&" + ent + ";")
		}
	}
	return b.String()
}

// charRef decodes the number of a character reference (233 in &#233;, E9 in
// &#xE9;). It returns false when the number is not a Unicode code point.
func charRef(s string, base int) (rune, bool) {
	n, err := strconv.ParseUint(s, base, 32)
	if err != nil || n > unicode.MaxRune {
		return 0, false
	}
	return rune(n), true
}
