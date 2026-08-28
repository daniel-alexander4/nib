package server

import (
	"reflect"
	"strings"
	"testing"

	"nib/internal/ceremony"
	"nib/internal/p2p"
)

// P07.S07a: the fields `l3Roster` used to drop.
//
// # Why this test exists separately from the block tests
//
// `internal/p2p`'s block tests build a roster by hand, so they prove what a block says GIVEN a
// roster that carries labels and capacities. Every one of them stays green if `l3Roster` goes back
// to copying `Fingerprint` and `Signs` and dropping the rest — which is the original defect, and
// the reason `Party.Label` sat inside the signed commitment for three phases with no display
// reader anywhere.
//
// A test whose fixture supplies the very thing the production code fails to supply is the vacuous
// green this repo keeps finding. This is the arm that cannot be satisfied by a fixture.

// TestTheInvitationsLabelsAndCapacitiesReachTheSigningRoster drives the actual conversion.
func TestTheInvitationsLabelsAndCapacitiesReachTheSigningRoster(t *testing.T) {
	c := &ceremonyID{inv: ceremony.Invitation{
		RosterHash: strings.Repeat("ab", 32),
		Roster: []ceremony.Party{
			{Fingerprint: strings.Repeat("11", 32), Label: "Alice Tenant", Signs: true},
			{Fingerprint: strings.Repeat("22", 32), Label: "Bob Landlord", Signs: true,
				Capacity: "as Director of Acme Ltd"},
			{Fingerprint: strings.Repeat("33", 32), Label: "Carol Convener", Signs: false},
		},
	}}

	got := c.l3Roster()
	if len(got.Entries) != 3 {
		t.Fatalf("the roster carries %d entries, want 3", len(got.Entries))
	}
	for i, want := range c.inv.Roster {
		e := got.Entries[i]
		if e.Label != want.Label {
			t.Errorf("party %d reaches the signing roster labelled %q; the invitation calls them "+
				"%q — a label dropped here is a block that says \"Nib User\"",
				i, e.Label, want.Label)
		}
		if e.Capacity != want.Capacity {
			t.Errorf("party %d reaches the signing roster with capacity %q, want %q — capacity is "+
				"a claim about a party's AUTHORITY and it is dropped silently",
				i, e.Capacity, want.Capacity)
		}
	}
}

// TestEveryDisplayFieldOfAPartyReachesTheRoster is the structural half, and it is what makes this
// hold for a field nobody has added yet.
//
// **The per-field test above goes stale the moment `ceremony.Party` grows a field**, which is
// exactly how `Label` came to be dropped: `l3Roster` was written against the fields that existed,
// and the two structs then diverged with nothing watching. `MatchesRecord` has the same problem
// one door over and solved it the same way — `matchesRosterFields` compares the WHOLE struct with
// `!=` so "there is no list to keep in step".
//
// This cannot compare structs (the two types differ on purpose — `p2p` may not import
// `ceremony`), so it does the next thing: every field of `ceremony.Party` must be accounted for,
// either as a field `l3Roster` carries or as one named here as deliberately not carried. A new
// field fails this test on its own name until somebody decides which it is.
func TestEveryDisplayFieldOfAPartyReachesTheRoster(t *testing.T) {
	// Fields carried into p2p.RosterEntry, by their ceremony.Party name.
	carried := map[string]string{
		"Fingerprint": "Fingerprint",
		"Signs":       "Signs",
		"Label":       "Label",
		"Capacity":    "Capacity",
	}
	// Fields deliberately NOT carried, each with the reason. Empty today, and the map exists so
	// that adding a field has an answer other than editing the `carried` map without thinking.
	notCarried := map[string]string{}

	pt := reflect.TypeOf(ceremony.Party{})
	et := reflect.TypeOf(p2p.RosterEntry{})
	for i := 0; i < pt.NumField(); i++ {
		name := pt.Field(i).Name
		if reason, ok := notCarried[name]; ok {
			if reason == "" {
				t.Errorf("ceremony.Party.%s is listed as not carried with no reason given", name)
			}
			continue
		}
		target, ok := carried[name]
		if !ok {
			t.Errorf("ceremony.Party.%s reaches no field of p2p.RosterEntry and is not listed as "+
				"deliberately dropped. `l3Roster` is the only conversion between them, and a "+
				"field it silently drops is a fact the signing path cannot see — which is how "+
				"Label came to be inside the signed commitment with no display reader anywhere. "+
				"Carry it, or list it in notCarried with why.", name)
			continue
		}
		if _, ok := et.FieldByName(target); !ok {
			t.Errorf("ceremony.Party.%s is mapped to p2p.RosterEntry.%s, which does not exist",
				name, target)
		}
	}
}
