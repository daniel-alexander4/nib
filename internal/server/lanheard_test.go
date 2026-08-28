package server

import (
	"encoding/json"
	"net/http"
	"testing"
)

// TestTheLinkReportAnswersWithoutPinsAndSaysWhy — /pending 300's instrument.
//
// Nothing in the tree could answer "what does this machine hear on the link?", and three separate
// investigations have wanted it: a ceremony falling through to the DHT, a peer dialled on the wrong
// transport, and a relay that works in one order and not the other. Every one is a question about
// the link, and the only evidence available was the failure itself.
//
// **What is asserted here is the part that holds with no network at all**, which is deliberate: a
// browse needs a usable interface and a multicast group that is not swallowed, and a test that
// required either would skip on the machines most likely to run it. What must hold everywhere is
// that the route ANSWERS rather than faulting, and that an empty answer is distinguishable from a
// broken one — the whole point of an instrument is that "nothing there" and "could not look" are
// different sentences.
func TestTheLinkReportAnswersWithoutPinsAndSaysWhy(t *testing.T) {
	ts, _ := startServer(t)
	c, _ := authedClient(t, ts)

	r, err := c.Get(ts.URL + "/api/lan/heard")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	if r.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 — a diagnostic that faults is not a diagnostic", r.StatusCode)
	}
	var got lanHeardResponse
	if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	// A fresh vault has no pins, and L1 is why that means an empty answer: `resolve` matches an
	// announcement's six-word name against the PINS, and there is no path from a name to a
	// fingerprint. So a machine that has pinned nobody cannot recognise anybody, however loud the
	// link is — and the route has to say that rather than returning a bare empty list.
	if len(got.Heard) != 0 {
		t.Errorf("a vault with no pins heard %d peer(s) — this route can only resolve names it "+
			"already holds fingerprints for, so anything here means it is reporting something it "+
			"could not have recognised", len(got.Heard))
	}
	if got.Note == "" {
		t.Error("an empty result came back with no note. 'Nothing is on the link' and 'this " +
			"machine cannot recognise anything' are different answers, and an instrument that " +
			"cannot tell them apart is the thing it was built to replace")
	}
	if got.WindowMs <= 0 {
		t.Errorf("windowMs = %d — a reader cannot tell 'nobody answered' from 'it barely listened' "+
			"without it", got.WindowMs)
	}
}

// TestTheLinkReportIsBehindTheVault — it browses, so it is not public.
func TestTheLinkReportIsBehindTheVault(t *testing.T) {
	ts, _ := startServer(t)
	// No authedClient: a bare request, as an unauthenticated caller would make it.
	r, err := http.Get(ts.URL + "/api/lan/heard")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	if r.StatusCode == http.StatusOK {
		t.Error("an unauthenticated caller got a link report. It opens a socket and listens on " +
			"the local network, which is not something an unauthenticated request may make this " +
			"machine do.")
	}
}
