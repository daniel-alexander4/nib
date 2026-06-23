package pdfops

import (
	"errors"
	"testing"
)

// TestEncrypt: protecting a plain PDF yields a copy that needs the password, and
// RemovePassword with that same password reverses it to a valid plain document —
// the round-trip the GUI and CLI both rely on (one password, used as user+owner).
func TestEncrypt(t *testing.T) {
	enc, err := Encrypt(threePagePDF(t), "secret123")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	// The output is genuinely protected: it doesn't validate without the password,
	// and a wrong password is rejected.
	if err := Validate(enc); err == nil {
		t.Fatal("encrypted PDF validated without a password — encryption did not take")
	}
	if _, err := RemovePassword(enc, "nope"); !errors.Is(err, ErrWrongPassword) {
		t.Fatalf("wrong password on encrypted PDF: err = %v, want ErrWrongPassword", err)
	}
	// The correct password reverses it to a plain, valid PDF.
	out, err := RemovePassword(enc, "secret123")
	if err != nil {
		t.Fatalf("RemovePassword on Encrypt output: %v", err)
	}
	if err := Validate(out); err != nil {
		t.Fatalf("round-tripped PDF does not validate: %v", err)
	}
}

// TestEncryptEmptyPassword: an empty password is rejected outright rather than
// producing an unprotected (or pdfcpu-error) file.
func TestEncryptEmptyPassword(t *testing.T) {
	if _, err := Encrypt(threePagePDF(t), ""); err == nil {
		t.Fatal("Encrypt(\"\") succeeded — want an error for an empty password")
	}
}

// TestEncryptAlreadyEncrypted: pdfcpu refuses to re-encrypt; Encrypt surfaces that
// as ErrAlreadyEncrypted so a batch (nib encrypt -w) can report it cleanly.
func TestEncryptAlreadyEncrypted(t *testing.T) {
	enc, err := Encrypt(threePagePDF(t), "secret123")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if _, err := Encrypt(enc, "other456"); !errors.Is(err, ErrAlreadyEncrypted) {
		t.Fatalf("re-encrypt: err = %v, want ErrAlreadyEncrypted", err)
	}
}
