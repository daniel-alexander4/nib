package rendezvous

import (
	"net/netip"
	"testing"
)

// TestTheMajorityIPIsWhatAMajorityOfSourcesSaw — /pending 775, ADR-149.
//
// `MajorityIP` answers the one question a port-mapped candidate needs corroborated — "is this the
// address the world sees us at?" — and it has to answer it behind an endpoint-DEPENDENT NAT, where
// `Class.Addr` is the zero value by design: every source saw a different port and the same IP.
func TestTheMajorityIPIsWhatAMajorityOfSourcesSaw(t *testing.T) {
	for _, c := range []struct {
		name  string
		obs   [][2]string // {from, self}
		want6 bool
		want  string // "" = no answer
		why   string
	}{
		{name: "nothing answered", why: "no sources is no answer, never the zero address as an answer"},
		{
			name: "one source", obs: [][2]string{{"9.9.9.9", "203.0.113.9:1000"}},
			why: "one source agreeing with itself is not corroboration — and it may be the liar",
		},
		{
			name: "two nodes in one /24", obs: [][2]string{{"9.9.9.1", "203.0.113.9:1000"}, {"9.9.9.2", "203.0.113.9:1000"}},
			why: "two nodes behind one NAT are one source",
		},
		{
			name: "an endpoint-dependent NAT: every port different, one IP", want: "203.0.113.9",
			obs: [][2]string{{"9.9.9.9", "203.0.113.9:1000"}, {"8.8.8.8", "203.0.113.9:2000"}, {"7.7.7.7", "203.0.113.9:3000"}},
			why: "this is the case a mapped port exists for, and Class.Addr is empty in it",
		},
		{
			name: "one liar among three", want: "203.0.113.9",
			obs: [][2]string{{"9.9.9.9", "203.0.113.9:1000"}, {"8.8.8.8", "203.0.113.9:1000"}, {"7.7.7.7", "198.51.100.66:1000"}},
			why: "a strict majority outvotes one lying node",
		},
		{
			name: "an even split", obs: [][2]string{{"9.9.9.9", "203.0.113.9:1000"}, {"8.8.8.8", "198.51.100.66:1000"}},
			why: "half is not a majority; with no way to tell who lies there is no answer",
		},
		{
			name: "the other family is not counted", want6: true,
			obs: [][2]string{{"9.9.9.9", "203.0.113.9:1000"}, {"8.8.8.8", "203.0.113.9:1000"}},
			why: "two IPv4 observations say nothing about where our IPv6 address is",
		},
		{
			name: "IPv6", want6: true, want: "2001:db8::9",
			obs: [][2]string{{"2001:db8:1::1", "[2001:db8::9]:1000"}, {"2001:db8:2::1", "[2001:db8::9]:2000"}},
			why: "the same rule in the other family",
		},
	} {
		var s SelfAddress
		for _, o := range c.obs {
			s.Observations = append(s.Observations, Observation{
				From: netip.MustParseAddr(o[0]), Self: netip.MustParseAddrPort(o[1]),
			})
		}
		got, ok := s.MajorityIP(c.want6)
		if c.want == "" {
			if ok {
				t.Errorf("%s: MajorityIP = %s, want no answer — %s", c.name, got, c.why)
			}
			continue
		}
		if !ok || got != netip.MustParseAddr(c.want) {
			t.Errorf("%s: MajorityIP = %v (ok=%v), want %s — %s", c.name, got, ok, c.want, c.why)
		}
	}
}
