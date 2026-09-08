// /pending 377 — the panel drew the commonest invitee state as damage.
//
// A party who has accepted holds a ceremony directory with a `me` marker and no `record.json`,
// because an invitee holds no record until the document reaches their hop. The listing therefore
// classified them `absent`, and the panel dressed that as a fault three ways at once: a badge
// reading **"Nothing on disk"**, a peach card border, and a peach reason. Nothing was wrong — the
// document had simply not arrived yet.
//
// **What only this tier can see.** The Go tests prove the server sends `joined` and the right
// sentence. They cannot see the badge word, the border, or which colour the sentence is drawn in,
// and those are what the user actually reads.
//
// **All three are asserted, because one `cerWaiting` decides all three.** Fixing the badge and
// leaving the card peach is the shape this repo keeps paying for — a rule that reaches some of its
// sites — so the test drives the whole set from one fixture.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { boot, REPO } from './boot.mjs';

const ID = 'a'.repeat(32);
let listing = {};

const h = await boot({
  routes: {
    '/api/ceremonies': () => listing,
    '/api/ceremony/next': () => ({ ceremony: ID, state: 'accepted', reason: 'nothing has arrived' }),
  },
});
const { document: doc, settle } = h;

// Accepted and waiting: the server says so with `joined`, which comes from the marker the accept
// writes. The reason is the server's own sentence, quoted rather than invented here.
const waiting = {
  id: ID,
  state: 'absent',
  joined: true,
  reason: 'this machine is a party to this ceremony and nothing has arrived for it yet — '
    + 'the document reaches you when it is your turn',
};

// The SAME class with no marker: a folder that was removed, or an accept interrupted before
// anything was written. This is what the badge and the border were written for, and it must keep
// them — otherwise the fix is a blanket softening that hides real damage.
const removed = {
  id: ID,
  state: 'absent',
  reason: 'this ceremony has no record on this machine — its folder may have been removed',
};

async function render(data) {
  listing = data;
  doc.querySelector('.modetab[data-tab="collaborate"]').click();
  await settle();
}

const card = () => doc.querySelector('.cercard');

test('a party waiting for the baton is not told their folder is empty', async () => {
  await render({ ceremonies: [waiting], primary: true });
  const c = card();
  assert.ok(c, 'setup: no ceremony card rendered, so nothing below is asserted');

  const badge = c.querySelector('.cerbadge');
  assert.ok(badge, 'the card carries no state badge, so this guard is reading nothing');
  assert.notEqual(badge.textContent, 'Nothing on disk',
    'a party who has accepted and is waiting for the baton is badged "Nothing on disk". Their '
    + 'folder is exactly as it should be — the document has not reached their hop yet — and that '
    + 'is the state most invitees are in most of the time');
  assert.equal(badge.textContent, 'Waiting for your turn',
    `the badge reads ${JSON.stringify(badge.textContent)} for an accepted-and-waiting ceremony`);
});

test('the waiting card drops the warning styling, and it is one decision', async () => {
  await render({ ceremonies: [waiting], primary: true });
  const c = card();
  assert.equal(c.dataset.waiting, '1',
    'the card is not marked as waiting, so the peach border rule (.cercard[data-state]:'
    + 'not([data-state="ok"])) still matches it and an ordinary invitee sees a warning-coloured '
    + 'card for a ceremony with nothing wrong');
  assert.equal(c.dataset.state, 'absent',
    'the card no longer echoes the server\'s own classification. `data-state` is what other '
    + 'readers key on; the waiting fact belongs beside it, not instead of it');
});

test('a folder that really was removed keeps its warning', async () => {
  await render({ ceremonies: [removed], primary: true });
  const c = card();
  assert.equal(c.dataset.waiting, undefined,
    'a ceremony directory with no marker is marked as waiting. Nothing on this machine says the '
    + 'user is a party to it, and softening it hides the case the badge was written for');
  assert.equal(c.querySelector('.cerbadge').textContent, 'Nothing on disk',
    'the removed-folder case lost its badge word — the fix softened the whole class rather than '
    + 'the one state inside it');
});

test('joined is trusted only when the server says it positively', async () => {
  // `joined` is `omitempty`: absent means the server could not say, which is not a definite no.
  // A truthiness test would read a missing field the same as `false`, which is right here — but
  // the case that matters is the other direction, and it is why `=== true` is written out: a
  // future server sending `joined: "yes"` must not soften a warning by accident.
  await render({ ceremonies: [{ ...waiting, joined: 'yes' }], primary: true });
  assert.equal(card().dataset.waiting, undefined,
    'a non-boolean `joined` softened the card. Only a definite true may turn a warning off');
});

// **The stylesheet is read, because this tier cannot resolve a cascade.** `boot.mjs` builds the
// document from `web/index.html` and loads no CSS, so `getComputedStyle` here answers about
// nothing — the same limit `cardhue.test.mjs` states when it measures its ladder "from the
// stylesheet" and leaves the rendered colour to tier 3.
//
// **What it checks is the SPECIFICITY trap, not the colour.** The first cut of this fix added
// `.cercard[data-waiting] { border-color: … }` after the warning rule and it would have shipped
// doing nothing: `.cercard[data-state]:not([data-state="ok"])` is one class and TWO attributes,
// because `:not()` carries its argument's specificity, against the two of the override. Nothing
// below tier 3 would have noticed a peach border on a card the badge had just called fine.
test('the warning border EXCLUDES a waiting card rather than being overridden', () => {
  const css = fs.readFileSync(path.join(REPO, 'web', 'style.css'), 'utf8');
  const rule = css.split('\n').find((l) => l.startsWith('.cercard[data-state]:not('));
  assert.ok(rule, 'the degraded-card border rule is gone from style.css — this guard reads nothing');
  assert.match(rule, /:not\(\[data-waiting\]\)/,
    `the degraded-card border rule is ${JSON.stringify(rule)}. It must exclude a waiting card, `
    + 'not be overridden by a later one: it carries one class and two attributes, and any '
    + '.cercard[data-waiting] override carries two — so the peach border wins and an ordinary '
    + 'invitee sees a warning-coloured card under a badge saying nothing is wrong');
});
