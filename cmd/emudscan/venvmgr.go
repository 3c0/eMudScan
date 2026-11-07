package main

// Virtual environments that a manager keeps, which pip alone does not update for good:
//
//   - pipx installs each application in an environment of its own, without pip
//     (pipx links its own shared pip into it, except with its uv backend): "pipx
//     runpip" runs pip in it, whatever its backend. "pipx upgrade" would update the
//     application only, not certifi.
//   - Pipenv, Poetry and uv keep a project's environment as its lock file says
//     (Pipfile.lock, poetry.lock, uv.lock): the project's next install brings back the
//     certifi it names. The command that updates the lock file is shown, never run:
//     it changes the project, which is the developer's decision.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// envNameRe: the name of a pipx application, as pipx names its folder, fit for a
// command.
var envNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// pipxHome is the folder of the pipx of this user, as pipx chooses it: the one that
// PIPX_HOME names, else the older ones when they exist (%USERPROFILE%\.local\pipx,
// %USERPROFILE%\pipx), else %LOCALAPPDATA%\pipx\pipx; "" when it cannot be known.
func pipxHome() string {
	if h := getenv("PIPX_HOME"); h != "" {
		if strings.ContainsAny(h, "%~") || !filepath.IsAbs(h) && !winAbs(h) { // pipx expands these itself
			return ""
		}
		return filepath.Clean(h)
	}
	if u := getenv("USERPROFILE"); u != "" {
		for _, f := range []string{filepath.Join(u, ".local", "pipx"), filepath.Join(u, "pipx")} {
			if exists(f) {
				return f
			}
		}
	}
	if l := getenv("LOCALAPPDATA"); l != "" {
		return filepath.Join(l, "pipx", "pipx")
	}
	return ""
}

// isPipxApp: the virtual environment env is that of an application that pipx
// installed, beside its record (pipx_metadata.json).
func isPipxApp(env pyEnv) bool {
	return env.venv && exists(filepath.Join(env.home, "pipx_metadata.json")) && envNameRe.MatchString(filepath.Base(env.home))
}

// pipxAdvice is how pipx updates pkg in its application in home: when the pipx of
// this user keeps its folder and is on the PATH, with a fix; else the command alone.
func pipxAdvice(home, pkg string) string {
	run := "pipx runpip " + filepath.Base(home) + " install --upgrade " + pkg
	text := Tf("py.pipx", pkg)
	venvs := filepath.Dir(home)
	if h := pipxHome(); h == "" || pathKey(filepath.Join(h, "venvs")) != pathKey(venvs) {
		return text + "\n" + cmd("$env:PIPX_HOME = "+psPath(filepath.Dir(venvs), false)+"; "+run)
	}
	if !onPath("pipx.exe") {
		return text + "\n" + cmd(run)
	}
	return text + "\n" + newFix(fixAction{Kind: fixPackage, Manager: "pipx", Cmds: []string{run}, Servers: managerServers["pipx"]}).lines()
}

// projectLock is the lock file of the project of a virtual environment, which names
// the version of a package installed in it.
type projectLock struct {
	project, file, version, update string
}

// lockOf finds the project of the virtual environment env (the folder of its .venv,
// or the one that Pipenv records in its .project) and its lock file, when that one
// names the version of pkg installed in env: the project's next install keeps it.
func lockOf(env pyEnv, pkg string) (projectLock, bool) {
	if !env.venv {
		return projectLock{}, false
	}
	project := ""
	if lower(filepath.Base(env.home)) == ".venv" {
		project = filepath.Dir(env.home)
	} else if b, err := os.ReadFile(filepath.Join(env.home, ".project")); err == nil {
		if p := strings.TrimSpace(string(b)); (filepath.IsAbs(p) || winAbs(p)) && isDir(p) {
			project = p
		}
	}
	version := distInfoVersion(env.sitePackages, pkg)
	if project == "" || version == "" {
		return projectLock{}, false
	}
	byUV := pyvenvValue(filepath.Join(env.home, "pyvenv.cfg"), "uv") != "" // uv made it
	for _, l := range []struct {
		file, update string
		ok           bool
	}{
		{"uv.lock", "uv sync --upgrade-package " + pkg, byUV},
		{"Pipfile.lock", "pipenv update " + pkg, true},
		{"poetry.lock", "poetry update " + pkg, true},
	} {
		data, err := os.ReadFile(filepath.Join(project, l.file))
		if !l.ok || err != nil {
			continue
		}
		if lockedVersion(l.file, data, pkg) == version {
			return projectLock{project, l.file, version, l.update}, true
		}
		return projectLock{}, false // it names another version: the next install changes it
	}
	return projectLock{}, false
}

// lockedVersion is the version of pkg that a lock file names: in Pipfile.lock (JSON),
// "==2024.7.4"; in poetry.lock and uv.lock (TOML), a [[package]] with its name, then
// its version.
func lockedVersion(file string, data []byte, pkg string) string {
	if file == "Pipfile.lock" {
		var lock map[string]json.RawMessage
		if json.Unmarshal(data, &lock) != nil {
			return ""
		}
		for _, group := range []string{"default", "develop"} {
			var pkgs map[string]struct {
				Version string `json:"version"`
			}
			if json.Unmarshal(lock[group], &pkgs) == nil && pkgs[pkg].Version != "" {
				return strings.TrimPrefix(pkgs[pkg].Version, "==")
			}
		}
		return ""
	}
	re := regexp.MustCompile(`(?m)^name = "` + regexp.QuoteMeta(pkg) + `"\r?\nversion = "([^"]+)"`)
	if m := re.FindSubmatch(data); m != nil {
		return string(m[1])
	}
	return ""
}

// lockAdvice is the command that updates pkg in the lock file l, in its project:
// after a command that updates the environment (too), or as the recommended action.
func lockAdvice(l projectLock, pkg string, too bool) string {
	key := "py.lockFix"
	if too {
		key = "py.lockToo"
	}
	return Tf(key, l.file, pkg, l.version) + "\n" + cmd("Set-Location -LiteralPath "+psPath(l.project, false)+"; "+l.update)
}
