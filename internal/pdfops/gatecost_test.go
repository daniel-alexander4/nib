package pdfops

import (
	"strconv"
	"testing"
	"time"
)

// TestTheStructureCarryGateCost is the stopwatch behind `/pending 530`'s numbers, kept so they can be
// re-taken rather than quoted — the same shape `TestDigestScale` keeps for `/pending 488`.
//
// # What it measures, and why the whole operation and not the gate alone
//
// `carryIsComplete` reads the output and then runs `checkStructConsistency`, `parentTreeOwners` and
// `formDrawCounts` as three independent full page sweeps, each calling `ctx.PageDict` per page — and
// `PageDict` walks the page tree from the root on every call with no cache
// (`model/xreftable.go:2141-2169`). That is the O(pages²) shape `pageselect.go` already records as a
// measured 6 s of a 27 s digest.
//
// The figure that matters to a user is what the whole carried operation costs, so that is what is
// timed: `Collect` over a full reverse permutation — the worst case, because nothing is pruned and
// every element is carried. The gate is shared with `NUp` through `completeOrHonest` (ADR-009), so
// both callers are timed.
//
// **It asserts almost nothing on purpose.** A timing assertion tight enough to catch a regression is
// tight enough to flake on a loaded machine, and this ran beside another repo's suite at load 6+.
// What it asserts is that the work actually happened — a carried tree, a full permutation — so the
// numbers it logs are of the thing they claim to be. Read them with `-v`; compare them across a
// change rather than against a threshold.
func TestTheStructureCarryGateCost(t *testing.T) {
	if testing.Short() {
		t.Skip("a timing log, not an assertion")
	}
	for _, clauses := range []int{100, 400, 800, 1200} {
		src, err := tagMarkdown(bigMarkdown(clauses), authoringFaces(), markdownFallbackFonts())
		if err != nil {
			t.Fatal(err)
		}
		n, err := PageCount(src)
		if err != nil {
			t.Fatal(err)
		}
		// SETUP: the fixture is genuinely tagged, or this times the untagged path and the gate never
		// runs at all — which would produce a comfortingly small number about nothing.
		if !ClaimsTagging(src) {
			t.Fatalf("setup: the %d-clause fixture is not tagged, so the carry gate never runs", clauses)
		}

		order := make([]string, n)
		for i := range order {
			order[i] = strconv.Itoa(n - i)
		}

		start := time.Now()
		out, err := Collect(src, order)
		if err != nil {
			t.Fatal(err)
		}
		collect := time.Since(start)

		start = time.Now()
		nup, err := NUp(src, 2, false)
		if err != nil {
			t.Fatal(err)
		}
		nupDur := time.Since(start)

		// The carry really happened on both, or the timings are of the honest path that drops the
		// tree — which is the cheap one, and the one this measurement is not about.
		carried := ClaimsTagging(out)
		nupCarried := ClaimsTagging(nup)
		t.Logf("%3d pages: Collect(full reverse) %7v carried=%-5v | NUp(2) %7v carried=%v",
			n, collect.Round(time.Millisecond), carried, nupDur.Round(time.Millisecond), nupCarried)
		if !carried {
			t.Errorf("%d pages: the reversed subset came back untagged, so this timed the honest "+
				"drop and not the carry the gate guards", n)
		}
	}
}
