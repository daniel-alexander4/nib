// Tier-2 stub for the vendored pdf.js viewer (`web/vendor/pdfjs/pdf_viewer.mjs`).
//
// Five classes and one enum, which is exactly what `web/app.js` imports from it. The viewer is
// the rendering engine, and rendering is precisely where this tier stops — so
// stubbing it is not a shortcut, it IS the tier boundary (see boot.mjs).
//
// One deliberate exception: EventBus is REAL, not inert. app.js drives a lot of
// its own behaviour through it (`pagesinit`, `pagesloaded`, `pagechanging`,
// `updatefindcontrolstate`), so a stub that swallowed dispatches would make those
// paths untestable while still booting — a harness that runs and observes nothing,
// which is the failure this whole phase exists to prevent.
export class EventBus {
  constructor() {
    this._listeners = new Map();
  }
  on(name, fn) {
    if (!this._listeners.has(name)) this._listeners.set(name, []);
    this._listeners.get(name).push(fn);
  }
  off(name, fn) {
    const a = this._listeners.get(name);
    if (!a) return;
    const i = a.indexOf(fn);
    if (i >= 0) a.splice(i, 1);
  }
  // Copied before iterating: a listener that unsubscribes during dispatch is a
  // real pattern in app.js, and mutating the live array would skip its neighbour.
  dispatch(name, data = {}) {
    for (const fn of (this._listeners.get(name) || []).slice()) {
      fn({ ...data, source: this });
    }
  }
}

// Inert, but observable: tests assert what app.js asked the viewer to do
// (setDocument(null) on a close, the editor mode it set) rather than what was
// painted, which jsdom could not tell them anyway.
export class PDFViewer {
  constructor(opts = {}) {
    Object.assign(this, opts);
    this.pdfDocument = null;
    this._pageNumber = 1;
    this.currentScaleValue = null;
    this.setDocumentCalls = [];
    this._editorMode = { mode: 0 };
  }
  // **`currentPageNumber` dispatches `pagechanging`, because the real PDFViewer does.**
  // It was a plain property until v1.129.7, so setting it here fired nothing — which made every
  // app behaviour hung off a page change invisible at this tier, including the one that stops
  // read-aloud at the page boundary (`/pending 408`). A stub that silently drops an event the real
  // component emits does not merely under-test: it reports the app as inert.
  //
  // Guarded on an actual change and on the bus existing, so it matches the real one's behaviour
  // rather than shouting on every assignment during construction.
  get currentPageNumber() { return this._pageNumber; }
  set currentPageNumber(n) {
    const changed = n !== this._pageNumber;
    this._pageNumber = n;
    if (changed && this.eventBus) this.eventBus.dispatch('pagechanging', { pageNumber: n });
  }
  setDocument(doc) {
    this.pdfDocument = doc;
    this.setDocumentCalls.push(doc);
  }
  getPageView() { return null; }
  refresh() {}
  set annotationEditorMode(v) { this._editorMode = v; }
  get annotationEditorMode() { return this._editorMode; }
}

export class PDFLinkService {
  constructor(opts = {}) { Object.assign(this, opts); this.pdfDocument = null; }
  setDocument(doc) { this.pdfDocument = doc; }
  setViewer() {}
  goToDestination() {}
}

export class PDFFindController {
  constructor(opts = {}) { Object.assign(this, opts); }
  setDocument() {}
}

export class GenericL10n {
  constructor() {}
  async get(_key, _args, fallback) { return fallback; }
}

// ScrollMode mirrors the vendored enum's values, which `app.js` sets on the viewer to drive the
// layout modes (v1.129.6). The numbers are pdf.js's own and are checked against the real file by
// `TestTheScrollModeStubMatchesTheVendoredEnum` at tier 1 — a stub whose constants drift from the
// thing it stands for is a harness that agrees with itself and with nothing else.
export const ScrollMode = { UNKNOWN: -1, VERTICAL: 0, HORIZONTAL: 1, WRAPPED: 2, PAGE: 3 };
