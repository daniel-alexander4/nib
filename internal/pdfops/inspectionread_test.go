package pdfops

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"nib/internal/pdfread"
	"nib/internal/testpdf"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// inspectionSites is every function that reads through `inspectionRead`, and how to ask it for its answer. A row
// is added when a site is switched, never before: the census below fails on a caller with no row and on a row with
// no caller.
var inspectionSites = map[string]func(pdf []byte) any{
	"inspectTags": func(pdf []byte) any { return inspectTags(pdf) },
	"Validate":    func(pdf []byte) any { return failed(Validate(pdf)) },
	"carryLang": func(pdf []byte) any {
		out, err := carryLang(pdf, carryTarget())
		return []any{stable(out), failed(err)}
	},
	"CarryAttachments": func(pdf []byte) any {
		out, n, err := CarryAttachments(pdf, carryTarget())
		return []any{stable(out), n, failed(err)}
	},
	"Attachments":     func(pdf []byte) any { l, err := Attachments(pdf); return []any{l, failed(err)} },
	"StructureSource": func(pdf []byte) any { s, ok := StructureSource(pdf); return []any{s, ok} },
	"carryIsComplete": func(pdf []byte) any { return carryIsComplete(pdf) },
	"ReadAttachment": func(pdf []byte) any {
		l, _ := Attachments(pdf)
		var out []any
		for _, a := range l {
			info, b, err := ReadAttachment(pdf, a.ID)
			out = append(out, info, b, failed(err))
		}
		return out
	},
	"CeremonyRecord":   func(pdf []byte) any { b, err := CeremonyRecord(pdf); return []any{b, failed(err)} },
	"SignatureWidgets": func(pdf []byte) any { w, err := SignatureWidgets(pdf); return []any{w, failed(err)} },
	"capturePageSources": func(pdf []byte) any {
		m, ok := capturePageSources(pdf)
		return []any{m, ok}
	},
	"watermarkMarkersBefore": func(pdf []byte) any {
		n, err := PageCount(pdf)
		if err != nil {
			return nil
		}
		byPage := map[int][]Word{}
		for p := 1; p <= n; p++ {
			byPage[p] = nil
		}
		m, err := watermarkMarkersBefore(pdf, byPage)
		return []any{m, failed(err)}
	},
	// Its read decides the blockers; the bytes it writes come from `StripActive` over the input, not from that read
	// (and carry fresh XMP identifiers each write), so whether it wrote is compared, not what.
	"PreparePDFA": func(pdf []byte) any {
		data, blockers, err := PreparePDFA(pdf)
		return []any{data != nil, blockers, failed(err)}
	},
	"readStructureView": func(pdf []byte) any { v, err := readStructureView(pdf); return []any{v, failed(err)} },
	"UnmarkedTextRuns":  func(pdf []byte) any { n, err := UnmarkedTextRuns(pdf); return []any{n, failed(err)} },
	"uncoveredDrawings": func(pdf []byte) any { n, err := uncoveredDrawings(pdf); return []any{n, failed(err)} },
	"ProposeTags":       func(pdf []byte) any { p, err := ProposeTags(pdf); return []any{p, failed(err)} },
	// Reviewed as proposed: every element kept in the role it was offered.
	"CommitTags": func(pdf []byte) any {
		p, err := ProposeTags(pdf)
		if err != nil || len(p.Elements) == 0 {
			return nil
		}
		reviewed := make([]TagReview, len(p.Elements))
		for i, el := range p.Elements {
			reviewed[i] = TagReview{ID: el.ID, Role: el.Role, Text: el.Text}
		}
		// Compared by the structure it committed, not its bytes: the write is not byte-stable across two runs of the
		// same reading (measured on nine producer files), and what the read decides is the proposal committed.
		out, err := CommitTags(pdf, reviewed)
		if err != nil {
			return true
		}
		v, verr := readStructureView(out)
		return []any{v, failed(verr)}
	},
	// The probe decides only whether there is a claim — returned untouched, or rewritten by the full read it has
	// always used — and the rewrite is not byte-stable across two runs of that read (measured on the veraPDF corpus).
	"dropUAIdentificationBytes": func(pdf []byte) any {
		out, had, err := dropUAIdentificationBytes(pdf)
		return []any{bytes.Equal(out, pdf), had, failed(err)}
	},
	"PageBox": func(pdf []byte) any {
		a, b, c, d, err := PageBox(pdf, 1)
		return []any{a, b, c, d, failed(err)}
	},
}

