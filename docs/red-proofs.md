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

**`./build/redproof.sh --all` replays every row**, reports the whole set rather than stopping at
the first failure, and exits non-zero if any row did not re-prove. It exists because the sweep it
belongs in had no door: `verify_test.go`'s count guard can see a row that DISAPPEARS and not one
that no longer re-proves, said so in its own comment, and stayed a known gap for as long as
running the set meant hand-rolling a shell loop. The first person to actually run it found eight
invalid rows of eighty-one (v1.117.156).

**Nineteen rows are replayable** (v1.117.43; nine at v1.117.26, six added at v1.117.39). Ask
`./build/redproof.sh` with no argument rather than reading a list here — **this sentence said
"nine" while the directory held fifteen**, because a count written into prose beside a set that
grows is a second statement of one fact, and it is the one nobody updates. The count is guarded in
`verify_test.go` with a floor that **moves with the set** — left at two while the set grew,
it would have tolerated losing four of six silently, which is the same defect as the prose
count it replaced. Raising the floor is the tax a new row pays.

The rest of this file is still prose. That is the honest state and the gap is the same one
named below: a row recorded as prose has been proven red **once**, and nothing re-checks it.
The four added in v1.117.20 were recorded the day their defects were caught, so unlike the
older rows their patches are known to apply to the tree they describe.

**It is not a `--fixture` switch in the product, deliberately.** The obvious shape is a flag
the app reads that turns a defect back on, and this repo has paid for that shape once already:
`toolbarStyle` shipped half-built and its default would have hidden the toolbar for every
existing vault — "a loaded gun, not inert" (v1.109.1). A switch whose whole purpose is to
break the program is the same gun with a better excuse, and it would ship in the binary users
run. So nothing is added to the product; each row is a patch against the tracked tree,
applied to a copy.

It distinguishes the two ways a replay can go wrong, because they mean opposite things:

**Three outcomes, not two.** The third was added at v1.117.2 and its absence was this file's
own V1 defect: the harness asserted only that the check exited non-zero, so a check that had
been **deleted** reported "still goes red against its own defect". Measured — `node --test
<deleted file>` exits 1, and the tier-2 row's patch touches `web/style.css` rather than the test
file, so removing the test made the row print `ok`. Any compile break or missing `node_modules`
in the exported tree does the same. Every row now records an `EXPECT` token that the real
assertion prints, a row without one is a hard error, and the replay set's size is pinned in
`verify_test.go` rather than in this sentence.

- **red, but not for its own reason** → the check no longer exists, or broke before it ran;
- **the patch did not apply** → the row is STALE; the code moved under it;
- **the patch applied and the check still passed** → the check no longer catches its own
  defect, which is this file's claim being false.

Each row's defect is generated with `git diff`, never typed: a hand-written diff gets its line
numbers wrong and then fails as "stale" for a reason that has nothing to do with the code —
the one failure this script must not invent.

**Not every row is recorded, and the sentence saying so has to keep its own number.** It began
at two (`empty-state-message`, tier 2; `risky-actions-rendition`, tier 1) — enough to prove the
shape, not to finish the job — and the set is now **117** replayable pairs against a ledger of
prose rows that still outnumber them. The rest can be added one `.sh` + `.patch` pair at a time.
Saying so is the point — a partially-mechanised ledger that claimed to be complete would be
the same failure this file exists to fix, and the count is pinned in `verify_test.go` rather
than here so that a stale sentence cannot be the only thing that knows.

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
| Both instances given the same identity (v1.109.49) | `the $N instances reported only $DISTINCT distinct identities … one key agreeing with itself` *(re-quoted 2026-08-23 — P07.S01 generalised the check to a set compared against `$N`; the two-instance wording this row used to quote is no longer in the harness, and a ledger that quotes a message nothing emits is the stale-description shape the repo rule warns about)* | **and the more interesting half: the realistic route to that state does not reach the assertion.** Pointing both instances at one home fails EARLIER — the second enrolment returns 409 because a vault already exists. The assertion was probed directly instead, by reading B's fingerprint from A, and fires. Recorded as defence-in-depth rather than as the primary guard |


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


## Tier 5 — `./build/mcastrepro.sh` (P03.S02, link-local discovery)

| Defect reintroduced | Check that fired | What it said |
|---|---|---|
| The self-filter keyed on the SOURCE ADDRESS instead of the per-process nonce (v1.110.1) | `TestTwoProcessesDiscoverEachOther` | the browser discarded the *peer's* announcement as its own and timed out after 8 s. Both processes are on one host, so their datagrams arrive from the same local address — which is exactly why the nonce exists, and this is the probe that shows it rather than asserting it |
| `SO_REUSEADDR` not set on the discovery socket | the same test | the second process could not bind port 8446 at all. `net.ListenPacket` does not set it (the stdlib only does when the *bind address* is multicast, and this binds the wildcard), so without it two Nibs on one machine can never discover each other |

## Tier 1 — `internal/discovery` (P03.S02, interface selection)

| Defect reintroduced | Check that fired | What it said |
|---|---|---|
| `FlagRunning` made a filter (v1.110.1) | `TestInterfaceSelectionSkipsWhatItShould` | "docker0 was skipped, which means FlagRunning became a filter — it is degenerate on Windows and would make the selection platform-dependent". **This probe was first aimed at tier 5 and came back GREEN** — see the vacuous-green row below |


## Tier 4 `--lan` — `./build/pairrepro.sh --lan` (P03.S04, a ceremony with no address typed)

| Defect reintroduced | Check that fired | What it said |
|---|---|---|
| The armed side stops announcing (v1.110.3) | the LAN ceremony itself | `initiate returned HTTP 502 before any spoken check: {"error":"that peer is not announcing on this network (listened for 2s on [d0])"}`. **Discovery is load-bearing**: with no bind and no address typed, nothing else can supply one |
| `peerAddress` returns the empty address with no error | the same run | `could not connect to peer: dial tcp: missing address` — the shape of every "resolved to nothing and carried on" defect |
| The app opens a connection to 1.1.1.1 during the ceremony | the egress counter | "the ceremony emitted 2 packets destined off the link — P03's exit criterion says a LAN ceremony completes with NO outbound internet traffic" |


## Tier 1 — `internal/discovery` (P03.S05, the Windows divergences)

Both are invisible on Linux by construction, so the guards are the only thing standing
between them and a silent failure on a platform no tier can reach.

| Defect reintroduced | Check that fired | What it said |
|---|---|---|
| A filter on the arrival interface (v1.110.4) | `TestNothingDecidesOnTheArrivalInterface` | "mcast.go uses the arrival interface (IfIndex). On Windows the control message is nil with a NIL ERROR, so any decision made on it silently accepts everything there" |
| One of the two `SetControlMessage` errors discarded | the same guard, after it was made to count | "1 of 2 SetControlMessage calls have their error checked" — see the vacuous-green row: at first it asked whether *any* were, and the probe passed |
| The IPv4 selection stops requiring an IPv4 address (tier 5) | `TestAnIPv6OnlyInterfaceIsSkippedForTheIPv4Group`, on a real v6-only interface in the namespace | the v4 group would be joined on an interface Windows refuses, so the two platforms would choose different interfaces — the divergence the requirement exists to remove |


## Tier 5 and tier 1 — P03's phase close (the two-agent review's findings)

| Defect reintroduced | Check that fired | What it said |
|---|---|---|
| The read loop stops rejecting off-link sources (v1.110.5) | `TestAnOffLinkUnicastIsDroppedByTheReadLoop`, tier 5 | an off-link unicast comes back as a peer. **The first version of this probe passed** — see the vacuous-green row: the predicate had a unit test and no test that it was wired in |
| `onLink` defaults open when no link was joined | `TestOffLinkSourcesAreRejected` | a socket that joined nothing would accept everything — the failure direction must be closed |
| Dedupe by fingerprint alone | `TestTwoHostsClaimingOneNameBothBecomeCandidates` | the impostor's address survives and the real peer's is discarded, so no caller could reach it |
| `dialAny` tries only the first candidate | `TestDialAnyTriesEveryCandidate` | "tried 1 address(es)" — returning every candidate is useless if only one is dialled |
| `Encode`'s datagram-cap check removed | `TestEncodeRefusesWhatParseWouldReject` | "Encode produced 261 bytes and Parse says 261 bytes exceeds the 256-byte cap — the encoder and the parser disagree about what is legal" |
| `internal/discovery` imports the vault | `TestResolutionLivesOutsideTheDiscoveryPackage`, rewritten over the AST | the guard now checks the invariant instead of a string in a neighbouring file |


## Tier 1 — `internal/udpmux` and `internal/rendezvous` (P04.S01)

| Defect reintroduced | Check that fired | What it said |
|---|---|---|
| The connection-id rule removed, back to the address rule (v1.111.0) | `TestKRPCFromAQUICPeerReachesTheDHT` | the ping to a host we hold a QUIC session with times out — **this is the collision P02.S03 documented and left, driven at last** |
| An unknown connection id falls back to the address rule | the same test | the fallback must stop the moment ids are registered, or the collision survives the fix |
| Ids registered but never consulted | the same test | a table nothing reads is the address rule wearing a new name |
| One of the two `quic.Transport` constructions loses its generator | `TestBothTransportsWireTheConnectionIDGenerator` | "1 of 2 … install a connection-id generator". **The failure is silent by design** — the mux falls back so the session keeps working, and only a DHT reply from a session peer is swallowed |
| `Open` accepts a nil connection | `TestTheDHTIsNeverGivenANilConn` | `dht.NewServer` opens its OWN socket, and caveat 7 is that the probe and the session must be one socket. **This one failed against the code as first written** |
| An empty routing table truncates a good cache | `TestAnEmptyTableDoesNotTruncateAGoodCache` | one bad network day would become a permanently cold start |
| A corrupt cache is misread rather than refused | `TestACorruptCacheIsRefusedRatherThanMisread` | a partial record becomes a node at an address nobody is at |
| The hostname bootstrap made reachable | `TestBootstrapResolvesNoHostname` | D6 forbids it: a DNS lookup is a third party learning who starts a ceremony and when |


## Tier 1 and the live harness — `internal/rendezvous` (P04.S02, the self-address probe)

Ten defects reintroduced one at a time against the shipped tests, each reverted before the
next. The tenth was not reintroduced at all — it was **observed happening**, which is the
only reason the send-limiter fix exists.

| Defect reintroduced | Check that fired | What it said |
|---|---|---|
| The DHT view handed to `dht.NewServer` unscreened (v1.111.1) | Tier 1 — `TestTwentyOneBytesCannotKillTheProcess` | the test binary **died**: `panic: runtime error: makeslice: len out of range`, in `krpc.(*NodeAddr).UnmarshalBinary` on `dht.(*Server).serve`. 21 bytes of UDP from any host |
| `netip.AddrFromSlice`'s refusal dropped, so a truncated `ip` is believed | Tier 1 — `TestABogonSelfAddressIsRefusedUnderItsOwnCause/a_FOUR-byte_ip_field` | "rejected length=0 port=0 scope=1 … refused, but filed under the wrong cause, and a 4-byte field (silent corruption) must never be summed with a bogon" |
| `sourcePrefix` counting per node instead of per /24 and /48 | Tier 1 — `TestClassifyIsThreeValuedAndCountsSources/two_nodes_in_one_/24_are_ONE_source` | "Sources = 2, want 1 — two nodes behind one NAT are one destination; counting them as two would resolve the clause with a number that cannot answer it" |
| The largest group wins with no majority test | Tier 1 — the same test, `one_liar_against_ONE_honest_node_wins_nothing` | "Mapping = endpoint-independent, want endpoint-dependent — with two sources and no majority the honest answer is that we cannot tell" |
| One observation treated as enough to classify | Tier 1 — `TestTwoNodesAreNeededToClassify` | "Mapping = endpoint-independent with one source, want unknown — this is caveat 9's degradation to D19 cause 4" |
| `writeNodes` back to `To4()`, dropping every IPv6 node | Tier 1 — `TestTheCacheCarriesIPv6` | "writeNodes wrote 1 of 2 nodes — the v6 one was dropped, which on a v6-only host means no cache is ever written and every run is a cold start" |
| `Ping` returning `res.Err` instead of `res.ToError()` | Tier 1 — `TestAKRPCErrorReplyIsNotALiveNode` | "a node that answered with a KRPC error counted as live — the reply arrived, so the transaction succeeded" |
| The L1 guard reading parameter **names** only, as `internal/discovery`'s does | Tier 1 — `TestNothingHereCanReachAnIdentity`'s own stimulus | "the walk collected no TYPE text — it is reading names only, which is the exact hole this guard was written to close" |
| `Open` refusing to start over an unreadable cache again | Tier 1 — `TestAnUnreadableCacheIsAColdStartNotARefusalToRun` | "Open refused to start over a corrupt cache — one damaged byte in a bootstrap hint file must not be the reason Nib cannot open a document" |
| **Not reintroduced — observed.** The library's process-global `DefaultSendLimiter` (v1.111.0) | Tier 1 — `TestTheNodeCacheSurvivesARestart`, under the full package run only | "ping: after 1 tries: transaction timed out". The counters added to that failure message named it: `B mux RoutedToDHT:1` with `A mux RoutedToDHT:0` — B received the query and its reply was never sent, because `reply()` writes with `wait=false` and drops when the shared burst is gone |

### The slice's own review added five more

| Defect reintroduced | Check that fired | What it said |
|---|---|---|
| The `OnQuery` gate unwired, reopening the handler door (v1.111.1) | Tier 1 — `TestAQueryWithNoArgumentsCannotKillTheProcess` | the test binary **died**: `panic: runtime error: invalid memory address or nil pointer dereference`, `dht.(*Server).handleQuery` at `server.go:540`, from 34 bytes |
| The gate refusing every query instead of malformed ones | Tier 1 — `TestAWellFormedQueryIsStillAnswered` | "no reply to a well-formed ping — a node that stops answering gets dropped from other nodes' routing tables, and our own reachability goes with it" |
| The gate counting into `Screened` rather than its own cause | Tier 1 — `TestAQueryWithNoArgumentsCannotKillTheProcess` | "Screened = 2 — these went through the decoder screen, and counting them there would merge two different doors into one number" |
| `MaxStrLen` dropped from the screen's decoder | Tier 1 — `TestAnAllocationBombIsDroppedNotDecodedTwice` | "the bomb passed the screen — the library then decodes it too"; 10 screened bombs allocated 1.3 GB |
| `0.0.0.0/8` and `240.0.0.0/4` removed from `reservedRanges` | Tier 1 — `TestIsPublishableAgainstTheRangesGoDoesNotCover` | "isPublishable(0.1.2.3) = true (IsGlobalUnicast reports true) — this becomes a candidate both sides punch hundreds of packets at" |
| A `Fingerprint` field added to an exported struct | Tier 1 — `TestNothingHereCanReachAnIdentity` | "exported type Observation names or carries an identity — L1 forbids this package holding one at all" |

## Vacuous greens caught, and how

Not red proofs — the opposite, and worth as much. Each is a check that was **passing while
measuring nothing**, found by its own setup assertion or by a deliberate probe.

| The check | Why it was vacuous | What exposed it |
|---|---|---|
| `TestAKRPCErrorReplyIsNotALiveNode`'s stimulus (P04.S02) | It called `net.DialUDP` and checked the error, labelled "the node is reachable and DOES answer". `connect(2)` on a UDP socket **sends no packet** — it succeeds against a closed port, an unbound port, and a fake node whose goroutine has already exited | the slice's own diff review. Replaced with a real send and read |
| `screen.go`'s comment, "nothing reaches the library's decoder unscreened" (P04.S02) | True, and it read as a clean bill of health for a class it half-covered. A query that decodes perfectly still killed the handler — 34 bytes | two review agents independently, and the second door was measured by fuzzing every query type rather than reasoned about |
| `observation`'s explicit `len(ip.IP)` check (P04.S02) | **Unreachable.** `netip.AddrFromSlice` already accepts 4 and 16 bytes and nothing else, so the branch below it could never run — two doors onto one refusal, one of them untestable | the red-proof pass: disabling the length check left `TestABogonSelfAddressIsRefusedUnderItsOwnCause` **green**. Collapsed to one door, which then went red |
| `build/dhtlive.sh`'s evidence block (P04.S02) | Printed **nothing**. The `sed` stripped leading whitespace but not `go test`'s `live_test.go:53: ` prefix, so no line matched and the harness reported PASS with no evidence under it | running it and reading the output instead of the exit status |
| The `internal/rendezvous` package doc's claim that "a guard enforces that" (P04.S01) | There was **no such guard** — the L1 import check existed only in `internal/discovery`. A claim about code, written in the code, reading as verification | writing the guard the comment promised |
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
| `build/mcastrepro.sh`'s claim to exercise the no-carrier interface case (v1.110.1) | its comment said *"a dummy interface reports UP but never RUNNING — no carrier"*, so the namespace was claimed to cover the choice not to filter on `FlagRunning`. **A dummy reports `up|broadcast|running`** — measured. A probe making `FlagRunning` a filter therefore stayed green in the namespace, and the coverage the comment claimed lived at tier 1 instead | aiming the probe at tier 5 first and getting a pass. The comment now says what the namespace cannot reach and points at the table test that can. It also records that a dummy lacks the MULTICAST flag and joins anyway, which is why the selection does not require it |
| `TestTwoProcessesDiscoverEachOther`'s evidence, before the harness guard (v1.110.1) | the test read the browser's stdout, asserted `DISCOVERED` in it, and **threw it away on success** — so `mcastrepro.sh` could not tell a pass about discovery from a pass about anything else | the harness's own "the pass above is not about discovery" guard, which failed on a passing test. Fixed by echoing the evidence with `t.Logf`: evidence a check consumes privately is evidence nobody else can audit |
| The egress counter itself, in its first form (v1.110.3) | an nft output counter in a namespace with **no default route** reads **zero after a real connect attempt** — the kernel refuses at the routing stage and the packet never reaches the output hook. "No outbound traffic" would have been true of a process trying constantly | probing the instrument before trusting it: 0 before *and* 0 after a deliberate connect to 1.1.1.1. Fixed with a black-hole default route (0 → **2**), and the harness now runs that provoke every time and **fails if the counter does not move** |
| The armed-side-announcer probe, first attempt | patched `if ann, err := startAnnouncing(...); err == nil` to `… && false`, which still **calls** `startAnnouncing` — the announcer started anyway and only the deferred close was skipped. Reported GREEN, and I nearly recorded a vacuous test | the result being implausible: discovery cannot be optional when nothing types an address. Re-probed at the function itself, where it is red |
| A probe's restore, when the run times out | the probe loop's `cp`-back never ran after a `timeout` killed it mid-iteration, so **the next run silently inherited the patch** and I misattributed its output to the following probe | the error text naming a defect the current probe could not produce. The lesson is the loop's, not the test's: a probe harness must restore in a trap, not after the command |
| `TestNothingDecidesOnTheArrivalInterface`, first version (v1.110.4) | it used `strings.Contains` over the source for `IfIndex` — and matched the **comment in `mcast.go` that explains why nothing uses it**. A guard failing on its own documentation, and the third instance of this hole here after the `.deb` guard and `published.test.mjs` | running it: it failed against correct code, naming a comment. Rewritten over the **AST**, where a selector expression exists and a comment does not |
| The same guard's `SetControlMessage` half | it asked whether such an error check existed *anywhere* in the package. There are **two** call sites, v4 and v6, so discarding one left the other satisfying it — the probe came back GREEN | probing exactly that. Now it counts call sites against checked call sites, which is the population-floor shape the repo already uses elsewhere |
| **The `--lan` egress counter was IPv6-blind** (v1.110.5) | `ip daddr` in an nft `inet` table matches **IPv4 only**, and the stimulus probe was an IPv4 connect — so it proved the counter live for exactly the family the rule could see and **could not detect its own blind spot**. Nib announces on an IPv6 group and dials whichever family the browse resolved. Measured: 0 → 2 on IPv4, **unchanged** on IPv6 | a review agent reading the rule against the probe. Fixed with a second `ip6 daddr` rule, a summed counter, and a **second stimulus probe per family** — now 0 → 2 (v4) → 4 (v6). The lesson: probing that an instrument *fires* is not probing *what it can see* |
| `TestInterfaceSelectionSkipsWhatItShould` was decided by the host's kernel | it built synthetic `net.Interface` values with indices 1–5 and passed them to `chooseInterfaces`, which calls `Addrs()` — **a kernel query by index**. The fixtures reported the *host's* interfaces' addresses; `docker0` survived only because this machine's interface 3 has one. On a CI container it would be dropped and the test would fail claiming "FlagRunning became a filter", the wrong diagnosis. Its own comment asserted the opposite and was false | measured: `net.Interface{Index: 3}.Addrs()` returns 1 address here, `{Index: 99}` returns 0. Fixed at the root — the flag decision is now `flagsAllow`, pure, and the kernel query is separate. A decision that queries the kernel cannot be table-tested |
| `TestOffLinkSourcesAreRejected` tested the predicate, not the wiring | removing `onLink`'s call from the read loop left it **green**: a predicate with no caller passes its own unit test perfectly. This is the "guard with no callers" shape, self-inflicted | the probe. Fixed by driving a real off-link datagram in the namespace, which needed a third dummy interface on a subnet the socket does not join |
| `TestResolutionLivesOutsideTheDiscoveryPackage` | it asserted a neighbouring **file contained a string** — satisfiable by a comment, or by a leftover after the guard was deleted. **Fourth instance** of this hole here, and written one file away from the third fix | a review agent. Now parses `internal/discovery` and asserts the invariant itself: that package imports no vault, sign or p2p |
| Two tests silently deleted by an edit | rewriting one test truncated the file, taking `TestARealAnnouncementResolvesToACandidate` and `TestALinkLocalCandidateCarriesItsZone` with it. `go test ./...` stayed green — **it cannot tell you a test stopped existing**, the same finding P07 recorded | `mcastrepro.sh` greps for a *specific* PASS line and reported "the resolution test did not PASS inside the namespace". A harness that names its expected tests catches what the suite cannot |
| `Stats.Resolutions`, in `internal/rendezvous`'s first draft (v1.111.0) | a counter incremented by `Add(0)` — it could only ever be zero — with a comment arguing that *made it worth asserting*. Asserting a constant proves nothing about the code meant to keep it constant. It is the vacuous instrument stated as a virtue | writing the comment and not believing it. Replaced by an AST guard over the things that can actually appear: `LookupHost`, `ResolveUDPAddr`, `GlobalBootstrapAddrs`, `NewDefaultServerConfig` |
| The same file's claim about a nil `StartingNodes` | the comment said leaving it nil "is opting into exactly what D6 forbids" — hostname bootstrap. **False.** `NewServer` fills in `Conn`, `Logger`, `Store` and `SendLimiter` and leaves `StartingNodes` alone; nil means *no bootstrap at all* (`server.go:1268`). The hostname fallback lives in `NewDefaultServerConfig`, which this code never calls | reading `NewServer` after writing the claim rather than before. Setting the field is still right — "the cache, deliberately" and "nothing, by omission" should not look identical — but for a different reason than the one written down |
| `TestKRPCFromAQUICPeerReachesTheDHT`'s first assertion point | it read `RoutedByCID` immediately after `Dial` and got 0 — because a handshake is all LONG headers and `Dial` returns the moment it completes. That is the rule having nothing yet to decide, not the rule failing | the numbers not matching the story: `RoutedLongHeader:2, RoutedByCID:0`. Fixed by writing to a stream first, which is when short headers start |
| The same test with the generator wired on ONE end | it still failed, and the reason is the finding: B's mux learns A as a QUIC peer from **its own handshake writes**, so B swallowed A's ping by the same address rule at the other end of the same exchange. **The collision is symmetric and a one-sided fix is not a fix** | wiring only the side under test, which seemed like isolation and was a hole |
| **Door three: a `get` reply carrying `k` and no `seq`** (v1.112.0) | `exts/getput.Get` dereferences `*r.Seq` at getput.go:55 inside a `&&`, after the target matches, with nothing between the operands checking it. `Seq` is `*int64, omitempty`. Unrecoverable — the deref runs on a goroutine started bare at traversal/operation.go:236. **Neither existing door sees it**: the datagram decodes perfectly, and `gateQuery` fires on queries while this is a response | removing the `killsGetHandler` branch from `screened.ReadFrom`. `TestAResponseCarryingAKeyWithNoSeqCannotKillTheProcess` fails at its stimulus assertions first — it proves door one passes the datagram and door two cannot see it *before* it grades the drop. The library's **own server guards this exact field** on the inbound `put` (server.go:578-583): the check existed and was applied in one direction only, the identical asymmetry P04.S02 found |
| The same rule written as "refuse every response" | over-blocking passes the test above perfectly and silently kills every fetch the slice exists to perform | `TestAResponseCarryingSeqIsNotRefused`. A drop rule needs its complement asserted or it is indistinguishable from a shutdown |
| **Nib as an unbounded BEP-44 store for strangers** | `ServerConfig.Store` unset installs `bep44.NewMemory()` — a bare `map[Target]*Item`, no cap, no eviction; expiry is lazy and per-target, so an attacker who puts and never gets leaves entries resident for the process lifetime | removing the `put`/`announce_peer` refusal from `gateQuery`. `TestNibDoesNotStoreOtherPeoplesData` sends a **well-formed** query — carrying its arguments dict — so only the store policy can refuse it; without that the nil-args rule would satisfy the test and it would say nothing about the policy |
| The same policy written as "refuse every query" | Nib stops participating in the DHT entirely and every test above still passes | `TestAGetQueryIsStillAnswered` |
| **`ServerConfig.Exp` unset** | not "never expire" — *expire everything immediately*. `bep44.NewWrapper(store, 0)` makes `Wrapper.Get` compute `created.Add(0).After(now)`, false for any item stored even a nanosecond ago, so it deletes and replies not-found. The 2h default lives only in `NewDefaultServerConfig`, which caveat 7 forbids | deleting the `Exp` line. `TestWeServeOurOwnPublishedRecord` fails: "we published a record and then answered a get for it with NOTHING". `dht.Server.Put` writes locally before sending, so this node legitimately holds its own record and a peer's traversal may ask us for it — unset, Nib serves nobody, itself included |
| **The ceremony id bound in the preimage but never checked** | a validly-signed candidate record from ceremony one verified in ceremony two: A is in both rosters, the signature is genuine, and nothing asked which ceremony the bytes claimed. **Binding a field and checking it are two different jobs** | `TestACandidateFromAnotherCeremonyIsRefused`, which went red against this slice's own first version of `Verify` — the defect was found by the test rather than reintroduced for it |
| The candidate record's author check | removing `got != want` from `Verify` lets a roster member publish as another party | `TestAnInvitedPartyCannotPublishAsAnother`. Its stimulus is the point: B seals at A's salt with keys B legitimately holds, and the record **decrypts cleanly** — asserted before the refusal is graded, so the test cannot pass by the seal having failed |
| The expiry moved outside the preimage | any secret-holder strips the expiry and re-seals, extending a stale endpoint indefinitely | `TestAnExpiredCandidateIsRefused` asserts both halves: an expired record is refused, **and** moving `Expires` forward invalidates the signature rather than extending the record |
| Both ECDSA preimages without a domain tag | `sign.SignDigest` signs a bare digest and does not know what it means, so two message types under one identity key are two things one signature can satisfy | `TestTheTwoPreimagesCannotBeConfused`. No collision is demonstrated; the tag makes the question unrepresentable for one chunk, and `FormatVersion` bumped to 2 so a v1 record fails with a version sentence instead of "the convener signature does not verify" — which would accuse a counterparty of tampering |
| The AEAD's associated data | dropping it leaves hop, salt and seq rewritable for free — all three live outside the ciphertext, and the seq is chosen by the traversal | `TestTheSealIsBoundToItsHopSaltAndSeq` |
| `RecordKey` without the hop | per-hop *addressing* shipped at P01.S07; per-hop *encryption* did not, so one key decrypted every hop and **D30's own P05 clause — "a party cannot read the candidates of a hop it is not in" — was unsatisfiable by construction** | `TestTheSecretKeysEverythingAndKeysThemDifferently`, extended. Free to fix here: `RecordKey` had zero non-test callers |
| An unkeyed BEP-44 salt | the salt travels in cleartext in every put and is held by every storing node. A salt that was the party's fingerprint would make the public DHT a **searchable index of Nib identities** — a fingerprint is the permanent pin people hand out on a card | the same test, asserting the salt does not contain the fingerprint verbatim and that two parties differ |
| `res.V` treated as the published record | `getput.GetResult.V` is **raw bencode** (the server fills it with `bencode.MustMarshal`), so using it directly glues a length prefix to the front — which would decrypt to nothing and read as "the peer published garbage" | `TestAPublishedRecordIsRetrievableByThePeer` |
| A `bep44.Put` not re-signed after the traversal picks the seq | `seq` is inside the signed preimage and `getput.Put` hands the callback the highest seq **unincremented**; returning it unchanged with different bytes is refused by every remote node *and* by our own store, silently | the same test. Confirms the `SeqToPut` trap is real, not theoretical |
| An over-size record | `bep44.Check` refuses over 1000 **bencoded** bytes inside our own store, inside `dht.Server.Put`, **before any datagram is sent**; `getput.Put` then logs a warning per node and returns nil. The record simply never leaves the machine | `TestAnOversizeRecordIsRefusedBeforeItIsSilentlyDropped`, which also asserts the fake storer received **zero** puts |
| The L1 guard's blindness to `ed25519.PrivateKey` | the alternation matched `ed25519.PublicKey` via `publickey` but not `PrivateKey`, `Seed` or `[32]byte` — so the natural **write** signature for a BEP-44 publisher passed the guard written to keep key material out of this package, while the **read** signature tripped it as a false positive. Wrong in both directions on the one change it was built for | exporting `keyPair` and watching `TestNothingHereCanReachAnIdentity` stay green. Widened with `ed25519|privatekey` — deliberately *not* `key`/`seed`/`secret`, since an opaque `seed []byte` handed in by the caller is the compliant shape L1 requires |
| **`dhtlive.sh` running one named test** | it ran `-run TestLiveSelfAddressProbe`, a single literal name, and nothing compared that to the package. A live test added later is gated behind `NIB_LIVE_DHT`, therefore skipped by `go test ./...`, therefore **executed by nothing at all** — and `verify_test.go`, this ledger and every tier stay green | a new guard in `TestVerifyContractIsTrue` that counts `TestLive*` in the package against what the harness runs. The vacuous green one level out: a harness reports a pass for the tests it happens to name |
| A counter storing only on one path | `FetchNodes` was recorded only where the fetch found **nothing**, so a *successful* fetch reported "0 nodes answered" — a false zero, in the counter whose entire job is to say whether an absent record means anything | the live harness printing the number beside a record it had just retrieved. Found by reading the output, not by a test |
| **`seq` bound into the AEAD's associated data** (v1.112.1) | BEP-44's sequence number is chosen by the publish traversal AFTER the value is sealed, so the sealer can only guess it and the reader passes what the traversal used. **Every fetched record was unopenable in production**, reporting `ErrCandidateFormat` — "this is not a candidate record this version of Nib understands" — which blames the peer's build for a number the local publisher could not observe | probing it directly: sealed at seq 1, opened at 8, refused. The whole suite missed it because all seven tests passed the literal `1` to both sides, and `TestTheSealIsBoundToItsHopSaltAndSeq` asserted that a *different* seq fails — proving the binding was strict, which is exactly what broke the publish path. Replaced by `TestASealOpensRegardlessOfSeq`, which makes the mismatch the default case |
| The roster domain tag, deleted | `RosterHash` computes its preimage inline and returns a digest, so no test could see the bytes. Removing `writeLP(rosterDomain)` left **the entire repo green** | a mutation audit. `TestTheTwoPreimagesCannotBeConfused` looked like it covered this and did not: `_ = rh` was dead, it parsed one preimage rather than two, and `candidateDomain == rosterDomain` compared two compile-time constants. Fixed by factoring `rosterPreimage` out — a function that returns a digest gives a test nothing to look at — and by one shared length-prefix builder, since three copies of it are what let the languages drift |
| `FetchNodes` stored on one path only, again | the defect the v1.112.0 commit message boasts of fixing shipped with **no instrument**. Reintroducing it left all twenty packages green; the counter appeared in no assertion anywhere, only in `Logf` text on a harness that runs when the public DHT cooperates | the same audit. Now asserted on the success path in the round trip, alongside `PublishAttempts`/`FetchAttempts`/`FetchUndecodable`, which were read by nothing at all |
| An unkeyed BEP-44 salt, in two shapes | the salt travels in cleartext in every put. Two mutants — `sha256("unkeyed-hop-N" ‖ fp)`, and one publishing sixteen **raw fingerprint bytes** — both kept the suite green, because every assertion was a *difference* assertion and any distinct-input hash satisfies those | the discriminator that was missing is one line: the same fingerprint under a **different invitation** must give a different salt. Nothing unkeyed survives it |
| Both derivations re-keyed off the PUBLIC ceremony id | `HopSeed` and `RecordKey` rebuilt from `Invitation.ID` instead of `Secret` — green. Every assertion compared two fully independent invitations whose IDs already differ, so "differently" was proved and "from the secret" never was | one invitation, one field changed. Caveat 11's 32 uniform bytes are the whole security argument; a test that cannot tell whether they are used is not testing it |
| A fingerprint in uppercase | `hex.DecodeString` accepts either case, so an uppercase roster entry was legitimate — and broke the ceremony twice: the two parties derived **different DHT salts** and never met, and had they met, `ErrCandidateAuthor` would have accused an honest counterparty of impersonation over letter case | normalising at `ParseInvitation` and deriving the salt from decoded bytes. Red-proved both halves |
| `seq + 1` on a squatted key | a roster member publishing at `math.MaxInt64` makes our increment wrap to `MinInt64` — Go wraps signed overflow silently. Every remote refuses, `getput.Put` shadows it, `Publish` returns **nil** and `published` increments having stored nothing. Turns a preemption race the honest party re-wins into a permanent block | driving it with a fake node serving a correctly-signed `MaxInt64` item |
| A cancelled lookup reported as `ErrNoRecord` | `getput.Get` sets `err = ctx.Err()` and leaves `ret` alone, so a caller's cancellation and our own expiring budget both arrived as "no record" — and incremented `FetchEmpty`, whose own doc says it must never be summed with a transport failure | `TestAnAbortedLookupIsNotAnEmptyFetch`. The ladder would have told a user their peer had not published when the truth was that we stopped asking |
| `Seal` on a record mutated after signing | `Seal` checked only that `Sig` was non-empty, so appending one candidate between `Sign` and `Seal` produced a well-formed record the peer refused as tampering **in transit** — pointing at the wrong machine entirely | `TestSealRefusesARecordModifiedAfterSigning` |
| An expiry a century out | `Verify` accepted any future instant, and `Expires` is the *entire* freshness margin, so a storing node could re-serve an endpoint for as long as the publisher felt like writing | a ceiling at the reader, because the publisher's good manners are not a property the reader can check |
| A non-canonical plaintext | the signature is verified over a preimage **re-built from the parsed struct**, so sub-second times, offset timezones and `[0:0:...:1]` all survived the round trip as different bytes than arrived. No exploit today; a signature bypass the day a weakly-canonical axis lands | requiring the re-emitted preimage to equal the consumed bytes. The doc claimed "the thing signed and the thing sent are the same bytes"; now it is true rather than nearly true |
| `MaxSealedRecord` raised 996 → 1000 | the plausible "fix the off-by-4" edit. Every ceremony test stays green and **every full record becomes silently unpublishable** — refused inside our own store before any datagram is sent, with `getput.Put` returning nil | the arithmetic asserted directly, since L1 forbids importing the package that holds the other half of it |
| The dhtlive guard, three variants | (A) narrowing `-run` back to one named test — caught. (B) narrowing it **with the correct pattern in a comment** — passed, the repo's own recurring hole, previously seen in `published.test.mjs` and a `.deb` guard. (C) the unquoted, functionally identical form — false red | matching the `go test` **invocation line** and compiling its `-run` pattern against every discovered `TestLive*` name. All three variants now behave |
| The harness's own UNREACHABLE gate, after it widened | scoped to the whole file, so a **passing** probe plus a skipped publish/fetch printed "the self-address probe is UNVERIFIED by this run" — false — suppressed the probe's evidence lines, and exited before the per-test block could report anything | running it. A gate that widens with its subject has to be re-scoped with it |
| An exported `[32]byte` in `internal/rendezvous` | the L1 pattern matches `ed25519.PublicKey` via `publickey` but cannot see a bare `[32]byte` — the literal type of a BEP-44 public key, and of what `keyPair` returns — while the widening comment implied it could | a probe exporting `HopKey() [32]byte`. Closed with a type check rather than a name check, because the whole hazard is that the type says nothing about itself. Also added a **positive control** for the pattern: nothing proved it still matched anything |
| `Stats().Responses` incremented but absent from the `Stats()` builder | the counter counted and the struct never carried it, so every reader saw zero — the "counter with no reader" shape, self-inflicted, in the fix for a vacuity finding | its own test timing out. Worth recording because the instrument caught the instrument |
| **The read-path candidate-address check** (v1.113.0) | eight bogons — `127.0.0.1:22`, `[::1]:53`, `192.168.1.1:53`, `10.0.0.1:11211`, `224.0.0.1:5353`, `255.255.255.255:9`, `100.64.0.1:123`, `1.2.3.4:0` — sealed, opened and **verified end to end**. Under the punch that is an inside-the-LAN port sweep run from the victim's own host, plus UDP reflection at 53/123/11211 | the check was added, and the FIRST test for it drove `Sign` — the write side, the one door an attacker never comes through. Deleting the read-path check left the whole repo green. Now hand-built sealed records, one per bogon, asserting the sentinel and the counter |
| `Routable` without `::/96` | `Unmap()` handles `::ffff:0:0/96` only, so IPv4-**compatible** form was undefended one prefix over: `::c0a8:101` IS 192.168.1.1, and `::7f00:1`, `::a00:1`, `::e000:1`, `::6440:1` all reported Routable — measured. The function's own doc claimed the family-crossing class was closed, and it was half closed, in the commit that closed the other half | the package's own table, added because it had none: its only coverage was a delegate test in another package that exercised `Routable` and never `Target`, `MinPort` or `SharedSpace` |
| `MinPort` with no exceptions | 443 and 80 refused. The reflection argument is entirely about UDP, but D8 races **TCP** concurrently and D14 exists for networks where outbound TCP is permitted and UDP is not — where a peer's only reachable inbound port is plausibly 443 | measured: `Target(93.184.216.34:443) == false`. Now two named exceptions, with the reasoning |
| The accumulated cap, removed | one rendezvous key yielded 18 candidates across three valid records — the per-record check cannot see a key that yields a **sequence** over a 300 s race | `TestOneKeyYieldsNoMoreThanNAcrossTheWholeRace`, which asserts its stimulus (18 offered) before grading the cap |
| Duplicate detection, removed | four copies of one address in one record admitted four times, aiming the whole per-candidate budget at one victim | `TestARepeatWithinARecordIsNotTheSameFactAsARefetch` |
| The duplicate counters, **conflated** | this is the inverse defect and it shipped in the first draft: an honest peer produced `DroppedDuplicate = 72` in a counter documented to mean *attack*, because BEP-44 serves the same value to all ~10 fetches of a race. An alarm that is always on is not an alarm | the review, then the split into `DroppedDuplicate` (within one record) and `Reoffered` (a later record) |
| `MaxRoster` on the invitation door only | a self-signed record naming 10,001 parties verified cleanly. The cap bound recipients and left the **convener** — the party that dials every hop and emits every packet — unbounded, and the convener never parses its own invitation | `TestAHostileDocumentCannotCarryAnUnboundedRoster`, driving `Record.Verify` |
| Three refusal-counter arms | `RefusedUnroutable`, `RefusedSignature` and `RefusedContext` could each be deleted with the suite green. `RefusedContext` is the cross-ceremony replay — filed as `RefusedFormat` it reads as "the peer's build is broken" instead of "a roster member is replaying across ceremonies" | one driven test each |
| The disclosure banner, reverted | `README.md` said `nib rendezvous` "publishes nothing" on the day `--self-test` started publishing — the **second** staleness of a disclosure written in this same arc, in the same document | `banner()` extracted and asserted both ways, plus an ordering test that it precedes the socket. Prose about network behaviour rots exactly like a doc comment, and nothing guarded it |
| D33's "and both sides" | the amendment made the punch budget per hop and left "across all candidates **and both sides**" untouched. Under it one hop demands 6,240 against 3,000 — a **52% shortfall**, not the ~4% the amendment itself claimed — and a budget shared between two machines has no mechanism to be shared | the review reading the amendment against its own arithmetic |
| The self-test's candidate constant | it published a signed record naming **93.184.216.34 — example.com**, a real third party, because the RFC 5737 documentation ranges that exist for this purpose are (correctly) refused by `addrscope`. Harmless only while nothing dials it, which is a property of an unfinished ladder | the review. Now the machine's own probed address, and the self-test **skips** rather than naming a stranger when the probe found none — observed doing exactly that on a run where the seed list was exhausted |
| **The PIN compare, deleted** (v1.114.0) | `subtle.ConstantTimeCompare(fp, pinnedSPKI)` in `verifyPinnedPeer` (cited by SYMBOL: the line moved twice during this very change) short-circuited to true. This is L1's whole load-bearing line — and until this slice **nothing dialled an address whose listener was not the pinned peer**, which is precisely what a lying rendezvous produces. The two existing near-misses are different attacks: one has the dialer *tricked into pinning the impostor* (the MITM case the spoken string exists for), the other has the wrong *dialer* reaching a correct listener | `TestAnAddressFromALyingRendezvousCannotSubstituteASigner`, over both transports, and `TestDialAnyWalksPastAnImpostorAndLandsOnThePinnedPeer` at the consumer. Each asserts its stimulus first — the same rig with the correct pin connects — because "refused" is otherwise satisfied by a closed port |
| D19 cause 5, reverted to the generic sentence | `"peer transport certificate is expired or not yet valid"` states two opposite facts in one sentence and names neither, and reached the user inside cause 4's "could not connect". `now`, `NotBefore` and `NotAfter` were all in hand at the refusal and discarded | `TestTheClockSkewRefusalNamesADirectionAndAMagnitude`, which also drives the direction **inverted** — a message that tells the user to fix the wrong clock is worse than the vague one it replaced |
| **A wire-derived pin at the consumer** | `resolve` rewritten to build `candidate.Fingerprint` from the announcement's name instead of the vault's pin. **Five AST guards exist in this tree and all five are producer-side**; none walked `internal/server`, and an import-shaped guard cannot work there because that package's job is to hold both halves. One wrong keyed field in one composite literal breaks L1 with nothing to notice | `TestNothingWireDerivedReachesAPin`, which finds the wire parameter by TYPE per file (so a rename cannot disarm it) and fails if it walks zero `candidate` literals |
| **The consumer-side guard, evaded four ways** (v1.114.0) | its first version inspected one keyed field in one composite literal, and a review evaded it with ordinary refactors: a local hop (`wireFP := []byte(s.Name)`), an unkeyed literal, a type alias on the parameter (`type wireSeen = discovery.Seen`), and build-then-assign. Two floors were also hollow — a type rename left the taint set empty and every check passed silently, and the `literals == 0` floor was satisfied by `resolve`'s two zero-value `candidate{}` early returns, staying green exactly when the literal it polices was gone | rewriting it as a taint to fixpoint with three floors, then re-running **all five mutants: red** |
| Cause 5 with no consumer | the typed error survived the TLS boundary and nothing recovered it, so the user read *"none answered as the pinned peer: this machine's clock is at least 1.9 hour(s) behind…"* — cause 4's headline plus a claim that is false, since the peer WAS the pinned peer | `TestAClockSkewIsNotReportedAsAPinFailure`, red without the `errors.As` lift |
| `isPinRefusal` accepting four strings | written believing QUIC surfaces the peer's TLS alert. Measured, QUIC surfaces OUR sentence tagged `(local)`, and the peer's refusal of US is `(remote): tls: bad certificate` — so the alternation matched **both directions** and could not distinguish the claim from its mirror | narrowing to the one exact string, which is present on both transports |

| **The seed filter computed and not applied** (v1.115.0) | `validateSeeds`'s result assigned to `_` instead of to `Invitation.Seeds`. **Both doors** — `Encode` and `ParseInvitation` — stayed green: the test asserted only the *count* of what was dropped, never that the surviving list was the one that travelled. A loopback or 127.0.0.1:53 seed would have ridden through an invitation that had "validated" it | `TestEachDoorAppliesTheFilteredList`, which reads back what each door actually produced |
| The dropped-seed count, removed | a partly-filtered list left no trace at all, so a recipient could not tell a three-seed invitation from a hostile eleven-seed one that had been quietly cut down | `Invitation.SeedsDropped`, and `Encode` now **refuses** rather than drops — a producer shipping fewer seeds than the convener chose is a different thing from a recipient refusing some |
| `SeedSample` shipping our own probed endpoint | the doc argued a routing table "holds other nodes, never self" — true of the table, and irrelevant, because the sampler had no filter for the address the self-probe had just established. Every invitation would have disclosed the convener's own public endpoint to every recipient and every mail server in between, which is the exact thing D6's second half is written to avoid | the filter, plus `sampleSeeds` split out as a **pure function** — its earlier test drove a live `Server`, whose hermetic table holds only loopback nodes, so the loop body never ran and `SeedSample` stubbed to `nil` kept it green |
| `addrscope.Seed` replaced by `addrscope.Target` | 80 and 443 admitted as DHT bootstrap addresses. Those two ports are the likeliest to belong to an unrelated third party's web server, and a seed is spoken to over **UDP only** — so an attacker-supplied list of them turns every recipient's cold start into unsolicited UDP at somebody's HTTPS endpoint | `TestSeedSampleFiltersAndDoesNotWatermark` against synthetic nodes |
| TRIED and USED, **conflated** (v1.115.0) | one flag for both facts, set before the retry so the closure could read it. The retry runs on the caller's context, which the first attempt may already have spent — measured: `err=context deadline exceeded, InvitationSeedsUsed=true, Nodes=0`. The operator's note then read *"this routing table came from a list the invitation's sender chose"* about a table that came from nothing at all | the split, plus `TestSeedsTriedButUselessIsNotReportedAsUsed`. The CLI note gained its **third branch** at the same time: tried-and-failed had been printing "unused (the shipped list worked)" over a machine where nothing worked |
| `saveNodes` persisting a stranger's table | an eclipse outliving the ceremony. The seeds answer, their neighbours become the cache, and every future run bootstraps from an attacker-chosen list with `Seeds 0`, `InvitationSeeds 0`, `InvitationSeedsUsed false` — **no trace on the machine of where its view of the DHT came from** | the refusal, driven live: `TestLiveInvitationSeedsRescueAMachineTheShippedListCannot` stats the cache file after a real 22-node bootstrap off invitation seeds |
| *(not a red proof — a refutation)* **F8, the concurrent-`Bootstrap` hazard** | the S06 review argued two callers could interleave across the read-modify-write of `invSeedsTried` and burn the one-shot retry without taking it. A serialising lock was added, then **removed**: deleting it left a four-goroutine test passing, and deleting `s.mu` from that same section left it passing under `-race` too — each `Bootstrap` spends seconds in its traversal, so four callers never overlap at the critical section. The test had no reach over either lock | recorded in `seeds_more_test.go` in place of the test, because a lock whose guard cannot fail is a moving part pretending to be a defence |
| **Both dial-control hooks, reverted to the stdlib vocabulary** (v1.116.0) | `IsLoopback \|\| IsPrivate \|\| IsLinkLocalUnicast \|\| IsLinkLocalMulticast \|\| IsUnspecified` — which is what they actually shipped, in two byte-identical copies, on the clients that fetch URLs out of **untrusted file content**. Ten measured classes passed: the four IPv4-compatible IPv6 forms (`::7f00:1` IS 127.0.0.1), `100.64/10`, `255.255.255.255`, `0/8`, `240/4`, benchmarking and ORCHIDv2. The reachable one is `100.64/10`, which on any carrier-NAT'd line addresses the carrier's equipment and the subscriber's own router | `TestTheDialHookRefusesWhatTheStdlibVocabularyMissed`, which asserts for **every** entry that the OLD predicate admitted it — otherwise the list silently overstates the finding — plus a public-host control, because a hook that refuses everything is an outage, not a fix |
| One hook keeping its own copy | the predicate perfect and a third implementation sitting in a dialer, which is the exact state the phase review found (the plan had named one of the copies and missed the other) | `TestBothDialHooksCallTheSharedPredicate`, an AST pass that finds dial-control hooks **by shape** — a trailing `syscall.RawConn` parameter, so a rename cannot disarm it — with a floor that fails if it finds fewer than two |
| **All four stamp sites, unescaped** (v1.116.1) | pdfcpu's `format.Text` reads `%` as a placeholder introducer. Measured against v0.13.0: `CONFIDENTIAL 100%` → `CONFIDENTIAL 100`, `50%P` → `503`, **`%v` → `v0.13.0 dev`**, `%` → empty and then a nil bounding box panic. `finalize.go:70` bakes the watermark and `:88` **signs the result**, so a user typing `%v` certifies a dependency's version string where they typed two characters | `TestStampTextKeepsWhatTheUserTyped`, which round-trips THROUGH pdfcpu rather than asserting the escaping (which would test our own `ReplaceAll`), plus `TestPdfcpuStillEatsAPercent` as a standing stimulus so the premise is re-measured every run rather than trusted from a comment |
| `%%` believed to be an escape | **the shipped `StampTextLayer` did exactly this**, and its comment claimed it "also stops e.g. an OCR'd `%P` turning into a page count". Measured: it does not. The doubling emits one `%` and advances a **single** character, so the trailing `%` pairs with the letter anyway — `%%P` → `%3`, `%%%%P` → `%%%3`. A literal `%P` is unrepresentable through this API, and runs of two or more `%` do not round-trip either | the round-trip check itself: `stampText` renders its own output back through `format.Text` and refuses when it does not match the input. Deliberately not a table of placeholder letters — that is a second copy of pdfcpu's grammar, and `coverage.go` already records why this package asks the encoder instead |
| One stamp site bypassing the helper | there were **four** call sites and **three** policies — escape, strip (silently deleting a `%` the user typed), and nothing. The review found three of the four; the fourth was found by grepping the call sites while fixing them | `TestEveryStampSiteGoesThroughStampText`, an AST pass matching `TextWatermark` calls by shape and requiring the text argument to be a local bound from `stampText`, with a floor that fails below four sites |
| **The deadline spanning two human gates** (v1.116.2) | `exchangeDeadline` is 6 minutes, absolute, and since P01.S05 had to cover the spoken check (5 min, both roles) AND the receiver's consent (5 min). `Initiate` and `SendDocument` never re-armed. Two ordinary users each taking ~3.5 minutes — comfortably inside the windows the product advertises — blow it: the initiator's read times out AFTER the responder has co-signed and saved, so **both users have signed and only one holds the artifact**. The exact outcome `postConsentDeadline` was written to prevent, on the two entry points that fix does not cover | `TestEveryEntryPointReArmsAfterItsHumanGate`, an AST pass requiring a `SetDeadline` after each `runVerification` call, with a floor at four entry points. Until this file **`grep` returned no test anywhere referencing any of the four timeout constants** |
| `KeepAlivePeriod` left at zero | quic-go: *"If set to 0, then no keep alive is sent"* — so a QUIC session emitted **nothing** while a user read the four words, and `quicIdle` (5 min) ended the connection with a transport error in place of the session's own message. `quicIdle`'s doc said it was *"deliberately longer than the session core's own exchangeDeadline"*; it was 5 against 6 — **shorter**, and the sentence had been false since the constant was written | `TestNoBudgetSpansTwoHumanWaits`, asserting the value `quicConfig()` actually returns rather than the constant — the constant can be perfectly sized and simply not wired, which is the state the field was in for the whole life of the transport. Its first draft scanned the source for the false sentence and went red against the comment *explaining* the sentence: a guard reading its own documentation as code, a shape this repo has paid for before |
| **`handlePDF` reading `doc.data` directly** (v1.116.3) | the last of fourteen readers not going through `docBytes`, whose own doc forbids it by name: *"Every handler that reads doc.data must come through here. Reading the field directly is a data race against undo.go's `doc.data = result`."* One user with two panes: pdf.js re-renders while the other pane runs a page op | **the race detector, once `/api/pdf` was added to `TestDocumentReadRoutesRaceMutation`'s population.** That test exists because *"a guard whose population is one is not a guard for the class it appears to cover"* — and its five-route list omitted the one route still reading the field, reproducing the class-of-one shape for the last offender |
| A commit failure answering 404 | ADR-004 rule 3 is *"an id naming a document the server no longer holds is `409`, never `404`"*, and **eight** handlers answered 404 — while `undo.go`'s own contract comment still instructed *"answer 404"*, which is how eight copies stayed wrong. `web/app.js:346` reconciles on 409 and does nothing on 404, so closing one tab during a long operation on it left a tab where every action failed | `TestACommitFailureIsAlwaysA409`, an AST pass over every `if !s.commitMutation(...)` / `commitBarrier` branch with a floor at eight — a source guard rather than eight route tests, because the ninth handler written is the one it has to catch |
| `handleUndo` pushing onto redo without trimming | ADR-003 bounds the undo+redo **pair** against one global budget; `handleRedo` trimmed on its push and `handleUndo` did not. Not byte-neutral — undoing a large OCR result moves a big `doc.data` onto redo while popping a small `prev` — so the total walked past the ceiling with nothing evicting, on a budget whose purpose is that the ceiling does not scale with what the user opens | `TestUndoingPastTheBudgetEvicts`, driven through the handler rather than through `trimHistoryLocked`, because the defect was never in the trimmer — it was in one of its two callers not calling it. Asserts its stimulus (the undo really pushed bytes onto redo) before grading the total |
| **`originIsLoopback` treating an absent Origin as same-origin** (v1.116.4) | browsers send no `Origin` on sub-resource GETs, so `<img src="http://127.0.0.1:PORT/api/status">` on any open page passed `requirePublicLoopback` and reached `ensureUnlocked` → `vault.AutoSetup`. That guard was added to this exact route because *"the method-based guard let any web page the user had open trigger first-run vault creation with a plain cross-site request"* — it stopped `fetch()` (which does send Origin) and nothing else | `TestASubResourceGetCannotReachTheVault`, whose **population is the finding**: a test driving only `Origin: https://evil` passes against the shipped code, because that case was already refused. Closed with `Sec-Fetch-Site` — a forbidden header name, so page script cannot forge it — with controls for Nib's own UI, a typed URL and a client sending no metadata, since requiring `Origin` would refuse the app itself |
| `safeFetch` truncating rather than refusing | `io.LimitReader(body, maxBytes)` returned exactly `maxBytes` of a larger document with no error, so a 250 MiB URL opened as a 200 MiB corrupt PDF with `canSave` set — `LooksLikePDF` only reads the header. **The old test codified it**, asserting `len(body) == 10` for a 100-byte response | `TestSafeFetchRefusesAnOverSizeBodyRatherThanTruncating`, which also drives the boundary from both sides — a body exactly at the cap must still be returned, or the fix is an outage |
| `requireHTTPScheme` deleted | every case still errored, from `http.Transport.RoundTrip`, which refuses non-http(s) schemes itself. The test asserted `err != nil` and was green with the guard gone — **twice**, in the scheme test and the redirect test | asserting the rule's own token (`unsupported URL scheme`), which is the only thing separating our refusal from the transport's — and the difference is real: ours happens before a DNS lookup |
| `assetURL` handing on a non-http URL | its result reaches `location.assign(d.downloadUrl)` and `window.open(d.url)`, so a `javascript:` URL executes in Nib's origin, holding the CSRF token and an unlocked vault. Latent — the listing is TLS-verified to GitHub. **The function had no test at all**, and the fixture that existed used relative URLs (`"u/lin-amd64"`) that the API never returns, which is part of why the scheme was never checked | `TestAssetURLNeverHandsOnANonHTTPURL`, plus making the existing fixture absolute https so it looks like what GitHub actually sends |
| **The LAN candidate list, uncapped** (v1.116.5) | `resolve` builds a candidate from the observed source host plus the port **from the datagram payload**, so one on-link host needs **no address spoofing**: it announces the six-word name — broadcast in the clear every 500 ms, and `browsePeers`' own comment says it is not a secret — with a different port each datagram. `dialAny` then walked every one serially at 30 s inside an HTTP handler on a server with no `WriteTimeout`. Measured in the guard: 200 announcements from one host, 200 candidates | `TestOneHostCannotFloodTheCandidateList`, which asserts its stimulus (the flood really did resolve) before grading the cap, and carries the reason the cap is not 1 — a cap of one reintroduces the capture attack that returning every address exists to defeat |
| The dial walk with no total budget | a cap bounds how many candidates there are and says nothing about how long each takes. Driven against TEST-NET-3 (`203.0.113.0/24` — routable-shaped, allocated to nobody, so a dial hangs rather than being refused, which is the attacker's case; a refused connection costs nothing) | `TestDialAnyStopsEvenWithCandidatesLeft`, with **real credentials** — `TestDialAnyTriesEveryCandidate` passes nil ones, so `SessionTLS` refuses before any dial and its "2 address(es)" assertion reads `len(addrs)` rather than attempts made. Asserts the walk took at least a second first, so a credential error returning instantly cannot satisfy the bound while contacting nothing |
| **The candidate AAD binding the BUILD's version constant** (v1.116.6) | not the record's wire version, which cannot be known before decryption. The first bump would make every v1 ciphertext in flight fail `aead.Open` and surface as `ErrCandidateSealed` — *"this record was not written for this ceremony, or has been altered"* — an accusation of tampering where the truth is that two builds disagree about a format. `record.go` spends six lines refusing exactly that for the sibling format, in the same package, under D32 | `TestAVersionSkewedRecordSaysSoInsteadOfAccusingThePeer`. **Its first draft was vacuous in the same way the original defect was**: it sealed using `candidateAAD` itself, so a mutation moved both sides together and stayed green — which is precisely how the bug hid (every test passed the same literal to both ends). Rewritten to hand-build the AAD, stating the wire contract independently |
| `BindingMAC` by bare concatenation | `("a","bc")` and `("ab","c")` write identical bytes and produce one MAC, and there was no domain tag in the input — while `preimageBuilder`'s doc calls itself *"the one length-prefix encoder every signed preimage in this package uses"*, which this exception made false. Two mitigations existed and neither was written down or asserted: the key is purpose-derived, and the only roles anyone passes happen to be the same length. `role` is a free string parameter on an exported method | `TestTheBindingMACCannotBeSlidAcrossItsFieldBoundary`, with the reflection property (initiator ≠ responder) as the control so the fix cannot be satisfied by breaking determinism |
| `MatchesRecord` never comparing the convener | `RosterHash` does not bind who convened and `Record.Verify` asks only that the signer appear SOMEWHERE in the roster — so any roster member re-signs the identical roster with their own key and the record verifies, `RosterToken` is byte-identical, and `Convener()` now names them. Meanwhile `ConvenerFingerprint`'s own doc said it exists *"so a party can check the record's signer against what the invitation told them to expect"*: that check did not exist, and the field was written at two sites and read nowhere | `TestAnInvitationCatchesARecordConvenedBySomeoneElse`, which asserts three stimuli before grading — the honest pair matches, the forgery **verifies**, and the roster token is unchanged (so nothing about the record alone could have caught it) |
| `MaxSealedRecord` as a producer-only bound | `Seal` refused over 996 bytes; `Accept` handed a slice of any length to `aead.Open`, which allocates `len(ct)`. The only thing bounding it was anacrolix/dht's bencode check — in a package **L1 forbids this one from importing**, so the ceiling was a hope about a transitive dependency rather than a property of the package. Both other doors here argue the opposite in terms: *"called at BOTH doors, and that is the whole point"* and *"Refused HERE, at the parse, not at the dialer"* | `TestAnOverSizeSealedRecordIsRefusedAtTheRead`, with a record exactly at the cap as the control. It also **corrected an existing test's stimulus**: `TestAnOverCapRecordIsRefusedWhole` hand-built `MaxCandidates+50` candidates, sealing to 1,859 bytes — past BEP-44's own 1000-byte cap, so a record no peer could deliver. It was measuring the candidate cap through a stimulus that cap would never see; now `+1`, the shape an attacker actually has |
| **A vault from a NEWER Nib, opened and downgraded** (v1.116.7) | every version gate was `env.Version < 2` — a floor with no ceiling. A Version 3 vault opened, was decrypted as v2, and `save()` unconditionally rewrote `envelope{Version: 2, …}`, dropping every field this build does not know because encoding/json discards unknown keys. Reached by an ordinary user: a downgrade, a second machine, a synced folder. The vault holds the **only** copy of the signing identity | `TestAVaultFromANewerNibIsRefusedRatherThanDowngraded`, with every older format as the control — a ceiling that refuses the user's own vault is worse than the defect |
| **A font name used as a path component** | `mdpdf` documents itself as a root package other projects import, so `Font.Name` is API surface; it is joined into a path and the result is **gob-decoded**. Worse than it reads: `font.UserFontDir` is EMPTY until pdfcpu's config initialises, so the join is relative to the process working directory | `TestAFontNameCannotEscapeTheFontDirectory`, and getting it to have reach took **three attempts**, each recorded because each is a distinct vacuity: (1) asserting `installedFallback` — its `os.Stat` fails with ENOENT on a traversal path anyway, so removing the guard left it green; (2) asserting `registerMetrics` without a file — same ENOENT; (3) planting a decodable file but guarding the block with `if err := os.MkdirAll(font.UserFontDir); err == nil`, which **skipped silently** because the dir is `""` — the never-exercised-subject defect, in the test written to catch it. Only the fourth version, setting `font.UserFontDir` explicitly and planting a real gob file, goes red |
| A charset-only font-name check | `"."` and `".."` are made entirely of permitted characters and are exactly the two names that traverse. Caught by the guard's own case list rather than by reasoning about it | the same test's case table |
| **`maxBlockIndent` above the content width** | the clamp cannot bound the wrap budget it exists for; past it `wrapWords` gets a negative budget and `splitWord` emits one rune per line | `TestNoBlockIndentEverExceedsTheClamp`. *(Recorded honestly: the `*ast.List` branch was missing the clamp its `*ast.Blockquote` sibling has, and both really do nest 40 deep giving an indent of 720 against a width of 468 — measured. But **no Markdown input reproduces the page-count harm for lists**: text indented far enough to sit inside the deepest item reparses as an indented code block first. The clamp is retained for symmetry and defence in depth; the test asserts the arithmetic that is true rather than manufacturing a red for a shape that does not occur.)* |
| **`nib verify` exiting 0 for content added after the last signature** (v1.116.8) | `sign.Verify` reports `State=Valid` with `AddedAfter=true`; the exit code was driven by `State` alone. The command's own help says *"Exit 2 if any file is unsigned or modified"* and **README ships `nib verify contract.pdf && echo "signature intact"`** — which printed for a document that is not wholly signed. The actor is the counterparty who returns your signed contract having appended pages, an ordinary tool-supported PDF operation | `TestVerifyExitsNonZeroForContentAddedAfterSigning`. **No test in `internal/cli` had ever built an added-after document** — `TestVerifyUnsigned` and `TestSignThenVerify` were the whole verify population, so the subject was never exercised. Asserts the fixture really is valid-plus-added-after before grading, and runs the intact control first |
| `watch` following a symlink out of the watched directory | `DirEntry.Info()` is an **Lstat**, so a symlink named `x.pdf` passed the extension filter; `watchTransform` read through it and `writeAtomic` — which calls `EvalSymlinks` — renamed over the **target**. Anyone who can drop a file into the watched directory rewrote any PDF elsewhere on disk the user can write | `TestWatchNeverFollowsASymlinkOutOfTheWatchedDirectory`, and its **first version was vacuous**: it called `scanOnce` once, and `scanOnce` only acts on the second sighting of an unchanged file ("first sight or still changing — let it settle"), so the action never ran and the traversal never had a chance to happen. Now called twice, with a regular file in the same directory as the stimulus proving the scanner acts at all |
| A browser reported as launched when it started and died | `cmd.Start()` reports only exec-level failure and `reap` explicitly discarded `Wait`'s status. A locked user-data-dir, snap confinement or an Edge policy produced a successful launch with no fallback and no diagnostic a double-clicked process can show anyone — *"I double-clicked Nib and nothing happened"* | `TestABrowserThatStartsAndDiesIsNotReportedAsSuccess`, with a still-running process as the stimulus so an `alive` that always answers false cannot pass |
| `--out-dir` and `--data` missing from the transform-flag list | `split`, `fill` and `pagenum --continuous` write through them, so `nib splt big.pdf --out-dir out` fell past the mistyped-verb guard into the desktop boot and opened a file named "splt" — the outcome that guard's own comment says it exists to prevent, for the three subcommands producing the most files | `TestAMistypedBatchVerbIsNotOpenedAsAFile`, with ordinary launches as controls so the guard cannot answer "transform" for a double-click |
| The probe verdict exiting 0 having observed nothing | *"the DHT is reachable but nothing reported our address back"* is the diagnostic failing to establish the fact it was run to establish, reported as a pass. The self-test arm one screen above already treats the same shape as non-zero, in terms | `TestTheProbeVerdictCannotReportSuccessWithoutObservingAnything`, with a healthy probe as the control |
| **The retry's nodes credited to the shipped seed list** (v1.116.9) | both bootstrap attempts added to one `bootstrapped` counter, so on the machine invitation seeds exist for — shipped list dead, seeds rescue it — `Stats()` reported `Seeds: 5, Bootstrapped: 25` and the rot alarm documented at `dht.go:96` (*"Zero while Seeds is non-zero is the rot alarm"*) read **"the shipped list worked"** on a run where every shipped address had failed. The plan defends the `Seeds` term of that comparison by name; the confounding landed on the other term | `TestTheShippedListsRotAlarmSurvivesAnInvitationRescue`, which skips rather than passes when the retry did not run or produced no table — the two states in which it would have nothing to attribute |
| **`fetchUndecodable` incremented by nothing** | `TestAValueThatIsNotBencodeIsCountedAsUndecodable` left `r.K`/`r.Sig` zero, so getput's `DoQuery` pushed nothing to `vChan`, `ret.V` stayed empty, and `Fetch` took the **fetchEmpty** path. Its assertion was `FetchUndecodable == 0 && FetchEmpty == 0` — an OR satisfied by the wrong outcome — so the counter the test is named for was unreachable and `fetchUndecodable.Add(1)` could be deleted with the suite green | rebuilt with a real ed25519 key derived from the same seed `Fetch` uses, signing the value so it survives every check between the wire and our decoder. **Raw garbage does not work and is why the first rebuild still measured nothing**: `krpc.Return.V` is raw bencode, so `bencode.Marshal` of the reply fails and the fake never answers — recording EMPTY, exactly what the old OR accepted. The value is now a bencoded *integer*: valid on the wire, and not a byte string |
| `TestSeedsTriedButUselessIsNotReportedAsUsed` asserting one half of its own name | it split into *tried* AND *not used* and checked only the second; its stimulus proved `Seed()` was called, not that the retry ran. **Deleting Bootstrap's whole retry block left it green** — seeds 1, used false, nodes 0, all as asserted | adding the `InvitationSeedsTried` assertion, and turning its "the dead seeds unexpectedly produced a table" `t.Skip` into a `Fatal`: a fixture pointing at `127.0.0.1:2` that produces a routing table means the fixture is broken, and skipping hides that. *(This is a test written earlier the same day, by the same session that is now fixing it.)* |
| **`ExportFormCSV` writing raw cells** (v1.116.10) | `csvSafe` exists with its argument written out — a cell beginning `= + - @` becomes a live formula the moment the file opens in Excel or LibreOffice — and was applied by `GridToCSV` and tested with `=HYPERLINK` and `=cmd\|' /c calc'!A0`. `ExportFormCSV` wrote AcroForm names and values unguarded, and those come from an arbitrary opened PDF: exactly as untrusted as extracted table text, same output format, same download | `TestFormCSVIsAsGuardedAsTableCSV`, which asserts the four lead characters **and** walks the function's `cw.Write` call sites with a floor — the defect was a call site skipping a guard, so a behavioural test of `csvSafe` alone would not have caught it |
| **A vendored directory or an embedded font missing from the notices** | `THIRD-PARTY-NOTICES.md` has two halves and only the Go one is reconciled (`go list -deps`). The vendored half is **hand-authored**, and `TestNoticesUpToDate` cannot see a gap in it — it compares the generator's output to the committed file, and both derive from the same hand-written list. On an AGPLv3 project distributing third-party code the failure is a licence-compliance gap with no detector | `TestEveryVendoredThingIsInTheNotices`, which checks the **directories that actually ship** — the missing external reference. Both arms red-proved. It matches the font **script** rather than the family phrase, because the Noto faces share one OFL grant under a combined heading and a stricter check cried wolf against a file that does credit them |
| The CSP comment's "every innerHTML assignment a static literal" | already false when written — `app.js` has one template interpolating a page counter. Harmless (the values are integers) but it is what the next person adding one relies on | `no innerHTML assignment takes anything but a literal or a number` in `escape.test.mjs`, red-proved by replacing that site with a user-controlled value. It uses an **allowlist of interpolated expressions** rather than a regex trying to prove a bare identifier is numeric — a regex cannot, and one that admits any identifier admits exactly what this keeps out |
| **The session mode used raw** (v1.116.11) | `if mode == sessionModeReceive`, and anything else co-signed — byte-for-byte the defect `checkTransport` was written to refuse, with sixteen lines of argument and a sibling test of nearly the same name. `"Receive"`, `"recieve"` and `"transfer"` all silently armed a **co-signing** listener for a user who asked for a one-way transfer, which is the difference between showing someone a document and putting your signing key on it. There was one constant and no Go name for the other member — the client called it `'cosign'` and Go knew it only by negation | `TestAnUnknownSessionModeIsRefusedNotSilentlyDowngraded`, with both real modes and the empty string older clients send as controls |
| A path-less document losing its name | `/api/upload`, `/api/combine` and `/api/office` patched `docResponse.Name` after the fact and stored it nowhere, so `docName(doc.path)` returned `""` and `GET /api/docs` reported that forever after. The client only assigns `originalName` when `meta.name` is truthy, so a reload rendered the tab "Untitled" and exports defaulted to "document" — the second half of the defect `/api/docs` was built to fix, which restored those documents' reachability without their identity | `TestAPathlessDocumentKeepsItsNameAcrossAReload`, which asserts the install response names it (the half that always worked) before grading the reload |
| **`TestReasonResistsTokenInjection`'s spoof, 62 hex characters** (v1.116.12) | `spkiToken` requires exactly **64** followed by `]`, so `"aaaa" + 58 zeros` could never match whether or not `safeText` stripped the brackets. Measured: with `safeText` replaced by the identity function the test still passed. It is the **only** test of `safeText`, and `safeText` is the only defence `attestation.go` names | making the spoof 64 characters — a two-character fix to a test that had been standing in for a security property since it was written. `reason()`'s two RAW interpolations (`AcceptedPeer`, `RosterHash`) now go through `safeHex` as well, since `safeText`'s doc claims "the real token is the only one that can appear" and both parsers take the FIRST match |
| *(no red proof — recorded as such)* **`sshkey.Generate`'s overwrite refusal** | `os.Stat` + `os.WriteFile` does not enforce "refuses to overwrite": WriteFile is `O_TRUNC`, so any Stat error other than `ErrNotExist` fell through to truncating what is typically `~/.ssh/id_ed25519` — the user's SSH identity **and** the key the vault's content key is sealed to. Moved to `O_CREATE\|O_EXCL` | **none, and the attempt is the entry.** The obvious probe — a key in a directory with no execute permission — has `os.Stat` fail with EACCES and then `os.WriteFile` fail with EACCES too, so the old code refused for its own reason and the test passed against it. The real difference is the TOCTOU window, which a test cannot open. The test was deleted rather than kept as a green implying coverage it does not have |
| **`Scan` and `StripActive` blind to field-level scripts** (v1.116.14) | both read `/AcroForm` only for `/XFA`. The page walk catches `/AA` and `/A` on widget ANNOTATIONS, which covers a merged field+widget dict and nothing else — so a **parent** field dict (what Acrobat produces for a multi-widget field, and §12.7.5.3's home for `/AA /K` keystroke, `/F` format, `/V` validate and `/C` calculate scripts) was never visited. A PDF whose only active content was field JavaScript scanned **clean**, and because `server/scan.go`'s residual re-scan is this same detector, StripActive then reported "all active content neutralized" with the scripts intact | `TestFieldLevelScriptsAreSeenAndStripped`, both doors red-proved separately. Its fixture asserts **two** stimuli independently of the walker under test — the parent really carries `/AA`, and no page annotation does, so it cannot pass against a shape the page walk already covered. Building it hit two pdfcpu population traps in a row: `api.ReadContext` does not populate the page tree (`PageDict` → "page not found"), and the validator refuses a form field with no `/DA`, so the fixture was not a PDF a reader would accept and the walk was never reached |
| **A second spoken check displacing the one on screen** (v1.116.15) | `setVerify` assigned unconditionally while `clearVerifyIf` beside it carries an identity guard — two halves of one invariant disagreeing. Two gates can genuinely be in flight (an armed receive session while the user posts `/api/session/send`); the second overwrote the first, the displaced goroutine sat on a channel nobody would write to until its five-minute timeout, and `respondVerify` routed the answer to whichever was current. The user cannot tell them apart: `verifyView` deliberately carries no fingerprint and no peer label, which is right for one gate and is exactly what makes two unanswerable | `TestASecondSpokenCheckCannotDisplaceTheOneOnScreen`, which asserts the incumbent is really parked first (so a `setVerify` refusing everything cannot pass) and that the seat frees after it clears (so the refusal is not a one-shot for the process) |
| **The TLS handshake on the accept path** (v1.116.15) | `Accept` did `ln.Accept()` then `TLSChannel(tc)` in one call, and the server's arm loop is a single goroutine calling it — so one TCP connection that opened and sent nothing held it for the whole 30 s `handshakeTimeout`. The listener binds `0.0.0.0:0` with its port broadcast to the multicast group every 500 ms, so ten of them consumed the entire five-minute arm window and the genuine peer was never accepted. **No scan required** | `TestAStalledPeerDoesNotBlockTheAcceptPath`, which **measures** rather than asserting survival: five stalled connections in front, genuine peer accepted in **2.1 ms**. That distinction is the finding — `TestAStrayConnectionDoesNotConsumeTheSession` already proved the session survives a stray connection and never measured that the loop was blocked, which is how the fix for the one-shot-consumption defect left this in place and untested. `internal/server/l1_test.go` had stated the property verbatim ("measured at 30 s per stalled connection, and filed as its own item") for two phases |
| **Page operations dropping the ceremony record** (v1.116.16) | `api.Collect` builds a brand-new context (`ExtractPages` → `CreateContextWithXRefTable` → `AddPages`) and migrates only the selected page dicts, the AcroForm fields whose widgets are on those pages, and `Names["Dests"]`. Everything else in the catalog is left behind — including `Names/EmbeddedFiles`, and **`nib-ceremony.json` is an attachment**, so reordering or deleting a page after `PrepareDocument` destroyed the record and the handler returned 200 | `TestPageOperationsKeepWhatIsNotPageIndexed`, both doors red-proved separately, asserting the attachment is **readable** and not merely present |
| *(a refuted premise, recorded)* the same review filed this as an **asymmetry** — *"delete → RemovePages keeps them; reorder, duplicate, split, crop and redact drop them"* | **measured: both lose them.** A five-page document carrying an outline, `/Lang` and an attachment came back `outline=false lang=false attach=false` from `Collect` **and** from `RemovePages`. The asymmetry was not real and the loss was total, which changed the fix from "make Collect behave like RemovePages" to "neither preserves document state" | the `RemovePages` arm of the same test, red on its own |
| *(a fix that failed loudly, recorded)* splicing `Names["EmbeddedFiles"]` across contexts | the obvious carry — lift the name tree from the source catalog into the destination — produces indirect refs into the **source** xref, so they dangle. It did not fail silently: the result would not re-read at all, `dict=fileSpecDict required entry=F missing`. Extract-and-re-add through the package's own primitives costs a rewrite per attachment and produces a document a reader accepts | the test asserts `ExtractAttachment` returns the original bytes, not just that `EmbeddedFiles` exists — which is what separates a carried attachment from a carried dangling reference |
| **The roster hash not binding who convened** (v1.116.17) | `RosterHash`'s exclusion list named only the six-word name and the secret, so `ConvenerCert`'s absence looked exactly like a decision — the C2 shape, where an omission and a decision are indistinguishable in code. It was not one: any roster member could re-sign an unchanged roster with their own key, `Verify` passed (it asks only that the signer appear SOMEWHERE in the roster), the hash and `RosterToken` were byte-identical, and `Convener()` then named the new signer. A verifier reading the finished document could not tell which of them convened | `TestTheRosterHashBindsWhoConvened`. It binds the **fingerprint**, not the certificate — a cert re-issued for the same key would otherwise change the token for a ceremony that has not changed — with same-identity re-signing as the control. And it asserts the forgery still **verifies**: this makes it visible, not impossible, and a test claiming refusal would describe a property the change does not have. `FormatVersion` → 3 |
| *(a stimulus the fix invalidated, recorded)* | `TestAnInvitationCatchesARecordConvenedBySomeoneElse` asserted the roster token was **unchanged** — correct at v2, and the reason `MatchesRecord` was the only thing that could catch the substitution. v3 makes the record self-describing, so the assertion inverted: the token must now differ. Both checks exist and answer different questions — "who does this record say convened" and "is that who the invitation told me to expect" | the same test, updated in place with the reasoning rather than deleted |
| **`ContentDigest` blind to `/Resources`** (v1.116.18) | the content stream is half of what is on a page. For a scan — Nib's central case — the stream is invariant boilerplate (`q W 0 0 H 0 0 cm /Im0 Do Q`) and the entire visible page is the image XObject. A party receiving a prepared contract before the first signature could swap the image bytes — clauses, amounts, the signature block — and `CheckDocument` returned clean. The function had **no test of any kind**, despite its doc making four specific measured claims | `TestContentDigestIsStableAcrossARewriteAndSeesTheImage`, which asserts **both** halves of a design in tension: stable across a rewrite (with the non-idempotence asserted as its own stimulus, or "stable" is a claim about an operation that changed nothing) and sensitive to the page image. Measured before building: the resource digest is stable across two `Optimize` passes whose raw bytes differ, so folding it in does not reintroduce the non-idempotence the function exists to avoid |
| Resource NAMES hashed but not their bytes | **the first version of the strong assertion missed this.** Emptying the hashed bodies while keeping the decode marker left the XOR-corruption test green — the marker flips 1→0 when garbage stops decoding, so corruption was caught and *substitution* was not. A substituted contract stays decodable | rewritten as two documents with the **same page geometry and different pictures**, so their content streams are byte-identical and any difference can only come from the resources. Both mutants red |
| **`Collect` carrying attachments into a REDACTION** (v1.117.0) | v1.116.16 made the page-selection primitive re-add the source's embedded files so a reorder would not destroy the ceremony record — and `RedactPages` builds its untouched runs with that primitive, so every redaction, split and page-export started shipping every embedded file. **Measured: a document carrying `original-unredacted.xlsx` came out of a redaction with `SECRET PAYROLL` readable.** `RedactPages`' contract is that the content is *"genuinely gone — not merely covered"*, and an embedded file is not covered by rasterising a page | `TestARedactionDoesNotShipTheEmbeddedOriginal`, and the sibling `TestPageOperationsKeepWhatIsNotPageIndexed` now pins **where each half lives** — `/Lang` in the primitive, attachments in an opt-in the rearrangement ops call — because fixing one of these broke the other, and a future change that moves either back has to fail one of them |
| **`ContentDigest` binding the stream body and not its dict** | `/Decode [1 0 1 0 1 0]` inverts the whole page image, `/ColorSpace` re-pointed at an all-white palette blanks it, `/Width` reinterprets the same bytes — all with an identical digest. The same attack the coverage was added to close, **one dictionary key over** | `TestContentDigestMovesOnEveryAxisAReaderCanSee`, a table so that "some mutation moves it" cannot stand in for six axes. Three arms red-proved separately |
| Page geometry not covered | `/MediaBox`, `/CropBox` and `/Rotate` were never read. Shrinking `/CropBox` excises a paragraph from what every reader displays. Not in the stated exclusion list either — simply missing | the `/CropBox` and `/Rotate` arms |
| Annotations and form values excluded on a false premise | the exclusion argued *"everything else is covered by the signatures"*. `CheckDocument` exists for the **pre-first-signature** hop, where there are none — and in that window **Nib's own operations** defeat a content-only digest: `AddNotes` writes `/Text` annotations and the form fill writes `/V` and widget `/AP`. For a contract, the form values *are* the agreement | the sticky-note arm |
| **`"Font"` in `resourceKinds` inert** | `DereferenceStreamDict` type-asserts `types.StreamDict` and errors otherwise (pdfcpu v0.13.0 `model/xreftable.go:989`); a page `/Font` entry is a font **dictionary**. So the loop `continue`d on every font on every page while the doc claimed fonts were folded in | `TestContentDigestSeesAPageLevelFont`. **Its own first fixture could not prove it**: pdfcpu nests a watermark's font inside a form XObject, so that assertion is carried by the recursion and dropping `"Font"` left it green — measured. An ordinary `testpdf.Text` document has the font on the page, which is the entry `resourceKinds` exists for |
| **The stream body not hashed at all** | every document-level axis is carried by the **dict** — two images of the same dimensions still differ in `/Length`. Measured: replacing `hashChunk(h, body)` with a zero-length write left the whole package green | `TestTheStreamBodyItselfEntersTheDigest`, two streams with byte-identical dicts and differing content — the one construction a document fixture cannot produce, because any real re-encode moves `/Length` too. That is the substitution that matters: a swapped page image that stays decodable and the same size |
| An indirect `/Lang` copied as the object found | `/Lang` is legally an indirect reference, and the first version assumed *"a plain string in the catalog, so it copies directly"* — writing an object number from the **source** xref into the destination catalog, which is the dangling-reference bug the same file's attachment comment warns about twelve lines below. A dict there fails `validateStringEntry`, after which every `pdfops` entry point refuses the document and the user can no longer open, save or sign it | *(no red proof — recorded: no fixture in the tree carries an indirect `/Lang`, and manufacturing one to prove a dereference is testing `DereferenceStringLiteral`. The fix is the deref itself.)* |
| **Two termination signals in one listener** (v1.117.1) | `loop` did `defer close(l.ready)` while handshake goroutines were still selecting on a send into it — up to `handshakeTimeout` later, so a **30-second window**, not an instant. A send on a closed channel is a *ready* select case that panics: after `Close` both cases were ready and Go chose pseudo-randomly (**≈50 % of every in-flight successful handshake**); after a non-close accept error `done` was never closed, so the send was the only case and it panicked **every time**. `safe.Recover` kept the process alive and leaked the connection unclosed | restoring `defer close(l.ready)` goes red under `-race`. `ready` is now never closed and `Close` is the one termination signal, so the hazard is structurally impossible rather than argued |
| `closeErr` returning the bare accept error | `runSession` exits only on `errors.Is(err, net.ErrClosed)`, so a terminal `EMFILE` made that loop spin at **100 % of a core with no syscall** — and `Close` could not rescue it, because it sets `cerr` only when `cerr` is nil, so the disarm timer fired and `closeErr` went on returning `EMFILE` forever. `runSession` never returned, its defers never ran, and the LAN announcer broadcast for the life of the process. `quicListener.Accept` already maps its equivalent, with a comment naming the same hazard | `TestATerminalAcceptErrorStillReportsNetErrClosed`, driven through a **fake listener that returns EMFILE from every Accept** — the real trigger is descriptor exhaustion and a test cannot arrange that without wrecking the machine, but the path is the one that produced the permanent spin, so it needs a driver. Asserts the loop closed the underlying listener before grading the error |
| The handshake pool unbounded | **the kernel backlog used to be this bound, and moving the handshake off the accept path removed it.** One goroutine and one fd per inbound connection for a 30 s timeout, driven by any host on the segment against a port broadcast every 500 ms, at one SYN each; a GUI process on macOS has 256 descriptors | a bound of 16 with a **blocking** acquire, so a full pool stalls the accept loop and pushes the excess back into the kernel backlog — where it was before, and where it costs Nib nothing |
| *(the six questions that had no answer)* | `Accept` after `Close`, a second `Close`, `Close` before `Accept` was ever called, `Close` with a handshake in flight, a non-`ErrClosed` accept failure, and the pool bound. **`TestAStalledPeerDoesNotBlockTheAcceptPath` uses connections that never speak, so not one of them ever reaches the statement that panicked** | `TestTheListenerTerminatesThroughExactlyOneDoor`, five subtests, plus the terminal-error test above |
| **The replay harness accepting any non-zero exit** (v1.117.2) | this file's own V1 defect, in the file that teaches V1: `redproof.sh` asserted only that `$PROVE` failed, so a check that had been **deleted** reported *"still goes red against its own defect"*. Measured: `node --test <deleted file>` exits 1, and the tier-2 row's patch touches `web/style.css` rather than the test file. Any compile break or missing `node_modules` in the exported tree does the same. *(The tier-1 row was safer only by accident — `go test -run <nonexistent>` exits 0.)* | every row now records an `EXPECT` token the real assertion prints, and the harness greps for it. Red-proved by pointing a row's `EXPECT` at a string its check never emits: the run goes red and the harness now says **"went red, but not for its own reason"**. A row with no `EXPECT` is a hard error rather than a permissive default |
| The replay set counted only in prose | `docs/red-proofs.md` says two rows are recorded; nothing counted the directory, and `redproof.sh` prints `(none)` and **exits 0** on an empty one — so deleting both pairs left the doc asserting two and the harness reporting no error. V2: a rule inventory whose count lives inside the thing it describes | `verify_test.go` now walks `test/redproofs/*.sh` with a floor, requires a matching `.patch`, and requires an `EXPECT` in each. Both arms red-proved. `build/redproof.sh` also joins the harness list that is held to exists-executable-and-named-in-the-contract — it had **no reader anywhere**, so deleting it would have taken the ledger's only mechanism with it and no tier would have noticed |
| **A prepended unresolvable attestation denying a genuine proof** (v1.117.3) | the per-attestation loop was added to stop exactly this and left the cheapest shape open: an attestation at a height **nothing resolves**. Every explorer 404s, `fetchAgreedHeader` errors, and the loop declared that terminal — refusing the document before the genuine branch was ever computed. About thirteen bytes of edit. *"The network refusal is the same for every branch"* is true of a transport failure and **false of a refusal that is about this height**, which is a property of the branch | `TestAPrependedUnresolvableAttestationDoesNotDenyAGenuineProof`, asserting both stimuli — the bogus height really does not resolve, and the genuine one really does — before grading. A fetch failure is now remembered: if no branch confirms and one failed, the error is reported (no opinion) rather than `StateInvalid` (a negative verdict never earned) |
| **A proof driving an unbounded number of outbound lookups** | `parseSequences` admits 100,000 instructions and one attestation is one, so the loop could drive that many `fetchAgreedHeader` calls — each fanning out to three explorers at two GETs apiece, from the user's IP, with `handleTimestampVerify` passing a context that has **no deadline**. ~130 KB of `.ots` for 60,000 requests: Nib as a request amplifier pointed at third parties | `TestAProofCannotDriveAnUnboundedNumberOfLookups`, with a `countingSource` because the existing httptest explorers cannot answer *"how many outbound requests did this proof cost"* — the whole question. **Both subtests caught a defect in the fix itself**: the cap first bounded `len(seen)`, which only grows on a SUCCESSFUL lookup, so 200 unresolvable heights still drove 200 fetches; and the memo subtest first used 40 *matching* attestations, which return on the first branch, so the memo was never reached and deleting it left the test green. Now: attempts are bounded, and the memo test uses non-matching tails so the loop actually iterates |
| **A dialing side's budget spanning two of the PEER's gates** (v1.117.4) | the v1.116.2 re-arm fixed the LOCAL gate and left the remote ones. `Initiate` re-arms when its own user answers — and its `readFrame` then waits on the remainder of the peer's spoken check, then the peer's consent, then the co-signature and a 128 MiB write-back. Two remote human waits against six minutes. **No attacker:** the initiator answers in five seconds, the responder takes four minutes then three — both inside the advertised windows — and the initiator times out at 6m05s while the responder co-signs at seven. Verbatim the outcome the re-arm was written to prevent, one hop out. `SendDocument` is the same shape | `TestADialingSideOutwaitsBothOfThePeersGates`, which asserts the **arithmetic** the invariant is about rather than the structure — `TestEveryEntryPointReArms…` AST-matches "is there a SetDeadline after runVerification", which is a different property and cannot see this |
| The cross-package gate coupling asserted against a copy | `internal/p2p` sizes the dialing wait from its own `PeerGateWindow` while `internal/server` enforces `sessionConsentTimeout`. Two statements of one number in two packages, and p2p **cannot** import server (the dependency runs the other way) — which is also why the drift went unnoticed | `TestTheSessionBudgetsCoverBothPeerGates`, in the package that can see both. Red-proved by widening `sessionConsentTimeout` to 9 minutes |

## v1.117.5 — the server majors batch

| Row | The defect restored | The check that went red |
| --- | --- | --- |
| ADR-003 tier 3 | trim only the undo ring | `TestASingleDocumentsUndoConverges` — a lone document never converges |
| stamp mapping | `wroteStampTextError` widened to any error / `StampPageNumbers` back to the general 500 | `TestEveryStampProducerReportsUnrepresentableTextAsTheUsersToFix` |
| path-less names | `urlDocName` without its no-path fallback, both `name:` fields removed | `TestAPathlessDocumentIsNamedByWhereItCameFrom` |
| OCR skip | `StampTextLayer` erroring instead of skipping | `TestOCRSkipsUnrepresentableWordsRatherThanFailingTheLayer` |

The OCR row is the one worth reading. The review filed `StampTextLayer` as a fourth producer
whose error was swallowed by a 422; it is not a producer at all — it skips the word and says
why (`internal/pdfops/ocr.go`). The door added for it was dead the moment it was written, and
the test's own stimulus assertion is what said so. The row now pins the *skip* instead.

## v1.117.6 — ADR-005's byte half, at every door

| Row | The defect restored | The check that went red |
| --- | --- | --- |
| growth cap | `byteCapLocked` removed from both commit doors | `TestGrowingAnOpenDocumentIsBoundedByTheSameCeiling` |
| one mapping | a call site mapping the error itself, at 404 | `TestACommitFailureIsAlwaysA409` |
| the guard's reach | matcher pointed at a name no call has | same test's floor of eight |

The middle row is the one the old guard could not have caught. It asserted the *string*
each of eight branches printed; it now asserts each call routes through the single door,
which is the property. The floor of eight is unchanged and is what caught the refactor
in the first place — it went red the moment the branches became calls.

## v1.117.7 — one loopback rule, and the site that ignores the port

| Row | The defect restored | The check that went red |
| --- | --- | --- |
| `same-site` | `case "same-origin", "same-site", "none"` | `TestASubResourceGetCannotReachTheVault` — a new row: another app on another loopback port |
| bracketed v6 | the bracket strip removed | `TestLoopbackIsOneRule` |
| the name arm | `==` replaced by `HasSuffix` / `HasPrefix` / `Contains`, three separate runs | same |

The name-arm row is the one that needed a second attempt. The first mutation was `HasSuffix`
against a table row reading `localhost.evil.example` — which is a *prefix* case, so the check
stayed green and the mutation looked survivable. The table was missing `evil.localhost`
entirely. A mutation that does not go red is a statement about the test, and the first reading
of it was wrong.

## v1.117.8 — the vault: one refusal, and a check-then-act over the only copy

| Row | The defect restored | The check that went red |
| --- | --- | --- |
| version refusal | the check moved back out of `readEnvelope` to the three callers that had it | `TestAVaultFromANewerNibIsRefusedAtEveryDoor` — Migrate, Slots, NeedsMigration |
| declared version | `save()` writing a drifted literal | `TestSaveWritesTheVersionItDeclares` |
| Create race | `O_EXCL` removed, leaving Exists → Save | `TestConcurrentCreateProducesExactlyOneVault` |
| failed Create | the placeholder's cleanup removed | `TestAFailedCreateLeavesNoVaultBehind` |

The race row is the one whose *assertion* had to be chosen carefully. Counting errors would
have passed against the shipped code, because the losers did not error — they succeeded, each
renaming a fresh content key over the last. The assertion is that exactly one call wins and
that the survivor still opens.

**Overturned:** the orphaned `.vault-*.tmp` finding. `writeFileAtomic` carries
`defer os.Remove(tmpName)` covering every error path (`internal/vault/vault.go`); only a
crash between create and rename leaves one, which no in-process cleanup can fix.

## v1.117.9 — the ceremony: a token with two implementations, and a check you delete a field to skip

| Row | The defect restored | The check that went red |
| --- | --- | --- |
| convener check | `if i.ConvenerFingerprint != ""` restored | `TestTheConvenerCheckIsNotOptIn` |
| hex case | `!=` in place of `strings.EqualFold` | same |
| refusal cause | oversize counted as `RefusedSealed` | `TestEveryRefusalCauseIsCountedUnderItsOwnName` |
| the token's shape | `[NibRoster=` at the real producer | `TestTheRosterTokenIsWellFormedWhereItIsActuallyBuilt` |
| the forgery guard | `safeText` returning its input | same |

The last two are the point of the entry. Before this, the token's format was tested through
`ceremony.Record.RosterToken` — a **second implementation with no production caller**. The
mutation above is at `p2p.Attestation.reason`, the only producer on the real path, and the
old ceremony test would have stayed green through it. The duplicate is deleted; p2p cannot
import ceremony (ceremony's own tests import p2p, so the edge is a cycle), so the coverage
moved to the producer rather than the definition moving to the consumer.

## v1.117.10 — the rendezvous: a fix that was the mirror of its own defect

| Row | The defect restored | The check that went red |
| --- | --- | --- |
| seeds-only retry | the shipped list emitted during the retry again | `TestTheRetryWithholdsTheShippedList` |
| aborted lookup | the abort branch made unreachable | `TestAnAbortedLookupIsNotAnEmptyFetch` |
| the counter's reader | the figure dropped from the disclosure | `TestTheDisclosureSaysHowMuchOfTheTableCameFromTheStranger` |

The first row is the entry. `TestTheShippedListsRotAlarmSurvivesAnInvitationRescue` fixed
"both attempts add to `bootstrapped`, so the rot alarm reads *the shipped list worked* on a
run where it did not" — by subtracting the retry's gains and crediting the invitation. But
the retry kept OFFERING the shipped list, so the same alarm read backwards: *the invitation
rescued this machine* on a run the shipped list rescued. The fix and the defect are the same
shape pointing opposite ways.

The second row's test is one `docs/red-proofs.md:343` has cited all along and which had
ceased to exist — a ledger row naming a check nothing runs. Rewritten rather than the
citation edited: the claim was right, the test went missing.

## v1.117.11 — the CLI: three sentences, two parsers, and a harness that named one package

| Row | The defect restored | The check that went red |
| --- | --- | --- |
| the shipped sentences | README's "invalid or absent" enumeration restored | `TestTheShippedSentencesDescribeTheExitCodeThatShips` |
| `-h` | `discover` hand-rolling `fs.Parse` and returning 2 | `TestEveryCommandTreatsDashHTheSameWay` |
| the banner's ordering | a line appended after the Ctrl-C invitation | `TestTheBannerPrecedesTheSocket` |
| the harness's reach | `dhtlive.sh` back to one package | `TestVerifyContractIsTrue` |

Two lessons, both about the checks rather than the code.

**A dead assertion in the branch that always runs.** `TestTheBannerPrecedesTheSocket`'s
hermetic half compared indices of `"local socket"` — a string the *banner* never contains,
since that line is printed later by `runRendezvous`. `strings.Index` returned -1, the
`i >= 0` arm was never true, and the ordering check in the only branch a normal test run
reaches had no reach at all.

**The first mutation was in the wrong branch.** The banner row above came back green on the
first attempt because the mutation appended to `banner(true)` while the hermetic test drives
`banner(false)`. That was a statement about the mutation, not about the test — the second
attempt, in the right branch, went red.

## v1.117.12 — the guards: four checks that could not fail

| Row | The defect restored | The check that went red |
| --- | --- | --- |
| innerHTML | `x.innerHTML = "<b>" + location.hash` | escape.test.mjs's innerHTML scan |
| — same | `` `x${location.hash}` `` | same |
| — same | `` `static` + location.hash `` | same |
| the notices' scope | `Bengali` dropped from the Noto heading ONLY | `TestEveryVendoredThingIsInTheNotices` |
| the notices' section | the `## Noto Sans` heading renamed | same |
| the empty token | a face named `NotoSans.ttf` | same |
| the fixture's vocabulary | `state: 'untampered'` restored | arrival.test.mjs's fixture scan |

**Three false reds, caught by running the control.** The first attempt at the notices rows
ran `TestNoticesUpToDate` — which is the *freshness* comparison, not the font loop (that is
in `TestEveryVendoredThingIsInTheNotices`). Both mutations edit the notices file, so both
went red for the byte-for-byte comparison and neither exercised the scoping at all. The
control run on the unmutated tree is what said so; without it, three rows would have been
recorded as proven against a test that never read a font.

**And one vacuous green of my own.** The first draft of the arrival assertion called
`h.window.nibTestRefreshAfterArrival?.()` — a hook that does not exist, so the optional call
no-opped and the badge kept the state a previous test had left. Rewritten as a scan over
every jsdom fixture, with the vocabulary read out of `internal/sign/verify.go` rather than
listed in the test, because a list written in a test agrees with itself.

## v1.117.13 — mdpdf: a clamp test that never called the renderer

| Row | The defect restored | The check that went red |
| --- | --- | --- |
| the list clamp | `mdpdf.go`'s list clamp deleted | `…/list` only |
| the quote clamp | `mdpdf.go`'s quote clamp deleted | `…/quote` only |
| the nesting refusal | `refuseAbsurdNesting` call removed | `TestAbsurdNestingIsRefusedRatherThanParsedForMinutes` |
| — its quote arm | the `'>'` case removed from the depth count | same |
| — its bound | `maxNestDepth` set to 2 | same, via the controls |

`TestNoBlockIndentEverExceedsTheClamp` walked `indent += step`, applied
`if indent > maxBlockIndent` **in its own loop**, and asserted the result — proving that the
arithmetic written in the test clamps. Both real clamps could be deleted with it green. It
now renders, and each deletion fails exactly its own subtest.

Two measurement corrections while building it. The test's own comment said the page-count
harm was *"NOT reproducible for lists"*; it is reproducible for both — the fixture was a
single sentence, and 44 runes on 44 lines still fit one page. With a paragraph, deleting the
quote clamp takes the same input from **1 page to 17**. And the first list fixture gave every
level its own paragraph, so the deep case had 28 against the control's 2 and failed against
correct code — a confound, not a finding.

**Overturned:** mdpdf's "unbounded recursion". No stack overflow at 50,000 levels of quote,
list, emphasis or bracket nesting; goldmark bounds it. What the probe found instead is on the
next row.

## v1.117.14 — the platform nobody compiled for

| Row | The defect restored | The check that went red |
| --- | --- | --- |
| the Windows build | `syscall.SetsockoptInt(int(fd), …)` back inline | `TestEveryPlatformCompiles/windows` |
| the symlink | `os.ReadFile` at both watch call sites | `TestTheWatchRefusesToReadThroughASymlink` |
| `O_NOFOLLOW` | dropped from `oNoFollow` | same |
| `O_NONBLOCK` | dropped from `oNoFollow` | same, by timing out on the fifo |

`nib.exe` could not be built. `internal/discovery` passed an `int` file descriptor to
`syscall.SetsockoptInt`, which takes a `syscall.Handle` on Windows — and every tier builds
for the host, so nothing saw it. `mcast.go`'s own note had argued against "a `//go:build`
file per platform" because "a no-op sibling is the shape that already shipped one silent
defect here"; it was right about the hazard and the outcome was worse than the hazard.

Two corrections while building the watch fix. The comment first claimed `O_NOFOLLOW` is
"defined and ignored" on Windows — `GOOS=windows go build` said it is not defined at all.
And `O_NOFOLLOW` refuses a symlink but not a **fifo**, and opening a fifo for reading blocks
until a writer appears, so the regular-file check on the handle never ran: a `pipe.pdf`
dropped into a watched directory hung the loop until Ctrl-C. The test's five-second timeout
is what found it.

## v1.117.16–.19 — the discovery sweep (pending items 6–9)

Four items off the backlog, each red-proved before it closed. The tier-4 row is the one
worth reading: it is the first time this repo has put a defect back and watched a **real
ceremony between two real binaries** fail with the reported symptom.

| Defect reintroduced | What it said | Check that fired |
| --- | --- | --- |
| **The loopback rule removed from `startAnnouncing`** (v1.117.16) | `bind 127.0.0.1:13897 is loopback and was announced on every joined interface (err=<nil>)` — the nil error is the point: the socket really opened and the announcer really ran. Also fired for `127.0.0.2` and `::1` | `TestALoopbackBindIsNotAnnouncedOnTheLink`. Its wildcard case is the other half and asserts the refusal does NOT fire: a `startAnnouncing` that refused everything passes the loopback assertion and breaks the LAN tier outright |
| **A second `&lanAnnouncer{}` construction site** (v1.117.16) | `an announcer is built at [startAnnouncing redProofSecondDoor]` | `TestAnnouncingHasExactlyOneDoor`. ADR-009's half: a rule inside one function is worth nothing if a second site can skip it, and asserting the *text* of `startAnnouncing` would say nothing about a site written elsewhere |
| **The browse's early exit removed** (v1.117.17) | `the browse spent 3.001765252s of its 3s window after hearing its answer at ~0s` | `TestABrowseStopsOnceTheLinkGoesQuiet`, which also asserts a LOWER bound — a browse returning at the first announcement would reintroduce the capture the multi-candidate fix removed |
| **`browseQuiet` shortened to 100 ms** (v1.117.17) | `browseQuiet is 100ms and announceEvery is 500ms; a quiet period no longer than one announce period cannot outlast an announcer whose ticker is offset` | `TestBrowseQuietIsDerivedFromTheAnnouncer`, plus the offset test's own **setup** assertion refusing to run vacuously |
| **The quiet period set once, never reset** (v1.117.17) | `browse returned 2 candidates, want 3` | `TestAnAnnouncerOffsetByOnePeriodIsStillHeard`. It needs THREE announcers: two inside one window are collected even by a fixed grace period, so a two-announcer test cannot tell a quiet period from a grace period. The first draft had two and proved nothing about the reset |
| **`nib discover`'s verdict switch reordered** (v1.117.18) | `VERDICT: 0 announcements left this machine and NOT ONE came back to us. A local firewall is dropping multicast on port 8446` — about a machine where **nothing was ever sent**. A confident wrong diagnosis, pointing the user at their firewall instead of at the interface list three lines above | `TestNothingSentIsDiagnosedBeforeNothingReturned` |
| **Two of the four verdicts collapsed to one message** (v1.117.18) | `states [0 2] all print the same verdict, so the command cannot tell them apart` | `TestTheFourVerdictsAreFourDifferentMessages`. The per-branch substring table cannot see this: each collapsed branch still contains its own substring |
| **The `OffLink` line dropped from the summary** (v1.117.18) | `summary omits the counter whose value is 88` | `TestEverySummaryCounterIsPrinted`, which asserts the COUNT rather than eight labels — OffLink slipped past this summary for a whole phase, and a list of names would let the tenth counter do it again |
| **The `--seconds` guard removed** (v1.117.18) | `nib discover --seconds 0` exited **1** (a verdict about the machine) instead of **2** (a usage error) | `TestANonPositiveWindowRefusesInsteadOfDiagnosing` |
| **The announced transport ignored — `candidate.Transport` forced to `""`** (v1.117.19) | at tier 1: `the candidate's transport is ""`. **At tier 4 `--lan`, the real thing:** `FAIL: [quic] initiate returned HTTP 502 … {"error":"could not connect to peer: tried 2 address(es), none answered as the pinned peer: dial tcp [fe80::4c7e:82ff:fea6:a647%d0]:60382: connect: connection refused"}` — a TCP dial at a QUIC peer's UDP port, surfaced to the user as an unreachable peer | `TestAQUICArmedPeerIsDialledOverQUIC` (tier 1, carrying its own red proof inline: the same address with the transport the old code chose must NOT connect), and `./build/pairrepro.sh --lan`'s new QUIC run (tier 4) |

**The tier-4 row could not have existed last week.** `pairrepro.sh` passed `-F transport=`
to *both* sides in every mode, so the harness was configured past the disagreement it
exists to find, and its LAN mode ran TCP only. Both are fixed in the same change: the LAN
runs tell the armed side only, and there are now two of them. A harness that tells both
sides the answer is not testing the protocol — it is testing that two programs given the
same constant agree.

## v1.117.21 — P05.S01, the arm and what spends it

The slice moves what consumes a one-shot armed session from *"a connection completed a
handshake"* to *"a connection produced an outcome"*. That is one rule with **three** arms,
and each needed its own proof — L1 alone is satisfied by a listener that never disarms.

| Defect reintroduced | What it said | Check that fired |
| --- | --- | --- |
| **The old rule: any completed handshake spends the arm** | `a pinned peer that completed a handshake and then closed without producing a session consumed the arm — the user's receive is gone and the peer they are waiting for can no longer reach them, for a connection that exchanged nothing` | `TestACompletedHandshakeThatProducesNoSessionLeavesTheArmOpen`. Its sibling `TestAStrayConnectionDoesNotConsumeTheSession` cannot see this: its stray is plain TCP with junk bytes, so it never reaches the statement |
| **`armedUntil` reassigned inside the accept loop** | `` `armedUntil` is assigned 2 times; the arm deadline must be fixed once, at arm time `` and `assigned inside the accept loop (1 time(s)), so every connection that produces no session pushes the arm window out` | `TestTheArmWindowIsNotExtendedByConnectionsThatProduceNoSession`, an AST check on the routing rather than on any line's text — the behavioural form costs 5 minutes of wall clock |
| **A completed session reports "not served"** | `the listener is still armed after a completed co-signing session — one arm has served more than one session, which is the containment the session TRIPWIRE names` | `TestSessionArmReceiveSign`'s new tail. **This is the counter-arm**: without it, a `serveOneSession` that always returned false passes every "the arm survives" assertion and silently removes D22 |
| **A decline reports "not served"** | `the listener is still armed after the user declined — the peer can re-dial and ask again, and the decline was treated as a failed connection` | `TestSessionDeclineLeavesOpenDoc` and `TestSessionReceiveTransferDecline`, one per sentinel. The co-signing decline had to *become* a sentinel (`p2p.ErrCoSignDeclined`) to be distinguishable — it was a bare `errors.New("co-signing declined")` one line from a protocol error |

| **The outcome ENUMERATION restored** (the slice's own first implementation) | `the listener is STILL ARMED after the user said the verification words did not match — the man-in-the-middle signal was filed as a failed connection, so the listener retries automatically and the attacker gets another attempt with no user action and no warning` | `TestADeclinedSpokenCheckSpendsTheArm`. **The slice's review found this, not its author.** Measured at two full spoken-check rounds in 0.47 s against a listener still reporting `Armed: true` |
| **The timer reset to a full `sessionAcceptTimeout`** | `the accept timer is reset to "sessionAcceptTimeout"; it must be reset to the REMAINDER of the window fixed at arm time` | the second half of `TestTheArmWindowIsNotExtended…`, added by the review — the first half polices `armedUntil` and stays **green** against this defect, so it could fail for a renamed variable and not for the behaviour |
| **The old spend-on-handshake rule, against the racer case** | the Verify gate never appears on the connection after the abandoned one | `TestAnAbandonedConnectionIsFollowedByAWorkingSession` — the only check here that proves the server *accepted* the abandoned connection and carried on, since `p2p.Dial` returns on the CLIENT's handshake and every other assertion would pass with the accept path deleted |

**The lesson, and it is about the shape of a rule rather than a missing case.** The first
implementation *enumerated* the outcomes that spend the arm. Enumerations of this kind fail
in both directions at once and did: it omitted `ErrVerificationDeclined`, the
man-in-the-middle signal, so the listener performed on the user's behalf exactly the retry
`internal/p2p/verify.go` says must never be invited; and it *claimed* an unanswered consent
left the arm open when `Confirm` returns `accept=false, err=nil` on timeout, so that case
had always arrived as a decline. Both were confident false statements in a comment about
code three lines away. The replacement asks one question the enumeration was a proxy for —
**did this connection put anything in front of the user?** — and its default for an error
nobody anticipated is the pre-slice behaviour rather than the loosened one.

**A latent test-isolation defect surfaced by adding a file, and it was ordering that hid
it.** `startServer` gives every test a fresh `HOME` from `t.TempDir()`, and pdfcpu caches
its user-font directory in a package-level global at first use. So the first
server-starting test in the package captured a directory that stopped existing when that
test ended. It had never fired because no such test sorted before `office_test.go`;
`armsurvival_test.go` does. Fixed by pinning one `XDG_CONFIG_HOME` for the package run in
`TestMain` — the *product* is right (one process, one HOME, one font directory), the
harness was not.

## v1.117.24 — P05.S03's unmet acceptance, found by grilling P05.S04

**Three of P05.S03's seven acceptance bullets shipped unmet and none appeared in its
ledger**, because that ledger reconciled against the phase exit criteria and never against
the slice's own `Acceptance:` line. All three become live the moment a second candidate
source exists, which is exactly what S04 adds — so they were remediated first, in their own
commit, before that slice opens. The fourth row is a defect the grill found while attacking
them; the fifth is one I nearly shipped in the fix itself.

| Defect reintroduced | What it said | Check that fired |
| --- | --- | --- |
| **The feeder's `ctx.Done()` arm removed — back to `for c := range in`** *(replayable: `race-feed-leaks-on-win`)* | `a candidate offered AFTER the race returned was consumed, so the feed goroutine is still running: it is blocked on the input channel with no ctx arm, its close(results) will never run, and the drain goroutine leaks with it` | `TestTheFeedStopsWhenTheRaceIsWon`. **No existing test could reach this**: `dialAny` closes the channel it builds, and the one test that drives an open channel drives the LOSS path, where the deadline ends the race anyway. The WIN path with an open channel had no test at all — and that is the shape S04's DHT feed introduces |
| **The per-source cap removed, leaving only the global one** *(replayable: `race-cap-not-per-source`)* | `one source offered 12 candidates and the race reported "tried 12 address(es)"; it must dial at most 8 from a single source, or a flooding tier spends the whole budget and the honest tier is never reached` | `TestOneSourceCannotSpendTheWholeRaceBudget`, plus `TestTwoSourcesEachGetTheirShare` as the counter-arm — without it, a racer that simply lowered the global cap to 8 passes the first test and starves the second tier |
| **`safe.Recover`'s body gutted to `_ = label`** | `the panic escaped safe.Recover and reached this frame … the AST guard below is satisfied by that NAME — so if the function itself does not swallow a panic, every one of those goroutines is unprotected while the guard stays green` | `TestSafeRecoverActuallyRecovers`. `internal/safe` had **no test of any kind**; the first draft of this row let the panic kill the test binary on a raw stack — red, but not for its own reason (`redproof.sh`'s third failure mode), so the test now catches it and names it |
| **One racer goroutine's `defer safe.Recover` removed** | `lan.go:373 launches a goroutine whose first statement is not defer safe.Recover(...)` | `TestEveryDetachedGoroutineIsRecovered`. It replaces a **comment** at `lan.go` asserting the announcer was "the one `go func` in internal/server without it" — true when written, false from S03, which added four and recovered none. A sentence cannot notice a fifth goroutine. Its first draft also reported `go s.runSession(...)` as unrecovered, which is wrong — that function recovers itself one frame down — so the guard now resolves same-package callees, with a stimulus assertion that the resolution arm was actually exercised |
| **`Source: sourceLAN` dropped from `resolve`** (the defect in the fix) | `discover.go:132 builds a dialable candidate without a Source. It will be accounted to the zero-value source, so one tier spends another tier's share of the race` | `TestEveryCandidateProducerNamesItsSource`. **I shipped this and the tests stayed green**, because every test set `Source` by hand — the fixture supplying what production omits. Caught by asking who the producers were, not by running anything. The guard's own first draft then found one producer of two and said so through its stimulus assertion: the typed-address producer is `[]candidate{{…}}`, an elided literal with a nil type |

## v1.117.26 — P05.S04 T01, a live bypass in the address table

| Defect reintroduced | What it said | Check that fired |
| --- | --- | --- |
| **`a.WithZone("")` removed from `addrscope.Routable`** *(replayable: `zone-bypasses-reserved`)* | `::c0a8:101%eth0 is Routable but ::c0a8:101 is not — the same address, and the only difference is a zone. 192.168.1.1 inside ::/96` and `Target accepts ::c0a8:101%eth0:5000 — a candidate record naming it would be sealed, published, opened and dialled` | `TestAZoneCannotSmuggleAnAddressPastTheReservedTable`. Each case carries its bare form as a **control**, because a table that refused everything would pass the zoned assertion and break every tier that ends in a dialable address; and the test asserts the other direction too — a zone on a genuinely global address must still be dialable, or stripping is indistinguishable from disqualifying |

## v1.117.27 — P05.S04 T02-T04, the candidate record carries its transport

| Defect reintroduced | What it said | Check that fired |
| --- | --- | --- |
| **The transport chunk dropped from `preimage()`** | four round-trip tests fail together — `TestACandidateRecordSurvivesThePublishRoundTrip`, `TestAnInvitedPartyCannotPublishAsAnother`, `TestASealOpensRegardlessOfSeq`, `TestACandidateFromAnotherCeremonyIsRefused` | the canonical re-encode at the end of `parseCandidate` does the work: a plaintext whose grammar differs from what `preimage()` writes cannot equal the bytes consumed, which is the same property that makes the encoding bijective |
| **The transport range check removed** | `transport 2 refused with "this is not a candidate record this version of Nib understands", which does not name the field` — and the same for 7, 255 and 256 | `TestAnUnknownTransportByteIsRefusedNotDefaulted`. **Written because removing the check left the package GREEN**: the canonical re-encode does eventually refuse such a record, but as a non-canonical plaintext, so the refusal pointed at the wrong thing. 256 is in the table because it narrows to 0 and would read as TCP — a byte an attacker picked, silently becoming the conservative default, which is exactly what ADR-010 says an enumeration exists to prevent |
| **The version check moved back to `Verify` from `parseCandidate`** | `a version-skewed record was refused as "this is not a candidate record this version of Nib understands: version 3"; want ErrVersion, naming the version the record carries and the one this build writes` | `TestAVersionSkewedRecordSaysSoInsteadOfAccusingThePeer`, **tightened in the same change**: it accepted `ErrVersion` OR `ErrCandidateFormat`, and the second is precisely the outcome T03 removed, so it could not tell the fix from its absence. Its body was also v1-shaped against a v2 parser, which would have kept it passing on a grammar mismatch wearing a version number |

**Measured at the bump**, because the margin is now the thing to watch: 8 IPv4 candidates seal to
**701 bytes** (was 574) and the IPv6 worst case to **932 of 996** (was 806). Headroom fell 190 → 64
and the IPv6 endpoint ceiling fell 11 → 8, so the count cap and the byte cap are now coincident for
IPv6. The next axis added to this record wants a cheaper encoding, not another chunk.

## v1.117.28 — P05.S04 T05-T07, what a ceremony record means

| Defect reintroduced | What it said | Check that fired |
| --- | --- | --- |
| **`MaxCeremonyLife` removed from `Record.Verify`** | `a deadline 721h0m0s ahead verified as <nil>; want ErrCeremonyTooLong. This is an externally-supplied security parameter and the plan's own rule for all four of them is that they are enforced rather than documented` | `TestACeremonyDeadlineHasACeiling`. It carries three companions the ceiling alone would get wrong: a deadline INSIDE the ceiling must verify (or the bound is indistinguishable from a blanket refusal); an EXPIRED ceremony must still verify (a signed record has to stay checkable after the proceeding ends, or the document's own evidence expires with it); and a record both forged and over-long must be reported as forged |
| **`PublishSalt()` returns the READ salt — the shape the API had before it existed** | `setup: this gate's read and publish salts are identical, so it is a one-party ceremony and cannot distinguish the two` | `TestAHopHasTwoSaltsAndTheyAreNotInterchangeable`. **The failure is caught by the test's own setup assertion**, which is the point: the one example in the tree was a one-party self-test where the two salts coincide, so every assertion about them was vacuous there |
| **The same-party guard removed from `NewCandidateGate`** | `a gate was built with one fingerprint as both ends of the hop. Every salt, key and target it derives would be self-consistent, so nothing would fail — the symptom is a counterparty who never publishes, which reads as an offline peer` | `TestAHopNeedsTwoDistinctParties` |
| **Non-adjacent parties allowed to name a hop** | `convener and bob are two apart and got hop <nil>; a hop joins adjacent parties, and letting a non-adjacent pair name one is how a convener ends up dialling a party three positions away` | `TestTheHopNumberComesFromTheSignedRoster` — criterion 19 made structural rather than remembered |

## v1.117.29 — P05.S04 T10, Close cancels and joins

| Defect reintroduced | What it said | Check that fired |
| --- | --- | --- |
| **`stopLive()` removed from `Close`** | Close took 2s+, at least as long as the fake held the publish for — so it waited the operation out instead of cancelling it | `TestCloseCancelsAndJoinsAnInFlightPublish` |
| **The cancellation re-check removed from `Publish`** | `the in-flight Publish returned <nil>; want context.Canceled` | the same test. **This found a real false success**: `getput.Put` logs each node's put error and discards it, and sets its own `err` only when the GET traversal is cancelled — so a publish cancelled during the PUT phase returned nil, and a user who quit the app was told their record had been published |
| **The closed check removed from `enter`** | `a Publish started after Close was accepted. It would run against a torn-down DHT server and write into counters nobody will read again` | `TestWorkStartedAfterCloseIsRefusedRatherThanQueued` |

**Two defects in the test itself, both caught by its own red proofs coming back green.**
The first version released the held publish immediately after starting `Close`, so the publish
finished on its own within microseconds and the assertion was true whether or not `Close` waited —
**both** red proofs passed. The second asserted the join by timing, which measures the opposite of
the property: a *correct* `Close` cancels, so it returns fast. The discriminating instrument is the
release timer — hold the publish for 3 s and require `Close` to return in less.

**And a declared limit.** With cancellation prompt, `inFlight.Wait()` is not independently
red-provable: removing it does not reliably fail anything, because the publish returns within
microseconds of being cancelled either way. That assertion is a regression guard on a structural
invariant, and it says so in the test rather than implying a red it never produced.

## v1.117.30 — P05.S04 T08 + T15, the server gains a ceremony identity

| Defect reintroduced | What it said | Check that fired |
| --- | --- | --- |
| **The hop taken as a constant instead of derived from the roster** | `party 1 with peer 2 derived hop 0, want 1 — the two ends of one hop must agree without negotiating` | `TestAnArmedSessionDerivesItsHopFromTheRoster`, on a THREE-party fixture: a two-party ceremony has exactly one hop and cannot distinguish a derived number from a constant |
| **A corrupt invitation treated as an absent one** | `a corrupt invitation reported "this session has no ceremony identity"; it must be refused, not treated as absent` | the same test. Silently dropping it arms a session the user believes is part of a ceremony and which is not |
| **A pin derived from a range variable over a record's addresses** *(the fixture this row cited was never in the tree; it is now `l1-wire-derived-pin`, see below)* | `zz_l1fixture.go: redProofWireDerivedPin sets candidate.Fingerprint from wire-derived data. L1: nothing learned from the network may influence WHICH peer is accepted` | `TestNothingWireDerivedReachesAPin`, widened at this slice |

**The vacuous fix, demonstrated rather than argued.** With the guard's vocabulary widened to
`ceremony.`/`rendezvous.` but its propagation left matching only `*ast.AssignStmt`, the planted
fixture above **passes**. A trickle-in racer consuming `rec.Addrs` is range-shaped by construction,
so the substring widening alone would have policed nothing while reading as coverage. Both halves
were changed together, and the two runs — caught, then not caught — are what says so.

## v1.117.31 — the hop rule was a chain and D22 is a hub

| Defect reintroduced | What it said | Check that fired |
| --- | --- | --- |
| **The chain rule restored — any two roster-adjacent parties share a hop** | `alice and bob were given hop <nil>; under a convener hub they never connect to each other, so a shared hop key between them is a key for a session that does not exist` | `TestTheHopNumberComesFromTheSignedRoster`, and `TestAnArmedSessionDerivesItsHopFromTheRoster` on the server side |

**This one was mine, shipped at v1.117.28 and corrected here.** `Party`'s doc says "the order of
the roster IS the signing order", and I inferred a chain from it: `roster[i]` to `roster[i+1]`,
with Alice handing to Bob. D22 says otherwise in its first sentence — *"the convener writes the
record, prepares the document, **dials each party in roster order**, and delivers the finished
document at the end. Every hop is exactly today's two-party session."* Signing order is not
connection topology.

What the wrong rule would have produced: a shared hop key between two parties who never connect,
and each of them arming for a peer D22's own TRIPWIRE argument says they never accept. It passed
its own tests because those tests asserted the rule I had written rather than the one the plan
states — the shape a grill is supposed to catch and did not, because the deepdive and the grill
both read `Party`'s doc comment and neither opened D22.

## v1.117.34 — P05.S04 T09, the DHT and the armed listener on one socket

| Defect reintroduced | What it said | Check that fired |
| --- | --- | --- |
| **The listener binds its own socket again** | `the listener answers on 127.0.0.1:15193 and the DHT is on 127.0.0.1:12804 — two sockets, so any mapping the probe measures belongs to one the session does not use, which is the whole of caveat 7` | `TestTheDHTAndTheArmedListenerShareOneSocket`, asserted on the SOCKET — one address reached two ways, plus a real UDP datagram to it, because comparing two strings would pass against an endpoint that never bound anything |
| **The shared listener closes the endpoint it does not own** | the shared socket stopped accepting datagrams after its listener closed | `TestASharedEndpointSurvivesItsListener` |
| **Teardown reversed — the socket before the DHT** | **`panic: use of closed network connection`** | `TestTheCeremonyTeardownOrderDoesNotPanicTheProcess`. This is the hazard the deepdive predicted and it is now driven: the mux closes, the DHT's read returns `net.ErrClosed`, `serveUntilClosed` sees an error with its `closed` flag unset and calls `panic(err)` on a goroutine nothing of ours is on. Process death, at shutdown, on the path a user reaches by pressing Cancel |
| **A listener constructor added in a file the population floor does not read** | `read 2 non-test files in internal/p2p — the glob is not seeing the package` | `TestEveryTransportIsInTheTable`, which used to read `transport.go` and `quic.go` BY NAME. `QUICListenOn` is in a third file, so the guard could not see the very thing it exists to count — its own defect class, happening to it. It now discovers the package, and carries an exemption list whose entries must name a reason and must still exist |

## v1.117.35 — P05.S04 T11-T14, T16, T18: the DHT becomes a candidate source

| Defect reintroduced | What it said | Check that fired |
| --- | --- | --- |
| **The self-test's zero-margin expiry (`now + 5 minutes`)** | `a published record claims 5m0s of life against a floor of 6m30s (45s publish + 5m0s race + 45s fetch). It expires while the peer is still reading it, and the peer sees a counterparty who never published` | `TestAPublishedRecordOutlivesThePeersRace`. The plan names that value as the anti-pattern; this asserts the arithmetic rather than the number, and also asserts the reader-side ceiling, because a record every peer refuses is worse than none |
| **Hop scoping removed** | `a candidate belonging to a LATER hop was admitted to this race. It would fail the pin — and it would be dialled first, which is the thing criterion 19 forbids` | `TestARaceNeverDialsAnotherHopsCandidate`, with both controls: a candidate for THIS hop must pass, and LAN/typed candidates — which belong to no hop — must not be dropped, since they are the two tiers D8 says survive when the DHT does not |
| **`Source` dropped from either candidate producer** | `discover.go:149 builds a dialable candidate without a Source` / `lan.go:286 …` | `TestEveryCandidateProducerNamesItsSource`, after its own false positive was fixed — see below |

**A guard of mine fired on correct code, and the fix is the interesting part.**
`TestEveryCandidateProducerNamesItsSource` treated every *elided* composite literal as a candidate,
copying `l1_test.go`'s `candidateLit`. That shortcut is right there and wrong here:
`candidateLit` keys on `Fingerprint`, which a non-candidate struct does not have, so its false
positives cost nothing — this one keys on `Addr`, which `ceremony.Endpoint` also has. So
`[]ceremony.Endpoint{{Addr: …}}` in `publishCandidates` was reported as a candidate missing its
source. It now resolves an elided literal from its parent (`[]candidate` / `map[K]candidate`)
instead of assuming, and both red proofs above confirm it still catches the real omission in both
literal shapes.

**Declared limit — T13b is NOT verified end to end**, and a pending item says so. Hermetically the
bootstrap finds no nodes and returns early, so the publish never happens *for the wrong reason* and
no test can tell the deferral from the failure. The discriminating observation needs a routing
table: `PublishAttempts == 0` after a LAN-answered arm and `== 1` after an unanswered one.

## v1.117.37 — the P05 sweep, batch 1: three clocks, one identity, one panic

| Defect reintroduced | What it said | Check that fired |
| --- | --- | --- |
| **`setPending` checks only that a listener exists, not that it is THIS one** | `a goroutine belonging to the CANCELLED session parked its consent request on the session that replaced it. The user is shown a document from the connection they just cancelled, attributed to the peer they have just armed for.` | `TestAStaleGoroutineCannotParkConsentOnTheSessionThatReplacedIt`. `setPending` was the one `session` mutator without the identity check its four siblings carry — and it is the one that decides what the user is shown and consents to |
| **The ceremony deadline compared against `now` alone** | `a hop was allowed to start with 3m0s left on a ceremony that gives every hop 6m0s. It is not expired — which is why comparing against \`now\` alone passes it — but it cannot finish` | `TestAHopDoesNotStartAfterTheCeremonyCanOutliveIt`. D16's clock 3 nests inside clock 2: the record must outlive one whole exchange budget, not merely be unexpired at the instant the hop starts |
| **`InstallOCRFonts` without `fault.Catch`** | `InstallOCRFonts PANICKED on an unwritable config directory rather than returning an error … CHILD-PANIC pdfcpu: config problem: mkdir …: permission denied` | `TestAnUnwritableConfigDirDoesNotCrashStartup`, driven in a **subprocess** because a panic on the parent's goroutine ends the test binary. pdfcpu panics where its API documents an error; `server.New` logs an error and continues, and a panic walks straight past that |

**Not red-proved, and why.** The `udpmux` ownership fixes in `internal/rendezvous` — three
double-closes and one leaked mux — sit behind `NIB_LIVE_DHT` or are resource hygiene with no
observable in-process. `Mux.Close` is `sync.Once`-guarded (`internal/udpmux/mux.go`), so the
double-close was harmless *today*; it is fixed because a test that closes a socket it does not own
is asserting an ownership rule the opposite way round. The leak is real on a live-network run and
cannot be reproduced hermetically, because the bootstrap it hangs off returns early with no nodes.

## v1.117.38 — the P05 sweep, batch 2: one protocol, one refusal vocabulary, one refusal that published

| Defect reintroduced | What it said | Check that fired |
| --- | --- | --- |
| **A listener carrying its own copy of the termination protocol** | `wsListener presents as a Listener but does not embed listenerCore — it is carrying its own copy of the termination protocol, which is the duplication ADR-009 forbids and which every behavioural test in this suite would pass on the day it was written` | `TestBothListenersRunOneTerminationProtocol`, driven by adding a **third** listener the way a third listener actually gets added: by copying. This is the case a two-entry behavioural table cannot see |
| **A listener shadowing one of the core's methods** | `quicListener declares Close itself, shadowing listenerCore's. A shadowed method is a second copy of the rule: it can drift from the core's without one test changing, because the tests call it through the interface either way.` | the same guard, second direction |
| **A co-signature decline writing nothing to the wire** | `initiator got receive co-signed document: EOF; want ErrCoSignDeclined` — the pending entry's own reported text, reproduced | `TestSessionReceiverDeclines`, **strengthened**: it asserted only `err != nil`, which is exactly why it passed against the defect it was written for. And `TestARefusalTellsThePeerWHICHRefusalItWas`, over both transports |
| **A timeout collapsed back into a decline** | `the SENDING side saw co-signing declined; want nobody answered the consent request in time` (and the transfer twin) | `TestARefusalTellsThePeerWHICHRefusalItWas`, 4 refusals × 2 transports. The wire byte and the sentinel go through one door, so the two ends cannot disagree |
| **A consent gate returning a bare `(false, nil)` on timeout** | `session.go:456 — a consent gate's timeout branch returns no TimedOut sentinel` | `TestEveryConsentGateNamesATimeoutAsATimeout`. A **source** guard, because the branch takes five minutes to reach — which is why none of the three gates was ever driven and two shipped wrong. It discovers the gates rather than listing them, and fails if it finds fewer than three |
| **The sequence-ceiling refusal returning `bep44.Put{}`** | `the refusal returned the EMPTY item (target da39a3ee5e6b4b0d3255bfef95601890afd80709 = sha1 of the empty string)` … `the refusal targets da39a3ee… ; want our own mutable target 2a102f27…` | `TestARefusedPublishEmitsNothingThatAnybodyStores`, asking the question with bep44's own `Check` and `CheckIncoming` rather than restating their rules |

**Three measurements, because reading was not enough.**
`getput.Put` uses the callback's return **unconditionally** (`exts/getput/getput.go:154`) and fans it
out (`:155-168`); `Server.Put` writes to the LOCAL store on its first line (`server.go:1081`) *before*
the context is consulted, so cancelling cannot stop that half; and a throwaway probe confirmed
`bep44.Put{}.Target()` is `da39a3ee…` — sha1 of the empty string — and that `bep44.Check` **accepts**
it. The finding was true in every particular including the one I had doubted.

**A test caught a gap in my own reasoning, on its first run.** `CheckIncoming` returns nil for equal
seq when the VALUES are equal ("the node SHOULD reset its timeout counter"), so a fixture that reused
our own value as the stored record reported the fix as broken. Reaching the ceiling requires an
in-roster holder to have taken the key, which means *their* record is stored — the fixture now uses
one, and the equal-value branch is written down as a stated limit rather than left to be rediscovered.

**And a cut that swallowed a declaration.** Extracting the protocol removed
`const maxConcurrentHandshakes` along with the methods around it; the compiler caught it, but the
check that belongs in the record is the one run afterwards — a diff of every top-level declaration
before against after, with each removal accounted for in the new file.

## v1.117.39 — six of this sweep's rows made replayable, and one existing row found broken

`test/redproofs/` goes from 9 rows to 15. The six added are this sweep's own defects, recorded
the day they were caught — which is the property the older prose rows do not have, since a
patch for a defect fixed at v1.105 does not apply to a tree at v1.117.

- `stale-consent-on-new-session` · `hop-starts-inside-its-own-budget` (v1.117.37)
- `cosign-decline-arrives-as-eof` · `timeout-reported-as-decline` · `consent-gate-returns-bare-nil`
  · `refusal-publishes-the-empty-item` (v1.117.38)

**And replaying the whole set found `zone-bypasses-reserved` broken.** Recorded during P05.S04,
it had its patch **reversed** — it applied the *fix* rather than the defect — so it had never
been a valid row and could never have re-proved anything. It failed as `STALE`, which is the
right kind of loud, but the diagnosis in that message ("the code moved under it") was wrong:
the code had not moved, the row was born backwards. Re-recorded and replaying green.

**The lesson is about the floor, and it is written into `verify_test.go` now.** The count in
that guard was satisfied by this row for as long as the file existed, because a count over
`*.sh` cannot tell a row from a file. Only `./build/redproof.sh <name>` can, and running all
fifteen is a minutes-long job that belongs in a sweep rather than in `go test` — so the gap is
recorded there rather than papered over. **A sweep that adds rows should replay the whole set,
not just its own**: this one was found by the run that had no reason to expect anything.

## v1.117.40 — a Go-side reader scan, and the three things it found

`test/jsdom/published.test.mjs` scans server→client fields for one with no reader. There was no
Go-side equivalent, so `udpmux.Stats`, `p2p.Channel` and `rendezvous.Stats` were invisible to
every scan in the tree. `observables_test.go` is that equivalent, and it paid on its first run.

| Defect reintroduced | What it said | Check that fired |
| --- | --- | --- |
| **A published observable with no reader** | `udpmux.Stats.RoutedLongHeader … is published and no named reader mentions it` | `TestEveryPublishedObservableHasANamedReader`, by deleting the `nib rendezvous` block that now prints them — the item's own headline example |
| **A NEW published shape nobody enters in the table** | `udpmux.Health (internal/udpmux/zzred.go, 2 field(s)) is published by an exported function and is in neither the table nor the exclusions` | the same guard's **completeness** half, driven by adding a third shape the way one actually gets added. This is what makes it close a CLASS: a walk over an inventory cannot see a shape nobody entered |
| **A read replaced by PROSE naming the same field** | `p2p.SignerAttestation.OneProceeding … no named reader mentions it` | the same guard, with the real read swapped for a comment mentioning it. The hole that bit `published.test.mjs` twice; this one is born without it |
| **The verifier summarising away a disagreement** | `two signatures naming DIFFERENT ceremonies were summarised as a co-signed document with no qualification — the verifier describing a proceeding that did not happen` | `test/jsdom/oneproceeding.test.mjs` |
| **`!oneProceeding` read naively** | `an ordinary two-party co-sign was accused of not being one proceeding` | the same file's FIRST case, which exists for exactly this — an ordinary co-sign carries no record, so its `oneProceeding` is false too, and a two-state reading slanders every plain co-signed document |
| **The one-family join note removed** | `the summary does not say "IPv6-ONLY"` | `TestTheSummarySaysWhenOnlyONEFamilyJoined` |
| **Its `Joined6 > 0` guard dropped** | `the summary says "ONLY", which is not true of this host` | the same test's nothing-joined case |
| **The per-family counting removed at the source** | `Open joined 2 interface(s) and counted 0 joins in either family (v4=0 v6=0)` | `TestTheJoinCountsAreCountedBySocketOpen` — see the vacuous green below, which is why this test exists at all |

**The find that mattered most.** `p2p.SignerAttestation.OneProceeding` was computed in Go,
serialized to the client as `oneProceeding`, and rendered **nowhere**. Its own doc states the
harm verbatim: *"A verifier that said only 'co-signed' about such a document would be describing
a proceeding that did not happen."* That is what the panel did — `✓ Mutually co-signed` fires on
`matched`, which is per-pair and says nothing about whether both signers agreed to the same
proceeding. Now rendered, in three states rather than two.

**A vacuous green, caught by my own probe, in the work of this same commit.** The first
per-family test drove `printSummary` with a hand-built `discovery.Stats` — so deleting BOTH
`s.joined4++`/`s.joined6++` lines at the source left the whole suite green. It tested the printer
and not the counter: this repo's own P03 lesson, *"a guard tested a predicate and not that
anything called it"*, happening to the guard written for it. Closed by
`TestTheJoinCountsAreCountedBySocketOpen`, which opens a real socket.

**And a limit declared rather than discovered.** That test cannot catch ONE family's counter
being dropped alone — probed, and green. This is correct: on a host where the IPv4 join genuinely
fails, `v4=0, v6=n` is the true reading, and a test that failed on it would be asserting the
machine's network rather than the package's code. Telling those apart needs a host known to
differ — which is exactly why `nib discover` on real Windows is what settles the original
question, and why the counter had to exist first.

**The table was re-derived, not remembered.** A first draft of the reader table was written from
memory and named four wrong files. A scan that reports a false orphan is worse than no scan —
people learn to ignore it — so the table is evidence like everything else here.

## v1.117.41 — an exclusion test became a structural guard, and gained a row

`Party.Name` was deleted (D21, Dan's call: **A**). The field was JSON-serialized, written by
nobody and read by nobody, and D21's *"an invitation whose name and fingerprint disagree must be
refused"* could not happen because `MatchesRecord` compares Fingerprint and Signs and never Name.

| Defect reintroduced | What it said | Check that fired |
| --- | --- | --- |
| **`Name string` added back to `ceremony.Party`** *(replayable: `roster-entry-carries-a-name`)* | `Party.Name is published but is not in rosterPreimage and carries no reason. A field outside the commitment can differ between the copy the signers read and the copy a verifier reads while both hash the same` | `TestEveryPartyFieldIsInTheCommitment` (tier 1), and independently `TestEveryPublishedObservableHasANamedReader`, which is what found the field in the first place |
| **`"Fingerprint"` misspelled in the guard's own `inPreimage` set** | `inPreimage names "Fingerprnt" and Party has no such field. Either the field was renamed and this guard is now covering nothing, or it left the preimage` | the same test's inverse loop — recorded because it is the **stimulus assertion**: the first loop walks the struct's fields, so a Party with no fields would satisfy it vacuously, and only the inverse loop can tell "nothing to complain about" from "nothing to look at" |

**The interesting part is what the old test could not see.** `TestTheNameIsNotInTheCommitment`
asserted that *one named field* stayed out of the commitment, and it was green for three phases
while that field's real defect — that nothing read it, so D21's refusal was unimplementable —
sat directly under it. An exclusion test proves an exclusion; it says nothing about the next
field somebody adds, and nothing at all about whether the field should exist. The replacement
walks every field of `Party` and demands each be in `rosterPreimage` or carry a written reason,
so the general case goes red in the same commit that adds it.

**Unrepresentable beat checked, and that is why the plan clause moved rather than being met.**
D21's evidence path was "construct a disagreeing invitation and observe the refusal"; with no
name on a roster entry there is nothing to disagree, so the clause is now discharged
structurally and P07 inherits the obligation to derive a display name at the point of display.
The amendment is written at the criterion in `PLAN-signing-ceremony.md`, not left implicit.

## v1.117.42 — the assertion that could not fail, and the list that had three copies

Pre-S05 remediation. Both defects were found while reconstructing P05.S04's missing seam-inventory
section, which is the argument for reconstructing it.

| Defect reintroduced | What it said | Check that fired |
| --- | --- | --- |
| **`udpmux.route`'s final arm sends everything to the QUIC view** *(replayable: `shared-socket-not-demultiplexed`)* | `a KRPC-shaped datagram sent to 127.0.0.1:40987 did not reach the DHT view (RoutedToDHT 0 -> 0); the DHT is not being served by this socket` | `TestTheDHTAndTheArmedListenerShareOneSocket`, **rewritten** — the defect leaves every address in the test equal, so the assertion it replaced could not see it |
| **`dropReport` reverted to its two-entry source list** | `the failure sentence is "… dropped 8 over the cap (2 from the address you typed, 3 from the local network) …" and never names "the meeting point", which dropped candidates in this race` | `TestTheGlobalCapBindsWhenEverySourceIsFull` — a new test, because the global cap had never been driven at all |
| **A fourth `candidateSource` declared but not listed** | `candidateSource(3) names itself "a relay", so a source exists that allCandidateSources() does not list` | `TestEveryCandidateSourceIsNamedAndRouted`, first half |
| **A second `[]candidateSource` literal, in `dropReport`** | `a []candidateSource list is built at [discover.go:allCandidateSources lan.go:dropReport]; it must be built only in allCandidateSources (ADR-009)` | the same test's second half — the routing check, not an agreement check |

**The first row is the one worth carrying, and it is a vacuous green that shipped as ledgered
evidence.** P05.S04 discharged caveat 7's probe-and-session half with
`ln.Addr().String() != cer.end.LocalAddr().String()` and recorded it in the plan as *"asserted on
the socket"*. Both sides are `e.mux.LocalAddr()` on the same `*udpmux.Mux` — `quicListener.Addr()`
is `l.mux.LocalAddr()` (`internal/p2p/quic.go:237`), `SharedEndpoint.LocalAddr()` is
`e.mux.LocalAddr()` (`internal/p2p/endpoint.go:68`), and both reach `m.pc.LocalAddr()`
(`internal/udpmux/mux.go:202`). **It compared a value with itself**, for any bind string, in any
address family. The UDP probe beside it proved the socket was *reachable*, never that it was
*shared* — and the test's own comment said the probe was there to stop exactly this, so the
awareness was present and aimed one step short.

**Two rows added at P08.S05d (2026-09-01, v1.117.317).**
`received-name-collides-within-a-second` reintroduces `/pending 342` — a received document's name
built from a peer label and a one-second timestamp alone, so two documents from one peer inside a
second share a path and `atomicfile.WriteDurable` renames the second over the first, after the
sender has been told `ackOK`. It is the rare row whose original was **measured** rather than
argued, and the probe output is in its spec. Recording it also produced a "did not compile" run —
deleting the digest left `crypto/sha256` unused — which is the outcome `redproof.sh` names
separately from a real red; the patch keeps the digest computed and drops it from the name.
`unattended-gates-escape-their-door` puts the delivery leg's auto-confirming `Verifier` in front of
an interactive arm. That is the gate `runVerification`'s nil check exists to make impossible, made
legitimate by scope alone — so the scope is the entire safety argument and it is asserted by
routing, not by comparing the sites anyone remembered to list.

**A row added at P08.S05c (2026-09-01, v1.117.314): `verify-busy-spends-the-arm`.**
`sv.saw.mark()` as `ConfirmVerification`'s first statement, ahead of the `setVerify` that discovers
the seat is taken — so a gate refused with `errVerifyBusy` reported its arm spent for a check it
never displayed. It earns a row because the defect moves in the direction that *looks* defensive:
marking early reads as caution, and the whole property is that it is not. **`errVerifyBusy` had no
test of any kind before this slice** — the incumbent-wins rule shipped with its reasoning in a doc
comment and nothing exercising it, which is why one line could sit in front of it unnoticed.

**Two rows were RE-SITED at P08.S05f (2026-09-01, v1.117.313), and neither defect changed.**
`bootstrap-has-a-second-door` inserted its second `Bootstrap` caller inside `startArmedRendezvous`,
which that slice deleted as dead; it now inserts into `publishCandidates`, one of the two DHT verbs
the door actually serves — a stronger site, because the old one was in a function nothing could
execute. `shared-socket-not-demultiplexed` keeps its patch (it is in `internal/udpmux`, untouched)
but its PROVE test had its setup repointed from the deleted `openRendezvous` to
`setupSharedEndpoint` + `QUICListenOn`; re-proved after the change, because a test that dies at a
setup fatal stops printing the EXPECT token and the row silently stops meaning anything.

**No red proof is recorded for the vacuity itself, deliberately.** A vacuous assertion cannot be
proved vacuous by making it fail; it is proved by reading the three lines above, and that is the
honest form of the claim. What IS red-proved is the replacement: the defect that breaks sharing
while leaving every address equal.

**The second and third rows are one defect at two altitudes.** `sourceDHT` was added by S04 to the
enum and to nothing else, so the drop counter written for it (`lan.go`) was rendered by a reporter
whose list did not include it. `raceFailure`'s own doc says *"a split nobody reports is the same as
no split"* and names D6 — the meeting point is the one source an attacker supplies, so the
unreportable source was the one that matters. The stale test list had additionally written down,
in a comment, that S04 would make the global cap reachable and that someone should come back for
it; the prediction was correct, the branch that would have logged it never ran, and the global
figure had never been driven by anything.

## v1.117.43 — P05.S05 T01–T05: two things asked for nothing, and one published half of itself

| Defect reintroduced | What it said | Check that fired |
| --- | --- | --- |
| **`DefaultWant` dropped from our `dht.ServerConfig`** *(replayable: `dht-asks-for-no-node-family`)* | `NewDefaultServerConfig sets DefaultWant and our ServerConfig does not, with no reason recorded. It is therefore at its ZERO value, which is a decision and not an absence` | `TestOurServerConfigAnswersEveryFieldTheLibraryDefaults` — written against the CLASS, not this instance: it reflects over what the library defaults and demands our literal set each field or name it with a reason |
| **`publishableEndpoints` reverted to v4-first-else-v6, one endpoint** *(replayable: `publish-drops-the-second-family`)* | `got 1 endpoint(s) [203.0.113.5:34154], want 2 … a dual-stack host that publishes one address cannot be dialled on the other` | `TestAPublishedRecordCarriesBothFamilies`, whose single-family rows are the controls that stop a function returning both entries unconditionally from passing |
| **`net.ListenPacket("udp4", …)` in place of `"udp"`** | `a socket bound "0.0.0.0:0" did not receive a datagram sent to [::1]:16576. It is not dual-stack on this platform, so D8's tier 2 cannot work here — and every other tier would keep passing` | `TestTheWildcardBindIsDualStack` |
| **The same, inside `NewSharedEndpoint`** | `a datagram sent over IPv6 to the shared endpoint at [::1]:42971 never reached the DHT view (RoutedToDHT 0)` | `TestASharedEndpointBoundWildcardAnswersOverIPv6` — a separate row because `net.ListenPacket` being dual-stack says nothing about what the ceremony's own door does with the string |
| **`localWildcardFor`'s v6 branch returning the v4 wildcard** | `localWildcardFor("2606:4700:4700::1111") = "0.0.0.0:0", want "[::]:0" — a v6 peer dialled from a v4 socket cannot be reached at all` | `TestLocalWildcardForPicksTheRemotesFamily`, this function's first test of any kind |

**The first row is the one to carry, because it is the second instance of its class and the
guard is now written against the class.** `dht.NewServer` fills in a few fields for a
caller-supplied config; everything else lives in `NewDefaultServerConfig`, which caveat 7 forbids
because it opens its own socket. Whatever that function sets and ours does not is left at its
**zero value** — and a zero value is a decision, not an absence:

- `Exp` unset meant "expire everything immediately", so our own published record was deleted the
  first time anyone read it, including us.
- `DefaultWant` unset meant `find_node` asked for nothing, a responder answered with the query
  source's family, and every seed we ship is IPv4 — so the routing table could never learn an
  IPv6 node and D8's tier 2 was unreachable.

Both were found by chasing a symptom, and neither was findable by reading our config, because
what is wrong with it is what is **absent**. The guard discovers the population by reflection
rather than listing it, so a field added by a dependency upgrade is in scope the day it appears.

**No red proof is recorded for the `Exp` case** even though it is the same guard: removing `Exp`
leaves `time` unused and the build fails first. The compiler catching it is a real answer, and
recording a contrived variant to claim two rows would be worse than saying this.

**Measurement is what separated a live seed from a dead one, again.** `dht.transmissionbt.com`
publishes an AAAA record and answers on IPv4; nothing answers on that v6 address — silent 3 of 3.
`dht.libtorrent.org`'s v6 answers 3 of 3 and returns `nodes6`. Shipping both on the strength of
the DNS lookup would have repeated this file's own recorded mistake one family over: *"silent" is
a verdict about an address, and an address is a host AND a port AND a family.*


## v1.117.44–.46 — sweep 10: five defects, and two blockers that were not there

| Defect reintroduced | What it said | Check that fired |
| --- | --- | --- |
| **`dialable` stops refusing a zone** *(replayable: `zone-reaches-the-dialer`)* | `Target accepts 2606:4700:4700::1111%eth0:5000 — a candidate record naming it is sealed, published, opened, and its zone is handed to the kernel by the racer` | `TestAZoneOnAGlobalAddressNeverReachesTheDialer`, whose SETUP asserts the BARE address passes both predicates — without it a predicate refusing everything would pass |
| **`raceKey` returns the raw address string** *(replayable: `race-key-is-a-raw-string`)* | `one IPv6 endpoint spelled three ways was tried 3 time(s), want 1 — the race key is a raw string, so a peer publishing one address in three spellings burns three of maxRaceCandidates (16)` | `TestOneIPv6EndpointIsOneRaceCandidateHoweverItIsSpelled`, read from the race's own tried-count end to end rather than from the key function |
| **The notices preamble names a licence class** *(replayable: `notices-preamble-names-a-class`)* | `the preamble names the license class "BSD". An enumeration here is a claim about the WHOLE set that goes stale silently as dependencies change — it already did, and the correction sat 3,100 lines below it` | `TestTheNoticesPreambleNamesNoLicenseClass`, reading the committed artifact rather than the generator |
| **`fieldsOf` stops resolving embeds** *(replayable: `embed-fields-invisible-to-the-scan`)* | `attestationView publishes 1 field(s) (pinned) — oneProceeding is missing, so embedded types are contributing nothing and every field this shape reaches through p2p.SignerAttestation is unchecked` | `published.test.mjs`'s SETUP assertion, which names a field reachable ONLY through the embed |
| **The `key-missing` recovery check moves back below the key-mode block** *(replayable: `retry-never-retries`)* | `Retry reported an error instead of re-checking the status — the key-missing screen offers no key choice, so any validation of one is validating a control the user cannot see` (actual: `No key selected.`) | `unlock.test.mjs` at tier 3, asserting the error line BEFORE the overlay so the defect is named in 0.6 s rather than arriving as a 30 s Playwright timeout |

**Two carried blockers were checked and neither existed.** M2's unlock click-through had been
filed as needing "a SECOND nib with an enrolled vault" because `uirepro.sh` starts one server and
enrols it before the browser opens. True of reaching the overlay by *navigating*; false of
reaching it at all — `applyStatus` branches on `st.state` and nothing else, so `/api/status` is
the overlay's whole input and tier 3 has intercepted routes since v1.109.19. And
`localWildcardFor`'s "no test of any kind" had been true when written and was fixed by v1.117.43
three commits earlier. **This is the third time a stated blocker turned out to be a hypothesis
nobody had run** (M3's stamp placement, the cmap vendoring, and now these), and each time the
item had been carried for weeks on it.

**One premise was overturned by the test written to settle it, and that is a success.** The
discovery read path was filed as "IPv4-shaped": it joins both groups, announces to both, and
reads through `s.p4.ReadFrom` alone. It hears both — `p4` and `p6` are views of one dual-stack
fd. **No row is recorded for a defect that was not there**; what is recorded is
`TestTheReadPathHearsBothFamilies`, which drives a v6 unicast at the port over `::1` and goes red
against a v4-only bind. Loopback unicast rather than the v6 group, deliberately: a multicast
probe cannot separate "the reader is v4-shaped" from "this host swallows v6 multicast", and that
ambiguity is exactly how a v4-shaped path would have stayed invisible under a green suite.

**The zone fix rests on a weaker claim than the item asked for, and says so.** The open question
was whether an attacker-chosen zone steers our source interface. Measured on Linux/Go:
`[2606:4700:4700::1111%lo]:443`, `%docker0`, `%99` and `%nosuchif` all dial, from the ordinary
global source, in the same ~20 ms as the bare form — the kernel ignores `sin6_scope_id` for a
global-scope destination, and `Dialer.Control` confirms the zone reaches the syscall regardless.
So no steering was demonstrated, and nothing is claimed for Windows or macOS. The refusal stands
on the ground that does not need the measurement: a zone means something only on a link-local
address, link-local is refused whatever its zone says, so a zone reaching that predicate is
attacker-chosen bytes the program can never act on.

## v1.117.48–.49 — sweep 11: two standing doubts, and a guard that could not see

Item 13 was a list of standing doubts; item 15's one open finding was gated on an artifact.
Neither closed. What follows is what the grills produced anyway — three tier-1 rows, one of
them for a vacuous green in a guard written the same hour.

| Defect reintroduced | What it said | Check that fired |
| --- | --- | --- |
| **The whole-file residue scanner resumes at `endstream`'s `stream`** *(replayable: `redaction-residue-whole-file`)* | `the whole-file scan did NOT find a flate stream carrying the secret that was appended to the file` | `TestTheTwoResidueChecksDifferAndTheDifferenceIsThePoint` — the discriminating test, which appends a stream no page references so the page-content and whole-file checks must diverge |
| **`describeSignFailure` bypassed, runSign returns the raw library error** *(replayable: `tsa-failure-unactionable`)* | `the failure is not identifiable as a timestamp problem: sign: failed to replace signature: … get timestamp: non success response (0)` | `TestAnUnreachableTimestampAuthoritySaysSoAndSignsNothing`, whose setup asserts the same call signs fine with no TSA so the failure is attributable to the timestamp |
| **`addedAfterVerdict` drops the error** *(replayable: `added-after-fails-closed`)* | `unreadable is a warning, not clean: addedAfterVerdict(false, malformed PDF) = false, want true` | `TestAddedAfterFailsClosed`, binding the combine directly because the error path is unreachable through Verify (both calls share dpdf on the same bytes) |

**The residue-scanner row is the one to carry.** Its defect and its guard were written in the
same hour, and the guard's first form was itself vacuous — the whole-file check passed because
the fixture's secret lives in the first stream (the control found it) and the desync made
everything after invisible (the redacted file scanned clean for the wrong reason). Two agreeing
green results, `TestRedactLeavesNoResidualContent` and its new whole-file sibling; the test that
separated them is the one that asserts they *differ*, by planting a stream no page references.
The lesson is the same one this file keeps recording: a green shared by two checks is not two
confirmations if one of them cannot see.

**Two measurements that produced no row, deliberately.** Item 15's headline finding — the
added-after-signing warning removable without touching the Valid verdict — is still gated on an
artifact, and the gate was re-measured this sweep rather than taken on faith: a classic-table
increment on nib's xref-stream output makes dpdf report `unsigned` (loud), and the
trailing-content check is a length comparison against the signature's frozen ByteRange, so it
catches content appended by any increment format. Building the artifact needs a non-signature
xref-stream revision, and nothing in the repo emits one onto a signed document. And item 13's
doubt 5 fallback — "sign with a local date when the TSA is offline" — was refused rather than
built, because `TimeBacking` makes a verifier report the difference and a silent downgrade is a
false statement about the document; the row above is for the *warning*, which is the half worth
keeping.

## v1.117.64 — P05.S09a: the QUIC stream that opened by the wrong end

The glare deadlock, caught before S09's coordinator could reach it. Pre-S09a the dialer was
always the ceremony initiator, so "the dialer opens the QUIC stream" was welded into both dial
and accept paths and never tested with the roles apart. Symmetric racing (S09) breaks that
coincidence: the party that dialled may be the receiver, the party that accepted the initiator.
`HandshakedConn.Promote` opens the stream by the ROLE, so this row reintroduces the weld — the
receiver opening, the initiator accepting — and shows it hangs.

| Defect reintroduced | What it said | Check that fired |
| --- | --- | --- |
| **`Promote` opens the stream by dial direction, not role** *(replayable: `stream-follows-the-dialer`)* | `deadlocked: the role-opposite-dialer session never completed — stream direction is following the dialer, not the role` | `TestQUICStreamOpensByRoleNotByDialer`, which promotes the dial side as receiver and the accept side as initiator and bounds the wait, so the weld manifests as a named failure in seconds rather than a hung suite |

**Why a timeout is the assertion here, not a value.** A deadlock has no wrong value to print —
both goroutines simply never report. The harness bounds the wait and fails by name, which is the
tier-2 lesson applied to tier 1: a hang is worse than a fail, because a hang tells you nothing
and blocks every test behind it. The EXPECT token is the deadlock message, so a row that went
red merely by failing to compile would not satisfy it.

## v1.117.67 — P05.S09 T05: the consent gate a dialer could not reach

The C4 hole the grill called the biggest. The consent gate's stale-goroutine guard keyed on the
armed listener (`se.ln == ln`) — which the manual/LAN receiver always has. A symmetric-racing hop
whose RECEIVE role wins by DIALING holds no listener, so its consent could never park and the
exchange hung at the gate. The re-anchor keys on the ceremony there; the row reintroduces the
listener-only guard and shows the dial-won consent hang.

| Defect reintroduced | What it said | Check that fired |
| --- | --- | --- |
| **`consentAnchor.current` drops its ceremony branch** *(replayable: `consent-anchors-on-the-listener-only`)* | `a ceremony-anchored consent could not park while its ceremony is armed — the dial-won receive role would hang here` | `TestACeremonyHopConsentAnchorsOnTheCeremonyNotAListener`, whose SETUP also asserts an anchor naming a DIFFERENT ceremony is refused while armed, so the pass is the identity matching and not the guard being absent |

**Why the guard could not simply be dropped.** `setVerify` was freed of its listener check for the
same dial-has-no-listener reason, but `setPending` could not follow it to nothing: the check stops
a cancelled session's goroutine from parking ITS consent as the session that replaced it. The
anchor keeps that — after a cancel-and-rearm `se.cer` is the new ceremony, so a stale hop's anchor
is refused — while making the gate reachable without a listener. The test proves both halves: the
dial-won park succeeds, and the stale park is still refused.

## v1.117.80 — P05.S09b T02: the announcer that would not stop

Criterion 14 over the extended arm. S09b lets a ceremony arm LISTEN for up to D33's 30-day
MaxCeremonyLife so a multi-party signer is not disarmed before the baton arrives. The 500ms LAN
announce ticker, left coupled to that, would emit a never-rotating six-word name for 30 days —
~5.2M multicast datagrams per ceremony. The announce window is capped independently; this row
reintroduces the coupling.

| Defect reintroduced | What it said | Check that fired |
| --- | --- | --- |
| **The announce loop drops its window case** *(replayable: `announcer-ignores-its-window`)* | `the announcer emitted N more datagram(s) AFTER its 700ms window — the cap does not fire, so an arm extended to ceremony scope emits at full rate for the whole deadline` | `TestTheAnnouncerStopsAtItsWindow`, driven over a short SIMULATED window; its `Sent==0` case is a SKIP not a pass (multicast fails in restricted sandboxes), so the "it stopped" assertion is never vacuous |

**Why the window is a parameter, not a const.** The single-builder guard (`TestAnnouncingHasExactlyOneDoor`,
ADR-009) keeps the `lanAnnouncer` literal in `startAnnouncing`, so the cap could not move to a testable
helper. Making the window a parameter that the product always fills with `lanAnnounceWindow` and the test
fills with 700ms drives the cap in 2.6s instead of 30 days, without a mutable product knob.

## v1.117.88 — P05.S10: the re-delivery that re-signed

D18/D24's "re-deliver, do not re-sign", made idempotent. A channel lost after the receiver signed
but before the initiator read the result must re-deliver the CACHED signature on a reconnect, not
sign again — because `Contribute` is non-deterministic (random ECDSA nonce + a wall-clock
timestamp), a re-sign would produce a second, different block (D25 wrong).

| Defect reintroduced | What it said | Check that fired |
| --- | --- | --- |
| **`coSignExchange` drops the cache lookup and always re-signs** *(replayable: `re-delivery-re-signs`)* | `Confirm was asked again on a re-delivery (now 2) — the user was re-prompted to consent to a document they already signed` | `TestReDeliveryIsIdempotent`, which co-signs one inbound twice and asserts the SAME bytes with consent asked ONCE; the sibling `TestReDelivererMissRunsAFreshExchange` is the control — a DISTINCT document is a miss that signs fresh, so the pass is idempotency, not a cache that returns everything |

**Why the key is the inbound hash, not the hop.** Keying on the hop alone would hand a reconnect
carrying a different document the previous document's signature (the grill's stale-signature risk).
Keyed on `sha256(inbound)`, a distinct document is a clean miss (proved by the control test), and the
lookup sits AFTER the peer-binding validation so only the pinned peer with the original document can
pull the cached bytes.

## v1.117.98 — P05.S11: the CGNAT user told to port-forward

D19's cause-3 advice, and D9's pin on it. A machine behind an endpoint-dependent NAT with no routable
port-map gets "a direct connection isn't possible" — but the FIX it names must not be a port-forward
when the user cannot perform one: a carrier-grade-NAT subscriber has no router to open a port on, and a
double-NAT's router answers with its own private address. Both need a VPN; telling them to port-forward
is the futile advice D9 exists to forbid.

| Defect reintroduced | What it said | Check that fired |
| --- | --- | --- |
| **cause 3 offers a port-forward unconditionally** *(replayable: `cgnat-told-to-port-forward`)* | `detail mentions port-forward = true, want false — D9 forbids offering a port-forward to a carrier/double-NAT user` | `TestD19ClassifierTable`, whose cause-3 rows split on the CGNAT (`sharedSpace`) and double-NAT (`mapUnroutable`) signals and assert the advice diverges; the controllable-NAT row is the control that keeps the pass from being "never offer a port-forward" |

**Why the tri-state matters.** `SharedAddressSpace` (100.64/10) catches carrier NAT, but a double-NAT's
reflexive DHT address can be a normal public one while the router's own external is RFC-1918 — caught only
by `mapUnroutable`, the port-map tier's "answered but I could not publish it" signal that S11 retained
(the connect path used to close and drop that mapper without recording it).

## v1.117.103 — P05.S12: the ladder was unreachable behind a client-side refusal

D9 makes the traversal ladder the default path, but `sessionInit()` refused to POST a live
co-sign without a typed address, so the shipped LAN tier — and, with an invitation, the DHT —
could never be reached from the UI. The manual address was not undemoted; it was the only path
a user had, for a peer Nib could have found by browsing the local link.

| Defect reintroduced | What it said | Check that fired |
| --- | --- | --- |
| **sessionInit refuses an empty address** *(replayable: `empty-address-refused`)* | `the co-sign never reached the quote — the empty-address refusal is back` | `test/jsdom/ladderdefault.test.mjs` test 1, which drives the real dialog with a blank address and asserts `/api/cosign/quote` is reached (the refusal sat before it) and `/api/session/initiate` is POSTed with a blank `address` field. The typed-address test beside it is the control — it must keep reaching the initiate — so the fix cannot be "always refuse" or "never send the address" |

**Why tier 2 and not the server.** S12 adds no server code: an empty address resolves through the
pre-existing `peerAddresses("")` -> `findPeerOnLAN` LAN browse (P03, covered by
`discover_test.go`'s fakeBrowser suite), and the DHT tier is wired only to the arm/serve flow, not
to `sessionInit`. The only new code is the client no longer refusing, so the client is where the
proof lives. The visual half — the address field collapsed behind the Advanced disclosure — is
`test/ui/ladderdefault.test.mjs` at tier 3, a layout fact jsdom cannot see.

## v1.117.105 — P05 close: the MITM signal reported as a network error

`handleSessionInitiate` routed `p2p.ErrVerificationDeclined` (the "four words don't match"
man-in-the-middle verdict) and `ErrVerificationTimedOut` to `writeConnectDiagnosis`, which renders
a 502 "could not connect" and may pick an unrelated D19 cause. verify.go's own doc forbids exactly
this: the verdict "must never be reported as a network error … 'could not connect' invites a retry,
which is the worst possible advice when someone is sitting between you."

| Defect reintroduced | What it said | Check that fired |
| --- | --- | --- |
| **the initiate side does not lift the verification sentinels** *(replayable: `mitm-reported-as-network-error`)* | `handleSessionInitiate does not lift p2p.ErrVerificationDeclined — a words-don't-match MITM verdict would fall through to the network-error diagnosis` | `TestInitiateLiftsTheMITMSignalBeforeTheNetworkDiagnosis`, a source-scan (consenttimeout_test.go's shape) asserting both sentinels are lifted before `writeConnectDiagnosis`; its stimulus assertions refuse to pass if the handler or either branch vanished |

## v1.117.106 — P05 close: a refused peer record reads as "hasn't started"

`classifyD19` cause 1 was keyed solely on `!peerSeen`, and `peerSeen` is set only when the gate
*admits* a candidate. A peer who published but whose record was refused (stale / wrong-ceremony /
forged) or carried no address collapsed into "hasn't started their ceremony yet" — a confident-false
statement about someone who has started.

| Defect reintroduced | What it said | Check that fired |
| --- | --- | --- |
| **remove the causePeerRecordUnusable branch** *(replayable: `d19-refused-record-reads-as-not-started`)* | `record refused -> unusable, not 'not started': cause = 2, want 5` | `TestD19ClassifierTable`, whose refused/empty rows assert the new cause, with a discriminator (a refused record does not hijack the diagnosis once any candidate was admitted), an ordering row (no DHT still beats a refused record), and a distinctness check that the refused and empty sub-messages differ |

## v1.117.110 — P05 graduation pass: diagnose() read the racy gate on the live path

The v1.117.106 D19 cause-1 fix read `c.gate.Stats()` directly in `diagnose()`. On the live arm-side
path (`sessionStatus.status -> diagnose`) that runs while `feedCandidates` is still writing the gate
via `gate.Accept`, so the read is a data race — the exact reason the S11 note said diagnose() reads
"only atomic/mutex-guarded signals" and never the gate. The graduation pass over the seam inventory
caught it; `-race` had not, because no test drives `status()` concurrently with an active feed.

| Defect reintroduced | What it said | Check that fired |
| --- | --- | --- |
| **diagnose() reads c.gate.Stats() instead of the atomic snapshot** *(replayable: `diagnose-races-the-gate`)* | `diagnose() no longer reads the c.recordRefused atomic — has the snapshot been removed?` | `TestDiagnoseReadsGuardedSignalsNotTheGate`, a source guard asserting diagnose() calls no method on `c.gate` and reads the `recordRefused`/`recordEmpty` atoms instead — chosen over a `-race` behavioural test because reproducing the race needs a live feed driven concurrently with a status poll |

## v1.117.113–.118 — sweep 12: seven backlog items, and the guard that could see one package of twelve

Eight rows, one per defect this sweep fixed. Seven are tier 1; the eighth is tier 2 and is a guard
about a guard, which is the shape this ledger keeps needing.

The through-line: **five of the seven items were not the item as written.** Two proposed fixes were
overturned outright (a lease clamp that cannot bind what the router holds; an IPv6-CGNAT detector no
reflexive probe can build), one had to be relocated before it was correct, and two turned out wider
than reported — the goroutine guard's move found seven sites beyond the one that prompted it, and
the reader scan's coincidence hole had nine fields in it rather than two, two of which were real
published-and-unread fields, now deleted.

| Defect reintroduced | What it said | Check that fired |
| --- | --- | --- |
| **a goroutine outside `internal/server`, recovered SECOND** *(replayable: `recover-registered-second`)* | `internal/ots/ots.go:78 launches a goroutine whose FIRST statement is not defer safe.Recover(...)` | `TestEveryDetachedGoroutineIsRecovered`, moved to the repo root, discovering its population by walking the tree rather than by a package list |
| **an unrecovered `time.AfterFunc` callback** *(replayable: `afterfunc-callback-unrecovered`)* | `internal/server/session.go:592 hands time.AfterFunc a callback whose first statement is not defer safe.Recover(...)` | the same guard, through the door the package-local one never walked — AfterFunc runs its callback on its own goroutine |
| **`DroppedOverCap` counts fetches, not addresses** *(replayable: `over-cap-counts-fetches`)* | `DroppedOverCap = 30 after the same 18 addresses were served three times, want 10 — the counter is reporting the fetch cadence, not the peer` | `TestAnOverCapAddressIsCountedOncePerAddressNotOncePerFetch`, whose two setup assertions hold against fixed AND unfixed code, so a red cannot be a dead stimulus |
| **D19 cause 3 promises a port-forward** *(replayable: `cause3-promises-a-port-forward`)* | `cause 3 promises a port-forward will work to a user it has no evidence controls a router` | `TestCause3NeverPromisesWhatItCannotKnow`, with a negative control (a router that DID answer with carrier space still gets no port-forward advice at all) so uniform conditioning cannot pass it |
| **the refresh is scheduled after the lease expires** *(replayable: `refresh-outlives-the-lease`)* | `granted 10s, refresh scheduled at 15s — the mapping expires 5s before we renew it` | `TestTheRefreshCadenceNeverOutlivesTheLease`, which also carries a standing row asserting a LONG grant is not clamped — the item's own proposed fix, encoded so it fails if anyone adds it |
| **a requested lease posing as a granted one** *(replayable: `requested-lease-posing-as-granted`)* | `a NAT-PMP MAP reply carries the granted lifetime, so it must report as observed` | `TestAGrantedLeaseIsDistinguishedFromARequestedOne`. The UPnP half has no behavioural row and says so: the defect there is an ABSENT DISTINCTION, not a wrong value |
| **every version skew reads as "newer"** *(replayable: `version-skew-always-says-newer`)* | `an older format: ParseInvitation("nib-invite-v0:…") = this invitation was made by a newer version of Nib` | `TestAVersionSkewNamesTheDirection`, which also asserts the two sentences differ — one sentinel wrapped twice satisfies `errors.Is` and still says the wrong thing |
| **a shape-level deferral that does not defer** *(replayable: `deferral-silently-ignored`)* | `only 0 fields are deferred … the mechanism is not being applied`, and beneath it the laundering itself: `cause` and `summary` return unread while `detail` sails through on `renderScanReport`'s line | `published.test.mjs`'s deferral stimulus. **No row exists for `decryptResponse.ok`, and that is the honest half:** re-adding the field passes the scan, because its collision is with `Response.ok`, the fetch API's own property, and that is exactly the blind spot the item names |

**And the ledger's own count had stopped describing it.** `verify_test.go`'s floor last moved at
v1.117.50, to 27, while the set grew to 36 — so the tree that shipped P05 would have tolerated
losing nine rows silently, the same erosion the floor exists to prevent. The count is bounded on
both sides now: it still fails when a row disappears, and it fails when the set outgrows it, naming
the number to write. The original reasoning — "adding a row should not need a test edit" — is
refuted by measurement: an edit that does not have to happen is an edit that does not happen.

## v1.117.120–.124 — sweep 13: the port-map seam, the arm that went dark, and a law driven at eleven doors

Six rows. The through-line repeats sweep 12's: **the entry was a hypothesis, and the grill's job was
to check it.** One item's stated blocker was false (257's "unimplementable at the current seam"),
one item's stated expectation was measured wrong (259 surfaces zero automatic reds), one item's
central premise was overturned and the item deferred (258), and one deepdive's own recommendation
did not survive contact (256's "one governor" would have republished every five seconds).

| Defect reintroduced | What it said | Check that fired |
| --- | --- | --- |
| **a request the router received leaves nothing to delete** *(replayable: `send-time-handle-not-recorded`)* | `Map recorded no handle for a request the router RECEIVED — the mapping it may have created is undeletable and lives to lease expiry` | `TestARequestThatReachedTheRouterLeavesADeletableHandle`, whose fixture asserts the gateway RECEIVED the request before claiming anything, so the row cannot pass over an absence |
| **the PCP delete names a mapping that never existed** *(replayable: `pcp-delete-mints-a-fresh-nonce`)* | `the delete carries nonce X but the mapping was created with Y — PCP names a mapping by its nonce, so this delete names one that never existed and is a no-op` | `TestThePCPDeleteCarriesTheMappingsOwnNonce`. It was a no-op on the SUCCESS path too, and nothing could see it: the mock echoes the nonce without validating, and the one delete test drives NAT-PMP |
| **a UPnP delete removes whatever holds that port** *(replayable: `upnp-delete-not-identity-checked`)* | `delete of a mapping that is not ours returned <nil>` — and the test prints the DeletePortMapping envelope it should never have sent | `TestAUPnPDeleteRefusesAMappingThatIsNotOurs`, three rows including an IGD that answers nothing, plus a POSITIVE arm: a check that refuses everything passes every refusal row while breaking the teardown D15 requires |
| **a thirty-day arm polls the DHT at race rates** *(replayable: `rendezvous-cadence-is-flat`)* | `rendezvousInterval just past the race = 5s, want more than 5s — nothing steps down` | `TestTheRendezvousCadenceStepsDownButNeverStops`, which also asserts the cadence never becomes a CAP — lan.go's announce cap is only safe while it delegates late discovery to this loop |
| **the arm publishes once and is un-findable for 29 days** *(replayable: `arm-publishes-once-and-goes-dark`)* | `the arm published 1 times over ~12 periods — it publishes ONCE and the record then expires` | `TestAnArmRepublishesForAsLongAsItIsArmed`, driven with small periods because there is no fake clock in this package, plus a row asserting the D6 LAN-window suppression still holds |
| **a misaddressed mutation reaches whatever is active** *(replayable: `misaddressed-mutation-gets-the-active-doc`)* | `/api/pages addressed to an unknown document = 200, want 409` | `TestEveryMutatingRouteRefusesAMisaddressedDocument` — ADR-001's whole failure mode, driven at all **eleven** mutating routes where the Go side previously drove two |

**One row was refused twice before it counted, and both refusals were the harness working.** The
first attempt replayed against a HEAD that did not yet contain the test file, so `redproof.sh`
reported "with the defect applied, the check still PASSED" over `[no tests to run]` — a row for an
uncommitted check is a claim about a tree nobody has. The sweep-12 jsdom row hit the sibling case,
going red on a stimulus assertion rather than the one its `EXPECT` named.

**No row for the three body-first routes.** `/api/pages`, `/api/redact` and `/api/outline` parse
their whole multipart body and run the PDF operation before resolving the document — but the law
still holds, the commit is still refused, and there is no defect to reintroduce. It is a cost, filed
as its own item, not a failure.

## v1.117.126–.128 — sweep 14: three sections, and every premise in them was wrong

Four rows. The sweep took the `Low Hanging Fruit`, `Finding` and `Instrument Missing` sections of
the newly-partitioned backlog, and **not one item was what its entry said it was**: one was a route
short, one would have shipped a defect as its fix, and one had been parked for months behind an
artifact it never needed.

| Defect reintroduced | What it said | Check that fired |
| --- | --- | --- |
| **a handler parses 128 MiB before checking the pin** *(replayable: `mutating-route-parses-before-it-pins`)* | `handlePages (pages.go:41) reads the body at line 42 and does not resolve the addressed document until line 177` | `TestAHandlerThatCommitsResolvesBeforeItReadsTheBody`, which checks the ORDERING because no status code can: the resolve at the commit still refuses, so the law holds and only the cost is visible |
| **the refresh loop never resolves the lease it could not read** *(replayable: `upnp-lease-never-observed`)* | `ObserveLease was called 0 times — the refresh loop never resolves a lease the obtain could not report, so LifetimeObserved is decoration` | `TestAnUnobservedLeaseIsResolvedAndReported`, which asserts the LOG as well as the value: /pending 258's gate reads "actually OBSERVED", and an observation nobody can read is not evidence |
| **an observed-permanent lease written as a zero lifetime** *(replayable: `permanent-lease-written-as-zero`)* | `a NewLeaseDuration of 0 is a mapping that never expires — D15's crash floor is unbounded there` | `TestObserveLease`, whose fourth row exists to fail if anyone "simplifies" by writing the 0 into `LifetimeSec`, which `refreshAfter` reads as its opposite |
| **an XFDF checkbox read with the CSV cell reader** *(replayable: `xfdf-checkbox-read-as-csv-cell`)* | `a checkbox given its export-value name came back unchecked — the XFDF path is reading a form-data name with the CSV cell reader` | `TestAnXFDFCheckboxValueIsAnExportNameNotABoolean`. The row carries a second field on purpose — see below |

**One row had to be rebuilt to show the right failure.** The XFDF row's first draft put only the
checkbox in the document, and an unchecked result changes nothing, so pdfcpu refused the whole fill
with "no form fields affected" — the defect surfaced as an **error**. Its real shape is silent: any
real export carries other fields, they apply, the fill reports success, and the box is quietly
wrong. The row now carries a text field and a setup assertion that it landed, so what it
demonstrates is a wrong ANSWER rather than a refused operation. A red proof that fires for the
wrong reason is the same failure as a green one that never met the case.

**One defect has no row, and the absence is the finding.** `/api/assemble` was missing from the
pinning inventory — it commits via `commitBarrier` and was excluded on the ground that it "never
reaches commitMutation". Removing it again breaks nothing: the jsdom guard checks that every listed
route is real, and nothing checks that every real one is listed. A hand-kept inventory's
completeness is exactly what a hand-kept inventory cannot guard, which is why the ordering guard
above walks the package instead.

## v1.117.130 — sweep 15: the item was deferred and its grill found three live defects

Three rows, none of them from the item that produced them. `/pending 262` asked whether Nib should
retry a refused UPnP mapping with a permanent lease; the answer is that D15 makes that Dan's call,
so the item is deferred. Reading the branch to answer it found three things shipping — **two of
them opened by this week's own commits.**

| Defect reintroduced | What it said | Check that fired |
| --- | --- | --- |
| **a UPnP refusal carried on a 200 read as success** *(replayable: `soap-fault-on-a-200-reads-as-success`)* | `an IGD that refused in the body while answering 200 was read as SUCCESS (err=<nil>) — Nib would publish a signed candidate naming a port that was never forwarded` | `TestAFaultOnA200IsStillARefusal`, whose two setup assertions establish that the POST was delivered and the device answered, so the row is about a response rather than a transport failure |
| **a failed obtain drops its delete handles** *(replayable: `failed-obtain-drops-its-delete-handles`)* | `ceremonynet.go:257 returns without closing the mapper` | `TestAFailedObtainStillClosesItsMapper` — a SOURCE scan, and it says so: the call site builds its own `portmap.Client` and cannot be driven with a fake |
| **a republish orphans the mapper it replaces** *(replayable: `replacing-the-mapper-orphans-the-old-one`)* | `replacing the stored mapper left the old one open (Unmap 0, want 1) — its refresh goroutine keeps running and its router mapping outlives the ceremony` | `TestReplacingTheStoredMapperClosesTheOldOne`, with a setup assertion that the first mapping was still live at the moment it was replaced |

**Two of the three are regressions from the four days before them, and that is the entry worth
re-reading.** v1.117.120 gave the mapper a send-time delete handle to close /pending 257's leak, and
in doing so made an existing early return into a leak of exactly the kind it was closing.
v1.117.123 turned a one-shot publish into a republish loop and made an existing overwrite into an
orphan. Neither was visible from the change that caused it: both needed a reader coming at the same
seam from a different question. **A fix that closes a leak at one door is the moment to walk the
other doors**, and the grill that found these was pointed at a third thing entirely.

## v1.117.133–.135 — sweep 16: the disposition pass's own backlog, worked

Six rows. This sweep's items were the ones filed by the disposition pass that followed sweep 15 —
findings earlier grills had produced and earlier sweeps had left in prose. **Every one of them was
a real defect, which is the argument for the pass**: nothing here was invented to justify a filing.

| Defect reintroduced | What it said | Check that fired |
| --- | --- | --- |
| **a refused router is told it stayed silent** *(replayable: `refused-router-told-it-was-silent`)* | `a router that ANSWERED and refused is told Nib got no answer from it — the one thing that is demonstrably untrue in this case` | `TestARefusedGatewayIsNotToldItStayedSilent`, with a carrier-NAT control so the fix cannot become a blanket "always offer a port-forward" |
| **the refusal flattened one level lower** *(replayable: `refusal-flattened-in-the-client`)* | `a gateway that ANSWERED and refused produced portmap: no mapping obtained — indistinguishable from no gateway at all` | `TestARefusalIsCarriedOutNotFlattened`. **This row exists because the test caught the first draft of its own fix**: carrying the refusal out of `mapWithSuggestion` leaves it flattened in `tryGatewayProtocols`, and the outer check then sees `ErrNoMapping` forever |
| **a definitive refusal records a delete handle** *(replayable: `refusal-records-a-delete-handle`)* | `a DEFINITIVE refusal recorded 1 delete handle(s) — a UPnP delete is keyed on the external port with no ownership check` | `TestMapViaUPnPLoop`, the first test of any kind over that orchestration — the rule had a comment and no coverage because the loop was untestable |
| **an XFDF in another encoding refused as invalid** *(replayable: `xfdf-encoding-refused-as-invalid`)* | `an XFDF declaring ISO-8859-1 was refused: invalid XFDF: xml: encoding "ISO-8859-1" declared but Decoder.CharsetReader is nil` | `TestAnXFDFInAnotherEncodingIsRead`, whose fixture is asserted NOT to be valid UTF-8 — or it would pass with the fix removed |
| **discovery spends the whole budget first** *(replayable: `ssdp-burns-the-whole-budget`)* | `the post-answer grace (2s) is not shorter than the discovery budget (2s), so an IGD that answers immediately still costs the full budget` | `TestTheUPnPBudgetLeavesRoomForTheCallsItHasToMake`. The wire timing is not driven and the test says so; the arithmetic is where the defect lives |
| **the arm republishes slower than its record lives** *(replayable: `arm-republishes-slower-than-its-record-lives`)* | `the arm republishes every 16m0s against a record that lives 8m0s — the record expires before it is replaced` | `TestAnArmedSideStaysFindableForItsWholeWindow` |

**One clause was deliberately NOT written, and the reason generalises.** The arm-side guard could
have asserted that the record's life covers the arm's window. That is unsatisfiable:
`MaxCandidateLife` is a reader-side ceiling every peer enforces, so no expiry can cover thirty days
and the assertion could only ever fail. A test that can only fail is the mirror image of one that
can only pass, and neither tells you anything — so the clause is about COVERAGE instead, which is
the property that is actually true and actually checkable.

## v1.117.139 — /pending 276: the page-compare hash, and the reduction under it

Two rows, both tier 3, both about `pageDHash`. The first is the defect the instrument was built to
find. The second is the one the instrument was *complicit in*, and it is the more interesting entry.

| Defect reintroduced | What it said | Check that fired |
| --- | --- | --- |
| **the gradient's sign decides the result** *(replayable: `compare-hash-gradient-sign-decides`)* | `the same page under a +10% illumination gradient is 0 bits away and under a -10% gradient 25, against a threshold of 12 — a scan of one page under an uneven lamp does not match itself` | `two sparse text pages…`, `an illumination gradient does not decide the result…` and the width guard, in `test/ui/compare-hash.test.mjs`. A strict `>` files all ~22 tied pairs of a mostly-paper page under "darker", so a brightening ramp moves nothing and a darkening one flips every one of them |
| **the reduction point-samples** *(replayable: `compare-hash-reduction-point-samples`)* | `a full page of text is 4 bits from blank paper against a threshold of 12 — a dropped or blank-fed sheet would align against a page of content` | `a blank sheet does not pair with a page that has content on it`, driven by `sparseField` — paper, a heading, eight lines |

**The second row is a lesson about instruments, not about hashes.** `pageDHash` reduced its render
with `ctx.drawImage(canvas, 0, 0, 9, 8)` and the comment on that line called it a box filter. It is
not one: Chromium's default smoothing on a ~150px-to-9px reduction effectively point-samples, so
the hash was reading 72 individual pixels and every property a perceptual hash exists for — noise,
texture, halftoning, a page of text averaging to grey — never reached it.

Four green tiers could not see it **because the test reduced the same way, with the same line.** The
copy agreed with the defect and confirmed it back to the product. Nothing in the assertion set could
have caught that, because agreement between two implementations of the same mistake looks exactly
like agreement between two implementations of the same rule. What found it was a probe that printed
the raw grid — nine flat columns coming back as the exact column greys with no blending at all.

The fix was structural as well as arithmetic: `gridMeans`, `dhashFromGrid` and `hamming` are now
exported and the instrument calls **the product's** versions, so the only thing left on the test's
side of the seam is the fixture and the page canvas.

**Two claims that measurement corrected, recorded because both were confident and wrong.** The grill
predicted "every content page pairs with a blank page"; against the nine-column fixture a blank was
never close, and the finding only reproduced against a realistic sparse page — where it was worse
than predicted. And the numbers that sized the fix came from a replica of the reduction rather than
the product; the replica box-filtered, the product did not, and every distance it predicted was
wrong. The replica said a text page was 23 bits from blank. The browser said 4.

## P07.S08 — the trust page describes a ceremony of N, and overflow stops being silent (v1.117.144)

Four rows, all tier 1. Two are about what the page *says*, two about whether anyone can *read* it.
The fourth is the interesting one: it is a row that **could not have been recorded the day before**.

| Defect reintroduced | What it said | Check that fired |
| --- | --- | --- |
| **one paragraph reverts to the two-party wording** *(replayable: `readme-still-says-two-people`)* | `the rendered readme still says "two people" — the page describes a ceremony of exactly two, and P07 makes it N` | `TestRenderedReadmeNoLongerSaysTwoParty`, against the RENDERED page rather than the Go constant |
| **`RenderReadme` stops refusing an oversized body** *(replayable: `readme-overflow-renders-silently`)* | `RenderReadme() error = <nil>, want ErrReadmeOverflow — a body past the page renders without complaint, which is the silent failure this door exists to stop` | `TestRenderReadmeRefusesAnOverflowingBody` |
| **the refusal is inert and the body grows into the blocks** *(replayable: `readme-body-overflows-the-block-stack`)* | `the last body baseline is 189 and the signature-block stack starts at 220: 40 lines is 31pt too many. The block appearance is an opaque fill, so it does not overlap the trust text, it erases it` | `TestReadmeBodyClearsTheAttestationStack` |
| **`#aboutMain` is deleted and the six claims survive in an HTML comment** *(replayable: `about-dialog-deleted-claims-survive`)* | `could not locate the About dialog's #aboutMain block in web/index.html — this scan would otherwise read the whole file and pass on a comment, so it fails rather than reporting a drift check it did not perform` | `TestAboutCopyContainsTrustClaims`, rewritten to read the dialog's TEXT |

**The fourth row is the fifth instance of a hole this file already records three times.** The guard
was `strings.Contains` over the whole of `web/index.html`, and it is the *sole* discharge of P07's
C08. Measured against exactly this mutation before the rewrite: with the dialog deleted outright and
the six claim strings left behind in one comment, the old form returned **true for all six**. The
vacuous-green table above records the same shape at `published.test.mjs` (second),
`TestNothingDecidesOnTheArrivalInterface` (third) and `TestResolutionLivesOutsideTheDiscoveryPackage`
(fourth), each time with the fix written one file away from the previous one.

**Two instruments were proposed for the overflow rows and both measured blind**, which is why the
guard reads a computed number rather than the artifact. `RenderReadme` computes a last baseline of
**−189** at 61 drawn lines, but pdfcpu **clamps** what it emits — a requested `y` of `−50` and of
`−5000` both land at **421.0**, A4's vertical centre — so 62 runs collapse to **49 distinct
baselines with 14 sharing one**. Reading the *rendered* position therefore saturates: forty overflow
lines and four hundred are indistinguishable. Reading the *extracted text* is blind too, because
every overflowing line is still in the content stream. And `PageCount` cannot move at all, because
the spec hardcodes `"pages": {"1": …}` — which is why `TestRenderReadmeOnePage` has never been able
to fail on this and now says so in its own doc comment.

**A third instrument was rejected for the text rows, for the opposite reason.** `digitorus/pdf`
returns one run **per glyph** with `W=0` and spaces expressed as positioning rather than glyphs, so a
naive join yields `"Aboutthisco-signeddocument"` and `contains("two people")` comes back **false
against the un-rewritten page**. Built that way, the load-bearing negative clause would have passed
*before* the work was done. The rows use `api.ExtractContent`, whose literal `(…) Tj` runs are joined
and whitespace-collapsed so a phrase spanning a wrap boundary still matches.


## L1 and L2 become replayable, and a patch stops being able to lie (v1.117.151)

Two rows, both tier 1, closing the two-thirds of the Stage-6 pin (verification pack V1) that
had never been replayable. The pin says **each** of L1, L2 and L3 ships a negative fixture
planting a violation of *that law specifically*, and each earns a row here. L3 is P07.S03's.
Before this, **zero of the 69 recorded rows drove an L1 or L2 guard** — their entries were
prose: proven red once, re-checked by nothing.

| Defect reintroduced | What it said | Check that fired |
| --- | --- | --- |
| **a non-test file lets wire data reach a pin** *(replayable: `l1-wire-derived-pin`)* | `zz_l1fixture.go: redProofWireDerivedPin sets candidate.Fingerprint from wire-derived data ([]byte(seen.From.String())). L1: nothing learned from the network may influence WHICH peer is accepted — the pin comes from the vault` | `TestNothingWireDerivedReachesAPin` |
| **`Initiate` stops calling `runVerification`** *(replayable: `l2-exchange-reached-unconfirmed`)* | `Initiate never asked the verifier — the path reaches the document exchange unconfirmed, which is exactly what L2 forbids` | `TestL2NoDocumentBytesCrossBeforeBothConfirmations`, on BOTH transports |

**The L1 row is the one the item was named for.** This file cited
`zz_l1fixture.go: redProofWireDerivedPin` as though it were a durable fixture and **no such
file was ever in the tree** — a throwaway recorded as a record, which is exactly what
`build/redproof.sh` exists to make impossible. The fixture now lives inside the patch, so it
is re-applied on every replay and cannot rot unnoticed. Note what its guard's shape forced:
`TestNothingWireDerivedReachesAPin` is an AST taint analysis over **non-test** files, so the
fixture has to be real Go in the package rather than a test helper — which is why the patch
adds a whole file, and why a throwaway was tempting in the first place.

**The L2 row proves which of its guard's two assertions is load-bearing.** The guard drives
four entry points with a declining verifier and asserts both that the call fails with
`ErrVerificationDeclined` **and** that the verifier was actually called. A path that never
asks fails too — for another reason — so it satisfies the first assertion while being the
precise defect L2 forbids. The planted defect fires the second one.

### And a patch can no longer carry more than the defect it names

`TestEveryRedProofPatchTouchesOneFile` is new, and it comes from a defect found in this
repo's own rows a version earlier: four P07.S08 patches had been generated with a bare
`git diff` while `test/redproofs/*.patch` are **themselves tracked**, so each regeneration
swept the previously-rewritten patches into the next. One reached 214 lines and six hunks for
a one-comment mutation — and **all four still replayed green**, because a patch carrying
extra hunks still applies and the `PROVE` command still fails for its own reason and prints
its own `EXPECT`.

**`build/redproof.sh` structurally cannot see this.** A red proof asserts that a defect makes
a check fail; it never asserts that the patch contains *only* the defect. The new guard is the
missing half, and it was free: all 71 patches already touch exactly one file, so the rule was
an invariant the set obeyed and nothing enforced.

## P07.S02 — the commitment stops folding what it claims to bind (v1.117.153)

Five rows, all tier 1, all replayable. The slice was re-scoped at its grill from "convene, behind
one door" to the half that is **irreversible** — everything inside a signed preimage or a hashed
structure — because three of these five are defects that could not be fixed after the first record
existed in the field.

**Two of the five are defects in GUARDS, not in features**, which is the shape this project keeps
finding: the check that would have caught the mistake was itself a claim.

| the defect, restored | what goes red | the check |
|---|---|---|
| **the embedded-files axis is dropped from `ContentDigest`** *(replayable: `ceremony-digest-blind-to-exhibits`)* | `an exhibit's contents changed under an unchanged filename and the digest did not move` | `TestContentDigestCoversAttachedExhibits`, three arms — contents, rename, removal — because "some mutation moves it" cannot stand in for three. Measured before building: a `Schedule-A.txt` reading `rent is 1000/mo` re-added as `rent is 100000/mo` left the digest byte-identical and `CheckDocument` nil. The exclusion was *argued* — "tamper-evidence for everything else is what the signatures are for" — and the argument fails in the pre-signature window, which is the **only** window this digest is checked in |
| **`Party.Capacity` is on the struct and out of `rosterPreimage`** *(replayable: `ceremony-capacity-outside-the-commitment`)* | `Party.Capacity varies (alpha vs beta) and RosterHash does NOT move, so the field is OUTSIDE the commitment` | `TestEveryPartyFieldIsInTheCommitment`, **rewritten from a claim into a measurement**. It compared a hand-maintained `inPreimage` map against `reflect.TypeOf(Party{})` and never against the preimage — measured on a pristine export, `Capacity` declared in the map ALONE ships green with `Director` and `Witness` hashing identically, and the guard's own failure message pointed the implementer at the map. It now drives the preimage per field, so there is nothing to silence |
| **`Verify` stops refusing a non-canonical record** *(replayable: `ceremony-record-not-canonical`)* | `…produced a record whose signature still verifies and Verify said <nil> — want ErrNotCanonical` | `TestAVerifiedRecordIsCanonical`, three axes (hex case, sub-second, non-UTC). **Each arm asserts the mutation does NOT break the signature before asserting the refusal** — otherwise the axis would already be committed and the row would be vacuous. `rosterPreimage` hex-decodes fingerprints, so the commitment is case-FOLDING: two byte-different rosters share one valid `ConvenerSig` |
| **`MatchesRecord` stops comparing the commitment** *(replayable: `one-invitation-many-records`)* | `the invitation for one ceremony ACCEPTED a second signed record with a different intent` | `TestOneInvitationMatchesExactlyOneRecord`. Measured: the per-field checks compare **nothing that varies between two records sharing a roster**, so a convener could run two chains under one ceremony id — one party carried a lease, another a deed of sale at a different price — and every check passed |
| **`CheckDocument` stops reading the record's digest version** *(replayable: `digest-version-bound-not-carried`)* | `a digest-rule skew reported <nil> — want ErrDigestVersion` | `TestADigestVersionSkewSaysSoRatherThanAccusing`, which asserts the **sentence** and not only the sentinel: both numbers present, and the tampering wording absent. `ContentDigestVersion` claimed in its own doc to prevent this and could not — bound INTO the digest, three occurrences in the tree, no reader anywhere. Binding a version inside a hash changes the number without giving any reader something to compare |

### The two guards that were claims, and how they now differ

`TestRosterHashCoversEveryAxis` names nine `Record` axes and says why each matters. That is worth
keeping — the reasons are the specification — but a hand list **cannot notice a tenth**, and
`Record.DigestVersion` was the tenth. `TestEveryRecordFieldIsInTheCommitment` now drives every field
with one named exclusion (`ConvenerSig`: a value cannot be inside the preimage it signs). Dropping
`DigestVersion` from the preimage turns the new guard red **while the nine-axis list stays green**,
which is the demonstration that the completeness half was needed.

Neither of those is recorded as a replayable row, deliberately: the `ceremony-capacity-outside-the-commitment`
patch already exercises the same mechanism one level down, and a second row proving the same class
would be a row that reads as coverage without adding any.

### And one the slice could not close

`TestARecordSurvivesIncrementalSignatures` was cited — in this file's ancestors, in `embed.go` and in
the plan — as discharging D20's hop-4 clause. It **signed invisibly**, so its final assertion was
unconditionally true. It now asserts the LIMIT: a *visible* signature moves the digest, so
`CheckDocument` fails from hop 2 on an honest ceremony. That is a measured boundary, not a fix — the
per-hop continuity mechanism is S05's and S06's, and the repair the plan had adopted (byte prefix
plus `AddedAfter == false`) was measured at this slice's grill to **pass** on a document whose first
page had been blacked out by the last signer.

## P07.S02a — convene exists, and three guards could not see it (v1.117.155)

Five rows, all tier 1, all replayable. **Four of the five restore a defect that the check written
for it could not see** — the slice's own finding, and the reason its ledger entry reads the way it
does: the code was mostly fine and the instruments were not. Every one was found by mutating the
subject and re-running; none by reading.

| the defect, restored | what goes red | the check |
|---|---|---|
| **`handleSave` loses its `ceremonyFreeze` call** *(replayable: `save-route-escapes-the-ceremony-freeze`)* | `/api/save (handleSave) is a MUTATING route and reaches neither a commit door nor ceremonyFreeze` | `TestEveryMutatingRouteReachesTheCeremonyFreeze`, reading tier 2's `MUTATING` inventory from its own file rather than restating it. `handleSave` reaches neither commit door — it writes the file itself — so a freeze hung on `commitMutation` and `commitBarrier` covered eleven routes and not this one. **The patch deletes the call and KEEPS the comment**: the first draft substring-matched the raw body, and `handleSave` names both doors only inside the prose explaining that it reaches neither, so the guard read that sentence as proof of coverage and this exact deletion left the suite green. If the comment-stripping is ever removed, this row goes green and says so |
| **`PrepareCeremonyDocument` becomes `return pdf, nil`** *(replayable: `convened-document-never-built`)* | `the convened document has 1 pages, want 4 — pages are allocated from the SIGNING count` | `TestTheConvenedDocumentIsBUILT`. Measured with this exact mutation applied: **every convene test in the package stayed green.** `CheckDocument` proves hash-then-embed and cannot see whether a page was ever appended — and `Convene`'s doc comment refuses dependency injection *precisely* on the ground that "the guard would not go green with the readme never appended". It did. The assertion is written against `SignaturePagesFor(signing)` rather than a literal, with a setup assertion that the signer count and the roster length **straddle a page boundary**: an earlier fixture used seven signers of eight parties, which both round to two pages, so the assertion was correct and could not fire |
| **`atomicfile.WriteDurable` becomes `os.WriteFile`** *(replayable: `durable-write-writes-through`)* | `the file kept inode N, so the bytes were written THROUGH the existing file` | `TestWriteDurableREPLACESTheFileRatherThanWritingThrough`. The package's other two tests — content-at-the-requested-mode, and overwrites-leaving-no-temporary — **both stay green against this patch**, because `os.WriteFile` satisfies every observation they make, and the one test that could tell them apart hit its own `t.Skip`. It now discriminates on **inode** and on the mode of a file pre-created at 0644, which is the state `os.WriteFile` preserves and a rename does not — `os.WriteFile` honours perm only on CREATE |
| **a second `Contents` decoder, receiver named `out`, in `builtin.go`** *(replayable: `contents-decoded-outside-the-door`)* | `these decode a Contents payload outside decodeContents: [builtin.go:recentFromPayload]` | `TestEveryContentsDecodeGoesThroughTheDoor` — ADR-009 asserted on the **routing**, not on each site's behaviour. The first draft counted the literal `json.Unmarshal(plain, &c)` in `vault.go` alone, so a decoder spelled differently evaded it and `builtin.go` was never read at all. A payload read outside the door means a vault written by a newer Nib is accepted and the next ordinary `AddRecent` — opening any PDF — rewrites it without the keys this build does not know. For a ceremony invitation secret that is the only copy |
| **`WriteMirror` writes `record.json` before `document.pdf`** *(replayable: `mirror-record-written-first`)* | `the record was written FIRST, so a torn write leaves (record, no document)` | `TestTheRecordIsTheCommitPoint`, the one guard here that was right as first written. A torn write in that order leaves a state **byte-identical** to the deliberately document-less mirror `WriteMirror` itself creates, so a resuming party cannot tell "no document yet" from "the document was lost". The stimulus is a `record.json` path made unwritable by a directory in its place, **asserted unwritable before the write is attempted** — without that, "document.pdf exists" says nothing about ordering |

### What these four have in common

Not one of them was a wrong computation. In each case a property was stated correctly — in a doc
comment, in a plan bullet, in the guard's own name — and the check written to hold it could be
satisfied without the property being true: by prose that named the call, by a hash that could not
observe the axis, by a `t.Skip`, by a spelling. **The tell is the same every time: patch the
subject to do nothing at all and see whether anything goes red.** Four of these five did not, and
none of the four would have been found by reading.

## The replay set replayed WHOLE, for the first time — eight of eighty-one were invalid (v1.117.156)

Recording P07.S02a's five rows meant running the harness anyway, so the whole set was replayed
rather than just the new rows. **Eight of eighty-one did not re-prove.** The set had never been
replayed end to end: `/pending 275` — "the red-proof archaeology" — was carried open for five
sweeps, attempted by none, and declined.

Every one of the eight had been counted as coverage the whole time. `verify_test.go`'s floor sees
a row that **disappears** and cannot see one that no longer re-proves; it says so in its own
comment, and the eight are that comment measured.

| row | how it failed | why |
|---|---|---|
| `added-after-fails-closed` | **the check still PASSED** | the sharp one — see below |
| `roster-entry-carries-a-name` | red, but **not for its own reason** | P07.S02 rewrote the guard "from a claim into a measurement", so its sentence changed; the `EXPECT` token still named the old one. The row's defect still fires — a stale token cannot be told from a deleted check by an exit status, which is the whole reason the token exists |
| `cgnat-told-to-port-forward` | STALE | `/pending 263` added the `mapRefused` branch above the one the patch cut |
| `deferral-silently-ignored` | STALE | `published.test.mjs` grew; the hunk moved ~100 lines |
| `failed-obtain-drops-its-delete-handles` | STALE | the `markMapRefused` block landed directly above the deleted `mapper.close()` |
| `hop-starts-inside-its-own-budget` | STALE | P07.S02a re-pointed the check at `ceremonyHopBudget()`, rewriting the lines the patch replaced |
| `stale-consent-on-new-session` | STALE | P05.S09 generalised `se.ln == ln` into a `consentAnchor`, so the guard the patch removes is spelled differently |
| `upnp-delete-not-identity-checked` | STALE | the two inline checks moved into `verifiedUPnPEntry` when a second caller needed them (ADR-009) |

All eight are re-recorded against `62424f9` and replay green.

### The one that was not bookkeeping

`added-after-fails-closed` applied cleanly and **the check passed with the defect in place**. That
is the harness's second failure mode, and it means the ledger's claim about that row was false.

The row cuts `addedAfterVerdict`'s `return trailing || err != nil` down to `return trailing` — the
fail-closed rule, which is a *named function* precisely because "an inline expression is one
careless refactor away from `a` alone, and nothing would fail". `TestAddedAfterFailsClosed` drives
it as a five-row table. **Not one of those rows could see the error arm any more.** `/pending 270`
later added a disagreement rule ahead of it —

```go
if librarySawSigners && !sawSignature { return true }
```

— and every table row carrying `err != nil` also satisfies *that*, or satisfies `trailing` alone.
So the arm the test is named for had been uncovered since 270 landed, and the table still read as
though it were tested. The missing case is the one where both enumerations agree a signature is
present and nothing trailing was found, so the error is the only thing left that can warn; it is
now the table's third row, and with it the defect goes red again.

**The verdict could have gone quiet exactly the way the guard did** — which is the sentence the
guard's own doc comment already contained, about the verdict, and not about itself.

### What this changes

`./build/redproof.sh --all` now replays the set as one command, reports every failure rather than
the first, and exits non-zero if any row did not re-prove. The gap was never that nobody knew —
`verify_test.go` names it, and so does the v1.117.39 entry above ("a sweep that adds rows should
replay the whole set, not just its own"). The gap was that doing it meant hand-rolling a loop, and
**an audit that has to be improvised each time is one that happens once every eighty-one rows.**

## P07.S02b — the invitation is consumed, and D22 turned out to be a hub (v1.117.157)

Seven rows, all tier 1, all replayable. **Two restore defects this slice INTRODUCED reachability
for** — `AddCeremonyPeer` and `PruneCeremonyPeers` had no production caller until now, so their
bugs had never been reachable — and one restores a state the product was actually in.

| the defect, restored | what goes red | the check |
|---|---|---|
| **a ceremony pin renames a pin the user made** *(replayable: `ceremony-pin-renames-a-user-pin`)* | `the ceremony renamed the user's peer to "Party 2"` | `TestACeremonyPinNeverRenamesAPinTheUserMade`. `addPinned` assigned the label unconditionally while `AddCeremonyPeer`'s doc said an existing pin is never downgraded — true of `Ceremony`, false of `Label`. Accepting an invitation would have overwritten the user's private nickname with whatever the convener published: **a stranger editing this machine's peer list by inviting it.** Two controls guard the fix from becoming "labels are frozen" — the user renaming their own peer still works, and an unnamed pin may be filled in |
| **ending one ceremony unpins a peer another still needs** *(replayable: `one-ceremony-prune-unpins-another`)* | `ending ceremony A removed a peer ceremony B still needs` | `TestTwoCeremoniesCanShareAPin`. Measured on a probe before the fix: two `AddCeremonyPeer` calls for one fingerprint left `ceremony-A`, and pruning A removed the pin outright, so B's next arm refused an unpinned peer the user never unpinned. `PinnedPeer.Ceremony` became `Ceremonies`, a set; `contentsVersion` 1 → 2, with no migration because there was no population. The same counterparty across two matters is the **ordinary** case for this product's user |
| **the consent gate never reconciles the document** *(replayable: `consent-gate-skips-the-arrival-check`)* | `Confirm does not contain "checkArrival("` | `TestTheConsentGateRoutesThroughTheArrivalCheck` — ADR-009, on the ROUTING, with `//` stripped. It asserts three orderings, not just the call: before `setPending` (C17's clause IS the order — an unreconciled document read and accepted is one already signed), before `saw.mark()`, and that a decline prunes while a consent **timeout** does not |
| **a TCP ceremony hop runs with no ceremony** *(replayable: `ceremony-session-loses-its-ceremony`)* | `serveOneSession reads ` + "`anchor.cer`" | `TestEveryCeremonySessionGetsItsCeremony`. The anchor carries a ceremony only on the QUIC coordinator path, so a TCP hop got nil — and `ReDeliverer`'s contract says nil means *"the manual/LAN path, which has no ceremony hop to key on"*, false there. **Structural, and the gap is measured rather than assumed:** the re-delivery test that exists runs the QUIC path and stays GREEN against this patch. A tier-4 drive is `/pending 289` |
| **the arrival check accepts any document** *(replayable: `arrival-check-accepts-any-document`)* | `a document with no ceremony record at all was accepted` | `TestTheArrivalCheckRefusesADocumentTheInvitationDoesNotDescribe`, two arms that fail differently — no record at all, and a valid record for a DIFFERENT ceremony — with the control driven FIRST, because a gate that refuses everything satisfies both |
| **accepting an invitation pins nobody** *(replayable: `accept-pins-nobody`)* | `after accepting the invitation, arming still failed` | `TestAcceptingAnInvitationRemovesTheManualPin`, written as **arm fails → accept → arm succeeds** rather than as "a pin appeared in the vault". The refusal this closes is `handleSessionArm`'s, and a pin that does not satisfy the door it was made for satisfies nothing |
| **convening pins nobody** *(replayable: `convene-pins-nobody`)* | `is not pinned after convening` | `TestConveningPinsItsRosterAndKeepsTheSecretOutOfTheMirror` — **the first Go test of any kind over `POST /api/ceremony/convene`.** S02a live-verified that route with a scratchpad script that no longer exists, so between the two slices the product's only ceremony-creating surface was exercised by nothing committed |

### What is NOT recorded, and why

`CheckDocument`'s absence from the arrival gate has no row, because the defect it would restore is
one the code cannot be in: the clause was **refuted**, not implemented and then guarded. Probed on
the real receive path — the hop-1 receiver's copy carries one valid signature, its record extracts
and verifies, and `CheckDocument` answers *"these are not the same document"*, accusing an honest
convener. A row would encode the wrong lesson.

The `MatchesRecord` completeness guard has no row either. Its own mutation — cutting the
whole-`Party` comparison down to fingerprints — is one level down from
`ceremony-capacity-outside-the-commitment`, already recorded at P07.S02, and a second row proving
the same class reads as coverage without adding any.

## P07.S03 — L3 gets a door (v1.117.159–.160)

Six rows, all tier 1, all replayable. **L3's negative fixtures are its own** — not shared with L1
or L2 — which is the slice's own acceptance clause and the reason the L2 population floor was
*updated* rather than loosened when `Receive`'s signature changed.

| the defect, restored | what goes red | the check |
|---|---|---|
| **the prefix is no longer compared against the roster** *(replayable: `l3-admits-the-wrong-prefix`)* | `want the signatures on this document are not the ceremony's signing order` | `TestTheGateRefusesEachThingByName`. **The assertion is the SENTENCE, and that is the finding**: with the identity check off the gate still refuses — it counts signatures and computes a different "next" — so a row asking only "was it refused?" would stay green. What it *says* is *"it is not this party's turn"*, sending the user to wait for a turn that will never come |
| **the prefix is no longer required to be cross-bound** *(replayable: `l3-admits-an-uncrossbound-prefix`)* | `prefix not cross-bound: admitted` | same test, its own fixture: A signs accepting a stranger who never signs, B signs on top so A's cross-binding is due, and the identities ARE the roster prefix — asserted before anything is graded, so a red here cannot be about order. The LAST signature is exempt by construction, measured on a three-signature chain |
| **the document's verify state is no longer asked** *(replayable: `l3-admits-an-unreadable-signature`)* | `want ErrPrefixUnproven` | `TestAnInvalidPrefixSignatureIsRefused`. **A broken signature does not report itself invalid — it VANISHES.** Measured: a body tamper leaves `sign.Verify` reporting `unsigned` with zero signers, a corrupted `/Contents` leaves it `invalid`, also zero. `ReadAttestations` iterates `st.Signers`, so the per-signature `Valid` check cannot fire and an attestation-only gate reads an unreadable document as "nobody has signed yet" |
| **a substituted proceeding is admitted** *(replayable: `l3-admits-a-substituted-proceeding`)* | `substituted proceeding: admitted` | same test. Valid signatures, right order, right identities — committing to a ceremony that is not this one. Its reach is limited today and the limit is a separate assertion, not a comment: no production attestation carries a commitment yet, so `TestTheCommitmentCheckIsLimitedUntilS04` skips loudly the day one does |
| **the channel binding reads the FIRST signer** *(replayable: `channel-binding-reads-the-first-signer`)* | `The channel binding is reading the first attestation` | `TestTheChannelBindingReadsTheLastSigner`, at THREE signatures — at N=2 index 0 and index last are the same attestation and every other test in the package is green either way. Conditioning `len(ats) != 1` is what made this reachable: at hop k, `ats[0]` is the party who signed first, not the one on the other end of the connection |
| **the INITIATING side skips the gate** *(replayable: `initiating-side-skips-the-l3-gate`)* | `a party signed out of roster order through the initiating door` | `TestTheInitiatingSideIsGatedToo`. It asserts that **nothing was signed**, not merely that an error came back: `buildCoSigned` applies the local signature and a signature cannot be taken back off a document — the same reasoning the ceremony deadline check one caller up already states |

### The routing guard has no row, deliberately

`TestEveryContributionEntryPointReachesTheGate` was mutation-tested in both directions while it was
written — delete the call in `internal/p2p` and it names `coSignExchange`; delete it in
`internal/server` and it names `buildCoSigned`. It earns no row because the two rows above already
restore exactly those deletions and assert the *behaviour* that follows, which is strictly more
than the routing claim. A row proving the guard notices a deletion whose consequence is already
recorded twice would read as coverage without adding any.

## P07.S03a — L3 on the wire (v1.117.162)

Five rows, all tier 1, all replayable. **Two of them restore the state the product was actually
in**, and one restores a defect that would have been introduced by the obvious implementation.

| the defect, restored | what goes red | the check |
|---|---|---|
| **an L3 refusal reaches the initiator as EOF** *(replayable: `l3-refusal-arrives-as-eof`)* | `want ErrNotYourTurn` | `TestAnL3RefusalReachesTheInitiatorByName`, end to end over **both** transports. This is where the product was: `refusalAck` recognised two classes, everything else wrote no frame and closed, so a refusal arrived as `receive co-signed document: EOF` — a network fault, inviting the retry a refusal must not invite. The test's stimulus asserts both ends negotiated v2 first, without which it would pass forever on a transport where ALPN was never wired |
| **a named refusal is sent to a peer that cannot read one** *(replayable: `named-refusal-sent-to-an-older-peer`)* | `was sent to a peer that did not negotiate it` | `TestARefusalIsOnlySentToAPeerThatCanReadIt`. **The D32 violation the negotiation exists to prevent.** A build predating this version maps a one-byte reply and otherwise falls to the prefix check, so an unfamiliar frame makes it tell its user *"returned document is not the one sent this session"* — a tampering verdict about an honest peer, produced by a version skew. The guard's other half stops the fix becoming "send nothing to anybody" |
| **the initiator maps only one-byte refusals** *(replayable: `initiator-maps-only-one-byte-refusals`)* | `want ErrNotYourTurn` | same end-to-end test. The named refusal is two bytes, so the old length gate sends it past to the prefix check and the honest peer that just refused is reported as having returned a replay. The discrimination is a property of the bytes — `refusalFor` checks its own shape, a document is never two bytes, and every PDF begins `%PDF-` (0x25, not 4) |
| **an unknown code decodes to a known one** *(replayable: `unknown-refusal-code-decodes-to-a-known-one`)* | `want ErrRefusedUnknown` | `TestEveryRefusalCodeRoundTripsToItsOwnSentinel`, which walks the whole enumeration in **both** directions — a wire code is a value two builds must agree on, and two tables that map it are a protocol that can disagree with itself, which is the defect `refusalAck`'s own doc records from the last time |
| **an ALPN config site drops the v2 offer** *(replayable: `alpn-site-drops-the-v2-offer`)* | `sets NextProtos to something other than sessionALPN` | `TestEveryALPNConfigSiteOffersTheSameList`. Connections through that listener never negotiate, so L3 refusals on that path silently revert to EOF — and **every behavioural test stays green**, because they drive the sites left alone. The population-floor shape, one layer below `TestL2CoversEveryDocumentCarryingEntryPoint` |

### The `check()` exclusion has no row, and it is guarded anyway

`Channel.check()` must NOT require `Proto`: empty is a legal value meaning "this peer predates the
versioned session protocol", so requiring it would refuse every older peer outright — a one-line
mistake with a total blast radius. `TestTheNegotiatedProtocolIsNotRequiredByCheck` holds it. No
row, because the patch would be a one-line addition to a function whose every other line is a
requirement, and a row restoring "somebody added a field to a list" teaches nothing the guard's
own failure message does not already say.

### Two older rows went stale in the same change, and `--all` is what found them

`cosign-decline-arrives-as-eof` and `timeout-reported-as-decline` both patch `refusalAck`, whose
signature changed here — so both applied cleanly against the code they were written for and not
against this one. Neither the Go suite nor any individual replay would have said so: a stale row
fails only when somebody re-proves it, and until v1.117.156 re-proving the whole set meant
hand-rolling a loop.

**This is the second time in one day.** The first replay of the whole set found eight invalid rows
of eighty-one; this one found two of ninety-nine, on the same day the change that staled them was
written. The difference is that `./build/redproof.sh --all` now exists, so the gap between "a row
stopped being valid" and "somebody noticed" is one command rather than one archaeology project.
Both re-recorded against `9f8a36e` and replaying green.

## P07.S03b — the refusal that arrived and was then dressed as a network fault (v1.117.165–.167)

One row, tier 1. It is the layer above `P07.S03a`'s, and it is what made that slice's fix
invisible from outside.

| the defect, restored | what goes red | the check |
|---|---|---|
| **a refusal is reported as a connect failure** *(replayable: `refusal-reported-as-a-connect-failure`)* | `a refusal wearing a transport sentence` | `TestARefusalIsNotReportedAsAConnectFailure`. Both lifts are in the patch, because a rule enforced at one of two doors is the ADR-009 shape: `diagnosis.go` reaches `connectFailure` independently of the handler. Measured at **tier 4**: the refusal crossed the wire correctly and came out of the API as `{"error":"could not connect to peer: a co-signature takes exactly one prior signer"}` — a 502, wrapped in a false claim, and on the ceremony path also given a D19 *network* cause |

The guard asserts the **whole enumeration** and the **routing**, not one sentence: the failure mode
is a class of refusals falling through, so a test naming one of them would go green while eight
others still landed in the 502. Its control keeps the predicate from becoming "everything is a
refusal" — a genuine dial failure still gets its connect sentence.

### And the probe that should have caught it could not

`pairrepro.sh -n 4` reported *"the refusal arrived FLATTENED to EOF"* about a refusal that had
arrived perfectly well by name. Two independent defects in the probe, either sufficient: its
by-name branch grepped a message P07.S03a had renamed, and its fallback branch matched
`could not connect to peer` — the server's **generic wrapper**, present on the named refusal too.
So it could never have distinguished the two shapes and would have reported the old one forever.

**No row for that**, and the reason is the shape of it: the fix is a harness grep, and a red proof
replays a patch against the product. What guards it instead is that the wrapper it keyed on no
longer exists — the row above is what keeps it gone.

## P07.S04 — "one proceeding" stops meaning "the signers agree" (v1.117.169)

Five rows, all tier 1. **Two restore the state the product was in**, and one restores a defect a
mutation pass found that neither existing test could reach.

| the defect, restored | what goes red | the check |
|---|---|---|
| **one proceeding means the signers agree** *(replayable: `agreement-among-signers-is-a-proceeding`)* | `is reported as part of one proceeding` | `TestAgreementAmongSignersIsNotAProceeding`. Measured at the grill: two parties signing with the same arbitrary `abab…` value **they chose themselves**, on a document with **no ceremony record at all**, reported one proceeding on every signature — and the client renders *"✓ One proceeding — every signature on this document commits to the same ceremony."* The token is inside the signed `/Reason`, so it is a value the signer picks |
| **an unsigned record supplies a commitment** *(replayable: `unsigned-record-supplies-a-commitment`)* | `commits to a record NOBODY SIGNED` | `TestAnUnsignedRecordIsNotAProceeding`. **Found by mutation, not by review** — neither existing arm went red against it, because both compare against a *different* hash, so a commitment lifted from an unverified record still failed to match and the tests stayed green. The third arm is same roster, same id, record unsigned |
| **the token loses its format version** *(replayable: `roster-token-loses-its-version`)* | `does not carry the token in its documented shape` | `TestTheRosterTokenIsWellFormedWhereItIsActuallyBuilt`. `FormatVersion` is the first substantive axis of `rosterPreimage`, so two builds at different versions digest the identical roster to different hashes — and the client's honest reading was *"not produced by a single agreed proceeding"*, an accusation about people who agreed on everything, caused by one of them updating Nib |
| **a commitment is written without its version** *(replayable: `bare-commitment-written-without-its-version`)* | `was written into the signature` | `TestARosterHashWithoutAVersionCarriesNoToken`. **Both, or neither** — emitting nothing is the fail-closed direction, because a missing commitment disqualifies. The reader refuses version 0 for the same reason, so the pair cannot come through one door and not the other. Three fixtures failed loudly on this rule when it landed rather than silently carrying no commitment |
| **the attestations route re-verifies** *(replayable: `attestations-route-re-verifies`)* | `which verifies the whole` | `TestTheAttestationsRouteDoesNotReVerify`. Signature-count × document-**size** work on a request path with `document.sig` cached beside the bytes. The guard is structural **and says why**: measured, nine signatures on 31 KB is single-digit milliseconds, so the plan's 5.2 s needs ~95 MiB — a timing assertion on a small fixture measures noise, and a large fixture costs a minute on every `go test` |

### The UI branch has no row, and the reader scan is why it exists at all

`RosterVersion` is published, so `TestEveryPublishedObservableHasANamedReader` refused it until the
client had a branch that used it — which is how the "this is a version difference, not a
disagreement" sentence came to be written. That scan is a standing guard over every published
field, so a row restoring "the field is published and nobody is told" would duplicate a check that
already runs on every `go test`.

### And a third stale row, from the slice two before it

`mitm-reported-as-network-error` patches the block of lifted refusals in
`handleSessionInitiate`, and **P07.S03b inserted a new lift into that block** — the contribution
refusal, which had been falling through to `writeConnectDiagnosis` and coming out as a 502. The
row's trailing context stopped matching.

That is now three rows staled in two days by changes made two and three slices away from them, and
all three were found by `./build/redproof.sh --all` rather than by anything else. The pattern is
worth naming: **a red proof is coupled to the SHAPE of the code around its defect, not only to the
defect.** A row whose neighbourhood is being actively edited goes stale without anybody touching
the thing it proves — which is exactly why the sweep has to be a command rather than an intention.

## P07.S05 — the carry route (v1.117.172–.173)

Four rows for the half of the slice that landed, all tier 1. **One of them restores a direction the
plan's own task list had written the wrong way round.**

| the defect, restored | what goes red | the check |
|---|---|---|
| **a signature accepts its successor** *(replayable: `a-signature-accepts-its-successor`)* | `want A` | `TestASignatureAcceptsItsPREDECESSOR`. `crossBind` matches only a party who has **already** signed, so accepting forward leaves every signature unmatched until its successor lands and the last one unmatched forever. Measured when the task was implemented as written: three two-party ceremony tests failed with *"peer's signature does not accept you"* |
| **the receiver refuses a carrier because it did not sign** *(replayable: `carrier-refused-because-it-did-not-sign`)* | `the signer refused the carrier` | `TestAFourPartyCeremonyCompletesOverTheCarryRoute`, both transports. The state the product was in: the binding compared the last signer to the **wire peer**, so `coSignExchange` answered *"the document was not signed by the connected peer"* on a hop `AdmitContribution` had already admitted |
| **the cross-binding rule stops firing** *(replayable: `carry-relays-a-hostile-hop`)* | `prefix not cross-bound: admitted` | `TestTheGateRefusesEachThingByName`. **The exemption inverted at this slice and its fixture inverted with it** — "all but the LAST" was written while `AcceptedPeer` named the successor; it is "all but the FIRST" now, caught by a four-party carry ceremony failing at hop 2 |
| **the route always contributes** *(replayable: `the-route-always-contributes`)* | `the route can no longer both carry and contribute` | `TestWhetherYouSignIsReadOffTheRoster`. A `signs:false` convener would sign a ceremony they were convened not to sign, and a signature cannot be taken back off a document. The guard also asserts the **order**: the decision must come before `buildCoSigned` |

### Carry's two return checks have no row yet, and the reason is the fixture

`TestCarryRefusesAHostileHop` drives both — a reply that is not a prefix extension, and one that is
but was signed by the wrong party — through a helper rather than through the verb, because a
hostile hop needs a hostile PEER and that is a second process or a hand-rolled frame writer. A red
proof patches the product and reruns a check; here the check would still pass, because the helper
carries its own copy of the two tests. **Recorded as owed** rather than counted: it lands with the
rest of S05 when the route can be driven end to end.

### S05 staled four more rows, and two of them were disposed of rather than re-recorded

The pattern named two entries up held again, harder: the carry route edited the neighbourhood of
four rows at once. Two were straightforward re-records (`initiating-side-skips-the-l3-gate`, and
`l3-admits-a-substituted-proceeding` — whose *fixture* had also been left pointing forward when the
cross-binding direction inverted, so it failed on the wrong axis until it was fixed). The other two
were not:

- **`carry-relays-a-hostile-hop` was DELETED as a duplicate.** It was written this slice to restore
  the cross-binding check being switched off — which, once the exemption inverted from "all but the
  last" to "all but the first", is byte-for-byte the mutation `l3-admits-an-uncrossbound-prefix`
  already restores. Two rows for one mutation is coverage that reads as two and is one. Its
  inversion story moved into the surviving row's spec.
- **`channel-binding-reads-the-first-signer` was RETIRED, because its defect stopped having a
  consequence.** It restored `peer := ats[0]`, and it earned its place when that variable fed the
  two channel bindings. P07.S05 replaced both with L3 inside a ceremony, and outside one
  `len(ats) != 1` makes index 0 and index last the same attestation — so the patch now applies
  cleanly and **the check passes**. What `peer` still decides is which signature the consent card
  describes, which is a real property and a different claim; it is owed a guard of its own and is
  named in P07.S05a rather than left standing under a row that no longer proves it.

`recorded` 109 → 107, and a floor going **down** is worth saying out loud: it is what the ledger
looks like when a slice removes the reason a guard existed rather than when somebody deletes one.

## P07.S05 closed — the relay's document, and the proofs that stopped being owed (v1.117.176)

Five more rows, all tier 1. Two of them are the pair the previous entry recorded as **owed**, and
the reason they stopped being owed is worth more than the rows.

| the defect, restored | what goes red | the check |
|---|---|---|
| **the carrier relays a document it did not hand over** *(replayable: `carry-relays-what-it-was-not-handed`)* | `a different document` | `TestCarryRefusesAHostileHop`, through the **real verb** against a hostile receiver |
| **the carrier relays a contribution out of order** *(replayable: `carry-relays-a-contribution-out-of-order`)* | `extended_by_the_wrong_party` | same test, and it is a separate row because **the prefix says the bytes grew from mine and nothing about who signed the part that grew** — measured: with the prefix alone, a reply extended by the wrong party is accepted |
| **a relay accumulates a document per hop** *(replayable: `a-relay-accumulates-a-document-per-hop`)* | `the registry holds` | `TestARelayReplacesTheBatonRatherThanAccumulating`, driven at **nine** hops — two is also what a door that replaces "the active document" looks like, and eight is where the count cap is |
| **the relay door skips the byte cap** *(replayable: `relay-door-skips-the-byte-cap`)* | `want ErrTooManyBytes` | `TestTheRelayDoorHonoursTheByteCap`. A tightening rather than a preserved property: the door these bytes used to take applies no cap at all |
| **the initiate route bypasses the relay door** *(replayable: `initiate-route-bypasses-the-relay-door`)* | `does not install through the relay door` | `TestTheInitiateRouteInstallsThroughTheRelayDoor`. **Found by mutation** — swapping the call back left every behavioural test green, because they drive the door directly and nothing drove the route |

### Why the first two were owed, and what closing them cost

They were recorded as owed because their fixture ran `Carry`'s two return checks **in a helper
beside the test** rather than through the verb. That is a fixture asserting itself: the mutation
that matters is deleting a check *from `Carry`*, and a helper carrying its own copy of the check
stays green against exactly that. A red proof would have replayed green and the ledger would have
claimed coverage it did not have — the second failure mode, arriving through the test rather than
through the code.

What it cost to close was a **hostile receiver**: accept one connection, run the spoken check, read
the frame, reply with anything. Twenty lines. The shortcut was taken because a hostile peer *sounded*
like a second process, and it is not.

`recorded` 107 → 112.

---

## P07.S05a — the count was right and no user could open it (v1.117.178–.179)

Five rows. **Four of the five are about a surface rather than a computation**, and that is the
slice: C18's completeness sentence was computed correctly by code that already existed, and the
question every one of these rows asks is whether anybody can *reach* it.

| Defect reintroduced | Check that fired | What it said |
|---|---|---|
| `completeness-gated-on-a-signature` — the attestations route resolves the proceeding only for a document that already carries a signature | `TestAConvenedDocumentReportsItsObligedSignersBeforeAnyoneHasSigned` | "obliged = 0, want 2 — the route reports no roster for a convened document, so C18 cannot say that nobody has signed yet" |
| `ceremony-panel-unreachable-unsigned` — `updateBadge` hides the details button on any document with no signatures | `ceremonycomplete.test.mjs`, tier 2 | "the details button is hidden on a ceremony document, so C18 has no surface at all" |
| `ceremony-panel-returns-early-unsigned` — `openSigDetails` returns early on the same condition | `ceremonycomplete.test.mjs`, tier 2 | "the panel did not report the ceremony's completeness" |
| `consent-gate-reads-the-first-signer` — `coSignExchange` hands the Confirmer `ats[0]` rather than the last attestation | `TestTheConsentGateIsGivenTheRightSignature` | "the consent gate was shown …, want the LAST signer B — the card names the party whose contribution the user is being asked to build on" |
| `hop-mirrored-after-the-response` — `handleSessionInitiate` mirrors after `writeJSON` rather than before | `TestBothSidesOfAHopMirrorIt` | "the hop is mirrored AFTER the response returns, so a crash in between leaves a user who was told their signature is safe and a machine with no copy of it" |

**Two rows for one sentence, deliberately.** The panel has two doors and each closes the surface
alone: restore the button gate and the control disappears; restore the early return and the control
is offered and does nothing. A single row would let the other regress in silence — the ADR-009
shape, arriving through the *proofs* rather than through the code.

**The patch restates a defect the fix DELETED, and says so.** The shipped gate was
`p2p.ClaimsAProceeding(doc.sig)`, which went with the fix, so a patch restoring it by name would
fail to **build** rather than fail its check — red for the wrong reason, the third failure mode.
`len(doc.sig.Signers) > 0` is the same gate in its consequence: it excludes exactly the unsigned
document, which is the whole defect.

**`attestations-route-re-verifies` was made STALE by this slice and re-recorded**, caught by
replaying it rather than by noticing — its patch context named the gate that no longer exists. Its
own comment now carries the correction, and the half of its claim that was wrong (the lookup being
conditional) points at `completeness-gated-on-a-signature` instead.

`recorded` 112 → 117.

---

## P07.S05b — a nine-party ceremony completes, and S04's writer was never there (v1.117.180–.181)

Four rows. **Three record a state the product had been shipping in for four slices**, which is the
slice's finding rather than its incidental output: the driver that completes a relay is what made
them visible, and none of them is about the relay.

| Defect reintroduced | Check that fired | What it said |
|---|---|---|
| `ceremony-signature-names-no-ceremony` — `StampCommitment` is a no-op, so no contribution carries `[NibRoster:…]` | `TestACeremonySignatureNamesItsCeremony`, tier 1 | "a signature that does not name its proceeding leaves OneProceeding false forever and L3's substituted-proceeding refusal with nothing to compare" |
| `one-contribution-door-skips-the-commitment` — the initiating door stops calling the one door | `TestEveryContributionEntryPointReachesTheGate`, tier 1 | "buildCoSigned adds a signature block without stamping the ceremony's commitment on it" |
| `ceremony-hop-forced-onto-quic` — the glare path is selected without consulting the transport | `TestACeremonyHopIsNotForcedOntoQUIC`, tier 1 | "so a TCP ceremony hop races an empty candidate set and spins until connectDeadline with the receiver armed and idle" |
| `relay-accumulates-instead-of-replacing` — `installCeremonyResult` adds instead of replacing | `./build/pairrepro.sh -n 4`, **tier 4** | "8 hops added 8 documents to the convener — the baton is accumulating instead of being replaced, and at N=9 ADR-005's cap of 8 refuses the ceremony partway through" |

**The first row is the one worth reading twice.** P07.S04's whole clause 2 was *"OneProceeding
means agreement WITH the document's record"*, and it shipped the token format, the reader, D32's
version-skew sentence and L3's proceeding check — **with no writer**. Every ceremony signature
carried no commitment, `OneProceeding` was false on every real document, and
`ErrProceedingMismatch` compared against a field that was always `""`.

**And the guard written to notice could not have.**
`TestTheCommitmentCheckIsLimitedUntilS04` asserted "no production attestation carries a RosterHash"
and had a `Skip` arm for the day one did — over a fixture it hand-signed with an explicit empty
commitment. It measured its own input, so the arm was unreachable whatever production did. That is
the difference between a boundary asserted and a boundary *described*, and it cost four slices.

**The fourth row is tier 4 and nothing cheaper can see it.** The failure needs a real relay with
enough hops to cross ADR-005's cap, and the property is a **delta** rather than a total — eight
hops must leave the same footprint as one. A unit test over `installCeremonyResult` asserts the
function replaces; this asserts a ceremony walked end to end through the real routes actually goes
through that door at every hop.

`recorded` 117 → 121.

---

## P07.S05c — the seeker announces, the sought listens (v1.117.182–.184)

Four rows. **Three of them cannot be reached by any run in the tree**, and that is the slice: the
mechanism exists for a peer arriving *after* the arm's five-minute announce window has expired, and
every hop of every run falls inside it. The policy was separated from its socket so a fake clock
could reach the state a real one needs five minutes to.

| Defect reintroduced | Check that fired | What it said |
|---|---|---|
| `arm-answers-any-announcer` — the L1 gate deleted, so an arm answers any sighting | `TestTheArmAnswersItsOwnPeerAndNobodyElse`, tier 1 | "answer 0 was for , want this arm's own peer … an arm that answers a name it does not hold is L1's whole prohibition, and it makes two armed parties answer each other forever" |
| `arm-answers-once-and-never-again` — the rate limit becomes permanent | same | "a first-only means a later hop of the same ceremony cannot find this party at all, which is the whole reason this mechanism exists" |
| `failed-answer-spends-the-window` — the clock is stamped even when nothing was announced | `TestAFailedAnswerDoesNotSpendTheWindow`, tier 1 | "a failed answer spent the rate-limit window, so a party that never actually announced was then silenced" |
| `lan-namespace-drops-n` — `FLAGS` loses `-n`, so `--lan -n 4` runs a two-party ceremony | `./build/pairrepro.sh --lan -n 4`, **tier 4 `--lan`** | the signature count: a two-party run produces 2 where a relay hop produces its hop number |

**The first row is the one to read twice, because the guard that catches it could not catch it
when it was written.** `answer` took no arguments, so the test counted answers without identifying
them — and a stranger's sighting followed by a later one produces the same **count** as the peer's
sighting followed by a later one. Deleting the gate left every assertion green. The seam now hands
the resolved candidate to the answer callback, which production ignores and the test asserts on.

**L1 is a property about WHICH peer, and a count cannot see it.** That is the same lesson P07.S05b
recorded one slice earlier from the other end of the codebase — *"`/ByteRange` counts blocks, so
one party signing eight times satisfies any count"* — and the distinct-signer set was the answer
there. Twice in two slices, on opposite sides of the tree: **when the property is identity, count
the identities.**

**The fourth row is the harness, and it is the third of three barriers** that stood between the LAN
clause and its only driver — the others being an explicit `--lan is N=2-only` refusal and the
`N != 2` block's `exit 0` three lines before the `LAN` block. Any one alone made `--lan -n 9`
impossible. With all three gone, the first measurement of a ceremony on a link found that a
ceremony carrying an invitation bootstraps the public DHT unconditionally, so P03's
no-outbound-traffic criterion is false for every ceremony P07 builds (`/pending 299`).

`recorded` 121 → 125.

---

## /pending 289 — re-delivery existed on one transport of two (v1.117.187)

| Defect reintroduced | Check that fired | What it said |
|---|---|---|
| `tcp-ceremony-cannot-redeliver` — `runSession` returns on `served`, so the TCP listener closes at the co-sign | `TestCeremonyReDeliversAfterReconnect/tcp`, tier 1 | `initiator: dial tcp 127.0.0.1:…: connect: connection refused` |
| `redelivery-window-rearms-per-reconnect` — `postSign` assigned again inside the loop | `TestTheArmWindowIsNotExtendedByConnectionsThatProduceNoSession`, tier 1 | "the re-delivery window must be fixed once when the hop signs, or each reconnect pushes it out" |

**The item said this needed tier 4, and its own twin refuted that.** The entry's reason for filing
rather than building was *"that is a tier-4 shape (`pairrepro.sh`), not a package test"* — while
`TestCeremonyReDeliversAfterReconnect`, the QUIC drive it cites as the reason the gap is invisible,
**is** a package test. The only transport-specific part was the handful of lines that dial. Made
table-driven, and the TCP subtest went red on its first run.

**What it found is larger than what was filed.** The entry described a *guard* that was structural.
The truth was that the *behaviour* was absent: P05.S10's criterion 15 is implemented in
`runCeremonyReceive` and was never implemented in `runSession`, so a TCP ceremony wrote a
re-delivery cache that nothing could reach. A structural guard asserting both call sites pass a
ceremony says nothing about what either does with it — which is the ADR-009 lesson read from the
other end: routing is necessary and not sufficient.

**And the fix tripped a guard, correctly.** There are two absolute deadlines now, so the arm-window
rule — every `timer.Reset` a remainder, never a fresh period — had to be stated for both, and it
gained a second arm: `postSign` assigned exactly once. Both arms mutation-tested; they fail
independently, which is why they are two rows.

`recorded` 125 → 127.

---

## /pending 284 — the blast radius was one shape (v1.117.189)

| Defect reintroduced | Check that fired | What it said |
|---|---|---|
| `embedded-shape-vanishes-from-the-scan` — `discoverObservables` drops any shape with an embedded field | `TestEveryPublishedObservableHasANamedReader`, tier 1 | "`published` names \"discovery.Seen\" and it was NOT discovered as a published observable … this scan has quietly stopped covering a shape it claims to cover" |

**The failure mode is the one this scan exists to prevent, turned on the scan itself.** A shape that
vanishes from a reader scan reads identically to a shape that publishes nothing: the count goes
down, every remaining entry passes, and nothing says a field went uncovered. It was caught only
because the `published` keys were later validated against the discovered set — the *discovery* half
had been unchecked, which is the same asymmetry `docs/red-proofs.md` records elsewhere as a check
that grades the response without asserting the stimulus.

**The item was filed as a Finding rather than Low Hanging Fruit because of a question nobody had
answered** — *"how many OTHER shapes in the ten scanned packages embed? Nobody has counted."*
Counted: **of 56 exported data shapes across all ten packages, exactly one embeds**, and the type it
embeds is itself discovered. The blast radius the fix had been waiting on was one shape and one
field.

**And the fix's second arm has no case in the tree, so it was probed rather than assumed.** An embed
whose type is *not* discovered is now reported by name instead of dropped — a state nothing in the
repo produces, so the probe made `Announcement` undiscoverable (one unexported field) and confirmed
the message names it. A guard for a state that cannot occur today is exactly where a vacuous green
hides.

`recorded` 127 → 128.

---

## /pending 285 — the identity race, and a guard that fired only sometimes (v1.117.190–.191)

| Defect reintroduced | Check that fired | What it said |
|---|---|---|
| `identity-minted-twice-under-race` — the if-absent check removed, so the identity store overwrites unconditionally | `TestConcurrentFirstCallersAllGetTheStoredIdentity`, tier 1 | "8 concurrent first callers minted 5 DISTINCT identities; every one of them believes it holds this machine's signing key, and all but one is wrong" |

**The row is here twice over: once for the defect, once for what recording it exposed.** The first
version of the guard reproduced 5 distinct identities on one run and **zero** on the next, and the
red proof came back *"the check still PASSED"* — the third failure mode, arriving through a guard
that was correct and merely probabilistic.

**A race guard that fires sometimes reads as a pass on the run where it mattered**, and nothing
distinguishes it from one that works. It now drives **40 fresh vaults** and fails on the first round
that diverges, naming the round; each round gets its own vault, because a second round against a
vault that already holds an identity takes the fast path and proves nothing. Confirmed reproducing
five runs out of five, and the row replayed three times.

**And the fix is a deletion, not an addition.** `SetIdentity` overwrote unconditionally and had
exactly one production caller. A checked setter *beside* it would have left the defect one call site
away, so the unconditional one is gone — the race is unrepresentable rather than fixed, which is the
difference between a bug that stays fixed and one that returns through a second door.

The check asserts two things and the second has the teeth: that every caller **agrees** is necessary
and not sufficient, because they could all agree on a key the vault does not hold — the same orphan
by another route. So it reads the vault back and requires the agreed identity to be the stored one.

`recorded` 128 → 129.

---

## /pending 287 — a ceiling on import, and the twin writer (v1.117.194)

| Defect reintroduced | Check that fired | What it said |
|---|---|---|
| `vault-import-accepts-a-future-format` — `Validate` keeps its floor and loses its ceiling | `TestValidateRefusesWhatCannotBeOpenedHere`, tier 1 | "a backup written by a NEWER Nib passed validation … the user loses the vault they had AND cannot open the one they imported until they update" |
| `server-hand-rolls-an-atomic-write` — a temp-file-plus-rename written inside `internal/server` | `TestEveryFileWriteGoesThroughTheAtomicDoor`, tier 1 | "export.go calls os.Rename directly … which is exactly what internal/vault's same-named twin was" |

**The first is ADR-009 read at its sharpest: the door existed and one caller did not use it.**
`checkEnvelopeVersion` had already been written, and its own history is recorded in `vault.go` —
this was the site the original fix did not reach. A floor with no ceiling let through exactly the
file `Open` would refuse, *after* the overwrite that destroyed the only other copy.

**The fixture bumps the version on a REAL, openable backup**, so the only thing wrong with the file
is the number. A hand-built envelope would be refused by the shape checks above it and pass the test
without the ceiling existing at all — the shape of vacuous green this ledger keeps recording.

**The second patch puts the hand-rolled write BESIDE a real door call, deliberately.** A guard that
only required `atomicfile.` to appear somewhere in the file would pass exactly that shape, and the
failure being reproduced *is* a second implementation nobody compares against the first. So the
guard bans `os.Rename` in the package rather than checking for a call to the door.

Structural, and durability is why: proving a write survived a crash needs a crash, and proving one
did not is proving a negative about the page cache. What is checkable — and what actually regressed
— is whether a caller reaches the door at all.

`recorded` 129 → 131.


---

## /pending 278 — two thirds already shipped, and the third is a guard the harness cannot be (v1.117.196)

**No new rows. This closes an item by measuring it, and two of its three halves were already done.**

The entry's headline — *"zero of the 65 rows drives an L1 or L2 guard"* — was true on 2026-08-23 and
is not now. `l1-wire-derived-pin` drives `TestNothingWireDerivedReachesAPin` and
`l2-exchange-reached-unconfirmed` drives `TestL2NoDocumentBytesCrossBeforeBothConfirmations`; both
replay red, both patch exactly one file. The phantom citation it named — a row quoting
`zz_l1fixture.go: redProofWireDerivedPin`, a file that was never in the tree — is annotated at both
of its sites and points at the real row.

**What was left is the half the harness structurally cannot check itself.** A red proof asserts that
a defect makes a check fail; it never asserts that the patch contains *only* that defect. Four rows
were once generated with a bare `git diff` while `test/redproofs/*.patch` are themselves tracked, so
each regeneration swept the previous ones in — one came out at 214 lines and six hunks for a
one-comment mutation, and **all four still replayed green**. Every check passed and every row was
wrong about what it recorded.

`verify_test.go` now requires each patch to touch exactly one file, and separately refuses any patch
that modifies `test/redproofs/` — the specific shape, which stays detectable even if a legitimate
multi-file mutation is ever needed. Measured across all 131 rows: **zero violations**, so the guard
is a tripwire rather than a repair.

**And writing it found a smaller thing.** Counting `diff --git` headers reported **four rows as
touching zero files** — they are hand-written unified diffs with no git header, they apply, and they
replay. A guard that fires on the wrong thing while missing what it was written for is the failure
this ledger keeps recording, so the count is on `+++` lines, which both formats share.

---

## /pending 300 — the stale announcement, found with the instrument built for it (v1.117.199–.201)

| Defect reintroduced | Check that fired | What it said |
|---|---|---|
| `signed-arm-keeps-advertising` — the per-iteration `wanted`/`stop` check removed | `TestASignedArmStopsAnnouncingAndStopsAnswering`, tier 1 | "the arm never released its announcer after signing … a release that waited for a sighting would wait forever" |

**This is the sweep's one item that went from "cause NOT established" to a measured mechanism**, and
what changed was building the instrument the item itself named as its blocker: *"nothing in the tree
can show what a machine hears on the link."* `GET /api/lan/heard` answered it in one run — after a
four-party QUIC relay completed, the convener still heard **all three parties announcing QUIC
endpoints**, six candidates, on both probes four seconds apart.

**The chain, each link measured rather than argued.** An arm keeps its announcer for the whole
post-signing re-delivery window (five minutes) → a later ceremony browses a link full of stale
announcements → if its own fresh announcement is missed inside the two-second browse the set is
entirely stale → all-QUIC, so the glare path is taken → the dial goes to endpoints that will not
serve this ceremony → *"Couldn't reach the rendezvous network"*, a verdict about the DHT for a peer
on the link and announcing.

**Two sites, and the second is why the first was not enough.** Closing the arm announcer at signing
took six stale candidates to **two** — the pair belonging to the party whose hop had just finished,
still present four seconds later. Those came from `answerHopSeekers`, whose gate ran only when a
sighting resolved; once a hop has signed nothing is announcing to it, **so the gate never ran at
all**. Per-iteration now, with a stop callback that releases an answer already in flight. Measured
after: two in-flight datagrams in the first probe, **nothing** in the second.

**And the check acts AFTER the read.** A `continue` at the top of the loop would skip its only
blocking call and spin a long-lived goroutine at full CPU — the read's one-second deadline is what
paces it. The comment says so at the line, because the obvious edit is the wrong one.

**The instrument shipped broken and using it is what found that too.** `link_report`'s f-string used
backslash-escaped quotes inside the expression — a `SyntaxError` before Python 3.12 — so it would
have printed nothing on the failure it exists for. One commit old, and caught on first use.

`recorded` 131 → 132.

## P08.S01 — a ceremony survives a restart on every machine *(v1.117.240)*

Three rows, tier 1. **One of them proves a defect that was live in shipped code**, and the other two
prove the slice's own new refusals.

**`invites-omit-the-recital`.** `handleCeremonyInvites` rebuilt each party's invitation from the
mirror's record plus the vault's stored secret, field by field, and omitted `Intent`. The recital is
compared by `MatchesRecord` and required by `Contribute`, so **every re-issued invitation parsed,
armed, and was then refused at the recipient's arrival gate** — after the convener had been told the
re-issue worked. It survived because the route has exactly one reference in the whole tree, its own
registration: no test, no harness clause, no caller in the web client. It is nonetheless the only
production reader of `ReadMirror` and the convener's only disk-based recovery path (D21, gap #24).

The guard runs `MatchesRecord` itself rather than diffing fields. A field-by-field comparison is the
same shape that produced the defect — a constructor that forgets one field, checked by a test that
forgets the same one — and running the production comparison means a future field added to the
commitment fails this row without anyone remembering to extend it.

**`accept-stores-no-invitation`.** The slice's own subject: an invitee that pins the convener and
keeps nothing has a pin, an identity, and no way to rejoin after a restart. Written as *arm by
ceremony id succeeds* rather than as "a row appeared in the vault", for `accept-pins-nobody`'s
reason — a row that does not satisfy the door it was created for satisfies nothing. Its stimulus is
an arm naming a ceremony this machine never accepted, which must be refused, so the pass cannot come
from a build that ignores the field.

**`arm-picks-a-source-silently`.** Both a stored ceremony id and a literal invitation are individually
valid, so an arm given both succeeds either way and the caller cannot learn which was used. Two
sources for one value with the loser silent is ADR-009's shape; `checkTransport` already records the
answer for its own case — refuse, never downgrade.

**Nothing staled.** All 23 existing rows whose patches target the files this slice touched were
replayed and each still goes red against its own defect and prints its `EXPECT`.

`recorded` 178 → 181.

## P08.S03 — the ceremonies on this machine *(v1.117.243)*

Three rows, tier 1.

**`skew-reported-as-a-forgery`.** Drop the `ErrVersion` branch and a record written by a *newer* Nib
falls through to `LoadUnverifiable` — the user is told their intact ceremony "does not verify".
`Record.Verify` checks the version first, so every skewed record also fails the checks below it;
`FormatVersion` has moved three times already and Nib self-updates. Without the branch, updating Nib
mid-ceremony reports every live ceremony as unverifiable *and* leaves them unprunable, because the
prune must first establish that a ceremony ended. The guard asserts the class **and** the sentence,
since the class alone passes with a message that still accuses somebody.

**`an-unloadable-ceremony-vanishes`.** Keep only the entries that loaded and a damaged one
disappears from the listing rather than appearing as a degraded row — the user's folder then holds a
ceremony their Nib will not admit exists. The guard is written on the **count** as well as the
state, because a test checking only the surviving entry passes against a listing that silently
dropped the other.

**`listing-opens-the-document`**, and its first attempt is worth recording. The mutation called
`ReadMirror` and **discarded** the result; the document was opened, the outcome was unchanged, and
`redproof.sh` reported *"the check still PASSED"* — the outcome it exists to distinguish from a real
red. The test asserts the state, so a call that touches the document and ignores it is invisible to
it. Re-cut as the realistic regression — `ReadStored` actually *using* `ReadMirror` — which a planted
non-PDF then makes fail. The lesson is the general one: a stimulus that changes nothing observable is
not a stimulus, and the harness caught it rather than the reviewer.

`recorded` 181 → 184.

## P08.S07 — two refusals that were already there *(v1.117.246)*

Two rows, tier 1, and both prove something about a check rather than about new behaviour.

**`already-convened-is-not-a-conflict`.** Stop mapping `ErrAlreadyConvened` to 409 and a second
convene answers 400 — "everything the convener can fix" — for the one refusal that is not about a
field they can correct. The refusal has existed since P07.S02a and was driven only at the *package*;
this row is the route, which C13's word "server-side" asks for and which nothing had ever asserted.

**`reissue-regenerates-the-secret`, and it exists because the test it proves is otherwise true by
construction.** `handleCeremonyInvites` reads each party's secret back from the vault, so "every
other party's state is untouched" passes against any behaviour at all. The patch mints fresh secrets
instead, which makes every re-issued invitation differ from the one convene handed out — silently
invalidating the copies every party still holds. Without it the assertion restates "nothing changed"
in a function that changes nothing.

**Its first cut went red for the wrong reason and the harness said so:** removing the only use of
`v` left `declared and not used`, so the check failed to *compile* — which `redproof.sh` reports
distinctly, because a non-zero exit is also what a deleted check produces. Same shape P07.S09a hit
with two of its first three mutations. Re-cut to keep the call and discard its result.

`recorded` 184 → 186.

## /pending 315 — the refusal enumeration, derived *(v1.117.257)*

Three rows, tier 1, all against one new guard — and the guard was written to close a *tidiness*
finding, then found a live defect on HEAD before it had a red proof of its own.

**`refusal-sentinel-reaches-the-initiator-as-eof`.** The one that reintroduces what was actually
wrong. `ErrNoSignaturePages` loses its wire code, so `refusalAck` returns `(nil, false)`, `Receive`
writes no frame, and the initiator's `readFrame` gets EOF — surfaced as *"502 co-signing did not
complete: receive co-signed document: EOF"*, a network fault inviting the retry a refusal must not
invite. That is P07.S03a's defect. It had survived in **three** sentinels (`ErrNoSignaturePages`,
`ErrBlockOffThePage`, `ErrNoCeremonyIntent`), all raised inside `coSignExchange`, because both
"every code" tests were hand-written literal slices and a slice that lists what it checks cannot
report what is missing from it. Three assertions fire, which is the sentinel-coverage arm working:
the code becomes un-emittable, the live round trip stops closing, and the sentinel is named as
carrying neither a code nor a `// No wire code:` line.

**`refusal-code-renumbered`.** `refusePrefixMismatch` moves 3 → 30. **No other test in the tree can
see this**, and that is the row's whole justification: a renumber keeps both switches symmetric,
keeps both literal slices valid, and leaves the package green, while a peer on the shipped build
reads 3 as this refusal and 30 as something it has no sentinel for. It is caught by the one part of
the guard that is deliberately **not** derived — a hand-written frozen table, honest precisely
because these are wire values: a number a future build may change was never a wire value, so the
table is a record and adding a line to it is the act of freezing.

**`refusal-decode-defines-an-unknown-code`.** A `case 42:` in `errorForCode` with no matching
constant — a decode-only code, reserved against a meaning no build can produce.

### The guard's own two defects, both found by these rows

Written up because the pattern is the one this ledger keeps recording, and this time it was the new
guard rather than the code under it.

- **`refusal-decode-defines-an-unknown-code` passed GREEN on its first run.** `caseSubject` returned
  `""` for anything that was not an identifier, so a bare literal case was invisible to the very
  check written to catch one. The `*ast.BasicLit` arm exists because of this row.
- **`refusal-sentinel-reaches-the-initiator-as-eof` went red with the wrong message.** A `len(enc) <
  12` setup floor fired on the single-case deletion the guard exists to catch and reported *"the
  switch walker has gone blind"*, masking the three assertions that name the sentinel. A floor that
  fires on the condition the guard is for is a dead conjunct in front of the real check; it is now a
  blindness floor (`== 0`) and the per-code assertions carry the diagnosis.

`recorded` 186 → 189.

## /pending 311 — the two arm doors *(v1.117.259)*

One row, tier 1, and it fires **two** checks that are different halves of the same rule.

**`arm-doors-disagree`.** `arm()` reverts to asking `se.ln != nil`. A connect-based ceremony arm
sets `se.cer` and leaves `se.ln` nil, so a following manual/TCP arm sees no listener and succeeds,
overwriting `se.cer` with no `close()` on what it displaced — orphaning that ceremony's rendezvous
server, shared UDP socket, router port-mapping lease (whose refresh goroutine only `close()` stops),
in-memory document and invitation secret.

The **behavioural** check catches the orphaning. The **structural** one catches it as ADR-009 —
`arm` deciding armedness without routing through `armedLocked()` — and it is the half that would
catch a *fourth* site added later, which comparing three copies for agreement cannot do.

**The predicate-COUNT assertion stays green under this patch, deliberately.** The reverted door
writes `se.ln != nil`, not the full predicate, so counting copies never sees it. That is precisely
why the guard asserts routing as well as counting, and it is worth recording: a guard written only
as "this expression appears once" would have replayed green against the defect it was written for.

`recorded` 189 → 190.

## /pending 316 — the two second atomic-write implementations *(v1.117.261)*

Two rows, tier 1. The guard promotion at v1.117.251 could not produce either of them, and warning 4
of P08's external plan-review says why: `atomicdoor_test.go`'s only error condition is
`os.Rename` reached from outside the door, so a write that simply becomes non-atomic introduces no
rename to find and leaves it green. Deleting the two exemptions proves nothing on its own.

**`node-cache-shares-one-temp-name`, and it is a measured corruption channel rather than a style
deviation.** `writeNodes` goes back to `os.WriteFile` into a fixed `dht-nodes.tmp` plus
`os.Rename`. Writer B opens the fixed name before A renames it, so both hold a descriptor on the
same inode; after A's rename that inode IS the published cache and B's truncate-and-write lands on
the live file — defeating the rename's atomicity and `writeNodes`' own *"do NOT truncate a good
cache"* rule twenty lines above. Reachable because `nodeCacheDir` is one path per config dir and
three sites open a `rendezvous.Server` against it, one per ceremony, each saving on `Close()`, plus
the second Nib this repo deliberately permits. **Measured with the defect applied: 211 of 600 writes
failed and 22 of 20001 reads saw a cache that was neither writer's file.** The check asserts the
CACHE rather than the error, because a failed write costs one cold start and a torn cache is silent
— and at an unlucky length parses into node addresses assembled from the wrong offsets, which
`cacheMagic`'s own doc works out.

*Its patch also drops the `atomicfile` import.* Without that the package fails to COMPILE, which is
red for the wrong reason — the shape P07.S09a hit twice and P08.S07 hit again.

**`in-place-rewrite-is-not-durable`.** `writeAtomic` reaches for `atomicfile.Write` instead of
`WriteDurable` — the weaker of two same-shaped functions one letter apart, which is verbatim the
mistake `internal/atomicfile`'s package doc records paying for once already when `handleVaultImport`
called the rename-only twin to replace `vault.nib`. `-w` renames over the user's only copy, so a
crash in the writeback window leaves a truncated PDF where the original was, after `nib: rewritten`
has printed. Asserted structurally, and the guard states its own ceiling: fsync is not observable
from inside the process, so what is checkable is which door the package reaches for — which is the
thing that regressed. It cannot see a `Sync()` deleted inside `atomicfile`; that is the door's own
contract.

`recorded` 190 → 192.

## Tier 2's own row — /pending 315's sweep found the suite red *(v1.117.262)*

No patch, and the reason is the strongest form a red proof takes: **`published.test.mjs` was red
against HEAD**, and had been since v1.117.243. `sessionStatus.until` and `noticeView.{what,summary,
at}` were published with no declared reader, which for a status field means the user is never told —
the whole of what C03 exists for. Five commits landed on the red suite, four of them this sweep's,
because the sweep took tiers 0 and 1 for a baseline and not tier 2.

Recorded here rather than as a replayable row because reintroducing it is a one-line deletion from
the reader, and the ledger's value in this case is the *process* finding: the guard was not stale,
it was correct and unread, and the totals line is what hides that.

**And tier 2 then went red on the fix**, which is the part worth keeping: the rescue action called
`exportBase()` after an await, so it would have named the saved file for whatever document was
current when the fetch resolved — in precisely the case where the user has two documents open and
is about to lose one. `pinning.test.mjs` requires the literal `const exportName = exportBase();`
capture idiom, not merely an early evaluation, and it was right to.

## /pending 308 — the record's id and text, judged before consent *(v1.117.264)*

Two rows, tier 1, and the first is filed against a reason the item did not give.

**`record-verify-ignores-the-id-shape`.** The item said "path traversal behind a pinned but hostile
convener". The grill refuted that: `MirrorDir` refuses a bad id before `filepath.Join` is reached,
at all five path sites, and `TestTheMirrorRefusesAnUnsafeID` has driven it since P07. What the
patch actually reintroduces is an ORDERING defect — the record passes `Verify`, passes
`MatchesRecord` (a hostile id equals itself), reaches the user's consent, is **signed**, and is
refused only at `WriteMirror`. The convener then holds a real signature and the signer is shown
*"Signed, but not saved — do not close Nib."* for a ceremony that was never storable. Worth writing
down: the exposure was closed and the item was still real, for a different reason than it claimed.

**`record-verify-bounds-only-the-convener`, which the item did not contain at all and is the larger
half.** `checkIntent` and `checkRosterText` had exactly two callers, both inside `Convene` — the
convener's own door. So the caps bound the party who TYPES the recital, the labels and the
capacities, and left every recipient unbounded. That is the exact mirror of the asymmetry
`Verify`'s own roster-bound comment describes one block above, running the other way, and it went
unnoticed because the emitter is the only party any test had played. Measured with the defect
applied: a 5000-rune intent, label and capacity all verify.

`recorded` 192 → 194.

## /pending 305 — a block on a page that is not the last *(v1.117.267)*

One row, tier 1, and it is here for what it discriminates rather than for what it catches.

**`block-drawn-on-the-last-page-not-the-placed-one`.** `SignApproval` resolves the appearance to the
document's LAST page instead of the one the placement named. It **passes at n=4** — where the single
allocated signature page IS the last page, so the two answers coincide — and **fails only at n=9**,
where two signature pages exist and the first block belongs on a page that is not the last.

That asymmetry is the whole value. `TestABlockIsActuallyDRAWNWhereItWasPlaced` had `const n = 4` for
its whole life, so `Appearance{Page}` was exercised at exactly one value, and `blockink`'s own head
argues a roster *"changes only the VALUES of Page and Rect"* — true, and never checked at any value
but the last. A mutation reddening both rows would only prove the test runs; this one proves the new
row catches something the old could not.

**Two earlier mutations were tried and rejected**, and they are worth recording because "it went
red" is not the standard: forcing page 1 fails both rows, and a byte-counting page estimate was
simply wrong about how many pages there were. The proof has to go red *in the case being added*.

The measurement behind the row: placed == drawn at both counts, every widget carrying `/AP`, and the
middle signature page surviving six incremental rewrites with its `/Resources` and `/Contents`
intact. The vendored writer is correct — this is the regression guard that was missing, not a bug
report.

`recorded` 194 → 195.

## Four stale rows, found by the first `--all` replay since the sweep began *(v1.117.270)*

`./build/redproof.sh --all` reported **190 of 195 re-proved**. Four were stale and one was
pollution; all four are repaired here, and each staled for a different reason worth separating.

- **`afterfunc-callback-unrecovered` — stale BEFORE this sweep.** Its mutation site moved at
  v1.117.245 when C05 replaced `sessionAcceptTimeout` with `armWindow` in `runSession`'s arm timer.
  Nothing noticed because `--all` had not been run since. A row can rot from a change that never
  touched it.
- **`durable-write-writes-through` — staled by /pending 316**, which moved `atomicfile`'s temp
  pattern from `.tmp-*` to `.nib-*.tmp`; the patch quoted the old literal.
- **`mirror-record-written-first` — staled by P08.S02's third mirror file.** The patch swapped two
  adjacent writes and `document.sha256` now sits between them. Re-recorded as a positional move
  rather than a text swap.
- **`block-line-bound-skips-the-roster` — staled SEMANTICALLY by /pending 308, and this is the one
  that matters.** The patch removed `checkRosterText`'s call from `Convene`. Once `Record.Verify`
  gained the same check, that stopped reintroducing the defect — because `Convene` calls
  `rec.Verify` (`internal/ceremony/convene.go:255`), so **the rule survived its own call site being
  deleted and the row replayed green against a tree that still enforced it.**

  That is ADR-009 working, visible from outside: one predicate reached from two call sites means
  deleting one site does not delete the rule. It also relocates the honest mutation — **to the
  PREDICATE, not to a caller.** A two-file patch removing both callers is what
  `TestEveryRedProofPatchTouchesOneFile` forbids, for reasons it argues at length, and trying it is
  how this was worked out.

**The fifth was not stale — it was polluted.** `relay-accumulates-instead-of-replacing` is tier 4
and the run reported `ports already held: 18542(tcp)`, from a `uirepro.sh --keep` server left
running by a concurrent browser walk. Re-proved in isolation afterwards. This repo has recorded
"replayed green while polluted" before; this is the same hazard in the other direction, and the
harness naming the port is what made it legible.

`recorded` stays 195 — four repairs, no new rows.

## The sweep of /pending 320–324 *(v1.117.271–.279)*

Six rows across four items, and three of them exist because a mutation first replayed **green**.

**`stale-sidecar-bricks-the-mirror`** (/pending 321). The sidecar is no longer unlinked before the
document, so a torn rewrite at hop ≥ 2 leaves a valid document beside the previous hop's checksum
and `ReadMirror` reports `ErrMirrorDamaged` forever — against the user's own disk, with no repair
path, on the file that for a non-convener is the sole durable copy of their signature. The torn
state needs a crash, which this repo refuses to fake in the product, so the probe asks the *other*
side of the same ordering: make the DOCUMENT write fail while a good sidecar exists and see what the
sidecar is afterwards. That **discriminates** the two orderings; a mutation reddening both would
prove only that the test runs.

**`unreadable-record-reads-as-never-signed`** (/pending 320). `persistedFor` collapses every
`ReadMirror` error to nil again, so "I could not find out whether I signed" becomes "I did not" and
a second differently-timestamped signature is minted from one identity. Three arms are driven
separately — absent is a MISS, stored is a HIT, unreadable is UNKNOWN — because the danger of the
fix is the opposite error: an over-broad "unknown" refuses every FIRST hop, since a machine that has
never signed has no mirror at all. **That is not hypothetical: it is what the first cut did**, on the
reasoning that a missing document returns `(record, 0 bytes, nil)` — true of `document.pdf`, false
of `record.json`. Two ceremony tests caught it. The absent arm is that regression pinned.

**`a-second-proceeding-overwrites-a-stored-ceremony`** (/pending 318). The guard discriminates on the
id instead of the roster commitment — and since the attack IS a colliding id, it walks through.
Recorded beside it, without its own row because it is a placement rather than a predicate anyone
would edit: moving the guard below the writes fires **only** the byte-identity assertion, which is
what proves the refusal and the surviving bytes are independent facts. The check also carries a
setup assertion that the legitimate per-hop rewrite still succeeds — every hop rewrites the mirror,
so a guard refusing that would break the product while passing everything else.

**`an-off-roster-signature-passes-as-complete`**, **`a-version-skew-is-accused-of-disagreeing`** and
**`disagreement-never-reaches-the-exit-code`** (/pending 324). The middle one is an **anti-proof**:
it guards against the FIX, not the defect. The naive form of defect 2 — a bare `!oneProc` in the
exit condition — was measured exiting 2 on a document whose counterparty had merely updated Nib,
because the CLI never had the web's two D32 discriminators. That would break the README's own
`nib verify x.pdf && echo intact` over something no party did wrong.

**And the third of those first replayed GREEN**, which is the most useful thing in this batch. Its
fixture used an off-roster stranger as the second signer, so `refuses()` still fired through
`hasUnrostered()` and dropping `disagrees()` from the door was invisible. **An arm that two
predicates can satisfy cannot say which one is missing.** Both signers are now on the roster, the
check asserts it, and only `disagrees()` can carry the arm.

`recorded` 198 → 201. (Six rows were written across this sweep; three of the earlier ones landed with their own commits, so the count moves by three here.)

## P08.S04a — the signing party's own clock *(v1.117.282–.283)*

Two rows, tier 1, and the second is the one the slice is really about.

**`expired-ceremony-signed-at-the-arrival-gate`.** The signer's deadline check is removed. Nothing
else on that path compares `Expires` to `now` — `Record.Verify`'s only clock is a *future* ceiling,
and the one refusal that exists has a single production caller on the **convener's** side. So the
signer is collected into a proceeding D28 declares over, and the party who convened holds the only
clock that could have said so.

**`arrival-gate-reserves-a-whole-hop` — the discriminating one, and the reason the check has three
arms rather than two.** The gate stays present and still refuses expired ceremonies; only its budget
moves, 0 → `ceremonyHopBudget()`. The "expired is refused" arm therefore stays **green**, and only
the honest-worst-case arm goes red. **Nothing else in the tree can tell "budget too large" from
"feature absent."**

It is not a hypothetical defect: it is the deepdive's own recommendation, made falsifiable. The
convener admits a hop when `Expires > t0 + 29m20s`; the signer's gate runs at worst `t0 + 22m20s`.
So a legitimately admitted hop arrives with as little as **7m00s** left, and any reservation at that
end refuses it with neither party at fault. **The margin IS the clock-skew tolerance between two
machines with no time sync** — spending it costs the whole thing, and at the proposed 8 minutes the
tolerance is minus one.

**The arithmetic behind both was wrong twice before it was re-derived at the line** — once counting
one of `Receive`'s two pre-gate `exchangeDeadline` arms and dropping the spoken-check gate entirely,
once restoring the gate and miscounting the arms as three. `Receive` arms **four** deadlines, two
before the gate. Neither error was visible to any test because the number lived in prose; two guards
now hold it, and they are deliberately in different packages — the arm-population scan where
`Receive` lives, the nesting inequality where the hop budget does.

`recorded` 201 → 203.

## P08.S04b — the termination object *(v1.117.285–.287)*

Two rows, tier 1, and the second is the more instructive by some distance.

**`a-termination-state-is-not-signed`.** `state` leaves the preimage. Every other axis stays valid —
version, convener, roster commitment, signature — so a `completed` object flips to `declined` and
verifies perfectly. Which of two things happened is the entire content of the artifact.

**`a-termination-binds-no-proceeding`, and it first replayed GREEN.** The `RosterHash` comparison is
disabled; that one field is the whole binding, committing to the id, DocHash, intent, deadline and
every roster entry. Removing it should let one convener's decline end any ceremony they are party
to — and the check did not notice, because **both of its refusal arms used a DIFFERENT convener**,
so `Verify`'s convener-fingerprint test caught them and the binding was never reached.

**An arm two predicates can satisfy cannot say which one is missing.** That is the second time this
exact hole was found in one day — the first was /pending 324's exit door, where `refuses()` fired
through `hasUnrostered()` and hid a missing `disagrees()`. Different file, different author's
reasoning, same shape. Worth naming as a class: when a guard has several independent reasons to
refuse, every red proof must be aimed at a case only ONE of them covers.

The arm that fixes it is one convener running two proceedings — which is also the real attack, and
`invitation_test.go` had already named it: *"Alice carried a lease, Bob carried a deed of sale at a
different price."* It carries two setup assertions — the conveners match, the commitments differ —
so it cannot silently decay back into the arms it was added to complement.

`recorded` 203 → 205.

## /pending 333 — the file changed on disk (v1.117.289)

Four rows for one defect, because it has four independent ways to be wrong and three of them
stay green while the other is broken.

`disk-change-goes-unreported` is the defect **as shipped**: a `document` recorded `path` and
`data` and nothing describing the file — no size, no mtime, no hash — so the bytes read at open
were served forever and a browser reload re-fetched the same copy. Measured on the reporter's
machine before the fix: `nib` opened the file at 11:03:17, the file was rewritten at 11:04:03,
and the stale copy was still being served twenty hours later.

`stale-save-overwrites-changed-file` is the one that matters, and it is a **placement** row
rather than a detection row. Its patch does not break the detector; it moves the refusal to
after `atomicfile.WriteDurable`. The route still answers 412 and the banner still appears, and
the user's file is already gone. **A status-only assertion cannot tell those two apart** — both
spellings return 412 — so the check reads the bytes on disk. This is the general shape worth
carrying: where a guard protects a destructive act, the assertion has to be on the thing being
protected, never on the guard's own report.

`save-never-rerecords-its-own-write` is the row nobody writes. `WriteDurable` renames into
place, so the inode and mtime both change on every save Nib performs; omit the re-stamp and the
document reports "changed on disk" against Nib's own write, forever, and every save after the
first is refused. **Every other row here stays green under that patch** — the feature would have
shipped permanently armed, which trains the user to ignore the banner and costs more than the
bug it warns about.

`mtime-touch-reported-as-a-change` guards the direction a detector is never tested in. Its
patch makes `diskChanged` answer on identity, size and mtime without comparing content, so a
bare `touch` or a byte-identical rewrite reports as changed. The failure is a **false
statement** — "This file has changed on disk" is untrue of identical bytes — and a detector
that answers too eagerly puts a banner over every document, which is the same outcome as no
banner at all.

**Two findings came from the mutation pass rather than from review**, and both are about the
probe rather than the code. A mutation labelled "the field is ignored" turned out to reach only
one of that line's **two** writers, and the survivor exposed a real hole: the background
re-check — the function that handles the reported scenario, a file rewritten while Nib is not
in front — had no coverage at all. And in tier 3 the harness's own dialog handler defaults to
**accept**, so a locally registered `page.once` lost the race: the test written to prove Save
declines an overwrite was quietly accepting it, and reported the file as overwritten.

`recorded` 205 → 209.

## P08.S05a — the receipt stops lying (v1.117.290)

Five rows: two on the wire, two on the ordering, one that crosses a process boundary. They split
that way because the ordering can be right while nobody believes it, and the wire can be right
while the write never happens.

`received-write-runs-after-the-ack` is the defect **as shipped**: `saveReceived` ran from
`serveOneSession`, *after* `ReceiveDocument` had already written `ackOK`, best-effort and
returning nothing. A party whose disk failed was recorded as delivered, never retried and never
told — and the function's own doc comment said the sender *"will not send it again"*. The row
asserts **routing** rather than comparing the text of each site (ADR-009), because the failure it
prevents is a *new* call site added without the ordering, and a test that checked the two known
sites for agreement would say nothing about a third.

`failed-write-is-acknowledged-as-ok` is the behavioural half, and it is a separate row because the
write can be in the right place and still not be believed: `Accept` swallows the error, `ackOK`
goes out anyway. Its fixture makes `~/nib` a **file** so `MkdirAll` refuses deterministically —
a chmod-based fixture is a no-op for root and on Windows, which is the shape `ocrfonts_test.go`
already pays for.

`not-stored-is-encoded-as-a-decline` guards the wire word. Without its own byte, a disk failure
reaches the sender either as nothing (an EOF read as a transport fault) or as `ackDeclined` —
a false statement about a person who said yes, which is exactly the collapse `ackTimedOut` was
added to undo one gate earlier.

`speaks-named-refusals-is-an-equality` is `/pending 338` and it is the only **latent** row of the
five: `SpeaksNamedRefusals` was `c.Proto == alpn2`, so a third ALPN would negotiate fine and then
be denied the capability it introduced. The reason it is worth a row rather than a comment is what
the existing guard would have done with it — `TestAnOlderPeerGetsTheBehaviourItExpects` enumerates
protocols that must **not** speak, and every one of them is older, so a newer version failing the
predicate reads to that test as correct. The ALPN *set* is pinned hard; the capability predicate
was pinned by nothing.

`transfer-lands-nothing-on-the-receiver` is tier 6, and it exists because **the transfer route had
no harness coverage above tier 1 at all**. `/api/session/send` and the armed `mode:"receive"` path
were driven only inside one process, sharing one vault and one identity, so *"the document reaches
the other machine and lands on its disk"* was asserted by nothing that crosses a process boundary.
With the receiver's write forced to fail, the sender learns it over both transports, through two
real binaries. **The first draft of this paragraph claimed the sentence quoted from the wire was
"the message the user actually sees", and it was not** — `ErrNotStored` fell through to a 502 that
the browser toasted as *"could not send: …"*, the same sentence a dead peer produces, which is the
mirror of the defect the byte was added to fix. The slice review caught it; `sendResult` now
carries `notStored` and the UI has its own sentence.

Two further rows came from the slice's own **review**, and both are the same lesson pointing in
opposite directions.

`named-refusals-granted-to-an-unknown-protocol` is the fail-OPEN the floor introduced. Turning the
equality into `rank >= protoRank(alpn2)` looks strictly safer and is not: the threshold is looked
up in the same list, so with `alpn2` absent it ranks 0 and admits **everyone**, sending two-byte
refusals to `nib/1` peers. Guarding only the *peer* was not enough either — the test that drove it
out removed `alpn2` and a peer negotiating `alpn` still passed, ranking 1 against a threshold of 0.

`not-stored-reported-as-could-not-send` is the mirror of the defect the byte was added to fix.
`ackNotStored` stops a disk failure being reported as a decline; without its own field at the HTTP
boundary it is reported as *"could not send"* instead — the sentence a dead peer produces. Same
collapse, other direction, one layer up. `sendResult`'s own comment said *"four outcomes, because
the server publishes three booleans and reading fewer than all of them reports one of the others"*,
and the slice made that five and four while leaving the comment at three.

`recorded` 209 → 216.

## P08.S05b — Convene reserves a delivery term (v1.117.292)

Four rows, and the slice needed every one of them because **nothing else in the tree can see this
change**. The nearest convene fixture sits twenty hours from the reservation boundary, and the only
`ErrDeadlineTooTight` case uses half a hop budget — refused with the delivery term and refused
without it. The whole slice could have been reverted with every pre-existing assertion still green.

`delivery-round-is-not-reserved` is the defect **as shipped**: `Convene` reserved `hops × HopBudget`
and nothing for the round that follows the last hop, so a ceremony could be admitted whose own
document could not be delivered inside the deadline the user set. The row's boundary is the arm that
carries the slice — enough for the hops and one second more must now be refused.

`delivery-budget-defaults-to-zero` exists because **a mutation found a hole the first cut shipped
with**. Deleting the zero-refusal guard for `DeliveryBudget` left the entire suite green: every
fixture supplies a budget, so nothing asked what happens when a caller does not. A two-arm rule
covered on one arm, and a duration whose zero means "everything fits" is the rule switched off.

`delivery-refusal-folds-both-terms-into-one` and `sitting-warning-folds-the-round-into-attention`
guard the two sentences a convener actually reads. Folding the delivery reservation into a single
"N hops need X" total makes both of them arithmetically false — X stops being hop time, and a reader
who multiplies the warning's own two figures no longer arrives at the total it prints.

**A note on how these rows were made honest**, because the first cut of two of them was not. Every
fixture in the tree sets `DeliveryBudget == HopBudget`, so a reservation reading the *wrong field*
passed all four rows — `hops × HopBudget` and `hops × DeliveryBudget` are the same number. The
slice's own review caught it. The three affected tests now set the delivery budget to **half** the
hop budget, which is also the direction P08.S05d moves in, and the wrong-field mutation kills two of
them. A red proof over fixtures that cannot distinguish the two values is a red proof of the wrong
thing.

`recorded` 216 → 220.

## /pending 343 — P08.S02's central property gets a runtime reader *(v1.117.300)*

Four rows, tier 1. The slice they belong to shipped without any: **`P08.S02` had zero red-proof
rows** (every `S02` row in this directory is *P07*.S02), and its property — the contribution reaches
disk before it reaches the wire — was held by one source scan.

**Three named searches established the gap rather than asserting it.** `grep -rn '\.Store('
--include=*_test.go internal/` returned a single hit and it was a `sync/atomic` in
`internal/rendezvous`; `grep -rn 'nib/ceremonies\|ceremonies/' build/*.sh` returned nothing, so no
harness on any machine inspects the mirror; and the existing failure test injects at a test double
(`mapReDeliverer.fail`), driving `coSignExchange`'s handling and never the production error path.

`store-never-reaches-disk`, `store-swallows-the-persist-failure` and
`failed-persist-drops-the-redelivery-cache` are the three arms of the new server-side test, and each
is red on its own — the third is the one that would have been easy to get wrong in the safe-looking
direction, since not caching what you could not store is a second signature from one identity.

**`contribution-stored-after-the-frame` is the row the source scan cannot be.** It restores the
pre-S02 ordering and leaves `Store` itself untouched, so `l3_test.go` still finds
`persistContribution(` in its body and still reports green. The document is identical, both
signatures verify, and the cache is populated. Only the ORDER moved — and an order is visible only
from the far side of the wire, which is why the reader is a real two-transport exchange with the
store held open while the initiator is asked whether it has anything yet.

`recorded` 220 → 224.

**A fifth arm was probed and is deliberately NOT a replayable row.** The tier-4 clause added in the
same change (`pairrepro.sh`: the receiving instance's `~/nib/ceremonies/<id>/document.pdf` is
byte-identical to the document that hop produced) was driven red three ways against a preserved work
dir — no mirror directory at all, a mirror holding no `document.pdf`, and a mirror holding *another
hop's* document — and green against the real one. It is not mechanised here because a tier-4 row
costs a full two-transport four-party run per replay, and `--all` is already a minutes-long job. The
probe is recorded rather than the row, which is what "not every row is mechanised" means in
`CONTRIBUTING.md`.

**That clause also found tier 4 red, at HEAD, for an unrelated reason.** `pairrepro.sh -n 4`
completed its QUIC relay and then failed with `instance 2 could not arm before hop 1 (HTTP 409): a
session is already armed` — deterministically, because a party that has signed holds a 300 s
re-delivery window (`connectDeadline`) and the harness never disarmed between transports. So the
**TCP arm of the N-party relay had not been running at all**, on a harness whose contract line says
"over BOTH transports" — the same shape as /pending 344 one level up, found the same day. Fixed by
disarming every party at the end of each transport's relay; both transports now run, and the new
mirror clause fires on all six hops.

## /pending 344 — a test named "OnBothTransports" that drove one twice *(v1.117.301)*

Two rows, tier 1, and the pair is deliberate: one catches the defect the item names, the other
catches what a behavioural assertion structurally cannot see.

`TestACeremonyArmWaitsForTheCeremonyOnBothTransports` sent `Transport: "tcp"` for **both** of its
arms. The QUIC door it is named for was never entered, and P08.S04's acceptance says the figure is
"asserted on BOTH transports" — so a criterion read as met on evidence covering one. They are
genuinely different doors: a TCP ceremony arm opens an accept listener and stamps `until` in
`session.arm`; a QUIC one hands the single handshaked listener to the racing coordinator and stamps
it in `session.armCeremony`.

**`quic-arm-window-is-the-manual-bound` is the defect made visible.** Only `armCeremony` stops
asking `armWindowFor`, so a party armed over TCP still waits for the ceremony and one armed over
QUIC is disarmed after five minutes. **Measured: with this patch applied, the version of that test
at v1.117.300 passes green.** That is the whole content of the item — not that the test was weak,
but that it was blind to one of the two things its name claims.

**`arm-timer-fires-on-its-own-figure` is the discriminating one.** `runCeremonyReceive`'s deadline
stops coming from the door while both arm doors still stamp `until` correctly, so the status reports
30 days, the panel counts down from 30 days, and the loop gives up after five minutes. Every
assertion about the *reported* figure stays green — the reported figure is right; nothing fires on
it. Only the routing guard sees it, which is why the item's second half was a routing assertion and
not a third transport.

The routing guard enumerates every `until` assignment in the file rather than checking the two known
doors for agreement, because two sites agreeing says nothing about a third — and this package
acquired its second arm path exactly that way ("two arm paths living in two functions is the same
count S05d and S05e each found").

`recorded` 224 → 226.

## /pending 345 + 346 — the two failure notices nothing produced, and one nothing observed *(v1.117.302)*

Four rows, tier 1, from two items that turned out to be the same shape one door apart: a
`noteFailure` kind whose PRODUCER was never driven.

**345 was an inventory row claiming a kind that did not exist.** P08.S04a's seam inventory carries
`notice.what == "ceremony-ended"`; a named search returned nothing, and only three kinds were built.
The row was **right and the kind was owed** — its own zero-meaning says so: *"the peer is told and
the local user is not"*. `checkArrival` has two callers, and only one of them can answer a user.
`handleSessionInitiate` is an HTTP handler and returns 409; `sessionConfirmer.Confirm` runs on the
receiving side in a background arm goroutine with no response to write into. S04a gave the refusal a
named wire code, so from that slice onward the peer got a diagnosis and the local user got silence
while going on waiting for a proceeding their own machine had refused.

`refused-arrival-tells-nobody-locally` restores that state exactly, and every existing refusal test
stays green because the peer's half is untouched. `every-refusal-reads-as-ceremony-ended` is the
discriminating one: a notice IS produced, at the right moment, with a sentence — the wrong sentence.
The two kinds are two ACTIONS, so collapsing them tells a party whose counterparty pasted the wrong
document that their ceremony is finished.

**346 was the mirror image: a kind that existed and whose producer nothing drove.** Of the three,
`hop-not-mirrored` is the one where the user has SIGNED and kept nothing — the state D24 exists to
prevent. Its surface test called `noteFailure` directly and `l3_test.go` drove `mirrorHop`'s success
path only, so the notice was asserted about a call the product might never make. Its sibling
`received-not-saved` got a real producer-side reader at P08.S05a; this one did not.

`hop-not-mirrored-only-logs` puts it back to a `log.Printf` into a stderr a double-clicked launch
sends nowhere. `ordinary-cosign-raises-the-mirror-notice` guards the other direction and is the
worse failure: a sticky warning on every ordinary co-sign trains the user to dismiss the one that
means something.

**A fixture measurement worth keeping.** Driving the ended case needs a record that has genuinely
expired, and `Expires: time.Now().Add(-time.Minute)` is REFUSED — not by the clock you would expect.
`Record.Verify`'s only comparison is a future ceiling, so it says nothing; `Convene` refuses on its
hop-plus-delivery reservation (P08.S05b). `convenedExpiring` therefore convenes at a `now` three
hours ago, which is possible only because `Convene` takes `now` as an argument.

`recorded` 226 → 230.

## /pending 337 + 286 — an exemption resting on a premise that had stopped applying *(v1.117.303)*

Two rows, tier 1, both about **337**, and the item resolved without a behaviour change.

`openArrival` takes the UNCAPPED door, past both of ADR-005's bounds. The exemption was named at the
site at v1.117.298 per ADR-009, and the reason given was that the bytes have *"no other home, since
the session path installs it and nothing writes it to disk"*. **P08.S02 made that half false** —
`ceremonyID.Store` writes a ceremony arrival's mirror before the frame — while it stayed true for a
two-party co-sign, whose `persistContribution` early-returns on `ErrNoRecord`. An exemption holding
for one of its two classes is one waiting to be deleted by whoever notices.

**What actually carries it was never written down, and it is structural.** Both `openArrival` call
sites are guarded by `final != nil && !opened`: `final` is non-nil only after `serveOneSession`
returned a co-signed document, which requires the user to have accepted at the consent gate, and
`opened` admits at most ONE document per arm. So the door cannot be pumped — every increment costs a
deliberate arm plus a deliberate consent, which is more work than opening a file, not less. That is
why the exemption stands, and it is why capping was **not** built: the refusal would land on a
document the user had just signed, to bound a growth path only they can drive.

`an-arm-installs-more-than-one-arrival` asserts that clause. Nothing else in the tree fails under it
— re-delivery is idempotent, so the second install is the same bytes and every behavioural assertion
still holds; only how many times it lands changes. `arrival-exemption-stops-being-named` is the
ADR-009 half, and it goes red for a *comment* on purpose: an exemption nobody can find is
indistinguishable from an oversight, and this item exists because the naming that was there had gone
quietly stale.

**The residual is recorded rather than hidden**: count is unbounded through this door for a user who
arms, consents and never closes a tab. Accepted, visibly — the tabs are on screen.

**286 got no rows and no code, and its two premises were corrected in place.** Measured: the block is
280×84pt and the client sizes text at `min(lineH*0.7, 9pt)`, so the cap binds through 6 lines — 7 is
7.60pt, 8 is 6.65pt, 10 is 5.32pt, 12 is 4.43pt. So wrapping would **not** "change what every
existing block renders as": anything at or under 6 lines is pixel-identical, and everything wider
than one line is refused today and renders nothing. And the real obstacle is not that the bounds
cannot see the block — every call site can — it is that the constraint becomes **joint**: four
fields competing for one vertical budget, so `convene` must refuse a combination and say which field
to cut. That sentence is the design. The dangling `S07` citation the entry named is removed.

`recorded` 230 → 232.

## /pending 341 — a convened document brought into line with its own file *(v1.117.304)*

Two rows, tier 1, in opposite directions — which is what makes this a narrowing of the freeze rather
than a hole in it.

The state: the commit doors mutate memory only, so a convened document's bytes reach disk at exactly
one place, `~/nib/ceremonies/<id>/document.pdf`. The file in the user's own matter folder stays the
PRE-CEREMONY draft forever — unsigned, carrying no record — and `nib verify` on that file reports
"unsigned" about a document under a live ceremony. Saving to fix that answered 409 **even with no
divergence at all**.

**The exemption is byte-identity, and it is the refusal's own sentence read literally.**
`ceremonyFreeze` says Nib "will not write DIFFERENT bytes over it". Bytes equal to the document under
ceremony are definitionally not different, and writing them cannot change what any party was invited
to sign. Nothing else the freeze protects moves: the two mutation doors never reach this branch,
because they change the document by construction.

**It is reachable rather than decorative, and that measurement is what the item turned on.** An
earlier reading of this path assumed `/api/save` always receives pdf.js `saveDocument()` output,
which re-serialises and would never compare equal — making the exemption a dead branch and the whole
option worthless. That is wrong: `bakedBytes` calls `getData()`, the RAW loaded bytes, whenever
`annotationStorage` is empty, and only `saveDocument()` when there are real edits. So the exemption
fires exactly in the case the user means by pressing Save on a document they have not touched, and
cannot fire once they have edited it.

`convened-save-refuses-its-own-bytes` restores the shipped defect. `convened-save-accepts-any-bytes`
inverts the condition so the freeze never runs on that route at all — a test that only checked the
exemption would pass against it, which is why both rows exist.

`recorded` 232 → 234.

## The full replay, run *(2026-08-31, sweep 22)*

`./build/redproof.sh --all` over all 234 rows — the first full replay since v1.117.156, ~24 minutes.
**227 re-proved. Seven did not**, and the useful half is which:

**One was caused by that sweep and re-recorded the same hour.**
`consent-gate-skips-the-arrival-check` patches `sessionConfirmer.Confirm`, and /pending 345 added a
line inside the patch's own hunk. Re-recorded against the new body and re-proved green.

**Six were already stale before the sweep began**, and that is a measurement rather than an
assumption: each patch was dry-run against an export of `762e66d`, the pre-sweep HEAD, and six failed
there too. They are filed as `/pending 348` with their files and checks. **Every one of their named
checks still exists and still passes** — so this is not six deleted guards, it is six recorded
defects the code has moved out from under, and only one (`mirror-record-written-first`) was already
known to this ledger.

**Two lessons worth keeping.**

The first is about guessing. Two of the seven were assumed to be the sweep's doing on the reasonable
grounds that it had edited `session.go` and `server.go`; the dry-run against the old tree said one
was and `save-route-escapes-the-ceremony-freeze` was not. Attribution here is one command, and it is
the difference between fixing a regression and adopting somebody else's.

The second is about the gate. Nothing runs `--all` routinely, and `verify_test.go`'s count guard
names that blind spot in its own comment — it can see a row that DISAPPEARS and not one that no
longer re-proves. So a row goes stale silently the moment an unrelated slice touches its file, and
five did without anybody hearing.

## /pending 348 — six stale rows re-recorded, and the cheap half of `--all` becomes a gate *(v1.117.306)*

The six rows the full replay found stale are re-recorded, **one at a time, each re-derived from what
its check asserts** rather than regenerated. No production code changed; only the six patches did.
What staled each is worth keeping, because no two were the same:

- **`re-delivery-re-signs`** — /pending 320 gave `Cached` an error return, so the old patch's
  10-line removal no longer matched. Re-recorded MINIMALLY: the cache is consulted and its answer
  discarded. Deleting the whole block would also have driven the unreadable-store branch, which has
  its own live row (`unreadable-record-reads-as-never-signed`) — two faults in one patch.
- **`hop-starts-inside-its-own-budget`** — the inline comparison moved into the shared
  `recordOutlivesBudget`. Re-recorded at `checkCeremonyDeadline`'s own CALL SITE (budget → 0), which
  is "compared against `now` alone" exactly; the helper is shared with `checkArrival`, which passes 0
  deliberately and must not move.
- **`save-route-escapes-the-ceremony-freeze`** — /pending 341 wrapped the freeze in a byte-identity
  test. Re-recorded to delete the CALL and keep the prose, which is this row's own second purpose:
  `handleSave` names both commit doors only inside a comment, so if the guard ever stops stripping
  comments this row goes green and says so.
- **`label-never-overrides-the-constant`** — pure comment drift. The code is byte-identical; a
  `/pending 317` paragraph was inserted into the patch's context.
- **`mirror-record-written-first`** — /pending 321 put the sidecar unlink inside the `pdf != nil`
  block. Re-recorded by hoisting the `record.json` write instead, leaving that unlink alone.
- **`refusal-decode-defines-an-unknown-code`** — two codes (13, 14) were appended to `errorForCode`
  after the row was written.

**Four of the six were staled by a change that did not alter the behaviour under test at all** —
a comment, a moved helper, a wrapped condition, an appended case. That is the argument for the gate.

**`TestEveryRedProofStillApplies` is that gate**, and it is deliberately only the cheap half.
`redproof.sh`'s header separates the two failures — the patch did not apply, versus the patch applied
and the check still passed — and this one sees only the first. **Measured: 0.24 s against 24 minutes,
for the failure that accounted for six of the seven.** A gate nobody runs catches nothing, which is
what the previous arrangement demonstrated over several weeks. `--all` stays the sweep's job for the
second failure, which no cheap check can reach.

It reads the WORKING TREE rather than an export of HEAD, and that is where it is stricter than the
harness: it fails on the edit that stales the row, before the commit, when the fix is a context
refresh rather than archaeology.

`unrelated-edit-stales-a-red-proof` is its row, and the defect is one comment line — because that is
literally how `label-never-overrides-the-constant` died.

`recorded` 234 → 235.

## /pending 286 — a block line wraps, and the four ceilings become one budget *(v1.117.307)*

Four rows, tier 1. The item sat open from P07.S07 because the one number it needed was not
derivable from code: **how small may text be on a signed legal instrument.** Dan set the floor on
2026-09-01 at 8 lines, which is 6.65pt on a 280x84pt block, and everything else followed from it.

**What changed.** `AppearanceLines` emitted one entry per field and nothing wrapped, so the recital,
the party's label, their capacity and the pinned peer's label each had a ONE-LINE ceiling. A recital
of ordinary length for a lease was refused outright — and the alternative that refusal protected
against was `ctx.fillText` with no `maxWidth` clipping it at the canvas edge. Lines wrap now, bounded
by the block's height.

**The four ceilings became one joint budget, and that is the substantive part.** Once lines wrap the
fields compete for one vertical space: a recital that fits beside a short capacity does not fit
beside a long one. So the question can only be asked of a whole block, per signing party.
`block-bound-is-per-field-not-joint` is the discriminating row — the bound still exists, still
refuses an over-long capacity, still names the party, and the only assertion that fails is the one
about two values that each fit ALONE. That property could not be stated before this change.

`block-height-floor-raised` guards the number itself, and it asserts the PROPERTY rather than
restating the constant: at 20 lines a block renders at 2.66pt, and a test comparing the constant to
itself would pass. `block-line-never-wraps` is the feature half — with it applied every block is
exactly what it was before.

**`unbreakable-token-wastes-a-line` records a real bug the tests found during the build.** A greedy
"break at the last space" finds the space after the PREFIX on a long unbroken token, so
`Signer: ZZZZ…` broke after `Signer:` and spent a whole line on seven characters. On a block whose
height IS the legibility budget, a wasted line is a smaller font for every other line — a
cosmetic-looking wrap choice spending the thing the floor exists to protect. Fixed with a half-line
floor on the word-boundary preference.

**Compatibility is asserted, not argued**: `TestAShortBlockIsUnchangedByWrapping` pins an ordinary
manual block line-for-line, because the item's case rested on the measurement that anything at or
under six lines renders identically.

**One existing row had to grow.** `record-verify-bounds-only-the-convener` removes the text caps
from `Record.Verify` to show they bind the convener and nobody else; `checkBlocksFit` is now a third
such cap, so removing only the original two left the height rule refusing a 5000-rune intent on its
own and the check stayed green. A row whose defect the code has grown past has to grow with it or
quietly stop being a proof.

`recorded` 235 → 239.

## /pending 347 — the reader scan learns to see the package that publishes most *(v1.117.308)*

Two rows, tier 1. The scan covered ten packages and **not `internal/server`**, which is the one that
publishes the most JSON to the web client — and the entry's own warning was that the obvious fix
would be a vacuous green.

**It was.** Adding the package to `observablePackages` and changing nothing else passes while
discovering **zero** shapes from it: `discoverObservables` requires an exported type, and
`internal/server` has exactly one exported struct in the tree (`Server`, no json tags) against 54
unexported json-tagged ones. `server-shapes-discover-nothing` is that state, and the global floor
cannot catch it — 33 shapes from the other ten packages clears "at least twenty" comfortably — so
the fix carries a floor that names the package.

**Three relaxations, each because the export rule is a proxy for something untrue here**: the type
may be unexported, an unexported FIELD is skipped rather than disqualifying the shape (it cannot be
serialised, so it is not published), and the shape need not be RETURNED by an exported function —
nothing returns these, `writeJSON` consumes them in place.

**The reader match had to change too, and the first cut of that was wrong.** `.field` is how a Go
reader consumes a response; a JavaScript reader builds a REQUEST as an object literal, often with
shorthand — `JSON.stringify({ fingerprint, intent })` contains no `.intent` anywhere. Held to the
strict form the scan called every request field unread. The first fix keyed the relaxation on the
shape's PACKAGE and produced a false finding on `pdfops.WatermarkStyle`, which reaches the client
inside a server shape; it is keyed on the READER's language instead, which is what actually decides
the idiom.

**`embedded-shape-is-never-discovered` records a shape that had been invisible for the life of this
scan.** `pdfops.WatermarkStyle` is exported and json-tagged and is only ever a parameter, so the
return test never admitted it — while its four fields reach the browser inside `server.watermarkParam`.
Embedding into a published shape IS publication. The cross-package embed check was wrong in the same
area and is now deferred until every package is discovered, because it was asking whether an embedded
type was discovered *in the same package* and reporting two false findings when the answer was
"yes, under its own key".

**What the extension found, which is the point of having built it.** 56 shapes across 10 packages →
**88 across 11**. Fourteen fields published with no reader: eleven are one fact — P06 has not built
the ceremony surface, so `/api/ceremony/convene`, `/api/ceremony/accept` and the ceremonies list have
zero references in `web/app.js` — and they are parked BY NAME rather than by a wildcard, so each one
fails again when P06 lands without using it. Three are real and filed: `/pending 349` (the D19
diagnosis, published for a waiting UI that never reads it — the blank wait its own doc says it
prevents) and `/pending 350` (`updateResponse.Managed`, consumed by the handler before it is written
out).

`recorded` 239 → 241.

## /pending 349 + 350 — the two unread fields the extended scan found, dispositioned *(v1.117.309)*

Four rows. Both items came out of `/pending 347`'s first pass over `internal/server`, and they close
in opposite directions: one field gained the reader it was built for, the other lost its place on
the wire.

**349 — the D19 diagnosis.** `sessionStatus.diagnosis` has been computed since P05.S11 and its own
doc states the purpose: *"so the polling UI shows why nothing has connected yet, rather than a blank
wait"*. Nothing read it. `reflectDiagnosis` now renders the summary in the wait view, puts the
technical detail behind a disclosure — D19's presentation pin, stated at `diagnosisResponse` — and
branches on `cause` for tone, because "the other person hasn't opened their side yet" is the
ordinary early state of every ceremony arm and dressing it as a fault teaches the user to ignore the
line that will one day say the rendezvous is unreachable. `role="status"`, never `alert`: assertive
is reserved for the signature-loss notice, and a line that updates every 1.5 s while somebody waits
is the last place to spend it.

`diagnosis-outlives-its-reason` is the discriminating row of that pair. The sentence appears, the
disclosure works, every assertion about presentation passes — and the line never goes away, because
the reader took `reflectNotice`'s never-clear rule by analogy. **The analogy is the defect**: a
notice is a sticky record of something that already failed, a diagnosis is live state, and a live
explanation left up after its condition passes is worse than none.

**A note on how it stayed unread for so long.** It was parked in
`test/jsdom/published.test.mjs`'s known-unread list with the reason *"rendered by P06 (the ceremony
panel), unbuilt"* — and that reason was **wrong rather than stale**. The surface the field's own doc
names, "the polling UI", has existed since P05.S11, the slice that added the field. Nothing was
waiting on P06. A plausible phase to blame is what kept anyone from looking.

**350 — `updateResponse.Managed`.** Consumed by `assetURL` inside its own handler, before the
response is written, and read by nothing at the far end. It is a local now. The JS scan had this one
parked too, with the argument already written out — *"a field consumed by the code that sets it has
no consumer at the far end, which is the whole property here"* — sitting as a park rather than as a
fix. `update-republishes-the-managed-flag` is also the proof that the Go scan now covers this
package: before /pending 347, re-adding a published-and-unread field to `internal/server` failed
nothing on the Go side.

**`fixed-park-is-never-noticed` closes the gap that let both sit.** The Go scan had only the
missing-field arm; the JS one has had the was-it-fixed arm since it was written, and it is what told
this sweep that `diagnosis` had gained a reader. Scoped to `/pending`-referencing entries, because
the matcher is deliberately loose and a loose matcher is safe in the direction the main loop uses it
and unsafe in this one — unscoped it flagged three, and two were correct as they stood.

`recorded` 241 → 245.

**One of the four was recorded INVALID and caught on its first replay**, which is the thing this
harness is for. `fixed-park-is-never-noticed`'s patch was written as
`if true || !strings.Contains(why, "/pending")`, meaning `continue` for every entry — so it did not
unscope the arm, it **disabled** it, and a disabled arm reports nothing and the check passes. The
row said "still goes red against its own defect" and the replay answered `ok  nib  0.071s`, which is
`redproof.sh`'s second failure mode: *the patch applied and the check still PASSED*. Re-recorded as
`if false && …`, which leaves the arm running over every entry, and it now flags the two design
parks the scoping exists to spare.

The lesson is narrow and worth keeping: **a mutation that turns a guard OFF is not the same as one
that makes the guard WRONG**, and only the second proves anything. The first is indistinguishable
from deleting the check, which is the case `EXPECT` was added to catch and which caught this.

## /pending 20 — the flood cap gets room for D15's mapping refresh *(v1.117.312)*

Three rows, tier 1. The item sat for four sweeps on a blocker that was wrong: *"it adds a security
constant to a DoS bound, which DISPOSE puts on Dan's side"*. It adds no constant.

**What the grill found.** `CandidateGate` refuses eviction in as many words, and its stated reason is
DISTINCT VICTIMS — *"evicting to make room would keep the set at eight while letting the race punch
at sixteen distinct victims over its life"*. A refreshed NAT mapping is the **same host** at a new
port, so replacing costs no victim. And packet VOLUME is bounded separately and independently:
`punchBudget` is per (hop, side) and its own doc says it *"deliberately does NOT reset on candidate
churn — a refreshed S07 mapping is a new candidate spending the SAME 3,000 faster, which is
correct"*. Both bounds hold exactly, so the fix is a replacement keyed on (IP, transport) and there
is no number for anybody to choose.

**The publish half was not a blocker either**, which two earlier passes had assumed. `publishWhenSlow`
says *"each republish re-reads the mapper's CURRENT endpoints"* — the moved address arrives at the
peer on the next cycle. The gate was the only thing in the way, and that comment names the item.

`moved-port-refused-at-a-full-gate` is the shipped defect. `moved-port-grows-the-candidate-set` is
the one that keeps the fix honest: the move is admitted, so every assertion about the refreshed
mapping getting in stays green, and only a length assertion sees the set pass the cap.

**`one-host-two-transports-folded-together` is a hole the mutation pass found twice.** Dropping the
transport term from `heldOnSameHost` left the entire package green, because every endpoint here is
built through `ep()`, which is TCP-only. The arm added to close that then put both transports on the
**same port**, where the port-differs term shielded the transport term — so the new test was green
under the mutation it was written for. Two transports on one host are two sockets and therefore two
ports; that is both the realistic shape and the only fixture that exercises the rule.

**One term is recorded as unreachable rather than claimed as covered.** The `port != port` clause
cannot be reached — an identical endpoint is caught upstream by `seen` and counted as a re-offer —
so removing it is a mutation nothing goes red for. It stays, with that written at the function,
because the contract is "the same host at a DIFFERENT port" and a reader should not have to
reconstruct the upstream guard to know it.

`recorded` 245 → 248.

## P08.S05e — the end state gets a producer, and the harness stops passing on the wrong 409 *(v1.117.322)*

Five rows, tier 1. The slice's remaining task was filed as harness-only and **the harness was not the
defect** — it was the thing that had been hiding one.

**The clause was green on a failure.** `decline_round` asserted that hop 3 returns 409, and it did:
`/api/session/initiate` answers 409 for **five** different things, and four of them mean nobody
refused anything. No watcher answered the spoken check, so both ends sat on the words,
`ErrVerificationTimedOut` fired, and the run read the resulting 409 as a person saying no — then
failed one line later on a termination that was never written because no decline ever happened.
Reading the response body is what settled it: `{"error":"the safety words went unconfirmed on the
other side in time, so nothing was signed"}`. The clause now drives the spoken check on both ends and
matches the refusal's own sentence. Probed three ways: watchers removed (the state at HEAD), party 4
accepting instead, and — the one that proves the body match earns its place — words confirmed on both
ends with the consent gate never answered, where the words assertion passes and only the body match
fires.

**Then the real decline exposed the slice's own defect, and it cost 300 seconds.** The round walked
the party that had just declined. That machine's `declineCeremony` prunes this ceremony's stored
invitation, so it has nothing left to arm a delivery rendezvous with, and a party that refuses at its
consent gate returns from `coSignExchange` 48 lines before `rd.Store`, so it holds no mirror for the
arrival gate to check an attestation against. The leg cannot succeed at either end. Measured:
`tried 0 address(es), none answered as the pinned peer: context deadline exceeded`, once per round
and once per re-run. The clause runs in **6 s** now instead of 360.

`ender-skip-catches-every-party` is the row that keeps the fix from becoming the worse bug. The walk
test was first written with a two-party roster — the convener plus the ender — where "skip the ender"
and "skip everybody" produce identical output, so weakening the guard to `if ender != ""` left it
green. That mutation is the round reporting success having delivered to nobody, which is C06's
telling half silenced by the change meant to make it cheap. Three parties now: the ender, a signer
who did not end it, and one an earlier run already acknowledged.

`end-state-anchored-on-the-file-beside-it` is the review's find and the one with teeth.
`checkDeliveredPayload` is a gate that REFUSES, and it verified an arriving attestation against the
`record.json` sitting in the directory it was about to write into — the anchor `ReadTermination`'s
own doc forbids by name, and `LoadState` restates the rule exactly where it deliberately takes the
weaker one: *"a gate that REFUSES on a termination must anchor on the document or the invitation
instead, because a planted matching pair verifies against itself."* The consequence is durable and
silent: a false `Ended` makes `rearmDeliveries` skip the ceremony forever, so the party never gets
their real copy. It binds through `cer.inv.MatchesRecord` now — the same anchor the sibling gate one
function up already used, which is what makes this an ADR-009 finding rather than a missing idea.

**Two rows exist because a claim in a doc comment was checked.** `deliveryOutcome.Skipped`'s own
text said `Delivered` distinguishes its two meanings and nothing asserted it
(`acknowledged-skip-stops-reporting-delivered`); `markEndedBy` overwrote unconditionally while its
companion `ceremony.WriteTermination` is deliberately write-once, and nothing on the arrival path
refuses a hop because a proceeding has ended — so a second decline was reachable and would move the
marker to the newer decliner while the round went back to walking the first
(`ender-marker-moves-on-a-second-decline`).

**Two mutations are recorded as unreachable rather than covered.** `endedBy`'s `if rerr != nil`
branch changes nothing observable — a nil read yields `""` either way. And an earlier cut folded case
at BOTH the read and the write, which made each unfalsifiable: either alone gives the same answer, so
removing one left every test green. One fold was deleted rather than both being claimed as covered,
and the case rule is now held where production actually depends on it, at the walk's `EqualFold`.

**A vacuous fixture, found by the mutation and not by reading it.** The case clause used
`"11"+"22"…` as a fingerprint, and `strings.ToUpper` of an all-digit string is the identity — so the
fold under test could be deleted with the clause still green. Hex letters now.

**Two tiers were RED at HEAD and this slice closed both.** Tier 2 since v1.117.320:
`deliveryOutcome` was in neither the `PUBLISHED` nor the `EXCLUDED` table of the reader scan, so
S05g closed over a red tier — the fifth shape that scan has caught entered nowhere, and the second
time it was red across a slice close. Tier 4 at N=3 since the same commit: S05g's recovery clause
picks its injected victim dynamically (`victim=3`, or `2` when `N<4`) while the witness party whose
file must not be rewritten was hard-coded to instance 2, so at N=3 the witness IS the victim, whose
`signed/` directory the injection had just wiped. Only N=2/4/9 are ever run, so nothing had asked.
N=3 passes for the first time.

`recorded` 248 → 253.

## P08.S05h — D33's unit, and the tier a delivery leg never had *(v1.117.323)*

Two new rows and three re-recorded ones. The slice was scoped as *measurement* — "the round's
egress budget is derived from D33 and DRIVEN, not asserted" — and the measurement it asked for
already existed and was **red**.

**`--lan -n 9` emits 78 packets destined off the link**, against a criterion that says zero. It
emits exactly 78 at v1.117.320 as well, so the regression arrived with P08's delivery arm rather
than with the slice that found it, and the plan has recorded P03's exit criterion as green since
v1.117.207. Nothing had run the shape in between: `--lan -n 9` is the only run that can see it and
it belongs to no slice's verification but this one's.

**Counting publishes found nothing, and that is the part worth keeping.** The first probe
instrumented `publishCandidates` and reported zero on every instance — because in a namespace with
no reachable DHT nothing is ever published. The packets are the **bootstrap** the publish path
performs on its way there, and a stack trace on `ensureBootstrapped` named the caller in one line:
`armForDelivery` → `publishWhenSlow` → `publishCandidates` → the bootstrap, two per party per
transport. A metric one step away from the mechanism reads as evidence of absence.

`delivery-arm-has-no-link-tier` is the shipped defect. `armForDelivery`'s own doc claimed
*"everything about the tier ladder is reused rather than rebuilt"* and tier 1 was missing at both
ends — the arm neither announced nor answered, so `cer.watchingLink` was never set and `holdDHT`
took its `ns == 0` arm; and the dial browsed nothing, so `feedCeremonyRace` never saw a
`sourceLAN` candidate and held the DHT for two seconds instead of thirty.

**What renews the hold is the convener's own announcements, and that was measured rather than
assumed.** A delivery arm never hears a browse — `browsePeers` is a pure listener and emits
nothing — so the natural worry was that the zero rested on each arm being spent inside
`lanFirstBudget`. Probed: 76, 54, 40 and 20 sightings on the four delivery hop indices of a
nine-party run. The convener announces while it dials the remaining hops, `resolve` matches those
to each arm's expected peer, and the renewal is real. The limit that survives is stated at the
site: once the hops finish the convener goes quiet, so an arm whose round runs a day later
publishes after 30 s — which is correct, because the parties are no longer demonstrably in one room.

`budget-keyed-by-ceremony-not-hop` is the other shipped defect, and it is a decision the plan had
already made. D33 reads *"3,000 packets per ~~ceremony~~ HOP"* and the code implemented the struck
form. Two guards sat on the neighbouring axes — both loops of one hop share a budget, two ceremonies
do not — and the unit is one indirection from each; the second row's own text says *"D33's unit is
per HOP"* while guarding a process-wide counter. `hopScoped(c, cer.hop)` filters the candidate
stream by hop eight lines from where the budget was taken by ceremony, so the right unit was already
in the file beside the wrong one.

**Its first mutation was a red for the wrong reason, and so was its neighbour's — the second time
after writing this paragraph.** Dropping the hop term left `strconv` unused and the package failed
to compile; re-recorded as `strconv.Itoa(0*hop)`, which compiles and is wrong. Then
`punch-budget-shared-across-every-ceremony` was re-recorded as `key := ""` with `_, _ = id, hop`,
which leaves `strconv` unused in exactly the same way — and `redproof.sh` caught it by name:
*"went red, but not for its own reason … a non-zero exit alone is also what a DELETED or
uncompilable check produces."* Re-recorded again as `key := strconv.Itoa(0)`, one entry for every
ceremony, which is what the row names.

**The lesson is narrower than "be careful with imports".** A mutation is only a proof if the tree
still builds, and the failure mode is asymmetric: a build failure looks like a pass to any harness
reading exit status, so a row recorded that way claims coverage it never had. `EXPECT` is what
separates the two, and it is the reason it exists.

**One probe is deliberately NOT a row, and the same trap then caught a row that WAS registered.**
Deleting the dial-side browse produces no packet count at all — the round has no candidate for any
party and every leg runs out its 300 s connect deadline, so a nine-party run takes forty minutes to
fail. Killed at ten. A row whose PROVE cannot terminate is worse than the prose.

`delivery-arm-has-no-link-tier` was then recorded with a mutation that removed the WHOLE
`watchLink` call — announce included — and it hit exactly that: under `--lan` the arm's announcement
is the only thing the round can discover, so the replay hung in the same place and was killed at
thirty-two minutes. **Having written the paragraph above, the row beneath it walked into the same
hole.** Re-recorded to take away the ANSWER half alone, which is what the row names anyway: the arm
stays findable, the round completes, and the egress assertion fires on a hold that never renewed.
Re-targeted at `-n 4` as well — three delivery arms move the counter, and a row nobody can afford
to replay is a row nobody replays.

`recorded` 253 → 258.

## P08.S03 / C04 — the arrival gate learns to look at the bytes *(v1.117.328)*

Three rows, tier 1. The slice closes C04's decoy clause, and everything sharp about it came from
reading what the tree already said rather than from the change itself.

**The gate never compared the document to anything.** `checkArrival` asked three questions — is
there a record and does it verify, has the deadline passed, does the roster commitment match the
invitation — and every one of them passes for a DIFFERENT document carrying the same valid record.
So a convener able to present two documents under one ceremony id had each party signing something
different while all of them claimed one proceeding, which is the substitution D29's identity pin
exists to refuse.

`DocHash` was written for exactly this: *"Every party agrees to the same bytes and a resumed hop can
prove it."* The only thing that compared it against a document was `CheckDocument`, and
`git grep CheckDocument( 4fbd279 -- '*.go'` outside tests and comments returns **nothing** —
recorded at P07.S02b as "unusable by any receiver" and left there.

**The window is narrow, measured, and the reason the second row exists.** `ContentDigest` covers
each page's `/Annots`, a visible signature adds a widget annot, and the production path signs
visibly, so from the first signature onward an honest document does not hash to its record. The
first cut of the test could not see that: both fixtures were unsigned, so making the anchor
unconditional left the whole test green. The signed arm then had to be built twice — an INVISIBLE
signature does not move the digest either, so the first version of that arm was vacuous in the same
way and needed an appearance image.

`substituted-refusal-loses-its-wire-code` is the review's find and the sharpest of the three. The
refusal was a bare `fmt.Errorf` with no sentinel, so `refusalCode` returned 0, nothing was written,
and the initiator saw `EOF` — a 502 with a D19 NETWORK cause, inviting the retry a refusal must
never invite. **And the codeless path is the threat-model path**: on an honest send the sender's own
copy of the gate refuses first, so this crosses the wire only when the sender skipped its gate,
which is the attacker. It also escaped `refusalenum_test.go`, which enumerates `internal/p2p`
sentinels and cannot see a bare error raised in `internal/ceremony`. Code 15 is frozen.

**Three claims in this slice's own new prose were wrong and the review caught all three.** That
`DocHash` had *"no reader on the signing path"* — it has one, `rosterPreimage` digests it, so
`MatchesRecord` covers it transitively; the defensible claim is "no reader that compares it against
the bytes". That the anchor could sit above `MatchesRecord` — it must sit below, or a document that
is both from another ceremony and content-mismatched loses the older refusal's wire code. And that
*"outside this window the signature chain IS the anchor"* — `DocumentHash`'s own doc records that
byte-prefix plus `AddedAfter == false` *"was measured at this slice's grill to PASS on a document
whose first page had been blacked out by the last signer"*, and nothing on the signing path reads
`AddedAfter` at all. `/pending 358` carries what that leaves open.

**And the guard shipped four commits earlier caught the author TWICE in this one slice.**
`HasSignatureBlob` was inserted between `signatureBlobPresent`'s doc block and its function, and
then `ShortHash` between `DocumentHash`'s and its own — both gluing two docs into one, both
`/pending 352`'s exact defect, both found by `/pending 352`'s own guard on its first outings. The
shape is not carelessness about comments: it is that adding an exported wrapper ABOVE the thing it
wraps reads as the natural order and puts the new function between an existing doc and its subject
every time. Worth knowing before writing the third one.

**The cost was measured, not estimated, and it is parked.** `CheckDocument` on the 4.4 MB fixture
costs 13.8 s; `DocumentHash` alone 8.7 s; `CheckRecord` — which `checkArrival` has already run —
4.8 s. The first cut used `CheckDocument` and paid the 4.8 s twice. What remains is on a request
path, which this repo's `CLAUDE.md` reserves for Dan, so it ships in its cheapest correct form with
the trade filed as `/pending 359`. It was already visible: the harness's spoken-check budget was
30 s, ~28 s of it went to the two sides' digests, and the run began failing with a timeout that was
really a cost.

`recorded` 258 → 261.

## P08.S06 — close-out: the prune moves (v1.117.330)

| Defect reintroduced | Check that fired | What it said |
| --- | --- | --- |
| `close-out-deletes-instead-of-moving` — `os.Rename` becomes `os.RemoveAll`, which is what `RemoveMirror`'s own doc invites | `TestTheCloseOutPrunePreservesThisMachinesOwnContribution`, tier 1 | "this machine's own signed contribution is GONE after the close-out … the user's signature destroyed by their own software's tidying" |
| `close-out-reaches-three-of-four-stores` — the invitee-side invitation store drops out | `TestTheCloseOutDoorReachesEveryCeremonyScopedStore`, tier 1 | "the stored invitation survived the close-out — it carries the ceremony secret, and this is the store P08.S01 found a teardown missing one door over" |
| `receipt-lets-a-derived-state-overwrite-an-attested-one` — the write-once check drops out | `TestTheReceiptKeepsTheFirstThingThisMachineObserved`, tier 1 | "a derived 'abandoned' overwrote an attested 'declined'" |
| `close-out-runs-on-a-ceremony-whose-record-did-not-verify` — the load-class guard drops out | `TestAnUnreadableCeremonyIsNeverClosedOut`, tier 1 | "a ceremony classified unparseable was closed out — its record did not verify, so its deadline is not a fact" |
| `the-grace-is-hand-copied-from-the-ceremony-ceiling` — the derivation becomes `72 * time.Hour` | `TestTheGraceIsDerivedFromTheCeremonyCeilingAndNotHandCopied`, tier 1 | "closeOutGrace's identifiers are \"time.Hour\" and none is ceremony.MaxCeremonyLife" |

**Eleven mutations were probed and ten went red; five are registered.** The eleventh is below. The four not registered
are the ones whose patch is a second spelling of a registered row — the absolute-root check moved
below the rename, the destination overwrite, and `roundIsFinished` pinned to each of `true` and
`false`. That last pair is worth naming even unregistered: **both directions had to be probed.** A
test that only checks the held case passes against a `roundIsFinished` that never returns true,
which holds every ceremony open forever — the failure that wears safety's face, and the one a
close-out test is most likely to ship with.

**Two guards caught this slice's own new code and neither was a false positive.**
`TestEveryHandRolledAtomicWriteIsDeclared` fired on `CloseOutMirror`'s `os.Rename` — correctly, and
the honest answer was the map's first exemption since v1.117.261, because this renames a
**directory** and writes no bytes, which is not the temp-file-plus-rename that guard hunts. Then
`TestEveryPublishedObservableHasANamedReader` fired **twice**: first on `ceremony.Receipt` having no
reader at all, and again on `Receipt.State` after a reader was added that never named that field.
Both were real. The first produced `ListEnded` and the `ended` half of the ceremonies route — a
receipt no surface shows preserves the contribution in secret. The second produced the write-once
rule, which is a correctness property nobody had asked for: `State` had been set by one function and
read by none, which is `CheckDocument`'s defect exactly, on a four-day-old type.

`recorded` 261 → 266.

**And the grace proof failed its first replay for a reason that was not the test.** `redproof.sh`
reported *"went red, but not for its own reason"* — the row's `EXPECT` was written as a grep
alternation, `does not reference\|none is ceremony.MaxCeremonyLife`, and the harness matches the
expectation literally. So the check went red, said exactly what it was supposed to say, and the row
was still refused. That is the harness doing precisely its job: **a non-zero exit alone is also what
a deleted or uncompilable check produces**, and it will not accept one as proof. Worth recording
because the failure looked like a stale patch and was a typo in the assertion about the assertion.

## P08.S09 — the phase close: two criteria that were driven by hand (v1.117.331)

| Defect reintroduced | Check that fired | What it said |
| --- | --- | --- |
| `a-second-ceremony-on-one-document-is-admitted` — the 409 mapping dropped | `./build/ceremonyrepro.sh` CLAUSE 10, tier 6 | "C13 second convene … a document already under a live ceremony was convened again" |
| `an-unloadable-ceremony-vanishes-from-the-live-listing` — `ListStored` filters to `LoadOK` | `./build/ceremonyrepro.sh` CLAUSE 11, tier 6 | "the damaged ceremony VANISHED from the listing rather than degrading … a ceremony Nib will not admit exists is one whose only remedy is finding and deleting the folder by hand" |

**Both rows exist because C15 was not met, and the ledger is what found it.** *"Every criterion in
this phase is driven by the multi-instance harness, not by hand"* — and C12 and C13 were driven only
inside one process, where the convener, the vault, the identity and the document registry are the
same object. Four criteria still are (C02, C06's expired and abandoned halves, C07, C14); those need
instruments that do not exist, and they are recorded in the phase ledger rather than rounded up.

`recorded` 266 → 268.

**And the phase close ran `--lan -n 4`, which had been RED for four commits.** Six off-link packets
against P03's zero. The cause was named by a stack trace on `ensureBootstrapped` —
`armForDelivery` → `publishWhenSlow` → `publishLoop` → `publishCandidates` — and attributed to a
single clause by counting packets at each clause boundary: `decline_round` 4, `close_out_round` 4,
`interrupted_hop` 4, `decoy_document` **10**, against a baseline of 4.

**The product was right.** ADR-011's hold is `lanFirstBudget` of *silence*, not forever: an arm that
hears nothing pays one budget and publishes, because a genuinely remote peer has to be reachable.
`interrupted_hop` and `decoy_document` deliberately leave a proceeding unfinished, so their arms are
parties still waiting. The reading moved above them and they are pinned below it; `/pending 363`
carries what that costs.

> **CORRECTED 2026-09-03 at `/pending 363`, and the correction is about this paragraph's evidence,
> not its conclusion.** *"`decoy_document` **10**"* above was read off a CUMULATIVE counter at each
> clause boundary, and that is not an attribution. Measured per clause across three `--lan -n 4`
> runs the same six packets split **6 / 0**, then **0 / 6**, then **0 / 6** — a hold started in one
> clause elapses in whichever clause is running thirty seconds later. So the six belong to the
> *pair*, no single clause caused them, and the budget built at 363 is over the pair for exactly
> that reason. The conclusion — the product is right and these arms are meant to publish — is
> unchanged and is what the per-clause reading confirms.

**The four commits are the lesson, not the six packets.** `decoy_document` arrived at v1.117.328
with no `--lan` run, and `CONTRIBUTING.md`'s tier-4b row still announced the harness as *"currently
RED against shipped code"* for a defect fixed four phases earlier — so a reader who ran it had been
told to expect the red. **A harness whose contract says it fails is a harness nobody reads the
output of.** ADR-011's own text records this exact shape one clause over: *"the only `--lan` run was
the two-party one, so the run that existed to prove P03's criterion was the one shape that could not
reach the defect."*

## P06.S01 — the listing leaves the lock, and is better guarded for it (v1.117.334)

| Defect reintroduced | Check that fired | What it said |
| --- | --- | --- |
| `the-ceremonies-listing-goes-back-behind-the-vault` — `requirePublicLoopback` → `requireUnlocked` | `TestTheCeremoniesListingAnswersWithTheVaultLocked`, tier 1 | "a locked read returned 0 ceremony/ies, want 1 — the panel would render an empty shelf to a user who has one" |
| `the-listing-accepts-a-cross-site-read` — the same mutation, graded on the other half | `TestTheCeremoniesListingRefusesACrossSiteRead`, tier 1 | "a cross-site GET to /api/ceremonies returned 200, want 403 … it runs a close-out sweep, which moves ceremony directories and drops vault pins" |

**One mutation, two rows, because it reintroduces two different defects** — and the second is the
one worth reading. `requireUnlocked` applies the CSRF check and the loopback-origin check to
**non-GET methods only**. That is not a discovery: `handleCeremonyInvites`' own doc comment says
*"POST because `requireUnlocked` applies CSRF and the loopback-origin check only to non-GET methods.
A GET here would be vault-gated and nothing else"* — the repo already knew, and used POST to route
around it. So this listing sat behind an auth gate with **no origin check at all**, and since
P08.S06 a GET to it runs a close-out sweep that moves ceremony directories and drops vault pins. A
state-changing side effect on a GET, reachable from any page in the user's browser, shipped by me
four commits earlier.

**So "take the route off the authentication gate" is exactly backwards as a description.** It moves
onto `requirePublicLoopback`, which is `originIsLoopback` and nothing else and which `/api/status`
already wears, and the route ends up **better** guarded than before. The lock was protecting nothing
here — the mirror is ordinary files by D29's design — while the guard that would have helped was
absent.

**And a third mutation came back GREEN, which is why the code changed.** `handleCeremonies` read the
vault and skipped the sweep on nil, and `closeOutEnded` did the same at its own head. Removing the
call-site guard left the whole suite green; removing both went red. Two guards for one rule is the
shape ADR-009 was written after finding six of, so the call-site copy is gone and the door keeps it.

**Tier 6 gained a genuinely locked instance** (CLAUSE 12), and its first cut did not work: copying
A's `.config` into a fresh home left the vault *openable*, because `unwrapSlot` tries `slot.KeyPath`
first and that is an absolute path recorded at enrol time, which reached back into A's live home.
Measured, not reasoned — the clause failed its own setup assertion with `/api/peers` answering 200.
The shape that works is to give the instance its own key and then take it away.

**Both rows failed their first replay, and neither failure was in the test.** The first had an
`EXPECT` written from the assertion I *meant* to trip rather than the one that trips first — the
same slip as `the-grace-is-hand-copied-from-the-ceremony-ceiling` two slices earlier, and
`redproof.sh` caught it both times with *"went red, but not for its own reason"*. The second was a
real defect in the test: it drove the cross-site read against a LOCKED server, so under the mutation
the same-origin read answered 401 as well and the test tripped its own setup before reaching the
cross-site assertion. **The origin refusal is a property independent of the lock and has to be
driven where the lock cannot mask it**, so it now uses an unlocked server. A row that cannot replay
is a row that was never proving what it claimed.

`recorded` 268 → 270.

## P06.S01–S03 — the panel, and the door it does not rewrite (v1.117.334–.336)

| Defect reintroduced | Check that fired | What it said |
| --- | --- | --- |
| `the-ceremonies-listing-goes-back-behind-the-vault` — `requirePublicLoopback` → `requireUnlocked` | `TestTheCeremoniesListingAnswersWithTheVaultLocked`, tier 1 | "GET /api/ceremonies returned 401, want 200" |
| `the-listing-accepts-a-cross-site-read` — the same mutation, other half | `TestTheCeremoniesListingRefusesACrossSiteRead`, tier 1 | "a cross-site GET … returned 200, want 403 … it runs a close-out sweep, which moves ceremony directories and drops vault pins" |
| `the-next-action-is-computed-from-the-roster-order` — the answer becomes the first roster entry | `TestTheNextActionComesFromTheRecordAndNotTheRosterOrder`, tier 1 | "the next action names \"The registrar\", which is the roster's FIRST entry — a non-signing convener who holds a position in the roster and none in the signing order (D22)" |

**The third row is the phase's one novel shape and its fixture is the whole assertion.** P06's
criterion asks that the panel's action be *"computed from the record by the same function the
server's L3 check uses"*, and a fixture where the roster order and the signing order agree cannot
tell a shared rule from two rules that happen to match — which is the entire failure ADR-009 exists
for. So the fixture puts a **non-signing convener at roster position 1**: every naive answer, and
every JS reimplementation over the roster, names the registrar; L3 names the first signing party.

**And the rule was already written.** `p2p.NextContributor` exists because P07.S03a built the
question form one phase early, for this criterion, recording why in its own doc: *"a predicate that
could only refuse would force that slice to retrofit a read-only query, which is two derivations of
one rule and the ADR-009 shape this gate exists to avoid."* The real risk was one level down — the
roster CONVERSION, which `l3Roster` held inline and S03 needed from a `Record` rather than an
`Invitation`. Seven copied lines would have agreed on the day they were written.

**Five mutations across the three slices came back GREEN, and each said something different.** One
removed a duplicate guard (S01's call-site nil check, which `closeOutEnded` already held). One
exposed a fixture using a value the server cannot send (`me: ''`, where `omitempty` means the key is
absent). One exposed a test driving one of `ReadStored`'s **two** `LoadUnparseable` arms. One kept
its code and moved the assertion (`ReadStored`'s pre-check buys the human SENTENCE, not the state).
And one did not compile, which is `redproof.sh`'s red-for-the-wrong-reason arriving in a hand probe
rather than in the harness.

`recorded` 270 → 271.

## P06.S04–S05 — the surfaces (v1.117.337–.338)

| Defect reintroduced | Check that fired | What it said |
| --- | --- | --- |
| `arm-progress-waits-for-the-bootstrap` — the progress view put behind the diagnosis's gate | `TestTheStatusPublishesProgressBeforeTheBootstrap`, tier 1 | "the status carries NO progress while the bootstrap has not run … under ADR-011 the bootstrap waits for the local link — thirty seconds on a LAN by lanFirstBudget. D16 says that window must never be blank, and it is the window" |

**One row, and it re-creates the state that shipped.** `sessionStatus` fills `Diagnosis` only once
`bootstrapDone` is true, and that condition is *right* — a cause computed before the DHT has had its
chance accuses the wrong tier. The defect is putting the live progress view behind the same
condition, because it would then arrive at the moment the diagnosis could already speak, which is
after the window it exists for.

**Two harness defects were found by running, not by reading, and neither is in the product.** The
new jsdom file hung the whole suite at its 900-second ceiling: `pollRecv` reschedules every 1.5 s
while armed and node's runner waits for a quiet event loop, so a file that leaves an arm up holds
the run open — and the file alone ran in ten seconds, which is what made it look like a suite
problem. And the tick was 1400 ms against a 1500 ms poll, so a state change could render one poll
late; measured when the `refused` case rendered the `silent` sentence. **A wait shorter than the
thing it waits for is a flake that passes most of the time.**

**And the published-observable scan collided with prose for the second time in one phase — this
time with the comment explaining the first collision.** It claims a reader when a `.js` file
contains `"."+field`, or mentions the json tag as a bare word. So the obvious word for the link
tier's found-state claimed a field that is genuinely unread, and so did a comment that spelled the
field out. Fixed at the wire value rather than the copy. **That a comment cannot name the field it
is about is the tell**, and it is recorded in the slice's inventory rather than worked around.

`recorded` 271 → 272.

## P06.S06 — the block shown is the block signed (v1.117.339)

| Defect reintroduced | Check that fired | What it said |
| --- | --- | --- |
| `the-quote-does-not-stamp-the-ceremony` — `StampCommitment` removed from the quote door | `TestTheQuotedBlockIsTheBlockThatGetsSigned`, tier 1 | "setup: the roster changed nothing, so this fixture cannot tell a stamp from a no-op" |

**It fails at its SETUP, and that is the design.** The fixture asserts the stamped and unstamped
blocks DIFFER before it compares them line by line — so a stamp that has become a no-op fails as a
setup failure rather than passing silently against two identical renderings. A comparison of a thing
with itself is the vacuous green this whole row is about.

**Tier 4 drives it across a real hop**, and its probe printed the shipped block verbatim:
`Signer: Nib User`, `Intent: I accept` — the *typed* intent instead of the record's recital, no
capacity, no position. That is `/pending 317` as a user would have seen it.

**Both harness defects in that clause were found by running it, and neither was in the product.**
Its first cut asked the signature for a roster POSITION, which the wire does not carry — the tag is
`[NibRoster:<version>:<hash>]`, and the position is rendered into the visible block and is derivable
from the roster, so putting it on the wire would be a second source of truth. It failed on a run
where everything was correct. And it fired on the manual two-party run, which has no roster and
correctly nothing to stamp; it is gated on `wantproc` now, with the reason at the line.

**A hand probe also reported RED on a mutation that did not compile** — `redproof.sh`'s
red-for-the-wrong-reason, arriving in a manual probe where no harness was watching for it. Re-probed
with a compiling mutation; the catch is real.

`recorded` 272 → 273.

## /pending 365 — a mutating GET keeps its cross-site door (v1.117.343)

| Defect reintroduced | Check that fired | What it said |
| --- | --- | --- |
| `a-mutating-get-loses-its-cross-site-door` — `GET /api/ceremonies` put back behind `requireUnlocked` | `TestEveryMutatingGETIsBehindTheLoopbackDoor`, tier 1 | "handleCeremonies answers GET /api/ceremonies behind requireUnlocked and calls closeOutEnded" |

**This is not a hypothetical defect — it is the one P06.S01 fixed, reintroduced verbatim.**
P08.S06 hung the close-out sweep off `GET /api/ceremonies`, and `requireUnlocked` applies the CSRF
check and the origin check to **non-GET methods only**. So a route that moves ceremony directories
and drops vault pins was reachable from any page the user had open — an `<img src>` is enough, and
the attacker never needs to read the response.

**The guard checks the DOOR, not a list of names** (ADR-009). A `declared` map would need an entry
for every legitimate mutating GET, and an entry is a claim somebody typed; `requirePublicLoopback`
in the registration is a fact the code carries, so the exemption and the fix are the same edit.

**Its predicate is a keyword set and cannot follow calls** — a write behind a helper is invisible,
and matching the call graph is the accurate-and-much-more-machinery shape /pending 365 named and
this deliberately is not. It catches the shape that actually occurred. It also carries **two setup
assertions**, because a source scan is where the vacuous green lives: the glob must match at least
twenty files and the parse must resolve at least fifteen GET registrations, so a scan that reads
nothing fails rather than reporting a clean bill over an empty set.

**A second mutation probed the other arm** — an `os.MkdirAll` typed into `handleRecent`, a GET
behind `requireUnlocked` that has nothing to do with ceremonies. Red, by name: the guard is not
special-cased to the one route that produced it.

`recorded` 273 → 274.

## /pending 364 — an accept that could not save leaves nothing behind (v1.117.344)

| Defect reintroduced | Check that fired | What it said |
| --- | --- | --- |
| `an-accept-that-failed-keeps-its-pin` — the compensating prune removed | `TestAnAcceptThatCouldNotSaveLeavesNothingBehind`, tier 1 | "after an accept that answered 500 …, arming against the convener SUCCEEDED — the pin survived in memory, so this machine accepted the invitation and told its user it had not" |
| `the-me-marker-runs-before-the-accept-succeeds` — `WriteMe` moved back above the two vault writes | `TestAnAcceptThatCouldNotSaveLeavesNothingBehind`, tier 1 | "the ceremonies listing carries e8ab… after an accept that failed: … state:\"absent\" … its folder may have been removed, or it was interrupted before anything was written" |

**The test the inventory row asked for could not be written, and finding out why is the result.**
Row S01-5 named `TestAFailedPersistFailsTheAccept` — a driver for `AddCeremonyInvitation` failing
after the pin succeeded. Both doors go through the one `Vault.save()` and the pin runs first, so an
unwritable vault **always** fails at the pin: measured, the 500 names the pin and never the
invitation. That branch is correct code no test in this tree can enter without an injection seam
the vault does not have, and saying so is worth more than a test that pretends otherwise.

**What is reachable is the mirror image of the half-state the row described, and it was live.**
Every vault mutator writes `v.contents` and then saves, so a failed save leaves the change standing
in memory. The accept answered *"nothing was accepted"*, the convener stayed pinned for the life of
the process, and `POST /api/session/arm` against them returned **200** — the machine had accepted
the invitation and told its user it had not. A restart would then take it away again.

**And a second residue, in the panel P06 had just shipped.** `WriteMe` ran before either write, and
`ListStored` lists every well-named directory without requiring a record, so a failed accept left a
ghost row reading *"its folder may have been removed, or it was interrupted"* — blaming a removal
for a ceremony the same user had just been told was never accepted. The fix is an ordering: the
marker is written last, after both writes have succeeded.

**The prune is a compensation, not a rollback**, and its own save fails too. That is fine and is the
point: memory is the state the arm door reads, and memory is what it corrects. Rolling the *vault*
back on a save failure was considered and rejected — it would discard the user's in-memory work (a
freshly enrolled identity, settings) to fix a trust grant, trading a narrow bug for data loss.

**`accept-pins-nobody` staled and was re-recorded**, on context lines only: its hunk quoted the
`WriteMe` block that moved. The defect it expresses is unchanged.

`recorded` 274 → 276.

## /pending 363 and 366 — the egress budget, and the pill's sentence (v1.117.345)

| Defect reintroduced | Check that fired | What it said |
| --- | --- | --- |
| `the-armed-pill-states-a-date-it-does-not-explain` — the old wording restored | `armedbound.test.mjs`, tier 2 | "the title states a date without saying what it bounds" |
| `the-armed-pill-loses-its-figure` — the date deleted, the sentence kept | the same file, tier 2 | "the title carries no date … the fix for the wording must not remove it" |

**366's second row is the failure the FIX could introduce**, which is why it is recorded beside the
first. The date on that pill is C05's and is the entire reason a five-minute manual bound and a
thirty-day ceremony bound are distinguishable; a wording fix that deleted it would trade one
ambiguity for the one it replaced. Both assertions are over PROSE and are worth exactly that: they
catch the wording being reverted or dropped, and cannot tell whether a reader understands it. That
ceiling is stated in the test file rather than dressed up.

**363 has no row, and the absence is argued rather than left.** Its assertion is a tier-4 namespace
run — ten minutes per replay — and the product-side defect the budget catches is the one
`delivery-arm-has-no-link-tier` already names. What is new is the WINDOW, not the defect. It was
probed by hand instead: with the allowance forced to 1 the run failed by name (*"decoy_document
emitted 6 packets off the link, over a budget of 2 for the 2 arm(s) it leaves waiting"*), and the
clean run reported `3 arm(s) waiting … 6 off-link packet(s), budget 12` with the run-wide zero
untouched.

**And a per-clause ASSERTION would have been a flake, which is the finding 363 actually produced.**
Four `--lan -n 4` runs split the same six packets `interrupted_hop`/`decoy_document` as **6/0, 0/6,
0/6, 0/6** — a hold started in one clause elapses in whichever clause is running thirty seconds
later. P08.S09's *"all six from `decoy_document`"* was an artifact of reading a cumulative counter at
clause boundaries; no single clause caused them. The budget is over the pair and the per-clause
counts print as diagnosis. That section is corrected in place above.

**Tier 2 had been RED for six slices and every one of them reported it green — the FOURTH instance
of the shape `jsdomtest.sh` documents three times in its own comment.** P06.S02 added
`ceremonypanel.test.mjs` and P06.S05 added `armprogress.test.mjs`, both on 2026-09-03, and
`Nib_expect_files` stayed at 26 against a directory of 28. So the harness exited 1 from v1.117.335
through .342 while `# fail 0` was true and printed above the failure. **It was found because this
sweep needed a 27th file and was told there were 29** — by a person needing the number, which is
luck and not a check. The literal is 29 and the comment carries the fourth instance.

`recorded` 276 → 278.

## /pending 355 — the delivery round stops when its caller does (v1.117.345)

| Defect reintroduced | Check that fired | What it said |
| --- | --- | --- |
| `the-delivery-race-ignores-its-caller` — `raceWithRendezvous` back on `context.Background()` | `TestARoundStopsWhenTheRequestThatStartedItGoesAway`, tier 1 | "the round was still running 45 s after the context that started it was cancelled" |
| `the-round-walks-into-a-dead-leg` — the loop's ctx short-circuit removed | the same test, tier 1 | "It walked into the leg anyway — minting an invitation and entering a race whose result nobody will read" |

**The two rows fail differently and that is why there are two.** In one the context is alive when
the round starts and dies inside the first leg, so the loop's own guard cannot be what ends it —
that row is red on the CLOCK, at the test's 45 s ceiling against a 2 s green, and it is the only
shape that reaches the race. In the other the context is dead at the top of the loop, and the round
used to mint the party's invitation and enter a race it knew was over, reporting *"tried 0
address(es), none answered as the pinned peer: context canceled"* — a sentence about dialling, from
a round that never dialled. **Deleting the guard leaves the first case green**, which is the whole
argument for the second row.

**The policy is per-caller and deliberately asymmetric.** The delivery round passes the request's
context, because nobody is waiting and every party it misses is reached by the re-run. A ceremony
HOP keeps `context.Background()`: a closing browser tab must not abandon a hop the far party may
already have consented to and signed. The reasoning is at the parameter, not at either call site,
because the difference between them *is* that parameter. The item left this open as a product
judgment; it was taken on the item's own reasoning rather than parked, and it is one line to
reverse.

`recorded` 278 → 280.


## P06.S07 — the panel behind the lock, and the roster on the consent screen (v1.117.346)

| Defect reintroduced | Check that fired | What it said |
| --- | --- | --- |
| `the-locked-screen-never-shows-its-ceremonies` — `applyStatus` stops filling the box | `lockedpanel.test.mjs`, tier 2 | "the ceremonies box is hidden on a locked machine that HAS a ceremony" |
| `the-locked-panel-sits-behind-the-modal` — the container moved back outside `#authOverlay` | `test/ui/lockedpanel.test.mjs`, tier 3 | "something else is on top of the locked ceremony panel at its own centre point" |
| `the-consent-screen-names-one-party` — the signer list truncated to the connected peer | `consentroster.test.mjs`, tier 2 | "the consent screen drew 1 signer row(s) for a document carrying three signatures" |
| `the-consent-screen-drops-an-invalid-signature` — the broken signature filtered out | the same file, tier 2 | "drew 2 signer row(s) for a document carrying three signatures" |

**The tier-3 row is the one that justifies its tier, and it is the shipped defect exactly.**
P06.S02 marked *"the panel renders roster, position and next action with the vault locked"* met.
`element.hidden === false` is true of a node under an `aria-modal` dialog, so a tier-2 assertion
passes over precisely the state that was wrong: the panel present, attached, and unreachable. The
assertion is `document.elementFromPoint` at the panel's own centre — what a user hits when they
click there — and the mutation moves the container back where it shipped.

**The locked state needed a second nib, because the vault has no lock route.** It unlocks once per
process and stays unlocked, and tier 3's shared server enrols a key at startup precisely so every
other file sees past the overlay. So `uirepro.sh` starts a second, never-enrolled process and
**refuses to run if its status says `ready`** — a locked-view file against an unlocked app passes
for the wrong reason, which is the fixture-shaped vacuous green this tier keeps producing.

**Clause 1's render shipped at v1.117.220 and nothing had ever driven it.** A named search over
`test/` and `build/` for `srvSigners|renderConsentSigners` returned **one** hit, and it was
`published.test.mjs` naming the function as the reader that lets `pending.signers` out of the
exclusions — a field with a reader and the reader with no driver. Writing the driver found a live
defect one function over: `loadPendingPreview`'s render loop had a `finally` and no `catch`, so a
page that would not render escaped as an unhandled rejection. The `getDocument` path above it
already ends in *"could not render the document"*; the render path ended in nothing, and the user
was left looking at a half-drawn preview on the screen where they decide whether to sign.

**And the commit gate refuted one of this slice's own lines, at five sites.** The deepdive wrote
*"every jsdom boot uses `state: 'ready'`"*. `firstrun.test.mjs` overrides it — to `setup` and
`key-missing` — and what that file asserts is the overlay's OWN content, never anything behind it.
The true statement is narrower and still decisive: nothing had rendered the ceremony PANEL locked.
Corrected in the plan, both test headers, the `applyStatus` comment and the red proof.

**Both browser tiers had been RED for six slices, and this slice found the second one.** Tier 2's
file count said 26 against 28 and tier 3's said 19 against 20 — `ceremonypanel.test.mjs` (P06.S02)
and `armprogress.test.mjs` (P06.S05), both added on 2026-09-03, neither literal bumped. So
v1.117.335 through .342 shipped with both harnesses exiting 1 while every slice reported them green,
because the verdict is in the exit status and what was read was the TAP totals, which were true.
Tier 2's own comment had recorded that shape three times; tier 3's had recorded it never, which is
what made it invisible there — **a lesson written down in the sibling harness is not a lesson this
one carries.**

`recorded` 280 → 284.

## P06.S08 — twelve outcomes, and the two that said the same thing (v1.117.347)

| Defect reintroduced | Check that fired | What it said |
| --- | --- | --- |
| `abandoned-borrows-the-unknown-sentence` — the `abandoned` arm removed | `outcomes.test.mjs`, tier 2 | "\"a-state-from-a-newer-nib\" and \"abandoned\" both render as \"No further word\"" |
| `the-ended-list-hides-when-nothing-is-live` — the early return skips the ended list again | the same file, tier 2 | "setup: no ended row rendered for completed, so nothing is being compared" |
| `a-network-cause-folds-into-an-end-state` — one D19 summary renamed to an end-state word | `TestEveryD19OutcomeSaysItsOwnThing`, tier 1 | "the end-state word \"Ran out of time\" is also the D19 summary for \"published, and it still did not connect\"" |

**The third row is the criterion's own example of failure, reproduced.** *"A screen that folds
'they declined' into 'couldn't establish a connection' fails this."*

**The check had to move to Go, and that is the finding underneath it.** D28's end-state words are
rendered by `renderEndedCeremonies` in `web/app.js`; D19's summaries are built by `classifyD19`. A
jsdom test can compare five sentences it was handed, which is a fact about its own fixture — the
vacuous green wearing a distinctness assertion. The Go test drives the classifier for every cause
and reads the other side out of the file that owns it, so both halves are the product.

**The count was wrong twice and is now enumerated rather than asserted.** The criterion says eight
outcomes and *"D19's four network causes"*; the plan review of 2026-08-18 had already corrected D19
to five, and this criterion kept the stale number. This slice's first pin then said nine, and
driving `classifyD19` says **seven summaries across five causes** — `causePeerRecordUnusable` has
two (a record refused, and a record with no address yet) and `causeMappingDependent` has two (with
and without a directly reachable IPv6 endpoint). Plus five end-state words: twelve. A sixth cause
added tomorrow joins the set without anybody bumping a literal.

**Two live defects, both found by writing the driver rather than by reading.** `abandoned` had no
arm of its own, so *"this proceeding ended in silence"* and *"this receipt was written by a newer
Nib"* were the same sentence — different facts, and only one of them is about the ceremony. And
`renderCeremonyPanel` returned early on an empty live list, so the ended list never rendered: a
machine whose ONLY ceremonies were finished showed *"No signing ceremonies on this machine yet."*
and nothing else. **ADR-012 moves a closed-out ceremony rather than deleting it precisely so the
party's own signed contribution stays findable**, and the panel hid it in exactly the case that
argument was written for.

**And the scrape that reads those words was over-inclusive on its first cut** — an 800-byte window
swept up the comment above the ternary and found eight "words" where there are five. It passed,
because an over-inclusive scrape only makes the collision check stricter; a check that passes for
the wrong reason is what this whole file is about. Bounded at the statement and filtered to the
quoted strings that begin with a capital, it reads exactly five.

`recorded` 284 → 287.

## P06.S09 — the freeze's surface, the record's label, and the name on the tab (v1.117.348)

| Defect reintroduced | Check that fired | What it said |
| --- | --- | --- |
| `the-manual-cosign-signs-a-ceremony-document` — the refusal removed from `buildCoSigned` | `TestTheManualCoSignRefusesADocumentInACeremony`, tier 1 | "the manual co-sign path signed a document that carries a ceremony record" |
| `the-ceremony-record-is-an-anonymous-attachment` — the flag never set | `TestTheCeremonyRecordIsLabelledInTheAttachmentsList`, tier 1 | "the ceremony record \"nib-ceremony.json\" is NOT marked" |
| `an-in-progress-copy-is-named-as-finished` — `arrivalDocName` ignores the ceremony again | `TestAnInProgressCopyIsNotNamedAsTheFinishedDocument`, tier 1 | "a hop of a live ceremony is named \"co-signed with Bob Landlord.pdf\"" |
| `a-refused-edit-says-nothing-about-the-ceremony` — `pageOp` back to its generic toast | `freezesurface.test.mjs`, tier 2 | "a refused edit said \"page operation failed\"" |
| `the-attachments-panel-does-not-label-the-record` — the label removed from the panel | the same file, tier 2 | "the ceremony record is listed as an anonymous embedded file" |

**The freeze was built and guarded, and nobody had asked whether it SAYS anything.** P07.S02a's
`TestEveryMutatingRouteReachesTheCeremonyFreeze` asserts the routing for the whole mutating
inventory — every route reaches the refusal. What no check asked is whether the refusal reaches the
user, and at `pageOp` it did not: `toast('page operation failed')` discarded a sentence that names
the ceremony and says where the document went. **A rule enforced at the server and swallowed at the
client is a rule the user cannot obey.** Five doors now pass the server's own words through
`errText`, which the file already had and half its call sites already used.

**`/pending 368`'s refusal is its own, and deliberately not folded into `ceremonyFreeze`.** That
guard is about mutating the document the server holds; an outbound co-sign mutates nothing — it
sends a signed copy — so stretching it would make one rule cover edits and sending both, and the
next reader would have to work out which half applied to them. Same footing, same sentence, its own
door: `buildCoSigned`, which both co-sign routes take, because a check at one of two doors is the
shape ADR-009 refuses.

**"Cannot be removed while the ceremony is live" is met by something stronger than the clause
says.** There is no attachment-removal route in the product — `add` and `extract` only — and the
path that does remove one, `POST /api/sanitize`, goes through `commitMutation` and is refused
outright. The clause is satisfied by the freeze rather than by a rule about attachments, and that is
recorded rather than dressed up as a new guard.

**And a latent tier-3 race surfaced, measured rather than guessed.** `ceremonypanel.test.mjs`
asserted `#cerPeerPick` visible immediately after the click that opens the form — but
`showCeremonyForm` calls `loadPeerPicker()` **without awaiting it**, so at that instant the div is
cleared and not yet filled. Six unrelated lines in `web/app.js` were enough to lose a race the
assertion had been winning: **2 runs red with the change against 3 green at the parent commit**, and
an instrumented run printed `{"html":"","w":167,"h":0,"display":"block","formHidden":false}` —
cleared, empty, still waiting. The product is right, and an un-awaited load is the correct shape for
a panel that must not block its own dialog; the test waits for the row now. **Bisected to the one
edit** by reverting `web/app.js` to HEAD and re-applying the label alone, rather than by reading the
diff and picking a suspect.

`recorded` 287 → 292.


## /pending 252 — the consent view stops publishing what nothing reads (v1.117.350)

| Defect reintroduced | Check that fired | What it said |
| --- | --- | --- |
| `consent-view-republishes-an-unread-peer-field` — the `Valid bool` field, json-tagged `valid`, back on `pendingView` | `TestTheConsentViewPublishesNoUnreadPeerFields`, tier 1 | "the consent view publishes 'valid' again: {"signer":"Alice",…,"valid":false,…}" |

**The row exists because neither reader scan can catch this one.** Both scans match a published
field against a bare property name in `web/app.js`. `.valid` is satisfied there by
`renderConsentSigners`'s `s.valid` — a `pendingSigner`, a different shape — and `acceptedPeer` by
`augmentSigDetails`'s `a.acceptedPeer`, an `attestationView`. `published.test.mjs` had both fields
parked in `COINCIDENTAL` for exactly that reason; `observables_test.go` has no `pendingView` entry
at all and was laundering them silently. So the two fields were **deleted** rather than parked, and
this guard is what makes the deletion falsifiable: it asserts over the marshalled JSON, because the
property is about what crosses the wire.

**The park that was asked for is refused by the scan itself, and that was measured rather than
argued.** `/pending 252` asked for `pendingView.valid` to go into `UNREAD_KNOWN`; adding it fails
`published.test.mjs` by name — *"these fields are parked in BOTH UNREAD_KNOWN and COINCIDENTAL …
They say different things … and a field cannot be both."* The item's remedy was written at P05's
close, before the coincidence door existed.

**One existing row was re-recorded in the same commit.** `consent-screen-names-only-the-caller`
carried `Reason: peer.Reason, Valid: peer.Valid,` as a context line — the only one of 293 rows that
did (`grep -ln "Valid: peer.Valid\|AcceptedPeer: peer.AcceptedPeer" test/redproofs/*.patch`). Note
for the next deletion: **`redproof.sh` replays against HEAD**, so a patch staled by an uncommitted
change still reports green; the check that catches it before the commit is
`git apply --check <patch>` against the working tree.

`recorded` 292 → 293.


## /pending 353 — the delivery round gets a convener, and a surviving mutation (v1.117.351)

| Defect reintroduced | Check that fired | What it said |
| --- | --- | --- |
| `the-listing-echoes-the-convener-from-the-marker` — `ReadStored` sets `Convener = Me` instead of reading `Record.Convener` | `TestTheListingNamesWhoConvenedIt`, tier 1 | "a ceremony this machine did not convene reports no convener at all" |
| `delivery-skipped-folds-into-one-meaning` — `renderDeliveryOutcomes` branches on `skipped` alone | `ceremonydeliver.test.mjs`, tier 2 | "the party that ENDED the proceeding is reported as though they simply have the document" |
| `delivery-control-matches-two-unknowns` — the gate compares `(c.me \|\| '')` to `(c.convener \|\| '')` | `ceremonydeliver.test.mjs`, tier 2 | "a ceremony that knows NEITHER its position nor its convener offers the delivery round" |
| `delivery-control-renders-behind-the-lock` — the `mayAct` gate dropped | `lockedpanel.test.mjs`, tier 2 | "the delivery control renders behind the lock… the button can only produce a 401" |

**The third row is here because it SURVIVED first, and that is the whole value of the pass.** The
fixture it was aimed at dropped only `me`, leaving `convener` known — so `'' === convener` was false
for the wrong reason and the mutation passed. The assertion's *message* named the `'' === ''` bug;
its *predicate* never reached it, which is the message-versus-predicate gap nothing in the toolchain
compares. A fifth ceremony knowing neither marker is the fix, and the row is the proof it now bites.

**A fifth and sixth mutation were proven red and not recorded**: dropping the `ended` gate, and
rendering `skipped` without its reason. Both fail the same two tests above for the same reasons, and
a row that duplicates another's diagnosis is a second expression of one check.

**One existing row was staled by this change and re-recorded in the same commit.**
`the-locked-screen-never-shows-its-ceremonies` carries the locked-panel call as context, and that
line gained an argument. **`TestEveryRedProofStillApplies` caught it before the commit** — which is
the difference between this and v1.117.350, where the same class of staleness had to be found by
reading, because `redproof.sh` replays against HEAD and the change was not yet committed.

`recorded` 293 → 297.


## /pending 23 — the discovery counters get a surface that is not a terminal (v1.117.352)

| Defect reintroduced | Check that fired | What it said |
| --- | --- | --- |
| `the-verdict-blames-the-firewall-first` — `Stats.Verdict()` tests `Own == 0` before `Sent == 0` | `TestTheVerdictSeparatesTheThreeWaysOfFindingNothing`, tier 1 | "Verdict() = 2, want 1" for a machine that sent nothing |
| `network-test-rebuilds-the-sentence` — the client writes its own prose instead of rendering `summary` | `networktest.test.mjs`, tier 2 | "the server's own sentence is not rendered" |
| `nobody-else-reads-as-success` — only the two hard failures are styled as warnings | `networktest.test.mjs`, tier 2 | "'nobody else is here' is styled as success" |

**The ordering row is the one worth reading.** A machine that sent nothing also heard nothing of
its own, so `Own == 0` is true there too — testing it first tells a user with no working network
connection that *a firewall is dropping their multicast*. The two remedies are opposite, and the
wrong one sends them into firewall settings for a cable that is not plugged in. The switch has
looked like an arbitrary order since `nib discover` was written; it is a rule, and now it has a row.

`recorded` 297 → 300.


## /pending 369 — the re-race gets a ceiling, and the second arm nobody had named (v1.117.353)

| Defect reintroduced | Check that fired | What it said |
| --- | --- | --- |
| `the-rerace-backoff-has-no-ceiling` — the doubling is uncapped | `TestTheReraceIsPacedAndBounded`, tier 1 | "after eleven attempts the wait is 3.2s, want the ceiling 2s" |
| `the-rerace-wait-outlives-its-window` — the delay is not clamped to the time left | `TestTheReraceStopsAtItsDeadlineAndNeverSleepsPastIt`, tier 1 | "the wait is 2s against a window with 10ms left — it would wake after the arm it is pacing had already ended" |
| `the-paced-arm-ignores-its-cancel` — `sleepOrDone` waits the timer unconditionally | `TestPacingIsAbandonedTheMomentTheArmIsCancelled`, tier 1 | "a cancelled arm completed its wait" |
| `the-post-signing-arm-retries-unpaced` — the promote retry keeps its bare `continue` | `TestBothRetryArmsRouteThroughTheReraceDoor`, tier 1 | "runCeremonyReceive calls reraceWait 1 time(s), want 2" |

**The fourth row is the one the item could not have asked for.** `/pending 369` named a single
unbounded `continue`; there were two in the same loop, and the one it missed had no deadline test of
its own at all. The guard therefore asserts **routing through the door** rather than either site's
numbers (ADR-009) — a guard written against the site the item named would have been green over the
site it did not.

**A fifth mutation was rejected rather than recorded**: deleting the post-signing door outright
leaves `promotes` unused and the package does not compile, and a patch that fails to build is not a
red proof — `redproof.sh` names that as its own failure. Re-cut to keep the counter and drop only
the pacing, which is the defect the row actually claims.

**Why there is no rate measurement here.** The item's blocker was that nothing had measured how fast
the loop spins, and the harness it wanted needs a counterpart that completes the p2p handshake and
drops it repeatedly. The ceiling makes the question moot: once warmed the arm cannot re-enter more
often than `reraceCap`, whatever the peer does. The unmeasured claim is now bounded rather than
outstanding.

`recorded` 300 → 304.
