package server

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nib/internal/ceremony"
	"nib/internal/p2p"
	"nib/internal/sign"
)

// P04.S02 — the per-party states the rail's worklist renders, and the numbering hazard they exist
// to remove.
//
// # The defect this is built against
//
// `ceremonyNextResponse.Position` is 1-based within the SIGNING order; the panel numbers over the
// FULL roster. They diverge whenever a party does not sign — and `Convene` PREPENDS the convener at
// roster position 0, so unticking "I sign this too" produces that divergence in every ceremony the
// shipped convene form can make. The natural client join, `roster[position - 1]`, then lands on the
// convener: a party who never signs, marked as the one signing now.
//
// The fix is that the server sends each party's state so no join exists. These tests drive the
// roster the hazard needs, which existed at NO tier before this slice — `railscale.test.mjs` builds
// every entry `signs: true`.
func TestTheWorklistNamesTheSignerAndNotTheRosterIndex(t *testing.T) {
	// The exact divergent roster: a non-signing convener at index 0, two signers after.
	roster := []ceremony.Party{
		{Fingerprint: "aa", Label: "Convener", Signs: false},
		{Fingerprint: "bb", Label: "Bob", Signs: true},
		{Fingerprint: "cc", Label: "Carol", Signs: true},
	}
	// SETUP: the two orders really do disagree, or this test is about a roster where the naive
	// join happens to be right and it would pass against the defect.
	order := p2p.SigningOrder(p2p.Roster{Entries: []p2p.RosterEntry{
		{Fingerprint: "aa", Signs: false}, {Fingerprint: "bb", Signs: true}, {Fingerprint: "cc", Signs: true},
	}})
	if len(order) != 2 || order[0].Fingerprint != "bb" {
		t.Fatalf("setup: the signing order is %v — this fixture exists so that roster[0] and "+
			"signingOrder[0] are DIFFERENT parties, and here they are not", order)
	}

	got := partyStates(roster, p2p.Progress{Order: order, Done: 0}, "cc")
	if len(got) != 3 {
		t.Fatalf("three roster members produced %d rows — the worklist renders the roster, so a "+
			"row per SIGNING party would silently drop everyone who does not sign", len(got))
	}
	if got[0].State != "watching" {
		t.Errorf("the non-signing convener is %q, want \"watching\". At Done=0 the naive join "+
			"roster[position-1] lands on roster[0] and calls them the current signer — a party who "+
			"will never sign, in every ceremony the convene form can produce", got[0].State)
	}
	if got[1].State != "signing" {
		t.Errorf("Bob is %q, want \"signing\" — he is signingOrder[0] and nothing has been signed", got[1].State)
	}
	if got[2].State != "waiting" {
		t.Errorf("Carol is %q, want \"waiting\"", got[2].State)
	}
	if !got[2].IsMe || got[0].IsMe || got[1].IsMe {
		t.Errorf("the \"you\" marker landed on the wrong row: %v", []bool{got[0].IsMe, got[1].IsMe, got[2].IsMe})
	}
}

// Every state, driven separately — one helper answering one word satisfies the test above while
// getting the other three wrong.
func TestEveryPartyStateIsReachedSeparately(t *testing.T) {
	roster := []ceremony.Party{
		{Fingerprint: "aa", Label: "Convener", Signs: false},
		{Fingerprint: "bb", Label: "Bob", Signs: true},
		{Fingerprint: "cc", Label: "Carol", Signs: true},
	}
	order := []p2p.RosterEntry{{Fingerprint: "bb", Signs: true}, {Fingerprint: "cc", Signs: true}}

	for _, tc := range []struct {
		done int
		want []string
	}{
		{0, []string{"watching", "signing", "waiting"}},
		{1, []string{"watching", "signed", "signing"}},
		{2, []string{"watching", "signed", "signed"}},
	} {
		got := partyStates(roster, p2p.Progress{Order: order, Done: tc.done}, "")
		for i := range tc.want {
			if got[i].State != tc.want[i] {
				t.Errorf("done=%d: party %d is %q, want %q", tc.done, i, got[i].State, tc.want[i])
			}
		}
	}
	// The floor: the three cases above really are three different answers, so a helper that
	// returned one constant could not have passed them.
	a := partyStates(roster, p2p.Progress{Order: order, Done: 0}, "")
	b := partyStates(roster, p2p.Progress{Order: order, Done: 2}, "")
	if a[1].State == b[1].State {
		t.Fatal("setup: Bob reads the same at Done=0 and Done=2, so this table is not driving anything")
	}
}

