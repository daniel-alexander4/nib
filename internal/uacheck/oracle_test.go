package uacheck

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"hash/crc32"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"nib/internal/pdfops"
	"nib/internal/testpdf"
)

// Law 5 — the oracle validates the checker. `PLAN-accessibility.md` P07.S05.
//
// # Why this is the test the whole phase rests on
//
// Law 5: *"nib's pure-Go checker is itself checked against veraPDF over a golden corpus. A checker
// nothing checks is the fatal-bug category of this whole plan."* Every rule in this package encodes
// someone's reading of a clause, and a reading tested only against its author's fixtures agrees with
// its author. This test asks veraPDF, per clause, per document, and requires nib to say the same.
//
// # Three states, not two — which veraPDF only reveals when asked
//
// veraPDF's default report lists FAILED rules only, so a clause absent from it could have passed or
// had nothing to check, and nib's `Pass` and `NotApplicable` could not be told apart against it —
// P07.S02 had to score both as agreement. With `--passed` every rule is listed with its check counts,
// and a rule at `passedChecks="0" failedChecks="0"` had no subject. So agreement here is strict:
//
//	veraPDF failed              ↔  nib Fail
//	veraPDF passed, checks > 0  ↔  nib Pass
//	veraPDF passed, 0 / 0       ↔  nib NotApplicable
//
// `CannotCheck` is permitted against any of them — law 4's third verdict is honest — and is COUNTED
// against `knownCannotCheck`, so the count cannot quietly grow.
//
// # What its runs found — three defects in nib, none in veraPDF
//
//   - 7.21.4.2 t2's subject is the embedded CID font, not the /CIDSet: veraPDF passes a font with no
//     set, and nib called it not applicable (4 documents, first run of the draft).
//   - 7.18.4 t1 is read from the annotation up: veraPDF passes a widget whose Form element has lost
//     its OBJR back-link, and nib failed it (first run of the guard).
//   - 7.2 t33's subject is language-alternative text, and an `xml:lang` on the alternative
//     satisfies it without a catalog /Lang; nib failed a packet veraPDF found no subject in.
//
// Before any of those, P07.S04's live run caught nib passing an over-claiming /CIDSet. Every one was
// nib being stricter or looser than the clause in a way its author's own fixtures could not show.

// oracleDoc is one corpus document and where it came from.
type oracleDoc struct {
	name string
	pdf  []byte
}

// oracleCorpus is every document the guard asks about.
//
// **Product doors first**, because a document only a test can produce is how P06's tagged Markdown
// was credited while no user could make one (/pending 481). Then the mutations that reach a clause's
// FAILED state where no product door does, each named for what it breaks. Then a real third-party
// document from LibreOffice, whose default conversion is tagged — the only structure here nib did
// not write.
func oracleCorpus(t *testing.T) []oracleDoc {
	t.Helper()
	must := func(name string, b []byte, err error) oracleDoc {
		if err != nil {
			t.Fatalf("corpus: %s: %v", name, err)
		}
		return oracleDoc{name, b}
	}
	plain, err := testpdf.Text("host")
	if err != nil {
		t.Fatal(err)
	}
	textField := []pdfops.FormField{{Page: 1, Rect: [4]float64{100, 700, 300, 720}, Kind: "text", Name: "n", Label: "Name"}}

	wrapped, _, err := pdfops.TagAuthored(plain)
	w := must("wrapped", wrapped, err)
	tf, err := pdfops.AuthorForm(plain, textField)
	cf, cerr := pdfops.AuthorForm(plain, []pdfops.FormField{{Page: 1, Rect: [4]float64{100, 660, 112, 672}, Kind: "check", Name: "a", Label: "I agree"}})
	df, _, derr := pdfops.AuthorTaggedForm(plain, textField)
	md, merr := pdfops.ConvertDocToPDF([]byte("# Heading\n\nA paragraph.\n\n- one\n- two\n"), ".md")
	mdt, terr := pdfops.SetTitle(md, "A named document")
	mdl, lerr := pdfops.SetLang(mdt, "en")
	st, serr := pdfops.StampWatermark(plain, "DRAFT", pdfops.WatermarkStyle{})
	ocr, _, oerr := pdfops.TagOCRLayer(plain, []pdfops.Word{{Page: 1, Rect: [4]float64{72, 700, 140, 712}, Text: "Invoice", Block: 1, Para: 1, Line: 1}}, "eng")

	docs := []oracleDoc{
		{"plain page", plain},
		w,
		must("text-only form", tf, err),
		must("checkbox form", cf, cerr),
		must("described form", df, derr),
		must("converted Markdown", md, merr),
		must("Markdown + title", mdt, terr),
		must("Markdown + title + lang", mdl, lerr),
		must("stamped page", st, serr),
		must("tagged OCR scan", ocr, oerr),
	}
	docs = append(docs,
		oracleDoc{"Markdown + exact CIDSet", withCIDSet(t, md, cidExact)},
		oracleDoc{"Markdown + padded CIDSet", withCIDSet(t, md, cidPadded)},
		oracleDoc{"wrapped + element /Lang", langOnEveryElement(t, w.pdf, "en")},
		oracleDoc{"stamped + /AS restored", withASOnDefault(t, st)},
		oracleDoc{"stamped − /Name", withoutNameOnDefault(t, st)},
		oracleDoc{"titled + DisplayDocTitle false", withDisplayDocTitle(t, mdt, false)},
		oracleDoc{"wrapped + Marked false", markedFalse(t, w.pdf)},
		oracleDoc{"described form − OBJR", widgetMutation(t, df, "drop-objr")},
		oracleDoc{"packet without dc:title", withoutDCTitle(t, mdt)},
		// The shapes law 5's first run measured, pinned so the rules cannot drift back:
		oracleDoc{"packet: dc:title with its own xml:lang", withPacketBody(t, mdt,
			`<dc:title><rdf:Alt><rdf:li xml:lang="en">A title</rdf:li></rdf:Alt></dc:title>`)},
		oracleDoc{"packet: dc:creator Seq only", withPacketBody(t, mdt,
			`<dc:creator><rdf:Seq><rdf:li>Someone</rdf:li></rdf:Seq></dc:creator>`)},
		oracleDoc{"described form − /StructParent", widgetMutation(t, df, "drop-structparent")},
	)
	if pdfops.LibreOfficeAvailable() {
		lo, err := pdfops.ConvertOfficeToPDF(oracleODT(t), "odt")
		if err != nil {
			t.Fatalf("corpus: LibreOffice is present and could not convert the fixture: %v", err)
		}
		docs = append(docs, oracleDoc{"LibreOffice ODT", lo})
	} else {
		t.Log("NOTE (a narrower corpus, not a pass): LibreOffice is absent, so the one document " +
			"whose structure nib did not write is missing from this run")
	}
	return docs
}

