package p2p

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"time"

	"nib/internal/sign"
)

// maxFrame caps a single length-prefixed message. The session moves whole PDFs in
// memory (the sign API is []byte, so streaming would save nothing), so a size cap
// — not chunking — is the right bound, mirroring the URL-fetch size limits.
const maxFrame = 128 << 20 // 128 MiB

// exchangeDeadline is the budget for one PHASE of a session — never for the whole of it.
//
// It must exceed the receiving side's consent window (the server's sessionConsentTimeout,
// 5 min) plus transfer and signing margin; without it a peer that stalls mid-frame ties up
// the single armed session forever (frames are size-capped by maxFrame but were not
// time-capped).
//
// **It is re-armed after every local human gate, and that is a correctness property.**
// `SetDeadline` is absolute wall-clock, so a user reading the four words on screen spends
// the wire budget while no byte is moving. Since P01.S05 there are now TWO human waits in
// one exchange — the spoken check (both roles) and the receiver's consent — and the
// justification above is arithmetic for ONE. Left un-armed, two ordinary users each taking
// ~3.5 minutes at their prompts, comfortably inside the windows the product advertises,
// blow a six-minute absolute budget: the initiator's read times out AFTER the responder
// has co-signed and saved, so both users have signed and only one holds the artifact.
//
// So each of the four entry points re-arms after `runVerification` returns, and the two
// receiving ones re-arm again after consent (postConsentDeadline). The invariant is that
// no single budget ever spans more than one human wait.
const exchangeDeadline = 6 * time.Minute

// postConsentDeadline is a FRESH budget for the I/O that follows the user's decision.
//
// The single absolute deadline above has to cover the remote user's whole consent window
// (5 minutes), so whatever they leave unspent is all that remains for the co-signature and
// for writing back up to maxFrame — 128 MiB. A user who takes four and a half minutes to
// read what they are signing leaves ninety seconds for both. The failure lands in the worst
// possible place: AFTER the local user has signed, so their key has been used on a document
// the peer never receives.
//
// Reset once, at the point the outcome is decided, rather than lengthening the total: a
// longer absolute budget would let a stalling peer hold the single armed session for longer
// too, which is the thing exchangeDeadline exists to bound.
const postConsentDeadline = 2 * time.Minute

// Confirmer is the receiving side's consent gate. Shown the connected peer's
// attestation (their identity, accepted-peer, and intent, read from the document
// they signed) and the document itself, it returns whether to co-sign, this user's
// own intent, and the rendered appearance image for this user's visible block (nil
// for an invisible signature). The live UI (P2P 11) implements it; tests inject an
// auto-confirmer.
type Confirmer interface {
	Confirm(peer SignerAttestation, doc []byte) (accept bool, intent string, appearance []byte, err error)
}

// Initiate runs the dialing side of a session: it sends the document this user has
// already prepared and signed, receives the fully co-signed result, and confirms
// the peer actually co-signed it and accepted this user. mySignedPDF is the output
// of the local prepare + Contribute; myFingerprint is this user's SPKI pin.
func Initiate(ch Channel, mySignedPDF, myFingerprint []byte, v Verifier) ([]byte, error) {
	if err := ch.check(); err != nil {
		return nil, err
	}
	conn := ch.Stream
	peerFP := ch.PeerFP
	_ = conn.SetDeadline(time.Now().Add(exchangeDeadline))
	// The spoken check, before a single document byte (L2).
	if err := runVerification(ch, true, myFingerprint, v); err != nil {
		return nil, err
	}
	// Re-armed: the gate above spent wall-clock while nothing was on the wire. See
	// exchangeDeadline — no budget may span more than one human wait.
	_ = conn.SetDeadline(time.Now().Add(exchangeDeadline))
	if err := writeFrame(conn, mySignedPDF); err != nil {
		return nil, fmt.Errorf("send document: %w", err)
	}
	final, err := readFrame(conn)
	if err != nil {
		return nil, fmt.Errorf("receive co-signed document: %w", err)
	}
	// Bind the result to the document sent THIS session. confirmCoSigned alone
	// accepts any document these two identities ever mutually co-signed — a
	// malicious peer could replay an older co-signed artifact and both signature
	// checks would still pass. The prefix check is sound because the signer is
	// strictly append-only: a legitimate co-signature is always mySignedPDF plus
	// a trailing incremental update (see sign/trailing_test.go).
	if !bytes.HasPrefix(final, mySignedPDF) {
		return nil, errors.New("returned document is not the one sent this session")
	}
	if err := confirmCoSigned(final, peerFP, myFingerprint); err != nil {
		return nil, err
	}
	return final, nil
}

