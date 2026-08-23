package cli

import (
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"time"

	"nib/internal/discovery"
	"nib/internal/pairing"
	"nib/internal/safe"
)

// cmdDiscover is the link-local discovery diagnostic.
//
// # Why this exists as a command rather than a test
//
// Discovery's failure mode is SILENCE, and its causes are indistinguishable from the
// outside: a firewall that drops the group, a VPN that swallows it, an interface with no
// carrier, a peer that is not armed. On someone else's machine — which is where this will
// break — "it didn't find anything" is the entire bug report unless the program can say
// what it tried.
//
// It is also what makes the Windows verification possible at all. Two divergences are
// known and measured, and both are invisible on Linux:
//
//   - x/net's SetControlMessage is unimplemented on Windows, so a control message is nil
//     with a NIL ERROR and any filter written on the arrival interface silently accepts
//     everything. This reports whether the platform supplied one.
//   - an IPv4 group join there resolves the interface to an ADDRESS rather than an index,
//     so an interface whose IPv4 lease has not arrived is joinable on Linux and refused on
//     Windows. This prints, per interface, whether it carried an IPv4 address.
//
// Running it on real Windows produces evidence for both. Nothing else in the tree can:
// build/winrepro.sh runs under wine, which models neither multicast nor interface
// enumeration.
//
// # It needs no vault and does not unlock one
//
// A diagnostic that required the user's passphrase would be useless in the case it is for
// — first run, nothing configured, nothing working. So it announces under a name derived
// from RANDOM bytes, which is safe precisely because of L1: a name only means something to
// a peer that has already pinned the matching fingerprint, and no peer has pinned this one.
// It cannot be mistaken for a real Nib by anything that matters.
func cmdDiscover(args []string) int {
	fs := flag.NewFlagSet("discover", flag.ContinueOnError)
	secs := fs.Int("seconds", 5, "how long to listen")
	quiet := fs.Bool("quiet", false, "only print the summary")
	fs.Usage = usageFunc(fs, "nib discover [--seconds N]",
		"Report what link-local discovery can see: which interfaces were joined and why,\n"+
			"whether announcements left this machine, and what came back.")
	// Through the shared parse, like every other command. Hand-rolling it made `-h` exit 2
	// here and 0 everywhere else — `-h` is a request for help, not a usage error, and a
	// script that checks the exit status of `nib discover -h` gets a different answer than
	// for `nib verify -h`. It also skipped reorder, so flags had to precede positionals in
	// exactly these two commands.
	if code, ok := parse(fs, args); !ok {
		return code
	}
	// A non-positive window would skip both loops and then print "VERDICT: nothing was
	// sent. Discovery cannot work from this machine" — a confident, wrong diagnosis
	// about a machine where nothing was attempted.
	if *secs <= 0 {
		fmt.Fprintln(os.Stderr, "nib discover: --seconds must be at least 1")
		return 2
	}
	return runDiscover(os.Stdout, os.Stderr, time.Duration(*secs)*time.Second, *quiet)
}

