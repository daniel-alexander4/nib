# docs/red-proofs.md, tier 1: "a committed picture is never bracketed" (/pending 514, v1.133.11)
#
# The defect: the commit writer brackets what `uncoveredDrawingSpans` returns, and that reader can only
# tell a picture from a form if the page's image names are handed to it. Passing none is the shape
# `/pending 495` shipped — uncovered TEXT and uncovered PATHS artifacted in this very loop, pictures
# left alone — and it is ADR-009's "a rule reaching some sites and not others" inside a single `for`.
#
# **The failure it produces is not the one it looks like.** A committed page with an unbracketed
# picture does not come back wrong; it does not come back at all. `claimTagging` now counts the
# drawings, so `commitProposal` hits its own `orphanedClaimError` and refuses — which is the honest
# outcome for a door that cannot describe what it draws, and the reason the guard and the writer are
# separate halves rather than one.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -run TestACommitDeclaresAPictureNoElementCoversAnArtifact -count=1"
EXPECT="refusing to return it"
