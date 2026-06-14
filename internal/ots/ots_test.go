package ots

import (
	"bytes"
	"context"
	"crypto/sha256"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// pendingSeq builds a synthetic, well-formed calendar commitment: an "append
// nonce" op, a sha256 op, then a pending attestation naming the calendar — the
// same shape a real calendar /digest response has.
func pendingSeq(nonce []byte, calRef string) []byte {
	var b []byte
	b = append(b, 0xf0)            // append
	b = appendVarbytes(b, nonce)   // its argument
	b = append(b, 0x08)            // sha256
	b = append(b, tagAttestation)  // attestation follows
	b = append(b, pendingMagic...) // ...a pending one
	inner := appendVarbytes(nil, []byte(calRef))
	b = appendVarbytes(b, inner) // outer wrapper around varbytes(ref)
	return b
}

func TestValidatePendingSequence(t *testing.T) {
	good := pendingSeq([]byte{1, 2, 3, 4}, "https://example.org")
	if err := validatePendingSequence(good); err != nil {
		t.Fatalf("valid sequence rejected: %v", err)
	}

	bad := map[string][]byte{
		"empty":                   {},
		"ops with no attestation": {0x08, 0x08},
		"unknown op tag":          {0x42, tagAttestation},
		"wrong attestation magic": append([]byte{tagAttestation}, bytes.Repeat([]byte{0x00}, 8)...),
		"trailing data":           append(append([]byte{}, good...), 0x08),
	}
	for name, b := range bad {
		if err := validatePendingSequence(b); err == nil {
			t.Errorf("%s: expected rejection, got nil", name)
		}
	}
}

func TestBuildOTSPreambleAndGlue(t *testing.T) {
	digest := sha256.Sum256([]byte("hello"))
	s0 := pendingSeq([]byte{0xaa}, "https://a.example")
	s1 := pendingSeq([]byte{0xbb}, "https://b.example")

	out := buildOTS(digest, [][]byte{s0, s1})

	if !bytes.HasPrefix(out, headerMagic) {
		t.Fatal("missing header magic")
	}
	rest := out[len(headerMagic):]
	if rest[0] != 0x01 || rest[1] != opSHA256 {
		t.Fatalf("bad version/op preamble: %x %x", rest[0], rest[1])
	}
	if !bytes.Equal(rest[2:2+32], digest[:]) {
		t.Fatal("digest not embedded after preamble")
	}
	body := rest[2+32:]
	// Independent sequences are written as a checkpoint (reset to the digest)
	// followed by each sequence back-to-back — matching the reference serializer
	// for calendar attestations that diverge from the first operation.
	want := append(append([]byte{tagCheckpoint}, s0...), s1...)
	if !bytes.Equal(body, want) {
		t.Fatalf("multi-calendar glue mismatch:\n got %x\nwant %x", body, want)
	}

	// Single calendar: no checkpoint byte, body == the lone sequence.
	one := buildOTS(digest, [][]byte{s0})
	if !bytes.Equal(one[len(headerMagic)+2+32:], s0) {
		t.Fatal("single-calendar body should equal the sequence verbatim")
	}
}

func TestStampAssemblesFromCalendars(t *testing.T) {
	cal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/digest" || r.Method != http.MethodPost {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		body, _ := io.ReadAll(r.Body)
		if len(body) != 32 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Write(pendingSeq([]byte{0x01, 0x02}, "https://cal.example"))
	}))
	defer cal.Close()

	digest := sha256.Sum256([]byte("doc"))
	proof, err := Stamp(context.Background(), cal.Client(), digest, []string{cal.URL})
	if err != nil {
		t.Fatalf("Stamp: %v", err)
	}
	if !bytes.HasPrefix(proof, headerMagic) || !bytes.Contains(proof, digest[:]) {
		t.Fatal("assembled proof missing header or digest")
	}

	// All calendars unreachable -> a clear error, no proof.
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer down.Close()
	if _, err := Stamp(context.Background(), down.Client(), digest, []string{down.URL}); err == nil {
		t.Fatal("expected error when no calendar accepts the stamp")
	}
}
