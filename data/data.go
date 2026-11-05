// Package data holds the reference data of eMudScan, embedded into the program:
// the compromised certificates, the roots and the test chains published by
// https://emudhra.we-have-a-problem.com (source:
// https://github.com/galadran/we-have-a-problem.com), with its notice and license,
// the license of Go, whose standard library is built into the program, and a copy
// of eMudScan's own license (the LICENSE file at the root of the repository).
package data

import "embed"

// FS holds the files of this folder, by name ("roots.json", "chains/….pem").
//
//go:embed impacted-intermediates.json roots.json chains/*.pem
//go:embed NOTICE-we-have-a-problem.txt LICENSE-we-have-a-problem.txt LICENSE-go.txt LICENSE-emudscan.txt
var FS embed.FS
