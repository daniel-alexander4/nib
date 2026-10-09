package server

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"sort"
	"strconv"

	"nib/internal/pdfops"
	"nib/internal/sign"
)

// maxOCRWordsBytes caps the OCR words body. A dense page runs to a few thousand
// Word structs and a long document is stamped page by page, so 32 MiB clears any
// real request by a wide margin while still bounding the decode.
const maxOCRWordsBytes = 32 << 20

// maxOCRSkippedListed is how many pages each list in the X-Nib-OCR header names.
const maxOCRSkippedListed = 64

// ocrFacts is what the OCR route says about the pages it was sent words for, in `X-Nib-OCR` (ADR-072: a fact about
// the returned bytes is a header).
type ocrFacts struct {
	// Skipped is the pages that have a text layer and were left as they are; More is how many are not listed.
	Skipped []int `json:"skipped,omitempty"`
	More    int   `json:"more,omitempty"`
	// Replaced is the pages whose layer was taken out and stamped again (a replace only).
	Replaced []int `json:"replaced,omitempty"`
	// Causes is why each listed skipped page could not be replaced, by page number (a replace only): a
	// `pdfops.TextLayerKind`.
	Causes map[string]string `json:"causes,omitempty"`
}

// write sets the header, when there is anything to say. Capped, so a long document cannot grow it.
func (f ocrFacts) write(w http.ResponseWriter) {
	if len(f.Skipped) == 0 && len(f.Replaced) == 0 {
		return
	}
	sort.Ints(f.Skipped)
	sort.Ints(f.Replaced)
	if len(f.Skipped) > 0 {
		log.Printf("ocr: %d page(s) already had a text layer and were left as they are", len(f.Skipped))
	}
	if len(f.Skipped) > maxOCRSkippedListed {
		for _, p := range f.Skipped[maxOCRSkippedListed:] {
			delete(f.Causes, strconv.Itoa(p))
		}
		f.Skipped, f.More = f.Skipped[:maxOCRSkippedListed], len(f.Skipped)-maxOCRSkippedListed
	}
	if len(f.Replaced) > maxOCRSkippedListed {
		f.Replaced = f.Replaced[:maxOCRSkippedListed]
	}
	if hdr, err := json.Marshal(f); err == nil {
		w.Header().Set("X-Nib-OCR", string(hdr))
	}
}

