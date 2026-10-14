package ceremony

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"nib/internal/p2p"
	"nib/internal/pdfops"
	"nib/internal/testpdf"
)

// TestADigestVersionSkewSaysSoRatherThanAccusing — D32 applied to the content-digest rule.
//
// `pdfops.ContentDigestVersion`'s own doc says it exists so that "improving the coverage"
// does not "accuse a counterparty of tampering". **Measured at the P07.S02 grill: it could
// not do that**, because it was bound INTO the digest and carried nowhere beside it — three
// occurrences in the whole tree, no reader. Binding a version inside a hash changes the
// number; it cannot produce a sentence, because nothing has anything to compare.
//
// The failure this prevents is a Nib point release telling a solicitor that a counterparty
// substituted their document. The two outcomes are opposite in meaning and must be opposite
// in wording.
func TestADigestVersionSkewSaysSoRatherThanAccusing(t *testing.T) {
	cert, key, cfp := identity(t, "Convener")
	_, _, afp := identity(t, "A")
	base, err := testpdf.Text("the lease")
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := p2p.PrepareDocument(base)
	if err != nil {
		t.Fatal(err)
	}
	h, err := DocumentHash(prepared)
	if err != nil {
		t.Fatal(err)
	}
	r := draft(t, cfp, afp)
	r.DocHash = h
	if err := r.Sign(cert, key); err != nil {
		t.Fatal(err)
	}
	doc, err := Embed(prepared, r)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)
	// Setup assertion: it must pass BEFORE the skew, or the refusal below proves nothing.
	if _, err := CheckDocument(doc, now); err != nil {
		t.Fatalf("setup: a freshly convened document must pass CheckDocument: %v", err)
	}

	// The skew: a record written under a digest rule this build does not use. Built by
	// re-signing rather than by editing the JSON, because DigestVersion is inside the
	// preimage — an edited copy would fail as a bad signature and prove the wrong thing.
	skewed := draft(t, cfp, afp)
	skewed.DocHash = h
	skewed.DigestVersion = pdfops.ContentDigestVersion + 1
	if err := skewed.Sign(cert, key); err != nil {
		t.Fatal(err)
	}
	if skewed.DigestVersion == pdfops.ContentDigestVersion {
		t.Fatal("Sign overwrote the skewed DigestVersion — this test cannot construct its own " +
			"stimulus and would pass for the wrong reason")
	}
	doc2, err := Embed(prepared, skewed)
	if err != nil {
		t.Fatal(err)
	}
	_, err = CheckDocument(doc2, now)
	if !errors.Is(err, ErrDigestVersion) {
		t.Fatalf("a digest-rule skew reported %v — want ErrDigestVersion. A hash mismatch says "+
			"somebody changed the document; this says two builds measure it differently and "+
			"nobody has done anything wrong.", err)
	}
	// The sentence, not just the sentinel: it must name BOTH numbers, or the reader cannot
	// tell which side is behind.
	msg := err.Error()
	for _, want := range []string{
		fmt.Sprintf("rule %d", skewed.DigestVersion),
		fmt.Sprintf("rule %d", pdfops.ContentDigestVersion),
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("the skew sentence does not contain %q: %s", want, msg)
		}
	}
	if strings.Contains(msg, "not the same document") {
		t.Errorf("the skew produced the TAMPERING sentence: %s", msg)
	}
	// This record is from a NEWER rule than this build computes, and the remedy is this machine's.
	if !strings.Contains(msg, "started with a newer Nib") || !strings.Contains(msg, "update Nib") {
		t.Errorf("a record under a rule this build does not compute YET must say so and name the update: %s", msg)
	}
	if !errors.Is(err, p2p.ErrCeremonyRuleTooNew) || errors.Is(err, p2p.ErrCeremonyRuleRetired) {
		t.Errorf("the skew is not the too-new wire refusal and only that one: %v", err)
	}
}

