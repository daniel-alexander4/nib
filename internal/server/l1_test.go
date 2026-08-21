package server

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"strings"
	"testing"
	"time"

	"nib/internal/p2p"
	"nib/internal/sign"
)

// L1 at the CONSUMER, which is where nothing was looking.
//
// Five AST-walking guards exist in this tree and all five are producer-side: they assert
// that internal/discovery and internal/rendezvous cannot reach an identity. None walks
// internal/server or internal/p2p, and an import-shaped guard cannot work here — this
// package's whole job is to hold both halves, so it legitimately imports vault, sign, p2p
// AND discovery. The guard has to be value-shaped.

// TestDialAnyWalksPastAnImpostorAndLandsOnThePinnedPeer — the driven half.
//
// This is what a lying rendezvous produces: a candidate list with a wrong address in it.
// The existing TestDialAnyTriesEveryCandidate dials two DEAD addresses with nil
// credentials, so it never reaches a handshake and cannot see identity at all.
func TestDialAnyWalksPastAnImpostorAndLandsOnThePinnedPeer(t *testing.T) {
	certA, keyA, _ := sign.GenerateIdentity("Alice")
	certB, keyB, _ := sign.GenerateIdentity("Bob")
	certM, keyM, _ := sign.GenerateIdentity("Impostor")
	fpA, err := sign.Fingerprint(certA)
	if err != nil {
		t.Fatal(err)
	}
	fpB, err := sign.Fingerprint(certB)
	if err != nil {
		t.Fatal(err)
	}

	serve := func(cert, key []byte) string {
		ln, err := p2p.Listen("127.0.0.1:0", cert, key, fpA)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { ln.Close() })
		// Handshake concurrently, so a refused dial does not make the NEXT one wait.
		//
		// **The reason used to be a property of the product and no longer is.** This
		// said `tlsListener.Accept` runs the handshake inline under a handshakeTimeout,
		// "a real property of the product's arm loop too, measured and filed" — true
		// until v1.117.1, which moved the handshake off the accept path under
		// `maxConcurrentHandshakes` (`internal/p2p/transport.go`), and the item it
		// referred to was closed on 2026-08-21 measured at 2.3 ms with five stalled
		// connections in front. The rig stays concurrent because this test still owns
		// its own listener and should not depend on the product's accept policy; the
		// justification is corrected because a stale comment here produced a wrong
		// finding from an adversarial reviewer during P05's phase-open grill.
		go func() {
			for {
				c, err := ln.Accept()
				if err != nil {
					return
				}
				go c.Close()
			}
		}()
		return ln.Addr().String()
	}
	// A FRESH impostor per case, deliberately: a listener that has just refused a dial is
	// not the same subject as one that has not, and reusing it would make the second case
	// depend on the first case's leftovers.
	//
	// **Not, as this used to say, because the accept path serialises handshakes.** That
	// was true until v1.117.1 and is not now — see the note in `serve` above.
	pinned := serve(certB, keyB)

	// STIMULUS: the real peer alone is reachable, so a failure below is about the
	// impostor and not about the rig.
	if c, err := dialAny(tcpCands(pinned), certA, keyA, fpB); err != nil {
		t.Fatalf("stimulus: the pinned peer alone was unreachable: %v", err)
	} else {
		c.Close()
	}

	// The impostor FIRST, as an attacker who answers faster would arrange.
	conn, err := dialAny(tcpCands(serve(certM, keyM), pinned), certA, keyA, fpB)
	if err != nil {
		t.Fatalf("an impostor in the candidate list denied the ceremony entirely: %v", err)
	}
	defer conn.Close()
	// It must be the PINNED peer we landed on, not merely someone.
	if got := conn.PeerFP; string(got) != string(fpB) {
		t.Fatalf("connected to %x, pinned %x — a lying rendezvous substituted a signer", got, fpB)
	}

	// And an impostor ALONE must fail, not silently succeed.
	if c, err := dialAny(tcpCands(serve(certM, keyM)), certA, keyA, fpB); err == nil {
		c.Close()
		t.Fatal("a candidate list containing only an impostor produced a channel")
	} else if !strings.Contains(err.Error(), "none answered as the pinned peer") {
		t.Errorf("refused, but not as a pin failure: %v", err)
	}
}

