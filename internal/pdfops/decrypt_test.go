package pdfops

import (
	"bytes"
	"errors"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// encryptPDF builds an encrypted copy of pdf with the given open (user) and owner
// passwords, via pdfcpu's own AES encryption — the fixture for the decrypt tests.
func encryptPDF(t *testing.T, pdf []byte, userPW, ownerPW string) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := api.Encrypt(bytes.NewReader(pdf), &out, model.NewAESConfiguration(userPW, ownerPW, 256)); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

// TestRemovePassword: the correct open password yields a plain, valid PDF that no
// longer needs any password (a second RemovePassword reports it's not encrypted).
func TestRemovePassword(t *testing.T) {
	enc := encryptPDF(t, threePagePDF(t), "open123", "owner456")
	out, err := RemovePassword(enc, "open123")
	if err != nil {
		t.Fatalf("RemovePassword with correct password: %v", err)
	}
	if err := Validate(out); err != nil {
		t.Fatalf("decrypted PDF does not validate: %v", err)
	}
	if _, err := RemovePassword(out, ""); !errors.Is(err, ErrNotEncrypted) {
		t.Fatalf("decrypted PDF still looks encrypted: err = %v, want ErrNotEncrypted", err)
	}
}

// TestRemovePasswordWrong: a wrong/missing password is reported distinctly so the
// UI can reprompt, and never silently "succeeds".
func TestRemovePasswordWrong(t *testing.T) {
	enc := encryptPDF(t, threePagePDF(t), "open123", "owner456")
	if _, err := RemovePassword(enc, "nope"); !errors.Is(err, ErrWrongPassword) {
		t.Fatalf("wrong password: err = %v, want ErrWrongPassword", err)
	}
	if _, err := RemovePassword(enc, ""); !errors.Is(err, ErrWrongPassword) {
		t.Fatalf("missing password: err = %v, want ErrWrongPassword", err)
	}
}

// TestRemovePasswordPlain: an unencrypted document reports ErrNotEncrypted rather
// than producing a bogus result.
func TestRemovePasswordPlain(t *testing.T) {
	if _, err := RemovePassword(threePagePDF(t), ""); !errors.Is(err, ErrNotEncrypted) {
		t.Fatalf("plain PDF: err = %v, want ErrNotEncrypted", err)
	}
}

// TestRemovePasswordOwnerOnly: an owner-password document (no open password) drops
// its restrictions with an empty password — the "remove restrictions" case.
func TestRemovePasswordOwnerOnly(t *testing.T) {
	enc := encryptPDF(t, threePagePDF(t), "", "owner456")
	out, err := RemovePassword(enc, "")
	if err != nil {
		t.Fatalf("owner-only RemovePassword(\"\"): %v", err)
	}
	if err := Validate(out); err != nil {
		t.Fatalf("decrypted owner-only PDF does not validate: %v", err)
	}
	if _, err := RemovePassword(out, ""); !errors.Is(err, ErrNotEncrypted) {
		t.Fatalf("owner-only PDF still encrypted after strip: err = %v", err)
	}
}
