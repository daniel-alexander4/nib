package sign

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	dpdf "github.com/digitorus/pdf"
	"github.com/digitorus/pdfsign/sign"
	"github.com/digitorus/pkcs7"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

	"nib/internal/fontcode"
	"nib/internal/pdfread"
)

// Fingerprint returns the SHA-256 of the certificate's SubjectPublicKeyInfo —
// the stable pin for an identity. It hashes the public key, not the whole
// certificate, so the pin survives any re-issue of the cert (new validity,
// fields) as long as the signing key is the same. This is the value exchanged
// out-of-band (QR / safety-number) to pin a peer.
func Fingerprint(certPEM []byte) ([]byte, error) {
	b, _ := pem.Decode(certPEM)
	if b == nil {
		return nil, errors.New("invalid certificate PEM")
	}
	cert, err := x509.ParseCertificate(b.Bytes)
	if err != nil {
		return nil, err
	}
	return fingerprintOf(cert), nil
}

// fingerprintOf is the one place the identity fingerprint is computed: the
// SHA-256 of the certificate's SubjectPublicKeyInfo. Both Fingerprint (from PEM)
// and the verifier (from a parsed signer cert) route through it so a pin, an
// attestation's accepted-peer, and a verified signer all hash the same bytes.
func fingerprintOf(cert *x509.Certificate) []byte {
	sum := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
	return sum[:]
}

// FingerprintCert is Fingerprint for an already-parsed certificate — the same
// SHA-256 SPKI value, exposed for the transport verifier, which works from parsed
// peer certs. Routes through the one fingerprint source so a pin and a verified
// peer hash identical bytes.
func FingerprintCert(cert *x509.Certificate) []byte {
	return fingerprintOf(cert)
}

// GenerateIdentity creates a self-signed ECDSA signing identity and returns its
// certificate and private key as PEM. Self-signed is sufficient for Nib's
// purpose — tamper-evidence (integrity), not third-party identity trust.
func GenerateIdentity(commonName string) (certPEM, keyPEM []byte, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, err
	}
	now := time.Now()
	tmpl := x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: commonName},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.AddDate(30, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, nil, err
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM, nil
}

// Appearance is an optional visible signature appearance: an image baked as the
// signature's on-page widget, added in the same incremental revision as the
// signature itself. This is the PDF-native way to attach visible content to a
// signature — one writer, one revision — so it always renders and is always
// covered, unlike a separately-added annotation.
type Appearance struct {
	Image []byte     // PNG or JPEG to draw as the widget
	Page  int        // 1-based page to place it on
	Rect  [4]float64 // llx, lly, urx, ury in PDF points
}

// Options controls a finalize-and-sign operation.
type Options struct {
	Name       string      // signer common name, recorded in the signature
	Reason     string      // e.g. "Finalized in Nib"; for co-signing, carries the attestation
	When       time.Time   // signing time
	TSAURL     string      // optional RFC3161 timestamp authority
	Appearance *Appearance // optional visible appearance (approval signatures only)
}

// Sign applies a certification signature (DocMDP "no changes allowed") to pdf
// using the given PEM identity. Any later edit invalidates it — that is the
// tamper-evidence. The signature is invisible; callers bake any visible mark
// into the page content before signing. This is the solo-Finalize path.
//
// It refuses a document that is already signed (`ErrAlreadySigned`, /pending 810): a certification
// may only be a document's first signature.
func Sign(pdfBytes, certPEM, keyPEM []byte, opts Options) ([]byte, error) {
	cert, signer, err := ParseIdentity(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}
	data := sign.SignData{
		Signature: sign.SignDataSignature{
			CertType:   sign.CertificationSignature,
			DocMDPPerm: sign.DoNotAllowAnyChangesPerms,
			Info:       sign.SignDataSignatureInfo{Name: opts.Name, Reason: opts.Reason, Date: opts.When},
		},
		Signer:          signer,
		Certificate:     cert,
		DigestAlgorithm: crypto.SHA256,
	}
	if opts.TSAURL != "" {
		data.TSA = sign.TSA{URL: opts.TSAURL}
	}
	return runSign(pdfBytes, data, refuseSigned)
}

