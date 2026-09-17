package cli

import (
	"os"
	"regexp"
	"sort"
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
// # It bans the PROPERTY, not one name (/pending 550)
//
// **This guard used to test for the literal string `atomicfile.Write(` and nothing else, and that
// was a hole rather than a simplification.** `internal/atomicfile` has TWO doors whose own comments
// say they are deliberately not durable — `Write` and `WriteFrom` (*"ATOMIC, and deliberately not
// durable, the same choice `Write` makes and for the same reason"*) — and a substring check for
// `atomicfile.Write(` matches neither `atomicfile.WriteFrom(` nor any door added after it. Named
// search, so the absence is evidence and not an impression: `grep -n WriteFrom
// internal/cli/atomicdurable_test.go` on the tree before this change returned nothing. So this
// package could have streamed a write through a non-durable door with the guard silent, and a
// fifth door tomorrow would arrive the same way.
//
// So the population is DISCOVERED from `internal/atomicfile`'s own source and each door is
// classified here. A door this file has never heard of is refused until somebody classifies it —
// which is the direction that fails safe: a new non-durable door is the thing being hunted, and
// making its arrival loud costs one line in `durable` when it turns out to be durable after all.
//
// # What this cannot see, so the next tier can
//
// It cannot see a `Sync()` deleted inside `atomicfile` itself; that is the door's own contract and
// `internal/atomicfile`'s tests own it. It also says nothing about durability actually reaching
// the platter — only that this package asked for it.
func TestTheInPlaceRewriteIsDurableNotMerelyAtomic(t *testing.T) {
	// **Discovered, never listed.** `atomicdoor_test.go` states the rule this borrows —
	// *"Discover the population; never list it … a package added later is invisible exactly the
	// way `internal/udpmux` was"* — and the failure here is the same shape one level down: a door
	// added to `atomicfile` and not to a hand-written list is a door this guard does not police.
	doorSrc, err := os.ReadFile("../atomicfile/atomicfile.go")
	if err != nil {
		t.Fatal(err)
	}
	doors := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?m)^func ([A-Z]\w*)\(`).FindAllStringSubmatch(string(doorSrc), -1) {
		doors[m[1]] = true
	}
	// durable is the classification, and it is the only thing hand-written here. Each entry is a
	// door whose contract includes the fsync of the file AND of the parent directory, checked by
	// reading it: WriteDurable does both itself, ReplaceDurable delegates to it after resolving
	// the link and carrying the mode, CreateDurable does both under O_EXCL.
	durable := map[string]bool{"WriteDurable": true, "ReplaceDurable": true, "CreateDurable": true}
	// STIMULUS on the discovery itself. A parse that matched nothing — the file moved, the
	// `func` spelling changed, gofmt stopped putting the receiver-less form at column 0 — leaves
	// `doors` empty, and an empty population makes every call site below unclassifiable rather
	// than clean. Both halves are checked: the doors must be found, and the ones this file
	// claims are durable must actually exist under those names.
	if len(doors) < 4 {
		t.Fatalf("setup: found %d exported door(s) in ../atomicfile/atomicfile.go — this guard is "+
			"not reading the door package, and every verdict below is about a population of %d",
			len(doors), len(doors))
	}
	for name := range durable {
		if !doors[name] {
			t.Fatalf("setup: this guard classifies %q as durable and ../atomicfile/atomicfile.go "+
				"exports no such function. The door was renamed or removed; re-classify it here "+
				"in the same change, because a stale name silently narrows what is allowed.", name)
		}
	}

	ents, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	call := regexp.MustCompile(`atomicfile\.([A-Za-z_]\w*)\(`)
	durableCalls, scanned := 0, 0
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
		// Comments stripped, because this file's own explanation names the doors and a scan
		// satisfied by prose is how a freeze guard once read its own doc as proof of coverage.
		src := stripComments(string(raw))
		var bad []string
		for _, m := range call.FindAllStringSubmatch(src, -1) {
			switch {
			case durable[m[1]]:
				durableCalls++
			case !doors[m[1]]:
				bad = append(bad, m[1]+" (unknown to ../atomicfile/atomicfile.go)")
			default:
				bad = append(bad, m[1])
			}
		}
		if len(bad) > 0 {
			sort.Strings(bad)
			t.Errorf("%s reaches a NON-DURABLE atomicfile door: %s. Every write this package "+
				"makes lands on a path the user named on the command line, or one derived inside "+
				"a directory they named — for -w, the only copy — so it takes a durable door "+
				"(WriteDurable / ReplaceDurable / CreateDurable), normally through writeNamed. "+
				"Write and WriteFrom are atomic and NOT durable: a crash in the writeback window "+
				"leaves a truncated file where the original was, after \"rewritten\" was printed. "+
				"A door this guard does not know is refused until it is classified in `durable`.",
				n, strings.Join(bad, ", "))
		}
		// `/pending 504`: four output paths and the watch's proof sidecar wrote with `os.WriteFile`, which
		// truncates in place (`-o` naming the input destroyed the only copy) and follows a planted symlink.
		// Every file goes through `writeNamed` or `atomicfile.WriteDurable`; the door's own device exemption,
		// in cli.go, is the one call allowed.
		if c := strings.Count(src, "os.WriteFile("); c > 0 && (n != "cli.go" || c > 1) {
			t.Errorf("%s calls os.WriteFile %d time(s). It truncates the target before writing — -o naming "+
				"the input loses the only copy — and follows a symlink planted in a watched directory. "+
				"Write through writeNamed (a path the user named) or atomicfile.WriteDurable (a sidecar).", n, c)
		}
	}
	// STIMULUS. A scan that read the wrong directory, or a package that stopped writing files,
	// produces the same clean result as a correct one.
	if scanned < 3 || durableCalls == 0 {
		t.Fatalf("setup: scanned %d source file(s) and found %d durable atomicfile call(s) — this "+
			"guard is not reading internal/cli, and its clean result above means nothing",
			scanned, durableCalls)
	}
	t.Logf("internal/cli: %d source file(s) scanned, %d durable atomicfile call(s); %d door(s) "+
		"discovered, %d classified durable", scanned, durableCalls, len(doors), len(durable))
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
