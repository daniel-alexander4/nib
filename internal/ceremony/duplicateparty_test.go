package ceremony

import (
	"errors"
	"testing"
	"time"
)

// TestARecordNamingOnePartyTwiceIsRefused — /pending 583.
//
// `Convene` refused a duplicate fingerprint and `Record.Verify` did not, so the rule held for the
// one record this machine builds and for none that arrive. The record here is one `Convene` would
// never produce: signed by its convener, canonical, in bounds — and naming party A twice.
func TestARecordNamingOnePartyTwiceIsRefused(t *testing.T) {
	cert, key, cfp := identity(t, "Convener")
	_, _, afp := identity(t, "A")
	_, _, bfp := identity(t, "B")

	// The CONTROL: the same shape with three different parties verifies, so the refusal below is
	// about the repeat and not about a three-party roster.
	ok := draft(t, cfp, afp, bfp)
	if err := ok.Sign(cert, key); err != nil {
		t.Fatal(err)
	}
	if err := ok.Verify(time.Now()); err != nil {
		t.Fatalf("setup: a record with three different parties does not verify: %v", err)
	}

	dup := draft(t, cfp, afp, afp)
	if err := dup.Sign(cert, key); err != nil {
		t.Fatal(err)
	}
	err := dup.Verify(time.Now())
	if !errors.Is(err, ErrDuplicateParty) {
		t.Fatalf("a convener-signed record naming one party twice verified as %v, want "+
			"ErrDuplicateParty — its two entries share one invitation and one hop, and the party "+
			"is owed two signatures it can give once", err)
	}
}

// TestAnInvitationNamingOnePartyTwiceIsRefused — the same rule at the other door a roster
// arrives through. An invitation is pasted text, and its roster is what the hop numbers are
// counted over before any record has been seen.
func TestAnInvitationNamingOnePartyTwiceIsRefused(t *testing.T) {
	cert, key, cfp := identity(t, "Convener")
	_, _, afp := identity(t, "A")
	_, _, bfp := identity(t, "B")
	r := draft(t, cfp, afp, bfp)
	if err := r.Sign(cert, key); err != nil {
		t.Fatal(err)
	}
	inv, err := oneInvitation(t, r)
	if err != nil {
		t.Fatal(err)
	}
	text, err := inv.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if _, perr := ParseInvitation(text); perr != nil {
		t.Fatalf("setup: the unaltered invitation does not parse: %v", perr)
	}

	inv.Roster = append([]Party(nil), inv.Roster...)
	inv.Roster[2] = inv.Roster[1]
	text, err = inv.Encode()
	if err != nil {
		t.Fatal(err)
	}
	_, perr := ParseInvitation(text)
	if !errors.Is(perr, ErrDuplicateParty) || !errors.Is(perr, ErrInvitationCorrupt) {
		t.Fatalf("an invitation naming one party twice parsed as %v, want a refusal that is both "+
			"ErrInvitationCorrupt and ErrDuplicateParty", perr)
	}
}
