package pdfops

import (
	"fmt"
	"sort"

	"nib/internal/contentstream"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// Tagging an OCR'd scan from tesseract's own hierarchy — `PLAN-accessibility.md` P06.S06.
//
// # What was actually wrong, which is worse than "untagged"
//
// `StampTextLayer` draws each OCR'd word through `api.TextWatermark`, and pdfcpu brackets every
// watermark as `/Artifact <</Subtype /Watermark /Type /Pagination>> BDC … EMC`. An artifact is
// content a conforming reader is told to SKIP. So the searchable text layer — the only thing that
// makes a scanned page readable at all — was explicitly declared not to be read, and the page
// satisfied ua1 7.1 t3 for that content by disclaiming it rather than by describing it.
//
// Tagging it is therefore a REPLACEMENT of that marker, which is why `contentstream` grew `Replace`
// for this slice.
//
// # The hierarchy, and where it comes from
//
// tesseract already computes `blocks → paragraphs → lines → words` and hands back flattened word
// entries carrying back-pointers to all three. The client sends three integer ids per word
// (`Word.Block`, `.Para`, `.Line`); until P06.S06 it sent the box and the string and dropped the
// layout the engine had just worked out.
//
// The mapping is block → `Sect`, paragraph → `P`, word → an MCID inside that `P`. **A line becomes
// no element**: PDF/UA has no line-level structure type, because a line is where text happened to
// wrap and not what it means. `Line` orders words within a paragraph and nothing else.
//
// # Why the source is `sourceApproximate` and not exact
//
// D4's exact tier is for structure read from an authoring format's own AST, as `tagMarkdown` does.
// This is an OCR engine's OPINION about a picture of a page: two columns can be read as one block,
// a table as paragraphs. It is far better than nothing and it is not the document's own structure,
// and `tagSource` is where that distinction is recorded rather than argued.

// wordGroup is one paragraph's worth of words, in reading order, with the indices into the caller's
// span list that draw them.
type wordGroup struct {
	block, para int
	spans       []int
}

// groupWords orders the words of one page into paragraphs, preserving the order they were stamped
// in — which is the order the spans appear in the content stream, and so the only order that can be
// matched to them.
//
// **Zero means "no hierarchy", never "group 0".** An older client sends no indices at all, and
// reading that as one giant block containing one giant paragraph would describe every word on the
// page as one sentence. A word with no hierarchy gets its own group, which is the honest reading:
// nothing is known about what it belongs with.
func groupWords(words []Word) []wordGroup {
	var out []wordGroup
	at := map[[2]int]int{} // (block, para) -> index into out
	for i, w := range words {
		if w.Block == 0 || w.Para == 0 {
			out = append(out, wordGroup{block: w.Block, para: w.Para, spans: []int{i}})
			continue
		}
		key := [2]int{w.Block, w.Para}
		j, seen := at[key]
		if !seen {
			out = append(out, wordGroup{block: w.Block, para: w.Para})
			j = len(out) - 1
			at[key] = j
		}
		out[j].spans = append(out[j].spans, i)
	}
	// Within a paragraph, reading order is line order then the order the words were stamped in.
	// Stamp order is already word order within a line, so a stable sort on the line alone is the
	// whole rule.
	for k := range out {
		g := out[k]
		sort.SliceStable(g.spans, func(a, b int) bool {
			return words[g.spans[a]].Line < words[g.spans[b]].Line
		})
	}
	return out
}

// tagOCRPage replaces every watermark-artifact marker on one page with real marked content, under a
// `Sect` per tesseract block and a `P` per paragraph.
func tagOCRPage(ctx *model.Context, tree *structTree, pageNr int, words []Word) error {
	d, _, _, err := ctx.PageDict(pageNr, false)
	if err != nil || d == nil {
		return fmt.Errorf("pdfops: page %d does not resolve: %w", pageNr, err)
	}
	src, cerr := ctx.PageContent(d, pageNr)
	if cerr != nil {
		return cerr
	}
	markers := watermarkArtifactSpans(src)

	// **The correspondence, checked — the same rule `tagOnePage` states and for the same reason.**
	// A mismatch attaches every element from the point of divergence to the wrong word, and the
	// document looks entirely correct. The count can differ legitimately: `StampTextLayer` SKIPS a
	// word it cannot represent rather than failing the layer, so more words than markers is the
	// ordinary outcome of an unrepresentable glyph — and it is still a correspondence this code
	// cannot establish by position.
	if len(markers) != len(words) {
		return fmt.Errorf("pdfops: page %d draws %d watermark artifact(s) and %d word(s) were "+
			"stamped — refusing to describe content by position when the two do not correspond",
			pageNr, len(markers), len(words))
	}
	if len(markers) == 0 {
		return nil
	}

	edit := contentstream.NewEdit(src)
	var sectRef *types.IndirectRef
	sectBlock := 0

	for _, g := range groupWords(words) {
		// One `Sect` per tesseract block, shared by its paragraphs. Block 0 is "no hierarchy" and
		// gets no Sect — a grouping element over content nothing said was grouped.
		if g.block == 0 {
			sectRef, sectBlock = nil, 0
		} else if sectRef == nil || sectBlock != g.block {
			sr, serr := addGroupingElement(ctx, tree, "Sect", nil)
			if serr != nil {
				return serr
			}
			sectRef, sectBlock = sr, g.block
		}

		mcid, elemRef, aerr := addMarkedElementUnder(ctx, tree, pageNr, "P", sectRef)
		if aerr != nil {
			return aerr
		}
		// **The paragraph owns every word in it.** One `P` with N MCIDs, not N paragraphs — the
		// distinction `tagOnePage` draws for a wrapped paragraph, arriving here as the difference
		// between a sentence and a column of single words.
		for n, si := range g.spans {
			id := mcid
			if n > 0 {
				extra, eerr := addMCIDTo(ctx, tree, pageNr, *elemRef)
				if eerr != nil {
					return eerr
				}
				id = extra
			}
			m := markers[si]
			edit.Replace(m.start, m.end, []byte(fmt.Sprintf("/P <</MCID %d>> BDC", id)))
		}
	}

	edited, eerr := edit.Apply()
	if eerr != nil {
		return eerr
	}
	return setPageContent(ctx, d, edited)
}

// TagOCRLayer stamps an OCR text layer and DESCRIBES it, instead of declaring it an artifact.
//
// It is `StampTextLayer` plus the tree: same words, same invisible render mode, same appearance on
// the page. What changes is what a reader is told about that text — see the file header.
//
// **It never costs the caller the text layer.** If the structure cannot be built — the marker count
// disagrees with the word count, a page will not resolve — the stamped document is returned as
// `StampTextLayer` would have produced it, with the reason. A scan whose text is searchable but
// artifacted is the state nib shipped for years; a scan with no text layer at all is worse than
// both, and refusing here would be choosing it.
func TagOCRLayer(pdf []byte, words []Word, lang string) (out []byte, tagged bool, err error) {
	stamped, err := StampTextLayer(pdf, words, lang)
	if err != nil {
		return nil, false, err
	}
	byPage := map[int][]Word{}
	for _, w := range words {
		byPage[w.Page] = append(byPage[w.Page], w)
	}
	tree, terr := writeMutated(stamped, func(ctx *model.Context) error {
		live := map[int]bool{}
		for p := 1; p <= ctx.PageCount; p++ {
			if ir, e := ctx.PageDictIndRef(p); e == nil && ir != nil {
				live[ir.ObjectNumber.Value()] = true
			}
		}
		st, err := ensureStructTree(ctx, live)
		if err != nil {
			return err
		}
		for p := 1; p <= ctx.PageCount; p++ {
			if len(byPage[p]) == 0 {
				continue
			}
			if err := tagOCRPage(ctx, st, p, byPage[p]); err != nil {
				return err
			}
		}
		return nil
	})
	if terr != nil {
		return stamped, false, nil
	}
	// Both halves and the tier, through the one door (ADR-009). D4's tier here is approximate: an
	// OCR engine's opinion about a picture.
	claimed, ok, cerr := claimTagging(tree, sourceApproximate)
	if cerr != nil || !ok {
		// Returning the artifacted stamp is the honest fallback and not a failure: the text layer
		// is what the user asked for and it is intact.
		return stamped, false, nil
	}
	return declareOCRLanguage(claimed, lang), true, nil
}

// declareOCRLanguage writes the recognised language onto the catalog, if the document declares none.
//
// # Why this is not the guess `/pending 471` refuses
//
// P03.S02 measured an office suite's `/Lang` and found it was the CONVERTING MACHINE'S LOCALE —
// three DOCX documents and an ODT all came out `en-US`, and the same ODT under a German locale came
// out `de-DE`. That is a guess dressed as a declaration, and `/pending 471` parks the question of
// where a real one comes from.
//
// **The OCR route is the one door where nib is TOLD.** The user picks the language in the OCR
// control before the scan is read; tesseract is handed it and the whole recognition depends on it.
// So this is not inference from a locale — it is the person who knows, saying so.
//
// # Why the tag comes from `OCRLangToBCP47` and not a map of this file's own
//
// It had one, briefly, and that was the defect ADR-009 names: `ocrLangBCP47` already existed and
// `internal/server/ocr.go` already called it on this very route. A second table would have been a
// rule with two implementations that agree today.
//
// # Why it runs here as well as in the route
//
// The route sets the language AFTER stamping, and until this slice that was a document-level
// nicety. It is now load-bearing: a tagged page whose language cannot be determined fails ua1
// **7.2 t34** — measured, tagging the OCR layer ADDS that clause — so a caller reaching
// `TagOCRLayer` directly must not produce a document that is worse for having been described. With
// it, a tagged OCR'd scan fails a strict SUBSET of what the untagged scan failed.
//
// An unknown code writes nothing, and a document that already declares a language keeps it: an
// author's statement is not an OCR run's to overrule.
func declareOCRLanguage(pdf []byte, lang string) []byte {
	tag := OCRLangToBCP47(lang)
	if tag == "" {
		return pdf
	}
	out, err := writeMutated(pdf, func(ctx *model.Context) error {
		cat, cerr := ctx.XRefTable.Catalog()
		if cerr != nil {
			return cerr
		}
		if s, ok := cat["Lang"].(types.StringLiteral); ok && len(s) > 0 {
			return nil // the document already says; an OCR run does not overrule an author
		}
		cat["Lang"] = types.StringLiteral(tag)
		return nil
	})
	if err != nil {
		return pdf
	}
	return out
}
