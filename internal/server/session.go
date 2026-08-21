package server

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"nib/internal/p2p"
	"nib/internal/pdfops"
	"nib/internal/safe"
	"nib/internal/sign"
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

func (se *session) arm(ln p2p.Listener) bool {
	se.mu.Lock()
	defer se.mu.Unlock()
	if se.ln != nil {
		return false
	}
	se.ln = ln
	se.addr = ln.Addr().String()
	se.received = nil // a fresh session clears any prior transfer result
	return true
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
	se.mu.Lock()
	if ln != nil && se.ln != ln {
		se.mu.Unlock()
		return // a later session is armed; this one is already over
	}
	cur, p, pv := se.ln, se.pending, se.verify
	se.ln, se.addr, se.pending, se.verify = nil, "", nil, nil
	se.mu.Unlock()
	ln = cur
	if ln != nil {
		ln.Close()
	}
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

func (se *session) setPending(p *pendingReq) bool {
	se.mu.Lock()
	defer se.mu.Unlock()
	if se.ln == nil {
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
	defer se.mu.Unlock()
	st := sessionStatus{Armed: se.ln != nil, Address: se.addr, Received: se.received}
	if se.pending != nil {
		pv := se.pending.view
		st.Pending = &pv
	}
	if se.verify != nil {
		st.Verify = &verifyView{Words: se.verify.words}
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
}

func (sc sessionConfirmer) Confirm(peer p2p.SignerAttestation, doc []byte) (bool, string, []byte, error) {
	sc.saw.mark() // the consent request is about to go on screen
	// Park the received document for review (served via /api/session/pending-pdf)
	// rather than replacing the open document — that only changes on accept, in
	// runSession. A declined or timed-out request leaves the open doc untouched.
	ch := make(chan sessionDecision, 1)
	view := pendingView{Signer: peer.Signer, Fingerprint: peer.Fingerprint, AcceptedPeer: peer.AcceptedPeer, Reason: peer.Reason, Valid: peer.Valid}
	// The request is held so the defer can name it: an unconditional clear drops whatever
	// is pending when it fires, which after a disarm-and-rearm is a LATER session's consent.
	req := &pendingReq{view: view, doc: doc, resp: ch}
	if !sc.s.sess.setPending(req) {
		return false, "", nil, errors.New("session not armed")
	}
	defer sc.s.sess.clearPendingIf(req)
	select {
	case d := <-ch:
		return d.accept, d.intent, d.appearance, nil
	case <-time.After(sessionConsentTimeout):
		return false, "", nil, nil
	}
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
	s     *Server
	label string
	saw   *reached
}

func (sa sessionAccepter) Accept(peerFP, doc []byte) (bool, error) {
	sa.saw.mark() // the transfer consent is about to go on screen
	ch := make(chan sessionDecision, 1)
	view := pendingView{Signer: sa.label, Fingerprint: hex.EncodeToString(peerFP), Reason: transferReason(doc), Valid: true}
	req := &pendingReq{view: view, doc: doc, resp: ch}
	if !sa.s.sess.setPending(req) {
		return false, errors.New("session not armed")
	}
	defer sa.s.sess.clearPendingIf(req)
	select {
	case d := <-ch:
		return d.accept, nil
	case <-time.After(sessionConsentTimeout):
		return false, nil
	}
}

// runSession accepts one pinned peer and, depending on the armed mode, either
// co-signs with the user's consent (making the result the open document) or accepts a
// one-way document transfer and saves it under ~/nib. It always disarms on exit — one
// session per arm.
// arrivalDocName names a co-signed document by who it came from, which is the one fact about
// it the user cannot see on the page.
func arrivalDocName(peerLabel string) string {
	if peerLabel == "" {
		return "co-signed.pdf"
	}
	return "co-signed with " + peerLabel + ".pdf"
}

func (s *Server) runSession(ln p2p.Listener, cert, key []byte, label, mode string) {
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
	if ann, err := startAnnouncing(cert, ln); err == nil {
		defer ann.Close()
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
	armedUntil := time.Now().Add(sessionAcceptTimeout)
	timer := time.AfterFunc(sessionAcceptTimeout, func() { s.sess.disarmIf(ln) })
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
		served := s.serveOneSession(conn, cert, key, label, mode, myFP)
		if served {
			return // the arm is spent on a session, which is what it is for
		}
		remaining := time.Until(armedUntil)
		if remaining <= 0 {
			return
		}
		timer.Reset(remaining)
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
func (s *Server) serveOneSession(conn *p2p.Conn, cert, key []byte, label, mode string, myFP []byte) bool {
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
		doc, err := p2p.ReceiveDocument(ch, sessionAccepter{s: s, label: label, saw: &saw}, myFP, sessionVerifier{s, &saw})
		if err != nil {
			return saw.v.Load()
		}
		s.saveReceived(doc, ch.PeerFP, label)
		return true
	}
	final, err := p2p.Receive(ch, cert, key, label, sessionConfirmer{s, &saw}, sessionVerifier{s, &saw})
	if err != nil {
		return saw.v.Load()
	}
	// D10: the co-signed document ARRIVES, so it opens alongside whatever the user
	// already had open rather than replacing it. Before this, completing a co-signature
	// on the receiving side silently discarded the recipient's open document and its
	// undo history — work they never asked to close.
	// Named, so an arrival is not "Untitled" after a reload — the fifth path-less producer.
	// handleDocs' own comment names "an arrival" in the population that needed this.
	s.addDoc(&document{name: arrivalDocName(label), data: final, sig: sign.Verify(final)})
	return true
}

// saveReceived writes an accepted one-way transfer under ~/nib, routed by what the
// document is: a flagged PDF (awaiting the user's signature) lands in to-sign/, an
// already-signed one in signed/, anything else in incoming/. Best-effort — a write
// failure leaves the user's other documents untouched and simply reports nothing.
func (s *Server) saveReceived(doc, peerFP []byte, peerLabel string) {
	path := filepath.Join(defaultOutputDir(), receivedSubdir(doc), receivedName(peerLabel, peerFP))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	if err := writeFileAtomic(path, doc); err != nil {
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
	Fingerprint string `json:"fingerprint"`         // the single peer to accept (hex SPKI)
	Bind        string `json:"bind"`                // host:port to bind, e.g. "0.0.0.0:8443"
	Mode        string `json:"mode,omitempty"`      // "receive" for a transfer; co-sign otherwise
	Transport   string `json:"transport,omitempty"` // "quic"; anything else is TCP
}

type sessionStatus struct {
	Armed    bool          `json:"armed"`
	Address  string        `json:"address,omitempty"`
	Verify   *verifyView   `json:"verify,omitempty"`
	Pending  *pendingView  `json:"pending,omitempty"`
	Received *receivedInfo `json:"received,omitempty"`
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
	ln, err := listenPeer(req.Transport, bind, cert, key, peerFP)
	if errors.Is(err, errUnknownTransport) {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		httpError(w, http.StatusBadRequest, "could not open listener: "+err.Error())
		return
	}
	if !s.sess.arm(ln) {
		ln.Close()
		httpError(w, http.StatusConflict, "a session is already armed")
		return
	}
	go s.runSession(ln, cert, key, label, req.Mode)
	writeJSON(w, s.sess.status())
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
	// as coSignExchange does on accept. The nominal rect mirrors stackPlacement's
	// constant block size (280×84 pt) — only its aspect is used, to size the PNG.
	att, ok := s.cosignAttestation(w, v, cosignParams{Fingerprint: fp, Intent: req.Intent})
	if !ok {
		return
	}
	writeJSON(w, cosignQuote{Lines: att.AppearanceLines(), Rect: [4]float64{40, 40, 320, 124}})
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
	signed, ok := s.buildCoSigned(w, pdfBytes, cert, key, att, appearance)
	if !ok {
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
	final, err := p2p.Initiate(conn.Channel, signed, myFP, sessionVerifier{s, nil})
	if err != nil {
		httpError(w, http.StatusBadGateway, "co-signing did not complete: "+err.Error())
		return
	}
	// D10: an arrival adds. Same reasoning as runSession's — the difference here is
	// only that a user is waiting on this response, so the document they get back is
	// the arrival, which addDoc has made active.
	peerLabel, _ := pinnedLabel(v, peerFP)
	installed := s.addDoc(&document{name: arrivalDocName(peerLabel), data: final, sig: sign.Verify(final)})
	writeJSON(w, s.docResponse(installed))
}

// sendResult reports the outcome of a one-way send: Sent on a confirmed receipt,
// Declined when the peer's user declined. A transport failure is an HTTP error.
type sendResult struct {
	Sent     bool `json:"sent"`
	Declined bool `json:"declined,omitempty"`
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
func dialPeerWithin(ctx context.Context, transport, address string, cert, key, peerFP []byte, timeout time.Duration) (*p2p.Conn, error) {
	if err := checkTransport(transport); err != nil {
		return nil, err
	}
	if transport == transportQUIC {
		return p2p.QUICDial(ctx, address, cert, key, peerFP, timeout)
	}
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
