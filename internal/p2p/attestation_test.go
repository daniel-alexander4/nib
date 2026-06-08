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