// TestADigestRuleSkewNamesWhichBuildIsBehind — ADR-143, /pending 691. The skew sentence ended "update Nib"
// in BOTH directions, and for a record under a RETIRED rule that is advice with nothing behind it: the machine
// reading it is the up-to-date one. Every bump retires the oldest readable rule (ADR-080), so this is the
// sentence a ceremony begun two rules back meets — rule 4, the rule the release before last wrote, on this
// build. It must say the ceremony is the older side and what can be done about that, at every comparing door,
// and it must be the wire refusal for that direction so the arrival gate's answer crosses by name.
func TestADigestRuleSkewNamesWhichBuildIsBehind(t *testing.T) {
	cert, key, cfp := identity(t, "Convener")
	_, _, afp := identity(t, "A")
	base, err := testpdf.Text("the lease")
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := p2p.PrepareDocument(base)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for _, c := range []struct {
		name          string
		rule          int
		retired       bool
		says, remedy  string
		mustNotAdvise string
	}{
		{"the rule this bump retired", pdfops.ContentDigestVersion - 2, true,
			"started with an older Nib", "convene it again", "update Nib"},
		{"a rule long gone", 3, true, "started with an older Nib", "convene it again", "update Nib"},
		{"a rule this build does not know yet", pdfops.ContentDigestVersion + 1, false,
			"started with a newer Nib", "update Nib", "convene it again"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if pdfops.ContentDigestRuleReadable(c.rule) {
				t.Fatalf("setup: this build computes rule %d, so a record under it is not a skew", c.rule)
			}
			r := draft(t, cfp, afp)
			r.DocHash = strings.Repeat("0", 64) // another rule's number: never compared
			r.DigestVersion = c.rule
			if err := r.Sign(cert, key); err != nil {
				t.Fatal(err)
			}
			if r.DigestVersion != c.rule {
				t.Fatal("setup: Sign overwrote the skewed DigestVersion")
			}
			doc, err := Embed(prepared, r)
			if err != nil {
				t.Fatal(err)
			}
			root := t.TempDir()
			if _, err := WriteMirror(root, r, doc); err != nil {
				t.Fatal(err)
			}
			_, checkErr := CheckDocument(doc, now)
			_, _, mirrorErr := ReadMirror(root, r.ID, now)
			_, hashErr := DocumentHashFor(doc, r)
			for door, err := range map[string]error{"CheckDocument": checkErr, "ReadMirror": mirrorErr, "DocumentHashFor": hashErr} {
				if !errors.Is(err, ErrDigestVersion) {
					t.Errorf("%s: %v — want ErrDigestVersion", door, err)
					continue
				}
				msg := err.Error()
				if !strings.Contains(msg, c.says) || !strings.Contains(msg, c.remedy) {
					t.Errorf("%s: the sentence does not say %q with the remedy %q: %s", door, c.says, c.remedy, msg)
				}
				if strings.Contains(msg, c.mustNotAdvise) {
					t.Errorf("%s: the sentence gives the OTHER direction's remedy (%q): %s", door, c.mustNotAdvise, msg)
				}
				if !strings.Contains(msg, fmt.Sprintf("rule %d", c.rule)) ||
					!strings.Contains(msg, fmt.Sprintf("rule %d", pdfops.ContentDigestVersion)) {
					t.Errorf("%s: the sentence does not name both rules: %s", door, msg)
				}
				if strings.Contains(msg, "not the same document") || errors.Is(err, ErrMirrorDamaged) {
					t.Errorf("%s: a version difference is reported as tampering or damage: %s", door, msg)
				}
				if got := errors.Is(err, p2p.ErrCeremonyRuleRetired); got != c.retired {
					t.Errorf("%s: is the retired-rule wire refusal: %v, want %v", door, got, c.retired)
				}
				if got := errors.Is(err, p2p.ErrCeremonyRuleTooNew); got == c.retired {
					t.Errorf("%s: is the too-new wire refusal: %v, want %v", door, got, !c.retired)
				}
				if !p2p.IsContributionRefusal(err) {
					t.Errorf("%s: the skew has no wire code, so an arrival gate's refusal of it reaches the "+
						"dialler as a bare EOF", door)
				}
			}
		})
	}
}

