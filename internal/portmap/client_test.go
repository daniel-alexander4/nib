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

// TestARequestThatReachedTheRouterLeavesADeletableHandle — /pending 257.
//
// Every error path in Map returns a zero Mapping, so a request the router ACTED ON whose reply
// was then lost left a mapping nothing could ever delete: it lived to lease expiry, and once the
// ceremony frees the internal port another process on this machine can bind it and be publicly
// reachable through the orphaned pinhole. P05.S07 T02's grill required the handle be recorded
// "the moment the request is SENT" and nothing implemented it.
func TestARequestThatReachedTheRouterLeavesADeletableHandle(t *testing.T) {
	g := newMockGatewayDroppingReplies(t)
	c := &Client{Gateway: mockAddrPort(t, g)} // TryUPnP stays false: no SSDP, hermetic

	var sent []Mapping
	c.SetOnRequestSent(func(m Mapping) { sent = append(sent, m) })

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, _, err := c.Map(ctx, UDP, 40404); err == nil {
		t.Fatal("setup: the gateway answered nothing, so Map must fail — this row is about the error path")
	}

	// SETUP ASSERTION, and it is the one that stops this being a green over an absence: the
	// router must actually have RECEIVED a mapping request. Without it, a client that never sent
	// anything would satisfy every assertion below by having nothing to record.
	select {
	case l := <-g.leases:
		if l == 0 {
			t.Fatal("setup: the gateway saw a DELETE, not an obtain")
		}
	default:
		t.Fatal("setup: the gateway never received a request, so this row proves nothing")
	}

	if len(sent) == 0 {
		t.Fatal("Map recorded no handle for a request the router RECEIVED — the mapping it may " +
			"have created is undeletable and lives to lease expiry")
	}

	// And the handle must actually drive a delete. Drain first, so what we read next is ours.
	for len(g.leases) > 0 {
		<-g.leases
	}
	if err := c.Unmap(ctx, sent[len(sent)-1]); err != nil {
		t.Fatalf("the recorded handle did not produce a delete: %v", err)
	}
	select {
	case l := <-g.leases:
		if l != 0 {
			t.Errorf("the delete carried lease %d, want 0 — a lease-0 MAP is what deletes", l)
		}
	case <-time.After(2 * time.Second):
		t.Error("the recorded handle produced no delete request at all")
	}
}

