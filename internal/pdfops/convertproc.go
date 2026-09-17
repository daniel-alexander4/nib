package pdfops

import (
	"path/filepath"
	"strconv"
)

// taskkillCommand is the command that ends a converter's process tree on Windows: `taskkill /T`, which
// walks child processes, resolved from `%SystemRoot%` rather than from PATH.
//
// **It sits in an UNTAGGED file so a machine that is not Windows can still test it** (`/pending 516`). The
// call that uses it is Windows-only and nothing on this repo's machines can run it — `convertproc_windows.go`
// declares that gap and it stands — but the argv is ordinary string handling, and the rule it carries is not
// about Windows at all: a program that ends other processes must not be whatever the user's PATH answers to
// `taskkill`. `fileURL` in `office.go` is the same shape for the same reason, out of the same pending item.
//
// The separator is the host's, because `filepath.Join` is: on Windows this yields `C:\Windows\System32\…`
// and the identical call on Linux yields forward slashes. What a test off Windows can pin is therefore the
// RULE — that the program comes from `%SystemRoot%` when the environment names one — and not the spelling.
func taskkillCommand(systemRoot string, pid int) (string, []string) {
	// No %SystemRoot% leaves PATH as the only thing there is, and a tree kill that never runs is worse
	// than one that goes through PATH: a hung `soffice.bin` holds the pipe the conversion waits on.
	name := "taskkill"
	if systemRoot != "" {
		name = filepath.Join(systemRoot, "System32", "taskkill.exe")
	}
	return name, []string{"/T", "/F", "/PID", strconv.Itoa(pid)}
}
