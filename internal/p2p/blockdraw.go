package p2p

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"math"
	"sync"
	"unicode"

	"github.com/go-text/typesetting/bidi"
	"github.com/go-text/typesetting/di"
	"github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/language"
	"github.com/go-text/typesetting/shaping"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/vector"

	"nib/internal/pdfops"
)

// The signature block is drawn HERE, by the server, and nowhere else (ADR-158, /pending 367).
//
// # What this replaced
//
// The window drew the block on a canvas from the lines the server quoted and posted the picture
// back; the server embedded whatever it was handed beside a `/Reason` it built itself. Nothing
// compared the two, so a stale or modified window produced a document whose picture said one thing
// and whose signed text another — and a reader looks at the picture. Now the picture is a function
// of the attestation and of nothing else: `Contribute` calls `BlockImage`, a quote calls it for the
// preview, and no route takes a picture.
//
// # One door, three questions
//
// `shapeBlockLine` is the one place a line of block text becomes glyphs. Three things read it and
// can therefore never disagree: the WRAP (`blockLineWidth`, the advance of the face that is drawn —
// it was a Helvetica estimate of a browser's sans-serif), the REFUSAL (`BlockDrawable`, a character
// no shipped face has), and the DRAWING (`drawBlock`).
//
// # Explicit faces only
//
// The faces are the ones in the binary (`pdfops.ShippedFace`), in the fixed order of
// `blockFaceFiles`. No system font is looked for, ever: the same input must draw the same picture on
// every machine, and a request must not read a font directory.
//
// # Why a shaper is a dependency
//
// `golang.org/x/image` draws one glyph per rune, left to right. Arabic joins, the Indic scripts
// reorder and conjoin, Thai stacks its marks, and Hebrew and Arabic run right to left — measured
// before this was built: 42 runes of a mixed-script block drew as boxes. `go-text/typesetting` is a
// pure-Go HarfBuzz port with its own bidi; it is used for exactly that and its font scanner is not
// imported.
//
// # Cost, measured (`TestWhatDrawingABlockCosts`; Linux, the faces already parsed)
//
//	5 lines, Latin                          3.6 ms a block
//	7 lines, Latin                          4.5 ms
//	8 lines, Latin (the limit)              5.7 ms
//	8 lines, mixed scripts and directions   5.3 ms
//
// It was 1.4-1.9 ms unshaped. Most of what is left is the PNG encoder, not the shaper (shaping a
// five-line block is 0.35 ms). Wrapping a recital is shapings too: 1.7 ms for 200 characters, 44 ms
// for 4,096, plus 11 ms and 113 ms for the refusal's bisection — see `blockMeasureCap`. Parsing the
// fourteen faces happens once per process, at the first block.
//
// Everything here runs under one mutex: a `font.Face` and a shaper are not safe for concurrent use,
// and a block is drawn once per quote and once per signature, never in a loop over requests. Nothing
// in this file is reached while the server's own lock is held.

// blockFaceFiles is the fallback chain, in order: the first face that has a character draws it.
//
// Roboto first — it is the face a Latin, Cyrillic or Greek block is set in, and it supplies the
// digits, spaces and punctuation between runs of every other script.
var blockFaceFiles = []string{
	"Roboto-Regular",
	"NotoSansArabic-Regular",
	"NotoSansHebrew-Regular",
	"NotoSansDevanagari-Regular",
	"NotoSansBengali-Regular",
	"NotoSansGujarati-Regular",
	"NotoSansGurmukhi-Regular",
	"NotoSansKannada-Regular",
	"NotoSansMalayalam-Regular",
	"NotoSansTamil-Regular",
	"NotoSansTelugu-Regular",
	"NotoSansThai-Regular",
	"DroidSansFallbackFull",
	"NanumGothic-Regular",
}

// blockHangulFace is the face Hangul is drawn in although the CJK face before it in the chain has
// part of the block: Droid's Hangul is incomplete, and a name set half in one face and half in
// another reads as two names.
const blockHangulFace = "NanumGothic-Regular"

// blockShapeSize is the size every line is shaped at, in pixels per em. The shaper rounds a size UP
// to a whole pixel, so shaping at the drawn size would measure a 6.65pt line as a 7pt one; one large
// fixed size, scaled afterwards, makes an advance exact at any size and the same at every size.
const blockShapeSize = 2048

