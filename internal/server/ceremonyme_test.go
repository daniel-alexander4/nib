package server

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"nib/internal/ceremony"
)

// P06.S02 — which party this machine is, recorded while the vault is open and read without one.

// TestConveningRecordsWhichPartyThisMachineIs.
//
// **The one thing about a ceremony a vault-less reader cannot work out.** P06.S01 took the listing
// off the lock because every field comes from `record.json` — and the field that does not is *which
// of these parties am I*, which needs `identity(v)`. Both write paths already hold the answer:
// `handleCeremonyConvene` has the convener's fingerprint and `handleCeremonyAccept` computes `me`.
// Neither recorded it.
//
// **Driven through the real route**, not by calling `WriteMe`, because the point of the slice is
// that the write happens on the path a user takes. A test that called the writer directly would
// pass against a route that never calls it.
func TestConveningRecordsWhichPartyThisMachineIs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	rec, _, _ := ceremonyOnDisk(t)

	// SETUP: `ceremonyOnDisk` writes the mirror directly and does NOT go through the convene
	// route, so it must leave no marker — otherwise the route-driven assertion below could pass
	// against a fixture that wrote one.
	st := ceremony.ReadStored(defaultOutputDir(), rec.ID, time.Now())
	if st.Me != "" {
		t.Fatalf("setup: the fixture already recorded a position (%q), so this test cannot tell "+
			"the route's write from the fixture's", st.Me)
	}

	// Now the real route.
	convened := conveneThroughRoute(t)

	got := ceremony.ReadStored(defaultOutputDir(), convened, time.Now())
	if got.State != ceremony.LoadOK {
		t.Fatalf("the convened ceremony does not load (%v: %s)", got.State, got.Reason)
	}
	if got.Me == "" {
		t.Fatal("convening recorded no position. The convener's own fingerprint is right there on " +
			"that path — it is passed to pinCeremonyRoster on the next line — and a reader " +
			"without a vault cannot work it out, so the panel can never say which party the user is")
	}
	// And it points at a party that is actually on the roster, which is the only thing that makes
	// it useful: a marker naming somebody absent is worse than none.
	onRoster := false
	for _, p := range got.Roster {
		if strings.EqualFold(p.Fingerprint, got.Me) {
			onRoster = true
		}
	}
	if !onRoster {
		t.Errorf("the recorded position %q names nobody on this ceremony's roster of %d — a "+
			"pointer into a list that does not contain it", got.Me, len(got.Roster))
	}
}

// TestAnAbsentPositionIsUnknownAndNotAbsence.
//
// **The two states a surface must keep apart** are *"we do not know which of these is you"* and
// *"you are not on this roster"*, and only the first is expressible. A ceremony mirrored before the
// marker shipped has none, and its user is still very much a party; a panel reading the empty
// string as absence would tell every one of them they are looking at somebody else's proceeding.
//
// This is a guard on the READ side, because that is where the mistake gets made: the writer cannot
// produce a wrong answer, only no answer.
func TestAnAbsentPositionIsUnknownAndNotAbsence(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	rec, _, _ := ceremonyOnDisk(t)

	st := ceremony.ReadStored(defaultOutputDir(), rec.ID, time.Now())
	if st.State != ceremony.LoadOK {
		t.Fatalf("setup: the fixture does not load (%v)", st.State)
	}
	if st.Me != "" {
		t.Fatalf("setup: a fixture that never went through a write path recorded %q", st.Me)
	}
	// The ceremony is otherwise WHOLE — roster, intent, deadline. An unknown position must not
	// degrade any of it, or the panel loses a ceremony over a missing label.
	if len(st.Roster) == 0 || st.Intent == "" || st.Expires.IsZero() {
		t.Errorf("a ceremony with no position marker came back degraded (roster=%d intent=%q "+
			"expires=%v). The position is one label; losing the whole entry over it is the "+
			"self-healing rule inverted", len(st.Roster), st.Intent, st.Expires)
	}
}

// TestNoDegradedClassReportsAPosition.
//
// `Me` is a pointer INTO the roster, so it means nothing beside a roster that did not check out. A
// panel showing "you are party 3" against a record it has just refused to trust is reading a
// position out of a file it does not believe.
//
// **Every degraded class, not one.** `ReadStored` has six early returns and the position is set
// only on the healthy path — so a test that drives one branch proves the rule for one branch. The
// first cut truncated the record (which is `LoadUnparseable`) and a mutation adding `readMe` to the
// `LoadUnverifiable` branch came back GREEN: the test and the mutation were on different arms of
// the same function, and neither noticed. Driving the classes the fixture can actually produce is
// what makes any one of those arms answerable.
func TestNoDegradedClassReportsAPosition(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	id := conveneThroughRoute(t)

	// SETUP: healthy, and it HAS a position — otherwise every absence below is absence for the
	// wrong reason and the whole test passes against a `Me` that is never set at all.
	before := ceremony.ReadStored(defaultOutputDir(), id, time.Now())
	if before.State != ceremony.LoadOK || before.Me == "" {
		t.Fatalf("setup: state=%v me=%q — this test needs a healthy ceremony WITH a position",
			before.State, before.Me)
	}
	dir, err := ceremony.MirrorDir(defaultOutputDir(), id)
	if err != nil {
		t.Fatal(err)
	}
	good, err := os.ReadFile(dir + "/record.json")
	if err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct {
		name  string
		write func()
	}{
		{"unparseable-decode", func() { _ = os.WriteFile(dir+"/record.json", []byte("{not json"), 0o600) }},
		// **The SECOND unparseable arm**, and it needed its own case. `ReadStored` reaches
		// `LoadUnparseable` from two places — a read error that is not `IsNotExist`, and a JSON
		// decode failure — and a mutation on the first came back GREEN while the truncated-record
		// case above only ever reaches the second. One class, two arms, and a test that drives a
		// class does not drive its arms.
		{"unparseable-unreadable", func() { _ = os.Chmod(dir+"/record.json", 0o000) }},
		{"absent", func() { _ = os.Remove(dir + "/record.json") }},
		{"tampered", func() {
			// A record that parses and fails its signature — the class that is an accusation, and
			// the one where showing a position would be most misleading.
			bad := strings.Replace(string(good), `"intent":"`, `"intent":"X`, 1)
			_ = os.WriteFile(dir+"/record.json", []byte(bad), 0o600)
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			c.write()
			got := ceremony.ReadStored(defaultOutputDir(), id, time.Now())
			if got.State == ceremony.LoadOK {
				t.Skipf("this fixture still loads OK (%v), so the class was not produced and "+
					"nothing here is asserted — recorded as a skip rather than a pass", got.State)
			}
			if got.Me != "" {
				t.Errorf("class %v reported a position (%q). The marker points into a roster this "+
					"read has just refused to trust", got.State, got.Me)
			}
			_ = os.Chmod(dir+"/record.json", 0o600) // undo the unreadable case before restoring
			_ = os.WriteFile(dir+"/record.json", good, 0o600)
		})
	}
}

