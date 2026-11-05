# ADR-208 — P05 phase close: the SVG tool waits at the gate, a style sheet is judged whole, a repeated picture is one part

Status: accepted, 2026-11-04 (PLAN-import-export P05 phase close, v2.47.2). Amends ADR-204, ADR-205, ADR-206; extends ADR-185, ADR-207.

1. **`ExportPageSVG` takes a slot at `ImportGate`** (ADR-185) after the page check and before any process: parallel requests could each start a
   `pdftocairo`. A full queue or a closing Nib is 503 `busy`. A document that does not parse is `ErrSVGUnreadable`, never "a page it lacks"; a
   non-zero exit, a timeout or no output is `ErrSVGUnreadable` wrapping the tool's words; the memory ceiling is 422 `unreadable`. A 500 is left for
   what Nib itself failed at.
2. **An SVG `<style>` is judged as one text** (ADR-204's allowlist). A comment or CDATA splits a sheet into tokens, and `@im<!---->port` passed the
   per-token check and reached the output joined. The text is collected and `cssIsInert` runs once, at the element's end. A second root element is
   refused, and an attribute that normalises to a name already written (`href` and `s:href`) is written once. Declared and filed: no cap on a
   picture's AREA (14400 x 14400 is accepted) and none on `<use>` fan-out; only the time budget bounds them.
3. **PowerPoint** (ADR-206): pictures come before text in the tree, because its order is the stacking order and a background picture hid its text; a
   picture drawn on many pages is one `ppt/media` part with a relationship per slide, and distinct pictures are bounded in total
   (`MaxMediaBytes`, 256 MiB, `ErrTooLarge` -> 422 `too-large`); `Write` takes ctx and observes it per slide and before the zip; the page count is
   asked before the read, so a document past `MaxSlides` is refused without laying out a page; a page over 56 in no longer has its text shrunk by the
   deck factor twice; a non-finite page size falls back to Letter; dropped links are the reader's own count.
4. **The census test that looped over every export format** (`TestEveryWriterObservesTheContextAndHonoursIt`) panicked on the PowerPoint entry, which
   has no flowing writer: the merge of ADR-207's branch with ADR-206 was never run as one tree. The slides path has its own cancel test.
5. **Declared, not fixed:** no right-to-left paragraph direction, language or line breaks inside a PowerPoint run; no HTTP-layer test for the SVG
   route's 503 or its moved-revision 409; the SVG sniff does not read a prefixed root, UTF-16, or a comment longer than the sniff window.
