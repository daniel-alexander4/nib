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

**Amended 2026-09-07 at P02.S02's deepdive — "the record's deadline" is not available at accept
time and cannot be made so within this plan.** An invitee holds no record until the document
reaches its hop, and the invitation carries no deadline: giving it one is `/pending 247`, deferred
behind an undischarged security gate. The bound is read from the record wherever this machine holds
one and is `ceremony.MaxCeremonyLife` where it does not — so the clause is met from the moment a
record exists and is honestly unmet before that, rather than being reported as met against a field
nothing populates. The real bound before the document arrives is the process lifetime, which is
what "renewed while Nib runs" already says.

### D15 — Machine steps are not screens *(settled 2026-09-07 via /grill)*
Verifying order, saving the signature, closing out and receiving the finished document are things
the software does. They appear as state, never as a step the user is asked to perform.

### D16 — A party who has not signed yet learns the proceeding ended, on the invitation as anchor *(settled 2026-09-07 via /discuss — Dan's call)*
D14 leaves a party who accepted holding an arm nothing local can close: every anchor that would say
the proceeding ended needs the record they do not have (`/pending 378`). The end state reaches them,
and it is verified against the **invitation**.

**No new object, and this was established by reading rather than assumed.** The convener already
mints a signed `Termination` on a decline and `runDeliveryRound` already carries it in place of the
document; it walks `rec.Roster` — every party but itself, the ender, and anyone already delivered —
so **a pre-hop party is already a target of the round**. And `Termination.Verify(rec)` uses the
record for exactly two values, `rec.RosterHash()` and `rec.Convener().Fingerprint`, both of which the
invitation carries directly. Its own doc already names the invitation as a legitimate anchor: *"`rec`
must come from the document or the invitation, never from the `record.json` sitting beside the
termination."*

**So the gap is two-sided and narrow.** The pre-hop party listens on the **hop** rendezvous that D14
arms, while the convener dials the **delivery** rendezvous at `deliveryHop`'s index — they never
meet — and no verification path exists for a machine holding no record.

**The anchor is extracted, not duplicated (ADR-009).** Both callers pass the same two values; a
second `Verify` that reimplemented the checks against an invitation would be the shape that ADR
refuses, and this is a signature check where two implementations disagreeing is a security bug
rather than an inconsistency.

**What this does NOT reach, stated so it is not read as more:** a ceremony that EXPIRES or is
ABANDONED mints no termination, because nobody can sign *"nothing happened"* — which is why those
two states are derived and `Termination`'s set is closed at two. Those parties still learn nothing,
and `/pending 247` cannot help: the deadline would need the record's `Version`, `DocHash` and
`DigestVersion` in the invitation to be verifiable at arm time, which is shipping the record inside
the invitation. **An end state can be made trustworthy to a pre-hop party with no format change; a
deadline cannot.** 247 is superseded on that asymmetry.

### D17 — A party can leave a ceremony, and leaving is local *(settled 2026-09-07 via /discuss — Dan's call)*
Today a party who wants out of a proceeding has no lever short of quitting Nib: D14's arm is raised
by a sweep and renewed at every unlock. Leaving prunes this machine's stored invitation, which is
what the sweep already keys on — `rearmCeremonies` skips a ceremony it holds no invitation for — so
the arm stops on the next sweep and never returns.

**It is not a decline and must not read as one.** A decline is an attested refusal the convener
learns about and the roster is entitled to; leaving is this machine saying it will no longer
participate, and it reaches nobody. Conflating them would either mint an attestation the user did
not intend or leave a decline nobody can see, and D28's end states are closed at two for reasons
that do not bend for a local action.

---

## Build order

### P01 — The rail *(done 2026-09-07, v1.128.15)*
**Goal.** A convener or signer opens a ceremony and is told, correctly and without asking anyone,
what happens next — including when the answer is "nothing, and here is why".

**Exit criteria.**
- ~~The rail's enabled action equals `/api/ceremony/next`'s answer, and no step state exists in the
  client.~~ **STRUCK and superseded 2026-09-07 on Dan's instruction (`/discuss`).** → **The rail
  RENDERS `/api/ceremony/next`'s answer and derives no second opinion from it, including the states
  it has no sentence of its own for; no step state exists in the client.**
- `next` is fetched on open and on hop completion only, proved by a fetch count over a minute of idling.
- A declined or expired ceremony names its state and offers no action.

**Why the first clause was struck.** It required an *enabled action* gated on `next`, and P01.S02's
reality-drift pin established there is no such action to gate: the card's only actions belong to
phases P01 does not own. Held to its original words the criterion could never be met by this phase,
and a criterion whose only enforcement point is a phase close that cannot fire is not a criterion.
**The superseding clause is what P01 actually built** and what D1 states — *"the wizard's enabled
action is a rendering of that answer"* — with the rendering half kept and the enabling half moved to
whichever phase ships an action. The parked question reached Dan three times before it was answered;
recorded so the next reader can see the criterion changed and by whose call.

**(pin, 2026-09-07 — the third clause's literal words are wider than its intent, and this records
the difference rather than quietly reading past it.)** *"Offers no action"* was written before any
close-out action existed. Two now do: the convener's delivery round, which **requires** an ended
ceremony because that is what it delivers, and *"Leave this ceremony"* (D17), which is the tidy-up
for a proceeding whose deadline passed. Neither advances the proceeding — nothing offers *sign* or
*continue* on a terminal ceremony, which is what the clause exists to forbid. Credited on that
reading, and the wider reading is named so a later slice cannot use it to add a continuation.

**Acceptance ledger — four clauses, split on every `and`.**

1. **"The rail renders `next`'s answer, deriving no second opinion"** — ✅ **met.**
   `ceremonyNextLine` branches on `state` and renders `d.reason` **verbatim** for every state it has
   no sentence of its own for, which is how P01.S03's `ended` reaches a person at all. The route's
   own header forbids restating the rule and this client does not.
2. **"no step state exists in the client"** — ✅ **met, by a named search.**
   `grep -nE "cerStep|ceremonyStep|wizardStep|currentStep|step *=|stepIndex" web/app.js` returns
   four hits, all of them compare-alignment or a panel colour index; none is a ceremony.
3. **"`next` is fetched on open and on hop completion only"** — ✅ **met by a stronger mechanism
   than the clause asks for**, which S04's pin records: it is fetched at exactly **one** call site
   (`app.js:11817`), behind a per-card button, so it is not merely un-polled — it is not fetched on
   open either. The fetch-count-over-a-minute proof the clause names is therefore vacuous by
   construction rather than owed: with no timer and no open-fetch there is nothing to count.
