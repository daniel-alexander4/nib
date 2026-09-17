# docs/red-proofs.md, tier 2: "a change after a failed peer fetch does not overwrite the saved
# roster" (/pending 566)
#
# The destructive half, and the one that reaches disk. `saveCeremonyDraft` reads the roster out of
# `#cerPeerPick`; with the picker unfilled it reads zero rows and POSTs `roster: []`, and
# `handleCeremonyDraft` REPLACES the stored blob. So a roster the convener picked in an earlier
# session is gone from the vault, permanently, for every later open — from one failed fetch and one
# blur.
#
# **The check asserts the POST BODY, not the DOM.** "The roster was lost" is not observable from the
# form: the picker looks the same either way, and the loss only appears on a later open, in another
# session. The only moment it is visible is the bytes going out, which is why this row's driver
# records them at the route rather than reading `boot`'s call log — that log carries the URL, the
# method and the headers and not the body.
#
# **The patch deletes the guard and nothing else.** Note the shape it restores is NOT the one the
# finding proposed as a fix: omitting the `roster` field instead of emptying it destroys the roster
# identically, because the write replaces the whole blob and the restore reads a missing array and
# an empty one the same way. There is no partial save here; the only write that keeps the roster is
# no write.
TIER="tier 2 — jsdom"
PROVE="node --test test/jsdom/pickerunread.test.mjs"
EXPECT='posted a draft carrying `roster: []`'
