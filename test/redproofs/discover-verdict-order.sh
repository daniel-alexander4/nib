# docs/red-proofs.md, tier 1: "`nib discover`'s verdict switch reordered"
#
# The defect: `Own == 0` is tested before `Sent == 0`. When nothing was sent, nothing can
# have come back — so the firewall verdict is ALSO true of that state, and reaching it first
# tells a user "a local firewall is dropping multicast" about a machine where no
# announcement was ever attempted. It points them at their firewall instead of at the
# interface list printed three lines above.
TIER="tier 1 — go test"
PROVE="go test ./internal/cli/ -run TestNothingSentIsDiagnosedBeforeNothingReturned"
EXPECT="the verdict does not say so"