func runDiscover(out, errw io.Writer, listen time.Duration, quiet bool) int {
	fmt.Fprintf(out, "nib discovery diagnostic — group %s / %s, port %d\n\n",
		discovery.Group4, discovery.Group6, discovery.Port)

	// The interfaces, and the reason for each verdict. Printed BEFORE anything is
	// opened, so a failure to open still leaves the reader with the list.
	ifs, err := net.Interfaces()
	if err != nil {
		fmt.Fprintf(errw, "cannot enumerate interfaces: %v\n", err)
		return 1
	}
	fmt.Fprintln(out, "interfaces:")
	for _, ifi := range ifs {
		v4 := "no IPv4"
		addrs, _ := ifi.Addrs()
		if discovery.HasIPv4(addrs) {
			v4 = "has IPv4"
		}
		fmt.Fprintf(out, "  %-14s %-38s %s\n", ifi.Name, ifi.Flags.String(), v4)
	}
	fmt.Fprintln(out)

	var nonce [8]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		fmt.Fprintf(errw, "cannot generate a nonce: %v\n", err)
		return 1
	}
	sock, err := discovery.Open(nonce)
	if err != nil {
		fmt.Fprintf(errw, "could not open the discovery socket: %v\n", err)
		fmt.Fprintln(errw, "\nNothing was joined. On a machine with a working network this usually means\n"+
			"every interface was skipped — the list above says which and why.")
		return 1
	}
	defer sock.Close()
	fmt.Fprintf(out, "joined: %v\n", sock.Interfaces())

	// The Windows divergence, surfaced rather than assumed. Nothing in Nib filters on
	// the arrival interface, precisely because this can be false with no error.
	if sock.Stats().NoControlMessage {
		fmt.Fprintln(out, "note:   this platform supplies no arrival interface for received datagrams\n"+
			"        (x/net SetControlMessage is unimplemented here). Nothing filters on it,\n"+
			"        which is why that is survivable — but a filter added later would\n"+
			"        silently accept everything on this platform.")
	}

	// A name from random bytes: see the doc comment. It matches nobody's pin.
	fpr := sha256.Sum256(nonce[:])
	name, err := pairing.Name(fpr[:])
	if err != nil {
		fmt.Fprintf(errw, "cannot derive a name: %v\n", err)
		return 1
	}
	fmt.Fprintf(out, "announcing as %q (a throwaway identity; it matches nobody's pin)\n\n", name)

	ann := discovery.Announcement{Name: name, Port: 8443, Nonce: nonce}
	deadline := time.Now().Add(listen)
	// Stopped and JOINED before the socket closes. Without the join the goroutine can
	// wake past the deadline, call Announce on a closed socket, and print "announce
	// failed: use of closed network connection" after the verdict — and, because
	// runDiscover takes writers, race with a test's buffer.
	stop := make(chan struct{})
	done := make(chan struct{})
	defer func() { close(stop); <-done }()
	go func() {
		defer safe.Recover("discover announcer")
		defer close(done)
		for time.Now().Before(deadline) {
			select {
			case <-stop:
				return
			default:
			}
			if n, err := sock.Announce(ann); err != nil {
				fmt.Fprintf(errw, "announce failed: %v\n", err)
			} else if n == 0 {
				fmt.Fprintln(errw, "announce reached no interface")
			}
			select {
			case <-stop:
				return
			case <-time.After(500 * time.Millisecond):
			}
		}
	}()

	for time.Now().Before(deadline) {
		seen, err := sock.Read(deadline)
		switch {
		case err == nil:
			if !quiet {
				fmt.Fprintf(out, "  peer   %q port %d from %s\n", seen.Name, seen.Port, seen.From)
			}
		case errors.Is(err, discovery.ErrOwn):
			if !quiet {
				fmt.Fprintln(out, "  self   heard our own announcement come back")
			}
		}
	}

	// ONE read of the counters, not one per consumer. The announce goroutine is still
	// running at this point — it is stopped by the deferred close above — so two calls
	// can return two different sets, and a summary that disagreed with the verdict
	// printed under it would be unfalsifiable by the person reading the output.
	st := sock.Stats()
	printSummary(out, st, listen)
	return printVerdict(out, st)
}

