# Architecture Decision Records

Short documents capturing significant design decisions and their rationale: what
was decided, why, and what was considered instead.

**ADRs are immutable in their decision content.** If a decision is reversed, write
a new ADR that supersedes the old one rather than editing it. A terminology
refresh may be applied in place with an `**Updated YYYY-MM-DD:**` note in the
header saying what changed; the decision and its rationale stay frozen.

## Why this directory exists, and why it starts at two

Nib went a long way without ADRs, and that was the right call while its
architecture was "one binary, one document, pdf.js in the middle" — a shape you
can read off the code in an afternoon. `STANDARDS.md` §11 puts it as *worth
adopting once a project has real architecture; overkill for tiny apps.*

Two things changed that. The multiple-open-documents work introduces a **repo
law** (ADR-001) that constrains every future async operation whether or not its
author has read the plan, and a **client architecture** (ADR-002) whose rationale
is a set of empirical findings about pdf.js and the DOM that are not recoverable
from reading the resulting code.

And the plan those decisions lived in was temporary. `PLAN.md` retired once its
build order was walked (`STANDARDS.md` §15.6) — so without this directory the
reasoning would have gone with it, leaving a law in the codebase with no surviving
record of what it protects against.

**That retirement happened on 2026-08-19** (all seven phases closed 2026-08-17 at
v1.108.4). Before the file was removed it was audited for reasoning that existed
nowhere else, and three decisions were written up here as ADR-004, ADR-005 and
ADR-006 — the wire protocol for document identity, the open-document cap's measured
byte figure, and the hand-off credential's security posture including the stronger
mechanism that was refused. Code comments that cited `PLAN.md` for a measurement or
a pin now cite the ADR that carries it. The file itself remains in git history; what
is gone is the working document, which is what retirement means.

A dated note inside ADR-002 still refers to `PLAN.md` in the present tense. That is
left as written: an ADR's text is the record of what was known when it was written,
and that note is superseded two paragraphs later within the same document.

**Earlier decisions are deliberately not backfilled.** Loopback-only binding, the
SSH-key-sealed vault, the single embedded binary, client-side fill through pdf.js
— all real architecture, all already documented where they are enforced (in
`CLAUDE.md`'s rationale-carrying rules, in code comments at the guards
themselves). Writing them up retroactively is a separate exercise worth doing on
its own merits, not a prerequisite for recording the two decisions that needed a
home today.

## Decisions

- [ADR-001: Operation pinning](001-operation-pinning.md) — no operation acts on a
  document it did not capture at its start; ids are never reused
- [ADR-002: One PDFViewer per document](002-per-view-viewers.md) — hidden, never
  destroyed, because an overlay's value lives in the DOM
- [ADR-003: Global history budget](003-global-history-budget.md) — the undo/redo
  byte budget is one figure for all open documents and bounds the undo+redo pair
- [ADR-004: Document id on the wire](004-document-id-on-the-wire.md) — an
  `X-Nib-Doc` header, optional only for the CLI, with a per-process epoch and 409
  for a document the server no longer holds
- [ADR-005: Open-document cap](005-open-document-cap.md) — count **and** aggregate
  bytes, refusing on whichever binds first, with the byte figure's method
- [ADR-006: Hand-off credential](006-handoff-credential.md) — a separate on-disk
  secret authorising one route, and why kernel-vouched peer credentials were refused
- [ADR-007: Discovery announcement](007-discovery-announcement.md) — the name, a
  port and a nonce; never the pin, and the socket treated as hostile everywhere
- [ADR-008: The byte cap binds every growth door](008-the-byte-cap-binds-every-growth-door.md) —
  extends ADR-005: the byte half was enforced only at open, so five writers of `doc.data`
  went past it
- [ADR-009: One door per rule](009-one-door-per-rule.md) — a rule that has to hold at more
  than one call site is written once and its guard checks the door, not the text; with the
  six from one review that reached some sites and not others
- [ADR-010: An announcement carries the transport](010-announcement-carries-the-transport.md) —
  extends ADR-007: a port without its transport is not an address, so a QUIC-armed peer was
  dialled over TCP; format version 2, and the tier-4 harness that was configured past it