// SignApproval applies an approval signature to pdf. Unlike Sign it asserts no
// DocMDP, so it does not lock the document: several parties can co-sign one PDF,
// each adding a signature by incremental update without invalidating the others
// (the basis of P2P / multi-signer). It refuses to co-sign a document that
// already carries a certification ("no changes") signature — a later signature
// would break that certification for strict validators.
func SignApproval(pdfBytes, certPEM, keyPEM []byte, opts Options) ([]byte, error) {
	cert, signer, err := ParseIdentity(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}
	data := sign.SignData{
		Signature: sign.SignDataSignature{
			CertType: sign.ApprovalSignature,
			Info:     sign.SignDataSignatureInfo{Name: opts.Name, Reason: opts.Reason, Date: opts.When},
		},
		Signer:          signer,
		Certificate:     cert,
		DigestAlgorithm: crypto.SHA256,
	}
	if opts.TSAURL != "" {
		data.TSA = sign.TSA{URL: opts.TSAURL}
	}
	if a := opts.Appearance; a != nil && len(a.Image) > 0 {
		page := a.Page
		if page < 1 {
			page = 1
		}
		data.Appearance = sign.Appearance{
			Visible:     true,
			Page:        uint32(page),
			LowerLeftX:  a.Rect[0],
			LowerLeftY:  a.Rect[1],
			UpperRightX: a.Rect[2],
			UpperRightY: a.Rect[3],
			Image:       a.Image,
		}
	}
	return runSign(pdfBytes, data, refuseCertified)
}

// runSign performs the digitorus incremental signing of pdfBytes with the given
// signature data and returns the signed bytes.
//
// It is the one door every production signature passes — `Sign` (finalize), `SignApproval`
// (every co-sign and ceremony contribution, through `p2p.Contribute`) and `SignExternal`
// (finalize with an imported certificate, and `nib sign`) — so the hybrid-reference guard is
// written once here and reaches all of them (ADR-009, /pending 740): a hybrid input is made
// readable to the signing library, or refused by name when it is signed (`readableBySigner`). The
// read-back check is here for the same reason (/pending 747): the output is handed back only if nib
// reads it as the input plus exactly the signature asked for (`signedAsIntended`). And the library's
// reader is opened through `libraryReader`, ADR-041's gate (/pending 712 R6-2, /pending 761), so
// nothing this door signs reaches `digitorus/pdf` unless pdfcpu read it first.
//
// refuse, when set, is a caller's own precondition over the document the library is about to sign,
// asked of the SAME reader so the gate is paid once — `SignApproval`'s certification refusal, and the
// certifying paths' refusal of a document already signed.
func runSign(pdfBytes []byte, data sign.SignData, refuse func(*dpdf.Reader) error) ([]byte, error) {
	in, err := readableBySigner(pdfBytes)
	if err != nil {
		return nil, err
	}
	rdr, err := libraryReader(in)
	if err != nil {
		return nil, err
	}
	if refuse != nil {
		if err := refuse(rdr); err != nil {
			return nil, err
		}
	}
	var out bytes.Buffer
	if err := containedLibrarySign(in, &out, rdr, data); err != nil {
		return nil, describeSignFailure(err, data.TSA.URL)
	}
	if err := signedAsIntended(in, out.Bytes(), data.Certificate); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// containedLibrarySign is `librarySign` with the reader's panics made errors (/pending 761).
//
// The library walks the document through the same `digitorus/pdf` reader, lazily, with no recover of
// its own (`fetchExistingSignatures` resolving `/AcroForm`), and that reader panics on ordinary
// corruption — and, since /pending 761, on a member read that runs past its object stream's end
// (`dpdf.ErrObjStmRunsPastEnd`), which pdfcpu reads and so ADR-041's gate admits. Uncontained, that
// panic took the process from `Sign`. It is the reader's panic `Verify`'s sweep already recovers, said
// the same way (`readPanicErr`).
func containedLibrarySign(in []byte, out *bytes.Buffer, rdr *dpdf.Reader, data sign.SignData) (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = readPanicErr(rec)
		}
	}()
	return librarySign(bytes.NewReader(in), out, rdr, int64(len(in)), data)
}

