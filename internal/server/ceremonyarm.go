package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"nib/internal/ceremony"
	"nib/internal/p2p"
	"nib/internal/safe"
	"nib/internal/sign"
	"nib/internal/vault"
)

// The ceremony HOP arm — one door, and the sweep that re-establishes it (P02.S02, D14).
//
// # Why this file exists at all
//
// A signer who accepts an invitation and never arms is, from the convener's side, exactly a signer
// who ignored it: the convener dials, nothing answers, and the ceremony stalls on a party who
// believes they have done their part. Measured before this slice, against a successful accept:
// `/api/session/status` answered `{armed: false}`. Accepting now arms.
//
// # It is a SIBLING of the delivery sweep, deliberately, down to its gate
//
// `rearmDeliveries` already does this shape one slot over — re-established at every unlock,
// best-effort per ceremony, anchored on the invitation rather than on `Stored.Ended`, failing OPEN
// toward arming. Everything that argument establishes holds here unchanged, so it is not restated;
// what differs is written at each point below.
//
// **The process gate is `deliveryRearm` and it is shared rather than duplicated.** Its whole
// content is *"this Server backs a real Nib process, so opening a socket is something it should
// do"* — which is one fact about the process, not two facts about two sweeps. A second flag would
// be a second answer to one question, and the first test that set only one of them would arm half
// of a machine. `EnableDeliveryRearm`'s doc records what an ungated sweep cost: `TempDir RemoveAll
// cleanup: directory not empty` across five unrelated tests, because a `Server` built in a test
// isolates `configDir` and not `$HOME`.

var (
	// errCeremonyEndpoint, errCeremonyAccept and errSessionArmed carry the three outcomes the
	// arm route answered with three different status codes before this door existed. They are
	// sentinels rather than a status field because the door has a non-HTTP caller now, and a
	// sweep has no status code to be handed.
	errCeremonyEndpoint = errors.New("could not open the ceremony endpoint")
	errCeremonyAccept   = errors.New("could not arm the racing accept")
	errSessionArmed     = errors.New("a session is already armed")
)

// armCeremonyHop opens this machine's arm for one ceremony hop: a shared endpoint, a handshaked
// racing accept on it, the interactive slot, and the receive goroutine.
//
// **Extracted from `handleSessionArm` rather than written beside it, and that is ADR-009 rather
// than tidiness.** Unlike the delivery arm — which is a standalone function because it arms a
// DIFFERENT hop index into a different slot — this is the same arm the route has always opened.
// A sweep that built its own would be two implementations of one rule, which is the shape this
// repo keeps finding; so the route became this door's first caller and its behaviour is unchanged.
//
// `cands` is the optional typed peer address that makes this arm DIAL as well as accept. It stays
// a parameter because resolving it needs the response writer (`peerAddresses` writes its own
// error), so it is the one thing the route must still do for itself.
func (s *Server) armCeremonyHop(ctx context.Context, cer *ceremonyID, cert, key, peerFP []byte,
	cands []candidate, bind, label, mode string, byPolicy bool) error {
	if serr := cer.setupSharedEndpoint(bind, s.configDir); serr != nil {
		cer.close()
		return fmt.Errorf("%w: %v", errCeremonyEndpoint, serr)
	}
	hl, herr := p2p.QUICListenHandshakeOn(cer.end, cert, key, peerFP)
	if herr != nil {
		cer.close()
		return fmt.Errorf("%w: %v", errCeremonyAccept, herr)
	}
	armCtx, cancel := context.WithCancel(ctx)
	// **The door serves both callers and the flag is which one.** The route's arm is the user's and
	// is never displaced; the sweep's is this machine's own policy and yields to an explicit
	// request — see `arm.byPolicy`.
	armed := s.sess.armCeremony
	if byPolicy {
		armed = s.sess.armCeremonyByPolicy
	}
	if !armed(cer, cer.end.LocalAddr().String(), cancel) {
		cancel()
		hl.Close()
		cer.close()
		return errSessionArmed
	}
	go s.runCeremonyReceive(armCtx, cer, hl, cands, cert, key, label, mode, peerFP)
	return nil
}