4. **"A declined or expired ceremony names its state and offers no action"** — ✅ **met**, on the
   pin above. `endedReason` produces the person-facing sentence and the rail renders it;
   `TestAnExpiredCeremonyIsNotSomebodysTurn` and `TestEndedReasonNamesTheStateThatEndedIt` are the
   readers.

**Required-run gates, discharged at v1.128.14.** Tier 0 ✅ · tier 1 ✅ · tier 2 215/215 ✅ ·
tier 3 93/93 ✅ · tier 4 ✅ · tier 6 19/19 ✅ · `-race` ✅. **No separate full-repo review was run
for this phase, and that is stated rather than skipped silently:** P01's three slices are two
no-code closures and one server change (v1.128.2), and P02's close reviewed the same tree hours
later — including the live pass that found the D14/D21 regression. A second agent-driven read of an
unchanged tree would have re-read P02's work, not P01's.

**Closure sweep over `/pending`: population EMPTY** — the Phase section holds no items.

#### P01.S01 — the ceremony list is the front door *(done 2026-09-07 — already built, no code)*
Scope: the sidebar card lists ceremonies from `/api/ceremonies` and opens one. Refs: D1, D2.

**(reality-drift pin, 2026-09-07 — deepdive before the grill.)** This was already shipped.
`loadCeremonyPanel` (`web/app.js:11641`) fetches `/api/ceremonies` unpinned, renders the cards
through `renderCeremonyPanel`, returns the live+ended count, and renders a distinct sentence on
failure rather than an empty machine. It has four call sites, including the **locked** screen
(`app.js:496`). Nothing was owed and nothing was written.

#### P01.S04 — the fetch discipline *(done 2026-09-07 — already satisfied, no code)*
Scope: fetch `next` on open and hop completion; never a timer. Refs: D2.

**(reality-drift pin, 2026-09-07.)** Already true, and by a stronger mechanism than D2 asked for:
`next` is fetched only when the user presses a per-card **"What happens next?"** button
(`web/app.js:11379-11387`), so it is not merely un-polled, it is not even fetched on open. The
per-card answer also checks the echoed ceremony id before rendering, so a slow answer for one card
cannot appear under another. **D2's hot-path rule is therefore a rule the code already keeps** —
recorded so a later slice does not "add" a fetch-on-open and think it is implementing this plan.

#### P01.S02 — the next answer becomes an action *(deferred out of P01 2026-09-07 — see the pin)*

**(reality-drift pin, 2026-09-07 — deepdive before the grill.) There is no action to enable yet.**
The card's only gated action is delivery (`web/app.js:11370`, convener of an ended ceremony);
`accept` (`:11923`) and `session/arm` (`:1379`) exist but are not panel actions, because P06.S02
made the panel read-only on purpose. The actions a `waiting` answer would enable — a convener
calling the next party, a signer arming — are what **P02** and **P03** build. Attempted here, this
slice would either invent a surface those phases own or ship a button that does nothing.

**Re-sequenced, not re-scoped:** it moves to after P03, where the surfaces it renders actions onto
exist. Sequencing is the arc's to settle; the exit criterion below is not.

**~~PARKED — a P01 exit criterion cannot be met and its amendment is Dan's.~~ ANSWERED 2026-09-07
on Dan's instruction (`/discuss`): the criterion was struck and superseded, and P01 closed at
v1.128.15.** The superseding clause keeps the rendering half and moves the enabling half to
whichever phase ships an action — which is what leaves this slice re-sequenced rather than blocked.

*(Pin corrected 2026-09-08 at resume. The paragraph above still read as an open park a day after it
was answered, and the phase heading twelve lines up already said `done`. A parked question that
stays written as parked after it is settled sends the next session to re-derive it — which is the
same cost as the park itself, paid again.)*

Scope: the per-card sentence becomes the rail's enabled action, labelled from `next`. Refs: D1, D6, D15.
Acceptance:
- The enabled action equals `next`'s answer; a red proof shows a client-side guess diverging.
- Machine steps render as state and are not clickable.
- Below the worklist threshold there is exactly one enabled action per ceremony.

#### P01.S03 — `next` learns the terminal states *(done 2026-09-07, v1.128.2)*
Scope: `/api/ceremony/next` reports a ceremony that has been declined or has passed its deadline,
and the panel names the state and offers nothing. Refs: D10, D9.
Acceptance:
- A ceremony past its deadline no longer answers `waiting`.
- A declined ceremony no longer answers `waiting`.
- The panel names the state and renders no action for it.
- The existing three states are unchanged for every non-terminal ceremony.

**(divergence pin, 2026-09-07.) The client needed no change, and the slice implied it would.**
The acceptance says *"the panel names the state"*; `ceremonyNextLine` already branches
`if (d.state !== 'waiting')` and renders `d.reason`, so a new state is named by the sentence the
server writes without a line of client code. Recorded rather than absorbed: a later reader
comparing the slice to the diff would otherwise look for the UI half and not find it.

**(build pin, 2026-09-07 — a mutation survived and the fix is a source scan.)** Removing the
route's call to `endedReason` left every behavioural test green: the rule was tested and nothing
asserted the route ran it. It cannot be tested behaviourally — the route reads a **signed**
`record.json`, so an expired ceremony cannot be staged on disk without making the record
unverifiable, at which point the route answers `unavailable` and proves nothing. Pinned instead by
a source scan over `handleCeremonyNext`, the idiom `docid.test.mjs` already uses over `app.js`;
it also pins that the check runs BEFORE the document read.

**(reality-drift pin, 2026-09-07 — this slice grew, and it grew server-side.)** It was planned as
UI. It is not: `internal/server/ceremonynext.go` contains **zero** occurrences of `Expires` or
`declin`, and its response type declares exactly three states — `waiting`, `complete`,
`unavailable` (`:42-48`). The route cannot report a terminal state it has no vocabulary for and
never looks for, so a ceremony past its deadline answers *somebody's turn* and the panel invites
the user to continue it. Expiry **is** computed elsewhere — `closeout.go:217` compares `Expires` to
now with a grace — so the fact exists and the answer does not carry it. **Ordered before S02
deliberately**: making a wrong action more prominent is worse than leaving it a sentence.

### P02 — The signer's surface *(done 2026-09-07, v1.128.14)*
**Goal.** One review surface — the document, the block where it will land, the roster — and a
recital that agrees with the record.

**Exit criteria.** The signature carries the ceremony's recital by default; review is one surface
rather than three screens; accepting arms; the spoken check is presented and its presentation is
recorded.

