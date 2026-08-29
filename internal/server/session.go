package server

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"nib/internal/atomicfile"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"nib/internal/ceremony"
	"nib/internal/p2p"
	"nib/internal/pdfops"
	"nib/internal/safe"
	"nib/internal/sign"
	"nib/internal/vault"
)

// TRIPWIRE: the armed session listener is Nib's only network-reachable surface that
// anything can be SENT TO and acted on — every other listener binds loopback
// (cmd/nib/main.go, the loopbackOnly guard in server.go). It is opened only by an
// explicit, vault-unlocked /api/session/arm, binds the address the caller chooses,
// accepts only the one pinned peer (pinned-peer mTLS, internal/p2p/transport.go),
// signs only with explicit per-document user consent, and is torn down after one
// session or on timeout / disarm / shutdown.
// Keep that containment intact: do not widen what arms it, how long it stays open,
// or which peers it accepts without a fresh security review (P2P 12).
//
// **Amended by P03.S02: it is no longer the ONLY socket that receives.**
// internal/discovery opens a second one — link-local multicast, and armed-only, both
// authorised in advance by PLAN-signing-ceremony.md's egress enumeration, which is
// the "fresh security review" the paragraph above demands rather than an exception to
// it. The distinction that keeps this tripwire meaningful is what the two sockets can
// be made to DO:
//
//   - this listener is where a document arrives and a signature is produced;
//   - the discovery socket cannot influence which peer is accepted at all. L1, and it
//     is structural rather than a rule: an announcement carries a NAME, the only way
//     back to a fingerprint is pairing.Matches against a pin the receiver already
//     holds, and internal/discovery is guarded against importing the vault, sign or
//     p2p (TestNothingHereCanReachAnIdentity). The worst a hostile announcer achieves
//     is wasting a two-second browse.
//
// So a discovery datagram is untrusted input to a parser and nothing more — and it is
// treated as internet-facing, because Go binds a multicast listener to the WILDCARD
// and it therefore accepts ordinary unicast from anywhere (ADR-007).

const (
	sessionAcceptTimeout = 5 * time.Minute // auto-disarm if no peer connects
	// (The handshake bound moved to internal/p2p's handshakeTimeout in P02.S05, value
	// unchanged: what must be bounded differs per transport, and the server no longer
	// sees a handshake at all.)
	sessionConsentTimeout = 5 * time.Minute // decline if the user never responds
)

// session is the receive side of a live session: an armed, routable, pinned-peer-only
// listener opened on explicit request and torn down after one use. It serves two
// modes — co-signing (the peer's signed doc comes back doubly-signed) and a plain
// one-way document transfer (the peer's doc is consented and saved to ~/nib). All
// state is guarded by mu; it is independent of the Server's document lock.
type session struct {
	mu       sync.Mutex
	ln       p2p.Listener   // non-nil while armed
	addr     string         // bound address, reported in status
	pending  *pendingReq    // set while a received request awaits the user's consent
	verify   *pendingVerify // set while the spoken check awaits the user's confirmation
	received *receivedInfo  // last accepted transfer, read by the poller after disarm
	// cer is the ceremony identity this arm carries, or nil for the manual/LAN path.
	// Set by arm and cleared by disarmIf with everything else: it holds the invitation
	// secret, and a secret that outlives the session it belongs to is residue.
	cer *ceremonyID
	// cerCancel cancels a connect-based ceremony arm's background goroutine (P05.S09), the
	// analogue of closing the accept listener for a runSession arm. nil for accept arms.
	cerCancel context.CancelFunc
	// notice is the last thing that went wrong in the background, and it OUTLIVES the session
	// (P08.S08). Cleared only by the next arm.
	//
	// # Why it has to be sticky, and why there was nothing here before
	//
	// Every failure on the receiving side happens on a goroutine with no HTTP response to write
	// into. `runSession` discards `serveOneSession`'s error into `_`; `runCeremonyReceive` uses it
	// only for loop control; `mirrorHop` and `saveReceived` report into `log.Printf` and a bare
	// `return` respectively. Nib ships no log file and no log viewer, and `cmd/nib/main.go` already
	// makes the argument about its own hand-off notice: *"a double-clicked launch has no terminal:
	// its stderr goes nowhere a user will look, so a refusal logged here alone is a refusal nobody
	// receives."* That reasoning was applied there and to nothing else.
	//
	// So the arm simply went quiet, and the user was shown an unarmed session and no reason. A
	// field cleared on disarm would be no better: the disarm IS the symptom, and a message that
	// vanishes with it is a message nobody reads.
	notice *noticeView
	// until is when this arm gives up, as an absolute time; zero when nothing is armed.
	//
	// **It exists to be ASSERTED, which is C05's whole difficulty (P08.S04).** The criterion is
	// that a party who arms and waits through three earlier hops is still armed when the baton
	// arrives — and nothing exposed a window, so the only available check was "the ceremony
	// completed", which is true whatever the bound is because a loopback relay finishes in
	// seconds. A five-minute manual bound and a thirty-day ceremony bound are indistinguishable
	// from the outcome; they are trivially distinguishable from the figure.
	until time.Time
}

// pendingVerify is the spoken verification string waiting on the user (D4, L2).
//
// A SEPARATE field from `pending`, not a variant of it, because the two gates answer
// different questions at different points: this one asks "are you talking to who you think
// you are" before a single document byte moves, and `pending` asks "will you sign this"
// after the document has arrived and been reviewed. Folding them into one field would let
// a consent decision satisfy a verification, which is the whole of what L2 forbids.
type pendingVerify struct {
	words string
	resp  chan bool
}

// pendingReq is a received request (a co-sign or a plain transfer) blocked on the
// user's accept/decline. view is what the consent UI shows about the sender.
type pendingReq struct {
	view pendingView
	doc  []byte // the received document, served for review via /api/session/pending-pdf
	resp chan sessionDecision
}

// receivedInfo reports where an accepted one-way transfer was saved, so the poller
// can tell the user once the session disarms.
type receivedInfo struct {
	Path string `json:"path"`
	Peer string `json:"peer"`
}

type sessionDecision struct {
	accept     bool
	intent     string
	appearance []byte
}

func (se *session) arm(ln p2p.Listener, cer *ceremonyID) bool {
	se.mu.Lock()
	defer se.mu.Unlock()
	if se.ln != nil {
		return false
	}
	se.ln = ln
	se.cer = cer
	se.addr = ln.Addr().String()
	se.received = nil // a fresh session clears any prior transfer result
	se.notice = nil   // and any prior failure: the user is trying again, so the old reason is spent
	// **Stamped HERE and not by the arm goroutine.** The first draft set it from `runSession`,
	// which starts after this handler has already answered — so a status poll landing in that
	// window saw an armed session with no window, and the race detector found it by being slow
	// enough to lose that race every time. The door knows whether there is a ceremony, so the
	// door is where the figure belongs.
	se.until = time.Now().Add(armWindowFor(cer))
	return true
}

// armWindowFor is how long an arm waits, and it is ONE door (ADR-009).
//
// A ceremony arm waits for the ceremony; a manual or LAN arm waits `sessionAcceptTimeout`. Both
// `runSession` and `runCeremonyReceive` compute their own timer from this, and both arm doors stamp
// `until` from it, so the figure the status reports and the figure the timer fires on cannot drift.
//
// **The bound is a constant and D16's amendment asks for the record's `Expires`.** Not available
// here: an arm holds an invitation and the invitation carries no deadline (`/pending 247`). This is
// the ceiling until it does.
func armWindowFor(cer *ceremonyID) time.Duration {
	if cer != nil {
		return ceremony.MaxCeremonyLife
	}
	return sessionAcceptTimeout
}

// armCeremony arms a connect-based ceremony session (P05.S09): it holds the ceremony and a cancel
// for its background goroutine rather than an accept listener, because the symmetric-racing
// coordinator owns the single (handshaked) listener and a transport permits only one. addr is the
// shared endpoint's address, reported in status; cancel stops the connect goroutine at disarm.
func (se *session) armCeremony(cer *ceremonyID, addr string, cancel context.CancelFunc) bool {
	se.mu.Lock()
	defer se.mu.Unlock()
	if se.ln != nil || se.cer != nil {
		return false
	}
	se.cer = cer
	se.addr = addr
	se.cerCancel = cancel
	se.received = nil
	se.notice = nil
	se.until = time.Now().Add(armWindowFor(cer))
	return true
}

// noteFailure records something that went wrong where no response could carry it (P08.S08).
//
// Last-write-wins rather than a queue: the useful thing is what most recently stopped working, and
// a list nobody prunes becomes its own problem. `what` is the stable key a surface can branch on.
func (se *session) noteFailure(what, summary, detail string) {
	se.mu.Lock()
	defer se.mu.Unlock()
	se.notice = &noticeView{What: what, Summary: summary, Detail: detail, At: time.Now()}
}

func (se *session) setReceived(r *receivedInfo) {
	se.mu.Lock()
	se.received = r
	se.mu.Unlock()
}

// disarm closes whatever is armed NOW and declines any in-flight consent. Idempotent.
//
// This is the unconditional form, and it is right for the two callers who mean exactly
// that: the user pressing Cancel, and shutdown. The accept goroutine must NOT use it — see
// disarmIf.
func (se *session) disarm() { se.disarmIf(nil) }

// disarmIf closes the session only if `ln` is still the armed listener; nil means
// unconditional.
//
// **The goroutine's defer used to act on whatever was armed when it fired, not on the
// session it belonged to.** A session's accept goroutine can live for minutes — p2p.Receive
// spans the user's consent, the signing, and a 128 MiB write — and during that time the
// user can Cancel and arm a NEW session. arm() only refuses while a listener is present, so
// the re-arm succeeds. Then the old goroutine finishes and its `defer disarm()` closed the
// NEW session's listener: the user sits waiting on a receive that was torn down by a
// predecessor, and sees "no peer connected" for a session that never got its chance.
//
// Identity rather than a generation counter, deliberately. The listener is the thing being
// disarmed and the goroutine already holds it, so there is nothing to keep in sync; a
// counter is a second truth that has to be incremented in exactly the right place to stay
// equal to the first.
func (se *session) disarmIf(ln p2p.Listener) {
	se.disarmWhen(func() bool { return ln == nil || se.ln == ln })
}

// disarmCeremony tears a connect-based arm down only if `cer` is still the armed ceremony — the
// cer analogue of disarmIf(ln), for an arm that has no listener to key on (P05.S09). It is what
// stops a stale connect goroutine, finishing after a cancel-and-rearm, from disarming the session
// that replaced it.
func (se *session) disarmCeremony(cer *ceremonyID) {
	se.disarmWhen(func() bool { return cer != nil && se.cer == cer })
}

// disarmWhen is the shared teardown: it captures and clears the armed state under the lock only if
// the guard holds, then closes the listener and ceremony and releases any parked gate outside it.
func (se *session) disarmWhen(ok func() bool) {
	se.mu.Lock()
	if !ok() {
		se.mu.Unlock()
		return // a later session is armed; this one is already over
	}
	cur, p, pv := se.ln, se.pending, se.verify
	cer := se.cer
	cancel := se.cerCancel
	se.ln, se.addr, se.pending, se.verify, se.cer, se.cerCancel = nil, "", nil, nil, nil, nil
	se.until = time.Time{}
	se.mu.Unlock()
	if cancel != nil {
		cancel() // stop the connect goroutine (S09), the analogue of ln.Close below
	}
	if cur != nil {
		cur.Close()
	}
	// **The ceremony's network comes down AFTER the listener and in its own order**, and
	// the ordering is not a style preference. `cer.close()` shuts the rendezvous server
	// before the socket; the reverse makes the DHT's read return net.ErrClosed, which
	// anacrolix/dht turns into `panic(err)` on a goroutine nothing of ours is on — process
	// death, at shutdown, on the path a user reaches by pressing Cancel or quitting.
	//
	// It also releases the invitation secret with the session it belonged to. A secret that
	// outlives its ceremony is residue.
	cer.close()
	if p != nil {
		select {
		case p.resp <- sessionDecision{accept: false}:
		default:
		}
	}
	// A disarm during the spoken check releases it too, refusing. Without this the peer's
	// goroutine sits on a channel nobody will ever write to until the session deadline —
	// the same hazard the consent release above exists for, one gate earlier.
	if pv != nil {
		select {
		case pv.resp <- false:
		default:
		}
	}
}

