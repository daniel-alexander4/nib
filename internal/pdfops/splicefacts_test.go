package pdfops

import (
	"context"
	"sort"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/testpdf"
)

// documentFactsOf reads what `/pending 602` is about: the embedded files' names, the `/Info` title, the
// trailer's permanent `/ID[0]` and the flags.
func documentFactsOf(t *testing.T, pdf []byte) (files []string, title, id string, flags []byte) {
	t.Helper()
	ctx := readCtx(t, pdf)
	att, err := Attachments(pdf)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range att {
		files = append(files, a.Name)
	}
	sort.Strings(files)
	if ctx.Info != nil {
		if d := derefDict(ctx.XRefTable, *ctx.Info); d != nil {
			title, _ = stringVal(ctx.XRefTable, d["Title"])
		}
	}
	if len(ctx.ID) > 0 {
		id = ctx.ID[0].String()
	}
	if flags, err = FlagsJSON(pdf); err != nil {
		t.Fatal(err)
	}
	return files, title, id, flags
}

// spliceHost is a three-page document carrying everything a splice used to lose.
func spliceHost(t *testing.T) []byte {
	t.Helper()
	host, err := testpdf.Text("alpha", "beta", "gamma")
	if err != nil {
		t.Fatal(err)
	}
	if host, err = AddAttachment(host, "record.txt", []byte("the record")); err != nil {
		t.Fatal(err)
	}
	if host, err = SetTitle(host, "Lease"); err != nil {
		t.Fatal(err)
	}
	if host, err = SetFlags(host, []byte(`[{"page":3,"frac":{"x":0.5,"y":0.5},"type":"sign"}]`)); err != nil {
		t.Fatal(err)
	}
	return host
}

// TestAnInsertOrASplitKeepsTheDocumentsOwnFacts — `/pending 602`. `splice` sets its page order with the
// page selection, which reduces a document to what an extract may carry; an insert and a split are the
// same document with a page added or cut, and lost its attachments, its title and its `/ID` to that
// (measured before the fix: 0 attachments, no title, a new `/ID[0]`, from both doors).
//
// The flags are asserted GONE, which is the decision `splicefacts.go` records: a flag names a page by
// number, and this one is on page 3 of a document about to gain a page before it.
func TestAnInsertOrASplitKeepsTheDocumentsOwnFacts(t *testing.T) {
	host := spliceHost(t)
	_, wantTitle, wantID, hostFlags := documentFactsOf(t, host)
	if wantTitle == "" || wantID == "" || len(hostFlags) == 0 {
		t.Fatalf("the fixture carries title %q, id %q, flags %q; it must carry all three", wantTitle, wantID, hostFlags)
	}
	other, err := testpdf.Text("inserted")
	if err != nil {
		t.Fatal(err)
	}
	if other, err = AddAttachment(other, "theirs.txt", []byte("theirs")); err != nil {
		t.Fatal(err)
	}
	inserted, err := withoutCount(InsertPDF(host, other, 2, true))
	if err != nil {
		t.Fatal(err)
	}
	split, _, err := SplitPage(host, 2, 2, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name  string
		out   []byte
		files string
	}{
		// The inserted document's file comes too, as it does through `Append`.
		{"InsertPDF", inserted, "record.txt theirs.txt"},
		{"SplitPage", split, "record.txt"},
	} {
		files, title, id, flags := documentFactsOf(t, c.out)
		if got := strings.Join(files, " "); got != c.files {
			t.Errorf("%s: embedded files %q, want %q", c.name, got, c.files)
		}
		if title != wantTitle {
			t.Errorf("%s: /Info title %q, want %q", c.name, title, wantTitle)
		}
		if id != wantID {
			t.Errorf("%s: /ID[0] %s, want the document's own %s", c.name, id, wantID)
		}
		if len(flags) != 0 {
			t.Errorf("%s: flags %q carried with page numbers the splice has changed", c.name, flags)
		}
	}
	// The record's bytes, not only its name.
	if _, data, err := ReadAttachment(split, "record.txt"); err != nil || string(data) != "the record" {
		t.Errorf("the split's record.txt reads %q (%v), want the record", data, err)
	}
}

// TestASpliceWritesNoPageThroughAFileThatReachesIt — pdfcpu writes by reachability, so an embedded
// file whose specification references the page a split replaces would write that page out again. The
// name tree is then not put back; the file itself is re-added from its bytes (`filesFromBytes`,
// `/pending 870` — until then the split returned no files and said nothing), and the `/Info` entry that
// reaches the page is not carried at all.
func TestASpliceWritesNoPageThroughAFileThatReachesIt(t *testing.T) {
	host, err := writeMutated(spliceHost(t), func(ctx *model.Context) error {
		pref, perr := ctx.PageDictIndRef(context.Background(), 2)
		if perr != nil {
			return perr
		}
		// And an `/Info` entry that does the same, so each guard is the only thing between its own
		// reference and the page.
		derefDict(ctx.XRefTable, *ctx.Info)["NibProbe"] = types.Array{*pref}
		return ctx.Names["EmbeddedFiles"].Process(context.Background(), ctx.XRefTable, func(xt *model.XRefTable, _ string, v *types.Object) error {
			fs := derefDict(xt, *v)
			fs["NibProbe"] = *pref
			return nil
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	if files, _, _, _ := documentFactsOf(t, host); len(files) != 1 {
		t.Fatalf("the fixture carries %d files, want 1", len(files))
	}
	split, _, err := SplitPage(host, 2, 2, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	ctx := readCtx(t, split)
	if got := pageObjectCount(t, split); got != ctx.PageCount {
		t.Errorf("the output holds %d page objects for %d pages: the replaced page was written through the file that names it", got, ctx.PageCount)
	}
	if files, _, _, _ := documentFactsOf(t, split); strings.Join(files, " ") != "record.txt" {
		t.Errorf("files %v, want record.txt re-added from its bytes", files)
	}
	if e, err := treeEntries(ctx.XRefTable); err != nil || len(e) != 1 || derefDict(ctx.XRefTable, e[0].fs)["NibProbe"] != nil {
		t.Errorf("the carried file's specification still holds its reference to the replaced page (%d entries, %v)", len(e), err)
	}
	if derefDict(ctx.XRefTable, *ctx.Info)["NibProbe"] != nil {
		t.Error("the /Info entry reaching the replaced page was carried")
	}
}
