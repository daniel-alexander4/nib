// The Document language field — `/pending 471`, Dan's option B: pre-filled from this computer's
// language, changeable or clearable, and sent with the Open dialog's open as `lang` when it names one. The field moved
// into the Open dialog when Import… folded into Open… (ADR-215), and it rides on both of the dialog's routes.
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
const stop = (opts) => {
  sent = JSON.parse(opts.body);
  return new Response(JSON.stringify({ error: 'stop here' }), { status: 400, headers: { 'Content-Type': 'application/json' } });
};
const h = await boot({
  routes: {
    '/api/open': stop, '/api/open-url': stop,
    '/api/listdir': { path: '/home/someone', parent: '/home', dirs: [], files: [{ name: 'brief.docx', path: '/home/someone/brief.docx' }] },
  },
});
const { document: doc, settle } = h;
const field = doc.getElementById('docLang');

// convert opens `value` from the Open dialog, typed, the way a person would, and returns the body it posted.
async function convert(value = '/home/someone/notes.md') {
  sent = null;
  doc.getElementById('openMenuItem').click();
  await settle();
  doc.getElementById('pathInput').value = value;
  doc.getElementById('openGo').click();
  await settle();
  // Stimulus before response: the flow must actually have posted, or "no lang" below is vacuous.
  assert.ok(sent && typeof sent === 'object', 'the Open dialog never posted ' + value);
  return sent;
}

test('the field is pre-filled from this computer\'s language', () => {
  assert.ok(field, 'no #docLang in index.html');
  assert.ok(field.closest('#openModal'), 'the language field is not in the Open dialog, the one place a conversion starts');
  assert.equal(h.window.navigator.language, 'en-US', 'jsdom\'s locale changed, so this row no longer says what it checks');
  assert.equal(field.value, 'en');
});

test('the pre-fill is sent as a language but never as one the user chose', async () => {
  // Before any change event: the value is the pre-fill. /pending 486 rests a conformance claim only on
  // a language someone picked, so this must not say "chosen".
  const prefilled = await convert();
  assert.equal(prefilled.path, '/home/someone/notes.md', 'control: the path was not what was opened');
  assert.equal(prefilled.lang, 'en', 'control: the pre-filled language is still sent');
  assert.equal(prefilled.langChosen, false, 'the pre-fill was reported as the user\'s choice');
});

test('converting sends the chosen language, and nothing when it is cleared', async () => {
  field.value = 'de';
  field.dispatchEvent(new h.window.Event('change'));
  const chosen = await convert();
  assert.equal(chosen.lang, 'de');
  assert.equal(chosen.langChosen, true, 'a language the user picked was not reported as chosen');
  const byURL = await convert('https://example.org/notes.md');
  assert.equal(byURL.url, 'https://example.org/notes.md', 'control: the URL was not what was opened');
  assert.equal(byURL.lang, 'de', 'a URL opened from the dialog dropped the language beside it');
  assert.equal(byURL.langChosen, true, 'a URL opened from the dialog dropped the choice');
  field.value = '';
  field.dispatchEvent(new h.window.Event('change'));
  const cleared = await convert();
  assert.equal(cleared.lang, '', 'a cleared field still sent a language');
  assert.equal(cleared.langChosen, false, 'a cleared field still claimed a chosen language');
  assert.ok(cleared.path, 'control: the path is still sent');
});

test('a file picked from the dialog\'s list carries the language too, not only a typed path', async () => {
  field.value = 'fr';
  field.dispatchEvent(new h.window.Event('change'));
  sent = null;
  doc.getElementById('openMenuItem').click();
  await settle();
  const row = [...doc.querySelectorAll('#openList li.file')].find((li) => li.textContent === 'brief.docx');
  assert.ok(row, 'setup: the listed file has no row in the Open dialog');
  row.click();
  await settle();
  assert.ok(sent, 'picking the row opened nothing');
  assert.equal(sent.path, '/home/someone/brief.docx');
  assert.equal(sent.lang, 'fr', 'a file picked from the list dropped the language beside it');
  assert.equal(sent.langChosen, true);
});
