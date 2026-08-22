package portmap

import (
	"context"
	"errors"
	"net"
	"net/netip"
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