// librarySign is the one call into `digitorus/pdfsign/sign`, as a variable so a test can make the
// library write something other than a signature and see `signedAsIntended` refuse it.
var librarySign = sign.Sign

// ErrSignedOutputUnreadable refuses a signature the library produced that nib cannot read back as
// one new, well-formed, verifying signature on a readable document (/pending 747). Nothing is
// returned and nothing is written: the caller has only its input.
var ErrSignedOutputUnreadable = errors.New("the signed document failed nib's own read-back check, " +
	"so it was NOT signed and nothing was written")

// signedAsIntended is `runSign`'s post-condition (/pending 747): the library's output is a document
// nib reads back with exactly the signature it was asked to add. Refused otherwise, whatever the
// library returned.
//
// # Why
//
// The signer and the verifier read a PDF with different parsers, and where they disagree the signer
// writes an incremental update over a document nib's verifier does not see. /pending 740 found one
// such disagreement (`/XRefStm`) and closed it at the input; co-signing the hand-built `synthSigned`
// fixture — no hybrid form — made the library write a file whose revision sweep fails
// (`malformed hex string`), which `Verify` reads `Invalid` with no signer. Nothing refused it: the
// user was handed a broken signed document as a success. This check does not know about either
// cause, so any other divergence fails closed too.
//
// # What it checks, and why not `Verify`
//
//  1. pdfcpu reads the output (`pdfcpuCanRead`) — the gate `Verify` puts before every digitorus read
//     (ADR-041), and it comes first here for the same reason: it is containment for the sweep that
//     follows, not a detector (every output the tests build that it refuses, the sweep refuses too).
//  2. The revision sweep (ADR-058) finishes on the output, and finds exactly ONE more record than on
//     the input — so the library neither hid an earlier signature (the /pending 740 shape) nor
//     wrote two.
//  3. Exactly one record reaches the end of the output, and it is well-formed (the sweep's own
//     eleven conjuncts), in the library's enumeration (`libPos`), not a document timestamp, and
//     names the certificate it was asked to sign with.
//  4. Its PKCS#7 verifies over its own `/ByteRange` — one hash of the output, of that signature only.
//
// **A full `Verify` of the output was built first and refused on cost** (/pending 747): it re-hashes
// every earlier signature's revision, and at 100 MB it took signing from 0.33 s to 1.10 s and peak
// heap from 400 to 841 MB. Earlier signatures are the input's, and signing did not touch their bytes
// — an incremental update only appends — so what can have changed about them is whether nib still
// SEES them, which (2) checks by count without hashing anything.
//
// # (4) is not a second verifier of the verify path
//
// `Verify`'s per-signature check is the library's, and the library exposes it only as a whole-document
// call. (4) is the same primitive it ends in (`pkcs7.Verify` over the concatenated ranges,
// `pdfsign verify/signature.go` `processByteRange` + `verifySignature`) over ranges the sweep has
// already proved well-formed. It decides nothing any reader is shown — only whether nib hands its
// own output back — so it cannot disagree with a verdict.
func signedAsIntended(in, out []byte, cert *x509.Certificate) error {
	fail := func(format string, a ...any) error {
		return fmt.Errorf("%w (%s)", ErrSignedOutputUnreadable, fmt.Sprintf(format, a...))
	}
	if err := pdfcpuCanRead(out); err != nil {
		return fail("the result is not a readable PDF: %v", err)
	}
	before, err := sweepRevisions(in)
	if err != nil {
		return fail("nib cannot read the signatures of the document it was given: %v", err)
	}
	after, err := sweepRevisions(out)
	if err != nil {
		return fail("nib cannot read the signatures of the result: %v", err)
	}
	if len(after) != len(before)+1 {
		return fail("the result carries %d signature record(s) where %d were expected", len(after), len(before)+1)
	}
	// Exactly one can reach the end: every input record ends inside the input, and the count above
	// leaves room for one new record.
	var added *Revision
	for i := range after {
		if after[i].CoverageEnd == int64(len(out)) {
			added = &after[i]
		}
	}
	switch {
	case added == nil:
		return fail("no well-formed signature reaches the end of the result")
	case added.libPos < 0 || added.Timestamp:
		// Defence in depth, unreachable by any output the test hook can build: `/SigFlags` and the
		// signature type are written inside the new signature's own coverage, so changing them
		// breaks (4) first. It is kept because an output that trips it is one `Verify` would not
		// count as a signer, whatever (4) says.
		return fail("the new signature is not one nib's verifier would check")
	case cert != nil && added.named != hex.EncodeToString(fingerprintOf(cert)):
		return fail("the new signature does not name the certificate it was made with")
	}
	p7, err := pkcs7.Parse(fontcode.Hex(out[added.gapStart:added.gapEnd]))
	if err != nil {
		return fail("the new signature's contents do not parse: %v", err)
	}
	br := added.ByteRange
	content := make([]byte, 0, len(out)-int(added.gapEnd-added.gapStart))
	for i := 0; i+1 < len(br); i += 2 {
		content = append(content, out[br[i]:br[i]+br[i+1]]...)
	}
	p7.Content = content
	if err := p7.Verify(); err != nil {
		return fail("the new signature does not verify: %v", err)
	}
	return nil
}

