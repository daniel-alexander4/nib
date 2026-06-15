package ots

import (
	"bytes"
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ripemd160"
	"golang.org/x/crypto/sha3"

	"nib/internal/safe"
)

// Verification states reported to the caller.
const (
	StateConfirmed = "confirmed" // proof matches the document and a real Bitcoin block
	StatePending   = "pending"   // not yet anchored to a block; calendar hasn't confirmed
	StateMismatch  = "mismatch"  // the .ots is for a different document (digest differs)
	StateInvalid   = "invalid"   // proof's computed root doesn't match the attested block
)

// DefaultExplorers are the public Esplora-API block sources used to look up an
// attested block's header. They are run by independent operators; all are queried
// and at least two must agree on the block (see defaultMinAgree), so a single
// lying or compromised explorer can't spoof a verification while one being down
// doesn't break it. Verification sends only a public block height to these —
// never the document or its hash. A user pointing Nib at their own Esplora
// endpoint overrides this set and is trusted on its own (minAgree 1).
var DefaultExplorers = []string{
	"https://blockstream.info/api",
	"https://mempool.space/api",
	"https://mempool.emzy.de/api",
}

// defaultMinAgree is how many of the DefaultExplorers must return the same block
// header before a result is trusted. Two keeps the "no single explorer can spoof"
// guarantee while tolerating one of the three being unreachable.
const defaultMinAgree = 2

// bitcoinMagic tags a Bitcoin block-header attestation in the .ots format.
var bitcoinMagic = []byte{0x05, 0x88, 0x96, 0x0d, 0x73, 0xd7, 0x19, 0x01}

// op tags we execute. Nib's own proofs (stamped via the standard calendars) take
// a sha256-only path to Bitcoin, but third-party proofs may hash with any of the
// spec's crypto ops, so compute handles all four — sha256, ripemd160, sha1, and
// keccak256 — plus append/prepend. keccak256 (tag 0x67) is the *legacy* Keccak-256
// (Ethereum-style), not NIST SHA3-256, pinned against python-opentimestamps's
// OpKECCAK256 (Cryptodome keccak, digest_bits=256). These hash ops are size-safe:
// each output is ≤32 bytes and append/prepend grow only by parsed argument bytes,
// so a proof can't blow up memory during compute.
//
// The parser also tolerates two transform ops, reverse (0xf2) and hexlify (0xf3),
// that compute deliberately does NOT execute — it returns a clear "unsupported
// operation" error instead. reverse is pending removal upstream and hexlify is
// effectively unused; more to the point, hexlify *doubles* its input, so executing
// it without a message-length cap (as the reference enforces) would let a tiny
// .ots encode an exponential-size intermediate — a DoS the four hash ops can't
// cause. If a real proof ever needs hexlify, add it with that bound, not without.
const (
	opAppend    = 0xf0
	opPrepend   = 0xf1
	opRIPEMD160 = 0x03
	opSHA1      = 0x02
	opKeccak256 = 0x67
)

// VerifyResult is the outcome of checking an .ots proof against a document.
type VerifyResult struct {
	State   string
	Height  uint64
	Time    time.Time
	Sources int
	// Upgraded is a self-contained .ots — the input proof with the calendar
	// commitment folded into its Bitcoin attestation — set only when verification
	// confirmed the proof AND an in-memory upgrade was performed this run, so it
	// can be persisted and later verified without any calendar. Nil otherwise.
	Upgraded []byte
}

// BlockSource resolves an attested Bitcoin block height to its header's merkle
// root and timestamp.
type BlockSource interface {
	BlockHeader(ctx context.Context, height uint64) (merkleRoot []byte, t time.Time, err error)
}

