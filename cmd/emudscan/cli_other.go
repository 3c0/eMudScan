//go:build !windows

package main

// Command-line runner used to test the checks on other systems (from the root of
// the repository):
//   go run ./cmd/emudscan [--fr] [folders to scan...]
//   go run ./cmd/emudscan --make-syso cmd/emudscan/rsrc_windows_amd64.syso

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type cliReporter struct {
	mu      sync.Mutex
	log     strings.Builder
	pending bool
}

func (r *cliReporter) write(s string) {
	r.log.WriteString(s)
	fmt.Print(s)
}

func (r *cliReporter) Log(kind LogKind, st Status, text string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if kind != LogEnd && r.pending {
		r.write("\n")
		r.pending = false
	}
	switch kind {
	case LogSection:
		r.write("\n== " + text + " ==\n")
	case LogStart:
		r.write(text)
		r.pending = true
	case LogEnd:
		if !r.pending {
			r.write("   ")
		}
		r.write(" " + statusMark(st) + " " + text + "\n")
		r.pending = false
	default:
		if st != StPending {
			r.write("   " + statusMark(st) + " ")
		}
		r.write(text + "\n")
	}
}

func (r *cliReporter) Upsert(Item)     {}
func (r *cliReporter) Activity(string) {}

func main() {
	var roots []string
	args := os.Args[1:]
	if len(args) == 2 && args[0] == "--previews" {
		if err := writePreviews(args[1]); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		return
	}
	if len(args) == 2 && (args[0] == "--make-syso" || args[0] == "--make-ico") {
		loadData()
		var err error
		if args[0] == "--make-syso" {
			err = writeSyso(args[1])
		} else {
			err = writeICO(args[1])
		}
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		return
	}
	for _, a := range args {
		switch a {
		case "--fr":
			Lang = 1
		default:
			roots = append(roots, a)
		}
	}
	if err := loadData(); err != nil {
		fmt.Println(Tf("err.data", err))
		os.Exit(1)
	}
	r := &cliReporter{}
	c := newChecker(r, nil)
	runAllChecks(c, nil, roots)
	if os.Getenv("REPORT") != "" {
		os.WriteFile(os.Getenv("REPORT"), []byte(buildReport(c, r.log.String())), 0o644)
	}
}

// writePreviews renders the spinner, status icons and button glyphs as PNG files (design review).
func writePreviews(dir string) error {
	os.MkdirAll(dir, 0o755)
	save := func(name string, px []byte, size int) error {
		img := &image.NRGBA{Pix: px, Stride: size * 4, Rect: image.Rect(0, 0, size, size)}
		f, err := os.Create(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		defer f.Close()
		return png.Encode(f, img)
	}
	for f := 0; f < spinFrames; f++ {
		for _, size := range []int{20, 30, 120} {
			if err := save(fmt.Sprintf("shield-%d-%02d.png", size, f), spinnerShield(f, spinFrames, size), size); err != nil {
				return err
			}
		}
	}
	for _, size := range []int{16, 24, 96} {
		for st := StPending; st <= StCaution; st++ {
			save(fmt.Sprintf("status%d-%d.png", st, size), renderStatus(st, size), size)
		}
		for gl := glyphPlay; gl <= glyphSearch; gl++ {
			save(fmt.Sprintf("glyph%d-%d.png", gl, size), render(size, glyphLayers(gl, rgba{0, 0, 0, 1})), size)
		}
	}
	return nil
}
