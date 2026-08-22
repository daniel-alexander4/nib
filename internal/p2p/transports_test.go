package p2p

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"
)

// transport is one way to reach a peer, as the session-logic tests see it: a
// listener and a dialler that both hand back a *Conn with its Channel established.
// Nothing below this line in a test knows which one it got, which is the whole of
// D14 — two transports cannot share one core while the tests name one of them.
type transport struct {
	name   string
	listen func(addr string, cert, key, pin []byte) (Listener, error)
	dial   func(ctx context.Context, addr string, cert, key, pin []byte, timeout time.Duration) (*Conn, error)
}

// transports is THE table. Every session-logic test runs over each entry, so a
// property that holds on TCP and not on QUIC fails by name rather than going unasked.
var transports = []transport{
	{"tcp", Listen, Dial},
	{"quic", QUICListen, QUICDial},
}

// eachTransport runs body once per transport, as a named subtest.
func eachTransport(t *testing.T, body func(t *testing.T, tr transport)) {
	t.Helper()
	for _, tr := range transports {
		t.Run(tr.name, func(t *testing.T) { body(t, tr) })
	}
}

// TestEveryTransportIsInTheTable is the population floor, and it is the reason the
// table is worth having at all.
//
// A parameterised suite is only as good as its parameter list: a third transport
// added later inherits nothing from these tests, and the suite stays green while
// covering two of three. So the count is pinned against the source rather than
// against itself — every exported constructor in this package that returns a
// Listener must appear above.
//
// This is the same shape as TestL2CoversEveryDocumentCarryingEntryPoint, and it is
// here for the same reason that one exists: the behavioural tests enumerate what
// they know about, and what nobody entered is invisible to them.
func TestEveryTransportIsInTheTable(t *testing.T) {
	// **DISCOVERED, not listed.** This used to read `transport.go` and `quic.go` by name,
	// and P05.S04 added a listener constructor in a THIRD file — so the population floor
	// could not see the very thing it exists to count. That is this guard's own defect
	// class, happening to this guard: what nobody entered is invisible to it.
	files, gerr := filepath.Glob("*.go")
	if gerr != nil {
		t.Fatal(gerr)
	}
	var srcs []byte
	read := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, rerr := os.ReadFile(f)
		if rerr != nil {
			t.Fatal(rerr)
		}
		srcs = append(srcs, b...)
		read++
	}
	// Stimulus: the glob really read this package. Without it the count below is equally
	// true of no files at all.
	if read < 3 {
		t.Fatalf("read %d non-test files in internal/p2p — the glob is not seeing the "+
			"package, so the count below would pass on almost nothing", read)
	}
	// Stimulus: the files really are the ones that declare listeners. Without this
	// the count below is equally true of two empty files.
	if len(srcs) == 0 {
		t.Fatal("no source read — the count below would pass on nothing")
	}

	// Matched on the RETURN TYPE, not on the name. The first draft looked for names
	// ending in "Listen" and could not match `Listen` itself — it reported one
	// constructor where there are two, which is the guard catching its own reach
	// before it ever guarded anything.
	found := regexp.MustCompile(`(?m)^func ([A-Z]\w+)\([^)]*\) \(Listener, error\)`).
		FindAllStringSubmatch(string(srcs), -1)

	// **Exemptions, each with a reason, and each checked to be what it claims.**
	//
	// The rule the table enforces is "every way of getting a Listener is exercised by the
	// session-logic suite". A constructor that is a VARIANT of a table entry — the same
	// listener, differing only in who owns the socket underneath — is covered by that
	// entry's runs, and adding it to the table would run the whole suite twice over one
	// transport while labelling half of it something else. That is precisely what the
	// pointer-distinctness check below exists to refuse.
	exempt := map[string]string{
		"QUICListenOn": "the same QUIC listener as QUICListen, on an endpoint the caller " +
			"owns (caveat 7's shared socket). QUICListen is the table entry and covers the " +
			"listener's behaviour; what differs is only who closes the transport.",
	}
	counted := []string{}
	for _, m := range found {
		if _, ok := exempt[m[1]]; ok {
			continue
		}
		counted = append(counted, m[1])
	}
	// An exemption must not outlive the function it excuses: a reason left behind for a
	// deleted constructor silently covers the next one given that name.
	for name := range exempt {
		if !strings.Contains(string(srcs), "func "+name+"(") {
			t.Errorf("%s is exempted from the transport table but does not exist. An "+
				"exemption for a deleted function is a hole waiting for the next one.", name)
		}
	}
	if len(counted) != len(transports) {
		t.Fatalf("this package exports %d unexempted listener constructors (%v) but the "+
			"transport table has %d entries. A transport missing from the table is one the "+
			"session-logic tests never run over, and the suite stays green while covering "+
			"the others.", len(counted), counted, len(transports))
	}

	// And each entry reaches a DISTINCT constructor. The count above cannot see this:
	// a table of {"tcp", Listen, Dial} and {"quic", Listen, Dial} has two entries and
	// two source constructors, satisfies every check so far, and quietly runs the whole
	// suite over TCP twice while every subtest is labelled quic. Comparing the function
	// values is the only thing that distinguishes a parameterised suite from a
	// convincingly labelled one.
	names := map[string]bool{}
	listens := map[uintptr]string{}
	dials := map[uintptr]string{}
	for _, tr := range transports {
		if tr.name == "" || tr.listen == nil || tr.dial == nil {
			t.Fatalf("transport %q is incompletely declared", tr.name)
		}
		if names[tr.name] {
			t.Fatalf("two transports are both named %q", tr.name)
		}
		names[tr.name] = true

		lp := reflect.ValueOf(tr.listen).Pointer()
		if other, dup := listens[lp]; dup {
			t.Fatalf("transports %q and %q share one listener — %q's subtests are labelled "+
				"for a transport they never use", other, tr.name, tr.name)
		}
		listens[lp] = tr.name

		dp := reflect.ValueOf(tr.dial).Pointer()
		if other, dup := dials[dp]; dup {
			t.Fatalf("transports %q and %q share one dialler — %q's subtests are labelled "+
				"for a transport they never use", other, tr.name, tr.name)
		}
		dials[dp] = tr.name
	}
}
