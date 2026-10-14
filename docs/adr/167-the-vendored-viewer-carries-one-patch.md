# ADR-167 — the vendored viewer carries one patch, and a text layer that was not rendered gives no links

**Status:** accepted
**Date:** 2026-10-13
**Extends:** ADR-066 and ADR-142 (nib's two modified Go modules, and the rule that a modified copy says what
differs and is held to it).
**Context:** `/pending 887`. The item was filed for a console error — pdf.js logging "`render` method must be
called before `injectLinkAnnotations`" when the window was resized just after a rotation — with one question
left open: is a page ever left without its links? Measured in a real Chromium window on a three-page document
whose pages each hold one declared `/Link` and one web address that is only text (which pdf.js's autolinker
turns into a link after the text layer is rendered): with a rotation followed by a resize, page 1 was left
**without its text link for good in 22 of 40 rounds** on the vendored pdf.js 6.0.227 — 0 of 40 with the rotation
alone, 0 of 60 with the resize alone, and never the declared link. Nothing is logged when it happens.

The error in the item's title is fixed upstream (mozilla/pdf.js #21396, first published in 6.1.200). The loss
is not: **15 of 40 rounds on 6.1.200 and 11 of 30 on 6.4.299**, the newest release. Traced in the running
viewer, it is this. A page redrawn while its text layer is still rendering keeps its annotation-layer and
text-layer builders. The second drawing's text render begins by cancelling the first's. `#renderTextLayer`
swallows that `AbortException` and returns, so the first drawing's wait on its text layer ends; its
`#injectLinkAnnotations` passes #21396's identity check — both builders are the same kept objects — reads the
second drawing's still-empty text, finds no addresses, and the builder records its links as added. The second
drawing's own step, with the text now there, is skipped.
**Applies:** `web/vendor/pdfjs/` (all four built files, `VERSION`, `NOTICE.md`), `vendorpatch_test.go`,
`test/ui/autolink.test.mjs`, `test/pdfjs/textcontent.mjs`, `build/gen-notices.sh`, `THIRD-PARTY-NOTICES.md`.

## Decision

**1. The vendored pdf.js is 6.1.200.** The smallest published step that carries #21396. The package's four
built files are taken as published; the CMap tables and the set of images the viewer references are the same in
both versions (diffed), so nothing else moves.

**2. `pdf_viewer.mjs` differs from the published file in two lines, each marked `Nib (ADR-167)`.**
`PDFPageView.#renderTextLayer` returns `false` when its text layer's rendering was aborted, and
`PDFPageView.#injectLinkAnnotations` returns when the promise it waited on answers `false`. A drawing whose
text was never rendered looks for no links in it; the drawing that replaced it does its own looking. With the
two lines: 0 of 80 rounds left a page without a link.

**3. The copy says what was modified, and a guard holds it to that.** `web/vendor/pdfjs/NOTICE.md` names the
file and the two lines. `TestTheVendoredViewerIsUpstreamsButForItsOnePatch` pins the version and the SHA-256 of
each of the four files as published: the three untouched files must hash as published, the viewer must carry
both lines, and with each put back as upstream wrote it must hash as published too. There is no module cache
to compare a vendored web file against, which is why the pins are hashes.

**4. `test/ui/autolink.test.mjs` is the standing reader.** Twelve rounds of rotation-then-resize, each ending
with every drawn page holding both of its links. It is red on the viewer as published (first lost round: 6).

## Refused

- **Working around it in `web/app.js`.** The mark the race sets is a private field of pdf.js's builder; the
  window could only notice a page with a web address in its text and no link for it, throw the page's layers
  away and draw it again — a visible redraw, a second copy of pdf.js's own address matcher (the `Autolinker`
  is not exported), and a race of its own. ADR-163 refused patching a library where nib's own code could hand
  it something it handles correctly; nothing nib can hand pdf.js avoids this.
- **`enableAutoLinking: false`.** It removes the race by removing the feature.
- **Waiting for upstream.** The loss is in every release to date and nib's users would keep it until one that
  has not been written.
- **Going to 6.4.299 in the same step.** The viewer alone differs from 6.0.227's in 9,470 lines against 137,
  and this change is judged by the tiers; the larger step is its own change with its own run.

## Consequences

- **Re-apply the two lines at every bump of pdf.js, or drop them when upstream has its own fix.** The guard
  fails at any bump until the pins move; whether the lines are still needed is what `autolink.test.mjs` says
  when run against the new viewer as published.
- `web/app.js`'s comments that cite `pdf_viewer.mjs` by line were re-pointed (ten citations, each found again by
  its text).
- 6.1.200 also changes how check boxes and radio buttons are drawn (#18907, #21428). The real-forms instrument
  (`build/formrt.sh`, ADR-166) read the same on both builds over one list of 203 documents: 5,616 of 5,616 values
  window → pdfcpu, 5,531 of 5,576 pdfcpu → window (the gap is one refused fill), no untouched field changed.
- `test/pdfjs/textcontent.mjs` (pdf.js run in Node 20 for the text-run comparison) fills
  `Map.prototype.getOrInsertComputed`: 6.1.200 reaches it when reading text. 6.0.227 already called it,
  unfilled, in 23 places, so the browsers nib needs did not change.
- `noticestack.test.mjs` no longer filters the console error; it asserts no console error at all.
- Declared: the patch is to the BUILT file nib vendors, not to pdf.js's source tree; a report to upstream is
  drafted and sending it is Dan's.
- Firefox 157 has the same loss (20 of 30 rounds on the published 6.1.200 viewer) and the same cure
  (`autolink.test.mjs` green there with the two lines).