// A consentAnchor names the armed operation a consent request belongs to, so setPending can
// refuse a request from a goroutine whose session was cancelled and re-armed. It is the armed
// LISTENER for the manual/LAN path — where the receiving side always has one — and the CEREMONY
// for a symmetric-racing hop, whose receive role may have WON BY DIALING and so holds no listener
// of its own (P05.S09 C4). Exactly one field is set; the ceremony wins if both somehow are.
type consentAnchor struct {
	ln  p2p.Listener
	cer *ceremonyID
}

// current reports whether this anchor still names the armed session. Called under se.mu. It is the
// same stale-goroutine guard `se.ln == ln` gave, generalised: after a cancel-and-rearm se.cer (or
// se.ln) is the NEW operation, so an anchor naming the old one is refused — which is the whole
// point of the check (see setPending).
func (a consentAnchor) current(se *session) bool {
	if a.cer != nil {
		return se.cer != nil && se.cer == a.cer
	}
	return se.ln != nil && se.ln == a.ln
}

// setPending parks a consent request, and refuses if its anchor no longer names the armed session.
//
// **The identity check, which this was the one mutator without.** `disarmIf`, `clearPendingIf`,
// `clearVerifyIf` and `setVerify` all carry one, and their comments give the reason: a session
// goroutine can live for minutes — `p2p.Receive` spans the user's consent, the signing and a
// 128 MiB write — so it can still be running when the user cancels and re-arms. Checking only
// `se.ln == nil` passes in exactly that window, and the stale goroutine parks ITS consent
// request as the NEW session's pending: the user is shown a document from the connection they
// cancelled, attributed to the peer they have just armed for.
//
// **The anchor, not the listener directly (P05.S09).** The guard used to be `se.ln == ln`, but a
// symmetric-racing hop's receive role can win by DIALING and then has no listener to name — the
// gate would be unreachable and consent would hang, the mirror of the bug setVerify's doc records
// for the dialing side. The anchor keys on the CEREMONY there, which survives the same
// cancel-and-rearm test: after a rearm se.cer is the new ceremony, so a stale hop's anchor fails.
//
// Identity rather than a generation counter, for `disarmIf`'s reason: the listener (or ceremony)
// IS the thing being armed, and the goroutine already holds it.
func (se *session) setPending(a consentAnchor, p *pendingReq) bool {
	se.mu.Lock()
	defer se.mu.Unlock()
	if !a.current(se) {
		return false
	}
	se.pending = p
	return true
}

// clearPendingIf drops the pending consent only if `p` is still the pending one.
//
// Same hazard as disarmIf, and a worse consequence: an unconditional clear fired by a
// finished confirmer discards a consent request belonging to a LATER session — a peer's
// document sitting in front of the user, silently abandoned while they were reading it.
func (se *session) clearPendingIf(p *pendingReq) {
	se.mu.Lock()
	if se.pending == p {
		se.pending = nil
	}
	se.mu.Unlock()
}

// pendingPDF returns the received document awaiting consent, or nil if none is
// pending. The bytes are exactly what coSignExchange will sign on accept, so the
// review pane shows precisely what the user co-signs.
func (se *session) pendingPDF() []byte {
	se.mu.Lock()
	defer se.mu.Unlock()
	if se.pending == nil {
		return nil
	}
	return se.pending.doc
}

// pendingFingerprint returns the hex SPKI of the peer whose request is awaiting
// consent, or "" if none. The responder's attestation quote names this peer as the
// accepted counterparty.
func (se *session) pendingFingerprint() string {
	se.mu.Lock()
	defer se.mu.Unlock()
	if se.pending == nil {
		return ""
	}
	return se.pending.view.Fingerprint
}

// respondVerify resolves the spoken check. Returns false when nothing is waiting, so a
// stray confirmation cannot be recorded against a session that has moved on.
func (se *session) respondVerify(ok bool) bool {
	se.mu.Lock()
	pv := se.verify
	se.mu.Unlock()
	if pv == nil {
		return false
	}
	select {
	case pv.resp <- ok:
		return true
	default:
		return false
	}
}

func (se *session) respond(d sessionDecision) bool {
	se.mu.Lock()
	p := se.pending
	se.mu.Unlock()
	if p == nil {
		return false
	}
	select {
	case p.resp <- d:
		return true
	default:
		return false
	}
}

func (se *session) status() sessionStatus {
	se.mu.Lock()
	st := sessionStatus{Armed: se.ln != nil || se.cer != nil, Address: se.addr, Received: se.received}
	if st.Armed && !se.until.IsZero() {
		u := se.until
		st.Until = &u
	}
	// Carried whether or not anything is armed — that is the point of it. The disarm is usually
	// the symptom, so a notice that went away with the session would be one nobody reads.
	st.Notice = se.notice
	if se.pending != nil {
		pv := se.pending.view
		st.Pending = &pv
	}
	if se.verify != nil {
		st.Verify = &verifyView{Words: se.verify.words}
	}
	cer := se.cer
	inSession := se.pending != nil || se.verify != nil
	se.mu.Unlock()

	// A waiting ceremony arm (not yet in a session) shows why nothing has connected — computed from
	// signals safe to read while the feed runs (rz.Stats mutex-guarded, peerSeen atomic). Outside the
	// lock: diagnose() takes rz.mu/cer.mu, and holding se.mu across them would nest three locks.
	if cer != nil && !inSession && cer.bootstrapDone.Load() {
		if d := cer.diagnose(); d.cause != causeUndiagnosed {
			st.Diagnosis = &diagnosisView{Cause: causeName(d.cause), Summary: d.summary, Detail: d.detail}
		}
	}
	return st
}

// sessionConfirmer is the consent bridge: p2p.Receive calls it after a peer sends a
// signed document; it surfaces the document for review, parks the request for the
// UI to accept/decline, and blocks until the user responds (or the timeout declines).
// reached records that a connection put something in front of the local user. It is what
// decides whether that connection SPENT the arm — see serveOneSession.
type reached struct{ v atomic.Bool }

// mark is nil-safe on purpose: the DIALING side (`/api/session/initiate`,
// `/api/session/send`) passes nil because it holds no arm to spend — it is the party that
// went looking, and nothing on this machine is listening for it.
func (r *reached) mark() {
	if r != nil {
		r.v.Store(true)
	}
}

type sessionConfirmer struct {
	s   *Server
	saw *reached
	// anchor names the operation this consent belongs to, so setPending can refuse a request from a
	// goroutine whose session has already been replaced — a listener (manual) or a ceremony (S09).
	anchor consentAnchor
	// cer is the ceremony this session belongs to, or nil outside one. **Separate from `anchor`
	// on purpose** — the anchor carries a ceremony only on the QUIC coordinator path, so a gate
	// reading it would be blind on every TCP ceremony hop. See serveOneSession.
	cer *ceremonyID
}

func (sc sessionConfirmer) Confirm(peer p2p.SignerAttestation, doc []byte) (bool, string, []byte, error) {
	// **C17, and it runs BEFORE anything is put in front of the user (P07.S02b).**
	//
	// The order is the clause: a party reconciles the document against the invitation its arm
	// was built from, and only then is asked to consent. Reversed, the user reads and accepts a
	// document that the ceremony they were invited to does not describe — and by then they have
	// signed it.
	//
	// It also sits before `sc.saw.mark()`, and that is defence rather than a fix — **stated
	// precisely, because the first draft of this comment claimed it protects the arm and that is
	// false on this path.** `p2p.Receive` runs the spoken check before it reads a single document
	// byte, and `sessionVerifier.ConfirmVerification` marks there — so on every co-sign the arm is
	// already spent by the time this function is entered, and moving this check after the mark
	// would change nothing today. It is kept in this order because the ordering is the one that
	// stays correct if a path ever reaches `Confirm` without a spoken check first: a document the
	// gate refused was shown to nobody, and nothing that shows nobody anything should spend an
	// arm. The load-bearing half is the ordering against `setPending` above.
	if sc.cer != nil {
		if err := sc.cer.checkArrival(doc, time.Now()); err != nil {
			return false, "", nil, err
		}
	}
	sc.saw.mark() // the consent request is about to go on screen
	// Park the received document for review (served via /api/session/pending-pdf)
	// rather than replacing the open document — that only changes on accept, in
	// runSession. A declined or timed-out request leaves the open doc untouched.
	ch := make(chan sessionDecision, 1)
	view := pendingView{
		Signer: peer.Signer, Fingerprint: peer.Fingerprint, AcceptedPeer: peer.AcceptedPeer,
		Reason: peer.Reason, Valid: peer.Valid,
		// Every party already on the document, not just the one on the other end of the socket
		// (P07.S07c).
		Signers: signersSoFar(doc),
	}
	// The request is held so the defer can name it: an unconditional clear drops whatever
	// is pending when it fires, which after a disarm-and-rearm is a LATER session's consent.
	req := &pendingReq{view: view, doc: doc, resp: ch}
	if !sc.s.sess.setPending(sc.anchor, req) {
		return false, "", nil, errors.New("session not armed")
	}
	defer sc.s.sess.clearPendingIf(req)
	select {
	case d := <-ch:
		if !d.accept {
			// **A decline ends this party's part in the ceremony, so its pins go (D29, P07.S02b).**
			//
			// The pins were taken on to make the ceremony possible; refusing the document is
			// refusing the ceremony, and a revocable pin that outlives the thing it was for is
			// a permanent pin with extra steps. P01's parked criterion is exactly this and it
			// has been waiting for a caller since that phase closed.
			//
			// **A TIMEOUT is not a decline and does not prune** — that path returns
			// `ErrConsentTimedOut` below rather than coming through here. Nobody was at the
			// machine; the user has decided nothing, and unpinning on their behalf would make
			// stepping away from the desk revoke a relationship.
			sc.s.declineCeremony(sc.cer)
		}
		return d.accept, d.intent, d.appearance, nil
	case <-time.After(sessionConsentTimeout):
		// **Not `(false, nil)`.** That is what a user who read the document and refused it
		// returns, and collapsing the two here means the peer is told a person declined
		// when nobody was at the machine. See p2p.ErrConsentTimedOut.
		return false, "", nil, p2p.ErrConsentTimedOut
	}
}

// declineCeremony revokes the ceremony-scoped pins this machine took on for one ceremony (D29).
//
// Nil-safe in the ceremony: the manual and LAN paths have none, and a declined transfer there has
// nothing to revoke.
//
// Best-effort and it SAYS SO when it fails. A pin left behind is the failure that matters here —
// the peer list then shows a peer the user never chose, indistinguishable from one they did — and
// this runs on the p2p goroutine with no response left to write into, so a log line is the only
// channel there is. That is the shape `unconvene`'s own review established.
func (s *Server) declineCeremony(cer *ceremonyID) {
	if cer == nil {
		return
	}
	v := s.unlockedVault()
	if v == nil {
		return // locked mid-session; the pins are in a vault nothing can write to right now
	}
	if _, err := v.PruneCeremonyPeers(cer.inv.ID); err != nil {
		log.Printf("declined ceremony %s: could not remove its peer pins: %v — the peer list "+
			"still carries a pin this machine took on only for a ceremony that was refused",
			cer.inv.ID, err)
	}
	// And the stored invitation, which is this side's key material for a ceremony it just refused
	// (P08.S01). This is the invitee's own path — `declineCeremony` runs from the consent gate, on
	// the machine that said no — so it is the site where an accepted invitation most obviously
	// stops being wanted. Reported separately from the pins for `unconvene`'s reason: the two fail
	// independently and a user can act on each.
	if _, err := v.PruneCeremonyInvitations(cer.inv.ID); err != nil {
		log.Printf("declined ceremony %s: could not remove its stored invitation: %v — it carries "+
			"the ceremony secret and this machine still holds it", cer.inv.ID, err)
	}
}

// armInvitation resolves the ONE invitation text an arm acts on (P08.S01).
//
// Three inputs, three answers: a stored ceremony id loads the invitation this machine accepted; a
// literal invitation is used as given; neither is the manual/LAN path, which arms with no ceremony
// at all and is what `errNoCeremony` downstream expects.
//
// **Both is refused.** Not because it is hard to pick one — because picking one makes the other
// disappear without a word, and the caller who supplied it has no way to learn which the arm used.
// The same reasoning `checkTransport` records for an unknown transport: refuse, never downgrade.
func (s *Server) armInvitation(v *vault.Vault, req armRequest) (string, error) {
	if req.Ceremony != "" && req.Invitation != "" {
		return "", errors.New("this request names a stored ceremony AND carries an invitation — " +
			"send one or the other, because Nib will not choose for you which ceremony you meant")
	}
	if req.Ceremony == "" {
		return req.Invitation, nil
	}
	text, ok := v.CeremonyInvitationFor(req.Ceremony)
	if !ok {
		// Distinguished from a malformed invitation, which is the 400 above: this machine has
		// simply never accepted one for that ceremony, or its close-out has already removed it.
		// The id is printed whole, not shortened: it is the caller's own input and the user's
		// only way to match this refusal against what they sent.
		return "", fmt.Errorf("this machine holds no invitation for ceremony %s — accept the "+
			"invitation first, or paste it into this request", req.Ceremony)
	}
	return text, nil
}