// VerifyProof checks an .ots proof against a document digest. It confirms the
// proof is for this document, upgrades any still-pending calendar commitments in
// memory, then validates the Bitcoin attestation against the block sources,
// requiring at least minAgree of them to return the same block header (and any
// that respond to agree). Network/parse failures return an error; the expected
// outcomes are returned as a VerifyResult state.
//
// client fetches the calendar URL embedded in the (untrusted) .ots during an
// upgrade; the caller must give one that refuses non-public addresses, since that
// URL is attacker-controllable — see internal/server untrustedFetchClient.
func VerifyProof(ctx context.Context, client *http.Client, sources []BlockSource, minAgree int, proofBytes []byte, docDigest [32]byte) (*VerifyResult, error) {
	p, err := parseProof(proofBytes)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(p.digest, docDigest[:]) {
		return &VerifyResult{State: StateMismatch}, nil
	}

	var attested []sequence
	updated := make([]sequence, 0, len(p.seqs)) // every sequence, upgraded where possible
	pending := false
	upgradedAny := false
	for _, s := range p.seqs {
		switch {
		case s.height != 0:
			attested = append(attested, s)
			updated = append(updated, s)
		case s.calURL != "":
			pending = true
			if up, ok, err := upgrade(ctx, client, s, p.digest); err == nil && ok {
				attested = append(attested, up)
				updated = append(updated, up)
				upgradedAny = true
			} else {
				updated = append(updated, s) // calendar hasn't confirmed yet — keep pending
			}
		}
	}
	if len(attested) == 0 {
		if pending {
			return &VerifyResult{State: StatePending}, nil
		}
		return nil, errors.New("proof carries no Bitcoin attestation")
	}

	s := attested[0]
	root, err := s.compute(p.digest)
	if err != nil {
		return nil, err
	}
	merkle, t, n, err := fetchAgreedHeader(ctx, sources, minAgree, s.height)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(root, merkle) {
		return &VerifyResult{State: StateInvalid}, nil
	}
	res := &VerifyResult{State: StateConfirmed, Height: s.height, Time: t, Sources: n}
	if upgradedAny {
		res.Upgraded = serialize(p.digest, updated) // a now self-contained proof to persist
	}
	return res, nil
}

// serialize emits the .ots bytes for a proof: the standard preamble plus each
// sequence's operations and terminating attestation, glued with the checkpoint
// byte (via buildOTS). It is the inverse of parseProof — Nib flattens proofs into
// independent digest-rooted sequences on parse, so re-emitting each one and
// gluing them reproduces a valid, spec-canonical proof.
func serialize(digest []byte, seqs []sequence) []byte {
	parts := make([][]byte, len(seqs))
	for i, s := range seqs {
		parts[i] = serializeSequence(s)
	}
	var d [32]byte
	copy(d[:], digest)
	return buildOTS(d, parts)
}

// serializeSequence emits one sequence: its operations followed by a single
// pending (calendar URL) or Bitcoin (block height) attestation.
func serializeSequence(s sequence) []byte {
	var b []byte
	for _, o := range s.ops {
		b = append(b, o.tag)
		if o.tag == opAppend || o.tag == opPrepend {
			b = appendVarbytes(b, o.arg)
		}
	}
	b = append(b, tagAttestation)
	if s.height != 0 {
		b = append(b, bitcoinMagic...)
		b = appendVarbytes(b, putVaruint(s.height))
	} else {
		b = append(b, pendingMagic...)
		b = appendVarbytes(b, appendVarbytes(nil, []byte(s.calURL)))
	}
	return b
}

// fetchAgreedHeader queries every source concurrently and requires at least
// minAgree of them to return the same block merkle root: fewer than minAgree
// responding is untrustworthy (can't cross-check), and any disagreement among
// those that did respond is treated as untrustworthy too.
func fetchAgreedHeader(ctx context.Context, sources []BlockSource, minAgree int, height uint64) ([]byte, time.Time, int, error) {
	type res struct {
		merkle []byte
		t      time.Time
	}
	var (
		mu      sync.Mutex
		results []res
		wg      sync.WaitGroup
	)
	for _, src := range sources {
		wg.Add(1)
		go func(src BlockSource) {
			defer wg.Done()
			defer safe.Recover("ots block-header fetch")
			m, t, err := src.BlockHeader(ctx, height)
			if err != nil {
				return
			}
			mu.Lock()
			results = append(results, res{m, t})
			mu.Unlock()
		}(src)
	}
	wg.Wait()

	if len(results) < minAgree {
		return nil, time.Time{}, 0, fmt.Errorf("only %d of %d block explorers confirmed the block; need %d to agree — try again", len(results), len(sources), minAgree)
	}
	for _, r := range results[1:] {
		if !bytes.Equal(r.merkle, results[0].merkle) {
			return nil, time.Time{}, 0, errors.New("block explorers disagree on the block — refusing to trust")
		}
	}
	return results[0].merkle, results[0].t, len(results), nil
}

