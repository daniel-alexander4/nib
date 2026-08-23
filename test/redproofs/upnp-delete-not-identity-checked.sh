# docs/red-proofs.md, tier 1: "a UPnP delete removes whatever holds that port" (v1.117.121, from
# grilling /pending 258)
#
# The defect: DeletePortMapping carries (remote host, external port, protocol) and NO internal client, so
# IGD removes whichever host on the LAN holds that external port. Harmless while every delete named a
# mapping created seconds earlier; it stopped being harmless when /pending 257 began recording a handle
# for a POST that was written and never answered. The external port is the internal one, a Linux
# ephemeral 32768-60999 — exactly where a console or a torrent client pins its UDP port.
TIER="tier 1 — go test"
PROVE="go test ./internal/portmap/ -run TestAUPnPDeleteRefusesAMappingThatIsNotOurs -count=1"
EXPECT="want a refusal naming"
