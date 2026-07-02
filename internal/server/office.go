package server

import (
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"nib/internal/pdfops"
	"nib/internal/sign"
)

// handleOffice converts a posted office document (DOCX/XLSX/ODT/…) to PDF via
// installed LibreOffice and installs the result as the working document, returning
// the same meta as an upload so the browser can render it immediately — a one-step
// "open & convert". The converted document is upload-origin (no path), so it can be
// edited and saved-as but not saved in place, exactly like any uploaded file.
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
	if !pdfops.SupportedOfficeExt(ext) {
		httpError(w, http.StatusBadRequest, "unsupported document type — convert a Word, Excel, PowerPoint, or OpenDocument file")
		return
	}

	pdf, err := pdfops.ConvertOfficeToPDF(data, ext)
	if err != nil {
		if errors.Is(err, pdfops.ErrLibreOfficeMissing) {
			httpError(w, http.StatusBadRequest, "LibreOffice is not installed")
		} else {
			httpError(w, http.StatusBadRequest, err.Error())
		}
		return
	}

	s.setDoc(&document{path: "", data: pdf, sig: sign.Verify(pdf)})
	resp := s.docResponse()
	// Present the converted PDF under the source name with a .pdf extension.
	base := strings.TrimSuffix(header.Filename, filepath.Ext(header.Filename))
	resp.Name = base + ".pdf"
	writeJSON(w, resp)
}
