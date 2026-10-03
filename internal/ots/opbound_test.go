package ots

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// growing returns a sequence that appends n bytes to the 32-byte digest in one-byte steps and
// then hashes — the shape whose result crosses maxOpResult when 32+n > 4096.
func growing(n int, height uint64) sequence {
	ops := []op{{opAppend, bytes.Repeat([]byte{7}, n%4000+1)}}
	left := n - len(ops[0].arg)
	for ; left > 0; left-- {
		ops = append(ops, op{opAppend, []byte{1}})
	}
	ops = append(ops, op{tag: opSHA256})
	return sequence{ops: ops, height: height}
}

// TestAnOpResultPastTheReferenceBoundIsRefused — /pending 781.
//
// The parser capped each argument and the op count but not the RESULT, and compute copies the
// running message on every append and prepend, so a 1 MB `.ots` cost seconds of CPU before any
// network. Bounded by result length, as python-opentimestamps bounds it (Op.MAX_RESULT_LENGTH,
// 4096): the boundary is pinned on both sides, by length, at parse AND in compute.
func TestAnOpResultPastTheReferenceBoundIsRefused(t *testing.T) {
	digest := sha256.Sum256([]byte("doc"))
	atBound := growing(maxOpResult-32, 800000)   // the last op yields exactly 4096 bytes
	overBound := growing(maxOpResult-31, 800000) // ... and 4097

	t.Run("compute", func(t *testing.T) {
		if _, err := atBound.compute(context.Background(), digest[:]); err != nil {
			t.Fatalf("a path whose longest result is exactly %d bytes was refused: %v", maxOpResult, err)
		}
		_, err := overBound.compute(context.Background(), digest[:])
		if !errors.Is(err, ErrOpResultTooLong) {
			t.Fatalf("a path producing a %d-byte result computed (err %v); it must be refused", maxOpResult+1, err)
		}
		// Prepend is the same door.
		pre := sequence{ops: []op{{opPrepend, bytes.Repeat([]byte{1}, maxOpResult-31)}}}
		if _, err := pre.compute(context.Background(), digest[:]); !errors.Is(err, ErrOpResultTooLong) {
			t.Errorf("a prepend producing %d bytes computed (err %v)", maxOpResult+1, err)
		}
	})

	t.Run("parse", func(t *testing.T) {
		if _, err := parseProof(serialize(digest[:], []sequence{atBound})); err != nil {
			t.Fatalf("a proof at the bound did not parse: %v", err)
		}
		_, err := parseProof(serialize(digest[:], []sequence{overBound}))
		if !errors.Is(err, ErrOpResultTooLong) || !errors.Is(err, ErrProofTooComplex) {
			t.Fatalf("a proof past the bound parsed (err %v); it must be refused as too complex", err)
		}
		// The parse refusal is what keeps VerifyProof from computing at all.
		if _, err := VerifyProof(context.Background(), nil, []BlockSource{failingSource{}}, 1,
			serialize(digest[:], []sequence{overBound}), digest); !errors.Is(err, ErrOpResultTooLong) {
			t.Errorf("VerifyProof on a proof past the bound returned %v", err)
		}
	})

	t.Run("argument", func(t *testing.T) {
		// The reference reads a binary argument as varbytes of 1..MAX_RESULT_LENGTH: empty is
		// refused by readOpArg, too long by the result bound it necessarily crosses.
		for _, n := range []int{0, maxOpResult + 1} {
			b := serialize(digest[:], []sequence{{ops: []op{{opAppend, make([]byte, n)}, {tag: opSHA256}}, height: 1}})
			if _, err := parseProof(b); err == nil {
				t.Errorf("an append argument of %d bytes parsed", n)
			}
		}
		b := serialize(digest[:], []sequence{{ops: []op{{opAppend, make([]byte, maxOpResult-32)}, {tag: opSHA256}}, height: 1}})
		if _, err := parseProof(b); err != nil {
			t.Errorf("an append argument of %d bytes was refused: %v", maxOpResult-32, err)
		}
	})
}

