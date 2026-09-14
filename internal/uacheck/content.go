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
	// covered is whether some enclosing marked-content sequence is an artifact or carries an MCID —
	// the two states 7.1 t3 accepts.
	covered  bool
	artifact bool // the nearest relevant enclosing sequence is an /Artifact
	mcid     int  // the innermost MCID in force, or -1
	spKey    int  // the /StructParents key of the stream that owns mcid, or -1
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
	return d.content, d.contentErr
}

// walker carries one stream's context through a walk.
type walker struct {
	d     *Document
	where string
	spKey int
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
	stack := append([]frame(nil), inherited...)
	var operands []contentstream.Token
	opIndex := 0
	for _, tk := range contentstream.Tokenize(src) {
		switch tk.Kind {
		case contentstream.Whitespace:
			continue
		case contentstream.InlineImage:
			opIndex++
			w.record(stack, false, fmt.Sprintf("%s, operator #%d `BI … EI` (inline image)", w.where, opIndex))
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
		case "Do":
			name := w.firstName(src, operands)
			w.doXObject(name, res, stack, chain, depth, opIndex)
		default:
			if textOperators[op] || paintOperators[op] {
				w.record(stack, textOperators[op], fmt.Sprintf("%s, operator #%d `%s`", w.where, opIndex, op))
			}
		}
		operands = operands[:0]
	}
}

// doXObject handles `Do`: an image is drawn content; a form is walked with its own resources.
func (w walker) doXObject(name string, res types.Dict, stack []frame, chain map[int]bool, depth, opIndex int) {
	where := fmt.Sprintf("%s, operator #%d `%s Do`", w.where, opIndex, name)
	xobjs := w.d.dict(res["XObject"])
	if xobjs == nil || len(name) < 2 {
		w.record(stack, false, where+" (unresolvable XObject)")
		return
	}
	raw := xobjs[name[1:]]
	sd, _, err := w.d.Ctx.DereferenceStreamDict(raw)
	if err != nil || sd == nil {
		w.record(stack, false, where+" (unresolvable XObject)")
		return
	}
	if sub := sd.Dict.NameEntry("Subtype"); sub == nil || *sub != "Form" {
		w.record(stack, false, where+" (image)")
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
	inner := walker{d: w.d, where: fmt.Sprintf("%s → form XObject %s (object %d)", w.where, name, objNr), spKey: w.spKey}
	if sp, ok := sd.Dict["StructParents"].(types.Integer); ok {
		inner.spKey = sp.Value()
	}
	next := map[int]bool{objNr: true}
	for k := range chain {
		next[k] = true
	}
	inner.walk(sd.Content, formRes, stack, next, depth+1)
}

// record adds one event, deriving its coverage from the open sequences.
func (w walker) record(stack []frame, text bool, where string) {
	ev := contentEvent{where: where, text: text, mcid: -1, spKey: -1}
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
	w.d.content = append(w.d.content, ev)
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
