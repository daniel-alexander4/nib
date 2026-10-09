# ADR-109 — Toggle Features is one page of three sections

**Status:** accepted
**Date:** 2026-10-09
**Context:** Dan: *"in the settings menu, combine updates, Advanced Features and Main Menu into one, called
Toggle Features."* Settings had nine entries (ADR-107's order); three of them — Updates, Advanced features,
Main menu — are almost wholly checkboxes that switch something on or off. `web/index.html`
(`#settingsFeaturesPage`, the Settings pane), `web/app.js` (`appPageLeave`, `flushDownloadDir`, the Simple Sign
checklist's off rows), `internal/server/advanced.go` (the refusal that names where the switch is).
**Supersedes:** three phrases and nothing else —
- ADR-036's *"its own card, next to Advanced features rather than inside it"* (as ADR-104 restated it: its own
  page). Its rules stand: a separate SWITCH from Advanced features, Settings never hideable.
- ADR-104's count and list of Settings entries, and ADR-108's *"the other eight"*.
- ADR-107's Settings order (*Appearance, Colours, Main menu, Read Aloud, Updates, Advanced features, Identity &
  Keys, Vault, About*).
**Applies:** the Settings menu, and anything that tells a user where one of these switches is.
**Superseded in part by [ADR-111](111-toggle-features-is-three-collapsible-cards-in-an-order.md)** (§2's `h3.pagesection` and order: the sections are collapsible cards, Main menu first; a link to a switch opens its card).

## Decision

**1. One entry, one page.** *Toggle Features* opens `#settingsFeaturesPage`. The pages
`#settingsUpdatesPage`, `#settingsAdvancedPage` and `#settingsMenuPage` are gone; every control on them keeps
its id and its handler. Settings is seven entries: Appearance, Colours, Read Aloud, Toggle Features, Identity &
Keys, Vault, About. The entry sits where Updates and Advanced features sat — after how Nib looks and reads,
before identity. That placement is this ADR's and not Dan's words.

**2. Three SECTIONS, never one list of boxes.** Each of Updates, Advanced features and Main menu is under its
own `h3.pagesection` with its own lead, in the order Dan named them. This is what answers ADR-036's objection.
That ADR kept Main menu off the Advanced features page because an Advanced box STOPS a feature at the door that
does the thing and a Main menu box only takes a tab off the menu — and a cosmetic switch on a page that says
"switching one off stops it" makes the page lie. The sentence that says which kind a box is now belongs to the
section, not to the page: the Advanced features lead still says *"Switching one off stops it — it is not just
hidden"*, and the Main menu lead still says *"Hiding one only removes it from the menu"*. Do not merge the two
lists, and do not move a lead up to the page.

**3. The mechanisms stay two.** `applyAdvanced` and `applyModeVisibility` are unchanged. Fusing them hides the
`collaborate` tab when ceremonies are switched off, taking co-signing and Simple Sign with it —
`test/jsdom/advanced.test.mjs`.

**4. The Download folder is on this page, and the page saves it as it is left.** It is the one control here that
is not a switch; it stays with *Check for updates on startup* because it is the same subject. `appPageLeave`
names `settingsFeaturesPage`, so a folder typed and not yet saved is still saved when the page is replaced or
put behind a document (ADR-104, ADR-107 §3).

**5. Everything that points a user at a switch says *Toggle Features*.** The ceremony page's off row
(`data-gosettings`), the Simple Sign checklist's off rows (`goCard('settings', 'Toggle Features')` and their
hint), the server's `switched off — turn it back on under Settings → Toggle Features`, and the README. `goCard`
finds an entry by its text, so a stale label there opens nothing and says nothing.

## Consequences

- A link to a switch lands on the page's heading, not on the switch's section; Advanced features is the second
  section. Not measured as a problem; scrolling to the section is the remedy if it is.
- The page is the longest in Settings: twelve checkboxes and a text field.
- **Covered by** `test/jsdom/apppages.test.mjs` (seven entries in order, every control on a page exactly once,
  the Main menu group named by its section heading, the typed folder saved once on leaving, the off row's button
  reaching the page), `test/jsdom/downloaddialog.test.mjs`, and tier 3's `apppages`, `signsteps` and `cardhue`
  files (each entry opens its page; no page runs past a 375px window; the ceremony switch is driven on it).
