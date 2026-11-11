package main

import (
	"reflect"
	"testing"
)

// The command line of the window: --lang and --offline. What is wrong with it is
// reported, in the language chosen; the window ignores it.
func TestParseArgs(t *testing.T) {
	Lang = 0
	for _, x := range []struct {
		args []string
		want cliOptions
		err  string
	}{
		{nil, cliOptions{lang: -1}, ""},
		{[]string{"--lang", "fr", "--offline"}, cliOptions{lang: 1, offline: true}, ""},
		{[]string{"/OFFLINE", "-Lang", "EN"}, cliOptions{lang: 0, offline: true}, ""},
		{[]string{"--lang=fr"}, cliOptions{lang: 1}, ""},
		{[]string{"--fr"}, cliOptions{lang: -1}, "Unknown option: --fr"}, // replaced by --lang
		{[]string{"--en"}, cliOptions{lang: -1}, "Unknown option: --en"},
		{[]string{"--lang", "fr", "--other"}, cliOptions{lang: 1}, "Unknown option: --other"},
		{[]string{"file.txt"}, cliOptions{lang: -1}, "Unexpected argument: file.txt"},
		{[]string{"--lang"}, cliOptions{lang: -1}, "The option --lang needs a value"},
		{[]string{"--lang", "de"}, cliOptions{lang: -1}, "Not a language, en or fr: de"},
		{[]string{"--offline=yes"}, cliOptions{lang: -1}, "Unknown option: --offline=yes"},
	} {
		o, err := parseArgs(x.args)
		msg := ""
		if err != nil {
			msg = err.Error()
		}
		if msg != x.err || !reflect.DeepEqual(o, x.want) {
			t.Errorf("%q: %+v, error %q; want %+v, %q", x.args, o, msg, x.want, x.err)
		}
	}
	// the error is written in the language chosen, known once the line is read
	_, err := parseArgs([]string{"--other", "--lang", "fr"})
	Lang = 1
	defer func() { Lang = 0 }()
	if err == nil || err.Error() != "Option inconnue\u00a0: --other" {
		t.Errorf("in French: %v", err)
	}
}
