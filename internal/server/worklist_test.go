package server

import (
	"encoding/hex"
	"encoding/json"
	"net/http"
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
