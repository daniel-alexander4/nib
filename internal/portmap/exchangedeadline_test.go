package portmap

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

// noDeadlineConn is a socket whose read deadline cannot be set and on which datagrams that are
// not our answer keep arriving.
type noDeadlineConn struct {
	net.Conn
	reads int
}

var errNoDeadline = errors.New("deadlines are not supported here")

func (c *noDeadlineConn) Write(b []byte) (int, error)     { return len(b), nil }
func (c *noDeadlineConn) SetReadDeadline(time.Time) error { return errNoDeadline }
func (c *noDeadlineConn) Read(b []byte) (int, error) {
	c.reads++
	time.Sleep(time.Millisecond)
	return copy(b, "noise"), nil
}

// TestAnExchangeWhoseDeadlineCannotBeSetReturns — /pending 653. `exchange`'s inner loop ends only
// when a Read fails, and it rested that on a `SetReadDeadline` whose error nothing read: where
// the deadline is not set, any stream of datagrams that are not the answer holds the loop — and
// the goroutine and socket with it — for good.
func TestAnExchangeWhoseDeadlineCannotBeSetReturns(t *testing.T) {
	conn := &noDeadlineConn{}
	done := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		_, err := exchange(ctx, conn, []byte("req"), nil, func([]byte) bool { return false })
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, errNoDeadline) {
			t.Errorf("exchange returned %v, want the deadline's own error", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("exchange is still reading 3 s after a 200 ms budget — its loop's only exit is a read deadline that was never set")
	}
}
