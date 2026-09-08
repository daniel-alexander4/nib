# docs/red-proofs.md, tier 3: "the idle-exit arms unconditionally" (P01.S03, v1.128.39)
#
# D2 arms the idle-exit only for a process that LAUNCHED a browser. Arming regardless is the shape
# the slice's own acceptance clause asks to be proved red, and it is the shape that at P01.S04
# would exit every harness mid-run — surfacing not as an assertion but as a connection refused
# somewhere unrelated, because the server under test has gone.
#
# **Tier 3 and not tier 1, because the claim is about the shipped binary.** `internal/server`'s
# tests prove the RULE (`IdleExitDecision`) and a source scan proves no harness CAN arm it from its
# launch lines. Neither can see a binary that ignores its own environment. This is the one that
# drives the real thing and reads what it logged.
TIER="tier 3 — the real binary in a real browser"
PROVE="./build/uirepro.sh"
EXPECT="the harness ARMED the idle-exit"
