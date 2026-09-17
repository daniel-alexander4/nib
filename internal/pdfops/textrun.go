package pdfops

import (
	"fmt"
	"math"
	"strconv"
	"unicode/utf16"

	"github.com/pdfcpu/pdfcpu/pkg/font"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
	"golang.org/x/text/encoding/charmap"

	"nib/internal/contentstream"
)

// Positioned runs — `PLAN-accessibility.md` P08.S02, which is `PLAN-text-reflow.md` P03.
//
// A page read into runs: one per text-showing operator (`Tj`, `TJ`, `'`, `"`), each carrying its text,
// the font it was selected by, its effective size, where its first glyph sits in user space, and how
// far it advances — with the width's source from the width reader (S01), so a run measured from
// nothing says so.
//
// # What a run is, and what it is not
//
// A run is the DOCUMENT's unit: one show operator. pdf.js's `getTextContent` items are not that —
// measured on a LibreOffice page, one `Tj` became three items split at a space. So agreement with
// pdf.js (seam S7) is asserted on what both readers must agree on regardless of how they cut text:
// the page's text, and the baselines it sits on. Grouping runs into lines is S03's, once.
//
// # Text is decoded only from what the document says
//
// `/ToUnicode` first. Without one, a simple font declaring `WinAnsiEncoding` or `MacRomanEncoding` is
// decoded through that table, and a Latin core font with no `/Encoding` through StandardEncoding's
// printable range. Anything else — `/Differences`, a Type0 font under a CMap other than Identity, a
// symbolic font with no map — is NOT guessed: its codes still advance the text position, and the run
// reports `decoded == false`.
//
// # Containment (reflow D6)
//
// The walk reads documents written by every PDF producer there is, through pdfcpu's dereferencing.
// `readPageRuns` recovers a panic anywhere beneath it and returns it as an error for that page, so one
// malformed page costs its text and not the process.

// textRun is one text-showing operator's glyphs, as drawn.
type textRun struct {
	text    string
	decoded bool // every code became text; false means text holds only what did
	// font is the resource name the run was selected by (`F1`), and baseFont the font's /BaseFont.
	font     string
	baseFont string
	// size is the effective size in user space: the `Tf` size scaled by the text and current
	// transformation matrices, so a 10pt font under a 2× `cm` is 20.
	size float64
	// x, y is the origin of the first glyph in user space, including text rise.
	x, y float64
	// width is the run's total advance in user space, kerning and spacing included.
	width float64
	// widthSrc is the WEAKEST source among the run's glyphs — one glyph measured from nothing makes
	// the run's width `none`, because a width that is partly invented is invented.
	widthSrc widthSource
	codes    int
	// mcid is the marked-content id the run was drawn under — the innermost enclosing sequence that
	// carries one — or -1, including inside an `/Artifact` sequence. It is how a structure tree's
	// element is matched to the text it tags (P08.S04 reads LibreOffice's tree as truth through it).
	mcid int
	// span is the byte range of the run's show operator and its own operands, in the stream it was
	// drawn from — what a commit brackets in marked content (P08.S06a). inForm says that stream was a
	// form XObject's, not the page's: bracketing the page stream there would describe the `Do`.
	span   opSpan
	inForm bool
	// stm is the OBJECT NUMBER of the content stream the run's marked-content sequence was OPENED in,
	// or 0 for the page's own. `inForm` says only *that* the glyphs were drawn inside a form; this
	// says which stream owns the SEQUENCE, which is the difference between "an MCID somewhere on this
	// page" and the one a `/Stm` names.
	//
	// **Opened-in, not drawn-in**, because `/Stm` is *"the content stream containing the marked-content
	// sequence"* (Table 324) and a `BDC` on the page can bracket a `Do` — see `currentStm`.
	//
	// **Two forms drawn on one sheet both carry an MCID 0**, and that is the ordinary shape of an
	// n-up, not a pathology — so without this the two are indistinguishable and a reader keying on
	// `(page, mcid)` hands both texts to both elements. Measured before this field existed: 31 of 61
	// elements came back with two pages' text concatenated (P02.S08).
	//
	// It is 0 rather than -1 for a stream that is not an indirect object, because `/Stm` "shall be an
	// indirect reference" (ISO 32000-1 Table 324) and so can never name one: unnameable and
	// page's-own are the same answer to the only question asked of this field.
	stm int
	// artifact is whether the run was drawn inside an `/Artifact` sequence — content the document
	// itself says is not content, a watermark or a running header. Grouping skips it, so an artifact
	// is never proposed as a paragraph (P08.S06a's watermark finding).
	artifact bool
	// rotated is whether the run's baseline is not upright left-to-right in user space — turned, vertical or
	// mirrored (`baselineTurns`). Grouping measures lines as horizontal baselines, so it reports a page
	// carrying one rather than reading it as upright (`/pending 503`).
	rotated bool
}

// baselineTurns reports whether text drawn under m runs anywhere but rightward along +x: the text-space x
// axis mapped to user space points more than a degree away. A skew (`c`) leaves the baseline flat and is
// not a turn; a page's `/Rotate` is not in the content matrix and turns every line alike, which grouping
// in unrotated space reads consistently.
func baselineTurns(m runMatrix) bool {
	return math.Abs(math.Atan2(m[1], m[0])) > math.Pi/180
}

