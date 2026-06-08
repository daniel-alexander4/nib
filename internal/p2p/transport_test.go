package p2p

import (
	"bytes"
	"crypto/tls"
	"testing"
	"time"

	"nib/internal/sign"
)

func newIdentity(t *testing.T) (certPEM, keyPEM []byte) {
	t.Helper()
	c, k, err := sign.GenerateIdentity("Test")
	if err != nil {
		t.Fatal(err)
	}
	return c, k
}

func fingerprint(t *testing.T, certPEM []byte) []byte {
	t.Helper()
	f, err := sign.Fingerprint(certPEM)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// rawChain mints a transport leaf for an identity and returns the [leaf, identity]
// DER chain a peer would present.
func rawChain(t *testing.T, certPEM, keyPEM []byte) [][]byte {
	t.Helper()
	idCert, idKey, err := sign.ParseIdentity(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := mintTransportCert(idCert, idKey)
	if err != nil {
		t.Fatal(err)
	}
	return leaf.Certificate
}

func TestVerifyPinnedPeer(t *testing.T) {
	aCert, aKey := newIdentity(t)
	aFP := fingerprint(t, aCert)
	chainA := rawChain(t, aCert, aKey)

	// The pinned peer is accepted and its fingerprint returned.
	got, err := verifyPinnedPeer(chainA, aFP, time.Now())
	if err != nil {
		t.Fatalf("pinned peer rejected: %v", err)
	}
	if !bytes.Equal(got, aFP) {
		t.Errorf("returned fingerprint = %x, want %x", got, aFP)
	}

	// A peer whose identity isn't the pinned one is rejected.
	bCert, _ := newIdentity(t)
	if _, err := verifyPinnedPeer(chainA, fingerprint(t, bCert), time.Now()); err == nil {
		t.Error("accepted a peer that isn't the pinned identity")
	}

	// An incomplete chain (no identity cert) is rejected.
	if _, err := verifyPinnedPeer(chainA[:1], aFP, time.Now()); err == nil {
		t.Error("accepted an incomplete chain")
	}

	// A cert outside its validity window is rejected (freshness / replay bound).
	if _, err := verifyPinnedPeer(chainA, aFP, time.Now().Add(transportTTL+time.Hour)); err == nil {
		t.Error("accepted an expired transport cert")
	}

	// A leaf signed by a DIFFERENT key than the presented (pinned) identity is
	// rejected: present identity A (pin matches) but a leaf B signed by identity B.
	bCert2, bKey2 := newIdentity(t)
	chainB := rawChain(t, bCert2, bKey2)
	idCertA, _, err := sign.ParseIdentity(aCert, aKey)
	if err != nil {
		t.Fatal(err)
	}
	mixed := [][]byte{chainB[0], idCertA.Raw} // leaf signed by B, identity claimed as A
	if _, err := verifyPinnedPeer(mixed, aFP, time.Now()); err == nil {
		t.Error("accepted a leaf not signed by the pinned identity")
	}
}

// TestSessionHandshakeAcceptsPinnedPeer drives a real crypto/tls handshake end to
// end: it proves VerifyPeerCertificate is actually invoked, the mutual pinning
// passes, and each side records the other's verified fingerprint.
func TestSessionHandshakeAcceptsPinnedPeer(t *testing.T) {
	aCert, aKey := newIdentity(t) // listener
	bCert, bKey := newIdentity(t) // dialer
	aFP, bFP := fingerprint(t, aCert), fingerprint(t, bCert)

	srvCfg, srvVP, err := SessionTLS(aCert, aKey, bFP, true) // A accepts B
	if err != nil {
		t.Fatal(err)
	}
	cliCfg, cliVP, err := SessionTLS(bCert, bKey, aFP, false) // B accepts A
	if err != nil {
		t.Fatal(err)
	}

	ln, err := tls.Listen("tcp", "127.0.0.1:0", srvCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	srvErr := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			srvErr <- err
			return
		}
		defer conn.Close()
		srvErr <- conn.(*tls.Conn).Handshake()
	}()

	conn, err := tls.Dial("tcp", ln.Addr().String(), cliCfg)
	if err != nil {
		t.Fatalf("client handshake failed: %v", err)
	}
	defer conn.Close()
	if err := <-srvErr; err != nil {
		t.Fatalf("server handshake failed: %v", err)
	}

	if !bytes.Equal(srvVP.Fingerprint(), bFP) {
		t.Errorf("server verified pin = %x, want %x", srvVP.Fingerprint(), bFP)
	}
	if !bytes.Equal(cliVP.Fingerprint(), aFP) {
		t.Errorf("client verified pin = %x, want %x", cliVP.Fingerprint(), aFP)
	}
}

// TestSessionHandshakeRejectsUnpinnedPeer proves the handshake actually drops a
// peer the listener hasn't pinned — the load-bearing guarantee of the armed listener.
func TestSessionHandshakeRejectsUnpinnedPeer(t *testing.T) {
	aCert, aKey := newIdentity(t) // listener
	bCert, bKey := newIdentity(t) // dialer (a stranger to A)
	cCert, _ := newIdentity(t)    // the only peer A will accept
	aFP, cFP := fingerprint(t, aCert), fingerprint(t, cCert)

	srvCfg, _, err := SessionTLS(aCert, aKey, cFP, true) // A accepts only C
	if err != nil {
		t.Fatal(err)
	}
	cliCfg, _, err := SessionTLS(bCert, bKey, aFP, false) // B dials A
	if err != nil {
		t.Fatal(err)
	}

	ln, err := tls.Listen("tcp", "127.0.0.1:0", srvCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	srvErr := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			srvErr <- err
			return
		}
		defer conn.Close()
		// TLS 1.3 rejects an unpinned *dialer* on the server, when it processes the
		// client certificate — the listener is where the guarantee lives. (The
		// client's Dial may return before it sees the rejection, per TLS 1.3 flight
		// ordering, so we assert on the server side.)
		srvErr <- conn.(*tls.Conn).Handshake()
	}()

	if conn, err := tls.Dial("tcp", ln.Addr().String(), cliCfg); err == nil {
		conn.Close()
	}
	if err := <-srvErr; err == nil {
		t.Fatal("listener accepted an unpinned peer")
	}
}
