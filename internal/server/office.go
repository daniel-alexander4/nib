package server

import (
	"errors"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strings"

	"nib/internal/pdfops"
	"nib/internal/sign"
)

// handleOffice converts a posted document to PDF — Markdown natively, office
// documents (DOCX/XLSX/ODT/…) via installed LibreOffice — and installs the result
// as the working document, returning the same meta as an upload so the browser can
// render it immediately — a one-step "open & convert". The converted document is
// upload-origin (no path), so it can be edited and saved-as but not saved in
// place, exactly like any uploaded file.
func (s *Server) handleOffice(w http.ResponseWriter, r *http.Request) {
	cleanup, ok := parseMultipart(w, r, maxPDFBytes)
	if !ok {
		return
	}
	defer cleanup()
	file, header, err := r.FormFile("file")
	if err != nil {
		httpError(w, http.StatusBadRequest, "no file uploaded")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		httpError(w, http.StatusBadRequest, "could not read upload")
		return
	}

	ext := filepath.Ext(header.Filename)
	if !pdfops.SupportedDocExt(ext) {
		httpError(w, http.StatusBadRequest, "unsupported document type — convert a Word, Excel, PowerPoint, OpenDocument, or Markdown file")
		return
	}

	pdf, err := pdfops.ConvertDocToPDF(data, ext)
	if err == nil {
		// The converted document's title is the source document's name — the one thing here that
		// identifies it, and what the user already calls it. Best-effort, and the door guarantees
		// it: a failed metadata write must not turn a conversion that succeeded into a 400.
		var terr error
		if pdf, terr = pdfops.TitleFromName(pdf, header.Filename); terr != nil {
			log.Printf("office: %v", terr)
		}
	}
	if err != nil {
		// One door classifies it (ADR-009, `pdfops.MissingToolFor`); the WORDING is this
		// surface's own, per the rule `handoff.go` states — ADR-009 unifies the checks and
		// explicitly does not require every site to print the same sentence.
		//
		// The body stays a flat sentence with no URL in it. The web client carries its own
		// link, authored statically in index.html, because a remedy URL on the wire would let
		// a response body choose where the page navigates — the reasoning ADR-039 used to
		// refuse a route that accepts a URL.
		if tool, ok := pdfops.MissingToolFor(err); ok {
			httpError(w, http.StatusBadRequest, "Nib "+tool.NotFound()+", so it cannot convert this document.")
		} else {
			httpError(w, http.StatusBadRequest, err.Error())
		}
		return
	}
	// The Document language field — `/pending 471`. The client pre-fills it with the machine's
	// locale and the user can change or clear it; whatever arrives is declared, replacing the
	// converter's own /Lang. Refused rather than dropped: a document installed without the language
	// the request named is the silent wrong answer the field exists to prevent.
	if lang := r.FormValue("lang"); lang != "" {
		if pdf, err = pdfops.SetLang(pdf, lang); err != nil {
			httpError(w, http.StatusBadRequest, err.Error())
			return
		}
		// The PDF/UA identification (`/pending 486`, ADR-033) — on nib's own Markdown conversion only, and
		// only when the user CHOSE the language (`langChosen`), because the field's pre-fill is this
		// computer's language and a conformance claim cannot rest on a guess. A refusal costs the user
		// nothing but the label: the conversion they asked for is already done.
		if pdfops.SupportedMarkdownExt(ext) {
			labelled, lerr := pdfops.LabelUA(pdf, r.FormValue("langChosen") == "1")
			switch {
			case lerr == nil:
				pdf = labelled
			case !errors.Is(lerr, pdfops.ErrUALanguageNotAsserted):
				log.Printf("office: %s was not labelled PDF/UA: %v", header.Filename, lerr)
			}
		}
	}

	// Present the converted PDF under the source name with a .pdf extension. Recorded ON
	// the document, so /api/docs and a reload report it too — see document.name.
	base := strings.TrimSuffix(header.Filename, filepath.Ext(header.Filename))
	installed, cerr := s.addDocCapped(&document{path: "", name: base + ".pdf", data: pdf, sig: sign.Verify(pdf)})
	if cerr != nil {
		httpError(w, http.StatusConflict, cerr.Error())
		return
	}
	resp := s.docResponse(installed)
	writeJSON(w, resp)
}
