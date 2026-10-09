# ADR-105 — configuration and wizards open pages; actions on the open document stay in the menu

**Status:** accepted
**Date:** 2026-10-08
**Context:** Dan, after ADR-104 made every Settings entry a page: *"The Signing menu should be the
same, they should open in their own tab with nicely formatted and informational pages rather than
expanding in the menu."* And then, the test for which entries that means: *"Only the pages that
requires configuration or runs a wizard should be opened in a tab. Any settings that are actions on
an open document, like placing flags should be expandable in menu."* The Signing menu held four
things: the flag tools, the Simple Sign checklist, the Send & Receive card and the ceremony panel.
`web/index.html` (the Signing pane; `#signingStepsPage`, `#signingCeremonyPage`); `appPageShow`,
`setMarkerMode`, `goCard` / `goPanel`, `applyAdvanced`, `applyModeVisibility`, `renderSignSteps` and
`ceremonyPanelFromPage` in `web/app.js`; *The Signing pages* in `web/style.css`.
**Supersedes:** nothing.
**Superseded in part by [ADR-107](107-the-pages-share-one-tab.md)** (§1's *"with its own tab"*, and the plural in §8's *"every open page"*: a page opens in the one tab the pages share, so at most one is open. Which entries open a page, and everything else here, stands.)
**Extends:** ADR-104 — its mechanism is used as it stands and gains one hook (`appPageShow`) and
one rule (a hidden menu's pages close). ADR-036 (a hidden mode) and the Advanced-features switches
keep their rules and now reach pages.
**Applies:** any entry added to any menu; anything added to a Signing page.

## Decision

**1. The test for any menu entry.** *Configuration, or a wizard* — something you read, set up or
are led through — **opens a page with its own tab** (ADR-104's mechanism). *An action on the open
document* — a tool you use on the page, or a button that acts on the file in front of you —
**stays in the menu and expands there.** A page stands in front of the document (ADR-104 §5), so a
tool for the document cannot live on one.

**2. Signing, entry by entry.**

| Entry | Side | Why |
|---|---|---|
| Place Signing Flags | menu, unchanged | Six tools used by clicking the document, and two acts on it. |
| Send & Receive | menu, unchanged | One-press actions on the open document; each opens its own dialog. |
| Simple Sign | **page** | The guided path: sixteen steps to read and follow, none of which acts where it is listed. |
| Signing ceremonies | **page**, and the live panel stays | Convening and accepting are setup. The running ceremony is live state used beside the document. |

The mode still **lands on the flag tools** (`SIDEBAR_FOR.collaborate[0]`), and entering it opens no
page and no tab.

**3. The checklist is on its page and nowhere else.** `#signSteps` is in `#signingStepsPage`; the
menu holds the entry. On the page a step shows its hint in words (it was a tooltip), the steps are
under four phase headings, and nothing is dimmed to carry a meaning. "On screen" for the kept-copy
row (ADR-073) is *the page is the one in front* — `simpleSignOpen()` — and the row is asked when the
page comes forward, by any door (`appPageShow`).

**4. `appPageShow` is the mirror of `appPageLeave`, called from the same door.** A page whose
content is computed names its refresh there. Nothing else opens, raises or refreshes a page.

**5. A step leads to where its tool is, and a tool for the document shows the document.**
`goPanel`, and `goCard` for a card, leave the page; `goCard` for an entry brings that entry's page
forward in its place. Without this a step that points at Signing's own Flags panel or Send &
Receive card changed no mode, so nothing left the page, and the tool opened beside a page still
covering the document it is for.

**6. A tool in the menu beside a page takes the screen back.** The flag buttons, *Signing marks
completed* and *Save for signing…* are in the menu next to the Signing pages and are not in
`DOC_REQUIRED`. Arming a flag (`setMarkerMode`) and the two acts leave the page first — the park,
not a refusal, the same rule the setup sheet follows. The `DOC_REQUIRED` buttons of Send & Receive
are inert under a page, as ADR-104 §5 has it.

**7. On a page, a feature that is switched off is said to be off.** A menu simply does not offer
what an Advanced-features switch has turned off. A page row marked `data-adv="<feature>"` is hidden
with the feature, and the row marked `data-advoff="<feature>"` — a sentence saying it is off, and a
button to the switch — shows exactly then. `applyAdvanced` is the one writer of both. A checklist
step that names a `feature` says it is off and leads to the switch. The ceremony page's **entry** is
hidden with the feature: what is off by default does not advertise itself in the menu.

**8. A hidden menu's pages close.** `applyModeVisibility` closes every open page whose `data-menu`
is hidden, through `closeAppPage`. ADR-104 lets a page's tab outlive leaving its menu because the
menu is there to go back to; a hidden one is not.

**9. The ceremony page explains and starts; it holds no live state.** Its two buttons click the
panel's own (`#ceremonyConveneBtn`, `#ceremonyAcceptBtn`) after putting on screen what each needs:
the Signing mode, the sidebar open, the ceremony panel shown. Convene's setup sheet takes the main
area from the page through `showCeremonySheet`; Accept's box opens in the panel, beside the page.
**Do not move the panel's DOM onto the page** — see the remainder.

## The wording standard

The pages are an honesty-sensitive surface and are written against README's signing sections.
A hand tick is the user's note, not something Nib checked. A finished ceremony document shows that
its signatures are intact, that they belong to one ceremony, and who of those obliged has signed —
and not how the ceremony ended. An invitation lets Nib watch for your turn and cannot sign as you.
What reaches the public DHT, and that it is never the document, is said where the ceremony is
explained. Change a sentence on a page and re-read the README passage it came from.

## What this costs, and what it does not cover

- **The Signing menu is mixed**: two entries that open pages and two surfaces that expand. That is
  the decision, not a half-conversion.
- **Remainder — the live ceremony panel and the two sheets are not pages.** `#ceremony` renders a
  running ceremony's roster, turn and delivery round into a sidebar panel that seven ADRs, tier 6
  and five test files address by its DOM, and it is used *beside* the document (review, then sign).
  `#ceremonySheet` and `#returnedSheet` stand in the main area behind `syncMainArea` and have no
  tab. Giving the two sheets tabs means making "a sheet is up" a member of `openAppPages` — their
  own park, focus-return and close-on-document-change rules moved onto `appPageLeave` / `appPageShow`
  — and re-aiming `setupsheet`, `returnedsheet`, `returnedverdict` and the red proofs recorded
  against them. It is a slice of its own.
- **A load that happens with the checklist in front asks the kept-copy row before the page
  yields** (the document the user opened is then shown). Harmless; the hold rules still bound it.
- **A page tab clicked from another mode shows the page under that mode's menu** (ADR-104's
  behaviour, unchanged). The ceremony page's buttons enter Signing themselves.
- **No Go change and no new request.** `foldThresholds` is untouched (ADR-087).
- **Covered by** `test/jsdom/apppages.test.mjs` (which entries are pages and which expand, the
  landing, the off rows, the hidden mode, the step doors), `test/jsdom/finalizekeep.test.mjs` (the
  kept-copy row asked as the page comes forward) and `test/ui/signsteps.test.mjs` (a real document
  beside the page: live ticks, a flag armed from the menu, the lock, focus, 414 and 375 pixels).
