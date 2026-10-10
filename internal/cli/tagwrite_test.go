package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"nib/internal/pdfops"
	"nib/internal/testpdf"
)

// `nib tag commit` and `nib tag edit` — `PLAN-accessibility.md` P10.S02.

// runTag runs `nib tag` with args, returning stdout, stderr and the exit status.
func runTag(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	var out string
	var code int
	errOut := captureStderr(t, func() { out, code = captureStdout(t, func() int { return cmdTag(args) }) })
	return out, errOut, code
}

// writeFile writes body to name in dir and returns its path.
func writeFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestTagCommitWritesTheProposalItReviewed — `nib tag propose --json` is a review that keeps every role, and
// commit writes it: to -o leaving the input alone, or with -w over the input.
func TestTagCommitWritesTheProposalItReviewed(t *testing.T) {
	dir := t.TempDir()
	in := writePDF(t, dir, "plain.pdf", "A page of plain text to tag")
	before := readPDF(t, in)
	js, _, code := runTag(t, "propose", "--json", in)
	if code != 0 {
		t.Fatalf("propose --json exited %d", code)
	}
	review := writeFile(t, dir, "review.json", js)

	out := filepath.Join(dir, "tagged.pdf")
	if _, errOut, code := runTag(t, "commit", in, "-o", out, "--review", review); code != 0 {
		t.Fatalf("commit exited %d: %s", code, errOut)
	}
	tree, err := pdfops.ReadStructure(readPDF(t, out))
	if err != nil || !tree.Tagged || len(tree.Elements) == 0 {
		t.Errorf("the committed document reads %+v, %v — want a tree", tree, err)
	}
	if !bytes.Equal(before, readPDF(t, in)) {
		t.Error("commit -o changed the input")
	}

	stdout, errOut, code := runTag(t, "commit", "-w", in, "--review", review)
	if code != 0 || !strings.Contains(stdout, "rewritten") {
		t.Fatalf("commit -w exited %d, stdout %q: %s", code, stdout, errOut)
	}
	if tree, err := pdfops.ReadStructure(readPDF(t, in)); err != nil || !tree.Tagged {
		t.Errorf("commit -w left the input untagged: %+v, %v", tree, err)
	}
}

// TestTagEditAppliesTheBatch — ids from `nib tag tree`, one batch, one write.
func TestTagEditAppliesTheBatch(t *testing.T) {
	dir := t.TempDir()
	in := taggedMarkdownPDF(t, dir)
	tree, err := pdfops.ReadStructure(readPDF(t, in))
	if err != nil {
		t.Fatal(err)
	}
	paragraph := 0
	for _, e := range tree.Elements {
		if e.Standard == "P" && e.ID > 0 {
			paragraph = e.ID
			break
		}
	}
	if paragraph == 0 {
		t.Fatal("setup: no addressable paragraph")
	}
	edits := writeFile(t, dir, "edits.json", `{"edits":[{"kind":"retype","element":`+itoa(paragraph)+`,"value":"Figure"},{"kind":"alt","element":`+itoa(paragraph)+`,"value":"a chart"}]}`)
	out := filepath.Join(dir, "edited.pdf")
	if _, errOut, code := runTag(t, "edit", in, "-o", out, "--edits", edits); code != 0 {
		t.Fatalf("edit exited %d: %s", code, errOut)
	}
	got, err := pdfops.ReadStructure(readPDF(t, out))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range got.Elements {
		if e.ID == paragraph {
			found = e.Standard == "Figure" && e.HasAlt && e.Alt == "a chart"
		}
	}
	if !found {
		t.Errorf("element %d was not retyped to a Figure with its alt text: %+v", paragraph, got.Elements)
	}
}

// TestTagWriteRefusesASignedDocument — through the door the routes share: both subcommands, -o and -w,
// nothing written.
func TestTagWriteRefusesASignedDocument(t *testing.T) {
	dir := t.TempDir()
	// A neutral name: the error names the file, and a file called "signed" would satisfy the assertion.
	in := filepath.Join(dir, "doc.pdf")
	signed := signedTestPDF(t)
	if err := os.WriteFile(in, signed, 0o644); err != nil {
		t.Fatal(err)
	}
	js, _, _ := runTag(t, "propose", "--json", in)
	review := writeFile(t, dir, "review.json", js)
	edits := writeFile(t, dir, "edits.json", `{"edits":[{"kind":"alt","element":1,"value":"x"}]}`)
	out := filepath.Join(dir, "out.pdf")
	for _, args := range [][]string{
		{"commit", in, "-o", out, "--review", review},
		{"commit", "-w", in, "--review", review},
		{"edit", in, "-o", out, "--edits", edits},
		{"edit", "-w", in, "--edits", edits},
		{"remove", in, "-o", out},
		{"remove", "-w", in},
	} {
		_, errOut, code := runTag(t, args...)
		if code != 1 || !strings.Contains(errOut, "would change the bytes its signatures cover") {
			t.Errorf("nib tag %v on a signed document: exit %d, %q — want 1 naming the signature", args, code, errOut)
		}
		if _, err := os.Stat(out); err == nil {
			t.Errorf("nib tag %v wrote %s", args, out)
		}
	}
	if !bytes.Equal(signed, readPDF(t, in)) {
		t.Error("a refused write changed the signed document")
	}
}

