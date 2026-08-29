package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"nib/internal/ceremony"
	"nib/internal/p2p"
	"nib/internal/rendezvous"
	"nib/internal/sign"
)

// ceremonyID is the ceremony identity an armed session carries: which proceeding this is,
// which hop of it, and the material every rendezvous derivation needs.
//
// **This is the import that did not exist.** Until P05.S04 `internal/server` imported neither
// `internal/ceremony` nor `internal/rendezvous`, so the whole of P04's output was reachable
// only from `nib rendezvous --self-test`. An arm knew a fingerprint, a bind, a mode and a
// transport, and nothing about the proceeding it was part of.
//
// It is OPTIONAL on the arm. A session without one is the manual and LAN path this route has
// always served, which D9 demotes rather than deletes.
type ceremonyID struct {
	inv  ceremony.Invitation
	hop  int
	gate *ceremony.CandidateGate
	// me and peer are the two ends of this hop, hex. Kept because the gate holds them
	// privately and the publish side needs its own.
	me, peer string
	// certPEM/keyPEM sign this party's candidate records. Held here because a record is
	// signed by the identity, not by the ceremony, and the publish path needs both.
	certPEM, keyPEM []byte

	// punch is D33's per-(hop, side) packet budget, shared with the OTHER ceremonyID this machine
	// holds for the same ceremony (P07.S09b). Set by punchBudget() below; read by diagnose() so
	// the drops reach D19's sentence. Guarded by mu, like every other field diagnose() reads.
	punch *punchBudget

	// end is the ONE socket the DHT and the armed listener share, and rz is the rendezvous
	// server on its DHT view. Both nil on the TCP transport — see openRendezvous.
	end *p2p.SharedEndpoint
	rz  *rendezvous.Server
	// mu guards stopNet and portMap, both written by the armed background goroutine and read by
	// close() on the teardown goroutine. **stopNet was a live data race before S07** (grill C5):
	// written at `startArmedRendezvous`'s third statement, read by close(), with no lock and
	// nothing but a tight window keeping -race quiet.
	mu sync.Mutex
	// stopNet cancels the arm's background rendezvous work — bootstrap, the LAN wait, the
	// publish. Set by startArmedRendezvous and called by close, so the goroutine ends with
	// the session rather than outliving it. Guarded by mu.
	stopNet context.CancelFunc
	// portMap is the router port-mapping lease for this arm (S07): obtained at publish,
	// refreshed while armed, deleted by close(). nil when the tier obtained nothing. Guarded
	// by mu.
	portMap *portMapper
	// closed guards the narrow window where close() runs BEFORE the arm goroutine has stored
	// stopNet/portMap (diff-grill #4): a setter arriving after close acts immediately rather
	// than storing state nothing will ever tear down.
	closed bool
	// reDelivery caches THIS hop's co-signed output so a reconnect after the signature but before
	// the initiator read it re-delivers the SAME bytes instead of signing again (P05.S10, D18/D24).
	// Keyed on sha256 of the INBOUND (the peer's already-signed document): keying on the hop alone
	// would hand a reconnect with a different document the wrong signature. The ceremonyID is
	// already per-hop, so the hop is implicit. Guarded by mu; cleared by close, so no signed bytes
	// outlive the ceremony. In-memory only — the process-kill persistence D24 names is P08's.
	reDelivery map[string][]byte
	// self is this side's most recent DHT self-address probe — its mapping CLASS and whether the
	// mapped address is in shared (CGNAT) space. `ProbeSelf` computes the full result but the
	// publish path used only the addresses and DISCARDED the class (P05.S11 deepdive); retained here
	// (under mu, last-write-wins) so the failure diagnosis (D19 cause 3) can read it. Zero value is
	// MappingUnknown, which degrades cause 3 to cause 4 cleanly.
	self rendezvous.SelfAddress
	// mapUnroutable is true once the router answered the port-map tier with an address that could
	// not be published (double-NAT / RFC-1918 / low port) — distinct from "the tier got no answer",
	// because D9's cause-3 advice diverges: an unroutable answer means a VPN, never a port-forward.
	mapUnroutable bool
	// mapRefused is true once a gateway ANSWERED the port-map tier with a refusal — a NAT-PMP or
	// PCP result code, or an IGD UPnPError. The opposite fact from silence: the router is the
	// user's, is reachable, and said no (/pending 263).
	mapRefused bool
	// peerSeen becomes true once the feed has admitted a candidate for the peer — the "the peer
	// published" signal for the D19 diagnosis (P05.S11). Atomic, not read off the gate: the gate is
	// not concurrent-safe and the ARM polls its diagnosis WHILE the feed is running, so cause 1 must
	// key on a signal safe to read live. Set by feedCandidates as it sends each candidate.
	peerSeen atomic.Bool
	// recordRefused / recordEmpty are the D19 cause signals for "the peer published but the gate
	// could not use it" — snapshotted from the gate stats in feedCandidates (the gate's ONLY
	// writer) so diagnose() reads atomics, never the gate itself. The live arm-side diagnosis
	// (sessionStatus.status -> diagnose) runs WHILE the feed is still writing the gate, so a direct
	// gate.Stats() read there is a data race; these atomics are how the invariant "diagnose reads
	// only guarded signals" survives the P05-close fix that added the cause (v1.117.106).
	recordRefused atomic.Bool
	recordEmpty   atomic.Bool
	// bootstrapDone gates the ARM-side live diagnosis: until the DHT bootstrap has had its chance,
	// zero DHT responses means "still warming up", not "unreachable", and showing cause 2 then is a
	// scary false alarm on a healthy machine (P05.S11 diff-grill). Set once, inside
	// ensureBootstrapped, which is the only thing that bootstraps.
	bootstrapDone atomic.Bool
	// bootstrapOnce/bootstrapErr are ensureBootstrapped's door. See its comment.
	bootstrapOnce sync.Once
	bootstrapErr  error
	// linkSeenAt is when this arm last resolved its OWN expected peer on the local link, in unix
	// nanoseconds; zero means never. Written by answerLoop's sighting hook, read by holdDHT.
	// Atomic because the answer loop and the candidate feed are different goroutines.
	linkSeenAt atomic.Int64
	// linkWatchAt is when this arm STARTED watching the link, same units. It is what makes "never
	// seen" different from "not looking": the dial side sets neither and is unaffected, and an arm
	// that has not heard anything yet is still owed its budget rather than released on a race
	// between its answer loop starting and the candidate feed's first wait ending.
	linkWatchAt atomic.Int64
}

