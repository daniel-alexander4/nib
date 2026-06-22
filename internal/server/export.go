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
	dir, ok := splitDir(w, r)
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
	if err := r.ParseMultipartForm(maxPDFBytes); err != nil {
		httpError(w, http.StatusBadRequest, "could not parse upload")
		return
	}
	pdfBytes, ok := formFileBytes(w, r, "pdf")
	if !ok {
		return
	}
	dir, ok := splitDir(w, r)
	if !ok {
		return
	}
	n, err := pdfops.PageCount(pdfBytes)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "could not read document")
		return
	}
	spans, err := splitSpans(r.FormValue("mode"), r.FormValue("every"), r.FormValue("ranges"), n)
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

// splitDir parses and validates the destination folder for a folder-writing split
// (an absolute path is required). It writes the error response itself, returning
// ok=false then.
func splitDir(w http.ResponseWriter, r *http.Request) (string, bool) {
	dir := filepath.Clean(expandHome(strings.TrimSpace(r.FormValue("dir"))))
	if dir == "" || dir == "." || !filepath.IsAbs(dir) {
		httpError(w, http.StatusBadRequest, "destination must be an absolute folder")
		return "", false
	}
	return dir, true
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
		full := filepath.Join(dir, p.Name+".pdf")
		if filepath.Dir(full) != dir { // containment: the name is user/title-derived
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

// splitSpans turns a page-split request into an ordered list of pdfcpu page
// selections, one per output file. mode "every" chunks the n-page document into
// runs of `every` pages; mode "ranges" parses a comma/space-separated list where
// each token ("4-8" or "5") becomes one file.
func splitSpans(mode, every, ranges string, n int) ([]string, error) {
	switch mode {
	case "every":
		k, err := strconv.Atoi(strings.TrimSpace(every))
		if err != nil || k < 1 {
			return nil, fmt.Errorf("pages-per-file must be a positive whole number")
		}
		var spans []string
		for from := 1; from <= n; from += k {
			thru := from + k - 1
			if thru > n {
				thru = n
			}
			spans = append(spans, spanLabel(from, thru))
		}
		return spans, nil
	case "ranges":
		var spans []string
		for _, tok := range strings.FieldsFunc(ranges, func(r rune) bool {
			return r == ',' || r == ' ' || r == '\n' || r == '\t'
		}) {
			from, thru, err := parseSpan(tok, n)
			if err != nil {
				return nil, err
			}
			spans = append(spans, spanLabel(from, thru))
		}
		if len(spans) == 0 {
			return nil, fmt.Errorf("enter at least one page range, e.g. 1-3, 4-8")
		}
		return spans, nil
	default:
		return nil, fmt.Errorf("unknown split mode")
	}
}

// parseSpan parses one range token ("A-B" or "A") and bounds it to 1..n.
func parseSpan(tok string, n int) (from, thru int, err error) {
	lo, hi, isRange := strings.Cut(strings.TrimSpace(tok), "-")
	from, err = strconv.Atoi(strings.TrimSpace(lo))
	if err != nil {
		return 0, 0, fmt.Errorf("bad page range %q", tok)
	}
	thru = from
	if isRange {
		if thru, err = strconv.Atoi(strings.TrimSpace(hi)); err != nil {
			return 0, 0, fmt.Errorf("bad page range %q", tok)
		}
	}
	if from < 1 || thru < from || thru > n {
		return 0, 0, fmt.Errorf("page range %q is outside 1-%d", tok, n)
	}
	return from, thru, nil
}

// spanLabel is the pdfcpu selection (and base filename) for a span: "5" for a
// single page, "4-8" for a run.
func spanLabel(from, thru int) string {
	if from == thru {
		return strconv.Itoa(from)
	}
	return fmt.Sprintf("%d-%d", from, thru)
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
