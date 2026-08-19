# Red-proof ledger

`CONTRIBUTING.md` says every tier of the verify contract "has been **proven red at least
once** against a deliberately reintroduced defect, and that matters more than any of them
being green: a check never seen to fail can only ever report pass."

**That sentence was, until v1.108.12, backed by nothing a reader could check.** There was no
record of which defect was reintroduced, which assertion fired, or when — the evidence
existed as prose in `PLAN.md`, scattered across phase notes, and `verify_test.go` guarded
only that the *claim was present in the file*, not that it was true. That is the same
failure one level out, in the guard that teaches the rule.

This file is the record. Every row is a defect that was actually put back into the tree and
a check that actually failed because of it, with the message it produced where the message
is the point. A row is only added after the red was observed — never from intent.

## How to read a row

- **Defect reintroduced** — what was changed, precisely enough to do it again.
- **Check that fired** — the tier and the assertion, by name.
- **What it said** — the failure message, quoted where it carries the diagnosis.

## Re-proving a row on demand

`./build/redproof.sh <name>` replays a recorded row: it exports HEAD to a throwaway tree,
applies the row's defect as a patch, runs the named check, and **asserts the check FAILS**.
`./build/redproof.sh` with no argument lists what is recorded.

**It is not a `--fixture` switch in the product, deliberately.** The obvious shape is a flag
the app reads that turns a defect back on, and this repo has paid for that shape once already:
`toolbarStyle` shipped half-built and its default would have hidden the toolbar for every
existing vault — "a loaded gun, not inert" (v1.109.1). A switch whose whole purpose is to
break the program is the same gun with a better excuse, and it would ship in the binary users
run. So nothing is added to the product; each row is a patch against the tracked tree,
applied to a copy.

It distinguishes the two ways a replay can go wrong, because they mean opposite things:

- **the patch did not apply** → the row is STALE; the code moved under it;
- **the patch applied and the check still passed** → the check no longer catches its own
  defect, which is this file's claim being false.

Each row's defect is generated with `git diff`, never typed: a hand-written diff gets its line
numbers wrong and then fails as "stale" for a reason that has nothing to do with the code —
the one failure this script must not invent.

**Not every row is recorded yet.** Two are (`empty-state-message`, tier 2;
`risky-actions-rendition`, tier 1), which is what proves the shape rather than what finishes
the job; the rest are still prose here and can be added one `.sh` + `.patch` pair at a time.
Saying so is the point — a partially-mechanised ledger that claimed to be complete would be
the same failure this file exists to fix.

---

## Tier 1 — `go test ./...`

