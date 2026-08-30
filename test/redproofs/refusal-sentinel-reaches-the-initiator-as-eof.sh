# docs/red-proofs.md, tier 1: "a refusal sentinel reaches the initiator as bare EOF"
# (/pending 315, v1.117.257)
#
# The defect this REINTRODUCES is the one the guard found live on HEAD. `ErrNoSignaturePages` loses
# its wire code, so `refusalAck` returns (nil, false), `Receive` writes no frame, and the
# initiator's `readFrame` gets EOF — surfaced as
# `502 co-signing did not complete: receive co-signed document: EOF`, a network fault inviting the
# retry a refusal must not invite. That is P07.S03a's defect, which had survived in three sentinels
# nobody had enumerated because both "every code" tests were hand-written lists.
#
# Three assertions fire, and that is the point of the sentinel-coverage arm: the code becomes
# un-emittable, the live round trip stops closing, and the sentinel is named as having neither a
# code nor a "No wire code:" line saying why.
TIER="tier 1 — go test"
PROVE="go test ./internal/p2p/ -run TestTheRefusalEnumerationIsDerivedFromSource -count=1"
EXPECT="has no wire code, is not named in refusalAck"
