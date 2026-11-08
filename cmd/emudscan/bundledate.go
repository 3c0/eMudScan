package main

// The date of the certificate data in a bundle that curl made from Mozilla's root
// store (its mk-ca-bundle script, used for curl's cacert.pem and by many programs):
// it writes at the top "## Certificate data from Mozilla as of: Fri Sep 25 03:12:01
// 2026 GMT", the date of Mozilla's file, or "… downloaded on: …" when Mozilla's
// server gave no date. eMudScan shows it with curl's words, under the bundle's file.
// A bundle that belongs to no software has no other date: the modification date of
// a file on its own only tells when it was copied.

import (
	"io"
	"os"
	"strings"
	"time"
)

// mozillaDateIn is the date that curl wrote in the start of a bundle; none if it has
// no such line.
func mozillaDateIn(head string) lastUpdate {
	for _, l := range strings.Split(head, "\n") {
		rest, ok := strings.CutPrefix(strings.TrimRight(l, "\r"), "## Certificate data from Mozilla ")
		if !ok {
			continue
		}
		src := dateMozilla
		if v, ok := strings.CutPrefix(rest, "as of: "); ok {
			rest = v
		} else if v, ok := strings.CutPrefix(rest, "downloaded on: "); ok {
			rest, src = v, dateMozillaDownload
		} else {
			return lastUpdate{}
		}
		t, err := time.Parse("Mon Jan _2 15:04:05 2006", strings.TrimSuffix(strings.TrimSpace(rest), " GMT"))
		if err != nil || !plausibleDate(t) {
			return lastUpdate{}
		}
		// the day that curl wrote (in GMT), whatever the time zone of this PC
		return lastUpdate{Time: time.Date(t.Year(), t.Month(), t.Day(), 12, 0, 0, 0, time.Local), Src: src}
	}
	return lastUpdate{}
}

// mozillaDateOf is the date that curl wrote in the bundle at path (its start: the
// line is in its first lines); none for a program, a folder or another file.
func mozillaDateOf(path string) lastUpdate {
	if path == "" || isPEName(path) {
		return lastUpdate{}
	}
	f, err := os.Open(path)
	if err != nil {
		return lastUpdate{}
	}
	defer f.Close()
	if fi, err := f.Stat(); err != nil || !fi.Mode().IsRegular() || isCloudPlaceholder(fi) {
		return lastUpdate{}
	}
	head := make([]byte, 8192)
	n, _ := io.ReadFull(f, head)
	u := mozillaDateIn(string(head[:n]))
	if u.known() {
		u.File = path
	}
	return u
}

// mozilla: the date is the one curl wrote in a bundle.
func (u lastUpdate) mozilla() bool { return u.Src == dateMozilla || u.Src == dateMozillaDownload }

// mozillaLabel names that date with curl's words.
func (u lastUpdate) mozillaLabel() string {
	if u.Src == dateMozillaDownload {
		return T("upd.downloadedOn")
	}
	return T("upd.asOf")
}
