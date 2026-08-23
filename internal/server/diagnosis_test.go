package server

import (
	"os"
	"strings"
	"testing"
)

// TestD19ClassifierTable — P05.S11 T05. Each of D19's four causes is produced by driving the signals,
// and the ORDERING and D9's conditional advice are pinned. Every row could fail: it names the exact
// cause and, for cause 3, whether port-forward advice is present.
func TestD19ClassifierTable(t *testing.T) {
	type row struct {
		name string
		in   d19Inputs
		want d19Cause
		// forPortForward: for cause 3, whether the detail may offer a port-forward (D9).
		mentionsPortForward bool
	}
	rows := []row{
		// Cause 2 FIRST — no DHT, even with peerSeen false (which cause 1 also has): ordering matters.
		{"no DHT at all", d19Inputs{dhtResponded: false}, causeDHTUnreachable, false},
		{"no DHT, peer also not seen", d19Inputs{dhtResponded: false, peerSeen: false}, causeDHTUnreachable, false},
		// Cause 1 — DHT up, peer silent AND no record of them at all.
		{"DHT up, peer not started", d19Inputs{dhtResponded: true, peerSeen: false}, causePeerNotStarted, false},
		// peer-record-unusable — DHT up, peerSeen false, BUT a record was there and the gate couldn't
		// use it. Before cause 1: telling a peer who published to "open the ceremony" is the false
		// statement this separates. Both sub-cases (refused, empty) land here.
		{"record refused -> unusable, not 'not started'", d19Inputs{dhtResponded: true, peerSeen: false, recordRefused: true}, causePeerRecordUnusable, false},
		{"record empty -> unusable, not 'not started'", d19Inputs{dhtResponded: true, peerSeen: false, recordEmpty: true}, causePeerRecordUnusable, false},
		// Discriminator: a refused record does NOT hijack the diagnosis once a candidate WAS admitted
		// (one address usable, another refused) — peerSeen wins and the mapping classes take over.
		{"refused but a candidate was admitted -> proceeds to cause 3", d19Inputs{dhtResponded: true, peerSeen: true, recordRefused: true, mappingDependent: true}, causeMappingDependent, true},
		// Ordering: no DHT still beats a refused record (cause 2 first).
		{"no DHT beats a refused record", d19Inputs{dhtResponded: false, recordRefused: true}, causeDHTUnreachable, false},
		// Cause 3 — peer published, endpoint-dependent, no mapping. The advice splits on D9:
		// RENAMED, and the rename is the fix's other half: these inputs are not "controllable NAT",
		// they are "the port-map tier learned NOTHING" — mapUnroutable is set only where a gateway
		// ANSWERED. A row asserting a promise over that state re-asserts the premise the fix removes,
		// and a guard that pins the bug is worse than no guard. What it may now say is CONDITIONED
		// advice, which TestCause3NeverPromisesWhatItCannotKnow checks in full.
		{"cause3 learned nothing about the router: advice mentions it, conditioned", d19Inputs{dhtResponded: true, peerSeen: true, mappingDependent: true}, causeMappingDependent, true},
		// A directly reachable IPv6 endpoint is not a case for port-forward advice at all.
		{"cause3 with v6 reachable: no port-forward, and no denial of reachability", d19Inputs{dhtResponded: true, peerSeen: true, mappingDependent: true, v6Independent: true}, causeMappingDependent, false},
		// A gateway that ANSWERED and refused: the one case where a manual port-forward is right.
		{"cause3 router refused: port-forward IS the advice", d19Inputs{dhtResponded: true, peerSeen: true, mappingDependent: true, mapRefused: true}, causeMappingDependent, true},
		{"cause3 CGNAT: VPN only, no port-forward", d19Inputs{dhtResponded: true, peerSeen: true, mappingDependent: true, sharedSpace: true}, causeMappingDependent, false},
		{"cause3 double-NAT: VPN only, no port-forward", d19Inputs{dhtResponded: true, peerSeen: true, mappingDependent: true, mapUnroutable: true}, causeMappingDependent, false},
		// Cause 3 DEGRADES to cause 4 when the mapping class is unknown/independent.
		{"degrade: mapping not dependent -> cause 4", d19Inputs{dhtResponded: true, peerSeen: true, mappingDependent: false}, causeOther, false},
		// Cause 4 — peer published but a routable mapping exists, so cause 3's premise is false.
		{"has a routable port-map -> cause 4", d19Inputs{dhtResponded: true, peerSeen: true, mappingDependent: true, havePortMap: true}, causeOther, false},
	}
	for _, r := range rows {
		d := classifyD19(r.in)
		if d.cause != r.want {
			t.Errorf("%s: cause = %d, want %d", r.name, d.cause, r.want)
			continue
		}
		if r.want == causeMappingDependent {
			mentions := strings.Contains(d.detail, "port-forward") || strings.Contains(d.detail, "UPnP")
			if mentions != r.mentionsPortForward {
				t.Errorf("%s: detail mentions port-forward = %v, want %v — D9 forbids offering a "+
					"port-forward to a carrier/double-NAT user. detail=%q", r.name, mentions, r.mentionsPortForward, d.detail)
			}
		}
		if d.summary == "" || d.detail == "" {
			t.Errorf("%s: empty summary or detail", r.name)
		}
	}
	// The two peer-record-unusable sub-cases must give DIFFERENT advice — a refused record (stale /
	// wrong ceremony / clock) needs a different fix than an empty one (up, no address yet). A single
	// shared message would defeat the split this cause exists to make.
	refused := classifyD19(d19Inputs{dhtResponded: true, peerSeen: false, recordRefused: true})
	empty := classifyD19(d19Inputs{dhtResponded: true, peerSeen: false, recordEmpty: true})
	if refused.detail == empty.detail || refused.summary == empty.summary {
		t.Errorf("refused and empty records give the same message (summary %q/%q, detail equal=%v) — "+
			"the split is vacuous", refused.summary, empty.summary, refused.detail == empty.detail)
	}
}