// TestAStoredMirrorUnderAnotherDigestRuleIsASkewNotDamage — /pending 725 bumped
// ContentDigestVersion, and ReadMirror compared an unsigned stored document's DocHash without
// asking which rule computed it, so every mirror written before the bump read "damaged or
// incomplete". It must say what CheckRecord says: a version skew, naming both rules.
func TestAStoredMirrorUnderAnotherDigestRuleIsASkewNotDamage(t *testing.T) {
	cert, key, cfp := identity(t, "Convener")
	_, _, afp := identity(t, "A")
	doc, err := testpdf.Text("the lease")
	if err != nil {
		t.Fatal(err)
	}
	r := draft(t, cfp, afp)
	r.DocHash = "0000000000000000000000000000000000000000000000000000000000000000" // a rule-3 number: not this build's
	// Rule 3, which no build since v4 computes. The legacy rule would not do: since ADR-080 it is still COMPUTED,
	// so a record under it is checked under it (TestARecordWrittenUnderTheLegacyRuleIsCheckedByIt), not skewed.
	r.DigestVersion = 3
	if err := r.Sign(cert, key); err != nil {
		t.Fatal(err)
	}
	if pdfops.ContentDigestRuleReadable(r.DigestVersion) {
		t.Fatal("setup: rule 3 is readable by this build, so this record is not a skew and proves nothing")
	}
	root := t.TempDir()
	if _, err := WriteMirror(root, r, doc); err != nil {
		t.Fatal(err)
	}
	_, _, err = ReadMirror(root, r.ID, time.Now())
	if errors.Is(err, ErrMirrorDamaged) {
		t.Fatalf("a mirror written under digest rule %d reads as damaged: %v — want ErrDigestVersion",
			r.DigestVersion, err)
	}
	if !errors.Is(err, ErrDigestVersion) {
		t.Fatalf("a mirror written under another digest rule reported %v — want ErrDigestVersion", err)
	}
}

// TestExtractReadsTheEntryTheDigestExcludes — /pending 745 (a). Extract read the record by name,
// and the name lookup falls back to the first filespec CALLING itself nib-ceremony.json; the
// digest's rule is nib's exact shape, once. Extract must answer with the digest's rule: a lookalike
// is no record, and two records are refused by name.
func TestExtractReadsTheEntryTheDigestExcludes(t *testing.T) {
	r := AttachmentName
	lookalike := testpdf.WithEmbedded(testpdf.Embedded{Key: "Schedule-A.txt", F: r, UF: r, Data: "not a record"})
	if _, err := Extract(lookalike); !errors.Is(err, ErrNoRecord) {
		t.Errorf("an entry that only calls itself %s was read as the record: %v — want ErrNoRecord", r, err)
	}
	two := testpdf.WithEmbedded(
		testpdf.Embedded{Key: r, F: r, UF: r, Data: "one"},
		testpdf.Embedded{Key: r, F: r, UF: r, Data: "two"},
	)
	if _, err := Extract(two); !errors.Is(err, ErrTwoRecords) {
		t.Errorf("a document carrying the record's key twice: %v — want ErrTwoRecords", err)
	}
}

