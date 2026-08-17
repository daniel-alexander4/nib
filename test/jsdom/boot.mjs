// Tier 2 of Nib's three-tier test harness: load the REAL `web/app.js` into a
// jsdom document built from the REAL `web/index.html`, and drive it through
// events the way a browser would.
//
// ── What this tier reaches ───────────────────────────────────────────────────
// Everything that is observable as DOM state or module behaviour: which controls
// are enabled, what a teardown resets, whether a mode is still armed, what the
// app asked the pdf.js viewer to do. It runs the shipped file — not a copy, not
// an extract — so a test here cannot pass against code that is not in the binary.
//
// ── Where it stops, and who covers the gap ───────────────────────────────────
// jsdom models the DOM. It does not model a rendering engine, and this is not an
// incidental gap to work around — it is the boundary this tier is defined by:
//
//   * no layout: every `clientWidth` / `getBoundingClientRect()` is 0, so the
//     fit-widest-page path and anything measuring a page div is unobservable here
//     (`fitWidestWidth` silently no-ops on a zero-width container — a real bug
//     class in this repo, v1.79.3 / v1.80.2);
//   * no canvas: thumbnails, the redaction bake, the compare pixel map;
//   * no media queries: jsdom implements no `matchMedia` AT ALL (polyfilled inert
//     below), so the device-pixel-ratio heal is unobservable;
//   * no real pdf.js: the engine is stubbed (see stub-pdfjs.mjs / stub-viewer.mjs),
//     so nothing that depends on a parsed document reaches its real behaviour.
//
// **Tier 3 (`build/uirepro.sh`) covers every one of those** by driving a real
// browser against the real binary. A gap named here is a delegation, not a
// silence — that is the point of writing the ceiling next to the harness rather
// than only in the plan.
//
// Tier 1 is the Go suite (`go test ./...`), which sees the server and never the
// client's state.
// ── One boot per process ─────────────────────────────────────────────────────
// `web/app.js` runs its wiring at module-evaluation time, and ES modules are
// cached per process — so a second boot() in the same process gets the cached
// module bound to the FIRST document, and every assertion after it would be read
// off a stale DOM while still passing. `node --test` runs each test FILE in its
// own process, so the rule is simply: one boot per file. Stated here because the
// failure it prevents is silent.
import { register } from 'node:module';
import { JSDOM } from 'jsdom';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const HERE = path.dirname(fileURLToPath(import.meta.url));
export const REPO = path.resolve(HERE, '..', '..');

// Response bodies for the endpoints the boot path actually touches, shaped to
// match the Go handlers. Shapes matter: an early version of this harness returned
// `{}` for everything and app.js threw `libraryImages is not iterable`, because
// /api/images returns an array. A stub whose shapes drift from the handlers tests
// the app against a server that does not exist.
//
// Anything NOT listed here throws by name rather than returning a bland default —
// so a new endpoint on the boot path shows up as a loud failure instead of a
// silent `{}` that the app mis-parses later.
const BOOT_ROUTES = {
  '/api/status': { state: 'ready', csrf: 'test-csrf', version: 'test', autoUpdate: false, updateCheckLocked: false, ghostscript: false, libreoffice: false },
  '/api/images': [],
  '/api/recent': [],
  '/api/doc': { name: '', path: '', canSave: false, signature: { state: '' }, canUndo: false, canRedo: false },
  // Close-view answers with whatever is active AFTER the close — the zero response when
  // nothing is left, which is what this default is. A test that needs the neighbour
  // named passes its own.
  '/api/close-view': { name: '', path: '', canSave: false, signature: { state: '' }, canUndo: false, canRedo: false },
};