// fileID is the trailer's /ID and writeDate a date string: pdfcpu derives the /ID from the clock on every write
// and stamps an embedded file's `/Params /ModDate` with the time it wrote it, so two writes of one document a
// second apart differ in both (measured on 7.11-t01-pass-a).
var (
	fileID    = regexp.MustCompile(`/ID\s*\[\s*<[0-9A-Fa-f]*>\s*<[0-9A-Fa-f]*>\s*\]`)
	writeDate = regexp.MustCompile(`\(D:[0-9]{14}[^)]*\)`)
)

// stable is a written document with its /ID and dates blanked.
func stable(pdf []byte) []byte {
	return writeDate.ReplaceAll(fileID.ReplaceAll(pdf, []byte("/ID[]")), []byte("(D:)"))
}

// carryTarget is the document a carry writes into: one blank page, no catalog extras.
func carryTarget() []byte {
	return testpdf.Assemble(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] >>",
	})
}

// failed is whether err is set. Not its text: pdfcpu's message for a document it cannot read can differ run to run
// (fda-176439 answers "not enough characters after #" or "encoding/hex: invalid byte" by the order it meets them).
func failed(err error) bool { return err != nil }

// TestReadOnlySitesAgreeAcrossReadings — `/pending 763`: a site switched to `inspectionRead` gives, on every
// document, the answer it gave through the full pass (`pdfread.ReadOptimized` with the per-page resource step).
// The documents are the corpora under ~/nib (skipped where absent), a flat tagged document whose pages inherit nothing,
// and one whose pages inherit /Resources from their /Pages node.
//
// **What this cannot see: the door's inherited-/Resources restore** (the P01 phase-close review, measured). Every
// switched site resolves inheritance through `pdfread.Pages`' attributes rather than the page's own dictionary, so
// with the restore deleted all 20 sites still agree on all 335 documents. The restore's one reader is the carry gate,
// and it is held there: `TestTheCarryGateSeesAFormDrawnTwiceThroughInheritedResources` goes red without it.
func TestReadOnlySitesAgreeAcrossReadings(t *testing.T) {
	docs := map[string][]byte{"flat tagged": flatTaggedDoc(3), "inherited resources": inheritedResourcesDoc()}
	home, _ := os.UserHomeDir()
	for _, root := range []string{filepath.Join(home, "nib", "producers"), filepath.Join(home, "nib", "verapdfs")} {
		_ = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() && strings.EqualFold(filepath.Ext(p), ".pdf") {
				if b, rerr := os.ReadFile(p); rerr == nil {
					docs[p] = b
				}
			}
			return nil
		})
	}
	if len(docs) == 2 {
		t.Log("no corpus under ~/nib/producers or ~/nib/verapdfs: comparing the two fixtures only")
	}
	full := func(pdf []byte) (*model.Context, error) {
		return pdfread.ReadOptimized(pdf, model.NewDefaultConfiguration())
	}
	answers := func(read func([]byte) (*model.Context, error)) map[string]map[string]any {
		saved := inspectionRead
		inspectionRead = read
		defer func() { inspectionRead = saved }()
		out := map[string]map[string]any{}
		for site, ask := range inspectionSites {
			out[site] = map[string]any{}
			for name, pdf := range docs {
				out[site][name] = ask(pdf)
			}
		}
		return out
	}
	want, again, got := answers(full), answers(full), answers(pdfread.ReadForInspection)
	sites := make([]string, 0, len(inspectionSites))
	for s := range inspectionSites {
		sites = append(sites, s)
	}
	sort.Strings(sites)
	for _, site := range sites {
		differ := 0
		for name := range docs {
			if !reflect.DeepEqual(want[site][name], again[site][name]) {
				t.Errorf("%s on %s: the full pass gives two different answers — the comparison cannot see this document: "+
					"%v, then %v", site, filepath.Base(name), abbrev(want[site][name]), abbrev(again[site][name]))
				continue
			}
			if !reflect.DeepEqual(want[site][name], got[site][name]) {
				differ++
				if differ <= 3 {
					t.Errorf("%s on %s: through the full pass %v, through inspectionRead %v", site, filepath.Base(name),
						abbrev(want[site][name]), abbrev(got[site][name]))
				}
			}
		}
		t.Logf("%-18s %d documents, %d differ", site, len(docs), differ)
	}
}

