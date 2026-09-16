package server

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nib/internal/ceremony"
	"nib/internal/p2p"
	"nib/internal/sign"
	"nib/internal/testpdf"
	"nib/internal/vault"
)

// /pending 497 — a completed ceremony offered Stop and no Deliver, and Stop told every party it
// ended early. These drive the server's half through its real doors: `mirrorHop` for the hop, the
// stop and deliver routes behind `requireUnlocked`, and `runDeliveryRound` for the round claim.

// finishedCeremony convenes a two-party ceremony under THIS vault's identity in which the convener
// is the only SIGNING party, and returns its record, the unsigned document and the document once
// the convener has signed. One signature is therefore the whole of "every signing party has
// signed", which is what lets these tests reach a complete document with no network hop.
func finishedCeremony(t *testing.T, v *vault.Vault) (ceremony.Record, []byte, []byte) {
	t.Helper()
	cert, key, err := identity(v)
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
			{Fingerprint: me, Label: "Alice Convener", Signs: true},
			{Fingerprint: strings.Repeat("5e", 32), Label: "Bob Witness", Signs: false},
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
	roster := l3RosterFrom(out.Record.Roster, rosterHashHex(out.Record), out.Record.Intent)
	place, err := p2p.PlacementFor(out.Document, roster, me)
	if err != nil {
		t.Fatal(err)
	}
	att := p2p.Attestation{Signer: "Alice Convener", Intent: out.Record.Intent, When: time.Now()}
	p2p.StampCommitment(&att, roster, me)
	signed, err := p2p.Contribute(out.Document, cert, key, att, nil, place)
	if err != nil {
		t.Fatal(err)
	}
	// STIMULUS: the two documents really are on either side of complete, by the same reading the
	// code under test uses. Without both, "refused because unfinished" and "attested because
	// finished" could each be satisfied by a fixture that is neither.
	if pr, perr := ceremonyProgress(out.Record, out.Document); perr != nil || pr.Complete {
		t.Fatalf("setup: the unsigned document reads complete=%v err=%v, want incomplete", pr.Complete, perr)
	}
	if pr, perr := ceremonyProgress(out.Record, signed); perr != nil || !pr.Complete {
		t.Fatalf("setup: the signed document reads complete=%v err=%v, want complete", pr.Complete, perr)
	}
	return out.Record, out.Document, signed
}

// ceremonyPost drives a POST ceremony route through `requireUnlocked`, so the handler reads the
// request-pinned vault exactly as production does.
func ceremonyPost(t *testing.T, s *Server, h http.HandlerFunc, path, id string) (int, string) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"ceremony": id})
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", s.csrf)
	rec := httptest.NewRecorder()
	s.requireUnlocked(h)(rec, req)
	return rec.Code, rec.Body.String()
}

func terminationState(t *testing.T, rec ceremony.Record) string {
	t.Helper()
	tm, err := ceremony.ReadTermination(defaultOutputDir(), rec)
	if errors.Is(err, ceremony.ErrNoTermination) {
		return ""
	}
	if err != nil {
		t.Fatalf("the stored end state does not read: %v", err)
	}
	return tm.State
}

// TestTheHopThatFinishesTheDocumentAttestsCompleted — nothing ever signed `completed`, so the card
// kept offering Stop on a finished proceeding and the convener's close-out had no way in.
func TestTheHopThatFinishesTheDocumentAttestsCompleted(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	s, v := unlockedServer(t)
	rec, unsigned, signed := finishedCeremony(t, v)

	// A hop that leaves a signature owing is not an end state.
	s.mirrorHop(unsigned)
	if got := terminationState(t, rec); got != "" {
		t.Fatalf("a hop that left the document UNFINISHED attested %q. `completed` is D28's "+
			"'every signing party has contributed', and a write-once attestation of it before the "+
			"last signature is a false statement no later hop can correct", got)
	}

	s.mirrorHop(signed)
	if got := terminationState(t, rec); got != ceremony.StateCompleted {
		t.Fatalf("the hop that FINISHED the document left end state %q, want %q. With nothing "+
			"recorded, the card reads `!c.ended` and offers Stop and Re-issue beside 'Everyone has "+
			"signed', and never offers Send everyone their copy (/pending 497)", got, ceremony.StateCompleted)
	}
	if st := ceremony.ReadStored(defaultOutputDir(), rec.ID, time.Now()); st.Ended != ceremony.StateCompleted {
		t.Errorf("the listing reads ended=%q after completion; the card gates on this field", st.Ended)
	}
	if n := s.sess.status().Notice; n != nil {
		t.Errorf("a successful completion left notice %+v", n)
	}

	// A machine that is NOT the convener mirrors the same finished document and attests nothing:
	// only the convener can sign an end state, and every reader refuses one from anybody else.
	t.Setenv("HOME", t.TempDir())
	party, _ := unlockedServer(t)
	party.mirrorHop(signed)
	if got := terminationState(t, rec); got != "" {
		t.Errorf("a party's machine attested %q on the convener's behalf", got)
	}
}