// inArtifact reports whether an `/Artifact` sequence is open at this point of the walk.
func (w *runWalker) inArtifact() bool {
	for _, v := range w.mcStack {
		if v == mcArtifact {
			return true
		}
	}
	return false
}

// pageRuns is a page read as text.
type pageRuns struct {
	runs []textRun
	// noText is true when the page draws no glyph at all, directly or through a form XObject — an
	// image-only or blank page. Said structurally, so a scan is not left to be inferred from an empty
	// slice; a reader failure is an error, never this. A show operator with an empty string draws
	// nothing and counts as nothing: counting operators instead was a distinction no caller reads.
	noText bool
	// sequences are the page's marked-content sequences that carry an MCID, in the order they open —
	// those inside the forms it draws included — so a writer can find an element's content whether or
	// not it is text (P09.S03).
	sequences []markedSeq
}

// maxFormDepth bounds form XObject recursion. A form may draw itself; the specification forbids it and
// documents do it anyway.
const maxFormDepth = 12

// maxToUnicodeRange bounds one `bfrange`: a 16-bit code space has 65,536 codes.
const maxToUnicodeRange = 1 << 16

// maxToUnicodeExpansion bounds the codes one CMap's ranges may expand to in TOTAL: two full 16-bit code
// spaces. A font nib can split has at most one (a simple font 256 codes, Identity-H 65,536), so the second
// is headroom for ranges that overlap, not for more distinct codes.
const maxToUnicodeExpansion = 2 << 16

// readPageRuns reads one page's positioned runs.
func readPageRuns(ctx *model.Context, pageNr int) (pageRuns, error) {
	return containRunRead(pageNr, func() (pageRuns, error) {
		d, _, attrs, err := ctx.PageDict(pageNr, false)
		if err != nil || d == nil {
			return pageRuns{}, fmt.Errorf("pdfops: page %d does not resolve: %w", pageNr, err)
		}
		content, cerr := ctx.PageContent(d, pageNr)
		if cerr != nil && cerr != model.ErrNoContent {
			return pageRuns{}, cerr
		}
		var res types.Dict
		if attrs != nil {
			res = attrs.Resources
		}
		w := newRunWalker(ctx.XRefTable)
		w.walk(content, res, newRunGState(), 0, map[int]bool{})
		return pageRuns{runs: w.runs, noText: len(w.runs) == 0, sequences: w.seqs}, nil
	})
}

// containRunRead runs read and turns a panic into an error for the page — reflow D6's containment,
// kept separate from the walk so its firing can be driven directly.
func containRunRead(pageNr int, read func() (pageRuns, error)) (pr pageRuns, err error) {
	defer func() {
		if r := recover(); r != nil {
			pr, err = pageRuns{}, fmt.Errorf("pdfops: page %d could not be read as text — the reader failed on this document: %v", pageNr, r)
		}
	}()
	return read()
}

// runMatrix is a PDF transformation matrix [a b c d e f].
type runMatrix [6]float64

var runIdentity = runMatrix{1, 0, 0, 1, 0, 0}

// mul returns m followed by n — PDF's row-vector order, so `cm` is `M.mul(CTM)`.
func (m runMatrix) mul(n runMatrix) runMatrix {
	return runMatrix{
		m[0]*n[0] + m[1]*n[2], m[0]*n[1] + m[1]*n[3],
		m[2]*n[0] + m[3]*n[2], m[2]*n[1] + m[3]*n[3],
		m[4]*n[0] + m[5]*n[2] + n[4], m[4]*n[1] + m[5]*n[3] + n[5],
	}
}

func (m runMatrix) apply(x, y float64) (float64, float64) {
	return x*m[0] + y*m[2] + m[4], x*m[1] + y*m[3] + m[5]
}

func runTranslate(tx, ty float64) runMatrix { return runMatrix{1, 0, 0, 1, tx, ty} }

// runGState is the graphics state a text run reads. Text state is part of the graphics state: `q`
// saves it, `Q` restores it, and `BT`/`ET` do not reset it — measured against veraPDF at P07.S04, and
// visible in nib's own Markdown output, which selects a font in one text object and draws in the next.
type runGState struct {
	ctm      runMatrix
	fontName string
	font     *runFont
	size     float64
	tc, tw   float64
	th       float64 // Tz / 100
	tl, ts   float64
}

func newRunGState() runGState { return runGState{ctm: runIdentity, th: 1} }

// runFont is what the walk needs from a font dictionary.
type runFont struct {
	baseFont string
	twoByte  bool // Identity-H or Identity-V: codes are two bytes and CID == code
	// splittable is false for a Type0 font under a CMap this reader does not parse: its code lengths
	// are unknown, so neither its text nor its widths can be read without guessing.
	splittable bool
	widths     fontWidths
	toUni      map[string]string
	simple     func(byte) (rune, bool)
}