// A party in the roster but absent from the signing order is "watching" — and a party in NEITHER
// cannot happen, so the map lookup's miss branch is the only place that decides it.
func TestAPartyWhoDoesNotSignIsNeverWaiting(t *testing.T) {
	roster := []ceremony.Party{{Fingerprint: "aa", Label: "Observer", Signs: false}}
	got := partyStates(roster, p2p.Progress{Order: []p2p.RosterEntry{{Fingerprint: "zz", Signs: true}}, Done: 0}, "")
	if got[0].State != "watching" {
		t.Errorf("a non-signing party reads %q. \"waiting\" would tell a coordinator that somebody "+
			"still owes a signature they will never give, which at a full roster is exactly the "+
			"misreading the worklist exists to prevent", got[0].State)
	}
}

// Case: fingerprints are compared case-insensitively everywhere else in this tree, and a roster
// read from disk is not guaranteed to match the order's casing.
func TestThePartyMatchIsCaseInsensitive(t *testing.T) {
	roster := []ceremony.Party{{Fingerprint: "AABB", Label: "Bob", Signs: true}}
	got := partyStates(roster, p2p.Progress{Order: []p2p.RosterEntry{{Fingerprint: "aabb", Signs: true}}, Done: 1}, "AaBb")
	if got[0].State != "signed" {
		t.Errorf("a roster fingerprint in a different case read %q, want \"signed\" — every other "+
			"comparison in this tree is EqualFold and a mismatch here silently marks a party who "+
			"has signed as one who has not", got[0].State)
	}
	if !got[0].IsMe {
		t.Error("the \"you\" marker missed a fingerprint in a different case")
	}
}

// ── P04.S03: who may re-issue, and for whom ──────────────────────────────────────────────────

