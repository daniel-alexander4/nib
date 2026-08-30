# docs/red-proofs.md, tier 1: "a torn rewrite leaves a document beside the previous hop's checksum"
# (/pending 321, v1.117.271)
#
# The defect: the sidecar is no longer unlinked before the document is written, so at hop 2 or later
# a crash between the two leaves a complete, valid document beside the PREVIOUS hop's checksum, and
# ReadMirror reports ErrMirrorDamaged permanently — against the user's own disk, with no repair
# path, on the file that for a non-convener is the sole durable copy of their own signature.
#
# **The probe discriminates the two orderings rather than merely breaking both, and that is the
# point.** The torn state itself needs a crash, which this repo refuses to fake in the product, so
# the observable is the other side of the same ordering: make the DOCUMENT write fail while a good
# sidecar exists and ask what the sidecar is afterwards. Document-first leaves the stale checksum;
# unlink-first leaves none. A mutation that reddened both orderings would prove only that the test
# runs.
#
# TestTheRecordIsTheCommitPoint stays GREEN under this patch — the record is still written last, so
# the other ordering invariant is untouched and this row cannot be satisfied by breaking it.
TIER="tier 1 — go test"
PROVE="go test ./internal/ceremony/ -run TestAFailedRewriteLeavesNoChecksumForADocumentItDidNotWrite -count=1"
EXPECT="a checksum survived a failed document write"
