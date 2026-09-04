package server

import (
	"context"
	"encoding/hex"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nib/internal/addrscope"
	"nib/internal/ceremony"
	"nib/internal/rendezvous"
	"nib/internal/sign"
	"nib/internal/udpmux"
)

// The live end of D6's candidate path: a record published to the PUBLIC BitTorrent DHT is
// fetched by a second party, passed through the real gate, and DIALLED.
//
// # Why this exists, and what it closes
//
// `/pending 2` carried two clauses. Clause 1 — that the publish deferral actually defers —
// was discharged by tier 4's `nft` egress counter, which counts the attempt at the wire
// rather than trusting a library's return value. Clause 2 survived: *"a candidate fetched
// from a real record reaching a real dial — publish → fetch → gate → racer, between two
// processes, against the public DHT."*
//
// Three of those four links already existed and the fourth did not:
//
//   - `internal/rendezvous`'s `TestLivePublishAndFetch` drives publish → fetch live, but the
//     value it publishes is an opaque byte string. No `CandidateRecord`, so no gate. It
//     cannot have one: **L1 forbids `internal/rendezvous` from importing `internal/ceremony`**
//     (`l1_test.go`), because a package that can read an invitation is a package that can
//     consult a pinned fingerprint. That ban is why this test is here and not there.
//   - `internal/cli`'s `runSelfTest` (`rendezvous.go:469`) DOES seal a real record, publish
//     it, fetch it at the peer's read salt and put it through `peerGate.Accept`. But it is a
//     **command**, and `build/dhtlive.sh` runs only `go test` — so nothing in any harness
//     reaches it. That is the "executed by nothing" shape `verify_test.go` was written for,
//     one level further out than that guard can see: it walks `_test.go` files for the
//     `NIB_LIVE_DHT` gate, and a CLI command carrying the same capability is invisible to it.
//   - The dial existed nowhere, and `runSelfTest` says so in its own comment: *"This
//     self-test never dials what it publishes, so there is no true answer to give."*
//
// So this test re-drives the whole chain **as a test**, which makes it discoverable by that
// guard and therefore run by the harness, and adds the missing link at the end.
//
// # What it is NOT
//
// **One process, two `rendezvous.Server`s.** The item asked for two processes. What two OS
// processes add over this is a process boundary and two real vaults; what they do not add is
// a single production code path, because the publisher and the fetcher already hold separate
// sockets, separate routing tables and separate caches here, and the publisher is CLOSED
// before the fetch. The residue is filed rather than pretended away.
//
// **Not a NAT traversal test.** The endpoint published is one this host has bound and can
// reach directly. Whether a *stranger* could reach it is the punch ladder's question and
// tier 4's, not this one.
func TestLiveACandidateFetchedFromTheDHTIsDialled(t *testing.T) {
	if os.Getenv("NIB_LIVE_DHT") == "" {
		t.Skip("set NIB_LIVE_DHT=1 (or run ./build/dhtlive.sh) — this test uses the public network")
	}

	// A globally-routable address this host actually answers on.
	//
	// It cannot be loopback and it cannot be private: `CandidateRecord.Seal` refuses an
	// endpoint `addrscope.Target` rejects (`candidate.go:265`, ErrCandidateUnroutable), and
	// that refusal is correct — a record naming an unroutable address is one no peer could
	// ever use. So the record must carry something real, and the dial at the end must be
	// able to reach it. A global IPv6 address satisfies both at once with no NAT in the way,
	// which is why it is preferred over the v4 reflection: dialling our own reflected v4
	// public address would require NAT hairpinning, which many routers do not do, and a
	// failure there would be the router's rather than this code's.
	target, ok := routableSelf()
	if !ok {
		t.Skip("no globally-routable address is bound on this host — the record would name " +
			"an address addrscope refuses, and the dial would have nothing to reach")
	}

	ln, err := net.Listen("tcp", net.JoinHostPort(target.String(), "0"))
	if err != nil {
		t.Skipf("cannot listen on %s: %v", target, err)
	}
	defer ln.Close()
	endpoint := netip.MustParseAddrPort(ln.Addr().String())
	// The port floor is a real refusal, not a formality: an ephemeral port is always above
	// it, but asserting that here means a kernel configured with a low ephemeral range
	// reports its own cause instead of surfacing as an unexplained gate refusal later.
	if endpoint.Port() < addrscope.MinPort {
		t.Skipf("the kernel gave us port %d, below addrscope.MinPort (%d) — the gate would "+
			"refuse this endpoint for a reason that has nothing to do with the path under test",
			endpoint.Port(), addrscope.MinPort)
	}
	accepted := make(chan net.Addr, 1)
	go func() {
		c, aerr := ln.Accept()
		if aerr != nil {
			return
		}
		accepted <- c.RemoteAddr()
		c.Close()
	}()

	// A real two-party ceremony, and the pairing is the production one.
	//
	// **Both gates come from ONE invitation, and they have to.** `NewInvitations` gives every
	// invited party its OWN secret, and `HopSeed` and `RecordKey` derive from that secret — so
	// two *invited* parties can never meet at a rendezvous, by design. The pair that does meet
	// is **convener and party**, on that party's invitation, because the convener holds every
	// party's. An earlier draft of this test built two invitations and derived one gate from
	// each: it published and fetched under different BEP-44 targets, and could only ever have
	// reported "the peer never published".
	inv, certPEM, keyPEM, meFP, peerFP := liveCeremony(t)

	const hop = 0
	// TWO gates, one per party. The peer's gate is the one that READS what we publish — its
	// counterparty is us, so its read salt is our publish salt. Our own gate would refuse
	// this record with ErrCandidateAuthor, correctly, which is why a self-loop could never
	// have exercised the author check.
	gate, err := ceremony.NewCandidateGate(inv, hop, meFP, peerFP)
	if err != nil {
		t.Fatalf("our gate: %v", err)
	}
	peerGate, err := ceremony.NewCandidateGate(inv, hop, peerFP, meFP)
	if err != nil {
		t.Fatalf("peer gate: %v", err)
	}

	rec := ceremony.CandidateRecord{
		CeremonyID: inv.ID,
		Hop:        hop,
		// Long enough to outlive a live publish and fetch against strangers, which the
		// existing live test measures in seconds but which has no upper bound anyone
		// controls. Short enough that a record left behind by a failed run expires.
		Expires: time.Now().Add(10 * time.Minute),
		Addrs:   []ceremony.Endpoint{{Addr: endpoint, Transport: ceremony.TransportTCP}},
	}
	if err := rec.Sign(certPEM, keyPEM); err != nil {
		t.Fatalf("sign the candidate record: %v", err)
	}
	key, err := inv.RecordKey(hop)
	if err != nil {
		t.Fatalf("record key: %v", err)
	}
	// Sealed at OUR publish salt, read at the PEER's. Two different 32-byte values, and
	// swapping them yields an empty fetch that reads exactly like a peer who never published.
	sealed, err := rec.Seal(key, gate.PublishSalt(), hop)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	seed, err := inv.HopSeed(hop)
	if err != nil {
		t.Fatalf("hop seed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	pubDir := t.TempDir()
	pub, pubClose := liveServer(t, pubDir)
	if err := pub.Bootstrap(ctx); err != nil {
		pubClose()
		t.Skipf("the publisher could not bootstrap: %v — that is the network's answer, "+
			"not this path's", err)
	}
	if err := pub.Publish(ctx, seed, gate.PublishSalt(), sealed); err != nil {
		pubClose()
		t.Skipf("publish failed: %v — nothing was written, so there is nothing to fetch", err)
	}
	st := pub.Stats()
	t.Logf("PUBLISHED a %d byte sealed record naming %s; %d node(s) answered the token traversal",
		len(sealed), endpoint, st.PublishNodes)
	if st.PublishNodes == 0 {
		pubClose()
		t.Skip("no node answered the token traversal, so there was nowhere to write")
	}
	// Closed BEFORE the fetch, and its saved node list is what warms the fetcher.
	//
	// This is not a shortcut around the property under test: the cache holds node ADDRESSES
	// and the record's whereabouts are not among them, and the publisher is gone by the time
	// the fetch runs, so nothing it held can answer. What it avoids is a second cold
	// bootstrap minutes after the first — `internal/rendezvous`'s own live test records
	// measuring exactly that failure, the surviving shipped seeds stopping returning nodes to
	// `find_node` under repeated use within one session. A warm cache is also what a real
	// second Nib has.
	pubClose()

	subDir := t.TempDir()
	warmFrom(t, pubDir, subDir)
	sub, subClose := liveServer(t, subDir)
	defer subClose()
	if err := sub.Bootstrap(ctx); err != nil {
		t.Skipf("the fetcher could not bootstrap: %v", err)
	}
	// The PEER's read salt — the same bytes as our publish salt, reached from the other end
	// of the hop.
	got, seq, err := sub.Fetch(ctx, seed, peerGate.Salt())
	if err != nil {
		t.Skipf("fetch found nothing (%v) after %d node(s) answered — on a public DHT that "+
			"can happen to an honest publish, so it is a skip and not a failure",
			err, sub.Stats().FetchNodes)
	}

	// THE GATE. Not a comparison of bytes: `Accept` opens the record at the peer's read
	// salt, verifies the signature against the roster, checks the author is who this hop
	// expects, scopes the hop, and screens every endpoint through addrscope.
	if err := peerGate.Accept(got, time.Now()); err != nil {
		t.Fatalf("the record came back from strangers at seq %d and the gate REFUSED it: %v", seq, err)
	}
	cands := peerGate.Candidates()
	if len(cands) == 0 {
		t.Fatal("the gate accepted the record and yielded no candidate — the endpoint was " +
			"dropped between Accept and Candidates, which is the path a dial can never reach")
	}
	t.Logf("GATE accepted the fetched record at seq %d: %d candidate(s), first %s",
		seq, len(cands), cands[0])

	// THE RACER. `raceCandidates` is the production racer, not a stand-in — the same
	// function `raceWithRendezvous` drives.
	in := make(chan candidate, len(cands))
	for _, e := range cands {
		in <- candidate{
			Addr:      e.Addr.String(),
			Transport: e.Transport.String(),
			Source:    sourceDHT,
			Hop:       hop,
		}
	}
	close(in)
	dctx, dcancel := context.WithTimeout(ctx, 20*time.Second)
	defer dcancel()
	conn, err := raceCandidates(dctx, in, func(c context.Context, cd candidate) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(c, "tcp", cd.Addr)
	})
	if err != nil {
		t.Fatalf("the racer could not dial a candidate the gate accepted: %v", err)
	}
	defer conn.Close()

	// The assertion, and it is about WHERE the dial landed rather than that one happened.
	// A racer that connected to something else entirely would satisfy "no error".
	if got, want := conn.RemoteAddr().String(), endpoint.String(); got != want {
		t.Fatalf("the racer reached %s; the record named %s", got, want)
	}
	select {
	case from := <-accepted:
		t.Logf("DIALLED: the listener named in the published record accepted a connection from %s", from)
	case <-time.After(10 * time.Second):
		t.Fatal("the racer reported a connection to the published endpoint and the listener " +
			"never accepted one — the dial did not reach the process that published")
	}
}

// routableSelf returns an address bound on this host that addrscope will admit.
//
// IPv6 global unicast first and deliberately: it needs no NAT, so the dial at the end of the
// test reaches the listener directly. A ULA (fc00::/7) is scope-global to the kernel and
// reserved to addrscope, so the predicate — not the kernel's idea of scope — is what decides.
func routableSelf() (netip.Addr, bool) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return netip.Addr{}, false
	}
	var v4 netip.Addr
	for _, ifi := range ifaces {
		if ifi.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, aerr := ifi.Addrs()
		if aerr != nil {
			continue
		}
		for _, a := range addrs {
			ipn, okp := a.(*net.IPNet)
			if !okp {
				continue
			}
			ap, okc := netip.AddrFromSlice(ipn.IP)
			if !okc {
				continue
			}
			ap = ap.Unmap()
			if !addrscope.Routable(ap) {
				continue
			}
			if ap.Is6() {
				return ap, true
			}
			if !v4.IsValid() {
				v4 = ap
			}
		}
	}
	return v4, v4.IsValid()
}

