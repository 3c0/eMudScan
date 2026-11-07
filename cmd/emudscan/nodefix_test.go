package main

import (
	"debug/pe"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeExeFor writes a minimal Windows program header for the processor machine.
func fakeExeFor(t *testing.T, p string, machine uint16) {
	t.Helper()
	b := make([]byte, 0x100)
	copy(b, "MZ")
	binary.LittleEndian.PutUint32(b[0x3c:], 0x80)
	copy(b[0x80:], "PE\x00\x00")
	binary.LittleEndian.PutUint16(b[0x84:], machine)
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

// A node.exe inside a program, of a release line that has a fix, can be replaced with
// the latest release of that line from nodejs.org, for its processor, once checked: its
// SHA-256 as Node.js publishes it, the OpenJS Foundation's signature, a fixed release,
// and its version. The fix is optional (not ticked by default), and backs the file up.
func TestNodeReplaceFix(t *testing.T) {
	Lang = 0
	dir := t.TempDir()
	for _, c := range []struct {
		machine uint16
		arch    string
	}{
		{pe.IMAGE_FILE_MACHINE_AMD64, "win-x64"},
		{pe.IMAGE_FILE_MACHINE_ARM64, "win-arm64"},
		{pe.IMAGE_FILE_MACHINE_I386, "win-x86"},
		{0x1c4, ""}, // ARM (32-bit): no build
	} {
		exe := filepath.Join(dir, c.arch+"x", "node.exe")
		fakeExeFor(t, exe, c.machine)
		f, ok := nodeReplaceFix(exe, 22, "22.23.4")
		if ok != (c.arch != "") {
			t.Errorf("%s: %v", c.arch, ok)
			continue
		}
		if !ok {
			continue
		}
		all := strings.Join(f.Cmds, "\n")
		for _, want := range []string{
			"https://nodejs.org/dist/index.json", "$_.version -like 'v22.*'", "Sort-Object { [version]$_.version.TrimStart('v') } -Descending",
			"SHASUMS256.txt", "'^[0-9a-f]{64}  " + c.arch + `/node\.exe$'`, `/` + c.arch + `/node.exe" -OutFile $nodeNew`,
			"(Get-FileHash -LiteralPath $nodeNew).Hash -eq $nodeSum", "Get-AuthenticodeSignature -LiteralPath $nodeNew).Status -eq 'Valid'",
			"O=OpenJS Foundation", "[version]'22.23.4'", "(& $nodeNew --version) -eq $nodeVersion",
		} {
			if !strings.Contains(all, want) {
				t.Errorf("%s: no %q in\n%s", c.arch, want, all)
			}
		}
		// the backup, then the copy, only when everything was checked; nothing else changes the file
		if len(f.Cmds) < 2 || !strings.Contains(f.Cmds[len(f.Cmds)-2], "$nodeOK -and ($emudscanBak = Copy-Item -LiteralPath "+psPath(exe, false)) ||
			!strings.Contains(f.Cmds[len(f.Cmds)-2], "Copy-Item -LiteralPath $nodeNew -Destination "+psPath(exe, false)+" -Force") ||
			strings.Count(all, psPath(exe, false)) != 3 || !strings.HasPrefix(f.Cmds[len(f.Cmds)-1], "Remove-Item -LiteralPath $nodeNew") {
			t.Errorf("%s: the change:\n%s", c.arch, all)
		}
		if f.Kind != fixNode || !f.Optional || len(f.Files) != 1 || f.Files[0] != exe || len(f.Plain) != len(f.Cmds) ||
			strings.Contains(strings.Join(f.Plain, "\n"), ".emudscan-") {
			t.Errorf("%s: %+v", c.arch, f)
		}
	}
	// the advice: what it does and that the program must be closed, then the commands
	exe := filepath.Join(dir, "win-x64x", "node.exe")
	a := nodeReplaceAdvice(exe, 22, "22.23.4", "Example App 1")
	if !strings.HasPrefix(a, Tf("node.replace", 22, "Example App 1")+"\n"+psHeader(exe)+"\n$ ") || fixesIn(a) == nil || !fixesIn(a)[0].Optional {
		t.Errorf("advice:\n%s", a)
	}
}
