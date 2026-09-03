package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"encoding/hex"
	"nib/internal/ceremony"
	"nib/internal/p2p"
	"nib/internal/sign"
	"nib/internal/testpdf"
)

// P06.S03 — the panel's enabled action comes from the server's own L3 door.

// TestTheNextActionComesFromTheRecordAndNotTheRosterOrder is the criterion's own fixture.
//
// P06's replacement for the struck role-conflict criterion reads: *"the panel's enabled action is
// computed from the record by the same function the server's L3 check uses — driven by a fixture
// whose UI position and record position disagree, which must show the record's."*
//
// **The disagreement IS the assertion.** A roster where the two orders agree cannot tell a shared
// rule from two rules that happen to match — which is the entire failure mode ADR-009 exists for,
// and the reason a fixture that agrees is worth nothing here. So this one is built with a
// **non-signing convener at position 1**: the roster's first entry and the signing order's first
// entry are then different parties, and every naive answer ("the first party", "the first entry
// that has not signed") names the convener while L3 names the first SIGNING party.
func TestTheNextActionComesFromTheRecordAndNotTheRosterOrder(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	ts, srv := nextRouteFixture(t)
	_ = srv

	id := conveneNonSigningConvener(t, ts)
	got := askNext(t, ts, id)

	if got.State != "waiting" {
		t.Fatalf("state %q (%s), want waiting", got.State, got.Reason)
	}
	// SETUP, and it is the load-bearing half: the roster's first entry and the signing order's
	// first entry must actually differ, or this test is the agreeing fixture it exists not to be.
	rec, _, err := ceremony.ReadMirror(defaultOutputDir(), id, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.Roster) < 2 || rec.Roster[0].Signs {
		t.Fatalf("setup: this fixture's first roster entry signs (%+v), so the roster order and "+
			"the signing order agree and the disagreement this test is about does not exist",
			rec.Roster[0])
	}

	if strings.EqualFold(got.Label, rec.Roster[0].Label) {
		t.Errorf("the next action names %q, which is the roster's FIRST entry — a non-signing "+
			"convener who holds a position in the roster and none in the signing order (D22). "+
			"That is what every naive answer produces, and it is what a JS reimplementation over "+
			"the roster would produce", got.Label)
	}
	if got.Position != 1 || got.Of != 1 {
		t.Errorf("position %d of %d, want 1 of 1 — the position is counted over the SIGNING order, "+
			"and counting it over the roster tells a signer they are one place later than the "+
			"document will show", got.Position, got.Of)
	}
	if !got.IsMe || !got.MeKnown {
		t.Errorf("isMe=%v meKnown=%v — this machine IS the only signing party here", got.IsMe, got.MeKnown)
	}
}

