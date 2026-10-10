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
// and at least two must agree on the block (see DefaultMinAgree), so a single
// lying or compromised explorer can't spoof a verification while one being down
// doesn't break it. Verification sends only a public block height to these —
// never the document or its hash. A user pointing Nib at their own Esplora
// endpoint overrides this set and is trusted on its own (minAgree 1).
var DefaultExplorers = []string{
	"https://blockstream.info/api",
	"https://mempool.space/api",
	"https://mempool.emzy.de/api",
}

// DefaultMinAgree is how many of the DefaultExplorers must return the same block
// header before a result is trusted. Two keeps the "no single explorer can spoof"
// guarantee while tolerating one of the three being unreachable.
// **Exported at /pending 425, because the rule had two implementations and the named one
// was unreachable.** `internal/server/timestamp.go` applied the same policy as a bare `2`
// with its own comment, so this constant was referenced only by the two comments above it
// and changing it changed nothing. Exporting it and calling it from there is the ADR-009
// shape: one rule, one door, every site calling it.
const DefaultMinAgree = 2

// bitcoinMagic tags a Bitcoin block-header attestation in the .ots format.
var bitcoinMagic = []byte{0x05, 0x88, 0x96, 0x0d, 0x73, 0xd7, 0x19, 0x01}

// ErrProofTooComplex refuses a .ots whose operation count exceeds
// maxProofInstructions — see that constant for why one byte can commit thousands.
// Exported and sentinel rather than a bare string so a caller can tell "this proof
// is hostile" from "this proof is malformed", and so the negative test can assert
// the specific refusal rather than being satisfied by any error at all.
var ErrProofTooComplex = errors.New("proof is too complex to verify safely")

// maxPendingUpgrades bounds the calendar fetches one proof can force. A stamp nib or the reference
// client makes carries one pending commitment per calendar — four, today (DefaultCalendars) — so
// eight admits a merged proof and refuses a file whose only purpose is the fan-out.
const maxPendingUpgrades = 8

// op tags we execute. Nib's own proofs (stamped via the standard calendars) take
// a sha256-only path to Bitcoin, but third-party proofs may hash with any of the
// spec's crypto ops, so compute handles all four — sha256, ripemd160, sha1, and
// keccak256 — plus append/prepend. keccak256 (tag 0x67) is the *legacy* Keccak-256
// (Ethereum-style), not NIST SHA3-256, pinned against python-opentimestamps's
// OpKECCAK256 (Cryptodome keccak, digest_bits=256). Each hash output is ≤32 bytes;
// what bounds append/prepend is maxOpResult, below — not the argument cap, because
// the RESULT is the running message and every op copies it (/pending 781).
//
// The parser also tolerates two transform ops, reverse (0xf2) and hexlify (0xf3),
// that compute deliberately does NOT execute — it returns a clear "unsupported
// operation" error instead, and Stamp refuses a calendar response that uses them
// (executable is the one list). reverse is pending removal upstream and hexlify is
// effectively unused. If a real proof ever needs one, add it to executable and to
// compute; maxOpResult already bounds hexlify's doubling.
const (
	opAppend    = 0xf0
	opPrepend   = 0xf1
	opRIPEMD160 = 0x03
	opSHA1      = 0x02
	opKeccak256 = 0x67
)

// maxOpResult is the longest message any operation may take or produce, in bytes:
// python-opentimestamps' Op.MAX_RESULT_LENGTH and MAX_MSG_LENGTH, both 4096, read
// from opentimestamps/core/op.py at master 3af46432. The reference raises
// MsgValueError past it and computes every result while DESERIALIZING, so a proof
// that crosses it is not one the reference will even load. Its binary-op argument
// is varbytes of 1..MAX_RESULT_LENGTH, which readOpArg mirrors.
//
// /pending 781: the parser capped each argument (64 KiB) and the op count
// (100,000) but not the result, and compute copies the running message on every
// append and prepend — sixteen 64 KiB appends then 8,000 one-byte ones, a 1 MB
// file, cost 1.8 s of CPU before any network, consulting no deadline. The cost is
// ops × message length, so bounding the message is what bounds the work.
const maxOpResult = 4096

