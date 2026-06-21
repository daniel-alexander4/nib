package server

import (
	"archive/zip"
	"bytes"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"nib/internal/pdfops"
	"nib/internal/sign"
)

// handleExtract collects the requested pages of the posted document into a new
// PDF and returns it as a download. Unlike the page ops in handlePages it does
// NOT touch the open document — extracting is a derive-a-file action, like
// flatten/export, so the result is handed back for Save-as, not adopted.
func (s *Server) handleExtract(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(maxPDFBytes); err != nil {
		httpError(w, http.StatusBadRequest, "could not parse upload")
		return
	}
	pdfBytes, ok := formFileBytes(w, r, "pdf")
	if !ok {
		return
	}
	pages := splitPages(r.FormValue("pages"))
	if len(pages) == 0 {
		httpError(w, http.StatusBadRequest, "select at least one page to extract")
		return
	}
	result, err := pdfops.Collect(pdfBytes, pages)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "could not extract pages: "+err.Error())
		return
	}
	sendDownload(w, "extract.pdf", "application/pdf", result)
}

// handleSplitBookmarks splits the posted document into one PDF per top-level
// bookmark and writes the files into a chosen folder (like PDFExplode: a scored
// orchestration in, one file per part out). Like extract it never touches the
// open document. Filenames come from the bookmark titles (sanitized + deduped in
// pdfops) with an optional prefix; each is confined to the chosen folder.
func (s *Server) handleSplitBookmarks(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(maxPDFBytes); err != nil {
		httpError(w, http.StatusBadRequest, "could not parse upload")
		return
	}
	pdfBytes, ok := formFileBytes(w, r, "pdf")
	if !ok {
		return
	}
	dir := filepath.Clean(expandHome(strings.TrimSpace(r.FormValue("dir"))))
	if dir == "" || dir == "." || !filepath.IsAbs(dir) {
		httpError(w, http.StatusBadRequest, "destination must be an absolute folder")
		return
	}
	parts, err := pdfops.SplitByBookmarks(pdfBytes, r.FormValue("prefix"))
	if err != nil {
		httpError(w, http.StatusInternalServerError, "could not split: "+err.Error())
		return
	}
	if len(parts) == 0 {
		httpError(w, http.StatusBadRequest, "this PDF has no bookmarks to split by")
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		httpError(w, http.StatusInternalServerError, "could not create folder")
		return
	}
	names := make([]string, 0, len(parts))
	for _, p := range parts {
		full := filepath.Join(dir, p.Name+".pdf")
		if filepath.Dir(full) != dir { // containment: the name is title-derived
			httpError(w, http.StatusBadRequest, "unsafe bookmark name")
			return
		}
		if err := writeFileAtomic(full, p.Data); err != nil {
			httpError(w, http.StatusInternalServerError, "could not write "+p.Name)
			return
		}
		names = append(names, p.Name+".pdf")
	}
	writeJSON(w, map[string]any{"dir": dir, "count": len(names), "names": names})
}

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
		b, err := readFormFile(fh)
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
			fw, err := zw.Create(fmt.Sprintf("page-%02d.png", i+1))
			if err == nil {
				_, err = fw.Write(img)
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
		sendDownload(w, "pages.zip", "application/zip", buf.Bytes())
		return
	}

	ws := r.MultipartForm.Value["pageW"]
	hs := r.MultipartForm.Value["pageH"]
	if len(ws) != len(images) || len(hs) != len(images) {
		httpError(w, http.StatusBadRequest, "page images and sizes must match")
		return
	}
	pages := make([]pdfops.RasterPage, len(images))
	for i, img := range images {
		pw, ph, ok := pageSize(ws[i], hs[i])
		if !ok {
			httpError(w, http.StatusBadRequest, "bad page size")
			return
		}
		pages[i] = pdfops.RasterPage{Image: img, W: pw, H: ph}
	}

	pdf, err := pdfops.ImagesToPDF(pages)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "could not assemble PDF")
		return
	}
	// reload=1 loads the flattened result back as the open document (the
	// guaranteed-inert sanitize floor) instead of downloading it.
	if r.FormValue("reload") == "1" {
		sig := sign.Verify(pdf)
		s.mu.Lock()
		if s.doc != nil {
			s.doc.data = pdf
			s.doc.sig = sig
		}
		s.mu.Unlock()
		writeJSON(w, s.docResponse())
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
