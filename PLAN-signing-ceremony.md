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
SME packs: **crypto (core tier)** — `go.mod` declares `filippo.io/age`,
`golang.org/x/crypto`, `edwards25519`, `hpke`, `go-pkcs12`, `digitorus/pdfsign`
(inferred, trigger 1); the consensus tier does not fire — two sequential
single signers, no aggregate or threshold dependency. **verification** — declared
by the sibling plan in this repo (`PLAN.md:17`), corroborated by
`mdpdf/coverage.go`'s `Unsupported()` capability oracle. Both declare `plan` in
`objects`.

**Where this plan and the original brief differ, the plan wins.**

**Sibling plan:** this repo currently carries a second, unrelated feature plan —
`PLAN.md`, "Multiple open documents in Nib", in flight with P01.S01 shipped at
v1.102.4. The two are independent and neither supersedes the other. `/createcode`
must be told which plan it is walking. This split is deliberate (D1) and ends when
one of the two retires into CLAUDE.md + ADRs per STANDARDS §15.6.

This plan covers **one feature project inside the existing `nib` repo**: replacing
the current **Collaboration** process with a **Signing Ceremony** — two people who
have never connected before, both running Nib, co-sign a document with no port
forwarding, no VPN, no rendezvous server, and one short spoken name in place of a
64-character fingerprint. It is not a plan for nib as a whole.

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

Two laws govern every decision below. They are stated here, not derived later,
because both exist to make an untrusted discovery layer safe to depend on.

**L1 — The rendezvous path affects reachability only, never identity.** Nothing
learned from the DHT, from multicast, or from any other network source may
influence *which* peer is accepted. Acceptance is decided solely by the pinned
fingerprint (`internal/p2p/transport.go:83`) and the verification string. A
rendezvous layer that lies can waste time; it can never substitute a signer.

**L2 — No silent downgrade.** Any path that would complete a ceremony at less than
full assurance must fail closed rather than proceed quietly. A shortened name that
lowers the pin's effective strength without the user knowing is exactly the defect
this law exists to forbid.

Both laws get guard tests, named at slice-grill time.

---

## Decisions

### D1 — Scope, topology, plan artifact, and license *(settled 2026-08-15 via /discuss, auto-adopted)*

A feature project inside the existing `nib` repo. No new repository, no new Go
module, no new binary. The plan artifact is this file, `PLAN-signing-ceremony.md`,
kept beside the sibling `PLAN.md` with a cross-reference in each. License unchanged:
**AGPLv3**. The DHT dependency is MPL-2.0, which is AGPL-compatible (MPL 2.0 §3.3
permits distribution under a Secondary License, and AGPLv3 is named as one) — so
the dependency creates no licensing conflict.

*Why:* a separate module would fragment one product; nib is mature (v1.102.4) with
its own CLAUDE.md and release machinery, and this feature is one surface within it.

### D2 — The existing co-signing cryptography is retained unchanged *(settled 2026-08-15 via /discuss, auto-adopted)*

Untouched by this plan: pinned-peer mTLS with SPKI-hash pinning
(`internal/p2p/transport.go:42`), ephemeral per-session leaf certificates so the
document-signing key never reaches the TLS layer (`:119`), attestation channel
binding (`internal/p2p/session.go:205`), and mutual co-signature verification with
the prefix-extension replay bound (`:83`).

*Why:* it is built, it is sound, and it is orthogonal to pairing. Re-planning it
would be scope creep against a surface that already survived review.

### D3 — The pairing name is derived from the identity fingerprint, not randomly generated *(settled 2026-08-15 via /discuss, auto-adopted)*

At first open Nib derives the user's name from the SHA-256 SPKI fingerprint it
already computes. The name is an *encoding* of the pin, not a label attached to it.

*Why:* it collapses two exchanges into one. A random name would still require the
fingerprint to be exchanged separately for pinning — which is the exact friction
this feature exists to remove. Because the name commits to the key, typing a name
*is* pinning a peer.

### D4 — Six words, and the verification string is mandatory *(settled 2026-08-15 via /discuss — Dan)*

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

### D5 — The name does not rotate *(settled 2026-08-15 via /discuss, auto-adopted)*

One name per identity, stable for the life of the key.

*Why:* the fingerprint is already a stable public identifier that Nib displays and
users share (`internal/server/peers.go:31`), so a derived name adds no linkability
that does not already exist. Rotation would break repeat counterparties for no gain.

