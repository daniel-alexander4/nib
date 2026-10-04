# docs/red-proofs.md, tier 1: "7.18.1 t1's exclusion list loses its Widget arm (a P05.S01 survivor until the review added the fixture)"
# (P05.S01, v1.149.0)
#
# Recorded at /pending 637/700 from a probe that was run by hand when the fix landed and never
# written down as a replayable row.
TIER="tier 1 — go test"
PROVE="go test ./internal/uacheck/ -run TestTheAnnotationRulesAgreeWithWhatVeraPDFMeasured -count=1"
EXPECT="a visible widget with no /StructParent, excluded from t1: 7.18.1 t1 = fail"
