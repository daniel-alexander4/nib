# ADR-115 — the checker reads a mistyped text entry itself, set aside for pdfcpu's validator only

**Status:** accepted
**Date:** 2026-10-09
**Context:** `/pending 612` and `/pending 633`. pdfcpu's validator refuses a whole document over a `/Lang` on the
catalog, or a `/Lang`, `/Alt`, `/ActualText` or `/E` on a structure element, that is not a string
(`decodeString: dict=StructElementDict entry=Alt invalid type types.Name`). `nib ua` therefore said *"the document
could not be read"* about files veraPDF reads and grades — and one of them is a lost FAIL, a name-typed
`/ActualText` with no language. Extends ADR-052 (the checker reads as veraPDF does). `internal/pdfread/pdfread.go`
(`validated`), `internal/pdfread/optimize.go` (`ReadOptimizedOrRefuseSettingAside`), `internal/uacheck/mistyped.go`.
**Applies:** the PDF/UA checker's read, and any later reader with the same need.

## Decision

**1. The read door takes one optional step between the read and the validation.** `aside(ctx)` removes what the
validator would refuse and returns what puts it back; it runs for the validator only, as `escapeInfoKeys` already
does for information-dictionary keys. The context every rule reads afterwards is the document as parsed.

**2. The checker is its only caller, and sets aside exactly four keys.** `setAsideMistypedText`: `/Lang` on the
catalog; `/Lang`, `/Alt`, `/ActualText`, `/E` on every structure element reached through `/K` from the tree root,
wherever the value resolves to something that is not a string. A string, a dangling reference and an absent key are
left alone. **Not for a reader that writes the context back out** — nothing has vouched for what was set aside, so
`pdfread.Validated` and every `pdfops` caller are unchanged and still refuse these documents.

**3. Put back, never rewritten.** Nothing in the step decides what a wrong type means. The rules read the value the
document wrote, through the doors that already answer "not a string" (`d.text`, `declaresLang`).

**4. `/ActualText` has its own door, because veraPDF reads it differently.** `d.actualText`: a string **or a name**
is text; a number, a boolean, an array, a dictionary and `null` are absent. `/Alt`, `/E` and `/Lang` stay on `d.text`
— a string or nothing. Measured on veraPDF 1.30.2: `/ActualText /a` on a Figure PASSES 7.3 t1 and, with no language,
FAILS 7.2 t21; every other wrong type on any of the four keys answers on every clause exactly as the key being
absent. `hasAlternateText` used to count an `/ActualText` of ANY type as present (`/pending 633`); it could not be
wrong while those documents could not be opened.

## Consequences

- 62 documents, every clause nib checks, strictly: no difference from veraPDF (`TestAMistypedTextEntryIsReadAsVeraPDFReadsIt`,
  which re-asks veraPDF when it is present and holds eighteen measured rows when it is not). Undone four ways — nothing
  set aside, never put back, a name not text, any type text — it went red each time.
- A Note with a name-typed `/ID`, `/pending 612`'s original case, already opened when this was built; it is in the
  population as a control.
- Declared gap: the same refusal class on other dictionaries — an annotation's or a file specification's mistyped
  entries, a TrapNet without `/F`, an `/Encoding` name pdfcpu does not know (`corpusUnreadable`) — is unchanged. The
  door now exists for them; each needs its own veraPDF measurement before a key is added to it.
- A Type 3 font written directly in `/Resources` is still DROPPED by the validator (`/pending 612`'s second
  amendment). That is a removal, not a refusal, and this step does not see it.
