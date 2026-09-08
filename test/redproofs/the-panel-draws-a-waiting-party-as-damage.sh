# docs/red-proofs.md, tier 2: "the panel draws a waiting party as damage" (/pending 377, v1.128.25)
#
# The defect: `cerWaiting` never fires, which is the panel as it stood. A party who has accepted and
# is waiting for the baton is `absent` on the listing, so the card was badged **"Nothing on disk"**,
# given the peach border every non-ok state gets, and had its sentence drawn in peach. Nothing was
# wrong: the document had not reached their hop yet, which is where most invitees are most of the
# time.
TIER="tier 2 — jsdom"
PROVE="node --test test/jsdom/prehopcard.test.mjs"
EXPECT="badged \"Nothing on disk\""
