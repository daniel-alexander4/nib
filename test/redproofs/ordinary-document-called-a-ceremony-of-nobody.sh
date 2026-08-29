# docs/red-proofs.md, tier 1: "A ceremony of nobody on every ordinary co-sign" (P07.S10, v1.117.231)
#
# The defect: the no-record case stops being treated as "this document has no ceremony", so a
# document that never belonged to a proceeding is described as one with zero parties.
#
# The control the slice's other two rows would not catch, and the one a naive fix produces. Most
# documents Nib signs belong to no ceremony: "0 of 0 obliged signers have signed" would appear on
# every ordinary co-sign in the product, and it is a verdict on a proceeding that does not exist.
#
# It is the same three-state discipline the signature panel uses one layer up — a ceremony that
# disagrees, a ceremony that agrees, and NO ceremony are three answers, and collapsing the third
# into either of the others is how a verifier starts describing things that are not there.
TIER="tier 1 — go test"
PROVE="go test ./internal/cli/ -run TestVerifyOnAnOrdinaryDocumentSaysNothingAboutCeremonies"
EXPECT="was described as having one"
