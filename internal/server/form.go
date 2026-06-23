package server

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"net/http"

	"nib/internal/pdfops"
)

// handleFormAuthor turns the client's detected/placed overlay fields into real
// interactive AcroForm widgets on the posted document, returning a blank fillable
// PDF as a download. Unlike /api/bake (which flattens overlay values into the page
// content), this emits live form fields and never touches the open document — it's
// a derive-a-new-artifact action, like extract/flatten.
func (s *Server) handleFormAuthor(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(maxPDFBytes); err != nil {
		httpError(w, http.StatusBadRequest, "could not parse upload")
		return
	}
	pdfBytes, ok := formFileBytes(w, r, "pdf")
	if !ok {
		return
	}
	var fields []pdfops.FormField
	if err := json.Unmarshal([]byte(r.FormValue("fields")), &fields); err != nil {
		httpError(w, http.StatusBadRequest, "could not read fields")
		return
	}
	out, err := pdfops.AuthorForm(pdfBytes, fields)
	if err != nil {
		httpError(w, http.StatusBadRequest, "could not author form: "+err.Error())
		return
	}
	sendDownload(w, "fillable.pdf", "application/pdf", out)
}

// handleFormFillCSV mail-merges a CSV onto the posted form template — one filled
// PDF per data row — and returns them bundled as a ZIP download. It reuses the
// same engine as `nib fill --data rows.csv` (pdfops.FillFormCSV); like author and
// extract it derives a new artifact and never touches the open document. The CSV
// header row must be the form's field names.
func (s *Server) handleFormFillCSV(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(maxPDFBytes); err != nil {
		httpError(w, http.StatusBadRequest, "could not parse upload")
		return
	}
	pdfBytes, ok := formFileBytes(w, r, "pdf")
	if !ok {
		return
	}
	// nameCol "" → FillFormCSV names each output row-NNN; a column picker can set it later.
	parts, err := pdfops.FillFormCSV(pdfBytes, []byte(r.FormValue("data")), r.FormValue("nameCol"))
	if err != nil {
		httpError(w, http.StatusBadRequest, "could not fill form: "+err.Error())
		return
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, p := range parts {
		fw, err := zw.Create(p.Name + ".pdf")
		if err == nil {
			_, err = fw.Write(p.Data)
		}
		if err != nil {
			httpError(w, http.StatusInternalServerError, "could not build zip")
			return
		}
	}
	if err := zw.Close(); err != nil {
		httpError(w, http.StatusInternalServerError, "could not build zip")
		return
	}
	sendDownload(w, "filled-forms.zip", "application/zip", buf.Bytes())
}
