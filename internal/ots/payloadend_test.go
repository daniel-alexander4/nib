package ots

import (
	"crypto/sha256"
	"errors"
	"testing"
)

// TestAnAttestationPayloadEndsAtItsValue (/pending 837).
//
// A pending attestation's payload is varbytes(URI) and a Bitcoin one's is
// varuint(height); both readers took the value and never looked at what followed
// it inside the payload. Each case differs from an accepted sequence by one byte
// placed there, with the payload's own length prefix grown to cover it.
func TestAnAttestationPayloadEndsAtItsValue(t *testing.T) {
	digest := sha256.Sum256([]byte("doc"))
	const uri = "https://cal.example"

	// ops, then an attestation whose payload is value followed by extra.
	seq := func(magic, value, extra []byte) []byte {
		b := []byte{opAppend, 0x01, 0xaa, opSHA256, tagAttestation}
		b = append(b, magic...)
		return appendVarbytes(b, append(append([]byte{}, value...), extra...))
	}
	pendingValue := appendVarbytes(nil, []byte(uri))
	heightValue := putVaruint(800000)
	extra := []byte{0x00}

	// STIMULUS: without the extra byte each sequence is accepted, and reads back the
	// value it was built from — so the refusals below are about that byte alone.
	if err := validatePendingSequence(seq(pendingMagic, pendingValue, nil)); err != nil {
		t.Fatalf("setup: an exact pending payload was refused by the calendar check: %v", err)
	}
	p, err := parseProof(buildOTS(digest, [][]byte{seq(pendingMagic, pendingValue, nil)}))
	if err != nil || p.seqs[0].calURL != uri {
		t.Fatalf("setup: an exact pending payload parsed as %+v, %v", p, err)
	}
	p, err = parseProof(buildOTS(digest, [][]byte{seq(bitcoinMagic, heightValue, nil)}))
	if err != nil || p.seqs[0].height != 800000 {
		t.Fatalf("setup: an exact Bitcoin payload parsed as %+v, %v", p, err)
	}

	if err := validatePendingSequence(seq(pendingMagic, pendingValue, extra)); !errors.Is(err, errAttestationTrailing) {
		t.Errorf("a calendar answer with a byte after its URI: got %v, want the trailing-payload refusal", err)
	}
	if _, err := parseProof(buildOTS(digest, [][]byte{seq(pendingMagic, pendingValue, extra)})); !errors.Is(err, errAttestationTrailing) {
		t.Errorf("a proof's pending payload with a byte after its URI: got %v, want the trailing-payload refusal", err)
	}
	if _, err := parseProof(buildOTS(digest, [][]byte{seq(bitcoinMagic, heightValue, extra)})); !errors.Is(err, errAttestationTrailing) {
		t.Errorf("a proof's Bitcoin payload with a byte after its height: got %v, want the trailing-payload refusal", err)
	}
}
