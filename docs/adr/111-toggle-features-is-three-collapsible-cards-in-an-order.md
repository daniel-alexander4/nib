# ADR-111 — Toggle Features is three collapsible cards, in an order

**Status:** accepted
**Date:** 2026-10-09
**Context:** Dan, of the page ADR-109 made: *"order the toggleable items in an intuitive manner. If necessary
group items in collapsible cards."* The page ran Updates, Advanced features, Main menu — the order the
instruction that created it happened to name them — as three headed sections one after another, ten switches
and a text field on one scroll. `#settingsFeaturesPage` in `web/index.html`, `.setcard` in `web/style.css`,
`goFeature` in `web/app.js`.
**Supersedes:** ADR-109 §2's *"under its own `h3.pagesection`"* and *"in the order Dan named them"*, and its
consequence *"a link to a switch lands on the page's heading"*. Its rule stands and is restated in §3.
**Applies:** the Toggle Features page, and any card added to it.

## Decision

**1. Three cards, in this order: Main menu, Advanced features, Updates.** What you see (the menu's tabs), then
what runs (the features that reach the network), then upkeep. The order is this ADR's reading of "intuitive",
not Dan's words.

**2. Inside a card, the card's own order.**
- Main menu: the tabs left to right as the menu shows them — Page Functions, Accessibility, Secure, Signing.
  A test compares the boxes with the `.modetab` order, so a tab moved in the menu moves its box.
- Advanced features: nearest reach first — Find peers on this network, Reach peers over the internet, Signing
  ceremonies (which uses those two to find a peer), Timestamping.
- Updates: Check for updates on startup, then Download folder.

**3. A card is a native `<details class="setcard">`.** Its summary is the card's name and one line saying what
is in it, so a shut card still says what it holds. Native because it is keyboard-operable and announced with no
script, as the `.advanced` disclosures already are. Cards are independent — opening one shuts nothing. **Each
card keeps its own sentence about what off means** (ADR-109 §2): an Advanced box stops a feature, a Main menu
box only hides a tab. Do not move a lead into the page's, and do not put the two kinds of box in one card.

**4. Main menu is open at first; the other two are shut.** Whatever the user opens stays open for the session,
because a page is hidden and never rebuilt (ADR-104). Nothing is stored.

**5. A link to a switch opens its card.** `goFeature(cardId)` goes to the page through `goCard`, opens the
card, brings it into view and puts focus on its summary. The ceremony page's off row and the Simple Sign
checklist's off rows use it for `featuresAdvancedCard`. A link that lands on a page whose card for the switch is
shut has not taken the user to the switch.

## Consequences

- A switch in a shut card is one press further away than it was.
- Tier 3's harness opens every card of a page it opens (`appPage`), so other files reach their control;
  the first-window state is `apppages.test.mjs`'s own test, which sets the cards back itself.
- **Covered by** `test/jsdom/apppages.test.mjs` (the three cards, which is open, each card's switches in order,
  the boxes against the tab order, the off row opening the card with focus on it) and tier 3's `apppages` (a
  shut card shows no control; its header opens it by mouse and by Enter; a box in a card still works) and
  `signsteps` (the off row puts the ceremony switch on screen).