// sessionVerifier is the spoken-check bridge: p2p calls it with the four words, on both
// sides, before any document byte. It parks them for the UI and blocks on the user.
//
// Timing out returns p2p.ErrVerificationTimedOut rather than a bare false, so "nobody was
// at the machine" stays distinguishable from "the words did not match" — which are the
// same outcome for the session and completely different facts about what happened.
type sessionVerifier struct {
	s   *Server
	saw *reached
}

// errVerifyBusy is returned when another session's spoken check is already on screen.
var errVerifyBusy = errors.New("another co-signing session is already waiting for the spoken check — finish or cancel that one first")

func (sv sessionVerifier) ConfirmVerification(words string) (bool, error) {
	sv.saw.mark() // the spoken check is about to go on screen
	ch := make(chan bool, 1)
	pv := &pendingVerify{words: words, resp: ch}
	if !sv.s.sess.setVerify(pv) {
		// A gate is already on screen for another session. Declining is the fail-closed
		// answer: silently displacing it would route this user's answer to words they
		// never saw, and waiting would hang until a five-minute timeout with nothing shown.
		return false, errVerifyBusy
	}
	defer sv.s.sess.clearVerifyIf(pv)
	select {
	case ok := <-ch:
		return ok, nil
	case <-time.After(sessionConsentTimeout):
		return false, p2p.ErrVerificationTimedOut
	}
}

// setVerify parks the spoken check. Unlike setPending it does NOT require an armed
// listener: verification happens on both roles, and the dialing side has no listener at
// all. Refusing there would have made the gate unreachable for whoever initiated — which
// is half of "both sides confirm".
//
// # It will not displace a live gate, and the INCUMBENT wins
//
// This used to assign unconditionally, while `clearVerifyIf` right below carries an
// identity guard for the mirror case — the two halves of one invariant disagreeing. Two
// gates can genuinely be in flight (an armed receive session while the user posts
// /api/session/send), and the second overwrote the first: the displaced goroutine then sat
// on a channel nobody would ever write to until its five-minute timeout, and `respondVerify`
// routed the user's answer to whichever was current.
//
// The user cannot tell them apart. `verifyView` deliberately carries no fingerprint and no
// peer label — sound reasoning for ONE gate, and exactly what makes two unanswerable — so a
// silent swap means they confirm four words belonging to a session they never saw.
//
// The incumbent wins because they may already be reading its words and about to answer.
// The newcomer is refused, which fails closed: its ConfirmVerification declines, and that
// session ends with a reason instead of hanging until a timeout.
func (se *session) setVerify(pv *pendingVerify) bool {
	se.mu.Lock()
	defer se.mu.Unlock()
	if se.verify != nil {
		return false
	}
	se.verify = pv
	return true
}

// currentVerify reports which gate is parked, for tests that must distinguish the
// incumbent from a newcomer. verifyView deliberately carries only the words, so it cannot
// tell two gates apart — which is the user's problem and is why setVerify refuses.
func (se *session) currentVerify() *pendingVerify {
	se.mu.Lock()
	defer se.mu.Unlock()
	return se.verify
}

// clearVerifyIf drops the pending verification only if `pv` is still the pending one —
// the same identity guard clearPendingIf carries, for the same reason: an unconditional
// clear fired by a finished check discards a LATER session's.
func (se *session) clearVerifyIf(pv *pendingVerify) {
	se.mu.Lock()
	if se.verify == pv {
		se.verify = nil
	}
	se.mu.Unlock()
}

// sessionAccepter is the consent bridge for a plain one-way transfer: p2p.ReceiveDocument
// calls it after a peer sends a document; it surfaces the document for review, parks the
// request for the UI to accept/decline, and blocks until the user responds (or the
// timeout declines). label is this user's pinned label for the sending peer.
type sessionAccepter struct {
	s      *Server
	label  string
	saw    *reached
	anchor consentAnchor
}

func (sa sessionAccepter) Accept(peerFP, doc []byte) (bool, error) {
	sa.saw.mark() // the transfer consent is about to go on screen
	ch := make(chan sessionDecision, 1)
	view := pendingView{Signer: sa.label, Fingerprint: hex.EncodeToString(peerFP), Reason: transferReason(doc), Valid: true}
	req := &pendingReq{view: view, doc: doc, resp: ch}
	if !sa.s.sess.setPending(sa.anchor, req) {
		return false, errors.New("session not armed")
	}
	defer sa.s.sess.clearPendingIf(req)
	select {
	case d := <-ch:
		return d.accept, nil
	case <-time.After(sessionConsentTimeout):
		return false, p2p.ErrConsentTimedOut // see sessionConfirmer.Confirm
	}
}

// arrivalDocName names a co-signed document by who it came from, which is the one fact about
// it the user cannot see on the page.
func arrivalDocName(peerLabel string) string {
	if peerLabel == "" {
		return "co-signed.pdf"
	}
	return "co-signed with " + peerLabel + ".pdf"
}

// inbound, when non-nil, is set the first time this listener ACCEPTS a connection. It is the
// arm's own answer to "did the local network reach me", which is what the DHT publish waits on
// — see startArmedRendezvous. Per-arm, unlike `reached`, which is per connection and asks a
// different question (did anything get in front of the user).
// runSession accepts one pinned peer and, depending on the armed mode, either
// co-signs with the user's consent (making the result the open document) or accepts a
// one-way document transfer and saves it under ~/nib. It always disarms on exit — one
// session per arm.
//
// cer is the ceremony this arm belongs to, or nil for the manual/LAN arm. **Handed in rather
// than left behind (P07.S02b):** `handleSessionArm` stored it on the session — `s.sess.arm(ln,
// cer)` — and then started this goroutine without it, so every TCP ceremony hop ran as though it
// were a manual transfer. See serveOneSession's own note for what depended on that.
func (s *Server) runSession(ln p2p.Listener, cer *ceremonyID, cert, key []byte, label, mode string, inbound *atomic.Bool) {
	// This goroutine handles a pinned peer's inbound document; a panic in the p2p or
	// sign code must not crash the desktop process. The defers below (disarm, Close)
	// still run as the stack unwinds.
	//
	// **First, before anything else defers.** safe.Recover's own contract says it must
	// be "deferred at the very top of a goroutine body", and the announcer block used
	// to sit above it — so on LIFO unwind the recover had already returned by the time
	// ann.Close() ran, and a panic there would have taken the process down. That is not
	// hypothetical: Close was a check-then-close that two callers could panic on.
	defer safe.Recover("session")
	// Announce on the link for as long as this session is armed — the plan's egress
	// enumeration authorises multicast "armed-only", and this is where that is true
	// rather than merely intended. It never fails the session: a host with no usable
	// interface, or a firewall that swallows the group, must still be able to run a
	// ceremony over a typed address.
	// The LISTENER, not its port: whether this session may be announced at all is a fact
	// about the address it bound, and `startAnnouncing` is the door that decides it
	// (ADR-009). A loopback bind announces nothing.
	var armAnnouncer *lanAnnouncer
	if ann, err := startAnnouncing(cert, ln, lanAnnounceWindow); err == nil {
		armAnnouncer = ann
		defer ann.Close()
	}
	// **The TCP arm answers hop seekers too, and reports the sighting (P07.S05e T02).**
	//
	// `runCeremonyReceive` has done this since S05c, and this path never did — so a TCP ceremony
	// arm could not be found after its five-minute announcement expired, which is the whole defect
	// S05c's own comment describes ("from the fourth party onward a same-room ceremony would
	// silently run over the public DHT"), and it had no evidence to hold its DHT publish on.
	//
	// **Two arm paths living in two functions is the same count S05d found one day earlier** for
	// the bootstrap: the plan named two eager sites and there were three, because these two arms
	// are not one function. A slice wired only where the machinery already existed would have
	// fixed QUIC and left TCP, with nothing failing to say so.
	//
	// Bound to this goroutine rather than to the process: the arm ends, the answering ends.
	if cer != nil {
		if fp, derr := hex.DecodeString(cer.peer); derr == nil && len(fp) > 0 {
			hctx, hcancel := context.WithCancel(context.Background())
			defer hcancel()
			go func() {
				defer safe.Recover("hop seeker answers")
				s.answerHopSeekers(hctx, cert, ln, fp,
					func() bool { return !cer.hasSigned() }, cer.noteLinkSighting, cer.watchingLink)
			}()
		}
	}
	// This user's own fingerprint, for the verification string — it binds both identities,
	// and this goroutine holds the cert rather than the fingerprint.
	myFP, err := sign.Fingerprint(cert)
	if err != nil {
		return
	}
	// Every teardown in this goroutine names THIS listener. The timer matters as much as
	// the defer: Stop() runs just after Accept returns, and a timer that fires in that
	// window would otherwise disarm whatever is armed by then.
	//
	// `armedUntil` is the same bound as an ABSOLUTE time, and it exists because the timer
	// is now stopped and restarted rather than stopped once. A duration cannot be
	// restarted correctly — resetting to `sessionAcceptTimeout` after a peer connected
	// and produced nothing would extend the arm window every time somebody dialled, which
	// is a window an attacker holds open for free.
	//
	// **A ceremony arm waits for the ceremony, and this path did not (P08.S04, C05, D16's
	// amendment).** `runCeremonyReceive` has bounded a ceremony arm by `ceremony.MaxCeremonyLife`
	// since P05.S09b, and this function — every TCP arm, ceremony or not — kept the five-minute
	// manual bound. So a party third in a roster armed, waited while two earlier hops ran, and was
	// disarmed before the baton reached them. `lan.go`'s own comment states the 30-day window as
	// though it were the arm's property generally; it was true of one of the two paths.
	//
	// **Two arm paths living in two functions is the same count S05d and S05e each found**, and
	// this is the third thing that had to be added to both and reached one.
	//
	// The bound is a CONSTANT and D16's amendment asks for a ceremony-scoped one — the record's
	// `Expires`. That is not buildable here: an arm holds an invitation, and the invitation carries
	// no deadline. Giving it one is `/pending 247`, whose own grill found the field would be
	// consumed at arm time while nothing could check it until the document arrives. So the honest
	// bound today is the same ceiling the other path uses, and the refinement waits on that item.
	armWindow := armWindowFor(cer)
	armedUntil := time.Now().Add(armWindow)
	// postSign is the re-delivery window's deadline, zero until this arm has signed. opened keeps
	// the co-signed document opening ONCE across re-deliveries. Both mirror runCeremonyReceive.
	var postSign time.Time
	opened := false
	timer := time.AfterFunc(armWindow, func() {
		defer safe.Recover("arm window expiry")
		s.sess.disarmIf(ln)
	})
	defer timer.Stop()
	defer s.sess.disarmIf(ln)

	// **Accept until a peer completes a SESSION, not until one completes a handshake.**
	//
	// tls.Listener.Accept returns on the TCP accept and does no handshake, so ANY
	// connection consumed the one-shot session: a port scan, a stray dial, a wrong
	// address — refusal is free for whoever connects and expensive for the user, who has
	// to notice their session died and arm it again. Handshaking here and looping on
	// failure means only a peer holding the pinned identity can take the session.
	//
	// **That argument did not go far enough, and P05.S01 is where it is finished.** The
	// loop tolerated a failed handshake and then spent the arm on the first COMPLETED
	// one, whatever came of it: a pinned peer whose connection dropped mid-exchange, or
	// whose protocol failed, took the session with it and left the user re-arming. The
	// distinction that matters is not "did somebody connect" but "did a session happen",
	// and every reason above applies unchanged one step later.
	//
	// It stops being a rare annoyance in P05.S02. The ladder races several candidate
	// addresses that all reach this one listener — a dual-stack peer, or one with wifi
	// and ethernet, publishes two or three — so several pinned handshakes complete here
	// and the racer keeps one. Under the old rule, whichever connection this loop
	// happened to accept first *was* the ceremony, and if the dialer kept a different one
	// the arm was consumed by a connection nobody was using.
	//
	// The arm window still bounds the whole of it: the timer is stopped while a session
	// is in flight — `disarmIf` declines a pending consent, so firing mid-session would
	// refuse on the user's behalf — and re-armed for the REMAINDER, never a fresh period.
	for {
		conn, err := ln.Accept()
		if err != nil {
			// net.ErrClosed is the listener being gone — the accept timeout fired, or
			// the session was disarmed. Anything else is THIS peer failing (not the
			// pinned identity, no handshake, a stray dial) and the session stays armed,
			// because refusal is free for whoever connects and expensive for the user.
			if errors.Is(err, net.ErrClosed) {
				return
			}
			continue
		}
		timer.Stop()
		if inbound != nil {
			inbound.Store(true)
		}
		served, final, _ := s.serveOneSession(consentAnchor{ln: ln}, cer, conn, cert, key, label, mode, myFP)
		if final != nil && !opened {
			s.openArrival(label, final) // once: a re-delivery re-sends the SAME idempotent result
			opened = true
		}
		if served {
			// ── The post-signing RE-DELIVERY window, on the TCP ceremony path (/pending 289) ──
			//
			// **P05.S10's criterion 15 was implemented on ONE of the two transports.**
			// `runCeremonyReceive` — the QUIC ceremony path — keeps accepting for a bounded window
			// after it signs, because *"a lost writeback is indistinguishable from a clean success:
			// writeFrame does not confirm the initiator READ it"*. This loop returned instead, so on
			// TCP the listener closed the moment the co-sign completed and a reconnect was met with
			// `connection refused`. `coSignExchange` still wrote its cache; nothing could ever come
			// back for it.
			//
			// It was invisible because the one behavioural drive of re-delivery ran QUIC, and the
			// TCP rule was guarded only structurally — asserting that both call sites PASS a
			// ceremony says nothing about what either does with it. Found by running that test's
			// own body over TCP.
			//
			// **Gated on a ceremony that has SIGNED, and both halves matter.** Without a ceremony
			// there is no `ReDeliverer` and no cache, so holding the arm open would buy nothing and
			// cost an arm that outlives its session — P05.S01's whole point is that the arm is
			// one-shot. Before signing there is nothing to re-deliver, and `served` is then a
			// decline or a consent timeout, which are decisions rather than losses.
			if cer == nil || !cer.hasSigned() {
				return // the arm is spent on a session, which is what it is for
			}
			if postSign.IsZero() {
				// **Stop announcing, for the reason runCeremonyReceive's twin gives (/pending 300):**
				// a re-delivery is a reconnect by a peer that already holds this address, so the
				// window needs the listener and not the advertisement. Announcing through it
				// leaves a stale candidate on the link that a later ceremony's browse can pick up.
				armAnnouncer.Close()
				// The initiator's own re-race bound, so the window closes at the moment the far
				// side stops trying — the same figure runCeremonyReceive uses, for the same reason.
				//
				// **An ABSOLUTE deadline, fixed once, and the timer reset to its REMAINDER** — the
				// rule `TestTheArmWindowIsNotExtendedByConnectionsThatProduceNoSession` polices,
				// and it applies to this second window for the same reason it applies to the
				// first: a `Reset` to a fresh period would let each reconnect push the window out,
				// and a re-delivery window anybody who can reach the listener holds open for free
				// is the same defect one phase later.
				postSign = time.Now().Add(connectDeadline)
				remaining := time.Until(postSign)
				timer.Reset(remaining)
				continue
			}
			if time.Now().After(postSign) {
				return
			}
			continue
		}
		remaining := time.Until(armedUntil)
		if postSign.IsZero() {
			if remaining <= 0 {
				return
			}
			timer.Reset(remaining)
			continue
		}
		// Inside the re-delivery window an unserved connection is an ordinary failed reconnect;
		// keep the window rather than falling back to the pre-signing bound, which is longer.
		if time.Now().After(postSign) {
			return
		}
	}
}