const (
	blockScale  = 3 // picture pixels per PDF point
	blockPadPt  = 4 // inner padding, points — `blockTextWidth` subtracts the same figure
	blockBorder = 2 // frame, pixels
	// blockLineShare is the share of a line's slot the text's em takes; with `blockTextPt` it gives the
	// sizes `maxBlockLines` records (9pt through six lines, 6.65pt at eight).
	blockLineShare = 0.7
)

type blockFace struct {
	file      string
	face      *font.Face
	upem      float32
	asc, desc float32 // the face's own ascender and descender, in ems (both positive)
}

var blockFaces struct {
	mu     sync.Mutex
	loaded bool
	err    error
	chain  []*blockFace
	hangul *blockFace
	shaper shaping.HarfbuzzShaper
	seg    shaping.Segmenter
	para   bidi.Paragraph
}

// loadBlockFaces parses the chain once. The caller holds blockFaces.mu.
func loadBlockFaces() error {
	if blockFaces.loaded {
		return blockFaces.err
	}
	blockFaces.loaded = true
	for _, file := range blockFaceFiles {
		bb, err := pdfops.ShippedFace(file)
		if err != nil {
			blockFaces.err = fmt.Errorf("the signature block's typeface %s is missing from this build: %w", file, err)
			return blockFaces.err
		}
		f, err := font.ParseTTF(bytes.NewReader(bb))
		if err != nil {
			blockFaces.err = fmt.Errorf("the signature block's typeface %s could not be read: %w", file, err)
			return blockFaces.err
		}
		bf := &blockFace{file: file, face: f, upem: float32(f.Upem())}
		if ext, ok := f.FontHExtents(); ok && bf.upem > 0 {
			bf.asc, bf.desc = ext.Ascender/bf.upem, -ext.Descender/bf.upem
		}
		if bf.upem <= 0 || bf.asc <= 0 {
			blockFaces.err = fmt.Errorf("the signature block's typeface %s carries no usable metrics", file)
			return blockFaces.err
		}
		blockFaces.chain = append(blockFaces.chain, bf)
		if file == blockHangulFace {
			blockFaces.hangul = bf
		}
	}
	return nil
}

// faceFor is the coverage rule: the first face in the chain with a glyph for r, or nil.
func faceFor(r rune) *blockFace {
	if unicode.Is(unicode.Hangul, r) && blockFaces.hangul != nil {
		if _, ok := blockFaces.hangul.face.NominalGlyph(r); ok {
			return blockFaces.hangul
		}
	}
	for _, bf := range blockFaces.chain {
		if _, ok := bf.face.NominalGlyph(r); ok {
			return bf
		}
	}
	return nil
}

// blockFontmap hands the shaper's segmenter the same rule. A character no face has is given the first
// face so the line still has a width to wrap by; it is never drawn — see `shapedLine.missing`.
type blockFontmap struct{}

func (blockFontmap) ResolveFace(r rune) *font.Face {
	if bf := faceFor(r); bf != nil {
		return bf.face
	}
	return blockFaces.chain[0].face
}

// shapedLine is one block line as glyphs, its runs in VISUAL order (left to right on the page).
type shapedLine struct {
	runs    []shaping.Output
	advance fixed.Int26_6 // at blockShapeSize
	// missing holds each character of the line that cannot be drawn: one no shipped face has, or one
	// the shaper answered with the font's "no such glyph" box. In first-seen order, without repeats.
	missing []rune
	// asc and desc are the tallest ascender and deepest descender among the faces the line uses, ems.
	asc, desc float32
}

