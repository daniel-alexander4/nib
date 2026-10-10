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
// **Every one of these was unreadable or half-read until the checker carried the stream across pdfcpu's validator**
// (`setAsideForValidator`, `/pending 857`): the validator dropped the CIDFontType0 font from the page's resources and
// refused the Type 1 document outright.
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
	cid0, t1c := cidCFFFixtures()[0].pdf, type1CFixtures()[0].pdf
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
	cases := []struct {
		name    string
		pdf     []byte
		nothing bool
	}{
		{"CIDFontType0: a number", swap(cid0, c0, "/Subtype 5             "), true},
		{"CIDFontType0: a string", swap(cid0, c0, "/Subtype (CIDFontType0)"), true},
		{"CIDFontType0: a boolean", swap(cid0, c0, "/Subtype true          "), true},
		{"CIDFontType0: an array", swap(cid0, c0, "/Subtype [/A]          "), true},
		{"CIDFontType2: a number", swap(cid2, c2, "/Subtype 5        "), true},
		{"CIDFontType2: a string", swap(cid2, c2, "/Subtype (OpenTyp)"), true},
		{"CIDFontType0: the name it should be", cid0, false},
		{"CIDFontType0: null", swap(cid0, c0, "/Subtype null          "), false},
		{"CIDFontType0: no /Subtype at all", swap(cid0, c0, "/Subtypx /CIDFontType0C"), false},
		{"CIDFontType2: the name it should be", cid2, false},
		{"a TrueType font: a string", tt("(OpenType)"), false},
		{"a TrueType font: a number", tt("5"), false},
		// A Type 1 font so written does not throw: veraPDF reports, with no program to embed.
		{"a Type 1 font: a number", swap(t1c, "/Subtype /Type1C", "/Subtype 5      "), false},
		{"a Type 1 font: a string", swap(t1c, "/Subtype /Type1C", "/Subtype (Type1)"), false},
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
			if c.nothing && !said {
				t.Errorf("%s: %s answers %v (%s) about a document veraPDF gives no report for", c.name, clause, got.Verdict, got.Why)
			}
		}
		if !c.nothing && refused > 0 {
			t.Errorf("%s: %d clauses say veraPDF reports nothing, and it reports", c.name, refused)
		}
		if vera != nil && (vera[i] == nil) != c.nothing {
			t.Errorf("%s: veraPDF gave a report=%v, measured the other way — the row is stale", c.name, vera[i] != nil)
		}
	}
	// Where veraPDF does report, nib answers as it does on every clause — the program being one neither opens.
	if vera != nil {
		words := map[Verdict]string{Pass: "passed", Fail: "failed", NotApplicable: "none"}
		for i, c := range cases {
			if c.nothing || vera[i] == nil {
				continue
			}
			for _, clause := range Clauses() {
				if got := verdictOf(t, c.pdf, clause); words[got.Verdict] != vera[i][clause] {
					t.Errorf("%s: %s — nib reports %v (%s), veraPDF %q", c.name, clause, got.Verdict, got.Why, vera[i][clause])
				}
			}
		}
	}
	for _, name := range []string{"a Type 1 font: a number", "a Type 1 font: a string"} {
		for _, c := range cases {
			if c.name == name {
				if got := verdictOf(t, c.pdf, "7.21.4.1 t1"); got.Verdict != Fail {
					t.Errorf("%s: 7.21.4.1 t1 reports %v (%s), want Fail — veraPDF opens no program there", name, got.Verdict, got.Why)
				}
			}
		}
	}
}