// executable reports whether compute walks tag. It is the one list: compute refuses
// anything else, and Stamp accepts from a calendar only what this admits, so nib
// never writes a .ots it cannot verify itself (/pending 807).
func executable(tag byte) bool {
	switch tag {
	case opAppend, opPrepend, opSHA256, opRIPEMD160, opSHA1, opKeccak256:
		return true
	}
	return false
}

// ErrOpResultTooLong refuses an operation whose message or result would exceed
// maxOpResult. It wraps ErrProofTooComplex: a hostile shape, not a malformed one.
var ErrOpResultTooLong = fmt.Errorf("%w: an operation's result is longer than %d bytes",
	ErrProofTooComplex, maxOpResult)

// opResultLen is the length o produces from a message of msgLen bytes, refusing
// one past maxOpResult. It is the ONE door for that bound: the parse-time walk
// (checkLengths) and compute both ask it, compute before it allocates. reverse and
// hexlify get lengths so a branch using them is still checked at parse, though
// compute will not run it.
func opResultLen(msgLen int, o op) (int, error) {
	var n int
	switch o.tag {
	case opAppend, opPrepend:
		n = msgLen + len(o.arg)
	case opSHA256, opKeccak256:
		n = 32
	case opRIPEMD160, opSHA1:
		n = 20
	case 0xf2: // reverse
		n = msgLen
	case 0xf3: // hexlify
		n = 2 * msgLen
	default:
		return 0, fmt.Errorf("unknown operation tag 0x%02x", o.tag)
	}
	if msgLen > maxOpResult || n > maxOpResult {
		return 0, ErrOpResultTooLong
	}
	return n, nil
}

// checkLengths walks s's lengths from the 32-byte file digest without computing
// anything, so a proof whose results cross maxOpResult is refused when it is read —
// as the reference refuses it — rather than one branch at a time inside compute.
func (s sequence) checkLengths() error {
	n := sha256.Size
	for _, o := range s.ops {
		var err error
		if n, err = opResultLen(n, o); err != nil {
			return err
		}
	}
	return nil
}

// readOpArg reads a binary op's argument as the reference does, which refuses an
// empty one. Its upper bound, 1..MAX_RESULT_LENGTH there, needs no check of its own
// here: the message is never shorter than 20 bytes, so any argument past
// maxOpResult-20 already fails opResultLen. The proof parser and Stamp's calendar
// check both read through it.
func readOpArg(c *cursor) ([]byte, error) {
	arg, err := c.varbytes()
	if err != nil {
		return nil, err
	}
	if len(arg) == 0 {
		return nil, errors.New("an append or prepend argument is empty")
	}
	return arg, nil
}

