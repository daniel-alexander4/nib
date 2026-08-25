package vault

import (
	"bytes"
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"
)

// TestCeremonySecretsSurviveAReopen — the acceptance clause, driven through DISK.
//
// "Re-readable after a restart" is satisfiable by reading back through the same in-memory
// handle, which proves the map and not the file. This reopens from the path.
func TestCeremonySecretsSurviveAReopen(t *testing.T) {
	dir := t.TempDir()
	pub, keyPath := newKey(t)
	v, err := Create(dir, pub, keyPath)
	if err != nil {
		t.Fatal(err)
	}
	const id = "0123456789abcdef0123456789abcdef"
	fpA := bytes.Repeat([]byte{0x11}, 32)
	fpB := bytes.Repeat([]byte{0x22}, 32)
	secA := bytes.Repeat([]byte{0xAA}, 32)
	secB := bytes.Repeat([]byte{0xBB}, 32)

	if err := v.AddCeremonySecret(id, fpA, secA); err != nil {
		t.Fatal(err)
	}
	if err := v.AddCeremonySecret(id, fpB, secB); err != nil {
		t.Fatal(err)
	}
	// Stimulus: they are there BEFORE the reopen, or the reopen below proves nothing about
	// persistence — it would be comparing two absences.
	if got := v.CeremonySecrets(id); len(got) != 2 {
		t.Fatalf("setup: %d secrets held before the reopen, want 2", len(got))
	}

	v2, err := OpenSSH(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := v2.CeremonySecret(id, fpA)
	if !ok {
		t.Fatal("party A's secret did not survive a reopen — the convener cannot re-issue an " +
			"invitation, so closing the tab makes the ceremony unrecoverable rather than stalled")
	}
	if !bytes.Equal(got, secA) {
		t.Errorf("party A's secret came back as %x, want %x", got[:4], secA[:4])
	}
	if _, ok := v2.CeremonySecret(id, fpB); !ok {
		t.Error("party B's secret did not survive a reopen")
	}
	// A fingerprint nobody stored, and a ceremony nobody convened, are both misses rather
	// than the first row.
	if _, ok := v2.CeremonySecret(id, bytes.Repeat([]byte{0x33}, 32)); ok {
		t.Error("an unknown fingerprint returned a secret")
	}
	if _, ok := v2.CeremonySecret("ffffffffffffffffffffffffffffffff", fpA); ok {
		t.Error("a different ceremony's id returned this ceremony's secret")
	}
}

func TestCeremonySecretsUpsertAndPrune(t *testing.T) {
	dir := t.TempDir()
	pub, keyPath := newKey(t)
	v, err := Create(dir, pub, keyPath)
	if err != nil {
		t.Fatal(err)
	}
	const id = "0123456789abcdef0123456789abcdef"
	const other = "ffffffffffffffffffffffffffffffff"
	fp := bytes.Repeat([]byte{0x11}, 32)

	if err := v.AddCeremonySecret(id, fp, bytes.Repeat([]byte{0xAA}, 32)); err != nil {
		t.Fatal(err)
	}
	if err := v.AddCeremonySecret(id, fp, bytes.Repeat([]byte{0xCC}, 32)); err != nil {
		t.Fatal(err)
	}
	if got := v.CeremonySecrets(id); len(got) != 1 {
		t.Errorf("re-issuing to one party left %d rows for one seat — only one would ever be "+
			"found again", len(got))
	}
	if got, _ := v.CeremonySecret(id, fp); !bytes.Equal(got, bytes.Repeat([]byte{0xCC}, 32)) {
		t.Error("re-issuing did not replace the secret")
	}

	// A second ceremony must be untouched by the first's prune.
	if err := v.AddCeremonySecret(other, fp, bytes.Repeat([]byte{0xDD}, 32)); err != nil {
		t.Fatal(err)
	}
	n, perr := v.PruneCeremonySecrets(id)
	if perr != nil {
		t.Fatal(perr)
	}
	if n != 1 {
		t.Errorf("prune removed %d, want 1", n)
	}
	if _, ok := v.CeremonySecret(id, fp); ok {
		t.Error("the pruned ceremony's secret is still there")
	}
	if _, ok := v.CeremonySecret(other, fp); !ok {
		t.Error("pruning one ceremony took another ceremony's secret with it")
	}
	// An empty id would match nothing and delete nothing — but the same shape on the pinned
	// peers would have matched EVERY user pin, so both doors refuse it by name.
	if _, err := v.PruneCeremonySecrets(""); err == nil {
		t.Error("prune accepted an empty ceremony id")
	}
	if err := v.AddCeremonySecret("", fp, bytes.Repeat([]byte{0xEE}, 32)); err == nil {
		t.Error("a secret was filed under an empty ceremony id — nothing could ever prune it")
	}
}

// TestAPayloadFromANewerNibIsRefusedRatherThanSilentlyRewritten — the Contents version gate.
//
// checkEnvelopeVersion's own comment says why this matters and applies to the envelope only:
// encoding/json discards unknown keys, so an older build that opens and re-saves drops
// everything it does not know. Measured before this gate existed: a payload carrying a
// `ceremonySecrets` key opened on a build without the field, and one AddRecent — i.e. opening
// any PDF — rewrote the file without it. The secrets are the only copy.
func TestAPayloadFromANewerNibIsRefusedRatherThanSilentlyRewritten(t *testing.T) {
	// Stimulus: an ordinary payload of THIS version decodes.
	ours, err := json.Marshal(Contents{Version: contentsVersion, Recent: []string{"/tmp/a.pdf"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeContents(ours); err != nil {
		t.Fatalf("setup: this build's own payload was refused (%v), so the refusal below "+
			"cannot distinguish a version gate from a broken decoder", err)
	}
	// A payload with no version at all is one this build wrote before the field existed.
	old, err := json.Marshal(Contents{Recent: []string{"/tmp/a.pdf"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeContents(old); err != nil {
		t.Errorf("a payload predating the version field was refused (%v) — those are this "+
			"build's own and refusing them locks users out of their own vault", err)
	}

	newer, err := json.Marshal(Contents{Version: contentsVersion + 1})
	if err != nil {
		t.Fatal(err)
	}
	_, err = decodeContents(newer)
	if err == nil {
		t.Fatal("a payload from a newer Nib was accepted; the next save would drop every field " +
			"this build does not know, and for a ceremony secret that is the only copy")
	}
	if !strings.Contains(err.Error(), "newer version of Nib") {
		t.Errorf("the refusal does not tell the user what to do about it: %v", err)
	}
}

// TestEveryContentsDecodeGoesThroughTheDoor — ADR-009, asserted on the ROUTING.
//
// The payload is unmarshalled at four places. A version gate applied at three of them is a
// gate that does not exist, and the way that happens is somebody adding a fifth site. This
// asserts the door is the only unmarshaller, rather than asserting each site's behaviour —
// which is what the ADR asks for and what a per-site test cannot give.
func TestEveryContentsDecodeGoesThroughTheDoor(t *testing.T) {
	// **Every non-test file in the package, and the rule is about the TYPE, not a spelling.**
	//
	// The first draft counted the literal `json.Unmarshal(plain, &c)` in vault.go alone.
	// Measured at the slice's diff review: a second, ungated unmarshaller with its receiver
	// named `out` evaded it completely, and `builtin.go` was never read at all. The property
	// is "a Contents payload is decoded in exactly one function", so the scan looks for
	// function bodies that unmarshal AND mention Contents.
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	found, offenders := 0, []string{}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		raw, rerr := os.ReadFile(e.Name())
		if rerr != nil {
			t.Fatal(rerr)
		}
		src := stripComments(string(raw))
		for _, fn := range splitFuncs(src) {
			if !strings.Contains(fn.body, "json.Unmarshal") {
				continue
			}
			// A function that CALLS the door is routing through it, not bypassing it —
			// `Validate` unmarshals the ENVELOPE and then calls decodeContents, and a naive
			// "mentions Contents" test flags it. Strip the call before asking.
			body := strings.ReplaceAll(fn.body, "decodeContents", "")
			if !strings.Contains(body, "Contents") {
				continue // unmarshals something else entirely
			}
			found++
			if fn.name != "decodeContents" {
				offenders = append(offenders, e.Name()+":"+fn.name)
			}
		}
	}
	// Stimulus: if NO function was found decoding a Contents payload, the scan is matching
	// nothing and a clean result means nothing.
	if found == 0 {
		t.Fatal("no function in this package was found unmarshalling a Contents payload — " +
			"the scan matched nothing, so its pass is empty")
	}
	if len(offenders) > 0 {
		t.Errorf("these decode a Contents payload outside decodeContents: %v. Exactly one "+
			"function may, because that is where the version gate lives — a vault that opens "+
			"a newer payload anywhere else silently rewrites it and drops what it cannot read.",
			offenders)
	}
	if !strings.Contains(string(mustRead(t, "vault.go")), "func decodeContents(") {
		t.Error("decodeContents is gone; the version gate has no door")
	}
}

func mustRead(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

type goFunc struct{ name, body string }

// splitFuncs returns each top-level function's name and brace-matched body.
func splitFuncs(src string) []goFunc {
	var out []goFunc
	re := regexp.MustCompile(`(?m)^func (?:\([^)]*\) )?(\w+)\(`)
	for _, m := range re.FindAllStringSubmatchIndex(src, -1) {
		name := src[m[2]:m[3]]
		rest := src[m[0]:]
		open := strings.Index(rest, "{")
		if open < 0 {
			continue
		}
		depth := 0
		for j := open; j < len(rest); j++ {
			switch rest[j] {
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					out = append(out, goFunc{name: name, body: rest[open : j+1]})
					j = len(rest)
				}
			}
		}
	}
	return out
}

// stripComments removes `//` tails so a scan cannot be satisfied — or evaded — by prose.
func stripComments(src string) string {
	var b strings.Builder
	for _, line := range strings.Split(src, "\n") {
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

// TestCeremonySecretsAreDetachedAndScrubbed — the two properties whose doc comments carry the
// most weight in this file, and which nothing checked until the slice's diff review said so.
//
// Measured then: removing BOTH zero() calls and returning the vault's own backing arrays
// instead of copies left every test green.
func TestCeremonySecretsAreDetachedAndScrubbed(t *testing.T) {
	dir := t.TempDir()
	pub, keyPath := newKey(t)
	v, err := Create(dir, pub, keyPath)
	if err != nil {
		t.Fatal(err)
	}
	const id = "0123456789abcdef0123456789abcdef"
	fp := bytes.Repeat([]byte{0x11}, 32)
	secret := bytes.Repeat([]byte{0xAA}, 32)
	if err := v.AddCeremonySecret(id, fp, secret); err != nil {
		t.Fatal(err)
	}

	t.Run("the caller cannot reach the vault's copy", func(t *testing.T) {
		got, ok := v.CeremonySecret(id, fp)
		if !ok {
			t.Fatal("setup: the secret is not there")
		}
		for i := range got {
			got[i] = 0xFF
		}
		again, _ := v.CeremonySecret(id, fp)
		if bytes.Equal(again, got) {
			t.Error("mutating a returned secret changed the vault's own copy — the return is " +
				"aliased, so anything downstream can corrupt key material without going " +
				"through save()")
		}
		if !bytes.Equal(again, secret) {
			t.Errorf("the vault's secret is now %x…, want %x…", again[:4], secret[:4])
		}
	})

	t.Run("the argument is copied, not retained", func(t *testing.T) {
		arg := bytes.Repeat([]byte{0xBB}, 32)
		if err := v.AddCeremonySecret(id, bytes.Repeat([]byte{0x22}, 32), arg); err != nil {
			t.Fatal(err)
		}
		for i := range arg {
			arg[i] = 0
		}
		got, ok := v.CeremonySecret(id, bytes.Repeat([]byte{0x22}, 32))
		if !ok || bytes.Equal(got, arg) {
			t.Error("the vault retained the CALLER's slice — zeroing the caller's buffer, " +
				"which is what a careful caller does, would empty the vault's copy too")
		}
	})

	t.Run("a pruned secret is scrubbed", func(t *testing.T) {
		fp3 := bytes.Repeat([]byte{0x33}, 32)
		sec3 := bytes.Repeat([]byte{0xCC}, 32)
		if err := v.AddCeremonySecret(id, fp3, sec3); err != nil {
			t.Fatal(err)
		}
		// The vault's own backing array, reached the only way a test can: through the
		// unexported contents. Held across the prune so the scrub is observable.
		v.mu.Lock()
		var held []byte
		for _, s := range v.contents.CeremonySecrets {
			if bytes.Equal(s.Fingerprint, fp3) {
				held = s.Secret
			}
		}
		v.mu.Unlock()
		if held == nil || !bytes.Equal(held, sec3) {
			t.Fatal("setup: could not reach the vault's own copy, so a scrub would be invisible")
		}
		if _, err := v.PruneCeremonySecrets(id); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(held, make([]byte, len(held))) {
			t.Errorf("the pruned secret's backing array still reads %x… — key material dropped "+
				"to the GC with its plaintext intact", held[:4])
		}
	})
}
