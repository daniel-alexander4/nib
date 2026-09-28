package sign

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/digitorus/timestamp"

	"nib/internal/testpdf"
)

// TestASelfMintedTimestampIsNotIndependent — `/pending 708`, the reviewer's reproduction as a standing
// test. The signer runs its own "timestamp authority": a self-signed timestamping certificate and a
// genTime of its choosing (2001, twenty-five years back). The token is well-formed and signed, so the
// only thing that can tell it from a real authority's is whether its certificate chains to a root this
// machine trusts — and nib used to call ANY token "an independent timestamp authority".
func TestASelfMintedTimestampIsNotIndependent(t *testing.T) {
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(7), Subject: pkix.Name{CommonName: "Totally Independent TSA"},
		NotBefore: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC), NotAfter: time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC),
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageTimeStamping}, KeyUsage: x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &k.PublicKey, k)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	backdated := time.Date(2001, 9, 9, 1, 46, 40, 0, time.UTC)
	tsa := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		req, err := timestamp.ParseRequest(b)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		ts := timestamp.Timestamp{HashAlgorithm: req.HashAlgorithm, HashedMessage: req.HashedMessage,
			Time: backdated, Nonce: req.Nonce, Policy: []int{1, 2, 3}, Accuracy: time.Second, AddTSACertificate: true}
		resp, err := ts.CreateResponseWithOpts(cert, k, crypto.SHA256)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/timestamp-reply")
		_, _ = w.Write(resp)
	}))
	defer tsa.Close()

	base, err := testpdf.Text("the lease")
	if err != nil {
		t.Fatal(err)
	}
	c, key, err := GenerateIdentity("Mallory")
	if err != nil {
		t.Fatal(err)
	}
	signed, err := SignApproval(base, c, key, Options{Name: "Mallory", Reason: "r", When: time.Now(), TSAURL: tsa.URL})
	if err != nil {
		t.Fatal(err)
	}
	st := Verify(signed)
	if len(st.Signers) != 1 {
		t.Fatalf("setup: %d signer(s), want 1", len(st.Signers))
	}
	s := st.Signers[0]
	// The stimulus: the token was embedded and read — its backdated time is what is reported. Without
	// this, "not TSA" could be a signature that simply carried no token.
	if s.When != backdated.Format("2006-01-02 15:04 MST") {
		t.Fatalf("setup: the signer's time is %q, not the token's %s — the token never reached verification", s.When, backdated)
	}
	if !s.Valid {
		t.Fatalf("setup: the signature itself does not verify (%s), so this is not the case in question", st.State)
	}
	if s.TimeBacking != TSAUnverified {
		t.Fatalf("a self-minted token, backdated to 2001, reads as %q — want %q: nothing independent vouched for that time",
			s.TimeBacking, TSAUnverified)
	}
}
