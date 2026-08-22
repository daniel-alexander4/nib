package p2p

import (
	"net"
	"testing"
	"time"
)

// A punch datagram from one shared endpoint to another is routed to the PEER's DHT view and
// dropped — not treated as a QUIC connection (grill 7b). Asserted via the mux routing counter:
// RoutedToDHT moves, RoutedByPeer/long-header do not, so the peer's quic-go never sees it.
func TestPunchDatagramGoesToThePeersDHTView(t *testing.T) {
	sender, err := NewSharedEndpoint("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer sender.Close()
	peer, err := NewSharedEndpoint("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()

	// A reader draining the peer's DHT view, so the datagram is actually received and counted.
	go func() {
		buf := make([]byte, 1500)
		peer.DHT().SetReadDeadline(time.Now().Add(2 * time.Second))
		peer.DHT().ReadFrom(buf)
	}()

	before := peer.Stats()
	if err := sender.Punch(peer.LocalAddr()); err != nil {
		t.Fatalf("Punch errored: %v", err)
	}
	// Wait for the datagram to be routed.
	deadline := time.Now().Add(2 * time.Second)
	for peer.Stats().RoutedToDHT <= before.RoutedToDHT && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	after := peer.Stats()
	if after.RoutedToDHT <= before.RoutedToDHT {
		t.Errorf("a punch datagram did not reach the peer's DHT view (RoutedToDHT %d→%d)", before.RoutedToDHT, after.RoutedToDHT)
	}
	if after.RoutedByPeer > before.RoutedByPeer || after.RoutedLongHeader > before.RoutedLongHeader {
		t.Errorf("a punch datagram was routed to the peer's QUIC view — it must not look like a connection (byPeer %d→%d, longHeader %d→%d)",
			before.RoutedByPeer, after.RoutedByPeer, before.RoutedLongHeader, after.RoutedLongHeader)
	}
}

var _ = net.UDPAddr{}
