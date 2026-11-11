package main

// The command line of eMudScan.exe, the window (--lang, --offline), and of
// eMudScan-cli.exe, the scan without the window (cliscan.go).

import (
	"slices"
	"strings"
)

// cliOptions are the options of the command line.
type cliOptions struct {
	lang    int      // --lang en: 0, --lang fr: 1; -1: the language of Windows
	offline bool     // --offline: the window starts with the online test cleared; the scan runs it only with --online
	reports []string // --report FILE, each: the files that eMudScan-cli.exe writes
	drives  []string // --drives: the roots that it scans ("C:\"), none when empty; nil: the drives that the window ticks
	online  bool     // --online: it runs the online test (unless --offline is given too)
	help    bool     // --help: it shows its options
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
// argument it is about, written in the language of the scan, which is known only
// once the whole command line is read.
type cliError struct{ key, arg string }

func (e cliError) Error() string { return Tf(e.key, e.arg) }

// parseArgs reads the command line. err is what is wrong with it, if anything.
// eMudScan-cli.exe then stops: a scan that nobody watches must not run with an
// option it does not know. The window ignores err, the options it does not know and
// those of eMudScan-cli.exe. Options start with "-" or "/", in upper or lower case; a
// value follows its option as the next argument or after "=".
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
		case (name == "help" || name == "h" || name == "?") && !hasValue:
			o.help = true
		case name == "report":
			if v, ok := next(); ok {
				o.reports = append(o.reports, v)
			} else {
				fail(cliError{"cli.noValue", arg})
			}
		case name == "drives":
			v, ok := next()
			if !ok {
				fail(cliError{"cli.noValue", arg})
			} else if o.drives, ok = parseDrives(v); !ok {
				fail(cliError{"cli.badDrives", v})
			}
		case name == "online" && !hasValue:
			o.online = true
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
	o.online = o.online && !o.offline
	return o, err
}

// parseDrives reads a list of drives, such as "C:,D:" ("c", "C:" and "C:\" are the
// same drive), into their roots; "none" is the empty list.
func parseDrives(s string) ([]string, bool) {
	out := []string{}
	if lower(strings.TrimSpace(s)) == "none" {
		return out, true
	}
	for _, d := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ';' }) {
		d = strings.ToUpper(strings.TrimSuffix(strings.TrimSuffix(strings.TrimSpace(d), `\`), ":"))
		if len(d) != 1 || d[0] < 'A' || d[0] > 'Z' {
			return nil, false
		}
		if root := d + `:\`; !slices.Contains(out, root) {
			out = append(out, root)
		}
	}
	return out, len(out) > 0
}
