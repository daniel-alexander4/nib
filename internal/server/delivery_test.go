package server

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"strings"
	"testing"

	"nib/internal/ceremony"
)

// TestTheUnattendedGatesHaveOneDoor — P08.S05d's clause 2, and the reason it is a guard and not a
// comment.
//
// `runVerification` refuses a nil `Verifier` outright, and its own doc says why: *"a nil Verifier
// is not 'skip the check' — it is a caller that forgot, and the whole of L2 is that no path
// reaches the exchange unconfirmed."* An auto-confirming verifier is that forgotten gate made
// legitimate by SCOPE alone. So the scope has to be checked, and "only the delivery path uses it"
// is a claim of ABSENCE — which cannot be settled by looking at the delivery path.
//
// It asserts ROUTING: every production reference to either auto type is inside `deliverOneLeg`.
// A tenth call site added later fails here whatever it looks like, which is what comparing the two
// known sites for agreement could never do (ADR-009).
//
// **The guard would have been vacuous if this slice had shipped only the types.** The round that
// uses them is S05g, so a guard written against "the delivery path" today would have asserted a
// property of zero production sites — the vacuous green this repo keeps finding in its own guards.
// `deliverOneLeg` exists in this slice so the door is real now.
func TestTheUnattendedGatesHaveOneDoor(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi fs.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	const door = "deliverOneLeg"
	gates := map[string]bool{"autoVerifier": true, "autoAccepter": true}

	var outside []string
	var insideDoor int
	var sawDoor bool
	for _, pkg := range pkgs {
		for path, file := range pkg.Files {
			for _, d := range file.Decls {
				fn, ok := d.(*ast.FuncDecl)
				if !ok || fn.Body == nil {
					continue
				}
				// The types' own methods are their definition, not a use of them.
				if fn.Recv != nil && len(fn.Recv.List) == 1 {
					if id, ok := fn.Recv.List[0].Type.(*ast.Ident); ok && gates[id.Name] {
						continue
					}
				}
				if fn.Name.Name == door {
					sawDoor = true
				}
				ast.Inspect(fn.Body, func(n ast.Node) bool {
					id, ok := n.(*ast.Ident)
					if !ok || !gates[id.Name] {
						return true
					}
					if fn.Name.Name == door {
						insideDoor++
						return true
					}
					pos := fset.Position(id.Pos())
					outside = append(outside, fn.Name.Name+" ("+path+":"+itoa(pos.Line)+")")
					return true
				})
			}
		}
	}
	// STIMULUS, two directions. Without the first, "no site outside the door" is true of a scan
	// that found no sites at all; without the second, it is true of a package where the door was
	// renamed and every reference therefore counted as outside.
	if !sawDoor {
		t.Fatalf("setup: %s not found in this package — the guard is pinned to a door that no "+
			"longer exists, and an empty violation list would prove nothing", door)
	}
	if insideDoor == 0 {
		t.Fatalf("setup: no reference to the unattended gates inside %s, so this scan is not "+
			"seeing them and its clean result means nothing", door)
	}
	if len(outside) > 0 {
		t.Errorf("the unattended gates are referenced outside %s: %v.\n"+
			"An auto-confirming Verifier reachable from an interactive arm removes the spoken "+
			"check from the path L2 is about — the exact failure runVerification's nil check "+
			"exists to make impossible. If a second delivery site is genuinely needed, route it "+
			"through the door rather than constructing the gates again.", door, outside)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// TestADeliveredNameIsDeterministicAndDoesNotCollideWithinASecond — P08.S05d clause 4.
//
// `receivedName` reads `time.Now()` inside itself at second granularity, so two documents from one
// peer inside a second collide and the second silently overwrites the first (/pending 342, measured
// at P08.S05a's first honest tier-6 run). A delivery round hands one machine several documents in
// quick succession, which is precisely that window.
func TestADeliveredNameIsDeterministicAndDoesNotCollideWithinASecond(t *testing.T) {
	a := ceremony.Record{ID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Intent: "Office lease 2026"}
	b := ceremony.Record{ID: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Intent: "Office lease 2026"}

	// Deterministic: the same record names the same file, however often it is asked.
	if deliveredName(a) != deliveredName(a) {
		t.Fatal("deliveredName is not deterministic for one record")
	}
	// STIMULUS: the name is not a constant. Without this, every assertion here is satisfied by a
	// builder that returns "x.pdf".
	if deliveredName(a) == "" || !strings.HasSuffix(deliveredName(a), ".pdf") {
		t.Fatalf("setup: deliveredName produced %q, which is not a filename", deliveredName(a))
	}
	// **The collision case, and it is the one `receivedName` fails.** Same intent, same instant,
	// different ceremony: two documents a round can deliver to one machine inside a second.
	if deliveredName(a) == deliveredName(b) {
		t.Errorf("two ceremonies with the same intent produce one filename (%q) — the second "+
			"delivery silently overwrites the first, which is /pending 342 one layer up",
			deliveredName(a))
	}
	// The human half survives, or the name is an id nobody can scan.
	if !strings.Contains(deliveredName(a), "office-lease-2026") {
		t.Errorf("deliveredName(%q) = %q and carries no readable half — a user cannot tell this "+
			"from Monday's copy without opening it", a.Intent, deliveredName(a))
	}
	// And the id survives in full: a truncated id is a collision nobody can see.
	if !strings.Contains(deliveredName(a), a.ID) {
		t.Errorf("deliveredName dropped or truncated the ceremony id: %q", deliveredName(a))
	}
	// An empty intent still yields a usable name rather than a bare dash.
	empty := ceremony.Record{ID: a.ID}
	if got := deliveredName(empty); !strings.HasPrefix(got, "ceremony-") {
		t.Errorf("a record with no intent produced %q; it must still name a file", got)
	}
}

// TestTwoDocumentsFromOnePeerInOneSecondDoNotCollide — /pending 342, closed here.
//
// **This was measured, not argued.** `ceremonyrepro.sh`'s two transfer legs run back to back and
// both landed at `incoming/alice-20260831-110425.pdf`: `receivedName` read the clock inside itself
// at one-second granularity, and `saveReceived` writes with `atomicfile.WriteDurable`, which
// renames over whatever is there. The second document destroyed the first — after the sender had
// been told `ackOK`, so neither side had any way to know.
//
// The item's coordinate was P08.S05d, and this slice's own naming rule is the reason it was found
// again: clause 4 says the DELIVERED name must not reuse this builder. Fixing the sibling while
// leaving the original destroying documents is not a defensible reading of that clause.
func TestTwoDocumentsFromOnePeerInOneSecondDoNotCollide(t *testing.T) {
	fp := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16,
		17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	first := []byte("%PDF-1.4\nthe lease")
	second := []byte("%PDF-1.4\nthe amendment")

	// Same peer, same instant, different documents — the exact shape the probe caught.
	a := receivedName("alice", fp, first)
	b := receivedName("alice", fp, second)
	// STIMULUS: the builder is really producing names. Without this the inequality below is
	// satisfied by two empty strings.
	if a == "" || !strings.HasSuffix(a, ".pdf") {
		t.Fatalf("setup: receivedName produced %q, which is not a filename", a)
	}
	if a == b {
		t.Errorf("two different documents from one peer in the same second share the name %q. "+
			"atomicfile.WriteDurable renames over what is there, so the second destroys the "+
			"first — and the sender has already been told ackOK.", a)
	}

	// And the direction that must NOT change: the same document re-sent keeps its name, so a
	// retry overwrites itself with identical bytes instead of accumulating copies.
	if receivedName("alice", fp, first) != a {
		t.Error("the same document from the same peer produced two names in one second; a resend " +
			"then leaves two copies and the user cannot tell which is current")
	}

	// The human half survives — the name is still something a person can scan.
	if !strings.HasPrefix(a, "alice-") {
		t.Errorf("receivedName = %q and no longer leads with the peer, so a directory listing "+
			"stops being readable", a)
	}
}

// TestAnUnwiredDeliveryAccepterFailsClosed — the gate's own nil check.
//
// `autoAccepter` says yes for a living, so the one thing it must never do is say yes when it has
// nothing to check with. This is `runVerification`'s nil-Verifier rule applied to the other gate:
// a caller that forgot is not a caller that meant to skip.
func TestAnUnwiredDeliveryAccepterFailsClosed(t *testing.T) {
	for _, c := range []struct {
		name string
		a    autoAccepter
	}{
		{"no verify and no save", autoAccepter{}},
		{"save but no verify", autoAccepter{save: func([]byte) error { return nil }}},
		{"verify but no save", autoAccepter{verify: func([]byte) error { return nil }}},
	} {
		ok, err := c.a.Accept(nil, []byte("%PDF-1.4\n"))
		if ok {
			t.Errorf("%s: an unwired delivery accepter ACCEPTED, so the peer is told its "+
				"document was kept by a gate that checked nothing and saved nothing", c.name)
		}
		if err == nil {
			t.Errorf("%s: it declined with no reason, which reads to the sender as the user "+
				"saying no rather than as this machine being misconfigured", c.name)
		}
	}
	// STIMULUS: a fully wired accepter DOES accept. Without it every assertion above is satisfied
	// by an Accept that refuses everything.
	wired := autoAccepter{
		verify: func([]byte) error { return nil },
		save:   func([]byte) error { return nil },
	}
	if ok, err := wired.Accept(nil, []byte("%PDF-1.4\n")); !ok || err != nil {
		t.Fatalf("setup: a wired accepter refused (ok=%v err=%v), so the refusals above cannot "+
			"be distinguished from a gate that never accepts", ok, err)
	}
}

// TestTheDeliveryAcceptGateChecksBeforeItSaves — the ordering inside the gate.
//
// P08.S05a made `ackOK` mean "the bytes are on disk". A gate that saved first and verified after
// would keep a document it then refused; one that accepted before verifying would acknowledge a
// document it never checked. Both orderings are wrong in the same direction, and neither shows up
// in a test that only asks whether a good document is accepted.
func TestTheDeliveryAcceptGateChecksBeforeItSaves(t *testing.T) {
	var order []string
	a := autoAccepter{
		verify: func([]byte) error { order = append(order, "verify"); return errRefuse },
		save:   func([]byte) error { order = append(order, "save"); return nil },
	}
	ok, err := a.Accept(nil, []byte("%PDF-1.4\n"))
	if ok || err == nil {
		t.Fatalf("a document its own verification refused was accepted (ok=%v err=%v)", ok, err)
	}
	if len(order) != 1 || order[0] != "verify" {
		t.Errorf("the gate ran %v; it must verify first and not save at all when that fails — "+
			"otherwise a refused document is still on disk and the sender is told it was not kept",
			order)
	}
}

var errRefuse = &refuseErr{}

type refuseErr struct{}

func (*refuseErr) Error() string { return "refused for the test" }
