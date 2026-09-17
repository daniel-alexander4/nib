# docs/red-proofs.md, tier 1: "An /Annot element's OBJR follows the annotation it names" (ADR-045)
#
# The defect: `repointOBJRs` returns without repointing anything. An OBJR is reachable from
# `/StructTreeRoot` and pdfcpu writes by reachability, so one left naming the SOURCE annotation
# keeps that annotation in the file — on no page, claiming the same `/ParentTree` key as the copy.
#
# Nothing else can see it: `parentTreeOwners` walks the annotations of PAGES, so the second claimant
# is invisible and `structureCarriedCompletely` scores both shapes identically. Measured:
# `OBJR -> obj 15 onPage=0` beside a fresh copy on sheet 1 the tree described nothing about.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -run TestACarriedNotesAnnotElementNamesTheAnnotationOnTheSheet -count=1"
EXPECT="which is on no page"
