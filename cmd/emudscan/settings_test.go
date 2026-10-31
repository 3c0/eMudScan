package main

import (
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIniValue(t *testing.T) {
	f := filepath.Join(t.TempDir(), "pip.ini")
	os.WriteFile(f, []byte("; comment\n[global]\ntimeout = 60\ncert = C:\\certs\\old.pem\n[install]\n# cert = ignored\ncert: C:\\certs\\new.pem\n"), 0o644)
	if got := iniValue(f, "cert"); got != `C:\certs\new.pem` {
		t.Errorf("iniValue = %q", got)
	}
	if got := iniValue(f, "cafile"); got != "" {
		t.Errorf("missing key = %q", got)
	}
}

func TestSettingPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CERTS_HOME", home)
	t.Setenv("USERPROFILE", home)
	for in, want := range map[string]string{
		`"%CERTS_HOME%/ca.pem"`: filepath.Join(home, "ca.pem"),
		"~/certs/ca.pem":        filepath.Join(home, "certs", "ca.pem"),
		"ca.pem":                filepath.Join(home, "conf", "ca.pem"),
	} {
		if got := settingPath(in, filepath.Join(home, "conf")); got != want {
			t.Errorf("settingPath(%q) = %q, want %q", in, got, want)
		}
	}
}

// certSettings finds the environment variables and the pip, Git and npm settings;
// Git's own bundle, set by its system configuration, is left to the Git check.
func TestCertSettingsFound(t *testing.T) {
	dir := t.TempDir()
	up, ad := filepath.Join(dir, "me"), filepath.Join(dir, "me", "AppData", "Roaming")
	gitRoot := filepath.Join(dir, "Git")
	for _, d := range []string{filepath.Join(ad, "pip"), filepath.Join(gitRoot, "etc")} {
		os.MkdirAll(d, 0o755)
	}
	t.Setenv("USERPROFILE", up)
	t.Setenv("HOME", "")
	t.Setenv("APPDATA", ad)
	t.Setenv("ProgramData", filepath.Join(dir, "ProgramData"))
	t.Setenv("XDG_CONFIG_HOME", "")
	for _, v := range certEnvVars {
		t.Setenv(v.name, "")
	}
	t.Setenv("SSL_CERT_FILE", filepath.Join(dir, "corp.pem"))
	t.Setenv("NODE_EXTRA_CA_CERTS", filepath.Join(dir, "extra.pem"))
	os.WriteFile(filepath.Join(ad, "pip", "pip.ini"), []byte("[global]\ncert = "+filepath.Join(dir, "pip.pem")+"\n"), 0o644)
	os.WriteFile(filepath.Join(up, ".gitconfig"), []byte("[http]\n\tsslCAInfo = "+filepath.Join(dir, "git.pem")+"\n"), 0o644)
	os.WriteFile(filepath.Join(gitRoot, "etc", "gitconfig"), []byte("[http]\n\tsslCAInfo = "+filepath.Join(gitRoot, "mingw64", "etc", "ssl", "certs", "ca-bundle.crt")+"\n"), 0o644)
	os.WriteFile(filepath.Join(up, ".npmrc"), []byte("registry=https://registry.npmjs.org/\ncafile="+filepath.Join(dir, "npm.pem")+"\n"), 0o644)
	var got []string
	for _, s := range certSettings([]string{gitRoot}) {
		got = append(got, s.owner+"/"+s.label+"="+filepath.Base(s.target))
	}
	want := []string{"Environment variables/SSL_CERT_FILE=corp.pem", "Environment variables/NODE_EXTRA_CA_CERTS=extra.pem",
		"pip/cert (pip.ini)=pip.pem", "Git/http.sslCAInfo=git.pem", "npm/cafile (npmrc)=npm.pem"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("settings found:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// The file a setting points to decides: outdated, up to date, missing; a setting
// used only by Python is harmless when every Python found protects itself.
func TestSettingVerdicts(t *testing.T) {
	Lang = 0
	dir := t.TempDir()
	outdated := filepath.Join(dir, "outdated.pem")
	os.WriteFile(outdated, outdatedBundle(t), 0o644)
	fine := filepath.Join(dir, "corp-root.pem")
	os.WriteFile(fine, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: testChains[0].Leaf.Raw}), 0o644)
	run := func(tp *appsContext, s certSetting) *Item {
		return tp.c.begin(GroupApps, "setting", s.label, s.target, func(it *Item) { tp.settingOne(it, s) })
	}
	tp := testApps(t)
	tp.d = newDiscovery()
	cases := []struct {
		s       certSetting
		status  Status
		summary string
	}{
		{certSetting{label: "SSL_CERT_FILE", env: "SSL_CERT_FILE", target: outdated, users: "set.users.openssl", remove: "x"}, StWarn, "Points to outdated certificates"},
		{certSetting{label: "SSL_CERT_FILE", env: "SSL_CERT_FILE", target: fine, users: "set.users.openssl"}, StOK, "Points to up-to-date certificates"},
		{certSetting{label: "CURL_CA_BUNDLE", env: "CURL_CA_BUNDLE", target: filepath.Join(dir, "gone.pem"), users: "set.users.curl"}, StInfo, "Points to a missing file"},
		{certSetting{label: "NODE_EXTRA_CA_CERTS", env: "NODE_EXTRA_CA_CERTS", target: outdated, users: "set.users.node", adds: true}, StWarn, "Points to outdated certificates"},
		{certSetting{label: "SSL_CERT_DIR", env: "SSL_CERT_DIR", target: dir, users: "set.users.openssl", dir: true}, StWarn, "Points to outdated certificates"},
		// no Python found: a Python setting is not known to be harmless
		{certSetting{label: "REQUESTS_CA_BUNDLE", env: "REQUESTS_CA_BUNDLE", target: outdated, users: "set.users.requests", python: true}, StWarn, "Points to outdated certificates"},
	}
	for _, c := range cases {
		it := run(tp, c.s)
		if it.Status != c.status || it.Summary != c.summary {
			t.Errorf("%s -> %s: %v %q, want %v %q\n%s", c.s.label, filepath.Base(c.s.target), it.Status, it.Summary, c.status, c.summary, it.Details)
		}
	}
	if !tp.handled[pathKey(outdated)] {
		t.Errorf("the file of a setting must not be listed again as a certificate bundle")
	}
	// with a Python that refuses the compromised certificates itself
	home := filepath.Join(dir, "Python313")
	fakePython(t, home, true)
	tp.d.pythonExes[filepath.Join(home, "python.exe")] = true
	it := run(tp, certSetting{label: "REQUESTS_CA_BUNDLE", env: "REQUESTS_CA_BUNDLE", target: outdated, users: "set.users.requests", python: true})
	if it.Status != StInfo || it.Summary != "Outdated certificates, harmless to Python on this PC" {
		t.Errorf("Python setting with a protected Python: %v %q", it.Status, it.Summary)
	}
}
