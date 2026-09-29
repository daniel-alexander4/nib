package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"nib/internal/p2p"
)

// /pending 660: the interactive arm and the delivery arm park consent in ONE slot. These are the
// P06 phase-close probe's scenario (R3 #1) made permanent, one test per half of the fix.

// pendingIDForTest is the ID of the parked request, or "" — what a page reads from
// `/api/session/status` as `pending.id`, for tests that drive `respond` directly.
func (se *session) pendingIDForTest() string {
	se.mu.Lock()
	defer se.mu.Unlock()
	if se.pending == nil {
		return ""
	}
	return se.pending.view.ID
}

// pendingIDAt reads the parked request's ID the way the page does, through the status route.
func pendingIDAt(t *testing.T, c *http.Client, baseURL string) string {
	t.Helper()
	var st sessionStatus
	sessGet(t, c, baseURL+"/api/session/status", &st)
	if st.Pending == nil {
		t.Fatal("no pending request to answer")
	}
	return st.Pending.ID
}

// twoArmedCeremonies arms ceremony A on the interactive arm and ceremony B on the delivery arm,
// exactly the probe's shape.
func twoArmedCeremonies() (se *session, cerA, cerB *ceremonyID) {
	se = &session{}
	cerA, cerB = &ceremonyID{}, &ceremonyID{}
	se.arms[armInteractive] = &arm{kind: armInteractive, cer: cerA}
	se.arms[armDelivery] = &arm{kind: armDelivery, cer: cerB}
	return se, cerA, cerB
}

// Half 1: a second arm's request cannot displace the one on screen. The incumbent wins, as it
// does for `setVerify`, and the refusal reaches Confirm as `errConsentBusy` WITHOUT spending the
// arm — nothing was put in front of the user for it.
func TestASecondArmCannotDisplaceTheConsentOnScreen(t *testing.T) {
	se, cerA, cerB := twoArmedCeremonies()
	chA := make(chan sessionDecision, 1)
	pA := &pendingReq{view: pendingView{Fingerprint: "AAAA"}, doc: []byte("docA"), resp: chA}
	if err := se.setPending(consentAnchor{cer: cerA, kind: armInteractive}, pA); err != nil {
		t.Fatalf("setup: ceremony A's consent could not park: %v", err)
	}
	chB := make(chan sessionDecision, 1)
	pB := &pendingReq{view: pendingView{Fingerprint: "BBBB"}, doc: []byte("docB"), resp: chB}
	if err := se.setPending(consentAnchor{cer: cerB, kind: armDelivery}, pB); !errors.Is(err, errConsentBusy) {
		t.Errorf("ceremony B's resumed hop parked over ceremony A's consent while A was on screen "+
			"(err=%v); the user's Accept, typed for A, would sign B", err)
	}
	if fp := se.pendingFingerprint(); fp != "AAAA" {
		t.Fatalf("the request on screen is now %q, want A's", fp)
	}
	if got := se.respond(se.pendingIDForTest(), sessionDecision{accept: true}); got != respondDelivered {
		t.Fatalf("answering the request on screen = %v, want delivered", got)
	}
	if len(chA) != 1 || len(chB) != 0 {
		t.Errorf("the accept went to the wrong request: A got %d, B got %d", len(chA), len(chB))
	}

	// Through the real bridge: B's Confirm is refused busy and leaves its arm unspent.
	se2, cerA2, cerB2 := twoArmedCeremonies()
	if err := se2.setPending(consentAnchor{cer: cerA2, kind: armInteractive},
		&pendingReq{resp: make(chan sessionDecision, 1)}); err != nil {
		t.Fatalf("setup: %v", err)
	}
	srv := &Server{}
	srv.sess.arms = se2.arms
	srv.sess.pending = se2.pending
	var saw reached
	sc := sessionConfirmer{s: srv, saw: &saw, anchor: consentAnchor{cer: cerB2, kind: armDelivery}}
	_, _, _, _, err := sc.Confirm(p2p.SignerAttestation{}, []byte("not a pdf"))
	if !errors.Is(err, errConsentBusy) {
		t.Errorf("Confirm for B with A on screen returned %v, want errConsentBusy", err)
	}
	if saw.v.Load() {
		t.Error("a consent refused busy SPENT the arm — nothing was shown to the user for it")
	}
}

