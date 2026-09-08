package server

import (
	"encoding/json"
	"net/http"
	"testing"
)

// P03.S02 — the convener's setup draft (D4).
//
// # The defect this ends
//
// The first durable state in a ceremony is the SIGNED RECORD, so a convener who closes Nib halfway
// through setup retypes the roster and the recital. D4: *"without it the sheet is a form that cannot
// be left, which is the abandonment case every long form has."*
//
// # Why these tests are about the VAULT and not about a browser
//
// The exit criterion is *"setup survives closing and reopening Nib"* — a process boundary, not a
// page reload. `localStorage` cannot meet it at all: it is keyed by origin and `cmd/nib` binds
// `127.0.0.1:0`, a random port by design, so a new run is a new origin and an empty store. What
// survives a restart is what the vault holds, so that is what is asserted here.

func draftGet(t *testing.T, c *http.Client, base string) string {
	t.Helper()
	res, err := c.Get(base + "/api/ceremony/draft")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var body draftResponse
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return body.Draft
}

// TestTheDraftSurvivesTheProcess is the acceptance clause, driven across a real restart.
//
// **A second `Server` over the same HOME is what a restart is**, and it is the only shape that can
// fail here: a draft held in memory, in a page, or in anything keyed to the running process passes
// every check made without one. That is exactly the test `localStorage` would have passed.
func TestTheDraftSurvivesTheProcess(t *testing.T) {
	ts, srv := startServerWith(t)
	c, csrf := authedClient(t, ts)

	// SETUP: nothing stored, so the assertion below cannot be satisfied by a leftover.
	if got := draftGet(t, c, ts.URL); got != "" {
		t.Fatalf("setup: a draft is already stored (%q), so surviving proves nothing", got)
	}

	const draft = `{"intent":"We agree to the lease of 14 Elm Row","roster":[{"fingerprint":"ab","picked":true}]}`
	if code, body := postForCode(t, c, csrf, ts.URL+"/api/ceremony/draft",
		draftRequest{Draft: draft}); code != http.StatusOK {
		t.Fatalf("saving the draft returned %d: %s", code, body)
	}
	if got := draftGet(t, c, ts.URL); got != draft {
		t.Fatalf("the draft did not round-trip in one process: got %q", got)
	}

	// THE RESTART. A second server over the same $HOME opens the same vault — which is what
	// "closing and reopening Nib" is, and what nothing kept in a page can survive.
	ts2, _ := startServerOverSameHome(t, srv.configDir)
	c2, _ := reopenedClient(t, ts2)
	if got := draftGet(t, c2, ts2.URL); got != draft {
		t.Errorf("after a restart the draft is %q, want it intact. The convener retypes the roster "+
			"and the recital, which is the abandonment case D4 exists to end — and it is the case "+
			"anything kept in the page would pass, because `cmd/nib` binds a random port and a new "+
			"origin has an empty store", got)
	}
}

// TestAnEmptyDraftIsNotStored — an emptied form and no form are one state, by construction.
//
// The vault's `SetCeremonyDraft` clears on empty rather than storing `""`, so that "the user emptied
// it" and "there is nothing to restore" cannot disagree. Without that, `omitempty` drops the row on
// write and the two spellings of the same state differ by whether the file happened to be saved.
func TestAnEmptyDraftIsNotStored(t *testing.T) {
	ts, _ := startServerWith(t)
	c, csrf := authedClient(t, ts)

	if code, _ := postForCode(t, c, csrf, ts.URL+"/api/ceremony/draft",
		draftRequest{Draft: `{"intent":"something"}`}); code != http.StatusOK {
		t.Fatal("setup: the first save failed, so clearing it proves nothing")
	}
	if draftGet(t, c, ts.URL) == "" {
		t.Fatal("setup: the draft did not store, so emptying it below cannot be observed")
	}

	if code, body := postForCode(t, c, csrf, ts.URL+"/api/ceremony/draft",
		draftRequest{Draft: ""}); code != http.StatusOK {
		t.Fatalf("emptying the draft returned %d: %s", code, body)
	}
	if got := draftGet(t, c, ts.URL); got != "" {
		t.Errorf("emptying the form left %q behind. An abandoned draft records who the user was "+
			"about to transact with and what they were about to agree, so a form the user cleared "+
			"must not keep it", got)
	}
}

// TestClearingTheDraftRemovesIt covers `ClearCeremonyDraft` DIRECTLY, and it exists because a probe
// showed nothing did.
//
// **That door has no production caller yet — P03.S03 is its user** — so it is a door built one slice
// before the thing that opens it, the same shape `observables_test.go` records for `ceremony.Anchor`.
// Making its clear a no-op left the whole file green, because `TestAnEmptyDraftIsNotStored` goes
// through `SetCeremonyDraft("")`, which is a different door reaching the same state.
//
// Two doors to one state is fine; two doors where only one is tested is how the untested one rots.
func TestClearingTheDraftRemovesIt(t *testing.T) {
	ts, srv := startServerWith(t)
	c, csrf := authedClient(t, ts)

	if code, _ := postForCode(t, c, csrf, ts.URL+"/api/ceremony/draft",
		draftRequest{Draft: `{"intent":"something"}`}); code != http.StatusOK {
		t.Fatal("setup: the save failed, so clearing it proves nothing")
	}
	srv.mu.Lock()
	v := srv.vault
	srv.mu.Unlock()
	if v == nil {
		t.Fatal("setup: the vault is not open")
	}
	if _, ok := v.CeremonyDraft(); !ok {
		t.Fatal("setup: no draft is stored, so the clear below cannot be observed")
	}

	if err := clearCeremonyDraft(v); err != nil {
		t.Fatalf("clearing the draft failed: %v", err)
	}
	if got, ok := v.CeremonyDraft(); ok {
		t.Errorf("the draft survives its own clear door (%q). P03.S03 consumes through this at "+
			"convene, so a clear that does not clear leaves the roster and the recital on disk "+
			"after the ceremony they described has been signed", got)
	}
	// Idempotent: the caller's question is "is it gone", not "did I remove it".
	if err := clearCeremonyDraft(v); err != nil {
		t.Errorf("clearing an already-clear draft returned %v, want nil", err)
	}
}