// **The positive control comes FIRST, and it is not a formality.** The convener check shipped in
// this slice's first cut destructured `identity(v)` as `(cert, fingerprint, err)` when it is
// `(cert, KEY, err)`, so it compared a private key's bytes to a fingerprint and refused EVERY
// caller — the convener included. Every negative assertion below passes against that build. Only a
// test that drives the legitimate case can see it.
func TestTheConvenerCanStillReIssue(t *testing.T) {
	ts, pdfPath := startServer(t)
	c, csrf := authedClient(t, ts)
	if code, body := postForCode(t, c, csrf, ts.URL+"/api/open", openRequest{Path: pdfPath}); code != http.StatusOK {
		t.Fatalf("open: %d %s", code, body)
	}
	me := myFingerprint(t, c, ts.URL)
	code, body := postForCode(t, c, csrf, ts.URL+"/api/ceremony/convene", conveneRequest{
		Roster: []convenePartyRequest{
			{Fingerprint: me, Label: "Convener", Signs: true},
			{Fingerprint: strings.Repeat("2b", 32), Label: "B", Signs: true},
			{Fingerprint: strings.Repeat("3c", 32), Label: "C", Signs: true},
		},
		Intent: "We agree to co-sign the lease", ConvenerSigns: true,
		Expires: time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339),
	})
	if code != http.StatusOK {
		t.Fatalf("convene: %d %s", code, body)
	}
	var first conveneResponse
	if err := json.Unmarshal([]byte(body), &first); err != nil {
		t.Fatal(err)
	}
	if len(first.Invites) != 2 {
		t.Fatalf("setup: convene issued %d invitations, want 2", len(first.Invites))
	}

	// Every party, as before the per-party field existed.
	code, body = postForCode(t, c, csrf, ts.URL+"/api/ceremony/invites",
		ceremonyInvitesRequest{Ceremony: first.Ceremony})
	if code != http.StatusOK {
		t.Fatalf("the CONVENER was refused its own ceremony: %d %s — a check that refuses everyone "+
			"satisfies every negative test in this file", code, body)
	}
	var all conveneResponse
	if err := json.Unmarshal([]byte(body), &all); err != nil {
		t.Fatal(err)
	}
	if len(all.Invites) != 2 {
		t.Fatalf("an unfiltered re-issue returned %d invitations, want 2 — omitting the fingerprint "+
			"has to keep meaning every party, or both of this route's existing tests are about a "+
			"behaviour that no longer exists", len(all.Invites))
	}

	// ONE named party.
	want := first.Invites[0]
	code, body = postForCode(t, c, csrf, ts.URL+"/api/ceremony/invites",
		ceremonyInvitesRequest{Ceremony: first.Ceremony, Fingerprint: want.Fingerprint})
	if code != http.StatusOK {
		t.Fatalf("a per-party re-issue: %d %s", code, body)
	}
	var one conveneResponse
	if err := json.Unmarshal([]byte(body), &one); err != nil {
		t.Fatal(err)
	}
	if len(one.Invites) != 1 {
		t.Fatalf("a re-issue for one named party returned %d invitations. Rendering every party's "+
			"channel secret when one was asked for is the maximal-exposure shape D21's own pin "+
			"argues against", len(one.Invites))
	}
	if !strings.EqualFold(one.Invites[0].Fingerprint, want.Fingerprint) {
		t.Errorf("asked for %s and got %s", want.Fingerprint[:12], one.Invites[0].Fingerprint[:12])
	}
	// And it is the SAME invitation — a re-issue reproduces, it never rotates.
	if one.Invites[0].Invitation != want.Invitation {
		t.Error("the per-party re-issue handed back different bytes from the first issue. A re-issue " +
			"reads the stored secret back; if these differ, the party holding the first one is now " +
			"holding something that will not match")
	}

	// A party the roster does not name is a question about somebody else's ceremony.
	code, body = postForCode(t, c, csrf, ts.URL+"/api/ceremony/invites",
		ceremonyInvitesRequest{Ceremony: first.Ceremony, Fingerprint: strings.Repeat("9f", 32)})
	if code != http.StatusNotFound {
		t.Errorf("re-issuing for a party not in the roster answered %d %s, want 404. A 200 with an "+
			"empty list reads as 'there is nothing to re-issue'", code, body)
	}
}

// A machine that holds a mirror but did not convene is refused BEFORE anything is minted, and with
// its own code — not the 410 that also means "the secrets were pruned".
func TestOnlyTheConvenerMayReIssue(t *testing.T) {
	ts, pdfPath := startServer(t)
	c, csrf := authedClient(t, ts)
	if code, body := postForCode(t, c, csrf, ts.URL+"/api/open", openRequest{Path: pdfPath}); code != http.StatusOK {
		t.Fatalf("open: %d %s", code, body)
	}
	// A ceremony convened by SOMEBODY ELSE, mirrored here — which is the state every non-convener
	// party is in, because every party writes a mirror.
	root := defaultOutputDir()
	rec := foreignRecord(t)
	if _, err := ceremony.WriteMirror(root, rec, nil); err != nil {
		t.Fatal(err)
	}
	// SETUP: the mirror really is readable, or the 404 branch would answer first and this test
	// would be about a missing directory rather than about who may re-issue.
	if _, _, err := ceremony.ReadMirror(root, rec.ID, time.Now()); err != nil {
		t.Fatalf("setup: the mirror this test plants is not readable: %v", err)
	}

	code, body := postForCode(t, c, csrf, ts.URL+"/api/ceremony/invites",
		ceremonyInvitesRequest{Ceremony: rec.ID})
	if code != http.StatusForbidden {
		t.Fatalf("a non-convener re-issuing answered %d %s, want 403. Without the check it reaches "+
			"the mint and is refused 410 on the first missing secret — which also means 'this "+
			"ceremony ended and its secrets were pruned', so one code carries three different "+
			"facts and none of them is actionable", code, body)
	}
	if strings.Contains(body, "no longer holds the invitation secret") {
		t.Error("the refusal is the mint's sentence, so the check ran too late — the point of " +
			"refusing first is that the caller is told the true reason")
	}
}