// Half 2: an answer names the request the page was shown. A clears and B parks between the page's
// last poll and the click; the Accept the page posts for A must not reach B.
func TestAnAnswerReachesOnlyTheRequestItNames(t *testing.T) {
	se, cerA, cerB := twoArmedCeremonies()
	pA := &pendingReq{view: pendingView{Fingerprint: "AAAA"}, resp: make(chan sessionDecision, 1)}
	if err := se.setPending(consentAnchor{cer: cerA, kind: armInteractive}, pA); err != nil {
		t.Fatalf("setup: %v", err)
	}
	idA := se.pendingIDForTest()
	if idA == "" {
		t.Fatal("a parked request carries no ID for the page to echo")
	}
	se.clearPendingIf(pA) // A's goroutine finished (timed out, or its peer went away)
	chB := make(chan sessionDecision, 1)
	pB := &pendingReq{view: pendingView{Fingerprint: "BBBB"}, resp: chB}
	if err := se.setPending(consentAnchor{cer: cerB, kind: armDelivery}, pB); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if se.pendingIDForTest() == idA {
		t.Fatal("two requests were given the same ID")
	}
	if got := se.respond(idA, sessionDecision{accept: true}); got != respondNotThatRequest {
		t.Errorf("an answer naming A was %v against B's request, want respondNotThatRequest", got)
	}
	if got := se.respond("", sessionDecision{accept: true}); got != respondNotThatRequest {
		t.Errorf("an answer naming no request was %v, want respondNotThatRequest", got)
	}
	if len(chB) != 0 {
		t.Fatal("the Accept the user gave for ceremony A signed ceremony B")
	}

	// And the route: no ID is 400, the wrong ID is 409, the right one is 200.
	srv := &Server{}
	srv.sess.arms = se.arms
	srv.sess.pending = se.pending
	post := func(body string) int {
		rr := httptest.NewRecorder()
		srv.handleSessionRespond(rr, httptest.NewRequest(http.MethodPost, "/api/session/respond",
			strings.NewReader(body)))
		return rr.Code
	}
	if code := post(`{"accept":true}`); code != http.StatusBadRequest {
		t.Errorf("an answer with no id = %d, want 400", code)
	}
	if code := post(`{"id":"` + idA + `","accept":true}`); code != http.StatusConflict {
		t.Errorf("an answer naming the request that went away = %d, want 409", code)
	}
	if len(chB) != 0 {
		t.Fatal("the route delivered an answer for A to B")
	}
	if code := post(`{"id":"` + srv.sess.pendingIDForTest() + `","accept":true}`); code != http.StatusOK {
		t.Errorf("an answer naming the request on screen = %d, want 200", code)
	}
	if len(chB) != 1 {
		t.Error("the answer naming B did not reach B")
	}
}

