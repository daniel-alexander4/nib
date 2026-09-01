package p2p

import (
	"context"
	"sync"
	"testing"
	"time"
)

// gatedReDeliverer holds `Store` open so the ordering it sits in becomes observable from the OTHER
// side of the wire. `entered` closes when Store is called; Store then blocks until `release` closes.
type gatedReDeliverer struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (g *gatedReDeliverer) Cached([]byte) ([]byte, error) { return nil, nil }

func (g *gatedReDeliverer) Store(inbound, final []byte) error {
	g.once.Do(func() { close(g.entered) })
	<-g.release
	return nil
}

// TestTheContributionIsStoredBeforeTheFrameGoesOut is D24's ordering, asserted at RUNTIME over a
// real transport rather than by reading the source.
//
// # What it replaces
//
// The ordering has been guarded since P08.S02 by a source scan (`l3_test.go`: `ceremonyID.Store`'s
// body contains `persistContribution(`) plus the placement of the `rd.Store` call inside
// `coSignExchange`. Neither observes the order actually holding: a scan sees a call, not when it
// runs relative to the wire, and it stays green if the store is moved after the frame in `Receive`
// — the exact regression D24 exists to prevent, and the state the code was in before S02.
// (/pending 343: the property had no runtime reader of any kind.)
//
// # Why it is falsifiable
//
// The store is held open. While it is held, the receiver has not reached `writeFrame`, so the
// INITIATOR is still blocked in `readFrame` and has nothing. If the store were moved after the
// frame, the initiator would already hold the co-signed document by the time `Store` is entered at
// all — so the same two assertions read in the opposite direction. The grace period is what makes
// the negative arm a wait rather than a guess: the frame would already be on the wire, so the
// document arrives within it or the ordering held.
func TestTheContributionIsStoredBeforeTheFrameGoesOut(t *testing.T) {
	eachTransport(t, func(t *testing.T, tr transport) {
		aCert, aKey := newIdentity(t) // initiator
		bCert, bKey := newIdentity(t) // receiver, whose disk this is about
		aFP, bFP := fingerprint(t, aCert), fingerprint(t, bCert)
		aSigned := signAsInitiator(t, aCert, aKey, bFP)

		g := &gatedReDeliverer{entered: make(chan struct{}), release: make(chan struct{})}

		ln, err := tr.listen("127.0.0.1:0", bCert, bKey, aFP)
		if err != nil {
			t.Fatal(err)
		}
		defer ln.Close()

		recvErr := make(chan error, 1)
		go func() {
			conn, aerr := ln.Accept()
			if aerr != nil {
				recvErr <- aerr
				return
			}
			defer conn.Close()
			_, e := Receive(conn.Channel, bCert, bKey, "Alice",
				confirmer{accept: true, intent: "I accept"}, okVerifier{}, g, Roster{})
			recvErr <- e
		}()

		conn, err := tr.dial(context.Background(), ln.Addr().String(), aCert, aKey, bFP, 10*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()

		got := make(chan []byte, 1)
		initErr := make(chan error, 1)
		go func() {
			final, ierr := Initiate(conn.Channel, aSigned, aFP, okVerifier{}, Roster{})
			if ierr != nil {
				initErr <- ierr
				return
			}
			got <- final
		}()

		select {
		case <-g.entered:
		case ierr := <-initErr:
			t.Fatalf("the initiator failed before the store was reached: %v", ierr)
		case <-time.After(30 * time.Second):
			t.Fatal("Store was never called, so this test asserts nothing about its ordering")
		}

		// THE ASSERTION. The signature exists and has not been stored; nothing may have gone out.
		select {
		case <-got:
			t.Error("the initiator holds the co-signed document while this machine's durable " +
				"write is still in flight. The bytes reached the peer before they reached " +
				"disk, so a crash here leaves a signature the counterparty has and this " +
				"machine has no record of — the ordering D24 inverts, restored")
		case ierr := <-initErr:
			t.Errorf("the initiator failed while the store was held: %v", ierr)
		case <-time.After(750 * time.Millisecond):
			// Held. Nothing on the wire.
		}

		close(g.release)

		select {
		case final := <-got:
			if n := len(ReadAttestations(final)); n != 2 {
				t.Errorf("the released exchange delivered %d signers, want 2 — the ordering "+
					"arm above would then be passing over an exchange that never completed", n)
			}
		case ierr := <-initErr:
			t.Fatalf("initiate: %v", ierr)
		case <-time.After(10 * time.Second):
			t.Fatal("the document never arrived after the store was released")
		}
		if e := <-recvErr; e != nil {
			t.Fatalf("receive: %v", e)
		}
	})
}