// shapeBlockLine is the door. The caller holds blockFaces.mu and has loaded the faces.
func shapeBlockLine(text string) shapedLine {
	var out shapedLine
	rs := []rune(text)
	if len(rs) == 0 {
		return out
	}
	seen := map[rune]bool{}
	miss := func(r rune) {
		if !seen[r] {
			seen[r] = true
			out.missing = append(out.missing, r)
		}
	}
	for _, r := range rs {
		if faceFor(r) == nil {
			miss(r)
		}
	}
	// The base direction is left to right for every line: each field begins with a Latin label, and a
	// continuation line belongs to the field above it. The language is named so the shaper never
	// consults the machine's locale.
	in := shaping.Input{
		Text: rs, RunStart: 0, RunEnd: len(rs),
		Direction: di.DirectionLTR,
		Size:      fixed.I(blockShapeSize),
		Language:  language.NewLanguage("en"),
	}
	parts := blockFaces.seg.Split(in, blockFontmap{})
	levelRuns := blockFaces.para.Segment(rs, bidi.LeftToRight)
	levelAt := func(i int) int {
		for k := 0; k < levelRuns.NumRuns(); k++ {
			if r := levelRuns.Run(k); i >= r.Start && i < r.End {
				return int(r.Level)
			}
		}
		return 0
	}
	shaped := make([]shaping.Output, len(parts))
	levels := make([]int, len(parts))
	for i, p := range parts {
		levels[i] = levelAt(p.RunStart)
		shaped[i] = blockFaces.shaper.Shape(p)
		out.advance += shaped[i].Advance
		for _, g := range shaped[i].Glyphs {
			if g.GlyphID == 0 {
				if g.ClusterIndex >= 0 && g.ClusterIndex < len(rs) {
					miss(rs[g.ClusterIndex])
				}
			}
		}
		for _, bf := range blockFaces.chain {
			if bf.face == p.Face {
				out.asc, out.desc = max(out.asc, bf.asc), max(out.desc, bf.desc)
			}
		}
	}
	for _, i := range visualOrder(levels) {
		out.runs = append(out.runs, shaped[i])
	}
	return out
}

// visualOrder is rule L2 of the Unicode bidirectional algorithm over whole runs: from the highest
// level down to 1, reverse every maximal sequence of runs at that level or above. The glyphs inside a
// run are already in visual order — the shaper returns them so.
func visualOrder(levels []int) []int {
	order := make([]int, len(levels))
	top := 0
	for i, l := range levels {
		order[i] = i
		top = max(top, l)
	}
	for l := top; l >= 1; l-- {
		for i := 0; i < len(order); {
			if levels[order[i]] < l {
				i++
				continue
			}
			j := i
			for j < len(order) && levels[order[j]] >= l {
				j++
			}
			for a, b := i, j-1; a < b; a, b = a+1, b-1 {
				order[a], order[b] = order[b], order[a]
			}
			i = j
		}
	}
	return order
}

// blockLineWidth is the width of s on one block line, in points at `blockTextPt`, in the faces it
// will be drawn in. It is the wrap's one measurement.
//
// If the faces cannot be loaded every line is as wide as the block, so nothing "fits" and the fault
// surfaces as a refusal at the first quote rather than as a block nobody measured.
func blockLineWidth(s string) float64 {
	blockFaces.mu.Lock()
	defer blockFaces.mu.Unlock()
	if loadBlockFaces() != nil {
		return blockTextWidth() * float64(len(s)+1)
	}
	return float64(shapeBlockLine(s).advance) / 64 / blockShapeSize * blockTextPt
}

// UndrawableError names a character Nib has no typeface to draw.
type UndrawableError struct {
	Field string // "the recital", "the signer's name", … — empty when the caller names the field itself
	Rune  rune
}

func (e *UndrawableError) Error() string {
	where := e.Field
	if where == "" {
		where = "this text"
	}
	return fmt.Sprintf("%s contains %s, which Nib has no typeface to draw. A signature block must show "+
		"every character of what is signed, so Nib refuses it rather than drawing an empty box in its "+
		"place. Remove or replace that character", where, describeRune(e.Rune))
}

// describeRune names a character for a person: the character itself where it can be shown, and its
// code point always, since the ones that reach a refusal are the ones a screen may not show either.
func describeRune(r rune) string {
	switch {
	case r == '\n' || r == '\r' || r == ' ' || r == ' ':
		return fmt.Sprintf("a line break (U+%04X)", r)
	case r == '\t':
		return "a tab (U+0009)"
	case unicode.IsControl(r) || !unicode.IsGraphic(r):
		return fmt.Sprintf("an invisible character (U+%04X)", r)
	}
	return fmt.Sprintf("the character “%c” (U+%04X)", r, r)
}

// ErrBlockFaces: the block's typefaces could not be loaded. They are in the binary, so this is a
// broken build, never something a user typed.
//
// No wire code: it is a fault of this build, not a decision about the peer's document, and the
// responder's own quote (`handleSessionQuote`) draws with the same faces before consent can be
// given — a build that cannot load them never reaches the contribution.
var ErrBlockFaces = errors.New("the signature block's typefaces could not be loaded")