// computeCheckEvery is how many operations compute runs between looks at its
// context — at ≤ maxOpResult bytes an op, about a MiB of work.
const computeCheckEvery = 256

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

	// **The upgrade loop is bounded before it sends anything** (/pending 502).
	//
	// Every pending sequence is one sequential GET to the calendar URL the (untrusted) file names,
	// and the parser admits up to maxProofInstructions of them: 500 were reproduced from a 17 KB
	// `.ots`, all to one host. maxAttestationsExamined below bounded the explorer half of this
	// function and left this half open. Counted up front rather than as attempts, so a hostile
	// file costs no request at all — and refused rather than truncated, because silently skipping
	// the sequences past a cap would let eight bogus calendars placed first report a genuine proof
	// as "pending", which is the prepend denial the attestation loop already refuses to allow.
	nPending := 0
	for _, s := range p.seqs {
		if s.height == 0 && s.calURL != "" {
			nPending++
		}
	}
	if nPending > maxPendingUpgrades {
		return nil, fmt.Errorf("%w: it carries %d pending calendar commitments and at most %d are "+
			"fetched", ErrProofTooComplex, nPending, maxPendingUpgrades)
	}

	// The instructions an upgrade may splice in are charged against the proof's own
	// maxProofInstructions, not granted afresh per calendar (/pending 781): each calendar's tail
	// is parsed under its own 100,000 cap, so eight of them stacked eight times the bound onto a
	// file that had already used it.
	budget := maxProofInstructions
	for _, s := range p.seqs {
		budget -= len(s.ops)
	}

	var attested []sequence
	updated := make([]sequence, 0, len(p.seqs)) // every sequence, upgraded where possible
	pending := false
	upgradedAny := false
	// "Pending" is a calendar's answer, so it is reported only when one GAVE it (/pending 813).
	// An upgrade that failed — DNS, refused, TLS, its own 20 s timeout, a tail nib refuses — is
	// no answer at all, and folding it into "not yet" told the user their proof is unconfirmed
	// when nib had simply failed to ask.
	answeredNotYet := false
	var upgradeErr error
	for _, s := range p.seqs {
		switch {
		case s.height != 0:
			attested = append(attested, s)
			updated = append(updated, s)
		case s.calURL != "":
			pending = true
			up, ok, err := upgrade(ctx, client, s, p.digest, budget)
			switch {
			case err != nil:
				upgradeErr = err
				updated = append(updated, s)
			case ok:
				budget -= len(up.ops) - len(s.ops)
				attested = append(attested, up)
				updated = append(updated, up)
				upgradedAny = true
			default:
				answeredNotYet = true
				updated = append(updated, s) // calendar hasn't confirmed yet — keep pending
			}
		}
	}
	// An upgrade that failed because the caller's deadline passed is not a calendar saying "not
	// yet": reporting it as pending would tell the user their proof is unconfirmed when nib simply
	// ran out of time asking.
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("verification ran out of time: %w", err)
	}
	if len(attested) == 0 {
		if pending {
			if !answeredNotYet && upgradeErr != nil {
				return nil, fmt.Errorf("could not ask the calendar server about this proof: %w", upgradeErr)
			}
			return &VerifyResult{State: StatePending}, nil
		}
		return nil, errors.New("proof carries no Bitcoin attestation")
	}

	// Every attestation, not only the first.
	//
	// Judging by `attested[0]` alone meant an attacker who PREPENDS a bogus attestation to
	// a genuine proof makes the user read "proof does not verify" for a validly timestamped
	// document — and a legitimate multi-branch proof whose first branch uses an op `compute`
	// declines was a hard error even though a later branch would have confirmed.
	// maxAttestationsExamined bounds the network work an attacker-supplied proof can force.
	//
	// `parseSequences` admits up to maxProofInstructions (100,000) and one attestation is one
	// instruction, so before this the loop could drive that many `fetchAgreedHeader` calls —
	// each fanning out to three explorers at two GETs apiece. Roughly 130 KB of `.ots` for
	// 60,000 outbound requests, from the user's IP, with `handleTimestampVerify` passing a
	// context that has **no deadline**: the only bound was the 20 s per GET, not the loop.
	// Nib becomes a request amplifier pointed at third parties and the user gets rate-limited
	// by blockstream.info and mempool.space.
	//
	// Real proofs carry a handful of branches. The memo below means the cap counts DISTINCT
	// heights, so a proof with many attestations at one height costs one fetch.
	const maxAttestationsExamined = 8

	type header struct {
		merkle []byte
		t      time.Time
		n      int
	}
	seen := map[uint64]header{}
	// ATTEMPTS, not cache entries.
	//
	// The first version bounded len(seen), which only grows on a SUCCESSFUL lookup — so a
	// proof carrying two hundred unresolvable heights drove two hundred fetches and never
	// tripped the cap. Caught by the test rather than by reading it back. What has to be
	// bounded is the outbound work an attacker can force, and that is attempts.
	attempts := 0
	var fetchErr error // a network refusal, remembered rather than acted on immediately
	var lastComputeErr error

	for _, s := range attested {
		root, err := s.compute(ctx, p.digest)
		if cerr := ctx.Err(); cerr != nil {
			return nil, fmt.Errorf("verification ran out of time: %w", cerr)
		}
		if err != nil {
			// A branch this build cannot walk (an op `compute` declines) is not a verdict
			// about the document — a later branch may still confirm it.
			lastComputeErr = err
			continue
		}
		hdr, cached := seen[s.height]
		if !cached {
			if attempts >= maxAttestationsExamined {
				// Refuse rather than truncate silently: a proof past the cap is not one
				// this build can judge, and saying "invalid" about it would be a verdict
				// we did not reach.
				return nil, fmt.Errorf("this proof carries more than %d distinct Bitcoin "+
					"attestations, which is not a shape a genuine timestamp takes",
					maxAttestationsExamined)
			}
			attempts++
			merkle, t, n, ferr := fetchAgreedHeader(ctx, sources, minAgree, s.height)
			if ferr != nil {
				// NOT terminal, and that correction is the point.
				//
				// The loop was added to stop an attacker who PREPENDS a bogus attestation
				// from making the user read "proof does not verify" for a validly
				// timestamped document — and it handled two prepend shapes while leaving
				// the cheapest one open: an attestation at a height NOTHING resolves. Every
				// explorer 404s, this returns an error, and returning it here refused the
				// document before the genuine branch was ever computed. ~13 bytes of edit.
				//
				// "The network refusal is the same for every branch" is true of a transport
				// failure and false of a refusal that is ABOUT THIS HEIGHT — that is a
				// property of the branch. So it is remembered: if no branch confirms and a
				// fetch failed, the error is reported (we have no opinion) rather than
				// StateInvalid (a negative verdict we did not earn).
				fetchErr = ferr
				continue
			}
			hdr = header{merkle, t, n}
			seen[s.height] = hdr
		}
		if !bytes.Equal(root, hdr.merkle) {
			// This branch does not attest to this document; another may.
			continue
		}
		res := &VerifyResult{State: StateConfirmed, Height: s.height, Time: hdr.t, Sources: hdr.n}
		if upgradedAny {
			res.Upgraded = serialize(p.digest, updated) // a now self-contained proof to persist
		}
		return res, nil
	}
	// Nothing confirmed. StateInvalid — "this proof does not verify" — is earned only when
	// EVERY branch was walked, fetched and compared. One branch left unchecked is no opinion,
	// whatever the others said (/pending 645): the test here was "some branch was checked",
	// so a decoy that fetched and mismatched, beside a genuine branch whose height no
	// explorer answered just then, read as a negative verdict instead of "try again". The
	// file is unsigned, so appending that decoy is all it took.
	if fetchErr != nil {
		return nil, fetchErr
	}
	if lastComputeErr != nil {
		return nil, lastComputeErr
	}
	return &VerifyResult{State: StateInvalid}, nil
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
	// Both fields are cross-checked, not just the merkle root.
	//
	// This compared `r.merkle` across sources and then returned `results[0].t` — and
	// `results` is appended from N goroutines under a mutex, so `results[0]` is whichever
	// explorer answered FIRST. The block TIME, which is the entire product of a timestamp
	// verification, came from one unagreed source; a hostile explorer cannot forge that a
	// proof verifies (agreement on the root still has to hold against the others) but it
	// could re-date a genuine attestation, and it can always be the fastest responder.
	// A threshold below one would let zero answers through to `results[0]` below, a panic on an
	// exported entry point. No caller passes one today; the floor is the function's, not theirs.
	if minAgree < 1 {
		minAgree = 1
	}
	var (
		mu      sync.Mutex
		results []res
		wg      sync.WaitGroup
	)
	for _, src := range sources {
		wg.Add(1)
		go func(src BlockSource) {
			defer safe.Recover("ots block-header fetch")
			defer wg.Done()
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
		if !r.t.Equal(results[0].t) {
			return nil, time.Time{}, 0, errors.New("block explorers disagree on the block's time — refusing to trust")
		}
	}
	return results[0].merkle, results[0].t, len(results), nil
}

