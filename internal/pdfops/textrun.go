package pdfops

import (
	"fmt"
	"math"
	"reflect"
	"sort"
	"strconv"

	"github.com/pdfcpu/pdfcpu/pkg/font"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
	"golang.org/x/text/encoding/charmap"

	"nib/internal/contentstream"
	"nib/internal/fontcode"
	"nib/internal/pdfread"
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
	// replaced is whether the run was drawn inside a sequence carrying replacement text (`mcFrame.replaced`):
	// readers report that text instead of these glyphs, so rewriting the glyphs alone leaves the old words readable.
	replaced bool
	// propsWithoutMCID is whether the run was drawn inside a sequence opened with a property list and no MCID — an
	// optional-content membership (`/OC /oc1 BDC`), a `/Span <</Lang (fr)>>` — at any depth (`mcFrame.props`). A carry to
	// another page re-opens only the sequence carrying the run's MCID, so it would draw the run outside that list: a
	// hidden layer drawn unconditionally, a language lost (P07 phase-close review).
	propsWithoutMCID bool
	// rotated is whether the run's baseline is not upright left-to-right in user space — turned, vertical or
	// mirrored (`baselineTurns`). Grouping measures lines as horizontal baselines, so it reports a page
	// carrying one rather than reading it as upright (`/pending 503`).
	rotated bool
	// glyphs are the run's glyphs in order, and kernAfter a `TJ` adjustment after the last of them, in user
	// space — kept only when the walker is asked (`readPageGlyphRuns`, `PLAN-text-reflow.md` P06.S01): reflow
	// needs each glyph's code and advance, and no other reader does. `Σ(kern + advance) + kernAfter == width`.
	glyphs    []runGlyph
	kernAfter float64
	// face is the font the run was drawn in, kept with the glyphs so reflow can ask it what it can draw (`codesFor`).
	face *runFont
	// state is the text state the run was shown under — kept with the glyphs, for reflow to re-emit text where the run
	// was and to restore what it leaves behind (P06.S04).
	state runTextState
}

// runTextState is the text state at a show operator: the text and line matrices as the operator began (text space,
// before the CTM), the `Tf` size, character and word spacing, horizontal scaling, rise, and the scale from text space
// to user space along the baseline.
type runTextState struct {
	tm, tlm runMatrix
	ctm     runMatrix // the CTM the run was drawn under: text space reaches user space through tm·ctm (P07.S02)
	// fill, stroke, extGState and clip are the graphics state the run was drawn in (`runGState`), for a paragraph set on
	// another page (P07.S05).
	fill, stroke   string
	extGState      bool
	clip           [4]float64
	tfSize, tc, tw float64
	th, ts, scale  float64
	tr             int // the text rendering mode: 3 draws nothing, 7 only clips
}

// runGlyph is one glyph of a run, as the show operator drew it.
type runGlyph struct {
	code    []byte // the character code's bytes, as the operand carried them
	text    string // what the code decodes to; "" with decoded false when it decodes to nothing
	decoded bool
	// fontWidth is the font's own advance for the code, in glyph space (thousandths of an em), and widthSrc
	// where it came from. A `none` source carries fontWidth 0 and says so — law 2: never 0 read as known.
	fontWidth float64
	widthSrc  widthSource
	// kern is the `TJ` adjustment immediately before this glyph, and advance the glyph's own advance —
	// character and word spacing and horizontal scaling included — both in user space.
	kern, advance float64
}

// baselineTurns reports whether text drawn under m runs anywhere but rightward along +x: the text-space x
// axis mapped to user space points more than a degree away. A skew (`c`) leaves the baseline flat and is
// not a turn; a page's `/Rotate` is not in the content matrix and turns every line alike, which grouping
// in unrotated space reads consistently.
func baselineTurns(m runMatrix) bool {
	return math.Abs(math.Atan2(m[1], m[0])) > math.Pi/180
}

// inReplacement reports whether a sequence carrying replacement text is open at this point of the walk.
func (w *runWalker) inReplacement() bool {
	n := len(w.mcStack)
	return n > 0 && w.mcStack[n-1].replaced
}

// inPropsWithoutMCID reports whether a sequence opened with a property list and no MCID is open at this point of the walk.
func (w *runWalker) inPropsWithoutMCID() bool {
	n := len(w.mcStack)
	return n > 0 && w.mcStack[n-1].props
}

