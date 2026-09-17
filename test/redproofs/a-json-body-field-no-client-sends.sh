# docs/red-proofs.md, tier 1: "every request field a handler reads is one some client sends"
# (/pending 477, the JSON body carrier)
#
# The THIRD row, and it is the one the first two could not stand in for. Until /pending 477 the
# guard matched exactly three selectors — FormValue, PostFormValue and Get on a Query() receiver —
# so a handler that decoded a JSON body contributed nothing and every field of every JSON route was
# outside it. Both rows above patch a FORM field, so both stayed red while that whole carrier was
# unwatched.
#
# The patch deletes the block/para/line the client sends inside each OCR word. They are the case
# that matters: `handleOCR` decodes `[]pdfops.Word`, so the three fields live one type and one
# package away from the handler, and PLAN-accessibility P06.S06 wired them up with nothing watching
# — deleting this exact send left the guard GREEN before the extension and fails here after it.
TIER="tier 1 — go test"
PROVE="go test ./ -run TestEveryRequestFieldAHandlerReadsIsOneSomeClientSends -count=1"
EXPECT="/api/ocr block"
