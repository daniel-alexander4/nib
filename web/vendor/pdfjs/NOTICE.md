Mozilla pdf.js, as published in the npm package pdfjs-dist at the version in VERSION, with TWO files
modified by Nib.

  pdf.min.mjs, pdf_viewer.css, cmaps/                         unmodified
  pdf.worker.min.mjs                                          modified — one span, marked "Nib (ADR-172)"
  images/annotation-note.svg                                 Nib's own drawing, in pdf.js's place for it
  pdf_viewer.mjs                                             modified — two lines, each marked "Nib (ADR-167)"

What the two lines change (docs/adr/167-the-vendored-viewer-carries-one-patch.md; /pending 887):

  PDFPageView.#renderTextLayer returns false when the text layer's rendering was aborted, where upstream
  returns nothing.

  PDFPageView.#injectLinkAnnotations returns without looking for links when the text layer promise it
  waited on answers false, where upstream goes on to read whatever text the page holds at that moment.

Why: a page redrawn while its text layer is still rendering keeps both layers; the second drawing aborts
the first one's text, the first drawing's link step then reads the second drawing's still-empty text,
finds nothing, and marks the annotation layer as having had its links added. The page is left without
the links pdf.js makes from web addresses in its text. Present in 6.0.227, 6.1.200 and 6.4.299.

Re-apply at every bump, or drop it when upstream has its own fix: vendorpatch_test.go fails until the
pinned version and hashes are moved, and test/ui/autolink.test.mjs is red on the viewer as published.

What the worker's span changes (docs/adr/172-a-ticked-box-of-a-pair-is-the-answer-the-window-saves.md;
/pending 893): in a button widget's setValue, when the parent field's /V has already been put for this save
and is Off, the widget being saved replaces it. Upstream keeps the first widget's /V, and saves an unticked
check box's Off too, so in a check box field whose two widgets carry different on-states (an Acrobat Yes/No
pair) ticking the box saved second stored no answer. Present in 6.1.200 and 6.4.299. test/ui/checkpair.test.mjs
is red on the worker as published.
