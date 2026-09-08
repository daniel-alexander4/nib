package server

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"nib/internal/discovery"
	"nib/internal/pairing"
	"nib/internal/vault"
)

// The dialer picks its ARM, not just its peer (announcement v3).
//
// # The defect these close
//
// A machine can hold a hop arm and a delivery arm at once, both pinned to the same peer — the
// convener — and both announcing a bare port. They do opposite things: the hop arm can co-sign and
// serve a party's stored contribution back after a restart; the delivery arm confirms the spoken
// check without a human (`autoVerifier`) and can do neither. With no discriminator on the link the
// racer dialled whichever answered first, so a resumed hop met the wrong arm about a third of the
// time — and in production every time, because only the delivery arm is restored automatically.
//
// The rendezvous path never had this problem: its targets are keyed per (ceremony, hop). This is
// the link path catching up.

func seenAt(t *testing.T, fp []byte, hop int) discovery.Seen {
	t.Helper()
	name, err := pairing.Name(fp)
	if err != nil {
		t.Fatal(err)
	}
	return discovery.Seen{
		Announcement: discovery.Announcement{
			Name: name, Port: 9000, Transport: discovery.TransportQUIC, Hop: hop,
		},
		From: &net.UDPAddr{IP: net.ParseIP("192.168.1.9"), Port: discovery.Port},
	}
}

// TestASightingForAnotherArmIsNotACandidate is the whole fix.
func TestASightingForAnotherArmIsNotACandidate(t *testing.T) {
	fp := fpOf(7)
	pins := []vault.PinnedPeer{{Fingerprint: fp, Label: "The convener"}}

	// SETUP: the SAME peer at the hop we want does resolve — so the refusal below is about the
	// arm and not about the pin, which is the distinction the whole test rests on.
	if _, ok := resolve(pins, seenAt(t, fp, 4), 4); !ok {
		t.Fatal("setup: the peer's own hop-4 arm did not resolve, so nothing below is meaningful")
	}

	if c, ok := resolve(pins, seenAt(t, fp, 9), 4); ok {
		t.Errorf("an arm at hop 9 became a candidate for a hop-4 dial (%+v). That is the delivery "+
			"arm and the hop arm being indistinguishable on the link: the racer dials both and "+
			"keeps whichever answers first, and the delivery arm cannot co-sign or serve a stored "+
			"contribution at all", c)
	}
}

// TestAnArmWithNoCeremonyTakesAnyPeer — the other arm of the rule.
//
// The manual and LAN receive paths have no ceremony and therefore no hop to match. If `HopNone`
// did not mean "any", this fix would break every non-ceremony receive on the link — which is a
// bigger surface than the one it repairs.
func TestAnArmWithNoCeremonyTakesAnyPeer(t *testing.T) {
	fp := fpOf(7)
	pins := []vault.PinnedPeer{{Fingerprint: fp, Label: "Someone"}}
	for _, hop := range []int{0, 4, discovery.HopNone} {
		if _, ok := resolve(pins, seenAt(t, fp, hop), discovery.HopNone); !ok {
			t.Errorf("a dial with no ceremony refused a sighting at hop %d. HopNone means ANY — "+
				"the manual and LAN receive paths have no hop to match on", hop)
		}
	}
}

// TestAStrangerIsStillRefusedWhateverItsHop — the hop is reachability, never identity.
//
// A candidate is still gated on the pin. Adding a match the announcement controls must not have
// created a way for an unpinned peer to be dialled by claiming the right hop.
func TestAStrangerIsStillRefusedWhateverItsHop(t *testing.T) {
	pins := []vault.PinnedPeer{{Fingerprint: fpOf(7), Label: "The convener"}}
	if c, ok := resolve(pins, seenAt(t, fpOf(99), 4), 4); ok {
		t.Errorf("an unpinned peer resolved by announcing the right hop (%+v). The hop is "+
			"reachability; the pin is identity, and the hop must not be able to buy one", c)
	}
}

