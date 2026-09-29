package pdfops

import (
	"errors"
	"io"
	"sort"
	"strings"
	"testing"

	"nib/internal/pdfread"
	"nib/internal/testpdf"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// shownFiles is what a reader shows of a document's embedded-files tree, entry by entry: each
// entry's key, the name its filespec gives (/UF, else /F) and its bytes, sorted. It is read through
// pdfcpu's own whole-tree enumeration — `ExtractAttachments(nil)` walks the tree and reads each
// value's own filespec — and not through this package, which is what is under test.
func shownFiles(t *testing.T, pdf []byte) []string {
	t.Helper()
	ctx, err := pdfread.ReadOptimized(pdf, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	if ctx.Names["EmbeddedFiles"] == nil {
		return nil
	}
	aa, err := ctx.ExtractAttachments(nil)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, a := range aa {
		b, _ := io.ReadAll(a.Reader)
		out = append(out, "key="+a.ID+" shown="+a.FileName+" bytes="+string(b))
	}
	sort.Strings(out)
	return out
}

// The two shapes /pending 745 is about. Each is an ordinary PDF pdfcpu will not write.
var collidingNames = []struct {
	name    string
	entries []testpdf.Embedded
}{
	{"two entries swap their names", []testpdf.Embedded{
		{Key: "a.txt", F: "b.txt", UF: "b.txt", Data: "the bytes of the entry keyed a.txt"},
		{Key: "b.txt", F: "a.txt", UF: "a.txt", Data: "the bytes of the entry keyed b.txt"},
	}},
	{"two entries share a /UF", []testpdf.Embedded{
		{Key: "a.txt", F: "a.txt", UF: "a.txt", Data: "rent is 1000/mo"},
		{Key: "b.txt", F: "a.txt", UF: "a.txt", Data: "rent is 100000/mo"},
	}},
}

// TestAPageOperationCarriesEachFileUnderItsOwnName — /pending 745 (b).
//
// CarryAttachments listed the files by displayed name and fetched each back by that name, which
// pdfcpu resolves by tree key first. Two entries whose names swap came out of a page operation with
// each file's bytes under the other's name, and two sharing a /UF lost one. A reader of the output
// must see exactly what a reader of the input saw.
func TestAPageOperationCarriesEachFileUnderItsOwnName(t *testing.T) {
	for _, c := range collidingNames {
		t.Run(c.name, func(t *testing.T) {
			src := testpdf.WithEmbedded(c.entries...)
			dst, err := Collect(src, []string{"1"})
			if err != nil {
				t.Fatal(err)
			}
			out, dropped, err := CarryAttachments(src, dst)
			if err != nil {
				t.Fatal(err)
			}
			want, got := shownFiles(t, src), shownFiles(t, out)
			if strings.Join(got, "\n") != strings.Join(want, "\n") || dropped != 0 {
				t.Errorf("a page operation changed what the embedded files ARE (dropped %d):\n"+
					"before:\n  %s\nafter:\n  %s", dropped, strings.Join(want, "\n  "), strings.Join(got, "\n  "))
			}
		})
	}
}

// TestEachListedAttachmentReadsItsOwnBytes — /pending 745 (c, d): the server's download and the
// CLI's --extract both read through ReadAttachment. Every row the listing shows must read back
// THAT entry's bytes, addressed by the row's id, and name itself as the row did.
func TestEachListedAttachmentReadsItsOwnBytes(t *testing.T) {
	for _, c := range collidingNames {
		t.Run(c.name, func(t *testing.T) {
			pdf := testpdf.WithEmbedded(c.entries...)
			rows, err := Attachments(pdf)
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) != len(c.entries) {
				t.Fatalf("listed %d attachments, want %d: %+v", len(rows), len(c.entries), rows)
			}
			for _, e := range c.entries {
				var row *AttachmentInfo
				for i := range rows {
					if rows[i].ID == e.Key {
						row = &rows[i]
					}
				}
				if row == nil {
					t.Fatalf("no listed attachment has the id %q: %+v", e.Key, rows)
				}
				if row.Name != e.UF {
					t.Errorf("entry %q is listed as %q; a reader shows %q", e.Key, row.Name, e.UF)
				}
				info, data, err := ReadAttachment(pdf, row.ID)
				if err != nil {
					t.Fatalf("reading the row listed as id %q: %v", row.ID, err)
				}
				if string(data) != e.Data {
					t.Errorf("the row with id %q (shown as %q) read %q — another entry's bytes; want %q",
						row.ID, row.Name, data, e.Data)
				}
				if info.Name != row.Name {
					t.Errorf("the row shown as %q named its download %q", row.Name, info.Name)
				}
			}
		})
	}
}

// TestANameTwoFilesShowIsRefused: addressed by a displayed name more than one entry carries, the
// read refuses rather than choosing the first — and each is still reachable by its id.
func TestANameTwoFilesShowIsRefused(t *testing.T) {
	pdf := testpdf.WithEmbedded(
		testpdf.Embedded{Key: "k1", F: "a.txt", UF: "a.txt", Data: "one"},
		testpdf.Embedded{Key: "k2", F: "a.txt", UF: "a.txt", Data: "two"},
	)
	if _, data, err := ReadAttachment(pdf, "a.txt"); err == nil {
		t.Errorf("a name two entries show resolved to one of them (%q) instead of refusing", data)
	}
	if _, data, err := ReadAttachment(pdf, "k2"); err != nil || string(data) != "two" {
		t.Errorf("by id k2: %q, %v — want %q", data, err, "two")
	}
}

// TestTheRecordIsTheEntryTheDigestExcludes — /pending 745 (a). `CeremonyRecord` and the listing's
// `Ceremony` label answer "which entry is the record" with ceremonyRecordEntry, the digest's rule.
func TestTheRecordIsTheEntryTheDigestExcludes(t *testing.T) {
	r := CeremonyRecordName
	cases := []struct {
		name    string
		entries []testpdf.Embedded
		want    string // the record's bytes, or "" for none
		wantErr error
	}{
		{"nib's shape", []testpdf.Embedded{{Key: r, F: r, UF: r, Data: "the record"}}, "the record", nil},
		{"an entry that only CALLS itself the record", []testpdf.Embedded{{Key: "x", F: r, UF: r, Data: "a lookalike"}}, "", nil},
		{"the record's key twice", []testpdf.Embedded{
			{Key: r, F: r, UF: r, Data: "one"},
			{Key: r, F: r, UF: r, Data: "two"},
		}, "", ErrTwoCeremonyRecords},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pdf := testpdf.WithEmbedded(c.entries...)
			got, err := CeremonyRecord(pdf)
			if !errors.Is(err, c.wantErr) || (c.wantErr == nil && err != nil) {
				t.Fatalf("CeremonyRecord: err %v, want %v", err, c.wantErr)
			}
			if string(got) != c.want {
				t.Errorf("CeremonyRecord read %q, want %q — not the entry ContentDigest excludes", got, c.want)
			}
			rows, err := Attachments(pdf)
			if err != nil {
				t.Fatal(err)
			}
			for _, row := range rows {
				if want := c.want != ""; row.Ceremony != want {
					t.Errorf("the entry with id %q is labelled the ceremony record = %v, want %v",
						row.ID, row.Ceremony, want)
				}
			}
		})
	}
}