- [ADR-011: The link gets its window first](011-the-link-gets-its-window-first.md) — nothing
  reaches the public DHT until the local link has had `browseWindow`: the bootstrap is lazy
  behind one door, the fetch waits as the publish always did, and the dial side holds on its
  browse result rather than a timer, and the ARM holds on evidence — a sighting of its own expected
  peer, which `answerLoop` already resolved. 120 off-link packets → 9 → 0; the two-party run that
  was supposed to prove the criterion was the one shape that could not reach the defect
- [ADR-012: The close-out moves a ceremony's folder](012-the-close-out-moves.md) — a ceremony that
  has ended is renamed into `~/nib/ended/`, never deleted, because on every machine but the
  convener's the mirror holds the only copy of that party's own signature and a declined or
  abandoned ceremony has no delivery round to have carried it anywhere. The vault stores still go,
  through one door taking four of them; `RemoveMirror` stays the ROLLBACK's verb and keeps its one
  caller; nothing removes what was moved, which is a decision (`/pending 361`) and not an oversight
- [ADR-013: `DocHash` is a hop-1 anchor](013-dochash-is-a-hop-one-anchor.md) — `ContentDigest`
  covers each page's `/Annots` and a visible signature adds a widget annot, so from the first
  signature onward `Record.DocHash` cannot be recomputed and the signatures, not `DocHash`, are
  what bind a party to bytes. The signature-stable digest that would make it checkable is
  REFUSED: it reopens sticky notes and form values in the one window with no signature to fall
  back on, and it is a `ContentDigestVersion` bump with a skew story. `/pending 358` closed
- [ADR-014: A reload is a mutation of the same document](014-a-reload-is-a-mutation-of-the-same-document.md)
  — re-reading a changed file replaces `doc.data` under the EXISTING id and commits through
  `commitMutation`, so it inherits the byte cap, the ceremony freeze, the registration re-test and
  the undo ring, and is therefore undoable — which an action fired without the user asking owes
  her. This REVERSES the open-then-close button `/pending 333` shipped: six sites already replace
  bytes under a stable id, so "a new id is honest" was refuted by the tree, and because
  `handleOpen` counts duplicates BEFORE installing, every press of that button falsely reported
  `sameFileOpen`. The automatic half runs only on a document that is clean and not in a ceremony,
  event-driven on focus, never a poll. The reading position is NOT preserved and the record says
  so — three restores measured inert, and it is the shared sink's problem (`/pending 372`)
- [ADR-015: The toolbar folds whole groups, within their own pane](015-the-toolbar-folds-within-its-pane.md)
  — every control lives in a `.tbgroup` with a label and a fold rank, and groups MOVE into a `⋯ More`
  menu built inside their own `.tbtab`. The destination is the point: mode gating is
  `#toolbar .tbtab.active`, a descendant selector with no `body[data-tab]` rule anywhere, so a group
  folded outside its pane shows in all five modes and nothing looks wrong until you change mode.
  Moving rather than duplicating because a `data-forward` twin cannot represent a `<select>` and OCR
  has two — and because a second list drifts. Before this the stylesheet had NO `@media` rule: Edit
  chrome ran 19.6% at 1920 to **63% at 360** over 12 rows, now 28.6% flat. The sideways scroll was
  never the toolbar — `#menubar` had a constant 611px minimum, and the planned `min-width: 0` on the
  viewer would have changed nothing
- [ADR-016: The modes are cut by what you do](016-modes-are-cut-by-what-you-do.md) — five modes, each
  a kind of thing you do to the document: File (in and out), Mark Up (put things ON the page, incl.
  form filling), Document (change its pages and content), Secure (remove, protect, Certify), Ceremony.
  Undo/Redo belong to none and move to the toolbar's fixed area — they were Edit-only while nine
  server files commit undoable operations. "Edit" had held four unrelated jobs, which no product in
  the category files together, and ADR-015's grouping could not fix it because group labels are
  deliberately absent from the bar. FIVE not six, measured: 408px before, 476 at six tabs, **392**
  at five once "Signing Ceremony" was trimmed to "Ceremony". A "Pages" tab was killed by a collision
  with File's "Page" group. Two lists fail SILENTLY when a mode changes — `SIDEBAR_FOR` and
  `[data-modejump]` — and `test/jsdom/modes.test.mjs` now covers all four
- [ADR-017: The sidebar carries the commands](017-the-sidebar-carries-the-commands.md) — a mode's
  commands live in a `#commands` sidebar panel, vertical and captioned; the toolbar holds only what
  is true in every mode (Open, Save, Page, Zoom, Find, Close, Undo/Redo). **Supersedes** ADR-015's
  "⋯ More inside its own `.tbtab`" and ADR-016's "gating is `#toolbar .tbtab.active`" — a pane now
  lives in the sidebar while it is open and the toolbar while it is shut, so gating is unrooted.
  "Fixed" means it does not change with the MODE; it still folds with the WIDTH, and conflating
  those measured 34.8% of the viewport at 800px against a 33% ceiling. The win is legibility, not
  space: a 200×580px column fits every mode's whole set (Mark Up 318px is the largest) and can
  afford headings the bar cannot. Cost four corrections and 39 tier-3 failures, all recorded
- [ADR-018: The sidebar is an accordion of cards](018-the-sidebar-is-an-accordion.md) — one card per
  command group and per content panel, exactly one expanded; the existing `.tab` buttons BECAME the
  headers, so their wiring and `SIDEBAR_FOR` still work. **Supersedes** ADR-017's single tabbed
  `#commands` panel. One-word tabs were asked for first and cannot be done: Document has a command
  group called "Pages" and the sidebar already has a "Pages" tab — two tabs, one word, two meanings
  — and seven tabs across 200px is 28px each. The collision was real rather than a tab artefact, so
  the group became **Compose**. The sidebar stops being a tablist (aria-expanded, not role=tab), and
  three layout defects were each found by measuring the DOM: headers travelling into the toolbar
  (33.5% at 800px), the pass-through claiming the column, and every header stretching to 203px
- [ADR-019: A theme lives in four places](019-a-theme-lives-in-four-places.md) — four Catppuccin
  flavours picked from ⚙, with `light`/`dark` keeping their names so saved vaults survive. A theme
  exists in the stylesheet, the Go whitelist, the picker and `THEMES`, and each disagreement fails
  silently: an unguarded palette, a choice gone after a restart, a flavour nobody can pick, a
  palette no test reads — so the four lists are now compared. Card colour is an accent at
  `--card-tint` over `--base`, **per theme**: accent TEXT fails all six accents in Latte
  (1.70–3.52), a rail clears 3:1 for only three, and a tint over `--surface0` leaves light red at
  3.92. Levels computed to hold the worst pair ≥4.8 — Mocha 28, Macchiato 26, Frappé 22, Latte 22.
  Frappé is the constraint and a single 30% put its green at 4.16, caught by the guard
- [ADR-020: A card header toggles, and its label names the action](020-a-card-toggles-and-its-label-names-the-action.md)
  — a click on the open card closes it, for **both** kinds: ADR-018 gave the sidebar group cards and
  panel cards and only the group half ever grew the close branch, so 2 of 4 pills in File mode could
  not be put away. Amends ADR-018 to **at most** one card expanded. Showing a panel is a different
  act from toggling one and gets one door, `showPanel()` — with the header a real toggle, a bare
  `.click()` closes what it meant to open, which is true of the tier-3 harness too. Labels become
  verb-led (*Pages* → **Arrange Pages**, *Certify* → **Sign & Timestamp**): a collapsed card's label
  is all a user has to go on, and the nouns were inherited from a toolbar where the buttons supplied
  the verb. Retires ADR-018's *Pages*/*Pages* collision — neither is one word now
- [ADR-021: Two flavours, and the toggle is the whole control](021-two-flavours-and-the-toggle-is-the-control.md)
  — Latte and Mocha; Frappé and Macchiato removed from the stylesheet, the server whitelist, the
  contrast guard and the settings menu, and the radio picker goes with them. **Supersedes**
  ADR-019's four-flavour half; its card-tint reasoning stands and the pills keep their six accents
  in both themes. A retired flavour normalises at the point of USE (`applyAppearance`), not by
  migrating vaults: nothing rewrites a preference on a machine where nothing went wrong, and
  without it `<html>` carries a `data-appearance` no rule claims while rendering Mocha by luck. The
  agreement guard now asserts the picker's ABSENCE, because silently no longer comparing a list is
  how the next picker gets added with a value nothing defines
- [ADR-022: The bar holds what you reach for continuously](022-the-bar-holds-what-you-reach-for-continuously.md)
  — the fixed toolbar is divided by RHYTHM, not by mode: Save, page, zoom and find are used
  repeatedly while reading one document and stay; Open, Save a Copy, Export & Print and Close are
  once-per-document acts and become File-mode cards. Measured: three toolbar rows down to one, and
  chrome from ~19.6% of the viewport to 9.1%. **Extends** ADR-017/018. Two groups left File mode
  in the same change because neither was a file operation (*Fill Forms from Data* → Mark Up,
  *Combine & Compare* → Document), each appended at the END of its pane so the mode keeps the card
  it lands on. Recent, Save as and Export are FLATTENED rather than moved as dropdowns — a popup
  inside a collapsed card is a second disclosure onto the same items, and no `.menu` has ever
  rendered inside `#commands`. The cost is named rather than hidden: Open… is now one header click
  away, and the fix if that proves wrong is to put Open back beside Save, not to unwind the rest
- [ADR-023: One undo order for one document](023-one-undo-order-for-one-document.md) — Ctrl+Z walks
  the document's changes newest-first across BOTH client stacks (nib's overlay commands and pdf.js's
  annotation-editor commands), which each knew only their own: draw, then arm a nib tool, and the
  drawing was beyond reach — four presses, nothing undone, Undo reading disabled. `clientHistory`
  records whose turn it is; the commit point is pdf.js's `addCommands`, because
  `editingstateschanged` carries booleans and a second stroke raises no event. The key is taken in
  the CAPTURE phase so pdf.js's own binding cannot fire first, while a field with its own undo still
  keeps it. Server ops stay outside: a reload already drops both client stacks, so their order is
  true without bookkeeping. Ink strokes cannot be synthesised in this harness — FreeText stands in
  at the same door, and that gap is named
