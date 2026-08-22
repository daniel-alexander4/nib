# docs/red-proofs.md, tier 1: "setPending checks only that a listener exists, not that it is
# THIS one" (P05 sweep, v1.117.37)
#
# The defect: `setPending` was the one `session` mutator without an identity check. Its four
# siblings — disarmIf, clearPendingIf, clearVerifyIf, setVerify — all carry one, and their
# comments state the reason: a session goroutine spans the user's consent, the signing and a
# 128 MiB write, so it can still be running after the user cancels and re-arms. Checking only
# `se.ln == nil` passes in exactly that window.
#
# What it costs: the stale goroutine parks ITS consent request as the NEW session's pending.
# The user is shown a document from the connection they just cancelled, attributed to the peer
# they have just armed for — and if they accept, they have consented to the wrong thing.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestAStaleGoroutineCannotParkConsentOnTheSessionThatReplacedIt"
EXPECT="parked its consent request on the session that replaced it"
