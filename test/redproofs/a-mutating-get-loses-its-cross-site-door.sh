# docs/red-proofs.md, tier 1: "a GET that mutates is behind requirePublicLoopback"
# (/pending 365, ADR-009, v1.117.343)
#
# This is the P08.S06 defect reintroduced exactly: `GET /api/ceremonies` runs `closeOutEnded`,
# which MOVES ceremony directories and drops vault pins, and the patch puts it back behind
# `requireUnlocked` — a gate that applies CSRF and the origin check to non-GET methods only, so
# any page in the user's browser can fire it with an <img src>. P06.S01 fixed the live instance;
# this proof is what keeps the class empty.
TIER="tier 1 — go test"
PROVE="go test ./ -run TestEveryMutatingGETIsBehindTheLoopbackDoor -count=1"
EXPECT="answers GET /api/ceremonies behind requireUnlocked and calls closeOutEnded"
