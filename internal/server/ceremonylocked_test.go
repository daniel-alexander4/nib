package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"nib/internal/ceremony"
)

// P06.S01 — the ceremonies listing answers with the vault locked, and is better guarded for it.

// TestTheCeremoniesListingAnswersWithTheVaultLocked is the precondition six of P06's criteria rest
// on: *"the panel renders roster, position and next action with the vault locked, and asks for the
// password at the moment of signing rather than the moment of looking."*
//
// **Nothing in the listing needs a vault and that is P08.S03's design, not a discovery here.**
// `ListStored` and `ReadStored` read `record.json` and nothing else; the mirror is ordinary files
// under `~/nib` because D29 puts the sealed material in the vault and leaves this unsealed on
// purpose. The route was behind `requireUnlocked` anyway, and the plan has said so since
// 2026-08-18.
//
// **Field for field, not "it returned something".** A route that answered locked with an empty
// list, or with the intent blank, would satisfy a status-code assertion and fail the criterion —
// the panel would render an empty shelf to a user who has three live ceremonies. So the same disk
// state is read twice, once locked and once unlocked, and the two responses are compared.
func TestTheCeremoniesListingAnswersWithTheVaultLocked(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	srv := New(os.DirFS("."), os.DirFS("."), t.TempDir(), "test")
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	rec, _, _ := ceremonyOnDisk(t)

	// SETUP: the server really is locked. Without this the "locked" read below is an unlocked one
	// and the whole test asserts nothing — the vacuous green in its most direct form.
	if srv.unlockedVault() != nil {
		t.Fatal("setup: this server already holds an unlocked vault, so the read below is not a " +
			"locked read and every assertion in this test is vacuous")
	}

	locked := getCeremonies(t, ts.URL)
	if len(locked.Ceremonies) != 1 {
		t.Fatalf("a locked read returned %d ceremony/ies, want 1 — the panel would render an "+
			"empty shelf to a user who has one", len(locked.Ceremonies))
	}
	got := locked.Ceremonies[0]
	if got.ID != rec.ID {
		t.Errorf("locked read named ceremony %q, want %q", got.ID, rec.ID)
	}
	if got.State != ceremony.LoadOK {
		t.Errorf("locked read classified the ceremony %v, want %v — a lock is not damage",
			got.State, ceremony.LoadOK)
	}
	if got.Intent == "" || len(got.Roster) != len(rec.Roster) || got.Expires.IsZero() {
		t.Errorf("a locked read returned a hollow entry (intent=%q roster=%d expires=%v). The "+
			"criterion is that the panel RENDERS roster, position and next action while locked; "+
			"a row with an id and nothing else passes a status-code check and fails the user",
			got.Intent, len(got.Roster), got.Expires)
	}

	// Now unlock and read the same disk state. The two must agree field for field.
	unlockedSrv, v := unlockedServer(t)
	_ = v
	uts := httptest.NewServer(unlockedSrv.Handler())
	t.Cleanup(uts.Close)
	// unlockedServer sets its own configDir but HOME — which is what defaultOutputDir reads — is
	// still this test's, so both servers see the same ceremonies directory.
	unlocked := getCeremonies(t, uts.URL)
	if len(unlocked.Ceremonies) != 1 {
		t.Fatalf("setup: the unlocked read returned %d, want 1 — the comparison below needs both "+
			"sides to have read the same disk", len(unlocked.Ceremonies))
	}
	lb, _ := json.Marshal(locked.Ceremonies)
	ub, _ := json.Marshal(unlocked.Ceremonies)
	if string(lb) != string(ub) {
		t.Errorf("the locked and unlocked reads of one directory differ.\nlocked:   %s\nunlocked: %s\n"+
			"Nothing in this listing is sealed — the whole point of taking it off the gate is that "+
			"the lock was protecting nothing here", lb, ub)
	}
}

