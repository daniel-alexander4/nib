# docs/red-proofs.md, tier 1: "the update route republishes the managed flag" (/pending 350, v1.117.309)
#
# `Managed` goes back onto `updateResponse` as a wire field. It is consumed by `assetURL` INSIDE the
# handler, before the response is written, and read by nothing at the far end: `managed` appears
# nowhere in `web/app.js` or `internal/cli/`.
#
# So the client is handed the INPUT to a choice the server has already made. What it uses is
# `downloadUrl`, which was picked using this value.
#
# **The row is also the proof that the scan now covers this package.** Before /pending 347 the
# reader scan could not see `internal/server` at all, so re-adding a published-and-unread field
# there failed nothing on the Go side. It fails now, by name.
TIER="tier 1 — go test"
PROVE="go test ./ -run TestEveryPublishedObservableHasANamedReader -count=1"
EXPECT="server.updateResponse.Managed"
