package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The fixes run in Windows PowerShell; on a PC without it, in the PowerShell of the
// highest major version, 7 or later, in Program Files; never in another pwsh.exe, such
// as one in the PATH; none when there is neither.
func TestFindPowerShell(t *testing.T) {
	root := t.TempDir()
	system, programFiles := filepath.Join(root, "Windows", "System32"), filepath.Join(root, "Program Files")
	put := func(parts ...string) string {
		p := filepath.Join(parts...)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte("MZ"), 0o755)
		return p
	}
	// another pwsh.exe, in the PATH: never taken
	path := filepath.Join(root, "Users", "me", "bin")
	put(path, "pwsh.exe")
	t.Setenv("PATH", path)
	if ps, ok := findPowerShell(system, programFiles); ok {
		t.Errorf("found %+v", ps)
	}
	// PowerShell 6, a preview, and a folder without pwsh.exe: not taken
	put(programFiles, "PowerShell", "6", "pwsh.exe")
	put(programFiles, "PowerShell", "7-preview", "pwsh.exe")
	os.MkdirAll(filepath.Join(programFiles, "PowerShell", "9"), 0o755)
	if ps, ok := findPowerShell(system, programFiles); ok {
		t.Errorf("found %+v", ps)
	}
	seven := put(programFiles, "PowerShell", "7", "pwsh.exe")
	if ps, ok := findPowerShell(system, programFiles); !ok || ps.exe != seven || ps.name != "PowerShell 7" {
		t.Errorf("PowerShell 7: %v %+v", ok, ps)
	}
	eight := put(programFiles, "PowerShell", "8", "pwsh.exe")
	if ps, ok := findPowerShell(system, programFiles); !ok || ps.exe != eight || ps.name != "PowerShell 8" {
		t.Errorf("PowerShell 8: %v %+v", ok, ps)
	}
	// Windows PowerShell, when it is there
	wps := put(system, "WindowsPowerShell", "v1.0", "powershell.exe")
	if ps, ok := findPowerShell(system, programFiles); !ok || ps.exe != wps || !strings.HasPrefix(ps.name, "Windows PowerShell") {
		t.Errorf("Windows PowerShell: %v %+v", ok, ps)
	}
	if ps, ok := findPowerShell("", ""); ok || ps.exe != "" || ps.name != "PowerShell" {
		t.Errorf("no folders: %+v", ps)
	}
}
