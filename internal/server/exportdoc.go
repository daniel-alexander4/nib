package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"nib/internal/docmodel"
	"nib/internal/docmodel/read"
	"nib/internal/docwrite/docx"
	"nib/internal/docwrite/epub"
	"nib/internal/docwrite/html"
	"nib/internal/docwrite/markdown"
	"nib/internal/docwrite/odt"
	"nib/internal/docwrite/text"
	"nib/internal/pdfops"
	"nib/internal/sign"
)

// maxExportFactList caps each list X-Nib-Export carries, so a hostile file cannot grow the header past what a browser
// accepts; a capped list says so (`truncated`). The same bound as X-Nib-Revision's.
const maxExportFactList = maxRevisionFactList

// exportFormats is the one list of document formats the route writes: the query word, the content type, and the
// writer. The window's Format line is the same three, from the format table's `exports` rows (W10).
var exportFormats = map[string]struct {
	mime  string
	write func(context.Context, docmodel.Document) ([]byte, error)
}{
	"md":   {"text/markdown; charset=utf-8", markdown.Write}, // ADR-194 writes
	"html": {"text/html; charset=utf-8", html.Write},
	"txt":  {"text/plain; charset=utf-8", text.Write},                                               // ADR-194 writes
	"docx": {"application/vnd.openxmlformats-officedocument.wordprocessingml.document", docx.Write}, // ADR-194 writes
	"odt":  {"application/vnd.oasis.opendocument.text", odt.Write},                                  // ADR-194 writes
	"epub": {"application/epub+zip", epub.Write},                                                    // ADR-194 writes
}

// exportPage is one flagged page in the header.
type exportPage struct {
	Page  int    `json:"page"`
	Cause string `json:"cause"`
}

// ExportFacts is what the X-Nib-Export header says about an export (PLAN-import-export W11, D23, D28). Integers, enums
// and page numbers only — never a name, a value or a word of the document (W3) — built field by field from
// `docmodel.Report`, and the one struct the window's wording table is checked against. Source is "tree", "proposal" or
// "mixed". Signed says the file the export was read from carried a signature: the export is a new, unsigned document.
type ExportFacts struct {
	V                   int          `json:"v"`
	Source              string       `json:"source"`
	DroppedLinks        int          `json:"droppedLinks"`
	PlaceholderImages   int          `json:"placeholderImages"`
	FlaggedPages        []exportPage `json:"flaggedPages"`
	ScannedPages        []int        `json:"scannedPages"`
	CoveredText         int          `json:"coveredText"`
	HiddenText          int          `json:"hiddenText"`
	OffPage             int          `json:"offPage"`
	UnreadGlyphs        int          `json:"unreadGlyphs"`
	TablesAsText        int          `json:"tablesAsText"`
	Signed              bool         `json:"signed"`
	UnappliedRedactions int          `json:"unappliedRedactions"`
	Truncated           bool         `json:"truncated"`
}

func sum(m map[string]int) (n int) {
	for _, v := range m {
		n += v
	}
	return n
}

// factsOf projects the reader's Report onto the header.
func factsOf(rep docmodel.Report, signed bool, unapplied int) ExportFacts {
	f := ExportFacts{
		V: 1, FlaggedPages: []exportPage{}, ScannedPages: []int{}, Source: rep.Source, DroppedLinks: sum(rep.DroppedLinks), PlaceholderImages: sum(rep.Placeholders),
		CoveredText: rep.CoveredText + rep.MaybeCovered, HiddenText: rep.HiddenText + rep.HiddenLayer, OffPage: rep.OffPage,
		UnreadGlyphs: rep.UnreadGlyphs, TablesAsText: rep.TablesAsText, Signed: signed, UnappliedRedactions: unapplied,
	}
	for _, p := range rep.FlaggedPages {
		f.FlaggedPages = append(f.FlaggedPages, exportPage{Page: p.Page, Cause: p.Cause})
	}
	f.ScannedPages = append(f.ScannedPages, rep.ScannedPages...)
	sort.Ints(f.ScannedPages)
	if len(f.FlaggedPages) > maxExportFactList {
		f.FlaggedPages, f.Truncated = f.FlaggedPages[:maxExportFactList], true
	}
	if len(f.ScannedPages) > maxExportFactList {
		f.ScannedPages, f.Truncated = f.ScannedPages[:maxExportFactList], true
	}
	return f
}

// exportTitle is the name the user confirmed, folded to one line and capped; else the literal "Document" (X9). It is
// never the PDF's /Title and never a source path.
func exportTitle(s string) string {
	t := docmodel.OneLine(s)
	if r := []rune(t); len(r) > 200 {
		t = string(r[:200])
	}
	if t == "" {
		return "Document"
	}
	return t
}

