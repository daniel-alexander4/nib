// The Document language field — `/pending 471`, Dan's option B: pre-filled from this computer's
// language, changeable or clearable, and sent to /api/office as `lang` when it names one.
//
// ── What this tier can see, and what it cannot ───────────────────────────────
// It sees the choice (`docLangForLocale`, tabled directly) and the request the convert flow builds.
// It cannot see the server declare it — that is `internal/server/officelang_test.go` — and jsdom's
// navigator.language is fixed at "en-US", so the boot row proves the pre-fill is WIRED, while the
// table proves what it picks.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';
import { docLangForLocale } from '../../web/doclang.js';

const OPTIONS = ['', 'en', 'de', 'pt', 'sr', 'zh-Hans', 'zh-Hant', 'ja'];

test('the pre-fill picks the exact tag, then the primary subtag, else nothing', () => {
  for (const [locale, want, why] of [
    ['de', 'de', 'an exact tag'],
    ['de-AT', 'de', 'a region the list does not carry falls back to its language'],
    ['EN-us', 'en', 'case does not matter'],
    ['pt_BR', 'pt', 'an underscore locale, as some platforms report it'],
    ['sr-Latn-RS', 'sr', 'a script and a region'],
    ['zh-TW', 'zh-Hant', 'Taiwan reads Traditional'],
    ['zh-HK', 'zh-Hant', 'Hong Kong reads Traditional'],
    ['zh-Hant', 'zh-Hant', 'an explicit script'],
    ['zh-CN', 'zh-Hans', 'mainland China reads Simplified'],
    ['zh', 'zh-Hans', 'bare Chinese defaults to Simplified'],
    ['xx', '', 'a language the list does not carry declares nothing'],
    ['', '', 'no locale at all'],
    [undefined, '', 'navigator.language missing'],
  ]) {
    assert.equal(docLangForLocale(locale, OPTIONS), want, `${String(locale)}: ${why}`);
  }
});

let sent = null;
const h = await boot({
  routes: {
    '/api/office': (opts) => {
      sent = opts && opts.body;
      return new Response(JSON.stringify({ error: 'stop here' }), { status: 400, headers: { 'Content-Type': 'application/json' } });
    },
  },
});
const { document: doc, settle } = h;
const field = doc.getElementById('docLang');
const input = doc.getElementById('officeInput');

async function convert() {
  sent = null;
  Object.defineProperty(input, 'files', { value: [new h.window.File(['# Notes\n'], 'notes.md')], configurable: true });
  input.dispatchEvent(new h.window.Event('change'));
  await settle();
  // Stimulus before response: the flow must actually have posted, or "no lang" below is vacuous.
  assert.ok(sent instanceof h.window.FormData, 'the convert flow never posted to /api/office');
  return sent;
}

test('the field is pre-filled from this computer\'s language', () => {
  assert.ok(field, 'no #docLang in index.html');
  assert.equal(h.window.navigator.language, 'en-US', 'jsdom\'s locale changed, so this row no longer says what it checks');
  assert.equal(field.value, 'en');
});

test('converting sends the chosen language, and nothing when it is cleared', async () => {
  field.value = 'de';
  assert.equal((await convert()).get('lang'), 'de');
  field.value = '';
  const cleared = await convert();
  assert.equal(cleared.get('lang'), null, 'a cleared field still sent a language');
  assert.ok(cleared.get('file'), 'control: the file is still sent');
});