// inArtifact reports whether an `/Artifact` sequence is open at this point of the walk.
func (w *runWalker) inArtifact() bool {
	n := len(w.mcStack)
	return n > 0 && w.mcStack[n-1].artifact
}

// pageRuns is a page read as text.
type pageRuns struct {
	runs []textRun
	// noText is true when the page draws no glyph at all, directly or through a form XObject — an
	// image-only or blank page. Said structurally, so a scan is not left to be inferred from an empty
	// slice; a reader failure is an error, never this. A show operator with an empty string draws
	// nothing and counts as nothing: counting operators instead was a distinction no caller reads.
	noText bool
	// marks are the page's non-text marks, boxed — kept only by reflow's reader (`PLAN-text-reflow.md` P07.S01).
	marks []pageMark
	// sequences are the page's marked-content sequences that carry an MCID, in the order they open —
	// those inside the forms it draws included — so a writer can find an element's content whether or
	// not it is text (P09.S03).
	sequences []markedSeq
}

// maxFormDepth bounds form XObject recursion. A form may draw itself; the specification forbids it and
// documents do it anyway.
const maxFormDepth = 12

// readPageRuns reads one page's positioned runs. The caller resolves the page (`pageAt`), so a loop over the
// pages walks the page tree once rather than once per page (/pending 756).
//
// shared is the budget a LOOP over pages passes, made once for the pages it reads (`newFormWalkBudget(pages)`) —
// `/pending 715`. Without it the page gets a budget of its own, which is right for one page and wrong for a loop:
// N pages that each stay just under a page's budget cost N budgets (measured: 64 pages sharing one fan-out form,
// 9.4 s, ~0.14 s a page for ~100 bytes of file a page), where the budget's own rule is one per document with an
// allowance per page. `TestEveryPageLoopSharesOneWalkBudget` holds every loop to it.
func readPageRuns(ctx *model.Context, pg pdfread.Page, shared ...*formWalkBudget) (pageRuns, error) {
	return readPageRunsKeeping(ctx, pg, false, pageBudget(shared))
}

// pageBudget is the shared budget when a loop passed one, else a budget for this page alone.
func pageBudget(shared []*formWalkBudget) *formWalkBudget {
	if len(shared) > 0 && shared[0] != nil {
		return shared[0]
	}
	return newFormWalkBudget(1)
}

// readPageGlyphRuns is readPageRuns with each run's glyphs kept (`textRun.glyphs`), and the page's non-text marks
// (`pageRuns.marks`) — reflow's reader.
func readPageGlyphRuns(ctx *model.Context, pg pdfread.Page) (pageRuns, error) {
	return readPageRunsKeeping(ctx, pg, true, newFormWalkBudget(1))
}

