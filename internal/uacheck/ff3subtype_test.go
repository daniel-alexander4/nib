package uacheck

import (
	"bytes"
	"strings"
	"testing"
)

// TestAFontFile3SubtypeThatIsNotANameReportsNothing — `/pending 683`.
//
// veraPDF casts a CIDFont's `/FontFile3` `/Subtype` to a name outside the `try` around the program's parse, so a
// value that is not a name throws and the job ends with NO report. nib read such a stream as "no program" and
// answered: on a CIDFontType2, 7.21.4.1 t1 failed and every other clause answered too. Each row was
// run on veraPDF 1.30.2 and is asked again here whenever it is present.
//
// The controls are the other half: `null` and an absent key do NOT throw (veraPDF reports, with the font unembedded),
// and a TrueType font's `/FontFile3` with a string `/Subtype` reports too — so a rule that refused every odd
// `/Subtype` would be caught by them.
func TestAFontFile3SubtypeThatIsNotANameReportsNothing(t *testing.T) {
	swap := func(pdf []byte, from, to string) []byte {
		if len(from) != len(to) || !bytes.Contains(pdf, []byte(from)) {
			t.Fatalf("setup: cannot rewrite %q as %q in the fixture", from, to)
		}
		return bytes.Replace(pdf, []byte(from), []byte(to), 1)
	}
	cid0 := cidCFFFixtures()[0].pdf
	var cid2 []byte
	for _, f := range fontDoorFixtures() {
		if f.name == "CIDSet: FontFile3 OpenType, every slot" {
			cid2 = f.pdf
		}
	}
	if cid2 == nil {
		t.Fatal("setup: the CIDFontType2 /FontFile3 fixture is gone from fontDoorFixtures")
	}
	tt := func(sub string) []byte {
		return glyphDoc(ttFontDict("/Encoding /WinAnsiEncoding"), "(ABC) Tj", ttObjects("/Flags 32", "FontFile3", ttProgram(sub31, sub10), "/Subtype "+sub))
	}
	const c0, c2 = "/Subtype /CIDFontType0C", "/Subtype /OpenType"
	// `dropped` is the CIDFontType0 shape: pdfcpu's validator REMOVES that font from the page's resources, so nib
	// never sees the stream and cannot know veraPDF throws. Its font clauses refuse (the font does not resolve) and the
	// rest answer — a reading limit, held here so it is not mistaken for agreement (`/pending 857` is this class).
	cases := []struct {
		name             string
		pdf              []byte
		nothing, dropped bool
	}{
		{"CIDFontType0: a number", swap(cid0, c0, "/Subtype 5             "), true, true},
		{"CIDFontType0: a string", swap(cid0, c0, "/Subtype (CIDFontType0)"), true, true},
		{"CIDFontType0: a boolean", swap(cid0, c0, "/Subtype true          "), true, true},
		{"CIDFontType0: an array", swap(cid0, c0, "/Subtype [/A]          "), true, true},
		{"CIDFontType2: a number", swap(cid2, c2, "/Subtype 5        "), true, false},
		{"CIDFontType2: a string", swap(cid2, c2, "/Subtype (OpenTyp)"), true, false},
		{"CIDFontType0: the name it should be", cid0, false, false},
		{"CIDFontType0: null", swap(cid0, c0, "/Subtype null          "), false, false},
		{"CIDFontType0: no /Subtype at all", swap(cid0, c0, "/Subtypx /CIDFontType0C"), false, false},
		{"CIDFontType2: the name it should be", cid2, false, false},
		{"a TrueType font: a string", tt("(OpenType)"), false, false},
		{"a TrueType font: a number", tt("5"), false, false},
	}
	var docs [][]byte
	for _, c := range cases {
		docs = append(docs, c.pdf)
	}
	vera := veraAsk(t, docs)
	for i, c := range cases {
		refused := 0
		for _, clause := range Clauses() {
			got := verdictOf(t, c.pdf, clause)
			said := got.Verdict == CannotCheck && strings.Contains(got.Why, "reports nothing")
			if said {
				refused++
			}
			if c.nothing && !c.dropped && !said {
				t.Errorf("%s: %s answers %v (%s) about a document veraPDF gives no report for", c.name, clause, got.Verdict, got.Why)
			}
		}
		if c.dropped {
			if got := verdictOf(t, c.pdf, "7.21.4.1 t1"); got.Verdict != CannotCheck || !strings.Contains(got.Why, "does not resolve") {
				t.Errorf("%s: 7.21.4.1 t1 reports %v (%s), want CannotCheck over a font pdfcpu dropped — if the font now "+
					"survives the read, this row belongs with the ones that say veraPDF reports nothing", c.name, got.Verdict, got.Why)
			}
			if refused > 0 {
				t.Errorf("%s: %d clauses already say veraPDF reports nothing — move the row", c.name, refused)
			}
		}
		if !c.nothing && refused > 0 {
			t.Errorf("%s: %d clauses say veraPDF reports nothing, and it reports", c.name, refused)
		}
		if vera != nil && (vera[i] == nil) != c.nothing {
			t.Errorf("%s: veraPDF gave a report=%v, measured the other way — the row is stale", c.name, vera[i] != nil)
		}
	}
	// The Type 1 shape is a reading limit, recorded so a pdfcpu bump that starts accepting it turns this red instead
	// of leaving it unmeasured: veraPDF reports on it (7.21.4.1 t1 fails), and nib cannot open it.
	t1 := swap(type1CFixtures()[0].pdf, "/Subtype /Type1C", "/Subtype 5      ")
	if _, err := open(t1); err == nil || !strings.Contains(err.Error(), "validateFontFile3SubType") {
		t.Errorf("a Type 1 font whose /FontFile3 /Subtype is a number: open says %v, want pdfcpu's validateFontFile3SubType refusal — "+
			"if it now opens, veraPDF reports on this document and the rules need a row for it", err)
	}
}
