package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"nib/internal/atomicfile"
	"nib/internal/ceremony"
	"nib/internal/p2p"
	"nib/internal/safe"
	"nib/internal/sign"
	"nib/internal/vault"
)

// The delivery leg (P08.S05d).
//
// A delivery leg hands a FINISHED ceremony document to a party who already signed it. It is not a
// hop: nothing is signed, nobody is asked anything, and the two ends have met before. Everything
// in this file exists because that makes it different from the transfer path it borrows.
//
// # Why both gates are unattended, and why that is not a hole in L2
//
// L2 says no path carries document bytes unconfirmed, and `runVerification` refuses a nil
// `Verifier` outright so a forgotten gate fails closed. Auto-confirming here is legitimate for a
// narrower and stronger reason than "the pin has not changed":
//
//	**These same two identities already completed a spoken check on this pin.** D22 makes the
//	convener a hub, so every delivery leg is convener↔party, and every party ran `runVerification`
//	against the convener at its own hop before it signed. The words are derived from both identity
//	fingerprints and the channel binding; re-deriving them on a second channel between the same
//	two keys re-asks a question that was answered, by the same people, about the same pin.
//
// That argument does NOT extend to a leg between two parties who never met, and the distinction
// matters: if the round ever delivered party-to-party, these gates would have to come back. Stated
// here rather than left implicit, because the weaker "the pin is unchanged" argument would license
// the wider case without anyone noticing the difference.
//
// # And the scope is structural, not a convention
//
// `TestTheUnattendedGatesHaveOneDoor` asserts that the two types below are referenced only from
// `deliverOneLeg`. An auto-confirming verifier reachable from `/api/session/arm` would silently
// remove the spoken check from the interactive path, which is the exact failure `runVerification`'s
// nil check was written to make impossible.

// autoVerifier confirms the spoken check without a human. See this file's header for why that is
// sound on a delivery leg and only there.
type autoVerifier struct{}

func (autoVerifier) ConfirmVerification(string) (bool, error) { return true, nil }

// autoAccepter accepts the delivered document without a human.
//
// **It does NOT write the file.** `sessionAccepter.Accept` persists inside the gate so `ackOK`
// means "the bytes are on disk" (P08.S05a), and a delivery leg needs the same guarantee — but it
// needs a different filename rule and a different verification, so the write is done by the caller
// through `saveDelivered` and this gate only reports consent. The ordering property is preserved
// by `deliverOneLeg`, which persists before it returns to `ReceiveDocument`'s acknowledgement.
type autoAccepter struct {
	// verify is the recipient's own check of what arrived. A gate that accepted first and checked
	// afterwards would acknowledge a document it then refused, which is the false receipt
	// P08.S05a removed from this path one layer down.
	verify func(doc []byte) error
	// saved receives the accepted document so the caller can persist it before the ack. nil is a
	// programmer error rather than "do not save", and it fails closed below.
	save func(doc []byte) error
}

func (a autoAccepter) Accept(_, doc []byte) (bool, error) {
	if a.verify == nil || a.save == nil {
		// Fail closed, for `runVerification`'s reason: a caller that forgot is not a caller that
		// meant to skip. Declining costs a retry; accepting would acknowledge bytes nothing checked.
		return false, errors.New("delivery accepter is not wired: no verify or save")
	}
	if err := a.verify(doc); err != nil {
		return false, err
	}
	if err := a.save(doc); err != nil {
		return false, err
	}
	return true, nil
}

// errDeliveryNotWired is returned when a leg is asked to run with no channel.
var errDeliveryNotWired = errors.New("delivery leg has no channel")

// deliverOneLeg is the ONE door onto the unattended gates (ADR-009), and it is what makes the
// guard above non-vacuous while the round that calls it is still being built (S05g).
//
// `send` chooses the side: the convener sends, the recipient receives. Both directions are here
// because the gates are the same fact about the same leg, and splitting them would give the two
// auto types two doors.
func (s *Server) deliverOneLeg(ch p2p.Channel, cer *ceremonyID, myFP []byte, pdf []byte, send bool) ([]byte, error) {
	if ch.Stream == nil {
		return nil, errDeliveryNotWired
	}
	if send {
		// PeerGatesUnattended: the round armed the far side itself, so the sender's third
		// deadline waits on no human. See p2p.DeliveryLegBudget.
		return nil, p2p.SendDocument(ch, pdf, myFP, autoVerifier{}, p2p.PeerGatesUnattended)
	}
	var kept []byte
	doc, err := p2p.ReceiveDocument(ch, autoAccepter{
		verify: func(d []byte) error { return s.checkDeliveredPayload(cer, d) },
		save: func(d []byte) error {
			// A termination is TOLD rather than left in `~/nib` as a document — there is none —
			// but the attestation itself IS persisted, in the same breath and for the same reason
			// the document is: `ackOK` must mean the bytes reached disk. `ErrNotStored` is the
			// wire's word for a write that failed after consent, and it is what the sender needs
			// in order to try again rather than record a delivery that did not happen.
			//
			// The payload is decoded twice on this path — once by `verify` to route and check it,
			// once here to store and tell. Cheap (it is a few hundred bytes of JSON) and the
			// alternative is threading a decoded value through a `func([]byte) error` callback
			// pair, which would give the gate a second shape to be wired wrongly.
			if t, ok := asTermination(d); ok {
				if werr := ceremony.WriteTermination(defaultOutputDir(), t); werr != nil {
					return fmt.Errorf("%w: %v", p2p.ErrNotStored, werr)
				}
				s.tellEndState(cer, t)
				kept = d
				return nil
			}
			path, serr := s.saveDelivered(cer, d)
			if serr != nil {
				return serr
			}
			kept = d
			s.sess.setReceived(armDelivery, &receivedInfo{Path: path, Peer: cer.peer})
			return nil
		},
	}, myFP, autoVerifier{})
	if err != nil {
		return nil, err
	}
	if kept == nil {
		// Belt and braces: ReceiveDocument returns the document only on accept, and accept only
		// runs after `save`. A nil here would mean the two disagreed.
		return nil, errors.New("delivery accepted but nothing was kept")
	}
	return doc, nil
}

// checkDelivered is the delivery arrival's gate, and it is NOT `checkArrival` (P08.S05d).
//
// # Why a second door rather than a reuse
//
// `checkArrival` composes three of the same checks, and reusing it would refuse exactly the
// documents this round exists to deliver. Its `recordOutlivesBudget(rec, now, 0)` arm exists to
// stop a party **being collected into a proceeding that has ended** (P08.S04a, D28). A delivery is
// the opposite case: the proceeding HAS ended, correctly, and this is its result — and S05b
// reserved grace precisely so the round may run at or past `Expires`. A shared door would make the
// last party's copy undeliverable on a ceremony that finished on time.
//
// ADR-009 is satisfied by the two doors being two RULES, not one rule with two spellings, and each
// says so at the other. Anything later that "unifies" them reintroduces the refusal above.
//
// # What it checks
//
//  1. the record is present, well-formed and signed by the convener (`CheckRecord`);
//  2. it is the ceremony this party accepted (`MatchesRecord`, the same binding `checkArrival` uses);
//  3. every obliged signer actually signed — a partial document is not a delivery;
//  4. it extends this party's OWN contribution byte for byte, so a document that dropped this
//     party's signature and re-collected the others cannot land as their copy.
//
// # Its blind spots, stated
//
// It cannot tell a convener who delivered an older complete document from one who delivered the
// newest, because nothing here carries a version and the mirror holds one `document.pdf` per
// ceremony. It does not re-verify the OTHER parties' signatures cryptographically beyond what
// `Attestations` reports as valid. And (4) is a prefix test, so a trailing incremental update
// signed by anyone in the roster still extends it — which is what makes a co-signature legal at
// all, and is why (3) rather than (4) is what refuses an incomplete document. **And (4) is
// skipped entirely when this machine has no readable mirror** — a convener who did not sign, or
// a moved `~/nib` — because refusing there would make a delivery undeliverable for a local
// storage problem while (3) still holds.
func (s *Server) checkDelivered(cer *ceremonyID, pdf []byte) error {
	if cer == nil {
		return errors.New("a delivered document arrived for no ceremony")
	}
	now := time.Now()
	rec, err := ceremony.CheckRecord(pdf, now)
	if err != nil {
		return fmt.Errorf("this delivered document's ceremony record could not be checked: %w", err)
	}
	if err := cer.inv.MatchesRecord(rec); err != nil {
		return err // unwrapped, for checkArrival's reason: the sentence already names the axis
	}
	proc := ceremony.ProceedingOf(pdf, now)
	signed, obliged := p2p.Completeness(p2p.Attestations(sign.Verify(pdf), proc), proc)
	if obliged == 0 || signed != obliged {
		return fmt.Errorf("this delivered document carries %d of %d obliged signatures, so the "+
			"proceeding it claims to complete is not complete", signed, obliged)
	}
	// **The mirror is read directly, NOT through `persistedFor` (this slice's own review).** That
	// function's parameter is *"the inbound document this hop was offered"*, and it answers
	// "is what is stored an answer to THIS question" — `persistedFor(nil)` would mean the empty
	// question, which its `HasPrefix(pdf, nil)` arm answers `true` for anything. It would work
	// today by accident of that identity and stop working the moment the miss rules change.
	// The question here is simply *what did this machine sign*, which is the mirror.
	_, mine, merr := ceremony.ReadMirror(defaultOutputDir(), cer.inv.ID, now)
	if merr != nil {
		// **A miss, not a refusal, and the asymmetry is deliberate.** The commonest reason there
		// is no mirror is that this party is the CONVENER who did not sign, or a machine that
		// restarted with `~/nib` moved. Refusing on an unreadable mirror would make a delivery
		// undeliverable for a local storage problem, and clause 3 above has already established
		// that every obliged party signed — which is the property that matters. Recorded as a
		// blind spot rather than papered over.
		return nil
	}
	if len(mine) > 0 && !bytes.HasPrefix(pdf, mine) {
		return errors.New("this delivered document does not extend the copy this machine signed, " +
			"so it is not this proceeding's result as this party contributed to it")
	}
	return nil
}

