package p2p

import (
	"os"
	"strings"
	"testing"
)

// TestReceiveArmsTheDeadlinesTheArrivalLagCountsOn — the population check `ReceiveArrivalLag`'s
// own doc points at (P08.S04a).
//
// `ReceiveArrivalLag()` is `2*exchangeDeadline + PeerGateWindow`, and the **2** is a claim about how
// many deadlines `Receive` arms BEFORE the arrival gate. `internal/server` nests its hop budget
// around that figure, so a deadline added or moved here silently invalidates an inequality in
// another package — and nothing else in the tree would notice.
//
// **This is not a hypothetical.** Both a deepdive and its grill miscounted these arms, in different
// directions (two, then three), and neither error was visible to any test because the count lived
// in prose. The real answer is FOUR arms, of which TWO precede the gate.
func TestReceiveArmsTheDeadlinesTheArrivalLagCountsOn(t *testing.T) {
	raw, err := os.ReadFile("session.go")
	if err != nil {
		t.Fatal(err)
	}
	// Comments stripped: ReceiveArrivalLag's own doc quotes the arm count, and a scan satisfied by
	// prose that merely names the call is how a freeze guard once read its own explanation as proof
	// of coverage (v1.117.155).
	var b strings.Builder
	for _, ln := range strings.Split(string(raw), "\n") {
		if j := strings.Index(ln, "//"); j >= 0 {
			ln = ln[:j]
		}
		b.WriteString(ln)
		b.WriteByte('\n')
	}
	src := b.String()
	i := strings.Index(src, "func Receive(")
	if i < 0 {
		t.Fatal("setup: Receive not found in session.go — this scan is pinned to a function that " +
			"no longer exists")
	}
	// The body ends at the next top-level func; the gate is the coSignExchange call inside it.
	end := strings.Index(src[i+1:], "\nfunc ")
	if end < 0 {
		end = len(src) - i - 1
	}
	body := src[i : i+1+end]

	arms := strings.Count(body, "SetDeadline(")
	// STIMULUS: a scan that matched nothing agrees with every count below.
	if arms == 0 {
		t.Fatal("setup: no SetDeadline arms found in Receive — the scan is blind, and a clean " +
			"result here would mean nothing")
	}
	gate := strings.Index(body, "coSignExchange(")
	if gate < 0 {
		t.Fatal("setup: coSignExchange not found in Receive — the arrival gate is reached through " +
			"it, so 'before the gate' cannot be located")
	}
	pre := strings.Count(body[:gate], "SetDeadline(")

	if arms != 4 {
		t.Errorf("Receive arms %d deadlines, want 4 — two exchangeDeadline before the gate and two "+
			"postConsentDeadline after it. A change here moves the worst-case lag that "+
			"internal/server's hop-nesting guard depends on.", arms)
	}
	if pre != 2 {
		t.Errorf("%d of Receive's deadlines are armed BEFORE the arrival gate, want 2 — "+
			"ReceiveArrivalLag multiplies exchangeDeadline by exactly that number, so this is the "+
			"figure another package's inequality is built on", pre)
	}
	t.Logf("Receive: %d deadline arm(s), %d before the arrival gate; ReceiveArrivalLag = %s",
		arms, pre, ReceiveArrivalLag())
}
