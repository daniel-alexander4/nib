# docs/red-proofs.md, tier 1: "no network cause says the same thing as an end state" (P06.S08, D19,
# D28, v1.117.347)
#
# **The criterion's own example of failure**, reproduced: a screen that folds "they declined" into
# "couldn't establish a connection". The mutation renames one D19 summary to an end-state word.
#
# The check has to live in Go, because the two halves of the set are in two languages: D28's words
# are rendered by `renderEndedCeremonies` in web/app.js and D19's summaries are built by
# `classifyD19`. A jsdom test can only compare sentences it was handed, which is a fact about its
# own fixture. This one drives the classifier and reads the other side out of the file that owns it.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestEveryD19OutcomeSaysItsOwnThing -count=1"
EXPECT="is also the D19 summary for"
