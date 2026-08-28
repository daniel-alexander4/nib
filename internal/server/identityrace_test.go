package server

import (
	"bytes"
	"sync"
	"testing"
)

// TestConcurrentFirstCallersAllGetTheStoredIdentity — /pending 285, the measurement inverted.
//
// **What was measured before the fix:** eight concurrent callers against one fresh vault produced
// **3 distinct identities, 6 of them holding a certificate the vault did not hold.** `identity()`
// was a read-then-write across two separate vault lock holds — `Identity()` said absent, the caller
// minted, `SetIdentity` overwrote — with nothing holding the lock across the gap.
//
// It needs only near-simultaneous FIRST calls, which two browser tabs will do, and `identity()` has
// eight callers. For `finalize` a loser signed with an orphaned key, which is bad and local. For a
// **ceremony record** it is durable and cross-party: the record names a convener whose key the
// machine discarded, so no later hop can act as convener and no re-convene can prove continuity.
//
// **Two assertions, and the second is the one with teeth.** That every caller agrees is necessary
// and not sufficient — they could all agree on a key the vault does not hold, which is a different
// disaster with the same symptom-free look. So this also reads the vault back and requires the
// agreed identity to be the STORED one.
// **It runs ROUNDS, and that is not belt-and-braces.** A race is probabilistic: the first version
// of this test reproduced 5 distinct identities against the unconditional overwrite on one machine
// and **zero** on the next run, which made its red proof report "the check still PASSED". A guard
// that catches a race only sometimes is worse than none — it reads as a pass on the run where it
// mattered. Rounds convert "this race sometimes fires" into "it fires at least once in N fresh
// vaults", and the failure names the round so a future flake is a number rather than a mystery.
func TestConcurrentFirstCallersAllGetTheStoredIdentity(t *testing.T) {
	const rounds = 40
	for round := range rounds {
		if !identityRound(t, round) {
			return // already reported; more rounds would only repeat it
		}
	}
}

// identityRound drives one fresh vault and reports whether it held. Split out so each round gets
// its own vault: a second round against a vault that already has an identity takes the fast path
// and proves nothing.
func identityRound(t *testing.T, round int) bool {
	t.Helper()
	_, v := unlockedServer(t)

	// STIMULUS: the vault really starts with no identity. Every caller below would otherwise take
	// the fast path, agree trivially, and prove nothing about the race.
	if _, _, ok := v.Identity(); ok {
		t.Fatal("setup: the fresh vault already holds an identity, so no caller reaches the " +
			"minting path and this test cannot discriminate")
	}

	const n = 8
	certs := make([][]byte, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start // release them together; a staggered start is the serial case
			c, _, err := identity(v)
			certs[i], errs[i] = c, err
		}(i)
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("round %d, caller %d: %v", round, i, err)
		}
	}
	distinct := 0
	for i, c := range certs {
		dup := false
		for j := 0; j < i; j++ {
			if bytes.Equal(c, certs[j]) {
				dup = true
				break
			}
		}
		if !dup {
			distinct++
		}
	}
	if distinct != 1 {
		t.Errorf("round %d: %d concurrent first callers minted %d DISTINCT identities; every one "+
			"of them believes it holds this machine's signing key, and all but one is wrong. A "+
			"ceremony record written by a loser names a convener whose key this machine discarded.",
			round, n, distinct)
		return false
	}

	stored, _, ok := v.Identity()
	if !ok {
		t.Fatalf("round %d: after %d callers the vault holds NO identity at all", round, n)
	}
	for i, c := range certs {
		if !bytes.Equal(c, stored) {
			t.Errorf("round %d: caller %d was handed a certificate the vault does not hold — "+
				"agreeing on a key nobody stored is the same orphan by another route", round, i)
			return false
		}
	}
	return true
}
