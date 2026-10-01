package pdfops

import (
	"fmt"
	"sort"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/contentstream"
	"nib/internal/pdfread"
)

// A paragraph set on another page — `PLAN-text-reflow.md` P07.S05.
//
// The source page loses the paragraph's shows, each replaced by what it does besides drawing — `Tj`/`TJ` by nothing, `'`
// by `T*`, `"` by its spacing and `T*` — so every state after it is exactly what it was. The target page's content is
// wrapped in `q … Q` and drawn after the paragraph, which is drawn first, in the page's default state, as its own
// text object: each run's own show operator, in its own font, size, spacing, scaling, rise and render mode, under the
// fill and stroke it was drawn in, at its user-space position translated. Its fonts enter the target's resources under
// the name the target already gives the same font, else under a fresh one: a name is never overwritten.
//
// What cannot be carried exactly is refused (law 3): a graphics state dictionary or a colour outside the device spaces
// (`state-not-carried`), a run its source page's clip would cut (the clip is not carried: same cause), a run inside a
// sequence whose property list carries no MCID — `/OC`, a `/Lang` span — (same cause: only the MCID's sequence is
// re-opened), invisible text (`invisible-text`), marked content
// that cannot go to the other page with its structure (`tagged-across-pages`, P07.S07's `planTagCarry`), replacement
// text, and a clipping render mode.

const (
	causeStateNotCarried = "state-not-carried"   // drawn in a state another page cannot be given: alpha, a colour space, a clip that cuts it
	causeTaggedAcross    = "tagged-across-pages" // its marked content cannot go to another page with its structure
)

// deleteRuns plans the removal of runs from src: each show replaced by its effect without its glyphs.
func deleteRuns(src []byte, runs []textRun) ([]runMove, string) {
	spans := make([]opSpan, 0, len(runs))
	for _, r := range runs {
		if r.inForm {
			return nil, causeTextInForm
		}
		spans = append(spans, r.span)
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].start < spans[j].start })
	if c := followersReposition(src, spans); c != "" {
		return nil, c
	}
	out := make([]runMove, 0, len(spans))
	for _, sp := range spans {
		effect, ok := showEffect(src, sp)
		if !ok {
			return nil, causeMixedContent
		}
		out = append(out, runMove{span: sp, with: []byte(" " + effect + " ")})
	}
	return out, ""
}

// showEffect is what the show operator in span does besides drawing: nothing for `Tj` and `TJ`, a new line for `'`, the
// spacing it sets and a new line for `"`.
func showEffect(src []byte, span opSpan) (string, bool) {
	show, ok := asTj(src, span)
	if !ok {
		return "", false
	}
	body := src[span.start:span.end]
	toks := contentstream.Tokenize(body)
	op := ""
	for _, t := range toks {
		if t.Kind == contentstream.Operator {
			op = string(t.Bytes(body))
		}
	}
	switch op {
	case "Tj", "TJ":
		return "", true
	case "'":
		return "T*", true
	case "\"":
		// asTj wrote `aw Tw ac Tc (s) Tj`; keep the spacing, drop the string and its Tj.
		i := strings.LastIndex(show, " Tc ")
		if i < 0 {
			return "", false
		}
		return show[:i+len(" Tc")] + " T*", true
	}
	return "", false
}

// carryRefusal is why run cannot be drawn on another page, or "".
func carryRefusal(r textRun) string {
	st := r.state
	switch {
	case r.inForm:
		return causeTextInForm
	case r.replaced:
		return causeReplacementText
	case invisibleMode(st.tr):
		return causeInvisible // a search layer's words belong over their scan (P07 phase-close review)
	case st.tr >= 4:
		return causeClips
	case st.extGState || st.fill == "" || ((st.tr == 1 || st.tr == 2) && st.stroke == ""):
		return causeStateNotCarried
	case r.propsWithoutMCID:
		// Only the sequence carrying the run's MCID is re-opened on the target: an `/OC` membership or a `/Lang` span
		// around it would be lost, and a hidden layer's text drawn for everyone (P07 phase-close review).
		return causeStateNotCarried
	case !covers(st.clip, runBox(r)):
		return causeStateNotCarried
	}
	return ""
}

// pageResources is page pg's own resource dictionary for writing: a copy of what it uses — its own or the one it
// inherits — set on the page itself, with a copy of its /Font, so adding a font never touches a dictionary another page
// shares.
func pageResources(ctx *model.Context, pg pdfread.Page) (res, fonts types.Dict) {
	xt := ctx.XRefTable
	var src types.Dict
	if pg.Attrs != nil && pg.Attrs.Resources != nil {
		src = pg.Attrs.Resources
	} else {
		src = derefDict(xt, pg.Dict["Resources"])
	}
	res = types.Dict{}
	for k, v := range src {
		res[k] = v
	}
	fonts = types.Dict{}
	for k, v := range derefDict(xt, res["Font"]) {
		fonts[k] = v
	}
	res["Font"] = fonts
	pg.Dict["Resources"] = res
	return res, fonts
}

