package main

import "strings"

// The colors of a Python script, such as check_ssl.py when the Fix window shows it: light
// colors on a very dark blue, other than those of the PowerShell commands, so that the
// two are told apart at first sight.

type pyKind int

const (
	pyName     pyKind = iota // a variable, a module, an attribute: sys, res, version
	pySymbol                 // ( ) [ ] { } : , . = + % !=, and the spaces
	pyKeyword                // def, import, return, if, not…
	pyBuiltin                // a built-in of Python: str, len, ValueError, True, None
	pyFunction               // a function, where it is defined or called: check, hexdigest
	pyString                 // "…", '…', b"…", and those of three quotes
	pyNumber                 // 0, 28, 0x1F, .5
	pyComment                // # to the end of the line
)

// pyBackground is the background of a Python script, and pySignColor the color of its
// wrap signs (wrapEnd, wrapStart).
const (
	pyBackground = 0x1E1510 // rgb(16,21,30): a very dark blue
	pySignColor  = 0x888F95 // rgb(149,143,136)
)

// pyColor is the color of a kind, as rgb() takes it.
func pyColor(k pyKind) (r, g, b byte) {
	switch k {
	case pySymbol, pyKeyword:
		return 0xFC, 0xFC, 0xFC // soft white
	case pyBuiltin:
		return 0x9B, 0xBA, 0xFF // light blue
	case pyFunction:
		return 0xFF, 0x8F, 0x87 // light red
	case pyString:
		return 0xC0, 0xEE, 0x96 // light green
	case pyNumber:
		return 0xA5, 0xEE, 0xF7 // light cyan
	case pyComment:
		return 0xB5, 0xAF, 0xA7 // warm grey
	}
	return 0xFF, 0xF1, 0x7F // yellow: the names
}

type pyToken struct {
	kind pyKind
	text string
}

var pyKeywords = map[string]bool{"and": true, "as": true, "assert": true, "async": true, "await": true, "break": true,
	"class": true, "continue": true, "def": true, "del": true, "elif": true, "else": true, "except": true,
	"finally": true, "for": true, "from": true, "global": true, "if": true, "import": true, "in": true, "is": true,
	"lambda": true, "nonlocal": true, "not": true, "or": true, "pass": true, "raise": true, "return": true,
	"try": true, "while": true, "with": true, "yield": true}

// pyBuiltins are the built-ins of Python 3.13 (dir(builtins), without those that start
// with _), True, False and None.
var pyBuiltins = map[string]bool{"True": true, "False": true, "None": true,
	"ArithmeticError": true, "AssertionError": true, "AttributeError": true, "BaseException": true,
	"BaseExceptionGroup": true, "BlockingIOError": true, "BrokenPipeError": true, "BufferError": true,
	"BytesWarning": true, "ChildProcessError": true, "ConnectionAbortedError": true, "ConnectionError": true,
	"ConnectionRefusedError": true, "ConnectionResetError": true, "DeprecationWarning": true, "EOFError": true,
	"Ellipsis": true, "EncodingWarning": true, "EnvironmentError": true, "Exception": true,
	"ExceptionGroup": true, "FileExistsError": true, "FileNotFoundError": true, "FloatingPointError": true,
	"FutureWarning": true, "GeneratorExit": true, "IOError": true, "ImportError": true, "ImportWarning": true,
	"IndentationError": true, "IndexError": true, "InterruptedError": true, "IsADirectoryError": true,
	"KeyError": true, "KeyboardInterrupt": true, "LookupError": true, "MemoryError": true,
	"ModuleNotFoundError": true, "NameError": true, "NotADirectoryError": true, "NotImplemented": true,
	"NotImplementedError": true, "OSError": true, "OverflowError": true, "PendingDeprecationWarning": true,
	"PermissionError": true, "ProcessLookupError": true, "PythonFinalizationError": true,
	"RecursionError": true, "ReferenceError": true, "ResourceWarning": true, "RuntimeError": true,
	"RuntimeWarning": true, "StopAsyncIteration": true, "StopIteration": true, "SyntaxError": true,
	"SyntaxWarning": true, "SystemError": true, "SystemExit": true, "TabError": true, "TimeoutError": true,
	"TypeError": true, "UnboundLocalError": true, "UnicodeDecodeError": true, "UnicodeEncodeError": true,
	"UnicodeError": true, "UnicodeTranslateError": true, "UnicodeWarning": true, "UserWarning": true,
	"ValueError": true, "Warning": true, "ZeroDivisionError": true, "abs": true, "aiter": true, "all": true,
	"anext": true, "any": true, "ascii": true, "bin": true, "bool": true, "breakpoint": true, "bytearray": true,
	"bytes": true, "callable": true, "chr": true, "classmethod": true, "compile": true, "complex": true,
	"copyright": true, "credits": true, "delattr": true, "dict": true, "dir": true, "divmod": true,
	"enumerate": true, "eval": true, "exec": true, "exit": true, "filter": true, "float": true, "format": true,
	"frozenset": true, "getattr": true, "globals": true, "hasattr": true, "hash": true, "help": true,
	"hex": true, "id": true, "input": true, "int": true, "isinstance": true, "issubclass": true, "iter": true,
	"len": true, "license": true, "list": true, "locals": true, "map": true, "max": true, "memoryview": true,
	"min": true, "next": true, "object": true, "oct": true, "open": true, "ord": true, "pow": true,
	"print": true, "property": true, "quit": true, "range": true, "repr": true, "reversed": true, "round": true,
	"set": true, "setattr": true, "slice": true, "sorted": true, "staticmethod": true, "str": true, "sum": true,
	"super": true, "tuple": true, "type": true, "vars": true, "zip": true,
}