// deliveredName is the delivered file's name, and it is DETERMINISTIC (P08.S05d).
//
// **It does not reuse `receivedName`.** That builder reads `time.Now()` inside itself at second
// granularity, so two documents from one peer inside a second collide and the second silently
// overwrites the first — measured at P08.S05a's first honest tier-6 run and filed as /pending 342.
// A delivery round hands one machine several documents in quick succession, which is exactly that
// window.
//
// The name carries the ceremony id, so two ceremonies never collide, and a human half derived from
// the record's own intent, so a finished lease is distinguishable from Monday's copy by name alone
// rather than by opening it. The id goes LAST and in full: a truncated id is a collision nobody
// can see, and the human half is the part a user scans.
func deliveredName(rec ceremony.Record) string {
	slug := labelSlug(rec.Intent)
	if len(slug) > 48 {
		slug = strings.TrimRight(slug[:48], "-")
	}
	if slug == "" {
		slug = "ceremony"
	}
	return slug + "-" + rec.ID + ".pdf"
}

// saveDelivered writes the delivered document under ~/nib, OUTSIDE ~/nib/ceremonies/.
//
// **Outside, and that is load-bearing rather than tidy.** The mirror holds one `document.pdf` per
// ceremony and `persistedFor` reads it as "this hop's previously co-signed output". A finished
// N-party document written there would be returned as a hop's re-delivery — handing a later
// party's completed document back to an earlier one as though it were that hop's own result.
//
// `signed/` because the document IS signed, which is where `receivedSubdir` already routes a signed
// arrival; the delivered file is an ordinary signed PDF as far as everything downstream is concerned.
func (s *Server) saveDelivered(cer *ceremonyID, pdf []byte) (string, error) {
	rec, err := ceremony.CheckRecord(pdf, time.Now())
	if err != nil {
		return "", fmt.Errorf("%w: %v", p2p.ErrNotStored, err)
	}
	// **Through `deliveredPathFor`, which is the one door for this path (ADR-009).** The two were
	// separate `filepath.Join(defaultOutputDir(), "signed", deliveredName(rec))` expressions —
	// this writer and `alreadyDelivered`'s reader — and P08.S06 made the disagreement expensive:
	// `closeOutReason` now MOVES a ceremony directory on the strength of that stat, so a writer
	// and a reader drifting apart would leave a delivered ceremony live forever, or move one whose
	// document had not arrived. Noticed while tracing who writes the path this slice reads.
	path := deliveredPathFor(rec)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("%w: %v", p2p.ErrNotStored, err)
	}
	if err := atomicfile.WriteDurable(path, pdf, 0o600); err != nil {
		return "", fmt.Errorf("%w: %v", p2p.ErrNotStored, err)
	}
	return path, nil
}

// deliveryHop is the rendezvous index a delivery leg to `partyFP` publishes and fetches at.
//
// **Past every hop index, and derivable by both ends with no format change (P08.S05g).** Hops run
// `0 .. len(roster)-2` — `hopBetween` numbers the non-convener parties — so `len(roster) + k` can
// never collide with one. Everything that addresses the DHT is already keyed off a plain `int`:
// `HopSeed`, `RecordKey` and `RecordSalt` all interpolate it and validate nothing, so this needed
// no new derivation, no new field and no version bump.
//
// **A leg sharing the hop's index would be a silent loss, not a refusal.** `RecordSalt`'s own doc
// says why two parties on one hop need distinct targets: *"sharing one target would mean the
// higher-seq write silently clobbered the other"*. A delivery arm publishing at its hop's index
// would clobber, or be clobbered by, that hop's own record.
func deliveryHop(inv ceremony.Invitation, partyFP string) (int, error) {
	k, err := inv.Hop(inv.ConvenerFingerprint, partyFP)
	if err != nil {
		return 0, err
	}
	return len(inv.Roster) + k, nil
}

// deliveryCeremony builds the `ceremonyID` a delivery leg uses: the same invitation and the same
// pinned counterparty, at `deliveryHop`'s index.
//
// It is an ORDINARY ceremonyID, which is the point — `feedCandidates`, `publishCandidates` and
// `punchBudgetFor` all key off `c.hop` and `c.inv`, so the delivery rendezvous reuses the whole
// tier ladder rather than reimplementing a second one beside it.
func (s *Server) deliveryCeremony(inv ceremony.Invitation, me, peer string, certPEM, keyPEM []byte) (*ceremonyID, error) {
	hop, err := deliveryHop(inv, deliveryParty(inv, me, peer))
	if err != nil {
		return nil, err
	}
	gate, err := ceremony.NewCandidateGate(inv, hop, me, peer)
	if err != nil {
		return nil, err
	}
	return &ceremonyID{inv: inv, hop: hop, gate: gate, me: me, peer: peer, certPEM: certPEM, keyPEM: keyPEM}, nil
}

// deliveryParty is whichever end of this leg is NOT the convener — the party the leg is about, and
// therefore the one whose index names the rendezvous. The convener delivers to everyone, so keying
// on it would give every leg one target.
func deliveryParty(inv ceremony.Invitation, me, peer string) string {
	if strings.EqualFold(me, inv.ConvenerFingerprint) {
		return peer
	}
	return me
}

// deliveredPathFor is where this party's copy of a finished ceremony lands, and it is the CHECK
// for whether the round already reached this machine (P08.S05g finding (f)).
//
// Without it, every unlock re-arms every ceremony this machine ever signed — holding a listener
// open for proceedings that finished months ago. That is the residue D29's prune exists to stop,
// arriving through a door the prune does not watch. The deterministic name S05d built is what
// makes "already delivered" answerable from the filesystem alone, with no extra bookkeeping.
func deliveredPathFor(rec ceremony.Record) string {
	return filepath.Join(defaultOutputDir(), "signed", deliveredName(rec))
}

func alreadyDelivered(rec ceremony.Record) bool {
	_, err := os.Stat(deliveredPathFor(rec))
	return err == nil
}