// boot builds the document, installs the globals app.js needs, and imports the
// real module. Returns the jsdom window plus the recorded fetch calls, so a test
// can assert what the app asked the server for as well as what it rendered.
export async function boot({ routes = {}, search = '' } = {}) {
  register('./hooks.mjs', import.meta.url);

  const html = fs.readFileSync(path.join(REPO, 'web', 'index.html'), 'utf8');
  const dom = new JSDOM(html, {
    url: 'http://127.0.0.1:65000/' + search,
    pretendToBeVisual: true,
  });

  // app.js is written for a browser, so the browser's globals have to exist
  // before it is imported — it touches many of them at module-evaluation time
  // (428 element lookups, ~220 handler assignments, and a `refreshStatus()` call
  // on its last line).
  const GLOBALS = [
    'window', 'document', 'navigator', 'location', 'history',
    'HTMLElement', 'HTMLInputElement', 'HTMLCanvasElement', 'Element', 'Node',
    'Event', 'CustomEvent', 'KeyboardEvent', 'MouseEvent', 'PointerEvent',
    'getComputedStyle', 'requestAnimationFrame', 'cancelAnimationFrame',
    'localStorage', 'sessionStorage', 'FormData', 'Blob', 'File', 'FileReader',
    'DOMParser', 'XMLSerializer', 'Image', 'URL', 'URLSearchParams',
    'devicePixelRatio', 'MutationObserver',
  ];
  // NOT setTimeout/clearTimeout: jsdom's window timers delegate to the global
  // ones, so copying them onto globalThis makes setTimeout call itself and the
  // boot dies in infinite recursion. Node's own timers are what app.js should
  // get. (Found by running it — the stack is 200 frames of jsdom Window.js.)
  for (const k of GLOBALS) {
    if (dom.window[k] !== undefined) globalThis[k] = dom.window[k];
  }

  // jsdom implements no media queries at all — see the ceiling above. Inert is
  // the honest shape: a polyfill that pretended to answer queries would let a
  // test assert a media-dependent behaviour this tier cannot actually observe.
  globalThis.matchMedia = (query) => ({
    media: query,
    matches: false,
    onchange: null,
    addEventListener() {}, removeEventListener() {},
    addListener() {}, removeListener() {},
    dispatchEvent() { return false; },
  });

  // jsdom stubs confirm() as "not implemented" and app.js calls it bare, so the
  // harness owns it. Recorded rather than merely answered: a test that asserts
  // "closing a clean document does not prompt" needs to know the call COUNT, not
  // just the return value — otherwise it passes against a confirm nobody called
  // for the wrong reason.
  const confirms = [];
  let confirmAnswer = true;
  globalThis.confirm = (message) => { confirms.push(message); return confirmAnswer; };
  globalThis.alert = () => {};

  const calls = [];
  const table = { ...BOOT_ROUTES, ...routes };
  globalThis.fetch = async (url, opts = {}) => {
    const u = String(url);
    // Headers are recorded, not just the URL: P03.S03's guard asserts that every
    // document-route call carries X-Nib-Doc, and an unpinned call differs from a
    // pinned one only by an absent header.
    calls.push({ url: u, method: opts.method || 'GET', headers: { ...(opts.headers || {}) } });
    const key = Object.keys(table).find((k) => u.split('?')[0].endsWith(k));
    if (key === undefined) {
      throw new Error(
        `jsdom harness: no stubbed response for ${opts.method || 'GET'} ${u}. ` +
        'Add it to BOOT_ROUTES (shaped like the Go handler) or pass it via routes.',
      );
    }
    const body = typeof table[key] === 'function' ? table[key](opts) : table[key];
    return new Response(JSON.stringify(body), {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    });
  };

  // pdf_viewer.mjs destructures `globalThis.pdfjsLib` rather than importing it,
  // so resolving the specifier is not enough — the global has to be there too.
  // Invisible from the import graph; found only by running it.
  globalThis.pdfjsLib = await import('./stub-pdfjs.mjs');

  const rejections = [];
  const onRejection = (e) => rejections.push(e);
  process.on('unhandledRejection', onRejection);

  await import(path.join(REPO, 'web', 'app.js'));
  // Let the boot's own async work (refreshStatus -> applyStatus -> loadImages)
  // settle before a test reads the DOM.
  await new Promise((r) => setTimeout(r, 50));
  process.off('unhandledRejection', onRejection);

  return {
    dom,
    window: dom.window,
    document: dom.window.document,
    calls,
    rejections,
    confirms,
    setConfirmAnswer(v) { confirmAnswer = v; },
    // settle waits for app.js's async work — a fetch round-trip and the DOM
    // writes that follow it — by DRAINING THE EVENT LOOP rather than sleeping a
    // wall-clock interval.
    //
    // That distinction is the difference between a deterministic test and a flaky
    // one. Every await in this harness resolves against a stubbed fetch, so the
    // work is queued microtasks and immediates, not real I/O; a fixed `setTimeout`
    // would be a guess that happens to be long enough on this machine and might
    // not be on a loaded one. Draining ticks finishes as soon as the queue is
    // empty and cannot be too short.
    settle: async (ticks = 12) => {
      for (let i = 0; i < ticks; i++) {
        await new Promise((r) => setImmediate(r));
      }
    },
  };
}