// TestTagWriteTellsAStaleRequestFromAMalformedOne — 1 with the door's sentence for a request the document
// no longer matches; 2 for one no document could take.
func TestTagWriteTellsAStaleRequestFromAMalformedOne(t *testing.T) {
	dir := t.TempDir()
	tagged := taggedMarkdownPDF(t, dir)
	plain := writePDF(t, dir, "plain.pdf", "Some plain text")
	out := filepath.Join(dir, "out.pdf")
	staleReview := writeFile(t, dir, "stale.json", `{"elements":[{"id":0,"role":"P","text":"not the text on the page"}]}`)
	for _, tc := range []struct {
		name string
		args []string
		code int
		says string
	}{
		{"an element the tree does not have", []string{"edit", tagged, "-o", out, "--edits", writeFile(t, dir, "e1.json", `{"edits":[{"kind":"alt","element":999999,"value":"x"}]}`)}, 1, "not in this document's structure tree"},
		{"a review of other text", []string{"commit", plain, "-o", out, "--review", staleReview}, 1, "propose again"},
		{"an edit that is not one", []string{"edit", tagged, "-o", out, "--edits", writeFile(t, dir, "e2.json", `{"edits":[{"kind":"paint","element":1}]}`)}, 2, "not an edit"},
		{"a request that is not JSON", []string{"edit", tagged, "-o", out, "--edits", writeFile(t, dir, "e3.json", `not json`)}, 2, "could not be read"},
		{"no request file named", []string{"commit", plain, "-o", out}, 2, "--review"},
	} {
		_, errOut, code := runTag(t, tc.args...)
		if code != tc.code || !strings.Contains(errOut, tc.says) {
			t.Errorf("%s: exit %d, %q — want %d saying %q", tc.name, code, errOut, tc.code, tc.says)
		}
		if _, err := os.Stat(out); err == nil {
			t.Fatalf("%s: wrote %s", tc.name, out)
		}
	}
	if _, errOut, code := runTag(t, "commit", "-w", plain, tagged, "--review", staleReview); code != 1 || !strings.Contains(errOut, "exactly one") {
		t.Errorf("-w over two files: exit %d, %q — want 1: a request describes one document", code, errOut)
	}
}

func itoa(n int) string { return strconv.Itoa(n) }

// TestTagEditWritesACellsSpansAndHeadersAndTheTreePrintsThem — ADR-119, through the command: the header
// cells are named by the ids `nib tag tree` prints, and the tree then says what each cell spans and which
// cells head it.
func TestTagEditWritesACellsSpansAndHeadersAndTheTreePrintsThem(t *testing.T) {
	dir := t.TempDir()
	in := writeFile(t, dir, "table.pdf", string(testpdf.Assemble(map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R >>",
		2:  "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << >> /Contents 4 0 R >>",
		4:  "<< /Length 1 >>\nstream\n \nendstream",
		7:  "<< /Type /StructTreeRoot /K [8 0 R] >>",
		8:  "<< /Type /StructElem /S /Table /P 7 0 R /K [10 0 R 11 0 R] >>",
		10: "<< /Type /StructElem /S /TR /P 8 0 R /K [12 0 R] >>",
		11: "<< /Type /StructElem /S /TR /P 8 0 R /K [14 0 R 15 0 R] >>",
		12: "<< /Type /StructElem /S /TH /P 10 0 R >>",
		14: "<< /Type /StructElem /S /TD /P 11 0 R >>",
		15: "<< /Type /StructElem /S /TD /P 11 0 R >>",
	})))
	edits := writeFile(t, dir, "edits.json", `{"edits":[{"kind":"colspan","element":12,"value":"2"},{"kind":"rowspan","element":12,"value":"3"},{"kind":"headers","element":15,"headers":[12]}]}`)
	out := filepath.Join(dir, "edited.pdf")
	if _, errOut, code := runTag(t, "edit", in, "-o", out, "--edits", edits); code != 0 {
		t.Fatalf("edit exited %d: %s", code, errOut)
	}
	stdout, errOut, code := runTag(t, "tree", out)
	if code != 0 {
		t.Fatalf("tree exited %d: %s", code, errOut)
	}
	var th, td string
	for _, line := range strings.Split(stdout, "\n") {
		switch {
		case strings.HasPrefix(line, "12 "):
			th = line
		case strings.HasPrefix(line, "15 "):
			td = line
		}
	}
	if !strings.Contains(th, "spans 2 columns") || !strings.Contains(th, "spans 3 rows") || !strings.Contains(td, "headed by: 12") {
		t.Errorf("the tree prints the header as %q and the cell as %q", th, td)
	}
}

