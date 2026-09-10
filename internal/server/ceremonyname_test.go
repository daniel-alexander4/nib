package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"nib/internal/ceremony"
)

// Naming a proceeding on this machine.
//
// # The defect these drive
//
// The panel headed every card with its Intent, and the Intent is the recital — a clause meant to be
// signed, not a handle to scan a list by. A user with three live proceedings over three leases read
// three cards that opened *"We agree to…"* and told them apart by squinting. Worse, a party before
// their hop has no record at all, so their card had no heading of its own to read: it fell to
// *"A ceremony on this machine"*, which is what every one of them says.

// nameCeremony posts a label and returns the code and body.
func nameCeremony(t *testing.T, c *http.Client, csrf, base, id, name string) (int, string) {
	t.Helper()
	return postForCode(t, c, csrf, base+"/api/ceremony/name",
		ceremonyNameRequest{Ceremony: id, Name: name})
}

// acceptedCeremony gets this machine to the state a named-before-anything-arrives party is in:
// an invitation accepted, a `me` marker on disk, and no record.
func acceptedCeremony(t *testing.T, c *http.Client, csrf, base string) string {
	t.Helper()
	me := myFingerprint(t, c, base)
	invitation, _ := inviteFor(t, me)
	code, body := postForCode(t, c, csrf, base+"/api/ceremony/accept",
		acceptRequest{Invitation: invitation})
	if code != http.StatusOK {
		t.Fatalf("setup: accept returned %d: %s", code, body)
	}
	var ar acceptResponse
	if err := json.Unmarshal([]byte(body), &ar); err != nil {
		t.Fatal(err)
	}
	return ar.Ceremony
}

// TestAPartyBeforeTheirHopCanNameTheirCeremonyAndTheListingCarriesIt is the whole point of the
// route, driven at the party who most needs it.
//
// **The pre-hop party is the case, not a corner of it.** They hold no record — `ReadStored`
// answers `LoadAbsent` with a `me` marker — so the server must accept a name for a ceremony it
// cannot describe, and the listing must carry that name back out of a directory holding nothing
// but two small files. A route gated on `state == ok` would pass a happy-path test and refuse
// exactly the party whose card is otherwise blank.
func TestAPartyBeforeTheirHopCanNameTheirCeremonyAndTheListingCarriesIt(t *testing.T) {
	ts, _ := startServerWith(t)
	c, csrf := authedClient(t, ts)
	id := acceptedCeremony(t, c, csrf, ts.URL)

	// SETUP: this really is the pre-hop shape. Without it the test could be passing over a
	// ceremony with a record, which is the easy case and not the one that was broken.
	if st := ceremony.ReadStored(defaultOutputDir(), id, time.Now()); st.State != ceremony.LoadAbsent || !st.Joined {
		t.Fatalf("setup: wanted an accepted, pre-hop ceremony (absent+joined); got state=%v joined=%v",
			st.State, st.Joined)
	}

	if code, body := nameCeremony(t, c, csrf, ts.URL, id, "The Elm Row lease"); code != http.StatusOK {
		t.Fatalf("naming an accepted ceremony returned %d: %s — the party with no record yet is "+
			"the one whose card says nothing but \"A ceremony on this machine\"", code, body)
	}

	got := listedCeremony(t, ts.URL, id)
	if got.Name != "The Elm Row lease" {
		t.Errorf("the listing carried name %q, want %q — a name the panel cannot read is a file "+
			"on disk and nothing else", got.Name, "The Elm Row lease")
	}
}

// TestANameIsRefusedForACeremonyThisMachineHasNeverHeardOf keeps the route from minting
// directories under ~/nib for ids nobody sent.
func TestANameIsRefusedForACeremonyThisMachineHasNeverHeardOf(t *testing.T) {
	ts, _ := startServerWith(t)
	c, csrf := authedClient(t, ts)
	unknown := strings.Repeat("ab", 16)

	code, _ := nameCeremony(t, c, csrf, ts.URL, unknown, "not mine")
	if code != http.StatusNotFound {
		t.Errorf("naming an unknown ceremony returned %d, want 404 — a label on nothing is a "+
			"directory the close-out will never visit", code)
	}
	// And nothing was created on the way to refusing — the path `WriteName` would have used, so
	// this fails if the refusal is removed rather than passing on a path nothing writes.
	dir, err := ceremony.MirrorDir(defaultOutputDir(), unknown)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("the refused name left %s behind (%v)", dir, err)
	}
}

// TestClearingANameRemovesItRatherThanStoringABlank — "never named" and "cleared" are one state.
func TestClearingANameRemovesItRatherThanStoringABlank(t *testing.T) {
	ts, _ := startServerWith(t)
	c, csrf := authedClient(t, ts)
	id := acceptedCeremony(t, c, csrf, ts.URL)

	if code, body := nameCeremony(t, c, csrf, ts.URL, id, "The Elm Row lease"); code != http.StatusOK {
		t.Fatalf("setup: naming returned %d: %s", code, body)
	}
	if code, body := nameCeremony(t, c, csrf, ts.URL, id, "   "); code != http.StatusOK {
		t.Fatalf("clearing returned %d: %s", code, body)
	}
	if got := listedCeremony(t, ts.URL, id).Name; got != "" {
		t.Errorf("clearing left the name %q", got)
	}
	// The file, not just the field: a blank file would make the panel render an empty heading
	// where it should fall back to the recital.
	dir, err := ceremony.MirrorDir(defaultOutputDir(), id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "name")); !os.IsNotExist(err) {
		t.Errorf("clearing the name left the file behind (%v) — \"never named\" and \"cleared\" "+
			"are supposed to be the same state on disk", err)
	}
}

