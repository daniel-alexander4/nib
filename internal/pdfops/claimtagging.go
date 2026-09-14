package pdfops

import (
	"fmt"

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
// It returns `ok` false — and the document unchanged — when the content could not honestly carry the
// claim, because every caller's honest answer to that is the same: return the untagged document it
// already has. A caller that would rather fail says so itself; `tagMarkdown` and `commitProposal` do.
// An `err` is reserved for a write that could not be performed at all, which is a different thing
// and which no caller should read as "this document cannot be described".
//
// **The post-condition is checking our own work, not somebody else's.** `honest` already refuses to
// ship a document whose claim its content cannot support; the claim here is one this function just
// made.
func claimTagging(pdf []byte, tier tagSource) (out []byte, ok bool, err error) {
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

// orphanedClaimError is the message a caller uses when it would rather fail than return an untagged
// document — one wording, so two doors cannot describe the same refusal differently.
func orphanedClaimError(door string, pdf []byte) error {
	s := inspectTags(pdf)
	return fmt.Errorf("pdfops: %s produced a document that claims tagging its content does not "+
		"support (%d element(s), %d anchored, %d page(s) with /StructParents) — refusing to return it",
		door, s.elements, s.anchored, s.pagesSP)
}
