package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"crypto/rand"
	"encoding/hex"
	"net/netip"
	"nib/internal/ceremony"
	"nib/internal/rendezvous"
	"nib/internal/sign"
	"nib/internal/udpmux"
	"runtime"
	"strings"
)

// cmdRendezvous is the DHT diagnostic, and the standing reader for this subsystem's
// counters.
//
// # Why it exists, twice over
//
// First, for the same reason `nib discover` does: the rendezvous fails by SILENCE, and
// its causes are indistinguishable from outside. A blocked UDP port, a dead seed list, a
// NAT that rewrites the mapping, a peer who has not published — on someone else's machine
// they all look like "it didn't connect". This prints what was tried.
//
// Second, and this is the part that is about the code rather than the user:
// `internal/rendezvous` had **thirteen counters and no reader outside its own tests**,
// because nothing in the shipped binary imported the package at all. P03 already paid for
// that lesson once — its own test says "a counter nobody can read is what P03 already
// paid for" — and the answer there was `nib discover`. This is the same answer for the
// same problem, and it is why the counters below are printed exhaustively rather than
// summarised: a counter that appears in no output is a counter that cannot be wrong in a
// way anyone notices.
//
// # It talks to the public internet, and says so first
//
// Everything else `nib` does is local. This is not, and a diagnostic that contacted
// strangers without saying so would be the disclosure failure this slice exists partly to
// fix. The banner is printed BEFORE the socket opens, not after.
//
// It needs no vault: it publishes nothing and derives no key, so there is no identity to
// unlock and nothing that could be mistaken for a ceremony.
func cmdRendezvous(args []string) int {
	fs := flag.NewFlagSet("rendezvous", flag.ContinueOnError)
	secs := fs.Int("seconds", 30, "how long to give the bootstrap and probe")
	selfTest := fs.Bool("self-test", false,
		"additionally publish one throwaway record and fetch it back (writes to the public DHT)")
	fs.Usage = usageFunc(fs, "nib rendezvous [--seconds N]",
		"Report whether this machine can reach the BitTorrent DHT that remote co-signing\n"+
			"uses to find a peer: bootstrap, routing table, observed public address, and\n"+
			"the counters behind each. Contacts the public internet — see the notice it prints.")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *secs <= 0 {
		fmt.Fprintln(os.Stderr, "nib rendezvous: --seconds must be at least 1")
		return 2
	}
	return runRendezvous(os.Stdout, os.Stderr, time.Duration(*secs)*time.Second, *selfTest)
}

// banner is the disclosure, extracted so it can be asserted.
//
// It is the only user-facing statement of what this command sends, and it is prose — which
// rots exactly like a doc comment and is guarded by nothing unless it is testable. Twice in
// one arc a claim of this kind went stale in this repo: the README said Nib made only one
// network call, and then said this command published nothing on the day it started to.
func banner(selfTest bool) string {
	var b strings.Builder
	b.WriteString("nib rendezvous diagnostic\n\n")
	b.WriteString("  This contacts the PUBLIC BitTorrent DHT — the same network remote\n")
	b.WriteString("  co-signing uses to find your peer. Strangers' computers will see this\n")
	b.WriteString("  machine's public IP address, as they would for anyone using the DHT.\n")
	if selfTest {
		b.WriteString("\n  --self-test is on, so this run also PUBLISHES one throwaway record and\n")
		b.WriteString("  fetches it back. It is encrypted under a one-off key made for this run,\n")
		b.WriteString("  tied to no ceremony and to no identity of yours — but publishing means\n")
		b.WriteString("  handing bytes to about eight strangers' computers.\n")
		b.WriteString("  There is no recall. Your copy stops being served when this command\n")
		b.WriteString("  exits; theirs age out on their own schedule, typically a couple of hours.\n")
		b.WriteString("  The bytes are opaque to them, but its size and shape do identify it as\n")
		b.WriteString("  a Nib self-test alongside your IP address.\n")
		b.WriteString("  Ctrl-C now if that is not wanted.\n\n")
	} else {
		b.WriteString("  Nothing about your documents is sent, and nothing is published here:\n")
		b.WriteString("  this command only asks questions. Ctrl-C now if that is not wanted.\n\n")
	}
	return b.String()
}

