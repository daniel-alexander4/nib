TIER="tier 1 — go test"
PROVE="go test ./internal/p2p/ -run TestAPeerThatDidNotNegotiateTheRoleFrameExchangesNothing -count=1 -timeout 40s"
EXPECT="panic: test timed out"
