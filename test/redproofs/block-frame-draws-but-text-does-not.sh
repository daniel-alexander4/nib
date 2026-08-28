# docs/red-proofs.md, tier 3: "A signature block with a frame and no attestation" (/pending 305, v1.117.214)
#
# The defect: `renderAttestation`'s `lines.forEach(... fillText ...)` is removed. The white
# field and the black frame still draw, so the block is plainly visible and plainly a Nib
# signature block — and it says nothing. Every structural check passes, the signature verifies,
# and the /Reason still carries the machine-readable attestation, so only a reader looking at
# the page can tell.
#
# It is the row that separates "the appearance arrived" from "the appearance has content":
# white-on-white loses both, and this loses only the second.
TIER="tier 3 — the real binary in a real browser"
PROVE="./build/uirepro.sh"
EXPECT="the attestation text is not"
