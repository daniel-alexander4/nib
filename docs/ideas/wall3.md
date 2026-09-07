# Wall3 — a serverless substrate for home machines that serve

**Status:** idea. Parked in the nib repo for convenience; **this is not nib work** and nothing in
nib depends on it. One file in one directory so `git mv` moves it out whole when it graduates.

**Provenance:** 2026-09-02, out of a conversation that started as "how do two disparate machines
find each other" and ended at "can I run a web or SMTP server on my home laptop."

**Why the name.** Three walls stand between a home machine and serving. Two are engineering — the
substrate is arranged-rendezvous-shaped rather than server-shaped, and unmodified clients don't
speak the protocol. The third is policy: CA issuance, IP reputation, port-25 blocking, and the
DNS-shaped assumptions welded into existing protocols. **The project is named after the wall it
cannot knock down**, because that is the honest scope boundary and naming it after the easy walls
would invite the wrong roadmap.

---

## The idea in one paragraph

NAT took away the property that every host on the early internet had: a real address, no gatekeeper,
and inbound connections that just worked. Nib rebuilt the missing piece incidentally — unguessable
rendezvous, cryptographic identity, and a NAT-traversal ladder, with no server anywhere in the
path — in order to get two people signing a PDF. Wall3 asks whether that substrate, generalised and
lifted to the IP layer, gives any home machine back the ability to *serve*: not a document to a
known peer, but a socket to whoever is allowed to arrive.

## The thesis, and it is a claim about an unoccupied position

Existing systems each pick two of three:

- **Tailscale / ZeroTier** — direct connections, kernel-level IP, **coordination server**.
- **Yggdrasil / cjdns** — no coordination server, kernel-level IP, but packets are **routed through
  the mesh**, so traffic transits strangers' machines and pays for it in latency and trust.
- **libp2p** — no coordination server, direct connections, but it is a **library**, not an IP
  substrate: applications must be rewritten against it.

**Wall3's position is the fourth corner: serverless coordination, direct transport, kernel-level
IP.** The DHT carries *signalling only* and never traffic; connections are peer-to-peer; and a TUN
device makes it work for applications nobody rewrote.

The second unusual choice is the rendezvous itself: **borrow the public BitTorrent DHT rather than
bootstrap a new network.** libp2p runs its own DHT and Yggdrasil its own mesh, each with the
cold-start problem that implies. A ten-million-node network already exists and will store a signed,
expiring, opaque blob for anyone. That is leverage — and a dependency, and at scale a civic
question. See Risks.

## Scope — what this is NOT

Written first, because every project in this space dies by accretion.

- **Not a DNS replacement.** No global namespace, no human-memorable names, no resolution by
  strangers. Zooko's triangle is refused, not solved.
- **Not browser HTTPS.** No CA will issue for a key-derived address and no browser trusts a pinned
  SPKI. `.onion` needed a CA/B Forum carve-out and years of governance to get there. Out of scope,
  permanently, unless someone else wins that fight.
- **Not email.** SMTP is the most DNS-welded protocol there is — MX is DNS, SPF/DKIM/DMARC are
  domain-keyed, residential IPs sit on Spamhaus's PBL by default, and most ISPs block outbound 25.
  A perfect substrate changes none of it. SMTP *between* Wall3 peers is trivial and is a private
  mail network, not email.
- **Not an anonymity network.** Direct connections mean both ends see each other's addresses. Tor
  exists and this is not it.
- **Not a replacement for hosting.** A laptop sleeps. See M3.

## The three walls, precisely

### Wall 1 — the substrate is ceremony-shaped

Three concrete gaps, all fixable, none free:

1. **One pinned peer per listener.** `SessionTLS(identityCertPEM, identityKeyPEM, pinnedSPKI,
   server)` takes exactly one SPKI, and its own doc records that "the same verification runs whether
   this side dials or listens" (`internal/p2p/transport.go:58`). The listener accepts one identity
   and refuses everyone else. A server is many-to-one — authenticate *me* to anyone, accept anyone —
   which needs a server-auth-only config with anonymous or separately-authorised clients. **This
   changes the security model**, not just a struct: "I know exactly who connected" stops being a
   structural guarantee and becomes an application-layer question.
2. **Presence is an arm window, not a residency.** The rendezvous record lives for the life of a
   ceremony arm. A server must be continuously findable, which means republishing every
   `candidateLife/2` forever rather than for a bounded window.
3. **No relay floor.** A named search — `grep -rniE 'turn server|derp|relay'` across `internal/p2p`,
   `internal/server`, `internal/rendezvous`, minus tests and minus the ceremony carry route —
   returns nothing. When the punch fails, the race fails. Tailscale ships DERP precisely because
   punching is not reliable enough alone; symmetric-NAT and some CGNAT deployments simply do not
   punch.

### Wall 2 — the client must speak it

Three shapes, and only one is right:

- **Local proxy** (SOCKS / PAC to a 127.0.0.1 daemon) — how Tor Browser and the IPFS gateway work.
  Fine for HTTP, per-application config, useless for anything that isn't proxy-aware.