// armForDelivery opens this party's delivery rendezvous and serves ONE leg from the convener.
//
// It is an ordinary ceremony arm at an extraordinary hop index: a shared endpoint, a rendezvous on
// it, a pinned-peer listener, and the publish that lets the convener find this machine off-LAN.
// Everything about the tier ladder is reused rather than rebuilt — see `deliveryCeremony`.
//
// **It takes S05c's delivery SLOT**, so it coexists with whatever the user has open. Before that
// slice this could only have run by displacing the interactive arm, which is why the round and the
// second slot were split apart in the first place.
func (s *Server) armForDelivery(ctx context.Context, inv ceremony.Invitation, cert, key []byte, me string) error {
	peerFP, err := hex.DecodeString(inv.ConvenerFingerprint)
	if err != nil || len(peerFP) != sha256.Size {
		return errors.New("this ceremony's convener fingerprint is not a fingerprint")
	}
	cer, err := s.deliveryCeremony(inv, me, inv.ConvenerFingerprint, cert, key)
	if err != nil {
		return err
	}
	if err := cer.setupSharedEndpoint("0.0.0.0:0", s.configDir); err != nil {
		cer.close()
		return err
	}
	ln, err := p2p.QUICListenOn(cer.end, cert, key, peerFP)
	if err != nil {
		cer.close()
		return err
	}
	armCtx, cancel := context.WithCancel(ctx)
	if !s.sess.armDeliveryForCeremony(cer, cer.end.LocalAddr().String(), cancel) {
		cancel()
		ln.Close()
		cer.close()
		return errors.New("a delivery arm is already open on this machine")
	}
	// **The window is ENFORCED, not merely reported (this slice's own review).** `armIn` stamps
	// `until` from `armWindowFor(armDelivery, cer)` — what remains of the record's `Expires` plus a
	// hop budget of grace — and `status()` shows it. Nothing fired on it: the first cut of this arm
	// looped on `Accept` until something else disarmed it, so a delivery arm lived until the
	// process exited and the TRIPWIRE's *"how long it stays open"* paragraph described a bound the
	// code did not keep. `runSession` has had a timer on its own window since P05; this is the same
	// rule for the second slot.
	window := armWindowFor(armDelivery, cer)
	go func() {
		defer safe.Recover("delivery arm")
		defer s.sess.disarmCeremony(cer)
		defer ln.Close()
		timer := time.AfterFunc(window, func() {
			defer safe.Recover("delivery arm expiry")
			// Closing the listener is what ends the Accept below — the same mechanism
			// `runSession`'s deadline uses, rather than a second cancellation path.
			ln.Close()
		})
		defer timer.Stop()
		// ── The LINK tier, which this arm did not have (P08.S05h) ────────────────────────
		//
		// **`armForDelivery`'s own doc claimed "everything about the tier ladder is reused rather
		// than rebuilt", and tier 1 was missing at both ends of a delivery leg.** Measured: a
		// nine-party `--lan` relay emitted **78 packets destined off the link** against P03's
		// exit criterion of zero, and a stack trace on `ensureBootstrapped` named this goroutine —
		// `armForDelivery` → `publishWhenSlow` → `publishCandidates` → the DHT bootstrap, two per
		// party per transport. Nothing was published; the packets are the bootstrap the publish
		// path performs on its way, which is why counting publishes alone found nothing.
		//
		// The two halves are one fix and neither works alone, which is ADR-011's own framing of
		// the same seam one layer up:
		//
		//   * **announce**, so a convener browsing this link finds this arm without the DHT;
		//   * **answer seekers**, so `cer.watchingLink` is set and `holdDHT` stops taking its
		//     `ns == 0` arm — the one that returns immediately after a flat `browseWindow`. With
		//     it set, the hold becomes `lanFirstBudget` measured from the watch, renewed by every
		//     resolved sighting of the convener, exactly as a hop arm's is.
		//
		// **On THIS path the answer itself is incidental and the sighting is the point**, which is
		// worth saying because the two arrive together and only one is wanted. A hop arm's answer
		// lands inside a peer's browse: the hop dial announces itself before it browses, so the
		// arm's reply is heard. A delivery dial does neither — `browsePeers` is a pure listener,
		// and the dial has no handshake listener to announce an address for, so announcing one
		// would be a statement about a socket nothing accepts on. The sightings therefore come
		// from the convener's HOP announcements, and the answers they provoke are heard by nobody
		// who is looking. They cost link multicast and nothing off it. **The two cannot be
		// separated**: `answerLoop` skips `resolve` and `sighted` together when a caller reports
		// itself idle, so suppressing the answer suppresses the evidence with it.
		//
		// **`wanted` is nil on purpose, and it is the one place this differs from a hop arm.**
		// There it is `!cer.hasSigned()` — "a peer that reaches us now can only be re-delivering,
		// and it already has the address". A delivery arm has no such state: it is one-shot and
		// spent by the leg that succeeds, so the goroutine returning IS the stop condition and a
		// predicate would be a second one that could disagree with it.
		//
		// Neither half is fatal. A host with no usable interface announces nothing, never sets
		// `watchingLink`, and falls through to the publish below — which is the correct answer for
		// a machine that cannot be found on a link it is not on.
		//
		// **What renews the hold is the CONVENER's own announcements, and that is measured rather
		// than assumed.** A delivery arm never hears a browse — `browsePeers` is a pure listener
		// and emits nothing — so the sightings come from the convener announcing while it dials
		// the remaining hops, which `resolve` matches to this arm's expected peer. Probed on a
		// nine-party `--lan` run: 76, 54, 40 and 20 sightings on the four delivery hop indices,
		// so the renewal is real and the zero is not a race against `lanFirstBudget`.
		//
		// **The stated limit: once the hops finish, the convener goes quiet.** An arm whose round
		// is run a day later hears nothing for 30 s and publishes — which is right, because the
		// parties are no longer demonstrably in the same room, and being findable off-LAN is then
		// the only way the round reaches them at all.
		if ann := s.watchLink(armCtx, cer, quicEndpointAnnounce{ln.Addr()}, cert, peerFP, nil); ann != nil {
			defer ann.Close()
		}
		// The publish is what makes the arm findable off-LAN, and it holds the link's window
		// first exactly as every other publish does (ADR-011) — a delivery round on one office
		// network must emit nothing either.
		//
		// **It races the answerer above for `watchingLink`, and the race is wide and one-sided.**
		// `holdDHT` waits `browseWindow` (2 s) before reading, and the answerer sets the flag as
		// soon as its socket opens; losing the race costs one early publish, never a wrong one.
		// Pre-setting the flag here would close it and is refused: a host whose socket never opens
		// would then claim to be watching a link it is not on, and hold the DHT for 30 s on
		// evidence that does not exist.
		go func() {
			defer safe.Recover("delivery publish")
			publishWhenSlow(armCtx, cer, transportQUIC, browseWindow)
		}()
		for {
			conn, aerr := ln.Accept()
			if aerr != nil {
				return // net.ErrClosed: disarmed, or the arm's window elapsed
			}
			_, derr := s.deliverOneLeg(conn.Channel, cer, nil, nil, false)
			conn.Close()
			if derr == nil {
				return // delivered; this arm is spent, exactly like every other one-shot arm
			}
			// A refused or failed leg does NOT spend the arm: refusal is free for whoever
			// connects and expensive for the party still waiting for their copy. Same rule
			// runSession states for a stray dial.
		}
	}()
	return nil
}

