package server

import (
	"bytes"
	"os"
	"testing"
	"time"

	"nib/internal/discovery"
	"nib/internal/vault"
)

// timedBrowser delivers announcements at scheduled offsets from the moment it is built,
// and otherwise blocks until the caller's deadline the way a socket with nothing on it
// does. `fakeBrowser`'s script is instantaneous, which cannot express the only question
// this file asks: what a browse does with TIME between two announcers.
type timedBrowser struct {
	start time.Time
	at    []time.Duration
	seen  []discovery.Seen
	i     int
}

func (b *timedBrowser) Read(deadline time.Time) (discovery.Seen, error) {
	if b.i < len(b.at) {
		due := b.start.Add(b.at[b.i])
		if !due.After(deadline) {
			if d := time.Until(due); d > 0 {
				time.Sleep(d)
			}
			s := b.seen[b.i]
			b.i++
			return s, nil
		}
	}
	if d := time.Until(deadline); d > 0 {
		time.Sleep(d)
	}
	return discovery.Seen{}, os.ErrDeadlineExceeded
}

// TestABrowseStopsOnceTheLinkGoesQuiet.
//
// `findPeerOnLAN` asks about exactly one pin and used to wait D16's full window after the
// answer arrived in milliseconds. Both bounds are asserted and the LOWER one is the point:
// a browse that returned the instant it heard anything would pass an "it was fast" check
// and reintroduce the capture this file's sibling test defends — the genuine peer's
// announcement arriving one tick after an impostor's would never be collected.
func TestABrowseStopsOnceTheLinkGoesQuiet(t *testing.T) {
	ada := fpOf(21)
	pins := []vault.PinnedPeer{{Fingerprint: ada, Label: "Ada"}}
	const window = 3 * time.Second

	b := &timedBrowser{
		start: time.Now(),
		at:    []time.Duration{0},
		seen:  []discovery.Seen{announcementFrom(t, ada, 8443, "10.0.0.1")},
	}

	start := time.Now()
	got := browsePeers(b, pins, window)
	elapsed := time.Since(start)

	// STIMULUS: the candidate really was collected. A browse that returned nothing
	// would satisfy "it was quick" for entirely the wrong reason.
	if len(got) != 1 || !bytes.Equal(got[0].Fingerprint, ada) {
		t.Fatalf("setup: browse returned %+v, want exactly Ada — there is no early exit to "+
			"measure if the candidate was never heard", got)
	}
	if elapsed >= window-browseQuiet {
		t.Errorf("the browse spent %v of its %v window after hearing its answer at ~0s; "+
			"every LAN ceremony pays that, and a racing ladder gives the tier away to a "+
			"slower one that reported sooner", elapsed, window)
	}
	if elapsed < browseQuiet {
		t.Errorf("the browse returned after %v, less than one quiet period (%v) — it stopped "+
			"at the first announcement it heard, so a second announcer offset by one "+
			"announceEvery (%v) would never be collected", elapsed, browseQuiet, announceEvery)
	}
	t.Logf("returned in %v with a %v window and a %v quiet period", elapsed, window, browseQuiet)
}

// TestAnAnnouncerOffsetByOnePeriodIsStillHeard is what sizes browseQuiet.
//
// Two hosts can legitimately claim one name — the name is broadcast in the clear every
// 500 ms — and the multi-candidate fix exists so the caller gets both and can try both.
// The quiet period must therefore be at least one `announceEvery`, because that is the
// most two independent tickers can be offset by. A shorter one silently re-creates the
// capture attack: whoever announces first ends the browse.
func TestAnAnnouncerOffsetByOnePeriodIsStillHeard(t *testing.T) {
	ada := fpOf(22)
	pins := []vault.PinnedPeer{{Fingerprint: ada, Label: "Ada"}}

	// STIMULUS: the two announcements must straddle a full announce period, or this test
	// is not asking its own question.
	offset := announceEvery
	if offset >= browseQuiet {
		t.Fatalf("setup: browseQuiet (%v) is not longer than one announceEvery (%v), so the "+
			"second announcer below cannot distinguish a correct bound from a lucky one",
			browseQuiet, offset)
	}
	// THREE, spaced one period apart, and the third is the one that matters. Two inside
	// a single quiet window would be collected even by a browse that set the deadline
	// once at the first candidate and never moved it — so a two-announcer test cannot
	// tell a quiet period from a fixed grace period, and the reset is the whole
	// mechanism. The third lands at 2x offset, past the first window's expiry.
	b := &timedBrowser{
		start: time.Now(),
		at:    []time.Duration{0, offset, 2 * offset},
		seen: []discovery.Seen{
			announcementFrom(t, ada, 8443, "10.0.0.1"),
			// The same pinned peer at a DIFFERENT address: a second host claiming the
			// name, or the same peer on a second interface. Both are candidates.
			announcementFrom(t, ada, 8443, "10.0.0.2"),
			announcementFrom(t, ada, 8443, "10.0.0.3"),
		},
	}
	// STIMULUS: the third really is outside the quiet window opened by the first, or it
	// proves nothing about the reset.
	if 2*offset <= browseQuiet {
		t.Fatalf("setup: the third announcer at %v is inside the first candidate's quiet "+
			"window (%v), so it would be heard with or without the reset", 2*offset, browseQuiet)
	}

	got := browsePeers(b, pins, 5*time.Second)

	if len(got) != 3 {
		t.Fatalf("browse returned %d candidates, want 3 — the announcers were %v apart, which "+
			"is one announceEvery and therefore inside what a browse must still hear; a "+
			"quiet period that does not RESET on each new candidate stops after the first: %+v",
			len(got), offset, got)
	}
	for i, want := range []string{"10.0.0.1:8443", "10.0.0.2:8443", "10.0.0.3:8443"} {
		if got[i].Addr != want {
			t.Errorf("candidate %d is %q, want %q — in the order heard", i, got[i].Addr, want)
		}
	}
}

// TestBrowseQuietIsDerivedFromTheAnnouncer pins the relationship rather than the number.
//
// The value is not a tuning constant: it is "one announce period plus slack", and if
// `announceEvery` ever moves this must move with it. A test asserting `750ms` would go on
// passing while the two drifted apart, which is the failure the constant's own doc names.
func TestBrowseQuietIsDerivedFromTheAnnouncer(t *testing.T) {
	if browseQuiet <= announceEvery {
		t.Errorf("browseQuiet is %v and announceEvery is %v; a quiet period no longer than "+
			"one announce period cannot outlast an announcer whose ticker is offset from "+
			"the first one's", browseQuiet, announceEvery)
	}
	if browseQuiet >= browseWindow {
		t.Errorf("browseQuiet is %v and browseWindow is %v; a quiet period at or past the "+
			"window can never fire, so the early exit is dead code", browseQuiet, browseWindow)
	}
}
