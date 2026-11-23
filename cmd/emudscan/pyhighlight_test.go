package main

import (
	"fmt"
	"strings"
	"testing"
)

// Put together, the pieces of each line give the script back, check_ssl.py included.
func TestPyTokensRoundTrip(t *testing.T) {
	for _, src := range []string{pyCheckScript, "", "x = '''a\nb # not a comment\nc''' + f\"{y}\"\n", "s = 'open"} {
		var got []string
		for _, toks := range pyTokens(src) {
			var b strings.Builder
			for _, tk := range toks {
				if tk.text == "" {
					t.Errorf("an empty piece in %q", src)
				}
				b.WriteString(tk.text)
			}
			got = append(got, b.String())
		}
		if strings.Join(got, "\n") != src {
			t.Errorf("not given back:\n%s\nwant:\n%s", strings.Join(got, "\n"), src)
		}
	}
}

func TestPyTokens(t *testing.T) {
	kinds := map[pyKind]string{pyName: "name", pySymbol: "symbol", pyKeyword: "keyword", pyBuiltin: "builtin",
		pyFunction: "function", pyString: "string", pyNumber: "number", pyComment: "comment"}
	show := func(toks []pyToken) string {
		var out []string
		for _, tk := range toks {
			if s := strings.TrimSpace(tk.text); s != "" {
				out = append(out, fmt.Sprintf("%s:%s", kinds[tk.kind], s))
			}
		}
		return strings.Join(out, " ")
	}
	for src, want := range map[string]string{
		`def check(name, fn):`:                 "keyword:def function:check symbol:( name:name symbol:, name:fn symbol:):",
		`class Box:`:                           "keyword:class function:Box symbol::",
		`    res[name] = [True, str(fn())]`:    "name:res symbol:[ name:name symbol:] = [ builtin:True symbol:, builtin:str symbol:( function:fn symbol:())]",
		`except BaseException as e:`:           "keyword:except builtin:BaseException keyword:as name:e symbol::",
		`"sha256": (hashlib.sha256(b"abc"),`:   `string:"sha256" symbol:: ( name:hashlib symbol:. function:sha256 symbol:( string:b"abc" symbol:),`,
		`if e.verify_code != 28:  # the list`:  "keyword:if name:e symbol:. name:verify_code symbol:!= number:28 symbol:: comment:# the list",
		`return "%d roots" % n`:                `keyword:return string:"%d roots" symbol:% name:n`,
		`x = 'it\'s' + r"\s" + 0x1F + .5`:      `name:x symbol:= string:'it\'s' symbol:+ string:r"\s" symbol:+ number:0x1F symbol:+ number:.5`,
		`bad = [k for k in kat if not k]`:      "name:bad symbol:= [ name:k keyword:for name:k keyword:in name:kat keyword:if keyword:not name:k symbol:]",
		`return lambda: handshake("c.pem", v)`: `keyword:return keyword:lambda symbol:: function:handshake symbol:( string:"c.pem" symbol:, name:v symbol:)`,
		`n = ssl.open(x) + open(x) + None`:     "name:n symbol:= name:ssl symbol:. function:open symbol:( name:x symbol:) + builtin:open symbol:( name:x symbol:) + builtin:None",
		`print(type(e).__name__, ValueError)`:  "builtin:print symbol:( builtin:type symbol:( name:e symbol:). name:__name__ symbol:, builtin:ValueError symbol:)",
		`raise ValueError("wrong: " + bad)`:    `keyword:raise builtin:ValueError symbol:( string:"wrong: " symbol:+ name:bad symbol:)`,
		`with open(os.path.join(D, ca)) as f:`: "keyword:with builtin:open symbol:( name:os symbol:. name:path symbol:. function:join symbol:( name:D symbol:, name:ca symbol:)) keyword:as name:f symbol::",
	} {
		if got := show(pyTokens(src)[0]); got != want {
			t.Errorf("%s\n got: %s\nwant: %s", src, got, want)
		}
	}
	// a string of three quotes goes on over its lines, a comment sign inside it too
	lines := pyTokens("x = \"\"\"a\n# b\nc\"\"\" + 1")
	if got := show(lines[1]); got != "string:# b" {
		t.Errorf("inside: %s", got)
	}
	if got := show(lines[2]); got != `string:c""" symbol:+ number:1` {
		t.Errorf("end: %s", got)
	}
}

// The colors are those chosen: light ones on a very dark blue, each kind its own but
// keywords and symbols, both soft white.
func TestPyColors(t *testing.T) {
	want := map[pyKind]string{pyName: "FFF17F", pySymbol: "FCFCFC", pyKeyword: "FCFCFC", pyBuiltin: "9BBAFF",
		pyFunction: "FF8F87", pyString: "C0EE96", pyNumber: "A5EEF7", pyComment: "B5AFA7"}
	for k, w := range want {
		if r, g, b := pyColor(k); fmt.Sprintf("%02X%02X%02X", r, g, b) != w {
			t.Errorf("kind %d: %02X%02X%02X, want %s", k, r, g, b, w)
		}
	}
	// COLORREF: 0x00BBGGRR
	if pyBackground != 0x1E1510 || pySignColor != 0x888F95 {
		t.Errorf("background %06X, signs %06X", pyBackground, pySignColor)
	}
}
