package p2p

import (
	"crypto/tls"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"nib/internal/sign"
)

// maxFrame caps a single length-prefixed message. The session moves whole PDFs in
// memory (the sign API is []byte, so streaming would save nothing), so a size cap
// — not chunking — is the right bound, mirroring the URL-fetch size limits.
const maxFrame = 128 << 20 // 128 MiB

// Confirmer is the receiving side's consent gate. Shown the connected peer's
// attestation (their identity, accepted-peer, and intent, read from the document
// they signed) and the document itself, it returns whether to co-sign, this user's
// own intent, and the rendered appearance image for this user's visible block (nil
// for an invisible signature). The live UI (P2P 11) implements it; tests inject an
// auto-confirmer.
type Confirmer interface {
	Confirm(peer SignerAttestation, doc []byte) (accept bool, intent string, appearance []byte, err error)
}

// Dial opens a co-signing session to a peer's armed listener at addr, presenting
// this user's identity and accepting only the pinned peer at the TLS handshake. The
// returned conn is a verified mTLS channel; close it when the session ends.
func Dial(addr string, identityCertPEM, identityKeyPEM, pinnedSPKI []byte, timeout time.Duration) (*tls.Conn, error) {
	cfg, err := SessionTLS(identityCertPEM, identityKeyPEM, pinnedSPKI, false)
	if err != nil {
		return nil, err
	}
	return tls.DialWithDialer(&net.Dialer{Timeout: timeout}, "tcp", addr, cfg)
}

// Listen returns a session listener bound to addr that drops any peer but the
// pinned one at the TLS handshake. addr is configurable — loopback for tests, a
// routable bind for a real session; the armed listener (P2P 6) owns the bind and
// lifecycle. The signing logic lives in Receive.
func Listen(addr string, identityCertPEM, identityKeyPEM, pinnedSPKI []byte) (net.Listener, error) {
	cfg, err := SessionTLS(identityCertPEM, identityKeyPEM, pinnedSPKI, true)
	if err != nil {
		return nil, err
	}
	return tls.Listen("tcp", addr, cfg)
}

// Initiate runs the dialing side of a session: it sends the document this user has
// already prepared and signed, receives the fully co-signed result, and confirms
// the peer actually co-signed it and accepted this user. mySignedPDF is the output
// of the local prepare + Contribute; myFingerprint is this user's SPKI pin.
func Initiate(conn *tls.Conn, mySignedPDF, myFingerprint []byte) ([]byte, error) {
	if err := writeFrame(conn, mySignedPDF); err != nil {
		return nil, fmt.Errorf("send document: %w", err)
	}
	final, err := readFrame(conn)
	if err != nil {
		return nil, fmt.Errorf("receive co-signed document: %w", err)
	}
	peerFP, err := verifiedPeerFingerprint(conn.ConnectionState())
	if err != nil {
		return nil, err
	}
	if err := confirmCoSigned(final, peerFP, myFingerprint); err != nil {
		return nil, err
	}
	return final, nil
}

// Receive runs the listening side of a session: it reads the document the connected
// peer signed, verifies the peer is the pinned identity and accepted this user, asks
// the Confirmer for consent and intent, contributes this user's signature, and sends
// the result back. peerLabel is this user's pinned label for the peer (for display).
func Receive(conn *tls.Conn, myCertPEM, myKeyPEM []byte, peerLabel string, c Confirmer) error {
	if err := conn.Handshake(); err != nil {
		return err
	}
	peerFP, err := verifiedPeerFingerprint(conn.ConnectionState())
	if err != nil {
		return err
	}
	inbound, err := readFrame(conn)
	if err != nil {
		return fmt.Errorf("receive document: %w", err)
	}
	final, err := coSignExchange(myCertPEM, myKeyPEM, peerFP, peerLabel, inbound, c)
	if err != nil {
		return err
	}
	if err := writeFrame(conn, final); err != nil {
		return fmt.Errorf("send co-signed document: %w", err)
	}
	return nil
}

