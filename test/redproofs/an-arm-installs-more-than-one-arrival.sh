# docs/red-proofs.md, tier 1: "one arm installs more than one arrival" (/pending 337, v1.117.303)
#
# The `!opened` guard is dropped from one `openArrival` call site, so a single arm can install a
# document per re-delivery instead of one per arm.
#
# **It is the property `addDoc`'s arrival exemption now rests on.** That door is uncapped — past
# BOTH of ADR-005's bounds — and until /pending 337 the reason given was that the bytes "have no
# other home, since the session path installs it and nothing writes it to disk". P08.S02 made that
# half false: a ceremony arrival's bytes go to the mirror before the frame. What actually carries
# the exemption is structural — an arm admits at most ONE document and only one the user accepted
# at the consent gate — so the door cannot be pumped by a peer. This row is that clause, asserted.
#
# Nothing else in the tree fails: re-delivery is idempotent, so the second install is the same bytes
# and every behavioural assertion about the result still holds. What changes is only how many times
# it lands.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestAnArrivalCannotBePumped -count=1"
EXPECT="is not guarded by \`!opened\`"
