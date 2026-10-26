package main

// Small readers written instead of standard packages that would make eMudScan
// bigger (debug/buildinfo, debug/pe, encoding/xml), checked against those packages.

import (
	"debug/buildinfo"
	"debug/pe"
	"encoding/xml"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestGoBuildInfo compares the build information read by goBuildInfo with what
// debug/buildinfo reads, on a Windows program built for the test.
func TestGoBuildInfo(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "main.go")
	os.WriteFile(src, []byte("package main\nfunc main() {}\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/tool\n\ngo 1.21\n"), 0o644)
	exe := filepath.Join(dir, "tool.exe")
	cmd := exec.Command(filepath.Join(runtime.GOROOT(), "bin", "go"), "build", "-o", exe, ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOOS=windows", "GOARCH=amd64", "CGO_ENABLED=0", "GOFLAGS=")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("cannot build the test program: %v\n%s", err, out)
	}
	want, err := buildinfo.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	vers, mod, ok := goBuildInfo(exe)
	if !ok || vers != want.GoVersion {
		t.Fatalf("goBuildInfo = %q, %v; want %q", vers, ok, want.GoVersion)
	}
	if want.Main.Path != "example.com/tool" || !strings.Contains(mod, "path\texample.com/tool\n") || !strings.Contains(mod, "mod\texample.com/tool\t"+want.Main.Version) {
		t.Fatalf("module information %q (debug/buildinfo: %+v)", mod, want.Main)
	}
	if v := goModuleVersion(exe); v != "" && v != want.Main.Version {
		t.Fatalf("goModuleVersion = %q, want %q", v, want.Main.Version)
	}
	if _, _, ok := goBuildInfo(src); ok {
		t.Fatal("a source file has no build information")
	}
}

// TestPEImportedSymbols compares peImportedSymbols with debug/pe, on 64-bit and
// 32-bit Windows programs built for the test (and on Windows' own DLLs when found).
func TestPEImportedSymbols(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nimport \"syscall\"\nfunc main() { syscall.NewLazyDLL(\"crypt32.dll\").NewProc(\"CertEnumCertificatesInStore\").Find() }\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/tool\n\ngo 1.21\n"), 0o644)
	var exes []string
	for _, arch := range []string{"amd64", "386"} {
		exe := filepath.Join(dir, "tool-"+arch+".exe")
		cmd := exec.Command(filepath.Join(runtime.GOROOT(), "bin", "go"), "build", "-o", exe, ".")
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GOOS=windows", "GOARCH="+arch, "CGO_ENABLED=0", "GOFLAGS=")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Skipf("cannot build the test program: %v\n%s", err, out)
		}
		exes = append(exes, exe)
	}
	if sys := os.Getenv("SystemRoot"); sys != "" {
		exes = append(exes, filepath.Join(sys, "System32", "curl.exe"), filepath.Join(sys, "System32", "crypt32.dll"))
	}
	for _, exe := range exes {
		f, err := pe.Open(exe)
		if err != nil {
			continue
		}
		want, err := f.ImportedSymbols()
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
		got, err := peImportedSymbols(exe)
		if err != nil {
			t.Fatal(err)
		}
		if len(want) == 0 || strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Fatalf("%s: got %d symbols %q…, want %d %q…", filepath.Base(exe), len(got), got[:min(len(got), 5)], len(want), want[:min(len(want), 5)])
		}
	}
	if _, err := peImportedSymbols(filepath.Join(dir, "main.go")); err == nil {
		t.Fatal("a source file is not a program")
	}
}

// TestParseNuspec compares parseNuspec with encoding/xml.
func TestParseNuspec(t *testing.T) {
	samples := []string{
		`<?xml version="1.0" encoding="utf-8"?>
<package xmlns="http://schemas.microsoft.com/packaging/2015/06/nuspec.xsd">
  <metadata>
    <id>git.install</id>
    <version>2.47.1</version>
    <packageSourceUrl>https://github.com/chocolatey-community/chocolatey-packages</packageSourceUrl>
    <title>Git (Install)</title>
    <authors>Johannes Schindelin</authors>
    <projectUrl>https://gitforwindows.org/</projectUrl>
    <description>Git &amp; tools &lt;3 &#233;t&#xE9;</description>
    <dependencies>
      <dependency id="chocolatey-core.extension" version="1.3.3" />
    </dependencies>
  </metadata>
</package>`,
		`<package><metadata xmlns="x"><id>curl</id><version>8.11.0</version><title><![CDATA[curl & libcurl]]></title><projectUrl/></metadata></package>`,
		`<package><metadata><idx>no</idx><id>a&amp;b</id><title>Tom&apos;s &quot;tool&quot;</title></metadata></package>`,
		`<package><files/></package>`,
	}
	for _, s := range samples {
		var n struct {
			Meta struct {
				ID         string `xml:"id"`
				Version    string `xml:"version"`
				Title      string `xml:"title"`
				ProjectURL string `xml:"projectUrl"`
			} `xml:"metadata"`
		}
		_ = xml.Unmarshal([]byte(s), &n)
		want := chocoPkg{strings.TrimSpace(n.Meta.ID), strings.TrimSpace(n.Meta.Title), strings.TrimSpace(n.Meta.Version), strings.TrimSpace(n.Meta.ProjectURL)}
		got, ok := parseNuspec(s)
		if got != want || ok != (want.ID != "") {
			t.Errorf("parseNuspec = %+v, %v; want %+v", got, ok, want)
		}
	}
}

// TestLicensesText checks that the notices and licenses shown in the About box are
// all there, whole.
func TestLicensesText(t *testing.T) {
	text := licensesText()
	for _, want := range []string{
		"                    GNU GENERAL PUBLIC LICENSE\n                       Version 3, 29 June 2007",
		"Copyright 2026 Dennis Jackson and the we-have-a-problem.com contributors",
		"                                 Apache License\n                           Version 2.0, January 2004",
		"END OF TERMS AND CONDITIONS",
		"Copyright 2009 The Go Authors.",
		"Redistributions in binary form must reproduce the above",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q", want)
		}
	}
}

func TestCharRef(t *testing.T) {
	for _, c := range []struct {
		s    string
		base int
		want rune
		ok   bool
	}{
		{"233", 10, 'é', true},
		{"E9", 16, 'é', true},
		{"10FFFF", 16, 0x10FFFF, true},
		{"110000", 16, 0, false},     // above the last Unicode code point
		{"4294967295", 10, 0, false}, // fits in 32 bits, not in a rune
		{"-1", 10, 0, false},
		{"zz", 16, 0, false},
	} {
		if r, ok := charRef(c.s, c.base); r != c.want || ok != c.ok {
			t.Errorf("charRef(%q, %d) = %q, %v; want %q, %v", c.s, c.base, r, ok, c.want, c.ok)
		}
	}
}