func loadRunFont(xt *model.XRefTable, obj types.Object) *runFont {
	d, err := xt.DereferenceDict(obj)
	if err != nil || d == nil {
		return nil
	}
	f := &runFont{widths: readFontWidths(xt, d), splittable: true}
	if bf := d.NameEntry("BaseFont"); bf != nil {
		f.baseFont = *bf
	}
	if st := d.NameEntry("Subtype"); st != nil && *st == "Type0" {
		if enc := d.NameEntry("Encoding"); enc != nil && (*enc == "Identity-H" || *enc == "Identity-V") {
			f.twoByte = true
		} else {
			f.splittable = false
		}
	}
	if tu, ok := d["ToUnicode"]; ok {
		if sd, _, serr := xt.DereferenceStreamDict(tu); serr == nil && sd != nil {
			if body := streamContent(sd); body != nil {
				f.toUni = parseToUnicode(body)
			}
		}
	}
	if !f.twoByte && f.splittable {
		f.simple = simpleDecoderFor(d)
	}
	return f
}

// textFor decodes one code, or reports that the document gives no way to.
func (f *runFont) textFor(code []byte) (string, bool) {
	if f == nil || !f.splittable {
		return "", false
	}
	if s, ok := f.toUni[string(code)]; ok {
		return s, true
	}
	if f.simple != nil && len(code) == 1 {
		if r, ok := f.simple(code[0]); ok {
			return string(r), true
		}
	}
	return "", false
}

// simpleDecoderFor returns the byte decoder a simple font's /Encoding names, or nil where reading its
// text would be a guess.
func simpleDecoderFor(d types.Dict) func(byte) (rune, bool) {
	// Presence and kind, not `NameEntry`: it returns nil for an absent key AND for a `/Differences`
	// dictionary, and the first cut took the second for the first — decoding a font whose glyphs were
	// renamed as though they were StandardEncoding's (found at P08.S06c).
	encObj, hasEnc := d["Encoding"]
	if !hasEnc {
		if bf := d.NameEntry("BaseFont"); bf != nil && font.IsCoreFont(*bf) && *bf != "Symbol" && *bf != "ZapfDingbats" {
			return decodeStandardPrintable
		}
		return nil
	}
	encName, isName := encObj.(types.Name)
	if !isName {
		return nil
	}
	switch encName.Value() {
	case "WinAnsiEncoding":
		return charmapByteDecoder(charmap.Windows1252)
	case "MacRomanEncoding":
		return charmapByteDecoder(charmap.Macintosh)
	case "StandardEncoding":
		return decodeStandardPrintable
	}
	return nil
}

func charmapByteDecoder(cm *charmap.Charmap) func(byte) (rune, bool) {
	return func(b byte) (rune, bool) {
		r := cm.DecodeByte(b)
		if r == '�' || r < 0x20 || (r >= 0x7F && r < 0xA0) {
			return 0, false
		}
		return r, true
	}
}

// decodeStandardPrintable reads StandardEncoding's printable ASCII range, where it differs from ASCII
// only at the two quotes. Codes above it name glyphs this reader does not carry a table for.
func decodeStandardPrintable(b byte) (rune, bool) {
	switch {
	case b == 0x27:
		return '’', true
	case b == 0x60:
		return '‘', true
	case b >= 0x20 && b <= 0x7E:
		return rune(b), true
	}
	return 0, false
}

// runWalker accumulates runs across a page and the forms it draws.
type runWalker struct {
	xt    *model.XRefTable
	fonts map[int]*runFont
	runs  []textRun
	// mcStack is the marked-content sequences open at this point of the walk, innermost last: an MCID,
	// -1 for a sequence with none, or mcArtifact. Shared across a page and the forms it draws, because
	// a `BDC` around a `Do` tags what the form draws.
	mcStack []int
	// seqs are the MCID-carrying sequences seen so far; seqOpen parallels mcStack with each open
	// sequence's index into seqs, or -1 for one that carries no MCID.
	seqs    []markedSeq
	seqOpen []int
	// stm is the object number of the stream currently being walked, 0 for the page's own. `drawForm`
	// saves and restores it around its recursion exactly as it does `visiting`, so it is always the
	// stream the token under the cursor came from rather than the one the walk started in.
	stm int
}

// markedSeq is one marked-content sequence that carries an MCID: the one reading of `BDC` the tree
// writers and the run reader share, so an element's content is found by the rule its text is read by.
type markedSeq struct {
	mcid int
	// opener covers the tag, its property list and `BDC`; close covers the `EMC`, and is zero when the
	// stream ended first. Both are offsets into the stream the sequence was read from.
	opener, close opSpan
	// inForm says that stream was a form XObject's; drawsForm says a form XObject is drawn inside the
	// sequence, so its content is not all in the stream the opener is in.
	inForm, drawsForm bool
	// stm is the object number of the stream the opener was read from, or 0 for the page's own — the
	// same field `textRun.stm` carries, for the same reason.
	stm int
}

// mcArtifact marks an `/Artifact` sequence on the stack: content inside it belongs to no element,
// whatever encloses it.
const mcArtifact = -2

// currentMCID is the innermost MCID in force, or -1.
func (w *runWalker) currentMCID() int {
	for i := len(w.mcStack) - 1; i >= 0; i-- {
		switch v := w.mcStack[i]; {
		case v == mcArtifact:
			return -1
		case v >= 0:
			return v
		}
	}
	return -1
}

