package pdfops

import (
	"bytes"
	"fmt"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/contentstream"
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
	// **Text AND the non-text drawings** — `/pending 514`, which is `/pending 495`'s declared residue.
	// 495 scoped this to text because refusing on an uncovered image would have switched the OCR door
	// off rather than fixed it. 514 measured what refusing actually buys: nothing. veraPDF fails the
	// bare scan on 7.1 t3 too, so the fallback the doors return is the SAME failure with less text in
	// it. So the doors mark their uncovered drawings instead (`uncoveredDrawingSpans`), and the rule
	// now reaches every drawing operator, not only the ones that draw glyphs.
	if !before.claimsHonestly() {
		if n, rerr := unmarkedTextRuns(pdf); rerr != nil || n > 0 {
			return pdf, false, nil
		}
		if n, rerr := uncoveredDrawings(pdf); rerr != nil || n > 0 {
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

// pathConstruction, pathPainting — ISO 32000-1 tables 59 and 60. A path object runs from its first
// construction operator to the painting operator that ends it; `n` ends one without painting, so there is
// nothing drawn to mark.
var (
	pathConstruction = map[string]bool{"m": true, "l": true, "c": true, "v": true, "y": true, "h": true, "re": true, "W": true, "W*": true}
	pathPainting     = map[string]bool{"S": true, "s": true, "f": true, "F": true, "f*": true, "B": true, "B*": true, "b": true, "b*": true}
)

// uncoveredDrawingSpans returns the byte span of every NON-TEXT drawing in one content stream that no
// marked-content sequence already covers as an artifact or with an MCID — a painted path, a shading,
// an inline image, and an image XObject named in images. The span starts at the first operand of the
// drawing's first operator, so a bracket encloses the whole object and never splits a path (a
// marked-content operator inside a path object is illegal).
//
// # Why images are in here and were not before
//
// `/pending 495` wrote this function for PATHS only, and said why: *"An image or a form XObject no
// element covers is the residue /pending 495 names: an artifact would tell a reader a picture is
// decoration, and nothing proposes a /Figure for it."* `/pending 514` settled that with measurement
// rather than taste, and the two that decided it are:
//
//   - **A `/Figure` nobody fills does not fix the failure, it renames it.** veraPDF's 7.3 t1 passes an
//     element only where `Alt` is present and not the empty string, or `ActualText` is present at all;
//     its own corpus file `7.3-t01-fail-a.pdf` is a `/Figure` with neither key and it FAILS. An EMPTY
//     `/Alt` fails too (`7.3-t01-fail-b.pdf`). So proposing a Figure for a picture nobody describes
//     trades a 7.1 t3 failure for a 7.3 t1 one. (The rule's exact text is in `claimdrawing_test.go`,
//     where no doc-comment formatter rewrites its quotes.)
//   - **Refusing the claim fixes nothing.** Measured on the OCR door's own input: the bare scan fails
//     7.1 t3 exactly as the tagged one did, because the unmarked image is there either way. Refusing
//     removes a false claim and costs the text layer, and leaves the clause failing.
//
// `/Artifact` is what is left, and it is what veraPDF's own corpus does: over its 297 PDF/UA-1 files,
// 39 images sit inside an `/Artifact` sequence and 16 inside a marked one, and exactly ONE image in
// the whole corpus is inside neither — `7.1-t03-fail-a.pdf`, the file written to fail this clause.
//
// # What it does NOT return, each because bracketing it here would describe something else
//
//   - **A form XObject's `Do`.** Its content is another stream, which may be drawn more than once;
//     `structartifact.go` refuses to rewrite inside a form for the same reason. It is returned as a
//     `drawnForm` instead, so `uncoveredDrawings` can COUNT what the form draws and the claim door
//     refuses rather than claiming over it — the shape `tagcommit` already holds for text drawn
//     inside a form (`errCommitInForm`).
//   - **An XObject name that does not resolve to an image.** A resource dictionary nib cannot read is
//     not a page full of pictures, and counting it would switch a door off over a document the door
//     handles. This is a declared gap: `uacheck` recurses further and `nib ua` still reports it.
func uncoveredDrawingSpans(src []byte, images map[string]bool) ([]opSpan, []drawnForm) {
	var (
		out          []opSpan
		forms        []drawnForm
		covered      []bool // one per open marked-content sequence
		operandStart = -1
		pathStart    = -1
		startCovered bool
	)
	anyCovered := func() bool {
		for _, c := range covered {
			if c {
				return true
			}
		}
		return false
	}
	toks := contentstream.Tokenize(src)
	for i, tk := range toks {
		if tk.Kind == contentstream.Whitespace {
			continue
		}
		// An inline image is one token and carries its own operands, so there is no operand start to
		// reach back for: the token IS the drawing.
		if tk.Kind == contentstream.InlineImage {
			if !anyCovered() {
				out = append(out, opSpan{tk.Start, tk.End})
			}
			operandStart = -1
			continue
		}
		if tk.Kind != contentstream.Operator {
			if operandStart < 0 {
				operandStart = tk.Start
			}
			continue
		}
		op := string(tk.Bytes(src))
		start := tk.Start
		if operandStart >= 0 {
			start = operandStart
		}
		switch {
		case op == "BMC" || op == "BDC":
			c := false
			for j := i - 1; j >= 0 && toks[j].Start >= start; j-- {
				switch string(toks[j].Bytes(src)) {
				case "/Artifact", "/MCID":
					c = true
				}
			}
			covered = append(covered, c)
		case op == "EMC":
			if n := len(covered); n > 0 {
				covered = covered[:n-1]
			}
		case op == "Do":
			switch name := operandName(src, toks, i); {
			case images[name]:
				if !anyCovered() {
					out = append(out, opSpan{start, tk.End})
				}
			case name != "":
				forms = append(forms, drawnForm{name: name, covered: anyCovered()})
			}
		case op == "sh":
			if !anyCovered() {
				out = append(out, opSpan{start, tk.End})
			}
		case pathConstruction[op]:
			if pathStart < 0 {
				pathStart, startCovered = start, anyCovered()
			}
		case pathPainting[op]:
			if pathStart >= 0 && !startCovered && !anyCovered() {
				out = append(out, opSpan{pathStart, tk.End})
			}
			pathStart = -1
		case op == "n":
			pathStart = -1
		}
		operandStart = -1
	}
	return out, forms
}

// drawnForm is one `Do` a stream makes on something that is not an image, and whether a marked-content
// sequence covered that `Do`. A covered one covers everything the form draws, because `mcStack` spans
// the form boundary — see `runWalker.mcStack`.
type drawnForm struct {
	name    string
	covered bool
}

// operandName returns the name operand nearest before toks[i], without its slash, or "" when the
// operator's last operand is not a name.
func operandName(src []byte, toks []contentstream.Token, i int) string {
	for j := i - 1; j >= 0; j-- {
		if toks[j].Kind == contentstream.Whitespace {
			continue
		}
		if b := toks[j].Bytes(src); len(b) > 1 && b[0] == '/' {
			return string(b[1:])
		}
		return ""
	}
	return ""
}

// imageXObjectNames is the set of names in one resource dictionary's `/XObject` that resolve to an
// image. A name that does not resolve, or resolves to a form, is absent — see `uncoveredDrawingSpans`.
func imageXObjectNames(ctx *model.Context, res types.Dict) map[string]bool {
	out := map[string]bool{}
	if res == nil {
		return out
	}
	xobjs, err := ctx.DereferenceDict(res["XObject"])
	if err != nil || xobjs == nil {
		return out
	}
	for name, o := range xobjs {
		sd, _, serr := ctx.DereferenceStreamDict(o)
		if serr != nil || sd == nil {
			continue
		}
		if st := sd.Dict.NameEntry("Subtype"); st != nil && *st == "Image" {
			out[name] = true
		}
	}
	return out
}

// uncoveredDrawings counts the non-text drawings, on every page and in the forms they draw, that no
// marked-content sequence covers — 7.1 t3's other half, where `unmarkedTextRuns` is the glyph half.
//
// **It recurses into form XObjects and the writers do not**, deliberately. A door can bracket only
// the stream it owns, so an image drawn through a form is content this guard sees and no door can
// mark: counting it is what makes the claim refused rather than made and wrong.
//
// A page that cannot be read is an error, for the reason `unmarkedTextRuns` gives.
func uncoveredDrawings(pdf []byte) (int, error) {
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		return 0, err
	}
	n := 0
	for p := 1; p <= ctx.PageCount; p++ {
		d, _, attrs, derr := ctx.PageDict(p, false)
		if derr != nil || d == nil {
			return 0, fmt.Errorf("pdfops: page %d does not resolve: %w", p, derr)
		}
		src, cerr := ctx.PageContent(d, p)
		if cerr == model.ErrNoContent {
			continue
		}
		if cerr != nil {
			return 0, cerr
		}
		var res types.Dict
		if attrs != nil {
			res = attrs.Resources
		}
		n += countDrawings(ctx, src, res, 0, map[int]bool{})
	}
	return n, nil
}

// countDrawings is `uncoveredDrawingSpans` over one stream plus every stream the forms it DRAWS
// contain, recursively.
//
// **Drawn, not merely present in `/Resources`.** A form XObject a resource dictionary names and no
// stream draws puts nothing on the page, so its content owes 7.1 t3 nothing — the shape
// `mcrcarry_test.go` already pins for the run reader.
func countDrawings(ctx *model.Context, src []byte, res types.Dict, depth int, visiting map[int]bool) int {
	here, forms := uncoveredDrawingSpans(src, imageXObjectNames(ctx, res))
	n := len(here)
	if depth >= maxFormDepth || res == nil {
		return n
	}
	xobjs, err := ctx.DereferenceDict(res["XObject"])
	if err != nil || xobjs == nil {
		return n
	}
	for _, f := range forms {
		// A `Do` a sequence covers covers everything the form draws too, because `mcStack` spans the
		// form boundary. The same form drawn again outside one is a separate entry and is counted.
		if f.covered {
			continue
		}
		o, ok := xobjs[f.name]
		if !ok {
			continue
		}
		sd, _, serr := ctx.DereferenceStreamDict(o)
		if serr != nil || sd == nil {
			continue
		}
		if st := sd.Dict.NameEntry("Subtype"); st == nil || *st != "Form" {
			continue
		}
		key := -1
		if ir, isRef := o.(types.IndirectRef); isRef {
			key = ir.ObjectNumber.Value()
			if visiting[key] {
				continue
			}
			visiting[key] = true
		}
		body := streamContent(sd)
		if body != nil {
			var inner types.Dict
			if r, rerr := ctx.DereferenceDict(sd.Dict["Resources"]); rerr == nil {
				inner = r
			}
			n += countDrawings(ctx, body, inner, depth+1, visiting)
		}
		if key >= 0 {
			delete(visiting, key)
		}
	}
	return n
}

// orphanedClaimError is the message a caller uses when it would rather fail than return an untagged
// document — one wording, so two doors cannot describe the same refusal differently.
func orphanedClaimError(door string, pdf []byte) error {
	s := inspectTags(pdf)
	return fmt.Errorf("pdfops: %s produced a document that claims tagging its content does not "+
		"support (%d element(s), %d anchored, %d page(s) with /StructParents) — refusing to return it",
		door, s.elements, s.anchored, s.pagesSP)
}