// serveOneSession runs one accepted connection to its outcome and reports whether that
// outcome SPENT the arm.
//
// True for a completed exchange and true for a DECLINE — a decline is a decision the user
// made, not a connection that failed, and leaving the listener armed after one would let a
// peer re-dial and ask the same person again. False for everything else: a dropped
// channel, a protocol error, a consent that timed out. That is the whole judgment in this
// function, and it is why the co-signing decline needed a sentinel of its own
// (`p2p.ErrCoSignDeclined`) rather than the bare error it used to return — `errors.Is`
// could not otherwise tell it from the protocol error declared one line away.
// cer is the ceremony this session belongs to, or nil outside one.
//
// **Explicit, rather than read off `anchor.cer` (P07.S02b).** The anchor carries a ceremony only
// on the QUIC coordinator path (`consentAnchor{cer: cer}`); the accept loop builds
// `consentAnchor{ln: ln}` and a TCP ceremony hop therefore arrived here with no ceremony at all,
// although the arm had stored one. Two things depended on that nil and were wrong for it — the
// re-deliverer below, and P07.S02b's C17 gate.
//
// Filling in `anchor.cer` on the TCP path would have been the smaller diff and the wrong change:
// `consentAnchor.current` PREFERS `cer` when it is non-nil, so that path's stale-goroutine test
// would silently switch from listener-identity to ceremony-identity — a change to what
// `stale-consent-on-new-session` guards, smuggled in under another slice's name.
func (s *Server) serveOneSession(anchor consentAnchor, cer *ceremonyID, conn *p2p.Conn, cert, key []byte, label, mode string, myFP []byte) (served bool, coSigned []byte, err error) {
	// **Deferred here rather than closed by the caller**, so the connection is released
	// even if this function panics. `runSession`'s `safe.Recover` catches such a panic and
	// keeps the desktop process alive, which is exactly the case where a caller-side
	// `conn.Close()` after the call would be skipped — the shape the pre-P05.S01 loop got
	// right with a `defer` and the first draft of this split lost.
	defer conn.Close()
	// **What spends the arm is whether this connection REACHED THE USER**, not which
	// error it ended with.
	//
	// The first draft enumerated outcomes — a decline spends it, anything else does not —
	// and the enumeration was both wrong and the wrong shape. It missed
	// `p2p.ErrVerificationDeclined`, which is the MAN-IN-THE-MIDDLE signal: the user
	// looked at the four words and said they did not match. `internal/p2p/verify.go` says
	// what that must never become — *"it must never be reported as a network error —
	// 'could not connect' invites a retry, which is the worst possible advice when someone
	// is sitting between you"* — and an enumeration that let it fall through to "no
	// session" made the listener **perform that retry itself**, silently, as many times as
	// the attacker liked, while the status still read `Armed: true`. Measured at two full
	// rounds in 0.47 s.
	//
	// It also mis-stated the timeouts in the opposite direction: a consent nobody answers
	// returns `accept=false` with a nil error (`sessionConfirmer.Confirm`), so it arrives
	// here as a decline — the enumeration's comment claimed it did not.
	//
	// Engagement is the honest predicate and it needs no enumeration to stay correct. A
	// connection abandoned by P05.S02's racer dies at the first frame, having shown the
	// user nothing, and leaves the arm. A connection that put the spoken check or a
	// consent request on screen has spent it, however it ended — declined, refused,
	// unanswered, or dropped after the user had already said yes. The default for an
	// error nobody anticipated is therefore "spent", which is HEAD's behaviour and the
	// safe direction; only the narrow, positively-identified never-reached-anyone case
	// is loosened, which is the whole of what this slice needs.
	var saw reached
	ch := conn.Channel
	if mode == sessionModeReceive {
		doc, derr := p2p.ReceiveDocument(ch, sessionAccepter{s: s, label: label, saw: &saw, anchor: anchor}, myFP, sessionVerifier{s, &saw})
		if derr != nil {
			return saw.v.Load(), nil, derr
		}
		s.saveReceived(doc, ch.PeerFP, label)
		return true, nil, nil // a transfer saves itself; no co-signed document to open
	}
	var rd p2p.ReDeliverer
	if cer != nil {
		// Idempotent re-delivery for a ceremony hop (P05.S10). **From `cer`, not `anchor.cer`
		// (P07.S02b)**: this used to read the anchor, which is empty on the TCP ceremony path,
		// so a TCP hop got nil — and `ReDeliverer`'s own contract says nil means "the manual/LAN
		// path, which has no ceremony hop to key on", which was false there. The accept loop
		// re-arms for the remainder and accepts again, so a peer reconnecting after a lost
		// channel reached `coSignExchange` with no cache and `Contribute` stacked a second,
		// different block.
		rd = cer
	}
	final, rerr := p2p.Receive(ch, cert, key, label, sessionConfirmer{s: s, saw: &saw, anchor: anchor, cer: cer}, sessionVerifier{s, &saw}, rd, cer.l3Roster())
	// **"Signed but not saved" is an outcome with a document, not a failure (P08.S02, D24 as
	// amended).** The peer has the signature; this machine could not keep a copy. So the error is
	// reported to the user and the document is still returned, opened and treated as arrived —
	// which is what makes the recovery possible at all: the bytes are in a tab the user can save
	// somewhere with space.
	//
	// D24's sentence VERBATIM, both halves. The criterion quoted only "signed but not saved", and
	// the clause that prevents the loss is the second one: closing Nib is what destroys it.
	if p2p.PersistFailed(rerr) {
		s.sess.noteFailure("signed-not-saved",
			"Signed, but not saved — do not close Nib.",
			"The other party has your signature; it is on their copy of the document either way. "+
				"What failed is this machine keeping its own copy, so closing Nib now would lose "+
				"it. The document is open — save a copy somewhere with space. Reason: "+rerr.Error())
		rerr = nil
	}
	if rerr != nil {
		return saw.v.Load(), nil, rerr
	}
	// The co-signed document is RETURNED, not opened here: under P05.S10's re-delivery loop this
	// function runs again on every reconnect, and opening on each would stack duplicate tabs of one
	// idempotent result (diff-grill). The caller opens it exactly once.
	return true, final, nil
}

// openArrival opens a co-signed document alongside whatever the user already had (D10) — an arrival
// opens, never replaces. Named so a reload does not show it as "Untitled".
func (s *Server) openArrival(label string, final []byte) {
	// **The mirror write is NOT here any more (P08.S02).** It used to be, and that was the defect:
	// `openArrival` runs after `p2p.Receive` has already put the document on the wire, so the bytes
	// reached the peer first and the disk second, best-effort, with a log line on failure. D24 asks
	// for the opposite order and says why. The write now happens inside `ceremonyID.Store`, which
	// `coSignExchange` calls before the frame — one door, before the wire, and able to report.
	//
	// `mirrorHop` survives for the INITIATING side (`handleSessionInitiate`), where there is no
	// `ReDeliverer` and the response is the ordering C22 names.
	s.addDoc(&document{name: arrivalDocName(label), data: final, sig: sign.Verify(final)})
}

// saveReceived writes an accepted one-way transfer under ~/nib, routed by what the
// document is: a flagged PDF (awaiting the user's signature) lands in to-sign/, an
// already-signed one in signed/, anything else in incoming/. Best-effort — a write
// failure leaves the user's other documents untouched and simply reports nothing.
func (s *Server) saveReceived(doc, peerFP []byte, peerLabel string) {
	path := filepath.Join(defaultOutputDir(), receivedSubdir(doc), receivedName(peerLabel, peerFP))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		s.sess.noteFailure("received-not-saved",
			"A document arrived and could not be saved.",
			"Nib could not create the folder it saves incoming documents into. Reason: "+err.Error())
		return
	}
	// A document a PEER sent. Losing it means asking them to send again, and they may be gone.
	//
	// **It used to fail in complete silence** — a bare `return`, with this function's own doc
	// saying "simply reports nothing". That is the one path in the tree that loses a peer's
	// document with no trace at all, and the peer has already been told it was accepted.
	if err := atomicfile.WriteDurable(path, doc, 0o600); err != nil {
		s.sess.noteFailure("received-not-saved",
			"A document arrived and could not be saved.",
			"The sender has already been told it was accepted, so they will not send it again. "+
				"Reason: "+err.Error())
		return
	}
	s.sess.setReceived(&receivedInfo{Path: path, Peer: peerLabel})
}

// receivedSubdir picks the ~/nib subdirectory for a received document from its own
// content — the signing workflow's state travels inside the PDF, not in app state.
func receivedSubdir(doc []byte) string {
	if flags, _ := pdfops.FlagsJSON(doc); len(flags) > 0 {
		return "to-sign"
	}
	if sign.Verify(doc).State != sign.Unsigned {
		return "signed"
	}
	return "incoming"
}

// transferReason describes an incoming transfer for the consent pane, derived from
// the document so the user knows what they're being asked to keep.
func transferReason(doc []byte) string {
	switch receivedSubdir(doc) {
	case "to-sign":
		return "wants to send you a document to sign"
	case "signed":
		return "is sending you a signed document"
	default:
		return "wants to send you a document"
	}
}

