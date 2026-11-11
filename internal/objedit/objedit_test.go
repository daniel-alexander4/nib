package objedit

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"nib/internal/ceremony"
	"nib/internal/pdfops"
	"nib/internal/sign"
	"nib/internal/testpdf"
)

func firstEdit(t *testing.T, pdf []byte) []pdfops.ObjectEdit {
	t.Helper()
	objs, err := pdfops.ReadPageObjects(pdf, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(objs.Objects) != 2 || !objs.Objects[0].Editable {
		t.Fatalf("fixture: %+v", objs.Objects)
	}
	return []pdfops.ObjectEdit{{Kind: pdfops.ObjectEditDelete, Page: 1, Ordinal: 0, Digest: objs.Objects[0].Digest}}
}

func wantCause(t *testing.T, err error, cause string) {
	t.Helper()
	ref, ok := Refusal(err)
	if !ok || ref.Cause != cause {
		t.Fatalf("want a %s refusal, got %v", cause, err)
	}
	if !strings.Contains(err.Error(), pdfops.ObjectCauseWords(cause)) {
		t.Fatalf("the %s refusal does not say its sentence: %q", cause, err)
	}
}

func signedTwoPictures(t *testing.T) []byte {
	t.Helper()
	certPEM, keyPEM, err := sign.GenerateIdentity("Nib Test")
	if err != nil {
		t.Fatal(err)
	}
	signed, err := sign.Sign(testpdf.TwoPictures(), certPEM, keyPEM, sign.Options{Name: "Nib Test", When: time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	if st := sign.Verify(signed).State; st != sign.Valid {
		t.Fatalf("fixture verifies as %q — it is not signed", st)
	}
	return signed
}

// ConvenedTwoPictures embeds a ceremony record and signs nothing: a convened, unsigned document (D5's pin).
func convenedTwoPictures(t *testing.T) []byte {
	t.Helper()
	pdf := testpdf.TwoPictures()
	hash, err := ceremony.DocumentHash(pdf)
	if err != nil {
		t.Fatal(err)
	}
	fp := sha256.Sum256([]byte("convener"))
	rec := ceremony.Record{ID: "picture-ceremony", DocHash: hash, Intent: "sign the lease", Expires: time.Now().Add(time.Hour),
		Roster: []ceremony.Party{{Fingerprint: hex.EncodeToString(fp[:]), Label: "Convener", Signs: true}}}
	out, err := ceremony.Embed(pdf, rec)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ceremony.Extract(out); err != nil {
		t.Fatalf("fixture: the record does not read back: %v", err)
	}
	return out
}

func TestAPlainDocumentHasItsPictureRemoved(t *testing.T) {
	pdf := testpdf.TwoPictures()
	out, res, err := Edit(pdf, firstEdit(t, pdf))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Removed) != 1 || bytes.Equal(out, pdf) {
		t.Fatalf("answer %+v", res)
	}
	objs, err := List(out, 1)
	if err != nil || len(objs.Objects) != 1 {
		t.Fatalf("after: %+v, %v", objs, err)
	}
}

// TestASignedDocumentIsRefusedBeforeAnyRead — the edit is one the unsigned document takes, so the refusal is the
// signature's and nothing else; the listing says it of every picture.
func TestASignedDocumentIsRefusedBeforeAnyRead(t *testing.T) {
	signed := signedTwoPictures(t)
	edits := firstEdit(t, testpdf.TwoPictures())
	out, _, err := Edit(signed, edits)
	if out != nil {
		t.Fatal("bytes were returned for a signed document")
	}
	wantCause(t, err, pdfops.CauseSigned)
	objs, err := List(signed, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range objs.Objects {
		if o.Editable || o.Why != pdfops.CauseSigned {
			t.Fatalf("a signed document's picture is listed %v %q", o.Editable, o.Why)
		}
	}
}

// TestAConvenedDocumentIsRefusedColdEvenUnsigned — the record alone refuses, read from the bytes: nothing here
// knows of a live ceremony, which is the reopened-file case D5's pin names.
func TestAConvenedDocumentIsRefusedColdEvenUnsigned(t *testing.T) {
	convened := convenedTwoPictures(t)
	if sign.HasSignatureBlob(convened) {
		t.Fatal("fixture: the convened document is signed")
	}
	_, _, err := Edit(convened, firstEdit(t, convened))
	wantCause(t, err, pdfops.CauseCeremony)
	objs, err := List(convened, 1)
	if err != nil {
		t.Fatal(err)
	}
	if objs.Objects[0].Editable || objs.Objects[0].Why != pdfops.CauseCeremony {
		t.Fatalf("listed %+v", objs.Objects[0])
	}
}

func TestARequestThatIsNotEditsIsABadRequest(t *testing.T) {
	for _, body := range []string{"", "{", `{"edits": 3}`, `{"edits": [], "extra": 1}`} {
		_, err := DecodeEdits(strings.NewReader(body))
		wantCause(t, err, pdfops.CauseBadRequest)
	}
	edits, err := DecodeEdits(strings.NewReader(`{"edits": [{"kind": "delete", "page": 1, "ordinal": 0, "digest": "ab"}]}`))
	if err != nil || len(edits) != 1 || edits[0].Digest != "ab" || edits[0].Kind != "delete" {
		t.Fatalf("%+v %v", edits, err)
	}
}

// A ceremony record that is THERE and does not decode is a convened document all the same: `ceremony.Extract` would
// call it "no record", so the door looks for the entry itself (code-review 2026-11-09).
func TestABrokenCeremonyRecordStillRefuses(t *testing.T) {
	content := "q 100 0 0 80 10 10 cm /Im0 Do Q"
	pdf := testpdf.Assemble(map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R /Names << /EmbeddedFiles << /Names [(" + pdfops.CeremonyRecordName + ") 10 0 R] >> >> >>",
		2:  "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 600 800] /Resources << /XObject << /Im0 6 0 R >> >> /Contents 4 0 R >>",
		4:  "<< /Length " + strconv.Itoa(len(content)) + " >>\nstream\n" + content + "\nendstream",
		6:  "<< /Type /XObject /Subtype /Image /Width 2 /Height 2 /ColorSpace /DeviceGray /BitsPerComponent 8 /Length 4 >>\nstream\nabcd\nendstream",
		10: "<< /Type /Filespec /F (" + pdfops.CeremonyRecordName + ") /UF (" + pdfops.CeremonyRecordName + ") /EF << /F 11 0 R >> >>",
		11: "<< /Type /EmbeddedFile /Filter /FlateDecode /Length 8 >>\nstream\nnotflate\nendstream",
	})
	if _, err := ceremony.Extract(pdf); !errors.Is(err, ceremony.ErrNoRecord) {
		t.Fatalf("setup: Extract answers %v; this test is about the record Extract calls absent", err)
	}
	objs, err := pdfops.ReadPageObjects(pdf, 1)
	if err != nil || len(objs.Objects) != 1 {
		t.Fatalf("setup: %+v %v", objs, err)
	}
	_, _, err = Edit(pdf, []pdfops.ObjectEdit{{Kind: pdfops.ObjectEditDelete, Page: 1, Ordinal: 0, Digest: objs.Objects[0].Digest}})
	wantCause(t, err, pdfops.CauseCeremony)
}
