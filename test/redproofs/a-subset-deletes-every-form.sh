# docs/red-proofs.md, tier 1: "a subset deleted every AcroForm" (PLAN-ua-coverage.md P02.S04a)
#
# Moving page selection into the source context made nib responsible for the AcroForm prune that
# pdfcpu's `migrateFields` used to rebuild for free. The first cut collected the surviving widgets
# by rebuilding each kept page's reference with `types.NewIndirectRef` — which returns a POINTER,
# while `DereferenceDict` switches on `case IndirectRef`. Every page dereferenced to nil, the
# surviving-widget set was always empty, no field ever survived, and every subset deleted the whole
# `/AcroForm`: a form document that went through reorder, delete or extract came back with its
# fields gone.
#
# The entire suite was green over it. Every form fixture in internal/pdfops is a single page, so no
# test had ever subset a form — the defect was found by mutating a DIFFERENT guard (the signature
# drop) and asking why the mutation survived.
#
# The guard is parity against the previous implementation rather than a hand-written expectation:
# the test runs `api.Collect` beside the new door on the same fixture and compares the field names
# each keeps, so "which fields should survive" is answered by the code this replaced.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -run TestASubsetKeepsTheFormFieldsWhoseWidgetsSurvive -count=1"
EXPECT="(what the previous implementation kept)"