// TestAnOverlongNameIsCappedAndTheAnswerIsWhatWasSTORED.
//
// **The echo is the assertion.** A route that trimmed on disk and echoed the request would tell
// the client a name it does not have, and the client writes that straight back into the field —
// so the next save would be of a name the user never typed.
func TestAnOverlongNameIsCappedAndTheAnswerIsWhatWasSTORED(t *testing.T) {
	ts, _ := startServerWith(t)
	c, csrf := authedClient(t, ts)
	id := acceptedCeremony(t, c, csrf, ts.URL)

	long := strings.Repeat("é", ceremony.MaxCeremonyNameLen+40)
	code, body := nameCeremony(t, c, csrf, ts.URL, id, long)
	if code != http.StatusOK {
		t.Fatalf("an over-long name returned %d: %s — it is capped, not refused", code, body)
	}
	var out struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	if n := len([]rune(out.Name)); n != ceremony.MaxCeremonyNameLen {
		t.Errorf("the answer carried %d runes, want %d — the client refills its field from this, "+
			"so an echo of the REQUEST hands back a name the machine does not hold", n,
			ceremony.MaxCeremonyNameLen)
	}
	if stored := listedCeremony(t, ts.URL, id).Name; stored != out.Name {
		t.Errorf("the route answered %q and stored %q", out.Name, stored)
	}
}

// TestTheNameFieldInTheClientMatchesTheServersCap holds one number in two languages together.
//
// A `maxlength` larger than the server's cap silently truncates what the user typed; smaller, and
// the field refuses characters the server would have kept. Neither shows up as a failure anywhere.
func TestTheNameFieldInTheClientMatchesTheServersCap(t *testing.T) {
	b, err := os.ReadFile("../../web/app.js")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`field\.maxLength = (\d+);`).FindSubmatch(b)
	if m == nil {
		t.Fatal("web/app.js sets no `field.maxLength` on the ceremony name field — either the " +
			"control lost its cap or it was renamed and this guard now checks nothing")
	}
	if want := strconv.Itoa(ceremony.MaxCeremonyNameLen); string(m[1]) != want {
		t.Errorf("web/app.js caps the name field at %s and ceremony.MaxCeremonyNameLen is %s",
			m[1], want)
	}
}

// listedCeremony reads one entry out of the panel's own listing, which is the only path the client
// ever sees a name by.
func listedCeremony(t *testing.T, base, id string) ceremony.Stored {
	t.Helper()
	for _, s := range getCeremonies(t, base).Ceremonies {
		if s.ID == id {
			return s
		}
	}
	t.Fatalf("the listing has no ceremony %s", id)
	return ceremony.Stored{}
}

// TestAFinishedCeremonyKeepsItsNameAndTheReceiptNeverStoresIt.
//
// **Two halves of one rule and they pull against each other.** The name has to survive the
// close-out, because a finished row otherwise carries a state and a date and nothing a person
// recognises — five completed proceedings read as five identical lines. And it must NOT be copied
// into `receipt.json`, which is write-once by design: a snapshot there would disagree with the
// `name` file the moment either changed, and `WriteReceipt`'s conflict rule would then be
// comparing a state against a stale record.
func TestAFinishedCeremonyKeepsItsNameAndTheReceiptNeverStoresIt(t *testing.T) {
	ts, _ := startServerWith(t)
	c, csrf := authedClient(t, ts)
	id := acceptedCeremony(t, c, csrf, ts.URL)
	root := defaultOutputDir()

	if code, body := nameCeremony(t, c, csrf, ts.URL, id, "The Elm Row lease"); code != http.StatusOK {
		t.Fatalf("setup: naming returned %d: %s", code, body)
	}
	// ADR-012's close-out: the folder MOVES, and the receipt lands beside what it moved.
	if err := ceremony.CloseOutMirror(root, id); err != nil {
		t.Fatalf("setup: close-out: %v", err)
	}
	if err := ceremony.WriteReceipt(root, id, ceremony.Receipt{
		Ceremony: id, State: ceremony.StateLeft, ObservedAt: time.Now(),
	}); err != nil {
		t.Fatalf("setup: receipt: %v", err)
	}

	ended, err := ceremony.ListEnded(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(ended) != 1 {
		t.Fatalf("setup: %d ended ceremonies, want 1", len(ended))
	}
	if ended[0].Name != "The Elm Row lease" {
		t.Errorf("the finished row carried name %q, want %q — the name travels with the folder "+
			"ADR-012 moves, and it is the only text on that row a person would recognise",
			ended[0].Name, "The Elm Row lease")
	}

	dir, err := ceremony.EndedDir(root, id)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "receipt.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "Elm Row") {
		t.Errorf("receipt.json stored the name:\n%s\nThe receipt is write-once and the name is "+
			"editable, so a copy here is a snapshot that disagrees with the `name` file as soon "+
			"as either moves", raw)
	}
}
