package main

// The wrap signs. A line of a script too long for its box is cut into rows: each row
// that goes on ends with ↩, half a space after its text and lower (wrapDrop), pointing
// to the next row, and the next one starts with ↪ and half a space, at the
// indent of the line, so that the rest of a command is not taken for another command.
// The details page draws them in its command blocks, and the Fix window in its
// console. They are only drawn: what is selected or copied is the line as it is,
// without them.
const (
	wrapEnd   = "↩"
	wrapStart = "↪"
)

// wrapDrop is how much lower than its row of height h ↩ is drawn: 5/12 of it, between a
// third and a half, so that its tail is on the row and its head points to the next one.
func wrapDrop(h int32) int32 { return h * 5 / 12 }

// leadSpaces is the number of spaces that start s.
func leadSpaces(s string) int {
	n := 0
	for n < len(s) && s[n] == ' ' {
		n++
	}
	return n
}

// wrapIndent is where ↪ starts the next rows of a line that starts with lead spaces,
// each spaceW wide, from the start of the line: at its indent, but never past half of
// the room, so that a line indented deeply keeps room for its text.
func wrapIndent(lead int, spaceW, room int32) int32 {
	return max(0, min(int32(lead)*spaceW, room/2))
}

// ofScript tells whether a line of the console is a line of the script: the software
// whose fix follows, a comment, a command, or another line of a script, as opposed to
// what the commands showed and the messages of eMudScan. Its rows show the wrap signs
// when it is cut.
func (k fixLineKind) ofScript() bool {
	switch k {
	case lineSoftware, lineComment, lineCommand, lineScript:
		return true
	}
	return false
}
