package instance

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// TestAProbeIsAliveOnlyWhenTheAnswerProvesTheToken — /pending 827. After a crash the record
// outlives the Nib and the kernel hands its port to whatever asks next. The probe sent the token
// and believed any 200, so that listener was "alive", and the launch then sent it the hand-off
// secret and the document's path.
func TestAProbeIsAliveOnlyWhenTheAnswerProvesTheToken(t *testing.T) {
	const tok = "the-recorded-token"

	var mu sync.Mutex
	var seen []string // every header value the squatter was sent
	mode := "bare"
	squatter := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		for _, vs := range r.Header {
			seen = append(seen, vs...)
		}
		switch mode {
		case "bare": // anything that says 200
			w.Write([]byte(`{"ok":true,"version":"9.9.9"}`))
		case "echo": // hands the probe's own proof back as the answer
			json.NewEncoder(w).Encode(map[string]any{"ok": true, "proof": r.Header.Get(HeaderToken)})
		case "nonce": // answers with something derived from the nonce alone
			json.NewEncoder(w).Encode(map[string]any{"ok": true, "proof": AnswerProof("", r.Header.Get(HeaderNonce))})
		}
	}))
	defer squatter.Close()
	addr := strings.TrimPrefix(squatter.URL, "http://")

	// The stimulus: this listener IS believed by the probe a pre-challenge record gets, so a
	// refusal below is the proof and not an unreachable server.
	if got := Probe(Record{Addr: addr, Token: tok}); got != Alive {
		t.Fatalf("setup: the squatter did not answer a plain probe (%v) — nothing below distinguishes anything", got)
	}
	mu.Lock()
	seen = nil
	mu.Unlock()

	rec := Record{Addr: addr, Token: tok, Challenge: true}
	for _, m := range []string{"bare", "echo", "nonce"} {
		mu.Lock()
		mode = m
		mu.Unlock()
		if got := Probe(rec); got != Gone {
			t.Errorf("squatter %q: the probe answered %v, want gone — a listener that does not hold the record's token was taken for the user's Nib, and is sent the document path next", m, got)
		}
	}

	// And the token itself never crossed: a listener handed it could compute the answer.
	mu.Lock()
	defer mu.Unlock()
	if len(seen) == 0 {
		t.Fatal("the squatter was never sent a request — the assertion below would pass over nothing")
	}
	for _, v := range seen {
		if strings.Contains(v, tok) {
			t.Errorf("a challenged probe sent the token itself (%q) to whatever holds the port", v)
		}
	}
}

// TestAChallengedProbeBelievesTheInstanceThatHoldsTheToken is the other half: the proof must be
// one a real instance can give, and a fresh nonce every time.
func TestAChallengedProbeBelievesTheInstanceThatHoldsTheToken(t *testing.T) {
	const tok = "the-recorded-token"
	var mu sync.Mutex
	nonces := map[string]bool{}
	live := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nonce := r.Header.Get(HeaderNonce)
		if nonce == "" || !TokenMatches(r.Header.Get(HeaderToken), AskProof(tok, nonce)) {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		mu.Lock()
		nonces[nonce] = true
		mu.Unlock()
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "proof": AnswerProof(tok, nonce)})
	}))
	defer live.Close()
	addr := strings.TrimPrefix(live.URL, "http://")

	for i := 0; i < 2; i++ {
		if got := Probe(Record{Addr: addr, Token: tok, Challenge: true}); got != Alive {
			t.Fatalf("the instance holding the token answered %v, want alive", got)
		}
	}
	if len(nonces) != 2 {
		t.Errorf("two probes used %d distinct nonces, want 2 — a repeated nonce makes one recorded answer good forever", len(nonces))
	}
	if got := Probe(Record{Addr: addr, Token: "another-token", Challenge: true}); got != Gone {
		t.Errorf("a probe holding the wrong token answered %v, want gone", got)
	}
	if AskProof(tok, "n") == AnswerProof(tok, "n") {
		t.Error("the proof a probe sends is the proof it waits for — any listener can hand it straight back")
	}
}
