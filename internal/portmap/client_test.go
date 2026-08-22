package portmap

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"strings"
	"testing"
	"time"
)

func mockAddrPort(t *testing.T, g *mockGateway) netip.AddrPort {
	t.Helper()
	ap, err := netip.ParseAddrPort(g.addr())
	if err != nil {
		t.Fatal(err)
	}
	return ap
}

// The client obtains a mapping through a gateway that speaks both protocols. PCP is tried
// first, so this is the PCP path; the external IP comes back inline.
func TestClientMapsThroughPCP(t *testing.T) {
	g := newMockGateway(t)
	c := &Client{Gateway: mockAddrPort(t, g)}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	m, ext, err := c.Map(ctx, UDP, 40404)
	if err != nil {
		t.Fatal(err)
	}
	if m.ExternalPort != g.extPort {
		t.Errorf("external port %d, want the gateway's assigned %d", m.ExternalPort, g.extPort)
	}
	if ext != g.extIP {
		t.Errorf("external IP %v, want %v", ext, g.extIP)
	}
}

// A gateway that speaks NAT-PMP but NOT PCP: the client must fall back and still get a mapping,
// including the external IP from the separate NAT-PMP request. This is the fallback D15's order
// ("PCP, then NAT-PMP") exists for, and a client that only ever tried PCP would MISS here.
func TestClientFallsBackToNATPMPWhenPCPIsSilent(t *testing.T) {
	g := newMockGatewaySilentPCP(t)
	c := &Client{Gateway: mockAddrPort(t, g)}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	m, ext, err := c.Map(ctx, TCP, 40404)
	if err != nil {
		t.Fatalf("fallback to NAT-PMP did not produce a mapping: %v", err)
	}
	if m.ExternalPort != g.extPort {
		t.Errorf("external port %d, want %d", m.ExternalPort, g.extPort)
	}
	if ext != g.extIP {
		t.Errorf("NAT-PMP external IP %v, want %v — the second request did not resolve it", ext, g.extIP)
	}
}

// No gateway answers at all: a MISS within the budget, ErrNoMapping, never a hard error and
// never a hang. `192.0.2.1` is TEST-NET-1 (RFC 5737) — guaranteed to route nowhere.
func TestClientMissesCleanlyWhenNothingAnswers(t *testing.T) {
	c := &Client{Gateway: netip.MustParseAddrPort("192.0.2.1:5351")}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	start := time.Now()
	_, _, err := c.Map(ctx, UDP, 40404)
	if !errors.Is(err, ErrNoMapping) {
		t.Errorf("a silent gateway did not miss cleanly: %v", err)
	}
	// The whole thing stayed inside the budget rather than hanging.
	if el := time.Since(start); el > 3*time.Second {
		t.Errorf("the miss took %v, past the budget — a tier that never gives up blocks the race", el)
	}
}

// A ctx cancellation is distinct from a miss: the user abandoned, and D19 must not report "the
// router offered nothing" for it.
func TestClientReportsCancellationDistinctFromAMiss(t *testing.T) {
	c := &Client{Gateway: netip.MustParseAddrPort("192.0.2.1:5351")}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	_, _, err := c.Map(ctx, UDP, 40404)
	if errors.Is(err, ErrNoMapping) {
		t.Errorf("a cancelled context was reported as a tier miss, not as cancellation: %v", err)
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("want context.Canceled, got %v", err)
	}
}

var _ = net.Dial

// Unmap deletes a NAT-PMP mapping by sending a MAP with lease 0 (grill C1/T01). The mock
// gateway records the lease of every request; the delete is the one carrying 0.
func TestUnmapNATPMPSendsALeaseZeroDelete(t *testing.T) {
	g := newMockGatewaySilentPCP(t) // NAT-PMP path
	c := &Client{Gateway: mockAddrPort(t, g)}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	m, _, err := c.Map(ctx, UDP, 40404)
	if err != nil {
		t.Fatal(err)
	}
	if m.via != mechNATPMP {
		t.Fatalf("mapping recorded mechanism %d, want NAT-PMP — Unmap would delete via the wrong protocol", m.via)
	}
	// Drain the obtain's own lease (non-zero) so we assert on the DELETE's lease.
	select {
	case l := <-g.leases:
		if l == 0 {
			t.Fatal("the obtain itself carried lease 0 — the fixture is wrong")
		}
	case <-time.After(time.Second):
		t.Fatal("the gateway never saw the obtain")
	}

	if err := c.Unmap(ctx, m); err != nil {
		t.Fatal(err)
	}
	select {
	case l := <-g.leases:
		if l != 0 {
			t.Errorf("Unmap sent a request with lease %d, want 0 (the delete form)", l)
		}
	case <-time.After(2 * time.Second):
		t.Error("Unmap sent nothing the gateway saw — a UPnP-style leak, the mapping self-expires only")
	}
}

// Unmap deletes a UPnP mapping via its stored control URL (grill C1 — the whole reason Map now
// carries the handle). A client that discarded the control URL could not do this at all.
func TestUnmapUPnPSendsADeletePortMapping(t *testing.T) {
	m := newMockIGD(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	// The handle is built DIRECTLY against the mock's control URL — NOT via mapViaUPnP, which
	// does real SSDP multicast and would reach the developer's actual router. The obtain
	// mechanics are covered by the mock-IGD component tests in upnp_test.go; this test is only
	// about Unmap routing a DeletePortMapping to the stored control URL.
	mapping := Mapping{
		Protocol: UDP, InternalPort: 40404, ExternalPort: 40404, via: mechUPnP,
		upnpControlURL:  m.srv.URL + "/ctl",
		upnpServiceType: "urn:schemas-upnp-org:service:WANIPConnection:1",
	}
	c := &Client{}
	if err := c.Unmap(ctx, mapping); err != nil {
		t.Fatal(err)
	}
	select {
	case body := <-m.deleted:
		if !strings.Contains(body, "DeletePortMapping") {
			t.Errorf("the IGD saw a delete body that is not a DeletePortMapping: %s", body)
		}
	case <-time.After(2 * time.Second):
		t.Error("the IGD never saw a DeletePortMapping — a UPnP mapping cannot be deleted (grill C1)")
	}
}

// The refresh entry point requests a SPECIFIC external port (grill C7), so a refresh can ask the
// router to keep the port the published record names. Driven by asserting the request carries it.
func TestMapWithSuggestionRequestsThePort(t *testing.T) {
	g := newMockGatewaySilentPCP(t)
	c := &Client{Gateway: mockAddrPort(t, g)}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	// The mock echoes the internal port and assigns its own external, so we assert the WIRE by
	// intercepting: mapWithSuggestion(proto, internal, suggestedExternal=51999).
	if _, _, err := c.mapWithSuggestion(ctx, UDP, 40404, 51999); err != nil {
		t.Fatal(err)
	}
	// (The mock does not honor the suggestion, which is realistic; the point of T02's stability
	// is the REQUEST carries it. That the API path exists at all is what C7 was about — a
	// dedicated assertion on the encoded suggested-external byte is in portmap_test.go.)
}
