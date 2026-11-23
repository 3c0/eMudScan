package main

// The other fixes that are scripts, run for real in each PowerShell (testPowerShells),
// as the Fix function runs them and as the details give them: the certificate bundle,
// downloaded from a server of the test in place of curl.se; the Java block list; the
// restore of backups.

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// fileText is the text of a file, or "missing".
func fileText(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return "missing"
	}
	return string(b)
}

// backupsOf are the texts of eMudScan's backups of file.
func backupsOf(t *testing.T, file string) []string {
	t.Helper()
	found, _ := filepath.Glob(file + ".emudscan-*.bak")
	var out []string
	for _, b := range found {
		out = append(out, fileText(b))
	}
	return out
}

// runAlone runs the lines of a fix as the details give them, saved as a script.
func runAlone(t *testing.T, ps string, lines []string) (string, bool) {
	t.Helper()
	script := filepath.Join(t.TempDir(), "alone.ps1")
	os.WriteFile(script, append([]byte{0xEF, 0xBB, 0xBF}, strings.Join(psAdapt(lines), "\r\n")...), 0o644)
	cmd := exec.Command(ps, "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", script)
	for _, e := range os.Environ() {
		if !strings.HasPrefix(strings.ToUpper(e), "PSMODULEPATH=") {
			cmd.Env = append(cmd.Env, e)
		}
	}
	out, err := cmd.CombinedOutput()
	return string(out), err == nil
}

