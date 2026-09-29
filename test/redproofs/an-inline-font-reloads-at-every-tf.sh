# docs/red-proofs.md, tier 1: "a direct font is reloaded at every Tf" (/pending 723, v1.169.8)
#
# The defect: runWalker.fontFor caches only indirect fonts, so a direct font dictionary re-parses its
# /ToUnicode at every Tf — 17.7 s for 14.4 KB of content, measured.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -run '^TestAnInlineFontIsLoadedOncePerWalk$' -count=1"
EXPECT="loaded it 1600 times, want 1"
