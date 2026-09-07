# docs/red-proofs.md, tier 1: "the unshown spoken check records nothing" (P02.S04, v1.128.12)
#
# The defect: the `errVerifyBusy` exit records no outcome. That is the one path where the four
# words reach NOBODY — `setVerify` refused the slot and `sv.saw.mark()` has not run, which its
# own comment states: "Nothing was put in front of anyone."
#
# Recording nothing there is not a quieter answer, it is a DIFFERENT one: absence means UNKNOWN — an
# older build, a failed write, a hop that has not happened — so a machine that showed nobody
# anything becomes indistinguishable from one running a build too old to say. D5's third state has
# to be written positively or it does not exist.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestEveryExitOfTheSpokenCheckRecordsItsOutcome -count=1"
EXPECT="exit(s) and"
