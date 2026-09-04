# Contributing to Nib

Nib ships as a single Go binary with the web UI embedded (`embed.go`). There is no
build system beyond the Go toolchain, and nothing in this file is needed to *use*
Nib — it is here so that "did I break anything?" has one answer instead of three
half-answers.

## The build & verify contract

Run them all after a change. Each states what it can see, and — more importantly —
what it cannot, because a check whose blind spots are undocumented gets trusted
for things it never looked at.

| # | Command | What it verifies |
|---|---------|------------------|
| 0 | `go build ./...` | it compiles **for this host** — see the note below |
| 1 | `go test ./...` | the Go side: server, PDF operations, vault, signing, CLI |
| 2 | `./build/jsdomtest.sh` | the front end's logic and DOM behaviour, in jsdom |
| 3 | `./build/uirepro.sh` | the whole app: the real binary, in a real browser |
| 4 | `./build/pairrepro.sh` | a ceremony between TWO real binaries, two vaults, two identities — over BOTH transports |
| 4b | `./build/pairrepro.sh --lan` (or `--lan -n 4`) | the same ceremony with **no address typed anywhere**, in a namespace, asserting nothing left the link. With `-n` it is an N-party baton relay on the link, and it measures **zero** off-link packets over both transports across the relays, the delivery rounds, the end-state round and the close-out. It did not always: 120 packets, then 9, then 0 (ADR-011). **The two clauses that deliberately leave a proceeding unfinished are the exception and share a budget** — an arm still waiting is meant to reach the DHT after `lanFirstBudget`, so `interrupted_hop` and `decoy_document` are measured against 4 packets per arm left waiting rather than against zero, and their per-clause counts are printed for diagnosis but not asserted: measured over three runs the same six packets moved between them (6/0, 0/6, 0/6), because a hold started in one clause elapses in whichever is running thirty seconds later (/pending 363). **This row said "currently RED against shipped code" until 2026-09-02, four phases after that was fixed** — and in the meantime a real red went unnoticed for four commits, because a reader who ran it had been told to expect one. A harness whose contract says it fails is a harness nobody reads the output of |
| 4c | `./build/pairrepro.sh --v6` | the same ceremony over **IPv6 loopback** (`[::1]`) on both transports, with the v6 bind asserted so a v4 fallback cannot pass for it — P05.S05's hermetic half of criterion 1 |
| 4d | `./build/pairrepro.sh -n 4` (or `-n 9`) | **an N-party ceremony COMPLETED as a baton relay**, over both transports: N homes, N vaults, N distinct identities, a non-signing convener carrying the document through N−1 hops, every party armed once before hop 1 and never re-armed, each hop's document containing the previous one as a byte prefix, every hop's verification string distinct, and the finished document signed by the roster's signing parties in order — one signature each. The model's old ceiling (a signing party cannot take a second turn) is still asserted alongside it |
| 5 | `./build/mcastrepro.sh` | link-local discovery between two processes, in a network namespace of its own |
| 6 | `./build/ceremonyrepro.sh` | a ceremony between two real **processes** over real HTTP: two homes, two vaults, two signing identities, and the invitation crossing between them as text |

**Tier 0 builds for the host only, and that is a hole tier 1 now covers.**
`internal/discovery` once called `syscall.SetsockoptInt` with an `int` file descriptor —
which is a `syscall.Handle` on Windows — so `GOOS=windows go build ./cmd/nib` failed and
**nib.exe could not be produced at all**, on the platform whose `nib register` command
exists only for it. Every tier stayed green, because every tier builds for the host.
`TestEveryPlatformCompiles` cross-compiles `./cmd/nib` for windows, darwin and linux and
is part of tier 1; it skips under `-short`.

Add `node --check web/app.js` after editing JavaScript, and `go test -race
./internal/server/` after touching anything concurrent.

