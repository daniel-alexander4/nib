package pdfops

import (
	"bytes"
	"fmt"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// The one door that claims tagging — `PLAN-accessibility.md` P06 phase close, ADR-009.
//
// # Why it exists
//
// By the end of P06 there were FOUR doors that build a tree, and each carried its own copy of the
// same three-part law: write `/MarkInfo /Marked true`, record D4's tier, and refuse to return a
// document whose claim its content cannot support. All four agreed — checked at the phase close,
// line by line — and that is exactly the state ADR-009 says not to be in:
//
//	"A rule holding at more than one call site is written ONCE and every site calls it … The guard
//	asserts routing through the door, not the text each site prints — eight copies checked for
//	agreement say nothing about a ninth site added without one."
//
// The ninth site here is P07's and P08's. A fifth tagging door would be written by someone reading
// one of the four, and the copy they did not read is the one that would go missing.
//
// # Why the three parts are one door and not three
//
// Because a document carrying any two of them is worse than one carrying none. `/MarkInfo` without
// a tree is `orphaned()` — ADR-031 law 1, a reader told a document is tagged stops reaching for the
// fallbacks it would otherwise use. A tree without `/MarkInfo` is structure nothing points a reader
// at. And a tree with neither key nor tier is one no later reader can decide how far to trust,
// which is what P06.S03 built the tier for.

// claimTagging writes the catalog's claim of tagging and the tier that produced it, then verifies
// its own work.
//
// given is the document the operation was handed before it built any structure — nil for a door that
// builds from nothing (`tagMarkdown`). pdf is that document with the structure written. **The claim is
// judged against given**, because what the door must not do is ADD a claim, and only the input says
// whether one was already there.
//
// It returns `ok` false — and the document unchanged — when the content could not honestly carry the
// claim, because every caller's honest answer to that is the same: return the untagged document it
// already has. A caller that would rather fail says so itself; `tagMarkdown` and `commitProposal` do.
// An `err` is reserved for a write that could not be performed at all, which is a different thing
// and which no caller should read as "this document cannot be described".
//
// **The post-condition is checking our own work, not somebody else's.** `honest` already refuses to
// ship a document whose claim its content cannot support; the claim here is one this function just
// made.
func claimTagging(given, pdf []byte, tier tagSource) (out []byte, ok bool, err error) {
	// **Asked before it is written, not only verified after.** `orphaned()` cannot answer this
	// question: it returns false for a document that claims nothing, because a document making no
	// claim cannot be lying — so a document with no tree is not orphaned until `/MarkInfo` makes it
	// so. `supportsAClaim` is the same law asked the other way round, and asking first is what
	// makes every refusal here one kind of thing rather than two: an honest "cannot claim this"
	// instead of a write error from `setTagSource`, which refuses a sourceless document one layer
	// down and would otherwise reach the caller as a failure.
	if !inspectTags(pdf).supportsAClaim() {
		return pdf, false, nil
	}
	var before tagState
	if given != nil {
		before = inspectTags(given)
	}
	// **A NEW claim may not cover text nothing describes** — `/pending 495`. `supportsAClaim` asks
	// whether the tree reaches a live page, and a `/Form` element's OBJR reaches one: authoring a form
	// on an untagged text PDF wrote `/Marked true` over a body no element describes, `tagged` came back
	// true, and veraPDF failed 7.1 t3 on the result. The OCR door did the same over a document that
	// already had text. So a claim this door would be the first to make is refused while any text run
	// is neither under an MCID nor inside an `/Artifact`.
	//
	// **An input that already claims honestly is exempt**, because the operation adds no claim: a form
	// described into a tagged document that has an untagged page leaves that page exactly as claimed as
	// it was, and refusing would cost the widget its description to protect nothing.
	//
	// **Text only**, measured: every door's honest output has zero such runs, while an unmarked path or
	// image is also 7.1 t3 but is what the OCR door's scan image and a commit's uncovered image are
	// today — refusing on them would switch those doors off rather than fix them (see the report of
	// `/pending 495` for that residue).
	if !before.claimsHonestly() {
		if n, rerr := unmarkedTextRuns(pdf); rerr != nil || n > 0 {
			return pdf, false, nil
		}
	}
	// **The tier never rises above the tree the document already had** — `/pending 495`'s sibling. A
	// form described into an OCR'd document recorded `Exact` over a tree that is mostly an OCR engine's
	// opinion, and D4's tier is exactly how far a reader may trust the WHOLE tree. A tree nib did not
	// record stays unrecorded: calling it `Exact` because a widget was added is `StructureSource`'s own
	// rule broken ("an unrecorded tree is NOT reported as exact").
	record := true
	if before.tree {
		if before.source != "" {
			tier = lowerTier(before.source, tier)
		} else {
			record = false
		}
	}
	out, err = writeMutated(pdf, func(ctx *model.Context) error {
		cat, cerr := ctx.XRefTable.Catalog()
		if cerr != nil {
			return cerr
		}
		mi, _ := ctx.DereferenceDict(cat["MarkInfo"])
		if mi == nil {
			mi = types.Dict{}
		}
		mi["Marked"] = types.Boolean(true)
		cat["MarkInfo"] = mi
		if !record {
			return nil
		}
		return setTagSource(ctx, tier)
	})
	if err != nil {
		return nil, false, err
	}
	if s := inspectTags(out); s.orphaned() {
		return nil, false, nil
	}
	return out, true, nil
}

// unmarkedTextRuns counts the text runs, on every page and in the forms they draw, that are neither
// under an MCID nor inside an `/Artifact` — content a claim of tagging would leave undescribed (ua1 7.1
// t3). It reads through `readPageRuns`, the reader the tree writers bracket by, so what counts as
// marked here is what they mark. A page that cannot be read is an error: a claim over content nobody
// could read is not one this door can verify.
func unmarkedTextRuns(pdf []byte) (int, error) {
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		return 0, err
	}
	n := 0
	for p := 1; p <= ctx.PageCount; p++ {
		pr, perr := readPageRuns(ctx, p)
		if perr != nil {
			return 0, perr
		}
		for _, r := range pr.runs {
			if r.mcid < 0 && !r.artifact {
				n++
			}
		}
	}
	return n, nil
}

// orphanedClaimError is the message a caller uses when it would rather fail than return an untagged
// document — one wording, so two doors cannot describe the same refusal differently.
func orphanedClaimError(door string, pdf []byte) error {
	s := inspectTags(pdf)
	return fmt.Errorf("pdfops: %s produced a document that claims tagging its content does not "+
		"support (%d element(s), %d anchored, %d page(s) with /StructParents) — refusing to return it",
		door, s.elements, s.anchored, s.pagesSP)
}
