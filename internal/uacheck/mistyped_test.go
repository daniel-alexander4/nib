package uacheck

import "testing"

// mistypedDocs is the `/pending 612` and `/pending 633` population: one tagged page whose catalog or whose one
// structure element carries a `/Lang`, `/Alt`, `/ActualText` or `/E` of each type — the five pdfcpu's validator
// refuses the document over, the two string forms and `null` — plus the controls beside them.
func mistypedDocs() (names []string, docs [][]byte) {
	content := "/P << /MCID 0 >> BDC BT /F1 12 Tf 72 700 Td (abc) Tj ET EMC"
	add := func(name string, pdf []byte) { names, docs = append(names, name), append(docs, pdf) }
	elem := func(kind, extra string) map[int]string {
		return map[int]string{8: "<< /Type /StructElem /S /" + kind + " /P 7 0 R /Pg 3 0 R /K 0 " + extra + " >>"}
	}
	values := []string{"/a", "/", "5", "true", "[(a)]", "<< /X 1 >>", "(a)", "()", "null"}
	add("clean", langClauseDoc("", "", content, "", false))
	for _, v := range []string{"/en", "5", "(en)"} {
		add("catalog /Lang "+v, langClauseDoc("/Lang "+v, "", content, "", false))
	}
	for _, key := range mistypedTextKeys {
		for _, v := range values {
			add("P /"+key+" "+v, langClauseDoc("", "/"+key+" "+v, content, "", false))
		}
	}
	// A Figure, where the alternate text is what 7.3 t1 asks for, under a catalog language so that nothing else moves.
	for _, key := range []string{"ActualText", "Alt"} {
		for _, v := range values {
			add("Figure /"+key+" "+v, langClauseDocWith(elem("Figure", "/"+key+" "+v), "/Lang (en)", "", content, "", false))
		}
	}
	add("Figure with neither", langClauseDocWith(elem("Figure", ""), "/Lang (en)", "", content, "", false))
	for _, v := range []string{"/Note0", "5", "(n0)"} {
		add("Note /ID "+v, langClauseDocWith(elem("Note", "/ID "+v), "", "", content, "", false))
	}
	return names, docs
}

// TestAMistypedTextEntryIsReadAsVeraPDFReadsIt — `/pending 612` and `/pending 633`.
//
// pdfcpu's validator refuses a whole document over a `/Lang`, `/Alt`, `/ActualText` or `/E` that is not a string, so
// nib said "the document could not be read" about files veraPDF reads and grades. The checker now sets those entries
// aside for the validation and reads them itself.
//
// **Two halves.** With veraPDF present, every document is compared with it on EVERY clause nib checks, strictly —
// passed, failed and no-subject are three answers. Measured 2026-10-09 on 1.30.2: no difference on any of the 62.
// Without it, the rows below are what was measured, and they are chosen to tell the readings apart: a name on
// `/ActualText` is text (so its language is asked for, and a Figure has its alternate), every other wrong type
// anywhere is an absent key.
func TestAMistypedTextEntryIsReadAsVeraPDFReadsIt(t *testing.T) {
	names, docs := mistypedDocs()
	byName := map[string][]byte{}
	for i, pdf := range docs {
		byName[names[i]] = pdf
		if _, err := open(pdf); err != nil {
			t.Errorf("%s: nib cannot open a document veraPDF reads: %v", names[i], err)
		}
	}
	for _, row := range []struct {
		doc, clause string
		want        Verdict
	}{
		{"P /ActualText /a", "7.2 t21", Fail}, // a name IS text, and nothing gives it a language
		{"P /ActualText (a)", "7.2 t21", Fail},
		{"P /ActualText 5", "7.2 t21", Pass}, // not text: the key reads as absent
		{"P /ActualText [(a)]", "7.2 t21", Pass},
		{"P /Alt /a", "7.2 t22", Pass}, // a name is NOT an /Alt
		{"P /Alt (a)", "7.2 t22", Fail},
		{"P /E /a", "7.2 t23", Pass},
		{"P /E (a)", "7.2 t23", Fail},
		{"Figure /ActualText /a", "7.3 t1", Pass},
		{"Figure /ActualText /", "7.3 t1", Pass}, // the empty name too, as the empty string does
		{"Figure /ActualText ()", "7.3 t1", Pass},
		{"Figure /ActualText 5", "7.3 t1", Fail},
		{"Figure /ActualText true", "7.3 t1", Fail},
		{"Figure /ActualText << /X 1 >>", "7.3 t1", Fail},
		{"Figure /ActualText null", "7.3 t1", Fail},
		{"Figure /Alt /a", "7.3 t1", Fail},
		{"Figure /Alt (a)", "7.3 t1", Pass},
		{"Figure with neither", "7.3 t1", Fail},
	} {
		pdf, ok := byName[row.doc]
		if !ok {
			t.Fatalf("setup: no document named %q", row.doc)
		}
		if got := verdictOf(t, pdf, row.clause); got.Verdict != row.want {
			t.Errorf("%s: %s reports %v (%s), want %v", row.doc, row.clause, got.Verdict, got.Why, row.want)
		}
	}
	// A wrongly-typed catalog or element /Lang declares no language: the document answers exactly as the clean one.
	for _, name := range []string{"catalog /Lang /en", "catalog /Lang 5", "P /Lang /a", "P /Lang 5"} {
		for _, clause := range []string{"7.2 t2", "7.2 t29", "7.2 t33", "7.2 t34"} {
			if got, base := verdictOf(t, byName[name], clause), verdictOf(t, byName["clean"], clause); got.Verdict != base.Verdict {
				t.Errorf("%s: %s reports %v where the same document with no /Lang reports %v", name, clause, got.Verdict, base.Verdict)
			}
		}
	}
	// Stimulus for that comparison: a real /Lang does move something, or the loop above compares nothing.
	if got, base := verdictOf(t, byName["catalog /Lang (en)"], "7.2 t34"), verdictOf(t, byName["clean"], "7.2 t34"); got.Verdict == base.Verdict {
		t.Errorf("setup: a string catalog /Lang leaves 7.2 t34 at %v, so the clean document is no control", got.Verdict)
	}

	vera := veraAsk(t, docs)
	if vera == nil {
		return
	}
	checked := map[string]bool{}
	for _, c := range Clauses() {
		checked[c] = true
	}
	words := map[Verdict]string{Pass: "passed", Fail: "failed", NotApplicable: "none"}
	for i, pdf := range docs {
		for clause, said := range vera[i] {
			if !checked[clause] {
				continue
			}
			if got := verdictOf(t, pdf, clause); words[got.Verdict] != said {
				t.Errorf("%s: %s — nib reports %v (%s), veraPDF %q", names[i], clause, got.Verdict, got.Why, said)
			}
		}
	}
}