// TestNothingWireDerivedReachesAPin — the consumer-side source guard.
//
// `candidate` is the one type in the tree carrying a pinned fingerprint and a discovered
// address in the same object, so this layer's slice of L1 is: **the Fingerprint must come
// from the vault, never from anything the wire supplied.** One wrong keyed field breaks it
// with nothing to notice — the neighbouring guard tests that field's ALIASING, not its
// source.
//
// # It taints, rather than pattern-matching one literal
//
// The first version inspected a single keyed field in one composite literal, and a review
// evaded it FOUR ways by ordinary refactors: assigning the wire value to a local first; an
// unkeyed literal; a type alias on the parameter; and building the struct then assigning the
// field. Each is something a person would write without thinking about this guard at all.
//
// So it does a one-hop-to-fixpoint taint instead: anything assigned from an expression that
// mentions the wire parameter is itself wire-derived, transitively. And it refuses the two
// shapes it cannot analyse rather than passing them silently.
func TestNothingWireDerivedReachesAPin(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0) // no ParseComments: a comment must not satisfy any of this
	if err != nil {
		t.Fatal(err)
	}
	pkg, ok := pkgs["server"]
	if !ok || len(pkg.Files) == 0 {
		t.Fatal("no non-test files parsed — every check below would pass on nothing")
	}

	wireParams, keyedFields, checked := 0, 0, 0
	for name, f := range pkg.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			fd, ok := n.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				return true
			}
			// Which parameters carry wire data? By TYPE, and the type is resolved through
			// local aliases — a `type wireSeen = discovery.Seen` evaded the first version.
			tainted := map[string]bool{}
			if fd.Type.Params != nil {
				for _, p := range fd.Type.Params.List {
					if !wireType(f, types.ExprString(p.Type)) {
						continue
					}
					for _, nm := range p.Names {
						tainted[nm.Name] = true
						wireParams++
					}
				}
			}
			if len(tainted) == 0 {
				return true // this function sees no wire data
			}

			// Taint to fixpoint: x := <expr mentioning anything tainted> taints x.
			for round := 0; round < 8; round++ {
				grew := false
				ast.Inspect(fd.Body, func(n ast.Node) bool {
					as, ok := n.(*ast.AssignStmt)
					if !ok {
						return true
					}
					dirty := false
					for _, rhs := range as.Rhs {
						if mentionsAny(types.ExprString(rhs), tainted) {
							dirty = true
						}
					}
					if !dirty {
						return true
					}
					for _, lhs := range as.Lhs {
						if id, ok := lhs.(*ast.Ident); ok && !tainted[id.Name] {
							tainted[id.Name] = true
							grew = true
						}
					}
					return true
				})
				if !grew {
					break
				}
			}

			// Now: nothing tainted may reach a Fingerprint, by literal or by assignment.
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				switch v := n.(type) {
				case *ast.CompositeLit:
					lit, isCand := candidateLit(v)
					if !isCand {
						return true
					}
					for _, el := range v.Elts {
						kv, ok := el.(*ast.KeyValueExpr)
						if !ok {
							// An UNKEYED candidate literal cannot be analysed by field
							// name, and one evaded the first version of this guard. Refuse
							// the shape rather than pass it.
							t.Errorf("%s: %s builds a candidate with positional fields; "+
								"this guard cannot see which one is Fingerprint. Use keyed "+
								"fields.", name, fd.Name.Name)
							continue
						}
						key, ok := kv.Key.(*ast.Ident)
						if !ok || key.Name != "Fingerprint" {
							continue
						}
						keyedFields++
						checked++
						if mentionsAny(types.ExprString(kv.Value), tainted) {
							t.Errorf("%s: %s sets candidate.Fingerprint from wire-derived "+
								"data (%s). L1: nothing learned from the network may "+
								"influence WHICH peer is accepted — the pin comes from the "+
								"vault.", name, fd.Name.Name, types.ExprString(kv.Value))
						}
					}
					_ = lit
				case *ast.AssignStmt:
					// c.Fingerprint = <wire> — the build-then-assign evasion.
					for i, lhs := range v.Lhs {
						sel, ok := lhs.(*ast.SelectorExpr)
						if !ok || sel.Sel.Name != "Fingerprint" {
							continue
						}
						checked++
						if i < len(v.Rhs) && mentionsAny(types.ExprString(v.Rhs[i]), tainted) {
							t.Errorf("%s: %s assigns a Fingerprint from wire-derived data (%s)",
								name, fd.Name.Name, types.ExprString(v.Rhs[i]))
						}
					}
				}
				return true
			})
			return true
		})
	}

	// Three floors, because each one failing silently is a different way to guard nothing.
	if wireParams == 0 {
		t.Fatal("no function in this package takes a wire type — the taint set is empty and " +
			"every check above passed on nothing. A type rename would look exactly like this.")
	}
	if keyedFields == 0 {
		t.Fatal("no keyed candidate.Fingerprint field was inspected. Counting candidate " +
			"LITERALS is not enough: resolve's two zero-value `candidate{}` early returns " +
			"satisfy that count while the one literal this guard exists to police is gone.")
	}
	if checked == 0 {
		t.Fatal("nothing was checked")
	}
}

