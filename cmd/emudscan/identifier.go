package main

// Owners of folders named after an application identifier.
//
// Some frameworks (Tauri, for example) keep an application's data in a folder named
// after its identifier, a reverse domain name, rather than after the application:
// %LOCALAPPDATA%\io.esphome.builder belongs to ESPHome Device Builder. Such a folder
// is attributed to an installed program only on evidence, and only when exactly one
// program matches; otherwise it keeps its own name, as before:
//  1. the identifier is written in the program's main executable (Tauri compiles it
//     in): this is checked first. The main executable of an Electron application is
//     Electron itself, which cannot hold it (an Electron application's identifier, if
//     it has one, is in its app.asar): it is not read;
//  2. every word of the identifier, apart from a top-level domain at either end, is a
//     word of the program's name ("io.esphome.builder": ESPHome Device Builder).
// When several programs contain the identifier, none is chosen, and the name is not
// tried either.

import (
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"unicode/utf16"
)

// identifierRe: three labels or more, each starting with a letter (io.esphome.builder,
// com.example.app); versions (1.2.3, app-1.2.0) do not match.
var identifierRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9-]*(\.[A-Za-z][A-Za-z0-9-]*){2,}$`)

func isIdentifierName(s string) bool { return identifierRe.MatchString(s) }

// topLevelDomains: labels ignored at either end of an identifier for the name match.
var topLevelDomains = map[string]bool{
	"com": true, "org": true, "net": true, "io": true, "dev": true, "app": true, "co": true, "ai": true,
	"me": true, "info": true, "biz": true, "edu": true, "gov": true, "eu": true, "uk": true, "us": true,
	"de": true, "fr": true, "nl": true, "be": true, "ch": true, "it": true, "es": true, "jp": true, "cn": true,
}

// identifierWords: io.esphome.builder -> esphome, builder.
func identifierWords(id string) []string {
	labels := strings.Split(lower(id), ".")
	if len(labels) > 1 && topLevelDomains[labels[0]] {
		labels = labels[1:]
	}
	if len(labels) > 1 && topLevelDomains[labels[len(labels)-1]] {
		labels = labels[:len(labels)-1]
	}
	var words []string
	for _, l := range labels {
		for _, w := range strings.FieldsFunc(l, func(r rune) bool { return r == '-' || r == '_' }) {
			words = append(words, w)
		}
	}
	return words
}

// nameWords: "ESPHome Device Builder" -> esphome, device, builder.
func nameWords(name string) map[string]bool {
	out := map[string]bool{}
	for _, w := range strings.FieldsFunc(lower(name), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
	}) {
		out[w] = true
	}
	return out
}

// ownerByWords: the only installed program whose name holds every word of the identifier.
func ownerByWords(id string, owners []ownerDir) (ownerDir, bool) {
	words := identifierWords(id)
	letters := 0
	for _, w := range words {
		letters += len(w)
	}
	if len(words) == 0 || letters < 4 {
		return ownerDir{}, false
	}
	var match ownerDir
	names := map[string]bool{}
	for _, o := range owners {
		nw := nameWords(o.name)
		all := true
		for _, w := range words {
			if !nw[w] {
				all = false
				break
			}
		}
		if all {
			if !names[lower(o.name)] || match.exe == "" && o.exe != "" {
				match = o
			}
			names[lower(o.name)] = true
		}
	}
	return match, len(names) == 1
}

// identifierOwner is the installed program an identifier-named folder belongs to.
// The folders are all resolved at once, the first time one is needed (each program
// is then read once), and again after "Scan again" has reloaded the programs.
func (tp *appsContext) identifierOwner(id string) (ownerDir, bool) {
	tp.idMu.Lock()
	defer tp.idMu.Unlock()
	if tp.idOwners == nil {
		tp.idOwners = tp.resolveIdentifiers(identifierFolders())
	}
	o, ok := tp.idOwners[lower(id)]
	return o, ok
}

// identifierFolders: the identifier-named folders of the user's application data.
func identifierFolders() []string {
	var ids []string
	for _, root := range []string{getenv("LOCALAPPDATA"), getenv("APPDATA")} {
		if root == "" {
			continue
		}
		entries, _ := os.ReadDir(root)
		for _, e := range entries {
			if e.IsDir() && isIdentifierName(e.Name()) {
				ids = append(ids, e.Name())
			}
		}
	}
	return ids
}

// maxIdentifierExe: larger programs are not read for identifiers (Tauri programs are
// small).
const maxIdentifierExe = 256 << 20

// resolveIdentifiers finds the owner of each identifier (lower case keys).
func (tp *appsContext) resolveIdentifiers(ids []string) map[string]ownerDir {
	out := map[string]ownerDir{}
	if len(ids) == 0 {
		return out
	}
	// 1. the identifier in a program's executable, as text or as UTF-16 text
	var pats [][]byte
	var patID []int
	for i, id := range ids {
		if len(id) >= 8 { // the search needs patterns of 8 bytes or more
			pats = append(pats, []byte(id), utf16Bytes(id))
			patID = append(patID, i, i)
		}
	}
	found := make([]map[string]ownerDir, len(ids)) // by program name
	if len(pats) > 0 {
		electron := map[string]bool{}
		if tp.d != nil {
			for dir := range tp.d.electronDirs {
				electron[pathKey(dir)] = true
			}
		}
		type program struct {
			o   ownerDir
			exe string
		}
		var programs []program
		seen := map[string]bool{}
		for _, o := range tp.owners {
			exe := o.exe
			if exe == "" {
				exe, _ = mainExeIn(o.dir, o.name)
			}
			if exe == "" || seen[pathKey(exe)] || !tp.inScope(exe) || electron[pathKey(filepath.Dir(exe))] {
				continue
			}
			seen[pathKey(exe)] = true
			fi, err := os.Stat(exe)
			if err != nil || fi.IsDir() || fi.Size() > maxIdentifierExe || isCloudPlaceholder(fi) {
				continue
			}
			programs = append(programs, program{o, exe})
		}
		// the programs are read several at once; their results are then taken in order
		w := newWM(pats)
		hits := make([]map[int32]bool, len(programs))
		var next atomic.Int64
		var wg sync.WaitGroup
		for range min(runtime.NumCPU(), 6, len(programs)) {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := int(next.Add(1) - 1); i < len(programs); i = int(next.Add(1) - 1) {
					hits[i] = fileHits(programs[i].exe, w)
				}
			}()
		}
		wg.Wait()
		for k, p := range programs {
			for pid := range hits[k] {
				i := patID[pid]
				if found[i] == nil {
					found[i] = map[string]ownerDir{}
				}
				found[i][lower(p.o.name)] = p.o
			}
		}
	}
	for i, id := range ids {
		switch len(found[i]) {
		case 1:
			for _, o := range found[i] {
				out[lower(id)] = o
			}
		case 0:
			// 2. the words of the identifier in a program's name
			if o, ok := ownerByWords(id, tp.owners); ok {
				out[lower(id)] = o
			}
		}
	}
	return out
}

func utf16Bytes(s string) []byte {
	var b []byte
	for _, u := range utf16.Encode([]rune(s)) {
		b = append(b, byte(u), byte(u>>8))
	}
	return b
}

// fileHits returns the patterns of w found in a file.
func fileHits(path string, w *wmMatcher) map[int32]bool {
	hits := map[int32]bool{}
	f, err := os.Open(path)
	if err != nil {
		return hits
	}
	defer f.Close()
	const chunk = 1 << 20
	buf := make([]byte, w.maxLen+chunk)
	carried := 0
	for {
		n, err := io.ReadFull(f, buf[carried:carried+chunk])
		data := buf[:carried+n]
		final := err != nil
		next := w.find(data, 0, 0, final, func(pid int32, end int) { hits[pid] = true })
		if final {
			return hits
		}
		carried = copy(buf, data[next:])
	}
}

// identifierOwnerOf: the owner of a path inside an identifier-named folder of root.
func (tp *appsContext) identifierOwnerOf(root, first string) (ownerInfo, bool) {
	if !isIdentifierName(first) {
		return ownerInfo{}, false
	}
	o, ok := tp.identifierOwner(first)
	if !ok {
		return ownerInfo{}, false
	}
	return ownerInfo{Name: o.name, URL: o.url, Loc: filepath.Join(root, first), Version: o.version, Exe: o.exe}, true
}
