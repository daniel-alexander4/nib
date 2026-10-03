package server

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestTheRegisteredSnapshotHasOneDoor — /pending 813, ADR-009. "Take the bytes and check the document is
// still open, in one hold, or 409" was written out at four routes beside `docBytes`. It is now
// `snapshotRegistered`; this census fails on a fifth hand-written copy anywhere in the package.
func TestTheRegisteredSnapshotHasOneDoor(t *testing.T) {
	shape := regexp.MustCompile(`s\.mu\.Lock\(\)\s*if !s\.isRegisteredLocked\(doc\) \{\s*(//[^\n]*\s*)*s\.mu\.Unlock\(\)\s*httpError\(w, http\.StatusConflict`)
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, rerr := os.ReadFile(f)
		if rerr != nil {
			t.Fatal(rerr)
		}
		for _, loc := range shape.FindAllIndex(b, -1) {
			found++
			line := 1 + strings.Count(string(b[:loc[0]]), "\n")
			before := string(b[:loc[0]])
			enclosing := before[strings.LastIndex(before, "\nfunc ")+1:]
			if !strings.HasPrefix(enclosing, "func (s *Server) snapshotRegistered(") {
				t.Errorf("%s:%d takes s.mu, checks registration and answers 409 by hand — call snapshotRegistered", f, line)
			}
		}
	}
	// Stimulus: the door itself must match, or the pattern has drifted and this census checks nothing.
	if found == 0 {
		t.Fatal("the shape matched nowhere, not even in snapshotRegistered — the census is reading nothing")
	}
}
