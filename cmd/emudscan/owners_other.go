//go:build !windows

package main

import "os"

// On other systems, where the checks are tested, files belong to administrators.

func readFileSecurity(path string) (fileSec, error) {
	if _, err := os.Lstat(path); err != nil {
		return fileSec{}, err
	}
	return fileSec{owner: sidAdministrators}, nil
}

func readAccountName(sid string) string { return sid }

func readCurrentUserSID() string { return "" }