// receivedName builds a stable, filesystem-safe name for a received document; the
// wire carries no original filename, so the sender's label and the arrival time
// identify it. labelSlug falls back to a short fingerprint when the label is empty
// or unprintable.
func receivedName(peerLabel string, peerFP []byte) string {
	slug := labelSlug(peerLabel)
	if slug == "" {
		slug = hex.EncodeToString(peerFP)[:8]
	}
	return slug + "-" + time.Now().Format("20060102-150405") + ".pdf"
}

// labelSlug reduces a peer label to lowercase alphanumerics-and-dashes for a filename.
func labelSlug(label string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(label) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

// --- HTTP handlers (all behind requireUnlocked: vault-unlocked, CSRF, loopback origin) ---

// sessionModeCoSign is the other member, and it had no Go name at all until v1.116.11.
//
// The mode was used raw — `if mode == sessionModeReceive`, and anything else co-signed —
// which is byte-for-byte the defect `checkTransport` was written to refuse, with the
// argument spelled out over sixteen lines and a test named
// `TestAnUnknownTransportIsRefusedNotSilentlyDowngraded`. "Receive", "recieve" and
// "transfer" all silently armed a CO-SIGNING listener when the user asked for a transfer.
// The client already had a name for it (`'cosign'`, web/app.js); Go knew it only by
// negation, which is also why the display↔code pair had drifted.
const sessionModeCoSign = "cosign"

// checkSessionMode refuses a mode this build does not know, rather than defaulting.
func checkSessionMode(mode string) error {
	switch mode {
	case sessionModeReceive, sessionModeCoSign, "":
		// "" is co-sign, kept because it is what older clients send and the route has
		// always treated it that way — an accepted spelling, not a silent fallback.
		return nil
	}
	return fmt.Errorf("%w: %q (this build knows %q and %q)",
		errUnknownSessionMode, mode, sessionModeCoSign, sessionModeReceive)
}

var errUnknownSessionMode = errors.New("unknown session mode")

// sessionModeReceive arms the listener to accept a one-way document transfer (save to
// ~/nib); any other mode value co-signs.
const sessionModeReceive = "receive"

type armRequest struct {
	Fingerprint string `json:"fingerprint"` // the single peer to accept (hex SPKI)
	Bind        string `json:"bind"`        // host:port to bind, e.g. "0.0.0.0:8443"
	// Address is an OPTIONAL typed address of the peer to DIAL as well as accept (P05.S09). An
	// arm normally waits and dials only what the DHT publishes, but a symmetric-racing ceremony
	// where the receive role must also reach out — the manual tier for the arm, and what lets a
	// forced glare be driven on one host — names the peer's address here.
	Address   string `json:"address,omitempty"`
	Mode      string `json:"mode,omitempty"`      // "receive" for a transfer; co-sign otherwise
	Transport string `json:"transport,omitempty"` // "quic"; anything else is TCP
	// Invitation is the pasteable ceremony invitation (D21), and it is what gives this arm
	// a ceremony identity: a roster, a hop, and the secret every rendezvous derivation
	// needs. Optional — an arm without one is the manual/LAN path this route has always
	// served, and D9 keeps that path rather than deleting it.
	//
	// **The invitation travels in the request rather than living on disk.** It is a channel
	// secret and the BEP-44 write authority for every hop of the ceremony, and ADR-006 has
	// already reasoned once about which secrets Nib persists. P06's resumption criterion
	// says the panel renders "from the local record" — the PDF attachment — not from a
	// stored invitation, so nothing here needs one at rest.
	Invitation string `json:"invitation,omitempty"`
	// Ceremony re-arms from what this machine already holds: the id of a ceremony whose
	// invitation is in the vault, stored when it was accepted (P08.S01, D24).
	//
	// **It is an alternative to `Invitation`, never a companion.** Supplying both is refused
	// rather than resolved by code order — two sources for one value is the drift this repo keeps
	// finding, and the loser would be silent. Supplying neither is the manual/LAN path, unchanged.
	//
	// It exists because a restart used to lose the ceremony on every machine but the convener's:
	// the invitation travelled in this request and nowhere else, so a party who closed Nib had a
	// pin, an identity, and no way to rejoin.
	Ceremony string `json:"ceremony,omitempty"`
}

type sessionStatus struct {
	Armed    bool          `json:"armed"`
	Address  string        `json:"address,omitempty"`
	Verify   *verifyView   `json:"verify,omitempty"`
	Pending  *pendingView  `json:"pending,omitempty"`
	Received *receivedInfo `json:"received,omitempty"`
	// Diagnosis is the live D19 diagnosis for a ceremony arm that is still WAITING — most usefully
	// "the other side hasn't started" (cause 1) — so the polling UI shows why nothing has connected
	// yet, rather than a blank wait (P05.S11). Computed lazily from safe signals; nil for a manual
	// arm or once a session is in progress.
	Diagnosis *diagnosisView `json:"diagnosis,omitempty"`
	// Until is when this arm gives up. Present so the window can be checked rather than inferred
	// from whether the ceremony happened to finish in time — see session.until.
	Until *time.Time `json:"until,omitempty"`
	// Notice is the last background failure, and it outlives the session — see session.notice.
	Notice *noticeView `json:"notice,omitempty"`
}

// noticeView is something that went wrong where no response could carry it (P08.S08).
//
// `What` is a stable machine-readable reason, so a surface can key on it without matching prose —
// the shape P06 will need, and the reason a refusal in this repo carries a code rather than a
// sentence chosen by somebody else.
type noticeView struct {
	What    string    `json:"what"`
	Summary string    `json:"summary"`
	Detail  string    `json:"detail,omitempty"`
	At      time.Time `json:"at"`
}

// diagnosisView is the arm-side D19 message the status poller carries — plain summary, cause tag,
// and technical detail behind a disclosure.
type diagnosisView struct {
	Cause   string `json:"cause"`
	Summary string `json:"summary"`
	Detail  string `json:"detail"`
}

// verifyView is the four words the user must confirm against what the other person reads
// aloud. It carries nothing else — no fingerprint, no peer label — because anything else
// on this card is something to read INSTEAD of the words.
type verifyView struct {
	Words string `json:"words"`
}

type pendingView struct {
	Signer       string `json:"signer"`
	Fingerprint  string `json:"fingerprint"`
	AcceptedPeer string `json:"acceptedPeer"`
	Reason       string `json:"reason"`
	Valid        bool   `json:"valid"`
	// Signers is every party who has ALREADY signed this document, in signature order
	// (D27 item 3, C09; P07.S07c).
	//
	// **The consent screen showed exactly one person, and in a ceremony that is the wrong
	// one.** It named the connected peer — who under a carry route is a non-signing convener,
	// and who at hop 6 is whoever dialled rather than the five parties whose signatures the
	// user is about to join. A party asked to sign sixth was told about one person and shown a
	// document bearing five signatures, which is the decision D27 says they must be equipped to
	// make.
	//
	// Empty on the first hop of a ceremony and on an ordinary transfer, and that is a fact the
	// screen states rather than a list it omits: "nobody has signed this yet" and "we did not
	// look" must not render the same.
	Signers []pendingSigner `json:"signers"`
}

// pendingSigner is one already-present signature, for the consent screen.
//
// `Signer` is the name the signature carries, which since P07.S07a is the party's roster label
// rather than the `"Nib User"` constant — so a nine-party ceremony's consent screen names nine
// different people instead of nine copies of one.
type pendingSigner struct {
	Signer      string `json:"signer"`
	Fingerprint string `json:"fingerprint"`
	Valid       bool   `json:"valid"`
}

// signersSoFar reads the signatures already on a document.
//
// `sign.Verify` rather than `p2p.Attestations`: the question is who has signed, which the
// signature list answers directly, and the attestation machinery would additionally parse
// /Reason and cross-bind — work whose answers this screen does not show. The consent gate is a
// human pause, not a hot path, but a verify it does not use is still a verify.
func signersSoFar(doc []byte) []pendingSigner {
	st := sign.Verify(doc)
	out := make([]pendingSigner, 0, len(st.Signers))
	for _, s := range st.Signers {
		out = append(out, pendingSigner{Signer: s.Name, Fingerprint: s.Fingerprint, Valid: s.Valid})
	}
	return out
}

func (s *Server) handleSessionArm(w http.ResponseWriter, r *http.Request) {
	v := vaultFrom(r)
	var req armRequest
	if err := readJSON(r, &req); err != nil {
		httpError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	peerFP, err := parseFingerprint(req.Fingerprint)
	if err != nil {
		httpError(w, http.StatusBadRequest, "not a valid fingerprint")
		return
	}
	label, ok := pinnedLabel(v, peerFP)
	if !ok {
		httpError(w, http.StatusBadRequest, "that peer isn't pinned — pin their fingerprint first")
		return
	}
	// Refused, not defaulted. See checkSessionMode.
	if err := checkSessionMode(req.Mode); err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	// An empty bind is the LAN path, not a mistake. The peer will learn the port from
	// the announcement, so there is nothing for a user to type and nothing to agree in
	// advance — which is the whole of P03's first exit criterion. Ephemeral rather than
	// a fixed default because two Nibs on one machine must both be able to arm, and a
	// hardcoded port makes the second one fail for a reason the user cannot act on.
	bind := req.Bind
	if bind == "" {
		bind = "0.0.0.0:0"
	}
	cert, key, err := identity(v)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "could not load identity")
		return
	}
	// **The ceremony identity is resolved BEFORE the listener opens.** A bad invitation is
	// the caller's mistake and deserves a 400, and resolving it first means a refusal costs
	// no socket — the same ordering `peerAddresses` uses for a typo'd transport, and the
	// reason it gives: a 400 the user can act on rather than a 502 about a peer.
	//
	// **One source for the invitation text, chosen before anything is parsed (P08.S01).** A
	// request may name a stored ceremony or carry the text, and supplying both is a 400: the
	// alternative is that one silently wins, and a caller who believed the other was in force
	// arms for a ceremony they did not name. `invText` is what `ceremonyFor` gets either way, so
	// there is one parse and one door.
	invText, terr := s.armInvitation(v, req)
	if terr != nil {
		httpError(w, http.StatusBadRequest, terr.Error())
		return
	}
	cer, cerr := ceremonyFor(invText, cert, key, peerFP)
	if cerr != nil && !errors.Is(cerr, errNoCeremony) {
		httpError(w, http.StatusBadRequest, cerr.Error())
		return
	}

	// P05.S09: a QUIC ceremony arm both LISTENS and DIALS over the one shared endpoint, joined by
	// the glare — so a peer we reach by dialing is co-signed here, not only one that dials us. The
	// coordinator owns the single handshaked listener (a transport permits one), so this path does
	// not open its own accept listener and does not start startArmedRendezvous: connect's feed does
	// the same bootstrap-fed publish, punch and port-mapping. TCP ceremonies and the non-ceremony
	// arm keep the runSession path below.
	if cer != nil && req.Transport == transportQUIC {
		// An optional typed peer address makes this arm DIAL as well as accept (the receive role
		// reaching out) — nil otherwise, and connect races only the DHT and its accept.
		var armCands []candidate
		if req.Address != "" {
			var ok2 bool
			armCands, ok2 = s.peerAddresses(w, v, req.Address, req.Transport, peerFP)
			if !ok2 {
				return // peerAddresses wrote the error
			}
		}
		if serr := cer.setupSharedEndpoint(bind, s.configDir); serr != nil {
			cer.close()
			httpError(w, http.StatusBadRequest, "could not open the ceremony endpoint: "+serr.Error())
			return
		}
		hl, herr := p2p.QUICListenHandshakeOn(cer.end, cert, key, peerFP)
		if herr != nil {
			cer.close()
			httpError(w, http.StatusInternalServerError, "could not arm the racing accept: "+herr.Error())
			return
		}
		armCtx, cancel := context.WithCancel(context.Background())
		if !s.sess.armCeremony(cer, cer.end.LocalAddr().String(), cancel) {
			cancel()
			hl.Close()
			cer.close()
			httpError(w, http.StatusConflict, "a session is already armed")
			return
		}
		go s.runCeremonyReceive(armCtx, cer, hl, armCands, cert, key, label, req.Mode, peerFP)
		writeJSON(w, s.sess.status())
		return
	}

	// With a ceremony, the listener is opened on a socket the DHT SHARES (caveat 7): a NAT
	// mapping is a function of the internal IP:port, so the socket the probe measures and the
	// socket the session answers on must be the same one. Without a ceremony this is
	// `listenPeer` exactly as before.
	var ln p2p.Listener
	if cer != nil {
		ln, err = cer.openRendezvous(req.Transport, bind, s.configDir, cert, key, peerFP)
	} else {
		ln, err = listenPeer(req.Transport, bind, cert, key, peerFP)
	}
	if errors.Is(err, errUnknownTransport) {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		httpError(w, http.StatusBadRequest, "could not open listener: "+err.Error())
		return
	}
	if !s.sess.arm(ln, cer) {
		ln.Close()
		cer.close()
		httpError(w, http.StatusConflict, "a session is already armed")
		return
	}
	var inbound atomic.Bool
	go s.runSession(ln, cer, cert, key, label, req.Mode, &inbound)
	// Warm the DHT and, unless the local network answers first, publish where we can be
	// reached. Started after the arm so a refused arm leaves no background work behind.
	s.startArmedRendezvous(cer, req.Transport, &inbound)
	writeJSON(w, s.sess.status())
}