Tiers 2, 3 and 4 **skip cleanly** when their dependencies are absent — they print a
line saying which one is missing and exit 0. A fresh clone therefore runs 0 and 1
with no setup at all. To enable the other two: `npm install` (jsdom and
playwright-core, dev-only, `node_modules/` is git-ignored) and have a
Chromium-family browser on `PATH` — the same browser Nib itself needs to show its
UI, so if Nib runs at all, tier 3 can run.

There is also `./build/dhtlive.sh`, the **one harness that is not hermetic**. It runs
the self-address probe against the real BitTorrent DHT, because P04.S02's acceptance
requires this host's mapped port to be observed *on the wire from a real node* and no
hermetic tier can show that: two `anacrolix/dht` servers on loopback do set the BEP-42
`ip` field, so a local test proves the plumbing and nothing about the network. It is
deliberately out of the routine loop for the same reason tier 3 was made hermetic —
a check that reaches the public internet imports every stranger's outage into your
build. Run it when touching `internal/rendezvous`, the seed list, NAT
classification, or **the candidate path** — it also drives a sealed `CandidateRecord`
the whole way: published to the public DHT, fetched by a second server, opened and
checked by the real gate, and the surviving endpoint dialled by the production racer.
That last clause cannot live in `internal/rendezvous`, because L1 forbids that package
from importing `internal/ceremony`, so it is in `internal/server` and the harness names
that package too.

Setting `NIB_LIVE_SEEDS=ip:port[,ip:port...]` additionally drives the invitation-seed
rescue path — a machine whose shipped list fails bootstrapping from addresses an
invitation carried. The addresses are an **input** rather than a lookup because Nib must
never resolve a name on the bootstrap path, and a live seed test is the one place tempted
to. Without the variable that one test skips and the rest of the harness is unchanged.

**A stale row is caught at tier 1 and costs nothing.** `TestEveryRedProofStillApplies` dry-runs
every recorded patch against the working tree, so an edit that moves the code out from under a row
fails immediately rather than at the next full replay. It is the cheap half of `--all` and only that
half: it proves a patch still APPLIES, never that the check still goes red for it. Measured — 0.24 s
against 24 minutes, for the failure that accounted for six of the seven found by the first full
replay in months (/pending 348).

And `./build/redproof.sh <name>` replays a recorded red proof: it exports HEAD to a throwaway
tree, applies the row's defect as a patch, runs the named check and asserts it FAILS **with its
own assertion's token** — not merely that something exited non-zero, which is also what a
deleted or uncompilable check produces. `./build/redproof.sh` with no argument lists what is
recorded, and **`./build/redproof.sh --all` replays every row** — a minutes-long job that belongs
in a sweep rather than in `go test`, and the only thing that can tell a valid row from a file that
merely exists. The ledger it backs is `docs/red-proofs.md`, and it is what makes the "proven red at
least once" sentence below auditable rather than asserted. Not every row is mechanised; the
ledger says which are.

And `./build/ceremonyrepro.sh` runs a ceremony between two real PROCESSES over real HTTP: A
convenes and B accepts, each with its own `$HOME`, its own vault and its own signing identity, so
the invitation crosses between two binaries as text rather than being handed around inside one
process. It is where "the invitation one binary produces is the invitation the other accepts" is
actually driven — a single-process test assumes that rather than checking it. It skips cleanly
when curl or python3 are absent. **It exists because the previous version of it did not:**
P07.S02a live-verified the convene route with a script in a session scratchpad, the scratchpad was
wiped, and the product's only ceremony-creating surface was then exercised by nothing committed.

There is also `./build/winrepro.sh`, which runs the Windows binary under wine to
check the places `path/filepath` answers differently, and to run a **second
launch** against a live first one — the single-instance hand-off, on the platform
where double-click is the ordinary way in and where the mechanism it replaced
never worked at all. It is not part of the routine loop — run it when touching
path handling, file dialogs, launch/hand-off, or packaging.

## The tiers, and why there is more than one

