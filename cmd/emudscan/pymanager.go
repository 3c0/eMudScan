package main

// The Python Install Manager (its py and pymanager commands), python.org's way of
// installing Python on Windows, installs each Python in a folder of its own
// (%LOCALAPPDATA%\Python\pythoncore-3.14-64…), which its __install__.json records,
// and updates it with "pymanager install --update <tag>", keeping its installed
// packages. eMudScan proposes that update only when the manager is installed, and only
// for the release lines that python.org publishes Windows builds of: from 3.14 (3.11
// to 3.13 get their security releases as source code only). The Pythons installed
// otherwise (python.org's installers, programs) are left to their own updates.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// pymanagerFirstLine is the oldest release line with a fixed Windows build in the
// Python Install Manager.
const pymanagerFirstLine = "3.14"

// pymanagerRuntime is a Python that the Python Install Manager installed: its
// company (PythonCore for python.org's) and tag (3.14-64).
type pymanagerRuntime struct{ company, tag string }

// safeTag: what an __install__.json may name in a command, without quoting.
var safeTag = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// pymanagerRuntimeOf reads the __install__.json of the Python in home, if the Python
// Install Manager installed it.
func pymanagerRuntimeOf(home string) (pymanagerRuntime, bool) {
	data, err := os.ReadFile(filepath.Join(home, "__install__.json"))
	if err != nil || len(data) > 1<<20 {
		return pymanagerRuntime{}, false
	}
	var j struct {
		Schema       int
		Company, Tag string
	}
	if json.Unmarshal(data, &j) != nil || j.Schema != 1 || !safeTag.MatchString(j.Company) || !safeTag.MatchString(j.Tag) {
		return pymanagerRuntime{}, false
	}
	return pymanagerRuntime{j.Company, j.Tag}, true
}

// arg names the runtime in a command: its tag, after its company when it is not
// python.org's.
func (r pymanagerRuntime) arg() string {
	if strings.EqualFold(r.company, "PythonCore") {
		return r.tag
	}
	return r.company + `\` + r.tag
}

// pymanagerInstalled: the pymanager command exists, from the Microsoft Store (an alias
// in WindowsApps) or from its MSI (in Program Files); both put it on the PATH.
func pymanagerInstalled() bool {
	for _, p := range [][]string{{getenv("LOCALAPPDATA"), "Microsoft", "WindowsApps"}, {getenv("ProgramFiles"), "PyManager"}} {
		if p[0] == "" {
			continue
		}
		if _, err := os.Lstat(filepath.Join(append(p, "pymanager.exe")...)); err == nil {
			return true
		}
	}
	return false
}

// pymanagerAdvice is how the Python Install Manager updates the Python of fx, which it
// installed as rt: with a fix for a line it has a fixed build of, else a newer line.
func pymanagerAdvice(rt pymanagerRuntime, fx pyFix) string {
	line := lineOf(ParseVersion(fx.version), 2)
	if line == nil {
		return ""
	}
	if fx.state == FixOutdated && line.Cmp(ParseVersion(pymanagerFirstLine)) >= 0 {
		f := newFix(fixAction{Kind: fixPackage, Cmds: []string{"pymanager install --update -y " + rt.arg()}, Manager: "Python Install Manager"})
		return T("py.pymanagerFix") + "\n" + f.lines()
	}
	return Tf("py.pymanagerNewer", line.String(), pymanagerFirstLine, pymanagerFirstLine) + "\n" + cmd("pymanager install "+pymanagerFirstLine)
}