// TestARecordWrittenUnderTheLegacyRuleIsCheckedByIt — ADR-080. A record signed under the legacy content-digest rule
// (rule 5 since ADR-143: every record the previous release wrote) is compared with the number THAT rule computes
// for the document, at every comparing door: not refused as a skew, which halted the ceremony across every bump
// before ADR-080, and not compared with the current rule's number, which is a tampering sentence. A real change to
// the document is still a mismatch under the legacy rule.
func TestARecordWrittenUnderTheLegacyRuleIsCheckedByIt(t *testing.T) {
	cert, key, cfp := identity(t, "Convener")
	_, _, afp := identity(t, "A")
	base, err := testpdf.Text("the lease")
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := p2p.PrepareDocument(base)
	if err != nil {
		t.Fatal(err)
	}
	// The rule before the current one, by its number: the one earlier rule ADR-080 keeps readable.
	const legacy = pdfops.ContentDigestVersion - 1
	if !pdfops.ContentDigestRuleReadable(legacy) {
		t.Fatalf("setup: this build does not compute rule %d, so no record under it can be checked by it", legacy)
	}
	hLegacy, err := pdfops.ContentDigestAt(prepared, legacy)
	if err != nil {
		t.Fatal(err)
	}
	hCurrent, err := DocumentHash(prepared)
	if err != nil {
		t.Fatal(err)
	}
	if hLegacy == hCurrent {
		t.Fatal("setup: the two rules agree on this document, so a comparison under the wrong one would pass too")
	}
	r := draft(t, cfp, afp)
	r.DocHash = hLegacy
	r.DigestVersion = legacy
	if err := r.Sign(cert, key); err != nil {
		t.Fatal(err)
	}
	doc, err := Embed(prepared, r)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if _, err := CheckDocument(doc, now); err != nil {
		t.Fatalf("CheckDocument on a legacy-rule record over its own document: %v — want nil", err)
	}
	root := t.TempDir()
	if _, err := WriteMirror(root, r, doc); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadMirror(root, r.ID, now); err != nil {
		t.Fatalf("ReadMirror on a legacy-rule record over its own document: %v — want nil", err)
	}

	// The control: the same record over a different document is still the tampering sentence, under the legacy rule.
	other, err := testpdf.Text("the other lease")
	if err != nil {
		t.Fatal(err)
	}
	otherPrepared, err := p2p.PrepareDocument(other)
	if err != nil {
		t.Fatal(err)
	}
	swapped, err := Embed(otherPrepared, r)
	if err != nil {
		t.Fatal(err)
	}
	_, err = CheckDocument(swapped, now)
	if err == nil || errors.Is(err, ErrDigestVersion) || !strings.Contains(err.Error(), "not the same document") {
		t.Fatalf("a legacy-rule record over a different document reported %v — want the mismatch sentence", err)
	}
}

// TestOnlyConveneHashesUnderTheCurrentRule — ADR-080's door, held by its callers (ADR-009). DocumentHash is the
// current rule and only a record being WRITTEN may use it; every comparison goes through DocumentHashFor, under the
// rule the record names. A comparing site that called DocumentHash would check every rule-4 record under rule 5 —
// the tampering sentence for a version difference — and pass every test that builds its record with this build.
func TestOnlyConveneHashesUnderTheCurrentRule(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	call := regexp.MustCompile(`\bDocumentHash\(`)
	var sites []string
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		if d.IsDir() {
			if p != root && strings.HasPrefix(d.Name(), ".") || d.Name() == "testdata" || d.Name() == "node_modules" || d.Name() == "third_party" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		b, rerr := os.ReadFile(p)
		if rerr != nil {
			return rerr
		}
		for i, line := range strings.Split(string(b), "\n") {
			code, _, _ := strings.Cut(line, "//")
			if call.MatchString(code) && !strings.HasPrefix(strings.TrimSpace(code), "func DocumentHash(") {
				rel, _ := filepath.Rel(root, p)
				sites = append(sites, fmt.Sprintf("%s:%d", filepath.ToSlash(rel), i+1))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(sites) != 1 || !strings.HasPrefix(sites[0], "internal/ceremony/convene.go:") {
		t.Errorf("DocumentHash (the current rule) is called at %v; want exactly one call, in convene.go — a "+
			"comparing site must call DocumentHashFor", sites)
	}
}
