package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"nib/internal/pdfops"
	"nib/internal/testpdf"
)

// /api/inspect answers the open document's own description, and a document it cannot read is a 422 naming its
// cause — never a 200 with an empty description, which would read as "this file says nothing about itself".
func TestInspectRouteDescribesTheOpenDocumentAndRefusesOneItCannotRead(t *testing.T) {
	ts, srv := startServerWith(t)
	c, csrf := authedClient(t, ts)
	pdf, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	srv.setDoc(&document{data: pdf})
	resp, err := c.Get(ts.URL + "/api/inspect?auth=" + csrf)
	if err != nil {
		t.Fatal(err)
	}
	var got pdfops.Inspection
	_ = json.NewDecoder(resp.Body).Decode(&got)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || got.Fonts == nil || got.Info == nil {
		t.Fatalf("inspect: %d %+v", resp.StatusCode, got)
	}
	for k, v := range resp.Header {
		if strings.HasPrefix(strings.ToLower(k), "x-nib") {
			t.Errorf("a value rides in a header (%s: %v); the description belongs in the body only (ADR-193)", k, v)
		}
	}

	srv.setDoc(&document{data: []byte("%PDF-1.7\nnot a document at all\n")})
	resp, err = c.Get(ts.URL + "/api/inspect?auth=" + csrf)
	if err != nil {
		t.Fatal(err)
	}
	var refusal map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&refusal)
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity || refusal["cause"] != "document-unreadable" {
		t.Errorf("an unreadable document answered %d %v; want 422 document-unreadable", resp.StatusCode, refusal)
	}
}