func runRendezvous(out, errw io.Writer, budget time.Duration, selfTest bool) int {
	fmt.Fprint(out, banner(selfTest))

	pc, err := net.ListenPacket("udp", ":0")
	if err != nil {
		fmt.Fprintf(errw, "cannot open a UDP socket: %v\n", err)
		return 1
	}
	m := udpmux.New(pc)
	defer m.Close()

	dir, err := os.MkdirTemp("", "nib-rendezvous-*")
	if err != nil {
		fmt.Fprintf(errw, "cannot make a scratch directory: %v\n", err)
		return 1
	}
	defer os.RemoveAll(dir)

	// A scratch directory, deliberately, not ~/nib. This command must not write a node
	// cache into the user's home as a side effect of being run once to answer a question
	// — the cache is a record of which DHT nodes this machine spoke to, and a diagnostic
	// should leave nothing behind.
	rz, err := rendezvous.Open(m.DHT(), dir)
	if err != nil {
		fmt.Fprintf(errw, "cannot start the rendezvous: %v\n", err)
		return 1
	}
	defer rz.Close()

	// Ctrl-C must leave nothing behind, and the banner above actively invites it.
	//
	// Without this the deferred RemoveAll never runs: measured, `nib rendezvous | head -9`
	// exits 141 on SIGPIPE and leaves the scratch directory on disk. It is empty at that
	// point — the node cache is only written from Close — so nothing leaks about which DHT
	// nodes were contacted, but the comment above promises a diagnostic leaves nothing
	// behind, and a promise the code does not keep is the thing to fix.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM, syscall.SIGPIPE)
	defer signal.Stop(sigs)
	go func() {
		select {
		case <-sigs:
			cancel()
		case <-ctx.Done():
		}
	}()

	// Two phases, two budgets, because one shared deadline lets the first starve the second.
	//
	// BootstrapContext returns on stall OR ctx.Done, so on a slow-but-working network it
	// spends the whole allowance. ProbeSelf would then run on an expired context, its own
	// 8 s cap a no-op, all sixteen queries failing instantly — and the verdict would read
	// "the DHT is reachable but nothing reported our address back", exit 0, with no hint
	// that the budget was the cause.
	probeShare := budget / 3
	if probeShare < 8*time.Second {
		probeShare = 8 * time.Second
	}
	bootShare := budget - probeShare
	if bootShare < time.Second {
		bootShare = time.Second
	}

	fmt.Fprintf(out, "nib %s on %s/%s\n", buildVersion, runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(out, "local socket    %s\n", m.LocalAddr())
	fmt.Fprintf(out, "bootstrapping   (up to %s of a %s budget)\n", bootShare, budget)
	bootCtx, bootCancel := context.WithTimeout(ctx, bootShare)
	started := time.Now()
	bootErr := rz.Bootstrap(bootCtx)
	bootTook := time.Since(started)
	bootCancel()
	st := rz.Stats()
	if bootErr != nil {
		fmt.Fprintf(out, "  bootstrap returned: %v\n", bootErr)
	}
	fmt.Fprintf(out, "  took                  %s%s\n", bootTook.Round(time.Millisecond), budgetNote(bootTook, bootShare))
	fmt.Fprintf(out, "  seed addresses used   %d%s\n", st.Seeds, coldNote(st.Seeds))
	fmt.Fprintf(out, "  nodes gained          %d\n", st.Bootstrapped)
	fmt.Fprintf(out, "  routing table         %d\n", st.Nodes)

	fmt.Fprintf(out, "\nprobing for this machine's own public address (up to %s)\n", probeShare)
	probeCtx, probeCancel := context.WithTimeout(ctx, probeShare)
	self, probeErr := rz.ProbeSelf(probeCtx)
	probeCancel()
	if probeErr != nil {
		fmt.Fprintf(out, "  probe returned: %v\n", probeErr)
	}
	st = rz.Stats()
	fmt.Fprintf(out, "  usable replies        %d\n", st.Observed)
	fmt.Fprintf(out, "  replies refused       %d length / %d port / %d scope\n",
		st.RejectedLength, st.RejectedPort, st.RejectedScope)
	fmt.Fprintf(out, "  outvoted dissenters   %d\n", st.Disagreements)
	fmt.Fprintf(out, "  IPv4                  %s\n", classLine(self.V4))
	fmt.Fprintf(out, "  IPv6                  %s\n", classLine(self.V6))
	if self.SharedAddressSpace {
		fmt.Fprintf(out, "  NOTE: this machine is behind carrier-grade NAT.\n")
	}

	fmt.Fprintf(out, "\ninbound\n")
	fmt.Fprintf(out, "  replies that reached us                %d%s\n", st.Responses, silentNote(st.Responses))
	fmt.Fprintf(out, "what this machine refused\n")
	fmt.Fprintf(out, "  datagrams that would have crashed us   %d\n", st.Screened)
	fmt.Fprintf(out, "  queries with no arguments              %d\n", st.RefusedQueries)
	fmt.Fprintf(out, "  replies shaped to crash the fetch      %d\n", st.RefusedResponses)
	fmt.Fprintf(out, "  requests to store other people's data  %d  (always refused)\n", st.RefusedStores)

	// The node cache this command used is a scratch directory, so `Loaded` is always 0 and
	// `CacheRejected` always false — printing them as if they were findings reads as a
	// broken cache where there is none, and `Saved` is written by the deferred Close AFTER
	// this line. What is worth reporting is the cache a real ceremony will actually use.
	fmt.Fprintf(out, "\nnode cache      %s\n", realCacheLine())

	// The publish/fetch counters are zero here by construction — this command does
	// neither — and printing them anyway is deliberate: it is what makes the fields
	// visible to anyone reading the output, and a non-zero one would mean this command
	// had grown a behaviour its own banner denies. All of them, because a counter that
	// appears in no output is a counter that cannot be wrong in a way anyone notices, and
	// the four fetch-legibility fields are the ones whose job is to be read by a human.
	// The self-test runs BEFORE the counters print, so they print once and describe the run
	// that just happened. Printing them either side produced two blocks, the first of which
	// was all zeros and read as a failure.
	selfTestFailed := false
	if selfTest {
		selfTestFailed = !runSelfTest(ctx, out, rz, self)
		st = rz.Stats()
	}
	fmt.Fprintf(out, "\npublish         %d/%d succeeded, %d node(s) held tokens, %d refused at the seq ceiling\n",
		st.Published, st.PublishAttempts, st.PublishNodes, st.PublishSeqCeiling)
	fmt.Fprintf(out, "fetch           %d/%d found, %d empty, %d aborted, %d undecodable, %d node(s) answered\n",
		st.Fetched, st.FetchAttempts, st.FetchEmpty, st.FetchAborted, st.FetchUndecodable, st.FetchNodes)
	// AFTER the self-test, which is the only thing that can supply seeds. Printed from the
	// snapshot taken before it, this line could only ever say nothing — the counter it
	// reads is written 300 lines later.
	fmt.Fprint(out, invitationSeedNote(st))
	if !selfTest {
		fmt.Fprintf(out, "                (this command publishes and fetches nothing; all zero is correct — "+
			"pass --self-test to drive them)\n")
	}

	line, code := verdict(st, self, bootErr, probeErr, ctx.Err() != nil)
	if selfTestFailed && code == 0 {
		// The mode whose entire purpose is to prove the publish path works must be able to
		// say that it didn't. Without this, `nib rendezvous --self-test` prints
		// "publish failed" and exits 0, which is invisible to anything scripted.
		line += "\n  BUT the self-test did not complete — see above."
		code = 1
	}
	fmt.Fprintf(out, "\n%s\n", line)
	return code
}

// verdict is pure so its branches can be table-tested — every finding the review raised
// about this logic was reachable only by constructing a Stats value, which the command
// itself made impossible.
func verdict(st rendezvous.Stats, self rendezvous.SelfAddress, bootErr, probeErr error, aborted bool) (string, int) {
	switch {
	case aborted:
		return "VERDICT: interrupted before it finished — nothing here is conclusive.", 1
	case st.Nodes == 0:
		// One branch, not two. The command always runs on a scratch directory, so the
		// cache is always empty and Seeds is always non-zero; a second branch keyed on
		// Seeds could never be reached and encoded a distinction this command cannot make.
		// Responses discriminates the two causes that used to be offered as a guess.
		//
		// Observed on a real run: the table came back empty while TWO replies had reached
		// us — which rules out "this network blocks UDP" outright, and the verdict was
		// naming it anyway. Something answered; it just did not lead anywhere.
		var why string
		switch {
		case bootErr != nil:
			why = "The bootstrap did not finish: " + bootErr.Error() + "."
		case st.Responses > 0:
			which := "the shipped seed addresses"
			if st.InvitationSeedsUsed {
				// Naming the shipped list here would be a confident wrong diagnosis: the
				// table was built from the invitation's addresses, so "your seed list is
				// stale" sends the user to fix the wrong thing.
				which = "the addresses tried (shipped AND invitation-supplied)"
			}
			why = fmt.Sprintf("%d reply/replies DID reach us, so this network is not "+
				"blocking UDP — %s answered but led nowhere.", st.Responses, which)
		default:
			why = "Nothing replied at all, which usually means outbound UDP is blocked here."
		}
		return "VERDICT: no routing table. " + why + "\n" +
			"  Remote co-signing cannot work from here; a ceremony on the same network still can.", 1
	case st.Observed == 0:
		why := ""
		if probeErr != nil {
			why = " (" + probeErr.Error() + ")"
		}
		return fmt.Sprintf("VERDICT: the DHT is reachable (%d nodes) but nothing reported our "+
			"address back%s.\n  Finding a peer may still work; diagnosing a failed connection "+
			"will be harder.", st.Nodes, why), 0
	}
	// Agreement is a property of the CLASSIFICATION, not of how many replies were usable.
	// Reporting Observed as agreement printed "16 nodes agreed" directly under a line
	// saying "1 of 16 sources agreed" whenever the NAT was endpoint-dependent — two
	// contradictory sentences in one pasted report, with the false one as the verdict.
	// Only an ENDPOINT-INDEPENDENT mapping means the sources agreed on one address. An
	// endpoint-dependent verdict is the opposite finding — every responder saw us
	// differently — and it is not MappingUnknown, so a check for "not unknown" falls
	// straight through into announcing an agreement that did not happen.
	best := self.V4
	if best.Mapping != rendezvous.MappingEndpointIndependent &&
		self.V6.Mapping == rendezvous.MappingEndpointIndependent {
		best = self.V6
	}
	if best.Mapping != rendezvous.MappingEndpointIndependent {
		return fmt.Sprintf("VERDICT: the DHT is reachable and %d node(s) replied, but they did "+
			"not agree on our public address.\n  That is what a symmetric NAT looks like; "+
			"remote co-signing will need the harder tiers.", st.Observed), 0
	}
	return fmt.Sprintf("VERDICT: the DHT is reachable and %d of %d independent sources agreed "+
		"we are %s (%s).", best.Agreed, best.Sources, best.Addr, best.Mapping), 0
}

// silentNote turns a zero into the sentence it deserves: with queries going out, nothing
// answering is a different failure from replies arriving and being dropped.
func silentNote(n uint64) string {
	if n == 0 {
		return "  (nothing answered us at all — outbound UDP may be blocked)"
	}
	return ""
}

func coldNote(seeds int) string {
	if seeds > 0 {
		return "  (cold start — no usable node cache)"
	}
	return ""
}

// invitationSeedNote reports the eclipse-relevant fact: this table came from a list
// somebody else chose, after everything Nib ships had failed.
func invitationSeedNote(st rendezvous.Stats) string {
	switch {
	case st.InvitationSeeds == 0:
		return ""
	case st.InvitationSeedsUsed:
		return fmt.Sprintf("  %d invitation-supplied address(es), AND THEY WERE USED — "+
			"everything Nib ships failed first,\n                        so this routing "+
			"table came from a list the invitation's sender chose\n", st.InvitationSeeds)
	case st.InvitationSeedsTried:
		// The third state, and it existed before this branch did. TRIED with USED false is
		// a machine whose shipped list failed AND whose invitation seeds then failed too —
		// the worst case this diagnostic has to describe, and it was printing "the shipped
		// list worked" over it.
		return fmt.Sprintf("  %d invitation-supplied address(es), tried and they answered "+
			"nothing either\n                        (the shipped list had already "+
			"failed) — this machine reached no DHT at all\n", st.InvitationSeeds)
	default:
		return fmt.Sprintf("  %d invitation-supplied address(es), unused (the shipped list "+
			"worked)\n", st.InvitationSeeds)
	}
}

func classLine(c rendezvous.Class) string {
	switch c.Mapping {
	case rendezvous.MappingUnknown:
		return "unknown — fewer than two independent observations"
	case rendezvous.MappingEndpointIndependent:
		return fmt.Sprintf("%s, %s (%d of %d sources agreed)", c.Addr, c.Mapping, c.Agreed, c.Sources)
	default:
		// Addr is deliberately left unset for a non-independent verdict — its own doc says
		// it is meaningful ONLY when the mapping is endpoint-independent — so printing it
		// yields the literal text "invalid AddrPort" on exactly the NAT class where remote
		// co-signing is hardest, i.e. in the report most likely to be pasted to someone.
		return fmt.Sprintf("%s — no single address to report (%d of %d sources agreed)",
			c.Mapping, c.Agreed, c.Sources)
	}
}

// budgetNote flags a phase that consumed its whole allowance, which is the difference
// between "the network is slow" and "we stopped asking".
func budgetNote(took, allowed time.Duration) string {
	if took >= allowed-(allowed/20) {
		return "  (used its whole allowance — the result below may be truncated, not final)"
	}
	return ""
}

// realCacheLine reports the cache an actual ceremony will use, which is NOT the scratch
// directory this command runs on. Read-only: it never creates or repairs anything.
func realCacheLine() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "could not locate your home directory, so the real cache was not inspected"
	}
	path := filepath.Join(home, "nib", "dht-nodes")
	fi, err := os.Stat(path)
	if os.IsNotExist(err) {
		return path + " does not exist yet — a real ceremony would cold-start from the shipped seeds"
	}
	if err != nil {
		return path + ": " + err.Error()
	}
	return fmt.Sprintf("%s, %d bytes, last written %s", path, fi.Size(),
		fi.ModTime().Format(time.RFC3339))
}

