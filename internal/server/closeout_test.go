package server

import (
	"encoding/hex"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nib/internal/ceremony"
	"nib/internal/vault"
)

// P08.S06's close-out, and the one idea every test here is about: **the prune moves.**

// TestTheCloseOutPrunePreservesThisMachinesOwnContribution is the slice's first acceptance bullet.
//
// **The observation is the file's presence at a NAMED path after the prune**, not merely that
// something survived. On every machine but the convener's, `~/nib/ceremonies/<id>/document.pdf` is
// the only place that party's own signed contribution exists — there is no delivery round on a
// declined, expired or abandoned ceremony, so nothing has carried it anywhere — and
// `RemoveMirror`, whose own doc says *"D29's close-out prune is P08.S06's, and this function is
// what it will call"*, is an `os.RemoveAll`.
//
// The SETUP assertion is the load-bearing half: the contribution is read back from the live
// directory first, so what the assertion below compares is a state against a state and not one
// absence against another. Without it, a close-out that wrote nothing anywhere passes.
func TestTheCloseOutPrunePreservesThisMachinesOwnContribution(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := defaultOutputDir()

	rec, _, stored := ceremonyOnDisk(t)

	// SETUP: the contribution really is in the live directory, with the bytes we will look for.
	live, err := ceremony.MirrorDir(root, rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(live, "document.pdf"))
	if err != nil {
		t.Fatalf("setup: no contribution in the live directory (%v) — every assertion below "+
			"would then be comparing one absence against another", err)
	}
	if len(before) != len(stored) {
		t.Fatalf("setup: the mirror holds %d bytes and the contribution is %d — this fixture is "+
			"not the document the assertions are about", len(before), len(stored))
	}

	if err := ceremony.CloseOutMirror(root, rec.ID); err != nil {
		t.Fatalf("close-out: %v", err)
	}

	// The live directory is gone, so the ceremony leaves the listing by construction.
	if _, serr := os.Stat(live); !os.IsNotExist(serr) {
		t.Errorf("the live ceremony directory still exists after close-out (%v) — a close-out "+
			"that leaves it behind puts an ended ceremony back in front of the listing and the "+
			"sweep, forever", serr)
	}

	// And the contribution is at the named path, byte for byte.
	ended, err := ceremony.EndedDir(root, rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(filepath.Join(ended, "document.pdf"))
	if err != nil {
		t.Fatalf("this machine's own signed contribution is GONE after the close-out: %v.\n"+
			"That is the user's signature destroyed by their own software's tidying, which is "+
			"the whole reason the prune is a move", err)
	}
	if string(after) != string(before) {
		t.Errorf("the preserved contribution is %d bytes and the original was %d — a move that "+
			"changes the bytes is not a move", len(after), len(before))
	}
	// The record travels with it, or the preserved document is an orphan PDF nothing can check.
	if _, rerr := os.Stat(filepath.Join(ended, "record.json")); rerr != nil {
		t.Errorf("the record did not travel with the document (%v) — preserved without it, the "+
			"contribution cannot be verified against the ceremony it belongs to", rerr)
	}
}

// TestTheCloseOutRefusesARelativeRootBeforeItMovesAnything is the slice's root bullet.
//
// `defaultOutputDir` returns a bare `"nib"` when `os.UserHomeDir()` fails, and that string reaches
// twenty-four production call sites. Everywhere else it is merely wrong; here it makes the rename
// relative to whatever the process's working directory happens to be.
//
// **The assertion is on the refusal AND on nothing having moved**, because a check placed after
// the destructive call refuses just as loudly and has already done the damage.
func TestTheCloseOutRefusesARelativeRootBeforeItMovesAnything(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	rec, _, _ := ceremonyOnDisk(t)
	live, err := ceremony.MirrorDir(defaultOutputDir(), rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	// SETUP: there is something to lose.
	if _, serr := os.Stat(live); serr != nil {
		t.Fatalf("setup: nothing on disk to move (%v)", serr)
	}

	err = ceremony.CloseOutMirror("nib", rec.ID)
	if !errors.Is(err, ceremony.ErrRootNotAbsolute) {
		t.Errorf("a relative root was not refused: %v", err)
	}
	if _, serr := os.Stat(live); serr != nil {
		t.Errorf("the ceremony directory moved despite the refusal (%v) — the check ran after "+
			"the destructive call, which is a check that reports the damage it failed to "+
			"prevent", serr)
	}
}

// TestASecondCloseOutIsRefusedRatherThanOverwriting.
//
// Two ways to reach it and both want the same answer: a re-run over a ceremony already dealt with,
// where the first move's contents are the ones to keep, and an id collision, where overwriting
// destroys the earlier party's contribution. They are indistinguishable from inside the function.
func TestASecondCloseOutIsRefusedRatherThanOverwriting(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := defaultOutputDir()

	rec, _, _ := ceremonyOnDisk(t)
	if err := ceremony.CloseOutMirror(root, rec.ID); err != nil {
		t.Fatalf("setup: the first close-out failed (%v)", err)
	}
	ended, _ := ceremony.EndedDir(root, rec.ID)
	first, err := os.ReadFile(filepath.Join(ended, "document.pdf"))
	if err != nil {
		t.Fatalf("setup: nothing preserved by the first close-out (%v)", err)
	}

	// A second live directory under the same id, with DIFFERENT bytes, so an overwrite is visible.
	live, _ := ceremony.MirrorDir(root, rec.ID)
	if err := os.MkdirAll(live, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(live, "document.pdf"), []byte("a different party"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := ceremony.CloseOutMirror(root, rec.ID); !errors.Is(err, ceremony.ErrAlreadyClosedOut) {
		t.Errorf("a second close-out over an existing destination was not refused: %v", err)
	}
	again, err := os.ReadFile(filepath.Join(ended, "document.pdf"))
	if err != nil {
		t.Fatalf("the preserved contribution is gone after the refusal: %v", err)
	}
	if string(again) != string(first) {
		t.Errorf("the second close-out overwrote the first's preserved contribution — the case " +
			"this refusal exists for")
	}
}

// TestTheCloseOutDoorReachesEveryCeremonyScopedStore is the four-store bullet.
//
// **FOUR, and the count is what the slice's deepdive corrected.** P08.S06's scope said three —
// pins, secrets and the mirror — and `unconvene` already took four: the invitee-side stored
// invitation is the fourth, added at P08.S01 with the reason *"'almost always' is not a reason for
// a teardown to reach one of two stores."* Built to the stated three, the close-out leaves the
// ceremony secret on disk.
//
// **And the user's own pin survives**, which is the clause the plan calls promotion. There is no
// promote door — `grep -rn "Promote|promoted|Provisional" --include=*.go internal/vault/` finds
// none — a pin becomes a user pin by being created with no ceremony scope at all.
func TestTheCloseOutDoorReachesEveryCeremonyScopedStore(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	_, v := unlockedServer(t)
	const id = "aa11bb22cc33dd44ee55ff6677889900aa11bb22cc33dd44ee55ff6677889900"

	scoped := []byte{0x01, 0x02, 0x03}
	mine := []byte{0x09, 0x08, 0x07}
	if err := v.AddCeremonyPeer(scoped, "the other party", id); err != nil {
		t.Fatal(err)
	}
	if err := v.AddCeremonyPeer(mine, "a peer I pinned myself", ""); err != nil {
		t.Fatal(err)
	}
	if err := v.AddCeremonySecret(id, []byte{0xaa}, []byte("the-secret")); err != nil {
		t.Fatal(err)
	}
	if err := v.AddCeremonyInvitation(id, "the-invitation-text"); err != nil {
		t.Fatal(err)
	}

	// SETUP: all three really are there. Without this the teardown below could be a no-op over an
	// empty vault and every assertion would pass having checked nothing.
	if _, ok := v.CeremonyInvitationFor(id); !ok {
		t.Fatal("setup: the stored invitation is not there, so its absence below proves nothing")
	}
	if n := countScopedPins(v, id); n != 1 {
		t.Fatalf("setup: %d ceremony-scoped pins, want 1", n)
	}

	if err := closeOutStores(v, id, "test"); err != nil {
		t.Fatalf("close-out stores: %v", err)
	}

	if _, ok := v.CeremonyInvitationFor(id); ok {
		t.Error("the stored invitation survived the close-out — it carries the ceremony secret, " +
			"and this is the store P08.S01 found a teardown missing one door over")
	}
	if n := countScopedPins(v, id); n != 0 {
		t.Errorf("%d ceremony-scoped pins survived the close-out, want 0", n)
	}
	if !hasPin(v, mine) {
		t.Error("the user's OWN pin was removed by a ceremony close-out — an unscoped pin is a " +
			"promotion the user made and no ceremony may take it back")
	}
}

// TestTheCloseOutDropsThisCeremonysPunchBudgetsAndNoOthers — `/pending 312`.
//
// `punchBudgets` had no `delete` anywhere in the tree: one `*punchBudget` per hop the process ever
// saw, surviving every disarm and living for the process lifetime. The close-out door is where it
// goes, because it is the one place that knows a proceeding is over ON THIS MACHINE — and
// `ceremonyID.close()`, the obvious door, is provably the wrong one, since two `ceremonyID`s share
// a hop's budget and deleting on the first lets the second re-emit the full D33 figure.
//
// **Driven through `closeOutCeremony`, not through `dropPunchBudgets`.** ADR-009: the guard asserts
// the rule is reached through the door. A test that called the helper directly would stay green if
// the door stopped calling it, which is the only way this can actually regress.
//
// **TWO arms, and the second is the one that matters.** A `delete` that cleared the whole map
// passes the first assertion perfectly — the budgets for this ceremony are indeed gone — while
// destroying the packet ceiling of every other proceeding in flight. The surviving stranger is what
// distinguishes a scoped drop from `punchBudgets = nil`. Both hops of the closed ceremony are
// asserted for the same reason one level down: since P08.S05h the key is `(ceremony, hop)`, so a
// drop that matched the whole key rather than its ceremony prefix would clear hop 1 and leave hop 2
// behind, and a single-hop fixture could never see it.
func TestTheCloseOutDropsThisCeremonysPunchBudgetsAndNoOthers(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	s, v := unlockedServer(t)
	rec, _, _ := ceremonyOnDisk(t)
	const stranger = "ffee0011223344556677889900aabbccddeeff00112233445566778899aabbcc"

	s.punchBudgetFor(rec.ID, 1)
	s.punchBudgetFor(rec.ID, 2)
	s.punchBudgetFor(stranger, 1)

	// SETUP: all three entries really exist. Without this the assertions below could each be one
	// absence compared against another, and a `dropPunchBudgets` that did nothing would pass.
	s.punchMu.Lock()
	n := len(s.punchBudgets)
	s.punchMu.Unlock()
	if n != 3 {
		t.Fatalf("setup: %d punch budget(s) before the close-out, want 3 — the fixture is not the "+
			"state these assertions are about", n)
	}

	if err := s.closeOutCeremony(v, rec.ID, "completed", time.Now()); err != nil {
		t.Fatalf("close-out: %v", err)
	}

	s.punchMu.Lock()
	_, hop1 := s.punchBudgets[rec.ID+"#1"]
	_, hop2 := s.punchBudgets[rec.ID+"#2"]
	_, other := s.punchBudgets[stranger+"#1"]
	left := len(s.punchBudgets)
	s.punchMu.Unlock()

	if hop1 || hop2 {
		t.Errorf("a punch budget survived the close-out (hop1=%v hop2=%v) — the map has no other "+
			"`delete`, so this entry would then live for the process lifetime", hop1, hop2)
	}
	if !other {
		t.Error("closing out one ceremony dropped ANOTHER proceeding's punch budget — that " +
			"resets a live D33 packet ceiling to zero and lets the same side emit the full " +
			"figure again, which is the defect the map was lifted onto the Server to prevent")
	}
	if left != 1 {
		t.Errorf("%d punch budget(s) left, want 1 (the stranger's)", left)
	}
}

// TestAnUnreadableCeremonyIsNeverClosedOut.
//
// A record that does not parse or does not verify has no trustworthy `Expires`, and moving a
// directory on the strength of a field that did not verify is how a live ceremony disappears
// because one file was damaged. P08.S03 built the four load classes for exactly this decision.
//
// **Driven at `closeOutReason` with a deadline long past**, so the only thing standing between
// this ceremony and a close-out is the load class. A fixture whose deadline had not passed would
// pass this test with the class check deleted.
func TestAnUnreadableCeremonyIsNeverClosedOut(t *testing.T) {
	long := time.Now().Add(-100 * 24 * time.Hour)
	rec := ceremony.Record{ID: strings.Repeat("ab", 32)}

	// SETUP: the healthy shape of this exact fixture IS closed out, or the negative below is
	// vacuous — it would pass against a `closeOutReason` that never returns true at all.
	ok := ceremony.Stored{ID: rec.ID, State: ceremony.LoadOK, Expires: long}
	if state, closed := closeOutReason(ok, rec, "me", time.Now()); !closed {
		t.Fatalf("setup: a readable ceremony %v past its deadline is not closed out either "+
			"(state %q) — the class check below would then be asserting nothing",
			time.Since(long), state)
	}

	for _, class := range []ceremony.LoadState{
		ceremony.LoadUnparseable, ceremony.LoadUnverifiable, ceremony.LoadAbsent,
	} {
		st := ceremony.Stored{ID: rec.ID, State: class, Expires: long}
		if _, closed := closeOutReason(st, rec, "me", time.Now()); closed {
			t.Errorf("a ceremony classified %v was closed out — its record did not verify, so "+
				"its deadline is not a fact and its directory must not move on one", class)
		}
	}
}

// TestTheReceiptKeepsTheFirstThingThisMachineObserved.
//
// The states are not equal in standing: `declined` and `completed` come from a verified
// termination somebody signed; `expired` and `abandoned` are conclusions drawn from a clock. Past
// the grace `closeOutReason` returns `abandoned` for anything with no end state, and the sweep can
// reach one ceremony more than once — so without write-once a re-sweep overwrites *"they declined"*
// with *"nothing ever said"*, replacing the better answer with the worse and leaving no trace.
func TestTheReceiptKeepsTheFirstThingThisMachineObserved(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := defaultOutputDir()

	rec, _, _ := ceremonyOnDisk(t)
	if err := ceremony.CloseOutMirror(root, rec.ID); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := ceremony.WriteReceipt(root, rec.ID, ceremony.Receipt{
		Ceremony: rec.ID, State: ceremony.StateDeclined, ObservedAt: now,
	}); err != nil {
		t.Fatalf("setup: the first receipt did not write (%v)", err)
	}

	// The same state again is an interrupted close-out finishing, and must not read as a fault.
	if err := ceremony.WriteReceipt(root, rec.ID, ceremony.Receipt{
		Ceremony: rec.ID, State: ceremony.StateDeclined, ObservedAt: now.Add(time.Hour),
	}); err != nil {
		t.Errorf("re-writing the SAME state was reported as a conflict (%v) — that is what an "+
			"interrupted close-out finishing looks like", err)
	}

	err := ceremony.WriteReceipt(root, rec.ID, ceremony.Receipt{
		Ceremony: rec.ID, State: ceremony.StateAbandoned, ObservedAt: now.Add(2 * time.Hour),
	})
	if !errors.Is(err, ceremony.ErrReceiptConflict) {
		t.Errorf("a derived 'abandoned' overwrote an attested 'declined': %v", err)
	}
	got, rerr := ceremony.ReadReceipt(root, rec.ID)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if got.State != ceremony.StateDeclined {
		t.Errorf("the receipt now says %q — the first thing this machine observed is the one "+
			"that stands", got.State)
	}
}

// TestAnUnfinishedDeliveryRoundHoldsTheCloseOut.
//
// D29 puts the delivery round BEFORE the close-out, and the round needs the very mirror the
// close-out would move. The convener's evidence that a round finished is one `delivered/<fp>`
// marker per party; a signer's is its own copy on disk.
//
// **Both directions are asserted from one fixture.** A test that only checked the held case passes
// against a `roundIsFinished` that always returns false, which would hold every ceremony open
// forever — the failure that looks like safety.
func TestAnUnfinishedDeliveryRoundHoldsTheCloseOut(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	rec, _, _ := ceremonyOnDisk(t)
	conv := convenerFingerprintOf(rec)
	other := ""
	for _, p := range rec.Roster {
		if !strings.EqualFold(p.Fingerprint, conv) {
			other = p.Fingerprint
		}
	}
	if other == "" {
		t.Fatal("setup: this fixture has no party but the convener, so there is no round to hold")
	}

	st := ceremony.Stored{
		ID: rec.ID, State: ceremony.LoadOK, Ended: ceremony.StateCompleted,
		Expires: time.Now().Add(48 * time.Hour), // inside the grace, so only the round decides
	}
	if _, closed := closeOutReason(st, rec, conv, time.Now()); closed {
		t.Error("a ceremony whose delivery round has reached nobody was closed out — the round " +
			"needs the mirror this move takes away, and the party never gets its copy")
	}

	if err := markDelivered(rec.ID, other); err != nil {
		t.Fatal(err)
	}
	if _, closed := closeOutReason(st, rec, conv, time.Now()); !closed {
		t.Error("every party has acknowledged and the ceremony is STILL not closed out — a " +
			"round that can never be finished holds the prune open forever, which is the same " +
			"defect wearing a safer-looking face")
	}
}

// TestASignerOnADeclinedCeremonyIsNotWaitingForADocument.
//
// **The two end states deliver different payloads and the close-out gate has to ask the matching
// question.** `runDeliveryRound` says it: a completed ceremony delivers the finished document, a
// declined one delivers the convener's signed termination, *"because there is no finished document
// and the parties who already signed are otherwise left believing it is still travelling."*
//
// The first cut of `roundIsFinished` asked `alreadyDelivered` for both — a stat on
// `~/nib/signed/<name>`, the finished document's path — which on a declined ceremony is a file the
// round will never send. Permanently false, so a signer who had already been TOLD the proceeding
// was over held the folder and its pins until the three-day grace ran out.
//
// **Both states from one fixture**, because the declined half alone passes against a
// `roundIsFinished` that returns true for every non-convener — which would close out a completed
// ceremony before its document arrived, destroying the mirror the round still needs.
func TestASignerOnADeclinedCeremonyIsNotWaitingForADocument(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	rec, _, _ := ceremonyOnDisk(t)
	signer := ""
	for _, p := range rec.Roster {
		if !strings.EqualFold(p.Fingerprint, convenerFingerprintOf(rec)) {
			signer = p.Fingerprint
		}
	}
	if signer == "" {
		t.Fatal("setup: no non-convener in this fixture, so neither branch below is reached")
	}
	inGrace := time.Now().Add(48 * time.Hour) // so only the round's answer decides

	declined := ceremony.Stored{ID: rec.ID, State: ceremony.LoadOK,
		Ended: ceremony.StateDeclined, Expires: inGrace}
	if _, closed := closeOutReason(declined, rec, signer, time.Now()); !closed {
		t.Error("a signer holding a verified 'declined' termination is still waiting — the " +
			"termination IS the delivery on that path, and nothing further is coming. It would " +
			"hold the folder and its pins for the whole grace on a finished proceeding")
	}

	completed := ceremony.Stored{ID: rec.ID, State: ceremony.LoadOK,
		Ended: ceremony.StateCompleted, Expires: inGrace}
	if _, closed := closeOutReason(completed, rec, signer, time.Now()); closed {
		t.Error("a signer was closed out on a COMPLETED ceremony before its finished document " +
			"arrived — the round still needs the mirror this move takes away, and the party " +
			"never gets its copy")
	}
}

// TestACompletedCeremonyIsClosedOutWhenItsDocumentArrives is C11's central case.
//
// *"A ceremony directory is gone after the ceremony has ended and its document has been delivered
// or saved."* On a completed ceremony the round delivers the finished DOCUMENT and not an
// attestation — `runDeliveryRound` chooses between the two — so a signer never receives a
// `completed` termination and `Stored.Ended` stays empty on its machine forever.
//
// Before this branch the only way out was the grace: every completed ceremony sat in the live set
// for three days past its deadline and was then recorded as `abandoned`, a durable local lie about
// a proceeding that finished exactly as intended. **Measured at tier 4** — two relay ceremonies
// whose finished documents were already in `~/nib/signed/` were still in `~/nib/ceremonies/` with
// nothing able to move them, and the run's own `find` is what showed it.
//
// The negative arm is the whole reason this is safe: with no delivered document the ceremony is
// still live and must not move, because the round needs the mirror.
func TestACompletedCeremonyIsClosedOutWhenItsDocumentArrives(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	rec, _, _ := ceremonyOnDisk(t)
	signer := ""
	for _, p := range rec.Roster {
		if !strings.EqualFold(p.Fingerprint, convenerFingerprintOf(rec)) {
			signer = p.Fingerprint
		}
	}
	st := ceremony.Stored{ID: rec.ID, State: ceremony.LoadOK, Expires: time.Now().Add(48 * time.Hour)}

	// SETUP and negative arm in one: no delivered document, no end state, inside the grace.
	if _, closed := closeOutReason(st, rec, signer, time.Now()); closed {
		t.Fatalf("setup: a live ceremony with no end state and no delivered document was closed " +
			"out — the round needs this mirror, and the positive arm below would then be " +
			"asserting a close-out that happens unconditionally")
	}

	// The finished document arrives, which is the only thing that changes.
	path := deliveredPathFor(rec)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("the finished document"), 0o600); err != nil {
		t.Fatal(err)
	}

	state, closed := closeOutReason(st, rec, signer, time.Now())
	if !closed {
		t.Fatal("a signer holding its finished document is still not closed out. C11 says the " +
			"directory goes once the document has been delivered; the only remaining exit is " +
			"the three-day grace, which then records a completed ceremony as 'abandoned'")
	}
	if state != ceremony.StateCompleted {
		t.Errorf("the close-out recorded %q, want %q — this machine holds the finished document, "+
			"which is what completion looks like from here", state, ceremony.StateCompleted)
	}
}

// TestACloseOutWithNothingToMoveStillLeavesAReceipt.
//
// **The party that declines never mirrored the ceremony at all.** It refuses at its consent gate,
// which returns before `rd.Store`, so `CloseOutMirror` correctly finds no source and does nothing —
// and the receipt then had no directory to land in. Measured at tier 4: the decline leg logged
// *"could not write the local receipt: … no such file or directory"* and the decliner was left with
// no local record that it had ever refused, on the one machine that record matters most on.
//
// The receipt is not an annotation on a moved folder. It is this machine's statement that it
// observed a proceeding end, and it has to survive the case where there was nothing of the user's
// to preserve.
//
// **This assertion was owed and missing**: the fix went in from the tier-4 failure, and deleting it
// again left the whole tier-1 suite green — the vacuous green, arriving on a fix rather than on a
// feature.
func TestACloseOutWithNothingToMoveStillLeavesAReceipt(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := defaultOutputDir()
	const id = "11aa22bb33cc44dd55ee66ff77889900"

	// SETUP: nothing is mirrored for this id, which is the decliner's true state.
	live, err := ceremony.MirrorDir(root, id)
	if err != nil {
		t.Fatal(err)
	}
	if _, serr := os.Stat(live); !os.IsNotExist(serr) {
		t.Fatalf("setup: something is already mirrored at %s (%v), so this is not the "+
			"nothing-to-move case the test is about", live, serr)
	}

	if err := ceremony.CloseOutMirror(root, id); err != nil {
		t.Fatalf("a close-out with nothing to move reported an error (%v) — an absent source is "+
			"the ordinary case for a sweep that runs on every unlock, and reporting it as damage "+
			"makes a clean second pass look like a fault", err)
	}
	now := time.Now()
	if err := ceremony.WriteReceipt(root, id, ceremony.Receipt{
		Ceremony: id, State: ceremony.StateDeclined, ObservedAt: now,
	}); err != nil {
		t.Fatalf("the receipt did not write when there was nothing to move: %v.\n"+
			"This is the decliner's exact case, and it leaves the machine that refused with no "+
			"record that it ever did", err)
	}
	got, rerr := ceremony.ReadReceipt(root, id)
	if rerr != nil {
		t.Fatalf("the receipt cannot be read back: %v", rerr)
	}
	if got.State != ceremony.StateDeclined || got.Ceremony != id {
		t.Errorf("the receipt reads %+v, want ceremony %s in state %q", got, id, ceremony.StateDeclined)
	}
	// And it is visible to the listing, or a decliner's own record is one nothing shows.
	ended, lerr := ceremony.ListEnded(root)
	if lerr != nil {
		t.Fatal(lerr)
	}
	if len(ended) != 1 || ended[0].Ceremony != id {
		t.Errorf("the ended listing holds %d entry/entries %v, want exactly this ceremony",
			len(ended), ended)
	}
}

// TestTheGraceIsDerivedFromTheCeremonyCeilingAndNotHandCopied is the slice's grace bullet.
//
// This is `maxCandidatesPerSource`'s rule and `lawplacement_test.go`'s technique: the difference
// between a derived figure and a hand-copied one is not observable at runtime, so only the source
// says which one you have. A literal here plus a comment claiming it agrees with
// `ceremony.MaxCeremonyLife` is not a mechanism — move the ceiling and this silently stays behind,
// closing out ceremonies the record still considers live.
//
// **It asserts a REFERENCE, not the value.** `closeOutGrace == ceremony.MaxCeremonyLife/10` would
// be a tautology that a hand-copied `72 * time.Hour` also satisfies today.
func TestTheGraceIsDerivedFromTheCeremonyCeilingAndNotHandCopied(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, tunableBlockFile, nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("cannot parse %s, so this guard scanned nothing: %v", tunableBlockFile, err)
	}
	var expr ast.Expr
	for _, d := range f.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, sp := range gd.Specs {
			vs, ok := sp.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range vs.Names {
				if name.Name == "closeOutGrace" && i < len(vs.Values) {
					expr = vs.Values[i]
				}
			}
		}
	}
	// SETUP: the constant is in the file this guard parses. A renamed or moved constant would
	// otherwise report "no literal found" — a pass for the wrong reason.
	if expr == nil {
		t.Fatalf("closeOutGrace is not declared in %s, so this guard scanned nothing. If it "+
			"moved, point this test at its new home rather than deleting it", tunableBlockFile)
	}
	// `exprText` (quoterect_test.go) renders the expression's identifiers, which is exactly the
	// question here: `ceremony.MaxCeremonyLife / 10` yields them and a hand-copied
	// `72 * time.Hour` yields `time.Hour`.
	src := exprText(fset, expr)
	if !strings.Contains(src, "MaxCeremonyLife") {
		t.Errorf("closeOutGrace's identifiers are %q and none is ceremony.MaxCeremonyLife.\n"+
			"A hand-copied duration plus a comment saying the two agree is not a mechanism: "+
			"raise the ceiling and this stays behind, closing out ceremonies the record still "+
			"considers live. This is maxCandidatesPerSource's rule, one constant over", src)
	}
}

// countScopedPins counts pins carrying the named ceremony's scope.
func countScopedPins(v *vault.Vault, id string) int {
	n := 0
	for _, p := range v.PinnedPeers() {
		for _, c := range p.Ceremonies {
			if strings.EqualFold(c, id) {
				n++
			}
		}
	}
	return n
}

// hasPin reports whether a pin for this fingerprint is still in the vault at all.
func hasPin(v *vault.Vault, fp []byte) bool {
	for _, p := range v.PinnedPeers() {
		if hex.EncodeToString(p.Fingerprint) == hex.EncodeToString(fp) {
			return true
		}
	}
	return false
}

// TestANonPrimaryNibDoesNotCloseOut is /pending 362.
//
// **The sentence was tested and the code that makes it true was not**, which is the wrong way
// round. The listing route tells the user *"another copy of Nib is already running on this machine.
// This one can show your ceremonies but must not continue or REMOVE them, because both would be
// writing to the same folder"* — and `TestTheCeremonyListingSaysWhenThisNibMustNotAct` asserts that
// prose while nothing drove the gate underneath it.
//
// The repo deliberately lets a second Nib run: P08.S03 killed the planned lock on
// `~/nib/ceremonies/` because `instanceToken` already carries the signal. So the signal has to be
// READ at every door that writes, and this door moves directories and drops vault pins.
//
// **Both arms, from one fixture.** A test that only checked the non-primary case passes against a
// sweep that never closes anything out — which would be a different defect wearing the same green.
func TestANonPrimaryNibDoesNotCloseOut(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	srv, v := unlockedServer(t)
	rec, _, _ := ceremonyOnDisk(t)
	root := defaultOutputDir()

	// Make it closeable: a delivered document is C11's central case, so a primary sweep WILL move
	// it. Without this the non-primary arm asserts that nothing happened to something nothing was
	// going to happen to.
	path := deliveredPathFor(rec)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("the finished document"), 0o600); err != nil {
		t.Fatal(err)
	}
	live, err := ceremony.MirrorDir(root, rec.ID)
	if err != nil {
		t.Fatal(err)
	}

	// ── The NON-PRIMARY arm: another Nib holds this machine's instance record.
	srv.mu.Lock()
	srv.instanceToken = ""
	srv.mu.Unlock()
	srv.closeOutEnded(v, time.Now())
	if _, serr := os.Stat(live); serr != nil {
		t.Fatalf("a NON-primary Nib closed out a ceremony (%v). The listing route tells the user "+
			"this copy 'must not continue or remove them, because both would be writing to the "+
			"same folder' — two processes moving one directory with no lock between them is "+
			"exactly what P08.S03 declined to solve with a lock, on the grounds that "+
			"instanceToken already carried the signal", serr)
	}

	// ── The PRIMARY arm, from the same fixture: it DOES close out. Without this the assertion
	// above is satisfied by a sweep that never moves anything.
	srv.mu.Lock()
	srv.instanceToken = "this-one"
	srv.mu.Unlock()
	srv.closeOutEnded(v, time.Now())
	if _, serr := os.Stat(live); !os.IsNotExist(serr) {
		t.Errorf("the PRIMARY Nib did not close out a delivered ceremony (%v), so the arm above "+
			"proves nothing: it would pass against a sweep that closes nothing at all", serr)
	}
}
