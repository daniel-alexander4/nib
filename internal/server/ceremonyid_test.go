package server

import (
	"encoding/hex"
	"errors"
	"strconv"
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
func threeParty(t *testing.T) (invs map[string]ceremony.Invitation, certs [3][]byte, fps [3]string) {
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
	// One invitation PER PARTY since P05.S04, each with its own secret. This fixture returns
	// the map so a test can ask what one party can and cannot derive about another.
	all, err := ceremony.NewInvitations(rec)
	if err != nil {
		t.Fatal(err)
	}
	return all, certs, fps
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
	invs, certs, fps := threeParty(t)
	// The convener holds every party's invitation; a party holds only its own. For a hop
	// between the convener and party k, BOTH ends use party k's invitation — that is what
	// makes it a shared secret for exactly the two ends of that hop.
	textFor := func(party int) string { return mustEncode(t, invs[fps[party]]) }

	// D22 is a convener HUB: the convener dials each party in roster order, and every hop is
	// a two-party session with the convener at one end. So convener↔Alice is hop 0 and
	// convener↔Bob is hop 1 — Alice and Bob never connect. Both ends of a hop must get the
	// same number, which is what makes it agreed rather than negotiated.
	for _, tc := range []struct {
		who, peer int
		want      int
	}{
		{0, 1, 0}, {1, 0, 0},
		{0, 2, 1}, {2, 0, 1},
	} {
		peerFP, err := hex.DecodeString(fps[tc.peer])
		if err != nil {
			t.Fatal(err)
		}
		// The invitation used is the one belonging to the hop's non-convener end.
		holder := tc.who
		if holder == 0 {
			holder = tc.peer
		}
		cer, err := ceremonyFor(textFor(holder), certs[tc.who], nil, peerFP)
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

	if _, err := ceremonyFor(textFor(1), certs[0], nil, strangerFP); err == nil {
		t.Error("a peer outside the roster was given a hop")
	}
	if _, err := ceremonyFor(textFor(1), stranger, nil, convFP); err == nil {
		t.Error("a party outside the roster was armed into this ceremony")
	}
	// Two counterparties share no hop — D22's hub, and criterion 19 refusing at the door
	// rather than being remembered later.
	aliceFP, _ := hex.DecodeString(fps[1])
	if _, err := ceremonyFor(textFor(2), certs[2], nil, aliceFP); err == nil {
		t.Error("Bob was armed for a hop with Alice; under a convener hub they never " +
			"connect, so this is a session that does not exist")
	}

	// No invitation is the ORDINARY case, not an error — D9 demotes the manual path rather
	// than deleting it, and every existing arm has none.
	if _, err := ceremonyFor("", certs[0], nil, bobFP); !errors.Is(err, errNoCeremony) {
		t.Errorf("an arm with no invitation reported %v; it must be the ordinary case", err)
	}
	// And a mangled one is refused rather than ignored — silently dropping it would arm a
	// session the user believes is part of a ceremony and which is not.
	//
	// **The prefix is DERIVED (2026-08-24, P07.S02).** It was the literal `nib-invite-v1:`,
	// and the moment `InvitationVersion` moved to 2 this fixture stopped being a *mangled*
	// invitation and became an *older-format* one. The assertion still passed — both are
	// refusals — so the test went on reporting that a corrupt invitation is refused while
	// exercising the version path instead. A check that passes for a different reason than it
	// names is the shape this slice exists to remove.
	mangled := "nib-invite-v" + strconv.Itoa(ceremony.InvitationVersion) + ":not-real.deadbeef"
	if _, err := ceremonyFor(mangled, certs[0], nil, bobFP); err == nil ||
		errors.Is(err, errNoCeremony) {
		t.Errorf("a corrupt invitation reported %v; it must be refused, not treated as absent", err)
	}
}