// liveCeremony mints a two-party ceremony and returns the INVITED party's invitation.
//
// The publisher is the invited party and the fetcher is the convener, which is the only
// pairing that can share a rendezvous target: `NewInvitations` skips the convener entirely
// — *"the convener receives no invitation: it holds every party's"* — and gives each party
// its own secret, so party-to-party is unreachable by construction and convener-to-party is
// the hop this path actually serves.
func liveCeremony(t *testing.T) (inv ceremony.Invitation, certPEM, keyPEM []byte, meFP, peerFP string) {
	t.Helper()
	convCert, convKey, err := sign.GenerateIdentity("live convener")
	if err != nil {
		t.Fatal(err)
	}
	cfp, err := sign.Fingerprint(convCert)
	if err != nil {
		t.Fatal(err)
	}
	peerFP = hex.EncodeToString(cfp)

	certPEM, keyPEM, err = sign.GenerateIdentity("live publisher")
	if err != nil {
		t.Fatal(err)
	}
	fp, err := sign.Fingerprint(certPEM)
	if err != nil {
		t.Fatal(err)
	}
	meFP = hex.EncodeToString(fp)

	id, err := ceremony.NewID()
	if err != nil {
		t.Fatal(err)
	}
	rec := ceremony.Record{
		ID:      id,
		DocHash: strings.Repeat("ab", 32),
		Intent:  "We agree to co-sign the lease",
		Expires: time.Now().Add(48 * time.Hour),
		Roster: []ceremony.Party{
			{Fingerprint: peerFP, Label: "Convener", Signs: true},
			{Fingerprint: meFP, Label: "Publisher", Signs: true},
		},
	}
	if err := rec.Sign(convCert, convKey); err != nil {
		t.Fatal(err)
	}
	all, err := ceremony.NewInvitations(rec)
	if err != nil {
		t.Fatal(err)
	}
	inv, ok := all[meFP]
	if !ok {
		t.Fatalf("no invitation for the invited party %s (the map holds %d)", meFP, len(all))
	}
	return inv, certPEM, keyPEM, meFP, peerFP
}

