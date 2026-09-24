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

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

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

	w := oracleDoc{"committed proposal", committedProposal(t, plain)}
	tf, err := pdfops.AuthorForm(plain, textField)
	cf, cerr := pdfops.AuthorForm(plain, []pdfops.FormField{{Page: 1, Rect: [4]float64{100, 660, 112, 672}, Kind: "check", Name: "a", Label: "I agree"}})
	// Both tagging doors add to the committed proposal (`/pending 495`): over `plain`'s own untagged text they
	// now claim nothing. Not a picture for the OCR layer: its text is then invisible only, and 7.21.4.1 t1
	// reads NotApplicable where veraPDF passes — a checker question outside that item.
	df, _, derr := pdfops.AuthorTaggedForm(w.pdf, textField)
	md, merr := pdfops.ConvertDocToPDF([]byte("# Heading\n\nA paragraph.\n\n- one\n- two\n"), ".md")
	mdt, terr := pdfops.SetTitle(md, "A named document")
	mdl, lerr := pdfops.SetLang(mdt, "en")
	st, serr := pdfops.StampWatermark(plain, "DRAFT", pdfops.WatermarkStyle{})
	ocr, _, oerr := pdfops.TagOCRLayer(w.pdf, []pdfops.Word{{Page: 1, Rect: [4]float64{72, 700, 140, 712}, Text: "Invoice", Block: 1, Para: 1, Line: 1}}, "eng")

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
		must("OCR layer tagged into a committed proposal", ocr, oerr),
	}
	// P05.S01. `AddNotes` is the product door for the PASSING side of 7.18.1 t1 and t2: it writes a
	// `/Text` annotation with `/Contents` and, on a tagged document, nests it in an `/Annot` element.
	// The two mutations are the only way to the FAILING side — nothing nib ships writes an annotation
	// that is undescribed or outside an Annot tag.
	noted, nerr := pdfops.AddNotes(w.pdf, []pdfops.Note{{Page: 1, X: 72, Y: 700, Text: "a note"}})
	uf, uferr := pdfops.AuthorForm(plain, []pdfops.FormField{{Page: 1, Rect: [4]float64{100, 620, 300, 640},
		Kind: "text", Name: "unnamed", Label: ""}})
	// P06.S02: the product door for an embedded file.
	attached, aerr := pdfops.AddAttachment(w.pdf, "schedule.csv", []byte("a,b\n1,2\n"))

	docs = append(docs,
		must("committed proposal + a note", noted, nerr),
		oracleDoc{"noted proposal − the note's /StructParent", annotMutation(t, noted, "drop-structparent")},
		oracleDoc{"noted proposal − the note's /Contents", annotMutation(t, noted, "drop-contents")},
		// P05.S02. Nib writes no Link, TrapNet or PrinterMark annotation — measured, including the Markdown
		// conversion, which emits no `/Annots` at all — so every half of these four clauses is a mutation.
		// `addAnnotation` says why that is the honest route here rather than a product door.
		oracleDoc{"committed proposal + a described link in a Link element",
			addAnnotation(t, w.pdf, "Link", types.Dict{"Contents": types.StringLiteral("a link to the schedule")}, "Link")},
		oracleDoc{"committed proposal + an undescribed link in a P element",
			addAnnotation(t, w.pdf, "Link", nil, "P")},
		oracleDoc{"committed proposal + a visible TrapNet",
			addAnnotation(t, w.pdf, "TrapNet", types.Dict{"Contents": types.StringLiteral("a trapping network")}, "")},
		oracleDoc{"committed proposal + a hidden TrapNet",
			addAnnotation(t, w.pdf, "TrapNet", types.Dict{"F": types.Integer(2)}, "")},
		oracleDoc{"committed proposal + an untagged printer's mark",
			addAnnotation(t, w.pdf, "PrinterMark", types.Dict{"Contents": types.StringLiteral("a registration mark")}, "")},
		oracleDoc{"committed proposal + a printer's mark tagged /Artifact",
			addAnnotation(t, w.pdf, "PrinterMark", types.Dict{"Contents": types.StringLiteral("a registration mark")}, "Artifact")},
		// P05.S03. Both PASSING halves are product doors already above: `AuthorForm` writes `/TU` from the
		// field's label (`form.go:42`) and `setStructureTabOrder` writes `/Tabs /S` on every page carrying an
		// annotation. The failing halves are a form whose field the user named NOTHING — a product door too,
		// since an empty label deliberately writes no `/TU` — and a mutation that drops `/Tabs`.
		must("unlabelled form", uf, uferr),
		oracleDoc{"noted proposal − /Tabs", withoutTabs(t, noted)},
		// P05.S04. **Nib plays no media and writes no clip**, so unlike the phase's other clauses this one has no
		// product door for either half — all three documents are mutations, and between them they reach all four
		// halves: the first passes both clauses, the second fails t1, the third fails t2.
		oracleDoc{"committed proposal + a described media clip", addMediaClip(t, w.pdf, "audio/mpeg", "|a media clip")},
		oracleDoc{"committed proposal + a media clip with no /CT", addMediaClip(t, w.pdf, "", "|a media clip")},
		oracleDoc{"committed proposal + a media clip whose /Alt is odd", addMediaClip(t, w.pdf, "audio/mpeg", "|a media clip|en")},
		oracleDoc{"Markdown + exact CIDSet", withCIDSet(t, md, cidExact)},
		oracleDoc{"Markdown + padded CIDSet", withCIDSet(t, md, cidPadded)},
		oracleDoc{"committed proposal + element /Lang", langOnEveryElement(t, w.pdf, "en")},
		// The failed half of 7.2 t21, t22 and t23: alternate text on the one structure element of a document that
		// declares no language anywhere (P04.S02). The passing half comes from every other corpus document.
		oracleDoc{"committed proposal + unlanguaged alternate text", alternateTextWithNoLanguage(t, w.pdf, "Alt", "ActualText", "E")},
		oracleDoc{"stamped + /AS restored", withASOnDefault(t, st)},
		oracleDoc{"stamped − /Name", withoutNameOnDefault(t, st)},
		oracleDoc{"titled + DisplayDocTitle false", withDisplayDocTitle(t, mdt, false)},
		oracleDoc{"committed proposal + Marked false", markedFalse(t, w.pdf)},
		oracleDoc{"described form − OBJR", widgetMutation(t, df, "drop-objr")},
		oracleDoc{"packet without dc:title", withoutDCTitle(t, mdt)},
		// The shapes law 5's first run measured, pinned so the rules cannot drift back:
		oracleDoc{"packet: dc:title with its own xml:lang", withPacketBody(t, mdt,
			`<dc:title><rdf:Alt><rdf:li xml:lang="en">A title</rdf:li></rdf:Alt></dc:title>`)},
		oracleDoc{"packet: dc:creator Seq only", withPacketBody(t, mdt,
			`<dc:creator><rdf:Seq><rdf:li>Someone</rdf:li></rdf:Seq></dc:creator>`)},
		oracleDoc{"described form − /StructParent", widgetMutation(t, df, "drop-structparent")},
		// P04.S03: an annotation carrying /Contents whose named element declares no /Lang, in a document
		// whose catalog declares none either — the one shape that reaches 7.2 t24's failing half.
		oracleDoc{"described form + unlanguaged annotation /Contents", widgetMutation(t, df, "contents-no-lang")},
		// 5 t1 and 5 t2 in both directions (`/pending 489`). No product door writes the identification
		// (`/pending 486`), so these are mutations, named for the value they declare.
		oracleDoc{"Markdown + title + pdfuaid:part 1", withUAPart(t, mdt, "1")},
		oracleDoc{"Markdown + title + pdfuaid:part 2", withUAPart(t, mdt, "2")},
		// 5 t3, t4 and t5 in both directions (P06.S01). No product door writes the identification at all
		// (`/pending 486`), so both halves are mutations of the same packet: the same three properties,
		// bound to the same namespace, written once under the required prefix and once under another.
		// The passing document also covers the case veraPDF counts as a real check rather than an absent
		// subject — `amd` and `corr` PRESENT and correctly prefixed.
		oracleDoc{"Markdown + title + identification under pdfuaid", withUAPrefix(t, mdt, "pdfuaid")},
		oracleDoc{"Markdown + title + identification under a foreign prefix", withUAPrefix(t, mdt, "pdfuaia")},
		// 6.1 t1's FAILED half (P06.S01). Every other document here reaches the passing half, since every
		// file nib can open has a header.
		//
		// **It has to be the MAJOR version, and that is a measured constraint rather than a choice.**
		// `%PDF-1.9` is the obvious fixture — one byte, inside the digit class the clause names — and
		// pdfcpu refuses to open it at all: *"headerVersion: unknown PDF Header Version: 1.9"*. So the
		// document veraPDF validates is one nib cannot read, and the clause could never report it.
		// `%PDF-2.0` is a version pdfcpu knows, fails the profile's literal `1.`, and keeps every xref
		// offset in the file. `checkFileHeader` records what that leaves unreachable.
		oracleDoc{"Markdown + title, header %PDF-2.0", withFileHeader(t, mdt, "%PDF-2.0")},
		// P06.S02. `7.11 t1`'s PASSING half is a product door — `AddAttachment` writes both `/F` and
		// `/UF` — and its failing half is a mutation, since nothing nib ships omits one. `7.1 t4` and
		// `7.15 t1` have no product door at all: nib never marks its own tagging as suspect and writes
		// no XFA, so both halves of the first and the failing half of the second are mutations.
		must("attached file", attached, aerr),
		oracleDoc{"attached file − its /UF", withSpecKey(t, attached, "UF", nil)},
		oracleDoc{"committed proposal + Suspects true", withSuspects(t, w.pdf, true)},
		// `Suspects false` was here and reached no veraPDF STATE that another document did not: this
		// harness grades states, and every other document supplies 7.1 t4's passing half. A static XFA
		// form does something nothing else does — it puts 7.15 t1's passing half WITH A SUBJECT in
		// front of veraPDF, which is a different fact from having no AcroForm at all.
		oracleDoc{"Markdown + title + a static XFA form", withStaticXFA(t, mdt)},
		// P06.S03. `7.16 t1` has no product door for EITHER half: `pdfops.Encrypt` sets the same secret
		// as user and owner, and a document with a user password is one veraPDF cannot open. Both are
		// owner-only encryptions, and the restrictive one carries `/P = -3901`, the exact value nib's
		// own `Encrypt` writes. `7.20 t1`'s failing half is a mutation too — nib writes no reference
		// XObject — while its passing half is every other document holding a form.
		oracleDoc{"Markdown + title, encrypted, all permissions", withEncryption(t, mdt, model.PermissionsAll)},
		oracleDoc{"Markdown + title, encrypted, no permissions", withEncryption(t, mdt, model.PermissionsNone)},
		oracleDoc{"committed proposal + a reference XObject", withReferenceXObject(t, w.pdf)},
		// P06.S04. Both halves through the structure editor's own door — the construction that used to
		// be the documented counterexample, now that the checker catches it.
		oracleDoc{"Markdown, a paragraph retyped Formula with no alternate text", withFormulaParagraph(t, mdl, "")},
		oracleDoc{"Markdown, a paragraph retyped Formula with alternate text", withFormulaParagraph(t, mdl, "the quadratic formula")},
		oracleDoc{"Markdown + title + a dynamic XFA form", withDynamicXFA(t, mdt)},
		// 7.4.2 t1's FAILED half (`/pending 487`). No product door writes a skipped level any more, so the
		// structure editor's own door retypes a correctly nested heading one level too deep.
		oracleDoc{"Markdown, second heading retyped H3 (skips a level)", headingSkipped(t)},
		// 7.1 t6 in both directions (`/pending 548`), and the pair is what settles whose subject the clause
		// has. No product door writes a /RoleMap at all, so both are mutations: the SAME circular dictionary
		// in both files, entered by an element in the first and by nothing in the second. veraPDF fails the
		// first and passes the second, which is the element-scoped reading measured rather than argued.
		oracleDoc{"Markdown + title + lang, one element on a role-map loop", withRoleMapCycle(t, mdl, true)},
		oracleDoc{"Markdown + title + lang, a role-map loop no element enters", withRoleMapCycle(t, mdl, false)},
		// 7.1 t5 and t7's FAILED halves (P03.S01). Measured on veraPDF 1.30.2 before being pinned: a
		// non-standard type whose chain dead-ends at another non-standard name fails t5 and nothing about
		// the loop document does; a standard type the map sends to another STANDARD type still fails t7.
		oracleDoc{"Markdown + title + lang, one element typed through a dead-end role map",
			withRoleMapOnLeaf(t, mdl, types.Dict{"Unmapped": types.Name("Nowhere")}, "Unmapped")},
		oracleDoc{"Markdown + title + lang, one element's standard type remapped",
			withRoleMapOnLeaf(t, mdl, types.Dict{"Code": types.Name("Span")}, "Code")},
		// The containment matrix in both directions (P03.S02). No product door writes a table, list or table of
		// contents shaped wrongly, so these are built trees: every relation the matrix holds kept in the first
		// and broken in the second. veraPDF was run on both before they were pinned — 17 of 17 each way.
		oracleDoc{"containment: every relation kept", treeDoc("", "Document("+
			"Table(Caption,THead(TR(TH)),TBody(TR(TD)),TFoot(TR(TD))),"+
			"L(Caption,LI(Lbl,LBody),L(LI(LBody))),TOC(Caption,TOCI,TOC(TOCI)))")},
		oracleDoc{"containment: every relation broken", treeDoc("", "Document("+
			"TR(TD,P),THead(TR(TH)),TBody(TR(TD)),TFoot(TR(TD)),Table(TH,TD,Span),L(P,LBody),P(LI(P)),"+
			"TOCI,TOC(TOCI,P),Table(THead(TD),TBody(TD),TFoot(TD)))")},
		// Cardinality and placement (P03.S03): every one of its eight clauses broken in one tree — the "every
		// relation kept" document above is its passing half. Measured on veraPDF before it was pinned.
		oracleDoc{"containment: every count and position broken", treeDoc("", cardinalityBroken)},
		// Table geometry and headers (P03.S04) — veraPDF's grid ported from its own source. Every error the layout
		// can stop at, one table each (the first error ends a table's layout, so they cannot share one), and a
		// regular table with row and column spans for the passing half. Measured on veraPDF before pinned.
		oracleDoc{"table geometry: every error, one table each", treeDoc("", "Document("+
			"Table(TR(TD,TD!r2),TR(TD!c2)),Table(TR(TD,TD!r3),TR(TD,TD)),Table(TR(TD,TD),TR(TD,TD,TD)),"+
			"Table(TR(TD,TD),TR(TD)),Table(TR(TH!id=a,TH!id=b),TR(TD!headers=zz,TD)))")},
		// Strongly or weakly structured (P03.S05), both halves, each tree measured on veraPDF before pinned.
		oracleDoc{"headings: H beside H, and an H1 among them", treeDoc("", "Document(H,H1,H)")},
		oracleDoc{"headings: unnumbered H only, one per section", treeDoc("", "Document(Sect(H,P),Sect(H,P))")},
		// Notes and the Form element (P03.S06), both halves, each measured on veraPDF before pinned.
		oracleDoc{"notes: two, each with its own ID", treeDoc("", "Document(Note!id=n1,Note!id=n2)")},
		oracleDoc{"notes: two with the same empty ID", treeDoc("", "Document(Note!id=,Note!id=)")},
		oracleDoc{"form: one object reference to its widget", formChildDoc(true, "[<< /Type /OBJR /Obj 9 0 R >>]", "")},
		oracleDoc{"form: an MCID beside the widget's reference", formChildDoc(true, "[<< /Type /OBJR /Obj 9 0 R >> 0]", "")},
		oracleDoc{"table geometry: a regular spanned table", treeDoc("", "Document(Table(TR(TH!r2!scope=Row,TH!c2!scope=Column),TR(TD,TD)))")},
		// Language (P04.S01), both halves of 7.2 t2 and t29, each measured on veraPDF before pinned.
		oracleDoc{"language: an outline, an empty catalog /Lang", langClauseDoc("/Lang ()", "", langText, "", true)},
		oracleDoc{"language: an outline, no catalog /Lang, a bad /Lang on a Span", langClauseDoc("", "", langSpanText("<< /Lang (en_US) >>"), "", true)},
		oracleDoc{"language: an outline, en-US, de on a Span", langClauseDoc("/Lang (en-US)", "", langSpanText("<< /Lang (de) >>"), "", true)},
		// Marked content (P04.S04), both halves of all five clauses. The artifact document reaches 7.1 t1 AND
		// 7.1 t2's failing half at once — one sequence breaks both, because `parentsTags` includes its own tag.
		oracleDoc{"marked content: an artifact inside tagged content",
			markedDoc{content: "/P <</MCID 0>> BDC\n" + markedArtifact + "\n72 100 100 10 re f\nEMC\n" + markedText + "\nEMC"}.build()},
		oracleDoc{"marked content: an artifact beside tagged content",
			markedDoc{content: markedArtifact + "\n72 100 100 10 re f\nEMC\n/P <</MCID 0>> BDC\n" + markedText + "\nEMC"}.build()},
		// One Span carrying all three alternates with no language anywhere reaches t30, t31 and t32's failing
		// half; the same Span with its own /Lang reaches the passing half through the disjunct that matters.
		oracleDoc{"marked content: a Span with every alternate and no language",
			markedDoc{content: "/P <</MCID 0>> BDC\n/Span << /ActualText (a) /Alt (b) /E (c) >> BDC\n" + markedText + "\nEMC\nEMC"}.build()},
		oracleDoc{"marked content: a Span with every alternate and its own language",
			markedDoc{content: "/P <</MCID 0>> BDC\n/Span << /ActualText (a) /Alt (b) /E (c) /Lang (en-US) >> BDC\n" + markedText + "\nEMC\nEMC"}.build()},
		// **`/pending 635`'s own fixture, and the reason it was held out is gone.** A structure `/Lang` on an
		// ANCESTOR of the element an annotation names moved 7.2 t34, where nib's second climb
		// (`declaresLangFor`) answered differently from `parentLang` — so adding this document used to make
		// the slice red over another rule's open defect. P04.S04 deleted that climb and t34 reads the one
		// door, so the document belongs in the corpus now, with its near control beside it.
		oracleDoc{"language: an annotation whose /Contents has a language only on an ancestor",
			widgetMutation(t, describedForm(t), "contents-lang-on-ancestor")},
		oracleDoc{"language: an annotation whose /StructParent element declares its own",
			widgetMutation(t, describedForm(t), "contents-lang-on-element")},
	)
	if pdfops.LibreOfficeAvailable() {
		lo, err := pdfops.ConvertOfficeToPDF(oracleODT(t), "odt")
		if err != nil {
			t.Fatalf("corpus: LibreOffice is present and could not convert the fixture: %v", err)
		}
		docs = append(docs, oracleDoc{"LibreOffice ODT", lo})
		docs = append(docs, tableFigureCorpus(t)...)
	} else {
		t.Log("NOTE (a narrower corpus, not a pass): LibreOffice is absent, so the documents " +
			"whose structure nib did not write — and the only tables and figures — are missing from this run")
	}
	return docs
}

