package p2p

import "testing"

// TestOneTransportRefusesTwoListeners documents the constraint that shapes P05.S09's coordinator:
// a quic.Transport permits ONE Listener, so a ceremony arm cannot keep a stream-opening
// QUICListenOn AND add the coordinator's handshaked QUICListenHandshakeOn on the same shared
// endpoint. The coordinator therefore owns the single (handshaked) listener, and the arm routes
// through it rather than accepting on its own.
func TestOneTransportRefusesTwoListeners(t *testing.T) {
	aCert, aKey := newIdentity(t)
	bCert, _ := newIdentity(t)
	bFP := fingerprint(t, bCert)
	e, err := NewSharedEndpoint("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	l1, err := QUICListenOn(e, aCert, aKey, bFP)
	if err != nil {
		t.Fatalf("first listen: %v", err)
	}
	defer l1.Close()
	if l2, err := QUICListenHandshakeOn(e, aCert, aKey, bFP); err == nil {
		l2.Close()
		t.Fatal("a second Listen on one transport SUCCEEDED — the coordinator's single-listener design assumes it cannot")
	}
}