// liveServer opens a rendezvous Server on its own socket and cache directory.
//
// The returned closer takes the mux down too: `Server.Close` deliberately does not close the
// socket, because in production the session shares it.
func liveServer(t *testing.T, dir string) (*rendezvous.Server, func()) {
	t.Helper()
	pc, err := net.ListenPacket("udp", ":0")
	if err != nil {
		t.Fatalf("socket: %v", err)
	}
	m := udpmux.New(pc)
	s, err := rendezvous.Open(m.DHT(), dir)
	if err != nil {
		m.Close()
		t.Fatalf("open: %v", err)
	}
	var done bool
	closer := func() {
		if done {
			return
		}
		done = true
		s.Close()
		m.Close()
	}
	t.Cleanup(closer)
	return s, closer
}

// warmFrom copies a closed server's saved node cache into a fresh directory.
//
// By filename discovery rather than a constant, because the cache's name is unexported and a
// test that hard-codes it goes quietly vacuous the day it changes — copying nothing, and
// reporting a cold bootstrap as though it were the warm one.
func warmFrom(t *testing.T, from, to string) {
	t.Helper()
	entries, err := os.ReadDir(from)
	if err != nil {
		t.Fatalf("read the publisher's cache dir: %v", err)
	}
	copied := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		b, rerr := os.ReadFile(filepath.Join(from, e.Name()))
		if rerr != nil {
			continue
		}
		if werr := os.WriteFile(filepath.Join(to, e.Name()), b, 0o600); werr != nil {
			t.Fatalf("warm the fetcher's cache: %v", werr)
		}
		copied++
	}
	if copied == 0 {
		t.Fatal("the publisher saved no cache on close, so the fetcher would bootstrap cold " +
			"— which is the condition its own live test measured as unreliable minutes apart")
	}
	t.Logf("WARMED the fetcher from %d file(s) of the publisher's saved node cache "+
		"(addresses only; the record's whereabouts are not among them)", copied)
}
