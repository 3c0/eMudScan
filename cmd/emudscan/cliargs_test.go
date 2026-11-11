package main

import (
	"reflect"
	"testing"
)

// The command line: --lang and --offline for the window; the other options for
// eMudScan-cli.exe, for which a wrong option is an error, since nobody watches its
// scan.
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
		{[]string{"--report", `C:\Reports\pc.json`, "--report=pc.csv", "--drives", "c:,D", "--online", "--lang=fr"},
			cliOptions{reports: []string{`C:\Reports\pc.json`, "pc.csv"}, drives: []string{`C:\`, `D:\`}, online: true, lang: 1}, ""},
		{[]string{"--drives=none"}, cliOptions{drives: []string{}, lang: -1}, ""},
		{[]string{"--online", "--offline"}, cliOptions{lang: -1, offline: true}, ""}, // --offline wins
		{[]string{"--report", "/reports/pc.json"}, cliOptions{reports: []string{"/reports/pc.json"}, lang: -1}, ""},
		{[]string{"--help"}, cliOptions{lang: -1, help: true}, ""},
		{[]string{"/?"}, cliOptions{lang: -1, help: true}, ""},
		{[]string{"-h", "--lang", "fr"}, cliOptions{lang: 1, help: true}, ""},
		{[]string{"--reprot", "pc.json"}, cliOptions{lang: -1}, "Unknown option: --reprot"},
		{[]string{"--scan"}, cliOptions{lang: -1}, "Unknown option: --scan"},
		{[]string{"pc.json"}, cliOptions{lang: -1}, "Unexpected argument: pc.json"},
		{[]string{"--report"}, cliOptions{lang: -1}, "The option --report needs a value"},
		{[]string{"--report="}, cliOptions{lang: -1}, "The option --report= needs a value"},
		{[]string{"--lang"}, cliOptions{lang: -1}, "The option --lang needs a value"},
		{[]string{"--lang", "de"}, cliOptions{lang: -1}, "Not a language, en or fr: de"},
		{[]string{"--drives", "C:D:"}, cliOptions{lang: -1}, "Not a list of drives, such as C:,D: or none: C:D:"},
		{[]string{"--help=yes"}, cliOptions{lang: -1}, "Unknown option: --help=yes"},
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
	// the error is written in the language of the scan, known once the line is read
	_, err := parseArgs([]string{"--reprot", "--lang", "fr"})
	Lang = 1
	defer func() { Lang = 0 }()
	if err == nil || err.Error() != "Option inconnue\u00a0: --reprot" {
		t.Errorf("in French: %v", err)
	}
}

func TestParseDrives(t *testing.T) {
	for in, want := range map[string][]string{
		"C:": {`C:\`}, "c": {`C:\`}, `C:\`: {`C:\`}, "C:,D:": {`C:\`, `D:\`}, "d;c; C:": {`D:\`, `C:\`}, " none ": {}, "NONE": {},
		"": nil, ",": nil, "CD": nil, "C:D:": nil, "1:": nil, `C:\Users`: nil, "C:,none": nil,
	} {
		got, ok := parseDrives(in)
		if ok != (want != nil) || ok && !reflect.DeepEqual(got, want) {
			t.Errorf("parseDrives(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
}
