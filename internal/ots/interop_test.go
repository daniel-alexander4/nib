package ots

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"sort"
	"testing"
)

// Real OpenTimestamps proofs from the reference javascript-opentimestamps
// examples, used to prove Nib's serializer (serialize/serializeSequence) emits
// proofs that external OTS tools accept. Interop was confirmed out-of-band
// (2026-06-14) by running javascript-opentimestamps over Nib's serialize()
// output: byte-identical for the confirmed proof, and an equivalent reconstructed
// tree for the shared-prefix proof. These tests lock that property in pure Go,
// with no external dependency.
const (
	// helloWorldOTS — a Bitcoin-confirmed proof whose path uses a ripemd160 op,
	// so Nib's compute() can't verify it (see the op-coverage note in verify.go),
	// but it parses and serializes BYTE-IDENTICALLY — which is exactly what proves
	// Nib's encoder reproduces the reference wire format with no drift.
	helloWorldOTS = "AE9wZW5UaW1lc3RhbXBzAABQcm9vZgC/ieLohOiSlAEIA7ogTlDRJuRnTABeBNguhMITZngK8fQ71Uo3gWtqs0AD8cgBAQAAAAHkgvnTLsw7ple2nYmAEIV7VEV6kEl5gv9W+XxOxY5vmAEAAABrSDBFAiEAslOt0dHPkIRDOKR1oE/xP8nnvSQrB3Yt6gf1YIst42cCIACyaMqcM0KzdpzdBiiRMXzc74eqwxC2hV6dk4mOu+jsASECDY5NEH0rM5sAUO/dS0oJJFqgVgSPElOWN06moqsHCcb/////AmUz5gUAAAAAGXapFAvwV9QPu6Z0SGJRX1tVojEN5XcviKyghgEAAAAAABl2qRTwBoisAAAAAAgI8SCph/cWxTORPDFMeONdNYhMrJQ/pCysSdKyxp9AA/hfiAgI8SDexVs0h+Hj9yKkm1WneDIVhieF9KOss5KEYBn3HcZKnQgI8SCyyhj0heCAR44CXas9RktBbA4ey2Ypya786MghTQQkMggI8CARsOkGYRlv9LCBPD7aFBurXpFgSDe996DJ3zfbDjoRmAgI8CDDS8GkoQk//RSMAWseZkdCkU6Tnvq+TT01ZRWRSybZ4ggI8CDD5ufDjGn2ryTCvjTrrEglft5h7AohuVNeREMne+MGRggI8SAHmL+GBuAAJOXV1UvwyWD2Kd+52taRV0VbbyZSwOjegQgI8CA/mtptYLqiRABrsKrVFEitL6+51LZIegmZz/JrkfD1NggI8SDHAwGelZqN0/rvdIm7MoukhVdHWOcJHwFGTrZYcsl1yAgI8CDL/v/1E/+EuRXj/tb515lnZjD4Nk6ipsdVf62UpbXXiAgI8SAL4jcJhZkTur1EYLvd+O0hPnyHc6Sx+s4w+Kz98JO3BQgIAAWIlg1z1xkBA/fvFQ=="
	// merkle1OTS — a pending proof with two calendar branches sharing an op prefix
	// (a merkle tree). Nib flattens it into two independent digest-rooted sequences
	// on parse; re-serialized it is LARGER but semantically equivalent, and the
	// reference tool reconstructs the same digest, calendars, and commitments.
	merkle1OTS = "AE9wZW5UaW1lc3RhbXBzAABQcm9vZgC/ieLohOiSlAEI0y/umoJ/Wg1YD4C+t+3OZi3Zn81lkeTvimJEQD3wt8nwEAIBo2rAxzaS0c7cq/8GCesI8CCfQphMzblq3beYhOcUzGtrBZxo8qw6LgfFGYwi2QTeFAjwIAJjVueXLwI5MOyEwhOt7cQFBGCXOTW70vTfPXvV3sVfCP/wEC4SBQr9ehDqT1ke1xfTXeYI8QRX2YLf8Aix8m4uVVkEdwCD3+MNLvkMji4taHR0cHM6Ly9hbGljZS5idGMuY2FsZW5kYXIub3BlbnRpbWVzdGFtcHMub3Jn8BBKqt6cL/uFPM/5wHaB0Bn9CPEEV9mC4PAIZkTvcTBxdioAg9/jDS75DI4sK2h0dHBzOi8vYm9iLmJ0Yy5jYWxlbmRhci5vcGVudGltZXN0YW1wcy5vcmc="
)

