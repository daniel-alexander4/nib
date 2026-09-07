package ceremony

import (
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"nib/internal/sign"
	"nib/internal/testpdf"
)

// P05.S02 — a termination is checkable by a party who holds no record (D16, /pending 378).
//
// # The defect these drive
//
// `Termination.Verify` took a `Record`. A party who has accepted and not yet signed holds none —
// `WriteMirror`'s only production callers on a non-convening machine are that party's own completed
// hop — so the parties who most need to be told a proceeding has ended were exactly the ones who
// could not be told. Everything the check actually used was two values, both of which an invitation
// carries directly.

// endedFixture convenes a real ceremony and mints a real termination for it.
//
// **A real `Convene` and a real `NewTermination`**, because every check under test is a signature
// or a commitment and a hand-built object exercises none of them.
func endedFixture(t *testing.T) (Invitation, Record, Termination) {
	t.Helper()
	cert, key, err := sign.GenerateIdentity("Convener")
	if err != nil {
		t.Fatal(err)
	}
	fpb, err := sign.Fingerprint(cert)
	if err != nil {
		t.Fatal(err)
	}
	other := strings.Repeat("ab", 32)
	base, err := testpdf.Text("the lease")
	if err != nil {
		t.Fatal(err)
	}
	out, err := Convene(base, ConveneRequest{
		Roster: []Party{
			{Fingerprint: hex.EncodeToString(fpb), Label: "Convener", Signs: true},
			{Fingerprint: other, Label: "The other party", Signs: true},
		},
		Intent:         "We agree",
		Expires:        time.Now().Add(24 * time.Hour),
		HopBudget:      5 * time.Minute,
		DeliveryBudget: 5 * time.Minute,
	}, cert, key, time.Now())
	// **`Fatal`, not `Skip`.** The first cut skipped on a convene error and every case in this file
	// went green without running — which is the failure mode this whole file is about one level
	// down: a check that cannot reach its subject is indistinguishable from one that passed.
	if err != nil {
		t.Fatalf("setup: convene failed, so nothing below ran: %v", err)
	}
	var text string
	for _, inv := range out.Invites {
		if strings.EqualFold(inv.Party.Fingerprint, other) {
			text = inv.Text
		}
	}
	if text == "" {
		t.Fatal("setup: convene issued no invitation for the counterparty")
	}
	inv, err := ParseInvitation(text)
	if err != nil {
		t.Fatal(err)
	}
	term, err := SignTermination(out.Record, StateDeclined, cert, key)
	if err != nil {
		t.Fatal(err)
	}
	return inv, out.Record, term
}

// TestATerminationVerifiesOnTheInvitationExactlyWhereItDoesOnTheRecord is the slice's first
// acceptance clause, and it is asserted as an EQUIVALENCE rather than as two separate passes.
//
// The point is not that both happen to accept this object — it is that the two anchors are the same
// two values, so a caller holding either gets the same answer. A pair of implementations that agree
// on one fixture is what ADR-009 refuses; this is one implementation reached from two sources.
func TestATerminationVerifiesOnTheInvitationExactlyWhereItDoesOnTheRecord(t *testing.T) {
	inv, rec, term := endedFixture(t)

	if err := term.Verify(rec); err != nil {
		t.Fatalf("setup: the termination does not verify against its own record: %v", err)
	}
	ia, err := inv.Anchor()
	if err != nil {
		t.Fatalf("an invitation for a live ceremony yields no anchor: %v", err)
	}
	if err := term.VerifyAgainst(ia); err != nil {
		t.Errorf("the termination verifies against the record and NOT against the invitation "+
			"(%v). A party who has accepted and not yet signed holds no record, so on this build "+
			"the parties who most need to know the proceeding ended are the ones who cannot be "+
			"told", err)
	}

	// And the anchors are the same two values, not merely two that agree here.
	ra, err := rec.Anchor()
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(ra.RosterHash) != hex.EncodeToString(ia.RosterHash) {
		t.Errorf("the record's commitment is %x and the invitation's is %x",
			ra.RosterHash, ia.RosterHash)
	}
	if !strings.EqualFold(ra.Convener, ia.Convener) {
		t.Errorf("the record names convener %q and the invitation %q", ra.Convener, ia.Convener)
	}
}

// TestTheInvitationAnchorRefusesWhatTheRecordAnchorWould — the second acceptance clause, both arms.
func TestTheInvitationAnchorRefusesWhatTheRecordAnchorWould(t *testing.T) {
	t.Run("a mismatched roster commitment", func(t *testing.T) {
		inv, _, term := endedFixture(t)
		// A commitment for some other proceeding. Same length and same shape, so what refuses it is
		// the comparison and not a parse.
		inv.RosterHash = strings.Repeat("cd", 32)
		a, err := inv.Anchor()
		if err != nil {
			t.Fatal(err)
		}
		if err := term.VerifyAgainst(a); err == nil {
			t.Error("a termination for a DIFFERENT proceeding verified against this invitation. " +
				"`RosterHash` is the whole binding — it commits to the id, the convener, the " +
				"deadline and every roster entry — so this is the one comparison that refuses a " +
				"cross-ceremony replay")
		}
	})

	t.Run("a signer who is not the convener", func(t *testing.T) {
		inv, rec, _ := endedFixture(t)
		// A valid signature by a real key that is not this ceremony's convener.
		cert, key, err := sign.GenerateIdentity("Somebody else")
		if err != nil {
			t.Fatal(err)
		}
		term, err := SignTermination(rec, StateDeclined, cert, key)
		if err != nil {
			t.Fatal(err)
		}
		a, err := inv.Anchor()
		if err != nil {
			t.Fatal(err)
		}
		if err := term.VerifyAgainst(a); err == nil {
			t.Error("a termination signed by somebody who is not the convener verified. Without " +
				"this, any roster member could end a proceeding they are only a party to")
		}
	})

	t.Run("an invitation whose convener is not on its roster", func(t *testing.T) {
		inv, _, _ := endedFixture(t)
		// The state `handleCeremonyAccept` refuses at its door, reached here because nothing
		// signs an invitation and this field is not checked by the parser.
		inv.ConvenerFingerprint = strings.Repeat("ef", 32)
		if _, err := inv.Anchor(); err == nil {
			t.Error("an invitation naming a convener who is not one of its parties produced an " +
				"anchor. `Record.Convener` resolves the signer against the ROSTER and fails when " +
				"they are not a member; an invitation anchor that skipped that would be the " +
				"weaker of the two, and would trust whoever the field happened to name")
		}
	})
}
