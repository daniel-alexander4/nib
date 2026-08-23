# docs/red-proofs.md, tier 1: "an XFDF in another encoding refused as invalid" (/pending 266,
# v1.117.134)
#
# The defect: encoding/xml declines a non-UTF-8 declaration unless a CharsetReader is set, and Nib
# wrapped that as "invalid XFDF" — the same sentence a corrupted or non-XFDF file gets. The user was
# told their file is not an XFDF when the truth was that Nib did not read that encoding. XFDF is an
# interchange format whose whole job is arriving from another vendor's product.
#
# The row's fixture is deliberately NOT valid UTF-8, asserted in its own setup, or it would pass with
# the fix removed.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -run TestAnXFDFInAnotherEncodingIsRead -count=1"
EXPECT="was refused"