// currentStm is the object number of the stream the innermost in-force sequence was OPENED in, or 0
// for the page's own.
//
// **Where the sequence was opened is not where the glyphs are, and `/Stm` names the former.** ISO
// 32000-1 Table 324 calls it *"the content stream containing the marked-content sequence"*, and
// `w.mcStack` deliberately spans the form boundary — a `BDC` around a `Do` tags what the form draws
// (see `runWalker.mcStack`). So a sequence opened on the page whose glyphs land inside a form belongs
// to the PAGE's stream, and stamping a run with the stream it was drawn in would file that text under
// a stream no `/Stm` will ever name. Caught in review before it shipped; `markedSeq.stm` was already
// recording the right number and nothing was reading it.
func (w *runWalker) currentStm() int {
	for i := len(w.mcStack) - 1; i >= 0; i-- {
		switch v := w.mcStack[i]; {
		case v == mcArtifact:
			return 0
		case v >= 0:
			// `seqOpen` is pushed and popped in lockstep with `mcStack`, so the same index is the
			// same sequence; the bound is belt-and-braces against an unbalanced stream desyncing them.
			if i < len(w.seqOpen) {
				if j := w.seqOpen[i]; j >= 0 && j < len(w.seqs) {
					return w.seqs[j].stm
				}
			}
			return 0
		}
	}
	return 0
}

// markedContentID reads `/MCID` from a `BDC` property list, written inline or named in the resources'
// `/Properties`.
func (w *runWalker) markedContentID(o runOperand, res types.Dict, src []byte) int {
	if o.opaque {
		for j := 0; j < len(o.dict); j++ {
			if o.dict[j].Kind != contentstream.Operand || string(o.dict[j].Bytes(src)) != "/MCID" {
				continue
			}
			for k := j + 1; k < len(o.dict); k++ {
				if o.dict[k].Kind == contentstream.Whitespace {
					continue
				}
				if v, err := strconv.Atoi(string(o.dict[k].Bytes(src))); err == nil && v >= 0 {
					return v
				}
				break
			}
		}
		return -1
	}
	name, ok := o.name(src)
	if !ok || res == nil {
		return -1
	}
	props, err := w.xt.DereferenceDict(res["Properties"])
	if err != nil || props == nil {
		return -1
	}
	d, derr := w.xt.DereferenceDict(props[name])
	if derr != nil || d == nil {
		return -1
	}
	if n := d.IntEntry("MCID"); n != nil && *n >= 0 {
		return *n
	}
	return -1
}

func newRunWalker(xt *model.XRefTable) *runWalker {
	return &runWalker{xt: xt, fonts: map[int]*runFont{}}
}

// runOperand is one operand of an operator: a token, or a whole array.
type runOperand struct {
	tok    contentstream.Token
	arr    []contentstream.Token
	isArr  bool
	opaque bool // a dictionary operand, which no text operator takes
	// dict holds a dictionary operand's tokens, for the one reader that looks inside: `BDC`'s
	// property list and its `/MCID`.
	dict []contentstream.Token
	// start is where the operand begins in the stream, so a show operator's span can include its own
	// operands.
	start int
}

func (o runOperand) number(src []byte) (float64, bool) {
	if o.isArr || o.opaque || o.tok.Kind != contentstream.Operand {
		return 0, false
	}
	v, err := strconv.ParseFloat(string(o.tok.Bytes(src)), 64)
	return v, err == nil
}

func (o runOperand) name(src []byte) (string, bool) {
	if o.isArr || o.opaque || o.tok.Kind != contentstream.Operand {
		return "", false
	}
	b := o.tok.Bytes(src)
	if len(b) < 2 || b[0] != '/' {
		return "", false
	}
	return decodePDFName(b[1:]), true
}

func (o runOperand) str(src []byte) ([]byte, bool) {
	if o.isArr || o.opaque {
		return nil, false
	}
	if o.tok.Kind != contentstream.LiteralString && o.tok.Kind != contentstream.HexString {
		return nil, false
	}
	return decodePDFString(o.tok.Bytes(src)), true
}

// tjPiece is one element of a `TJ` array: codes to show, or a position adjustment.
type tjPiece struct {
	codes    []byte
	adjust   float64
	isAdjust bool
}

