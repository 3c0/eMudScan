package main

// The PowerShell that runs the fixes. Windows PowerShell, which is part of Windows,
// runs them. On a PC where it was removed, PowerShell 7 or later runs them: the one
// installed in Program Files (PowerShell\<major version>\pwsh.exe), where only
// administrators can write. Never a pwsh.exe found in the PATH or in a user's folder:
// the fixes can run as administrator. The tests run the scripts of the fixes in both
// (fixps_test.go).

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// psHost is the PowerShell that runs the fixes: its program, and its name with its
// version, as the Fix window and the report show it.
type psHost struct {
	exe, name string
}

// findPowerShell finds the PowerShell that runs the fixes: Windows PowerShell, in
// system (Windows' System32 folder), else the PowerShell of the highest major version
// in programFiles; false when there is none, with "PowerShell" for its name, which the
// messages give.
func findPowerShell(system, programFiles string) (psHost, bool) {
	if exe := filepath.Join(system, "WindowsPowerShell", "v1.0", "powershell.exe"); system != "" && exists(exe) {
		name := "Windows PowerShell"
		if v, ok := regString(hklm, `SOFTWARE\Microsoft\PowerShell\3\PowerShellEngine`, "PowerShellVersion", view64); ok {
			if p := ParseVersion(v); len(p) >= 2 { // 5.1.26100.1
				name = fmt.Sprintf("Windows PowerShell %d.%d", p[0], p[1])
			}
		}
		return psHost{exe, name}, true
	}
	none := psHost{name: "PowerShell"}
	if programFiles == "" {
		return none, false
	}
	dir := filepath.Join(programFiles, "PowerShell")
	entries, _ := os.ReadDir(dir)
	found, major := psHost{}, 0
	for _, e := range entries {
		n, err := strconv.Atoi(e.Name()) // 7; not 7-preview
		exe := filepath.Join(dir, e.Name(), "pwsh.exe")
		if err == nil && n >= 7 && n > major && exists(exe) {
			found, major = psHost{exe, "PowerShell " + e.Name()}, n
		}
	}
	if major == 0 {
		return none, false
	}
	if fv, ok := getFileVersion(found.exe); ok && len(fv.Fixed) >= 3 && fv.Fixed[0] == major { // 7.5.4.500
		found.name = fmt.Sprintf("PowerShell %d.%d.%d", fv.Fixed[0], fv.Fixed[1], fv.Fixed[2])
	}
	return found, true
}