They are not redundancy. Each one exists because the tier below it is blind to
something, and each says so in its own file rather than only here.

**Tier 1 — `go test ./...`**
Sees: every server route, the PDF pipeline, the vault, signing and verification,
the CLI. This is the bulk of the suite and the fastest feedback.
**Cannot see: the client at all.** The server never observes the browser's state,
so no amount of Go testing can tell you whether a button is enabled, whether a
teardown cleared a sidebar, or whether an armed tool survived a document switch.
Every client-side failure mode is silent here. → **tier 2**

**Tier 2 — `./build/jsdomtest.sh`** (ceiling written in `test/jsdom/boot.mjs`)
Sees: the real `web/app.js`, loaded into a jsdom document built from the real
`web/index.html`. Module behaviour, DOM state, control enablement, teardowns,
which of pdf.js's calls the app made.
**Cannot see: anything that needs rendering.** jsdom models the DOM, not an
engine — no layout (every `clientWidth` is 0), no canvas, no media queries, and
pdf.js itself is stubbed. → **tier 3**

**Tier 3 — `./build/uirepro.sh`** (ceiling written in `build/uirepro.sh`)
Sees: everything above, plus real layout, real canvas, the real pdf.js parsing a
real PDF, real files on disk, and the real Go server answering — because it builds
the binary, runs it under a throwaway `HOME`, and drives it in a browser.
**Cannot see: other engines.** Chromium only, which is deliberate — Nib opens its
UI in an installed Chromium-family browser (`internal/browser`), so Chromium is
what users get. The fallback path can still land someone in Firefox, and that gap
is tracked as a `VERIFY` item on the pending list rather than pretended away.

**Tier 4 — `./build/pairrepro.sh`** (ceiling written in `build/pairrepro.sh`)
Sees: everything above, twice over — two binaries with two homes, two vaults and
two identities, completing a ceremony between them over loopback — **once over
TCP and once over QUIC** (D14 keeps both). It is the only tier that can assert
anything a second party observes: that the peer is a different identity, that
both sides derive the *same* four verification words, and that the finished
document carries two signatures when read from the receiving side rather than
reported by the sender.

Everything in that list is transport-blind, which is why the run also **observes
the socket**: it connects to the armed port over TCP and requires that to succeed
on the TCP run and fail on the QUIC one. Without it, a build that ignored the
transport field would run TCP twice and report QUIC coverage it did not have.
**Tier 5 — `./build/mcastrepro.sh`** (ceiling written in `build/mcastrepro.sh`)
Sees: two processes discovering each other over **real multicast** — two sockets
sharing a port, two group memberships, a datagram that crosses a kernel — inside a
network namespace it creates itself with `unshare -rn`. The namespace is the point:
a multicast loopback copy traverses INPUT, so a default-deny host swallows discovery
on Nib's port with **no error at either end**, and a harness on the host would be
green on a permissive machine and red on a locked-down one without testing this code
either time. Tier 1's discovery tests skip on such a host — honestly, and a skip is
not a verification, which is what this tier is for.
**Cannot see: a network.** A dummy interface has no switch, no IGMP snooping, no
second machine to disagree with. And it cannot see Windows at all, which is where
this code's known divergences live: `x/net`'s `SetControlMessage` is unimplemented
there, IPv4 group joins resolve the interface to an address rather than an index, and
`FlagRunning` carries no information. Those close on a real-Windows run.

**`--lan` is the same harness with the addresses removed** — the armed side omits its
bind and announces, the dialing side omits the address and browses. It re-execs into a
network namespace, for the reason tier 5 does: a default-deny host swallows discovery
silently. That namespace is given a **black-hole default route**, which looks wrong and
is the instrument: P03's criterion says the ceremony completes with no outbound internet
traffic, and an nft output counter in a namespace with *no* route reads **zero even
after a real connect attempt**, because the kernel refuses at routing before the output
hook. With the route, attempts become packets the counter sees. The run provokes a
deliberate connection first and **fails if the counter does not move**, so a counter that
could never fire cannot pass for silence.

