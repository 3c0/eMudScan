//go:build !windows

package main

// Command-line runner used to test the checks on other systems (from the root of
// the repository):
//   go run ./cmd/emudscan [--lang fr] [folders to scan...]
// The commands that make files from the source (--make-syso…) work on any system:
// see toolCommand in mkrsrc.go.

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
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
	case LogNote:
		r.write("\n" + statusMark(st) + " " + text + "\n")
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
	if toolCommand(args) {
		return
	}
	for i := 0; i < len(args); i++ {
		if args[i] == "--lang" && i+1 < len(args) {
			i++
			if l, ok := langOption(args[i]); ok {
				Lang = l
			}
			continue
		}
		roots = append(roots, args[i])
	}
	if err := loadData(); err != nil {
		fmt.Println(Tf("err.data", err))
		os.Exit(1)
	}
	r := &cliReporter{}
	c := newChecker(r, nil)
	runAllChecks(c, nil, roots)
	if path := os.Getenv("REPORT"); path != "" { // text, JSON or CSV, by its extension
		_, format, _ := saveTarget(path, formatText, noFormat)
		data, err := reportFile(c, r.log.String(), format, time.Now())
		if err == nil {
			err = os.WriteFile(path, data, 0o644)
		}
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
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
