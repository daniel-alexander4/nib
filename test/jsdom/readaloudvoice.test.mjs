// Read aloud's voice and speed — /pending 482.
//
// ── What only this tier can see ──────────────────────────────────────────────
// The Go tests prove the choice is stored and carried to the client. They cannot see what the
// client does with it against a browser whose voice list arrives late and differs per machine,
// which is the whole of the item's three hazards. So the Web Speech API is stubbed here, installed
// BEFORE boot so the app can subscribe to `voiceschanged`, and what is asserted is what Nib asks it
// to say and in which voice.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';
import { setNextDocument } from './stub-pdfjs.mjs';

const DOC = '/tmp/nib-harness/voice.pdf';
const SAMANTHA = { name: 'Samantha', lang: 'en-US' };
const DANIEL = { name: 'Daniel', lang: 'en-GB' };

// Voices are empty until the engine "loads", as Chromium's are.
let voices = [];
const listeners = [];
const spoken = [];
class FakeUtterance { constructor(text) { this.text = text; } }
const fakeSpeech = {
  getVoices: () => voices,
  addEventListener: (type, fn) => { if (type === 'voiceschanged') listeners.push(fn); },
  speak(u) { spoken.push(u); },
  cancel() {},
};
globalThis.speechSynthesis = fakeSpeech;
globalThis.SpeechSynthesisUtterance = FakeUtterance;

const saved = [];
const h = await boot({
  routes: {
    '/api/status': {
      state: 'ready', csrf: 'test-csrf', version: 'test', autoUpdate: false, updateCheckLocked: false,
      ghostscript: false, libreoffice: false,
      advanced: { ceremony: true, discovery: true, rendezvous: true, timestamp: true },
      readAloudVoice: 'Daniel', readAloudRate: 1.25,
    },
    '/api/settings': (opts) => { saved.push(JSON.parse(opts.body)); return { status: 'ok' }; },
    '/api/open': () => ({
      name: 'voice.pdf', path: DOC, canSave: true,
      signature: { state: 'unsigned' }, canUndo: false, canRedo: false,
    }),
  },
});
const { document: doc, window: win, settle } = h;
win.speechSynthesis = fakeSpeech;
win.SpeechSynthesisUtterance = FakeUtterance;

const picker = () => doc.getElementById('readAloudVoiceSel');
const names = () => [...picker().options].map((o) => o.value);
function voicesLoad(list) {
  voices = list;
  for (const fn of listeners) fn();
}

test('before the engine loads, the saved voice is shown as unavailable rather than hidden', () => {
  assert.ok(listeners.length > 0, 'nothing subscribed to voiceschanged, so a late-loading voice list never reaches the picker');
  assert.deepEqual(names(), ['', 'Daniel'], `the picker offers ${JSON.stringify(names())}`);
  assert.equal(picker().value, 'Daniel', 'the picker does not show the saved voice');
  assert.match(picker().selectedOptions[0].textContent, /not available/);
});

test('the picker fills when the engine reports its voices', () => {
  voicesLoad([SAMANTHA, DANIEL]);
  assert.deepEqual(names(), ['', 'Daniel', 'Samantha'],
    `after voiceschanged the picker offers ${JSON.stringify(names())} — a list filled only at start-up is empty the first time it is opened`);
  assert.equal(picker().value, 'Daniel');
  assert.equal(doc.getElementById('readAloudRateSel').value, '1.25', 'the saved speed is not shown');
});

test('the picker is refilled when it takes focus, for a list that changed with no event', () => {
  // A browser whose engine loaded before the app subscribed fires no voiceschanged the app hears.
  voices = [SAMANTHA, DANIEL, { name: 'Moira', lang: 'en-IE' }];
  picker().dispatchEvent(new win.Event('focus'));
  assert.ok(names().includes('Moira'),
    `focusing the picker did not refill it: ${JSON.stringify(names())} — a list that loaded before the app listened would stay stale`);
  voicesLoad([SAMANTHA, DANIEL]);
});

test('reading uses the saved voice and speed', async () => {
  setNextDocument({ numPages: 1, outline: null, text: ['a page to read aloud'] });
  doc.getElementById('pathInput').value = DOC;
  doc.getElementById('openGo').click();
  await settle();
  spoken.length = 0;
  doc.getElementById('readAloudBtn').click();
  await settle();
  assert.equal(spoken.length, 1, 'nothing was read');
  assert.equal(spoken[0].voice, DANIEL, 'the utterance does not carry the saved voice');
  assert.equal(spoken[0].rate, 1.25, 'the utterance does not carry the saved speed');
  doc.getElementById('readAloudBtn').click(); // stop
  await settle();
});

test('a saved voice this machine does not have still reads, in the default voice', async () => {
  voicesLoad([SAMANTHA]);
  spoken.length = 0;
  doc.getElementById('readAloudBtn').click();
  await settle();
  assert.equal(spoken.length, 1,
    'read-aloud went quiet because the saved voice is missing — silence is indistinguishable from a broken feature');
  assert.equal(spoken[0].voice, undefined, 'a voice was set that is not the saved one');
  doc.getElementById('readAloudBtn').click();
  await settle();
});

test('choosing a voice or a speed saves it', async () => {
  voicesLoad([SAMANTHA, DANIEL]);
  saved.length = 0;
  picker().value = 'Samantha';
  picker().dispatchEvent(new win.Event('change'));
  const rate = doc.getElementById('readAloudRateSel');
  rate.value = '0.75';
  rate.dispatchEvent(new win.Event('change'));
  await settle();
  assert.deepEqual(saved, [{ readAloudVoice: 'Samantha' }, { readAloudRate: 0.75 }]);
});
