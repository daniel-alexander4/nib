package pdfops

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"nib/internal/testpdf"
)

// unwalkableFilesPDF is a two-page document with two embedded files, a.txt and b.txt, where a.txt's file
// specification carries a private entry nested deeper than `reachesObject` follows. With broken, a third entry
// c.txt names a stream whose FlateDecode data is not flate, so its bytes cannot be read at all.
func unwalkableFilesPDF(broken bool) []byte {
	deep := strings.Repeat("<< /X ", 40) + "1" + strings.Repeat(" >>", 40)
	names := "(a.txt) 10 0 R (b.txt) 12 0 R"
	objs := map[int]string{
		2:  "<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 >>",
		3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] >>",
		4:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] >>",
		10: "<< /Type /Filespec /F (a.txt) /UF (a.txt) /EF << /F 11 0 R >> /Private " + deep + " >>",
		11: "<< /Type /EmbeddedFile /Length 7 >>\nstream\nA-BYTES\nendstream",
		12: "<< /Type /Filespec /F (b.txt) /UF (b.txt) /EF << /F 13 0 R >> >>",
		13: "<< /Type /EmbeddedFile /Length 7 >>\nstream\nB-BYTES\nendstream",
	}
	if broken {
		names += " (c.txt) 14 0 R"
		objs[14] = "<< /Type /Filespec /F (c.txt) /UF (c.txt) /EF << /F 15 0 R >> >>"
		objs[15] = "<< /Type /EmbeddedFile /Filter /FlateDecode /Length 9 >>\nstream\nnot-flate\nendstream"
	}
	objs[1] = fmt.Sprintf("<< /Type /Catalog /Pages 2 0 R /Names << /EmbeddedFiles << /Names [%s] >> >> >>", names)
	return testpdf.Assemble(objs)
}

// TestAnInsertAndADuplicateKeepFilesTheSpliceCouldNotWalk — `/pending 870`. `documentFacts.restore` withholds the
// embedded-files tree when an entry "reaches a dropped page", and answers yes for an entry it could not walk
// (`reachesObject`: nesting past its cap, a reference that will not dereference). An insert and a duplicate drop
// no page, so they threw that answer away by name — and one unwalkable entry took EVERY file of the document out
// of the result, with nothing said. They now take the second way back the delete and the splits take
// (`filesFromBytes`): each file is re-added from its bytes, and the ones even that cannot carry are counted.
func TestAnInsertAndADuplicateKeepFilesTheSpliceCouldNotWalk(t *testing.T) {
	other := untaggedFixture()
	files := func(pdf []byte) string {
		t.Helper()
		list, err := Attachments(pdf)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, a := range list {
			b, err := ExtractAttachment(pdf, a.Name)
			if err != nil {
				b = []byte("unreadable")
			}
			out = append(out, a.Name+"="+string(b))
		}
		sort.Strings(out)
		return strings.Join(out, " ")
	}
	for _, tc := range []struct {
		name  string
		drive func([]byte) ([]byte, int, error)
	}{
		{"DuplicatePage", func(b []byte) ([]byte, int, error) { return DuplicatePage(b, 1) }},
		{"InsertPDF", func(b []byte) ([]byte, int, error) { return InsertPDF(b, other, 1, true) }},
	} {
		src := unwalkableFilesPDF(false)
		if got, want := files(src), "a.txt=A-BYTES b.txt=B-BYTES"; got != want {
			t.Fatalf("setup: the fixture's files read as %q, want %q", got, want)
		}
		out, dropped, err := tc.drive(src)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if got, want := files(out), "a.txt=A-BYTES b.txt=B-BYTES"; got != want || dropped != 0 {
			t.Errorf("%s of a document with an entry the splice cannot walk: files %q, %d dropped — want %q and 0", tc.name, got, dropped, want)
		}

		// One file whose bytes cannot be read: the two that can be are kept and the third is COUNTED.
		out, dropped, err = tc.drive(unwalkableFilesPDF(true))
		if err != nil {
			t.Fatalf("%s with an unreadable file: %v", tc.name, err)
		}
		if got, want := files(out), "a.txt=A-BYTES b.txt=B-BYTES"; got != want || dropped != 1 {
			t.Errorf("%s with one unreadable file: files %q, %d dropped — want %q and 1", tc.name, got, dropped, want)
		}
	}
}