// TestEveryAnnouncerNamesItsHop — the zero-value guard, and it exists because this repo has
// already paid for a meaningful zero once.
//
// `candidate.Source`'s own comment records it: an unset producer "would carry the zero value and be
// accounted to the typed-address source — a split that is present, green, and wrong", and the fix
// was `TestEveryCandidateProducerNamesItsSource`. `Announcement.Hop` has exactly that shape, and
// worse: `Transport`'s zero is *deliberately* meaningful (TCP), but hop 0 is the CONVENER'S OWN
// INDEX. A producer that forgets the field does not announce "no ceremony" — it announces itself as
// hop 0, which is a real arm a real dial will match.
//
// So every composite literal building an Announcement in non-test code must name Hop. This is an
// AST walk rather than a grep for the same reason its sibling is: a grep cannot tell a literal from
// a mention in a comment, and it cannot see a field that is absent.
func TestEveryAnnouncerNamesItsHop(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	pkg, ok := pkgs["server"]
	if !ok || len(pkg.Files) == 0 {
		t.Fatal("setup: internal/server did not parse — this guard walked nothing")
	}

	var found, named int
	for file, f := range pkg.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			cl, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			sel, ok := cl.Type.(*ast.SelectorExpr)
			if !ok || sel.Sel == nil || sel.Sel.Name != "Announcement" {
				return true
			}
			found++
			for _, e := range cl.Elts {
				kv, ok := e.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				if id, ok := kv.Key.(*ast.Ident); ok && id.Name == "Hop" {
					named++
					return true
				}
			}
			t.Errorf("%s:%d builds a discovery.Announcement without naming Hop. The zero value is "+
				"hop 0 — the convener's own index — not \"no ceremony\", so this arm would "+
				"advertise itself as an arm it is not. Pass discovery.HopNone if it has no "+
				"ceremony.", file, fset.Position(cl.Pos()).Line)
			return true
		})
	}
	// The floor: a walk that finds no literals reports a clean tree forever.
	if found == 0 {
		t.Fatal("this guard found no discovery.Announcement literals at all — it is reading " +
			"nothing, which is indistinguishable from a clean result")
	}
	t.Logf("%d Announcement literal(s), %d name their hop", found, named)
}

// TestABrowseWithBothArmsUpReturnsOnlyTheOneAsked is the actual scenario, not a per-sighting unit.
//
// One machine, one pinned peer, TWO announcements — its hop arm and its delivery arm, on different
// ports as they really are. Before the hop travelled on the wire both resolved, the racer dialled
// both, and it kept whichever answered first. This drives the browse the dialer actually calls.
//
// **It is also the shape ADR-010's harness lesson warns about.** That defect stayed latent because
// `pairrepro.sh` passed `-F transport=` to BOTH sides, so tier 4 was configured past the
// disagreement it existed to find. Here the browse is given both announcements and told only what
// it WANTS — it has to pick, rather than being handed the answer.
func TestABrowseWithBothArmsUpReturnsOnlyTheOneAsked(t *testing.T) {
	fp := fpOf(7)
	pins := []vault.PinnedPeer{{Fingerprint: fp, Label: "The convener"}}

	hopArm := seenAt(t, fp, 4)
	hopArm.Port = 9001
	deliveryArm := seenAt(t, fp, 37) // deliveryHop = len(roster) + k, so a much higher index
	deliveryArm.Port = 9002

	feed := []discovery.Seen{deliveryArm, hopArm} // delivery first: it is the one that answers in production
	i := 0
	b := browserFunc(func(time.Time) (discovery.Seen, error) {
		if i >= len(feed) {
			return discovery.Seen{}, context.DeadlineExceeded
		}
		s := feed[i]
		i++
		return s, nil
	})

	got := browsePeers(b, pins, 50*time.Millisecond, 4)
	if len(got) != 1 {
		t.Fatalf("a browse for hop 4 returned %d candidates, want exactly 1. Both arms of one "+
			"machine resolved, so the racer would dial both and keep whichever answered first — "+
			"and the delivery arm cannot co-sign or serve a stored contribution", len(got))
	}
	if got[0].Addr != "192.168.1.9:9001" {
		t.Errorf("the browse returned %s, want the hop arm at :9001. It picked the delivery arm, "+
			"which auto-confirms the spoken check and can never serve the contribution this dial "+
			"is for", got[0].Addr)
	}
}
