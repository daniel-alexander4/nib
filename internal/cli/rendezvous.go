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

	"nib/internal/rendezvous"
	"nib/internal/udpmux"
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
	return runRendezvous(os.Stdout, os.Stderr, time.Duration(*secs)*time.Second)
}

func runRendezvous(out, errw io.Writer, budget time.Duration) int {
	fmt.Fprintf(out, "nib rendezvous diagnostic\n\n")
	fmt.Fprintf(out, "  This contacts the PUBLIC BitTorrent DHT — the same network remote\n")
	fmt.Fprintf(out, "  co-signing uses to find your peer. Strangers' computers will see this\n")
	fmt.Fprintf(out, "  machine's public IP address, as they would for anyone using the DHT.\n")
	fmt.Fprintf(out, "  Nothing about your documents is sent, and nothing is published here:\n")
	fmt.Fprintf(out, "  this command only asks questions. Ctrl-C now if that is not wanted.\n\n")

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
	fmt.Fprintf(out, "publish         %d/%d succeeded, %d node(s) held tokens, %d refused at the seq ceiling\n",
		st.Published, st.PublishAttempts, st.PublishNodes, st.PublishSeqCeiling)
	fmt.Fprintf(out, "fetch           %d/%d found, %d empty, %d aborted, %d undecodable, %d node(s) answered\n",
		st.Fetched, st.FetchAttempts, st.FetchEmpty, st.FetchAborted, st.FetchUndecodable, st.FetchNodes)
	fmt.Fprintf(out, "                (this command publishes and fetches nothing; all zero is correct)\n")

	line, code := verdict(st, self, bootErr, probeErr, ctx.Err() != nil)
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
			why = fmt.Sprintf("%d reply/replies DID reach us, so this network is not "+
				"blocking UDP — the shipped seed addresses answered but led nowhere, which "+
				"is what a stale seed list looks like.", st.Responses)
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
