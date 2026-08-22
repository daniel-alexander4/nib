# docs/red-proofs.md, tier 1: "A timeout collapsed back into a decline" (P05 sweep, v1.117.38)
#
# The defect: the refusal→byte door mapped ErrConsentTimedOut to ackDeclined, so a consent
# request nobody answered reached the peer as a decline.
#
# What it costs: a false statement about a person's decision, sent to another machine and
# shown to its user. They are told somebody read the document and refused it when in fact
# nobody was there. `verify.go` draws exactly this distinction one gate earlier and writes
# down why: "nobody answered. Distinct from declining, because it means something different
# to the user and to whoever reads the log."
TIER="tier 1 — go test"
PROVE="go test ./internal/p2p/ -run TestARefusalTellsThePeerWHICHRefusalItWas"
EXPECT="want nobody answered the consent request in time"
