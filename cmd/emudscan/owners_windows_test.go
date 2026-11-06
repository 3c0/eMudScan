package main

import (
	"strings"
	"testing"
)

// Windows reads the owner of a folder that the test creates: the user, or the
// administrators when the test runs as administrator.
func TestReadFileSecurity(t *testing.T) {
	u := readCurrentUserSID()
	if !strings.HasPrefix(u, "S-1-5-") {
		t.Fatalf("current user: %q", u)
	}
	s, err := readFileSecurity(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !trustedSID(s.owner) {
		t.Errorf("owner of a new folder: %s (%s), user %s", s.owner, readAccountName(s.owner), u)
	}
	if n := readAccountName(sidAdministrators); !strings.Contains(n, `\`) {
		t.Errorf("name of the administrators: %q", n)
	}
}
