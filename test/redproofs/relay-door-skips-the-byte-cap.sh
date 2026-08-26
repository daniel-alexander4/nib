# docs/red-proofs.md, tier 1: "the relay door skips the byte cap" (P07.S05, v1.117.176)
#
# The defect: the fourth commit door stops calling `byteCapLocked`. ADR-008 says the cap binds
# **every door that grows a document**, and a relay hop grows one — so this is the door a ceremony's
# bytes now arrive through.
#
# It is a tightening rather than a preserved property, and that is why it is asserted: the door
# these bytes used to take, `addDoc`, applies no cap at all. The guard also drives the other
# direction — a hop that makes the document SMALLER can never be refused, because `byteCapLocked`
# measures the total after the write rather than the delta.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheRelayDoorHonoursTheByteCap -count=1"
EXPECT="want ErrTooManyBytes"
