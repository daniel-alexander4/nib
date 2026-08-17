# PLAN — Multiple open documents in Nib

Planning arc started 2026-08-15 (Stage 1 seed, from a completed `/tshoot` →
`/grill` → `/deepdive` chain rather than a cold brief — the design inputs listed
under "Inherited evidence" are treated as settled findings, not re-derived).
Stage 2 grill 2026-08-15: every load-bearing claim re-read at its cited line;
all held, and two amendments landed (the shared-teardown pin on P01.S03, the
2× undo-peak pin on D8).
Stage 3 subsystem rounds 2026-08-15: three flows (lifecycle, how an operation
reaches the right document, how state survives a switch) → D15, D16, the shared
sidebar pin on P05.
Stage 4 flat-spot passes 2026-08-15: ran to dry in two passes → D17 (`setDoc` is a
seven-caller chokepoint), D18 + P07 (no single-instance path exists).
Stage 5 standards alignment 2026-08-15: read `STANDARDS.md` in full → D19; P07 and
P02 both replaced with proven house patterns; the ADR question raised there was
answered 2026-08-16 (adopted — see D19).
Stage 6 dimension reviews 2026-08-15: hot-path/security, host availability, scale,
the crypto and verification SME packs, and "what haven't we looked at" → D9
amended from a count cap to count+bytes, plus pins on P05 (the one hot path; the
three bindings that fail as safety defects) and P06 (what a reload does).
Stage 7 plan-review 2026-08-15: structural gate + SME panel → one **critical**
finding landed as a pin on D7 (ids must never be reused, or the pinning law passes
while corrupting), three consistency defects fixed, D-numbering repaired.
Stage 8 bootstrap: **not run, by decision** — see D14.
Plan-review of P03 2026-08-16 (phase-scoped, ahead of phase-open): structural gate
passed with two defects (stale guard count, `Refs` omitting D15); panel returned
one **critical** — the optional-default id lets a call site unpin silently — plus
six warnings and three info. All adopted; pins landed on D15, D8 and P03.

**Where this plan and the original brief differ, the plan wins.**

**Sibling plan:** this repo also carries `PLAN-signing-ceremony.md` (Stage 1 seed
2026-08-15), which replaces the Collaboration process with the Signing Ceremony.
The two are independent and neither supersedes the other; `/createcode` must be
told which plan it is walking. The split ends when one of the two retires into
CLAUDE.md + ADRs per STANDARDS §15.6.

This plan covers **one feature project inside the existing `nib` repo**: making
Nib hold more than one document at a time, with per-document state preserved
across switches, plus **Close view** and **Close all**. It is not a plan for nib
as a whole — nib is a mature product (v1.102.2) with its own CLAUDE.md, release
machinery, and repo laws, all of which govern here unchanged. (It adopted ADRs on
2026-08-16 — see D19.)

---

## Inherited evidence (input, not to be re-derived)

Seam inventories, written during the grills that produced this plan:
`~/.claude/projects/-home-dan-repos-nib/memory/instruments/` —
`close-document.md`, `multi-document.md`, plus the Windows-work inventories
(`windows-open-dialog.md`, `embedded-asset-mime.md`, `key-path-normalization.md`,
`tool-discovery.md`) that establish the harness conventions this plan reuses.

The deepdive that sized this work found, all verified against cited lines:

1. **Post-await document reads** — 38 sites read `pdfDocument`/`docMeta` from a
   module binding after an `await`; 13 can corrupt. `save()` is the worst:
   captures A's bytes, reads B's `docMeta.canSave`, POSTs to a server whose
   working document is B — writing A's content over B's file, past the signature
   guard, silently.
2. **No document identity on the wire** — `/api/undo` is a bodyless POST;
   `/api/save`, `/api/ocr`, `/api/outline`, `/api/pdf` all address the single
   `s.doc`. N tabs would share one document and one undo ring.
3. **Seven client bindings lose work silently if shared** — `overlayFields`,
   `overlayHistory`, `redactMarks`, `docHadFlags`, `splitRects`+`sbPage`,
   `cropRect`+`cropPage`, `fillTarget`/`activeMarker`; plus `signLocked` and
   `lastSig`, which are role/security-relevant rather than merely lossy.
4. **Per-view arrays are necessary but not sufficient** — `detachField`,
   `reattachField` and `relayoutOverlays` resolve `overlayFields` and `viewer` at
   *call* time, so a command recorded in A and executed in B corrupts B.
5. **Verified empirically** — overlay elements and typed values survive hiding
   (`pv.div.appendChild(f.el)`, and the repo's own undo already detaches and
   re-attaches them); `display:none` preserves `scrollTop` (reads 0 while hidden,
   restores on show); `fitWidestWidth` silently no-ops on a hidden container
   (`clientWidth` 0); the 26 pointer listeners can move to a stable parent because
   `pageAt()` uses viewport coordinates.
6. **Undo budget** — `maxUndoBytes = 256 MiB` bounds *the* document; per-document
   rings would silently make it N × 256 MiB — and in fact 2N×, per the grill pin
   on D8, because redo is depth-capped but not byte-capped.
7. **Tool cleanup is document-wide** — `all()` is `document.querySelectorAll`, so
   `clearSplitRects` removes `.splitmark` nodes from every hidden view.

## Repo laws (inherited from nib; restated because this feature can break them)

- **Local only.** No document bytes leave the machine. The single network call is
  the version check.
- **Loopback only.** The server binds loopback and refuses any other address.
- **The vault is sealed to an SSH key**, and a document's *role* (`signLocked`,
  recipient copies) is a guarantee, not decoration — a bug that makes a
  counterparty's locked copy editable is a security defect, not a UI one.
- **Version bump on every change**, in the same commit (CLAUDE.md).
- **No remotes in the workflow** except nib's documented GitHub release publish,
  which is Dan-initiated only.
- **New law, added by this plan:** *no operation may act on a document it did not
  capture at its start.* See D7.

---

## Decisions

### D1 — Scope: multiple open documents, state preserved *(settled 2026-08-15 via /grill)*
Nib holds N open documents. Switching between them preserves each document's
state — scroll, zoom, page, form fills, overlays and their typed values. Menu
actions: **Close view** (the active document) and **Close all**. Dan chose this
over the cheaper "server holds N, client renders one and reloads on switch"
option, which would have discarded transient overlay state on every switch.

### D2 — Ordering: the single-document Close ships first *(settled 2026-08-15, Dan's call: "B first")*
Phase P01 delivers **Close** for the one-document app: real user value, already
grilled, independently releasable, and a strict prerequisite — Close-view is
Close plus "activate a neighbour". Multi-document work follows in P03–P06.
Rationale for the ordering: the dangerous slices (P03–P04) produce **no
user-visible output**, so front-loading them would mean a long stretch with
nothing shippable.

### D3 — Client architecture: one `PDFViewer` per document, hidden not destroyed *(settled 2026-08-15 via /grill)*
Each open document owns its own `PDFViewer` and its own
`#viewerContainer`/`#viewer` pair inside the stable `#viewerWrap`. Inactive views
are `display:none`. The page DOM is never torn down, so preservation is the
default rather than something re-implemented per overlay kind.

### D4 — Rejected: snapshot-and-rebuild overlays on switch *(settled 2026-08-15 via /grill)*
The obvious alternative — one viewer, serialize overlays on switch, rebuild on
return — is rejected because **an overlay's value lives in the DOM**
(`f.el.value`, `f.el.checked`), and `setDocument()` runs `_resetView()` →
`viewer.textContent = ""`. There are 12 overlay kinds; a missed property loses
the user's typing with no error. Preservation must not depend on twelve
hand-written round-trips being individually correct.

### D5 — Binding strategy: reassignable module bindings *(settled 2026-08-15 via /grill; **SUPERSEDED 2026-08-16** by the P05 phase-open decision)*
`viewer`, `eventBus`, `linkService`, `findController` become `let` and are
repointed to the active view on switch; `pdfDocument` already is one. This is
what makes D3 affordable: roughly 198 call sites read these at call time and need
no edits. Only `els.viewerContainer` (50 refs) needs a swappable binding.

**(superseded 2026-08-16, recorded during P05.S03 — this decision IS swap-on-switch,
which P05's phase-open decision refuses.)** The paragraph above and the phase-open
decision "swap-on-switch is refused" contradict each other outright: one prescribes
repointing module bindings at the switch boundary, the other refuses exactly that and
gives the reason (the bindings become a cache whose correctness depends on nothing async
reading them across a switch — the class P04 spent three slices closing server-side).
The phase-open decision governs, and `web/app.js:1256-1262` already records the refusal
in code. **D5 is left in place rather than struck**, because the useful artifact is the
contradiction: a reader sizing this work from D5's "roughly 198 call sites … need no
edits" would size it at zero, when the true figure is 79 code sites that all get edited.
Its `els.viewerContainer` count is also wrong — 51, not 50.

### D6 — Document identity is a wire concern *(settled 2026-08-15 via /deepdive)*
Every document-touching endpoint carries a document id. A client-side view
registry cannot fix a server that holds one working copy: without this, every
server operation in one tab rewrites the others. Server-side, `s.doc` becomes an
ordered registry plus an active id, reached through a single `activeDoc()`
accessor so the existing `doc := …; if doc == nil` guard shape is preserved at
all ~14 sites.

### D7 — Operation pinning (the new repo law) *(settled 2026-08-15 via /deepdive)*
**No operation may act on a document it did not capture at its start.** Every
async operation captures its document (and id) before its first `await`, carries
that id to the server, and discards a reply whose id no longer matches the
target. This is the whole of the safety work and the whole of the risk; it is
invisible to users and cannot be skipped.

**(plan-review pin, critical: ids are never reused, 2026-08-15)** The whole law
reduces to an id comparison, so **an id that can be reused defeats it silently and
completely**: close document 3, open another that is also assigned 3, and an
operation pinned to the *old* 3 passes its check and commits to the new document —
the exact corruption D7 exists to prevent, now wearing a passed guard. Ids are
therefore a monotonic counter for the life of the process, never an index into the
registry, never reclaimed on close. This is cheap to get right and impossible to
detect later, since the failure looks like a correct comparison.

### D8 — Per-document undo rings, one global byte budget *(settled 2026-08-15 via /grill)*
Rings move per-document, but `maxUndoBytes` becomes a **shared** budget across all
open documents, evicting from inactive documents first. With one document open,
behaviour is byte-identical to today. Per-document budgets would silently
multiply memory by the tab count.

**(grill pin: the peak is 2×, 2026-08-15)** `undo.go:9-16` documents that redo is
depth-capped but *not* byte-capped, so a deep undo of large documents transiently
holds up to **2×** `maxUndoBytes` across the two stacks. That accepted peak
becomes **2N×** under per-document rings. The shared budget must therefore bound
the undo+redo pair across all documents, not the undo side alone — otherwise the
per-document change quietly re-scales a limit the single-document design
deliberately reasoned about and accepted.

**(plan-review pin, warning: eviction must be observable, 2026-08-16)** Evicting
"from inactive documents first" discards that document's undo history while the
user is looking elsewhere. They switch back and their undo is gone, having done
nothing to cause it and having been told nothing. *Its own observation:* after an
eviction, the affected document's `docResponse.canUndo` is false **and** the UI
says why on activation — asserted by a test that fills two documents past the
budget and reads the evicted one's flag, red if eviction is silent.

### D9 — Open-document cap *(settled 2026-08-15; ~~a count~~ **amended to count + bytes (dimension review, 2026-08-15)**)*
Eight open documents, refused with a clear message rather than degrading. Each
document costs its bytes (up to 200 MiB), a pdf.js proxy, a rendered DOM subtree
and a share of the undo budget.

**Amended — a count cap does not bound the thing it exists to bound.** Eight
documents is a number; eight *documents* is anywhere from a few hundred KB to
1.6 GB, because nib accepts documents up to 200 MiB and real ones vary by three
orders of magnitude. A count-only cap is therefore either far too loose (eight
large scans) or pointlessly tight (eight forms). The cap becomes **count AND
aggregate bytes, refusing on whichever binds first**, with the byte figure chosen
against a measurement in P06 rather than assumed here. The undo side is already
bounded by D8's global budget; this bounds the document bytes beside it.

### D10 — Arrivals open a new tab *(settled 2026-08-15 via /deepdive)*
A completed co-sign or p2p receive calls `setDoc` today, which under tabs would
replace whatever the user was editing. Arrivals open as a new tab instead.

### D11 — Compare is closed on switch, not made per-view *(settled 2026-08-15 via /deepdive)*
The `cmp*` family is state about a *pair* — `cmpAlign` maps A-pages to B-pages,
`cmpText` caches A's extracted text — anchored to the module `pdfDocument`. Left
shared and open it would render a diff that was never computed. Switching closes
Compare via the existing `closeCmpDoc()`. Making it per-view is a separate
redesign and would put up to 2N pdf.js documents in memory.

### D12 — View-scoped cleanup *(settled 2026-08-15 via /deepdive)*
`all()` is `document.querySelectorAll`. Every tool-cleanup sweep
(`clearSplitRects`, `clearCropRect`, and siblings) must be scoped to the active
view's subtree, or exiting a tool in one document deletes another's marks.

### D13 — A test harness is a prerequisite phase *(settled 2026-08-15, auto-adopted; shape amended by D19)*
The repo has **no JS tests**, and every failure mode this feature risks is silent
by construction: the wrong file overwritten, a redaction burned onto the wrong
pages, a signature guard that stays quiet. P02 builds a browser-driving harness
before the dangerous phases, and every acceptance criterion in P03–P06 is written
to be asserted by it. Rationale for adopting without asking: correctness cannot
defend building P04's 38-site change with no way to observe a regression.

### D14 — P00 is pre-satisfied; no bootstrap runs *(settled 2026-08-15)*
The planning playbook's Stage 8 instantiates templates, writes `VERSION` as
`0.1.0`, and runs `git init`. Nib is a mature repo at v1.102.2 with its own
CLAUDE.md, release machinery, and history. Running that stage would be
destructive. P00 is marked pre-satisfied; this plan starts at P01.

### D15 — How the document id travels *(settled 2026-08-15 via subsystem round)*
- **A request header, `X-Nib-Doc`.** Roughly 30 of the ~68 registered routes touch
  the document, and they take three different body shapes — bodyless POST
  (`/api/undo`, `/api/redo`), JSON, and multipart. A header carries the id across
  all three without editing a single body schema, and it matches the existing
  `X-CSRF-Token` convention.
- **Except `/api/pdf`, which takes a query parameter.** Verified: that URL is
  fetched by **pdf.js**, not by nib's own `apiFetch` — `app.js:1221` calls
  `getDocument({ url: '/api/pdf?t=' + Date.now() })`. The fetch belongs to the
  library, so a header would mean opting into pdf.js's `httpHeaders` plumbing to
  gain uniformity nobody reads. The URL already carries a cache-buster; the id
  joins it. **The exception is named here so it is not "fixed" into uniformity
  later by someone who did not know why it exists.**
- **The id is optional and defaults to the active document.** Around 20 existing
  Go tests `GET /api/pdf` with no id at all, and `nib`'s CLI verbs address a
  single document by construction. Optional-with-a-default keeps every one of
  them meaningful; a required id would edit ~20 tests to say nothing new.
- **An id that names a document the server no longer holds is `409`, not `404`.**
  404 already means "no document open"; a closed tab is a different fact and the
  client must be able to tell them apart to remove the tab rather than blank the
  app.

**(plan-review pin, critical: who may omit the id, 2026-08-16)** The optional
default and D7's pinning law meet badly. D7 is *no operation may act on a document
it did not capture*; the default says omitting the id is fine, and a call site that
simply **forgets** the header then gets "whatever the server currently thinks is
active" — which, during exactly the switch D7 exists to survive, is the wrong
document. It commits having passed no check, because it never made one.

What makes this critical is that **the omission is invisible**: a pinned call and
an unpinned call differ by the *absence* of a header. No error, no log line, and
nothing in review distinguishes "correctly defaulted" from "forgot" — the same
shape as the id-reuse trap, a guard reporting success because it was never reached.

So the default is **for the CLI and the pre-existing Go tests only**. The web
client always sends an id, and that is enforced **by the transport rather than by
discipline**: `apiFetch` attaches the calling view's captured id to every
document-touching route, so a call site cannot omit it by forgetting.

*Its own observation, not a clause riding an existing bullet:* a tier-2 test that
walks every `apiFetch` call to a document route and asserts `X-Nib-Doc` is present,
**proven red by removing the attachment in `apiFetch`**. That test is what
discharges this pin; no other criterion covers it.

**(plan-review pin, warning: ids are per-process, 2026-08-16)** ADR-001 makes ids
monotonic and never reused — *within a process*. A restart restarts the counter at
1. Usually harmless, because the default `NIB_ADDR` is `127.0.0.1:0` and a restart
takes a different port, so a surviving browser tab simply cannot reconnect. **But
`NIB_ADDR` pins a fixed port for headless/remote runs** (`cmd/nib/main.go:58-60`),
which is a documented, supported mode — and there a stale tab reconnects and can
pin to an id the new process has since reassigned to a different document. That is
ADR-001's exact failure, crossing a process boundary.

The id therefore carries a per-process epoch (`<nonce>:<counter>`). A mismatched
epoch is **409**, the same answer as a closed document, because it is the same
fact: the document you named is not one I hold.

### D16 — Opening a path that is already open focuses it *(settled 2026-08-15, auto-adopted; override welcome)*
Two tabs on one path would be two independent working copies, both savable to the
same file: whichever saves last silently discards the other's work — the same
class of loss as the `save()` bug in D7, arrived at by a different road. Opening
an already-open path activates the existing document instead. Closing the active
document activates its right-hand neighbour, else its left.

### D17 — Every `setDoc` caller declares replace-or-new-tab *(settled 2026-08-15 via flat-spot pass)*
`setDoc` is the **single chokepoint** through which any document becomes current —
verified, seven callers and no other path: `server.go:244` (open by path),
`server.go:275` (upload), `sources.go:53` (open from a source), `combine.go:40`
(combine), `office.go:54` (office→PDF), `session.go:252` and `session.go:549`
(completed exchanges). This sizes D6 favourably: one function becomes
add-and-activate and every entry path inherits it.

It also makes a question unavoidable that D10 only asked of two of them. Opens and
arrivals clearly open a new tab. **Combine and office-convert are not obviously
the same** — combine consumes N *files* and produces one, and today it replaces
whatever was open even when that document was not an input. Each of the seven
declares its answer at P03; none inherits a default silently.

### D18 — Single-instance is out of P06's scope, scheduled as P07 *(settled 2026-08-15, auto-adopted; flagged for override)*
Verified: `cmd/nib/main.go:62` binds `127.0.0.1:0` unless `NIB_ADDR` is set, and
nothing anywhere handles an already-running instance. So a second launch always
succeeds and yields a **second nib** — its own window, its own server, its own
vault unlock. With `nib register` associating `.pdf` on Windows, that means
double-clicking a second document opens a second *application*, not a second tab.

Tabs are fully usable without fixing this (File→Open opens tabs), and the fix is a
distinct and riskier change — a lock file or named pipe, stale-lock recovery, and
a hand-off of the path to the running process, all interacting with the vault. So
it is **its own phase, not a rider on P06** — but it is named as a decision
because "the app has tabs yet the OS still opens a second window" is a coherence
gap a user meets on their first double-click, not an edge case.

### D19 — Standards alignment *(settled 2026-08-15 via Stage 5, against `~/repos/project-standards/STANDARDS.md`)*
Read in full against this plan. Four adoptions, one deviation, one question for Dan.

**Adopted — P07 is a house pattern, not new design.** §8 already prescribes the
"startup safety trio": *attach-first (open a window onto the live instance instead
of double-running) → refuse-second-instance guard → hard flock/pidfile mutual
exclusion, plus wait-for-port-free before binding a stable port*, proven in
hespera and zoetrope. P07 adopts it rather than inventing one. Consequence to
carry: §9 permits a random port "by default; a stable port only when stale-tab
reattachment matters" — attach-first is exactly when it matters, so P07 moves nib
off `127.0.0.1:0` (`main.go:62`) onto a stable port with the wait-for-port-free
step. That is a real change to startup and is P07's main risk, named here.

**Adopted — P02's shape, and its ceiling.** §10 prescribes *"test the shipped
frontend JS, not a copy"*: a dev-only jsdom harness (`node --test`, jsdom the sole
devDependency, `package.json` marked not-shipped) loading the real `web/app.js`,
**with its ceiling documented and the gap explicitly delegated to Playwright**.
That splits P02 correctly into tiers rather than one undifferentiated harness —
see the phase's rewritten exit criteria.