// runCeremonyReceive is the arm side of a symmetric-racing ceremony (P05.S09): it LISTENS and
// DIALS over the one shared endpoint through the connect coordinator, promotes the surviving
// channel as the RECEIVER, and runs the exchange on it. It replaces runSession + startArmedRendezvous
// for a QUIC ceremony arm — connect's feed does the same bootstrap-fed publish, punch and
// port-mapping — and reaches the consent gate through the ceremony anchor (T05), because a receive
// role that won by DIALING has no listener to key on.
func (s *Server) runCeremonyReceive(ctx context.Context, cer *ceremonyID, hl *p2p.HandshakeListener, cands []candidate, cert, key []byte, label, mode string, peerFP []byte) {
	defer safe.Recover("ceremony receive")
	defer hl.Close()
	// Identity-guarded: a stale goroutine finishing after a cancel-and-rearm must not disarm the
	// ceremony that replaced it (the cer analogue of runSession's disarmIf(ln)).
	defer s.sess.disarmCeremony(cer)

	myFP, err := sign.Fingerprint(cert)
	if err != nil {
		return
	}
	// Announce this arm's shared endpoint on the LAN, as runSession did (diff-grill, HIGH): the
	// dialing side BROWSES for a ceremony peer, so without this a LAN-local ceremony finds nothing
	// and is forced onto the DHT — the privacy leak the LAN-window suppression exists to avoid — or
	// cannot connect at all where the DHT is unreachable. It never fails the arm; a host with no
	// usable interface still races over the DHT and the accept.
	var armAnnouncer *lanAnnouncer
	if ann, aerr := startAnnouncing(cert, quicEndpointAnnounce{hl.Addr()}, lanAnnounceWindow); aerr == nil {
		armAnnouncer = ann
		defer ann.Close()
	}
	// **And after that window closes, this arm can still be FOUND (P07.S05c, T02).** The
	// announcement above is five minutes; the arm is ceremony-scoped and may be thirty days, so
	// from the fourth party onward a same-room ceremony would silently run over the public DHT.
	// This listens — which costs nothing — and answers the one peer it is armed for when that peer
	// announces itself at the start of its hop. See `answerHopSeekers` for why it is one
	// fingerprint and not every pinned one.
	go func() {
		defer safe.Recover("hop seeker answers")
		// `cer.hasSigned` is the "still worth finding" test: once this hop has signed, a peer that
		// reaches us can only be re-delivering, and it already has the address (/pending 300).
		s.answerHopSeekers(ctx, cert, quicEndpointAnnounce{hl.Addr()}, peerFP,
			func() bool { return !cer.hasSigned() }, cer.noteLinkSighting, cer.watchingLink)
	}()
	// **No bootstrap here (S05d).** The QUIC arm used to warm the DHT before anyone knew whether
	// the link would answer, which is off-link traffic on every hop of every ceremony carrying an
	// invitation. connect's feed and publish now reach it through `cer.ensureBootstrapped` after
	// the LAN window, and `bootstrapDone` is set inside that door — so this path no longer claims
	// a bootstrap it did not do. Still not fatal when it fails: the accept side and any fixed
	// candidates race regardless, and D19 cause 2 names a dead DHT.

	// The re-race / re-delivery loop (P05.S10). Two phases keyed on whether this hop has signed:
	//
	//  - BEFORE signing: the arm waits for the ceremony's WHOLE life (S09b T01, D33's MaxCeremonyLife
	//    — the invitation carries no per-ceremony deadline, so the 30-day max is the only bound) for
	//    the baton, racing dial+accept through connect. A channel lost here re-races and re-confirms
	//    on a fresh channel (D18); a DECIDED outcome (decline, MITM, consent timeout) ends the arm.
	//  - AFTER signing: a lost writeback is indistinguishable from a clean success (writeFrame does
	//    not confirm the initiator READ it — grill P0), so the receiver stays reachable for a BOUNDED
	//    window (~connectDeadline, the initiator's own re-race bound) to RE-DELIVER the cached
	//    signature idempotently, then disarms. Post-signing it only ACCEPTS — the initiator dials us —
	//    so it reuses hl.Accept rather than re-running connect's whole publish/punch/DHT feed (grill P2).
	//
	// The TRIPWIRE (see its comment) is honoured as "SIGNS at most once", which the cache enforces:
	// a re-delivery hands back the same bytes, never a second signature.
	overallDeadline := time.Now().Add(armWindowFor(cer))
	var postSignDeadline time.Time
	opened := false // the co-signed document opens once, not per re-delivery
	for {
		var conn *p2p.Conn
		var cerr error
		if cer.hasSigned() {
			// Post-signing: accept a reconnect for re-delivery, bounded by the post-sign window.
			actx, acancel := context.WithDeadline(ctx, postSignDeadline)
			hc, aerr := hl.Accept(actx)
			if aerr != nil {
				acancel()
				return // the re-delivery window closed with no reconnect — the initiator got it or gave up
			}
			conn, cerr = hc.Promote(actx, false) // receiver: the initiator opens the stream
			acancel()
			if cerr != nil {
				continue // a failed accept; try again within the window
			}
		} else {
			// Pre-signing: the full race for the baton, bounded once by the ceremony life.
			cctx, ccancel := context.WithDeadline(ctx, overallDeadline)
			conn, cerr = s.connect(cctx, cer, hl, cands, cert, key, peerFP, myFP, false, label, label)
			ccancel()
			if cerr != nil {
				// A peer that connected and dropped BEFORE the exchange (a pre-gate channel loss)
				// surfaces as a connect error; re-race it while the ceremony window has time. The
				// deadline itself is a net.Error too, but by then Before(overallDeadline) is false, so
				// it falls through to disarm rather than spinning.
				if isTransportLoss(cerr) && time.Now().Before(overallDeadline) {
					continue
				}
				return // the baton never arrived, or the ceremony deadline passed
			}
		}
		_, final, xerr := s.serveOneSession(consentAnchor{cer: cer}, cer, conn, cert, key, label, mode, myFP)
		if final != nil && !opened {
			s.openArrival(label, final) // once: a re-delivery re-sends the SAME idempotent result
			opened = true
		}
		if postSignDeadline.IsZero() && cer.hasSigned() {
			postSignDeadline = time.Now().Add(connectDeadline) // first signature arms the re-delivery window
			// **And STOP ANNOUNCING, because the window needs the listener and not the
			// advertisement (/pending 300).**
			//
			// A re-delivery is a RECONNECT: the peer already has this address, so nothing about
			// the post-signing window requires telling the link about it. Announcing through it
			// invites somebody NEW to a hop that is finished.
			//
			// Measured: after a four-party QUIC relay completed, the convener still heard all
			// three parties announcing QUIC endpoints — six candidates, two per party. A later
			// ceremony on the same machines then browses a link full of them, and if its own
			// fresh announcement is missed inside the two-second browse, the candidate set is
			// entirely stale. That is how a TCP relay following a QUIC one came to fail at hop 1
			// with a D19 verdict about the DHT, for a peer that was on the link and announcing.
			//
			// `allQUICCandidates` (v1.117.182) already stops a MIXED set from taking the glare
			// path, which is why the failure became intermittent rather than reliable. This
			// removes the stale candidates instead of tolerating them, which is the half that
			// keeps the ordinary case honest too: a machine should not advertise a session that
			// will refuse the next person to answer it.
			armAnnouncer.Close()
		}
		switch {
		case xerr == nil:
			if !cer.hasSigned() {
				return // a clean completion that produced no signature (not the co-sign path) — done
			}
			// Signed and delivered cleanly, but a clean writeFrame does not prove the initiator read
			// it; keep the window open for a possible re-delivery until postSignDeadline.
		case isTransportLoss(xerr):
			// The channel dropped: re-race (pre-signing) or re-accept (post-signing).
		default:
			return // a decided outcome — decline, MITM, consent timeout — ends the ceremony
		}
	}
}

func (s *Server) handleSessionDisarm(w http.ResponseWriter, r *http.Request) {
	s.sess.disarm()
	writeJSON(w, s.sess.status())
}

func (s *Server) handleSessionStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.sess.status())
}

