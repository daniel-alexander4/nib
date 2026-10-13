package server

import (
	"net/netip"
	"os"
	"strings"
	"testing"

	"nib/internal/ceremony"
	"nib/internal/rendezvous"
)

// probeSaw builds the probe result of three sources in three prefixes each placing this machine at
// ip, on a different port each — an endpoint-dependent NAT, the case a mapped port exists for.
func probeSaw(ip string) rendezvous.SelfAddress {
	var s rendezvous.SelfAddress
	for i, from := range []string{"1.1.1.1", "8.8.8.8", "7.7.7.7"} {
		s.Observations = append(s.Observations, rendezvous.Observation{
			From: netip.MustParseAddr(from),
			Self: netip.AddrPortFrom(netip.MustParseAddr(ip), uint16(40000+i)),
		})
	}
	return s
}

// TestAMappedAddressIsPublishedOnlyWhereTheProbeSawThisMachine — /pending 775, ADR-149.
//
// On the UPnP path the external address is whatever the first SSDP responder said it was, and any
// host on the link can be that responder. The address went into a signed candidate record both
// parties dial and punch at, so a stranger chose where this machine's and its peer's traffic was
// aimed. `screenedMappedEndpoint` passes it — a victim's address is public and well-formed.
func TestAMappedAddressIsPublishedOnlyWhereTheProbeSawThisMachine(t *testing.T) {
	mapped := func(ip string) ceremony.Endpoint {
		ep, ok := screenedMappedEndpoint(netip.MustParseAddr(ip), 51234)
		if !ok {
			t.Fatalf("setup: %s does not pass the screen, so this is not the case the screen misses", ip)
		}
		return ep
	}
	const ours, victim = "9.9.9.9", "8.8.4.4" // addrscope refuses documentation space, so real public ones

	if ep, ok, _ := corroboratedMappedEndpoint(probeSaw(ours), mapped(ours)); !ok || ep != mapped(ours) {
		t.Errorf("a mapping at the address the probe saw us at was not published (ok=%v, %v) — the "+
			"mapped port is the one candidate that works behind this NAT", ok, ep)
	}
	if _, ok, elsewhere := corroboratedMappedEndpoint(probeSaw(ours), mapped(victim)); ok || !elsewhere {
		t.Errorf("a mapping naming %s was published (ok=%v, elsewhere=%v) while every probe source saw "+
			"this machine at %s — whoever answered the SSDP search chose where both parties punch",
			victim, ok, elsewhere, ours)
	}
	// No answer from the probe for this family: not published, and not reported as "another NAT".
	if _, ok, elsewhere := corroboratedMappedEndpoint(rendezvous.SelfAddress{}, mapped(ours)); ok || elsewhere {
		t.Errorf("with no probe answer a mapped address came back ok=%v elsewhere=%v, want neither — "+
			"an address nothing corroborated is what this refuses", ok, elsewhere)
	}
	one := rendezvous.SelfAddress{Observations: probeSaw(victim).Observations[:1]}
	if _, ok, _ := corroboratedMappedEndpoint(one, mapped(victim)); ok {
		t.Error("ONE probe source agreeing with the router's answer was enough to publish it — one " +
			"node agreeing is not corroboration, and it may be the same liar")
	}
	// The other family's observations say nothing about this one.
	if _, ok, _ := corroboratedMappedEndpoint(probeSaw("2606:4700::9"), mapped(ours)); ok {
		t.Error("an IPv4 mapping was published on the strength of IPv6 observations")
	}
}

// TestTheMappedCandidateGoesThroughTheCorroboration — the source guard, for
// `TestAFailedObtainStillClosesItsMapper`'s reason: the call site builds its own portmap.Client and
// cannot be driven with a fake. The check must sit between the mapper's creation and the line that
// stores it and appends the candidate, and take the probe `publishCandidates` made.
func TestTheMappedCandidateGoesThroughTheCorroboration(t *testing.T) {
	src, err := os.ReadFile("ceremonynet.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	start := strings.Index(s, "mapper := newPortMapper(")
	stop := strings.Index(s, "c.setPortMap(mapper)")
	if start < 0 || stop < start {
		t.Fatal("setup: could not find the mapper's creation and storage in appendMappedCandidate")
	}
	if !strings.Contains(s[start:stop], "corroboratedMappedEndpoint(self, ep)") {
		t.Error("appendMappedCandidate stores and publishes a mapped address without asking " +
			"corroboratedMappedEndpoint — the router's (or an SSDP responder's) word is then enough")
	}
	if !strings.Contains(s, "c.appendMappedCandidate(armCtx, self, addrs)") {
		t.Error("publishCandidates does not hand its probe to appendMappedCandidate")
	}
}
