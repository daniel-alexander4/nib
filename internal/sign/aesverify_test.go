package sign

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"testing"

	dpdf "github.com/digitorus/pdf"
	"github.com/digitorus/pkcs7"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/testpdf"
)

// `/pending 889` part 6, ADR-170: the signature reader decrypts AES-128 and AES-256, so a signed document
// encrypted the way a real forms engine encrypts it — opening with no password, AES, the signature added to the
// ENCRYPTED file — verifies. The fixtures are built in that order: pdfcpu encrypts an unsigned document with a
// signature field, then an update is appended by hand, its strings encrypted under each object's key and the
// signature's `/Contents` stored as it must be, unencrypted (ISO 32000-2 §7.6.2). Encrypting AFTER signing — what
// the fixture of `TestAnEncryptedSignedDocumentIsNotCalledModified` does — rewrites the file and can never be valid.

// aesCase is one encryption the fixtures are built at: AES-128, and AES-256 at revision 5 (what pdfcpu writes for a
// PDF 1.x document) and revision 6 (for a PDF 2.0 one, and algorithm 2.B's hash).
type aesCase struct {
	name     string
	bits     int
	revision int
}

var aesCases = []aesCase{{"AES-128", 128, 4}, {"AES-256 R5", 256, 5}, {"AES-256 R6", 256, 6}}

