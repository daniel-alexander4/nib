# ADR-061 — the DHT keeps, and queries, only what the node-cache rule admits

**Status:** accepted. /pending 707 (v1.169.12, `7188f392`) and /pending 743. Extends ADR-011: the local link gets
its window before anything leaves the machine, and nothing a stranger says may aim what does leave it.

## Context

The node cache (`~/.config/nib/…/dht-nodes`) is where every later run's bootstrap starts. anacrolix adds any node
that sends us a query to the routing table (`server.go:492`), BEP-42 exempts loopback and private sources, and
`saveNodes` wrote the table OVER the cache. Under ADR-011's lazy bootstrap the ordinary run is Open, one inbound
query, Close — measured: a 200-node cache persisted as the one stranger who pinged us, every later run started
from that address with `Seeds` 0, and the shipped list's rot alarm could not fire. The same openness reached
traversal: a hostile public response could list private or loopback addresses, and `TraversalNodeFilter` sent KRPC
to them (`IPBlocklist` unset) — a stranger aiming this machine's queries at its own LAN.

## Decision

**One address rule, `Server.scope`, governs what the cache reads, what it writes, and where a query may go.** In
production it is `addrscope.Seed` — the rule invitation seeds and `sampleSeeds` already apply (dialable, port
floor, no private, loopback or link-local space); `OpenAdmittingLoopback` widens it by loopback for hermetic tests
only, and `TestOnlyTestsOpenAdmittingLoopback` fails on a production caller.

- **Load:** an out-of-scope cached node does not count, so a cache of only such nodes is empty and the shipped
  seeds (and their alarm) are reached.
- **Save:** the cache is MERGED, never overwritten — table first, previous cache after, capped at 512 — and a table
  node is fresh only if it ANSWERED a query nib sent (`contacts`, recorded on nib's own socket wrapper, because
  anacrolix exports no node-quality accessor). A node that only queried us is never written; nothing fresh leaves
  the file untouched. Cached nodes queried this run that never answered move to the end, so the cap drops them
  first — not deleted, so one run on a captive network cannot empty a good cache.
- **Send:** nib's `screened` socket refuses an outgoing QUERY to an out-of-scope address; a reply always goes. It
  fails closed (an undecodable datagram is treated as a query). `IPBlocklist` was refused: it also drops inbound,
  and the outbound half is the whole of the risk. Counted as `Stats.RefusedSends`.

## Consequences

- A stranger can add itself to the cache only by answering nib's own queries from a public address — it can no
  longer thin, replace or aim it by pinging.
- Public DHT nodes on ports below 1024 are no longer queried (the Seed port floor). Unmeasured how much of the
  real DHT that excludes; expected to be very little.
- **Not decided here:** a shipped-seed floor below N cached nodes. D6's "only when the cache is empty" is Dan's
  wording and stands; "empty" now means "no usable node".
