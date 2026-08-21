// Package mdpdf renders CommonMark Markdown into a paginated PDF with crisp,
// selectable vector text. It is deliberately self-contained — its only
// dependencies are goldmark (parsing) and pdfcpu (PDF generation) — so it can
// be imported on its own, without the rest of Nib.
//
// The output uses the PDF Base-14 core fonts (Helvetica for body text, Courier
// for code), so Convert embeds no font files and coverage is limited to Latin
// (WinAnsi) text — anything else renders as spaces, which is what Unsupported
// exists to warn about. ConvertWithFonts takes a pool of embeddable faces and
// sets each word the core fonts cannot print in the first one that covers it,
// so a document with one Greek word embeds one face and nothing else changes.
// The FONTS ARE THE CALLER'S: this package holds none, which is what keeps it
// importable on its own. Images and tables are skipped; links render as their text.
//
// Beyond Markdown rendering, the package provides in-memory PDF-assembly
// primitives: AssemblePacket concatenates a cover PDF with exhibit files
// (PDFs and raster images) into one paginated packet, and ImageToPDF renders
// an image as US Letter page(s).
package mdpdf

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// Typography: Base-14 core fonts only, so nothing needs embedding.
const (
	fontBody       = "Helvetica"
	fontBold       = "Helvetica-Bold"
	fontItalic     = "Helvetica-Oblique"
	fontBoldItalic = "Helvetica-BoldOblique"
	fontCode       = "Courier"

	sizeBody = 11
	sizeCode = 10

	paraGap     = 6.0  // after a paragraph
	tightGap    = 2.0  // after a tight list item
	codeGap     = 5.0  // around code blocks
	listIndent  = 18.0 // per nesting level (also the marker gutter)
	quoteIndent = 18.0
	// maxBlockIndent caps nested indentation so a deeply nested quote or list cannot push
	// the wrap width to nothing. Two thirds of the content width leaves room for text at
	// any depth; see the Blockquote case.
	maxBlockIndent = contentW * 2 / 3
	markerGutter   = listIndent
)

// headingSizes maps heading level 1-6 to point size (always bold).
var headingSizes = [6]int{20, 16, 13, 12, 11, 11}

// Convert renders CommonMark Markdown to a paginated US Letter PDF using the Base-14
// core fonts only — nothing is embedded, and coverage is WinAnsi (Latin) as a result.
// Text outside that repertoire renders as spaces; see Unsupported, and ConvertWithFonts
// for how to print it.
func Convert(md []byte) ([]byte, error) { return ConvertWithFonts(md, nil) }

// ConvertWithFonts is Convert with a fallback pool for scripts the core fonts cannot
// print — Cyrillic, Greek, CJK, Arabic, Devanagari and the rest.
//
// A word containing runes WinAnsi has no glyph for is set in the first supplied Font
// whose Covers include them; everything else stays on the core fonts, so a document with
// one Greek word embeds one face and the rest of the file is unchanged. A word nothing
// supplied can print keeps the core font and still renders as spaces — substituting a
// face that cannot print it either would only move the blanks, and Unsupported still
// reports it either way.
func ConvertWithFonts(md []byte, fallbacks []Font) ([]byte, error) {
	if err := installFallbacks(fallbacks); err != nil {
		return nil, err
	}
	doc := goldmark.New().Parser().Parse(text.NewReader(md))
	r := &renderer{src: md, l: newLayout(), fonts: fallbacks}
	r.blocks(doc, 0, nil)
	spec, err := r.l.spec()
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := api.Create(nil, bytes.NewReader(spec), &out, model.NewDefaultConfiguration()); err != nil {
		return nil, fmt.Errorf("render pdf: %w", err)
	}
	return out.Bytes(), nil
}

// renderer walks the goldmark AST and drives the layout.
type renderer struct {
	src   []byte
	l     *layout
	fonts []Font
}

// blocks lays out the children of n. A non-nil marker (list bullet/number)
// attaches to the first child only.
func (r *renderer) blocks(n ast.Node, indent float64, marker *word) {
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		r.block(c, indent, marker)
		marker = nil
	}
}

