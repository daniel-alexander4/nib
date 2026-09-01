# docs/red-proofs.md, tier 2: "a waiting arm says nothing" (/pending 349, v1.117.309)
#
# `reflectDiagnosis` stops being called from the status poll — the state the product shipped in from
# P05.S11 until 2026-09-01. The server still computes the diagnosis and still publishes it; nothing
# renders it.
#
# **The field's own doc names exactly what that costs**: it exists "so the polling UI shows why
# nothing has connected yet, RATHER THAN A BLANK WAIT", and the blank wait is what the user got for
# the whole life of the feature. A named search found `diagnosis` in web/app.js once, inside a
# comment.
#
# Nothing else fails under this patch: the arm still works, the poll still promotes to consent, the
# pill still counts down. Only the sentence is gone — which is why it went unnoticed.
TIER="tier 2 — jsdom"
PROVE="./build/jsdomtest.sh"
EXPECT="the blank wait is what the user got"
