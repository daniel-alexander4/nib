# docs/red-proofs.md, tier 1: "an unrelated edit stales a red proof" (/pending 348, v1.117.306)
#
# The defect is a COMMENT — one line, inside a function whose behaviour is unchanged, added by a
# slice with no idea a recorded patch reads those lines as context. `label-never-overrides-the-constant`
# then stops applying, and the ledger silently claims a coverage it can no longer demonstrate.
#
# **This is not hypothetical and it is not rare: it is exactly how that row went stale.** A
# `/pending 317` paragraph was inserted into the same comment block, the code beneath it never
# changed, and the row was dead from that commit until a full `--all` found it weeks later. A full
# replay costs ~24 minutes and nothing ran it; this scan costs ~0.24 seconds and runs every time.
#
# The scan reads the WORKING TREE rather than an export of HEAD, which is where it is stricter than
# `redproof.sh`: it fails on the edit that stales the row, before the commit, when the fix is a
# context refresh rather than archaeology.
TIER="tier 1 — go test"
PROVE="go test ./ -run TestEveryRedProofStillApplies -count=1"
EXPECT="no longer applies to this tree"