**`--v6` is the same harness over IPv6 loopback** — the two HTTP control planes stay on
`127.0.0.1` (they are not what is under test), but the ceremony socket binds and dials
`[::1]`. The bind, the dialled address and the `/dev/tcp` transport probe change family
together, or the run is a v4 run wearing a v6 label; and the run asserts the address the
armed side reports back is a bracketed v6 literal, red-proved by forcing a v4 bind. It is
the buildable half of criterion 1 (IPv6-to-IPv6, neither side forwarding a port); the
two-machine run stays Dan-only, exactly like tier 4's v4 two-machine case.
`NIB_PAIR_V6_ADDR=<this host's global v6, no port>` moves it off loopback onto a real
address on the same host.

**Cannot see: two networks.** Both instances are on loopback, so NAT, routing,
MTU and firewalls are invisible — and those are exactly what the connection
ladder exists to survive. What it delegates upward is the two-machine run, which
stays a `VERIFY` item on the pending list.

**Tier 6 — `./build/ceremonyrepro.sh`** (ceiling written in `build/ceremonyrepro.sh`)
Sees: the ceremony routes driven between **two processes** that hold different keys —
A convenes, B accepts, and the invitation crosses as text. That is the property a
single-process Go test assumes rather than checks: `httptest` gives every party one
vault, one identity and one registry, so "the invitation one binary produces is the
invitation the other binary accepts" cannot be asked there at all. It also covers the
refusals that need two identities to be meaningful — an arm before accepting (the step
D21 removes), a contribution out of roster order — and asserts the invitation secret is
in none of the convener's `~/nib` files (D29).
**Cannot see: a hop completing.** B never signs here; the L3 clause asserts B is
REFUSED for trying. Driving a hop through needs a rendezvous and a second armed
listener, which is tier 4. And two processes on one machine are not two machines: one
loopback, one clock, one filesystem, one kernel, so it says nothing about NAT or the
DHT. → **tier 4**

**Its number is not its rank.** Tier 6 is cheaper and narrower than tiers 4 and 5 —
one machine, no namespace — and the numbers are the order the harnesses were built,
not an order of strength. It exists because P07.S02a live-verified the convene route
with a script in a session scratchpad, the scratchpad was wiped, and the product's only
ceremony-creating surface was then exercised by nothing committed. **A verification that
lives outside the repo is one that has already been lost once.**

That chain is the point. A gap named and delegated is a decision; a gap nobody
wrote down is discovered later by a user.

### Proving a tier can fail

Every tier here has been **proven red at least once** against a deliberately
reintroduced defect, and that matters more than any of them being green: a check
never seen to fail can only ever report pass.

**The record is `docs/red-proofs.md`** — which defect was put back, which assertion
fired, and what it said. Until that file existed the claim above was backed by
nothing a reader could check, and `verify_test.go` guarded only that the *sentence
was present*: the same failure one level out, in the file that teaches the rule. The
ledger names its own gap too — there is no fixture mode, so re-proving a row is a
manual edit-run-revert.

Two guards enforce the parts of this that rot quietly — `verify_test.go` (the tier
table's rows survive, each tier still states its ceiling **in its own harness file**,
and the proven-red claim has a ledger behind it) and `internal/browser/browser_test.go`
(tier 3 hunts for the same browsers Nib does). Both are in tier 1, so they run first
and for free.

## The multiple-documents laws

Nib holds several documents at once (see `docs/adr/`). Three rules constrain code
whose author will never read the plan that produced them, so they are written here —
in the committed file a fresh clone actually gets — rather than only in an ADR or in
a local `CLAUDE.md`.

