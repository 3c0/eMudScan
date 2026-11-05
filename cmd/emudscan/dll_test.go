package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Windows looks for a DLL loaded by name in the program's folder first (see
// dll_windows.go): eMudScan 1.0 loaded winhttp.dll from its own folder. This test
// reads the source and refuses any DLL loaded by name, so that the mistake cannot
// come back. It runs on every system, Windows files included, since it only parses them.
func TestDLLsLoadedFromSystem32(t *testing.T) {
	// loaded by name on purpose: kernel32 is a known DLL, already in the process;
	// comctl32 must come from the manifest (version 6, WinSxS)
	allowed := map[string]bool{"kernel32.dll": true, "comctl32.dll": true}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	loads := 0
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		f, err := parser.ParseFile(fset, name, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok || pkg.Name != "syscall" && pkg.Name != "windows" {
				return true
			}
			switch sel.Sel.Name {
			case "NewLazyDLL", "LoadDLL", "MustLoadDLL", "LoadLibrary", "LoadLibraryEx", "NewLazySystemDLL":
			default:
				return true
			}
			loads++
			pos := fset.Position(call.Pos())
			if name == "dll_windows.go" && sel.Sel.Name == "NewLazyDLL" && len(call.Args) == 1 {
				if _, isLit := call.Args[0].(*ast.BasicLit); !isLit {
					return true // sysDLL itself: the full path in System32
				}
			}
			if len(call.Args) != 1 {
				t.Errorf("%s: %s.%s with %d arguments: load Windows' DLLs with sysDLL", pos, pkg.Name, sel.Sel.Name, len(call.Args))
				return true
			}
			lit, ok := call.Args[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				t.Errorf("%s: %s.%s with a computed name: load Windows' DLLs with sysDLL", pos, pkg.Name, sel.Sel.Name)
				return true
			}
			dll, _ := strconv.Unquote(lit.Value)
			if !allowed[strings.ToLower(dll)] {
				t.Errorf("%s: %q is loaded by name, which lets a copy next to eMudScan.exe be loaded instead: use sysDLL(%q)", pos, dll, dll)
			}
			return true
		})
	}
	if loads < 3 { // sysDLL, kernel32, comctl32
		t.Errorf("found %d DLL loads, expected at least 3: is the test still looking at the right calls?", loads)
	}
}
