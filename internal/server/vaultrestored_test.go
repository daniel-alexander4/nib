package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nib/internal/sshkey"
	"nib/internal/vault"
)

// ADR-159, /pending 873 part 2 — seam B2: vault → server → window.

// nextVaultNotice reads the next `event: vault` frame, or reports that none came within wait.
func nextVaultNotice(t *testing.T, lines <-chan lineOrErr, wait time.Duration) (noticeView, bool) {
	t.Helper()
	deadline := time.After(wait)
	seen := false
	for {
		select {
		case <-deadline:
			return noticeView{}, false
		case l := <-lines:
			if l.err != nil {
				t.Fatalf("read stream: %v", l.err)
			}
			switch {
			case l.s == "event: vault":
				seen = true
			case seen && strings.HasPrefix(l.s, "data: "):
				var n noticeView
				if err := json.Unmarshal([]byte(strings.TrimPrefix(l.s, "data: ")), &n); err != nil {
					t.Fatalf("vault event did not decode: %v (%q)", err, l.s)
				}
				return n, true
			}
		}
	}
}

// TestAnOlderCopyWrittenOverTheVaultIsSaidToEveryWindow: the vault's record reaches a window that
// is open when the restore happens, and one that opens afterwards — the other writer is still
// running in both cases — and a vault nothing has written over says nothing.
func TestAnOlderCopyWrittenOverTheVaultIsSaidToEveryWindow(t *testing.T) {
	ts, s := startServerWith(t)
	authedClient(t, ts)
	v := s.unlockedVault()
	if v == nil {
		t.Fatal("setup: no vault is open")
	}
	// A second key, removed again: the only way a vault's generation leaves 0.
	keyPath := filepath.Join(t.TempDir(), "second")
	pub, err := sshkey.Generate(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := v.AddKey(pub, keyPath); err != nil {
		t.Fatal(err)
	}
	file := vault.Path(s.configDir)
	older, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if err := v.RemoveKey(pub); err != nil {
		t.Fatal(err)
	}

	open := listenAsAWindow(t, ts)
	waitForWindows(t, s, 1)
	if n, ok := nextVaultNotice(t, open, 300*time.Millisecond); ok {
		t.Fatalf("a vault nothing has written over told the window %+v", n)
	}

	// The older writer: the file as it was before the removal, written with no lock.
	if err := os.WriteFile(file, older, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := v.AddRecent("/any-write-at-all.pdf"); err != nil {
		t.Fatal(err)
	}
	got, ok := nextVaultNotice(t, open, streamWait(t))
	if !ok {
		t.Fatal("the open window heard nothing after the vault put itself back over an older copy — " +
			"the other Nib goes on writing its old vault and nobody is told to close it")
	}
	_, at := v.OlderCopyRestored()
	if got.What != vaultRestoredWhat || got.Summary != vaultRestoredSummary || got.Detail != vaultRestoredDetail || !got.At.Equal(at) {
		t.Errorf("the window heard %+v, want %q with the vault's own time %v", got, vaultRestoredWhat, at)
	}

	// An ordinary write afterwards is not a second event.
	if err := v.AddRecent("/another.pdf"); err != nil {
		t.Fatal(err)
	}
	if n, ok := nextVaultNotice(t, open, 300*time.Millisecond); ok {
		t.Errorf("an ordinary write after the restore was announced again: %+v", n)
	}

	late := listenAsAWindow(t, ts)
	waitForWindows(t, s, 2)
	if n, ok := nextVaultNotice(t, late, streamWait(t)); !ok || !n.At.Equal(at) {
		t.Errorf("a window opened after the restore heard %+v (arrived %v), want the same notice", n, ok)
	}
}

// TestTheVaultNoticeNamesBothThingsALowerGenerationCanBe — the file does not say whether an older
// Nib wrote its copy back or another Nib imported an old backup, so the words cover both and say
// what to do about each.
func TestTheVaultNoticeNamesBothThingsALowerGenerationCanBe(t *testing.T) {
	said := vaultRestoredSummary + " — " + vaultRestoredDetail
	for _, want := range []string{"older copy", "key was removed", "put the removal back", "close any other Nib", "imported a backup", "import it again"} {
		if !strings.Contains(said, want) {
			t.Errorf("the notice does not say %q: %s", want, said)
		}
	}
	if n := vaultRestoredNotice(nil); n != nil {
		t.Errorf("no open vault gave a notice: %+v", n)
	}
}