- **TUN device — the answer.** Synthesize an address per peer key, install a route, let the kernel
  do the rest. Every application works unmodified. **Derive the address from the public key** as
  cjdns (`fc00::/8`) and Yggdrasil (`200::/7`) do, and the address *is* the pin — the identity model
  expressed as something the kernel can route.
- **Public gateway** — reintroduces a server. Defeats the exercise.

### Wall 3 — policy, reputation, and trust anchors

Not solvable by this project, enumerated so nobody tries: CA issuance for non-DNS identities;
residential IP reputation; ISP port blocking; and every protocol whose federation model presumes DNS.

## What it inherits, and what needs generalising

Read from nib at 2026-09-02; all of it would move to a standalone library.

| Inherit as-is | Generalise |
| --- | --- |
| Candidate model and concurrent racing (`internal/server/discover.go:75-97`) — ICE's shape, already built | Server-mode TLS (Wall 1.1) |
| Link-local multicast discovery (`internal/discovery/mcast.go:37-41`) | Permanent presence (Wall 1.2) |
| Reflexive address discovery on the session's own socket (caveat 7) | Relay floor (Wall 1.3) |
| Port mapping PCP → NAT-PMP → UPnP (`internal/portmap/`) — note IPv4-only, deliberately (`gateway_linux.go:18`) | TUN + key-derived v6 addressing |
| Simultaneous-open punch with a budget (`internal/server/punch.go:14`) | Public rendezvous keys (today the DHT key is HKDF over a shared secret — unguessable by design) |
| Bootstrap cache, seeds only on cold start (`internal/rendezvous/dht.go:313`) | |
| **L1** — the rendezvous package structurally cannot import identity code, enforced by `l1_test.go` | |

**L1 is the piece most worth carrying forward.** libp2p and ICE both rely on a later authentication
step to catch a compromised discovery layer; making it unrepresentable at the package boundary is
stronger and costs nothing.

## Milestones

- **M0 — the null experiment.** Publish a rendezvous record under a *public* key from one machine;
  fetch and dial it from another network with no shared secret. Settles whether the traversal path
  has any hidden dependency on the secret beyond addressing. Reuses the `nib rendezvous` diagnostic.
  An afternoon.
- **M1 — server mode.** Server-auth-only TLS, permanent presence, accept from unknown peers. At this
  point two machines running the software can serve each other arbitrary TCP.
- **M2 — the relay floor.** A volunteer relay protocol for the punch-failure slice. Without it the
  system is down for a population it cannot identify or explain to.
- **M3 — availability.** A laptop sleeps, and a stranger arriving at 03:00 gets nothing and no
  explanation. Reciprocal caching pools — you hold encrypted slices for *n* peers and they hold
  yours — turn "always on" from a hardware requirement into a statistical one. **This is the
  milestone that decides whether Wall3 is a demo or a service**, and it is the one most likely to be
  postponed for being unglamorous.
- **M4 — TUN.** Key-derived v6 addresses, a route, and unmodified applications.

Ordering is deliberate: M0 is cheap and could kill the project; M3 is the one that matters and is
placed before M4 so the exciting milestone cannot be reached by skipping the necessary one.

## Open questions

1. Does the punch ladder work with **no prior arrangement** — a cold client arriving at a server
   that has never heard of it? Simultaneous open needs both sides active at once; today a signalling
   round arranges that. A server must instead maintain readiness continuously, which may be a
   different mechanism rather than a longer timer.
2. What does **permanent DHT presence cost**, and does it constitute abuse of a network we are
   borrowing? Unmeasured — and per the repo's own law, a cost claim is settled by running it.
3. Is **IPv6 enough to skip M2**? With v6 there is no translation, so punching becomes opening a
   firewall pinhole, which PCP does as a first-class operation. If v6 coverage on both ends is high
   enough, the relay floor may be a v4 legacy concern.
4. What is the **authorisation model** once the listener accepts strangers? "Anyone" is a policy, not
   an absence of one, and it needs to be expressible.
5. Does borrowing the BitTorrent DHT survive **contact with the operators of the BitTorrent DHT**?

## Risks and kill criteria

- **Prior art may already be sufficient.** If Yggdrasil's mesh routing is fast enough in practice,
  the fourth-corner argument collapses and the right move is to contribute there instead. **Measure
  before building**: this is the single most likely way the project should end.
- **The BitTorrent DHT is somebody else's infrastructure.** Nib's use is bounded and rare; a
  serving substrate's is continuous and permanent. If the answer to open question 2 is "this is
  abuse", the project needs its own DHT and inherits the cold-start problem it was avoiding.
- **Punch success rates are a field number, not a reading.** Unmeasured here.
- **Availability may be unsolvable at home.** If M3's reciprocal pool cannot be made free-rider-proof
  without identity cost — and a fresh keypair is free — then home machines cannot serve reliably and
  the honest conclusion is that this is a peer-to-peer transport, not a hosting substrate.

## What would make this obviously worth doing

One person, one laptop, no account with anyone, serving something another person can reach from
across the world — with nothing in the middle that can be subpoenaed, acquired, or shut off. That
was ordinary in 1995 and it is now the exclusive property of people who rent infrastructure. The
substrate to get it back mostly exists; what it has never had is someone assembling it at the IP
layer without a company in the path.
