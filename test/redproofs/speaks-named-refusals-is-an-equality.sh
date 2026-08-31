# docs/red-proofs.md, tier 1: "a newer ALPN is denied the capability it introduced" (/pending 338, v1.117.290)
#
# The defect as SHIPPED until P08.S05a: SpeaksNamedRefusals was `c.Proto == alpn2`. A third
# version added to sessionALPN negotiates fine and then reports FALSE — a silent downgrade of the
# NEWEST peers, in the one direction nothing looked. The older-peer guard would have blessed it,
# because it enumerates protocols that must not speak and every one of them is older.
#
# The check prepends a future version to the offer list and requires it to speak.
TIER="tier 1 — go test"
PROVE="go test ./internal/p2p/ -run TestSpeaksNamedRefusalsIsAFloorNotAnEquality -count=1"
EXPECT="was told it cannot read a named refusal"
