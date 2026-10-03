package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestTheConvenersOwnDocumentIsInTheCeremony — /pending 813. `doc.ceremony` had one writer,
// `installCeremonyResult`, and convene commits through `commitBarrier`, so the document the convener
// had just convened answered `inCeremony:false`: the signature-details button and the permanence
// notice stayed hidden on the "0 of N have signed" document they exist for.
func TestTheConvenersOwnDocumentIsInTheCeremony(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	ts, pdfPath := startServer(t)
	c, csrf := authedClient(t, ts)
	if code, body := postForCode(t, c, csrf, ts.URL+"/api/open", openRequest{Path: pdfPath}); code != http.StatusOK {
		t.Fatalf("open: %d %s", code, body)
	}
	me := myFingerprint(t, c, ts.URL)
	code, body := postForCode(t, c, csrf, ts.URL+"/api/ceremony/convene", conveneRequest{
		Roster: []convenePartyRequest{
			{Fingerprint: me, Label: "Me", Signs: true},
			{Fingerprint: strings.Repeat("7c", 32), Label: "Other", Signs: true},
		},
		Intent:        "We agree to the terms",
		Expires:       time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339),
		ConvenerSigns: true,
	})
	if code != http.StatusOK {
		t.Fatalf("convene returned %d: %s", code, body)
	}
	var out struct {
		Ceremony string      `json:"ceremony"`
		Doc      docResponse `json:"doc"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("convene response is not JSON (%v): %s", err, body)
	}
	// SETUP: the response describes the convened document, or the assertion below is asked of nothing.
	if out.Ceremony == "" || out.Doc.ID == "" {
		t.Fatalf("setup: the convene response names no ceremony or no document: %s", body)
	}
	if !out.Doc.InCeremony {
		t.Error("the convener's own convened document reports inCeremony:false — the details button " +
			"and the permanence notice stay hidden on the document they exist for")
	}
}