// coSignExchange is the transport-agnostic core of the receiving side: given the
// document the connected (and TLS-pinned) peer signed, it verifies the peer's
// attestation binds to this channel, gets the user's consent, contributes this
// user's acceptance signature, and returns the co-signed result. A future gRPC
// transport would be another adapter calling this same function.
func coSignExchange(myCertPEM, myKeyPEM, peerFP []byte, peerLabel string, inbound []byte, c Confirmer) ([]byte, error) {
	ats := ReadAttestations(inbound)
	if len(ats) != 1 {
		return nil, fmt.Errorf("expected exactly one prior signer, got %d", len(ats))
	}
	peer := ats[0]
	if !peer.Valid {
		return nil, errors.New("the peer's signature does not verify")
	}
	// Channel binding (the "right channel / right attested peer" check): the
	// document's signer must be the very identity the TLS handshake pinned — not
	// just any valid signature — and that signer must have accepted *this* user.
	if peer.Fingerprint != hex.EncodeToString(peerFP) {
		return nil, errors.New("the document was not signed by the connected peer")
	}
	myFP, err := sign.Fingerprint(myCertPEM)
	if err != nil {
		return nil, err
	}
	if peer.AcceptedPeer != hex.EncodeToString(myFP) {
		return nil, errors.New("the peer's attestation does not accept you")
	}

	accept, intent, appearance, err := c.Confirm(peer, inbound)
	if err != nil {
		return nil, err
	}
	if !accept {
		return nil, errors.New("co-signing declined")
	}

	idCert, _, err := sign.ParseIdentity(myCertPEM, myKeyPEM)
	if err != nil {
		return nil, err
	}
	place, err := NextPlacement(inbound)
	if err != nil {
		return nil, err
	}
	att := Attestation{
		Signer:            idCert.Subject.CommonName,
		AcceptedPeer:      hex.EncodeToString(peerFP),
		AcceptedPeerLabel: peerLabel,
		Intent:            intent,
		When:              time.Now(),
	}
	return Contribute(inbound, myCertPEM, myKeyPEM, att, appearance, place)
}

// confirmCoSigned checks the returned document carries the connected peer's valid
// signature accepting this user — so the initiator knows the round-trip produced a
// genuine mutual co-signature, not a tampered or substituted reply.
func confirmCoSigned(final, peerFP, myFP []byte) error {
	want := hex.EncodeToString(peerFP)
	for _, a := range ReadAttestations(final) {
		if a.Fingerprint != want {
			continue
		}
		if !a.Valid {
			return errors.New("peer's returned signature does not verify")
		}
		if a.AcceptedPeer != hex.EncodeToString(myFP) {
			return errors.New("peer's signature does not accept you")
		}
		return nil
	}
	return errors.New("returned document is not co-signed by the connected peer")
}

// verifiedPeerFingerprint reads the verified peer's SPKI fingerprint from a
// completed handshake. PeerCertificates is [leaf, identity] and verification has
// already passed (the handshake would have failed otherwise), so the identity cert
// there is the pinned peer.
func verifiedPeerFingerprint(cs tls.ConnectionState) ([]byte, error) {
	if len(cs.PeerCertificates) < 2 {
		return nil, errors.New("peer presented no identity certificate")
	}
	return sign.FingerprintCert(cs.PeerCertificates[1]), nil
}

// writeFrame writes a length-prefixed message: a 4-byte big-endian length then the
// payload.
func writeFrame(w io.Writer, b []byte) error {
	if len(b) > maxFrame {
		return fmt.Errorf("frame too large: %d bytes", len(b))
	}
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(b)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	_, err := w.Write(b)
	return err
}

// readFrame reads a length-prefixed message, rejecting a declared length over the
// cap *before* allocating — the length prefix is attacker-controlled.
func readFrame(r io.Reader) ([]byte, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(hdr[:])
	if n > maxFrame {
		return nil, fmt.Errorf("declared frame too large: %d bytes", n)
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}
