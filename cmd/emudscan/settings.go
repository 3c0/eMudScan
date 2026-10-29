package main

// Settings that point software to a certificate file of their own choosing:
// environment variables (SSL_CERT_FILE, REQUESTS_CA_BUNDLE…) and the settings of
// pip, Git and npm. Such a file replaces the software's own list (or, for
// NODE_EXTRA_CA_CERTS, adds to it), so an up-to-date curl, Git or Node.js still
// trusts the compromised intermediates when the file is outdated. eMudScan only
// reads these settings; the file they point to is tested against the advisory's
// test chains like any certificate bundle.

import (
	"bufio"
	"crypto/x509"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// certSetting is one setting found.
type certSetting struct {
	label  string // shown as the component: SSL_CERT_FILE, cert (pip.ini)…
	owner  string // shown as the software: the environment, pip, Git, npm
	config string // the configuration file it is in ("" for an environment variable)
	env    string // the environment variable, if it is one
	target string // the file (or folder, for SSL_CERT_DIR) it points to
	users  string // text key: the software that uses it
	adds   bool   // the certificates are added to the software's own list
	dir    bool   // target is a folder of certificates
	python bool   // used only by Python software (harmless with a Python that blocks)
	remove string // how to remove the setting (a command), "" when it is a line to delete
	admin  bool   // removing it needs administrator rights
}

// certEnvVars: environment variables that make software trust a certificate file.
var certEnvVars = []struct {
	name, users   string
	adds, dir, py bool
}{
	{"SSL_CERT_FILE", "set.users.openssl", false, false, false},
	{"SSL_CERT_DIR", "set.users.openssl", false, true, false},
	{"CURL_CA_BUNDLE", "set.users.curl", false, false, false},
	{"REQUESTS_CA_BUNDLE", "set.users.requests", false, false, true},
	{"PIP_CERT", "set.users.pip", false, false, true},
	{"GIT_SSL_CAINFO", "set.users.git", false, false, false},
	{"NODE_EXTRA_CA_CERTS", "set.users.node", true, false, false},
	{"NPM_CONFIG_CAFILE", "set.users.npm", false, false, false},
}

// certSettings lists the settings found on this PC. gitRoots are the Git for Windows
// installations (their own bundle, set by their system configuration, is checked
// with Git itself).
func certSettings(gitRoots []string) []certSetting {
	var out []certSetting
	for _, v := range certEnvVars {
		val := strings.TrimSpace(getenv(v.name))
		if val == "" {
			continue
		}
		scope := "User"
		if _, ok := regString(hkcu, `Environment`, v.name, view64); !ok {
			if _, ok := regString(hklm, `SYSTEM\CurrentControlSet\Control\Session Manager\Environment`, v.name, view64); ok {
				scope = "Machine"
			}
		}
		out = append(out, certSetting{label: v.name, owner: T("set.env"), env: v.name, target: settingPath(val, ""),
			users: v.users, adds: v.adds, dir: v.dir, python: v.py, admin: scope == "Machine",
			remove: fmt.Sprintf(`[Environment]::SetEnvironmentVariable('%s', $null, '%s')`, v.name, scope)})
	}
	up, ad, pd := getenv("USERPROFILE"), getenv("APPDATA"), getenv("ProgramData")
	under := func(base string, elem ...string) []string { // the file, when its base folder is known
		if base == "" {
			return nil
		}
		return []string{filepath.Join(append([]string{base}, elem...)...)}
	}
	// pip: pip.ini, cert = <file>
	var pipFiles []string
	pipFiles = append(pipFiles, under(ad, "pip", "pip.ini")...)
	pipFiles = append(pipFiles, under(up, "pip", "pip.ini")...)
	pipFiles = append(pipFiles, under(pd, "pip", "pip.ini")...)
	for _, f := range pipFiles {
		if v := iniValue(f, "cert"); v != "" {
			out = append(out, certSetting{label: "cert (pip.ini)", owner: "pip", config: f, target: settingPath(v, filepath.Dir(f)),
				users: "set.users.pip", python: true})
		}
	}
	// Git: http.sslCAInfo in the global configuration (the system one points to Git's own bundle)
	var gitFiles []string
	if pd != "" {
		gitFiles = append(gitFiles, filepath.Join(pd, "Git", "config"))
	}
	if x := getenv("XDG_CONFIG_HOME"); x != "" {
		gitFiles = append(gitFiles, filepath.Join(x, "git", "config"))
	} else if up != "" {
		gitFiles = append(gitFiles, filepath.Join(up, ".config", "git", "config"))
	}
	if h := firstNonEmpty(getenv("HOME"), up); h != "" {
		gitFiles = append(gitFiles, filepath.Join(h, ".gitconfig"))
	}
	for _, root := range gitRoots {
		gitFiles = append(gitFiles, filepath.Join(root, "etc", "gitconfig"), filepath.Join(root, "mingw64", "etc", "gitconfig"))
	}
	for _, f := range gitFiles {
		v := gitConfigValue(f, "http", "sslcainfo")
		if v == "" || strings.HasPrefix(v, "/") && !filepath.IsAbs(v) { // /mingw64/…: inside Git, checked with it
			continue
		}
		p := settingPath(v, filepath.Dir(f))
		inGit := false
		for _, root := range gitRoots {
			inGit = inGit || hasPathPrefix(p, root)
		}
		if !inGit {
			s := certSetting{label: "http.sslCAInfo", owner: "Git", config: f, target: p, users: "set.users.git"}
			if up != "" && hasPathPrefix(f, up) { // the global configuration
				s.remove = `git config --global --unset http.sslCAInfo`
			}
			out = append(out, s)
		}
	}
	// npm: .npmrc, cafile = <file>
	for _, f := range append(under(up, ".npmrc"), under(ad, "npm", "etc", "npmrc")...) {
		if v := iniValue(f, "cafile"); v != "" {
			out = append(out, certSetting{label: "cafile (npmrc)", owner: "npm", config: f, target: settingPath(v, filepath.Dir(f)),
				users: "set.users.npm", remove: `npm config delete cafile`})
		}
	}
	return out
}

// settingPath makes a setting's value a path: quotes removed, %VARIABLES% and ~
// expanded, relative to base.
func settingPath(v, base string) string {
	v = strings.Trim(strings.TrimSpace(v), `"'`)
	var b strings.Builder
	for {
		i := strings.IndexByte(v, '%')
		j := -1
		if i >= 0 {
			j = strings.IndexByte(v[i+1:], '%')
		}
		if i < 0 || j < 0 {
			break
		}
		name := v[i+1 : i+1+j]
		b.WriteString(v[:i])
		if val := getenv(name); val != "" {
			b.WriteString(val)
		} else {
			b.WriteString("%" + name + "%")
		}
		v = v[i+j+2:]
	}
	b.WriteString(v)
	p := b.String()
	if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		p = filepath.Join(getenv("USERPROFILE"), p[2:])
	}
	p = filepath.FromSlash(p)
	if base != "" && !filepath.IsAbs(p) && !(len(p) > 1 && p[1] == ':') {
		p = filepath.Join(base, p)
	}
	return filepath.Clean(p)
}

