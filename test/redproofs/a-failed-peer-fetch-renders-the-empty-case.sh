# docs/red-proofs.md, tier 2: "a failed peer fetch does not claim the user has paired with nobody"
# (/pending 566)
#
# The defect is a shared branch. `loadPeerPicker`'s failure leaves `peers = []`, and an empty array
# is indistinguishable from an answer of none — so the sheet renders *"You have not paired with
# anyone yet"* over a fetch that never arrived. That is a false statement about the user's own data,
# and it is also what hides the second defect: a convener told they have no peers has no reason to
# suspect the roster in their vault is one keystroke from being overwritten.
#
# **The patch removes the failure branch and nothing else**, which is exactly the shape a later
# author reaches for when the two cases look like one case — it is how the code read before this
# row existed.
TIER="tier 2 — jsdom"
PROVE="node --test test/jsdom/pickerunread.test.mjs"
EXPECT="rendered the empty-peer-list sentence"
