package nib

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestTheKeyedLogLineHasOneReader — /pending 731 (5), ADR-009.
//
// A headless Nib logs `open Nib at <url>#k=<key>` and that line is the only way a harness gets in
// (ADR-054). `build/launchkey.sh` says it is the ONE reader of it, because four harnesses each
// carried a copy of the regex and a rewording of the log would have broken all four separately —
// and `build/windowfreeze.mjs` still carried a fifth. The routing is what is asserted: no harness
// file outside `launchkey.sh` may PARSE the line, which is spotted as the literal `open Nib at`
// sitting next to the `#k=` capture in code (comments stripped, so prose describing the line does
// not count).
func TestTheKeyedLogLineHasOneReader(t *testing.T) {
	const door = "build/launchkey.sh"
	parse := regexp.MustCompile(`open Nib at[^\n]*#k=`)
	lineComment := regexp.MustCompile(`(?m)^\s*(//|#).*$`)
	scanned := 0
	var readers []string
	for _, dir := range []string{"build", filepath.Join("test", "ui"), filepath.Join("test", "jsdom")} {
		ents, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range ents {
			name := e.Name()
			if e.IsDir() || !(strings.HasSuffix(name, ".sh") || strings.HasSuffix(name, ".mjs")) {
				continue
			}
			rel := filepath.ToSlash(filepath.Join(dir, name))
			b, err := os.ReadFile(rel)
			if err != nil {
				t.Fatal(err)
			}
			scanned++
			src := lineComment.ReplaceAllString(string(b), "")
			if parse.MatchString(src) && rel != door {
				readers = append(readers, rel)
			}
		}
	}
	// The floor: the door itself must still be found by the same pattern, or the scan is matching
	// nothing and a clean report means nothing.
	b, err := os.ReadFile(door)
	if err != nil {
		t.Fatalf("%s is gone: %v", door, err)
	}
	if !parse.MatchString(lineComment.ReplaceAllString(string(b), "")) {
		t.Fatalf("%s no longer matches the reader pattern, so this scan finds nothing anywhere", door)
	}
	if scanned < 20 {
		t.Fatalf("scanned %d harness files; the scan is not reading the tree", scanned)
	}
	for _, r := range readers {
		t.Errorf("%s parses the keyed log line itself. %s is the one reader (ADR-009): source it "+
			"and call launch_key / launch_base, or a rewording of the log breaks this file alone",
			r, door)
	}
}
