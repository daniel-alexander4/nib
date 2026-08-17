package instance

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// P07.S01 — the rendezvous. A second launch cannot find the first one without this:
// the server binds 127.0.0.1:0, so there is no fixed port to knock on.

func TestCreateRefusesASecondRecordAndKeepsTheFirst(t *testing.T) {
	dir := t.TempDir()
	first := Record{Addr: "127.0.0.1:4001", Token: "aaa", Version: "test"}
	if err := Create(dir, first); err != nil {
		t.Fatalf("the first Create failed: %v", err)
	}
	// The stimulus: a record really is there, so the refusal below is about O_EXCL and
	// not about a write that failed for some other reason.
	if got, err := Read(dir); err != nil || got.Addr != first.Addr {
		t.Fatalf("setup: the record did not land (%+v, %v)", got, err)
	}

	err := Create(dir, Record{Addr: "127.0.0.1:4002", Token: "bbb", Version: "test"})
	if err != ErrExists {
		t.Fatalf("a second Create returned %v, want ErrExists", err)
	}
	// **And the incumbent is untouched.** This is the half that matters: a
	// last-writer-wins create would also "succeed", and would leave the first instance
	// serving on a port nothing points at while the record named the second.
	got, err := Read(dir)
	if err != nil {
		t.Fatalf("reading after the refused Create: %v", err)
	}
	if got.Addr != first.Addr || got.Token != first.Token {
		t.Errorf("the refused Create overwrote the incumbent: got %+v, want %+v", got, first)
	}
}

func TestTheRecordIsPrivateToItsOwner(t *testing.T) {
	if os.Getenv("GOOS") == "windows" {
		t.Skip("POSIX mode bits do not describe Windows ACLs")
	}
	dir := t.TempDir()
	if err := Create(dir, Record{Addr: "127.0.0.1:4001", Token: "secret", Version: "test"}); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(Path(dir))
	if err != nil {
		t.Fatal(err)
	}
	// The token is a secret. 0600 is not decoration: a world-readable rendezvous hands
	// every local user the identity of this instance.
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("the instance record is mode %04o, want 0600 — the probe token is readable by others", perm)
	}
}

func TestRemoveIsIdempotentAndReadRejectsAPartialRecord(t *testing.T) {
	dir := t.TempDir()
	if err := Remove(dir); err != nil {
		t.Errorf("removing an absent record returned %v, want nil — a crash-then-cleanup and a clean exit must not be distinguishable", err)
	}
	if err := os.WriteFile(Path(dir), []byte(`{"addr":"127.0.0.1:9"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	// A record with no token cannot authenticate a probe, so it is not a record. Read
	// refuses it rather than handing the caller something that will silently fail later.
	if _, err := Read(dir); err == nil {
		t.Error("Read accepted a record with no token — a probe built from it can never succeed, and the caller would read that as a stale instance")
	}
}

// TestProbeAnswersForALiveInstanceAndNotForAStaleOne is the row the whole slice turns
// on, and both halves are asserted because they fail differently.
func TestProbeAnswersForALiveInstanceAndNotForAStaleOne(t *testing.T) {
	const tok = "the-right-token"
	live := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/instance" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if !TokenMatches(r.Header.Get(HeaderToken), tok) {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Write([]byte(`{"ok":true}`))
	}))
	defer live.Close()
	addr := strings.TrimPrefix(live.URL, "http://")

	if !Probe(Record{Addr: addr, Token: tok}) {
		t.Fatal("a live instance did not answer its own probe — every assertion below is meaningless without this")
	}

	// **A port that answers is not enough.** A random port freed by a dead Nib is
	// reassigned to whatever asks next, so a bare connect would say "alive" to an
	// unrelated service and a launch would hand it a document path.
	if Probe(Record{Addr: addr, Token: "the-wrong-token"}) {
		t.Error("the probe accepted a wrong token — it is testing that SOMETHING is listening, not that it is our instance")
	}

	// The ordinary stale case: the instance exited, the record outlived it.
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadAddr := strings.TrimPrefix(dead.URL, "http://")
	dead.Close()
	if Probe(Record{Addr: deadAddr, Token: tok}) {
		t.Error("the probe reported a closed instance as alive")
	}
}

// TestProbeRefusesANonLoopbackRecord — a record is read off disk, and a tampered or
// hand-edited one must not turn a launch into an outbound request. Nib is never
// network-exposed, and that includes as a client.
func TestProbeRefusesANonLoopbackRecord(t *testing.T) {
	reached := false
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.Write([]byte(`{"ok":true}`))
	}))
	defer remote.Close()
	// The stimulus: this server DOES answer a probe when addressed as loopback, so a
	// refusal below is the address check and not an unreachable host.
	if !Probe(Record{Addr: strings.TrimPrefix(remote.URL, "http://"), Token: "t"}) {
		t.Skip("the test server is not on a loopback address; nothing to distinguish")
	}
	reached = false

	for _, addr := range []string{"203.0.113.5:1234", "example.com:80", "[2001:db8::1]:80"} {
		if Probe(Record{Addr: addr, Token: "t"}) {
			t.Errorf("the probe accepted the non-loopback address %q", addr)
		}
	}
	if reached {
		t.Error("a non-loopback probe reached a server — the address check runs after the request rather than before it")
	}
}

func TestNewTokenIsLongAndDoesNotRepeat(t *testing.T) {
	a, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Error("two tokens are identical — the record would not identify one instance from another")
	}
	if len(a) < 32 {
		t.Errorf("token is %d characters; too short to be worth comparing in constant time", len(a))
	}
}

func TestPathIsInsideTheGivenDirectory(t *testing.T) {
	dir := t.TempDir()
	if got, want := Path(dir), filepath.Join(dir, Name); got != want {
		t.Errorf("Path = %q, want %q", got, want)
	}
}

// TestTheRecordDoesNotOutliveASIGTERM is a source guard, and it exists because the
// signal that kills Nib in ordinary use is the one Go does not handle by default.
//
// `build/nib.desktop` passes `--replace` on every activation and
// `singleton.ReplaceOthers` sends SIGTERM. An unhandled SIGTERM terminates without
// running a single deferred function, so the instance record outlives its instance —
// every desktop double-click leaving one behind, pointing at a dead process, while the
// launch that killed it finds ErrExists and publishes nothing. Found by P07's plan
// review; the behaviour itself is driven live in the slice's verification, and this
// guard is what notices if the signal list is ever trimmed back.
func TestTheRecordDoesNotOutliveASIGTERM(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "cmd", "nib", "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	// The stimulus: the notify call must be there at all, or the assertion below is a
	// scan over a file that stopped handling signals.
	if !strings.Contains(text, "signal.NotifyContext(") {
		t.Fatal("main no longer installs a signal handler — this guard is reading the wrong thing")
	}
	if !strings.Contains(text, "syscall.SIGTERM") {
		t.Error("main does not handle SIGTERM, so a --replace launch kills this process without running its deferred cleanup and the instance record outlives the instance")
	}
	// And run() must still RETURN rather than os.Exit, or the defers never fire anyway.
	if !strings.Contains(text, "func main() { os.Exit(run()) }") {
		t.Error("main no longer wraps run() — os.Exit skips deferred functions, so the record's removal would not run on the error path")
	}
}
