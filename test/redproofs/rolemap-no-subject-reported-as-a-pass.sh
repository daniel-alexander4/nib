# docs/red-proofs.md, tier 1: "rolemap no subject reported as a pass" (/pending 548, 2026-09-16)
#
# The defect: a document with no structure elements answers `Pass` rather than `NotApplicable`.
# Both are conformant verdicts, so nothing downstream changes — which is the point: law 4 keeps the
# two apart because `Pass` claims the rule was evaluated against a subject, and veraPDF reports 0
# passed and 0 failed checks over such a file. The oracle's strict mapping is what would catch it in
# the field; this row is the cheap version.
TIER="tier 1 — go test"
PROVE="go test ./internal/uacheck/ -run TestADocumentWithNoStructureElementsHasNoSubjectFor7_1t6 -count=1"
EXPECT="want NotApplicable"
