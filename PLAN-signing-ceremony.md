# PLAN — The Signing Ceremony

Planning arc started 2026-08-15 (Stage 1 seed, from a design conversation that
worked the problem from first principles — portless rendezvous, an IPv4 path, and
a Nib-specific pairing design — rather than a cold brief. Every claim made about
nib's current behaviour in that conversation was read at the cited line before it
was written down here).
Amended 2026-08-16 (transport-fallback pass, Dan's instruction: TCP retained as a
peer transport rather than deleted, and router port mapping added to the ladder —
D14 and D15 added, D7/D8/D9 pinned, P02 and P05 amended, caveats 6–8 added. Stage 2
has not yet run).
Amended 2026-08-16 (connection-algorithm pass, Dan's instruction: the four
specifications the ladder was missing — clocks, symmetry and glare, channel loss,
failure diagnosis — settled as D16–D19, with D8/D11/D13 pinned, P01.S05, P05 and P06
amended and caveat 9 added. Stage 2 still has not run).
Amended 2026-08-16 (connect deadline raised to 300 s and role-conflict handling fixed,
both Dan's calls: D16's deadline superseded with backoff, candidate-expiry and
mapping-refresh consequences adopted alongside it; D17's roles chosen pre-connection
with a conflict stopping the ceremony for a retry rather than resolving itself.
Stage 2 still has not run).
Reviewed 2026-08-17 (**plan-review pass** — structural gate passed; 13 findings, all
dispositioned **adopt**: 3 critical (the verification string's grindability, the
rendezvous key's authentication and blast radius, and caveat 7 binding in a gate that did
not enforce it), 7 warnings, 3 info. Pins written below at D4, D6, D12, D16, D1, caveats
4 and 7, and P01–P06. No decision was struck or rewritten. Stage 2 still has not run.)
Amended 2026-08-18 (**UX pass**, Dan's instruction: the ceremony must run with more than two
parties, survive interruption and resume, ask nothing to be exchanged once connected, and make
signing out of order impossible. All four are answered by one artifact the plan did not have —
a **Ceremony Record**, written before anyone connects, from which roster, order, position and
deadline are all *read* rather than chosen. Dan took **option A on all four forks**: the
invitation carries a full-strength secret; parties are present in one sitting rather than
continuously connected; a convener may be non-signing; signature blocks get a fresh page every
six. **D20–D25 and L3 added; D2, D4, D6, D13, D16, D17, D18 amended; D12 withdrawn on Dan's
instruction; the group-ceremony out-of-scope entry struck; P01, P05 and P06 amended and P07
added; caveat 5 narrowed and caveats 10–11 written.** Stage 2 has still not run.)
Reviewed 2026-08-23 (**plan-review pass** over P07's firmed slice list — structural gate passed with one
warning; 14 seats, 10 criticals, 51 warnings, 24 info, all dispositioned. **Verdict: the firmed slice list
failed and was restructured** — eleven slices, twenty-two numbered criteria, S08 and D25's page allocation
moved ahead of the convene route, and a new S02b for invitation consumption. Three decisions amended on
Dan's call: D20 gains a per-party `capacity` (FormatVersion 3 → 4), D22 re-bases `AcceptedPeer` onto the
previous *signing* roster entry, D23 keeps signing order fixed and records why. The pass's own headline is
that **`docHash` is dead on the real path and its guard signs invisibly** — measured, not argued. No slice
had been built when this ran.)
Amended 2026-08-18 (**holistic pass** — the second of the day, Dan's instruction: the whole plan
read end to end against the code it names, after the UX pass had rebuilt its spine. Twenty-one
items: six read at the line, eight unwritten lifecycle states, four judgment calls, three that
were Dan's. **The through-line: the plan reasoned carefully about the channel and barely at all
about the surfaces at either end of it** — twenty-five decisions covering pairing, transport,
rendezvous, NAT, clocks, glare, ordering and resumption, and not one covering what the consent
screen shows a fourth signer, what the appended trust page says when there are five people, what
a decline does to a proceeding, or what a ceremony leaves on disk. **D26–D31 added; D22 and D24
amended; P01, P02, P05, P06 and P07 amended; the CLI scoped out; and the Stage 2 grill made P01's
entry condition rather than a phrase repeated in six datelines.** Three calls were taken on Dan's
behalf under the ASK ladder and are marked reversible where they sit: the pairing default (D31),
the CLI (out of scope), and the grill's scheduling (bookkeeping).)
**Grilled 2026-08-18 — Stage 2 of the arc, at last, on Dan's instruction. Verdict: amended.**
The pairing design, the ordering law and the resumption model all survived unchanged. One
structural crack: caveat 7 requires QUIC, the DHT and the mapping probe to share **one UDP
socket**, and no phase named the component that separates them — see the caveat. **Caveat 10 was
discharged by measurement rather than argument** (D20). Six changes landed: the demultiplexer
named in P02 and in library selection; **P07 built before P06**; D26's harness narrowed to the
HTTP API with its ceiling written down; caveat 10 struck; D29's freeze extended to the attachments
route; and this document's own framing corrected — after D21/D31 the spoken name describes the
*fallback*, not the default path. Seam inventory at
`<project-memory>/instruments/ceremony.md` (11 paths / 12 seams / 9 gap-downs, no hot-path rows).
Stages 3–6 of the arc still have not run.
**Stages 3–6 run 2026-08-18 (unattended sweep, Dan's instruction: "like a runpending, don't stop for
anything but the most serious issues").** Subsystem rounds, flat-spot passes, standards alignment and
the dimension reviews, dispositioned on the best-and-most-correct test and landed without gates.
**D32–D34 added; five pins written** — the roster commitment's preimage axes (D20), the nested-clock
reservation (D16), the invitation as unsigned root of trust (D21), negative fixtures for all three
law guards (Repo laws), and the seam inventory's graduation pass (P07). **The sweep's own
through-line: the plan specified mechanisms and left their *numbers* and their *versions* unstated**
— three security bounds that no criterion could have failed, and four interoperating formats with no
version between two independently-updated Nibs. Stage 7 (`/plan-review`) last ran 2026-08-17, before
D20–D34 existed, and is owed again.
Reviewed 2026-08-18 (**plan-review pass, the second** — structural gate passed; the plan's machinery
is intact, including on a withdrawn decision. 13 findings: **2 critical**, 8 warnings, 3 info. **All
eight warnings dispositioned adopt by Dan**; pins written at D6, D8, D11, D22, D29, D33, P01 and P04,
plus a citation rule in Bookkeeping. **Both criticals are consequence findings where two decisions
settled on different days meet** — `docHash` defined circularly (D20), and invitation pins dropped at
the end state while delivery runs after it (D29 × D28 × D22) — and are **not yet dispositioned**.
Report: `<project-memory>/plan-reviews/2026-08-18-2.md`.)
Reviewed 2026-08-18 (**plan-review pass, the third** — Dan's remit: *the seams and the gaps*.
Structural gate passed. 10 findings: **1 critical**, 6 warnings, 3 info — **and the two criticals
carried from the second pass. Dan dispositioned ALL of it adopt.** The pass's own finding is that
after three grills and two reviews there was nothing new *inside* the decisions: **everything of
substance sat at a boundary** — between the ceremony and code the sibling plan already shipped
(the pinned document id versus ADR-001's per-process counter), between the invitation's roster and
the record's, and between an end state and the delivery round that runs after it. Pins written at
D4, D20, D21, D22, D27, D29 ×3, D32 and caveat 7; new criteria at P01.S06, P01.S07 and P07.
Report: `<project-memory>/plan-reviews/2026-08-18-3.md`.)
Reviewed 2026-08-18 (**plan-review pass, the fourth** — same remit, and it found what the third had
described from the outside: **the co-signing channel binding assumes the wire peer is the party who
signed before you, and D22's hub makes it the carrier.** D22 had called `len(ats) != 1` "the whole
two-party assumption in the transport"; it is one of three, and the other two refuse at every hop
under a non-signing convener while `crossBind` reports `Matched: false` on every signature. 1
critical, 2 warnings, 3 info — **all adopted by Dan.** Pins at D2, D20, D22, D27 and D29 ×2; new
criteria at P07. **Amending D2's "retained unchanged" claim is owed through `/discuss`.**
Report: `<project-memory>/plan-reviews/2026-08-18-4.md`.)
**Amended 2026-08-18 (the collapse to one pairing path — Dan's decision, after the 256-versus-66
analysis).** Pinning by six-word name is **retired**; the invitation is the only way to pair. The
four-word verification string **survives with a different job**: it confirms that the invitation
arrived intact, which is a claim about the *delivery channel* rather than about the pin's length.
**The correction that forced it:** an earlier pin held that tampering with an invitation buys
"denial, not substitution". That assumed an attacker altering one field while the rest of the trust
chain stayed honest. **An attacker who controls the delivery channel replaces the whole
invitation** — their fingerprint as convener, their roster, their signed record — and every check
passes, because both halves of every comparison are theirs. D3, D4, D21 and D31 amended; caveat 5
struck and caveat 4 relaxed; P01.S03 struck and P01.S04/S05 amended.
**Amended 2026-08-18 (the seven gaps from the disruption walk-through).** **D35 added** — a
disagreeing clock is diagnosed from the certificate the failed handshake already holds, which is the
grill's finding: the first instinct was to exchange timestamps on the channel, and the channel is
precisely what has failed. **Five pins** at D21, D24, D29 and D34 close the rest: atomic persist,
disk-full honesty, no second ceremony on one document, a re-issued invitation, and self-healing
widened from *corrupt* to *unreadable or absent*. Criteria at P05 and P07.
Reviewed 2026-08-18 (**plan-review pass, the sixth — coherence.** Warranted rather than routine: the
collapse to one pairing path touched decisions written across four days, and **retirement is where
dangling references live.** No criticals; the mechanics held — 38 pins, no pin-versus-pin conflict.
**Four warnings, all the same defect: the collapse did not finish.** D19 still said *four* causes
while D35 and P05 said five; the opening paragraph still sold two pairing paths, having drifted
*twice*; D4's heading and a mid-decision blockquote still carried the retired conditional; and D18
described behaviour on a path that no longer exists. **All four adopted and corrected.** **P07 split
on Dan's instruction** — it had reached 30 exit criteria against a slice sketch, so the model stays
at P07 and lifecycle and delivery become **P08**; build order is now
P00 → P01 → P02 → P03 → P04 → P05 → P07 → P08 → P06. Report:
`<project-memory>/plan-reviews/2026-08-18-6.md`.)
Amended 2026-08-19 (**the three owed decision-text amendments, via /discuss — Dan.** A plan-review
pass marks a spot and never rewrites a decision, so three pins had been carrying an unpaid debt
against the decisions' own words. **D33's** closing *"all four are tunable"* is struck for the
adopted split — `N` and the punch ceiling are **law**, the deadline and roster maxima are tunable —
because two decisions settled a day apart gave a builder opposite instructions about where a
security bound lives. **D6's** *"also rejected: a per-ceremony key from a shared secret"* is struck,
D21 having built exactly that; and the *first* rejected clause, which the pin did not name, has its
premise marked stale for the same reason — nobody holds 66 bits since D31, so a record carrying the
signer's public key is verifiable against a pinned fingerprint. That re-opens a design question and
it is **deliberately left open and filed against P04** rather than settled in an amendment pass:
every invited party holds the invitation secret, so within the roster one party can publish
candidates as another. **D2's** *"retained unchanged"* is corrected in **three** places — the title,
the untouched-items enumeration, and the UX pin's own closing sentence, which repeated the claim a
day later — because D22's hub is precisely what breaks the attestation channel binding. What is
genuinely untouched is now stated positively, so the amendment reads as a correction rather than as
D2 being in doubt.)
Amended 2026-08-30 (**the four decision-level items the seventh plan-review parked, settled by Dan
through `/discuss` — two rounds, eight decisions.** The external pass owed by `/pending 307` ran
first, cold against a sealed brief; its verdict was that **four P08 criteria that read as met were
not**, and its through-line is one sentence: *a criterion whose only enforcement point is the phase
close is not a criterion during the phase.* Two of its criticals were fixed inside the review's own
sweep (the sticky failure surface had no reader, and tier 2 had been red since the first P08 slice);
eight became `/pending` 319–326, five of which shipped.
**What is settled here, all four on Dan's call and none of them striking a decision:**
**C12/D29** — the ceremonies listing moves off `requireUnlocked` and *locked* becomes a fifth
degradation class, because the vault lock protects nothing there: the mirror is unsealed by D29's
own design. **C11/D28/S06** — the end state gets a small **unattested local receipt** that survives
the prune, reopening only S06's prune of the three decisions that composed to make an end state
unrecordable; `Embed` and D28 are untouched. **S04/C06** — the termination object is **convener-
signed and transmitted**, a second artifact rather than the same one, because an unattested claim
about a proceeding arriving over the wire is a denial-of-service on a live ceremony; D28 stands
unstruck, since the end state was never a `Record` field. **D33/S05** — the delivery arm's DHT cost
is bounded, ADR-011 having appeared nowhere in P08. **C17's enforcement point moved to the slice**
(v1.117.272, `/pending 322`) with a gate that can see an absent row, which the phase-close pass
structurally cannot.
**The asymmetry written down rather than glossed:** only *declined* and *completed* can be attested,
because *expired* and *abandoned* have no convener to sign them — those two are derived locally and
identically by every machine from the record's own `Expires` plus S06's grace. A reader who assumes
all four are attested builds the wrong verifier.)
SME packs: **crypto (core tier)** — `go.mod` declares `filippo.io/age`,
`golang.org/x/crypto`, `edwards25519`, `hpke`, `go-pkcs12`, `digitorus/pdfsign`
(inferred, trigger 1); the consensus tier does not fire — ~~two~~ **N (2026-08-18)** sequential
single signers, no aggregate or threshold dependency **— multi-party (D22) is a relay, not a
quorum, so the tier's trigger still does not fire**. **verification** — declared
by the sibling plan in this repo (`PLAN.md:17`, since retired), corroborated by
`mdpdf/coverage.go`'s `Unsupported()` capability oracle. Both declare `plan` in
`objects`.

**Where this plan and the original brief differ, the plan wins.**

**This is the repo's only plan.** ~~Sibling plan: this repo currently carries a second,
unrelated feature plan — `PLAN.md`, "Multiple open documents in Nib" … `/createcode` must be
told which plan it is walking.~~ **(discharged 2026-08-19 — Dan.)** `PLAN.md` closed
2026-08-17 at v1.108.4 (all seven phases; P07's ledger 6 met / 0 not met / 0 not exercised)
and was **retired per STANDARDS §15.6 on 2026-08-19**: audited for reasoning that lived
nowhere else, which became **ADR-004** (document id on the wire), **ADR-005** (the
open-document cap's measured byte figure) and **ADR-006** (the hand-off credential, and the
stronger mechanism refused); code comments citing it for a measurement or a pin re-pointed at
those ADRs; the file removed. The split D1 called deliberate has ended, so `/createcode` needs
no disambiguation — there is one plan and this is it.

This plan covers **one feature project inside the existing `nib` repo**: replacing
the current **Collaboration** process with a **Signing Ceremony** — ~~two people who
have never connected before, both running Nib, co-sign a document~~ **two or more people who
have never connected before, all running Nib, co-sign one document in a proceeding they can
interrupt and resume (2026-08-18, D20–D25)** — with no port
forwarding, no VPN, no rendezvous server, and ~~one short spoken name in place of a
64-character fingerprint~~ ~~one artifact in place of a 64-character fingerprint and a typed
address: an invitation on the default path, or one short spoken name on the fallback~~ **one
invitation in place of a 64-character fingerprint and a typed address (2026-08-18, D31 collapsed —
there is one pairing path; the six-word name is a spoken identity and pins nothing)**. It is not a
plan for nib as a whole.

**(plan-review pin: this paragraph has now drifted twice — 2026-08-18, adopted by Dan.)** It was
corrected at the Stage 2 grill for describing the pre-D21 design, and the correction itself went
stale within hours when Dan collapsed the two paths. It is the **first prose in the document**,
which is why it drifts: every amendment pass edits the decision it is changing and few think to
re-read the opening. *What is true now, in one clause:* the invitation is the only way to pair, and
the four-word verification string confirms that the invitation arrived intact (D4's second
supersession).

---

## What is being replaced

Nib's co-signing cryptography is built and sound. What is being replaced is
everything *around* it — how two people identify each other and how their machines
meet on the network.

Today, both halves are manual:

- **Identity** is a 64-hex-character SPKI fingerprint the user reads out and the
  other pastes (`internal/server/peers.go:60`, `:118`).
- **Connectivity** is a host:port the user types, reachable only by port-forward or
  a shared VPN (`web/index.html:1210`, `:1212`), dialled over TCP
  (`internal/p2p/session.go:44`, `:56`).

Both are being removed from the user's path. Neither is being made less rigorous.

## Repo laws

~~Two~~ **Three (2026-08-18)** laws govern every decision below. They are stated here, not
derived later, because the first two exist to make an untrusted discovery layer safe to depend
on, and the third exists to make a multi-party proceeding safe to interrupt.

**L1 — The rendezvous path affects reachability only, never identity.** Nothing
learned from the DHT, from multicast, or from any other network source may
influence *which* peer is accepted. Acceptance is decided solely by the pinned
fingerprint (~~`internal/p2p/transport.go:83`~~ **the `subtle.ConstantTimeCompare(fp, pinnedSPKI)`
in `verifyPinnedPeer`, `internal/p2p/transport.go` — corrected 2026-08-20 at P04.S05**) and the
verification string.

*Cited by SYMBOL and not by line, and the reason is a measurement.* Line 83 was the closing brace
of the `tls.Config` literal, so the law cited punctuation. The correction first read
`transport.go:110-111` — and **that was stale before the same commit finished**, because adding the
clock seam a few lines above pushed the comparison to 136. A line number in a law is a citation
that rots on every edit above it; a symbol name does not. A
rendezvous layer that lies can waste time; it can never substitute a signer.

**L2 — No silent downgrade.** Any path that would complete a ceremony at less than
full assurance must fail closed rather than proceed quietly. A shortened name that
lowers the pin's effective strength without the user knowing is exactly the defect
this law exists to forbid.

**L3 — No contribution out of roster order. *(added 2026-08-18, D23)*** A signature may be
contributed only by the party the Ceremony Record (D20) names at the current position, and only
onto a document carrying exactly the record's preceding signers, in order, each one valid and
cross-bound. The check lives in Go and refuses by name; a UI that merely declines to offer the
button satisfies nothing. L3 exists because the ordering of a multi-party signing is a property
of the document, and a property nothing enforces is a convention.

All three laws get guard tests, named at slice-grill time.

**(Stage 6 pin, verification pack V1, 2026-08-18 — a guard with no negative fixture is a guard
verified by its own absence.)** "Gets a guard test" is half a requirement. **Each of L1, L2 and L3
ships a negative fixture that plants a violation of *that law specifically*, and each earns a row in
`docs/red-proofs.md`** — the ledger that already backs `CONTRIBUTING.md`'s proven-red claim. A shared
fixture will not do: three laws behind one fixture means two can be deleted with it still red, which
is the exact shape V1 was earned by. This repo has already been bitten by the softer version — a
guard satisfied by prose in a doc comment, twice in two sweeps.

---

## Decisions

### D1 — Scope, topology, plan artifact, and license *(settled 2026-08-15 via /discuss, auto-adopted)*

A feature project inside the existing `nib` repo. No new repository, no new Go
module, no new binary. The plan artifact is this file, `PLAN-signing-ceremony.md`,
~~kept beside the sibling `PLAN.md` with a cross-reference in each~~ **now the repo's only
plan (2026-08-19 — the sibling retired per STANDARDS §15.6; see the header)**. License unchanged:
**AGPLv3**. The DHT dependency is MPL-2.0, which is AGPL-compatible (MPL 2.0 §3.3
permits distribution under a Secondary License, and AGPLv3 is named as one) — so
the dependency creates no licensing conflict.

*Why:* a separate module would fragment one product; nib is mature (v1.102.4) with
its own CLAUDE.md and release machinery, and this feature is one surface within it.

### D2 — The existing co-signing cryptography is retained ~~unchanged~~ **unchanged except the attestation's binding target (amended 2026-08-19 via /discuss — Dan; see the D22 pin)** *(settled 2026-08-15 via /discuss, auto-adopted)*

Untouched by this plan: pinned-peer mTLS with SPKI-hash pinning
(`internal/p2p/transport.go:42`), ephemeral per-session leaf certificates so the
document-signing key never reaches the TLS layer (`:119`), ~~attestation channel
binding (`internal/p2p/session.go:205`),~~ and mutual co-signature verification with
the prefix-extension replay bound (`:83`).

**(amended 2026-08-19 via /discuss — Dan: the channel binding is struck from this list, because
D22's hub is what breaks it.)** `coSignExchange` derives three checks from `peerFP`, the
TLS-verified **wire** counterparty, and under a hub every hop's wire peer is the convener — so with
a non-signing convener check 2 refuses at every hop and `crossBind` leaves `Matched` false on every
signature of the finished document. **The re-basing is adopted in the D22 pin** (the attestation
binds to the **record**, not to the wire; `AcceptedPeer` names the roster predecessor), and this
decision no longer claims otherwise. **What remains genuinely untouched, and is the substance of
this decision:** the pinned-peer mTLS with SPKI-hash pinning, the ephemeral per-session leaf, the
prefix-extension replay bound, and the `[NibCoSign:1]` tag. The *cryptography* is retained; what
moves is what the attestation binds **to**.

*Why:* it is built, it is sound, and it is orthogonal to pairing. Re-planning it
would be scope creep against a surface that already survived review.

**(UX pin, 2026-08-18 — one addition, and it is a format change.)** Multi-party (D22) needs
every signature to attest to *the same ceremony*, and today an attestation accepts exactly one
counterparty: `Attestation.AcceptedPeer` is a single hex fingerprint written into the signed
`/Reason` (`internal/p2p/attestation.go:43`, `:91`). Signer 3 attesting only to signer 2 is a
chain of pairwise claims, not a record of one proceeding. **The signed `/Reason` gains a
`[NibRoster:<hash>]` token beside the existing `[SPKI:…]`**, committing to the Ceremony Record
(D20). Everything else in this decision stands untouched: the pinned mTLS, the ephemeral leaf,
~~the channel binding,~~ the prefix-extension replay bound, and the `[NibCoSign:1]` tag whose
absence still means "not one of ours" (`attestation.go:65`). **(amended 2026-08-19 via /discuss —
Dan: the channel binding is struck here too. It was written a day after the enumeration above and
repeated the same claim, so correcting only one list would have left this paragraph — the one a
builder reads last — asserting the opposite.)** The existing parse is unaffected —
`ReadAttestations` reads the first `[SPKI:…]` and ignores what it does not know.

*What was read rather than assumed:* `crossBind` (`internal/p2p/attestation.go:153`) already
cross-binds each accepted peer against **every other signer** on the document, and
`confirmCoSigned` (`internal/p2p/session.go:281`) requires this user's signature and the peer's
while tolerating extras. Both survive N parties unchanged.

**(plan-review pin: "unchanged" holds for a chain and fails for a hub — 2026-08-18, adopted by
Dan.)** The paragraph above is right about the *code* and wrong about the *topology*, which was
chosen in D22 on the same day. `crossBind` survives N parties when each party's wire peer is their
**predecessor signer**; D22 makes it the **carrier**, and with a non-signing convener no signer holds
the accepted fingerprint at all. **This decision's headline is therefore in question:** it retains
"attestation channel binding" **unchanged**, and that binding is precisely what the hub breaks — see
the D22 pin for the three checks and the adopted re-basing. **Amending this decision's
retained-unchanged claim is owed through `/discuss`**; a plan-review pass marks the spot and never
rewrites a decision. The two-party assumption is not in
the artifact model; it is in the live path (D22) and in the UI.

### D3 — The pairing name is derived from the identity fingerprint, not randomly generated *(settled 2026-08-15 via /discuss, auto-adopted)*

At first open Nib derives the user's name from the SHA-256 SPKI fingerprint it
already computes. The name is an *encoding* of the pin, not a label attached to it.

*Why:* it collapses two exchanges into one. A random name would still require the
fingerprint to be exchanged separately for pinning — which is the exact friction
this feature exists to remove. ~~Because the name commits to the key, typing a name
*is* pinning a peer.~~

**(amended 2026-08-18 — Dan, the collapse to one path.)** **The name no longer pins anything.**
Typing a name *was* pinning a peer while name-only pairing existed; with pairing collapsed onto the
invitation (D31), the six-word name is a **display identity** — shown beside a signature, spoken to
confirm "yes, that is me" — and nothing derives a pin from it. The derivation from the fingerprint
(this decision's substance) is **kept**: a name that is a function of the key still means a name
cannot be claimed by someone else, which is what makes it usable as a spoken identity at all.

### D4 — Six words, and the verification string is mandatory ~~always~~ ~~exactly when the pin is short (2026-08-18)~~ **— it confirms the invitation arrived intact, and is required whenever the parties have a voice channel (2026-08-18, second supersession)** *(settled 2026-08-15 via /discuss — Dan; amended twice 2026-08-18 — Dan)*

The name is **six words** from a 2048-word list, encoding ~66 bits of the
fingerprint. After the channel is established both sides display a **four-word
verification string** derived from the session (both identity keys plus the
handshake transcript); one party reads it aloud and the other confirms. **The
ceremony cannot complete until both sides confirm** — there is no skip.

*Why Dan's call, not the process's:* it trades usability against cryptographic
margin on a product whose output carries legal weight. Correctness names the
failure mode; it cannot set risk appetite.

*The mechanism the mandate protects:* six words commit only ~66 bits, so an
adversary who grinds a keypair matching those bits passes the pin check — the
machine cannot tell. But a man-in-the-middle must run two separate handshakes, so
the verification strings on the two screens derive from different inputs and cannot
match. The spoken comparison catches precisely the attack the shortened name opens.
Skippable confirmation would leave the pin silently at 66 bits, which L2 forbids.

**(plan-review pin: the verification string is a 2²² target, not a 2⁴⁴ one — 2026-08-17,
adopted by Dan.)** The paragraph above says a man-in-the-middle "must run two separate
handshakes, so the verification strings … cannot match." **They can be made to match.**
Four words from a 2048-word list is **44 bits**, and the string derives from "both identity
keys plus the handshake transcript" — the identity keys are pinned, **the transcript is
not**. A MITM chooses its own ephemeral contribution on each leg *after* seeing that
victim's, so it can enumerate candidates on the A-leg, enumerate on the B-leg, and search
for a collision **between the two sets**. That is a birthday problem over 44 bits:
**~2²² cheap operations — seconds on one machine**, inside the window where two people are
still saying hello. This is the textbook short-authentication-string failure, and ZRTP and
PGPfone both carry a commitment step for exactly this reason.

*The consequence for this decision's central claim:* **the composed strength is 66 bits,
not 66 + 44.** An adversary who has paid 2⁶⁶ to grind a matching key gets the spoken check
for free. Caveat 5's conditional — "if the verification step is ever weakened, this number
becomes the whole security of the pairing" — is therefore **already true, unweakened**.

*What is required:* the string must be derived only over **committed** values. Each side
sends `H(nonce ‖ contribution)` before either reveals its contribution, and the string is
computed over revealed values checked against those commitments; a peer that reveals after
receiving is rejected before any string exists. With the commitment the attacker must
*guess* (2⁻⁴⁴ per attempt, and a wrong guess is seen by two humans); without it, it
searches.

**What discharges this specifically:** the new P01.S04 acceptance bullet driving an
out-of-order reveal — **not** the three bullets already there, every one of which
(identical words per session, different words across sessions, different words under
identity substitution) is satisfied by the grindable design. That is why this pin adds a
criterion rather than conjoining a clause to an existing one.

*Owed, and Dan's to make, not this pass's:* the mechanism paragraph above is now wrong as
written and wants amending through `/discuss` — a plan-review pass marks the spot and never
rewrites a decision.

**(UX amendment, 2026-08-18 — Dan's call, option A. The mandate is superseded, and the pin is
raised rather than the check relaxed.)** Dan's requirement is that **nothing is exchanged once
connected**. The spoken four words are the plan's only post-connection exchange, and they exist
for exactly one reason: six words commit to ~66 bits, so the machine cannot finish the
authentication and two people have to. Multi-party makes it worse in a way worth naming — with
four parties a spoken check is three comparisons, on three pairs of screens, at three different
moments, and it recurs on every resumption.

**The supersession, stated as a rule rather than as a relaxation:**

> ~~**The verification string is mandatory exactly when the pin is short.** A ceremony paired from
> an **invitation** (D21) pins the counterparty's **full 32-byte fingerprint** and shares a
> 32-byte secret, so the pin is at full strength and the check is displayed as reassurance, not
> as a gate. A ceremony paired from a **spoken six-word name alone** pins ~66 bits, and there the
> string stays **mandatory, with no skip**, exactly as this decision originally required.~~
>
> **(struck 2026-08-18 by the second supersession below — there is one pairing path and one pin
> strength, so there is nothing left to condition on. The rule that replaced it is stated there.)**

*Why this is L2-honest and not an L2 breach:* L2 forbids completing a ceremony at less than full
assurance without the user knowing. The original mandate was the compensating control for a
66-bit pin; option A removes the 66-bit pin from the invited path instead of removing its
compensating control. **Assurance goes up, and the human step disappears as a consequence rather
than as a concession.** The rule above is what keeps that true: the short pin never silently
loses its check, because the check is conditioned on the pin's strength and not on the flow.

*What this does to the plan-review's C1:* **C1's premise is removed on the invited path, not its
finding.** The birthday search C1 describes needs an adversary who has already defeated the pin
by grinding a keypair to 66 bits; against a 256-bit pin no such keypair exists, so there is no
second handshake to collide. **The commitment step is therefore required on the name-only path
and not on the invited one** — and P01.S04's out-of-order-reveal criterion stays, because that
path stays. Recorded this way deliberately: C1 was correct, and it is being answered by making
its precondition unreachable rather than by disagreeing with it.

*What is kept:* the six-word name (D3), unchanged and still derived from the fingerprint. It
remains the human identity — spoken to confirm "yes, that is me", shown beside every signature
in the roster. It stops being the sole carrier of the authentication.

**(second supersession, 2026-08-18 — Dan's decision. The check survives; what it is FOR changes.)**
The rule adopted this morning — *mandatory exactly when the pin is short* — was built on the premise
that the spoken words compensate for a truncated pin. **That premise was wrong about what the
invited path actually rests on.** With pairing collapsed onto the invitation (D31) there is no short
pin left anywhere, and yet the check is more necessary than the conditional rule made it, because
the thing it should have been confirming was never the pin.

> **The verification string confirms that the invitation arrived intact.** It is a claim about the
> **delivery channel**, not about how many bits are pinned. It is **offered on every ceremony** and
> **required whenever the parties have a voice channel** — which, in a proceeding people schedule
> and attend together, is the ordinary case.

*Why the channel is the thing that needs confirming:* everything a party knows about a ceremony —
the convener's fingerprint, the roster, the secret — arrives in the invitation, and the signed
record inside the document is verified against a convener fingerprint **learned from that same
invitation**. An attacker who controls the delivery channel does not tamper; they **replace**, and
every check passes because both halves of every comparison are theirs. Nothing else in this design
can see that. Two people who recognise each other's voices can.

*The consequence for the commitment step, which reverses this morning's call:* **it is mandatory on
the only remaining path.** An attacker who substituted both invitations is running two pinned
sessions and can birthday-grind the four words to match across them — the ~2²² search the
2026-08-17 pin describes. That attack was thought to need a defeated 66-bit pin; it needs only a
defeated *channel*. So `H(nonce ‖ contribution)` before either side reveals, and a peer that reveals
after receiving is rejected before any string exists — **unconditionally**, not on the fallback path
that no longer exists.

*What this deletes:* the conditional rule above, the two-assurance-levels distinction it created,
and the L2 restatement D31 needed to keep it honest. **One path, one rule.**

### D5 — The name does not rotate *(settled 2026-08-15 via /discuss, auto-adopted)*

One name per identity, stable for the life of the key.

*Why:* the fingerprint is already a stable public identifier that Nib displays and
users share (`internal/server/peers.go:31`), so a derived name adds no linkability
that does not already exist. Rotation would break repeat counterparties for no gain.

### D6 — Endpoint carrier: the BitTorrent DHT, with cached bootstrap *(settled 2026-08-15 via /discuss — Dan)*

Endpoints are published to and fetched from the BitTorrent DHT under a key derived
from the two names. Bootstrapping uses a **cached node list**, populated on first
contact — not hardcoded bootstrap hostnames.

**(amended 2026-08-19, P04.S02 — Dan's call, "best, most correct for all of these".)**
"Populated on first contact" never said first contact with *what*, and the slice that
needed an answer measured the consequence: **a fresh install reaches the DHT never.**
The four public bootstrap routers answer a query and **none returns the BEP-42 `ip`
field** (0 of 5 addresses), nothing in the tree seeds `~/nib/dht-nodes`, and hostname
bootstrap is forbidden here — so the routing table is empty, the self-address probe has
nobody to ask, and the record has nowhere to go. Adopted in two halves:

- **A shipped seed list of IP literals**, consulted only when the cache is empty and
  replaced by the cache on first success. This keeps *both* reasons hostnames were
  refused — no DNS lookup telling a third party who starts a ceremony and when, and no
  single point that fails on a network which blocks or rewrites DNS — while giving a
  genuinely cold machine somewhere to start. Measured at P04.S02: seeding the five
  router addresses and traversing from them yields 28 nodes and 16 self-address
  observations inside 30 s.
- **Seed nodes carried in the invitation** (D21), so the common case never consults the
  shipped list and a stale or blocked list is not fatal. This is the argument that
  declined the second signalling path, applied again: the participants already hold a
  channel and it cost nothing. ~~**Filed against P04.S03**~~ **Filed against P04.S06
  (moved 2026-08-20, S03's slice grill)**, being a change to the invitation's payload
  rather than to bootstrap — which is exactly why it is its own slice and not a rider on
  the record's.

*Unchanged:* hostname bootstrap stays refused, and `GlobalBootstrapAddrs`,
`NewDefaultServerConfig` and every resolver stay forbidden on the bootstrap path. The
seed list is IP literals precisely so the guard that enforces that does not move.

*And the second half is not a nicety — measured within the hour.* Of the five addresses
shipped, **three were already dead** the day the list was written, including
`router.bittorrent.com` and `router.utorrent.com`, the two every guide still names. Then
heavy use of the two live ones got them to keep answering `ping` while returning **no
nodes** to `find_node`, so three consecutive cold starts bootstrapped to an empty table;
patience recovered it. A two-address list is a single point of failure and this is what
hitting one looks like. The cache is the durable mechanism and the invitation is what
keeps a cold start rare.

*Why Dan's call:* the DHT is not centralised, but it is a peer-discovery system
being used for peer discovery, which brushes the "nothing designed for the purpose"
constraint. That is a judgment about the constraint's intent.

*Why cached bootstrap is not optional:* resolving hardcoded bootstrap hostnames is a
centralised touchpoint and would be the one place a rendezvous could be cut off.
It must be designed in, not left to a library default.

*Alternatives rejected:* DNS cache-timing (anycast resolvers mean the two parties
may read different caches — a silent failure); Bitcoin `addr` gossip (propagation in
hours, far too slow for a live ceremony).

**(plan-review pin: the record is public, permanent and unauthenticated — 2026-08-17,
adopted by Dan.)** This decision, **D5** (names never rotate) and **D8/D16/D17** (both
sides dial everything published, at 250 ms for 30 s then 1 s, against a 300 s deadline)
compose into a defect none of them contains alone. Names are the *public* pairing
identifier — saying yours aloud is the feature — so the rendezvous key is computable by
anyone who learns both, and it is **the same key forever**.

- **Nib becomes a packet cannon someone else aims.** A third party who knows both names
  publishes arbitrary `IP:port` candidates and both Nibs punch at them: ~390 packets per
  candidate per side over the deadline, from two hosts, with **no cap on candidate count
  anywhere in this plan**. **L1 does not cover this** — it guarantees a lying rendezvous
  can never substitute a *signer*, and says nothing about the rendezvous making Nib emit
  traffic at a victim. D16's backoff bounds the rate; nothing bounds N.
- **A permanent correlation handle.** A stable key under a public DHT tells anyone who
  knows two names every time those two people sign, and both their IP addresses. D17
  already refuses to publish the ceremony *role* because it would "add metadata to exactly
  the surface the plan is already watching"; that reasoning is right and is simply not
  applied to the endpoints themselves.

*Both halves are adopted.* **(i) The published record is encrypted** under a key derived
from both names — free, since both sides know both names by construction, and it takes the
record from readable-and-writable by any DHT scraper to readable-and-writable only by
someone who already holds both names. **(ii) Candidate count and total punch budget are
capped as law** — see the D16 pin — because someone who *does* know both names can still
publish, and confidentiality alone does not bound emission.

*Rejected:* signing the record with the identity key. ~~The counterparty holds only 66 bits
of the fingerprint from the name, not the public key, so there is nothing to verify against
until after a handshake — chicken-and-egg.~~ **(premise stale 2026-08-19 via /discuss — Dan.
D31 collapsed pairing onto the invitation, which pins the FULL 32-byte fingerprint, so nobody
holds 66 bits of anything; a record carrying the signer's public key is verifiable against a
pinned fingerprint by hashing its SPKI. The rejection's reasoning is therefore gone. Whether the
record SHOULD be signed is a live design question and is deliberately NOT settled here — an
amendment pass may not decide one. It is filed against P04: the record is encrypted under the
invitation secret, so an outsider cannot write one, but every invited party holds that secret,
so within the roster one party can publish candidates as another.)**
~~Also rejected: a per-ceremony key from a shared
secret, there being no shared secret before the ceremony, which is the problem the DHT
exists to solve.~~ **(struck 2026-08-19 via /discuss — Dan. This is exactly what D21 built: the
invitation carries a 32-byte secret delivered before anything connects, and this decision's own UX
amendment re-keys both the rendezvous key and the record encryption to it. The premise — that no
shared secret can exist before the ceremony — was true when written and was falsified by D21, and a
builder reading this decision top-to-bottom met the adopted design listed as rejected.)**

**(plan-review pin: this paragraph now rejects the adopted design — 2026-08-18, adopted by Dan.)**
"A per-ceremony key from a shared secret, there being no shared secret before the ceremony" is
**exactly what D21 built**: the invitation carries a 32-byte secret, delivered before anything
connects, and the UX amendment below re-keys both the rendezvous and the record encryption to it. The
rejection's premise — that no shared secret can exist before the ceremony — was true when written and
was falsified by D21, and the amendment superseded the *key derivation* without touching the
*rejected-alternatives* paragraph that now contradicts it. A builder reading this decision
top-to-bottom meets the adopted design listed as rejected. **Striking the clause is owed through
`/discuss`**, since it is a decision's own text.

**What discharges this specifically:** P04's new third-party-flood criterion, driven by
publishing N+50 candidates. **Not** the existing "a hostile or absent DHT degrades to the
next tier without ever affecting which peer is accepted" — a hostile DHT that floods a
bystander satisfies that bullet completely, because nothing about which peer is accepted
changed.

**(UX amendment, 2026-08-18 — the record is re-keyed to the invitation secret, and the rendezvous
key with it.)** The pin above encrypts the published record under a key derived from **both
names**, reasoning that both sides know both names by construction. They do — and so does anyone
who overhears them, because **the name is the public identifier this plan exists to have people
say out loud**. A key derived from two public identifiers is obfuscation against a scraper, not
confidentiality against the party the pin is actually worried about: the one who "knows both
names" and publishes candidates.

**Superseded:** both the **rendezvous key** and the **record encryption** derive from the D21
invitation secret — 32 bytes, per ceremony, never spoken, never published. The consequences are
the ones the pin wanted and could not get from names:

- **The rendezvous key is no longer computable by a listener.** Someone who overhears every name
  in the roster cannot find the record, so the packet-cannon needs the invitation, not an
  earshot.
- **The correlation handle stops being permanent.** D5 keeps names stable for the life of the
  key and that is right; the *rendezvous* key now changes every ceremony, so a stable pair of
  people no longer publish under a stable key.
- **Forgery needs the secret**, so "readable and writable by anyone holding both names" becomes
  "by anyone holding the invitation".

*What is not changed:* the candidate cap and the punch budget (the D16 pin) stay exactly as
adopted. A party who *does* hold the invitation — a real participant, or someone the invitation
leaked to — can still publish, and confidentiality has never bounded emission. That was true
under the names key and it is true under this one.

*What an intercepted invitation gets its holder:* the rendezvous, and nothing beyond it. The pin
is the fingerprint in the roster and they do not hold the private key, so they are refused at the
handshake. The invitation is a channel secret, **never a signing credential**, and P06 says so on
screen.

### D7 — The session transport ~~moves from TCP to QUIC~~ **gains QUIC alongside TCP (2026-08-16, D14)** *(settled 2026-08-15 via /discuss, auto-adopted; amended 2026-08-16)*

`Dial` and `Listen` (`internal/p2p/session.go:44`, `:56`) are re-based from
`tls.DialWithDialer`/`tls.Listen` over TCP onto QUIC. `SessionTLS()` is reused as-is.

*Why:* UDP is the transport that punches reliably; TCP simultaneous open is handled
badly by many middleboxes. QUIC is UDP with TLS 1.3 built in, and QUIC libraries
accept a `*tls.Config` — which is exactly what `SessionTLS()` already returns, with
the pinned-peer `VerifyPeerCertificate` callback intact. `Initiate`, `Receive`,
`coSignExchange`, the attestation logic and the length-prefixed framing all operate
on a stream and are indifferent to what carries it.

*Standing caveat:* that the chosen QUIC library invokes `VerifyPeerCertificate`
exactly as `crypto/tls` does is **unverified** and load-bearing. See Caveats.

**(transport-fallback pin, 2026-08-16)** QUIC becomes the *primary* transport, not
the only one — TCP is retained beside it per D14, so "re-based" here means "gains a
QUIC path", not "loses the TCP path". Two claims in this decision's rationale were
read at the line and are wrong as stated: `Initiate` (`internal/p2p/session.go:68`)
and `Receive` (`:101`) are typed to `*tls.Conn` and call `conn.ConnectionState()`
(`:86`, `:106`), feeding `verifiedPeerFingerprint(cs tls.ConnectionState)` (`:296`);
so are `SendDocument` (`:149`) and `ReceiveDocument` (`:173`), and three production
call sites type-assert `conn.(*tls.Conn)` off a `net.Listener`
(`internal/server/session.go:241`, `:248`). Only `coSignExchange` (`:205`) is truly
transport-agnostic — it takes `peerFP []byte` and `inbound []byte` and never sees a
conn. The session core must therefore be re-typed to a stream plus an
already-verified fingerprint. Under D14 that is no longer optional: two transports
cannot share one core while the core names one of them in its signatures.

### D8 — The connection ladder: ~~four~~ **five (2026-08-16)** tiers, attempted concurrently *(settled 2026-08-15 via /discuss, auto-adopted; amended 2026-08-16 per D15)*

In preference order, all attempted at once rather than in series:

1. **Local network** — multicast discovery. No internet, no NAT, no rendezvous.
2. **IPv6 direct** — both publish global v6 addresses and dial outward.
3. **Port-mapped inbound** — ask the router for a temporary mapping and publish the
   mapped `IP:port` as a dialable candidate (D15). **(added 2026-08-16)**
4. ~~**IPv4 hole punch**~~ **IPv4 hole punch (renumbered from tier 3, 2026-08-16)** —
   each learns its own mapped `IP:port`, publishes it, both punch.
5. ~~**Manual address**~~ **Manual address (renumbered from tier 4, 2026-08-16)** —
   the existing host:port path (D9).

First tier to complete wins.

*Why concurrent:* serial attempts pay every failed tier's timeout. *Why LAN first:*
two people signing in the same office is likely the most common real case and the
cheapest to serve.

**(transport-fallback pin, 2026-08-16)** *The transport rule across the ladder:* a
tier that ends in a **dialable address** — 1, 2, 3 and 5 — races QUIC and TCP
concurrently, on the same principle that makes the ladder itself concurrent. Tier 4
is **QUIC-only**, because hole punching is a property of UDP and has no TCP
equivalent inside these constraints. This is what keeps a UDP-hostile network from
being fatal: tiers 3, 4 and the DHT that feeds them all die there, but tiers 2 and 5
still complete over TCP.

*Why the port-mapping tier outranks the punch:* a mapping is a stable inbound
address rather than a timing-dependent one, and — the case that matters — **only one
side needs it**. Two peers whose NATs both do endpoint-dependent mapping cannot punch
at all, but if either one obtains a mapping, the other simply dials it. That converts
a class the plan previously had no answer for into a working case.

**(connection-algorithm pin, 2026-08-16)** The race's mechanics are now specified:
its clocks in **D16** (candidates trickle in; one connect deadline ends the race),
its symmetry and glare resolution in **D17** (both sides dial, deterministic
lower-fingerprint tie-break, the ceremony role decoupled from who dialled), its
behaviour on channel loss in **D18**, and its failure diagnosis in **D19**.

**A structural correction this pass surfaced, recorded rather than fixed:** the DHT
is not tier 4's private input — it is the **signalling channel for tiers 2, 3 and 4
alike**, because a dialable address is useless if the peer cannot learn it. An
unreachable DHT therefore collapses three tiers at once, leaving only tier 1 (LAN)
and tier 5 (manual). P04's framing understates this. Whether the plan wants a second
signalling path is a **Stage 2 grill target**, not this pass — D19 cause 2 makes the
failure legible in the meantime.

**(plan-review pin: the gate this was deferred to has passed — 2026-08-18, adopted by Dan.)** The
Stage 2 grill ran on 2026-08-18 and **did not address the second signalling path**; nor did it address
P04's I1 note, which carries the same question. A deferral whose gate has been walked through reads as
settled to every later reader, which is how a parked question becomes an assumed answer. **Both are
re-targeted to the P04 slice grill**, where a second signalling path is a build decision with a cost
rather than a plan question — and P04 is the phase that would have to carry it. **What discharges this
specifically:** P04's slice grill recording a decision on the second signalling path, adopt or
decline, with a reason — not any later grill of the plan as a whole, which is the deferral that just
failed.

### D9 — The manual address path is demoted, not deleted *(settled 2026-08-15 via /discuss, auto-adopted)*

The host:port field survives, moved out of the primary flow into an advanced
fallback the user does not normally see.

*Why:* carrier-grade NAT at both ends has no solution inside these constraints.
Deleting the escape hatch would turn a working case into an impossible one.

**(transport-fallback pin, 2026-08-16)** The manual path carries **both** transports
(D14). It is the tier that survives a network permitting only outbound TCP, so
binding it to QUIC alone would have removed the escape hatch this decision exists to
keep. Two corrections to the rationale above, neither of which changes the decision:
the *escape hatch for carrier-grade NAT is not usually a typed address* — someone
behind CGNAT cannot port-forward, having no control of the carrier's NAT and no
public IPv4 to forward from, so the working answer in that case is the VPN both
parties already run, which the current UI hint already names
(`web/index.html:~1214`). And the case with no answer is more precisely **endpoint-
dependent (symmetric) mapping at both ends with no port mapping available at either**,
which is not the same set as CGNAT: a hotel or campus NAT can be symmetric without
being carrier-grade, and plenty of CGNAT is endpoint-independent and punches fine.
Correcting the ladder's *diagnosis taxonomy* to that axis is a Stage 2 grill target,
not this pass.

### D10 — Existing hex-fingerprint pins remain valid; there is no migration *(settled 2026-08-15 via /discuss, auto-adopted)*

Peers pinned today by pasted fingerprint continue to work untouched. The name and
the hex string denote the same value.

*Why:* the name is an encoding of the fingerprint (D3), so no stored data changes
meaning. A migration step would be inventing work.

### D11 — The ~~two~~ **three (2026-08-18)** repo laws *(settled 2026-08-15 via /discuss, auto-adopted)*

**(plan-review pin: the decision index names two laws and there are three — 2026-08-18, adopted by
Dan.)** L3 arrived through **D23** and is stated in full in *Repo laws* above, but this decision is
the one a builder reaches from the decision list, and it enumerates L1 and L2 only. L3 is also the
law most likely to be looked up, because it is the one that refuses at runtime. **What discharges
this specifically:** this decision naming all three and pointing at D23 for L3's origin — not the
*Repo laws* section, which already had it and is not where a reader of the decisions looks.

L1 (reachability, never identity) and L2 (no silent downgrade), as stated above.
Both become named guard tests.

*Why they are decisions and not commentary:* L1 is what makes depending on an
untrusted discovery layer defensible at all, and L2 is what makes D4's mandate
enforceable rather than aspirational.

**(connection-algorithm pin, 2026-08-16)** Both laws gain a named consequence in this
pass. **L2 extends to D18:** a verification confirmation is valid for exactly one
channel and may never be carried across a reconnection — reuse would be a silent
downgrade of precisely the kind the law forbids, so the L2 guard test covers it.
**L1 extends to D19:** NAT classification is diagnostic only; it may steer messages
and tier preference and may never touch the pin check.

### ~~D12 — External gate: an outside cryptographic review before the ceremony ships~~ **WITHDRAWN 2026-08-18 — Dan's instruction** *(settled 2026-08-15 via /discuss, auto-adopted; withdrawn 2026-08-18)*

**Withdrawn on Dan's instruction, 2026-08-18: "remove the outside crypto review requirement.
It won't happen."** A gate nobody will walk through is not a gate, and a plan that carries one
reports an assurance it does not have — the same defect the plan-review pin below identified,
arriving from the other direction. Struck rather than deleted, per the house mechanics.

**Both exit criteria it produced are struck with it** — P01's "an outside cryptographic reviewer
has read the pairing and verification design … before P02 opens" and P06's "…completed against
the shipped design and its findings dispositioned". Neither phase now carries an external gate.

*What is lost, recorded plainly and without argument:* **C1 — the 2²² grindable verification
string — is the kind of finding an outside crypto reader produces, and it was found here by a
review pass rather than by an acceptance bullet.** No bullet in P01.S04 could see it. Removing
this gate removes the only step in the plan whose job was to catch the next one of those.

*What remains, and it is not nothing:* the Stage 2 grill, which has still never run and whose
targets include this decision's whole subject; the L1/L2/L3 guard tests; and — the material
change since D12 was written — **D4's supersession raises the invited path's pin from 66 bits to
256**, which removes C1's own precondition rather than defending against it. The residual
exposure is concentrated on the name-only path (66 bits, mandatory string, commitment step) and
on D21's channel-binding mechanism, which is unchosen and is now caveat 11.

**Everything below this line is the withdrawn decision and its 2026-08-17 pin, retained
unaltered for the record. None of it is live.**

*The original decision:* the feature does not ship to users until an
outside reviewer has read the pairing and verification design. Named as a phase gate, not a
footnote.

*Why:* this changes an authentication surface on a product whose output has legal
weight, and the planning playbook asks for external gates to be named at Stage 1.

**(plan-review pin: a gate named in no phase is not a gate — 2026-08-17, adopted by Dan.)**
This decision says "named here as a phase gate, not a footnote", and then no phase's exit
criteria mention it — including P06's. Adopted with a change of *placement*: the review
happens **twice**, and the first is the one that matters. The pairing and verification
design is settled in **P01**, and P02–P06 stack on it; a reviewer meeting it for the first
time at P06 finds it five phases deep. **C1 is the evidence** — a 2²² grindable SAS,
invisible to every acceptance bullet P01.S04 already carried, is exactly what an outside
crypto reader is for. Both P01 and P06 gain the gate as their own exit criterion.

### D13 — The UI surface is renamed Collaborate → Signing Ceremony *(settled 2026-08-15 — Dan's instruction)*

The mode tab (`web/index.html:30`) and the role-picker flow (`:257`) are renamed and
restructured around the new process.

**(connection-algorithm pin, 2026-08-16)** The role picker survives, but what it
selects narrows: per **D17** it chooses the *document-flow* role only — who owns the
document and therefore who calls `Initiate` — and no longer implies who dials or who
listens, since both sides now do both. Two parties who pick the same role must get a
named error at pairing time rather than a hang.

**(UX amendment, 2026-08-18 — the role picker is deleted, and the surface stops being modal.)**
Two corrections, both consequences of D20 rather than preferences:

- **The role picker is deleted, not narrowed.** ~~It chooses the document-flow role~~ **Roles are
  read from the Ceremony Record's roster (D20, D23)**: the convener is the party who created the
  record, every other party is the signer at their index. A role that is *read* cannot be chosen
  wrongly, so the same-role conflict of the 2026-08-16 pin has no way to occur — see the D17
  amendment, where the machinery for it is struck. The `Originate / Receive` toggle
  (`web/index.html:272`) and its two role-specific tool sets (`:277`, `:284`) go with it.
- **The surface is a sidebar panel, not a toolbar tab of modals.** Today the whole flow is
  buttons opening one-shot modals (`web/index.html:271`–`:289`, and the three modals at
  `:1190`, `:1215`, `:1239`) — the right shape for an operation that begins and ends in one
  sitting, and the wrong shape for a proceeding that spans days (D24). The ceremony becomes a
  panel beside Pages / Outline / Library / Flags (`web/index.html:295`), showing roster,
  position, and this user's single next action, rendered from the local record with no network.
  Per **ADR-001** the ceremony pins its document id like any other operation, and the panel names
  which tab it belongs to.

### D14 — TCP is retained as a peer transport, never deleted *(settled 2026-08-16 — Dan's instruction)*

QUIC (D7) becomes the primary transport; the existing TCP path
(`internal/p2p/session.go:44` `tls.DialWithDialer(…, "tcp", …)`, `:56`
`tls.Listen("tcp", …)`) stays beside it as a peer, raced concurrently on every tier
that ends in a dialable address (D8). `SessionTLS()` already serves both unchanged —
it returns a `*tls.Config`, and the pinned-peer `VerifyPeerCertificate` callback
(`internal/p2p/transport.go:65`) is transport-independent.

*Why:* QUIC is UDP-only, and a meaningful population of corporate, campus and guest
networks permits outbound TCP while blocking or throttling UDP wholesale — including
the DHT's UDP traffic specifically. On such a network today's ceremony still
completes over the manual address; deleting TCP would have taken a working case and
made it impossible, in exchange for nothing, since QUIC's advantage is punching and
punching is already dead on those networks. Retention also costs nearly nothing: the
code exists, is tested, and is being kept rather than written.

*What this forbids:* the P02 slice "the TCP path removed" is struck (see P02). No
phase may delete a transport as cleanup.

*The consequence that is not optional:* two transports cannot share one session core
while that core is typed to one of them. Re-typing `Initiate`, `Receive`,
`SendDocument` and `ReceiveDocument` off `*tls.Conn` — see the D7 pin for the exact
lines — becomes a requirement of this decision, not a refactor of taste.

### D15 — Router port mapping is a ladder tier: PCP, then NAT-PMP, then UPnP-IGD *(settled 2026-08-16 — Dan, posture option A)*

While a ceremony is armed, Nib asks the local router for a temporary inbound mapping
and publishes the mapped `IP:port` as a dialable candidate (D8 tier 3). Three
protocols are attempted in order — **PCP** (RFC 6887), then **NAT-PMP**, then
**UPnP-IGD** — because PCP supersedes NAT-PMP and both are simpler and less
error-prone than IGD, while IGD is what the majority of consumer routers actually
have enabled. The first to return a mapping wins; all three failing is an ordinary
tier miss, not an error.

**The mapping's lifecycle is law, not configuration:**

- Requested **only while a ceremony is armed** — never at app start, never held idle.
- **Short lease, refreshed** while armed, so a crashed Nib's mapping expires on its
  own rather than outliving the process.
- **Explicitly deleted on teardown**, on every exit path including cancel and error.
- **Never a fixed well-known port** — the mapping follows the ephemeral port the
  session actually bound.
- Both **UDP and TCP** are mapped when both transports are offered on that tier (D14).

**Posture *(Dan's call, option A, 2026-08-16)*: on by default, armed-only, and
disclosed.** The ceremony screen states that a temporary opening was requested and
names the port; the user is told, but is not asked to make a networking decision to
get a working ceremony.

*Why Dan's call, not the process's:* the mechanism above has an honest best answer,
but whether a document-signing product may mutate the user's router by default is
risk appetite about someone else's equipment — the same class of judgment as D4 and
D6.

*Why the tier earns its place:* it is the only thing in the plan that answers
double-endpoint-dependent NAT, because one mapping suffices for both parties (D8
pin). It is also cheaper to build than the tier it now precedes.

*Not assumed:* that carriers deploy PCP. RFC 6887 was specified with carrier-grade
NAT in mind, but carrier-side deployment is a caveat (8), not a premise — the CGNAT
case remains D9's.

### D16 — Two clocks: candidates trickle, one connect deadline ends the race *(settled 2026-08-16 via /discuss, auto-adopted)*

The ladder runs on two independent clocks, and conflating them is the defect this
decision exists to prevent.

**Clock 1 — per-mechanism gather deadlines.** Each candidate source has its own short
budget: multicast browse, the port-mapping request, the DHT self-address probe. A
source that misses its budget **contributes no candidate and is not an error** —
gathering never blocks the race. Candidates **join the race as they arrive**; the race
does not wait for gathering to finish.

**Clock 2 — one connect deadline** for the whole ceremony's connection attempt. When
it expires with no established channel, the ceremony fails with the diagnosis from
D19. Every per-mechanism budget is strictly shorter than it.

Initial values, named together in one constant block and **tunable, not law** —
the structure above is the law:

| clock | initial value |
|---|---|
| multicast browse | 2 s |
| port-mapping request (all three protocols) | 3 s |
| DHT self-address probe | 8 s |
| DHT candidate fetch | ~~every 2 s for the first 30 s, then every 5 s~~ **one traversal with a ≥30 s budget, the next started when the last returned (amended 2026-08-20, P04.S03's grill)** |
| punch retransmit cadence | 250 ms for the first 30 s, then 1 s |
| published candidate expiry | > connect deadline + margin (D17) |
| **overall connect deadline** | ~~60 s~~ **300 s (Dan, 2026-08-16)** |

**(amended 2026-08-20 — P04.S03's slice grill, a premise correction; D16's own text says these
values are "tunable, not law", so the structure is untouched.)** *"Every 2 s"* was unbuildable, and
the reason is a property of the traversal rather than of the number. `getput.Get` has **no early
return for a mutable item** (`goto receiveResults`, `exts/getput/getput.go:110`), and mutable is
forced on us: an immutable BEP-44 target is `sha1(bencode(V))`, which a fetcher would have to
already hold the record to compute. `defaultMaxQuerySends = 1` (`transaction.go:24`) with a flat 2 s
timeout (`dht.go:24-27`, `server.go:1052-1057`) means **every unanswered query costs 2 s**, and at
Alpha 15 against Nib's own measured ~60 % reply rate essentially every round pays it. So a 2 s
budget expires inside traversal round 1 every single time; `op.queried` is per-operation, so each
attempt restarts from our own routing table and **never reaches the eight nodes that actually store
the record**. The ladder would show "DHT: no candidate" for the full 300 s while the record sat
there, retrievable. One traversal with a real budget, polled for its result, is the shape that works.

*A companion trap, recorded because it inverts the normal Go contract:* `getput.Get`'s `ctx.Done()`
arm sets `err` and **does not touch `ret`** (`getput.go:112`), so a perfectly good record can be
returned alongside a non-nil error. `ret.Seq` is pre-set to `math.MinInt64` (`:94`), so a `Seq == 0`
emptiness check is wrong too. The result must be read as `(ret, err)` jointly.

*Why trickle rather than gather-then-dial:* the tiers have latencies an order of
magnitude apart. LAN answers in milliseconds; the DHT round trip is seconds. Waiting
for the slowest source before dialing the fastest would make the common case pay for
the rare one, which is the same reasoning that made D8 concurrent.

*Why ~~60 s~~ **300 s** (Dan's call, 2026-08-16):* two people are on a phone call when
this runs, and a ceremony they scheduled is worth more than a fast failure. Five
minutes is long enough that a slow DHT, a router that takes its time answering a
mapping request, or a peer who arms a couple of minutes late all still land inside
one attempt. The UI shows per-tier progress throughout (D19), so the wait is never a
blank spinner, and the user can abandon at any point — the deadline bounds the
*machine's* patience, not the person's.

**Three consequences of the longer deadline, adopted with it:**

- **Retries back off.** At 250 ms for 300 s a punch would emit ~1,200 packets per
  candidate, and a 2 s fetch loop ~150 DHT queries. Both step down after the first
  30 s (table above). Sustained full-rate traffic for five minutes is both wasteful
  and the kind of pattern a carrier or a DHT peer is entitled to treat as abuse.
- **Published candidates must outlive the race.** A record whose expiry is shorter
  than the connect deadline vanishes mid-attempt, so the expiry floor is the deadline
  plus margin — otherwise a peer arming at minute four finds nothing.
- **The port mapping must be refreshed, not merely short-leased.** D15's lease is
  shorter than 300 s by design, so refresh-while-armed moves from incidental to
  load-bearing: without it the mapping dies during its own tier's race.

**(plan-review pin: the race needs a size bound, not only a rate bound — 2026-08-17,
adopted by Dan.)** The backoff above bounds *how fast* the race emits and nothing bounds
*how much*: candidate count is unbounded, and under the D6 pin an attacker supplies the
candidates. Two figures join the constant block above and are **law, not tunable** — the
distinction the table's own preamble draws: a **maximum number of distinct candidates per
ceremony**, and a **global packet ceiling for the whole race**. Exceeding either drops the
excess and reports it rather than failing the ceremony.

**(plan-review pin: nothing asserts the clocks are independent — 2026-08-17, adopted by
Dan.)** The paragraph below warns that "neither clock may be implemented in terms of the
other", and no criterion in any phase observes it. **What discharges this specifically:** a
guard that lets the connect deadline elapse in full and then shows the exchange budget
undiminished — not the presence of two constants in the source.

*Not to be confused with the existing session clocks.* `exchangeDeadline`
(`internal/p2p/session.go:29`, 6 minutes) budgets an *established* session — handshake,
frames both ways, and the wait on the remote user's consent — and the server's
consent window is 5 minutes inside it (`:25`). D16's clocks run **before** any of
that and are independent of all of it. The two now sit in the same order of magnitude,
so state the total plainly: worst case is ~5 minutes connecting followed by up to
6 minutes exchanging, and neither clock may be implemented in terms of the other.

**(UX amendment, 2026-08-18 — there is a third clock, and it is the only one measured in days.)**
D24 makes a ceremony survive being interrupted, which means it outlives every clock this decision
names. **Clock 3 — the ceremony deadline**, set by the convener and carried in the Ceremony
Record (D20), measured in **days**. The same independence rule binds it: no clock may be
implemented in terms of another.

| clock | bounds | value |
|---|---|---|
| connect deadline (clock 2 above) | the ladder's race for a channel | 300 s |
| exchange deadline | one established session, consent included | 6 min (`internal/p2p/session.go:29`) |
| **ceremony deadline (new)** | the whole proceeding, across interruptions | days, convener's choice, in the record |

**A defect this exposes, read at the line:** `sessionAcceptTimeout` is **5 minutes**
(`internal/server/session.go:34`) and auto-disarms a listener no peer has reached. In a
four-party ceremony a party who arms and waits their turn is disarmed before the baton arrives.
The arm window therefore becomes a ceremony-scoped decision rather than a constant — and it is
**the only place in this whole amendment that touches the session TRIPWIRE**
(`internal/server/session.go:24`), which is why it is named here rather than left to a slice.
What must not change with it: the listener still accepts exactly **one** pinned peer, and still
serves exactly **one** session per arm (D22).

**(Stage 6 pin, crypto pack PLAN-8, 2026-08-18 — the outer clock and the inner one can disagree.)**
A hop admitted one second before the ceremony deadline still gets `exchangeDeadline`'s **six
minutes**, so the ceremony outlives its own expiry by up to that much — and the party at that hop is
asked to consent to a signature on a ceremony that has already expired. **The rule: no hop starts
unless the ceremony deadline exceeds now plus one full exchange budget.** A ceremony inside that
window ends as *expired* (D28) rather than admitting a hop it cannot honour. This is PLAN-8's
endpoint question — nesting three clocks means the outer one must reserve the inner one's worst case,
not merely be larger than it.

*Presentation rule:* the ceremony deadline is the only clock a person may see in human units.
The other two are the machine's patience; a UI that renders them as "you have 4 minutes left to
decide" turns a timeout into pressure on the one decision that must not be hurried.

### D17 — Both sides race symmetrically; the ceremony role is ~~decoupled from who dialled~~ **read from the roster (2026-08-18)** *(settled 2026-08-16 via /discuss, auto-adopted; amended 2026-08-18 — the role-conflict machinery is struck)*

**Every ceremony is symmetric at the transport layer.** Both sides publish candidates,
both sides listen, and both sides dial everything the other published. There is no
"client" and no "server" in the ladder.

**The document-flow role is a separate thing, unaffected by the race.**
~~Chosen in the UI: Originate (you own the document) and Receive (`web/index.html:257`, D13)
determine who calls `Initiate` and who calls `Receive` *after* a channel exists.~~
**Superseded 2026-08-18 (D20, D22): read from the roster, not chosen. The party holding the
baton calls `Initiate`; the party whose turn the record names calls `Receive`.**

~~**Roles are chosen before connecting, and a conflict stops the ceremony
*(Dan, 2026-08-16)*.** Each side commits to its role in the UI before the ladder
starts — the role is never inferred from who dialled, never negotiated, and never
swapped automatically. If both sides chose the same role, the ceremony **stops and
both parties try again** after one of them changes their pick. There is no recovery
in place: no auto-swap, no "one of you becomes the originator", no silent coin flip.~~

**Superseded 2026-08-18 (D20, D23): roles are not chosen at all, so a conflict cannot exist.**
Every role is **read** from the Ceremony Record's roster — the convener is the party who created
the record, and each other party is the signer at their own index. The three paragraphs below
that specify *where the conflict is detected*, *why stop rather than
auto-resolve*, and *what a tampered role bit can do* are struck with it: there is no role bit on
the wire to tamper with, nothing to detect at the first exchange, and nothing to stop for. The
substance those paragraphs were protecting survives intact in a stronger form — **the machine
still never decides whose copy is authoritative**; the convener decided it in the record, before
anyone connected, and every party verifies that record against its signature.

*What is deliberately retained from this decision:* the transport half — symmetric racing,
both sides publishing and dialling, and the deterministic lower-fingerprint glare tie-break
below. None of that depends on roles, and all of it is still needed.

~~*Where the conflict is detected, and why not earlier:* the *choice* precedes
connection, but the *conflict* cannot be discovered before the two sides can talk —
detecting it requires comparing two locally-held values. The comparison therefore
happens at **the first exchange on the surviving channel**: after glare resolution has
picked the one channel (below), and before the verification string is derived, before
consent, before any document byte. The alternative — publishing the role alongside the
DHT candidate, which would catch it sooner — is rejected: it would put a second
observable attribute under a stable public rendezvous key, adding metadata to exactly
the surface the plan is already watching.~~ **(struck 2026-08-18 — and note the rejected
alternative's reasoning is now moot for a different reason: under the D6 amendment the
rendezvous key is neither stable nor public.)**

~~*Why stop rather than auto-resolve:* the role determines who is attesting to what, on
a document that carries legal weight. A machine that silently picks the originator
has decided which person's copy is authoritative. Two people on a call resolve it in
five seconds; the software should not guess.~~ **(struck 2026-08-18 — the principle is kept and
strengthened: the software still does not guess, because a person wrote the order down before
anyone connected.)**

~~*What a tampered role bit can do:* the role exchange happens on a pinned channel but
before the verification string, so an attacker who has defeated the 66-bit pin could
flip it. The worst outcome is a spurious conflict and a restart — a nuisance, not a
compromise, since the verification string still gates everything that matters (D4).~~
**(struck 2026-08-18 — no role bit crosses the wire. What replaces it is a signed record every
party verifies, and L3, which refuses a contribution out of roster order in Go.)**

**Punch synchronization needs no shared clock.** Both sides retransmit punch packets
at the D16 cadence for the life of the race; the first packet to traverse opens the
mapping and the peer's next retransmit arrives. The only requirement is that the two
arming windows *overlap*, which the ceremony flow guarantees by construction — two
people arrange this live. Published candidates carry an expiry so a stale record from
a previous session is never punched at — floored above the connect deadline (D16), so
a record can never expire during its own race.

**Glare resolution is deterministic.** Symmetric dialing means both sides can complete
a handshake at nearly the same instant, leaving two channels. On first success the
ladder waits a short settle window before proceeding, then keeps exactly one channel
by a rule both sides compute identically: **the channel whose dialer holds the
numerically lower 32-byte identity fingerprint survives; the other is closed.** Both
fingerprints are known to both sides once the handshakes complete
(`verifiedPeerFingerprint`, `internal/p2p/session.go:296`), so neither side needs to
ask the other which channel won.

*Why the tie-break lands before the verification string:* the string is derived per
channel (D4), so displaying it and *then* dropping that channel would show the user
four words that attest to a connection they are no longer on.

*What this costs elsewhere:* `Dial` and `Listen` currently bake direction into the
transport (`session.go:44`, `:56`), and the server arms exactly one listener today.
Symmetric racing means both roles run at once on both sides — a P05 concern, but it
lands on the same re-typed core D14 already requires.

### D18 — A confirmation is valid for exactly one channel *(settled 2026-08-16 via /discuss, auto-adopted)*

**Law.** The four-word verification confirmation (D4) attests to *the channel it was
computed on*. It is never carried across a reconnection. This is a direct consequence
of L2: a confirmation reused on a second channel would assert an agreement the two
people never made about that channel.

Behaviour splits at the confirmation gate:

- **Channel lost before both confirmations** — harmless. The ladder re-races within
  the remaining connect deadline (D16). Because the new channel derives a new
  verification string, **both sides re-read and re-confirm**; a stale string on screen
  is replaced, never silently reused.
- **Channel lost after both confirmations** — the ceremony ~~**fails and restarts from
  the beginning**~~ **restarts the *hop* (amended 2026-08-18, D24)**, including a fresh
  verification string. The **channel** does not resume. Resuming the channel
  would require re-establishing one whose replay bound is per-session: the
  prefix-extension check binds the returned document to the bytes sent *this* session
  (`internal/p2p/session.go:97`), and `exchangeDeadline` budgets one exchange (`:29`).

*Why fail rather than auto-reconnect after confirmation:* an automatic reconnect would
still demand a full re-confirmation, so it buys only the saving of one button press,
at the cost of a resumption state machine on the one path where correctness matters
most. Rare event, simple rule.

*Guard test:* the L2 guard named in D11 extends to cover it — no path may reach the
signing exchange carrying a confirmation computed on a different channel.

**(UX amendment, 2026-08-18 — the law is unchanged and is what makes resumption safe; only the
blast radius of "restart" shrinks.)** With a Ceremony Record (D20) there is now a beginning to
return to that is not the beginning of everything: **the unit of restart is the hop**, identified
by `(ceremony id, roster index)` (D24). A four-party ceremony that loses its channel at hop 3
re-races hop 3. The two signatures already on the document are untouched — they are signatures,
not session state.

*Why the law gets easier to live with rather than harder:* under D4's supersession the
re-confirmation on the new channel is **machine-to-machine on the invited path**, so a party
whose Wi-Fi dropped re-confirms without being asked to do anything. D18 was the decision whose
cost was a repeated human step; option A pays that cost off. ~~On the name-only path the law is
felt exactly as originally written, and correctly so.~~ **(struck 2026-08-18, plan-review, adopted
by Dan: there is no name-only path. The sentence before this one is now the whole truth.)**

*The one thing the hop must not do:* re-sign. A hop that has already produced a signature
**re-delivers** it (D24) — otherwise resumption stacks a second block from the same identity on
the page, which is wrong as a record and, per D25, wrong as a layout.

**(Amendment 2026-08-22, P05.S10 deepdive — Dan chose A. The LAW is unchanged; the MECHANISM is named.)**
"Lost before both confirmations" was found not locally decidable — confirmation is one-sided and the
peer's never crosses the wire (two-generals). The re-race clause is therefore realised by IDEMPOTENCY,
not by a "both confirmations" check: the losing side re-races unconditionally, and the receiver, on a
reconnect, RE-DELIVERS its cached signature (keyed on the hop and the inbound content hash) or, on a
cache miss, exchanges fresh and re-confirms on the new channel. The cache — not a wire signal — decides
re-deliver vs fresh, which is why no post-confirmation ACK is needed (and why one would not help). This
is the implementable form of D24's "re-deliver, do not re-sign".

### D19 — Failure is diagnosed on the mapping/filtering axis, and names ~~four~~ **five (2026-08-18, D35)** distinct causes *(settled 2026-08-16 via /discuss, auto-adopted; supersedes the CGNAT framing in P05)*

The ladder classifies NAT behaviour on the **RFC 4787 axis — mapping behaviour
(endpoint-independent vs endpoint-dependent) and filtering behaviour** — not on
whether the NAT is carrier-grade. The classification comes from a DHT probe:
comparing the mapped `IP:port` reported by **two different DHT nodes** distinguishes
the two mapping classes. ~~It comes free from the DHT probe~~, ~~exactly as a
two-server STUN check does~~.

**(amended 2026-08-19, P04.S02 — Dan's call, "best, most correct for all of these".)**
Both struck clauses were wrong, and the slice that implemented the sentence is what
found them.

- ~~"comes **free**"~~ — it does not. It needs distinctness rules (two DHT nodes behind
  one NAT are one destination, so the count is over **/24 and /48 prefixes**, not over
  nodes), per-family scoping, a bogon filter, corroboration across nodes, and a third
  value. Each is small; together they are the slice.
- ~~"exactly as a two-server STUN check does"~~ — it is *analogous*, and the difference
  is load-bearing. RFC 5780 uses a server the client chose, with a known alternate
  address **and alternate port**. The DHT offers no alternate port, no assurance that
  two responders are topologically distinct, and every responder is an anonymous
  stranger with an incentive to lie.

*What the probe actually resolves*, stated so nothing downstream over-reads it:
**one bit** — endpoint-independent versus the merged {address-dependent,
address-and-port-dependent}. Separating those two needs a second observation from the
*same* address on a *different* port, which the DHT cannot be asked for.
**Filtering behaviour is not measured at all**, and it is half of what decides whether
a punch succeeds. And the reading is a snapshot of one allocation rather than a property
of the NAT: Linux MASQUERADE preserves a free source port, so a conditionally-symmetric
NAT reads as endpoint-independent until that port is contended.

~~Four~~ **Five (2026-08-18)** causes, ~~four~~ **five** messages — where the plan previously had one:

1. **The peer never published.** They are not armed, or they are on a different
   ceremony. → "The other side hasn't started their ceremony yet."
2. **The rendezvous is unreachable.** No DHT responses at all — the usual cause is a
   network that blocks outbound UDP. → names the LAN and manual/VPN paths, which are
   the two that survive (D14).
3. **The peer published but nothing connects, and the mapping classes explain it.**
   ~~Both ends~~ **this side (2026-08-19)** endpoint-dependent with no port mapping
   obtained. → says a direct connection is not possible between these two networks, and
   names the two things that fix it: either side enabling port mapping on their router,
   or a VPN both already run.

   **(amended 2026-08-19, P04.S02 — Dan's call, "best, most correct for all of these".)**
   Two corrections, both found by building the probe rather than by reading the decision.

   **The predicate becomes one-sided.** "Both ends" is a joint predicate over two hosts
   and **nothing carries the peer's class** — P04.S03's record defines no such field — so
   as written this cause could never be computed, and would degrade to cause 4 *even when
   both probes succeeded*. That is not caveat 9's degradation; it is a cause that never
   fires. Adding the field was considered and **refused**: D17 already declined to publish
   the ceremony *role* because it "would add metadata to exactly the surface the plan is
   already watching", a NAT class is the same category of metadata on the same surface,
   and every roster member holds the encryption secret — so a forged "I am
   endpoint-dependent" would make the other side give up early with a *confident wrong*
   message, which is worse than a vague one. One-sided is weaker, honest, and always
   computable.

   **The advice becomes conditional, because as written it is wrong for the population
   that most often triggers it.** "Either side enabling port mapping on their router" is
   what D9's pin says must not be said to someone behind CGNAT, who "cannot port-forward,
   having no control of the carrier's NAT", and caveat 8 records that carriers mostly do
   not answer PCP. The sting is that a CGNAT violating RFC 4787's Paired pooling hands out
   a different external IP per destination — which is *precisely* what makes two
   observations disagree and lands the user in this cause. So port-forwarding is offered
   only on evidence the user controls a NAT: the mapped address is **not** in
   `100.64.0.0/10`, **and** D15's port-mapping tier got an answer. Otherwise the message
   names the VPN path and stops.
4. **The peer published, the classes do not explain it.** Something else — filtering,
   a firewall, an asymmetric failure. → an honest "couldn't establish a connection"
   with the per-tier detail available.

Presentation is **plain language first, with the technical detail behind a
disclosure** — the person who can act on "endpoint-dependent mapping" is exactly the
person who will open the details.

**L1 pin:** the classification is **diagnostic only**. Nothing it produces may
influence which peer is accepted; it changes messages and tier preference, never the
pin check. The L1 guard covers it.

5. **The clocks disagree.** *(added 2026-08-18, D35.)* The peer's ephemeral certificate was
   rejected on its validity window, and the window's own `NotBefore`/`NotAfter` say by how much and
   in which direction. → "This machine's clock is about N minutes behind the other side's." **The
   only one of the five that names a fix the user can perform in ten seconds** — and the one that,
   before D35, arrived as cause 4's "something else".

   *(plan-review pin, 2026-08-18, adopted by Dan: this decision's heading and list said **four** while
   D35 and P05's criteria said **five**. The decision that owns the taxonomy is the one a builder
   implements from, so the taxonomy is corrected here rather than referenced from elsewhere.)*

*Depends on:* caveat 7 — the classification is only meaningful if the probe runs on
the same local socket the session will use.

### D20 — The Ceremony Record: one signed artifact, written before anyone connects *(settled 2026-08-18 — UX pass, Dan's four requirements)*

Every ceremony begins with a **record**, created by the convener before a packet moves. It is
the ceremony's identity, roster, order, intent and deadline, and **every other decision in this
pass reads from it rather than asking a person**.

| field | carries | why it is in the record and not in a UI |
|---|---|---|
| `id` | 128 random bits | names the hop that resumes; without it "continue" is a guess about which ceremony |
| `docHash` | SHA-256 of the prepared document | every party agrees to the same bytes, and a resumed hop can prove it |
| `roster` | ordered `{fingerprint, label, signs, capacity}` | the order **is** the roster's order; `signs:false` is the non-signing convener (Dan, 2026-08-18); `capacity` added 2026-08-23, see the pin below (`name` deleted v1.117.41 — it was a pure function of the fingerprint) |
| `intent` | the ceremony's **recital** — what the proceeding is about — **the only home; see the pin below** | today each side types its own sentence; a four-party signing agrees to one thing |
| `expires` | the ceremony deadline, in days | D16's clock 3 |
| `rosterHash` | commitment over all of the above — **axes enumerated below** | the token every signature carries (D2 pin), so all signatures attest to one proceeding |

**(amended 2026-08-23 — plan-review, Dan's call: a roster entry gains a `capacity`, and `intent`
becomes the recital rather than the whole story. FormatVersion 3 → 4.)**

D20 made `intent` a single string and P07 was to write it **verbatim into every signature's signed
`/Reason`**. The legal-documents seat read that against how executed documents actually work and
found the collapse: real instruments have parties signing in **different capacities** — as principal,
as witness, as guarantor, "signed for and on behalf of" a company — and a witness is bound by nothing
the principal agreed to. Under D20 as written, a witness's own key signs *"I agree to sign this
document"*, and the block above their signature says the same. **That is an affirmative false
statement about that party's obligation, inside the artifact, signed by them** — and it is what an
opposing solicitor reads out.

The two are different fields and the decision had collapsed them. So: **`intent` stays exactly as it
is** — one recital, one home, committed, rendered once — and **`Party` gains a `capacity` string,
inside the preimage** on precisely the argument that put `signs` there: a convener who could present
one roster to the signers and another to a verifier, differing only in who was obliged to do what,
would have both hash the same. Each party's block renders its own capacity; the recital renders once.

**The timing is the whole decision, not a detail of it.** This is a `FormatVersion` 3 → 4 bump, and
**P07.S02 is the first code in the product that ever constructs a `Record`** — the panel established
that every `Record` literal in the tree today is inside a `_test.go`. There are no records in the
field, so the bump costs nothing now and costs a migration forever after P07 ships. Taken now for
that reason and no other.

**What discharges it:** a P07 criterion in which a four-party ceremony convened with two distinct
capacities produces a document whose blocks render **each party's own**, and whose signed `/Reason`s
differ in capacity while carrying one identical recital. An empty capacity is the ordinary case and
renders nothing — it is not a required field, and a ceremony that does not need it must not look
misconfigured.

**(Stage 6 pin, crypto pack PLAN-2, 2026-08-18 — "commitment over all of the above" is a gesture, not
a specification.)** `rosterHash` is the token every signature in the document carries, and this table
did not say what it commits to. A slice would have decided, and the decision would have been
invisible. **The preimage axes, in order, each length-prefixed:** the format version (D32), `id`,
`docHash`, `intent`, `expires`, and then for each roster entry in order — `fingerprint` (32 raw
bytes), `signs` (one byte), `label`. **Deliberately excluded, and the exclusion argued:** the
six-word `name`, because it is a pure function of the fingerprint (D3) and including it would let a
wordlist change alter a commitment the freeze exists to keep stable; and the invitation secret,
because verifiers who are not participants must be able to check the commitment from the document
alone.

**(implementation pin, 2026-08-19 — `docHash` cannot be a hash of the document's bytes, and this was settled by measurement.)** The table says *"SHA-256 of the prepared document"*, and the plan-review pin requires a party at hop 4 to **recompute** it. Those two cannot both be true of a byte hash: the record contains the hash, so the document carrying the record is not what was hashed. The obvious repair — hash it with the record stripped — **also fails**, and not marginally: **pdfcpu's rewrite is not idempotent.** Normalising the same document twice produces two different files (measured 2026-08-19), so attach-then-detach is not an identity and the convener and a later party compute different numbers from the same document every time.

**Adopted: `docHash` is a content digest** — SHA-256 over the page count and every page's content stream, each length-prefixed (`pdfops.ContentDigest`). Measured on the same run: **identical across adding the record attachment and across three incremental signatures**, which is what makes the hop-4 clause buildable at all.

**The narrowing is stated rather than left to be discovered.** It covers what is on the pages and **not** annotations, form field values, attachments or metadata — so a form value could change without moving `docHash`. That is acceptable here and the reason is not "it is unlikely": the signatures cover the real bytes and flip to invalid on any edit, so document integrity is already carried by a stronger mechanism. `docHash` answers the narrower question the record needs — *is this the document the ceremony was convened over* — and answers it in a way every party can check. **What discharges this specifically:** the recompute after three incremental signatures, driven; not the convener's own round-trip, which is satisfied without anyone recomputing anything.

*Why `signs` is inside the preimage and not a display field:* without it, a convener could present
one roster to the signers and another to a verifier, differing only in who was obliged to sign, and
both would hash the same. That is the whole class PLAN-2 exists to catch — an axis short is the
attack.

**(plan-review pin: `intent` has two homes — 2026-08-18, adopted by Dan.)** The record carries one
`intent`, justified above as replacing the per-side sentence. But `coSignExchange`
(`internal/p2p/session.go`) asks the `Confirmer` for **that signer's own** intent and writes it into
their attestation and their visible block, and D2 keeps the attestation format otherwise unchanged.
Nothing reconciles the two, so a finished document can say one thing in the record and a different
thing on each signature block — the same face-versus-signature divergence `Attestation`'s own doc
comment was corrected for. **Adopted: the record's `intent` is *the* intent.** The per-signer field
is **populated from it, not typed**, and the consent screen shows it as the thing being agreed to
rather than a box to fill. **What discharges this specifically:** a P07 criterion in which **every
signature block on a completed ceremony carries the record's intent verbatim** — not the record
round-trip bullets, none of which ever read a signature block.

**The record is signed by the convener** and verified by every party against the convener's
pinned fingerprint. A convener cannot reorder a ceremony in flight, because the order every party
checks against is the one they each verified at the start.

**Where it lives, and when it is written — both are constrained, not chosen:**

- **Embedded in the document, in the same pre-signing structural pass that appends the readme**
  — `PrepareDocument` (`internal/p2p/cosign.go:17`) gains it. The timing is forced by the
  package's own contract: after one clean structural pass, every later revision is a pure signing
  increment (`internal/p2p/attestation.go:1`). Attach it later and every signature already on the
  file breaks. **Both halves measured 2026-08-18 (Stage 2 grill, caveat 10 struck):** a record
  attached in the pre-signing pass survives three incremental signatures byte-identical with all
  three valid; the same call *after* signing yields `state=invalid` with every signer
  `valid=false`. The timing rule is a demonstrated consequence, not a reading of pdfcpu's docs.
- **Mirrored to `~/nib/ceremonies/<id>/`** with the current document bytes and this user's own
  contribution. That directory is what makes D24 possible at all: the ceremony survives quitting
  Nib, rebooting, and coming back on Thursday.

**(plan-review pin: `docHash` is circular as written, and the pre-signing order decides it —
2026-08-18, adopted by Dan.)** The table says `docHash` is "SHA-256 of the prepared document", and
this decision embeds the record **in** that document. Hashing the prepared document hashes the
record, which contains the hash. A builder must invent the answer and the invention becomes the
definition — invisibly, which is the failure the `rosterHash` pin one field above was written to
prevent.

**The definition, adopted:** `docHash` covers the document **after the readme is appended and the
signature pages are allocated (D25), and immediately before the record is attached** — the last
well-defined state that is not self-referential.

**The pre-signing pass therefore has three steps and a fixed order:** append the readme → allocate
signature pages from the roster → hash → attach the record. The order is not a style choice: it
fixes `docHash`, and it decides which page `stackPlacement` selects, since that function places on
the **last** page (`internal/p2p/cosign.go`, `stackPlacement`).

**Recoverability, which is the half that makes this checkable by anyone but the convener:** a later
party holds a document carrying N incremental signatures, and PDF incremental updates make each
revision a **byte-prefix** of the next — so the pre-record revision is a prefix of what they hold and
`docHash` is recomputable from it. **What discharges this specifically:** a P01.S06 criterion in
which **a party at hop 4 recomputes `docHash` from the document it received and matches the record** —
not the existing round-trip bullet, which the convener's own bytes satisfy without any later party
ever recomputing anything.

*Why one object rather than four features:* multi-party has no roster to be multi about,
resumption has nothing to resume from, ordering has no order to enforce, and the spoken check
exists only because no earlier artifact carried more than 66 bits. All four of Dan's requirements
are the same absence, and this is it.

### D21 — The invitation carries the roster and a full-strength pairing secret *(settled 2026-08-18 — Dan, option A)*

The convener issues one **invitation** per party: the ceremony id, the roster with **full 32-byte
fingerprints**, and a **32-byte secret**. It is an object — copyable string, QR, or file —
delivered over whatever channel the parties already use.

- **The pin is full strength.** The roster carries whole fingerprints, not 66 bits of them, so
  the invited path is a 256-bit pin and D4's spoken check stops being load-bearing there.
- **The secret binds the channel**, so the confirmation that used to be spoken is computed by the
  two machines. **Nothing is exchanged once connected** — Dan's requirement, met literally.
- **It keys the rendezvous** (D6 amendment): both the DHT key and the record encryption derive
  from it rather than from names that are public by design.
- **It is not a signing credential.** An intercepted invitation reaches the rendezvous and stops
  at the handshake, because the pin is the fingerprint and the holder has no private key. P06
  says this on screen rather than leaving the user to reason about it.

*Why an object at all, when the plan's premise was one spoken name:* **a roster of four cannot
be spoken.** Multi-party needs a distributable invitation whether or not it carries a secret, so
the secret is free — and it is the thing that removes the human step rather than merely
tolerating it. The six-word name (D3) is untouched and stays the human identity.

**(pin: a lost invitation is re-issued, not re-convened — 2026-08-18, gap #24.)** D28 says re-running
a ceremony is "a new record, with a new id and new invitations", which is right for a ceremony whose
*terms* changed and far too heavy for someone who deleted an email. **Re-issuing the same invitation
is safe and is the answer:** the id, the roster and the secret are unchanged, so nothing about the
ceremony's identity or its cryptography moves, and the record every party already verified still
matches. *What discharges this specifically:* re-issuing to one party mid-ceremony and completing,
with the other parties' state untouched — not the convene criteria, which only ever issue once.

*Dan's call, not the process's:* it trades the plan's purest property — one short name and
nothing copyable — for the removal of every post-connection exchange. That is risk appetite and
product shape, the same class as D4 and D6. **Option A taken 2026-08-18.**

*Unspecified here, deliberately:* the channel-binding mechanism (a PAKE over the secret, or an
HKDF over secret ‖ transcript) is named at slice-grill time and is **caveat 11**.

**(Stage 6 pin, crypto pack PLAN-1, 2026-08-18 — the invitation is the root of trust and nothing
signs it.)** D20's record is signed by the convener and verified against the convener's pinned
fingerprint — but a party learns that fingerprint **from the invitation**, so the invitation is the
root and it carries no signature. Stated rather than fixed, because the honest answer is that a
signature would not help: there is nothing to verify it against until the invitation has already been
trusted.

~~*What tampering with an invitation actually buys:* **denial, not substitution.** Alter a roster
fingerprint and that party pins a key nobody holds, so the hop never connects. Insert an attacker's
own fingerprint and the convener — who holds their own copy of the record — refuses the contribution
under L3, and the roster commitment on every existing signature no longer matches.~~ The invitation's
integrity assumption is therefore the same one the spoken name always had: it travels over a channel
the parties already trust. That was never written down, and a reader would otherwise assume the
signed record covers it.

**(CORRECTED 2026-08-18 — Dan's decision. "Denial, not substitution" is wrong for the attacker who
matters.)** The struck analysis holds for an attacker who **alters one field** while the rest of the
trust chain stays honest. An attacker who **controls the delivery channel does not alter — they
replace.** They issue their own invitation carrying their own fingerprint as convener, their own
roster, and their own signed record. Every check the recipient can perform passes, **because both
halves of every comparison are theirs**: the record's signature verifies against the convener
fingerprint the recipient holds, and the recipient holds it because the attacker sent it.

**So the invited path's entire security rests on the integrity of the channel that carried the
invitation, and nothing in the ceremony verifies that channel.** This is now the single point of
failure of the whole design, stated plainly because it was previously stated as its opposite.

*What answers it:* not a signature on the invitation — there is nothing to verify one against until
the invitation is already trusted, which is why this decision rejected signing it in the first
place. **The answer is the four-word verification string, re-purposed by D4's second supersession**:
two people who recognise each other's voices confirming that the words match is the one check an
email-level attacker cannot pass, and the only one in this design anchored outside the channel under
attack.

**(plan-review pin: the roster has two homes and nothing reconciles them — 2026-08-18, adopted by
Dan.)** A party pins from the **invitation's** roster and later receives a document carrying the
convener's signed **record**, which has its own. The pin above *reasons* that tampering yields denial
— but **no criterion anywhere requires the comparison that would produce it**, so a tampered
invitation surfaces as an unexplained connection failure rather than as the named refusal the
reasoning promises. **Adopted:** on first receipt of the document, the party compares the
invitation's roster against the record's and **refuses by name on mismatch**. **What discharges this
specifically:** a P01.S07 criterion driven with a **one-byte-altered invitation**, observing the
named refusal — not the existing "an invitation pins the full 32-byte fingerprint" bullet, which a
tampered invitation satisfies perfectly by pinning the wrong key at full length.

### D22 — Topology: a convener hub and a serial baton; the convener may be non-signing *(settled 2026-08-18 — Dan, option A)*

**N parties sign in sequence and the convener carries the baton between them.** The convener
writes the record, prepares the document, dials each party in roster order, and delivers the
finished document at the end. Every hop is exactly today's two-party session: one dialer, one
listener, one pinned peer.

**The convener may be non-signing** (`signs:false`) — a clerk or solicitor who convenes,
carries and delivers without contributing a signature. **Dan's call, 2026-08-18:** it costs one
boolean in the roster and it is how these meetings actually run.

*Why a hub, and the argument is about exposure rather than convenience:* the TRIPWIRE at
`internal/server/session.go:24` says the armed listener accepts **one** pinned peer and tears
down after **one** session, and that widening either needs a fresh security review. **Under a hub
neither widens.** Each participant still arms for one pinned peer — the convener — and still
serves one session. The only thing that changes is how long a listener may wait (the D16
amendment). A mesh would break both clauses at once, need N(N−1)/2 pinnings, and still not work,
because a PDF gains signatures by **incremental append** and cannot take concurrent writers.

*What "multiple parties at one time" means here, and what it does not:* everyone is present in
one sitting — same call, same room, same roster filling in. What is serial is the **signature**,
not the attendance. **Dan's call (option A, 2026-08-18):** live presence between hops is *not*
built; a party learns the roster's state when the convener reaches them, and the convener saying
"you are next" on the call does the rest. Persistent presence channels are the one part of this
design that would touch the tripwire, and they are deferred until someone misses them.

**(plan-review pin: the roster maximum and "one sitting" imply different products — 2026-08-18,
adopted by Dan.)** D33 caps the roster at **32**, and hops are serial: at up to 300 s connecting
(D16) plus a six-minute exchange, a full 32-party ceremony is on the order of **six hours of
continuous convener attention**. "Everyone present in one sitting" does not survive that, and the two
decisions currently describe different products to whoever reads them first. **Both numbers are kept
and their roles separated:** 32 is the **hard cap** — what the code refuses past, and what D25 sizes
signature pages against — and **~8 is the practical single-sitting ceiling**, which is what this
decision's premise assumes and what the UI should be designed and copy-written for. **What discharges
this specifically:** this decision stating the sitting ceiling in its own text, so a builder sizing a
roster picker does not read 32 as the design target — not P07's nine-party layout criterion, which
exercises the page allocator and says nothing about how long a human sits.

*What this needs from the live path, read at the line:* `coSignExchange` refuses anything but
exactly one prior signer — `len(ats) != 1` (`internal/p2p/session.go:229`). It becomes "the
attestations are the record's roster prefix" (D23). That is the whole two-party assumption in the
transport; the artifact model never had one (D2 pin).

*Delivery:* the finished document reaches every party over the existing one-way transfer
(`SendDocument` / `ReceiveDocument`, `internal/p2p/session.go:166`, `:190`), dialled by the
convener as a final round. Nothing new is invented for it.

**(plan-review pin: "nothing new is invented for it" is where the delivery round's state went missing
— 2026-08-18, adopted by Dan.)** D24 makes a **hop** idempotent and says nothing about delivery, and
the transfer this reuses names its file by wall clock: `receivedName` is
`slug-YYYYMMDD-HHMMSS.pdf` (`internal/server/session.go`, `receivedName`). So a round that fails at
party 3 of 4 has **no safe re-run** — re-delivering writes a *second copy* to the two parties who
already have it, or silently overwrites inside the same second. **Adopted:** delivery is keyed by
**`(ceremony id, party)`** and is a **no-op** once that party's acknowledgement is recorded, and the
written filename carries the ceremony id. **What discharges this specifically:** a P07 criterion in
which **a delivery round re-run after a mid-round failure leaves exactly one file per party** — not
the existing "the finished document reaches every party" bullet, which a round that delivers twice
satisfies completely.

**(plan-review pin: "nothing new is invented for it" is false in four places — 2026-08-29, seventh
pass.)** Measured against the tree, the delivery round needs four things this decision says it does
not. **(1)** The recipient is not listening: the post-signature window is `connectDeadline`, 300
seconds, after which the loop returns and disarms (`internal/server/session.go:1329`, `:1300`), while
P08's C08 says *hours*. **(2)** Nothing can find them: `handleSessionSend` resolves through
`peerAddresses`, a typed address or a LAN browse and nothing else (`internal/server/lan.go:311`) —
no DHT, no punch, no ladder. **(3)** The transport asks a human: `SendDocument` and `ReceiveDocument`
both run `runVerification` (`internal/p2p/session.go:570`, `:608`), so delivering to four parties is
four attended, vault-unlocked sessions with a spoken identity check for a document they already
signed. **(4)** The acknowledgement is emitted before the receiver has stored a byte
(`internal/p2p/session.go:640`, then `saveReceived`, which reports nothing on failure). The re-arm is
also a **second session per party**, which the TRIPWIRE at `internal/server/session.go:24-37` says may
not be widened without a fresh security review. The decision is not edited; P08.S05 carries the
consequences and the amendment is Dan's.

**(holistic pin, 2026-08-18 — the non-signing convener has no code path, and would fail its own
check.)** This decision's own option-A clause collides with the tree, and it is the only place in
the whole UX pass that does. Read at the line: `handleSessionInitiate` calls `buildCoSigned`
**unconditionally** before it dials (`internal/server/session.go:625`), and `Initiate` then runs
`confirmCoSigned`, which **requires the caller's own signature in the returned document** —
"returned document is missing your own signature" (`internal/p2p/session.go:281`, `:308`). A
convener with `signs:false` therefore either signs against its own roster or is refused at the
door.

*What is actually needed:* a **third route** — carry a document to a pinned peer, wait for the
signed result, verify it against the **record's** expectations rather than against the caller's own
signature, and return it. `Initiate` signs and demands its own signature back; `SendDocument` is
one-way and nothing returns; neither is it. **Building it is a P07 slice, and naming it here is the
point** — a phase that inherits "the convener may be non-signing" without this paragraph would
discover it at the first four-party run, after the panel and the roster were built around it.

*What does not change:* a **signing** convener works on today's shapes unmodified, because it is
`roster[0]` and its signature is present in every later return, so `confirmCoSigned` is satisfied
at every hop. The gap is exactly and only the non-signing case.

*Also owed by this decision, and carried in D27:* the consent gate the peer sees at each hop is
typed to **one** counterparty, so a fourth signer cannot be shown the three who preceded them.

**(plan-review pin: the channel binding assumes the wire peer signed before you; under a hub it is
the carrier — 2026-08-18, adopted by Dan.)** This decision says `coSignExchange`'s two-party
assumption is `len(ats) != 1` and calls that "**the whole** two-party assumption in the transport".
**It is one of three, and the other two are the security-load-bearing ones.** Read at the line
(`internal/p2p/session.go`, `coSignExchange`), all three derive from `peerFP`, the TLS-verified wire
counterparty:

1. `len(ats) != 1` — named above, re-based on the roster prefix by D23.
2. `peer.Fingerprint != hex(peerFP)` — *"the document was not signed by the connected peer."*
3. `peer.AcceptedPeer != hex(myFP)` — *"the peer's attestation does not accept you."*

…and the attestation this function writes takes `AcceptedPeer: hex(peerFP)` — **the wire peer**.

**Under this decision's hub, every hop's wire peer is the convener.** With a **non-signing**
convener — this decision's own option-A clause — the connected peer has signed nothing, so check 2
**refuses at every hop**, and check 3 refuses because the previous signer accepted the *carrier*,
not the party now being asked to sign. With a **signing** convener the checks do not refuse cleanly
either: `ats` is a prefix of N, the code reads `ats[0]`, the convener is `ats[0]` and the party who
signed immediately before is `ats[k-1]`, and nothing says which the binding is against.

**`crossBind` degrades in the same motion.** `Matched` is set only when *another signer* holds the
accepted fingerprint, so every party accepting a non-signing convener leaves **`Matched` false on
every signature of the finished document** — a verifier reads a document whose every signer attests
to somebody absent.

**Adopted: the attestation binds to the record, not to the wire.** ~~`AcceptedPeer` names the party's
**predecessor in the roster**~~ **— amended 2026-08-23, see below**; checks 2 and 3 are re-based onto
*the document's signers equal the record's roster prefix* (which L3 already requires) plus a separate,
weaker check that **the wire peer is the record's carrier**. **What discharges this specifically:** a
P07 criterion in which **a four-party ceremony with a non-signing convener completes and every
signature on the finished document reports `Matched`** — not the existing "every attestation shares
one `[NibRoster:…]`commitment" bullet, which is satisfied perfectly by a document on which no
signature matches any other.

**(amended 2026-08-23 — plan-review, Dan's call. `AcceptedPeer` names the previous SIGNING roster
entry, and the first signer gets a state of its own.)** The pin above said "predecessor in the
roster", and **six independent panel seats found that unsatisfiable by the criterion it was written
to discharge.** `crossBind` (`internal/p2p/attestation.go`, `crossBind`) sets `Matched` only when
*another valid signature on this document* holds the accepted fingerprint, and skips any attestation
whose `AcceptedPeer` is empty. Read literally, a `signs:false` convener at `roster[0]` is the
predecessor of the first signing party — and holds no signature — so that signature is `Matched:
false` **always, in exactly the configuration D22 introduced**. Reading it as "predecessor signer"
does not rescue it either: the first signer still has none.

Three things settle, and the third is the point:

1. **`AcceptedPeer` names the previous entry in the roster that has `signs:true`.** Non-signing
   parties are skipped rather than named, so the field always names somebody who is expected to have
   a signature on the document.
2. **`crossBind` stays document-scoped, unchanged.** The tempting repair — "`Matched` if the
   fingerprint is anywhere in the roster" — converts `Matched` from *a real valid co-signer's key is
   on this document* into *the record says so*, under which a **one-signature** document reports
   `Matched`. That is L2's silent downgrade arriving through the criterion written to prevent it, and
   it is refused here so no slice reaches for it under pressure.
3. **The first signer is reported as its own state — *first signer, no predecessor* — never as an
   unmatched attestation.** This is what makes 1 and 2 compatible. It also fixes a live UI defect the
   panel found underneath: `augmentSigDetails` (`web/app.js`) drops any attestation with a falsy
   `acceptedPeer` from `attested` **entirely**, so on an N-party document the first signer vanishes
   from the summary while `attested.every(a => a.matched)` can still be true — the verdict line prints
   over a document with an unaccounted-for signer.

**What discharges this, amended:** criterion 14 becomes *every signature **that has a signing
predecessor** reports `Matched`*, **plus** a counter-clause — an attestation naming a fingerprint that
is in the roster but has produced no valid signature must still report `Matched: false`. The
counter-clause is not decoration: without it, clause 2 above is unenforced and the weakening it
refuses is invisible.

*Amending D2 is owed through `/discuss`* — see the pin there.

### D23 — Order is derived from the record and enforced three times *(settled 2026-08-18 — UX pass)*

Signing out of order must be impossible in three independent places, each of which would catch it
alone. **L3** is the middle one, and it is the one that counts.

1. **The artifact refuses it.** A signature is an append: party *k*'s contribution can only be
   built on a document already carrying *k−1* of them, and `Initiate`'s prefix check
   (`internal/p2p/session.go:97`) binds what returns to what went out.
2. **The server refuses it (L3).** A contribution is accepted only when `ReadAttestations(doc)`
   equals the record's first *k* roster entries, in order, each valid and cross-bound, and the
   caller is `roster[k]`. Anything else is a **named refusal** — never a hang, never a silent
   no-op.
3. **The screen never offers it.** One ceremony panel, one enabled action, computed by the same
   function the server uses. Every other party reads *waiting for…* with the roster shown and the
   current position marked.

*Why three and not one:* layer 3 alone is decoration — this repo's habit is to put the gate in
Go. Layer 2 alone leaves the user discovering the refusal instead of never reaching it. Layer 1
alone cannot tell a *wrong* signer from a *late* one.

*What this deletes:* D13's role picker and the whole of D17's role-conflict machinery. Two people
picking the same role is only possible because roles are picked.

**(amended 2026-08-23 — plan-review, Dan's call: order stays fixed, and the reason it stays is now
written down, because it never was.)**

The legal-documents seat put the question D23 had not answered: **the three enforcement layers above
all justify *detecting* out-of-order signing, and none of them says why the order should be fixed in
the first place.** In practice execution order is usually not meaningful; where it is (a guarantee
executed after the principal obligation) it is a property of the instrument, not of the software. And
the cost is concrete: party 4 is available today, party 2 is on leave for a week, and nothing may
happen — parties 3 and 4 are refused by L3 though both are willing, present and named in the same
signed roster. If party 2's leave outruns `expires`, the ceremony ends and every signature already
collected is discarded (D25/D28's "start a new ceremony", whose real cost is stated there).

Cheaper than it looks, too: `hopBetween` numbers hops by **static roster position**, so every
rendezvous key, salt and seed derivation is already independent of who signs when. What actually
binds order is L3's *prefix* predicate and D22's predecessor rule — and both are being written from
scratch in P07.

**Order stays fixed, and the argument is not that flexibility is unwanted.** It is:

- **The loosening is forward-compatible; the tightening is not.** Prefix → subset can ship in any
  later phase, and every document signed under the stricter rule stays valid under the looser one.
  Waiting forecloses nothing. That is not true in the other direction, so the asymmetry decides it.
- **It couples to D22's 2026-08-23 amendment.** "The previous signing roster entry" is well-defined
  only under fixed order; with a free order `AcceptedPeer` must name the predecessor *in signing
  sequence* or the whole prior set — which reopens the field D22 has just settled, in the same phase,
  with L3 still unimplemented.
- **The prefix predicate is smaller.** "Signers equal the roster prefix" is one comparison; the
  subset form needs no-duplicates, each-valid, each-cross-bound and caller-has-not-yet-signed. L3 is a
  security law with no implementation yet, and the simpler correct thing is what gets built first.

**What this obliges instead, and it is not optional.** The convener must order the roster by expected
availability **at convene time**, and that order is permanent — so the boundary is **convene, not the
first signature** (P07 criterion 4 is re-pointed accordingly), and P06's convene copy must tell the
convener that order is fixed once invitations go out. A constraint that exists, is enforced three
times, and is disclosed nowhere is what this amendment refuses to leave standing.

**Deferred, with its trigger rather than as a road not taken:** relax L3 to *a subset of the roster,
no duplicates, each valid and cross-bound, and the caller is a roster member who has not yet signed*
— **when a real ceremony is blocked by roster order**, which is the evidence this decision currently
lacks in either direction.

### D24 — The hop is the unit of resumption; a signature is persisted before it is delivered *(settled 2026-08-18 — UX pass)*

A ceremony is interruptible because its state is the record plus the document, both on disk. The
unit of resumption is the **hop**, `(ceremony id, roster index)`.

- **Persist before delivering.** The contribution is written to `~/nib/ceremonies/<id>/` the
  instant it exists, *before* the frame that returns it.
- **A completed hop re-delivers; it never re-signs.** Re-signing would put a second block from the
  same identity on the page — wrong as a record, and per D25 wrong as a layout.
- **Resumption is idempotent and offline-first.** Reopening Nib renders *2 of 4 signed — waiting
  for Amir* from the local record with no network at all (D13 pin: a panel, not a modal).
- **The ceremony deadline (D16 clock 3) bounds it**, and it is the only clock shown in days.

**(pin: the persist must not tear, and it must not fail silently — 2026-08-18, gaps #15 and #17.)**
This decision makes "persist before deliver" the property that keeps a used key from vanishing, and
says nothing about the write itself. Two failures land on exactly that path:

- **A torn write.** Power loss mid-write leaves a partial contribution that reads as *a* contribution.
  The repo already has an atomic-write helper (added for the CLI's in-place edits, which chmods the
  temp file to the original's mode before the rename); **the ceremony's persist uses it**. *What
  discharges this specifically:* killing the process mid-write and observing that the mirror holds
  either the previous state or the complete contribution and never a prefix — not the re-delivery
  criterion, which never writes twice.
- **A full disk.** The signature exists and cannot be written, which is this decision's own defect
  with the mitigation removed. **The signature is held in memory, ~~the delivery is not attempted,~~
  **the delivery PROCEEDS (amended 2026-08-29 — Dan, option A)**, and
  the failure is reported as "signed but not saved — do not close Nib"** — the only honest sentence
  available, because closing Nib is what destroys it. Reporting it as an ordinary write error invites
  exactly that.

  **(amended 2026-08-29 — Dan, option A, after the P08.S02 deepdive measured the original clause
  unimplementable.)** "The delivery is not attempted" cannot be achieved by withholding the frame.
  `rd.Cached` is consulted **before** the consent gate — deliberately, so a reconnect does not re-ask
  the user (`internal/p2p/session.go:770-774`) — and `rd.Store` runs before the caller writes back
  (`:825-827`). So the sequence is: sign, cache, persist fails, frame withheld, initiator gets EOF,
  EOF is transport loss, the glare path **re-races**, and the reconnect hits the cache and returns the
  document *before it could reach the persist point again*. The peer is served one reconnect later,
  this machine still has no durable copy, and the failed write is never retried.

  Three shapes existed and each was wrong differently. Withhold-and-keep-the-cache is incoherent: it
  claims not to deliver, and delivers. Withhold-and-drop-the-cache makes the reconnect **re-sign**,
  producing the second block from one identity that this very decision forbids two bullets above and
  that P07's C01 exists to catch. Deliver-and-warn is the only coherent one.

  **So: the peer receives the signature they are owed** — it is real, it was consented to, and
  withholding it protects nothing — **and the SIGNER is told, in this decision's own words, that
  their machine kept no copy, with an action that lets them put one somewhere else.** What survives
  of this bullet is the sentence, not the withholding. `mirrorHop`'s own comment had already reached
  the same conclusion from the other side: *"failing the hop over it would discard a real signature
  to protect a copy of it."*

**The defect this exists to close, read at the line:** `p2p.Receive` builds the co-signed document
and *then* writes it back; if that write fails it returns the error
(`internal/p2p/session.go:135`) and the caller discards everything
(`internal/server/session.go:324`). **The user has signed and their machine keeps nothing** —
precisely the case `postConsentDeadline` was added to make rarer, and under resumption it turns
into "sign it again", which is the one thing a signing product must never ask. Note that the
one-way transfer path already persists what it accepts (`saveReceived`, `internal/server/session.go:338`);
the co-signing path is the one that does not.


**(plan-review pin: the disk-full clause and `mirrorHop`'s doc comment are opposite rules about one
write — 2026-08-29, seventh pass.)** This decision says the persist happens before the frame and that
a failed persist means *"the delivery is not attempted"*. `mirrorHop`, written at P07.S02a, argues
the reverse in as many words — *"failing the hop over it would discard a real signature to protect a
copy of it"* (`internal/server/ceremonyid.go:620-627`) — and is best-effort into a `log.Printf`.
Once P08.S02 makes them one write, both cannot stand. **Neither is edited here**; the spot is marked
and the choice is Dan's. Two facts the choice needs: `ReDeliverer.Store` returns no error today
(`internal/p2p/session.go:657-662`), so refusing the delivery is not expressible without widening
that interface; and the durable write is currently the ONLY copy a non-convener party has, since
`saveReceived` is not on the co-signing path at all. **The `internal/server/session.go:338` citation
above is stale** — `saveReceived` is now at `:915`.
### D25 — Signature pages are allocated from the roster, six blocks to a page *(settled 2026-08-18 — Dan, option A)*

**Dan's call, option A:** a fresh signature page per **six** blocks, appended in the pre-signing
pass, sized from the roster's signing count.

**Why this is forced rather than preferred — measured against the tree at v1.109.12, by running
the code rather than reasoning about it:** `stackPlacement` (`internal/p2p/cosign.go:68`) places
block *i* at `y = 40 + 96i` with height 84, on the last page — the readme page, whose body text
runs from baseline 735 down to **343**.

- **Block 3 — the *fourth* signature — spans y 328–412**, and the appearance is drawn by a later
  incremental revision, so it paints over five lines of the trust explainer the readme exists to
  provide.
- **Block 8 spans 808–892 on an 842 pt A4 page** and is simply off it.

So the current placement is correct for **three** signatures and silently wrong from the fourth:
no error, no refusal, a rendered document that has lost the text every signature covers. Two
parties never reach it, which is why it has never been seen.

*The constraint that dictates "allocated up front":* pages cannot be added after the first
signature without breaking it (D20's timing, same reason). **The number of signature pages is
therefore computed from the roster in the pre-signing pass** — a roster of nine gets two pages
before anyone signs. A ceremony cannot grow a party mid-flight; that is a new record, and the plan
would rather say so than discover it at signature ten.

*Six, not eight:* eight blocks fit an empty page arithmetically (block 7 tops out at 796), and
six leaves room for a page heading and a margin that is not a rounding error.

### D26 — A ceremony is proven by a multi-instance harness, and it is built at P02 *(settled 2026-08-18 — holistic pass)*

**Nib's test tiers stop at one process, so nothing in this repo can run a ceremony of any size.**
A new harness — N binaries, N `HOME`/`XDG_CONFIG_HOME` pairs, loopback rendezvous, driven
headlessly — is built as **P02's first slice**, and every ceremony-shaped criterion from P02
onward is driven by it.

**(Stage 2 grill amendment, 2026-08-18 — it drives the HTTP API, not N browsers, and it states
what it cannot see.)** Four Chromium instances per four-party run would be expensive and would buy
nothing the protocol criteria need: the ceremony's decisions are HTTP routes behind an unlocked
vault on loopback. **The harness drives the API.** Its ceiling, written in its own file exactly as
the other three tiers write theirs (`CONTRIBUTING.md`'s tier table; `verify_test.go` guards that
each states its own): **it cannot see the client** — the panel, its roster rendering, its
enabled-action computation and its locked-vault behaviour stay tier-3, single-instance. What it
delegates upward is the two-machine run, which stays Dan-only.

*Why this is a narrowing and not a weakening:* the criteria it must drive — a four-party ceremony
completing, an L3 refusal with the UI bypassed, a hop killed between signature and write, a party
still armed after three hops — are all **protocol** properties, and P07's L3 criterion explicitly
requires the UI bypassed. A browser-driven harness would satisfy that clause by driving the thing
the clause excludes.

*Read at the line, not inferred:* `build/uirepro.sh` sets **one** `HOME` and **one**
`XDG_CONFIG_HOME` (`:94`–`:110`) and runs **one** binary, so tier 3 is one process, one vault, one
identity. Tier 2 is jsdom with no server at all. Tier 1 covers the p2p layer properly —
`internal/p2p/session_test.go` runs both sides in one process over loopback — and has never seen a
server, a vault and a browser on both ends.

*The consequence for the plan as it stands:* P07's "a four-party ceremony completes", P06's
resumed panel with the network down, P05's party still armed after three hops — **sentences
nothing can execute.** The plan's answer so far has been the *Dan-only run* marker (plan-review
W4), which is right for a case needing two real networks and wrong as the answer for everything.

*Why P02 rather than P07:* P02 already carries two Dan-only criteria a **two**-instance harness
discharges outright (a ceremony completes over QUIC; the same over TCP). P03 is the first phase
that **cannot be honestly closed without one**, because discovery is multi-instance by definition.
And a four-instance harness is a two-instance harness with a bigger loop — building it late buys
nothing and leaves every phase in between unproven.

*What stays Dan-only, and this is not weakened:* anything needing two real networks — the NAT
mapping classes, port mapping against a real router, IPv6 between two sites, Windows multicast on
real Windows. The harness takes the **ceremony** off that list, not the network.

*Why this is a decision and not a slice:* the repo's own record. One sweep caught **four vacuous
greens**, three by their own setup assertions; another found that `go test ./...` cannot tell you
a test stopped existing. A criterion that can only be checked by hand is checked by hand once, and
this plan's headline criteria are all of that shape.

### D27 — The multi-party trust story: what a signer sees, and what the document says about itself *(settled 2026-08-18 — holistic pass)*

Three user-facing surfaces still describe exactly two people. All three carry the product's
honesty rather than its mechanics, so they move together in one change.

1. **The consent gate shows the roster, not one name.** `Confirmer.Confirm(peer SignerAttestation,
   doc []byte)` (`internal/p2p/session.go:51`, called at `:249`) takes **one** attestation. The
   fourth signer is consenting to a document three people have already signed, and the interface
   can hand them one of the three. It becomes a **slice** of attestations plus the record, and the
   consent screen shows every prior signer and their state. The screen a person reads before
   committing their key is the last place for a partial roster.
2. **The appended trust page is rewritten for N.** `readmeParagraphs` (`internal/p2p/readme.go:30`)
   says "signed by **two people**", "in **two-party signing** the second person's acceptance block
   is added after the first", and "how the **two people** know who they signed with". It is the
   page the signature blocks sit on and the text a stranger reads to learn what the document
   proves. **It cannot be edited alone:** `trustClaims` is a single source and a drift guard
   asserts every claim appears both in the rendered readme and in the in-app About dialog
   (`internal/p2p/readme_test.go:61`), so `web/index.html`'s About copy moves in the same change
   with the guard green.
3. **The visible block and the verifier's verdict name the ceremony.** `AppearanceLines`
   (`internal/p2p/attestation.go:98`) renders `Accepts: <label> [<short fp>]` — one counterparty —
   and the signature-details copy says each signature attests to *the other's* key. **D2's pin
   fixed the machine-readable half and left the half a human reads.** Both are re-worded to name
   the roster and the ceremony commitment. **(plan-review pin, 2026-08-18, adopted by Dan:
   `AcceptedPeerLabel` is the *local* pinned label — `Receive` takes it from the receiving side's own
   vault — so across a roster each block renders that signer's private name for their counterparty,
   and one document can call the same person three things. Harmless, because the fingerprint is the
   identity and the block already says to trust the signature details over the printed text — but
   this decision owns what the block says, so it says it.)**

*Why one decision rather than three slices:* they are the same claim told three times, and letting
them drift is how a document ends up asserting one thing in its signature and another on its face
— the precise defect `Attestation`'s own doc comment was corrected for (`attestation.go:30`).

**(plan-review pin: a non-signing convener has no verdict — 2026-08-18, adopted by Dan.)** The D20
pin puts **`signs`** inside `rosterHash`'s preimage, so a verifier reading a finished document sees a
roster entry with **no matching signature** — and the only shapes the verdict currently knows are
signed and missing. Rendering the convener as a missing signer would be wrong on precisely the roster
shape Dan asked for (D22, option A), and it would be wrong in the direction that matters: a document
reported as incompletely signed when it is complete. **Adopted:** the verdict distinguishes **roster
member not obliged to sign** from **obliged and absent**. **What discharges this specifically:** a
P07 criterion rendering the verdict for a completed ceremony whose convener has `signs:false` and
observing that it reads as complete — not the existing "the finished document contains no signature
of theirs" bullet, which is a fact about the *document* and says nothing about what the verifier
says.

### D28 — End states: a ceremony ends by completing, declining, expiring, or being abandoned *(settled 2026-08-18 — holistic pass)*

The plan has D19's four **network** failure causes and nothing at all for the failures that are
people. With N parties and a proceeding that can be interrupted, four end states exist, and each
gets a defined behaviour and its own message.

- **Completed** — every signing party on the roster has contributed and the delivery round runs
  (D22).
- **Declined** — a party refuses. Today that is one error string on one connection
  (`internal/p2p/session.go:254`) between two people who are talking anyway. **In a ceremony it
  ends the whole proceeding:** the record is marked declined, the convener learns it on the live
  channel, and **the parties who have already signed are told at delivery time**, because they are
  no longer connected to anything. A ceremony is never silently restarted around a refusal —
  re-running it is a new record, with a new id and new invitations.
- **Expired** — the ceremony deadline (D16, clock 3) passes. The record refuses any further
  contribution, and **the partially-signed document remains a valid document**: the signatures on
  it are real and always were. The panel says exactly that, rather than implying the signatures
  died with the ceremony.
- **Abandoned** — the convener never comes back. **There is no failover and no baton hand-off, and
  that is the decision rather than an omission.** A ceremony whose convener's machine dies is over;
  the parties who signed keep their partial document and convene a new ceremony. Recorded because a
  limit nobody wrote down is discovered by the person it happens to, at the worst moment.

Two adjacent states also get defined behaviour, because both are new with D24:

- **A party holding an intermediate document** between their own hop and the delivery round holds
  something signed by the roster's prefix, which is **not the artifact**. The panel labels it as
  in-progress and never presents it as finished.
- **A signer's identity changed** since the record was written — a re-enrolled vault. The pin no
  longer matches the roster, and today that surfaces as a generic handshake failure. It gets a
  named message and **ends the ceremony rather than accepting the new key**: accepting it would be
  exactly the substitution L1 exists to forbid, arriving through the front door.


  **(plan-review pin: three of the four end states have no home, and one is enforced on the wrong
  machine — 2026-08-29, seventh pass.)** *Declined* and *abandoned* cannot be recorded in the
  `Record`: every field is inside the convener-signed preimage (`internal/ceremony/record.go:154-197`,
  `:253-320`) and `ReadMirror` re-verifies on every read, so a field outside the preimage is an
  unauthenticated byte that `Decode`→`Encode` drops, and a field inside it changes `rosterHash` and
  invalidates every `[NibRoster:…]` commitment already collected. *Expired* has one enforcement door,
  `checkCeremonyDeadline`, whose only production caller is `handleSessionInitiate`
  (`internal/server/session.go:1550`) — the **dialing** side, which in a ceremony is the convener —
  while the party being asked to sign gates on `Record.Verify`, which deliberately does not refuse an
  expired ceremony and says so (`internal/ceremony/record.go:456-462`). So whoever convenes owns the
  only clock check. P08.S04 puts the expired gate at the signer's own door and makes the decline a
  separately signed termination object; **whether an end state may be recorded at all, and by whom, is
  a decision and is Dan's.**
### D29 — The ceremony's footprint: what it freezes, what it stores, and where *(settled 2026-08-18 — holistic pass)*

D20 gives the ceremony a record and D24 gives it a mirror on disk. Neither says what that does to
the user's document, their vault, or their machine.

- **The document is frozen while a ceremony is live.** D20 pins `docHash` and nothing stopped the
  convener redacting or rotating that tab afterwards, so a mismatch would surface at the far end as
  a refusal instead of at the edit — the worst place to learn it. **Mutating operations refuse on a
  document under a live ceremony and name the ceremony.** Per ADR-001 the ceremony pins the
  document id; this is the same law facing the other way. **(Stage 2 grill, 2026-08-18: the
  attachments route is one of them, and the refusal is server-side.)** `handleAttachmentAdd`
  (`internal/server/attachments.go:46`) calls `pdfops.AddAttachment` on whatever is open and
  commits the result, with **no signature guard** — and the control arm of caveat 10's measurement
  shows that on a signed document this yields `state=invalid` with every signer `valid=false`. The
  client does warn (`confirmSignatureLoss`, `web/app.js:3825`), which is why this is a gap in the
  freeze rather than a live defect — **but a client confirm is not a freeze**, by the same
  reasoning L3 uses for the contribution gate: a UI that merely asks satisfies nothing.
  **(pin: convening twice on one document is refused too — 2026-08-18, gap #28.)** The freeze stops
  *mutation* and said nothing about *convening*. Two records on one document would both carry a
  `docHash` that matches it, two rosters would both claim it, and the signature pages allocated by
  the first would be sized for the first roster alone. **A document already under a live ceremony
  refuses to start another**, by the same rule and with the same server-side placement. *What
  discharges this specifically:* attempting to convene on a document that already has a live
  ceremony and observing the named refusal — the mutation criterion cannot see it, because convening
  is not a mutation.
- **The invitation secret lives in the vault, not in the mirror.** `~/nib/ceremonies/<id>/` is the
  ordinary output directory — plain files, the same place `saveReceived` writes
  (`internal/server/session.go:338`). The vault is the encrypted store and already holds the
  identity key and the pinned peers (`internal/vault/vault.go:155`). D21's secret is the first
  genuinely secret value in this design and it goes where the other one is. **The mirror holds the
  document, the record and this user's own contribution — no key material.**
- **(plan-review pin: the pinned document id cannot survive the restart the ceremony is built to
  survive — 2026-08-18, adopted by Dan.)** The freeze bullet above says "Per ADR-001 the ceremony
  pins the document id". **ADR-001 scopes that id to one process**: "Document ids are a monotonic
  counter for the life of the process", and the ADR states that this is "not an implementation note;
  it is the load-bearing half", because a reusable id "defeats the law **silently and completely**".
  D24 makes a ceremony span quitting Nib. On the next launch the counter starts over, so a persisted
  docId either names nothing or **names a different document**, and every pinning check against it
  passes — ADR-001's own nightmare, one process boundary out where its guarantee does not reach.

  **Adopted:** the ceremony's identity of its document is **`(ceremony id, docHash)`** — both
  restart-stable and both already in the record. The docId pin remains a **within-process**
  mechanism, re-established when the ceremony is loaded and **never persisted**. **What discharges
  this specifically:** a P07 criterion resuming a ceremony **in a fresh process with other documents
  opened first, so the counter has advanced**, and showing it acts on its own document and refuses a
  decoy now holding the id it used to have — not the existing resumption bullet, which passes with a
  dangling id because nothing in it opens a second document.


  **(plan-review pin: this pin's own identity pair was refuted five days after it was written —
  2026-08-29, seventh pass.)** P07.S02b measured that `docHash` is a **convene-time** identity and
  wrote it down three times: *"a receiving party can never pass `CheckDocument`, at any hop —
  measured, not argued"* (`internal/ceremony/embed.go:186-206`), the same limit made load-bearing at
  `internal/ceremony/mirror.go:151-163`, and `CheckRecord`'s existence as the split. A visible
  signature adds a widget annot and `ContentDigest` hashes `/Annots`, so from hop 2 the in-flight
  document legitimately does not hash to `DocHash` — and hop 2 onward is the resumption case this
  pin exists for. **The pin is not struck here**; P08.S03 matches on the ceremony id read from the
  document's own convener-signed record and keeps `docHash` only in the unsigned window, and the
  supersession of this pin's text is Dan's.
- **(plan-review pin: the ceremony's document has two homes — 2026-08-18, adopted by Dan.)** The
  mirror holds "the current document bytes" (D24); the same document is open in a tab and may have
  been saved elsewhere by the user. Nothing said which is authoritative on resume. **The mirror is
  authoritative for the ceremony**; the open tab is a view of it; a divergence is **reported, never
  silently resolved** — silently preferring either one is how a party signs bytes they did not read.

- **The panel renders while the vault is locked.** Roster, position and next action are local and
  non-secret; the unlock prompt belongs at the moment of signing, not at the moment of looking. A
  resumption screen that demands a password to tell you whose turn it is has misunderstood what it
  is for.
- **Invitation pins are ceremony-scoped and disclosed.** Arming refuses an unpinned peer outright —
  "that peer isn't pinned" (`internal/server/session.go:441`) — so consuming an invitation *must*
  pin the roster (P01.S07). But pins are permanent, vault-persisted and user-visible: accept an
  invitation to a four-party signing you then decline, and three strangers are in your peer list
  for good. **Pins created by an invitation carry their ceremony and are removed when it ends,
  unless the user promotes them**, and the invitation screen names who is being added before they
  are.
- **The record is a visible attachment, and it is named.** Nib lists and extracts embedded files,
  so the record appears in the attachments panel whatever the plan says. **It is not hidden** —
  hiding it would be a second and worse surprise — it is labelled for what it is, and removing it
  is refused while the ceremony is live, for the same reason the document is frozen.
- **(plan-review pin: `nib watch` rewrites signed documents in place — 2026-08-18, adopted by
  Dan.)** Delivered ceremony documents land in `~/nib/signed/` (`saveReceived`), and `cmdWatch`
  (`internal/cli/watch.go`) polls a directory running `timestamp | optimize | sanitize` — where
  **`optimize` and `sanitize` rewrite the file in place**, which on a signed document invalidates
  every signature, exactly as caveat 10's control arm measured. It also processes **files already
  present when it starts**, so pointing it at that directory is immediately destructive. *Narrowed
  honestly:* `os.ReadDir` is **not recursive**, so a watch on `~/nib` reaches neither
  `~/nib/signed/` nor `~/nib/ceremonies/` — the exposure needs the user to name the subdirectory,
  which "process my inbox" makes plausible. **The fix belongs in `watch`, not in the ceremony:**
  `optimize`/`sanitize` refuse a **signed** PDF with a named message. That is a general improvement
  this plan merely makes likely, so it is carried as a `/pending` item rather than a phase of this
  plan — recorded here because the ceremony is what makes the directory worth watching.

- **Ended ceremonies are pruned.** The mirror is not a growing archive. A ceremony's directory is
  removed once it has ended and its document has been delivered or saved, and the panel offers the
  removal for one that ended without delivering. **(plan-review pin, 2026-08-18, adopted by Dan: a
  restored vault backup resurrects ceremony-scoped pins for ceremonies that have since ended. It is
  self-limiting — the pins name their ceremony and the mirror is gone — but the panel must not offer
  a ceremony it cannot load.)**

  **(plan-review pin: the lifecycle is end state → delivery → close-out, and pins drop at the END of
  it — 2026-08-18, adopted by Dan.)** Read with D28 and D22, the bullet above and the pin-removal
  bullet below break every ceremony's delivery. *Completed* is an **end state** whose own definition
  is "every signing party has contributed **and the delivery round runs**"; the convener delivers by
  **dialling each party**; and arming refuses an unpinned peer outright
  (`internal/server/session.go`, `handleSessionArm` — "that peer isn't pinned"). Drop the invitation
  pins when the ceremony *ends* and the finished document reaches **nobody**. The declined path is
  worse: D28 says the parties who already signed "are told at delivery time", so dropping the pins at
  the end state closes the one channel to the people who have already committed their signatures.

  **The lifecycle, stated once:** *end state → delivery round → close-out.* Pins are dropped and the
  mirror pruned at **close-out**, never at the end state. **What discharges this specifically:** a
  P07 criterion in which **a four-party ceremony's delivery round reaches every party and their
  invitation pins are absent afterwards** — one observation in the right order, which neither the
  existing delivery bullet nor the existing pin-scoping bullet can produce alone.

  **(plan-review pin: closing the document's tab is not a mutation, so the freeze above does not
  reach it — 2026-08-18, adopted by Dan.)** The sibling plan shipped *Close view*, and ADR-003 drops
  an inactive document's history whole. Closing a live ceremony's tab is **allowed and harmless** —
  the mirror holds the bytes and the panel is the ceremony's home — but every neighbour of this rule
  refuses, so a builder will reasonably guess this one does too. Said here so the guess is not made.

  **(plan-review pin: an abandoned ceremony is never pruned — 2026-08-18, adopted by Dan.)** The rule
  above waits on delivery, and D28's **abandoned** state is defined by the convener never coming
  back — so delivery never happens and that directory lives forever, on every party's machine, for
  every ceremony that dies quietly. The offer-in-the-panel half needs a person who is still looking.
  **The prune must also fire on time, independent of delivery:** `expires` plus a stated grace, which
  is bounded by D33's 30-day maximum. **What discharges this specifically:** a ceremony abandoned
  before its delivery round, whose directory is gone after expiry with nobody having touched the
  panel — not the existing "gone after the ceremony has ended and its document has been delivered or
  saved", which an abandoned ceremony satisfies vacuously by never reaching either state.

### D30 — The rendezvous is per hop, not per ceremony *(settled 2026-08-18 — holistic pass)*

D6 and D21 derive **one** rendezvous key from the ceremony secret; D22 makes connectivity a
**sequence of pairs**. Under one key every party's candidates land in one record, so the convener
has to work out whose is whose — and **every party can read every other party's IP addresses**, a
property the two-party design never had to state because there was only ever one other person.

**The rendezvous key is derived per hop:** ~~HKDF over the ceremony secret and the two participating
fingerprints.~~ **HKDF over THAT PARTY'S secret and the hop index (amended 2026-08-21, P05.S04 — Dan).**

**(pin, 2026-08-21: this decision's mechanism never matched the code, and its remedy did not reach
its own harm.)** Two corrections, recorded together because the second is why the first went
unnoticed. (a) The shipped `RecordKey(hop)` is `derive("nib-record-v1/hop-%d")` — the secret and the
hop NUMBER, with no fingerprint anywhere; a fingerprint appears only in `RecordSalt`, which is the
public BEP-44 addressing salt and not the key. (b) More importantly, the derivation as written here
would not have fixed the harm this decision opens with — *"every party can read every other party's
IP addresses"* — because every roster member holds the secret AND every fingerprint, so "both ends of
a hop hold all three inputs" is true of every other party too. **The fix is one secret per party**,
which under D22's hub means a secret shared by exactly the two ends of the hop it is for. What
remains, and it is D22 rather than a gap, is that the convener holds every party's secret. It costs nothing — both ends of a hop hold all three inputs by construction — it
scopes a candidate record to the pair that needs it, and it leaves D6's amendment exactly intact,
since the key remains uncomputable without the invitation.

**D17's race is scoped to the hop with it.** "Both sides publish candidates, both sides listen, and
both sides dial everything the other published" is correct for a pair; under a hub it must say *the
current pair*, or a convener dials candidates belonging to a party three hops away. The
lower-fingerprint glare tie-break likewise ranges over the hop's two fingerprints. A sentence
rather than a redesign — but an unsaid sentence here is a builder's guess, and D17 was written
when there was only one possible pair.

### D31 — ~~Two pairing paths, and the invitation is the default~~ **ONE pairing path: the invitation (superseded 2026-08-18 — Dan)** *(settled 2026-08-18 — taken by recommendation on Dan's behalf; **superseded the same day by Dan's own decision**)*

**Superseded: pairing by six-word name is retired, and the invitation is the only path.** Dan's
reasoning, and it is correct: the 66-bit pin is *strictly worse* than the 256-bit one and buys
nothing it does not. What the reversal below adds is that the thing worth keeping from the fallback
was never its pin — it was the **voice**, and that survives as D4's re-purposed verification string.

**What retires with it:** this decision's own second and third bullets, **caveat 5** (the 66-bit
floor), **P01.S03** (pin-by-name), and D4's *mandatory-exactly-when-the-pin-is-short* conditional
with the two assurance levels it created. **What does not retire:** the six-word name itself (D3),
now display-only, and therefore **P01.S01 and P01.S02**, which build the encoding and show it.

**And one obligation relaxes — see caveat 4.** With no name ever decoding to a pin, a wordlist
change no longer makes a name decode to a *different fingerprint*; it makes a display label change.
That is a breaking UI change rather than a silent authentication failure, which is a materially
smaller thing to promise forever.

*Retained below, unaltered, as the superseded decision.*

After D21 the plan has two ways to pair: an **invitation** carrying full fingerprints and a secret,
and the original **spoken six-word name** carrying ~66 bits. Both survive, and the plan now says
which is which — because until this decision it did not, and **L2's entire protection is
conditioned on which path was taken**.

- **The invitation is the default path.** The ceremony's primary screen convenes and invites, and
  never asks anyone to type a name.
- **Name-only pairing is a disclosed fallback**, behind the same advanced disclosure that holds the
  manual address (D9) and the hex fingerprint (P01, W7). It exists for the case this plan opened
  with: two people on a phone with nothing copyable between them.
- **The two paths are visibly different, in the user's terms.** The fallback screen states that the
  spoken check is required on it and why. A user must never arrive at the weaker pin without being
  told, which is L2 restated for the one choice that now selects between two assurance levels.

*What was weighed and not taken:* retiring name-only pairing would delete caveat 5's 66-bit floor,
C1's commitment step, the mandatory spoken string, P01.S03's pin-by-name and half of S04 and S05 —
a genuine simplification, and the largest one available anywhere in this plan. It was not taken
because the property it removes is the one the plan was written to have. **The six-word name is
kept either way**: it is the human identity (D3, D5), so P01.S01 and S02 are built regardless —
only *pinning* by name is what the fallback adds.

*Marked reversible, and what reversing costs:* this is one of three items in the holistic pass
taken on Dan's behalf rather than settled by him. Retiring the fallback strikes this decision's
second and third bullets, caveat 5, P01.S03's pin path, and the commitment work in P01.S04/S05 —
and nothing else. Nothing downstream is built on the fallback existing.

### D32 — Every format and protocol the ceremony introduces carries a version, and a skew is refused with a message *(settled 2026-08-18 — Stage 4 flat-spot pass)*

**The ceremony is the first Nib feature where two independently-updated installations must
interoperate**, and nothing in this plan — or in the p2p code it inherits — announces or negotiates a
version. Everything else Nib does is single-machine, or already-versioned, or a file format someone
else specified. A ceremony is four new interoperating surfaces at once.

Four things get an explicit version, and each is an **on-wire identifier**, immutable once shipped
(STANDARDS §9: "renaming them invalidates every deployed peer"):

| surface | version lives | on skew |
|---|---|---|
| **Ceremony Record** (D20) | a `v` field inside the signed record | a party that cannot read the record **refuses the ceremony by name**, never parses partially |
| **Invitation** (D21) | in the token's text form, before the payload | a reader that does not know the version says so, and says which Nib version it needs |
| **Ceremony protocol** — hop framing, role exchange, end-state messages | announced on the channel **before** the first frame | mismatch ends the hop with a named error, before any document byte |
| **`[NibRoster:<hash>]`** (D2 pin) | the tag itself, as `[NibCoSign:1]` already does | an unknown tag reads as "not one of ours", which `ReadAttestations` already does correctly |

**The rule that makes this worth a decision rather than a field:** *a version mismatch must produce a
sentence, not a parse error.* The failure this prevents is two people on a call, one of whom updated
last week, watching a ceremony die with `unexpected EOF` — the ladder's whole D19 diagnosis effort
made pointless by the one failure it does not cover.

*Why announce rather than negotiate:* there is no back-compatibility to preserve (this repo has zero
users and forbids shims), and a negotiation is a second protocol needing its own version. Announce,
compare, refuse.

*Not in scope:* the wordlist, which is frozen rather than versioned (caveat 4) — a version would
invite the swap the freeze exists to forbid.

**(plan-review pin: three of these four are owed at P01, not P07 — 2026-08-18, adopted by Dan.)**
This decision's only criterion is P07's version-skew bullet, and that is three phases too late for
two of the four surfaces. **The D20 pin's preimage axes begin "the format version (D32)"**, and
P01.S06 builds that commitment — so P01 would sign a commitment over a field the plan schedules at
P07, and adding a field to a signed preimage afterwards is exactly the migration-or-wipe question
(crypto pack PLAN-6). **Adopted:** the **record's** version lands in **P01.S06** and the
**invitation's** in **P01.S07**, each with its own criterion; only the **protocol announce** stays
in P07, where the channel it announces on first exists. **What discharges the P01 half specifically:**
a record and an invitation each carrying a version at the moment they are first written, driven by
reading it back — not P07's skew bullet, which tests two versions meeting and says nothing about
whether the field existed three phases earlier.

### D33 — The numbers, firmed here rather than at measurement *(settled 2026-08-18 — Stage 6, crypto pack PLAN-4/PLAN-5)*

Four bounds are stated in this plan as prose and nowhere as values. **An unstated threshold is
unfalsifiable — any observed value satisfies it** — and three of these four bound a security
property, so a criterion written against them today could not fail.

- **Candidate cap `N` = 8 per hop.** D6's pin makes the cap law and P04 drives it by "publishing
  N+50"; neither says what N is. Eight covers every tier the ladder can legitimately produce **(amended 2026-08-20: three, not five — P04.S04's address rule makes a private address unsignable and a typed manual address never enters a record, so LAN and manual candidates cannot exist. The constant is unchanged; its stated reason was stale.)**
  (LAN, v6, mapped, punched, manual) with margin, and bounds a third party's amplification to
  8 candidates × 2 hosts.
- **Total punch budget = 3,000 packets per ~~ceremony~~ HOP**, across all candidates, **per side ~~and both sides~~**
  — the ceiling D6's pin calls for and D16's backoff was sized against (~390/candidate at the
  stepped cadence). Exceeding it drops and **reports**; it never fails the ceremony.
  **(amended 2026-08-20 — Dan, option A, at P04.S04's slice grill. The unit was wrong and the two
  law figures contradicted each other.)** At the stepped cadence one hop with a full candidate set
  is 8 × 390 = **3,120 packets per side**, so a *per-ceremony* budget was exhausted inside the first
  hop: N = 8 was unreachable, and in a 31-hop ceremony hops 2–31 would get **zero packets**. Two
  figures both declared law cannot both be built.
  *Why the unit and not either value:* D16's clock 2 is "one connect deadline for the whole
  **ceremony's connection attempt**", and D22 makes connectivity a sequence of pairs — so a hop IS
  the connection attempt, with its own deadline and its own race. A packet budget scoped differently
  from the deadline it accompanies was the anomaly. The two rejected alternatives fixed only the
  arithmetic inside hop 1 and left the multi-party defect: cutting N to 3 fits one hop and still
  starves hop 2 (and breaks N's own coverage argument — eight was chosen to span LAN, v6, mapped,
  punched and manual with margin); cheapening the cadence to fit 31 hops into 3,000 yields ~6 packets
  per candidate, which is not a punch but a coin flip, since punching works by both sides
  retransmitting until their mappings coincide.
  ***"Per side" is the second half of the same correction, found by this slice's own code
  review.*** The original text said "across all candidates **and both sides**", which is
  (a) arithmetically inconsistent with the amendment above — under it one hop demands 6,240
  against 3,000, a **52% shortfall** rather than the ~4% below — and (b) **unimplementable**:
  there is no mechanism by which two peers share a packet counter, so neither machine could
  ever enforce it. The per-side reading is the only buildable one and the only one the
  arithmetic uses. The ceremony-wide worst case is therefore 31 hops × 3,000 × 2 sides.
  *The 3,120-vs-3,000 gap inside one hop is a rounding artifact, not a conflict* — the budget drops
  and reports, so the tail of the last candidate's retries is trimmed by ~4%, which is the mechanism
  working as designed.
  *What this re-opens, and where it is closed:* 31 hops × 3,000 is 93,000 packets per ceremony. That
  number exists only because the **roster maximum below is documented here and enforced nowhere in code**, so
  P04.S04 builds it and the ceiling becomes stated and bounded rather than emergent. Accepted on the
  ground that supplying candidates at all requires the invitation secret, so the actor is a roster
  member or an invitation interceptor — never an anonymous internet reflector.
- **Ceremony deadline maximum = 30 days.** D16 clock 3 is "days, convener's choice", which is an
  externally-supplied security parameter with no bound: it governs how long a listener may arm, how
  long invitation-scoped pins persist (D29), and how long a mirror lives. A convener setting ten
  years is a config away today.
- **Roster maximum = 32 parties.** D25 allocates signature pages from the roster length, so an
  unbounded roster is an unbounded page count; 32 is six pages and is far past any real signing.

*All four are enforced, not documented* — the externally-loaded path is the guarded one, which is the
half PLAN-4 exists for. ~~*All four are tunable, not law*, in the sense D16 already defines: the
structure is the law, the value is a constant.~~ **(amended 2026-08-19 via /discuss — Dan.) Two are
law and two are tunable.** `N` (the candidate cap) and the punch ceiling sit with the **structure**,
because under D6's pin an attacker supplies the candidates and a bound an operator can raise is not a
bound. The **ceremony-deadline maximum** and the **roster maximum** are tunable constants, in the
sense D16 defines: the structure is the law, the value is a constant. **What discharges this: a guard
that fails if either law figure is reachable from the tunable block** — not the P07 bullet driving a
value past each bound, which passes identically whichever file the constant lives in.

**(plan-review pin: this contradicts the D16 pin on a security bound — 2026-08-18, adopted by Dan.)**
The sentence above says all four are tunable. **The 2026-08-17 pin on D16 says the opposite about two
of them** — the candidate cap and the global packet ceiling "are **law, not tunable** — the
distinction the table's own preamble draws" — and its reasoning is that under the D6 pin *an attacker
supplies the candidates*. Two decisions settled a day apart give a builder opposite instructions about
where a security bound lives.

**The split, adopted:** `N` (candidate cap) and the punch ceiling are **law** — they sit with the
structure, not in the tunable constant block. The **ceremony-deadline maximum** and the **roster
maximum** are tunable constants, as this decision says. **What discharges this specifically:** a guard
that fails if either of the two law figures is reachable from the tunable block — not the P07 bullet
driving a value past each bound, which passes identically whichever file the constant lives in.

*The amendment to this decision's own sentence is owed through `/discuss`* — a plan-review pass marks
the spot and never rewrites a decision.

### D34 — The ceremony's outbound calls are enumerated, and the DHT is disclosed *(settled 2026-08-18 — Stage 5, STANDARDS §9)*

STANDARDS §9 says Nib sends nothing by default and that "any other outbound call is a deliberate,
flagged decision". Today Nib's entire egress is one opt-in GET to the releases API. **The ceremony
adds five kinds of outbound traffic**, and the plan flags exactly one of them.

Enumerated, one call at a time, as the standard asks:

1. **Multicast announce/browse** (tier 1) — link-local only, armed-only.
2. **DHT bootstrap + KRPC** (D6) — to a cached node list, armed-only.
3. **DHT publish/fetch** of the encrypted candidate record (D6, D30).
4. **Port-mapping requests** — PCP/NAT-PMP/UPnP-IGD to the local gateway (D15), armed-only.
5. **Punch datagrams and session traffic** to the peer's candidates (D8).

**The disclosure gap this pass found:** D15 already makes Nib *tell the user* it asked the router for
an opening and name the port — Dan's option A, and right. **Nothing discloses the DHT**, which is the
larger disclosure by a distance: a ceremony publishes a record to a **public, global, third-party
network** and contacts strangers' nodes. Under D6's amendment the record is encrypted and its key is
uncomputable without the invitation, so the *content* is protected — but participation is not
private, and the user should be told in the same breath they are told about the router.

**So: the ceremony screen discloses, while armed, that Nib is using the BitTorrent DHT to find the
other party**, in one plain sentence, beside the router line. Not a consent gate — the ceremony
cannot work without it and a modal per ceremony is friction for no decision — a disclosure, which is
what §9 asks for.

*Also adopted from §9:* **self-healing.** A corrupt or unreadable ceremony record degrades the panel
to "this ceremony's record could not be read" and leaves every other ceremony and document working.
It never blocks startup, and it never requires the user to delete state by hand.

**(pin: widen "corrupt" to "unreadable or absent" — 2026-08-18, gaps #18 and #19.)** The rule above
was written for a *corrupt record* and two neighbouring situations fall outside its wording. **A
mirror directory deleted by hand** is not corruption — it is absence, and the panel must degrade the
same way rather than fail on a missing file. **A locked vault** is the third state: the panel renders
from local, non-secret state (D29), so a ceremony whose party has not unlocked is *legible but not
actionable*, and it must say which. There is one consequence worth naming for the far side: a party
who has not unlocked **is not armed**, so the convener sees D19's cause 1, "hasn't started their
ceremony yet" — which is true and is the best available answer, since nothing on the convener's side
can distinguish a locked machine from an absent one.

### D35 — A disagreeing clock is diagnosed from the certificate we already hold *(settled 2026-08-18 — grill of gap #16)*

**The plan runs three clocks, a 30-day deadline, candidate expiry and signature timestamps, and had
never mentioned the system clock being wrong.** Two failures hide behind that, at different
magnitudes, and the smaller one bites far more often.

**The effective tolerance is five minutes, not fifteen.** `mintTransportCert`
(`internal/p2p/transport.go`) sets the ephemeral leaf's `NotBefore` to `now − transportSkew` (5 min)
and `NotAfter` to `now + transportTTL` (15 min), and `verifyPinnedPeer` rejects outside that window.
A verifier may therefore be up to **15 minutes ahead** of the minter but only **5 minutes behind** —
and because **both sides verify each other's leaf**, whichever side is behind is the one that fails.
The pair's usable budget is the tighter direction: **±5 minutes**.

**Beyond that the handshake fails, and today it fails as D19's cause 4 — "unexplained".** A laptop
that slept, a VM, or a machine whose NTP is blocked by the same corporate firewall that blocks the
DHT's UDP: two people on a call watch the ceremony die with "couldn't establish a connection", and
nothing on either screen mentions time.

**The decision:** *do not widen the window; diagnose it.* Widening `transportSkew` would loosen the
replay bound on the ephemeral key — a security parameter traded away to paper over a message.

**Where the diagnosis comes from, and this is the grill's finding:** the check that rejects
**already holds the peer's certificate**, parsed, with its `NotBefore` and `NotAfter` in hand. Those
two values *are* the peer's clock. So a validity-window rejection reports **how far apart the two
clocks are and in which direction**, derived from bytes already in memory — **no extra round trip,
no new wire field, and no channel, which matters because the handshake is exactly what has failed.**

**D19 gains a fifth cause: the clocks disagree** — plain language first ("this machine's clock is
about 12 minutes behind the other side's"), technical detail behind the disclosure. It is the only
cause of the five that names a fix the user can perform in ten seconds.

*The second failure, and it is the lesser one:* against a **days-long** ceremony deadline a party
whose clock is a month out silently expires a live ceremony or honours a dead one. Once a channel
exists this is cheap to catch — the same comparison, on a value the peers can now exchange — and the
same fifth-cause message covers it. **Deliberately not solved by anchoring:** an external time
source (OpenTimestamps, a TSA) would put a network dependency inside the one design whose premise is
that it has none, and it would not help the handshake, which fails first.

*Also considered and rejected:* making the convener's clock authoritative. It is not a new trust —
the convener already chooses `expires` — but it does not help a party judge whether *it* is inside
the window, because that judgment needs a local `now` whatever the reference is. **The honest
position is that a wrong clock cannot be fixed from outside the machine; it can only be detected and
named.** That is what this decision does.

---

## Build order

**Phase numbers are identifiers, not the order. (Stage 2 grill, 2026-08-18.)** They are
referenced by every pin in this document and by the project memory, so they are *not* renumbered.
The build order is: ~~**P00 → P01 → P02 → P03 → P04 → P05 → P07 → P06.**~~
**P00 → P01 → P02 → P03 → P04 → P05 → P07 → P08 → P06 (2026-08-18, P07 split).**

*Why P07 precedes P06:* P07 is **model** work — the roster-prefix gate, the carry route for a
non-signing convener, `Confirmer`'s shape, the placement policy, hop persistence, the end-state
machine. P06 is the **view**. Every item in P07 changes something P06 would have rendered, so the
original order builds the surface twice and reviews it twice. P07 does not need P06 in return: its
criteria are driven by D26's API harness, and its L3 clause explicitly requires the UI bypassed.
*The cost, stated rather than discovered:* the plan's first user-visible output moves one phase
later, extending an already-long invisible stretch (P02–P05). That is a real cost and it is the
smaller one.

### P00 — Bootstrap *(pre-satisfied — see D1)*
nib is a mature repo with VERSION, CLAUDE.md, release machinery and git history.
Stage 8 scaffolding would write `0.1.0` over a shipping product. Recorded as
pre-satisfied, exactly as the sibling plan records it.

### P01 — Pairing identity: the name, the record, and the invitation **(amended 2026-08-18)** *(done 2026-08-19, v1.109.48)*

**Phase close, 2026-08-19.** Ledger: **14 met / 0 not met / 2 not exercised**, both waiting on
P07's convene-and-decline machinery and both recorded rather than collapsed — the
accept-then-decline *flow* (9c) and "after a ceremony is armed" (10's second half). The
properties they protect are guarded now; the paths through the UI are not.
**The close found a criterion that had never been built**: invitation-scoped pins (D29). No
test could fail for a feature nobody wrote, and only walking the criteria verbatim found it.
Built at the close, red-proved twice. **Caveat 11 discharged** at S07's grill (HKDF, not a
PAKE). **Caveat 10 discharged empirically** at S06 — and the record's `docHash` was
re-specified in the same slice, because pdfcpu's rewrite is not idempotent and a byte hash is
not recomputable by anyone but its writer. Review:
`<project-memory>/code-reviews/P01-phase-close-2026-08-19.md`.
Goal: replace the 64-hex exchange with a six-word name, and ~~establish the
verification string as a mandatory gate~~ **establish the verification string as a gate conditioned
on the pin's strength (corrected 2026-08-18, plan-review, adopted by Dan — see D4's supersession and
this phase's third exit criterion)** — both in one phase, because shipping the
shortened name without the spoken check would be the silent downgrade L2 forbids.

**(plan-review pin: the goal contradicted its own exit criterion — 2026-08-18, adopted by Dan.)** The
goal read "mandatory gate" while the third criterion below supersedes exactly that, and a goal is what
a builder reads first and returns to. The correction above is to the *phase's* text, not to a
decision — D4 carries the decision and is untouched.
**Amended 2026-08-18 (D20, D21): the phase also builds the Ceremony Record and the invitation,
because they are what the name attaches to and what makes the check conditional rather than
mandatory.** Connectivity is untouched; this phase still runs over the manual address.

**Entry condition — the Stage 2 grill runs before this phase is built. (added 2026-08-18,
holistic pass.)** "Stage 2 has still not run" has now appeared in **six datelines**, and "a Stage 2
grill target" is how twelve open questions have been parked — including **caveat 11**, the channel
binding, which is the largest unreviewed cryptographic surface left since D12 was withdrawn. A gate
deferred to twelve times and scheduled zero times is precisely the defect D12's own plan-review pin
named ("a gate named in no phase is not a gate"), and D12 is gone, so this is the only gate this
plan still has. *Taken by recommendation rather than referred to Dan: this is the plan's own
process, not risk appetite.*

Exit criteria:
- A user never sees a hex fingerprint in the normal pairing path, and never types one — **settled 2026-08-17 (plan-review, W7): "normal pairing path" means the default pairing screen; hex is reachable only behind the advanced disclosure, never merely de-emphasised in place.** P01.S02's "hex moves to a secondary position" admitted a second reading in which hex is still on screen, and two builders would each have cited the plan.
- ~~**An outside cryptographic reviewer has read the pairing and verification design, including the commitment step, before P02 opens.**~~ **Struck 2026-08-18 — D12 withdrawn on Dan's instruction. This phase carries no external gate.**
- ~~A ceremony cannot reach the signing exchange until both sides confirm the verification string.~~ ~~**Superseded 2026-08-18 (D4): a ceremony paired from a six-word name alone… the *conditioning* is what is driven.**~~ **Re-superseded 2026-08-18 (D4's second supersession, D31): the verification string is offered on every ceremony and required whenever a voice channel exists, and the commitment step is unconditional. Driven by an out-of-order reveal on an ordinary invited ceremony — the case the earlier wording scoped to a path that no longer exists.**
- Peers pinned by hex before this phase still work (D10), proven by a test that pins the old way and connects the new way.
- The name↔fingerprint encoding round-trips on a fixed vector corpus.
- **A Ceremony Record round-trips through the document: written in the pre-signing pass, readable after N incremental signatures, and its convener signature verifies. (added 2026-08-18, D20 — this is caveat 10 discharged empirically, not by reading pdfcpu's documentation.)**
- **An invitation pins the full 32-byte fingerprint, and a ceremony built from one never derives a pin from the six-word name — ~~driven by an invitation whose name and fingerprint disagree, which must be refused rather than resolved either way~~ **driven by the roster entry being unable to CARRY a name to disagree with (amended 2026-08-22, Dan's call — the evidence clause is relocated into the type, not struck).** (added 2026-08-18, D21.)** The requirement is unchanged and the first half is driven exactly as written; what moved is what discharges the second half. `ceremony.Party` carried a `Name` field that was JSON-serialized, written by nobody and read by nobody: `MatchesRecord` compared `Fingerprint` and `Signs` and never `Name`, so the refusal this clause demands **could not happen**, and no test could fail for its absence — the criterion was ledgered met at P01's close on the strength of the half that was implemented. The name is a pure function of the fingerprint, so storing it beside the fingerprint states one fact twice in a shape where the two can disagree; the field was **deleted** (v1.117.41) and any consumer derives the words with `pairing.Name` from the fingerprint the roster already carries, as `internal/server/peers.go:nameOrEmpty` already does for pinned peers. Nothing signed moved — it sat outside every preimage. **Why this is an amendment and not a weakening:** unrepresentable is stronger than refused. A check can be forgotten at a new call site; a type with no such field cannot express the disagreement at all, and `TestEveryPartyFieldIsInTheCommitment` now goes red in the same commit as any future field added to a roster entry without entering the commitment — which is the general case the old single-field exclusion test could not see. **The cost, stated rather than left to be discovered:** the literal evidence path ("construct a disagreeing invitation and observe the refusal") is no longer constructible, so this clause is discharged structurally and P07 inherits the obligation that a display name it renders is **derived at the point of display**, never carried in the roster. The directly analogous case is P01.S01's transposition clause, relocated on the same reasoning and also on Dan's call.
- ~~**The default pairing screen offers the invitation; name-only pairing is reachable only behind the advanced disclosure…**~~ **Superseded 2026-08-18 (D31 collapsed): there is no second pairing screen to place anywhere. Replaced by:** **no screen anywhere accepts a six-word name as a way to pin a peer — driven by attempting it and observing the refusal, not by observing its absence from the default screen. (2026-08-18.)** An absence is satisfied by hiding the field; a refusal is satisfied only by removing the path.
- **A pin created by consuming an invitation is marked with its ceremony and is gone when the ceremony ends; a pin the user promoted survives. (added 2026-08-18, D29.)** Driven by accepting an invitation and then declining the ceremony — the case that otherwise leaves strangers in the peer list for good.
- **The invitation secret is never written to `~/nib/ceremonies/`, driven by searching the mirror for it after a ceremony is armed. (added 2026-08-18, D29.)** A test that asserts the vault *contains* it cannot see the copy left on disk beside the document.

#### P01.S01 — The wordlist and the encoding *(done 2026-08-19, v1.109.38)*
Scope: fingerprint → six words → fingerprint bits, one package, no UI. Refs: D3, D4.
Acceptance:
- Round-trip holds for a corpus of fixed vectors, including all-zero and all-ones fingerprints.
- Decoding rejects a wrong-length phrase and an out-of-list word, ~~and a transposition,~~ each with a distinct error.
- **A transposed name does not match the fingerprint it was derived from — driven through `Matches`, not through the decoder. (amended 2026-08-19, slice grill, Dan's call to take the best-and-most-correct shape.)** The clause is **relocated, not struck**. Six words from a 2048-word list is 66 bits with no room left over, so *every* six-word phrase is a valid encoding and a decoder has nothing to reject a transposition **with**; detecting it inside `decode` costs checksum bits. Under D31 nothing decodes a name to a pin, so those bits would be spent protecting input no user can supply, while the payload they came out of is the one property the name still has — grinding resistance for a display identity, 2⁶⁶ against 2⁵⁵ if a word were spent. The property survives intact in the comparison, which is where it is actually exercised: a human reading six words aloud sees order directly, and `Matches` returns false.
- The wordlist's licence is recorded in THIRD-PARTY-NOTICES.md if it carries one.
- **The wordlist is frozen: a checksum over the list file is asserted by a test, and changing any word fails it. (added 2026-08-17, plan-review, caveat 4 pin; caveat 4 RELAXED 2026-08-18 — the guard stays, the obligation is now "not without a version bump and a note" rather than "forever".)** The fixed-vector corpus above cannot serve — it is computed from the list and moves with it.
- **The list is constructed, not vendored, and its phonetic distinctness is measured rather than inherited. (added 2026-08-19, slice grill — Dan's call, option B.)** Caveat 4 asks for "phonetic distinctness good enough to read over a phone" and a vendored list can only assert it. A generated list carries its own selection constraints as guards.
Tasks:
- **T01 — Source and construct the list.** A checked-in generator producing a frozen 2048-word `wordlist.txt`, from public-domain / permissively-licensed corpora, with provenance recorded. The generator is kept for the record and **is not run at build time**; the artifact is the source of truth.
- **T02 — List-quality guards.** Exactly 2048 entries, all unique, lowercase ASCII, 3–8 characters, unique 4-character prefixes, no homophones, and a minimum phoneme-level edit distance across every pair. **These exist because a freeze guard freezes a mistake exactly as well as a good list** — T03 alone would pin a duplicate forever.
- **T03 — The freeze guard.** SHA-256 over the embedded file against a literal, with the failure message carrying caveat 4's relaxed obligation rather than "forever".
- **T04 — `Name(fp []byte) (string, error)`.** The leading 66 bits, big-endian from byte 0, as six 11-bit indices. The bit selection is pinned by vectors, not by comment.
- **T05 — `decode` (unexported) and `Matches(fp, name) bool`.** Distinct errors for wrong-length phrase and out-of-list word. **The decoder is deliberately not exported**: a public function returning 66 bits invites a caller to treat them as an identity, which is the misuse D31 just removed.
- **T06 — The fixed-vector corpus.** All-zero and all-ones hand-derived (`word[0]`×6 and `word[2047]`×6), plus at least one hand-computed vector from a real fingerprint; round-trip in both directions. Vectors generated by running the implementation would assert only that the code equals itself.
- **T07 — Seam inventory rows.** S01 is a pure library with no runtime path, so its rows are **test-only observables**, recorded as such rather than inventing counters no one would read.

#### P01.S02 — Show the user their own name *(done 2026-08-19, v1.109.41)*
Scope: the peers payload and the UI display the name; ~~hex moves to a secondary position~~ **hex moves BEHIND AN ADVANCED DISCLOSURE (pinned 2026-08-19, slice grill — to W7, not to the weaker reading)**. Refs: D3, D5.
Acceptance:
- `peersPayload` returns the name alongside the existing fingerprint; nothing existing is removed.
- ~~The name shown is derived from the live identity, not stored — deriving twice yields the same words.~~ **Re-specified 2026-08-19 (slice grill): the clause as written could not fail.** Deriving twice yields the same words *whether or not the name is stored* — a stored name is stable too — so the stated observable cannot see the property it names. **The name is asserted where the property is decidable: the payload's name equals a derivation the test performs itself, and `vault.PinnedPeer` carries no field that could hold one.** *(re-specified twice — the second attempt was worse than the first.)* Reading the vault file for the words, the obvious replacement, is **satisfied by the encryption rather than by the design**: the vault is encrypted at rest, so nothing appears in it in plaintext and the assertion passes with the name stored. The derivation check catches a stored name that has *drifted* from what the key encodes — the failure that matters — and the structural check catches the storage being introduced at all, including on the day it still agrees.

**(slice-grill scope note, 2026-08-19 — what this slice does NOT claim.)** The phase's first exit criterion is *"never sees a hex fingerprint … **and** never types one"* (W7). **S02 owes the seeing half only.** The typing half needs the invitation: `#peerPaste` is currently the *only* way to pin anyone, so removing it here would leave no pairing path at all. P01.S07 replaces it, and the criterion is met at phase close rather than here. Recorded so a phase-close ledger does not read S02's green as covering both halves.

**(slice-grill finding: five display sites, not one.)** The deepdive found the user's own hex rendered in **two** surfaces (`#peerSelfFp` in Identity & peers, `#srvSelfFp` in the listen/serve dialog) and peer hex in **four** (the peers list and three `<select>` labels). A slice built against "the UI" leaves the serve dialog — itself a pairing surface — showing raw hex while the criterion reads as met.

Tasks:
- **T01 — `pairing.Name` into `peersPayload`**: a `Name` field on `peersResponse` and on each `peer`. Nothing existing removed.
- **T02 — the not-stored assertion**, read from the vault file on disk rather than through the API.
- **T03 — the five client render sites.** No second cached global: `selfFingerprint` is already written by two loaders and a parallel `selfName` doubles a thing that is only benign because both writers agree. Read the name from the payload where it is rendered.
- **T04 — hex behind a `<details>` disclosure**, both self surfaces and the peer rows. **The `<option>` values and the Copy buttons are untouched**: the value is the addressing key posted to the co-sign and serve routes (and L1 forbids a name resolving a pin), and hex is still how a peer pins you until S07.
- **T05 — tier-2**: the default pairing view carries no 64-hex string in its rendered text, and hex appears once the disclosure is opened. Absence *and then* presence, because absence alone passes if the panel never rendered.
- **T06 — seam inventory rows.**

#### ~~P01.S03 — Accept a name wherever a fingerprint is accepted~~ **STRUCK 2026-08-18 — Dan's collapse to one pairing path (D31). No name decodes to a pin, so there is nothing for this slice to build.** *(retained below)*
#### P01.S03 — Accept a name wherever a fingerprint is accepted *(struck 2026-08-18 — retained text, not a slice)*
**(marker added 2026-08-19: the retained copy carried no status marker, and `/createcode` resumes by scanning top-down for the first slice heading WITHOUT one — so a resume would have selected a struck slice and started building the pin-by-name path D31 removed. The strike above was already unambiguous to a reader and invisible to the scan.)**
Scope: `parseFingerprint`'s callers accept a six-word phrase and resolve it to a pin. Refs: D3, D10.
Acceptance:
- Pinning by name and pinning by the equivalent hex produce a byte-identical stored pin.
- A name that decodes to a fingerprint no peer presents fails at the handshake, not at pin time — L1 holds: nothing about reachability decided identity.
Tasks: *(written at slice-grill time)*

#### P01.S04 — The verification string *(done 2026-08-19, v1.109.43)*
Scope: derive four words from the completed session and display them on both sides. Refs: D4.

**(slice-grill notes, 2026-08-19.)**
- **The commitment is the slice, not a detail of it.** Three of the four acceptance bullets are satisfied by *any* per-session derivation; only the fourth — the out-of-order reveal — can see the 2²² birthday search D4's pin describes. It is built first and everything else hangs off it.
- **One 32-byte uniformly random contribution serves as its own nonce.** D4's pin writes the commitment as `H(nonce ‖ contribution)`, which anticipates a structured or low-entropy contribution; 32 random bytes carry 256 bits and hashing them is preimage-resistant on its own. Recorded as a deliberate simplification of the pin's *form*, not of its substance — what the pin requires is that the string derive only over values committed before either side saw the other's, and that is unchanged.
- **The string binds the channel as well as the contributions.** D4 says "both identity keys plus the handshake transcript"; `ExportKeyingMaterial` is what makes the transcript half true, and it is also what lets D18's clause (a confirmation computed on one channel is rejected on any other) be met at S05 rather than needing a second mechanism then.

Tasks:
- **T01 — `pairing.Verification`**: four words (44 bits) from a digest, reusing S01's list and bit-packing. The 6-word and 4-word renderings share one packer or they will drift.
- **T02 — the commit/reveal exchange** in `internal/p2p`, before any document bytes: commitments both ways, then reveals, each checked against the commitment it was promised by.
- **T03 — the derivation**: `KDF(fpA ‖ fpB ‖ cA ‖ cB ‖ exporter)`, ordered canonically so both sides compute the same value from opposite viewpoints.
- **T04 — the out-of-order harness**: a peer that reveals a contribution not matching its commitment is refused *before any string exists*, driven rather than asserted.
- **T05 — the MITM case**: two legs, two different peer identities, and the two strings must differ.
- **T06 — seam inventory rows.**
Acceptance:
- Both endpoints of one session derive identical words.
- Two sessions between the same pair derive different words.
- A test that substitutes a different peer identity produces different words on the two sides — the man-in-the-middle case, driven rather than asserted.
- **The string is derived only over committed values, and a peer that reveals its contribution after receiving the other's is rejected before any string is derived — driven by a harness that replays the exchange out of order. (added 2026-08-17, plan-review C1, D4 pin.)** The three bullets above are all satisfied by a design a man-in-the-middle can birthday-grind at ~2²²; this one is the only clause that can see it. **(retained 2026-08-18: it now scopes to the name-only path, which is the only one where C1's precondition still exists. See D4's amendment — the criterion is kept, not narrowed away, because the path is kept.)**
Tasks: *(written at slice-grill time)*

#### P01.S05 — Make it mandatory ~~when the pin is short (amended 2026-08-18, D4)~~ **— unconditionally (re-amended 2026-08-18, D4's second supersession)** *(done 2026-08-19, v1.109.45)*

**(slice-grill notes, 2026-08-19.)**
- **Four entry points carry document bytes, not one.** `Initiate`, `Receive`, `SendDocument` and `ReceiveDocument`. The clause is "no document bytes cross the wire before both confirmations are recorded", so the gate goes on all four; a guard that enumerates them needs a population floor or a fifth path added later inherits nothing.
- **The confirmation is not a token, so "rejected on any other channel" cannot be a replay test.** D18's clause reads as though a confirmation could be carried; there is nothing to carry — it is a local boolean. What is testable is that a reconnect **requires a fresh confirmation**, because the string is bound to the channel through the exporter. Driven by counting confirmations across a reconnect.
- **This changes the wire ordering, and two nibs of different versions will not interoperate.** Recorded rather than versioned: the session is nib-to-nib, both ends are the same product, and a clean failure on a version mismatch is the honest outcome. Noted so P08's delivery work does not discover it.
- **F2 from S04's review is discharged here**: `verificationExchange` assumes conn already carries a deadline, and the callers this slice writes set `exchangeDeadline` before anything crosses the wire.

Tasks:
- **T01 — the `Verifier` gate**: an interface both sides call with the four words, plus distinct `ErrVerificationDeclined` / `ErrVerificationTimedOut` so a refusal never reads as a network failure.
- **T02 — wire all four entry points**, verification before any document byte.
- **T03 — the L2 guard**: for each path, a declining verifier produces an error AND the peer receives zero document frames — with a source-scan population floor so a fifth path cannot be added without one.
- **T04 — the reconnect drive**: a second channel requires a second confirmation, and the two strings differ.
- **T05 — the server bridge**: a `sessionVerifier` beside the existing consent bridges, parking the words for the UI on the same pattern.
- **T06 — seam inventory rows.**
Scope: the ceremony fails closed until both sides confirm. ~~— always on the name-only path, and until the machines confirm on the invited path.~~ **One path, one rule: the string is offered on every ceremony and required whenever the parties have a voice channel, and the commitment step is unconditional.** Refs: D4, D11 (L2), **D21**, **D31**.
Acceptance:
- No document bytes cross the wire before both confirmations are recorded.
- Declining, or timing out, ends the session with a distinct, user-legible outcome — not the same error as a network failure.
- A guard test named for L2 fails if any path reaches the signing exchange unconfirmed.
- **A confirmation computed on one channel is rejected on any other, driven by reconnecting mid-ceremony rather than asserted. (added 2026-08-16, D18)**
- ~~**The check is conditioned on the pin's strength, not on the flow…**~~ **Superseded 2026-08-18 (D4's second supersession, D31): there is one path and one pin strength, so there is nothing to condition on. Replaced by:** **the commitment step is unconditional — a peer that reveals its contribution after receiving the other's is rejected before any string exists, driven on the ordinary invited ceremony rather than on a fallback. (2026-08-18.)** The out-of-order-reveal criterion in P01.S04 was scoped to the name-only path; that path is gone and the criterion is not — it moves here and applies always, because the attacker it stops needs only a defeated delivery channel, not a defeated pin.

#### P01.S06 — The Ceremony Record **(added 2026-08-18, D20)** *(done 2026-08-19, v1.109.46)*

**(slice-grill notes, 2026-08-19.)**
- **The preimage is the slice's security content**, and PLAN-2 already specified it axis by axis. Building it from the table above rather than from the pin would have shipped a commitment that omits `signs` — which is the attack the pin names, not a detail of it.
- **The record is a PDF attachment, embedded in the pre-signing pass.** Caveat 10 was discharged by measurement on 2026-08-18: it survives three incremental signatures byte-identical, and attaching *after* signing invalidates every signature. That measurement is what makes `PrepareDocument` the only place this can happen.
- **`docHash` is over the prepared document — the bytes with the readme and the record's own attachment slot already in place — and the record cannot contain its own hash.** So the hash is taken over the document as prepared *without* the record, and the record is attached after. A later party recomputes the same way: strip the record, hash what is left. Stated because "SHA-256 of the prepared document" admits a reading in which the record hashes itself.
- **The hop-4 clause is drivable here and does not need P07.** Three incremental signatures on one document, then recompute — no multi-party machinery required, only a document that has been signed three times.

Tasks:
- **T01 — the record type and its canonical preimage**: length-prefixed, in PLAN-2's order, version first; `signs` inside it; the six-word name and the invitation secret deliberately out.
- **T02 — the convener signature** over the preimage, and verification from the document alone.
- **T03 — embed and extract** as a PDF attachment; `docHash` over the document with the record removed, so a later party can recompute it.
- **T04 — `PrepareDocument` carries the record**, and still refuses an already-signed document.
- **T05 — the `[NibRoster:<hash>]` token** in each signer's `/Reason`, and a cross-bind report when signers do not share one commitment.
- **T06 — the `~/nib/ceremonies/<id>/` mirror.**
- **T07 — seam inventory rows.**
Scope: the record's format, its convener signature, its embedding in the pre-signing pass, and the `~/nib/ceremonies/<id>/` mirror. Refs: D20, D2 pin.
Acceptance:
- A record survives N incremental signatures and is still readable and still verifies (caveat 10, driven).
- A record whose convener signature does not verify is refused before any pairing, with a distinct message.
- `PrepareDocument` refuses to embed a record into an already-signed document, for the reason it already refuses the readme.
- The `[NibRoster:<hash>]` token appears in each signer's `/Reason` and cross-binds; a document whose signers do not share one commitment is reported as such rather than as co-signed.
- **A party at hop 4 recomputes `docHash` from the document it received — carrying three incremental signatures — and matches the record. (added 2026-08-18, plan-review, D20 pin.)** The round-trip bullet above cannot see this: the convener's own bytes satisfy it without any later party recomputing anything.
- **The record carries its format version at the moment it is first written, driven by reading it back. (added 2026-08-18, plan-review, D32 pin.)** P07's skew bullet tests two versions meeting and says nothing about whether the field existed three phases earlier — and `rosterHash`'s preimage begins with it.
Tasks: *(written at slice-grill time)*

#### P01.S07 — The invitation **(added 2026-08-18, D21)** *(done 2026-08-19, v1.109.47)*

**(slice-grill notes, 2026-08-19 — including caveat 11, which this slice was assigned.)**

**Caveat 11 is settled: HKDF over `secret ‖ exporter`, confirmed with a MAC. Not a PAKE.** The plan left the mechanism open between "a PAKE over the secret" and "an HKDF over secret ‖ transcript", and the choice is not close once the secret's entropy is stated. **A PAKE exists to make a LOW-entropy secret safe against offline dictionary attack** — it bounds an attacker to one guess per live interaction because the alternative is a wordlist. D21's secret is **32 bytes of uniform randomness**: there is no dictionary, and an offline attacker faces 2²⁵⁶ whatever the protocol. A PAKE would buy a property the secret already has, and charge a new dependency, a new licence to clear, and extra round trips for it.
*What HKDF gives, and it is what D21 asks for:* the binding key is a function of the secret **and** of this channel's `ExportKeyingMaterial` — the same exporter P01.S04's verification string already uses — so a holder of the invitation proves possession on **this** connection and a recording of one connection is useless on another. Each side sends a MAC over its role; a peer without the secret cannot produce one, and a peer on a different channel produces the wrong one.
*Recorded as a slice-grill decision rather than referred:* the best-and-most-correct test answers it — fewer moving parts, no new dependency, and the alternative solves a problem this design does not have. Caveat 11 is discharged, not deferred.

**Two further notes.**
- **The invitation is the root of trust and nothing signs it** (PLAN-1's Stage 6 pin) — that stands, and this slice does not pretend otherwise. What it can do is make a *tampered* invitation loud rather than silent: the roster it carries is compared against the record's when the document arrives, so an altered invitation is refused by name at the first moment there is anything to check it against.
- **`docHash` is not in the invitation.** A ceremony's document may not exist when invitations go out, and the record carries the hash. Recorded because "the invitation carries the roster" invites the question.

Tasks:
- **T01 — the invitation type, its text form and its version**: prefix, base64url payload, checksum; a corrupted one refused with a distinct error rather than a partial pairing.
- **T02 — key derivation from the secret**: rendezvous key (per hop, D30), record key, and the channel-binding key — all HKDF, all domain-separated by info string.
- **T03 — the channel binding**: MAC over the role, keyed on `HKDF(secret, exporter)`; caveat 11's mechanism.
- **T04 — consume**: pin every roster fingerprint at FULL length, and a guard that no path decodes a name into a pin.
- **T05 — the roster comparison** against the record when the document arrives.
- **T06 — seam inventory rows.**
Scope: issue, encode, deliver-by-paste, and consume an invitation; the 32-byte secret and the full-fingerprint roster. Refs: D21, D6 amendment.
Acceptance:
- An invitation round-trips through a copy-paste of its text form, and a corrupted one is refused with a distinct error rather than a partial pairing.
- Consuming an invitation pins every roster fingerprint at full length, and the six-word name is displayed but never decoded into a pin on this path.
- The rendezvous key and the record encryption derive from the invitation secret, and two ceremonies between the same parties produce different keys (D6 amendment, driven — the point of re-keying).
- **An invitation is not a signing credential: a party holding a valid invitation but not the roster's private key is refused at the handshake, driven rather than argued. (D21.)**
- **A one-byte-altered invitation is refused by name when the document arrives, because the party compares the invitation's roster against the record's. (added 2026-08-18, plan-review, D21 pin.)** The full-fingerprint bullet above cannot see it: a tampered invitation satisfies that bullet perfectly by pinning the wrong key at full length.
- **The invitation carries its format version, driven by reading it back and by presenting one with an unknown version. (added 2026-08-18, plan-review, D32 pin.)**
Tasks: *(written at slice-grill time)*

### P02 — QUIC session transport **(TCP retained beside it — amended 2026-08-16, D14)** *(done 2026-08-19, v1.109.55)*
Goal: ~~re-base `Dial`/`Listen` onto QUIC~~ **give the session a QUIC path beside its existing TCP one (2026-08-16)**, with `SessionTLS()` reused unchanged, still over the manual address, so the transport change is proven in isolation before any discovery depends on it.
**Amended 2026-08-18 (D26): P02's first slice is the multi-instance harness, because it is what
lets any later phase prove a ceremony at all.**

Exit criteria *(all met 2026-08-19, v1.109.55 — ledger in `code-reviews/P02-phase-close-2026-08-19.md`; one clause **not exercised** at the two-machine level and it is the Dan-only run this phase already carved out)*:
- ✅ **The multi-instance harness exists and runs a two-instance ceremony end to end: two binaries, two `HOME`/`XDG_CONFIG_HOME` pairs, two vaults, two identities, over loopback, headless and unattended. (added 2026-08-18, D26.)** Driven by *completing a ceremony*, not by the harness starting two processes — a harness that boots two Nibs and asserts nothing is the vacuous green this decision exists to prevent.
- ~~A full ceremony completes over QUIC between two machines using the manual address. **(Dan-only run — plan-review W4.)**~~ **Amended 2026-08-18 (D26): the single-host case is now driven by the harness above and is no longer Dan-only; the two-*machine* run remains Dan-only and remains the standing `pending.md` VERIFY item, because two hosts on two networks is what the harness cannot model.** The distinction is the point — W4's marker was doing the work of two different claims.
- ✅ **The selected QUIC library accepts an externally-supplied `net.PacketConn` and completes a ceremony over it — proven by a spike that binds the socket first and hands it in, not by reading the library's documentation. (added 2026-08-17, plan-review C3, caveat 7.)**
- ✅ **The QUIC library and the DHT library share one socket through a demultiplexer, proven by driving interleaved QUIC and KRPC traffic at the same port and asserting both arrive intact. (added 2026-08-18, Stage 2 grill, caveat 7 pin.)** The bullet above cannot see this: **each library can accept an external `net.PacketConn` and the pair still be unusable**, because separating them needs a passthrough hook one of them must expose. And the cheap discriminator is refuted by arithmetic — a bencode dict's leading `'d'` (`0x64`) is bit-for-bit a QUIC short-header packet. Without this, tiers 3 and 4 cannot be built on the library P02 chooses, and P02 is redone. Pairs naturally with caveat 1's `VerifyPeerCertificate` spike — one spike answers both.
- ✅/⚠️ **A full ceremony still completes over TCP between the same two machines, after the QUIC path exists. (added 2026-08-16, D14)** *Met on one host (tier 4 runs the ceremony over TCP after the QUIC path exists, `TestSessionRoundTrip/tcp` beside it); **not exercised** between two machines, which is the Dan-only run the criterion above already carved out and which stays a standing VERIFY item.*
- ✅ The pinned-peer callback demonstrably rejects a non-pinned peer under QUIC, driven red.
- ✅ ~~`Initiate`, `Receive` and `coSignExchange` are unchanged.~~ **`coSignExchange` is unchanged; `Initiate`, `Receive`, `SendDocument` and `ReceiveDocument` are re-typed off `*tls.Conn` to a stream plus an already-verified fingerprint, and one set of session-logic tests runs green over both transports. (superseded 2026-08-16 — the original criterion was unmeetable: those four are typed to `*tls.Conn` today, see the D7 pin.)**

**PHASE CLOSE — 2026-08-28, v1.117.236. Ledger: 18 met / 1 not met / 3 partly.**

*Required-run gates, enumerated because nothing else walks them and a ledger can be green beside a
gate that never ran:* **tier 0** build ✓, **tier 1** `go test ./...` ✓, **tier 2** `jsdomtest.sh` ✓
(exit status, not the TAP totals — see below), **tier 3** `uirepro.sh` ✓ 59 tests, **tier 4**
`pairrepro.sh -n 4` ✓ — a four-party ceremony COMPLETED as a baton relay over BOTH transports, a
non-signing convener carrying it through 3 hops, each document a byte prefix of the next, every
hop's verification string distinct — **tier 6** `ceremonyrepro.sh` ✓ 13/13, **`redproof.sh --all`**
✓ **178/178** — after 13 stale rows were re-recorded and each re-verified individually. Tier 5 is P03's and winrepro is Dan's; neither is this phase's gate. All measured at
v1.117.235–.236.

- **C01 ✓ met** — tier 4: 4-party ceremony completed; 3 distinct signers in roster order, one
  signature each; `proceeding claimed=True`; cross-bound. All four clauses.
- **C02 ✓ met** — tier 4, non-signing convener; the finished document carries the signers' and none
  of the convener's.
- **C03 ⚠ PARTLY** — "all on the page" and "none overlaps the readme body" are met at N=9
  (`placement_test.go`, and blocks now live on dedicated pages). **"driven by rendering and
  measuring" is `not exercised` at nine parties**: the rendered differential exists
  (`blockink.test.mjs`, six stacked blocks, 4660 px of readme prose covered) but on the MANUAL
  path. The nine-party ceremony render is **`/pending 305`** and needs nine identities in a browser.
- **C04 ✓ met** — refused, names a new ceremony as the answer, AND states the collected signatures
  cannot be carried. **The third clause was unbuilt until this close**; the criterion predicted it
  ("it is the half a builder omits, being the bad news").
- **C05 ✓ met** — both cases driven separately in Go (`l3_test.go`, two packages) and through the
  real route with the UI bypassed (tier 6, and tier 4's hop-2 clause).
- **C06 ⚠ PARTLY** — "not by hand" is met for every criterion: nothing here is verified manually.
  "driven by the multi-instance harness" is met for the criteria that need a live multi-party
  ceremony (C01, C02, C05, C07) and **not** for the rest, which are driven by automated tests
  rather than by `pairrepro`. Read literally the criterion is not met; read against its own gloss —
  *"verified once, manually, at the phase gate is the shape of check this repo has already been
  burned by"* — it is. Recorded as partly rather than argued either way.
- **C07 ✓ met** — tier 4, over the carry route, no signature of the convener's.
- **C08 ✓ met** — `readme_test.go` / `sigpages_test.go`; the drift guard is green.
- **C09 ⚠ partly met** — blocks at N=9 (`blockname_test.go`), details panel at N=9
  (`test/jsdom/nineparty.test.mjs`); the two-party "attests to the other's key" is unreachable
  above a mutual pair.

  **⚠ CORRECTED 2026-08-30 (post-close, `/pending 317`): this is `⚠ partly met`, and the ✓ was
  measured false.** `blockname_test.go` calls `StampCommitment` **in its own fixture** and then
  reads `AppearanceLines()` — it proves the function. The production quote routes call
  `AppearanceLines()` on an attestation nothing stamped: `cosignAttestation`
  (`internal/server/cosign.go:57-107`) never stamps, and both `/api/cosign/quote` and
  `/api/session/quote` render its lines. Driven at the route against an 8-signer roster, the block
  a user SEES is 5 lines and the signature it sits on is 6, and **only the header matches**. This
  is `rosterfields_test.go:21`'s own named shape — *"a test whose fixture supplies the very thing
  the production code fails to supply is the vacuous green this repo keeps finding."* The corrected
  half is scheduled as a P06 slice; leaving a known-false ✓ in a closed ledger is worse than the
  defect, which is why this is written here rather than only in the backlog.
- **C10 ✓ met** — README command table, exit-status paragraph and a worked example checked against
  the real binary's output.
- **C11 ✓ met, for the rows this file holds** — 224 rows, 224 dispositioned: 219 `keep-live`, 5
  `deleted`, 6 retagged to the phase that owns them, 4 corrected. **Seventeen P07 slices have no
  rows at all** and a pass over the inventory cannot see them; `/pending 297` amended from seven to
  seventeen with the trade written out.
- **C12 ✓ met** — all four of D33's numbers driven PAST their bound. **The roster maximum was
  driven by nothing until this close** — `ErrRosterTooLarge` appeared in no test in the tree.
- **C13 ✓ met** — a sentence, not a parse error, driven separately for the record, the invitation,
  the ceremony protocol, **and the attestation tag**, which was the fourth surface and the silent one.
- **C14 ✓ met** — tier 4 completes; `Matched` per predecessor; the first signer's state now has a
  surface. Clause 4's roster-member framing holds by construction (`crossBind` takes no roster) and
  is driven in its absent-peer form (`cosign_test.go`).
- **C15 ⚠ partly met** — `recital_test.go`: the Confirmer's intent is discarded and `defaultIntent` is
  unreachable when a record is present.

  **⚠ CORRECTED 2026-08-30 (post-close, `/pending 317`): this is `⚠ partly met`, and the ✓ was
  measured false.** `blockname_test.go` calls `StampCommitment` **in its own fixture** and then
  reads `AppearanceLines()` — it proves the function. The production quote routes call
  `AppearanceLines()` on an attestation nothing stamped: `cosignAttestation`
  (`internal/server/cosign.go:57-107`) never stamps, and both `/api/cosign/quote` and
  `/api/session/quote` render its lines. Driven at the route against an 8-signer roster, the block
  a user SEES is 5 lines and the signature it sits on is 6, and **only the header matches**. This
  is `rosterfields_test.go:21`'s own named shape — *"a test whose fixture supplies the very thing
  the production code fails to supply is the vacuous green this repo keeps finding."* The corrected
  half is scheduled as a P06 slice; leaving a known-false ✓ in a closed ledger is worse than the
  defect, which is why this is written here rather than only in the backlog.
- **C16 ✓ met** — `record_test.go`: a `signs:false` convener's completed ceremony reads complete.
- **C17 ⚠ PARTLY** — (a) and (b) driven and refused by name before consent (S02b); the reconciliation
  covers **intent** (this phase) and **capacity** (whole-`Party` comparison). **`expires` is not
  covered and cannot be**: the invitation does not carry it, which is `/pending 247`, deferred on a
  measured argument this phase re-confirmed.
- **C18 ✓ met** — driven at five of nine in Go and through `nib verify`, which names the four who
  have not signed.
- **C19 ⚠ partly met** — blocks render each party's own capacity AND the signed `/Reason`s differ in
  capacity while carrying one identical recital. **The signed half was unbuilt until this close.**

  **⚠ CORRECTED 2026-08-30 (post-close, `/pending 317`): this is `⚠ partly met`, and the ✓ was
  measured false.** `blockname_test.go` calls `StampCommitment` **in its own fixture** and then
  reads `AppearanceLines()` — it proves the function. The production quote routes call
  `AppearanceLines()` on an attestation nothing stamped: `cosignAttestation`
  (`internal/server/cosign.go:57-107`) never stamps, and both `/api/cosign/quote` and
  `/api/session/quote` render its lines. Driven at the route against an 8-signer roster, the block
  a user SEES is 5 lines and the signature it sits on is 6, and **only the header matches**. This
  is `rosterfields_test.go:21`'s own named shape — *"a test whose fixture supplies the very thing
  the production code fails to supply is the vacuous green this repo keeps finding."* The corrected
  half is scheduled as a P06 slice; leaving a known-false ✓ in a closed ledger is worse than the
  defect, which is why this is written here rather than only in the backlog.
- **C20 ✓ met** — `convene_test.go`, `ErrDeadlineTooTight` at convene.
- **C21 ✗ NOT MET** — parked, and it is the phase's one open decision. It needs an `Expires` on the
  invitation that `/pending 247` declined on a measured security argument; P07.S07b confirmed that
  argument survives, because a field consumed at ARM time cannot be protected by a comparison that
  only runs once a document arrives. Dan's call: leave it, overrule 247's G2, or amend C21.
- **C22 ✓ met** — `mirror_durable_test.go` and S02a: written before the response returns, findable
  by id.

**What the close itself found, because none of it was visible from the slices.** Three criteria
were NOT met and read as met until asked clause by clause (C04, C12, C19 — all fixed here).
**Thirteen of 178 red-proof rows had gone stale**, nine of them staled by this phase's own last
third; one row's "defect" had become correct code and was re-recorded one layer down. **Two
harnesses were driving two faults at once** — tier 6's L3 clause and tier 4's — both green for
months while testing something other than what they claimed. And **tier 2 was red for four commits**
while its TAP totals said `# fail 0`, which its own file had already recorded happening twice.

~~Slices *(sketch)*: the multi-instance harness, first, driving the HTTP API…~~ **Firmed 2026-08-19 at phase-open. The sketch's seven items become six slices: "the pinned-rejection test ported" and "the session-logic tests parameterised over both" are one piece of work, not two, and splitting them would mean writing the parameterised harness twice.** *(Sketch retained below the slices.)*

#### P02.S01 — The multi-instance harness *(done 2026-08-19, v1.109.49)* *(D26; the phase's first slice by its own amendment)*
Scope: a fourth tier that runs **two** real nib binaries against each other over loopback, headless and unattended, driving each one's HTTP API. Refs: D26, `CONTRIBUTING.md`'s tier table.
Acceptance:
- Two binaries, two `HOME`/`XDG_CONFIG_HOME` pairs, two vaults, two identities — asserted to be *different* identities, because one vault reused is the shape that makes every later assertion vacuous.
- **A ceremony completes end to end**, driven by *completing* it: one instance arms, the other initiates, both confirm the spoken check on their own APIs, consent is given, and the returned document carries two valid signatures. **A harness that boots two Nibs and asserts nothing is the vacuous green D26 exists to prevent.**
- The harness skips cleanly when its dependencies are absent, as tiers 2 and 3 do.
- **Its ceiling is written in its own file** as every other tier's is (`verify_test.go` guards that each tier states one): **it cannot see two networks**, and what it delegates upward is the Dan-only two-machine run.
- The tier is added to `CONTRIBUTING.md`'s table and to `verify_test.go`'s guard, or the contract describes three tiers while four exist.

#### P02.S02 — Library selection, as a pair *(done 2026-08-19, v1.109.50)* *(caveat 7 pin)*
Scope: choose the QUIC and DHT libraries **together**, against the socket-sharing constraint, with a spike that binds a `net.PacketConn` first and hands it in. Refs: caveat 6, caveat 7, D8, D15.
Acceptance:
- Each candidate accepts an externally-supplied `net.PacketConn` — **proven by a spike, not by reading the documentation**.
- **The pair is evaluated as a pair**: a QUIC library and a DHT library that each accept an external conn can still be unusable together, because separating them needs a passthrough hook one of them must expose. A candidate with no hook is disqualified even if it passes the bullet above.
- Licences are AGPLv3-compatible and recorded in `THIRD-PARTY-NOTICES.md`.
- The spike also answers caveat 1: `VerifyPeerCertificate` is invoked exactly as `crypto/tls` does it, with `InsecureSkipVerify` set and `RequireAnyClientCert` honoured. One spike, two caveats.

#### P02.S03 — The socket demultiplexer *(caveat 7's Stage-2 pin)* *(done 2026-08-19, v1.109.51)*
Scope: separate QUIC and KRPC arriving on one UDP port. Refs: caveat 7.
Acceptance:
- Interleaved QUIC and KRPC traffic at the same port, both arriving intact — driven, not argued.
- **The cheap discriminator is asserted to be wrong**, in a test, so nobody re-derives it: a bencode dict's leading `'d'` (`0x64`) has the QUIC header-form bit clear and the fixed bit set, which is exactly a QUIC short header. The collision is on the steady state, not an edge.

Tasks:
- T01 — `internal/udpmux`: one owned socket, two `net.PacketConn` views, the two routing rules.
- T02 — the routing rules as unit tests, each probed red.
- T03 — the driven half: real quic-go and real anacrolix/dht on one port, interleaved.
- T04 — race detector, seam inventory, notices, red-proof ledger.

**Built as two rules, not one** *(2026-08-19)*: a **long-header** packet (`0x80` set) is QUIC —
unambiguous, because a bencode dictionary's `'d'` can never set that bit — and this is what lets
an inbound connection bootstrap from an address never seen before. Everything else from an address
**this process has sent QUIC to** is QUIC; the rest is KRPC. The peer table is learned **only from
our own outbound writes**, never from an inbound packet, so it grows with this process's own
connections rather than with a stranger's forged long headers.

**The one thing it gets wrong, recorded rather than defended:** a DHT node at the same `IP:port` as
an active QUIC peer is routed to QUIC and its message is lost. The rule only ever over-claims for
QUIC, so a **session is never misrouted** — the cost is that two peers' DHT nodes cannot query each
other while a session is open, which the DHT already tolerates as an unresponsive node. Asserted in
a test of its own so it stays a measured property. The exact fix — routing short headers on the
destination connection ID via a `quic.Transport.ConnectionIDGenerator` that registers what it issues
— is **filed, not built**: a mis-wired generator would break the *session* rather than a DHT query,
which is the worse failure of the two, and P05 wiring the transport is when that trade changes.

#### P02.S04 — The session core re-typed off `*tls.Conn` *(D14)* *(done 2026-08-19, v1.109.52)*
Scope: `Initiate`, `Receive`, `SendDocument` and `ReceiveDocument` take a stream plus an already-verified fingerprint; `coSignExchange` is unchanged. Refs: D14, D7 pin.
Acceptance:
- The four entry points no longer name `*tls.Conn`; `coSignExchange` is untouched.
- Everything P01 hung on those signatures still holds — the verification gate, the L2 guard's population floor, the four-path enumeration.

Tasks:
- T01 — `Channel`/`Stream`/`Exporter`, and `TLSChannel` as the one place the handshake, the peer fingerprint and the exporter are taken together.
- T02 — the four entry points re-typed onto `Channel`.
- T03 — `verify.go` off `*tls.Conn` too: `verificationExchange` and `verificationString`.
- T04 — the call sites in `internal/server/session.go`, and the three test files.
- T05 — a zero-value `Channel` is refused at each entry point, probed red.

**The scope line understated the surface** *(2026-08-19)*: `runVerification`, `verificationExchange`
and `verificationString` name `*tls.Conn` or `tls.ConnectionState` as well, and the last of the three
needs `ExportKeyingMaterial` — D4's channel binding. So the re-typing is not four signatures, it is
the whole verification path with them.

**And the constraint that would have blocked it does not exist** *(2026-08-19, measured)*: quic-go
contains no `ekm` of its own, which reads like a hard stop for the channel binding under QUIC. It is
not — a spike over a real QUIC connection got a **32-byte exporter output on both ends, agreeing**
(`TestSpikeEKMWorksUnderQUIC`). Kept as a standing spike beside caveat 1's, because it is the same
kind of claim: load-bearing, about a dependency, and cheap to re-ask.

**Built as a `Channel`, not three parameters** *(2026-08-19)*: `Stream` (read, write, `SetDeadline`),
the **already-verified** `PeerFP`, and `Export`, the RFC 5705 exporter. Three adjacent `[]byte`
parameters would have been silently transposable; more importantly, all three are properties a
transport has **already established**, so a zero value is not an incomplete argument list but an
unauthenticated session with a verification string bound to nothing — `Channel.check` refuses one at
every entry point, and each refusal is probed red.

`Dial`, `Listen`, `TLSChannel` and `verifiedPeerFingerprint` moved into `transport.go`, so the whole
of the package's TLS knowledge is one file and **S05 adds `QUICChannel` beside `TLSChannel`** rather
than threading a second transport through the core. `TestTheSessionCoreDoesNotNameATransport` is the
acceptance clause as a guard: `session.go`, `verify.go` and `channel.go` may not contain the string
`*tls.Conn` at all, and `transport.go` exports exactly one function that does.

`ReceiveDocument` **no longer returns the peer fingerprint** — the caller supplied it in the
`Channel`, and handing it back invites a caller to trust the echo rather than the value it verified.

#### P02.S05 — QUIC `Dial`/`Listen` behind the core *(done 2026-08-19, v1.109.53)*
Scope: a QUIC path beside the TCP one, `SessionTLS()` reused unchanged, still over the manual address. Refs: D14.
Acceptance:
- A ceremony completes over QUIC in the multi-instance harness.
- The pinned-peer callback rejects a non-pinned peer under QUIC, **driven red**.

Tasks:
- T01 — `Conn` (a `Channel` plus its Close) and `Listener` (`Accept() (*Conn, error)`), and the TCP `Dial`/`Listen` reshaped onto them.
- T02 — `QUICDial`/`QUICListen`/`QUICChannel`, over `internal/udpmux` from the first line.
- T03 — the server selects a transport: a field on arm, a form value on the two dial routes, defaulting to TCP.
- T04 — tier 4 runs the whole ceremony over QUIC; the pinned rejection under QUIC driven red.
- T05 — the guards updated deliberately, race detector, docs.

**The listener had to become an abstraction too** *(2026-08-19)*: the scope names `Dial` and
`Listen`, and the server holds a `net.Listener` that a QUIC listener is not. Rather than a second
accept path beside the first, `Listener.Accept()` returns a `*Conn` with its `Channel` already
established — which is D14's own argument one level up, and it moves each transport's handshake
timeout and retry-on-wrong-peer into that transport instead of the server.

**It uses the demultiplexer from the first line** *(2026-08-19)*: S03's mux costs one expression
here (`quic.Transport{Conn: mux.QUIC()}` rather than a bare socket) and means P04 attaches a DHT to
a socket the session is *already* sharing, instead of changing transport code to make room.

**The connection-ID refinement is NOT built here, and the reason changed** *(2026-08-19)*: the
pending item filed at S03 names this coordinate. It is wrong. Until P04 attaches a DHT there is no
KRPC on the socket for the address rule to misroute, so the fix would be unexercised by construction
and its guard could not fail — which is the exact defect class this repo keeps finding. Re-pointed
at the coordinate where a DHT and a session share one socket for real.

**No UI** *(2026-08-19)*: the transport is selectable over the API, which is what tier 4 drives, and
deliberately not exposed in the interface. D8's ladder means the user should never be choosing a
transport; a toggle shipped now would be a control the ladder exists to remove.

**Closing a QUIC connection is not the polite thing closing a TCP one is** *(2026-08-19, measured)*:
`CloseWithError` sends CONNECTION_CLOSE immediately and abandons anything unacknowledged, so the
listening side destroyed the co-signed document in its own deferred `Close` — observed as
`receive co-signed document: Application error 0x0 (remote)`. The teardown closes the stream first
and then **waits, on one side only**: in all four entry points the listening side writes last and
the dialing side reads last, so only the listener has anything owed. Waiting on both was the first
fix and cost the full grace on every ceremony — **5.05s, against 0.06s asymmetric**. quic-go exposes
no "everything I wrote is acknowledged" signal; `SendStream`'s own Context is cancelled inside
`Close`, before any of it is acked.

**A rejected dial returns success on BOTH transports** *(2026-08-19, measured)*: under TLS 1.3 the
client finishes its handshake before the server has processed the client certificate, so an unpinned
peer's dial succeeds and the refusal surfaces on the first **read** — `tls: bad certificate` on TCP,
`CRYPTO_ERROR 0x12a (remote)` on QUIC. Pre-existing and identical on both, not anything QUIC
introduced, and it costs nothing: the peer never answers, so the verification exchange cannot
complete and no document byte crosses (L2). Recorded because the obvious test asserts the dial
fails, and that test would be asserting something untrue.

#### P02.S06 — Both transports, one set of session tests *(D14)* *(done 2026-08-19, v1.109.54)*
Scope: the TCP dialer kept as a peer behind the same core, with the session-logic tests parameterised over both. Refs: D14.
Acceptance:
- One set of session-logic tests runs green over both transports.
- A full ceremony still completes over TCP after the QUIC path exists.

Tasks:
- T01 — one transport table, and a population guard so a third transport cannot be added without entering it.
- T02 — the three session-logic tests and the L2 gate parameterised over it.
- T03 — `livePair` parameterised, so the verification string is derived over both channels.
- T04 — the QUIC-only ceremony test folded in, because two sets is what this slice exists to remove.

**The table's guard had two holes and both were live** *(2026-08-19)*: its first regex matched names
ending in `Listen` and so could not match `Listen` itself — the same re-typed-regex shape S04 hit one
slice earlier — and its distinctness check compared **names**, so a table of `{"tcp", Listen, Dial}`
and `{"quic", Listen, Dial}` would have run the whole suite over TCP twice with every subtest
labelled `quic`. A parameterised suite is only as good as its parameters being different, and only
comparing the function values says they are.

*Sketch retained:* the multi-instance harness, first, driving the HTTP API, with its ceiling written in its own file as every other tier's is (2026-08-18, D26); library selection as a PAIR — QUIC and DHT chosen together against the socket-sharing constraint, never one then the other (2026-08-18, caveat 7 pin); the socket demultiplexer and a spike proving `VerifyPeerCertificate` fires as under `crypto/tls`; the session core re-typed off `*tls.Conn` (D14); QUIC `Dial`/`Listen` added behind that core; the pinned-rejection test ported; the TCP dialer kept as a peer behind the same core, with the session-logic tests parameterised over both (2026-08-16, D14).

*Note on the harness's place in the chain:* `CONTRIBUTING.md`'s tier table is the contract, and
`verify_test.go` guards that each tier states its own ceiling. A fourth harness that does not say
what it cannot see would be the one exception to a rule this repo enforces in tier 1 — so it says
it: **it cannot see two networks**, and what it delegates upward is the Dan-only run.

### P03 — Local discovery (tier 1) *(done 2026-08-19, v1.110.5)*
Goal: two Nibs on the same network find each other with no address typed and no internet.
Exit criteria *(reconciled 2026-08-19, v1.110.5 — ledger in `code-reviews/P03-phase-close-2026-08-19.md`; **5 met / 2 not exercised**, and both unexercised rows are Dan-only runs this plan already carved out rather than gaps found at the gate)*:
- ✅/⚠️ A ceremony completes on a LAN with no address entered anywhere and no outbound internet traffic. *Driven on one host: no `bind` and no `address` sent at all, and the egress counter zero across the ceremony with its ability to fire proved **per family** first. **Not exercised across two machines on a real switch** — the namespace has dummy interfaces and no L2 segment worth the name, which is the standing VERIFY item.*
- ✅ Discovery announces the name's public bits only — never anything that could influence which peer is accepted (L1).
- ⚠️ Behaviour on Windows is verified on Windows, not inferred — **on real Windows, as a Dan-only run (added 2026-08-17, plan-review W3).** `build/winrepro.sh` runs `nib.exe` under **wine**, which was defensible for `path/filepath` behaviour at the sibling plan's P07 and is not defensible here: wine models neither multicast nor interface enumeration. A green `winrepro` may not discharge this bullet.

Slices *(firmed 2026-08-19 at phase-open; sketch retained below)*. **The sketch's four items become five slices.** "The L1 guard" is not separable from designing the announcement — you cannot decide the format without deciding what it may carry, and the guard *is* that decision expressed as a test — so it folds into S01; and "a ceremony with no address typed" is pulled out of "resolving a candidate" because it is the phase's first exit criterion and owes a driven run of its own.

Two carve-outs, written here so neither is inherited by assumption:

- **The multicast socket is never `internal/udpmux`** — caveat 7's plan-review pin says so directly, and the mux exists for a NAT-mapping reason that link-local traffic does not have.
- **D16's 300 s connect deadline against `sessionDialTimeout = 30 s` (`internal/server/session.go:39`) is P05's, not P03's.** Tier 1 is browse-then-dial inside a 2 s browse budget and fits in 30 s with room. The ladder's restructure belongs to the phase that builds the ladder.

#### P03.S01 — The announcement, and what it may say *(L1; ADR-007)* *(done 2026-08-19, v1.110.0)*
Scope: the announcement record's wire format, its parser, and the guard that nothing in it can reach acceptance. Refs: L1, D3, caveat 3.
Acceptance:
- The announcement carries the six-word **name** and the bound port and nothing else — asserted against the encoder's output, not by reading the struct.
- **No code path turns an announcement into a pin or a peer selection** (L1), guarded at the source.
- The parser refuses an oversize, truncated or malformed datagram **without allocating on its content**, and creates no state from an unauthenticated one.
- ADR-007 records the format, why it is the name and not the fingerprint, and why the socket is treated as internet-facing on every platform.

Tasks:
- T01 — `internal/discovery`: the announcement record, its encoder and its parser.
- T02 — the parser's refusals, each probed red: oversize, truncated, wrong magic, wrong version, a name that is not six words.
- T03 — the L1 guard: no exported path from an announcement to a fingerprint or a pin.
- T04 — ADR-007, and the seam inventory rows.

#### P03.S02 — Announce and browse over multicast *(done 2026-08-19, v1.110.1)*
Scope: the multicast socket, interface selection, announce-while-armed, passive browse. Refs: caveat 3, caveat 7's carve-out, the `session.go` tripwire.
Acceptance:
- Two processes discover each other over **real multicast**, driven — not a model of it.
- The interfaces joined are **asserted**, not whatever the stdlib chose: a link-local join must skip loopback and point-to-point, and must not depend on `FlagRunning`, which is degenerate on Windows.
- The `TRIPWIRE` comment at `internal/server/session.go:24` is amended **in the same change**, naming link-local and armed-only and citing the plan's egress enumeration — it currently claims the armed listener is the *only* network-reachable surface, which this slice falsifies.
- Nothing in the announce or browse path imports `internal/udpmux`.

Tasks:
- T01 — interface selection as a **pure function** over `[]net.Interface`, so the choice is table-testable without a machine that happens to have a docker bridge.
- T02 — the socket: `x/net/ipv4`+`ipv6` over one `net.ListenConfig` with `SO_REUSEADDR`, joined per chosen interface.
- T03 — announce once per interface; read with the self-nonce filter.
- T04 — two processes discover each other over real multicast, driven.
- T05 — the tripwire amendment, and the seam inventory.

**The driven clause needed a fifth tier** *(2026-08-19)*: "two processes discover each other over
real multicast" cannot be verified on the development host, because a multicast loopback copy
traverses INPUT and a default-deny firewall swallows it silently. `build/mcastrepro.sh` creates its
own network namespace with `unshare -rn`, so the host's rules do not decide the result; tier 1's
discovery tests **skip** on such a host rather than fail, and a skip is not a verification, which is
what the new tier is for. Groups: `239.255.90.90` (RFC 2365 administratively scoped) and
`ff12::6e69:62` (RFC 4291 transient, link-local) — and **the scope guarantee is the hop limit of 1,
not the address**, because scope bits are a statement a router may respect and arithmetic is not.

#### P03.S03 — A discovered peer becomes a dialable candidate *(done 2026-08-19, v1.110.2)*
Scope: match announcements against pinned peers; surface the result. Refs: L1, D16's browse budget.
Acceptance:
- An announcement from a **pinned** peer resolves to that peer's fingerprint and address; one from an **unpinned** peer resolves to nothing — both driven, because only the pair distinguishes matching from accepting.
- `vault.PinnedPeers()` round-trips `Ceremony`, with a test. It does not today (`internal/vault/vault.go:725` drops it), and this is the first slice whose code reads that accessor.
- The browse window is D16's **2 s**.

Tasks:
- T01 — `vault.PinnedPeers()` copies `Ceremony`, with a round-trip test.
- T02 — resolution in `internal/server`, not in `internal/discovery`: S01's L1 guard forbids that package importing the vault, and the layer that holds pins is the layer that decides what to dial.
- T03 — both directions driven with really-encoded announcements; the 2 s window asserted as a bound, not a sleep.
- T04 — tier 5 resolves end to end inside the namespace.

**Where the matching lives is decided by S01's guard, not by taste** *(2026-08-19)*:
`TestNothingHereCanReachAnIdentity` forbids `internal/discovery` importing `internal/vault`,
`internal/sign` or `internal/p2p`. So resolution cannot live there, and that is the guard doing its
job rather than an obstacle to route around — discovery stays a link-layer package that knows
nothing about identity, and `internal/server` joins the two because it already holds both.

#### P03.S04 — A LAN ceremony with no address typed *(done 2026-08-19, v1.110.3)*
Scope: the dial side takes a discovered candidate; the harness drives it. Refs: P03 exit criteria 1.
Acceptance:
- A ceremony completes **with no address entered anywhere** — driven in the multi-instance harness, inside a network namespace so the run is hermetic and independent of the host firewall.
- **No outbound internet traffic, asserted rather than suppressed.** `NIB_NO_UPDATE_CHECK=1` (`build/uirepro.sh:114`) silences the one known caller; it does not observe absence, and the criterion says no traffic.

Tasks:
- T01 — the armed side announces while armed, and stops when it disarms.
- T02 — an empty `bind` binds ephemerally, and an empty `address` browses instead of refusing.
- T03 — the harness: two nibs in a namespace, a ceremony with neither address typed.
- T04 — the egress counter, with its own stimulus assertion.

**The obvious egress instrument is vacuous, and that is measured** *(2026-08-19)*: an nft output
counter inside a namespace reads **zero after a real connect attempt**, because with no default
route the kernel refuses at the routing stage and the packet never reaches the output hook — so
"no outbound traffic" would be true of a process that tried constantly. The namespace therefore
gets a **black-hole default route**, which makes attempts real packets the counter can see: probed
at 0 before, **2 after a connect to 1.1.1.1**. The assertion runs that provoke first, so a counter
that could never fire fails the harness rather than passing it.

#### P03.S05 — The Windows pass *(caveat 3 — the run is Dan's)* *(done 2026-08-19, v1.110.4 — the run parked, by construction)*
Scope: everything a real-Windows verification needs, short of the run. Refs: caveat 3, P03 exit criteria 3.
Acceptance:
- The two measured Windows divergences have **named, deliberate handling**: `x/net`'s `SetControlMessage` is a `TODO` returning `errNotImplemented` there, so a control message is nil with no error and interface filtering silently disappears; and IPv4 group join resolves the interface to an **address** (`setIPv4MreqToInterface`), so an interface with no IPv4 lease is joinable on Linux and refused on Windows.
- **Parked by construction:** the run itself. The phase's own criterion says a green `winrepro` may not discharge it, because wine models neither multicast nor interface enumeration.

Tasks:
- T01 — `nib discover`, the diagnostic: what was joined and why, what was sent, what came back.
- T02 — a guard that nothing decides on the arrival interface, since on Windows it is nil with a nil error.
- T03 — a guard that the IPv4 selection differs from the IPv6 one exactly where Windows needs it to.
- T04 — `winrepro.sh` states that it cannot discharge this, and `verify_test.go` guards the statement.

**The artifact this slice owes is something Dan can RUN** *(2026-08-19)*: "the run is Dan's" is only
meaningful if there is something to run. Without a diagnostic, a real-Windows verification means
building a test binary and reading Go output — so the slice ships `nib discover`, which prints the
interface selection with a reason per interface, announces, listens, and reports the counters that
separate *"I said nothing"* from *"I said it and nobody answered"*. Both Windows divergences surface
in its output rather than as silence.

**The port is Nib's own, not `5353`** *(2026-08-19, measured)*: a multicast loopback copy traverses INPUT, so on a default-deny desktop discovery is **silently dead on any port the firewall does not already permit** — measured on the development machine, where `224.0.0.251:5353` delivers and `224.0.0.251:15400` times out with no error at either end. The alternative, sharing the mDNS group, was refused: it puts non-mDNS bytes on a port other implementations parse as mDNS, and doing it properly means speaking DNS-SD, which is a phase of its own. So the cost is paid where it is visible instead — **hearing your own loopback copy is a firewall-independent liveness check on the send path**, and a browse that hears neither a peer nor itself can say which of the two failed.

~~Slices *(sketch)*: multicast announce/browse; resolving a discovered peer to a candidate; the L1 guard; the Windows pass.~~ *(retained; superseded by the firmed slices above.)*

### P04 — Endpoint exchange over the DHT *(done 2026-08-20, v1.116.0 — 6 slices, ledger 12 met / 0 not met / 1 not exercised (clause 4b, moved to P05 by amendment) / 1 not measurable at this granularity (conceded in the clause's own text). Close-out: `code-reviews/P04-phase-close-2026-08-20.md`)*
Goal: the two sides learn each other's public endpoints, and their own, with no server. **(plan-review note, 2026-08-17, I1 — the framing still understates the blast radius, as D8's own correction records: the DHT is the signalling channel for tiers 2, 3 **and** 4, so an unreachable DHT collapses three tiers at once and leaves only LAN and manual. Carried as a Stage 2 grill target; recorded here so the next pass does not re-derive it.)** **(plan-review pin, 2026-08-18, adopted by Dan: that gate has passed — the Stage 2 grill ran on 2026-08-18 and did not take this up. Re-targeted to this phase's slice grill, with D8's pin carrying the same instruction and the discharging observation.)**
Exit criteria:
- Each side learns its own public `IP:port` and its NAT class from DHT responses alone.
- A published endpoint is retrievable by the peer computing the same key ~~from the two names~~ **from the invitation secret** *(amended 2026-08-20, P04.S03's grill — D6's amendment re-keyed this to the secret and `RecordKey`'s own comment refuses name-derivation by name; a builder satisfying the old text ships obfuscation and passes the gate)*.
- Bootstrap works from a cached node list with no hostname resolution (D6).
- A hostile or absent DHT degrades to the next tier without ever affecting which peer is accepted (L1). **(amended 2026-08-20 at P04.S05: the two halves close at different phases, and leaving this whole is what would make P04 unclosable.** *"Without ever affecting which peer is accepted"* closes HERE, driven at both tiers over both transports. *"Degrades to the next tier"* moves to **P05**, because there is no tier ordering in the tree to degrade through — `peerAddresses` is a two-way if/else and `findPeerOnLAN` says so in terms. The slice-level clause was amended first; amending only there would have left this one claiming a coverage the phase cannot have.)**
- **The published record is encrypted under a key derived from ~~both names~~ the invitation secret: a DHT scraper holding ~~neither name~~ neither the invitation nor the secret sees opaque bytes and can neither read nor forge a candidate. (added 2026-08-17, plan-review C2, D6 pin; *amended 2026-08-20, P04.S03's grill — same correction*.)** **"Cannot forge" is graded `not measurable at this granularity`**: no instrument resolves it. What is measurable is that a record written under the wrong key is refused, which is narrower, and is what the ledger will say.
- **A record published by a party who knows both names but holds neither identity key yields no more than N candidates; the N+1th is dropped and reported — driven by publishing N+50. (added 2026-08-17, plan-review C2.)** The bullet above it cannot see this: a hostile DHT that floods a bystander never affects which peer is accepted, and so satisfies it completely.
- **The DHT library shares the session's local socket rather than opening its own, proven by a spike. (added 2026-08-17, plan-review C3, caveat 7.)** A self-address probe on a different socket measures a mapping the session will never use. **(amended 2026-08-18, Stage 2 grill: "shares" means *through the demultiplexer P02 built*, and the pair was chosen together at P02 — if this phase is where a DHT library is first tried against the QUIC one, P02's selection was not done.)**

**The second signalling path: DECLINED** *(2026-08-19, P04's slice grill — this is the discharge D8's pin asks for by name, and the pin is explicit that only a decision recorded here settles it, not a later grill of the plan as a whole).*

D8's correction stands and is not in dispute: the DHT signals for tiers 2, 3 **and** 4, so an unreachable DHT collapses three at once and leaves only LAN and manual. The question was whether the plan should carry a second automatic signalling path. It should not.

- **The fallback exists and is already legible.** Tiers 1 and 5 survive, and D19 cause 2 exists precisely to *say so to the user* rather than leave a blank failure.
- **The participants already hold a second channel, and it cost nothing.** They exchanged the invitation (D21) out of band. If the DHT is unreachable they can pass a `host:port` on the same channel. A second automatic path would rebuild in software what the humans already have.
- **What adopting costs is what decides it:** a second third-party network dependency, a second library and licence, a second unauthenticated ingress surface, and a second thing to keep working — permanently — on a design whose founding premise is no servers. That is a large standing cost to shorten a fallback that is one field typed once.

***Revisit trigger:*** any flow where two parties who have **no prior channel** must reach each other. This reasoning rests entirely on the ceremony being arranged between people who already know each other; strangers-meeting would remove its foundation, not weaken it.

Slices *(firmed 2026-08-19 at phase-open; sketch retained below)*. **Two of the sketch's five items had already been overtaken by earlier phases**, which is what firming against the tree is for:

- *"library selection and cached bootstrap"* — **selection is done.** P02.S02 chose anacrolix/dht v2.24.0 and quic-go as a *pair*, against the socket-sharing constraint, exactly as this phase's last exit criterion demands ("if this phase is where a DHT library is first tried against the QUIC one, P02's selection was not done"). Only cached bootstrap remains.
- *"the derived rendezvous key"* — **largely built.** `Invitation.RendezvousKey(hop)` and `RecordKey()` shipped with P01.S07 (D30's per-hop keying). What remains is *using* them, which belongs with the record.

#### P04.S01 — The DHT on the shared socket, with cached bootstrap *(D6, caveat 7)* *(done 2026-08-19, v1.111.0)*
Scope: attach a `dht.Server` to the demultiplexer's DHT view; persist and reload a node list. Refs: D6, caveat 7's amendment.
Acceptance:
- A DHT and a live QUIC session share **one** socket, driven — through `internal/udpmux`, which is what caveat 7's amendment means by "shares".
- Bootstrap works from a **cached node list with no hostname resolution** — asserted by observing that no name is resolved, not by the absence of a hostname in a config.
- The node list survives a restart: written on close, reloaded on start, and the reload is driven.

**S01 absorbs the connection-ID routing, and the measurement is why** *(2026-08-19, Reality drift)*:
P02.S03 shipped the demultiplexer keying on **peer address**, documented its one collision — a DHT
node at the same `IP:port` as an active QUIC peer is routed to QUIC and lost — and filed the exact
fix against "wherever a DHT and a session share one socket for real". **That is here, and the very
first test to drive both hit it:** A pings C with no QUIC anywhere and gets a reply; A pings B, with
whom A has a QUIC session, and the query **times out**, with A's mux reporting `RoutedByPeer:1` —
B's KRPC reply routed to the QUIC view and discarded.

So this is not a refinement to schedule; **S01's own driven clause cannot pass without it.** The
filed item's warning stands and shapes the fix: a mis-wired generator would break the *session*
rather than a DHT query, which is the worse failure, so the wiring needs a guard of its own.

Tasks:
- T01 — `internal/rendezvous`: a DHT server over `udpmux.Mux.DHT()`, opened and closed with the socket.
- T05 — the demultiplexer routes short headers on the **destination connection ID**, and the address rule survives only where no connection ID has ever been registered.
- T02 — the node cache: dump on close, reload on start, under `~/nib`.
- T03 — the no-resolution guard: nothing on the bootstrap path resolves a hostname.
- T04 — driven at tier 5: a DHT and a QUIC session on one socket, and a restart that reloads.

#### P04.S02 — The self-address probe and NAT classification *(D19, caveat 9)* *(done 2026-08-19, v1.111.1)*
Scope: learn this host's own public endpoint from DHT responses, and classify the mapping. Refs: D19, caveat 2, caveat 9.
Acceptance:
- This side learns its own public `IP:port` **from DHT responses alone**, and the port is observed **on the wire from a real node**, not inferred from the type carrying a port field (see the phase-open note: the representation is settled, the behaviour is not).
- Two observations from **different** nodes distinguish the two mapping classes (D19).
- Caveat 9 — that two distinct nodes answer inside D16's probe budget — is discharged by measurement, or its degradation to cause 4 is recorded rather than assumed.

**The field this slice exists to read is a remote process kill, and the slice absorbs the
fix** *(2026-08-19, Reality drift)*: `krpc.NodeAddr.UnmarshalBinary` does
`make(net.IP, len(b)-2)` with no length check (`nodeaddr.go:35`); bencode's decoder
**re-panics runtime errors** deliberately (`decode.go:45`); and the decode runs on the
goroutine `dht.NewServer` starts (`server.go:247`), which nothing Nib writes can recover.
**Reproduced through Nib's own mux:** 21 bytes — `d2:ip0:1:t2:zz1:y1:re` — from any host
that can send one datagram to the session port, with a well-formed ping answered first to
prove the stimulus. No session, no routing-table entry, no race required. Its silent
sibling: a **4-byte** `ip` field returns *no error* and yields a plausible port from a
message that never contained one, so caveat 2's clause cannot be satisfied by reading
`Reply.IP.Port` alone.

It is not yet reachable in a shipped binary — `internal/rendezvous` has no importer
outside its own tests — which is exactly why it is fixed **here** rather than filed: S02
and S03 are the slices that would ship it.

**And there was a second door, which the first fix could not see** *(2026-08-19, found by
the slice's own review, both agents independently)*. `handleQuery` binds `args := m.A` and
reads `args.Token` for `announce_peer` and `put` with no nil check, while `get_peers`
guards exactly that field one case earlier — so **34 bytes of
`d1:q13:announce_peer1:t2:zz1:y1:qe` is a nil dereference**, on the same unrecoverable
goroutine. These datagrams *decode perfectly*, so the decode screen passes them, correctly
and uselessly. Screening the decoder is not screening the library. Closed with an
`OnQuery` gate, and the rule is general rather than a list of the two query names: every
query type was fuzzed against three shapes of `a`, and **only an entirely absent
arguments dict crashes anything** — an empty one is survivable everywhere.

**A third, cheaper attack came out of the same review** and is fixed with it: bencode
allocates a declared string length before reading it, with a ~128 MiB ceiling
`bencode.Unmarshal` cannot lower, so **17 bytes buy 134,221,493 bytes of allocation**. The
screen now decodes under a bound of the datagram's own length — a string longer than its
container cannot exist — and drops on that error, so neither decoder pays it. Measured
after: 640 bytes.

**Three further defects in P04.S01's own code**, all on this slice's path and all found by
driving it: `Ping` takes a `ctx` the library discards (`context.TODO()`,
`server.go:1064`); it returns `res.Err`, so a node answering a **KRPC error** counts as
alive; and `writeNodes` drops every non-IPv4 node, which with the empty-table guard means
a v6-only host writes no cache and cold-starts forever.

Tasks:
- T01 — screen the DHT view: decode each datagram with the library's **own** decoder under
  `recover`, and drop what does not survive. It cannot drift from the library's parsing
  rules, because it asks them.
- T02 — `Ping` honours its context (`Query`) and its errors (`ToError()`).
- T03 — the node cache carries IPv6 (20 + 16 + 2 bytes), and an unreadable cache stops
  being fatal — which is what its own comment already claimed.
- T04 — the seed list of IP literals, consulted only when the cache is empty (D6's
  amendment), plus a bootstrap traversal that turns seeds into a usable table.
- T05 — the probe: up to 16 nodes in **distinct /24 and /48 prefixes** inside D16's 8 s,
  rejecting an `ip` whose decoded length is not 4 or 16, whose port is 0, or which is
  loopback / private / CGNAT / link-local / documentation — a counter per cause.
- T06 — three-valued classification, `MappingUnknown` as the **zero value**, per family,
  compared with `net.IP.Equal`; a strict majority across ≥2 distinct prefixes gives
  endpoint-independent, so a lying node is outvoted rather than obeyed.
- T07 — the L1 guard this package's doc already claims to have, **repaired**: it walks
  parameter and result *types*, not only names, and forbids `dht.ServerConfig.PublicIP`.
- T08 — `build/dhtlive.sh`: out-of-loop like `winrepro.sh`, opt-in, skipping cleanly,
  running the product's own probe against the real DHT.

#### P04.S03 — The published record: signed, encrypted, expiring *(D6, D30, C2)* *(done 2026-08-20, v1.112.0; its code review ran late and its fixes landed v1.112.1)*
Scope: publish and fetch the candidate record under the per-hop rendezvous key. Refs: D6, D21, D30, plan-review C2.

**Scope split at the slice grill (2026-08-20).** D6's second half — *seed nodes carried in the
invitation* — was filed against this slice and is **moved to P04.S06**. It is not in this slice's
acceptance, it is a second wire format (D21's invitation, not this record), and its attack surface
is its own: an unsigned invitation names arbitrary IP:port, a cold-start table has nothing else to
consult, and the chain ends at D33's 3,000 punch packets aimed at an attacker-chosen victim. One
gate each rather than one gate for both.

Acceptance:
- A published endpoint is **retrievable by the peer computing the same key ~~from the two names~~ from the invitation secret** *(amended 2026-08-20 — Reality drift; see the correction below)*.
- A DHT scraper holding **neither ~~name~~ the invitation** sees opaque bytes and can neither read nor forge a candidate *(same amendment)*.
- Records expire.
- **The in-roster forgery question is settled, adopt or decline, with a reason** — the pending item filed 2026-08-19 asks whether an invited party can publish *as another*, since D6's amendment gives every roster member the encryption secret. ~~BEP-44 mutable items are **ed25519-signed by construction** (`bep44.Put` carries `K [32]byte` and `Sig [64]byte`, and the target derives from the key plus salt), so a per-party key makes the DHT itself refuse the write. That shape is cheap and available; the slice must decide it rather than inherit it.~~ **The proposed shape is REFUTED — see "What the grill refuted" below. Settled: ADOPT an inner ECDSA identity signature.**
- **The disclosure lands with the publishing, not at P06** *(added 2026-08-20)* — see the D34 ordering correction below.
- **Neither door the DHT is currently open through is left open** *(added 2026-08-20)*.

**What the grill refuted** *(2026-08-20, three deepdive agents + four persona seats, every claim re-verified at the line)*. Four premises this slice rested on are false against the tree:

1. **The identity key cannot sign a BEP-44 item.** `sign.GenerateIdentity` makes an **ECDSA P-256** key (`internal/sign/identity.go:60`, signing at `:283`, verifying at `:300`). BEP-44's `K` is a 32-byte **ed25519** public key with a 64-byte ed25519 signature (`bep44/item.go:23-25`).
2. **A per-party key derived from the secret stops nothing.** Every roster member holds the `Secret` (`invitation.go:74-76`), so every member derives every party's key. The DHT would refuse no write any member wishes to make. What the BEP-44 signature actually proves is *"whoever wrote this held the invitation"* — a **group credential**, not a speaker.
   **Therefore: the inner ECDSA identity signature is the only construct that separates the authenticity boundary (the party) from the confidentiality boundary (the ceremony), which is what the pending item says a multi-party ceremony needs.** D31 removed the chicken-and-egg by pinning the full 32-byte fingerprint, so there is something to verify against. Cost measured: SPKI DER 91 B + ASN.1 sig 71 B = 162 B of the 996 B budget.
3. **"From the two names" is superseded text.** D6's amendment re-keyed the record to the invitation secret, and `RecordKey`'s own comment refuses name-derivation by name: *"names are the public identifier this plan exists to have people say aloud, so a key derived from them is obfuscation against a scraper and not confidentiality"* (`invitation.go:197-199`). **A builder satisfying the criterion as written ships the weaker thing and passes the gate.** Both bullets amended above; the identical phrasing at the phase level is amended too.
4. **D16's DHT fetch cadence cannot work.** `getput.Get` has no early return for a mutable item (`goto receiveResults`, `exts/getput/getput.go:110`), and mutable is forced on us — an immutable target is `sha1(bencode(V))`, which you must already hold the record to compute. `defaultMaxQuerySends = 1` (`transaction.go:24`) with a flat 2 s timeout (`dht.go:24-27`, `server.go:1052-1057`) means every unanswered query costs 2 s, and at Alpha 15 against Nib's own measured ~60 % reply rate essentially every round pays it. A 2 s budget expires inside round 1, always; `op.queried` is per-operation, so each attempt restarts from our routing table and **never reaches the eight nodes that store the record**. D16's own text says its numbers are *"tunable, not law — the structure above is the law"*, so this is a premise amendment and not a decision strike. Amended at D16.

**Two doors the DHT is open through today, neither of which this slice created** *(2026-08-20)*:
- **Nib is an unbounded remote memory sink.** `dht.go:171-220` sets no `Store`, so `NewServer` installs `bep44.NewMemory()` (`server.go:219-220`) — a bare `map[Target]*Item`, no cap, no eviction (`bep44/memory.go:21-28`). Eviction happens only on a read of that exact target, so an attacker who puts and never gets leaves entries resident forever. **Closed by refusing inbound `put`/`announce_peer` at `gateQuery`**: Nib is a DHT *client*; it needs to issue put/get, not serve them.
- **A third remote process kill.** `exts/getput/getput.go:55` dereferences `*r.Seq` inside a `&&` once the target matches; `Seq` is `*int64, omitempty` (`krpc/msg.go:85`). The library's **own server guards this exact field** on the inbound `put` (`server.go:578-583`) — the check exists and was applied in one direction only, the identical asymmetry P04.S02 found at the query handler. Reachable by the ~8 nodes that stored the record, an on-path observer of the `put`, and **any roster member** — in a two-party ceremony, the counterparty. Closed in `screened`, which already decodes every inbound datagram.
- **And `ServerConfig.Exp` is unset**, so `Wrapper.Get` computes `created.Add(0).After(now)` → false → deletes and returns not-found for **every** item (`bep44/store.go:50-64`). `Exp: 2 * time.Hour` exists only in `NewDefaultServerConfig` (`server.go:167`), which Nib does not use. Nib serves nothing, including its own record.

**The D34 ordering correction** *(2026-08-20)*. D34 puts the DHT disclosure on the ceremony screen, which is **P06 — built LAST, after P07**. Nothing in `README.md`, `web/` or `docs/` mentions the DHT to a user today (verified by grep). Three shipped claims become false the moment this slice lands: `README.md:45` *"Runs 100% offline, no account, no telemetry ✅"*, `:745` *"This is the only call Nib makes on its own"*, and `:636` *"Binds `127.0.0.1` only — never reachable from the network"*. **Publishing a user's IP to a public global network several phases before the sentence that tells them is the wrong order however the phases are numbered**, so the disclosure lands here. D34 is not struck — its ceremony-screen line is still owed at P06.

**Also settled here, and deliberately not built:** the interesting in-roster power is **preemption, not forgery**. Any invitation holder can publish at `seq = MaxInt64`; `CheckIncoming` then refuses every later write at that target (`bep44/item.go:151-165`), and the honest party's failure looks exactly like an offline peer. The inner signature does not stop it. A counter that distinguishes *"a record was present and failed the inner check"* from *"nothing was there"* is what makes it visible; filed for P04.S04, whose scope is already the flood cap.

Tasks:
- T01 — the disclosure: `README.md`'s three claims restated, and a line in `nib`'s own output.
- T02 — close both doors: `gateQuery` refuses inbound `put`/`announce_peer`; `screened` drops a response carrying `k` with no `seq`; `ServerConfig.Exp` set so our own record is served.
- T03 — keys and domain tags in `internal/ceremony`: `RecordKey(hop)` (per hop, so D30's P05 clause becomes satisfiable at all); a separate `nib-bep44-seed-v1/hop-%d` for the ed25519 seed, so the value that may be logged is not the write authority; a **secret-keyed** salt; and a domain tag as the first chunk of every preimage this identity key signs — `RosterHash` has none today.
- T04 — the candidate record: length-prefixed preimage binding version, ceremony id, hop, party, publisher SPKI, expiry and candidates, with the exclusions written down as `RosterHash` does; ECDSA via `sign.SignDigest`; XChaCha20-Poly1305, 24-byte random nonce, AAD = salt‖hop‖version‖seq.
- T05 — publish and fetch in `internal/rendezvous`, opaque bytes only (L1 forbids importing `ceremony` and `sign`, and its comment names this slice).
- ~~T06 — the republish loop: this package's first background goroutine.~~ **DROPPED during implementation (2026-08-20), and the reason is a simplification rather than a deferral.** D16 requires only that *"a published record outlives the race that depends on it"* — the connect deadline is 300 s, the record carries its own expiry, and a BEP-44 item lives ~2 h at each storer (`server.go:167`). One publish therefore covers the whole race with an order of magnitude to spare, and a refresh loop would be machinery with no requirement behind it. Dropping it also removes, rather than manages, this package's first background goroutine and every hazard that came with it: a `sync.Once` self-deadlock if the loop ever called `Close`, a lifetime context used as a per-attempt budget, and a monotonic ticker that does not advance across suspend while the storers' wall-clock expiry does. If a later phase needs a rendezvous that outlives one race, it can add the loop against a real requirement.
- T07 — counters, and a standing reader for them: **`nib rendezvous`**, the DHT's analogue of P03.S05's `nib discover` and for the same reason. Before it, this package had **thirteen counters and no reader outside its own tests**, because nothing in the shipped binary imported it at all.
- T08 — the PLAN amendments this grill earned.
- T09 — the coverage, red-proved, with the vacuity guards the grill named.

#### P04.S04 — The flood cap *(plan-review C2, D33)* *(done 2026-08-20, v1.113.0)*
Scope: bound how many candidates one rendezvous key may yield. Refs: plan-review C2, D33.

Acceptance *(amended 2026-08-20 at the slice grill — three premise corrections, each measured)*:
- ~~**A record published by a party who knows both names but holds neither identity key yields no more than N candidates**~~ **A record from the counterparty yields no more than N candidates, and a KEY yields no more than N across the whole race.** Two corrections. *The actor:* post-P04.S03 a party holding neither identity key is refused as a **forgery** before the candidates are ADMITTED (the list is parsed first, so `MaxCandidates` and the address rule both apply before `Verify` runs — but a forger can never reach the accumulator) — driven, the refusal is `ErrCandidateAuthor` — so the criterion as written is satisfied by a check that never reaches the cap, which is a vacuous green by construction. The only actor whose candidates a fetcher honours is the counterparty. *The unit:* "one rendezvous key" is a target yielding a **sequence** of records over D16's 300 s race, not one record, and D16's clock 1 expects candidates to **trickle** — so a per-message bound cannot see the accumulation the criterion is about.
- ~~the N+1th is dropped and reported~~ **an over-cap RECORD is refused whole; the N+1th CANDIDATE is dropped from the accumulated set and reported.** Both shapes are right, at different objects. A record is one signed statement: keeping 8 of 12 acts on a statement the signer never made, and since **the attacker orders the list**, "keep the first N" hands them the selection. The accumulated set is assembled locally from several valid records, and dropping there is what "never fails the ceremony" asks for. Refusing one record does not fail the ceremony either — the ladder falls to the next tier (D19 cause 2). The same reasoning P04.S06 already commits to for the seed list.
- ~~driven by publishing N+50~~ **driven by admitting N+50 at the parse boundary.** Measured: 58 candidates seals to **1,873 bytes against a 996-byte ceiling**, so BEP-44 refuses it before any datagram leaves — the plan's stimulus cannot be published at all. And `Sign` refuses to build one, so the driver must hand-build the preimage or it tests a constructor that declines to produce the stimulus.
- The report is a counter something reads, not a log line — the criterion says *reported*, and P03 established what an unread counter is worth.
- **Every refusal cause is separately counted** *(added 2026-08-20)*. `ErrCandidateFormat` currently fuses an over-cap record with a wrong-key ciphertext — measured, both return the same sentinel — so "reported" is unbuildable until the taxonomy is split.
- **A candidate address that is not a routable public unicast target is refused** *(added 2026-08-20)*. Measured: `127.0.0.1:22`, `[::1]:53`, `192.168.1.1:53`, `10.0.0.1:11211`, `224.0.0.1:5353`, `255.255.255.255:9`, `100.64.0.1:123` and `1.2.3.4:0` all seal, open and **verify end-to-end** today. Under P05's punch that is an inside-the-LAN port sweep run from the victim's own host plus UDP reflection at 53/123/11211. The cap's own arithmetic — "8 candidates × 2 hosts" — assumes legitimate distinct targets and bounds neither.
- **Duplicate candidates are collapsed** *(added 2026-08-20)*. Eight copies of one address concentrate the entire per-candidate budget on one victim, which is the worst case the cap does not bound.
- **The roster is bounded** *(added 2026-08-20, D33's third figure)* — unenforced today, and it is the real per-ceremony multiplier now that the punch budget is per hop.

Tasks:
- T01 — split the error taxonomy: `ErrCandidateTooMany` and an AEAD-failure sentinel out of `ErrCandidateFormat`, so a cause can be counted.
- T02 — drive the existing read-path cap. Deleting `n > MaxCandidates` leaves the whole repo green today, which is what D33's own discharge clause forbids by name.
- T03 — the address predicate: `isPublishable`'s range table moved where both packages reach it, **with `Unmap()`** (a v4-mapped `::ffff:240.0.0.1` clears a v4-prefix loop — measured) and a port floor.
- T04 — the hop-scoped gate: owns the cap, the dedupe, the taxonomy and the counters; composes `OpenCandidate`/`Verify` rather than replacing them, so the separation those two argue for survives.
- T05 — the counters, one per cause, plus `Accepted` and `DroppedOverCap`. This also discharges P04.S03's filed preemption item: "a record was present and failed the inner check" is the sum of the refusals, against `FetchEmpty` for "nothing was there".
- T06 — the roster cap (D33).
- T07 — a standing reader: `nib rendezvous --self-test`, opt-in, with its own banner, publishing and fetching one throwaway record.
- T08 — the coverage, red-proved, and the PLAN amendments this grill earned.

#### P04.S05 — The L1 guard and graceful degradation *(L1, D19)* *(done 2026-08-20, v1.114.0)*
Scope: prove the rendezvous cannot influence acceptance, and that its absence degrades rather than breaks. Refs: L1, D19.

Acceptance *(amended 2026-08-20 at the slice grill; the deepdive read the acceptance path this plan did not author, and three clauses named things that do not exist yet)*:
- ~~**A hostile or absent DHT degrades to the next tier**~~ **The tier-degradation half MOVES TO P05.** There is no tier ordering in the tree to degrade *through*: `peerAddresses` is a two-way if/else — typed address, else LAN browse — and `findPeerOnLAN`'s own doc says the browse is *"deliberately the whole of it for now: the caller reports that rather than falling through to a tier that does not exist yet."* No instrument at this slice can resolve the clause, and building a tier ladder inside S05 would be building P05.
- **without ever affecting which peer is accepted (L1)** — **this is the slice, and it has never been driven.** Every `Dial` in the tree pins the listener it reaches, with two exceptions that are different attacks: one where the dialer was *tricked into pinning the impostor* (the MITM case, caught by the spoken string) and one where the *dialer* is the wrong identity. **Nothing anywhere dials an address whose listener is not the pinned peer**, which is exactly what a lying rendezvous produces.
- **driven in both directions** — the absent case (`errNoPeerOnTheLink` reports rather than hangs) and the hostile case. **The hostile case is driven as an untrusted ADDRESS arriving at the dial, not as a hostile DHT**, because nothing in the shipped binary consumes the DHT — `internal/cli/rendezvous.go` is its only importer in the tree. Same threat, different stimulus, and the ledger says so rather than implying a DHT was driven.
- ~~The degradation is the message D19 names, not a generic failure.~~ **Cause 5 lands here; causes 1–3 move to P05.** Cause 4 is the *generic* failure the clause forbids and is the only thing the ceremony path produces today. Causes 1–3 each need a rendezvous consumer or a tier ordering. **Cause 5 fits entirely inside this slice**: `verifyPinnedPeer` holds `now`, `leaf.NotBefore` and `leaf.NotAfter` at the refusal and discards all three into a bare error with no direction and no magnitude — and by D19's own text it is *"the only one of the five that names a fix the user can perform in ten seconds."*
- **The CONSUMER side of L1 gains its first guard** *(added 2026-08-20)*. Five AST-walking guards exist in the tree and all five are producer-side; none walks `internal/server` or `internal/p2p`. An import-shaped guard cannot work at the consumer — that package's whole job is to hold both halves — so it must be value-shaped, and there is exactly one place it is sharp: `internal/server`'s `candidate` struct carries a **pinned fingerprint and a discovered address in one object**.
- **The guard ships with a negative fixture and a red-proof row** *(added 2026-08-20)*. Every existing L1 row is against the producer-side guard; none plants a consumer-side violation.

Tasks:
- T01 — correct L1's citation (done above): it pointed at a closing brace.
- T02 — the driven L1 test at both tiers: `internal/p2p` over the transport table, and `internal/server` through `/api/session/initiate` with a real fingerprint and an impostor's address.
- T03 — each assertion gets its stimulus (the same rig with the correct pin connects) and asserts the **named** error, not merely non-nil. QUIC needs a different assertion from TCP: a rejected QUIC dial returns *success* and the refusal surfaces on the first read.
- T04 — D19 cause 5: a typed error out of `verifyPinnedPeer` carrying direction and magnitude, and the sentence it produces.
- T05 — the consumer-side guard over `candidate`'s composite literal, plus the pin chokepoint (`dialPeer`, `listenPeer`, `dialAny`).
- T06 — the negative fixture and its `docs/red-proofs.md` row.

**Both open measurements answered at the type level** *(2026-08-19, phase-open)*, and neither forces an amendment:

- **A BEP-44 value is capped at 1000 bytes** (`bep44/item.go:132`). A candidate record of a few hundred bytes fits with room, so S03's record needs no splitting.
- **The KRPC `ip` field carries a PORT, not only an address** — `krpc.Msg.IP` is a `NodeAddr{IP net.IP; Port int}`, the BEP-42 compact 6-byte form. **Caveat 2's premise therefore holds structurally.**

*What that does NOT settle, and S02 still owes:* whether real DHT nodes actually **populate** it. Reading the type proves the library can represent a port; it does not prove one comes back. Caveat 2 is discharged on the representation and **still open on the wire**, which is a distinction worth keeping — the arc's failure-mode #1 is a "verified" claim about a dependency that was verified against documentation rather than behaviour.

~~Slices *(sketch)*: library selection and cached bootstrap; self-address probe and NAT classification; the derived rendezvous key; publish/fetch with expiry; the L1 guard.~~ *(retained; superseded by the firmed slices above.)*

#### P04.S06 — Seed nodes carried in the invitation **(added 2026-08-20 — split out of P04.S03 at its slice grill)** *(D6, D21)* *(done 2026-08-20, v1.115.0)*
Scope: D6's second half. The invitation carries DHT seed addresses so the common case never
consults the shipped list, and a stale or blocked list is not fatal. Refs: D6, D21, D32.

*Why it is a slice and not a task of S03:* it moves a **different** wire format — D21's
invitation, not the candidate record — and it is the only invitation field that will **never**
have a signed counterpart. `MatchesRecord` compares id, roster and `signs` flags
(`invitation.go:260-281`) and the `Record` has no seeds to compare against, so seeds are
permanently unauthenticated attacker-controllable data. That earns its own gate.

Acceptance:
- ~~A cold-start machine bootstraps from invitation-carried seeds without consulting the shipped list.~~ ~~**A machine that cannot bootstrap otherwise uses invitation-carried seeds without consulting the shipped list**~~ **A machine that cannot bootstrap otherwise reaches the DHT through invitation-carried seeds, and no machine that can reaches them at all** *(amended twice, 2026-08-20 — the first amendment kept a trailing clause that its own reasoning refutes)*. **"Without consulting the shipped list" is not merely unmet, it is the opposite of the design that replaced it**: the trigger is *demonstrated* failure, so the shipped list is consulted **first, always, on every machine** — the failed attempt is what arms the seeds. The clause survived the amendment because the amendment rewrote the subject and left the predicate, and as written a builder could satisfy it only by restoring the file-absence branch the same bullet spends a paragraph refusing. What the clause is *for* is the exposure bound, and that is what it now says: seeds are unreachable except from a machine that has already failed without them. `Open`'s branch is `if len(nodes) == 0`, which tests whether the cache FILE is empty, **not whether it works**. A machine with forty stale cached nodes never reaches the seed branch at all — so invitation seeds would be ignored on precisely the machine whose cache has gone bad, which is the case they exist for, and `Bootstrapped` would read 0 with `Seeds` 0 and nothing would say why. Seeds are consulted on **demonstrated failure to bootstrap**, not on file absence. That is what D6's stated purpose asks for ("a stale or blocked list is not fatal"), and it bounds the hostile-seed exposure to machines that already cannot reach the DHT any other way.
- **Seeds are parsed with `netip.ParseAddrPort` and never a resolver** — which *structurally* cannot resolve a hostname, a stronger statement of D6 than `dht_test.go`'s name blacklist. Driven by feeding it `router.bittorrent.com:6881` and asserting the refusal, because the blacklist is name-based and would pass `ParseAddrPort` silently.
- **A seed that is not a routable public unicast address is refused**, reusing ~~`selfaddr.go`'s `reservedRanges` table~~ **`internal/addrscope`'s table — it moved there at P04.S04, so the clause named a symbol that no longer exists** rather than writing a third copy — the second one (`internal/server/fetch.go:66`) is already weaker than the first. Plus a port floor **through a new `addrscope.Seed`, NOT `addrscope.Target` (amended 2026-08-20)**: `Target`'s 80/443 exceptions are D14 TCP-fallback reasoning and a DHT seed is UDP-only, so reusing it would reopen exactly what the floor closes — and those two ports are the likeliest to belong to an unrelated third party's web server.
- **The count is capped and an over-cap list is refused, not truncated** — silent truncation hides the attack. A bad *entry* is a transcription error and is dropped-and-counted; an oversized *list* cannot be, so it is loud.
- **Something calls the validator** — a guard on the predicate alone is P03's recorded lesson repeated (*"a guard tested a predicate and not that anything called it"*).
- **`Stats().Seeds` keeps its meaning, or gains a sibling.** Its doc says "how many **shipped** seed addresses were used" and `Bootstrapped == 0 while Seeds > 0` is the **rot alarm** for the shipped list (`dht.go:63-72`). Counting invitation seeds into it makes the alarm unable to distinguish "our list rotted" from "the invitation's seeds were bad", and `live_test.go:54-57` asserts on it.
- **The eclipse chain is answered in writing**, adopt or accept-with-reason: hostile seeds → a cold-start table containing nothing else → `probeTargets` draws every probe target from that table → the attacker controls every `ip` field → `classify`'s strict-majority rule is satisfied by two VMs in two /24s → the believed public endpoint is the attacker's choice → **D33's 3,000 punch packets are aimed at a victim of their choosing, from the user's IP**. `isPublishable` bounds *which* victim; it does not stop the chain, and a majority rule is no defence when the attacker owns the electorate.
- ~~**A unique seed address per invitation is a read receipt**…~~ **THE DISCLOSURE HALF MOVES TO P06 (2026-08-20): the fact stands and there is nowhere to say it.** `ParseInvitation` has **zero non-test callers** — no HTTP route, no CLI command and no UI parses an invitation anywhere in the tree, and the ceremony panel that would is P06, built last. A disclosure clause with no surface can only be claimed, never delivered. The fact, unchanged and carried: a unique seed address per invitation is a read receipt, and it defeats the reason D6 refused hostnames. DNS tells the resolver operator; an attacker-supplied literal tells the attacker directly, with the recipient's real IP and an exact timestamp, before any ceremony begins. The user is told that opening an invitation causes outbound contact to addresses **the sender chose** — a different consent question from "Nib uses the DHT", needing its own sentence.

Tasks:
- T01 — `addrscope.Seed`: `Routable` plus the port floor, without `Target`'s TCP-fallback exceptions.
- T02 — `Invitation.Seeds []netip.AddrPort`, `omitempty`, and **no version bump** — Go drops unknown fields silently, so an older build ignores seeds and falls back to the shipped list, which is the degradation wanted. Bumping would make every older build refuse a whole invitation over a field it does not need. Recorded against D32 rather than assumed.
- T03 — one `validateSeeds`, called from **both** doors. Validation only at the parse repeats the `MaxRoster` defect verbatim: that bound recipients and left the convener — the party that dials every hop — unbounded.
- T04 — `MaxSeeds`; refuse an over-cap list, drop-and-count a bad entry, and never let a partly-filtered slice escape a refusal.
- T05 — `rendezvous.Seed()` after `Open` (`StartingNodes` is evaluated lazily per traversal, and the CLI opens its socket long before an invitation exists), plus the demonstrated-failure branch.
- T06 — `InvitationSeeds` as a sibling counter, and the two CLI sentences that go false without it.
- T07 — seeds sampled randomly per invitation from the live table, never the issuer's own endpoint.
- T08 — the coverage, red-proved, and the eclipse decision written down.

- **Never `dht.Server.AddNode` for seeds** — a zero-id node fires `go s.Ping(...)` (`server.go:391-394`), one uncancellable goroutine per seed, and `Ping` already discards its context. `StartingNodes` is the path `Open` already uses.
- The seed list never carries the issuer's own endpoint, and is not a stable per-issuer identifier — otherwise the invitation discloses the convener's home IP to every recipient and every mail server between, and two invitations from one issuer link by their seed set.

### P05 — The ladder *(done 2026-08-22, v1.117.112)*
Goal: tiers 2, **3** and ~~3~~ **4** exist **(renumbered 2026-08-16, D8)**, all tiers race concurrently, and the manual path is demoted.

**Phase closed 2026-08-22 (v1.117.104→.112).** Twelve slices S01–S12. Full-repo review (5-agent
fan-out) → hand-off `code-reviews/v1.117.104-p05-close-2026-08-22.md`. Acceptance ledger: **16 MET,
4 NOT EXERCISED, 1 STRUCK** — the four not-exercised are all Dan-only real-network runs (IPv6-to-IPv6
crit 1, IPv4-through-NAT crit 2, both-ends-dependent-NAT crit 3, the UDP-blocked half of crit 7) or
router-side lease expiry (crit 5b); the struck clause is the same-role case (D17 amendment). Review
fixes v1.117.105→.111: the initiate side now reports a words-don't-match verdict as the MITM signal
(not a network error); D19 stops telling a peer who published "you haven't started"; a portmap
`Refresh` mislabel, a udpmux missing recover, a glare/PeerFP contract, and the S12-twin LAN-receive
reachability — each with a red-proof where it earned one. **The graduation pass caught a data race the
D19 fix introduced** (`diagnose()` reading the racy gate on the live-status path) and it was fixed +
guarded. Six findings filed as pending (#248 dead TCP-ceremony path, #250 DroppedOverCap, #251
IPv6-CGNAT advice, #253 lease clamp, #254 reader-scan precision, #255 recover-guard scope) and three
design questions parked for Dan (#248 TCP-ceremony intent, #249 send-flow default, #252 consent-card
validity).

Exit criteria:
- IPv6-to-IPv6 completes with neither side forwarding a port. **(Dan-only run — plan-review W4, 2026-08-17.)**
- IPv4-to-IPv4 completes through at least one endpoint-independent NAT. **(Dan-only run — plan-review W4, 2026-08-17.)**
- **A ceremony completes with both ends behind endpoint-*dependent* NAT when exactly one side obtains a port mapping — the case tier 4 cannot serve. (added 2026-08-16, D15)**
- **A mapping is never held while no ceremony is armed, and is explicitly deleted from the router on teardown and on cancel — driven, not asserted. (added 2026-08-16, D15; split 2026-08-17, plan-review W1)**
- **After SIGKILL the mapping is absent from the router within one lease period — driven by killing the process and polling. (added 2026-08-17, plan-review W1.)** The original bullet said "gone … after teardown, cancel and crash alike", which is unmeetable as written: a crashed process deletes nothing, and D15's actual mechanism for that case is lease expiry. One sentence covering all three let a builder satisfy two and call it done.
- **When the two DHT observations caveat 9 depends on do not arrive, cause 3 degrades to cause 4 and that is the expected outcome, not a defect. (added 2026-08-17, plan-review I2.)** Stated as acceptance because it will otherwise read as a bug to whoever first tests it.
- **Every tier that ends in a dialable address completes over TCP as well as QUIC, proven with UDP blocked. (added 2026-08-16, D14)**
- **A peer whose clock is more than the transport's tolerance out produces D19's fifth cause — naming the direction and the approximate size of the disagreement — and never cause 4. (added 2026-08-18, D35.)** Driven by skewing one instance's clock, not by asserting on a constant: the tolerance is asymmetric (`−transportSkew` / `+transportTTL`) and both sides verify each other, so the test must skew in **both** directions to find the binding one.
- **The diagnosis is derived from the rejected peer's own certificate, with no additional round trip and no new wire field. (added 2026-08-18, D35.)** A test that reads the skew from a value the peers exchanged cannot see this — the handshake it is diagnosing is the thing that failed.
- All tiers are attempted concurrently; the first to complete is used and the rest are cancelled. **(amended 2026-08-21, P05.S04 — Dan. The DHT PUBLISH waits for the LAN window; the race does not.)** An armed session publishes its candidates to the public DHT only once its own listener has gone the LAN window without an inbound connection. The cost is bounded and one-sided: about two seconds added to the remote path, against a 300 s connect deadline, and **nothing published at all in the same-office case** — which D8's own "why LAN first" calls the most common. Unpublished, neither party's IP, permanent SPKI, nor the two-party correlator a shared BEP-44 target creates reaches a stranger. **This does not touch criterion 11**: the arm waits on its OWN listener, not on another tier's gathering, which is the distinction that makes it implementable at all — the publishing side is the arm, and the arm announces rather than browses, so it has no browse result to wait on.
- **The racing dialer, the DHT self-address probe and the established session share ONE local socket. (added 2026-08-21, P05 phase-open, caveat 7.)** Caveat 7 names P02 and P04 as the phases it binds and the 2026-08-17 plan-review pin gave each of them a socket-sharing criterion of its own; **it binds this phase too and was never written down here.** Read at the line: `p2p.QUICDial` binds a fresh `net.ListenPacket` and its own `udpmux` PER DIAL (`internal/p2p/quic.go:82-86`), so a racer built on today's dialer multiplies the constraint by the candidate count instead of satisfying it — and a mapping obtained for a socket the session does not listen on is useless under any NAT, which is caveat 7's whole point. **What discharges this specifically:** the winning channel's local `AddrPort` equals the one the mapping was requested for and the one the probe observed — asserted on the socket, not on the intent to share it.
- **A candidate arriving late joins the race in flight; no tier waits on another tier's gathering. (added 2026-08-16, D16)**
- **Simultaneous success on both sides converges on one channel by the lower-fingerprint rule, driven by forcing the glare rather than waiting to observe it. (added 2026-08-16, D17)**
- ~~**A same-role pair stops on the surviving channel before any verification string is derived; no document byte and no session-derived word exists at that point. (added 2026-08-16, D17)**~~ **STRUCK 2026-08-21 (P05 phase-open, propagating D17's own 2026-08-18 amendment — reversible).** D17's amendment states it in as many words: *"roles are not chosen at all, so a conflict cannot exist"* — every role is READ from the Ceremony Record's roster. **P06's identical clause was struck that same day** (see it below, struck with the D17/D23 reason); this one was missed, so the phase carried an exit criterion for machinery the plan had deleted. It is unsatisfiable rather than merely stale: no screen offers a role, so no same-role pair can be constructed to drive it. Nothing is lost — the substance it protected (the machine never decides whose copy is authoritative) is held by the signed record and by L3, which is what D17's amendment says replaces it.
- **Nothing in the race emits at full rate for the whole deadline: retry cadences step down, and a published record outlives the race that depends on it. (added 2026-08-16, D16)**
- **Losing the channel before confirmation re-races and re-confirms; losing it after confirmation ~~fails the ceremony~~ **restarts the hop and re-delivers rather than re-signs (amended 2026-08-18, D18, D24)**. Both are driven. (added 2026-08-16, D18)**
- **The armed listener's wait is bounded by the ceremony, not by a five-minute constant, and still accepts exactly one pinned peer and serves exactly one session. (added 2026-08-18, D16 amendment.)** `sessionAcceptTimeout` is 5 min today (`internal/server/session.go:34`), which disarms a party waiting their turn; this is the only bullet in the plan that moves the TRIPWIRE (`internal/server/session.go:24`), and the two clauses it must *not* move are named in it rather than left implied.
- **The three clocks are independent: letting the connect deadline elapse in full leaves both the exchange budget and the ceremony deadline undiminished. (added 2026-08-18, D16 amendment — extends the 2026-08-17 two-clock guard to the third.)**
- **The rendezvous key is derived per hop: two hops of one ceremony publish under different keys, and a party cannot read the candidates of a hop it is not in. (added 2026-08-18, D30.)** **(2026-08-21, P05.S04 — the clause is TRUE AS WRITTEN again, and the amendment that weakened it is withdrawn.** It was struck mid-slice because per-hop derivation does not reach D30's own stated harm: `derive` takes the secret, the ceremony id and an info string, with no per-party input, so every holder of a shared secret computes every hop's key — `RecordKey`'s doc conceded it in as many words. **Dan's call, 2026-08-21: mint one secret PER PARTY.** Under D22's convener hub every hop is convener-to-party and two counterparties never connect, so a per-party secret is shared by exactly the two ends of the hop it is for. A party can no longer derive another's hop key, locate their BEP-44 target, read their addresses, overwrite their record, or take their key to the sequence ceiling. The wire format is unchanged — only the bytes in `Secret` differ between recipients — so `InvitationVersion` stays at 1. **The limit that remains is D22 and not a gap: the convener holds every party's secret**, because it carries the document and dials everyone; P06's disclosure should say so.)** Driven with a three-party record — a two-party ceremony has exactly one hop and cannot distinguish a per-hop key from a per-ceremony one, so the obvious test is the vacuous one.
- **The race and the glare tie-break are scoped to the current hop: a convener holding candidates for a later party never dials them during this hop. (added 2026-08-18, D30.)**
- ~~Both ends behind carrier-grade NAT fails with an explanation that names the fallback, not a generic timeout **— and the fallback it names is the one that actually applies: a shared VPN or a manual address one side can accept, not a port-forward the carrier's NAT forbids (amended 2026-08-16, D9 pin)**.~~ **Each of D19's four causes produces its own message, and the mapping-class test distinguishes the two NAT classes from two DHT observations. Cause 3's message names port mapping and a shared VPN — never a port-forward the carrier's NAT forbids. (superseded 2026-08-16, D19)**

**Slices firmed 2026-08-21 (phase-open).** The sketch's eleven items are re-cut into the slices below —
**twelve since 2026-08-21**, when S03's grill found two listener-side defects that racing
creates and split them out as S02, because a racer landing before them puts the tree in a state
strictly worse than the serial walk it replaces. Three sketch items are not slices and the reasons are recorded rather than left to be
re-derived: the **mapping-class probe** shipped at P04.S02; the **TCP dialer on every dialable
tier** is a property of the racer (S02) rather than separate work; and the **armed-only disclosure
line** is a P06 exit criterion *verbatim* (twice — D15's router line and D34's DHT line), so P05
owes the DATA and P06 the sentence. Two orderings differ from the sketch, each because the
phase-open grill found a reason: the arm-side fix leads because S02 depends on it, and the
ceremony-scoped arm window moves next to the race because 300 s meets 300 s (S02).

*The starting point, read at the line at phase-open:* `internal/server` imports **neither**
`internal/rendezvous` nor `internal/ceremony`, so the whole of P04's output is reachable only from
`nib rendezvous --self-test`; `dialAny` is strictly serial (`internal/server/lan.go:305-331`) under
a comment claiming the opposite (`session.go:977`); neither dialer accepts a context
(`transport.go:258`, `quic.go:71`, which builds its own at `:101`); there is no `Tier` type and no
D19 cause type; and `web/app.js:1026` refuses to POST without a typed address, so P03's shipped LAN
tier is unreachable from the product.

#### P05.S01 — An accepted connection that produces no session does not consume the arm *(D22; criterion 16's second half)* *(done 2026-08-21, v1.117.21)*
Tasks (grilled 2026-08-21):
- T01 — the arm is consumed by a completed SESSION, not by a completed handshake.
- T02 — `sessionAcceptTimeout`'s timer is untouched, so connect-and-fail cannot outlast the arm window.
- T03 — extend `TestAStrayConnectionDoesNotConsumeTheSession` to the completed-handshake case.
- T04 — correct the two stale `internal/server/l1_test.go` comments describing the pre-v1.117.1 inline handshake.
Scope: `runSession` accepts one connection, `break`s, and disarms on any later error
(`internal/server/session.go:461-477`). The loop already tolerates a *failed handshake* — it keeps
accepting, because "refusal is free for whoever connects and expensive for the user". It does not
tolerate a **completed handshake that produces no session**, and that is the same argument one step
later. A live gap today, rarely reached; S02 makes it routine, because several candidates reach one
listener and the racer closes the ones it did not pick.
Acceptance:
- A connection that completes a pinned handshake and then closes without producing a session leaves the listener **armed**, driven — not asserted on a flag.
- The arm window still bounds it: connect-and-fail repeated cannot hold the listener past `sessionAcceptTimeout`.
- Still **exactly one pinned peer and exactly one session per arm** (D22) — the half of criterion 16 that must not move while its other half does.

**Ledger: 3 met / 1 not exercised.** The unexercised clause is *"connect-and-fail repeated
cannot hold the listener past `sessionAcceptTimeout`"*, asserted structurally (the deadline is
fixed at arm time and the timer resets to the remainder, both AST-guarded) and **not driven** —
driving it means letting a five-minute constant elapse and there is no injection point until
**S02** makes the arm window ceremony-scoped. Recorded rather than folded into the structural
green.

**What the slice's review changed, because it is the phase's first lesson.** The rule was first
implemented as an ENUMERATION of the outcomes that spend the arm, and the enumeration was the
wrong shape rather than merely incomplete — it omitted `p2p.ErrVerificationDeclined`, the
man-in-the-middle signal, so a listener whose user had just said *the words do not match*
re-armed and retried automatically (measured: two full rounds in 0.47 s, status still reading
armed) — the precise retry `internal/p2p/verify.go` says must never be invited. It also claimed
an unanswered consent left the arm when `Confirm` returns `accept=false, err=nil` on timeout, so
that case had always arrived as a decline. Replaced by one question the enumeration was a proxy
for: **did this connection put anything in front of the user?** Its default for an unanticipated
error is the pre-slice behaviour, so only the never-reached-anyone case is loosened.

#### P05.S02 — The listener under concurrent connections *(D22, caveat 7; prerequisite for S03)* *(done 2026-08-21, v1.117.22)*
Tasks (grilled 2026-08-21):
- T01 — a completed handshake parked on `ready` releases its semaphore slot when its connection dies.
- T02 — the QUIC listener stops handshaking on the accept path, or bounds a connection that opens no stream far below 30 s.
- T03 — drive N concurrent pinned connections against one armed listener, both transports.
- T04 — assert the (N+1)th peer still gets in after N losers are closed.
Scope: **the peer side of racing, built before the racer so no intermediate state is worse than
today.** S01 made the arm survive a connection that produced no session; this slice makes the
listener survive *several at once*. Both defects are live today and reachable by anything that
opens more than one connection — they are simply unreachable from our own dialer while
`dialAny` is serial, which is why no phase has met them.

**The two defects, read at the line, and the reason racing creates them.** Today at most ONE
connection ever completes a pinned handshake against an armed listener, because `dialAny`
returns on the first success. Racing a dual-stack or multi-homed peer completes all of them.

- **TCP: a completed-but-unaccepted handshake holds a semaphore slot until disarm.**
  `tlsListener.ready` is **unbuffered** (`internal/p2p/transport.go:397`) and the handshake
  goroutine's `defer func(){ <-l.sem }()` (`:451`) does not release until its `select` on
  `ready` resolves (`:456-460`). Closing the connection from the far end does **not** unblock a
  goroutine parked on a channel send, and `handshake` clears the deadline before returning
  (`:484`), so nothing times it out. Seven losers therefore hold **7 of the 16
  `maxConcurrentHandshakes` slots** for the whole ceremony — half the pool that constant's own
  doc (`:403-416`) sizes against an fd-exhaustion attacker on the segment, spent on ourselves.
  At sixteen the pool fills, the accept loop blocks at `:442`, and the genuine peer may never be
  handshaked at all.
- **QUIC: the accept path still handshakes inline.** `quicListener.Accept` is strictly serial
  (`internal/p2p/quic.go:201`) and then blocks **on the accept path** in `qc.AcceptStream(ctx)`
  under `handshakeTimeout` = 30 s (`quic.go:218-220`) — the exact head-of-line defect
  `transport.go:366-383` records as fixed for TCP and which was never fixed here. A loser whose
  QUIC handshake SUCCEEDED is queued and returned by `Accept`; it never writes, so it never opens
  a stream. `quic.go:188-200` argues the outer wait is safely unbounded because a *failed*
  handshake never surfaces — true, and it does not reach these, whose handshakes succeed.
  Worse, `runSession` resets the arm timer to the REMAINING window (`internal/server/session.go:524-529`),
  so stalled losers burn real arm window and can disarm the session with the winner still queued.

Acceptance:
- **N pinned connections opened at once against one armed listener leave exactly one session
  served and N−1 released** — driven by opening them, on **both** transports, not asserted on a
  counter.
- ~~**A completed-but-unaccepted handshake releases its `maxConcurrentHandshakes` slot when its
  connection dies**~~ **— amended 2026-08-21 at implementation: the slot is never held in the
  first place.** The clause described a mechanism (release on death) and the build found a better
  one: the hand-off is buffered, so the send completes at once and the deferred release runs
  immediately, whether or not the connection ever dies. Bounding the park at `handshakeTimeout`
  was the first attempt and only shortened the hold to 30 s. **The property is unchanged and is
  what the test drives**: the (N+1)th connection is still handshaked after N are abandoned. The
  vacuous version measures the slot count directly; the property is that a later peer still gets
  in.
- **A QUIC connection that completes its handshake and never opens a stream does not delay the
  next connection's acceptance** — driven with a live QUIC listener, because the 30 s bound is
  exactly what a test that only asserts "it eventually accepted" cannot see.
- The arm is still spent by exactly one session and survives connections that produce none
  (S01's rules, unchanged and re-run).

**Ledger: 4 met.** The first clause's "exactly one session served" is driven by
`TestManyAbandonedConnectionsAreFollowedByExactlyOneSession`, added when reconciling showed the
`internal/p2p` test never runs a session at all; that test's own comment records that it does
**not** red-prove the starvation, and why — `runSession` drains each abandonment as it arrives,
so the starvation needs the server to be BUSY, which is the racer's case. The red proof lives in
`internal/p2p`, at dial 17 of 20.

**Found in this slice's own code by its own review**, and worth carrying: the termination guard
was TCP-only while the protocol it guards had just been copied to QUIC (fixed — it runs under
`eachTransport`, and hung on QUIC the first time it did); a comment describing `ready` as
unbuffered in the present tense, in the change that buffered it; and a `default` branch dropping
a **pinned peer's** connection with no argument stated. Two QUIC hazards were hit twice each —
a stream is invisible until data crosses it, and a close abandons unacknowledged data, so
write-then-close races the frame and the test **hangs rather than fails**. Both are in the seam
inventory for S03.

**Not in scope, and named so it is not absorbed silently:** the ceremony-scoped arm window
(criterion 16's first half) moves to **S09** with the symmetric-racing work. The grill found it
carries a defect no phase has stated — the announcer ticks every `announceEvery` = 500 ms for as
long as a session is armed (`internal/server/lan.go:28`, `session.go:459`), so a window capped at
D33's 30 days is **~5.2 million multicast datagrams broadcasting a stable, never-rotating
six-word identity**, which is verbatim the harm `lan.go:70-75` exists to prevent and is a
violation of this phase's own criterion 14 ("nothing in the race emits at full rate for the whole
deadline"). The arm window cannot simply be extended; that constraint travels with the slice.

#### P05.S03 — The race: concurrent attempts, trickle-in candidates, one connect deadline *(D8, D16, D14, caveat 7; criteria 10, 11, 14, 17 and the socket-sharing criterion)* *(done 2026-08-21, v1.117.23)*
Tasks (grilled 2026-08-21, after a deepdive and two adversarial passes):
- T01 — `p2p.Dial`/`p2p.QUICDial` take a context. The `timeout` parameter STAYS as a per-dial floor, so no converted call site can lose its bound. Property: no goroutine outlives a successful dial; cancellation is expressed only as an error return.
- T02 — D16's constant block, with `connectDeadline < sessionAcceptTimeout` asserted. Delete `dialPeer`/`sessionDialTimeout` and correct the four descriptions that call them live.
- T03 — the racer: ONE mux and ONE `quic.Transport` for the whole race (caveat 7), trickle-in, bounded concurrency, keyed on `(AddrPort, Transport)`, every loser closed at the decision.
- T04 — the error surface: `*p2p.ClockSkewError` survives aggregation; transports validated before the race.
- T05 — re-point the three tests the deepdive named as going vacuous.
- T06 — criterion 17's honest guard, whose red proof is wiring the race context into the established conn.

**Ledger: 4 met / 1 not exercised / 1 not measurable at this granularity.**

- *"All tiers are attempted concurrently; the first to complete is used and the rest are
  cancelled."* — **met.** A dead candidate ahead of a live one no longer delays it (0.76 s
  against a walk's 6 s), and nothing abandoned is left live at the peer.
- *"A candidate arriving late joins the race in flight; no tier waits on another tier's
  gathering."* — **met**, driven on a channel that is never closed, so a drain-then-race
  implementation times out rather than passing.
- *"Nothing in the race emits at full rate for the whole deadline"* — **the size half met**
  (the cap drops the excess and reports it, red-proved by silencing the report); **the rate
  half is S08's**, since retry cadences belong to the punch. The clause's second half — *"a
  published record outlives the race that depends on it"* — is **S04's**, which is the slice
  that publishes one.
- *"The three clocks are independent"* — **met for the clause that can fail**, which is that
  the race's context does not reach the established session; red-proved on both transports by
  attaching a teardown to it. The literal reading — burn the deadline, read the exchange
  budget — is **not measurable at this granularity**: every entry point calls `SetDeadline`
  unconditionally, so it compares two literals and passes with the racer never written.
- **The socket-sharing criterion (caveat 7) — NOT EXERCISED, and it is the honest gap.** The
  racer does not yet own one mux and one `quic.Transport` for the whole race; `QUICDial` still
  binds a socket per dial. The criterion binds this phase and nothing here discharges it. It
  moves to **S04**, which is the first slice with a NAT mapping to be wrong about.

**The review found the slice's worst defect, not the grill.** `raceCandidates` read results
with `for r := range results`, which ends only when the INPUT channel closes — and a trickle
source stays open for the whole race by design. Every candidate failing meant the race never
returned, with the local user's document already signed. Five tests, all green, none asking;
`dialAny` cannot reach it because it closes the channel it builds. Fixed by giving the caller
the deadline and watching it. `code-reviews/P05.S03-2026-08-21.md`.

**The criterion this slice's first draft DROPPED, recorded because the drop is the lesson.**
The socket-sharing criterion was added to this phase at its own phase-open on 2026-08-21, and
the draft written the next day omitted it from S03's acceptance list — a criterion added to a
plan and then not carried into the slice that owes it, which is the exact failure this phase has
been finding elsewhere. It is not bookkeeping: `p2p.QUICDial` binds a fresh `net.ListenPacket`
and its own `udpmux` **per dial** (`internal/p2p/quic.go:82-86`), so racing eight QUIC candidates
opens eight sockets and discards seven, and the winner's socket — the one any NAT mapping would
have to be for — is chosen after the fact.

**What it forces, and it is a better design than the draft had.** `quic.Transport` manages
connections on **a single** `net.PacketConn` (its own doc). So the racer owns **one mux and one
transport for the whole race** and dials every QUIC candidate through it: caveat 7 satisfied by
construction, the intended use of the library, and peak cost cut from ~8 fds and ~32 goroutines
to one socket and one read loop.
Scope: replace the serial walk with a racer every later tier feeds. Includes the three things the
sketch did not name and the grill found: **context-aware dialers** in `internal/p2p` (today's take a
`time.Duration`, so "the rest are cancelled" cancels nothing and each loser holds an fd, a
`udpmux` and its `readLoop` goroutine to its own timeout); **D16's constant block**, which does not
exist in code at all; and the **ceremony-scoped arm window**, because `sessionAcceptTimeout` is
300 s and the connect deadline is 300 s started at a different instant, so the tail of every race
dials a closed listener.
Acceptance:
- Criteria 10, 11 and 17 verbatim. **17's honest form:** establish a real session *after* an exhausted race and assert the conn is armed with a **fresh full** exchange budget, not `6m − elapsed`. Asserting `exchangeDeadline == 6m` compares two literals and passes with the racer deleted.
- The candidate key is `(AddrPort, Transport)`, never `AddrPort` alone — ADR-010 exists because a port without its transport names two different sockets.
- The size bound is **per source**, not global first-come: a global cap is won by whoever emits fastest, which re-opens at the race level the capture attack `maxLANCandidates` closed at the browse level (`internal/server/discover.go:145-152`).
- D19 cause 5 survives aggregation: `connectFailure` finds `*p2p.ClockSkewError` by `errors.As` over ONE wrapped error today, and "last" is a lottery under a race.
- `errUnknownTransport` still yields 400, not a 502 after the full deadline — validated before the race, not inside it.
- Every racer goroutine carries `safe.Recover` (`internal/server/lan.go:119-126`); N goroutines on the ceremony path, and an unrecovered panic takes the desktop process with the user's unsaved documents.
- `dialPeer` and `sessionDialTimeout` are **deleted** and the four descriptions that call them live are corrected (`lan.go:285`, `lan.go:293`, `session.go:1037-1040`, and `discover_test.go:556`, which prints `"sessionDialTimeout each"` while formatting `lanDialTimeout`).

**Deepdive, 2026-08-21 (`deepdives/2026-08-21-outbound-dial-path.md`), before the grill. Two
findings that constrain the design rather than inform it.**

**A loser must be CLOSED, not abandoned, and the cost of getting it wrong is six minutes.**
`Receive` arms `exchangeDeadline` = 6 min (`internal/p2p/session.go:154`) and then calls
`runVerification` (`:161`) **before** it reads the document (`:167`); `runVerification` does the
wire exchange first and shows the user the words only after (`internal/p2p/verify.go`). So a
**closed** loser dies at the wire and never reaches the peer's screen — which is exactly why
P05.S01's engagement rule is correct — but an **open-but-silent** loser is accepted by the
peer's serial loop and wedges it for the full six minutes, four minutes past this side's own
300 s connect deadline. The ceremony is lost with no error at either end. **Prompt, synchronous
closing of every loser is therefore a correctness requirement of the racer**, and it is what
makes context-aware dialers a *precondition* of this slice rather than a companion task: without
a context an in-flight loser cannot be cancelled, only closed on arrival.

**And S02's green does not protect it either — verified at the line, 2026-08-21.** S02 fixed the
handshake POOL: `ready` is buffered, so an abandoned connection no longer holds one of sixteen
slots, and the winner is queued rather than never handshaked. **It did not touch the session
path.** `runSession` still calls `serveOneSession` **inline** in a serial loop
(`internal/server/session.go:507-529`), so a connection the racer abandoned but left OPEN, if the
loop accepts it first, still blocks inside `Receive`'s `runVerification` for the full
`exchangeDeadline` — six minutes — while the winner sits in the queue. What S02 changed is which
resource is exhausted, not the wedge. **Synchronous closing of every loser remains the property
this slice rests on**, and a *closed* loser still fails its first read at once, which is why
closing is sufficient.

**S01's green does not protect this slice.** S01 proved the arm survives an abandoned
connection. Under a racer that leaves losers open the arm still survives — six minutes later.
The property holds and the feature dies, which is why the acceptance below cannot be discharged
by S01's tests.

**A cheaper shape, named so the grill decides it deliberately rather than rediscovering it.**
D8 requires the **tiers** to race, not every candidate within a tier. Racing tiers while walking
candidates *inside* one tier bounds concurrency to ~5, preserves `lanDialBudget`'s meaning and
its only test, keeps candidate order meaningful (`discover.go:234`, `lan.go:302-304`), and
shrinks the loser population that the six-minute wedge is dangerous in. It is a genuine trade,
not a free win: it does not satisfy criterion 11's *"a candidate arriving late joins the race in
flight"* for candidates within a tier.

**Tests that go silently vacuous and must be re-pointed by name, not left to the suite:**
`TestDialAnyStopsEvenWithCandidatesLeft` (`discover_test.go:574`) asserts a 31 s ceiling that a
racer satisfies in ~6 s, so **it passes with `lanDialBudget` deleted** and it is the only test of
the wedged-handler bound; `TestDialAnyWalksPastAnImpostorAndLandsOnThePinnedPeer`
(`l1_test.go:32`) is order-shaped ("the impostor FIRST") and position orders nothing in a race;
`TestDialAnyTriesEveryCandidate` (`discover_test.go:335`) matches `len(cands)` rather than what
was tried. Also: **no test asserts the `errUnknownTransport` → 400 passthrough**, and
`TestAClockSkewIsNotReportedAsAPinFailure` (`l1_test.go:336`) types the wrapper string into
itself, so it cannot see the racer losing `*ClockSkewError` to aggregation.

**And the guard S01 left behind:** `armsurvival_test.go:214` requires the accept timer's `Reset`
argument to contain `remaining`, with a setup fatal at `:220` if the timer disappears entirely.
The ceremony-scoped arm window must re-express that guard deliberately — it fails loudly, which
is the point, but it does not update itself.

#### P05.S04 — The armed session gains a ceremony identity, and the DHT becomes a candidate source *(D6, D21, D30; criteria 18, 19)* *(done 2026-08-21, v1.117.35)*
Tasks (grilled 2026-08-21, after a deepdive and a six-adversary attack):
- **R01–R04 land first as a separate remediation commit** — three of P05.S03's own acceptance bullets shipped unmet and unledgered, and S04 makes all three live.
- R01 — `raceCandidates`' feeder gains a `ctx.Done()` arm. Today `for c := range in` (`lan.go:362`) exits only when the caller closes the channel, so a win on a trickle source leaks the feeder AND the drain goroutine forever.
- R02 — `safe.Recover` on all four racer goroutines (`lan.go:360,381,433,436`), plus the first unit test of `safe.Recover` itself, which has none.
- R03 — the candidate bound becomes **per source**; `dropped` splits per source so a lumped counter cannot read backwards.
- R04 — `clocks.go:30-31` corrected: it names a test that has never existed and states a property that is not the right one, since the two clocks start at different instants. The real fix is criterion 16's and is due when a `Record` reaches the server.
- T01 — `addrscope` refuses a zoned address that is not link-local. **Measured: an IPv6 zone bypasses the entire `reserved` prefix table**, because `netip.Prefix.Contains` is false for any zoned address — `[::c0a8:101%eth0]` (192.168.1.1), `[::7f00:1%eth0]` (127.0.0.1), 6to4 and NAT64 all pass `addrscope.Target` today.
- T02 — `CandidateFormatVersion` 1 → 2: every address carries its transport, as a range-checked enumerant (ADR-010 refuses a string for a two-value field). One-way; free now because no v1 record has ever left a process.
- T03 — the version is range-checked **at `parseCandidate`**, returning `ErrVersion`, not left to `Verify` — otherwise a zero-address v1 record parses cleanly under the v2 grammar and every other v1 record is refused with its version discarded.
- T04 — `CandidateGate` keys on `(AddrPort, Transport)`. Today's `netip.AddrPort` key would count a legal dual-transport publisher as `DroppedDuplicate`, whose own doc says "nobody honest does this".
- T05 — `MaxCeremonyLife` bounds `Record.Expires` in `Record.Verify`, symmetric with `MaxCandidateLife`. Today clock 3 is committed to and read against a clock nowhere.
- T06 — the hop is derived from the convener-signed roster at one door. **(corrected 2026-08-21, mid-slice: the first implementation read a CHAIN, `roster[i]`→`roster[i+1]`, from Party's "the order of the roster IS the signing order". D22 is a convener HUB — "the convener… dials each party in roster order; every hop is exactly today's two-party session" — so the convener is one end of EVERY hop and two counterparties never share one. Signing order is not connection topology.)** Today `hop` is unbounded and the only assignment in the tree is `const hop = 0`.
- T07 — `CandidateGate` gains `PublishSalt()`; publish at `RecordSalt(hop, me)`, read at `gate.Salt()`. The only in-tree example is a one-party self-loop and copying it cannot work.
- T08 — `armRequest` gains the ceremony identity; the invitation travels **in the request**, not on disk.
- T09 — the **arm** owns one socket + mux: DHT on `m.DHT()`, listener on `m.QUIC()`; teardown order pinned by a guard (3 of 6 plausible orderings panic the process); a `Server` shutdown path wired at `cmd/nib/main.go:211`.
- T10 — `rendezvous.Server.Close()` cancels and joins, without re-entering its `sync.Once`.
- T11 — `bootstrapBudget` in `clocks.go`; bootstrap runs at arm, not inside the race.
- T12 — record expiry = `connectDeadline + 2*PublishBudget + skew`, clamped to `Record.Expires`.
- T13 — the feed loop: caller-owned ctx, closes its channel on `Done`, feeds `raceCandidates`; replaces `dialAny` at `session.go:966` and `:1041`.
- T13b — **the publish waits for the LAN window** (criterion 10's 2026-08-21 amendment): the arm publishes only after its own listener has gone `browseWindow` with no inbound connection, so a same-office ceremony publishes nothing. The signal is the arm's own socket, never another tier's gathering.
- T14 — `candidate` carries its hop; the racer **refuses** a mismatched hop rather than dialling it (criterion 19).
- T15 — L1's consumer guard widened to ceremony wire types **and its propagation re-shaped** (range statements, `var` specs, func literals) — widening the substring alone is vacuous.
- T16 — `README.md:676,:708,:717` synced, with a guard tying the claim to the import graph.
- T17 — criterion 18's second clause amended by tagged pin (below).
- T18 — D34's disclosure line lands beside the arm control in this slice rather than P06.

**Ledger: 3 met / 1 met with a declared limit / 1 not exercised.**

- *Criterion 18 — "two hops publish under different keys, and a party cannot read the
  candidates of a hop it is not in"* — **met, and the clause is true as written again.**
  Its second half was false by construction under one shared secret; Dan's 2026-08-21 call
  moved to one secret PER PARTY, so a party can no longer derive another's hop key, locate
  their target, read their addresses, overwrite their record, or take their key to the
  sequence ceiling. Driven three-party, because two parties have one hop and cannot
  distinguish a per-hop key from a per-ceremony one. **The limit is D22, not a gap: the
  convener holds every party's secret.**
- *Criterion 19 — "the race is scoped to the current hop"* — **met**, and structurally
  rather than by discipline: the hop travels on the candidate and the racer refuses a
  mismatch. Both controls driven — this hop's candidate passes, and LAN/typed candidates,
  which belong to no hop, are not dropped.
- *A published record's expiry is the connect deadline plus margin* — **met**, derived
  rather than chosen (publish + race + fetch + skew), red-proved against the self-test's
  zero-margin value, and bounded above by the reader-side ceiling.
- *`rendezvous.Server.Close()` cancels and joins* — **met.** Found while testing it that
  `getput.Put` shadows cancellation, so a publish cancelled mid-put returned nil — a false
  success the moment Close began cancelling. **Declared limit: `inFlight.Wait()` is not
  independently red-provable**, because cancellation is prompt enough that removing it does
  not reliably fail; it is a regression guard on a structural invariant and the test says so.
- *L1's consumer guard widened to this slice's wire types* — **met, and the widening alone
  was proved vacuous**: with the vocabulary extended but the taint loop still matching only
  assignments, a planted range-shaped pin passes.
- **The socket-sharing criterion — the probe-and-session half is MET** (the DHT and the armed
  listener on one socket, asserted on the socket and with a real datagram; the teardown order
  driven, its reversal producing a live process-killing panic). **(CORRECTED 2026-08-22, at
  P05.S05's open: "asserted on the socket" was an assertion that could not fail.** The clause was
  discharged by `ln.Addr().String() != cer.end.LocalAddr().String()`, and both sides are
  `e.mux.LocalAddr()` on the same `*udpmux.Mux` — `quicListener.Addr()` is `l.mux.LocalAddr()`
  (`internal/p2p/quic.go:237`), `SharedEndpoint.LocalAddr()` is `e.mux.LocalAddr()`
  (`internal/p2p/endpoint.go:68`), and both bottom out in `m.pc.LocalAddr()`
  (`internal/udpmux/mux.go:202`). It compared a value with itself, for any bind string in any
  address family. The UDP probe beside it proved the socket was *reachable*, never that it was
  *shared*. **What one socket serving two consumers actually means is a demultiplex**, so the
  clause is now driven by sending a QUIC-shaped and a KRPC-shaped datagram to the one address and
  asserting they reach different views (`RoutedLongHeader` and `RoutedToDHT` both move) — which
  also gives two of the mux routing counters their first reader. Red-proved in isolation, with
  the addresses still equal, as `shared-socket-not-demultiplexed`. **The criterion is met on the
  new evidence; it was not met on the old.** v1.117.42.) **The racing-dialer half is
  NOT EXERCISED and moves to S06/S08/S09** — see below.
- **T13b's deferral is NOT EXERCISED end to end.** Hermetically the bootstrap finds no nodes
  and returns early, so the publish never happens *for the wrong reason* and no test can tell
  the deferral from the failure. Filed as a live-DHT verification item rather than collapsed
  into met.

**What this slice found that was not in its plan.** Three live defects — an IPv6 zone
bypassing the whole of `addrscope`'s reserved table (192.168.1.1 and 127.0.0.1 both passed
`Target` when zoned), a two-goroutine-per-race leak in S03's racer, and `getput.Put` shadowing
cancellation. Plus **three of S03's seven acceptance bullets shipped unmet and unledgered**,
because that ledger reconciled against the phase exit criteria and never against the slice's
own `Acceptance:` line. And **my own hop rule was a chain when D22 is a hub** — corrected at
v1.117.31; the doc comment I read was true and simply did not answer the question I put to it.

**The socket-sharing criterion is RE-TIMED, not discharged here.** S03's ledger moved it to S04 on the
premise that "S04 is the first slice with a NAT mapping to be wrong about" — false against the plan's
own text: S06 is the port-mapping client and its scope already reads "Caveat 7 decides where the request
is sent FROM", S07 owns the lease lifecycle, S08 the punch. The **probe + established session** half has
an owner here (the arm, T09); the **racing dialer** half belongs to S06/S08 and to S09, which is the
first slice where the dialing side has a listener at all. An adversarial pass argued the criterion is
over-specified because an outbound dial makes its own NAT state — true of a plain dial, false of a
punch, where the source port must be the one the peer learned (D8 tier 4: "each learns its own mapped
`IP:port`, publishes it, both punch").

Scope: the import that does not exist. `armRequest` is fingerprint/bind/mode/transport
(`session.go:600-605`), there is no `/api/ceremony/*` route, and `ceremony.NewInvitation` has no
non-test caller — so the hop, roster and invitation secret every rendezvous derivation needs are
absent from the server. Tiers 2, 3 and 4 all signal through the DHT (D8's structural correction),
so this gates three tiers rather than one.
Acceptance:
- Criteria 18 and 19 verbatim, driven with a **three-party** record — a two-party ceremony has one hop and cannot distinguish a per-hop key from a per-ceremony one.
- A published record's expiry is the connect deadline **plus margin** (D16). *Not to be copied:* the only code that publishes one today sets `now + 5 minutes` — exactly the deadline, zero margin (`internal/cli/rendezvous.go:533`).
- `rendezvous.Server.Close()` cancels and joins in-flight `Publish`/`Fetch` — with the trap its pending entry records: the in-flight work must never call `Close` itself, since `sync.Once` deadlocks if `f` re-enters `Do`.
- **L1's consumer guard is widened to this slice's wire types before a record reaches a pin.** `wireType` matches the single substring `discovery.` (`internal/server/l1_test.go`), and `CandidateRecord.Fingerprint()` derives a pin from the record's own `SPKI` (`internal/ceremony/candidate.go:164`).

#### P05.S05 — The IPv6 tier *(D8 tier 2; criterion 1)* *(done 2026-08-22, v1.117.51)*
~~Scope: the arm's default bind is `0.0.0.0:0` (`session.go:659`) — v4-only, so tier 2 cannot work today. Dual-stack bind; this host's global v6 addresses become candidates.~~
**Scope re-stated 2026-08-22 (tagged pin, at the slice's open — the premise was MEASURED false, not re-argued).** Two errors, and the second is the one that matters:

- **The line cite was stale.** The default bind is `session.go:807`, not `:659`.
- **The bind is not v4-only. It is already dual-stack, and it always was.** Measured on this host rather than read from the stdlib: `net.ListenPacket("udp", "0.0.0.0:0")` returns a socket with `SO_DOMAIN=AF_INET6` and `IPV6_V6ONLY=0`, whose `LocalAddr()` reads back `[::]:port` — and a datagram sent to `[::1]:port` **arrives on it**. `net.Listen("tcp", "0.0.0.0:0")` is the same. Go rewrites a wildcard listen to `AF_INET6` dual-stack wherever `supportsIPv4map()` holds (`net/ipsock_posix.go`), which is every platform Nib targets; the exceptions are OpenBSD and DragonFly, which it does not. **So "change the bind" is not this slice's work — the bind is already right, and there is no test in the tree that says so, which is why three sessions could read the line and believe it.**

**Where tier 2 actually fails, read at the line.** A dual-stack socket gets a v6 tier no closer, because nothing ever produces a v6 candidate to dial:

- **The DHT never learns a v6 node.** Every shipped bootstrap seed is an IPv4 literal (`internal/rendezvous/seeds.go:69-75`), and `dht.ServerConfig` is built without `DefaultWant` (`internal/rendezvous/dht.go:320-388`), so `find_node` goes out with `Want: nil` and a responder answers with the family of the query source — v4 in, v4 out. `Get` (BEP-44) is the one query that asks for both, so the table is *starved* rather than structurally v4-only.
- **So the probe's v6 half is dead in practice.** `classify(obs, true)` (`internal/rendezvous/selfaddr.go:159`) is correct code with no input, so `SelfAddress.V6.Addr` is the zero value on a real host.
- **And a dual-stack host could not advertise both families anyway.** `publishCandidates` takes `self.V4.Addr`, falls back to `self.V6.Addr` only when v4 is invalid, and publishes **exactly one** endpoint (`internal/server/ceremonynet.go:81-96`). The v6 fallback is unreachable whenever v4 works — which is the case tier 2 is for.
- **Nothing gathers host candidates at all.** There is no ICE-style local-interface enumeration anywhere in the tree; the only two self-address sources are the DHT reflexive probe and the LAN tier's observed peer source address. So "this host's global v6 addresses become candidates" describes a mechanism that does not exist, rather than one that is v4-limited.

**Scope, restated:** a guard that pins the dual-stack bind as a *property* rather than a comment; the DHT asking for and keeping v6 nodes (`DefaultWant`, a v6-reachable seed); a published record carrying **both** families rather than one; and the v6 dial branch (`localWildcardFor`, `internal/p2p/quic.go:147-152`) getting its first test of any kind — it is the only family-selecting function in the tree and it has none.

Acceptance: criterion 1 (Dan-only run, harness reduced to one command); a driven hermetic analogue over v6 loopback/ULA; no v4 regression; **and the dual-stack bind asserted as a socket property, so a platform where it is not true fails rather than degrades silently**.

Tasks (grilled 2026-08-22, after a three-agent deepdive and two rounds of live measurement):
- T01 — the dual-stack bind pinned as a **socket property**, not a comment: a socket bound `0.0.0.0:0` must receive a datagram sent to `[::1]`, asserted through a bare `ListenPacket` AND through `NewSharedEndpoint`, which is the door the ceremony uses. *(done, v1.117.43)*
- T02 — `DefaultWant: {n4, n6}` on the DHT server config, with the measurement that justifies it recorded at the line. *(done, v1.117.43)*
- T03 — one **measured** IPv6 seed literal (`dht.libtorrent.org`, same operator and port as the v4 entry). `dht.transmissionbt.com`'s AAAA is measured SILENT 3 of 3 and deliberately not shipped, recorded so the next reader does not add it from the DNS record. *(done, v1.117.43)*
- T04 — `publishableEndpoints` publishes **both** families; extracted from `publishCandidates` so the rule is drivable without a live DHT. *(done, v1.117.43)*
- T05 — `localWildcardFor` gets its first test of any kind, including the `nil` remote that falls silently to the v4 wildcard. *(done, v1.117.43)*
- T06 — a hermetic ceremony driven **end to end over v6 loopback/ULA**, which is the acceptance clause's own analogue and is NOT discharged by T01: T01 proves the socket answers, not that a ceremony completes across it. **Outstanding.**
- T07 — criterion 1's harness reduced to one command, for the Dan-only run. *(done, v1.117.51 — `--v6` is the one command; `NIB_PAIR_V6_ADDR` moves it off loopback onto a global/ULA address on the same host. The two-machine execution stays Dan-only, named in the harness header beside the v4 one.)*

**Ledger (closed 2026-08-22, v1.117.51): criterion 1 not exercised (Dan-only two-machine run, phase carve-out — T07 built the buildable half); harness-one-command met; hermetic v6 analogue (T06) met and driven; no v4 regression met; dual-stack bind property met (T01).**

**Superseded ledger, kept for the trail — "1 met / 1 met / 2 outstanding":** *No v4 regression* — met, full suite plus the race detector green on `internal/server`, `internal/p2p` and `internal/rendezvous`. *The dual-stack bind asserted as a socket property* — met, red-proved by binding `udp4` (both the bare socket and the shared endpoint). *A driven hermetic analogue over v6 loopback/ULA* — **not met**, T06. *Criterion 1* — **not exercised**, and it is Dan's run by the phase's own carve-out; T07 is the buildable half.

#### P05.S06 — The port-mapping client: PCP, then NAT-PMP, then UPnP-IGD *(D15; caveats 6, 7, 8)* *(done 2026-08-22, v1.117.56)*
Scope: tier 3's mechanism. **Caveat 6 is discharged or refuted in this slice** — no Go port-mapping dependency exists in the tree and its licence-compatibility is explicitly an unverified assumption; the caveat's own fallback ("if only some protocols are covered, the tier still ships — with narrower router coverage, recorded rather than assumed") is the acceptable outcome. Caveat 7 decides where the request is sent FROM.
Acceptance: the 3 s budget; all three protocols failing is an ordinary tier miss and never an error; whatever coverage is achieved is **recorded, not assumed**; and if a dependency lands, `THIRD-PARTY-NOTICES.md` regenerates and its licence claim is true of it.

**Firmed 2026-08-22 (slice-open, read against the tree).** The mapping produces a **self-address**
(the mapped external `IP:port`) that joins the published record beside the DHT reflexive probe —
`publishableEndpoints` (`ceremonynet.go`) is where it lands, exactly as S05's v6 endpoint did.
**Caveat 6 is discharged by NOT taking a dependency:** NAT-PMP (RFC 6886) and PCP (RFC 6887) are
small binary UDP request/response protocols to the gateway on port 5351, implemented natively —
which removes the licence question the caveat is about rather than answering it. **UPnP-IGD is
SSDP discovery + SOAP over HTTP, an order more code and the one most consumer routers actually
have**; per caveat 6's own fallback it is the tier's *narrower-coverage* arm and is built last,
and if it is deferred that is recorded, not assumed. **Caveat 7's clause is the internal port:**
the mapping requests the shared endpoint's `LocalAddr().Port` as its internal port, so the
mapped external port belongs to the socket the session actually answers on. The lease *lifecycle*
(armed-only, refresh, delete-on-every-exit) is **S07**, not here; this slice is the client that
obtains and releases one mapping on demand.

Tasks (firmed 2026-08-22):
- **T01 — the NAT-PMP + PCP wire codec, pure functions with a driven mock-gateway test.** *(done, v1.117.52)*
  Request/response encode+decode for both protocols (map a UDP or TCP port for a lease, and the
  delete form — lease 0), with the result-code and epoch fields parsed rather than skipped. Pure
  `[]byte`↔struct so it tests without a socket; driven end to end against an in-process UDP mock
  gateway that speaks both. **This is the caveat-6 discharge**: it is the whole dependency,
  written rather than imported.
- **T02 — gateway discovery.** The default gateway address, read from the OS without a
  dependency (the routing table / `/proc/net/route` on Linux, the platform equivalents behind a
  `//go:build` split where they differ — a real gap declared, never a stub). No gateway is an
  ordinary tier miss. *(done, v1.117.53 — Linux `/proc/net/route`; !linux returns ErrNoGateway, the real error, not a stub.)*
- **T03 — the client: PCP then NAT-PMP, within the 3 s budget.** PCP first (it supersedes
  NAT-PMP), falling back to NAT-PMP; the first to return a mapping wins; all failing is a tier
  miss and never an error. The internal port is the shared endpoint's, per caveat 7. Both UDP
  and TCP mapped when both transports are offered (D15). *(done, v1.117.54 — one 3 s budget, PCP-then-NAT-PMP, driven against the mock; miss and cancellation are distinct outcomes.)*
- **T04 — the mapped address becomes a published candidate.** *(done, v1.117.55 — grilled by a
  two-lens agent pass, security + correctness, before a line was written.)* Wire the obtained
  external `IP:port` into the published set behind the armed-only posture. The grill changed the
  shape in four ways, all now built:
  - **NOT into `publishableEndpoints`** (grill F5): that function is a pure function of a DHT
    observation with no network reach, and a live `portmap.Client.Map` call has no place in it.
    Obtained in `publishCandidates`, appended to the slice it returns.
  - **Screened before the append, drop-and-continue** (grill F1/#1, the must-fix): a router
    legitimately returns a private/CGNAT/sub-1024 external (double-NAT, carrier-grade NAT —
    caveat 8), and `preimage`'s `addrscope.Target` is **all-or-nothing** — a bad mapped addr
    reaching `Sign` aborts the WHOLE record, dropping the good reflexive candidates. So it is
    filtered at `screenedMappedEndpoint` and dropped on failure. Red-proved.
  - **Its own 3 s clock** (grill F2): `portMapBudget`, a new constant in the D16 block, not the
    45 s publish context — the clock-independence pin.
  - **The miss is swallowed** (grill F8): `ErrNoMapping`/cancellation leave `addrs` unchanged.
  Caveat 7 internal port is `c.end.LocalAddr()` (the shared UDP socket); the mapped pinhole is
  published as QUIC unconditionally (ADR-010, grill F2/#2 — a UDP mapping labelled TCP is a
  signed lie). **UDP/QUIC-only, declared** (grill F4): the whole publish path runs only for a
  QUIC arm, so D15's "both UDP and TCP" has no call site until a TCP publish path exists.
  **Lifecycle deferred to S07** (grill F3): T04 obtains and publishes one mapping; refresh
  (load-bearing across the 300 s race vs the ~120 s lease) and delete-on-every-exit-path are
  S07, built as one scheduler. The residue until then is a ≤120 s inbound hole that self-expires
  — D15's own crash-safety lease, bounded and recorded. The live real-router mapping is Dan-only
  (criterion 3); the buildable half — the screen and the miss-swallow — is tested here, the
  client against a mock gateway at T01–T03.
- **T05 — UPnP-IGD, or its recorded absence.** SSDP+SOAP for the routers NAT-PMP/PCP miss; if
  the coverage is narrower than all three, that is recorded in the tier's own doc and in the
  caveat, not assumed. `THIRD-PARTY-NOTICES.md` regenerates only if a dependency lands (none
  is planned).
- **T06 — seam inventory rows**, per `instrument.md`: the tier's attempt/won/miss observables
  and the caveat-7 internal-port assertion.

#### P05.S07 — The mapping lease lifecycle *(D15; criteria 4, 5)* *(done 2026-08-22, v1.117.58)*
Scope: D15's lifecycle is law, not configuration — armed-only, short lease refreshed while armed, explicitly deleted on every exit path including cancel and error.
Acceptance: criteria 4 and 5 verbatim, the second driven by killing the process and polling. The refresh's interaction with **both** bounds is settled here: D33's packet budget and `CandidateGate`'s slot cap are different resources and each has a pending entry (items 20, 21).

**Firmed 2026-08-22 (slice-open, read against the tree).** S06 obtains ONE mapping and deletes
nothing (`appendMappedCandidate`, a one-shot at publish time); S07 turns that into a managed
lease. The blast radius is a concurrency seam — a refresh goroutine sharing mapping state with
the publish path and with `close()` — so this slice is grilled before a line is written.

Tasks (RE-firmed 2026-08-22 after the slice grill — the grill found the first firming was not
codeable: eight concrete holes, two of which meant the mapping could not be deleted, plus a
LIVE data race in shipped code that S07 now owns):

- **T01 — `Map` returns a delete handle, and `Client.Unmap` uses it.** *(grill C1.)* Today `Map`
  discards which mechanism won and, for UPnP, the `controlURL`+`serviceType` the SOAP delete
  needs — so a UPnP mapping is undeletable and "send both PCP/NAT-PMP deletes" cannot reach it.
  `Map` returns a `Mapping` carrying the mechanism (PCP / NAT-PMP / UPnP) and the UPnP control
  handle; `Unmap(ctx, m)` deletes via that mechanism (lease-0 for the socket protocols,
  `soapDeletePortMapping` for UPnP). Best-effort — a failed delete falls back to lease expiry.
- **T02 — the managed mapper.** Obtains the FIRST mapping synchronously (the publish needs the
  address). A refresh goroutine bound to the **ARM ctx, not the 45 s publish child** *(grill
  C4)* — the publish budget expiring must not kill the refresh mid-race. Refresh at ~half the
  **granted** lease, **requesting the same external port** for stability, which needs a
  suggested-external-port entry point `Map`/`Refresh` does not have today *(grill C7)*. It
  records the mapping the moment the request is SENT, not only on screened success, so an
  obtain that races a cancel is still deletable *(grill P1)*. If the router assigns a DIFFERENT
  external port on refresh, that is detected and left to item 20, not silently continued
  *(grill P2)*.
- **T03 — delete on EVERY exit path, on a FRESH context.** *(grill C2.)* `close()` cancels
  `stopNet` first, so a delete on anything derived from the arm ctx is an instant no-op (`Map`'s
  first line is `ctx.Err()`). The delete uses a fresh, short, bounded context, and is bounded /
  async so a slow or absent IGD does not stall the user's Cancel/quit teardown *(grill P3)*.
  Criterion 4 driven against a mock gateway that RECORDS the delete, not a flag.
- **T04 — never held while unarmed; the crash floor is the GRANTED lease.** *(grill C8.)* The
  requested lease is short (120 s < the 300 s deadline, trivially true and not the point); the
  crash floor is whatever the router GRANTS, which a test shows can be 7200 s. Recorded honestly
  in the caveat, with the mapper re-requesting or capping when granted ≫ requested. Criterion 5's
  kill-and-poll is Dan-only against a real router.
- **T05 — the concurrency guard, and it fixes a PRE-EXISTING race.** *(grill C3, C5, C6.)*
  `ceremonyID` has no lock today, yet `stopNet` is written by the armed goroutine
  (`ceremonynet.go:311`) and read by `close()` (`ceremonyid.go:104`) — a live data race that
  `-race` has not tripped only because the write is the goroutine's third statement. S07 adds the
  mutex that covers `stopNet` AND the mapper state AND a `closed` flag: `close()` cancels, marks
  closed under the lock, and NO refresh `Map` fires after — cancel is not join *(C3)*. The mapper
  is stored on `ceremonyID` beside `end`/`rz` so `close()` can reach it *(C6)*.
- **T06 — seam rows**: obtain / refresh-fired / delete-sent (recorded by the mock), the
  requested-lease-vs-deadline invariant, the granted-lease crash-floor note, and the Dan-only
  real-router criterion-5 row.

**Correctly OUT of scope, confirmed by the grill:** item 21 (D33 punch budget — refresh traffic
goes to the gateway ~1 pkt/60 s, not punch packets to the peer); item 20 (CandidateGate slot cap
— refresh reuses the port and does not re-publish, so no slot is spent, *provided* the port stays
stable, which P2 handles); and the dialer side, which opens its own endpoint and never maps, so
its `close()` has nothing to delete.
#### P05.S08 — The one-transport racer: dial through the shared endpoint *(caveat 7's racing-dialer half; S03's deferred T03)* *(done 2026-08-22, v1.117.59)*
Scope: the racer dials its QUIC candidates through the ceremony's **one** shared endpoint
(`cer.end.tr`) instead of a fresh `net.ListenPacket` per dial. This is the racing-dialer half of
caveat 7 — the half `endpoint.go:33-37` defers "to S08/S09 by name" — and it is **S03's T03 as
written** ("ONE mux and ONE `quic.Transport` for the whole race", plan §3053) which S03's own
ledger then deferred to S04 (§3076-3080), and which S04 did not build either. The punch rests on
it entirely: a punch sends from a socket, and unless that socket is the one whose mapping the
peer learned, the hole is opened for a source the QUIC Initial never egresses.

**Why its own slice, split from the punch (the punch's grill, 2026-08-22).** It is testable with
NO punch — it either dials from the shared socket or it does not — and it carries a second hard
problem the punch does not: **close semantics invert on a shared transport.** Today each loser's
`Conn.Close` does `tr.Close(); mux.Close()` on its OWN per-dial socket (`quic.go:106-107`). On
one shared `cer.end`, `tr.Close()` abruptly terminates ALL connections on the socket — the winner
included — and `mux.Close()` makes the DHT read `net.ErrClosed`, which anacrolix/dht turns into a
panic on a goroutine nothing of ours is on. So both loser-close and winner-close must become
**`CloseWithError`-only** — the non-owning path that exists for listeners (`quic.go:186-199`,
`ownsEndpoint=false`) but has no dial counterpart today.

Acceptance:
- The racer's winning channel's local `AddrPort` equals the shared endpoint's — asserted on the
  socket, not on intent (caveat 7's discharge).
- N QUIC candidates race concurrently on the one transport and the losers are torn down without
  killing the winner or the DHT view (driven by racing several, cancelling all but one, and
  asserting the winner survives and the DHT still reads). *Not a concern, recorded so it is not
  re-litigated: CID collision is 2⁻⁶⁴ — `newCIDGen` at construction, inbound demux by destination
  CID (`mux.go:212-222`).*
- TCP candidates and the no-ceremony `dialAny` path (`lan.go:546-557`) keep their own fresh
  sockets — only the ceremony QUIC dial moves to the shared endpoint.
Tasks (firmed 2026-08-22):
- **T01 — `QUICDialOn(end, …)`**: a dial on the shared endpoint's `tr`, returning a **non-owning**
  `Conn` whose closer is `qc.CloseWithError` only (never `tr.Close`/`mux.Close`).
- **T02 — thread `cer.end`** through `raceWithRendezvous` → `raceCandidates` → the per-candidate
  goroutine; the ceremony QUIC dial uses it, TCP and `dialAny` do not.
- **T03 — loser teardown is non-owning**: `raceCandidates` cancels in-flight dials AND closes
  established losers with `CloseWithError`, leaving the shared transport/mux alone.
- **T04 — the dial side's two sockets collapse to one**: `dialerCeremony`'s `cer.end` (the one
  `ProbeSelf` measures and S08b publishes) is the one the race dials from.
- **T05 — seam rows**: the source-port assertion; the winner-survives-loser-teardown observable.

#### P05.S08b — The IPv4 punch *(D8 tier 4, D16, D17, D33; rests on S08)* *(done 2026-08-22, v1.117.63)*
Acceptance: criterion 14's cadence step-down, driven; QUIC-only by D8's transport pin; a punch
completes a ceremony over the opened hole with the arm listening and the initiator dialing.

**Firmed after a two-agent deepdive and the slice grill.** The punch is **symmetric-SEND** (both
sides emit NAT-opening datagrams from their shared socket), not symmetric-dial — so it precedes
S09 (symmetric listen+dial + glare), verified: both punch, then the initiator's QUIC Initial from
`cer.end` traverses the arm's now-open NAT onto the arm's `QUICListenOn` listener on the SAME
`cer.end`; the arm never dials. The parameters (D16/D33): 250 ms for 30 s then 1 s to the 300 s
deadline = **390 packets/candidate/side**, hard-capped at **3,000 packets/hop/side across all
candidates** (8×390 = 3,120 is ~4% over; the cap trims the tail by design). Both figures are law.

Tasks (firmed 2026-08-22):
- **T01 — symmetric publish+fetch, with the dialer's publish SUPPRESSED like the arm's** (grill
  CONFIRMED-3). The dial side runs `ProbeSelf`+publish and the arm runs a `feedCandidates` fetch —
  but the dialer's publish carries the SAME "already-answered ⇒ don't publish" suppression the arm
  has (`ceremonynet.go:340-354`), or every LAN-local ceremony leaks a second DHT correlation
  handle the arm was built to suppress. The arm's fetch **goes through `c.gate`** (author/hop/
  roster/routability screened, capped at `MaxCandidates`), never a raw `Fetch`.
- **T02 — `SharedEndpoint.Punch(addr)`**: a raw datagram on the shared socket via the DHT view's
  `WriteTo` (`mux.go:469-484`, `learns=false`, so it does not disturb the QUIC peer table). The
  payload is a deterministically non-QUIC-long-header, non-bencode (or empty) datagram so the
  peer's mux discards it cleanly rather than quic-go treating a stray `0x80` byte as a connection
  (grill 7b).
- **T03 — the punch sender, on an INJECTED clock** (grill CONFIRMED-5): both sides emit to each
  tier-4 candidate at `punchInterval(elapsed)`, driven by a fake ticker in the test so the real
  loop is exercised — a sender on bare `time.After` makes "driven" vacuous, the S03 trap. Runs on
  the arm too, despite it only listening.
- **T04 — the cadence step-down** as a pure function `punchInterval(elapsed) → 250ms|1s`, the
  constant block owed to S08 (`clocks.go:27`). Asserted at the t=30 s boundary.
- **T05 — the D33 packet budget**: a per-`(hop, side)` counter across all candidates, cap 3,000,
  hard; **checked BEFORE each send** so the 8th candidate cannot overshoot; **not reset on
  candidate churn** (a refreshed S07 mapping is a new candidate spending the same 3,000 faster,
  which is correct); drop-and-report, never fail. New construction — no existing counter counts
  datagrams. Both law figures unreachable from the tunable block (D33's discharge).
- **T06 — QUIC over the hole + the dial-vs-punch-lifetime reconciliation** (grill 7a, the
  high-value gap). `raceCandidates` dials a candidate ONCE with a 6 s budget, but the hole may not
  open inside 6 s (the peer's fetch is on a 5 s cadence, so it may punch late). A tier-4 candidate
  must therefore be **re-dialled while its hole is being opened**, or given a dial budget that
  spans the aggressive punch window — the task list before the grill named neither. A punch that
  never traverses becomes a distinguishable "attempted, no traversal" state (D19 cause 3/4; S11
  renders it), not a generic timeout.
- **T07 — seam rows**: the cadence step-down, the per-side budget drop-and-report, the failure
  state, and the Dan-only real-two-NAT run (IPv4-to-IPv4 through an endpoint-independent NAT).

#### P05.S09a — The stream-direction fix: who opens the QUIC stream follows the role, not the dial *(D17; the glare deadlock)* *(done 2026-08-22, v1.117.65; code v1.117.64)*
Scope: on a QUIC channel, WHICH end opens the bidi stream must follow the document role
(`initiator`), not who dialled — so a baton-holder that wins on the ACCEPT side does not deadlock.
Testable in `internal/p2p` alone (dial a QUIC conn, have the DIALER run Receive and the LISTENER
run Initiate with the stream opened by the Initiate side), no server/DHT/glare needed.

**Firmed 2026-08-22 (S09's grill, C1-C3).** `verificationExchange` has the `initiator` write first
(`verify.go:92`, commit-before-reveal) and the non-initiator read first — a **security property that
must not change**. On QUIC the stream is opened by the DIALER (`quic.go:181` `OpenStreamSync`) and
the listener's `AcceptStream` unblocks only on the dialer's first frame (`quic.go:324-329`). Welded
to who dialled, safe only while dialer==initiator. **The fix is NOT in the session core** (C3:
`Initiate`/`Receive` never call OpenStream/AcceptStream, they only touch `ch.Stream`) — it is in
`quic.go`: the QUIC stream is opened by the party whose role is `initiator`, mapping the existing
`initiator bool` (already passed to `runVerification`, `session.go:123`) onto OpenStream-vs-Accept.
Either QUIC end may open a bidi stream (connection client/server role is fixed at handshake, stream
initiation is independent) — confirmed sound, no chicken-and-egg: the glare fp comes from
`qc.ConnectionState().TLS` (`transport.go:484`), not the stream, so it is in hand before any stream.
Tasks (firmed 2026-08-22):
- **T01 — a "handshaked, no-stream-yet" intermediate** (grill C2): the `Channel` bundles the stream
  and `Channel.check()` refuses a stream-less one (`channel.go:69`), but the coordinator (S09) must
  race+glare over connections whose fp is known before a stream exists. Return the bare `*quic.Conn`
  + fp from dial/accept, and build the `Channel` (open/accept the stream) only after the role is
  known. Touches `channel.go`'s invariant.
- **T02 — the QUIC stream is opened by the `initiator` role**, not the dialer: a `Channel` factory
  that takes the conn + the role boolean and does OpenStreamSync (initiator) or AcceptStream (not).
- **T03 — the role-opposite-dialer deadlock harness**, QUIC-only: a dialer that runs Receive and a
  listener that runs Initiate complete the verification exchange (they would deadlock today). This
  is the test that would go red against the current welded code — the whole point of the slice.
- **T04 — seam rows**: the stream-opener-follows-role observable; the deadlock harness.

**Built 2026-08-22 (v1.117.64 code, v1.117.65 close).** `HandshakedConn` (bare `qc` + `PeerFP`
from the handshake) + `Promote(ctx, initiator)` opening/accepting the stream by role;
`QUICDialHandshakeOn` for S09's coordinator; `QUICDialOn`/accept-loop refactored to thin
`Promote(true)`/`Promote(false)`, behaviour-preserving under `-race`. Red-proved
(`stream-follows-the-dialer`: inverting the role key hangs the harness). **Deviation from T01,
recorded:** built as a separate `HandshakedConn` type rather than relaxing `channel.go`'s
stream-required invariant — `Channel` stays always-complete, which is strictly stronger.
Diff-grill: two agents (correctness; Go-SME + test-teeth), zero findings — pin wiring cross-pinned
(not vacuous), non-owning/close-discipline held, red-proof faithful.

#### P05.S09 — Symmetric racing, the glare join, and the consent re-anchor *(D17; criterion 12; rests on S09a)* *(done 2026-08-22, v1.117.77)*

**Progress 2026-08-22 (the de-riskable pieces first, each committed + tested):**
- **T08 done (v1.117.66)** — `TestOneSharedTransportBothListensAndDials`: the coordinator's one
  unproven library premise, that ONE `quic.Transport` both accepts and dials at once, holds (~10ms,
  right fingerprints; sandbox-EPERM-guarded).
- **T05 done (v1.117.67, red-proved .68)** — the consent re-anchor (grill C4, the biggest hole).
  `consentAnchor{ln | cer}`: `setPending` keys on the ceremony for a hop, so a dial-won receive can
  consent; behaviour-preserving for the manual/LAN path (`ln`, C6); the stale-goroutine guard is
  preserved (red-proof `consent-anchors-on-the-listener-only`).
- **T03 core done (v1.117.69)** — `glareKeepsDial`: the pure tie-break, keep the connection dialed
  by the lower-fingerprint party; both ends converge on one connection. The join/loser-close wiring
  that USES it is T01/T04.

**Remaining (the coordinator integration, one coupled unit):** T01 `connect(cer, cands, role)` — a
private `QUICListenOn` accept loop on `cer.end` AND the dial race, joined via `glareKeepsDial`; T02
merge `/arm` and `/initiate` onto it (both listen AND race); T04 synchronous loser-close for BOTH
the dialled and accepted loser (grill P9); T06 role from `Record.Hop`/`Convener` for ceremony hops
ONLY (grill C6 — manual/LAN keeps role-from-endpoint); T07 split `runSession` into
get-channel/run-exchange/lifecycle so the channel may arrive from a dial; T09 the force-the-glare
tier-4 harness (grill P7 — inject two channels visible to BOTH sides); T10 seam rows.

**Progress 2026-08-22 (the coordinator wired, both sides):** T02 both handlers now go through
`connect` for a QUIC ceremony — `handleSessionInitiate` (initiator) and a new `runCeremonyReceive`
(receiver) replacing runSession+startArmedRendezvous, which `connect`'s feed subsumes (publish,
punch, port-map). T07 `runSession` split: `serveOneSession` takes a `consentAnchor` not a listener,
and the session model gained `armCeremony`/`disarmCeremony`/`disarmWhen` so a connect-arm holds the
ceremony and a cancel rather than an accept listener (a transport permits one Listen —
`TestOneTransportRefusesTwoListeners`). **T06 disposition:** the role comes from the HANDLER
(arm=Receive, initiate=Initiate), which under symmetric racing is authoritative because the convener
holds the document and POSTs initiate — handler-role and `Record.Hop/Convener` never diverge, and
`connect`+`Promote(role)` handles the role-opposite-dialer case without reading the record. Explicit
record-role reading would give the identical answer; recorded as satisfied-by-handler, the record
check available as future hardening if a flow ever decouples handler from role. **Remaining: T09**
(tier-4 force-the-glare harness — the acceptance instrument for criterion 12) **and T10** (seams).

**CLOSED 2026-08-22 (v1.117.77). Acceptance ledger — criterion 12, split on `and`:**
- *"both sides converge on the SAME channel (identical verification string)"* — **MET.**
  `TestCeremonyReceiverDialsAndCoSigns` shows both ends derive identical words end to end; the full
  formation-table convergence (both ends keep the same physical connection) is unit-proven in
  `TestGlareBothEndsConvergeOnOneConnection`.
- *"with the loser closed"* — **MET (unit).** `closeHandshaked`+`drainHandshaked` on every exit, the
  two no-survivor exits fixed by the diff-grill (CONFIRMED-1). A FULL glare (two connections forming
  so there IS a loser) with two real binaries is the two-machine case below.
- *"INCLUDING the role-opposite-dialer case"* — **MET (end to end).** `TestCeremonyReceiverDialsAndCoSigns`:
  the RECEIVER dials the survivor and still receives while the accepter initiates — the welded deadlock
  S09a red-proved, here through the server's connect + Promote(receiver) + serveOneSession.
- **not exercised (Dan-only):** two real binaries BOTH dialing simultaneously on two networks (the full
  glare across a real NAT). The phase's standing two-machine carve-out; the arm `Address` field (T09)
  is what will drive it when a second machine exists.

**T09 built as a Go-level integration instrument** (real server HTTP flow — arm/verify/respond — against
a real p2p peer), more precise than a bash pairrepro extension and needing no invitation-minting CLI; the
two-binary pairrepro is folded into the two-machine VERIFY item above. **T10** seam rows in
`instruments/P05.md`. Diff-grill (2 agents) found and fixed a leaked-loser (moderate) and a dead LAN
announce (HIGH); all other lifecycle/concurrency concerns verified sound.
Scope: both sides listen AND dial over the ONE shared endpoint; a coordinator joins the dialer's
won conn and the listener's accepted conn; a glare tie-break keeps one; the consent/verify gate is
re-anchored off the armed listener; the no-record path stays asymmetric.
Acceptance: criterion 12, driven by forcing the glare — both sides converge on the SAME channel
(identical verification string) with the loser closed, INCLUDING the role-opposite-dialer case.
Scope: both sides listen **and** dial over the ONE shared endpoint; a glare tie-break keeps one
channel; and the document exchange runs on the surviving channel regardless of who dialled it.
Acceptance: criterion 12, driven by **forcing** the glare — including the **role-opposite-dialer**
case, or the deadlock below ships green (TCP is immune, so a tier-1 test passes while QUIC hangs).

**Firmed 2026-08-22 after a two-agent deepdive — the two-line sketch hid a re-architecture with a
QUIC deadlock at its core.** What the deepdive established, all cited to code:

- **THE DEADLOCK (the crux).** `p2p.Initiate` is write-first (`OpenStreamSync` then `writeFrame`,
  `session.go:123-129`) and `p2p.Receive` is read-first (`AcceptStream`, which on QUIC returns only
  after the dialer's first frame, `quic.go:325-333`). This is welded to **who dialled** — safe only
  while dialer==initiator. Under symmetric racing the surviving channel may be dialled by EITHER
  party while the document role comes from the roster, so the **baton-holder can hold the QUIC
  ACCEPT side** and both sides wait to read → deadlock. **TCP is immune** (full-duplex, no
  first-frame gate), so a tier-1 test passes green while QUIC hangs — the tier-4-vs-tier-1 blindness
  this repo keeps hitting. **Fix:** decouple stream-direction from dial-direction — after glare, the
  **baton-holder** `OpenStreamSync`s on the surviving `qc` (either QUIC end may open a stream), the
  other `AcceptStream`s; stream creation is DEFERRED past glare and driven by the roster role.
- **The shape:** a unified `connect(cer, cands, role)` — `QUICListenOn` a private accept loop on
  `cer.end`, run the dialer race on `cer.end`, JOIN the two winners through the tie-break, return
  one `Channel`. Both HTTP paths call it, then run Initiate/Receive by roster role. `runSession`
  welds three concerns S09 must split: get-a-channel / run-the-exchange / lifecycle.
- **The glare rule (confirmed):** compare `myFP` (`sign.Fingerprint(cert)`) vs the peer's
  (`ch.PeerFP`, `channel.go:50`) — a single comparison, not per-channel. Both sides converge on the
  channel dialled by the lower-fp party. A **new settle window** (none exists in the tree) waits on
  first success for the other channel, BEFORE the verification string is derived (D4-per-channel).
  The loser is closed **synchronously** on both ends — an abandoned-but-open loser wedges the peer's
  serial `Receive` for the 6-min `exchangeDeadline` (the S03 lesson, `lan.go:318`).
- **The document role is read from the record**, not who dialled: `Record.Hop`/`Convener`
  (`record.go:399-421`) already computes it; today it only *happens* to line up because the convener
  always dials and the party always listens. S09 routes Initiate/Receive off the record.
- **One shared endpoint CAN listen+dial at once** (QUIC): `e.tr.Listen` + concurrent `e.tr.Dial`,
  inbound routed by destination CID (`quic.go:158-160`) — the mechanism is feasible.
- **The socket-sharing criterion is discharged here** (`endpoint.go:36-38` parks the racing-dialer
  clause "for S09, the first slice where the dialing side has a listener at all"): the winning
  channel's local `AddrPort` equals the mapped/probed socket.

**Firmed 2026-08-22 (deepdive + grill).** The shape is a unified `connect(cer, cands, role)` —
`QUICListenOn` a private accept loop on `cer.end`, race on `cer.end`, JOIN the two winners through
the tie-break, return one `Channel` (built by S09a's factory from the surviving conn + the role).
The grill found the biggest hole is the **consent gate** (C4): `setPending` refuses unless
`se.ln == ln` (`session.go:210`), so a Receive-role party that wins by DIALING (no `se.ln`) cannot
consent → hang. `setVerify` was already freed of this (`session.go:382`); `setPending`/`respond`
were not. And symmetric racing/role-from-record applies **only to ceremony hops** — the plain
two-party co-sign (no invitation) keeps role-from-endpoint (C6).

Tasks (firmed 2026-08-22):
- **T01 — the `connect(cer, cands, role)` coordinator**: on `cer.end`, a private accept loop
  (`QUICListenOn`, NOT `s.sess.arm` — no consent/announce/disarm) AND the dialer race, concurrently,
  into one join point that yields both a dial-won and an accept-won conn.
- **T02 — merge the two HTTP handlers onto it**: `/arm` also races and `/initiate` also listens, both
  through `connect`; `dialerCeremony` gains the listener `endpoint.go:36-38` parks for S09.
- **T03 — the glare tie-break + settle window**: `min(myFP, ch.PeerFP)` at the join; a new short
  settle window BEFORE the verification string (D4-per-channel); the constant is pinned, not guessed.
- **T04 — the loser-close, synchronous, for BOTH the dialled AND the accepted loser** (grill P9): the
  accept loop hands a `*Conn` off and moves on, so the coordinator must OWN and synchronously close
  the losing accepted conn in its own goroutine — an abandoned-open loser wedges the peer's serial
  Receive for 6 min (S03, `lan.go:320`).
- **T05 — re-anchor the consent/verify gate off the armed listener** (grill C4): `setPending`/`respond`
  key on the ceremony hop / connect operation, not `se.ln == ln`, so a dial-won Receive role can
  consent.
- **T06 — route Initiate/Receive off `Record.Hop`/`Convener`** (`record.go:399`) **for ceremony hops
  ONLY** (grill C6): the no-record two-party co-sign and the manual/LAN path KEEP role-from-endpoint
  (armed=Receive, dialer=Initiate). Not qualifying this regresses the primary flow.
- **T07 — split `runSession`** into get-channel / run-exchange / lifecycle so the channel can arrive
  from a dial or an accept and the exchange role comes from the roster.
- **T08 — the `tr.Listen`-after-`Dial` spike** (grill P8): confirm quic-go permits opening a Listener
  on a transport that has already dialled (S08), the one unproven library assumption; and add a seam
  row for the minor DoS surface (stranger Initials on the shared punched port, bounded by the accept
  semaphore).
- **T09 — force-the-glare harness**: `pairrepro.sh` arms BOTH instances to listen+dial, injects two
  channels VISIBLE TO BOTH SIDES before either decides (grill P7 — else the same-survivor assertion
  flakes), and asserts both end on the same channel with the loser closed, including the
  role-opposite-dialer case.
- **T10 — seam rows**: the source-port assertion (caveat 7), same-survivor convergence, loser-closed
  (both dialled and accepted), consent-on-dial-won-channel, and the no-record-stays-asymmetric row.

#### P05.S09b — Criterion 16: the ceremony-scoped arm window and the bounded announcer *(D16 amendment, D33; criterion 16 first half)* *(done 2026-08-22, v1.117.80)*
Scope: extend the armed listener's wait from the 5-min `sessionAcceptTimeout` to ceremony scope, so
a multi-party signer who arms and waits their turn is not disarmed before the baton arrives — AND
bound the LAN announcer so the extension does not turn it into a packet cannon.
Acceptance: the arm window is the ceremony's, bounded by D33's 30-day maximum; the announcer does
not emit at full rate for the whole window (criterion 14); driven.

**Firmed 2026-08-22 (deepdive).** Criterion 16's first half is moved here from S01 (@3040-3047). The
trap the deepdive quantified: `startAnnouncing` (`lan.go:84`) tickers at `announceEvery=500ms`
(`lan.go:32`) broadcasting the stable six-word identity for the whole arm window. Extending the
window naively to D33's 30-day max is 2 datagrams/s × 86400 × 30 = **~5.2M multicast datagrams**
per ceremony, each a never-rotating name on every segment — the D6 privacy harm `errLoopbackBind`
exists against, and a criterion-14 violation ("nothing emits at full rate for the whole deadline").
Tasks (firmed 2026-08-22):
- **T01 — extend the arm window** to ceremony scope, bounded by D33's 30-day ceremony-deadline max
  (`session.go:56` `sessionAcceptTimeout` is 5 min today). Re-express the S01 arm-timer guard
  (`armsurvival_test.go:214`) deliberately when the window moves.
- **T02 — bound the announcer**: cap it to the browse/LAN window (as S08b already stops the
  publish/punch after `browseWindow` via the `inbound` check) and/or step its cadence down over the
  arm, consistent with criterion 14 and D6. Driven — a count over a simulated window, not 30 days.
- **T03 — seam rows**: the arm-window-is-ceremony-scoped observable, the announcer-emission-bounded
  count.

**CLOSED 2026-08-22 (v1.117.80). Acceptance ledger, split on `and`:**
- *"the arm window is the ceremony's, bounded by D33's 30-day maximum"* — **MET.** `runCeremonyReceive`
  bounds the connect ctx by `ceremony.MaxCeremonyLife`. **Grill finding, recorded:** the invitation
  carries no per-ceremony deadline (only the convener-signed record does, which the arm lacks at arm
  time), so the arm applies the 30-day MAX not the specific deadline — never disarming before a valid
  baton, but coarse. An `Expires` on the invitation would scope it precisely (/pending 247).
- *"the announcer does not emit at full rate for the whole window (criterion 14)"* — **MET, red-proved**
  (`announcer-ignores-its-window`). `lanAnnounceWindow` caps ANNOUNCING (~600 datagrams) independently
  of the extended LISTEN window; the socket is released when it closes; late discovery falls to the DHT.
- *"driven"* — **MET.** `TestTheAnnouncerStopsAtItsWindow` drives the cap over a short SIMULATED window
  (not 30 days), skipping honestly when multicast is unavailable; the T09 co-sign also completes through
  the extended-window arm. Diff-grill (1 agent): clean bill, double-close / lifetime / no-runSession-
  regression all verified sound.

#### P05.S10 — Channel loss either side of the confirmation gate *(D18, D24; criterion 15)* *(done 2026-08-22, v1.117.92)*
Scope: implement D18's split at the confirmation gate (`runVerification`, `verify.go:73`, the L2
spoken check that precedes any document byte in both `Initiate` and `Receive`):
- **Channel lost BEFORE both confirmations** — re-race within the remaining connect deadline; the
  new channel derives a NEW verification string, so both sides re-read and re-confirm (a stale string
  is replaced, never reused). Today the coordinator does NOT re-race: `runCeremonyReceive` does one
  `connect` + one `serveOneSession` and disarms (the S09 loop-drop, filed there); the dial side's
  request simply fails. This is where that is deliberately reworked.
- **Channel lost AFTER both confirmations** — restart the HOP (not the ceremony; `(ceremony id,
  roster index)`, D24), re-deriving a fresh verification string, and **RE-DELIVER rather than
  re-sign**: a hop's receiver signs via `Contribute` right after consent (`session.go:Receive`), so a
  loss after the receiver signed but before the initiator read the result must NOT sign again — that
  stacks a second block from one identity, wrong as a record and as a layout (D25). **Re-delivery does
  not exist anywhere in the tree today** (grep: no reDeliver/redeliver/hopRestart) — it is new state
  keyed on the hop: "this hop already produced signature X; re-deliver X."
Acceptance: criterion 15 — *"Losing the channel before confirmation re-races and re-confirms; losing
it after confirmation restarts the hop and re-delivers rather than re-signs. Both driven"* — driven by
RECONNECTING mid-ceremony (not asserted), plus D18's channel-binding clause (a confirmation computed
on one channel is rejected on any other, already held by `ExportKeyingMaterial`, `verify.go`).

Refs: D18 (@1085), D24 (hop as the restart unit), criterion 15 (@2908), `verify.go` (the gate),
`session.go` Initiate/Receive (the sign point), the S09 connect coordinator (the re-race), and the
S09 loop-drop note. **DEEPDIVE TRIGGER: fires** — it modifies the existing exchange/session code AND
adds a re-delivery seam (new stateful flow, a payload the peer must recognise as a re-delivery). Run
`/deepdive` before the grill. Tasks to be firmed by the deepdive + grill.

**DEEPDIVE 2026-08-22 (two agents, verified). It surfaced a scope defect in D18/criterion 15 and a
DECISION now PARKED for Dan.** Findings, all cited to code:

- **`Contribute` is NON-DETERMINISTIC** — random ECDSA nonce (`sign/identity.go:329`,
  `ecdsa.SignASN1(rand.Reader,…)`) AND a wall-clock timestamp (`p2p/session.go:436`, `When: time.Now()`).
  Re-signing the same input yields DIFFERENT bytes and stacks a second block (D25 wrong). So re-delivery
  MUST CACHE the co-signed `final`; it cannot re-sign-identically. The `final` is a local var dropped at
  `session.go:691` today; nothing caches it, on disk or in the `session`/`ceremonyID` structs.
- **The sign point + loss window:** receiver signs in `coSignExchange` (`p2p/session.go:189` → `:438`
  `Contribute`), writes back at `:210`; the initiator reads at `:134`. A loss in that window = receiver
  signed, initiator never got it — D18's "after confirmation" case.
- **THE SCOPE DEFECT (the crux): "before BOTH confirmations" is NOT locally decidable.** Confirmation is
  ONE-SIDED — each side knows only its own bit (`verify.go:246`); the peer's confirmation NEVER crosses
  the wire. A receiver can confirm, co-sign, write back and disarm (`session.go:978`, considering it done)
  while the lost write-back leaves the dialer (`session.go:134` errors) unable to tell a pre- from a
  post-confirmation loss. If it re-races, the receiver is gone; if the receiver also re-raced it could be
  asked to co-sign the SAME document twice. **D18/criterion 15 as written assumes a knowledge no side has.**
- **The channel-binding half already holds:** the verification string hashes the RFC-5705 exporter
  (`verify.go:162`+`:194`), so a fresh channel necessarily derives fresh words — D18's re-confirm-on-new-
  channel and reject-on-other-channel are already satisfied by the crypto. S10's work is the loop + the
  re-entry rule + re-delivery, not the string.
- **Re-race is mechanically feasible:** `connect` is re-invokable with the same persistent `hl`
  (`ceremonynet.go:598`); the arm would loop `connect`+`serveOneSession` over its `MaxCeremonyLife` cctx,
  the dial over its `connectDeadline` cctx — an ASYMMETRIC bound (30 days vs 300s) that is itself a seam.

**RECOMMENDED RESOLUTION (idempotency) — mine, but it REWRITES D18 and reinterprets D22, so PARKED:**
The undecidable "both confirmations" boundary is the two-generals problem; ACKs cannot solve it. The
correct pattern is IDEMPOTENCY: the losing side re-races UNCONDITIONALLY, and the receiver, on a
reconnect, either RE-DELIVERS its cached signature (cache hit, keyed on `(ceremony hop, hash(inbound))`,
no re-sign, no consent re-prompt) or exchanges FRESH (cache miss, re-confirm on the fresh channel). This
makes the pre/post-confirmation distinction moot — the receiver's cache, not a wire signal, decides. It
requires the receiver to STAY REACHABLE after co-signing (serving re-delivery reconnects), which
reinterprets D22's one-session-per-arm TRIPWIRE: a re-delivery is idempotent COMPLETION of the same
co-signature, not a second one. Cache bounded to the ceremony life, cleared with `cer.close()`.

**RESOLVED 2026-08-22 — Dan chose A, the idempotent cache.** D18's re-race clause is refined and D22's
TRIPWIRE reinterpreted (amendment notes on both decisions). The mechanism, and the tasks:

Tasks (firmed 2026-08-22, deepdive + Dan's choice A):
- **T01 — the re-delivery cache.** After `coSignExchange` returns `final` (`p2p/session.go:189`) and
  BEFORE the write-back (`:210`), cache `final` keyed on `(ceremony hop, sha256(inbound))` — the inbound
  CONTENT hash, not the hop alone, so a reconnect with a DIFFERENT document at the same hop never gets the
  old signature (the stale-signature risk the deepdive named). Lives on `ceremonyID` (per-hop, already the
  natural key: `ceremonyid.go:28`), bounded to the ceremony life and cleared by `cer.close()`
  (`session.go:215`) — no signed bytes outlive their ceremony. The manual/LAN path (no `ceremonyID`) is
  out of scope: it has no hop to key on and no long arm to reconnect into.
- **T02 — the receiver loops, re-delivering or exchanging fresh.** `runCeremonyReceive` loops
  `connect`+`serveOneSession` over its `MaxCeremonyLife` cctx instead of one-shot-then-disarm (stop
  discarding the served bool `session.go:1014`; move `disarmCeremony` off the unconditional defer). On each
  accepted connection, before running `coSignExchange`: look up the cache by `sha256(inbound)`+hop — a HIT
  RE-DELIVERS the cached `final` and skips BOTH `Confirm` (no consent re-prompt) and `Contribute` (no
  re-sign); a MISS runs the fresh exchange, then caches. **This is D22's TRIPWIRE reinterpreted:** a
  re-delivery is idempotent completion of the ONE co-signature, not a second — the guard that must still
  hold is "this hop signs at most once", which the cache enforces directly.
- **T03 — the initiator re-races.** `handleSessionInitiate` loops `connect`+`Initiate` over its
  `connectDeadline` cctx, re-sending the UNCHANGED `mySignedPDF` (`Initiate` never re-signs its own
  contribution), so a reconnect hits the receiver's cache. The asymmetric bound is deliberate: the dialer,
  a waiting user, gives up at `connectDeadline`; the receiver stays reachable for the ceremony.
- **T04 — the re-entry predicate: re-race a LOST CHANNEL, never a decided outcome.** Loop only on a
  transport/wire error (`verificationExchange`'s frame errors `verify.go:93-124`, a dropped read/write);
  a definitive protocol outcome — `ErrCoSignDeclined`, `ErrVerificationDeclined`, `ErrConsentTimedOut` —
  ends the ceremony, it does not re-race. Re-racing a decline would re-ask a person who already answered.
- **T05 — driven by reconnecting mid-ceremony, both sides of the gate, plus seams.** Before the gate: a
  channel dropped inside `verificationExchange` re-races and re-derives FRESH words (EKM, `verify.go:162`).
  After the gate: a channel dropped after the receiver signed re-delivers the SAME signature — assert the
  result carries exactly ONE block from the receiver, never two, and that consent was NOT re-prompted.
  Red-proof the idempotency (a cache that re-signed would double the block).

**GRILL 2026-08-22 (1 agent, verified against code). Six holes; dispositions fold into the tasks:**
- **P0 (load-bearing) — the loop has no "done" signal, and staying reachable collides with the TRIPWIRE.**
  `serveOneSession` returns `true` on a LOST writeback identically to a clean success (`session.go:34`
  vs `:43`), and a successful `writeFrame` (`:210`) does not mean the initiator READ it (QUIC buffering),
  so the receiver can never know delivery happened. **Disposition:** after co-signing, stay reachable for a
  BOUNDED re-delivery window ~= `connectDeadline` (the initiator's own re-race bound), serving idempotent
  re-deliveries, then disarm — NOT the 30-day pre-signing baton window (S09b). The TRIPWIRE (`session.go:32`,
  "torn down after one session ... do not widen how long it stays open without a fresh security review") is
  reinterpreted to "SIGNS at most once" (cache-enforced), and the deepdive+grill+this disposition ARE that
  review; the live assertion `session_test.go:221` ("one arm has served more than one session") is SCOPED
  for the ceremony path (it still holds for the manual/LAN arm).
- **P1 — the re-entry classifier must be a WHITELIST.** `errCommitmentBroken` (the MITM signal, `verify.go:56`)
  sits OUTSIDE the frame-error range, so "re-race unless the 3 decline sentinels" would RETRY UNDER MITM.
  **Disposition:** re-race ONLY on positive transport errors (`net.Error`, `io.EOF`, `io.ErrUnexpectedEOF`,
  deadline); default TERMINATE. Residue: a bare `io.EOF` after decided-then-crashed is indistinguishable
  from a drop (two-generals), tolerable.
- **P1 — cross-package seam.** The cache lives on `ceremonyID` (server) but `inbound` exists only in
  `p2p.Receive` (`session.go:185`). **Disposition:** a `ReDeliverer` interface (`Cached`/`Store`) threaded
  into `coSignExchange`, consulted AFTER peer-binding validation (`session.go:395-412`, no cross-peer theft)
  and BEFORE `Confirm`, with the lookup after `runVerification`+the inbound read so the fresh spoken check
  still runs (D18).
- **P2 — the true invariant is "at most once per distinct inbound", not "per hop".** A pinned peer varying
  the document each misses the cache and re-prompts consent. **Disposition:** state the true invariant;
  bounded by consent+pin (not a new vector), do NOT add a refuse-second-document rule.
- **P2 — cache concurrency.** Read/write on the receive goroutine races `cer.close()`'s clear. **Disposition:**
  guard with `ceremonyID.mu` + the `closed`-flag drop pattern (`ceremonyid.go:56,64`).
- **P2 — looping `connect` re-runs publish/punch/portmap.** **Disposition:** the post-signing window reuses
  ONE armed `hl.Accept` rather than re-invoking the full `connect` feed per reconnect.
- **Scope (state, do not build): S10's cache is IN-MEMORY, for channel loss.** D24's "a signature is
  PERSISTED before it is delivered" (@1553) and P08's process-kill criterion (@3907) are the PERSISTENCE,
  which is P08's — S10 states it owes P08 nothing there.
- **Not unattended (state):** re-delivery re-runs the fresh spoken check, so a reconnect needs the human
  present; a peer reconnecting hours later finds the words gate unmanned and re-delivery times out — correct
  under D18, and why the post-signing window is bounded.

Grill verdict: the design is sound; the six are joint defects, all dispositioned. **Ready to build.**

**CLOSED 2026-08-22 (v1.117.92). Acceptance ledger — criterion 15, split on `and`:**
- *"Losing the channel before confirmation re-races and re-confirms"* — **MET, driven E2E.**
  `TestCeremonyReRacesAfterEarlyChannelLoss`: Alice drops before the spoken check, Bob re-races and
  completes on a fresh channel — whose EKM exporter gives fresh words (`verify.go:162`), the re-confirm.
- *"losing it after confirmation restarts the hop and re-delivers rather than re-signs"* — **MET,
  driven E2E + red-proved.** `TestCeremonyReDeliversAfterReconnect` (reconnect re-delivers the cached
  signature, consent not re-asked, receiver opens it once), `TestReDeliveryIsIdempotent` + red-proof
  `re-delivery-re-signs` (a re-sign would differ, Contribute being non-deterministic).
- *"Both driven"* — **MET** (both E2E, plus the unit/red-proof crux and the classifier whitelist test).
- D18's channel-binding half (a confirmation on one channel rejected on any other) is held by the EKM
  exporter, structural and pre-existing (S10-5).

**Diff-grill (1 agent) fixed one defect** — the receiver opened a duplicate document per re-delivery —
and verified seven points sound. **Scope stated, not built:** the cache is in-memory (channel loss);
D24's persist-before-deliver / P08's process-kill is P08's; re-delivery re-runs the human spoken check
(D18-correct — D4's machine verification for the invited path is a P01/D4 improvement, not S10's).


#### P05.S11 — D19's causes 1-4, and the status surface P06 renders *(D19, D34; criteria 6, 7, 8, 9)* *(done 2026-08-22, v1.117.100)*
Scope: turn a FAILED connect into D19's plain-language diagnosis. Cause 5 (clock skew) already
ships (D35, `lan.go:497`), and the mapping-class probe already ships (P04: `rendezvous.ProbeSelf`
→ `Class{Mapping: EndpointIndependent|Dependent}` and `SharedAddressSpace` for 100.64/10). What is
missing is the CLASSIFICATION of causes 1-4 and their messages: today `connectFailure`
(`session.go`) returns clock-skew-or-generic, and `ceremonynet.go:335` already says "S11 renders it".

The four causes (D19@1140) and their SIGNALS, all gathered from the connect attempt:
- **Cause 1 — the peer never published.** The DHT answered but the peer's key held no candidates.
  → "The other side hasn't started their ceremony yet."
- **Cause 2 — the rendezvous is unreachable.** No DHT responses at all (bootstrap/query failed).
  → names the LAN and manual/VPN paths (the two that survive, D14).
- **Cause 3 — the peer published, the mapping classes explain it.** ONE-SIDED (amended 2026-08-19,
  no peer class on the wire — D17): THIS side is `EndpointDependent` with NO port-map obtained.
  → names port mapping and a shared VPN — **but port-mapping advice is CONDITIONAL** (D9 pin): only
  when the mapped address is NOT in 100.64/10 AND the port-map tier got an answer; otherwise name
  the VPN path and stop (a CGNAT user cannot port-forward). **Degrades to cause 4 when the two DHT
  observations do not arrive** (criterion @2907 — expected, not a defect).
- **Cause 4 — the peer published, the classes do not explain it.** Filtering, a firewall, an
  asymmetric failure. → an honest "couldn't establish a connection" with the per-tier detail.

Acceptance (criteria @2907, @2922): each of the four causes produces its OWN message; the
mapping-class test distinguishes the two NAT classes from two DHT observations; cause 3's message
names port mapping + VPN, never a port-forward a carrier's NAT forbids; cause 3 degrades to cause 4
when observations are absent. **L1 pin (D19@735): the classification is DIAGNOSTIC ONLY** — it
changes messages and tier preference, never the pin check; the L1 guard covers it.

Refs: D19 (@1140, the five causes), D9 (the CGNAT port-forward ban), D17 (no peer class on the wire),
D34 (outbound-call enumeration / DHT disclosure — the disclosure half is P06's, per @2884), the
existing `ProbeSelf`/`Class`/`SharedAddressSpace`, `connectFailure`, `ClockSkewError` (cause 5).
**DEEPDIVE TRIGGER: fires** — it modifies the existing connect/failure/status path and reads the
mapping-class seam P04 built; the signals (DHT-reachable, peer-published, port-map-obtained) are
scattered across the feed/race and must be gathered without a new wire field. Run `/deepdive` before
the grill. Tasks firmed by that pass; likely T01 gather the signals, T02 classify + message, T03 the
status surface P06 reads, T04 the L1-diagnostic-only guard, T05 driven tests (each cause + the
degrade).

**DEEPDIVE 2026-08-22 (S11, two agents, verified to file:line). Signals gatherable with NO wire field;
`*ceremonyID` is the aggregation point (outlives the re-race loop, owns rz/gate/end/portMap).**
- **Signal 1 — DHT reachable** (cause 2): `cer.rz.Stats()` (`Responses`/`Bootstrapped`/`FetchNodes`/
  `FetchEmpty`). Collected, ZERO server readers. Cumulative → the union across re-race iterations.
- **Signal 2 — peer published** (cause 1 vs 3/4): `cer.gate.Stats()` — `Accepted==0` = never published;
  `Accepted>0 + dialErr` = published-but-unreachable. Collected, unread. Read AFTER the feed goroutines
  stop (the gate is not concurrent-safe).
- **Signal 3 — mapping class + 100.64** (cause 3): PRIMARY defect — `ProbeSelf`'s `SelfAddress{Class.Mapping,
  SharedAddressSpace}` is computed then DISCARDED in `publishCandidates` (`ceremonynet.go:107`). Fix: store
  it on ceremonyID under mu (mirror `setPortMap`). Caveat-7 satisfied (probe on `cer.end`); present on a
  genuine remote failure (the LAN did not answer, so publish→ProbeSelf ran).
- **Signal 4 — port-map obtained** (cause 3 advice, D9): `cer.portMap != nil` = obtained AND publishable.
  Gap: an obtained-but-UNROUTABLE answer is closed and dropped (`ceremonynet.go:206`), indistinguishable
  from no-answer. Fix: a tri-state {no-answer, answered-unroutable, answered-published} set in
  `appendMappedCandidate`'s three branches.
- **Surface:** a CLASSIFIER, not typed-errors-per-cause (causes 1-4 are the ABSENCE of a connection across
  the race; cause 5's `ClockSkewError` is lifted FIRST via errors.As). Two readers: the dial-side HTTP
  error body (flat `{"error": str}` at `server.go:1259` → a `{cause, summary, detail}` body) AND a new
  `sessionStatus` field for the arm-side cause-1. "Plain first, detail behind a disclosure" needs a NEW
  two-field message struct.
- **Fix first:** the co-sign initiate path (`session.go:1345`) does NOT call `connectFailure` (only /send
  does). Out of scope (nil-guard): non-ceremony `dialAny`, TCP ceremonies (`cer.rz==nil`).

Tasks (firmed 2026-08-22, deepdive):
- **T01** — retain the two discarded signals: store `SelfAddress` on ceremonyID in `publishCandidates`; add
  the port-map tri-state in `appendMappedCandidate`.
- **T02** — the classifier `cer.diagnosis() D19Inputs` (nil-guarding TCP/non-ceremony) reading rz.Stats()/
  gate.Stats()/cer.self/the tri-state; classify causes 1-4, produce each `{cause, summary, detail}`; cause 3
  ONE-SIDED (D17), CONDITIONAL advice (D9: only not-100.64 AND port-map-answered), DEGRADES to cause 4 when
  the mapping observations are absent.
- **T03** — the surfaces: a `{cause, summary, detail}` message struct; a structured HTTP error body; route
  the co-sign initiate failure through the classifier; a new `sessionStatus` field for the arm side.
- **T04** — the L1 pin: DIAGNOSTIC ONLY, never the pin check (asserted in a test).
- **T05** — driven tests: each cause by driving the signals; cause 3 degrades to cause 4; the CGNAT case
  names VPN-only.

**GRILL 2026-08-22 (S11, verified). Five confirmed holes; corrections fold into T01-T05. `*(in progress 2026-08-22)*`**
- **CONFIRMED (biggest) — data race on the gate.** `connect`'s failure path cancels but NEVER joins the
  feed goroutines (`ceremonynet.go:679-691`); an in-flight `Fetch` can run `gate.Accept` AFTER `connect`
  returns, while `diagnosis()` reads `gate.Stats()` (non-atomic uint64) — a torn read `-race` flags. The
  deepdive's "read after the feed stops" is asserted, never enforced. **T01 adds a real join** (a WaitGroup
  through `feedCeremonyRace`/`connect`, or the feed snapshots its Stats at exit).
- **CONFIRMED — cause 1 vs cause 2 collide on `Accepted==0`**, and `FetchNodes` is the wrong discriminator
  (it is `.Store()`, last-fetch-only, reads 0 after the cancel aborts the last traversal). **T02 orders the
  predicates (cause 2 FIRST) and keys DHT-reachable on a CUMULATIVE field** — `FetchEmpty>0` is documented
  (`dht.go:164`) as exactly "DHT reachable, peer has not published" = cause 1; `Responses==0`/no cumulative
  response = cause 2.
- **CONFIRMED — the port-map advice is inverted for double-NAT.** Cause 3 requires `portMap==nil` (the
  unroutable mapper is closed+dropped, so `setPortMap` never runs), so within cause 3 "answered" means
  answered-UNROUTABLE = double-NAT, where port-forward advice is the futile thing D9 forbids. **T02's
  state→advice map:** `no-answer && !Shared` → try UPnP/port-forward; `answered-unroutable OR Shared` →
  VPN-only. The tri-state is NOT redundant with SharedAddressSpace (double-NAT's reflexive DHT addr can be
  non-100.64). T02 also states the V4/V6 combination for the single cause-3 decision.
- **CONFIRMED — store-placement.** Cause 3 is the `len(addrs)==0` case, and `publishCandidates` returns
  EARLY there (`ceremonynet.go:119`) before the publish — **T01 stores `SelfAddress` immediately after
  `ProbeSelf` (`:107`), before that early return**, or the signal cause 3 needs is never stored.
- **CONFIRMED — co-sign bypass, worse than stated.** `handleSessionInitiate` funnels to a bare 502
  (`session.go:1345`) and does NOT lift `ClockSkewError` (only `/send` does). **T03 puts the classifier in
  the `else` AFTER the decline(409)/timeout(409)/errUnknownTransport(400) lifts, and adds the
  `ClockSkewError` errors.As on initiate** (cause 5, new behaviour on this path).
- **CONFIRMED — the arm side has no failure-return** (`runCeremonyReceive` blocks to MaxCeremonyLife), so a
  cause-1 `sessionStatus` computed only at the terminal return is 30 days late. **T03 computes the arm-side
  diagnosis LAZILY when status is polled** (`handleSessionStatus` → `cer.diagnosis()` while not-yet-connected),
  not a ticker.
- FINE (verified): the cause-3 degrade (`MappingUnknown` is the zero value, distinct from Independent — no
  misfire, no nil-deref); the L1 pin (diagnosis feeds nothing back into the pin check).
- **T05 adds** the `-race` case and the double-NAT (answered-unroutable) case, not only CGNAT.

Grill verdict: the diagnosis design is sound; these are joint defects at the code seams, all dispositioned.

**CLOSED 2026-08-22 (v1.117.100). Acceptance ledger — criteria @2907/@2922, split on `and`:**
- *"each of D19's four causes produces its own message"* — **MET.** `classifyD19` + `TestD19ClassifierTable`
  drive all four with the grill-corrected ordering (cause 2 before cause 1, keyed on the atomic peerSeen).
- *"the mapping-class test distinguishes the two NAT classes from two DHT observations"* — **MET** (the
  P04 `ProbeSelf`/`Class` probe S11 now RETAINS on the ceremony rather than discarding — the deepdive's
  primary defect).
- *"cause 3's message names port mapping and a shared VPN, never a port-forward a carrier's NAT forbids"*
  — **MET, red-proved** (`cgnat-told-to-port-forward`): the advice is conditional on CGNAT/double-NAT (D9),
  and the double-NAT `mapUnroutable` signal — which `SharedAddressSpace` alone misses — is retained.
- *"cause 3 degrades to cause 4 when the observations don't arrive"* — **MET** (the degrade row; `MappingUnknown`
  is the zero value).
- **L1 pin (diagnostic only)** — **MET** (`TestD19DiagnosisIsIdentityFree`; `classifyD19` takes no identity).
- Surfaced two ways: the dial-side structured HTTP error body (routing the co-sign path through the classifier,
  which it bypassed before, and lifting cause 5) and the arm-side `sessionStatus` (gated on `bootstrapDone`).

**Diff-grill (1 agent): seven attack points clean, two LOW fixed** (the join-comment misattribution, the
warmup false cause-2). **Deferred to P06** (D34): the DHT disclosure surface — `ParseInvitation` still has no
non-test caller, so a disclosure clause has nowhere to land until the ceremony panel is built (@2884). Full
suite green under `-race`.


Scope: `connectFailure` yields three sentences — two clock-skew directions and one generic (`session.go:1023-1029`). Causes 1-4 do not exist; P04.S02 built the classification they read.
Acceptance: criteria 6 and 7 verbatim; criteria 8 and 9 are **already met** by `p2p.ClockSkewError` and are ledgered, not rebuilt. The mapped port and **"no mapping obtained"** become distinguishable states on `/api/session/status`, which is what P06's two disclosure criteria render.

#### P05.S12 — The ladder becomes the default path *(D9)* *(done 2026-08-22, v1.117.103)*
Scope: `web/app.js` refuses to POST without a typed address (`if (!address) { toast('Enter the peer's address'); return; }`), so the shipped LAN tier is unreachable from the product and the manual path is not merely undemoted — it is the only path. The address field moves behind the existing `details.advanced` pattern. P06 restructures the panel; this slice makes the ladder reachable.

**GRILL 2026-08-22 (light — a UI toggle, low blast radius; no deepdive: a localized web change, no seam).**
The server already handles an empty address: `peerAddresses("")` does the LAN browse, and a ceremony
(invitation) uses the DHT — so POSTing with no address reaches the ladder, and a failure now carries
S11's D19 diagnosis. The one thing to preserve: the typed-address path stays reachable (behind the
disclosure) so the manual tier (D8 tier 5) is undemoted, not deleted.
Tasks (firmed 2026-08-22):
- **T01 — remove the empty-address refusal** in `sessionInit` (`web/app.js`): an empty address POSTs and
  the server's ladder (LAN browse / DHT) finds the peer.
- **T02 — move the address field behind `<details class="advanced">`** in `web/index.html`, and reword the
  hint so it says the peer must be ARMED (the address is the manual fallback, not a requirement).
- **T03 — verify** (tiers 2/3): the co-sign initiates with NO typed address, and the typed-address path
  still works from the disclosure.


### P06 — The Signing Ceremony surface **(built LAST, after P07 — Stage 2 grill, 2026-08-18)**
Goal: the Collaborate tab becomes the Signing Ceremony, restructured around ~~name-in, connect, confirm, sign~~ **convene, invite, connect, review, sign, deliver — as a sidebar panel rather than a tab of modals (amended 2026-08-18, D13 pin, D24)**.
**Built roster-shaped from the start (2026-08-18):** a roster of two is a roster, so P06 renders the record's roster and position even while only two-party ceremonies exist. This is deliberate — building a two-party screen here and replacing it in P07 would be the rebuild the phase order exists to avoid.
Exit criteria:
- The primary flow contains no address field and no hex fingerprint.
- Every failure tier has a distinct, actionable message **— the four of D19, plain language first with the technical detail behind a disclosure (amended 2026-08-16)**.
- **The connection screen shows per-tier progress for the whole connect deadline, never a blank spinner. (added 2026-08-16, D16)**
- ~~**Picking the same document-flow role on both sides stops the ceremony with a named message telling them one must change and both retry — detected on the channel before the verification string, never a hang and never auto-resolved.**~~ **Struck 2026-08-18 (D17 amendment, D23): roles are read from the roster, so the conflict cannot occur. What replaces it: no screen in the ceremony offers a role choice, and the panel's enabled action is computed from the record by the same function the server's L3 check uses — driven by a fixture whose UI position and record position disagree, which must show the record's.**
- **While a ceremony is armed, the screen discloses that a temporary router opening was requested and names the port; when no mapping was obtained it says so rather than staying silent. (added 2026-08-16, D15)**
- The advanced fallback is reachable but never on the default path.
- Documentation and README updated in the same phase (STANDARDS docs-parity).
- ~~**The outside cryptographic review of D12 has been completed against the shipped design and its findings dispositioned.**~~ **Struck 2026-08-18 — D12 withdrawn on Dan's instruction. Neither phase now carries an external gate; see the withdrawal for what that removes.**
- **The ceremony survives Nib being quit and reopened: the panel renders roster, position and next action from the local record with no network reachable. (added 2026-08-18, D24.)** Driven with the network down, because a resumption screen that silently needs the DHT is the failure this bullet exists to catch.
- **The screen states that the invitation is a channel secret and not a signing credential, in those terms. (added 2026-08-18, D21.)** A user who forwards an invitation should know what they did and did not give away.
- **Only the ceremony deadline is ever shown in human units; neither the connect deadline nor the exchange deadline appears as a countdown. (added 2026-08-18, D16 amendment.)**
- **While armed, the screen discloses that Nib is using the BitTorrent DHT to find the other party — beside the router-opening line, in one plain sentence. (added 2026-08-18, D34, STANDARDS §9.)** D15 already discloses the smaller thing; this is the larger one.
- **A corrupt or unreadable ceremony record degrades that ceremony's panel entry and leaves every other ceremony and open document working — driven with a truncated record. (added 2026-08-18, D34, STANDARDS §9 self-healing.)**
- **The consent screen shows every party who has already signed, not one. (added 2026-08-18, D27.)** Driven with a three-signature document, because a two-party fixture cannot tell a roster from a single peer.
- **The panel renders roster, position and next action with the vault locked, and asks for the password at the moment of signing rather than at the moment of looking. (added 2026-08-18, D29.)**
- **Each of D28's four end states — completed, declined, expired, abandoned — produces its own message, distinct from each other and from D19's four network causes. (added 2026-08-18, D28.)** Eight distinct outcomes, driven separately; a screen that folds "they declined" into "couldn't establish a connection" fails this.
- **A document under a live ceremony refuses mutating operations and names the ceremony; the refusal is driven through a real edit, not asserted on a flag. (added 2026-08-18, D29.)**
- **The ceremony record is labelled in the attachments panel for what it is, and cannot be removed while the ceremony is live. (added 2026-08-18, D29.)**
- **A party's in-progress copy is labelled as in-progress and never as the finished document. (added 2026-08-18, D28.)**

Slices *(sketch)*: the ceremony panel replacing the tab and its three modals; **the convene-and-invite PANEL, over P07.S02's route** *(re-pointed 2026-08-23 — the server-side convene is P07's; a pin two hundred lines below is not what a builder reading this list will see)*; the connect-and-confirm screen; the roster and position display; **the roster-shaped consent screen (D27); the end-state surfaces (D28); the document freeze and the attachments label (D29);** failure surfaces; docs and README.

### P07 — More than two parties: **the model** *(CLOSED 2026-08-28, v1.117.236)* **(added 2026-08-18 — D22, D23, D25, D27; amended the same day — D28, D29; built BEFORE P06 — Stage 2 grill; **split 2026-08-18, plan-review: lifecycle and delivery moved to P08**)**
Goal: a ceremony of N parties completes as a convener-driven serial ~~relay~~ **baton** (renamed
2026-08-23 — "relay" is what the out-of-scope list forbids for carrier-grade NAT, and D22's own word
is baton), ~~survives being interrupted at any hop,~~ **persists every hop before it returns
(C22) — surviving interruption is P08's, and the struck clause described machinery no slice in this
phase builds** *(struck 2026-08-23, plan-review)*, and cannot be signed out of order. Last, because it needs the record
(P01), a working transport (P02), a working ladder (P05) and a roster-shaped surface (P06); the
artifact model it rides on already carries no two-party assumption (D2 pin).

Exit criteria:
- **C01.** **A four-party ceremony completes, every signature valid, every attestation sharing one `[NibRoster:…]` commitment, and `crossBind` matching each signer against the others. (D2 pin, D22.)**
- **C02.** **A non-signing convener completes a ceremony: the finished document carries the signers' signatures and none of the convener's. (D22, Dan's call.)**
- **C03.** **The signature blocks of a nine-party ceremony are all on the page and none overlaps the readme body — driven by rendering and measuring, not by asserting a rect. (D25.)** The measurement is the point: the defect this closes is invisible to every assertion about placement arithmetic, because the arithmetic is what is wrong.
- **C04.** **A ceremony cannot grow a party ~~after the first signature~~ *once its invitations have been issued* (re-pointed 2026-08-23, plan-review, D23 amendment): the attempt is refused with a distinct message naming a new ceremony as the answer, AND that message states that the signatures already collected cannot be carried into it. (D25 — pages are allocated up front.)** The boundary is **convene**, not the first signature: adding a party changes `rosterHash`, so every invitation already sent fails `MatchesRecord` on roster length — the gap between convene and first signature is precisely when a convener remembers the second landlord, and it was unspecified. The carried-signatures clause is asserted separately because it is the half a builder omits, being the bad news.
- **C05.** **A contribution offered by the wrong party, or onto a document with the wrong prefix, is refused by the L3 guard in Go with a named error — driven for both cases separately, and with the UI bypassed. (D23, L3.)** A test that drives it through the panel proves the panel, not the law.
- **C06.** **Every criterion in this phase is driven by the multi-instance harness (D26), not by hand. (added 2026-08-18.)** A four-party ceremony verified once, manually, at the phase gate is the shape of check this repo has already been burned by.
- **C07.** **A non-signing convener completes a ceremony over the carry route, and the finished document contains no signature of theirs. (added 2026-08-18, D22 pin.)** Driven — and note it cannot pass on `Initiate`, which demands the caller's own signature back, so a green here is evidence the third route exists.
- **C08.** **The appended trust page describes a ceremony of N, the About dialog says the same thing, and the drift guard is green. (added 2026-08-18, D27.)** The guard (`internal/p2p/readme_test.go:61`) is what makes this one criterion rather than two that can drift.
- **C09.** **A nine-party document's visible blocks and signature details each name the ceremony rather than one neighbour. (added 2026-08-18, D27.)**
- **C10.** Documentation and README updated in the same phase (STANDARDS docs-parity).
- **C11.** **Every row of `<project-memory>/instruments/ceremony.md` carries a disposition — `keep-live` / `gated` / `deleted` — filled at this phase's close. (added 2026-08-18, verification pack V8.)** An inventory whose disposition column was never filled is a record of intentions; 224 such rows accumulated in another project before anyone noticed. A row whose reader is a standing criterion is never silenced.
- **C12.** **Each of D33's four numbers is enforced on the externally-supplied path, driven by supplying a value past the bound. (added 2026-08-18, crypto pack PLAN-4.)** A test that supplies a value *inside* the bound cannot see an unenforced parameter.
- **C13.** **A version skew produces a sentence naming the mismatch, not a parse error — driven for the record, the invitation and the ceremony protocol separately. (added 2026-08-18, D32.)**
- **C14.** ~~**A four-party ceremony with a NON-SIGNING convener completes, and every signature on the finished document reports `Matched`.**~~ **Amended 2026-08-23 (plan-review, D22's amendment): a four-party ceremony with a NON-SIGNING convener completes; every signature THAT HAS A SIGNING PREDECESSOR reports `Matched`; the first signer reports *first signer — no predecessor*; and an attestation naming a fingerprint that is in the roster but produced no valid signature still reports `Matched: false`.** The last clause is the one that keeps this a criterion rather than a formality — without it, the repair that satisfies the first two is loosening `crossBind` to consult the roster, under which a one-signature document reports `Matched`. (added 2026-08-18, plan-review, D22/D2 pin.) The "every attestation shares one `[NibRoster:…]` commitment" bullet is satisfied perfectly by a document on which no signature matches any other — this is the clause that sees the hub breaking the channel binding.
- **C15.** **Every signature block on a completed ceremony carries the record's intent verbatim. (added 2026-08-18, plan-review, D20 intent pin.)** None of the record round-trip bullets ever reads a signature block.
- **C16.** **A completed ceremony whose convener has `signs:false` renders as complete, not as missing a signer. (added 2026-08-18, plan-review, D27 pin.)** The "no signature of theirs" bullet is a fact about the document and says nothing about what the verifier reports.
- **C17.** **A party reconciles the document it was handed against the invitation it holds, BEFORE the consent gate — driven separately with (a) a one-byte-altered invitation and (b) a well-formed record for a different ceremony, each refused by name, UI bypassed. (added 2026-08-23, plan-review.)** `CheckDocument` and `MatchesRecord` exist, are tested, and have **zero production callers**; P01.S07's "a one-byte-altered invitation is refused by name" was discharged by a unit test on a function nothing on the real path calls. D21's corrected pin calls this comparison the single point of failure of the whole design. The reconciliation must also cover `intent`, `expires` and `capacity` — all are inside the commitment and `MatchesRecord` compares none of them.
- **C18.** **A document carrying the roster prefix of an INCOMPLETE ceremony renders as incomplete, naming how many obliged signers are absent — driven at five of nine. (added 2026-08-23, plan-review.)** C16 is its twin and is satisfied vacuously today, because nothing in the verdict path knows a roster: a nine-party ceremony abandoned at hop 5 currently renders *untampered, 5 signers, every attestation matched, one proceeding*, and no surface says four obliged parties never signed. C16 asks the verifier not to cry wolf; this asks it to cry at all.
- **C19.** **A four-party ceremony convened with two distinct capacities produces a document whose blocks render each party's OWN capacity, and whose signed `/Reason`s differ in capacity while carrying one identical recital. (added 2026-08-23, D20's capacity amendment.)** An empty capacity renders nothing and is the ordinary case.
- **C20.** **A four-party ceremony whose deadline admits hop 1 but not hop 3 is refused AT CONVENE, not at hop 3. (added 2026-08-23, plan-review.)** `checkCeremonyDeadline` reserves one exchange budget regardless of hops remaining, so at N=9 a ceremony passes early hops and then refuses parties 6-9 on a document already carrying five signatures. The reservation is also under-sized for even one hop: the real per-hop ceiling from the constants is ~23 minutes, reserved as six.
- **C21.** **An armed party whose ceremony deadline has passed disarms without being told by anyone. (added 2026-08-23, plan-review.)** The arm bounds itself by `MaxCeremonyLife` — thirty days — because the invitation carries no expiry, and P07 turns one arm into N−1. Measured: ~21,700 DHT operations per armed party over that window, so a stalled nine-party ceremony is ~3.2 M packets against D33's budgeted 93,000 — and D33's figure counts *hops*, not *arms*.
- **C22.** **The convener's baton and every hop's output are written to the ceremony mirror before the HTTP response returns, and a ceremony's directory is findable by id. (added 2026-08-23, plan-review.)** Write-only; resumption stays P08. `WriteMirror` has zero production callers and no session path writes to disk, so a ceremony's whole state is an unsaved in-memory tab: lose it between hops and every signature collected is gone on every party's machine. This criterion is what turns *ceremony destroyed* into *ceremony stalled*, and it is why P07's goal sentence no longer claims interruption-survival it does not build.

~~Slices *(sketch)*: the roster-prefix contribution gate and the L3 guard; `coSignExchange` re-based off `len(ats) != 1` **and off the wire peer (D22/D2 pin)**; the carry route for a non-signing convener (D22 pin); the roster-shaped `Confirmer` (D27); the readme and About rewrite behind the drift guard (D27); the placement policy and page allocation; the record's intent populating every block (D20 pin).~~ **Firmed 2026-08-23 into S01–S10 below.**

**Firmed 2026-08-23, then restructured the same day by a 14-seat `/plan-review`.** The sketch's seven
items all survive; the phase now has **eleven** slices and **twenty-two** criteria, and the build order
is S01 · S08 · S02 · S02b · S03 · S04 · S05 · S06 · S07 · S09 · S10.

**Amended 2026-08-24 at S02's grill: S02 SPLIT, so the phase has TWELVE slices and the build order is
S01 · S08 · S02 · S02a · S02b · S03 · S04 · S05 · S06 · S07 · S09 · S10.** S02 keeps everything inside
a signed preimage or a hashed structure (unpayable after the first field record); the new S02a is the
door, the route and what it persists. See the block above S02 for the three measurements that forced it.

**Pin taken 2026-08-24 rather than asked — D29's document freeze moves into this phase, and it is
reversible.** *"A document under a live ceremony refuses mutating operations and names the ceremony"*
is currently a **P06** exit criterion (line 4179 below), and **P06 is built LAST, after P07**. So P07
creates the first convened documents in the product's history and the server-side freeze that protects
them arrives a phase later. The client-side stopgap does not cover the gap either: `confirmSignatureLoss`
(`web/app.js:2301`) is `!isSigned() || confirm(...)`, `isSigned()` reads `state !== 'unsigned'`, and a
convened document **is** Unsigned — so it returns true with **no dialog at all**. Measured. D29's own
text already says *"a client confirm is not a freeze"*. The criterion is therefore **owed by P07.S02a**,
where the population it guards is created; P06 keeps the *surface* half (naming the ceremony on screen).
This moves work between two phases and changes no decision — the same shape as the phase-open pin that
moved the server-side convene out of P06, and reversible in a word.

**What the firming pass found, each read in the code.** Three things the criteria presuppose were not
there: **nothing in the product constructs a `ceremony.Record`** (every literal is inside a `_test.go`;
the only production reader is `ceremony.Extract`) → S02; **L3 has no code**, so "no contribution out of
roster order" is at present the convention D23 says it must not be → S03; and **the harness boots
exactly two nibs** while four criteria name four or nine parties → S01.

**What the review then found, and it reframed the phase.** *P07's criteria assume an integrity
substrate that is written but unwired.* `docHash`, `CheckDocument`, `MatchesRecord`, `AddCeremonyPeer`,
`PruneCeremonyPeers` and `WriteMirror` are all built, all tested, and all have **zero production
callers**. Ten critical clusters, six of them reached by three or more seats that could not see each
other. Four consequences shaped the list above:

1. **`docHash` is dead on the real path** — `ContentDigest` was widened to hash `/Annots` *after* D20
   took its "identical across three incremental signatures" measurement, and a visible signature adds a
   widget annot. Measured: `CheckDocument` passes before any signature and fails after the first
   *visible* one. **The guard that says otherwise signs invisibly** — a vacuous green over the clause
   D20 calls "what makes the hop-4 clause buildable at all". `docHash` becomes a **convene-time-only**
   identity; hop continuity is the byte prefix plus `AddedAfter == false`. D20's exclusion paragraph,
   `embed.go`'s doc comment and `attachments.go`'s "pre-FIRST-signature hop" comment are all currently
   untrue of the code and are fixed with it.
2. **Order of build changed twice for one reason: page count and readme body are both inside the
   digest.** S08 moves ahead of S02, and D25's page allocation moves *into* S02, because a slice that
   changes either after a record is convened moves `docHash` under every party.
3. **A whole half of the model had no owner** — invitation consumption, which is now S02b, and which
   also carries the two criteria P01's close parked on "P07's convene-and-decline machinery" that
   neither this phase's criteria nor its first firming had picked up.
4. **The first firming's S06 scope was wrong in all three of its numbers**, and D25 had already said so
   on 2026-08-18. See S06.

**One pin taken rather than asked, at firming:** the server-side convene is P07.S02's; P06's sketch item
is the panel over it. P06 is the *surface* and is built last, so a screen over a route that does not
exist cannot be what these criteria are driven through. Reversible — it moves work between two phases
and changes no decision.

**Three decisions moved, all Dan's call 2026-08-23** and written where they belong rather than here:
D22 (`AcceptedPeer` names the previous *signing* roster entry, `crossBind` stays document-scoped, the
first signer gets a state of its own), D20 (`Party` gains a `capacity` inside the preimage;
`FormatVersion` 3 → 4, taken now because no records exist in the field), and D23 (signing order stays
fixed, with its reason written down for the first time and the relaxation deferred against a named
trigger).

**Hand-off:** `<project-memory>/plan-reviews/2026-08-23.md` — every finding, every `file:line`, and the
21 info items not carried here.

#### P07.S01 — The harness grows a roster: N instances and their lifecycle *(D26; C06 is a precondition for every other)* *(done 2026-08-23, v1.117.143)*
Tasks (grilled 2026-08-23):
- T01 — a real flag parser: `-n N` plus `--lan`/`--v6`/`--keep` in any order and any combination.
- T02 — the instance array: N homes, N config dirs, N identities, one `start()` per instance.
- T03 — one port array derived once, probed by **bind** (TCP and UDP) and then used, so the probed population *is* the run's.
- T04 — the identity block per instance: fingerprint set of size exactly N, six-word name each, CSRF each.
- T05 — the lifecycle: `trap 'cleanup $?' EXIT`, every pid tracked and `wait`ed, `kill -0` post-conditions, work dir preserved on failure with its path printed.
- T06 — the two-party ceremony re-expressed through the generalised boot, fetching the finished document **by id**.
- T07 — the harness's own ceiling paragraph extended for N; `verify_test.go`'s tier-table guard extended to rows `4b`/`4c` (closes /pending 279).
- T08 — the expected-red assertion at N≥3, naming S03 as the slice that must delete it.

**The slice narrowed at its grill, and the reason is the build order.** It was scoped as "N
instances **and** an N-party driver". The attack established that the driver cannot be built in
this position: `coSignExchange` refuses anything but one prior signer
(`internal/p2p/session.go:407`), so **hop 2 of any baton fails until S03 removes it**; and the only
relay expressible before **S05**'s carry verb is the *chain* — party *k* re-uploads to
`/api/session/initiate`, which signs the initiator again (`buildCoSigned`, `internal/server/session.go:1295`), so
`N_signing` is forced to N, a non-signing convener is unrepresentable, and the block count is
2(N−1) rather than N. "The relay is expressed **once**" was therefore false as scoped: it would be
expressed once against the chain and rewritten by S05.

**And P02.S01's precedent does not transfer.** D26 put the harness first *there* because the
two-party co-sign already worked and the harness was built to observe it. "Harness first" and
"harness for a ceremony that does not exist" are different arguments; only the first has precedent.

So S01 keeps its coordinate and delivers **the instance lifecycle** — all of it drivable today,
none of it dependent on the model. The driver's acceptance moves to S03 and S05, verbatim, where it
can be driven.
Acceptance:
- **`-n N` boots N instances with N homes, N config dirs and N identities**, asserted as a set of N distinct fingerprints **of size exactly N** — compared to `N`, never to the length of the list that answered — with `instances-up == N` as a **separate** assertion and its own message, because "an instance is missing" and "two instances share a key" are two defects with one symptom. Every instance's pairing name is six words, not instance 1's alone (P01.S02's only tier-4 reader, today applied to A and sitting in the block this rewrites). Driven at N=4 and N=9.
- **`-n` defaults to 2 and the default run is the ceremony that passes today** — same two-party path, both transports, same cross-run assertions — so `CONTRIBUTING.md` row 4 keeps its meaning for the four slices before an N-party ceremony can complete at all. The finished document is fetched **by document id** rather than by the active-document fallback, which makes the two-party run stricter today and gives "which document did this hop produce" an answer at N.
- **Flags parse in any order, and every combination is either honoured or refused by name** *(amended at the slice's review: `--lan --v6` is now REFUSED rather than accepted — every v6 consumer is gated on `port != "lan"`, so the pair parsed and printed the ordinary LAN pass while the operator believed they had driven 4b and 4c at once. The LAN arm binds no address at all, which is P03's whole point, so there is nothing for v6 to be).** Today `LAN`, `KEEP` and `V6` each test `"${1:-}"` alone, so `--keep --v6` silently runs the ordinary loopback ceremony while the operator believes they drove tier 4c — the ADR-010 "configured past the disagreement" shape, in the harness's own argument parsing. `--lan` and `--v6` are declared **N=2-only in this slice**, in the file's own ceiling section, with the reason.
- **The pre-flight probes every port the run will use** — N API ports and the computed session-port block — **by bind attempt on both `SOCK_STREAM` and `SOCK_DGRAM`, in the family the run will actually bind**, printing the whole block on refusal. (`SO_REUSEADDR` on the TCP probe only: it is what stops a `TIME_WAIT` socket from the previous run reading as held, and UDP has no `TIME_WAIT`, so there it would only cost the discrimination.) Today's probe is `curl …/api/status`, which detects only a *nib*; and a **connect** probe cannot answer for UDP at all, because it succeeds against a free port — so the QUIC half of a connect-shaped pre-flight could only ever report pass.
- **The lifecycle is asserted, not assumed:** every instance and every watcher subshell is tracked, killed, and then **polled until `kill -0` fails** — `wait` is deliberately not the instrument for the instances, because they come out of a command substitution and are this shell's grandchildren, so `wait` returns instantly with an error, which is the degenerate "waited" that proves nothing. On failure the work dir is preserved and its path printed, driven by a failure injected at instance 5 of 9; the trap reads `$?` rather than a flag, because `fail` runs inside subshells and a flag set there never reaches the parent (and because a banner printed unconditionally instead of on the exit status is v1.117.131's lesson).
- **At N≥3 the harness asserts the product still refuses hop 2, as an expected red** — accepting *either* the named refusal *or* today's EOF flattening and printing which it saw, because the probe measured that the refusal is carried on the wire in neither direction (see S03's clause); a verbatim assertion would go red for the right reason with a message pointing at the wrong thing, with a comment naming S03 as the slice that must delete it. This is the slice's honesty and it is load-bearing: without it the N≥3 path is a skip, an `|| true`, or a run behind an env var nobody sets, and nothing makes anyone switch it on. With it, the harness goes red the day the product stops refusing.

#### P07.S08 — The readme and About describe a ceremony of N, honestly *(D27; C08)* — **moved ahead of S02, 2026-08-23** *(done 2026-08-23, v1.117.147 — clause 4 struck and moved to S02; clause 3 amended, two of its three sub-clauses measured FALSE. Review: `code-reviews/v1.117.145-p07s08-2026-08-23.md`, 1 critical + 7 warnings + 11 info, all dispositioned)*
Tasks (grilled 2026-08-23 — 13 seats; the grill **amended** the slice, struck clause 4, and corrected clause 3's wording twice):
- T01 — the line budget becomes a **refusal at one door**: `RenderReadme` returns `ErrReadmeOverflow` when the computed last baseline would fall under the block stack, and the guard asserts routing through the door (ADR-009).
- T02 — the rendered-text instrument: `api.ExtractContent`, runs joined and whitespace collapsed.
- T03 — the prose rewrite, inside the measured budget, asserting only what the document RECORDS.
- T04 — `trustClaims` gains the N-party claims; the About dialog gains its first co-signing copy; the shared modal rule gains `overflow`, and `.sigcard` a `max-height`, or the new copy is unreachable.
- T05 — four red-proof rows, and `verify_test.go`'s `recorded` 65 → 69 in the same commit.

**What the grill changed, and why the slice is not what it was.**

**Clause 4 (name the convener — label and fingerprint) is STRUCK from S08 and moved to S02**, where a
verified `Record` exists. Five seats reached this independently. It is not buildable here — `RenderReadme()`
takes no arguments, `buildCoSigned` (`internal/server/cosign.go:225`) holds no record, and **no production
code constructs a `ceremony.Record` at all**. Worse, building it would re-create the precise defect
`Party.Name` was deleted to make unrepresentable: one fact in two places with nothing comparing them —
`MatchesRecord` (`internal/ceremony/invitation.go:578`) compares `Fingerprint` and `Signs` and **skips
`Label`**, though `Label` IS inside the roster preimage (`record.go:239`). And the page is written by
whoever signs **first**, not necessarily the convener, with every later signature then vouching for it.
S02 inherits four measured render-door guards with it (WinAnsi, `%`, newline, length) — see the inventory.

**Clause 3's wording was wrong in two ways, and both over-claimed.** "Each party *verified* the convener"
is **false**: nothing signs the invitation, so the convener's fingerprint IS the root of trust
(`internal/ceremony/invitation.go:34-39`) — the honest verb is *accepted*. And "non-adjacent parties"
imports a **chain**; D22 is a **hub**, and `hopBetween` (`record.go:415`) says "Alice and Bob in a
three-party ceremony never speak" — so the true limitation is stronger: *no two parties other than the
convener verified each other*. The page therefore asserts only what the document **records**, never what
the humans **did** off-page — the one formulation true on both exchange routes (the live session runs a
fail-closed spoken check at `internal/p2p/verify.go:229`; the manual file-passing route has no channel and
runs none) and at every N.

**The overflow defect is real, silent, and destructive — and the two obvious instruments are blind.**
Measured: `RenderReadme` computes a last baseline of **−189** at 61 drawn lines, but pdfcpu **clamps** the
emitted position (a requested y of −50 and of −5000 both land at **421.0**, A4's centre), so 62 runs
collapse to **49 distinct baselines with 14 sharing one** — an illegible smear over the trust text, with
`err == nil` and `PageCount == 1`. Rendered y **saturates**; extracted text is **always present**;
`PageCount` **cannot move**, because the spec hardcodes `"pages": {"1": …}`. Only the **computed** baseline
has reach. The appearance is an opaque white fill (`web/app.js:961`), so a collision **erases** rather than
overlaps.

**The budget, measured against the rendered artifact rather than the constants:** the body is 29 lines with
a last baseline of **346** (not 343 — pdfcpu adds a 3pt `Td`), block 1's top is **220**, and
`minY = 346 − 14 × (added lines)`. So **8 added lines clear by 14pt and the 9th lands on the block**. The
deepdive costed the rewrite as additive at 12–20 lines and concluded it could not fit; paragraph 4 is the
false one and is itself 6 rendered lines, so **replacing** rather than appending measures **+4**.

**Clause 2 is larger than it reads, in two directions.** The About dialog contains **no co-signing copy at
all** — its four `trustClaims` matches are single-signature claims it satisfies incidentally — so this is
new copy, not an edit. And the copy would be **unreachable**: `body > div[id$="Modal"]` is
`align-items: center` with no `overflow` (`web/style.css:475`) and `.sigcard` has no `max-height` (`:661`),
so overflow escapes both ends with no scroll in either direction. Only `.aboutdoc` — the *licence* pane —
scrolls, while the comment above `.aboutcard` claims both are bounded. The fix goes on the shared rule,
which that comment's own reasoning demands ("a dialog that ever needs to opt out should get a class, not a
removal from a list").

**The guard this slice must strengthen is the fifth instance of a hole this repo has already recorded four
times.** `TestAboutCopyContainsTrustClaims` is `strings.Contains` over the whole of `web/index.html`: it
never locates the dialog, so it is satisfied by an HTML comment, a `<script>` string, or a leftover after
`#aboutModal` is deleted. `docs/red-proofs.md`'s vacuous-green table names instances two, three and four.
`internal/server/pinbehaviour_test.go:96` is the model — extract the named block, `Fatal` if it is gone,
and hold a population floor.

**Refuted, and recorded so it is not re-raised:** the `github.com/daniel-alexander4/nib` link in the
About block does **not** violate the global no-github rule — `origin` is that remote and
`internal/server/update.go:24` polls it for releases. It stays.

Scope: **it moves first because it changes the readme body, and the readme body is inside `ContentDigest`,
which is inside the `docHash` S02 commits to.** Rewriting it after S02 would move the digest under every
record already convened. `readme.go` asserts two-party in **three** places, not the one the first firming
named: "signed by **two people**", "In **two-party** signing the second …", and "How the **two people** know
who they signed with". `trustClaims` is the single source the drift guard binds, and none of its four entries
mentions parties, order or a ceremony — so "the About dialog says the same thing and the guard is green" is
dischargeable today by an About that never mentions co-signing at all.
Acceptance:
- The **rendered** body — text extracted from `RenderReadme`'s output, not the Go string constant — describes a ceremony of N, and **no longer contains "two people" or "In two-party signing"**. The negative clause is the load-bearing half: the positive one cannot fail.
- **The N-party claims are ADDED to `trustClaims`**, which is what makes them guarded, and the guard is shown to still bite.
- ~~The page states the **trust topology honestly**: identities were *asserted by the convener in an invitation*, each party verified the convener, and **non-adjacent parties did not verify each other**.~~ **Amended at the slice grill, 2026-08-23 — two of these three clauses were themselves wrong, and both over-claimed.** (a) *"each party verified the convener"* is **FALSE**: `internal/ceremony/invitation.go:34-39` says in terms that **nothing signs the invitation**, so a party learns the convener's fingerprint *from* it and "there is nothing to verify a signature against until it has already been trusted" — the honest verb is *accepted*, and "verified" is stronger in the one direction that costs the holder. (b) *"non-adjacent parties"* imports a **chain**; D22 is a **hub**, and `hopBetween` (`record.go:415`) states "Alice and Bob in a three-party ceremony never speak" — so the true limitation is stronger: **no two parties other than the convener checked each other**. (c) "asserted by the convener in an invitation" is right in substance but names machinery that does not exist in production yet, so the page says the convener "tells the rest who is on it". **What the page now does instead, and it is the generalisation of all three:** it asserts only what the document **records**, never what the humans **did** off the page — the one formulation true on the manual pass-the-file route (no channel, no spoken check), on the live session (`verify.go:229`'s fail-closed check), at N=2 and at N=9.
- ~~The page **names the convener** — label and fingerprint — and says what a shared roster commitment does and does not prove.~~ **STRUCK from S08 and moved to S02, 2026-08-23 — reached independently by five seats of the slice grill.** Not buildable at this coordinate: `RenderReadme()` takes no arguments, `buildCoSigned` (`internal/server/cosign.go:225`) holds no record, and **nothing in production constructs a `ceremony.Record`**, so it would ship a ninth member of the set P07's own plan-review called the phase's through-line. And building it re-creates the defect `Party.Name` was deleted to make unrepresentable — one fact in two places with nothing comparing them, since `MatchesRecord` compares `Fingerprint` and `Signs` and **skips `Label`** though `Label` is inside the roster preimage (`record.go:239`). The page is also written by whoever signs **first**, not necessarily the convener, and every later signature then vouches for it. **The roster hash itself can never go on this page**: `rosterPreimage` digests `DocHash`, which is `ContentDigest` of the prepared document, which contains this page — a hash fixed point, impossible rather than merely hard. `Record.ID` and the convener's own fingerprint are acyclic and printable, which is what S02 inherits.
- **Added at the slice grill:** the body cannot outgrow the space above the signature blocks. Measured: the page is 31 rendered lines with a last baseline of **315** against a block-stack top of **220**, and the failure is silent AND destructive — pdfcpu clamps the position it emits, so surplus lines stack on one baseline, and the appearance is an opaque white fill, so a collision **erases** the trust text. `RenderReadme` refuses (`ErrReadmeOverflow`) at the one door, against a floor derived from `stackPlacement` rather than restated as a literal.
- **Added at the slice grill:** every rendered line fits the column, measured with the **real** Base-14 metrics. `wrapText` hand-rolled a three-bucket estimate with a 3% safety factor while its worst under-measurement was **3.29%** — the margin was luck. Fixed *here* because changing how text is measured moves every line break, which moves `ContentDigest` → `DocHash` → `RosterHash`: free today, unpayable from S02.

**S02 was SPLIT IN TWO at its grill (2026-08-24) and its irreversible half re-scoped. Verdict:
overturned.** Record: `<project-memory>/grills/2026-08-24-p07s02-convene.md`. Fourteen seats; thirteen
returned (the PDF-format seat died on a rate limit before reading its brief — a declared coverage hole,
not a clean pass). The seam of the split is **inside the commitment vs outside it**, not the panel's
proposed "silent vs loud": what a signed preimage or a digest commits to is unpayable after the first
field record, and everything else is an ordinary fix.

**Three measurements refute the text S02 was to be built against, each re-verified by the driver:**

1. **`docHash` is dead from hop 2 on an HONEST ceremony, and the repair this plan adopted at line 4241
   is REFUTED.** `ContentDigest` hashes `/Annots`; a *visible* signature adds a widget annot; the
   production path signs visibly. Measured: hop 1 OK, then `CheckDocument` = *"these are not the same
   document"* — a false accusation against an honest counterparty, every time. The guard cited as
   discharging it (`record_test.go:366`) signs **invisibly** and cannot fail for the clause's reason.
   And "byte prefix + `AddedAfter == false`" was run against a document whose **page 1 is blacked out
   by the last signer**, through both `sign.SignApproval` and the production `p2p.Contribute`:
   `prefix=true docHashFromPrefix=true addedAfter=false state=valid` — **every clause passes**, two
   signers, both valid. The repair is anchored only at revision 0 and says nothing about revisions 1..N.
2. **`ContentDigest` excludes the embedded-files name tree, so an exhibit swap is invisible.** Measured:
   an attachment's contents changed under the same filename — digest unmoved, `CheckDocument` = nil.
   The exclusion is *argued* at `embed.go:43-47` ("the signatures cover everything else"), an argument
   already refuted once **inside the same function** when `/Annots` was folded in.
3. **FOUR irreversible format decisions, not two — and the count moves BOTH ways.**
   `ContentDigestVersion` is a third (three occurrences in the whole tree; bound *into* the digest and
   carried nowhere beside it, so a bump yields the tampering accusation its own comment says it
   prevents). The ceremony **wire protocol has no version at all**, so C13's third leg is unbuildable.
   But the *invitation* bump is **not** forced: `/pending 247` already grilled and deferred this work,
   and its recorded answer is **optional → no bump**. As a `*time.Time` it lands at **S02b**, where its
   own gate (G1: `MatchesRecord` gains a production caller) is discharged — with nothing locked in by
   S02 shipping.

**The one rule that kills the largest class:** *every axis the preimage NORMALISES is malleable.*
`rosterPreimage` hex-decodes fingerprints to raw bytes, so the commitment is case-**folding** — two
byte-different rosters share one `RosterHash` and one valid `ConvenerSig` — and it commits `Expires` at
RFC3339 **second** granularity while the JSON carries nanoseconds. Seven comparison sites around that
one commitment implement two different rules. **The door canonicalises the record before signing**, so
the disagreement becomes unrepresentable rather than checked — the move that deleted `Party.Name`.

#### P07.S02 — The commitment becomes honest: the digest, the versions, and the guards *(D20, D32; C19, and the precondition for C01/C17)* — **re-scoped 2026-08-24 at its grill** *(done 2026-08-24, v1.117.153)*
Tasks (grilled 2026-08-24):
- T01 — the three guards become measurements FIRST, before any field lands: `inPreimage` derived by driving `rosterPreimage`; `TestEveryPartyFieldIsComparedByMatchesRecord` as its twin; a `RosterHash` golden vector; `FormatVersion` pinned to a literal. **First, because every later task is a preimage edit and today nothing can see one.**
- T02 — the reader scan gains an out-of-package reader per shape and validates its own `published` keys (one unexported field currently drops a whole shape, PASS).
- T03 — `Party.Capacity`, chunk added unconditionally inside the per-party loop; `FormatVersion` 3 → 4. T01's driven guard is what proves it landed in the commitment.
- T04 — canonicalisation: `Record.Canonical()` lowercases roster fingerprints and truncates `Expires` to the second the preimage commits to; `Sign` canonicalises; `Verify` refuses a non-canonical record; re-encode reproduces the bytes.
- T05 — `ContentDigest` covers the embedded-files name tree **minus `nib-ceremony.json`**; `ContentDigestVersion` 2 → 3; the self-reference row (S02-2) stays green.
- T06 — the `Record` carries the digest version inside the preimage, and a skew produces a sentence naming both numbers rather than *"these are not the same document"*.
- T07 — `RosterHash` travels in the invitation; `MatchesRecord` compares it. Measured today: one invitation matches any number of records sharing a roster.
- T08 — the five measured-false doc comments move (`embed.go:14-18`, `:39-41`, `:43-47`, `attachments.go:188-189`, `:191-195`). `attachments.go:265` stays — it is the one that is true.
- T09 — red proofs for S02-1, S02-3, S02-4, S02-6, S02-7; `verify_test.go`'s `recorded` count moves with them.

Scope: everything that is **inside a signed preimage or a hashed structure**, and therefore free only
until the first field record. No route, no vault, no mirror — those are S02a. This slice exists because
S02a cannot be built correctly on a digest that refuses honest documents and a commitment that folds
case.
Acceptance:
- `ContentDigest` covers the **embedded-files name tree minus `nib-ceremony.json`** (self-reference), `ContentDigestVersion` 2 → 3 — driven by swapping an attachment's *contents* under an unchanged filename and requiring the digest to move. The current exclusion is measured false in the window the digest is checked in.
- **The `Record` carries the digest version it was written under**, inside the preimage — so a future coverage change produces a **version sentence naming both numbers** (D32), never *"these are not the same document"*. Driven by reading a record stamped with a version this build does not write.
- **What `docHash` proves is written down and the code says the same thing.** Five doc comments are measured FALSE today (`embed.go:14-18`, `:39-41`, `:43-47`, `attachments.go:188-189`, `:191-195` — the last contradicting its own function body at `:265-271`) and move in this commit. `attachments.go:265` is NOT among them: it is the one that describes the code correctly, and deleting it would delete the true statement.
- `Party.Capacity` inside the per-party loop, **chunk added unconditionally including when empty** (a conditional chunk makes the chunk count variable and injectivity then rests on a length coincidence), `FormatVersion` 3 → 4.
- **The record is canonical before it is signed** — every roster fingerprint lowercased, `Expires` truncated to the second the preimage commits to — and `Record.Verify` **refuses** a non-canonical record. Driven by requiring that re-encoding a verified record reproduces its own bytes.
- **`RosterHash` travels in the invitation and `MatchesRecord` compares it.** Measured: `MatchesRecord` compares nothing that varies between two records sharing a roster, so **one invitation authorises any number of records** — a convener can run two chains under one ceremony id with different intents, deadlines and documents, and both parties' checks pass. One field subsumes C17's whole field list and fails closed.
- **Three guards stop being claims and start being measurements**, each red-proved: `TestEveryPartyFieldIsInTheCommitment` derives `inPreimage` by *driving* `rosterPreimage` (vary one field, require the digest to move) rather than restating it — measured: `Capacity` declared in the map alone ships GREEN with `Director` and `Witness` hashing identically; `MatchesRecord` gains its reflection twin over every `Party` field; and the reader scan gains an out-of-package reader for each new field plus validation of its own `published` keys — measured: one **unexported** field silently drops a whole shape from the scan.
- A **golden vector** pins `RosterHash` for a fixed record, and a test pins `FormatVersion` to a **literal**. Neither exists today, so the preimage can change with every test green.

#### P07.S02a — Convene: the door, the route, and what it persists *(D20, D21, D22, D25, D29; C04, C20, C22)* — **new, 2026-08-24, split out of S02** *(done 2026-08-24, v1.117.155; T09's red proofs recorded against that commit at v1.117.156, 2026-08-25)*
Tasks (grilled 2026-08-24):
- T01 — `p2p.PrepareCeremonyDocument(pdf, signers)`: readme + `signaturePages(signers)` rendered-and-appended, ONE pre-signing geometry door. `PrepareDocument(pdf)` re-expressed through it at signers=2 so today's output is byte-for-byte unchanged; the divisor derives from `stackPlacement` rather than restating it.
- T02 — the ceremony page (id, recital, obliged count, convener as fingerprint + six-word name), rendered in the same pass, with a guard that it carries no party-supplied bytes.
- T03 — `ceremonyHopBudget()` in `internal/server/clocks.go` — the only package that can see all four terms — and `checkCeremonyDeadline` re-pointed at it in the same commit.
- T04 — `ceremony.Convene(pdf, req, cert, key, now)` in `internal/ceremony`, importing `p2p`: the ordering, the refusals, the local fingerprint inserted by the door, `[]Invite` in roster order.
- T05 — vault: `CeremonySecret` + accessors, fingerprint stored as RAW bytes, **plus a `Contents` version gate** (an older build silently drops unknown keys on the next `AddRecent`).
- T06 — the mirror through `WriteFileAtomicDurable`, `document.pdf` before `record.json` so the record's presence is the commit point.
- T07 — `POST /api/ceremony/convene` + the read door for invitations; `requireUnlocked`, `resolveDoc`, `commitMutation`; the `MUTATING` inventory row.
- T08 — D29's freeze: a document under a live ceremony refuses mutating operations, server-side, driven through a real edit.
- T09 — red proofs for the new rows; `recorded` moves with them. *(done 2026-08-25, v1.117.156 — five rows, **four of them restoring a defect the check written for it could not see**; `recorded` 76 → 81)*
Scope: **nothing in the product constructs a `ceremony.Record`** — every literal is inside a `_test.go`,
and the only production reader is `ceremony.Extract`. This slice is the missing constructor and its
route. **It absorbs D25's page allocation** (moved from S06, 2026-08-23): page count is inside
`ContentDigest`, so the ordering is **readme → signature pages → `docHash` → `Embed` → first
signature**. One door for the whole pre-signing pass, per ADR-009.
**Measured at the grill: `internal/ceremony` MAY import `internal/p2p`** in production code — build,
vet **and test-compile** green on a clean tree; only the reverse edge cycles, and it is invisible to
`go build`. So `Convene` lives in `internal/ceremony` and calls `p2p` directly. An injected
`PrepareDocument` was considered and **refused**: a test could pass a no-op and the ordering guard
would go green with the readme never appended.
**Pin (2026-08-23, phase-open): the server-side convene is P07's; P06's sketch item is the panel over it.**
Acceptance:
- A four-party record is convened over the API and `Extract` + `Verify` round-trip it. **The `CheckDocument` clause is asserted on the bytes the route RETURNS, re-fetched over HTTP** — not on the handler's own slice, and not framed as "a party other than the convener", which `CheckDocument`'s signature cannot express (it takes bytes and a clock) and which is a copy asserted against itself. **It is asserted at hop 1 only, and says so**: from hop 2 the honest answer is S02's, not a pass.
- `Signs:false` and a non-empty `capacity` are representable and survive the round trip.
- **D25's page allocation is driven**, at N=2, N=6, N=7 and N=9 — six blocks to a page, the divisor D25 chose (eight fit geometrically; six leaves a heading and a margin, and `ceil(32/6)=6` pages is what `invitation.go:50` already states). **At N≤2 it allocates ZERO extra pages**, preserving today's shipped two-party output and `readmeFloor`'s pin exactly. `stackPlacement`'s signature does not change — `readmeFloor` and `NominalBlockRect` both derive from it.
- **The allocation mechanism is RENDER-AND-APPEND, not insert — measured at the slice grill, because the obvious call is a silent no-op.** `pdfops.InsertBlank(pdf, pageCount+1)` returns the document **unchanged and with no error** (measured: 3 pages in, 3 pages out), so allocating trailing pages that way reports success and does nothing. And `pdfops.CreateFromJSON` **refuses a page with no content** (`Please supply page "content"`), so a truly blank page is not constructible. Both point the same way: signature pages are *rendered* like the readme — each with a heading — and appended. **The heading is load-bearing, not decoration:** D25 chose six over the eight that geometrically fit *because of* "a page heading and a margin that is not a rounding error", so without it six is arbitrary and eight is correct — and this slice freezes the divisor into `ContentDigest`. The heading carries Nib text, the ceremony id and "page N of M" and **no party-supplied bytes**, so none of S08's four measured hazards is reachable on it.
- **What allocation does NOT fix, asserted as a non-claim so the next slice does not inherit it as settled.** `stackPlacement` puts *every* block on the page it is handed, indexed by the GLOBAL signer count — measured: block 8 tops out at 892 on an 842pt page whatever the page count. So allocation fixes D25's **readme-overlap** half (blocks land on an allocated page because `NextPlacement` targets the last one) and leaves the **page-box clip** at N≥9 exactly where it is. Distributing blocks across the allocated pages is **S06's**, and the acceptance here must not read a block's page number — which is right for the wrong reason at N≤6 and wrong at N=9.
- **The convener's own fingerprint is inserted by the door**, at a caller-chosen roster position, because `identity()` mints a key on first use and a client-supplied roster cannot contain it. Driven on a fresh vault.
- Refused **by name, through the convene route** — driven separately, each asserting **the route's own sentence**, and the sentences asserted **distinct from one another** (one helper printing one message satisfies six rows otherwise): a duplicate fingerprint (naming *which* party and at *which two positions*); a roster of one; an empty intent; an intent longer than a signature block can render **verbatim** (C15) — refused, never clamped, because `cosign.go:64` silently truncates at 200 runes today and `ctx.fillText` has no `maxWidth`; a deadline that does not admit every remaining hop (C20); and **a second convene on a document that already carries a record**, refused *before* `Embed` so the user never sees pdfcpu's *"an attachment named … already exists"*.
- **C20 is driven at N≥4**, and its per-hop budget comes from a new `ceremonyHopBudget()` in `internal/server/clocks.go` — the only package that can see all four terms (`connectDeadline` and `bootstrapBudget` are unexported there; `ExchangeBudget` and `MaxRemoteDecisionWait` are in `p2p`). Measured **~29m20s**, not the plan's 23m: `exchangeDeadline` is *"the budget for one PHASE of a session — never for the whole of it"* and `checkCeremonyDeadline` reserves exactly one of them for a whole session, re-pointed here in the same commit. At N=2 the guard cannot tell per-hop from per-ceremony and is vacuous by arithmetic.
- Duplicate-fingerprint, roster-of-one and empty-intent refusals also land in `Record.Verify` — the door every **non-convener** passes — **driven by constructing the bad record directly**, never through the route, which would refuse first and leave `Verify` uncalled.
- **`Party.Label` is typed for publication, not lifted from the vault.** `pinnedLabel` returns the convener's *private nickname*, and convene would publish it to every party, inside the commitment, irreversibly. The route echoes what will be published before it commits.
- The convener's N−1 invitation secrets are persisted **in the vault**, keyed by `(ceremony id, party fingerprint)` with the fingerprint stored as **raw bytes**, re-readable after a genuine reopen from disk — and **`vault.Contents` gains a version gate in the same change**, because an older build silently drops unknown keys on the next ordinary `AddRecent` and the loss is unrecoverable. Driven by an **absence** check over `~/nib/ceremonies/<id>/` that first asserts the directory exists and is non-empty, then walks it recursively — a glob over a directory nothing created is green having read nothing.
- **A read door for the invitations**, so they are recoverable after a restart. Without it the secrets are written, tested, and reachable by nothing — the ninth member of this phase's built-but-unwired set.
- The unconvened source bytes are retained under the ceremony id, so C04's "start a new ceremony" is a **route** and not a sentence. **C04's message is true in the state it is reached in**: at convene there are no signatures, so *"the signatures already collected cannot be carried into it"* is false and must be conditioned.
- The convene route **commits into the open document** (`commitMutation`, inheriting ADR-008's byte cap and the 409), never a download — a download-only convene leaves the convener's tab holding the *unconvened source*, so re-convening it succeeds silently and every issued invitation is dead. It joins `MUTATING` in the pin inventory in the same commit; both reconciliations are subset-only and cannot catch an addition.
- The mirror is written through `WriteFileAtomicDurable`, not two bare `os.WriteFile`s, and **`document.pdf` is written before `record.json`** so the record's presence is the commit point.
- A roster above the D22 sitting ceiling (~8) returns a **distinct, named soft refusal — a warning the API carries, not a refusal** (D22's pin makes ~8 a copywriting bound and 32 the hard cap). A hard refusal at 8 would make C03, C18 and C21 — all nine-party — unreachable through the door S02a exists to build.
- **A visible ceremony page is rendered in the same pre-signing pass**, carrying **the ceremony id, the recital verbatim, how many obliged signers the ceremony has, and the convener named as fingerprint + six-word pairing name**. This lands S08's struck "name the convener" clause, which was moved here and had no bullet.
  **It carries NO party-supplied bytes, and that is a decision with a reason rather than caution.** The legal seat argued for the full roster — every party with label, capacity and short fingerprint — on the ground that a flattened or scanned bundle otherwise carries `[NibRoster:…]` tokens whose preimage exists nowhere in the exhibit. **That argument is overstated, and measuring why is what settled the scope:** `RosterHash` digests `DocHash`, and `DocHash` is `ContentDigest` of the page that would carry it — a fixed point. So **no printed page can ever make those tokens recomputable**; a flattened copy is unverifiable with the full roster on it and unverifiable without. Once verifiability is off the table the page's job is *legibility and honest incompleteness*, and that needs no free text at all: the id is hex, the recital is already rendered verbatim into every block by C15, "5 of 9 obliged signers" is a **count** and is C18's whole substance, and the convener's six-word name is **derived** from their fingerprint. Measured: the wordlist is 2048 words, ASCII `a-z`, ≤8 characters, no `%`, no backslash — render-safe by construction and bounded at 53 characters.
  **Per-party labels and capacities are S07's**, which already owns rendering capacity into signature blocks and must solve S08's four measured hazards there (WinAnsi-encodability, `%` as a pdfcpu placeholder introducer, embedded newlines, an unsplittable over-long token). Doing it in two slices would be two implementations of one escaping rule — ADR-009. This also removes the `Label`-from-the-vault hazard from this page entirely: the convener is named by a value nobody typed.

#### P07.S02b — Invitation consumption: parse, reconcile, pin, scope *(D21, D29; C17)* — **new, 2026-08-23** *(done 2026-08-25, v1.117.157 — ledger below; one clause SUPERSEDED by measurement (`CheckDocument` cannot pass for a receiving party at any hop) and one `not exercised` (the TCP-ceremony path is guarded structurally, not driven), filed as `/pending`)*
Tasks (grilled 2026-08-25 — `grills/2026-08-25-p07s02b-invitation-consumption.md`, verdict **amended**; deepdive first, `deepdives/2026-08-25-p07s02b-invitation-consumption.md`):
- T01 — one door for the ceremony pin, called by convene AND accept (ADR-009); the guard asserts routing, not each site's text.
- T02 — `POST /api/ceremony/accept`: parse, pin the convener ceremony-scoped, answer with the invitation's public facts.
- T03 — the ceremony passed **explicitly** to `serveOneSession`/`runSession`; `consentAnchor` untouched. Closes the TCP re-delivery gap in the same change.
- T04 — the C17 gate in `sessionConfirmer.Confirm`, before `setPending`: `MatchesRecord` every hop, `CheckDocument` **hop 1 only**.
- T05 — the `MatchesRecord` completeness guard, structural rather than a per-field list.
- T06 — `PruneCeremonyPeers` on decline; P01's parked criterion driven.
- T07 — P01's other parked criterion: the invitation secret is absent from `~/nib/ceremonies/` after an arm.
- T08 — red proofs; `recorded` moves. *(done 2026-08-25, v1.117.158 — seven rows, two restoring defects this slice made REACHABLE for the first time; `recorded` 81 → 88)*
**Amended 2026-08-25 at its grill — three findings the slice as written could not have produced.**
**(1) D22 is a HUB, not a chain.** `hopBetween` (`record.go:585`) refuses any pair without the
convener at one end, so a non-convener's set of possible hop partners is **{the convener}**, of
size one. "Establishes the pins its roster carries" reads as N−1; the topology makes it one, and
pinning the other thirty would pin strangers this party can never dial — the harm D29 exists to
prevent, delivered by its own fix.
**(2) The convener has the same problem from the other side and nothing was going to fix it.**
`internal/server/convene.go` never pins: its only vault write is `AddCeremonySecret`
(`convene.go:173`). So the convener manually pins N−1 parties before arming against any of them.
Folded in here as one door both routes call, per ADR-009.
**(3) A gate keyed on the consent anchor would be BLIND on the TCP ceremony path.** `consentAnchor`
is built at two sites and only `session.go:1084` carries the ceremony; `runSession` is never handed
one although the arm stored it (`session.go:989` then `:996`). The same nil already costs
re-delivery: `rd = anchor.cer` (`session.go:709`) leaves `ReDeliverer` nil on a TCP ceremony, whose
contract says nil means "the manual/LAN path, **which has no ceremony hop to key on**" — false
there, and the accept loop re-arms, so a reconnect re-signs. The ceremony is therefore passed
**explicitly** rather than filled into the anchor: `current` (`session.go:247`) prefers `cer`, so
populating it would silently re-point that path's staleness test and change what
`stale-consent-on-new-session` guards.
**And bullet 3 below is REALITY DRIFT, in two directions.** `capacity` and `label` have been
compared since v1.117.153 — `matchesRosterFields` compares the whole `Party` struct with `!=`, so
there is no work owed. `intent` and `expires` are **not carried by the invitation at all**
(`invitation.go:128`), so there is nothing to compare, and the harm the bullet names is already
unreachable: the only intent a party sees is the record's, which is signed and inside
`rosterPreimage`. **Adding the fields would CREATE the exposure** — `RosterHash` is the record's
own digest copied into an unsigned invitation, so an attacker editing `i.Intent` leaves it
matching. The bullet is replaced by the rule that generalises it, the same trade `RosterHash` made
over a per-field list.
**And `CheckDocument` cannot run at every hop.** `embed.go:74` measures it passing at hop 1 and
failing from hop 2 on an *honest* ceremony, because a visible signature moves the digest. Running
it unconditionally would refuse honest documents and take S03's N=4 driver with it.
Scope: the panel found the convene half owned and the **consume half owned by nobody**. `AddCeremonyPeer` and
`PruneCeremonyPeers` — D29's ceremony-scoped revocable pins — have **zero production callers**, and
`handleSessionArm` refuses an unpinned peer. So without this slice the phase's own four- and nine-party runs
create **permanent** pins on strangers, which is verbatim the harm D29 was written to prevent, delivered by the
harness that proves the phase. It also carries **P01's two parked criteria**, which its close recorded as
"waiting on P07's convene-and-decline machinery" and which no P07 criterion or slice had picked up.
Acceptance:
- Accepting an invitation establishes the pins it needs: **no party performs a manual fingerprint pin to take part in a ceremony they were invited to**, which is the step D21 exists to remove. **For a counterparty that is exactly ONE pin — the convener** *(amended 2026-08-25: D22 is a hub)* — and **the convene route pins the roster on the other side**, through the same door.
- On first receipt of the document the party runs `CheckDocument` **and** `MatchesRecord` against the invitation its arm was built from, before the consent gate — the C17 clause, guarded ADR-009-style by asserting the call site routes through the door, not by asserting the function can return an error. **`MatchesRecord` at every hop; `CheckDocument`'s hash comparison at NO hop** *(amended twice on
  2026-08-25 — first to "hop 1 only" from `embed.go:74`, then to this by probing the real receive
  path)*. **Measured:** the document a counterparty is handed always carries at least the sender's
  co-signature, that signature is visible on every production path, and `ContentDigest` hashes
  `/Annots` — so at the **hop-1** receiver the record extracts and verifies while `CheckDocument`
  answers *"these are not the same document"*, accusing an honest convener of tampering. The
  clause is therefore not buildable at hop 1 either, and the honest split is `ceremony.CheckRecord`
  (record present, convener signature verifies, digest rule comparable) for the arrival gate, with
  the hash comparison left where `embed.go` already says it lives: a **convene-time identity**,
  answerable only before the first visible signature. **The gate must reach the TCP ceremony path too**, which is finding (3) above.
- ~~`MatchesRecord` compares `intent`, `expires`, `capacity` and `label` as well as the fingerprints and `signs` flags. It compares none of them today…~~ **Superseded 2026-08-25 at the slice grill — see the amendment above.** In its place: **a structural guard that every `Invitation` field with a `Record` counterpart is compared**, so the field `/pending 247` adds cannot arrive uncompared. A per-field list is what left `Label` uncompared for three phases.
- After a ceremony a party **declined**, `PruneCeremonyPeers` leaves the peer list byte-identical to before (P01's parked criterion, driven at last).
- The invitation secret is never written to `~/nib/ceremonies/`, driven by searching the mirror after a ceremony is **armed** (P01's other parked criterion — the half its own close could not exercise).

#### P07.S03 — The L3 guard: the roster-prefix contribution gate *(D23, L3; C05, Stage 6 pin V1)* *(done 2026-08-25, v1.117.159 — T01 REFUTED and replaced, slice SPLIT; the substituted-record clause is met at the predicate and `not exercised` on the production path until S04 makes signatures carry a commitment, asserted as a limit rather than left green)*
Tasks (grilled 2026-08-25 — `grills/2026-08-25-p07s03-the-l3-gate.md`, verdict **amended and SPLIT**; deepdive first, `deepdives/2026-08-25-p07s03-the-l3-gate.md`):
- T01 — `p2p.NextContributor` + `p2p.AdmitContribution`: one predicate, two shapes, over `[]SignerAttestation` and a roster of primitives. *(done)*
- T02 — both contribution entry points routed through it, roster threaded from the arm's verified record. *(done — and the dialing route's `dialerCeremony` MOVED above `buildCoSigned`, because refusing after the local signature is applied leaves the user signed out of turn and a signature cannot be taken back off a document)*
- T03 — the four refusals, each asserted **distinct from the others**. *(done — five, in fact: 'not in the roster at all' is its own fact and its own sentence)*
- T04 — the two-package routing guard, with its stimulus **per directory**, because a total of two is also what reading one package and finding both there looks like. *(done, mutation-tested in both directions)*
- T05 — `len(ats) != 1` **conditioned** on the presence of a roster, not deleted. *(done — and it forced a second change: with the rule off, the two channel bindings had to read the LAST attestation rather than `ats[0]`, or they would bind the channel to whoever signed FIRST and let every later hop past. The limit is named at the line: it assumes the carrier also signs, which S05's non-signing convener breaks.)*
- T06 — red proofs; `recorded` moves. *(done 2026-08-25, v1.117.161 — six rows, one of which (`l3-admits-the-wrong-prefix`) asserts the SENTENCE because the mutation still refuses, just wrongly; `recorded` 88 → 94)*

**T01 IS REFUTED AND REPLACED (2026-08-25, measured).** The task said the import cycle must be
broken by moving `record_test.go` to `package ceremony_test`. Two things are wrong with that and
the second is fatal. It is **four files, not one** (`convene_test.go`, `embed_test.go`,
`mirror_durable_test.go`, `record_test.go` are all `package ceremony` and all import `p2p`), and
they use `ceremony`'s unexported identifiers throughout — so the move breaks them, and the task's
"its `p2p` uses touch no unexported identifier" checks the wrong direction. And **the cycle is now
a PRODUCTION cycle that no test move can touch**: P07.S02a put `Convene` in `internal/ceremony`
calling `p2p` directly (`convene.go:10`), so adding the reverse edge fails to *build*:
`imports nib/internal/ceremony from session.go / imports nib/internal/p2p from convene.go: import
cycle not allowed`. The task was written before S02a existed.

**And no cycle needs breaking.** `ReadAttestations` (`internal/p2p/attestation.go:184`) already
returns every fact L3 asks about, in signature order — `Fingerprint`, `Valid`, `Matched` (the
cross-binding), `RosterHash`, `OneProceeding`. So the predicate is a **pure function in `p2p` over
primitives**, taking a roster of `(fingerprint, signs)` pairs and the local fingerprint, and `p2p`
needs no `ceremony` type at all. Better than the interface shape also considered: no indirection,
no implementation to keep in step, both entry points calling one function. It also honours this
slice's own scope note — the caller supplies the roster, and the caller is the side holding the
record it verified at arm time.

**SPLIT — the wire and the driver become P07.S03a.** Two of the ten clauses are not gate work: the
initiator-learns-the-name clause is a **protocol version step** (`refusalAck`, `session.go:284`,
carries exactly two classes and everything else reaches the initiator as `receive co-signed
document: EOF`; a new ack byte an older build must not misread is D32's skew rule), and the N=4
tier-4 driver over both transports is a harness slice. Both move to S03a, verbatim.

**And the `len(ats) != 1` removal is CONDITIONAL, which this slice did not say.** Removed
outright, an ordinary **non-ceremony** two-party co-sign would accept a document carrying three
prior signers with nothing to refuse it — the gate exists only where there is a roster. With a
roster the gate decides; without one the single-prior-signer rule stands, as one branch with the
reason at the line.

Scope: **L3 has no code** — the law matches nothing in the Go tree outside comments, so "no contribution out of
roster order" is at present the convention D23 says it must not be. One door per ADR-009, and the slice opens
with the seam that makes one door possible at all.
- **T01 — break the import cycle first.** `internal/p2p` cannot import `internal/ceremony`: `record_test.go` is
  `package ceremony` and imports `p2p`, so the edge is a cycle — compile-tested by the panel, and already
  recorded in `record.go` as the reason `RosterToken` was deleted. S03, S04 and S07 all need record facts inside
  `p2p`, and so does D22's amendment. Move `record_test.go` to `package ceremony_test`; its `p2p` uses touch no
  unexported identifier. Decided here rather than discovered at S04, because the alternative is a partial
  re-implementation of the roster rule inside `p2p` — the ADR-009 shape this slice's own scope invokes.
Scope, continued: **the gate reads the record the party verified at arm time** (S02b's invitation-matched copy).
The document-borne record is only ever *compared* to it, never the authority — otherwise the gate reads a
substituted record and answers yes to its own question, which is reachable today because `Embed` refuses only an
*already-signed* document, `Record.Verify` asks only that the signer appear in that record's own roster, and
`ContentDigest` excludes attachments.
Acceptance:
- A contribution by the **wrong party** is refused in Go with a named error, UI bypassed.
- A contribution onto a document with the **wrong prefix** is refused in Go with a named error, UI bypassed — **driven separately**, because one fixture satisfying both is what C05's own note forbids.
- A prefix naming the right identities but carrying a signature that **does not verify, or is not cross-bound**, is refused by name. L3 and D23 both say "each one valid and cross-bound"; without this clause S03's acceptance is narrower than the law it implements.
- A **substituted but well-formed** record is refused by name — the compound above, driven.
- ~~The refusal reaches the INITIATOR by name…~~ **MOVED to P07.S03a, 2026-08-25 at the grill** — it is a protocol version step, not gate work.
- A right-party, right-prefix contribution is **admitted**, and the slice removes `coSignExchange`'s `len(ats) != 1` refusal in the same commit, so the door has a live call site rather than sitting beside a two-party legacy.
- The same predicate answers **"whose turn is it, and is it mine"** as a question, not only as a refusal — P06's replacement role-conflict criterion promises the panel computes its enabled action from "the same function the server's L3 check uses", and a check that only refuses forces P06 to retrofit a read-only query.
- **A source-level guard enumerates every contribution entry point and fails when one does not call the predicate**, with a stimulus assertion that the walk found a non-zero population. ADR-009 asserts routing, not the text each site prints; the precedent is `TestL2CoversEveryDocumentCarryingEntryPoint` in the same package.
- **L3's own negative fixture** — not shared with L1 or L2 — earns a row in `docs/red-proofs.md`, replays under `./build/redproof.sh`, and **the floor constant in `verify_test.go` moves in the same commit**.
- ~~The N-party driver completes at N=4…~~ **MOVED to P07.S03a, 2026-08-25 at the grill** — a harness slice, and it cannot run until S03a puts the refusal on the wire.



#### P07.S03a — L3 on the wire *(D23, L3, D32; C05)* — **new, 2026-08-25, split out of S03** *(done 2026-08-25, v1.117.162 — the end-to-end clause is met on BOTH transports; the tier-4 half of it moved to S03b with the driver, since a tier-4 run needs the driver that S03b builds)*
Scope: S03 makes the gate refuse **in Go**; this slice makes the party who offered the contribution
**learn why**, and drives the whole thing at N=4. Split because both are outside the gate:
`refusalAck` (`internal/p2p/session.go:284`) is the ONE door between a refusal and its wire byte
and carries exactly two classes — consent-timeout and declined. Everything else returns
`(0, false)`, writes no ack frame, and the receiver closes, so every protocol refusal arrives at
the initiator as `receive co-signed document: EOF` and is logged nowhere on the receiving side.
Adding L3's refusals is therefore a **protocol change**: a new ack byte an older build must not
mistake for one of the two it knows, which is D32's skew rule and a design step rather than a
switch statement.
Acceptance:
- **The refusal reaches the INITIATOR by name, not as a transport error** *(added 2026-08-23 — measured by S01's ceiling probe, which is what a tier-4 hop-2 attempt is for)*. `refusalAck` (`internal/p2p/session.go:266-273`) carries exactly two classes, consent-timeout and declined; every other `coSignExchange` refusal returns `(0, false)`, writes no ack frame, and the receiver closes — so `expected exactly one prior signer`, *the document was not signed by the connected peer* and *the peer's attestation does not accept you* all arrive at the initiator as `receive co-signed document: EOF`, and are logged nowhere on the receiving side either. **D23 says a refusal is "never a hang, never a silent no-op"; over the wire it is currently silent.** Refusing by name *in Go* is not the same as the party who offered the contribution learning the name, and the L3 criterion's "UI bypassed" wording tests the predicate, not the wire. Driven end-to-end at tier 4, both transports.
- ~~The N-party driver completes at N=4…~~ **MOVED to P07.S03b, 2026-08-25 at this slice's grill.** A harness slice, and downstream of this one: it asserts a NAMED refusal at tier 4, which does not exist until the wire carries one. Built together, the driver's first run is also the protocol's first run and a failure is ambiguous between them.

**Amended 2026-08-25 at its grill — a bare new ack byte VIOLATES D32, measured by tracing an older
initiator.** `Initiate` reads one frame, maps a ONE-BYTE frame through `refusalFor`, and otherwise
falls to `if !bytes.HasPrefix(final, mySignedPDF)`. So a build that predates this slice, handed a
frame `[4]`, gets `refusalFor` → `default` → `(nil, false)` → the prefix check → **"returned
document is not the one sent this session"**: a verdict about the counterparty, reading as a replay
or a tamper, produced by a version skew. That is the exact shape D32 forbids. A multi-byte frame
lands on the same sentence by a shorter path. **So the question is not which byte — it is whether
the new refusal is sent to a peer who can read it**, which makes this a negotiation.

**And the negotiation already exists.** `alpn = "nib/1"` is set on every QUIC config; quic-go
requires a non-empty `NextProtos` and a mismatch is a hard handshake failure, so offering
`["nib/2","nib/1"]` negotiates `nib/1` against an older peer in both directions and nothing
breaks. TCP (`transport.go:70`) sets no `NextProtos` at all, and adding it degrades correctly by
Go's own rules: a peer that offers none leaves `NegotiatedProtocol` empty on the other side, which
IS the signal. No new frames, no new round trip, no new failure mode. **The negotiated protocol
must NOT join `Channel.check()`'s required set** — empty is legal and means an old peer, so
requiring it would turn a compatibility signal into a compatibility break.
Tasks (grilled 2026-08-25 — `grills/2026-08-25-p07s03a-l3-on-the-wire.md`):
- T01 — `alpn2` offered alongside `alpn` on every QUIC config and added to the TCP config; `Channel` gains the negotiated protocol; `check()` does NOT require it. *(done — and a source-level guard now holds every `NextProtos` site to one list, because a new listener written with `[]string{alpn}` would silently never negotiate v2 while every behavioural test stayed green)*
- T02 — the refusal frame: a new ack byte plus a **CODE**, not a reason string, written only to a v2 peer, through the ONE door `refusalAck`/`refusalFor` already are (ADR-009). *(done — **the code is a security decision rather than economy**: the text would be written by the REFUSING side and displayed by the initiator, so free text is a string a hostile peer chooses appearing in this user's interface. A code is mapped to this build's own sentence, so the peer chooses which of a fixed set of things is said and never what it says.)*
- T03 — L3's sentinels routed onto it, and the two channel bindings with them. *(done — and it needed three NEW sentinels: `expected exactly one prior signer`, *the document was not signed by the connected peer* and *the peer's attestation does not accept you* were bare `errors.New` values, so "refuse by name" was not expressible for them at all)*
- T04 — the skew driven in BOTH directions, because it is the case no ordinary test reaches. *(done)*
- T05 — red proofs; `recorded` moves. *(done 2026-08-25, v1.117.163 — five rows, two of which restore the state the product was actually in; `recorded` 94 → 99)*

#### P07.S03b — The N-party driver at N=4 *(D23, L3; C05)* — **new, 2026-08-25, split out of S03a** *(done 2026-08-27, v1.117.209 — T01/T02/T04 at v1.117.165–.168, T03 at P07.S05b v1.117.181; the slice heading simply never got its marker when its resequenced task landed)*
**Build-order note (2026-08-25):** S03b's remaining task is behind S05, so the order is
S01 · S08 · S02 · S02a · S02b · S03 · S03a · **S03b(T01,T02)** · S04 · S05 · S05a · S05b ·
**S03b(T03)** · S05c · S05d · S06 · S07 · S09 · S10. *(S05a, S05b and S05c were split out of S05 and
of each other on 2026-08-25; S03b's T03 lands with S05b's driver, which is the run it needed.)*
Resequencing is not a decision about the product; it is where the measurement put the work.
Tasks:
- T01 — the refusal stops being reported as a connect failure, and the probe that should have seen it stops being pointed at the wrong string. *(done 2026-08-25, v1.117.165)*
- T02 — the harness drives a REAL ceremony: convene, issue invitations, accept them, arm WITH the invitation — so hop 2 is a ceremony hop at all. Its own doc names hand-pinning as the residue D29 forbids and says this harness should stop when S02b lands; S02b has landed. *(done 2026-08-25, v1.117.167 — for the N≥3 path; hop 1 still hand-pins and is the remaining half, named in the harness's own blind-spot list)*
  **And the old probe's TOPOLOGY was wrong, not only its pinning.** It drove hop 2 as party 2 → party 3; under D22 that is **not a hop at all**, because `hopBetween` refuses any pair without the convener at one end. A ceremony's second hop is convener → party 3, and the old shape was a chain the model has never had. The rewritten probe also needs **no watchers, no far-side arm and no port**: the refusal is raised at the carrier's own machine, inside `buildCoSigned`, before any network work.
- T03 — ~~the relay completes at N=4~~ **DONE at P07.S05b, 2026-08-25** (the driver, at N=4 and N=9, with this clause's own extra assertions — armed-once, still-armed-at-its-hop, the distinct-signer set). It was **BLOCKED on P07.S05's carry verb — measured 2026-08-25, and the measurement contradicts this clause's own arithmetic.** The clause assumes the relay completes once `len(ats) != 1` is conditioned, and notes that *"through `Initiate` every intermediate party signs twice, so the count is 2(N−1)"*. **Those two cannot both be true under L3**: a signature twice from the same party is a prefix that is not the roster's signing order, and refusing that is the whole of D23. Driven through the real hop sequence — hop 1 completes with exactly the roster prefix; at hop 2 `/api/session/initiate` applies the LOCAL signature before it sends (`buildCoSigned`), so the carrier signs again and **L3 refuses it by name, at the near end**; and B, handed the document unchanged, **IS admitted**. So the model already supports the relay and what does not exist is a route that hands the baton on **without contributing** — which is S05's carry verb, exactly. ~~`TestTheRelayCeilingAtFourParties` starts failing at its last assertion the day S05 lands, which is when to delete it.~~ **PIN, 2026-08-27: false, and checked rather than assumed.** S05 landed and the test is green, because its last assertion is that **B is admitted a document carrying exactly the roster prefix** — and S05 added a *route* (`p2p.Carry`) without changing the *predicate*. The assertion was true before the carry verb and is true after it, so the expiry could never fire. The comment is corrected in place rather than the test deleted: what it pins — L3 refusing a second signature from one party **by name**, and admitting a correct prefix — is D23 and does not expire. **A self-declared expiry that cannot fire is worse than none**, because the next reader reads the green as "the condition has not arrived yet".
- T04 — red proofs; `recorded` moves. *(done 2026-08-25, v1.117.168 — one row, for the layer that undid S03a's fix; `recorded` 99 → 100. T03's row follows T03.)*

**T01 found three defects and the first two were mine (2026-08-25).** Running the N=4 probe after
S03a — which its own doc says "goes red on the day S03 lands" — it reported *"the refusal arrived
FLATTENED to EOF"*. It had not. **(1)** The product wrapped the named refusal in
`connectFailure`'s "could not connect to peer" and sent it to `writeConnectDiagnosis`, which
renders a 502 and picks a **D19 network cause** — for an exchange the peer connected to and
refused. That is the wire fix undone one layer up, and it is the harm `verify.go` already states
for its own case: *"could not connect" invites a retry*. Lifted at BOTH doors, because
`connectFailure` is reached from `diagnosis.go` too. **(2)** The probe's by-name branch grepped
`expected exactly one prior signer`, which S03a renamed when it gave the refusal a sentinel.
**(3)** Its EOF branch matched `could not connect to peer` — the server's generic wrapper, present
on the named refusal too — so it could **never** have distinguished the two and would have
reported the old shape forever. A guard pointed at the wrong string, reading as a measurement.
Scope: the tier-4 harness. Downstream of S03a, which is what gives it a named refusal to assert.
Acceptance:
- **The N-party driver completes at N=4** *(moved here from S01 at its grill, 2026-08-23 — this is the first slice where a document carrying more than one prior signature is admissible at all)*: all N−1 parties **armed before the first hop and never re-armed** (a per-instance arm-POST count of exactly 1, plus the reported `address` byte-identical to what it was at arm time, because a re-arm changes the ephemeral port); each asserted `armed:true` immediately before its own hop is dialled, so an expiry fails by party number rather than as "hop 8 could not connect"; the per-hop words watcher keyed on the **absent→present transition** with per-hop filenames, because one filename plus a reset is safe only at N=2 and a stale file from hop k−1 otherwise satisfies hop k's stimulus check; and the block count asserted against **`N_signing` derived from the roster the driver was handed**, never a literal — through `Initiate` every intermediate party signs twice, so the count is 2(N−1) and becomes N only over S05's carry route. The **distinct-signer set** is asserted too, because `/ByteRange` counts blocks and one party signing four times satisfies any count.

#### P07.S04 — `coSignExchange` re-based off the record *(D22 as amended 2026-08-23, D2 pin; C01)* *(done 2026-08-25, v1.117.169 — T01–T04; clause 1 and clause 4's N≥3 half RESEQUENCED behind P07.S05, clause 5 met at S02b)*
Tasks (grilled 2026-08-25 — `grills/2026-08-25-p07s04-cosign-rebased.md`; deepdive folded in, because its whole output is this slice's shape):
- T01 — the seam: `Attestations(st sign.Status, p Proceeding)` beside `ReadAttestations(pdf)`. **This is the "T01 seam" clause 2 already names, and it is the same seam clause 6 wants.** *(done — plus `ceremony.ProceedingOf`, ONE door for "which proceeding is this document's", because deriving it twice is how one of them ends up comparing the signatures to each other)*
- T02 — `OneProceeding` means agreement **with the document's record**, not agreement among signers. *(done — and a mutation pass found a THIRD false-✓ shape neither arm reached: a record that is present and **unsigned**. `ProceedingOf` goes through `CheckRecord`, so a roster nobody vouched for supplies no commitment.)*
- T03 — the token carries its format version. *(done — `[NibRoster:<v>:<hash>]`, and **both or neither**: `reason()` emits no token for a hash without a version, and the reader refuses version 0, so the pair cannot be produced through one door and not the other. The UI gained the branch that says "this is a version difference, not a disagreement" — which the published-shape reader scan is what forced, by refusing a field nobody was told about.)*
- T04 — the attestations route stops re-verifying. *(done — it reads `document.sig`, and the proceeding lookup is **conditional** on a signature actually naming a ceremony, so an ordinary document pays no pdfcpu parse per request. `NextPlacement` is **not** changed and says why: its two call sites hold freshly-built bytes with no cached status to read, so there is nothing there to stop re-verifying. The number is not asserted — see the size finding above; the guard is structural and states that.)*
- T05 — red proofs; `recorded` moves. *(done 2026-08-25, v1.117.170 — five rows, one of which exists only because a mutation pass found what review did not; `recorded` 100 → 105)*

**MEASURED at the grill: the ✓ this slice would make reachable is false today.** `markOneProceeding`
(`attestation.go:234`) compares commitments **only to each other** — the clause says so, and the probe
shows the cost. Two parties signing with the same arbitrary `abab…` commitment they chose themselves,
on a document carrying **no `nib-ceremony.json` at all**, report `oneProceeding: true` on every
signature, and `web/app.js:3814` renders *"✓ One proceeding — every signature on this document commits
to the same ceremony."* The token is inside the signed `/Reason`, so it is a value the signer picks.
**It is latent only because nothing populates the token** — and this slice is the change that
populates it. So T02 is not an improvement shipped alongside C01; it is a **precondition** of C01 not
shipping a false ✓.

**Clause 1 is BLOCKED on P07.S05 — the same wall S03b's T03 hit**, and for the same measured reason:
`TestTheRelayCeilingAtFourParties` shows the carrier cannot re-sign, so a four-party ceremony cannot
complete until S05's carry verb exists. Clause 1 and clause 4's N≥3 half are **resequenced behind
S05**, not parked.

**Clause 5 was met at P07.S02b, by a different slice** — the TCP-ceremony re-delivery gap (`rd` read
off `anchor.cer`, empty on the accept-loop path). Guarded structurally; `/pending 289` owns the
behavioural drive. Recorded so this slice's ledger does not credit itself with it.

**Clause 6's number is SIZE-driven, not signature-driven.** Measured: nine signatures on a 31 KB
document cost single-digit milliseconds and scale roughly linearly in signature count. The 5.2 s
figure is dominated by document SIZE — each signature's byte range is hashed over the whole file, so
the cost is size × signers. The remedy is right either way, but **the fixture has to be large or the
criterion cannot see its own number**, which is the "instrument coarser than the clause" failure.
Scope: `coSignExchange` refuses anything but a single prior signer, and the two checks under it bind the
document's signer to the **wire** peer and require that signer to have accepted **this user** — the second is
what D22 re-bases. `att` sets no `RosterHash`, so today no production signature carries a commitment at all.
Per D22's amendment, `AcceptedPeer` names the previous roster entry with `signs:true`; `crossBind` stays
document-scoped and unchanged. C14 and C16 belong to **S05**, which is where a non-signing convener can first
complete a ceremony at all.
Acceptance:
- A four-party ceremony completes, every signature `Valid`, every attestation carrying the same `[NibRoster:…]` commitment with `OneProceeding` true.
- **The commitment on every valid signature equals `RosterHash()` of the record embedded in the same document.** `markOneProceeding` compares the tokens only to *each other*, so a document with no record at all whose signers share an arbitrary 64-hex reports one proceeding today. This clause needs the T01 seam and is why T01 is not optional.
- The **token carries a version** (`[NibRoster:1:…]`), because `FormatVersion` sits inside the roster preimage — so a format skew between two builds yields different tokens, `OneProceeding` goes false, and the UI accuses the parties of tampering. That is exactly the wrong-sentence failure D32 exists to forbid, arriving through the one surface D32 excused.
- A document arriving from a peer who is not the record's carrier for this hop is refused by name.
- **Re-delivery works on BOTH transports.** A TCP ceremony routes past the re-delivery cache entirely, so the receiver consents, signs, and the signature is discarded in-process — at N=9 that is eight independent chances. Driven per transport, because `pairrepro.sh` drives both and a clause that does not say so will only ever exercise QUIC.
- `ReadAttestations` is split so the two HTTP handlers and `NextPlacement` read the **already-computed** `document.sig` instead of re-verifying: measured at **5.2 s per request** for nine signatures on a 95 MiB document, on request-handling paths, with the answer already cached and thrown away. **A criterion with a number in it**, because one phrased as "completes" cannot see a quadratic. (CLAUDE.md hot-path rule — the change is a removal of work from that path, not an addition.)

#### P07.S05 — The carry route: a non-signing convener moves the baton *(D22 pin; C02, C07, C14)* *(done 2026-08-25, v1.117.176 — every clause met at the PROTOCOL level over both transports; C21/C22 and the N=4/N=9 harness drive moved to S05a)*
Tasks (grilled 2026-08-25 — `grills/2026-08-25-p07s05-the-carry-route.md`; deepdive folded in):
- T01 — `AcceptedPeer` names the **previous signing roster entry** (D22 amended), not the wire peer. *(done 2026-08-25, v1.117.172 — and **the direction was wrong in this task's own wording until the suite refuted it**: written as the NEXT signer, three two-party ceremony tests failed with *"peer's signature does not accept you"*. `crossBind` matches only a party who has ALREADY signed, so accepting forward leaves every signature unmatched until its successor lands and the last one unmatched forever. C14 as amended says predecessor, and that is what it is.)*
- T02 — the receive-side binding re-based. *(done — and **L3 subsumes BOTH old checks** rather than one being relaxed: its prefix rule establishes that the signatures are the roster's, in order, valid and cross-bound, against the record verified at arm time. What it does not say is that the party on the SOCKET belongs to this proceeding, so `InRoster` is asked and nothing else is. `confirmCoSigned` was re-based with it — a chain's first signer accepts nobody, so demanding that my own signature accept the peer fails for the initiator, measured.)*
- T03 — `p2p.Carry`. *(done — and the two return checks are **not** redundant, measured: with the prefix check alone a hostile hop's reply signed by the WRONG party is accepted, because the prefix says the bytes grew from mine and says nothing about who signed the part that grew.)*
- T04a — the route takes the carry path when the roster says so. *(done 2026-08-25, v1.117.173 — and there is **no separate route and no flag**: whether you sign is a fact about the roster, so `/api/session/initiate` reads it. A non-signing convener cannot accidentally sign and a signer cannot accidentally skip their turn — unrepresentable rather than checked. The decision is asserted to come BEFORE `buildCoSigned`, which applies the local signature.)*
- T04b — ONE ceremony document per relay rather than an arrival per hop. *(done 2026-08-25, v1.117.176 — `installCeremonyResult`, a **fourth commit door**, and the reason it is a fourth rather than one of the three is the whole task.)* **It needed a commit-door decision rather than a line of code.** `handleSessionInitiate` ends in `s.addDoc(...)`, which adds an arrival per hop — at N=9 the convener ends with nine documents against a cap of eight. Replacing instead of adding means a door that writes over an existing document, and the three that exist (`commitMutation`, `commitBarrier`, `addDocCapped`) all run **D29's freeze**, which refuses a convened document by design. A relay is precisely how a convened document legitimately changes, so this needs either a fourth door or a named exemption — and writing a freeze exemption is not something to jam onto the end of a slice.
- T05 — a four-party ceremony with a `signs:false` convener completing; the same through `Initiate` failing. *(done at the PROTOCOL level, both transports — three signatures, none the convener's, every one after the first cross-bound, chain complete. The server-side route drive belongs with T04.)*
- T06 — red proofs; `recorded` moves. *(done 2026-08-25, v1.117.176 — and the **owed pair was closed rather than carried**. The reason they were owed was that their fixture ran the checks in a helper beside the test, which is a fixture asserting itself: the mutation that matters is deleting a check from `Carry`, and a helper carrying its own copy stays green against exactly that. A hostile RECEIVER is buildable in-package — accept, run the spoken check, read the frame, reply with anything — so both arms now drive the real verb and each fails on its own arm under mutation.)*

**MEASURED at the grill: the receive side refuses a carrier, and L3 disagrees with it.** A
three-party roster with a `signs:false` convener, A having signed, the convener carrying to B:
`coSignExchange` answers *"the document was not signed by the connected peer"* while
`AdmitContribution` answers `<nil>` and names B as the next contributor. The binding compares the
document's last signer against the TLS-pinned **wire peer**, and under a carry route the wire peer
is the carrier while the last signer is the previous *signing* party. **That conflation — "who
signed" with "who I am connected to" — is true only while every carrier also signs**, which is the
assumption this slice exists to remove. `crossBind` then reports the last signature as unmatched
until its successor signs, which is C14 as amended in as many words.

**SPLIT — the surfaces and the harness become P07.S05a.** C16/C18's rendering, the mirror per hop
(C22), the record deadline (C21), the N=4/N=9 driver over both transports and the LAN re-announce
are downstream of the verb existing: their first run would otherwise also be the verb's first run,
and a failure ambiguous between them — the argument that split S03a from S03b.

**The three deferred clauses land with THIS half**, not S05a: `S03b`'s T03, `S04`'s clause 1 and
`S04`'s clause 4 N≥3 half were all resequenced behind "S05's carry verb", and this is that verb.
Scope: there is no carry verb — `Initiate` demands the caller's own signature back, and
`SendDocument`/`ReceiveDocument` is the one-way flow with no attestation and no record. C07 says in as many
words that it *cannot pass on `Initiate`*, which is true of the code as it stands.
Acceptance:
- A four-party ceremony with a `signs:false` convener completes **over the carry route**, driven by the harness; the finished document contains **no signature of the convener's**; and the same ceremony attempted through `Initiate` fails.
- **Every signature that has a signing predecessor reports `Matched`; the first signer reports its own state; an attestation naming a roster fingerprint that produced no valid signature reports `Matched: false`** (C14 as amended).
- ~~The completed document renders as complete (C16) … the LAN tier is re-announced at hop k~~ **MOVED to P07.S05a, 2026-08-25 at this slice's grill** — rendering and harness work, downstream of the verb existing at all.


#### P07.S05a — The ceremony's surfaces *(C16, C18, C22; D22 pin)* — **new, 2026-08-25, split out of S05** *(done 2026-08-25, v1.117.179 — three clauses met, one PARKED (C21); the completeness clause had three doors and the plan named one)*
Tasks (grilled 2026-08-25 — `grills/2026-08-25-p07s05a-the-surfaces.md`; deepdive folded in):
- T01 — the consent card names which signature it describes, including the hop-1 carry case where there is none.
- T02 — the proceeding carries the obliged signers; the attestations route publishes signed-and-obliged counts; the client renders complete vs "5 of 9 obliged signers".
- T03 — every hop writes the mirror before its response returns (C22).
- T04 — red proofs; `recorded` moves.

**THE MEASUREMENT THAT DECIDED T02's SHAPE: every cheap gate on the proceeding lookup is unsound,
and the route pays one parse.** The lookup is a pdfcpu read on a request path, so the first two
attempts were both gates. A **byte scan for the attachment's name is a FALSE NEGATIVE on a real
record** — measured, `Extract` succeeds while `bytes.Contains(pdf, "nib-ceremony.json")` is false,
because pdfcpu puts the file-spec into a compressed object stream, so it is not even a necessary
condition. **Caching the proceeding beside `doc.sig` is unsound twice** — fourteen sites assign
`sig`, so ADR-009's one door would have to be built first, and `ProceedingOf` takes `now` because
the record expires. So the lookup is unconditional, and the hot-path rule is satisfied by what the
route no longer does: before S04 it re-verified every signature over the whole file (size × signers)
and now it reads the file once, independent of signature count, on a route a user opens by clicking.

**TWO CLAUSES BELOW WERE ALREADY MET, BY P07.S05** — the carry route's return binding (driven
through the real verb against a hostile receiver, both arms) and the baton replacement
(`installCeremonyResult`, driven at nine hops). They were listed here when the split was drafted
and settled into S05 as it was built. Recorded so this slice's ledger does not credit itself with
another slice's work.

**C18's OWN PREMISE STOPPED BEING TRUE AT P07.S04, and that is what makes it buildable.** C18 says
the verdict is unbuildable because *"nothing in the verdict path knows a roster"* — true when
written, and false since `ceremony.ProceedingOf` put a record reader on the attestations route. So
the proceeding already crossing that boundary carries the obliged signers, and **C16 falls out of
the same count** rather than needing a mechanism of its own: a `signs:false` convener is not
obliged, so a completed ceremony with one renders complete.

**C21 IS BLOCKED ON `/pending 247`, WHICH WAS DECLINED ON A MEASURED ARGUMENT — parked, see the
closing batch.** `session.go:1137` sets the arm's bound to `MaxCeremonyLife`, and the comment gives
the plan's own reason: *"the invitation carries no per-ceremony deadline"*. Adding one is 247,
deferred behind three gates; G1 was discharged at S02b, and **G2 is not bookkeeping** — it measured
that a tamperable arm-time deadline is *"negative in one direction, zero in the other"*, since an
attacker who SHORTENS it closes the arm early and `MatchesRecord` cannot run until the document
arrives. So C21 asks for the field 247 declined to build, for a reason 247 measured; **the clause
is left unbuilt and the collision written here rather than quietly satisfied.** What is NOT blocked,
and is worth separating: `checkCeremonyDeadline` already refuses to START a hop that cannot finish
before the record expires, which is the harm C21's sentence is about. What remains is an arm that
stops *waiting* — cost, a listener idle for up to thirty days.

**SPLIT — the N-party driver becomes P07.S05b.** It asserts what a completed relay *renders*, so
building them together makes the driver's first run also the renderer's first run.
Scope: everything about a completed relay that is not the relay. Downstream of S05, which is what
produces the document these clauses describe.
Acceptance:
- **The consent card names WHICH signature it is describing.** Owed from P07.S05: `coSignExchange` picks `ats[len(ats)-1]` for the gate, and at hop 1 of a carry route there is no prior signature at all, so it is handed the identity the TLS handshake pinned with `Valid` false. The retired `channel-binding-reads-the-first-signer` row proved the OLD consequence of getting that index wrong; nothing proves the new one.
- The completed document **renders as complete** (C16) **and a five-of-nine document renders as incomplete, naming how many obliged signers are absent** (C18). The two are one piece of work and neither is safe alone. **Built — and the clause had THREE doors, of which the plan named one.** The count is computed by `p2p.Completeness` and published by the attestations route; the two that were not planned were found by driving it: (1) the route's proceeding lookup was gated on `ClaimsAProceeding` — *does any signature name a ceremony* — so a **convened but unsigned** document, which is C18's own extreme case, reported no counts at all; (2) the details modal is reached through a button gated on `signers.length` and an early return on the same, so even a correct count was **unreachable** on that document. Both fixed, both guarded (`TestAConvenedDocumentReportsItsObligedSignersBeforeAnyoneHasSigned`, `ceremonycomplete.test.mjs`), both mutation-tested.
- ~~The carry route binds what comes back to what went out…~~ **MET AT P07.S05** (`TestCarryRefusesAHostileHop`, both arms, through the real verb against a hostile receiver). Recorded rather than re-credited. `Initiate` has this and says why; without it the convener relays whatever a malicious party hands back, and S03's door — which answers the *contributor's* question — is passed through by nobody.
- ~~A hop replaces the baton rather than accumulating arrivals…~~ **MET AT P07.S05** (`installCeremonyResult`, driven at nine hops). Recorded rather than re-credited.
- Every hop's output is **written to the mirror before the response returns** (C22). **Built** — `mirrorHop`, called at `session.go:1543` before `writeJSON` and at `openArrival` (`:815`) for the receiving side, which C22's "before the response returns" wording does not reach because there is no response there. ~~and an arm ends at the record's deadline rather than at `MaxCeremonyLife` (C21)~~ — **C21 PARKED, see above.**
- ~~The relay is expressed once … the LAN tier is re-announced at hop k~~ **MOVED to P07.S05b, 2026-08-25 at this slice's grill** — a harness slice, downstream of the surfaces it asserts.


#### P07.S05b — The N-party driver at N=4 and N=9 *(C05, C21 pin; D22)* — **new, 2026-08-25, split out of S05a** *(done 2026-08-25, v1.117.181 — the relay COMPLETES at N=4 and N=9 over both transports; it absorbed P07.S03b's T03 and found three production defects, none of them in the relay; the LAN clause moved to S05c)*
Scope: the tier-4 harness for a completed relay. Downstream of S05a, which produces what it asserts.
It also absorbs **P07.S03b's T03**, which was resequenced behind S05's carry verb and is the same run.

Tasks (grilled 2026-08-25 — `grills/2026-08-25-p07s05b-the-n-party-driver.md`; deepdive did NOT fire,
and the trigger is recorded there rather than skipped):
- T01 — `ceremony()` parameterised over (from, to); the two-party callers are the control. *(done — the two-party run is byte-for-byte the same steps, so the refactor's own control is that nothing about it changed)*
- T02 — the relay driver at N=4, both transports, hop k's document a byte prefix of hop k+1's. *(done — and it found three production defects, above)*
- T03 — N=9, and the word-string property as amended below. *(done — 8 hops on both transports, 8 distinct strings each, and no string shared between the two relays)*
- **T03b — P07.S03b's T03, absorbed here rather than left pointing at a coordinate that has closed.**
  Its own list asked for more than "the relay completes": every party **armed before hop 1 and never
  re-armed**, each asserted `armed:true` immediately before its hop so an expiry fails by party
  number, and the **distinct-signer set** — because `/ByteRange` counts blocks, so one party signing
  eight times satisfies every per-hop count. *(done — and the binds had to become EPHEMERAL for the
  no-re-arm check to have teeth: with a fixed port the address is identical before and after a
  re-arm, which is the vacuous green the clause itself warns about.)*
- ~~T04 — the seek announcement~~ and ~~T05 — `--lan -n 9`~~ **MOVED to P07.S05c**, see above.
- T06 — red proofs; `recorded` moves. *(done 2026-08-25, v1.117.181 — four rows, and THREE of them record the state the product had been shipping in for four slices. The fourth is **tier 4**, because the accumulation property needs a real relay with enough hops to cross ADR-005's cap and is a DELTA rather than a total. `recorded` 117 → 121.)*

**BUILDING T01–T02 FOUND THREE DEFECTS, AND THE LARGEST IS P07.S04's MISSING WRITER.**

- **No production signature carried its ceremony commitment.** S04 shipped the `[NibRoster:v:hash]`
  token format, the reader, D32's version-skew sentence and L3's substituted-proceeding check —
  and **no writer**. Neither contribution door set `Attestation.RosterHash`, so `OneProceeding` was
  false on every real ceremony, C19/C01's "every signature names its ceremony" was unimplemented,
  and `ErrProceedingMismatch` was unreachable. Measured: a completed relay hop's attestation read
  `[NibCoSign:1] Accepts p1 [SPKI:]. I accept`. Fixed by `p2p.StampCommitment`, ONE door called by
  both contribution sites (ADR-009), with `Roster` gaining a `CommitmentVersion` because
  `internal/p2p` cannot import `internal/ceremony`. **And the guard meant to catch this could
  never have fired**: `TestTheCommitmentCheckIsLimitedUntilS04` had a `Skip` arm for the day a
  signature carried a commitment, over a fixture it hand-signed with an explicit empty one — it
  measured its own input. Replaced by `TestACeremonySignatureNamesItsCeremony`, which drives the
  production door, plus a second arm on the two-door routing walk.
- **A ceremony hop could not be dialled over TCP at all.** `dialerCeremony` opens a QUIC shared
  endpoint unconditionally, so `handleSessionInitiate` took the glare branch for every hop, and
  that branch feeds its race through `filterQUIC` — every TCP candidate dropped, the hop spinning
  until `connectDeadline` with the receiver armed and idle. **No tier had ever carried a document
  over a ceremony dial on TCP**: the N≥3 probe is refused 409 at the near end before any network
  work, and the two-party runs carry no invitation. Fixed for an explicitly-named transport; the
  residual — an unnamed transport still drops TCP candidates learned from the LAN or DHT — is
  `/pending 298`, because it is a change to P05's coordinator.
- **The arm must carry the invitation** (harness). `handleSessionArm` resolves the ceremony from
  `req.Invitation` and the receive path reads its roster from there; without it the receiver has an
  empty roster and refuses hop 1 with *"a co-signature takes exactly one prior signer"* — correctly,
  because outside a ceremony an unsigned document is not something to co-sign.

**And one POSITIVE confirmation, recorded because a driver's job is to confirm as well as refute:**
S05a's completeness counts are right in a live relay — the attestations route reported
`obliged: 3, signed: 1` at hop 1 of a four-party ceremony with a non-signing convener.

**CLAUSE 1's ARITHMETIC IS WRONG, AND THE SHIPPED HARNESS REFUTES IT.** *"All 2(N−1) word-strings
pairwise distinct"* is unsatisfiable by construction: `pairrepro.sh:697` asserts that the two sides
of a hop derive the **same** string, which is L2's entire point, so N−1 of the 2(N−1) pairs are
necessarily equal. Re-stated as two properties, and the pair is **stronger** than the original —
"pairwise distinct over 16" would have been satisfied by a run that never compared the two sides of
anything.

**CLAUSE 2 NAMES A MECHANISM THAT CANNOT EXIST.** `startAnnouncing` runs on the **armed** side
(`session.go:642`, `:1116`) and `browsePeers` only listens, so **a dialer has nothing it can send
that makes a remote peer speak** — the convener's dial cannot re-announce party k. The harm is real
(`lan.go:34-43` measures the naive fix at 5.2M datagrams per ceremony) and the intent is one hop from
a good mechanism: **the seeker announces and the armed side answers**, ~2 datagrams per hop and no
standing beacon. A slow beacon was refuted by measurement — the browse is **2 s** (`discover.go:21`),
so a 30 s beacon is heard 7% of the time, and matching the two costs either 1.3M datagrams or 30 s
added to every remote hop. The cost of the recommendation is stated: it is a **discovery wire-format
change**, version 2 → 3 under ADR-010. L1 is unchanged and is what makes an answer safe — a seek
carries the six-word name, and `Matches` resolves it only against a fingerprint the receiver already
holds, so an armed party answers only a name it already recognises.

**THE DHT-OFF RUN NEEDS TIER 5's ISOLATION, AND `--lan -n 9` CANNOT REACH IT TODAY.** No
DHT-disable switch exists and none should be added — `redproof.sh`'s own argument, *"a switch whose
whole purpose is to break the program is the same gun with a better excuse"*, covers a switch that
disables a tier. The namespace `--lan` already runs in IS the instrument. But the `N != 2` probe at
`:797` `exit 0`s at `:900`, three lines before the `LAN` block, so the two modes are mutually
exclusive by ordering rather than by design.

**SPLIT AFTER ALL — the LAN re-announce becomes P07.S05c, and the grill's own argument INVERTS
once the driver exists.** The grill said not to split: *the ambiguity a split would remove is
removed by ordering T04 after T03*. That held while the driver was unbuilt. It is built and green
now — N=4 and N=9, both transports — so a split no longer creates the ambiguity the earlier splits
were avoiding; it removes it, because the instrument that would judge the announce change is
already landed, passing, and committed.

**And measurement changed what the clause IS.** The grill costed it as a discovery wire-format
change (a "seek" datagram, version 2 → 3). Reading the code refutes that: `runCeremonyReceive`
already announces its shared endpoint for `lanAnnounceWindow`, and its own comment says *"the
dialing side BROWSES for a ceremony peer"* — so the browse exists and the seek does not need a new
message at all. **The asymmetry that decides the design is that LISTENING IS FREE**: the party who
must be found can hold a browse open for the ceremony's whole life at zero egress, while every
beacon shape pays per datagram. What is missing is one bounded announcement from the DIALING side,
which today announces nothing. That is a change to P05's ceremony networking on both sides — not a
harness change, and not a wire-format change either.

So S05b closes as what its title says it is: the N-party driver.

Acceptance:
- **The relay is expressed once, in the baton topology**, driven at N=4 and N=9 over both transports *(moved here from S01 at its grill, 2026-08-23 — this is the first slice with a carry verb, so it is the first slice in which the topology is final rather than a chain S05 would rewrite)*, with the 2(N−1) word-string observations **equal within each hop and pairwise distinct across hops** *(amended 2026-08-25 at this slice's grill — "all pairwise distinct" is unsatisfiable, see above)*, and each hop's document asserted to **contain the previous hop's as a byte prefix** — not merely to differ from it, which at N is a tautology, since `/api/pdf` returns that instance's active document and no instance is fetched twice in one relay.
- The LAN tier is **re-announced when the convener's dial for hop k begins**: the announce window is five minutes, so from the fourth party onward a "same room" ceremony silently runs over the public DHT. Driven by a nine-party ceremony completing **with the DHT tier disabled**.

#### P07.S05c — The LAN tier at hop k *(C05 pin; D6, D8)* — **new, 2026-08-25, split out of S05b** *(done 2026-08-27, v1.117.182–.185 — the mechanism, its driver, and the three barriers that made the clause undrivable; the EGRESS clause is RED against shipped code and moves to S05d with the instrument that found it)*
Tasks (grilled 2026-08-25 — `grills/2026-08-25-p07s05c-the-lan-tier-at-hop-k.md`; the deepdive did
NOT fire, and the one trigger that would have — a wire-format move — was *checked* rather than
assumed, because the S05b grill had costed this as a discovery version bump):
- T01 — the dialing side announces its ceremony endpoint before it browses, bounded by the hop. *(built — `hopAnnounceWindow`, and the ORDER is the whole of it: arm, announce, browse)*
- T02 — an armed party browses for the arm's life and re-announces on hearing a peer it has pinned
  for this ceremony; bounded, rate-limited, never itself a seek. *(built — `answerHopSeekers`, which answers ONE fingerprint, the peer the arm was raised for; under D22 that is the convener, and it is what stops two armed parties answering each other forever)*
- T03 — the driver reaches a hop **after** the announce window has expired; the window becomes a
  parameter so the test does not wait five minutes. *(done, and at tier 1 rather than tier 4 — see below)*

**T01/T02 SHIPPED WIRED, CORRECT AND UNPROVEN, WHICH IS THE VACUOUS-GREEN SHAPE.** Every hop of
every run in the tree falls **inside** the five-minute announce window, so the one state the
mechanism exists for — a peer arriving after it has expired — was reached by nothing. A real socket
cannot drive that without five minutes of wall clock or a knob in the shipped binary, so the
POLICY was separated from the socket (`answerLoop`) and driven with a fake clock and a fake
browser: a stranger gets no answer, the arm's own peer does, the same peer inside the window does
not, and after the window does. Three mutations, all caught.

**And the first version of that guard could not see the property it was written for.** `answer`
took no arguments, so the test could count answers and not identify them — and a stranger's
sighting plus a later one produces the same COUNT as the peer's sighting plus a later one. Deleting
the `resolve` gate left it green. The seam now hands the resolved candidate to the answer callback,
which production ignores and the test asserts on. **L1 is a property about WHICH peer, and a count
cannot see it.**
- T04 — `--lan -n 9` reaches the LAN block, and a nine-party ceremony completes in the namespace. *(the three barriers are removed and a four-party LAN relay COMPLETES over both transports — see below. N=9 not yet driven.)*
- T05 — egress measured and reported for N=9, against the 5.2M figure `lan.go` refuses. *(the instrument is built and wired; the measurement is RED against shipped code and the clause **moves to P07.S05d** — `/pending 299`. Recorded as resequenced rather than met, because the run exists and says so.)*
- T06 — red proofs; `recorded` moves. *(done 2026-08-27, v1.117.184–.185 — four rows, `recorded` 121 → 125. **Three record states no run in the tree reaches**, which is why the policy was separated from its socket. The fourth came back **"the check still PASSED"** and was right: `--lan -n 4` losing its `-n` is invisible by construction, because a two-party ceremony passes its own assertions. So the requested N now travels out of band and the child compares parsed against asked — a guard for the class, not the instance.)*

**THE CLAUSE'S DRIVER HAD THREE INDEPENDENT BARRIERS, AND ANY ONE ALONE MADE IT IMPOSSIBLE.** An
explicit *"`--lan` and `--v6` are N=2-only"* refusal, dating from P07.S01 when there was no relay
for either to drive; the `N != 2` block's `exit 0`, three lines before the `LAN` block, so the two
modes were mutually exclusive by ordering rather than by design; and **the namespace re-exec
silently dropping `-n`** — `FLAGS` carries `--lan --keep --v6` and nothing else, so `--lan -n 4`
re-executed inside the namespace as `--lan` alone and ran a TWO-PARTY ceremony while printing a
pass. That last one is the exact defect `FLAGS` was created for, one flag later.

**And with them gone, the first measurement of a ceremony on a link found a D6 leak the plan's own
criterion forbids.** A two-party LAN ceremony emits **zero** off-link packets; a four-party LAN
relay emits **120**. The difference is the invitation: a ceremony hop calls `dialerCeremony`, which
bootstraps the public DHT unconditionally, and the arm side does the same — `ceremonynet.go`
suppresses the late *publish* when the LAN answers and nothing suppresses the *bootstrap*. So P03's
exit criterion, *"a LAN ceremony completes with NO outbound internet traffic"*, is false for every
ceremony that carries an invitation. `/pending 299`.

**A second finding is recorded with its cause NOT established**, because saying so is the honest
state: running the QUIC relay before the TCP one makes the TCP relay fail at hop 1 with a D19
verdict about the DHT, for a peer that is on the link and announcing. TCP first, both complete.
`/pending 300`. The harness runs TCP first and says why at the line — QUIC-first masks the leak
above, because the run dies before reaching the egress assertion.

**THE MEASUREMENT THAT SETTLES THE DESIGN, AND IT REVERSES S05b's GRILL.** That grill costed this
as a discovery format version 2 → 3 "seek" datagram. Reading the code refuses it: **the seek is the
convener's ORDINARY announcement**, because a convener in a QUIC ceremony already holds a listening
endpoint (`connect` arms `QUICListenHandshakeOn` on the shared socket), so it has something
truthful to announce and needs no new message. What decides the shape is an asymmetry rather than a
trade-off — **listening is free**: every beacon pays per datagram and a held-open browse pays
nothing, so the party who must be found listens indefinitely and the party who knows a hop is
starting sends one bounded burst.

**Four measurements shape the build.** `peerAddresses` **never browses when an address is typed**
(`lan.go:296`), so the change must not hang off the typed path. `findPeerOnLAN` is a **one-shot
2-second browse** (`lan.go:234`, `discover.go:21`) — enough for an announce-then-listen exchange,
but **the order matters**, since an answer that arrives before the browse opens is lost. **The armed
side never browses at all**, which is the half that does not exist. And `runCeremonyReceive`
already announces, with a comment that says why — the machinery is there and the *window* is what
is wrong.

**One existing sentence is amended rather than quietly contradicted.** `lan.go:43` ends *"The
socket is released when this closes, not held idle until disarm."* Holding a **receive** socket for
the arm's life breaks its letter and keeps its reason, which is datagrams — a listener emits none.
The comparison belongs in the code: a socket and a goroutine for the ceremony, against 5.2M
datagrams.
Scope: a ceremony that starts in one room stays on the link for all N−1 of its hops. Downstream of
S05b, whose driver is the instrument that judges it and is already green.

**The clause the plan wrote cannot be implemented, and the mechanism that CAN be is cheaper than
the one the grill designed.** The text says *"the LAN tier is re-announced when the convener's dial
for hop k begins"*. `startAnnouncing` runs on the **armed** side (`session.go:642`, `:1116`) and
`browsePeers` only listens, so a dialer has nothing it can send that makes a remote peer speak. The
harm is real and `lan.go:34-43` measures it against itself: the arm lives for the ceremony's life —
up to D33's thirty days since P05.S09b — while the announcement stops after **five minutes**, so
from the fourth party onward a same-room ceremony silently runs over the public DHT. Keeping the
2/s ticker for the arm's life is ≈5.2M datagrams per ceremony, which is what that comment refuses.

**The asymmetry that decides the design: listening is free.** Every beacon shape pays per datagram;
a browse held open pays nothing at all. And the pieces are already built — `runCeremonyReceive`
announces its shared endpoint and its own comment says *"the dialing side BROWSES for a ceremony
peer"*. What is missing is one **bounded announcement from the DIALING side**, which today announces
nothing, so the armed party can hear the convener arrive and answer. That is not a wire-format
change: the grill costed it as a discovery version 2 → 3 "seek" datagram, and reading the code
refutes that — the seek is the convener's ordinary announcement, which it must send anyway.

Acceptance:
- The **dialing** side announces its shared endpoint while a ceremony hop is in flight, bounded by
  the hop rather than by `lanAnnounceWindow`, and stops when the hop ends.
- An armed ceremony party **hears its convener arrive after its own announce window has closed**
  and the hop completes on the link — driven at the hop where today's five minutes have expired,
  because a hop inside the window passes without exercising anything.
- A **nine-party ceremony completes with the DHT unreachable**, in the namespace `--lan` already
  runs in, with the egress counter still at its baseline. **No DHT-disable switch is added:**
  `redproof.sh`'s own argument — *"a switch whose whole purpose is to break the program is the same
  gun with a better excuse"* — covers a switch that disables a tier, and the namespace is the
  honest instrument.
- `--lan -n 9` **reaches the LAN block at all.** Today the `N != 2` probe `exit 0`s three lines
  before it, so the two modes are mutually exclusive by ordering rather than by design.
- Egress is **measured, not argued**: datagrams per ceremony reported for N=9 and compared against
  the 5.2M figure `lan.go` refuses.

#### P07.S05d — A LAN ceremony stops reaching the internet *(C05 pin; D6, D8, P03's exit criterion)* — **new, 2026-08-27, split out of S05c** *(done 2026-08-27, v1.117.205)*
Scope: the DHT bootstrap stops firing before anyone knows whether the LAN will answer. Downstream
of S05c only in the sense that matters — **its instrument already exists and is green on everything
else**, which is the condition that made the last two splits right rather than evasive.

**Measured, not argued.** In the namespace, with an nft counter on off-link traffic: a **two-party**
LAN ceremony emits **0** packets; a **four-party** LAN relay emits **120**. The difference is the
invitation. `dialerCeremony` opens a rendezvous and calls `rz.Bootstrap` unconditionally, and
`runCeremonyReceive` does the same — so the public DHT is contacted on every hop, before anyone
knows whether the link will answer. `ceremonynet.go` already suppresses the late PUBLISH and
further punch packets under LAN-window logic, and says so; **nothing suppresses the bootstrap**.

So **P03's exit criterion — "a LAN ceremony completes with NO outbound internet traffic" — is false
for every ceremony that carries an invitation**, which is every ceremony P07 builds. It survived
four phases because the only `--lan` run was the two-party one, which has no invitation and
therefore no `cer` at all.

**The shape, and it is a shape rather than a decision.** The dialer browses BEFORE it dials
(`peerAddresses`), so on that side the answer is already in hand: bootstrap only when the browse
returned nothing. The arm side is the harder half — it cannot know whether the dialer will find it
on the link — and the existing `publishWhenSlow` delay is where a lazy bootstrap would naturally
fold in. The cost to state and measure: the DHT tier starts later, which D8's racing ladder is
built to absorb, but `bootstrapBudget` and the punch both want the socket warm early.

Acceptance:
- ~~A **nine-party** LAN relay completes in the namespace with the egress counter **at its
  baseline**, over both transports.~~ **Resequenced to S05e, 2026-08-27, on a measurement.** See the
  split note below.
- A ceremony with **no LAN peer** still reaches the DHT tier, and the added latency is **measured
  and printed** rather than assumed acceptable.
- The arm side's `bootstrapDone` reader keeps a truthful answer: D19's arm-side diagnosis is only
  meaningful after a bootstrap, and a lazy one must not make that flag lie.
- The **dial** side stops guessing: `peerAddresses` browses before the race, so a `sourceLAN`
  candidate is the link having already answered, and the DHT tier holds on that fact rather than on
  a two-second timer racing the hop.

**SPLIT at the measurement, 2026-08-27.** With the bootstrap lazy, the fetch windowed and the dial
side holding on its own browse result, a four-party LAN relay went **120 → 9** off-link packets and
a nine-party relay stayed at **~60–111**. A per-call stack probe over all nine instances named the
remainder exactly: **instances 1 and 2 emit nothing; instances 3 through 9 each reach the DHT twice
(publish and fetch), and they are precisely the parties whose hop arrives after their LAN window
has closed.** All eight signing parties arm before hop 1, hops take 1–3 s, and an arm's window is
2 s — so from the third party onward every arm pre-publishes.

That is **a different mechanism from anything this slice touches**, and it needs a signal the arm
does not have: the arm ANNOUNCES rather than browses, and `answerHopSeekers` filters sightings to
its own expected peer, who is not on the link until its turn. The cheap fixes are all timers, and a
timer is what was just measured losing. It becomes **S05e**, below.

Tasks *(from the 2026-08-27 deepdive + grill; verdict confirmed, and the fix is TWO changes)*:
- **T01** — `ensureBootstrapped(ctx)` on `ceremonyID`, `sync.Once`-guarded, replacing **three**
  eager call sites. The scope above names two; `startArmedRendezvous` (`ceremonynet.go:397`) is a
  third, and it is the one the **TCP** arm uses — `runCeremonyReceive` says in as many words that
  it does not start it. `bootstrapDone` is set inside the door, so a path that no longer bootstraps
  eagerly cannot leave its reader believing it did.
- **T02** — `feedCandidates` waits `browseWindow` before its first fetch. **Without this T01 buys
  nothing**: `publishLoop` already takes that first-delay (`:462`, `:478-480`) and the *fetch* does
  not (`:315-316`), so a lazy bootstrap merely moves the first off-link packet by microseconds.
- **T03** — the cost measured and printed: hop start to first DHT reach with no LAN peer.
- **T04** — `--lan -n 9` at the baseline, over both transports.
- **T05** — red proofs; `recorded` moves.

**Ledger — 3 met, 1 resequenced.** Clause 2 (no LAN peer still reaches the DHT, latency measured
and printed): met — **2.005 s**, one `browseWindow`, printed by
`TestTheAddedLatencyToTheDHTTierIsMeasured`. Clause 3 (`bootstrapDone` stays truthful): met, set
inside the door on success **and** failure, red-proved — a flag set only on success inverts D19,
because the machine whose network is dead is the one that never gets told. Clause 4 (the dial side
holds on its browse result): met, two-armed, both arms red-proved. Clause 1 (nine parties at
baseline): **resequenced to S05e** on the per-instance measurement above; four parties went
120 → 9. Four red proofs, `recorded` 132 → 136.

**And tier 2 was red before this slice touched anything** — v1.117.178 added a jsdom file without
raising the count, and nothing had run tier 2 in the nine slices since. Fixed at v1.117.205, along
with two findings underneath it: `/api/lan/heard`'s shapes were outside the reader scan, and the
scan's matcher could not see its reader's style at all. A harness that skips cleanly when its
dependencies are absent also skips cleanly when nobody runs it.

#### P07.S05e — The arm stops pre-publishing *(C05 pin; D6, P03's exit criterion)* — **new, 2026-08-27, split out of S05d on a measurement** *(done 2026-08-27, v1.117.207)*
Scope: an arm whose hop has not arrived yet publishes its address to the public DHT, and in a relay
that is every party from the third onward.

**Measured, per instance.** `./build/pairrepro.sh --lan -n 9` at v1.117.203, with a stack probe on
every call to `ensureBootstrapped`: **i1 and i2 reach the DHT zero times; i3 through i9 reach it
twice each** — once through `publishCandidates` and once through `feedCandidates`, both from the
arm's own `connect` → `feedCeremonyRace`. The pattern is not noise: i1 is the non-signing convener
and i2's hop starts immediately, so those two are reached inside their window. Every other party
arms before hop 1 and waits 2 s × its position.

~~**Why the S05d shape does not extend:** the arm announces rather than browses, and `answerLoop`
filters sightings to its own expected peer, who does not appear on the link until the hop begins.~~
~~**The shape:** an arm that browses for the whole roster renews its DHT hold on evidence.~~

**PIN, 2026-08-27 (S05e's grill). Both struck: the premise was false and the shape it produced was
wrong.**

**The premise, measured at the lines.** Every armed party announces **from the moment it arms**, for
`lanAnnounceWindow` = **5 minutes** at 2/s (`internal/server/lan.go:43`, `session.go:1180`) —
unprompted, not in answer to a seek — and a browse is passive (`discover.go:290`). A nine-party
relay completes in ~20 s. So a waiting arm's predecessor **is** on the link the whole time, and
"does not appear until the hop begins" was simply not true.

**Which makes the roster-wide browse both wrong and unnecessary.** Wrong, because audibility of
*some* roster party is not evidence that *your* hop arrives over the link: roster A→B→C with **A and
C on one link and B remote** has C hold its publish on hearing A, while B — the party actually
dialling C — cannot find it, for as long as A keeps announcing. Unnecessary, because `answerLoop`
(`lan.go:782`) already resolves the right question every iteration — `resolve(pins, seen)`, with
`pins` built from the arm's **own expected peer** (`lan.go:678-683`). That `ok` is *"the party I am
waiting for is on this link"*: already asked, already screened against pins rather than wire bytes
(L1 intact, and a stranger is already dropped there), and already correct on the mixed ceremony
above, because A is not in C's pins.

**So the slice is to feed the sighting the arm already resolves into the hold** — not a new browse,
not a new socket, not a roster walk.

**And the S05d lesson repeats at the same count:** `answerHopSeekers` has **one** caller
(`session.go:1194`, the QUIC arm). The TCP arm never starts it, so wiring the hold only where the
signal already is would fix one of two paths with nothing failing.
Acceptance:
- A **nine-party** LAN relay completes in the namespace with the egress counter **at its baseline**,
  over both transports — `./build/pairrepro.sh --lan -n 9`, which is red at 60–111 packets and whose
  per-instance distribution is recorded above, so a partial fix cannot read as a whole one.
- The hold renews on **evidence**, not on a timer: driven by a fake browser, an arm that keeps
  hearing its peer holds indefinitely and one that stops falls through within `lanFirstBudget`.
- A **stranger's** announcement does not renew the hold, driven separately — a count cannot see the
  difference and `answerLoop`'s own history says so.
- The remote path's cost is measured: an arm with nothing on the link reaches the DHT within
  `lanFirstBudget`, and that number is printed rather than assumed.

Tasks *(from the 2026-08-27 grill; verdict amended — the correction makes the slice smaller)*:
- **T01** — the resolved sighting renews a link-liveness deadline on the ceremony, hooked after
  `resolve` and **before** the answer rate limit. That gate is `hopAnnounceWindow` and exists so a
  re-dial does not stack a second announcer; renewing only on *answered* sightings would let the
  hold lapse during exactly the period the peer is most present. Observing and announcing are two
  different rates over one stream, and one gate over both makes the second inherit the first's period.
- **T02** — the TCP arm gets the same signal, or the slice fixes one of two paths.
- **T03** — driven: renews on the peer, does **not** renew on a stranger, lapses within
  `lanFirstBudget` when the link goes quiet, and an arm hearing nothing still reaches the DHT.
- **T04** — `--lan -n 9` at the baseline over both transports; the remote-path cost printed.
- **T05** — red proofs; `recorded` moves.

**Ledger — 4 met, 0 not met.** Clause 1 (nine-party LAN relay at baseline, both transports):
**met** — `./build/pairrepro.sh --lan -n 9` prints *"16 hops over two transports, and nothing left
the link"*, confirmed on two consecutive runs, with the two-party control unregressed. **P03's exit
criterion is green for the first time since it was written.** Clause 2 (renews on evidence, lapses
without): met, driven by the fake browser and the injected clock, four arms, each probed red.
Clause 3 (a stranger does not renew): met, and asserted on WHOSE sighting rather than on a count —
`answerLoop`'s own first version was green because a stranger's and the peer's produce the same
count. Clause 4 (remote cost measured): met — **30.001 s**, one `lanFirstBudget` from the moment
the arm starts watching, against a 300 s connect deadline.

**The grill made this slice smaller, not larger.** Its scope named a roster-wide browse; that shape
has collateral on a mixed ceremony, and the exact signal was already being computed one function
away in `answerLoop`. What was added is a reader, not a browse. **And `answerHopSeekers` had one
caller** — the QUIC arm — so the TCP arm gained it too, which also closes a findability gap S05c
left open.

#### P07.S06 — Placement: measured, on the pages S02 allocated *(D25; C03)* *(done 2026-08-27, v1.117.210 — 6 met, 1 PARTLY: the rendered half of the differential measurement owed and filed as /pending 302. **Discharged 2026-08-28, v1.117.212**: the item's blocker — "a tier-3 ceremony fixture" — was refuted, the appearance writer being roster-blind and reachable through `/api/cosign/sign` with no peer. `test/ui/blockink.test.mjs` renders the readme page across a sixth contribution and measures the block's ink against the widget's own /Rect; block 6 covers 4660 pixels of readme prose, and blocks 1–2 sit on a blank margin, which is why two contributions were not enough. Three replayable rows, one of which this slice's own structural guard stays GREEN against. Residue: /pending 305, the nine-party rendered case with real block content, which P07.S07's fixture unblocks.)*
Scope, **re-derived from measurement 2026-08-23 — the first firming got all three numbers wrong.** The block
page is **always A4 595×842**, whatever the source paper, because `RenderReadme` hardcodes `A4P`; the earlier
scope said 792. With `y0 = 40 + i*96` and `height 84`: **blocks 3 through 7 overlap the readme body** (baselines
735 → 343) and **block 8 is clipped by 50 pt** — one block lost, not two, and **the overlap fires at four
parties, not nine**, which is what D25 said on 2026-08-18 and the first firming lost. Measured: the whole
nine-block stack applies **silently**, `state=valid signers=9 addedAfter=false`. Allocation itself moved to S02;
this slice is placement and its measurement.
Acceptance:
- **Six blocks to a page** (D25's number, absent from the first firming): a roster of nine allocates **two** signature pages, sized from the roster's **signing count** — an appended page for a party who never signs is a blank page in a signed document. Driven by a roster whose length and signing count straddle a page boundary.
- **Rendered and measured, and the measurement is DIFFERENTIAL** — the block is painted on opaque white and the readme body is black on white, so a scan of the composited page finds no readme ink under a block *because the block covered it*, satisfying the criterion with exactly the defect D25 exists to catch. Compare the ink extent of the block-free document against the placed rects, or key the fixture's blocks by colour.
- A **positive control** asserts each block's ink **is present**: a raster cannot distinguish "off the page" from "never drawn", and absence must not pass as compliance.
- Overlap driven at **N=4** and the page-box clip at **N=9** — different fixtures, because one nine-party fixture satisfying both is the shape C05's own note forbids elsewhere in this phase.
- `NextPlacement` reads the target page's **MediaBox** and refuses — named error, never a clamp — a rect that will not fit, driven on a page that is not A4. `stackPlacement` reads no geometry today and `bottom = 40.0` is a distance from the *coordinate origin*, not the page's lower edge; Nib's own split path produces tiles with an offset MediaBox.
- The block index comes from the contributing party's **roster position**, not from `len(Verify(pdf).Signers)`: an invisible contribution burns a slot, the count can go **down** when a signer's blob will not parse, and a foreign approval signature shifts the origin.
- The red proof shows the failure is **silent** — `valid`, `addedAfter=false` — beside the missing ink, or it will be read as a crash-class bug.

Tasks *(deepdive + grill 2026-08-27, `grills/2026-08-27-p07s06-placement.md`; verdict amended)*:
- **T01** — the ceremony placement door: page and within-page index from the party's roster
  **signing position**, both derived from `blocksPerPage`/`SignaturePagesFor` so D25's six gets no
  second copy. `NextPlacement(pdf)` is unchanged for the non-ceremony path — the two-party co-sign
  has no roster, so these are two rules and not two implementations of one. **The defect S02's own
  comment already named:** page order is `[source…][readme][ceremony][sig 1 … sig n]` and
  `NextPlacement` targets the LAST page, so at nine signers every block lands on sig page 2 indexed
  by the global signer count — signature page 1 receives nothing and blocks 6–8 climb off the box.
- **T02** — `stackPlacement` reads the target page's **MediaBox** and refuses by name, never
  clamps. Driven on a non-A4 page, which Nib's own split path produces.
- **T03** — **the finding this scope did not contain.** `NominalBlockRect` exists because *"the
  rule had TWO implementations"* — and it fixed one and left another. Two routes emit
  `cosignQuote`: `session.go:1418` uses `NominalBlockRect()`, and `cosign.go:243` runs a full
  `PageCount` + `sign.Verify` over the open document to publish a rect whose position the client
  never reads (`web/app.js:956` takes width and height and nothing else) — its own comment beside
  it says so. It is a **precondition** here, not a tidy: placement is about to need a roster and
  that site has none, nor should it.
- **T04** — the differential ink measurement with its positive control, N=4 and N=9 as separate
  fixtures.
- **T05** — the driver clause 1 still owes (below).
- **T06** — red proofs; `recorded` moves.

**Ledger — 6 met, 1 partly.** Six blocks to a page: **met at S02** (see below), driver added here.
Roster-position index: met, driven on a roster whose signing order and roster order differ, and
red-proved. Overlap at N=4 / clip at N=9 as separate fixtures: met. MediaBox refusal, named and
never a clamp: met, driven on **Nib's own offset tiles** — measured, `SplitPage(base,1,2,2,false)`
produces boxes like `(297.5,421)-(595,842)`, so the clause's premise is checked rather than
asserted. Positive control: met **structurally** — `pdfops.SignatureWidgets` reads the widget back
out of the signed bytes (page, rect, and whether an /AP exists at all), because a check on
placement arithmetic cannot tell "placed correctly" from "not placed at all"; probed red by
dropping the appearance.

**PARTLY: the RENDERED half of the differential measurement.** The clause wants a scan of the
composited page. There is no PDF rasteriser in Go (checked: `go.mod` has none), so it belongs at
tier 3 with pdf.js, where `stamplace.test.mjs` is the precedent — and tier 3 has no ceremony
fixture, which is machinery comparable in size to this slice. **What the fix changes about the
question:** blocks now live on dedicated signature pages, so "no readme ink under a block" is no
longer a scan — it is that no block is on that page at all, which is asserted structurally here.
What remains genuinely unrendered is a block drawn white-on-white or an /AP positioning content
outside its own BBox. Filed rather than implied.

**Clause 1's build half is ALREADY MET, by S02** — recorded so this slice does not credit itself
with another's work, the bookkeeping S04 and S05a both owed. `internal/ceremony/convene.go:166-171`
counts `p.Signs` and nothing else and hands that to `PrepareCeremonyDocument` (`:196`), so a
`signs:false` convener already allocates no page. What is owed is the **driver** the clause names:
a roster whose length and signing count straddle a page boundary.

#### P07.S07 — Every block names its party, its capacity and its ceremony *(D20 pin as amended, D27; C09, C15, C19)* — **SPLIT 2026-08-28 at its grill into S07a · S07b · S07c**
**Why it split, and both reasons were read at the line.** (1) The scope says of the party's name
"the data is already there and already committed; this is a read". True of the RECORD and false of
the roster the signing path is handed: `l3Roster` (`ceremonyid.go:531`) copies `Fingerprint` and
`Signs` out of the invitation and drops `Label` and `Capacity`. It is a read plus a widening.
(2) `rec.Intent` is not reachable the way the acceptance assumes. `l3Roster`'s own doc states the
S03 rule — the gate reads "the record the party verified at arm time" and **never** the one carried
by the document, because "a gate reading the document's own record answers its own question". The
invitation is that verified copy and it carries `Label` and `Capacity` (compared as a WHOLE `Party`
struct by `matchesRosterFields`, so both are checked against the signed record before consent) — but
it carries **no intent**. So the recital clause is a format change plus a new comparison, which is
also C17's unbuilt intent half, and it must not ride along with a widening that needs neither.
Scope: `coSignExchange` takes the intent from `c.Confirm` — typed per hop — while `Record.Intent` calls itself
"the ONLY home for it". `AppearanceLines` renders `Accepts: <label> [<short fp>]`: one neighbour, which is what
C09 says an N-party block may not say. And the panel found the axis nobody had checked — **every signer is
called "Nib User"**, a hardcoded constant at three sites, while `Party.Label` sits inside the signed commitment
with **no display reader anywhere**. A criterion about naming the *ceremony* is satisfied perfectly by nine
identical blocks reading `Signer: Nib User`.

#### P07.S07a — The party's own name and capacity reach the block *(D20 pin as amended, D27; C09 part, C19 part)* — **new, 2026-08-28, split out of S07** *(done 2026-08-28, v1.117.215 — 4 clauses, all met; 6 red proofs. Its diff-grill found a defect the slice itself INTRODUCED: `Label` and `Capacity` reach a block that `ctx.fillText` draws with no `maxWidth`, so each was a second and third silent clipping of the kind `IntentFitsBlock` exists to refuse — bounded at the convene door through one measurement, capacity being the one that matters since it is a claim about a party's AUTHORITY)*
Scope: the widening the split found. `p2p.RosterEntry` carries `Label` and `Capacity`; `l3Roster`
stops dropping them; and the two contribution doors read this party's own entry through **one** door
rather than each building its own attestation facts (ADR-009 — `StampCommitment` is already that
door and gains the party). No format change: the invitation already carries both fields and
`matchesRosterFields` already compares them against the signed record.
- T01 — `p2p.RosterEntry` carries `Label` and `Capacity`; `l3Roster` stops dropping them. *(done)*
- T02 — `StampCommitment` gains the party: label-or-fingerprint, capacity, and position in the signing order, at the one door both contribution sites already call. *(done — folded into the existing door rather than opened beside it, so `l3_test.go`'s routing scan covers both sites for free)*
- T03 — `AppearanceLines` grows a ceremony shape: `Party k of n` instead of `Accepts: <one neighbour>`, and a Capacity line only when there is one. *(done)*
- T04 — the block-line bound covers all three user-supplied strings, refused at convene. *(done — **not in the original plan**; the diff-grill found the slice introducing it)*
- T05 — guards at N=9 with nine distinct identities, plus the `l3Roster` arm no p2p fixture can reach. *(done — 6 red proofs)*
Acceptance:
- **Every block names the party as the record names them** — `Party.Label`, falling back to the fingerprint — and `Attestation.Signer` comes from the roster rather than the `"Nib User"` constant, at **both** contribution doors.
- A block renders **that party's own `capacity`** (C19's rendering half), and an empty capacity renders nothing — a ceremony that needs no capacities must not look misconfigured.
- Every visible block in a ceremony names the **ceremony** by roster position ("Party 6 of 9"), **not by a hex id**, and the two-party `Accepts: <one neighbour>` line is **unreachable** inside a ceremony (C09's block half).
- Driven at **N=9** in Go, over a real ceremony fixture with nine distinct identities, and the nine blocks are **distinct** — the defect this closes is nine identical blocks reading `Signer: Nib User`, which every assertion about one block passes.

#### P07.S07b — The record's intent becomes the recital *(D20 pin; C15, C19 part, advances C17)* — **new, 2026-08-28, split out of S07** *(done 2026-08-28, v1.117.218 — 3 clauses, all met; 5 red proofs. **Its deepdive found `checkArrival` had exactly ONE caller**: the dial side never reconciled its invitation against the document's record, so a party who INITIATES gated L3 and — since S07a — wrote labels and capacities onto its own signature block from an invitation nothing had checked. C17 at the door nobody looked at, closed here because this slice adds the recital to that same unchecked set. And the tier-6 harness's L3 clause was firing on TWO conditions at once — it paired ceremony 1's invitation with ceremony 2's document — so it read as driving out-of-turn while nothing could say which refusal it got)*
Scope: `Invitation` gains `Intent` (version bump) and `MatchesRecord` compares it against `r.Intent`
— C17's own words are that the reconciliation "must also cover `intent`, `expires` and `capacity`",
and this is the intent third. Only then can `p2p.Roster` carry an intent the receiving side is
entitled to trust, and only then can the discard rule below be more than a preference.
- T01 — `Invitation.Intent`, `InvitationVersion` 2 → 3, `NewInvitations` populates it. *(done)*
- T02 — `MatchesRecord` compares it against `r.Intent`, by name, quoting both sentences. *(done — C17's intent third)*
- T03 — `p2p.Roster.Intent`; `l3Roster` carries it; `StampCommitment` overwrites the caller's unconditionally. *(done)*
- T04 — `Contribute` refuses a commitment-bearing signature with no recital, so `defaultIntent` is unreachable by construction rather than by the roster happening to carry one. *(done)*
- T05 — `checkArrival` at the DIAL door too. *(done — **not in the plan**; the deepdive found the door had no check, and the guard's own comment said "the only caller")*
Acceptance:
- Every signature's signed `/Reason` carries the record's `Intent` **verbatim** as the recital, plus that party's own `capacity` (C19's signed half).
- The `Confirmer`-returned intent is **discarded** on the ceremony path, and `defaultIntent` is **unreachable** when a record is present — driven by a Confirmer returning something else and a Confirmer returning `""`.
- An invitation whose intent differs from the record's is **refused by name** before consent (C17's intent third).

#### P07.S07c — The surfaces: the consent gate, the panel, the verdict *(D27; C09 other half)* — **new, 2026-08-28, split out of S07** *(done 2026-08-28, v1.117.220 — 3 clauses, all met; 4 red proofs, **two of which failed on their first attempt and both failures were in my own tests**: the denominator fixture had nine rows and nine signatures, so the two counts it distinguishes were the same number and the patch changed nothing; and the consent-signer tests called the function directly, so deleting its only caller left them green. One clause PINNED — the discriminator is the document's SHAPE, not whether it carries a record)*
Scope, **re-derived at the split from the code**: on a nine-party document the first signer's
`AcceptedPeer` is `""` (`PredecessorOf`), so `augmentSigDetails` (`web/app.js:3787`) returns early on
its row and the panel renders **8 of 9**; `attested.length >= 2 && every(matched)` then prints *"each
party's signature attests to **the other's** key"* over a nine-party ceremony.
- T01 — `pendingView` carries every signature already on the arriving document; `Confirm` fills it. *(done)*
- T02 — the consent screen renders them, and renders the empty case as a sentence rather than as nothing. *(done)*
- T03 — the panel draws a row for every co-signing signature, including the first signer, whose row states C14's no-predecessor case. *(done — it had never had a surface)*
- T04 — the denominator is Go's signature count. *(done)*
- T05 — the mutual sentence is reserved for a mutual PAIR; a baton of any length gets a chain sentence. *(done — pinned; see below)*
Acceptance:
- The `Confirmer` is shown **every** party who has already signed, driven with a three-signature document.
- The panel renders **every** signature on a nine-party document, and its "of N" denominator equals what Go reports.
- ~~The two-party string *"each party's signature attests to the other's key"* is **unreachable** on a document carrying a roster token.~~ **Pinned 2026-08-28 at the slice, on a driven contradiction: the discriminator is the PARTY COUNT, not the presence of a record.** A two-party ceremony *is* mutual and that sentence is true of it — and `test/jsdom/oneproceeding.test.mjs` drives precisely that document (two parties naming different ceremonies) and asserts the positive survives, so a disagreement is *reported* rather than summarised away by deleting the good news. Suppressing on the roster token would have removed a true sentence to fix a false one. Re-pointed at C09's own text, which says nine: **the string is unreachable on a document with MORE THAN TWO signing parties**, which get a chain sentence instead — and "mutually" is itself false of a baton, since party 1 accepts nobody and nobody accepts party 9.

#### P07.S09 — D33's placement guard, and the protocol version *(D32, D33; C12, C13)* — **SPLIT 2026-08-28 at its grill into S09a · S09b · S09c**
**Why, and the second reason is a premise this slice got wrong.** (1) The placement guard is
self-contained, source-level and is what D33 says discharges it; the version work is a wire
question. (2) **The scope says of the ceremony protocol version "it does not exist today — the only
protocol identifier is the QUIC ALPN, which yields a TLS alert". It exists.** `alpn`/`alpn2`
(`internal/p2p/quic.go`) are a versioned, NEGOTIATED session protocol with a production reader
(`Channel.SpeaksNamedRefusals`), offered most-preferred-first at every config site so an older peer
negotiates down rather than failing — P07.S03a built exactly that, for exactly D32's reason. A TLS
alert happens only when the offer lists are DISJOINT, which needs a future build that has dropped
`nib/1` and `nib/2`. So what is owed is not a version — it is **a sentence when the handshake fails
that way**, and `alpn`'s own doc already argues the handshake failure is the right *mechanism*: *"an
ALPN mismatch is a clean, immediate handshake failure naming the protocol… The alternative —
negotiating and then failing somewhere inside the exchange — is the confusing one."* Building the
announce frame the acceptance describes would add an unauthenticated parse **ahead of the human
gate**, which the acceptance itself flags as needing its own bound — a new attack surface bought to
replace a mechanism that works. D32 is untouched: a skew still owes a sentence. **Pinned, not
struck** — the mechanism changes, the decision does not.
Scope, **re-derived 2026-08-23**: most of what the first firming asked for already ships. Record skew and
invitation skew both refuse with directional sentences and the invitation half is already red-proved; three of
D33's four numbers are already enforced at their external doors. Two things are genuinely missing, and one of
them is what D33 says discharges it.

#### P07.S09a — D33's law/tunable placement guard *(D33; C12 part)* — **new, 2026-08-28, split out of S09** *(done 2026-08-28, v1.117.222 — 2 clauses, both met; 3 red proofs, and **two of the first three mutations failed to compile rather than failing the guard**, which is the outcome `redproof.sh` distinguishes from a real red. Re-done as compiling mutations, and the second of them exposed a false positive in the guard itself: a substring match on `name + " = "` would have failed an ordinary typed declaration, so the check is parsed rather than grepped)*
Scope: the source-level check D33 names twice as what discharges it. Found at the grill and it is
not hypothetical: **`maxCandidatesPerSource = 8` sits INSIDE the tunable block (`clocks.go`) as a
bare literal, and its own comment says the eight is D33's law figure** — *"it is what each source is
already bounded to upstream: `maxLANCandidates` is 8 per browse and `ceremony.MaxCandidates` is 8
per record"*. So the law figure's value is reachable from the tunable block today, by hand-copy,
which is the exact condition D33's discharge names.
- T01 — `maxCandidatesPerSource` derived from the bounds it tracks rather than copying their literal. *(done)*
- T02 — the source-level guard, both arms: no law figure declared in the tunable block, and each law figure still declared with the structure it bounds. *(done — the second arm exists because deleting a figure outright passes the first)*
Acceptance:
- **The law/tunable placement guard** — a source-level check that fails if either law figure is reachable from the tunable block. D33 says **twice** that this, and *not* the drive-a-value-past-the-bound bullet, is what discharges it; no such guard exists. (P05.S04 T05 built part of this; point at it rather than rebuilding it.)
- The per-source cap is **derived** from the bounds it claims to track rather than copying their value, so raising one upstream cannot silently leave it behind.

#### P07.S09b — The punch budget: one per side, and its report gets a reader *(D33; C12 part)* — **new, 2026-08-28, split out of S09** *(done 2026-08-28, v1.117.224 — 3 clauses, all met; 3 red proofs. A LAW figure was being emitted at 2× silently, with two doc comments asserting the opposite of the code; and the test that shows the report has a reader is what moved the report above `diagnose`'s scope check, where it had been unreachable without a live DHT)*
Scope, from the grill, and the finding is a law figure exceeded rather than an unread counter.
`punchLoop`'s own doc says the armed side and the dialing side run one loop each *"sharing one
per-side budget"*. **They do not share it.** Each call site constructs `&punchBudget{}` inline
(`ceremonynet.go:441` and `:628`), the arm and the dial hold different `ceremonyID`s and different
sockets, and P05.S09's symmetric racing has one machine doing both for one hop — so a side emits
**6,000** packets against D33's 3,000. And `report()`, whose doc says it exists *"for D19/S11 to
surface"*, has **no production caller at all**: S11 shipped without wiring it, so the "reports" half
of D33's drop-and-report is written and unread.
- T01 — the budget keyed by ceremony id on the Server; both punch loops reach it through `ceremonyID.punchBudget`. *(done)*
- T02 — `punchReport` on D19's diagnosis, above the scope check. *(done — moved there by its own test)*
- T03 — guards: identity, exhaustion across both handles, per-ceremony isolation, the empty-id branch, and the registry under `-race`. *(done)*
Acceptance:
- One budget per **(hop, side)** — one machine, one ceremony — so the arm's punch loop and the dial's spend the same 3,000. Driven by running both for one ceremony id and asserting the total.
- The report has a **named production reader**: D19's diagnosis, which is where `report()`'s own doc says it was going.
- A hop that exhausts the budget **drops and reports** and never fails the ceremony (D33's own words), driven at the loop.

#### P07.S09c — The version sentence and skew at four surfaces *(D32; C13)* — **new, 2026-08-28, split out of S09** *(done 2026-08-28, v1.117.226–.229 — 2 clauses, both met; 3 red proofs, one of which was rejected on replay because the CLIENT FIXTURE modelled a state Go never produces. The attestation tag was the worst of the four skew surfaces because it did not fail loudly at all: one signature from a newer build made the whole document report *not one proceeding*, an accusation about the parties caused by an upgrade)*
- T01 — `ProtocolSkewError`, produced at one door for all three dial sites and lifted by the server before D19. *(done)*
- T02 — the attestation tag parsed at any version; a newer one publishes `TagVersion` and parses nothing else. *(done — reading the version is not trusting the payload)*
- T03 — the client reports the skew INSTEAD of the accusation, with both controls. *(done)*
Acceptance:
- ~~The **ceremony protocol version** exists and is announced before the first frame~~ **Pinned 2026-08-28 (see S09's split note): the version exists and is negotiated; what is owed is a SENTENCE when two builds' ALPN offer lists are disjoint, rather than a bare handshake error. No pre-gate frame is added.**
- ~~The fourth number's clause is re-worded to what is observable…~~ **Moved to S09b at the split and built there (v1.117.224) — it is a D33 clause, not a D32 one, and it turned out to be a law figure emitted at 2× rather than an unread counter.**
- Skew is driven for **four** surfaces, not three: record, invitation, protocol, **and the attestation tag**, whose skew today silently yields `AcceptedPeer=""`, `RosterHash=""` and a verdict of *not one proceeding* — the failure D32 forbids, arriving through the surface D32 excused.

#### P07.S10 — Docs, README, and the phase close *(C10, C11)* *(slice half done 2026-08-28, v1.117.230 — both acceptance bullets met; 3 red proofs. **C11 is the phase close's graduation pass and is NOT this slice's** — it is discharged at the close, not here. The live drive is what earned this slice: the first fixture used `PrepareCeremonyDocument`, which allocates the pages and embeds no record, so `nib verify` reported nothing at all and every unit test would have passed against it)*
- T01 — `ceremonyReportOf`: roster, obliged, signed, the one-proceeding verdict, and the recital, read from the DOCUMENT's own record (the verifier holds no invitation — the opposite of the L3 gate's rule, and for the opposite reason). *(done)*
- T02 — an unfinished ceremony exits 2. **Rung 2, taken and recorded**: the README ships `nib verify contract.pdf && echo "signature intact"`, and a nine-party deed four obliged parties never signed must not pass that. Same divergence `AddedAfter` was added to this condition to close. Help text moved with it. *(done)*
- T03 — README: the command table, the exit-status paragraph, and a worked example checked against the real binary's output rather than written from memory. *(done)*
Acceptance:
- Documentation and README updated in this phase (STANDARDS docs-parity).
- `nib verify` reports the ceremony: the roster, who was **obliged** to sign, who signed, and the one-proceeding verdict. It prints state, signer count and `AddedAfter` today, so a stranger told "check it with nib" gets `valid (9 signer(s))` — and the CLI is the surface a dispute actually uses.
- Every row of the seam inventory carries a disposition — and the criterion names **both files**: `instruments/ceremony.md` (233 inventory rows) **and the 60 ceremony rows in `instruments/P05.md`**, which that file itself flags as a hazard. **210 rows name a declared reader** and disposition mechanically; **4** are tagged *no standing reader* and need judgment; **23 have no reader column at all** — Category 2 seams and Category 3 gap-downs — and fit neither path, which is stated here rather than discovered. A per-row Disposition column replaces the range-addressed prose of prior closes: a range cannot distinguish a row individually considered from one swept in.

**(plan-review pin: this phase was one phase doing two jobs — 2026-08-18, adopted by Dan.)** P07 had
reached **30 exit criteria against a slice sketch**, having absorbed pins from five review passes,
and a phase with thirty criteria and no slices is a phase nobody can close: `/createcode` walks
slices. The criteria clustered cleanly in two, which is the tell. **The model stays here; lifecycle
and delivery become P08.** Phase numbers remain identifiers rather than an order, so nothing
renumbers — see the build order note above.

### P08 — More than two parties: **lifecycle and delivery** **(added 2026-08-18 — split out of P07 at the sixth plan-review)**
Goal: a ceremony survives interruption, ends in exactly one of D28's four states, delivers its
finished document to every party, and leaves nothing behind. P07 gives it a model that works for N
parties; this phase gives that model a life longer than one sitting.

*Why the split lands here and not elsewhere:* every criterion below is about **time and state** —
what persists, what resumes, what is delivered, what is removed. Every criterion left in P07 is
about **shape** — who may sign, what a signature attests to, where a block goes. A slice grill can
hold one of those in its head; it could not hold both.

Exit criteria:
- **C01.** **A hop interrupted after its signature exists re-delivers on resumption and never re-signs; the finished document carries exactly one block per signer. (D24.)** Driven by killing the process between the signature and the write — the case `internal/p2p/session.go:135` discards today.

  **(plan-review pin: the stated observable cannot fail, and the citation is stale — 2026-08-29.)** *Counting blocks* is produced identically by the behaviour this criterion forbids: `NextContributor` matches signature *i* against roster signer *i* positionally and `AdmitContribution` gates every ceremony contribution on it (`internal/p2p/l3.go:245-259`, `:374-381`), so no delivered artifact can carry two blocks from one identity — and if the cache is *lost*, the initiator simply re-runs the hop, a fresh signature is made, and the count is still right. **What discharges this specifically:** the mirror's stored contribution is snapshotted at the kill and the finished artifact's bytes for that party are byte-identical to it, *and* the user's `Confirm` gate is observed firing exactly once across the kill and restart. Block count survives as a weaker second clause, never as the only one. The citation `session.go:135` is now the `Confirmer` interface; the discard is `writeFrame` at `:324` and `rd.Store` at `:826`.
- **C02.** **The contribution write is atomic: a process killed mid-write leaves the previous state or the complete contribution, never a prefix. (gap #15.)** The re-delivery criterion cannot see this — it never writes twice.

  **(plan-review pin: atomicity has no cheap observation, and the door guard does not reach the new write — 2026-08-29.)** `WriteMirror` already routes both files through `atomicfile.WriteDurable` (`internal/ceremony/mirror.go:88`, `:101`), which is temp → `Sync` → rename → parent-dir `Sync`, so a torn prefix is unreachable at that door by construction and a planted-prefix fixture proves the *detector*, not the writer. `internal/server/atomicroute_test.go:38` scans `internal/server` only — one package of twelve, the shape `goroutines_test.go` was promoted to the repo root to fix. **What discharges this specifically:** the atomic-door guard is promoted to a repo-root `package nib` guard *before* the new write exists, and the red proof is a one-hunk patch swapping the new persist site to `os.WriteFile` — routing is the falsifiable property.
- **C03.** **A disk-full persist reports ~~"signed but not saved" and does not attempt delivery~~ "signed but not saved — do not close Nib" VERBATIM, delivers the signature anyway, and offers the signer somewhere else to put a copy. (gap #17; amended 2026-08-29 — Dan, option A, following D24's amendment.)**

  **(plan-review pin: the sentence is truncated, the channel does not exist, and there is no recovery half — 2026-08-29.)** D24's wording is *"signed but not saved — **do not close Nib**"*, and D24 argues that clause is the load-bearing half; a builder implementing the truncated string passes this criterion while the instruction that prevents the loss is gone. The message also has nowhere to be said: the persist runs on the p2p goroutine with no HTTP response, `sessionStatus` carries no failure field (`internal/server/session.go:1032-1043`), the accept loop discards the error into `_` (`:746`), and the front end has **no live region at all** — `toast` sets `textContent` on a bare div and clears it after 2500 ms (`web/app.js:9625-9634`), which a screen reader never announces. **What discharges this specifically:** an exact-string assertion on D24's full sentence; a sticky failure field on `sessionStatus` that outlives the session; and a driven recovery action that writes the in-memory contribution to a user-chosen path — a warning label over an unrescuable state is not a discharge.
- **C04.** **A ceremony resumed in a fresh process — with other documents opened first, so the id counter has advanced — acts on its own document and refuses a decoy holding the id it used to have. (D29 identity pin.)** The resumption bullet alone passes with a dangling id, because nothing in it opens a second document.

  **(plan-review pin: the decoy is unrepresentable across a process boundary — 2026-08-29.)** `docID` is `{Epoch, Seq}` where `Epoch` is a per-process nonce (`internal/server/server.go:55-78`), and ADR-004 already answers a stale id from a previous process with 409 — *"a mismatched per-process epoch: same fact"*. So a decoy "holding the id it used to have" cannot be built in a fresh process, and the bullet degrades to an assertion about `Seq`, which is not the id. **What discharges this specifically:** the decoy is restated as **a different document carrying the same ceremony id** — the substitution that does survive a restart — refused on the content match rather than on the id.
- **C05.** **A party who arms and waits through three earlier hops is still armed when the baton arrives. (D16 amendment.)**

  **(plan-review pin: the observable does not exist and the bound is the wrong one — 2026-08-29.)** `sessionStatus` exposes no deadline and no remaining window, so nothing can be asserted through a surface that exists; and a loopback ceremony finishes in seconds, so *"remaining > 0"* is true under the 30-day ceremony bound **and** under the 5-minute manual bound (`sessionAcceptTimeout`, `internal/server/session.go:59`) that the criterion exists to distinguish. The TCP arm path goes through `runSession`, which arms `sessionAcceptTimeout` with no ceremony in the arithmetic (`:693`) — so this is code on one of the two transports, not a no-op. D16's amendment also says the window *"becomes a ceremony-scoped decision rather than a constant"*, and `MaxCeremonyLife` is a constant. **What discharges this specifically:** `armedUntil` is exposed in `sessionStatus` and the assertion is on the **bound's shape** — derived from the record's `Expires` — not on the remainder.
- **C06.** **Each of D28's end states is driven at the protocol level, not only in the UI: a decline at hop 3 ends the ceremony, and the parties who already signed learn of it. (D28.)**

  **(plan-review pin: "each" is four states and the phase drove one; and the expired gate is on the wrong machine — 2026-08-29.)** *Expired* has exactly one enforcement site in the tree, `checkCeremonyDeadline`, whose only production caller is `handleSessionInitiate` (`internal/server/session.go:1550`) — **the dialing side**, which in a ceremony is the convener. The party being *asked to sign* gates on `checkArrival` → `ceremony.CheckRecord` → `Record.Verify`, and `Verify` deliberately does not refuse an expired ceremony and says so at the line (`internal/ceremony/record.go:456-462`). So whoever convenes owns the only clock check, and a signer can be collected into a proceeding D28 declares over. ADR-009's shape, on the rule whose whole point is a protocol-level end state. **What discharges each state specifically:** *declined* — a contribution at hop 4 refused by name after a hop-3 decline; *expired* — a contribution offered after `Expires` refused **by the signing party's own arrival gate, with the convener bypassed**, routed through the one `checkCeremonyDeadline` door; *abandoned* — the close-out of a ceremony whose convener never returns, which is C11's; *completed* — C08.

  **(settled 2026-08-30, Dan, `/pending 323` — the termination object is convener-signed and
  transmitted, and D28 is not struck.)** D28's parked question was *may an end state be recorded,
  and by whom?*, and its finding was that every `Record` field sits inside the convener-signed
  commitment so *"mark it declined"* has no home. That finding stands and needs no amendment,
  because **the end state was never a `Record` field** — S04 already posits a separate object.

  **It must be attested, and that is what settles the shape.** C06 requires the parties who already
  signed to *learn of it*. Learning is a delivery, and an unattested claim about a proceeding
  arriving over the wire and believed is a denial-of-service on a live ceremony: a hostile roster
  member tells the hop-1 and hop-2 signers the proceeding is dead and they stop. That is the class
  L1 and D32 exist against. **The convener signs it** — the same authority that signed the record —
  so a decline is reported *to* the convener, who ends the proceeding and signs that it ended.

  **Two artifacts, not one.** The local receipt (C11's, above) records what a machine believes; this
  signed object is the evidence that justifies believing it. On the convener's own machine they
  coincide.

  **The asymmetry, written down rather than glossed: only two of the four states can be attested.**
  *Declined* and *completed* have a convener to sign them. *Expired* and *abandoned* do not —
  abandoned means the convener never came back — so for those the receipt is all there is, derived
  from the record's own `Expires` plus S06's grace, which every machine evaluates independently and
  identically. A reader who assumes all four are attested will build the wrong verifier.
- **C07.** **An identity that no longer matches the roster ends the ceremony with a named message and never pairs on the new key. (D28 — the L1 guard covers it.)**

  **(plan-review pin: name the party, not the key — 2026-08-29.)** P06's first exit criterion forbids a hex fingerprint on the primary flow, and the natural way to write a mismatch message is to print both fingerprints. The tree already has the discipline — *"that peer isn't pinned — pin their fingerprint first"* names the word and never the value (`internal/server/session.go:1122`). **What discharges this specifically:** the message names the party by **roster label and position**, contains no hex, and the vault's pin set is asserted to hold no entry for the new key — the error string alone is not the observation.
- **C08.** **The finished document reaches every party, including those whose hop completed hours earlier. (D22 delivery round.)**
- **C09.** **A four-party ceremony's delivery round reaches every party, and their invitation pins are absent afterwards — in that order. (D29 lifecycle pin.)** Neither the delivery bullet nor the pin-scoping bullet can produce this alone; the defect is that both are satisfiable while delivery fails.

  **(plan-review pin: the vault's secrets are not in this observation, and shipped code already claims they are removed — 2026-08-29.)** `PruneCeremonySecrets` has exactly one production caller and it is `unconvene`, the convene **rollback** (`internal/server/convene.go:268`). So every ceremony that completes, declines, expires or is abandoned leaves N−1 full-strength invitation secrets in the convener's vault permanently — the value D29 calls the design's first genuinely secret one, and the key to every hop's candidate records. Meanwhile `handleCeremonyInvites` already answers 410 with *"A ceremony's secrets are removed when it ends"* (`internal/server/convene.go:371-373`) — a sentence no code implements. **What discharges this specifically:** the ordered observation also asserts the vault holds **no `CeremonySecret`** for that ceremony afterwards.
- **C10.** **A delivery round re-run after a mid-round failure leaves exactly one file per party. (D22 pin.)** "The finished document reaches every party" is satisfied completely by a round that delivers twice.

  **(plan-review pin: the acknowledgement is emitted before anything is written — 2026-08-29.)** `ReceiveDocument` sends `ackOK` (`internal/p2p/session.go:640`) and the server persists **afterwards** via `saveReceived`, whose own comment says a write failure *"simply reports nothing"* (`internal/server/session.go:911-925`). Compose that with "a no-op once that party's acknowledgement is recorded" and a party whose disk write fails is recorded as delivered, is never retried, and is never told. **What discharges this specifically:** an injected write failure at party 3 of 4, after which the party is **not** recorded as acknowledged and the re-run delivers to that party and to no other.
- **C11.** **A ceremony directory is gone after the ceremony has ended and its document has been delivered or saved. (D29.)**

  **(plan-review pin: on three of D28's four end states the prune destroys the only copy of a real signature — 2026-08-29. Five seats found this independently.)** For a non-convener party the mirror is the **sole** durable copy of the document carrying their own signature: `openArrival` calls `mirrorHop` and `addDoc` — a mirror file and an in-memory tab — and `saveReceived` is not on the co-sign path at all, running only under `mode == sessionModeReceive` (`internal/server/session.go:878`, `:904-910`). `RemoveMirror` is an unconditional `os.RemoveAll` (`internal/ceremony/mirror.go:178-184`). *Declined*, *expired* and *abandoned* never reach a delivery round, so nothing is ever delivered **or saved** — and D28 says in terms that those parties *"keep their partial document"* and that the partially-signed document *"remains a valid document"*. **What discharges this specifically:** the prune is a **move, not a delete** — this machine's own signed contribution is written outside `~/nib/ceremonies/` first — and the observation is the file's presence at a named path *after* the prune has run, never the directory's absence alone.

  **(settled 2026-08-30, Dan, `/pending 325` — the end state gets a local receipt, and only S06's
  prune reopens.)** Three decisions composed to make an end state unrecordable anywhere: `Embed`
  refuses an already-signed document, so it cannot go in the PDF; D28's pin puts every `Record`
  field inside the convener-signed commitment, so it cannot go there; and this prune `RemoveAll`s
  whatever S04 wrote beside `record.json`. S09's *"names an end state only where a local mirror
  exists"* therefore evaluated to **never**, on every machine, for all four states.

  **Of the three, only this one reopens.** The first is a PDF-format fact. The second would put a
  forgeable field on the one artifact whose value is that every field is attested. This prune is
  unbuilt and is *already* being amended to a move rather than a delete, so what changes is one
  clause of what survives: **the contribution plus a small end-state receipt**, written outside
  `~/nib/ceremonies/`. It is **local bookkeeping and not an attestation** — unsigned, and it must
  not present itself as proof. The proof of what happened is the document, which D28 already
  establishes: a declined party *"keeps their partial document"* and it *"remains a valid
  document"*. The receipt is navigation.
- **C12.** **A ceremony whose mirror directory has been deleted by hand degrades that panel entry and leaves every other ceremony and open document working. (gap #19.)**

  **(plan-review pin: absence is the easy case; the four that brick the ceremony are the other ones — 2026-08-29.)** `ReadMirror` returns on any `Verify` failure before it ever exposes `Expires` (`internal/ceremony/mirror.go:130-137`), and `Verify`'s first check is exact version equality (`internal/ceremony/record.go:464`) against a `FormatVersion` already at 4 and moved three times. So a Nib update mid-ceremony makes every live ceremony on the machine unlistable, unresumable **and unprunable**, reported in the vocabulary of forgery — while the vault does this properly next door, refusing a newer payload with a plain sentence naming both versions (`internal/vault/vault.go:239-248`). **What discharges this specifically:** four degraded outcomes with four different sentences — absent, unparseable, **version-skewed** (named as a skew per D32, not as a verification failure), and unverifiable — and a version-skewed mirror is still prunable.

  **(settled 2026-08-30, Dan, `/pending 319` — a FIFTH class, and the route moves.)** The four
  classes were built behind `s.requireUnlocked(s.handleCeremonies)`, so C12 was met only on an
  unlocked machine — and D29 forbids exactly that in terms: *"The panel renders while the vault is
  locked… A resumption screen that demands a password to tell you whose turn it is has
  misunderstood what it is for."* **The lock protects nothing here**, which is what settles it: the
  listing is derived from the mirror, which D29 itself establishes as ordinary unsealed files under
  the user's home, so anyone with filesystem access already reads every label in it. `/api/ceremonies`
  moves off `requireUnlocked`; everything mirror-derived renders while locked; any field that
  genuinely needs the vault says *"unlock to see"* in place rather than 401-ing the page. That is
  also D34's *"the third state… it must say which"*, which the four classes did not carry. **D29 is
  not amended** — the implementation was wrong, not the decision.
- **C13.** **Convening a second ceremony on a document already under a live one is refused, server-side and by name. (gap #28.)** The mutation criterion cannot see it, because convening is not a mutation.

  **(plan-review pin: the covered direction is the one that already works — 2026-08-29.)** `Convene` refuses on a record being present in the bytes it is handed (`internal/ceremony/convene.go:159-166`), so convening again on the *convened output* is refused. Convening a second ceremony after **re-opening the original file** is not: two live ceremonies, two rosters, one underlying document, which is the case C13's wording actually describes. **What discharges this specifically:** the refusal is driven from the **pre-convene bytes** and keys on document identity against the live mirrors, not on a record being present in the bytes at hand.
- **C14.** **An invitation re-issued to one party mid-ceremony completes, with every other party's state untouched. (gap #24.)** The convene criteria only ever issue once. *(gap #24 is **D21's** pin, not D29's.)*
- **C15.** **Every criterion in this phase is driven by the multi-instance harness (D26), not by hand.**

  **(plan-review pin: this is P07's C06 verbatim, and P07 closed it ⚠ PARTLY for the reason that will recur — 2026-08-29.)** P07's ledger records *"Read literally the criterion is not met"*, because criteria that are not live multi-party ceremonies cannot be tier-4 driven. P08 restates it in **stronger** terms with no mechanism to make the outcome different: C02 (a planted prefix), C03 (ENOSPC), C04's absence guard and C11's grace are all structurally outside tier 4 today. The tree reads twelve `NIB_*` variables and not one is a fault knob, and `build/redproof.sh:14-25` is the standing doctrine against adding one — *"a switch whose whole purpose is to break the program is the same gun with a better excuse, and it would ship in the binary users run."* **What discharges this specifically:** C15 carries its own exception list; every criterion names the tier that drives it; a criterion the harness cannot express states its stimulus and its reason **in the acceptance bullet**; and the two capabilities that are pure harness — a `restart <instance>` verb and redproof-shaped fault patches — are built rather than assumed.
- **C16.** Documentation and README updated in the same phase (STANDARDS docs-parity).
- **C17.** **`<project-memory>/instruments/ceremony.md` holds a section named for every P08 slice, compared against this phase's build order at the close, and a missing section FAILS. (added 2026-08-29, plan-review.)** P07 carried C11 for the inventory and its close still found seventeen slices with no rows at all, because *an absent row is invisible to a pass over rows* — and `/pending 297`'s own conclusion is that *"a slice that closes without writing its rows should be the thing that fails, and nothing checks it."* A preamble sentence is not a criterion and a phase-close ledger can cite nothing from it, which is why this is C17 rather than a paragraph.

  **(enforcement point moved to the SLICE, 2026-08-30, `/pending 322`.)** C17 was written on
  2026-08-29 and by the end of that day four of the five slices that shipped had breached it, with
  nothing failing — because its only enforcement point was S09, the last slice. A criterion whose
  only enforcement point is the phase close is not a criterion during the phase; it is a debt that
  discovers itself when it is most expensive to pay. The gate is now
  `~/.claude/tools/inventorycheck --phase=<PHASE> <PLAN.md> <project-memory>/instruments/<phase>.md`,
  run by `/createcode`'s **slice close** and blocking it on a non-zero exit. It fails on a shipped
  slice with no section AND on a section whose status marker contradicts the plan's — the second
  arm found P08.S01's inventory still saying *in progress* against a plan that says *done*.
  It is a script rather than a test because the inventory lives in the project memory directory and
  the plan lives in the repo, and no repo-side guard can read across that boundary. **The five
  slices that shipped before the gate are DECLARED** under the inventory's `## Known gaps`, where
  they report as warnings rather than silently passing; their backfill is `/pending 332`.

**The `C01…C17` labels are editorial, added 2026-08-29 at the phase-open (`/createcode`), and no
criterion's text is changed by them.** P07's criteria carry labels and P08's did not, so a ledger
over this phase had nothing to cite but a paraphrase — which is the drift the acceptance ledger
exists to catch. The labels are the citation. **C17 is the one addition**, made at the plan-review
below rather than editorially.

Reviewed 2026-08-29 (**plan-review pass, the seventh — the P08 phase-open**, run by `/createcode`'s
own trigger because P08 is security-, migration- and egress-heavy. Structural gate **passed**;
sixteen seats — the seven-seat `plan` set, the `crypto`/core and `verification` pack lenses, the PDF
document-format SME, and five of the six conditional seats this repo declares. The Go SME was not
seated: its declared remit is `code only`. **The pass found that P08's criteria, written 2026-08-18
against a design, rest on three premises the tree has since measured false**, and that the seven
slices firmed hours earlier had inherited all three. Pins written at C01–C07 and C09–C15, D22, D24,
D28 and D29; **C17 added**; the slice list re-firmed from seven slices to nine. Four items are
decision-level and are **parked for Dan**, listed in the run's closing batch. Report:
`<project-memory>/plan-reviews/2026-08-29.md`.)

**The through-line, stated once because every critical is an instance of it.** P08's sixteen criteria
were written on 2026-08-18 against the design; its slices were firmed on 2026-08-29 against a tree
that has since measured three of the design's load-bearing premises false.

1. **The ceremony has no mutable local state, and four slices each wrote as if another had built
   one.** Every field of `Record` is inside the convener-signed preimage and `ReadMirror` re-verifies
   the signature on every read (`internal/ceremony/record.go:154-197`, `:463-497`;
   `internal/ceremony/mirror.go:130`), so "mark the record declined" is unbuildable in both
   directions: outside the preimage it is an unauthenticated field on a root of trust that
   `Decode`→`Encode` silently drops, and inside it, it changes `rosterHash` and invalidates every
   `[NibRoster:…]` commitment already collected. The mirror is exactly two files and the plan named
   no third.
2. **A party who is not the convener persists nothing at all.** `AddCeremonySecret` has one
   production caller and it is `handleCeremonyConvene`; `/api/ceremony/accept` stores a pin and
   throws the invitation away; and the vault states it as a design decision at the line — *"A
   recipient's invitation travels in the arm request and is never persisted"*
   (`internal/vault/vault.go:171-181`). Nothing re-arms at startup. So C01, C04, C08 and C09 are
   unreachable on every machine but the convener's — **and the harness hides it**, because
   `pairrepro.sh` keeps each invitation in a shell variable and would re-arm a "restarted" party
   from the harness's own copy. That is ADR-010's *configured past the disagreement* shape, in the
   one criterion whose subject is surviving a restart.
3. **`docHash` stopped being a document identity at the first signature, and P07 measured it and
   wrote it down in three places** — `internal/ceremony/embed.go:186-206` (*"a receiving party can
   never pass `CheckDocument`, at any hop — measured, not argued"*), `mirror.go:151-163`, and
   `CheckRecord`'s existence. D29's identity pin adopting `(ceremony id, docHash)` predates that
   measurement by five days, and the firmed S02 copied the pin without it.

A fourth, which is not a stale premise but a live defect the panel surfaced while reading for one:
**the party being asked to sign never checks the ceremony's deadline.** See C06's pin.

Slices *(re-firmed 2026-08-29 at the plan-review above — nine, from the seven firmed hours earlier
at the phase-open. The seven are not listed separately: every one of them survives inside these
nine, and the changes are recorded as the pins at C01–C15 and in the hand-off.)*

**Two things are built before anything that resumes, because six criteria rest on them and no slice
owned either:** a machine-local state file, and an invitee that persists its own ceremony. They are
S01. **A standing constraint on all nine, not a slice: C15 as pinned** — every slice names the tier
that drives each of its bullets, and a bullet tier 4 cannot express says so and says why. **And
C17** — each slice writes its seam-inventory section as part of the slice, and the close fails on a
missing one.

#### P08.S01 — A ceremony survives a restart on every machine *(D21, D24, D29; precondition for C01, C04, C08, C09)* *(done 2026-08-29, v1.117.240 — **re-scoped at its grill**, and the title changed with it: the mirror is NOT where an invitee's state goes)*
Scope: the three absences the plan-review found under everything else. **(a)** A third mirror file,
`state.json` — unsigned, machine-local, carrying its own layout version — because `Record` cannot
hold mutable state and `WriteMirror`'s document-then-record ordering is a *first-write* argument
that does not survive a third file. **(b)** `/api/ceremony/accept` persists the invitee's own ceremony — **in the vault, and NOT as a
mirror**, which is an amendment this slice's own deepdive forced before its grill (2026-08-29). The
first firming said "writes the mirror"; that is not buildable. `WriteMirror` writes `r.Encode()` and
`ReadMirror` calls `r.Verify(now)`, so a mirror needs `ConvenerSig` and `ConvenerCert` — and the
`Invitation` type carries neither, nor `DocHash`, nor `Expires`. **An invitee holds no Record until
the document reaches its hop** (`checkArrival` → `CheckRecord`, `internal/server/session.go:424` and
`:1600`), so there is nothing to write. What it *can* persist is the invitation itself, and the
deepdive established that is sufficient: `ceremonyFor` needs only the invitation text, this
machine's cert/key and the peer fingerprint (`internal/server/ceremonyid.go:657-684`). The secret
travels inside it, so the whole thing goes where D29 puts key material — a `contentsVersion` 2→3
bump, whose skew direction the vault already handles correctly (`internal/vault/vault.go:242-249`).
**It ships with its prune in the same change**: `PruneCeremonySecrets` has only the convene-rollback
caller today, so the write alone would leave permanent key material on every invitee's machine —
D29's own harm, delivered by its own fix.
**(c)** `ReadMirror`'s version skew lifted out of its generic *"does not verify"* wrap into a named
D32 skew outcome, so a Nib update mid-ceremony does not make every live ceremony unloadable *and*
unprunable while telling the user the vocabulary of forgery.
**The grill dropped two of the four parts and added one.** *`state.json` went* — nothing in this
slice writes mutable state, and the store's home (vault or a third mirror file) is a decision that
belongs to the first slice that actually needs it, S04's end states or S05's delivery acks, made
against a real requirement rather than speculatively. Position and next action are derivable from
`record.json` plus the document's signature count, so S03's listing does not need it either.
*The mirror's version-skew classification went to S03*, because a named error has **no reader**
until the loader exists: `handleCeremonyInvites` collapses every `ReadMirror` error into one 404.
*And the re-issue repair arrived*, because it is not adjacent work — it is the **convener's half of
this slice's own criterion**. A convener's disk-based re-arm path is that route, and it was emitting
invitations every recipient refuses.
Acceptance:
- **A party who accepts an invitation can rejoin it after a restart** — the invitation is stored, in the **vault** and never in `~/nib/ceremonies/`, and an arm carrying only a ceremony id succeeds. *Met.* `TestAcceptPersistsTheInvitationSoAReArmNeedsNoPaste`, whose stimulus is an arm naming a ceremony this machine never accepted, so the pass cannot come from a build that ignores the field.
- **The re-arm request carries NO invitation**, asserted on the marshalled body and not on the arm succeeding — which is true either way. *Met*, in Go and again at tier 4 through real binaries across a real process restart.
- **Naming two sources for one invitation is refused by name**, both individually valid so a silent pick would have armed. *Met.*
- **A re-issued invitation still matches the record it was built from**, run through `MatchesRecord` itself rather than by diffing fields. *Met*, and it was RED before this slice: the route omitted `Intent`.
- **The write ships with its prune**, wired at the two sites that already prune ceremony material. *Met* at the store; **`not exercised` for a ceremony that COMPLETES**, which has no close-out caller in this tree for the convener's secrets either — recorded rather than closed over, and it is P08.S06's.
- **`pairrepro.sh` gains `restart <instance>`** — pure harness, no product flag, per `redproof.sh`'s standing doctrine — and a run asserts the restarted instance re-armed from its own disk. *Met.* Tier 4 `-n 4` PASS, both transports.
- Tier: 4 and 6 for the accept/re-arm and restart bullets; tier 1 for the store's own rules; stated per C15.

#### P08.S02 — The contribution reaches disk before it reaches the wire *(D24 as amended, gaps #15 and #17; C01, C02, C03)* *(**partly done** 2026-08-29, v1.117.255 — C02 and C03 met, C01's mechanism built; only C01's kill-at-the-instant DRIVER remains, and it needs a fault seam this repo has deliberately refused in the product)*
Scope: what the first firming had as S01, corrected on five counts the panel established. The
in-memory half is already ordered correctly — `rd.Store` sits inside `coSignExchange` before
`writeFrame` (`internal/p2p/session.go:826`, `:324`) — and the durable half runs *after*, from
`openArrival`, best-effort, logging into a stderr no user reads. This slice moves it, and: widens
`ReDeliverer` to `Store(inbound, final) error` / `Cached(inbound) ([]byte, error)`, without which C03
is unbuildable and a read failure is indistinguishable from a cache miss — which falls through to a
**second signature from the same identity**, the one thing D24 forbids; keys the durable side on the
inbound hash beside the bytes, since the mirror holds one `document.pdf` and the cache's own comment
says keying on the hop alone hands a reconnect the wrong signature; makes `hasSigned` read through
too, or a resumed process takes the pre-signing branch with a zero `postSignDeadline` and returns
immediately; writes a `document.sha256` sidecar that `ReadMirror` checks **unconditionally**, since
its `DocHash` check is switched off from the first signature onward — which is every byte this slice
persists; and states `mirrorHop`'s fate, because a rule at two doors with opposite failure semantics
is ADR-009's shape.
Acceptance:
- **The atomic-door guard is promoted to a repo-root `package nib` guard before this slice writes a byte** — `internal/server/atomicroute_test.go` scans one package of twelve, and the new write is in `internal/p2p` or `internal/ceremony`. Red-proved by a one-hunk patch swapping the new site to `os.WriteFile`.
- **A hop killed after its signature and before the frame re-delivers the SAME BYTES.** The **mechanism is built and driven in-process**: `Cached` reads through to the mirror, returns the stored contribution only when it is a byte-prefix extension of the inbound actually offered, and misses on a different document or on one with nothing appended. **The kill-at-the-instant DRIVER is `not exercised`**, and what it needs is named rather than implied: a way to stop the process between `Store` and `writeFrame`. `pairrepro.sh` can now restart an instance (S01) but cannot kill one at an instruction, and `build/redproof.sh:14-25` refuses the obvious mechanism in as many words — *"a switch whose whole purpose is to break the program is the same gun with a better excuse, and it would ship in the binary users run"*. **What would settle it:** a redproof-shaped patch that inserts the stop between those two statements in a throwaway export, driven at tier 4 with S01's restart verb. That is a slice's worth of harness, not a task.
- **A correction to the deepdive that produced this slice, recorded because it is load-bearing:** it proposed `hasSigned` read through to disk as well. It must not. The pre-signing branch is the one that PUBLISHES CANDIDATES, and a restarted party is exactly the party the initiator can no longer find; the post-signing branch only accepts, which is sound within one process and false across a restart.
- **A truncated `document.pdf` is refused by the sidecar check whether or not it is signed.** *Met.* The fixture is **signed**, with its own setup assertion saying so — an unsigned one would have exercised the `DocHash` comparison instead and proved nothing about the window, which is every mirror from hop 2 onward. A missing sidecar is tolerated and the reason is stated: every mirror written before this slice has none, and it is a damage detector rather than an access control.
- **`/pending 314` closed by the same change that made it live.** The D29 secret-absence guard read `record.json` alone, on a document-less mirror — `mirror.go`'s own header calls it "an absence check over this directory" and it was an absence check over one name. Adding a third file is what made that actionable. It now walks the directory and asserts it read at least three files, so it cannot silently narrow again.
- **A persist that fails for want of space reports D24's full sentence — "signed but not saved — do not close Nib" — VERBATIM, and the frame is written anyway** (D24 as amended 2026-08-29, Dan's option A). The exact-string match is the assertion: C03 quoted only the first half, and the "do not close Nib" clause is the one that prevents the loss, so a build implementing the truncated string would pass a looser check while dropping the part that matters.
- **Two things this slice no longer needs, and the amendment is why.** *No new wire refusal code* — nothing is refused on the wire, so `refusalCode`/`errorForCode`/`refusalAck` are untouched and the ALPN-v1 question does not arise. (`/pending 315`, the enumeration's missing derived guard, is still real and is still owed; it is simply no longer this slice's.) *And no cache surgery* — the withhold-and-drop-the-cache shape, which would have made a reconnect re-sign, is dead.
- **The persist sits between `coSignExchange` and the deadline reset, and outside `c.mu`.** Before the reset, so the frame still gets a full fresh `postConsentDeadline`; outside the mutex because `diagnose` takes the same one and the UI polls it every 800 ms, so an fsync inside it stalls `/api/session/status` and `/api/session/disarm`. **The peer's budget is the one that binds and it is NOT widened here** — `remoteDecisionDeadline` is `2*PeerGateWindow + postConsentDeadline`, so two full human gates leave exactly two minutes for the co-signature *and* a write-back of up to `maxFrame`. The persist spends from that. Widening it cascades into `Convene`'s reservation and C20, which is a slice rather than a task; the term is **stated in `postConsentDeadline`'s composition** and the limit is recorded rather than silently absorbed.
- Tier: the kill/restart bullets are tier 4 with S01's verb; the ENOSPC and torn-write bullets are redproof-shaped patches at tier 1 and say so (C15).

#### P08.S03 — A ceremony is loaded, not remembered *(D24, D29 identity pin, D34 gap #19; C04, C12)* *(**partly done** 2026-08-29, v1.117.243 — C12 met, the listing and its four degradation classes; C04's resume/decoy half NOT built and named below)*
Scope: give the mirror a reader — and match on something that exists. The resume key is the ceremony
id read out of the document's **own convener-signed record** (`CheckRecord` / `ProceedingOf`), with
`docHash` retained only in the unsigned window where it is answerable; a decoy then needs a forged
`ConvenerSig` rather than a colliding digest. The listing answers from `record.json` and
`state.json` **only** — measured at this review: `ReadMirror` costs 10 ms at 100 pages, 69 ms at 500
and 195 ms at 1000, superlinear, on text-only fixtures, so a request-path enumerate of fifty stored
ceremonies is seconds. The panel is P06's and this slice does not build it; what P08 owes is the
answer it will render.
**Two amendments its grill made, both before any code.** *The lock is gone.* The plan asked for an
exclusive lock on `~/nib/ceremonies/`; there is no locking anywhere in this tree and adding some
would be a **second** cross-process policy contradicting the one that exists — `cmd/nib/main.go`
decides deliberately that a launch losing the instance race *"carries on and serves"*, because *"a
launch that loses twice is better off running than refusing to start"*. The signal is already
maintained: `instanceToken` is empty exactly when this process is not the recorded instance. One
mechanism, already tested, and no new file in a directory whose file set other checks assume.
*And an acceptance bullet cracked:* the listing cannot both answer from `record.json` alone — which
the measured cost forces — and report a **next action**, because that needs the signature count and
the count is in the document. The listing answers what the record knows; the count comes when one
ceremony is opened, which is what a panel does anyway.
Acceptance:
- **One unloadable ceremony never costs another** — driven at two ceremonies with one broken, asserting both are still NAMED and the intact one still loads. *Met* (C12).
- **Four degraded outcomes with four sentences** — absent, unparseable, **version-skewed**, unverifiable. *Met.* The skew case is the one that mattered: a Nib update moves `FormatVersion`, `Verify` refuses, and every live ceremony would otherwise be reported in the vocabulary of forgery. It now names the cause and the remedy and stays prunable.
- **The listing does not open `document.pdf`** — driven by planting bytes that are not a PDF at all, so anything that parsed them would fail. *Met.*
- **A Nib that is not this machine's recorded instance lists but says it must not act.** *Met*, and the fixture is that case by construction.
- **A ceremony resumed in a fresh process refuses a decoy document carrying the same id**, matched on the ceremony id from the document's own convener-signed record and never on a recomputed `docHash`. **NOT BUILT** — C04 needs a resume route and a second open document, and it is scoped here rather than silently dropped.
- **Nothing persists a document id**, as a type rule over `ceremony.Record`'s field set. **NOT BUILT**, with C04.
- **A resumed ceremony whose mirror bytes differ from the open tab's reports both and resolves neither** (D29's divergence clause). **NOT BUILT**, with C04.
- Tier: 1 for the classes and the listing's own rules; the resume half is tier 4's when it exists (C15).

#### P08.S04 — Between hops: the arm, and the end states as protocol facts *(D16 amendment, D28; C05, C06, C07)* *(**partly done** 2026-08-29, v1.117.248 — C05's transport parity and C07 both met; C06 PARKED on a decision)*
Scope: three properties of the gap between hops, and one of them is a live defect rather than a gap.
C05 is **not** a no-code slice: the QUIC arm waits `MaxCeremonyLife` but the TCP arm goes through
`runSession` on `sessionAcceptTimeout` — five minutes, no ceremony in the arithmetic — and no surface
exposes a window to assert. The expired end state has one enforcement door and it is on the
**convener's** side, so a signer can be collected into a proceeding that is over. And the decline
needs an authenticator: a plain boolean on a local disk is assertable by any local write and
strippable by the decliner.
Acceptance:
- **`armedUntil` is exposed in `sessionStatus` and the ceremony arm's bound is asserted on BOTH transports** — the bound, not the remainder, because a loopback ceremony finishes in seconds and a remainder assertion passes under either. *Met, and it found the defect:* `runCeremonyReceive` has bounded a ceremony arm by `MaxCeremonyLife` since P05.S09b and `runSession` — every TCP arm, ceremony or not — kept the five-minute manual bound, so a party third in a roster was disarmed while the earlier hops ran. Third time this phase that a rule reached one of the two arm paths. **`Expires`-shaped is NOT met and cannot be here**: an arm holds an invitation and the invitation carries no deadline, which is `/pending 247` — so the bound is the same ceiling the other path uses and the refinement waits on that item.
- **A contribution offered after `Expires` is refused by the SIGNING party's own arrival gate, with the convener bypassed**, routed through the single `checkCeremonyDeadline` door (ADR-009), and the guard asserts the routing rather than the sentence each site prints.
- **A decline is a separate signed termination object** — the decliner signs `(domain tag, ceremony id, rosterHash, state, time)` through the existing preimage builder — written beside `record.json`, verified on read as the record is, and carried by S05's round. A decline at hop 3 then refuses hop 4 by name and is *checkable* rather than hearsay.
- **An identity the roster does not name is refused by a message that says so**, distinct from the "you two are not adjacent" refusal it shared a sentence with, and carrying no fingerprint. *Met.* **Whose check this is turned out to be the finding:** from the CONVENER's side it is unfixable and should be — a re-enrolled party and a stranger present the same unpinned certificate, refused at the handshake, which is L1 working and which D28 itself says accepting would be "the substitution L1 exists to forbid, arriving through the front door". The half that can be answered is the party's own, offline, before anything is dialled: their fingerprint is no longer in the roster of the invitation they hold. The two refusals call for different actions — wait your turn, versus the ceremony must be convened again — and were one sentence.
- **"Never pairs on the new key" holds structurally rather than by a check**, and is recorded as that: nothing pins from anywhere but the roster (`pinCeremonyRoster`), so a key the roster does not name has no door to be pinned through. *Met by construction, stated rather than asserted.*
- Tier: 4 for the arm bound and the decline; tier 1 plus a redproof row for the arrival-gate refusal, stated (C15).

#### P08.S04a — The arrival gate refuses a proceeding that has ended *(D28, C06's expiry half; split from S04 2026-08-30 at its grill)* *(done 2026-08-30, v1.117.283 — budget ZERO after the arithmetic was re-derived at the line; wire codes 13 and 14, the second closing an older bare-EOF that predates this slice)*
Scope: S04 left two bullets unbuilt and they are **two slices, cut on risk class** — this one is
reversible and touches no format; S04b writes a one-way on-disk artifact. They share only
`checkArrival` as a consumption site, and S04b is inert until S05 delivers it (the strongest
argument for the cut).

**The finding that shaped it: nothing on a SIGNING party's path refuses an expired ceremony.**
`internal/ceremony/record.go:553` is the only `Expires`-vs-`now` comparison a signer reaches, and it
is a **future** ceiling (`Expires.After(now + MaxCeremonyLife)`) — it refuses a deadline too far
ahead and never one in the past. The one deadline refusal that exists, `checkCeremonyDeadline`, has
a single production caller and it is on the **convener's** side. *(Scoped precisely at the commit
gate: `internal/ceremony/convene.go:213` also compares a ceremony `Expires` to `now`, but it is the
CONVENER's own door at convene time asking a different question — whether the deadline covers every
hop — and no signer reaches it. The first draft of this paragraph said "the only one in the tree",
which the grep refutes.)*

**The budget is ZERO, and the arithmetic is the slice's hardest part.** The receiver must refuse
only `!rec.Expires.After(now)` — which is what the bullet says. Reserving a hop's worth at the
receiver refuses honest hops: `ceremonyHopBudget()` is `20s + 300s + SessionBudget()` = **29m20s**,
while the worst-case lag from the convener's dial to the signer's gate is
`20s + 300s + 360s + 300s + 360s` = **22m20s** — two `exchangeDeadline` arms in `Receive` **plus**
the spoken-check gate, which no connection deadline bounds because no I/O happens during it. The
margin is **7m00s and that figure IS the tolerable clock skew**; an 8-minute reservation makes it
minus one minute. *(Both the deepdive and its grill got this arithmetic wrong in different
directions before it was re-derived at the line; the guard in T05 is what makes it falsifiable.)*

Tasks:
- **T01** — split `recordOutlivesBudget(rec, now, budget)` out of `checkCeremonyDeadline`; the
  initiator's door keeps its signature, its caller and `ceremonyHopBudget()`.
- **T02** — `p2p.ErrCeremonyEnded` + refusal code 13, **and `ceremony.ErrRosterMismatch`'s missing
  code 14 folded in**: C17's existing arrival refusal reaches the initiator as bare EOF today, and
  shipping the new guard over the older instance of the same defect is not a fix.
- **T03** — `p2p.ReceiveArrivalLag()`, **guard-only and never called from production**, because a
  copy of `Receive`'s arm count in `internal/server` is the duplicate derivation `SessionBudget`'s
  own doc grades critical. With it, a scan asserting `Receive`'s deadline arms — it has **four**,
  and only the **two** before the gate enter the lag.
- **T04** — `checkArrival` calls the rule with budget 0 on the record it already holds (zero added
  parses), and `Confirm` calls `noteFailure` so the protected party gets a sentence.
- **T05** — the nesting guard:
  `bootstrapBudget + connectDeadline + ReceiveArrivalLag() ≤ ceremonyHopBudget()`, with the surplus
  printed as the declared clock-skew tolerance.
- **T06** — a routing guard over `checkArrival`'s own body. Neither existing structural scan reads
  inside it (both brace-match only its two callers), so a check added there is invisible to every
  guard in the tree today.
- **T07** — the behavioural test, convener bypassed, three arms: live admitted, expired refused,
  and **an honest hop at the convener's worst case** (`Expires = t0+29m20s+ε`, `now = t0+22m20s`)
  admitted.
Acceptance:
- A record whose `Expires` is past is refused by `checkArrival` **with the convener bypassed**.
- The honest worst-case hop is **admitted** — the clause that separates this from a hop reservation.
- The refusal reaches the initiator as a **named** refusal rather than EOF.
- The nesting inequality holds and a guard fails if either side moves, with the surplus printed.
- `rd.Cached` re-delivery of an already-made signature still succeeds **after** `Expires` — the gate
  sits downstream of the cache, so a party who signed can always hand that signature over.
- Tier: 1 plus red-proof rows (C15). **Tier 4 cannot reach this**: `pairrepro.sh` hard-codes
  `expires` at now+48h with no override, and closing that needs a knob ADR-010's lesson governs —
  it must not feed one constant to both sides.

#### P08.S04b — The termination object *(D28, C06's telling half; split from S04 2026-08-30)* *(done 2026-08-30, v1.117.287 — party and When deliberately excluded, so no canonical form is needed; the binding is RosterHash alone)*
Scope: the signed artifact a decline or a completion leaves behind. ~~the decliner signs~~
**the CONVENER signs (2026-08-30, Dan — see the dateline)**: under D22's hub the convener is one end
of every hop and the only party that ever hears a decline, so it is the only shape mintable from
what a machine observes.

**Two fields are deliberately NOT in it, and that is the grill's finding.** `party` is out because a
convener-signed *"X declined"* is a **framing attack** — mintable before that hop even runs, naming
an innocent decliner non-repudiably. `When` is out because it is convener-chosen and unverifiable,
and letting it drive S06's grace would hand a convener control of when other machines prune;
retention starts from C11's **local** receipt's observed-at time. With both gone every surviving
axis is fixed-width or a closed literal, so the object needs **no `Canonical`/`IsCanonical`** — the
two malleabilities that machinery exists for were exactly those two fields.

**What it buys, stated honestly: earliness, not enforcement.** It **cannot bind the convener**, who
can simply not mint one and is also the sole courier. Every consumer must read absence as *unknown*,
never *live* — which is why S04a's expiry rule must not depend on it. What it does buy is an honest
convener ending a proceeding at every party promptly instead of leaving them to derive *abandoned*
at `Expires` + grace, and non-repudiable evidence of who ended it.
Acceptance:
- The preimage binds a domain tag, its own version, the convener fingerprint, the **raw** roster
  hash and the state — and `RosterHash` alone defeats cross-ceremony and same-id replay, because
  it already binds `ID`, `DocHash`, `Intent` and `Expires`.
- **Write-once**: a second write with the same roster hash and a different state is reported, never
  clobbered. Absence tolerated; present-and-unverifiable is its own state and never `ErrMirrorDamaged`.
- It has its **own door** — `ReadMirror`, `WriteMirror` and `refuseDifferentProceeding` unchanged, so
  a stale in-flight `Store` cannot print "Signed, but not saved" for a name collision.
- It is verified against the record from the **document/invitation** path, never the `record.json`
  sitting beside it — an anchor a naive cross-ceremony test passes.
Tasks:
- **T01** — `internal/ceremony/termination.go`: the type, the domain tag, its own version constant
  (not `FormatVersion` — D32), the five-chunk preimage, `Sign`, `Verify(rec)`.
- **T02** — `WriteTermination` / `ReadTermination` in `mirror.go`, **their own door**:
  `ReadMirror`, `WriteMirror` and `refuseDifferentProceeding` are unchanged, so a stale in-flight
  `Store` cannot print "Signed, but not saved" for what is really a name collision. Write-once;
  absence tolerated; present-and-unverifiable is its own state and never `ErrMirrorDamaged`.
- **T03** — a test asserting the preimage has **no malleable axis**, which is what earns the
  absence of `Canonical`/`IsCanonical`: every chunk is fixed-width, a derived lowercase hex
  fingerprint, raw bytes, or a closed literal.
- **T04** — `Stored` gains the end state (a field, **not** a fifth `LoadState` — that classifies
  the *load*, not the proceeding), so `/api/ceremonies` can report it.
- Tier: 1. Delivery is S05's, and **the object is inert until then** — which is what made the
  S04a/S04b cut the right one.

#### P08.S05 — The delivery round *(D22 and its delivery pin; C08, C10, C06's telling half)* — **SPLIT 2026-08-31 at its grill into S05a · S05b · S05c · S05d · S05e**
The nine acceptance bullets below are retained verbatim as the split's source; each is now owned by
one of the five slices that follow, and none is discharged at this coordinate. **The split's reason
is not size but that each bullet carries its own structural change** — an arm registry and a keyed
spoken-check gate, an off-LAN delivery rendezvous, an unattended verification gate,
ack-means-persisted, a per-party ack store P08.S01 deferred *to this slice*, a deterministic
delivered filename, recipient-side verification, termination delivery, a `Convene` deadline term,
and a measured egress budget.

**Three corrections the grill made to this slice's own text, recorded here rather than edited into
the bullets, because the bullets are the split's source:**

**(1) `state.json` does not exist, and S05 owns the decision.** The first bullet says the arm is
*"re-established from S01's persisted state at load"*. Named search
(`grep -rn "state.json|stateFile|mirrorState" --include=*.go .`) returns nothing: P08.S01's grill
dropped it, and S01's own text assigns the store's home to *"S04's end states or **S05's delivery
acks**"*. The per-party ack store is therefore this slice's to design, not to inherit.

**(2) A TCP ceremony arm has no DHT tier at all, so *"the round finds parties off-LAN"* is
unbuildable for TCP.** Measured 2026-08-31: `openRendezvous` returns at
`internal/server/ceremonyid.go:537-540` for any non-QUIC transport, **before** it sets `c.end`/`c.rz`
at `:560`; a QUIC *ceremony* arm never reaches `startArmedRendezvous` because `handleSessionArm`
returns at `internal/server/session.go:1408`; and that function's sole call site is `session.go:1438`.
So its guard at `ceremonynet.go:409-411` always fires and its whole body — LAN window, `inbound`
check, `feedCandidates`, `punchLoop`, the publish — **never runs**. This is `/pending 248`, filed at
the P05 close and now measured. Caveat 7 forbids the obvious repair (the probe and the session must
share a socket; a TCP listener cannot share a UDP one). **Taken at rung 2 and reversible by Dan:** a
TCP ceremony is link-local-or-typed-address only, stated as a limit, and the vestigial path is
removed. S05b carries it.

**(3) D35 already declares the clock-skew budget, and it is ±5 minutes — not the 7m00s two comments
claim.** `recordOutlivesBudget`'s derivation (`internal/server/ceremonynet.go:695-716`) and
`checkArrival`'s zero-budget justification (`internal/server/ceremonyid.go:635-651`) both assert
*"that margin IS the tolerable clock skew"*. It is a **residual**, not a budget, and it is 2 minutes
looser than what the handshake enforces: `transportSkew = 5m` / `transportTTL = 15m`
(`internal/p2p/transport.go:41,45`), both sides verify each other's leaf, so D35's own words are
*"the pair's usable budget is the tighter direction: ±5 minutes"*. At δ ≥ 5m there is no connection
and `checkArrival` never runs, so the 7m gate is not reachable by skew alone. Neither site cites D35.
**And the arm's skew is not handshake-bounded at all** — when an arm decides to close there is no
channel — which makes the arm's requirement strictly stronger than `checkArrival`'s and is why the
arm's bound is S05c's problem rather than a constant anyone can copy.

**The nine bullets, retained verbatim as the split's source.** Each is now owned by one of
the five slices below and none is discharged at this coordinate; the owner is named in
square brackets after the bullet's first clause where it is not obvious from the slice titles.

Acceptance:
- **A delivery arm**: a ceremony-scoped receive arm, re-established from S01's persisted state at load, accepting only the record's convener, bounded by `Expires` plus S06's grace rather than by `connectDeadline` — and a party idle **past `connectDeadline`**, and a party whose process has restarted, are both reached.
- **The round finds parties off-LAN** — a second armed rendezvous per party on the same D30 derivation, with its egress enumerated against D34 rather than inherited silently.
- **The verification gate for the delivery leg is settled in the plan and driven** — it is a document these parties already signed, over a pin and a secret that have not changed.
- **The acknowledgement means PERSISTED**: the receiver writes durably before the ack, a failed write is not recorded as acknowledged, and a re-run reaches that party **and no other** — driven by injecting the write failure at party 3 of 4.
- **Exactly one file per party after a re-run**, counted on each party's own disk; the delivered filename is deterministic and carries the ceremony id **and a human half derived from the record's intent**, so the finished lease is distinguishable from Monday's copy by name alone.
- **The recipient verifies what arrives** — `CheckRecord`, `MatchesRecord`, completeness over the full roster, and a byte-prefix check against **its own persisted contribution**, which is newly possible because S02 stored it and is the only check that catches a substituted prefix. Its blind spots are stated, not implied.
- **A ceremony declined at hop 3 delivers the signed termination object to the parties who signed at hops 1 and 2**, and the telling states the proceeding is over, who ended it, that their signature stands, and that a re-run starts from the original unsigned file.
- **`Convene` reserves a delivery term** as well as the hops, so the round is inside the deadline the user set and S06's grace is derived from the same figure rather than hand-chosen.
- **The arm's egress is BOUNDED, and ADR-011 binds it (added 2026-08-30, `/pending 326`).**
  `republishEvery() = candidateLife()/2` and `candidateLife()` is 480 s, so a waiting party
  publishes to the DHT **every 240 s for the life of the arm** — which this slice bounds by
  `Expires` plus grace, i.e. up to 30 days per party. ADR-011's hold does not save it: `holdDHT`
  renews on sightings from the LAN answerer, which is gated on `!cer.hasSigned()`, and a delivery
  arm is exactly the state that gate excludes. **`ADR-011` appears nowhere in P08's section**, and
  D33's packet law is per *hop* while a delivery leg is not a hop, so at N=9 the round's punch
  budget is undefined. P03's exit criterion reached **0 off-link packets** four phases late; this is
  the shape that undoes it. The slice states the delivery round's egress budget, derives it from
  D33 rather than inheriting it silently, and drives the figure.
- **Compose this with a `Cached` hit and the loop never returns (same pin).** A hit returns early
  from `coSignExchange` without calling `Store`, so `c.reDelivery` stays empty, `hasSigned()` is
  permanently false after a restart-and-re-delivery, `postSignDeadline` is never armed, and the arm
  runs to the 30-day bound publishing throughout. The plan costs this as *"one publish"*; it is
  ~360 a day. **An unmeasured cost claim, which this repo's own law forbids** — the figure above is
  arithmetic over constants read at the line, and the slice measures it rather than repeating it.
- Tier: 4 at N=4, with S01's restart verb (C15).

#### P08.S05a — The receipt stops lying: the write lands before the ack *(D24's ordering, C10's pin)* — **new, 2026-08-31, split out of S05** *(done 2026-08-31, v1.117.290 — 5 clauses, all met; 8 red proofs, 7 registered. **Its own review found the fix's mirror image twice**: the capability floor was fail-OPEN where the equality it replaced was fail-closed, and `ErrNotStored` was destroyed at the HTTP boundary and toasted as *"could not send"* — the sentence a dead peer produces, which is the same false statement the new byte was added to stop, one layer up. And the new tier-6 clause found a live defect on its first honest run: two documents from one peer inside a second collide and the second silently overwrites the first (/pending 342), which is P08.S05d's to close.)*
Scope: C10's pin says `ReceiveDocument` sends `ackOK` (`internal/p2p/session.go:750`) and the server
persists **afterwards** via `saveReceived`, whose own comment says a write failure *"simply reports
nothing"* — so a party whose disk write fails is recorded as delivered, never retried, never told.
The fix needs **no new interface and no wire-format bump**, which is what makes it the first slice:
`sessionAccepter.Accept` (`internal/server/session.go:718-731`) already holds all three of
`saveReceived`'s arguments, and `ackOK` is written *after* `Accept` returns. Moving the durable write
inside `Accept` puts this path on the ordering `coSignExchange` already uses for `rd.Store`
(`internal/p2p/session.go:963-970` — *"persist before deliver, and it is here because here is BEFORE
the frame"*). A failed write must not be reported as `ackDeclined`, which is a false statement about
the human's decision.
Acceptance:
- **A receiver whose durable write fails does not send `ackOK`**, and the sender is told *accepted but not stored* rather than *declined*, driven by injecting a write failure at the door. Tier 1.
- **The write is ordered before the frame at ONE door**, asserted structurally by routing rather than by the text each site prints (ADR-009) — the guard fails if a second site persists outside it. Tier 1.
- **An older peer receiving the new receipt gets a message that is vague but never an accusation** — `refusalFor`'s one-byte `default` (`internal/p2p/session.go:645-656`) yields *"unexpected receipt from peer"* at `:702`, not the *"returned document is not the one sent this session"* tampering verdict D32 forbids. No ALPN gate is needed **because** of that default, and the reason is asserted rather than assumed. Tier 1.
- **`SpeaksNamedRefusals` becomes a floor rather than an exact equality** (`internal/p2p/channel.go:71`, `/pending 338`), and the guard that enumerates non-speakers (`internal/p2p/refusalwire_test.go:259-270`) can no longer bless a newer protocol falling into that class. Done here because this slice is the first to reason about receipt compatibility; it is a precondition for any later `alpn3`, never a companion to one. Tier 1, red proof.
- **The one-way transfer still completes end to end on both transports**, so the reordering did not break the path it protects. Tier 6.
- Tier: 1 for every rule; tier 6 for the end-to-end clause (C15).
Tasks:
- T01 — `SpeaksNamedRefusals` becomes a floor over the negotiated ALPN, and `refusalwire_test.go`'s non-speaker enumeration is rewritten so a newer protocol cannot fall into it (`/pending 338`).
- T02 — a one-byte `ackNotStored` receipt plus its decode arm, with the older-peer path asserted to yield *"unexpected receipt from peer"* rather than a tampering verdict.
- T03 — `saveReceived` moves inside `sessionAccepter.Accept`, returning its outcome, so the durable write precedes `ackOK`.
- T04 — the structural guard that the persist routes through that one door (ADR-009), with a second site made to fail it.
- T05 — the one-way transfer driven end to end on both transports (tier 6).

#### P08.S05b — `Convene` reserves a delivery term, and the TCP limit is stated *(D16, D22; C10's grace, `/pending 248`)* — **new, 2026-08-31, split out of S05**
Scope: S05's eighth bullet and S06's grace both need one figure, and neither can be hand-chosen.
`Convene` today reserves `hops × HopBudget` (`internal/ceremony/convene.go:212-217`) and reserves
nothing for the round that follows. Plus S05's correction (2) above: the dead TCP rendezvous path
goes, and the limit it implies is written down rather than left as code that cannot run.
Acceptance:
- **`Convene`'s reservation gains a delivery term**, derived from figures already in the tree rather than chosen — `bootstrapBudget + connectDeadline + postConsentDeadline` = 20s + 300s + 120s = **7m20s**, carrying **no `PeerGateWindow`** because a delivery leg runs no `Confirmer`. Tier 1.
- **The term is added at `convene.go:213` only**, never folded into `ceremonyHopBudget()` — which would redden `ceremonydeadline_test.go:99` and `convene_test.go:24` (both literal 29m20s) and, worse, leave `ceremonypin_test.go:418`'s hard-coded 7m green while it silently stopped being the real margin. Asserted. Tier 1.
- **`WarnSittingCeiling`'s sentence is recomputed from the same figure** (`convene.go:289-296` recomputes `hops × HopBudget` independently today), so the warning cannot disagree with what the door reserved. Tier 1, red proof — the existing test asserts the warning CODE, not the number, so this drift is invisible without one.
- **The dead arm-rendezvous path is removed and the TCP-ceremony limit is stated in the docs** — a TCP ceremony reaches its peer on the link or at a typed address, never through the DHT, because caveat 7 requires the probe and the session to share a socket. Tier 1 for the removal; the limit is a docs clause.
- Tier: 1 throughout; nothing here needs a live peer (C15).

#### P08.S05c — The arm becomes addressable, and the spoken check gets a key *(D22; C05, C08; the TRIPWIRE)* — **new, 2026-08-31, split out of S05**
Scope: the structural slice, and the grill's hardest finding. A delivery arm cannot avoid `s.sess`
the way `handleSessionInitiate` avoids it — that listener is `defer hl.Close()`
(`internal/server/session.go:1868`), request-scoped, with a human at the keyboard, and the comment
above it states the opposite rule about this exact construct. `s.sess` is the machine's single answer
to *what is armed*: `noteFailure` and `setReceived` (`session.go:225-235`) are a delivery arm's only
failure and success channels and **both arm doors clear them** (`:176`, `:216`). Three further
collisions are keyed by ceremony id rather than by socket — the punch budget
(`internal/server/punch.go:102-117`), the BEP-44 publish target, and the mirror's single
`document.pdf`.
Acceptance:
- **Two arms coexist, keyed**, and arming for an unrelated ceremony no longer erases a live delivery arm's notice or received record — driven by arming both and asserting each keeps its own. Tier 1.
- **`armedLocked` becomes a COLLISION predicate rather than "is anything armed"**, still through one door (ADR-009); `TestASecondArmCannotOrphanALiveCeremony` is re-expressed from *"must fail"* to its own stated harm, *"must not overwrite"*. Tier 1.
- **The spoken-check slot carries an arm key**, so a second arm's gate cannot be refused with `errVerifyBusy` after `saw.mark()` has already spent it (`session.go:638` runs before `:641` discovers the slot is taken). Tier 1, red proof.
- **The arm's own bound is decided and justified against D35 rather than copied** — see S05's correction (3): the arm's skew is not handshake-bounded, so its bound may not be derived from the 7m residual. The chosen figure names what it tolerates. Tier 1.
- **The TRIPWIRE is cited and what it widens is stated** (`session.go:30-57`): *"what arms it"* — a listener re-established at load rather than by an explicit vault-unlocked `/api/session/arm` — and *"how long it stays open"*. *"Which peers it accepts"* is NOT widened, and saying which of the three moved is the point. Docs clause plus the citation at the site.
- Tier: 1 for the keying and the guards; tier 4 for two live arms on one machine (C15).

#### P08.S05d — The round itself: found off-LAN, unattended, named on disk *(D22, D30, D34; C08, C10)* — **new, 2026-08-31, split out of S05**
Scope: the delivery leg. The rendezvous is derived at **its own hop index** — `RecordKey(hop)` and
`RecordSalt(hop, fp)` are already hop-parameterised (`internal/ceremony/invitation.go:543`, `:565`),
so a delivery leg sharing the hop's index would publish at the same BEP-44 target and, in
`RecordSalt`'s own words, *"the higher-seq write silently clobbered the other"*. The delivered file
goes **outside** `~/nib/ceremonies/`, which is also what stops it colliding with the mirror's single
`document.pdf` and making `persistedFor` return a finished N-party document as a hop's re-delivery.
Acceptance:
- **The round reaches a party off-LAN**, over a second armed rendezvous at its own hop index, with its egress enumerated against D34 rather than inherited. Tier 4.
- **The verification gate for the delivery leg is unattended, and settled in the plan** — the leg is a document these parties already signed, over a pin and a secret that have not changed. Implemented by passing a non-interactive `Verifier` at the two delivery call sites, **never** by a bypass inside `internal/p2p`: `runVerification` fails closed on nil (`internal/p2p/verify.go:239-244`) and `deadlines_test.go:122-126` fatals unless exactly five entry points call it. Tier 1 + tier 4.
- **The delivered filename is deterministic** and carries the ceremony id and a human half derived from the record's intent, so the finished lease is distinguishable from Monday's copy by name alone. It does not reuse `receivedName` (`session.go:1132-1138`), which reads `time.Now()` inside the builder at second granularity and collides within one second. Tier 1.
- **The recipient verifies what arrives** — `CheckRecord`, `MatchesRecord`, completeness over the full roster, and a byte-prefix check against its own persisted contribution. Its blind spots are stated, not implied. Tier 1.
- **A re-run after a mid-round failure reaches that party and no other, leaving exactly one file per party**, driven by injecting the write failure at party 3 of 4 — which is only meaningful because S05a made the ack mean persisted. Tier 4.
- Tier: 4 for the round, 1 for the naming and verification rules (C15).

#### P08.S05e — The end state is delivered, and the round's egress is bounded *(D28, D33, D34, ADR-011; C06's telling half)* — **new, 2026-08-31, split out of S05**
Scope: wiring P08.S04b's termination object, which is built, verified and has **zero production
callers** (`SignTermination`, `WriteTermination`, `ReadTermination`), and bounding what the round
emits. ADR-011 appears nowhere else in P08's section, and D33's packet law is per *hop* while a
delivery leg is not a hop — but the budget is in fact keyed per **ceremony**
(`punchBudgetFor(c.inv.ID)`, `internal/server/punch.go:102-117`) and never resets (`:72-73`), so the
risk is the hops **starving** the delivery legs, not an unbounded round.
Acceptance:
- **A ceremony declined at hop 3 delivers the signed termination object to the parties who signed at hops 1 and 2**, and the telling states the proceeding is over, who ended it, that their signature stands, and that a re-run starts from the original unsigned file. Tier 4.
- **The round's egress budget is derived from D33 and DRIVEN, not asserted** — the figure is measured, per this repo's own law that a claim containing a number is measured or declared unmeasured. Tier 4, with `pairrepro.sh`'s off-link counter.
- **The delivery legs cannot be starved by the hops**, or the sharing is stated as a limit with the arithmetic — `punchBudgetPerSide = 3000` is one D33 LAW figure per machine per ceremony across every hop. Tier 1 for the arithmetic, tier 4 for the N=9 case.
- **ADR-011's hold is re-examined for a delivery arm**, whose renewal is gated on `!cer.hasSigned()` (`internal/server/session.go:801`, `:1480` → `lan.go:777`, `:788-790`) — exactly the state a delivery arm is in. Tier 4, against P03's zero-packet criterion.
- Tier: 4 for every clause but the budget arithmetic (C15).

#### P08.S06 — Close-out: end state, then delivery, then the prune — and nothing is destroyed *(D29 lifecycle pin; C09, C11)*
Scope: D29 states the lifecycle once — *end state → delivery round → close-out* — and puts the pin
drop and the prune at close-out. Two corrections the panel forced. The prune **moves, it does not
delete**: on declined, expired and abandoned there is no delivery round, and the mirror is the only
place a non-convener's own signed contribution exists. And close-out is **three** stores, not two:
the vault's per-party secrets are pruned only by the convene *rollback* today, while the product
already tells the user they go when a ceremony ends.
Acceptance:
- **The prune is a move**: this machine's own signed contribution is written outside `~/nib/ceremonies/` first, and the observation is that file's presence at a named path **after** the prune — asserted for a party who signed at hop 2 of an abandoned ceremony.
- **One close-out door** takes pins, **secrets** and the mirror, reporting the three separately as `unconvene` already does; **C09's ordered observation asserts the vault holds no secret for that ceremony afterwards**, and a pin the user promoted survives.
- **A ceremony abandoned before its delivery round is closed out after `expires` plus the grace** — evaluated against the record's own `Expires` at startup, at unlock, and as a side effect of S03's listing, never on a wall-clock timer, because every ceremony route is behind `requireUnlocked` and a machine left locked over a weekend prunes nothing while wall time runs.
- **The grace is a named constant in the tunable block, derived from `ceremony.MaxCeremonyLife` rather than hand-copied** — the `maxCandidatesPerSource` rule, not D33's law/tunable guard, whose `lawFigures` list is a deliberate two-name whitelist that a tunable must not join.
- **The root is resolved once and refused if it is not absolute** before any `RemoveAll` — `defaultOutputDir` falls back to a relative `"nib"` when `os.UserHomeDir` fails.
- **A decline no longer prunes at the end state**, driven by the declined ceremony's round still reaching the signers.
- Tier: 4 for the ordered observation and the abandoned close-out; tier 1 for the root and grace rules (C15).

#### P08.S07 — Two refusals that are already there, driven honestly *(D29 gap #28, D21 gap #24; C13, C14)* *(done 2026-08-29, v1.117.246 — both driven at the route for the first time; C13's LOOSER direction found unreachable and scoped out with the reason)*
Scope: both look met and this slice finds out. The convene refusal covers the direction that already
works and misses C13's own — re-opening the **original** file and convening again. The re-issue hands
back the same stored secret, so a before/after comparison of other parties' state is true by
construction and proves nothing unless it is shown able to fail.
Acceptance:
- **Convening again on a document already carrying a record is refused server-side by name, with 409 and the cost clause.** *Met*, and driven at the route for the first time. `ErrAlreadyConvened` has existed since P07.S02a and was driven only at the PACKAGE; `conveneStatus`'s 409 and the sentence a user reads were asserted by nothing, and tier 6's "SECOND ceremony" clause convenes on a FRESH document, which is the allowed case.
- **C13's looser direction is UNREACHABLE, and is scoped out here rather than left implied.** Re-opening the ORIGINAL file and convening again cannot be keyed on the hash: `docHash` is computed over the **prepared** document (`internal/ceremony/convene.go:230-237`), which embeds a fresh 128-bit ceremony id, so two convenes of one source file produce two different prepared documents with two different hashes. Catching it needs the ORIGINAL's hash stored in the record as well — a `FormatVersion` bump, and therefore a slice rather than a task. **`not exercised`, with what would settle it named.**
- **The treatment of an ENDED ceremony is settled: it stays refused, and here is the reason.** A document that carries a record carries it forever — the record is an embedded file inside what becomes a signed document, and `Embed` refuses an already-signed one, so it can be neither removed nor replaced after the first signature. "Learn liveness" would therefore mean consulting the local mirror, which C12 makes hand-deletable: a deleted mirror would re-permit convening on a document that already carries a record, which is precisely the two-records-on-one-document harm D29 names. The document is the only source of truth that travels with it. *Settled at rung 1 and recorded, rather than shipped as an unstated stricter-than-specified behaviour.*
- **A re-issue returns each party's invitation byte-identical to the one convene issued**, so a party who lost their email is not made to re-accept a different one and every other party's copy stays valid. *Met*, with the red proof that makes the comparison able to fail — without it the assertion is true by construction, since the route reads the secrets back from the vault.
- **Driving a re-issue after a hop has actually completed is `not exercised`** — it needs the relay. The assertion that mattered is P08.S01's `TestAReIssuedInvitationStillMatchesItsRecord`, which is where the live defect was and which this slice's work rests on.
- Tier: 1 at the route for both refusals; tier 6 and the relay are what the two `not exercised` rows would need (C15).

#### P08.S08 — The lifecycle gets a reader *(D34 self-healing, D19's model; new at the plan-review, 2026-08-29)* *(**done** 2026-08-29, v1.117.253 — the surface, the two silent paths, and the recovery action once D24 was amended; the event RECORD deliberately not built)*
Scope: P08 adds five failure modes — the persist failed, the re-delivery never happened, the resume
found a damaged mirror, the round stalled at party 3, the prune moved something — and every one of
them reports today through `log.Printf` to a stderr that a double-clicked launch sends nowhere. The
tree already knows this and says it once, about the hand-off notice: *"a double-clicked launch has no
terminal: its stderr goes nowhere a user will look, so a refusal logged here alone is a refusal
nobody receives"* (`cmd/nib/main.go:271-274`). That reasoning was applied there and to nothing else.
Without this slice, C11's *"the prune moved something"* is undetectable and unattributable by
construction, and C03's sentence has no channel.
Acceptance:
- **A sticky failure field on `sessionStatus`** that outlives the session and is cleared only by the next arm. *Met.* Sticky is the load-bearing word: a field cleared on disarm would be worthless because **the disarm is the symptom** — the user looks after the arm has gone quiet, which is exactly when a session-scoped field is already gone. It is cleared by the next ARM instead, because the user trying again is what makes the old reason spent. It carries a stable `what` key so a surface branches on a value rather than matching prose.
- **The "signed but not saved" state offers a recovery action.** *Met*, unblocked by D24's amendment (Dan, option A). The remedy is that the document is still **opened**: the bytes are complete and valid — the peer has them — so a tab the user can Save-As from is the whole recovery, through the door that already exists. That only works because the persist failure stops being an error before the caller opens the document, which is asserted structurally on `TestBothSidesOfAHopMirrorIt`'s precedent. **A sentence alone would not have been a discharge**: if all it does is say "do not close Nib", the only action available is to leave Nib running forever and the next power cut destroys the signature anyway. **Owed:** the end-to-end drive against a real unwritable directory (the mode-0500 mechanism `ocrfonts_test.go` uses), which needs a filesystem this suite cannot assume.
- **`saveReceived`'s silent write failure gains a reader**, and `mirrorHop`'s log-only one too. *Met.* These were the two worst: `saveReceived` used a bare `return` with its own doc saying it "simply reports nothing", and it is the one path in the tree that loses a peer's document with no trace **after the sender has already been told it was accepted**. **A full event RECORD is not built** — the single last-failure notice is what the two existing producers needed, and a log of events wants producers that do not exist yet (delivery, close-out). Scoped down deliberately rather than half-built.
- **The counters live in a package the reader scans cover**, or the gap is recorded: `internal/server` is absent from `observables_test.go`'s package list, so a counter placed there is invisible to every reader scan in the tree.
- Tier: 3 for the recovery action, 1 for the field and the record (C15).

#### P08.S09 — Docs, README, and the phase close *(C15, C16, C17)*
Scope: the docs-parity bullet and the phase's own close. C15 and C17 are standing constraints on
S01–S08 and are **ledgered** here, not discharged here.
Acceptance:
- **The README and `docs/` describe the lifecycle as it now is** — what persists, what resumes, what is delivered, what is moved and when — with D28's end states named, and the **limits** stated: the deadline cannot be extended, a party cannot be replaced, ending early means starting again from the unsigned document, and Nib executes one instrument in sequence rather than in counterparts.
- **The document's own silence is stated**: a Nib PDF does not record how its ceremony ended, and completeness is the only proceeding-level fact it proves about itself. `nib verify` names an end state **only** where a local mirror for that ceremony exists, under a heading that says so, and never prints "completed" from an absent one.
- **`nib verify` names a valid signature the roster does not account for and exits 2** — `Completeness` iterates the roster and breaks on the first match (`internal/p2p/attestation.go:356-367`), so `signed` can never exceed `obliged` and a duplicate or off-roster signature reads as a clean complete ceremony on both the CLI and the web verdict. The cheapest forensic yield in the phase.
- **`CONTRIBUTING.md`'s tier table and `verify_test.go`'s ceiling list still describe what the harnesses now do**, including tier 6, which is absent from both today.
- **Nothing in this phase writes to a document after its last signature**, stated in the scope so the constraint is visible to whoever builds the end-state reporting.

---

## Out of scope

- **Legal, GTM, and marketing-site work** — handled after the product works, by their own skills.
- ~~**Group ceremonies (more than two parties).** The attestation model is two-party today (`coSignExchange` requires exactly one prior signer, `session.go:207`). Widening it is a separate project.~~ **Struck 2026-08-18 on Dan's instruction — group ceremonies are now the feature (D22, P07).** Two corrections to what this entry claimed: the line is `session.go:229`, not `:207`, and **the attestation model is not two-party** — `buildCoSigned` (`internal/server/cosign.go:223`) counts nothing, `stackPlacement` places an *n*-th block, and `crossBind` already cross-binds every signer against every other. The offline path can produce an N-signature document today. The two-party assumption is one `len(ats) != 1` in the live path and a layout that breaks at the fourth block (D25) — which is why this was cheaper than "a separate project" made it sound, and why the entry is struck rather than deferred.
- **A relay for the carrier-grade-NAT case.** Excluded by the constraints; the manual path (D9) is the answer instead.
- **Changing the signature or attestation format.** D2.
- **The multiple-open-documents feature** — shipped and its plan retired 2026-08-19; the surviving record is `docs/adr/` (ADR-001 through ADR-006) and `CONTRIBUTING.md`'s multiple-documents laws.
- **Mobile or web clients.** Nib is a desktop app with a loopback UI.
- **The ceremony on the CLI.** `internal/cli/cli.go:78` carries twenty verbs and no session verb — live co-signing is GUI-only today, and stays so. *Why:* every step that matters in a ceremony **is a consent** — reading what you are about to sign and deciding — so a scriptable ceremony is either an interactive program wearing a CLI's clothes, or the unattended signing this project has already declined. *The revisit trigger, which D22 made plausible for the first time:* a real convener asking for it. A clerk driving a four-party signing is exactly the person who would want to script the invitations, and issuing an invitation is the one part of a ceremony that carries no consent. *(added 2026-08-18, holistic pass — taken by recommendation on Dan's behalf, and reversible: it adds a phase, it does not disturb one.)*

## Standing caveats

Load-bearing claims not yet verified. Each is a Stage 2 grill target, and the arc's
own failure-mode #1 is a "verified" claim about a dependency that was never
re-verified.

1. **The QUIC library invokes `VerifyPeerCertificate` exactly as `crypto/tls` does**, with `InsecureSkipVerify` set and `RequireAnyClientCert` honoured. The entire pinned-peer model rides on it. If false, D7 needs rework, not adjustment.
2. **DHT responses carry the requester's port, not only its IP.** P04's self-address probe depends on it; without the port, IPv4 punching loses its input.
3. **Multicast discovery behaves on Windows** as it does on Linux. Recent releases show Windows-specific paths needing their own handling (`v1.101.0`, `v1.102.1`).
4. **A suitable wordlist exists with a licence compatible with AGPLv3 distribution**, and with phonetic distinctness good enough to read over a phone. **(RELAXED 2026-08-18 — Dan's collapse to one pairing path.** The freeze below was justified by names decoding to fingerprints: swap a word and a name written on paper last year decodes to a *different key*. **With D31 retired, no name ever decodes to a pin** — a wordlist change alters a *displayed label*, which is a breaking UI change and not a silent authentication failure. The checksum guard is kept, because a label people learn to recognise should still not move quietly; the *forever* is downgraded to *not without a version bump and a note*. **This also discharges the pending item filed after the Stage 2 grill**, which asked where the freeze's cost should be stated — the cost is now small enough that the question dissolves.)** **(plan-review pin, 2026-08-17, adopted by Dan: the list must be FROZEN at first release.)** The name is an *encoding* of the fingerprint (D3) that never rotates (D5), so this list defines the meaning of every name ever spoken — and this caveat leaves selection open on *phonetic distinctness* grounds, which is precisely the reason someone later swaps a word. Do that and every user's name silently changes, and a name written on paper or read over a phone last year now decodes to a **different fingerprint**. Stored pins survive because they are bytes (D10, P01.S03); spoken ones do not. **What discharges this specifically:** P01.S01's checksum guard over the list file, failing on any change — not the fixed-vector corpus, which is computed *from* the list and therefore moves with it.
5. ~~**~66 bits is the intended floor** given D4's mandatory verification.~~ **STRUCK 2026-08-18 — Dan's collapse to one pairing path (D31). There is no 66-bit pin anywhere in the design: the name never decodes to a pin, so there is no floor to defend.** *Retained below for the record:* **~66 bits is the intended floor** given D4's mandatory verification. If the verification step is ever weakened, this number becomes the whole security of the pairing. **(narrowed 2026-08-18, D4/D21: this caveat now binds the *name-only* pairing path alone. A ceremony paired from an invitation pins the full 32 bytes and has no 66-bit floor to defend — which is exactly why the spoken check may drop there and may not drop here. The caveat is narrowed rather than struck because the name-only path is retained, and it is now the only place the plan's original security argument still has to hold.)**
6. **A Go port-mapping library covering PCP, NAT-PMP and UPnP-IGD exists under a licence compatible with AGPLv3 distribution** (D15). If only some protocols are covered, the tier still ships — with narrower router coverage, recorded rather than assumed. *(added 2026-08-16)*
7. **The mapped port, the DHT self-address probe and the live session must all be the same local socket.** A NAT mapping — learned or requested — is a function of the *internal* `IP:port`, so a mapping obtained on the DHT socket or on a throwaway socket is useless for a session that listens elsewhere, even under a perfectly endpoint-independent NAT. This constrains library selection in **both P02 and P04**: the QUIC library must accept an existing `net.PacketConn` and the DHT must be willing to share it. Load-bearing for tiers 3 and 4 alike, and not currently reflected in either phase's slice sketch. *(added 2026-08-16)* **(plan-review pin, 2026-08-17, adopted by Dan: now reflected, and it had to be.)** This caveat identified the constraint, named the two phases it binds, and recorded that neither enforced it — and **P02 is the next phase built, with library selection as its first slice.** A QUIC library that owns its own socket passes every exit criterion P02 carried (a ceremony completes, the pinned callback rejects, the core is re-typed) and makes tiers 3 and 4 unbuildable three phases later. P02 and P04 now each carry a socket-sharing criterion of their own.

   **(Stage 2 grill pin, 2026-08-18 — one socket needs a demultiplexer, and the cheap one does
   not work.)** This caveat says three things must share a socket and treats that as a
   *library-selection* constraint. It is also an **unbuilt component**, and no phase names it:
   two protocols arriving on one UDP port must be separated before either library sees them.

   *Why the obvious separator fails — arithmetic, not argument:* every KRPC message on the
   BitTorrent DHT is a bencode **dictionary**, so its first byte is always `'d'` — `0x64`,
   binary `0110 0100`. QUIC's header-form bit (`0x80`) is therefore **clear** and its fixed bit
   (`0x40`) is **set**, which is exactly the encoding of a QUIC **short-header** packet — the
   steady state of every established connection. The header bits do not separate DHT traffic
   from session traffic; they collide on the common case, not on an edge. (Bencode integers,
   lists and strings would separate cleanly. KRPC never sends one at the top level.)

   **(implementation pin, 2026-08-19, P02.S02's selection spike — the library's own hook is
   built on the refuted discriminator.)** This pin says a pair is disqualified if neither
   library exposes a passthrough hook. **quic-go exposes one — `Transport.ReadNonQUICPacket`
   — and it is unusable for KRPC**, because its classifier is exactly the arithmetic above:
   `IsPotentialQUICPacket(b) = b&0x40 > 0` (`internal/wire/header.go`), and a bencode dict's
   `'d'` = 0x64 has that bit set. Measured before it was read: a hand-sent KRPC ping vanished,
   with and without a DHT attached; a fixed-bit-CLEAR datagram (STUN's shape) arrived. So the
   hook works and cannot carry bencode.

   **What this corrects:** the passthrough hook is **not** the property that makes a pair
   selectable, and testing for one would have disqualified the only viable pair. What matters
   is that **both libraries accept an externally-supplied `net.PacketConn`**, so a
   demultiplexer *we* write can own the socket and hand each a shim. Both do —
   `quic.Transport.Conn` and `dht.ServerConfig.Conn`, each proven by a spike that binds first
   and hands in. **Selected: quic-go v0.61.0 (MIT) and anacrolix/dht v2.24.0 (MPL-2.0).**
   P02.S03 builds the demultiplexer, and it keys on peer address, not on a leading byte.

   *What the demultiplexer must key on instead:* the destination connection ID, or an active
   QUIC path's peer address — QUIC's own mechanism for deciding "not mine". That requires the
   QUIC library either to expose that decision or to accept unrecognised datagrams being handed
   elsewhere.

   **(implementation pin, 2026-08-19, P02.S03 — built, and the caveat understated one rule.)**
   This caveat offers two keys and treats them as alternatives. **Neither alone is enough**, and
   the built demultiplexer uses both: an active peer address cannot speak for a connection that
   has not been established yet, so an inbound dial from a stranger would be handed to the DHT
   and the ceremony could never be *received*. The **long header** covers exactly that gap and is
   sound in the one direction it is used — a bencode dictionary's `'d'` can never set `0x80`. So
   the rules are long-header-first, then peer address, then DHT.

   *And a property the caveat did not ask for, which turned out to matter more than the choice of
   key:* **where the peer table learns from.** Learning on an inbound long header is the obvious
   reading and it is wrong — the table would then grow from unauthenticated remote input, and
   forged long headers from spoofed addresses would grow it without bound. It learns from **our
   own outbound writes** only. Deciding whether an Initial packet is genuine remains quic-go's
   job, which is equipped to do it; the mux just declines to remember the sender.

   *What this changes:* **selection is disqualifying in both directions.** A QUIC library that
   owns its socket is out; a DHT that owns its socket is out; **and so is a pair with no
   passthrough hook, even when each accepts an external `net.PacketConn` on its own** — which is
   the case the 2026-08-17 pin above could not see, because it tested each library alone. P02's
   slices and P04's criterion are amended. Found at Stage 2 rather than at P04, which is the
   difference between a selection constraint and three phases of rework.

   **(plan-review pin, 2026-08-18, adopted by Dan: multicast is not in this list, and that is
   deliberate.)** This caveat names the mapped port, the DHT self-address probe and the live session.
   **Tier 1's multicast is UDP too and is deliberately outside it** — it is link-local, it carries no
   NAT mapping, and nothing about it needs to share the session's socket. Recorded so **P03 does not
   inherit P02's demultiplexer by assumption**, which is the shape of mistake a shared-socket rule
   invites once it exists.
8. **Carrier-side PCP deployment is not assumed.** RFC 6887 was specified with carrier-grade NAT in mind, but whether carriers actually answer PCP is unverified and, on present evidence, mostly no. The CGNAT case stays D9's until measured. *(added 2026-08-16)*
9. **Two DHT observations are enough to separate the mapping classes, and the DHT will answer two distinct nodes within D16's probe budget.** D19's diagnosis rests on it; a two-server STUN check is the established form, but that the BitTorrent DHT's response pattern supplies the same two observations in ~8 s is unverified. If it does not, cause 3 degrades to cause 4 — a worse message, not a broken ladder. *(added 2026-08-16)*

10. ~~**A PDF attachment written in the pre-signing pass survives N incremental signatures and stays readable and verifiable.**~~ **STRUCK 2026-08-18 — measured at the Stage 2 grill, not argued.** A test PDF through `PrepareDocument` → `pdfops.AddAttachment` → **three** `Contribute` increments: the attachment came back **byte-identical after every one**, `sign.Verify` reported `valid` with 1, 2 and 3 signers, and **every signer verified**. Setup assertions fired first (attachment readable before signing; document `Unsigned` at that point), so the result is not a green over an absence. **The control arm is the more valuable half:** attaching *after* signing returned `state=invalid` with **all three signers `valid=false`** — so D20's "same pre-signing pass" timing is a demonstrated consequence rather than an assumption, and it is what D29's freeze exists to protect. *(The original text, retained: )* The whole Ceremony Record model (D20) rides on it, and so does resumption, because the record is what a resumed hop reads. Also unverified: that pdfcpu's attachment API can run inside the same structural pass as `AppendReadme` without a second full rewrite — a second rewrite after a signature is exactly what `PrepareDocument` already refuses. **P01.S06's first acceptance bullet discharges this empirically; reading the library's documentation does not.** *(added 2026-08-18)*
11. ~~**The invitation secret's channel-binding mechanism is unchosen.**~~ **DISCHARGED 2026-08-19 at P01.S07's slice grill — HKDF over `secret ‖ exporter`, confirmed with a MAC over the role. Not a PAKE: a PAKE bounds an attacker to one online guess against a secret small enough to enumerate, and D21's secret is 32 bytes of uniform randomness — there is no dictionary, and an offline attacker faces 2²⁵⁶ whatever the protocol. It would buy a property the secret already has and charge a dependency, a licence and round trips for it. `golang.org/x/crypto` was already a dependency; the binding reuses the same RFC 5705 exporter P01.S04's spoken string binds to.** *(Original text follows.)* **The invitation secret's channel-binding mechanism is unchosen.** D21 says the secret binds the channel and deliberately does not say how — a PAKE over the secret, or an HKDF over secret ‖ transcript folded into the session's key confirmation. The choice has a licence consequence and a correctness consequence, and it is now the plan's **largest unreviewed cryptographic surface**, because D12's external gate was withdrawn on 2026-08-18. Named at slice-grill time in P01.S07, and a Stage 2 grill target in its own right. *(added 2026-08-18)*

## Bookkeeping

- Amendments follow the house mechanics: a dateline clause per pass, tagged pins, strike-and-supersede. No silent rewrites.
- Every amendment is a commit with a patch bump per this repo's CLAUDE.md.
- ~~`/createcode` must be told it is walking *this* plan and not `PLAN.md`.~~ **(moot 2026-08-19 — `PLAN.md` retired; this is the repo's only plan.)**
- Residual doubts go to the `/pending` memory lists, not to chat.
- **L3 gets an ADR in the change that first makes it constrain code** (CLAUDE.md, STANDARDS §11),
  as ADR-001 did for operation pinning. Until then it is a plan law and lives here. *(added 2026-08-18)*
- **The plan now carries no external review gate** (D12 withdrawn 2026-08-18). The remaining gates
  are the Stage 2 grill, the three law guards, and the per-slice `/grill`.
  Recorded here because a plan that quietly loses a gate reads exactly like one that never had it.
- **The Stage 2 grill is scheduled: it runs before P01 is built** (P01's entry condition, added
  2026-08-18). It had appeared as "still has not run" in six datelines and as "a Stage 2 grill
  target" against twelve parked questions, with no phase requiring it — the same defect D12's pin
  named, and D12 is now gone, so this is the only gate left to lose. *Taken by recommendation under
  the ASK ladder: process, not risk appetite.* *(added 2026-08-18)*
- **Cite by identifier, with the line number as a hint — not the other way round.** *(plan-review
  pin, 2026-08-18, adopted by Dan.)* This plan's discipline is read-at-the-line, and its citations go
  stale under ordinary work: three `web/index.html` refs in D13's pin drifted eight lines **on the day
  they were written** (`:1190`/`:1215`/`:1239` → `:1198`/`:1223`/`:1247`), and D13's body still cites
  `:257` for a role picker now at `:272`. Two sessions commit to this repo daily, so this is a
  recurring class rather than an instance — Go citations were spot-checked and held, because Go files
  moved less. **New citations name the identifier first** — `#sessionInitModal`,
  `handleAttachmentAdd`, `Confirmer` — so a stale reference degrades to *findable* instead of to
  *wrong*. Existing citations are corrected as they are touched, not in a sweep: a bulk renumber would
  be a large diff whose only content is line numbers, and it would be stale again tomorrow.
- **Three items of the 2026-08-18 holistic pass were taken on Dan's behalf and are reversible where
  they sit:** D31 (the invitation is the default pairing path), the CLI scope-out in *Out of scope*,
  and the grill's scheduling above. Each says at its own site what reversing it costs. *(added
  2026-08-18)*
