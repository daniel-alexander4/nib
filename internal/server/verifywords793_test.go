package server

import (
	"net/http"
	"strings"
	"testing"
)

// TestASpokenCheckAnswerIsForTheWordsItNames — /pending 793.
//
// The answer carried `confirmed` and nothing else, so it resolved whichever check was parked when
// the POST landed. Checks follow one another in the one slot: the first times out with its words
// still on the page, a second parks, and the press meant for the first confirmed the second —
// words nobody compared.
//
// Through the route, because the binding is a field the handler has to read and pass on; the two
// checks are parked directly, since reaching the state through real sessions needs two peers and
// a five-minute timeout to test one comparison.
func TestASpokenCheckAnswerIsForTheWordsItNames(t *testing.T) {
	ts, srv := startServerWith(t)
	c, csrf := authedClient(t, ts)
	verify := func(body map[string]any) (int, string) {
		t.Helper()
		return postForCode(t, c, csrf, ts.URL+"/api/session/verify", body)
	}

	// The first check goes up and goes away unanswered, as its timeout does it.
	first := &pendingVerify{words: "amber birch cobalt dune", resp: make(chan bool, 1)}
	if err := srv.sess.setVerify(first); err != nil {
		t.Fatalf("setup: the first check would not park: %v", err)
	}
	srv.sess.clearVerifyIf(first)
	second := &pendingVerify{words: "ember flint grove harbour", resp: make(chan bool, 1)}
	if err := srv.sess.setVerify(second); err != nil {
		t.Fatalf("setup: the second check would not park: %v", err)
	}
	defer srv.sess.clearVerifyIf(second)

	for _, confirmed := range []bool{true, false} {
		code, body := verify(map[string]any{"confirmed": confirmed, "words": first.words})
		if code != http.StatusConflict || !strings.Contains(body, "not the words") {
			t.Errorf("an answer (confirmed=%t) naming the FIRST check's words returned %d %q while the "+
				"second was waiting; want 409 saying those are not the words waiting", confirmed, code, body)
		}
		select {
		case got := <-second.resp:
			t.Fatalf("an answer given for %q resolved the check for %q as %t — a confirmation of "+
				"words the user never compared", first.words, second.words, got)
		default:
		}
	}
	if srv.sess.currentVerify() != second {
		t.Fatal("the refused answer took the waiting check down")
	}

	// The answer that names the waiting check's words is delivered.
	if code, body := verify(map[string]any{"confirmed": true, "words": second.words}); code != http.StatusOK {
		t.Fatalf("an answer naming the waiting check's own words returned %d: %s", code, body)
	}
	select {
	case got := <-second.resp:
		if !got {
			t.Error("a confirmation was delivered as a refusal")
		}
	default:
		t.Fatal("an answer naming the waiting check's own words was not delivered")
	}

	// The declared exemption: a caller with no page names no words and answers what is waiting.
	if code, body := verify(map[string]any{"confirmed": true}); code != http.StatusOK {
		t.Fatalf("an answer naming no words returned %d: %s", code, body)
	}
	select {
	case <-second.resp:
	default:
		t.Fatal("an answer naming no words was not delivered")
	}
}
