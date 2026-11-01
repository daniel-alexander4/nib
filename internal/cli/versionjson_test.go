package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestVersionJSONShapeIsLocked — ADR-183. A program pins `api`, so the object's keys, their types and the
// verbs it lists are a contract: this fails on a rename or a removal, which is the change that must raise
// APIVersion. A new verb is an addition — extend the list here and leave APIVersion alone.
func TestVersionJSONShapeIsLocked(t *testing.T) {
	out, code := captureStdout(t, func() int { _, c := Run([]string{"version", "--json"}, "9.8.7"); return c })
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		t.Fatalf("not one JSON object: %v\n%s", err, out)
	}
	if len(raw) != 3 {
		t.Errorf("keys = %d, want exactly version, api, commands: %s", len(raw), out)
	}
	var got struct {
		Version  string   `json:"version"`
		API      int      `json:"api"`
		Commands []string `json:"commands"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	if got.Version != "9.8.7" || got.API != 1 {
		t.Errorf("version %q api %d, want 9.8.7 and 1 (APIVersion is a number a program has pinned: raise it for an incompatible change only)", got.Version, got.API)
	}
	const want = "attachments booklet contents decrypt discover encrypt export-xfdf fields fill flatten images info merge normalize nup office optimize outline pagelabels pagenum pages pdfa register rendezvous rotate sanitize shrink sign split stamp tag text timestamp ua unregister verify version watch"
	if have := strings.Join(got.Commands, " "); have != want {
		t.Errorf("commands = %q\n want     %q", have, want)
	}
	plain, _ := captureStdout(t, func() int { _, c := Run([]string{"version"}, "9.8.7"); return c })
	if plain != "nib 9.8.7\n" {
		t.Errorf("plain version = %q, want it unchanged", plain)
	}
	if stderr := captureStderr(t, func() {
		_, c := Run([]string{"version", "--bogus"}, "9.8.7")
		if c != 1 {
			t.Errorf("version --bogus exited %d, want 1", c)
		}
	}); !strings.Contains(stderr, "unknown argument") {
		t.Errorf("version --bogus said %q", stderr)
	}
}
