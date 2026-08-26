# docs/red-proofs.md, tier 1: "the convened document is never actually built" (P07.S02a, v1.117.155)
#
# The defect: `PrepareCeremonyDocument` becomes `return pdf, nil` — the readme, the ceremony page
# and the signature pages are never rendered or appended, and the door reports success.
#
# **This row is a guard's red proof, not a feature's.** Measured at the slice's diff review: with
# this exact mutation applied, every convene test in the package stayed GREEN. `CheckDocument`
# proves hash-then-embed and cannot see whether a page was ever appended, and `Convene`'s own doc
# comment refuses dependency injection precisely so that "the guard would not go green with the
# readme never appended" — and it did. `TestTheConvenedDocumentIsBUILT` is what closed it, and the
# assertion is written against `SignaturePagesFor(signing)` rather than a literal, with a setup
# assertion that the signer count and the roster length straddle a page boundary — an earlier
# fixture used seven signers of eight parties, which both round to two pages, so the assertion was
# correct and could not fire.
TIER="tier 1 — go test"
PROVE="go test ./internal/ceremony/ -run TestTheConvenedDocumentIsBUILT -count=1"
EXPECT="pages are allocated from the SIGNING count"
