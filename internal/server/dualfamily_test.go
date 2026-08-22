package server

import (
	"net/netip"
	"testing"

	"nib/internal/ceremony"
	"nib/internal/rendezvous"
)

// TestAPublishedRecordCarriesBothFamilies — D8 tier 2's precondition, and the defect it names
// is the one that made the tier unreachable rather than merely untested.
//
// `publishCandidates` used to take `self.V4.Addr` and fall back to `self.V6.Addr` only when the
// v4 observation was invalid, then publish a single endpoint. Read as a sentence that is
// dual-stack support; read as code it is the opposite, because the fallback fires only when v4
// has FAILED. A dual-stack host — the case tier 2 exists for — published its v4 address and
// nothing else, so a peer had no v6 address to dial and the tier could not complete however
// well the sockets worked.
//
// The dual-stack row is therefore the point of the test, and the two single-family rows are its
// controls: without them, a function that returned both entries unconditionally (including
// invalid ones) would pass the row that matters.
func TestAPublishedRecordCarriesBothFamilies(t *testing.T) {
	v4 := netip.MustParseAddrPort("203.0.113.5:34154")
	v6 := netip.MustParseAddrPort("[2606:4700:4700::1111]:34154")

	for _, tc := range []struct {
		name   string
		self   rendezvous.SelfAddress
		want   []netip.AddrPort
		lesson string
	}{
		{
			name: "dual-stack publishes both",
			self: rendezvous.SelfAddress{
				V4: rendezvous.Class{Addr: v4},
				V6: rendezvous.Class{Addr: v6},
			},
			want:   []netip.AddrPort{v4, v6},
			lesson: "a dual-stack host that publishes one address cannot be dialled on the other",
		},
		{
			name:   "v4 only",
			self:   rendezvous.SelfAddress{V4: rendezvous.Class{Addr: v4}},
			want:   []netip.AddrPort{v4},
			lesson: "an invalid v6 observation must not become an endpoint",
		},
		{
			name:   "v6 only",
			self:   rendezvous.SelfAddress{V6: rendezvous.Class{Addr: v6}},
			want:   []netip.AddrPort{v6},
			lesson: "a v6-only host must still publish, which the old v4-first read could do only by accident",
		},
		{
			name:   "neither",
			self:   rendezvous.SelfAddress{},
			want:   nil,
			lesson: "publishing an address we do not have sends the peer somewhere that is not us",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := publishableEndpoints(tc.self, transportQUIC)
			if len(got) != len(tc.want) {
				t.Fatalf("got %d endpoint(s) %v, want %d %v — %s",
					len(got), addrsOf(got), len(tc.want), tc.want, tc.lesson)
			}
			for i := range got {
				if got[i].Addr != tc.want[i] {
					t.Errorf("endpoint %d is %s, want %s — %s", i, got[i].Addr, tc.want[i], tc.lesson)
				}
				if got[i].Transport != ceremonyTransport(transportQUIC) {
					t.Errorf("endpoint %d carries transport %v, want the one the arm opened",
						i, got[i].Transport)
				}
			}
		})
	}

	// The record must still seal. Two endpoints is far under the measured worst case (eight
	// IPv6 endpoints seal to 932 of 996 bytes), but the margin is coincident enough at eight
	// that a slice which doubled the endpoint count should say it checked.
	if n := len(publishableEndpoints(rendezvous.SelfAddress{
		V4: rendezvous.Class{Addr: v4},
		V6: rendezvous.Class{Addr: v6},
	}, transportQUIC)); n > ceremony.MaxCandidates {
		t.Errorf("publishing %d endpoints exceeds MaxCandidates of %d", n, ceremony.MaxCandidates)
	}
}

func addrsOf(eps []ceremony.Endpoint) []netip.AddrPort {
	out := make([]netip.AddrPort, 0, len(eps))
	for _, e := range eps {
		out = append(out, e.Addr)
	}
	return out
}