// watchingLink records that this arm has started listening for its peer on the link.
func (c *ceremonyID) watchingLink(at time.Time) {
	if c == nil {
		return
	}
	c.linkWatchAt.CompareAndSwap(0, at.UnixNano())
}

// noteLinkSighting records that the party this arm is waiting for is on the local link.
//
// **It is the arm's half of ADR-011, and the signal was already being computed.** `answerLoop`
// resolves `resolve(pins, seen)` on every iteration, where `answerHopSeekers` built `pins` from
// this arm's own expected peer — so "the party I am waiting for is on this link" was already
// asked, already screened against PINS rather than wire bytes (L1: a stranger's announcement does
// not resolve, and never reaches here), and simply had no reader for this purpose.
func (c *ceremonyID) noteLinkSighting(at time.Time) {
	if c == nil {
		return
	}
	c.linkSeenAt.Store(at.UnixNano())
}

// holdDHT blocks until this ceremony may reach the public DHT, and reports whether it may at all
// (false means the ceremony ended first).
//
// **Why a renewable hold rather than a duration.** S05d gave the DIAL side a decisive answer —
// `peerAddresses` browses before the race, so a LAN candidate is the link having already answered.
// The arm has no such one-shot answer: it is waiting, and in a relay it may wait through seven
// other hops. Measured at nine parties, that is exactly who leaked — instances 1 and 2 reached the
// DHT zero times and 3 through 9 reached it twice each, which is precisely the parties whose turn
// comes after a fixed window closes.
//
// So the arm holds on EVIDENCE and re-asks: every sighting of its own expected peer pushes the
// deadline out by `lanFirstBudget`, and a link that stops carrying that peer stops renewing. It
// degrades in the right direction — a genuinely remote ceremony never renews at all and pays
// `base`, which is the same cost it paid before this existed.
//
// On the dial side nothing writes `linkSeenAt`, so the loop below runs zero times and the
// behaviour is identical to the plain wait it replaced. That is deliberate: one door, and the
// side that does not need renewal does not get a second code path (ADR-009).
func (c *ceremonyID) holdDHT(ctx context.Context, base time.Duration) bool {
	if !waitCtx(ctx, base) {
		return false
	}
	for {
		ns := int64(0)
		if c != nil {
			ns = c.linkSeenAt.Load()
			if ns == 0 {
				// Never heard anything YET is not the same as not looking. Measuring from when
				// the watch began is what stops the release being a race between the answer loop
				// starting and this wait ending: announcements arrive at 2/s, `base` is two
				// seconds, and a first sighting that lands at 2.1 s would otherwise have missed.
				// A genuinely remote arm therefore pays exactly one `lanFirstBudget` and then
				// publishes, which is the acceptance clause in as many words.
				ns = c.linkWatchAt.Load()
			}
		}
		if ns == 0 {
			return true // nobody is watching the link at all — the dial side, unchanged
		}
		left := lanFirstBudget - time.Since(time.Unix(0, ns))
		if left <= 0 {
			return true // the link went quiet and stayed quiet
		}
		if !waitCtx(ctx, left) {
			return false
		}
	}
}

