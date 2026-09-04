package server

import (
	"encoding/json"
	"net/http"
	"testing"

	"nib/internal/discovery"
)

// /pending 23 — the discovery counters get a surface that is not a terminal.
//
// **The counters had a reader and it was `nib discover`.** Nib's primary user is non-technical,
// on one machine, with no IT, and a LAN ceremony that fails is silent by nature. D19 cannot cover
// it either: `diagnose()` returns `causeUndiagnosed` for a LAN or TCP ceremony by construction,
// and the status path publishes nothing for an undiagnosed cause — so the armed screen has no
// sentence at all for this failure.
func TestTheNetworkTestAnswersWithAVerdictAndItsCounters(t *testing.T) {
	ts, _ := startServer(t)
	c, _ := authedClient(t, ts)

	r, err := c.Get(ts.URL + "/api/lan/test")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	if r.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 — a diagnostic that faults is not a diagnostic, and a "+
			"machine with no usable interface is the case this route is most often asked about",
			r.StatusCode)
	}
	var got networkTestResponse
	if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}

	// The verdict is one of the four the door defines, and it is NAMED. An empty string would
	// leave a client branching on nothing.
	switch got.Verdict {
	case "working", "nobody-else", "not-heard-back", "nothing-sent":
	default:
		t.Errorf("verdict = %q, which is none of the four `discovery.Verdict` defines. A client "+
			"branches on this string, so an unknown value is a surface with no branch", got.Verdict)
	}
	if got.Summary == "" {
		t.Error("the network test returns no sentence. The whole item is that a non-technical " +
			"user cannot be sent to a terminal, and a machine tag with no prose is a terminal " +
			"answer wearing a JSON hat")
	}
	// **The window is reported, and it is the test's OWN window rather than the browse's.** A
	// reader has to tell "nobody is there" from "nobody answered in two seconds", and this route
	// deliberately listens longer than a browse because too few announce ticks produce a
	// confident, wrong "something is blocking discovery" on a healthy machine.
	if got.WindowMs != int(networkTestWindow.Milliseconds()) {
		t.Errorf("windowMs = %d, want %d — a verdict without the window it was measured over "+
			"cannot be read, and this route must not borrow the browse's shorter one",
			got.WindowMs, networkTestWindow.Milliseconds())
	}

	// The counters are published so the answer can be CHECKED rather than trusted, and they must
	// agree with the verdict — a summary that says one thing while its own evidence says another
	// is worse than no summary.
	st := discovery.Stats{Sent: got.Sent, Own: got.Own, Peers: got.Peers}
	if st.Verdict().Name() != got.Verdict {
		t.Errorf("the route reports verdict %q and counters (sent=%d own=%d peers=%d) that "+
			"classify as %q. The published counters are what let a user quote the evidence, so "+
			"they must be the evidence the verdict was drawn from",
			got.Verdict, got.Sent, got.Own, got.Peers, st.Verdict().Name())
	}
}

// The classifier's own truth table, driven directly — the route above can only ever produce
// whatever this machine's network happens to do, so the four states get asserted here.
//
// **`Sent` is checked FIRST and that ordering is the rule, not an implementation detail.** A
// machine that sent nothing also heard nothing of its own, so `Own == 0` is true there too;
// classifying on `Own` first would tell a user with no working interface that a firewall is
// dropping their multicast, which sends them to the wrong place entirely.
func TestTheVerdictSeparatesTheThreeWaysOfFindingNothing(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   discovery.Stats
		want discovery.Verdict
	}{
		{"nothing left the machine", discovery.Stats{}, discovery.VerdictNothingSent},
		{"sent, and not one came back", discovery.Stats{Sent: 6}, discovery.VerdictNotHeardBack},
		{"heard ourselves, nobody else", discovery.Stats{Sent: 6, Own: 6}, discovery.VerdictNobodyElse},
		{"a peer answered", discovery.Stats{Sent: 6, Own: 6, Peers: 1}, discovery.VerdictWorking},
		// The ordering case: nothing was sent AND nothing came back. Only one of those is the
		// user's actual problem.
		{"neither sent nor heard", discovery.Stats{Own: 0, Peers: 0}, discovery.VerdictNothingSent},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.in.Verdict(); got != tc.want {
				t.Errorf("Verdict() = %v, want %v for %+v", got, tc.want, tc.in)
			}
			if tc.in.Verdict().Summary() == "" {
				t.Error("this verdict has no sentence for a person")
			}
			if tc.in.Verdict().Name() == "" {
				t.Error("this verdict has no machine tag")
			}
		})
	}

	// Every verdict says something DIFFERENT. Four states folded into one sentence is the defect
	// this whole item is about, one level up.
	seen := map[string]string{}
	for _, v := range []discovery.Verdict{
		discovery.VerdictWorking, discovery.VerdictNothingSent,
		discovery.VerdictNotHeardBack, discovery.VerdictNobodyElse,
	} {
		if prev, dup := seen[v.Summary()]; dup {
			t.Errorf("%q and %q share a sentence, so a user cannot tell them apart", prev, v.Name())
		}
		seen[v.Summary()] = v.Name()
	}
	if len(seen) != 4 {
		t.Errorf("four verdicts produced %d distinct sentences", len(seen))
	}
}
