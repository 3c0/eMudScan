package main

import "strings"

// scriptLines builds the commands of a fix as a script written to be read
// (fixAction.Script): blocks, each made of a comment that says what it does and of its
// commands, one on each line, with an empty line between two blocks.
type scriptLines []string

// block adds a block: its comment, then its commands.
func (s *scriptLines) block(comment string, cmds ...string) {
	if len(*s) > 0 {
		*s = append(*s, "")
	}
	*s = append(append(*s, "# "+comment), cmds...)
}

// runsAlone is the script of a fix as the details page shows it, to be run without
// eMudScan, also pasted into a PowerShell window: one block, in which every error
// stops it ($ErrorActionPreference) and which leaves no setting and no variable
// behind. The Fix function runs the script as it is, and stops it at its first command
// that fails itself. A script of a single command needs none of this: nil.
func runsAlone(lines []string) []string {
	n := 0
	for _, l := range lines {
		if isCommand(l) {
			n++
		}
	}
	if n < 2 {
		return nil
	}
	out := []string{"& {", "    $ErrorActionPreference = 'Stop'", ""}
	for _, l := range lines {
		if l != "" {
			l = "    " + l
		}
		out = append(out, l)
	}
	return append(out, "}")
}

// fetchLines are the lines of a script that download what it needs (fixAction.Fetch):
// when one fails, the Fix function stops all the fixes, which would fail the same way.
func fetchLines(lines []string) []int {
	var out []int
	for i, l := range lines {
		if strings.Contains(l, "Invoke-WebRequest") {
			out = append(out, i)
		}
	}
	return out
}

// psPaths is the paths, as PowerShell takes a list of them.
func psPaths(paths []string, admin bool) string {
	q := make([]string, len(paths))
	for i, p := range paths {
		q[i] = psPath(p, admin)
	}
	return strings.Join(q, ", ")
}

// backupLines are the commands that back up the files of $files, or of the variable
// given, with their dates, all named after the same time ($stamp): the copy gets the
// creation date of its file, which Windows would date from now, so that a restore,
// which moves it back, puts back both dates.
func backupLines(variable string) []string {
	if variable == "$files" {
		return []string{`$stamp = Get-Date -Format 'yyyyMMdd-HHmmss'`,
			`$files | ForEach-Object { Copy-Item -LiteralPath $_ -Destination "$_.emudscan-$stamp.bak" }`,
			`$files | ForEach-Object { (Get-Item -LiteralPath "$_.emudscan-$stamp.bak").CreationTime = (Get-Item -LiteralPath $_).CreationTime }`}
	}
	return []string{`$stamp = Get-Date -Format 'yyyyMMdd-HHmmss'`,
		`Copy-Item -LiteralPath ` + variable + ` -Destination "` + variable + `.emudscan-$stamp.bak"`,
		`(Get-Item -LiteralPath "` + variable + `.emudscan-$stamp.bak").CreationTime = (Get-Item -LiteralPath ` + variable + `).CreationTime`}
}