// Receive runs the listening side of a session: it reads the document the connected
// peer signed, verifies the peer is the pinned identity and accepted this user, asks
// the Confirmer for consent and intent, contributes this user's signature, and sends
// the result back — returning the co-signed document so the receiver keeps it too.
// peerLabel is this user's pinned label for the peer (for display).
func Receive(ch Channel, myCertPEM, myKeyPEM []byte, peerLabel string, c Confirmer, v Verifier) ([]byte, error) {
	if err := ch.check(); err != nil {
		return nil, err
	}
	conn := ch.Stream
	peerFP := ch.PeerFP
	_ = conn.SetDeadline(time.Now().Add(exchangeDeadline))
	myFP, err := ownFingerprint(myCertPEM)
	if err != nil {
		return nil, err
	}
	// Before the document is read, not after: reading it first would mean the peer's bytes
	// had already crossed by the time the user was asked who they were talking to.
	if err := runVerification(ch, false, myFP, v); err != nil {
		return nil, err
	}
	// Re-armed: the gate above spent wall-clock while nothing was on the wire. See
	// exchangeDeadline — no budget may span more than one human wait.
	_ = conn.SetDeadline(time.Now().Add(exchangeDeadline))
	inbound, err := readFrame(conn)
	if err != nil {
		return nil, fmt.Errorf("receive document: %w", err)
	}
	final, err := coSignExchange(myCertPEM, myKeyPEM, peerFP, peerLabel, inbound, c)
	if err != nil {
		return nil, err
	}
	// The signature exists now. Give the write its own budget rather than whatever the
	// user's deliberation left over — see postConsentDeadline.
	_ = conn.SetDeadline(time.Now().Add(postConsentDeadline))
	if err := writeFrame(conn, final); err != nil {
		return nil, fmt.Errorf("send co-signed document: %w", err)
	}
	return final, nil
}

// ackOK / ackDeclined are the one-byte receipts ReceiveDocument sends and SendDocument
// waits for, so the sender learns the document was accepted or declined — not merely
// delivered to the socket. An explicit declined byte distinguishes a refusal from a
// dropped connection.
const (
	ackOK       = 1
	ackDeclined = 2
)

// ErrDeclined reports that the receiving user declined a one-way document transfer.
var ErrDeclined = errors.New("document transfer declined")

// Accepter is the receiving side's consent gate for a plain document transfer. Shown
// the verified peer's SPKI fingerprint and the document, it returns whether to accept
// (and save) it. Unlike Confirmer this carries no signing — the document may be an
// unsigned flagged PDF awaiting the user, not a co-signature.
type Accepter interface {
	Accept(peerFP, doc []byte) (accept bool, err error)
}

// SendDocument runs the dialing side of a one-way transfer: it sends the document and
// waits for the receiver's acknowledgement that the user accepted it. Nothing is
// signed and nothing comes back — the pinned-mTLS channel is a plain authenticated
// courier, used to hand a flagged PDF to a peer for signing or to return the signed
// result. The pin is enforced by the TLS config, exactly as in Initiate.
func SendDocument(ch Channel, pdf []byte, myFingerprint []byte, v Verifier) error {
	if err := ch.check(); err != nil {
		return err
	}
	conn := ch.Stream
	_ = conn.SetDeadline(time.Now().Add(exchangeDeadline))
	// A one-way transfer carries document bytes too, so it carries the same gate. The
	// clause is about document bytes, not about co-signing.
	if err := runVerification(ch, true, myFingerprint, v); err != nil {
		return err
	}
	// Re-armed: the gate above spent wall-clock while nothing was on the wire. See
	// exchangeDeadline — no budget may span more than one human wait.
	_ = conn.SetDeadline(time.Now().Add(exchangeDeadline))
	if err := writeFrame(conn, pdf); err != nil {
		return fmt.Errorf("send document: %w", err)
	}
	ack, err := readFrameMax(conn, 1)
	if err != nil {
		return fmt.Errorf("await receipt: %w", err)
	}
	switch {
	case len(ack) == 1 && ack[0] == ackOK:
		return nil
	case len(ack) == 1 && ack[0] == ackDeclined:
		return ErrDeclined
	default:
		return errors.New("unexpected receipt from peer")
	}
}