// ErrSignedHybridReference refuses to sign a document that already carries a signature and uses the
// hybrid cross-reference form (/pending 740).
var ErrSignedHybridReference = errors.New("this document is already signed and stores part of its " +
	"structure in a hybrid cross-reference stream (/XRefStm) that nib's signer cannot read; making it " +
	"readable means rewriting the file, which would destroy the signature it already carries, so " +
	"nothing was signed — ask whoever sent it for a copy saved without the hybrid form (for example, " +
	"re-exported before anyone signs it), or add this signature in the application that made the " +
	"earlier one")

// readableBySigner returns pdf in a form `digitorus/pdf` reads the way pdfcpu does.
//
// # Why (/pending 740)
//
// `digitorus/pdf` follows a trailer's `/Prev` and never a hybrid-reference trailer's `/XRefStm`
// (ISO 32000-1 7.5.8.4), so every object that stream lists is invisible to it — and the signer
// writes its incremental update over the document it saw. Measured on constructed files: an
// unsigned hybrid with the catalog in the stream signed to a file pdfcpu could not read; with the
// pages there it failed `page number 1 not found`; a SIGNED one co-signed to an `Invalid`, 0-signer
// file, two of three shapes unreadable (`decodeObjectStream: missing entry for obj#6`).
//
// # What it does
//
//   - Not hybrid: returned unchanged.
//   - Hybrid and unsigned (`HasSignatureBlob`, the /pending 733 door, which answers by byte scan for
//     a hybrid file): rewritten through pdfcpu, which writes one cross-reference form, and the
//     rewrite is what is signed. Nothing is lost that a signature protected, because there is none.
//   - Hybrid and signed: refused, `ErrSignedHybridReference`. A rewrite destroys the signature.
//
// # Real producers write this form
//
// 2 of the 36 files in the real-producer corpus carry `/XRefStm` (InDesign, Acrobat). Both signed
// valid BEFORE this door existed — their hybrid streams list nothing the signer needed — so for them
// the rewrite is a cost, not a repair. It is paid anyway because "the stream lists nothing the
// signer needs" is not something nib can know without a second reader.
//
// # PDF/UA (ADR-032)
//
// The rewrite is the same read-and-write the save path's `pdfops.DropUAIdentificationUnlessSigned`
// performs, without its claim drop: this package does not import `pdfops`. Every production caller
// that can reach it with an unsigned document has already been through that door (finalize, `nib
// sign`) or through `p2p.PrepareDocument`'s rewrite (co-sign), and a document carrying a claim comes
// out of either rewritten and no longer hybrid. A new caller that signs unsigned bytes directly
// must drop the claim first, exactly as it would have to for the signature itself.
func readableBySigner(pdf []byte) ([]byte, error) {
	if !hybridReference(pdf) {
		return pdf, nil
	}
	if HasSignatureBlob(pdf) {
		return nil, ErrSignedHybridReference
	}
	conf := model.NewDefaultConfiguration()
	conf.ValidationMode = model.ValidationRelaxed // as pdfcpuCanRead: "can it be read", not "is it valid"
	ctx, err := pdfread.ReadOptimized(pdf, conf)
	if err != nil {
		return nil, fmt.Errorf("read pdf: this document uses a hybrid cross-reference form nib must "+
			"rewrite before signing, and the rewrite could not read it: %w", err)
	}
	var out bytes.Buffer
	if err := api.WriteContext(ctx, &out); err != nil {
		return nil, fmt.Errorf("rewrite hybrid cross-reference document before signing: %w", err)
	}
	return out.Bytes(), nil
}