// upgrade folds a pending calendar commitment into a complete Bitcoin-attested
// sequence by asking the calendar for the path to a block. It works in memory and
// does not persist the upgraded proof. ok is false if the calendar has not yet
// confirmed the commitment (still pending).
func upgrade(ctx context.Context, client *http.Client, s sequence, digest []byte) (sequence, bool, error) {
	commitment, err := s.compute(digest)
	if err != nil {
		return s, false, err
	}
	ctx, cancel := context.WithTimeout(ctx, perCalendarTimeout)
	defer cancel()

	// calURL comes from the untrusted .ots file. Reject anything but http(s) here
	// (a defence-in-depth scheme guard); the no-private-address guard lives in the
	// caller's client (untrustedFetchClient), which also covers redirects.
	cu, err := url.Parse(s.calURL)
	if err != nil {
		return s, false, err
	}
	if cu.Scheme != "http" && cu.Scheme != "https" {
		return s, false, fmt.Errorf("calendar URL has unsupported scheme %q", cu.Scheme)
	}

	reqURL := strings.TrimRight(s.calURL, "/") + "/timestamp/" + hex.EncodeToString(commitment)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return s, false, err
	}
	req.Header.Set("Accept", "application/vnd.opentimestamps.v1")
	req.Header.Set("User-Agent", "nib")
	resp, err := client.Do(req)
	if err != nil {
		return s, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return s, false, nil // not yet confirmed
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return s, false, err
	}
	tails, err := parseSequences(&cursor{b: body})
	if err != nil {
		return s, false, err
	}
	for _, t := range tails {
		if t.height != 0 {
			return sequence{ops: append(append([]op{}, s.ops...), t.ops...), height: t.height}, true, nil
		}
	}
	return s, false, nil // calendar responded but no block attestation yet
}

// --- .ots structure -------------------------------------------------------

type op struct {
	tag byte
	arg []byte
}

// sequence is a chain of operations applied to the file digest, terminating in
// one attestation: a pending calendar URL or a Bitcoin block height.
type sequence struct {
	ops    []op
	calURL string
	height uint64
}

type proof struct {
	digest []byte
	seqs   []sequence
}

// compute applies the sequence's operations to the digest and returns the result
// — the calendar commitment (pending) or the block merkle root (Bitcoin).
func (s sequence) compute(digest []byte) ([]byte, error) {
	cur := append([]byte{}, digest...)
	for _, o := range s.ops {
		switch o.tag {
		case opAppend:
			cur = append(append([]byte{}, cur...), o.arg...)
		case opPrepend:
			cur = append(append([]byte{}, o.arg...), cur...)
		case opSHA256:
			h := sha256.Sum256(cur)
			cur = h[:]
		case opRIPEMD160:
			h := ripemd160.New()
			h.Write(cur)
			cur = h.Sum(nil)
		case opSHA1:
			h := sha1.Sum(cur)
			cur = h[:]
		case opKeccak256:
			h := sha3.NewLegacyKeccak256() // legacy Keccak, not NIST SHA3-256
			h.Write(cur)
			cur = h.Sum(nil)
		default:
			return nil, fmt.Errorf("proof uses unsupported operation 0x%02x", o.tag)
		}
	}
	return cur, nil
}

func parseProof(b []byte) (*proof, error) {
	c := &cursor{b: b}
	magic, err := c.read(len(headerMagic))
	if err != nil || !bytes.Equal(magic, headerMagic) {
		return nil, errors.New("not an OpenTimestamps (.ots) file")
	}
	ver, err := c.varuint()
	if err != nil || ver != 1 {
		return nil, errors.New("unsupported .ots version")
	}
	fileOp, err := c.byte()
	if err != nil || fileOp != opSHA256 {
		return nil, errors.New("unsupported .ots file digest (only sha256)")
	}
	digest, err := c.read(32)
	if err != nil {
		return nil, errors.New("truncated .ots digest")
	}
	seqs, err := parseSequences(c)
	if err != nil {
		return nil, err
	}
	return &proof{digest: append([]byte{}, digest...), seqs: seqs}, nil
}

