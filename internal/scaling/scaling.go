// Package scaling is the one door for a test that guards a cost's SHAPE by the clock: does this grow
// linearly in its input, or has it gone quadratic — asked on a machine whose load nobody controls
// (/pending 785, 799).
//
// # Why one door
//
// Ten tests asked this question, each with its own clock: best-of-N per size measured to completion
// one size after the other, a ×2 size step against a ×3 bound, a ×4 step against ×6. Every shape that
// measured the two sizes in separate windows went red under full-suite contention against perfectly
// good code — `proposeStructure` ×3.86 against ×3, the codespace read ×9.3 against a 6 s bound, the
// object-stream reader ×6.1 against ×6 — because a load spike that covers one size and not the other
// moves the ratio and nothing else. `TestMarkingARunCostsTheSameHoweverManyRunsCameBefore` had
// already measured the shape that holds (interleaved rounds, the MINIMUM per-round ratio: 8 of 8
// green with all eight cores saturated, a dominant quadratic 3 of 3 red); this package is that shape,
// written once.
//
// # What it cannot see
//
// A MILD quadratic — one whose quadratic term has not yet become the dominant cost — reads as linear
// here, as it did under every shape before this one. The guard catches a regression that has become
// the cost, which is the one that matters and not the only one that exists. Where a cost can be
// COUNTED (bytes allocated, operations, calls), count it instead; this door is for what only a clock
// can see.
package scaling

import (
	"runtime"
	"testing"
	"time"
)

// rounds is the most interleaved rounds a verdict takes.
const rounds = 5

// TimeOnce runs f once, after a collection so the previous measurement's garbage is not billed to
// this one, and returns how long it took.
func TimeOnce(f func()) time.Duration {
	runtime.GC()
	start := time.Now()
	f()
	return time.Since(start)
}

// GrowsLinearly fails t unless cost grows linearly between small and large. cost(n) returns the time
// of ONE measurement at size n (fixture building excluded — `TimeOnce` around the timed part).
//
// The two sizes are measured in alternation for `rounds` rounds and the verdict is the SMALLEST
// per-round ratio cost(large)/cost(small) — never the ratio of separately-taken minimums, which lets
// a spike over one size alone decide it. bound must lie strictly between the linear ratio k =
// large/small and the quadratic k²; the log midpoint k^1.5 separates them best (×4 sizes: 4, 16, 8).
func GrowsLinearly(t testing.TB, what string, small, large int, bound float64, cost func(n int) time.Duration) {
	t.Helper()
	k := sizeRatio(t, what, small, large, bound)
	best, n, s, l := leastRatio(t, what, bound, func() time.Duration { return cost(small) }, func() time.Duration { return cost(large) })
	t.Logf("%s: %d → %d (×%.0f) cost %v → %v, ×%.2f (the least of %d interleaved round(s); bound ×%.1f)",
		what, small, large, k, s, l, best, n, bound)
	if best > bound {
		t.Fatalf("%s: ×%.0f the size cost ×%.2f the time in EVERY one of %d interleaved rounds (best %v → %v) — "+
			"it has gone superlinear", what, k, best, n, s, l)
	}
}

