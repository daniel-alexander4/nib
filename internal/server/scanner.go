package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"time"

	"nib/internal/pdfops"
	"nib/internal/sign"
)

// Scan straight from a scanner (/pending 903, ADR-213). Two routes: the list, asked when the dialog opens (a
// network scanner is found by discovery and that takes seconds, so `/api/status` never asks), and the scan,
// which installs ONE new document like any other open. The pages are PNGs from scanimage, each turned into a PDF by
// `Import` — the same door a picture file uses, so the pixel caps are the picture door's — and joined.

// handleScannerDevices answers the scanners scanimage can see, and remembers their names for handleScan.
func (s *Server) handleScannerDevices(w http.ResponseWriter, r *http.Request) {
	if !pdfops.ScannerAvailable() {
		httpError(w, http.StatusNotFound, "Nib could not find scanimage, so it cannot scan")
		return
	}
	devs, err := pdfops.ScanDevices(r.Context())
	if err != nil {
		httpError(w, http.StatusBadGateway, err.Error())
		return
	}
	known := make(map[string]bool, len(devs))
	for _, d := range devs {
		known[d.ID] = true
	}
	s.scanMu.Lock()
	s.scanKnown = known
	s.scanMu.Unlock()
	if devs == nil {
		devs = []pdfops.ScanDevice{} // [] and not null, so the window reads it as a list
	}
	writeJSON(w, map[string]any{"devices": devs, "modes": pdfops.ScanModes, "resolutions": pdfops.ScanResolutions})
}

type scannerRequest struct {
	Device string `json:"device"`
	DPI    int    `json:"dpi"`
	Mode   string `json:"mode"`
	Feeder bool   `json:"feeder"`
}

// handleScannerScan scans and installs the pages as one new, unsaved document.
func (s *Server) handleScannerScan(w http.ResponseWriter, r *http.Request) {
	var req scannerRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<12)).Decode(&req); err != nil {
		httpError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	s.scanMu.Lock()
	listed := s.scanKnown[req.Device]
	s.scanMu.Unlock()
	switch {
	case !pdfops.ScannerAvailable():
		httpError(w, http.StatusNotFound, "Nib could not find scanimage, so it cannot scan")
		return
	case !listed:
		httpError(w, http.StatusBadRequest, pdfops.ErrScanDevice.Error())
		return
	case !slices.Contains(pdfops.ScanModes, req.Mode):
		httpError(w, http.StatusBadRequest, "that scan mode is not one Nib offers")
		return
	case !slices.Contains(pdfops.ScanResolutions, req.DPI):
		httpError(w, http.StatusBadRequest, "that resolution is not one Nib offers")
		return
	}
	// Asked before the scan: a minute of the person's scanner for a page the install would then refuse.
	if err := s.roomToOpen(); err != nil {
		httpError(w, http.StatusConflict, err.Error())
		return
	}
	pages, err := pdfops.ScanPages(r.Context(), pdfops.ScanOptions{Device: req.Device, DPI: req.DPI, Mode: req.Mode, Feeder: req.Feeder})
	if err != nil {
		status := http.StatusBadGateway
		switch {
		case errors.Is(err, pdfops.ErrScanNoFeeder), errors.Is(err, pdfops.ErrScanDevice):
			status = http.StatusBadRequest
		case errors.Is(err, pdfops.ErrConvertedTooLarge), errors.Is(err, pdfops.ErrConverterOverMemory):
			status = http.StatusRequestEntityTooLarge
		}
		httpError(w, status, err.Error())
		return
	}
	pdfs := make([][]byte, 0, len(pages))
	for i, png := range pages {
		src, size := pdfops.BytesSource(png)
		res, ok := s.importOrRefuse(w, r.Context(), src, size, fmt.Sprintf("page-%03d.png", i+1), s.importOptions("scan"))
		if !ok {
			return
		}
		pdfs = append(pdfs, res.PDF)
	}
	data := pdfs[0]
	if len(pdfs) > 1 {
		var cerr error
		if data, cerr = pdfops.Combine(pdfs); cerr != nil {
			httpError(w, http.StatusUnprocessableEntity, "the scanned pages could not be joined: "+cerr.Error())
			return
		}
	}
	// Pathless: a scan is not a file until the person saves it, and then Save As lists it (/pending 957).
	name := "Scan " + time.Now().Format("2006-01-02 15.04") + ".pdf"
	installed, err := s.addDocCapped(&document{path: "", name: name, data: data, sig: sign.Verify(data)})
	if err != nil {
		httpError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, s.docResponse(installed))
}