// waitCtx sleeps for d, reporting false if ctx ended first. A non-positive d still checks ctx, so
// a caller cannot skip cancellation by asking for no wait.
func waitCtx(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}

// ensureBootstrapped is the ONE door onto the DHT's first contact with the network (ADR-009),
// and it is LAZY.
//
// **P07.S05d, and it is a packet count rather than an argument.** P03's exit criterion says a LAN
// ceremony completes with NO outbound internet traffic. Measured in the namespace with an nft
// counter on off-link traffic: a two-party LAN ceremony emitted 0 packets and a four-party LAN
// relay emitted 120. The difference is the invitation. THREE sites bootstrapped eagerly — the
// dialer at construction, and BOTH arm paths, which are different functions (`startArmedRendezvous`
// for TCP, `runCeremonyReceive` for QUIC) — so the public DHT was contacted on every hop of every
// ceremony that carries an invitation, which is every ceremony P07 builds. It survived four phases
// because the only `--lan` run was the two-party one, which has no invitation and therefore no
// ceremony object at all: the run that was supposed to prove the criterion was the one shape that
// could not reach the defect.
//
// Lazy alone would not have been enough, and that is the other half of the fix. `publishLoop`
// already waits `browseWindow` before its first publish and says why; `feedCandidates` did not
// wait before its first Fetch. So a bootstrap deferred to first use, with an unwindowed fetch
// immediately after it, moves the first off-link packet by microseconds. Both DHT verbs now come
// through this door, and both are behind the window.
//
// **Once per ceremony object is exactly the attempt count the three eager calls already had** — a
// fresh ceremonyID per hop on the dialer, one per arm — so `sync.Once` preserves today's behaviour
// rather than introducing a new limit on retry. Said out loud because a future reader will
// otherwise read Once as one.
//
// The first caller's ctx governs the attempt and later callers get the cached result; every caller
// is bound to the ceremony's own lifetime, so a ctx that dies is a ceremony that is ending.
func (c *ceremonyID) ensureBootstrapped(ctx context.Context) error {
	if c == nil || c.rz == nil {
		return errNoCeremony
	}
	c.bootstrapOnce.Do(func() {
		bctx, cancel := context.WithTimeout(ctx, bootstrapBudget)
		defer cancel()
		c.bootstrapErr = c.rz.Bootstrap(bctx)
		// Set INSIDE the door, at the moment the attempt completes, rather than at the old
		// call sites. D19's arm-side diagnosis is gated on this flag, and a lazy path that
		// never bootstraps must not leave a reader believing it did: a ceremony the LAN
		// answered never bootstraps and now never claims to have.
		c.bootstrapDone.Store(true)
	})
	return c.bootstrapErr
}

// setStopNet and setPortMap store the two shared fields under the lock.
func (c *ceremonyID) setStopNet(cancel context.CancelFunc) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		cancel() // close() already ran; do not store a canceller nothing will call
		return
	}
	c.stopNet = cancel
	c.mu.Unlock()
}

// reDeliverKey hashes the inbound document — the idempotency key for re-delivery (P05.S10).
func reDeliverKey(inbound []byte) string {
	sum := sha256.Sum256(inbound)
	return string(sum[:])
}