// rearmDeliveries re-establishes a delivery arm for every ceremony this machine signed and has not
// yet received its copy of (`/pending 323`(a)).
//
// **Hung off `adoptVault`, which is the ONE door for "the vault just opened".** That function's own
// comment records why: its four lines were written twice and P07.S02 hangs the pending-open drain
// off this moment, warning that a second copy *"would mean a hand-off queued against a locked
// instance opens when the user unlocks through one route and silently never opens through the
// other"*. This is the same fact for a different queue.
//
// **Why it is needed at all:** a party who restarts between signing and delivery had, before this,
// no way back onto the network for the rest of the ceremony — the arm that would receive their copy
// existed only for as long as the process that made it. A named search over `internal/` for
// `re-?arm|resumeCeremon|restoreArm|atStartup|onStart` found nothing, and `Server.New` starts
// nothing ceremony-related.
//
// Best-effort per ceremony: one that will not arm must not stop the others, and there is no
// response to write a failure into. Failures reach the user through the sticky notice.
func (s *Server) rearmDeliveries(v *vault.Vault) {
	// **Nil-guarded, and not defensively for its own sake.** This runs on a detached goroutine
	// with no response to write into, so a panic here is caught by `safe.Recover` and then reaches
	// nobody — the failure mode is a party silently never re-arming. `adoptVault` only ever passes
	// a live vault; the guard is what makes that a stated contract rather than a fact a future
	// caller has to rediscover.
	if v == nil {
		return
	}
	stored, err := ceremony.ListStored(defaultOutputDir(), time.Now())
	if err != nil {
		return
	}
	cert, key, err := identity(v)
	if err != nil {
		return
	}
	myFP, err := sign.Fingerprint(cert)
	if err != nil {
		return
	}
	me := hex.EncodeToString(myFP)
	for _, st := range stored {
		if st.State != ceremony.LoadOK {
			continue // unreadable: nothing here can be trusted enough to act on
		}
		rec, _, rerr := ceremony.ReadMirror(defaultOutputDir(), st.ID, time.Now())
		if rerr != nil || alreadyDelivered(rec) {
			continue
		}
		text, ok := v.CeremonyInvitationFor(st.ID)
		if !ok {
			continue // this machine holds no invitation for it — nothing to derive a rendezvous from
		}
		inv, ierr := ceremony.ParseInvitation(text)
		if ierr != nil {
			continue
		}
		if strings.EqualFold(me, inv.ConvenerFingerprint) {
			continue // the convener DELIVERS; it does not wait to be delivered to
		}
		// **"Has this proceeding ended?" — asked HERE, and anchored on the invitation** (`/pending
		// 354`). It used to be `st.Ended != ""` in the skip fifteen lines above, and that field is
		// the one anchor this decision may not use.
		//
		// `LoadState` computes `Ended` from the `record.json` sitting in the same directory as the
		// termination, and says in terms why that is acceptable there and nowhere else: *"this is a
		// listing, it renders a word to a user, and it **authorises nothing**. A gate that REFUSES
		// on a termination must anchor on the document or the invitation instead, because a planted
		// matching pair verifies against itself."* Deciding **not to arm** is an authorisation, so a
		// matching (record, termination) pair dropped into `~/nib/ceremonies/<id>/` suppressed the
		// arm and the party never received its real copy.
		//
		// `inv` is the anchor a planted file cannot control, and this is the same shape
		// `checkDeliveredPayload` was corrected to at P08.S05h — ADR-009: one rule, and both doors
		// take it. It could not be asked before the loop because the invitation is only in hand
		// eight lines up; moving the question down to meet it is the whole change.
		//
		// **Fails OPEN, toward arming.** If the record does not match the invitation, or the
		// termination does not verify, this machine does NOT conclude the proceeding is over — it
		// arms. Suppressing on unverifiable evidence is the defect; a needless arm costs one slot
		// until the next unlock.
		if merr := inv.MatchesRecord(rec); merr == nil {
			if _, terr := ceremony.ReadTermination(defaultOutputDir(), rec); terr == nil {
				continue // a verified end state, on an anchor a planted file cannot forge
			}
		}
		if aerr := s.armForDelivery(context.Background(), inv, cert, key, me); aerr != nil {
			s.sess.noteFailure(armDelivery, "delivery-arm-failed",
				"Nib could not listen for your copy of a signed document.",
				"A ceremony you signed has not delivered your copy yet, and this machine could "+
					"not open the connection that receives it. Reason: "+aerr.Error())
			return // one slot: a second ceremony cannot arm behind a failure either
		}
		return // the slot holds one; the next unlock takes the next ceremony
	}
}

// EnableDeliveryRearm tells this Server it is backing a real Nib process, so an unlock may
// re-establish a delivery arm (P08.S05g).
//
// **A process concern, and the twin of `DisarmSession`.** That method exists because tearing down
// an armed listener belongs to whoever owns the process lifetime, not to whoever constructs a
// `Server`; this is the same fact at the other end. Constructing a `Server` must not put a socket
// on the network — a test does that dozens of times, in temporary directories, with `$HOME` still
// pointing at the developer's real `~/nib`.
//
// Default OFF, which is the safe direction and the opposite of the `toolbarStyle` mistake this
// repo already paid for: a flag whose default does something surprising is a loaded gun, and this
// one's default does nothing at all.
func (s *Server) EnableDeliveryRearm() { s.deliveryRearm.Store(true) }

// deliveredMarkerPath is where the convener records that one party acknowledged its copy.
//
// **Beside the mirror, because this is durable ceremony state and that is where the rest lives**
// (P08.S05g finding (e)). A marker per party rather than one file listing them: two legs of one
// round can finish in either order and a shared file makes the second write clobber the first —
// the same shape `RecordSalt` refuses for two parties on one BEP-44 target.
func deliveredMarkerPath(id, partyFP string) (string, error) {
	dir, err := ceremony.MirrorDir(defaultOutputDir(), id)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "delivered", strings.ToLower(partyFP)), nil
}

func markDelivered(id, partyFP string) error {
	path, err := deliveredMarkerPath(id, partyFP)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return atomicfile.WriteDurable(path, []byte(time.Now().UTC().Format(time.RFC3339)+"\n"), 0o600)
}

func wasDelivered(id, partyFP string) bool {
	path, err := deliveredMarkerPath(id, partyFP)
	if err != nil {
		return false
	}
	_, serr := os.Stat(path)
	return serr == nil
}

// endedByPath names the convener's own record of WHICH party ended a proceeding.
//
// **Separate from the termination object on purpose.** P08.S04b excluded the party and the time
// from the attestation deliberately — the binding is the roster hash alone, so no canonical form
// is needed — and that decision is not reopened here. This is convener-local bookkeeping about a
// round, in the same mirror directory and with the same durability as the `delivered/` markers,
// and nothing signs it because nothing outside this machine reads it.
func endedByPath(id string) (string, error) {
	dir, err := ceremony.MirrorDir(defaultOutputDir(), id)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "ended-by"), nil
}