func (r *renderer) block(n ast.Node, indent float64, marker *word) {
	switch t := n.(type) {
	case *ast.Heading:
		lvl := min(t.Level, 6)
		sty := style{font: fontBold, size: headingSizes[lvl-1]}
		// Keep the heading with at least two body lines; break early otherwise.
		bodyLead := style{font: fontBody, size: sizeBody}.leading()
		r.l.need(0.8*float64(sty.size) + sty.leading() + 2*bodyLead + paraGap)
		r.l.gap(0.8 * float64(sty.size))
		r.l.para(r.inline(n, sty.size), indent, marker, sty.leading())
		r.l.gap(4)
	case *ast.Paragraph:
		r.l.para(r.inline(n, 0), indent, marker, style{font: fontBody, size: sizeBody}.leading())
		r.l.gap(paraGap)
	case *ast.TextBlock: // paragraph inside a tight list item
		r.l.para(r.inline(n, 0), indent, marker, style{font: fontBody, size: sizeBody}.leading())
		r.l.gap(tightGap)
	case *ast.List:
		num := t.Start
		for li := t.FirstChild(); li != nil; li = li.NextSibling() {
			m := word{[]frag{{"•", style{font: fontBody, size: sizeBody}}}}
			if t.IsOrdered() {
				m = word{[]frag{{strconv.Itoa(num) + ".", style{font: fontBody, size: sizeBody}}}}
				num++
			}
			// Clamped, exactly as *ast.Blockquote below is and for the same measured
			// reason: past maxBlockIndent, wrapWords has a non-positive budget and emits
			// ONE RUNE PER LINE for the rest of the document. contentW is 468 and
			// listIndent is 18, so 26 nesting levels reach it — about a kilobyte of input.
			// A file with forty '>' characters is not a realistic document but is a
			// realistic paste, and a file with 26 nested list markers is the same thing;
			// mdpdf renders Markdown a stranger sent for both the GUI import and
			// `nib office`. The clamp shipped on one of the two branches.
			next := indent + listIndent
			if next > maxBlockIndent {
				next = maxBlockIndent
			}
			r.blocks(li, next, &m)
		}
		r.l.gap(paraGap - tightGap)
	case *ast.Blockquote:
		// Bounded. Each level indents, and nothing stopped the indent exceeding the
		// content width — at which point wrapWords has a negative budget and emits ONE
		// RUNE PER LINE for the rest of the document. A file with forty `>` characters is
		// not a realistic document, but it is a realistic paste, and the output is not an
		// error: it is hundreds of pages of single characters.
		next := indent + quoteIndent
		if next > maxBlockIndent {
			next = maxBlockIndent
		}
		r.blocks(t, next, marker)
	case *ast.FencedCodeBlock, *ast.CodeBlock:
		r.l.gap(codeGap)
		r.l.code(r.codeLines(n), indent)
		r.l.gap(codeGap)
	case *ast.ThematicBreak:
		r.l.rule(indent)
	case *ast.HTMLBlock:
		// Raw HTML is not rendered.
	default:
		if n.HasChildren() {
			r.blocks(n, indent, marker)
		}
	}
}

// inline collects the inline content of a block into styled words.
// headSize > 0 sets the heading base size (headings render bold).
func (r *renderer) inline(n ast.Node, headSize int) []word {
	in := &inliner{src: r.src, headSize: headSize, fonts: r.fonts}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		in.node(c)
	}
	in.flush()
	return in.words
}

// codeLines returns the raw source lines of a code block, tabs and all.
func (r *renderer) codeLines(n ast.Node) []string {
	var out []string
	ls := n.Lines()
	for i := 0; i < ls.Len(); i++ {
		seg := ls.At(i)
		out = append(out, strings.TrimRight(string(seg.Value(r.src)), "\n"))
	}
	return out
}

// inliner turns inline AST nodes into words, gluing fragments that had no
// whitespace between them in the source (so `**bold**.` wraps as one word).
type inliner struct {
	src      []byte
	fonts    []Font // fallback faces for words the core fonts cannot print
	headSize int
	bold     int
	italic   int
	code     int
	words    []word
	cur      []frag
}

func (in *inliner) node(n ast.Node) {
	switch t := n.(type) {
	case *ast.Text:
		in.addText(string(t.Segment.Value(in.src)))
		if t.HardLineBreak() {
			in.br()
		} else if t.SoftLineBreak() {
			in.flush() // soft break = word boundary
		}
	case *ast.String:
		in.addText(string(t.Value))
	case *ast.CodeSpan:
		in.code++
		in.children(n)
		in.code--
	case *ast.Emphasis:
		if t.Level >= 2 {
			in.bold++
		} else {
			in.italic++
		}
		in.children(n)
		if t.Level >= 2 {
			in.bold--
		} else {
			in.italic--
		}
	case *ast.AutoLink:
		in.addText(string(t.URL(in.src)))
	case *ast.Image:
		// Images are skipped (raster embedding is out of scope).
	case *ast.RawHTML:
		// Raw HTML is not rendered.
	default: // Link and anything unknown: render child text
		in.children(n)
	}
}

