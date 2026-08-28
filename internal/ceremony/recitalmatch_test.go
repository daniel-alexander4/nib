package ceremony

import (
	"errors"
	"strings"
	"testing"
	"time"

	"nib/internal/testpdf"
)

// P07.S07b: an invitation whose recital differs from the record's is refused BY NAME, before
// consent (C17's intent third).
//
// # Why this comparison is what makes the field safe rather than a hint
//
// `Invitation.Intent` is carried in an UNSIGNED invitation, and `RosterHash` is a digest of the
// record copied into it — so editing the intent alone leaves that digest matching. The
// completeness guard next door recorded exactly this objection as the reason not to carry the
// field at all, and it is right about the field ALONE. What makes it safe is this comparison: the
// invitation's copy is the record's copy, or the hop is refused before anybody consents to
// anything.
//
// That matters because `internal/p2p` signs the /Reason and reads its recital from the roster,
// which `l3Roster` builds from the invitation and never from the document (the S03 rule). So this
// comparison is the only thing standing between a tampered invitation and a signature that says
// something the record does not.

func TestAnInvitationWhoseRecitalDiffersIsRefusedByName(t *testing.T) {
	cert, key, cfp := identity(t, "Convener")
	_, _, afp := identity(t, "A")
	base, err := testpdf.Text("the lease")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)

	got, err := Convene(base, conveneReq(t, cfp, afp), cert, key, now)
	if err != nil {
		t.Fatal(err)
	}
	var inv Invitation
	for _, iv := range got.Invites {
		if strings.EqualFold(iv.Party.Fingerprint, afp) {
			inv = iv.Invitation
		}
	}
	if inv.ID == "" {
		t.Fatalf("setup: no invitation for party A")
	}
	// SETUP: the honest invitation matches, or the refusal below is a function that refuses
	// everything.
	if err := inv.MatchesRecord(got.Record); err != nil {
		t.Fatalf("setup: the invitation Convene produced does not match its own record: %v", err)
	}
	// SETUP: and it really carries the recital — a field left empty on both sides compares equal
	// and would make the mutation below the only thing under test.
	if inv.Intent == "" || inv.Intent != got.Record.Intent {
		t.Fatalf("setup: the invitation carries %q and the record %q; this test needs the field "+
			"populated on both sides", inv.Intent, got.Record.Intent)
	}

	// One word changed — the shape a tamperer uses, not a corruption.
	tampered := inv
	tampered.Intent = strings.Replace(inv.Intent, "co-sign", "witness", 1)
	if tampered.Intent == inv.Intent {
		t.Fatalf("setup: the mutation changed nothing (%q)", inv.Intent)
	}

	err = tampered.MatchesRecord(got.Record)
	if err == nil {
		t.Fatal("an invitation whose recital differs from the record's was accepted. Every " +
			"signature carries the recital verbatim and the signing path reads it from the " +
			"INVITATION, so this party would have signed a sentence the record does not contain " +
			"— and the roster commitment would still have matched, because the recital is not " +
			"what that digest covers on the invitation's side")
	}
	if !errors.Is(err, ErrRosterMismatch) {
		t.Errorf("the refusal is %v, not the mismatch error a caller can match on", err)
	}
	// By NAME: the sentence has to say which axis, because "these are different ceremonies" over
	// an identical roster sends a convener looking at the roster.
	msg := err.Error()
	if !strings.Contains(msg, "recital") {
		t.Errorf("the refusal does not name the axis that differs: %q", msg)
	}
	if !strings.Contains(msg, tampered.Intent) || !strings.Contains(msg, got.Record.Intent) {
		t.Errorf("the refusal does not quote both sentences, so a convener cannot see what "+
			"differs: %q", msg)
	}
}

// TestTheInvitationCarriesTheRecordsRecital is the writer half. The comparison above is
// satisfied perfectly by an invitation that carries "" and a record that carries "" — which is
// what `NewInvitations` produced before this slice, and what would make every assertion above
// pass while the signing path still had no recital to read.
func TestTheInvitationCarriesTheRecordsRecital(t *testing.T) {
	cert, key, cfp := identity(t, "Convener")
	_, _, afp := identity(t, "A")
	_, _, bfp := identity(t, "B")
	base, err := testpdf.Text("the lease")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)

	got, err := Convene(base, conveneReq(t, cfp, afp, bfp), cert, key, now)
	if err != nil {
		t.Fatal(err)
	}
	if got.Record.Intent == "" {
		t.Fatal("setup: the record carries no recital, so there is nothing for the invitation to carry")
	}
	if len(got.Invites) == 0 {
		t.Fatal("setup: no invitations were produced")
	}
	for _, iv := range got.Invites {
		if iv.Invitation.Intent != got.Record.Intent {
			t.Errorf("the invitation for %s carries recital %q and the record says %q — the "+
				"signing path reads the recital from the invitation, so a party holding this one "+
				"could not sign what the ceremony agreed", short(iv.Party.Fingerprint),
				iv.Invitation.Intent, got.Record.Intent)
		}
	}
}

// TestTheInvitationFormatVersionMovedWithTheField is D32's half. `Intent` is required, so a build
// that does not write it must not be able to hand this build an invitation that parses.
func TestTheInvitationFormatVersionMovedWithTheField(t *testing.T) {
	if InvitationVersion < 3 {
		t.Errorf("InvitationVersion is %d; adding a REQUIRED field without bumping it leaves an "+
			"older build's invitation parsing here with an empty recital, which is the state "+
			"`ErrNoCeremonyIntent` exists to refuse and a worse sentence than a version mismatch",
			InvitationVersion)
	}
	if !strings.Contains(invitationPrefix, "v3") {
		t.Errorf("the text prefix is %q and does not carry version 3 — `ParseInvitation` reads "+
			"the version out of the PREFIX to tell a newer format from an older one, so a prefix "+
			"left behind makes that answer wrong in one direction", invitationPrefix)
	}
}