// encryptedThenSigned is that fixture at c, signed by id with the name given.
//
// Two shapes a real document carries ride along: an empty string written as a vector alone, with no block after it
// (the `/Reason` here — a real forms engine writes its signature's `/Name` so), and, on the field, a reference to a
// time stamp's dictionary carrying `/Type /DocTimeStamp` and no `/ByteRange` (`/NibProbe`), so that each half of the
// reader's test for a signature's `/Contents` is reached alone. typed says whether the signature dictionary itself
// carries `/Type /Sig`; without it, only its `/ByteRange` says it is a signature's.
func encryptedThenSigned(t *testing.T, c aesCase, id identity, signerName string, typed bool) []byte {
	t.Helper()
	base := testpdf.Assemble(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [4 0 R] /SigFlags 3 >> >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Contents 5 0 R /Annots [4 0 R] >>",
		4: "<< /FT /Sig /T (Sig1) /Type /Annot /Subtype /Widget /Rect [0 0 0 0] /P 3 0 R /F 132 >>",
		5: "<< /Length 15 >>\nstream\n0 0 m 90 90 l S\nendstream",
	})
	if c.revision == 6 {
		copy(base, "%PDF-2.0") // pdfcpu writes revision 6 for a PDF 2.0 document
	}
	enc := encryptedForOwner(t, base, c.bits)
	if !regexp.MustCompile(fmt.Sprintf(`/R\s*%d\b`, c.revision)).Match(enc) {
		t.Fatalf("setup: pdfcpu did not encrypt at revision %d for %s", c.revision, c.name)
	}

	// The file key and the field's place, as pdfcpu reads them back.
	ctx, err := api.ReadContext(context.Background(), bytes.NewReader(enc), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("setup: pdfcpu cannot read its own encrypted fixture: %v", err)
	}
	fileKey := ctx.EncKey
	root, err := ctx.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	form, _ := ctx.DereferenceDict(root["AcroForm"])
	fields, _ := ctx.DereferenceArray(form["Fields"])
	fieldRef, ok := fields[0].(types.IndirectRef)
	if !ok {
		t.Fatalf("setup: the field is not an indirect object: %v", fields[0])
	}
	field, _ := ctx.DereferenceDict(fieldRef)
	pageRef, ok := field["P"].(types.IndirectRef)
	if !ok {
		t.Fatalf("setup: the field names no page: %v", field)
	}
	fieldNum := fieldRef.ObjectNumber.Value()
	sigNum := *ctx.XRefTable.Size
	probeNum := sigNum + 1
	iv := make([]byte, aes.BlockSize)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		t.Fatal(err)
	}
	sigType := ""
	if typed {
		sigType = "/Type /Sig "
	}

	str := func(num int, s string) string {
		return "<" + hex.EncodeToString(aesEncrypt(t, objectKey(fileKey, c.bits, num), []byte(s))) + ">"
	}
	const placeholder = "/ByteRange [0 XXXXXXXXXX XXXXXXXXXX XXXXXXXXXX]"
	contents := "<" + strings.Repeat("0", 2*8192) + ">"
	out, err := testpdf.AppendRevision(enc, map[int]string{
		fieldNum: fmt.Sprintf("<< /FT /Sig /T %s /Type /Annot /Subtype /Widget /Rect [0 0 0 0] /P %d 0 R /F 132 /V %d 0 R /NibProbe %d 0 R >>",
			str(fieldNum, "Sig1"), pageRef.ObjectNumber.Value(), sigNum, probeNum),
		sigNum: fmt.Sprintf("<< %s/Filter /Adobe.PPKLite /SubFilter /adbe.pkcs7.detached /Name %s /M %s /Reason <%x> %s /Contents %s >>",
			sigType, str(sigNum, signerName), str(sigNum, "D:20261013120000Z"), iv, placeholder, contents),
		probeNum: "<< /Type /DocTimeStamp /Contents <00ff10> >>",
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	// The byte range around the /Contents hole, then the signature over it.
	at := bytes.Index(out, []byte(contents))
	gapStart, gapEnd := at, at+len(contents)
	br := fmt.Sprintf("/ByteRange [0 %-10d %-10d %-10d]", gapStart, gapEnd, len(out)-gapEnd)
	pi := bytes.Index(out, []byte(placeholder))
	copy(out[pi:], br)
	signed := append(append([]byte{}, out[:gapStart]...), out[gapEnd:]...)
	sd, err := pkcs7.NewSignedData(signed)
	if err != nil {
		t.Fatal(err)
	}
	sd.SetDigestAlgorithm(pkcs7.OIDDigestAlgorithmSHA256)
	if err := sd.AddSigner(id.cert, id.signer, pkcs7.SignerInfoConfig{}); err != nil {
		t.Fatal(err)
	}
	sd.Detach()
	der, err := sd.Finish()
	if err != nil {
		t.Fatal(err)
	}
	copy(out[gapStart+1:], hex.EncodeToString(der))
	return out
}

// objectKey is ISO 32000-1 §7.6.2 algorithm 1 for AESV2 (the file key, the object number and generation, "sAlT"),
// and the file key itself for AESV3 — written here from the standard, not from the reader under test.
func objectKey(fileKey []byte, bits, num int) []byte {
	if bits == 256 {
		return fileKey
	}
	h := md5.New()
	h.Write(fileKey)
	h.Write([]byte{byte(num), byte(num >> 8), byte(num >> 16), 0, 0, 's', 'A', 'l', 'T'})
	return h.Sum(nil)
}

// aesEncrypt is §7.6.3.1's string encryption: a random vector, then CBC over the PKCS#5-padded plaintext.
func aesEncrypt(t *testing.T, key, plain []byte) []byte {
	t.Helper()
	cb, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	pad := aes.BlockSize - len(plain)%aes.BlockSize
	data := append(append([]byte{}, plain...), bytes.Repeat([]byte{byte(pad)}, pad)...)
	out := make([]byte, aes.BlockSize+len(data))
	if _, err := io.ReadFull(rand.Reader, out[:aes.BlockSize]); err != nil {
		t.Fatal(err)
	}
	cipher.NewCBCEncrypter(cb, out[:aes.BlockSize]).CryptBlocks(out[aes.BlockSize:], data)
	return out
}

func TestASignatureOnAnAESEncryptedDocumentIsVerified(t *testing.T) {
	id := newIdentity(t, "Jane Roe")
	for _, c := range aesCases {
		for _, typed := range []bool{true, false} {
			bits := fmt.Sprintf("%s, /Type /Sig %v", c.name, typed)
			verifiesAndCatchesATamper(t, bits, id, encryptedThenSigned(t, c, id, "Jane Roe", typed))
		}
	}
}

func verifiesAndCatchesATamper(t *testing.T, name string, id identity, doc []byte) {
	t.Helper()
	st := Verify(doc)
	if st.State != Valid {
		t.Errorf("%s: state = %q (unchecked %q, added after %v %q), want valid: the signature was made over these bytes",
			name, st.State, st.Unchecked, st.AddedAfter, st.AddedAfterCause)
		return
	}
	if len(st.Signers) != 1 {
		t.Fatalf("%s: %d signers, want 1", name, len(st.Signers))
	}
	// The /Name is stored encrypted; read back as typed, it was decrypted under its object's key.
	if s := st.Signers[0]; s.Name != "Jane Roe" || s.Fingerprint != id.fp {
		t.Errorf("%s: signer %q (%s), want %q (%s)", name, s.Name, s.Fingerprint, "Jane Roe", id.fp)
	}

	// One byte the signature covers, changed: the binary comment on the header's second line, which no
	// reader parses — so the file still opens and only the hash can tell.
	tampered := append([]byte{}, doc...)
	i := bytes.IndexByte(tampered, '\n') + 2
	if tampered[i-1] != '%' || tampered[i] < 0x80 {
		t.Fatalf("setup (%s): no binary comment at the header's second line", name)
	}
	tampered[i] ^= 0x01
	if st := Verify(tampered); st.State != Invalid || st.Unchecked != "" {
		t.Errorf("%s, one covered byte changed: state = %q (unchecked %q), want invalid with nothing unchecked", name, st.State, st.Unchecked)
	}
}

// The signature's /Contents is never decrypted: the reader hands it on as stored, and the bag the sweep and the
// library read is the one the signer wrote. Asked of the reader directly, because a decrypted /Contents shows in
// `Verify` only as a refused record — the same `Invalid` a tampered file reads as.
func TestTheReaderLeavesASignaturesContentsAsStored(t *testing.T) {
	id := newIdentity(t, "Jane Roe")
	for _, c := range aesCases {
		readsAsStored(t, c, encryptedThenSigned(t, c, id, "Jane Roe", true))
	}
}

func readsAsStored(t *testing.T, c aesCase, doc []byte) {
	t.Helper()
	r, err := dpdf.NewReader(bytes.NewReader(doc), int64(len(doc)))
	if err != nil {
		t.Fatalf("%s: %v", c.name, err)
	}
	// An unfiltered stream reads as written, its last block's padding gone.
	content, err := io.ReadAll(r.Page(1).V.Key("Contents").Reader())
	if err != nil || string(content) != "0 0 m 90 90 l S" {
		t.Errorf("%s: the page's content reads %q (%v), want %q", c.name, content, err, "0 0 m 90 90 l S")
	}
	sig := r.Trailer().Key("Root").Key("AcroForm").Key("Fields").Index(0).Key("V")
	if sig.Key("Type").Name() != "Sig" {
		t.Fatalf("setup (%s): the field's value is not the signature: %v", c.name, sig)
	}
	got := sig.Key("Contents").RawString()
	if _, err := pkcs7.Parse(bytes.TrimRight([]byte(got), "\x00")); err != nil {
		t.Errorf("%s: the signature's /Contents does not parse as the stored PKCS#7 (%v) — it was decrypted", c.name, err)
	}
	// A time stamp's dictionary with no /ByteRange: its /Type alone says its /Contents is stored as written.
	if got := r.Trailer().Key("Root").Key("AcroForm").Key("Fields").Index(0).Key("NibProbe").Key("Contents").RawString(); got != "\x00\xff\x10" {
		t.Errorf("%s: a /DocTimeStamp's /Contents reads %x, want 00ff10 as stored", c.name, got)
	}
	// A vector with no block after it is the empty string, not a malformed one.
	if reason := sig.Key("Reason").RawString(); reason != "" {
		t.Errorf("%s: a /Reason written as a vector alone reads %q, want empty", c.name, reason)
	}
	if name := sig.Key("Name").Text(); name != "Jane Roe" {
		t.Errorf("%s: the signature's /Name reads %q, want %q — the dictionary's other strings ARE decrypted", c.name, name, "Jane Roe")
	}
}

// Every signing path passes `runSign`, which refuses an encrypted document by name: the library writes its signature
// and the catalog it re-emits unencrypted into it. Before the reader read AES this was refused only by accident.
func TestAnEncryptedDocumentIsNotSigned(t *testing.T) {
	base, err := testpdf.Text("the lease")
	if err != nil {
		t.Fatal(err)
	}
	id := newIdentity(t, "Jane Roe")
	for _, bits := range []int{128, 256} {
		enc := encryptedForOwner(t, base, bits)
		out, err := SignApproval(enc, id.certPEM, id.keyPEM, Options{Name: "Jane Roe", Reason: "r"})
		if !errors.Is(err, ErrEncryptedDocument) {
			t.Errorf("%d bits: signing answered %v, want ErrEncryptedDocument", bits, err)
		}
		if out != nil {
			t.Errorf("%d bits: signing an encrypted document handed back %d bytes", bits, len(out))
		}
	}
}

// A 40-bit RC4 document's streams read as written: the object key is the first n+5 bytes of its hash, and upstream
// used all 16, so every such stream decrypted to noise ("zlib: invalid header").
func TestAFortyBitRC4DocumentsStreamsAreRead(t *testing.T) {
	base := testpdf.Assemble(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Contents 4 0 R >>",
		4: "<< /Length 15 >>\nstream\n0 0 m 90 90 l S\nendstream",
	})
	conf := model.NewRC4Configuration("", "owner", 40)
	conf.Cmd = model.ENCRYPT
	var enc bytes.Buffer
	if err := api.Encrypt(context.Background(), bytes.NewReader(base), &enc, conf); err != nil {
		t.Fatalf("setup: %v", err)
	}
	r, err := dpdf.NewReader(bytes.NewReader(enc.Bytes()), int64(enc.Len()))
	if err != nil {
		t.Fatal(err)
	}
	var got []byte
	func() {
		defer func() {
			if rec := recover(); rec != nil {
				t.Errorf("reading the page's content panicked: %v", rec)
			}
		}()
		got, err = io.ReadAll(r.Page(1).V.Key("Contents").Reader())
	}()
	if err != nil || !bytes.Contains(got, []byte("90 90 l")) {
		t.Errorf("the page's content reads %q (%v), want the stream as written", got, err)
	}
}
