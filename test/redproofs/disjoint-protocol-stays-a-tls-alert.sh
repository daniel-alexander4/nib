# docs/red-proofs.md, tier 1: "A version difference reported as a network failure" (P07.S09c, D32, v1.117.227)
#
# The defect: `asProtocolSkew` stops recognising the ALPN alert, so a peer speaking no protocol
# version in common comes back as `remote error: tls: no application protocol`.
#
# It arrives on the connect path, where every other failure means the peer is unreachable — so the
# honest reading of it is about the network, and the true cause is that somebody needs to update
# Nib. D32: a skew produces a sentence naming the mismatch, not a parse error, and an alert IS the
# parse error one layer down.
#
# **The guard drives a REAL disjoint handshake** — a listener offering `nib/99` — rather than
# asserting the substring the classifier matches on. That substring is somebody else's error text,
# so a test asserting the constant would be checking it against itself; this one goes red if the
# stdlib ever rewords the alert, which is the only way this classifier can rot.
TIER="tier 1 — go test"
PROVE="go test ./internal/p2p/ -run TestADisjointProtocolIsASentenceNotATLSAlert"
EXPECT="a raw TLS alert on the connect path"