// iniValue reads key from an INI-like file (pip.ini, .npmrc): key = value or key: value,
// in any section. The last one found wins, as with pip and npm.
func iniValue(file, key string) string {
	f, err := os.Open(file)
	if err != nil {
		return ""
	}
	defer f.Close()
	val := ""
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] == '#' || line[0] == ';' || line[0] == '[' {
			continue
		}
		i := strings.IndexAny(line, "=:")
		if i > 0 && lower(strings.TrimSpace(line[:i])) == key {
			// "C:\…" contains ':': a key is followed by its separator, so the first one splits
			val = strings.TrimSpace(line[i+1:])
		}
	}
	return val
}

// certsOf reads the certificates a setting points to: a PEM file, or every PEM or
// DER file of a folder.
func certsOf(target string, dir bool) ([]*x509.Certificate, error) {
	read := func(p string) []*x509.Certificate {
		data, err := os.ReadFile(p)
		if err != nil || len(data) > 32<<20 {
			return nil
		}
		if certs := parsePEMBundle(data); len(certs) > 0 {
			return certs
		}
		if c, err := x509.ParseCertificate(data); err == nil {
			return []*x509.Certificate{c}
		}
		return nil
	}
	if !dir {
		if _, err := os.Stat(target); err != nil {
			return nil, err
		}
		return read(target), nil
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		return nil, err
	}
	var certs []*x509.Certificate
	for i, e := range entries {
		if i >= 2000 {
			break
		}
		if !e.IsDir() {
			certs = append(certs, read(filepath.Join(target, e.Name()))...)
		}
	}
	return certs, nil
}

