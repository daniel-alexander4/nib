package p2p

import (
	"strings"
	"testing"
	"time"
)

func TestAttestationReasonRoundTrip(t *testing.T) {
	att := Attestation{
		Signer:            "Alice",
		AcceptedPeer:      "ffeeddccbbaa00998877665544332211ffeeddccbbaa00998877665544332211",
		AcceptedPeerLabel: "Bob",
		// Intent left empty to exercise the default.
		When: time.Date(2026, 6, 8, 14, 30, 0, 0, time.UTC),
	}
	r := att.reason()
	if !strings.Contains(r, "Accepts Bob") || !strings.Contains(r, defaultIntent) {
		t.Errorf("reason not human-readable: %q", r)
	}
	m := spkiToken.FindStringSubmatch(r)
	if m == nil || m[1] != att.AcceptedPeer {
		t.Errorf("reason does not carry parseable peer SPKI: %q", r)
	}
}

func TestAppearanceLines(t *testing.T) {
	att := Attestation{
		Signer:            "Alice",
		AcceptedPeer:      "a1b2c3d4e5f600112233445566778899aabbccddeeff00112233445566778899",
		AcceptedPeerLabel: "Bob",
		Intent:            "I agree to sign this document.",
		When:              time.Date(2026, 6, 8, 14, 30, 0, 0, time.UTC),
	}
	lines := att.AppearanceLines()
	if len(lines) != 5 {
		t.Fatalf("appearance lines = %d, want 5", len(lines))
	}
	joined := strings.Join(lines, "\n")
	for _, want := range []string{"Nib co-signing attestation", "Alice", "Bob", "a1b2 c3d4 e5f6 0011..."} {
		if !strings.Contains(joined, want) {
			t.Errorf("appearance missing %q in:\n%s", want, joined)
		}
	}
}

func TestShortFingerprint(t *testing.T) {
	if got := shortFingerprint("a1b2c3d4e5f60011deadbeef"); got != "a1b2 c3d4 e5f6 0011..." {
		t.Errorf("shortFingerprint = %q", got)
	}
	if got := shortFingerprint("short"); got != "short" {
		t.Errorf("short input should pass through, got %q", got)
	}
}

// A crafted label or intent must not be able to inject a second [SPKI:...] token
// that ReadAttestations would parse instead of the real accepted peer.
func TestReasonResistsTokenInjection(t *testing.T) {
	real := "ffeeddccbbaa00998877665544332211ffeeddccbbaa00998877665544332211"
	// 64 hex characters, and the count is the whole test.
	//
	// This was "aaaa" + 58 zeros = **62**, and spkiToken requires exactly 64 followed by
	// "]". So the injected [SPKI:<62hex>] could never match whether or not safeText
	// stripped the brackets: with safeText replaced by the identity function this test
	// still passed. It is the ONLY test of safeText, and safeText is the only defence
	// attestation.go names.
	spoof := "aaaa" + strings.Repeat("0", 60) // 4 + 60 = 64
	att := Attestation{
		Signer:            "Nib User",
		AcceptedPeer:      real,
		AcceptedPeerLabel: "Mallory [SPKI:" + spoof + "]",
		Intent:            "see [SPKI:" + spoof + "]",
	}
	r := att.reason()
	all := spkiToken.FindAllStringSubmatch(r, -1)
	if len(all) != 1 || all[0][1] != real {
		t.Errorf("reason has %d SPKI tokens (want 1 = the real peer %s); reason=%q", len(all), real, r)
	}
}

// crossBind must require BOTH signatures valid: a tampered (invalid) co-signature
// attests to nothing, so it can't produce a "matched" / mutually-co-signed verdict.
func TestCrossBindRequiresValidity(t *testing.T) {
	A := "aaaa000000000000000000000000000000000000000000000000000000000000"
	B := "bbbb000000000000000000000000000000000000000000000000000000000000"

	// Both valid, mutual → both matched.
	both := []SignerAttestation{
		{Fingerprint: A, AcceptedPeer: B, Valid: true},
		{Fingerprint: B, AcceptedPeer: A, Valid: true},
	}
	crossBind(both)
	if !both[0].Matched || !both[1].Matched {
		t.Errorf("two valid mutual sigs should both match: %+v", both)
	}

	// B's signature is invalid → A (valid) accepts an invalid peer (no match), and
	// B itself is invalid (no match). No false "mutually co-signed".
	bBad := []SignerAttestation{
		{Fingerprint: A, AcceptedPeer: B, Valid: true},
		{Fingerprint: B, AcceptedPeer: A, Valid: false},
	}
	crossBind(bBad)
	if bBad[0].Matched || bBad[1].Matched {
		t.Errorf("a tampered co-signature must not cross-bind: %+v", bBad)
	}
}

