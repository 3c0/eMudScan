package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// yt-dlp's programs are recognized by their version information, as yt-dlp's build
// writes it (bundle/pyinstaller.py): its organization as the company, yt-dlp (or
// yt-dlp_x86…) as the product, the version as FileVersion.
func TestIsYtDlp(t *testing.T) {
	for _, c := range []struct {
		fv   fileVersion
		want bool
	}{
		{fileVersion{Company: "https://github.com/yt-dlp", Name: "yt-dlp"}, true},
		{fileVersion{Company: "https://github.com/yt-dlp/", Name: "yt-dlp_x86"}, true},
		{fileVersion{Company: "Someone else", Name: "yt-dlp"}, false},
		{fileVersion{Company: "https://github.com/yt-dlp", Name: "other"}, false},
		{fileVersion{}, false},
	} {
		if got := isYtDlp(c.fv); got != c.want {
			t.Errorf("%+v: %v", c.fv, got)
		}
	}
	for _, c := range []struct {
		fv   fileVersion
		want string
	}{
		{fileVersion{File: "2026.10.26", Product: "2026.10.26 on Python 3.13.9"}, "2026.10.26"},
		{fileVersion{File: "2026.10.27.232838"}, "2026.10.27.232838"},
		{fileVersion{Fixed: Version{2026, 10, 21, 0}}, "2026.10.21"},
		{fileVersion{}, ""},
	} {
		if got := ytdlpVersion(c.fv); got != c.want {
			t.Errorf("%+v: %q, want %q", c.fv, got, c.want)
		}
	}
}

// The homepage of yt-dlp installed as itself is its project page, whatever its
// package says; a copy inside other software has that software's homepage.
func TestYtDlpHomepage(t *testing.T) {
	for _, c := range []struct {
		o    ownerInfo
		want string
	}{
		{ownerInfo{Name: "yt-dlp", URL: "https://discord.gg/example"}, ytdlpHome},
		{ownerInfo{Name: "yt-dlp.yt-dlp"}, ytdlpHome},
		{ownerInfo{}, ytdlpHome},
		{ownerInfo{Name: "Video Downloader", URL: "https://example.org"}, "https://example.org"},
	} {
		if got := ytdlpOwnerURL(c.o); got != c.want {
			t.Errorf("%+v: %q", c.o, got)
		}
	}
}

// 2026.10.26 is the fixed version: 2026.10.21, whose certifi no longer holds the
// eMudhra root, is still outdated. The advice warns against the self-updater.
func TestYtDlpVerdict(t *testing.T) {
	old := embedFindings{Roots: []string{rootNameECCG3}, Packed: []string{`certifi\cacert.pem`}}
	for _, c := range []struct {
		ver    string
		f      embedFindings
		status Status
		want   []string
	}{
		{"2026.08.19", old, StWarn, []string{rootNameECCG3, `certifi\cacert.pem`, Tf("app.fix", "yt-dlp", "2026.10.26"), T("ytdlp.noSelfUpdate")}},
		{"2026.10.21", embedFindings{}, StWarn, []string{Tf("app.fix", "yt-dlp", "2026.10.26"), T("ytdlp.noSelfUpdate")}},
		{"2026.10.26", embedFindings{Blocklist: true}, StOK, []string{Tf("app.okDetails", "yt-dlp", "2026.10.26")}},
		{"2026.10.27.232838", embedFindings{}, StOK, []string{T("app.sumOK")}},
		{"", embedFindings{}, StCaution, []string{T("ytdlp.noSelfUpdate")}},
	} {
		st, sum, details := ytdlpVerdict(c.ver, c.f)
		if st != c.status {
			t.Errorf("%s: status %v", c.ver, st)
		}
		for _, w := range c.want {
			if !strings.Contains(sum+"\n"+details, w) {
				t.Errorf("%s: missing %q in %q / %q", c.ver, w, sum, details)
			}
		}
	}
}

// A yt-dlp element reports its program file only: the rest of the folder of an
// internet program that holds it is still checked, and when nothing else is found
// there, the yt-dlp element speaks for the program.
func TestYtDlpInInternetProgram(t *testing.T) {
	Lang = 0
	g3 := rootByName(rootNameECCG3)
	for _, c := range []struct {
		name  string
		other []byte // another file of the program
		want  Status // of the program's element; StPending: none
	}{
		{"own root store", append([]byte("MZ "), g3.Cert.RawSubjectPublicKeyInfo...), StWarn},
		{"nothing else", []byte("MZ nothing"), StPending},
	} {
		dir := t.TempDir()
		exe := filepath.Join(dir, "yt-dlp.exe")
		pad := make([]byte, 300<<10) // program files from 256 KB are read
		os.WriteFile(exe, append(append([]byte("MZ "), g3.Cert.RawSubjectPublicKeyInfo...), pad...), 0o644)
		os.WriteFile(filepath.Join(dir, "engine.dll"), append(c.other, pad...), 0o644)
		tp := testApps(t)
		tp.c.begin(GroupNet, "ytdlp", "yt-dlp", dir, func(it *Item) {
			tp.c.finish(it, result{File: exe, Exe: exe, Folder: dir, Status: StWarn})
		})
		tp.netPrograms = []netProgram{{name: "Downloader", dir: dir}}
		tp.checkInternetFolders()
		got := StPending
		for _, it := range tp.c.snapshot() {
			if it.Kind == "netapp" {
				got = it.Status
			}
		}
		if got != c.want {
			t.Errorf("%s: program element %v, want %v", c.name, got, c.want)
		}
	}
}
