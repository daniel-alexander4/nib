// P02.S04 — what this machine observed about the spoken check, on the card (D5).
//
// **The case that matters is the ABSENCE.** No note means an older build, a failed write, or a
// ceremony whose hop has not happened — never "the modal did not appear". A card that rendered
// absence as a negative would accuse a party of skipping a check they performed, and it would do
// it silently, on every ceremony that predates this version.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';

const ID = 'a'.repeat(32);
let listing = {};

const { document: doc, settle } = await boot({
  routes: {
    '/api/ceremonies': () => listing,
    '/api/ceremony/next': () => ({ ceremony: ID, state: 'unavailable', reason: 'no record' }),
  },
});

async function render(verification) {
  listing = {
    primary: true,
    ceremonies: [{ id: ID, state: 'absent', me: 'b'.repeat(64), reason: 'nothing yet',
      ...(verification ? { verification } : {}) }],
  };
  doc.querySelector('.modetab[data-tab="collaborate"]').click();
  await settle();
  const el = doc.querySelector('.cerverify');
  return el ? el.textContent : null;
}

test('a ceremony with no note says nothing about the spoken check', async () => {
  const got = await render(null);
  assert.equal(got, null,
    `the card says ${JSON.stringify(got)} about a ceremony carrying no note. Absence is UNKNOWN — `
    + 'an older build, a failed write, or a hop that has not happened — and rendering it as a '
    + 'statement accuses a party of skipping a check they may well have performed');
});

test('a confirmed check reads as confirmed', async () => {
  const got = await render({ presented: true, confirmed: true, at: new Date().toISOString() });
  assert.match(got || '', /confirmed the spoken words/i,
    'a ceremony whose spoken check was confirmed does not say so');
});

test('presented and unconfirmed is distinguishable from never presented', async () => {
  const unanswered = await render({ presented: true, confirmed: false, at: new Date().toISOString() });
  const never = await render({ presented: false, confirmed: false, at: new Date().toISOString() });
  assert.ok(unanswered && never, 'one of the two states rendered nothing at all');
  assert.notEqual(unanswered, never,
    `"presented and not confirmed" and "never presented" both read as ${JSON.stringify(never)}. `
    + 'They are different facts and the acceptance clause asks for three states, not two — one is '
    + 'a person who saw the words and did not confirm, the other is a machine that showed nobody '
    + 'anything');
  assert.match(never, /did not appear/i, 'the never-presented case does not say the words never appeared');
});
