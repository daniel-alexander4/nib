# Red-proof ledger

`CONTRIBUTING.md` says every tier of the verify contract "has been **proven red at least
once** against a deliberately reintroduced defect, and that matters more than any of them
being green: a check never seen to fail can only ever report pass."

**That sentence was, until v1.108.12, backed by nothing a reader could check.** There was no
record of which defect was reintroduced, which assertion fired, or when — the evidence
existed as prose in the multiple-documents plan (since retired), scattered across phase
notes, and `verify_test.go` guarded
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

## Tier 4 — `./build/pairrepro.sh`

| Defect reintroduced | Check that fired | What it said |
|---|---|---|
| The session is dialled at a port nothing is listening on, so the ceremony never starts (v1.109.49) | the spoken-check stimulus assertion | "instance A was never shown the verification words — the ceremony reached the document exchange without the spoken check (L2)". The point is the ORDER: the harness refuses to grade the ceremony before confirming the gate happened, so a ceremony that never began cannot pass as one that completed |
| The verification is never confirmed on either side (v1.109.49) | the ceremony itself | `initiate returned HTTP 502` — the gate holds both parties and the session times out, which is the behaviour P01.S05 built |
| The server ignores the `transport` field, so both runs use TCP (v1.109.53) | the socket probe in `ceremony()` | "[quic] port 18544 answers TCP — the QUIC run is listening on a TCP socket, so it is the TCP path wearing a different label". **Everything else in that function is transport-blind** — same API calls, same words, same signature count — so without this probe the harness would report QUIC coverage it did not have |
| Both instances given the same identity (v1.109.49) | `both instances have the SAME identity … one key agreeing with itself` | **and the more interesting half: the realistic route to that state does not reach the assertion.** Pointing both instances at one home fails EARLIER — the second enrolment returns 409 because a vault already exists. The assertion was probed directly instead, by reading B's fingerprint from A, and fires. Recorded as defence-in-depth rather than as the primary guard |


## Tier 1 — `internal/udpmux` (P02.S03, the socket demultiplexer)

Nine probes, each restoring one defect the router exists to refuse. All nine went red; the
patch-applied check is not decoration — the first attempt at this battery verified the patch
with `git diff`, which is silent on an **untracked** package, so every probe reported
"did not apply" and none ran.

| Defect reintroduced | Check that fired | What it said |
|---|---|---|
| The router keys on `b&0x40` — the cheap discriminator, as the whole router (v1.109.51) | `TestQUICAndKRPCInterleavedOnOnePort` | `a KRPC ping on the shared port failed: after 1 tries: transaction timed out (A: {… RoutedByPeer:0 RoutedToDHT:0})` — a bencode `'d'` is routed to QUIC and every DHT query on the shared port dies |
| Everything routed to QUIC | `TestTheCheapDiscriminatorIsWrong` | "a KRPC ping did not reach the DHT view — the cheap discriminator is in the path" |
| An inbound long header ALSO learns the sender | `TestAnInboundLongHeaderTeachesNothing` | "Peers = 50 after 50 inbound long headers, want 0 — remote input is growing the table" |
| An outbound QUIC write teaches the router nothing | `TestAShortHeaderReachesQUICOnlyAfterWeHaveWrittenThere` | "Learned = 0 — the write taught the router nothing", so the steady state of every session goes to the DHT |
| An expired peer is never refreshed (the half-TTL fast path made unconditional) | `TestAPeerExpires` | "a re-written address did not become a peer again" — a session outliving `peerTTL` could never be re-learned |
| Closing one view closes the shared socket | `TestClosingOneViewLeavesTheOtherRunning` | "the shared socket died with one view" — which is what `quic.Transport.Close` or `dht.Server.Close` would do to the other protocol without the shim |
| The deadline error is not `os.ErrDeadlineExceeded` (so not `Temporary()`) | `TestADeadlineExpiresAndUnexpires` | "the deadline error is not Temporary(); quic-go's listen loop would treat a transport close as a fatal read error instead of a shutdown" |
| `SetReadDeadline` made a no-op | `TestQUICTransportCloseReturnsOverTheShim` | "quic.Transport.Close hung on the shim — its read loop was never unblocked". It would have hung **in a defer at the end of a passing test**, which is the worst place to learn it |
| A deadline that cannot be un-expired (the zero-time branch of `set` deleted) | `TestADeadlineExpiresAndUnexpires` | "a read after the deadline was reset to zero never returned" — see the vacuous-green row below, because this probe passed the first time it was run |


## Tier 1 — `internal/p2p` (P02.S04, the session core re-typed off `*tls.Conn`)

Seven probes. The re-typing moved every document-carrying entry point onto a `Channel`, so
each probe puts back a property the old signature enforced by type and the new one enforces
by check.

| Defect reintroduced | Check that fired | What it said |
|---|---|---|
| `Initiate` stops calling `Channel.check` (v1.109.52) | `TestAnIncompleteChannelIsRefusedByEveryEntryPoint/Initiate` | "a Channel with no peerfp did not return — it got past the boundary and blocked on the stream". **The hang is the finding**: see the vacuous-green row below |
| `check` stops caring about the exporter | the same test, all four entry points | a session would run with the spoken check bound to no channel at all |
| The core names `*tls.Conn` again | `TestTheSessionCoreDoesNotNameATransport` | "session.go names *tls.Conn … a transport type in here is D14's two-transports-one-core rule broken at the signature" |
| A fifth document-carrying entry point taking a `Channel`, with no `Verifier` | `TestL2CoversEveryDocumentCarryingEntryPoint` | "5 exported functions take a Channel or a Stream ([SendQuietly Initiate Receive SendDocument ReceiveDocument]) but 4 are pinned above" |
| The same, taking a raw `Stream` — the hole one type further down | the same guard | caught, which is why the re-based regex matches `Stream` as well as `Channel` |
| `TLSChannel` returns a fingerprint that is not the verified one | `TestBothSidesDeriveTheSameWords` | the two ends derive different words, so the spoken check fails for honest users — which is what a channel built on an unverified identity is worth |
| The document write moved before the spoken check | `TestL2NoDocumentBytesCrossBeforeBothConfirmations` | L2 itself, still firing under the new shape — the guard was re-pointed at the new signatures and re-probed rather than assumed to have survived |


