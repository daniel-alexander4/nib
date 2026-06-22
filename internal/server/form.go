package server

import (
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
