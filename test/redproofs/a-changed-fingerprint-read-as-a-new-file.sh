# docs/red-proofs.md, tier 1: "nib watch treats a changed fingerprint as a new file" (/pending 508,
# v1.133.10)
#
# The defect: `scanOnce` drops a path from `processed` as soon as its size or mtime differs from
# the one last seen. That is the tempting way to catch a file COPIED over the old one — which never
# leaves the directory, so `/pending 504`'s prune-on-absence cannot see it — and it is refused: a
# changed fingerprint cannot be told apart from the user editing a document that happens to be
# sitting in the watched folder, which is the unrequested in-place rewrite the startup rule exists
# to prevent and which `--do sanitize` makes irreversible.
#
# The row records a REFUSAL rather than a fix, which is why it is worth having: without it the only
# evidence that this was decided is a paragraph of prose.
TIER="tier 1 — go test"
PROVE="go test ./internal/cli -run TestAFileReplacedInPlaceIsNotTreatedAsNew"
EXPECT="after its bytes changed"
