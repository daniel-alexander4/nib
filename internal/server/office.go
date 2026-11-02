package server

import (
	"net/http"

	"nib/internal/sign"
)

// handleOffice converts a posted document to PDF — Markdown natively, office
// documents (DOCX/XLSX/ODT/…) via installed LibreOffice — and installs the result
// as the working document, returning the same meta as an upload so the browser can
// render it immediately — a one-step "open & convert". The converted document is
// upload-origin (no path), so it can be edited and saved-as but not saved in
// place, exactly like any uploaded file.
func (s *Server) handleOffice(w http.ResponseWriter, r *http.Request) {
	release, ok := s.admitUpload(w, r)
	if !ok {
		return
	}
	defer release()
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

	// **The same function as the four open routes** (ADR-185, D8): `pdfops.Import` classifies by content,
	// converts, titles the document from its name and applies the Document language field
	// (`/pending 471`) — whatever arrives is declared, replacing the converter's own /Lang. The PDF/UA
	// identification (ADR-033) is on nib's own Markdown conversion only, and only when the user CHOSE the
	// language (`langChosen`), because the field's pre-fill is this computer's language and a conformance
	// claim cannot rest on a guess. Its refusals — a missing tool, a name that contradicts the contents, a
	// cap — are `importRefusal`'s: the body stays a flat sentence with no URL in it, and the web client
	// carries its own link (ADR-039).
	o := s.importOptions("office")
	o.Lang = r.FormValue("lang")
	o.LangChosen = r.FormValue("langChosen") == "1"
	res, ok := s.importOrRefuse(w, r.Context(), file, header.Size, header.Filename, o)
	if !ok {
		return
	}
	// Present the converted PDF under the source name with a .pdf extension. Recorded ON
	// the document, so /api/docs and a reload report it too — see document.name.
	installed, cerr := s.addDocCapped(&document{path: "", name: res.Name, data: res.PDF, sig: sign.Verify(res.PDF), imported: importedBase(res, header.Filename)})
	if cerr != nil {
		httpError(w, http.StatusConflict, cerr.Error())
		return
	}
	resp := s.docResponse(installed)
	resp.ImportedFrom = installed.imported
	writeJSON(w, resp)
}
