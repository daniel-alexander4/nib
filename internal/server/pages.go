package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"nib/internal/pdfops"
)

// handlePages applies a structural page operation (rotate, delete, reorder,
// append) to the posted document and makes the result the current document.
// The client posts its saved bytes (edits already baked) since these ops
// restructure the PDF in ways pdf.js cannot do client-side.
func (s *Server) handlePages(w http.ResponseWriter, r *http.Request) {
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

	var (
		result []byte
		err    error
	)
	switch r.FormValue("op") {
	case "rotate":
		deg, convErr := strconv.Atoi(r.FormValue("deg"))
		if convErr != nil {
			httpError(w, http.StatusBadRequest, "rotation must be a whole number of degrees")
			return
		}
		result, err = pdfops.Rotate(pdfBytes, pages, deg)
	case "delete":
		result, err = pdfops.RemovePages(pdfBytes, pages)
	case "reorder":
		result, err = pdfops.Collect(pdfBytes, pages)
	case "insertblank":
		afterPage, pErr := strconv.Atoi(r.FormValue("page"))
		if pErr != nil {
			httpError(w, http.StatusBadRequest, "insert needs a whole-number page")
			return
		}
		result, err = pdfops.InsertBlank(pdfBytes, afterPage)
	case "append":
		other, ok2 := formFileBytes(w, r, "append")
		if !ok2 {
			return
		}
		result, err = pdfops.Append(pdfBytes, other)
	case "insertpdf":
		beforePage, pErr := strconv.Atoi(r.FormValue("page"))
		if pErr != nil {
			httpError(w, http.StatusBadRequest, "insert needs a whole-number page")
			return
		}
		other, ok2 := formFileBytes(w, r, "append") // the secondary PDF reuses the append file field
		if !ok2 {
			return
		}
		result, err = pdfops.InsertPDF(pdfBytes, other, beforePage)
	case "duplicate":
		pageNum, pErr := strconv.Atoi(r.FormValue("page"))
		if pErr != nil {
			httpError(w, http.StatusBadRequest, "duplicate needs a whole-number page")
			return
		}
		result, err = pdfops.DuplicatePage(pdfBytes, pageNum)
	case "split":
		cols, cErr := strconv.Atoi(r.FormValue("cols"))
		rows, rErr := strconv.Atoi(r.FormValue("rows"))
		pageNum, pErr := strconv.Atoi(r.FormValue("page"))
		if cErr != nil || rErr != nil || pErr != nil {
			httpError(w, http.StatusBadRequest, "split needs whole-number page, cols and rows")
			return
		}
		resize := r.FormValue("resize") == "1" || r.FormValue("resize") == "true"
		result, err = pdfops.SplitPage(pdfBytes, pageNum, cols, rows, resize)
	case "splitrects":
		pageNum, pErr := strconv.Atoi(r.FormValue("page"))
		if pErr != nil {
			httpError(w, http.StatusBadRequest, "split needs a whole-number page")
			return
		}
		var rects [][4]float64
		if uErr := json.Unmarshal([]byte(r.FormValue("rects")), &rects); uErr != nil {
			httpError(w, http.StatusBadRequest, "could not read split regions")
			return
		}
		result, err = pdfops.SplitRegions(pdfBytes, pageNum, rects)
	case "crop":
		var frac [4]float64 // keep-box as page fractions [fx, fy, fw, fh], top-left origin
		if uErr := json.Unmarshal([]byte(r.FormValue("rect")), &frac); uErr != nil {
			httpError(w, http.StatusBadRequest, "could not read crop region")
			return
		}
		result, err = pdfops.Crop(pdfBytes, frac, pages)
	case "nup":
		n, nErr := strconv.Atoi(r.FormValue("n"))
		if nErr != nil {
			httpError(w, http.StatusBadRequest, "n-up needs a whole number of pages per sheet")
			return
		}
		result, err = pdfops.NUp(pdfBytes, n, r.FormValue("border") == "1")
	case "pagenum":
		start, _ := strconv.Atoi(r.FormValue("start"))
		pad, _ := strconv.Atoi(r.FormValue("pad"))
		size, _ := strconv.Atoi(r.FormValue("size"))
		result, err = pdfops.StampPageNumbers(pdfBytes, pdfops.PageNumberStyle{
			Position: r.FormValue("position"),
			Prefix:   r.FormValue("prefix"),
			Start:    start,
			Pad:      pad,
			OfTotal:  r.FormValue("total") == "1",
			Size:     size,
			Color:    r.FormValue("color"),
		})
	case "pagelabels":
		var ranges []pdfops.PageLabelRange
		if uErr := json.Unmarshal([]byte(r.FormValue("ranges")), &ranges); uErr != nil {
			httpError(w, http.StatusBadRequest, "could not read page-label ranges")
			return
		}
		result, err = pdfops.SetPageLabels(pdfBytes, ranges)
	case "normalize":
		result, err = pdfops.NormalizePageSizes(pdfBytes)
	default:
		httpError(w, http.StatusBadRequest, "unknown page operation")
		return
	}
	if err != nil {
		httpError(w, http.StatusInternalServerError, "page operation failed: "+err.Error())
		return
	}

	if !s.commitMutation(pdfBytes, result) {
		httpError(w, http.StatusNotFound, "no document open")
		return
	}
	writeJSON(w, s.docResponse())
}

// splitPages parses a comma-separated page selection ("1,3,5"); empty means all.
func splitPages(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
