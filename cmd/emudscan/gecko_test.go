package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// oneCRLFor builds a OneCRL list blocking the compromised TLS certificates, half by
// issuer and serial number, half by subject and key.
func oneCRLFor(t *testing.T) []byte {
	b64 := base64.StdEncoding.EncodeToString
	var entries []map[string]string
	entries = append(entries, map[string]string{"issuerName": "AAAA", "serialNumber": "AQ=="}) // unrelated
	for i, c := range badCerts {
		if c.Usage != "tls" {
			continue
		}
		if i%2 == 0 {
			s := c.Cert.SerialNumber.Bytes()
			if s[0]&0x80 != 0 {
				s = append([]byte{0}, s...)
			}
			entries = append(entries, map[string]string{"issuerName": b64(c.Cert.RawIssuer), "serialNumber": b64(s)})
		} else {
			h := sha256.Sum256(c.Cert.RawSubjectPublicKeyInfo)
			entries = append(entries, map[string]string{"subject": b64(c.Cert.RawSubject), "pubKeyHash": b64(h[:])})
		}
	}
	raw, err := json.Marshal(map[string]any{"data": entries, "timestamp": 1792274400000})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func writeJar(t *testing.T, path string, files map[string][]byte) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, data := range files {
		w, _ := zw.Create(name)
		w.Write(data)
	}
	zw.Close()
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, buf.Bytes(), 0o644)
	return buf.Bytes()
}

func TestGeckoOneCRL(t *testing.T) {
	dir := t.TempDir()
	jar := writeJar(t, filepath.Join(dir, "omni.ja"), map[string][]byte{"defaults/settings/main/x.json": []byte("{}"), oneCRLEntry: oneCRLFor(t)})
	if b, n, ok := geckoOneCRL(dir); !ok || n != 6 || b != 6 {
		t.Fatalf("updated list: %d of %d (%v)", b, n, ok)
	}
	// an archive the standard reader refuses: the local headers are searched
	broken := append([]byte{}, jar[:len(jar)-22]...)
	os.WriteFile(filepath.Join(dir, "omni.ja"), broken, 0o644)
	if b, n, ok := geckoOneCRL(dir); !ok || b != n {
		t.Fatalf("optimized archive: %d of %d (%v)", b, n, ok)
	}
	// an older list
	writeJar(t, filepath.Join(dir, "omni.ja"), map[string][]byte{oneCRLEntry: []byte(`{"data":[{"issuerName":"AAAA","serialNumber":"AQ=="}]}`)})
	if b, n, ok := geckoOneCRL(dir); !ok || b != 0 || n != 6 {
		t.Fatalf("old list: %d of %d (%v)", b, n, ok)
	}
	if _, _, ok := geckoOneCRL(t.TempDir()); ok {
		t.Fatal("no omni.ja")
	}
	os.WriteFile(filepath.Join(dir, "platform.ini"), []byte("[Build]\r\nBuildID=1\r\nMilestone=158.0.1\r\n"), 0o644)
	if v := geckoVersion(dir); v != "158.0.1" {
		t.Errorf("milestone: %q", v)
	}
}

// The list shipped with Firefox 158.0.1 blocks the 6 TLS intermediates by issuer and
// serial number (an excerpt of the real file).
func TestOneCRLFirefox158(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "onecrl-firefox-158.0.1-excerpt.json"))
	if err != nil {
		t.Fatal(err)
	}
	if b, n, err := oneCRLBlocks(raw); err != nil || b != 6 || n != 6 {
		t.Fatalf("Firefox 158.0.1 list: %d of %d (%v)", b, n, err)
	}
}

func TestGeckoVersions(t *testing.T) {
	for v, want := range map[string]bool{"158.0.1": true, "158.0": false, "159.0": true, "153.5.1": true, "153.5.0": false,
		"115.43.1": true, "115.43.0": false, "140.17.0": false, "128.20.0": false} {
		if got := geckoVersionFixed(ParseVersion(v)); got != want {
			t.Errorf("%s: %v", v, got)
		}
	}
	for v, want := range map[string][2]bool{"158.0.1-2": {true, true}, "158.0.1-1": {false, true}, "158.0-1": {false, true}, "159.0-1": {true, true}, "158.0.1": {false, false}} {
		if r, ok := librewolfReadsBundled(v); r != want[0] || ok != want[1] {
			t.Errorf("librewolf %s: %v %v", v, r, ok)
		}
	}
}

// The update that the updater of Firefox has staged in its "updated" folder is not an
// installation of its own: it is named in the details of Firefox.
func TestStagedUpdate(t *testing.T) {
	root := t.TempDir()
	install := filepath.Join(root, "Mozilla Firefox")
	exe, staged := filepath.Join(install, "firefox.exe"), filepath.Join(install, "updated", "firefox.exe")
	if err := os.MkdirAll(filepath.Dir(staged), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{exe, staged} {
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// without updater.exe, nothing staged it: a copy in a folder named "updated"
	if isStagedUpdate(staged) || stagedUpdate(exe) != "" {
		t.Fatal("staged update found without updater.exe")
	}
	if err := os.WriteFile(filepath.Join(install, "updater.exe"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if !isStagedUpdate(staged) || !isBrowserCopy(staged) {
		t.Error("the staged update is listed as a browser")
	}
	// (not isBrowserCopy, which skips the programs in a Temp folder, where Windows
	// keeps the files of this test)
	if isStagedUpdate(exe) {
		t.Error("the installation is taken for a staged update")
	}
	if got := stagedUpdate(exe); got != staged {
		t.Errorf("stagedUpdate = %q, want %q", got, staged)
	}
	// a program that is not based on Firefox, or an "updated" folder alone
	other := filepath.Join(root, "App", "updated", "app.exe")
	alone := filepath.Join(root, "Tools", "updated", "firefox.exe")
	for _, p := range []string{other, alone} {
		if isStagedUpdate(p) {
			t.Errorf("%s taken for a staged update", p)
		}
	}

	if got := stagedNote("Firefox", "159.0", "159.0.1"); got != "Update 159.0.1 is ready: Firefox will install it the next time it starts." {
		t.Errorf("stagedNote = %q", got)
	}
	for _, v := range []string{"", "159.0"} { // unreadable, or the same version
		if got := stagedNote("Firefox", "159.0", v); got != "" {
			t.Errorf("stagedNote(%q) = %q, want none", v, got)
		}
	}
}
