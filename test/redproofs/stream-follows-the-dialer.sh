# docs/red-proofs.md, tier 1: "The QUIC stream could open by the dialer, not the role" (P05.S09a, v1.117.64)
#
# The defect: HandshakedConn.Promote opens or accepts the session stream by who DIALLED rather
# than by the ceremony ROLE. Pre-S09a the two coincided — the dialer was always the initiator —
# so it was welded that way and never tested apart. Under symmetric racing (S09) they diverge: a
# dial-won RECEIVER would open a stream it then only reads, while the accept-won INITIATOR sits on
# AcceptStream waiting for a first frame the receiver's role never sends. That is the glare
# deadlock, invisible on TCP's full duplex and fatal on QUIC. The patch inverts Promote's role key
# so the receiver opens and the initiator accepts — exactly the welded arrangement — and the
# role-opposite-dialer harness never completes.
TIER="tier 1 — go test"
PROVE="go test ./internal/p2p/ -run TestQUICStreamOpensByRoleNotByDialer -count=1 -timeout 60s"
EXPECT="stream direction is following the dialer"
