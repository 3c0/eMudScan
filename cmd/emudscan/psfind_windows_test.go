package main

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// On Windows, the fixes run in Windows PowerShell 5.1, which is part of Windows. The
// PowerShell 7 of Program Files, when it is there, is named with its version.
func TestFixPowerShell(t *testing.T) {
	ps, ok := fixPowerShell()
	if !ok || !strings.EqualFold(ps.exe, filepath.Join(systemDir, "WindowsPowerShell", "v1.0", "powershell.exe")) || ps.name != "Windows PowerShell 5.1" {
		t.Errorf("%v %+v", ok, ps)
	}
	programFiles := getenv("ProgramW6432")
	if programFiles == "" {
		programFiles = getenv("ProgramFiles")
	}
	seven, ok := findPowerShell("", programFiles)
	if !ok {
		t.Skip("no PowerShell 7 in Program Files")
	}
	if !regexp.MustCompile(`^PowerShell \d+\.\d+\.\d+$`).MatchString(seven.name) || !strings.EqualFold(filepath.Base(seven.exe), "pwsh.exe") {
		t.Errorf("%+v", seven)
	}
}
