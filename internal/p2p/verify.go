package p2p

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"errors"
	"fmt"

	"nib/internal/pairing"
)

// The spoken verification string (D4), and the commitment step that is the whole of its
// security argument.
//
// # What it is for
//
// Both sides derive four words from the session and one party reads them aloud. Two
// people who recognise each other's voices confirming that the words match is the one
// check an email-level attacker cannot pass: a man-in-the-middle must run two separate
// TLS sessions, so the two legs have different key material and the strings differ.
//
// # Why the commitment is not optional
//
// Four words from a 2048-word list is 44 bits. If each side chose its contribution after
// seeing the other's, a man-in-the-middle could enumerate candidates on the A-leg,
// enumerate on the B-leg, and search for a collision BETWEEN the two sets. That is a
// birthday problem over 44 bits — about 2²² cheap operations, seconds on one machine, and
// well inside the window in which two people are still saying hello. This is the textbook
// short-authentication-string failure; ZRTP and PGPfone both carry a commitment step for
// exactly this reason.
//
// With the commitment, each side is bound to a contribution it chose before it saw the
// other's. The attacker can no longer search — it must GUESS, at 2⁻⁴⁴ per attempt, and a
// wrong guess is heard by two people.
//
// # The contribution is its own nonce
//
// D4's pin writes the commitment as `H(nonce ‖ contribution)`. That form anticipates a
// structured or low-entropy contribution, whose hash could be brute-forced back. A
// uniformly random 32-byte value carries 256 bits, so its SHA-256 is preimage-resistant
// on its own and a separate nonce would add a field without adding a property. The
// substance of the pin — that the string derives only over values committed before either
// side saw the other's — is unchanged.

// contributionLen is 32 bytes of uniform randomness: the per-session secret each side
// commits to before either reveals.
const contributionLen = 32

var (
	// errCommitmentBroken is the out-of-order reveal, and it is the failure this whole
	// file exists to make impossible. It means the peer sent a contribution that is not
	// the one it committed to — which is what choosing after seeing looks like on the wire.
	errCommitmentBroken = errors.New("peer's contribution does not match the commitment it sent")
	errShortCommitment  = errors.New("commitment is not a SHA-256 digest")
	errShortReveal      = errors.New("contribution is not 32 bytes")
)

// verificationExchange runs commit-then-reveal over conn and returns the four-word string
// both sides must compare.
//
// initiator says which side writes first. The two orderings are mirror images and both
// commit before either reveals — the property does not depend on who goes first, only on
// no reveal preceding both commitments.
//
// **Precondition: conn already carries a deadline.** This function does four blocking
// reads and sets none of its own, so a peer that opens a connection and then says nothing
// would hold the listener forever. Its only callers are inside Initiate and Receive, which
// set exchangeDeadline before anything crosses the wire; stated here because the next
// caller to be written is the one that will not know.
func verificationExchange(conn *tls.Conn, initiator bool, myFP, peerFP []byte) (string, error) {
	mine := make([]byte, contributionLen)
	if _, err := rand.Read(mine); err != nil {
		return "", fmt.Errorf("generate contribution: %w", err)
	}
	myCommit := sha256.Sum256(mine)

	var theirCommit, theirs []byte
	var err error

	// Both commitments cross before either contribution does. That ordering is the
	// property; everything else here is bookkeeping.
	if initiator {
		if err = writeFrame(conn, myCommit[:]); err != nil {
			return "", fmt.Errorf("send commitment: %w", err)
		}
		if theirCommit, err = readFrameMax(conn, sha256.Size); err != nil {
			return "", fmt.Errorf("receive commitment: %w", err)
		}
	} else {
		if theirCommit, err = readFrameMax(conn, sha256.Size); err != nil {
			return "", fmt.Errorf("receive commitment: %w", err)
		}
		if err = writeFrame(conn, myCommit[:]); err != nil {
			return "", fmt.Errorf("send commitment: %w", err)
		}
	}
	if len(theirCommit) != sha256.Size {
		return "", fmt.Errorf("%w: got %d bytes", errShortCommitment, len(theirCommit))
	}

	if initiator {
		if err = writeFrame(conn, mine); err != nil {
			return "", fmt.Errorf("reveal contribution: %w", err)
		}
		if theirs, err = readFrameMax(conn, contributionLen); err != nil {
			return "", fmt.Errorf("receive contribution: %w", err)
		}
	} else {
		if theirs, err = readFrameMax(conn, contributionLen); err != nil {
			return "", fmt.Errorf("receive contribution: %w", err)
		}
		if err = writeFrame(conn, mine); err != nil {
			return "", fmt.Errorf("reveal contribution: %w", err)
		}
	}
	if len(theirs) != contributionLen {
		return "", fmt.Errorf("%w: got %d bytes", errShortReveal, len(theirs))
	}

	// The check the whole design rests on. Constant-time because the comparison is against
	// a value the peer chose and can vary — there is no secret to leak here, but a
	// data-dependent compare on a security check is a habit worth not having.
	sum := sha256.Sum256(theirs)
	if subtle.ConstantTimeCompare(sum[:], theirCommit) != 1 {
		return "", errCommitmentBroken
	}

	return verificationString(conn.ConnectionState(), myFP, peerFP, mine, theirs)
}

// verificationString derives the four words both sides compare.
//
// Inputs, and why each is there:
//
//   - **both identity fingerprints**, so the string is about these two keys;
//   - **both contributions**, so it is fresh per session and — because they were
//     committed — unsearchable;
//   - **the TLS exporter**, which binds the string to THIS channel. D4 asks for "the
//     handshake transcript" and this is what makes that true; it is also what lets D18's
//     clause (a confirmation computed on one channel is rejected on any other) be met
//     without inventing a second mechanism.
//
// Ordering is canonical — the two fingerprints sorted, and the two contributions in the
// same order as the fingerprints they came from — so the two sides, which see the pair
// from opposite viewpoints, hash identical bytes.
func verificationString(cs tls.ConnectionState, myFP, peerFP, mine, theirs []byte) (string, error) {
	// RFC 5705 exporter. A fixed label and no context: the label namespaces this use, and
	// the material is already unique per connection.
	ekm, err := cs.ExportKeyingMaterial("EXPORTER-nib-verification", nil, 32)
	if err != nil {
		return "", fmt.Errorf("channel binding: %w", err)
	}
	return deriveVerification(ekm, myFP, peerFP, mine, theirs)
}

// deriveVerification is the derivation with no I/O in it.
//
// Split out for one reason, and it is a test reason worth stating: through the live
// exchange, the fingerprints and the channel binding are **unobservable**. Every session
// has fresh contributions, so every session's string differs whatever else goes into the
// hash — removing the fingerprints entirely, or the exporter entirely, leaves all four
// end-to-end tests green. That was measured, not assumed.
//
// A pure function can be called twice with one input varied and everything else held, so
// each input can be shown to change the answer. Without that, "D4 says the string binds
// both identity keys plus the transcript" is a claim the suite cannot check.
func deriveVerification(ekm, myFP, peerFP, mine, theirs []byte) (string, error) {
	loFP, hiFP := myFP, peerFP
	loC, hiC := mine, theirs
	if bytes.Compare(myFP, peerFP) > 0 {
		loFP, hiFP = peerFP, myFP
		loC, hiC = theirs, mine
	}

	h := sha256.New()
	h.Write([]byte("nib-verification-v1"))
	h.Write(loFP)
	h.Write(hiFP)
	h.Write(loC)
	h.Write(hiC)
	h.Write(ekm)
	return pairing.Verification(h.Sum(nil))
}