// walk reads one content stream under res and gs.
func (w *runWalker) walk(src []byte, res types.Dict, gs runGState, depth int, visiting map[int]bool) {
	var stack []runGState
	tm, tlm := runIdentity, runIdentity
	var ops []runOperand
	toks := contentstream.Tokenize(src)
	// A stream's sequences end with the stream: an unbalanced `EMC` inside a form cannot close the page's
	// sequence, and a form that leaves one open cannot tag what the page draws after it.
	base, seqBase := len(w.mcStack), len(w.seqOpen)
	defer func() {
		if len(w.seqOpen) > seqBase {
			w.seqOpen = w.seqOpen[:seqBase]
		}
		// Only ever SHRINK. Reslicing to `base` when the stack is already shorter reaches back into the
		// backing array and resurrects entries an `EMC` removed — which a probe found masking a popped
		// page sequence as though it had never closed.
		if len(w.mcStack) > base {
			w.mcStack = w.mcStack[:base]
		}
	}()

	last := func(n int) []runOperand {
		if len(ops) < n {
			return nil
		}
		return ops[len(ops)-n:]
	}
	numbers := func(n int) ([]float64, bool) {
		os := last(n)
		if os == nil {
			return nil, false
		}
		out := make([]float64, n)
		for i, o := range os {
			v, ok := o.number(src)
			if !ok {
				return nil, false
			}
			out[i] = v
		}
		return out, true
	}
	nextLine := func() {
		tlm = runTranslate(0, -gs.tl).mul(tlm)
		tm = tlm
	}
	// showString shows the string that is the operator's last operand; arity is how many operands the
	// operator takes, so the run's span starts at the first of them and not at a stray one before.
	showString := func(arity, end int) {
		os := last(arity)
		if os == nil {
			return
		}
		if s, ok := os[arity-1].str(src); ok {
			w.show(&tm, gs, []tjPiece{{codes: s}}, opSpan{os[0].start, end}, depth > 0)
		}
	}

	for i := 0; i < len(toks); i++ {
		tok := toks[i]
		switch tok.Kind {
		case contentstream.Whitespace, contentstream.InlineImage:
			continue
		case contentstream.ArrayOpen:
			end := matchingClose(toks, i, contentstream.ArrayOpen, contentstream.ArrayClose)
			ops = append(ops, runOperand{arr: toks[i+1 : end], isArr: true, start: tok.Start})
			i = end
			continue
		case contentstream.DictOpen:
			end := matchingClose(toks, i, contentstream.DictOpen, contentstream.DictClose)
			ops = append(ops, runOperand{opaque: true, dict: toks[i+1 : end], start: tok.Start})
			i = end
			continue
		case contentstream.Operator:
		default:
			ops = append(ops, runOperand{tok: tok, start: tok.Start})
			continue
		}

		switch string(tok.Bytes(src)) {
		case "q":
			stack = append(stack, gs)
		case "Q":
			if n := len(stack); n > 0 {
				gs, stack = stack[n-1], stack[:n-1]
			}
		case "cm":
			if v, ok := numbers(6); ok {
				gs.ctm = runMatrix{v[0], v[1], v[2], v[3], v[4], v[5]}.mul(gs.ctm)
			}
		case "BT":
			tm, tlm = runIdentity, runIdentity
		case "Tf":
			if os := last(2); os != nil {
				name, nok := os[0].name(src)
				size, sok := os[1].number(src)
				if nok && sok {
					gs.fontName, gs.size, gs.font = name, size, w.fontFor(res, name)
				}
			}
		case "Tc":
			if v, ok := numbers(1); ok {
				gs.tc = v[0]
			}
		case "Tw":
			if v, ok := numbers(1); ok {
				gs.tw = v[0]
			}
		case "Tz":
			if v, ok := numbers(1); ok {
				gs.th = v[0] / 100
			}
		case "TL":
			if v, ok := numbers(1); ok {
				gs.tl = v[0]
			}
		case "Ts":
			if v, ok := numbers(1); ok {
				gs.ts = v[0]
			}
		case "Td", "TD":
			if v, ok := numbers(2); ok {
				if string(tok.Bytes(src)) == "TD" {
					gs.tl = -v[1]
				}
				tlm = runTranslate(v[0], v[1]).mul(tlm)
				tm = tlm
			}
		case "T*":
			nextLine()
		case "Tm":
			if v, ok := numbers(6); ok {
				tm = runMatrix{v[0], v[1], v[2], v[3], v[4], v[5]}
				tlm = tm
			}
		case "Tj":
			showString(1, tok.End)
		case "'":
			nextLine()
			showString(1, tok.End)
		case "\"":
			if os := last(3); os != nil {
				aw, aok := os[0].number(src)
				ac, cok := os[1].number(src)
				if aok && cok {
					gs.tw, gs.tc = aw, ac
					nextLine()
					showString(3, tok.End)
				}
			}
		case "TJ":
			if os := last(1); os != nil && os[0].isArr {
				var pieces []tjPiece
				for _, at := range os[0].arr {
					switch at.Kind {
					case contentstream.LiteralString, contentstream.HexString:
						pieces = append(pieces, tjPiece{codes: decodePDFString(at.Bytes(src))})
					case contentstream.Operand:
						if v, err := strconv.ParseFloat(string(at.Bytes(src)), 64); err == nil {
							pieces = append(pieces, tjPiece{adjust: v, isAdjust: true})
						}
					}
				}
				w.show(&tm, gs, pieces, opSpan{os[0].start, tok.End}, depth > 0)
			}
		case "BMC":
			entry := -1
			if os := last(1); os != nil {
				if tag, ok := os[0].name(src); ok && tag == "Artifact" {
					entry = mcArtifact
				}
			}
			w.mcStack = append(w.mcStack, entry)
			w.seqOpen = append(w.seqOpen, -1)
		case "BDC":
			entry, opener := -1, -1
			if os := last(2); os != nil {
				if tag, ok := os[0].name(src); ok && tag == "Artifact" {
					entry = mcArtifact
				} else {
					entry = w.markedContentID(os[1], res, src)
				}
				opener = os[0].start
			}
			w.mcStack = append(w.mcStack, entry)
			if entry >= 0 && opener >= 0 {
				w.seqs = append(w.seqs, markedSeq{mcid: entry, opener: opSpan{opener, tok.End},
					inForm: depth > 0, stm: w.stm})
				w.seqOpen = append(w.seqOpen, len(w.seqs)-1)
			} else {
				w.seqOpen = append(w.seqOpen, -1)
			}
		case "EMC":
			if n := len(w.mcStack); n > base {
				w.mcStack = w.mcStack[:n-1]
			}
			if n := len(w.seqOpen); n > seqBase {
				if i := w.seqOpen[n-1]; i >= 0 {
					w.seqs[i].close = opSpan{tok.Start, tok.End}
				}
				w.seqOpen = w.seqOpen[:n-1]
			}
		case "Do":
			if os := last(1); os != nil {
				if name, ok := os[0].name(src); ok && w.drawForm(res, name, gs, depth, visiting) {
					for _, i := range w.seqOpen {
						if i >= 0 {
							w.seqs[i].drawsForm = true
						}
					}
				}
			}
		}
		ops = ops[:0]
	}
}

