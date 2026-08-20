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
fingerprint (`internal/p2p/transport.go:83`) and the verification string. A
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
| `roster` | ordered `{name, fingerprint, label, signs}` | the order **is** the roster's order; `signs:false` is the non-signing convener (Dan, 2026-08-18) |
| `intent` | what everyone is agreeing to — **the only home; see the pin below** | today each side types its own sentence; a four-party signing agrees to one thing |
| `expires` | the ceremony deadline, in days | D16's clock 3 |
| `rosterHash` | commitment over all of the above — **axes enumerated below** | the token every signature carries (D2 pin), so all signatures attest to one proceeding |

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

**Adopted: the attestation binds to the record, not to the wire.** `AcceptedPeer` names the party's
**predecessor in the roster**; checks 2 and 3 are re-based onto *the document's signers equal the
record's roster prefix* (which L3 already requires) plus a separate, weaker check that **the wire
peer is the record's carrier**. **What discharges this specifically:** a P07 criterion in which **a
four-party ceremony with a non-signing convener completes and every signature on the finished
document reports `Matched`** — not the existing "every attestation shares one `[NibRoster:…]`
commitment" bullet, which is satisfied perfectly by a document on which no signature matches any
other.

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
  with the mitigation removed. **The signature is held in memory, the delivery is not attempted, and
  the failure is reported as "signed but not saved — do not close Nib"** — the only honest sentence
  available, because closing Nib is what destroys it. Reporting it as an ordinary write error invites
  exactly that.

**The defect this exists to close, read at the line:** `p2p.Receive` builds the co-signed document
and *then* writes it back; if that write fails it returns the error
(`internal/p2p/session.go:135`) and the caller discards everything
(`internal/server/session.go:324`). **The user has signed and their machine keeps nothing** —
precisely the case `postConsentDeadline` was added to make rarer, and under resumption it turns
into "sign it again", which is the one thing a signing product must never ask. Note that the
one-way transfer path already persists what it accepts (`saveReceived`, `internal/server/session.go:338`);
the co-signing path is the one that does not.

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

**The rendezvous key is derived per hop:** HKDF over the ceremony secret and the two participating
fingerprints. It costs nothing — both ends of a hop hold all three inputs by construction — it
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
- **An invitation pins the full 32-byte fingerprint, and a ceremony built from one never derives a pin from the six-word name — driven by an invitation whose name and fingerprint disagree, which must be refused rather than resolved either way. (added 2026-08-18, D21.)**
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