// handleSessionPendingPDF streams the received document awaiting consent so the UI
// can render it for review — separate from /api/pdf (the open document), which a
// received request never touches until the user accepts.
func (s *Server) handleSessionPendingPDF(w http.ResponseWriter, r *http.Request) {
	doc := s.sess.pendingPDF()
	if doc == nil {
		httpError(w, http.StatusNotFound, "no pending session request")
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(doc)
}

// handleSessionVerify records the user's answer to the spoken check (D4, L2).
//
// A separate route from /respond, because it answers a different question at a different
// point: this one is asked before any document byte moves, and /respond after the document
// has been read. One route taking both would let a consent answer resolve a verification.
func (s *Server) handleSessionVerify(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Confirmed bool `json:"confirmed"`
	}
	if err := readJSON(r, &req); err != nil {
		httpError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !s.sess.respondVerify(req.Confirmed) {
		httpError(w, http.StatusConflict, "no verification is waiting")
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) handleSessionRespond(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Accept     bool   `json:"accept"`
		Intent     string `json:"intent"`
		Appearance string `json:"appearance"` // base64 PNG, optional
	}
	if err := readJSON(r, &req); err != nil {
		httpError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	var appearance []byte
	if req.Appearance != "" {
		b, err := base64.StdEncoding.DecodeString(req.Appearance)
		if err != nil {
			httpError(w, http.StatusBadRequest, "invalid appearance image")
			return
		}
		appearance = b
	}
	if !s.sess.respond(sessionDecision{accept: req.Accept, intent: req.Intent, appearance: appearance}) {
		httpError(w, http.StatusConflict, "no pending session request")
		return
	}
	writeJSON(w, s.sess.status())
}

// handleSessionQuote returns the appearance lines for the responder's own visible
// attestation block, accepting the peer whose request is pending. Unlike
// /api/cosign/quote it never reads the open document: the responder's block is
// placed server-side on the *received* document (coSignExchange recomputes the
// placement), so the client needs only the canonical lines and a nominal rect to
// size the rasterized image — the same single-source guarantee, without binding to
// the wrong (open) document's page geometry.
func (s *Server) handleSessionQuote(w http.ResponseWriter, r *http.Request) {
	v := vaultFrom(r)
	var req struct {
		Intent string `json:"intent"`
	}
	if err := readJSON(r, &req); err != nil {
		httpError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	fp := s.sess.pendingFingerprint()
	if fp == "" {
		httpError(w, http.StatusConflict, "no pending session request")
		return
	}
	// The pending peer is the one the listener was armed for, so it is pinned;
	// cosignAttestation re-checks that and names "Nib User" as the signer, exactly
	// as coSignExchange does on accept. The nominal rect comes from p2p's one door
	// rather than being copied here — only its aspect is used, to size the PNG.
	att, ok := s.cosignAttestation(w, v, cosignParams{Fingerprint: fp, Intent: req.Intent})
	if !ok {
		return
	}
	writeJSON(w, cosignQuote{Lines: att.AppearanceLines(), Rect: p2p.NominalBlockRect()})
}

// handleDoc returns metadata for the open document — name, path, save-ability, and
// signature state — so the UI can refresh after the document changes out of band,
// as it does when runSession applies a received live co-signature asynchronously.
func (s *Server) handleDoc(w http.ResponseWriter, r *http.Request) {
	// Reports the addressed document. With one open document this resolves to the
	// same answer as before; the 409 is what a client needs to tell "that tab is
	// gone" from "nothing is open" — different facts driving different behaviour.
	doc, err := s.docFor(r)
	if err != nil {
		httpError(w, http.StatusConflict, "that document is no longer open")
		return
	}
	// The resolved document is now REPORTED, not discarded. It was thrown away when
	// docResponse took no argument and could only describe the active one — so a
	// client polling /api/doc for an inactive document was answered about a
	// different document entirely, with a 200 and no way to tell.
	writeJSON(w, s.docResponse(doc))
}

// handleSessionInitiate runs the dialing side of a live co-signing session: it
// signs the open document accepting the chosen pinned peer (the same prepare+sign
// path as Track A co-sign), dials that peer at the supplied reachable address over
// pinned-peer mTLS, exchanges the document, verifies the peer co-signed and accepted
// this user, and makes the doubly-signed result the open document. The appearance
// block is rasterized client-side and uploaded, exactly like /api/cosign/sign.
//
// Dialing an arbitrary address is safe: the mTLS handshake aborts before any bytes
// of the document are sent unless the address answers with the pinned peer's
// identity, which an impostor cannot present. This endpoint is reachable only
// behind requireUnlocked (unlocked + CSRF + loopback origin), so the dial is
// always a deliberate local action.
func (s *Server) handleSessionInitiate(w http.ResponseWriter, r *http.Request) {
	v := vaultFrom(r)
	cleanup, ok := parseMultipart(w, r, maxPDFBytes)
	if !ok {
		return
	}
	defer cleanup()
	// An empty address is the LAN path, not a mistake. It is resolved further down,
	// AFTER the peer's fingerprint is parsed and its pin confirmed: a browse needs the
	// pin to match against, and asking the link about a peer this user has not pinned
	// would be discovery choosing who to talk to, which is what L1 forbids.
	address := r.FormValue("address")
	pdfBytes, ok := formFileBytes(w, r, "pdf")
	if !ok {
		return
	}
	var p cosignParams
	if raw := r.FormValue("params"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &p); err != nil {
			httpError(w, http.StatusBadRequest, "invalid params")
			return
		}
	}
	att, ok := s.cosignAttestation(w, v, p)
	if !ok {
		return
	}
	peerFP, err := parseFingerprint(p.Fingerprint)
	if err != nil {
		httpError(w, http.StatusBadRequest, "not a valid fingerprint")
		return
	}
	appearance, ok := formFileBytes(w, r, "appearance")
	if !ok {
		return
	}
	cert, key, err := identity(v)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "could not load identity")
		return
	}
	myFP, err := sign.Fingerprint(cert)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "could not read own fingerprint")
		return
	}
	// **Before signing anything.** D16's Stage 6 pin: a hop must not start unless the ceremony
	// outlives one full exchange budget, because a hop admitted just before the deadline still
	// gets six minutes and would ask the far party to consent to a signature on a proceeding
	// that has already ended. Checked here rather than after `buildCoSigned`, which applies the
	// LOCAL user's signature — refusing after that leaves them signed into a ceremony this
	// build has just declared over.
	if err := checkCeremonyDeadline(pdfBytes, time.Now()); err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	// The dialing side's ceremony identity, from the same pasteable invitation the arm takes.
	// Absent, this is the manual and LAN path exactly as before.
	//
	// **Resolved BEFORE buildCoSigned (moved 2026-08-25, P07.S03).** It used to sit below, and
	// the L3 gate needs the roster before the local signature is applied — for the reason the
	// deadline check above already states in its own words: refusing after `buildCoSigned` leaves
	// the user signed into something this build has just refused. A signature cannot be taken
	// back off a document.
	peerLabel, _ := pinnedLabel(v, peerFP)
	cer, cerr := s.dialerCeremony(r.FormValue("invitation"), cert, key, peerFP)
	if cerr != nil && !errors.Is(cerr, errNoCeremony) {
		httpError(w, http.StatusBadRequest, cerr.Error())
		return
	}
	defer cer.close()
	// **Carrying or contributing is read off the roster, not chosen (P07.S05, C07).**
	//
	// A `signs:false` roster member moves the baton and adds nothing; anyone else contributes.
	// There is no separate route and no flag on the request, so a non-signing convener cannot
	// accidentally sign and a signer cannot accidentally skip their turn — both unrepresentable
	// rather than checked. `buildCoSigned` is SKIPPED on the carry path, which is the whole
	// point: it is the door that applies the local signature.
	// **C17 on the DIAL side, and it had exactly one caller before this (P07.S07b).**
	//
	// `checkArrival` reconciles the document against the invitation this ceremony identity was
	// built from, and it ran only in `sessionConfirmer.Confirm` — the receiving side. So a party
	// who INITIATES used its invitation's roster to gate L3 and, since S07a, to write labels and
	// capacities onto its own signature block, having never compared that invitation to the
	// record the document actually carries. `checkCeremonyDeadline` above verifies the record's
	// own signature, which is a different question: it says the record is genuine, not that it is
	// the record this invitation names.
	//
	// S07b is what made that untenable rather than merely asymmetric: the recital joins the
	// labels and capacities read off the invitation, and it is signed into every /Reason
	// verbatim. One rule, both doors (ADR-009).
	//
	// Placed BESIDE the deadline check and before `buildCoSigned`, on the deadline check's own
	// stated reasoning: refusing after the local signature is applied leaves the user signed into
	// something this build has just refused, and a signature cannot be taken back off a document.
	//
	// **Guarded on `cer != nil`, and the suite caught its absence.** `checkArrival` calls
	// `CheckRecord` before it touches its receiver, so a nil `*ceremonyID` does not panic — it
	// refuses with "document carries no ceremony record", which is every ordinary two-party
	// co-sign. `TestSessionInitiate` went red on exactly that. The receiving side has always had
	// this guard (`if sc.cer != nil`); this door needed the same one.
	if cer != nil {
		if err := cer.checkArrival(pdfBytes, time.Now()); err != nil {
			httpError(w, http.StatusConflict, err.Error())
			return
		}
	}
	carrying := cer.carries(hex.EncodeToString(myFP))
	signed := pdfBytes
	if !carrying {
		var ok bool
		signed, ok = s.buildCoSigned(w, pdfBytes, cert, key, att, appearance, cer.l3Roster())
		if !ok {
			return
		}
	}
	// One exchange verb, chosen once, so the two dial paths below cannot drift into disagreeing
	// about which one this hop is.
	exchange := func(ch p2p.Channel) ([]byte, error) {
		if carrying {
			return p2p.Carry(ch, signed, myFP, sessionVerifier{s, nil}, cer.l3Roster())
		}
		return p2p.Initiate(ch, signed, myFP, sessionVerifier{s, nil}, cer.l3Roster())
	}
	// ── The dialing side ANNOUNCES before it browses (P07.S05c, T01) ────────────────────────
	//
	// **The clause said "the LAN tier is re-announced when the convener's dial for hop k begins",
	// and no such mechanism can exist**: `startAnnouncing` runs on the armed side only and
	// `browsePeers` only listens, so a dialer has nothing it can send that makes a remote peer
	// speak. What CAN exist is the inverse, and it is cheaper than every beacon: **listening is
	// free**. An armed party holds a browse open for its ceremony's life at zero egress; the party
	// that knows a hop is starting sends one bounded burst. This is that burst.
	//
	// It is the convener's ORDINARY announcement — no new message and no format version. A
	// ceremony dialler already holds a listening endpoint (the glare join arms one on the shared
	// socket below), so it has something truthful to announce, which is exactly what an earlier
	// design for a "seek" datagram was working around.
	//
	// **Order is the whole of it, and it is why the handshake listener is armed HERE rather than
	// after the browse.** `peerAddresses` browses for two seconds and closes (`lan.go:234`,
	// `discover.go:21`) — an answer that arrives before that socket opens is lost, and an
	// announcement made after it closes is heard by nobody. So: arm, announce, browse. Arming
	// first also means a peer that answers by DIALLING us finds the endpoint already listening
	// rather than a socket that will exist in two seconds.
	//
	// Bounded by this request rather than by `lanAnnounceWindow`: the burst exists to be heard
	// during one hop's browse, and outliving the hop would put it back in the standing-beacon
	// class this whole design refuses.
	var hl *p2p.HandshakeListener
	if cer != nil && cer.rz != nil {
		var herr error
		if hl, herr = p2p.QUICListenHandshakeOn(cer.end, cert, key, peerFP); herr != nil {
			httpError(w, http.StatusInternalServerError, "could not arm the racing accept: "+herr.Error())
			return
		}
		defer hl.Close()
		// Never fatal, exactly as on the arm side: a host with no usable interface, or a loopback
		// bind, still races the DHT and its own accept. `startAnnouncing` refuses a loopback bind
		// BY NAME, which is why this is silent on the tier-4 harness and live in the namespace.
		if ann, aerr := startAnnouncing(cert, quicEndpointAnnounce{hl.Addr()}, hopAnnounceWindow); aerr == nil {
			defer ann.Close()
		}
	}
	cands, ok := s.peerAddresses(w, v, address, r.FormValue("transport"), peerFP)
	if !ok {
		return
	}
	// P05.S09: for a CEREMONY the dialing side also LISTENS, over the one shared endpoint, and the
	// glare join keeps whichever connection both ends agree on — so a peer that wins by dialing US
	// is joined here instead of being refused. The dialing side of a co-sign is the INITIATOR
	// (role-from-endpoint, the C6 default; the record-role refinement for multi-hop is T06), so it
	// promotes the surviving channel as the initiator and runs Initiate below. Outside a ceremony
	// this is raceWithRendezvous + Initiate exactly as before.
	// **The glare join is QUIC-ONLY, so a hop that is not QUIC must not take it (P07.S05b).**
	//
	// `connect` feeds its dial race through `filterQUIC` — *"the shared endpoint speaks QUIC, and
	// a non-QUIC candidate cannot be handshake-dialled on it"* — and `dialerCeremony` opens that
	// endpoint for EVERY ceremony, unconditionally. So the branch below was taken for every hop of
	// every ceremony, and a TCP candidate was filtered out of all of them: a ceremony dialled over
	// TCP raced an empty candidate set and spun until `connectDeadline`, five minutes, with the
	// receiver armed and idle the whole time.
	//
	// **It had never been driven.** The tier-4 N>=3 probe passes `transport=tcp` with an invitation
	// and is refused 409 by L3 at the near end, before any network work; every other ceremony test
	// is in-process with a hand-built channel; and the two-party tier-4 runs carry no invitation,
	// so `cer` is nil and they take the else branch. Found by P07.S05b's relay driver, whose first
	// TCP hop hung.
	//
	// `raceWithRendezvous` is the else branch and it already handles a ceremony correctly — it
	// dials each candidate on that candidate's own transport (`ceremonynet.go:534`) out of the
	// shared endpoint. What it does not do is the symmetric glare join, which is exactly right:
	// there is no shared endpoint on TCP to join over (`ceremonyid.go:240`, "QUIC only, and the
	// limit is structural rather than an omission").
	//
	// **The residual gap, stated rather than left to be discovered:** when no transport is named,
	// the glare branch still runs and a TCP-only candidate learned from the LAN or the DHT is
	// still dropped. That needs the racer to run both kinds side by side, which is a change to
	// P05's coordinator rather than to this route. Filed.
	// **ONE predicate, named once, and DECIDED AFTER THE BROWSE (P07.S05c).**
	//
	// The glare path races QUIC candidates only, so taking it when there is no QUIC candidate to
	// race is the P07.S05b defect: an empty race that spins until `connectDeadline`. S05b fixed
	// the case where the request NAMES a transport; this fixes the case that matters on a link,
	// where nothing is named and the transport arrives in the announcement (ADR-010). The
	// candidates are in hand by now — `peerAddresses` has already browsed — so the question is
	// decidable rather than a guess from the request.
	//
	// Measured: a four-party LAN relay over TCP. Nothing is typed, so the old predicate saw an
	// empty transport, took the glare path, and dropped the only candidate there was.
	//
	// **ALL, not ANY** — see `allQUICCandidates`. The glare path discards everything it cannot
	// dial, so a mixed set must go to the path that can dial every member. Handling both kinds
	// side by side is `/pending 298` and belongs to P05's coordinator.
	glare := hl != nil && (r.FormValue("transport") == transportQUIC ||
		(r.FormValue("transport") == "" && allQUICCandidates(cands)))
	var final []byte
	if glare {
		// P05.S10: re-race a LOST channel, re-sending the UNCHANGED signed document (Initiate never
		// re-signs its own contribution), so the peer re-delivers its cached signature idempotently
		// instead of co-signing twice. Bounded by connectDeadline; a decided outcome (decline, MITM,
		// consent timeout) breaks out and is reported below, never retried.
		deadline := time.Now().Add(connectDeadline)
		for {
			cctx, ccancel := context.WithDeadline(context.Background(), deadline)
			conn, cerr := s.connect(cctx, cer, hl, cands, cert, key, peerFP, myFP, true, peerLabel, peerLabel)
			if cerr != nil {
				ccancel()
				err = cerr
				break
			}
			final, err = exchange(conn.Channel)
			conn.Close()
			ccancel()
			if err == nil || !isTransportLoss(err) || !time.Now().Before(deadline) {
				break
			}
			// the channel dropped with time left — reconnect and let the peer re-deliver
		}
	} else {
		conn, cerr := s.raceWithRendezvous(cer, cands, cert, key, peerFP, peerLabel, peerLabel)
		if cerr != nil {
			err = cerr
		} else {
			final, err = exchange(conn.Channel)
			conn.Close()
		}
	}
	if errors.Is(err, errUnknownTransport) {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		// **A refusal is not a fault, and 502 says it is.** These two arrive as sentinels
		// now rather than as the EOF a silent close used to produce, so they are reported
		// as what happened rather than as a failed request the user should retry.
		if errors.Is(err, p2p.ErrCoSignDeclined) {
			httpError(w, http.StatusConflict, "the other party declined to co-sign this document")
			return
		}
		if errors.Is(err, p2p.ErrConsentTimedOut) {
			httpError(w, http.StatusConflict, "nobody answered on the other machine — "+
				"the request was shown and went unanswered, so nothing was signed")
			return
		}
		// A words-don't-match verdict is the man-in-the-middle signal, not a connect failure —
		// verify.go's own doc says it "must never be reported as a network error … 'could not
		// connect' invites a retry, which is the worst possible advice when someone is sitting
		// between you." The receive side already ends the ceremony on it (runCeremonyReceive's
		// default arm); the initiate side is the one synchronous caller a user waits on, so it is
		// where the sentence gets written. Lifted BEFORE writeConnectDiagnosis, which would
		// otherwise render this as a 502 and — worse — pick an unrelated D19 cause.
		if errors.Is(err, p2p.ErrVerificationDeclined) {
			httpError(w, http.StatusConflict, "the safety words did not match — this can mean "+
				"a machine is sitting between you and the other party, so nothing was signed. "+
				"Do not retry blindly: check the four words with them over a channel you trust first.")
			return
		}
		if errors.Is(err, p2p.ErrVerificationTimedOut) {
			httpError(w, http.StatusConflict, "the safety words went unconfirmed on the other "+
				"side in time, so nothing was signed")
			return
		}
		// **A contribution refusal is not a connect failure either (P07.S03b).** The three
		// branches above lift the refusals that had sentinels; P07.S03a gave sentinels to nine
		// more and they fell straight through to here, where `writeConnectDiagnosis` renders a
		// 502 wrapped in "could not connect to peer" AND picks a D19 network cause — for an
		// exchange in which the peer connected perfectly well and refused. Measured at tier 4:
		// `{"error":"could not connect to peer: a co-signature takes exactly one prior signer"}`,
		// which is the wire fix undone one layer up.
		//
		// The message is the refusal's own, because those sentences already name the party, the
		// position and the axis — a re-write here would be a second copy of them drifting.
		if p2p.IsContributionRefusal(err) {
			httpError(w, http.StatusConflict, err.Error())
			return
		}
		// P05.S11: a genuine connect failure (not a decline) gets D19's plain-language diagnosis,
		// classified from the signals on `cer`. `cer` is nil-safe in diagnose(); a non-ceremony or
		// TCP dial falls back to the flat connectFailure message.
		s.writeConnectDiagnosis(w, cer, err)
		return
	}
	// **A relay REPLACES the baton; an ordinary arrival still adds (P07.S05).**
	//
	// D10's rule — an arrival adds — is about a document that arrives out of the blue, and it is
	// unchanged for the manual and two-party paths. A ceremony hop is not that: each hop of an
	// N-party relay returned the SAME proceeding one signature further on, and every one of them
	// opened a new document, so a nine-party ceremony left the convener holding nine copies
	// against a count cap of eight. `installCeremonyResult` replaces by ceremony id and states
	// why it is a fourth commit door rather than one of the three.
	installed, ierr := s.installCeremonyResult(ceremonyIDOf(cer), arrivalDocName(peerLabel), final)
	if wroteCommitFailure(w, ierr) {
		return
	}
	// C22: **before the response returns**, not after. A caller that mirrored afterwards would
	// have told the user the hop completed and then written the record — so a crash in between
	// leaves a user who was told their signature is safe and a machine with no copy of it.
	s.mirrorHop(final)
	writeJSON(w, s.docResponse(installed))
}

