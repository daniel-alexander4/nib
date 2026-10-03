package pdfops

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCheckPageOrderAgreesWithTheDigestOnTheCorpora — CheckPageOrder reads without pdfcpu's Optimize
// (for cost) and ContentDigest with it, so the claim that they apply one predicate to one tree is
// held here: on every corpus document that ContentDigest reads, the two give the same page-order
// verdict. /pending 755.
func TestCheckPageOrderAgreesWithTheDigestOnTheCorpora(t *testing.T) {
	home, _ := os.UserHomeDir()
	var files []string
	for _, root := range []string{filepath.Join(home, "nib", "producers"), filepath.Join(home, "nib", "verapdfs")} {
		_ = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() && strings.EqualFold(filepath.Ext(p), ".pdf") {
				files = append(files, p)
			}
			return nil
		})
	}
	if len(files) == 0 {
		t.Skip("no corpus under ~/nib/producers or ~/nib/verapdfs")
	}
	read := 0
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		_, derr := ContentDigest(b)
		if derr != nil && !errors.Is(derr, ErrPageTreeAmbiguous) {
			continue // the digest's read refused it; nothing to compare
		}
		read++
		if dig, chk := errors.Is(derr, ErrPageTreeAmbiguous), errors.Is(CheckPageOrder(b), ErrPageTreeAmbiguous); dig != chk {
			t.Errorf("%s: ContentDigest ambiguous=%v, CheckPageOrder ambiguous=%v", f, dig, chk)
		}
	}
	// A corpus present and NOTHING compared is a red, never a pass (`/pending 804`, R2): the agreement this test
	// holds is a claim about the documents it read, and a count of zero is no evidence about any of them.
	if read == 0 {
		t.Fatalf("%d corpus document(s) found and none compared — ContentDigest refused every one, so the agreement is unchecked", len(files))
	}
	t.Logf("%d corpus documents compared", read)
}
