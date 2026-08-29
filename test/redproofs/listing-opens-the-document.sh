# docs/red-proofs.md, tier 1: "the ceremonies listing opens every document" (P08.S03, v1.117.243)
#
# The defect: `ReadStored` calls `ReadMirror`, which runs `sign.Verify` and — while the document is
# unsigned — a full `ContentDigest`. Measured at the P08.S01 deepdive: 10 ms at 100 pages, 69 ms at
# 500, 195 ms at 1000, superlinear, on TEXT-ONLY fixtures. At fifty stored ceremonies that is
# seconds on a request path, and these are contracts with images.
#
# The guard plants a `document.pdf` that is not a PDF at all, so anything which parsed it would fail
# rather than merely being slow. **The stimulus is the assertion**: cost has no cheap observation in
# a unit test, but "did you touch it" does.
TIER="tier 1 — go test"
PROVE="go test ./internal/ceremony/ -run TestTheListingDoesNotOpenTheDocument -count=1"
EXPECT="it opened document.pdf"
