package addrscope

import (
	"net/netip"
	"testing"
)

// This package was extracted so two callers could share one table. It owns that table, so
// it owns the tests — before this, its only coverage was internal/rendezvous's delegate
// test, which exercises Routable and never Target, MinPort or SharedSpace.

func TestRoutableRefusesWhatItMust(t *testing.T) {
	refuse := []string{
		// the ordinary shapes
		"0.0.0.0", "127.0.0.1", "10.0.0.1", "192.168.1.1", "172.16.0.1",
		"169.254.1.1", "224.0.0.1", "255.255.255.255", "100.64.0.1",
		"192.0.2.1", "198.51.100.1", "203.0.113.1", "198.18.0.1",
		"192.88.99.1", "240.0.0.1",
		"::1", "::", "fe80::1", "ff02::1", "fc00::1", "fd12:3456::1",
		"2001:db8::1", "2001::1", "2002::1", "64:ff9b::1", "100::1", "5f00::1",
		// FAMILY-CROSSING, both forms. Prefix.Contains is false across families, so each
		// of these clears every v4 prefix in the table unless it is handled.
		"::ffff:127.0.0.1", "::ffff:192.168.1.1", "::ffff:240.0.0.1",
		"::7f00:1", "::a00:1", "::c0a8:101", "::e000:1", "::6440:1", "::1.2.3.4",
		"2001:20::1",
	}
	for _, s := range refuse {
		if Routable(netip.MustParseAddr(s)) {
			t.Errorf("Routable(%s) = true — this becomes a punch target, and both sides send "+
				"hundreds of packets at every candidate", s)
		}
	}
}

func TestRoutableAcceptsOrdinaryPublicAddresses(t *testing.T) {
	for _, s := range []string{"93.184.216.34", "8.8.8.8", "1.1.1.1", "2606:4700:4700::1111"} {
		if !Routable(netip.MustParseAddr(s)) {
			t.Errorf("Routable(%s) = false — an ordinary public address was refused, which "+
				"would silently remove a working tier", s)
		}
	}
}

func TestTheTableIsNotEmpty(t *testing.T) {
	// Stimulus: the loops above pass trivially if the table were empty and the Go
	// predicates alone were doing the work. They are not — that is the whole reason this
	// table exists — so assert it has entries.
	if len(reserved) < 10 {
		t.Fatalf("the reserved table has %d entries; the checks above would be resting on "+
			"Go's predicates alone, which is what this table exists because they do not do",
			len(reserved))
	}
}

func TestTargetAppliesThePortRuleAndItsExceptions(t *testing.T) {
	cases := map[string]bool{
		"93.184.216.34:34154": true,
		"93.184.216.34:1024":  true,
		"93.184.216.34:443":   true,  // TCP fallback — D14's whole reason
		"93.184.216.34:80":    true,  // ditto
		"93.184.216.34:53":    false, // DNS reflection
		"93.184.216.34:123":   false, // NTP
		"93.184.216.34:19":    false, // chargen
		"93.184.216.34:0":     false,
		"192.168.1.1:34154":   false, // routable rule still applies
	}
	for s, want := range cases {
		if got := Target(netip.MustParseAddrPort(s)); got != want {
			t.Errorf("Target(%s) = %v, want %v", s, got, want)
		}
	}
}

func TestSharedSpaceIsAnswerableSeparately(t *testing.T) {
	// CGNAT is a distinct fact from unroutable — the self-address probe reports it to the
	// user as a diagnosis — so it stays separately answerable, in both address forms.
	for _, s := range []string{"100.64.0.1", "100.127.255.254", "::ffff:100.64.0.1"} {
		if !SharedSpace(netip.MustParseAddr(s)) {
			t.Errorf("SharedSpace(%s) = false", s)
		}
	}
	for _, s := range []string{"93.184.216.34", "100.63.255.255", "100.128.0.0"} {
		if SharedSpace(netip.MustParseAddr(s)) {
			t.Errorf("SharedSpace(%s) = true", s)
		}
	}
}