**Acceptance ledger — five clauses, split on every `;` and `and`.**

1. **"The signature carries the ceremony's recital by default"** — ✅ **met, upgraded from ⚠ at
   v1.128.16.** The record's `Intent` reaches `pendingView.Recital` and defaults the signer's box,
   and the submit path is untouched, so what is signed is what is in the box (tiers 1 and 2).
   `/pending 379` then built the missing half at **tier 4d**: on every hop of an `-n 3` relay — 4 of
   4, both transports — what the server offered the signer is compared against that party's own
   `record.json`. The client half stays tier 2's, because `pairrepro.sh` posts its own `intent` and
   asserting a client default from there would grade the harness's own input.
2. **"review is one surface rather than three screens"** — ✅ **met.** The document, the block and
   the roster render together; the block is drawn on the page it lands on, at the right scale, with
   the PDF→CSS origin flip asserted (`consentblock.test.mjs`). Two thirds of this clause were
   already true before the phase and the deepdive said so rather than claiming them.
3. **"accepting arms"** — ✅ **met and driven live.** Tier 6's *"after accepting, B arms with no
   manual pin anywhere (D21)"* passes, and `/api/session/status` reports armed within the wait.
4. **"the spoken check is presented"** — ✅ **met**, and not newly built: `runVerification` refuses a
   nil `Verifier` and `sessionVerifier` parks the gate. Driven live at tier 4, which prints a
   distinct verification string per transport.
5. **"and its presentation is recorded"** — ✅ **met, upgraded from ⚠ at v1.128.16.** Three
   states, written positively, seven mutations red — and `/pending 379` now reads
   `verification.json` back off each responder's home after a live hop, requiring `presented` AND
   `confirmed`, 4 of 4 across both transports. Asserted where it lands, not only where it is
   written.

**Required-run gates, discharged at v1.128.13 and enumerated from `CONTRIBUTING.md` rather than
from memory.** Tier 0 ✅ · tier 1 ✅ · tier 2 215/215 ✅ · tier 3 93/93 ✅ · tier 4
`pairrepro.sh` ✅ (both transports, 2 s of hops) · tier 6 `ceremonyrepro.sh` **19/19 ✅ — and it
was 18/1 before the fix below.** `go test -race ./internal/server/` ✅ over the arm, ceremony,
spoken-check and verification tests. Tier 4b/4c/4d and tier 5 not run: this phase touches no
discovery, transport or off-link path.

**The phase close earned its keep.** Tier 6 caught a regression nothing below it could see: **D14
inverted D21's observable invariant.** Accepting an invitation now arms, so the arm a party makes
afterwards was refused `409 a session is already armed` — the step D21 removed came back as a
conflict. Fixed by `arm.byPolicy` + `displacePolicyArm`: an explicit request displaces **this
machine's own accept-time arm and nothing else**, never a user's own arm and never one with a
consent request or a spoken check on screen. An idempotent 200 was refused as a silent downgrade —
the sweep arms QUIC on `0.0.0.0:0` and the caller asked for TCP on a bound address. Three guards,
three mutations, three red, and the third was written only because a probe showed the in-flight
condition decided nothing.

**Closure sweep over `/pending`: population EMPTY.** The Phase section holds no items, so no entry
was waiting on a P02 coordinate and nothing was falsified by the close.

**(pin, 2026-09-07 at v1.128.16 — tier 4d found a SECOND D14 defect after this phase was marked
done, and the marker says so rather than being quietly rewritten.)** The ledger above correctly did
not claim 4d: it was recorded as not run, because this phase touches no discovery or off-link path.
Running it for `/pending 379` failed at *"[quic] instance 3 could not arm before hop 1 (HTTP 409): a
session is already armed"*. A machine holding more than one accepted-and-unsigned ceremony auto-armed
for whichever id sorted first, and the explicit arm for the right one was refused by the
same-ceremony condition in this phase's own close-out fix. That condition is now gone — D22's
tripwire is what an armed listener ACCEPTS, not which of this machine's own guesses holds the slot a
moment earlier. **Two phase closes, two regressions from D14, both found by a tier that needs more
than one process** — which is the argument for running 4 and 6 at a close and not only 0–3.

**Slices firmed 2026-09-07 at phase-open**, against the code as it now stands.

#### P02.S01 — the recital comes from the record *(done 2026-09-07, v1.128.3)*
Scope: inside a ceremony, the signer's "what you're agreeing to" defaults to the ceremony's own
recital rather than a hardcoded sentence. Refs: D13.
Acceptance:
- In a ceremony, the default text is the record's `Intent`.
- Outside a ceremony (a plain two-party co-sign) the existing default is unchanged.
- A signer who edits it still signs what they typed.

**(build pin, 2026-09-07.) The recital is read from the INVITATION, and that is the intended
source rather than a compromise.** `invitation.go:707-717` reconciles the invitation's recital
against the record's and refuses a mismatch as *"two different proceedings however alike they
look"* — and its own comment says the invitation's copy *"is the copy the signing path reads,
because `internal/p2p` cannot read a record"*. So `cer.inv.Intent` **is** the record's recital,
pinned by a guard, and the acceptance clause is met at the line rather than by resemblance.

**(build pin.) The third clause is `not exercised`.** Nothing drives "an edited value is what gets
signed". The diff touches `srvIntent.value` exactly once — the default — and the submit read
(`app.js:1655`) is untouched, so the behaviour is unchanged rather than unverified-and-changed. It
is recorded here rather than counted as a pass, and it is not this slice's point.

**(review pin, F3.) The rule was extracted mid-slice so it could be tested.** It began as two lines
at a call site reachable only with a live ceremony session in flight, which left a source scan as
the only instrument. `recitalFor(cer)` is now a pure function with a real test, and the scan is
narrowed to the one thing still unreachable — that the consent view calls it.

#### P02.S02 — accepting arms the listener *(done 2026-09-07, v1.128.6)*
Scope: `/api/ceremony/accept` leaves the machine listening for the convener, renewed while Nib runs
and bounded by the record's deadline. Refs: D14.
Acceptance: accepting reaches an armed state without a second user action; the arm is bounded; a
harness run does not arm.

**(deepdive, 2026-09-07 — `deepdives/2026-09-07-p02s02-accepting-arms-the-listener.md`.)** Ran
because the slice moves *when* an existing arm opens. Both halves of D14's premise were measured
rather than argued: after a successful accept, `/api/session/status` reports `{armed: false}` and
`/api/ceremony/next` reports `unavailable`.

