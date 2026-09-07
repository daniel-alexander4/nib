# PLAN — the ceremony wizard

**Dateline.** Seeded 2026-09-07 from `/grill "this plan"`, run with the `ceremony-flow` panel
(8 seats: closing agent, long-running-workflow engineer, e-signature implementation consultant,
notary/RON operator, corporate secretary, multi-step-flow designer, legal-documents practitioner,
single-operator user). **Two of the pre-grill recommendations were overturned by that panel** and
both are recorded below with the argument that beat them, because both look like simplifications
and are not.

**Where this plan and the wizard sketch differ, the plan wins.**

**Status: unbuilt.**

---

## What this is

The ceremony works and has no surface worth the name: the panel is read-only, the steps live in
`PLAN-signing-ceremony.md`, and a user is expected to hold the order in their head. This plan is
the surface — the thing that tells a convener what to do next and a signer what they are agreeing
to.

**There is no P00**; the repo needs no bootstrap.

## What is already true, and was read rather than assumed

- **`/api/ceremony/next` already computes whose turn it is**, from the record, via
  `p2p.NextContributor` — the same function the server's L3 check uses. Its own header:
  *"The rule is not written here and must never be… this file is a route, not a rule."*
- **It opens the document**, and its header records the cost: **10 / 69 / 195 ms for 100 / 500 /
  1000 pages, superlinear**. It is a per-ceremony route for exactly that reason.
- **Invitations can be reissued** — `handleCeremonyInvites` reads the mirror by ceremony id.
- **There is no draft state.** Named search across `internal/ceremony` and `internal/server`: the
  first durable artifact is the signed record written by `convene`. Everything a convener types
  before that is held in the page and lost on a close.
- **A wait-tier diagnosis already exists** client-side (`reflectDiagnosis`, `TIER_WORDS`).
- **The signer's intent is a second recital.** `convene.go:40` says *"Intent is the recital every
  party agrees to, and D20 makes it the only home for it"* — and the signer is separately asked
  "what you're agreeing to", prefilled with the hardcoded `"I agree to sign this document."`
  (`app.js:1362`), which is the string the signature carries.
- **Accepting an invitation is purely local** — `accept.go` makes zero `p2p` calls, so no
  acceptance ever reaches the convener.

## The law this establishes

**The wizard renders state; it never holds it.** Anything the wizard shows about where a ceremony
stands is read from the server, which computes it from the record. A step counter, a "current
stage", or a cached roster position in the client is a second derivation of a rule the server
already owns — the ADR-009 shape this repo keeps paying for — and it is also what makes a
days-long, interruptible proceeding resumable for free.

---

## Decisions

### D1 — The rail renders `next`; the client holds no step *(settled 2026-09-07 via /grill)*
`/api/ceremony/next` is the single answer to "what happens now", and the route's own comment
forbids restating the rule. The wizard's enabled action is a rendering of that answer. Resumption
then needs no design: reopening Nib after two days asks the same question and gets the right one.

### D2 — `next` is fetched on open and on hop completion, never on a timer *(settled 2026-09-07 via /grill)*
It opens the document, at a cost its own header measured as superlinear in page count. A rail that
polls it converts a deliberate per-question cost into a background load that scales with how large
the user's document is. This is the plan's only hot-path rule.

### D3 — Two surfaces, cut by what the user is doing *(settled 2026-09-07 via /grill)*
**Setup** is form work — roster, recital, deadline — and does not fit a 200px sidebar
(`style.css:347`); a second rail is refused outright, because `responsive.test.mjs` holds chrome to
≤33% of the viewport down to 360px and two rails breach it at every width where the toolbar already
folds. Setup is therefore a **full-width sheet owned by the Ceremony mode**, dismissible and
re-enterable. **Running** is document-referential and interrupt-driven, so it is the **sidebar
rail**, with the document still on screen. Placing signature blocks leaves the sheet for the page,
where that panel already lives.

