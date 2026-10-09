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
  tagged stops reaching for the fallbacks it would otherwise use. **Nothing violates it today**, and
  the ADR records why the first version of it said otherwise: the evidence was a byte count of
  `/StructElem`, and pdfcpu writes the tree into a **compressed object stream**, so that count is 0
  for every output whatever it contains. Parsed, `Rotate` and `Optimize` carry all 14 elements of a
  LibreOffice document and `NUp`/`Collect` drop claim and content together. The enforcement built on
  the byte count was **stripping trees that had survived** and is gone; the census (law 2) and the
  guard stay.

- **[ADR-032 — A PDF/UA identification survives only what nib verified; any change drops it](032-a-conformance-identification-survives-only-what-nib-verified.md)**
  — ADR-031's law one field over. `pdfuaid:part` claims the whole document conforms, and pdfcpu carried
  it through every write: on veraPDF's own corpus `AddNotes` and `StampWatermark` kept it while failing
  7.18.1 t1 and 7.21.4.1 t1. nib checks 19 of 106 rules, so it cannot know an edit kept conformance, and
  every change drops the claim — **inside the rewrite the change already performs**, never as a second
  write, because the first plan (drop at the commit doors) was refuted by a trace: save has no door,
  reload shares one, convene is byte-bound to its mirror, and a second write can land after a signature.
  **Declared gap:** a signed document keeps its claim, since dropping it would destroy the signature.
  `/pending 492`; it unblocks `/pending 486`.

- **[ADR-033 — nib writes the PDF/UA identification only on its own Markdown conversion, with a language someone chose](033-nib-labels-only-its-own-markdown-conversion.md)**
  — `/pending 486`, option B. One door (`pdfops.LabelUA`), two callers (`nib office`, `/api/office`),
  Markdown only, and five named refusals: language not chosen, untagged, no `/Lang`, no displayed title,
  no packet. The claim rests on **veraPDF's measurement of the conversion** across every construct mdpdf
  renders — never on nib's 19-of-106 checker — and a guard fails when mdpdf learns a node kind the fixture
  lacks. The one construct that failed, the thematic break's rule, is now an `/Artifact`. A pre-filled
  language earns no label, because veraPDF checks that a language is present, not that it is right.

- **[ADR-034 — a page subset carries the structure of the pages it keeps, and a subset feeding a composition does not](034-a-subset-carries-the-structure-of-the-pages-it-keeps.md)**
  — `PLAN-ua-coverage.md` P02.S04b, D5. The tree is pruned in place, in the source context S04a made
  stable; an element dies when it loses every kid and **never** because its own `/Pg` died (measured:
  LibreOffice writes `/Pg` on 523 of 523 elements, and the other rule empties the tree on 5 of 7 real
  documents); an OBJR's liveness is its `/Obj`'s; rows are renumbered `0..n-1`, because the source's keys
  are its page indices; a repeat gets its own key and a deep-cloned subtree. **The gate reads the bytes it
  wrote** and an incomplete carry falls back to the honest loss, which is why the refusals are free. `/IDTree`,
  a carried element's `/AF` and a changed element's `/T` are dropped as declared losses — each re-anchors a
  removed page or an embedded file. A subset whose result is COMPOSED does not carry, because `api.MergeRaw`
  keeps only the first `/ParentTree` and `CutPage` leaves `/StructParents` on tiles: P02.S05/S06/S07 own that.
  Extends ADR-031 (five census verdicts flip; a subset joins `Append` as a `partial` producer) and ADR-009
  (one primitive, two wrappers, one declared exemption with a test that the two claimant walks agree).

- **[ADR-035 — accessibility is a mode, because it was a concern spread across three](035-accessibility-is-a-mode.md)**
  — Dan's instruction. Tag structure… (Page Functions), Check accessibility (Secure) and the Review
  Structure Tree panel (`SIDEBAR_FOR.edit`) gather into a seventh mode between Page Functions and
  Secure. Extends ADR-016 rather than reversing it: ADR-016 cuts by what you DO to the document, and
  a concern that spans three verbs is what forced these three apart in the first place. ADR-025 is
  the precedent — Settings earned a mode on the same instruction for the same reason.
  `PLAN-accessibility.md` D10's placement is superseded; its reasoning is not. The seventh tab's
  width against the 719px fold threshold is `/pending 532`.

