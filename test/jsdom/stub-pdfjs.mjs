// Tier-2 stub for pdf.js (`web/vendor/pdfjs/pdf.min.mjs`).
//
// The surface here is not invented and not minimal-by-guess: it is every
// `pdfjsLib.X` that `web/app.js` actually references — seven symbols, enumerated
// from the source. That matters for a reason specific to stubs: a stub that is
// merely "enough to boot" grows a silent gap the moment app.js reaches for
// something new, and the failure then looks like a test bug rather than a missing
// dependency. Because this one is complete by construction, a new reference fails
// loudly as a TypeError naming the missing member (inventory row P5).
//
// Re-derive it, don't extend it by hand, if app.js changes:
//   grep -o 'pdfjsLib\.[A-Za-z_][A-Za-z0-9_]*' web/app.js | sort -u
export const GlobalWorkerOptions = { workerSrc: '' };

// Only NONE is referenced by name; the rest are reached dynamically as
// AnnotationEditorType[activeTool], where activeTool is a data-mode string.
export const AnnotationEditorType = {
  NONE: 0,
  FREETEXT: 3,
  HIGHLIGHT: 9,
  INK: 15,
  STAMP: 13,
};

export const AnnotationMode = { DISABLE: 0, ENABLE: 1, ENABLE_FORMS: 2, ENABLE_STORAGE: 3 };
export const AnnotationEditorParamsType = { HIGHLIGHT_COLOR: 31 };

// Real values: nib divides by PDF_TO_CSS_UNITS in the fit-width path, and getting
// it wrong is not hypothetical — a missing divisor zoomed every document by 4/3
// (v1.80.2). A stub that returned 1 here would make that class of bug untestable.
export const PixelsPerInch = { PDF: 72, CSS: 96, PDF_TO_CSS_UNITS: 96 / 72 };

export const Util = {
  normalizeRect: (r) => r,
  transform: (a) => a,
};

// A document that resolves to null: enough for the open path to be driven up to
// the point where rendering would begin, which is where this tier stops.
export function getDocument() {
  return {
    promise: Promise.resolve(null),
    destroy: async () => {},
  };
}
