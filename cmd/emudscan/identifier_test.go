package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsIdentifierName(t *testing.T) {
	for s, want := range map[string]bool{
		"io.esphome.builder": true, "stirling.pdf.dev": true, "ui.wifiman.desktop": true,
		"com.squirrel.Discord.Discord": true, "com.example": false, "1.2.3": false, "app-1.2.0": false,
		"OO Software": false, "Microsoft": false, "Python3.12.1": false, ".lmstudio": false,
	} {
		if got := isIdentifierName(s); got != want {
			t.Errorf("isIdentifierName(%q) = %v, want %v", s, got, want)
		}
	}
}

func TestIdentifierWords(t *testing.T) {
	for id, want := range map[string]string{
		"io.esphome.builder": "esphome builder", "stirling.pdf.dev": "stirling pdf",
		"ui.wifiman.desktop": "ui wifiman desktop", "com.github.desktop": "github desktop",
		"org.my-app.main_window": "my app main window",
	} {
		if got := strings.Join(identifierWords(id), " "); got != want {
			t.Errorf("identifierWords(%q) = %q, want %q", id, got, want)
		}
	}
}

// The name is used only when exactly one installed program holds every word.
func TestOwnerByWords(t *testing.T) {
	owners := []ownerDir{
		{dir: `C:\Users\me\AppData\Local\ESPHome Device Builder`, name: "ESPHome Device Builder"},
		{dir: `C:\Program Files\Stirling PDF`, name: "Stirling PDF"},
		{dir: `C:\Program Files\GitHub CLI`, name: "GitHub CLI"},
		{dir: `C:\Program Files\Foo Bar`, name: "Foo Bar"},
		{dir: `C:\Program Files\Foo Bar Pro`, name: "Foo Bar Pro"},
	}
	for id, want := range map[string]string{
		"io.esphome.builder": "ESPHome Device Builder",
		"stirling.pdf.dev":   "Stirling PDF",
		"com.github.desktop": "", // GitHub Desktop is not installed: not GitHub CLI
		"ui.wifiman.desktop": "",
		"com.foo.bar":        "", // two programs match: no guess
		"a.b.c":              "", // too short to tell anything
	} {
		o, ok := ownerByWords(id, owners)
		if got := map[bool]string{true: o.name}[ok]; got != want {
			t.Errorf("ownerByWords(%q) = %q, want %q", id, got, want)
		}
	}
}

// The identifier written in a program's executable, as text or UTF-16 text, names
// its owner; a program whose name has nothing in common is then found too. An
// identifier found in two programs is attributed to neither.
func TestResolveIdentifiersFromExe(t *testing.T) {
	dir := t.TempDir()
	exe := func(name string, content ...[]byte) string {
		p := filepath.Join(dir, name)
		b := []byte(strings.Repeat("\x00", 5000))
		for _, c := range content {
			b = append(append(b, c...), make([]byte, 3000)...)
		}
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	tp := testApps(t)
	tp.allDrives = true
	tp.owners = []ownerDir{
		{dir: dir, name: "WiFiman", exe: exe("wifiman.exe", []byte("ui.wifiman.desktop"))},
		{dir: dir, name: "Example Tool", exe: exe("tool.exe", utf16Bytes("com.vendor.thing"))},
		{dir: dir, name: "Shared One", exe: exe("one.exe", []byte("org.shared.lib"))},
		{dir: dir, name: "Shared Two", exe: exe("two.exe", []byte("org.shared.lib"))},
		{dir: dir, name: "ESPHome Device Builder", exe: exe("esphome-desktop.exe")},
	}
	got := tp.resolveIdentifiers([]string{"ui.wifiman.desktop", "com.vendor.thing", "org.shared.lib", "io.esphome.builder", "com.unknown.app"})
	for id, want := range map[string]string{
		"ui.wifiman.desktop": "WiFiman",
		"com.vendor.thing":   "Example Tool",
		"org.shared.lib":     "",                       // in two programs
		"io.esphome.builder": "ESPHome Device Builder", // not in its exe here: by its name
		"com.unknown.app":    "",
	} {
		o, ok := got[id]
		if name := map[bool]string{true: o.name}[ok]; name != want {
			t.Errorf("%s: owner %q, want %q", id, name, want)
		}
	}
}

// ownerOf gives an identifier-named folder of the application data to its program.
func TestOwnerOfIdentifierFolder(t *testing.T) {
	lad := t.TempDir()
	t.Setenv("LOCALAPPDATA", lad)
	t.Setenv("APPDATA", filepath.Join(lad, "Roaming"))
	for _, d := range []string{"io.esphome.builder", "com.nobody.else", "Some App"} {
		os.MkdirAll(filepath.Join(lad, d, "python"), 0o755)
	}
	tp := testApps(t)
	tp.allDrives = true
	tp.owners = []ownerDir{{dir: filepath.Join(lad, "ESPHome Device Builder"), name: "ESPHome Device Builder", url: "https://esphome.io", version: "1.1.3"}}
	for path, want := range map[string]string{
		filepath.Join(lad, "io.esphome.builder", "python", "python.exe"): "ESPHome Device Builder",
		filepath.Join(lad, "com.nobody.else", "python", "python.exe"):    "com.nobody.else",
		filepath.Join(lad, "Some App", "python", "python.exe"):           "Some App",
	} {
		o := tp.ownerOf(path)
		if o.Name != want {
			t.Errorf("ownerOf(%s) = %q, want %q", path, o.Name, want)
		}
		if want == "ESPHome Device Builder" && (o.Version != "1.1.3" || o.URL != "https://esphome.io" || o.Loc != filepath.Join(lad, "io.esphome.builder")) {
			t.Errorf("owner details: %+v", o)
		}
	}
}