// show advances the text matrix across one show operator's glyphs and records the run.
func (w *runWalker) show(tm *runMatrix, gs runGState, pieces []tjPiece, span opSpan, inForm bool) {
	start := tm.mul(gs.ctm)
	run := textRun{font: gs.fontName, size: gs.size * math.Hypot(start[2], start[3]), decoded: true,
		mcid: w.currentMCID(), span: span, inForm: inForm, stm: w.currentStm(), artifact: w.inArtifact()}
	if gs.font != nil {
		run.baseFont = gs.font.baseFont
	}
	run.x, run.y = start.apply(0, gs.ts)
	run.rotated = baselineTurns(start)
	var text []byte
	var advance float64
	weakest := widthSource("")
	for _, p := range pieces {
		if p.isAdjust {
			tx := -p.adjust / 1000 * gs.size * gs.th
			*tm = runTranslate(tx, 0).mul(*tm)
			advance += tx
			continue
		}
		for _, code := range splitCodes(gs.font, p.codes) {
			run.codes++
			w0, src := 0.0, widthNone
			if gs.font != nil && gs.font.splittable {
				w0, src = gs.font.widths.advance(codeValue(code))
			}
			weakest = weakerWidthSource(weakest, src)
			if s, ok := gs.font.textFor(code); ok {
				text = append(text, s...)
			} else {
				run.decoded = false
			}
			tx := w0/1000*gs.size + gs.tc
			if len(code) == 1 && code[0] == ' ' {
				tx += gs.tw
			}
			tx *= gs.th
			*tm = runTranslate(tx, 0).mul(*tm)
			advance += tx
		}
	}
	if run.codes == 0 {
		return
	}
	run.text = string(text)
	run.width = advance * math.Hypot(start[0], start[1])
	run.widthSrc = weakest
	w.runs = append(w.runs, run)
}

// widthRank orders width sources from least to most trustworthy, so a run reports its weakest.
var widthRank = map[widthSource]int{
	widthNone: 0, widthFromDefault: 1, widthFromMissing: 2, widthFromStd14: 3,
	widthFromDW: 4, widthFromW: 5, widthFromWidths: 5,
}

func weakerWidthSource(a, b widthSource) widthSource {
	if a == "" {
		return b
	}
	if widthRank[b] < widthRank[a] {
		return b
	}
	return a
}

// fontFor resolves a font resource, cached by object number.
func (w *runWalker) fontFor(res types.Dict, name string) *runFont {
	if res == nil {
		return nil
	}
	fonts, err := w.xt.DereferenceDict(res["Font"])
	if err != nil || fonts == nil {
		return nil
	}
	obj, ok := fonts[name]
	if !ok {
		return nil
	}
	if ir, isRef := obj.(types.IndirectRef); isRef {
		key := ir.ObjectNumber.Value()
		if f, cached := w.fonts[key]; cached {
			return f
		}
		f := loadRunFont(w.xt, obj)
		w.fonts[key] = f
		return f
	}
	return loadRunFont(w.xt, obj)
}

