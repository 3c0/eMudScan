package main

// Chromium's list of blocked certificates (CRLSet). Chrome and most browsers based
// on it download it in the background, into
// <User Data>\CertificateRevocation\<version>\crl-set. It blocks certificates by the
// SHA-256 hash of their public key (BlockedSPKIs), or by serial number under the hash
// of their issuer's public key. Reading it tells whether the browser has received
// the block of the compromised intermediates, without visiting a test page.
//
// Format (Chromium, net/cert/crl_set.cc): a little-endian uint16 header length, a
// JSON header, then for each issuer: its 32-byte SPKI hash, a little-endian uint32
// number of serials, and each serial as a length byte followed by the serial.

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type crlSet struct {
	Sequence     int
	blockedSPKIs map[string]bool            // SHA-256 of blocked public keys
	serials      map[string]map[string]bool // issuer SPKI hash → blocked serials
}

func parseCRLSet(data []byte) (*crlSet, error) {
	if len(data) < 2 {
		return nil, errors.New("too short")
	}
	hl := int(binary.LittleEndian.Uint16(data))
	if len(data) < 2+hl {
		return nil, errors.New("truncated header")
	}
	var h struct {
		ContentType              string
		Version                  int
		Sequence                 int
		BlockedSPKIs             []string
		BlockedInterceptionSPKIs []string
	}
	if err := json.Unmarshal(data[2:2+hl], &h); err != nil {
		return nil, err
	}
	if h.ContentType != "CRLSet" || h.Version != 0 {
		return nil, errors.New("not a CRLSet")
	}
	s := &crlSet{Sequence: h.Sequence, blockedSPKIs: map[string]bool{}, serials: map[string]map[string]bool{}}
	for _, b := range append(h.BlockedSPKIs, h.BlockedInterceptionSPKIs...) {
		if raw, err := base64.StdEncoding.DecodeString(b); err == nil {
			s.blockedSPKIs[string(raw)] = true
		}
	}
	rest := data[2+hl:]
	for len(rest) > 0 {
		if len(rest) < 32+4 {
			return nil, errors.New("truncated entry")
		}
		parent := string(rest[:32])
		n := binary.LittleEndian.Uint32(rest[32:])
		rest = rest[36:]
		if n > 32<<20 {
			return nil, errors.New("invalid entry")
		}
		set := map[string]bool{}
		for i := uint32(0); i < n; i++ {
			if len(rest) < 1 || len(rest) < 1+int(rest[0]) {
				return nil, errors.New("truncated serial")
			}
			set[string(bytes.TrimLeft(rest[1:1+int(rest[0])], "\x00"))] = true
			rest = rest[1+int(rest[0]):]
		}
		s.serials[parent] = set
	}
	return s, nil
}

// blocks counts the compromised TLS intermediates (with their Sectigo cross-signed
// copies) that the CRLSet blocks, by public key or by issuer and serial number.
func (s *crlSet) blocks() (blocked, total int) {
	for _, c := range badCerts {
		if c.Usage != "tls" {
			continue
		}
		total++
		spki := sha256.Sum256(c.Cert.RawSubjectPublicKeyInfo)
		if s.blockedSPKIs[string(spki[:])] {
			blocked++
			continue
		}
		for _, r := range rootCerts {
			if !bytes.Equal(r.Cert.RawSubject, c.Cert.RawIssuer) {
				continue
			}
			parent := sha256.Sum256(r.Cert.RawSubjectPublicKeyInfo)
			if s.serials[string(parent[:])][string(bytes.TrimLeft(c.Cert.SerialNumber.Bytes(), "\x00"))] {
				blocked++
				break
			}
		}
	}
	return blocked, total
}

// crlSetInfo is the CRLSet a browser has downloaded.
type crlSetInfo struct {
	path           string
	sequence       int
	downloaded     time.Time
	blocked, total int
}

// browserCRLSet finds and reads the newest CRLSet of a Chromium-based browser: in the
// usual "User Data" folder of its publisher, or next to a portable browser.
func browserCRLSet(exe string) (crlSetInfo, bool) {
	var best crlSetInfo
	bestVer := Version(nil)
	found := false
	for _, ud := range userDataDirs(exe) {
		vers, _ := filepath.Glob(filepath.Join(ud, "CertificateRevocation", "*", "crl-set"))
		for _, p := range vers {
			v := ParseVersion(filepath.Base(filepath.Dir(p)))
			if found && v.Cmp(bestVer) <= 0 {
				continue
			}
			fi, err := os.Stat(p)
			if err != nil || fi.Size() > 64<<20 || isCloudPlaceholder(fi) {
				continue
			}
			data, err := os.ReadFile(p)
			if err != nil {
				continue
			}
			s, err := parseCRLSet(data)
			if err != nil {
				continue
			}
			b, t := s.blocks()
			best, bestVer, found = crlSetInfo{p, s.Sequence, fi.ModTime(), b, t}, v, true
		}
	}
	return best, found
}

// userDataDirs lists the folders where a Chromium-based browser may keep its profile.
func userDataDirs(exe string) []string {
	lp, dir := lower(exe), filepath.Dir(exe)
	lad, ad := getenv("LOCALAPPDATA"), getenv("APPDATA")
	var out []string
	add := func(parts ...string) {
		if parts[0] != "" {
			out = append(out, filepath.Join(parts...))
		}
	}
	for _, v := range []struct{ key, sub string }{
		{`\google\chrome sxs\`, `Google\Chrome SxS`}, {`\google\chrome beta\`, `Google\Chrome Beta`},
		{`\google\chrome dev\`, `Google\Chrome Dev`}, {`\google\chrome\`, `Google\Chrome`},
		{`\brave-browser-beta\`, `BraveSoftware\Brave-Browser-Beta`}, {`\brave-browser-nightly\`, `BraveSoftware\Brave-Browser-Nightly`},
		{`\brave-browser\`, `BraveSoftware\Brave-Browser`}, {`\vivaldi\`, `Vivaldi`}, {`\thorium\`, `Thorium`},
		{`\chromium\`, `Chromium`},
	} {
		if strings.Contains(strings.ReplaceAll(lp, "/", `\`), v.key) {
			add(lad, v.sub, "User Data")
			break
		}
	}
	if strings.HasSuffix(lp, `opera.exe`) {
		add(ad, "Opera Software", "Opera Stable")
		add(ad, "Opera Software", "Opera GX Stable")
	}
	// portable browsers, Scoop (User Data kept in the app folder) and installs whose
	// program is in an "Application" folder next to "User Data"
	for d, i := dir, 0; i < 3; d, i = filepath.Dir(d), i+1 {
		add(d, "User Data")
	}
	return dedupe(out)
}

func dedupe(paths []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range paths {
		if !seen[pathKey(p)] {
			seen[pathKey(p)] = true
			out = append(out, p)
		}
	}
	return out
}

// crlSetName names a CRLSet in the explanations: "no. 9871, downloaded on 18 October 2026".
func (c crlSetInfo) describe() string {
	return Tf("crlset.describe", strconv.Itoa(c.sequence), longDate(c.downloaded))
}
