package server

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"log"
	"net/http"

	"nib/internal/pdfops"
)

// handleFormAuthor turns the client's detected/placed overlay fields into real
// interactive AcroForm widgets on the posted document, returning a blank fillable
// PDF as a download. Unlike /api/bake (which flattens overlay values into the page
// content), this emits live form fields and never touches the open document — it's
// a derive-a-new-artifact action, like extract/flatten.
func (s *Server) handleFormAuthor(w http.ResponseWriter, r *http.Request) {
	cleanup, ok := parseMultipart(w, r, maxPDFBytes)
	if !ok {
		return
	}
	defer cleanup()
	pdfBytes, ok := formFileBytes(w, r, "pdf")
	if !ok {
		return
	}
	var fields []pdfops.FormField
	if err := json.Unmarshal([]byte(r.FormValue("fields")), &fields); err != nil {
		httpError(w, http.StatusBadRequest, "could not read fields")
		return
	}
	// **The widgets are DESCRIBED, not just placed** — `PLAN-accessibility.md` P06.S07.
	//
	// `AuthorForm` gives each field a `/TU` and the page `/Tabs /S` (P06.S05) and builds no tree, so
	// a screen reader meets an annotation nothing in the structure points at — ua1 7.18.4 t1, *"A
	// Widget annotation shall be nested within a Form tag"*. `AuthorTaggedForm` is the same fields
	// with a `/Form` element per widget.
	//
	// `tagged` false is not an error and is not surfaced: it means the tree could not be built and
	// the ordinary authored form came back. A form whose widgets are undescribed is what nib shipped
	// for years; no form at all is worse than both.
	out, tagged, err := pdfops.AuthorTaggedForm(pdfBytes, fields)
	if err != nil {
		httpError(w, http.StatusBadRequest, "could not author form: "+err.Error())
		return
	}
	if !tagged {
		log.Printf("form: %d field(s) authored but not described", len(fields))
	}
	sendDownload(w, "fillable.pdf", "application/pdf", out)
}

// handleFormFillCSV mail-merges a CSV onto the posted form template — one filled
// PDF per data row — and returns them bundled as a ZIP download. It reuses the
// same engine as `nib fill --data rows.csv` (pdfops.FillFormCSV); like author and
// extract it derives a new artifact and never touches the open document. The CSV
// header row must be the form's field names.
func (s *Server) handleFormFillCSV(w http.ResponseWriter, r *http.Request) {
	cleanup, ok := parseMultipart(w, r, maxPDFBytes)
	if !ok {
		return
	}
	defer cleanup()
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

// handleFormFillXFDF fills the posted form template from an uploaded XFDF document
// and returns the single filled PDF. It reuses the same engine as `nib fill --data
// data.xfdf` (pdfops.FillFormXFDF); like author and the CSV merge it derives a new
// artifact and never touches the open document.
func (s *Server) handleFormFillXFDF(w http.ResponseWriter, r *http.Request) {
	cleanup, ok := parseMultipart(w, r, maxPDFBytes)
	if !ok {
		return
	}
	defer cleanup()
	pdfBytes, ok := formFileBytes(w, r, "pdf")
	if !ok {
		return
	}
	out, err := pdfops.FillFormXFDF(pdfBytes, []byte(r.FormValue("data")))
	if err != nil {
		httpError(w, http.StatusBadRequest, "could not fill form: "+err.Error())
		return
	}
	sendDownload(w, "filled.pdf", "application/pdf", out)
}
