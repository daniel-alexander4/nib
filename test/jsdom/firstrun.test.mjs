// What a first-run user is shown, and what they are shown only once.
//
// **This file exists because first run is the flow with no coverage at all.**
// build/uirepro.sh enrols a key with curl BEFORE the browser opens — deliberately, since
// every document route is behind requireUnlocked and the harness would otherwise only ever
// see the auth overlay — so tier 3 never reaches the setup state. Nothing drove it at tier
// 2 either. It is also the one flow Dan never sees, because his vault has existed for
// months, which is exactly the combination that lets a regression live forever.
//
// The assertions are about PROPERTIES a first-run user depends on, not about which element
// carries them, so they survive a change to how the explanation is presented:
//   * the explanation is shown on first run,
//   * the "lose the key, lose the vault" warning is shown ONCE,
//   * neither appears for a returning user (migrate / key-missing), who knows what Nib is
//     and needs the form, not the pitch.
//
// ── Ceiling ─────────────────────────────────────────────────────────────────
// jsdom has no layout and no hit-testing, so the original defect — the intro overlay at
// z-index 200 SWALLOWING clicks aimed at the setup form beneath it — is not observable
// here. That was measured in a real browser when it was diagnosed. What this tier can
// observe is what is rendered and how many times, which is what the assertions below stick
// to.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';

// A status the server would send on a genuinely fresh machine.
const SETUP = {
  state: 'setup',
  candidates: ['/home/u/.ssh/id_ed25519'],
  defaultKeyPath: '/home/u/.ssh/id_ed25519',
  version: 'test',
};
let status = SETUP;

// The enroll route answers with the session's NEW status, and app.js hands that straight
// to applyStatus — which is how a state transition actually reaches the client, and the
// only re-render this tier can drive. boot() is once per process (see boot.mjs), so a
// second state cannot be reached by booting again.
let afterEnroll = SETUP;
const h = await boot({
  routes: {
    '/api/status': () => status,
    '/api/ssh/enroll': () => afterEnroll,
  },
});
const { document: doc, settle } = h;

// The sentence a first-run user must not miss, in whatever element carries it. Matched on
// a distinctive phrase rather than an id, so moving it between elements does not silently
// stop this checking anything.
const WARNING = /only way in/i;
const EXPLANATION = /SSH key/i;

// visibleText is what a user could actually read: the text of every element that is not
// itself hidden and has no hidden ancestor. `hidden` is how this app shows and conceals
// its overlays, so honouring it is what makes a count meaningful.
function visibleText() {
  const shown = (el) => {
    for (let n = el; n; n = n.parentElement) if (n.hidden) return false;
    return true;
  };
  return [...doc.querySelectorAll('p, h1, h2, h3, label, span, div')]
    .filter((el) => shown(el) && el.children.length === 0 && el.textContent.trim())
    .map((el) => el.textContent.replace(/\s+/g, ' ').trim());
}

// showStatus re-renders the auth surface for a different server state, by submitting the
// setup form and having the enroll route answer with that state.
//
// **The obvious driver does not work and looked like it did.** The first draft dispatched
// `visibilitychange`, on the assumption that the app reconciles on it — nothing in app.js
// listens for that event (P07 considered such a reconcile and deliberately left it
// unbuilt), so the status never changed and the test below "failed" against a DOM still
// showing the setup state. A stimulus that does nothing produces a red as convincing as a
// real one; this one was caught only because the failure it reported was implausible.
async function showStatus(st) {
  afterEnroll = st;
  status = st;
  doc.getElementById('authForm').dispatchEvent(new h.window.Event('submit', { bubbles: true, cancelable: true }));
  await settle();
}

test('a first-run user is told what the key is for', async () => {
  await settle();
  const text = visibleText();
  assert.ok(text.some((t) => EXPLANATION.test(t)),
    'nothing visible on first run mentions the SSH key at all — the user is asked to choose one with no statement of what it does');
});

test('the lose-the-key warning is shown exactly ONCE', async () => {
  await settle();
  const hits = visibleText().filter((t) => WARNING.test(t));
  assert.equal(hits.length, 1,
    `the "${WARNING}" warning appears ${hits.length} times on first run, want exactly 1. Said twice it reads as a template error and stops being read; said not at all, the one irreversible property of this app goes unstated. Occurrences: ${JSON.stringify(hits)}`);
});

test('the setup form and the explanation are reachable together', async () => {
  await settle();
  // One overlay, not two stacked. The count is the observable jsdom CAN see; the reason it
  // matters — the upper one swallowing clicks aimed at the lower — is the tier-3 half this
  // file names as its ceiling.
  const overlays = [...doc.querySelectorAll('body > div')]
    .filter((d) => !d.hidden && (d.id === 'introOverlay' || d.id === 'authOverlay'));
  assert.equal(overlays.length, 1,
    `first run shows ${overlays.length} stacked full-screen overlays (${overlays.map((o) => o.id).join(', ')}). The one on top covers the form the user is being asked to fill in, and in a real browser it swallows every click aimed at it.`);
});

test('a returning user is not shown the first-run explanation', async () => {
  await showStatus({ state: 'key-missing', keyPath: '/home/u/.ssh/id_ed25519', version: 'test' });
  const text = visibleText();
  // The stimulus: the app must actually be showing the key-missing surface now, or this
  // asserts about the setup screen under a different name.
  assert.ok(text.some((t) => /can't read the SSH key|cannot read the SSH key/i.test(t)),
    'the app is not showing the key-missing state, so the assertion below is about the setup screen');
  assert.ok(!text.some((t) => WARNING.test(t)),
    'the first-run warning is shown to a user whose key has gone missing — they know what the key is; they need the recovery form, not the pitch');
});