**(build pin — D14's second clause names a source that does not exist on this path, and D14 is
amended rather than met.)** An invitee holds **no record** until the document reaches its hop
(`accept.go:40` says exactly that), `Stored.Expires` is populated only from `record.json`, and the
invitation carries no deadline — that is **`/pending 247`, deferred behind an undischarged G2** (a
shortened `Expires` is consumed at arm time and `MatchesRecord` cannot run until the document
arrives, so the mismatch is never seen). `armWindowFor` already states this and cites 247 by
number. **The bound is therefore read where it exists and not invented where it does not:** the
interactive ceremony window becomes the record's remaining life when this machine holds a record,
and stays `ceremony.MaxCeremonyLife` when it does not. The fallback is deliberately the *opposite*
direction from `deliveryWindowFor`'s, and the asymmetry is the reason: a delivery arm exists only
after this party has signed, so a missing record there is anomalous and a short floor is safe; an
interactive arm exists *before* the document arrives, so a missing record is the ordinary case and
a short floor would take the signer off the network minutes after they accepted.

**(build pin — the mechanism exists one slot over, so this is a sibling sweep and not an
invention.)** `rearmDeliveries` + `EnableDeliveryRearm` + the `adoptVault` hook is already
"re-established at every unlock, best-effort per ceremony, anchored on the invitation rather than
on `Stored.Ended`, failing open toward arming". **Its process gate is the whole of the third
acceptance clause** — `EnableDeliveryRearm`'s own doc records the measured failure an ungated
sweep caused (`TempDir RemoveAll cleanup: directory not empty` across five unrelated tests). A
harness run does not arm because it does not call the enabling method, not because of anything new.

