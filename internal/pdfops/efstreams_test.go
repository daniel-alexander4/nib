package pdfops

import (
	"sort"
	"strings"
	"testing"

	"nib/internal/pdfread"
	"nib/internal/testpdf"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// efStreams is each /EF stream of every embedded-files entry, as "key /K=bytes", sorted — read with
// pdfcpu's own dereference and decode, not this package's.
func efStreams(t *testing.T, pdf []byte) []string {
	t.Helper()
	ctx, err := pdfread.ReadOptimized(pdf, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	tree := ctx.Names["EmbeddedFiles"]
	if tree == nil {
		return nil
	}
	var out []string
	if err := tree.Process(ctx.XRefTable, func(xt *model.XRefTable, key string, v *types.Object) error {
		fs, err := xt.DereferenceDict(*v)
		if err != nil {
			return err
		}
		ef, err := xt.DereferenceDict(fs["EF"])
		if err != nil {
			return err
		}
		for _, k := range []string{"F", "UF"} {
			o, ok := ef.Find(k)
			if !ok {
				out = append(out, key+" /"+k+" absent")
				continue
			}
			sd, _, err := xt.DereferenceStreamDict(o)
			if err != nil || sd == nil {
				t.Fatalf("%s /EF /%s: %v", key, k, err)
			}
			if sd.FilterPipeline != nil {
				if err := sd.Decode(); err != nil {
					t.Fatal(err)
				}
			} else {
				sd.Content = sd.Raw
			}
			out = append(out, key+" /"+k+"="+string(sd.Content))
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	sort.Strings(out)
	return out
}

// TestAPageOperationCarriesBothEmbeddedStreams — /pending 750 (3). /EF may hold a /F stream and a
// /UF stream that differ; a reader picks one by its own rule. The carry re-added /F's bytes under
// both keys, so a page operation silently deleted the /UF file.
func TestAPageOperationCarriesBothEmbeddedStreams(t *testing.T) {
	src := testpdf.WithEmbedded(
		testpdf.Embedded{Key: "a.txt", F: "a.txt", UF: "a.txt", Data: "the /F stream", UFData: "the /UF stream"},
		testpdf.Embedded{Key: "b.txt", F: "b.txt", UF: "b.txt", Data: "one stream under both"},
	)
	want := efStreams(t, src)
	if strings.Join(want, "\n") != "a.txt /F=the /F stream\na.txt /UF=the /UF stream\n"+
		"b.txt /F=one stream under both\nb.txt /UF=one stream under both" {
		t.Fatalf("setup: the fixture does not have the shape under test:\n  %s", strings.Join(want, "\n  "))
	}
	dst, err := Collect(src, []string{"1"})
	if err != nil {
		t.Fatal(err)
	}
	out, dropped, err := CarryAttachments(src, dst)
	if err != nil || dropped != 0 {
		t.Fatalf("carry: dropped %d, %v", dropped, err)
	}
	if got := efStreams(t, out); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("a page operation changed an entry's embedded streams:\nbefore:\n  %s\nafter:\n  %s",
			strings.Join(want, "\n  "), strings.Join(got, "\n  "))
	}
}
