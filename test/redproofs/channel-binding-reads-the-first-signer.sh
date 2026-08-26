# docs/red-proofs.md, tier 1: "the channel binding reads the FIRST signer" (P07.S03, v1.117.160)
#
# The defect: `coSignExchange` binds the connection against `ats[0]` again — the state it was in
# before P07.S03, and correct only while the single-prior-signer rule held.
#
# Conditioning that rule for ceremonies is what made this reachable: at hop k the document carries
# k attestations and `ats[0]` is the party who signed FIRST, not the one on the other end of this
# connection. So the two bindings — "the document was signed by the connected peer" and "that
# signer accepted you" — would be asked about the wrong party at every hop past the first: an
# honest peer refused, and a peer whose only qualification is that the FIRST signer happened to be
# the pinned identity let through.
#
# Driven at THREE signatures, because at N=2 index 0 and index last are the same attestation and
# every other test in the package stays green either way. The fixture asserts first != last before
# it grades anything.
TIER="tier 1 — go test"
PROVE="go test ./internal/p2p/ -run TestTheChannelBindingReadsTheLastSigner -count=1"
EXPECT="The channel binding is reading the first attestation"
