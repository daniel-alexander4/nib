# docs/red-proofs.md, tier 2: "a shape-level deferral that does not actually defer" (/pending 254,
# v1.117.117)
#
# The defect: `deferredFields` ignored, so P06's unbuilt diagnosis shapes are scanned again — and
# `diagnosisResponse.detail` passes as consumed on one `f.detail` in renderScanReport, a
# pdfops.Finding with nothing to do with D19. That is the whole of /pending 254: a field whose
# renderer does not exist reading as covered, one field over from siblings correctly parked.
#
# EXPECT names the STIMULUS assertion rather than the evidence one, deliberately: with deferrals
# ignored there are zero deferred fields, so the stimulus fires first and the evidence check never
# runs. The full failure shows the laundering itself — `cause` and `summary` come back unread while
# `detail` sails through on renderScanReport's line, which is the asymmetry the item reported.
TIER="tier 2 — jsdom"
PROVE="node --test test/jsdom/published.test.mjs"
EXPECT="the mechanism is not being applied"