// TestTagRemoveLeavesADocumentThatCanBeTaggedAgain — ADR-120, through the command: commit refuses a tagged
// document and names the way out; after `nib tag remove` the tree is gone and the same commit succeeds.
func TestTagRemoveLeavesADocumentThatCanBeTaggedAgain(t *testing.T) {
	dir := t.TempDir()
	in := taggedMarkdownPDF(t, dir)
	bare := filepath.Join(dir, "bare.pdf")
	if _, errOut, code := runTag(t, "remove", in, "-o", bare); code != 0 {
		t.Fatalf("remove exited %d: %s", code, errOut)
	}
	if tree, err := pdfops.ReadStructure(readPDF(t, bare)); err != nil || tree.Tagged {
		t.Fatalf("after remove the document still reads as tagged: %+v, %v", tree.Tagged, err)
	}
	js, errOut, code := runTag(t, "propose", "--json", bare)
	if code != 0 {
		t.Fatalf("propose exited %d: %s", code, errOut)
	}
	review := writeFile(t, dir, "review.json", js)
	if _, errOut, code := runTag(t, "commit", in, "-o", filepath.Join(dir, "over.pdf"), "--review", review); code == 0 || !strings.Contains(errOut, "remove its tags first") {
		t.Errorf("commit over the tagged original: exit %d, %q — want a refusal naming the way out", code, errOut)
	}
	again := filepath.Join(dir, "again.pdf")
	if _, errOut, code := runTag(t, "commit", bare, "-o", again, "--review", review); code != 0 {
		t.Fatalf("commit after remove exited %d: %s", code, errOut)
	}
	if tree, err := pdfops.ReadStructure(readPDF(t, again)); err != nil || !tree.Tagged {
		t.Errorf("the document was not tagged again: %v", err)
	}
	if _, errOut, code := runTag(t, "remove", bare, "-o", filepath.Join(dir, "twice.pdf")); code != 1 || !strings.Contains(errOut, "no structure tree") {
		t.Errorf("remove on an untagged document: exit %d, %q — want 1 saying it has no tree", code, errOut)
	}
}

// picturedPDF is an untagged page with a line of text and one image XObject drawn 100 by 80 points (ADR-122).
func picturedPDF() []byte {
	content := "BT /F1 12 Tf 72 700 Td (Above the picture) Tj ET q 100 0 0 80 50 500 cm /Im0 Do Q"
	return testpdf.Assemble(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /Lang (en-US) >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> /XObject << /Im0 6 0 R >> >> /Contents 4 0 R >>",
		4: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
		5: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		6: "<< /Type /XObject /Subtype /Image /Width 2 /Height 2 /ColorSpace /DeviceGray /BitsPerComponent 8 /Length 4 >>\nstream\nabcd\nendstream",
	})
}

// TestTagProposeNamesAFigureAndCommitWantsItsDescription — ADR-122 on the command line: `propose` prints a
// Figure with its page and the box its picture fills; `propose --json` is a review of everything but what the
// picture shows, so committing it as printed is refused (exit 2, with the reason), and with `"alt"` filled in
// it writes a Figure that `tag tree` reads back.
func TestTagProposeNamesAFigureAndCommitWantsItsDescription(t *testing.T) {
	dir := t.TempDir()
	in, out, review := filepath.Join(dir, "in.pdf"), filepath.Join(dir, "out.pdf"), filepath.Join(dir, "review.json")
	if err := os.WriteFile(in, picturedPDF(), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, _, code := runTag(t, "propose", in)
	if code != 0 {
		t.Fatalf("nib tag propose exited %d", code)
	}
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[1], "1    Figure p1   a picture at 50 500 150 580") || !strings.Contains(lines[1], `"alt"`) {
		t.Errorf("printed\n%s\nwant a second line naming the Figure, its page and its box, and what it needs", stdout)
	}
	js, _, code := runTag(t, "propose", "--json", in)
	if code != 0 {
		t.Fatalf("nib tag propose --json exited %d", code)
	}
	if err := os.WriteFile(review, []byte(js), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, errOut, code := runTag(t, "commit", in, "-o", out, "--review", review); code != 2 || !strings.Contains(errOut, "a figure needs a description of what it shows, or must be ignored") {
		t.Errorf("committing the proposal as printed exited %d saying %q, want 2 and the reason", code, errOut)
	}
	if _, err := os.Stat(out); err == nil {
		t.Error("a refused commit wrote its output")
	}
	var prop map[string]any
	if err := json.Unmarshal([]byte(js), &prop); err != nil {
		t.Fatal(err)
	}
	prop["elements"].([]any)[1].(map[string]any)["alt"] = "A grey square"
	described, _ := json.Marshal(prop)
	if err := os.WriteFile(review, described, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, errOut, code := runTag(t, "commit", in, "-o", out, "--review", review); code != 0 {
		t.Fatalf("committing the described figure exited %d: %s", code, errOut)
	}
	tree, _, code := runTag(t, "tree", out)
	if code != 0 || !strings.Contains(tree, "Figure") || !strings.Contains(tree, "A grey square") {
		t.Errorf("nib tag tree exited %d and printed\n%s\nwant the Figure and its description", code, tree)
	}
}