// markEndedBy records the party that ended a proceeding. **Write-once, first ender wins**, which
// is `ceremony.WriteTermination`'s rule and is here for the same reason.
//
// Nothing on the arrival path refuses a hop because a proceeding has already ended — the deadline
// gate reads `Expires`, not an end state — so a convener CAN drive another hop after a decline and
// collect a second one. Overwriting would then move the marker to the newer decliner, and the
// round would skip them and walk the FIRST one: the impossible leg, restored, with its 300 s cost
// intact and the report naming the wrong party. A proceeding ends once, at the first refusal.
func markEndedBy(id, partyFP string) error {
	path, err := endedByPath(id)
	if err != nil {
		return err
	}
	if prior := endedBy(id); prior != "" {
		if strings.EqualFold(prior, partyFP) {
			return nil // idempotent: the same decline recorded twice
		}
		return fmt.Errorf("this proceeding is already recorded as ended by %s, so it cannot also "+
			"be ended by %s — an end state is reached once", prior[:12], partyFP[:12])
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return atomicfile.WriteDurable(path, []byte(partyFP+"\n"), 0o600)
}

// endedBy reports the party that ended this proceeding as recorded, or "" when none is.
//
// **No case fold, because BOTH consumers compare with `EqualFold`** — the walk's skip and
// `markEndedBy`'s write-once check — and a normalisation nothing depends on is dead weight held
// up by a test. An earlier cut folded at both the read and the
// write, which made each unfalsifiable — either alone gives the same answer, so removing one left
// every test green. Dropping both and leaving the comparison to fold is the shape with no
// redundant half to rot.
//
// **`""` is not only "nobody ended it".** It also covers an unreadable marker and a `MirrorDir`
// that refuses the id. Both fail in the safe direction — the round walks the party as it did
// before this rule existed — but the two are not the same fact, and a caller that needs to tell
// them apart must not read this function's answer as the first one.
func endedBy(id string) string {
	path, err := endedByPath(id)
	if err != nil {
		return ""
	}
	b, rerr := os.ReadFile(path)
	if rerr != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// deliveryOutcome is one party's result in a round, reported to the caller so a re-run is a
// decision the user makes rather than a retry loop nobody can see.
type deliveryOutcome struct {
	Fingerprint string `json:"fingerprint"`
	Label       string `json:"label,omitempty"`
	Delivered   bool   `json:"delivered"`
	// Skipped means the round did not attempt this leg, for one of TWO reasons, and **`Delivered`
	// is what distinguishes them** — the already-acknowledged branch carries no `Reason` at all,
	// so a consumer branching on that gets "". True: the party already acknowledged an earlier
	// run, and this round did not need to repeat it. False: the party ENDED the proceeding, and
	// `Reason` says so.
	Skipped bool   `json:"skipped,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

// runDeliveryRound walks the roster and delivers the finished document to every party that has not
// already acknowledged it.
//
// # Why it is a route the convener triggers and not a background orchestrator
//
// **The relay has never had an in-product driver.** `pairrepro.sh` hand-drives every hop with
// `/api/session/initiate`; no production function walks a roster, and a named search at this
// slice's deepdive found none. So an autonomous round would be the product's first self-directed
// multi-step network flow, with its own retry policy, running unattended for the ceremony's life.
//
// The criteria do not ask for that. C08 is *"the finished document reaches every party"* and C10 is
// *"a delivery round **re-run** after a mid-round failure leaves exactly one file per party"* — and
// a re-run is something somebody runs. As a route, C10's re-run is literally a second POST rather
// than bookkeeping inside a daemon, and the product is not committed to autonomous orchestration,
// which is a shape decision belonging with P06's surface. Recorded as an assumption Dan can
// reverse; the RECIPIENT half stays autonomous, because arming a listener at unlock is what the
// product already does for every other arm.
//
// # Idempotence is at the convener, not only at the recipient
//
// S05d's deterministic filename means a second delivery to one party overwrites itself, so the
// recipient cannot accumulate copies. C10's harder half is that a re-run must reach the party that
// FAILED and skip the ones that succeeded — otherwise a crash mid-round re-delivers to everyone.
// That needs the acknowledgement recorded durably, which is `markDelivered`.
func (s *Server) runDeliveryRound(ctx context.Context, v *vault.Vault, rec ceremony.Record, pdf []byte, addrs map[string]string) ([]deliveryOutcome, error) {
	// Nil-guarded for `rearmDeliveries`'s reason, and one more: the convener check below NEEDS the
	// identity to know who this machine is, so it cannot precede the load. Without this guard the
	// honest refusal *"you are not the convener"* is reached only by a machine that could already
	// load an identity, and one that could not panics instead.
	if v == nil {
		return nil, errors.New("no vault is open, so this machine cannot say who it is in this ceremony")
	}
	cert, key, err := identity(v)
	if err != nil {
		return nil, err
	}
	myFP, err := sign.Fingerprint(cert)
	if err != nil {
		return nil, err
	}
	me := hex.EncodeToString(myFP)
	conv := convenerFingerprintOf(rec)
	if !strings.EqualFold(me, conv) {
		return nil, errors.New("only the convener delivers a finished document: this machine is " +
			"a party to this ceremony, not the one that convened it")
	}
	// **What the round carries depends on how the proceeding ENDED (P08.S05e, C06).** A completed
	// ceremony delivers the finished document; a declined one delivers the convener's signed
	// termination, because there is no finished document and the parties who already signed are
	// otherwise left believing it is still travelling. Same walk, same per-party rendezvous, same
	// acknowledgement markers — a second round would have duplicated all three.
	payload := pdf
	t, terr := ceremony.ReadTermination(defaultOutputDir(), rec)
	switch {
	case terr == nil && t.State == ceremony.StateDeclined:
		b, merr := json.Marshal(t)
		if merr != nil {
			return nil, merr
		}
		payload = b
	case terr == nil:
		// A completed ceremony: the finished document is the payload, as it always was.
	case errors.Is(terr, ceremony.ErrNoTermination):
		// The ordinary case — the proceeding has not ended — and it must never read as damage.
	default:
		// **`ErrBadTermination` is a planted file far more likely than a corrupted one**, in
		// `ReadTermination`'s own words, and swallowing it here shipped the partially-signed
		// mirror document to every party instead of the attestation. Recipients refuse it at
		// `checkDelivered`'s completeness clause, so nothing bad lands — the user simply gets N
		// failures whose reasons never mention the end state that could not be read. A round that
		// cannot tell what it is carrying does not start.
		return nil, fmt.Errorf("this ceremony's end state is present and does not check out, so "+
			"this machine will not start a round without knowing what it is delivering: %w", terr)
	}
	// **The party that ENDED the proceeding is not walked, and that is not an optimisation.**
	// Declining runs `declineCeremony` on the refusing machine, which since P08.S06 routes
	// straight to `closeOutCeremony` — moving the folder out of the live set and dropping the
	// pins, the secrets and the stored invitation — so that machine has nothing left to arm a
	// delivery rendezvous with; and
	// a party that refuses at its consent gate returns from `coSignExchange` before `rd.Store`,
	// so it holds no mirror for `checkDeliveredPayload` to check an attestation against. Walking
	// it cost the full `connectDeadline` — measured at tier 4 on 2026-09-02 as 300 s, reported as
	// `tried 0 address(es), none answered as the pinned peer: context deadline exceeded` — on
	// that round and on every re-run of it.
	//
	// **Stated as the ordinary case rather than as "impossible", because neither half is
	// absolute.** `declineCeremony` is best-effort: it returns early on a locked vault and only
	// logs when the close-out fails, so an invitation can survive a decline and `rearmDeliveries`
	// would read it back. The skip is therefore the right default and not a proof — and a party
	// wrongly skipped loses nothing they can act on, since they are the one who refused.
	ender := endedBy(rec.ID)
	// **How many legs this round will ATTEMPT, for the watcher's "n of N" (/pending 370).** The
	// roster length is the wrong figure: the convener is skipped, so is the party that ended the
	// proceeding, and so is anyone an earlier run already reached. Counting those in would show a
	// round stalling at "2 of 6" and finishing there, which reads as an abandoned round rather
	// than a complete one.
	walked := 0
	for _, party := range rec.Roster {
		if strings.EqualFold(party.Fingerprint, me) {
			continue
		}
		if ender != "" && strings.EqualFold(party.Fingerprint, ender) {
			continue
		}
		if wasDelivered(rec.ID, party.Fingerprint) {
			continue
		}
		walked++
	}
	attempt := 0
	out := make([]deliveryOutcome, 0, len(rec.Roster))
	for _, party := range rec.Roster {
		if strings.EqualFold(party.Fingerprint, me) {
			continue // the convener already holds it
		}
		res := deliveryOutcome{Fingerprint: party.Fingerprint, Label: party.Label}
		if ender != "" && strings.EqualFold(party.Fingerprint, ender) {
			res.Skipped = true
			res.Reason = "this party ended the proceeding, so Nib did not try to reach them: " +
				"declining removes this ceremony's invitation from their machine and they hold " +
				"no record to check an attestation against. They already know it is over."
			out = append(out, res)
			continue
		}
		if wasDelivered(rec.ID, party.Fingerprint) {
			res.Delivered, res.Skipped = true, true
			out = append(out, res)
			continue
		}
		// **A per-party invitation, minted here rather than read from the vault.** The convener
		// holds no invitation for its own ceremony — it ISSUES them — and each party's carries a
		// DIFFERENT secret, which is what makes their rendezvous targets distinct. The live tier-4
		// run found this: the round asked `CeremonyInvitationFor` and got a 409 saying this machine
		// holds no invitation, which was true and would have been true forever.
		// **The round stops when the caller goes away, and it did not (/pending 355).** The ctx
		// now reaches the race (see raceWithRendezvous), so a leg started after a disconnect fails
		// fast rather than burning `connectDeadline` — but "fast" is still one round trip per
		// remaining party, and there is nothing to learn from a leg whose result nobody will read.
		// Recorded rather than dropped, so the outcome list stays one row per party for a caller
		// that IS still there, which is every other way this context ends.
		if cerr := ctx.Err(); cerr != nil {
			res.Reason = "the request that started this round ended before this party was " +
				"reached, so Nib did not try: " + cerr.Error()
			out = append(out, res)
			continue
		}
		inv, ierr := convenerInvitationFor(v, rec, party)
		if ierr != nil {
			res.Reason = ierr.Error()
			out = append(out, res)
			continue
		}
		// **The leg goes on the record BEFORE it is attempted, not after (/pending 370).** This is
		// the call that can burn `connectDeadline`, and publishing after it would name every leg
		// exactly once it stopped being the interesting one. `endLeg` is deferred into the
		// iteration by the closure `beginLeg` returns, so an early `continue` below cannot leave a
		// ceremony reporting a leg that is no longer running.
		// `attempt`, not `len(out)`: the outcome list already holds the skipped parties — the
		// ender and anyone an earlier run reached — so counting it would number this leg against a
		// denominator it is not drawn from and show "4 of 2".
		attempt++
		endLeg := s.beginLeg(rec.ID, party.Label, attempt, walked)
		derr := s.deliverToParty(ctx, v, inv, party.Fingerprint, addrs[strings.ToLower(party.Fingerprint)], cert, key, myFP, payload)
		endLeg()
		if derr != nil {
			res.Reason = derr.Error()
			out = append(out, res)
			continue // one party's failure does not end the round; the re-run reaches them
		}
		if err := markDelivered(rec.ID, party.Fingerprint); err != nil {
			// **Delivered but not RECORDED, and the honest report is not "delivered".** A re-run
			// will deliver to them again, which the deterministic filename makes harmless — the
			// alternative, reporting success on an unrecorded leg, makes C10's "exactly one file
			// per party" depend on a write that failed.
			res.Reason = "delivered, but this machine could not record it: " + err.Error()
			out = append(out, res)
			continue
		}
		res.Delivered = true
		out = append(out, res)
	}
	return out, nil
}

// deliverToParty runs one leg: derive the delivery rendezvous, race the tiers to reach the party,
// and hand over the document through the unattended gates.
func (s *Server) deliverToParty(ctx context.Context, v *vault.Vault, inv ceremony.Invitation, partyFP, addr string, cert, key, myFP, pdf []byte) error {
	peerFP, err := hex.DecodeString(partyFP)
	if err != nil || len(peerFP) != sha256.Size {
		return errors.New("that party's fingerprint is not a fingerprint")
	}
	cer, err := s.deliveryCeremony(inv, inv.ConvenerFingerprint, partyFP, cert, key)
	if err != nil {
		return err
	}
	defer cer.close()
	if err := cer.setupSharedEndpoint("0.0.0.0:0", s.configDir); err != nil {
		return err
	}
	// **An optional typed address, and it is the same escape hatch `/api/session/initiate` has.**
	// Discovery for a delivery leg is the rendezvous at this party's own index — that is the
	// production path and what clause 1 is about. A caller that already knows where the party
	// listens may say so, and tier 4 needs that: `pairrepro.sh` gives its instances no DHT and no
	// multicast, so nothing there can resolve a rendezvous at all.
	//
	// **The harness OBSERVES the address rather than being told it** — it reads each party's own
	// `/api/session/status`, which reports the delivery arm's bound address. That distinction is
	// ADR-010's lesson: a harness handed a constant on both sides proves nothing, and one reading
	// what the product actually bound is making an observation.
	var cands []candidate
	switch {
	case addr != "":
		// `Source` is NOT optional: the per-source cap accounts an unset one to the zero value,
		// so one tier spends another's share and the drop report names the wrong tier as the
		// flooder. `TestEveryCandidateProducerNamesItsSource` caught this literal, which is what
		// that guard is for — a producer added later, not the ones anybody remembered to list.
		cands = []candidate{{Fingerprint: peerFP, Addr: addr, Transport: transportQUIC,
			Label: partyLabel(inv, partyFP), Source: sourceTyped}}
	default:
		// **The link, BEFORE the DHT (P08.S05h).** `feedCeremonyRace` decides how long to hold the
		// DHT tier from whether `cands` already contains a `sourceLAN` entry, and its comment says
		// why in as many words: *"`peerAddresses` browses BEFORE this runs, so a LAN candidate in
		// `cands` is the link having answered — not a guess about whether it will."* That held of
		// BOTH callers it had when it was written — `connect`'s `cands` come from `peerAddresses`
		// too — and a delivery leg is the third, which browsed nothing. So every delivery dial took
		// the 2 s `browseWindow` hold rather than the 30 s `lanFirstBudget` one, and a same-room
		// round reached for the DHT on a timer.
		//
		// Best-effort and never fatal: a browse that fails leaves `cands` empty, which is exactly
		// the state this leg was already in, and the rendezvous still races.
		// **The round's own pinned vault, threaded down rather than re-fetched.** `auth.go` states
		// the contract: a protected handler uses the vault pinned to its request, "so it stays
		// non-nil even if a concurrent vault import nils `s.vault` mid-request". `runDeliveryRound`
		// already holds and nil-checks that one; reaching for `s.unlockedVault()` here would be a
		// second door onto the same fact, with a nil branch the first door has already excluded.
		if found, ferr := findPeerOnLAN(v, peerFP); ferr == nil {
			cands = found
		}
	}
	conn, err := s.raceWithRendezvous(ctx, cer, cands, cert, key, peerFP, partyLabel(inv, partyFP), partyLabel(inv, partyFP))
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = s.deliverOneLeg(conn.Channel, cer, myFP, pdf, true)
	return err
}

func partyLabel(inv ceremony.Invitation, fp string) string {
	for _, p := range inv.Roster {
		if strings.EqualFold(p.Fingerprint, fp) {
			return p.Label
		}
	}
	return ""
}

// handleCeremonyDeliver is the round's door: the convener hands every party its copy.
//
// **Re-running it is the documented remedy, not a workaround (C10).** A round that failed partway
// is re-run by posting again; parties that acknowledged are skipped from the durable markers, and
// the party that failed is reached. The response reports every party's outcome so the user can see
// which one to worry about rather than being told "delivery failed" about a round that reached
// three of four.
func (s *Server) handleCeremonyDeliver(w http.ResponseWriter, r *http.Request) {
	v := vaultFrom(r)
	var req struct {
		Ceremony string `json:"ceremony"`
		// Addresses is an optional hint per party fingerprint, for a caller that already knows
		// where a party listens. Production discovery is the rendezvous; see deliverToParty.
		Addresses map[string]string `json:"addresses,omitempty"`
	}
	if err := readJSON(r, &req); err != nil {
		httpError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Ceremony == "" {
		httpError(w, http.StatusBadRequest, "this request names no ceremony to deliver")
		return
	}
	rec, pdf, err := ceremony.ReadMirror(defaultOutputDir(), req.Ceremony, time.Now())
	if err != nil {
		httpError(w, http.StatusNotFound, "this machine holds no readable copy of that ceremony: "+err.Error())
		return
	}
	if len(pdf) == 0 {
		httpError(w, http.StatusConflict, "this machine holds that ceremony's record but not its "+
			"document, so there is nothing to deliver")
		return
	}
	// **The roster is checked before a round starts, and the claim that used to stand here was
	// bigger than the code.** It said the document is verified against `checkDelivered`'s
	// completeness test before it is sent; no completeness test runs here, and since P08.S05e the
	// round may not send the document at all — a declined proceeding carries the convener's
	// attestation instead. What this genuinely refuses is a round with nobody to walk.
	if len(rec.Roster) == 0 {
		httpError(w, http.StatusConflict, "that ceremony's record carries no roster")
		return
	}
	addrs := make(map[string]string, len(req.Addresses))
	for k, val := range req.Addresses {
		addrs[strings.ToLower(k)] = val
	}
	outcomes, rerr := s.runDeliveryRound(r.Context(), v, rec, pdf, addrs)
	if rerr != nil {
		httpError(w, http.StatusConflict, rerr.Error())
		return
	}
	writeJSON(w, struct {
		Ceremony string            `json:"ceremony"`
		Parties  []deliveryOutcome `json:"parties"`
	}{Ceremony: req.Ceremony, Parties: outcomes})
}

// convenerInvitationFor re-mints one party's invitation from the record and the convener's own
// stored secret — the ONE door for that (ADR-009).
//
// **The convener holds no invitation for its own ceremony**, because it issues them; what it holds
// is a `CeremonySecret` per party, and each party's is different. That is what makes their
// rendezvous targets distinct, and it is why a round cannot be driven from one shared invitation.
// `/api/ceremony/invites` has always done this; P08.S05g's live run needed the same thing and the
// logic is extracted here rather than written twice, which is the duplicate derivation this repo
// keeps paying for.
//
// `Seeds` is absent and cannot be recovered — the record does not carry them — so a re-minted
// invitation has no DHT seed hints. Stated rather than papered over, exactly as the invites route
// states it; every other field a recipient CHECKS is present.
func convenerInvitationFor(v *vault.Vault, rec ceremony.Record, party ceremony.Party) (ceremony.Invitation, error) {
	conv := convenerFingerprintOf(rec)
	if conv == "" {
		return ceremony.Invitation{}, errors.New("that ceremony's record does not name a convener")
	}
	rh := rosterHashHex(rec)
	if rh == "" {
		return ceremony.Invitation{}, errors.New("that ceremony's record will not re-hash")
	}
	fp, err := hex.DecodeString(party.Fingerprint)
	if err != nil {
		return ceremony.Invitation{}, errors.New("that ceremony's record names a party Nib cannot read")
	}
	secret, ok := v.CeremonySecret(rec.ID, fp)
	if !ok {
		return ceremony.Invitation{}, errors.New("Nib no longer holds the invitation secret for " +
			party.Fingerprint[:12] + ", so it cannot reach them. A ceremony's secrets are removed when it ends.")
	}
	return ceremony.Invitation{
		Version:             ceremony.InvitationVersion,
		ID:                  rec.ID,
		Roster:              rec.Roster,
		Secret:              secret,
		ConvenerFingerprint: conv,
		Intent:              rec.Intent,
		RosterHash:          rh,
	}, nil
}

// armDeliveryAfterHop opens this party's delivery arm the moment its hop is mirrored.
//
// **The unlock hook is not enough, and the live run proved it rather than a review.** A party arms
// for delivery at unlock — which covers the machine that RESTARTS between signing and delivery, and
// covers nothing else. In an ordinary ceremony every instance unlocked long before the ceremony
// existed, so when the round ran nobody was listening: the convener raced candidates to
// `connectDeadline` for each party in turn and the harness timed out. Two triggers, one arm: this
// one for the party that just signed, `rearmDeliveries` for the one that came back.
//
// The convener is skipped — it delivers rather than waits — and so is a ceremony whose copy is
// already on disk, for `alreadyDelivered`'s reason.
func (s *Server) armDeliveryAfterHop(final []byte) {
	rec, rerr := ceremony.Extract(final)
	if rerr != nil {
		return // an ordinary two-party co-sign carries no record: nothing to be delivered
	}
	if !s.deliveryRearm.Load() {
		return // not a real Nib process: see EnableDeliveryRearm
	}
	v := s.unlockedVault()
	if v == nil {
		return
	}
	cert, key, err := identity(v)
	if err != nil {
		return
	}
	myFP, err := sign.Fingerprint(cert)
	if err != nil {
		return
	}
	me := hex.EncodeToString(myFP)
	if strings.EqualFold(me, convenerFingerprintOf(rec)) {
		return
	}
	if alreadyDelivered(rec) {
		return
	}
	text, ok := v.CeremonyInvitationFor(rec.ID)
	if !ok {
		return // no invitation, so no secret, so no rendezvous to arm at
	}
	inv, ierr := ceremony.ParseInvitation(text)
	if ierr != nil {
		return
	}
	if aerr := s.armForDelivery(context.Background(), inv, cert, key, me); aerr != nil {
		s.sess.noteFailure(armDelivery, "delivery-arm-failed",
			"Nib could not listen for your copy of the signed document.",
			"You have signed, and this machine could not open the connection that receives the "+
				"finished document. The convener can re-run delivery. Reason: "+aerr.Error())
	}
}

// endCeremony records that a proceeding is over, attested by the convener (P08.S05e, D28, C06).
//
// # Why the convener, and why that is a stated limit rather than a design goal
//
// Only the convener can mint one: `SignTermination` binds the roster hash with the convener's key,
// and under D22's hub only the convener learns that a hop declined — it holds the channel to
// everyone. **So the producer and the courier are the same machine, and P08.S04b's deepdive already
// recorded what that costs:** *"a convener-signed termination object cannot bind the convener — it
// is also the sole courier."* A convener that never mints one is indistinguishable from a ceremony
// still in progress, which is why `Stored.Ended` reads empty as UNKNOWN and never as live.
//
// That limit is inherited from S04b rather than introduced here, and it is restated at the site
// because a limitation recorded only in a deepdive file is one the next reader will not find.
//
// Best-effort, and quiet: this runs while an HTTP handler is on its way to telling the user their
// counterparty declined, and a failure to write the attestation must not replace that sentence
// with a storage error. The ceremony is over either way; what is lost is the ability to TELL the
// other parties, which the round reports on its own next run.
func (s *Server) endCeremony(cer *ceremonyID, state string) {
	if cer == nil {
		return
	}
	v := s.unlockedVault()
	if v == nil {
		return
	}
	rec, _, err := ceremony.ReadMirror(defaultOutputDir(), cer.inv.ID, time.Now())
	if err != nil {
		return // no verified record here: nothing to bind an attestation to
	}
	cert, key, err := identity(v)
	if err != nil {
		return
	}
	myFP, err := sign.Fingerprint(cert)
	if err != nil || !strings.EqualFold(hex.EncodeToString(myFP), convenerFingerprintOf(rec)) {
		return // only the convener attests an end state
	}
	t, terr := ceremony.SignTermination(rec, state, cert, key)
	if terr != nil {
		return
	}
	if werr := ceremony.WriteTermination(defaultOutputDir(), t); werr != nil {
		s.sess.noteFailure(armInteractive, "end-state-not-recorded",
			"This proceeding has ended, but Nib could not record that.",
			"The other parties cannot be told it is over until this machine can write the "+
				"attestation. Reason: "+werr.Error())
		return
	}
	// **And WHO ended it, so the round does not spend its connect deadline on them.**
	//
	// **After the attestation and only on its success**, because the marker is about a round that
	// carries a termination, and without one there is no such round: the skip would suppress a
	// leg for a delivery that has nothing to deliver.
	//
	// Reported through `noteFailure` rather than `log.Printf` for the reason `mirrorHop` gives at
	// its own best-effort write: a log line goes to a stderr that a double-clicked launch sends
	// nowhere, and this failure has a user-visible consequence — the round spends its connect
	// deadline on a party it cannot reach, once per re-run, and says nothing about why.
	if state == ceremony.StateDeclined && cer.peer != "" {
		if merr := markEndedBy(rec.ID, cer.peer); merr != nil {
			s.sess.noteFailure(armInteractive, "ender-not-recorded",
				"This proceeding has ended, and Nib could not record which party ended it.",
				"Delivering the end state will still reach everyone who signed, but it will also "+
					"spend several minutes trying to reach the party who refused, on this round "+
					"and on every re-run. Reason: "+merr.Error())
		}
	}
}

// asTermination reports whether these bytes are a termination object rather than a document.
//
// **Shape, not a flag.** A PDF starts `%PDF-`; a termination is JSON with a version and a state.
// Adding a discriminator byte to the wire would be a format change for a question the payload
// already answers, and the two cannot be confused: `json.Unmarshal` refuses a PDF outright.
func asTermination(b []byte) (ceremony.Termination, bool) {
	var t ceremony.Termination
	if err := json.Unmarshal(b, &t); err != nil {
		return ceremony.Termination{}, false
	}
	if t.Ceremony == "" || t.State == "" || t.Sig == "" {
		return ceremony.Termination{}, false
	}
	return t, true
}

// checkDeliveredPayload routes an arrival to the gate its SHAPE calls for.
//
// A termination is verified by `ReadTermination`'s own rules against this party's record — the
// convener's signature over the roster hash — not by `checkDelivered`, whose completeness and
// byte-prefix clauses are about a finished document and would refuse an attestation outright.
func (s *Server) checkDeliveredPayload(cer *ceremonyID, d []byte) error {
	t, ok := asTermination(d)
	if !ok {
		return s.checkDelivered(cer, d)
	}
	if cer == nil {
		return errors.New("an end-state attestation arrived for no ceremony")
	}
	rec, _, err := ceremony.ReadMirror(defaultOutputDir(), cer.inv.ID, time.Now())
	if err != nil {
		return fmt.Errorf("this machine cannot check that end state against its own record: %w", err)
	}
	// **The record is bound to the INVITATION before it anchors anything, and the first cut of
	// this gate skipped that.** `ReadTermination`'s own doc states the rule — *"`rec` must come
	// from the document or the invitation, never from the `record.json` beside it"* — and
	// `LoadState` restates it where it deliberately takes the weaker anchor: *"a gate that
	// REFUSES on a termination must anchor on the document or the invitation instead, because a
	// planted matching pair verifies against itself."* This is a refusing gate, and it was
	// verifying a termination against the `record.json` sitting in the same directory. A matching
	// record-and-termination pair dropped into `~/nib/ceremonies/<id>/` would have verified
	// perfectly against itself and written a durable false end state, which `rearmDeliveries`
	// then reads as `st.Ended != ""` and skips the ceremony forever — so the party never receives
	// its real copy.
	//
	// `cer.inv` is the anchor a planted file cannot control, and it is the same one the sibling
	// gate uses (`checkDelivered`, one function up). ADR-009: one rule, and both doors take it.
	if merr := cer.inv.MatchesRecord(rec); merr != nil {
		return merr // unwrapped: the sentence already names the axis
	}
	// **Verified against OUR record, never against the object's own claims.** The roster hash is
	// what refuses a substitution, and it commits to the ceremony id as well — so an attestation
	// minted for a different proceeding cannot be replayed into this one.
	if verr := t.Verify(rec); verr != nil {
		return fmt.Errorf("that end state does not verify against this ceremony: %w", verr)
	}
	// **Checked here, WRITTEN in `save` — the two halves of `autoAccepter`'s own contract.** Its
	// `verify` is *"the recipient's own check of what arrived"* and its `save` is where *"the
	// caller can persist it before the ack"*, and the first cut of this gate did both here. The
	// ordering property survived either way, since verify runs before save runs before the
	// acknowledgement; what did not survive was the split the type documents, and a check that
	// also mutates disk is one `TestTheDeliveryAcceptGateChecksBeforeItSaves` cannot see past.
	return nil
}

// tellEndState is C06's telling half: what a party who already signed is owed when the proceeding
// they signed into has ended.
//
// **Four things, and the criterion names all four** — so they are written as four sentences rather
// than one summary, because a party reading this has a signed document on their disk and needs to
// know what it is now worth:
//
//  1. it is over;
//  2. who ended it — the convener, who is the only party that can attest an end state;
//  3. their signature STANDS — nothing about a decline unmakes a signature already given;
//  4. a re-run starts from the original unsigned file, not from what they hold.
//
// It goes to the sticky notice because a delivery arm has no response to write into and no surface
// of its own (/pending 353) — `noticeView`'s own doc makes that argument, and this is the case it
// most obviously covers: the disarm IS the symptom, and a message that vanished with it would be
// one nobody reads.
func (s *Server) tellEndState(cer *ceremonyID, t ceremony.Termination) {
	// **Item 2 is per-state, and one shared sentence got it wrong.** The first cut said *"The
	// convener ended this proceeding"* for both states — but in the only state reachable today
	// the convener did NOT end it, a party refused, and telling a signer the wrong party ended
	// their proceeding is the item C06 asks for stated backwards. The convener ATTESTS the end
	// state in both; who reached it differs.
	//
	// Neither sentence names the party who refused, and that is S04b's decision showing through
	// rather than an omission: the termination binds the roster hash alone, deliberately, so the
	// convener cannot prove who declined and an unprovable accusation would name an innocent.
	what := "ceremony-declined"
	summary := "The proceeding you signed has been declined, so it is over."
	ended := "One of the parties refused, and the convener attested that the proceeding is over — " +
		"they are the only party who can attest an end state. "
	if t.State == ceremony.StateCompleted {
		what, summary = "ceremony-completed", "The proceeding you signed has completed."
		ended = "Every party has now signed, and the convener attested that the proceeding is " +
			"complete — they are the only party who can attest an end state. "
	}
	s.sess.noteFailure(armDelivery, what, summary,
		ended+
			"Your signature stands: nothing about this unmakes a signature you have already given, "+
			"and the copy on your disk is still a valid record of what you signed. If these parties "+
			"want to try again it starts from the ORIGINAL unsigned file, not from anything you "+
			"hold now — a new proceeding, with a new record and a new set of signatures.")
}

// deliveryLeg is the round's current leg, for a watcher (/pending 370).
//
// `Started` is what makes this useful rather than decorative. Within one stalled leg the INDEX does
// not move — that is the whole shape of the problem — so a surface reporting only "2 of 4" is as
// silent as no surface at all for the five minutes that matter. Elapsed time against a known
// ceiling is the thing that ticks, and it is what separates a round that is working from one that
// is hung.
type deliveryLeg struct {
	Label   string
	Index   int
	Of      int
	Started time.Time
}

// beginLeg publishes the leg about to be attempted, and returns the function that clears it.
//
// The clear is a RETURNED CLOSURE rather than a second exported call, so a caller cannot take the
// publish and forget the clear: a round that returned early — or panicked into `safe.Recover` —
// would otherwise leave a ceremony reporting a leg in flight forever, which is the stale-artifact
// failure this was put in memory to avoid.
func (s *Server) beginLeg(id, label string, index, of int) func() {
	s.legMu.Lock()
	if s.legs == nil {
		s.legs = map[string]deliveryLeg{}
	}
	s.legs[id] = deliveryLeg{Label: label, Index: index, Of: of, Started: time.Now()}
	s.legMu.Unlock()
	return func() {
		s.legMu.Lock()
		delete(s.legs, id)
		s.legMu.Unlock()
	}
}

// currentLeg reports the leg in flight for a ceremony, if any.
func (s *Server) currentLeg(id string) (deliveryLeg, bool) {
	s.legMu.Lock()
	defer s.legMu.Unlock()
	l, ok := s.legs[id]
	return l, ok
}

// deliveryProgressResponse is what a watcher polls while a round runs.
type deliveryProgressResponse struct {
	// Running is false between rounds. A watcher that polled a finished round and got a zero
	// struct could not tell it from one that had not started.
	Running bool `json:"running"`
	// Label is the party being reached, by the name the roster gives them — never a fingerprint,
	// which is the panel's standing rule.
	Label string `json:"label,omitempty"`
	// Index and Of place the leg in the round: "2 of 4".
	Index int `json:"index,omitempty"`
	Of    int `json:"of,omitempty"`
	// ElapsedMs is how long this leg has been running, and CeilingMs is how long it may run before
	// it gives up. The pair is the answer to "is this hung": one number rising against a stated
	// bound is progress even when the index is still.
	ElapsedMs int `json:"elapsedMs,omitempty"`
	CeilingMs int `json:"ceilingMs,omitempty"`
}

// handleCeremonyDeliveryProgress reports the round's current leg (/pending 370).
//
// **A second route rather than a field on the listing.** `/api/ceremonies` is read on every panel
// load and every lock-screen render; this is polled only while somebody is watching a round, so
// the cost is paid by the watcher rather than charged to every reader of the listing.
func (s *Server) handleCeremonyDeliveryProgress(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("ceremony")
	if id == "" {
		httpError(w, http.StatusBadRequest, "this request names no ceremony")
		return
	}
	l, ok := s.currentLeg(id)
	if !ok {
		writeJSON(w, deliveryProgressResponse{Running: false})
		return
	}
	writeJSON(w, deliveryProgressResponse{
		Running:   true,
		Label:     l.Label,
		Index:     l.Index,
		Of:        l.Of,
		ElapsedMs: int(time.Since(l.Started) / time.Millisecond),
		CeilingMs: int(connectDeadline / time.Millisecond),
	})
}
