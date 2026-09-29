package sign

import (
	"bytes"
	"errors"
	"io"
	"testing"

	dpdf "github.com/digitorus/pdf"
	psign "github.com/digitorus/pdfsign/sign"

	"nib/internal/testpdf"
)

// withLibrary runs fn with the signing library replaced by lib, restoring it after.
func withLibrary(t *testing.T, lib func(in io.ReadSeeker, w io.Writer, r *dpdf.Reader, n int64, d psign.SignData) error, fn func()) {
	t.Helper()
	saved := librarySign
	librarySign = lib
	defer func() { librarySign = saved }()
	fn()
}

// TestASignatureNibCannotReadBackIsRefused is /pending 747's own case: `runSign` hands back a signed
// document only when nib reads it as the input plus exactly the signature it asked for. Each case is
// an output the library returned WITHOUT an error, which was handed to the user as a success.
func TestASignatureNibCannotReadBackIsRefused(t *testing.T) {
	a, b := newIdentity(t, "Alice"), newIdentity(t, "Bob")
	base, err := testpdf.Text("the lease")
	if err != nil {
		t.Fatal(err)
	}
	signs := map[string]func([]byte) ([]byte, error){
		"Sign":         func(d []byte) ([]byte, error) { return Sign(d, b.certPEM, b.keyPEM, Options{Name: "B"}) },
		"SignApproval": func(d []byte) ([]byte, error) { return SignApproval(d, b.certPEM, b.keyPEM, Options{Name: "B"}) },
	}

	// The divergence found in /pending 740: no hybrid form, and the library's co-signature over this
	// hand-built signed file is one nib's revision sweep cannot read — `Verify` says Invalid, 0 signers.
	t.Run("co-signing the synthetic signed fixture", func(t *testing.T) {
		in := synthSigned(t, a)
		for name, fn := range signs {
			var raw []byte
			withLibrary(t, func(i io.ReadSeeker, w io.Writer, r *dpdf.Reader, n int64, d psign.SignData) error {
				var buf bytes.Buffer
				err := psign.Sign(i, &buf, r, n, d)
				raw = buf.Bytes()
				_, _ = w.Write(raw)
				return err
			}, func() {
				out, err := fn(in)
				// STIMULUS: the library itself reported success and wrote a file nib reads as broken.
				if st := Verify(raw); raw == nil || st.State == Valid {
					t.Fatalf("STIMULUS %s: the library's raw output verifies %q — the divergence is gone, pick another fixture", name, st.State)
				}
				if !errors.Is(err, ErrSignedOutputUnreadable) || out != nil {
					t.Errorf("%s over the synthetic signed fixture: err=%v, %d bytes returned — want ErrSignedOutputUnreadable and nothing (/pending 747)", name, err, len(out))
				}
			})
		}
	})

	bad := []struct {
		name string
		lib  func(i io.ReadSeeker, w io.Writer, r *dpdf.Reader, n int64, d psign.SignData) error
	}{
		{"the input copied back unchanged", func(i io.ReadSeeker, w io.Writer, _ *dpdf.Reader, _ int64, _ psign.SignData) error {
			_, err := io.Copy(w, i)
			return err
		}},
		{"bytes that are not a PDF", func(_ io.ReadSeeker, w io.Writer, _ *dpdf.Reader, _ int64, _ psign.SignData) error {
			_, err := w.Write([]byte("%PDF-1.7\nnot a document\n"))
			return err
		}},
		{"an honest signature with bytes appended past it", func(i io.ReadSeeker, w io.Writer, r *dpdf.Reader, n int64, d psign.SignData) error {
			if err := psign.Sign(i, w, r, n, d); err != nil {
				return err
			}
			_, err := w.Write([]byte("\n% appended\n"))
			return err
		}},
		{"a signature made with a different certificate", func(i io.ReadSeeker, w io.Writer, r *dpdf.Reader, n int64, d psign.SignData) error {
			d.Certificate, d.Signer = a.cert, a.signer
			return psign.Sign(i, w, r, n, d)
		}},
		{"a signature whose covered bytes were changed after signing", func(i io.ReadSeeker, w io.Writer, r *dpdf.Reader, n int64, d psign.SignData) error {
			var buf bytes.Buffer
			if err := psign.Sign(i, &buf, r, n, d); err != nil {
				return err
			}
			out := buf.Bytes()
			// Flip a byte of the header's binary-marker comment: inside the new signature's first
			// range, and nothing any parser reads.
			k := bytes.IndexByte(out, '\n') + 2
			if out[k-1] != '%' {
				return errors.New("STIMULUS: no binary-marker comment after the header")
			}
			out[k] ^= 0x01
			_, err := w.Write(out)
			return err
		}},
		{"two signatures written at once", func(i io.ReadSeeker, w io.Writer, r *dpdf.Reader, n int64, d psign.SignData) error {
			var first bytes.Buffer
			if err := psign.Sign(i, &first, r, n, d); err != nil {
				return err
			}
			fb := first.Bytes()
			r2, err := dpdf.NewReader(bytes.NewReader(fb), int64(len(fb)))
			if err != nil {
				return err
			}
			d.Signature.CertType = psign.ApprovalSignature
			d.Signature.DocMDPPerm = 0
			return psign.Sign(bytes.NewReader(fb), w, r2, int64(len(fb)), d)
		}},
	}
	for _, c := range bad {
		t.Run(c.name, func(t *testing.T) {
			withLibrary(t, c.lib, func() {
				out, err := SignApproval(base, b.certPEM, b.keyPEM, Options{Name: "B"})
				if !errors.Is(err, ErrSignedOutputUnreadable) || out != nil {
					t.Errorf("err=%v, %d bytes returned — want ErrSignedOutputUnreadable and nothing (/pending 747)", err, len(out))
				}
			})
		})
	}

	// Control: the honest library, over an unsigned and an already-signed input, still signs.
	t.Run("control: honest signatures pass", func(t *testing.T) {
		once, err := SignApproval(base, a.certPEM, a.keyPEM, Options{Name: "A"})
		if err != nil {
			t.Fatalf("honest first signature refused: %v", err)
		}
		for name, fn := range signs {
			out, err := fn(once)
			if err != nil {
				t.Fatalf("%s over a nib-signed document refused: %v", name, err)
			}
			if st := Verify(out); st.State != Valid || len(st.Signers) != 2 {
				t.Errorf("%s: state=%s signers=%d, want valid/2", name, st.State, len(st.Signers))
			}
		}
	})
}
