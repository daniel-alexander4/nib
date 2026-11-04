# ADR-202 — An email prints five named headers and counts its attachments; `.msg` is refused by name

Status: accepted, 2026-11-04 (PLAN-import-export P04.S05, v2.44.0). Extends ADR-200 and ADR-201; Y8, D2.

**One row, `eml`** (`.eml`; `FamilyEmail`, sniffed as RFC 5322 header lines at the top with at least two headers and one of
From, To, Subject, Date, Received, Message-ID, MIME-Version, Return-Path; `message/rfc822` moved here from `mhtml`). A message
is decided after a saved page, so `multipart/related` with `MIME-Version` stays MHTML. A text file that opens with header-shaped
lines and is named `.txt` stays text, and a message with no `.eml` name is not guessed.

**Five headers, as text.** From, To, Cc, Date and Subject are printed, decoded from RFC 2047, with control and
bidirectional-override characters removed, white space collapsed, 1,000 characters a header and 50 addresses a list. They are
escaped; no other header (Received, X-Originating-IP, Message-ID, Return-Path, every tracking header) is ever shown.

**Attachments are counted, never read.** `walkMIME` gained a `decodeIf` hook: a part that is `Content-Disposition: attachment`, or
not text or a picture, is recorded with no body and never decoded, so a 12 MiB attachment cannot trip a cap or reach memory
decoded. A picture with no `Content-ID` is an attachment. The count is `ImportResult.AttachmentsDropped` (read by `nib office`,
which prints a note) and the PDF says "N attachments were not imported". The window shows the PDF.

**The body takes ADR-200's path.** The first HTML part is sanitised with a resolver holding only the message's own `cid:` pictures
(sniffed by bytes, as ADR-201), joined under the header block; else the first plain-text part is escaped in a wrapping `pre`. A
tracking pixel `<img src=http://…>` and a linked style sheet become nothing and fetch nothing (a canary server sees 0 hits).
Caps are the saved-page ones: 500 parts, 8 MiB a decoded text part, 96 MiB together, 4 levels, a 64 KiB header block.

**`.msg` is refused by name** (`ErrMsgNotSupported`, 422, cause `unsupported`): an OLE file named `.msg` gets a sentence telling
the person to save as `.eml`. There is no row for `.msg`, so no picker offers it. Not taken as a dependency: an OLE reader built
to Y8's bar (FAT size, cyclic chains, stream-size mismatch) is its own slice.

**Declared.** Only the first HTML or plain-text body is printed (a forwarded message in `message/rfc822` is an attachment);
the window shows the attachment count only through the PDF's own line; a non-UTF-8 charset on a plain-text body is
converted, an unknown label is read as it is. No message from a real mail client other than hand-built fixtures was run.
