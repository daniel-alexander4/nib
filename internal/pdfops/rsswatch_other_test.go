//go:build !linux

package pdfops

import "testing"

// A sibling that cannot enforce says so in a sentence a notice can use; a silent no-op would be the Go
// pack's `ReplaceOthers` returning 0 off Linux.
func TestTheCeilingSiblingsAreHonest(t *testing.T) {
	if CeilingEnforced || CeilingUnenforcedNote == "" {
		t.Fatal("off Linux the ceiling is unenforced and must say so")
	}
}
