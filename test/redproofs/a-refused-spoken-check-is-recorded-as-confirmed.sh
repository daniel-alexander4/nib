# docs/red-proofs.md, tier 1: "a refused spoken check is recorded as confirmed" (P02.S04, v1.128.12)
#
# The defect: the answer is discarded and every presented check records `confirmed: true` — so a
# user who said the four words did NOT match leaves a durable local claim that they vouched for
# that identity.
#
# **This mutation SURVIVED the first probe, and the survival is why the row exists.** Every Go test
# in the slice called `noteVerification` with its own arguments, and the exit-population scan
# counts CALLS rather than what is passed to them — so the whole file was green against it.
# `TestTheAnswerRecordedIsTheAnswerGiven` drives `ConfirmVerification` itself and answers it
# through `respondVerify`, which is the only way to see the call site at all.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheAnswerRecordedIsTheAnswerGiven -count=1"
EXPECT="the note records confirmed"