// foreignRecord is a signed ceremony record convened by an identity this machine does not hold —
// the state every non-convener party's mirror is in, because every party writes one.
func foreignRecord(t *testing.T) ceremony.Record {
	t.Helper()
	cert, key, err := sign.GenerateIdentity("somebody else")
	if err != nil {
		t.Fatal(err)
	}
	fp, err := sign.Fingerprint(cert)
	if err != nil {
		t.Fatal(err)
	}
	other, _, err := sign.GenerateIdentity("a second party")
	if err != nil {
		t.Fatal(err)
	}
	ofp, err := sign.Fingerprint(other)
	if err != nil {
		t.Fatal(err)
	}
	id, err := ceremony.NewID()
	if err != nil {
		t.Fatal(err)
	}
	rec := ceremony.Record{
		ID: id, Intent: "a ceremony this machine did not convene",
		Expires: time.Now().Add(48 * time.Hour),
		Roster: []ceremony.Party{
			{Fingerprint: hex.EncodeToString(fp), Label: "Their convener", Signs: true},
			{Fingerprint: hex.EncodeToString(ofp), Label: "Someone", Signs: true},
		},
	}
	if err := rec.Sign(cert, key); err != nil {
		t.Fatal(err)
	}
	return rec
}

// **The THRESHOLD itself, driven either side — which nothing did until P04's phase close.**
//
// The rail's two renderings are driven at tier 2 with `worklist` stubbed true and false, so the
// panel is asserted on both sides of the boundary. What that cannot see is the boundary: the server
// decides it (`len(rec.Roster) > ceremony.SittingCeiling`), and a wrong comparison — `>=`, or a
// different constant — renders perfectly on both sides of whatever number it happens to use.
//
// P04's first exit criterion is "the worklist threshold is a stated number AND the rail is asserted
// at sizes either side of it". The number is stated; this is the half that makes it the number the
// code uses.
func TestTheWorklistThresholdIsSittingCeiling(t *testing.T) {
	ts, pdfPath := startServer(t)
	c, csrf := authedClient(t, ts)
	me := ""

	convene := func(parties int) string {
		t.Helper()
		if code, body := postForCode(t, c, csrf, ts.URL+"/api/open", openRequest{Path: pdfPath}); code != http.StatusOK {
			t.Fatalf("open: %d %s", code, body)
		}
		if me == "" {
			me = myFingerprint(t, c, ts.URL)
		}
		roster := []convenePartyRequest{{Fingerprint: me, Label: "Convener", Signs: true}}
		for i := 1; i < parties; i++ {
			roster = append(roster, convenePartyRequest{
				// Distinct, valid hex, one per party.
				Fingerprint: strings.Repeat(string("0123456789abcdef"[i%16]), 64),
				Label:       "Party " + string(rune('A'+i)), Signs: true,
			})
		}
		code, body := postForCode(t, c, csrf, ts.URL+"/api/ceremony/convene", conveneRequest{
			Roster: roster, Intent: "co-sign the lease", ConvenerSigns: true,
			Expires: time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339),
		})
		if code != http.StatusOK {
			t.Fatalf("convene %d parties: %d %s", parties, code, body)
		}
		var out conveneResponse
		if err := json.Unmarshal([]byte(body), &out); err != nil {
			t.Fatal(err)
		}
		return out.Ceremony
	}

	worklistFor := func(id string) bool {
		t.Helper()
		res, err := c.Get(ts.URL + "/api/ceremony/next?ceremony=" + id)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var out ceremonyNextResponse
		if derr := json.NewDecoder(res.Body).Decode(&out); derr != nil {
			t.Fatal(derr)
		}
		// SETUP: the route really answered about this ceremony and really got as far as walking it.
		if out.Ceremony != id {
			t.Fatalf("the route answered about %s, asked about %s", out.Ceremony, id)
		}
		if out.State != "waiting" {
			t.Fatalf("a freshly convened ceremony is %q (%s), not \"waiting\" — the worklist flag is "+
				"only set on the waiting branch, so any other state makes this test vacuous",
				out.State, out.Reason)
		}
		return out.Worklist
	}

	atCeiling := convene(ceremony.SittingCeiling)
	overCeiling := convene(ceremony.SittingCeiling + 1)

	if worklistFor(atCeiling) {
		t.Errorf("a roster of exactly %d asks for a worklist. The ceiling is the largest roster the "+
			"single action is right for — D22's own doc calls %d \"what the UI should be designed "+
			"and copy-written for\" — so the switch is at ceiling+1, not at the ceiling",
			ceremony.SittingCeiling, ceremony.SittingCeiling)
	}
	if !worklistFor(overCeiling) {
		t.Errorf("a roster of %d does not ask for a worklist, so the rail keeps offering one action "+
			"past the size D6 says it is wrong at — and the threshold is a number nothing reads",
			ceremony.SittingCeiling+1)
	}
}