// /pending 744 (1): cancelling ONE arm releases the consent that arm parked, and only that one.
// The gates used to release only when nothing was left armed, so with the delivery arm still up,
// Cancel on the interactive arm left ceremony A's consent on screen and its goroutine waiting out
// the whole timeout for a session that no longer existed.
func TestCancellingOneArmReleasesOnlyTheConsentItParked(t *testing.T) {
	se, cerA, _ := twoArmedCeremonies()
	chA := make(chan sessionDecision, 1)
	if err := se.setPending(consentAnchor{cer: cerA, kind: armInteractive},
		&pendingReq{view: pendingView{Fingerprint: "AAAA"}, resp: chA}); err != nil {
		t.Fatalf("setup: A could not park: %v", err)
	}
	se.disarmKind(armInteractive)
	if se.arms[armDelivery] == nil {
		t.Fatal("setup: the delivery arm went too, so this is the nothing-left-armed case the old rule already covered")
	}
	if fp := se.pendingFingerprint(); fp != "" {
		t.Errorf("the interactive arm was cancelled and ITS consent (%q) is still parked, answerable, "+
			"while the delivery arm beside it stays up", fp)
	}
	select {
	case d := <-chA:
		if d.accept || !d.torn {
			t.Errorf("A's waiter was answered %+v, want a teardown refusal", d)
		}
	default:
		t.Error("A's waiter was not woken — its goroutine sits on the channel until the consent timeout")
	}

	// The mirror: the arm that goes is NOT the one that parked, so the request stays.
	se2, _, cerB := twoArmedCeremonies()
	chB := make(chan sessionDecision, 1)
	if err := se2.setPending(consentAnchor{cer: cerB, kind: armDelivery},
		&pendingReq{view: pendingView{Fingerprint: "BBBB"}, resp: chB}); err != nil {
		t.Fatalf("setup: B could not park: %v", err)
	}
	se2.disarmKind(armInteractive)
	if fp := se2.pendingFingerprint(); fp != "BBBB" {
		t.Errorf("cancelling the interactive arm dropped the delivery arm's consent (now %q) — "+
			"a live session's request abandoned while the user reads it", fp)
	}
	if len(chB) != 0 {
		t.Error("the delivery arm's waiter was answered by a teardown of the OTHER arm")
	}
}

// /pending 744 (2): the preview and the quote name the request they are for. A cleared and B
// parked between the page's poll and its fetch; the page, still showing A, must not be handed B's
// document or B's roster.
func TestThePreviewAndTheQuoteNameTheirRequest(t *testing.T) {
	se, cerA, cerB := twoArmedCeremonies()
	cerA.inv.Intent, cerB.inv.Intent = "ceremony A's recital", "ceremony B's recital"
	pA := &pendingReq{view: pendingView{Fingerprint: "AAAA"}, doc: []byte("docA"), resp: make(chan sessionDecision, 1)}
	if err := se.setPending(consentAnchor{cer: cerA, kind: armInteractive}, pA); err != nil {
		t.Fatalf("setup: %v", err)
	}
	idA := se.pendingIDForTest()
	se.clearPendingIf(pA)
	pB := &pendingReq{view: pendingView{Fingerprint: "BBBB"}, doc: []byte("docB"), resp: make(chan sessionDecision, 1)}
	if err := se.setPending(consentAnchor{cer: cerB, kind: armDelivery}, pB); err != nil {
		t.Fatalf("setup: %v", err)
	}
	idB := se.pendingIDForTest()

	// The roster is the PARKING arm's, not the first ceremony arm's — A's interactive slot comes first.
	if _, roster, out := se.pendingNamed(idB); out != respondDelivered || roster.Intent != "ceremony B's recital" {
		t.Errorf("B's request quoted with roster intent %q (outcome %v), want B's — the block would "+
			"carry another ceremony's recital", roster.Intent, out)
	}

	srv := &Server{}
	srv.sess.arms = se.arms
	srv.sess.pending = se.pending
	get := func(q string) (int, string) {
		rr := httptest.NewRecorder()
		srv.handleSessionPendingPDF(rr, httptest.NewRequest(http.MethodGet, "/api/session/pending-pdf"+q, nil))
		return rr.Code, rr.Body.String()
	}
	if code, body := get("?id=" + idA); code != http.StatusConflict || strings.Contains(body, "docB") {
		t.Errorf("the preview for A = %d %q, want 409 and not B's document", code, body)
	}
	if code, _ := get(""); code != http.StatusBadRequest {
		t.Errorf("a preview naming no request = %d, want 400", code)
	}
	if code, body := get("?id=" + idB); code != http.StatusOK || body != "docB" {
		t.Errorf("the preview for B = %d %q, want 200 docB", code, body)
	}
	quote := func(body string) int {
		rr := httptest.NewRecorder()
		srv.handleSessionQuote(rr, httptest.NewRequest(http.MethodPost, "/api/session/quote", strings.NewReader(body)))
		return rr.Code
	}
	if code := quote(`{"id":"` + idA + `","intent":"x"}`); code != http.StatusConflict {
		t.Errorf("a quote for A while B is parked = %d, want 409", code)
	}
	if code := quote(`{"intent":"x"}`); code != http.StatusBadRequest {
		t.Errorf("a quote naming no request = %d, want 400", code)
	}
}

