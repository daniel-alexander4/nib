package pdfops

import (
	"errors"
	"fmt"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

	"nib/internal/pdfread"
)

// The reflow doors the server calls — `PLAN-text-reflow.md` P06.S05.

// ReflowCauseSigned is D11's refusal: a signed document is never reflowed, because rewriting a page's content changes
// the bytes its signatures cover. It is a refusal the user is told, like every other cause, not an error. The SERVER door
// decides it (`internal/server/reflow.go`, through `sign.HasSignatureBlob`, tagwrite's predicate): this package reads
// no signatures, and `sign`'s own tests import it.
const ReflowCauseSigned = "signed"

// ReflowCauseInvalidOutput is the server door's refusal of a rewrite that does not validate as a PDF: the document is
// left as it was.
const ReflowCauseInvalidOutput = "invalid-output"

// ErrReflowStale is a reflow whose paragraph no longer reads as the text the request says it edited — the page changed
// under it, or the index names another paragraph now. ADR-001: an operation acts only on what it captured.
var ErrReflowStale = errors.New("pdfops: the paragraph no longer reads as the text being edited")

// Paragraph is one paragraph of a page as a reflow editor needs it: its index in reading order, its text, and — when it
// cannot be reflowed at all — why.
type Paragraph struct {
	Index   int    `json:"index"`
	Text    string `json:"text"`
	Refusal string `json:"refusal,omitempty"`
}

// Paragraphs lists page's paragraphs, each with the cause it cannot be reflowed, if any.
func Paragraphs(pdf []byte, page int) ([]Paragraph, error) {
	ctx, err := pdfread.Validated(pdf, model.NewDefaultConfiguration())
	if err != nil {
		return nil, err
	}
	if page < 1 || page > ctx.PageCount {
		return nil, fmt.Errorf("pdfops: there is no page %d", page)
	}
	// The page is resolved once for the layout and every paragraph's refusal, not once per paragraph (/pending 756).
	pg := pageAt(ctx, nil, page)
	l, err := readPageGlyphLayout(ctx, pg)
	if err != nil {
		return nil, err
	}
	out := make([]Paragraph, 0, len(l.paragraphs))
	for i, p := range l.paragraphs {
		para := Paragraph{Index: i, Text: p.text()}
		para.Refusal = paragraphRefusal(ctx, pg, p)
		if para.Refusal == "" && annotatedOver(ctx, pg, p) {
			para.Refusal = causeAnchored // told before the user types, not after
		}
		out = append(out, para)
	}
	return out, nil
}

// Refusal is why a reflow fell back: its cause, and — when a paragraph below the edited one could not move with it — that
// paragraph's text, so the user is told which one stands in the way (P07.S03).
type Refusal struct {
	Cause string `json:"cause,omitempty"`
	Below string `json:"below,omitempty"`
}

// ReflowParagraph re-sets paragraph index of page as text and returns the whole document rewritten — or the refusal it
// fell back on, with no document. original is the paragraph's text as the caller read it: when the paragraph no longer
// reads that way the answer is ErrReflowStale, never a rewrite of whatever the index names now.
//
// **A paragraph that keeps its line count stays inside its own box**: the same baselines, and no line past the right edge
// its lines already reach — so a NibFlag placed beside it still sits beside it (`/pending 457`'s decision for P06); a
// paragraph of one line reaches its column's edge, short of anything drawn or annotated beside it — and a centred line is
// re-set about its axis, both sides of it (P08.S04), so a longer one starts left of its old box. One that needs more
// lines GROWS DOWN (P07.S03): its new lines at its own pitch, the paragraphs below it in its column moved down by the
// growth, and what no longer fits above the page's margin carried whole to the next page, which is pushed in turn (P07.S06).
// What is anchored wholly inside what moves — an annotation, a flag, a destination — moves with it, across pages included
// (P07.S04, S06); anything else where the text will be — straddling, in the free room, an article bead, a drawing — refuses
// `anchored` rather than being orphaned (P07 phase-close review: this said every anchor refused).
func ReflowParagraph(pdf []byte, page, index int, original, text string) ([]byte, Refusal, error) {
	var refusal Refusal
	out, err := writeMutated(pdf, func(ctx *model.Context) error {
		if page < 1 || page > ctx.PageCount {
			return fmt.Errorf("pdfops: there is no page %d", page)
		}
		pg := pageAt(ctx, nil, page) // once, for the read, the rewrite and the write (/pending 756)
		l, err := readPageGlyphLayout(ctx, pg)
		if err != nil {
			return err
		}
		if index < 0 || index >= len(l.paragraphs) || normalizedText(l.paragraphs[index].text()) != normalizedText(original) {
			return ErrReflowStale
		}
		o, err := reflowParagraphIn(ctx, l, pg, index, text)
		if err != nil {
			return err
		}
		if o.content == nil {
			refusal = Refusal{Cause: o.cause, Below: o.below}
			return errNothingToWrite
		}
		d, err := pg.Dict, pg.Err
		if err != nil || d == nil {
			return fmt.Errorf("pdfops: page %d does not resolve: %w", page, err)
		}
		if err := setPageContent(ctx, d, o.content); err != nil {
			return err
		}
		return applyFlow(ctx, o.flow)
	})
	if errors.Is(err, errNothingToWrite) {
		return nil, refusal, nil
	}
	if err != nil {
		return nil, Refusal{}, err
	}
	return out, Refusal{}, nil
}

// errNothingToWrite stops the rewrite when the reflow fell back: the document is not written at all.
var errNothingToWrite = errors.New("pdfops: nothing to write")
