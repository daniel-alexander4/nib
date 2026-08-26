package p2p

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestNoBudgetSpansTwoHumanWaits — the arithmetic, which nothing checked.
//
// Four timeout constants govern a ceremony and, until this file, `grep` returned **no
// test anywhere** referencing any of them. Both defects this guards shipped because of
// that: quicIdle was 5 minutes against a 6-minute exchangeDeadline while its own doc said
// it was "deliberately longer", and one budget covered two 5-minute human gates.
func TestNoBudgetSpansTwoHumanWaits(t *testing.T) {
	// A ceremony is not idle while a human reads — so the transport must say so on the
	// wire. Without a keep-alive, quic-go sends nothing at all (its own doc: "If set to 0,
	// then no keep alive is sent"), and any prompt longer than quicIdle kills the
	// connection with a transport error in place of the session's own message.
	if quicKeepAlive <= 0 {
		t.Fatal("KeepAlivePeriod is zero — a QUIC session emits nothing while a user is " +
			"at a prompt, and the idle timer ends the ceremony instead of the session core")
	}
	// quic-go clamps to half of MaxIdleTimeout; a keep-alive at or above that is not a
	// keep-alive, it is the idle timeout with extra steps.
	if quicKeepAlive >= quicIdle/2 {
		t.Errorf("quicKeepAlive %v is not comfortably under half of quicIdle %v", quicKeepAlive, quicIdle)
	}

	// The mux coupling, stated as an assertion rather than as a comment. A connection that
	// outlives its connection-id entry starts routing short headers to the DHT — which
	// udpmux's own doc calls "the worse failure of the two". The keep-alive refreshes that
	// entry, so it must fire well inside the TTL.
	const muxPeerTTL = 5 * time.Minute // internal/udpmux: peerTTL
	if quicKeepAlive >= muxPeerTTL/2 {
		t.Errorf("quicKeepAlive %v does not refresh the mux's %v peer/CID entry with margin",
			quicKeepAlive, muxPeerTTL)
	}

	// And the constant must actually reach the config. quicKeepAlive can be perfectly
	// sized and simply not wired — which is the state the field was in, at zero, for the
	// whole life of the transport. Assert the value quic-go will read, not the constant.
	//
	// Deliberately NOT a source scan for the old "deliberately longer than the session
	// core" sentence: the first draft did exactly that and went red against the comment
	// explaining why the claim had been wrong. A guard that reads its own documentation as
	// code is a shape this repo has already paid for.
	cfg := quicConfig()
	if cfg.KeepAlivePeriod != quicKeepAlive {
		t.Errorf("quicConfig().KeepAlivePeriod = %v, want %v — the constant exists and the "+
			"config does not carry it, so nothing is sent and the idle timer still ends "+
			"the ceremony", cfg.KeepAlivePeriod, quicKeepAlive)
	}
	if cfg.MaxIdleTimeout != quicIdle {
		t.Errorf("quicConfig().MaxIdleTimeout = %v, want %v", cfg.MaxIdleTimeout, quicIdle)
	}
}

// TestEveryEntryPointReArmsAfterItsHumanGate.
//
// The structural half. `SetDeadline` is absolute, so a gate spends wire budget while
// nothing moves; the four entry points must re-arm after `runVerification` returns. Two of
// them did not, and the failure lands after the peer has already signed.
//
// By shape rather than by name: find each function that calls runVerification, and require
// a SetDeadline call after it in the same body.
func TestEveryEntryPointReArmsAfterItsHumanGate(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("session.go"))
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "session.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	var checked int
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		var gateAt, resetAfter token.Pos
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			switch fun := call.Fun.(type) {
			case *ast.Ident:
				if fun.Name == "runVerification" && !gateAt.IsValid() {
					gateAt = call.Pos()
				}
			case *ast.SelectorExpr:
				if fun.Sel.Name == "SetDeadline" && gateAt.IsValid() &&
					call.Pos() > gateAt && !resetAfter.IsValid() {
					resetAfter = call.Pos()
				}
			}
			return true
		})
		if !gateAt.IsValid() {
			continue
		}
		checked++
		if !resetAfter.IsValid() {
			t.Errorf("%s calls runVerification and never re-arms the deadline afterwards — "+
				"the user's time at the prompt is spent from the wire budget, and the "+
				"timeout lands after the peer has already co-signed",
				fn.Name.Name)
		}
	}
	// The floor. FIVE entry points carry document bytes and all five carry the gate; zero or one
	// means the matcher stopped matching and every assertion above ran over nothing.
	//
	// `Carry` joined at P07.S05 — a carrier moves a whole document across the wire, so it takes
	// the same gate for the same reason, and the count moving is the tax a new entry point pays.
	if checked != 5 {
		t.Fatalf("found %d entry point(s) calling runVerification, want 5 (Initiate, Carry, "+
			"Receive, SendDocument, ReceiveDocument) — the matcher has gone blind", checked)
	}
}

// TestADialingSideOutwaitsBothOfThePeersGates.
//
// `TestEveryEntryPointReArmsAfterItsHumanGate` is structural — it AST-matches "is there a
// SetDeadline after runVerification". That is not the property the invariant asserts. The
// arithmetic one is: a read that waits on the peer's decisions must outlast **two** of their
// gates plus the transfer, and for the two dialing entry points it did not.
//
// The failure needs no attacker: the initiator answers in five seconds, the responder takes
// four minutes at the spoken check and three at consent — both inside the advertised windows —
// and the initiator times out while the responder co-signs and saves. Both users have signed
// and one holds the artifact.
func TestADialingSideOutwaitsBothOfThePeersGates(t *testing.T) {
	if remoteDecisionDeadline < 2*PeerGateWindow {
		t.Errorf("remoteDecisionDeadline %v cannot cover two peer gates of %v — the read that "+
			"waits on the peer's spoken check AND their consent times out while they are "+
			"still deciding, after which they co-sign and save and the dialer holds nothing",
			remoteDecisionDeadline, PeerGateWindow)
	}
	// And it must leave room for the co-signature and the write-back on top, or the timeout
	// lands in the worst place: after the peer's key has been used.
	if remoteDecisionDeadline <= 2*PeerGateWindow {
		t.Errorf("remoteDecisionDeadline %v leaves nothing for the co-signature and a %d MiB "+
			"write-back after the peer's last gate", remoteDecisionDeadline, maxFrame>>20)
	}

	// The dialing entry points must actually arm it. By shape, so a rename cannot disarm this.
	src, err := os.ReadFile("session.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "session.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"Initiate": false, "SendDocument": false}
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		if _, ours := want[fn.Name.Name]; !ours {
			continue
		}
		body := string(src[fset.Position(fn.Body.Pos()).Offset:fset.Position(fn.Body.End()).Offset])
		want[fn.Name.Name] = strings.Contains(body, "remoteDecisionDeadline")
	}
	for name, armed := range want {
		if !armed {
			t.Errorf("%s never arms remoteDecisionDeadline — its read waits on two of the "+
				"peer's human gates under a budget sized for one", name)
		}
	}
}
