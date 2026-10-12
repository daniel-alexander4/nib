package ceremony

import (
	"errors"
	"sync"
	"testing"
	"time"
)

// TestConcurrentCloseOutsLeaveOneReceipt — /pending 668's residue, ADR-012's write-once rule.
//
// `WriteReceipt` read, found nothing, and renamed a receipt into place; a close-out has three
// callers no one lock covers (the sweep, the leave route, the decline), so two could both find
// nothing and the later rename replaced the earlier one's state — both having been told they wrote
// it. Driven with callers that DISAGREE about the state, because agreeing callers cannot show a
// lost write.
func TestConcurrentCloseOutsLeaveOneReceipt(t *testing.T) {
	const id = "0123456789abcdef0123456789abcdef"
	states := []string{StateDeclined, StateAbandoned, StateLeft, StateCompleted}
	const callers = 16
	for round := 0; round < 25; round++ {
		root := t.TempDir()
		errs := make([]error, callers)
		start := make(chan struct{})
		var wg sync.WaitGroup
		for i := 0; i < callers; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start
				errs[i] = WriteReceipt(root, id, Receipt{
					Ceremony: id, State: states[i%len(states)], ObservedAt: time.Unix(1_700_000_000, 0).UTC(),
				})
			}(i)
		}
		close(start)
		wg.Wait()

		got, rerr := ReadReceipt(root, id)
		if rerr != nil {
			t.Fatalf("round %d: after %d concurrent close-outs the receipt does not read: %v", round, callers, rerr)
		}
		for i, err := range errs {
			mine := states[i%len(states)]
			switch {
			case err == nil && mine != got.State:
				t.Fatalf("round %d: a close-out recording %q was told it succeeded and the receipt "+
					"says %q — two callers both found no receipt and one write replaced the other",
					round, mine, got.State)
			case err != nil && mine == got.State:
				t.Fatalf("round %d: a close-out recording the state that stands (%q) was refused: %v",
					round, mine, err)
			case err != nil && !errors.Is(err, ErrReceiptConflict):
				t.Fatalf("round %d: a close-out that lost the race was answered %v, want %v — the "+
					"loser must read the winner's whole receipt, never a partial one", round, err, ErrReceiptConflict)
			}
		}
	}
}