// AllocsGrowLinearly fails t unless the heap allocations of a run grow linearly between small and large —
// COUNTED, so no load moves them. prep(n) builds the size-n fixture and returns the run to count.
//
// It is the first assertion wherever the quadratic it guards allocates, which is every page-tree walk: pdfcpu's
// `PageDict` allocates per kid it passes, so one per page on a flat tree is quadratic in allocations exactly as in
// time (`pdfread.Pages`, 3,000 → 12,000 pages: ×4.0 healthy, ×16.0 with a `PageDict` per page, measured). The clock
// (`GrowsLinearly`) stays beside it for a quadratic that does not allocate.
func AllocsGrowLinearly(t testing.TB, what string, small, large int, bound float64, prep func(n int) func()) {
	t.Helper()
	k := sizeRatio(t, what, small, large, bound)
	mallocs := func(n int) uint64 {
		run := prep(n)
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		run()
		runtime.ReadMemStats(&after)
		return after.Mallocs - before.Mallocs
	}
	s, l := mallocs(small), mallocs(large)
	if s == 0 {
		t.Fatalf("%s: %d allocated nothing, so the ratio compares nothing — count something else", what, small)
	}
	ratio := float64(l) / float64(s)
	t.Logf("%s: %d → %d (×%.0f) allocated %d → %d, ×%.2f (bound ×%.1f)", what, small, large, k, s, l, ratio, bound)
	if ratio > bound {
		t.Fatalf("%s: ×%.0f the size allocated ×%.2f as often (%d → %d) — it has gone superlinear", what, k, ratio, s, l)
	}
}

// WithinFactor fails t unless cost takes at most bound times base, base being a workload of the same code
// measured in the same rounds, so the ceiling moves with the machine and not with the code. Two uses: a cost
// held by a BUDGET, where an absolute ceiling measures the load (a refusal bounded at 6 s took 9.96 s at load
// ~8 and 1.15 s alone, /pending 799); and the same work against a sixteen-times larger structure, where
// "linear" means a ratio near one rather than near the size ratio (udpmux's table fill).
func WithinFactor(t testing.TB, what string, bound float64, base, cost func() time.Duration) {
	t.Helper()
	best, n, b, c := leastRatio(t, what, bound, base, cost)
	t.Logf("%s: %v against a base of %v, ×%.2f (the least of %d interleaved round(s); bound ×%.1f)",
		what, c, b, best, n, bound)
	if best > bound {
		t.Fatalf("%s: took ×%.2f its base in EVERY one of %d interleaved rounds (best %v against %v; bound ×%.1f)",
			what, best, n, c, b, bound)
	}
}

// leastRatio measures a and b in alternation — a, b, a, b, …, a — for up to `rounds` rounds and returns the
// smallest per-round ratio, how many rounds it took, and the two readings it came from. A round's ratio is b
// over the LESSER of the two a's that bracket it.
//
// Two spikes, two defences. A spike over b alone raises a round's ratio, and a regression is the one thing
// that breaches in every round, so the verdict is the least round. A spike over a alone LOWERS a round's
// ratio, and the minimum then forgives a regression: with one a per round, a re-decoding object-stream reader
// passed at load ~30 and a per-insert table sweep read ×7.2 against ×8 (both measured, /pending 799). So a is
// read on both sides of b and the lesser kept: a spike must cover both brackets, and the b between them, to
// hide anything.
//
// It stops at the first round within bound: the verdict is "some round was within bound", so the rounds after
// that one cannot change it, and a healthy run pays one round where a regression pays all of them.
func leastRatio(t testing.TB, what string, bound float64, a, b func() time.Duration) (best float64, n int, ra, rb time.Duration) {
	t.Helper()
	measureA := func() time.Duration {
		d := a()
		if d <= 0 {
			t.Fatalf("%s: the base took no measurable time, so the ratio compares nothing", what)
		}
		return d
	}
	before := measureA()
	for n < rounds {
		n++
		db := b()
		after := measureA()
		da := min(before, after)
		if ratio := float64(db) / float64(da); n == 1 || ratio < best {
			best, ra, rb = ratio, da, db
		}
		if best <= bound {
			break
		}
		before = after
	}
	return best, n, ra, rb
}

// sizeRatio is large/small, refusing a bound that cannot separate linear (×k) from quadratic (×k²).
func sizeRatio(t testing.TB, what string, small, large int, bound float64) float64 {
	t.Helper()
	k := float64(large) / float64(small)
	if !(bound > k && bound < k*k) {
		t.Fatalf("%s: a bound of ×%.1f for ×%.0f the size cannot separate linear (×%.0f) from quadratic (×%.0f)",
			what, bound, k, k, k*k)
	}
	return k
}
