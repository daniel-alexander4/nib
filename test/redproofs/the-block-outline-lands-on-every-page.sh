# docs/red-proofs.md, tier 2: "the block outline lands on every page" (P02.S03, v1.128.8)
#
# The defect: the `block.page === i` match is dropped, so the outline is drawn on whichever page
# renders — telling a signer their signature lands on page 1 of a document where it lands on page 2.
#
# **This row is here because the mutation was GREEN first, and the fixture was why.** The test used
# a one-page document with the block on page 1, so the condition removed was always true and the
# file stayed green against a branch it could not reach. Two pages with the block on page 2 now.
# Same class as P02.S02's M5: an assertion that cannot reach the branch it names reads exactly like
# one that passes.
TIER="tier 2 — jsdom"
PROVE="node --test test/jsdom/consentblock.test.mjs"
EXPECT="the block was drawn on the wrong page"
