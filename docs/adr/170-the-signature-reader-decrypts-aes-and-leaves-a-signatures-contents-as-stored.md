# ADR-170 — the signature reader decrypts AES, and leaves a signature's `/Contents` as stored

**Status:** accepted
**Date:** 2026-10-13
**Extends:** ADR-066 and ADR-068 (the patched `third_party/digitorus-pdf`, now NOTICE.nib divergence 10).
**Context:** /pending 889 part 6. Three of 203 real signed documents — a real-estate forms engine's output,
AES-128 encrypted to forbid changes, opening with no password — could not be verified at all: the reader
decrypted RC4 and panicked "AES not implemented" at the first AES string, and refused AES-256 outright. v1.216.3
made the verdict honest (`unchecked: encrypted`); this makes it a verdict. Measured on the three: the signature's
`/Contents` is stored as plaintext DER, every other string of the signature dictionary is encrypted, and a detached
PKCS#7 check over the stored `/ByteRange` bytes passes.

Grilled 2026-10-13 (`memory/instruments/889-three-decisions.md`). The decision as first written would have read the
documents and then failed: the reader decrypted EVERY string of an indirect object (`lex.go`), the signature's
`/Contents` included, so an AES-aware reader turns the signature into noise. And nib's signer had refused encrypted
input only because this reader could not read it.

## Decision

1. **The reader decrypts AES-128 (V 4, AESV2) and AES-256 (V 5, AESV3, revisions 5 and 6)** with Go's
   `crypto/aes`, by ISO 32000-2's algorithms: the per-object key with "sAlT", the file key itself for AESV3,
   algorithm 2.B's hash at revision 6, the user then the owner password, `/UE` or `/OE` unwrapped. A string is a
   vector and padded CBC blocks; a stream's padding is removed. A malformed string is a panic carrying
   `ErrMalformedAESString`, contained where every reader panic is. A vector with no block after it is read as the
   empty string — a real forms engine writes an empty `/Name` that way.
2. **The hash stays over the stored bytes.** Only the walk that FINDS a signature is decrypted; pdfsign's verify
   already hashes the file by `/ByteRange` (`io.NewSectionReader` over the file), and nothing moves that.
3. **A signature's `/Contents` is left as stored**, as ISO 32000-2 §7.6.2 requires: a dictionary with `/Type /Sig`
   or `/DocTimeStamp` (pdfcpu's test) OR a `/ByteRange` (nib's sweep's) — the union, so neither reading can meet a
   signature this reader decrypted. Decided once the dictionary is read whole (`readDict`), because whether it is a
   signature's is not known at the string. The `/Encrypt` dictionary's own strings are not decrypted either: a real
   document's signature was otherwise read as a malformed string, because the dictionary is resolved again later.
4. **Every signing path refuses an encrypted document** (`ErrEncryptedDocument`, in `runSign`, the one door; 400
   from Finalize). pdfsign writes its signature and the catalog it re-emits unencrypted into it, and an xref stream
   with no `/Encrypt`. It was refused before only by accident.
5. Two upstream defects in the same functions are fixed: a crypt filter `/Length` in bits (pdfcpu writes 128) as
   well as bytes (16), and the per-object key truncated to n+5 bytes — 40-bit RC4 streams decrypted to noise.

## Refused

**Decrypting with pdfcpu first and handing the reader the plaintext.** The `/ByteRange` is over the ENCRYPTED
bytes, so the reader would locate and hash a file that is not the one signed.

## Consequences

- The three real documents read valid, naming their signer (`Authentisign Security`, `ETSI.CAdES.detached`).
- `unchecked: encrypted` stays for what the reader still does not decrypt: a V 4 RC4 filter, a custom handler.
- **Declared:** `/Metadata` under `/EncryptMetadata false`, a cross-reference stream read after the key is set, and
  `/Identity` or `/Crypt` stream filters are still decrypted (no signature path reads them — an exemption for the
  cross-reference stream was written and no test or real document could reach it, so it was not kept), and revision 4's `/EncryptMetadata false` key step is not taken.
  Nib cannot SIGN an encrypted document. The fourth real document (`disclosure_full.pdf`) is still refused by
  pdfcpu before the reader runs (a crypt filter with no `/Length`, /pending 889 part 3).
- Proof: `internal/sign/aesverify_test.go` — encrypted-then-signed fixtures at both lengths read valid and invalid
  with one covered byte changed; the reader hands a signature's `/Contents` on as stored; signing is refused; a
  40-bit RC4 stream reads as written. The root guard holds the change in the copy.