// handleOCR bakes an invisible, searchable text layer onto the current document
// from OCR results the browser produced. The browser rasterizes each page and
// runs OCR (tesseract.js) entirely client-side — nothing leaves the machine —
// then maps each word's box to PDF points and POSTs the words here. The server
// stamps them in render mode 3 (invisible) so the page still looks like the scan
// but its text is selectable, copyable, and findable. It rides the undo ring like
// any other document operation.
func (s *Server) handleOCR(w http.ResponseWriter, r *http.Request) {
	// A malformed scan can make pdfcpu panic deep in the stamp path; without this
	// the http server's default recovery just drops the connection (the browser
	// sees a bare "Failed to fetch"). Turn it into a clean, logged 422 instead.
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("ocr: recovered panic: %v", rec)
			httpError(w, http.StatusUnprocessableEntity, "could not add the text layer")
		}
	}()
	doc, ok := s.resolveDoc(w, r)
	if !ok {
		return
	}
	// The browser sends the OCR language alongside the words: it picks the font
	// the invisible layer is stamped in (Thai/Devanagari need a non-Roboto face).
	var body struct {
		Lang  string        `json:"lang"`
		Words []pdfops.Word `json:"words"`
		// Replace asks for Nib's own earlier text layer to be taken off each page these words are for, and the
		// words stamped in its place (ADR-101). Absent, a page that has a layer is left as it is (ADR-094).
		Replace bool `json:"replace"`
	}
	// Capped like every other JSON body in this package (1<<16 for the small
	// settings bodies, 1<<20 for profile). This one was uncapped, and it is the
	// largest of them: a page of OCR output is thousands of Word structs, so the
	// ceiling has to clear a real document while still bounding the stream. An
	// uncapped decoder here let a single POST grow the heap without limit.
	if err := json.NewDecoder(io.LimitReader(r.Body, maxOCRWordsBytes)).Decode(&body); err != nil {
		httpError(w, http.StatusBadRequest, "could not read OCR words")
		return
	}
	if len(body.Words) == 0 {
		writeJSON(w, s.docResponse(doc))
		return
	}
	// Read ONCE, and this is the state the undo entry records.
	//
	// commitMutation's contract: "Callers pass the input they actually operated on … so
	// undo restores precisely the pre-op document." Calling docBytes twice — as the
	// operation's input and again as the commit's — lets a concurrent mutation land in
	// between, so the undo entry records the NEW bytes as the state to return to and a
	// later undo restores a document that never existed.
	before := s.docBytes(doc)
	var (
		result []byte
		tagged bool
		err    error
		facts  ocrFacts
	)
	if body.Replace {
		// **Asked for, never implied, and never on a signed document** (ADR-101). Taking a layer out rewrites the
		// pages a signature covers, and "read it again" is not a reason to break one: refused here, at the server
		// door, whatever the window offered — by the one signedness predicate (`sign.HasSignatureBlob`).
		if sign.HasSignatureBlob(before) {
			writeJSONStatus(w, http.StatusUnprocessableEntity, map[string]string{"cause": "signed",
				"error": "this document is signed, and replacing its text layer would change the bytes its signatures cover"})
			return
		}
		var left map[int]pdfops.TextLayerKind
		result, tagged, facts.Replaced, left, err = pdfops.ReplaceOCRLayer(before, body.Words, body.Lang)
		if len(left) > 0 {
			facts.Causes = map[string]string{}
			for p, kind := range left {
				facts.Skipped = append(facts.Skipped, p)
				facts.Causes[strconv.Itoa(p)] = string(kind)
			}
		}
	} else {
		// **A page that already has a text layer does not get a second one** (/pending 851 part 4, ADR-094). The
		// rule is `pdfops.PagesWithTextLayer`, asked here whatever the window did: the window asks the same reader
		// so as not to spend a recognition pass, and a window that did not ask must not be able to double a layer.
		words := body.Words
		if layered, lerr := pdfops.PagesWithTextLayer(before); lerr == nil && len(layered) > 0 {
			words = make([]pdfops.Word, 0, len(body.Words))
			seen := map[int]bool{}
			for _, wd := range body.Words {
				if !layered[wd.Page] {
					words = append(words, wd)
				} else if !seen[wd.Page] {
					seen[wd.Page] = true
					facts.Skipped = append(facts.Skipped, wd.Page)
				}
			}
		}
		body.Words = words
		if len(words) > 0 {
			// **The text layer is DESCRIBED, not disclaimed** — `PLAN-accessibility.md` P06.S06.
			//
			// `StampTextLayer` alone leaves every word wrapped `/Artifact <</Subtype /Watermark>>`, which
			// tells a conforming reader to skip it: the one thing that makes a scan readable was marked as
			// not-to-be-read. `TagOCRLayer` is the same stamp with a structure tree over it, built from the
			// block/paragraph/line indices the client now sends.
			//
			// No wroteStampTextError here, deliberately: StampTextLayer cannot produce that error. It SKIPS an
			// unrepresentable word rather than failing the layer (pdfops/ocr.go), because OCR text is the
			// scan's own words and there is nothing for the user to retype. A door here would be a door no
			// traffic reaches.
			result, tagged, err = pdfops.TagOCRLayer(before, words, body.Lang)
		}
	}
	// Which pages were left as they were — and, for a replace, which were read again and why the others were not.
	facts.write(w)
	if err != nil {
		log.Printf("ocr: stamp failed (%d words, lang %q, replace %v): %v", len(body.Words), body.Lang, body.Replace, err)
		httpError(w, http.StatusUnprocessableEntity, "could not add the text layer")
		return
	}
	if result == nil {
		// Nothing was left to stamp: the document is not rewritten, and no undo entry is made for a change that
		// was not made.
		writeJSON(w, s.docResponse(doc))
		return
	}
	// `tagged` false is not an error and is not surfaced: it means the structure could not be built
	// and the ordinary stamped layer was returned instead. A scan whose text is searchable but
	// artifacted is what nib shipped for years; a scan with no text layer is worse than both.
	if !tagged {
		log.Printf("ocr: the text layer was stamped but could not be described (%d words, lang %q)",
			len(body.Words), body.Lang)
	}
	if verr := pdfops.Validate(result); verr != nil {
		log.Printf("ocr: stamped output failed validation (%d words): %v", len(body.Words), verr)
		httpError(w, http.StatusUnprocessableEntity, "could not add the text layer")
		return
	}
	// Declare the document's language for assistive technology (the WCAG
	// "language of page" primitive) — an OCR'd scan now knows what it's written in.
	// Best-effort: a failure here must not fail the OCR itself.
	if tag := pdfops.OCRLangToBCP47(body.Lang); tag != "" {
		if tagged, lerr := pdfops.SetLang(result, tag); lerr == nil {
			result = tagged
		} else {
			log.Printf("ocr: could not set document language %q: %v", tag, lerr)
		}
	}
	// One commit: one undo step back to the layer that was there, and the byte cap's door (ADR-008).
	if err := s.commitMutation(doc, snapshotBase(before), result, false); wroteCommitFailure(w, err) {
		return
	}
	writeJSON(w, s.docResponse(doc))
}

// handleOCRPages answers which pages of the open document already have a text layer, so the window does not spend a
// recognition pass on a page the OCR route would leave alone. One read of the document for all of them: asking the
// page map page by page re-reads the whole file each time (measured on a 14-page, 10.7 MB OCR'd scan: 1.2 s a page
// against 2.3 s for every page here). The rule is the one handleOCR applies (`pdfops.PagesWithTextLayer`'s), asked
// through `pdfops.TextPages`, which also says which of those pages carry Nib's own layer and can be read again.
func (s *Server) handleOCRPages(w http.ResponseWriter, r *http.Request) {
	doc, ok := s.resolveDoc(w, r)
	if !ok {
		return
	}
	data := s.docBytes(doc)
	kinds, unread, err := pdfops.TextPages(data)
	if err != nil {
		// The document was well-formed enough to open and cannot be read for this (ADR-072). The window carries on
		// as though no page had a layer, and the OCR route decides.
		writeJSONStatus(w, http.StatusUnprocessableEntity, map[string]string{"cause": "document-unreadable", "error": err.Error()})
		return
	}
	// `own` is the layered pages that can be read again: Nib's own layer and nothing else (ADR-101). None on a
	// signed document, where the route refuses a replace.
	signed := sign.HasSignatureBlob(data)
	pages, own := make([]int, 0, len(kinds)), []int{}
	for p, kind := range kinds {
		pages = append(pages, p)
		if kind == pdfops.LayerOwn && !signed {
			own = append(own, p)
		}
	}
	sort.Ints(pages)
	sort.Ints(own)
	// `unread` is the pages that set no text and are not blank — a scan nothing has read. A command that needs a
	// page's text reads exactly those first (ADR-106), and asks here because it is one read for every page.
	writeJSON(w, map[string]any{"layered": pages, "own": own, "unread": unread})
}
