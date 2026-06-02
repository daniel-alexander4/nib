package server

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"net/http"

	"nib/internal/pdfops"
)

// handleAssemble turns client-rendered page images into a download: a flattened
// image-PDF (the rasterize flatten / guaranteed-flat export) or a ZIP of the
// images. The browser renders each page to a PNG; the server packages them.
func (s *Server) handleAssemble(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(maxPDFBytes); err != nil {
		httpError(w, http.StatusBadRequest, "could not parse upload")
		return
	}
	parts := r.MultipartForm.File["image"]
	if len(parts) == 0 {
		httpError(w, http.StatusBadRequest, "no images provided")
		return
	}
	images := make([][]byte, 0, len(parts))
	for _, fh := range parts {
		f, err := fh.Open()
		if err != nil {
			httpError(w, http.StatusBadRequest, "could not read image")
			return
		}
		b, err := io.ReadAll(f)
		f.Close()
		if err != nil {
			httpError(w, http.StatusBadRequest, "could not read image")
			return
		}
		images = append(images, b)
	}

	if r.FormValue("format") == "zip" {
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		for i, img := range images {
			fw, _ := zw.Create(fmt.Sprintf("page-%02d.png", i+1))
			_, _ = fw.Write(img)
		}
		if err := zw.Close(); err != nil {
			httpError(w, http.StatusInternalServerError, "could not build zip")
			return
		}
		sendDownload(w, "pages.zip", "application/zip", buf.Bytes())
		return
	}

	pdf, err := pdfops.ImagesToPDF(images)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "could not assemble PDF")
		return
	}
	sendDownload(w, "flattened.pdf", "application/pdf", pdf)
}

// handleFormData exports the current document's form fields as JSON or CSV.
func (s *Server) handleFormData(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	doc := s.doc
	s.mu.Unlock()
	if doc == nil {
		httpError(w, http.StatusNotFound, "no document open")
		return
	}
	if r.URL.Query().Get("format") == "csv" {
		data, err := pdfops.ExportFormCSV(doc.data)
		if err != nil {
			httpError(w, http.StatusInternalServerError, "could not export form")
			return
		}
		sendDownload(w, "form-data.csv", "text/csv", data)
		return
	}
	data, err := pdfops.ExportFormJSON(doc.data)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "could not export form")
		return
	}
	sendDownload(w, "form-data.json", "application/json", data)
}
