package uacheck

import (
	"fmt"
	"strconv"

	"nib/internal/contentstream"

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
	// covered is whether some enclosing marked-content sequence is an artifact or carries an MCID —
	// the two states 7.1 t3 accepts.
	covered  bool
	artifact bool // the nearest relevant enclosing sequence is an /Artifact
	mcid     int  // the innermost MCID in force, or -1
	spKey    int  // the /StructParents key of the stream that owns mcid, or -1
}

// textState is the part of the graphics state the font rules need. `Tf` and `Tr` are graphics
// state, saved by `q` and restored by `Q`, and NOT reset by `BT`/`ET` — both measured against veraPDF:
// a `3 Tr` inside `q … Q` does not survive the `Q`, and one set inside a closed `BT … ET` does survive
// into the next text object.
type textState struct {
	fontName   string
	renderMode int
}

// frame is one open marked-content sequence.
type frame struct {
	artifact bool
	mcid     int
	spKey    int
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
		src, cerr := d.Ctx.PageContent(page, p)
		if cerr == model.ErrNoContent || len(src) == 0 {
			continue
		}
		if cerr != nil {
			d.contentErr = fmt.Sprintf("page %d's content stream could not be read: %v", p, cerr)
			return d.content, d.contentErr
		}
		spKey := -1
		if sp, ok := page["StructParents"].(types.Integer); ok {
			spKey = sp.Value()
		}
		w := walker{d: d, where: fmt.Sprintf("page %d (object %d)", p, objNr), spKey: spKey}
		w.walk(src, d.resourcesOf(page), nil, map[int]bool{}, 0)
	}
	d.walkAppearances()
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
	for p := 1; p <= d.Ctx.PageCount; p++ {
		page, _, _, err := d.Ctx.PageDict(p, false)
		if err != nil || page == nil {
			continue
		}
		annots, _ := d.Ctx.DereferenceArray(page["Annots"])
		for i, a := range annots {
			ad := d.dict(a)
			if ad == nil {
				continue
			}
			ap := d.dict(ad["AP"])
			if ap == nil {
				continue
			}
			for _, key := range []string{"N", "R", "D"} {
				for state, so := range d.appearanceStreams(ap[key]) {
					sd, _, serr := d.Ctx.DereferenceStreamDict(so)
					if serr != nil || sd == nil {
						continue
					}
					if derr := sd.Decode(); derr != nil {
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
					w := walker{d: d, where: label, spKey: -1, appearance: true}
					w.walk(sd.Content, res, nil, map[int]bool{}, 0)
				}
			}
		}
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
}

// walk classifies one content stream's drawing operators, recursing into form XObjects.
//
// `inherited` is the marked-content state in force where a form was invoked: content inside a form
// drawn from within an `/Artifact` or an MCID sequence is covered by that sequence. `chain` holds
// the forms currently being walked, so a form that draws itself stops rather than recursing forever
// — while the SAME form invoked twice from one page is still walked twice, as it is drawn twice.
func (w walker) walk(src []byte, res types.Dict, inherited []frame, chain map[int]bool, depth int) {
	if depth > 8 {
		return
	}
	w.walkWithState(src, res, inherited, chain, depth, textState{})
}

// walkWithState is walk with the text state in force at the point of invocation — a form XObject
// inherits the invoking stream's graphics state (ISO 32000-1 §8.10.1).
func (w walker) walkWithState(src []byte, res types.Dict, inherited []frame, chain map[int]bool, depth int, ts textState) {
	if depth > 8 {
		return
	}
	stack := append([]frame(nil), inherited...)
	gs := []textState{}
	var operands []contentstream.Token
	opIndex := 0
	for _, tk := range contentstream.Tokenize(src) {
		switch tk.Kind {
		case contentstream.Whitespace:
			continue
		case contentstream.InlineImage:
			opIndex++
			w.d.content = append(w.d.content, w.event(stack, false, fmt.Sprintf("%s, operator #%d `BI … EI` (inline image)", w.where, opIndex)))
			operands = operands[:0]
			continue
		case contentstream.Operator:
		default:
			operands = append(operands, tk)
			continue
		}
		opIndex++
		op := string(tk.Bytes(src))
		switch op {
		case "BMC":
			stack = append(stack, frame{artifact: w.firstName(src, operands) == "/Artifact", mcid: -1, spKey: -1})
		case "BDC":
			f := frame{artifact: w.firstName(src, operands) == "/Artifact", mcid: -1, spKey: -1}
			if m, ok := w.mcidOf(src, operands, res); ok {
				f.mcid, f.spKey = m, w.spKey
			}
			stack = append(stack, f)
		case "EMC":
			if len(stack) > len(inherited) {
				stack = stack[:len(stack)-1]
			}
		case "q":
			gs = append(gs, ts)
		case "Q":
			if n := len(gs); n > 0 {
				ts, gs = gs[n-1], gs[:n-1]
			}
		case "Tf":
			if name := w.firstName(src, operands); name != "" {
				ts.fontName = name
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
		default:
			if textOperators[op] || paintOperators[op] {
				ev := w.event(stack, textOperators[op], fmt.Sprintf("%s, operator #%d `%s`", w.where, opIndex, op))
				if ev.text {
					ev.fontName = ts.fontName
					ev.invisible = ts.renderMode == 3
					if fonts := w.d.dict(res["Font"]); fonts != nil && len(ts.fontName) > 1 {
						raw := fonts[ts.fontName[1:]]
						ev.font = w.d.dict(raw)
						if ir, ok := raw.(types.IndirectRef); ok {
							ev.fontObj = ir.ObjectNumber.Value()
						}
					}
				}
				w.d.content = append(w.d.content, ev)
			}
		}
		operands = operands[:0]
	}
}

// doXObject handles `Do`: an image is drawn content; a form is walked with its own resources.
func (w walker) doXObject(name string, res types.Dict, stack []frame, chain map[int]bool, depth, opIndex int, ts textState) {
	where := fmt.Sprintf("%s, operator #%d `%s Do`", w.where, opIndex, name)
	xobjs := w.d.dict(res["XObject"])
	if xobjs == nil || len(name) < 2 {
		w.d.content = append(w.d.content, w.event(stack, false, where+" (unresolvable XObject)"))
		return
	}
	raw := xobjs[name[1:]]
	sd, _, err := w.d.Ctx.DereferenceStreamDict(raw)
	if err != nil || sd == nil {
		w.d.content = append(w.d.content, w.event(stack, false, where+" (unresolvable XObject)"))
		return
	}
	if sub := sd.Dict.NameEntry("Subtype"); sub == nil || *sub != "Form" {
		w.d.content = append(w.d.content, w.event(stack, false, where+" (image)"))
		return
	}
	objNr := 0
	if ir, ok := raw.(types.IndirectRef); ok {
		objNr = ir.ObjectNumber.Value()
	}
	if chain[objNr] {
		return
	}
	if derr := sd.Decode(); derr != nil {
		w.d.contentErr = fmt.Sprintf("form XObject %s (object %d) could not be decoded: %v", name, objNr, derr)
		return
	}
	formRes := w.d.dict(sd.Dict["Resources"])
	if formRes == nil {
		formRes = res
	}
	inner := walker{d: w.d, where: fmt.Sprintf("%s → form XObject %s (object %d)", w.where, name, objNr), spKey: w.spKey, appearance: w.appearance}
	if sp, ok := sd.Dict["StructParents"].(types.Integer); ok {
		inner.spKey = sp.Value()
	}
	next := map[int]bool{objNr: true}
	for k := range chain {
		next[k] = true
	}
	inner.walkWithState(sd.Content, formRes, stack, next, depth+1, ts)
}

// event builds one event, deriving its coverage from the open sequences.
func (w walker) event(stack []frame, text bool, where string) contentEvent {
	ev := contentEvent{where: where, text: text, mcid: -1, spKey: -1, appearance: w.appearance}
	for i := len(stack) - 1; i >= 0; i-- {
		f := stack[i]
		if f.artifact || f.mcid >= 0 {
			ev.covered = true
		}
		if ev.mcid < 0 && f.mcid >= 0 && !ev.artifact {
			ev.mcid, ev.spKey = f.mcid, f.spKey
		}
		if f.artifact && ev.mcid < 0 {
			ev.artifact = true
		}
	}
	return ev
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

// mcidOf reads the MCID a `BDC` declares, from an inline property dictionary or from a named
// property list in the stream's resources.
func (w walker) mcidOf(src []byte, operands []contentstream.Token, res types.Dict) (int, bool) {
	// Inline: /Tag << … /MCID n … >> BDC
	for i, tk := range operands {
		if tk.Kind == contentstream.Operand && string(tk.Bytes(src)) == "/MCID" && i+1 < len(operands) {
			if n, err := strconv.Atoi(string(operands[i+1].Bytes(src))); err == nil {
				return n, true
			}
		}
	}
	// Named: /Tag /MC0 BDC, resolved through /Resources /Properties.
	names := 0
	for _, tk := range operands {
		if tk.Kind != contentstream.Operand {
			continue
		}
		b := tk.Bytes(src)
		if len(b) == 0 || b[0] != '/' {
			continue
		}
		names++
		if names < 2 {
			continue
		}
		props := w.d.dict(w.d.dict(res["Properties"])[string(b[1:])])
		if props != nil {
			if n := props.IntEntry("MCID"); n != nil {
				return *n, true
			}
		}
	}
	return -1, false
}
