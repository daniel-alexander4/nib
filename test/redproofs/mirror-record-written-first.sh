# docs/red-proofs.md, tier 1: "the mirror writes its record before its document" (P07.S02a, v1.117.155)
#
# The defect: `WriteMirror` writes `record.json` first and `document.pdf` second. A crash — or any
# failed second write — then leaves (record, no document), which is BYTE-IDENTICAL to the
# deliberately document-less mirror `WriteMirror` itself creates when pdf is nil. A resuming party
# cannot tell "no document yet" from "the document was lost", and C22's durability claim degrades
# to a state nothing can interpret. Document first inverts it: the record is the last thing to
# land, so the record's presence means both are here and a torn write reads as an ordinary miss.
#
# The stimulus is a `record.json` path that cannot be written because a DIRECTORY is already
# there, asserted unwritable before the write is attempted — without that, "document.pdf exists"
# says nothing about ordering. The guard also drives the inverse state (a legitimate
# document-less mirror reading back cleanly), which is exactly why a torn write must not produce it.
TIER="tier 1 — go test"
PROVE="go test ./internal/ceremony/ -run TestTheRecordIsTheCommitPoint -count=1"
EXPECT="the record was written FIRST, so a torn write leaves (record, no document)"