// TestComputeStopsWhenItsContextIsDone — /pending 781.
//
// compute consulted no context, so the verification deadline the handler sets bounded the network
// and not the work. Counted, not timed: a cancelled context must refuse before the first op.
func TestComputeStopsWhenItsContextIsDone(t *testing.T) {
	digest := sha256.Sum256([]byte("doc"))
	s := sequence{ops: make([]op, 4*computeCheckEvery)}
	for i := range s.ops {
		s.ops[i] = op{tag: opSHA256}
	}
	if _, err := s.compute(context.Background(), digest[:]); err != nil {
		t.Fatalf("setup: the sequence does not compute: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.compute(ctx, digest[:]); !errors.Is(err, context.Canceled) {
		t.Errorf("compute under a cancelled context returned %v; it must stop", err)
	}
}

// tailCalendar answers every upgrade with a Bitcoin-attested tail of n sha256 ops.
func tailCalendar(t *testing.T, n int) *httptest.Server {
	t.Helper()
	tail := bytes.Repeat([]byte{opSHA256}, n)
	tail = append(tail, tagAttestation)
	tail = append(tail, bitcoinMagic...)
	tail = appendVarbytes(tail, putVaruint(800000))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(tail)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestAnUpgradeIsChargedAgainstTheProofsInstructionBudget — /pending 781.
//
// upgrade spliced the calendar's ops onto the file's, and each tail was parsed under its own
// maxProofInstructions — so a file at the bound plus eight calendars carried nine times it. The
// splice is charged against what the file left. Counted by instructions on either side of the
// boundary, not timed.
func TestAnUpgradeIsChargedAgainstTheProofsInstructionBudget(t *testing.T) {
	digest := sha256.Sum256([]byte("doc"))
	const tailOps = 50
	srv := tailCalendar(t, tailOps)
	prefix := func(n int) sequence {
		s := sequence{ops: make([]op, n), calURL: srv.URL}
		for i := range s.ops {
			s.ops[i] = op{tag: opSHA256}
		}
		return s
	}
	// The file's own ops plus the tail land exactly on the bound: the upgrade is made, so the
	// verdict comes from the (failing) explorer, not from the budget.
	fits := prefix(maxProofInstructions - tailOps)
	if _, ok, err := upgrade(context.Background(), srv.Client(), fits, digest[:], maxProofInstructions-len(fits.ops)); err != nil || !ok {
		t.Fatalf("an upgrade that fits the budget exactly was not made: ok %v err %v", ok, err)
	}
	over := prefix(maxProofInstructions - tailOps + 1)
	_, ok, err := upgrade(context.Background(), srv.Client(), over, digest[:], maxProofInstructions-len(over.ops))
	if ok || !errors.Is(err, ErrProofTooComplex) {
		t.Fatalf("an upgrade one instruction past the budget: ok %v err %v; it must be refused", ok, err)
	}
	// And VerifyProof passes the right budget: the same proof, whole, is refused rather than
	// reported pending.
	if _, err := VerifyProof(context.Background(), srv.Client(), nil, 1, serialize(digest[:], []sequence{over}), digest); !errors.Is(err, ErrProofTooComplex) {
		t.Errorf("VerifyProof over a proof whose upgrade exceeds the budget returned %v", err)
	}
}

// TestAnUnreachableCalendarIsNotReportedAsPending — /pending 813.
//
// Any upgrade error that was not the caller's deadline — DNS, refused, TLS, the 20 s timeout —
// was folded into StatePending, which is a calendar's answer ("not yet") that no calendar gave.
func TestAnUnreachableCalendarIsNotReportedAsPending(t *testing.T) {
	var digest [32]byte
	digest[0] = 3
	down := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	downURL := down.URL
	down.Close() // connection refused from here on
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(broken.Close)
	notYet, _ := countingCalendar(t)

	for name, url := range map[string]string{"refused": downURL, "500": broken.URL} {
		res, err := VerifyProof(context.Background(), http.DefaultClient, nil, 1, pendingProof(digest, 1, url), digest)
		if err == nil {
			t.Errorf("%s: a calendar that never answered was reported %+v; it must be an error", name, res)
		} else if !strings.Contains(err.Error(), "calendar") {
			t.Errorf("%s: the error does not say what failed: %v", name, err)
		}
	}
	// A calendar that DID answer "not yet" still makes the proof pending, even beside one that failed.
	mixed := serialize(digest[:], []sequence{
		{ops: []op{{opAppend, []byte{1}}, {tag: opSHA256}}, calURL: downURL},
		{ops: []op{{opAppend, []byte{2}}, {tag: opSHA256}}, calURL: notYet.URL},
	})
	if res, err := VerifyProof(context.Background(), http.DefaultClient, nil, 1, mixed, digest); err != nil || res.State != StatePending {
		t.Errorf("one calendar answered not-yet: res %+v err %v; want pending", res, err)
	}
}

// TestStampAcceptsOnlyWhatComputeExecutes — /pending 807.
//
// validatePendingSequence admitted reverse (0xf2) and hexlify (0xf3), which compute refuses, so
// Stamp could write a `.ots` nib cannot verify. Every tag is checked both ways: a calendar
// response is accepted iff compute walks the op.
func TestStampAcceptsOnlyWhatComputeExecutes(t *testing.T) {
	digest := sha256.Sum256([]byte("doc"))
	for tag := 0; tag < 256; tag++ {
		if tag == tagAttestation || tag == tagCheckpoint {
			continue
		}
		o := op{tag: byte(tag)}
		resp := []byte{byte(tag)}
		if o.tag == opAppend || o.tag == opPrepend {
			o.arg = []byte{9}
			resp = appendVarbytes(resp, o.arg)
		}
		_, cerr := sequence{ops: []op{o}}.compute(context.Background(), digest[:])
		verr := validatePendingSequence(withAttestation(resp))
		if (cerr == nil) != (verr == nil) {
			t.Errorf("tag 0x%02x: compute err %v, stamp's check err %v — they must agree", tag, cerr, verr)
		}
	}
	// And the result bound reaches the stamp side too.
	big := withAttestation(appendVarbytes([]byte{opAppend}, make([]byte, maxOpResult-31)))
	if err := validatePendingSequence(big); !errors.Is(err, ErrOpResultTooLong) {
		t.Errorf("a calendar response producing a %d-byte result was accepted (err %v)", maxOpResult+1, err)
	}
}

// withAttestation terminates ops with a well-formed pending attestation.
func withAttestation(ops []byte) []byte {
	b := append([]byte{}, ops...)
	b = append(b, tagAttestation)
	b = append(b, pendingMagic...)
	return appendVarbytes(b, appendVarbytes(nil, []byte("https://cal.example")))
}