// sendResult reports the outcome of a one-way send: Sent on a confirmed receipt,
// Declined when the peer's user declined. A transport failure is an HTTP error.
type sendResult struct {
	Sent     bool `json:"sent"`
	Declined bool `json:"declined,omitempty"`
	// TimedOut is "nobody answered", reported separately from Declined because it is a
	// different fact about a person and the user watching this send can act on it —
	// ring them, wait, try later — where a decline is final.
	TimedOut bool `json:"timedOut,omitempty"`
}

// handleSessionSend runs the dialing side of a one-way transfer: it dials the chosen
// pinned peer (who must be armed to receive) at the supplied address over pinned-peer
// mTLS and hands them the posted document — nothing is signed and nothing comes back.
// Like initiate, the mTLS handshake aborts before any bytes flow unless the address
// answers with the pinned peer's identity, and the endpoint is reachable only behind
// requireUnlocked (unlocked + CSRF + loopback origin).
func (s *Server) handleSessionSend(w http.ResponseWriter, r *http.Request) {
	v := vaultFrom(r)
	cleanup, ok := parseMultipart(w, r, maxPDFBytes)
	if !ok {
		return
	}
	defer cleanup()
	// An empty address is the LAN path, not a mistake. It is resolved further down,
	// AFTER the peer's fingerprint is parsed and its pin confirmed: a browse needs the
	// pin to match against, and asking the link about a peer this user has not pinned
	// would be discovery choosing who to talk to, which is what L1 forbids.
	address := r.FormValue("address")
	peerFP, err := parseFingerprint(r.FormValue("fingerprint"))
	if err != nil {
		httpError(w, http.StatusBadRequest, "not a valid fingerprint")
		return
	}
	if _, ok := pinnedLabel(v, peerFP); !ok {
		httpError(w, http.StatusBadRequest, "that peer isn't pinned — pin their fingerprint first")
		return
	}
	pdfBytes, ok := formFileBytes(w, r, "pdf")
	if !ok {
		return
	}
	cert, key, err := identity(v)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "could not load identity")
		return
	}
	myFP, err := sign.Fingerprint(cert)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "could not load identity")
		return
	}
	cands, ok := s.peerAddresses(w, v, address, r.FormValue("transport"), peerFP)
	if !ok {
		return
	}
	conn, err := dialAny(cands, cert, key, peerFP)
	if errors.Is(err, errUnknownTransport) {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		httpError(w, http.StatusBadGateway, connectFailure(err))
		return
	}
	defer conn.Close()
	if err := p2p.SendDocument(conn.Channel, pdfBytes, myFP, sessionVerifier{s, nil}); err != nil {
		if errors.Is(err, p2p.ErrDeclined) {
			writeJSON(w, sendResult{Sent: false, Declined: true})
			return
		}
		if errors.Is(err, p2p.ErrConsentTimedOut) {
			writeJSON(w, sendResult{Sent: false, TimedOut: true})
			return
		}
		httpError(w, http.StatusBadGateway, "send did not complete: "+err.Error())
		return
	}
	writeJSON(w, sendResult{Sent: true})
}

// DisarmSession tears down any armed listener; called on process shutdown.
func (s *Server) DisarmSession() { s.sess.disarm() }

// readJSON decodes a JSON request body, capped (the appearance image rides in it).
func readJSON(r *http.Request, v any) error {
	return json.NewDecoder(io.LimitReader(r.Body, maxPDFBytes)).Decode(v)
}

// transportQUIC is the only value that selects the QUIC path. Everything else — the
// empty string above all — is TCP, so every caller that predates D14 keeps working.
//
// The choice is deliberately not in the interface. D8's connection ladder attempts
// every tier concurrently and picks the first that completes, which means the user
// should never be choosing a transport at all; a toggle shipped now would be a control
// the ladder exists to remove. It is selectable over the API because that is what the
// multi-instance harness drives.
const (
	// Aliases of internal/p2p's, not copies. The string that selects a dialer, the
	// string a listener reports and the string this package compares against a
	// request must be one value or they drift (ADR-009).
	transportTCP  = p2p.TransportTCP
	transportQUIC = p2p.TransportQUIC
)

// errUnknownTransport names what was asked for and what exists.
var errUnknownTransport = errors.New(`transport must be "tcp" or "quic"`)

// checkTransport refuses a value it does not recognise instead of falling back.
//
// The empty string is TCP, because every caller that predates D14 sends nothing and
// must keep working. Anything else non-empty is an ERROR, and that is the part worth
// arguing: silently treating "QUIC" or a typo as TCP produces the two failures that
// are hardest to diagnose. Either the two sides disagree — one arms a TCP listener,
// the other dials QUIC at the same port, and the user is shown "could not connect to
// peer" for what is a spelling mistake — or they agree on the fallback and the user
// believes they are on a transport they are not.
func checkTransport(transport string) error {
	switch transport {
	case "", transportTCP, transportQUIC:
		return nil
	default:
		return fmt.Errorf("%w, not %q", errUnknownTransport, transport)
	}
}

// connectFailure turns a dial error into the sentence D19 asks for.
//
// **Without this, cause 5 arrives wearing cause 4's headline AND a false claim.** The dial
// error is wrapped by dialAny as "tried N address(es), none answered as the pinned peer",
// which is exactly right when nobody answered as the pinned peer — and exactly wrong when
// the peer WAS the pinned peer and the two machines disagree about the time. The user was
// told to check an address that is correct, about a peer that is right, while the ten-second
// fix went unmentioned inside the wrapper.
//
// D19's fifth cause is the only one naming a fix a user can perform immediately, and P05's
// criterion requires it "and never cause 4". So it is lifted out of the wrapping rather than
// concatenated onto it.
func connectFailure(err error) string {
	var skew *p2p.ClockSkewError
	if errors.As(err, &skew) {
		return skew.Error()
	}
	// **And a refusal is not a connect failure either (P07.S03b).** The handler lifts these
	// before it gets here, and this is the SECOND door: `writeConnectDiagnosis` also reaches this
	// function from `diagnosis.go`, so a rule enforced only at the caller holds at one of two
	// sites — the ADR-009 shape. Same argument as the skew above it: the peer connected, and
	// "could not connect" invites the retry that is wrong advice for every one of these.
	if p2p.IsContributionRefusal(err) {
		return err.Error()
	}
	return "could not connect to peer: " + err.Error()
}

// dialPeerWithin dials one candidate with the timeout the caller names, on the transport the
// candidate carries (ADR-010), under the caller's context (P05.S03).
//
// Every caller passes `lanDialTimeout`. **It used to say something else** — that this was the
// short budget beside a 30 s `sessionDialTimeout` "sized for an address a user TYPED" — and that
// was false for as long as it stood: `sessionDialTimeout` was reachable only through `dialPeer`,
// which had no callers at all. Both are deleted (P05.S03); the per-dial floor is
// `lanDialTimeout` and the whole-race budget is `connectDeadline`.
func dialPeerWithin(ctx context.Context, transport, address string, cert, key, peerFP []byte, timeout time.Duration, end *p2p.SharedEndpoint) (*p2p.Conn, error) {
	if err := checkTransport(transport); err != nil {
		return nil, err
	}
	if transport == transportQUIC {
		// In a ceremony (end != nil) every QUIC dial goes out the ONE shared endpoint (caveat
		// 7, S08): the source port must be the one the peer learned from our published mapping,
		// and one transport is what lets the punch (S08b) open a hole the dial actually uses.
		// Outside a ceremony — a typed address, dialAny — a fresh socket is correct and cheaper.
		if end != nil {
			return p2p.QUICDialOn(ctx, end, address, cert, key, peerFP, timeout)
		}
		return p2p.QUICDial(ctx, address, cert, key, peerFP, timeout)
	}
	// TCP always dials fresh: there is no shared TCP endpoint (SharedEndpoint is UDP/QUIC only).
	return p2p.Dial(ctx, address, cert, key, peerFP, timeout)
}

func listenPeer(transport, bind string, cert, key, peerFP []byte) (p2p.Listener, error) {
	if err := checkTransport(transport); err != nil {
		return nil, err
	}
	if transport == transportQUIC {
		return p2p.QUICListen(bind, cert, key, peerFP)
	}
	return p2p.Listen(bind, cert, key, peerFP)
}
