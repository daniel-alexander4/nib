package ots

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func varuint(n uint64) []byte {
	var b []byte
	for {
		x := byte(n & 0x7f)
		n >>= 7
		if n != 0 {
			x |= 0x80
		}
		b = append(b, x)
		if n == 0 {
			return b
		}
	}
}

// bitcoinSeqBytes encodes a calendar /timestamp response: ops then a Bitcoin
// block-height attestation.
func bitcoinSeqBytes(nonce []byte, height uint64) []byte {
	var b []byte
	b = append(b, opAppend)
	b = appendVarbytes(b, nonce)
	b = append(b, opSHA256)
	b = append(b, tagAttestation)
	b = append(b, bitcoinMagic...)
	b = appendVarbytes(b, varuint(height))
	return b
}

func TestComputeOps(t *testing.T) {
	digest := []byte{0x01, 0x02}
	got, err := sequence{ops: []op{{opAppend, []byte{0xaa}}, {opPrepend, []byte{0xbb}}}}.compute(digest)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0xbb, 0x01, 0x02, 0xaa}
	if !bytes.Equal(got, want) {
		t.Fatalf("append/prepend: got %x want %x", got, want)
	}

	h := sha256.Sum256(digest)
	got, _ = sequence{ops: []op{{opSHA256, nil}}}.compute(digest)
	if !bytes.Equal(got, h[:]) {
		t.Fatalf("sha256: got %x want %x", got, h[:])
	}

	if _, err := (sequence{ops: []op{{0x03, nil}}}).compute(digest); err == nil {
		t.Fatal("expected unsupported-op error for ripemd160")
	}
}

func TestParseProofRoundTrip(t *testing.T) {
	digest := sha256.Sum256([]byte("doc"))
	seq := pendingSeq([]byte{0xaa}, "https://cal.example")
	p, err := parseProof(buildOTS(digest, [][]byte{seq}))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(p.digest, digest[:]) {
		t.Fatal("digest mismatch after parse")
	}
	if len(p.seqs) != 1 || p.seqs[0].calURL != "https://cal.example" {
		t.Fatalf("parsed sequences wrong: %+v", p.seqs)
	}
	if _, err := parseProof([]byte("garbage")); err == nil {
		t.Fatal("expected error parsing non-ots bytes")
	}
}

func TestVerifyProofConfirmed(t *testing.T) {
	digest := sha256.Sum256([]byte("verify me"))
	const height = uint64(800000)
	blockTime := int64(1_700_000_000)
	nonce := []byte{0xaa, 0xbb}
	tailNonce := []byte{0x01, 0x02, 0x03}

	// commitment the calendar will be asked about, and the merkle root the
	// upgraded sequence computes to.
	commitment, _ := sequence{ops: []op{{opAppend, nonce}, {opSHA256, nil}}}.compute(digest[:])
	root, _ := sequence{ops: []op{{opAppend, tailNonce}, {opSHA256, nil}}}.compute(commitment)

	calendar := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/timestamp/"+hex.EncodeToString(commitment) {
			w.Write(bitcoinSeqBytes(tailNonce, height))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer calendar.Close()

	hdr := make([]byte, 80)
	copy(hdr[36:68], root)
	binary.LittleEndian.PutUint32(hdr[68:72], uint32(blockTime))
	explorer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/block-height/"):
			w.Write([]byte(strings.Repeat("ab", 32)))
		case strings.HasSuffix(r.URL.Path, "/header"):
			w.Write([]byte(hex.EncodeToString(hdr)))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer explorer.Close()

	proof := buildOTS(digest, [][]byte{pendingSeq(nonce, calendar.URL)})
	sources := []BlockSource{NewEsplora(explorer.URL, explorer.Client())}
	res, err := VerifyProof(context.Background(), calendar.Client(), sources, 1, proof, digest)
	if err != nil {
		t.Fatalf("VerifyProof: %v", err)
	}
	if res.State != StateConfirmed || res.Height != height || res.Time.Unix() != blockTime {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestVerifyProofPendingAndMismatch(t *testing.T) {
	digest := sha256.Sum256([]byte("doc"))

	// Calendar that never confirms -> pending.
	cal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer cal.Close()
	proof := buildOTS(digest, [][]byte{pendingSeq([]byte{0x01}, cal.URL)})

	res, err := VerifyProof(context.Background(), cal.Client(), nil, 2, proof, digest)
	if err != nil || res.State != StatePending {
		t.Fatalf("expected pending, got %+v (err %v)", res, err)
	}

	// Same proof, wrong document digest -> mismatch (no network needed).
	other := sha256.Sum256([]byte("a different document"))
	res, err = VerifyProof(context.Background(), cal.Client(), nil, 2, proof, other)
	if err != nil || res.State != StateMismatch {
		t.Fatalf("expected mismatch, got %+v (err %v)", res, err)
	}
}

func TestVerifyProofAgreementThreshold(t *testing.T) {
	digest := sha256.Sum256([]byte("threshold doc"))
	const height = uint64(800001)
	nonce := []byte{0x09, 0x08}
	root, _ := sequence{ops: []op{{opAppend, nonce}, {opSHA256, nil}}}.compute(digest[:])

	hdr := make([]byte, 80)
	copy(hdr[36:68], root)
	newExplorer := func() *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case strings.HasPrefix(r.URL.Path, "/block-height/"):
				w.Write([]byte(strings.Repeat("ab", 32)))
			case strings.HasSuffix(r.URL.Path, "/header"):
				w.Write([]byte(hex.EncodeToString(hdr)))
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
	}
	e1, e2 := newExplorer(), newExplorer()
	defer e1.Close()
	defer e2.Close()

	// Proof is already Bitcoin-attested, so VerifyProof skips the calendar upgrade
	// (nil client) and goes straight to the explorer agreement check.
	proof := buildOTS(digest, [][]byte{bitcoinSeqBytes(nonce, height)})

	// Two explorers agree, minAgree 2 -> confirmed.
	both := []BlockSource{NewEsplora(e1.URL, e1.Client()), NewEsplora(e2.URL, e2.Client())}
	res, err := VerifyProof(context.Background(), nil, both, 2, proof, digest)
	if err != nil || res.State != StateConfirmed || res.Sources != 2 {
		t.Fatalf("two agreeing explorers: got %+v err %v", res, err)
	}

	// Only one explorer available but minAgree 2 -> refuse (can't cross-check).
	one := []BlockSource{NewEsplora(e1.URL, e1.Client())}
	if _, err := VerifyProof(context.Background(), nil, one, 2, proof, digest); err == nil {
		t.Fatal("single explorer with minAgree 2: expected error, got nil")
	}
}
