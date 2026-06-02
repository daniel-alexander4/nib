package sign

import (
	"testing"

	"nib/internal/testpdf"
)

func TestVerifyUnsigned(t *testing.T) {
	data, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	got := Verify(data)
	if got.State != Unsigned {
		t.Errorf("Verify(unsigned form) state = %q, want %q", got.State, Unsigned)
	}
}

// A non-PDF must not panic or error out — it's simply "unsigned".
func TestVerifyGarbage(t *testing.T) {
	got := Verify([]byte("this is not a pdf"))
	if got.State != Unsigned {
		t.Errorf("Verify(garbage) state = %q, want %q", got.State, Unsigned)
	}
}
