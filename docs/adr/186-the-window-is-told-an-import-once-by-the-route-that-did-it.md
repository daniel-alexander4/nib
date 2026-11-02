# ADR-186 — The window is told an import once, by the route that did it

**Status:** accepted
**Date:** 2026-11-02
**Context:** `PLAN-import-export.md` P01.S02b (D6, D12, W28, W31, Y1, Y12); extends ADR-185.

## Decision

- **`importedFrom` rides on the answer of the route that imported** — open, upload, fetch-by-URL, Import… — set after `docResponse` from `document.imported`, and on **no later answer**: `/api/docs`, a reload and a second window's read omit it, or every window that joined later would announce every import again. The OS hand-off is not a request the window made, so its event on the window stream carries `importedFrom` (opened) or `reason` (refused) instead; a refusal says why in the import function's own words rather than "Nib may be full".
- **One polite status region, `#importNotice`** (`role="status"`, `aria-live="polite"`; not `#sessionNotice`, whose role is `alert`), moves no focus, coalesces imports landing together into "Imported 3 files", is keyed by document id (which carries the process epoch, so ids never repeat across restarts) so a replay says nothing twice, and carries *Show Export*. The sentence is one function, `importNoticeWords`.
- **`document.imported` holds the base name only.** The plan's `imported{Base, Path}` had a `Path` nothing reads; a field nothing reads is a standing invitation to be trusted, so it is not carried.
- **The Open group is "Open & Import"** and the entry that converted-with-options is **Import…** (same id, `officeOpenBtn`). The Quit modal names a converted tab's source file instead of "has unsaved changes".

## Declared limits

The hand-off backlog list and per-item Cancel (/pending 939); one LibreOffice paragraph instead of one per tool (the table names one); the first-window launch path announces nothing; no wording-table guards.
