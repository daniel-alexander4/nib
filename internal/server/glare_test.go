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

// TestGlareDecideTable — P05.S09 T01/T03. The join must (1) take the designated survivor the
// instant it forms, (2) fall back to the other connection once the settle window expires or both
// attempts finish, and (3) both ends must reach the SAME physical connection. Because the survivor
// is the lower-fingerprint party's dial, the low side's glareDial and the high side's glareAccept
// name that one connection.
func TestGlareDecideTable(t *testing.T) {
	type row struct {
		name                           string
		keepDial, haveDial, haveAccept bool
		settleExpired, bothDone        bool
		want                           glareChoice
	}
	rows := []row{
		// Survivor in hand → take it at once, without waiting for the loser.
		{"low: my dial is the survivor", true, true, false, false, false, glareDial},
		{"low: survivor here even with the loser also here", true, true, true, false, false, glareDial},
		{"high: my accept is the survivor", false, false, true, false, false, glareAccept},
		{"high: survivor here with loser also here", false, true, true, false, false, glareAccept},
		// Survivor not yet formed, still time → wait.
		{"low: only the accept so far, keep waiting for my dial", true, false, true, false, false, glareWait},
		{"high: only the dial so far, keep waiting for my accept", false, true, false, false, false, glareWait},
		{"nothing yet", true, false, false, false, false, glareWait},
		// Settle expired with the non-survivor present → fall back to it (both ends fall to the same conn).
		{"low: dial never came, fall back to accept", true, false, true, true, false, glareAccept},
		{"high: accept never came, fall back to dial", false, true, false, true, false, glareDial},
		// Both attempts done, survivor absent → fall back to whatever formed.
		{"low: both done, only accept formed", true, false, true, false, true, glareAccept},
		{"high: both done, only dial formed", false, true, false, false, true, glareDial},
		// Both done, nothing formed → fail.
		{"both done, nothing", true, false, false, false, true, glareFail},
		{"settle expired, nothing", false, false, false, true, false, glareFail},
	}
	for _, r := range rows {
		got := glareDecide(r.keepDial, r.haveDial, r.haveAccept, r.settleExpired, r.bothDone)
		if got != r.want {
			t.Errorf("%s: glareDecide(keepDial=%v,dial=%v,accept=%v,settle=%v,both=%v) = %d, want %d",
				r.name, r.keepDial, r.haveDial, r.haveAccept, r.settleExpired, r.bothDone, got, r.want)
		}
	}
}

// TestGlareBothEndsConvergeOnOneConnection — the property that matters: for every way the two
// connections can form, the low end's choice and the high end's choice name the SAME physical
// connection (the low party's dial == the high party's accept; the high party's dial == the low
// party's accept).
func TestGlareBothEndsConvergeOnOneConnection(t *testing.T) {
	// A connection is identified by who dialed it. lowDialed = the low party's dial (= high's accept).
	// After both settle, decode each end's choice into "which physical connection".
	whichConn := func(lowEnd bool, choice glareChoice) string {
		// lowEnd's glareDial is the low party's dial; its glareAccept is the high party's dial.
		switch choice {
		case glareDial:
			if lowEnd {
				return "low->high"
			}
			return "high->low"
		case glareAccept:
			if lowEnd {
				return "high->low"
			}
			return "low->high"
		}
		return "none"
	}
	// Enumerate which physical connections formed, from each end's local view.
	// lowHasDial/highHasAccept both reflect the low->high connection; lowHasAccept/highHasDial the high->low.
	for _, lh := range []bool{true, false} { // low->high formed?
		for _, hl := range []bool{true, false} { // high->low formed?
			// Low end sees: haveDial=lh (its own dial), haveAccept=hl (it accepted high's dial).
			low := glareDecide(true, lh, hl, true /*settled*/, true /*bothDone*/)
			// High end sees: haveDial=hl (its own dial), haveAccept=lh (it accepted low's dial).
			high := glareDecide(false, hl, lh, true, true)
			lc, hc := whichConn(true, low), whichConn(false, high)
			if lc != hc {
				t.Errorf("low->high=%v high->low=%v: ends diverged — low kept %q, high kept %q", lh, hl, lc, hc)
			}
			if lh && !hl && lc != "low->high" {
				t.Errorf("only low->high formed but ends kept %q", lc)
			}
		}
	}
}