### D6 — Endpoint carrier: the BitTorrent DHT, with cached bootstrap *(settled 2026-08-15 via /discuss — Dan)*

Endpoints are published to and fetched from the BitTorrent DHT under a key derived
from the two names. Bootstrapping uses a **cached node list**, populated on first
contact — not hardcoded bootstrap hostnames.

*Why Dan's call:* the DHT is not centralised, but it is a peer-discovery system
being used for peer discovery, which brushes the "nothing designed for the purpose"
constraint. That is a judgment about the constraint's intent.

*Why cached bootstrap is not optional:* resolving hardcoded bootstrap hostnames is a
centralised touchpoint and would be the one place a rendezvous could be cut off.
It must be designed in, not left to a library default.

*Alternatives rejected:* DNS cache-timing (anycast resolvers mean the two parties
may read different caches — a silent failure); Bitcoin `addr` gossip (propagation in
hours, far too slow for a live ceremony).

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

### D11 — The two repo laws *(settled 2026-08-15 via /discuss, auto-adopted)*

L1 (reachability, never identity) and L2 (no silent downgrade), as stated above.
Both become named guard tests.

*Why they are decisions and not commentary:* L1 is what makes depending on an
untrusted discovery layer defensible at all, and L2 is what makes D4's mandate
enforceable rather than aspirational.

### D12 — External gate: an outside cryptographic review before the ceremony ships *(settled 2026-08-15 via /discuss, auto-adopted)*

The feature does not ship to users until an outside reviewer has read the pairing
and verification design. Named here as a phase gate, not a footnote.

*Why:* this changes an authentication surface on a product whose output has legal
weight, and the planning playbook asks for external gates to be named at Stage 1.

### D13 — The UI surface is renamed Collaborate → Signing Ceremony *(settled 2026-08-15 — Dan's instruction)*

The mode tab (`web/index.html:30`) and the role-picker flow (`:257`) are renamed and
restructured around the new process.

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

---

## Build order

### P00 — Bootstrap *(pre-satisfied — see D1)*
nib is a mature repo with VERSION, CLAUDE.md, release machinery and git history.
Stage 8 scaffolding would write `0.1.0` over a shipping product. Recorded as
pre-satisfied, exactly as the sibling plan records it.

### P01 — Pairing identity: the name and the verification string
Goal: replace the 64-hex exchange with a six-word name, and establish the
verification string as a mandatory gate — both in one phase, because shipping the
shortened name without the spoken check would be the silent downgrade L2 forbids.
Connectivity is untouched; this phase still runs over the manual address.

Exit criteria:
- A user never sees a hex fingerprint in the normal pairing path, and never types one.
- A ceremony cannot reach the signing exchange until both sides confirm the verification string.
- Peers pinned by hex before this phase still work (D10), proven by a test that pins the old way and connects the new way.
- The name↔fingerprint encoding round-trips on a fixed vector corpus.

#### P01.S01 — The wordlist and the encoding
Scope: fingerprint → six words → fingerprint bits, one package, no UI. Refs: D3, D4.
Acceptance:
- Round-trip holds for a corpus of fixed vectors, including all-zero and all-ones fingerprints.
- Decoding rejects a wrong-length phrase, an out-of-list word, and a transposition, each with a distinct error.
- The wordlist's licence is recorded in THIRD-PARTY-NOTICES.md if it carries one.
Tasks: *(written at slice-grill time)*

#### P01.S02 — Show the user their own name
Scope: the peers payload and the UI display the name; hex moves to a secondary position. Refs: D3, D5.
Acceptance:
- `peersPayload` returns the name alongside the existing fingerprint; nothing existing is removed.
- The name shown is derived from the live identity, not stored — deriving twice yields the same words.
Tasks: *(written at slice-grill time)*

#### P01.S03 — Accept a name wherever a fingerprint is accepted
Scope: `parseFingerprint`'s callers accept a six-word phrase and resolve it to a pin. Refs: D3, D10.
Acceptance:
- Pinning by name and pinning by the equivalent hex produce a byte-identical stored pin.
- A name that decodes to a fingerprint no peer presents fails at the handshake, not at pin time — L1 holds: nothing about reachability decided identity.
Tasks: *(written at slice-grill time)*

