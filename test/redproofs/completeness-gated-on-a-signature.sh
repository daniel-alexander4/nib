# docs/red-proofs.md, tier 1: "the completeness count is gated on the document being signed"
# (P07.S05a, v1.117.178)
#
# The defect: `/api/attestations` resolves the document's proceeding only when the document
# already carries a signature — so a **convened but unsigned** document, which is C18's own
# extreme case (two parties obliged, nobody has signed), reports no obliged count at all and
# the completeness sentence has nothing to render. Measured at tier 6 before the fix: the
# route answered `{"attestations":[]}`.
#
# **The patch restates the defect in symbols that still exist.** The shipped gate was
# `p2p.ClaimsAProceeding(doc.sig)` — *does any signature name a ceremony* — which was deleted
# with the fix, so a patch restoring it by name would fail to BUILD rather than fail its
# check, and a build error is red for the wrong reason. `len(doc.sig.Signers) > 0` is the
# same gate in its consequence: it excludes exactly the unsigned document, which is the whole
# defect.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestAConvenedDocumentReportsItsObligedSignersBeforeAnyoneHasSigned -count=1"
EXPECT="the route reports no roster for a convened document"
