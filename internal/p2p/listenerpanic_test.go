package p2p

import (
	"errors"
	"net"
	"strings"
	"testing"
	"time"
)

// TestAPanickingAcceptLoopClosesTheListener — /pending 501.
//
// Both accept loops end through `Close` on their own error paths, but a panic unwound past those
// calls: `safe.Recover` kept the process alive and nothing closed `done`, so `Accept` blocked until
// someone else closed the listener — the whole arm window — on a listener that could accept nothing.
func TestAPanickingAcceptLoopClosesTheListener(t *testing.T) {
	l := newListenerCore()
	shut := make(chan struct{})
	l.shut = func() error { close(shut); return nil }
	entered := make(chan struct{})
	l.loop = func() {
		close(entered)
		panic("the accept loop blew up")
	}

	errc := make(chan error, 1)
	go func() {
		_, err := l.Accept()
		errc <- err
	}()

	// STIMULUS: the loop ran, so the panic happened.
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("setup: the accept loop never started")
	}

	select {
	case err := <-errc:
		if !errors.Is(err, net.ErrClosed) {
			t.Errorf("Accept returned %v; want an error wrapping net.ErrClosed, which is how the "+
				"session loop ends", err)
		}
		if err == nil || !strings.Contains(err.Error(), errAcceptLoopEnded.Error()) {
			t.Errorf("Accept returned %v, which does not say the loop ended — a diagnostic cannot "+
				"tell this from an ordinary Close", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Accept is still blocked after the accept loop panicked — nothing closed the listener")
	}
	select {
	case <-shut:
	default:
		t.Error("the transport beneath the listener was not shut")
	}
}
