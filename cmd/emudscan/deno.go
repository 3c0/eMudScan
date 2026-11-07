package main

// Deno installed with its own installer is updated with "deno upgrade", Deno's own
// updater, which downloads its latest release; one installed with a package manager is
// updated with that package manager (packageAdvice).

import "path/filepath"

// denoOwnInstall: exe is the Deno that Deno's own installer put in place (in
// %DENO_INSTALL%\bin, else %USERPROFILE%\.deno\bin), which "deno upgrade" updates; a
// package manager's is updated with that one.
func denoOwnInstall(exe string) bool {
	var dirs []string
	if d := getenv("DENO_INSTALL"); d != "" {
		dirs = append(dirs, filepath.Join(d, "bin"))
	}
	if u := getenv("USERPROFILE"); u != "" {
		dirs = append(dirs, filepath.Join(u, ".deno", "bin"))
	}
	for _, d := range dirs {
		if pathKey(filepath.Dir(exe)) == pathKey(d) && lower(filepath.Base(exe)) == "deno.exe" {
			return true
		}
	}
	return false
}

// denoUpgradeAdvice is the update of the Deno exe with its own updater, run on that
// copy, whatever deno.exe the PATH finds first.
func denoUpgradeAdvice(exe string) string {
	f := newFix(fixAction{Kind: fixPackage, Manager: "Deno", Admin: needsAdmin(exe), Cmds: []string{"& " + psPath(exe, needsAdmin(exe)) + " upgrade"}})
	return T("deno.upgrade") + "\n" + f.lines()
}
