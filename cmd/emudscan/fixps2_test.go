package main

// The other fixes that are scripts, run for real in each PowerShell (testPowerShells),
// as the Fix function runs them and as the details give them: the certificate bundle,
// downloaded from a server of the test in place of curl.se; the Java block list; the
// restore of backups; the node.exe of a program, with a stand-in for its download.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

// runAlone runs the lines of a fix as the details give them, saved as a script, with
// the variables env added to the environment.
func runAlone(t *testing.T, ps string, lines []string, env ...string) (string, bool) {
	t.Helper()
	script := filepath.Join(t.TempDir(), "alone.ps1")
	os.WriteFile(script, append([]byte{0xEF, 0xBB, 0xBF}, strings.Join(psAdapt(lines), "\r\n")...), 0o644)
	cmd := exec.Command(ps, "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", script)
	for _, e := range os.Environ() {
		if !strings.HasPrefix(strings.ToUpper(e), "PSMODULEPATH=") {
			cmd.Env = append(cmd.Env, e)
		}
	}
	cmd.Env = append(cmd.Env, env...)
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
					if c.wrong && !strings.Contains(errorText(out), tightText(T("fix.badSum"))) {
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

// standInNode builds, in dir, a program that stands in for node.exe: with --version, it
// shows the version written at its end, after "version=". It gives the program, with v
// written at its end.
func standInNode(t *testing.T, dir string) func(v string) []byte {
	t.Helper()
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte(`package main

import (
	"bytes"
	"fmt"
	"os"
)

func main() {
	exe, _ := os.Executable()
	b, _ := os.ReadFile(exe)
	if i := bytes.LastIndex(b, []byte("\nversion=")); i >= 0 && len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Println(string(bytes.TrimSpace(b[i+len("\nversion="):])))
		return
	}
	os.Exit(9)
}
`), 0o644)
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/node\n\ngo 1.21\n"), 0o644)
	exe := filepath.Join(dir, "node.exe")
	cmd := exec.Command(filepath.Join(runtime.GOROOT(), "bin", "go"), "build", "-o", exe, ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOFLAGS=")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("cannot build the stand-in node.exe: %v\n%s", err, out)
	}
	data, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	return func(v string) []byte { return append(append([]byte(nil), data...), "\nversion="+v+"\n"...) }
}

// The node.exe of a program: replaced, after its backup, with the download, when it is
// the expected one (its SHA-256, its signature by the OpenJS Foundation, the version it
// gives) and the program is closed; unchanged when one of them is not, or when the
// download fails, which stops all the fixes. The download is already in its folder: the
// commands name a server that cannot be reached. Only Windows checks signatures: the
// test gives the signature, but in one case, on Windows, where the stand-in, which is
// not signed, is checked for real.
func TestNodeFixInPowerShell(t *testing.T) {
	Lang = 0
	shells := testPowerShells(t)
	base := t.TempDir() // the folders of the cases are in it, by number: short paths, for Windows PowerShell
	download := standInNode(t, filepath.Join(base, "build"))
	signed := func(status, subject string) string {
		return "$signature = [pscustomobject]@{ Status = '" + status + "'; SignerCertificate = [pscustomobject]@{ Subject = '" + subject + "' } }"
	}
	openJS := signed("Valid", "CN=OpenJS Foundation, O=OpenJS Foundation, L=San Francisco, S=California, C=US")
	badSignature, badVersion := T("node.badSignature"), T("node.badVersion")
	type nodeCase struct {
		name          string
		backup, alone bool
		signature     string // the line in place of Get-AuthenticodeSignature; "" for Windows' own
		version       string // what the download gives with --version
		badSum        bool   // the download is not the one the commands name
		inUse         bool   // the program is running (Windows)
		offline       bool   // no download, and no server
		stopsAt       string // the command that fails, by a part of it; "" when none does
		says          string // what it says
	}
	cases := []nodeCase{
		{name: "fixed", backup: true, signature: openJS, version: "v22.23.5"},
		{name: "fixed, no backups", signature: openJS, version: "v22.23.5"},
		{name: "another download", backup: true, signature: openJS, version: "v22.23.5", badSum: true, stopsAt: "Get-FileHash", says: T("fix.badSum")},
		{name: "another signer", backup: true, signature: signed("Valid", "CN=OpenJS Foundation, O=Example Signer, C=US"), version: "v22.23.5", stopsAt: "$signature.Status", says: badSignature},
		{name: "a longer name", backup: true, signature: signed("Valid", "CN=Example Signer, O=OpenJS Foundation Example, C=US"), version: "v22.23.5", stopsAt: "$signature.Status", says: badSignature},
		{name: "a signature not valid", backup: true, signature: signed("HashMismatch", "CN=OpenJS Foundation, O=OpenJS Foundation, C=US"), version: "v22.23.5", stopsAt: "$signature.Status", says: badSignature},
		{name: "not signed", backup: true, version: "v22.23.5", stopsAt: "$signature.Status", says: badSignature},
		{name: "another version", backup: true, signature: openJS, version: "v22.23.4", stopsAt: "--version", says: badVersion},
		{name: "in use", backup: true, signature: openJS, version: "v22.23.5", inUse: true, stopsAt: "[IO.File]::Open", says: "node.exe"},
		{name: "no server", backup: true, signature: openJS, offline: true, stopsAt: "Invoke-WebRequest"},
	}
	for _, c := range cases {
		if c.offline || c.name == "fixed, no backups" {
			continue
		}
		c.name, c.alone = c.name+", alone", true
		cases = append(cases, c)
	}
	n := 0
	for version, ps := range shells {
		for _, c := range cases {
			n++
			root, run := filepath.Join(base, fmt.Sprint(n), "a"), filepath.Join(base, fmt.Sprint(n), "r")
			t.Run("PowerShell "+version+"/"+c.name, func(t *testing.T) {
				t.Parallel()
				if (c.inUse || c.signature == "") && runtime.GOOS != "windows" {
					t.Skip("only Windows checks this")
				}
				exe := filepath.Join(root, "Example App 1", "resources", "node.exe")
				os.MkdirAll(filepath.Dir(exe), 0o755)
				os.WriteFile(exe, []byte("old node.exe"), 0o644)
				created := fileCreated(exe)
				data := download(c.version)
				sum := sha256.Sum256(data)
				f := nodeFix(exe, nodeSource{version: "22.23.5", arch: "win-x64", sha256: hex.EncodeToString(sum[:])})
				if c.badSum {
					data = append(data, "changed"...)
				}
				folder := run // where the commands look for the download
				if c.alone {
					folder = filepath.Join(run, "eMudScan-fix")
				}
				os.MkdirAll(folder, 0o755)
				if !c.offline {
					os.WriteFile(filepath.Join(folder, "node-22.23.5-win-x64.exe"), data, 0o755)
				}
				if c.inUse {
					held, err := os.Open(exe)
					if err != nil {
						t.Fatal(err)
					}
					defer held.Close()
				}
				adapt := func(lines []string) []string {
					var out []string
					for _, l := range lines {
						l = strings.ReplaceAll(l, "https://nodejs.org/", "http://127.0.0.1:9/")
						if c.signature != "" && strings.TrimSpace(l) == "$signature = Get-AuthenticodeSignature -LiteralPath $new" {
							l = strings.Replace(l, "$signature = Get-AuthenticodeSignature -LiteralPath $new", c.signature, 1)
						}
						out = append(out, l)
					}
					return psAdapt(out)
				}
				var report string
				fixed := c.stopsAt == ""
				if c.alone {
					out, ok := runAlone(t, ps, adapt(f.Alone), "TEMP="+run)
					if ok != fixed || !fixed && !strings.Contains(errorText(out), tightText(c.says)) {
						t.Errorf("ran: %v, want fixed %v and %q\n%s", ok, fixed, c.says, out)
					}
					report = out
					// the download: deleted at the end, or when it is not the expected one; else
					// left for the next time
					if _, err := os.Stat(folder); (err != nil) != fixed {
						t.Errorf("the folder of the download: %v, with fixed %v", err, fixed)
					}
					if _, err := os.Stat(filepath.Join(folder, "node-22.23.5-win-x64.exe")); c.badSum && err == nil {
						t.Error("the wrong download is still there")
					}
				} else {
					unit := (&fixRunner{backup: c.backup}).unit(Item{ID: 1, Name: "Example App 1"}, f)
					unit.cmds = adapt(unit.cmds)
					lines, res := psRun(t, ps, run, []fixUnit{unit})
					report = strings.Join(reportLines(lines), "\n")
					if len(res) != 1 || !res[0].ran || res[0].ok != fixed || res[0].halted != c.offline || !strings.Contains(report, c.says) {
						t.Errorf("result %+v, want fixed %v and %q\n%s", res, fixed, c.says, report)
					}
					var failed []string
					for i, l := range lines {
						if l.kind == lineFailed && i > 0 {
							failed = append(failed, lines[i-1].text)
						}
					}
					if !fixed && (len(failed) != 1 || !strings.Contains(failed[0], c.stopsAt)) || fixed && len(failed) > 0 {
						t.Errorf("failed: %q, want %q\n%s", failed, c.stopsAt, report)
					}
				}
				want, backups := []byte("old node.exe"), 0
				if fixed {
					want = data
					if c.backup {
						backups = 1
					}
				}
				if got, _ := os.ReadFile(exe); !bytes.Equal(got, want) {
					t.Errorf("node.exe: %.20q, want %.20q\n%s", got, want, report)
				}
				found, _ := filepath.Glob(exe + ".emudscan-*.bak")
				if len(found) != backups || backups == 1 && (fileText(found[0]) != "old node.exe" || fileCreated(found[0]) != created) {
					t.Errorf("backups %q\n%s", found, report)
				}
			})
		}
	}
}
