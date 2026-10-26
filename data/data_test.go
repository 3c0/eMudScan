package data

import (
	"bytes"
	"os"
	"testing"
)

// The program shows its license from the copy in this folder: it must stay
// identical to the LICENSE file at the root of the repository.
func TestLicenseCopy(t *testing.T) {
	root, err := os.ReadFile("../LICENSE")
	if err != nil {
		t.Fatal(err)
	}
	embedded, err := FS.ReadFile("LICENSE-emudscan.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(root, embedded) {
		t.Error("data/LICENSE-emudscan.txt differs from LICENSE: copy LICENSE over it")
	}
}
