package server

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	"nib/internal/instance"
)

// TestTheProbeRouteProvesItHoldsTheToken — /pending 827. The launch's probe believes only an
// answer that proves the record's token, so the route has to give one: this drives the real
// client against the real route, which is the pair that must agree.
func TestTheProbeRouteProvesItHoldsTheToken(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	srv := New(os.DirFS("."), os.DirFS("."), t.TempDir(), "test-version")
	srv.SetInstanceToken("probe-token")
	ts := serveTest(t, srv)
	t.Cleanup(ts.Close)
	addr := strings.TrimPrefix(ts.URL, "http://")

	if got := instance.Probe(instance.Record{Addr: addr, Token: "probe-token", Challenge: true}); got != instance.Alive {
		t.Fatalf("a challenged probe of the running instance answered %v, want alive — every launch would read a live Nib as stale and start a second one", got)
	}
	if got := instance.Probe(instance.Record{Addr: addr, Token: "another-token", Challenge: true}); got != instance.Gone {
		t.Errorf("a challenged probe holding the wrong token answered %v, want gone", got)
	}

	// A caller that sends a nonce but has not proved the token is told nothing, and in
	// particular is not handed a proof: sending the token itself beside a nonce is refused.
	c := newClient(t)
	for name, presented := range map[string]string{
		"the token itself": "probe-token",
		"nothing":          "",
		"the answer proof": instance.AnswerProof("probe-token", "n-1"),
	} {
		req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/instance", nil)
		req.Header.Set(instance.HeaderNonce, "n-1")
		if presented != "" {
			req.Header.Set(instance.HeaderToken, presented)
		}
		resp, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		var out map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&out)
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden || out["proof"] != nil {
			t.Errorf("a challenge presenting %s answered %d with proof %v, want 403 and none", name, resp.StatusCode, out["proof"])
		}
	}
}
