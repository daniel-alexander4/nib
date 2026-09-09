# docs/red-proofs.md, tier 1: "a measured field trusts the font's name" (P01.S01, v1.128.64)
#
# Field.Font is arbitrary client input — pdf.js hands over whatever the document's BaseFont
# said — and pdfcpu's metrics table PANICS on a name it does not carry ("pdfcpu: user font
# not loaded: Arial"). StampFields is called from handleBake, so measuring the REQUESTED
# font rather than the coerced one puts a panic on /api/bake for most real documents.
#
# The guard is not a validation bolted in front of the measurement: coreFont is the same
# call that already decides which face is stamped, so routing the measurement through it
# makes "measured the wrong font" and "panicked on an unlisted font" both unrepresentable.
# The test also asserts that CoreWidth still panics, so it can tell a working coercion from
# an unnecessary one.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -run TestStampWidthMeasuresTheFaceThatIsActuallyStamped -count=1"
EXPECT="want the Helvetica the stamp will actually use"