// rearmCeremonies arms the hop this machine is still waiting to be dialled for, if there is one.
//
// # The two triggers, and why they are the same function
//
// Accepting an invitation must arm *now* — that is D14 — and an arm must survive quitting Nib,
// because D24 makes a ceremony span a restart and a listener does not. Those are one rule reached
// at two moments, so they are one function called twice: from `adoptVault`, which is the one door
// for "the vault just opened", and from the tail of `handleCeremonyAccept`. A second copy for the
// second trigger is exactly what `rearmDeliveries`' own comment warns about one queue over.
//
// # "Which ceremonies are waiting" is answered by the ABSENCE of a record, and that is exact
//
// On a machine that did not convene, `WriteMirror` has three production callers and two of them
// are this machine's own completed hop (`persistContribution`, `mirrorHop`); the third is
// `convene`. So a record on disk means *this party's hop has already happened*, and no record
// means it has not. That makes the two sweeps disjoint by construction rather than by agreement:
// this one takes the ceremonies with no record, `rearmDeliveries` takes the ones with a record and
// no delivered copy, and neither has to know what the other is doing.
//
// It is also why the `LoadOK` filter `rearmDeliveries` opens with is INVERTED here. Copying that
// line would have skipped every ceremony this sweep exists for — the pre-record state is not a
// damaged one, it is the ordinary state of a party who has accepted and is waiting.
//
// # What it will not arm for, and the one thing it CANNOT see
//
// The convener, because the convener dials and does not wait to be dialled. And a ceremony whose
// invitation this machine no longer holds, which is what the D29 prune leaves behind.
//
// **There is no ended-check here, and its absence is a limitation rather than an omission**
// (`/pending 378`). `rearmDeliveries` has one, anchored on the invitation because a planted
// (record, termination) pair verifies against itself (`/pending 354`) — and every anchor it uses
// needs a record. This sweep's ceremonies are exactly the ones with no record: `ReadStored` returns
// at `LoadAbsent` before it ever sets `Ended`, `ReadTermination` takes a `Record`, and the
// invitation carries no deadline (`/pending 247`). `closeOutReason` cannot help either — it opens
// with `st.State != ceremony.LoadOK { return "", false }`, so a record-less directory is never
// closed out.
//
// So a party whose proceeding was declined or abandoned *before the baton reached them* holds this
// arm for the life of the Nib process, and nothing local can tell them otherwise. That is the same
// gap as D14's bound, seen from the other side: everything that would say the proceeding ended
// lives in the record this party does not have. Writing a check against `Stored.Ended` anyway
// would be worse than the gap — it is the exact anchor `/pending 354` proved forgeable, and the
// decision NOT to arm is an authorisation.
//
// Best-effort per ceremony, and it never fails the accept that triggered it: the interactive slot
// is shared with the user's own manual receive arm, so "a session is already armed" is an ordinary
// outcome here rather than a fault, and telling a user their invitation was not accepted because
// of it would be false.
// prefer is the ceremony the accept trigger just took on, tried before any other.
//
// **It exists because tier 4d caught the sweep arming for the wrong proceeding.** `ListStored`
// sorts by id, so on a machine holding more than one accepted-and-unsigned ceremony the sweep armed
// for whichever id sorted first — which is not the one the user just accepted, and is arbitrary
// from their point of view. Measured: `pairrepro.sh -n 3` failed at *"instance 3 could not arm
// before hop 1 (HTTP 409)"*, because accepting the relay's invitation had armed for an earlier
// ceremony still on that machine's disk.
//
// Empty on the unlock trigger, which has no ceremony in mind and correctly takes them in order.
func (s *Server) rearmCeremonies(v *vault.Vault) { s.rearmCeremoniesPreferring(v, "") }