// sameObject says whether a and b are the same indirect object.
func sameObject(a, b types.Object) bool {
	ra, aok := a.(types.IndirectRef)
	rb, bok := b.(types.IndirectRef)
	return aok && bok && ra.ObjectNumber == rb.ObjectNumber && ra.GenerationNumber == rb.GenerationNumber
}

// fontNameOn is the name font is drawn by in fonts: the name fonts already gives it, else want when free, else the first
// free `NibF<n>` — adding it under that name. A name already in use is never overwritten.
func fontNameOn(fonts types.Dict, want string, font types.Object) string {
	for k, v := range fonts {
		if sameObject(v, font) {
			return k
		}
	}
	name := want
	for n := 1; ; n++ {
		if _, taken := fonts[name]; !taken {
			break
		}
		name = fmt.Sprintf("NibF%d", n)
	}
	fonts[name] = font
	return name
}

// setRunsOn plans runs, read from src — the content of the page whose resources are srcRes — as a text object drawn
// before old, dst's content (with any edit of its own already made), each dx right and dy DOWN in user space, and adds the
// fonts it needs to dst. tags is the structure the runs take with them (`planTagCarry`) and dstSeqs the marked-content
// sequences dst's own stream draws: each carried sequence is drawn under its own tag and property list at an MCID free
// on dst, and the tree is written to say so (P07.S07). It returns dst's new content, or the cause.
func setRunsOn(ctx *model.Context, src []byte, srcRes types.Dict, runs []textRun, dst pdfread.Page, old []byte, dx, dy float64,
	tags *tagCarry, dstSeqs []markedSeq) ([]byte, string, error) {
	for _, r := range runs {
		if c := carryRefusal(r); c != "" {
			return nil, c, nil
		}
		if r.mcid >= 0 && (tags == nil || tags.byMCID[r.mcid] == nil) {
			return nil, causeTaggedAcross, nil
		}
		if _, ok := shiftedTm(r.state, 0); !ok || !finite(dx, dy) {
			return nil, causeDegenerate, nil
		}
	}
	// Each sequence's runs drawn together, in the order the runs first name it: a sequence is opened once. Runs are placed
	// by their own matrices, so the order they are drawn in moves nothing.
	runs = groupedByMCID(runs)
	srcFonts := derefDict(ctx.XRefTable, srcRes["Font"])
	_, dstFonts := pageResources(ctx, dst)
	var b strings.Builder
	if tags != nil {
		if c := targetRefusal(ctx, dst); c != "" {
			return nil, c, nil
		}
		if err := tags.land(ctx, dst, dstSeqs); err != nil {
			return nil, "", err
		}
	}
	fill, stroke := "", ""
	b.WriteString("q BT\n")
	open := -1
	for _, r := range runs {
		st := r.state
		if r.mcid != open {
			if open >= 0 {
				b.WriteString("EMC\n")
			}
			if r.mcid >= 0 {
				b.WriteString(tags.byMCID[r.mcid].opener() + "\n")
			}
			open = r.mcid
		}
		font, ok := srcFonts[r.font]
		if !ok {
			return nil, causeNoWidths, nil // a font the page does not name cannot be carried; it had no widths either
		}
		if st.fill != fill || st.stroke != stroke {
			// Colour operators are allowed inside a text object (ISO 32000-1 Figure 9's text object state), so a run's
			// colour is set where it changes.
			fmt.Fprintf(&b, "%s %s\n", st.fill, st.stroke)
			fill, stroke = st.fill, st.stroke
		}
		name := fontNameOn(dstFonts, r.font, font)
		// The run's matrix in user space, moved: its CTM is folded in, and the target draws under the default CTM.
		tm := st.tm.mul(st.ctm).mul(runTranslate(dx, -dy))
		show, ok := asTj(src, r.span)
		if !ok {
			return nil, causeMixedContent, nil
		}
		fmt.Fprintf(&b, "%s %s Tf %s Tc %s Tw %s Tz %s Ts %d Tr %s Tm %s\n", pdfName(name), num(st.tfSize), num(st.tc), num(st.tw),
			num(st.th*100), num(st.ts), st.tr, matrixOperands(tm), show)
	}
	if open >= 0 {
		b.WriteString("EMC\n")
	}
	b.WriteString("ET Q\n")
	// **The carried block is drawn LAST, over the target's own content** — which runs first, wrapped, whatever it leaves
	// open closed inside its own `q … Q`. The phase-close review tried the other order, so that stream order would match
	// reading order on an untagged page, and its re-review measured the cost: a target that paints a backdrop — a page-
	// colour fill, a letterhead, a cell's shading, which `regionOf` lets text land on — then painted OVER the carried
	// paragraph, hiding it while copy-and-paste still found it. A paragraph that cannot be seen is the worse failure.
	var out strings.Builder
	out.WriteString("q\n")
	out.Write(old)
	out.WriteString("\n" + closeOpen(old) + "Q\n")
	out.WriteString(b.String())
	return []byte(out.String()), "", nil
}

