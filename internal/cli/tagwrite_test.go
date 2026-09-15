package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"nib/internal/pdfops"
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