### P04 — Endpoint exchange over the DHT *(in progress)*
Goal: the two sides learn each other's public endpoints, and their own, with no server. **(plan-review note, 2026-08-17, I1 — the framing still understates the blast radius, as D8's own correction records: the DHT is the signalling channel for tiers 2, 3 **and** 4, so an unreachable DHT collapses three tiers at once and leaves only LAN and manual. Carried as a Stage 2 grill target; recorded here so the next pass does not re-derive it.)** **(plan-review pin, 2026-08-18, adopted by Dan: that gate has passed — the Stage 2 grill ran on 2026-08-18 and did not take this up. Re-targeted to this phase's slice grill, with D8's pin carrying the same instruction and the discharging observation.)**
Exit criteria:
- Each side learns its own public `IP:port` and its NAT class from DHT responses alone.
- A published endpoint is retrievable by the peer computing the same key ~~from the two names~~ **from the invitation secret** *(amended 2026-08-20, P04.S03's grill — D6's amendment re-keyed this to the secret and `RecordKey`'s own comment refuses name-derivation by name; a builder satisfying the old text ships obfuscation and passes the gate)*.
- Bootstrap works from a cached node list with no hostname resolution (D6).
- A hostile or absent DHT degrades to the next tier without ever affecting which peer is accepted (L1).
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

#### P04.S05 — The L1 guard and graceful degradation *(L1, D19)*
Scope: prove the rendezvous cannot influence acceptance, and that its absence degrades rather than breaks. Refs: L1, D19.
Acceptance:
- **A hostile or absent DHT degrades to the next tier without ever affecting which peer is accepted (L1)** — driven in both directions, because a guard that only sees the absent case cannot see the hostile one.
- The degradation is the message D19 names, not a generic failure.

**Both open measurements answered at the type level** *(2026-08-19, phase-open)*, and neither forces an amendment:

- **A BEP-44 value is capped at 1000 bytes** (`bep44/item.go:132`). A candidate record of a few hundred bytes fits with room, so S03's record needs no splitting.
- **The KRPC `ip` field carries a PORT, not only an address** — `krpc.Msg.IP` is a `NodeAddr{IP net.IP; Port int}`, the BEP-42 compact 6-byte form. **Caveat 2's premise therefore holds structurally.**

*What that does NOT settle, and S02 still owes:* whether real DHT nodes actually **populate** it. Reading the type proves the library can represent a port; it does not prove one comes back. Caveat 2 is discharged on the representation and **still open on the wire**, which is a distinction worth keeping — the arc's failure-mode #1 is a "verified" claim about a dependency that was verified against documentation rather than behaviour.

~~Slices *(sketch)*: library selection and cached bootstrap; self-address probe and NAT classification; the derived rendezvous key; publish/fetch with expiry; the L1 guard.~~ *(retained; superseded by the firmed slices above.)*

#### P04.S06 — Seed nodes carried in the invitation **(added 2026-08-20 — split out of P04.S03 at its slice grill)** *(D6, D21)*
Scope: D6's second half. The invitation carries DHT seed addresses so the common case never
consults the shipped list, and a stale or blocked list is not fatal. Refs: D6, D21, D32.

*Why it is a slice and not a task of S03:* it moves a **different** wire format — D21's
invitation, not the candidate record — and it is the only invitation field that will **never**
have a signed counterpart. `MatchesRecord` compares id, roster and `signs` flags
(`invitation.go:260-281`) and the `Record` has no seeds to compare against, so seeds are
permanently unauthenticated attacker-controllable data. That earns its own gate.

Acceptance:
- A cold-start machine bootstraps from invitation-carried seeds without consulting the shipped list.
- **Seeds are parsed with `netip.ParseAddrPort` and never a resolver** — which *structurally* cannot resolve a hostname, a stronger statement of D6 than `dht_test.go`'s name blacklist. Driven by feeding it `router.bittorrent.com:6881` and asserting the refusal, because the blacklist is name-based and would pass `ParseAddrPort` silently.
- **A seed that is not a routable public unicast address is refused**, reusing `selfaddr.go`'s `reservedRanges` table rather than writing a third copy — the second one (`internal/server/fetch.go:66`) is already weaker than the first. Plus a port floor, which removes NTP/DNS/SNMP/SSDP from the target space at no cost.
- **The count is capped and an over-cap list is refused, not truncated** — silent truncation hides the attack. A bad *entry* is a transcription error and is dropped-and-counted; an oversized *list* cannot be, so it is loud.
- **Something calls the validator** — a guard on the predicate alone is P03's recorded lesson repeated (*"a guard tested a predicate and not that anything called it"*).
- **`Stats().Seeds` keeps its meaning, or gains a sibling.** Its doc says "how many **shipped** seed addresses were used" and `Bootstrapped == 0 while Seeds > 0` is the **rot alarm** for the shipped list (`dht.go:63-72`). Counting invitation seeds into it makes the alarm unable to distinguish "our list rotted" from "the invitation's seeds were bad", and `live_test.go:54-57` asserts on it.
- **The eclipse chain is answered in writing**, adopt or accept-with-reason: hostile seeds → a cold-start table containing nothing else → `probeTargets` draws every probe target from that table → the attacker controls every `ip` field → `classify`'s strict-majority rule is satisfied by two VMs in two /24s → the believed public endpoint is the attacker's choice → **D33's 3,000 punch packets are aimed at a victim of their choosing, from the user's IP**. `isPublishable` bounds *which* victim; it does not stop the chain, and a majority rule is no defence when the attacker owns the electorate.
- **A unique seed address per invitation is a read receipt**, and it defeats the reason D6 refused hostnames. DNS tells the resolver operator; an attacker-supplied literal tells the attacker directly, with the recipient's real IP and an exact timestamp, before any ceremony begins. The user is told that opening an invitation causes outbound contact to addresses **the sender chose** — a different consent question from "Nib uses the DHT", needing its own sentence.
- **Never `dht.Server.AddNode` for seeds** — a zero-id node fires `go s.Ping(...)` (`server.go:391-394`), one uncancellable goroutine per seed, and `Ping` already discards its context. `StartingNodes` is the path `Open` already uses.
- The seed list never carries the issuer's own endpoint, and is not a stable per-issuer identifier — otherwise the invitation discloses the convener's home IP to every recipient and every mail server between, and two invitations from one issuer link by their seed set.

### P05 — The ladder
Goal: tiers 2, **3** and ~~3~~ **4** exist **(renumbered 2026-08-16, D8)**, all tiers race concurrently, and the manual path is demoted.
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
- All tiers are attempted concurrently; the first to complete is used and the rest are cancelled.
- **A candidate arriving late joins the race in flight; no tier waits on another tier's gathering. (added 2026-08-16, D16)**
- **Simultaneous success on both sides converges on one channel by the lower-fingerprint rule, driven by forcing the glare rather than waiting to observe it. (added 2026-08-16, D17)**
- **A same-role pair stops on the surviving channel before any verification string is derived; no document byte and no session-derived word exists at that point. (added 2026-08-16, D17)**
- **Nothing in the race emits at full rate for the whole deadline: retry cadences step down, and a published record outlives the race that depends on it. (added 2026-08-16, D16)**
- **Losing the channel before confirmation re-races and re-confirms; losing it after confirmation ~~fails the ceremony~~ **restarts the hop and re-delivers rather than re-signs (amended 2026-08-18, D18, D24)**. Both are driven. (added 2026-08-16, D18)**
- **The armed listener's wait is bounded by the ceremony, not by a five-minute constant, and still accepts exactly one pinned peer and serves exactly one session. (added 2026-08-18, D16 amendment.)** `sessionAcceptTimeout` is 5 min today (`internal/server/session.go:34`), which disarms a party waiting their turn; this is the only bullet in the plan that moves the TRIPWIRE (`internal/server/session.go:24`), and the two clauses it must *not* move are named in it rather than left implied.
- **The three clocks are independent: letting the connect deadline elapse in full leaves both the exchange budget and the ceremony deadline undiminished. (added 2026-08-18, D16 amendment — extends the 2026-08-17 two-clock guard to the third.)**
- **The rendezvous key is derived per hop: two hops of one ceremony publish under different keys, and a party cannot read the candidates of a hop it is not in. (added 2026-08-18, D30.)** **(2026-08-20, P04.S03's grill: the second clause was UNSATISFIABLE against shipped code and is now buildable.** Per-hop *addressing* shipped at P01.S07; per-hop *encryption* did not — `RecordKey()` was `derive("nib-record-v1", 32)` with no hop, so one key decrypted every hop and every roster member holds it. S03 makes `RecordKey` per hop. Note what this still does **not** buy, so the clause is not over-read: it excludes a party from a hop's *ciphertext*, and every roster member can still derive any hop's key from the secret. D30's motivating harm — "every party can read every other party's IP addresses" — is bounded by the roster, not below it.)** Driven with a three-party record — a two-party ceremony has exactly one hop and cannot distinguish a per-hop key from a per-ceremony one, so the obvious test is the vacuous one.
- **The race and the glare tie-break are scoped to the current hop: a convener holding candidates for a later party never dials them during this hop. (added 2026-08-18, D30.)**
- ~~Both ends behind carrier-grade NAT fails with an explanation that names the fallback, not a generic timeout **— and the fallback it names is the one that actually applies: a shared VPN or a manual address one side can accept, not a port-forward the carrier's NAT forbids (amended 2026-08-16, D9 pin)**.~~ **Each of D19's four causes produces its own message, and the mapping-class test distinguishes the two NAT classes from two DHT observations. Cause 3's message names port mapping and a shared VPN — never a port-forward the carrier's NAT forbids. (superseded 2026-08-16, D19)**

Slices *(sketch)*: candidate gathering **and the trickle-in race with its two clocks (D16)**; concurrent attempt and cancellation **including the glare tie-break (D17)**; IPv6 tier; **the port-mapping client (PCP → NAT-PMP → UPnP-IGD) and its licence notice; the mapping lease lifecycle and teardown-on-every-path; the armed-only disclosure line in the ceremony screen (D15);** IPv4 punch with keepalives **and symmetric retransmit (D17)**; ~~the CGNAT diagnosis and message~~ **the mapping-class probe and D19's four-cause diagnosis**; **channel-loss behaviour either side of the confirmation gate (D18); the TCP dialer wired into every dialable tier (D14);** manual path demoted to advanced.

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

Slices *(sketch)*: the ceremony panel replacing the tab and its three modals; convene-and-invite; the connect-and-confirm screen; the roster and position display; **the roster-shaped consent screen (D27); the end-state surfaces (D28); the document freeze and the attachments label (D29);** failure surfaces; docs and README.

### P07 — More than two parties: **the model** **(added 2026-08-18 — D22, D23, D25, D27; amended the same day — D28, D29; built BEFORE P06 — Stage 2 grill; **split 2026-08-18, plan-review: lifecycle and delivery moved to P08**)**
Goal: a ceremony of N parties completes as a convener-driven serial relay, survives being
interrupted at any hop, and cannot be signed out of order. Last, because it needs the record
(P01), a working transport (P02), a working ladder (P05) and a roster-shaped surface (P06); the
artifact model it rides on already carries no two-party assumption (D2 pin).

Exit criteria:
- **A four-party ceremony completes, every signature valid, every attestation sharing one `[NibRoster:…]` commitment, and `crossBind` matching each signer against the others. (D2 pin, D22.)**
- **A non-signing convener completes a ceremony: the finished document carries the signers' signatures and none of the convener's. (D22, Dan's call.)**
- **The signature blocks of a nine-party ceremony are all on the page and none overlaps the readme body — driven by rendering and measuring, not by asserting a rect. (D25.)** The measurement is the point: the defect this closes is invisible to every assertion about placement arithmetic, because the arithmetic is what is wrong.
- **A ceremony cannot grow a party after the first signature: the attempt is refused with a distinct message naming a new ceremony as the answer. (D25 — pages are allocated up front.)**
- **A contribution offered by the wrong party, or onto a document with the wrong prefix, is refused by the L3 guard in Go with a named error — driven for both cases separately, and with the UI bypassed. (D23, L3.)** A test that drives it through the panel proves the panel, not the law.
- **Every criterion in this phase is driven by the multi-instance harness (D26), not by hand. (added 2026-08-18.)** A four-party ceremony verified once, manually, at the phase gate is the shape of check this repo has already been burned by.
- **A non-signing convener completes a ceremony over the carry route, and the finished document contains no signature of theirs. (added 2026-08-18, D22 pin.)** Driven — and note it cannot pass on `Initiate`, which demands the caller's own signature back, so a green here is evidence the third route exists.
- **The appended trust page describes a ceremony of N, the About dialog says the same thing, and the drift guard is green. (added 2026-08-18, D27.)** The guard (`internal/p2p/readme_test.go:61`) is what makes this one criterion rather than two that can drift.
- **A nine-party document's visible blocks and signature details each name the ceremony rather than one neighbour. (added 2026-08-18, D27.)**
- Documentation and README updated in the same phase (STANDARDS docs-parity).
- **Every row of `<project-memory>/instruments/ceremony.md` carries a disposition — `keep-live` / `gated` / `deleted` — filled at this phase's close. (added 2026-08-18, verification pack V8.)** An inventory whose disposition column was never filled is a record of intentions; 224 such rows accumulated in another project before anyone noticed. A row whose reader is a standing criterion is never silenced.
- **Each of D33's four numbers is enforced on the externally-supplied path, driven by supplying a value past the bound. (added 2026-08-18, crypto pack PLAN-4.)** A test that supplies a value *inside* the bound cannot see an unenforced parameter.
- **A version skew produces a sentence naming the mismatch, not a parse error — driven for the record, the invitation and the ceremony protocol separately. (added 2026-08-18, D32.)**
- **A four-party ceremony with a NON-SIGNING convener completes, and every signature on the finished document reports `Matched`. (added 2026-08-18, plan-review, D22/D2 pin.)** The "every attestation shares one `[NibRoster:…]` commitment" bullet is satisfied perfectly by a document on which no signature matches any other — this is the clause that sees the hub breaking the channel binding.
- **Every signature block on a completed ceremony carries the record's intent verbatim. (added 2026-08-18, plan-review, D20 intent pin.)** None of the record round-trip bullets ever reads a signature block.
- **A completed ceremony whose convener has `signs:false` renders as complete, not as missing a signer. (added 2026-08-18, plan-review, D27 pin.)** The "no signature of theirs" bullet is a fact about the document and says nothing about what the verifier reports.

Slices *(sketch)*: the roster-prefix contribution gate and the L3 guard; `coSignExchange` re-based off `len(ats) != 1` **and off the wire peer (D22/D2 pin)**; the carry route for a non-signing convener (D22 pin); the roster-shaped `Confirmer` (D27); the readme and About rewrite behind the drift guard (D27); the placement policy and page allocation; the record's intent populating every block (D20 pin).

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
- **A hop interrupted after its signature exists re-delivers on resumption and never re-signs; the finished document carries exactly one block per signer. (D24.)** Driven by killing the process between the signature and the write — the case `internal/p2p/session.go:135` discards today.
- **The contribution write is atomic: a process killed mid-write leaves the previous state or the complete contribution, never a prefix. (gap #15.)** The re-delivery criterion cannot see this — it never writes twice.
- **A disk-full persist reports "signed but not saved" and does not attempt delivery. (gap #17.)**
- **A ceremony resumed in a fresh process — with other documents opened first, so the id counter has advanced — acts on its own document and refuses a decoy holding the id it used to have. (D29 identity pin.)** The resumption bullet alone passes with a dangling id, because nothing in it opens a second document.
- **A party who arms and waits through three earlier hops is still armed when the baton arrives. (D16 amendment.)**
- **Each of D28's end states is driven at the protocol level, not only in the UI: a decline at hop 3 ends the ceremony, and the parties who already signed learn of it. (D28.)**
- **An identity that no longer matches the roster ends the ceremony with a named message and never pairs on the new key. (D28 — the L1 guard covers it.)**
- **The finished document reaches every party, including those whose hop completed hours earlier. (D22 delivery round.)**
- **A four-party ceremony's delivery round reaches every party, and their invitation pins are absent afterwards — in that order. (D29 lifecycle pin.)** Neither the delivery bullet nor the pin-scoping bullet can produce this alone; the defect is that both are satisfiable while delivery fails.
- **A delivery round re-run after a mid-round failure leaves exactly one file per party. (D22 pin.)** "The finished document reaches every party" is satisfied completely by a round that delivers twice.
- **A ceremony directory is gone after the ceremony has ended and its document has been delivered or saved. (D29.)**
- **A ceremony whose mirror directory has been deleted by hand degrades that panel entry and leaves every other ceremony and open document working. (gap #19.)**
- **Convening a second ceremony on a document already under a live one is refused, server-side and by name. (gap #28.)** The mutation criterion cannot see it, because convening is not a mutation.
- **An invitation re-issued to one party mid-ceremony completes, with every other party's state untouched. (gap #24.)** The convene criteria only ever issue once.
- **Every criterion in this phase is driven by the multi-instance harness (D26), not by hand.**
- Documentation and README updated in the same phase (STANDARDS docs-parity).

Slices *(sketch)*: hop persistence, the atomic write and the disk-full path; resumption in a fresh process; the ceremony-scoped arm window; the end-state machine; the delivery round and its per-party keying; the prune, the scoped pins and the convene refusal.

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
