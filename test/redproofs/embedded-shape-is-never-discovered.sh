# docs/red-proofs.md, tier 1: "an embedded shape is never discovered" (/pending 347, v1.117.308)
#
# The promotion of embedded types into discovery is removed, so a type that reaches the wire only by
# being EMBEDDED in a published shape goes back to being invisible.
#
# `pdfops.WatermarkStyle` is the live instance and it had been invisible for the life of this scan:
# it is exported and json-tagged, and it is only ever a PARAMETER — `StampWatermark(pdf, text, st)`
# — so "returned by an exported function" never admitted it. Its four fields reach the browser
# inside `server.watermarkParam` with nothing checking any of them was consumed, and one of them
# (`Angle`) took a full pass to confirm was actually set by the client.
#
# Embedding into a published shape IS publication. The return test is a proxy for "this leaves the
# package", and this is a place the proxy is simply wrong.
TIER="tier 1 — go test"
PROVE="go test ./ -run TestEveryPublishedObservableHasANamedReader -count=1"
EXPECT="which this scan does not discover"
