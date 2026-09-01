# docs/red-proofs.md, tier 1: "the freeze is bypassed for every save" (/pending 341, v1.117.304)
#
# **The other direction, and the one that makes the exemption a narrowing rather than a deletion.**
# The condition is inverted, so `ceremonyFreeze` never runs on the save route at all and ALTERED
# bytes overwrite a convened document.
#
# A test that only checked the exemption would pass against this. The rule is byte-identity: writing
# bytes equal to the document under ceremony cannot change what anyone was invited to sign, and
# writing anything else is precisely the act the freeze exists to refuse — the other parties' copies
# stop matching.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestAConvenedDocumentCanBeBroughtIntoLineWithItsOwnFile -count=1"
EXPECT="answered 200, want 409"
