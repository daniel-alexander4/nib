package server

import (
	"testing"

	"nib/internal/ceremony"
	"nib/internal/p2p"
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