// conveneThroughRoute convenes a two-party ceremony over the real route and returns its id.
//
// Uses the package's own `startServer`/`authedClient` shape rather than a hand-built request, so
// this drives the same door every other convene test drives.
func conveneThroughRoute(t *testing.T) string {
	t.Helper()
	ts, pdfPath := startServer(t)
	c, csrf := authedClient(t, ts)
	if code, body := postForCode(t, c, csrf, ts.URL+"/api/open", openRequest{Path: pdfPath}); code != http.StatusOK {
		t.Fatalf("open: %d %s", code, body)
	}
	me := myFingerprint(t, c, ts.URL)
	req := conveneRequest{
		Roster: []convenePartyRequest{
			{Fingerprint: me, Label: "Me", Signs: true},
			{Fingerprint: strings.Repeat("7c", 32), Label: "Other", Signs: true},
		},
		Intent:        "We agree to the terms",
		Expires:       time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339),
		ConvenerSigns: true,
	}
	code, body := postForCode(t, c, csrf, ts.URL+"/api/ceremony/convene", req)
	if code != http.StatusOK {
		t.Fatalf("convene returned %d: %s", code, body)
	}
	var out struct {
		Ceremony string `json:"ceremony"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("convene response is not JSON (%v): %s", err, body)
	}
	if out.Ceremony == "" {
		t.Fatalf("convene returned no ceremony id: %s", body)
	}
	return out.Ceremony
}

// TestTheListingNamesWhoConvenedIt is /pending 353's missing published fact.
//
// **The delivery round is the convener's alone and nothing published said who that was.** `Me`
// tells a reader which party it is; without a convener fingerprint beside it there is nothing to
// compare against, so the panel could only offer the round to every party and let the server
// refuse — and that refusal is *"Nib no longer holds the invitation secret"*, which is true, means
// nothing to a non-convener, and reads identically to a ceremony whose secrets were cleaned up.
//
// **The two fields are asserted to be DIFFERENT things, not just present.** A `Convener` that
// merely echoed `Me` would satisfy a presence check and tell the panel nothing: the fixture mirror
// below is a ceremony this machine did NOT convene, so it must carry a convener and no position.
// That pair is what makes the comparison in `ceremonyCard` meaningful.
func TestTheListingNamesWhoConvenedIt(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// A ceremony this machine convened: the two fingerprints are the same party.
	convened := conveneThroughRoute(t)
	got := ceremony.ReadStored(defaultOutputDir(), convened, time.Now())
	if got.State != ceremony.LoadOK {
		t.Fatalf("the convened ceremony does not load (%v: %s)", got.State, got.Reason)
	}
	if got.Convener == "" {
		t.Fatal("the listing does not say who convened this ceremony. The record is signed by the " +
			"convener and Record.Convener resolves them off the roster, so a panel deciding " +
			"whether this machine may run a delivery round has nothing to compare Me against")
	}
	if !strings.EqualFold(got.Convener, got.Me) {
		t.Errorf("this machine convened the ceremony and the listing disagrees: convener=%s me=%s.\n\n"+
			"The convene route records both, and the delivery control is gated on them matching",
			got.Convener, got.Me)
	}
	onRoster := false
	for _, p := range got.Roster {
		if strings.EqualFold(p.Fingerprint, got.Convener) {
			onRoster = true
		}
	}
	if !onRoster {
		t.Errorf("the convener %s is not on the roster it convened", got.Convener)
	}

	// And a ceremony this machine did NOT convene: a convener, and no position. This is the arm
	// that proves the field is read off the RECORD rather than echoed from the marker — an
	// implementation that set Convener from Me would leave this one empty.
	rec, _, _ := ceremonyOnDisk(t)
	other := ceremony.ReadStored(defaultOutputDir(), rec.ID, time.Now())
	if other.State != ceremony.LoadOK {
		t.Fatalf("the fixture ceremony does not load (%v: %s)", other.State, other.Reason)
	}
	if other.Me != "" {
		t.Fatalf("setup: the fixture recorded a position (%q), so this arm cannot tell a convener "+
			"read from the record from one echoed off the marker", other.Me)
	}
	if other.Convener == "" {
		t.Error("a ceremony this machine did not convene reports no convener at all. The record " +
			"names one — it is signed by them — so this is read off Me rather than off the record, " +
			"and every mirrored ceremony would offer its delivery control to nobody")
	}
}
