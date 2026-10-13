package sign

import (
	"bytes"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"strings"
	"testing"
	"time"

	"nib/internal/testpdf"
)

// TestACertificationIsOnlyEverTheFirstSignature — /pending 810. `Sign` and `SignExternal` certify with DocMDP P=1,
// and ISO 32000-1 12.8.2.2 allows a certification only as a document's FIRST signature. Both certified a document
// already signed, so Acrobat reported the earlier signature violated while nib's `Verify` (which does not judge DocMDP)
// called the result valid. Both now refuse, by name, whatever kind of signature is already there.
func TestACertificationIsOnlyEverTheFirstSignature(t *testing.T) {
	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	certA, keyA, _ := GenerateIdentity("Alice")
	certB, keyB, _ := GenerateIdentity("Bob")
	p12 := makeP12(t, "Jane External", "p12pass")
	when := time.Now()

	approved, err := SignApproval(base, certA, keyA, Options{Name: "Alice", When: when})
	if err != nil {
		t.Fatal(err)
	}
	certified, err := Sign(base, certA, keyA, Options{Name: "Alice", When: when})
	if err != nil {
		t.Fatalf("the control: certifying an unsigned document: %v", err)
	}
	if _, err := SignExternal(base, p12, "p12pass", Options{When: when}); err != nil {
		t.Fatalf("the control: certifying an unsigned document with an imported identity: %v", err)
	}

	// A signature nested under a parent's /Kids, which the field walk must see (/pending 693's shape).
	a := newIdentity(t, "Carol")
	objs := baseObjs(sigDict("1", ""))
	objs[3] = sobj{num: 4, body: "<</T(parent)/Kids[6 0 R]>>"}
	objs = append(objs, sobj{num: 6, body: "<</FT/Sig/T(child)/Parent 4 0 R/V 5 0 R>>"})
	nested := fillSig(t, synthRevision(t, nil, objs, 1), "1", nil, detached(t, a))
	if st := Verify(nested); st.State != Valid || len(st.Signers) != 1 {
		t.Fatalf("STIMULUS: the nested document reads %s with %d signers, want valid with one", st.State, len(st.Signers))
	}

	for _, c := range []struct {
		name string
		doc  []byte
	}{
		{"approval-signed", approved},
		{"certified", certified},
		{"signed in a field nested under /Kids", nested},
		{"certified through the catalog's /Perms alone", permsOnlyCertified(t)},
	} {
		if _, err := Sign(c.doc, certB, keyB, Options{Name: "Bob", When: when}); !errors.Is(err, ErrAlreadySigned) {
			t.Errorf("Sign over a document %s: err = %v, want ErrAlreadySigned", c.name, err)
		}
		if _, err := SignExternal(c.doc, p12, "p12pass", Options{When: when}); !errors.Is(err, ErrAlreadySigned) {
			t.Errorf("SignExternal over a document %s: err = %v, want ErrAlreadySigned", c.name, err)
		}
	}
}

// permsOnlyCertified is a document whose one signature is named by the catalog's `/Perms /DocMDP` and by no form field
// — the entry ISO 32000-1 12.8.4 makes the mark of a certified document. Neither the field walk nor `/V` sees it.
func permsOnlyCertified(t *testing.T) []byte {
	t.Helper()
	objs := []sobj{
		{num: 1, body: "<</Type/Catalog/Pages 2 0 R/Perms<</DocMDP 5 0 R>>>>"},
		{num: 2, body: "<</Type/Pages/Kids[3 0 R]/Count 1>>"},
		{num: 3, body: "<</Type/Page/Parent 2 0 R/MediaBox[0 0 200 200]>>"},
		{num: 4, body: "<<>>"},
		{num: 5, body: sigDict("1", docMDPReference)},
	}
	doc := fillSig(t, synthRevision(t, nil, objs, 1), "1", nil, detached(t, newIdentity(t, "Dave")))
	if HasSignatureBlob(doc) {
		t.Fatalf("STIMULUS: the field walk sees a signature in a document with no form")
	}
	return doc
}

// docMDPReference is the `/Reference` a certification signature dictionary carries (ISO 32000-1 Table 253), P=1.
const docMDPReference = "/Reference[<</Type/SigRef/TransformMethod/DocMDP/TransformParams<</Type/TransformParams/P 1/V/1.2>>>>]"