// Cached returns THIS hop's previously co-signed output for `inbound`, or nil if this hop has not
// signed that document (a cache miss runs the fresh exchange). Nil after close, so a re-delivery
// races a teardown to a clean miss rather than a signature off a dead ceremony.
func (c *ceremonyID) Cached(inbound []byte) []byte {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	return c.reDelivery[reDeliverKey(inbound)]
}

// Store records THIS hop's co-signed output for `inbound`, so a later reconnect re-delivers it. A
// Store arriving after close is dropped (the closed-flag pattern setStopNet/setPortMap use), so no
// signed bytes are stored on a ceremony nothing will tear down.
func (c *ceremonyID) Store(inbound, final []byte) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	if c.reDelivery == nil {
		c.reDelivery = map[string][]byte{}
	}
	c.reDelivery[reDeliverKey(inbound)] = final
}

// hasSigned reports whether this hop has produced any signature — the receiver's "am I past the
// gate" signal, since a lost writeback is otherwise indistinguishable from a clean completion.
func (c *ceremonyID) hasSigned() bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.reDelivery) > 0
}

// setSelf records this side's DHT self-address probe (mapping class + shared-space) for the D19
// diagnosis (P05.S11). Under mu; last-write-wins across re-race iterations — the NAT class is a
// property of this host, stable across them.
func (c *ceremonyID) setSelf(self rendezvous.SelfAddress) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.self = self
	c.mu.Unlock()
}

// markMapUnroutable records that the port-map tier got an UNROUTABLE answer (double-NAT). Monotonic:
// once true it stays, so a later iteration's no-answer does not erase the double-NAT signal.
func (c *ceremonyID) markMapUnroutable() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.mapUnroutable = true
	c.mu.Unlock()
}

// markMapRefused records that a gateway answered the port-map tier and REFUSED. Monotonic for the
// same reason as its sibling: a later iteration finding no gateway must not erase the fact that one
// answered.
func (c *ceremonyID) markMapRefused() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.mapRefused = true
	c.mu.Unlock()
}

// setPortMap stores the mapper close() will tear down, and closes whatever it replaces.
//
// **The replace case became reachable at v1.117.123** (/pending 256), which turned the one-shot
// publish into a republish loop: every cycle builds a fresh mapper, and overwriting the field
// orphaned the previous one — its refresh goroutine still running, its router mapping still
// installed, and nothing holding a handle to either. It is reachable in one ordinary ceremony,
// not a corner: the republish period is 240 s inside a 300 s connect deadline.
//
// The old mapper is closed OUTSIDE the lock. close() joins a goroutine and then talks to the
// router on a fresh context, and holding the ceremony's mutex across that would stall every
// other reader of it for up to the join timeout.
func (c *ceremonyID) setPortMap(pm *portMapper) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		pm.close() // close() already ran; delete this mapping rather than leak it
		return
	}
	old := c.portMap
	c.portMap = pm
	c.mu.Unlock()
	if old != nil && old != pm {
		old.close()
	}
}

// nodeCacheDir is where the DHT's node list lives.
//
// **The config directory, not `~/nib`.** A node cache is a list of IP addresses this machine
// has spoken to, and `~/nib` is the DOCUMENT output directory — the folder a small practice is
// most likely to put in Dropbox or a network share, because it is where their finished client
// files land. It is also created 0755 by the save path, so `writeNodes`' own 0700 MkdirAll is
// a no-op there and the file's existence and mtime are world-readable on a shared machine:
// a per-user signal of *this person does remote signings, most recently at T*. The config
// directory is 0700 and is where the vault already lives.
func nodeCacheDir(configDir string) string { return filepath.Join(configDir, "dht-nodes") }

