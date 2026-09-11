# docs/red-proofs.md, tier 1: "an authoring call site stops reaching the title door"
# (PLAN-accessibility.md P03.S01, v1.129.30)
#
# The defect: `handleOffice` converts a Word document and hands the result back without a
# title. Nothing else notices — the conversion succeeds, the document opens, and it fails
# PDF/UA 7.1 t8, t9 and t10 silently, because a missing catalog key is not an error anywhere.
#
# This is the shape ADR-009 exists for: the rule holds at SIX call sites and is written once,
# so the guard must assert the routing rather than checking that five sites agree. Five sites
# agreeing says nothing about a sixth added without one — and the sixth here was real, found
# while building the slice (`handleAssemble`, which has no file name of its own).
TIER="tier 1 — go test"
PROVE="go test . -run TestEveryAuthoredDocumentGetsATitle"
EXPECT="authored document with no title"
