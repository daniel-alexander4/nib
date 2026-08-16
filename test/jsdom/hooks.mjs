// Module resolve hook: redirect the two vendored pdf.js specifiers to the tier-2
// stubs, and nothing else.
//
// "And nothing else" is the contract, not a detail. `web/app.js` also imports
// `./detect.js` (nib's own, pure), `diff.min.mjs` and `pixelmatch.mjs` (pure ESM,
// both load in Node unmodified) — and those must load FOR REAL. A hook that
// over-matched would quietly swap real code for fakes while the suite went on
// reporting coverage of `app.js`, which is the exact shape of a harness that
// observes nothing.
//
// Hence the suffix match on the full vendor path rather than a bare filename:
// `endsWith('pdf.min.mjs')` would also catch anything else ending that way.
const STUBS = {
  'vendor/pdfjs/pdf.min.mjs': './stub-pdfjs.mjs',
  'vendor/pdfjs/pdf_viewer.mjs': './stub-viewer.mjs',
};

export async function resolve(specifier, context, next) {
  for (const [tail, stub] of Object.entries(STUBS)) {
    if (specifier.endsWith(tail)) {
      return { url: new URL(stub, import.meta.url).href, shortCircuit: true };
    }
  }
  return next(specifier, context);
}
