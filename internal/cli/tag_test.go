package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"nib/internal/pdfops"
	"nib/internal/testpdf"
)

// `nib tag tree` and `nib tag propose` — `PLAN-accessibility.md` P10.S01.

// taggedMarkdownPDF writes a converted Markdown document — tagged from its own structure — into dir.
func taggedMarkdownPDF(t *testing.T, dir string) string {
	t.Helper()
	pdf, err := pdfops.ConvertDocToPDF([]byte("# A heading\n\nA paragraph of body text.\n\n- one\n- two\n"), ".md")
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "tagged.pdf")
	if err := os.WriteFile(p, pdf, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestTagTreePrintsTheTreeTheDocumentHas — one line per element with its type and text; `--json` reads
// back as exactly what the door answers, under the route's field names; the file is not touched.
func TestTagTreePrintsTheTreeTheDocumentHas(t *testing.T) {
	p := taggedMarkdownPDF(t, t.TempDir())
	before := readPDF(t, p)
	want, err := pdfops.ReadStructure(before)
	if err != nil || !want.Tagged || len(want.Elements) < 3 {
		t.Fatalf("setup: the converted document reads %+v, %v", want, err)
	}

	out, code := captureStdout(t, func() int { return cmdTag([]string{"tree", p}) })
	if code != 0 {
		t.Fatalf("nib tag tree exited %d", code)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != len(want.Elements) {
		t.Fatalf("printed %d line(s) for %d element(s):\n%s", len(lines), len(want.Elements), out)
	}
	for i, e := range want.Elements {
		if !strings.Contains(lines[i], e.Standard) {
			t.Errorf("line %d does not name element %d's type %s: %q", i, i, e.Standard, lines[i])
		}
	}
	if !strings.Contains(out, "A paragraph of body text") {
		t.Errorf("the tree does not show the text under its elements:\n%s", out)
	}

	js, code := captureStdout(t, func() int { return cmdTag([]string{"tree", "--json", p}) })
	if code != 0 {
		t.Fatalf("nib tag tree --json exited %d", code)
	}
	var got pdfops.StructureTree
	if err := json.Unmarshal([]byte(js), &got); err != nil {
		t.Fatalf("--json is not JSON: %v\n%.300s", err, js)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("--json reads back as\n%+v\nand the door answers\n%+v", got, want)
	}
	for _, name := range []string{`"hasAlt"`, `"pageBox"`, `"unaddressable"`} {
		if !strings.Contains(js, name) {
			t.Errorf("--json does not carry the route's field %s:\n%.300s", name, js)
		}
	}
	if !bytes.Equal(before, readPDF(t, p)) {
		t.Error("reading the tree changed the file")
	}
}

// TestTagTreeShowsNestingAndWhatIsMissing — an element is indented under its parent, and a figure with no
// alt text or a header cell with no scope says so. The two defects are made through the product's own
// edit door on the converted document, so the fixture is a tree nib can reach, not one hand-assembled.
func TestTagTreeShowsNestingAndWhatIsMissing(t *testing.T) {
	dir := t.TempDir()
	src := readPDF(t, taggedMarkdownPDF(t, dir))
	tree, err := pdfops.ReadStructure(src)
	if err != nil {
		t.Fatal(err)
	}
	// The converted document has one paragraph (its list items are LBody) and one heading: the paragraph
	// becomes the figure and the heading the header cell.
	paragraph, heading := 0, 0
	for _, e := range tree.Elements {
		switch {
		case e.Standard == "P" && e.ID > 0 && paragraph == 0:
			paragraph = e.ID
		case e.Standard == "H1" && e.ID > 0 && heading == 0:
			heading = e.ID
		}
	}
	if paragraph == 0 || heading == 0 {
		t.Fatalf("setup: the converted document has paragraph %d and heading %d", paragraph, heading)
	}
	edited, err := pdfops.EditStructure(src, []pdfops.StructureEdit{
		{Kind: "retype", Element: paragraph, Value: "Figure", Index: -1},
		{Kind: "retype", Element: heading, Value: "TH", Index: -1},
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	p := filepath.Join(dir, "edited.pdf")
	if err := os.WriteFile(p, edited, 0o644); err != nil {
		t.Fatal(err)
	}
	want, err := pdfops.ReadStructure(edited)
	if err != nil {
		t.Fatal(err)
	}

	out, code := captureStdout(t, func() int { return cmdTag([]string{"tree", p}) })
	if code != 0 {
		t.Fatalf("nib tag tree exited %d", code)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != len(want.Elements) {
		t.Fatalf("printed %d line(s) for %d element(s):\n%s", len(lines), len(want.Elements), out)
	}
	// The id column is seven characters wide; the indentation is what follows it.
	indent := func(line string) int {
		if len(line) < 7 {
			return 0
		}
		rest := line[7:]
		return len(rest) - len(strings.TrimLeft(rest, " "))
	}
	nested := 0
	for i, e := range want.Elements {
		if e.Parent < 0 {
			continue
		}
		nested++
		if indent(lines[i]) <= indent(lines[e.Parent]) {
			t.Errorf("element %d (%s) is not indented under its parent %d (%s):\n  %q\n  %q", i, e.Standard, e.Parent, want.Elements[e.Parent].Standard, lines[e.Parent], lines[i])
		}
	}
	if nested == 0 {
		t.Fatal("setup: no element has a parent, so nesting is not exercised")
	}
	if !strings.Contains(out, "Figure") || !strings.Contains(out, "no alt text") {
		t.Errorf("a figure with no alt text does not say so:\n%s", out)
	}
	if !strings.Contains(out, "TH") || !strings.Contains(out, "no scope") {
		t.Errorf("a header cell with no scope does not say so:\n%s", out)
	}
}

// TestTagTreeOnAnUntaggedDocumentSaysSoAndSucceeds — no tree is an answer, not a failure.
func TestTagTreeOnAnUntaggedDocumentSaysSoAndSucceeds(t *testing.T) {
	p := writePDF(t, t.TempDir(), "plain.pdf", "plain text")
	var out string
	var code int
	errOut := captureStderr(t, func() { out, code = captureStdout(t, func() int { return cmdTag([]string{"tree", p}) }) })
	if code != 0 || strings.TrimSpace(out) != "" {
		t.Errorf("an untagged document: exit %d, stdout %q — want 0 and nothing on stdout", code, out)
	}
	if !strings.Contains(errOut, "no structure tree") {
		t.Errorf("an untagged document is not said to have no tree: %q", errOut)
	}
	js, _ := captureStdout(t, func() int { return cmdTag([]string{"tree", "--json", p}) })
	var got pdfops.StructureTree
	if err := json.Unmarshal([]byte(js), &got); err != nil || got.Tagged || got.Elements == nil || len(got.Elements) != 0 {
		t.Errorf("--json on an untagged document reads %+v (%v) — want untagged with an empty list", got, err)
	}
}

// TestTagProposePrintsTheProposalTheCardReads — the same, for the proposal.
func TestTagProposePrintsTheProposalTheCardReads(t *testing.T) {
	p := writePDF(t, t.TempDir(), "plain.pdf", "A page of plain text to propose a structure for")
	before := readPDF(t, p)
	want, err := pdfops.ProposeTags(before)
	if err != nil || len(want.Elements) == 0 {
		t.Fatalf("setup: the proposal reads %+v, %v", want, err)
	}
	out, code := captureStdout(t, func() int { return cmdTag([]string{"propose", p}) })
	if code != 0 {
		t.Fatalf("nib tag propose exited %d", code)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != len(want.Elements) {
		t.Fatalf("printed %d line(s) for %d proposed element(s):\n%s", len(lines), len(want.Elements), out)
	}
	for i, e := range want.Elements {
		if !strings.Contains(lines[i], e.Role) {
			t.Errorf("line %d does not name the proposed role %s: %q", i, e.Role, lines[i])
		}
	}
	js, code := captureStdout(t, func() int { return cmdTag([]string{"propose", "--json", p}) })
	if code != 0 {
		t.Fatalf("nib tag propose --json exited %d", code)
	}
	var got pdfops.TagProposal
	if err := json.Unmarshal([]byte(js), &got); err != nil {
		t.Fatalf("--json is not JSON: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("--json reads back as\n%+v\nand the door answers\n%+v", got, want)
	}
	if !strings.Contains(js, `"noText"`) || !strings.Contains(js, `"pageBox"`) {
		t.Errorf("--json does not carry the route's field names:\n%.300s", js)
	}
	if !bytes.Equal(before, readPDF(t, p)) {
		t.Error("proposing changed the file")
	}
}

// TestTagProposeIndentsATablesRowsAndCells — ADR-121: a proposed table prints as it is nested, its rows under
// it and its cells under their row, and `--json` carries each element's parent.
func TestTagProposeIndentsATablesRowsAndCells(t *testing.T) {
	content := "100 700 m 400 700 l S 100 670 m 400 670 l S 100 640 m 400 640 l S " +
		"100 640 m 100 700 l S 250 640 m 250 700 l S 400 640 m 400 700 l S " +
		"BT /F1 12 Tf 106 680 Td (Name) Tj ET BT /F1 12 Tf 256 680 Td (Qty) Tj ET " +
		"BT /F1 12 Tf 106 650 Td (Apple) Tj ET BT /F1 12 Tf 256 650 Td (3) Tj ET"
	p := filepath.Join(t.TempDir(), "ruled.pdf")
	if err := os.WriteFile(p, testpdf.WithContent(content, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, code := captureStdout(t, func() int { return cmdTag([]string{"propose", p}) })
	if code != 0 {
		t.Fatalf("nib tag propose exited %d", code)
	}
	var got []string
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			t.Fatalf("a line names no role: %q", line)
		}
		// The indent is what stands between the id column and the role.
		indent := strings.Index(line, f[1]) - 5
		got = append(got, strings.Repeat(">", indent/2)+f[1])
	}
	if want := "Table >TR >>TH >>TH >TR >>TD >>TD"; strings.Join(got, " ") != want {
		t.Errorf("printed\n%s\nwhich nests as %q, want %q", out, strings.Join(got, " "), want)
	}
	js, code := captureStdout(t, func() int { return cmdTag([]string{"propose", "--json", p}) })
	if code != 0 {
		t.Fatalf("nib tag propose --json exited %d", code)
	}
	var prop pdfops.TagProposal
	if err := json.Unmarshal([]byte(js), &prop); err != nil {
		t.Fatalf("--json is not JSON: %v", err)
	}
	var parents []int
	for _, e := range prop.Elements {
		parents = append(parents, e.Parent)
	}
	if !reflect.DeepEqual(parents, []int{-1, 0, 1, 1, 0, 4, 4}) || !strings.Contains(js, `"parent": -1`) {
		t.Errorf("--json carries parents %v", parents)
	}
}

// TestTheTagCommandsReachTheRoutesDoors — routing: each subcommand calls the door its route calls, and the
// read-only ones write nothing.
func TestTheTagCommandsReachTheRoutesDoors(t *testing.T) {
	src, err := os.ReadFile("tag.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	fn := func(name string) string {
		i := strings.Index(body, "func "+name+"(")
		if i < 0 {
			t.Fatalf("%s is gone", name)
		}
		j := strings.Index(body[i:], "\n}\n")
		return body[i : i+j]
	}
	for name, door := range map[string]string{"tagTree": "pdfops.ReadStructure(", "tagPropose": "pdfops.ProposeTags(", "tagUntagged": "pdfops.ReadUntagged("} {
		f := fn(name)
		if !strings.Contains(f, door) {
			t.Errorf("%s does not reach %s", name, door)
		}
		for _, write := range []string{"writeOut(", "writeOutput(", "os.WriteFile(", "writeAtomic("} {
			if strings.Contains(f, write) {
				t.Errorf("%s calls %s — a read-only subcommand writes nothing", name, write)
			}
		}
	}
}

// TestTagRefusesAMissingOrUnknownSubcommand.
func TestTagRefusesAMissingOrUnknownSubcommand(t *testing.T) {
	for _, args := range [][]string{nil, {"paint"}} {
		var code int
		captureStderr(t, func() { code = cmdTag(args) })
		if code != 1 {
			t.Errorf("nib tag %v exited %d, want 1", args, code)
		}
	}
}
