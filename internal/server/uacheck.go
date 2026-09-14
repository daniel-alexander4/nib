package server

import (
	"net/http"

	"nib/internal/uacheck"
)

// uaResultView is one clause's verdict as the report shows it.
//
// **The verdict travels as a string**, because `uacheck.Verdict` is an integer whose zero value is
// `NotRun` — a client reading `0` would have to know that mapping, and a client that guessed would be
// guessing about law 4's most important distinction.
type uaResultView struct {
	Clause  string `json:"clause"`
	Summary string `json:"summary"`
	Verdict string `json:"verdict"`
	Why     string `json:"why,omitempty"`
	Where   string `json:"where,omitempty"`
}

// uaReportResponse is the accessibility report for the open document.
type uaReportResponse struct {
	Conformant bool           `json:"conformant"`
	Results    []uaResultView `json:"results"`
	Refusals   []string       `json:"refusals"`
}

// handleUACheck reports the open document against PDF/UA-1 — `PLAN-accessibility.md` P07.S06.
//
// It is read-only, so it is a GET, like the hidden-content scan beside it. It reaches the same door as
// `nib ua` (`uacheck.CheckForUA`), and a guard at the repo root refuses a direct `uacheck.Check` call
// here, so the UI and the CLI cannot come to hold two readings of what conformance means.
func (s *Server) handleUACheck(w http.ResponseWriter, r *http.Request) {
	doc, ok := s.resolveDoc(w, r)
	if !ok {
		return
	}
	rep, refusals, err := uacheck.CheckForUA(s.docBytes(doc))
	if err != nil {
		httpError(w, http.StatusUnprocessableEntity, "could not check the document: "+err.Error())
		return
	}
	out := uaReportResponse{Conformant: rep.Conformant(), Refusals: refusals}
	if out.Refusals == nil {
		out.Refusals = []string{}
	}
	for _, res := range rep.Results {
		out.Results = append(out.Results, uaResultView{
			Clause:  res.Clause,
			Summary: uacheck.SummaryOf(res.Clause),
			Verdict: res.Verdict.String(),
			Why:     res.Why,
			Where:   res.Where,
		})
	}
	writeJSON(w, out)
}
