# ADR-130 — a node is admitted to the DHT table only for answering us

**Status:** accepted
**Date:** 2026-10-11
**Extends:** ADR-061 (the DHT keeps, and queries, only what the node-cache rule admits) from the cache and the
outbound door to the routing table itself. ADR-009 (one door). ADR-011 is untouched.
**Context:** /pending 775. anacrolix starts every traversal from its routing table and reads `StartingNodes` —
the node cache, the shipped seeds, an invitation's seeds — only when the table is empty (`server.go:1256-1268`),
and it adds whoever sends it a query (`:492`). Under ADR-011's lazy bootstrap the table is empty until the first
publish or fetch, so one query from any host that can reach the port, landing first, became the whole starting
set for the life of the process. A LAN stranger's address is refused by ADR-061's outbound door, the traversal
has no other, and the rendezvous fails with the cache and the seeds never read (driven:
`TestANodeThatOnlyQueriedUsIsNeverAStartingPoint`, red before the change). Separately, `SeedSample` and
`probeTargets` read the table raw, so a node that had only queried us could be shipped to an invitation's
recipient as a seed, or asked where we are and believed, while the cache save refused exactly those nodes.
**Applies:** `internal/rendezvous`.

## Decision

**1. An inbound query never admits its sender.** `screened.ReadFrom` restates every inbound query with BEP-43's
read-only flag (`asReadOnly`) before the library sees it — the library's one lever for "answer, do not add"
(`handleQuery`'s `tryAdd = !m.ReadOnly`). The query is still answered. The restatement is the decoded message
re-encoded, so it is screened again; a query that cannot be restated is dropped and counted, because passing the
sender's original would admit the sender.

**2. The table therefore holds only nodes that answered a query of ours** — a response is admitted only against
a transaction we opened.

**3. Anything that TRUSTS a table node reads it through one predicate, `answeredNodes`** (`contacts.go`): the
cache save, `SeedSample`, `probeTargets`. `Nodes()` is the raw table, for a diagnostic. A census test holds that
only those two call `dht.Nodes()`. Decision 3 stands beside decision 1 on purpose: 1 is a property of one
library version's handler, 3 is ours.

**4. A datagram the library's own decoder errors on is dropped in the screen and counted** (`Stats.Malformed`,
printed by `nib rendezvous`). The library discards it too, after logging most of them whole, once per datagram,
at a rate the sender sets.

## Consequences

- Nib learns nodes only from its own traversals. It was never a long-lived DHT citizen (ADR-011: nothing leaves
  the machine until a ceremony needs it), so what is given up is table entries nobody vouched for.
- Cost: one encode and one decode per inbound query, on the DHT view only.
- Refused: clearing the table before the first bootstrap (the stranger can query again between the clear and
  the traversal), and forking the library to change `handleQuery` (a patched module for one boolean the wire
  format already carries).
- Declared: an unknown key in a sender's query does not survive the restatement; the library decodes into the
  same struct and would not have read it.
- Not run by any tier below the live DHT: `build/dhtlive.sh` is the harness for the probe and the seed sample
  against real nodes.