// drawForm walks a form XObject at its /Matrix, under its own resources where it has them.
func (w *runWalker) drawForm(res types.Dict, name string, gs runGState, depth int, visiting map[int]bool) bool {
	if res == nil {
		return false
	}
	xobjs, err := w.xt.DereferenceDict(res["XObject"])
	if err != nil || xobjs == nil {
		return false
	}
	obj, ok := xobjs[name]
	if !ok {
		return false
	}
	sd, _, serr := w.xt.DereferenceStreamDict(obj)
	if serr != nil || sd == nil {
		return false
	}
	if st := sd.Dict.NameEntry("Subtype"); st == nil || *st != "Form" {
		return false
	}
	// From here it IS a form, whether or not it is walked: a sequence around it tags what it draws.
	key := -1
	if ir, isRef := obj.(types.IndirectRef); isRef {
		key = ir.ObjectNumber.Value()
		if visiting[key] {
			return true
		}
	}
	if depth >= maxFormDepth {
		return true
	}
	body := streamContent(sd)
	if body == nil {
		return true
	}
	m := runIdentity
	if arr, aerr := w.xt.DereferenceArray(sd.Dict["Matrix"]); aerr == nil && len(arr) == 6 {
		for i, o := range arr {
			if v, vok := pdfNumber(w.xt, o); vok {
				m[i] = v
			}
		}
	}
	formRes := res
	if r, rerr := w.xt.DereferenceDict(sd.Dict["Resources"]); rerr == nil && r != nil {
		formRes = r
	}
	if key >= 0 {
		visiting[key] = true
		defer delete(visiting, key)
	}
	// The stream identity is saved and restored around the recursion for the same reason `visiting`
	// is: what comes back out is the caller's stream, not the callee's. `key` is -1 for a form held
	// as a direct stream, and that becomes 0 — see `textRun.stm` for why unnameable and
	// page's-own are the same answer here.
	outer := w.stm
	if key >= 0 {
		w.stm = key
	} else {
		w.stm = 0
	}
	defer func() { w.stm = outer }()
	gs.ctm = m.mul(gs.ctm)
	w.walk(body, formRes, gs, depth+1, visiting)
	return true
}

// streamContent returns a stream's decoded bytes, decoding it if nothing has yet.
func streamContent(sd *types.StreamDict) []byte {
	if sd.Content != nil {
		return sd.Content
	}
	if err := sd.Decode(); err != nil {
		return nil
	}
	return sd.Content
}

// matchingClose returns the index of the token closing the one at i, or len(toks) when the stream ends
// first — a malformed stream still tokenizes, and so still walks. Callers slice `toks[i+1:end]` and
// resume at `end`, which with len(toks) takes the rest and stops.
//
// **It returned len(toks)-1 until P08.S04, and that panicked** when the opener was itself the last
// token: `toks[i+1:len-1]` is a reversed slice. The array branch had that exposure from S02; the
// truncation test found it only once the dictionary branch began slicing too, because nib's own
// Markdown draws no `TJ` array for a truncation to cut through.
func matchingClose(toks []contentstream.Token, i int, open, close contentstream.Kind) int {
	depth := 0
	for j := i; j < len(toks); j++ {
		switch toks[j].Kind {
		case open:
			depth++
		case close:
			depth--
			if depth == 0 {
				return j
			}
		}
	}
	return len(toks)
}

// splitCodes cuts shown bytes into character codes: two bytes under Identity, otherwise one.
func splitCodes(f *runFont, b []byte) [][]byte {
	n := 1
	if f != nil && f.twoByte {
		n = 2
	}
	out := make([][]byte, 0, len(b)/n+1)
	for i := 0; i < len(b); i += n {
		end := i + n
		if end > len(b) {
			end = len(b)
		}
		out = append(out, b[i:end])
	}
	return out
}

func codeValue(c []byte) int {
	v := 0
	for _, b := range c {
		v = v<<8 | int(b)
	}
	return v
}

// decodePDFName decodes a name's `#xx` escapes.
func decodePDFName(b []byte) string {
	out := make([]byte, 0, len(b))
	for i := 0; i < len(b); i++ {
		if b[i] == '#' && i+2 < len(b) {
			hi, hok := hexNibble(b[i+1])
			lo, lok := hexNibble(b[i+2])
			if hok && lok {
				out = append(out, hi<<4|lo)
				i += 2
				continue
			}
		}
		out = append(out, b[i])
	}
	return string(out)
}

// decodePDFString returns a literal or hex string token's bytes, delimiters and escapes removed.
//
// `contentstream` deliberately hands out spans, not values — *"a caller that does need one decodes
// the span itself"* — so this is the reader's, and nothing in the tokenizer changes for it.
func decodePDFString(raw []byte) []byte {
	if len(raw) == 0 {
		return nil
	}
	if raw[0] == '<' {
		return decodeHexString(raw)
	}
	if raw[0] != '(' {
		return nil
	}
	body := raw[1:]
	if n := len(body); n > 0 && body[n-1] == ')' {
		body = body[:n-1]
	}
	out := make([]byte, 0, len(body))
	for i := 0; i < len(body); i++ {
		c := body[i]
		if c == '\r' {
			// An unescaped end-of-line in a literal is read as a single newline, whatever its spelling.
			out = append(out, '\n')
			if i+1 < len(body) && body[i+1] == '\n' {
				i++
			}
			continue
		}
		if c != '\\' {
			out = append(out, c)
			continue
		}
		i++
		if i >= len(body) {
			break
		}
		switch e := body[i]; e {
		case 'n':
			out = append(out, '\n')
		case 'r':
			out = append(out, '\r')
		case 't':
			out = append(out, '\t')
		case 'b':
			out = append(out, '\b')
		case 'f':
			out = append(out, '\f')
		case '(', ')', '\\':
			out = append(out, e)
		case '\r':
			// A backslash at the end of a line continues the string; neither byte is content.
			if i+1 < len(body) && body[i+1] == '\n' {
				i++
			}
		case '\n':
		default:
			if e >= '0' && e <= '7' {
				v := int(e - '0')
				for n := 1; n < 3 && i+1 < len(body) && body[i+1] >= '0' && body[i+1] <= '7'; n++ {
					i++
					v = v*8 + int(body[i]-'0')
				}
				out = append(out, byte(v))
				continue
			}
			// An unknown escape: the specification says the backslash is ignored.
			out = append(out, e)
		}
	}
	return out
}

