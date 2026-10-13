package sign

import (
	"fmt"
	"runtime"
	"strings"
	"testing"

	"nib/internal/testpdf"
)

// TestVerifyingASignatureAllocatesAboutWhatItCovers — /pending 735, ADR-142. `Verify` runs on every
// install, mutation and undo, and the signature library allocated about six times a signature's
// covered bytes to hash them once (measured 6.3, 6.1 and 7.1 times the file per signature at 1, 4
// and 16 MB): each /ByteRange pair read into a doubling buffer, then copied into another. The
// patched library (`third_party/digitorus-pdfsign/NOTICE.nib`) reads the pairs into one buffer of
// the right size — measured 1.3, 1.1 and 1.0.
//
// What is held is the cost of each signature AFTER the first, so pdfcpu's own read of the file,
// which is paid once whatever the count, is not in the figure. The ceiling is twice the file: a
// second copy per signature is the regression, and upstream's function is at six.
func TestVerifyingASignatureAllocatesAboutWhatItCovers(t *testing.T) {
	a := newIdentity(t, "Alice")
	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	pad := strings.Repeat("x", 4<<20)
	doc, err := testpdf.AppendRevision(base, map[int]string{900: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(pad), pad)})
	if err != nil {
		t.Fatal(err)
	}
	const signatures = 3
	var allocated [signatures]uint64
	for n := 0; n < signatures; n++ {
		if doc, err = SignApproval(doc, a.certPEM, a.keyPEM, Options{}); err != nil {
			t.Fatalf("signature %d: %v", n+1, err)
		}
		// Best of three: TotalAlloc counts every goroutine, and a best case is what the function
		// itself asks for.
		best := ^uint64(0)
		for r := 0; r < 3; r++ {
			runtime.GC()
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			st := Verify(doc)
			runtime.ReadMemStats(&after)
			if st.State != Valid || len(st.Signers) != n+1 {
				t.Fatalf("stimulus: %d signature(s) read state %q with %d signer(s)", n+1, st.State, len(st.Signers))
			}
			best = min(best, after.TotalAlloc-before.TotalAlloc)
		}
		allocated[n] = best
	}
	per := float64(allocated[signatures-1]-allocated[0]) / (signatures - 1)
	if ratio := per / float64(len(doc)); ratio > 2 {
		t.Errorf("each signature after the first cost Verify %.1f MB on a %.1f MB file — %.2f times the file, "+
			"and the patched library reads a signature's ranges once", per/(1<<20), float64(len(doc))/(1<<20), ratio)
	}
}
