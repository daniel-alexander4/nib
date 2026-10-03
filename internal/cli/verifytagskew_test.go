package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nib/internal/ceremony"
	"nib/internal/sign"
)

// /pending 809 and 649: a version skew excuses a PARSE, never the roster or the commitment check.
//
// # The attack
//
// `[NibCoSign:N]` lives in `/Reason`, which is text the signer types. `skew` was a DOCUMENT-wide
// flag: one signature carrying a tag newer than this build's set it, and it switched `disagrees()`
// off for every signature on the file. A newer tag also leaves the roster token unparsed, and
// `markUnrostered` only looked at signatures carrying one. So anyone holding a completed ceremony
// could append a valid signature with `/Reason "[NibCoSign:2]"` and `nib verify` exited 0 with no
// UNROSTERED line — "a version difference, not a disagreement" — about a signature nobody on the
// roster made.
//
// # What a skew may still excuse
//
// A rostered party on a newer Nib is the case D32 exists for, and it must keep exiting 0: that is
// `TestTheCeremonyVerdictRefusesWhatItUsedToCallComplete`'s anti-proof, driven here at the
// artifact rather than on a hand-built report. What it may not do is vouch for anybody else.

// signRaw appends an approval signature by a fresh key carrying exactly `reason` — the signer's
// own text, which is all an attacker needs.
func signRaw(t *testing.T, doc []byte, name, reason string) []byte {
	t.Helper()
	c, k, err := sign.GenerateIdentity(name)
	if err != nil {
		t.Fatal(err)
	}
	return signRawAs(t, doc, c, k, name, reason)
}