// ReceiveDocument runs the listening side of a one-way transfer: it reads the document
// the connected (pinned) peer sent, asks the Accepter for consent, and on accept
// returns the document after acknowledging. The peer's fingerprint is not returned
// because the caller supplied it in the Channel — handing it back would invite a
// caller to trust the echo rather than the value it verified. On
// decline it returns ErrDeclined and sends no acknowledgement, so the sender learns
// the document was not kept.
func ReceiveDocument(ch Channel, a Accepter, myFingerprint []byte, v Verifier) (doc []byte, err error) {
	if err := ch.check(); err != nil {
		return nil, err
	}
	conn := ch.Stream
	_ = conn.SetDeadline(time.Now().Add(exchangeDeadline))
	if err := runVerification(ch, false, myFingerprint, v); err != nil {
		return nil, err
	}
	// Re-armed: the gate above spent wall-clock while nothing was on the wire. See
	// exchangeDeadline — no budget may span more than one human wait.
	_ = conn.SetDeadline(time.Now().Add(exchangeDeadline))
	inbound, err := readFrame(conn)
	if err != nil {
		return nil, fmt.Errorf("receive document: %w", err)
	}
	accept, err := a.Accept(ch.PeerFP, inbound)
	if err != nil {
		return nil, err
	}
	// Same reset as Receive's, for the same reason: the acknowledgement is one byte, but
	// it is sent after a wait that can have consumed the whole budget, and a sender that
	// never gets it reports the transfer as failed when the receiver has kept the file.
	_ = conn.SetDeadline(time.Now().Add(postConsentDeadline))
	if !accept {
		_ = writeFrame(conn, []byte{ackDeclined}) // best-effort: tell the sender it was refused
		return nil, ErrDeclined
	}
	if err := writeFrame(conn, []byte{ackOK}); err != nil {
		return nil, fmt.Errorf("acknowledge receipt: %w", err)
	}
	return inbound, nil
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

// confirmCoSigned checks the returned document is a genuine mutual co-signature of
// exactly this channel's two pinned identities: it carries the connected peer's
// valid signature accepting this user AND this user's own valid signature accepting
// the peer. Requiring the initiator's own signature too anchors the result to the
// document this user signed — a peer cannot strip it and substitute a different
// document that only it signed, since a valid signature binds to its own bytes.
func confirmCoSigned(final, peerFP, myFP []byte) error {
	peer, me := hex.EncodeToString(peerFP), hex.EncodeToString(myFP)
	var gotPeer, gotMe bool
	for _, a := range ReadAttestations(final) {
		switch a.Fingerprint {
		case peer:
			if !a.Valid {
				return errors.New("peer's returned signature does not verify")
			}
			if a.AcceptedPeer != me {
				return errors.New("peer's signature does not accept you")
			}
			gotPeer = true
		case me:
			if !a.Valid {
				return errors.New("your own signature is missing or altered in the returned document")
			}
			if a.AcceptedPeer != peer {
				return errors.New("your signature in the returned document does not accept the peer")
			}
			gotMe = true
		}
	}
	if !gotPeer {
		return errors.New("returned document is not co-signed by the connected peer")
	}
	if !gotMe {
		return errors.New("returned document is missing your own signature")
	}
	return nil
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
func readFrame(r io.Reader) ([]byte, error) { return readFrameMax(r, maxFrame) }

// readFrameMax is readFrame bounded by what the caller actually expects.
//
// maxFrame is 128 MiB because a document frame legitimately is. A frame whose expected
// size is 32 bytes has no business admitting that: a peer can declare 128 MiB in four
// bytes and make the listener allocate it before anything looks at the contents. The
// peer must be pinned to get this far, which lowers the severity and does not remove it —
// a pinned peer is a person you have agreed to sign with, not a person you have agreed to
// let allocate 128 MiB per connection.
//
// So every fixed-size frame reads with its own bound and the general reader keeps the
// document's.
func readFrameMax(r io.Reader, max uint32) ([]byte, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(hdr[:])
	if n > max {
		return nil, fmt.Errorf("declared frame too large: %d bytes (max %d)", n, max)
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}
