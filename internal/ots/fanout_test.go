package ots

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// pendingProof builds a proof of n pending sequences, all naming url, each with a distinct
// commitment so no two could be collapsed by a memo.
func pendingProof(digest [32]byte, n int, url string) []byte {
	seqs := make([]sequence, 0, n)
	for i := 0; i < n; i++ {
		seqs = append(seqs, sequence{ops: []op{{opAppend, []byte{byte(i), byte(i >> 8)}}, {tag: opSHA256}}, calURL: url})
	}
	return serialize(digest[:], seqs)
}

// countingCalendar answers every upgrade "not yet" and counts the requests.
func countingCalendar(t *testing.T) (*httptest.Server, *int64) {
	t.Helper()
	var hits int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&hits, 1)
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

// TestAProofCannotFanOutCalendarUpgrades — /pending 502.
//
// Every pending sequence in an untrusted `.ots` was one sequential GET to the calendar URL the file
// names: 500 were reproduced from a 17 KB proof. The explorer half of VerifyProof had a cap
// (maxAttestationsExamined); the upgrade half had none.
func TestAProofCannotFanOutCalendarUpgrades(t *testing.T) {
	var digest [32]byte
	digest[0] = 7

	t.Run("a proof past the cap is refused before any request", func(t *testing.T) {
		srv, hits := countingCalendar(t)
		const n = 500
		proof := pendingProof(digest, n, srv.URL)
		// STIMULUS: the file really parses to n pending sequences, or the count below is about a
		// proof the parser rejected for some other reason.
		p, err := parseProof(proof)
		if err != nil || len(p.seqs) != n {
			t.Fatalf("setup: the proof parsed to %d sequences (err %v), want %d", len(p.seqs), err, n)
		}
		_, err = VerifyProof(context.Background(), srv.Client(), nil, 1, proof, digest)
		if !errors.Is(err, ErrProofTooComplex) {
			t.Errorf("a proof with %d pending commitments returned %v; it must be refused as too complex", n, err)
		}
		if got := atomic.LoadInt64(hits); got != 0 {
			t.Errorf("the refused proof still drove %d calendar requests", got)
		}
	})

	t.Run("a proof at the cap is fetched, one request per commitment", func(t *testing.T) {
		srv, hits := countingCalendar(t)
		proof := pendingProof(digest, maxPendingUpgrades, srv.URL)
		res, err := VerifyProof(context.Background(), srv.Client(), nil, 1, proof, digest)
		if err != nil || res.State != StatePending {
			t.Fatalf("a proof with %d pending commitments: res %+v err %v; want pending", maxPendingUpgrades, res, err)
		}
		if got := atomic.LoadInt64(hits); got != int64(maxPendingUpgrades) {
			t.Errorf("%d pending commitments drove %d requests, want one each", maxPendingUpgrades, got)
		}
	})
}

// TestAnExpiredDeadlineIsNotReportedAsPending — /pending 502.
//
// The handler now gives VerifyProof a deadline. An upgrade that fails because that deadline passed
// was folded into "calendar hasn't confirmed yet", so running out of time would have told the user
// their proof is unconfirmed.
func TestAnExpiredDeadlineIsNotReportedAsPending(t *testing.T) {
	var digest [32]byte
	digest[0] = 9
	srv, _ := countingCalendar(t)
	proof := pendingProof(digest, 2, srv.URL)

	// STIMULUS: with time to spare the same proof really is pending.
	if res, err := VerifyProof(context.Background(), srv.Client(), nil, 1, proof, digest); err != nil || res.State != StatePending {
		t.Fatalf("setup: res %+v err %v; want pending", res, err)
	}

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	res, err := VerifyProof(ctx, srv.Client(), nil, 1, proof, digest)
	if err == nil {
		t.Errorf("a verification whose deadline had passed reported %+v; it must say it ran out of time", res)
	}
}

type failingSource struct{}

func (failingSource) BlockHeader(context.Context, uint64) ([]byte, time.Time, error) {
	return nil, time.Time{}, errors.New("down")
}

// TestAZeroThresholdWithNoAnswersDoesNotPanic — /pending 502 (info).
//
// fetchAgreedHeader compared `len(results) < minAgree` and then read `results[0]`; a threshold of
// zero with no source answering indexed an empty slice.
func TestAZeroThresholdWithNoAnswersDoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("fetchAgreedHeader panicked with minAgree 0 and no answers: %v", r)
		}
	}()
	_, _, n, err := fetchAgreedHeader(context.Background(), []BlockSource{failingSource{}}, 0, 800000)
	if err == nil {
		t.Errorf("no source answered and the threshold was 0, yet it reported agreement from %d", n)
	}
}
