# docs/red-proofs.md, tier 1: "the claim door counts only text" (/pending 514, v1.133.11)
#
# The defect, verbatim as `/pending 495` shipped it: `claimTagging` refuses a NEW claim of tagging
# while any TEXT RUN is undescribed, and asks nothing about anything else the page draws. Its own
# comment named the residue — *"refusing on them would switch those doors off rather than fix them"* —
# and the site that residue reaches is a form authored onto a scanned, image-only page: it draws no
# glyph at all, so the text guard answers 0 and `AuthorTaggedForm` writes `/Marked true` over a
# picture no element covers.
#
# The form door must NOT artifact that picture, and that is why the guard has to be the one to see it:
# the image carries the field labels a sighted user reads, and whether the widgets' `/TU` repeats them
# is not something nib can know. So the honest answer is 495's own — make no claim.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -run TestAFormOnAnImageOnlyPageMakesNoClaim -count=1"
EXPECT="whose only content is a picture"
