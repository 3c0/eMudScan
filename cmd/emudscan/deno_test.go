package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// Deno installed with its own installer is updated with its own updater, run on that
// copy; one installed otherwise is not.
func TestDenoUpgrade(t *testing.T) {
	Lang = 0
	user := t.TempDir()
	t.Setenv("USERPROFILE", user)
	t.Setenv("DENO_INSTALL", "")
	own := filepath.Join(user, ".deno", "bin", "deno.exe")
	if !denoOwnInstall(own) || denoOwnInstall(filepath.Join(user, "scoop", "apps", "deno", "current", "deno.exe")) {
		t.Errorf("own install")
	}
	t.Setenv("DENO_INSTALL", filepath.Join(user, "tools", "deno"))
	if !denoOwnInstall(filepath.Join(user, "tools", "deno", "bin", "deno.exe")) {
		t.Errorf("DENO_INSTALL")
	}
	a := denoUpgradeAdvice(own)
	f := fixesIn(a)
	if !strings.HasPrefix(a, T("deno.upgrade")+"\n") || len(f) != 1 || f[0].Kind != fixPackage || f[0].Manager != "Deno" || f[0].Optional ||
		f[0].key() != "& "+psPath(own, false)+" upgrade" || strings.Join(f[0].Servers, " ") != "dl.deno.land" {
		t.Errorf("%+v\n%s", f, a)
	}
}
