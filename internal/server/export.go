package server

import (
	"archive/zip"
	"bytes"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"nib/internal/pdfops"
)

// handleExtract collects the requested pages of the posted document into a new
// PDF and returns it as a download. Unlike the page ops in handlePages it does
// NOT touch the open document — extracting is a derive-a-file action, like
// flatten/export, so the result is handed back for Save-as, not adopted.
func (s *Server) handleExtract(w http.ResponseWriter, r *http.Request) {
	cleanup, ok := parseMultipart(w, r, maxPDFBytes)
	if !ok {
		return
	}
	defer cleanup()
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
	cleanup, ok := parseMultipart(w, r, maxPDFBytes)
	if !ok {
		return
	}
	defer cleanup()
	pdfBytes, ok := formFileBytes(w, r, "pdf")
	if !ok {
		return
	}
	dir, ok := destDir(w, r)
	if !ok {
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
	writeSplitParts(w, dir, parts)
}

// handleSplitPages splits the posted document by page SEQUENCE — either every N
// pages or a set of custom ranges — into a chosen folder, one file per chunk.
// Like the bookmark split it never touches the open document. (The imposed-page
// splitters op:split/splitrects cut one sheet's geometry instead.)
func (s *Server) handleSplitPages(w http.ResponseWriter, r *http.Request) {
	cleanup, ok := parseMultipart(w, r, maxPDFBytes)
	if !ok {
		return
	}
	defer cleanup()
	pdfBytes, ok := formFileBytes(w, r, "pdf")
	if !ok {
		return
	}
	dir, ok := destDir(w, r)
	if !ok {
		return
	}
	n, err := pdfops.PageCount(pdfBytes)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "could not read document")
		return
	}
	spans, err := pdfops.PageSpans(r.FormValue("mode"), r.FormValue("every"), r.FormValue("ranges"), n)
	if err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	parts, err := pdfops.SplitBySpans(pdfBytes, spans, r.FormValue("prefix"))
	if err != nil {
		httpError(w, http.StatusBadRequest, "could not split: "+err.Error())
		return
	}
	writeSplitParts(w, dir, parts)
}

// destDir parses and validates the destination folder for any folder-writing
// operation (an absolute path is required). Shared by the two split handlers and
// the save-as writer, so one rule governs every folder the UI can write into —
// the writer used to carry its own copy of this, which drifted into a different
// error message and, more importantly, no containment check at all. It writes the
// error response itself, returning ok=false then.
//
// There is no dir == "" case to reject: filepath.Clean("") is ".".
func destDir(w http.ResponseWriter, r *http.Request) (string, bool) {
	dir := filepath.Clean(expandHome(strings.TrimSpace(r.FormValue("dir"))))
	if dir == "." || !filepath.IsAbs(dir) {
		httpError(w, http.StatusBadRequest, "destination must be an absolute folder")
		return "", false
	}
	return dir, true
}

// containedJoin joins a file name onto dir and reports whether the result stayed
// inside dir. Every name the UI supplies is ultimately free text — a typed
// save-as name, a bookmark title — so "../x" must not escape the folder the user
// chose to write into.
func containedJoin(dir, name string) (string, bool) {
	full := filepath.Join(dir, name)
	return full, filepath.Dir(full) == dir
}

// writeSplitParts writes each part as <dir>/<name>.pdf — atomic, and contained to
// dir (the name is user/title-derived, so the join is re-checked) — then replies
// with a JSON manifest. Shared by the bookmark and page-range split handlers.
func writeSplitParts(w http.ResponseWriter, dir string, parts []pdfops.SplitPart) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		httpError(w, http.StatusInternalServerError, "could not create folder")
		return
	}
	names := make([]string, 0, len(parts))
	for _, p := range parts {
		full, ok := containedJoin(dir, p.Name+".pdf") // the name is user/title-derived
		if !ok {
			httpError(w, http.StatusBadRequest, "unsafe file name")
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
	cleanup, ok := parseMultipart(w, r, maxPDFBytes)
	if !ok {
		return
	}
	defer cleanup()
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
		s.commitBarrier(pdf) // flatten makes covered/edited content unrecoverable
		writeJSON(w, s.docResponse())
		return
	}
	sendDownload(w, "flattened.pdf", "application/pdf", pdf)
}

// handleOptimize losslessly shrinks the current document (dedup + object-stream
// repacking; no image/quality change) and returns the smaller copy for save-as.
// The before/after byte sizes ride in headers so the UI can show the result.
func (s *Server) handleOptimize(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	doc := s.doc
	s.mu.Unlock()
	if doc == nil {
		httpError(w, http.StatusNotFound, "no document open")
		return
	}
	result, err := pdfops.Optimize(doc.data)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "could not optimize: "+err.Error())
		return
	}
	w.Header().Set("X-Original-Size", strconv.Itoa(len(doc.data)))
	w.Header().Set("X-New-Size", strconv.Itoa(len(result)))
	sendDownload(w, "optimized.pdf", "application/pdf", result)
}

// handleFormData exports the current document's form fields as JSON, CSV, or XFDF.
func (s *Server) handleFormData(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	doc := s.doc
	s.mu.Unlock()
	if doc == nil {
		httpError(w, http.StatusNotFound, "no document open")
		return
	}
	switch r.URL.Query().Get("format") {
	case "csv":
		data, err := pdfops.ExportFormCSV(doc.data)
		if err != nil {
			httpError(w, http.StatusInternalServerError, "could not export form")
			return
		}
		sendDownload(w, "form-data.csv", "text/csv", data)
		return
	case "xfdf":
		data, err := pdfops.ExportFormXFDF(doc.data)
		if err != nil {
			httpError(w, http.StatusInternalServerError, "could not export form")
			return
		}
		sendDownload(w, "form-data.xfdf", "application/vnd.adobe.xfdf", data)
		return
	}
	data, err := pdfops.ExportFormJSON(doc.data)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "could not export form")
		return
	}
	sendDownload(w, "form-data.json", "application/json", data)
}

// handleExtractImages bundles the current document's embedded images into a ZIP.
// The client fetches the bytes and saves them through the in-app picker; a
// document with no extractable images returns 200 with X-Image-Count: 0 and an
// empty body, so the client can say so rather than save an empty archive.
func (s *Server) handleExtractImages(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	doc := s.doc
	s.mu.Unlock()
	if doc == nil {
		httpError(w, http.StatusNotFound, "no document open")
		return
	}
	data, count, err := pdfops.ExtractImagesZip(doc.data)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "could not extract images")
		return
	}
	w.Header().Set("X-Image-Count", strconv.Itoa(count))
	if count == 0 {
		return
	}
	sendDownload(w, "images.zip", "application/zip", data)
}