**Adopted — a source-of-truth document.** §5.2 calls for one canonical
data-ownership doc per hairy domain, naming what owns each piece of data and what
survives which operation, made a mandatory pre-read in `CLAUDE.md`. "Who owns this
state and does it survive a switch?" is *the* question of this whole feature —
12 overlay kinds, 7 loss-bearing bindings, per-view versus shared. P05 produces
`docs/document-state-source-of-truth.md`; it is the artifact that keeps the
answer from living only in this plan, which retires.

**Adopted — guards ship with their rules.** §6: *when you write a "keep X in sync
with Y" rule, write the test in the same commit.* This plan writes two such rules —
D7's pinning law, and P01.S03's "open and close reset the same set" — so each
lands with a guard, in the docs-parity shape §6 credits to vigo.

**Deviation, deliberate — the zero-users doctrine (§2) does not apply.** Nib has
real users; the §2 tripwire has already fired (a user's Windows bug report drove
v1.101–v1.102). The doctrine's *consequence* is still mostly available, though,
and D15's optional id leans on it: the UI ships embedded in the binary
(`embed.go`), so client and server upgrade as one artifact and an internal wire
change needs no negotiation. The id is optional to keep ~20 existing tests and the
CLI meaningful — not for client compatibility.

**Answered 2026-08-16 (Dan's call): ADRs adopted.** `docs/adr/` now holds
**ADR-001 — operation pinning (D7)**, carrying the never-reuse-an-id pin as its
load-bearing half, and **ADR-002 — one `PDFViewer` per document (D3)**, with
`_index.md` recording why the corpus starts at two and why earlier decisions are
deliberately not backfilled. `CLAUDE.md` and `CONTRIBUTING.md` both carry the rule.

Why it mattered rather than being bookkeeping: **this plan retires when its build
order is walked** (§15.6). Without ADRs, D7's law would go on constraining every
future async operation while the only record of *what it protects against* — the
13 corrupting sites, the `save()` case, the id-reuse trap — retired with the
document that found them.

---

## Build order

### P00 — Bootstrap *(pre-satisfied — see D14)*
Goal: repo scaffolding. Already present: CLAUDE.md, VERSION, build/install
scripts, embed.go, nfpm packaging, desktop entry, wine harness.
Exit criteria: n/a.

### P01 — Close the open document *(done 2026-08-16, v1.103.0)*
Goal: the app can put a document down without quitting or opening another,
returning to exactly the state it launches in.
Exit criteria:
- [x] `/api/pdf` 404s and the UI shows "Open a PDF to begin." after Close.
      Both halves driven live — 404 with the literal `no document open`, and
      `#empty` computed visible with its text asserted literally.
- [x] The undo rings are cleared — the document's bytes are not retained.
      Rings: asserted directly on the `*Server` with both non-empty first, and
      red-fixtured. Bytes: **met at the reference level** (all three byte-holding
      fields dropped, ring entries nil'd) and **not measurable at this
      granularity** as a heap property — no instrument in this phase observes
      reclamation, and that is recorded rather than read as met.
- [x] Opening a document after a Close works normally.
- [x] Full-repo `/code-review` clean; shippable as a release. One finding,
      pre-existing and outside scope (Compare is not torn down on open — D11's,
      filed); `-race` green across all 15 packages.

#### P01.S01 — Server: honest empty state on the mutating routes *(done 2026-08-15, v1.102.4)*
Scope: the four routes that currently commit against a closed document and return
200 (`pages.go:142`, `outline.go:55`, `redact.go:60`, `export.go:229`). Refs: D2.

**(slice-grill pin: order swapped with S02, 2026-08-15)** This was S02. The four
routes' wrong-success is near-unreachable today — `s.doc` is nil only before the
first open, when the UI disables those controls — and **Close is what makes it
reachable**. Landing the close route first would put a live wrong-success on the
redaction path into this repo's history for one commit; landing the guards first
costs nothing.

**(reality drift, 2026-08-15: the guard shape changed during implementation)** The
grill approved copying the 13 siblings' `doc := s.doc; if doc == nil` guard. Reading
the handlers refuted the premise: **these four never read `doc.data`.** They work
entirely from bytes the client posted and only *install* the result. So the sibling
shape is a pattern that does not fit, and worse, a check placed before the commit
call leaves a TOCTOU — a close landing in between still yields a 200 for discarded
work, which is the exact defect. Instead `commitMutation`/`commitBarrier` **return
whether they committed**, and all 8 production callers act on it. One lock
acquisition, no window, and the four already-guarded callers (`attachments.go:57`,
`scan.go:76,146`, `ocr.go:70`) get the same race closed for free.

Tasks:
1. T01 — `commitMutation` and `commitBarrier` return `bool`.
2. T02 — all 8 production callers 404 on a false return.
3. T03 — table-driven test over the four routes: **200 with a document open first**,
   then 404 without.
4. T04 — assert the 404 body is the literal `no document open` the siblings emit.

Acceptance:
- Each of the four returns 404 with no document open, matching the other 13.
- Each still returns its normal status **with** a document open — the non-zero probe.
- The 404 body is the same string the siblings emit, not a new one.
- A race between an in-flight op and a close cannot report success for
  discarded work.

#### P01.S02 — Server: drop the document *(done 2026-08-16, v1.102.6)*
Scope: `POST /api/close` → `setDoc(nil)`, which already clears both rings.
Refs: D2.

**(reality drift, 2026-08-16: the ring-clearing instrument could not fail)** The
acceptance clause below used to justify itself with "`docResponse` computes
`canUndo` from `len(s.undo)` **independently of `doc`**, so this detects an
uncleared ring." **Refuted at `server.go:344-352`**: it computes `canUndo` under
the lock and then returns the *zero struct* when `doc == nil`, discarding it. So
`canUndo` is false after any close because the document is gone, not because the
ring was cleared — the test would have passed unchanged against a server whose
`setDoc(nil)` left the ring fully populated. Since the clause it discharges is a
**phase** exit criterion about retained document bytes, the defect grades at the
clause's severity, not the check's. The clause now asserts the ring directly; the
server tests are `package server`, so `len(s.undo)` is available and resolves it
exactly.

**(grill pin: the in-flight clause needs no timing, 2026-08-16)** Exactly one
lock acquisition decides it (`undo.go:37-41`, `:61-65`), so a test places the
close *before* it deterministically. The concurrent probe is retained for the
race detector but explicitly does not discharge the clause: a 404 cannot
distinguish "the close landed inside the window" from "the close won outright",
so a both-outcomes rule would be a proxy for window-visited rather than a
measurement of it — the same coarser-than-the-clause defect just fixed above.

Tasks:
1. T01 — `handleClose`: `setDoc(nil)`, then `writeJSON(w, s.docResponse())`.
2. T02 — register `POST /api/close` behind `requireUnlocked`.
3. T03 — transition test: open → `/api/pdf` 200 → close → 404 + `no document open`.
4. T04 — ring clearing asserted on the `*Server` directly, non-empty first.
5. T05 — idempotency: close with nothing open, and close twice.
6. T06 — close, then open another document.
7. T07 — the in-flight clause, deterministically, on both commit helpers.
8. T08 — `-race` probe: `/api/pages` concurrent with `/api/close`. **(reality
   drift, 2026-08-16)** Its planned invariant — *no reply is ever 200 with an
   empty `docResponse`* — is refuted by `pages.go:142-146`: the lock is released
   between the successful commit and the `docResponse()` call, so a close landing
   there yields a 200 with an empty body that is entirely correct. The probe now
   asserts every reply is **200 or 404** — never a panic, a 500, or a dropped
   connection. The commit-landed-or-404 property is T07's, and deterministic.
9. T09 — post-close exposure: `/api/redact` 404s *after* a close.
10. T10 — this amendment.

Acceptance:
- After close, `/api/pdf` 404s and `/api/doc` returns the empty `docResponse`.
- Undo history is gone: **`len(s.undo)` and `len(s.redo)` are both zero after the
  close, with the undo ring asserted non-empty before it.** Asserting only the
  post-close state would pass against a server that never had history, and
  asserting it through `docResponse.canUndo` would pass against a server that
  never cleared the ring (see the drift pin above).
- `POST /api/close` with nothing open is idempotent: 200 and the empty
  `docResponse`, matching the `ErrNotEncrypted`-passes-through precedent (v1.57.0).
- **(carried from P01.S01, 2026-08-15)** A close landing *while* one of the four
  mutating routes is in flight makes that route answer 404, not 200. S01 closed
  this class structurally — the test-and-write is one lock acquisition — but
  recorded the clause `not exercised`, because until this slice there was no way
  to make a close happen mid-operation. It is exercisable here and must actually
  be driven, not inherited as met.
- Opening a document after a close works normally.

#### P01.S03 — Client: the teardown *(done 2026-08-16, v1.102.9)*

**(deepdive pin, 2026-08-16 — the dive ran before the grill and changed the slice
in both directions)** Three claims verified at the line, two work items retired,
one defect found that the plan does not scope:

- `viewer.setDocument(null)` is a supported, thorough teardown
  (`web/vendor/pdfjs/pdf_viewer.mjs:8385-8402`): it dispatches `pagesdestroy`,
  cancels rendering, runs `_resetView()` (`:8720-8749`, which empties
  `viewer.textContent`, resets scale/page/rotation and aborts its event
  `AbortController`), nulls the find controller and scripting manager, **destroys
  the annotation-editor UI manager** and sets its mode to `NONE`. So no Nib-side
  find or editor teardown is needed, and the slice must not poke
  `viewer.annotationEditorMode` at all.
- `updateBadge(null)` resets `lastSig` *and* the badge *and* the details button in
  one call (`:1976`, `:1984`, `:1996`) — the plan listed those separately.
- `applySignLock()` reads `!!pdfDocument` (`:4865`), so ordering is load-bearing:
  null the document first, then the flags, then call it.
- Three launch-state items the scope line omits, found by diffing against
  `index.html:62-63,93,125`: `saveBtn.disabled` **and its `title`** (`:1248`
  overwrites it with a path-specific string), `.pageCount` → `/ 0`, `.pageNum` → 1.

Tasks:
1. T01 — `resetSharedDocState()`: `clearOverlays()` plus the four armed modes.
2. T02 — it does not touch `viewer.annotationEditorMode`; pdf.js owns that.
3. T03 — `setDocumentFromServer` calls it in place of the bare `clearOverlays()`.
4. T04 — `closeDocument()`: null first, bump `docGen`, close Compare, shared
   reset, `viewer.setDocument(null)`, `linkService.setDocument(null, null)`,
   destroy the captured doc.
5. T05 — the launch chrome, asserted against `index.html` rather than memory.
6. T06 — `docMeta`/`originalName`/`docHadFlags`/`signLocked`, then
   `applySignLock()` and `setDocControls(false)`.
7. T07 — `closeCmpDoc`'s `destroy()` gains the `.catch()` its siblings have.
8. T08 — the G2 fix in `buildThumbnails` (see the gap-down pin below).
9. T09 — live-drive all nine paths in a real browser.

**(gap-down G2, found by the deepdive — a defect the plan does not scope)**
`buildThumbnails` (`app.js:2366-2404`) has **two awaits per iteration** and its
staleness guard sits before both. The loop condition re-reads
`pdfDocument.numPages` after every await, and `els.thumbGrid.appendChild(wrap)`
(`:2400`) sits *after* the guard and *before* the second await. So a close landing
during `await pdfDocument.getPage(n)` lets that iteration run on and append **one
orphan thumbnail into the grid the close just cleared**, then throw a `TypeError`
on the nulled binding into the `.catch` at `:1257`, which logs and swallows it.
**Bumping `docGen` is necessary but not sufficient**, and the acceptance clause
"the empty state is byte-identical to launch" fails intermittently and silently.
`buildOutline` (`:2889-2916`) is correct by contrast — one await, guard
immediately after, everything downstream synchronous. Fix: hoist `numPages` before
the loop and re-check the generation immediately before the append.

**(measurement correction, 2026-08-16, S04 — the reachability above was
overstated.)** Driven for real in S04 with an 80-page document, closing while the
build was demonstrably in flight (3 of 80 thumbnails present), the fix produced an
empty grid — **and so did a red fixture with the fix removed.** The console says
why: `thumbnails failed RenderingCancelledException`. Tearing the document down
**cancels the in-flight page render**, and that exception unwinds the entire
`buildThumbnails` loop through the caller's `.catch` before any further iteration
can append or re-evaluate the loop condition.

So the window is real in the code but **not reachable via a Close**, and neither
half of the fix repairs an observable defect. Both are kept as defence-in-depth
and re-commented to say so: the loop's actual protection is a *pdf.js* guarantee
it never declares a dependency on, and a bump that stopped cancelling renders
would reopen the window with nothing in the function explaining it. The original
claim is left above rather than deleted, because the correction is the useful part
— a defect reasoned from the code and then measured to be unreachable is exactly
what the "severities are assumptions too" rule is about.

**(harness pin, 2026-08-16)** D2 puts P01 first and D13/D19 put the three-tier
harness in P02, so this slice and S04 have **no automated tier** and are verified
by live browser drive only. Recorded so the absence of a test file here is not
later read as an oversight.

**(reality drift, 2026-08-16, found during implementation — the slice cannot
verify its own close path)** `web/index.html:1310` loads `app.js` as
`<script type="module">`, so `closeDocument` is module-scoped and unreachable from
the console; and this slice's own scope puts the Close *control* in S04. So there
is no way to make a Close happen while S03 is the slice in hand.

Rather than add a test-only hook to production code, the clauses are **split by
what is actually drivable now**, using the same carried-clause mechanic S01 used
to hand its race clause to S02:

- **Discharged here** — the shared reset and the open-over-open bug it fixes. That
  is this slice's genuinely novel content and its one behaviour change to an
  existing path, and it is fully drivable: arm a tool, open a *different*
  document, observe no lit button and no crosshair.
- **Carried to S04, recorded `not exercised` here** — everything that needs a
  Close to happen: the empty state, the pdf.js and Compare teardown, the
  mid-build thumbnail race, and the next-open-still-works clause. S04 must
  actually drive these, not inherit them as met.
Scope: `closeDocument()` mirroring `setDocumentFromServer` — bump `docGen`, the
shared reset (below), `closeCmpDoc()`, detach then destroy via
`loadingTask.destroy()`, drop `has-doc`, reset `docMeta`/`lastSig`/`originalName`/
flags/badge, clear both sidebars, `setDocControls(false)`. Refs: D2, D11, D12.

**(grill pin: the four modes are an *open* bug too, 2026-08-15)** Verified at the
line: `setDocumentFromServer` (`app.js:1215-1218`) calls `clearOverlays()` and
nothing else, and `clearOverlays` (`:6193-6204`) resets seven draw modes but not
`redactMode` (`:4248`), `editMode` (`:4671`), `markerMode` (`:4787`) or
`activeTool` (`:5421`). So **opening a second document already leaves a lit tool
button and a crosshair cursor describing a mode whose marks were just wiped.**
Resetting those four only in `closeDocument()` would make close stricter than
open and start the two teardowns drifting — the exact seam this slice exists to
avoid. They go in the **shared** reset both paths call, which fixes the existing
open-over-open defect as a side effect. That widening is deliberate and is the
only behaviour change this slice makes to an existing path.

Acceptance:
- No second pdf.js document survives a Close (Compare included).
- A throw mid-teardown cannot leave a half-closed state that poisons the next
  open — `pdfDocument` is nulled before anything that can throw. (`closeCmpDoc`
  at `:1524` is today the one `destroy()` call of three with no `.catch()`;
  `:959` and `:1239` both have one. Bring it into line.)
- Arming the redact tool and then opening a *different* document leaves no lit
  button and no crosshair — the pre-existing bug, asserted directly.
- The empty state is byte-identical to launch.

#### P01.S04 — Client: the Close control and the unsaved-work confirm *(done 2026-08-16, v1.102.10)*

**(grill pin, 2026-08-16: the three signals mean "ever edited", not "unsaved")**
The scope line below specifies the confirm "from the three signals that exist".
Tracing the save path refutes the premise: **a successful save clears none of
them.** `save()` (`app.js:2001-2029`) reloads only `if (overlayFields.length)`, so
an AcroForm fill with no overlays keeps the same `annotationStorage` with the same
entries; and the server's `handleSave` writes the file and updates
`doc.data`/`doc.sig` but never touches `s.undo`, so `docMeta.canUndo` survives a
save — correctly, because undo-after-save is a feature.

So after any save the confirm still fires. The error direction is **safe** (it
over-prompts, never under-prompts), but the consequence is not cosmetic: a confirm
that fires every time trains the user to dismiss it, so it stops protecting on the
one close where it mattered.

**Shipped as specified anyway, for a sequencing reason.** The honest fix is a
dirty flag set by the mutation funnel and cleared by a successful save — which
needs every `setDocumentFromServer` caller to declare what it is doing. **P04
rewrites exactly those call sites** for operation pinning (D17). Building it here
means building it twice. What this slice owes instead is that **the prompt must
not claim more than the signals support**.

Tasks:
1. T01 — a `Close` button after `Print…`, at the end of the file-actions group.
2. T02 — register `closeBtn` in `DOC_REQUIRED`.
3. T03 — the handler: prompt when any edit signal is set; cancel returns first.
4. T04 — **server first, then client**: `POST /api/close`, tear down only on success.
5. T05 — wording that does not overclaim ("since the last save").
6. T06 — drive the four clauses S03 recorded `not exercised`.
7. T07 — drive the orphan-thumbnail race on a document large enough to still be
   building; report `not exercised` if the build was not in flight.
8. T08 — record the double-click bypass (G2) in the slice.
Scope: a **Close** button in the File tab toolbar, registered in `DOC_REQUIRED`;
confirm before discarding unsaved work, from the three signals that exist
(`annotationStorage.size`, overlay fields/history, `docMeta.canUndo`). No Ctrl+W
— Chromium app-mode owns it. Refs: D2.
Acceptance:
- The control greys out with nothing open.
- Closing with unsaved edits prompts; cancelling leaves the document untouched.
- **(carried from P01.S03, 2026-08-16)** With the control in place, drive the
  teardown clauses S03 could not reach and must not be inherited as met: the
  empty state is byte-identical to launch (badge, `saveBtn` **and its title**,
  `/ 0`, `.pageNum` 1, both sidebars); no second pdf.js document survives a Close,
  Compare included; a Close during a large document's thumbnail build leaves **no
  orphan thumbnail**; and the next open works normally.
- **(carried from P07's premise correction, 2026-08-16)** The confirm is
  **bypassable by the ordinary double-click path** until P07 lands — a second
  launch SIGTERMs this instance and discards the document with no prompt. Say so
  in the slice rather than implying the prompt is unconditional.

### P02 — Test harness, in three tiers *(done 2026-08-16, v1.103.6)*
Exit criteria — all met:
- [x] Tiers 2 and 3 each runnable from one command, skipping cleanly when their
      dependencies are absent. Tier 2's skip verified *before* `npm install`;
      tier 3's two skips (driver, browser) verified separately, the browser one
      naming all six candidates it looked for.
- [x] Each tier proven red at least once. Tier 2: a renamed id, naming the id.
      Tier 3: a changed empty-state string, naming the assertion. Plus three more
      red paths in S05 and the cross-tier fixture below.
- [x] P01's acceptance expressed as tier-2/tier-3 tests, retroactively — including
      the two edit signals and the thumbnail grid tier 2 could not reach.
- [x] The tier ceilings written where the harness lives. Tier 1 had no file, so it
      leads the chain in `CONTRIBUTING.md`; `verify_test.go` guards all three.

**The result that justifies the tiering, and it is measured rather than argued:**
removing the grid clear from `closeDocument()` leaves **tier 2 at 13/13 — blind**
— and **fails tier 3 twice**. A defect one tier cannot see and the other catches
is the whole argument for having two; until that run it was a comment.

Goal: a way to assert this feature's behaviour mechanically, because every failure
mode in P03–P06 is silent. Refs: D13, D19.

The tiers exist because no single harness reaches all of it, and pretending
otherwise is how an untested path passes silently (§10's "verification boundary"
rule). Each tier's ceiling is stated so the next one is a deliberate delegation
rather than a gap:

1. **Go tests** *(exist today — 84 test files)*. The document registry, the id on
   the wire, the shared byte budget, the four 404 guards. **Ceiling:** the server
   never sees the client's state, so nothing about overlays or bindings.
2. **jsdom + `node --test`** *(new; §10's prescribed shape, proven in hespera)*.
   jsdom as the sole devDependency, `package.json` marked not-shipped, loading the
   **real** `web/app.js`. Covers per-view state records, binding reassignment
   (D5), the teardown, and that `detachField`/`reattachField` resolve the owning
   view rather than the active one (D4's failure). **Ceiling:** jsdom models the
   DOM, not pdf.js's rendering — no layout, no `clientWidth`, no canvas. So it
   cannot see the `fitWidestWidth`-on-a-hidden-container class of bug at all.
3. **Playwright against a real nib** *(the tier this repo has already used at
   v1.79.2, v1.79.4 and v1.93.6, and whose server-side twin is
   `build/winrepro.sh`)*. Everything tier 2's ceiling excludes plus anything
   touching real files: scroll and zoom preservation, re-fit on activation, and
   P04's save-to-the-wrong-file assertion.

Exit criteria:
- Tiers 2 and 3 each runnable from one command, skipping cleanly when their
  dependencies are absent — the convention the poppler/Ghostscript/veraPDF tests
  and `winrepro.sh` already follow.
- **Each tier proven red at least once** against a deliberately reintroduced
  defect. A harness never probed red is a harness that can only report pass.
- P01's acceptance criteria expressed as tier-2/tier-3 tests, retroactively.
- The tier ceilings written down where the harness lives, not only here.

**(phase-open, 2026-08-16 — slices firmed against the codebase as it now stands.)**
Tier 1 already exists (90 test files), so this phase builds tiers 2 and 3 and the
contract naming all three. **Each tier proves itself red inside its own slice**
rather than deferring that to a closing slice: a harness that ships before it has
been seen to fail is the thing this phase exists to prevent.

Feasibility was **measured at phase-open, not assumed** — the probe is recorded in
`instruments/P02.md`. Its findings, each of which changed a slice: only two vendor
modules need stubbing (not four); `pdf_viewer.mjs` reads `globalThis.pdfjsLib`
rather than its import; jsdom implements no `matchMedia` at all; the `pdfjsLib`
surface `app.js` uses is exactly **7 symbols**; and the real `index.html` satisfies
all **428** element ids `app.js` requests, with **0** missing.

#### P02.S01 — Tier 2: the jsdom harness that loads the real `web/app.js` *(done 2026-08-16, v1.103.1)*
Scope: `package.json` (private, not shipped, jsdom the sole devDependency), the two
vendor stubs, the resolve hook, the jsdom boot, a smoke test, the id-coverage
guard, and `build/jsdomtest.sh`. Refs: D13, D19.

Tasks:
1. T01 — `package.json` (`private: true`, not shipped); `node_modules/` git-ignored.
2. T02 — `test/jsdom/stub-pdfjs.mjs` (7 symbols) + `stub-viewer.mjs` (5 classes),
   surfaces enumerated from the code; `EventBus` real because `app.js` drives
   behaviour through it.
3. T03 — `test/jsdom/hooks.mjs`: a resolve hook matching **exactly** the two
   vendor specifiers, so `detect.js`, `diff` and `pixelmatch` load for real.
4. T04 — `test/jsdom/boot.mjs`: jsdom from the real `index.html`, the browser
   globals, a `matchMedia` polyfill, a per-endpoint `fetch` stub derived from the
   Go handlers, and `globalThis.pdfjsLib` set before the import.
5. T05 — smoke test: the module boots and three P01 values read back.
6. T06 — the id-coverage guard (428 requested, 0 missing).
7. T07 — `build/jsdomtest.sh`, one command, skipping cleanly.
8. T08 — proven red: remove one id, confirm T06 fails **naming that id**.
9. T09 — the ceiling written in `boot.mjs`, naming tier 3 as what covers the gap.

Acceptance:
- One command runs it; absent deps print a skip line and exit 0 — distinguished
  from a pass **by the line, not the code**.
- The suite reports a **non-zero test count**; a runner that discovers nothing
  also exits 0.
- The real `app.js` and the real `index.html` are loaded, not copies.
- The id guard is proven red against a removed id, and the run confirms **that**
  assertion failed rather than merely that something did.
- The ceiling is written where the harness lives.

#### P02.S02 — Tier 2: P01's client acceptance, retroactively *(done 2026-08-16, v1.103.2)*

**(grill pin, 2026-08-16: booting is not opening.)** S01's `getDocument()`
resolves to `null`, which is enough to boot and is exactly why S01 passed. It is
not enough to open: `setDocumentFromServer` assigns that null to `pdfDocument` and
then reads `pdfDocument.numPages` (`app.js:1245`). So T01 is a minimal document
object, sized by what the open path touches — the same discipline that produced
S01's 7-symbol surface.

Tasks:
1. T01 — a minimal document stub the open path can run against.
2. T02 — the shared reset on the **open** path (the pre-existing bug S03 fixed).
3. T03 — the shared reset on the **close** path.
4. T04 — `closeDocument()` restores the launch chrome, asserted as a transition.
5. T05 — `hasEditsSinceOpen()` from each of its four signals, plus the negative arm.
6. T06 — proven red: restore the pre-S03 `clearOverlays()`, confirm T02 fails.
7. T07 — each test names the P01 acceptance line it discharges.

Scope: express as `node --test` tests the P01 clauses that live in the client —
the shared reset clearing all four armed modes on **both** open and close,
`closeDocument()` restoring the launch chrome value-for-value, and
`hasEditsSinceOpen()` for each of its four signals.
Acceptance:
- Each of the four edit signals driven **on its own**, so a signal that never
  fires cannot hide behind the others.
- The open-over-open bug pinned: arm a mode, run the open path, assert cleared —
  red against the pre-P01.S03 `clearOverlays()`.
- Every test asserts its stimulus before grading the response.

#### P02.S03 — Tier 3: the browser harness *(done 2026-08-16, v1.103.3)*

**(grill pin, 2026-08-16: why this tier drives a browser at all.)** Raised at the
gate — should nib not be self-contained? Verified at the line: it is not.
`internal/browser/browser.go:25-44` execs an **installed** Chromium-family browser
with `--app=`, falling back to `xdg-open`; there is no bundled engine anywhere in
the tree. So tier 3 drives Chromium because **that is what nib drives** — a UI
harness avoiding browsers would be testing something no user runs. The
self-contained-app question is filed to `pending.md` as its own architecture
decision and does not block this tier.

`playwright-core` (14 MB, **no browser download**) pointed at the system browser,
rather than `playwright` (~130 MB of bundled engines): it keeps the harness on the
same engine as the product and adds no dependency nib does not already have.
Measured before grilling — against the real binary it reported
`canvasWorks: true, layoutWorks: true`, which are exactly the two things tier 2
declared it could not reach, and the launch state read back identically to tier 2.

Tasks:
1. T01 — `playwright-core` as a devDependency.
2. T02 — resolve the browser by nib's own candidate order, `NIB_UI_BROWSER` override.
3. T03 — `build/uirepro.sh`: build, launch headless under a throwaway `HOME`,
   enroll a key, run the tests, kill the server on every exit path.
4. T04 — two **distinct** skip paths: no driver, and no browser.
5. T05 — `test/ui/smoke.test.mjs`: launch state, plus canvas and layout
   non-degenerate — the row that justifies the tier.
6. T06 — the non-zero test-count guard, reusing S01's lesson.
7. T07 — proven red, naming the assertion.
8. T08 — the ceiling written in the script, including what it still does not cover.

Scope: `build/uirepro.sh`, sibling of `winrepro.sh` — build nib, launch it
headless on a fixed loopback port with a throwaway `HOME`, enroll a key over the
API, drive a real browser.
Acceptance:
- One command; skips cleanly when the driver is absent, in `winrepro.sh`'s shape.
- Runs against the real binary, not a mock server.
- Proven red once, naming the assertion expected to break.
- The ceiling written in the script.

#### P02.S04 — Tier 3: P01's end-to-end acceptance, retroactively *(done 2026-08-16, v1.103.4)*

**(grill pin, 2026-08-16: this is the last tier, which changes the rules.)** S02
could record a row `not exercised — tier 3 owns it` because a tier 3 was coming.
Nothing comes after this one, so a row this slice cannot exercise is **filed as a
standing gap**, never delegated. Stated up front so the convenient verdict is not
reached for at the end.

**(measured before the grill)** Driving the real app found three steps no reading
would have predicted, each surfaced by Playwright refusing to click an invisible
element: the Open dialog must be opened before `#pathInput` can be filled; the
sign/date/initial flags are **not** under `Sign` (`SIDEBAR_FOR.sign` is
`['library']` — the Flags panel belongs to `collaborate`, `app.js:6901-6907`); and
the working sequence is open → `[data-tab="collaborate"]` → `[data-marker="date"]`
→ click a page, verified to place **1 `.ovl`**. That last result is what makes
`overlayFields` and `overlayHistory` — S02's two `not exercised` rows — reachable
here.

Tasks:
1. T01 — `test/ui/fixtures.mjs`: a generated PDF, parameterized by page count.
2. T02 — explicit, counted dialog handling in the harness (Playwright
   auto-dismisses by default, which silently means "cancel").
3. T03 — the P01 flow: prompt, cancel-leaves-untouched, confirm, empty state
   **including the thumbnail grid**, which tier 2 could not assert.
4. T04 — `overlayFields` alone.
5. T05 — `overlayHistory` alone (place then delete).
6. T06 — the mid-build thumbnail close on 80 pages, in-flight count captured
   **at close time**.
7. T07 — `/api/close` failure injected; nothing tears down.
8. T08 — reopen after close.
9. T09 — proven red, naming the assertion.

Scope: script the flows driven by hand during P01 — the empty state value-for-value
against the launch markup, Close/cancel/confirm, reopen, the mid-build thumbnail
close, and the injected-failure path.
Acceptance:
- The non-zero probe precedes every zero read (the prompt driven before "no
  prompt"; the control enabled before disabled).
- The mid-build close asserts the build was **in flight**, or reports
  `not exercised`.

#### P02.S05 — The verify contract *(done 2026-08-16, v1.103.5)*

**(grill pin, 2026-08-16: the slice is smaller than written, and one item nearly
evaporated.)** Two of its three stated jobs were already done by S01 and S03 —
tiers 2 and 3 each carry a ceiling naming the tier that covers its gap. What is
actually left:

- **`CLAUDE.md` is git-ignored** in this repo ("project instructions stay local"),
  so STANDARDS §10's "written into CLAUDE.md as the build/verify contract" would
  put the contract where a fresh clone cannot see it. Resolved by writing a
  committed copy and having `CLAUDE.md` point at it — the standard's intent
  without its only copy being invisible.
- **`README.md` has no Build/Develop section at all**; it is user-facing feature
  documentation, so the contract does not belong there either.
- **Tier 1 has no ceiling of its own** — it is described only inside tier 2's
  file, which is a chain documented from its middle link.
- **The browser-candidate drift guard**, proposed at S03's gate. `uirepro.sh`
  duplicates `internal/browser.chromiumCandidates()`; the guard is what makes that
  duplication acceptable rather than a debt, and it is the item that would have
  quietly evaporated if nobody wrote it down.

Tasks:
1. T01 — `CONTRIBUTING.md`: three commands, three ceilings, read as a chain.
2. T02 — tier 1's ceiling written there (it has no file of its own).
3. T03 — `CLAUDE.md` points at it.
4. T04 — `verify_test.go` at the root, beside `notices_test.go`: the commands
   exist, are executable, and are named in the contract.
5. T05 — `internal/browser/browser_test.go`: the bash list matches the Go list.
6. T06 — both guards proven red, each naming its assertion.

Scope: each tier's ceiling written where its harness lives and naming the tier
that covers its gap; `CLAUDE.md` gains the build/verify contract naming all three
commands (STANDARDS §10).
Acceptance:
- All three commands named in `CLAUDE.md`.
- Each tier's ceiling names the tier that covers the gap, so a delegation is
  explicit rather than a silence.

### P03 — Document identity, server side *(done 2026-08-16, v1.103.14)*
Goal: the server holds N documents and every document-touching endpoint says
which one it means. Refs: D6, **D7**, D8, D9, D10, **D15**.
Exit criteria:
- `s.doc` replaced by a registry + active id behind one accessor; the **16** nil
  guards keep their shape. *(tier 1)*
- Every document-touching route carries an id. *(tier 1 for the routes; tier 2 for
  the client always sending one — see D15's critical pin)*
  **(reality drift, 2026-08-16)** This clause used to end *"; `/api/undo` and
  `/api/redo` stop being bodyless"* — struck, because **D15 chose a header
  precisely so no body schema changes**, and names those two routes as the example
  it accommodates. The criterion predates D15 and assumed a body was how an id
  arrives. The requirement (both are addressable) stands and is met; only the
  mechanism was wrong. See P03.S02.
- Per-document rings under one global byte budget; single-document behaviour
  unchanged, proven by the existing Go tests **plus the probe below**. *(tier 1)*
- Arrivals (co-sign, p2p) open a new document rather than replacing one. *(tier 1)*
- **(plan-review pin, 2026-08-16)** A **two-document probe** that is red without
  the registry: open two documents, address an operation to the *inactive* one,
  and assert the active one was not touched. Stated as its own criterion rather
  than appended to the "behaviour unchanged" bullet, because those existing tests
  pass today and would pass against a registry that ignored ids entirely — a
  criterion that cannot fail is not one.
- **(plan-review pin, 2026-08-16)** The **all-tabs-stale** case resolves to the
  launch empty state, not to N error tabs. D15 gives 409 for one closed document;
  a *server* restart makes every id stale at once, and P06's reload pin covers
  restoring tabs from the server, not the server having none to restore. Observed
  by restarting the server under a client holding ≥2 ids and reading the resulting
  UI state.

**(plan-review pin: refs and counts, 2026-08-16)** `Refs` gained **D7** and
**D15** — D15 is *the* decision on how the id travels and carries the `/api/pdf`
query-param exception written explicitly *"so it is not 'fixed' into uniformity
later by someone who did not know why it exists"*, and the person most likely to
do that fixing is the one reading this phase. The guard count was `~14`; it is
**16**, measured twice (P01.S02's inventory, and again at this review). Counts in
prose go stale — prefer citing the guard.

**(phase-open, 2026-08-16 — slices firmed against the codebase as it now stands
and against the plan-review pins.)** Measured rather than inherited: **16** guard
sites (the plan said ~14), **7** `setDoc` callers, and **29 of 78** registered
routes touch document state directly — D15's "~30 of ~68" is right on the count
and stale on the denominator, because P01 added routes.

**S02 and S03 are split deliberately.** "The server accepts an optional id" is
independently shippable and harmless; "the client always sends one" is a different
claim needing a different guard. Landing them together would let the second hide
inside the first's green — the exact shape of the plan-review's critical finding.

**(phase close, 2026-08-16, v1.103.14)** Exit criteria, every clause, split on `and`:

- ✅ `s.doc` replaced by a registry + active id behind one accessor — the field is gone;
  the six textual matches left in the tree are comments describing the history.
- ✅ …**and** the 16 nil guards keep their shape — `TestEveryDocumentResolutionIsHandled`
  asserts 17 `resolveDoc` + 7 direct `docFor` sites, each count changed deliberately.
- ✅ Every document-touching route carries an id (mechanism amended: header, not body).
- ✅ Per-document rings **and** ✅ one global byte budget — S04, plus ADR-003.
- ✅ …**and** single-document behaviour unchanged — *within the premise pin*: identical
  wherever the depth cap binds, deliberately different above 256 MiB, which is what
  bounding the undo+redo pair means. Not claimed unqualified.
- ✅ Arrivals open a new document rather than replacing one — **criterion amended** to
  co-sign only; `saveReceived` writes a one-way transfer to disk and never touches the
  registry, so the "p2p" half named a path that does not exist.
- ✅ (plan-review pin) The two-document probe — `TestOperationAddressedToInactiveDocumentLeavesActiveAlone`, which found the defect it now guards.
- ⏳ (plan-review pin) The all-tabs-stale case — **not exercised, carried to P06** for
  the reason stated below; P03 delivers the 409 that makes it expressible.

**Three findings this phase produced that were not in it:** a live data race in shipped
code on `/api/doc` (S04, pre-existing); an eviction pass that discarded the active
document's history (S04, caught in its own diff review); and a silent unpinning
regression introduced by S03 and found by S05's deepdive. All three were invisible to
the tests that existed when they were written, and all three now have red-probed guards.

**The all-tabs-stale pin cannot be discharged in this phase.** It says the case
resolves to the launch empty state "not N error tabs" — and there are no tabs until
P06, so N is 1 and the clause is unfalsifiable here. P03 delivers the 409 that makes
it possible; the resolution **carries to P06**, on the mechanic P01.S03→S04 used.

#### P03.S01 — the registry, behind one accessor *(done 2026-08-16, v1.103.9)*
Scope: `s.doc` becomes an ordered registry + active id + a monotonic,
epoch-prefixed counter, all behind one `activeDoc()` accessor. **Internal only** —
no wire change, no behaviour change. Refs: D6, D7/ADR-001.

Tasks:
1. T01 — registry, `activeID`, counter and per-process epoch on `Server`.
2. T02 — `activeDoc()` as the single accessor; the 16 sites keep their guard shape.
3. T03 — `setDoc`'s 7 callers stay on **replace**; the replace-vs-add split is S05's.
4. T04 — `docResponse()` and the commit helpers operate on the active document.
5. T05 — the shape guard: 16 sites, each guard within 5 lines of its read.
6. T06 — ids advance across close/reopen; two servers differ in epoch.
7. T07 — the existing Go suite green and unchanged.

Acceptance:
- The registry holds the document and names it active — asserted on the **registry**,
  not on app behaviour.
- All 16 guards keep their shape; the count is asserted, not described.
- Ids strictly increase across a close and reopen; two servers differ in epoch.
- Every existing single-document test passes, **with no assertion changed**.
  *This suite is a regression net, **not** evidence the registry works* — it passes
  today and would pass against a registry that ignored ids entirely. The evidence
  is S02's two-document probe.
  **(reality drift, 2026-08-16: "unchanged" was too strong.)** Four tests
  hand-constructed `&Server{doc: …}`, reaching past the accessor into a field the
  registry removes, so they could not compile untouched. They now go through a
  shared `openTestServer` helper that calls the real `setDoc` — which is better
  than the original, since hand-assembling `docs`/`activeID`/`nextSeq`/`epoch` in a
  test would be a second, silently-drifting copy of the id invariants ADR-001 turns
  on. **No assertion changed**; the edits are construction only, and that is the
  clause worth holding.
- `setDoc` still replaces; nothing arrives as a new document yet.

#### P03.S02 — the id on the wire *(done 2026-08-16, v1.103.10)*

**(reality drift, 2026-08-16: two decisions contradict, and the older one's
*premise* is what is wrong.)** This phase's exit criterion 2 says *"`/api/undo` and
`/api/redo` stop being bodyless."* **D15 says the opposite, and says it as the
reason for its own choice**: a header *"carries the id across all three [body
shapes] — bodyless POST (`/api/undo`, `/api/redo`), JSON, and multipart — **without
editing a single body schema**."*

The criterion predates D15: it was written while the transport was unsettled and
assumed a body was how an id would arrive. D15 settled the transport at Stage 3 and
superseded that mechanism without the criterion being re-read — the accretion bug,
two decisions each sound when written. It also survived the plan-review, which read
"carries an id" and not the clause behind the semicolon.

**The requirement stands; only its premise falls.** Undo and redo must be
addressable — they are, by the header — so they **stay bodyless**, and building the
criterion literally would edit exactly the schemas D15 exists to leave alone.

**(trace, 2026-08-16: the wire surface is a third smaller than "every
document-touching route".)** Routes split three ways: **23 address** an existing
document and need the id; **6 create** one (`/api/open`, `/api/open-url`,
`/api/upload`, `/api/combine`, `/api/office`, `/api/session/initiate`) and have
nothing to name; 49 touch no document at all. Putting an id on the create six would
be a parameter that cannot mean anything, and an invitation to a later reader to
wonder what it does.

Tasks:
1. T01 — `docFor(r)`: header → query param → active default; a distinguishable
   not-found.
2. T02 — the 23 addressing routes resolve through it; the 6 create routes untouched.
3. T03 — 404 and 409 differ in **body** as well as status; they drive different
   client behaviour (blank the app vs drop a tab).
4. T04 — the epoch is compared **before** Seq.
5. T05 — undo and redo stay bodyless, carrying the header.
6. T06 — **rewrite** S01's guard to the new idiom, count re-derived. It is coupled
   to the idiom this slice replaces, so it will go red; loosening its regex until it
   passes is how a guard stops guarding.
7. T07 — the two-document probe, 409-by-body, epoch-409, and `?doc=`.

**(reality drift, 2026-08-16 — the grill under-scoped this, and writing the probe
is what found it.)** "The 23 addressing routes resolve through `docFor`" is not
sufficient, because **the mutating routes do not resolve a document at all.**
`outline.go`, `export.go`, `pages.go` and `redact.go` — the same four P01.S01 dealt
with — take their bytes from the request and commit through `commitMutation` /
`commitBarrier`, which resolve **the active document internally**. So an operation
addressed to document B would commit its result into document A: precisely the
corruption ADR-001 exists to prevent, arriving through the helper rather than
through a forgotten header.

The probe could not have been written without discovering this, which is the
argument for writing probes before believing plans.

8. T08 — `commitMutation`/`commitBarrier` take the **target document** rather than
   resolving active; all 8 callers pass the one they resolved, and the four
   unguarded routes resolve first. The helpers keep their single-lock
   test-and-write (P01.S01's TOCTOU fix) — the test just becomes "is this document
   still registered" instead of "is anything active", which is strictly stronger.

Scope: `X-Nib-Doc` on document routes; `/api/pdf?doc=` (D15's named exception);
optional, defaulting to active, for the CLI and pre-existing tests; **409** for an
id the server does not hold, including an epoch mismatch; `/api/undo` and
`/api/redo` stop being bodyless. Refs: D6, D15.
Acceptance:
- **The two-document probe** (P03's pinned criterion): address an operation to the
  *inactive* document and assert the active one was untouched.
- 409 is distinguishable from 404 in body as well as status.
- An id from a previous process epoch is 409, not silently accepted.

#### P03.S03 — the client always sends an id *(done 2026-08-16, v1.103.11)*

**(deepdive, 2026-08-16: the load-bearing task is a wire addition the slice does
not mention.)** `docResponse` carries no id — Name, Path, CanSave, Signature,
Flags, CanUndo, CanRedo, and nothing the client could echo back. The enforcement is
**impossible** until the server tells the client what its document is called, so
T01 is that addition and the header plumbing is wiring behind it.

Also traced: **nothing bypasses `apiFetch` today.** Exactly three bare `fetch()`
calls exist (`/api/status`, `/api/ssh/unlock`, `/api/update/check`) and none is a
document route. That is the property the whole enforcement rests on, it is true
now, and nothing keeps it true — hence T06.

**The attachment is unconditional, not route-scoped.** A client-side list of
document routes would be a second copy of the server's knowledge, in a different
language, drifting on the next route added; the header is inert on the 49 routes
that never call `docFor`, so over-sending costs nothing and a drifted list costs a
route that silently falls back to the active document — the finding this slice
closes.

Tasks:
1. T01 — `docResponse.id` (server).
2. T02 — the client stores it.
3. T03 — `apiFetch` attaches `X-Nib-Doc` whenever a document is open.
4. T04 — `/api/pdf?doc=` where pdf.js builds the URL (D15's exception).
5. T05 — tier-2 guard: a document-route call carries the header, and the id matches
   the one the server issued. Proven red by removing the attachment.
6. T06 — static guard: the three `apiFetch` bypasses stay exactly those three.

**What this slice does NOT do.** The critical pin says the client sends the calling
view's **captured** id; S03 sends the **current** one. Capturing is operation
pinning, which is **P04**. With one view they are the same value, so the difference
is invisible today and total once P05 lands — recorded so the pin is not read as
discharged here, because a pin marked satisfied is one nobody re-reads.

Scope: the critical pin's enforcement — `apiFetch` attaches the active view's id to
every document route, so a call site cannot omit it by forgetting. Refs: D15's
critical pin.
Acceptance:
- A tier-2 test asserts `X-Nib-Doc` on every document-route `apiFetch`, **proven red
  by removing the attachment**.
- `/api/pdf` carries its id as a query parameter, not a header (D15's exception).

#### P03.S04 — per-document rings, one global budget *(done 2026-08-16, v1.103.12)*
Scope: D8 — rings per document, `maxUndoBytes` shared across all open documents and
bounding the **undo+redo pair** (the 2× → 2N× pin), evicting inactive documents
first and **observably** (the plan-review pin).

**(reality drift, 2026-08-16, deepdive)** the slice's stated scope is a data-structure
move and its real load-bearing task is a wire change. `docResponse()` takes no argument
and reads the **active** document plus the two server-global rings (`server.go:552-557`),
so "the evicted document's own observation" does not exist to be asserted; 25 call sites
must pass the document they resolved. Found by tracing the seam, not visible from the
slice as written.

**(reality drift, 2026-08-16, deepdive — pre-existing defect on this slice's lines)**
`docResponse` releases `s.mu` at `server.go:557` and *then* reads `doc.path`, `doc.sig`
and `doc.data`. That is a live data race against `undo.go:54`, demonstrated with `-race`
on an ordinary `/api/doc` + `/api/pages` pair — it reproduces today, with one document,
on a user running two browser panes. Not introduced here; fixed here as T01 because T02
rewrites the same lines (Dan's call, 2026-08-16).

**(premise pin, 2026-08-16, Dan's call)** the acceptance clause "single-document
behaviour byte-identical" predates the plan-review's undo+redo pin and contradicts it:
counting redo toward the budget *is* a change to single-document behaviour, taking one
document's ceiling from ~512 MiB across the two stacks to 256 MiB. The premise is struck,
the requirement kept — behaviour is identical wherever the depth cap binds, which with
`maxUndoDepth = 30` against ordinary PDFs is every realistic case, and deliberately
different above 256 MiB. The pin exists so the ledger cannot report "byte-identical"
against a reading that silently excluded the case the pin added.

**(scope note, 2026-08-16)** eviction is **unreachable through the GUI** until P05 lands
arrivals — the real app cannot open a second document — so no clause of this slice is
exercisable at tier 3. Tier 1 drives the registry directly; tier 3 stays in the run as a
regression check that single-document undo/redo is untouched. Those are different claims
and the ledger reports them as different rows.

Tasks:
- T01 — `docResponse(doc *document)`: the response is built entirely under one lock hold
  and re-checks registration inside it. Closes the race above; the `-race` probe that
  found it is kept as a standing fixture.
- T02 — `undo`/`redo` move from `Server` onto `document`; clear/trim become
  document-scoped; the 25 `docResponse()` call sites pass their resolved document.
- T03 — `trimHistoryLocked`: total = undo+redo across **all** documents against one
  `maxUndoBytes`; inactive documents' histories are evicted **whole** (entry-by-entry
  cannot converge when the bytes sit in redo, and a partially-truncated history is the
  silent eviction the pin refuses), then the active document's undo trims as today.
- T04 — `historyEvicted` on the document and on the wire: the pin's own observation,
  because `canUndo:false` alone is indistinguishable from "you never had any history".
- T05 — tests: eviction fires and is red if silent; the evicted document's own
  `docResponse` reads false; single-document depth behaviour unchanged.

Defaults taken (logged, reversible):
- Eviction order among inactive documents is **open order, oldest first**. LRU is the
  better model but there is no last-active signal to record until switching exists (P06);
  filed rather than faked.
- `maxUndoBytes` becomes the **global** figure, not per-document — per-document would be
  256N MiB, which is what the pin refuses.
- The budget is a bound with one named exception, not a hard cap: the active document
  always keeps its last undo entry, as today.

**(diff-review finding, 2026-08-16)** `trimHistoryLocked`'s parameter was named
`active` and was in fact *the document that just grew* — different documents whenever an
operation is addressed to an inactive one, which is the case this phase exists to
support. The eviction pass therefore skipped the addressed document and evicted the
genuinely active one: the acceptance clause inverted, green against all five eviction
tests, because every one of them grows the active document and cannot tell the two
apart. Fixed with a three-tier order and a dedicated regression test; recorded in
ADR-003, because the same conflation is the natural way to write it.

Acceptance:
- Single-document behaviour unchanged wherever the depth cap binds (see the premise pin).
- Two documents past the budget evicts the inactive one, and the evicted document's
  `canUndo` reads false — its own observation, red if eviction is silent.
- Eviction is distinguishable from never-having-had-history on the wire (the T04 flag),
  red if the only evidence is `canUndo:false`.
- `/api/doc` concurrent with a mutation is clean under `-race`, red against the code as
  it stood before this slice.

#### P03.S05 — arrivals open a new document *(done 2026-08-16, v1.103.13)*
Scope: D10 — `setDoc` splits into replace-active and add-new; the arrival paths
(`session.go:252,561`) add. Refs: D10.

**(reality drift, 2026-08-16, deepdive)** the phase exit criterion says "Arrivals
(co-sign, p2p) open a new document rather than replacing one", but the p2p one-way
receive does **not** open a document at all — `saveReceived` (`session.go:255`) writes
the file under `~/nib` and never touches the registry. Only the two co-sign paths call
`setDoc`: `session.go:252` (background, the receiving peer) and `session.go:561`
(the dialing peer, with a user waiting on the response). The criterion's p2p half is
about a path that does not exist; the ask is unchanged for the two that do.

**(regression found by this slice's deepdive, 2026-08-16 — live at v1.103.12)**
P03.S03 made `apiFetch` attach `X-Nib-Doc` unconditionally, and `apiFetch` throws only
on **401** — a **409 comes back as an ordinary response**. `reloadOpenDoc`
(`web/app.js:1044`), the refresh that runs after a co-signature is applied out of band,
does `setDocumentFromServer(await res.json())`. So on the receiving peer:

1. the arrival replaces the document, unregistering the client's id;
2. `/api/doc` carrying that id answers 409 with `{"error": "..."}` (tier 1: proven);
3. `apiFetch` returns it as data (tier 2: proven — the body reaches the caller and
   carries no `id`);
4. `docMeta` becomes the error object, so **`docMeta.id` is undefined and every later
   request stops sending the header** — the session silently reverts to the unpinned
   path S03 existed to close, and `canSave` goes undefined so Save breaks.

The document still *renders*, because `/api/pdf` with no id falls back to the active
one — which is why this is invisible rather than loud.

**S05 does not fix it by adding documents; it makes it worse.** Once arrivals *add*,
the client's id stays valid and `/api/doc` answers about the **old** document, so the
co-signed result never appears at all. The root cause is that `reloadOpenDoc` asks a
question about the *session* ("what is active now?") through a call pinned to a
*document*. Fixed here as part of the slice.
**(criterion amended, 2026-08-16, autonomous — evidence above)** the phase exit
criterion's "(co-sign, p2p)" is narrowed to co-sign. `saveReceived` writes a received
one-way transfer to `~/nib` and never touches the registry, so there is no p2p arrival
that opens a document to change. Making one would be a behaviour change to a shipped
feature and new scope; the criterion is corrected to describe the two paths that exist.

Tasks:
- T01 — `setDoc` splits: `setDoc` keeps its replace contract, `addDoc` adds a document
  and makes it active. Id minting and the registry write are shared; `addDoc` clears
  nothing, which is the whole difference.
- T02 — the two co-sign paths (`session.go:252`, `session.go:561`) call `addDoc`.
- T03 — ~~`apiFetch` stops handing a 409 body to its caller as data~~ **(struck by this
  slice's own diff review, 2026-08-16)**. Built as written first, then reverted: the
  review counted the document-route call sites and found **20, of which 15 already
  handle a refusal correctly** as `if (!res.ok) { toast('…'); return; }`. Throwing in
  `apiFetch` would have converted fifteen clear, user-visible failures into unhandled
  rejections that say nothing — a strictly worse outcome, introduced while fixing an
  unrelated defect. `reloadOpenDoc` was the **only** site missing that check, which is
  the whole of the original bug. Replaced by:
  - T03a — `reloadOpenDoc` gains the `res.ok` check every other site already had.
  - T03b — `setDocumentFromServer` refuses a refusal body (`meta.error`, which is exact:
    a 200 body never carries that field, and "no id" would wrongly reject the legitimate
    empty response a closed document produces). The guard sits at the **sink** because
    that is where the mistake does damage and where one check covers all twenty callers,
    any of which could grow the same omission.
- T04 — `reloadOpenDoc` asks its question **un-pinned**, deliberately and with the
  reason written at the call site: after an out-of-band arrival it is asking "what is
  active now?", which is a question about the session, not about a document.
- T05 — tests: tier 1 (an arrival adds, the previous document survives with its
  history, the arrival is active); tier 2 (a 409 never becomes `docMeta`, and the
  refresh sends no id).

**Arrivals add AND activate** (default taken, 2026-08-16). Add-without-activate is the
right P06 behaviour — a background arrival should badge a tab, not seize the view — but
there is no tab strip until P06, so an added-but-inactive document is unreachable: the
feature would ship as "your co-signature completed and you can never see it". Activating
also preserves today's observable behaviour exactly, so the change is confined to the
part that was invisible anyway (the previous document surviving). Revisit at P06.

Acceptance:
- A completed co-sign adds a document rather than replacing the open one.
- The previously open document survives the arrival **with its undo history intact** —
  its own observation, red if the arrival clears it.
- The arrival is the active document, so the existing refresh still shows it.
- A refusal body never becomes the document, and the session stays pinned after one —
  red against the code as it stood (`docMeta.id` became undefined and every later
  request silently unpinned).
- `apiFetch` does **not** throw on 409, red if the reverted design returns.
- The post-arrival refresh carries no `X-Nib-Doc`, red if it pins to the stale id.
- The other five `setDoc` callers still replace, asserted individually.

### P04 — Operation pinning, client side *(done 2026-08-16, v1.104.3)*
Goal: no operation acts on a document it did not capture at its start. Refs: D7.
Exit criteria:
- The 13 corrupting post-await sites capture their document and id before the
  first `await` and carry it through.
- The ~25 mislabeling sites (all through `exportBase()`) name the document they
  actually exported.
- A harness test proves the `save()` case: begin a save, switch documents mid-
  flight, and assert the *other* document's file is untouched.
**No user-visible output — this is the safety phase and it cannot be skipped.**

**(phase close, 2026-08-16, v1.104.3)** Exit criteria, every clause, split on `and`:

- ✅ The corrupting post-await sites capture their document and id before the first
  `await` — **6, not 13**; the phase-open count was wrong and agreed with the plan by
  coincidence (see the correction above). 5 self-corrupting, 1 caller-corrupting.
- ✅ …**and** carry it through — `apiFetch` gains `docId`, decided by presence rather
  than truthiness so a captured-but-missing id cannot silently fall back.
- ✅ The mislabeling sites (all through `exportBase()`) name the document they actually
  exported — **20 sites across 19 scopes**; the plan said ~25.
- ✅ A harness test proves the `save()` case — `TestSaveGoesToTheCapturedDocument`
  `NotTheCurrentOne`, which under the pre-P04 client fails at its *stimulus* gate
  because A's file is never written at all: the bytes went to B.
- ⏳ **Not exercised at any tier:** the join — a real browser saving while a real
  document change lands inside the request's flight window. No harness can arrange it.

**The phase's own lesson, recorded because it cost a slice and a half:** the scanner
used to size this work was an instrument, was wrong, and had no red fixture. It now has
one, checked against a synthetic corrupting function and a synthetic safe one. *A
measurement used to size work needs a red fixture before its number is quoted.*

**(phase-open, 2026-08-16 — slices firmed against the codebase as it now stands,
with every count re-derived rather than inherited.)**

The plan's counts came from a planning-time deepdive. Re-measured against the tree:

- **"38 post-await document reads"** → the scan finds **19 async functions** holding
  **~33** such reads. The number moved because prose counts do; more importantly, the
  *category* is too broad to act on. Most post-await reads are of `pdfDocument`, the
  client's own rendering object — reading a stale one costs a stale render, not a
  corrupted file.
- **"13 can corrupt"** → **the true number is 6**, and the "confirmed exactly 13"
  recorded here at phase-open was **wrong**. See the correction below.

**(correction, 2026-08-16, P04.S02 — the phase-open measurement was wrong and it agreed
with the plan by coincidence.)** The scanner used at phase-open asked *"does the word
`await` appear earlier in this function than the `apiFetch` call?"* In
`const res = await apiFetch(...)` the word `await` sits **before** `apiFetch`, so a
call that **is** its own first await counted as preceded by one. Eight functions were
reported as corrupting that structurally cannot be — `runSanitize`, `postDecrypt`,
`loadAttachments`, `extractAttachment`, `openOutlineEditor`, `doUndo`, `doRedo`, and
`embedFlags` — and the total came to 13, matching the plan's number exactly. That match
was reported as independent confirmation of the plan's count. It was a coincidence
between two different errors.

The corrected rule strips the introducer belonging to the call itself before looking for
a preceding `await`. Re-measured:

- **5 self-corrupting sites** — an await inside the function precedes its own mutating
  call: `save` (S01), `runOCR`, `flattenPages`, `assembleBlob`, `compressBlob`.
- **1 caller-corrupting site** — `embedFlags`, whose `apiFetch` *is* its own first
  await, but which receives document-derived bytes from callers that have already
  awaited to produce them. **A scan over await-ordering inside a function is
  structurally blind to this shape**, so it is named in the code rather than left to the
  guard.
- **6 in total.** The other seven of the original eight take no document-derived payload
  (`method`, `password`, `name`, or nothing) and act on the current document by
  definition, so there is nothing for them to capture.

The lesson is the one the plan already states about counts in prose, applied to a
measurement: *the scanner is an instrument and needed its own red fixture.* It now has
one — checked against a synthetic corrupting function and a synthetic safe one, rather
than against the source it is meant to judge.
- **"~25 mislabeling sites (all through `exportBase()`)"** → **20 call sites**, and the
  "all through `exportBase()`" half is exactly right.

**What makes this phase fixable at all is P03.** The remedy is to send the **captured**
id, and that is only safe because ADR-001 guarantees ids are never reused: a captured id
whose document is gone gets a **409 and the operation is refused**, rather than being
silently redirected at whatever now holds that identity. The client-side law rests on
the server-side one.

#### P04.S01 — capture at the transport, and the worst site *(done 2026-08-16, v1.104.0)*
Scope: `apiFetch` gains `docId` — an explicitly captured id that overrides the current
one, the exact counterpart of S05's `unpinned`. `save()` captures its document before
its first `await` and uses the captured id **and** the captured `canSave`. A guard test
freezes the remaining unpinned sites so the set can only shrink. Refs: D7, ADR-001.
Tasks:
- T01 — `apiFetch` gains `docId`. Decided by **presence**, not truthiness: a caller that
  captured a document and found no id has a bug, and falling back to the current id
  would send exactly the request pinning exists to prevent, through the option meant to
  stop it.
- T02 — `save()` captures before its first `await`; the captured document supplies the
  id, `canSave`, and the identity check before the badge/reload.
- T03 — the scan guard, frozen as a **list** of the 12 remaining sites rather than a
  count — a count sits still while one site is fixed and another introduced.
- T04 — tier 1 proves the file-level property; tier 2 proves the client's half.

**(diff-review findings, 2026-08-16, both fixed in slice)**
- The post-save check compared `docMeta !== doc` by **object identity**, but
  `setDocumentFromServer` builds a fresh meta object every run, including for the *same*
  document — so any concurrent refresh would have reported "the document changed" and
  silently skipped a reload that was owed. Compares ids now.
- `apiFetch` decided pinning by truthiness, so a captured-but-missing id fell back to the
  current one. Unreachable today (an open document always has an id), fixed anyway
  because the failure mode is the phase's own defect arriving through its own remedy.

Acceptance:
- `save()` reads no document state after its first `await`.
- Begin a save, change the document mid-flight, and the **other** document's file is
  untouched — red against the code as it stands, where A's bytes are written to B's path.
- A captured id whose document is gone yields a refusal, not a redirect.
- A guard lists exactly the **12** remaining unpinned mutating sites; a **new** one fails
  it. Red if the scan matches nothing.

#### P04.S02 — the remaining sites *(done 2026-08-16, v1.104.1)*
Scope: the other mutating sites capture their document before their first `await`; the
guard's allowlist empties. **Five, not twelve** — see the correction above, which this
slice produced. Refs: D7.

Capture point, decided here: a helper captures **at its own entry** via a default
parameter, which is a defined moment before any await inside it — and a caller that has
already awaited passes its own captured id explicitly, overriding. `els.reduceGo`,
`sendableForm`, `saveForSigning` and `bakedBytes` are the callers where entry is already
too late, and each captures at the top of the operation instead.
Acceptance:
- The guard's allowlist is **empty**, and the guard is still red against a newly
  introduced unpinned site — proven by unpinning each fixed site in turn.
- The scanner has its own red fixture, because with an empty allowlist "no unpinned
  sites" is both the pass and what a broken scanner reports.
- Each site's behaviour is otherwise unchanged (the existing suites stay green).

#### P04.S03 — exports are named for the document they came from *(done 2026-08-16, v1.104.2)*
Scope: the export NAME is captured at operation entry rather than `exportBase()` being
called at the point of use — 20 call sites across **19 scopes**, each capturing into
`exportName` before its first `await`. Refs: D7.

**(shape changed during implementation, 2026-08-16)** The slice planned to pass a
captured *document* into `exportBase(doc)`. Capturing the resulting *string* at entry is
the same guarantee with a smaller surface: `exportBase` keeps its signature, nothing
threads a document through 19 scopes, and the property — the name is fixed before the
operation can be overtaken — is identical. The mechanism was the plan's; the guarantee
is what the acceptance asks for.
Acceptance:
- An export begun on A and resolving after the document changed is named for **A**.
- Red against the current code, which names it for whatever is current when the blob
  resolves.
- The signing-workflow names (`-cosigned`, `-for-signing`) are covered by name, since
  those are the ones where the filename is how a user tells two documents apart.

### P05 — Per-view state and viewers *(done 2026-08-17, v1.105.20)*
Goal: each document owns its viewer, its DOM and its state records. Refs: D3, D5,
D11, D12.
Exit criteria:
- Per-view `PDFViewer`/container; inactive views hidden, never destroyed.
- The seven silent-loss bindings plus `signLocked`, `lastSig`, `docGen` **and
  `overlayHistory`** are per-view; `detachField`/`reattachField`/`relayoutOverlays`
  resolve the owning view, not the active one. **(amended 2026-08-16, S03 deepdive —
  `overlayHistory` was missing from the phase-open enumeration; see the measurement
  correction below.)**
- The 26 pointer listeners re-homed to the stable parent; cleanup sweeps
  view-scoped.
- Re-fit and dpr-heal on activation (a view that loads hidden gets no scale).
- The two sidebars show the active document's content and nothing else.

**(phase close, 2026-08-17, v1.105.20)** All six slices shipped (v1.105.0→v1.105.5) and
the three-part close-out ran: the full-repo `/code-review`
(`<project-memory>/code-reviews/P05-phase-close-2026-08-17.md`, 96 findings, 8 criticals,
**fully dispositioned** — 47 fixed, 5 no-change-needed, 44 carried by named pending items),
the remediation (v1.105.6→v1.105.19, seventeen commits), and the **graduation pass over all
34 seam rows** (`<project-memory>/instruments/P05.md`) — all 34 `keep-live`, one row (V29)
corrected, and the pass's own blind spot closed with a new published-field reader scan.
Five tiers green.

Exit criteria, every clause, split on `and`:

- ✅ Per-view `PDFViewer` — 0 module-scope declarations of the four pdf.js objects, 0 bare
  reads outside `newView()` (V7–V9).
- ✅ …**and** per-view container — 0 `els.viewerContainer` references (V10); each view
  builds its own at `app.js:1439`.
- ✅ …**and** inactive views hidden — `activateView` hides the outgoing container, grid and
  list; `newView` hides when it is not the first.
- ✅ …**and** never destroyed — `container.remove()` occurs only on close and on the
  arrival-load-failure cleanup, never on a switch.
- ✅ The seven silent-loss bindings **and** `signLocked`, `lastSig`, `docGen` **and**
  `overlayHistory` are per-view (V1–V5, V30) — **fourteen, not thirteen**, per the S03
  amendment above.
- ✅ …**and** `detachField`/`reattachField`/`relayoutOverlays` resolve the owning view —
  all three take an `owner` and use it (`app.js:7255`, `:7259`, `:7454`); V6 refuses a bare
  `view` inside `relayoutOverlays`.
- ✅ The 26 pointer listeners re-homed to the stable parent — measured 26 on
  `els.viewerWrap`, 0 on a per-view container (V11, a floor plus two negative populations).
- ✅ …**and** cleanup sweeps view-scoped — V13/V15, two-directional against a decoy.
- ⏳ Re-fit on activation **and** ⏳ dpr-heal on activation — **not exercised, carried to
  P06**, on the mechanic P01.S03→S04 and P03's all-tabs-stale pin both used. Present at
  `app.js:1619` and `:1625-1628`; unobservable by any tier, for the reason below.
- ✅ The two sidebars show the active document's content (V27/V29) **and** ✅ nothing else
  — V28, against a decoy grid rather than a second view, and stated that way.

**14 met, 0 not met, 2 not exercised. Closed over the two by Dan's call, 2026-08-17** — the
ledger rule allows exactly that and requires it be recorded, so this is the record.

**Why clause 4 cannot be discharged in this phase.** Both halves are unobservable, not
unbuilt: jsdom reports every `clientWidth` as 0, tier 3 has one view, and the only path
that creates a second view in production is a co-sign arrival, which needs a live pinned
peer. P06 adds the way to create one **without** a peer — which is what a test needs — so
the clause becomes drivable there and is P06's first acceptance work.

**377 references across 13 bindings in a 7,222-line file** — measured, and it is what
drives the slicing below: `pdfDocument` 148, `viewer` 67, `docMeta` 37, `overlayFields`
35, `eventBus` 16, `signLocked` 16, `redactMarks` 13, `docGen` 12, `outlineItems` 10,
`lastSig` 8, `linkService` 7, `originalName` 6, `findController` 2.

**(measurement correction, 2026-08-16, found by S03's deepdive — the enumeration above
is short by one binding, and the missing one is safety-critical.)** `overlayHistory`
(`app.js:6446`) is declared at module scope and is not in the 13. It holds the client
overlay-edit undo/redo stacks, whose entries are **closures over overlay elements**
(`:6527-6533`), drained by `undoAny()` before the server ring (`:6538`). Shared across
views, Ctrl+Z in document A replays a command captured against document B's element —
the same irreversible-loss family as `redactMarks`, not the stale-label family. It is
therefore **fourteen** bindings, and the exit criterion above is amended to say so.
The lesson is P04's, again and exactly: *a count used to size work is an instrument, and
an instrument that has never been red-fixtured will happily report a population it did
not read.*

**(measurement correction, 2026-08-16, S04's deepdive — and this one is a 3x error, not a
one-binding error.)** The enumeration above counts *bindings with many references*. The
question activation actually asks is **what must swap when the active view changes**, and
that finds roughly **35**: the 13 here, `overlayHistory` (S06's), ~22 more per-document
bindings still at module scope, ~11 transient ones that must be **aborted rather than
restored**, and ~20 modal-state bindings that collapse to one rule. Named because the two
corrections have different causes and only the second is interesting: the first was a
missed row in a list; this one is a list that was answering a different question than the
phase needed. **The reference-count instrument was never wrong — it was never the right
instrument**, and nothing in the phase would have surfaced that until a slice tried to use
its output. The standout omission is `selectedPages`/`selAnchor` (`app.js:2822-2823`),
which drives the bulk rotate/delete/reorder bar off one document's page numbers: with two
views that is a destructive wrong-document operation, and it appears in no enumeration in
this plan. The 377/13 figures also count comment prose — the guard suite strips comments
(`view.test.mjs:34`), so the population it actually polices is smaller than the figure
that sized the phase. **`overlayHistory` is P05.S06's** (added below); no earlier slice
claims it.

**(decision, 2026-08-16 — swap-on-switch is refused.)** There is a tempting shortcut:
leave all 13 as module-level bindings and *save/restore* them at the switch boundary, so
the active view's values are copied in and out. It would cost zero reference churn
against 377 sites. **It is refused, and the reason is this plan's own history:** the
module-level bindings would then be a cache whose correctness depends on nothing async
reading them across a switch — exactly the class P04 just spent three slices closing on
the server-addressed side. Swap-on-switch reintroduces it on the client-state side,
where there is no id to check and no 409 to refuse. ADR-002's argument applies unchanged:
a single missed binding loses data silently. The explicit refactor is the price of not
having that failure mode.

**(decision, 2026-08-16 — the sidebars go per-view; the phase-open call the subsystem pin
asked for.)** The pin offered per-view containers (consistent with D3, costs hidden
thumbnail DOM) or rebuild-on-activation (cheaper, re-renders every thumbnail on every
switch). **Per-view containers** — rebuild-on-activation is precisely what ADR-002 exists
to avoid, and the memory cost is already accepted there and bounded by P06's document
cap. `docGen` is per-view either way: a shared token lets a background document's
finishing build abort the foreground's.

#### P05.S01 — the view record, and the bindings where sharing is a safety defect *(done 2026-08-16, v1.105.0)*
Scope: introduce the `view` record and move the **small, safety-critical** bindings onto
it — `redactMarks`, `signLocked`, `lastSig`, plus `docGen`, `outlineItems`,
`originalName` (~65 references). One view exists, so behaviour is identical. Refs: D5,
D11, D12, and the dimension-review pin on the three safety bindings.

**Sliced this way deliberately.** The three bindings the pin calls safety defects rather
than UI defects are also among the smallest, so they land first and are asserted before
the 220-reference bulk moves. Biggest-first would put the irreversible-destruction
binding behind the riskiest edit in the phase.
Acceptance:
- `redactMarks` is per-view, with a guard that marks drawn on one view cannot be baked
  through another — the worst outcome in this plan, because redaction commits through
  `commitBarrier`, which clears undo by design.
- `signLocked` is per-view, **and ambiguity resolves toward locked** — a guard asserts a
  locked document is still locked after a switch.
- `lastSig` is per-view, with a guard that the signature-details modal cannot show one
  document's verification result under another's name.
- Single-view behaviour unchanged; the existing suites stay green.

#### P05.S02 — the bulk state bindings *(done 2026-08-16, v1.105.1)*
Scope: `pdfDocument`, `docMeta`, `overlayFields` onto the view (~220 references). Refs:
D5, D12, and the hot-path pin.
**(note, 2026-08-16)** Two P04 guards went red on this slice's idiom change — they name
`docMeta` and it became `view.docMeta`. They were **re-derived** to the new idiom, not
loosened until they passed: the same distinction P03.S02 had to make when the registry
changed the resolution idiom out from under its guard, and worth repeating because a
regex widened to stop failing is a guard that has stopped guarding.

Acceptance:
- `relayoutOverlays` walks **only the active view's** fields — asserted, because a
  version iterating every open document turns the one genuinely frequent path in this
  feature into an N× regression on the path a user feels most.
- Single-view behaviour unchanged.

#### P05.S03 — per-view viewer and DOM *(done 2026-08-16, v1.105.2)*
Scope: `viewer`/`eventBus`/`linkService`/`findController` per view; per-view
container/pages pair inside the stable `#viewerWrap`; the **26** pointer
listeners re-homed to the stable parent; cleanup sweeps view-scoped. Refs: D3, ADR-002.

**(deepdive pin, 2026-08-16 — the dive ran before the grill and hardened the premise
rather than the plan.)** The four bindings being per-view was written as a design
preference. It is **forced by the vendored pdf.js, four independent ways**, each read at
the line:
- `PDFViewer`'s constructor registers `thumbnailrendered` on the bus it is handed
  (`pdf_viewer.mjs:8065`). N viewers on one bus each index `_pages[pageNumber-1]` from
  another document's event and call `pdfPage.cleanup()`.
- The same constructor **mutates** the find controller it is handed —
  `findController.onIsPageVisible = …` (`:8012`) — a single slot, last writer wins.
- `PDFFindController`'s constructor registers `find`/`findbarclose`/`pagesedited` on the
  bus (`:927-929`), so one `dispatch('find')` makes every open document search at once
  and race to answer the single counter.
- `PDFLinkService` holds one `pdfViewer` field and `setViewer` is 1:1 (`:1582,:1589`),
  so outline clicks drive the last-constructed viewer.

**Re-homing the 26 listeners is safe by construction, and that is a fact about one
function.** All ten `pointerdown` handlers resolve the page through `pageAt`
(`app.js:4649`), which is pure viewport geometry against `getPageView(i)` rects. There is
no `e.currentTarget` and no `elementFromPoint` anywhere in `app.js`, so moving a listener
up one ancestor changes no coordinate and no target. The **16** `pointermove`/`pointerup`
handlers are each gated on their own drag-state variable, so they need no origin guard —
and leaving them unguarded fixes a latent orphan-`div` bug (see the behaviour-change pin).

**(grill pin, 2026-08-16 — the origin guard is real, and it belongs on ten handlers, not
26.)** `#signBanner` is today a *sibling* of `#viewerContainer` (`index.html:341`), so a
pointerdown on it does not reach the tools. On the wrap it would. Seven tools —
splitBox, crop, border, dropdown, radio, shape, note — are **not** in `EDITING_TOOLS`
(`app.js:5205-5209`, verified), so they stay armable while the banner is up, and a click
on the banner would land a note or start a drag on the page beneath it. The guard
reproduces today's semantics. It goes on the ten `pointerdown` handlers **after** each
one's existing mode-flag bail: putting a `closest()` tree-walk first would add eight
ancestor walks per `pointermove` across the text layer, which `CLAUDE.md`'s hot-path rule
governs. `#empty` needs no guard — it is `pointer-events:none` (`style.css:772`) and
hidden whenever a document is open (`:774`).

**(grill pin, 2026-08-16 — build the container visible, then hide.)** pdf.js's
`container must be absolutely positioned` check (`pdf_viewer.mjs:8000`) is guarded by
`this.container.offsetParent`, which is **null when the element is `display:none`**. So
constructing a viewer into a hidden container **skips** the check rather than satisfying
it, and it never re-runs — a CSS mistake would then throw only for the first visible view
and stay silent for every hidden one. `newView()` therefore appends and constructs while
visible, and hides afterwards only if the view is not active. This also disposes of the
"created hidden leaves the app blank" defect: one view exists and it is always active.

**(reality drift, 2026-08-16 — removing the `#viewer` id would have blinded tier 3.)**
`test/ui/harness.mjs:86` waits on `#viewer .page` **inside `openDocument()`**, the helper
every tier-3 test calls; `:118` and `:135` use it too. Since tier 2 declares no layout and
no rendering engine (`boot.mjs:15-21`), cursor behaviour and pointer routing are
*delegated* to tier 3 — so dropping the id would have deleted the one instrument able to
see this slice's own regression, and it would have failed as a **timeout**, not an
assertion. The three selectors are re-derived to the new idiom (T11), not loosened.

**(behaviour change, recorded deliberately, 2026-08-16)** A drag released outside the
container today never reaches the container's `pointerup`, so its drag-state variable
stays non-null and its preview div is orphaned. Re-homing to the wrap terminates the drag
correctly. This is a fix, not a regression, and it is written down rather than smuggled in
because "reproduce today's semantics" would otherwise imply today's semantics were right.

**(note, 2026-08-16 — a latent shared global, left alone on purpose.)** Every `PDFViewer`
writes `--viewer-container-height` onto the document root (`pdf_viewer.mjs:552`,
`:9590-9595`), so with N viewers a hidden view's `0px` clobbers the active view's value.
Inert in Nib: the only consumers are the presentation-mode `dummyPage`
(`pdf_viewer.mjs:8778-8781`) and a pdf.js sidebar Nib does not render, and `_resetView`
pins `ScrollMode.VERTICAL` (`:8733`). One comment, no fix — it goes live the day spread
or presentation mode does.

**(note, 2026-08-16 — ADR-002 is not edited.)** ADR-002 spells the decision as a
`#viewerContainer`/`#viewer` pair. This slice makes those classes, because two elements
cannot share an id. The **decision content is untouched** — each document still owns its
own container and pages pair, nested in the stable wrap, hidden not destroyed — so the
ids read as illustrative of the shape, not as the decision. Per `CLAUDE.md`, ADRs are
superseded and never edited; nothing here rises to a supersession.

Tasks:
1. T01 — `newView()` builds its own DOM (`.viewerContainer` > `.pdfViewer.viewerPages`),
   appends to `#viewerWrap`, and constructs the four pdf.js objects onto the record,
   passing `viewer:` explicitly so the inner div needs no id (`pdf_viewer.mjs:7995`).
2. T02 — remove the static markup and the `els.viewerContainer` handle; CSS
   `#viewerContainer` → `.viewerContainer`, `#viewer` → `.viewerPages`.
3. T03 — the nine module-scope `eventBus.on` registrations move into `newView()`.
4. T04 — handlers writing **shared** chrome (`pagechanging`; both find-count events)
   gain an active-view guard.
5. T05 — `relayoutOverlays`/`relayoutRedactMarks`/`fitWidestWidth`/`pagesinit` act on the
   **owning** view; the three direct callers (`:5520`, `:4857`, `:6999`) pass it.
6. T06 — the bare `viewer.` reads, three `linkService` sites and two `eventBus.dispatch`
   sites resolve through the view.
7. T07 — re-home the 26 listeners to `els.viewerWrap`; ten `pointerdown` guards, placed
   after the mode-flag bail.
8. T08 — the 23 cursor writes move to the wrap.
9. T09 — `all('.splitmark')` (`:4875`) and `all('.cropmark')` (`:4959`) re-root at the
   owning view's container.
10. T10 — tier 2: `lifecycle.test.mjs:47` reads the cursor off the wrap.
11. T11 — tier 3: `harness.mjs:86,118,135` re-scoped off the `#viewer` id.
12. T12 — `view.test.mjs` guards re-derived (the `relayoutOverlays` anchor at `:104`, the
    `overlayFields` shape at `:108`), the four bindings added to the per-view scan, and
    anchor-staleness assertions added.
13. T13 — the decoy fixture, asserting **both** directions.
14. T14 — this amendment, the D5 supersession pin, and the `overlayHistory` correction.

Acceptance:
- Each view owns its viewer and container; inactive views hidden, never destroyed.
  **Ownership** is asserted at the source and by construction; **inactive** is
  `not exercised` — no production path creates a second view until P06.
- The pointer listeners survive a view being hidden — they are on the stable parent.
  Bound-to-the-wrap is asserted and probed red; surviving an actual hide is
  `not exercised` (jsdom has no layout, tier 3 has one view).
- A document-wide DOM sweep cannot reach a hidden view's marks (ADR-002's consequence).
  **Driven for real** at tier 2 by injecting a decoy container: the test asserts the
  active view's own mark **is** removed *and* the decoy's survives. One-directional would
  pass against a sweep that removes nothing, which is the vacuous green this plan keeps
  meeting.
- Single-view behaviour unchanged; the existing suites stay green.
- **The redaction tripwire applies** (`app.js:4636-4640`): this slice changes how `pageAt`
  resolves its viewer, which feeds `pageContentRect`, so the manual procedure in
  `reference_redaction_visual_check.md` runs before the commit. **Run 2026-08-16 against a
  known-secret fixture: 24/24 interior pixels black, 3/3 just-outside controls pure white —
  matching the 2026-06-02 v0.9.19 baseline exactly.** The procedure needed no adaptation,
  which is itself the result: it drives bubbling PointerEvents from a text-layer span, so it
  exercised the re-homed listeners on their new root without knowing they had moved.

**(live verification, 2026-08-16 — the origin guard, with a control arm.)** The one behaviour
this slice *changes* is that `#signBanner` now sits under the pointer root. Driven in a real
browser: the same armed note tool placed 2 notes on the page and placed **nothing** on the
banner. The control arm is the point — the first two attempts at this probe both reported
"refused" while proving nothing, once because the tool had never armed (its button lives in
another toolbar tab) and once because the control placed no note either (`makeNote` is async
and the read was synchronous). *A refusal is only evidence when the same stimulus is shown to
succeed somewhere.*

**(review, 2026-08-16 — the critical was in this slice's instrument, not its code.)** The
first decoy fixture built `.viewerContainer > .splitmark` with no page stack and parked the
live mark on `.viewerPages` rather than in a `.page` — neither matching what `newView()`
builds nor where the app appends a mark. Measured: a `document.querySelectorAll('.viewerPages
.splitmark')` sweep — document-wide, reaching every open view — passed **both** assertions with
the suite at 42/42. It cleared the live mark, and missed the decoy's only because the decoy
lacked the element the selector keyed on. **A decoy that differs from the real thing tests the
difference, not the rule**, and this is the third time this phase has met a green over a
population that was never exercised. Rebuilt to the real shape and re-probed red three ways.
Full disposition: `<project-memory>/code-reviews/P05.S03-per-view-viewer.md`.

#### P05.S04 — activation *(done 2026-08-16, v1.105.3)*
Scope: the remaining per-document module state onto the view record; `activateView(v)`
(quiesce, swap, geometry, repaint); per-view dpr tracking and scroll restore; arrivals
open into a new hidden view; Close drops all views. Refs: D3, D10, ADR-002.

**(re-scoped 2026-08-16 by the S04 deepdive, Dan's call — option A.)** The slice was
scoped as "show/hide on switch; re-fit and dpr-heal on activation". Three things the dive
established changed that:

**(1) The slice cannot verify itself, at any tier.** Its one clause needs a view that loads
while hidden, and nothing can produce one: every open path calls `setDoc`, which does
`s.docs = nil` (opens **replace**, server-side); jsdom has no layout, so `clientWidth` is
always 0 and the clause is invisible there *by declared ceiling*; and the one adding path —
a co-sign arrival calling `addDoc` — needs a live pinned peer, which `arrival.test.mjs`
already records as undriveable from jsdom and carried to P06. **The clause is
`not exercised` and carries to P06**, on Dan's explicit call at the S04 gate. The
*mechanism* is verified as necessary: a hidden container makes pdf.js compute
`(clientWidth - 40) / width` → negative, reaching `#setScaleUpdatePages` unclamped
(`pdf_viewer.mjs:8933`), and nothing repairs it on show — `#resizeObserverCallback`
updates one CSS var and nulls a cached offset, no re-scale and no re-render (`:9596`).

**(2) The phase's binding count was short by roughly 3x, not by one.** S03 found a
fourteenth. Asking the different question — *what must swap when the active view changes?*
— finds about **35**: the 13 already moved, `overlayHistory` (S06's), ~22 per-document
bindings still at module scope, ~11 transient ones that must be **aborted rather than
restored**, and ~20 modal-state bindings that collapse to one rule (close them on switch).
The worst of the new ones is **`selectedPages`/`selAnchor`** (`app.js:2822-2823`), which no
enumeration in this plan has ever named: 1-based page numbers in one document's pagination
driving the bulk rotate/delete/reorder bar, so with two views it applies the *old*
document's page numbers to the *new* document. That is a destructive wrong-document
operation, not a stale label.

**(3) Two live defects and two live `ReferenceError`s sit in the code this slice touches,
and fixing them is most of what makes the slice worth shipping.** They are listed as tasks
below rather than filed, because three of the four are in the exact call paths activation
has to get right, and the fourth is two lines.

**(live defect — an arrival destroys unsaved work.)** `reloadOpenDoc` (`app.js:1082`)
states the correct premise in its own comment — *"A co-signature applied out of band ADDS a
document and makes it active (D10); the document this client currently names is still open
and still perfectly valid"* — and then calls `setDocumentFromServer`, which repoints the
single view at the arrival and runs `clearOverlays()` on the way. P03.S05 built the server
half; the client half was never built, so every overlay element and typed value on the
document the user was working on is destroyed, and that document survives only on the
server, unreachable from the UI. **This is also S04's stimulus**: it is what makes a second
view exist.

**(live defect — Close closes everything.)** `handleClose` calls `setDoc(nil)`, which empties
the whole registry and clears every open document's undo rings. Arrivals can already leave
two documents open, so closing one drops both. S04 makes the *client* match that
(a Close drops all views), which is honest rather than a regression — Close is "Close all"
today. **P06 splits Close view from Close all**, and that is where the server needs a
per-document remove.

**(live defect — two Save buttons throw.)** `app.js:4225` ("Save reduced PDF") and `:4626`
("Save complete proof") read `exportName`, declared inside a *different* handler (`:4191`
and `:4605`) with no module-level binding anywhere. Sibling arrow functions at module
scope. P04 residue: these are the two export scopes split across a second handler, so the
capture-at-operation-entry rewrite had no local entry to capture at. `node --check` passes
because it is a scope error and not a syntax error, which is exactly why tier 0 never saw
it — and why the fix ships with a test that drives each flow far enough to have caught it.

**(ordering constraints the dive pinned, all load-bearing.)** `view = v` before every
repaint, because eleven repaint functions resolve the module-level `view` internally.
`updateBadge` **after** the swap, because its first statement is a *write* —
`view.lastSig = sig` — so called before it, it overwrites the outgoing view's `lastSig`,
one of the three SAFETY fields. `docHadFlags` restored before `applySignLock`, which reads
it as `recipient`. `applySignLock` **after** `setDocControls(true)`, because `EDITING_TOOLS`
is a strict subset of `DOC_REQUIRED` and the latter re-enables everything the former just
disabled. Unhide before the re-fit, or `clientWidth` is 0 and the fit silently no-ops.
Abort in-flight drags **before** hiding, because the preview nodes live in the outgoing
view's page divs and a release after the swap writes the drag into the wrong document.

**(and what must never run on activation.)** `setDocument` — it calls `_resetView()`, which
does `viewer.textContent = ""`, emptying the page stack and every overlay value with it,
which is the whole of ADR-002. Also barred: `setDocumentFromServer`, `resetSharedDocState`,
`clearOverlays`, `clearOverlayHistory`, `reconstructFlags` (it *pushes* with no clear of its
own, so re-running duplicates every flag), `showSignBanner` (it resets a recipient's
mid-flow progress — `setSignBanner` is the repaint), `updateBadge(null)`, `closeDocument`,
`loadingTask.destroy()`, and `newView()`.

**(scroll is not preserved by hiding, and ADR-002 does not cover it.)** `display:none`
drops `scrollTop`, so on show the scroll watcher runs with 0 and the view returns to page 1.
The ADR's "preservation is the browser's default" holds for DOM *content*; scroll offset is
Nib's to restore. Same for dpr: `lastDpr` is one module global and `dprChanged` refreshes
only the active viewer, so a dpr change while a view was hidden is recorded and never
delivered — that view's canvases stay soft permanently.

Tasks:
1. T01 — the ~22 remaining per-document module bindings move onto the view record,
   including `selectedPages`/`selAnchor`.
2. T02 — `activateView(v)` phase A: abort every live drag, capture scroll and page number,
   close the document-bound modals.
3. T03 — phase B: the swap, and the restore of the per-document state, in the pinned order.
4. T04 — phase C: unhide, re-fit, then relayout overlays and marks **explicitly** — a
   re-fit that computes the same scale fires no event, so the self-serving path is not
   enough.
5. T05 — per-view dpr tracking (`v.renderedDpr`) and the refresh on activation.
6. T06 — scroll and page restore.
7. T07 — phase D: repaint the shared chrome — wrap classes, Save label, page counters,
   badge, both sidebars, find counter, doc-required controls, sign lock, flags panel,
   banner, the nine tool reflectors and the wrap cursor.
8. T08 — arrivals open into a new hidden view (the client half of D10).
9. T09 — Close drops all views, matching the server.
10. T10 — the two `ReferenceError`s, each with a test that drives its flow.
11. T11 — this amendment.

Acceptance:
- A view that loads while hidden gets a scale when activated — red without the re-fit,
  since a hidden container reports `clientWidth` 0. **`not exercised` — carried to P06**
  (Dan's call, 2026-08-16): no path creates a second view until the tab strip, jsdom has no
  layout, and the arrival that could is undriveable below P06.
- An arrival opens a **new** view and leaves the current one intact — its overlay fields
  and redaction marks survive, asserted directly rather than through the DOM.
- The per-document bindings are on the view record, `selectedPages` among them; a
  module-scope scan for them is red before the move.
- The transient drag state is **aborted** on switch, never restored — asserted by name,
  because restoring it is the plausible wrong shape.
- Both Save buttons produce a download rather than throwing, each driven through its own
  flow.
- Single-view behaviour unchanged; all four tiers green.

**(diff review, 2026-08-16 — three criticals, and all three were in machinery this slice
added to prevent their own class.)**

1. **The scripted move of 193 references missed two, and the guard was blind to exactly the
   form it missed.** `[...selectedPages]` in `selectedPagesParam` and `moveSelected`: the
   character before the identifier is the third dot of the spread, so the guard's
   property-access lookbehind rejected both. They were the only two spread reads among the
   193 sites, and both were live `ReferenceError`s that killed **every bulk page operation**
   with one document open and no arrival needed. The lookbehind now rejects a preceding dot
   only when it is not part of `...`, and both the defect and the fix were driven live:
   rotate sent `pages=1,2`, ⤒ sent `reorder 3,1,2,4`, and `eval('[...selectedPages]')` in the
   same page returns `ReferenceError`.
2. **The arrival still destroyed the active view's work.** `setDocumentFromServer` guarded
   every shared-chrome write on `target === view` and then called `resetSharedDocState()`
   unguarded — a function entirely module-`view` bound, which empties `overlayFields`, nulls
   the marker bindings, clears `redactMarks` and drops the overlay undo stack. It was
   *reported* fixed before the review caught it, and the cause is the useful part: **a
   scripted edit asserted on six substitutions, aborted on the fourth, and wrote none of
   them** — a composed fix that was never applied, under a narrative that had moved on. A
   scripted edit that asserts before writing must be checked for having written.
3. **The rename broke the arrival path.** `reloadOpenDoc` became `openArrivalInNewView` and
   its one call site did not. `node --check` passed and all 44 tests passed, because nothing
   drives that path — the exact ceiling `arrival.test.mjs` declares, met within an hour of
   reading it. Now guarded by an unresolved-call scan, red against that defect.

Also fixed: `views.splice(indexOf, 1)` with no `-1` check (a Close racing the load removed the
**active** view); the sidebar staleness token, which compared generations across views that
both start at 0 and so could not identify a document (ADR-001's id-reuse shape again, now a
`(view, generation)` pair); `dprChanged` not recording what it refreshed, which left a view
permanently soft; the "selection restore" that called `clearSelection()` on the *incoming*
view and so cleared rather than restored; the decrypt prompt writing the wrong document; and
four operations resolving the view after an await (`splitRects`, `bakedBytes`' flag-strip
predicate, and the redact and remove-originals disarms).

**(correction, 2026-08-16 — a comment that justified itself with claims that were untrue.)**
The record's note said `sbPage`, `splitRects`, `cropPage`, `cropRect` and `selectedPages` were
all read after an await. Only `splitRects` was, and it is the one that went unfixed while the
comment asserted the class had been considered. Corrected in both the code and the guard.

**(deferred, enumerated rather than described.)** The rest of the resolve-at-fire-time family
— `fillTarget` across the Library pick, `placeIntoMarker` after the image decode, the two
pointer-capture gesture closures `abortDrags` cannot reach, `pageOp` plus 19 unpinned
`setDocumentFromServer` call sites, the shape/note handlers already inside their await — is
listed site-by-site in the pending item. A new item covers Open orphaning every other view and
Close-all never asking about their unsaved work; both are P06-shaped.

#### P05.S05 — the sidebars *(done 2026-08-16, v1.105.4)*
Scope: per-view thumbnail and outline containers; the drag subsystem re-homed and
capture-pinned. Refs: D3, ADR-002, and the subsystem-round pin.

**(scope correction, 2026-08-16 — half of this slice shipped three slices ago.)** The scope
line says "per-view thumbnail and outline containers, **per-view `docGen`**". `docGen` became
per-view in **S01** (`app.js:1308`, guarded at `view.test.mjs:42`). Only the containers are
left. Recorded rather than silently skipped, because a scope line that names work already done
reads later as work that was dropped.

**(corrects S04, and the phase's own decision is the reason.)** The phase-open decision chose
per-view containers over rebuild-on-activation in those words — *"rebuild-on-activation is
precisely what ADR-002 exists to avoid"* — and S04's `repaintForActiveView` rebuilds both
sidebars on every switch. That was the honest interim while the containers were shared: there
was nowhere else to render. This slice deletes the rebuild, and with it the `selectedPages`
snapshot-and-restore that exists **only** because `buildThumbnails` calls `clearSelection`.

It also retires the `owner !== view` half of three staleness guards S04 added
(`buildThumbnails` twice, `buildOutline` once). Those exist solely because the grid is shared —
the comment says so — and once it is not, they abort **exactly the background build this
slice's second acceptance clause requires to finish**. `gen !== owner.docGen` alone is exact,
because `owner` is captured and `docGen` is per-view.

**(deepdive pin — the drag subsystem is the hazard, and no existing guard can see it.)** Four
listeners are bound once at module scope to `els.thumbGrid` (`app.js:3240-3243`). Per-view
grids leave every later view's thumbnails `draggable` (set at `:3082`) and completely inert —
the same shape as S03's pointer listeners, except that S03's guard **cannot catch it**: its
regexes key on `addEventListener('pointer` and these are `dragstart`/`dragover`/`drop`/
`dragend`. The slice owes the counterpart guard.

**The worst single line is `onThumbDragEnd`'s restore** (`app.js:3234`). It re-appends every
snapshotted node into a grid. Written as a resolve-at-call-time `view.thumbGrid`, a cancelled
drag whose view is no longer active **physically relocates one document's `.thumbwrap` nodes
into another's grid** — after which the next drop reads `dataset.page` off those nodes
(`:3226`) and fires `pageOp('reorder')` against the wrong document. **No `docGen` comparison
catches it, because the drag never touches `docGen`**: this is ADR-001's law applied to a DOM
node rather than a document id. It is correct today only by accident of ordering —
`activateView` calls `abortDrags()` before the swap. Capturing the grid at `dragstart` makes it
immune to ordering rather than dependent on it.

**(measurement correction, 2026-08-16 — two comments assert a number that is false.)**
`lifecycle.test.mjs:124-127` and `stub-pdfjs.mjs:52-55` both justify not asserting the
thumbnail grid at tier 2 by saying jsdom's missing canvas leaves it empty. **Measured through
the real harness: the grid holds exactly 1 `.thumbwrap`, not 0** — `buildThumbnails` appends
the wrapper *before* the render that rejects, so one lands every time. The conclusion (do not
assert it there) still holds; the reason does not, and a green written against "always 0" would
have been an accident rather than a result. Also measured: `#outline` has **2** children with
no outline present (the Edit button and the empty-state div), so `lifecycle.test.mjs:107`'s
`> 0` passes on chrome alone.

**(memory, stated rather than inherited.)** ADR-002 accepts hidden DOM and says it is *"bounded
by the open-document cap"*. **That cap does not exist** — not in count, not in bytes, in Go or
JS; D9 defers it to P06. A per-view grid costs roughly **35 MiB of canvas and 2,100 DOM nodes
per hidden 300-page document**, on top of the page DOM. Built eagerly as decided, because
arrivals are the only multi-view path so the exposure is one document at a time — but the bound
the ADR names is planned, not real, and belongs in P06 with the cap rather than being mitigated
here with a lazy-build state machine.

Tasks:
1. T01 — `newView()` builds a `.thumbgrid` into the stable `#thumbs` and an `.outlinelist`
   into the stable `#outline`, hidden unless first.
2. T02 — delete the static `#thumbGrid`; `#outline` stays a unique panel id, because the tab
   machinery resolves panels by `getElementById` and `#outline a` matches through the wrapper.
3. T03 — both builders write to the owner's container.
4. T04 — `markCurrentThumb`, `markSelectedThumbs`, `clearSelection` take the owner.
5. T05 — drop the `owner !== view` half of the three staleness guards, and rewrite the comments
   that justify them by "the shared grid".
6. T06 — `markSelectedThumbs` writes the shared `#thumbSelBar` only when its owner is active.
7. T07 — split the `pagechanging` handler: the thumbnail highlight is this view's own DOM and
   runs ungated; the page-number inputs stay gated. Correct the two-category comment.
8. T08 — `activateView` toggles the containers' `hidden`; the rebuild block and the
   snapshot-restore dance are deleted.
9. T09 — remove the `target === view` gate on the load path's builds.
10. T10 — re-home the four drag listeners to `#thumbs`, with an origin guard.
11. T11 — capture `dragGrid` and `dragView` at `dragstart`; the drop refuses on a changed view.
12. T12 — close-all removes every view's containers; the arrival rollback removes its own.
13. T13 — the tests: three `getElementById('thumbGrid')` sites, `counts().thumbs`, and the
    post-close emptiness assertions, which now mean "no view has any".
14. T14 — the drag-listener guard, a decoy-grid test, and the two corrected comments.

Acceptance:
- The two sidebars show the active document's content and nothing else.
- A background document finishing its build cannot abort the foreground's — **and its own
  build finishes**, which is the corollary the shared grid made impossible.
- A cancelled drag cannot move one view's thumbnails into another's grid, asserted directly.
- The four drag listeners are on the stable parent, asserted by name — the pointer guard
  cannot see them.
- Single-view behaviour unchanged; all four tiers green.

**(diff review, 2026-08-16 — the lead finding was a regression this slice introduced, and its
own task list had named the fix.)** T10 reads "re-home the four drag listeners to `#thumbs`,
**with an origin guard**". The guard went on `dragstart` only. Because the listeners now sit on
`#thumbs`, whose subtree holds the append row and the selection bar **above** the grid,
releasing a dragged page onto that bar changed silently from *cancel* to *commit a
whole-document reorder* — before the re-homing, a release outside the grid produced no drop
event at all. Fixed with `overDragGrid` on both remaining handlers, and driven live in both
directions: a drag inside the grid still reorders (`4,1,2,3,5`), a release on the bar sends
nothing.

**That is the third time in this phase a specified multi-site fix landed incompletely while the
narrative moved on** — a scripted edit that aborted before writing (S04), a rename that missed
its call site (S04), and now a guard applied to one handler of three. The shape is not
carelessness about the fix; each was *reported* complete on the strength of having been
composed.

Also fixed: `dragDropped` was set **before** the wrong-view refusal, so the refusal disabled the
revert it depends on and would have left the grid permuted against a server that never
reordered; `newView()` now removes what it appended if the `PDFViewer` constructor throws (an
orphan `.viewerContainer` is transparent, `inset:0` and never hidden — it sits over the active
document swallowing its pointer events); and `edit.onclick` on the outline panel was the last
handler in the file resolving the active view at click time, sitting directly under the
paragraph that states the rule.

**(and the decoy test went in one-directional.)** The new per-view-grid test asserted only that
a decoy grid survived, never that the active grid was cleared — so it would have passed against
a build that never ran. It was written into the file whose header explains that exact vacuity,
four slices after the same defect was found in the same file. Fixed with a sentinel and proven
red. *Knowing the rule is not the same as applying it to the instrument in hand.*

**(version, 2026-08-16 — corrected from the gate.)** Presented there as a minor bump because
per-view sidebars are user-facing. They are not: with one document open nothing visible changes,
and the multi-document behaviour they enable is unreachable until P06. Patch, matching S03/S04.

#### P05.S06 — the fourteenth binding: `overlayHistory` *(done 2026-08-16, v1.105.5)*
**(added 2026-08-16 by S03's deepdive — Dan's call, option A.)** Scope: `overlayHistory`
(`app.js:6446`) onto the view record. Refs: D5, D12, and the measurement correction in
the phase-open notes.

Sliced separately and placed **last** rather than folded into S01, because S01 is closed
and because this binding is in S01's category, not S02's: it is a *safety* binding, and
retrofitting it into a closed slice would leave the phase with no record that the
enumeration which sized the work had missed it. The residue is the point.

The failure it removes: the undo/redo entries are **closures over overlay elements**
(`:6527-6533`), and `undoAny()` (`:6538`) drains this stack **before** the server ring.
So with one shared stack, Ctrl+Z in document A pops a command captured against document
B and calls `c.undo()` on B's element — a mutation of a document the user is not looking
at, from a keystroke they aimed at the one they are. That is the `redactMarks` family
(irreversible, silent), not the stale-label family.

**(scope addition, 2026-08-16, found by S03's diff review — the phase criterion names two
functions and this is where they belong.)** The exit criterion says
`detachField`/`reattachField` must resolve the **owning** view, not the active one. Both
(`app.js:6620-6627`, plus `layoutFieldNow` at `:6629`) still read the module-level `view`
after S03 — deliberately, because **every** stored-closure use of them is inside a
`recordOverlayEdit(...)` call (`:5441-5459`, `:6637-6646`), which is to say inside this
slice's binding. Fixing them without per-view history is the half of the fix that does not
remove the failure: a *direct* call is always correct, since a user can only interact with
the visible view, and it is only the stored closure that can outlive a switch. So the two
land together, here.

Acceptance:
- `overlayHistory` is per-view, and the undo/redo button state (`:6476-6477`) reflects
  the **active** view's stack.
- `detachField`, `reattachField` and `layoutFieldNow` act on the view that owns the field,
  not whichever is active when an undo is drained — discharging the phase criterion that
  names the first two.
- A guard asserts an overlay-edit undo recorded on one view cannot be drained through
  another — red without the fix.
- `hasEditsSinceOpen` (`:1480`) reads the owning view's stack, not whichever is active.
- The phase-open enumeration says **fourteen** bindings, and the exit criterion names
  this one.

**(deepdive trigger, 2026-08-16 — evaluated, not fired, and recorded rather than skipped.)**
The surface is 20 sites in one file with no wire, disk or schema reach, and this seam has
already been dived twice: S03's diff review found the stored closures, S04's deepdive
enumerated the three helpers, and both findings are written above. A third pass would
re-derive what is recorded. The recorded claims were re-verified at the line instead.

**(found while verifying, and not previously recorded.)** `clearOverlayHistory()` sits
**below** S04's `if (owner !== view) return;` inside `clearOverlays`. That guard is correct
for the draw-mode exits, which repaint shared toolbar buttons — but the undo stack is per-view
DATA, not shared chrome, so once it moves onto the record a background reload would keep a
stale stack whose closures point at elements that have been removed. It moves above the
return, beside `overlayFields` and `redactMarks`.

Tasks:
1. T01 — `overlayHistory` onto `newView()`.
2. T02 — `recordOverlayEdit(cmd, owner)` pushes to the owner's stack; the five recording
   sites pass theirs.
3. T03 — `detachField`, `reattachField`, `layoutFieldNow` take the owning view, and the
   stored closures capture it.
4. T04 — `removeField` takes its owner: it reads `activeMarker`/`fillTarget` and is called
   from a keydown handler bound to one view's element.
5. T05 — `undoAny`/`redoAny`/`reflectUndoControls`/`hasEditsSinceOpen` read the ACTIVE view's
   stack, which is correct: the keystroke aims at what the user can see.
6. T06 — `clearOverlayHistory(owner)` moves above the early return in `clearOverlays`.
7. T07 — the guards: the binding joins the per-view scan, plus a behavioural test that a
   command recorded on one view cannot be drained through another.
8. T08 — the phase enumeration says fourteen; this amendment.

**(diff review, 2026-08-16 — a deferral that rested on a claim I had repeated and that is
false.)** The review found the deferral of `placeIntoMarker` resting on "no second view is
creatable below P06". **That is wrong**: `pollRecv` calls `openArrivalInNewView`
(`app.js:974`), live since S04, so a second view exists in production today for anyone doing a
live co-sign. What P06 adds is a way to create one *without a pinned peer* — which is what a
TEST needs, and the two had been conflated in several places in this plan.

The distinction inverts the framing: the cross-view hazards this phase closes are reachable
now, not latent until P06. It also made the deferral wrong on the merits — the comment's list
of what needed threading omitted `makeStamp`, which is called inside the awaited `Image.onload`
and pushes to `view.overlayFields` while appending into a `pv` captured from the *other* view.
An arrival landing mid-image-load would register the stamp in the arrival's fields with its
element in this document's page div, and the arrival's bake would burn it on at this document's
coordinates. **Fixed here** — `placeIntoMarker` captures at entry and threads through
`makeStamp` and `removeField`; the shared-chrome repaint is gated rather than threaded.

Also fixed: the owner scan accepted `, owner)` anywhere within its window, so a command
containing `removeField(f, false, owner)` would have counted as owned with no owner on the
outer call — it now requires `}, owner)`, proven red against exactly that shape. And three
drifted comments, including a SAFETY count that said "three" when there are five and was
already wrong at four before this diff.

**(a test deleted rather than kept, 2026-08-16.)** The first behavioural guard written for this
slice asserted the undo button starts disabled — and passed identically against a shared stack,
so it tested the per-view property not at all. No overlay can be placed at tier 2 (every
`getBoundingClientRect` is 0, so `pageAt` never resolves a page), which means the stack cannot
be made non-empty there and the clause is genuinely unexercisable at any tier. The absence is
documented in `perview.test.mjs` with the reason, rather than papered over with a green that
would have read as coverage for the clause it was written under.

**(decision, 2026-08-16 — the closures capture their owner even though the stack makes it
redundant.)** With a per-view stack drained only when its view is active, the owner IS the
active view at drain time, so reading the module `view` inside a stored closure would be
correct today. The capture goes in anyway: it makes the property hold by construction rather
than by depending on that invariant. Same argument as S05's `dragGrid`, and S05 is where
relying on an ordering accident nearly cost a destructive reorder.

**(dimension-review pin: the one hot path this feature has, 2026-08-15)** Switching
is a `display` toggle and opening is a user action, so neither is per-frame. But
`relayoutOverlays` runs on scroll and zoom, and it resolves `overlayFields` at call
time (D4). Per-view records must leave it walking **only the active view's**
fields; a version that iterates every open document's overlays turns the one
genuinely frequent path in this feature into an N× regression on the path a user
feels most. Nib's `CLAUDE.md` hot-path rule governs: check in before widening it.

**(dimension-review pin: the three bindings where "shared" is a safety defect, not
a UI defect, 2026-08-15)** Of the seven silent-loss bindings, three fail in a
different category and the plan should not file them beside the others:
- **`redactMarks`** — marks drawn on A, baked onto B. Redaction commits through
  `commitBarrier`, which **clears the undo history by design** (`undo.go:46-57`),
  so the wrong-document outcome is irreversible destruction of content with no
  path back. This is the worst single outcome anywhere in this plan.
- **`signLocked`** (`:1252`, applied at `:1255`) — a received signing document
  opens locked and non-editable, which is a **guarantee made to a counterparty**.
  Shared, it either unlocks A (breaks the guarantee) or locks B (harmless
  friction). Where the binding is ever ambiguous the resolution must fail toward
  *locked*; a guard test asserts a locked document is still locked after a switch.
- **`lastSig`** — the signature-details modal is where the user makes a **trust
  decision**. Showing A's verification result under B's name is not a stale-label
  bug; it is misreporting a cryptographic fact. Per-view, and asserted.

**(subsystem-round pin: the sidebars are shared, 2026-08-15)** Verified: `els.outline`
is one element, cleared with `innerHTML = ''` at `app.js:2890`, and both
`buildThumbnails` (`:2366`) and `buildOutline` (`:2889`) default their staleness
token to the single module `docGen`. So they are document-scoped *content* in a
document-agnostic *container*, gated by a counter that is about to become
per-view. Either the containers become per-view like the viewers (consistent with
D3, costs hidden thumbnail DOM) or they rebuild on activation (cheaper, but
re-renders every thumbnail on every switch — the thing D3 exists to avoid).
Decide at phase-open, with `docGen` per-view either way — a shared token would let
a background document's finishing build abort the foreground's.

### P06 — Tabs, Close view, Close all
Goal: the user-facing feature. Refs: D1, D2, D9.
Exit criteria:
- A document switcher; **Close view** and **Close all**.
- Switching preserves scroll, zoom, page, form fills and typed overlay values —
  asserted by reading a typed value back out of its DOM element.
- The document cap refuses with a message.
- README and the in-app help describe the multi-document model; `CLAUDE.md` gains
  the operation-pinning law (D7) so the next change cannot unknowingly break it.
- Full-repo `/code-review` clean.
- **(carried from P05, 2026-08-17 — P05 closed over these two by Dan's call, and this
  is where they land.)** Re-fit **and** dpr-heal on activation: a view that loads while
  hidden gets no scale, because a hidden container reports `clientWidth` 0. Both are
  built (`app.js:1619`, `:1625-1628`) and neither has ever been driven. **This is P06's
  first acceptance work, and the reason is an ordering constraint rather than a
  preference:** the clause only becomes drivable once this phase can create a second
  view without a live pinned peer, so whatever slice adds that owes the test in the same
  change — otherwise the clause carries a second time, which is how a `not exercised`
  becomes permanent.
- **(carried from P03, plan-review pin)** The all-tabs-stale case resolves to the launch
  empty state, not N error tabs. Unfalsifiable until there are tabs; P03 shipped the 409
  that makes it expressible.
Slices: sketched at phase-open.

**(dimension-review pin: what a reload does, 2026-08-15)** The server holds the
documents; the tab strip is client state. So a browser reload — or a tab crash —
with eight documents open currently has no defined behaviour under this design,
and the accidental one is "the app comes back showing a single document while the
server still holds eight." Reload restores the full tab set from the server,
which is also why `/api/docs` returns the list rather than just a count. Named
here because it is the failure playbook this feature lacks, not a nicety.

### P07 — Single instance
Goal: opening a PDF from the OS reaches the running nib as a new tab instead of
launching a second application. Refs: D18.
Exit criteria:
- A second launch hands its path to the running instance and exits; the running
  window raises with the file as a new tab.
- A stale lock left by a killed process recovers without user action.
- Verified on Windows through `build/winrepro.sh`, where `nib register` makes this
  the ordinary double-click path.
Slices: sketched at phase-open. **Deliberately after P06** — tabs work without it,
and it is the one phase whose risk is process lifecycle rather than document
state.

**(reality drift, 2026-08-16, found during P01.S02's review sweep — premise
correction only, the phase is not re-scoped)** The multi-document pending entry
introduced P07 as *"new: nothing today handles an already-running nib"*. **That
is false at the line.** `internal/singleton` exists; `cmd/nib/main.go:52` calls
`singleton.ReplaceOthers()` behind a `--replace` flag; and `build/nib.desktop`
ships `Exec=nib --replace %f`, so the desktop launcher passes it on every
activation. `ReplaceOthers` **SIGTERMs every other process running the same
executable** (`internal/singleton/singleton_linux.go`).

So the real starting point is not "nothing handles it" but "it is handled by
killing the other instance". P07 is therefore a **change of policy — replace-and-
kill into hand-off-and-focus** — not a build from zero, and it inherits a working
process-discovery mechanism rather than needing one.

Two consequences worth carrying, neither actioned here:
- **It interacts with P01.S04.** Opening a second PDF from the file manager
  today SIGTERMs the running nib and discards whatever was open — no Close, no
  unsaved-work prompt. So the confirm S04 adds is bypassable by the ordinary
  double-click path until P07 lands. S04 should say so rather than imply the
  prompt is unconditional.
- **The Windows exit criterion is the sharper one**, because `nib register`
  (v1.102.0) makes double-click the ordinary path there, and `ReplaceOthers`
  matches on the executable — worth confirming that predicate on Windows during
  P07 rather than assuming the Linux shape carries.

---

## Out of scope

- Folding **Compare** into the tab model (D11).
- A general "dirty document" model. P01.S04 uses the three signals that exist;
  a unified dirty flag is a larger change.
- Zeroing document bytes in memory on close — the rings release references but do
  not wipe the heap (see Standing caveats).
- Save-all / multi-document batch operations. Not asked for; not implied.
- Legal, GTM, and marketing-site work (out of scope for this arc by construction).

## Standing caveats

- **Closing does not erase.** `clearUndoLocked` nils entries so the byte slices
  are collectable, but nothing zeroes them; a closed document's plaintext remains
  in freed heap until the allocator reuses it. Close reduces exposure; it does not
  guarantee erasure.
- **Window close still discards silently.** There is no `beforeunload` handler
  anywhere, so closing the *window* with unsaved edits loses them today and will
  continue to after P01. Adding one is its own decision about an app-mode window.
- **`display:none` hides scroll reads.** Scroll position survives, but reads
  return 0 while hidden — any save/restore logic must not read it from a hidden
  view.
- **No test coverage exists until P02.** Everything before it is verified by
  driving a browser by hand.
- **An armed p2p session is not document-scoped**, and stays armed through Close
  and Close all. That is deliberate — arming is a standing arrangement the user
  set up independently of whatever is on screen — but "Close all" makes it far
  more visible than a single Close did, because the app can now sit at the empty
  state while still listening. The status surface must say so.