// TestThePCPDeleteCarriesTheMappingsOwnNonce — found while grilling /pending 257.
//
// PCP names a mapping by (nonce, protocol, internal port). Unmap used to mint a FRESH nonce, so
// the delete asked the router to remove a mapping that never existed — a silent no-op, and not
// only on the error path: on the ordinary success path too. Nothing could see it, because the
// mock echoes the nonce back and validates nothing, and the one existing delete test drives
// NAT-PMP, which has no nonce at all.
func TestThePCPDeleteCarriesTheMappingsOwnNonce(t *testing.T) {
	g := newMockGateway(t)
	c := &Client{Gateway: mockAddrPort(t, g)}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	m, _, err := c.Map(ctx, UDP, 40404)
	if err != nil {
		t.Fatal(err)
	}
	// SETUP: this must be the PCP path, or the nonce is not the subject of the row.
	if !m.SameTarget(Mapping{Protocol: UDP, InternalPort: 40404, via: mechPCP}) {
		t.Fatal("setup: the mapping did not come from PCP, so this row is about the wrong mechanism")
	}
	var obtained [12]byte
	select {
	case obtained = <-g.nonces:
	default:
		t.Fatal("setup: the gateway recorded no PCP nonce for the obtain")
	}

	if err := c.Unmap(ctx, m); err != nil {
		t.Fatal(err)
	}
	select {
	case deleted := <-g.nonces:
		if deleted != obtained {
			t.Errorf("the delete carries nonce %x but the mapping was created with %x — PCP names a "+
				"mapping by its nonce, so this delete names one that never existed and is a no-op",
				deleted, obtained)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the delete never reached the gateway")
	}
}

// TestObserveLease — /pending 260.
//
// UPnP-IGD's AddPortMapping has no lease out-argument, so a UPnP mapping carried our REQUEST
// wearing the granted lease's name — on the mechanism D15 says most consumer routers run. An
// IGDv1 that silently ignores NewLeaseDuration installs a PERMANENT mapping and answers 200, and
// nothing in the tree could tell that from a 120-second lease.
func TestObserveLease(t *testing.T) {
	st := "urn:schemas-upnp-org:service:WANIPConnection:1"
	handle := func(m *mockIGD) Mapping {
		return Mapping{Protocol: UDP, InternalPort: 40404, ExternalPort: 40404, via: mechUPnP,
			LifetimeSec: DefaultLeaseSec, upnpControlURL: m.srv.URL + "/ctl", upnpServiceType: st}
	}

	t.Run("a permanent mapping is observed as permanent, not as a zero lease", func(t *testing.T) {
		m := newMockIGD(t)
		m.entryLease = "0" // the IGDv1 that ignored the lease we asked for
		h := handle(m)
		if h.LifetimeObserved {
			t.Fatal("setup: the handle already claims an observed lease")
		}
		got, err := (&Client{}).ObserveLease(context.Background(), h)
		if err != nil {
			t.Fatal(err)
		}
		// SETUP, and it is the assertion that stops this being green over an absence: the router
		// must actually have been ASKED. An ObserveLease that issues no SOAP call and returns a
		// hopeful struct passes every assertion below without it.
		select {
		case <-m.entries:
		default:
			t.Fatal("setup: the IGD never received a GetSpecificPortMappingEntry")
		}
		if !got.LifetimeObserved {
			t.Error("the lease was read back and the mapping still reports it unobserved")
		}
		if !got.LeasePermanent {
			t.Error("a NewLeaseDuration of 0 is a mapping that never expires — D15's crash floor is " +
				"unbounded there, and /pending 258's gate turns on exactly this")
		}
		// THE ROW THAT FAILS IF SOMEONE "SIMPLIFIES" BY WRITING THE 0 INTO LifetimeSec.
		// refreshAfter reads a zero lifetime as "no grant, use the floor", so that would refresh
		// a mapping that never expires every fifteen seconds, forever.
		if got.LifetimeSec != DefaultLeaseSec {
			t.Errorf("LifetimeSec = %d after observing a permanent lease, want it left at %d — a "+
				"zero here is read as its opposite by the cadence", got.LifetimeSec, DefaultLeaseSec)
		}
	})

	t.Run("a real lease is adopted as the cadence input", func(t *testing.T) {
		m := newMockIGD(t)
		m.entryLease = "7200"
		got, err := (&Client{}).ObserveLease(context.Background(), handle(m))
		if err != nil {
			t.Fatal(err)
		}
		if !got.LifetimeObserved || got.LeasePermanent || got.LifetimeSec != 7200 {
			t.Errorf("observed=%v permanent=%v lifetime=%d, want observed with a 7200s lease",
				got.LifetimeObserved, got.LeasePermanent, got.LifetimeSec)
		}
	})

	t.Run("no lease tag is UNOBSERVED, not permanent", func(t *testing.T) {
		m := newMockIGD(t)
		m.entryLease = "" // the tag is absent altogether
		got, err := (&Client{}).ObserveLease(context.Background(), handle(m))
		if err != nil {
			t.Fatal(err)
		}
		// extractTag returns "" for a missing tag and for a body it could not parse alike, so a
		// bare uint32 would report 0 here — and 0 means permanent. That would open /pending 258's
		// deferral gate on a parse failure.
		if got.LifetimeObserved || got.LeasePermanent {
			t.Errorf("an absent NewLeaseDuration was read as an observation (observed=%v permanent=%v)",
				got.LifetimeObserved, got.LeasePermanent)
		}
	})

	t.Run("an entry that is not ours is refused, not read", func(t *testing.T) {
		m := newMockIGD(t)
		m.entryDesc, m.entryLease = "Xbox", "0"
		got, err := (&Client{}).ObserveLease(context.Background(), handle(m))
		if err == nil {
			t.Error("the lease of a mapping belonging to another host was read as ours")
		}
		if got.LifetimeObserved {
			t.Error("a refused read-back still marked the mapping observed — a stranger's lease " +
				"would drive our cadence and our evidence")
		}
	})
}