func abbrev(v any) string {
	s := strings.Join(strings.Fields(fmt.Sprintf("%+v", v)), " ")
	if len(s) > 160 {
		s = s[:160] + "…"
	}
	return s
}

// inheritedResourcesDoc is two pages that draw a form they do not name themselves: the /XObject is on their
// /Pages node, so a reader of the page's own dictionary finds it only once inheritance is resolved.
func inheritedResourcesDoc() []byte {
	content := "/P <</MCID 0>> BDC /Fm0 Do EMC"
	return testpdf.Assemble(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /Lang (en) /MarkInfo << /Marked true >> >>",
		2: "<< /Type /Pages /Kids [3 0 R 5 0 R] /Count 2 /Resources << /XObject << /Fm0 6 0 R >> >> >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Contents 4 0 R >>",
		5: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Contents 4 0 R >>",
		4: "<< /Length " + strconv.Itoa(len(content)) + " >>\nstream\n" + content + "\nendstream",
		6: "<< /Type /XObject /Subtype /Form /BBox [0 0 10 10] /Length 15 >>\nstream\n0 0 10 10 re f\nendstream",
	})
}

// TestEveryInspectionReadCallerHasARow — the census: `inspectionRead` is called only from functions with a row in
// inspectionSites, every row names a caller, and nothing in this package calls `pdfread.ReadForInspection` except
// through the variable.
func TestEveryInspectionReadCallerHasARow(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi os.FileInfo) bool { return !strings.HasSuffix(fi.Name(), "_test.go") }, 0)
	if err != nil {
		t.Fatal(err)
	}
	callers := map[string]bool{}
	for _, pkg := range pkgs {
		for fname, f := range pkg.Files {
			for _, d := range f.Decls {
				fn, ok := d.(*ast.FuncDecl)
				if !ok || fn.Body == nil {
					continue
				}
				ast.Inspect(fn.Body, func(n ast.Node) bool {
					switch n := n.(type) {
					case *ast.Ident:
						if n.Name == "inspectionRead" {
							callers[fn.Name.Name] = true
						}
					case *ast.SelectorExpr:
						if x, ok := n.X.(*ast.Ident); ok && x.Name == "pdfread" && n.Sel.Name == "ReadForInspection" {
							t.Errorf("%s: %s calls pdfread.ReadForInspection directly — read through inspectionRead, so "+
								"TestReadOnlySitesAgreeAcrossReadings holds its answer", fname, fn.Name.Name)
						}
					}
					return true
				})
			}
		}
	}
	for c := range callers {
		if inspectionSites[c] == nil {
			t.Errorf("%s reads through inspectionRead and has no row in inspectionSites — add one, so its answer is "+
				"compared with the full pass's", c)
		}
	}
	for s := range inspectionSites {
		if !callers[s] {
			t.Errorf("inspectionSites has a row for %s, which does not read through inspectionRead", s)
		}
	}
}
