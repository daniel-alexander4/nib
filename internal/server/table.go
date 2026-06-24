package server

import (
	"encoding/json"
	"io"
	"net/http"

	"nib/internal/pdfops"
)

// handleTableExport turns a table grid the browser extracted from the current page
// (pdf.js text positions clustered into rows/columns) into a downloadable
// spreadsheet. Extraction is client-side because only pdf.js can read PDF text;
// the server just serializes the grid (CSV or XLSX). It derives a new artifact
// and never touches the open document.
func (s *Server) handleTableExport(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Grid [][]string `json:"grid"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, maxPDFBytes)).Decode(&body); err != nil {
		httpError(w, http.StatusBadRequest, "could not read table data")
		return
	}
	if len(body.Grid) == 0 {
		httpError(w, http.StatusBadRequest, "no table data")
		return
	}
	if r.URL.Query().Get("format") == "csv" {
		data, err := pdfops.GridToCSV(body.Grid)
		if err != nil {
			httpError(w, http.StatusInternalServerError, "could not build CSV")
			return
		}
		sendDownload(w, "table.csv", "text/csv", data)
		return
	}
	data, err := pdfops.GridToXLSX(body.Grid)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "could not build spreadsheet")
		return
	}
	sendDownload(w, "table.xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", data)
}
