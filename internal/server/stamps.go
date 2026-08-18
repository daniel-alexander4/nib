package server

import (
	"net/http"

	"nib/internal/pdfops"
)

// stampsResponse answers one question: does the open document already carry a stamp
// layer? Deliberately a single bit and not a list of kinds — see pdfops.HasStampLayer
// for why the document cannot tell page numbers from a watermark.
type stampsResponse struct {
	Stamped bool `json:"stamped"`
}

// handleStamps reports whether the addressed document already carries a pdfcpu stamp
// layer, so the page-number and watermark dialogs can warn before adding a second one
// on top of the first.
//
// **Its own route, rather than a field on docResponse, and that is the whole design.**
// HasStampLayer is a full PDF parse. docResponse is built by every document route's
// reply — including /api/docs, which builds one PER OPEN DOCUMENT — and it already
// carries a cache (doc.flags) for exactly this reason: a parse per reply cost eight
// whole PDFs on an eight-tab boot restore. Answering here instead means the parse
// happens when a user opens a dialog, which is not a hot path, and needs no cache and
// no invalidation to go stale.
func (s *Server) handleStamps(w http.ResponseWriter, r *http.Request) {
	doc, ok := s.resolveDoc(w, r)
	if !ok {
		return
	}
	// A parse failure reads as "no stamp". The answer drives a warning and nothing
	// else, so refusing to open the dialog because a probe failed would be a worse
	// outcome than the doubled stamp the warning exists to prevent.
	stamped, err := pdfops.HasStampLayer(s.docBytes(doc))
	if err != nil {
		stamped = false
	}
	writeJSON(w, stampsResponse{Stamped: stamped})
}