// pyTokens cuts a Python script into colored pieces, line by line: put together, the
// pieces of a line give it back, without its "\n". A string of three quotes goes on over
// the lines it spans; a built-in is one only where it is not an attribute (ssl.open is
// not open).
func pyTokens(src string) [][]pyToken {
	var out [][]pyToken
	quote := "" // the quotes of a string that a line before opened and did not close
	for _, line := range strings.Split(src, "\n") {
		var toks []pyToken
		emit := func(k pyKind, t string) {
			if t == "" {
				return
			}
			if n := len(toks); n > 0 && toks[n-1].kind == k {
				toks[n-1].text += t
				return
			}
			toks = append(toks, pyToken{k, t})
		}
		i, prev := 0, "" // prev: the last word, for the name after def or class
		if quote != "" { // the rest of a string of three quotes
			if j := strings.Index(line, quote); j >= 0 {
				emit(pyString, line[:j+3])
				i, quote = j+3, ""
			} else {
				emit(pyString, line)
				i = len(line)
			}
		}
		for i < len(line) {
			c := line[i]
			switch {
			case c == '#':
				emit(pyComment, line[i:])
				i = len(line)
			case pyStringStart(line[i:]) > 0:
				j := pyStringStart(line[i:])
				q := line[i+j-1 : i+j]
				if strings.HasPrefix(line[i+j-1:], q+q+q) { // three quotes, maybe over several lines
					q += q + q
					if k := strings.Index(line[i+j+2:], q); k >= 0 {
						emit(pyString, line[i:i+j+2+k+3])
						i += j + 2 + k + 3
					} else {
						emit(pyString, line[i:])
						i, quote = len(line), q
					}
					prev = ""
					continue
				}
				k := i + j
				for k < len(line) && line[k] != q[0] {
					if line[k] == '\\' {
						k++
					}
					k++
				}
				k = min(k+1, len(line)) // with its closing quote
				emit(pyString, line[i:k])
				i = k
			case isPyNameStart(c):
				j := i
				for j < len(line) && (isPyNameStart(line[j]) || line[j] >= '0' && line[j] <= '9') {
					j++
				}
				w := line[i:j]
				attribute := strings.HasSuffix(strings.TrimRight(line[:i], " "), ".")
				switch {
				case pyKeywords[w]:
					emit(pyKeyword, w)
				case prev == "def" || prev == "class":
					emit(pyFunction, w)
				case pyBuiltins[w] && !attribute:
					emit(pyBuiltin, w)
				case strings.HasPrefix(strings.TrimLeft(line[j:], " "), "("):
					emit(pyFunction, w)
				default:
					emit(pyName, w)
				}
				prev, i = w, j
				continue
			case c >= '0' && c <= '9' || c == '.' && i+1 < len(line) && line[i+1] >= '0' && line[i+1] <= '9':
				j := i
				for j < len(line) && (isPyNameStart(line[j]) || line[j] >= '0' && line[j] <= '9' || line[j] == '.') {
					j++
				}
				emit(pyNumber, line[i:j])
				i = j
			default:
				emit(pySymbol, line[i:i+1])
				i++
			}
			if c != ' ' {
				prev = ""
			}
		}
		out = append(out, toks)
	}
	return out
}

// pyStringStart is the length of the start of a string at the start of s, up to and
// with its first quote, such as `"` or `b"`, or 0 when no string starts there.
func pyStringStart(s string) int {
	for j := 0; j < len(s) && j <= 2; j++ {
		switch c := s[j]; {
		case c == '"' || c == '\'':
			return j + 1
		case !strings.ContainsRune("rRbBuUfF", rune(c)):
			return 0
		}
	}
	return 0
}

func isPyNameStart(c byte) bool { return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }
