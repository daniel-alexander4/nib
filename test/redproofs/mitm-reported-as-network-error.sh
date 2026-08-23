# docs/red-proofs.md, tier 1: "The MITM signal reported as a connection failure" (P05 close, v1.117.105)
#
# The defect: handleSessionInitiate routed p2p.ErrVerificationDeclined — the "four words don't
# match" man-in-the-middle verdict — and ErrVerificationTimedOut to writeConnectDiagnosis, which
# renders a 502 "could not connect" (and can show an unrelated D19 cause). verify.go's own doc
# says this verdict "must never be reported as a network error … 'could not connect' invites a
# retry, which is the worst possible advice when someone is sitting between you."
#
# What it costs: a user who looked at the words and said they do not match — the one signal that
# catches an email-level attacker — is told to check their address and clock, i.e. to retry, under
# an active MITM.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestInitiateLiftsTheMITMSignalBeforeTheNetworkDiagnosis"
EXPECT="does not lift p2p.ErrVerificationDeclined"