// hybridReference reports whether pdf names a hybrid-reference `/XRefStm` anywhere. A byte scan, as
// `signatureBlobPresent` uses it: it over-reports (the token in a string) and that routes an
// ordinary file to a rewrite or the byte scan, never past a stream the library cannot see. The name
// is read in either spelling (`nameIn`, /pending 579): pdfcpu follows `/XRef#53tm` as it follows
// `/XRefStm`, and a signed file whose field only that stream lists read as unsigned.
func hybridReference(pdf []byte) bool { return nameIn(pdf, "XRefStm") }

// ErrTimestampAuthority reports that signing failed because the timestamp authority the
// user named could not be used. Callers reprompt or offer to sign without one.
var ErrTimestampAuthority = errors.New("timestamp authority unreachable")

// describeSignFailure turns the signing library's message into one a user can act on.
//
// # Why this is here and not at each caller
//
// `Sign`, `SignApproval` and `SignExternal` all set `data.TSA` and all return through
// `runSign`, so this is the one door every timestamped signature passes (ADR-009). It also
// means the CLI gets it without its own copy — `nib sign --tsa` reached the same library
// error and printed it raw.
//
// # What it does NOT do, and that is the decision
//
// **It does not fall back to signing without a timestamp.** The residual-doubt list proposed
// exactly that — "fall back to a local date + warning when offline" — and it is wrong on the
// merits. `SignerInfo.TimeBacking` distinguishes `TSA` (a token from an independent
// authority) from the signer's own clock, and a verifier reports the difference. A silent
// downgrade produces a document whose verifier says "signer clock" for a user who asked for,
// paid attention to, and believes they have an independent timestamp — a false statement
// about the document, which is worse than a refusal. Measured before deciding: an unreachable
// TSA fails the whole signature, and the raw message was
// `sign: failed to replace signature: failed to create signature: get timestamp: non success
// response (0)`, which names neither the authority nor the fact that nothing was signed.
//
// So the salvageable half of that doubt is the WARNING, and this is it.
func describeSignFailure(err error, tsaURL string) error {
	if tsaURL == "" || !mentionsTimestamp(err) {
		return fmt.Errorf("sign: %w", err)
	}
	return fmt.Errorf("%w: could not get a timestamp from %s, so the document was NOT signed "+
		"— check the address, or turn the timestamp option off to sign with this computer's "+
		"clock instead (underlying error: %v)", ErrTimestampAuthority, tsaURL, err)
}

// mentionsTimestamp matches on the library's message because it exposes no sentinel for
// this. Named and narrow rather than inlined, so the fragility is visible: if digitorus ever
// reworks the wording this stops firing and the raw error comes back — a worse message, not a
// wrong one, which is the safe direction for a string match to fail in.
func mentionsTimestamp(err error) bool {
	m := strings.ToLower(err.Error())
	return strings.Contains(m, "timestamp") || strings.Contains(m, "tsa")
}