## Tier 1 — `internal/p2p` (P02.S05, QUIC behind the core)

| Defect reintroduced | Check that fired | What it said |
|---|---|---|
| The QUIC teardown made abrupt again — `CloseWithError` without closing the stream first (v1.109.53) | `TestSessionRoundTrip/quic` (was `TestAWholeCeremonyOverQUIC`, folded in at v1.109.54) | `initiate: receive co-signed document: Application error 0x0 (remote)` — the initiator watching the finished document evaporate one frame from the end. **This was found as a real failure, not planted**: closing a QUIC connection is not the polite thing closing a TCP one is. **Re-probed against the new reader after the fold**, and the parameterisation is what makes it worth reading: `--- PASS: TestSessionRoundTrip/tcp` beside `--- FAIL: TestSessionRoundTrip/quic`, so the failure names the transport |
| The QUIC listener armed with no pin | `TestQUICRejectsAPeerThatIsNotPinned` | a third identity is handed up by Accept, which is the pinned-peer model gone on this transport |
| A transport missing from the table (v1.109.54) | `TestEveryTransportIsInTheTable` | "this package exports 2 listener constructors ([Listen QUICListen]) but the transport table has 1 entries. A transport missing from the table is one the session-logic tests never run over, and the suite stays green while covering the others" |
| The table running TCP twice, with one row labelled quic | the same guard | "transports \"tcp\" and \"quic\" share one listener — \"quic\"'s subtests are labelled for a transport they never use". See the vacuous-green row: the first draft compared names and allowed exactly this |
| `quicChannel` reports a fingerprint that is not the verified peer's | `TestAQUICSessionEstablishesAChannelOnBothEnds`, `TestSessionRoundTrip/quic` | the two ends hold the wrong peer, and the co-signature fails to cross-bind |


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
| The un-expiry half of `TestADeadlineExpiresAndUnexpires` (v1.109.51) | it reset the deadline to zero and then called the `arrives` helper — which sets a deadline **of its own**, and setting a FUTURE deadline recreates the channel too. So the assertion was satisfied by the helper, not by the reset under test, and stayed green with the zero-time branch of `set` deleted | probing that branch and getting a PASS. Replaced with a read that must block and then succeed: reset, start a blocking read, sleep, then send |
| The whole nine-probe battery, on its first run | its patch-applied guard was `git diff --quiet internal/udpmux/`, and `internal/udpmux/` was **untracked** — so `git diff` was silent by construction and the guard could never fire. It reported "did not apply" for all nine, which is the guard failing SAFE; a guard of the opposite polarity would have reported nine greens | the guard itself, which is why it was written before the probes rather than after |
| `TestAnIncompleteChannelIsRefusedByEveryEntryPoint`' first draft called each entry point synchronously (v1.109.52) | when `check` is weakened the call gets **past** the boundary and blocks forever writing to a pipe with nobody on the other end — so the test **hung** instead of failing. A hang reports nothing and takes the suite with it; this is the tier-2 armed-session hazard in a different package | the probe battery itself timing out on its second probe. Each call now runs with a 3-second bound and names the hang: "it got past the boundary and blocked on the stream" |
| The population-floor guard's fifth-path regex, on re-typing | it matched `^func ([A-Z]\w+)\(conn \*tls\.Conn`, and after S04 **nothing in `session.go` matches it** — it would have gone quietly to zero and compared 0 against 4, reporting a failure whose message named the wrong problem | re-basing it deliberately as part of the slice, then probing both a `Channel` and a raw `Stream` fifth path. A guard whose subject is re-typed has to be re-typed with it |
| Tier 4's second ceremony, before the socket probe (v1.109.53) | every assertion in `ceremony()` is transport-blind — the same API calls, the same four words, the same signature count — so a build that ignored the `transport` field would run TCP twice and pass. It would have reported QUIC coverage it did not have | asking what the run would do with the field ignored, then making the answer red. The probe connects to the armed port over TCP and requires success on the TCP run and failure on the QUIC one — the socket family is the one thing the app cannot self-report its way past |
| The QUIC teardown's first fix waited on BOTH sides | it was correct and cost the full 5-second grace on every ceremony, because each side waited for the other. A green suite hid it as latency rather than failure | timing the test: 5.05s, against 0.06s once the wait was made asymmetric. The protocol is what settles it — in all four entry points the listening side writes last and the dialing side reads last, so only the listener has anything owed |
| The transport table's own guard, first draft (v1.109.54) | it matched `^func ([A-Z]\w*Listen)\(`, which **cannot match `Listen` itself** — it reported one constructor where there are two and failed with a message naming the wrong problem. The same shape as P02.S04's re-typed regex, one slice later | running it. It went red on correct code, which is the cheap direction for this mistake to fall |
| The transport table's distinctness check, first draft | it compared **names**, so a table of `{"tcp", Listen, Dial}` and `{"quic", Listen, Dial}` passed: two entries, two source constructors, two distinct names, and the whole suite running over TCP twice with every subtest labelled quic. A parameterised suite is only as good as its parameters being different | asking what the guard would allow, then probing it — S2 in the tier-1 table above. Fixed by comparing `reflect.ValueOf(fn).Pointer()` |
