package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
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
		verify: func(d []byte) error { return s.checkDelivered(cer, d) },
		save: func(d []byte) error {
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
	path := filepath.Join(defaultOutputDir(), "signed", deliveredName(rec))
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
		// The publish is what makes the arm findable off-LAN, and it holds the link's window
		// first exactly as every other publish does (ADR-011) — a delivery round on one office
		// network must emit nothing either.
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
		if st.State != ceremony.LoadOK || st.Ended != "" {
			continue // unreadable, or a proceeding that has already ended
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

// deliveryOutcome is one party's result in a round, reported to the caller so a re-run is a
// decision the user makes rather than a retry loop nobody can see.
type deliveryOutcome struct {
	Fingerprint string `json:"fingerprint"`
	Label       string `json:"label,omitempty"`
	Delivered   bool   `json:"delivered"`
	Skipped     bool   `json:"skipped,omitempty"` // already acknowledged by an earlier run
	Reason      string `json:"reason,omitempty"`
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
	out := make([]deliveryOutcome, 0, len(rec.Roster))
	for _, party := range rec.Roster {
		if strings.EqualFold(party.Fingerprint, me) {
			continue // the convener already holds it
		}
		res := deliveryOutcome{Fingerprint: party.Fingerprint, Label: party.Label}
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
		inv, ierr := convenerInvitationFor(v, rec, party)
		if ierr != nil {
			res.Reason = ierr.Error()
			out = append(out, res)
			continue
		}
		if err := s.deliverToParty(ctx, inv, party.Fingerprint, addrs[strings.ToLower(party.Fingerprint)], cert, key, myFP, pdf); err != nil {
			res.Reason = err.Error()
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
func (s *Server) deliverToParty(ctx context.Context, inv ceremony.Invitation, partyFP, addr string, cert, key, myFP, pdf []byte) error {
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
	if addr != "" {
		// `Source` is NOT optional: the per-source cap accounts an unset one to the zero value,
		// so one tier spends another's share and the drop report names the wrong tier as the
		// flooder. `TestEveryCandidateProducerNamesItsSource` caught this literal, which is what
		// that guard is for — a producer added later, not the ones anybody remembered to list.
		cands = []candidate{{Fingerprint: peerFP, Addr: addr, Transport: transportQUIC,
			Label: partyLabel(inv, partyFP), Source: sourceTyped}}
	}
	conn, err := s.raceWithRendezvous(cer, cands, cert, key, peerFP, partyLabel(inv, partyFP), partyLabel(inv, partyFP))
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
	// **The document is verified before it is sent, not only when it arrives.** `checkDelivered` is
	// the recipient's gate; running the same completeness test here means a convener does not spend
	// a round handing out a document that every recipient will refuse.
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