// certifiedIn reports whether a document carries a certification (DocMDP)
// signature — the "no changes allowed" kind a later approval signature would
// break for strict validators. Nib's own Verify is purely cryptographic and
// does not surface the signature type, so we read it from the PDF structure:
// the catalog's `/Perms /DocMDP` (ISO 32000-1 12.8.4, Table 258 — the entry that makes a document
// certified), or a signature field whose /V has a /Reference with /TransformMethod /DocMDP, which is
// all `digitorus/pdfsign` writes for nib's own certifications (it writes no `/Perms`).
//
// **The field half is `sigFieldWalk`, the whole field tree with `/FT` inherited** (/pending 734). It
// walked `/Fields`' top level alone, so a certification nested under `/Kids` was invisible and
// SignApproval added an approval signature a DocMDP P=1 certification forbids. Widening it refuses
// more co-signs, and every one it adds is a co-sign a strict validator would report as breaking the
// certification. A tree the walk cannot finish is an error, refused like any unreadable document.
// It is still blind to a certification reachable only through a hybrid-reference `/XRefStm`, by
// the same reader — and that one cannot be co-signed over: runSign refuses every SIGNED hybrid file
// (`readableBySigner`, /pending 740), certified or not, so the blindness costs a less specific refusal,
// never a signature.
//
// A panic in the walk is returned as an error (/pending 502): the same lazy dereferences that needed
// a recover in signatureBlobPresent panic here on corrupt input, and an unreadable document is one
// SignApproval must refuse rather than one that takes the process.
//
// **It reads a reader `libraryReader` opened, never bytes of its own** (/pending 712 R6-2, /pending
// 761): it used to call `dpdf.NewReader` itself with no readability gate in front, and its one
// production caller is the co-sign path, which a peer's document reaches.
func certifiedIn(r *dpdf.Reader) (certified bool, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			certified, err = false, fmt.Errorf("read pdf: %v", rec)
		}
	}()
	root := r.Trailer().Key("Root")
	if !root.Key("Perms").Key("DocMDP").IsNull() {
		return true, nil
	}
	acro := root.Key("AcroForm")
	if acro.IsNull() {
		return false, nil
	}
	found, bounded := sigFieldWalk(acro, func(f dpdf.Value) bool {
		refs := f.Key("V").Key("Reference")
		for j := 0; j < refs.Len(); j++ {
			if refs.Index(j).Key("TransformMethod").Name() == "DocMDP" {
				return true
			}
		}
		return false
	})
	if !bounded {
		return false, errFieldTreeUnbounded
	}
	return found, nil
}

// refuseCertified is `SignApproval`'s precondition, run by `runSign` over the reader it opened.
func refuseCertified(r *dpdf.Reader) error {
	certified, err := certifiedIn(r)
	if err != nil {
		return err
	}
	if certified {
		return errors.New("document is certified (no changes allowed); it cannot be co-signed")
	}
	return nil
}

// ErrAlreadySigned refuses to certify a document that already carries a signature (/pending 810).
var ErrAlreadySigned = errors.New("this document is already signed, and a certification (Finalize) can only be " +
	"a document's first signature — adding one now would be reported as breaking the signature already " +
	"there, so nothing was signed")

// refuseSigned is the certifying paths' precondition (`Sign`, `SignExternal`), run by `runSign` over the reader
// it opened: ISO 32000-1 12.8.2.2 allows a certification signature only as the document's FIRST signature, and
// over a certified "no changes" document it is also a forbidden change. Both certify with DocMDP P=1, so before
// this a document already signed came back with a second certification that Acrobat reports as violating the
// first, while nib's `Verify` — which does not judge DocMDP — called it valid.
//
// **Refused, not downgraded to an approval signature.** The fallback signs, but it is not what the user asked
// for: Finalize promises a document locked against any change, and an approval signature locks nothing.
//
// "Signed" is `HasSignatureBlob`'s field-tree answer (`signedFieldIn`, over the same reader), which every other
// gate on a signed document reads, plus a certification the field tree does not hold — the catalog's `/Perms
// /DocMDP`. Where the reader cannot answer, the reader's error is the refusal: `HasSignatureBlob` falls back to a
// byte scan for `/ByteRange` there, and that is right for "is this unsigned" and would be a false sentence here.
func refuseSigned(r *dpdf.Reader) error {
	signed, err := signedFieldIn(r)
	if err != nil {
		return err
	}
	if signed {
		return ErrAlreadySigned
	}
	certified, err := certifiedIn(r)
	if err != nil {
		return err
	}
	if certified {
		return ErrAlreadySigned
	}
	return nil
}

