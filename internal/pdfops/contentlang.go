package pdfops

import (
	"fmt"

	"nib/internal/contentstream"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// A page's own language, declared on its CONTENT — `PLAN-accessibility.md` P06.S04.
//
// # The defect this fixes, measured at P03.S02
//
// `AppendReadme` staples nib's own English prose into the user's document, and `pdfops.Append` keeps
// the **first** document's catalog. From that moment nib's English text is declared to be in
// whatever language the user's document says — for a German contract, German. Nib creates that,
// every time, and no catalog key can fix it: the catalog in question belongs to the user.
//
// # Why marked content and not a structure element
//
// Both can carry a language. Only one survives.
//
// A structure element's `/Lang` lives in the tree, the tree hangs off `/StructTreeRoot`, and
// `Append` discards the appended document's catalog — **measured**, and the same measurement that
// made two of P03.S01's titles inert. A `/Span <</Lang (en)>> BDC … EMC` lives in the PAGE CONTENT,
// which is exactly what `Append` carries across. Measured on the composed document: the catalog
// keeps the user's `(de)`, page 1 is untouched, and page 2 carries the English span.
//
// # And it claims no tagging
//
// Marked content is not a structure tree. Measured: a fragment carrying this reports
// `claims()=false` and `orphaned()=false`, so ADR-031's law 1 is not engaged — nib says *this text
// is English*, which it knows, and says nothing about structure, which it has not got here.

// AuthoredProseLang is the language of the prose nib itself writes.
//
// **It is a constant because nib knows this one with certainty**, which is the whole distinction
// P03.S02 drew: every other door either carries a language something else determined or has none
// available to it, and this is the single case where the text is literals in nib's own source. If
// nib is ever localised, the value moves to wherever the prose is chosen and this constant goes
// with it.
const AuthoredProseLang = "en"

// declareContentLang brackets every page's content in a `/Span` carrying lang.
//
// It returns the number of pages it bracketed, so a caller can tell *nothing needed doing* from
// *nothing was done* — the two are the same bytes and different facts.
//
// **Idempotent**: a page already carrying marked content
// is left alone, because a second bracket would nest a language inside a producer's own marked
// content and no reader can be asked to resolve that.
func declareContentLang(pdf []byte, lang string) (out []byte, bracketed int, err error) {
	if lang == "" {
		return nil, 0, fmt.Errorf("pdfops: a content language declaration needs a language")
	}
	out, err = writeMutated(pdf, func(ctx *model.Context) error {
		for p := 1; p <= ctx.PageCount; p++ {
			d, _, _, derr := ctx.PageDict(p, false)
			if derr != nil || d == nil {
				continue
			}
			src, cerr := ctx.PageContent(d, p)
			if cerr == model.ErrNoContent || len(src) == 0 {
				continue
			}
			if cerr != nil {
				return cerr
			}
			if alreadyMarked(src) {
				continue
			}
			// The language is a NAME-keyed string in the property list, and it is written through
			// pdfcpu's own escaping rather than formatted in: a BCP 47 tag has no PDF
			// metacharacter in it today, and `SetLang`'s own comment records what happened when
			// something assumed that about a value it did not control.
			prop := types.Dict{"Lang": types.StringLiteral(lang)}
			edited, eerr := contentstream.NewEdit(src).
				InsertBefore(0, []byte("/Span "+prop.PDFString()+" BDC\n")).
				InsertBefore(len(src), []byte("\nEMC")).
				Apply()
			if eerr != nil {
				return eerr
			}
			if serr := setPageContent(ctx, d, edited); serr != nil {
				return serr
			}
			bracketed++
		}
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	if bracketed == 0 {
		// **The original bytes, not a re-serialisation that happens to look the same.**
		// `writeMutated` rewrites the whole file whether or not the callback changed anything, so
		// without this a second call returns a different document — measured, 93,891 bytes against
		// 93,886 — and a caller checking idempotence by comparing bytes would be told the operation
		// is not. A document that needed nothing costs nothing.
		return pdf, 0, nil
	}
	return out, bracketed, nil
}

// DeclareAuthoredProseLang declares nib's own prose language on every page of a fragment nib wrote.
//
// It is the door `internal/p2p`'s readme and signature pages use, and the exported half of
// `declareContentLang` — the language is not a parameter, because the only documents this is for are
// the ones whose language nib knows, and a parameter would invite it onto documents whose language
// nib does not.
func DeclareAuthoredProseLang(pdf []byte) ([]byte, int, error) {
	return declareContentLang(pdf, AuthoredProseLang)
}