// oracleODT is a minimal ODT with a heading, a paragraph and a list. `mimetype` is stored first and
// uncompressed through CreateRaw, as ODF requires — Go's streaming writer breaks the fixed offset,
// which is how an earlier fixture in this repo was rejected by LibreOffice.
func oracleODT(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	mt := []byte("application/vnd.oasis.opendocument.text")
	w, err := zw.CreateRaw(&zip.FileHeader{Name: "mimetype", Method: zip.Store, CRC32: crc32.ChecksumIEEE(mt),
		CompressedSize64: uint64(len(mt)), UncompressedSize64: uint64(len(mt))})
	if err != nil {
		t.Fatal(err)
	}
	w.Write(mt)
	for name, body := range map[string]string{
		"META-INF/manifest.xml": `<?xml version="1.0" encoding="UTF-8"?><manifest:manifest xmlns:manifest="urn:oasis:names:tc:opendocument:xmlns:manifest:1.0" manifest:version="1.2"><manifest:file-entry manifest:full-path="/" manifest:media-type="application/vnd.oasis.opendocument.text"/><manifest:file-entry manifest:full-path="content.xml" manifest:media-type="text/xml"/></manifest:manifest>`,
		"content.xml":           `<?xml version="1.0" encoding="UTF-8"?><office:document-content xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0" xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0" office:version="1.2"><office:body><office:text><text:h text:outline-level="1">A heading</text:h><text:p>A paragraph of body text.</text:p><text:list><text:list-item><text:p>one</text:p></text:list-item><text:list-item><text:p>two</text:p></text:list-item></text:list></office:text></office:body></office:document-content>`,
	} {
		f, ferr := zw.Create(name)
		if ferr != nil {
			t.Fatal(ferr)
		}
		f.Write([]byte(body))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// veraPDFPath locates veraPDF the way `internal/pdfops` does: $NIB_VERAPDF, then PATH, then ~/verapdf.
func veraPDFPath() string {
	if p := os.Getenv("NIB_VERAPDF"); p != "" {
		return p
	}
	if p, err := exec.LookPath("verapdf"); err == nil {
		return p
	}
	if home, _ := os.UserHomeDir(); home != "" {
		cand := filepath.Join(home, "verapdf", "verapdf")
		if _, err := os.Stat(cand); err == nil {
			return cand
		}
	}
	return ""
}

// veraState is what veraPDF concluded about one rule on one document.
type veraState string

const (
	veraFailed    veraState = "failed"
	veraPassed    veraState = "passed"
	veraNoSubject veraState = "no subject"
)

// agrees is the strict three-state mapping, with CannotCheck permitted against any state.
func (v veraState) agrees(nib Verdict) bool {
	switch nib {
	case CannotCheck:
		return true
	case Fail:
		return v == veraFailed
	case Pass:
		return v == veraPassed
	case NotApplicable:
		return v == veraNoSubject
	}
	return false
}

type veraReport struct {
	Jobs []struct {
		Item struct {
			Name string `xml:"name"`
		} `xml:"item"`
		Report struct {
			Status string `xml:"jobEndStatus,attr"`
			Rules  []struct {
				Clause string `xml:"clause,attr"`
				Test   string `xml:"testNumber,attr"`
				Status string `xml:"status,attr"`
				Passed string `xml:"passedChecks,attr"`
				Failed string `xml:"failedChecks,attr"`
			} `xml:"details>rule"`
		} `xml:"validationReport"`
	} `xml:"jobs>job"`
}

// knownCannotCheck records every (document, clause) where nib answers CannotCheck, with the reason.
// **Empty, measured.** A row appearing is a gap in nib a person must look at; a row that stops
// appearing means the gap closed and the row is a claim about code that no longer behaves that way.
var knownCannotCheck = map[string]string{}

// notYetReachable records a veraPDF state a clause cannot reach on any corpus document yet, with the
// coordinate that will make it reachable. Checked in both directions, and against the plan's marker.
var notYetReachable = map[string]string{
	// P07.S07 closed WITHOUT writing the identification — law 1 forbids it on a 15-of-106 checker — so
	// no nib document reaches `5 t1 passed`. The gate is the strategy decision, not a plan coordinate.
	"5 t1 passed": "/pending 486",
}

// TestTheOracleValidatesTheChecker — law 5.
func TestTheOracleValidatesTheChecker(t *testing.T) {
	vp := veraPDFPath()
	if vp == "" {
		t.Skip("SKIP (not a pass): veraPDF is absent, so PLAN-accessibility.md law 5 — the checker " +
			"agrees with the oracle over the corpus — is UNCHECKED in this run. Set NIB_VERAPDF, put " +
			"verapdf on PATH, or install to ~/verapdf.")
	}
	docs := oracleCorpus(t)

	// Floor, EXACT over the generated documents: a probe that dropped one passed a `>= n-1` floor,
	// because another document happened to reach the same state. LibreOffice's document is counted
	// separately — its absence narrows the corpus loudly rather than failing it.
	generated := 0
	for _, d := range docs {
		if d.name != "LibreOffice ODT" {
			generated++
		}
	}
	const wantGenerated = 22
	if generated != wantGenerated {
		t.Fatalf("the corpus holds %d generated document(s), want exactly %d — change this number in the "+
			"same edit that adds or removes a document, so a shrunken corpus cannot pass as the whole one",
			generated, wantGenerated)
	}

	dir := t.TempDir()
	files := make([]string, len(docs))
	names := make([]string, len(docs))
	for i, d := range docs {
		files[i] = filepath.Join(dir, fmt.Sprintf("doc%02d.pdf", i))
		if err := os.WriteFile(files[i], d.pdf, 0o600); err != nil {
			t.Fatal(err)
		}
		names[i] = d.name
	}
	out, _ := exec.Command(vp, append([]string{"--flavour", "ua1", "--passed"}, files...)...).Output()
	var rep veraReport
	if err := xml.Unmarshal(out, &rep); err != nil {
		t.Fatalf("the veraPDF report did not parse: %v\n%.500s", err, out)
	}
	vera := veraStates(rep, files)
	reports := make([]*Report, len(docs))
	for i, d := range docs {
		nr, err := Check(d.pdf)
		if err != nil {
			t.Errorf("%s: nib could not read a document veraPDF validated: %v", d.name, err)
			continue
		}
		reports[i] = &nr
	}
	cmp := compareToOracle(names, vera, reports)
	for _, e := range cmp.errors {
		t.Error(e)
	}
	t.Logf("law 5: %d of %d (document, clause) pairs agree strictly over %d documents", cmp.agreed, cmp.total, len(docs))

	for k, why := range cmp.cannot {
		if _, known := knownCannotCheck[k]; !known {
			t.Errorf("CannotCheck not recorded: %s (%s). Record it in knownCannotCheck with the reason, or close the gap", k, why)
		}
	}
	for k := range knownCannotCheck {
		if _, still := cmp.cannot[k]; !still {
			t.Errorf("knownCannotCheck has %q, which nib now answers — remove the row", k)
		}
	}

	// Every clause must be exercised in BOTH directions somewhere in the corpus, or agreement on it
	// is agreement on half a rule.
	markers := planMarkers(t)
	for _, c := range Clauses() {
		for _, s := range []veraState{veraFailed, veraPassed} {
			key := c + " " + string(s)
			gate, declared := notYetReachable[key]
			switch {
			case cmp.reached[key] && declared:
				t.Errorf("%q is reachable now and notYetReachable still declares it — remove the row", key)
			case !cmp.reached[key] && !declared:
				t.Errorf("no corpus document makes veraPDF report %s for %s, so nib's rule is never "+
					"checked against that half of the clause — add a document that reaches it", s, c)
			case !cmp.reached[key] && declared && strings.HasPrefix(gate, "PLAN-") && markers[gate]:
				t.Errorf("%q is declared not yet reachable until %s, and the plan marks %s done — "+
					"the slice shipped without a corpus document that reaches it", key, gate, gate)
			}
		}
	}
}

// veraStates turns a batch report into one clause→state map per input file, in input order. A file
// veraPDF returned no job for stays nil — which compareToOracle reports rather than skips.
func veraStates(rep veraReport, files []string) []map[string]veraState {
	index := map[string]int{}
	for i, f := range files {
		index[filepath.Base(f)] = i
	}
	out := make([]map[string]veraState, len(files))
	for _, j := range rep.Jobs {
		i, ok := index[filepath.Base(j.Item.Name)]
		if !ok {
			continue
		}
		m := map[string]veraState{}
		for _, r := range j.Report.Rules {
			pc, _ := strconv.Atoi(r.Passed)
			fc, _ := strconv.Atoi(r.Failed)
			s := veraPassed
			switch {
			case r.Status == "failed" || fc > 0:
				s = veraFailed
			case pc == 0 && fc == 0:
				s = veraNoSubject
			}
			m[r.Clause+" t"+r.Test] = s
		}
		out[i] = m
	}
	return out
}

// oracleComparison is compareToOracle's result.
type oracleComparison struct {
	errors        []string
	agreed, total int
	reached       map[string]bool   // "<clause> <state>" veraPDF reported on some document
	cannot        map[string]string // "<document> / <clause>" → nib's reason
}

// compareToOracle holds the guard's whole judgment as a pure function, so every branch of it can be
// driven without veraPDF.
//
// **Why it was lifted out of the test.** Four mutations to the guard survived its first probe pass —
// scoring Pass and no-subject alike, skipping a lost job, skipping an unlisted clause — not because
// the checks were wrong but because the corpus never produced those situations, so the branches had
// no stimulus. A guard whose failure paths only a broken veraPDF run could reach is a guard nobody
// has seen fail. Here each one is driven by a synthetic input.
func compareToOracle(names []string, vera []map[string]veraState, nib []*Report) oracleComparison {
	c := oracleComparison{reached: map[string]bool{}, cannot: map[string]string{}}
	for i, name := range names {
		if vera[i] == nil {
			c.errors = append(c.errors, fmt.Sprintf("%s: veraPDF returned no job for it — a lost file reads exactly like a clean one", name))
			continue
		}
		if nib[i] == nil {
			continue
		}
		for _, r := range nib[i].Results {
			c.total++
			v, listed := vera[i][r.Clause]
			if !listed {
				c.errors = append(c.errors, fmt.Sprintf("%s: veraPDF's report does not list %s at all, so there is "+
					"nothing to agree with — the clause spelling may have drifted from veraPDF's", name, r.Clause))
				continue
			}
			c.reached[r.Clause+" "+string(v)] = true
			if r.Verdict == CannotCheck {
				c.cannot[name+" / "+r.Clause] = r.Why
			}
			if v.agrees(r.Verdict) {
				c.agreed++
				continue
			}
			c.errors = append(c.errors, fmt.Sprintf("%s: %s — veraPDF %s, nib %v (%s at %s)", name, r.Clause, v, r.Verdict, r.Why, r.Where))
		}
	}
	return c
}

// planMarkers returns the coordinates PLAN-accessibility.md marks done.
func planMarkers(t *testing.T) map[string]bool {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "PLAN-accessibility.md"))
	if err != nil {
		t.Fatalf("the plan could not be read, so a gated row cannot be checked against its gate: %v", err)
	}
	out := map[string]bool{}
	re := regexp.MustCompile(`(?m)^#### (P\d+\.S\d+)\b.*\*\(done `)
	for _, m := range re.FindAllStringSubmatch(string(b), -1) {
		out["PLAN-accessibility.md "+m[1]] = true
	}
	if len(out) == 0 {
		t.Fatal("no slice in the plan reads as done, so the marker scan has stopped matching and every gated row passes vacuously")
	}
	return out
}