// A ceremony past its deadline has nobody left to invite, and the client cannot know it.
//
// **`Stored.Ended` is the ATTESTED end state only.** Expiry is derived, never written there, so the
// rail's `!c.ended` gate — which is "not known to have ended", not "still running" — lets the
// control through. Measured at P04's phase close: the same card said "this ceremony's deadline has
// passed" on one line and offered a re-issue on the next, and the mint worked.
func TestAReIssueIsRefusedOnceTheDeadlineHasPassed(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	s, v := unlockedServer(t)
	cert, key, err := identity(v)
	if err != nil {
		t.Fatal(err)
	}
	myFP, err := sign.Fingerprint(cert)
	if err != nil {
		t.Fatal(err)
	}
	other, _, err := sign.GenerateIdentity("the other party")
	if err != nil {
		t.Fatal(err)
	}
	ofp, err := sign.Fingerprint(other)
	if err != nil {
		t.Fatal(err)
	}
	id, err := ceremony.NewID()
	if err != nil {
		t.Fatal(err)
	}
	// **Planted rather than convened, and that is forced.** `Convene` refuses a deadline that does
	// not leave time for every hop — measured: it wants ~49m for a single hop — so a ceremony that
	// has ALREADY expired cannot be created through the door. The record is therefore built and
	// signed with this machine's own identity, which is what makes the convener check pass and
	// leaves the deadline as the only thing left to refuse.
	rec := ceremony.Record{
		ID: id, Intent: "co-sign the lease",
		Expires: time.Now().Add(-24 * time.Hour),
		Roster: []ceremony.Party{
			{Fingerprint: hex.EncodeToString(myFP), Label: "Convener", Signs: true},
			{Fingerprint: hex.EncodeToString(ofp), Label: "B", Signs: true},
		},
	}
	if err := rec.Sign(cert, key); err != nil {
		t.Fatal(err)
	}
	root := defaultOutputDir()
	if _, err := ceremony.WriteMirror(root, rec, nil); err != nil {
		t.Fatal(err)
	}
	// SETUP: the mirror reads back and the record still VERIFIES with a past deadline — otherwise
	// the route answers 404 and this test is about an unreadable directory rather than an expiry.
	if _, _, rerr := ceremony.ReadMirror(root, rec.ID, time.Now()); rerr != nil {
		t.Fatalf("setup: the planted mirror does not read back: %v", rerr)
	}

	body, _ := json.Marshal(ceremonyInvitesRequest{Ceremony: rec.ID})
	req := httptest.NewRequest(http.MethodPost, "/api/ceremony/invites", bytes.NewReader(body))
	w := httptest.NewRecorder()
	// The vault is attached the way `requireUnlocked` attaches it, so the handler is reached in the
	// state the wrapper leaves it in rather than through a second door.
	s.handleCeremonyInvites(w, req.WithContext(context.WithValue(req.Context(), vaultCtxKey{}, v)))

	if w.Code != http.StatusConflict {
		t.Fatalf("re-issuing past the deadline answered %d %s, want 409. The client's gate is "+
			"`!c.ended`, and Stored.Ended is the ATTESTED end state — expiry is derived and never "+
			"written there — so the control is offered and this is the only thing that refuses it",
			w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "deadline has passed") {
		t.Errorf("the refusal reads %s — it has to say WHICH thing ended, because the convener's "+
			"next step (run it again as a new ceremony) follows from that and not from a generic "+
			"conflict", w.Body.String())
	}
}