// openRendezvous gives this ceremony a socket the DHT and the listener share, and returns the
// listener built on it.
//
// **QUIC only, and the limit is structural rather than an omission.** A TCP listener is a
// `net.Listener` over a different IP protocol, and a NAT keeps separate mapping tables per
// protocol — which is exactly why D15 requires both UDP and TCP to be mapped when both are
// offered. So on TCP the listener binds its own socket and this returns no endpoint: the
// ceremony still runs, and caveat 7's clause is simply not satisfied for that transport.
// Stated here rather than discovered at S06, the slice that first asks a router for a mapping.
// setupSharedEndpoint opens the ceremony's shared QUIC socket and its rendezvous WITHOUT a
// listener (P05.S09): the symmetric-racing coordinator owns the single handshaked listener, and a
// transport permits only one. It is openRendezvous's QUIC branch minus the QUICListenOn — the arm
// and the dialer now set the endpoint up the same way, and connect arms the listener on top.
func (c *ceremonyID) setupSharedEndpoint(bind, configDir string) error {
	end, err := p2p.NewSharedEndpoint(bind)
	if err != nil {
		return err
	}
	rz, err := rendezvous.Open(end.DHT(), nodeCacheDir(configDir))
	if err != nil {
		end.Close()
		return err
	}
	c.end, c.rz = end, rz
	return nil
}

func (c *ceremonyID) openRendezvous(transport, bind, configDir string, cert, key, peerFP []byte) (p2p.Listener, error) {
	if transport != transportQUIC {
		ln, err := listenPeer(transport, bind, cert, key, peerFP)
		return ln, err
	}
	end, err := p2p.NewSharedEndpoint(bind)
	if err != nil {
		return nil, err
	}
	rz, err := rendezvous.Open(end.DHT(), nodeCacheDir(configDir))
	if err != nil {
		end.Close()
		return nil, err
	}
	ln, err := p2p.QUICListenOn(end, cert, key, peerFP)
	if err != nil {
		// Teardown order matters even on the failure path: the rendezvous server first,
		// then the socket. Closing the mux while the DHT still reads its view makes that
		// read return net.ErrClosed, which anacrolix/dht turns into a panic on a goroutine
		// nothing of ours is on.
		rz.Close()
		end.Close()
		return nil, err
	}
	c.end, c.rz = end, rz
	return ln, nil
}

// close tears the ceremony's network down IN ORDER, and the order is the whole of it.
//
// rendezvous first, then the socket. Three of the six plausible orderings panic the process:
// any that closes the mux while the DHT server is still reading it. The one existing call site
// in the tree gets this right only by accident of `defer` LIFO, with nothing naming the rule.
func (c *ceremonyID) close() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.closed = true
	stop := c.stopNet
	mapper := c.portMap
	c.reDelivery = nil // drop the signed outputs with the ceremony (D6: no signed bytes at rest)
	c.mu.Unlock()
	if stop != nil {
		// Cancel the background work FIRST, so Close's own join has something finite to
		// wait for rather than a publish still holding its 45 s budget.
		stop()
	}
	// The mapping is released before the sockets close. Its delete opens its OWN socket to the
	// gateway (portmap.Client.Unmap dials fresh) on a FRESH context (grill C2), so it neither
	// needs c.end nor is cancelled by the stop() above; it joins the refresh goroutine first so
	// nothing re-creates the mapping after the delete (grill C3).
	if mapper != nil {
		mapper.close()
	}
	if c.rz != nil {
		c.rz.Close()
	}
	if c.end != nil {
		c.end.Close()
	}
}

// checkArrival is C17: the party reconciles the document it was handed against the invitation
// its arm was built from, BEFORE the consent gate.
//
// # What it asks, and the one it deliberately does not
//
// `ceremony.CheckRecord` — a record is there, its convener signature verifies, and its digest
// rule is one this build can compare against. Then `MatchesRecord`, which compares the
// invitation's commitment against the record's and so binds this invitation to exactly ONE
// record, covering every axis the preimage covers.
//
// **`CheckDocument`'s hash comparison is NOT asked, and that is measured rather than
// conceded.** The document a counterparty is handed always carries at least the sender's
// co-signature; that signature is visible on every production path; `ContentDigest` hashes
// `/Annots`. Measured at this slice: the hop-1 receiver's copy verifies, its record extracts and
// verifies, and `CheckDocument` answers *"these are not the same document"* — accusing an honest
// convener of tampering. A gate that asked it would refuse every honest ceremony at hop 1, which
// is the whole product. See `ceremony.CheckRecord`.
//
// # Why it is here and not in the confirmer
//
// ADR-009: the gate is one door, and its guard asserts that both callers route THROUGH it
// rather than asserting the text this function prints. There are TWO: `sessionConfirmer.Confirm`,
// where a received document exists before the user sees it, and `handleSessionInitiate`, where a
// pasted invitation supplies the roster whose label, capacity and recital the local signature is
// about to carry. This comment named only the first until P07.S07b, and the second had no check.
func (c *ceremonyID) checkArrival(pdf []byte, now time.Time) error {
	rec, err := ceremony.CheckRecord(pdf, now)
	if err != nil {
		return fmt.Errorf("this document's ceremony record could not be checked: %w", err)
	}
	// Returned unwrapped: MatchesRecord's sentences already name the axis and the two values,
	// and a preamble in front of "the invitation commits to ceremony X and this document's
	// record commits to Y" makes the user read past the diagnosis. Unwrapped also keeps
	// `errors.Is(err, ceremony.ErrRosterMismatch)` answerable by the caller.
	return c.inv.MatchesRecord(rec)
}