**(build pin — the hop arm has to be EXTRACTED, and that is this slice's risk.)** Unlike the
delivery arm, the hop arm exists only inside `handleSessionArm`'s QUIC branch and is reachable
only from an HTTP request. A sweep that reimplemented it would be two implementations of one rule,
which is the ADR-009 shape this repo keeps finding. T01 extracts the door and makes the route its
first caller, with no behaviour change; everything after that is wiring.

**(build pin — the auto-arm is QUIC, and the reason is a defect in the manual path.)** `armRecv`
sends no `transport`, and `ceremonyTransport` defaults anything that is not `"quic"` to TCP — so
**every browser-driven ceremony arm today is TCP**, the path whose own comment says it *"can be
REACHED but never FOUND"*. Filed rather than fixed here; it settles this slice's transport, which
matches `armForDelivery`'s.

**(scope pin.)** The rail's sentence for a just-accepted invitee is false — it blames a removed
folder. Measured, and **filed as `/pending 377`** rather than folded in: its cause is
`ReadStored` having no state for "a party, and nothing has arrived yet", and it belongs where the
rail lives.

**(build pin — one interactive slot, so the arm is best-effort.)** `armInteractive` is shared with
the user's manual receive arm and `armIn` refuses a collision. The sweep therefore never fails an
accept over a slot; a machine in two live ceremonies arms exactly one, chosen by `ListStored`'s
sort. Recorded as this slice's residual doubt.

Tasks:
- T01 — extract the ceremony hop arm from `handleSessionArm` into one door; the route becomes its
  first caller, byte-for-byte in behaviour.
- T02 — `armWindowFor(armInteractive, cer)` reads the mirror where one exists, falling back to
  `MaxCeremonyLife` where it does not.
- T03 — `rearmCeremonies`, the sibling sweep: the invitation as anchor, the convener skipped, an
  ended or completed proceeding skipped, best-effort per ceremony.
- T04 — two triggers behind the existing process gate: the `adoptVault` hook, and the tail of
  `handleCeremonyAccept` on a detached goroutine.
- T05 — tests, each probed red: accepting arms; an unenabled `Server` does not; the window is the
  record's where a record exists; the convener is not armed for; an ended ceremony is not armed for.

#### P02.S03 — one review surface *(done 2026-09-07, v1.128.8)*
Scope: the document, the block where it will land, and the roster on one surface instead of three
consecutive screens. Refs: D3, D15.
Acceptance: a signer sees all three without navigating; the block shown is the block stamped.

**(deepdive, 2026-09-07 — `deepdives/2026-09-07-p02s03-one-review-surface.md`.)**

**(scope pin — two thirds of this slice is already built, and the premise is stale rather than
wrong.)** `showConsent` already renders the peer, the recital, `renderConsentSigners`' roster with
invalid signatures marked, and `loadPendingPreview` over **every page** of the received document,
all on one screen. `showRecvView`'s three views are the arm, the wait and the consent — a sequence
in time, not three screens a reviewer navigates between. The slice reduces to **the block**, which
is the one of the three that is genuinely absent.

**(build pin — the client has never been told where its block goes, and the code says so at both
ends.)** `handleSessionQuote` returns `p2p.NominalBlockRect()`, whose own doc calls it *"a size
template, not a placement — the caller wants a rect of the right shape and must not care where it
says it is"*, and `app.js:956` consumes only its width and height. The real placement is computed
server-side after consent.

**(build pin — the acceptance clause is met by ONE DOOR ON ONE INPUT, not by agreement.)**
`p2p.PlacementFor` is already ADR-009's single door for this question. Its stamp-side call is
`PlacementFor(inbound, roster, me)` at `p2p/session.go:1097`, eight lines after
`c.Confirm(peer, inbound)` — and `Confirm` is handed `doc []byte`, which **is** `inbound`. Named
search: `p2p.Receive(` has one production call site and `sessionConfirmer{` one construction, and
they are the same line, built with `cer: cer` against a roster argument of `cer.l3Roster()`. So the
confirmer can call the same door with the same bytes and the same roster object. Two computations
checked for agreement is the shape ADR-009 refuses; this is not that.

**(build pin — no branch for the manual co-sign.)** `PlacementFor` already answers `NextPlacement`
when there is no roster, so a plain two-party co-sign gets a real placement from the same call.

**(build pin — the quote's pinned `when` is NOT moved.)** The placement is a fact about the
document and is computed independently; the quote stays minted at Accept. Moving it to consent time
would put the pinned time at the mercy of how long the user reads, which is the defect P06.S06
fixed.

Tasks:
- T01 — `sessionConfirmer.Confirm` computes the placement from the bytes it is already handed, and
  it reaches the pending view.
- T02 — the consent preview draws the block over the page it will land on.
- T03 — tests, each probed red: the placement reported is the one stamped; a document whose
  placement cannot be computed still shows the rest of the surface; the box lands on the right page.

#### P02.S04 — the spoken check records that it was presented *(done 2026-09-07, v1.128.12)*
Scope: the record notes whether the verification modal was shown, so a later reader can tell a
confirmed ceremony from one where it never appeared. Refs: D5.
Acceptance: presented / confirmed / not-presented are distinguishable after the fact.

**`/plan-review` trigger: FIRED and discharged in place.** P02 changes what a signature *carries*
(S01) and when a machine listens (S02), which is security-heavy by the arc's test. Discharged
rather than fanned out because the phase's security surface reduces to two questions, both settled
below at the line rather than by a panel: **(a)** the recital is already a signed field of the
record with its own length bound (`ErrIntentTooLong`), so S01 changes which *existing* validated
string is defaulted into a signer's box and adds no new signed content; **(b)** S02 moves *when* an
existing arm is opened, not what it accepts — the listener still takes one pinned peer and one
session, which is the tripwire D22 protects. A slice that turns out to touch either property
re-fires this trigger rather than inheriting this paragraph.

### P03 — The convener's setup sheet *(**done** 2026-09-08, v1.128.46 — four slices. **Acceptance ledger: 3 criteria, 7 clauses, 6 met and ONE refused** — `ledgers/2026-09-08-p03-acceptance.md`. The refusal is C03, *"block placement leaves the sheet for the page"*: in a ceremony nobody places a signature block, and the clause is **refuted rather than unbuilt** — parked for Dan rather than re-worded to fit what shipped. C03b, what it was reaching for, is built and driven at two tiers. **C01b was met by NOTHING when this phase's last slice closed**: S02 proved the draft's blob survives a process restart and nothing anywhere asserted it is read back into the FORM — found by splitting the criterion on its own `and`, built here, 4 mutations red, one of them the exact `.value`-vs-`dataset` defect S02's review had caught by eye. **The full-repo review found four defects across four slices that no slice review could see**, three of them created by the joins between slices: the draft consumed ABOVE a refusal that can still roll the ceremony back (so a convener could be left with no ceremony AND no setup); `clearCeremonyForm` resetting `#cerISign` to false against a markup that ships it checked, so every ceremony after the first defaulted to *"the convener does not sign"*; leaving the Collaborate mode hiding the sheet without parking it, so the path the excursion exists for was the one with no thread back; and a null document binding that is not a refusal. **The ordering guard that should have caught the first was green over it** — it matched `httpError(` and the refusal is spelled `wroteCommitFailure(`, which is `CLAUDE.md`'s own lesson about a guard asserting the text a site prints. **Required-run gates, enumerated because a ledger over criteria cannot see them**: tier 0 ✓, tier 1 ✓, `go vet` ✓, `gofmt` ✓, tier 2 ✓ (246), tier 3 ✓ (101), **tier 6 ✓ 19/19 and tier 4 ✓ both transports — and they FIRED**, against a first reading that said they did not: P03.S03 touched `internal/server/convene.go`, a ceremony path, and its commit records no gate line in either direction (`/pending 426`). **Graduation pass: 22 rows, 3 actionable, 0 hot-path, nothing deleted or gated**; all 31 readers resolve. **Closure sweep: the `Phase` section is EMPTY**, so this close falsified nothing — recorded because a sweep that closes nothing must say so.)*
**Goal.** Roster, recital and deadline in a surface with room for them, resumable before commit.

**Exit criteria.** Setup survives closing and reopening Nib; the draft is consumed exactly once at
`convene`; ~~block placement leaves the sheet for the page and returns~~ **→ the third criterion is
REFUTED, not unmet, and is parked for Dan rather than re-worded** (P03.S04's pin: in a ceremony
nobody places a signature block, `ceremonyPlacement` derives it from the roster position). What it
was reaching for — leaving the sheet for the document and returning without a rebuild — is built and
driven.

**Slices firmed 2026-09-07 at phase-open**, against the code as it now stands.

**Two facts established before cutting, both by reading:** the convene form is `#ceremonyConveneForm`
and it lives **inside `<aside id="sidebar">`** — which is the 200px surface D3 says it does not fit.
And a named search for a persisted setup draft (`grep -rniE draft web/app.js internal/server/*.go`)
returns only prose about first drafts of code: **nothing persists one today**, so S02 builds an
artifact rather than moving one.

**`/plan-review` trigger: did NOT fire.** This phase is not security-, migration- or egress-heavy:
the draft is local, unsigned, carries no key material and crosses no boundary. What it *does* is
persist a roster and a recital, which fires the **privacy/data-protection seat** at S02's own grill —
`CLAUDE.md` puts that seat on any change that *"persists, publishes or transports anything"*, and
its question is always residue. Recorded rather than skipped silently.

#### P03.S01 — the setup sheet has room *(done 2026-09-07, v1.128.23)*
Scope: the convene form leaves the sidebar for a full-width sheet owned by the Ceremony mode,
dismissible and re-enterable. Refs: D3.
Acceptance: the roster picker, recital and deadline are usable at 1024×768 ~~without the chrome
breaching `responsive.test.mjs`'s ≤33% ceiling~~ **with the sidebar still rendering the rail**; the
sheet is dismissible and re-enterable within a session without losing what was typed; the sidebar
keeps the running rail; **and the reader's page survives the round trip.**

**(deepdive, 2026-09-07 — `deepdives/2026-09-07-p03s01-the-setup-sheet.md`.)**

**(pin — D3's cited evidence for refusing a second rail is wrong, and its conclusion is right
anyway.)** D3 says a second rail *"breaches `responsive.test.mjs`'s ≤33% ceiling"*. That test sums
`menubar.height + toolbar.height` against viewport **height** (`responsive.test.mjs:55`), so a
second *vertical* rail cannot enter it however wide it gets. The conclusion survives on arithmetic
the decision did not cite: `#sidebar { width: 200px }` and two of those at a 360px viewport leaves
nothing for the document. **The acceptance clause is corrected with it** — leaning on an instrument
that cannot see this slice's subject would have produced a criterion that can only report pass.

**(pin — this sheet is the first of its kind, so there is no structure to copy.)** ADR-025, accepted
the same day, made Settings the sixth mode with its items as **sidebar cards**. `#main` is
`display: flex` over `#sidebar` and **`#viewerCol`**, and the column holds the tab strip and the
viewer — so the sheet is a **sibling of `#viewerWrap` inside `#viewerCol`**, shown in its place, and
not an overlay, which D7 reserves for the two synchronised moments. *(The deepdive first recorded
`#main` as the parent and the test caught it; landing in the column is the better placement anyway,
because the sheet takes the document's space and leaves the strip and the sidebar alone.)*

**(pin — a fourth acceptance clause, added because the grill found a live hazard.)** `#viewerWrap`
is **never hidden today**; it is only class-toggled. Hiding and re-showing it is new ground, and
`/pending 372` is exactly the defect that lives there: *"nothing survives pdf.js re-laying the
document out"* — `currentPageNumber` → `resetCurrentPageView` → `scrollIntoView` scrolls willingly.
A save/restore pattern already exists for view switching (`app.js:2525`, `:2567`). **So the round
trip is an acceptance clause rather than a hope**, and it is asserted at tier 3, because only a real
browser lays a document out.

Tasks:
- T01 — `#ceremonySheet` as a third child of `#main`; the convene form moves into it, markup only.
- T02 — show and dismiss wired to the Ceremony mode, with the reader's page surviving the trip.
- T03 — leaving the Ceremony mode dismisses the sheet; the sidebar keeps the rail.
- T04 — tests: tier 2 for the structure and for typed values surviving a dismiss; tier 3 for the
  reader's page and for 1024×768.

#### P03.S02 — the draft survives closing Nib *(done 2026-09-07, v1.128.24)*
Scope: the roster, recital and deadline persist locally before `convene` writes anything signed.
Refs: D4.
Acceptance: closing and reopening Nib restores all three; the draft is unsigned and per-machine;
nothing about it reaches the network.

**(deepdive, 2026-09-07 — `deepdives/2026-09-07-p03s02-where-the-draft-lives.md`. It answers the
plan's standing caveat, and one candidate is eliminated outright rather than on preference.)**

**`localStorage` cannot meet the exit criterion at all.** It is keyed by ORIGIN, and
`cmd/nib/main.go:94` binds `127.0.0.1:0` — *"a random port by design"*. A new port is a new origin is
an empty store, so a draft kept there is gone on the restart the clause is about. A slice built on it
would have passed every test that did not restart the process. (`grep -c localStorage web/app.js` →
**0**: it would be a new mechanism as well as an unworkable one.)

**The vault is where this app already keeps per-machine state** — `handleSettings` persists
appearance, card hue, the update preference and recent highlight colours there. D29 says key material
must be *in* the vault, not that nothing else may be.

**And the privacy seat's own question settles the remaining choice.** Its question is always residue,
and an abandoned draft leaves *who the user was about to transact with and what they were about to
agree*. Under `~/nib/` that is plaintext beside the documents; in the vault it is encrypted at rest.

**A dedicated store, not a `Settings` field**: `Settings` is read back through `/api/status`, which
the client polls, so a form's contents there would ship on every poll. The vault already holds
ceremony data in dedicated stores with their own doors, and a draft follows that shape with one
difference — **a single slot, because it has no ceremony id yet**, which is also what gives S03 one
door to clear.

**It does not interact with the mirror**, which the caveat also asked: `convene` writes the record
and the mirror at the moment the draft is consumed, and a draft has no id to collide on.

Tasks:
- T01 — a single-slot ceremony-draft store in the vault, with its own read, write and clear doors.
- T02 — routes to save and load it, and the client saving on change and restoring on open.
- T03 — tests, each probed red: a draft survives a restart; an empty draft is not stored; the
  roster, recital and deadline all round-trip.

#### P03.S03 — the draft is consumed exactly once *(done 2026-09-08, v1.128.44)*
Scope: a successful `convene` clears the draft and a failed one does not. Refs: D4.
Acceptance: convening clears it; a refused convene leaves it intact and re-enterable; a second
convene cannot reuse a consumed draft.
Tasks: *(written at slice-grill time, 2026-09-08, after tracing both halves of the draft's life)*
1. T01 — `handleCeremonyConvene` calls `clearCeremonyDraft` after its LAST failure path
   (`pinCeremonyRoster`), so a refused convene leaves the draft intact by construction rather than
   by ordering luck. Best-effort with a log, exactly as `WriteMe` two lines above it is: the
   ceremony IS convened by then, and failing the request over a draft that would not clear would
   report a failure for something that succeeded.
2. T02 — **the client resets the form too, and the scope sentence does not say so.** After a
   successful convene the client calls `showCeremonyForm(null)`, which HIDES the sheet without
   clearing it — so the consumed draft's values stay in the fields, `#ceremonyConveneForm`'s
   `change` listener re-saves them on any keystroke (resurrecting a consumed draft), and reopening
   shows stale values because `restoreCeremonyDraft` finds nothing and returns early. The third
   acceptance clause is false without this half.
3. T03 — tests, each probed red: a successful convene leaves the store empty; a REFUSED convene
   leaves it byte-identical; the form is empty after a successful convene, so a stray change event
   cannot re-persist what was consumed.
4. T04 — seam inventory rows for the consume and its failure.

**Divergence from the task list, recorded rather than absorbed (2026-09-08).** One test outside
T01–T04: `TestTheDraftIsConsumedAfterTheLastRefusal`, a source-level ordering guard. It exists
because a probe showed the **behavioural** test could not see the property it was written for —
"a refused convene leaves the draft intact" is an ordering claim, and the refusal that test drives
(an empty intent) is rejected EARLY, so moving the consume into the middle of the handler left it
green. A late refusal is not reachable from a test: `pinCeremonyRoster` fails only on a vault error.

**And the scope sentence was satisfiable and incomplete, which is the slice's real finding.**
*"A successful `convene` clears the draft and a failed one does not"* reads as server-only. After a
successful convene the client calls `showCeremonyForm(null)`, which HIDES the sheet without clearing
it — so the consumed draft's values stay in the fields, `#ceremonyConveneForm`'s change listener
re-saves them on the next keystroke (resurrecting a consumed draft), and reopening shows them as
though restored because `restoreCeremonyDraft` finds nothing and returns early. The third acceptance
clause — *"a second convene cannot reuse a consumed draft"* — is false with the server half working
perfectly.

#### P03.S04 — leaving the sheet for the document, and coming back *(done 2026-09-08, v1.128.45)*
Scope: ~~signature-block placement leaves the sheet for the page and comes back with what was
typed.~~ **the setup sheet can be stepped out of to the document and re-entered without being
rebuilt.** Refs: D3.
Acceptance: leaving for the page and returning preserves the roster, recital and deadline; the sheet
is re-entered rather than rebuilt.

**(deepdive, 2026-09-08 — `deepdives/2026-09-08-p03s04-leaving-the-sheet-for-the-page.md`.)**

**(reality-drift pin — D3's last clause names an act that does not exist in a ceremony, and the
acceptance clauses survive the correction untouched.)** D3 says *"Placing signature blocks leaves the
sheet for the page, where that panel already lives"*, and the scope sentence above was written from
it. **In a ceremony nobody places a signature block.** `PlacementFor` is the one door onto "where
does this party's block go" (`internal/p2p/cosign.go:107-119`) and inside a ceremony it is
`ceremonyPlacement`, which puts each party's block *"on the signature page their ROSTER POSITION
allocates, at that page's own index"* (`cosign.go:121-146`) — on pages `PrepareCeremonyDocument`
appends for exactly that purpose (`internal/p2p/sigpages.go:199-224`). Three named searches confirm
nothing else feeds it, and each is stated as it actually ran, because the search is the evidence:
`conveneRequest` carries roster, intent, expires and `convenerSigns` and no placement
(`internal/server/convene.go:40-52`); `grep -rniE 'flag|marker|widget' internal/ceremony/*.go`
returns **28 hits, 18 of them outside tests**, and every one is the roster's `Signs` flag, the `me`
file marker inside a ceremony directory, or the *signature* widget annot — never a signing flag; and
`grep -rniE 'signingflag|signflag|/api/flags|collectFlags|overlayField' internal/ceremony/*.go
internal/p2p/*.go` returns **zero**, which is the panel's own objects looked for by name. On the
client `embedFlags` has three callers, all on save-and-flatten paths, and none is the convene path.

*(The first version of this paragraph said that first search "returns only the signature widget annot
and a `me` file". It returns 28 things. The conclusion held; the claim about the search did not, and
a search quoted wrongly is not evidence.)*

**What the panel D3 points at actually is.** `#flags` opens with its own sentence — *"Get a Sign /
Date / Initial flag onto every blank the other person must fill, then email them the file"*
(`web/index.html:163`). It is the prepare-and-email product. A convener sent there during setup would
place flags **no ceremony party is ever asked to fill**, and would then receive auto-generated
signature pages she did not ask for. So the excursion is built, and its destination is not that
panel.

**The conclusion D3 was reaching for survives, and it is what the acceptance clauses already say.**
Neither clause mentions a flag or a block: they say *leaving for the page and returning* preserves
what was typed, and that the sheet is re-entered rather than rebuilt. The sheet stands in place of
the document (`showCeremonySheet`, `web/app.js`), so **any** setup work that needs the
document needs this trip — reading the lease to write the recital, checking the file is the right
one, and finishing markup **before** convening, which `internal/ceremony/convene.go:296-303` makes
irreversible: *"Nothing may append after this line: page count and page content are both inside
ContentDigest."* A pin rather than a strike-and-supersede, because the decision's conclusion and both
acceptance clauses stand; it is the cited act that was wrong, which is the same shape as S01's pin.

**(pin — the way back cannot live in the sidebar, and that was measured rather than preferred.)**
`#sidebar.collapsed { display: none }` (`web/style.css:356`) and a crossing listener collapses it
automatically below 899px (`sidebarNarrow`, `web/app.js`). So a return control in the Flags or Ceremony panel
disappears when the user narrows the window mid-excursion, leaving a half-filled ceremony in memory
with nothing on screen saying so. It goes **inside `#viewerWrap`** instead — the one surface that is
present by definition while parked, since parked *is* `#viewerWrap` showing.

**(pin — three things the grill proposed and the code refused.)** An auto-park on arming a marker
fires on the **toggle-off** click too (the `.markers button` handler passes `null` when the lit button is
clicked again), so a user putting a tool away would be ejected from the form. A `view.pdfDocument`
guard on the step-out would be a **fourth** implementation of "may flags be placed now" beside the `.markers button`
handler's `view.pdfDocument` toast, `reflectSignControls`'s disabled sweep and `flagsEditable()` — ADR-009's named failure — and with no flags button
there is nothing to guard: stepping out with no document shows `#empty`'s *"Open a PDF to begin"*,
which outlasts a 2.5-second toast. And `resumeCeremonySheet` does **not** write the sheet itself:
`showCeremonyForm` is today the only writer of the sheet↔form pair, and a
second one desynchronises them — reachable in six clicks, ending with a visible sheet whose only
child is a hidden form.

Tasks: *(written at slice-grill time, 2026-09-08, after the deepdive and two attack passes)*
1. T01 — `ceremonySetupParked` plus `parkCeremonySheet` / `resumeCeremonySheet`, the resume routed
   through the existing `showCeremonyForm('convene')` so no second writer of the sheet↔form pair
   exists. `showCeremonyForm` clears the park and reflects it in the same breath, so the flag and
   the control it drives cannot disagree.
2. T02 — the two controls: `#cerSeeDoc` in the sheet HEAD beside Close (outside `<form>`, so a
   defaulted `type="submit"` that would convene the ceremony is structurally impossible), and
   `#cerSetupBar` inside `#viewerWrap` carrying the sentence and `#cerBackToSetup`.
3. T03 — `#ceremonyConveneBtn` resumes instead of rebuilding while parked, so the two ways back are
   one door; focus moves after the visibility flips, not before, and each leg is announced through
   the existing `role="status"` toast.
4. T04 — tests, each probed red. Tier 2 asserts the round trip makes **no** `/api/peers` and **no**
   `/api/ceremony/draft` request, which is what "re-entered rather than rebuilt" means as a
   behaviour rather than as a description; tier 3 drives it in a browser and collapses the sidebar
   mid-excursion.
5. T05 — seam inventory rows for the trip.

**Divergence from the task list, recorded rather than absorbed (2026-09-08).** Two additions, both
from review, both outside T01–T05.

**The convene POST is now PINNED to the document the sheet was opened on, and this slice is what
made that necessary.** `/api/ceremony/convene` has been in `pinning.test.mjs`'s MUTATING inventory
since P07.S02a, with a comment naming this exact defect — *"an unpinned convene would commit a
ceremony record into whichever tab the user switched to while it ran"* — and it was unpinned anyway,
because `scanUnpinned` finds mutating calls *"preceded by an `await` in its own function"* and in
`conveneFromPanel` the convene POST **is** the first await. The pause that lets the document change
is not an await: it is the user filling in a form. `#tabstrip` is a sibling of the sheet and
`showCeremonySheet` never touches it, so the switcher was always live behind the sheet — but before
this slice the only ways out read as abandonment, and `parkCeremonySheet` now invites the user onto
the page with the strip right there. ADR-001 in its own words: *no operation acts on a document it
did not capture at its start.* Driven at tier 3 by closing the document mid-excursion, where a
pinned convene is refused **409** and an unpinned one answers **404** — ADR-004's two statuses doing
the discriminating.

**The parked-setup bar outranks the page overlays (`z-index: 11`, not `#signBanner`'s 6).**
`.viewerContainer` is positioned with `z-index: auto` and creates no stacking context, so `.ovl` (8),
its variants (9) and `.shapemark` (10) paint in `#viewerWrap`'s context and are pointer-interactive.
A stamp near the bottom-left covered "Back to setup" and took the click. `#signBanner` can afford 6
because its button is a convenience; this bar carries the only route back.

### P04 — Scale and repair
**Goal.** The rail at a full roster, and the operational steps the design has never had.

**Exit criteria.** The worklist threshold is a stated number and the rail is asserted at sizes
either side of it; invitations can be reissued from the rail; the no-correction rule is stated
before the first hop.

### P05 — The pre-hop party learns, and can leave
**Goal.** Close the two ends D14 left open: a party who has accepted and not yet signed can find
out the proceeding ended, and can decide to stop taking part.

**Exit criteria.** A declined ceremony reaches a party who never signed, and is refused when its
anchor does not match; leaving stops the arm and survives a restart; ~~neither path can mint or
consume an attestation~~ → **leaving neither mints nor consumes an attestation, and the end-state
path consumes only one it has verified.**

**(pin, 2026-09-07 — the fourth clause was falsified by this phase's own approved design, and it is
corrected rather than quietly credited.)** *"Neither path can mint or consume an attestation"* was
written at phase-firming, before the deepdive established that the end state **is** a `Termination`
the convener already mints and delivers. S03 exists to CONSUME one. The clause's real content was
always about leaving — that a local act must not mint an attestation, and must not act on somebody
else's — and that half is intact and tested. The corrected form keeps both protections and names the
verification the end-state path owes. **Put to Dan and RATIFIED 2026-09-07 via
`/discuss`** — flagged because it is a criterion I wrote and then corrected, which is the shape exit
criteria exist to prevent, so it does not stand on my own say-so.

**Sequenced AFTER P04 rather than before it**, because P04 is the rail at a full roster and this
phase changes what the rail has to say. Its slices are sketches until phase-open.

#### P05.S01 — leaving a ceremony *(done 2026-09-07, v1.128.10)*
Scope: a control that prunes this machine's stored invitation for one ceremony, so the sweep stops
arming for it. Refs: D17.
Acceptance: after leaving, a sweep does not arm for that ceremony and does not after a restart;
nothing is sent and no termination is written.

#### P05.S02 — the end state is verifiable on the invitation *(done 2026-09-07, v1.128.18)*
Scope: extract `Termination.Verify`'s anchor so an invitation can supply it, through one door.
Refs: D16.
Acceptance: a termination verifies against an invitation exactly where it verifies against the
record, and is refused on a mismatched roster commitment or a non-convener signer.

#### P05.S03 — the declined end state reaches a party who never signed *(PARTLY done 2026-09-07, v1.128.19; the arm BACKED OUT at v1.128.20 — `/pending 380`)*

**(reality-drift pin, 2026-09-07, and it corrects a marker I wrote.)** This slice was marked done at
v1.128.19 having passed tiers 0–3 and 6. **Tier 4d had not been run, and it is a required-run gate**
— `CONTRIBUTING.md` says *"run them all after a change"*. Run at the phase close, it failed
deterministically: *"[quic] a party is not reported delivered after the recovery run"*, twice, and
passed with the sweep admission reverted.

**What shipped and stands:** `checkDeliveredPayload` verifies a convener's end state against the
**invitation** for a machine holding no record — P05.S02's door, its first caller, four mutations
red. **What was backed out:** the `rearmDeliveries` admission that would let such a machine be
armed to receive one. Three hypotheses were tried and none held; the evidence and the dead ends are
in `/pending 380`, and a guard fails if the admission reappears without `-n 4` being re-run.

**So the receiving half is built and the arriving half is not**, and this marker says so rather than
claiming a slice that is two thirds of itself.

**(deepdive, 2026-09-07 — `deepdives/2026-09-07-p05s03-the-slot-contention.md`. The arriving half is
OUT OF THIS PLAN'S SCOPE, and that is the plan's own words rather than a convenience.)** The
instrumentation built at `/pending 381` produced the mechanism on its first run: the recipient's
single delivery slot was armed for a **different ceremony** than the convener arrived with. A
delivery rendezvous is keyed `(ceremony, hop)` and its listener pins ONE peer, so two ceremonies are
two peers and two rendezvous — **no single arm can serve both**, and no ordering or yielding rule
changes that.

The obvious answer, a ceremony-keyed slot map, was **already refused** by `armKind`'s own doc — and
its premise is narrower than its conclusion: *"a machine needs at most one of each"* is true per
CEREMONY and false per MACHINE, because two conveners running two rounds coordinate with each other
not at all. Re-opening it is a decision, not a correction.

And widening it is what *Out of scope* names: *"the serial hub and its one-pinned-peer tripwire …
widening either needs a fresh security review."* N concurrent armed listeners **is** that widening.

**So this slice closes at its scope boundary rather than at its acceptance clause**, and P05's first
exit criterion goes to Dan as a parked amendment rather than being re-worded to fit what shipped.
Scope: the receiving half — a pre-hop party is reachable by the round that already walks them, and
acts on what it verifies. Refs: D16.
Acceptance: a declined ceremony delivered to a party holding no record closes their arm; a planted
or mismatched object does not.

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
- **P05.S03's reachability is the one thing in that phase not settled by reading.** The convener
  dials the delivery rendezvous and D14's arm listens on the hop rendezvous; whether the right fix
  is to arm the delivery slot for a pre-hop party or to carry the end state on the hop rendezvous
  is a question about the tier ladder, and it should be dived at phase-open rather than decided
  here.
- **The setup draft is a new persisted artifact this plan did not author.** Where it lives, whether
  it needs the vault, and how it interacts with the mirror `convene` later writes should be dived
  before P03 rather than decided inside it.

## Seam inventory

`~/.claude/projects/-home-dan-repos-nib/memory/instruments/ceremony-wizard.md` — 19 rows (7 paths,
6 seams, 6 gap-downs), written against this plan before any code. One hot-path row (`next` on open),
one row coarser than its own clause (the worklist threshold), and two
`diagnostic, no standing reader` entries, one of which is a permanent structural zero: nib does not
deliver invitations and cannot observe delivery.
