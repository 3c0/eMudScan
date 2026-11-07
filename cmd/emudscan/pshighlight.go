package main

// The colors of the PowerShell commands, in the details and in the Fix window: those
// that PowerShell 7 gives a command as it is typed (PSReadLine), on the palette of
// Windows Terminal. psTokens cuts a command as the PowerShell that eMudScan writes
// needs it, not as a full PowerShell parser: what it does not recognize keeps the
// color of the text.

import (
	"strings"
	"unicode"
)

type psKind int

const (
	psPlain     psKind = iota
	psCommand          // Copy-Item, scoop, python.exe: a word where a command starts
	psParameter        // -LiteralPath, --upgrade
	psVariable         // $ok, $env:LOCALAPPDATA, also inside "…"
	psString           // '…', "…"
	psKeyword          // if, foreach, in, try, catch…
	psOperator         // -and, -eq, =, +, |, ;, &
	psType             // [IO.File]
	psNumber           // 0, 4096
	psComment          // # to the end of the line
)

// psColor is the color of a kind, as rgb() takes it, or false for the color of the text.
func psColor(k psKind) (r, g, b byte, ok bool) {
	switch k {
	case psCommand:
		return 0xF9, 0xF1, 0xA5, true // Yellow
	case psParameter, psOperator:
		return 0x76, 0x76, 0x76, true // DarkGray
	case psVariable, psKeyword:
		return 0x16, 0xC6, 0x0C, true // Green
	case psString:
		return 0x3A, 0x96, 0xDD, true // DarkCyan
	case psType:
		return 0xCC, 0xCC, 0xCC, true // Gray
	case psNumber:
		return 0xF2, 0xF2, 0xF2, true // White
	case psComment:
		return 0x6A, 0x99, 0x55, true // as the comments of the Fix window
	}
	return 0, 0, 0, false
}

type psToken struct {
	kind psKind
	text string
}

var psKeywords = map[string]bool{"if": true, "else": true, "elseif": true, "foreach": true, "for": true, "while": true, "do": true,
	"switch": true, "try": true, "catch": true, "finally": true, "return": true, "break": true, "continue": true, "throw": true, "in": true}

var psOperators = map[string]bool{"and": true, "or": true, "xor": true, "not": true, "band": true, "bor": true, "bxor": true, "bnot": true,
	"eq": true, "ne": true, "gt": true, "ge": true, "lt": true, "le": true, "like": true, "notlike": true, "match": true, "notmatch": true,
	"replace": true, "split": true, "join": true, "is": true, "isnot": true, "as": true, "contains": true, "notcontains": true,
	"in": true, "notin": true, "f": true}

