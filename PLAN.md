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
P02 both replaced with proven house patterns; one open question for Dan (ADRs).
Stage 6 dimension reviews 2026-08-15: hot-path/security, host availability, scale,
the crypto and verification SME packs, and "what haven't we looked at" → D9
amended from a count cap to count+bytes, plus pins on P05 (the one hot path; the
three bindings that fail as safety defects) and P06 (what a reload does).
Stage 7 plan-review 2026-08-15: structural gate + SME panel → one **critical**
finding landed as a pin on D7 (ids must never be reused, or the pinning law passes
while corrupting), three consistency defects fixed, D-numbering repaired.
Stage 8 bootstrap: **not run, by decision** — see D14.

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
machinery, and repo laws, all of which govern here unchanged. (It has no ADR
practice; see D19, where that is raised as a question rather than assumed.)

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

### D5 — Binding strategy: reassignable module bindings *(settled 2026-08-15 via /grill)*
`viewer`, `eventBus`, `linkService`, `findController` become `let` and are
repointed to the active view on switch; `pdfDocument` already is one. This is
what makes D3 affordable: roughly 198 call sites read these at call time and need
no edits. Only `els.viewerContainer` (50 refs) needs a swappable binding.

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

**Question for Dan (does not block P01)** — §11 says a new architectural decision
gets an ADR in the same change, and nib has no `docs/adr/` at all. D3 (per-view
viewers) and D7 (the pinning law) are architecture by any reading. Adopting ADRs
is a repo-wide convention change, so this plan does not impose one unilaterally.

---

## Build order

### P00 — Bootstrap *(pre-satisfied — see D14)*
Goal: repo scaffolding. Already present: CLAUDE.md, VERSION, build/install
scripts, embed.go, nfpm packaging, desktop entry, wine harness.
Exit criteria: n/a.

### P01 — Close the open document
Goal: the app can put a document down without quitting or opening another,
returning to exactly the state it launches in.
Exit criteria:
- `/api/pdf` 404s and the UI shows "Open a PDF to begin." after Close.
- The undo rings are cleared — the document's bytes are not retained.
- Opening a document after a Close works normally.
- Full-repo `/code-review` clean; shippable as a release.

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

#### P01.S02 — Server: drop the document
Scope: `POST /api/close` → `setDoc(nil)`, which already clears both rings.
Refs: D2.
Acceptance:
- After close, `/api/pdf` 404s and `/api/doc` returns the empty `docResponse`.
- Undo history is gone: `canUndo` **true before the close and false after** —
  `docResponse` computes it from `len(s.undo)` independently of `doc`, so this
  detects an uncleared ring; asserting only the post-close `false` would pass
  against a server that never had history.
- `POST /api/close` with nothing open is idempotent: 200 and the empty
  `docResponse`, matching the `ErrNotEncrypted`-passes-through precedent (v1.57.0).
- **(carried from P01.S01, 2026-08-15)** A close landing *while* one of the four
  mutating routes is in flight makes that route answer 404, not 200. S01 closed
  this class structurally — the test-and-write is one lock acquisition — but
  recorded the clause `not exercised`, because until this slice there was no way
  to make a close happen mid-operation. It is exercisable here and must actually
  be driven, not inherited as met.

#### P01.S03 — Client: the teardown
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

#### P01.S04 — Client: the Close control and the unsaved-work confirm
Scope: a **Close** button in the File tab toolbar, registered in `DOC_REQUIRED`;
confirm before discarding unsaved work, from the three signals that exist
(`annotationStorage.size`, overlay fields/history, `docMeta.canUndo`). No Ctrl+W
— Chromium app-mode owns it. Refs: D2.
Acceptance:
- The control greys out with nothing open.
- Closing with unsaved edits prompts; cancelling leaves the document untouched.

### P02 — Test harness, in three tiers
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
Slices: sketched at phase-open.

### P03 — Document identity, server side
Goal: the server holds N documents and every document-touching endpoint says
which one it means. Refs: D6, D8, D9, D10.
Exit criteria:
- `s.doc` replaced by a registry + active id behind one accessor; the ~14 nil
  guards keep their shape.
- Every document-touching route carries an id; `/api/undo` and `/api/redo` stop
  being bodyless.
- Per-document rings under one global byte budget; single-document behaviour
  unchanged, proven by the existing Go tests.
- Arrivals (co-sign, p2p) open a new document rather than replacing one.
Slices: sketched at phase-open.

### P04 — Operation pinning, client side
Goal: no operation acts on a document it did not capture at its start. Refs: D7.
Exit criteria:
- The 13 corrupting post-await sites capture their document and id before the
  first `await` and carry it through.
- The ~25 mislabeling sites (all through `exportBase()`) name the document they
  actually exported.
- A harness test proves the `save()` case: begin a save, switch documents mid-
  flight, and assert the *other* document's file is untouched.
Slices: sketched at phase-open. **No user-visible output — this is the safety
phase and it cannot be skipped.**

### P05 — Per-view state and viewers
Goal: each document owns its viewer, its DOM and its state records. Refs: D3, D5,
D11, D12.
Exit criteria:
- Per-view `PDFViewer`/container; inactive views hidden, never destroyed.
- The seven silent-loss bindings plus `signLocked`, `lastSig`, `docGen` are
  per-view; `detachField`/`reattachField`/`relayoutOverlays` resolve the owning
  view, not the active one.
- The 26 pointer listeners re-homed to the stable parent; cleanup sweeps
  view-scoped.
- Re-fit and dpr-heal on activation (a view that loads hidden gets no scale).
- The two sidebars show the active document's content and nothing else.
Slices: sketched at phase-open.

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
