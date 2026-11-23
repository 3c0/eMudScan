package main

// ctrlChar: c is the character that a Ctrl key makes with a letter (1 to 26), but Tab
// (Ctrl+I, 9), Enter (Ctrl+M, 13) and the line feed (Ctrl+J, 10), which keyboards also
// make with their own keys (viewDlgCode).
func ctrlChar(c uintptr) bool { return c >= 1 && c <= 26 && c != '\t' && c != '\n' && c != '\r' }
