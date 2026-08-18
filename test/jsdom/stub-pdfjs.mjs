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

// ── The document ─────────────────────────────────────────────────────────────
// Booting is not opening. Resolving to `null` is enough for app.js to evaluate
// (which is all P02.S01 needed), but the open path assigns the result to
// pdfDocument and immediately reads `pdfDocument.numPages` to fill the page count
// (app.js:1245) — so a null document throws there.
//
// The surface below is sized by what the open path actually touches, not by what
// a PDF has: numPages, getPage (thumbnails), getOutline (the outline sidebar),
// annotationStorage (the unsaved-edit signal), and loadingTask.destroy (teardown).
// Same discipline as the seven symbols above — complete by construction, so a new
// dependency fails loudly by name rather than silently returning undefined.
//
// getPage's viewport is real arithmetic rather than a constant: buildThumbnails
// scales by `150 / base.width`, and a viewport that lied about its width would
// make that scaling untestable. What it CANNOT do is render — jsdom has no canvas, so
// `render()` rejects and buildThumbnails ends in its caller's .catch.
//
// It does NOT leave the grid empty, which this comment claimed until P05.S05 measured it.
// buildThumbnails appends the wrapper BEFORE awaiting the render, so exactly ONE
// `.thumbwrap` lands before the rejection unwinds the loop. The ceiling is real — a
// thumbnail COUNT is a tier-3 assertion, because only one page ever renders here — but
// "the grid is empty" was a false premise, and an emptiness assertion written against it
// would have passed for the wrong reason.
function makePage(n) {
  return {
    pageNumber: n,
    getViewport({ scale = 1 } = {}) {
      return { width: 612 * scale, height: 792 * scale, scale, rotation: 0 };
    },
    render() {
      return { promise: Promise.reject(new Error('jsdom has no canvas — rendering is tier 3')) };
    },
    getTextContent: async () => ({ items: [] }),
    getAnnotations: async () => [],
  };
}

let nextDocument = null;

// The most recently created document, so a test can reach its annotationStorage — the
// app installs `onSetModified` there as its pdf.js-edit signal (it read `.size` until
// v1.108.7, which could not tell an edited value from an unchanged one), and there is no
// other way in from outside the module.
export let lastDocument = null;

// setNextDocument configures what the next getDocument() resolves to. Tests call
// it to open a document of a known size; passing null restores the boot-only
// behaviour S01 relied on.
export function setNextDocument(opts) {
  nextDocument = opts === null ? null : { numPages: 3, outline: null, ...opts };
}

export function getDocument() {
  const cfg = nextDocument;
  const task = { destroy: async () => {} };
  if (cfg === null) {
    return { promise: Promise.resolve(null), destroy: async () => {} };
  }
  const doc = {
    numPages: cfg.numPages,
    loadingTask: task,
    // A real Map SUBCLASS, because the app no longer reads `.size` — it installs
    // pdf.js's own `onSetModified` callback, which the real AnnotationStorage fires from
    // setValue when a value actually changed. A plain Map would silently never fire it,
    // and the "a form fill prompts on close" test would go green against a signal that
    // does not exist. Modelled on the real contract rather than on what the test needs:
    // `set` fires only on a genuine change, and `resetModified` is here because the real
    // API has it and a stub that answers half an interface invites a caller to use the
    // half that is missing.
    annotationStorage: new (class extends Map {
      onSetModified = null;
      #modified = false;
      set(k, v) {
        const changed = !this.has(k) || JSON.stringify(this.get(k)) !== JSON.stringify(v);
        super.set(k, v);
        if (changed && !this.#modified) { this.#modified = true; this.onSetModified?.(); }
        return this;
      }
      resetModified() { this.#modified = false; }
    })(),
    getPage: async (n) => makePage(n),
    getOutline: async () => cfg.outline,
    getData: async () => new Uint8Array(),
    saveDocument: async () => new Uint8Array(),
    destroy: async () => {},
  };
  lastDocument = doc;
  task.promise = Promise.resolve(doc);
  return task;
}
