package uacheck

import (
	"fmt"

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
	if d.clipsDone {
		return d.clipList, d.clipsErr
	}
	d.clipsDone = true
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
	if missed != "" {
		d.clipsErr = missed
	}

	// Every page's `/AA`.
	for p := 1; p <= d.Ctx.PageCount; p++ {
		page, _, _, err := d.Ctx.PageDict(p, false)
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
	return d.clipList, d.clipsErr
}

// outlineTruncated records why the outline walk stopped short, so the clip population's reason is set and both
// clauses answer CannotCheck rather than Pass.
func (d *Document) outlineTruncated(why string) {
	if d.clipsErr == "" {
		d.clipsErr = why
	}
}

// outlineActions walks every outline item's actions, following `/First` and `/Next` under the walk bound.
//
// The bound is `maxWalkDepth` on the `/First` descent and on the `/Next` chain together, with a visited set, so
// an outline that names itself cannot spin. 7.2 t2 reads `/Outlines` only for whether an item EXISTS; this is
// the first reader in the package that walks the items themselves.
func (d *Document) outlineActions(visit func(types.Dict, string)) {
	root := d.dict(d.Catalog["Outlines"])
	if root == nil {
		return
	}
	seen := map[uintptr]bool{}
	var walk func(item types.Dict, depth int, where string)
	walk = func(item types.Dict, depth int, where string) {
		n := 0
		for ; item != nil; n++ {
			if seen[dictID(item)] {
				return
			}
			if n > maxWalkDepth {
				// **A truncation must be a REASON, not a quiet stop.** veraPDF's `OutlinesHelper` walks the
				// outline unbounded with only a containment check, so a document with 70 top-level bookmarks —
				// an ordinary table of contents — would otherwise have its 70th item's action silently unread,
				// and both clauses would report Pass over a clip nib never looked at.
				d.outlineTruncated(fmt.Sprintf("the outline has more than %d items in one chain; nib stops "+
					"reading there, so any media clip below was never seen", maxWalkDepth))
				return
			}
			seen[dictID(item)] = true
			visit(item, where)
			if depth < maxWalkDepth {
				walk(d.dict(item["First"]), depth+1, where+" → a child outline item")
			} else {
				d.outlineTruncated(fmt.Sprintf("the outline nests deeper than %d levels; nib stops reading "+
					"there, so any media clip below was never seen", maxWalkDepth))
			}
			item = d.dict(item["Next"])
		}
	}
	walk(d.dict(root["First"]), 0, "an outline item")
}
