package pdfops

import (
	"testing"

	"nib/internal/pdfread"
	"nib/internal/testpdf"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// withAttachments builds a one-page document carrying the named attachments, in order, then
// rewrites each entry's filespec /UF and /F to the string `rename` gives for its key (a key absent
// from `rename` keeps what AddAttachment wrote). The name-tree KEY is never touched.
//
// It exists because every shape below is one nib's own AddAttachment refuses to write — it keys
// the tree and names the filespec with one string — and every one of them is an ordinary PDF a
// counterparty can hand over.
func withAttachments(t *testing.T, files [][2]string, rename map[string]string) []byte {
	t.Helper()
	pdf, err := testpdf.Text("the lease, see the schedules")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if pdf, err = AddAttachment(pdf, f[0], []byte(f[1])); err != nil {
			t.Fatal(err)
		}
	}
	if len(rename) == 0 {
		return pdf
	}
	out, err := writeMutated(pdf, func(ctx *model.Context) error {
		if ctx.Names["EmbeddedFiles"] == nil {
			if err := ctx.LocateNameTree("EmbeddedFiles", false); err != nil {
				return err
			}
		}
		for key, name := range rename {
			v, ok := ctx.Names["EmbeddedFiles"].Value(key)
			if !ok {
				t.Fatalf("setup: no name-tree entry %q", key)
			}
			fs, err := ctx.DereferenceDict(v)
			if err != nil || fs == nil {
				t.Fatalf("setup: entry %q has no filespec (%v)", key, err)
			}
			fs["UF"] = types.StringLiteral(name)
			fs["F"] = types.StringLiteral(name)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func mustDigest(t *testing.T, pdf []byte) string {
	t.Helper()
	d, err := ContentDigest(pdf)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// TestEachEmbeddedFileIsHashedFromItsOwnEntry — /pending 725.
//
// The embedded-files axis (v3) enumerated the tree, took each entry's NAME, and fetched the bytes
// back through pdfcpu's ExtractAttachment — which resolves a name by tree key and then by the first
// filespec whose /UF, /F or /Desc matches. So the entry the bytes came from was chosen by a string
// the document's author writes, and three shapes followed:
//
//   - two entries whose /UF collide hash the FIRST entry's bytes twice, so the second is unbound;
//   - a /UF carrying a path is cleaned to a basename nothing resolves, and hashes a constant;
//   - the ceremony-record exclusion keyed on the filespec's name, so ANY entry calling itself
//     nib-ceremony.json dropped out of the digest altogether.
//
// Each is the Schedule-A swap of P07.S02 reopened, in the unsigned window where there is no
// signature to fall back on. Each subtest changes ONE entry's bytes and nothing else, and requires
// the digest to move.
func TestEachEmbeddedFileIsHashedFromItsOwnEntry(t *testing.T) {
	cases := []struct {
		name    string
		files   func(b string) [][2]string
		rename  map[string]string
		message string
	}{
		{
			name:   "two entries share a /UF",
			files:  func(b string) [][2]string { return [][2]string{{"a.txt", "rent is 1000/mo"}, {"b.txt", b}} },
			rename: map[string]string{"b.txt": "a.txt"},
			message: "b.txt's bytes changed and the digest did not — its /UF names a.txt, so the digest " +
				"fetched a.txt's bytes for both entries and b.txt is covered by nothing",
		},
		{
			name:   "a /UF carrying a path",
			files:  func(b string) [][2]string { return [][2]string{{"k1", b}} },
			rename: map[string]string{"k1": "schedules/Schedule-A.txt"},
			message: "an entry whose /UF carries a path changed its bytes and the digest did not — the " +
				"name cleans to a basename nothing resolves, so a constant marker was hashed instead",
		},
		{
			name:   "an ordinary entry calling itself the ceremony record",
			files:  func(b string) [][2]string { return [][2]string{{"Schedule-A.txt", b}} },
			rename: map[string]string{"Schedule-A.txt": CeremonyRecordName},
			message: "an entry that is NOT nib's record but names itself " + CeremonyRecordName +
				" changed its bytes and the digest did not — the exclusion keyed on a name the " +
				"attacker writes",
		},
		{
			name:   "the record's key with another file's name",
			files:  func(b string) [][2]string { return [][2]string{{CeremonyRecordName, b}} },
			rename: map[string]string{CeremonyRecordName: "Schedule-A.txt"},
			message: "an entry keyed " + CeremonyRecordName + " but NOT in the shape nib writes " +
				"changed its bytes and the digest did not — the exclusion must require nib's own shape",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			one := withAttachments(t, c.files("rent is 1000/mo"), c.rename)
			two := withAttachments(t, c.files("rent is 100000/mo"), c.rename)
			if mustDigest(t, one) == mustDigest(t, two) {
				t.Error(c.message)
			}
		})
	}
}

// TestTheRecordExclusionIsNibsShapeAndOnlyOnce — the exclusion's other half, which a PDF built
// through pdfcpu cannot reach: pdfcpu refuses a duplicate key on Add, but its reader keeps every
// entry of a malformed tree (`Node.AppendToNames`), and Extract resolves the record by the FIRST.
// With two entries in the record's shape, excluding either leaves the other unbound, and excluding
// both is a hiding place; so neither is excluded, and the document's DocHash cannot match.
func TestTheRecordExclusionIsNibsShapeAndOnlyOnce(t *testing.T) {
	base, err := testpdf.Text("any document; only its xref table is used, to decode strings")
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := pdfread.ReadOptimized(base, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	xt := ctx.XRefTable
	fs := func(f, uf string) types.Object {
		return types.Dict{"F": types.StringLiteral(f), "UF": types.StringLiteral(uf)}
	}
	r := CeremonyRecordName
	cases := []struct {
		name    string
		entries []embeddedEntry
		want    int
	}{
		{"nib's own record", []embeddedEntry{{"a.txt", fs("a.txt", "a.txt")}, {r, fs(r, r)}}, 1},
		{"no record", []embeddedEntry{{"a.txt", fs("a.txt", "a.txt")}}, -1},
		{"the name without the key", []embeddedEntry{{"x", fs(r, r)}}, -1},
		{"the key with another /UF", []embeddedEntry{{r, fs(r, "a.txt")}}, -1},
		{"the key with another /F", []embeddedEntry{{r, fs("a.txt", r)}}, -1},
		{"the key with no /F", []embeddedEntry{{r, types.Dict{"UF": types.StringLiteral(r)}}}, -1},
		{"two in the record's shape", []embeddedEntry{{r, fs(r, r)}, {r, fs(r, r)}}, -1},
		{"not a filespec at all", []embeddedEntry{{r, types.Integer(7)}}, -1},
	}
	for _, c := range cases {
		got, twice := ceremonyRecordEntry(xt, c.entries)
		if got != c.want {
			t.Errorf("%s: excluded entry %d, want %d", c.name, got, c.want)
		}
		// /pending 745: the duplicate is REPORTED, so CeremonyRecord can refuse it by name
		// rather than reading it as "no record".
		if want := c.name == "two in the record's shape"; twice != want {
			t.Errorf("%s: reported the record's key twice = %v, want %v", c.name, twice, want)
		}
	}
}
