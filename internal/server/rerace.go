package server

import "nib/internal/p2p"

// isTransportLoss reports whether err is a lost CHANNEL to re-race (P05.S10) rather than a decided
// outcome that ends the ceremony. It delegates to p2p, which owns what a dropped session looks like
// on the wire (QUIC/net errors); a decided outcome — decline, consent timeout, the MITM commitment
// signal — is not a transport error and so terminates. WHITELIST, not blacklist (grill P1).
func isTransportLoss(err error) bool { return p2p.IsTransportLoss(err) }
