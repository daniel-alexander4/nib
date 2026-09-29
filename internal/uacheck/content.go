package uacheck

import (
	"fmt"
	"sort"
	"strconv"

	"nib/internal/contentstream"
	"nib/internal/fontcode"
	"nib/internal/pdfread"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// Walking page content — `PLAN-accessibility.md` P07.S03.
//
// `7.1 t3` asks whether every piece of content is either tagged or an artifact, and `7.2 t34` asks
// whether every piece of TEXT has a determinable language. Both are questions about operators in
// content streams and about the marked-content sequences around them, so both read one walk.
//
// # Why tokens and not bytes
//
// `contentstream.Tokenize` returns a string operand as one token and an inline image as one opaque
// token, so image bytes that happen to spell `EMC` cannot close a sequence and text that happens to
// spell `/Artifact` cannot open one. P06 built the tokenizer for exactly this; the same reasoning
// applies to reading.

// contentEvent is one drawing operator and what surrounds it.
type contentEvent struct {
	where string // human-readable location, modelled on veraPDF's context path
	text  bool   // a text-showing operator (Tj, TJ, ', ")
	// appearance is whether the operator is in an annotation's appearance stream rather than page
	// content. 7.1 t3 reads page content only — veraPDF does not apply it to appearances — while
	// the font rules read both, because an appearance stream renders glyphs like any other stream.
	appearance bool
	// font is the font dictionary in force for a text operator, resolved through the stream's
	// resources, and fontName the resource name it was selected by (`/F1`). Nil when no `Tf` had set
	// one, which a conforming stream never does and which the font rules report as CannotCheck.
	font     types.Dict
	fontName string
	fontObj  int
	// invisible is render mode 3 (neither fill nor stroke). veraPDF does not count invisible text
	// as "used for rendering": measured, a non-embedded Helvetica in `3 Tr` passes 7.21.4.1 and the
	// same text drawn visibly fails it. An OCR layer is exactly this text.
	invisible bool
	// mode is the render mode in force (P07.S04: a font drawn in two modes is two font objects to veraPDF).
	mode int
	// offPage is drawing inside a form XObject or an annotation appearance, not a page's own content.
	offPage bool
	// covered is 7.1 t3's disjunction, `isTaggedContent == true || parentsTags.contains('Artifact')`:
	// some enclosing sequence is an `/Artifact`, or the innermost struct parent in force reaches the
	// structure tree root. **An MCID is not enough** — see `taggedContent`.
	covered bool
	// coverUnread is why nib could not settle `covered`, and it is `CannotCheck`, never a Pass. It is
	// empty whenever `covered` is true.
	coverUnread string
	// langDetermined is whether anything enclosing this content — IN THIS STREAM — determines its
	// language: a sequence's own property-list `/Lang` (`/pending 489`), or the structure element an
	// `/MCID` names, by that element's own `/Lang` or an ancestor's. It is 7.2 t34's `Lang != null`, and
	// it is the same `inheritedLangOf` the Span rules read, which is the point: the question had two
	// implementations that disagreed (`/pending 635`) and now has one door (ADR-009).
	langDetermined bool
	// langUnread is why nib could not settle it, and it is `CannotCheck`, never a Pass.
	langUnread string
	// mcid and spKey are the innermost `/MCID` in force IN THIS STREAM, or -1. **They exist only to word
	// a failure**, which is not decoration: deleting the "in no tagged sequence" branch once left the rule
	// green while handing the user a reason about a missing structure element, so the three shapes 7.2 t34
	// can fail in are told apart by name. Nothing reads them on the passing path.
	mcid, spKey int
}

// textState is the part of the graphics state the font rules need. `Tf` and `Tr` are graphics
// state, saved by `q` and restored by `Q`, and NOT reset by `BT`/`ET` — both measured against veraPDF:
// a `3 Tr` inside `q … Q` does not survive the `Q`, and one set inside a closed `BT … ET` does survive
// into the next text object.
//
// **The font is BOUND at `Tf`**, to the object the name resolves to in the resources in force there, and a
// form XObject inherits that object — not the name. veraPDF resolves it in `OperatorParser`'s `Tf` branch
// (`OperatorParser.java:371`); resolving the name again at the show let a form whose own /Resources rebinds `/F0`
// judge its text against the wrong font, both ways round (the P07.S02 review, measured on veraPDF).
type textState struct {
	fontName   string
	font       types.Dict
	fontObj    int
	renderMode int
}

// frame is one open marked-content sequence — and, once it closes, one `SEMarkedContent` subject.
//
// **veraPDF adds the subject in the `EMC` branch**, so an UNBALANCED `BMC`/`BDC` is no subject at all:
// its operators fall to `GFSEUnmarkedContent` instead (`GFPDSemanticContentStream.java:109-121`,
// measured — a `BDC` with no `EMC` yields one subject on a page holding two sequences, not two).
type frame struct {
	// tag is the sequence's tag operand without its slash, or "" when the operator does not carry one
	// where veraPDF looks for it. See `markedContentTag` — the position differs between BMC and BDC,
	// and a malformed `/Artifact BDC` written with no property list has NO tag and breaks no rule.
	tag      string
	artifact bool // tag == "Artifact"
	mcid     int
	spKey    int
	// lang is whether the sequence's own property list declares a `/Lang`.
	lang bool
	// actualText, alt and expansion are whether the property list carries `/ActualText`, `/Alt` and `/E`
	// as strings — 7.2 t30, t31 and t32 each ask about one of them.
	actualText, alt, expansion bool
	// elem is the structure element this sequence's OWN `/MCID` resolves to through the parent tree, and
	// elemUnread why nib could not tell. Exactly one is ever set.
	//
	// **A sequence sets NEITHER when it has no `/MCID` — and also when its `/MCID` resolves to nothing**, and
	// both then inherit the struct parent of the sequence around them. That is veraPDF's own fallback
	// (`GFOp_BDC.getStructElem` returns null on a failed lookup and `GFOpMarkedContent.java:172-174` takes the
	// enclosing one), and it is MEASURED rather than reasoned: `/Span <</MCID 5>>` naming a slot the parent
	// tree does not hold FAILS 7.1 t3 at the top level and PASSES nested inside a `/Div <</MCID 0>>` that
	// resolves. A review read this as the false pass surviving one level of nesting; the oracle says it is the
	// rule.
	elem       types.Dict
	elemUnread string
	// stream numbers the content stream the sequence was opened in.
	//
	// **`inheritedLang` does not cross a stream boundary while `parentsTags` and the struct parent do**,
	// and all three were measured in both directions: a `/Lang` in force on the page does not reach a Span
	// inside a form the page draws (`GFOpMarkedContent.java:153-166` walks a per-parser stack), while an
	// `/Artifact` around the `Do` does reach it (`OperatorParser.java:538-550`).
	stream int
	where  string

	// **The aggregates below make every question the rules ask of the stack O(1)** (the P07 phase-close review,
	// R3-2: every event and every closed sequence used to walk the whole stack, and 40,000 nested `BMC`s took
	// 13.5 s). Each is set ONCE, by `pushFrame`, from this frame and the frame directly below it — which already
	// carries the answer for everything beneath — so each one is exactly the old full scan over the stack up to
	// and including this frame. The three inheritances differ in how they treat a stream boundary, and each
	// aggregate keeps its own inheritance's rule:
	//
	//   - anyArtifact is `parentsTags.contains('Artifact')`: the own tag included, and it CROSSES the boundary.
	//   - structElem/structUnread is the innermost struct parent in force (`taggedContent`): it CROSSES too.
	//   - mcidIn/spKeyIn and langIn/langWhy stop at the boundary: they carry the frame below only when that frame
	//     was opened in the same stream. `inheritedLang`'s boundary is the WALKER's stream, which the query
	//     (`inheritedLangOf`) enforces by comparing it with the top frame's stream.
	//
	// `TestTheStackAggregatesAgreeWithTheFullScan` compares every one with the old scan over random nestings.
	anyArtifact  bool
	structElem   types.Dict
	structUnread string
	mcidIn       int
	spKeyIn      int
	// ownDecided is whether this frame's element settles the inherited language on its own (its own `/Lang`,
	// an ancestor's, or a refusal), and ownLangFound/ownLangWhy that answer. It excludes the property list's
	// own `/Lang`, which a sequence asking about itself does not count (`inheritedLangOf`'s `ownCounts`).
	ownDecided   bool
	ownLangFound bool
	ownLangWhy   string
	// langIn/langWhy is `inheritedLangOf(stack[:this+1], this.stream, ownCounts=true)`.
	langIn  bool
	langWhy string
}

// pushFrame opens f on top of stack, filling f's aggregates from the frame below (see `frame`).
//
// The appended slice may share `stack`'s backing array: a nested walk pushes only above the frames it
// inherited and pops back only down to them, and its caller never reads past its own length, so no frame a
// caller can see is ever overwritten (`walkWithState`).
func (d *Document) pushFrame(stack []frame, f frame) []frame {
	var below *frame
	if n := len(stack); n > 0 {
		below = &stack[n-1]
	}
	sameStream := below != nil && below.stream == f.stream
	f.anyArtifact = f.artifact || (below != nil && below.anyArtifact)
	switch {
	case f.elem != nil:
		f.structElem, f.structUnread = f.elem, ""
	case f.elemUnread != "":
		f.structElem, f.structUnread = nil, f.elemUnread
	case below != nil:
		f.structElem, f.structUnread = below.structElem, below.structUnread
	}
	f.mcidIn, f.spKeyIn = -1, -1
	if f.mcid >= 0 {
		f.mcidIn, f.spKeyIn = f.mcid, f.spKey
	} else if sameStream {
		f.mcidIn, f.spKeyIn = below.mcidIn, below.spKeyIn
	}
	switch {
	case f.elem != nil:
		if d.declaresLang(f.elem["Lang"]) {
			f.ownDecided, f.ownLangFound = true, true
		} else if found, why := d.parentLang(f.elem); why != "" {
			f.ownDecided, f.ownLangWhy = true, why
		} else if found {
			f.ownDecided, f.ownLangFound = true, true
		}
	case f.elemUnread != "":
		f.ownDecided, f.ownLangWhy = true, f.elemUnread
	}
	switch {
	case f.lang:
		f.langIn = true
	case f.ownDecided:
		f.langIn, f.langWhy = f.ownLangFound, f.ownLangWhy
	case sameStream:
		f.langIn, f.langWhy = below.langIn, below.langWhy
	}
	return append(stack, f)
}

// mcSubject is one closed marked-content sequence as veraPDF's `SEMarkedContent` — the subject of
// `7.1 t1`, `7.1 t2` and `7.2 t30`/`t31`/`t32`.
//
// It is built where the sequence closes, from the stack in force there, because each of its three
// inheritances reads a different part of that stack (`subject`).
type mcSubject struct {
	where string
	tag   string
	// actualText, alt and expansion are the three keys t30, t31 and t32 ask about, and ownLang the
	// sequence's own `/Lang` — the rules' `Lang != null` disjunct.
	actualText, alt, expansion, ownLang bool
	// inheritedLang is the rules' `inheritedLang != null`, and langUnread why nib could not settle it.
	inheritedLang bool
	langUnread    string
	// tagged is `isTaggedContent`, and taggedUnread why nib could not settle it.
	tagged       bool
	taggedUnread string
	// insideArtifact is `parentsTags.contains('Artifact')`, the OWN tag included.
	insideArtifact bool
}

// textOperators and paintOperators are the operators that put something on the page.
//
// Path CONSTRUCTION (`m`, `l`, `re`…) and clipping (`W`, `n`) draw nothing; only painting does.
var (
	textOperators  = map[string]bool{"Tj": true, "TJ": true, "'": true, `"`: true}
	paintOperators = map[string]bool{
		"f": true, "F": true, "f*": true, "B": true, "B*": true, "b": true, "b*": true,
		"S": true, "s": true, "sh": true,
	}
)

// contentEvents classifies every drawing operator in the document, once.
//
// A page with no content is simply absent from the events. A content stream that cannot be read is
// recorded in `contentErr`, and the rules turn that into `CannotCheck` — nib failing to read a stream
// is not the stream being untagged.
func (d *Document) contentEvents() ([]contentEvent, string) {
	if d.contentDone {
		// **A walk that did not finish must not read as a complete one.** A panic inside it (recovered per rule by
		// `runOne`) left the population half built with no error, and every LATER rule read it as whole — the P07.S02
		// re-review measured 7.1 t3 going from Fail to NotApplicable that way.
		if !d.contentFinished && d.contentErr == "" {
			return d.content, "the content walk stopped part-way on an internal error, so what it had not reached was never read"
		}
		return d.content, d.contentErr
	}
	d.contentDone = true
	for p := 1; p <= d.Ctx.PageCount; p++ {
		page, _, _, err := d.Ctx.PageDict(p, false)
		if err != nil || page == nil {
			d.contentErr = fmt.Sprintf("page %d does not resolve", p)
			return d.content, d.contentErr
		}
		objNr := 0
		if ir, e := d.Ctx.PageDictIndRef(p); e == nil && ir != nil {
			objNr = ir.ObjectNumber.Value()
		}
		// **pdfcpu's join, not `pdfread.PageContent`'s (ADR-056).** The checker reads as veraPDF does (ADR-052), and
		// how veraPDF joins a divided `/Contents` is unmeasured; a join nib chose here could make it disagree with the
		// oracle it is scored against. /pending 719. It is still read through pdfread's door, which bounds the join
		// at `maxContentBytes` (`/pending 748`: one stream named six times ran this check 13.7 s at 3.2 GiB).
		src, cerr := pdfread.PageContentAsPdfcpu(d.Ctx, page, p)
		if cerr == model.ErrNoContent || len(src) == 0 {
			continue
		}
		if cerr != nil {
			d.contentErr = fmt.Sprintf("page %d's content stream could not be read: %v", p, cerr)
			return d.content, d.contentErr
		}
		spKey := -1
		if sp, ok := d.intValue(page["StructParents"]); ok {
			spKey = sp
		}
		// A page's content is a stream veraPDF traverses once per object KEY: two pages naming one
		// content stream traverse it once, and an array names no key at all (`retraversal`).
		contentsNr := 0
		if ir, ok := page["Contents"].(types.IndirectRef); ok {
			contentsNr = ir.ObjectNumber.Value()
		}
		w := walker{d: d, where: fmt.Sprintf("page %d (object %d)", p, objNr), spKey: spKey, stream: d.nextStream(),
			repeat: d.retraversal(contentsNr, false)}
		w.walk(src, d.resourcesOf(page), nil, map[int]bool{}, 0)
	}
	d.walkAppearances()
	// Set on RETURN only — never deferred, since a deferred assignment runs while a panic unwinds too.
	d.contentFinished = true
	return d.content, d.contentErr
}

// walkAppearances walks every annotation's appearance streams, as page content's sibling.
//
// veraPDF locates the authored form's non-embedded Helvetica at
// `annots[0]/appearance[0]/contentStream[0]/operators[10]/font[0]` — an appearance stream renders
// glyphs, so the font rules must read it. It is walked AFTER page content, and its events are
// flagged, because 7.1 t3 does not apply to appearances and must not start failing forms on content
// veraPDF does not ask about.
func (d *Document) walkAppearances() {
	// The population is the shared annotation door's (`annots.go`, ADR-009): the question "which entries
	// of this page's /Annots are annotations" is one rule, and it used to be answered here, in
	// `scanAnnotsAndFields` and in `checkWidgetsInFormElements` independently.
	subjects, missed := d.annots()
	for _, a := range subjects {
		p, i, ad, page := a.page, a.index, a.dict, a.pageDict
		// **An INDIRECT annotation is tallied once, however many pages name it**: it has an object key,
		// so veraPDF's validator visits it once (measured: one annotation on two pages PASSES `7.20 t2`).
		// **A DIRECT one has no key** — veraPDF gives it a fresh id each time it is reached
		// (`GFPDObject.java:74-80`) — so one direct annotation in an `/Annots` array two pages share is
		// visited twice, and measured, FAILS. Keyed by object number, never by dictionary identity, which
		// is the same map for both of those shapes (the slice review's finding 2). Each appearance entry of
		// one visit is its own `PDXForm`, so `/N` and `/D` naming one stream are two (measured FAILED).
		firstVisit := true
		if a.object != 0 {
			if d.reachedAnnots == nil {
				d.reachedAnnots = map[int]bool{}
			}
			firstVisit = !d.reachedAnnots[a.object]
			d.reachedAnnots[a.object] = true
		}
		ap := d.dict(ad["AP"])
		if ap == nil {
			if ad["AP"] != nil {
				// An /AP that is there and does not resolve to a dictionary is an appearance nib did not
				// read, not an annotation without one (`/pending 507`).
				d.contentErr = fmt.Sprintf("page %d annotation %d carries an /AP that is not a dictionary, "+
					"so its appearance streams were never walked", p, i)
				return
			}
			continue
		}
		for _, key := range []string{"N", "R", "D"} {
			states := d.appearanceStreams(ap[key])
			names := make([]string, 0, len(states))
			for state := range states {
				names = append(names, state)
			}
			// In a fixed order, so a report names the same two appearances for the same file every run.
			sort.Strings(names)
			for _, state := range names {
				so := states[state]
				sd, _, serr := d.Ctx.DereferenceStreamDict(so)
				if serr != nil {
					// An /AP entry that is not a stream is an appearance nib did not read (`/pending 507`).
					// `sd == nil` with no error is NOT this: the entry resolves to null, so there is no
					// appearance to walk. Measured against pdfcpu v0.13.0, `open`'s validator refuses every
					// non-stream /AP entry ahead of the rules, on every annotation subtype tried
					// ("DereferenceStreamDict: wrong type"), so nothing reaches this today.
					d.contentErr = fmt.Sprintf("page %d annotation %d's /AP /%s entry is not a stream nib can "+
						"read, so what it draws was never walked: %v", p, i, key, serr)
					return
				}
				if sd == nil {
					continue
				}
				apNr := 0
				if ir, ok := so.(types.IndirectRef); ok {
					apNr = ir.ObjectNumber.Value()
				}
				src, derr := d.decodedContent(sd, apNr)
				if derr != nil {
					d.contentErr = fmt.Sprintf("page %d annotation %d's /AP /%s stream could not be decoded: %v", p, i, key, derr)
					return
				}
				res := d.dict(sd.Dict["Resources"])
				if res == nil {
					res = d.resourcesOf(page)
				}
				label := fmt.Sprintf("page %d, annotation %d, appearance /%s", p, i, key)
				if state != "" {
					label += " /" + state
				}
				// **An appearance stream IS a form XObject**, and veraPDF grades it as one: measured,
				// a widget's `/AP /N` carrying `/Ref` fails 7.20 t1 on a document that draws nothing
				// else. Recorded here rather than at the `Do` operator, because nothing draws it —
				// the annotation is what puts it on the page.
				d.recordDrawnForm(sd.Dict, apNr, label, firstVisit)
				w := walker{d: d, where: label, spKey: -1, appearance: true, stream: d.nextStream(),
					repeat: d.retraversal(apNr, !firstVisit), form: apNr}
				w.walk(src, res, nil, map[int]bool{}, 0)
			}
		}
	}
	// The appearances past the page the door stopped at were never read, so nothing they draw may be
	// reported as read (`/pending 507`). `rules_content.go` says why nothing reaches this today.
	//
	// **Set LAST and only when nothing else failed, because the door stops the population at that page.**
	// Every subject above therefore sits on an earlier page, so an `/AP` failure among them is the
	// FIRST failure in page order — which is the one the early return this replaced used to report.
	if missed != "" && d.contentErr == "" {
		d.contentErr = missed + ", so the appearance streams there were never walked"
	}
}

// appearanceStreams returns the streams under one /AP entry, keyed by appearance state — an entry is
// either a stream itself or a dictionary of states (`/Off`, `/Yes`) each naming a stream.
func (d *Document) appearanceStreams(o types.Object) map[string]types.Object {
	out := map[string]types.Object{}
	if o == nil {
		return out
	}
	if sd, _, err := d.Ctx.DereferenceStreamDict(o); err == nil && sd != nil {
		out[""] = o
		return out
	}
	if states := d.dict(o); states != nil {
		for k, v := range states {
			out[k] = v
		}
	}
	return out
}

// walker carries one stream's context through a walk.
type walker struct {
	d          *Document
	where      string
	spKey      int
	appearance bool
	// stream is this stream's number, taken from `d.streams` when the walk starts.
	stream int
	// langOnly walks a stream for 7.2 t29's `/Lang` values and NOTHING else: no drawing event, no
	// marked-content subject. It is how the tiling patterns and Type 3 glyph procedures veraPDF reads
	// `CosLang` in are read without putting their content in front of the rules that must not see it
	// (`walkPattern`, `walkType3`).
	langOnly bool
	// repeat is whether this stream is one veraPDF does NOT traverse here, because it already traversed
	// the same object — so a `Do` inside it builds no `PDXForm` and is not tallied for `7.20 t2`. It
	// gates that tally and nothing else (`retraversal`).
	repeat bool
	// form is the object number of the form XObject whose content this walk is inside — itself for a form
	// or appearance walk, inherited by a pattern or glyph procedure it enters, 0 in page content. It lets
	// `formTwins`' caller ask whether a form draws any other, which is when a fusion could move a count.
	form int
}

// walk classifies one content stream's drawing operators, recursing into form XObjects.
//
// `inherited` is the marked-content state in force where a form was invoked: content inside a form
// drawn from within an `/Artifact` or an MCID sequence is covered by that sequence. `chain` holds
// the forms currently being walked, so a form that draws itself stops rather than recursing forever
// — while the SAME form invoked twice from one page is still walked twice, as it is drawn twice.
func (w walker) walk(src []byte, res types.Dict, inherited []frame, chain map[int]bool, depth int) {
	w.walkWithState(src, res, inherited, chain, depth, textState{})
}

// maxFormDepth bounds how deeply form XObjects are walked inside one another. Lower than `maxWalkDepth`
// because a form invoked twice at every level is walked twice at every level, so the cost is exponential
// in the depth. **Reaching it is `contentErr`, never a silent stop** (`/pending 496`): the forms below were
// never read, and until then twelve forms of untagged text read as a page that draws nothing.
const maxFormDepth = 8

// maxFormWalks and maxContentEvents bound the whole content walk, because the depth bound alone does not: a
// form that draws another form N times is walked N times at every level, so eight levels of fan-out ten is
// 10^8 walks from a file of a few kilobytes (P03's phase-close review measured 10^6 events at six levels:
// 2.2 s and 2.6 GB). **Past either one the walk stops and `contentErr` says so**, so every rule reading the
// events answers CannotCheck — never a Pass over the part it did not read (law 4). A real document is nowhere
// near either: its forms are walked once per `Do`, and its events are its own drawing operators.
const (
	maxFormWalks     = 1 << 16
	maxContentEvents = 1 << 20
	// maxContentOperators counts every operator, drawing or NOT — including an inline image, which is one token
	// and used to spend nothing, so a stream of `BI … EI` grew the events with no ceiling evaluated at all.
	// A stream of marked-content operators draws nothing
	// and trips neither budget above, and 4,368 walks of one ran 10.5 s and 13.6 GB (the P04.S01 review, measured).
	maxContentOperators = 1 << 24
	// maxCharProcAsks bounds how often `enterType3` meets a glyph procedure that is already read (R3-1). A real
	// document meets none: each `/CharProcs` dictionary is enumerated once. Only procedures shared between fonts,
	// or a font shown inside its own glyphs, spend it — and they spend it per procedure, not per operator.
	maxCharProcAsks = 1 << 16
	// maxContentBytes counts every byte walked, once per walk — so a form drawn N times is N times its size, the
	// cost the walk actually pays. Whitespace and comments are no operator and spent nothing, and one 64 MiB form of
	// spaces drawn 80 times ran 25.1 s (`/pending 721`, measured). 512 MiB is ADR-005's ceiling on a whole open
	// document, and it sits above what `maxContentOperators` admits of real content (~10-20 bytes an operator), so
	// it binds on padding, not on drawing.
	maxContentBytes = pdfread.MaxPageContentBytes
)

// decodedContent is the ONE door through which the content walk decodes a nested stream — a form XObject, an
// appearance stream, a tiling pattern or a Type 3 glyph procedure — and it decodes each object once per check.
//
// **Cached here because pdfcpu does not**: `DereferenceStreamDict` hands back a fresh copy of the stream dictionary
// at every call, so `Decode` on it inflated the same form again at every `Do` (`/pending 721`, 0.29 s a draw for a
// 64 MiB form). Only the decoded BYTES are kept, never what a walk made of them: a form's reading depends on the
// graphics state and marked-content stack it is drawn in, so every draw is still walked, and charged
// (`maxContentBytes`). An inline stream (object number 0) has no key and is decoded where it is met.
func (d *Document) decodedContent(sd *types.StreamDict, objNr int) ([]byte, error) {
	if src, ok := d.decoded[objNr]; ok && objNr != 0 {
		return src, nil
	}
	d.streamDecodes++
	// Decoded only as far as the byte budget has left (`/pending 748`): the first walk charges the whole stream,
	// so a stream past what is left can never be walked, and inflating it first was the cost — one 400 KB form
	// inflated to 400 MiB before anything could refuse it.
	if err := pdfread.DecodeWithin(sd, int64(maxContentBytes-d.contentBytes)); err != nil {
		return nil, err
	}
	if objNr != 0 {
		if d.decoded == nil {
			d.decoded = map[int][]byte{}
		}
		d.decoded[objNr] = sd.Content
	}
	return sd.Content, nil
}

// overBudget reports whether the content walk has spent its budget, recording why the first time.
func (d *Document) overBudget() bool {
	var why string
	switch {
	case d.contentOps > maxContentOperators:
		why = fmt.Sprintf("the page content, with every form XObject it draws, runs more than %d operators; nib stops "+
			"reading there, so what lies beyond was never read", maxContentOperators)
	case d.contentBytes > maxContentBytes:
		why = fmt.Sprintf("the page content, with every form XObject it draws, runs more than %d bytes of content "+
			"(a form is counted at every draw); nib stops reading there, so what lies beyond was never read", maxContentBytes)
	case d.formWalks > maxFormWalks:
		why = fmt.Sprintf("the page content enters nested streams — form XObjects, and the tiling patterns and "+
			"Type 3 glyph procedures read for their /Lang — more than %d times (a form drawn inside forms fans "+
			"out); nib stops reading there, so what lies beyond was never read", maxFormWalks)
	case len(d.mcSubjects) > maxContentEvents:
		why = fmt.Sprintf("the page content, with every form XObject it draws, opens more than %d marked-content "+
			"sequences; nib stops reading there, so what lies beyond was never read", maxContentEvents)
	case d.charProcAsks > maxCharProcAsks:
		why = fmt.Sprintf("the page content asks for Type 3 glyph procedures it has already read more than %d times "+
			"(fonts sharing procedures, or a font shown inside its own glyphs); nib stops reading there, so what lies "+
			"beyond was never read", maxCharProcAsks)
	case d.glyphsOver:
		why = fmt.Sprintf("the page content, with every form XObject it draws, shows more than %d distinct glyphs; nib "+
			"stops reading there, so the glyphs beyond were never read", maxDistinctGlyphs)
	case d.glyphCodes > maxGlyphCodes:
		why = fmt.Sprintf("the page content, with every form XObject it draws, shows more than %d character codes; nib "+
			"stops reading there, so the glyphs beyond were never read", maxGlyphCodes)
	case len(d.content) > maxContentEvents:
		why = fmt.Sprintf("the page content, with every form XObject it draws, holds more than %d drawing operators; "+
			"nib stops reading there, so what lies beyond was never read", maxContentEvents)
	default:
		return false
	}
	if !d.contentOver {
		d.contentOver = true
		d.contentErr = why
	}
	return true
}

// walkWithState is walk with the text state in force at the point of invocation — a form XObject
// inherits the invoking stream's graphics state (ISO 32000-1 §8.10.1).
func (w walker) walkWithState(src []byte, res types.Dict, inherited []frame, chain map[int]bool, depth int, ts textState) {
	// start is the text state the stream was entered with: a tiling pattern this stream selects inherits THAT, not
	// the state at the `scn` (`GFPDTilingPattern` takes the invoking stream's inherited graphics state).
	start := ts
	// Charged BEFORE the tokenizer runs, since tokenizing is the cost: a stream of whitespace is no operator.
	if w.d.contentBytes += len(src); w.d.overBudget() {
		return
	}
	// **Not copied** (R3-2): the copy was O(depth) per nested stream, so a deep nesting that draws a form at every
	// level paid for the whole stack each time. Pushes land above `inherited` and pops stop at it (the `EMC`
	// guard), so the caller's frames are never touched — see `pushFrame`.
	stack := inherited
	gs := []textState{}
	var operands []contentstream.Token
	opIndex := 0
	for _, tk := range contentstream.Tokenize(src) {
		switch tk.Kind {
		case contentstream.Whitespace:
			continue
		case contentstream.InlineImage:
			opIndex++
			w.d.contentOps++
			// **Spending a unit and never consulting the ceiling is the same defect one step along.** A page
			// whose stream is nothing but `BI … EI` grew the events past `maxContentEvents` with `contentErr`
			// never set inside that stream, because `overBudget` was reached only from the operator branch.
			if w.d.overBudget() {
				return
			}
			w.emit(stack, false, fmt.Sprintf("%s, operator #%d `BI … EI` (inline image)", w.where, opIndex))
			operands = operands[:0]
			continue
		case contentstream.Operator:
		default:
			operands = append(operands, tk)
			continue
		}
		opIndex++
		w.d.contentOps++
		if w.d.overBudget() {
			return
		}
		op := string(tk.Bytes(src))
		switch op {
		case "BMC", "BDC":
			stack = w.d.pushFrame(stack, w.openSequence(src, operands, res, opIndex, op))
		case "EMC":
			// A sequence becomes a subject only where it CLOSES, and only inside its own stream — the
			// guard is what keeps an `EMC` in a form from closing the sequence that drew it.
			if len(stack) > len(inherited) {
				if !w.appearance && !w.langOnly {
					w.d.mcSubjects = append(w.d.mcSubjects, w.subject(stack))
				}
				stack = stack[:len(stack)-1]
			}
		case "q":
			gs = append(gs, ts)
		case "Q":
			if n := len(gs); n > 0 {
				ts, gs = gs[n-1], gs[:n-1]
			}
		case "gs":
			// **An ExtGState's `/Font` binds the font as `Tf` does** (`GraphicState.copyPropertiesFormExtGState`):
			// `[font size]`, the font an indirect dictionary. Without it, text after the `gs` was judged in the stale
			// `Tf` font — a false pass the P07.S02 re-review measured.
			// Only a DICTIONARY of a font subtype veraPDF builds rebinds it (`PDExtGState.getFont`, `PDFontFactory`); any
			// other `/Font` leaves the font in force, as a missing one does (the P07.S02 re-review measured both).
			if name := w.firstName(src, operands); len(name) > 1 {
				egs := w.d.dict(w.d.dict(res["ExtGState"])[fontcode.Name([]byte(name[1:]))])
				if arr, err := w.d.Ctx.DereferenceArray(egs["Font"]); err == nil && len(arr) > 0 {
					if f := w.d.dict(arr[0]); f != nil && extGStateFontTypes[w.d.name(f["Subtype"])] {
						ts.fontName = name + " (its ExtGState /Font)"
						ts.font, ts.fontObj = f, 0
						if ir, ok := arr[0].(types.IndirectRef); ok {
							ts.fontObj = ir.ObjectNumber.Value()
						}
					}
				}
			}
		case "Tf":
			if name := w.firstName(src, operands); name != "" {
				ts.fontName = name
				ts.font, ts.fontObj = nil, 0
				if fonts := w.d.dict(res["Font"]); fonts != nil && len(name) > 1 {
					raw := fonts[fontcode.Name([]byte(name[1:]))]
					ts.font = w.d.dict(raw)
					if ir, ok := raw.(types.IndirectRef); ok {
						ts.fontObj = ir.ObjectNumber.Value()
					}
				}
			}
		case "Tr":
			for _, o := range operands {
				if n, err := strconv.Atoi(string(o.Bytes(src))); err == nil {
					ts.renderMode = n
				}
			}
		case "Do":
			name := w.firstName(src, operands)
			w.doXObject(name, res, stack, chain, depth, opIndex, ts)
		case "scn", "SCN":
			// **Selecting a tiling pattern is enough to read it — painting is not required.** Measured on
			// 1.30.2: a bad `/Lang` inside a pattern the page selects and never paints fails 7.2 t29, while
			// the same pattern merely NAMED in `/Resources` is not a subject at all (zero checks).
			w.enterPattern(w.firstName(src, operands), res, chain, depth, opIndex, start)
		default:
			if !textOperators[op] && !paintOperators[op] {
				break
			}
			text := textOperators[op]
			font, fontObj := ts.font, ts.fontObj
			if text {
				// **Showing ANY glyph reads EVERY `CharProc`.** Measured: a document that shows `/b` fails
				// 7.2 t29 on a bad `/Lang` in `/a`'s procedure, while selecting the font with `Tf` and showing
				// nothing reads none of them.
				w.enterType3(font, ts, res, chain, depth, opIndex)
			}
			where := fmt.Sprintf("%s, operator #%d `%s`", w.where, opIndex, op)
			if text {
				// **Glyphs are read in the lang-only streams too**: veraPDF judges the glyphs a tiling pattern or a
				// Type 3 procedure draws like any other (`GFPDTilingPattern`, `GFPDType3Font.getCharProcStreams`).
				w.showGlyphs(src, operands, font, fontObj, ts.fontName, where, ts.renderMode != 3)
				if w.d.contentOver {
					return
				}
				if w.langOnly && font != nil {
					// A pattern or a Type 3 procedure draws this font where no font event is recorded (/pending 678),
					// so its render modes there are unseen — the per-glyph clauses refuse where that decides.
					if w.d.drawnUnrecorded == nil {
						w.d.drawnUnrecorded = map[uintptr]bool{}
					}
					w.d.drawnUnrecorded[dictID(font)] = true
				}
			}
			if w.langOnly {
				break
			}
			ev := w.event(stack, text, where)
			if ev.text {
				ev.fontName = ts.fontName
				ev.invisible = ts.renderMode == 3
				ev.mode = ts.renderMode
				ev.offPage = w.form != 0 || w.appearance
				ev.font, ev.fontObj = font, fontObj
			}
			w.d.content = append(w.d.content, ev)
		}
		operands = operands[:0]
	}
}

// doXObject handles `Do`: an image is drawn content; a form is walked with its own resources.
func (w walker) doXObject(name string, res types.Dict, stack []frame, chain map[int]bool, depth, opIndex int, ts textState) {
	where := fmt.Sprintf("%s, operator #%d `%s Do`", w.where, opIndex, name)
	// **An XObject nib cannot reach is a form it may not have graded.** Since P06.S03 the drawing walk
	// is also `7.20 t1`'s population, so silently skipping one turns a missing subject into a Pass —
	// the refusal below is what keeps "nib did not read it" from reading as "there is none".
	xobjs := w.d.dict(res["XObject"])
	if xobjs == nil && res["XObject"] != nil && w.d.contentErr == "" {
		w.d.contentErr = fmt.Sprintf("%s: /Resources /XObject is not a dictionary nib can read, so "+
			"what it names was never walked", where)
	}
	if xobjs == nil || len(name) < 2 {
		w.emit(stack, false, where+" (unresolvable XObject)")
		return
	}
	raw := xobjs[name[1:]]
	sd, _, err := w.d.Ctx.DereferenceStreamDict(raw)
	if err != nil {
		// Separated from `sd == nil` the way `walkAppearances` separates them (`/pending 507`): an
		// entry that ERRORS is one nib could not read, and it may have been a form carrying /Ref.
		//
		// **A DECLARED unreached branch, measured.** pdfcpu's validator refuses a non-stream
		// `/XObject` entry ahead of the rules — `DereferenceStreamDict: wrong type <(9 0 R)>
		// types.Dict` — so a document that would reach this never opens, and probing the refusal away
		// leaves the package green. veraPDF FAILS such a document, so nib emitting no report at all is
		// a real divergence, of the same declared class as `6.1 t1`'s `%PDF-1.9`. The refusal is kept
		// because the walk is now a POPULATION as well as a reader: the day this becomes reachable,
		// the alternative is a subject dropped in silence.
		if w.d.contentErr == "" {
			w.d.contentErr = fmt.Sprintf("%s: the XObject %s could not be read, so whether it is a "+
				"form and what it draws were never established: %v", where, name, err)
		}
		w.emit(stack, false, where+" (unresolvable XObject)")
		return
	}
	if sd == nil {
		// Resolves to null: there is no XObject there to walk, which is not a failure to read one.
		w.emit(stack, false, where+" (unresolvable XObject)")
		return
	}
	if w.d.name(sd.Dict["Subtype"]) != "Form" {
		w.emit(stack, false, where+" (image)")
		return
	}
	objNr := 0
	if ir, ok := raw.(types.IndirectRef); ok {
		objNr = ir.ObjectNumber.Value()
	}
	// Tallied BEFORE the self-draw stop: veraPDF builds a form at every `Do` in a traversed stream,
	// including one naming the form being traversed — measured, a form that draws itself FAILS
	// `7.20 t2`, its own `Do` being the second reach. It is also ahead of the depth and budget stops,
	// which moves `7.20 t1` too: a `/Ref` form reached past either is now FAILED where it was refused.
	// That is veraPDF's answer — it has no depth cap, so it builds the form — and a definite failure on
	// the form in hand beats a refusal about what lies below it.
	w.d.recordDrawnForm(sd.Dict, objNr, where, !w.repeat)
	if w.form != 0 {
		if w.d.drawsForms == nil {
			w.d.drawsForms = map[int]bool{}
		}
		w.d.drawsForms[w.form] = true
	}
	// **Object 0 is no object** (R3-10): a form with no object number is not one the chain can recognise, so
	// it is neither looked up nor added below — otherwise the first key-less form would mark every later
	// key-less form as "drawing itself" and they would go unwalked in silence. Unreachable today (a stream is
	// always an indirect object, so `raw` is always a reference) and kept correct rather than merely unreached.
	if objNr != 0 && chain[objNr] {
		// A form drawing itself: its content is already being walked once up the chain, so nothing is unread.
		return
	}
	if depth+1 > maxFormDepth {
		if w.d.contentErr == "" {
			w.d.contentErr = fmt.Sprintf("form XObjects nest deeper than %d levels (%s); nib stops walking there, "+
				"so what the deeper forms draw was never read", maxFormDepth, where)
		}
		return
	}
	if w.d.formWalks++; w.d.overBudget() {
		return
	}
	src, derr := w.d.decodedContent(sd, objNr)
	if derr != nil {
		w.d.contentErr = fmt.Sprintf("form XObject %s (object %d) could not be decoded: %v", name, objNr, derr)
		return
	}
	formRes := w.d.dict(sd.Dict["Resources"])
	if formRes == nil {
		formRes = res
	}
	// `langOnly` travels INTO the form: veraPDF's semantic branch requires the invoking stream to be
	// semantic (`GFPDXForm.java:205-211`), so a form drawn from a tiling pattern or a glyph procedure is a
	// plain content stream too, and nothing it draws is a content item or a marked-content subject.
	inner := walker{d: w.d, where: fmt.Sprintf("%s → form XObject %s (object %d)", w.where, name, objNr), spKey: w.spKey, appearance: w.appearance, stream: w.d.nextStream(), langOnly: w.langOnly,
		repeat: w.d.retraversal(objNr, w.repeat), form: objNr}
	if sp, ok := w.d.intValue(sd.Dict["StructParents"]); ok {
		inner.spKey = sp
	}
	next := withLink(chain, objNr)
	inner.walkWithState(src, formRes, stack, next, depth+1, ts)
}

// recordLang counts a BDC property list's string `/Lang` for 7.2 t29 (`checkLanguageIdentifiers`) and keeps the
// first that fails the grammar — only that one: keeping every value held 1.6 GB for a 63 KB file of repeated forms
// (the slice review, measured), and the rule needs one failure and a count. A DP's is not
// kept: measured, veraPDF 1.30.2 passes 7.2-29 on `/Span << /Lang (en_US) >> DP` although its source lists DP as
// marked content — the oracle's answer, not its source's, is the one nib agrees with.
func (w walker) recordLang(value string, ok bool, opIndex int, op string) {
	if !ok {
		return
	}
	w.d.mcLangCount++
	if w.d.mcLangBad == nil && !languageTag.MatchString(value) {
		w.d.mcLangBad = &mcLang{value: value, where: fmt.Sprintf("%s, operator #%d `%s`", w.where, opIndex, op)}
	}
}

// openSequence builds the frame one `BMC` or `BDC` opens.
func (w walker) openSequence(src []byte, operands []contentstream.Token, res types.Dict, opIndex int, op string) frame {
	tag, props := w.d.markedContent(src, operands, res, op)
	f := frame{
		tag:    tag,
		mcid:   -1,
		spKey:  -1,
		stream: w.stream,
		where:  fmt.Sprintf("%s, operator #%d `%s`", w.where, opIndex, op),
	}
	f.artifact = tag == "Artifact"
	lang, hasLang := props.text("Lang")
	w.recordLang(lang, hasLang, opIndex, op)
	f.lang = hasLang
	_, f.actualText = props.text("ActualText")
	_, f.alt = props.text("Alt")
	_, f.expansion = props.text("E")
	if m, ok := props.integer("MCID"); ok {
		f.mcid, f.spKey = m, w.spKey
		f.elem, f.elemUnread = w.d.elementForMCID(w.spKey, m)
	}
	return f
}

// subject is one closed marked-content sequence as the five rules read it. The stack's LAST frame is the
// one that just closed; everything below it is what encloses it.
func (w walker) subject(stack []frame) mcSubject {
	f := stack[len(stack)-1]
	s := mcSubject{
		where:      f.where,
		tag:        f.tag,
		actualText: f.actualText,
		alt:        f.alt,
		expansion:  f.expansion,
		ownLang:    f.lang,
	}
	// `parentsTags` includes the object's OWN tag (`GFOpMarkedContent.java:142-151`) and crosses a form
	// XObject boundary into the invoking stream, so 7.1 t2 fires on an `/Artifact` itself and on anything a
	// form draws inside one. Measured in both directions. (`anyArtifact` is that scan, carried — `pushFrame`.)
	s.insideArtifact = f.anyArtifact
	s.tagged, s.taggedUnread = w.d.taggedContent(stack)
	s.inheritedLang, s.langUnread = w.d.inheritedLangOf(stack, w.stream, false)
	return s
}

// taggedContent is veraPDF's `isTaggedContent` (`GFSEGroupedContent.java:145-163`), and it is NOT
// "an MCID is present".
//
// It takes the innermost struct parent in force — the sequence's own `/MCID` resolved through the parent
// tree, else the one it inherits from the sequence around it, which DOES cross a form XObject boundary —
// and asks whether that element's `/P` chain reaches the structure tree root. **An MCID naming a slot the
// parent tree does not hold, and an element detached from the root, are both untagged**: measured, veraPDF
// fails 7.1 t3 on both and nib passed them, which is a false pass in the sense `Verdict.conformant` means.
//
// The second result is why nib could not settle it, and it is `CannotCheck` — never a Pass.
//
// The innermost struct parent is the top frame's `structElem`/`structUnread`, which `pushFrame` carried up from
// every frame below — the scan from the top for the first frame with `elem` or `elemUnread` set, done once per
// push instead of once per question (R3-2).
func (d *Document) taggedContent(stack []frame) (bool, string) {
	if len(stack) == 0 {
		return false, ""
	}
	top := &stack[len(stack)-1]
	if top.structElem != nil {
		return d.reachesStructTreeRoot(top.structElem)
	}
	return false, top.structUnread
}

// reachesStructTreeRoot climbs an element's `/P` chain asking whether it arrives at the structure tree root.
//
// **The bound and the loop guard are `parentLang`'s, for `parentLang`'s reasons** — a sideways `/P` chain is
// not the tree and is unbounded in the input, and a cycle is a complete answer rather than a refusal: every
// ancestor was seen and none was the root.
//
// **And the ARITHMETIC is `parentLang`'s too, which it was not.** This refused one ancestor later, so an element
// whose `/P` chain reaches the StructTreeRoot at link 66 read as tagged content here while 7.2 t34 answered
// CannotCheck over the same element in the same walk — two readers of one chain disagreeing by one, under a
// comment claiming they were identical. Found at the P04 close; both climbs now refuse at the same link.
func (d *Document) reachesStructTreeRoot(elem types.Dict) (bool, string) {
	// **Memoised for `parentLang`'s reason too**: `taggedContent` asks once per drawing operator AND once per
	// closed sequence, so a deep tree under a page of text repeats one 65-link climb ten thousand times. A
	// refusal is NOT memoised — it depends on where the climb started, which is the same rule `parentLang`
	// keeps for the same reason.
	if d.rootReach == nil {
		d.rootReach = map[uintptr]bool{}
	}
	if answer, ok := d.rootReach[dictID(elem)]; ok {
		return answer, ""
	}
	seen := map[uintptr]bool{dictID(elem): true}
	for p, n := d.dict(elem["P"]), 0; p != nil; p, n = d.dict(p["P"]), n+1 {
		if d.name(p["Type"]) == "StructTreeRoot" {
			d.rootReach[dictID(elem)] = true
			return true, ""
		}
		if id := dictID(p); seen[id] {
			d.rootReach[dictID(elem)] = false
			return false, ""
		} else if n+1 > maxLangClimb {
			return false, fmt.Sprintf("the element describing this content climbs through more than %d /P links "+
				"without reaching the structure tree root; nib stops climbing there, so whether it is tagged "+
				"content was never settled", maxLangClimb)
		} else {
			seen[id] = true
		}
	}
	d.rootReach[dictID(elem)] = false
	return false, ""
}

// inheritedLangOf is veraPDF's `inheritedLang` for the sequence that just closed
// (`GFOpMarkedContent.java:153-166`), in the order it stops at the first hit: the sequence's own
// `/MCID`-resolved element, its own `/Lang` or an ancestor's; else the ENCLOSING sequence's own property-list
// `/Lang`; else that sequence's own inherited language, recursively.
//
// **It stops at the stream boundary**, which `parentsTags` and the struct parent do not: veraPDF's chain is a
// per-parser stack (`OperatorParser.java:168,176`), and measured, a `/Lang` in force on the page does not
// reach a Span inside a form the page draws — in both the marked-content and the structure-element spelling.
//
// **The element's own `/Lang` counts and the climb is `parentLang`'s blind one**, measured: a `/Lang` on the
// StructTreeRoot, on a non-ancestor dictionary named by `/P`, and an EMPTY `()` on the element all satisfy
// the rule. `declaresLangFor` answered differently at exactly those points and was 7.2 t34's second
// implementation of this question (`/pending 635`); it is gone, and t34 reads this.
// **`stream` is the WALKER's, never `stack[last].stream`.** A form XObject whose own content opens no
// sequence has a stack of INHERITED frames only, every one carrying the invoking stream's number — so
// comparing against the last frame would walk the invoker's frames and inherit exactly what the boundary
// forbids. Measured: text in such a form, drawn from inside a sequence the page gave a `/Lang`, FAILS 7.2 t34.
//
// **`ownCounts` is the difference between a sequence asking and a content ITEM asking.** A sequence's own
// `/Lang` is not part of its inherited language — the rules carry a separate `Lang != null` disjunct for it —
// while a content item has no property list of its own, so the sequence around it counts in full.
//
// **It is O(1)** (R3-2): the walk down the stack — stop at the first own `/Lang`, element language or refusal,
// and at the first frame from another stream — is carried per frame as `langIn`, and only the top frame's
// `ownCounts` exception is decided here.
func (d *Document) inheritedLangOf(stack []frame, stream int, ownCounts bool) (bool, string) {
	last := len(stack) - 1
	if last < 0 || stack[last].stream != stream {
		return false, ""
	}
	top := &stack[last]
	if ownCounts {
		return top.langIn, top.langWhy
	}
	if top.ownDecided {
		return top.ownLangFound, top.ownLangWhy
	}
	if last > 0 && stack[last-1].stream == stream {
		return stack[last-1].langIn, stack[last-1].langWhy
	}
	return false, ""
}

// emit records one drawing operator — and is the ONE door that does, apart from the text/paint branch that
// needs to decorate its event with the font first.
//
// **`langOnly` is honoured here and not at each append site**, because it was not: the check lived in the
// text-and-paint branch alone, so an inline image, an image XObject and an unresolvable one all reached the
// content events from inside a tiling pattern or a Type 3 glyph procedure. An inline image is the canonical
// Type 3 bitmap glyph, and `enterLangOnly` passes an empty stack, so such a glyph was UNCOVERED content on a
// fully tagged page — a false FAIL of 7.1 t3 on an ordinary document, and it falsified this file's own claim
// that nothing in those streams is a content item.
func (w walker) emit(stack []frame, text bool, where string) {
	if w.langOnly {
		return
	}
	w.d.content = append(w.d.content, w.event(stack, text, where))
}

// event builds one event, deriving its coverage from the open sequences.
func (w walker) event(stack []frame, text bool, where string) contentEvent {
	ev := contentEvent{where: where, text: text, appearance: w.appearance, mcid: -1, spKey: -1}
	// Read off the top frame's aggregates (`pushFrame`, R3-2): the innermost MCID in THIS stream, and whether any
	// enclosing sequence, in any stream, is an `/Artifact`.
	artifact := false
	if n := len(stack); n > 0 {
		top := &stack[n-1]
		artifact = top.anyArtifact
		if top.stream == w.stream {
			ev.mcid, ev.spKey = top.mcidIn, top.spKeyIn
		}
	}
	// 7.1 t3 is `isTaggedContent == true || parentsTags.contains('Artifact')`, so an enclosing `/Artifact`
	// settles coverage without the parent tree being read at all. It does NOT settle the language: measured,
	// text inside an artifact still needs one, and still inherits it from the sequence around the artifact.
	if artifact {
		ev.covered = true
	} else {
		ev.covered, ev.coverUnread = w.d.taggedContent(stack)
	}
	if text {
		ev.langDetermined, ev.langUnread = w.d.inheritedLangOf(stack, w.stream, true)
	}
	return ev
}

// enterPattern walks a tiling pattern `scn`/`SCN` selects, for its `/Lang` values and nothing else.
func (w walker) enterPattern(name string, res types.Dict, chain map[int]bool, depth, opIndex int, ts textState) {
	if len(name) < 2 {
		return
	}
	raw := w.d.dict(res["Pattern"])[name[1:]]
	sd, _, err := w.d.Ctx.DereferenceStreamDict(raw)
	if err != nil && raw != nil {
		// **A SHADING pattern is a dictionary, not a stream**, and `DereferenceStreamDict` answers "wrong type"
		// for it — it has no content to walk, so it is skipped, not refused. The P06.S05 re-review measured the
		// refusal below firing here: nine content clauses CannotCheck on an ordinary gradient fill.
		// `/PatternType 2` is asked here rather than trusted to pdfcpu's validator, which enforces it today
		// (`validate/pattern.go`) — a tiling pattern written as a plain dictionary must still refuse.
		if pd, derr := w.d.Ctx.DereferenceDict(raw); derr == nil && pd != nil {
			if pt, ok := w.d.intValue(pd["PatternType"]); ok && pt == 2 {
				return
			}
		}
	}
	if err != nil || sd == nil {
		// **A pattern name that resolves to nothing is a pattern nib did not read, never one that draws
		// nothing** (P06.S05). pdfcpu's reader DROPS a page's `/Pattern` resource that the page's own
		// content never selects — measured: a form with no `/Resources` inheriting the page's pattern loses
		// it — so the name reaches here unbound although the file binds it. Since P06.S05 a pattern's `Do`s
		// are `7.20 t2`'s population, and returning quietly was a live false PASS: veraPDF FAILS a document
		// whose inherited pattern draws a keyed form twice, and nib passed it. `doXObject` already refuses
		// the same drop on the XObject route.
		if w.d.contentErr == "" {
			w.d.contentErr = fmt.Sprintf("%s, operator #%d selects the pattern %s, which nib's reader could not "+
				"resolve, so what that pattern draws was never read", w.where, opIndex, name)
		}
		return
	}
	if pt, ok := w.d.intValue(sd.Dict["PatternType"]); !ok || pt != 1 {
		return // a shading pattern has no content stream to read
	}
	objNr := 0
	if ir, ok := raw.(types.IndirectRef); ok {
		objNr = ir.ObjectNumber.Value()
	}
	w.enterLangOnly(sd, objNr, res, fmt.Sprintf("%s, operator #%d → tiling pattern %s", w.where, opIndex, name),
		chain, depth, ts)
}

// enterType3 walks EVERY `CharProc` of a Type 3 font a text operator shows a glyph in.
//
// A font entry that is nil is not "no font": pdfcpu's validator DROPS a Type 3 font dictionary written
// directly inside another dictionary, and what it dropped may have held glyphs with marked content. The raw
// file is asked once whether the document writes one (`hasInlineType3Font`), and that is `contentErr`.
func (w walker) enterType3(font types.Dict, ts textState, res types.Dict, chain map[int]bool, depth, opIndex int) {
	fontName := ts.fontName
	if font == nil {
		if len(fontName) > 1 && w.d.dict(res["Font"]) != nil && w.d.hasInlineType3Font() {
			if w.d.contentErr == "" {
				w.d.contentErr = fmt.Sprintf("%s, operator #%d shows text in font %s, which is missing from what nib's "+
					"reader kept while the file writes a Type 3 font directly inside a dictionary — the shape pdfcpu's "+
					"validator drops — so that font's glyph procedures were never read", w.where, opIndex, fontName)
			}
		}
		return
	}
	if w.d.name(font["Subtype"]) != "Type3" {
		return
	}
	fontRes := w.d.dict(font["Resources"])
	if fontRes == nil {
		fontRes = res
	}
	procs := w.d.dict(font["CharProcs"])
	if len(procs) == 0 {
		return
	}
	// **Once per `/CharProcs` dictionary, decided BEFORE the per-glyph work** (the P07 phase-close review, R3-1).
	// `langWalked` below made each procedure's WALK once-only, but this loop ran on every text-showing operator
	// ahead of it — sorting every key, dereferencing every procedure and formatting a label — so N shows of a font
	// with N glyphs cost N² with no budget charged: 20,000 of each took 4m30s. Once every procedure has been
	// handed to `enterLangOnly`, a second enumeration can only find them walked already, so it is skipped.
	//
	// **Not memoised when a procedure was skipped for being on the chain** — the font shown inside its own glyph
	// procedure — because that procedure is still unread by this enumeration and a later show must reach it, as
	// it always did. Such re-enumerations, and any other that meets a procedure already walked, are charged to
	// `maxCharProcAsks`, so the recursive shape is bounded too and answers CannotCheck past the ceiling.
	id := dictID(procs)
	if w.d.charProcsRead[id] {
		return
	}
	complete := true
	for _, glyph := range sortedKeys(procs) {
		sd, _, err := w.d.Ctx.DereferenceStreamDict(procs[glyph])
		if err != nil || sd == nil {
			if w.d.contentErr == "" {
				w.d.contentErr = fmt.Sprintf("%s, Type 3 font %s glyph /%s does not resolve to a stream, so its marked "+
					"content was never read", w.where, fontName, glyph)
			}
			continue
		}
		objNr := 0
		if ir, ok := procs[glyph].(types.IndirectRef); ok {
			objNr = ir.ObjectNumber.Value()
		}
		onChain := objNr != 0 && chain[objNr]
		if onChain {
			complete = false
		}
		if onChain || w.d.langWalked[dictID(sd.Dict)] {
			// `enterLangOnly` would return at once: charge the ask, and do not format a label nobody reads.
			if w.d.charProcAsks++; w.d.overBudget() {
				return
			}
			continue
		}
		// A glyph procedure is parsed in the state of the operator that showed the Type 3 font (`GFPDType3Font`, which
		// FontFactory hands that graphics state) — whose font IS this font, so text a procedure shows without a `Tf`
		// is judged in it.
		w.enterLangOnly(sd, objNr, fontRes, fmt.Sprintf("%s → Type 3 font %s, glyph /%s", w.where, fontName, glyph),
			chain, depth, ts)
		if w.d.contentOver {
			return
		}
	}
	if complete {
		if w.d.charProcsRead == nil {
			w.d.charProcsRead = map[uintptr]bool{}
		}
		w.d.charProcsRead[id] = true
	}
}

// withLink is chain with objNr added — a fresh map, since the caller's chain must not change. Object 0 is no
// object and is never added (R3-10).
func withLink(chain map[int]bool, objNr int) map[int]bool {
	next := make(map[int]bool, len(chain)+1)
	for k := range chain {
		next[k] = true
	}
	if objNr != 0 {
		next[objNr] = true
	}
	return next
}

// enterLangOnly decodes one nested stream and walks it in lang-only mode, under the walk's own budgets.
//
// **The enclosing marked-content stack is NOT passed in.** veraPDF builds a plain content stream for a
// pattern and a glyph procedure, so nothing in them is a marked-content subject or a content item at all;
// the only thing this walk contributes is 7.2 t29's `/Lang` values.
func (w walker) enterLangOnly(sd *types.StreamDict, objNr int, res types.Dict, label string, chain map[int]bool, depth int, ts textState) {
	if chain[objNr] && objNr != 0 {
		return
	}
	// **Once per stream, not once per use.** `enterType3` fires on every text-showing operator and
	// `enterPattern` on every `scn`, so a page showing ten thousand glyphs in one Type 3 font would walk that
	// font's every `CharProc` ten thousand times — and each entry spends a `formWalks` unit, so an ordinary
	// document would trip `maxFormWalks` and turn EVERY content rule into CannotCheck. veraPDF reads a pattern
	// and a font once, as objects, so repeating is not faithful either. Keyed by the stream dictionary's
	// identity rather than its object number, because a pattern or glyph procedure written inline has none.
	//
	// **The FIRST entry's font is the one its glyphs are judged in** (P07.S02): text a pattern or glyph procedure shows
	// without its own `Tf` inherits the font it was entered with, and veraPDF reads the object once — a pattern entered
	// under two fonts is judged under the first only (measured: walking it again under the second failed a document
	// veraPDF passes).
	if w.d.langWalked == nil {
		w.d.langWalked = map[uintptr]bool{}
	}
	if id := dictID(sd.Dict); w.d.langWalked[id] {
		return
	} else {
		w.d.langWalked[id] = true
	}
	if depth+1 > maxFormDepth {
		if w.d.contentErr == "" {
			w.d.contentErr = fmt.Sprintf("%s nests deeper than %d levels; nib stops walking there, so what lies "+
				"below was never read", label, maxFormDepth)
		}
		return
	}
	if w.d.formWalks++; w.d.overBudget() {
		return
	}
	src, err := w.d.decodedContent(sd, objNr)
	if err != nil {
		if w.d.contentErr == "" {
			w.d.contentErr = fmt.Sprintf("%s could not be decoded, so its marked content was never read: %v", label, err)
		}
		return
	}
	// Once per stream already (`langWalked` above), which is veraPDF's own once-per-key for a pattern
	// and a glyph procedure: measured, a pattern used twice PASSES `7.20 t2` and one whose content draws
	// the form twice FAILS. So the stream inherits only whether its invoker was traversed.
	inner := walker{d: w.d, where: label, spKey: -1, appearance: w.appearance, stream: w.d.nextStream(), langOnly: true, repeat: w.repeat, form: w.form}
	next := withLink(chain, objNr)
	streamRes := w.d.dict(sd.Dict["Resources"])
	if streamRes == nil {
		streamRes = res
	}
	inner.walkWithState(src, streamRes, nil, next, depth+1, ts)
}

// markedContent reads a `BMC`/`BDC`'s tag and property list the way veraPDF locates them.
//
// **The tag's position differs between the two operators**: `BDC` takes `arguments[size-2]`
// (`GFOpMarkedContent.java:108-116`) and `BMC` the LAST argument (`GFOp_BMC.java:57-65`). So a malformed
// `/Artifact BDC` written with no property list has NO tag and breaks no rule — measured on 1.30.2, and
// three of this slice's first-round fixtures were that shape and measured the wrong thing.
//
// The property list is an inline dictionary or a name resolved through `/Resources /Properties`
// (`GFOpMarkedContent.java:68-84`); `BMC` never has one, so `/Lang`, `/ActualText`, `/Alt` and `/E` are all
// absent for it and 7.2 t30-t32 cannot fire on one.
func (d *Document) markedContent(src []byte, operands []contentstream.Token, res types.Dict, op string) (string, propertyList) {
	names := []int{}
	dictAt := -1
	for i, tk := range operands {
		switch {
		case tk.Kind == contentstream.DictOpen && dictAt < 0:
			dictAt = i
		case tk.Kind == contentstream.Operand && dictAt < 0:
			if b := tk.Bytes(src); len(b) > 0 && b[0] == '/' {
				names = append(names, i)
			}
		}
	}
	if op == "BMC" {
		// BMC takes its last argument, and it has no property list.
		if len(names) == 0 {
			return "", propertyList{}
		}
		return string(operands[names[len(names)-1]].Bytes(src)[1:]), propertyList{}
	}
	if dictAt >= 0 {
		if len(names) == 0 {
			return "", propertyList{}
		}
		return string(operands[names[len(names)-1]].Bytes(src)[1:]), propertyList{d: d, src: src, inline: operands[dictAt:]}
	}
	// `/Tag /MC0 BDC`: the tag is the argument before the property name, so one name alone is no tag.
	if len(names) < 2 {
		return "", propertyList{}
	}
	named := string(operands[names[len(names)-1]].Bytes(src)[1:])
	return string(operands[names[len(names)-2]].Bytes(src)[1:]), propertyList{dict: d.dict(d.dict(res["Properties"])[named]), d: d}
}

// propertyList is a marked-content property list read from one of its two spellings.
//
// The inline form holds TOKENS, because a content stream's dictionary is never parsed into objects — and it
// cannot hold an indirect reference either, so a value is whatever its own bytes say.
type propertyList struct {
	dict   types.Dict
	d      *Document
	src    []byte
	inline []contentstream.Token
}

// text is a key's value when it is a STRING, which is what veraPDF's `getAttribute(…, COS_STRING)` requires
// of every one of `/Lang`, `/ActualText`, `/Alt` and `/E` — a name is no answer.
func (p propertyList) text(key string) (string, bool) {
	if p.dict != nil {
		return p.d.text(p.dict[key])
	}
	tk, ok := p.at(key)
	if !ok {
		return "", false
	}
	b := tk.Bytes(p.src)
	switch {
	case tk.Kind == contentstream.LiteralString && len(b) >= 2:
		return p.d.text(types.StringLiteral(b[1 : len(b)-1]))
	case tk.Kind == contentstream.HexString && len(b) >= 2:
		return p.d.text(types.HexLiteral(b[1 : len(b)-1]))
	}
	return "", false
}

// integer is a key's value when it is an integer — `/MCID` is the only one asked for.
func (p propertyList) integer(key string) (int, bool) {
	if p.dict != nil {
		return p.d.intValue(p.dict[key])
	}
	tk, ok := p.at(key)
	if !ok || tk.Kind != contentstream.Operand {
		return 0, false
	}
	n, err := strconv.Atoi(string(tk.Bytes(p.src)))
	return n, err == nil
}

// at is the token following a key at the inline dictionary's TOP level — a nested dictionary's keys are not
// this property list's, and an `/Artifact << /BBox [ … ] >>`'s array must not swallow the key after it.
func (p propertyList) at(key string) (contentstream.Token, bool) {
	depth := 0
	want := "/" + key
	for i, tk := range p.inline {
		switch tk.Kind {
		case contentstream.DictOpen, contentstream.ArrayOpen:
			depth++
			continue
		case contentstream.DictClose, contentstream.ArrayClose:
			depth--
			if depth == 0 {
				return contentstream.Token{}, false
			}
			continue
		}
		if depth == 1 && tk.Kind == contentstream.Operand && string(tk.Bytes(p.src)) == want && i+1 < len(p.inline) {
			return p.inline[i+1], true
		}
	}
	return contentstream.Token{}, false
}

// firstName returns the first name operand, or "".
func (w walker) firstName(src []byte, operands []contentstream.Token) string {
	for _, tk := range operands {
		if tk.Kind == contentstream.Operand {
			if b := tk.Bytes(src); len(b) > 0 && b[0] == '/' {
				return string(b)
			}
		}
	}
	return ""
}

// extGStateFontTypes are the subtypes veraPDF's `PDFontFactory` builds a font for; any other yields null.
var extGStateFontTypes = map[string]bool{"Type1": true, "MMType1": true, "TrueType": true, "Type3": true, "Type0": true}
