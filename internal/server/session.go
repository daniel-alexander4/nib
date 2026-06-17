package server

import (
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"nib/internal/p2p"
	"nib/internal/pdfops"
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

// session is the receive side of a live session: an armed, routable, pinned-peer-only
// listener opened on explicit request and torn down after one use. It serves two
// modes — co-signing (the peer's signed doc comes back doubly-signed) and a plain
// one-way document transfer (the peer's doc is consented and saved to ~/nib). All
// state is guarded by mu; it is independent of the Server's document lock.
type session struct {
	mu       sync.Mutex
	ln       net.Listener  // non-nil while armed
	addr     string        // bound address, reported in status
	pending  *pendingReq   // set while a received request awaits the user's consent
	received *receivedInfo // last accepted transfer, read by the poller after disarm
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

func (se *session) arm(ln net.Listener) bool {
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
	return se.pending.view.Fingerprint
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
	view := pendingView{Signer: peer.Signer, Fingerprint: peer.Fingerprint, AcceptedPeer: peer.AcceptedPeer, Reason: peer.Reason, Valid: peer.Valid}
	if !sc.s.sess.setPending(&pendingReq{view: view, doc: doc, resp: ch}) {
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

// sessionAccepter is the consent bridge for a plain one-way transfer: p2p.ReceiveDocument
// calls it after a peer sends a document; it surfaces the document for review, parks the
// request for the UI to accept/decline, and blocks until the user responds (or the
// timeout declines). label is this user's pinned label for the sending peer.
type sessionAccepter struct {
	s     *Server
	label string
}

func (sa sessionAccepter) Accept(peerFP, doc []byte) (bool, error) {
	ch := make(chan sessionDecision, 1)
	view := pendingView{Signer: sa.label, Fingerprint: hex.EncodeToString(peerFP), Reason: transferReason(doc), Valid: true}
	if !sa.s.sess.setPending(&pendingReq{view: view, doc: doc, resp: ch}) {
		return false, errors.New("session not armed")
	}
	defer sa.s.sess.clearPending()
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
func (s *Server) runSession(ln net.Listener, cert, key []byte, label, mode string) {
	// This goroutine handles a pinned peer's inbound document; a panic in the p2p or
	// sign code must not crash the desktop process. The defers below (disarm, Close)
	// still run as the stack unwinds.
	defer safe.Recover("session")
	timer := time.AfterFunc(sessionAcceptTimeout, s.sess.disarm)
	conn, err := ln.Accept()
	timer.Stop()
	if err != nil {
		s.sess.disarm() // listener closed by timeout/disarm, or a real error
		return
	}
	defer s.sess.disarm()
	defer conn.Close()
	if mode == sessionModeReceive {
		doc, peerFP, err := p2p.ReceiveDocument(conn.(*tls.Conn), sessionAccepter{s: s, label: label})
		if err != nil {
			return // declined, timed out, or a protocol error — nothing saved
		}
		s.saveReceived(doc, peerFP, label)
		return
	}
	final, err := p2p.Receive(conn.(*tls.Conn), cert, key, label, sessionConfirmer{s})
	if err != nil {
		return // declined, timed out, or a protocol error — nothing to apply
	}
	s.setDoc(&document{data: final, sig: sign.Verify(final)})
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

// sessionModeReceive arms the listener to accept a one-way document transfer (save to
// ~/nib); any other mode value co-signs.
const sessionModeReceive = "receive"

type armRequest struct {
	Fingerprint string `json:"fingerprint"`    // the single peer to accept (hex SPKI)
	Bind        string `json:"bind"`           // host:port to bind, e.g. "0.0.0.0:8443"
	Mode        string `json:"mode,omitempty"` // "receive" for a transfer; co-sign otherwise
}

type sessionStatus struct {
	Armed    bool          `json:"armed"`
	Address  string        `json:"address,omitempty"`
	Pending  *pendingView  `json:"pending,omitempty"`
	Received *receivedInfo `json:"received,omitempty"`
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
	if err := r.ParseMultipartForm(maxPDFBytes); err != nil {
		httpError(w, http.StatusBadRequest, "could not read upload")
		return
	}
	address := r.FormValue("address")
	if address == "" {
		httpError(w, http.StatusBadRequest, "a peer address is required")
		return
	}
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
	conn, err := p2p.Dial(address, cert, key, peerFP, sessionDialTimeout)
	if err != nil {
		httpError(w, http.StatusBadGateway, "could not connect to peer: "+err.Error())
		return
	}
	defer conn.Close()
	if err := p2p.SendDocument(conn, pdfBytes); err != nil {
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
