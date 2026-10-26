package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var verbRe = regexp.MustCompile(`%(\[\d+\])?[-+# 0]*\d*(\.\d+)?[vdsqxXf]`)

func TestTranslations(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	keyRe := regexp.MustCompile(`\bTf?\("([a-zA-Z0-9_.]+)"`)
	used := map[string]bool{}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		data, _ := os.ReadFile(f)
		for _, m := range keyRe.FindAllStringSubmatch(string(data), -1) {
			used[m[1]] = true
		}
	}
	for k := range used {
		if strings.HasSuffix(k, ".") {
			continue // prefix of keys built at run time
		}
		v, ok := tr[k]
		if !ok {
			t.Errorf("missing translation key %q", k)
			continue
		}
		if v[0] == "" || v[1] == "" {
			t.Errorf("empty translation for %q", k)
		}
		en, fr := verbRe.FindAllString(v[0], -1), verbRe.FindAllString(v[1], -1)
		if len(en) != len(fr) {
			t.Errorf("%q: verbs differ: %v vs %v", k, en, fr)
		}
	}
	for k := range tr {
		if !used[k] && !strings.HasPrefix(k, "status.") {
			t.Logf("unused key %q", k)
		}
	}
}
