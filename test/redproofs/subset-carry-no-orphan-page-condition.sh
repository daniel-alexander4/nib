# docs/red-proofs.md, tier 1: "carryIsComplete without the orphan-page condition" (P02.S04b, v1.129.142)
#
# The defect: the gate asks only whether the TREE contradicts itself, not whether a page the operation
# removed is still in the FILE. pdfcpu writes by reachability, so every re-anchoring defect this
# package has had was something that still NAMED a dropped page — a field's `/Kids`, a link's
# destination, a surviving named destination, an element's `/Pg`, an `/IDTree` entry, an OBJR's
# `/Obj` — each found and fixed separately. This condition asks what all six answer.
#
# Measured before its reader existed: removing the conjunct left the whole `internal/pdfops` suite
# green, because every assertion that touched it asserted ZERO and nothing drove it non-empty. The
# stimulus is a kept page's annotation whose `/P` names a dropped page: nothing prunes an annotation's
# `/P`, so the dropped page reaches the output while the tree stays clean.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -run TestTheOrphanPageConditionDECIDESTheGate -count=1"
EXPECT="claims tagging"
