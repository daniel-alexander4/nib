package server

import (
	"encoding/json"
	"log"
	"net/http"

	"nib/internal/pdfops"
)

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
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpError(w, http.StatusBadRequest, "could not read OCR words")
		return
	}
	if len(body.Words) == 0 {
		writeJSON(w, s.docResponse())
		return
	}
	result, err := pdfops.StampTextLayer(doc.data, body.Words, body.Lang)
	if err != nil {
		log.Printf("ocr: stamp failed (%d words, lang %q): %v", len(body.Words), body.Lang, err)
		httpError(w, http.StatusUnprocessableEntity, "could not add the text layer")
		return
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
	if !s.commitMutation(doc, doc.data, result) {
		httpError(w, http.StatusNotFound, "no document open")
		return
	}
	writeJSON(w, s.docResponse())
}
