package cli

import (
	"os"
	"strings"
	"testing"
)

// TestTheInPlaceRewriteIsDurableNotMerelyAtomic — /pending 316's CLI half, asserted structurally.
//
// # Why structurally
//
// `-w` renames over the user's ONLY copy: the original inode is gone the instant the rename lands.
// `internal/atomicfile` draws exactly that line — *"callers that hold the only copy of something
// get [WriteDurable]; callers that can re-derive their output do not need it"* — and `writeAtomic`
// used to be a hand-rolled temp-file-plus-rename with **no fsync at all**, so a crash inside the
// writeback window left a truncated PDF where the original was, after `nib: rewritten` had already
// been printed.
//
// fsync is not observable from inside the process. What IS checkable is which door this package
// reaches for, and that is what regressed: `atomicfile` offers a non-durable `Write` one letter
// away, and reaching for the weaker of two same-shaped functions is the exact mistake
// `atomicfile`'s own package doc records — `handleVaultImport` calling the rename-only twin to
// replace `vault.nib`.
//
// # What this cannot see, so the next tier can
//
// It cannot see a `Sync()` deleted inside `atomicfile` itself; that is the door's own contract and
// `internal/atomicfile`'s tests own it. It also says nothing about durability actually reaching
// the platter — only that this package asked for it.
func TestTheInPlaceRewriteIsDurableNotMerelyAtomic(t *testing.T) {
	ents, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	durable, plain, scanned := 0, 0, 0
	for _, e := range ents {
		n := e.Name()
		if !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		raw, rerr := os.ReadFile(n)
		if rerr != nil {
			t.Fatal(rerr)
		}
		scanned++
		src := string(raw)
		if strings.Contains(src, "atomicfile.WriteDurable(") {
			durable++
		}
		// Comments stripped, because this file's own explanation names both functions and a scan
		// satisfied by prose is how a freeze guard once read its own doc as proof of coverage.
		if strings.Contains(stripComments(src), "atomicfile.Write(") {
			plain++
			t.Errorf("%s calls atomicfile.Write. Every write this package makes replaces a file "+
				"the user named on the command line — for -w, the only copy — so it takes "+
				"WriteDurable. Write is atomic and NOT durable: a crash in the writeback window "+
				"leaves a truncated file where the original was, after \"rewritten\" was printed.", n)
		}
	}
	// STIMULUS. A scan that read the wrong directory, or a package that stopped writing files,
	// produces the same clean result as a correct one.
	if scanned < 3 || durable == 0 {
		t.Fatalf("setup: scanned %d source file(s) and found %d WriteDurable caller(s) — this "+
			"guard is not reading internal/cli, and its clean result above means nothing",
			scanned, durable)
	}
	t.Logf("internal/cli: %d file(s) scanned, %d durable writer(s), %d non-durable", scanned, durable, plain)
}

// stripComments removes // line comments so a scan cannot be satisfied by prose.
func stripComments(src string) string {
	var b strings.Builder
	for _, ln := range strings.Split(src, "\n") {
		if i := strings.Index(ln, "//"); i >= 0 {
			ln = ln[:i]
		}
		b.WriteString(ln)
		b.WriteByte('\n')
	}
	return b.String()
}
