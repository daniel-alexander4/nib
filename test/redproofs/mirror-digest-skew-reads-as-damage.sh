# docs/red-proofs.md, tier 1: "a mirror under another digest rule reads as damage" (/pending 725, v1.169.14)
#
# The defect: ReadMirror does not ask digestRuleSkew, so a mirror stored under an older ContentDigest
# rule falls through to the hash comparison and is reported damaged — an accusation made by a point release.
TIER="tier 1 — go test"
PROVE="go test ./internal/ceremony/ -run '^TestAStoredMirrorUnderAnotherDigestRuleIsASkewNotDamage$' -count=1"
EXPECT="a mirror written under digest rule 3 reads as damaged"
