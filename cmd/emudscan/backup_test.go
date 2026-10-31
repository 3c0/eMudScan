package main

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The commands name each backup after the file and the time they run, in a form that
// eMudScan reads back.
func TestBackupNames(t *testing.T) {
	f := filepath.Join(t.TempDir(), "o'neil.pem")
	copyItem := "(Copy-Item -LiteralPath " + psQuote(f) + " -Destination (" + psQuote(f+".emudscan-") + " + (Get-Date -Format 'yyyyMMdd-HHmmss') + '.bak') -PassThru)"
	// the file is changed only once its copy is made, and when the condition is true
	for cond, want := range map[string]string{"": "if (" + copyItem + ") { Change }", "$ok": "if ($ok -and " + copyItem + ") { Change }"} {
		if got := backupThen(f, cond, "Change"); got != cmd(want) {
			t.Errorf("backup:\n%s\nwant\n%s", got, cmd(want))
		}
	}
	// what PowerShell writes, for 31 October 2026 at 01:15:00 ('yyyyMMdd-HHmmss')
	os.WriteFile(f, []byte("changed"), 0o644)
	os.WriteFile(f+".emudscan-20261031-011500.bak", []byte("before"), 0o644)
	b, ok := latestBackup(f)
	if !ok || !b.at.Equal(time.Date(2026, 10, 31, 1, 15, 0, 0, time.Local)) {
		t.Errorf("backup read back: %v %+v", ok, b)
	}
}

// The copy proposed is eMudScan's most recent one that differs from the file; other
// copies (.bak of other programs, another file's) and unreadable names are ignored.
func TestLatestBackup(t *testing.T) {
	Lang = 0
	dir := t.TempDir()
	f := filepath.Join(dir, "cacert.pem")
	files := map[string]string{
		"cacert.pem": "new",
		"cacert.pem.emudscan-20261029-000000.bak": "first",
		"cacert.pem.emudscan-20261030-120000.bak": "original",
		"cacert.pem.emudscan-20261031-010000.bak": "new", // the commands ran twice
		"cacert.pem.bak":                          "another program's",
		"cacert.pem.emudscan-2026103-010000.bak":  "not eMudScan's",
		"cacert.pem.emudscan-20261399-010000.bak": "no such date",
		"cacert.pem.emudscan-20261101-000000.txt": "not a backup",
		"other.pem.emudscan-20261101-000000.bak":  "another file's",
	}
	for n, c := range files {
		os.WriteFile(filepath.Join(dir, n), []byte(c), 0o644)
	}
	b, ok := latestBackup(f)
	if !ok || filepath.Base(b.path) != "cacert.pem.emudscan-20261030-120000.bak" {
		t.Fatalf("latest: %v %+v", ok, b)
	}
	got := restoreAdvice(f, filepath.Join(dir, "none.pem"))
	for _, w := range []string{T("restore.intro1"), "cacert.pem.emudscan-20261030-120000.bak", " — saved on 30 October 2026 at 12:00", T("restore.how1") + "\n" + T("restore.warn1") + "\n" + T("fix.ps"),
		"Move-Item -LiteralPath " + psQuote(b.path) + " -Destination " + psQuote(f) + " -Force"} {
		if !strings.Contains(got, w) {
			t.Errorf("%q missing:\n%s", w, got)
		}
	}
	if strings.Count(got, "Move-Item") != 1 {
		t.Errorf("one file to put back:\n%s", got)
	}
	g := filepath.Join(dir, "other.pem")
	os.WriteFile(g, []byte("changed"), 0o644)
	if got := restoreAdvice(f, g); !strings.Contains(got, T("restore.introN")) || !strings.Contains(got, T("restore.howN")+"\n"+T("restore.warnN")) || strings.Count(got, "Move-Item") != 2 {
		t.Errorf("two files to put back:\n%s", got)
	}
	if got := restoreAdvice(filepath.Join(dir, "none.pem"), ""); got != "" {
		t.Errorf("nothing to put back: %s", got)
	}
}

// A list that eMudScan's commands replaced has its own line while their copy is there,
// up to date or not, with the commands that put it back.
func TestBundleRestore(t *testing.T) {
	Lang = 0
	// an up-to-date list: roots of its own, none leading to the compromised intermediates
	var good bytes.Buffer
	for i := range minBundleCerts {
		key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		tpl := &x509.Certificate{SerialNumber: big.NewInt(int64(i + 1)), Subject: pkix.Name{CommonName: fmt.Sprint("Root ", i)},
			NotBefore: time.Now(), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true}
		der, _ := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
		pem.Encode(&good, &pem.Block{Type: "CERTIFICATE", Bytes: der})
	}
	dir := t.TempDir()
	write := func(n string, data []byte) string {
		p := filepath.Join(dir, n)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, data, 0o644)
		return p
	}
	replaced := write("a/cacert.pem", good.Bytes())
	write("a/cacert.pem.emudscan-20261031-011500.bak", outdatedBundle(t))
	untouched := write("b/cacert.pem", good.Bytes())
	outdated := write("c/cacert.pem", outdatedBundle(t))
	write("c/cacert.pem.emudscan-20261031-011500.bak", []byte("before"))
	tp := testApps(t)
	tp.d = newDiscovery()
	for _, p := range []string{replaced, untouched, outdated} {
		tp.d.bundles[p] = true
	}
	tp.checkBundles()
	got := map[string]*Item{}
	for _, id := range tp.c.order {
		if it := tp.c.items[id]; it.Kind == "bundle" {
			got[it.File] = it
		}
	}
	if len(got) != 2 || got[untouched] != nil {
		t.Fatalf("lines: %v", got)
	}
	for p, st := range map[string]Status{replaced: StOK, outdated: StWarn} {
		it := got[p]
		if it == nil || it.Status != st || !strings.Contains(it.Details, "Move-Item -LiteralPath "+psQuote(p+".emudscan-20261031-011500.bak")+" -Destination "+psQuote(p)+" -Force") {
			t.Errorf("%s: %+v", p, it)
		}
	}
}

// A Java whose block list eMudScan's commands completed proposes to put it back.
func TestJavaRestore(t *testing.T) {
	Lang = 0
	home := filepath.Join(t.TempDir(), "jre")
	sec := filepath.Join(home, "lib", "security")
	os.MkdirAll(sec, 0o755)
	for _, r := range rootCerts {
		if r.Name == rootNameECCG3 {
			os.WriteFile(filepath.Join(sec, "cacerts"), append([]byte("padding"), r.Cert.Raw...), 0o644)
		}
	}
	blocked := filepath.Join(sec, "blocked.certs")
	list := "Algorithm=SHA-256\n"
	os.WriteFile(blocked+".emudscan-20261031-011500.bak", []byte(list), 0o644)
	for _, c := range badCerts {
		list += c.SHA256 + "\n"
	}
	for _, r := range javaRoots {
		list += r.sha256 + "\n"
	}
	os.WriteFile(blocked, []byte(list), 0o644)
	tp := testApps(t)
	it := tp.c.begin(GroupApps, "java", "Java", home, func(it *Item) { tp.javaOne(it, home) })
	if it.Status != StOK || !strings.Contains(it.Details, T("java.okRootsBlocked")+"\n\n"+T("restore.intro1")) ||
		!strings.Contains(it.Details, "Move-Item -LiteralPath "+psQuote(blocked+".emudscan-20261031-011500.bak")+" -Destination "+psQuote(blocked)+" -Force") {
		t.Errorf("%v:\n%s", it.Status, it.Details)
	}
}