// TestStopRefusesAFinishedProceeding — the stop attested `stopped` over a complete document, and its
// round told every party the convener ended it before everyone signed.
func TestStopRefusesAFinishedProceeding(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	s, v := unlockedServer(t)
	rec, unsigned, signed := finishedCeremony(t, v)
	if _, err := ceremony.WriteMirror(defaultOutputDir(), rec, signed); err != nil {
		t.Fatal(err)
	}

	code, body := ceremonyPost(t, s, s.handleCeremonyStop, "/api/ceremony/stop", rec.ID)
	if code != http.StatusConflict || !strings.Contains(body, "already signed") {
		t.Errorf("stopping a proceeding every party has signed answered %d %s, want 409 naming "+
			"that everyone has signed. `stopped` says the convener ended it BEFORE every party "+
			"signed, it is sent to every party, and it is write-once", code, body)
	}
	if got := terminationState(t, rec); got != "" {
		t.Errorf("the refused stop still attested %q", got)
	}

	// CONTROL: the same route on an unfinished proceeding stops it. Without this the refusal above
	// is satisfied by a stop route that refuses everything.
	t.Setenv("HOME", t.TempDir())
	s2, v2 := unlockedServer(t)
	rec2, unsigned2, _ := finishedCeremony(t, v2)
	_ = unsigned
	if _, err := ceremony.WriteMirror(defaultOutputDir(), rec2, unsigned2); err != nil {
		t.Fatal(err)
	}
	code, body = ceremonyPost(t, s2, s2.handleCeremonyStop, "/api/ceremony/stop", rec2.ID)
	if code != http.StatusOK {
		t.Fatalf("control: stopping an UNFINISHED proceeding answered %d %s, want 200", code, body)
	}
	if got := terminationState(t, rec2); got != ceremony.StateStopped {
		t.Errorf("control: the stop attested %q, want %q", got, ceremony.StateStopped)
	}
}

// TestDeliverAttestsAFinishedProceedingAndRefusesAnUnfinishedOne — the lazy half of the mint, and
// the refusal a round with nothing to deliver never had.
func TestDeliverAttestsAFinishedProceedingAndRefusesAnUnfinishedOne(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	s, v := unlockedServer(t)
	rec, unsigned, _ := finishedCeremony(t, v)
	if _, err := ceremony.WriteMirror(defaultOutputDir(), rec, unsigned); err != nil {
		t.Fatal(err)
	}
	code, body := ceremonyPost(t, s, s.handleCeremonyDeliver, "/api/ceremony/deliver", rec.ID)
	if code != http.StatusConflict || !strings.Contains(body, "has not ended") {
		t.Errorf("delivering a proceeding that has NOT ended answered %d %s, want 409 saying so. "+
			"The round would ship the partial document as the finished one, and every recipient "+
			"refuses it at checkDelivered's completeness clause after a connect each", code, body)
	}
	if got := terminationState(t, rec); got != "" {
		t.Errorf("delivering an unfinished proceeding attested %q", got)
	}

	// A ceremony that FINISHED with no end state on record — an older build, or a failed last-hop
	// mint. The route writes the missing `completed` and runs.
	t.Setenv("HOME", t.TempDir())
	s2, v2 := unlockedServer(t)
	rec2, _, signed2 := finishedCeremony(t, v2)
	if _, err := ceremony.WriteMirror(defaultOutputDir(), rec2, signed2); err != nil {
		t.Fatal(err)
	}
	if got := terminationState(t, rec2); got != "" {
		t.Fatalf("setup: the finished ceremony already carries end state %q", got)
	}
	code, body = ceremonyPost(t, s2, s2.handleCeremonyDeliver, "/api/ceremony/deliver", rec2.ID)
	if code != http.StatusOK {
		t.Fatalf("delivering a FINISHED proceeding with no end state answered %d %s, want 200", code, body)
	}
	if got := terminationState(t, rec2); got != ceremony.StateCompleted {
		t.Errorf("delivering a finished proceeding left end state %q, want %q — the convener's "+
			"close-out and the card both read it", got, ceremony.StateCompleted)
	}
}

// TestOneDeliveryRoundPerCeremony — the round's `disabled` lived on a button a panel rebuild
// replaces, and the server had no guard.
func TestOneDeliveryRoundPerCeremony(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	s, v := unlockedServer(t)
	rec, _, signed := finishedCeremony(t, v)

	release, ok := s.holdRound(rec.ID)
	if !ok {
		t.Fatal("setup: the first claim on a ceremony's round was refused")
	}
	if _, second := s.holdRound(rec.ID); second {
		t.Error("a second round claim on the same ceremony was granted while the first is held")
	}
	if _, other := s.holdRound(strings.Repeat("0", 32)); !other {
		t.Error("a round on a DIFFERENT ceremony was refused; the claim is per ceremony")
	}
	if _, err := s.runDeliveryRound(context.Background(), v, rec, signed, nil); !errors.Is(err, errRoundInFlight) {
		t.Errorf("a round started while one is running on the same ceremony returned %v, want "+
			"errRoundInFlight — two rounds walk the same parties at once", err)
	}
	release()
	if _, err := s.runDeliveryRound(context.Background(), v, rec, signed, nil); err != nil {
		t.Errorf("after the first round released its claim, the next was refused: %v", err)
	}
}

// TestTheNextAnswerStillSaysComplete — the route now reads progress through the shared door, and
// the card's legacy delivery offer keys on this exact state.
func TestTheNextAnswerStillSaysComplete(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	s, v := unlockedServer(t)
	rec, _, signed := finishedCeremony(t, v)
	if _, err := ceremony.WriteMirror(defaultOutputDir(), rec, signed); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	s.handleCeremonyNext(w, httptest.NewRequest(http.MethodGet, "/api/ceremony/next?ceremony="+rec.ID, nil))
	var got ceremonyNextResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("%d %s: %v", w.Code, w.Body.String(), err)
	}
	if got.State != "complete" {
		t.Errorf("the next answer for a finished document is %q (%s), want \"complete\"", got.State, got.Reason)
	}
}