func signRawAs(t *testing.T, doc, cert, key []byte, name, reason string) []byte {
	t.Helper()
	out, err := sign.SignApproval(doc, cert, key, sign.Options{
		Name: name, Reason: reason, When: time.Date(2026, 8, 24, 1, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func writeDoc(t *testing.T, dir, name string, b []byte) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestANewerTagExcusesAParseNeverTheRosterOrTheCommitment(t *testing.T) {
	now := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)

	t.Run("an off-roster signature tagged [NibCoSign:2] refuses", func(t *testing.T) {
		dir := t.TempDir()
		path := convenedFixture(t, dir, 3, 3)
		// SETUP: the completed ceremony passes, or the refusal below is about the fixture.
		if cer := reportOf(t, path, now); cer.refuses() {
			t.Fatal("setup: the completed ceremony already refuses")
		}
		doc, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		signed := signRaw(t, doc, "Mallory", "[NibCoSign:2] I am a party to this ceremony")
		bad := writeDoc(t, dir, "mallory.pdf", signed)

		// SETUP: the signature verifies and the document still reads 3 of 3 — so nothing but the
		// ceremony verdict stands between it and exit 0.
		if st := sign.Verify(signed); st.State != sign.Valid || st.AddedAfter || len(st.Signers) != 4 {
			t.Fatalf("setup: state=%v addedAfter=%v signers=%d", st.State, st.AddedAfter, len(st.Signers))
		}
		cer := reportOf(t, bad, now)
		if cer.signed != 3 || cer.obliged != 3 {
			t.Fatalf("setup: %d of %d", cer.signed, cer.obliged)
		}

		out, code := captureStdout(t, func() int { return cmdVerify([]string{bad}) })
		if code == 0 {
			t.Errorf("nib verify exited 0 on a completed ceremony carrying a valid signature from "+
				"a key the roster does not name, because its /Reason said [NibCoSign:2] — a tag the "+
				"signer typed. A newer tag excuses a parse, never roster membership:\n%s", out)
		}
		if !strings.Contains(out, "UNROSTERED") || !strings.Contains(out, "Mallory") {
			t.Errorf("the intruder is not named — the reader's next action is about a person:\n%s", out)
		}
		if j := cer.json(); j == nil || j.Complete {
			t.Error("--json still reports complete:true for it")
		}
	})

	t.Run("a rostered party's newer tag does not silence the commitment check on another signature", func(t *testing.T) {
		// Bob, ON the roster, signs from a newer Nib — the genuine skew. Then a stranger appends a
		// plain signature that claims nothing. Without the skew that stranger fails the commitment
		// check (`markOneProceeding` fails closed on an empty commitment, which is the population
		// `markUnrostered` deliberately leaves to it); with the skew as a DOCUMENT flag, Bob's
		// upgrade excused the stranger too.
		dir := t.TempDir()
		_, ps, rec := twoPartyCeremony(t, dir, now)
		doc := signRawAs(t, rec.doc, ps[1].cert, ps[1].key, "Bob Landlord",
			"[NibCoSign:2] a format this build does not know")
		skewOnly := writeDoc(t, dir, "skew.pdf", doc)
		// SETUP: the skew alone passes — this is the anti-proof at the artifact, and the reason the
		// fix is per-signature rather than "a newer tag refuses".
		if cer := reportOf(t, skewOnly, now); cer.skew == "" || cer.refuses() || cer.signed != 2 {
			t.Fatalf("setup: a rostered party on a newer Nib must read as a skew and pass "+
				"(skew=%q refuses=%v signed=%d)", cer.skew, cer.refuses(), cer.signed)
		} else if j := cer.json(); j == nil || !j.Complete {
			t.Fatal("setup: the genuine skew's JSON is not complete — one predicate must feed both")
		}

		bad := writeDoc(t, dir, "skewplus.pdf", signRaw(t, doc, "Mallory", "Approved"))
		cer := reportOf(t, bad, now)
		// SETUP: not unrostered — the stranger claims nothing — so only the commitment check can
		// catch it, and this arm discriminates that clause.
		if cer.hasUnrostered() {
			t.Fatalf("setup: the plain signature was flagged unrostered (%v)", cer.unrostered)
		}
		if !cer.disagrees() {
			t.Errorf("a signature committing to nothing was excused because ANOTHER party's tag "+
				"was newer (skew=%q claimed=%d)", cer.skew, cer.claimed)
		}
		if !cer.refuses() {
			t.Error("…and it exited 0")
		}
		lines := strings.Join(cer.lines(), "\n")
		if !strings.Contains(lines, "do NOT all commit") || !strings.Contains(lines, "newer version") {
			t.Errorf("the text must say BOTH facts — the skew and the disagreement:\n%s", lines)
		}
	})
}

// TestARecordFormatSkewStillExcusesTheSignatureItDescribes is the second D32 discriminator's
// anti-proof at the artifact: a rostered party whose commitment is in another record format is a
// version difference and passes — the per-signature excuse must cover it, or the fix for 809
// re-creates the accusation /pending 324 removed.
func TestARecordFormatSkewStillExcusesTheSignatureItDescribes(t *testing.T) {
	now := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	_, ps, rec := twoPartyCeremony(t, dir, now)
	doc := signRawAs(t, rec.doc, ps[1].cert, ps[1].key, "Bob Landlord",
		fmt.Sprintf("[NibCoSign:1] [NibRoster:%d:%s]", ceremony.FormatVersion-1, strings.Repeat("cd", 32)))
	cer := reportOf(t, writeDoc(t, dir, "format.pdf", doc), now)
	if cer.skew == "" || cer.hasUnrostered() {
		t.Fatalf("setup: want a format skew from a rostered party (skew=%q unrostered=%v)",
			cer.skew, cer.unrostered)
	}
	if cer.refuses() {
		t.Error("a rostered party's commitment in another record format was refused — a version " +
			"difference, not a disagreement (D32)")
	}
}

// TestTheJSONCompleteAndTheExitCodeAreOnePredicate is /pending 649: `complete` folded in
// `hasUnrostered()` and dropped `disagrees()`, so a document whose signatures name different
// proceedings exited 2 and said "complete": true to a script.
func TestTheJSONCompleteAndTheExitCodeAreOnePredicate(t *testing.T) {
	now := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	_, ps, rec := twoPartyCeremony(t, dir, now)
	// Bob is ON the roster and commits to a different proceeding: a disagreement, and nothing else.
	doc := signRawAs(t, rec.doc, ps[1].cert, ps[1].key, "Bob Landlord",
		fmt.Sprintf("[NibCoSign:1] [NibRoster:%d:%s]", ceremony.FormatVersion, strings.Repeat("ab", 32)))
	cer := reportOf(t, writeDoc(t, dir, "two.pdf", doc), now)
	if !cer.disagrees() || cer.hasUnrostered() || cer.incomplete() {
		t.Fatalf("setup: want a disagreement alone (disagrees=%v unrostered=%v incomplete=%v)",
			cer.disagrees(), cer.unrostered, cer.incomplete())
	}
	if !cer.refuses() {
		t.Fatal("setup: the exit code does not refuse it")
	}
	if j := cer.json(); j == nil || j.Complete {
		t.Error("--json says complete:true about a document the exit code refuses — a script " +
			"reading `.complete` waves through what the text output calls a disagreement")
	}
}