// libraryReader is the one way the signing paths open `digitorus/pdf`'s reader, and it is ADR-041's
// rule applied to them: **nothing enters the library that pdfcpu cannot read** (/pending 712 R6-2,
// /pending 761). `Verify` had the gate and the signer did not — `runSign` and the certification walk
// each called `dpdf.NewReader` on the bytes they were handed. What the gate keeps out is what pdfcpu
// cannot READ — a damaged object-stream `/Filter` (ADR-041), and a stream dictionary whose `/N` or
// `/First` is a reference, which compounds per level in the library and which the reader does not
// charge (ADR-067). It does NOT keep out the library's endless loops: an unterminated `[`, `<` or `(`
// at an object stream's clean end, and `endobj` inside an array, are documents pdfcpu reads, and the
// patched reader bounds those itself (ADR-068). The lookup-cost ceiling (`libraryLookupCost`, /pending 751) follows for the reason
// the sweep runs it: the library would pay that cost too.
//
// The sweep opens its own reader and is not routed here, because both of its callers run the gate
// first — `Verify` (`pdfcpuRead`) and `signedAsIntended` (over `out` by `pdfcpuCanRead`, over `in`
// because `runSign` opened it here) — and a second pdfcpu read per call would be paid on every
// verify. `TestEveryLibraryReaderIsBehindTheGate` holds that list.
func libraryReader(pdf []byte) (r *dpdf.Reader, err error) {
	if err := pdfcpuCanRead(pdf); err != nil {
		return nil, fmt.Errorf("read pdf: nib cannot read this document, so it is not signed: %w", err)
	}
	defer func() {
		if rec := recover(); rec != nil {
			r, err = nil, fmt.Errorf("read pdf: %v", rec)
		}
	}()
	r, err = dpdf.NewReader(bytes.NewReader(pdf), int64(len(pdf)))
	if err != nil {
		return nil, fmt.Errorf("read pdf: %w", err)
	}
	if _, err := libraryLookupCost(r); err != nil {
		return nil, err
	}
	return r, nil
}

// ParseIdentity decodes an identity's PEM certificate and key into a parsed
// certificate and its signer. Exposed for the P2P transport layer, which mints a
// short-lived child certificate signed by the identity key.
func ParseIdentity(certPEM, keyPEM []byte) (*x509.Certificate, crypto.Signer, error) {
	cb, _ := pem.Decode(certPEM)
	kb, _ := pem.Decode(keyPEM)
	if cb == nil || kb == nil {
		return nil, nil, errors.New("invalid identity PEM")
	}
	cert, err := x509.ParseCertificate(cb.Bytes)
	if err != nil {
		return nil, nil, err
	}
	key, err := x509.ParsePKCS8PrivateKey(kb.Bytes)
	if err != nil {
		return nil, nil, err
	}
	signer, ok := key.(crypto.Signer)
	if !ok {
		return nil, nil, errors.New("key is not a signer")
	}
	return cert, signer, nil
}

// SignDigest signs a 32-byte digest with the identity's private key (ECDSA P-256,
// ASN.1 DER), for artifacts that are not PDFs.
//
// Separate from Sign, which produces a PKCS#7 embedded in a PDF's signature dictionary —
// a different container for a different job. This one exists for the Ceremony Record
// (D20), which is signed before any document is signed and must be verifiable by a party
// holding only the record.
//
// It takes a DIGEST rather than a message, deliberately: the caller decides what the
// preimage is, and for the record that decision is a specified list of length-prefixed
// axes. A function taking a message would invite a second, implicit preimage.
func SignDigest(digest []byte, keyPEM []byte) ([]byte, error) {
	if len(digest) != sha256.Size {
		return nil, fmt.Errorf("digest is %d bytes, want %d", len(digest), sha256.Size)
	}
	blk, _ := pem.Decode(keyPEM)
	if blk == nil {
		return nil, errors.New("invalid private key PEM")
	}
	key, err := x509.ParseECPrivateKey(blk.Bytes)
	if err != nil {
		k, err2 := x509.ParsePKCS8PrivateKey(blk.Bytes)
		if err2 != nil {
			return nil, fmt.Errorf("parse private key: %w", err)
		}
		ec, ok := k.(*ecdsa.PrivateKey)
		if !ok {
			return nil, errors.New("private key is not ECDSA")
		}
		key = ec
	}
	return ecdsa.SignASN1(rand.Reader, key, digest)
}

