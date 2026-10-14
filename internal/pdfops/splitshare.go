package pdfops

import (
	"errors"
	"sort"
)

// The parts of a multi-file split, collected without parsing the whole document once per part (`/pending 834`).
//
// # What it cost, measured
//
// `SplitBySpans` and `SplitByBookmarks` called `Collect` on the WHOLE source for every part, so n parts of an
// n-page document were n full reads: quadratic. Profiled at n=200 (one-page parts of a tagged 200-page document):
// 64% of the split is `pdfread.ReadOptimized`, and inside it the parse of the bytes is half, pdfcpu's validation a
// fifth, the reference door a sixth and the optimize pass a tenth — every one of them linear in the SOURCE, so no
// step can be skipped to make a part cheap. The write is ~11 ms a part and flat.
//
// # The rule: a part is collected from the smallest document that still holds its pages
//
// The selections are halved, and wherever a half names at most half of the pages of the document in hand, those
// pages are collected ONCE, in ascending order, into a smaller document that both of its halves are then cut
// from. A part is still one `Collect` — the same door, the same carry, the same gate, on its own page list — but
// of a document that holds little more than the part. Each level of the halving reads the source about twice, so
// the whole split is n log n where it was n².
//
// # Why that is the same part, and what makes it so where it would not be
//
// A page selection is an extract: no `/Info`, none of the source's files, a new `/ID` (`sameDocument`'s header).
// Everything it DOES carry is decided per kept page — the page's content and resources, the structure pruned to
// the pages kept (ADR-034), labels following their pages (ADR-046), the outline, fields and named destinations
// that still reach a kept page, a layer's state (ADR-043). So selecting S from a selection U ⊇ S of the source
// keeps what selecting S from the source keeps. Three things could break that, and each is closed:
//
//   - **The carry's gate reads the OUTPUT.** A form holding marked content that two pages of the smaller document
//     both draw fails the gate there (`carryIsComplete`), and the smaller document would be written with no tree —
//     while each one-page part cut from the SOURCE carries. So an intermediate document of a tagged source must
//     come out carried and complete, or the fast path is abandoned.
//   - **A refusal must be the direct path's refusal**, in the direct path's words and for the same part. Any error
//     anywhere on the fast path — a selection, a read, the write door's refusals (ADR-136, ADR-145) — abandons it.
//   - **A selection's ceiling is a factor of the page count** (`selectionCeiling`), and a smaller document has a
//     lower one. A part that names more pages than its intermediate allows is an error there, so: abandoned.
//
// Abandoned means `collectEachDirect` runs, whole — the code this replaced, kept as the definition of the answer.
// `TestASharedSplitGivesThePartsTheDirectSplitGives` holds the two against each other part by part.
//
// The intermediate is never larger than half of what it was cut from, so a split whose parts overlap heavily
// (every part naming most of the document) builds no intermediates and costs what it did.

// errShareAbandoned is the fast path saying its answer could differ from the direct path's.
var errShareAbandoned = errors.New("pdfops: the shared split was abandoned")

// minSharedParts is the fewest parts worth one extra read of the source: below it no half can be both more than
// one part and at most half the document.
const minSharedParts = 4

// collectEach is `Collect(pdf, []string{sel})` for every selection, in order. failed is the index of the
// selection an error belongs to.
func collectEach(pdf []byte, sels []string) (parts [][]byte, failed int, err error) {
	if len(sels) >= minSharedParts {
		if parts, ok := collectEachShared(pdf, sels); ok {
			return parts, 0, nil
		}
	}
	return collectEachDirect(pdf, sels)
}

// collectEachDirect is one full `Collect` of the source per selection: the definition of what a split's parts are.
func collectEachDirect(pdf []byte, sels []string) (parts [][]byte, failed int, err error) {
	parts = make([][]byte, 0, len(sels))
	for i, sel := range sels {
		data, err := Collect(pdf, []string{sel})
		if err != nil {
			return nil, i, err
		}
		parts = append(parts, data)
	}
	return parts, 0, nil
}

// collectEachShared is the fast path; ok false means it was abandoned and nothing it built may be used.
func collectEachShared(pdf []byte, sels []string) (parts [][]byte, ok bool) {
	total, tagged, err := splitSource(pdf)
	if err != nil {
		return nil, false
	}
	share := sharedSplit{tagged: tagged}
	lists := make([][]int, len(sels))
	for i, sel := range sels {
		if lists[i], err = collectPick([]string{sel})(total); err != nil {
			return nil, false
		}
	}
	parts = make([][]byte, len(sels))
	if err := share.cut(pdf, total, lists, parts); err != nil {
		return nil, false
	}
	return parts, true
}

// splitSource is the two facts of the source the shared path decides by: how many pages it has, and whether it
// has a structure tree. It writes nothing, so it reads for inspection.
func splitSource(pdf []byte) (pages int, tagged bool, err error) {
	ctx, err := inspectionRead(pdf)
	if err != nil {
		return 0, false, err
	}
	root, err := ctx.XRefTable.Catalog()
	if err != nil {
		return 0, false, err
	}
	return ctx.PageCount, root["StructTreeRoot"] != nil, nil
}

type sharedSplit struct {
	// tagged is whether the SOURCE has a structure tree: every intermediate document must then carry one.
	tagged bool
}

// cut fills out[i] with the pages lists[i] names of doc, a document of pages pages.
func (s sharedSplit) cut(doc []byte, pages int, lists [][]int, out [][]byte) error {
	if len(lists) == 1 {
		part, err := subsetCarrying(doc, exactly(pages, lists[0]), false, readInPageOrder)
		out[0] = part
		return err
	}
	if union := pagesNamed(lists); len(union)*2 <= pages {
		smaller, err := s.intermediate(doc, pages, union)
		if err != nil {
			return err
		}
		at := make(map[int]int, len(union))
		for i, p := range union {
			at[p] = i + 1
		}
		moved := make([][]int, len(lists))
		for i, l := range lists {
			moved[i] = make([]int, len(l))
			for j, p := range l {
				moved[i][j] = at[p]
			}
		}
		doc, pages, lists = smaller, len(union), moved
	}
	mid := len(lists) / 2
	if err := s.cut(doc, pages, lists[:mid], out[:mid]); err != nil {
		return err
	}
	return s.cut(doc, pages, lists[mid:], out[mid:])
}

// intermediate is doc reduced to the pages in keep (ascending, each once), through the selection's own door —
// refused unless it is the document a part may be cut from in the source's place.
func (s sharedSplit) intermediate(doc []byte, pages int, keep []int) ([]byte, error) {
	out, carried, _, err := subsetOnce(doc, exactly(pages, keep), true, keepUnreadable, false, readInPageOrder)
	if err != nil {
		return nil, err
	}
	if s.tagged && !(carried && carryIsComplete(out)) {
		return nil, errShareAbandoned
	}
	return out, nil
}

// exactly is a selection already decided against a document of pages pages.
func exactly(pages int, keep []int) func(int) ([]int, error) {
	return func(total int) ([]int, error) {
		if total != pages {
			return nil, errShareAbandoned
		}
		return keep, nil
	}
}

// pagesNamed is every page any of lists names, ascending, each once.
func pagesNamed(lists [][]int) []int {
	seen := map[int]bool{}
	var all []int
	for _, l := range lists {
		for _, p := range l {
			if !seen[p] {
				seen[p] = true
				all = append(all, p)
			}
		}
	}
	sort.Ints(all)
	return all
}
