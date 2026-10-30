package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The roots that OpenJDK removes, as eMudScan blocks them, are those of the reference
// data where both list them.
func TestJavaRootsMatchTheReferenceData(t *testing.T) {
	for _, j := range javaRoots {
		for _, r := range rootCerts {
			if r.Name == j.name && !strings.EqualFold(r.SHA256, j.sha256) {
				t.Errorf("%s: %s in the reference data, %s blocked", j.name, r.SHA256, j.sha256)
			}
		}
	}
}

// A Java is judged by its list of trusted certificates (cacerts) and its block list
// (blocked.certs), which Java applies to every certificate of a chain, its root
// included. Roots leading to the compromised intermediates, with neither blocked:
// red. The intermediates blocked, as by the first fix of 21 October, but not the
// roots, which OpenJDK has since removed: amber. The roots absent or blocked: green.
// While there is no update, the block list is completed with the compromised
// certificates, the four roots that OpenJDK removes and any other root found.
func TestJavaVerdicts(t *testing.T) {
	Lang = 0
	der, tlsG3 := map[string][]byte{}, ""
	for _, r := range rootCerts {
		der[r.Name] = r.Cert.Raw
		if r.Name == rootNameTLSG3 {
			tlsG3 = r.SHA256
		}
	}
	openJDK := []string{rootNameECCG3, rootNameAAA} // those of OpenJDK's cacerts
	withTLSG3 := []string{rootNameECCG3, rootNameTLSG3, rootNameAAA}
	intermediates, roots := "", ""
	for _, c := range badCerts {
		intermediates += c.SHA256 + "\n"
	}
	for _, r := range javaRoots {
		roots += r.sha256 + "\n"
	}
	header := "Algorithm=SHA-256\n"
	root := t.TempDir()
	for i, c := range []struct {
		name         string
		roots        []string // in cacerts
		bundled      bool
		blocked      string // the block list; none when empty
		status       Status
		want, absent []string // in this order; not there
	}{
		{"nothing blocked", openJDK, false, header, StWarn,
			[]string{T("java.fix"), T("java.blockRoots"), ".bak'", "Add-Content", badCerts[0].SHA256, javaRoots[0].sha256, javaRoots[3].sha256},
			[]string{"keytool"}},
		{"first fix only", openJDK, false, header + intermediates, StCaution,
			[]string{T("java.blockOnly"), T("java.fix"), T("java.blockRoots"), "Add-Content", javaRoots[0].sha256, javaRoots[3].sha256},
			[]string{badCerts[0].SHA256}},
		{"roots blocked", openJDK, false, header + intermediates + roots, StOK,
			[]string{T("java.okRootsBlocked")}, []string{"Add-Content", T("java.fix")}},
		{"roots blocked, not the intermediates", openJDK, false, header + roots, StOK,
			[]string{T("java.okRootsBlocked")}, []string{"Add-Content"}},
		{"no roots", nil, false, header, StOK, []string{T("java.okNoRoots")}, []string{"Add-Content"}},
		{"no block list", openJDK, false, "", StWarn, []string{T("java.fix")}, []string{T("java.blockRoots"), "Add-Content"}},
		// a root that OpenJDK does not ship, which issued the direct intermediates
		{"another root, the four blocked", withTLSG3, false, header + roots, StWarn,
			[]string{T("java.fix"), Tf("java.blockOthers", rootNameTLSG3), "Add-Content", badCerts[0].SHA256, tlsG3},
			[]string{javaRoots[0].sha256}},
		{"another root, the intermediates and the four blocked", withTLSG3, false, header + intermediates + roots, StCaution,
			[]string{T("java.blockOnly"), "Add-Content", tlsG3}, []string{badCerts[0].SHA256, javaRoots[0].sha256}},
		{"all roots blocked", withTLSG3, false, header + roots + tlsG3, StOK,
			[]string{T("java.okRootsBlocked")}, []string{"Add-Content"}},
		{"inside another program", openJDK, true, header, StWarn,
			[]string{"**This copy belongs to Stirling PDF.**", T("java.blockRoots"), "Add-Content"},
			[]string{T("java.fix")}},
	} {
		app := filepath.Join(root, "app"+itoa(i))
		home := filepath.Join(app, "runtime", "jre")
		sec := filepath.Join(home, "lib", "security")
		os.MkdirAll(sec, 0o755)
		cacerts := []byte("padding")
		for _, n := range c.roots {
			cacerts = append(cacerts, der[n]...)
		}
		os.WriteFile(filepath.Join(sec, "cacerts"), cacerts, 0o644)
		if c.blocked != "" {
			os.WriteFile(filepath.Join(sec, "blocked.certs"), []byte(c.blocked), 0o644)
		}
		tp := testApps(t)
		if c.bundled {
			tp.owners = []ownerDir{{dir: app, name: "Stirling PDF", version: "3.2.2"}}
		}
		it := tp.c.begin(GroupApps, "java", "Java", home, func(it *Item) { tp.javaOne(it, home) })
		if it.Status != c.status {
			t.Errorf("%s: status %v, want %v", c.name, it.Status, c.status)
		}
		d, at := it.Details, 0
		for _, w := range c.want {
			i := strings.Index(d[at:], w)
			if i < 0 {
				t.Errorf("%s: %q missing or out of order:\n%s", c.name, w, d)
				break
			}
			at += i + len(w)
		}
		for _, a := range c.absent {
			if strings.Contains(d, a) {
				t.Errorf("%s: %q should not be there:\n%s", c.name, a, d)
			}
		}
	}
}
