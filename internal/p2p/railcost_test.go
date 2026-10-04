package p2p

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"nib/internal/sign"
	"nib/internal/testpdf"
)

// P04.S02's T01 — what the ceremony listing actually costs per ceremony.
//
// # Why this exists at all
//
// The rail wants a per-party progress discriminator, and the only authoritative source is the
// document's own signatures (P04.S02's deepdive enumerated every file a ceremony directory holds;
// none records it). So the slice's whole design rests on a cost, and the cost has never been run:
// `grep -rn "func Benchmark" --include=*_test.go .` returns **zero** across this repo, and the
// 10 / 69 / 195 ms figure quoted in three files is P08.S01's, on text-only fixtures, for a
// different function — `/pending 360` says so in as many words.
//
// # The question, precisely
//
// `handleCeremonies` already pays one `ReadMirror` per stored ceremony (its header says it opens no
// document; `closeOutEnded` opens every one and discards the bytes — that is `/pending 360`).
// `ReadMirror` runs `sign.Verify` once, plus `ContentDigest` while the document is unsigned.
// `NextContributor` ran `sign.Verify` TWICE more until /pending 711 R5-7 and runs it ONCE now.
// So the marginal cost of progress is that pass, and whether that matters depends on the split between `sign.Verify` and `ContentDigest`
// inside a figure nobody has decomposed.
//
// # What this is NOT
//
// Not a benchmark and not a threshold. It RECORDS, with `-v`, and asserts only that its fixture is
// what it claims (the "signed" readings carry a signature). The shape property the arithmetic above
// rests on — `NextContributor` costs ONE bare verify and not two — is held structurally by
// `TestEveryVerifyingDoorReachesOneVerify`; a ratio of two clocks held it here until it went red on a
// busy machine (/pending 785). Wall-clock numbers are machine facts.
func TestWhatTheCeremonyListingCostsPerCeremony(t *testing.T) {
	if testing.Short() {
		t.Skip("timing under -short is a measurement of the scheduler")
	}
	a, b, c := l3Identity(t, "A"), l3Identity(t, "B"), l3Identity(t, "C")
	roster := l3Roster(a, b, c)

	// `median of 5`, because a single reading on a shared machine measures whatever else was
	// running. The spread is printed too: a median with an unstated spread is a number that cannot
	// be argued with.
	timeIt := func(n int, f func()) (time.Duration, time.Duration) {
		runs := make([]time.Duration, n)
		for i := range runs {
			start := time.Now()
			f()
			runs[i] = time.Since(start)
		}
		for i := range runs { // insertion sort, n=5
			for j := i; j > 0 && runs[j] < runs[j-1]; j-- {
				runs[j], runs[j-1] = runs[j-1], runs[j]
			}
		}
		return runs[len(runs)/2], runs[len(runs)-1] - runs[0]
	}

	for _, pages := range []int{1, 50, 200} {
		body := make([]string, pages)
		for i := range body {
			body[i] = "page " + strconv.Itoa(i+1) + " of the lease of 14 Elm Row"
		}
		base, err := testpdf.Text(body...)
		if err != nil {
			t.Fatalf("%d pages: build the fixture: %v", pages, err)
		}
		prepared, err := PrepareDocument(base)
		if err != nil {
			t.Fatalf("%d pages: PrepareDocument: %v", pages, err)
		}

		// UNSIGNED — the state `ReadMirror` pays `ContentDigest` in.
		unsignedVerify, uvSpread := timeIt(5, func() { _ = sign.Verify(prepared) })

		// SIGNED once, which is every ceremony past hop 1.
		signed := l3Chain(t, prepared, []l3Party{a}, []l3Party{a}, "")
		signedVerify, svSpread := timeIt(5, func() { _ = sign.Verify(signed) })
		nextCost, ncSpread := timeIt(5, func() { _, _ = NextContributor(signed, roster) })

		t.Logf("%4d pages, %7d bytes: verify(unsigned) %8v ±%v | verify(signed) %8v ±%v | NextContributor %8v ±%v",
			pages, len(prepared), unsignedVerify, uvSpread, signedVerify, svSpread, nextCost, ncSpread)

		// **The shape property is not asserted here, because it is not a time.** `NextContributor`
		// ran `sign.Verify` twice — once for the Invalid test and once inside `ReadAttestations` —
		// and since /pending 711 R5-7 runs it once. This file held that with a ratio (NextContributor
		// under 1.6× a bare verify, at 200 pages): healthy ~1.0, spread ±60-130%, two medians taken in
		// back-to-back windows, and it went red at 1.7× with jsdom alongside against good code
		// (/pending 785). How many verifies a door reaches is read off its code instead —
		// `TestEveryVerifyingDoorReachesOneVerify` — and the readings above stay a record.
	}

	// The floor for the whole file: the fixture really does carry a signature, so "signed" and
	// "unsigned" are two different states rather than the same document timed twice.
	prepared, err := PrepareDocument(mustText(t, "one page"))
	if err != nil {
		t.Fatal(err)
	}
	st := sign.Verify(prepared)
	if len(st.Signers) != 0 {
		t.Fatalf("setup: the prepared document already carries %d signature(s)", len(st.Signers))
	}
	one := l3Chain(t, prepared, []l3Party{a}, []l3Party{a}, "")
	st2 := sign.Verify(one)
	if len(st2.Signers) != 1 {
		t.Fatalf("setup: after one contribution the document carries %d signature(s), want 1 — the "+
			"'signed' readings above were taken on an unsigned document", len(st2.Signers))
	}
	_ = fmt.Sprint(b, c) // the three-party roster is what NextContributor walks
}

func mustText(t *testing.T, pages ...string) []byte {
	t.Helper()
	b, err := testpdf.Text(pages...)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