| Defect reintroduced | Check that fired | What it said |
|---|---|---|
| No guard on any in-place rewrite door — `transformInPlace`, `runContinuousPagenum`, `watchTransform` (v1.109.33) | `TestInPlaceRewriteRefusesSignedPDF` (all three subtests) | "signed.pdf was rewritten — the signed document on disk was destroyed", and for the watch door "watch status = \"optimized\", want a skip" |
| pdfcpu stops tagging its stamps with an OCG (simulated by asking for a name it does not use) (v1.109.16) | `TestHasStampLayerSeesPdfcpuOwnStamp` | "a document stamped with page numbers reports NO stamp layer … if that stopped being true, this detector is silently answering false for every document and the double-stamp warning built on it never fires" |
| `flattenField` returns as soon as a field has children, dropping the parent's own `<value>` (v1.108.11) | `TestFlattenFieldKeepsAParentsOwnValue` | "the parent field's own value was dropped because it also has children; got `map[…]{"address.city":["Ipswich"]}`" |
| The flags cache keyed on "have I cached anything" instead of on the byte slice (v1.108.10) | `TestDocResponseFlagsCacheInvalidatesWithTheBytes` | "after the document's bytes were replaced with a flagless PDF, docResponse still reports flags …" |
| `Rendition` removed from `riskyActions` (v1.108.9) | `TestRiskyActionsCoverTheTypesThatCanRunOrHide` | names the missing type AND the count mismatch — two failures, because the list and the map are maintained separately on purpose |
| `Scan`/`StripActive` read only an action's head, not its `/Next` chain (v1.108.9) | `TestScanAndStripFollowNextChains` | "a JavaScript action behind a benign /GoTo head was not reported" |
| `StripActive` does not remove media annotations (v1.108.9) | `TestStripActiveIsAtLeastAsStrongAsRemoveFilesAndMedia` | "a Screen annotation survives StripActive but not RemoveFilesAndMedia — the stronger tier is weaker than the gentle one" |
| `eachPage` dedupes by object number, skipping a page referenced twice (v1.109.9) | `TestEachPageCountsDuplicatesAndStopsCycles` | "counted 2 times, want 3 — the reader shows three pages, so every finding after the duplicate is reported a page early" |
| The README's `-w` list names only optimize and sanitize (v1.109.9) | `TestREADMEListsEveryInPlaceCommand` | names all eight missing commands |
| Nested quotes uncapped (v1.109.8) | `TestDeepQuoteNestingStaysReadable` | "rendered across 6 pages — the indent consumed the wrap width" |
| `UnsupportedWith` ignores the supplied fonts (v1.109.8) | `TestUnsupportedWithHonoursTheSuppliedFonts` | lists the Thai runes as unprintable in a document that prints them |
| The session teardown is unconditional (v1.109.6) | `TestDisarmDoesNotCloseALaterSession` + `TestClearPendingDoesNotDropALaterSessionsConsent` | "a predecessor closed it — they wait for a peer that can no longer reach them" |
| `ReadAttestations` reads any `[SPKI:…]` without the kind tag (v1.109.6) | `TestCraftedReasonIsNotReadAsAnAttestation` | reports both the forged parse AND the forged `Matched` verdict |
| No handshake gate on accept (v1.109.7) | `TestAStrayConnectionDoesNotConsumeTheSession` | "a bare TCP connection that sent no TLS consumed the armed session" |
| `Record.version` unread / skew blamed always (v1.109.2) | `TestHandOffRefusalNamesTheVersionsAndOnlyBlamesASkewWhenThereIsOne` | both directions: missing versions, and a skew claimed for two identical ones |
| `toolbarStyle` removal — a strict decoder rejecting the removed key (v1.109.1) | `TestVaultWithRemovedToolbarStyleKeyStillOpens` + a probe | shows the compatibility this rests on is real rather than assumed |

## Tier 2 — `./build/jsdomtest.sh`

| Defect reintroduced | Check that fired | What it said |
|---|---|---|
| The close confirm reads the four "edited since open" signals instead of a dirty flag (v1.108.7) | `a save clears the prompt, even though the server still reports undo history` | "closing straight after a successful save still prompted. Nothing has changed since the save…" |
| A new unread field added to `docResponse` (v1.108.8) | `every field of every published shape has a reader…` | listed `docResponse.lastOpKind (declared internal/server/server.go, readers: web/app.js)` |
| A whole new json-tagged shape added to `internal/server` (v1.108.8) | `the table covers every shape the packages publish` | "these json-tagged shapes are neither in PUBLISHED nor in EXCLUDED… transferReport" |
| A renamed document id, from P03 | `docid.test.mjs` | fails naming the id |
| An `UNREAD_KNOWN` entry for a field that no longer exists (v1.109.1) | `every field of every published shape has a reader…` | "UNREAD_KNOWN names fields that no longer exist on any published shape" |
| The first-run intro left as a separate stacked overlay (v1.109.4) | `firstrun.test.mjs`, three of its four | the warning shown twice, two stacked overlays, and the pitch shown to a returning user |
| Latte's own `--surface1` (#bcc0cc) restored (v1.109.19) | `--text meets AA on every surface it is set on` | "--text in the light theme is 4.39:1 on --surface1, below AA's 4.5:1 — the BODY colour is unreadable there" |
| `color: var(--blue)` put back on the active toolbar tab (v1.109.19) | `the active toolbar tab does not carry its label in --blue` | quotes the offending rule back and says why: "under AA on --surface1 in both themes; the underline is what signals active" |
| Catppuccin Latte's own `--subtext0` (#6c6f85) restored in the light theme (v1.109.18) | `BOTH muted tokens meet AA on every surface muted text sits on` | "--subtext0 in the light theme is 4.37:1 on --base, below AA's 4.5:1 for normal text" |
| The two light muted tokens collapsed onto one value (v1.109.18) | `the muted scale stays a scale` **and** `--subtext1 is the HIGHER-contrast muted token` | "the light theme's muted scale is not ordered on --mantle: --text 6.57, --subtext1 5.81, --subtext0 5.81" — two guards, because a collapse is both an ordering failure and a hierarchy failure |
| A `:empty::after` message put back into `web/style.css` (v1.109.14) | `no empty-state message is generated content` | names the offending selector and says why: "cannot be selected or copied and is not reliably announced — build a real element instead" |

