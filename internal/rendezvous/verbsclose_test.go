package rendezvous

import (
	"bytes"
	"context"
	"net/netip"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anacrolix/dht/v2/krpc"
	"github.com/anacrolix/torrent/bencode"
)

// TestCloseCancelsEveryVerbThatReachesTheNetwork — /pending 501.
//
// Publish and Fetch were admitted through `enter`; Bootstrap, Ping and ProbeSelf were not, so
// Close neither cancelled nor waited for them. The server reaches this: `disarmWhen` cancels the
// connect goroutine and closes the ceremony without joining it, so `Close` could save the node
// cache and close the DHT while Bootstrap was mid-retry.
//
// **What is asserted is CANCELLATION, and the threshold is below the library's query timeout.** An
// unadmitted verb runs on its caller's context, which Close does not touch, so it stays blocked on
// the held query until that query times out on its own (about two seconds). An admitted one is
// cancelled by Close and returns at once. The join half — that Close does not return first — is
// structural (`inFlight.Wait`) and, as in TestCloseCancelsAndJoinsAnInFlightPublish, is not what
// carries the red.
func TestCloseCancelsEveryVerbThatReachesTheNetwork(t *testing.T) {
	cases := []struct {
		name string
		host string
		// hold is the query the fake holds open; skip lets the first N of them through, so a
		// verb that needs a populated table can get one first.
		hold string
		skip int
		pre  func(t *testing.T, rz *Server, f *fakeNode)
		verb func(ctx context.Context, rz *Server, f *fakeNode) error
	}{
		{name: "Bootstrap", host: "127.0.0.81", hold: "find_node",
			verb: func(ctx context.Context, rz *Server, _ *fakeNode) error { return rz.Bootstrap(ctx) }},
		{name: "Ping", host: "127.0.0.82", hold: "ping",
			verb: func(ctx context.Context, rz *Server, f *fakeNode) error { return rz.Ping(ctx, f.addr) }},
		{name: "ProbeSelf", host: "127.0.0.83", hold: "ping", skip: 1,
			pre: func(t *testing.T, rz *Server, f *fakeNode) {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := rz.Ping(ctx, f.addr); err != nil {
					t.Fatalf("setup: the ping that puts the fake in the table failed: %v", err)
				}
			},
			verb: func(ctx context.Context, rz *Server, _ *fakeNode) error {
				_, err := rz.ProbeSelf(ctx)
				return err
			}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			arrived := make(chan struct{})
			release := make(chan struct{})
			var releaseOnce, arriveOnce sync.Once
			letGo := func() { releaseOnce.Do(func() { close(release) }) }
			t.Cleanup(letGo)
			var seen atomic.Int64

			f := newFakeNode(t, tc.host, func(q krpc.Msg, id krpc.ID) []byte {
				if q.A == nil {
					return nil
				}
				if q.Q == tc.hold && int(seen.Add(1)) > tc.skip {
					arriveOnce.Do(func() { close(arrived) })
					<-release
					return nil // never answered: only a cancel or the query timeout ends it
				}
				b, err := bencode.Marshal(krpc.Msg{T: q.T, Y: krpc.YResponse, R: &krpc.Return{ID: id}})
				if err != nil {
					return nil
				}
				return b
			})
			n := nodeSeeded(t, f)
			if tc.pre != nil {
				tc.pre(t, n.rz, f)
			}

			var returned atomic.Bool
			done := make(chan time.Time, 1)
			go func() {
				_ = tc.verb(context.Background(), n.rz, f)
				returned.Store(true)
				done <- time.Now()
			}()

			// STIMULUS: the verb really is in flight on the wire when Close is called.
			select {
			case <-arrived:
			case <-time.After(20 * time.Second):
				t.Fatalf("setup: no %s reached the fake, so %s never got to the wire", tc.hold, tc.name)
			}
			if returned.Load() {
				t.Fatalf("setup: %s returned before Close was called, so nothing was in flight", tc.name)
			}

			closeStart := time.Now()
			closed := make(chan struct{})
			go func() { n.rz.Close(); close(closed) }()

			var verbEnd time.Time
			select {
			case verbEnd = <-done:
			case <-time.After(30 * time.Second):
				t.Fatalf("%s never returned after Close", tc.name)
			}
			// The fake stops holding once the question has been asked, so the cleanup cannot hang.
			letGo()
			select {
			case <-closed:
			case <-time.After(30 * time.Second):
				t.Fatal("Close did not return")
			}

			const cancelled = time.Second
			if took := verbEnd.Sub(closeStart); took >= cancelled {
				t.Errorf("%s returned %v after Close began — Close did not cancel it, so it ran on "+
					"under a server being torn down until its own query timed out", tc.name, took)
			}
		})
	}
}

// TestATableTheInvitationSeedsCouldHaveFilledIsNeverSaved — /pending 501.
//
// `saveNodes` refused a stranger-seeded table only when `invSeedsUsed` was set, and that flag is
// set only when Bootstrap's retry ITSELF left a table. Once the seeds are tried they sit in
// `StartingNodes` for every later traversal — Publish and Fetch included — so a retry that found
// nothing followed by a traversal that filled the table read as "not from a stranger" and was
// written into the cache every future run bootstraps from.
func TestATableTheInvitationSeedsCouldHaveFilledIsNeverSaved(t *testing.T) {
	n := nodeWithCache(t, deadCache(t))
	n.rz.Seed([]netip.AddrPort{netip.MustParseAddrPort("127.0.0.1:2")})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = n.rz.Bootstrap(ctx)

	// STIMULUS, three facts: the seeds were tried, the retry left no table, and so USED is false —
	// the state the old key read as safe.
	st := n.rz.Stats()
	if !st.InvitationSeedsTried {
		t.Fatal("setup: the retry never ran, so the seeds never entered StartingNodes")
	}
	if st.InvitationSeedsUsed || st.Nodes != 0 {
		t.Fatalf("setup: used=%v nodes=%d — this row is about a retry that found nothing",
			st.InvitationSeedsUsed, st.Nodes)
	}

	// A later exchange fills the table while the seeds are in StartingNodes.
	live := newFakeNode(t, "127.0.0.84", func(q krpc.Msg, id krpc.ID) []byte {
		if q.A == nil {
			return nil
		}
		return bencode.MustMarshal(krpc.Msg{T: q.T, Y: krpc.YResponse, R: &krpc.Return{ID: id}})
	})
	if err := n.rz.Ping(ctx, live.addr); err != nil {
		t.Fatalf("setup: ping: %v", err)
	}
	if n.rz.Stats().Nodes == 0 {
		t.Fatal("setup: the table is still empty, so there is nothing a save could have written")
	}

	path := filepath.Join(n.dir, bootstrapFile)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := n.rz.Close(); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Error("the node cache was rewritten from a table built after the invitation's seeds " +
			"entered StartingNodes — the next run bootstraps from a list nobody can attribute")
	}
	if got := n.rz.Stats().Saved; got != 0 {
		t.Errorf("Saved = %d, want 0", got)
	}
}
