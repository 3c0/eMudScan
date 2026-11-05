package main

// Commands to run as administrator, and files that other users can change. Any user
// can create folders in C:\ProgramData, and add files to the folders created there by
// others unless their installer restricted it. So a python.exe, a module that Python
// loads when it starts, or a file named like an eMudScan backup may have been put
// there by another user. Run as administrator, the pip commands would run what they
// put there, and restoring a backup would put their file in place of the real one.
// eMudScan proposes these commands only when the files and folders involved belong to
// administrators, SYSTEM, TrustedInstaller or the user, and no one else may add files
// to the folders that Python loads code from.

import (
	"os"
	"path/filepath"
	"sync"
)

// Security identifiers (SIDs) of the accounts that may change the system's files.
const (
	sidSystem           = "S-1-5-18"
	sidAdministrators   = "S-1-5-32-544"
	sidTrustedInstaller = "S-1-5-80-956008885-3418522649-1831038044-1853292631-2271478464"
	sidEveryone         = "S-1-1-0"
	sidCreatorOwner     = "S-1-3-0" // stand-ins, replaced by the creator of each new file
	sidCreatorGroup     = "S-1-3-1"
	sidOwnerRights      = "S-1-3-4"
)

// changeRights: the rights that let an account change a file or folder: write to it
// (for a folder, add a file or a folder to it), delete it or what it holds, or change
// its permissions or its owner.
const changeRights = 0x2 | 0x4 | 0x40 | 0x10000 | 0x40000 | 0x80000 | 0x10000000 | 0x40000000

// fileSec is who owns a file or folder, and the accounts that its permissions allow
// to change it.
type fileSec struct {
	owner   string // SID
	writers []string
}

// fileSecurity reads the security of a file or folder; accountName names a SID
// (DOMAIN\name); currentUserSID is the SID of the user running eMudScan. The tests
// replace them.
var (
	fileSecurity   = readFileSecurity
	accountName    = readAccountName
	currentUserSID = sync.OnceValue(readCurrentUserSID)
)

// trustedSID: an account that may change the system's files anyway, or the user.
func trustedSID(sid string) bool {
	switch sid {
	case sidSystem, sidAdministrators, sidTrustedInstaller:
		return true
	}
	u := currentUserSID()
	return u != "" && sid == u
}

// foreignOwner finds, among paths and the folders above them, one that belongs to
// another account, which it names: the highest such folder, which may hold the others.
// A file or folder whose owner cannot be read counts as such. Paths that do not exist
// are passed over.
func foreignOwner(paths ...string) (path, owner string, found bool) {
	seen := map[string]bool{}
	for _, p := range paths {
		if _, err := os.Lstat(p); err != nil {
			continue
		}
		var chain []string // from the root of the drive down to p
		for q := p; ; q = filepath.Dir(q) {
			chain = append([]string{q}, chain...)
			if filepath.Dir(q) == q {
				break
			}
		}
		for _, q := range chain {
			if seen[pathKey(q)] {
				continue
			}
			seen[pathKey(q)] = true
			s, err := fileSecurity(q)
			if err != nil {
				return q, T("owner.unknown"), true
			}
			if !trustedSID(s.owner) {
				return q, accountName(s.owner), true
			}
		}
	}
	return "", "", false
}

// othersCanChange finds, among dirs, the first one that another account may change
// (add a file to, for a folder), which it names. Folders that do not exist are
// passed over; one whose permissions cannot be read counts as such.
func othersCanChange(dirs ...string) (path, who string, found bool) {
	for _, d := range dirs {
		if _, err := os.Lstat(d); err != nil {
			continue
		}
		s, err := fileSecurity(d)
		if err != nil {
			return d, T("owner.unknown"), true
		}
		for _, w := range s.writers {
			switch w {
			case sidCreatorOwner, sidCreatorGroup, sidOwnerRights:
				continue
			}
			if !trustedSID(w) {
				return d, accountName(w), true
			}
		}
	}
	return "", "", false
}

// pipUnsafe is why the pip commands of a Python must not be run as administrator, or
// "" when they may (or need no administrator rights). They run its python.exe (for a
// virtual environment, its base Python's), which loads DLLs from its folder and
// modules from Lib, DLLs and site-packages, and the .pth files of site-packages.
func pipUnsafe(env pyEnv) string {
	if !needsAdmin(env.exe) && !needsAdmin(env.sitePackages) {
		return ""
	}
	files := []string{env.exe, env.sitePackages}
	homes := []string{env.home, filepath.Dir(env.exe)}
	if env.venv {
		if base := pyvenvHome(filepath.Join(env.home, "pyvenv.cfg")); base != "" {
			files = append(files, filepath.Join(base, "python.exe"))
			homes = append(homes, base)
		}
	}
	if p, who, ok := foreignOwner(files...); ok {
		return Tf("pip.ownedBy", lk(p), plain(who))
	}
	// the Python's own folder first: its permissions usually apply to the folders
	// inside it, so it is the one to fix
	var dirs []string
	for _, h := range homes {
		dirs = append(dirs, h, filepath.Join(h, "Lib"), filepath.Join(h, "DLLs"))
	}
	if p, who, ok := othersCanChange(append(dirs, env.sitePackages)...); ok {
		return Tf("pip.changeableBy", lk(p), plain(who))
	}
	return ""
}

// restoreUnsafe is why eMudScan does not offer to put back file from its backup, or
// "" when it may: when the command needs administrator rights, the backup, the file
// and their folders must belong to administrators, SYSTEM, TrustedInstaller or the
// user.
func restoreUnsafe(file, backup string) string {
	if !needsAdmin(file) {
		return ""
	}
	if p, who, ok := foreignOwner(backup, file); ok {
		return Tf("restore.ownedBy", lk(file), lk(p), plain(who))
	}
	return ""
}
