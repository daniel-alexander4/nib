# docs/red-proofs.md, tier 1: "a sighting for another arm is still a candidate" (/pending 385, v1.128.30)
#
# The defect: `resolve` stops matching the announced hop, so a machine's delivery arm and its hop
# arm are indistinguishable on the link again. The racer dials both and keeps whichever answers
# first — and the delivery arm auto-confirms the spoken check (`autoVerifier`) and can never serve a
# stored contribution, because `ReceiveDocument` never reaches `coSignExchange`.
#
# The reader drives the real scenario: one peer, TWO announcements on two ports, and a browse that
# is told only what it WANTS. That shape is deliberate — ADR-010's defect stayed latent because the
# harness handed both ends the same constant.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestABrowseWithBothArmsUpReturnsOnlyTheOneAsked -count=1"
EXPECT="want exactly 1"