### D4 — Setup persists a local draft *(settled 2026-09-07 via /grill — multi-step-flow seat)*
Today the first durable state is the signed record, so a convener who closes Nib mid-setup retypes
the roster and the recital. The draft is local, unsigned, and consumed at `convene`. Without it the
sheet is a form that cannot be left, which is the abandonment case every long form has.

### D5 — The spoken check stays mandatory *(settled 2026-09-07 via /grill — OVERTURNS the pre-grill recommendation)*
The sketch made it conditional on having a voice channel, reasoning from D21 that the invited path
is already a full-strength pin. **The notary seat refused that and is right.** The pin proves the
peer is a machine holding that key; the spoken words prove a *human the party recognises* is at it.
Those are different claims and the cryptography answers only the first. Worse, "only when you have
a voice channel" asks the user to self-assess a security control, which is how controls stop being
performed. **The record notes whether the check was presented**, so a later reader can tell a
confirmed ceremony from one where the modal never appeared.

### D6 — The rail scales: one action, then a worklist *(settled 2026-09-07 via /grill — OVERTURNS "one enabled action, always")*
One enabled action is right at three parties and wrong at thirty-two, where a coordinator working
through six hours of hops needs to see who remains. The rail shows a single action below a stated
threshold and a worklist above it. **The threshold is a number this plan owes**, and until it is
chosen no test of the behaviour can fail — it renders at whatever size the fixture uses.

### D7 — Overlay is reserved for the two synchronised moments *(settled 2026-09-07 via /grill)*
The spoken check and the final sign/decline: seconds long, and the user must not be able to do
anything else. Everything else spans days, and a modal wizard would tell the user they are in a
session when they are in a document that has a ceremony attached.

### D8 — The ceremony is not a document tab *(settled 2026-09-07 via /grill)*
`#tabstrip` is `role="tablist"` over documents. A ceremony is not one, the accessibility sweep
already found an orphan `role=tabpanel` there, and a non-document member makes "what is a tab"
ambiguous for users and screen readers alike. It would also hide the document at the moments the
wizard exists to keep it visible.

### D9 — A waiting screen shows the diagnosis, or says plainly that there is none *(settled 2026-09-07 via /grill)*
Two moments have nothing true to display, because no acceptance and no presence travel back. Where
nib already computes a wait-tier diagnosis, the rail renders it. Where it cannot, the rail says so
in words rather than spinning — a spinner claims knowledge the software does not have, and the
absence of a back-channel is the one fact a user must understand about this design.

### D10 — A terminal ceremony stops offering an action *(settled 2026-09-07 via /grill)*
Declined and expired are defined end states with no handler in any surface today. The rail names
the state and offers nothing, rather than inviting a call for a proceeding that has ended.

### D11 — "Send" is named honestly, and reissue is offered *(settled 2026-09-07 via /grill — closing-agent seat)*
Nib does not deliver invitations; the convener hands them out over their own channel. The step says
so. And because the API already supports regenerating them, the rail offers reissue — a settlement
agent re-sends constantly, and presenting it as one-shot is a surface limitation, not a design one.

### D12 — There is no correction path, and the wizard says so before the first hop *(settled 2026-09-07 via /grill — closing-agent seat)*
A signature is an append; a wrong one cannot be undone, and the remedy is to abandon and re-convene,
losing every signature collected. That is the most common bad day in this line of work and the
design has no answer. Stating it up front is the honest surface; whether the answer should change is
`/pending`, not this plan.

### D13 — The recital has one home *(settled 2026-09-07 via /grill — legal-documents seat)*
Prefill the signer's statement from the record's recital. Today the signature carries a generic
hardcoded sentence while the record carries the real one, so the two disagree in every ceremony
where a signer does not retype it.

### D14 — Accepting arms *(settled 2026-09-07 via /grill)*
A signer who accepts and never arms is indistinguishable, from the convener's side, from one who
ignored the invitation. Accepting arms the listener, renewed while Nib runs and bounded by the
record's deadline.