- [ADR-024: The sidebar has two sections, and the bar names the document](024-the-sidebar-has-two-sections.md)
  — Pages (the thumbnail grid) and Functions (the accordion and the rest); ADR-018's width objection
  to one-word tabs does not reach a TWO-tab strip at 100px each, and it is a real tablist. The bar
  loses Undo/Redo (Ctrl+Z is the route, ADR-023) and Previous/[n]/Next (the keyboard pages; the
  READOUT moved to Pages), gains the document name at the left edge with a save-state dot, and
  pushes Zoom/Reload/Save right. The unsaved flag gets one door — ten sites wrote it directly and a
  freshly opened document read "Unsaved changes", because the sink marks every arrival dirty and
  `installOpened` corrects it a line later. Costs named: undo has NO visible control now, its
  eviction hint went with the tooltip (the toast remains), and a red proof was retired because it
  asserted a control that no longer exists
- [ADR-025: Settings is a mode, and the cards can take one hue](025-settings-is-a-mode-and-the-cards-take-one-hue.md)
  — the ⚙ dropdown becomes the sixth mode with its items as cards (and the gear goes, rather than
  staying as a second route); Settings → Colours swaps the six-accent rotation for ONE hue at six
  stepped tints. **Supersedes** ADR-021's no-picker rule for the colour axis only — light/dark is
  still the toggle. The ladder is bounded by ADR-019's measured `--card-tint`: the darkest rung IS
  today's card and the rest are lighter, so the ceiling needs no new figure (the guard recomputes
  all thirty-six regardless). `all` is the absence of the attribute, which makes the rotation the
  default and an unknown value harmless without a migration. The hue set lives in three places and
  they are compared
