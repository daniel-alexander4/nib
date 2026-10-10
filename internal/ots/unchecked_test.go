package ots

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"testing"
	"time"
)

// flakySource answers for the heights in roots except those in down, which fail as an
// explorer outage does.
type flakySource struct {
	roots map[uint64][]byte
	down  map[uint64]bool
}

func (f flakySource) BlockHeader(_ context.Context, h uint64) ([]byte, time.Time, error) {
	if root, ok := f.roots[h]; ok && !f.down[h] {
		return root, time.Unix(1_700_000_000, 0), nil
	}
	return nil, time.Time{}, errors.New("explorer unavailable")
}

// TestABranchLeftUncheckedIsNotANegativeVerdict (/pending 645).
//
// "Invalid" is a statement about every branch. With one branch that fetched and did not
// match, and another nib could not check — its height unanswered, or an operation compute
// declines — the verdict was StateInvalid, because the guard asked whether SOME branch had
// been checked. The decoy is appended by anyone: the file is unsigned.
func TestABranchLeftUncheckedIsNotANegativeVerdict(t *testing.T) {
	digest := sha256.Sum256([]byte("verify me"))
	const genuine, decoy = uint64(800000), uint64(700000)
	tail := []byte{0x01, 0x02, 0x03}
	root, _ := sequence{ops: []op{{opAppend, tail}, {opSHA256, nil}}}.compute(context.Background(), digest[:])
	roots := map[uint64][]byte{genuine: root, decoy: bytes.Repeat([]byte{0x5a}, 32)}

	unwalkable := append([]byte{0xf2}, bitcoinSeqBytes(tail, genuine)...) // reverse: parsed, never computed

	for name, c := range map[string]struct {
		seqs [][]byte
		down map[uint64]bool
	}{
		"the genuine height is unanswered, decoy last": {
			[][]byte{bitcoinSeqBytes(tail, genuine), bitcoinSeqBytes(tail, decoy)}, map[uint64]bool{genuine: true}},
		"the genuine height is unanswered, decoy first": {
			[][]byte{bitcoinSeqBytes(tail, decoy), bitcoinSeqBytes(tail, genuine)}, map[uint64]bool{genuine: true}},
		"a branch compute declines, beside a mismatch": {
			[][]byte{unwalkable, bitcoinSeqBytes(tail, decoy)}, nil},
	} {
		t.Run(name, func(t *testing.T) {
			src := flakySource{roots: roots, down: c.down}
			// STIMULUS: the decoy branch really is fetched and really mismatches — alone it
			// is an earned "invalid" — so the case below differs only by the unchecked branch.
			res, err := VerifyProof(context.Background(), nil, []BlockSource{src}, 1,
				buildOTS(digest, [][]byte{bitcoinSeqBytes(tail, decoy)}), digest)
			if err != nil || res.State != StateInvalid {
				t.Fatalf("setup: the decoy alone gave %+v, %v; want invalid", res, err)
			}

			res, err = VerifyProof(context.Background(), nil, []BlockSource{src}, 1, buildOTS(digest, c.seqs), digest)
			if err == nil {
				t.Fatalf("state = %q with a branch never checked; want an error and no verdict", res.State)
			}
		})
	}

	// With the outage over, the same two-branch proof confirms: the error above was about
	// the fetch, not the proof.
	res, err := VerifyProof(context.Background(), nil, []BlockSource{flakySource{roots: roots}}, 1,
		buildOTS(digest, [][]byte{bitcoinSeqBytes(tail, decoy), bitcoinSeqBytes(tail, genuine)}), digest)
	if err != nil || res.State != StateConfirmed || res.Height != genuine {
		t.Fatalf("the proof with its explorer back gave %+v, %v; want confirmed at %d", res, err, genuine)
	}
}