// TextDrawable reports the first character of s that no shipped face can draw, or nil. It is the
// early form of the refusal, for the places a name or a recital first enters — a pinned label, a
// roster entry — where there is no whole block to ask yet.
func TextDrawable(s string) error {
	s = measurable(s) // a text past the cap is refused for its length by the fit; see blockMeasureCap
	blockFaces.mu.Lock()
	defer blockFaces.mu.Unlock()
	if err := loadBlockFaces(); err != nil {
		return fmt.Errorf("%w: %v", ErrBlockFaces, err)
	}
	if m := shapeBlockLine(s).missing; len(m) > 0 {
		return &UndrawableError{Rune: m[0]}
	}
	return nil
}

// BlockDrawable reports whether every character of a's block can be drawn, naming the field and the
// character when one cannot. It is asked wherever `BlockFits` is: at convene, at a quote, before a
// signature — so a ceremony nobody could sign is never convened.
func BlockDrawable(a Attestation) error {
	fields := []struct{ name, text string }{
		{"the signer's name", a.Signer},
		{"the capacity", a.Capacity},
		{"the recital", a.intent()},
	}
	if a.RosterSize == 0 {
		fields = append(fields, struct{ name, text string }{"the peer's label", a.AcceptedPeerLabel})
	}
	for _, f := range fields {
		var u *UndrawableError
		if err := TextDrawable(f.text); errors.As(err, &u) {
			u.Field = f.name
			return u
		} else if err != nil {
			return err
		}
	}
	return nil
}

// ErrBlockTooTall: a block of more lines than `maxBlockLines` reached the drawing. Every door that
// builds an attestation asks `BlockFits` first, so this is the backstop — the drawing refuses rather
// than shrinking the text past the floor or letting a line fall outside the frame.
//
// No wire code: on the receive path the block is this party's own, and both the quote and the answer
// (`handleSessionQuote`, `handleSessionRespond`) refuse one that does not fit before the consent is
// handed to the exchange — the peer is never waiting on a block this error describes.
var ErrBlockTooTall = errors.New("this signature block has more lines than fit its frame at a " +
	"readable size")

// BlockImage is the picture of a's block: the PNG a quote shows and `Contribute` embeds.
//
// It is a function of `a.AppearanceLines()` alone — no clock, no machine state — so the picture a
// party is shown and the picture that is signed are the same bytes whenever the lines are the same,
// and the lines are the quote's own (`TestTheQuotedPictureIsThePictureSigned`).
func (a Attestation) BlockImage() ([]byte, error) {
	if err := BlockDrawable(a); err != nil {
		return nil, err
	}
	return drawBlock(a.AppearanceLines())
}