1. **No operation may act on a document it did not capture at its start.** ADR-001.
   An operation that bakes document A's bytes and then posts them must name A, or
   the server applies them to whatever is active when the request arrives. Capture
   the id before the first `await` and pass it as `apiFetch`'s `docId`.
   *Guarded by* `test/jsdom/pinning.test.mjs` — "no mutating call is unpinned".

   The law has a **third** failure mode on the client, and it fails independently of
   the other two: a gesture holding a `setPointerCapture` keeps receiving
   `pointermove` after its view is hidden, because capture is released when the
   element leaves the DOM and `display:none` does not remove it. Such a gesture
   cannot pin an owner and carry on — a hidden container reports `clientWidth` 0, so
   continuing would lay the field out against nothing. It must be **cancelled**:
   `enableStampGestures` and `enableMarkerGestures` register an `end` in
   `activeGestures`, and `abortDrags()` drains it. Anything new that takes a pointer
   capture registers there too.
   *Guarded by* `test/ui/gestures.test.mjs` — "a drag in flight when the user switches
   documents neither moves the flag nor records onto the new document" (tier 3: jsdom
   has neither `setPointerCapture` nor layout).
2. **Every reload names the view it lands in.** The same law's other half, and they
   fail apart: `setDocumentFromServer(meta)` with no target writes into whatever
   view is active when the round-trip *returns*, wiping that view's overlays, redact
   marks and undo stack. Only the document-installing routes (`open`, `open-url`,
   `upload`, `combine`, `office`) may omit a target, because installing into the
   active view is what opening a document means.
   *Guarded by* `pinning.test.mjs` — "every reload names the view it lands in".
3. **`views` is mutated in exactly three places** — `addView`, `removeView`,
   `resetViews` — and each re-renders the tab strip. The strip is a rendering of
   that array; a fourth mutator leaves it describing a set of documents the app does
   not hold, and nothing else would say so.
   *Guarded by* `test/jsdom/view.test.mjs` — "views is mutated in exactly three
   places, and each re-renders the strip".

Each guard has been proven red against the defect it names. If you are changing one
of these and the guard is in your way, the guard is the thing that is right.

## The goroutine law

**Every detached goroutine defers `safe.Recover(...)` as its first statement.** A
panic in any goroutine takes the whole desktop process down with the user's unsaved
documents; `safe.Recover` degrades it to a logged line. "First statement" is not
style — registered first means it runs last, so the goroutine's other defers
(`wg.Done`, a `Close`, a disarm) still run as the stack unwinds.

A goroutine is detached through **two** doors and the law binds both: a `go`
statement, and `time.AfterFunc`, whose callback runs on its own goroutine with no
caller to unwind into. A bare `go f()` has nowhere to hang a defer, so either `f`
recovers itself as *its* first statement or the launch is wrapped in a literal that
does.

*Guarded by* `goroutines_test.go` — "every detached goroutine is recovered". It walks
the repo, not a package list, and it walks both doors. It lived in
`internal/server/racelifetime_test.go` and was scoped to that one package until
v1.117.113, which is why `internal/udpmux`'s `readLoop` — the shared socket's sole
reader of untrusted inbound datagrams — shipped with no recover at all: the guard
could not see it. Promoting it found seven more sites, two of them behind the
`AfterFunc` door.

Test goroutines are excluded on purpose: a recover there swallows the panic that *is*
the failure signal.

## Repo conventions

- **Bump `VERSION` in the same commit as the change.** It is a single semver line
  and the source of truth for `build.sh`, `install.sh`, and the `.deb`. Patch for
  fixes and refactors; minor for a new user-facing feature.
- **Keep docs in step with the code** in the same change, not afterwards.
- `THIRD-PARTY-NOTICES.md` is generated — run `build/gen-notices.sh`, and
  `TestNoticesUpToDate` will tell you if you forgot.
- **A new architectural decision gets an ADR in the same change** —
  `docs/adr/`, numbered, with `_index.md` listing them. ADRs are immutable in
  their decision content: supersede with a new one rather than editing an old one.
- Licensed AGPLv3; the project ships as-is, with no warranty.
