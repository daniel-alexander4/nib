package server

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
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

// TestAConvenedCeremonyConsumesItsDraft — P03.S03, and the pair of clauses is the test.
//
// The draft exists so a convener's setup survives closing Nib. Once the ceremony is convened, the
// setup describes a proceeding that already exists: keeping it means the next sheet opens
// pre-filled with a ceremony the user has already started, and it means the roster and recital of a
// signed proceeding sit on disk after it. A REFUSED convene is the opposite case — the setup is
// exactly what the user still needs, and losing it there is the defect P03.S02 was built against.
//
// **Both clauses in one test, deliberately.** They are one rule with two sides, and split across
// two tests either half can pass while the other rots — which is how a consume that fires on every
// attempt, or on none, would still look green from one side.
func TestAConvenedCeremonyConsumesItsDraft(t *testing.T) {
	ts, pdfPath := startServer(t)
	c, csrf := authedClient(t, ts)
	if code, body := postForCode(t, c, csrf, ts.URL+"/api/open", openRequest{Path: pdfPath}); code != http.StatusOK {
		t.Fatalf("setup: open: %d %s", code, body)
	}
	me := myFingerprint(t, c, ts.URL)

	// Read through the ROUTE rather than a vault handle: it is what the client can observe, and
	// it is the same door the restore uses.
	stored := func(t *testing.T) string {
		t.Helper()
		res, err := c.Get(ts.URL + "/api/ceremony/draft")
		if err != nil {
			t.Fatalf("read draft: %v", err)
		}
		defer res.Body.Close()
		var out draftResponse
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatalf("decode draft: %v", err)
		}
		return out.Draft
	}

	saveDraft := func(what string) {
		t.Helper()
		if code, _ := postForCode(t, c, csrf, ts.URL+"/api/ceremony/draft",
			draftRequest{Draft: what}); code != http.StatusOK {
			t.Fatalf("setup: saving the draft %q failed", what)
		}
		if stored(t) != what {
			t.Fatal("setup: nothing is stored, so the assertions below observe nothing")
		}
	}

	_ = me

	// ── A REFUSED convene leaves it intact ─────────────────────────────────────
	//
	// Asserted FIRST, and that ordering is not incidental: a consume wired before the last failure
	// path would pass the success clause and fail only here, and running this second on a shared
	// server would let the success case's clear hide it.
	saveDraft(`{"intent":"the refused one"}`)
	bad := conveneRequest{
		Roster:        []convenePartyRequest{{Fingerprint: me, Label: "Convener", Signs: true}},
		Intent:        "", // no intent: the route refuses
		Expires:       time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339),
		ConvenerSigns: true,
	}
	code, _ := postForCode(t, c, csrf, ts.URL+"/api/ceremony/convene", bad)
	if code == http.StatusOK {
		t.Fatal("setup: the deliberately-bad convene SUCCEEDED, so the assertion below is about a " +
			"refusal that did not happen")
	}
	if got := stored(t); got != `{"intent":"the refused one"}` {
		t.Errorf("a refused convene left the draft as %q, want it byte-identical. The setup is "+
			"exactly what the user still needs after a refusal — losing it there is the defect "+
			"P03.S02 was built against", got)
	}

	// ── A SUCCESSFUL convene consumes it ───────────────────────────────────────
	saveDraft(`{"intent":"the consumed one"}`)
	good := conveneRequest{
		Roster: []convenePartyRequest{
			{Fingerprint: me, Label: "Convener", Signs: true},
			{Fingerprint: strings.Repeat("2b", 32), Label: "The other party", Signs: true},
		},
		Intent:        "We agree to co-sign the lease",
		Expires:       time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339),
		ConvenerSigns: true,
	}
	if code, body := postForCode(t, c, csrf, ts.URL+"/api/ceremony/convene", good); code != http.StatusOK {
		t.Fatalf("the convene failed (%d %s), so the consume below is not about a success", code, body)
	}
	if got := stored(t); got != "" {
		t.Errorf("a convened ceremony left its draft on disk (%q). The next setup sheet opens "+
			"pre-filled with a proceeding the user already started, and the roster and recital of "+
			"a signed proceeding stay on disk after it", got)
	}
}

// TestTheDraftIsConsumedAfterTheLastRefusal — P03.S03, and it exists because a probe showed the
// behavioural test could not see this.
//
// "A refused convene leaves the draft intact" is a claim about ORDERING: the consume must sit after
// every path that can still refuse. The behavioural test above drives a refusal the route rejects
// EARLY — an empty intent — so moving the consume to the middle of the handler left it green.
// A late refusal is not reachable from a test: `pinCeremonyRoster` fails only on a vault error.
//
// So the ordering is asserted over the source: within the handler, the consume comes after the last
// `httpError` in it. That is exactly the property, and it is what the behavioural test cannot hold.
func TestTheDraftIsConsumedAfterTheLastRefusal(t *testing.T) {
	b, err := os.ReadFile("convene.go")
	if err != nil {
		t.Fatal(err)
	}
	code := string(b)
	const fn = "func (s *Server) handleCeremonyConvene("
	i := strings.Index(code, fn)
	if i < 0 {
		t.Fatalf("cannot find %s — this guard is pinned to a name that has moved, so its clean "+
			"result says nothing", fn)
	}
	body := funcBodyFrom(code, i)
	if body == "" {
		t.Fatal("the brace matcher read an empty body")
	}

	consume := strings.Index(body, "clearCeremonyDraft(")
	lastRefusal := strings.LastIndex(body, "httpError(")

	// STIMULUS, both halves: a handler with no consume, or none that can refuse, would satisfy the
	// comparison below by having nothing to compare.
	if consume < 0 {
		t.Fatal("handleCeremonyConvene never consumes the draft, so a convened ceremony leaves its " +
			"setup on disk (P03.S03)")
	}
	if lastRefusal < 0 {
		t.Fatal("handleCeremonyConvene contains no httpError at all — the scan is not reading the " +
			"handler it thinks it is, and the ordering below is vacuous")
	}

	if consume < lastRefusal {
		t.Errorf("the draft is consumed at offset %d, BEFORE the handler's last refusal at %d. A "+
			"convene that refuses after that point would take the user's setup with it — and the "+
			"setup is exactly what they still need after a refusal, which is the defect P03.S02 "+
			"was built against.", consume, lastRefusal)
	}
}