func (in *inliner) children(n ast.Node) {
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		in.node(c)
	}
}

// addText appends s in the current style, breaking words on whitespace.
//
// The style is resolved PER WORD rather than once for the whole string, because a word
// containing non-WinAnsi runes has to change face: see pickFont, which is also why the
// split happens here — this is already the place that knows where words end.
func (in *inliner) addText(s string) {
	sty := in.style()
	for s != "" {
		i := strings.IndexAny(s, " \t\n")
		if i < 0 {
			in.append(s, in.faceFor(s, sty))
			return
		}
		if i > 0 {
			in.append(s[:i], in.faceFor(s[:i], sty))
		}
		in.flush()
		s = strings.TrimLeft(s[i:], " \t\n")
	}
}

// faceFor returns sty with its font swapped for a fallback when the core font cannot
// print this word. Non-core styles carry `core: false` so width() stops byte-encoding —
// pdfcpu measures a user font by RUNE and a core font by BYTE, and using the wrong one
// makes every following fragment on the line land in the wrong place.
func (in *inliner) faceFor(word string, sty style) style {
	name := pickFont(word, sty, in.fonts)
	if name == "" || name == sty.font {
		return sty
	}
	return style{font: name, size: sty.size, embedded: true}
}

// append adds text to the word being built, merging same-style tails.
func (in *inliner) append(s string, sty style) {
	if n := len(in.cur); n > 0 && in.cur[n-1].sty == sty {
		in.cur[n-1].text += s
		return
	}
	in.cur = append(in.cur, frag{s, sty})
}

// flush completes the word being built, if any.
func (in *inliner) flush() {
	if len(in.cur) > 0 {
		in.words = append(in.words, word{in.cur})
		in.cur = nil
	}
}

// br records a hard line break.
func (in *inliner) br() {
	in.flush()
	in.words = append(in.words, word{})
}

// style resolves the active font face and size from the nesting flags.
func (in *inliner) style() style {
	size := sizeBody
	if in.headSize > 0 {
		size = in.headSize
	}
	if in.code > 0 {
		return style{font: fontCode, size: max(size-1, 4)}
	}
	b := in.bold > 0 || in.headSize > 0
	switch {
	case b && in.italic > 0:
		return style{font: fontBoldItalic, size: size}
	case b:
		return style{font: fontBold, size: size}
	case in.italic > 0:
		return style{font: fontItalic, size: size}
	}
	return style{font: fontBody, size: size}
}

// spec serializes the laid-out pages as a pdfcpu "create" JSON document.
// pdfcpu substitutes %-placeholders (%p, %P, %t, %v) in text values, so every
// literal % is escaped to %%.
func (l *layout) spec() ([]byte, error) {
	pages := map[string]any{}
	for i := range l.runs {
		content := map[string]any{}
		var texts []any
		for _, r := range l.runs[i] {
			texts = append(texts, map[string]any{
				"value": strings.ReplaceAll(r.text, "%", "%%"),
				"pos":   []any{r.x, r.y},
				"font":  map[string]any{"name": r.sty.font, "size": r.sty.size},
			})
		}
		if len(texts) == 0 && len(l.boxes[i]) == 0 {
			// pdfcpu rejects empty page content; a lone space keeps blank pages valid.
			texts = []any{map[string]any{
				"value": " ",
				"pos":   []any{marginX, pageTop},
				"font":  map[string]any{"name": fontBody, "size": sizeBody},
			}}
		}
		content["text"] = texts
		if len(l.boxes[i]) > 0 {
			var bx []any
			for _, b := range l.boxes[i] {
				bx = append(bx, map[string]any{
					"pos":     []any{b.x, b.y},
					"width":   b.w,
					"height":  b.h,
					"fillCol": "#9c9c9c",
				})
			}
			content["box"] = bx
		}
		pages[strconv.Itoa(i+1)] = map[string]any{"content": content}
	}
	return json.Marshal(map[string]any{
		// Must agree with layout.go's pageW/pageH (Letter, 612x792) or the
		// wrap/pagination math desyncs from the emitted MediaBox.
		"paper":  "LetterP",
		"origin": "LowerLeft",
		"pages":  pages,
	})
}
