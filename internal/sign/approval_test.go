package sign

import (
	"testing"
	"time"

	"nib/internal/testpdf"
)

// Two approval signatures must stack via incremental update and BOTH verify —
// the basis of P2P / multi-signer co-signing.
func TestSignApprovalTwoSignersBothValid(t *testing.T) {
	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	certA, keyA, _ := GenerateIdentity("Alice")
	certB, keyB, _ := GenerateIdentity("Bob")

	one, err := SignApproval(base, certA, keyA, Options{Name: "Alice", When: time.Now()})
	if err != nil {
		t.Fatalf("first approval sign: %v", err)
	}
	two, err := SignApproval(one, certB, keyB, Options{Name: "Bob", When: time.Now()})
	if err != nil {
		t.Fatalf("second approval sign: %v", err)
	}

	st := Verify(two)
	if st.State != Valid {
		t.Errorf("co-signed doc state = %s, want valid", st.State)
	}
	if len(st.Signers) != 2 {
		t.Fatalf("signers = %d, want 2", len(st.Signers))
	}
	for _, s := range st.Signers {
		if !s.Valid {
			t.Errorf("signer %q not valid", s.Name)
		}
	}
}

// A single approval signature on an unsigned doc is the ordinary first-signer case.
func TestSignApprovalOverUnsigned(t *testing.T) {
	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	cert, key, _ := GenerateIdentity("Solo")
	signed, err := SignApproval(base, cert, key, Options{Name: "Solo", When: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if st := Verify(signed); st.State != Valid || len(st.Signers) != 1 {
		t.Errorf("state=%s signers=%d, want valid/1", st.State, len(st.Signers))
	}
}

// SignApproval must refuse a document already certified "no changes" — a later
// signature would break that certification for strict validators.
func TestSignApprovalRefusesCertifiedDoc(t *testing.T) {
	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	certA, keyA, _ := GenerateIdentity("Certifier")
	certB, keyB, _ := GenerateIdentity("Approver")

	certified, err := Sign(base, certA, keyA, Options{Name: "Certifier", When: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SignApproval(certified, certB, keyB, Options{Name: "Approver", When: time.Now()}); err == nil {
		t.Error("SignApproval should refuse to co-sign a certified document, but it succeeded")
	}
}

// The guard must not fire on an unsigned doc or an approval-signed doc.
func TestHasCertificationSignature(t *testing.T) {
	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := hasCertificationSignature(base); got {
		t.Error("unsigned doc reported as certified")
	}
	cert, key, _ := GenerateIdentity("X")
	approved, err := SignApproval(base, cert, key, Options{Name: "X", When: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := hasCertificationSignature(approved); got {
		t.Error("approval-signed doc misreported as certified")
	}
	certified, err := Sign(base, cert, key, Options{Name: "X", When: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := hasCertificationSignature(certified); !got {
		t.Error("certified doc not detected")
	}
}