// psTokens cuts a command into colored pieces, which put together give it back.
func psTokens(cmd string) []psToken {
	s := []rune(cmd)
	var out []psToken
	emit := func(k psKind, t []rune) {
		if n := len(out); n > 0 && out[n-1].kind == k {
			out[n-1].text += string(t)
			return
		}
		out = append(out, psToken{k, string(t)})
	}
	isWord := func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' }
	// varEnd is the end of the variable at i ($name, $env:name, ${…}, $_, $?, $$), or i.
	varEnd := func(i int) int {
		j := i + 1
		if j < len(s) && s[j] == '{' {
			for j < len(s) && s[j] != '}' {
				j++
			}
			return min(j+1, len(s))
		}
		if j < len(s) && (s[j] == '?' || s[j] == '$') {
			return j + 1
		}
		start := j
		for j < len(s) && isWord(s[j]) {
			j++
		}
		if j < len(s) && s[j] == ':' && j+1 < len(s) && isWord(s[j+1]) { // $env:NAME
			for j++; j < len(s) && isWord(s[j]); j++ {
			}
		}
		if j == start {
			return i
		}
		return j
	}
	cmdPos := true // where a command starts: the start, and after ; | = ( { } &
	prev := func(i int) rune {
		if i > 0 {
			return s[i-1]
		}
		return ' '
	}
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == ' ' || c == '\t':
			j := i
			for j < len(s) && (s[j] == ' ' || s[j] == '\t') {
				j++
			}
			emit(psPlain, s[i:j])
			i = j
		case c == '#' && (i == 0 || prev(i) == ' ' || prev(i) == ';'):
			emit(psComment, s[i:])
			i = len(s)
		case c == '\'':
			j := i + 1
			for j < len(s) {
				if s[j] == '\'' {
					if j+1 < len(s) && s[j+1] == '\'' { // '' inside: a quote
						j += 2
						continue
					}
					break
				}
				j++
			}
			j = min(j+1, len(s))
			emit(psString, s[i:j])
			i, cmdPos = j, false
		case c == '"':
			j, k := i+1, i
			for j < len(s) && s[j] != '"' {
				switch {
				case s[j] == '`':
					j += 2
				case s[j] == '$' && varEnd(j) > j:
					emit(psString, s[k:j])
					e := varEnd(j)
					emit(psVariable, s[j:e])
					j, k = e, e
				default:
					j++
				}
			}
			j = min(j+1, len(s))
			emit(psString, s[k:j])
			i, cmdPos = j, false
		case c == '$' && varEnd(i) > i:
			j := varEnd(i)
			emit(psVariable, s[i:j])
			i, cmdPos = j, false
		case c == '[' && !isWord(prev(i)) && prev(i) != ')' && prev(i) != ']' && prev(i) != '$':
			j := i + 1
			for j < len(s) && (isWord(s[j]) || s[j] == '.') {
				j++
			}
			if j < len(s) && s[j] == '[' && j+1 < len(s) && s[j+1] == ']' { // [string[]]
				j += 2
			}
			if j > i+1 && j < len(s) && s[j] == ']' && unicode.IsLetter(s[i+1]) {
				emit(psType, s[i:j+1])
				i, cmdPos = j+1, false
			} else {
				emit(psPlain, s[i:i+1])
				i++
			}
		case c == '-' && i+1 < len(s) && (unicode.IsLetter(s[i+1]) || s[i+1] == '-') && (i == 0 || strings.ContainsRune(" (,", prev(i))):
			j := i + 1
			for j < len(s) && (isWord(s[j]) || s[j] == '-') {
				j++
			}
			if psOperators[strings.ToLower(string(s[i+1:j]))] {
				emit(psOperator, s[i:j])
			} else {
				emit(psParameter, s[i:j])
			}
			i, cmdPos = j, false
		case unicode.IsDigit(c) && !isWord(prev(i)) && prev(i) != '.':
			j := i
			for j < len(s) && unicode.IsDigit(s[j]) {
				j++
			}
			if j < len(s) && (isWord(s[j]) || s[j] == '.') { // a word or a version, not a number
				for j < len(s) && !strings.ContainsRune(" ;(){}|,'\"", s[j]) {
					j++
				}
				emit(psPlain, s[i:j])
			} else {
				emit(psNumber, s[i:j])
			}
			i, cmdPos = j, false
		case isWord(c) || c == '.' && cmdPos && i+1 < len(s) && isWord(s[i+1]):
			// a word: up to a space or a separator; after . or :: a member
			j := i
			for j < len(s) && !strings.ContainsRune(" \t;(){}|,'\"=[]", s[j]) && !(s[j] == '.' && j > i && j+1 < len(s) && unicode.IsUpper(s[j+1]) && !cmdPos) {
				j++
			}
			w := string(s[i:j])
			member := prev(i) == '.' || prev(i) == ':'
			switch {
			case member:
				emit(psPlain, s[i:j])
			case psKeywords[strings.ToLower(w)] && (cmdPos || strings.ToLower(w) == "in"):
				emit(psKeyword, s[i:j])
				cmdPos = false
				i = j
				continue
			case cmdPos:
				emit(psCommand, s[i:j])
			default:
				emit(psPlain, s[i:j])
			}
			i, cmdPos = j, false
		case c == '&' || c == '|' || c == ';' || c == '=' || c == '+' || c == ',' || c == '!' || c == '>' || c == '<' || c == '*':
			emit(psOperator, s[i:i+1])
			// a command can start after & | ; and =; not after a comparison such as -eq
			cmdPos = c == '&' || c == '|' || c == ';' || c == '='
			i++
		default:
			emit(psPlain, s[i:i+1])
			cmdPos = c == '(' || c == '{' || c == '}'
			i++
		}
	}
	return out
}
