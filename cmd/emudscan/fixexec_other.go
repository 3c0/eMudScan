//go:build !windows

package main

import "errors"

// runFixes runs fixes only on Windows: elsewhere, the runner is tested with its own.
func runFixes(units []fixUnit, admin bool, out func(fixLine)) ([]unitResult, error) {
	return nil, errors.New(T("fix.windowsOnly"))
}
