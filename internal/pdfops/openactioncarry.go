package pdfops

import (
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// carriedOpenAction prunes the document's `/OpenAction` onto the pages a selection kept
// (/pending 555, ADR-046).
//
// # Why it is worth carrying at all
//
// Measured in `/pending 525`'s corpus scan: **220 of 295** readable PDF_UA-1 catalogs carry an
// `/OpenAction`, which makes it the second most common key `catalogAllowlist` drops after
// `/Outlines`. It is what makes a document open at the place its author meant — a report that opens
// on its summary rather than on its cover — and the subset dropped it silently, so every extract
// opened at page one whatever the source said.
//
// # The key has TWO forms and only one of them is a navigation (ISO 32000-1 §12.3.2)
//
// This is the whole reason `/pending 524` carried the outline and deliberately left this key alone:
// the helper generalises and the DECISION does not.
//
//   - A **destination** — an array, or a name resolved through the `/Dests` name tree. It positions
//     the view and runs nothing. This is the outline's own case exactly, and it uses the outline's
//     own predicate.
//   - An **action dictionary**. It may be `/JavaScript`, `/Launch`, `/SubmitForm` or `/GoToR`, and it
//     may CHAIN to one of those through `/Next`. `StripActive` deletes the key outright
//     (`scan.go:285`) and `Scan` reports its mere presence as high severity, so carrying a dictionary
//     here would re-admit an auto-run hook that the subset drops for free today.
//
// **So the split is by form, and it is the ruling `outlinecarry.go` already made one key over**: a
// `/S /GoTo` action is read for its `/D` and rewritten as a plain destination, and every other
// action — chain and all — is dropped. Navigation is preserved; the executable surface after this
// change is the one that existed before it. Writing a second, looser rule for the same question is
// the ADR-009 shape this repo keeps finding, and it would be looser at the door where the stakes
// are higher: an outline item runs when a user clicks it, and this one runs when the file opens.
//
// # Disambiguating the two, since both can be a Dict
//
// A destination is normally an array, but a NAMED destination resolves to `<< /D [ … ] >>`, so
// "dereferences to a Dict" cannot tell the two apart. `/S` does: an action dictionary requires it
// (Table 196) and a destination dictionary has no such key. The test is therefore `/S` present, not
// `/Type /Action` — `/Type` is optional on an action, so keying on it would read every `/Type`-less
// JavaScript action as a destination and hand it to `destNamesAKeptPage`.
//
// # What a dropped one leaves behind
//
// Nothing, and that is deliberate: a document whose `/OpenAction` went opens at its first page, which
// is what every subset did before this file existed. The page-operation route has no channel to say
// so (`/pending 574`); when it has one, this is one of the sentences it owes.
func carriedOpenAction(xt *model.XRefTable, root types.Dict, keptPages map[int]bool) types.Object {
	raw, has := root["OpenAction"]
	if !has {
		return nil
	}
	dest, act := openActionForm(xt, raw)
	if act != nil {
		// An action, not a destination. Only /GoTo has a destination to salvage, and only its /D
		// is taken: /Next chains away, and /SD (a PDF 2.0 structure destination) goes with the
		// dictionary for the reason `outlineItemKeys` states.
		if nameVal(act, "S") != "GoTo" {
			return nil
		}
		var ok bool
		if dest, ok = act["D"]; !ok {
			return nil
		}
	}
	// The same predicate the outline and `pruneNames` share, so the three cannot disagree about a
	// destination — including the named form, which resolves through the very `/Dests` tree this
	// selection is separately pruning.
	if !destReachesAKeptPage(xt, dest, keptPages) {
		return nil
	}
	return dest
}

// openActionForm splits an `/OpenAction` value into the two things it can be, and it is the ONE
// place that decides which (ADR-009). Exactly one of the results is non-nil for a value that is
// either; both are nil for one that is neither.
//
// **Its second caller is `Scan`, and that caller is why it exists as a function.** `Scan` reported
// the mere PRESENCE of the key as *"Runs an action automatically when the document opens"* at high
// severity — true of an action dictionary and false of a destination, which runs nothing and only
// says where to open. Nobody had noticed because a subset dropped the key outright, so the false
// claim was only ever made about a source document; `carriedOpenAction` carries the destination
// form, and a scanner left as it was would have started making it about 220 of every 295 extracts.
// Two readings of one ambiguous key, disagreeing, is the shape ADR-009 names.
//
// The discriminator is `/S`, not `/Type /Action`: `/Type` is optional on an action dictionary
// (Table 196) and `/S` is required, so keying on `/Type` reads every `/Type`-less JavaScript action
// as a destination — which is the wrong answer in the one direction that matters.
func openActionForm(xt *model.XRefTable, raw types.Object) (dest types.Object, action types.Dict) {
	r, err := xt.Dereference(raw)
	if err != nil || r == nil {
		return nil, nil
	}
	if d, ok := r.(types.Dict); ok {
		if _, isAction := d.Find("S"); isAction {
			return nil, d
		}
	}
	return raw, nil
}
