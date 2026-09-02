# docs/red-proofs.md, tier 1: "a re-issue mints fresh secrets" (P08.S07, v1.117.246)
#
# The defect: `handleCeremonyInvites` generates a new secret per party instead of reading the stored
# one back, so every re-issued invitation is different from the one convene handed out — and every
# party still holding theirs is holding a stale one that no longer keys the rendezvous, the record
# encryption or the channel binding. A convener re-issuing for ONE party who lost an email would
# silently invalidate the whole roster's copies.
#
# **This row exists because the test it proves is otherwise true by construction.** The route reads
# the secrets back from the vault, so "every other party's state is untouched" passes against any
# behaviour at all. Without a patch that makes it fail, the assertion is a restatement of "nothing
# changed" in a function that changes nothing — which is the vacuous green this repo keeps paying for.
#
# **RE-SITED at P08.S05g (2026-09-01).** Same move as `invites-omit-the-recital`: the secret is
# read once, at `convenerInvitationFor`, by both the re-issue route and the delivery round. A
# freshly minted secret there now breaks BOTH — which is the point of collapsing the two copies.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestAnInvitationReIssuedMidCeremonyLeavesEveryoneElseUntouched -count=1"
EXPECT="differs from the one convene issued"