// l3Roster is what the L3 gate is handed: the signing order and this proceeding's commitment,
// as primitives (P07.S03).
//
// **From the INVITATION, and that is the scope's own requirement rather than convenience.** S03
// says the gate reads "the record the party verified at arm time" and never the one carried by
// the document — a gate reading the document's own record answers its own question, which is
// reachable today because `Embed` refuses only an already-signed document, `Record.Verify` asks
// only that the signer appear in that record's own roster, and `ContentDigest` excludes
// attachments. The invitation is that verified copy: `MatchesRecord` binds it to exactly one
// signed record (P07.S02b's arrival gate), so its roster is the record's roster or the hop never
// got this far.
//
// Nil-safe: the manual and LAN paths have no ceremony, and the zero Roster is what tells the gate
// there is no signing order to be out of.
func (c *ceremonyID) l3Roster() p2p.Roster {
	if c == nil {
		return p2p.Roster{}
	}
	// **The version travels with the commitment.** It is this build's record format version, not
	// a field of the invitation, and that is the right value: the commitment is what THIS build
	// computes and verifies the record under. Two parties on different formats digest the same
	// roster to different hashes, their tokens differ, and D32's skew sentence — not an accusation
	// — is what the reader sees. An invitation-carried version would only let a sender claim a
	// format it is not using.
	// **`Intent` comes from the invitation and is safe to because `MatchesRecord` compares it
	// against the record's (P07.S07b).** Without that comparison this would be an unsigned hint;
	// with it, the invitation's copy is the record's copy or the hop was refused.
	out := p2p.Roster{
		Commitment:        c.inv.RosterHash,
		CommitmentVersion: ceremony.FormatVersion,
		Intent:            c.inv.Intent,
	}
	// **The WHOLE entry, and the fields that used to be dropped here are the point (P07.S07a).**
	// `Label` and `Capacity` were left on the floor, so every block said `Signer: Nib User` while
	// the label sat inside the signed commitment with no display reader anywhere. They are safe to
	// carry from the invitation for the reason `p2p.RosterEntry` records: `matchesRosterFields`
	// compares the whole `ceremony.Party` struct against the signed record, and `checkArrival`
	// runs it before consent.
	for _, p := range c.inv.Roster {
		out.Entries = append(out.Entries, p2p.RosterEntry{
			Fingerprint: p.Fingerprint, Signs: p.Signs, Label: p.Label, Capacity: p.Capacity,
		})
	}
	return out
}

// punchBudget returns this ceremony's packet budget on this machine, and remembers it so
// `diagnose` can report the drops.
//
// Both punch loops of one hop call it and both get the same counter — that is the whole point,
// and it is why the budget is keyed by ceremony id on the Server rather than built at the call
// site. Nil-safe: a punch outside any ceremony has no proceeding to share a budget within.
func (c *ceremonyID) punchBudget(s *Server) *punchBudget {
	if c == nil {
		return &punchBudget{}
	}
	b := s.punchBudgetFor(c.inv.ID)
	c.mu.Lock()
	c.punch = b
	c.mu.Unlock()
	return b
}

// carries reports whether this machine moves the baton for this ceremony without contributing to
// it — a roster member with `signs:false` (P07.S05, C07).
//
// **Whether you sign is a fact about the roster, not a choice**, which is why there is no separate
// carry route and no flag on the request. `/api/session/initiate` asks this and takes the carry
// path or the contribution path accordingly, so a non-signing convener cannot accidentally sign
// and a signer cannot accidentally skip their turn — both are unrepresentable rather than checked.
func (c *ceremonyID) carries(meFP string) bool {
	if c == nil {
		return false
	}
	for _, p := range c.inv.Roster {
		if strings.EqualFold(p.Fingerprint, meFP) {
			return !p.Signs
		}
	}
	return false
}