// TestTheCeremoniesListingRefusesACrossSiteRead is the half that makes the move an improvement.
//
// **This is a live defect the move closes, not a precaution.** `requireUnlocked` applies the CSRF
// check and the loopback-origin check to non-GET methods ONLY — `handleCeremonyInvites`' own doc
// comment says so in as many words, which is why that route is a POST. So while this listing sat
// behind `requireUnlocked` it had no origin check at all, and since P08.S06 a GET to it runs a
// close-out sweep: a state-changing side effect, reachable from any page in the user's browser.
//
// `requirePublicLoopback` is `originIsLoopback` and nothing else, and it refuses
// `Sec-Fetch-Site: cross-site` before the handler runs.
//
// **Driven against an UNLOCKED server, deliberately.** The first cut used a locked one and its
// red proof failed to replay: under the mutation that puts this route back behind
// `requireUnlocked`, a locked server answers 401 to the SAME-ORIGIN read too, so the test tripped
// its own setup assertion and never reached the cross-site one. The origin refusal is a property
// independent of the lock and has to be driven where the lock cannot mask it.
func TestTheCeremoniesListingRefusesACrossSiteRead(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	srv, _ := unlockedServer(t)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	rec, _, _ := ceremonyOnDisk(t)

	// SETUP: a same-origin read works and sees the ceremony, so the refusal below is about the
	// origin and not about an empty directory or a broken route.
	if n := len(getCeremonies(t, ts.URL).Ceremonies); n != 1 {
		t.Fatalf("setup: a same-origin read returned %d ceremony/ies, want 1", n)
	}

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/ceremonies", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("a cross-site GET to /api/ceremonies returned %d, want 403. It cannot read the "+
			"response, but the request executes — and it runs a close-out sweep, which moves "+
			"ceremony directories and drops vault pins. A state-changing side effect on a GET "+
			"with no origin check is reachable from any page the user has open", resp.StatusCode)
	}
	// And nothing moved. The status code alone would pass against a handler that ran and then
	// wrote 403.
	live, err := ceremony.MirrorDir(defaultOutputDir(), rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, serr := os.Stat(live); serr != nil {
		t.Errorf("the ceremony directory is gone after a REFUSED cross-site read (%v) — the "+
			"refusal came after the side effect, which is a refusal that reports the damage it "+
			"failed to prevent", serr)
	}
}

// TestALockedReadRunsNoCloseOutSweep.
//
// The sweep moves directories and drops vault pins; it needs a vault and it must not half-run
// without one. `closeOutEnded` returns on a nil vault, and `handleCeremonies` calls it only inside
// `if v := s.unlockedVault(); v != nil` — two guards for one rule, which is worth an assertion
// precisely because either alone looks sufficient to a reader.
//
// **Driven with a ceremony the sweep WOULD close out**, or the assertion is that nothing happened
// to something nothing was going to happen to.
func TestALockedReadRunsNoCloseOutSweep(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	srv := New(os.DirFS("."), os.DirFS("."), t.TempDir(), "test")
	srv.instanceToken = "primary"
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	rec, _, _ := ceremonyOnDisk(t)
	// Make it closeable: a delivered document is C11's central case, so an UNLOCKED sweep would
	// move this one. Without that, a directory still present afterwards proves nothing.
	path := deliveredPathFor(rec)
	if err := os.MkdirAll(strings.TrimSuffix(path, "/"+filepathBase(path)), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("the finished document"), 0o600); err != nil {
		t.Fatal(err)
	}
	st := ceremony.Stored{ID: rec.ID, State: ceremony.LoadOK, Expires: time.Now().Add(48 * time.Hour)}
	if _, closed := closeOutReason(st, rec, rec.Roster[1].Fingerprint, time.Now()); !closed {
		t.Fatal("setup: this ceremony would NOT be closed out even by an unlocked sweep, so its " +
			"survival below says nothing about the lock")
	}

	if n := len(getCeremonies(t, ts.URL).Ceremonies); n != 1 {
		t.Fatalf("setup: the locked read returned %d, want 1", n)
	}
	live, err := ceremony.MirrorDir(defaultOutputDir(), rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, serr := os.Stat(live); serr != nil {
		t.Errorf("a LOCKED read closed out a ceremony (%v). The sweep drops vault pins and it "+
			"cannot open the vault to do so, which is a half-teardown: the folder moves and the "+
			"key material stays", serr)
	}
}

// getCeremonies reads the listing route and decodes it, failing the test on anything but 200.
func getCeremonies(t *testing.T, base string) ceremoniesResponse {
	t.Helper()
	resp, err := http.Get(base + "/api/ceremonies")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/ceremonies returned %d, want 200", resp.StatusCode)
	}
	var out ceremoniesResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

// filepathBase is the last path element, kept local so this file needs no extra import for one use.
func filepathBase(p string) string {
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
}