func (tp *appsContext) checkCertSettings() {
	settings := certSettings(dedupePaths(tp.d.gitRoots))
	sort.SliceStable(settings, func(i, j int) bool { return settings[i].owner < settings[j].owner })
	for _, s := range settings {
		s := s
		tp.c.begin(GroupApps, "setting", s.label, firstNonEmpty(s.config, s.target), func(it *Item) { tp.settingOne(it, s) })
	}
}

// pythonsProtected: every Python found refuses the compromised certificates itself.
func (tp *appsContext) pythonsProtected() bool {
	found := false
	for _, exe := range dedupePaths(tp.d.pythonExes) {
		env := pythonEnvFor(exe)
		if env.venv {
			continue
		}
		found = true
		if !tp.pythonFix(env).protected() {
			return false
		}
	}
	return found
}

func (tp *appsContext) settingOne(it *Item, s certSetting) {
	r := result{Owner: s.owner, Component: s.label, File: s.target, Search: s.label}
	if s.config != "" {
		r.Folder = filepath.Dir(s.config)
	}
	where := Tf("set.envWhere", s.env)
	if s.config != "" {
		where = Tf("set.fileWhere", lk(s.config))
	}
	what := Tf("set.replaces", where, lk(s.target), T(s.users))
	switch {
	case s.adds:
		what = Tf("set.adds", where, lk(s.target), T(s.users))
	case s.dir:
		what = Tf("set.replacesDir", where, lk(s.target), T(s.users))
	}
	certs, err := certsOf(s.target, s.dir)
	if !s.dir {
		tp.handled[pathKey(s.target)] = true // reported here, not again as a certificate bundle
	}
	removal := T("set.removeLine")
	if s.remove != "" {
		header := T("fix.ps")
		if s.admin {
			header = T("fix.psAdmin")
		}
		removal = T("set.remove") + "\n" + header + "\n" + cmd(s.remove)
	}
	switch {
	case err != nil:
		r.Status, r.Summary = StInfo, T("set.sumMissing")
		r.Details = joinParas(what, T("set.missing"), removal)
	case len(certs) == 0:
		r.Status, r.Summary = StInfo, T("set.sumEmpty")
		r.Details = joinParas(what, T("set.empty"))
	default:
		br := checkCertList(certs)
		if !br.Bad() {
			r.Status, r.Summary = StOK, T("set.sumOK")
			r.Details = joinParas(what, T("set.ok"))
			break
		}
		fix := T("set.fix")
		if !s.dir {
			fix = joinParas(fix, T("set.fixPublic")+"\n"+replaceBundleFix(s.target))
		}
		fix = joinParas(fix, T("set.fixOwn"), removal)
		if s.python && tp.pythonsProtected() {
			r.Status, r.Summary = StInfo, T("set.sumPySafe")
			r.Details = joinParas(what, bundleProblem(br), T("set.pySafe"), T("py.updateAnyway")+" "+fix)
			break
		}
		r.Status, r.Summary = StWarn, T("set.sumBad")
		r.Details = joinParas(what, bundleProblem(br), fix)
	}
	tp.c.finish(it, r)
}
