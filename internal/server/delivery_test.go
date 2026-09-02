package server

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"strings"
	"testing"
	"time"

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

// TestADeliveryMarkerIsPerPartyAndSurvivesARerun — C10's harder half.
//
// *"A delivery round re-run after a mid-round failure leaves exactly one file per party."* S05d's
// deterministic filename covers the RECIPIENT: a second delivery overwrites itself. What that does
// not cover is the convener — a re-run must reach the party that FAILED and skip the ones that
// succeeded, or a crash mid-round re-delivers to everyone.
//
// **Per party, not one file listing them.** Two legs of one round finish in either order, and a
// shared file makes the second write clobber the first — the same shape `RecordSalt` refuses for
// two parties sharing one BEP-44 target.
func TestADeliveryMarkerIsPerPartyAndSurvivesARerun(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	const id = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	a := "11" + strings.Repeat("22", 31)
	b := "33" + strings.Repeat("44", 31)

	// STIMULUS: nothing is recorded before anything is written. Without this, "b is not recorded"
	// below is true of a checker that always says no.
	if wasDelivered(id, a) || wasDelivered(id, b) {
		t.Fatal("setup: a marker exists before anything was delivered")
	}
	if err := markDelivered(id, a); err != nil {
		t.Fatalf("markDelivered: %v", err)
	}
	if !wasDelivered(id, a) {
		t.Error("a party's acknowledgement did not survive being written — a re-run then delivers " +
			"to them again, and C10's 'exactly one file per party' rests on a write nothing checked")
	}
	// **The discriminator.** One party recorded must not record the other, or a mid-round failure
	// at party 3 of 4 marks all four and the re-run reaches nobody.
	if wasDelivered(id, b) {
		t.Error("recording one party's delivery recorded another's; a re-run would skip the party " +
			"that failed, which is the exact case C10 names")
	}
	// Case-insensitively, because a fingerprint is hex and both cases name one party. Two markers
	// for one party would make a re-run deliver twice.
	if !wasDelivered(id, strings.ToUpper(a)) {
		t.Error("the marker is case-sensitive, so the same party in a different case reads as " +
			"undelivered and the round delivers to them twice")
	}
	// Idempotent: recording twice is what a re-run of a successful leg does.
	if err := markDelivered(id, a); err != nil {
		t.Errorf("re-recording a delivered party failed: %v", err)
	}
}

// TestOnlyTheConvenerRunsARound — the round's one refusal, and it is about identity not permission.
//
// D22 makes the convener the hub: it is the only party holding a channel to everyone. A party
// running a round would be dialling peers it has no pin for and no rendezvous with.
//
// **The refusal cannot precede the identity load, and the first draft of this test assumed it
// could.** Knowing whether this machine is the convener requires knowing which machine it is, so
// the order is forced. What the draft did find is that the function panicked on a nil vault rather
// than saying so — a detached caller would have seen a recovered panic and no reason.
func TestOnlyTheConvenerRunsARound(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	s, v := unlockedServer(t)

	// A roster whose convener is somebody else entirely.
	inv := ceremony.Invitation{
		ID:                  "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ConvenerFingerprint: "99" + strings.Repeat("88", 31),
	}
	// STIMULUS: this server really can load an identity, so the refusal below is about the ROLE
	// and not about a machine that cannot say who it is. The first draft asked `unlockedVault()`
	// without unlocking and got nil, which `identity` dereferences — so the "refusal" it observed
	// would have been a panic in the test's own setup.
	if v == nil {
		t.Fatal("setup: the vault did not unlock, so a refusal below says nothing about the role")
	}
	if _, _, err := identity(v); err != nil {
		t.Fatalf("setup: this server cannot load an identity (%v), so a refusal proves nothing", err)
	}
	if _, err := s.runDeliveryRound(context.Background(), v, inv, []byte("%PDF-1.4\n")); err == nil {
		t.Error("a machine that is NOT the convener ran a delivery round — it would dial parties " +
			"it holds no pin for, at a rendezvous it shares with nobody")
	}

	// And the nil-vault case says so instead of panicking: this runs where a recovered panic
	// reaches nobody.
	if _, err := s.runDeliveryRound(context.Background(), nil, inv, []byte("%PDF-1.4\n")); err == nil {
		t.Error("a round with no vault open returned no error")
	}
}

