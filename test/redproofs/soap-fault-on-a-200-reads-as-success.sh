# docs/red-proofs.md, tier 1: "a UPnP refusal carried on a 200 is read as success" (v1.117.130, from
# grilling /pending 262)
#
# The defect: soapCall looked for a UPnPError only when the status line was not 200, and
# soapAddPortMapping never inspects the body — it infers success from the absence of an error. So a
# device that answers its refusal with a 200 produced a CONFIRMED mapping out of a refused one: the loop
# recorded a delete handle, GetExternalIPAddress succeeded (an unrelated action), and Nib published a
# SIGNED record naming a public port that was never forwarded. D19 then read havePortMap as true and
# told the user their trouble was "most likely a firewall".
TIER="tier 1 — go test"
PROVE="go test ./internal/portmap/ -run TestAFaultOnA200IsStillARefusal -count=1"
EXPECT="was read as SUCCESS"
