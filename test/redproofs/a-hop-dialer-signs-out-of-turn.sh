# docs/red-proofs.md, tier 1: "a hop dialer signs out of turn" (/pending 517)
#
# The defect, verbatim as it stood until /pending 517: `ceremonyID.carries` answered `i < pr.Done`
# — carry only once this document already holds my signature — so a SIGNING party whose turn had
# not come yet was told to contribute. Its own doc defended that, on the grounds that deciding
# "not my turn" here would be a second implementation of the turn rule and that
# `AdmitContribution` refuses it downstream anyway.
#
# Both halves were wrong. `canonicalRoster` prepends a signing convener at position 0 only when the
# client did not name them ("a caller who wants another position includes themselves in the roster
# and this branch does not run"), and `SigningOrder` does no promotion — so a convener who names
# themselves second IS second. Under D22's hub that convener is at one end of every hop, so at hop
# 1 they are a pure carrier, and `carries` called that "contribute": the quote answered
# {"mine":false,"contributes":false} (no block to render) while the dial demanded one, and
# `/api/ceremony/hop` came back 400 "this hop needs your signature block and none was sent".
# Measured through both routes. EVERY hop of such a ceremony failed, at the convener's own machine.
#
# `Progress`'s own doc says Order[Done] IS whose turn it is and that a caller wanting per-party
# state derives it from those two fields — so `i != pr.Done` reads the one walk rather than adding
# a rule.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run 'TestAPartyWhoseTurnHasNotComeCarries' -count=1"
EXPECT="reports that it contributes at this hop"