// TestTheDeliveryRearmIsAProcessConcernAndOffByDefault — P08.S05g, and it is a defect this slice
// introduced and then had to gate.
//
// **The measured failure:** the re-arm sweep hung off `adoptVault` reads `~/nib/ceremonies` and
// opens a socket whose DHT node cache lands under `configDir`. A `Server` constructed in a test
// isolates `configDir` and NOT `$HOME`, so the sweep read the developer's real ceremonies and wrote
// into a `t.TempDir()` that was being torn down — *"TempDir RemoveAll cleanup: directory not
// empty"*, in five unrelated tests that never mention delivery.
//
// So arming is a **process** concern, the twin of `DisarmSession`: constructing a `Server` must not
// put a socket on the network. The flag defaults OFF, which is the direction this repo learned the
// hard way — `toolbarStyle` shipped a flag whose default did something surprising.
//
// This asserts both halves. The default-off half is what stops the tree regressing to the measured
// failure; the enable half is what stops the gate becoming a switch nothing ever turns on.
func TestTheDeliveryRearmIsAProcessConcernAndOffByDefault(t *testing.T) {
	var s Server
	if s.deliveryRearm.Load() {
		t.Error("a freshly constructed Server would re-arm on unlock. Constructing a Server puts " +
			"a socket on the network then, which is what wrote a DHT cache into five unrelated " +
			"tests' temp directories.")
	}
	s.EnableDeliveryRearm()
	// STIMULUS: the enable actually flips it. Without this the assertion above is satisfied by a
	// flag nothing can ever set, which is a gate with no caller.
	if !s.deliveryRearm.Load() {
		t.Error("EnableDeliveryRearm did not enable it, so a real Nib process never re-arms and " +
			"a party who restarts is unreachable for the rest of the ceremony")
	}
}

// TestTheRearmSweepSkipsWhatItMust — the three refusals, driven with an isolated HOME.
//
// The sweep must not arm for: a ceremony this machine has already received (or every unlock holds
// a listener open for proceedings finished months ago — D29's residue through a new door), a
// proceeding that has ended, or one where this machine is the CONVENER (which delivers rather than
// waiting to be delivered to).
//
// Driven at the sweep rather than at its parts, because the defect this guards is the sweep
// arming when it should not — a fact about the composition, not about any one predicate.
func TestTheRearmSweepSkipsWhatItMust(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// STIMULUS: with an isolated HOME there are no stored ceremonies at all, so the sweep has
	// nothing to arm and must return without touching the network. Without this line the sweep
	// reads the developer's real ~/nib, which is exactly the defect that made this gate necessary.
	if got, err := ceremony.ListStored(defaultOutputDir(), time.Now()); err != nil || len(got) != 0 {
		t.Fatalf("setup: an isolated HOME still lists %d stored ceremon(ies) (err=%v) — this test "+
			"is reading a real directory and proves nothing", len(got), err)
	}

	s := &Server{configDir: t.TempDir()}
	s.EnableDeliveryRearm()
	// It must return, not panic and not block, with no vault: the sweep runs on a goroutine with
	// no response to write into, so anything other than a quiet return is invisible in production.
	done := make(chan struct{})
	go func() { defer close(done); s.rearmDeliveries(nil) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("rearmDeliveries did not return on a machine with no ceremonies — it runs at " +
			"every unlock, so a hang here is a hang on the unlock path")
	}
	if s.sess.armedLocked() {
		t.Error("the sweep armed something on a machine with no stored ceremonies at all")
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
