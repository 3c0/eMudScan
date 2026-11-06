package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The Java that Bazel extracts into its install base belongs to Bazel, of the
// version written there: the advice is Bazel's update, not Java's.
func TestBazelOwner(t *testing.T) {
	Lang = 0
	root := t.TempDir()
	base := filepath.Join(root, "_bazel_me", "install", "0123456789abcdef0123456789abcdef")
	home := filepath.Join(base, "embedded_tools", "jdk")
	sec := filepath.Join(home, "lib", "security")
	os.MkdirAll(sec, 0o755)
	os.WriteFile(filepath.Join(base, "build-label.txt"), []byte("8.8.1\n"), 0o644)
	os.WriteFile(filepath.Join(sec, "cacerts"), append([]byte("padding"), rootByName(rootNameECCG3).DER...), 0o644)

	if o, ok := bazelOwner(home); !ok || o.Name != "Bazel" || o.Version != "8.8.1" || o.Loc != base {
		t.Errorf("install base: %+v %v", o, ok)
	}
	for _, p := range []string{
		filepath.Join(root, "_bazel_me", "0123456789abcdef0123456789abcdef", "external", "remotejdk", "lib"), // an output base
		filepath.Join(root, "bazel", "install", "0123456789abcdef0123456789abcdef"),
		filepath.Join(root, "_bazel_me", "install", "0123"),
	} {
		if o, ok := bazelOwner(p); ok {
			t.Errorf("%s: %+v", p, o)
		}
	}
	tp := testApps(t)
	it := tp.c.begin(GroupApps, "java", "Java", home, func(it *Item) { tp.javaOne(it, home) })
	if it.Status != StWarn || !strings.Contains(it.Details, "update Bazel, which this copy belongs to,") || strings.Contains(it.Details, T("java.fix")) {
		t.Errorf("status %v:\n%s", it.Status, it.Details)
	}
}
