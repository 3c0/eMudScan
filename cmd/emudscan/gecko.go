package main

// Firefox-based programs (LibreWolf, Tor Browser, Waterfox, Thunderbird…): the
// list of blocked certificates (OneCRL) that they carry. Mozilla blocked the
// compromised intermediates in OneCRL on 17 October 2026 (online), and Firefox
// 158.0.1, ESR 153.5.1 and ESR 115.43.1 (20 October 2026) ship the updated list, so
// that it applies even where the online list is not downloaded. The list is read
// from the program's omni.ja (defaults/settings/security-state/onecrl.json).

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const oneCRLEntry = "defaults/settings/security-state/onecrl.json"

// geckoFixed: the first releases of each branch that carry the updated list.
var geckoFixed = []string{"158.0.1", "153.5.1", "115.43.1"}

// geckoVersionFixed reports whether a Gecko (Firefox) version carries the list.
func geckoVersionFixed(v Version) bool {
	if len(v) == 0 {
		return false
	}
	for _, f := range geckoFixed {
		fv := ParseVersion(f)
		if v[0] == fv[0] && v.Cmp(fv) >= 0 {
			return true
		}
	}
	return v[0] > ParseVersion(geckoFixed[0])[0]
}

type oneCRLData struct {
	Data []struct {
		IssuerName   string `json:"issuerName"`
		SerialNumber string `json:"serialNumber"`
		Subject      string `json:"subject"`
		PubKeyHash   string `json:"pubKeyHash"`
	} `json:"data"`
}

// oneCRLBlocks counts the compromised TLS intermediates issued by eMudhra's root (6)
// that a OneCRL list blocks, by issuer and serial number or by subject and key.
// Their Sectigo cross-signed copies do not matter here: Firefox does not trust
// Sectigo's AAA Certificate Services root for websites (NSS trust "must verify"),
// and Mozilla blocked exactly these 6 certificates (bug 2083382).
func oneCRLBlocks(raw []byte) (blocked, total int, err error) {
	var d oneCRLData
	if err := json.Unmarshal(raw, &d); err != nil {
		return 0, 0, err
	}
	b64 := base64.StdEncoding.EncodeToString
	byIssuer, bySubject := map[string]bool{}, map[string]bool{}
	for _, e := range d.Data {
		if e.IssuerName != "" && e.SerialNumber != "" {
			byIssuer[e.IssuerName+"|"+e.SerialNumber] = true
		}
		if e.Subject != "" && e.PubKeyHash != "" {
			bySubject[e.Subject+"|"+e.PubKeyHash] = true
		}
	}
	for _, c := range badCerts {
		if c.Usage != "tls" || c.CrossSigned() {
			continue
		}
		total++
		serial := c.Cert.SerialNumber.Bytes()
		withZero := append([]byte{0}, serial...)
		spki := sha256.Sum256(c.Cert.RawSubjectPublicKeyInfo)
		if byIssuer[b64(c.Cert.RawIssuer)+"|"+b64(serial)] || byIssuer[b64(c.Cert.RawIssuer)+"|"+b64(withZero)] ||
			bySubject[b64(c.Cert.RawSubject)+"|"+b64(spki[:])] {
			blocked++
		}
	}
	return blocked, total, nil
}

// geckoOneCRL reads the OneCRL list carried by the Firefox-based program in dir.
// found is false when the program carries no readable list.
func geckoOneCRL(dir string) (blocked, total int, found bool) {
	for _, ja := range []string{filepath.Join(dir, "omni.ja"), filepath.Join(dir, "browser", "omni.ja")} {
		raw, err := readZipEntry(ja, oneCRLEntry)
		if err != nil {
			continue
		}
		if b, t, err := oneCRLBlocks(raw); err == nil {
			return b, t, true
		}
	}
	return 0, 0, false
}

// geckoVersion is the Gecko (Firefox) version of a Firefox-based program, from its
// platform.ini or application.ini.
func geckoVersion(dir string) string {
	for _, f := range []struct{ file, key string }{{"platform.ini", "Milestone"}, {"application.ini", "Version"}} {
		data, err := os.ReadFile(filepath.Join(dir, f.file))
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			if k, v, ok := strings.Cut(strings.TrimSpace(line), "="); ok && strings.EqualFold(k, f.key) {
				return strings.TrimSpace(v)
			}
		}
	}
	return ""
}

// geckoBuildDate is the build date of a Firefox-based program: the BuildID of its
// application.ini (or platform.ini), "20261019153012". ini is the file it was read from.
func geckoBuildDate(dir string) (t time.Time, ini string, ok bool) {
	for _, name := range []string{"application.ini", "platform.ini"} {
		p := filepath.Join(dir, name)
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			k, v, found := strings.Cut(strings.TrimSpace(line), "=")
			if !found || !strings.EqualFold(k, "BuildID") {
				continue
			}
			if t, err := time.Parse("20060102150405", strings.TrimSpace(v)); err == nil && t.Year() >= 2000 && plausibleDate(t) {
				return t, p, true
			}
		}
	}
	return time.Time{}, "", false
}

var librewolfRevRe = regexp.MustCompile(`^\s*(\d+(?:\.\d+)*)-(\d+)`)

// librewolfReadsBundled: LibreWolf 158.0.1-2 is the first release that reads the
// bundled list at startup instead of waiting for a synchronisation that may never
// happen (LibreWolf release notes). ok is false when the version is not known.
func librewolfReadsBundled(version string) (reads, ok bool) {
	m := librewolfRevRe.FindStringSubmatch(version)
	if m == nil {
		return false, false
	}
	rev, _ := strconv.Atoi(m[2])
	c := ParseVersion(m[1]).Cmp(ParseVersion("158.0.1"))
	return c > 0 || c == 0 && rev >= 2, true
}

// readZipEntry reads one file of a zip archive (omni.ja). Mozilla's archives are
// "optimized" (central directory first); when the standard reader refuses one, the
// local file headers are searched instead.
func readZipEntry(path, name string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if zr, err := zip.NewReader(f, fi.Size()); err == nil {
		for _, e := range zr.File {
			if e.Name == name {
				rc, err := e.Open()
				if err != nil {
					break
				}
				defer rc.Close()
				return io.ReadAll(io.LimitReader(rc, 64<<20))
			}
		}
	}
	if fi.Size() > 512<<20 {
		return nil, errors.New("archive too large")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	sig := []byte("PK\x03\x04")
	for off := 0; ; {
		i := bytes.Index(data[off:], sig)
		if i < 0 {
			return nil, errors.New("not found")
		}
		h := off + i
		off = h + 4
		if h+30 > len(data) {
			continue
		}
		method := binary.LittleEndian.Uint16(data[h+8:])
		csize := int(binary.LittleEndian.Uint32(data[h+18:]))
		nlen := int(binary.LittleEndian.Uint16(data[h+26:]))
		xlen := int(binary.LittleEndian.Uint16(data[h+28:]))
		if h+30+nlen > len(data) || string(data[h+30:h+30+nlen]) != name {
			continue
		}
		start := h + 30 + nlen + xlen
		end := len(data)
		if csize > 0 && start+csize <= len(data) {
			end = start + csize
		}
		switch method {
		case 0:
			return data[start:end], nil
		case 8:
			return io.ReadAll(io.LimitReader(flate.NewReader(bytes.NewReader(data[start:end])), 64<<20))
		}
		return nil, errors.New("unsupported compression")
	}
}
