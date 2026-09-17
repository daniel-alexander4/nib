package server

import (
	"archive/zip"
	"bytes"
	"fmt"
	"log"
	"net/http"
	"nib/internal/atomicfile"
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
	// Resolved for its PATH, not for its bytes — the split still cuts the posted document, which
	// carries edits this server has not been told about. What only the registry knows is the file
	// that document came from, and that is the one file no part may be written over (/pending 569).
	// Before the body, per the ordering `bodyfirst_test.go` states for the routes that commit:
	// a request addressed to a document that is gone should not cost a maxPDFBytes parse.
	src, ok := s.resolveDoc(w, r)
	if !ok {
		return
	}
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
	writeSplitParts(w, src.path, dir, parts)
}

// handleSplitPages splits the posted document by page SEQUENCE — either every N
// pages or a set of custom ranges — into a chosen folder, one file per chunk.
// Like the bookmark split it never touches the open document. (The imposed-page
// splitters op:split/splitrects cut one sheet's geometry instead.)
func (s *Server) handleSplitPages(w http.ResponseWriter, r *http.Request) {
	// For its path, and before the body — see handleSplitBookmarks.
	src, ok := s.resolveDoc(w, r)
	if !ok {
		return
	}
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
	writeSplitParts(w, src.path, dir, parts)
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
//
// src is the file the document being split lives at on disk, or "" for a document that was
// uploaded through the browser and has none.
//
// # Two passes, and src, because this door CAN destroy the user's document (/pending 569)
//
// The CLI's writer was measured replacing its own input, and this one was reported to have the
// same shape by reading. The shape is the same and the mechanism is not: nothing here opens a
// file, so there is no descriptor to clobber and no input argument to compare against. What made
// it the same defect is `document.path` — the file the open document was loaded from is in the
// folder the user is about to fill, under a name a prefix can reproduce. Driven rather than
// reasoned: a `--ranges 1-2 --prefix foo` split of an open `foo1-2.pdf` into its own folder
// replaced the 1,239-byte file with the 1,076-byte part, status 200.
//
// Every output path is built and checked before the first is written, so a refusal leaves the
// folder as it found it rather than half filled.
func writeSplitParts(w http.ResponseWriter, src, dir string, parts []pdfops.SplitPart) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		httpError(w, http.StatusInternalServerError, "could not create folder")
		return
	}
	outs := make([]string, len(parts))
	for i, p := range parts {
		full, ok := containedJoin(dir, p.Name+".pdf") // the name is user/title-derived
		if !ok {
			httpError(w, http.StatusBadRequest, "unsafe file name")
			return
		}
		outs[i] = full
	}
	// 400, not 409: 409 is this server's answer for "that document is no longer open" and the
	// document here is very much open — it is the thing being protected. The client already warns
	// *"Files with the same name will be replaced"* before a split, and the one file a user cannot
	// mean by that is the document on their screen, so the message names it rather than repeating
	// the general warning. Its own voice, per ADR-009; the door decided WHICH part.
	if clash, ok := pdfops.OutputOverwritingSource(src, outs); ok {
		httpError(w, http.StatusBadRequest, "“"+filepath.Base(clash)+"” is the document you are "+
			"splitting — that part would replace the whole document with one piece of it. "+
			"Choose a different prefix or folder.")
		return
	}
	names := make([]string, 0, len(parts))
	for i, p := range parts {
		full := outs[i]
		// Re-derivable: every part comes from the document still open in this process, so an
		// export can simply be run again. Atomic, not durable — see atomicfile.Write.
		//
		// **The CLI's split writes the same parts through a DURABLE door, and that is settled
		// rather than an oversight (/pending 550).** `internal/cli`'s `writeSplitFiles` goes
		// through `writeNamed` → `atomicfile.ReplaceDurable`, which fsyncs, resolves a symlink at
		// the destination, and carries an existing file's mode. /pending 508 measured only the
		// fsync — 16.8–20.5 ms a part against 73–106 µs here, ~5 s on a 280-part split — and
		// proposed moving the CLI to this door to match. Refused: the link resolution is the half
		// that was not costed, and `internal/cli` acquired it by fixing a real silent data loss
		// (/pending 515). Two doors, two reasons, both written down; the symmetry would have cost
		// more than it bought.
		//
		// **The mode WAS the other disagreement, and it is settled** (/pending 570). This door
		// wrote new parts 0600 and the CLI's wrote them 0644, so where a split's output landed
		// depended on which surface produced it — a hard-coded literal at each door, chosen by
		// nobody. `pdfops.SplitPartMode` is the one answer and its own comment carries the
		// reasoning and the exposure; both doors name it here rather than agreeing by coincidence.
		if err := atomicfile.Write(full, p.Data, pdfops.SplitPartMode); err != nil {
			httpError(w, http.StatusInternalServerError, "could not write "+p.Name)
			return
		}
		names = append(names, p.Name+".pdf")
	}
	// **`names` was published and read by nobody** (/pending 419). The client reads `count`
	// and `dir`; the full list was the one field nothing wanted, and it is the one that grows
	// with the document — a 500-page split shipped 500 filenames for a caller that renders
	// "500 files in ~/nib". Dropped rather than parked: nothing anywhere reads it, and the
	// filenames are on disk in `dir` for anyone who does.
	writeJSON(w, map[string]any{"dir": dir, "count": len(names)})
}