// /pending 744 (4): a busy refusal crosses the wire BY NAME, so the dialler is told the machine is
// busy rather than seeing its session fail like a network fault.
func TestABusyConsentRefusalIsNamedForTheWire(t *testing.T) {
	if !errors.Is(errConsentBusy, p2p.ErrConsentBusy) {
		t.Fatal("errConsentBusy is not p2p.ErrConsentBusy to errors.Is, so refusalAck cannot name it")
	}
	if !p2p.IsContributionRefusal(errConsentBusy) {
		t.Error("a busy refusal is not a named refusal — it reaches the dialler as a bare EOF, read as a network fault")
	}
	if strings.Contains(errConsentBusy.Error(), "other machine") {
		t.Errorf("this machine's own sentence is written for the far end: %q", errConsentBusy)
	}
}

// /pending 750 (1): the spoken check goes with the arm that parked it, the way the consent after
// it does (/pending 744). Cancelling the interactive arm while the delivery arm stays up used to
// leave the interactive session's words on screen, answerable, with its goroutine waiting out the
// timeout — the check carried no anchor. The dialling side still has none and keeps the old rule.
func TestCancellingOneArmReleasesOnlyTheSpokenCheckItParked(t *testing.T) {
	se, cerA, _ := twoArmedCeremonies()
	chA := make(chan bool, 1)
	if err := se.setVerify(&pendingVerify{words: "a b c d", resp: chA,
		anchor: &consentAnchor{cer: cerA, kind: armInteractive}}); err != nil {
		t.Fatalf("setup: A could not park its check: %v", err)
	}
	se.disarmKind(armInteractive)
	if se.arms[armDelivery] == nil {
		t.Fatal("setup: the delivery arm went too, so this is the nothing-left-armed case")
	}
	if pv := se.currentVerify(); pv != nil {
		t.Errorf("the interactive arm was cancelled and ITS spoken check (%q) is still parked, "+
			"answerable, while the delivery arm beside it stays up", pv.words)
	}
	select {
	case ok := <-chA:
		if ok {
			t.Error("A's check was answered as a match by a teardown")
		}
	default:
		t.Error("A's waiter was not woken — its goroutine sits on the channel until the timeout")
	}

	// The mirror: the arm that goes is NOT the one that parked, so the check stays.
	se2, _, cerB := twoArmedCeremonies()
	chB := make(chan bool, 1)
	pvB := &pendingVerify{words: "e f g h", resp: chB, anchor: &consentAnchor{cer: cerB, kind: armDelivery}}
	if err := se2.setVerify(pvB); err != nil {
		t.Fatalf("setup: B could not park: %v", err)
	}
	se2.disarmKind(armInteractive)
	if se2.currentVerify() != pvB || len(chB) != 0 {
		t.Error("cancelling the interactive arm released the delivery arm's spoken check")
	}

	// The dialler's check has no arm: another arm's teardown leaves it while anything is armed.
	se3, _, _ := twoArmedCeremonies()
	pvD := &pendingVerify{words: "i j k l", resp: make(chan bool, 1)}
	if err := se3.setVerify(pvD); err != nil {
		t.Fatalf("setup: the dialler could not park: %v", err)
	}
	se3.disarmKind(armInteractive)
	if se3.currentVerify() != pvD {
		t.Error("the dialler's check, which names no arm, was released while an arm is still up")
	}
	se3.disarmKind(armDelivery)
	if se3.currentVerify() != nil {
		t.Error("the dialler's check survived the last arm going — the old rule is lost")
	}

	// And a check parked after its arm is gone is refused, or the release above has already run
	// and it waits out the whole timeout.
	se4, cerA4, _ := twoArmedCeremonies()
	se4.disarmKind(armInteractive)
	if err := se4.setVerify(&pendingVerify{words: "m n o p", resp: make(chan bool, 1),
		anchor: &consentAnchor{cer: cerA4, kind: armInteractive}}); !errors.Is(err, errConsentNotArmed) {
		t.Errorf("a check for a torn-down arm parked with %v, want errConsentNotArmed", err)
	}
}

