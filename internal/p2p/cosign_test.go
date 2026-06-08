package p2p

import (
	"bytes"
	"encoding/hex"
	"image"
	"image/color"
	"image/png"
	"testing"
	"time"

	"nib/internal/pdfops"
	"nib/internal/sign"
	"nib/internal/testpdf"
)

// appearancePNG stands in for the client-rendered attestation block: a bordered
// block sized to the placement rect. (In the app the browser renders the text;
// the backend only bakes the image, like flatten/redact.)
func appearancePNG(p Placement) []byte {
	w := int((p.Rect[2] - p.Rect[0]) * 2)
	h := int((p.Rect[3] - p.Rect[1]) * 2)
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if x < 4 || y < 4 || x >= w-4 || y >= h-4 {
				img.Set(x, y, color.RGBA{0, 0, 0, 255})
			} else {
				img.Set(x, y, color.RGBA{245, 245, 245, 255})
			}
		}
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}

func fp(t *testing.T, certPEM []byte) string {
	t.Helper()
	b, err := sign.Fingerprint(certPEM)
	if err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(b)
}

// contribute runs one party's full step: pick placement, render appearance, sign.
func contribute(t *testing.T, pdf, cert, key []byte, att Attestation) []byte {
	t.Helper()
	p, err := NextPlacement(pdf)
	if err != nil {
		t.Fatal(err)
	}
	out, err := Contribute(pdf, cert, key, att, appearancePNG(p), p)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// The full Track A flow: prepare (append readme) -> A signs with attestation ->
// B signs with attestation. The result must carry two valid approval signatures,
// each attestation cross-bound to the other party's fingerprint, nothing flagged
// as added after the last signature, and nothing the scan considers suspicious.
func TestCoSignRoundTrip(t *testing.T) {
	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	certA, keyA, _ := sign.GenerateIdentity("Alice")
	certB, keyB, _ := sign.GenerateIdentity("Bob")
	fpA, fpB := fp(t, certA), fp(t, certB)
	now := time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC)

	prepared, err := PrepareDocument(base)
	if err != nil {
		t.Fatal(err)
	}

	docA := contribute(t, prepared, certA, keyA, Attestation{
		Signer: "Alice", AcceptedPeer: fpB, AcceptedPeerLabel: "Bob", When: now,
	})
	if st := sign.Verify(docA); st.State != sign.Valid || len(st.Signers) != 1 || st.AddedAfter {
		t.Fatalf("after A: state=%s signers=%d addedAfter=%v", st.State, len(st.Signers), st.AddedAfter)
	}

	docB := contribute(t, docA, certB, keyB, Attestation{
		Signer: "Bob", AcceptedPeer: fpA, AcceptedPeerLabel: "Alice", When: now,
	})

	st := sign.Verify(docB)
	if st.State != sign.Valid {
		t.Errorf("final state = %s, want valid", st.State)
	}
	if len(st.Signers) != 2 {
		t.Fatalf("signers = %d, want 2", len(st.Signers))
	}
	for _, s := range st.Signers {
		if !s.Valid {
			t.Errorf("signer %q not valid", s.Name)
		}
	}
	if st.AddedAfter {
		t.Error("addedAfter = true; B's attestation+signature should be covered by B's signature")
	}

	// Both attestations present, cross-bound: each accepts the other's fingerprint.
	atts := ReadAttestations(docB)
	if len(atts) != 2 {
		t.Fatalf("attestations = %d, want 2", len(atts))
	}
	if atts[0].Signer != "Alice" || atts[0].AcceptedPeer != fpB {
		t.Errorf("attestation 0 not Alice->Bob: %+v", atts[0])
	}
	if atts[1].Signer != "Bob" || atts[1].AcceptedPeer != fpA {
		t.Errorf("attestation 1 not Bob->Alice: %+v", atts[1])
	}
	for _, a := range atts {
		if !a.Valid {
			t.Errorf("attestation by %q rides an invalid signature", a.Signer)
		}
	}

	// The artifact must not trip Nib's own hidden-content scan.
	rep, err := pdfops.Scan(docB)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range rep.Findings {
		switch f.Kind {
		case "attachment", "action", "javascript", "openAction", "additionalActions":
			t.Errorf("co-signed artifact flagged by Scan: %+v", f)
		}
	}
}

// An attestation must refuse to co-sign a certified (no-changes) document.
func TestCoSignRefusesCertified(t *testing.T) {
	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	certA, keyA, _ := sign.GenerateIdentity("Certifier")
	certB, keyB, _ := sign.GenerateIdentity("Bob")
	certified, err := sign.Sign(base, certA, keyA, sign.Options{Name: "Certifier", When: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Contribute(certified, certB, keyB, Attestation{Signer: "Bob", AcceptedPeer: fp(t, certA), AcceptedPeerLabel: "Certifier", When: time.Now()}, nil, Placement{}); err == nil {
		t.Error("Contribute should refuse to co-sign a certified document")
	}
}

// ReadAttestations must cross-bind: in a mutual A<->B co-signing each signer's
// accepted peer is the other's actual fingerprint, so both are Matched; a single
// signer whose peer hasn't co-signed yet is not Matched.
func TestReadAttestationsCrossBinding(t *testing.T) {
	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	certA, keyA, _ := sign.GenerateIdentity("Alice")
	certB, keyB, _ := sign.GenerateIdentity("Bob")
	fpA, fpB := fp(t, certA), fp(t, certB)
	now := time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC)

	prepared, err := PrepareDocument(base)
	if err != nil {
		t.Fatal(err)
	}
	docA := contribute(t, prepared, certA, keyA, Attestation{
		Signer: "Alice", AcceptedPeer: fpB, AcceptedPeerLabel: "Bob", When: now,
	})

	// Mid-flow: only Alice has signed; she accepts Bob, who isn't a co-signer yet.
	one := ReadAttestations(docA)
	if len(one) != 1 || one[0].Fingerprint != fpA || one[0].AcceptedPeer != fpB || one[0].Matched {
		t.Fatalf("mid-flow attestation = %+v, want Alice(fp=%s) accepts %s, not matched", one, fpA, fpB)
	}

	docB := contribute(t, docA, certB, keyB, Attestation{
		Signer: "Bob", AcceptedPeer: fpA, AcceptedPeerLabel: "Alice", When: now,
	})
	two := ReadAttestations(docB)
	if len(two) != 2 {
		t.Fatalf("attestations = %d, want 2", len(two))
	}
	if two[0].Fingerprint != fpA || two[1].Fingerprint != fpB {
		t.Errorf("signer fingerprints wrong: %s / %s", two[0].Fingerprint, two[1].Fingerprint)
	}
	if !two[0].Matched || !two[1].Matched {
		t.Errorf("mutual co-signing should cross-bind both: %+v", two)
	}
}
