package server

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// lateConn is a won connection that reports being closed.
type lateConn struct {
	once   sync.Once
	closed chan struct{}
}

func newLateConn() *lateConn { return &lateConn{closed: make(chan struct{})} }

func (c *lateConn) Close() error {
	c.once.Do(func() { close(c.closed) })
	return nil
}

func wantClosed(t *testing.T, c *lateConn, how string) {
	t.Helper()
	select {
	case <-c.closed:
	case <-time.After(3 * time.Second):
		t.Errorf("a dial that won after the race had returned (%s) was never closed: the connection "+
			"stays open on the peer's side, where its serial loop reads it for the whole exchange "+
			"deadline while the connection that is wanted waits behind it", how)
	}
}

// TestARaceThatReturnsEarlyClosesADialThatWinsLate — /pending 772, /pending 623.
//
// The drain that closes losers was spawned on the WIN path only. `raceCandidates` has two returns
// that leave dials in flight without a winner — the caller's context ending, and a caller error —
// and a dial that completed after either was parked in the buffered results channel for good.
//
// **The dial here ignores its context on purpose.** A handshake that has already completed when
// the cancel lands is exactly the case: cancelling stops a dial, never a connection.
func TestARaceThatReturnsEarlyClosesADialThatWinsLate(t *testing.T) {
	t.Run("the caller's context ends", func(t *testing.T) {
		in := make(chan candidate, 1) // never closed: a trickle source
		in <- candidate{Addr: "10.0.0.1:1", Transport: "tcp", Source: sourceLAN}
		parent, cancel := context.WithCancel(context.Background())
		defer cancel()
		started, release := make(chan struct{}), make(chan struct{})
		won := newLateConn()

		returned := make(chan error, 1)
		go func() {
			_, err := raceCandidates(parent, in, func(context.Context, candidate) (*lateConn, error) {
				close(started)
				<-release
				return won, nil
			})
			returned <- err
		}()
		<-started
		cancel()
		// STIMULUS: the race returned WITHOUT a winner, with the dial still in flight.
		select {
		case err := <-returned:
			if err == nil {
				t.Fatal("setup: the race returned a winner, so the early return was never taken")
			}
		case <-time.After(3 * time.Second):
			t.Fatal("setup: the race did not return when its caller's context ended")
		}
		close(release)
		wantClosed(t, won, "its caller's context had ended")
	})

	t.Run("a caller error", func(t *testing.T) {
		in := make(chan candidate, 2)
		in <- candidate{Addr: "10.0.0.1:1", Transport: "tcp", Source: sourceLAN}
		in <- candidate{Addr: "10.0.0.2:1", Transport: "nonsense", Source: sourceLAN}
		close(in)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		started, release := make(chan struct{}), make(chan struct{})
		won := newLateConn()

		_, err := raceCandidates(ctx, in, func(_ context.Context, c candidate) (*lateConn, error) {
			if c.Transport == "nonsense" {
				<-started // the other dial is in flight before this one fails the race
				return nil, errUnknownTransport
			}
			close(started)
			<-release
			return won, nil
		})
		if !errors.Is(err, errUnknownTransport) {
			t.Fatalf("setup: the race returned %v, want the caller error — the early return was never taken", err)
		}
		close(release)
		wantClosed(t, won, "a caller error had ended it")
	})
}