// headingSkipped is converted Markdown whose second heading the structure editor retyped H3, so the tree
// goes H1 → H3: the 7.4.2 t1 failure no product door writes since `/pending 487`.
func headingSkipped(t *testing.T) []byte {
	t.Helper()
	md, err := pdfops.ConvertDocToPDF([]byte("# Top\n\n## Next\n"), ".md")
	if err != nil {
		t.Fatalf("corpus: %v", err)
	}
	tree, err := pdfops.ReadStructure(md)
	if err != nil {
		t.Fatalf("corpus: %v", err)
	}
	id := 0
	for _, e := range tree.Elements {
		if e.Standard == "H2" && e.ID > 0 {
			id = e.ID
		}
	}
	if id == 0 {
		t.Fatalf("corpus: the converted Markdown has no addressable H2 to retype: %+v", tree.Elements)
	}
	out, err := pdfops.EditStructure(md, []pdfops.StructureEdit{{Kind: "retype", Element: id, Value: "H3", Index: -1}})
	if err != nil {
		t.Fatalf("corpus: retyping the H2: %v", err)
	}
	return out
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
// A row appearing is a gap in nib a person must look at; a row that stops appearing means the gap closed
// and the row is a claim about code that no longer behaves that way.
var knownCannotCheck = map[string]string{
	// P06.S04. `7.7 t1` cannot type an element whose role map loops, exactly as `7.3 t1` cannot — both
	// refuse rather than say "the document has no Formula", because the untypable element MAY be one.
	"Markdown + title + lang, one element on a role-map loop / 7.7 t1": "the role map loops, so no element can be typed and any of them might be the Formula",
	// `/pending 548`: the tree rules over the document whose role map loops. They KEEP answering `CannotCheck`
	// and that is right — the cycle is reported by 7.1 t6, which is the clause about the cycle, and they still
	// cannot type the element their own subject might be. (7.5 t1 was a third row until P03.S04 ported
	// veraPDF's table algorithm, which never takes an untyped element for a cell; the oracle agrees with it.)
	"Markdown + title + lang, one element on a role-map loop / 7.3 t1":   "an element on a role-map loop may be the Figure",
	"Markdown + title + lang, one element on a role-map loop / 7.4.2 t1": "an element on a role-map loop may be a numbered heading, and one moves the whole sequence",
}

// cardinalityBroken breaks every P03.S03 clause at once: two THeads and two TFoots with no TBody, a Caption
// in the middle and a second one, and a TOC and an L each with its Caption last.
const cardinalityBroken = "Document(Table(THead(TR(TH)),THead(TR(TH)),TFoot(TR(TD)),TFoot(TR(TD)),Caption,TR(TD),Caption,TR(TD))," +
	"TOC(TOCI,Caption),L(LI(LBody),Caption))"

// notYetReachable records a veraPDF state a clause cannot reach on any corpus document yet, with the
// coordinate that will make it reachable. Checked in both directions, and against the plan's marker.
var notYetReachable = map[string]string{
	// Empty since `/pending 489`. It held `5 t1 passed`, gated on /pending 486, because no nib document
	// carries the identification; the corpus now reaches both halves of 5 t1 and 5 t2 through the two
	// `pdfuaid:part` mutations above. No product door writes the identification — that is still 486.
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
	// because another document happened to reach the same state. LibreOffice's documents are counted
	// separately — their absence narrows the corpus loudly rather than failing it.
	generated := 0
	for _, d := range docs {
		if !strings.HasPrefix(d.name, "LibreOffice") {
			generated++
		}
	}
	const wantGenerated = 78
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
