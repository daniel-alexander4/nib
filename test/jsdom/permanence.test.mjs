// P04.S04 — D12 stated before the first hop, at both doors.
//
// # What this slice is actually about
//
// D12's own words are *"the remedy is to abandon and re-convene, losing every signature
// collected"* — and there is no abandon. The plan review traced it: no route, `unconvene` is the
// rollback verb with one caller inside the convene failure path, `endCeremony` fires only on a
// counterparty's decline, `SignTermination` refuses any end state but `declined` and `completed`,
// and `StateAbandoned` is derived, local, and up to three days past a deadline that may be a month
// out. Leaving is local too and sends nothing.
//
// So the sentence this ships is NOT D12's. It is what is true of this code, and these tests are
// about that difference: a statement naming an action the product does not offer would be a false
// expectation on the one surface built to prevent one.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';

const { document: doc } = await boot();

const sin = () => doc.getElementById('sinPermanence');
const srv = () => doc.getElementById('srvPermanence');

test('both doors carry the statement, and it is the same words', () => {
  assert.ok(sin(), 'the initiating door has no permanence statement');
  assert.ok(srv(), 'the consent screen has no permanence statement');
  assert.ok(sin().textContent.trim().length > 0,
    'the initiating door\'s paragraph is empty. It is filled from one constant at boot, so an empty '
    + 'one means the fill never ran and the element is a placeholder nobody notices');
  assert.equal(sin().textContent, srv().textContent,
    'the two doors say different things about permanence. They are one rule at two sites, written '
    + 'once and injected — two copies of a sentence about permanence is how one of them comes to '
    + 'say something else');
});

test('it states what is TRUE of this code, not D12\'s wording', () => {
  const text = sin().textContent;
  // The three facts, each asserted separately: one sentence covering two of them would satisfy a
  // single "mentions permanence" check while leaving the third unsaid.
  assert.match(text, /cannot remove a signature/i,
    'the statement does not say a signature cannot be removed, which is the whole of D12\'s first half');
  assert.match(text, /cancel a ceremony/i,
    'the statement does not say Nib cannot cancel a ceremony. That is the half D12 got wrong — it '
    + 'names "abandon and re-convene" as the remedy, and there is no abandon route at all');
  assert.match(text, /does not tell the other parties/i,
    'the statement does not say the other parties are not told. Without it a convener reads "run it '
    + 'again" as a thing the software communicates — the first ceremony stays live in every other '
    + 'party\'s rail until its deadline, and they keep a valid invitation to it');

  // And it must NOT promise the action that does not exist.
  assert.doesNotMatch(text, /\babandon/i,
    'the statement offers to "abandon" a ceremony. There is no abandon route: endCeremony has one '
    + 'caller and it fires on a counterparty\'s decline, and SignTermination refuses any end state '
    + 'but declined and completed. Naming it would be a false expectation on the surface built to '
    + 'prevent one');
});

test('it is a paragraph in the card, not a toast', () => {
  for (const [name, el] of [['the initiating door', sin()], ['the consent screen', srv()]]) {
    assert.equal(el.tagName, 'P', `${name}'s statement is a <${el.tagName.toLowerCase()}>`);
    assert.equal(el.id === 'toast' || el.closest('#toast') !== null, false,
      `${name}'s statement is inside the toast. A toast is 2.5 seconds in a corner; this has to be `
      + 'readable at the moment of deciding');
    // Above the button that commits, which is what "before the first hop" means on screen.
    const card = el.closest('.sigcard') || el.closest('#srvConsent');
    assert.ok(card, `${name}'s statement is not inside the card that carries the commit button`);
    const go = card.querySelector('#sinGo, #srvAccept');
    assert.ok(go, `${name}'s card has no commit button, so "before" is about nothing`);
    assert.equal(el.compareDocumentPosition(go) & 4, 4,
      `${name} states permanence AFTER the button that commits, so a user who has already pressed `
      + 'it is the first to read it');
  }
});