- [ADR-026: Simple Sign is one card, and role belongs to the ceremony](026-simple-sign-is-one-card.md)
  — Collaborate's Originate/Receive toggle goes; its seven buttons become one `Simple Sign` card
  (with `Identity & peers…` deduplicated, having been in both halves). Role is a CEREMONY's
  concept — a proceeding has sides, a command list does not. Place Signing Flags leads the column.
  Moving a panel to the front exposed that a mode landing on a PANEL must not auto-open a card
  (`openCard` deactivates panels, so Collaborate stopped landing on Flags), and that two geometry
  guards had encoded "content panels are always last" — one read the Flags panel as a 684px gap,
  the other as a pill with square corners
- [ADR-027: The sign checklist ticks only what it can see](027-the-sign-checklist-ticks-only-what-it-can-see.md)
  — Simple Sign lists the steps of signing in order, each row a link to the tool, each marked
  required/optional and done / not done / **not tracked**. A step is ticked only where Nib can
  observe it (seven can be; eight honestly cannot) — `done` is a probe or `null`, because a tick
  nothing backs is worse than no tick when the list exists to answer "what is left". A checklist,
  not a wizard: nothing is enforced, but the order contains two one-way doors and says so. Cost: a
  TDZ trap (the boot call sat above the `const` it reads and took the rest of app.js with it) and
  a caption written as `.menucap`, which is display:none inside #commands