#### P01.S04 — The verification string
Scope: derive four words from the completed session and display them on both sides. Refs: D4.
Acceptance:
- Both endpoints of one session derive identical words.
- Two sessions between the same pair derive different words.
- A test that substitutes a different peer identity produces different words on the two sides — the man-in-the-middle case, driven rather than asserted.
Tasks: *(written at slice-grill time)*

#### P01.S05 — Make it mandatory
Scope: the ceremony fails closed until both sides confirm. Refs: D4, D11 (L2).
Acceptance:
- No document bytes cross the wire before both confirmations are recorded.
- Declining, or timing out, ends the session with a distinct, user-legible outcome — not the same error as a network failure.
- A guard test named for L2 fails if any path reaches the signing exchange unconfirmed.
Tasks: *(written at slice-grill time)*

### P02 — QUIC session transport **(TCP retained beside it — amended 2026-08-16, D14)**
Goal: ~~re-base `Dial`/`Listen` onto QUIC~~ **give the session a QUIC path beside its existing TCP one (2026-08-16)**, with `SessionTLS()` reused unchanged, still over the manual address, so the transport change is proven in isolation before any discovery depends on it.
Exit criteria:
- A full ceremony completes over QUIC between two machines using the manual address.
- **A full ceremony still completes over TCP between the same two machines, after the QUIC path exists. (added 2026-08-16, D14)**
- The pinned-peer callback demonstrably rejects a non-pinned peer under QUIC, driven red.
- ~~`Initiate`, `Receive` and `coSignExchange` are unchanged.~~ **`coSignExchange` is unchanged; `Initiate`, `Receive`, `SendDocument` and `ReceiveDocument` are re-typed off `*tls.Conn` to a stream plus an already-verified fingerprint, and one set of session-logic tests runs green over both transports. (superseded 2026-08-16 — the original criterion was unmeetable: those four are typed to `*tls.Conn` today, see the D7 pin.)**

Slices *(sketch)*: library selection and a spike proving `VerifyPeerCertificate` fires as under `crypto/tls`; the session core re-typed off `*tls.Conn` **(D14)**; QUIC `Dial`/`Listen` added behind that core; the pinned-rejection test ported; ~~the TCP path removed~~ **the TCP dialer kept as a peer behind the same core, with the session-logic tests parameterised over both (2026-08-16, D14)**.

### P03 — Local discovery (tier 1)
Goal: two Nibs on the same network find each other with no address typed and no internet.
Exit criteria:
- A ceremony completes on a LAN with no address entered anywhere and no outbound internet traffic.
- Discovery announces the name's public bits only — never anything that could influence which peer is accepted (L1).
- Behaviour on Windows is verified on Windows, not inferred.

Slices *(sketch)*: multicast announce/browse; resolving a discovered peer to a candidate; the L1 guard; the Windows pass.

### P04 — Endpoint exchange over the DHT
Goal: the two sides learn each other's public endpoints, and their own, with no server.
Exit criteria:
- Each side learns its own public `IP:port` and its NAT class from DHT responses alone.
- A published endpoint is retrievable by the peer computing the same key from the two names.
- Bootstrap works from a cached node list with no hostname resolution (D6).
- A hostile or absent DHT degrades to the next tier without ever affecting which peer is accepted (L1).

Slices *(sketch)*: library selection and cached bootstrap; self-address probe and NAT classification; the derived rendezvous key; publish/fetch with expiry; the L1 guard.

### P05 — The ladder
Goal: tiers 2, **3** and ~~3~~ **4** exist **(renumbered 2026-08-16, D8)**, all tiers race concurrently, and the manual path is demoted.
Exit criteria:
- IPv6-to-IPv6 completes with neither side forwarding a port.
- IPv4-to-IPv4 completes through at least one endpoint-independent NAT.
- **A ceremony completes with both ends behind endpoint-*dependent* NAT when exactly one side obtains a port mapping — the case tier 4 cannot serve. (added 2026-08-16, D15)**
- **A mapping is never held while no ceremony is armed, and is gone from the router after teardown, cancel and crash alike — driven, not asserted. (added 2026-08-16, D15)**
- **Every tier that ends in a dialable address completes over TCP as well as QUIC, proven with UDP blocked. (added 2026-08-16, D14)**
- All tiers are attempted concurrently; the first to complete is used and the rest are cancelled.
- Both ends behind carrier-grade NAT fails with an explanation that names the fallback, not a generic timeout **— and the fallback it names is the one that actually applies: a shared VPN or a manual address one side can accept, not a port-forward the carrier's NAT forbids (amended 2026-08-16, D9 pin)**.

