package server

import (
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"nib/internal/p2p"
	"nib/internal/safe"
	"nib/internal/sign"
)

// TRIPWIRE: the armed session listener is Nib's ONLY network-reachable surface —
// every other listener binds loopback (cmd/nib/main.go, the loopbackOnly guard in
// server.go). It is opened only by an explicit, vault-unlocked /api/session/arm,
// binds the address the caller chooses, accepts only the one pinned peer (pinned-
// peer mTLS, internal/p2p/transport.go), signs only with explicit per-document user
// consent, and is torn down after one session or on timeout / disarm / shutdown.
// Keep that containment intact: do not widen what arms it, how long it stays open,
// or which peers it accepts without a fresh security review (P2P 12).

const (
	sessionAcceptTimeout  = 5 * time.Minute  // auto-disarm if no peer connects
	sessionConsentTimeout = 5 * time.Minute  // decline if the user never responds
	sessionDialTimeout    = 30 * time.Second // give up establishing the outbound connection
)

// session is the receive side of a live co-signing session: an armed, routable,
// pinned-peer-only listener opened on explicit request and torn down after one use.
// All state is guarded by mu; it is independent of the Server's document lock.
type session struct {
	mu      sync.Mutex
	ln      net.Listener // non-nil while armed
	addr    string       // bound address, reported in status
	pending *pendingReq  // set while a received request awaits the user's consent
}

// pendingReq is a received co-sign request blocked on the user's accept/decline.
type pendingReq struct {
	peer p2p.SignerAttestation
	doc  []byte // the received document, served for review via /api/session/pending-pdf
	resp chan sessionDecision
}

type sessionDecision struct {
	accept     bool
	intent     string
	appearance []byte
}

func (se *session) arm(ln net.Listener) bool {
	se.mu.Lock()
	defer se.mu.Unlock()
	if se.ln != nil {
		return false
	}
	se.ln = ln
	se.addr = ln.Addr().String()
	return true
}

// disarm closes the listener and declines any in-flight consent. Idempotent, so it
// is safe to call from the accept goroutine's defer, a manual disarm, and shutdown.
func (se *session) disarm() {
	se.mu.Lock()
	ln, p := se.ln, se.pending
	se.ln, se.addr, se.pending = nil, "", nil
	se.mu.Unlock()
	if ln != nil {
		ln.Close()
	}
	if p != nil {
		select {
		case p.resp <- sessionDecision{accept: false}:
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

func (se *session) clearPending() {
	se.mu.Lock()
	se.pending = nil
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
	return se.pending.peer.Fingerprint
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
	st := sessionStatus{Armed: se.ln != nil, Address: se.addr}
	if se.pending != nil {
		p := se.pending.peer
		st.Pending = &pendingView{Signer: p.Signer, Fingerprint: p.Fingerprint, AcceptedPeer: p.AcceptedPeer, Reason: p.Reason, Valid: p.Valid}
	}
	return st
}

// sessionConfirmer is the consent bridge: p2p.Receive calls it after a peer sends a
// signed document; it surfaces the document for review, parks the request for the
// UI to accept/decline, and blocks until the user responds (or the timeout declines).
type sessionConfirmer struct{ s *Server }

func (sc sessionConfirmer) Confirm(peer p2p.SignerAttestation, doc []byte) (bool, string, []byte, error) {
	// Park the received document for review (served via /api/session/pending-pdf)
	// rather than replacing the open document — that only changes on accept, in
	// runSession. A declined or timed-out request leaves the open doc untouched.
	ch := make(chan sessionDecision, 1)
	if !sc.s.sess.setPending(&pendingReq{peer: peer, doc: doc, resp: ch}) {
		return false, "", nil, errors.New("session not armed")
	}
	defer sc.s.sess.clearPending()
	select {
	case d := <-ch:
		return d.accept, d.intent, d.appearance, nil
	case <-time.After(sessionConsentTimeout):
		return false, "", nil, nil
	}
}

// runSession accepts one pinned peer, co-signs with the user's consent, and makes
// the result the open document. It always disarms on exit — one session per arm.
func (s *Server) runSession(ln net.Listener, cert, key []byte, label string) {
	// This goroutine handles a pinned peer's inbound document and verifies it; a
	// panic in p2p.Receive or sign.Verify must not crash the desktop process. The
	// defers below (disarm, Close) still run as the stack unwinds.
	defer safe.Recover("co-sign session")
	timer := time.AfterFunc(sessionAcceptTimeout, s.sess.disarm)
	conn, err := ln.Accept()
	timer.Stop()
	if err != nil {
		s.sess.disarm() // listener closed by timeout/disarm, or a real error
		return
	}
	defer s.sess.disarm()
	defer conn.Close()
	final, err := p2p.Receive(conn.(*tls.Conn), cert, key, label, sessionConfirmer{s})
	if err != nil {
		return // declined, timed out, or a protocol error — nothing to apply
	}
	s.setDoc(&document{data: final, sig: sign.Verify(final)})
}

// --- HTTP handlers (all behind requireUnlocked: vault-unlocked, CSRF, loopback origin) ---

type armRequest struct {
	Fingerprint string `json:"fingerprint"` // the single peer to accept (hex SPKI)
	Bind        string `json:"bind"`        // host:port to bind, e.g. "0.0.0.0:8443"
}

type sessionStatus struct {
	Armed   bool         `json:"armed"`
	Address string       `json:"address,omitempty"`
	Pending *pendingView `json:"pending,omitempty"`
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
	if req.Bind == "" {
		httpError(w, http.StatusBadRequest, "a bind address is required (e.g. 0.0.0.0:8443)")
		return
	}
	cert, key, err := identity(v)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "could not load identity")
		return
	}
	ln, err := p2p.Listen(req.Bind, cert, key, peerFP)
	if err != nil {
		httpError(w, http.StatusBadRequest, "could not open listener: "+err.Error())
		return
	}
	if !s.sess.arm(ln) {
		ln.Close()
		httpError(w, http.StatusConflict, "a session is already armed")
		return
	}
	go s.runSession(ln, cert, key, label)
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
	writeJSON(w, s.docResponse())
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
	if err := r.ParseMultipartForm(maxPDFBytes); err != nil {
		httpError(w, http.StatusBadRequest, "could not read upload")
		return
	}
	address := r.FormValue("address")
	if address == "" {
		httpError(w, http.StatusBadRequest, "a peer address is required")
		return
	}
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
	conn, err := p2p.Dial(address, cert, key, peerFP, sessionDialTimeout)
	if err != nil {
		httpError(w, http.StatusBadGateway, "could not connect to peer: "+err.Error())
		return
	}
	defer conn.Close()
	final, err := p2p.Initiate(conn, signed, myFP)
	if err != nil {
		httpError(w, http.StatusBadGateway, "co-signing did not complete: "+err.Error())
		return
	}
	s.setDoc(&document{data: final, sig: sign.Verify(final)})
	writeJSON(w, s.docResponse())
}

// DisarmSession tears down any armed listener; called on process shutdown.
func (s *Server) DisarmSession() { s.sess.disarm() }

// readJSON decodes a JSON request body, capped (the appearance image rides in it).
func readJSON(r *http.Request, v any) error {
	return json.NewDecoder(io.LimitReader(r.Body, maxPDFBytes)).Decode(v)
}