func decodeHexString(raw []byte) []byte {
	body := raw[1:]
	if n := len(body); n > 0 && body[n-1] == '>' {
		body = body[:n-1]
	}
	nibbles := make([]byte, 0, len(body))
	for _, c := range body {
		if v, ok := hexNibble(c); ok {
			nibbles = append(nibbles, v)
		}
	}
	if len(nibbles)%2 == 1 {
		nibbles = append(nibbles, 0) // a missing final digit is zero
	}
	out := make([]byte, len(nibbles)/2)
	for i := range out {
		out[i] = nibbles[2*i]<<4 | nibbles[2*i+1]
	}
	return out
}

func hexNibble(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}

// cmapItem is one operand inside a `bfchar`/`bfrange` block.
type cmapItem struct {
	b     []byte
	arr   [][]byte
	isArr bool
}

// parseToUnicode reads a `/ToUnicode` CMap's `bfchar` and `bfrange` blocks — both range forms, the
// incrementing destination and the array of destinations — into code bytes → text.
//
// # A total budget, not only a per-range one (`/pending 503`)
//
// `maxToUnicodeRange` bounds ONE range, and one range is not the cost: a 2.2 KB CMap of a hundred
// overlapping full-plane ranges expanded 6.5 million codes into 65,536 entries, 5.1 s and 110 MB, reached
// from `ProposeTags` and `CommitTags` on any page drawn in that font. So every range spends from
// `maxToUnicodeExpansion`, and a range that does not fit what is left is not expanded — its codes read as
// undecoded, which the run already reports, rather than as a stall.
func parseToUnicode(src []byte) map[string]string {
	out := map[string]string{}
	budget := maxToUnicodeExpansion
	toks := contentstream.Tokenize(src)
	mode := ""
	var items []cmapItem
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		switch t.Kind {
		case contentstream.Whitespace:
			continue
		case contentstream.HexString:
			if mode != "" {
				items = append(items, cmapItem{b: decodeHexString(t.Bytes(src))})
			}
			continue
		case contentstream.ArrayOpen:
			end := matchingClose(toks, i, contentstream.ArrayOpen, contentstream.ArrayClose)
			if mode != "" {
				var arr [][]byte
				for _, at := range toks[i+1 : end] {
					if at.Kind == contentstream.HexString {
						arr = append(arr, decodeHexString(at.Bytes(src)))
					}
				}
				items = append(items, cmapItem{arr: arr, isArr: true})
			}
			i = end
			continue
		}
		switch string(t.Bytes(src)) {
		case "beginbfchar":
			mode, items = "char", nil
		case "beginbfrange":
			mode, items = "range", nil
		case "endbfchar":
			for j := 0; j+1 < len(items); j += 2 {
				if !items[j].isArr && !items[j+1].isArr {
					out[string(items[j].b)] = utf16Text(items[j+1].b)
				}
			}
			mode, items = "", nil
		case "endbfrange":
			for j := 0; j+2 < len(items); j += 3 {
				lo, hi := items[j], items[j+1]
				if lo.isArr || hi.isArr || len(lo.b) != len(hi.b) {
					continue
				}
				expandBFRange(out, lo.b, hi.b, items[j+2], &budget)
			}
			mode, items = "", nil
		}
	}
	return out
}

func expandBFRange(out map[string]string, lo, hi []byte, dst cmapItem, budget *int) {
	l, h := codeValue(lo), codeValue(hi)
	if h < l || h-l >= maxToUnicodeRange || h-l+1 > *budget {
		return
	}
	*budget -= h - l + 1
	for k := 0; k <= h-l; k++ {
		code := make([]byte, len(lo))
		v := l + k
		for b := len(code) - 1; b >= 0; b-- {
			code[b] = byte(v)
			v >>= 8
		}
		if dst.isArr {
			if k < len(dst.arr) {
				out[string(code)] = utf16Text(dst.arr[k])
			}
			continue
		}
		d := append([]byte(nil), dst.b...)
		switch {
		case len(d) >= 2:
			u := int(d[len(d)-2])<<8 | int(d[len(d)-1])
			u += k
			d[len(d)-2], d[len(d)-1] = byte(u>>8), byte(u)
		case len(d) == 1:
			d[0] += byte(k)
		}
		out[string(code)] = utf16Text(d)
	}
}

// utf16Text decodes a CMap destination, which is UTF-16BE and may carry surrogate pairs or several
// characters (a ligature maps to two).
func utf16Text(b []byte) string {
	if len(b)%2 == 1 {
		return string(b)
	}
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = uint16(b[2*i])<<8 | uint16(b[2*i+1])
	}
	return string(utf16.Decode(u))
}
