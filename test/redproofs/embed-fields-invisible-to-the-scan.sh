# docs/red-proofs.md, tier 2: "The published-shape scan cannot see an embed" (sweep 10, v1.117.45)
#
# The defect: `fieldsOf` returns only the json tags in a struct's OWN body, so a type reached
# through an embed contributes nothing. `attestationView` embeds `p2p.SignerAttestation`,
# publishes ten fields, and the scan checked one (`pinned`).
#
# That is how `oneProceeding` — computed, serialized, and at the time rendered nowhere —
# survived the very scan built to catch a published-and-never-read field, and had to be found
# on the Go side by observables_test.go instead. The two scans were complementary by accident.
#
# The check that fires is the SETUP assertion, and deliberately so: it names a field reachable
# ONLY through the embed, because asserting on `pinned` would pass with embed resolution gone.
TIER="tier 2 — the jsdom suite"
PROVE="node --test test/jsdom/published.test.mjs"
EXPECT="oneProceeding is missing"
