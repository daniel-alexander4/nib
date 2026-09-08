TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestEveryDialDeclaresItsRole -count=1"
EXPECT="are reached BEFORE their function declares a role"
