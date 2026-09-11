# docs/red-proofs.md, tier 1: "an XMP packet that does not parse" (PLAN-accessibility.md
# P03.S01, v1.129.30)
#
# The defect: the title lands in the packet as `innerxml` rather than `chardata`, so
# `encoding/xml` stops escaping it. Titles come from FILE NAMES — `Smith & Jones.docx` is an
# ordinary name — so a raw `&` reaches the stream and the packet is no longer well-formed.
#
# That is strictly worse than having no packet at all: the document fails the clause it was
# written to pass AND breaks every reader that tries to parse its metadata. The assertion is
# that the packet PARSES, not that it contains a string, because a substring check is green
# against a packet no parser will accept.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -run TestSetTitleEscapesAHostileTitle"
EXPECT="XMP packet is not well-formed XML"