### D15 — Machine steps are not screens *(settled 2026-09-07 via /grill)*
Verifying order, saving the signature, closing out and receiving the finished document are things
the software does. They appear as state, never as a step the user is asked to perform.

---

## Build order

### P01 — The rail
**Goal.** A convener or signer opens a ceremony and is told, correctly and without asking anyone,
what happens next — including when the answer is "nothing, and here is why".

**Exit criteria.**
- The rail's enabled action equals `/api/ceremony/next`'s answer, and no step state exists in the client.
- `next` is fetched on open and on hop completion only, proved by a fetch count over a minute of idling.
- A declined or expired ceremony names its state and offers no action.

#### P01.S01 — the ceremony list is the front door
Scope: the sidebar card lists ceremonies from `/api/ceremonies` and opens one. Refs: D1, D2.
Acceptance: several ceremonies list; opening one fetches `next` exactly once; listing fetches it
none, because the listing route deliberately never opens a document.

#### P01.S02 — the rail renders the next action
Scope: one enabled action, labelled from `next`. Refs: D1, D15.
Acceptance: the rendered action equals the server's answer, asserted; a red proof shows a
client-side guess diverging; machine steps appear as state and are not clickable.

#### P01.S03 — terminal states and the honest wait
Scope: declined and expired name themselves; waiting renders `TIER_WORDS` where a diagnosis exists
and a plain sentence where none does. Refs: D9, D10.
Acceptance: a terminal ceremony offers no action; a waiting rail never shows a bare spinner.

#### P01.S04 — the fetch discipline
Scope: fetch on open and hop completion; never a timer. Refs: D2.
Acceptance: idling for a minute produces zero further fetches, asserted rather than observed;
`nextFetchMs` is recorded per open.

### P02 — The signer's surface
**Goal.** One review surface — the document, the block where it will land, the roster — and a
recital that agrees with the record.

**Exit criteria.** The signature carries the ceremony's recital by default; review is one surface
rather than three screens; accepting arms; the spoken check is presented and its presentation is
recorded.

### P03 — The convener's setup sheet
**Goal.** Roster, recital and deadline in a surface with room for them, resumable before commit.

**Exit criteria.** Setup survives closing and reopening Nib; the draft is consumed exactly once at
`convene`; block placement leaves the sheet for the page and returns.

### P04 — Scale and repair
**Goal.** The rail at a full roster, and the operational steps the design has never had.

**Exit criteria.** The worklist threshold is a stated number and the rail is asserted at sizes
either side of it; invitations can be reissued from the rail; the no-correction rule is stated
before the first hop.

---

## Out of scope

- **Changing the topology.** The serial hub and its one-pinned-peer tripwire are `PLAN-signing-ceremony.md`'s, and widening either needs a fresh security review.
- **A presence or acceptance channel.** Deliberately deferred there; this plan surfaces its absence rather than filling it.
- **Making the delivery round concurrent** — that is `/pending 376`.
- **A correction or substitution path.** D12 states the limitation; changing it is a different plan.

## Standing caveats

- **The worklist threshold does not exist yet** (D6), and until it does, any test of that behaviour
  can only report pass. It is chosen from rendering at several roster sizes, not guessed.
- **Whether a correction path is genuinely needed is unmeasured.** The inventory's row 19 is the
  only evidence that would settle it, and it has no reader today.
- **The setup draft is a new persisted artifact this plan did not author.** Where it lives, whether
  it needs the vault, and how it interacts with the mirror `convene` later writes should be dived
  before P03 rather than decided inside it.

## Seam inventory

`~/.claude/projects/-home-dan-repos-nib/memory/instruments/ceremony-wizard.md` — 19 rows (7 paths,
6 seams, 6 gap-downs), written against this plan before any code. One hot-path row (`next` on open),
one row coarser than its own clause (the worklist threshold), and two
`diagnostic, no standing reader` entries, one of which is a permanent structural zero: nib does not
deliver invitations and cannot observe delivery.
