package sign

import (
	"bytes"
	"errors"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// `/pending 889`: three real signed documents — a forms engine's output, AES-encrypted to forbid changes and
// opening with no password — read `Invalid` with no signer, nothing refused and NO cause, and `nib verify` and
// the window both said "modified since signing" of signatures nobody had checked. The signature reader does not
// decrypt AES: it reads nothing of such a file.

// encryptedForOwner is pdf encrypted with AES under an owner password only, so it opens with none.
func encryptedForOwner(t *testing.T, pdf []byte, bits int) []byte {
	t.Helper()
	conf := model.NewAESConfiguration("", "owner", bits)
	conf.Cmd = model.ENCRYPT
	var out bytes.Buffer
	if err := api.Encrypt(bytes.NewReader(pdf), &out, conf); err != nil {
		t.Fatalf("setup: could not encrypt the fixture: %v", err)
	}
	return out.Bytes()
}

func TestAnEncryptedSignedDocumentIsNotCalledModified(t *testing.T) {
	signed := signedFixture(t)
	if st := Verify(signed); st.State != Valid {
		t.Fatalf("setup: the fixture reads %q before it is encrypted, want valid", st.State)
	}
	// Both key lengths: the signature reader refuses each in different words.
	for _, bits := range []int{128, 256} {
		enc := encryptedForOwner(t, signed, bits)
		// Stimulus: the sweep does fail on it. Were the reader to learn AES, this test would be about nothing.
		if _, err := sweepRevisions(enc); err == nil {
			t.Fatalf("setup (%d bits): the sweep read an AES-encrypted file, so no verdict below is the one this test is about", bits)
		}
		st := Verify(enc)
		if st.State != Invalid {
			t.Errorf("%d bits: state = %q, want invalid: nothing was checked, and unchecked is never valid", bits, st.State)
		}
		if st.Unchecked != UncheckedEncrypted {
			t.Errorf("%d bits: unchecked = %q, want %q — with no cause every reader says \"modified since signing\" of a signature nobody checked", bits, st.Unchecked, UncheckedEncrypted)
		}
		if !st.AddedAfter || st.AddedAfterCause != AddedAfterCouldNotCheck {
			t.Errorf("%d bits: added after = %v (%q), want true (could-not-check): nothing measured the coverage either", bits, st.AddedAfter, st.AddedAfterCause)
		}
	}
}

// A sweep that fails on a file that is NOT encrypted keeps no cause of this kind.
func TestOnlyAnEncryptedFileIsCalledEncrypted(t *testing.T) {
	if got := sweepUnchecked(errors.New("read pdf: something else"), false); got != "" {
		t.Errorf("a sweep error on an unencrypted file was named %q", got)
	}
	if got := sweepUnchecked(errLookupCostCeiling, true); got != UncheckedLookupCost {
		t.Errorf("the lookup-cost refusal of an encrypted file was named %q, want its own cause", got)
	}
}

// And a verdict that checked no signer names that, whatever was said of the coverage.
func TestAVerdictThatCheckedNoSignerSaysSoWhateverItsCoverageCause(t *testing.T) {
	plain := []byte("%PDF-1.7\n")
	for _, c := range []struct {
		name string
		st   Status
		want UncheckedCause
	}{
		{"no signer, coverage could not be checked", Status{State: Invalid, AddedAfter: true, AddedAfterCause: AddedAfterCouldNotCheck}, UncheckedUnread},
		{"no signer, nothing said about coverage", Status{State: Invalid}, UncheckedUnread},
		// Controls: each of these is its own answer, and stays one.
		{"content appended after a signature", Status{State: Invalid, AddedAfter: true, AddedAfterCause: AddedAfterAppended}, ""},
		{"a refused signature present", Status{State: Invalid, AddedAfter: true, AddedAfterCause: AddedAfterRefusedSignature}, ""},
		{"a signer was read", Status{State: Invalid, Signers: []SignerInfo{{}}, AddedAfter: true, AddedAfterCause: AddedAfterCouldNotCheck}, ""},
		{"valid", Status{State: Valid, AddedAfter: true, AddedAfterCause: AddedAfterCouldNotCheck}, ""},
	} {
		if got := uncheckedOf(c.st, plain, false); got != c.want {
			t.Errorf("%s: unchecked = %q, want %q", c.name, got, c.want)
		}
	}
}