// upgrade folds a pending calendar commitment into a complete Bitcoin-attested
// sequence by asking the calendar for the path to a block. It works in memory and
// does not persist the upgraded proof. ok is false if the calendar has not yet
// confirmed the commitment (still pending): only a 404 says that, as in the reference
// client's calendar.get_timestamp; any other status is an error, not an answer.
// budget is how many operations the calendar's tail may add (see VerifyProof).
func upgrade(ctx context.Context, client *http.Client, s sequence, digest []byte, budget int) (sequence, bool, error) {
	commitment, err := s.compute(ctx, digest)
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
	if resp.StatusCode == http.StatusNotFound {
		return s, false, nil // not yet confirmed
	}
	if resp.StatusCode != http.StatusOK {
		return s, false, fmt.Errorf("calendar %s returned %d", cu.Host, resp.StatusCode)
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
			if len(t.ops) > budget {
				return s, false, fmt.Errorf("%w: the calendar's path adds %d operations to a proof "+
					"with room for %d", ErrProofTooComplex, len(t.ops), budget)
			}
			up := sequence{ops: append(append([]op{}, s.ops...), t.ops...), height: t.height}
			if err := up.checkLengths(); err != nil {
				return s, false, err
			}
			return up, true, nil
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
// — the calendar commitment (pending) or the block merkle root (Bitcoin). Every op
// passes opResultLen BEFORE it allocates, and ctx is consulted every
// computeCheckEvery ops, so the caller's deadline bounds it (/pending 781).
func (s sequence) compute(ctx context.Context, digest []byte) ([]byte, error) {
	cur := append([]byte{}, digest...)
	for i, o := range s.ops {
		if i%computeCheckEvery == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		if !executable(o.tag) {
			return nil, fmt.Errorf("proof uses unsupported operation 0x%02x", o.tag)
		}
		if _, err := opResultLen(len(cur), o); err != nil {
			return nil, err
		}
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
	for _, s := range seqs {
		if err := s.checkLengths(); err != nil {
			return nil, err
		}
	}
	return &proof{digest: append([]byte{}, digest...), seqs: seqs}, nil
}

// maxProofInstructions bounds the total operations one .ots may materialize while
// parsing. A checkpoint (0xff) duplicates the whole instruction block built so far,
// so N operations followed by N checkpoints materializes N² instructions out of 2N
// bytes of input — measured at 876 MiB from an 8 KB file, and cleanly quadratic
// (4× the input is 16× the memory), so ~32 KB reaches tens of gigabytes.
//
// That is an OOM of the entire app — taking every open document with it, and past
// anything safe.Recover can catch — from a file the user was merely asked to check.
// The bound is on total work rather than on checkpoint count because the
// amplification is inherent to what a checkpoint MEANS: a shared prefix is
// legitimately duplicated once per branch, so no restructuring of the copy removes
// it (making checkpoints lazy just moves the quadratic onto the attestations that
// pop them). Real proofs carry tens of operations; this ceiling is four orders
// above any of them and holds the parse to a few MiB.
//
// Its sibling is maxOpResult, which holds each op's RESULT — what compute copies — so
// the two together bound compute's work: this one the count, that one the bytes per
// op. An upgrade's spliced calendar ops are charged against this same figure
// (VerifyProof's budget), not given a second one of their own.
const maxProofInstructions = 100_000

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
	// Every instruction materialized, across live blocks and saved checkpoints alike.
	// Counted where memory is actually committed, not where bytes are read: one
	// checkpoint byte can commit thousands of instructions.
	total := 0

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
			total++
			if total > maxProofInstructions {
				return nil, ErrProofTooComplex
			}
			if n := len(checkpoints); n > 0 {
				blocks = append(blocks, checkpoints[n-1])
				checkpoints = checkpoints[:n-1]
				cur++
			}
		case tagCheckpoint:
			b := blocks[cur]
			// Charged BEFORE the copy: the whole point is that this one byte can
			// commit an arbitrarily large allocation, so checking afterwards would
			// mean the allocation the bound exists to refuse has already happened.
			total += len(b)
			if total > maxProofInstructions {
				return nil, ErrProofTooComplex
			}
			cp := make([]instr, len(b))
			copy(cp, b)
			checkpoints = append(checkpoints, cp)
		default:
			in := instr{tag: tag}
			if tag == opAppend || tag == opPrepend {
				arg, err := readOpArg(c)
				if err != nil {
					return nil, err
				}
				in.arg = arg
			} else if !knownNoArgOp(tag) {
				return nil, fmt.Errorf("unknown operation tag 0x%02x", tag)
			}
			blocks[cur] = append(blocks[cur], in)
			total++
			if total > maxProofInstructions {
				return nil, ErrProofTooComplex
			}
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

// isBlockHash reports whether s is exactly 64 lowercase-or-uppercase hex characters.
func isBlockHash(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f', c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return true
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
	// The hash is checked before it becomes a path segment.
	//
	// `hashHex` is up to 64 KiB of whatever the explorer returned, interpolated straight
	// into the next URL — so one source controlled path segments, `?` and `#` in Nib's
	// follow-up call. Scheme and host are fixed, so this was never SSRF to a third party;
	// it let a compromised explorer redirect the second call within its own API surface,
	// and it also catches an HTML error page being pasted into the path.
	hash := strings.TrimSpace(string(hashHex))
	if !isBlockHash(hash) {
		return nil, time.Time{}, fmt.Errorf("%s returned something that is not a block hash", e.base)
	}
	hdrHex, err := e.get(ctx, e.base+"/block/"+hash+"/header")
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
