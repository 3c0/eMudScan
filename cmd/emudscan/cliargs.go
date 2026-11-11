package main

// The command line of eMudScan.exe: --lang and --offline.

import "strings"

// cliOptions are the options of the command line.
type cliOptions struct {
	lang    int  // --lang en: 0, --lang fr: 1; -1: the language of Windows
	offline bool // --offline: the window starts with the online test cleared
}

// langOption is the language that the value of --lang names.
func langOption(v string) (lang int, ok bool) {
	switch lower(strings.TrimSpace(v)) {
	case "en":
		return 0, true
	case "fr":
		return 1, true
	}
	return -1, false
}

// cliError is what is wrong with the command line: a text of texts.go with the
// argument it is about, written in the language chosen, which is known only once the
// whole command line is read.
type cliError struct{ key, arg string }

func (e cliError) Error() string { return Tf(e.key, e.arg) }

// parseArgs reads the command line. err is what is wrong with it, if anything; the
// window ignores it, and the options it does not know. Options start with "-" or
// "/", in upper or lower case; a value follows its option as the next argument or
// after "=".
func parseArgs(args []string) (o cliOptions, err error) {
	o.lang = -1
	fail := func(e error) {
		if err == nil {
			err = e
		}
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		name, value, hasValue := strings.Cut(strings.TrimLeft(arg, "-/"), "=")
		name = lower(name)
		next := func() (string, bool) {
			if !hasValue && i+1 < len(args) {
				i++
				value, hasValue = args[i], true
			}
			return value, hasValue && value != ""
		}
		switch {
		case !strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "/"):
			fail(cliError{"cli.unexpected", arg})
		case name == "offline" && !hasValue:
			o.offline = true
		case name == "lang":
			if v, ok := next(); !ok {
				fail(cliError{"cli.noValue", arg})
			} else if l, ok := langOption(v); ok {
				o.lang = l
			} else {
				fail(cliError{"cli.badLang", v})
			}
		default:
			fail(cliError{"cli.unknown", arg})
		}
	}
	return o, err
}
