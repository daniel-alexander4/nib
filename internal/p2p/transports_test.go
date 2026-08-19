package p2p

import (
	"os"
	"reflect"
	"regexp"
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
	dial   func(addr string, cert, key, pin []byte, timeout time.Duration) (*Conn, error)
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
	var srcs []byte
	for _, f := range []string{"transport.go", "quic.go"} {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		srcs = append(srcs, b...)
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
	if len(found) != len(transports) {
		names := []string{}
		for _, m := range found {
			names = append(names, m[1])
		}
		t.Fatalf("this package exports %d listener constructors (%v) but the transport table "+
			"has %d entries. A transport missing from the table is one the session-logic tests "+
			"never run over, and the suite stays green while covering the others.",
			len(found), names, len(transports))
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
