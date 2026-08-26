# docs/red-proofs.md, tier 1: "one proceeding means the signers agree" (P07.S04, v1.117.169)
#
# The defect: `markOneProceeding` compares the commitments **only to each other** — the state the
# product was in until this slice.
#
# Measured at the slice's grill: two parties signing with the same arbitrary `abab…` value they
# chose themselves, on a document carrying **no ceremony record at all**, report
# `oneProceeding: true` on every signature — and `web/app.js` renders that as *"✓ One proceeding —
# every signature on this document commits to the same ceremony."* The token lives inside the
# signed `/Reason`, so it is a value the SIGNER picks; agreement among signers is a fact about what
# they wrote, not evidence that the ceremony they name exists.
#
# **Latent only because nothing populated the token, and C01 is the change that populates it** —
# which is why this was a precondition of C01 rather than an improvement beside it.
TIER="tier 1 — go test"
PROVE="go test ./internal/ceremony/ -run TestAgreementAmongSignersIsNotAProceeding -count=1"
EXPECT="is reported as part of one proceeding"