// printSummary renders the counters. Split out of runDiscover with printVerdict below,
// so both can be driven without a socket — see the note on printVerdict.
func printSummary(out io.Writer, st discovery.Stats, listen time.Duration) {
	fmt.Fprintf(out, "\nsummary after %s:\n", listen)
	fmt.Fprintf(out, "  interfaces joined   %d  (%d IPv4, %d IPv6)\n",
		st.Interfaces, st.Joined4, st.Joined6)
	// The one-family failure, said outright rather than left to be inferred from the two
	// numbers above.
	//
	// `Interfaces` counts distinct interfaces and either family succeeding lists one, so a
	// host where every IPv4 join failed is indistinguishable from a healthy dual-stack host
	// by that number alone. This is the suspected behaviour of IP_ADD_MEMBERSHIP on the
	// AF_INET6 socket Go hands us, off Linux — and this command on a real Windows box is
	// what settles it, so the line it needs to print is this one.
	if st.Joined4 == 0 && st.Joined6 > 0 {
		fmt.Fprintf(out, "  NOTE: no IPv4 group was joined — discovery here is IPv6-ONLY. A peer\n"+
			"        that announces only on IPv4 will not be heard, and nothing else in this\n"+
			"        output would say so.\n")
	}
	if st.Joined6 == 0 && st.Joined4 > 0 {
		fmt.Fprintf(out, "  NOTE: no IPv6 group was joined — discovery here is IPv4-ONLY.\n")
	}
	fmt.Fprintf(out, "  announcements sent  %d (failed %d)\n", st.Sent, st.SendErrors)
	fmt.Fprintf(out, "  own copies heard    %d\n", st.Own)
	fmt.Fprintf(out, "  peers heard         %d\n", st.Peers)
	fmt.Fprintf(out, "  other traffic       %d foreign, %d malformed\n", st.Foreign, st.Malformed)
	// OffLink, which this summary omitted while printing all eight of its siblings.
	//
	// Its own doc: "Non-zero means somebody is sending link-local discovery traffic from
	// off the link, which is not a thing that happens by accident." It is the one
	// security-relevant counter in the set, and its only reader was a test — in a command
	// whose doc comment cites P03's lesson that "a counter nobody can read is worth
	// nothing" as the reason it prints its counters exhaustively.
	//
	// Called out on its own line rather than folded in with foreign/malformed, because
	// those two are ordinary on a shared link and this one is not: a zero here is quiet
	// and a non-zero is a finding.
	if st.OffLink > 0 {
		fmt.Fprintf(out, "  OFF-LINK            %d datagram(s) from outside this link — "+
			"not something that happens by accident\n", st.OffLink)
	} else {
		fmt.Fprintf(out, "  off-link            0\n")
	}
	fmt.Fprintln(out)
}

// printVerdict is the diagnostic's conclusion, and it is a pure function of the counters.
//
// # Why it is a function and not four cases inside runDiscover
//
// The verdict IS the result of this command — `nib discover` exists so a user on a
// machine Nib cannot reach can say which of four things went wrong, and it is the only
// instrument the parked Windows verification has. Inside runDiscover the branches were
// decided by a `*discovery.Socket` opened three lines earlier, so nothing could reach
// them without a real network and a real multicast join: 175 lines including all four
// verdicts, and no test file in this package mentioned it. Both verdict paths were driven
// BY HAND at P03.S05 — which is evidence, not coverage, and evidence that expires.
//
// Taking `discovery.Stats` by value makes every branch reachable from a table. That
// matters more than usual here: if this logic is wrong, the Windows run reports the wrong
// thing and nobody would know, because the run's whole purpose is that nothing else can
// see what it sees.
//
// # The order of the cases is load-bearing
//
// `Sent == 0` must be tested before `Own == 0`, and it is not a style question. When
// nothing was sent, nothing can have come back — so the firewall verdict is *also* true
// of that state, and reaching it first would tell a user "a local firewall is dropping
// multicast" about a machine where no announcement was ever attempted. That is the same
// confident-wrong-diagnosis failure the `--seconds` guard above exists to prevent.
func printVerdict(out io.Writer, st discovery.Stats) int {
	// The verdict, and it is the whole reason the counters are separated. "Found
	// nothing" has three causes and a user can act differently on each.
	switch {
	case st.Sent == 0:
		fmt.Fprintln(out, "VERDICT: nothing was sent. Discovery cannot work from this machine —\n"+
			"         no interface accepted an announcement.")
		return 1
	case st.Own == 0:
		fmt.Fprintf(out, "VERDICT: %d announcements left this machine and NOT ONE came back to us.\n"+
			"         A local firewall is dropping multicast on port %d — that is the one\n"+
			"         cause this test can distinguish, because our own copy never leaves\n"+
			"         the host. Peers will not hear us either.\n", st.Sent, discovery.Port)
		return 1
	case st.Peers == 0:
		fmt.Fprintln(out, "VERDICT: sending and receiving both work — we heard ourselves — but no\n"+
			"         other Nib announced. Either nobody has armed a session on this\n"+
			"         network, or they are on a different one.")
		return 0
	default:
		fmt.Fprintf(out, "VERDICT: working. %d peer announcement(s) received.\n", st.Peers)
		return 0
	}
}
