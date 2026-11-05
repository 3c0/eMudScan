package main

// Paths as shown: a path in a folder of the user is shown with the environment
// variable that names that folder, as File Explorer understands it, such as
// %LOCALAPPDATA%\Programs\App\app.exe, and commands name it as PowerShell does, such as
// "$env:LOCALAPPDATA\Programs\App\app.exe". It is shorter, and the user's name stays
// out of the details, the report, the log and the Fix window. Links, Open folder and
// copied paths keep the real path, and so do the commands that run as administrator:
// Windows may run them as another account, whose variables name its own folders.

import (
	"os"
	"sort"
	"strings"
	"sync"
)

// userFolderVars are the variables that name the folders of the user.
var userFolderVars = []string{"LOCALAPPDATA", "APPDATA", "USERPROFILE"}

// namedFolder is a folder and the variable that names it.
type namedFolder struct{ name, path string }

// userFolders are the folders of the user, the longest first (%LOCALAPPDATA% before
// %USERPROFILE%, which holds it).
var userFolders = sync.OnceValue(func() []namedFolder { return namedFolders(os.Getenv) })

// namedFolders reads the folders of userFolderVars with getenv, keeping only full
// paths on a drive (C:\Users\me).
func namedFolders(getenv func(string) string) []namedFolder {
	var out []namedFolder
	for _, v := range userFolderVars {
		p := strings.TrimRight(getenv(v), `\`)
		if len(p) > 3 && p[1] == ':' && p[2] == '\\' {
			out = append(out, namedFolder{v, p})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return len(out[i].path) > len(out[j].path) })
	return out
}

// shownPath is p as shown: with the variable of the folder of the user that holds
// it, if any.
func shownPath(p string) string { return shownPathIn(p, userFolders()) }

func shownPathIn(p string, folders []namedFolder) string {
	if f, rest, ok := userFolderOf(p, folders); ok {
		return "%" + f.name + "%" + rest
	}
	return p
}

// userFolderOf is the folder of the user that holds p, if any, and the rest of p
// after it ("" or starting with a backslash).
func userFolderOf(p string, folders []namedFolder) (namedFolder, string, bool) {
	for _, f := range folders {
		if len(p) >= len(f.path) && strings.EqualFold(p[:len(f.path)], f.path) &&
			(len(p) == len(f.path) || p[len(f.path)] == '\\' || p[len(f.path)] == '/') {
			return f, p[len(f.path):], true
		}
	}
	return namedFolder{}, "", false
}

// psPath writes the path p in a PowerShell command: in a folder of the user, starting
// with the variable that names it ("$env:LOCALAPPDATA\Programs\App\app.exe"), else
// between single quotes (psQuote). In double quotes, PowerShell replaces $ and `
// sequences and ends the string at a straight or curly double quote: with any of these
// in the rest of the path, the rest stays in single quotes ($env:APPDATA + '\a$b').
// A command that runs as administrator (admin) keeps the full path.
func psPath(p string, admin bool) string {
	if admin {
		return psQuote(p)
	}
	return psPathIn(p, userFolders())
}

func psPathIn(p string, folders []namedFolder) string {
	f, rest, ok := userFolderOf(p, folders)
	switch {
	case !ok:
		return psQuote(p)
	case strings.ContainsAny(rest, "$`\"\u201c\u201d\u201e"):
		return "($env:" + f.name + " + " + psQuote(rest) + ")"
	}
	return `"$env:` + f.name + rest + `"`
}
