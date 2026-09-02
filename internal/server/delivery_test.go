package server

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"strings"
	"testing"
	"time"

	"nib/internal/ceremony"
	"nib/internal/sign"
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

// TestTheDeliveryArmsWindowIsEnforcedAndNotJustReported — this slice's own review found it.
//
// `armIn` stamps `until` from `armWindowFor(armDelivery, cer)` and `status()` shows it, so the
// window LOOKED enforced from every surface that reports it. Nothing fired on it: the arm looped
// on `Accept` until something else disarmed it, so a delivery arm lived until the process exited
// and the TRIPWIRE's *"how long it stays open"* paragraph described a bound the code did not keep.
//
// **Asserted through `armWindowFor`, not through the arm goroutine**, because the property is that
// the reported figure and the enforced one are the SAME figure — which is exactly what
// `runSession`'s equivalent guard asserts, and exactly what "reported but not enforced" breaks.
// A test that armed and waited would take the window's length to fail.
func TestTheDeliveryArmsWindowIsEnforcedAndNotJustReported(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	// STIMULUS: the two kinds really produce different windows, or "the delivery arm uses its own"
	// is true of a function that ignores its argument.
	interactive := armWindowFor(armInteractive, nil)
	delivery := armWindowFor(armDelivery, nil)
	if interactive != sessionAcceptTimeout {
		t.Fatalf("setup: a manual interactive arm's window is %s, want %s", interactive, sessionAcceptTimeout)
	}
	if delivery <= 0 {
		t.Fatalf("a delivery arm's window is %s — a non-positive window closes the listener "+
			"immediately and the round can never reach this party", delivery)
	}

	if delivery != sessionAcceptTimeout {
		t.Errorf("with no ceremony the delivery window is %s, want the interactive floor %s",
			delivery, sessionAcceptTimeout)
	}

	// ── The UNREADABLE-RECORD branch, which is a different path from the nil one ─────────────
	//
	// **The first cut of this test asserted the floor using `armWindowFor(armDelivery, nil)` and
	// called that the unreadable-record case.** It is not: `deliveryWindowFor` returns the floor
	// for a nil ceremony BEFORE it ever reads a mirror, so a mutation making the read's failure
	// path return `MaxCeremonyLife` left this test green. Found by the mutation pass, which is
	// what it is for. A real ceremonyID with no mirror on disk is what exercises the branch.
	unreadable := &ceremonyID{inv: ceremony.Invitation{ID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}
	got := armWindowFor(armDelivery, unreadable)
	if got != sessionAcceptTimeout {
		t.Errorf("a delivery arm whose record cannot be read got a %s window, want the "+
			"interactive floor %s. Defaulting LONG on the input that tells us least holds this "+
			"machine's one network-reachable surface open for a proceeding nothing can confirm "+
			"is live.", got, sessionAcceptTimeout)
	}
	if got >= ceremony.MaxCeremonyLife {
		t.Errorf("an unreadable record yielded %s, at or beyond MaxCeremonyLife", got)
	}

	// And the source is one door: the arm goroutine must take its expiry from `armWindowFor`
	// rather than computing a second figure, or the status reports one number and the listener
	// closes on another — the drift that door exists to make impossible.
	src, err := os.ReadFile("delivery.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	if !strings.Contains(body, "window := armWindowFor(armDelivery, cer)") {
		t.Error("armForDelivery does not take its window from armWindowFor — the figure status " +
			"reports and the figure the listener closes on can then diverge silently")
	}
	if !strings.Contains(body, "time.AfterFunc(window,") {
		t.Error("nothing fires on the delivery arm's window. It is stamped on the arm and shown " +
			"in status, so it LOOKS enforced from every surface that reports it; without a timer " +
			"the arm lives until the process exits and the TRIPWIRE's 'how long it stays open' " +
			"describes a bound the code does not keep.")
	}
}

// TestATerminationIsToldNotSaved — P08.S05e's routing, and the shape test that decides it.
//
// A delivery round carries a finished DOCUMENT when a ceremony completed and a signed TERMINATION
// when it was declined — same walk, same rendezvous, same acknowledgement markers. The receiving
// side tells them apart by shape rather than by a discriminator byte, because the payload already
// answers the question and adding one would be a format change for nothing.
//
// **The direction that matters is the false positive.** A document misread as a termination is
// never saved, so the party loses the document they signed; the assertions below therefore lead
// with a real PDF and require it NOT to be taken for an attestation.
func TestATerminationIsToldNotSaved(t *testing.T) {
	// A PDF must never read as a termination — the losing direction.
	if _, ok := asTermination([]byte("%PDF-1.4\n1 0 obj\n")); ok {
		t.Error("a PDF was read as a termination object. The receiving side then TELLS instead of " +
			"SAVING, and the party loses the finished document they signed.")
	}
	// Nor must arbitrary JSON that is not one.
	for _, bad := range [][]byte{
		[]byte(`{}`),
		[]byte(`{"ceremony":"a"}`),                    // no state, no signature
		[]byte(`{"state":"declined","sig":"x"}`),      // no ceremony
		[]byte(`{"ceremony":"a","state":"declined"}`), // unsigned
	} {
		if _, ok := asTermination(bad); ok {
			t.Errorf("%s was read as a termination; an unsigned or partial object must not be, "+
				"or a peer can end a proceeding by sending a JSON literal", bad)
		}
	}
	// STIMULUS: a well-formed one IS recognised. Without this every assertion above is satisfied
	// by a function that always says no, and the declined path would silently never fire.
	good := ceremony.Termination{
		Version: 1, Ceremony: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		RosterHash: "bb", State: ceremony.StateDeclined, ConvenerCert: "cc", Sig: "dd",
	}
	b, err := json.Marshal(good)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := asTermination(b)
	if !ok {
		t.Fatal("a well-formed termination was not recognised, so the declined path is dead")
	}
	if got.State != ceremony.StateDeclined || got.Ceremony != good.Ceremony {
		t.Errorf("the recognised termination lost its fields: %+v", got)
	}
}

// TestTheEndStateTellingSaysAllFourThings — C06's telling half, asserted clause by clause.
//
// The criterion names four things a party who already signed is owed: that it is over, who ended
// it, that their signature STANDS, and that a re-run starts from the original unsigned file. They
// are asserted separately because a summary that covers three of four reads as complete — and the
// third is the one a person actually needs, since they are holding a signed document and do not
// know what it is now worth.
func TestTheEndStateTellingSaysAllFourThings(t *testing.T) {
	srv := &Server{}
	srv.tellEndState(&ceremonyID{}, ceremony.Termination{State: ceremony.StateDeclined})

	st := srv.sess.status()
	if st.Notice == nil {
		t.Fatal("a declined proceeding told the party nothing at all — a delivery arm has no " +
			"response to write into, so the sticky notice is the only channel there is")
	}
	body := st.Notice.Summary + " " + st.Notice.Detail
	for _, c := range []struct{ name, want string }{
		{"it is over", "over"},
		{"who ended it", "convener"},
		{"their signature stands", "signature stands"},
		{"a re-run starts from the original unsigned file", "ORIGINAL unsigned file"},
	} {
		if !strings.Contains(body, c.want) {
			t.Errorf("the telling does not say %s (looked for %q). C06 names four things and a "+
				"telling that covers three reads as complete.", c.name, c.want)
		}
	}
	// And the kind is branchable: a surface has to distinguish declined from completed without
	// parsing prose.
	if st.Notice.What != "ceremony-declined" {
		t.Errorf("the notice kind is %q, so a surface cannot branch on it", st.Notice.What)
	}
}

// TestCancelDoesNotTearDownTheDeliveryArm — a defect P08.S05c introduced and P08.S05g's live run
// surfaced.
//
// `/api/session/disarm` is a person pressing Cancel on the co-signing session THEY opened.
// `DisarmSession` is the process exiting. S05c made `disarm()` empty every slot — right for the
// second, and for the first it silently stopped a party receiving their own copy of a document
// they had already signed. Nothing tells them: a delivery arm has no surface yet (/pending 353).
//
// Both directions are asserted. Cancel must spare the delivery arm, and shutdown must still take
// it — a fix that simply stopped tearing anything down would satisfy the first and reintroduce the
// orphaned-ceremony leak `TestAnUnconditionalDisarmTearsDownEverySlot` exists for.
func TestCancelDoesNotTearDownTheDeliveryArm(t *testing.T) {
	var se session
	if !se.arm(&stubListener{}, nil) {
		t.Fatal("setup: the interactive arm was refused")
	}
	delivery := &ceremonyID{}
	if !se.armDeliveryForCeremony(delivery, "127.0.0.1:9", func() {}) {
		t.Fatal("setup: the delivery arm was refused, so there is nothing for Cancel to spare")
	}

	se.disarmKind(armInteractive)

	se.mu.Lock()
	iv, dv := se.arms[armInteractive], se.arms[armDelivery]
	se.mu.Unlock()
	if iv != nil {
		t.Error("Cancel did not tear down the interactive arm it was pressed on")
	}
	if dv == nil {
		t.Fatal("Cancel tore down the DELIVERY arm. The user cancelled the co-signing session " +
			"they opened; they did not ask to stop receiving their copy of a document they have " +
			"already signed, and nothing would have told them it had stopped.")
	}
	if dv.cer != delivery {
		t.Errorf("the delivery slot holds %+v, not the ceremony it armed for", dv)
	}

	// STIMULUS and the other direction: shutdown still takes everything. Without this, a fix that
	// disarmed nothing at all would pass every assertion above.
	se.disarm()
	se.mu.Lock()
	stillArmed := se.armedLocked()
	se.mu.Unlock()
	if stillArmed {
		t.Error("shutdown left something armed — the orphaned-ceremony leak is back")
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
	rec := ceremony.Record{
		ID:     "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Roster: []ceremony.Party{{Fingerprint: "99" + strings.Repeat("88", 31), Signs: false}},
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
	if _, err := s.runDeliveryRound(context.Background(), v, rec, []byte("%PDF-1.4\n"), nil); err == nil {
		t.Error("a machine that is NOT the convener ran a delivery round — it would dial parties " +
			"it holds no pin for, at a rendezvous it shares with nobody")
	}

	// And the nil-vault case says so instead of panicking: this runs where a recovered panic
	// reaches nobody.
	if _, err := s.runDeliveryRound(context.Background(), nil, rec, []byte("%PDF-1.4\n"), nil); err == nil {
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

// TestTheEnderIsRecordedOncePerCeremony — P08.S05e's marker, and the defect
// this slice introduced by routing a decline through the round at all.
//
// **Named for what it checks, not for the rule it serves.** It never constructs a round; the walk's
// decision is its sibling's, and a name claiming otherwise is the over-claim this repo grades as a
// finding against the test.
//
// **The leg is impossible at BOTH ends, which is why skipping it is not an optimisation.**
// `declineCeremony` prunes this ceremony's stored invitation on the refusing machine, so it can
// never arm a delivery rendezvous for the proceeding again; and a party that refused at its
// consent gate never wrote a mirror, so `checkDeliveredPayload`'s `ReadMirror` would refuse the
// attestation even if one arrived. Measured at tier 4 on 2026-09-02: the round spent the whole
// 300 s `connectDeadline` on that party and reported `tried 0 address(es)` — on that round and on
// every re-run of it.
//
// Driven at the marker rather than through a round, because a round dials: what is under test is
// the WALK's decision, and `runDeliveryRound` reaches the network for any party it does not skip.
func TestTheEnderIsRecordedOncePerCeremony(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	const id = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	// **Hex LETTERS, not digits.** The first draft used "11"+"22"… and its case clause was
	// vacuous: `strings.ToUpper` of an all-digit fingerprint is the same string, so removing the
	// fold under test left it green. Found by the mutation, not by reading it.
	decliner := "3c" + strings.Repeat("4d", 31)

	// STIMULUS: nothing is recorded before anything is written, so "the signer is not the ender"
	// below is not true of a reader that always answers "".
	if got := endedBy(id); got != "" {
		t.Fatalf("setup: a proceeding nobody ended reports %q as its ender", got)
	}
	if err := markEndedBy(id, decliner); err != nil {
		t.Fatalf("markEndedBy: %v", err)
	}
	if got := endedBy(id); got != decliner {
		t.Errorf("the party that ended the proceeding did not survive being written: got %q, want %q — "+
			"the round then walks them and spends its whole connect deadline reaching nobody", got, decliner)
	}
	// **There is deliberately no "does it name a different party?" clause here.** `endedByPath`
	// keys on the ceremony id ALONE, so `endedBy` has no per-party dimension to bleed across and
	// such a check cannot fail — it was copied from the delivered-marker test above, where the
	// path DOES include the party and the same clause has real bite. The property it was reaching
	// for lives at the walk and is asserted there.
	// **Write-once, first ender wins**, which is `ceremony.WriteTermination`'s rule. Nothing on
	// the arrival path refuses a hop because a proceeding has already ended, so a convener can
	// collect a SECOND decline; overwriting would move the marker to the newer decliner and the
	// round would go back to walking the first one for the full connect deadline.
	second := "7a" + strings.Repeat("8b", 31)
	if err := markEndedBy(id, second); err == nil {
		t.Error("a second, different party was recorded as having ended the same proceeding — a " +
			"proceeding ends once, and moving the marker restores the impossible leg this rule " +
			"exists to remove, now naming the wrong party in the round's report")
	}
	if got := endedBy(id); got != decliner {
		t.Errorf("the recorded ender moved to %q after a second decline; want the first, %q", got, decliner)
	}
	// Idempotent for the SAME party, case-insensitively: recording one decline twice is what a
	// retry does, and it must not read as a conflict.
	if err := markEndedBy(id, strings.ToUpper(decliner)); err != nil {
		t.Errorf("re-recording the same ender in a different case was refused as a conflict: %v", err)
	}
	// **There is no case-fold assertion on `endedBy` itself, deliberately.** Its one consumer
	// compares with `strings.EqualFold`, so a fold at the read changes no production behaviour —
	// asserting it here would pin a property nothing depends on and call that coverage. The
	// case rule is real and it lives at the walk, where its sibling test drives it.
	// And a proceeding that ended by COMPLETING has no ender, so its round walks every party.
	const other = "cccccccccccccccccccccccccccccccc"
	if got := endedBy(other); got != "" {
		t.Errorf("a different ceremony reports %q as its ender — the marker is not per-ceremony, "+
			"so one declined proceeding would silence a leg of another", got)
	}
}

// TestTheRoundReportsTheEnderSkippedRatherThanDialling — the rule at the WALK, which the marker
// test above cannot see.
//
// Its sibling drives `markEndedBy`/`endedBy` and would stay green with the skip clause deleted
// from `runDeliveryRound` entirely. This one drives the decision.
//
// **The roster carries THREE parties, and each one is load-bearing.** A two-party roster (the
// convener plus the ender) cannot tell "skip the ender" from "skip everybody": weakening the
// clause to a bare `if ender != ""` left that shape green, which is the worst outcome the rule
// has — the round would skip the parties C06's telling is owed to and report success. So:
//
//   - the ENDER, whose leg is impossible at both ends and must be skipped with a reason;
//   - a SIGNER who did not end it, who must NOT be skipped — the discriminator;
//   - a party ALREADY ACKNOWLEDGED, which is the other kind of skip, and the pair is what
//     `deliveryOutcome.Skipped`'s doc claims `Delivered` distinguishes. Two shapes sharing one
//     field is a claim, and nothing checked it.
//
// **The ender's roster entry is UPPERCASE hex.** `endedBy` returns lowercase, so with both sides
// lowercase the walk's `strings.EqualFold` could be `==` and this test would not notice. A roster
// fingerprint is not normalised anywhere on the record path, so the mixed case is realistic and
// not a contrivance.
//
// The context carries a deadline so a regression is a failed assertion rather than a hung test.
// **It is not what keeps this test fast, and an earlier comment here claimed it was.** The fixture
// vault holds no `CeremonySecret` for these parties, so an unskipped leg stops at
// `convenerInvitationFor` and never reaches a dial — which is also why the signer's expected
// outcome below is a secret-miss reason and not a connect failure.
func TestTheRoundReportsTheEnderSkippedRatherThanDialling(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	s, v := unlockedServer(t)
	cert, _, err := identity(v)
	if err != nil {
		t.Fatalf("setup: no identity, so this machine cannot be the convener: %v", err)
	}
	fp, err := sign.Fingerprint(cert)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	me := hex.EncodeToString(fp)
	decliner := "3c" + strings.Repeat("4d", 31) // letters, per the sibling test's note
	signer := "1a" + strings.Repeat("2b", 31)
	acked := "5e" + strings.Repeat("6f", 31)
	const id = "dddddddddddddddddddddddddddddddd"
	rec := ceremony.Record{
		ID:           id,
		ConvenerCert: string(cert),
		Roster: []ceremony.Party{
			{Fingerprint: me, Label: "convener", Signs: false},
			{Fingerprint: strings.ToUpper(decliner), Label: "ender", Signs: true},
			{Fingerprint: signer, Label: "signer", Signs: true},
			{Fingerprint: acked, Label: "acked", Signs: true},
		},
	}
	// STIMULUS: this machine really is the convener of this record, or the round would refuse for
	// a reason that has nothing to do with the ender.
	if !strings.EqualFold(convenerFingerprintOf(rec), me) {
		t.Fatal("setup: the record does not name this machine as convener, so the round would " +
			"refuse for a reason that has nothing to do with the ender")
	}
	if err := markEndedBy(id, decliner); err != nil {
		t.Fatalf("markEndedBy: %v", err)
	}
	if err := markDelivered(id, acked); err != nil {
		t.Fatalf("markDelivered: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out, rerr := s.runDeliveryRound(ctx, v, rec, []byte("%PDF-1.4\n"), nil)
	if rerr != nil {
		t.Fatalf("the round refused outright: %v", rerr)
	}
	if len(out) != 3 {
		t.Fatalf("the round reported %d outcomes for a roster of the convener plus three parties, want 3", len(out))
	}
	by := map[string]deliveryOutcome{}
	for _, o := range out {
		by[strings.ToLower(o.Fingerprint)] = o
	}

	ender, ok := by[decliner]
	if !ok {
		t.Fatalf("the round's report names no outcome for the party that ended the proceeding; it reported %v", out)
	}
	if ender.Delivered {
		t.Error("the round reports the party that ended the proceeding as DELIVERED — that " +
			"machine pruned this ceremony's invitation when it declined and holds no mirror to " +
			"check an attestation against, so nothing could have arrived")
	}
	if !ender.Skipped {
		t.Errorf("the round did not skip the party that ended the proceeding; it reported %q. "+
			"The leg is impossible at both ends, so attempting it spends the whole connectDeadline "+
			"(300 s) on this round and on every re-run of it.", ender.Reason)
	}
	if !strings.Contains(ender.Reason, "ended the proceeding") {
		t.Errorf("the skip's reason is %q, which does not say why the party was not reached — a "+
			"user reading a round's report cannot tell an impossible leg from a failed one", ender.Reason)
	}

	// **The discriminator.** Skipping the ender must not skip everyone: this party signed and is
	// owed the telling, so the round has to ATTEMPT their leg. Without this clause a walk that
	// skips every party whenever anyone ended the proceeding passes every assertion above —
	// measured, by weakening the clause to `if ender != ""`.
	sig, ok := by[signer]
	if !ok {
		t.Fatalf("the round's report names no outcome for a party who SIGNED; it reported %v", out)
	}
	if sig.Skipped {
		t.Errorf("the round skipped a party who signed and did not end the proceeding, reporting "+
			"%q — C06's telling is owed to exactly these parties, and a round that skips them "+
			"reports success having delivered to nobody", sig.Reason)
	}
	if strings.Contains(sig.Reason, "ended the proceeding") {
		t.Errorf("a party who did NOT end the proceeding is reported as though they had: %q", sig.Reason)
	}
	if sig.Reason == "" {
		t.Error("a party the round attempted and did not reach carries no reason at all, so the " +
			"convener cannot tell an attempted leg from one that was never tried")
	}

	// **The OTHER kind of skip, and the claim that they are distinguishable.**
	// `deliveryOutcome.Skipped`'s doc says `Delivered` is what tells them apart; nothing checked
	// it until this clause, and two shapes silently sharing one field is how a re-run's report
	// starts meaning something different from what its reader believes.
	ack, ok := by[acked]
	if !ok {
		t.Fatalf("the round's report names no outcome for the already-acknowledged party; it reported %v", out)
	}
	if !ack.Skipped || !ack.Delivered {
		t.Errorf("an already-acknowledged party is reported skipped=%v delivered=%v, want both true — "+
			"a re-run must skip it AND still report it delivered, or C10's 'exactly one file per "+
			"party' reads as a failure on every re-run", ack.Skipped, ack.Delivered)
	}
	if ack.Delivered == ender.Delivered {
		t.Error("the two kinds of skip are indistinguishable: an acknowledged party and the party " +
			"that ended the proceeding report the same Delivered value, which is the field " +
			"deliveryOutcome.Skipped's own doc says tells them apart")
	}
}

// TestTheDeclineBranchRecordsWhoEndedIt — the wiring the two tests above cannot see.
//
// They drive `markEndedBy`/`endedBy` and the walk's use of them. Neither reaches the one site that
// WRITES the marker in production: `endCeremony`, called from `handleSessionInitiate`'s
// `ErrCoSignDeclined` arm. With that call deleted the marker is never written, `endedBy` returns
// "" forever, the walk skips nobody, and every test above stays green — the round goes back to
// spending 300 s on a party it cannot reach and nothing at tier 1 says so.
//
// A source scan rather than a driven test, for the reason `ceremonypin_test.go` gives at the same
// shape: reaching this branch needs a peer that declines over a real session, which is tier 4's
// job. What tier 1 can pin is that the route exists and sits on the DECLINE arm specifically — a
// marker written on every ended proceeding would silence the telling for a COMPLETED one, where
// every party is still reachable and still owed their copy.
func TestTheDeclineBranchRecordsWhoEndedIt(t *testing.T) {
	src, err := os.ReadFile("delivery.go")
	if err != nil {
		t.Fatal(err)
	}
	code := stripLineComments(string(src))
	i := strings.Index(code, "func (s *Server) endCeremony(")
	if i < 0 {
		t.Fatal("cannot find endCeremony — this guard is reading the wrong thing")
	}
	body := funcBodyFrom(code, i)
	if body == "" {
		t.Fatal("could not brace-match endCeremony's body")
	}
	// STIMULUS: the name really appears in this file, so "not found in the body" below is a fact
	// about placement and not about spelling.
	if !strings.Contains(code, "func markEndedBy(") {
		t.Fatal("markEndedBy is not defined in this file, so the scan below looks for a name that " +
			"could not appear and its clean result would mean nothing")
	}
	if !strings.Contains(body, "markEndedBy(") {
		t.Error("endCeremony does not record WHICH party ended the proceeding. The delivery round " +
			"then walks the party that declined, whose machine pruned this ceremony's invitation " +
			"and holds no mirror — a leg that cannot succeed, costing the full connectDeadline " +
			"(300 s) on that round and on every re-run of it.")
	}
	// And it is on the DECLINE arm, not on every end state. A completed ceremony's parties are all
	// still reachable and all still owed their copy; a marker written there would skip one.
	if !strings.Contains(body, "ceremony.StateDeclined") {
		t.Error("endCeremony records the ender without discriminating on the end state — a " +
			"COMPLETED proceeding would then skip a party who is reachable and owed their document")
	}
	// The caller: a decline reaches endCeremony at all. Without this the function could be as
	// correct as it likes and never run.
	sess, err := os.ReadFile("session.go")
	if err != nil {
		t.Fatal(err)
	}
	scode := stripLineComments(string(sess))
	at := strings.Index(scode, "ErrCoSignDeclined)")
	if at < 0 {
		t.Fatal("session.go no longer tests for ErrCoSignDeclined, so this guard cannot find the " +
			"branch it exists to pin")
	}
	// Bounded to the arm rather than the file: an endCeremony call anywhere in session.go would
	// satisfy an unscoped search, including one on a path that has nothing to do with a decline.
	arm := scode[at:]
	if n := strings.Index(arm, "\n\t\t}"); n > 0 {
		arm = arm[:n]
	}
	if !strings.Contains(arm, "endCeremony(") {
		t.Error("the ErrCoSignDeclined arm does not end the ceremony, so nothing anywhere decides " +
			"that a declined proceeding is over — which is the state this slice exists to produce")
	}
}

// TestAnEndStateIsCheckedAgainstTheInvitationNotTheFileBesideIt — P08.S05e, and the rule this
// slice's first cut of `checkDeliveredPayload` broke.
//
// `ReadTermination`'s doc states it: *"`rec` must come from the document or the invitation, never
// from the `record.json` beside it. A planted pair — a matching record and termination for another
// proceeding, dropped into this directory — verifies perfectly against itself."* `LoadState` takes
// the weaker anchor deliberately and says why it may: it renders a word and authorises nothing. A
// gate that REFUSES may not.
//
// `checkDeliveredPayload` is such a gate, and it was verifying an arriving attestation against the
// `record.json` sitting in the very directory it was about to write into. The consequence is
// durable and silent: a false `Ended` makes `rearmDeliveries` skip the ceremony forever
// (`st.Ended != ""`), so the party never receives its real copy and nothing tells them why.
//
// Driven at the ROUTING rather than with a forged pair, for the same reason the sibling structural
// guards give: minting a convener-signed termination for a second proceeding needs a second
// identity and a second record, which is tier 4's shape. What tier 1 can hold is that the gate
// takes the invitation anchor at all — the property whose absence was the defect.
func TestAnEndStateIsCheckedAgainstTheInvitationNotTheFileBesideIt(t *testing.T) {
	src, err := os.ReadFile("delivery.go")
	if err != nil {
		t.Fatal(err)
	}
	code := stripLineComments(string(src))
	i := strings.Index(code, "func (s *Server) checkDeliveredPayload(")
	if i < 0 {
		t.Fatal("cannot find checkDeliveredPayload — this guard is reading the wrong thing")
	}
	body := funcBodyFrom(code, i)
	if body == "" {
		t.Fatal("could not brace-match checkDeliveredPayload's body")
	}
	// STIMULUS: the anchor's name really appears in this file, so "not found in the body" is a
	// fact about this gate and not about spelling. Its sibling `checkDelivered` uses it.
	if !strings.Contains(code, "MatchesRecord(") {
		t.Fatal("MatchesRecord appears nowhere in delivery.go, so the scan below looks for a name " +
			"that could not appear and its clean result would mean nothing")
	}
	bind := strings.Index(body, "MatchesRecord(")
	if bind < 0 {
		t.Fatal("checkDeliveredPayload does not bind the record to the invitation before " +
			"anchoring an end state on it. A matching record-and-termination pair planted in " +
			"~/nib/ceremonies/<id>/ verifies against itself, and the false Ended it writes makes " +
			"rearmDeliveries skip this ceremony forever — the party never gets their real copy.")
	}
	// And it binds BEFORE it verifies, or the anchor is checked after the thing it anchors.
	verify := strings.Index(body, ".Verify(")
	if verify < 0 {
		t.Fatal("checkDeliveredPayload no longer verifies the end state at all")
	}
	if bind > verify {
		t.Error("checkDeliveredPayload verifies the end state BEFORE binding the record to the " +
			"invitation, so the planted pair is checked against itself first and the binding is " +
			"a second opinion on a question already answered")
	}
	// **And the gate does NOT write.** `autoAccepter` splits the two: `verify` is "the recipient's
	// own check of what arrived", `save` is where "the caller can persist it before the ack". The
	// first cut did both here, which is a check that mutates disk and one that
	// `TestTheDeliveryAcceptGateChecksBeforeItSaves` cannot see past. Asserted as an ABSENCE with
	// its own stimulus below, because an absence is the half a scan gets wrong.
	if strings.Contains(body, "WriteTermination(") {
		t.Error("checkDeliveredPayload stores the end state as well as checking it. Its own gate " +
			"documents verify as the check and save as the persist, and the ordering guard that " +
			"exists to keep those apart cannot see a write that happens inside the check.")
	}
	// Where the write actually is: the save arm of the delivery accepter, beside the document arm
	// that persists there too. Without this the absence above is satisfied by deleting the write.
	i = strings.Index(code, "func (s *Server) deliverOneLeg(")
	if i < 0 {
		t.Fatal("cannot find deliverOneLeg")
	}
	leg := funcBodyFrom(code, i)
	if !strings.Contains(leg, "WriteTermination(") {
		t.Error("nothing on the delivery leg stores an arriving end state, so a party is TOLD the " +
			"proceeding ended and keeps no attestation of it — and `ackOK` then means the bytes " +
			"reached a sentence rather than a disk")
	}
}