- **[ADR-036 — the menu is the user's to cut, and that is a different switch from Advanced features](036-the-menu-is-the-users-to-cut.md)**
  — Dan's instruction. Any main-menu tab can be switched off from a **Main menu** card in Settings,
  stored as `Settings.HiddenModes`. **A second mechanism, deliberately not the advanced-features
  switch**: that one stops a feature at the door and cuts at panel granularity *never* at the mode
  (hiding the Signing tab would take co-signing with it, which `advanced.test.mjs` asserts); this
  one is presentational and cuts only at the mode. One door hides both mode lists, `setMode`
  redirects off a hidden mode whoever asked, `settings` is not hideable, and the Go whitelist is
  held against the markup by `TestEveryHideableModeIsARealTab`.

- **[ADR-037 — the document switcher appears at one document, and the close controls do not](037-the-strip-appears-at-one-document.md)**
  — Dan's instruction. The strip shows whenever a document is open and hides only when none is;
  `#closeBtn`'s label and `#closeAllBtn` keep the appear-at-two threshold, because "is there a
  document to tab" and "do Close view and Close all differ" are different questions that shared one
  predicate. **Not `views.length >= 1`**: `views` always holds the empty launch view, so that test
  would put an empty strip on the launch screen. Supersedes the appear-at-two rule, which was never
  an ADR — it lived as a comment in three files.

- **[ADR-038 — `/Stm` is a marked-content reference's key, and content that moved into a form is reached
  through one](038-stm-is-a-marked-content-references-key.md)**
  — `PLAN-ua-coverage.md` P02.S08, D5. ISO 32000-1 puts `/Stm` in Table 324 and in neither Table 323 nor 325,
  so the key `carryTagsThroughNUp` wrote on the *element* was inert: a conforming reader ignored it and fell
  back to the page's own stream, which is the failure it was written to prevent. An operation that moves page
  content into a Form XObject rewrites its bare-integer kids as MCRs naming that form, and a reader keys text
  by the **stream** rather than by the page, because an MCID is unique only within a content stream and two
  forms on one sheet both carry an MCID 0. **It needed deciding because the old shape passed everything**:
  veraPDF, `nib ua` and the repo's own completeness gate score the broken and repaired carries identically,
  while nib's own reader had 31 of 61 elements reading two pages' text concatenated. Extends ADR-034 and
  ADR-031; turns `structartifact`'s `errCommitInForm` from a
  refusal only a hand-built fixture reached into one that fires on nib's own output.
- **[ADR-039 — the download is Nib's, and the install is not](039-the-download-is-nibs-and-the-install-is-not.md)**
  — Dan's ask for progress, a destination and a next step. Nib fetches the release itself and streams
  it to a folder the user can see (`~/nib` by default), reporting progress as a new `event: download`
  on the window stream it already holds — never a second `EventSource`, because each connection counts
  as a window. **Measured, not argued:** a page cannot do this itself — a real release asset answers
  with no `Access-Control-Allow-Origin` on either hop — and the asset is ~95 MB behind a signed URL
  good for about an hour. **The install half is refused**: `build.sh` publishes no checksum and no
  signature, so nothing can verify the bytes; the file is written `0o644` and the dialog offers *Show
  in folder*, keeping README's promise that Nib never installs or replaces itself. The route takes a
  folder and never a URL, reveal takes no path at all, and both sit behind `requireUnlocked` + CSRF
  rather than the check route's public loopback gate. Supersedes v1.95.0's `confirm()` +
  `location.assign` download.
- **[ADR-040 — a converter nib cannot find is one door, and the claim is about the SEARCH](040-a-converter-nib-cannot-find.md)**
  — Dan's ask for a popup naming the missing library. Four sites classified "it is not there" for
  themselves, producing six strings and three wordings per tool, and all six said *"is not
  installed"* — which nib could not know: `exec.LookPath` answers "is this name on PATH", and the
  stock install is OFF PATH on two of the three platforms nib ships to (a macOS `.app` bundle,
  `%ProgramFiles%` on Windows). `internal/browser`'s `findChromium` had already solved this the
  other way. So `pdfops.MissingToolFor` classifies once and every surface words it itself —
  ADR-009 as `handoff.go` states it, *"unifies the CHECKS; it explicitly does not require every
  site to print the same sentence"* — discovery gains per-OS candidates (globs, because Windows
  Ghostscript lives in a version-numbered directory) checked for **executability** rather than
  `browser.fileExists`'s stat-only shape, and a found path is cached while an **empty answer is
  re-probed**, which is what makes *Check again* honest. **Measured, not argued:** a miss costs
  ~107 µs / ~159 µs on a 19-entry PATH against `/api/status`, which is on no timer and fetched
  about once per page load (the ~1 s poller is `/api/session/status`); and *"restart Nib"* was
  refuted as an instruction, because a relaunch **hands off to the running instance** and idle-exit
  never fires in a `noBrowser` run. The GUI's Ghostscript-absent branch existed nowhere: `#pdfaGsGo`
  is revealed only when gs is present, so `pdfa.go`'s refusal was unreachable. The remedy URL is a
  client-side constant, never on the wire — ADR-039's reasoning applied to navigation.
- **[ADR-041 — nothing enters the signature parser that nib's own parser cannot read](041-nothing-enters-the-signature-parser-that-nib-cannot-read.md)**
  — /pending 509's residue from 453 and 502. `digitorus/pdf` v0.1.2 lexes object-stream content past
  its end: `readLiteralString` appends without bound (**`fatal error: out of memory`**) and
  `readHexString` **spins** (a hang, allocating nothing). **Neither is a panic**, so the library's own
  recover, `internal/safe.Recover` and every per-request recover are all irrelevant — and the bytes
  arrive from another party (`l3.go:287`, `cosign.go:19,48`). **Measured, not argued:** over all 7,438
  single-bit flips of a signed fixture (the earlier "4 of 2,480" was a 1-in-3 offset sample), **12**
  were fatal, all 12 inside the word `Filter` of the `/ObjStm` dict — damaging the *key* means the
  stream is never inflated and raw deflate bytes reach the lexer. So `sign.Verify` asks `pdfcpu`
  whether nib can read the document at all, **after** the `/ByteRange` scan and **before**
  `verify.Verify` — one door for 27 call sites, which is what a receive-path-only gate could not be.
  The differential was measured in both directions: pdfcpu answered on **7,438 of 7,438**, refused
  **12 of 12** fatal ones, and the cell that would matter — pdfcpu refusing what digitorus calls
  `valid` — is **empty**; six encryption shapes were swept separately and pdfcpu is at least as
  permissive in every one. Cost 0.15–0.31x the `Verify` it precedes over 3.7 KB–94 KB, and **zero**
  for an unsigned document. Relaxed validation is **pinned rather than inherited**, because
  `NewDefaultConfiguration` returns the user's `config.yml`. A fork behind `replace` (a standing tax),
  a subprocess under a memory cap (no `RLIMIT_AS` on Windows) and a hand-rolled lex gate (the
  differential, written by hand) were all priced and refused. v0.2.0 bounds the allocation and
  **cannot be adopted**: `pdfsign` calls `Reader.Resolve`, which v0.2.0 removed.

- **[ADR-042 — a repeat copies the highest ancestor wholly on its page, and never the `/Document`](042-a-repeat-copies-the-highest-ancestor-wholly-on-its-page.md)**
  A `/ParentTree` row names a LEAF, so a carry driven from it copied leaves and attached each beside
  its original — under the ORIGINAL page's parent. A duplicated page therefore had no `/L` and no
  `/Table` of its own, and page one's `/LI` held **two** `/Lbl` and **two** `/LBody`; on veraPDF's
  own five-column table fixture the header row came out with **ten** cells and the duplicate had no
  table at all. **Every gate scored that as correct** — every element anchored, every MCID resolved,
  `structureCarriedCompletely` empty, `carried` true, veraPDF green — so it is a defect the gates
  cannot express, not one they missed. The climb is monotone and floored at the leaf, so the
  alternative of REFUSING where no wholly-within ancestor exists was refuted by measurement: a
  spanning table has one at every leaf, and refusing costs it its whole tree. Copies splice in as a
  BLOCK, which is what makes the order page-then-page and what fixes three repeats coming out
  reversed. The climb is 1.5–3.0 µs per row leaf and flat, because `span` bails at the first page
  disagreement. Declared gap: the carried tree is ordered against the SOURCE, not the output's page
  sequence.

- **[ADR-043 — a page selection may not reveal what the source hid](043-a-page-selection-may-not-reveal-what-the-source-hid.md)**
  — /pending 525. Every disposition on `catalogAllowlist` until now rested on "a key carried onto a
  subset can be a positive false statement about content that is gone". `/OCProperties` inverts it:
  the layer's content and the page's `/Properties` travel with the page dictionary, and the catalog
  key is the only thing saying the group is OFF, so **dropping it REVEALS**. Measured on a group
  listed in `/OFF`, page 1 at 40 dpi: **39 → 18,855** dark pixels under Ghostscript 10.02.1 and
  **6 → 18,598** under poppler — and `RedactPages` builds its untouched runs through
  `collectWithoutStructure`, so the reveal was landing inside a redaction. So the carry is NOT gated
  on the structure carry, the guard is a **reachability refusal** rather than a key list (the one
  harm is re-anchoring a dropped page — `/StructTreeRoot`'s hazard one key over, and mutating the
  walk out puts three page dictionaries in a two-page output), and **nothing prunes the groups**,
  because a walk that misses one `/OC` drops the group that was hiding something. `/PageLayout` is
  carried as a resolved NAME (an indirect one is dropped); `/OutputIntents` is dropped and the code
  now says why — the subset removed the claim already, and **26 of 26 corpus entries are
  `/S /GTS_PDFA1`**, so carrying it is ADR-032's rule one key over; `/PageMode` gains `/UseOC` and
  nothing else, its corpus being 25 `/UseOutlines` + 8 `/UseAttachments`. Caveat carried forward:
  283 of the 295 scanned files are `veraPDF Test Builder 1.0`.

- **[ADR-044 — a face nib draws with is named nib's own](044-a-face-nib-draws-with-is-named-nibs.md)**
  — /pending 494. pdfcpu matches a watermark's font to one already in the document **by name and
  nothing else** (`fo.FontName == wm.FontName && fo.Prefix != ""`) and then rebuilds it from nib's
  TTF with the DOCUMENT's glyph ids. Nib stamps with Liberation; LibreOffice writes Liberation. So
  the name is the whole of the match, and the whole of the fix: nib installs its faces as `Nib…`
  rather than `Liberation…`. Measured through nib's own office door — before, `StampFields` added
  **7.21.4.1 t1**; after, it adds nothing, and the document's own font program is byte-identical
  across the stamp (13,468 → 13,468). The rename is confined to the `name` table and to IDs 1/3/4/6
  and 16–25: every other table byte-identical across all twelve faces, `head` differing only in
  `checkSumAdjustment`. **The vendored bytes are not modified** — the rename is applied to a runtime
  copy. **And the licence points this way, not against it**: pdfcpu already embeds a MODIFIED
  Liberation (`font.Subset` prunes `glyf`/`loca`), so nib was shipping a modified Liberation still
  called `LiberationSans`, which is what OFL §3's Reserved Font Name asks you not to do. IDs 0, 7
  and 13/14 carry through unchanged, held by a red proof. Declared gap: `AuthoredTextFaces` (Roboto
  ×4 + LiberationMono) is not renamed, because it would move every Markdown conversion's `/BaseFont`
  and touch ADR-033's label claim.

- **[ADR-045 — a composition carries the comments of the pages it composes](045-a-composition-carries-the-comments-of-the-pages-it-composes.md)**
  — /pending 562. `api.NUp` never looks at `/Annots`, and pdfcpu writes by reachability, so every
  annotation left the file with the page dictionaries the composition removed: **3 sticky notes in,
  0 out**. On a tagged document it cost the whole tag tree as well — the note's `/Annot` element's
  `/ParentTree` key became `unowned-key` and `completeOrHonest` abandoned the carry. The n-up now
  puts each note back on the sheet its page landed on, with the rect through `R × FormMatrix × CM`
  read out of the document rather than re-derived, and an `OBJR` repointed at the copy so the
  orphan is not written. `/Text` alone: an annotation with an appearance stream would be drawn
  upright inside a rect the placement turns 90°, and a `/Widget` would put signature rubble back
  past four gates that key on an edited document having none. Cost measured at +10 ms on a 40-page
  document with 40 notes, and inside the noise with none. Declared gap: everything that is not a
  `/Text` is still dropped, still without a sentence — the page-op route has no notice channel.

- **[ADR-046 — a page-indexed key is restated, and a two-form key is split by form](046-a-page-indexed-key-is-restated-and-a-two-form-key-is-split.md)**
  — /pending 554 and /pending 555, the two keys ADR-043 named and could not take. `/PageLabels` is
  keyed by page INDEX, so it is REBUILT against the output's positions from `keep` rather than
  pruned, and the label follows the PAGE — a reorder does not renumber the document, which is
  `outlinecarry.go`'s own ruling. A page the source never labelled gets an explicit empty entry, or
  it inherits the range above it in the output and says something this document never said. No range
  ceiling: the worst case (200 pages reversed, 200 ranges) is **+6,582 bytes** and no measurable
  time, and a document with none is worse than one with 200 right. `/OpenAction` is ONE key meaning
  two things, so it is split by form — a destination is carried through the outline's own predicate,
  a `/S /GoTo` is reduced to its `/D`, every other action is dropped, and the discriminator is `/S`
  (required) and not `/Type` (optional). `Scan` read the same key the same wrong way, calling a
  destination an auto-run hook at high severity; one door now decides for both, and `StripActive`
  keeps deleting it whole as a named exemption.
- **[ADR-047 — a structure tree describes the file, not the view](047-a-structure-tree-describes-the-file-not-the-view.md)**
  — P02.S05. A crop hides without removing, so the tags keep describing what it clipped, and Crop now
  moves each page's `/MediaBox` in the page's own space instead of rebuilding the page — carrying the
  tree without a carry, and no longer deleting the cropped pages' annotations. The principle stops
  where a page would be READ more than once: a split tile carries no subtree (P02.S06).
- **[ADR-048 — a merge grafts onto the host, and only extends a claim the host already makes](048-a-merge-grafts-onto-the-host-and-only-extends-its-claim.md)**
  — P02.S07a. `Append(tagged, tagged)` gave both pages `/StructParents 0`, so the second document's page
  resolved to the first's element — invisible to a census that only merged untagged second documents.
  nib now owns the merge loop (the graft cannot sit behind `MergeRaw`, which frees the second catalog) and
  grafts a tagged later document onto a tagged host, keys offset; onto an untagged host its claims are
  stripped. Amends ADR-031's merge note; `partial` stands for tagged + untagged.
- **[ADR-049 — a split tile carries no subtree, and the rest of the document keeps its tree](049-a-split-tile-carries-no-subtree.md)**
  — P02.S06, the boundary of ADR-047. A tile holds its page's WHOLE content stream, so a subtree cloned onto
  each tile makes the document read that page N times — a false account of structure, not a view mismatch.
  The tiles carry nothing; since P02.S07b the original is `splice`'s host, so every other page keeps its
  tags and the fate is `partial`. The positional partition that would tag each tile honestly is unbuilt.
- **[ADR-050 — nib's own pages are tagged as real content, exactly, before any signature](050-nibs-own-pages-are-tagged-as-real-content.md)**
  — P02.S09. The co-sign readme, the ceremony page and the signature pages were untagged, so a tagged
  document left preparation claiming tagging over 35 undescribed text runs. nib knows every line's role
  when it draws it, so the pages are tagged at the `Exact` tier through `TagAuthoredPages` (the
  autotagger's `Inferred` would downgrade the host through the graft), after the language declaration.
  Declared gap: signature widgets (`/pending 576`).
- **[ADR-051 — the signer is the certificate the SignerInfo names, never the one that leads the bag](051-the-signer-is-the-certificate-the-signerinfo-names.md)**
  — /pending 613. A PKCS#7 certificate bag is a SET, sits in the `/Contents` hole in the `/ByteRange` and
  is therefore unsigned, and `p7.Verify` resolves the signer by issuer and serial rather than by position —
  while `verify.Signer` reports the bag and discards the name. Reading element 0 let a signature made with
  an attacker's key, carrying a victim's certificate first, verify `Valid` under the VICTIM's fingerprint,
  which `Completeness` counted towards the roster. nib re-parses each blob and takes `GetOnlySigner`; where
  it cannot establish who signed there is NO fingerprint, and an empty one discharges no obligation. Joined
  by the bag, order included — and by the LIBRARY's own enumeration, because a cheaper `/Fields` walk
  let the attacker write both sides of the key (a decoy field supplying the bag for a real signature
  listed nowhere) and reinstated the bug inside its own fix. 18.7 ms at 400 pages beside the 20.0 ms
  call it follows, paid deliberately.
- **[ADR-052 — shown bytes are read through one door, and the checker reads CMaps as veraPDF does](052-shown-bytes-are-read-through-one-door-and-the-checker-reads-cmaps-as-verapdf-does.md)**
  — P07.S02, `/pending 657`. `internal/fontcode` decodes string operands, cuts codes by codespace and reads
  `/ToUnicode` for both the checker and `pdfops`, and holds two readings of a CMap side by side: veraPDF's
  (counted lists, a range cut at its last byte, a malformed CMap discarded whole) for the checker, and the
  specification's lenient one for text extraction. What the checker cannot reproduce it refuses.
- **[ADR-053 — a page gets its credentials only from a launch key](053-a-page-gets-its-credentials-only-from-a-launch-key.md)**
  — /pending 685, supersedes ADR-006's premise. `/api/status` served the CSRF token to any loopback caller
  (curl sends neither `Sec-Fetch-Site` nor `Origin`), and GETs behind `requireUnlocked` needed no token at
  all, so every local process — other OS users included — could read and drive Nib. Nib now opens its window
  at a single-use key in the URL fragment, traded once for a per-port `HttpOnly` `SameSite=Strict` session
  cookie (every method) and the CSRF token (writes), through one door, `requireSession`. A second launch gets
  a key through the hand-off, which is ADR-006's secret's one new grant. Residuals: a same-user process,
  and the key in the browser's argv from launch to trade (`/pending 701`).
- **[ADR-054 — the token is the only credential, and every route but three needs it](054-the-token-is-the-only-credential-and-every-route-needs-it.md)**
  — /pending 704, supersedes ADR-053's cookie and its public-route list. The wizard routes were a
  passphrase oracle and `/api/quit` a kill switch for any local user; the cookie reached every other
  loopback server the browser visited. The token alone, header on any method or `auth` query on a GET,
  through `requireSession` on every route but launch, instance and handoff; the page keeps it in
  per-origin `sessionStorage`. Guarded by a route census read from `server.go` and driven as a stranger.
- **[ADR-055 — pdfcpu validates and optimizes only through `internal/pdfread`](055-pdfcpu-validates-and-optimizes-only-through-pdfread.md)**
  — /pending 675, 714 (after 706). pdfcpu's validator recursed on a `/UseCMap` loop until the stack overflowed —
  unrecoverable, so opening a 2.5 KB file killed nib — and its optimize pass is cubic/exponential on hostile forms.
  One package is the door: the loop is refused between read and validation, the pass runs only when nib's estimate
  says it is bounded, and the accessibility checker refuses an unaffordable document rather than report from a
  reading it was not calibrated against. An AST census holds every other package off pdfcpu's validating reads.
- **[ADR-056 — a page's content is its streams joined at token boundaries](056-a-page-s-content-is-its-streams-joined-at-token-boundaries.md)**
  — `PLAN-text-reflow.md` P05.S01. pdfcpu joins a `/Contents` array with no separator, so `(A) Tj` + `ET` read as
  `TjET` and a trailing comment swallowed the next stream's first line — in seventeen readers and seven writers.
  One door, `pdfread.PageContent`, separates only where the join would fuse (0 of 77 real joins); the digest and
  the checker are named exemptions, guarded by an AST census.
- **[ADR-057 — a read compared with pdfcpu's own output reads pdfcpu's join](057-a-read-compared-with-pdfcpus-output-reads-pdfcpus-join.md)**
  — supersedes ADR-056's exemption list in part. The n-up note and tag carries compare a page with the form pdfcpu's
  `NUp` wrote from its OWN join; through the door a divided page failed the match and lost its notes and tags.
- **[ADR-058 — a signature is one record, and the library is joined to it by position](058-a-signature-is-one-record-and-the-library-is-joined-by-position.md)**
  — `PLAN-returned-document.md` P01.S01, supersedes ADR-051's join key. `sign.Revision` is the one home of who
  signed, how far and whether it is well-formed; the library's signers line up with the records by ordinal in
  the library's own enumeration, the certificate bag survives only as the per-position cross-check (an empty
  bag must mean not valid), and any disagreement blanks every fingerprint and sets `AddedAfter`. Two
  signatures sharing a bag are now each named. The sweep and its byte-range gate run before the library.
- **[ADR-059 — a document timestamp never bounds `AddedAfter`, and the refusal is the published answer](059-a-document-timestamp-never-bounds-addedafter.md)**
  — `PLAN-returned-document.md` P01.S02, /pending 661. The `/Fields` ByteRange walk is deleted: `AddedAfter` is
  measured over records that verified, are well-formed and are not document timestamps, because a stamp names
  nobody and anyone can obtain one over any bytes. `Status.Refused` names every refused record and
  `AddedAfterCause` says which fact set the bit; the zero-signer path is `Invalid` on a non-empty `/Contents`
  the library would have processed or the sweep refused. A stamp's imprint is never checked on the verdict path
  (the dispute surface's, on demand). One enumeration in `internal/sign`, two marked exemptions, guarded by an
  AST census whose bypasses are each driven red.
- **[ADR-060 — a signer is a well-formed record that is not a timestamp, and `State` is theirs](060-a-signer-is-a-well-formed-record-that-is-not-a-timestamp.md)**
  — `PLAN-returned-document.md` P01.S03, /pending 687, 737. `Revision.countsAsSigner` is the one predicate and
  `bounds` is built on it: a refused copy and a document timestamp are not in `Signers` and do not vote on
  `State`, so a copy no longer halts a ceremony or moves the next signature and a B-LTA document reads valid.
  Zero counted signers is `Invalid` where a checkable blob exists; `addedAfter` keeps the library's count; a
  join error excludes nothing. `Status.Timestamps` names each stamp; a refusal is shown, and `nib verify` exits
  2, whether or not it set `AddedAfter`.
- **[ADR-061 — the DHT keeps, and queries, only what the node-cache rule admits](061-the-dht-keeps-and-queries-only-what-the-cache-rule-admits.md)**
  — /pending 707, 743. One rule, `Server.scope` (`addrscope.Seed` in production), governs cache load, cache save
  and outgoing queries: the cache is merged not overwritten and keeps only nodes that answered nib's own queries,
  and nib's socket refuses a query aimed out of scope (a reply always goes). `OpenAdmittingLoopback` is the
  test-only widening, held by a census.
- **[ADR-062 — a signature nib could not check says why, and a signature nib made is read back](062-a-signature-nib-could-not-check-says-why-and-a-signature-nib-made-is-read-back.md)**
  — /pending 741, 747. `Status.Unchecked` names why a verdict rests on no checked signer (`hybrid-reference`,
  `unread`, `unreadable`, with `could-not-check`); `runSign`'s `signedAsIntended` reads the new signature back via
  the sweep and one PKCS#7 verify — +20–25% time and +50% memory at 100 MB, where a full `Verify` cost +230%.
- **[ADR-063 — a signature only one reader can see makes the verdict `Invalid`](063-a-signature-only-one-reader-sees-invalidates-the-verdict.md)**
  — /pending 749; supersedes ADR-062 in part. pdfcpu's population (it follows `/XRefStm`) is compared with the
  sweep's; a signature only pdfcpu holds makes `State=Invalid` with `Unchecked` named, on the signer path too.
- **[ADR-064 — a stream is decoded within what is left of its budget, at one door, and page content is bounded where pdfcpu decodes it](064-a-stream-is-decoded-within-what-is-left-of-its-budget.md)**
  — /pending 748. `pdfread.DecodeWithin` caps every budgeted decode during the decode; both page-content joins stop
  at 512 MiB; pdfcpu's optimize pass is estimated only where it decodes page content (`passDecodesPageContent`) and
  only when a stream is named twice, then decodes once. Declared gap: distinct streams, bounded by file size.
- **[ADR-065 — a per-page loop reads the pages from one walk of the tree](065-a-per-page-loop-reads-the-pages-from-one-walk.md)**
  — /pending 753. `pdfread.Pages` answers what `PageDict` answers for every page from one walk, and falls back to
  `PageDict` per page wherever one walk cannot reproduce pdfcpu. Prepare on 7,059 pages: 50 s → 12 s.
- **[ADR-066 — nib carries a patched `digitorus/pdf`, and the patch is its object-stream lookup alone](066-nib-carries-a-patched-digitorus-pdf-and-the-patch-is-its-object-stream-lookup.md)**
  — /pending 758; supersedes ADR-041's refusal of option A in part. `third_party/digitorus-pdf` differs from v0.1.2 in
  `read.go` only: each object stream decoded and its header lexed once per `Reader`, lazily, so nothing past the member is
  reached. One declared divergence (a backward `/First` resolves). `libraryLookupCost` re-fitted; census 3.26 s → 0.49 s a pass.
- **[ADR-067 — the patched object-stream reader refuses what it cannot bound](067-the-patched-reader-refuses-what-it-cannot-bound.md)**
  — /pending 759, 760; supersedes ADR-066's "one divergence". Member reads are bounded by what the stream decoded and a
  stream by 64 MiB, refused as `lookup-cost`; the reader is fuzzed against an unchanged v0.1.2 kept in testdata.
- **[ADR-068 — the patched library also patches its lexer](068-the-patched-library-also-patches-its-lexer.md)**
  — /pending 761; supersedes ADR-066's read.go-only scope and ADR-067's "the gate covers Verify" in part. pdfcpu read
  every endless-loop shape and each took the process on all five paths; `lex.go`'s readArray refuses `endobj`, the
  object-stream view refuses reads past its end, and pdfcpu's gate is described as keeping out what it cannot read.
- **[ADR-069 — the reference door mirrors pdfcpu's unguarded recursion](069-the-reference-door-mirrors-pdfcpus-unguarded-recursion.md)**
  — /pending 764; extends 675's `/UseCMap` check. One walk before the validator over the edges pdfcpu follows without
  a guard, refusing a loop (fatal stack overflow), more paths than the budget (sharing is walked once per path and per
  page), and depth; objects pdfcpu marks first stop the walk. 0 of 333 real documents refused; 1.6% of a read.
- **[ADR-070 — the patched reader refuses an indirect object-stream key](070-the-patched-reader-refuses-an-indirect-object-stream-key.md)**
  — /pending 768; supersedes ADR-067's "declared, not charged" in part. An object stream whose `/Type`, `/N` or
  `/First` is a reference compounded per level; the reader refuses it (NOTICE.nib divergence 5), so the ungated
  `HasSignatureBlob` (712, decided C) needs no gate for it. Verified by review only.
- **[ADR-071 — text that moves takes what is anchored to it, and its structure, or refuses](071-text-that-moves-takes-what-is-anchored-to-it-or-refuses.md)**
  — PLAN-text-reflow P07. A flow's anchors are chosen from the original page and applied by identity; a flow leaves a
  page only past its margin; a carried paragraph takes its marked content and element (MCR in place of the kid); the
  write half writes nested `/ParentTree`s (reversing its blanket refusal, 7 of 14 real documents); one `/StructParents`
  reader.
- **[ADR-072 — a refusal about a document is a 422 naming its cause, and a fact about returned bytes rides in a header](072-a-document-fact-refusal-is-a-422-and-a-returned-bytes-fact-rides-in-a-header.md)**
  — PLAN-returned-document P02.S03. 409 stays ADR-004's "not that document" (it reconciles the tabs); a contents
  refusal is 422 + `{cause}`; facts about returned bytes ride in one JSON header, lists capped; a malformed parameter is
  400 after normalisation.
- **[ADR-073 — a copy kept when you signed is opt-in, named by one door, and its failure refuses the signing](073-a-copy-kept-when-you-signed-is-opt-in-named-by-one-door-and-its-failure-refuses-the-signing.md)**
  — PLAN-returned-document P04.S01. Off at every opening, with an unencrypted-on-disk disclosure; the exact signed
  bytes, durable at 0600, written before they are sent, a failed write a 500 `copy-not-kept` that refuses the signing;
  `kept_<slug>_<ts>-<8hex>.pdf`, a grammar no other `~/nib/signed` writer produces, named only through `keptPathFor`;
  the name's digest is never evidence.
- **[ADR-074 — the accessibility checker reads every resource the file binds, never pdfcpu's pruned set](074-the-checker-reads-every-resource-the-file-binds.md)**
  — `/pending 782`. pdfcpu's per-page resource step pruned by an undecoded name scan and deleted the resource `/X#30`
  draws; the checker turns it off, keeps the inheritance half as `pdfread.InheritResources`, keeps form/font fusion, and
  decodes every content name through `nameKey`.
- **[ADR-075 — the PDF/UA label needs every font embedded](075-a-label-needs-every-font-embedded.md)**
  — /pending 820; extends ADR-033, whose five refusals become six. A Markdown conversion that degraded to the Base-14
  core fonts (the faces could not be installed) fails 7.21.4.1 and is not the conversion veraPDF measured; `LabelUA`
  refuses it as `ErrUAFontNotEmbedded`, asked of the bytes through `nonEmbeddedFonts`.
- **[ADR-076 — the spoken check's verdict crosses the wire](076-the-spoken-checks-verdict-crosses-the-wire.md)**
  — /pending 802; ALPN `nib/3` → `nib/4`, extends ADR-028. After its gate each side sends a one-byte verdict (one code
  for decline and timeout) and a confirming side waits for the peer's before any document byte; a rejection arrives as
  `p2p.ErrPeerDidNotConfirm`, never a transport loss, so it is no longer re-raced. Read concurrently with the local gate
  and lingered on by the decliner, because a QUIC close destroys an unread frame.
- **[ADR-077 — the reference door bounds depth through the edges pdfcpu guards, too](077-the-reference-door-bounds-depth-through-guarded-edges.md)**
  — /pending 803; extends ADR-069. A loop-free chain of form XObjects passed the door whole (it stops at guarded
  objects) and 700,000 links overflowed pdfcpu's validator stack, fatally. A depth-only pass over guarded and unguarded
  edges, SCC-weighted as an upper bound over pdfcpu's map order, refuses past 8,192 as `ErrReferenceDepth`.
- **[ADR-078 — Complete & sign offers the kept copy, in its banner](078-complete-and-sign-offers-the-kept-copy-in-its-banner.md)**
  — /pending 814; supersedes ADR-073 decision 1's "Complete & sign offers no tick". The recipient's tick sits in the
  signing banner beside both of Complete & sign's buttons, off at every opening, its disclosure read from the Finalize
  modal's; the request carries `keep` as the modal's does and a `copy-not-kept` refusal is worded by the one
  `keptRefusal`. The CLI still offers none.
- **[ADR-079 — a stamp leaves a turned page where it was, and a flag needs no anchor while every rewrite keeps its place](079-a-stamp-leaves-a-turned-page-where-it-was-and-a-flag-needs-no-anchor.md)**
  — /pending 457. No content anchor in `NibFlags`: every route that commits a rewrite is classified for its flags and
  every keeps-geometry rewrite is driven over flagged documents. That census found pdfcpu's stamp turning a turned page
  about the origin, so a page whose box does not start there left its box on OCR, bake, watermark and page numbers;
  `stampInPlace` is now the one door every pdfcpu stamp runs through, turning about the box's corner.
- **[ADR-080 — a record is checked by the digest rule it names, and rule 5 covers what the reader sees](080-a-record-is-checked-by-the-digest-rule-it-names.md)**
  — /pending 718, 719, 720, 578, 616; supersedes ADR-013's "a coverage change reads as tampering" and ADR-056/057's
  exemption list in part. `ContentDigestVersion` 4 → 5, and rule 4 is still COMPUTED: every comparing site calls
  `ceremony.DocumentHashFor`, under the record's signed `DigestVersion`, so a bump no longer halts a ceremony in
  flight. Rule 5 joins at token boundaries, hashes a reached page as its position, reads effective (inherited)
  geometry and resources, and covers `/OCProperties`, `/OpenAction`, `/AA`, `/Names /JavaScript`, `/UserUnit`. The
  checker reads `pdfread.PageContent` because veraPDF separates an array's streams (measured).
- **[ADR-081 — a cyclic component is charged by its guarded targets and its unguarded path](081-a-cyclic-component-is-charged-by-its-guarded-targets-and-its-unguarded-path.md)**
  — /pending 803's regression; supersedes ADR-077's `((n+2)/2)²` in part. That charge was the worst split of n, so it
  refused 2,000 forms naming their own dictionary (2,000 levels) and passed a pattern/form shape pdfcpu walks k(k+1)/2
  deep. A component is now (g+1)·l — guarded targets inside it, longest unguarded path — measured at ~1,081 bytes of
  stack a level on all three shapes; the 8,192 bound stays, ~60× under the usable stack. An indirect dictionary of
  guarded entries is a node of its own and no level, so sharing one is 2n edges, not n² (22 s → 29 ms at 4,000 forms).
- **[ADR-082 — no reader of a PDF is handed to pdfcpu; its reader-taking `api` functions are restated in `pdfread`](082-no-reader-of-a-pdf-is-handed-to-pdfcpu.md)**
  — /pending 716, 717; supersedes ADR-055 decision 1 in part. `pdfread.Reader` handed pdfcpu a reader and the `api`
  function ran its own optimize pass unbudgeted (every one past 30 s on the 400-form chain). Each is restated over
  `Validated`/`ReadOptimized` (`apiread.go`); `MergeRaw`'s closing pass is budgeted, so exhibits skip rather than refuse;
  the guard bans every reader-taking `api` function read from pdfcpu's source, so where a reader was built is moot.
- **[ADR-083 — a PDF/A identification survives only what nib verified, through ADR-032's door](083-a-pdfa-identification-survives-only-what-nib-verified.md)**
  — /pending 641; extends ADR-032, supersedes ADR-082 in part. `pdfaid` is dropped by the one door that drops
  `pdfuaid` (measured: `Rotate` kept it on both PDF/A writers' output); `PreparePDFA` and `ConvertPDFAGhostscript` are
  its writing doors. `Encrypt`/`RemovePassword` now rewrite through `rewriteWithConf`, and the census refuses a row
  that changes a document and is asked by no drive.
- **[ADR-084 — pdfcpu's page operations are handed separated contents, and the n-up carries read the door](084-pdfcpus-page-operations-are-handed-separated-contents.md)**
  — /pending 728; supersedes ADR-057. pdfcpu's n-up, resize and cut wrote their own bare join of a divided page, so its
  text became the unknown operator `TjET`; `pdfread.SeparateContents` puts a `\n` stream wherever the door separates,
  before every such call (census-guarded), so the carries read the door again. The census also sees `/Contents` key
  reads (`resourceprune.go` was invisible), and the door decodes a repeated stream once.
- **[ADR-085 — a launch into a windowless Nib is a new session, and what was open is remembered by path](085-a-launch-into-a-windowless-nib-is-a-new-session.md)**
  — a launch inside the 10 s exit grace was handed to the old process and reopened everything the closed window held.
  A hand-off that cancels a grace now closes those documents; a reload keeps them. The open paths are recorded in the
  vault when the last window goes and on Quit, and *Resume last session* reopens them.
- **[ADR-086 — a launch into a running Nib is a tab in the window that is already open](086-a-launch-into-a-running-nib-is-a-tab-in-its-window.md)**
  — the open window could not hear a hand-off, so every launch made another window showing everything. The hand-off is
  pushed on the window stream and the launch opens a window only when none has it; launches that start together become
  one Nib; `?open=` is spent once; a session's end clears its locked queue. A covered window is raised on X11 when a
  helper exists and marks its title otherwise.
- **[ADR-087 — the bar is icons, and Undo and the page position are back in it](087-the-bar-is-icons-and-undo-and-the-page-are-back-in-it.md)**
  — supersedes ADR-024's removal of Undo/Redo and Previous/Next. Every bar control is an icon whose word stays in the
  markup (hidden in the bar, shown in ⋯ More); Zoom is its own group with the level shown; one group appears only
  while a tool is armed; the fold ladder is re-measured.
- **[ADR-088 — where things are on a page is read from what the page draws](088-the-page-map.md)**
  — field detection scanned pixels and search-redaction estimated glyph positions; neither was measured. `pdfops.MapPage`
  gives each rule, box, glyph boundary and existing field in displayed-page fractions, and `build/accuracy.sh` scores
  detection and redaction against it on a local corpus (baseline: 61% of real fields found, 27% of proposals on labels).
- **[ADR-089 — a field is read from the lines the page draws](089-fields-are-read-from-the-lines-the-page-draws.md)**
  — extends ADR-088. `proposeFields` reads cells, lines to write on, underscore blanks and squares from the page map,
  and the picture-based detector adds only where the map proposed nothing. Real fields found 62.9% → 91.2% (well
  placed 56.0% → 84.5%); the harness's "found" and "on a label" measures are corrected in the same step.
- **[ADR-090 — a search match is boxed from its own glyphs](090-a-search-match-is-boxed-from-its-glyphs.md)**
  — extends ADR-088. `matchesInMap` boxes a search-redaction match from the map's glyph boundaries; `placeMatches`
  lets that box replace the estimate only when it holds the estimate's centre, so a match the map did not place is
  never dropped for a neighbour's sake. Overreach 2.6 → 0.37 glyph-widths; a neighbouring glyph taken 72% → 3.7%.
- **[ADR-091 — an OCR word is redacted out to the next word](091-an-ocr-word-is-redacted-out-to-the-next-word.md)**
  — supersedes ADR-090 §4 for hidden text. Nib's stamped OCR word is a median 0.72 of the scanned word's width, and a
  search-redaction on an OCR'd scan left the word's end uncovered in 30 of 40 searches. A hidden run is read as
  stretching to the next hidden run on its line, at most 2.5×: 0 of 40, and 0.5% of 2,995 words.
- **[ADR-092 — an OCR word is stamped to its scanned box](092-an-ocr-word-is-stamped-to-its-scanned-box.md)**
  — step six of ADR-088; closes ADR-091's "the stamp is still narrow" and narrows its reading to a SHORT stamp. pdfcpu
  places a word at unit scale and a whole-point size, so the fit is multiplied into the `cm` it wrote: the advance
  across the box, the glyphs' own ink on its top and bottom. Width ÷ ink 0.72 → 1.00, in Nib's map and in poppler.
- **[ADR-093 — a fitted OCR word is boxed by its ink](093-a-fitted-ocr-word-is-boxed-by-its-ink.md)**
  — extends ADR-092 and ADR-090. The map gave a fitted word the line's reach (a full size up, a quarter down), 1.65
  times its ink, and a redaction reached the line above; it is now boxed by its own glyphs' ink, which is the scanned
  box. Part of a right-to-left OCR word takes the whole word: the stamp runs from the left, the scan from the right.
- **[ADR-094 — a page that already has a text layer is not OCR'd again](094-a-page-with-a-text-layer-is-not-ocrd-again.md)**
  — a second OCR stamped every word twice. One rule (`pdfops.PagesWithTextLayer`), applied by the route whatever it is
  sent and asked by the window first (`GET /api/ocr/pages`), so no recognition pass is spent. Replacing a layer is the
  declared gap.
- **[ADR-095 — a match that wraps a line is found, and an estimate is drawn on its own line](095-a-match-that-wraps-a-line-is-found-and-an-estimate-is-drawn-on-its-own-line.md)**
  — extends ADR-090 and closes its declared gap. Both readings searched a row at a time, so a name split over a line
  end was found by neither; `wrappedMatches` is the one door across a line end. The estimate drew every match at its
  row's first baseline, and a row can hold two: the four words ADR-090 could not explain were estimates set 4.5–6pt high.
- **[ADR-096 — a white rectangle is the ground under a blank](096-a-white-ground-is-a-blank.md)**
  — extends ADR-088/089; supersedes ADR-088 §3 for white rectangles. A designer-made form lays a white rectangle under
  each blank, and the map dropped them and then refused the page as shaded; they are now kind `white`, `Filled` means
  a colour, and a line just under another closes it. Found 91.2% → 98.5%, well placed 84.5% → 98.2%. Nothing built
  for symbol-font checkboxes: the corpus has none (`fda-135045`'s are bullets).
- **[ADR-097 — a right-to-left OCR word is set where its letters are](097-a-right-to-left-ocr-word-is-set-where-its-letters-are.md)**
  — extends ADR-092/093; supersedes ADR-093 rule 5 for a Hebrew word written this way. The word is set last letter
  first inside `/ReversedChars`, as print sets it: poppler and pdf.js had been reading the old stamp backwards. Part of
  a Hebrew word is boxed by its glyphs; Arabic stays whole (its letters are printed joined). pdf.js's span is the
  line's em box, not a fault of the fit: nothing is built in the stamp for it.
- **[ADR-098 — print is boxed by the ink its font's own glyphs reach](098-print-is-boxed-by-the-ink-its-fonts-own-glyphs-reach.md)**
  — extends ADR-093 to print. A search-redaction's box was a line's reach, and took the tails of the line above for
  774 of 6,490 corpus words. A run now carries its own ink where an embedded TrueType program says what it is — the
  run's own glyphs where a code is a glyph number, else every glyph of the program and only to tighten. Descriptors
  and a constant were refused by measurement. 774 → 399, ink outside a box 0 → 0. `build/accuracy.sh` gains the
  vertical-reach column that measured it. CFF and Type 1 programs are the declared gap.
- **[ADR-099 — faint print in a blank is a hint, not a label; and a choice word is placed on its own baseline](099-faint-print-is-a-hint-and-a-choice-word-is-placed-on-its-own-baseline.md)**
  — extends ADR-088/089/095/096. The map says which print is faint (`MapText.Faint`, a fill at least 0.7 light and not
  white); a hint stands in no blank's way and two or more set apart divide it (`byHints`) — the 1040's six date fields.
  The choice detectors drew at `row.y`, ADR-095's defect in code it did not touch: each word now carries its own baseline.
- **[ADR-100 — a search that did not look across every line end says so](100-a-search-that-did-not-look-across-every-line-end-says-so.md)**
  — extends ADR-095. The chains `wrappedMatches` searches are cubic in stretches per line and none can be dropped
  without changing what is found, so they are bounded: `WRAP_BUDGET` 200,000 a call, three-line chains left out first,
  and the user is told which page ("check by eye") — a redaction search must never read as complete when it was
  not. The function was also rewritten for cost with its first version kept as the test's oracle (200 × 20: 2.2 → 1.3 s
  unbounded; the densest real page 56 → 19 ms, 9,386 chains).
- **[ADR-101 — a text layer is replaced only when asked, and only where it is Nib's own](101-a-text-layer-is-replaced-only-when-asked-and-only-where-it-is-nibs-own.md)**
  — supersedes ADR-094's "a layer cannot be replaced" gap only. `replace` on `POST /api/ocr`, offered by the window as
  "Run OCR on them again?"; a word is Nib's when ALL of it is the stamp's shape (`ownOCRWords`) and a page when taking those
  words out leaves no invisible text; structure goes with the content or the page is refused; a signed document is
  refused; `GET /api/ocr/pages` says which pages are `own` by running the removal on a copy.
- **[ADR-102 — an update downloads to the browser's own folder, and nothing asks](102-an-update-downloads-to-the-browsers-folder-and-nothing-asks.md)**
  — supersedes ADR-039's "the client sends a destination folder" and its `~/nib` default only. ONE door, `downloadDir`:
  Nib's setting, else the folder set in the browser the window was opened in (`browser.Opened`; exactly the named keys
  of `Preferences` / `prefs.js`, as untrusted text), else the system Downloads folder, else `~/nib`; a folder that is not
  there is skipped, never created. The pill starts the download; the popup says "Downloading to <folder>".
- **[ADR-103 — an export with several formats is one button, and a document is closed on its tab](103-an-export-with-several-formats-is-one-button-and-a-document-is-closed-on-its-tab.md)**
  — supersedes ADR-022's "Close Document" card and ADR-037's "the close controls keep the appear-at-two threshold" only.
  The format is chosen on the Save dialog's Format line (`openSaveAs`'s fourth argument; the bytes are the SELECTED
  format's, made at Save from what was captured at the press); never a per-format button or a dropdown in a card. One
  document closes with its tab's ×; Close all is the strip's SIBLING in `#tabrow`. PDF/A is in Save a Copy; the
  certificate is Secure's.
- **[ADR-104 — a menu entry that holds settings or a workflow opens a page with its own tab](104-a-settings-entry-opens-a-page.md)**
  — supersedes ADR-025's "its items as sidebar cards" and ADR-036's "a Main menu card" only. An entry
  (`.tbgroup[data-entry]`, one button) opens a `.apppage` in the main area with a tab in `#tabstrip`; it does not expand
  and it is not a popup. A page is NEVER a `view`: `openAppPages` / `activeAppPage` sit beside `views`, and Close all,
  `several` and `anyDoc` count documents only. `syncMainArea` is the one writer of what the main area shows (viewer,
  sheet or page); `docShowing()` is the one answer to "is a document on screen", so its controls are inert under a page.
  A page is registered in markup alone (`data-apppage`). About still opens its dialog (a page since ADR-108).
- **[ADR-105 — configuration and wizards open pages; actions on the open document stay in the menu](105-configuration-and-wizards-open-pages-actions-on-the-document-stay-in-the-menu.md)**
  — extends ADR-104; supersedes nothing. The test for ANY menu entry: something you set up or are led through opens a
  page with its own tab; a tool or button that acts on the open document stays in the menu and expands there. In
  Signing: Simple Sign and the ceremony's explanation are pages (`#signingStepsPage`, `#signingCeremonyPage`); Place
  Signing Flags and Send & Receive are unchanged, and the mode still lands on Flags and opens no tab. `appPageShow`
  mirrors `appPageLeave`; a step that leads to a tool for the document leaves the page; a tool in the menu beside a
  page takes the screen back (`setMarkerMode`); a page says when an Advanced feature is off (`data-adv` /
  `data-advoff`); a hidden menu's pages close. Remainder: the live ceremony panel and the two sheets are not pages.
- **[ADR-106 — a command runs the prerequisite the app can perform](106-a-command-runs-the-prerequisite-the-app-can-perform.md)**
  — extends ADR-009, ADR-001, ADR-094 and ADR-101. A command that needs a step the app can do does it first, visibly,
  through ONE door per prerequisite, and continues: `ensureText(owner, pages)` reads exactly the pages the server calls
  `unread` (`GET /api/ocr/pages`: no text set, not blank, no layer) for Read aloud, the table export, Reflow, the
  redaction search and the text export; `detectFields()` for Save as fillable form…; `openFirst()`; `openPeers(then)`.
  Never automatic: anything destructive, replacing a layer, a signed document without asking, a sign-locked one at
  all, anything that is the user's choice (the app takes them to it). A read stops when its view is no longer in
  front; a search that did not read a scan NAMES the pages it did not search. The OCR button is unchanged.
- **[ADR-107 — the pages share one tab, and opening another replaces what it shows](107-the-pages-share-one-tab.md)**
  — supersedes ADR-104's "a tab of its own" / "several pages can be open" and ADR-105's "with its own tab" only. At
  most ONE app page is open, in one `.pagetab` after the documents' — Settings and Signing pages take turns in it.
  `openAppPage` replaces what `openAppPages` holds (it never holds two; nothing pushes to it) and the tab is rebuilt
  where it was, named for the page showing. The replaced page is left ONCE by the usual door (`showAppPage` →
  `appPageLeave`) if it was in front, and not again if it was behind a document — `openAppPage` adds no leave call.
- **[ADR-108 — About is a page like the other eight, and its two documents open in place](108-about-is-a-page-like-the-other-eight.md)**
  — supersedes ADR-104 §9's "About opens the About dialog" only. `#settingsAboutPage` is registered in markup like
  every page; `#aboutModal` is gone. `#aboutMain` is still what `TestAboutCopyContainsTrustClaims` reads, from its
  opening tag to `#aboutDocs`' — keep both ids in that order. The licence and the notices open IN PLACE under their
  buttons (`aria-expanded`), fetched only when asked for and written as text; no swapped view, no Back. The
  dialog-focus tier's fixture is the profile editor.