// groupedByMCID is runs with each MCID's runs together, in the order the runs first name each; runs with none keep their
// place among the groups as one of their own.
func groupedByMCID(runs []textRun) []textRun {
	var order []int
	groups := map[int][]textRun{}
	for _, r := range runs {
		if _, ok := groups[r.mcid]; !ok {
			order = append(order, r.mcid)
		}
		groups[r.mcid] = append(groups[r.mcid], r)
	}
	out := make([]textRun, 0, len(runs))
	for _, m := range order {
		out = append(out, groups[m]...)
	}
	return out
}

// setParagraphOn moves paragraph pi of page srcPg onto page dst, dx right and dy down in user space: the source page stops
// drawing it and the target draws it. The caller writes the document; it returns the cause when it cannot.
func setParagraphOn(ctx *model.Context, layout pageLayout, srcPg pdfread.Page, pi int, dst pdfread.Page, dx, dy float64) (string, error) {
	if pi < 0 || pi >= len(layout.paragraphs) {
		return causeNoParagraph, nil
	}
	if srcPg.Nr == dst.Nr {
		return "", fmt.Errorf("pdfops: a paragraph set on its own page is a move, not a carry")
	}
	src, err := pdfread.PageContent(ctx, srcPg.Dict, srcPg.Nr)
	if err != nil {
		return "", err
	}
	runs := paragraphRunsWithBlanks(layout, pi)
	var srcRes types.Dict
	if srcPg.Attrs != nil {
		srcRes = srcPg.Attrs.Resources
	}
	old, err := pdfread.PageContent(ctx, dst.Dict, dst.Nr)
	if err != nil && err != model.ErrNoContent {
		return "", err
	}
	dels, cause := deleteRuns(src, runs)
	if cause != "" {
		return cause, nil
	}
	tags, brackets, cause := planTagCarry(ctx, srcPg, src, srcRes, layout.sequences, runs)
	if cause != "" {
		return cause, nil
	}
	dels = append(dels, brackets...)
	dl, err := readPageGlyphLayout(ctx, dst)
	if err != nil {
		return "", err
	}
	dstContent, cause, err := setRunsOn(ctx, src, srcRes, runs, dst, old, dx, dy, tags, dl.sequences)
	if err != nil || cause != "" {
		return cause, err
	}
	e := contentstream.NewEdit(src)
	for _, d := range dels {
		e.Replace(d.span.start, d.span.end, d.with)
	}
	srcContent, err := e.Apply()
	if err != nil {
		return "", err
	}
	if err := setPageContent(ctx, srcPg.Dict, srcContent); err != nil {
		return "", err
	}
	return "", setPageContent(ctx, dst.Dict, dstContent)
}

// closeOpen is what closes whatever content leaves open — an `ET` for a text object still open at its end, an `EMC` for a
// marked-content sequence (P07.S07: a carried sequence would otherwise be read as nested in it), and a `Q` for every `q`
// it did not restore — innermost first, so the `Q` wrapped around it pops the state wrapped around it, and what follows
// draws under the page's default state, not under a CTM, a text object or a sequence the content forgot to close.
func closeOpen(content []byte) string {
	var open []string
	pop := func(closer string) {
		for i := len(open) - 1; i >= 0; i-- {
			if open[i] == closer {
				open = append(open[:i], open[i+1:]...)
				return
			}
		}
	}
	for _, tk := range contentstream.Tokenize(content) {
		if tk.Kind != contentstream.Operator {
			continue
		}
		switch string(tk.Bytes(content)) {
		case "q":
			open = append(open, "Q")
		case "Q":
			pop("Q")
		case "BT":
			open = append(open, "ET")
		case "ET":
			pop("ET")
		case "BMC", "BDC":
			open = append(open, "EMC")
		case "EMC":
			pop("EMC")
		}
	}
	out := ""
	for i := len(open) - 1; i >= 0; i-- {
		out += open[i] + "\n"
	}
	return out
}