// TestTheNextActionAgreesWithTheGateThatRefuses is ADR-009's routing assertion.
//
// **The guard asserts routing through the door, not the text each site prints.** Eight copies
// checked for agreement say nothing about a ninth site added without one — so this drives the
// question form and the refusing form over the SAME document and roster and requires them to name
// the same party. `AdmitContribution` is built on `NextContributor`, so they cannot disagree today;
// the assertion is what makes that a checked property rather than an implementation detail nobody
// is watching.
func TestTheNextActionAgreesWithTheGateThatRefuses(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	ts, _ := nextRouteFixture(t)
	id := conveneNonSigningConvener(t, ts)

	rec, pdf, err := ceremony.ReadMirror(defaultOutputDir(), id, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	rh, err := rec.RosterHash()
	if err != nil {
		t.Fatal(err)
	}
	roster := l3RosterFrom(rec.Roster, encodeHex(rh), rec.Intent)

	want, err := p2p.NextContributor(pdf, roster)
	if err != nil {
		t.Fatalf("setup: the shared door cannot answer for this fixture (%v), so the comparison "+
			"below has nothing to compare", err)
	}
	got := askNext(t, ts, id)
	if !strings.EqualFold(got.Label, want.Label) {
		t.Errorf("the route says %q and the shared door says %q — the route is not answering from "+
			"p2p.NextContributor, which is the one thing this slice is", got.Label, want.Label)
	}
	// The refusing form, over the same inputs: the party the route names is the one party
	// `AdmitContribution` would ADMIT, and every other roster member is refused.
	if aerr := p2p.AdmitContribution(pdf, roster, want.Fingerprint); aerr != nil {
		t.Errorf("the party the route names is refused by the gate that admits contributions: %v. "+
			"The question form and the refusing form have gone out of step, which is exactly what "+
			"building them separately would produce", aerr)
	}
	for _, p := range rec.Roster {
		if strings.EqualFold(p.Fingerprint, want.Fingerprint) {
			continue
		}
		if aerr := p2p.AdmitContribution(pdf, roster, p.Fingerprint); aerr == nil {
			t.Errorf("the gate also admits %q, so 'whose turn is it' has more than one answer and "+
				"the route's is not distinguishable from a guess", p.Label)
		}
	}
}

// TestAnUnreadableCeremonyReportsUnavailableAndNotAnOrder.
//
// **Three states and not two.** A route answering "waiting for X" or nothing would make "the
// ceremony is finished" and "Nib cannot tell" the same screen, and those want opposite actions.
// This drives the third: a record that does not load must not produce an order read out of a file
// this machine has refused to trust — the rule `Stored.Me` follows one field over.
func TestAnUnreadableCeremonyReportsUnavailableAndNotAnOrder(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	ts, _ := nextRouteFixture(t)
	id := conveneNonSigningConvener(t, ts)

	// SETUP: it answers "waiting" while healthy, or "unavailable" below is unavailable for the
	// wrong reason.
	if got := askNext(t, ts, id); got.State != "waiting" {
		t.Fatalf("setup: a healthy ceremony answers %q, want waiting", got.State)
	}
	dir, err := ceremony.MirrorDir(defaultOutputDir(), id)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir+"/record.json", []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := askNext(t, ts, id)
	if got.State != "unavailable" {
		t.Errorf("a damaged ceremony answers %q, want unavailable", got.State)
	}
	if got.Label != "" || got.Position != 0 {
		t.Errorf("it named a party (%q, position %d) from a record that does not load", got.Label, got.Position)
	}
	if got.Reason == "" {
		t.Error("no reason. The state is a word; the sentence is what tells the user whether this " +
			"is damage, a forgery, or a Nib that is out of date")
	}
	// **The sentence is `ReadStored`'s, not a raw decode error, and that is what the pre-check
	// buys.** `ReadMirror` fails on this record too, so removing the `ReadStored` branch still
	// produces "unavailable" — a mutation proved exactly that, coming back GREEN against an
	// earlier version of this test. What it does NOT produce is a sentence written for a person:
	// P08.S03 built four classes with four sentences precisely so a user is told whether their
	// ceremony is damaged, forged, or merely newer than their Nib, and `invalid character 'n'` is
	// none of those.
	want := ceremony.ReadStored(defaultOutputDir(), id, time.Now()).Reason
	if want == "" {
		t.Fatal("setup: ReadStored produced no sentence for this damaged record, so the " +
			"comparison below cannot distinguish it from a raw error")
	}
	if got.Reason != want {
		t.Errorf("the reason is %q; ReadStored's sentence for this class is %q. A raw decoder "+
			"error reaches the user instead of the sentence written for them", got.Reason, want)
	}
}

// TestTheNextRouteAnswersWithTheVaultLocked — the same footing as the listing (P06.S01).
//
// Nothing here needs a vault: the record and the document are ordinary files, and "is it my turn"
// is answerable because P06.S02 recorded which party this machine is.
func TestTheNextRouteAnswersWithTheVaultLocked(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	ts, _ := nextRouteFixture(t)
	id := conveneNonSigningConvener(t, ts)

	locked := New(os.DirFS("."), os.DirFS("."), t.TempDir(), "test")
	lts := httptest.NewServer(locked.Handler())
	t.Cleanup(lts.Close)
	// SETUP: it really is locked, or this asserts nothing.
	if locked.unlockedVault() != nil {
		t.Fatal("setup: this server holds an unlocked vault")
	}
	got := askNext(t, lts, id)
	if got.State != "waiting" {
		t.Fatalf("a locked read answers %q (%s), want waiting", got.State, got.Reason)
	}
	if !got.MeKnown {
		t.Error("a locked read does not know which party this machine is. That is what P06.S02's " +
			"marker exists for — without it this route would need identity(v) and could not be " +
			"lock-free at all")
	}
}

// nextRouteFixture starts an unlocked server with its own home.
func nextRouteFixture(t *testing.T) (*httptest.Server, *Server) {
	t.Helper()
	srv, _ := unlockedServer(t)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts, srv
}

// conveneNonSigningConvener convenes a ceremony whose FIRST roster entry does not sign.
//
// That shape is the whole point: the roster order and the signing order then name different
// parties, so an answer computed over the roster and an answer computed by L3 differ.
func conveneNonSigningConvener(t *testing.T, _ *httptest.Server) string {
	t.Helper()
	ts, pdfPath := startServer(t)
	c, csrf := authedClient(t, ts)
	if code, body := postForCode(t, c, csrf, ts.URL+"/api/open", openRequest{Path: pdfPath}); code != http.StatusOK {
		t.Fatalf("open: %d %s", code, body)
	}
	me := myFingerprint(t, c, ts.URL)
	req := conveneRequest{
		Roster: []convenePartyRequest{
			{Fingerprint: strings.Repeat("3d", 32), Label: "The registrar", Signs: false},
			{Fingerprint: me, Label: "Alice Tenant", Signs: true},
		},
		Intent:        "We agree to the terms",
		Expires:       time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339),
		ConvenerSigns: false,
	}
	code, body := postForCode(t, c, csrf, ts.URL+"/api/ceremony/convene", req)
	if code != http.StatusOK {
		t.Fatalf("convene: %d %s", code, body)
	}
	var out struct {
		Ceremony string `json:"ceremony"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("convene response is not JSON (%v): %s", err, body)
	}
	return out.Ceremony
}

// askNext calls the route and decodes it.
func askNext(t *testing.T, ts *httptest.Server, id string) ceremonyNextResponse {
	t.Helper()
	resp, err := http.Get(ts.URL + "/api/ceremony/next?ceremony=" + id)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/ceremony/next returned %d", resp.StatusCode)
	}
	var out ceremonyNextResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func encodeHex(b []byte) string {
	const hexdigits = "0123456789abcdef"
	out := make([]byte, 0, len(b)*2)
	for _, c := range b {
		out = append(out, hexdigits[c>>4], hexdigits[c&0x0f])
	}
	return string(out)
}

// TestTheNextActionMovesOnAfterAHopIsSigned.
//
// **Every other fixture in this file is at hop 0, and that is a coverage hole the review found.**
// A route that reported "the first signing party" unconditionally passes all of them: with nothing
// signed, the first signing party IS the answer. What none of them asks is whether the answer
// MOVES — which is the only thing a user of this panel actually cares about, since the question
// they have is "is it me yet".
//
// So this signs hop 1 for real, through `buildCoSigned` — the same door the initiating path uses —
// writes the result back to the mirror as a hop does, and asks again. The answer must move from
// party 1 to party 2, and `isMe` must flip with it.
func TestTheNextActionMovesOnAfterAHopIsSigned(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	ts, srv := nextRouteFixture(t)
	_ = srv

	id, meCert, meKey := conveneTwoSigners(t)
	root := defaultOutputDir()

	// Before: it is this machine's turn, and it is first in the signing order.
	before := askNext(t, ts, id)
	if before.State != "waiting" || !before.IsMe || before.Position != 1 {
		t.Fatalf("setup: before any signature the route says %+v — this test needs it to be this "+
			"machine's turn at position 1, or the move below has nowhere to move from", before)
	}

	rec, pdf, err := ceremony.ReadMirror(root, id, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	rh, err := rec.RosterHash()
	if err != nil {
		t.Fatal(err)
	}
	roster := l3RosterFrom(rec.Roster, encodeHex(rh), rec.Intent)

	// The real signing door, not a hand-built signature: `buildCoSigned` is what the initiating
	// path calls, and it applies the same L3 gate. A hand-rolled blob would prove the route reads
	// signatures and not that it reads the ones this product produces.
	other := ""
	for _, p := range rec.Roster {
		if !strings.EqualFold(p.Fingerprint, before.labelFingerprint(rec)) {
			other = p.Fingerprint
		}
	}
	att := p2p.Attestation{Signer: "Alice Tenant", AcceptedPeer: other, Intent: rec.Intent, When: time.Now()}
	w := httptest.NewRecorder()
	signed, ok := srv.buildCoSigned(w, pdf, meCert, meKey, att, nil, roster)
	if !ok {
		t.Fatalf("setup: the party whose turn it IS was refused by the signing door: %d %s",
			w.Code, w.Body.String())
	}
	if _, err := ceremony.WriteMirror(root, rec, signed); err != nil {
		t.Fatal(err)
	}

	after := askNext(t, ts, id)
	if after.State != "waiting" {
		t.Fatalf("after one signature the route says %q (%s), want waiting — there is still one "+
			"signing party to go", after.State, after.Reason)
	}
	if after.Position != 2 || after.Of != 2 {
		t.Errorf("after one signature the route says position %d of %d, want 2 of 2. The answer "+
			"did not move, which is the one thing a user of this panel is asking about",
			after.Position, after.Of)
	}
	if after.IsMe {
		t.Errorf("the route still says it is this machine's turn after this machine signed — a " +
			"panel showing that invites a party to sign twice, which L3 exists to refuse")
	}
	if !after.MeKnown {
		t.Error("the position marker was lost across the hop's write-back")
	}
}

// labelFingerprint resolves the roster entry the response named back to its fingerprint.
//
// The response deliberately carries no fingerprint — P06's criterion is that no hex appears on the
// primary flow — so a test that needs one resolves it through the roster, which is exactly what a
// surface would do.
func (c ceremonyNextResponse) labelFingerprint(rec ceremony.Record) string {
	for _, p := range rec.Roster {
		if p.Label == c.Label {
			return p.Fingerprint
		}
	}
	return ""
}

// conveneTwoSigners builds a ceremony this machine signs FIRST of two, and returns its identity.
//
// Built through `ceremony.Convene` and written to the mirror directly, the way `ceremonyOnDisk`
// does, because this fixture needs the signing KEY and no route hands one back. The `me` marker is
// written through the same door the accept and convene paths use, so what the route reads here is
// what it reads in production.
func conveneTwoSigners(t *testing.T) (id string, cert, key []byte) {
	t.Helper()
	cert, key, err := sign.GenerateIdentity("Alice Tenant")
	if err != nil {
		t.Fatal(err)
	}
	fpb, err := sign.Fingerprint(cert)
	if err != nil {
		t.Fatal(err)
	}
	me := hex.EncodeToString(fpb)
	base, err := testpdf.Text("the lease")
	if err != nil {
		t.Fatal(err)
	}
	out, err := ceremony.Convene(base, ceremony.ConveneRequest{
		Roster: []ceremony.Party{
			{Fingerprint: me, Label: "Alice Tenant", Signs: true},
			{Fingerprint: strings.Repeat("5e", 32), Label: "Bob Landlord", Signs: true},
		},
		Intent:         "We agree to the terms",
		Expires:        time.Now().Add(48 * time.Hour),
		HopBudget:      ceremonyHopBudget(),
		DeliveryBudget: ceremonyDeliveryLegBudget(),
		ConvenerSigns:  true,
	}, cert, key, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	root := defaultOutputDir()
	if _, err := ceremony.WriteMirror(root, out.Record, out.Document); err != nil {
		t.Fatal(err)
	}
	if err := ceremony.WriteMe(root, out.Record.ID, me); err != nil {
		t.Fatal(err)
	}
	return out.Record.ID, cert, key
}
