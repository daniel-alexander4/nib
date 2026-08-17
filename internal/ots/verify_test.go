package ots

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// bitcoinSeqBytes encodes a calendar /timestamp response: ops then a Bitcoin
// block-height attestation.
func bitcoinSeqBytes(nonce []byte, height uint64) []byte {
	var b []byte
	b = append(b, opAppend)
	b = appendVarbytes(b, nonce)
	b = append(b, opSHA256)
	b = append(b, tagAttestation)
	b = append(b, bitcoinMagic...)
	b = appendVarbytes(b, putVaruint(height))
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

	// sha1/keccak256/ripemd160 are now executed (see the Compute* vector tests);
	// the transform ops reverse (0xf2) and hexlify (0xf3) stay deliberately
	// unsupported, so an unknown/unexecuted tag must still surface a clear error.
	if _, err := (sequence{ops: []op{{0xf3, nil}}}).compute(digest); err == nil {
		t.Fatal("expected unsupported-op error for hexlify (0xf3)")
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

func TestVerifyProofPersistsUpgrade(t *testing.T) {
	digest := sha256.Sum256([]byte("persist me"))
	const height = uint64(800500)
	blockTime := int64(1_700_500_000)
	nonce := []byte{0x11, 0x22}
	tailNonce := []byte{0x33, 0x44, 0x55}

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
	if res.State != StateConfirmed {
		t.Fatalf("expected confirmed, got %+v", res)
	}
	if res.Upgraded == nil {
		t.Fatal("expected an upgraded proof to persist, got nil")
	}

	// The upgraded proof must be self-contained: no pending calendar attestation.
	p, err := parseProof(res.Upgraded)
	if err != nil {
		t.Fatalf("upgraded proof does not parse: %v", err)
	}
	for _, s := range p.seqs {
		if s.calURL != "" {
			t.Fatal("upgraded proof still carries a pending calendar attestation")
		}
	}

	// And it re-verifies to confirmed WITHOUT a calendar: nil client would panic
	// if the upgrade path were reached, proving no calendar contact is needed.
	res2, err := VerifyProof(context.Background(), nil, sources, 1, res.Upgraded, digest)
	if err != nil {
		t.Fatalf("re-verify upgraded proof: %v", err)
	}
	if res2.State != StateConfirmed || res2.Height != height {
		t.Fatalf("upgraded proof did not re-verify: %+v", res2)
	}
	if res2.Upgraded != nil {
		t.Fatal("re-verifying an already-complete proof should not produce a new upgrade")
	}
}

// TestParseRefusesCheckpointAmplification pins the bound on parseSequences.
//
// The shape it refuses: N no-argument operations followed by N checkpoint bytes.
// Each checkpoint duplicates the whole block built so far, so the proof materializes
// N² instructions out of 2N bytes of input. Measured before the bound existed: an
// 8 KB input allocated 876 MiB, cleanly quadratic, so a ~32 KB file — four orders
// below the 200 MiB upload cap on /api/timestamp/verify — reaches tens of gigabytes
// and OOM-kills the process with every open document.
//
// Two halves, and the second is what stops the first being satisfied by a parser
// that refuses everything:
//   - the hostile shape is refused, with ErrProofTooComplex SPECIFICALLY. A test
//     happy with any error also passes when the cursor primitives beneath it break
//     — before the bound, this input failed with "sequence has no attestation",
//     which is exactly the wrong-reason green.
//   - a real reference-encoded proof that genuinely USES checkpoints (merkle1OTS,
//     two calendar branches over a shared op prefix) still parses. Without this arm
//     a bound of zero would pass.
func TestParseRefusesCheckpointAmplification(t *testing.T) {
	build := func(k int) []byte {
		var b []byte
		b = append(b, headerMagic...)
		b = append(b, 0x01, opSHA256)
		b = append(b, make([]byte, 32)...)
		for i := 0; i < k; i++ {
			b = append(b, opSHA256) // no-argument op: one byte, one instruction
		}
		for i := 0; i < k; i++ {
			b = append(b, tagCheckpoint) // one byte, k instructions
		}
		return b
	}

	// Well under the ceiling: refused for its own reasons, never for complexity.
	if _, err := parseProof(build(50)); errors.Is(err, ErrProofTooComplex) {
		t.Fatal("a 50-op proof is nowhere near the ceiling and must not be refused as too complex")
	}

	// Over it: refused, and refused for THIS reason.
	_, err := parseProof(build(600)) // 600 + 600*600 = 360,600 instructions
	if err == nil {
		t.Fatal("an amplifying proof parsed successfully — the bound is not in force")
	}
	if !errors.Is(err, ErrProofTooComplex) {
		t.Fatalf("refused for the wrong reason: got %v, want %v", err, ErrProofTooComplex)
	}

	// The positive control: a REAL proof built on checkpoints still parses. The
	// bound must refuse the amplification, not the feature.
	if _, err := parseProof(mustB64(t, merkle1OTS)); err != nil {
		t.Fatalf("the bound refuses a legitimate checkpoint-using proof: %v", err)
	}
}
