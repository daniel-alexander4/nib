package server

import (
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"nib/internal/ceremony"
	"nib/internal/sign"
)

// threeParty builds a signed three-party record and its invitation, and returns each
// party's certificate alongside. Three because a two-party ceremony has exactly ONE hop and
// cannot distinguish a derived hop number from a constant — the plan says so of criterion
// 18 and it is just as true of the derivation itself.
func threeParty(t *testing.T) (inv ceremony.Invitation, certs [3][]byte, fps [3]string) {
	t.Helper()
	names := [3]string{"convener", "alice", "bob"}
	var keys [3][]byte
	for i, n := range names {
		c, k, err := sign.GenerateIdentity(n)
		if err != nil {
			t.Fatal(err)
		}
		fp, err := sign.Fingerprint(c)
		if err != nil {
			t.Fatal(err)
		}
		certs[i], keys[i], fps[i] = c, k, hex.EncodeToString(fp)
	}
	id, err := ceremony.NewID()
	if err != nil {
		t.Fatal(err)
	}
	rec := ceremony.Record{
		ID:      id,
		DocHash: strings.Repeat("ab", 32),
		Intent:  "We agree to co-sign the lease",
		Expires: time.Now().Add(48 * time.Hour),
		Roster: []ceremony.Party{
			{Fingerprint: fps[0], Label: "Convener", Signs: true},
			{Fingerprint: fps[1], Label: "Alice", Signs: true},
			{Fingerprint: fps[2], Label: "Bob", Signs: true},
		},
	}
	if err := rec.Sign(certs[0], keys[0]); err != nil {
		t.Fatal(err)
	}
	inv, err = ceremony.NewInvitation(rec)
	if err != nil {
		t.Fatal(err)
	}
	return inv, certs, fps
}

func mustEncode(t *testing.T, inv ceremony.Invitation) string {
	t.Helper()
	text, err := inv.Encode()
	if err != nil {
		t.Fatal(err)
	}
	return text
}

// TestAnArmedSessionDerivesItsHopFromTheRoster — P05.S04 T08, and the hop is the point.
//
// Before this, `internal/server` imported neither internal/ceremony nor internal/rendezvous,
// so an arm knew a fingerprint, a bind, a mode and a transport and nothing about the
// proceeding it was part of. The hop every rendezvous derivation needs was absent, and the
// only hop number anywhere in the tree was a literal 0 in a self-test.
//
// **The hop is DERIVED, never supplied.** A hop passed in the request would be a number the
// two sides must agree on with nothing to make them agree; read off a convener-signed roster,
// both ends compute it from the same artifact.
func TestAnArmedSessionDerivesItsHopFromTheRoster(t *testing.T) {
	inv, certs, fps := threeParty(t)
	text := mustEncode(t, inv)

	// Convener↔Alice is hop 0; Alice↔Bob is hop 1. Both ends of each hop must get the same
	// number, which is what makes it agreed rather than negotiated.
	for _, tc := range []struct {
		who, peer int
		want      int
	}{
		{0, 1, 0}, {1, 0, 0},
		{1, 2, 1}, {2, 1, 1},
	} {
		peerFP, err := hex.DecodeString(fps[tc.peer])
		if err != nil {
			t.Fatal(err)
		}
		cer, err := ceremonyFor(text, certs[tc.who], peerFP)
		if err != nil {
			t.Fatalf("party %d with peer %d: %v", tc.who, tc.peer, err)
		}
		if cer.hop != tc.want {
			t.Errorf("party %d with peer %d derived hop %d, want %d — the two ends of one "+
				"hop must agree without negotiating", tc.who, tc.peer, cer.hop, tc.want)
		}
		// And the gate's two salts differ, which is the property a one-party ceremony
		// cannot exhibit and the reason this fixture has three parties.
		if string(cer.gate.Salt()) == string(cer.gate.PublishSalt()) {
			t.Errorf("party %d's gate reads and writes at the same salt", tc.who)
		}
	}

	// **The three refusals, each with a different cause.** A user given one sentence for all
	// three cannot act on any of them.
	stranger, _, err := sign.GenerateIdentity("stranger")
	if err != nil {
		t.Fatal(err)
	}
	strangerFP, err := sign.Fingerprint(stranger)
	if err != nil {
		t.Fatal(err)
	}
	convFP, _ := hex.DecodeString(fps[0])
	bobFP, _ := hex.DecodeString(fps[2])

	if _, err := ceremonyFor(text, certs[0], strangerFP); err == nil {
		t.Error("a peer outside the roster was given a hop")
	}
	if _, err := ceremonyFor(text, stranger, convFP); err == nil {
		t.Error("a party outside the roster was armed into this ceremony")
	}
	// Convener and Bob are two apart. This is criterion 19 refusing at the door rather than
	// being remembered later: a convener holding a later party's candidates cannot even
	// derive that hop.
	if _, err := ceremonyFor(text, certs[0], bobFP); err == nil {
		t.Error("the convener and a party two positions away were given a hop; a hop joins " +
			"adjacent parties, and this is where a convener would start dialling somebody " +
			"three hops down")
	}

	// No invitation is the ORDINARY case, not an error — D9 demotes the manual path rather
	// than deleting it, and every existing arm has none.
	if _, err := ceremonyFor("", certs[0], bobFP); !errors.Is(err, errNoCeremony) {
		t.Errorf("an arm with no invitation reported %v; it must be the ordinary case", err)
	}
	// And a mangled one is refused rather than ignored — silently dropping it would arm a
	// session the user believes is part of a ceremony and which is not.
	if _, err := ceremonyFor("nib-invite-v1:not-real.deadbeef", certs[0], bobFP); err == nil ||
		errors.Is(err, errNoCeremony) {
		t.Errorf("a corrupt invitation reported %v; it must be refused, not treated as absent", err)
	}
}
