# docs/red-proofs.md, tier 1: "a TCP ceremony hop runs with no ceremony" (P07.S02b, v1.117.157)
#
# The defect: `serveOneSession` reads the ceremony off `anchor.cer` again. The anchor carries one
# only on the QUIC coordinator path — the accept loop builds `consentAnchor{ln: ln}` — so every
# TCP ceremony hop got nil, and `ReDeliverer`'s contract says nil means "the manual/LAN path,
# which has no ceremony hop to key on", which is false there. The accept loop re-arms for the
# remainder and accepts again, so a peer reconnecting after a lost channel reached
# `coSignExchange` with no cache and `Contribute` stacked a second, different block.
#
# **Structural, and the behavioural gap is measured rather than assumed:** the re-delivery test
# that exists runs the QUIC path, where `anchor.cer` is set, and stays GREEN against this patch.
# A tier-4 drive over a TCP ceremony is /pending 289.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestEveryCeremonySessionGetsItsCeremony -count=1"
EXPECT="serveOneSession reads \`anchor.cer\`"
