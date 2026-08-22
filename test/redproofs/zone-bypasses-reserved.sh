# docs/red-proofs.md, tier 1: "A zone bypasses the reserved prefix table" (P05.S04 T01)
#
# The defect: addrscope.Routable does not strip an IPv6 zone before walking `reserved`, and
# netip.Prefix.Contains is false for any zoned address. So a zone clears the entire table
# while the byte-test predicates above it keep working — which is why it stayed invisible:
# ::1%eth0 and fe80::1%eth0 are still refused, and those are the cases anyone would try.
#
# What it lets through: ::c0a8:101%eth0 (that is 192.168.1.1), ::7f00:1%eth0 (127.0.0.1),
# 6to4, NAT64, documentation and ORCHID space. The attacker is an in-roster counterparty,
# and what they gain is aiming Nib's dials at hosts on the victim's own network.
TIER="tier 1 — go test"
PROVE="go test ./internal/addrscope/ -run TestAZoneCannotSmuggleAnAddressPastTheReservedTable"
EXPECT="is Routable but"
