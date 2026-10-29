package main

import (
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// On Windows, the DLLs of the checks and of the window must all name their file in
// System32 (TestDLLsLoadedFromSystem32 checks the source; this checks the result).
func TestDLLPathsAreInSystem32(t *testing.T) {
	if !filepath.IsAbs(systemDir) || !strings.EqualFold(filepath.Base(systemDir), "System32") {
		t.Fatalf("systemDir = %q, want Windows' System32 folder", systemDir)
	}
	mods := map[string]*syscall.LazyDLL{
		"version": modversion, "winhttp": modwinhttp, "crypt32": modcrypt32, "ntdll": modntdll, "shell32 (checks)": modshell32w,
		"user32": moduser32, "gdi32": modgdi32, "uxtheme": moduxtheme, "shell32 (window)": modshell32, "ole32": modole32,
	}
	for name, m := range mods {
		if !strings.HasPrefix(strings.ToLower(m.Name), strings.ToLower(systemDir)+`\`) {
			t.Errorf("%s: loaded as %q, want a path in %s", name, m.Name, systemDir)
		}
	}
	if modcomctl32.Name != "comctl32.dll" {
		t.Errorf("comctl32: loaded as %q, want by name so that the manifest selects version 6", modcomctl32.Name)
	}
	if !strings.EqualFold(kernel32.Name, "kernel32.dll") {
		t.Errorf("kernel32: loaded as %q", kernel32.Name)
	}
}
