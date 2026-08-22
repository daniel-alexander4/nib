package server

import (
	"bytes"
	"testing"
)

// TestGlareConvergesOnOneConnection — P05.S09 T03. The two ends of a glare, computing from
// opposite viewpoints, must keep the SAME physical connection, or the document runs over two
// half-open channels. The lower-fingerprint party's DIAL is the survivor; the other party keeps
// what it ACCEPTED, which is that same connection.
func TestGlareConvergesOnOneConnection(t *testing.T) {
	lo := []byte{0x01, 0x02, 0x03}
	hi := []byte{0x01, 0x02, 0x04}

	// The low party keeps its own dial; the high party keeps the low party's dial (i.e. what it
	// accepted). Both therefore hold the connection the LOW party dialed — the same one.
	if !glareKeepsDial(lo, hi) {
		t.Error("the lower-fingerprint party must keep its own dialed connection")
	}
	if glareKeepsDial(hi, lo) {
		t.Error("the higher-fingerprint party must keep the connection it accepted, not its own dial")
	}
	// Stated as convergence: exactly one of the two ends keeps its dial, and it is the low one, so
	// both name the low party's dial as the survivor.
	lowKeepsOwnDial := glareKeepsDial(lo, hi)
	highKeepsOwnDial := glareKeepsDial(hi, lo)
	if lowKeepsOwnDial == highKeepsOwnDial {
		t.Fatalf("both ends made the same keep-own-dial choice (%v) — they cannot converge on one "+
			"connection", lowKeepsOwnDial)
	}
}

// TestGlareIsSymmetricInInputs — the decision depends only on the fingerprint order, so it is
// stable however the bytes are shaped, and equal inputs are deterministic (and cannot arise
// between two distinct identities).
func TestGlareIsSymmetricInInputs(t *testing.T) {
	a := []byte{0xaa, 0xbb}
	b := []byte{0xaa, 0xbc}
	if glareKeepsDial(a, b) == glareKeepsDial(b, a) {
		t.Error("glare must resolve oppositely for the two ends")
	}
	// Equal: deterministic false, no panic. A real ceremony never reaches this (distinct identities).
	if glareKeepsDial(a, a) {
		t.Error("equal fingerprints must resolve deterministically to keep-accept (false)")
	}
	// Longer/shorter prefixes compare lexicographically, as bytes.Compare documents.
	if !glareKeepsDial([]byte{0x01}, []byte{0x01, 0x00}) {
		t.Errorf("a proper prefix sorts below, want keep-dial; got %v", bytes.Compare([]byte{0x01}, []byte{0x01, 0x00}))
	}
}
