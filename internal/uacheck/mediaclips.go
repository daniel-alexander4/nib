// SPDX-License-Identifier: MPL-2.0 OR AGPL-3.0-only
//
// Parts of this file are derived from veraPDF 1.30.2 (veraPDF-validation and veraPDF-parser),
// Copyright (c) 2015-2026 veraPDF Consortium, which is offered under GPLv3+ or MPLv2+. nib takes the
// MPL-2.0 option: this file is available under MPL-2.0, and is distributed as part of nib under the
// AGPL-3.0 (MPL 2.0 section 3.3). See THIRD-PARTY-NOTICES.md, "veraPDF".

package uacheck

import (
	"fmt"
	"nib/internal/pdfread"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// The media-clip door — `PLAN-ua-coverage.md` P05.S04.
//
// # The population is reached through ACTIONS, and it is the one subject in this phase `/Annots` does not give
//
// A media clip dictionary sits at `<action>/R/C`: `GFPDRenditionAction.getC` takes the action's `/R`
// (`PDAction.getRendition`, which is a plain `getKey(ASAtom.R)`) and then that rendition's `/C`, **requiring it
// to be a dictionary**. Only a `/S /Rendition` action becomes a `GFPDRenditionAction`, so only such an action
// contributes a clip.
//
// **Seven holders reach an action in veraPDF's model**, read from it rather than guessed — and an action's
// `/Next` chain carries the walk further from any of them:
//
//   - an annotation's `/A` (`GFPDAnnot:381-389`) and its `/AA` entries (`:371-379`);
//   - an outline item's `/A` (`GFPDOutline:75-85`);
//   - the catalog's `/OpenAction` — only when it is an action dictionary, since an ARRAY there is a destination
//     and `GFPDDocument` links it as one (`:180-196`) — and the catalog's `/AA` (`:199-206`);
//   - a form field's `/AA` (`GFPDFormField:139-146`);
//   - a page's `/AA` (`GFPDPage:222-229`).
//
// **A NULL RESULT IS ONLY AS GOOD AS ITS TRIGGER NAME.** This walk dropped the form-field path on a measurement
// whose fixture used `/U` — an ANNOTATION trigger, absent from the field's `{K, F, V, C}` — so veraPDF evaluated
// nothing and the emptiness was read as "fields are not traversed". Re-measured with `/K`, veraPDF fails and nib
// had a false pass. Every holder's list is written out below for that reason.
//
// **The `/AA` trigger names are a FIXED LIST PER HOLDER, not every entry of the dictionary.**
// `PDAbstractAdditionalActions.getActions` iterates `getActionNames()`, which each subclass hard-codes:
// an annotation (and a widget, which inherits it) has ten — `/E /X /D /U /Fo /Bl /PO /PC /PV /PI` — a page has
// two, `/O /C`, and the catalog has five, `/WC /WS /DS /WP /DP`. Walking every entry instead would grade a
// Rendition action under a name veraPDF ignores, which is a false FAIL; the lists below are that method's.
//
// **Cycles are possible on both axes** — an action may name itself through `/Next`, and an outline item through
// `/First` or `/Next` — so the walk keeps a visited set per axis, keyed by dictionary identity the way
// `scanAnnotsAndFields` keys its own. Where a bound bites, the population records a REASON: veraPDF's outline
// traversal is unbounded, so a document with more bookmarks than nib will follow must make both clauses refuse
// rather than pass over a clip nobody looked at.
// The `/AA` trigger names each holder's own `getActionNames()` lists, and nothing else.
var (
	annotTriggers   = []string{"E", "X", "D", "U", "Fo", "Bl", "PO", "PC", "PV", "PI"}
	pageTriggers    = []string{"O", "C"}
	catalogTriggers = []string{"WC", "WS", "DS", "WP", "DP"}
	fieldTriggers   = []string{"K", "F", "V", "C"}
)

type mediaClip struct {
	dict  types.Dict
	where string
}

// mediaClips is every media clip dictionary in the document, with why the population may be short.
func (d *Document) mediaClips() ([]mediaClip, string) {
	if built, why := d.clipsBuild.again(d.clipsErr, "the media-clip walk"); built {
		return d.clipList, why
	}
	seen := map[uintptr]bool{}
	// clipOne adds the clip one action carries, if it is a Rendition action carrying one.
	clipOne := func(action types.Dict, where string) {
		if d.name(action["S"]) != "Rendition" {
			return
		}
		rendition := d.dict(action["R"])
		if rendition == nil {
			return
		}
		clip := d.dict(rendition["C"])
		if clip == nil {
			return
		}
		if seen[dictID(clip)] {
			return
		}
		seen[dictID(clip)] = true
		d.clipList = append(d.clipList, mediaClip{dict: clip, where: where})
	}
	// clipOf walks an action AND its `/Next` chain, which `GFPDAction` links (`:66-90`) and
	// `PDAction.getNext` reads as **either a dictionary or an array** of further actions. Measured both ways: a
	// `/GoTo` whose `/Next` is the Rendition action is a document veraPDF FAILS and nib passed until this.
	//
	// The chain is a graph — an action may name itself — so it carries its own visited set, and a chain longer
	// than the walk bound RECORDS a reason rather than truncating quietly.
	actionsSeen := map[uintptr]bool{}
	var clipOf func(action types.Dict, depth int, where string)
	clipOf = func(action types.Dict, depth int, where string) {
		if action == nil || actionsSeen[dictID(action)] {
			return
		}
		actionsSeen[dictID(action)] = true
		if depth > maxWalkDepth {
			if d.clipsErr == "" {
				d.clipsErr = fmt.Sprintf("an action's /Next chain runs deeper than %d actions; nib stops reading "+
					"there, so any media clip below was never seen", maxWalkDepth)
			}
			return
		}
		clipOne(action, where)
		if next := d.dict(action["Next"]); next != nil {
			clipOf(next, depth+1, where+" → its /Next")
		}
		if arr, err := d.Ctx.DereferenceArray(action["Next"]); err == nil {
			for i, raw := range arr {
				clipOf(d.dict(raw), depth+1, fmt.Sprintf("%s → its /Next %d", where, i))
			}
		}
	}
	// actionsOf walks a holder's `/A` and the `/AA` entries under the trigger names that holder's own
	// `getActionNames()` lists — never every entry.
	actionsOf := func(holder types.Dict, readA bool, triggers []string, where string) {
		if holder == nil {
			return
		}
		// **`readA` is false for the catalog**, which has no `/A` key — `GFPDDocument` links `/OpenAction`,
		// `/OpenActionDestination` and `/AA` and nothing else. Reading one anyway was a false FAIL: measured, a
		// catalog `/A` holding a Rendition action is a document veraPDF evaluates no check on and nib failed.
		if readA {
			clipOf(d.dict(holder["A"]), 0, where+"'s /A")
		}
		aa := d.dict(holder["AA"])
		if aa == nil {
			return
		}
		for _, trigger := range triggers {
			if raw, has := aa[trigger]; has {
				clipOf(d.dict(raw), 0, fmt.Sprintf("%s's /AA /%s", where, trigger))
			}
		}
	}

	// The catalog: `/OpenAction` only when it is an action dictionary — an array there is a destination — and
	// `/AA`.
	actionsOf(d.Catalog, false, catalogTriggers, "the catalog")
	if oa := d.dict(d.Catalog["OpenAction"]); oa != nil {
		clipOf(oa, 0, "the catalog's /OpenAction")
	}

	// Every annotation, through the shared door, so an annotation nib could not read truncates this population
	// too rather than silently shortening it.
	annots, missed := d.annots()
	for _, a := range annots {
		actionsOf(a.dict, true, annotTriggers, a.where)
	}
	if missed != "" && d.clipsErr == "" {
		// First-wins, as every other writer of this field is. It was assigned unconditionally here, so a
		// truncation recorded by the outline or action walk was silently replaced by the annotation
		// door's reason — one reason for a refusal, and it may as well be the first (P05 phase close).
		d.clipsErr = missed
	}

	// Every page's `/AA`.
	for _, pa := range pdfread.Pages(d.Ctx) {
		p, page, err := pa.Nr, pa.Dict, pa.Err
		if err != nil || page == nil {
			if d.clipsErr == "" {
				d.clipsErr = fmt.Sprintf("page %d does not resolve", p)
			}
			break
		}
		actionsOf(page, true, pageTriggers, fmt.Sprintf("page %d", p))
	}

	// Every form field, restricted to ITS four triggers — and the story of this path is the slice's cautionary
	// tale. It was dropped on a measurement that used `/U`, a trigger name in the ANNOTATION list and not in
	// the field's `{K, F, V, C}` (`PDFormFieldActions.java:33`), so veraPDF evaluated nothing and the null
	// result was read as "veraPDF does not traverse fields". Re-measured with `/K`: veraPDF FAILS and nib
	// passed. A false PASS created by a fixture whose trigger name was outside the list under test.
	sc := d.scanAnnotsAndFields()
	for _, sub := range sc.subjects {
		if sub.kind == kindFormField {
			actionsOf(sub.holder, true, fieldTriggers, "a form field")
		}
	}
	if sc.fields != "" && d.clipsErr == "" {
		d.clipsErr = sc.fields
	}
	// An outline item has no `/AA` in veraPDF's model — `GFPDOutline` links its `/A` alone.
	d.outlineActions(func(item types.Dict, where string) { actionsOf(item, true, nil, where) })
	d.clipsBuild.finish()
	return d.clipList, d.clipsErr
}

// outlineTruncated records why the outline walk stopped short, so the clip population's reason is set and both
// clauses answer CannotCheck rather than Pass.
func (d *Document) outlineTruncated(why string) {
	if d.clipsErr == "" {
		d.clipsErr = why
	}
}

// maxOutlineItems bounds how many outline items the clip walk visits in one document.
//
// **It counts ITEMS, not the length of one chain** (the P07 phase-close review, R4-6). The walk used to apply
// `maxWalkDepth` — a NESTING bound, 64 — to the `/Next` chain as well, so an outline with 66 top-level
// bookmarks, an ordinary table of contents, refused 7.18.6.2 t1 and t2 where veraPDF checks every item:
// measured, 70 bookmarks and no clip is no subject to veraPDF and was CannotCheck in nib, and 70 bookmarks whose
// last carries a clip with no `/CT` FAIL t1 there and were CannotCheck here. Termination never needed that
// bound — the visited set already stops a chain that names itself — so what is left to bound is the work, and
// that is the item count. A large book's outline runs to a few thousand items; this is far past that, and a
// document past it still refuses, because the items past the budget were never read.
const maxOutlineItems = 100_000

// outlineActions walks every outline item's actions, following `/First` and `/Next`, with a visited set so an
// outline that names itself cannot spin. 7.2 t2 reads `/Outlines` only for whether an item EXISTS; this is the
// first reader in the package that walks the items themselves.
func (d *Document) outlineActions(visit func(types.Dict, string)) {
	d.outlineActionsWithin(maxOutlineItems, visit)
}

// outlineActionsWithin is outlineActions under a given item budget, and answers how many items it visited — the
// budget is a parameter so a test can put a control just under it without building a hundred thousand items.
//
// Two bounds, each a REFUSAL when it binds and never a quiet stop — veraPDF's `OutlinesHelper` walks the whole
// outline, so an item nib did not read may hold the clip that fails the clause:
//   - the item budget, over the whole outline, siblings and descendants alike;
//   - `maxWalkDepth` on the `/First` descent, which is recursion. It binds only on an item that HAS a `/First`:
//     a leaf at the bound has nothing below it to miss, and recording a truncation there was a false refusal.
func (d *Document) outlineActionsWithin(budget int, visit func(types.Dict, string)) int {
	root := d.dict(d.Catalog["Outlines"])
	if root == nil {
		return 0
	}
	seen := map[uintptr]bool{}
	visited := 0
	var walk func(item types.Dict, depth int, where string)
	walk = func(item types.Dict, depth int, where string) {
		for ; item != nil; item = d.dict(item["Next"]) {
			if seen[dictID(item)] {
				return
			}
			if visited >= budget {
				d.outlineTruncated(fmt.Sprintf("the outline has more than %d items; nib stops reading there, so any "+
					"media clip past them was never seen", budget))
				return
			}
			seen[dictID(item)] = true
			visited++
			visit(item, where)
			child := d.dict(item["First"])
			switch {
			case child == nil:
			case depth < maxWalkDepth:
				walk(child, depth+1, where+" → a child outline item")
			default:
				d.outlineTruncated(fmt.Sprintf("the outline nests deeper than %d levels; nib stops reading "+
					"there, so any media clip below was never seen", maxWalkDepth))
			}
		}
	}
	walk(d.dict(root["First"]), 0, "an outline item")
	return visited
}
