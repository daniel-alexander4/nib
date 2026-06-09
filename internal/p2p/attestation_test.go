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
	spoof := "aaaa" + "0000000000000000000000000000000000000000000000000000000000" // 64 hex
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