Slices *(sketch)*: candidate gathering; concurrent attempt and cancellation; IPv6 tier; **the port-mapping client (PCP → NAT-PMP → UPnP-IGD) and its licence notice; the mapping lease lifecycle and teardown-on-every-path; the armed-only disclosure line in the ceremony screen (D15);** IPv4 punch with keepalives; the CGNAT diagnosis and message; **the TCP dialer wired into every dialable tier (D14);** manual path demoted to advanced.

### P06 — The Signing Ceremony surface
Goal: the Collaborate tab becomes the Signing Ceremony, restructured around name-in, connect, confirm, sign.
Exit criteria:
- The primary flow contains no address field and no hex fingerprint.
- Every failure tier has a distinct, actionable message.
- **While a ceremony is armed, the screen discloses that a temporary router opening was requested and names the port; when no mapping was obtained it says so rather than staying silent. (added 2026-08-16, D15)**
- The advanced fallback is reachable but never on the default path.
- Documentation and README updated in the same phase (STANDARDS docs-parity).

Slices *(sketch)*: the renamed tab and flow; the connect-and-confirm screen; failure surfaces; docs and README.

---

## Out of scope

- **Legal, GTM, and marketing-site work** — handled after the product works, by their own skills.
- **Group ceremonies (more than two parties).** The attestation model is two-party today (`coSignExchange` requires exactly one prior signer, `session.go:207`). Widening it is a separate project.
- **A relay for the carrier-grade-NAT case.** Excluded by the constraints; the manual path (D9) is the answer instead.
- **Changing the signature or attestation format.** D2.
- **The multiple-open-documents feature** — that is `PLAN.md`, independent of this.
- **Mobile or web clients.** Nib is a desktop app with a loopback UI.

## Standing caveats

Load-bearing claims not yet verified. Each is a Stage 2 grill target, and the arc's
own failure-mode #1 is a "verified" claim about a dependency that was never
re-verified.

1. **The QUIC library invokes `VerifyPeerCertificate` exactly as `crypto/tls` does**, with `InsecureSkipVerify` set and `RequireAnyClientCert` honoured. The entire pinned-peer model rides on it. If false, D7 needs rework, not adjustment.
2. **DHT responses carry the requester's port, not only its IP.** P04's self-address probe depends on it; without the port, IPv4 punching loses its input.
3. **Multicast discovery behaves on Windows** as it does on Linux. Recent releases show Windows-specific paths needing their own handling (`v1.101.0`, `v1.102.1`).
4. **A suitable wordlist exists with a licence compatible with AGPLv3 distribution**, and with phonetic distinctness good enough to read over a phone.
5. **~66 bits is the intended floor** given D4's mandatory verification. If the verification step is ever weakened, this number becomes the whole security of the pairing.
6. **A Go port-mapping library covering PCP, NAT-PMP and UPnP-IGD exists under a licence compatible with AGPLv3 distribution** (D15). If only some protocols are covered, the tier still ships — with narrower router coverage, recorded rather than assumed. *(added 2026-08-16)*
7. **The mapped port, the DHT self-address probe and the live session must all be the same local socket.** A NAT mapping — learned or requested — is a function of the *internal* `IP:port`, so a mapping obtained on the DHT socket or on a throwaway socket is useless for a session that listens elsewhere, even under a perfectly endpoint-independent NAT. This constrains library selection in **both P02 and P04**: the QUIC library must accept an existing `net.PacketConn` and the DHT must be willing to share it. Load-bearing for tiers 3 and 4 alike, and not currently reflected in either phase's slice sketch. *(added 2026-08-16)*
8. **Carrier-side PCP deployment is not assumed.** RFC 6887 was specified with carrier-grade NAT in mind, but whether carriers actually answer PCP is unverified and, on present evidence, mostly no. The CGNAT case stays D9's until measured. *(added 2026-08-16)*

## Bookkeeping

- Amendments follow the house mechanics: a dateline clause per pass, tagged pins, strike-and-supersede. No silent rewrites.
- Every amendment is a commit with a patch bump per this repo's CLAUDE.md.
- `/createcode` must be told it is walking *this* plan and not `PLAN.md`.
- Residual doubts go to the `/pending` memory lists, not to chat.
