# docs/red-proofs.md, tier 1: "`nib discover`'s verdict checks Sent before Own"
#
# The defect: `Own == 0` is tested before `Sent == 0`. When nothing was sent, nothing can have come
# back — so the firewall verdict is ALSO true of that state, and reaching it first tells a user "a
# local firewall is dropping multicast" about a machine where no announcement was ever attempted.
# It points them at their firewall instead of at the interface list printed three lines above.
#
# **The patch moved with the rule (/pending 23, v1.117.352) and the row got STRONGER for it.** The
# switch used to live in `printVerdict`; it is now `discovery.Stats.Verdict()`, the one door both
# callers walk through (ADR-009). So this row mutates the DOOR and proves with the CLI's own test —
# which means a green here says the CLI still ROUTES through the shared rule, not merely that it
# still classifies correctly. A re-inlined private copy that happened to agree would leave this red.
# Its sibling `the-verdict-blames-the-firewall-first` is the same mutation proved through the
# SERVER's test, for the same reason on the other caller.
TIER="tier 1 — go test"
PROVE="go test ./internal/cli/ -run TestNothingSentIsDiagnosedBeforeNothingReturned"
EXPECT="the verdict does not say so"
