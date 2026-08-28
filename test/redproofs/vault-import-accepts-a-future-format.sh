# docs/red-proofs.md, tier 1: "the destructive vault import accepts a future format"
# (/pending 287, v1.117.194)
#
# The defect: `Validate` has a floor (`env.Version < 2`) and no ceiling, so a vault stamped with a
# newer format passes, is written over the user's own by the import handler, and is then refused by
# `Open`. The previous vault is gone and the new one will not open until they update.
#
# **The handler's own doc calls `Validate` the only thing standing between a mis-picked file and the
# permanent loss of the signing identity**, and it let through exactly the file the reader would not
# open — after the overwrite.
#
# `checkEnvelopeVersion` is the door and it already existed. This was the one caller that did not
# use it, which is that function's own recorded history repeating one site over: ADR-009, a rule
# with a door and a caller that does not call it.
#
# The check bumps the version on a REAL, openable backup, so the only thing wrong with the file is
# the number — a hand-built envelope would be refused by the shape checks and pass without the
# ceiling existing at all.
TIER="tier 1 — go test"
PROVE="go test ./internal/vault/ -run TestValidateRefusesWhatCannotBeOpenedHere -count=1"
EXPECT="a backup written by a NEWER Nib passed validation"