func (s *Server) rearmCeremoniesPreferring(v *vault.Vault, prefer string) {
	// Nil-guarded for `rearmDeliveries`' stated reason: this runs detached, so a panic reaches
	// nobody and the failure mode is a party who silently never arms.
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
	// **The preference decides only which ceremony gets a FREE slot, and never takes an occupied
	// one — a displacement here was tried and backed out.**
	//
	// Making the accept trigger displace looked right (accepting names a proceeding, so it should
	// outrank an earlier guess) and it is wrong for a reason the direct test showed immediately:
	// two accepts in quick succession each start a sweep, so both displace and **whichever runs
	// last wins**. The arm a user ends up with then depends on goroutine scheduling rather than on
	// anything they did, which is a worse answer than the one it replaced.
	//
	// The single interactive slot means something has to lose when a machine holds two live
	// ceremonies; that is `/pending 378`'s recorded residual doubt and not a thing this sweep can
	// decide. What tier 4d actually needed is that an EXPLICIT arm is never refused because of a
	// guess, and that is `handleSessionArm`'s displacement, where there is exactly one caller and
	// no race.
	//
	// The preferred ceremony first, then the rest in listing order. Reordering rather than
	// filtering: if the preferred one cannot be armed for — no invitation, already signed, this
	// machine convened it — the sweep must still do its ordinary job rather than give up.
	if prefer != "" {
		for i, st := range stored {
			if st.ID == prefer {
				stored = append([]ceremony.Stored{st}, append(stored[:i:i], stored[i+1:]...)...)
				break
			}
		}
	}
	for _, st := range stored {
		if st.State == ceremony.LoadOK {
			continue // this party's hop has already happened — see the header
		}
		text, ok := v.CeremonyInvitationFor(st.ID)
		if !ok {
			continue // nothing to derive a rendezvous from
		}
		inv, ierr := ceremony.ParseInvitation(text)
		if ierr != nil {
			continue
		}
		// **The convener dials; it does not wait to be dialled.** Kept as DEFENCE IN DEPTH and
		// labelled as such, because a red proof showed it is never the refusal that fires: with
		// this branch disabled the sweep still does not arm, because `ceremonyFor` reaches
		// `hopBetween`, which refuses `a == b` with *"was given as both ends"* — the convener's
		// only possible counterparty under D22's hub is itself. It is here anyway because it
		// refuses on a fact this loop already holds, before a hex decode, a vault read and a
		// ceremony construction, and because the *reason* the convener is skipped is a statement
		// about the topology rather than a side effect of one. What it is not is a rule with a
		// behavioural test: nothing can make it the deciding branch, and `TestTheHopSweepLeaves-
		// TheConvenerAlone` says which half it proves.
		if strings.EqualFold(me, inv.ConvenerFingerprint) {
			continue
		}
		peerFP, derr := hex.DecodeString(inv.ConvenerFingerprint)
		if derr != nil || len(peerFP) != sha256.Size {
			continue
		}
		label, pinned := pinnedLabel(v, peerFP)
		if !pinned {
			// The accept pins the convener, so this cannot happen on a ceremony this machine
			// accepted — but a user may have pruned the pin, and arming for a peer this machine
			// has un-trusted would put the pin back by the side door.
			continue
		}
		cer, cerr := ceremonyFor(text, cert, key, peerFP)
		if cerr != nil || cer == nil {
			continue
		}
		// **QUIC, as `armForDelivery` is, and the reason is a defect in the manual path.** A
		// ceremony armed over TCP *"can be REACHED but never FOUND"* — `handleSessionArm` says so
		// at the branch — because it has no rendezvous to publish through. An arm nobody off-link
		// can locate is not the arm D14 asks for.
		if aerr := s.armCeremonyHop(context.Background(), cer, cert, key, peerFP, nil,
			"0.0.0.0:0", label, sessionModeCoSign, true); aerr != nil {
			if errors.Is(aerr, errSessionArmed) {
				return // the user has the slot; this is not a fault and needs no notice
			}
			s.sess.noteFailure(armInteractive, "ceremony-arm-failed",
				"Nib could not listen for the ceremony you joined.",
				"You accepted an invitation, and this machine could not open the connection "+
					"that receives the document when it is your turn. Reason: "+aerr.Error())
			return // one slot: a second ceremony cannot arm behind a failure either
		}
		return // the slot holds one; the next unlock takes the next ceremony
	}
}

// rearmCeremoniesAsync is the detached form the two triggers use.
//
// **Detached, and that is what keeps the accept honest.** `handleCeremonyAccept` answers with a
// roster the user is about to read; opening a socket, publishing a rendezvous and reading
// `~/nib/ceremonies` on that path would put the network between the user and their answer, and a
// failure there is not a failure to accept.
func (s *Server) rearmCeremoniesAsync(v *vault.Vault, prefer string) {
	if !s.deliveryRearm.Load() {
		return // not a real Nib process: see EnableDeliveryRearm
	}
	go func() {
		defer safe.Recover("ceremony re-arm")
		s.rearmCeremoniesPreferring(v, prefer)
	}()
}