func mustB64(t *testing.T, s string) []byte {
	t.Helper()
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	return b
}

// TestSerializeByteIdenticalRealProof proves Nib's serializer reproduces a real,
// reference-encoded confirmed proof byte-for-byte — so an external OTS tool that
// accepts the original accepts Nib's re-serialization identically.
func TestSerializeByteIdenticalRealProof(t *testing.T) {
	orig := mustB64(t, helloWorldOTS)
	p, err := parseProof(orig)
	if err != nil {
		t.Fatalf("parse real confirmed proof: %v", err)
	}
	ser := serialize(p.digest, p.seqs)
	if !bytes.Equal(ser, orig) {
		t.Fatalf("serialize not byte-identical: in=%d out=%d", len(orig), len(ser))
	}
	if _, err := parseProof(ser); err != nil {
		t.Fatalf("reparse: %v", err)
	}
}

// TestSerializeFlattenedRealProof proves that for a foreign proof whose branches
// share an op prefix, Nib's flattened re-serialization is a different (larger)
// byte string but a SEMANTICALLY equivalent proof: same digest and the same
// multiset of per-branch commitments — which is why external tools still accept it.
func TestSerializeFlattenedRealProof(t *testing.T) {
	commits := func(b []byte) []string {
		p, err := parseProof(b)
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		var out []string
		for _, s := range p.seqs {
			c, err := s.compute(p.digest)
			if err != nil {
				t.Fatalf("compute: %v", err)
			}
			out = append(out, hex.EncodeToString(c))
		}
		sort.Strings(out)
		return out
	}

	orig := mustB64(t, merkle1OTS)
	p, err := parseProof(orig)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	ser := serialize(p.digest, p.seqs)
	if bytes.Equal(ser, orig) {
		t.Fatal("expected a flattened (non-identical) serialization for a shared-prefix proof")
	}
	if len(ser) <= len(orig) {
		t.Fatalf("flattened proof should be larger: in=%d out=%d", len(orig), len(ser))
	}
	got, want := commits(ser), commits(orig)
	if len(got) != len(want) {
		t.Fatalf("branch count changed: %d -> %d", len(want), len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("commitment multiset changed:\n orig %v\n ser  %v", want, got)
		}
	}
}

// TestComputeRIPEMD160 locks the ripemd160 op against the published RIPEMD-160
// test vectors, proving compute() executes it correctly independent of any proof.
func TestComputeRIPEMD160(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"", "9c1185a5c5e9fc54612808977ee8f548b2258d31"},
		{"abc", "8eb208f7e05d987a9b044a8e98c6b087f15a0bfc"},
	} {
		got, err := (sequence{ops: []op{{tag: opRIPEMD160}}}).compute([]byte(tc.in))
		if err != nil {
			t.Fatalf("compute ripemd160(%q): %v", tc.in, err)
		}
		if hex.EncodeToString(got) != tc.want {
			t.Fatalf("ripemd160(%q) = %s, want %s", tc.in, hex.EncodeToString(got), tc.want)
		}
	}
}

// TestComputeRealRipemd160Proof verifies that compute() now resolves the real,
// Bitcoin-confirmed helloWorldOTS proof — whose path hashes with ripemd160, the
// op Nib previously rejected. The expected merkle root is the value compute()
// derives for the proof's Bitcoin block (height 358391), locking it against
// regression; ripemd160's correctness itself is proven by TestComputeRIPEMD160.
func TestComputeRealRipemd160Proof(t *testing.T) {
	p, err := parseProof(mustB64(t, helloWorldOTS))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(p.seqs) != 1 {
		t.Fatalf("want 1 sequence, got %d", len(p.seqs))
	}
	root, err := p.seqs[0].compute(p.digest)
	if err != nil {
		t.Fatalf("compute (ripemd160 path): %v", err)
	}
	const wantRoot = "007ee445d23ad061af4a36b809501fab1ac4f2d7e7a739817dd0cbb7ec661b8a"
	if got := hex.EncodeToString(root); got != wantRoot {
		t.Fatalf("merkle root = %s, want %s", got, wantRoot)
	}
	if p.seqs[0].height != 358391 {
		t.Fatalf("block height = %d, want 358391", p.seqs[0].height)
	}
}
