package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
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

	ctx, cancel := context.WithTimeout(context.Background(), budget)
	defer cancel()

	fmt.Fprintf(out, "local socket    %s\n", m.LocalAddr())
	fmt.Fprintf(out, "bootstrapping   (up to %s)\n", budget)
	bootErr := rz.Bootstrap(ctx)
	st := rz.Stats()
	if bootErr != nil {
		fmt.Fprintf(out, "  bootstrap returned: %v\n", bootErr)
	}
	fmt.Fprintf(out, "  seed addresses used   %d%s\n", st.Seeds, coldNote(st.Seeds))
	fmt.Fprintf(out, "  nodes gained          %d\n", st.Bootstrapped)
	fmt.Fprintf(out, "  routing table         %d\n", st.Nodes)

	fmt.Fprintf(out, "\nprobing for this machine's own public address\n")
	self, probeErr := rz.ProbeSelf(ctx)
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

	fmt.Fprintf(out, "\nwhat this machine refused\n")
	fmt.Fprintf(out, "  datagrams that would have crashed us   %d\n", st.Screened)
	fmt.Fprintf(out, "  queries with no arguments              %d\n", st.RefusedQueries)
	fmt.Fprintf(out, "  replies shaped to crash the fetch      %d\n", st.RefusedResponses)
	fmt.Fprintf(out, "  requests to store other people's data  %d  (always refused)\n", st.RefusedStores)

	fmt.Fprintf(out, "\nnode cache      %d loaded, %d written%s\n",
		st.Loaded, st.Saved, rejectedNote(st.CacheRejected))

	// The publish/fetch counters are zero here by construction — this command does
	// neither — and printing them anyway is deliberate: it is what makes the fields
	// visible to anyone reading the output, and a non-zero one would mean this command
	// had grown a behaviour its own banner denies.
	fmt.Fprintf(out, "publish/fetch   %d/%d published, %d/%d fetched (this command does neither)\n",
		st.Published, st.PublishAttempts, st.Fetched, st.FetchAttempts)

	fmt.Fprintf(out, "\nVERDICT: ")
	switch {
	case st.Nodes == 0 && st.Seeds > 0:
		fmt.Fprintf(out, "nothing answered. Every address Nib ships was unreachable —\n"+
			"  either this network blocks UDP, or the shipped seed list has gone stale.\n"+
			"  Remote co-signing cannot work from here; a ceremony on the same network still can.\n")
		return 1
	case st.Nodes == 0:
		fmt.Fprintf(out, "no routing table. The DHT could not be reached from this machine.\n")
		return 1
	case st.Observed == 0:
		fmt.Fprintf(out, "the DHT is reachable (%d nodes) but nothing reported our address back.\n"+
			"  Finding a peer may still work; diagnosing a failed connection will be harder.\n", st.Nodes)
		return 0
	default:
		fmt.Fprintf(out, "the DHT is reachable and %d nodes agreed on our public address.\n", st.Observed)
		return 0
	}
}

func coldNote(seeds int) string {
	if seeds > 0 {
		return "  (cold start — no usable node cache)"
	}
	return ""
}

func rejectedNote(b bool) string {
	if b {
		return "  (an existing cache could not be read and was ignored)"
	}
	return ""
}

func classLine(c rendezvous.Class) string {
	if c.Mapping == rendezvous.MappingUnknown {
		return "unknown — fewer than two independent observations"
	}
	return fmt.Sprintf("%s, %s (%d of %d sources agreed)", c.Addr, c.Mapping, c.Agreed, c.Sources)
}
