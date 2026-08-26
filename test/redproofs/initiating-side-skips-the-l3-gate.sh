# docs/red-proofs.md, tier 1: "the INITIATING side skips the L3 gate" (P07.S03, v1.117.159)
#
# The defect: `buildCoSigned` loses its `AdmitContribution` call. L3 then holds at one of its two
# contribution entry points — the receiving side in `internal/p2p` — and not at the one in this
# package, so a party can sign out of roster order simply by being the one who dials.
#
# **Refusing late here is irreversible**, which is why the guard asserts that nothing was signed
# rather than only that an error came back: `buildCoSigned` applies the LOCAL user's signature, and
# a signature cannot be taken back off a document. It is the same reasoning the ceremony deadline
# check one caller up already states in its own words.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheInitiatingSideIsGatedToo -count=1"
EXPECT="a party signed out of roster order through the initiating door"