- [ADR-028: A dial declares its role before either side picks a gate set](028-a-dial-declares-its-role.md)
  — a session connection carries a one-byte ROLE, written by the dialer and acknowledged, before
  either side picks a gate set. ALPN `nib/2` → `nib/3`; the frame goes only to a peer that
  negotiated it, and `SpeaksRoleFrame` is a floor that fails closed. ADR-010's argument one layer
  in: a connection that cannot say what it is for is not an address. It exists because a party
  that has committed HAS a record, so the hop sweep skips it and the delivery sweep arms it —
  and that arm cannot serve a resumed hop (`ReceiveDocument` never reaches `coSignExchange`),
  which was a one-in-three tier-4d failure. The party could not have chosen the other arm either:
  nothing distinguishes "died before the frame landed" from "hop delivered" on its own disk. So
  the dialer says. The arm's `mode` stays POLICY and the wire never overrides it; a co-sign
  reaching the delivery arm gets the REAL human gates, never the unattended ones. Cost, reserved
  rather than discovered: `DeliveryLegBudget` 14m → 14m30s, ~16 min per 32-party ceremony
- [ADR-029: A tick is a probe or a claim, and the row says which](029-a-tick-is-a-probe-or-a-claim-and-says-which.md)
  — supersedes ADR-027's *"a step is ticked only where Nib can observe it"*. That rule protected
  against an UNATTRIBUTED tick and was too strong: eight of the checklist's steps have no probe and
  never will, so under it the list could not record the one thing it exists to answer — what is
  left — for exactly the steps that needed it. The guarantee is kept by PROVENANCE rather than
  scarcity: `row.dataset.by` is `nib` or `hand`, a manual tick never overwrites a probe, and the
  hover text distinguishes them. **Written after the behaviour shipped, which is the second defect
  it closes** (`/pending 417`): a new architectural decision gets an ADR in the same change.

- [ADR-030: An announcement carries which ARM its port belongs to](030-an-announcement-carries-its-arm.md)
  — the format is version **3** and carries a `hop`. ADR-010's argument one level in: a version-2
  announcement's port could be either a hop arm or a delivery arm, on one machine, pinned to the same
  peer, and guessing between them is the defect the version field exists to remove. A version-2
  speaker is refused, not best-guessed. **Written because the code had been at 3 while ADR-007 and
  this index both said 2** (`/pending 420`) — and ADR-010 is the decision that established a bump is
  ADR-worthy, so its own successor going unrecorded is the defect.

- **[ADR-031 — Nothing claims tagging it has not, and every operation declares its tag fate](031-nothing-claims-tagging-it-has-not.md)**
  — a false tagging claim is worse than a visible loss, because a screen reader told a document is
  tagged stops reaching for the fallbacks it would otherwise use. **Eight shipped operations were
  emitting that lie** and none was on a list; law 2's guard found them on its first run by
  enumerating the population from the code. The check is a POST-CONDITION at two write doors, so it
  expires on its own the day the write path carries a tree (`/pending 467`) rather than needing
  anyone to remember to delete it. Measured, not argued: a **no-op** pdfcpu write loses 14 struct
  elements from a LibreOffice-tagged PDF while keeping the claim.
