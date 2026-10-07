package pdfops

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/pdfread"
)

// The accuracy harness's first half (ADR-088, `build/accuracy.sh`).
//
// For each PDF in a LOCAL list it writes two things into an output directory: the page maps of the pages worth
// measuring, and a copy of the document with its form fields taken out. A document that already has real fields is
// the best answer key there is — a field's own rectangle is where a field belongs — and the copy is the same form as
// someone without those fields would have it, which is what detection is run on.
//
// It runs only when both variables are set, and never in a suite: the list names files on one machine.

type accuracyDoc struct {
	Source   string    `json:"source"`
	Stripped string    `json:"stripped"`
	Pages    int       `json:"pages"`
	Maps     []PageMap `json:"maps"`
	// Words is the OCR engine's word boxes for a scan that has been OCR'd, where the corpus keeps them beside it:
	// `<name>.words.json` next to `<name>.pdf`. They are the only truth there is for where a scanned word's ink is,
	// and with them the harness scores search-redaction on the scan (test/accuracy/scanscore.mjs).
	Words string `json:"words,omitempty"`
	Error string `json:"error,omitempty"`
}

func TestAccuracyPrep(t *testing.T) {
	list, out := os.Getenv("NIB_ACCURACY_LIST"), os.Getenv("NIB_ACCURACY_OUT")
	if list == "" || out == "" {
		t.Skip("set NIB_ACCURACY_LIST and NIB_ACCURACY_OUT (build/accuracy.sh does)")
	}
	f, err := os.Open(list)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var docs []accuracyDoc
	sc := bufio.NewScanner(f)
	for i := 0; sc.Scan(); i++ {
		src := sc.Text()
		if src == "" || src[0] == '#' {
			continue
		}
		docs = append(docs, prepAccuracyDoc(src, filepath.Join(out, fmt.Sprintf("doc%02d.pdf", len(docs)))))
	}
	b, err := json.Marshal(docs)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "manifest.json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Logf("prepared %d documents in %s", len(docs), out)
}

// accuracyPagesPerDoc bounds what one document contributes, so a 58-page return does not outweigh a one-page form.
const accuracyPagesPerDoc = 3

func prepAccuracyDoc(src, dst string) (doc accuracyDoc) {
	doc = accuracyDoc{Source: src, Stripped: dst}
	if words := strings.TrimSuffix(src, filepath.Ext(src)) + ".words.json"; fileIsThere(words) {
		doc.Words = words
	}
	defer func() {
		if r := recover(); r != nil {
			doc.Error = fmt.Sprint(r)
		}
	}()
	data, err := os.ReadFile(src)
	if err != nil {
		doc.Error = err.Error()
		return doc
	}
	ctx, err := pdfread.Validated(data, model.NewDefaultConfiguration())
	if err != nil {
		doc.Error = err.Error()
		return doc
	}
	doc.Pages = ctx.PageCount
	// The pages with the most fields first; a document with none gives its first pages.
	type scored struct {
		m PageMap
		n int
	}
	var all []scored
	for p := 1; p <= ctx.PageCount && p <= 12; p++ {
		m, merr := MapPage(data, p)
		if merr != nil {
			continue
		}
		all = append(all, scored{m, len(m.Widgets)})
	}
	for len(doc.Maps) < accuracyPagesPerDoc && len(all) > 0 {
		best := 0
		for i := range all {
			if all[i].n > all[best].n {
				best = i
			}
		}
		doc.Maps = append(doc.Maps, all[best].m)
		all = append(all[:best], all[best+1:]...)
	}
	stripped, err := withoutFormFields(ctx)
	if err != nil {
		doc.Error = "strip: " + err.Error()
		return doc
	}
	// A document saved for signing carries its sign-here request (NibFlags) and opens LOCKED — editing off, so
	// neither Detect nor the search can be driven on it. The request is not the form: the copy goes without it,
	// as it goes without its fields, and the page under it is measured like any other.
	if stripped, err = ClearFlags(stripped); err != nil {
		doc.Error = "sign-here request: " + err.Error()
		return doc
	}
	if err := os.WriteFile(dst, stripped, 0o600); err != nil {
		doc.Error = err.Error()
	}
	return doc
}

func fileIsThere(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.Mode().IsRegular()
}

// withoutFormFields writes ctx with every widget annotation and the form itself removed.
func withoutFormFields(ctx *model.Context) ([]byte, error) {
	for p := 1; p <= ctx.PageCount; p++ {
		d, _, _, err := ctx.PageDict(p, false)
		if err != nil || d == nil {
			continue
		}
		annots, err := ctx.DereferenceArray(d["Annots"])
		if err != nil || annots == nil {
			continue
		}
		kept := types.Array{}
		for _, a := range annots {
			if ad := derefDict(ctx.XRefTable, a); ad != nil && nameVal(ad, "Subtype") == "Widget" {
				continue
			}
			kept = append(kept, a)
		}
		if len(kept) == 0 {
			d.Delete("Annots")
		} else {
			d["Annots"] = kept
		}
	}
	if root, err := ctx.Catalog(); err == nil && root != nil {
		root.Delete("AcroForm")
	}
	var buf bytes.Buffer
	if err := api.WriteContext(ctx, &buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
