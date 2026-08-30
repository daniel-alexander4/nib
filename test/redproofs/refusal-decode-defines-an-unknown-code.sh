# docs/red-proofs.md, tier 1: "errorForCode decodes a code the enumeration does not define"
# (/pending 315, v1.117.257)
#
# The defect: a `case 42:` is added to `errorForCode` with no matching constant. Nothing can ever
# emit 42, so this build decodes a value its own enumeration does not define — a decode-only code,
# reserved against a meaning no build produces.
#
# It is also the proof that caught a hole in the guard itself. The first draft's `caseSubject`
# returned "" for anything that was not an identifier, so a BARE LITERAL case was invisible and
# this proof passed GREEN. The literal arm exists because of this row.
TIER="tier 1 — go test"
PROVE="go test ./internal/p2p/ -run TestTheRefusalEnumerationIsDerivedFromSource -count=1"
EXPECT="is not a member of the refusal-code const block"
