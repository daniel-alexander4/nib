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

## What this ledger is not

It is not a fixture mode. There is no `--fixture` switch that replays these on demand, so
re-proving a row is a manual edit-run-revert. That is a real gap and it is named here
rather than left for someone to discover: the cheap half (this record) is done, the
expensive half (a mode that reintroduces a defect on request and asserts the specific
assertion fires) is not.

---

## Tier 1 — `go test ./...`

| Defect reintroduced | Check that fired | What it said |
|---|---|---|
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



## Tier 3 — `./build/uirepro.sh`

| Defect reintroduced | Check that fired | What it said |
|---|---|---|
| The marker drag reads the ACTIVE view at fire time and `abortDrags` cannot reach the gesture (v1.108.5) | `a drag in flight when the user switches documents neither moves the flag nor records onto the new document` | "the flag moved from 334.29px to 554.29px after its document was switched away" — a measured number, which is why this row is worth more than the others |
| The placement handler acts on a pointerdown aimed at an overlay's × (v1.108.6) | `the close prompt fires from overlay history alone` — via `harness.mjs`'s `deleteMarker` | its own `=== 0` wait never completes, because the deleted flag is replaced by a new one |
| Rebuild-on-activation, the design ADR-002 refuses by name (P06) | 8 of 19 tier-3 tests | recorded at P06's close; the best red-proof in the tree at the time |
| Zoom discarded on activation (P06) | `switching away and back preserves the zoom you set` | 1375px → 1040px |
| A box drawn across a page boundary is unclamped (v1.109.5) | `a redaction drag that crosses onto the next page is bounded by the page it started on` | "ends at 719.5px, past page 1's bottom edge at 524.8px — it is painted over page 2" |

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
| `TestCraftedReasonIsNotReadAsAnAttestation`' first draft inlined the tag-and-token check | a test of the copy, not of the code — the same shape that let a gap in `riskyActions` hide from the test built on the same map | rewritten to drive `ReadAttestations` against a really-signed document |
