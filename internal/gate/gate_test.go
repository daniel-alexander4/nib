package gate

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestAtMostWidthRunAtOnce(t *testing.T) {
	g := New(2, 16)
	var mu sync.Mutex
	cur, peak := 0, 0
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rel, err := g.Acquire(context.Background())
			if err != nil {
				t.Error(err)
				return
			}
			mu.Lock()
			cur++
			if cur > peak {
				peak = cur
			}
			mu.Unlock()
			time.Sleep(5 * time.Millisecond)
			mu.Lock()
			cur--
			mu.Unlock()
			rel()
		}()
	}
	wg.Wait()
	if peak != 2 {
		t.Fatalf("peak concurrency %d, want exactly 2 (the width, and reached)", peak)
	}
	if g.Held() != 0 || g.Queued() != 0 {
		t.Fatalf("after all runs: held %d queued %d, want 0 0", g.Held(), g.Queued())
	}
}

// With 2 held and 16 queued, exactly the 17th is refused — by name, and without being left behind.
func TestTheSeventeenthWaiterIsRefusedByName(t *testing.T) {
	g := New(2, 16)
	var holds []func()
	for i := 0; i < 2; i++ {
		r, err := g.Acquire(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		holds = append(holds, r)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errs := make(chan error, 17)
	for i := 0; i < 16; i++ {
		go func() {
			r, err := g.Acquire(ctx)
			if err == nil {
				r()
			}
			errs <- err
		}()
	}
	for g.Queued() != 16 {
		time.Sleep(time.Millisecond)
	}
	if _, err := g.Acquire(ctx); !errors.Is(err, ErrQueueFull) {
		t.Fatalf("the 17th waiter = %v, want ErrQueueFull", err)
	}
	if g.Refused() != 1 || g.Queued() != 16 {
		t.Fatalf("refused %d queued %d, want 1 and 16 (a refusal leaves no trace in the queue)", g.Refused(), g.Queued())
	}
	cancel()
	for i := 0; i < 16; i++ {
		if err := <-errs; !errors.Is(err, context.Canceled) {
			t.Fatalf("a cancelled waiter = %v", err)
		}
	}
	for _, r := range holds {
		r()
	}
}

// A queued run that is cancelled holds no slot, and a third acquire then gets the freed one.
func TestACancelledQueuedRunHoldsNothingAndALaterOneProceeds(t *testing.T) {
	g := New(1, 4)
	hold, _ := g.Acquire(context.Background())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := g.Acquire(ctx); done <- err }()
	for g.Queued() != 1 {
		time.Sleep(time.Millisecond)
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled acquire = %v", err)
	}
	if g.Queued() != 0 {
		t.Fatalf("queue depth %d after the cancel, want 0", g.Queued())
	}
	hold()
	third, err := g.Acquire(context.Background())
	if err != nil {
		t.Fatalf("a later acquire after the cancel: %v", err)
	}
	third()
}

func TestReleaseTwiceFreesOneSlotOnly(t *testing.T) {
	g := New(1, 0)
	r, _ := g.Acquire(context.Background())
	r()
	r()
	a, err := g.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.Acquire(context.Background()); !errors.Is(err, ErrQueueFull) {
		t.Fatalf("a double release freed a second slot: %v", err)
	}
	a()
}

func TestAnAcquireOnAnEndedContextTakesNothing(t *testing.T) {
	g := New(1, 1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := g.Acquire(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
	if g.Held() != 0 {
		t.Fatal("an ended context took a slot")
	}
}

// The budget starts at acquire: a run that waited its turn still gets its whole budget.
func TestTheBudgetStartsAtAcquire(t *testing.T) {
	g := New(1, 4)
	hold, _ := g.Acquire(context.Background())
	got := make(chan time.Duration, 1)
	go func() {
		_ = g.Run(context.Background(), 400*time.Millisecond, nil, func(ctx context.Context) error {
			dl, _ := ctx.Deadline()
			got <- time.Until(dl)
			return nil
		})
	}()
	time.Sleep(300 * time.Millisecond) // waits longer than most of the budget
	hold()
	if left := <-got; left < 300*time.Millisecond {
		t.Fatalf("only %v of a 400ms budget was left after waiting: the clock started at the request", left)
	}
}

func TestRunReleasesWhenTheFunctionPanicsAndWhenAdmitRefuses(t *testing.T) {
	g := New(1, 0)
	func() {
		defer func() { _ = recover() }()
		_ = g.Run(context.Background(), time.Second, nil, func(context.Context) error { panic("boom") })
	}()
	if g.Held() != 0 {
		t.Fatal("a panicking run kept its slot")
	}
	want := errors.New("no room")
	if err := g.Run(context.Background(), time.Second, func(context.Context) error { return want }, func(context.Context) error {
		t.Fatal("fn ran after admit refused")
		return nil
	}); !errors.Is(err, want) {
		t.Fatalf("Run = %v, want admit's refusal", err)
	}
	if g.Held() != 0 {
		t.Fatal("a refused admit kept its slot")
	}
}
