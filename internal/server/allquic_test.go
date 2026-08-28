package server

import "testing"

// TestTheGlarePathIsOnlyTakenWhenItCanDialEverything — /pending 298, the predicate the fix rests on.
//
// The glare/shared-endpoint dial races QUIC candidates only: `connect` feeds its race through
// `filterQUIC`, because *"the shared endpoint speaks QUIC, and a non-QUIC candidate cannot be
// handshake-dialled on it"*. So taking that path with anything else in hand silently discards it.
//
// **Three states, and the two that are not "all QUIC" are the ones with history.** P07.S05b took
// the path whenever the request did not name a transport, which dropped the only reachable
// candidate on a link and told the user *"Couldn't reach the rendezvous network"* about a peer that
// was announcing. P07.S05c replaced that with ANY-QUIC, which a single lingering announcement from
// a previous run was enough to satisfy — same outcome, one degree rarer. It is ALL-QUIC now.
//
// The empty case is the one a reader is most likely to get wrong when editing this: no candidates
// means the browse found nothing, and the ordinary dialer — which also feeds from the DHT — is the
// path that can still recover. Vacuous truth would send it to the race that cannot.
func TestTheGlarePathIsOnlyTakenWhenItCanDialEverything(t *testing.T) {
	q := func(a string) candidate { return candidate{Addr: a, Transport: transportQUIC} }
	tcp := func(a string) candidate { return candidate{Addr: a, Transport: "tcp"} }

	for _, tc := range []struct {
		name  string
		cands []candidate
		want  bool
		why   string
	}{
		{"all QUIC", []candidate{q("a:1"), q("b:2")}, true,
			"every candidate can be handshake-dialled on the shared endpoint, so the glare join applies"},
		{"all TCP", []candidate{tcp("a:1"), tcp("b:2")}, false,
			"filterQUIC would discard every one of them and the race would run empty until connectDeadline"},
		{"mixed", []candidate{q("a:1"), tcp("b:2")}, false,
			"the TCP half would be discarded — a peer that armed TCP and also published a UDP port mapping produces exactly this"},
		{"empty", nil, false,
			"no candidates is not 'all of them are QUIC'; the ordinary dialer still feeds from the DHT and can recover, and the glare race cannot"},
		{"one TCP among many QUIC", []candidate{q("a:1"), q("b:2"), q("c:3"), tcp("d:4")}, false,
			"ANY-QUIC was the previous rule and a single stale announcement satisfied it"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := allQUICCandidates(tc.cands); got != tc.want {
				t.Errorf("allQUICCandidates(%v) = %v, want %v — %s", tc.cands, got, tc.want, tc.why)
			}
		})
	}
}
