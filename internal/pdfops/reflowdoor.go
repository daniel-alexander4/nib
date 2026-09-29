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
	l, err := readPageGlyphLayout(ctx, page)
	if err != nil {
		return nil, err
	}
	out := make([]Paragraph, 0, len(l.paragraphs))
	for i, p := range l.paragraphs {
		para := Paragraph{Index: i, Text: p.text()}
		para.Refusal = paragraphRefusal(ctx, page, p)
		out = append(out, para)
	}
	return out, nil
}

// ReflowParagraph re-sets paragraph index of page as text and returns the whole document rewritten — or the cause it
// fell back on, with no document. original is the paragraph's text as the caller read it: when the paragraph no longer
// reads that way the answer is ErrReflowStale, never a rewrite of whatever the index names now.
//
// **Everything it draws stays inside the paragraph's own box**: the same baselines, no more lines, and no line past the
// right edge its lines already reach. So a NibFlag placed beside the paragraph still sits beside it — the decision
// `/pending 457` waited on for this phase; text that MOVES is P07's.
func ReflowParagraph(pdf []byte, page, index int, original, text string) ([]byte, string, error) {
	var cause string
	out, err := writeMutated(pdf, func(ctx *model.Context) error {
		if page < 1 || page > ctx.PageCount {
			return fmt.Errorf("pdfops: there is no page %d", page)
		}
		l, err := readPageGlyphLayout(ctx, page)
		if err != nil {
			return err
		}
		if index < 0 || index >= len(l.paragraphs) || normalizedText(l.paragraphs[index].text()) != normalizedText(original) {
			return ErrReflowStale
		}
		o, err := reflowParagraphIn(ctx, l, page, index, text)
		if err != nil {
			return err
		}
		if o.content == nil {
			cause = o.cause
			return errNothingToWrite
		}
		d, _, _, err := ctx.PageDict(page, false)
		if err != nil || d == nil {
			return fmt.Errorf("pdfops: page %d does not resolve: %w", page, err)
		}
		return setPageContent(ctx, d, o.content)
	})
	if errors.Is(err, errNothingToWrite) {
		return nil, cause, nil
	}
	if err != nil {
		return nil, "", err
	}
	return out, "", nil
}

// errNothingToWrite stops the rewrite when the reflow fell back: the document is not written at all.
var errNothingToWrite = errors.New("pdfops: nothing to write")