func readPageRunsKeeping(ctx *model.Context, pg pdfread.Page, keep bool, budget *formWalkBudget) (pageRuns, error) {
	pageNr := pg.Nr
	return containRunRead(pageNr, func() (pageRuns, error) {
		d, attrs, err := pg.Dict, pg.Attrs, pg.Err
		if err != nil || d == nil {
			return pageRuns{}, fmt.Errorf("pdfops: page %d does not resolve: %w", pageNr, err)
		}
		content, cerr := pdfread.PageContent(ctx, d, pageNr)
		if cerr != nil && cerr != model.ErrNoContent {
			return pageRuns{}, cerr
		}
		var res types.Dict
		if attrs != nil {
			res = attrs.Resources
		}
		w := newRunWalker(ctx.XRefTable)
		budget.nextPage()
		w.budget = budget
		w.keepGlyphs, w.keepMarks = keep, keep
		w.walk(content, res, newRunGState(), 0, map[int]bool{})
		// Runs from a walk that stopped are the runs of part of the page: every caller would read the rest
		// as absent, so the page is an error (`formWalkBudget`).
		if err := w.budget.err(); err != nil {
			return pageRuns{}, fmt.Errorf("pdfops: page %d could not be read as text: %w", pageNr, err)
		}
		return pageRuns{runs: w.runs, noText: len(w.runs) == 0, sequences: w.seqs, marks: w.marks}, nil
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
	tr       int     // `Tr`, the text rendering mode
	lw       float64 // `w`, the line width a stroke is drawn at — read only for a mark's box (P07.S01)
	// fill and stroke are the operators that set the current colours, re-emittable anywhere — when the colour is in a
	// DEVICE space; "" when it is not (a colour space, a pattern), which a paragraph moved to another page cannot carry.
	// fillSpace and strokeSpace are the device space in force for `sc`, or "" (P07.S05).
	fill, stroke           string
	fillSpace, strokeSpace string
	// extGState is whether a `gs` has applied a graphics state dictionary — alpha, blend, a soft mask — none of which is
	// carried to another page.
	extGState bool
	// clip is the box the clipping path is known to lie within, in user space: every `W` path's box intersected.
	clip [4]float64
}

func newRunGState() runGState {
	inf := math.Inf(1)
	return runGState{ctm: runIdentity, th: 1, lw: 1, fill: "0 g", stroke: "0 G", fillSpace: "DeviceGray", strokeSpace: "DeviceGray",
		clip: [4]float64{-inf, -inf, inf, inf}}
}

// runFont is what the walk needs from a font dictionary.
type runFont struct {
	baseFont string
	twoByte  bool // Identity-H or Identity-V: codes are two bytes and CID == code
	// vertical is Identity-V: glyphs advance DOWN the page, and a reader measuring them along the baseline is wrong.
	vertical bool
	// splittable is false for a Type0 font under a CMap this reader does not parse: its code lengths
	// are unknown, so neither its text nor its widths can be read without guessing.
	splittable bool
	widths     fontWidths
	toUni      map[string]string
	simple     func(byte) (rune, bool)
	// drawing is textFor inverted — text to the codes that draw it — built on first use by `codesFor`.
	drawing map[string][][]byte
}

// codesFor answers which codes of this font draw text — `PLAN-text-reflow.md` P06.S02, D8's trigger. It is `textFor`
// run backwards over every code the font can express (the ToUnicode keys; every byte, for a simple font), so it can
// never disagree with what the reader decodes: one rule, one door (law 4). Nil means the font draws no such text —
// ABSENT, and reflow falls back naming it. More than one code is returned as more than one: two codes drawing the same
// character is a fact about the font, and choosing between them is the caller's, never guessed here.
//
// A font this reader cannot split (a Type0 under a CMap it does not parse) draws nothing it can vouch for — `textFor`
// refuses every code of one, so its inverse is empty.
func (f *runFont) codesFor(text string) [][]byte {
	if f == nil {
		return nil
	}
	if f.drawing == nil {
		f.drawing = map[string][][]byte{}
		add := func(code []byte) {
			if t, ok := f.textFor(code); ok && t != "" {
				f.drawing[t] = append(f.drawing[t], code)
			}
		}
		keys := make([]string, 0, len(f.toUni))
		for k := range f.toUni {
			keys = append(keys, k)
		}
		// Codes come back in code order, so the answer is a property of the font, not of map order.
		sort.Strings(keys)
		// A code is as long as `splitCodes` cuts it; a ToUnicode key of another length is never drawn.
		want := 1
		if f.twoByte {
			want = 2
		}
		for _, k := range keys {
			if len(k) == want {
				add([]byte(k))
			}
		}
		if !f.twoByte {
			for b := 0; b < 256; b++ {
				if _, mapped := f.toUni[string([]byte{byte(b)})]; !mapped {
					add([]byte{byte(b)})
				}
			}
		}
	}
	return f.drawing[text]
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
			f.vertical = *enc == "Identity-V"
		} else {
			f.splittable = false
		}
	}
	if tu, ok := d["ToUnicode"]; ok {
		if sd, _, serr := xt.DereferenceStreamDict(tu); serr == nil && sd != nil {
			if body := streamContent(sd); body != nil {
				f.toUni = fontcode.TextMap(body)
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
	xt *model.XRefTable
	// keepGlyphs asks `show` to record each run's glyphs (P06.S01). Off for every reader but reflow's.
	keepGlyphs bool
	// keepMarks asks the walk to record every non-text mark with its box (`pageMark`, P07.S01). Off for every reader but
	// reflow's.
	keepMarks bool
	marks     []pageMark
	// fonts holds every font the walk has loaded, keyed by its object number when the resource names an
	// indirect font and by the dictionary's identity when it is a direct one (`/pending 723`): a direct
	// font reloaded at every `Tf` re-parsed its `/ToUnicode` each time, 17.7 s for 14.4 KB of content.
	// Identity, never the resource NAME, because a form's own `/Resources` may bind `/F1` to another
	// font. `internal/uacheck`'s `glyphFontFor` keys its cache the same way.
	fonts map[any]*runFont
	// fontLoads counts `loadRunFont` calls — the observable the cache is measured by.
	fontLoads int
	runs      []textRun
	// mcStack is the marked-content sequences open at this point of the walk, innermost last. Shared
	// across a page and the forms it draws, because a `BDC` around a `Do` tags what the form draws.
	// Pushed only through `push`, which carries what the questions asked per show need, so none of them
	// scans the stack (`/pending 664`: N nested sequences and N shows had cost N² — 80k of each, 8.2 s).
	mcStack []mcFrame
	// seqs are the MCID-carrying sequences seen so far; seqOpen parallels mcStack with each open
	// sequence's index into seqs, or -1 for one that carries no MCID.
	seqs    []markedSeq
	seqOpen []int
	// stm is the object number of the stream currently being walked, 0 for the page's own. `drawForm`
	// saves and restores it around its recursion exactly as it does `visiting`, so it is always the
	// stream the token under the cursor came from rather than the one the walk started in.
	stm int
	// budget bounds the forms the walk enters (`formWalkBudget`); past it the page is an error.
	budget *formWalkBudget
}

// mcFrame is one open marked-content sequence, with the answers about the stack at and below it.
type mcFrame struct {
	v int // the sequence's MCID, -1 for one with none, or mcArtifact
	// force is the index of the innermost frame at or below this one whose v is not -1 — the frame that
	// decides the MCID and stream in force — or -1.
	force int
	// artifact is whether this frame or any below it is an `/Artifact` sequence.
	artifact bool
	// seqAt is the index of the innermost frame at or below this one that opened an entry in `seqs`, or
	// -1; the frame below seqAt holds the next one out.
	seqAt int
	// replaced is whether this frame or any below it carries replacement text — `/ActualText`, `/Alt` or `/E` in
	// its property list, written inline or named in `/Properties` — which readers report INSTEAD of the glyphs.
	replaced bool
	// props is whether this frame or any below it is a `BDC` whose property list carries no MCID (and is not an
	// `/Artifact`'s): content a carry would take out of it (`textRun.propsWithoutMCID`).
	props bool
}

// push opens a sequence: v as `mcFrame.v`, and seq its index into `seqs` or -1. `mcStack` and `seqOpen`
// move in lockstep, so this is the only place either grows.
func (w *runWalker) push(v, seq int, replaced, props bool) {
	f := mcFrame{v: v, force: -1, seqAt: -1, replaced: replaced, props: props}
	i := len(w.mcStack)
	if i > 0 {
		below := w.mcStack[i-1]
		f.force, f.artifact, f.seqAt = below.force, below.artifact, below.seqAt
		f.replaced = f.replaced || below.replaced
		f.props = f.props || below.props
	}
	if v != -1 {
		f.force = i
	}
	if v == mcArtifact {
		f.artifact = true
	}
	if seq >= 0 {
		f.seqAt = i
	}
	w.mcStack = append(w.mcStack, f)
	w.seqOpen = append(w.seqOpen, seq)
}

// markDrawsForm records that a form XObject is drawn inside every sequence open here. Walked outward from
// the innermost and stopped at the first already marked: a sequence is marked only together with every
// sequence enclosing it, so everything further out is marked already, and each sequence is marked once.
func (w *runWalker) markDrawsForm() {
	n := len(w.mcStack)
	if n == 0 {
		return
	}
	for i := w.mcStack[n-1].seqAt; i >= 0; {
		j := w.seqOpen[i]
		if w.seqs[j].drawsForm {
			return
		}
		w.seqs[j].drawsForm = true
		if i == 0 {
			return
		}
		i = w.mcStack[i-1].seqAt
	}
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
	n := len(w.mcStack)
	if n == 0 || w.mcStack[n-1].force < 0 {
		return -1
	}
	if v := w.mcStack[w.mcStack[n-1].force].v; v >= 0 {
		return v
	}
	return -1 // an artifact is in force
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
	n := len(w.mcStack)
	if n == 0 {
		return 0
	}
	i := w.mcStack[n-1].force
	if i < 0 || w.mcStack[i].v < 0 {
		return 0 // nothing in force, or an artifact
	}
	// `seqOpen` is pushed and popped in lockstep with `mcStack` (`push`), so the same index is the same
	// sequence; the bound is belt-and-braces against an unbalanced stream desyncing them.
	if i < len(w.seqOpen) {
		if j := w.seqOpen[i]; j >= 0 && j < len(w.seqs) {
			return w.seqs[j].stm
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

// carriesReplacementText reports whether a `BDC` property list — inline, or named in the resources' `/Properties` —
// carries `/ActualText`, `/Alt` or `/E`: text a reader reports in place of the glyphs the sequence draws (ISO 32000-1
// §14.9.3–14.9.5). The same two spellings `markedContentID` reads, so the two answers are about one list.
func (w *runWalker) carriesReplacementText(o runOperand, res types.Dict, src []byte) bool {
	isKey := func(k string) bool { return k == "ActualText" || k == "Alt" || k == "E" }
	if o.opaque {
		// Keys and values alternate at depth 0; only a name in KEY position is a key — `/Foo /Alt` is a value.
		depth, atKey := 0, true
		for _, t := range o.dict {
			switch t.Kind {
			case contentstream.Whitespace:
				continue
			case contentstream.DictOpen, contentstream.ArrayOpen:
				depth++
				continue
			case contentstream.DictClose, contentstream.ArrayClose:
				if depth--; depth == 0 {
					atKey = true // a composite value just closed
				}
				continue
			}
			if depth != 0 {
				continue
			}
			if atKey {
				if b := t.Bytes(src); len(b) > 1 && b[0] == '/' && isKey(fontcode.Name(b[1:])) {
					return true
				}
			}
			atKey = !atKey
		}
		return false
	}
	name, ok := o.name(src)
	if !ok || res == nil {
		return false
	}
	props, err := w.xt.DereferenceDict(res["Properties"])
	if err != nil || props == nil {
		return false
	}
	d, derr := w.xt.DereferenceDict(props[name])
	if derr != nil || d == nil {
		return false
	}
	for k := range d {
		if isKey(k) {
			return true
		}
	}
	return false
}

func newRunWalker(xt *model.XRefTable) *runWalker {
	return &runWalker{xt: xt, fonts: map[any]*runFont{}, budget: newFormWalkBudget(1)}
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
	return fontcode.Name(b[1:]), true
}

func (o runOperand) str(src []byte) ([]byte, bool) {
	if o.isArr || o.opaque {
		return nil, false
	}
	if o.tok.Kind != contentstream.LiteralString && o.tok.Kind != contentstream.HexString {
		return nil, false
	}
	return fontcode.String(o.tok.Bytes(src)), true
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
	// path is the box of the path under construction, in user space; a painting operator records it and every path
	// operator that ends one (`n` included) clears it. clipping says a `W` marked it as the next clip.
	var path markBox
	clipping := false
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
			w.show(&tm, tlm, gs, []tjPiece{{codes: s}}, opSpan{os[0].start, end}, depth > 0)
		}
	}

	for i := 0; i < len(toks); i++ {
		tok := toks[i]
		switch tok.Kind {
		case contentstream.InlineImage:
			w.markImage(markInlineImage, gs)
			continue
		case contentstream.Whitespace:
			continue
		case contentstream.ArrayOpen:
			end := fontcode.MatchingClose(toks, i, contentstream.ArrayOpen, contentstream.ArrayClose)
			ops = append(ops, runOperand{arr: toks[i+1 : end], isArr: true, start: tok.Start})
			i = end
			continue
		case contentstream.DictOpen:
			end := fontcode.MatchingClose(toks, i, contentstream.DictOpen, contentstream.DictClose)
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
		case "w":
			if v, ok := numbers(1); ok {
				gs.lw = v[0]
			}
		case "m", "l":
			if v, ok := numbers(2); ok {
				path.add(gs.ctm, v...)
			}
		case "c":
			// A Bézier curve lies inside its control polygon's hull, so the control points bound it.
			if v, ok := numbers(6); ok {
				path.add(gs.ctm, v...)
			}
		case "v", "y":
			if v, ok := numbers(4); ok {
				path.add(gs.ctm, v...)
			}
		case "re":
			if v, ok := numbers(4); ok {
				path.add(gs.ctm, v[0], v[1], v[0]+v[2], v[1], v[0], v[1]+v[3], v[0]+v[2], v[1]+v[3])
			}
		case "S", "s", "B", "B*", "b", "b*":
			w.markPath(path, gs, true)
			gs.clip, clipping = clipTo(gs.clip, path, clipping)
			path = markBox{}
		case "f", "F", "f*":
			w.markPath(path, gs, false)
			gs.clip, clipping = clipTo(gs.clip, path, clipping)
			path = markBox{}
		case "n":
			gs.clip, clipping = clipTo(gs.clip, path, clipping)
			path = markBox{}
		case "W", "W*":
			clipping = true
		case "gs":
			gs.extGState = true
		case "g", "rg", "k", "G", "RG", "K":
			// The colour is read only for a reader that re-emits it (a paragraph set on another page, P07.S05): every other
			// reader walks past it, as it walks past every mark (P07 phase-close review — measured +0.75-1 µs per colour
			// operator in every reader before it was gated).
			if !w.keepGlyphs && !w.keepMarks {
				break
			}
			op := string(tok.Bytes(src))
			n := deviceColourOps[op]
			space := deviceSpaceOf[n]
			set := ""
			if v, ok := numbers(n); ok {
				set = colourOp(v, op)
			}
			if op == "g" || op == "rg" || op == "k" {
				gs.fill, gs.fillSpace = set, space
			} else {
				gs.stroke, gs.strokeSpace = set, space
			}
		case "cs", "CS":
			if !w.keepGlyphs && !w.keepMarks {
				break
			}
			op := string(tok.Bytes(src))
			space := ""
			if os := last(1); os != nil {
				if name, ok := os[0].name(src); ok && deviceComponents[name] > 0 {
					space = name
				}
			}
			// Setting a colour space sets its initial colour: black, in every device space.
			set := ""
			if space != "" {
				set = "/" + space + " " + op + " " + initialDeviceColour[space] + " " + colourSetterOf[op]
			}
			if op == "cs" {
				gs.fill, gs.fillSpace = set, space
			} else {
				gs.stroke, gs.strokeSpace = set, space
			}
		case "sc", "scn", "SC", "SCN":
			if !w.keepGlyphs && !w.keepMarks {
				break
			}
			op := string(tok.Bytes(src))
			fill := op == "sc" || op == "scn"
			space := gs.strokeSpace
			if fill {
				space = gs.fillSpace
			}
			set := ""
			if n := deviceComponents[space]; n > 0 {
				if v, ok := numbers(n); ok && len(ops) == n {
					set = "/" + space + " " + spaceOp[fill] + " " + colourOp(v, colourSetterOf[spaceOp[fill]])
				}
			}
			if fill {
				gs.fill = set
			} else {
				gs.stroke = set
			}
		case "sh":
			w.markShading()
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
		case "Tr":
			if v, ok := numbers(1); ok {
				gs.tr = int(v[0])
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
						pieces = append(pieces, tjPiece{codes: fontcode.String(at.Bytes(src))})
					case contentstream.Operand:
						if v, err := strconv.ParseFloat(string(at.Bytes(src)), 64); err == nil {
							pieces = append(pieces, tjPiece{adjust: v, isAdjust: true})
						}
					}
				}
				w.show(&tm, tlm, gs, pieces, opSpan{os[0].start, tok.End}, depth > 0)
			}
		case "BMC":
			entry := -1
			if os := last(1); os != nil {
				if tag, ok := os[0].name(src); ok && tag == "Artifact" {
					entry = mcArtifact
				}
			}
			w.push(entry, -1, false, false)
		case "BDC":
			entry, opener, replaced := -1, -1, false
			if os := last(2); os != nil {
				if tag, ok := os[0].name(src); ok && tag == "Artifact" {
					entry = mcArtifact
				} else {
					entry = w.markedContentID(os[1], res, src)
				}
				replaced = w.carriesReplacementText(os[1], res, src)
				opener = os[0].start
			}
			seq := -1
			if entry >= 0 && opener >= 0 {
				w.seqs = append(w.seqs, markedSeq{mcid: entry, opener: opSpan{opener, tok.End},
					inForm: depth > 0, stm: w.stm})
				seq = len(w.seqs) - 1
			}
			// A property list with no MCID: `entry` is -1 only for a `BDC` that is not an `/Artifact` and names none.
			w.push(entry, seq, replaced, opener >= 0 && entry == -1)
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
				if name, ok := os[0].name(src); ok {
					if w.drawForm(res, name, gs, depth, visiting) {
						w.markDrawsForm()
					} else if w.drawsImage(res, name) {
						w.markImage(markImage, gs)
					}
				}
			}
		}
		ops = ops[:0]
	}
}

// show advances the text matrix across one show operator's glyphs and records the run.
func (w *runWalker) show(tm *runMatrix, tlm runMatrix, gs runGState, pieces []tjPiece, span opSpan, inForm bool) {
	textAt := *tm
	start := tm.mul(gs.ctm)
	run := textRun{font: gs.fontName, size: gs.size * math.Hypot(start[2], start[3]), decoded: true,
		mcid: w.currentMCID(), span: span, inForm: inForm, stm: w.currentStm(), artifact: w.inArtifact(),
		replaced: w.inReplacement(), propsWithoutMCID: w.inPropsWithoutMCID()}
	if gs.font != nil {
		run.baseFont = gs.font.baseFont
	}
	run.x, run.y = start.apply(0, gs.ts)
	run.rotated = baselineTurns(start)
	var text []byte
	var advance, pendingKern float64
	scale := math.Hypot(start[0], start[1]) // text space → user space along the baseline
	weakest := widthSource("")
	for _, p := range pieces {
		if p.isAdjust {
			tx := -p.adjust / 1000 * gs.size * gs.th
			*tm = runTranslate(tx, 0).mul(*tm)
			advance += tx
			pendingKern += tx
			continue
		}
		for _, code := range splitCodes(gs.font, p.codes) {
			run.codes++
			w0, src := 0.0, widthNone
			if gs.font != nil && gs.font.splittable {
				w0, src = gs.font.widths.advance(fontcode.Value(code))
			}
			weakest = weakerWidthSource(weakest, src)
			s, ok := gs.font.textFor(code)
			if ok {
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
			if w.keepGlyphs {
				run.glyphs = append(run.glyphs, runGlyph{code: code, text: string(s), decoded: ok, fontWidth: w0,
					widthSrc: src, kern: pendingKern * scale, advance: tx * scale})
			}
			pendingKern = 0
		}
	}
	if run.codes == 0 {
		return
	}
	if w.keepGlyphs {
		run.kernAfter = pendingKern * scale
		run.face = gs.font
		run.state = runTextState{tm: textAt, tlm: tlm, ctm: gs.ctm, fill: gs.fill, stroke: gs.stroke, extGState: gs.extGState, clip: gs.clip, tfSize: gs.size, tc: gs.tc, tw: gs.tw, th: gs.th, ts: gs.ts, scale: scale, tr: gs.tr}
	}
	run.text = string(text)
	run.width = advance * scale
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

// fontFor resolves a font resource, loaded once per font: keyed by object number for an indirect font
// and by dictionary identity for a direct one (see `runWalker.fonts`).
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
	var key any
	switch o := obj.(type) {
	case types.IndirectRef:
		key = o.ObjectNumber.Value()
	case types.Dict:
		key = reflect.ValueOf(o).Pointer()
	default:
		return nil // neither a reference nor a dictionary: loadRunFont would refuse it too
	}
	if f, cached := w.fonts[key]; cached {
		return f
	}
	w.fontLoads++
	f := loadRunFont(w.xt, obj)
	w.fonts[key] = f
	return f
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
			w.markForm(sd, gs)
			return true
		}
	}
	if !w.budget.deeper(depth, maxFormDepth) {
		w.markForm(sd, gs)
		return true
	}
	body := w.budget.formContent(sd, obj)
	if body == nil || !w.budget.enterForm(len(body)) {
		w.markForm(sd, gs)
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

// splitCodes cuts shown bytes into character codes: two bytes under Identity, otherwise one.
//
// **This is the text reader's lenient cut, not the checker's** (ADR-052): a string ending inside a two-byte code
// keeps its last byte as a one-byte code, where veraPDF — and so `fontcode.Codespace` — completes it with 0xFF and
// judges a glyph nobody drew. Reading a width and text for CID 0x41FF would be inventing one.
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
