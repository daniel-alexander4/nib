package p2p

import (
	"strings"
	"testing"
)

// TestAnEmptyFingerprintIsNoParty — /pending 636. "" is what nib reports for a signer it could not
// identify (ADR-051), and two absences must not agree.
func TestAnEmptyFingerprintIsNoParty(t *testing.T) {
	fp := strings.Repeat("ab", 32)
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"", "", false},
		{"", fp, false},
		{fp, "", false},
		{fp, fp, true},
		{fp, strings.ToUpper(fp), true}, // /pending 648: a roster arrives from JSON un-normalised
		{fp, strings.Repeat("cd", 32), false},
	} {
		if got := SameParty(c.a, c.b); got != c.want {
			t.Errorf("SameParty(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

// TestAnUnidentifiedSignerIsOnNoRosterLine — the fourth site, which was guarded by accident: the
// roster was a map and an empty fingerprint merely missed it. A roster line with no fingerprint
// made the miss a hit, and a signer nobody could name read as a party the record names.
func TestAnUnidentifiedSignerIsOnNoRosterLine(t *testing.T) {
	fp := strings.Repeat("ab", 32)
	claim := strings.Repeat("c0", 32)
	atts := []SignerAttestation{
		{Valid: true, RosterHash: claim, Fingerprint: ""},
		{Valid: true, RosterHash: claim, Fingerprint: strings.ToUpper(fp)},
	}
	markUnrostered(atts, []string{"", fp})
	if !atts[0].Unrostered {
		t.Error("a signer with no fingerprint was matched to a roster line with none: two absences agreed, " +
			"and a signature nib could not attribute was not flagged as outside the roster")
	}
	if atts[1].Unrostered {
		t.Error("a rostered signer was flagged as outside the roster — the comparison stopped folding case")
	}
	if signed, obliged := Completeness(atts[:1], Proceeding{Signing: []string{""}}); signed != 0 || obliged != 1 {
		t.Errorf("Completeness = %d of %d, want 0 of 1: an empty roster entry was discharged by an unidentified signer",
			signed, obliged)
	}
}