// /pending 750 (2): an accepted request is SETTLED once its session is finished with it, and the
// page's applying stage ends on that rather than on the machine disarming — which a delivery arm
// beside the interactive one keeps from ever happening. Two halves: the door records the id of the
// request the user said yes to (and nothing for a no), and every `serveOneSession` caller settles
// it after its `openArrival`, so a page that sees it and asks what is active is shown the arrival.
func TestAnAcceptedRequestIsSettledAfterItsSession(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, accept := range []bool{true, false} {
		s := &Server{}
		ln := &stubListener{}
		if !s.sess.arm(ln, nil) {
			t.Fatal("setup: the session refused to arm")
		}
		answerWhenParked := func() chan string {
			idc := make(chan string, 1)
			go func() {
				for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(2 * time.Millisecond) {
					if id := s.sess.pendingIDForTest(); id != "" {
						idc <- id
						s.sess.respond(id, sessionDecision{accept: accept})
						return
					}
				}
				idc <- ""
			}()
			return idc
		}
		check := func(door string, saw *reached, id string) {
			t.Helper()
			if id == "" {
				t.Fatalf("setup: %s: the request never parked", door)
			}
			s.sess.settled = "" // each door judged on its own
			s.sess.settle(saw.answeredID())
			got := s.sess.status()
			switch {
			case accept && got.Settled != id:
				t.Errorf("%s: an accepted request settled %q, want its own id %q — the page waits on it "+
					"and stays on \"Saving…\" while another arm keeps the machine armed", door, got.Settled, id)
			case !accept && got.Settled != "":
				t.Errorf("%s: a DECLINED request settled %q; nothing was accepted, so nothing is applying", door, got.Settled)
			}
			if !got.Armed {
				t.Fatalf("setup: %s: the arm went, so this is the case a disarm already ended", door)
			}
		}

		var saw reached
		sa := sessionAccepter{s: s, label: "Bob", saw: &saw, anchor: consentAnchor{ln: ln}}
		idc := answerWhenParked()
		if _, err := sa.Accept([]byte("peer-fingerprint-bytes-0123456789"), []byte("%PDF-1.4\nx")); err != nil {
			t.Fatalf("accept=%t: %v", accept, err)
		}
		check("transfer", &saw, <-idc)

		var sawC reached
		sc := sessionConfirmer{s: s, saw: &sawC, anchor: consentAnchor{ln: ln}}
		idc = answerWhenParked()
		if _, _, _, _, err := sc.Confirm(p2p.SignerAttestation{}, []byte("not a pdf")); err != nil {
			t.Fatalf("co-sign accept=%t: %v", accept, err)
		}
		check("co-sign", &sawC, <-idc)
		s.sess.disarm()
	}

	src, err := os.ReadFile("session.go")
	if err != nil {
		t.Fatal(err)
	}
	dsrc, err := os.ReadFile("delivery.go")
	if err != nil {
		t.Fatal(err)
	}
	sites := 0
	for _, code := range []string{stripLineComments(string(src)), stripLineComments(string(dsrc))} {
		for rest := code; ; {
			i := strings.Index(rest, "s.serveOneSession(")
			if i < 0 {
				break
			}
			sites++
			after := rest[i:]
			if j := strings.Index(after, "s.sess.settle(answered)"); j < 0 || j > 700 {
				t.Errorf("a serveOneSession call site does not settle the request it accepted: %.120q", after)
			}
			rest = after[1:]
		}
	}
	// And serveOneSession hands its callers the id on EVERY return, not only the ones that remembered.
	code := stripLineComments(string(src))
	body := funcBodyFrom(code, strings.Index(code, "func (s *Server) serveOneSession("))
	if !strings.Contains(body, "defer func() { answered = saw.answeredID() }()") {
		t.Error("serveOneSession no longer returns the accepted id from a deferred read of its `reached`, " +
			"so some return path hands its caller \"\" and the page never leaves its applying stage")
	}
	if sites != 3 {
		t.Errorf("found %d serveOneSession call sites, want 3 — a new one must settle what it accepts", sites)
	}
}
