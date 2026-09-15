// The accessibility report — `PLAN-accessibility.md` P07.S06, law 4.
//
// ── What only this tier can see ──────────────────────────────────────────────
// The server test proves the verdicts travel as words. This proves what a PERSON sees: that a clause
// nib could not check is never drawn the way a pass is, and that the summary never claims
// conformance while any clause is unchecked. Both are the collapse law 4 forbids, arriving through
// the rendering instead of the checker.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';
import { setNextDocument } from './stub-pdfjs.mjs';

let nextReport = null;

const { document: doc, settle, calls } = await boot({
  routes: {
    '/api/open': {
      id: 'ua:1', name: 'doc.pdf', path: '/tmp/nib-harness/doc.pdf',
      canSave: true, canUndo: false, canRedo: false, signature: { state: 'unsigned' },
    },
    '/api/scan': { hidden: [] },
    '/api/uacheck': () => nextReport,
  },
});

async function openDoc() {
  setNextDocument({ numPages: 1 });
  doc.getElementById('pathInput').value = '/tmp/nib-harness/doc.pdf';
  doc.getElementById('openGo').click();
  await settle();
}

async function showReport(report) {
  nextReport = report;
  doc.getElementById('uaBtn').click();
  await settle();
  await settle();
}

const row = (clause) => [...doc.querySelectorAll('#uaBody .ua-row')].find((r) => r.textContent.includes(clause));

test('the button reaches the report route, and the modal opens', async () => {
  await openDoc();
  await showReport({ conformant: false, results: [{ clause: '7.1 t11', summary: 's', verdict: 'fail', why: 'no tree' }], refusals: ['7.1 t11 fails: no tree'] });
  assert.ok(calls.some((c) => c.url.includes('/api/uacheck')), 'the button did not ask the server — the UI would be deciding conformance itself');
  assert.equal(doc.getElementById('uaModal').hidden, false, 'the report modal did not open');
});

test('the report shows the server\'s sentence about where the structure came from', async () => {
  const sentence = 'This document\'s structure was read from a scan by text recognition (OCR).';
  await showReport({ conformant: false, results: [{ clause: '7.1 t11', summary: 's', verdict: 'fail', why: 'no tree' }], refusals: ['x'], structure: sentence });
  const shown = doc.querySelector('#uaBody .ua-provenance');
  assert.ok(shown, 'the report shows no provenance line — D4 says the user is told which tier produced the tree');
  assert.equal(shown.textContent, sentence, 'the provenance line is not the server\'s own sentence');
});

test('a clause nib could not check is never drawn the way a pass is', async () => {
  await showReport({
    conformant: false,
    results: [
      { clause: '7.1 t11', summary: 'tree', verdict: 'pass' },
      { clause: '7.21.7 t1', summary: 'unicode', verdict: 'cannot check', why: 'an unmeasured font', where: 'page 1' },
      { clause: '7.18.4 t1', summary: 'forms', verdict: 'not applicable', why: 'no widgets' },
      { clause: '7.1 t3', summary: 'tagged', verdict: 'fail', why: 'untagged text', where: 'page 1' },
    ],
    refusals: ['7.1 t3 fails: untagged text (page 1)', '7.21.7 t1 could not be checked: an unmeasured font (page 1)'],
  });
  const pass = row('7.1 t11');
  const cannot = row('7.21.7 t1');
  assert.ok(pass && cannot, 'the rows were not rendered');
  // Stimulus first: the two rows exist and are different elements.
  assert.notEqual(pass, cannot);
  assert.notEqual(pass.className, cannot.className,
    'a cannot-check row carries the same classes as a pass — law 4\'s third verdict styled as its first');
  const verdictText = (r) => r.querySelector('.ua-verdict').textContent;
  assert.notEqual(verdictText(pass), verdictText(cannot), 'the words distinguishing them are identical');
  assert.ok(!verdictText(cannot).includes('✓'), 'a cannot-check row shows a tick');
  assert.ok(cannot.textContent.includes('an unmeasured font'), 'the cannot-check row does not say why');
  // Failures are listed first, so the first thing a person reads is a thing to fix.
  const first = doc.querySelector('#uaBody .ua-row');
  assert.ok(first.textContent.includes('7.1 t3'), 'a failure is not the first row');
});

test('the summary never says the document conforms while a clause is unchecked', async () => {
  await showReport({
    conformant: false,
    results: [
      { clause: '7.1 t11', summary: 'tree', verdict: 'pass' },
      { clause: '7.21.7 t1', summary: 'unicode', verdict: 'cannot check', why: 'unmeasured' },
    ],
    refusals: ['7.21.7 t1 could not be checked: unmeasured'],
  });
  const summary = doc.getElementById('uaSummary').textContent;
  assert.ok(!/passes|conform/i.test(summary) || /not/i.test(summary),
    `the summary reads "${summary}" for a report with an unchecked clause and no failure`);
  assert.ok(summary.includes('could not check'), `the summary does not name the unchecked clause: "${summary}"`);
  // And the door's own refusal sentences are shown, not recomposed.
  assert.ok(doc.querySelector('#uaBody .ua-refusals').textContent.includes('7.21.7 t1 could not be checked'),
    'the refusals from the door are not shown');
});

test('only a conformant report from the server is summarised as passing', async () => {
  await showReport({ conformant: true, results: [{ clause: '7.1 t11', summary: 'tree', verdict: 'pass' }], refusals: [] });
  const passing = doc.getElementById('uaSummary').textContent;
  assert.ok(passing.startsWith('Every clause Nib checks passes'), `the passing summary reads "${passing}"`);
  // A document can pass every clause nib checks and fail veraPDF: measured, a paragraph tagged /Formula
  // with no alternate text fails 7.7 t1 (internal/uacheck/counterexample_test.go).
  // So even the best summary must say it is not a certificate, and must never say "conforms" or
  // "is PDF/UA" as a claim.
  assert.ok(/not a conformance certificate/.test(passing),
    `the passing summary "${passing}" does not say it is not a certificate — nib checks part of PDF/UA`);
  assert.ok(!/\bconforms\b|\bis PDF\/UA\b/i.test(passing), `the passing summary claims conformance: "${passing}"`);
  await showReport({ conformant: false, results: [{ clause: '7.1 t11', summary: 'tree', verdict: 'pass' }], refusals: ['x'] });
  assert.ok(doc.getElementById('uaSummary').textContent.startsWith('Not PDF/UA'),
    'the summary followed the rows instead of the server\'s conformance');
});
