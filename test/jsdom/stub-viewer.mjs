// Tier-2 stub for the vendored pdf.js viewer (`web/vendor/pdfjs/pdf_viewer.mjs`).
//
// Five classes, which is exactly what `web/app.js` imports from it. The viewer is
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
    this.currentPageNumber = 1;
    this.currentScaleValue = null;
    this.setDocumentCalls = [];
    this._editorMode = { mode: 0 };
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
