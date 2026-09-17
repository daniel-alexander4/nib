# docs/red-proofs.md, tier 1: "the Markdown unprintable-rune scan runs on every input"
# (/pending 491, v1.133.10)
#
# The defect: `cmdOffice`'s guard on `pdfops.SupportedMarkdownExt(ext)` removed, so the scan runs
# on the raw bytes of whatever was handed in. An ODT or DOCX is a ZIP, so every office conversion
# warns about its own compressed bytes — a one-line German ODT that converted perfectly reported
# 163 unprintable characters, and a warning that fires on every office document is the one nobody
# reads on the Markdown document where it is true.
#
# Two of the test's three inputs go red on it: the byte-level .docx, which needs no converter and
# so runs everywhere, and the real ODT, which needs LibreOffice. The Markdown control stays green,
# which is what says the guard was added rather than the warning deleted.
TIER="tier 1 — go test"
PROVE="go test ./internal/cli -run TestTheUnprintableWarningIsAskedOfMarkdownOnly"
EXPECT="warned about the container's own bytes"