// TestACertificationNestedUnderKidsIsSeen — /pending 734. `certifiedIn` walked `/Fields`' top level alone, so a
// certification whose field sits under a parent's `/Kids`, or inherits `/FT`, was invisible and `SignApproval` added
// an approval signature a DocMDP P=1 certification forbids. It now reads the field tree through `sigFieldWalk`, the
// walk `HasSignatureBlob` uses.
func TestACertificationNestedUnderKidsIsSeen(t *testing.T) {
	a := newIdentity(t, "Alice")
	certB, keyB, _ := GenerateIdentity("Bob")
	for _, c := range []struct {
		name        string
		parent, kid string
	}{
		{"the field under a parent's /Kids", "<</T(parent)/Kids[6 0 R]>>", "<</FT/Sig/T(child)/Parent 4 0 R/V 5 0 R>>"},
		{"/FT inherited from the parent", "<</FT/Sig/T(parent)/Kids[6 0 R]>>", "<</T(child)/Parent 4 0 R/V 5 0 R>>"},
	} {
		t.Run(c.name, func(t *testing.T) {
			objs := baseObjs(sigDict("1", docMDPReference))
			objs[3] = sobj{num: 4, body: c.parent}
			objs = append(objs, sobj{num: 6, body: c.kid})
			doc := fillSig(t, synthRevision(t, nil, objs, 1), "1", nil, detached(t, a))
			if st := Verify(doc); st.State != Valid || len(st.Signers) != 1 {
				t.Fatalf("STIMULUS: Verify reads %s with %d signers, want valid with one", st.State, len(st.Signers))
			}
			if !certificationOf(t, doc) {
				t.Errorf("a certification nested under /Kids is not seen")
			}
			_, err := SignApproval(doc, certB, keyB, Options{Name: "Bob", When: time.Now()})
			if err == nil || !strings.Contains(err.Error(), "certified") {
				t.Errorf("SignApproval over a nested certification: err = %v, want the certification refusal", err)
			}
		})
	}
	// The control: the same nesting with an APPROVAL signature is not a certification, and co-signs.
	objs := baseObjs(sigDict("1", ""))
	objs[3] = sobj{num: 4, body: "<</T(parent)/Kids[6 0 R]>>"}
	objs = append(objs, sobj{num: 6, body: "<</FT/Sig/T(child)/Parent 4 0 R/V 5 0 R>>"})
	doc := fillSig(t, synthRevision(t, nil, objs, 1), "1", nil, detached(t, a))
	if certificationOf(t, doc) {
		t.Errorf("a nested approval signature reads as a certification")
	}
}

// TestASignerCarriesItsCertificatesSubjectAndNotTheTypedName — ADR-147, `/pending 831`: the subject a surface
// leads with is the CERTIFICATE's, from the certificate the fingerprint is of, and it is not `/Name`, which
// the signer typed (ADR-051). A signature that does not verify carries neither a fingerprint nor a subject.
func TestASignerCarriesItsCertificatesSubjectAndNotTheTypedName(t *testing.T) {
	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	cert, key, err := GenerateIdentity("Alice Certificate")
	if err != nil {
		t.Fatal(err)
	}
	signed, err := SignApproval(base, cert, key, Options{Name: "The Queen of England", When: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	st := Verify(signed)
	if st.State != Valid || len(st.Signers) != 1 {
		t.Fatalf("setup: %s with %d signers", st.State, len(st.Signers))
	}
	got := st.Signers[0]
	if got.Name != "The Queen of England" {
		t.Fatalf("setup: /Name reads %q — the typed name is what this test tells apart from the subject", got.Name)
	}
	if got.Fingerprint == "" {
		t.Fatal("setup: the signer has no fingerprint")
	}
	if got.Subject != "Alice Certificate" {
		t.Errorf("the signer's subject reads %q, want the certificate's common name %q", got.Subject, "Alice Certificate")
	}
	// The record is the one home (ADR-058): the same subject, beside the same fingerprint.
	revs, err := Revisions(signed)
	if err != nil || len(revs) != 1 {
		t.Fatalf("revisions: %d (err %v)", len(revs), err)
	}
	if revs[0].Subject != got.Subject || revs[0].Fingerprint != got.Fingerprint {
		t.Errorf("the record says %q/%q and the signer %q/%q", revs[0].Subject, revs[0].Fingerprint, got.Subject, got.Fingerprint)
	}

	// A byte changed inside the signed range: the signature fails, and nothing is said about who made it.
	broken := bytes.Replace(signed, []byte("/Type /Catalog"), []byte("/Type /Catalog "), 1)
	if bytes.Equal(broken, signed) {
		broken = append([]byte(nil), signed...)
		broken[len(broken)/4] ^= 0x01
	}
	for _, sg := range Verify(broken).Signers {
		if !sg.Valid && (sg.Subject != "" || sg.Fingerprint != "") {
			t.Errorf("a signature that does not verify names %q (%q): nothing reports an identity it has not established", sg.Subject, sg.Fingerprint)
		}
	}
}

// TestACertificateSubjectIsOneBoundedLine — the subject is a stranger's text: control characters and
// direction overrides become spaces, and a long one is cut.
func TestACertificateSubjectIsOneBoundedLine(t *testing.T) {
	cases := []struct{ cn, want string }{
		{"Alice", "Alice"},
		{"Alice\nNib says: verified", "Alice Nib says: verified"},
		{"evil\u202Egnp.exe", "evil gnp.exe"},
		{strings.Repeat("x", maxSubjectRunes+50), strings.Repeat("x", maxSubjectRunes) + "…"},
	}
	for _, c := range cases {
		cert := &x509.Certificate{Subject: pkix.Name{CommonName: c.cn}}
		if got := subjectOf(cert); got != c.want {
			t.Errorf("subjectOf(%q) = %q, want %q", c.cn, got, c.want)
		}
	}
	// No common name: the whole subject, so an organisation-only certificate is not shown as nothing.
	if got := subjectOf(&x509.Certificate{Subject: pkix.Name{Organization: []string{"Acme Ltd"}}}); got != "O=Acme Ltd" {
		t.Errorf("a subject with no common name reads %q, want %q", got, "O=Acme Ltd")
	}
}