// parseSequences walks the operation/attestation/checkpoint encoding into a set
// of independent sequences (the checkpoint byte 0xff resets to a shared prefix),
// mirroring the reference OpenTimestamps parser.
func parseSequences(c *cursor) ([]sequence, error) {
	type instr struct {
		isAtt  bool
		tag    byte
		arg    []byte
		calURL string
		height uint64
	}
	blocks := [][]instr{{}}
	var checkpoints [][]instr
	cur := 0

	for {
		tag, err := c.byte()
		if err != nil {
			break // EOF
		}
		switch tag {
		case tagAttestation:
			magic, err := c.read(len(pendingMagic))
			if err != nil {
				return nil, err
			}
			payload, err := c.varbytes()
			if err != nil {
				return nil, err
			}
			ab := &cursor{b: payload}
			var in instr
			in.isAtt = true
			switch {
			case bytes.Equal(magic, pendingMagic):
				u, err := ab.varbytes()
				if err != nil {
					return nil, err
				}
				in.calURL = string(u)
			case bytes.Equal(magic, bitcoinMagic):
				h, err := ab.varuint()
				if err != nil {
					return nil, err
				}
				in.height = h
			default:
				return nil, fmt.Errorf("unsupported attestation type 0x%x", magic)
			}
			blocks[cur] = append(blocks[cur], in)
			if n := len(checkpoints); n > 0 {
				blocks = append(blocks, checkpoints[n-1])
				checkpoints = checkpoints[:n-1]
				cur++
			}
		case tagCheckpoint:
			b := blocks[cur]
			cp := make([]instr, len(b))
			copy(cp, b)
			checkpoints = append(checkpoints, cp)
		default:
			in := instr{tag: tag}
			if tag == opAppend || tag == opPrepend {
				arg, err := c.varbytes()
				if err != nil {
					return nil, err
				}
				in.arg = arg
			} else if !knownNoArgOp(tag) {
				return nil, fmt.Errorf("unknown operation tag 0x%02x", tag)
			}
			blocks[cur] = append(blocks[cur], in)
		}
	}

	var out []sequence
	for _, b := range blocks {
		if len(b) == 0 {
			continue
		}
		var s sequence
		for i, in := range b {
			if in.isAtt {
				if i != len(b)-1 {
					return nil, errors.New("attestation not at end of sequence")
				}
				s.calURL, s.height = in.calURL, in.height
			} else {
				s.ops = append(s.ops, op{in.tag, in.arg})
			}
		}
		if s.calURL == "" && s.height == 0 {
			return nil, errors.New("sequence has no attestation")
		}
		out = append(out, s)
	}
	return out, nil
}

func knownNoArgOp(tag byte) bool {
	switch tag {
	case 0x02, 0x03, 0x08, 0x67, 0xf2, 0xf3: // sha1, ripemd160, sha256, keccak256, reverse, hexlify
		return true
	}
	return false
}

// --- Esplora block source -------------------------------------------------

// NewEsplora returns a BlockSource backed by an Esplora-API endpoint (e.g.
// https://blockstream.info/api). It fetches the raw 80-byte block header and
// reads the merkle root (bytes 36–68) and timestamp (little-endian uint32 at
// 68–72) directly — no Bitcoin library required.
func NewEsplora(base string, client *http.Client) BlockSource {
	return esplora{base: strings.TrimRight(base, "/"), client: client}
}

type esplora struct {
	base   string
	client *http.Client
}

func (e esplora) BlockHeader(ctx context.Context, height uint64) ([]byte, time.Time, error) {
	hashHex, err := e.get(ctx, e.base+"/block-height/"+strconv.FormatUint(height, 10))
	if err != nil {
		return nil, time.Time{}, err
	}
	hdrHex, err := e.get(ctx, e.base+"/block/"+strings.TrimSpace(string(hashHex))+"/header")
	if err != nil {
		return nil, time.Time{}, err
	}
	hdr, err := hex.DecodeString(strings.TrimSpace(string(hdrHex)))
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("malformed block header: %w", err)
	}
	if len(hdr) < 80 {
		return nil, time.Time{}, errors.New("short block header")
	}
	merkle := append([]byte{}, hdr[36:68]...)
	t := time.Unix(int64(binary.LittleEndian.Uint32(hdr[68:72])), 0).UTC()
	return merkle, t, nil
}

func (e esplora) get(ctx context.Context, url string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, perCalendarTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s returned %d", url, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
}