// The certificate bundle: replaced, with its backup, when the download matches its
// SHA-256; unchanged when it does not, or when the server cannot be reached, which
// stops all the fixes.
func TestBundleFixInPowerShell(t *testing.T) {
	Lang = 0
	shells := testPowerShells(t)
	bundle := "-----BEGIN CERTIFICATE-----\nnew\n-----END CERTIFICATE-----\n"
	sum := sha256.Sum256([]byte(bundle))
	wrong := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ca/cacert.pem":
			w.Write([]byte(bundle))
		case "/ca/cacert.pem.sha256":
			s := hex.EncodeToString(sum[:])
			if wrong {
				s = strings.Repeat("0", 64)
			}
			w.Write([]byte(s + "  cacert.pem\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	local := func(lines []string) []string {
		out := make([]string, len(lines))
		for i, l := range lines {
			out[i] = strings.ReplaceAll(l, "https://curl.se/", srv.URL+"/")
		}
		return out
	}
	for version, ps := range shells {
		for _, c := range []struct {
			name           string
			backup, alone  bool
			wrong, offline bool
			fixed          bool
		}{
			{name: "fixed", backup: true, fixed: true},
			{name: "fixed, no backups", fixed: true},
			{name: "fixed, alone", backup: true, alone: true, fixed: true},
			{name: "another download", backup: true, wrong: true},
			{name: "another download, alone", backup: true, alone: true, wrong: true},
			{name: "no server", backup: true, offline: true},
		} {
			t.Run("PowerShell "+version+"/"+c.name, func(t *testing.T) {
				dir := t.TempDir()
				files := []string{filepath.Join(dir, "a", "cacert.pem"), filepath.Join(dir, "b c", "ca-bundle.crt")}
				for _, f := range files {
					os.MkdirAll(filepath.Dir(f), 0o755)
					os.WriteFile(f, []byte("old"), 0o644)
				}
				fixes := fixesIn(replaceBundleFix(files...))
				if len(fixes) != 1 {
					t.Fatalf("fixes: %+v", fixes)
				}
				f := fixes[0]
				f.Cmds, f.Plain, f.Alone = local(f.Cmds), local(f.Plain), local(f.Alone)
				wrong = c.wrong
				url := srv.URL
				if c.offline {
					f.Cmds = (strings.Split(strings.ReplaceAll(strings.Join(f.Cmds, "\n"), url, "http://127.0.0.1:9"), "\n"))
				}
				var report string
				if c.alone {
					out, ok := runAlone(t, ps, f.Alone)
					if ok != c.fixed {
						t.Errorf("ran: %v, want %v\n%s", ok, c.fixed, out)
					}
					if c.wrong && !strings.Contains(strings.Join(strings.Fields(out), ""), strings.Join(strings.Fields(T("fix.badSum")), "")) {
						t.Errorf("no message:\n%s", out)
					}
					report = out
				} else {
					unit := (&fixRunner{backup: c.backup}).unit(Item{ID: 1, Name: "Example App 1"}, f)
					unit.cmds = psAdapt(unit.cmds)
					lines, res := psRun(t, ps, t.TempDir(), []fixUnit{unit})
					report = strings.Join(reportLines(lines), "\n")
					if res[0].ok != c.fixed || res[0].halted != c.offline {
						t.Errorf("result %+v\n%s", res, report)
					}
				}
				for _, file := range files {
					want, backups := "old", 0
					if c.fixed {
						want = bundle
						if c.backup {
							backups = 1
						}
					}
					if got := fileText(file); got != want {
						t.Errorf("%s: %q, want %q\n%s", file, got, want, report)
					}
					if b := backupsOf(t, file); len(b) != backups || backups == 1 && b[0] != "old" {
						t.Errorf("%s: backups %q\n%s", file, b, report)
					}
				}
			})
		}
	}
}

// The Java block list: the certificates added at its end, after its backup; the
// restore puts the original back.
func TestJavaAndRestoreInPowerShell(t *testing.T) {
	Lang = 0
	shells := testPowerShells(t)
	for version, ps := range shells {
		for _, alone := range []bool{false, true} {
			name := "PowerShell " + version + "/run by eMudScan"
			if alone {
				name = "PowerShell " + version + "/alone"
			}
			t.Run(name, func(t *testing.T) {
				list := filepath.Join(t.TempDir(), "jre", "lib", "security", "blocked.certs")
				os.MkdirAll(filepath.Dir(list), 0o755)
				os.WriteFile(list, []byte("Algorithm=SHA-256\nAAAA"), 0o644) // no line break at its end
				f := fixesIn(javaBlockCmd(javaBlockList{file: list}, javaRoots))[0]
				run := func(f fixAction) string {
					if alone {
						out, ok := runAlone(t, ps, f.shown())
						if !ok {
							t.Errorf("failed:\n%s", out)
						}
						return out
					}
					unit := (&fixRunner{backup: true}).unit(Item{ID: 1, Name: "Java"}, f)
					unit.cmds = psAdapt(unit.cmds)
					lines, res := psRun(t, ps, t.TempDir(), []fixUnit{unit})
					if !res[0].ok {
						t.Errorf("failed: %+v\n%s", res, strings.Join(reportLines(lines), "\n"))
					}
					return strings.Join(reportLines(lines), "\n")
				}
				out := run(f)
				got := strings.Split(strings.TrimRight(fileText(list), "\r\n"), "\n")
				if len(got) != 2+len(badCerts)+len(javaRoots) || strings.TrimSpace(got[1]) != "AAAA" || strings.TrimSpace(got[2]) != badCerts[0].SHA256 {
					t.Errorf("the list:\n%s\n%s", fileText(list), out)
				}
				if b := backupsOf(t, list); len(b) != 1 || b[0] != "Algorithm=SHA-256\nAAAA" {
					t.Errorf("backups %q", b)
				}
				// the restore, as the details give it, and as PowerShell runs it for a
				// file that needs administrator rights
				fixes := fixesIn(restoreAdvice(list))
				if len(fixes) != 1 || !fixes[0].Script {
					t.Fatalf("restore: %+v", fixes)
				}
				run(fixes[0])
				if got := fileText(list); got != "Algorithm=SHA-256\nAAAA" {
					t.Errorf("restored: %q", got)
				}
				if b := backupsOf(t, list); len(b) != 0 {
					t.Errorf("backups left: %q", b)
				}
			})
		}
	}
}
