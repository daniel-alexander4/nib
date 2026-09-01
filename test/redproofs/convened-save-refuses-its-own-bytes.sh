# docs/red-proofs.md, tier 1: "a convened document cannot be saved to its own file" (/pending 341, v1.117.304)
#
# The byte-identity exemption is removed, restoring the shipped behaviour: saving a convened document
# to its own original path answers 409 **even with no divergence at all**.
#
# What that costs is not the save. The commit doors mutate memory only, so a convened document's
# bytes reach disk at exactly one place — `~/nib/ceremonies/<id>/document.pdf` — and the file in the
# user's own matter folder stays the PRE-CEREMONY draft forever, unsigned and carrying no record.
# `nib verify` on that file reports "unsigned" about a document under a live ceremony.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestAConvenedDocumentCanBeBroughtIntoLineWithItsOwnFile -count=1"
EXPECT="answered 409, want 200"