// TestTheRosterTokenIsWellFormedWhereItIsActuallyBuilt covers the ONLY producer of the
// [NibRoster:<hash>] token on the real path.
//
// It was previously tested through ceremony.Record.RosterToken — a second implementation of
// the same format string with no production caller. The dead one was the tested one, so
// this one could have changed shape and the ceremony test would still have passed. That
// duplicate is gone (see the note where it used to live in internal/ceremony/record.go);
// p2p cannot import ceremony, because ceremony's own tests import p2p.
func TestTheRosterTokenIsWellFormedWhereItIsActuallyBuilt(t *testing.T) {
	const h = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	a := Attestation{AcceptedPeer: strings.Repeat("a", 64), AcceptedPeerLabel: "Marta",
		RosterHash: h, RosterVersion: 4}
	got := a.reason()
	// The VERSION travels with the hash (P07.S04). Without it a reader cannot tell a different
	// ceremony from a different record format, and the client's honest reading of a mismatch is
	// an accusation about the parties.
	if !strings.Contains(got, "[NibRoster:4:"+h+"]") {
		t.Fatalf("reason() = %q\ndoes not carry the token in its documented shape", got)
	}

	// It must survive the parser that reads it back, which is the whole point of a token.
	m := rosterToken.FindStringSubmatch(got)
	if m == nil || m[1] != "4" || m[2] != h {
		t.Fatalf("the token this code produced does not match the regexp this code parses "+
			"it with: %q", got)
	}

	// The control: no hash, no token. An empty RosterHash is an ordinary two-party
	// co-signature and must not acquire a commitment it never made.
	plain := Attestation{AcceptedPeer: strings.Repeat("a", 64), AcceptedPeerLabel: "Marta"}
	if strings.Contains(plain.reason(), "NibRoster") {
		t.Fatalf("an attestation with no roster carries a roster token: %q", plain.reason())
	}

	// The forgery safeHex's doc describes: user-controlled text cannot place an earlier
	// token that wins FindStringSubmatch.
	// The crafted token is VERSIONED too (P07.S04) — an attacker writes whatever shape the
	// parser reads, so a forgery arm using the old unversioned spelling would be testing that
	// the regexp ignores a string it was never going to match.
	evil := Attestation{
		AcceptedPeer:      strings.Repeat("a", 64),
		AcceptedPeerLabel: "x] [NibRoster:4:" + strings.Repeat("b", 64) + "] y",
		RosterHash:        h,
		RosterVersion:     4,
	}
	if m := rosterToken.FindStringSubmatch(evil.reason()); m == nil || m[2] != h {
		t.Fatalf("a crafted label displaced the real roster token: %q", evil.reason())
	}
}

// TestARosterHashWithoutAVersionCarriesNoToken — both, or neither.
//
// A commitment with no format version is one nothing can interpret: `FormatVersion` is the first
// substantive axis of the roster preimage, so a bare hash leaves a reader unable to tell a
// different ceremony from a different record format — which is the ambiguity the version exists
// to remove. Emitting no token is the fail-CLOSED direction, because `markOneProceeding` treats a
// missing commitment as disqualifying: the signature reads as "not part of this proceeding"
// rather than as a commitment somebody might compare.
func TestARosterHashWithoutAVersionCarriesNoToken(t *testing.T) {
	const h = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	// Stimulus: WITH a version the same attestation does carry one, so the absence below is
	// about the version and not about the fixture.
	withV := Attestation{AcceptedPeer: strings.Repeat("a", 64), RosterHash: h, RosterVersion: 4}
	if !strings.Contains(withV.reason(), "NibRoster") {
		t.Fatal("setup: a versioned commitment produced no token at all")
	}
	bare := Attestation{AcceptedPeer: strings.Repeat("a", 64), RosterHash: h}
	if got := bare.reason(); strings.Contains(got, "NibRoster") {
		t.Errorf("a commitment with no format version was written into the signature: %q. "+
			"A reader handed a bare hash cannot tell a different ceremony from a different "+
			"record format, and the client renders the second as the first.", got)
	}
}
