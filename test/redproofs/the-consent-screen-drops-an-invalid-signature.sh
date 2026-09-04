# docs/red-proofs.md, tier 2: "an invalid signature is listed and marked, never dropped" (P06.S07,
# D27, v1.117.346)
#
# The other direction, and it needs its own row because it fails a different assertion. Filtering
# the broken signature out makes the list shorter and the document look CLEANER than it is, on the
# one screen where a user decides whether to add their own name to it.
TIER="tier 2 — jsdom"
PROVE="node --test test/jsdom/consentroster.test.mjs"
EXPECT="signer row(s) for a document carrying three signatures"
