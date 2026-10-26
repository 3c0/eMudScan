//go:build !windows

package main

// Stand-ins so the platform-independent logic can be built and tested on other systems.

import (
	"io/fs"
	"time"
)

type regRoot = uintptr

const (
	hkcu   regRoot = 1
	hklm   regRoot = 2
	view64 uint32  = 0x100
	view32 uint32  = 0x200
)

type fileVersion struct {
	Fixed   Version
	Product string
	File    string
	Name    string
	Desc    string
	Company string
}

type driveInfo struct {
	Root, Label string
	Fixed       bool
}

func listDrives() []driveInfo { return []driveInfo{{"/", "/", true}} }

func regString(regRoot, string, string, uint32) (string, bool) { return "", false }
func regSubkeys(regRoot, string, uint32) []string              { return nil }
func getFileVersion(string) (fileVersion, bool)                { return fileVersion{}, false }
func isCloudPlaceholder(fs.FileInfo) bool                      { return false }
func creationTime(fs.FileInfo) (time.Time, bool)               { return time.Time{}, false }
func shortDate(t time.Time) string                             { return t.Local().Format("2006-01-02") }
func fixedDrives() []string                                    { return []string{"/"} }
func windowsDir() string                                       { return "" }
func userUILanguage() string                                   { return "en" }
func windowsVersionString() string                             { return "not Windows" }

func runWindowsChecks(c *checker) (bool, int) {
	c.section(T("log.winStart"))
	c.line(StInfo, "Windows checks skipped (not running on Windows)")
	return true, 0
}
