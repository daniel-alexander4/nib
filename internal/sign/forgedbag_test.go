package sign

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	"github.com/digitorus/pdfsign/sign"
	"github.com/digitorus/pdfsign/verify"
	"github.com/digitorus/pkcs7"

	"nib/internal/testpdf"
)

// moveCertToFrontOfBag rewrites signed's PKCS#7 blob so that wantDER leads the
// certificate bag, leaving the signature, its SignerInfo and the /ByteRange untouched.
//
// It is the attack in one function. The `/Contents` region is the hole in the
// `/ByteRange` — the bag is NOT covered by the signature — so anyone who can hand nib a
// file can order it however they like, and a producer that is not pdfsign needs no
// surgery at all to emit this order in the first place.
//
// The splice is done on the raw bytes rather than by re-marshalling the ASN.1, because a
// reorder is exactly length-preserving: every length octet in the enclosing structures,
// the hex string's own extent, and therefore every xref offset in the document, is
// unchanged. Re-marshalling would risk changing what the test thinks it is testing.
func moveCertToFrontOfBag(t *testing.T, signed, wantDER []byte) []byte {
	t.Helper()
	start, end := contentsHexRange(t, signed)
	blob, err := hex.DecodeString(string(signed[start:end]))
	if err != nil {
		t.Fatalf("decode /Contents hex: %v", err)
	}
	p7, err := pkcs7.Parse(blob)
	if err != nil {
		t.Fatalf("parse PKCS#7: %v", err)
	}
	if len(p7.Certificates) != 2 {
		t.Fatalf("bag holds %d certificates, want the 2 this attack needs", len(p7.Certificates))
	}
	if bytes.Equal(p7.Certificates[0].Raw, wantDER) {
		t.Fatal("the certificate to promote already leads the bag; the reorder would be a no-op")
	}

	var before, after []byte
	for _, c := range p7.Certificates {
		before = append(before, c.Raw...)
	}
	after = append(after, wantDER...)
	for _, c := range p7.Certificates {
		if !bytes.Equal(c.Raw, wantDER) {
			after = append(after, c.Raw...)
		}
	}
	at := bytes.Index(blob, before)
	if at < 0 {
		t.Fatal("the bag's certificates are not contiguous in the blob; the splice cannot be located")
	}
	if len(before) != len(after) {
		t.Fatalf("reorder changed the bag length %d -> %d", len(before), len(after))
	}
	copy(blob[at:], after)

	spliced := []byte(hex.EncodeToString(blob))
	if len(spliced) != end-start {
		t.Fatalf("spliced blob is %d hex chars, was %d", len(spliced), end-start)
	}
	out := append([]byte(nil), signed...)
	copy(out[start:end], spliced)
	return out
}

// contentsHexRange locates the hex digits of the SIGNATURE's /Contents string.
//
// `/Contents` is also a page's content-stream key, and in a document whose page objects are not in
// an object stream the page's comes first. The discriminator is the value: a signature's is a hex
// string of at least `SignatureMaxLengthBase` (1024 hex characters), where a page's is a reference.
// Matching on length rather than on position keeps this off a property of `testpdf`'s output that
// nothing states.
func contentsHexRange(t *testing.T, signed []byte) (start, end int) {
	t.Helper()
	const minSignatureHex = 1024
	for i := 0; ; {
		at := bytes.Index(signed[i:], []byte("/Contents"))
		if at < 0 {
			break
		}
		at += i
		i = at + len("/Contents")
		lt := bytes.IndexByte(signed[at:], '<')
		if lt < 0 {
			break
		}
		s := at + lt + 1
		gt := bytes.IndexByte(signed[s:], '>')
		if gt < 0 {
			break
		}
		if gt >= minSignatureHex {
			return s, s + gt
		}
	}
	t.Fatalf("no /Contents carrying a hex string of at least %d characters — no signature blob",
		minSignatureHex)
	return 0, 0
}

