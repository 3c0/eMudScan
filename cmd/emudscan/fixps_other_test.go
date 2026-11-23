//go:build !windows

package main

// fileCreated is the creation date of a file, which only Windows keeps for the test.
func fileCreated(string) int64 { return 0 }