// VerifyDigestSPKI checks a SignDigest signature against a raw SubjectPublicKeyInfo.
//
// The certificate is 501 bytes and the SPKI inside it is 91 (measured, P-256). Anywhere
// the verifier's carrier has a hard size budget — P04.S03's candidate record lives inside
// BEP-44's 1000-byte value cap — the cert does not fit beside the payload and the SPKI
// does, with room.
//
// Nothing is lost by carrying the smaller one. The pin IS the SHA-256 of these exact
// bytes (see fingerprintOf), so a reader can both verify the signature and check the
// signer against a pinned fingerprint from the SPKI alone. The certificate's other fields
// — subject, validity, serial — are not consulted by either operation: Nib's identity is
// self-signed and its trust comes entirely from the pin.
func VerifyDigestSPKI(digest, sig, spki []byte) error {
	pub, err := x509.ParsePKIXPublicKey(spki)
	if err != nil {
		return fmt.Errorf("parse public key: %w", err)
	}
	ec, ok := pub.(*ecdsa.PublicKey)
	if !ok {
		return errors.New("public key is not ECDSA")
	}
	if len(digest) != sha256.Size {
		return fmt.Errorf("digest is %d bytes, want %d", len(digest), sha256.Size)
	}
	if !ecdsa.VerifyASN1(ec, digest, sig) {
		return errors.New("signature does not verify")
	}
	return nil
}

// SPKIFromCert returns the raw SubjectPublicKeyInfo from a certificate PEM — the 91
// bytes a size-bounded carrier can afford where the 501-byte certificate cannot.
func SPKIFromCert(certPEM []byte) ([]byte, error) {
	blk, _ := pem.Decode(certPEM)
	if blk == nil {
		return nil, errors.New("invalid certificate PEM")
	}
	cert, err := x509.ParseCertificate(blk.Bytes)
	if err != nil {
		return nil, err
	}
	return cert.RawSubjectPublicKeyInfo, nil
}

// FingerprintSPKI is the pin computed from a raw SubjectPublicKeyInfo — the same value
// Fingerprint and FingerprintCert produce, routed through the same one-line definition so
// a pin, a verified signer and a candidate record's author all hash identical bytes.
func FingerprintSPKI(spki []byte) []byte {
	sum := sha256.Sum256(spki)
	return sum[:]
}

// VerifyDigest checks a SignDigest signature against the certificate's public key.
func VerifyDigest(digest, sig, certPEM []byte) error {
	blk, _ := pem.Decode(certPEM)
	if blk == nil {
		return errors.New("invalid certificate PEM")
	}
	cert, err := x509.ParseCertificate(blk.Bytes)
	if err != nil {
		return err
	}
	pub, ok := cert.PublicKey.(*ecdsa.PublicKey)
	if !ok {
		return errors.New("certificate key is not ECDSA")
	}
	// The same length rule as VerifyDigestSPKI. ECDSA verification truncates whatever it is given,
	// so without this a caller handing over the wrong buffer gets a clean "does not verify" rather
	// than being told it passed something that is not a SHA-256 digest (/pending 502).
	if len(digest) != sha256.Size {
		return fmt.Errorf("digest is %d bytes, want %d", len(digest), sha256.Size)
	}
	if !ecdsa.VerifyASN1(pub, digest, sig) {
		return errors.New("signature does not verify")
	}
	return nil
}