// mintCAIssuedIdentity returns a CA identity and a leaf identity the CA issued.
//
// Two certificates in one bag is the ORDINARY shape, not an exotic one: `SignExternal`
// embeds a leaf and its CA chain on every imported-identity signature. The bag is a SET
// OF, which DER orders canonically by encoding rather than by role, so which of the two
// leads is a fact about the bytes — not about who signed.
func mintCAIssuedIdentity(t *testing.T, caName, leafName string) (caPEM, leafPEM, leafKeyPEM []byte) {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	caTmpl := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: caName},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, &caTmpl, &caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	ca, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatal(err)
	}

	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	leafTmpl := x509.Certificate{
		SerialNumber:          big.NewInt(2),
		Subject:               pkix.Name{CommonName: leafName},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, &leafTmpl, ca, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(leafKey)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}),
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
}

func certDER(t *testing.T, certPEM []byte) []byte {
	t.Helper()
	blk, _ := pem.Decode(certPEM)
	if blk == nil {
		t.Fatal("no PEM block in certificate")
	}
	return blk.Bytes
}

func fingerprintHex(t *testing.T, certPEM []byte) string {
	t.Helper()
	fp, err := Fingerprint(certPEM)
	if err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(fp)
}

// signWithBagOf signs pdf with the identity in certPEM/keyPEM, embedding `extra`
// alongside the signer's own certificate in the PKCS#7 bag.
//
// This is the honest half of the attack: a signature may legitimately carry more than
// one certificate (a leaf and its CA chain — `SignExternal` does exactly this), and
// nothing about the extra certificate is a claim that it signed anything.
func signWithBagOf(t *testing.T, pdf, certPEM, keyPEM []byte, extra *x509.Certificate) []byte {
	t.Helper()
	cert, signer, err := ParseIdentity(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	out, err := runSign(pdf, sign.SignData{
		Signature:         sign.SignDataSignature{CertType: sign.ApprovalSignature, Info: sign.SignDataSignatureInfo{Name: "Attacker", Reason: "r"}},
		Signer:            signer,
		Certificate:       cert,
		CertificateChains: [][]*x509.Certificate{{cert, extra}},
		DigestAlgorithm:   crypto.SHA256,
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestAForgedBagDoesNotRenameTheSigner is /pending 613.
//
// A signature made with the attacker's own key, carrying the victim's certificate FIRST
// in the bag, is a perfectly valid signature — by the attacker. The bag's ORDER is not
// signed and not checked, so reading the identity out of element 0 reports the victim as
// having signed a document they never saw: a forged "X co-signed".
//
// Both halves are asserted, because either alone passes for the wrong reason: the
// signature must still verify (otherwise integrity catches the file and the identity
// question never arises) AND the reported fingerprint must be the attacker's.
func TestAForgedBagDoesNotRenameTheSigner(t *testing.T) {
	base, err := testpdf.Text("the lease")
	if err != nil {
		t.Fatal(err)
	}
	victimCert, attackerCert, attackerKey := mintCAIssuedIdentity(t, "Victim", "Attacker")
	victim, err := x509.ParseCertificate(certDER(t, victimCert))
	if err != nil {
		t.Fatal(err)
	}
	victimFP, attackerFP := fingerprintHex(t, victimCert), fingerprintHex(t, attackerCert)

	signed := signWithBagOf(t, base, attackerCert, attackerKey, victim)
	if st := Verify(signed); st.State != Valid || len(st.Signers) != 1 || st.Signers[0].Fingerprint != attackerFP {
		t.Fatalf("precondition: honest two-certificate bag reports state=%q signers=%d fingerprint=%q, want valid/1/%q",
			st.State, len(st.Signers), firstSignerFP(st), attackerFP)
	}

	forged := moveCertToFrontOfBag(t, signed, certDER(t, victimCert))

	st := Verify(forged)
	if len(st.Signers) != 1 {
		t.Fatalf("forged doc reports %d signers, want 1 — the forgery did not survive parsing", len(st.Signers))
	}
	if st.State != Valid || !st.Signers[0].Valid {
		t.Fatalf("precondition: forged doc state = %q valid=%v; the attack this pins needs a VALID signature",
			st.State, st.Signers[0].Valid)
	}
	if got := st.Signers[0].Fingerprint; got == victimFP {
		t.Errorf("forged bag renamed the signer: fingerprint = the victim's, %s…", got[:16])
	} else if got != attackerFP {
		t.Errorf("fingerprint = %q, want the attacker's %q", got, attackerFP)
	}
}

func firstSignerFP(st Status) string {
	if len(st.Signers) == 0 {
		return ""
	}
	return st.Signers[0].Fingerprint
}

// blankFieldsArray empties every `/Fields [...]` array in place, padding with spaces so the
// document's byte length and therefore every xref offset is unchanged.
func blankFieldsArray(t *testing.T, pdf []byte) []byte {
	t.Helper()
	out := append([]byte(nil), pdf...)
	blanked := 0
	for i := 0; ; {
		at := bytes.Index(out[i:], []byte("/Fields"))
		if at < 0 {
			break
		}
		at += i
		open := bytes.IndexByte(out[at:], '[')
		if open < 0 {
			break
		}
		open += at
		close := bytes.IndexByte(out[open:], ']')
		if close < 0 {
			break
		}
		close += open
		for j := open + 1; j < close; j++ {
			out[j] = ' '
		}
		blanked++
		i = close
	}
	if blanked == 0 {
		t.Fatal("no /Fields array to empty; the fixture does not drive this case")
	}
	return out
}

// TestASignatureAbsentFromFieldsIsStillAttributed pins WHICH walk finds the signatures, and it
// exists because choosing the cheaper walk re-opened /pending 613 inside its own fix.
//
// The library enumerates signatures by sweeping the xref for `/Filter /Adobe.PPKLite`; it never
// reads `AcroForm/Fields`. A first cut of `signerFingerprintsByBag` read `/Fields` because that is
// two orders of magnitude cheaper, on the reasoning that a signature the cheap walk misses simply
// gets no fingerprint — fail-closed.
//
// **That reasoning was wrong, and this is the shape that shows it.** Under ADR-051 the bag was the
// join key, and it sits in the unsigned `/Contents` hole, so an attacker wrote both sides of it: a
// real signature reachable only through the xref, plus a decoy `/Fields` entry carrying the same
// certificate bag and a `SignerInfo` naming the victim. A gap in the walk does not mean no object
// supplies the identity — it means a DIFFERENT object does. Under ADR-058 the join is positional
// over the library's own enumeration, so a record set built from any other walk misaligns the
// positions instead; `sign.sweep` walking the xref, as the library does, is what this pins.
//
// **It asserts on the walk rather than through `Verify`, and that is forced by the library.**
// `processSignature` returns as soon as verification fails and never reaches
// `buildCertificateChainsWithOptions` (`pdfsign verify/signature.go:50-61`), so a signature that
// does not verify is reported with an EMPTY certificate bag — nib has never been able to attribute
// one, before this change or after. Emptying `/Fields` lands inside the signed byte range, so the
// only end-to-end fixture for this case is one whose signature no longer verifies, where the
// identity question is already unanswerable for an unrelated reason. The walk is where the choice
// this test is about actually lives.
func TestASignatureAbsentFromFieldsIsStillAttributed(t *testing.T) {
	base, err := testpdf.Text("the lease")
	if err != nil {
		t.Fatal(err)
	}
	certPEM, keyPEM, err := GenerateIdentity("Signer")
	if err != nil {
		t.Fatal(err)
	}
	signed, err := SignApproval(base, certPEM, keyPEM, Options{Name: "Signer", Reason: "r"})
	if err != nil {
		t.Fatal(err)
	}
	want := fingerprintHex(t, certPEM)
	if st := Verify(signed); len(st.Signers) != 1 || st.Signers[0].Fingerprint != want {
		t.Fatalf("precondition: honest document reports %d signers, fingerprint %q, want 1 and %q",
			len(st.Signers), firstSignerFP(st), want)
	}

	emptied := blankFieldsArray(t, signed)

	// Stimulus, not decoration: the signature must genuinely be unreachable through `/Fields` now,
	// or this test passes without ever driving the case. `signatureBlobPresent` IS the /Fields
	// walk, so it is the honest witness — and `scanForSignatureBlob` confirms the blob is still
	// there to be found by the walk that counts.
	if signatureBlobPresent(emptied) {
		t.Fatal("setup: the signature is still reachable through /Fields; the case is not driven")
	}
	if !scanForSignatureBlob(emptied) {
		t.Fatal("setup: the signature blob is gone entirely, not just delisted")
	}

	// The sweep is the walk the library performs; a signature it cannot reach is a signature the
	// attacker could have supplied the identity for from a decoy. The record's `named` fingerprint
	// is asserted rather than `Fingerprint`, because emptying `/Fields` lands inside the signed
	// range and the signature no longer verifies — `Fingerprint` is set only where it did.
	revs, err := sweepRevisions(emptied)
	if err != nil {
		t.Fatal(err)
	}
	var found []Revision
	for _, r := range revs {
		if r.libPos >= 0 {
			found = append(found, r)
		}
	}
	if len(found) != 1 {
		t.Fatalf("the walk found %d signature(s) in a document whose /Fields lists none, want 1 — "+
			"a walk the attacker can empty is a walk the attacker chooses the identity from",
			len(found))
	}
	if found[0].named != want {
		t.Errorf("fingerprint = %q, want %q", found[0].named, want)
	}
}

// reorderBag rewrites a PKCS#7 blob so that first leads its certificate bag, length-preserving —
// the bag is a SET outside what the SignerInfo signs, so the signature still verifies.
func reorderBag(t *testing.T, blob, first []byte) []byte {
	t.Helper()
	p7, err := pkcs7.Parse(blob)
	if err != nil {
		t.Fatal(err)
	}
	var before, after []byte
	for _, c := range p7.Certificates {
		before = append(before, c.Raw...)
	}
	after = append(after, first...)
	for _, c := range p7.Certificates {
		if !bytes.Equal(c.Raw, first) {
			after = append(after, c.Raw...)
		}
	}
	at := bytes.Index(blob, before)
	if at < 0 || len(before) != len(after) {
		t.Fatal("the bag's certificates are not contiguous in the blob")
	}
	out := append([]byte(nil), blob...)
	copy(out[at:], after)
	return out
}

// TestOneBagWithTwoSignersNamesEach — under ADR-051 the certificate bag was the join key, so two
// signatures carrying byte-identical bags but made by different keys made the key ambiguous, and
// both were blanked. ADR-058 joins by POSITION in the library's own enumeration and keeps the bag
// only as a cross-check, so the ambiguity was an artefact of the map: each signature is now named
// by the key that made it.
//
// pdfsign will not produce this document (`AddSignerChain` refuses a chain whose parent did not
// issue the leaf), so it is built by hand: Alice signs carrying Bob's certificate after her own,
// Bob co-signs carrying Alice's, and Bob's bag is re-ordered to Alice's order. The oracle is the
// test's own certificates, never what `Verify` reports.
func TestOneBagWithTwoSignersNamesEach(t *testing.T) {
	a, b := newIdentity(t, "Alice"), newIdentity(t, "Bob")
	rev1 := fillSig(t, synthRevision(t, nil, baseObjs(sigDict("1", "")), 1), "1", nil, detached(t, a, b.cert))
	rev2 := synthRevision(t, rev1, []sobj{
		{num: 1, body: "<</Type/Catalog/Pages 2 0 R/AcroForm<</SigFlags 3/Fields[4 0 R 7 0 R]>>>>"},
		{num: 7, body: "<</FT/Sig/T(Signature2)/V 8 0 R>>"},
		{num: 8, body: sigDict("2", "")},
	}, 1)
	bobs := detached(t, b, a.cert)
	doc := fillSig(t, rev2, "2", nil, func(content []byte) []byte { return reorderBag(t, bobs(content), a.cert.Raw) })

	resp, err := verify.Verify(bytes.NewReader(doc), int64(len(doc)))
	if err != nil || len(resp.Signers) != 2 {
		t.Fatalf("library: %v", err)
	}
	// STIMULUS: two valid signatures whose bags are byte-identical — the ambiguous key.
	if !resp.Signers[0].ValidSignature || !resp.Signers[1].ValidSignature ||
		bagKeyOfSigner(&resp.Signers[0]) != bagKeyOfSigner(&resp.Signers[1]) {
		t.Fatal("STIMULUS: the two signatures are not both valid with one shared bag")
	}
	st := Verify(doc)
	if len(st.Signers) != 2 {
		t.Fatalf("%d signers, want 2", len(st.Signers))
	}
	if st.Signers[0].Fingerprint != a.fp || st.Signers[1].Fingerprint != b.fp {
		t.Errorf("fingerprints = [%.12s… %.12s…], want Alice's %.12s… then Bob's %.12s…",
			st.Signers[0].Fingerprint, st.Signers[1].Fingerprint, a.fp, b.fp)
	}
}