// TestD19DiagnosisIsIdentityFree — P05.S11 T04, the L1 pin. The classification is DIAGNOSTIC ONLY: it
// is a pure function of NETWORK signals (d19Inputs carries no identity), so it cannot depend on, or
// leak, who the peer is — and no message names a fingerprint. If the classification could steer the
// pin check this would be the first place identity crept in.
func TestD19DiagnosisIsIdentityFree(t *testing.T) {
	// Every cause's message, checked for any hex-fingerprint-looking content.
	for _, in := range []d19Inputs{
		{dhtResponded: false},
		{dhtResponded: true, peerSeen: false},
		{dhtResponded: true, peerSeen: true, mappingDependent: true},
		{dhtResponded: true, peerSeen: true, mappingDependent: true, sharedSpace: true},
		{dhtResponded: true, peerSeen: true},
	} {
		d := classifyD19(in)
		for _, s := range []string{d.summary, d.detail} {
			// A 40+ hex run would be a fingerprint; the messages are about the network, not the peer.
			run := 0
			for _, ch := range s {
				if (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') {
					run++
					if run >= 16 {
						t.Errorf("a diagnosis message contains a long hex run — possible identity leak: %q", s)
						break
					}
				} else {
					run = 0
				}
			}
		}
	}
}

// diagnose() runs on TWO paths: the post-connect failure path (feed joined) AND the live-status
// path (sessionStatus.status -> diagnose), which is called WHILE the ceremony is armed and the
// feed goroutine is still writing the gate. A direct gate read there is a data race — which is why
// the D19 cause signals (recordRefused/recordEmpty) are snapshotted into atomics in feedCandidates,
// the gate's only writer, and diagnose() reads the atomics. This guard encodes that invariant: a
// future edit that reads the gate directly from diagnose() (the natural way to add another signal)
// reintroduces the race, and -race will not catch it unless a test happens to drive status()
// concurrently with an active feed. The source scan does.
func TestDiagnoseReadsGuardedSignalsNotTheGate(t *testing.T) {
	src, err := os.ReadFile("diagnosis.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	const marker = "func (c *ceremonyID) diagnose()"
	start := strings.Index(s, marker)
	if start < 0 {
		t.Fatal("diagnose() not found in diagnosis.go — this guard has gone blind")
	}
	rest := s[start+len(marker):]
	end := strings.Index(rest, "\nfunc ")
	if end < 0 {
		end = len(rest)
	}
	body := rest[:end]

	// STIMULUS: diagnose must still be the function we think it is — it reads the atomic snapshot
	// and keeps the nil guard. Without these the negative check below could pass on a gutted body.
	if !strings.Contains(body, "c.recordRefused.Load()") {
		t.Fatal("diagnose() no longer reads the c.recordRefused atomic — has the snapshot been removed?")
	}
	if !strings.Contains(body, "c.gate == nil") {
		t.Fatal("diagnose() no longer nil-guards c.gate — this guard has gone blind")
	}
	// THE INVARIANT: no method call on the gate (c.gate.Stats(), c.gate.Candidates(), …). The nil
	// guard is "c.gate == nil" (no trailing dot), so it does not trip this.
	if strings.Contains(body, "c.gate.") {
		t.Error("diagnose() calls a method on c.gate — it runs concurrently with the feed that writes " +
			"the gate (the live-status path), so a direct gate read is a data race. Snapshot the signal " +
			"into an atomic in feedCandidates and read that instead.")
	}
}

// TestCause3NeverPromisesWhatItCannotKnow — /pending 251.
//
// Cause 3's `else` branch is reached by learning NOTHING (see classifyD19), and it used to end
// "…would let it connect" — a promise, to a user Nib has no evidence controls a router. The
// concrete cause named in the item is that `sharedSpace` is IPv4-only, so an IPv6-transition
// CGNAT subscriber (DS-Lite, NAT64, 464XLAT) has both signals false and lands here; the wider
// truth is that a gateway which never answers leaves them false too, whatever the family.
func TestCause3NeverPromisesWhatItCannotKnow(t *testing.T) {
	learnedNothing := classifyD19(d19Inputs{dhtResponded: true, peerSeen: true, mappingDependent: true})
	if strings.Contains(learnedNothing.detail, "would let it connect") {
		t.Errorf("cause 3 promises a port-forward will work to a user it has no evidence controls a "+
			"router — D9 forbids exactly this. detail=%q", learnedNothing.detail)
	}
	if !strings.Contains(learnedNothing.detail, "couldn't get an answer from your router") {
		t.Errorf("cause 3's advice does not say WHY it is conditional, so the condition reads as "+
			"hedging rather than as the fact it is. detail=%q", learnedNothing.detail)
	}

	// THE NEGATIVE CONTROL, and it is what stops a vacuous green: a fix that conditioned every
	// branch identically would pass the assertions above while destroying the distinction cause 3
	// exists to draw. Where the router DID answer with a carrier/double-NAT address, there must be
	// no port-forward advice at all — conditioned or otherwise.
	for _, tc := range []struct {
		name string
		in   d19Inputs
	}{
		{"double-NAT (router answered, unroutable)", d19Inputs{dhtResponded: true, peerSeen: true, mappingDependent: true, mapUnroutable: true}},
		{"carrier space (100.64/10 observed)", d19Inputs{dhtResponded: true, peerSeen: true, mappingDependent: true, sharedSpace: true}},
	} {
		d := classifyD19(tc.in)
		if strings.Contains(d.detail, "port-forward") || strings.Contains(d.detail, "UPnP") {
			t.Errorf("%s: the detail offers a port-forward to a user who provably has no router to "+
				"open one on. detail=%q", tc.name, d.detail)
		}
	}

	// And the half that is a false statement rather than bad advice: `mappingDependent` is either
	// family, so a host whose V4 is carrier-dependent and whose V6 is directly reachable was told a
	// direct connection "isn't possible" while holding the endpoint P05.S05 built the tier for.
	v6 := classifyD19(d19Inputs{dhtResponded: true, peerSeen: true, mappingDependent: true, v6Independent: true})
	if strings.Contains(v6.summary, "isn't possible") {
		t.Errorf("cause 3 denies a direct connection to a host with a directly reachable IPv6 "+
			"endpoint. summary=%q", v6.summary)
	}
	if !strings.Contains(v6.detail, "IPv6") {
		t.Errorf("the v6-reachable case does not tell the user the one thing that would let them "+
			"act on it. detail=%q", v6.detail)
	}
}

// TestARefusedGatewayIsNotToldItStayedSilent — /pending 263.
//
// Every mechanism reports a definitive refusal the same way — a NAT-PMP or PCP result code, an
// IGD UPnPError — and all of them used to end as a bare ErrNoMapping, so "the router refused" and
// "no router answered" became the same value before the diagnosis ever saw them. The refused user
// was then told, in cause 3's else: "Nib couldn't get an answer from your router — so it can't
// tell whether there is one you control. If there is, enabling UPnP or a port-forward on it may
// help." Every clause of that is false here. The router answered, specifically and by code; UPnP
// is not merely on, it is on and talking.
//
// This is a CLASS rather than the 725 case /pending 262 is deferred on: 718 ConflictInMappingEntry
// is what a stale mapping on the same port produces, and it reaches the same branch.
func TestARefusedGatewayIsNotToldItStayedSilent(t *testing.T) {
	refused := classifyD19(d19Inputs{dhtResponded: true, peerSeen: true, mappingDependent: true, mapRefused: true})
	silent := classifyD19(d19Inputs{dhtResponded: true, peerSeen: true, mappingDependent: true})

	// SETUP: the silent case must still say what it says, or "they differ" below could be true
	// because the WRONG one changed.
	if !strings.Contains(silent.detail, "couldn't get an answer from your router") {
		t.Fatalf("setup: the learned-nothing branch no longer says so, so this row is comparing "+
			"against something else. detail=%q", silent.detail)
	}

	if strings.Contains(refused.detail, "couldn't get an answer") {
		t.Errorf("a router that ANSWERED and refused is told Nib got no answer from it — the one "+
			"thing that is demonstrably untrue in this case. detail=%q", refused.detail)
	}
	if refused.detail == silent.detail {
		t.Error("a refusal and a silence produce the same sentence, so carrying the refusal buys nothing")
	}
	// And the advice inverts: this is the case where a manual port-forward is right, because the
	// router is the user's and is reachable.
	if !strings.Contains(refused.detail, "refused") || !strings.Contains(refused.detail, "by hand") {
		t.Errorf("the refusal case does not tell the user what actually happened or what to do about "+
			"it. detail=%q", refused.detail)
	}
	// The control that keeps this from being a blanket "always offer a port-forward": a carrier
	// NAT still must not be told to open one.
	cgnat := classifyD19(d19Inputs{dhtResponded: true, peerSeen: true, mappingDependent: true, sharedSpace: true})
	if strings.Contains(cgnat.detail, "port-forward") || strings.Contains(cgnat.detail, "UPnP") {
		t.Errorf("the carrier-NAT case is offering a port-forward again. detail=%q", cgnat.detail)
	}
}