// runSelfTest drives the publish/fetch path and the candidate gate end to end.
//
// Returns false if it did not complete, so the command's exit code can say so.
//
// # Why this exists rather than a test
//
// `internal/ceremony`'s gate counts every reason a candidate record can be refused, and
// P03's recorded lesson is that a counter nobody can read is worth nothing. Until the
// ladder exists (P05) there is no product path that reads them, and a test as the only
// reader is exactly what that lesson forbids. So the diagnostic grows a mode that exercises
// the real path against the real network — the same answer `nib discover` was for the
// link-local counters one phase ago.
//
// **What it can and cannot prove, stated so the doc does not overclaim.** It proves the
// fields are wired, formatted and reachable on a live path. It cannot drive the eight
// refusal counters: it builds a valid record and feeds it to its own gate, so those are
// structurally zero here. Driving each cause is `TestEveryRefusalCauseIsCountedSeparately`
// and its siblings — test-only readers, which is the half of P03's lesson this does not
// discharge.
//
// It uses a throwaway identity and a random one-off invitation secret: no vault, no
// ceremony, nothing of the user's on the wire beyond what any DHT participant reveals.
func runSelfTest(ctx context.Context, out io.Writer, rz *rendezvous.Server, self rendezvous.SelfAddress) bool {
	fmt.Fprintf(out, "\nself-test — publishing one throwaway record and fetching it back\n")
	fmt.Fprintf(out, "  (publish and fetch each get up to %s of their own, so this phase can\n", rendezvous.PublishBudget)
	fmt.Fprintf(out, "   take a couple of minutes and a long pause here is not a hang)\n")

	certPEM, keyPEM, err := sign.GenerateIdentity("nib self-test")
	if err != nil {
		fmt.Fprintf(out, "  could not make a throwaway identity: %v\n", err)
		return false
	}
	fp, err := sign.Fingerprint(certPEM)
	if err != nil {
		fmt.Fprintf(out, "  %v\n", err)
		return false
	}
	fpHex := hex.EncodeToString(fp)

	secret := make([]byte, ceremony.SecretLen)
	if _, err := rand.Read(secret); err != nil {
		fmt.Fprintf(out, "  %v\n", err)
		return false
	}
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		fmt.Fprintf(out, "  %v\n", err)
		return false
	}
	inv := ceremony.Invitation{
		Version:             ceremony.InvitationVersion,
		ID:                  hex.EncodeToString(id),
		Roster:              []ceremony.Party{{Fingerprint: fpHex, Signs: true}},
		Secret:              secret,
		ConvenerFingerprint: fpHex,
	}

	// Seeds, sampled from the live table — the producing half of D6's second half, and the
	// only production caller of the seed validator.
	//
	// A validator with no caller is this repo's recorded lesson twice over (P03's "a guard
	// tested a predicate and not that anything called it"), and this slice's own acceptance
	// asks for a caller by name. It is possible here only because seeds reach the DHT
	// through `Seed()` AFTER `Open` — the socket opens long before an invitation exists, so
	// a third `Open` parameter could never have been fed from one.
	inv.Seeds = rz.SeedSample(ceremony.MaxSeeds)
	if len(inv.Seeds) > 0 {
		// Round-trip them through the wire form, so what is exercised is the real
		// validating path a recipient runs and not a helper call.
		text, eerr := inv.Encode()
		if eerr != nil {
			fmt.Fprintf(out, "  seed encode: %v\n", eerr)
			return false
		}
		back, perr := ceremony.ParseInvitation(text)
		if perr != nil {
			fmt.Fprintf(out, "  seed round trip: %v\n", perr)
			return false
		}
		fmt.Fprintf(out, "  sampled %d seed(s) from the live table; %d survived the "+
			"invitation round trip (%d chars)\n", len(inv.Seeds), len(back.Seeds), len(text))
		rz.Seed(back.Seeds)
	}

	const hop = 0
	gate, err := ceremony.NewCandidateGate(inv, hop, fpHex)
	if err != nil {
		fmt.Fprintf(out, "  %v\n", err)
		return false
	}

	// The candidate is OUR OWN observed address, not a constant.
	//
	// It was `93.184.216.34` — example.com, a real third party's allocation — because the
	// documentation ranges reserved for exactly this purpose are (correctly) refused by
	// addrscope. Publishing a signed record that names a stranger as a candidate is
	// harmless only while nothing dials it, and "nothing dials it" is a property of an
	// unfinished ladder rather than of this code. Our own probed address is already public
	// by this point in the run and belongs to us.
	target := self.V4.Addr
	if !target.IsValid() {
		target = self.V6.Addr
	}
	if !target.IsValid() {
		fmt.Fprintf(out, "  skipped: the probe did not establish this machine's own public\n")
		fmt.Fprintf(out, "  address, and the self-test will not name a stranger's as a candidate\n")
		return true // not a failure of the publish path; nothing was attempted
	}
	rec := ceremony.CandidateRecord{
		CeremonyID: inv.ID, Hop: hop,
		Expires: time.Now().Add(5 * time.Minute),
		Addrs:   []netip.AddrPort{target},
	}
	if err := rec.Sign(certPEM, keyPEM); err != nil {
		fmt.Fprintf(out, "  sign: %v\n", err)
		return false
	}
	key, err := inv.RecordKey(hop)
	if err != nil {
		fmt.Fprintf(out, "  %v\n", err)
		return false
	}
	sealed, err := rec.Seal(key, gate.Salt(), hop)
	if err != nil {
		fmt.Fprintf(out, "  seal: %v\n", err)
		return false
	}
	seed, err := inv.HopSeed(hop)
	if err != nil {
		fmt.Fprintf(out, "  %v\n", err)
		return false
	}

	started := time.Now()
	fmt.Fprintf(out, "  publishing %d bytes…\n", len(sealed))
	if err := rz.Publish(ctx, seed, gate.Salt(), sealed); err != nil {
		fmt.Fprintf(out, "  publish failed after %s: %v\n", time.Since(started).Round(time.Millisecond), err)
		return false
	}
	fmt.Fprintf(out, "  published in %s; fetching it back…\n", time.Since(started).Round(time.Millisecond))
	started = time.Now()
	got, seq, err := rz.Fetch(ctx, seed, gate.Salt())
	if err != nil {
		fmt.Fprintf(out, "  fetch failed after %s: %v\n", time.Since(started).Round(time.Millisecond), err)
		fmt.Fprintf(out, "  (for THIS command that means the put reached no node we could read\n")
		fmt.Fprintf(out, "   back from — getput.Put cannot report a per-node refusal)\n")
		return false
	}
	if err := gate.Accept(got, time.Now()); err != nil {
		fmt.Fprintf(out, "  the record came back in %s and the gate REFUSED it: %v\n",
			time.Since(started).Round(time.Millisecond), err)
		return false
	}
	g := gate.Stats()
	fmt.Fprintf(out, "  round trip OK in %s at seq %d\n", time.Since(started).Round(time.Millisecond), seq)
	fmt.Fprintf(out, "  gate            %d record(s) offered, %d refused; %d candidate address(es) accepted\n",
		g.Records(), g.Refused(), g.Accepted)
	fmt.Fprintf(out, "                  dropped: over-cap %d, duplicate %d, re-offered %d, empty records %d\n",
		g.DroppedOverCap, g.DroppedDuplicate, g.Reoffered, g.EmptyRecords)
	fmt.Fprintf(out, "                  refused: sealed %d, format %d, too-many %d, unroutable %d,\n",
		g.RefusedSealed, g.RefusedFormat, g.RefusedTooMany, g.RefusedUnroutable)
	fmt.Fprintf(out, "                           signature %d, author %d, context %d, expired %d\n",
		g.RefusedSignature, g.RefusedAuthor, g.RefusedContext, g.RefusedExpired)
	fmt.Fprintf(out, "  (a non-zero refusal here with an empty fetch elsewhere is how in-roster\n")
	fmt.Fprintf(out, "   preemption is told apart from a peer who simply has not published)\n")
	return true
}
