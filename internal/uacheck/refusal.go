package uacheck

// heldRefusal is the package's convention — **a definite failure beats a refusal** — written once (ADR-009).
//
// A rule that scans a population meets two kinds of subject it cannot settle: one whose own reading nib could
// not finish (its `/StructParent` row, its role map, its coverage), and the population itself being short (a
// walk that stopped). Neither says anything about the subjects nib DID read, and returning `CannotCheck` at the
// first of them threw away a defect already in hand, or one a later subject was about to show: a document with
// one unreadable annotation and one plainly untagged one reported "nib could not look" about the second.
// `checkEmbeddedFileNames` stated the rule, and the P07 phase-close review (R4-7) found the annotation, content,
// marked-content and content-language rules still returning at the first refusal. Each now holds it here, judges
// every subject it could read, and answers the held refusal only when none of them failed.
// `TestADefiniteFailureBeatsARefusalAtEverySite` holds one row per site, each veraPDF's verdict.
//
// The FIRST reason is kept, so a report names the earliest subject nib could not settle, as the early return did.
type heldRefusal struct{ why, where string }

// hold keeps a refusal's reason, unless one is already held; an empty reason holds nothing.
func (h *heldRefusal) hold(why, where string) {
	if h.why == "" && why != "" {
		h.why, h.where = why, where
	}
}

// result is the held refusal as a verdict, and whether one is held.
func (h heldRefusal) result() (Result, bool) {
	if h.why == "" {
		return Result{}, false
	}
	return Result{Verdict: CannotCheck, Why: h.why, Where: h.where}, true
}