// wireType reports whether a rendered parameter type is wire data, resolving local aliases.
func wireType(f *ast.File, rendered string) bool {
	if strings.Contains(rendered, "discovery.") {
		return true
	}
	// `type wireSeen = discovery.Seen` — an alias evaded the first version.
	alias := false
	ast.Inspect(f, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSpec)
		if !ok || ts.Name.Name != strings.TrimLeft(rendered, "*[]") {
			return true
		}
		if strings.Contains(types.ExprString(ts.Type), "discovery.") {
			alias = true
		}
		return true
	})
	return alias
}

// candidateLit reports whether a composite literal builds a candidate, including the
// elided-type form inside []candidate{{…}} or map[string]candidate{k: {…}}.
func candidateLit(cl *ast.CompositeLit) (*ast.CompositeLit, bool) {
	if cl.Type == nil {
		// Elided: the enclosing literal names the type. Treated as a candidate so an
		// elided form cannot be the hiding place; a false positive here costs one keyed
		// check on a non-candidate struct, which is harmless.
		return cl, true
	}
	if id, ok := cl.Type.(*ast.Ident); ok && id.Name == "candidate" {
		return cl, true
	}
	return cl, false
}

// mentionsAny reports whether an expression mentions any tainted identifier.
func mentionsAny(expr string, tainted map[string]bool) bool {
	for w := range tainted {
		if mentions(expr, w) {
			return true
		}
	}
	return false
}

// mentions reports whether an expression's rendered text uses identifier w as a whole word.
func mentions(expr, w string) bool {
	for i := 0; i+len(w) <= len(expr); i++ {
		if expr[i:i+len(w)] != w {
			continue
		}
		before := i == 0 || !isIdentByte(expr[i-1])
		afterIdx := i + len(w)
		after := afterIdx == len(expr) || !isIdentByte(expr[afterIdx])
		if before && after {
			return true
		}
	}
	return false
}

func isIdentByte(b byte) bool {
	return b == '_' || (b >= '0' && b <= '9') || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

// TestAClockSkewIsNotReportedAsAPinFailure — D19 cause 5 reaching a user.
//
// The dial error is wrapped by dialAny as "tried N address(es), none answered as the pinned
// peer". That is exactly right when nobody answered as the pinned peer, and exactly WRONG
// when the peer was the pinned peer and the two machines disagree about the time: the user
// was told to check an address that is correct, about a peer that is right, while the
// ten-second fix sat inside the wrapper. P05's criterion asks for cause 5 "and never cause 4".
func TestAClockSkewIsNotReportedAsAPinFailure(t *testing.T) {
	skew := &p2p.ClockSkewError{Behind: true, By: 90 * time.Minute}
	wrapped := fmt.Errorf("tried 1 address(es), none answered as the pinned peer: %w", skew)

	got := connectFailure(wrapped)
	if strings.Contains(got, "none answered as the pinned peer") {
		t.Errorf("a clock skew is reported as a pin failure, which is false — the peer WAS "+
			"the pinned peer: %q", got)
	}
	if strings.Contains(got, "could not connect to peer") {
		t.Errorf("cause 5 is still wearing cause 4's headline: %q", got)
	}
	if !strings.Contains(got, "clock") {
		t.Errorf("the sentence does not mention the clock: %q", got)
	}

	// And an ordinary failure keeps its wrapper — the lift must not swallow cause 4.
	ordinary := connectFailure(errors.New("tried 2 address(es), none answered as the pinned peer: EOF"))
	if !strings.Contains(ordinary, "could not connect to peer") {
		t.Errorf("an ordinary dial failure lost its headline: %q", ordinary)
	}
}