// handleExportDocument answers POST /api/export/document?format=md|html|txt|docx|odt|epub&title=&redactionsPending=: the open
// document as a Markdown, HTML, text, Word, OpenDocument or EPUB file, read through the ONE reader (`docmodel/read.Read`, D16) in one pass, with
// what it did not carry in X-Nib-Export (D23). It reads the server's copy: overlay marks and typed field values the
// window has not baked are not in it (W8), and the window says so on the Format line. 422 names a refusal about the
// document; 409 is ADR-004's "not that document" and also the document having changed while it was read.
func (s *Server) handleExportDocument(w http.ResponseWriter, r *http.Request) {
	doc, ok := s.resolveDoc(w, r)
	if !ok {
		return
	}
	format, known := exportFormats[r.URL.Query().Get("format")]
	if !known {
		httpError(w, http.StatusBadRequest, "format must be md, html, txt, docx, odt or epub")
		return
	}
	unapplied, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("redactionsPending")))
	if err != nil || unapplied < 0 {
		unapplied = 0
	}
	// A second request for the same read (the Save dialog asks again for the format it was told to write) names the
	// revision the first answered with; a document that has moved since is refused as ADR-146 refuses a posted operation
	// (/pending 941 #5). None named is last-writer-wins, as for the CLI and a Go test. This is a read, not a commit, so it
	// does not build an `echoedBase` (that is the two posted-bytes routes' alone).
	var want uint64
	named := false
	if v := r.URL.Query().Get("baseRev"); v != "" {
		n, perr := strconv.ParseUint(v, 10, 64)
		if perr != nil {
			httpError(w, http.StatusBadRequest, errBaseRev.Error())
			return
		}
		want, named = n, true
	}
	var data []byte
	var rev uint64
	if !s.snapshotRegistered(w, doc, func() { data, rev = doc.data, doc.rev }) {
		return
	}
	if named && rev != want {
		httpError(w, http.StatusConflict, errDocChanged.Error())
		return
	}
	// doc.data is replaced wholesale and never mutated in place (document.install), so the slice is safe to read
	// outside the lock; `rev` is the count the read was made at, and the answer carries it (X-Nib-Doc-Rev) for a later request to name.
	model, err := read.Read(r.Context(), data, read.Options{Title: exportTitle(r.URL.Query().Get("title"))})
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) && r.Context().Err() != nil {
		return // the caller went away; nothing is listening for an answer
	}
	if err != nil {
		writeJSONStatus(w, http.StatusUnprocessableEntity, map[string]string{"error": "Nib could not read this document's text", "cause": "unreadable"})
		return
	}
	body, err := format.write(r.Context(), model)
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) && r.Context().Err() != nil {
		return
	}
	if err != nil {
		httpError(w, http.StatusInternalServerError, "could not write the document")
		return
	}
	if len(body) == 0 {
		writeJSONStatus(w, http.StatusUnprocessableEntity, map[string]string{"error": "This document has no text or pictures to export", "cause": "empty"})
		return
	}
	moved := false
	if !s.snapshotRegistered(w, doc, func() { moved = doc.rev != rev }) {
		return
	}
	if moved {
		httpError(w, http.StatusConflict, "the document changed while it was being read; export again")
		return
	}
	hdr, err := json.Marshal(factsOf(model.Report, sign.HasSignatureBlob(data), unapplied))
	if err != nil {
		httpError(w, http.StatusInternalServerError, "could not describe the export")
		return
	}
	w.Header().Set("X-Nib-Export", string(hdr))
	w.Header().Set("X-Nib-Doc-Rev", strconv.FormatUint(rev, 10)) // the count this read was made on (ADR-146)
	w.Header().Set("Content-Type", format.mime)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	_, _ = w.Write(body)
}

// handleExportSVG answers POST /api/export/svg?page=N: one page of the open document drawn as an SVG file by poppler's
// pdftocairo (PLAN-import-export P05.S01b, ADR-205). It draws the server's copy, so a mark the window has not baked is
// not in it — the Format line says so, as it does for Document as… (W8). 503 is a missing tool, 422 a refusal about the
// page, 409 ADR-004's "not that document" and also the document having changed while it was drawn.
func (s *Server) handleExportSVG(w http.ResponseWriter, r *http.Request) {
	doc, ok := s.resolveDoc(w, r)
	if !ok {
		return
	}
	page, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("page")))
	if err != nil || page < 1 {
		httpError(w, http.StatusBadRequest, "page must be a page number, 1 or more")
		return
	}
	var data []byte
	var rev uint64
	if !s.snapshotRegistered(w, doc, func() { data, rev = doc.data, doc.rev }) {
		return
	}
	body, err := pdfops.ExportPageSVG(r.Context(), data, page)
	switch {
	case err == nil:
	case errors.Is(err, pdfops.ErrPdftocairoMissing):
		writeJSONStatus(w, http.StatusServiceUnavailable, map[string]string{"error": "Nib " + pdfops.ToolPdftocairo.NotFound() + ", so it cannot write an SVG", "cause": "missing-tool"})
		return
	case errors.Is(err, pdfops.ErrSVGPage):
		writeJSONStatus(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error(), "cause": "page"})
		return
	case errors.Is(err, pdfops.ErrSVGUnreadable), errors.Is(err, pdfops.ErrConvertedTooLarge):
		writeJSONStatus(w, http.StatusUnprocessableEntity, map[string]string{"error": "pdftocairo could not draw that page as an SVG", "cause": "unreadable"})
		return
	default:
		httpError(w, http.StatusInternalServerError, "could not write the SVG")
		return
	}
	moved := false
	if !s.snapshotRegistered(w, doc, func() { moved = doc.rev != rev }) {
		return
	}
	if moved {
		httpError(w, http.StatusConflict, "the document changed while it was being drawn; export again")
		return
	}
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	_, _ = w.Write(body)
}
