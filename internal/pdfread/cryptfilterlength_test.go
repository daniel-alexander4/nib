package pdfread_test

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

	"nib/internal/pdfread"
	"nib/internal/testpdf"
)

// withoutCryptFilterLength is an AES-128 document (V 4, R 4, empty user password) whose one crypt filter
// carries no /Length — the shape of /pending 892's real-estate disclosure. pdfcpu writes `/Length 128` in the
// filter; it is blanked to spaces of the same width, so every offset in the file stays where the xref says.
func withoutCryptFilterLength(t *testing.T, header string) []byte {
	t.Helper()
	base := testpdf.Assemble(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Contents 4 0 R >>",
		4: "<< /Length 15 >>\nstream\n0 0 m 90 90 l S\nendstream",
	})
	conf := model.NewAESConfiguration("", "owner", 128)
	conf.Cmd = model.ENCRYPT
	var enc bytes.Buffer
	if err := api.Encrypt(bytes.NewReader(base), &enc, conf); err != nil {
		t.Fatalf("setup: %v", err)
	}
	b := enc.Bytes()
	cf := regexp.MustCompile(`/CF\s*<<\s*/StdCF\s*<<[^>]*>>`)
	loc := cf.FindIndex(b)
	if loc == nil {
		t.Fatalf("setup: no /StdCF crypt filter written in the clear — this fixture cannot be built (%q)", b[:200])
	}
	filter := b[loc[0]:loc[1]]
	length := regexp.MustCompile(`/Length\s+[0-9]+`).FindIndex(filter)
	if length == nil {
		t.Fatalf("setup: the crypt filter %q carries no /Length to take out", filter)
	}
	copy(filter[length[0]:length[1]], strings.Repeat(" ", length[1]-length[0]))
	if !bytes.HasPrefix(b, []byte("%PDF-1.")) {
		t.Fatalf("setup: header %q", b[:8])
	}
	copy(b, header)
	return b
}

// TestACryptFilterWithNoLengthIsReadOutsidePDF20 holds ADR-173's one-line patch to pdfcpu v0.13.0: ISO 32000-1
// Table 25 makes a crypt filter's /Length optional, and v0.13.0 required it everywhere EXCEPT PDF 2.0. Both arms,
// as upstream's fix has both: a 1.x document reads, and a PDF 2.0 document without the key is still refused.
func TestACryptFilterWithNoLengthIsReadOutsidePDF20(t *testing.T) {
	ctx, err := pdfread.Validated(withoutCryptFilterLength(t, "%PDF-1.7"), model.NewDefaultConfiguration())
	if err != nil {
		t.Errorf("a PDF 1.7 AES-128 document whose crypt filter has no /Length was refused: %v", err)
	} else if ctx.PageCount != 1 || ctx.E == nil {
		t.Errorf("read %d pages, encrypted %v; want 1 page, encrypted", ctx.PageCount, ctx.E != nil)
	}
	if _, err := pdfread.Validated(withoutCryptFilterLength(t, "%PDF-2.0"), model.NewDefaultConfiguration()); err == nil ||
		!strings.Contains(err.Error(), `crypt filter missing entry "Length"`) {
		t.Errorf("a PDF 2.0 crypt filter with no /Length answered %v, want upstream's refusal", err)
	}
}
