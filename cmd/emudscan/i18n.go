package main

import (
	"fmt"
	"strings"
)

// Lang 0 = English, 1 = French.
var Lang = 0

// T returns the translation of key in the current language.
func T(key string) string {
	s, ok := tr[key]
	if !ok {
		return "!" + key
	}
	if Lang == 1 && s[1] != "" {
		return s[1]
	}
	return s[0]
}

// Tf formats the translation of key.
func Tf(key string, args ...any) string { return fmt.Sprintf(T(key), args...) }

func setLangFromCode(code string) {
	if strings.HasPrefix(strings.ToLower(code), "fr") {
		Lang = 1
	} else {
		Lang = 0
	}
}
