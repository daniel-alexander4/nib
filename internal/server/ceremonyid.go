package server

import (
	"encoding/hex"
	"errors"
	"fmt"

	"nib/internal/ceremony"
	"nib/internal/sign"
)

// ceremonyID is the ceremony identity an armed session carries: which proceeding this is,
// which hop of it, and the material every rendezvous derivation needs.
//
// **This is the import that did not exist.** Until P05.S04 `internal/server` imported neither
// `internal/ceremony` nor `internal/rendezvous`, so the whole of P04's output was reachable
// only from `nib rendezvous --self-test`. An arm knew a fingerprint, a bind, a mode and a
// transport, and nothing about the proceeding it was part of.
//
// It is OPTIONAL on the arm. A session without one is the manual and LAN path this route has
// always served, which D9 demotes rather than deletes.
type ceremonyID struct {
	inv  ceremony.Invitation
	hop  int
	gate *ceremony.CandidateGate
	// me and peer are the two ends of this hop, hex. Kept because the gate holds them
	// privately and the publish side needs its own.
	me, peer string
}

// errNoCeremony reports an arm with no invitation — not an error, the ordinary case.
var errNoCeremony = errors.New("this session has no ceremony identity")

// ceremonyFor builds the identity from an invitation and the two parties.
//
// The hop is DERIVED, never supplied: it is the roster's own order (D22 makes connectivity a
// sequence of pairs, and Party's doc says the roster order is the signing order), read off an
// artifact both ends already hold. A hop passed in a request would be a number the two sides
// have to agree on, and there is nothing to make them.
func ceremonyFor(text string, myCertPEM []byte, peerFP []byte) (*ceremonyID, error) {
	if text == "" {
		return nil, errNoCeremony
	}
	inv, err := ceremony.ParseInvitation(text)
	if err != nil {
		return nil, fmt.Errorf("that invitation could not be read: %w", err)
	}
	myFP, err := sign.Fingerprint(myCertPEM)
	if err != nil {
		return nil, err
	}
	me := hex.EncodeToString(myFP)
	peer := hex.EncodeToString(peerFP)

	hop, err := inv.Hop(me, peer)
	if err != nil {
		// The three ways this fails are all worth distinguishing for a user, and `Hop`'s
		// own error already does: not in the roster at all, the same party twice, or two
		// parties who are not adjacent and therefore share no hop.
		return nil, fmt.Errorf("this invitation does not put you and that peer on one hop: %w", err)
	}
	gate, err := ceremony.NewCandidateGate(inv, hop, me, peer)
	if err != nil {
		return nil, err
	}
	return &ceremonyID{inv: inv, hop: hop, gate: gate, me: me, peer: peer}, nil
}
