# docs/red-proofs.md, tier 1: "A bare touch is reported as a change" (/pending 333, v1.117.289)
#
# The defect: diskChanged answers on identity/size/mtime alone and never compares content. A file
# whose mtime moved but whose bytes are identical — `touch`, a backup restored over itself, a sync
# client rewriting the same bytes — is then reported as changed.
#
# It is recorded because the failure is a FALSE STATEMENT, not a missed detection: the banner
# says "This file has changed on disk", which is untrue of identical bytes, and the save is
# refused for a change that did not happen. A detector that answers "changed" too eagerly
# produces a banner over every document, which is the same outcome as no banner at all.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestUntouchedFileIsNeverReportedChanged"
EXPECT="bytes are identical is reported as changed"
