package server

import (
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
		{"cause3 controllable NAT: port-forward offered", d19Inputs{dhtResponded: true, peerSeen: true, mappingDependent: true}, causeMappingDependent, true},
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
