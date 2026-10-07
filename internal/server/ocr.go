package server

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"sort"

	"nib/internal/pdfops"
)

// maxOCRWordsBytes caps the OCR words body. A dense page runs to a few thousand
// Word structs and a long document is stamped page by page, so 32 MiB clears any
// real request by a wide margin while still bounding the decode.
const maxOCRWordsBytes = 32 << 20

// maxOCRSkippedListed is how many skipped pages the X-Nib-OCR header names.
const maxOCRSkippedListed = 64

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
	// **A page that already has a text layer does not get a second one** (/pending 851 part 4). The rule is
	// `pdfops.PagesWithTextLayer`, asked here whatever the window did: the window asks the same reader page by page so
	// as not to spend a recognition pass, and a window that did not ask must not be able to double a layer.
	words, skipped := body.Words, []int{}
	if layered, lerr := pdfops.PagesWithTextLayer(before); lerr == nil && len(layered) > 0 {
		words = make([]pdfops.Word, 0, len(body.Words))
		seen := map[int]bool{}
		for _, wd := range body.Words {
			if !layered[wd.Page] {
				words = append(words, wd)
			} else if !seen[wd.Page] {
				seen[wd.Page] = true
				skipped = append(skipped, wd.Page)
			}
		}
		sort.Ints(skipped)
	}
	// A fact about the returned bytes rides in a header (ADR-072): which pages were left as they were. Capped, so a
	// long document cannot grow the header; `more` says how many are not listed.
	if len(skipped) > 0 {
		facts := struct {
			Skipped []int `json:"skipped"`
			More    int   `json:"more,omitempty"`
		}{Skipped: skipped}
		if len(skipped) > maxOCRSkippedListed {
			facts.Skipped, facts.More = skipped[:maxOCRSkippedListed], len(skipped)-maxOCRSkippedListed
		}
		if hdr, jerr := json.Marshal(facts); jerr == nil {
			w.Header().Set("X-Nib-OCR", string(hdr))
		}
		log.Printf("ocr: %d page(s) already had a text layer and were left as they are", len(skipped))
	}
	if len(words) == 0 {
		writeJSON(w, s.docResponse(doc))
		return
	}
	body.Words = words
	// No wroteStampTextError here, deliberately: StampTextLayer cannot produce that error.
	// It SKIPS an unrepresentable word rather than failing the layer (pdfops/ocr.go), because
	// OCR text is the scan's own words and there is nothing for the user to retype. A door
	// here would be a door no traffic reaches. The review finding that named this route as
	// "swallowing" the error was wrong on that point; what it does swallow is the count of
	// skipped words, which is a separate question and not this one.
	// **The text layer is DESCRIBED, not disclaimed** — `PLAN-accessibility.md` P06.S06.
	//
	// `StampTextLayer` alone leaves every word wrapped `/Artifact <</Subtype /Watermark>>`, which
	// tells a conforming reader to skip it: the one thing that makes a scan readable was marked as
	// not-to-be-read. `TagOCRLayer` is the same stamp with a structure tree over it, built from the
	// block/paragraph/line indices the client now sends.
	//
	// `tagged` false is not an error and is not surfaced: it means the structure could not be built
	// and the ordinary stamped layer was returned instead. A scan whose text is searchable but
	// artifacted is what nib shipped for years; a scan with no text layer is worse than both.
	result, tagged, err := pdfops.TagOCRLayer(before, body.Words, body.Lang)
	if err != nil {
		log.Printf("ocr: stamp failed (%d words, lang %q): %v", len(body.Words), body.Lang, err)
		httpError(w, http.StatusUnprocessableEntity, "could not add the text layer")
		return
	}
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
	if err := s.commitMutation(doc, snapshotBase(before), result, false); wroteCommitFailure(w, err) {
		return
	}
	writeJSON(w, s.docResponse(doc))
}

// handleOCRPages answers which pages of the open document already have a text layer, so the window does not spend a
// recognition pass on a page the OCR route would leave alone. One read of the document for all of them: asking the
// page map page by page re-reads the whole file each time (measured on a 14-page, 10.7 MB OCR'd scan: 1.2 s a page
// against 2.3 s for every page here). The rule is `pdfops.PagesWithTextLayer`, the one handleOCR applies.
func (s *Server) handleOCRPages(w http.ResponseWriter, r *http.Request) {
	doc, ok := s.resolveDoc(w, r)
	if !ok {
		return
	}
	layered, err := pdfops.PagesWithTextLayer(s.docBytes(doc))
	if err != nil {
		// The document was well-formed enough to open and cannot be read for this (ADR-072). The window carries on
		// as though no page had a layer, and the OCR route decides.
		writeJSONStatus(w, http.StatusUnprocessableEntity, map[string]string{"cause": "document-unreadable", "error": err.Error()})
		return
	}
	pages := make([]int, 0, len(layered))
	for p := range layered {
		pages = append(pages, p)
	}
	sort.Ints(pages)
	writeJSON(w, map[string]any{"layered": pages})
}