// ceremonyIDOf is the ceremony a session belongs to, or "" outside one.
//
// Nil-safe, because every caller that has a `*ceremonyID` also has the manual path where there is
// none — and a helper that made each of them write the nil check is a rule at N call sites.
func ceremonyIDOf(c *ceremonyID) string {
	if c == nil {
		return ""
	}
	return c.inv.ID
}

// mirrorHop writes a completed hop's output to this machine's ceremony mirror (C22).
//
// **One door for both sides of a hop (ADR-009).** The initiating side calls it before its HTTP
// response returns, which is C22's wording; the receiving side calls it when it installs its own
// copy, which C22's wording does not reach and which is the same fact about the same party — the
// document they just put their signature on. A rule written at one of two sides is the shape this
// repo keeps finding.
//
// **`WriteMirror` had exactly ONE caller before this — convene.** So the mirror recorded what a
// convener started and never what anybody signed: the durable record of a ceremony stopped at the
// moment it began.
//
// **It takes the DOCUMENT and not a ceremony**, which is what lets both sides call it: the record
// travels inside the bytes, so a caller that has the result has everything. A document with no
// record is an ordinary co-sign and returns silently — that is the majority of arrivals and it is
// not a failure to report.
//
// Best-effort and it SAYS SO when it fails. The signature exists on the document whether or not
// this write lands, so failing the hop over it would discard a real signature to protect a copy of
// it. A log line is the channel, on the shape `unconvene`'s own review established, and it names
// what was lost rather than that something was.
func (s *Server) mirrorHop(final []byte) {
	if len(final) == 0 {
		return
	}
	rec, err := ceremony.Extract(final)
	if errors.Is(err, ceremony.ErrNoRecord) {
		return // an ordinary co-sign: no ceremony, nothing to mirror
	}
	if err != nil {
		log.Printf("a completed hop carries a ceremony record that cannot be read, so nothing "+
			"was mirrored: %v — this machine keeps no durable copy of what it just signed", err)
		return
	}
	if _, err := ceremony.WriteMirror(defaultOutputDir(), rec, final); err != nil {
		log.Printf("ceremony %s: the completed hop could not be written to the mirror: %v — the "+
			"signature is on the document either way, but this machine has no copy of it under "+
			"~/nib/ceremonies", rec.ID, err)
	}
}

// errNoCeremony reports an arm with no invitation — not an error, the ordinary case.
var errNoCeremony = errors.New("this session has no ceremony identity")

// ceremonyFor builds the identity from an invitation and the two parties.
//
// The hop is DERIVED, never supplied: it is the roster's own order (D22 makes connectivity a
// sequence of pairs, and Party's doc says the roster order is the signing order), read off an
// artifact both ends already hold. A hop passed in a request would be a number the two sides
// have to agree on, and there is nothing to make them.
func ceremonyFor(text string, myCertPEM, myKeyPEM []byte, peerFP []byte) (*ceremonyID, error) {
	if text == "" {
		return nil, errNoCeremony
	}
	inv, err := ceremony.ParseInvitation(text)
	if err != nil {
		return nil, fmt.Errorf("that invitation could not be read: %w", err)
	}
	myFP, err := sign.Fingerprint(myCertPEM)
	if err != nil {
		return nil, err
	}
	me := hex.EncodeToString(myFP)
	peer := hex.EncodeToString(peerFP)

	hop, err := inv.Hop(me, peer)
	if err != nil {
		// The three ways this fails are all worth distinguishing for a user, and `Hop`'s
		// own error already does: not in the roster at all, the same party twice, or two
		// parties who are not adjacent and therefore share no hop.
		return nil, fmt.Errorf("this invitation does not put you and that peer on one hop: %w", err)
	}
	gate, err := ceremony.NewCandidateGate(inv, hop, me, peer)
	if err != nil {
		return nil, err
	}
	return &ceremonyID{inv: inv, hop: hop, gate: gate, me: me, peer: peer, certPEM: myCertPEM, keyPEM: myKeyPEM}, nil
}