// drawBlock rasterises lines into the block's frame. It refuses, and never clips: more lines than the
// frame holds, a character it cannot draw, a line wider than the frame, ink outside the picture.
func drawBlock(lines []string) ([]byte, error) {
	if len(lines) == 0 || len(lines) > maxBlockLines {
		return nil, fmt.Errorf("%w: %d lines, and %d is the limit", ErrBlockTooTall, len(lines), maxBlockLines)
	}
	blockFaces.mu.Lock()
	defer blockFaces.mu.Unlock()
	if err := loadBlockFaces(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBlockFaces, err)
	}
	rect := NominalBlockRect()
	w := int(math.Round((rect[2] - rect[0]) * blockScale))
	h := int(math.Round((rect[3] - rect[1]) * blockScale))
	pad := float32(blockPadPt * blockScale)
	lineH := (float32(h) - 2*pad) / float32(len(lines))
	px := float32(math.Min(float64(lineH)*blockLineShare, blockTextPt*blockScale))

	ink := image.NewAlpha(image.Rect(0, 0, w, h))
	var z vector.Rasterizer
	for i, text := range lines {
		ln := shapeBlockLine(text)
		if len(ln.missing) > 0 {
			return nil, &UndrawableError{Field: "the signature block", Rune: ln.missing[0]}
		}
		unit := px / blockShapeSize / 64 // one 26.6 shaping unit, in pixels
		if width := float32(ln.advance) * unit; width > float32(w)-2*pad+0.5 {
			return nil, fmt.Errorf("a line of the signature block is %.0f pixels wide and its frame "+
				"holds %.0f, so it was not drawn: %q", width, float32(w)-2*pad, text)
		}
		// The line's own font box, centred in its slot: a script taller than Latin sits lower in the
		// slot rather than reaching into the line above.
		baseline := pad + float32(i)*lineH + (lineH-(ln.asc+ln.desc)*px)/2 + ln.asc*px
		pen := pad
		for _, run := range ln.runs {
			var upem float32
			for _, bf := range blockFaces.chain {
				if bf.face == run.Face {
					upem = bf.upem
				}
			}
			if upem == 0 {
				return nil, ErrBlockFaces
			}
			for _, g := range run.Glyphs {
				x := pen + float32(g.XOffset)*unit
				y := baseline - float32(g.YOffset)*unit
				if outline, ok := run.Face.GlyphDataOutline(g.GlyphID); ok && len(outline.Segments) > 0 {
					if !fillGlyph(ink, &z, outline.Segments, x, y, px/upem) {
						return nil, fmt.Errorf("a character of the signature block would be drawn outside "+
							"its frame, so the block was not drawn: %q", text)
					}
				}
				pen += float32(g.Advance) * unit
			}
		}
	}

	img := image.NewGray(ink.Bounds())
	for i, a := range ink.Pix {
		img.Pix[i] = 255 - a
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if x < blockBorder || y < blockBorder || x >= w-blockBorder || y >= h-blockBorder {
				img.Pix[y*img.Stride+x] = 0
			}
		}
	}
	// BestSpeed: the encoder was most of the cost (measured: 10.5 ms a block at the default level
	// against 4.7 ms, for 17.8 KB against 17.0 KB), and the signing library decodes the PNG and
	// compresses the samples again itself, so nothing downstream keeps these bytes' size.
	var buf bytes.Buffer
	if err := (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// fillGlyph fills one glyph outline (font units, y up) into ink with its origin at (x, y) pixels. It
// reports false, drawing nothing, when the glyph would not lie wholly inside the picture.
//
// Each glyph is filled on its own small raster and composed with Over, so two glyphs that overlap
// add rather than cancel — one shared winding buffer would cut a hole where opposite contours cross.
func fillGlyph(ink *image.Alpha, z *vector.Rasterizer, segs []ot.Segment, x, y, scale float32) bool {
	minX, minY := float32(math.Inf(1)), float32(math.Inf(1))
	maxX, maxY := float32(math.Inf(-1)), float32(math.Inf(-1))
	at := func(p ot.SegmentPoint) (float32, float32) { return x + p.X*scale, y - p.Y*scale }
	for i := range segs {
		for _, p := range segs[i].ArgsSlice() {
			px, py := at(p)
			// Spelled out: a test file in this package declares its own integer `min`.
			if px < minX {
				minX = px
			}
			if px > maxX {
				maxX = px
			}
			if py < minY {
				minY = py
			}
			if py > maxY {
				maxY = py
			}
		}
	}
	x0, y0 := int(math.Floor(float64(minX))), int(math.Floor(float64(minY)))
	x1, y1 := int(math.Ceil(float64(maxX)))+1, int(math.Ceil(float64(maxY)))+1
	box := image.Rect(x0, y0, x1, y1)
	if !box.In(ink.Bounds()) {
		return false
	}
	z.Reset(box.Dx(), box.Dy())
	z.DrawOp = draw.Over
	ox, oy := float32(x0), float32(y0)
	for i := range segs {
		a := segs[i].Args
		ax, ay := at(a[0])
		switch segs[i].Op {
		case ot.SegmentOpMoveTo:
			z.MoveTo(ax-ox, ay-oy)
		case ot.SegmentOpLineTo:
			z.LineTo(ax-ox, ay-oy)
		case ot.SegmentOpQuadTo:
			bx, by := at(a[1])
			z.QuadTo(ax-ox, ay-oy, bx-ox, by-oy)
		case ot.SegmentOpCubeTo:
			bx, by := at(a[1])
			cx, cy := at(a[2])
			z.CubeTo(ax-ox, ay-oy, bx-ox, by-oy, cx-ox, cy-oy)
		}
	}
	z.Draw(ink, box, image.Opaque, image.Point{})
	return true
}