// handleAssemble turns client-rendered page images into a download: a flattened
// image-PDF (the rasterize flatten / guaranteed-flat export) or a ZIP of the
// images. The browser renders each page to a PNG; the server packages them.
func (s *Server) handleAssemble(w http.ResponseWriter, r *http.Request) {
	// **Refuse a request addressed to a document this server no longer holds BEFORE reading the
	// body.** Without this the handler parses up to maxPDFBytes and runs the whole PDF operation
	// before the resolve at the commit discovers the document is gone — real work, on a 128 MiB
	// document, for a request that was never going to land (/pending 261).
	//
	// It is ADVISORY and the resolve at the commit stays authoritative: the document can be
	// closed while the body is being read, so this one cannot be trusted to still hold by the
	// time there is something to install. Do not "optimise" by reusing the document it resolves.
	src, ok := s.resolveDoc(w, r)
	if !ok {
		return
	}
	// The flattened result's title is the source document's name. Only the NAME is taken, as a
	// string, and the document itself is not held: the warning above stands whole — what may not be
	// reused is the resolved document as a commit target, because it can be closed while the body
	// is read. A title is a label on bytes this handler is building and nothing downstream depends
	// on its freshness.
	srcName := src.displayName()
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
	if err == nil {
		// Best-effort: the pages assembled, and a metadata write must not throw that away.
		var terr error
		if pdf, terr = pdfops.TitleFromName(pdf, srcName); terr != nil {
			log.Printf("assemble: %v", terr)
		}
	}
	if err != nil {
		httpError(w, http.StatusInternalServerError, "could not assemble PDF")
		return
	}
	// reload=1 loads the flattened result back as the open document (the
	// guaranteed-inert sanitize floor) instead of downloading it.
	if r.FormValue("reload") == "1" {
		// flatten makes covered/edited content unrecoverable
		// Resolve the document this result is being installed INTO. These four routes
		// never read the open document — they work from posted bytes — so before the
		// registry there was nothing to resolve and "the open one" was unambiguous. It
		// is not any more: without this, an operation addressed to one document commits
		// its result into another.
		doc, ok := s.resolveDoc(w, r)
		if !ok {
			return
		}
		// **The field is spelled out here rather than behind a helper or a constant, and that is
		// deliberate.** /pending 447's guard reads `r.FormValue("…")` as an `*ast.BasicLit` and
		// follows only helpers whose names match its prefix list — so a shared `acceptsSignatureLoss(r)`
		// or a named constant would make this field invisible to the one check that verifies a client
		// still sends it. Three routes can erase a signature; each states so in its own words.
		acceptLoss := r.FormValue("acceptSignatureLoss") == "1"
		if err := s.commitBarrier(doc, pdf, acceptLoss); wroteCommitFailure(w, err) {
			return
		}
		writeJSON(w, s.docResponse(doc))
		return
	}
	sendDownload(w, "flattened.pdf", "application/pdf", pdf)
}

// handleOptimize losslessly shrinks the current document (dedup + object-stream
// repacking; no image/quality change) and returns the smaller copy for save-as.
// The before/after byte sizes ride in headers so the UI can show the result.
func (s *Server) handleOptimize(w http.ResponseWriter, r *http.Request) {
	doc, ok := s.resolveDoc(w, r)
	if !ok {
		return
	}
	// Snapshotted once: the reported original size must describe the bytes that
	// were actually optimized, not whatever a concurrent page op left behind.
	before := s.docBytes(doc)
	result, err := pdfops.Optimize(before)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "could not optimize: "+err.Error())
		return
	}
	w.Header().Set("X-Original-Size", strconv.Itoa(len(before)))
	w.Header().Set("X-New-Size", strconv.Itoa(len(result)))
	sendDownload(w, "optimized.pdf", "application/pdf", result)
}

// handleFormData exports the current document's form fields as JSON, CSV, or XFDF.
func (s *Server) handleFormData(w http.ResponseWriter, r *http.Request) {
	doc, ok := s.resolveDoc(w, r)
	if !ok {
		return
	}
	switch r.URL.Query().Get("format") {
	case "csv":
		data, err := pdfops.ExportFormCSV(s.docBytes(doc))
		if err != nil {
			httpError(w, http.StatusInternalServerError, "could not export form")
			return
		}
		sendDownload(w, "form-data.csv", "text/csv", data)
		return
	case "xfdf":
		data, err := pdfops.ExportFormXFDF(s.docBytes(doc))
		if err != nil {
			httpError(w, http.StatusInternalServerError, "could not export form")
			return
		}
		sendDownload(w, "form-data.xfdf", "application/vnd.adobe.xfdf", data)
		return
	}
	data, err := pdfops.ExportFormJSON(s.docBytes(doc))
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
	doc, ok := s.resolveDoc(w, r)
	if !ok {
		return
	}
	data, count, err := pdfops.ExtractImagesZip(s.docBytes(doc))
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
