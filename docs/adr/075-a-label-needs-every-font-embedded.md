# ADR-075 — the PDF/UA label needs every font embedded

**Status:** accepted. `/pending 820` (2026-10-03), from the P04 phase-close review of `PLAN-returned-document.md`.
Extends ADR-033: its five refusals become six. ADR-033's decision is otherwise unchanged.

## Context

ADR-033 rests the label on veraPDF's measurement of nib's own Markdown conversion — and that measurement was taken
with nib's authoring faces installed, so every font in the measured document is embedded. `ConvertDocToPDF` does not
always produce that document. When the faces cannot be installed (a read-only or full user-font directory), `mdpdf`
degrades on purpose to the Base-14 core fonts rather than fail the conversion, and logs it. A core font is by
definition not embedded, so the degraded document fails ua1 7.21.4.1 however correct its tags, title and language are.

`LabelUA` checked tags, `/Lang`, `DisplayDocTitle` and the packet, never embedding. So on exactly the machine where the
install failed, nib wrote `pdfuaid:part 1` over a document veraPDF would fail — measured: a tagged, titled, declared
conversion with no base faces was labelled.

## Decision

`LabelUA` refuses a document with any font whose program is not embedded, as `ErrUAFontNotEmbedded` naming the fonts.
It asks the **bytes** (`nonEmbeddedFonts`, the same sweep PDF/A's blockers use), not whether the install succeeded,
because the bytes are what the claim is about and a second source of the same fact can disagree with the first.

Both callers already surface any refusal other than "language not chosen" — the CLI as a note, the server in its log —
so the conversion still succeeds and the user loses only the label: ADR-031's visible loss rather than a false claim.

## Considered

- **Refuse at the call sites when `InstallFaces` fails.** Two copies of one rule (ADR-009), and it trusts the install
  report over the document, which is the weaker evidence.
- **Fail the conversion.** ADR-033 and `mdpdf`'s degrade both decided a font directory must cost a prettier document,
  never the document.
