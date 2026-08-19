package udpmux

import (
	"sync"
	"time"
)

// deadline is the read or write deadline of one side. It is a faithful port of the
// stdlib's own pipeDeadline (net/pipe.go) because the semantics are subtle and
// already solved: the zero time means NO deadline, a time in the past expires
// immediately, and setting a new deadline after one expired must UN-expire the side.
//
// That last property is not academic here. quic-go's Transport.Close on a
// user-supplied conn calls SetReadDeadline(time.Now()) to unblock its read loop and
// then resets it to the zero time in a defer (transport.go:460). A deadline that
// could not be un-expired would leave the view permanently unreadable after any
// close of a transport sharing the socket.
//
// The wait-on-Stop below is the part that is easy to get wrong and easy to miss:
// time.Timer.Stop reports false when the callback has already started, and the
// callback closes the channel this method is about to replace. Without the wait, a
// deadline set at exactly the wrong moment is closed a moment later by the timer it
// thought it had cancelled — a spurious timeout with no cause visible at the call.
type deadline struct {
	mu     sync.Mutex
	timer  *time.Timer
	waitCh chan struct{}
}

func (d *deadline) init() {
	d.waitCh = make(chan struct{})
}

// wait returns a channel that is closed once the deadline has passed.
func (d *deadline) wait() <-chan struct{} {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.waitCh
}

func (d *deadline) set(t time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.timer != nil && !d.timer.Stop() {
		<-d.waitCh // the callback has started; wait for it to finish closing the channel
	}
	d.timer = nil

	closed := isClosedChan(d.waitCh)
	if t.IsZero() {
		if closed {
			d.waitCh = make(chan struct{})
		}
		return
	}
	if dur := time.Until(t); dur > 0 {
		if closed {
			d.waitCh = make(chan struct{})
		}
		ch := d.waitCh
		d.timer = time.AfterFunc(dur, func() { close(ch) })
		return
	}
	if !closed {
		close(d.waitCh)
	}
}

func isClosedChan(c <-chan struct{}) bool {
	select {
	case <-c:
		return true
	default:
		return false
	}
}
