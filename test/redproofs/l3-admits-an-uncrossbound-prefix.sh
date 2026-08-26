# docs/red-proofs.md, tier 1: "L3 stops requiring the prefix to be cross-bound" (P07.S03, v1.117.159)
#
# The defect: the gate compares identities and stops asking whether each prefix signature attests
# to a real, valid co-signer on the same document. L3 and D23 both say "each one valid and
# cross-bound"; a check that only compared identities would satisfy the sentence while missing
# half of it — a signature accepting somebody who never signed attests to nothing.
#
# The fixture isolates exactly that: A signs accepting a stranger who never signs, B signs on top
# so A is no longer the last signature and its cross-binding is due, and the identities ARE the
# roster prefix — asserted before the refusal is graded, so a red here cannot be about order.
#
# **The exemption INVERTED at P07.S05 and this row inverted with it.** The rule was "all but the
# LAST", written while `AcceptedPeer` named the wire peer — a signer's successor in a two-party
# exchange. D22's amendment points it at the PREDECESSOR, so every signature after the first
# attests to a party who has already signed and the first accepts nobody. Caught by a four-party
# carry ceremony failing at hop 2 with *"signature 1 attests to a peer who is not a valid signer"*.
#
# A second row for the same mutation (`carry-relays-a-hostile-hop`) was written at S05 and deleted:
# once the exemption inverted, the two patches were byte-for-byte identical, and two rows for one
# mutation is coverage that reads as two and is one.
TIER="tier 1 — go test"
PROVE="go test ./internal/p2p/ -run TestTheGateRefusesEachThingByName -count=1"
EXPECT="prefix not cross-bound: admitted"
