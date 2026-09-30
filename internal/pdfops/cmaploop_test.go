package pdfops

import (
	"bytes"
	"errors"
	"testing"

	"nib/internal/pdfread"
	"nib/internal/testpdf"
)

// TestAUseCMapLoopIsRefusedAtEveryDoor — `/pending 675`. Each of these handed the bytes to pdfcpu's validator,
// which recursed on the loop until the process died (~5 s; a Go stack overflow is not recoverable): FlagsJSON
// is what the server runs answering an Open, so opening the file killed nib. **Before the fix this test does
// not fail, it kills the test binary.** Now each refuses with the loop named.
func TestAUseCMapLoopIsRefusedAtEveryDoor(t *testing.T) {
	doors := map[string]func([]byte) error{
		"FlagsJSON (the server's Open)":  func(b []byte) error { _, err := FlagsJSON(b); return err },
		"PageCount (validated door)":     func(b []byte) error { _, err := PageCount(b); return err },
		"Validate (ReadOptimized door)":  func(b []byte) error { return Validate(b) },
		"RemovePages (a page operation)": func(b []byte) error { _, err := RemovePages(b, []string{"1"}); return err },
		"mergeDocs (Validated door)":     func(b []byte) error { _, err := mergeDocs([][]byte{b, b}); return err },
	}
	for name, c := range testpdf.UseCMapLoops() {
		// Stimulus first: the loop's last hop is in the bytes (the raw chain is asserted in pdfread's own test).
		if len(c.Loop) < 2 || !bytes.Contains(c.PDF, []byte("/UseCMap 20 0 R")) {
			t.Fatalf("setup: %s carries no /UseCMap back to 20", name)
		}
		for door, f := range doors {
			if err := f(c.PDF); !errors.Is(err, pdfread.ErrReferenceCycle) {
				t.Errorf("%s over %s returned %v, want the /UseCMap loop refused", door, name, err)
			}
		}
	}
	// And the legitimate shape still reads at the busiest door.
	if _, err := FlagsJSON(testpdf.UseCMapChain()); err != nil {
		t.Errorf("FlagsJSON refused a /UseCMap chain with no loop: %v", err)
	}
}