## Tier 3 — `./build/uirepro.sh`

| Defect reintroduced | Check that fired | What it said |
|---|---|---|
| **None — this one was LIVE.** No `cMapUrl`, and no vendored CMap tables, so a predefined-CMap CJK document decoded to nothing (v1.109.35) | `a document using a predefined CJK CMap yields its text` | "the page reads as \"\", not \"日本語\" … Every CJK document from an office suite is unreadable, unsearchable and uncopyable in the same way". Not a reintroduced defect: it went red against shipped code and green after the tables were vendored — a before/after on the same assertion |
| `rectPoints` drops the screen-to-PDF y flip — `(1-fy1)*pageH` becomes `fy0*pageH` (v1.109.34) | `a stamp bakes where it was placed, not mirrored up the page` | "the baked stamp's centre is 0.256 down the page but it was placed at 0.740 — off by 48.4% of the page height. It is in the OPPOSITE half: the screen-to-PDF y flip in rectPoints is wrong, and every stamp, border, checkbox and circled choice bakes at the wrong end of the page." |
| The marker drag reads the ACTIVE view at fire time and `abortDrags` cannot reach the gesture (v1.108.5) | `a drag in flight when the user switches documents neither moves the flag nor records onto the new document` | "the flag moved from 334.29px to 554.29px after its document was switched away" — a measured number, which is why this row is worth more than the others |
| The placement handler acts on a pointerdown aimed at an overlay's × (v1.108.6) | `the close prompt fires from overlay history alone` — via `harness.mjs`'s `deleteMarker` | its own `=== 0` wait never completes, because the deleted flag is replaced by a new one |
| Rebuild-on-activation, the design ADR-002 refuses by name (P06) | 8 of 19 tier-3 tests | recorded at P06's close; the best red-proof in the tree at the time |
| Zoom discarded on activation (P06) | `switching away and back preserves the zoom you set` | 1375px → 1040px |
| *(not a defect — the test's own race, v1.109.23)* `switched()` is a state check that can already be true of the view being left, so the zoom test acted while a second document was still loading | probed directly: `viewerContainer-1:3p:VISIBLE viewerContainer-2:0p` at the test's first line | the fix is `settled()`, and the test now asserts the view it measures IS the view it zoomed — a recurrence fails by name instead of flaking |
| A box drawn across a page boundary is unclamped (v1.109.5) | `a redaction drag that crosses onto the next page is bounded by the page it started on` | "ends at 719.5px, past page 1's bottom edge at 524.8px — it is painted over page 2" |
| `bakedBytes` never bakes annotation storage — `getData()` unconditionally (v1.109.13) | `a form fill reaches the file on disk when you Save` | "the saved file does not carry \"typed into the form and saved\" — it is byte-identical to the 849 bytes that were there before the save, so nothing was written at all" |
| `handleFinalize` returns the baked bytes without signing them (v1.109.17) | `finalize signs the open document and writes it where you choose` | "the saved file carries no /ByteRange, so it is not signed. 2008 bytes were written — the export path ran and produced an unsigned document" |
| `markStale` neutered, so a failed render says nothing (v1.109.19) | `a render that fails after the metadata changed says so, and can be retried` | "the render failed and nothing on screen said so — the view is showing the previous version under the new document's identity, silently" |
| The stale banner never cleared on a successful reload (v1.109.19) | same test, second half | "the banner survived a successful retry, so it reports staleness that is over — and the next real one will be ignored" |
| `/api/stamps` always answers false (v1.109.16) | `a document that already carries a stamp says so before you add another` | "the page-number dialog does not warn on a document it has already stamped … adding numbers again would silently draw a second set on top of the first" |
| **None — this one was LIVE.** `.thumbacts` is `display: none` until `:hover`, so rotate/delete could not take keyboard focus at all (v1.109.16) | `rotating one page from its thumbnail turns that page` | "the rotate button cannot take keyboard focus (laid out: false, focused: false) … per-page rotate exists nowhere else in the UI" |
| **None — this one was LIVE.** Autofill wrote to `annotationStorage` and never touched the rendered input (v1.109.13) | `autofill from the saved profile visibly updates the rendered form` | "the rendered input still shows [\"typed into the form and saved\"] … ten seconds after the toast said the fields were filled". Not a reintroduced defect: the check went red against shipped code on its first run, which is the strongest form of the same evidence |
| The `pagesloaded` widest-page refine runs unconditionally, over a zoom the user set while the document was still loading (v1.109.12) | `a zoom set while the document is still loading is not thrown away` | "page 1 is 520px, not the 1375px the user zoomed it to: the fit that lands when the document finishes loading overwrote a scale the user had already chosen" |

## Vacuous greens caught, and how

Not red proofs — the opposite, and worth as much. Each is a check that was **passing while
measuring nothing**, found by its own setup assertion or by a deliberate probe.

| The check | Why it was vacuous | What exposed it |
|---|---|---|
| `TestScanAndStripFollowNextChains`' first draft asserted `bytes.Contains(pdf, "/JavaScript")` | pdfcpu writes OBJECT STREAMS, so the string is absent from a document that *does* carry the action — green whatever the strip did | a probe printing the byte presence BEFORE the strip; replaced with `annotFacts`, which walks the object graph |
| `TestStripActiveIsAtLeastAsStrongAsRemoveFilesAndMedia`' first draft used direct annotation dicts | `RemoveAnnotations` works off `ctx.PageAnnots`, filled only for annotations reached by an INDIRECT reference — so neither tier removed anything and the test passed against unfixed code | its own setup assertion, then a probe printing what each tier left |
| `test/ui/gestures.test.mjs`' cleanup loop conditioned on the tab count | `syncTabs` renders no tabs below two documents, so the loop stopped with the last document still open | 13 sibling tier-3 tests failing on the next run |
| `published.test.mjs` reported `Record.version` as read (v1.109.2) | it matched a DOC COMMENT — "see Record.Version" — not a read. Second instance of the same `strings.Contains`-over-comments hole in two sweeps, after the `.deb` guard | deleting the actual read and watching the scan still pass |
| `TestDeepQuoteNestingStaysReadable`' first draft used one short sentence | nine words emitted one rune at a time is ~45 lines, which fits on one page — so a page count could not tell fixed from unfixed | red-proving it and getting a pass |
| `firstrun.test.mjs`' first draft drove state with a `visibilitychange` event | NOTHING in app.js listens for it (P07 left that reconcile unbuilt), so the status never changed and the test "failed" against an unchanged DOM | the failure it reported was implausible |
| The keyboard-reachability assertion's first draft read `getComputedStyle(...).opacity` | a `display: none` element reports **opacity 1** — display and opacity are independent — so the assertion was green with the defect put back, and `page.focus()` does not require visibility either | probing it: the defect was restored and the test still passed. Replaced with `getClientRects().length > 0` and `document.activeElement === b`, which are what "focusable" actually means |
| `a zoom set while the document is still loading is not thrown away`' first draft used a 200-page fixture | 200 pages load fast enough that `pagesloaded` had already fired before the test could zoom — the window it exists to act inside had closed, so its final assertion could not have failed whatever the code did | its own setup assertion, which reads whether the widest-page refine has run rather than assuming it has not: "page 1 is 520px in a 1080px container — the widest-page refine has ALREADY run". Widened to 2000 pages |
| `TestCraftedReasonIsNotReadAsAnAttestation`' first draft inlined the tag-and-token check | a test of the copy, not of the code — the same shape that let a gap in `riskyActions` hide from the test built on the same map | rewritten to drive `ReadAttestations` against a really-signed document |
